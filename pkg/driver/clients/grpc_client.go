/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package clients

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/workload/proto"
	grpcClients "github.com/vhive-serverless/vSwarm-proto/grpcclient"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	mc "github.com/vhive-serverless/loader/pkg/metric"
)

type invoker interface {
	Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification, conn *grpc.ClientConn, record *mc.ExecutionRecord, executionCxt context.Context) bool
}

type ExecutorRPC struct {
}

func (i ExecutorRPC) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification, conn *grpc.ClientConn, record *mc.ExecutionRecord, executionCxt context.Context) bool {
	grpcClient := proto.NewExecutorClient(conn)

	response, err := grpcClient.Execute(executionCxt, &proto.FaasRequest{
		Message:           "nothing",
		RuntimeInMilliSec: uint32(runtimeSpec.Runtime),
		MemoryInMebiBytes: uint32(runtimeSpec.Memory),
	})

	if err != nil {
		logrus.Debugf("gRPC timeout exceeded for function %s - %s", function.Name, err)

		record.ConnectionTimeout = true // WithBlock deprecated in new gRPC interface
		record.FunctionTimeout = true

		return false
	}

	record.Instance = extractInstanceName(response.GetMessage())
	record.ActualDuration = response.DurationInMicroSec

	if strings.HasPrefix(response.GetMessage(), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = common.Kib2Mib(response.MemoryUsageInKb)
	}

	logrus.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, response.Message,
		float64(response.DurationInMicroSec)/1e3, common.Kib2Mib(response.MemoryUsageInKb))

	return true
}

type SayHelloRPC struct {
}

func (i SayHelloRPC) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification, conn *grpc.ClientConn, record *mc.ExecutionRecord, executionCxt context.Context) bool {
	_ = runtimeSpec

	grpcStart := time.Now()
	invocationParams := resolveInvocationParameters(function)
	if function.VSwarmClient == nil {
		logrus.Debugf("Missing initialized vSwarm gRPC client for function %s", function.Name)
		record.ConnectionTimeout = true
		record.FunctionTimeout = true
		return false
	}

	generator := function.VSwarmClient.GetGenerator()
	generator.SetGenerator(grpcClients.StringToGeneratorType(invocationParams.Generator))
	generator.SetLowerBound(invocationParams.LowerBound)
	generator.SetUpperBound(invocationParams.UpperBound)
	generator.SetValue(invocationParams.Value)
	generator.SetMethod(invocationParams.Method)
	in := generator.Next()
	record.GRPCConnectionEstablishTime += time.Since(grpcStart).Microseconds()

	var trailer metadata.MD
	_, err := function.VSwarmClient.Request(executionCxt, in, grpc.Trailer(&trailer))
	if err != nil {
		logrus.Debugf("gRPC invocation failed for function %s - %s", function.Name, err)
		record.ConnectionTimeout = true
		record.FunctionTimeout = true

		return false
	}
	// logrus.Debugf("Received response for function %s, trailers: %v", function.Name, trailer)
	var delay uint64
	if val, ok := trailer["function-delay"]; ok {
		logrus.Tracef("Received delay from activator: %s\n", val[0])
		delay, err = strconv.ParseUint(val[0], 10, 32)
	}
	if val, ok := trailer["node-overhead"]; ok {
		logrus.Tracef("Received overhead from activator: %s\n", val[0])
		overhead, _ := strconv.ParseUint(val[0], 10, 32)
		delay -= overhead
	}
	if err != nil {
		logrus.Debugf("Failed to parse function delay for function %s - %s", function.Name, err)
	}
	record.ActualDuration = uint32(delay)
	record.Instance = invocationParams.FunctionName
	record.ActualMemoryUsage = common.Kib2Mib(0) //Memory usage may not be available for all vSwarm benchmarks

	// logrus.Tracef("(Replied)\t %s: %s", function.Name, response)

	return true
}

type grpcInvoker struct {
	cfg                        *config.LoaderConfiguration
	invoker                    invoker
	initializedVSwarmFunctions []*common.Function
}

func newGRPCInvoker(cfg *config.LoaderConfiguration, invoker invoker) *grpcInvoker {
	return &grpcInvoker{
		cfg:     cfg,
		invoker: invoker,
	}
}

func (i *grpcInvoker) InitializeFunctions(functions []*common.Function) error {
	if !i.cfg.VSwarm {
		return nil
	}

	i.initializedVSwarmFunctions = i.initializedVSwarmFunctions[:0]
	for _, function := range functions {
		if err := i.initializeVSwarmFunction(function); err != nil {
			return err
		}
		i.initializedVSwarmFunctions = append(i.initializedVSwarmFunctions, function)
	}

	return nil
}

func (i *grpcInvoker) Close() {
	for _, function := range i.initializedVSwarmFunctions {
		if function != nil && function.VSwarmClient != nil {
			function.VSwarmClient.Close()
			function.VSwarmClient = nil
		}
	}
	i.initializedVSwarmFunctions = nil
}

func (i *grpcInvoker) initializeVSwarmFunction(function *common.Function) error {
	if function.VSwarmClient != nil {
		return nil
	}

	invocationParams := resolveInvocationParameters(function)
	host, port, err := net.SplitHostPort(function.Endpoint)
	if err != nil {
		return err
	}

	serviceName := grpcClients.FindServiceName(invocationParams.FunctionName)
	vswarmClient := grpcClients.FindGrpcClient(serviceName)

	connectionCtx, cancel := context.WithTimeout(context.Background(), time.Duration(i.cfg.GRPCConnectionTimeoutSeconds)*time.Second)
	defer cancel()
	if err := vswarmClient.Init(connectionCtx, host, port); err != nil {
		return err
	}
	function.VSwarmClient = vswarmClient

	return nil
}

func (i *grpcInvoker) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification) (bool, *mc.ExecutionRecord) {
	logrus.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	record := &mc.ExecutionRecord{
		ExecutionRecordBase: mc.ExecutionRecordBase{
			RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
		},
	}

	////////////////////////////////////
	// INVOKE FUNCTION
	////////////////////////////////////
	start := time.Now()
	record.StartTime = start.UnixMicro()
	var conn *grpc.ClientConn
	var err error

	if !i.cfg.VSwarm {
		var dialOptions []grpc.DialOption
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if strings.Contains(i.cfg.Platform, common.PlatformDirigent) {
			dialOptions = append(dialOptions, grpc.WithAuthority(function.Name)) // Dirigent specific
		}
		if i.cfg.EnableZipkinTracing {
			dialOptions = append(dialOptions, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
		}

		grpcStart := time.Now()

		conn, err = grpc.NewClient("passthrough:///"+function.Endpoint, dialOptions...)
		if err != nil {
			logrus.Debugf("Failed to establish a gRPC connection - %v\n", err)

			record.ResponseTime = time.Since(start).Microseconds()
			record.ConnectionTimeout = true

			return false, record
		}
		defer gRPCConnectionClose(conn)

		conn.Connect()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for {
			state := conn.GetState()
			if state == connectivity.Ready {
				break // Connection is established
			}
			// Wait for the state to transition
			if !conn.WaitForStateChange(ctx, state) {
				logrus.Error("Timeout waiting for gRPC connection to become ready")
				record.ResponseTime = time.Since(start).Microseconds()
				record.ConnectionTimeout = true
				record.FunctionTimeout = true
				return false, record
			}
		}
		record.GRPCConnectionEstablishTime = time.Since(grpcStart).Microseconds()
	} else if function.VSwarmClient == nil {
		grpcStart := time.Now()
		if err := i.initializeVSwarmFunction(function); err != nil {
			logrus.Debugf("Failed to initialize vSwarm gRPC client for function %s - %s", function.Name, err)
			record.ResponseTime = time.Since(start).Microseconds()
			record.ConnectionTimeout = true
			record.FunctionTimeout = true
			return false, record
		}
		record.GRPCConnectionEstablishTime = time.Since(grpcStart).Microseconds()
	}

	executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(i.cfg.GRPCFunctionTimeoutSeconds)*time.Second)
	defer cancelExecution()
	success := i.invoker.Invoke(function, runtimeSpec, conn, record, executionCxt)
	record.ResponseTime = time.Since(start).Microseconds()
	logrus.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)
	return success, record
}

func extractInstanceName(data string) string {
	indexOfHyphen := strings.LastIndex(data, common.FunctionNamePrefix)
	if indexOfHyphen == -1 {
		return data
	}

	return data[indexOfHyphen:]
}

func resolveInvocationParameters(function *common.Function) common.InvocationParameters {
	baseFunctionName, inferredLowerBound, inferredUpperBound, hasInferredRange := normalizeInvocationFunctionName(function.Name)

	params := common.InvocationParameters{
		FunctionName: baseFunctionName,
		Generator:    "unique",
		LowerBound:   1,
		UpperBound:   10,
	}
	if hasInferredRange {
		params.Generator = "random"
		params.LowerBound = inferredLowerBound
		params.UpperBound = inferredUpperBound
	}
	if requiresMethodDefault(params.FunctionName) {
		params.Method = "0"
	}

	if function.InvocationParams != nil {
		if function.InvocationParams.FunctionName != "" {
			params.FunctionName = function.InvocationParams.FunctionName
		}
		if function.InvocationParams.Generator != "" {
			params.Generator = function.InvocationParams.Generator
		}
		if function.InvocationParams.LowerBound != 0 {
			params.LowerBound = function.InvocationParams.LowerBound
		}
		if function.InvocationParams.UpperBound != 0 {
			params.UpperBound = function.InvocationParams.UpperBound
		}
		if function.InvocationParams.Value != "" {
			params.Value = function.InvocationParams.Value
		}
		if function.InvocationParams.Method != "" {
			params.Method = function.InvocationParams.Method
		}
	}

	if params.FunctionName == "" {
		params.FunctionName = function.Name
	}
	if params.Method == "" && requiresMethodDefault(params.FunctionName) {
		params.Method = "0"
	}

	if params.UpperBound == 0 {
		params.UpperBound = params.LowerBound
	}

	return params
}

func normalizeInvocationFunctionName(functionName string) (string, int, int, bool) {
	parts := strings.Split(strings.TrimSpace(functionName), "-")
	if len(parts) >= 3 && isInt(parts[len(parts)-1]) && isInt(parts[len(parts)-2]) {
		parts = parts[:len(parts)-2]
	}
	baseName := strings.Join(parts, "-")

	baseParts := strings.Split(baseName, "-")
	if len(baseParts) >= 3 && isInt(baseParts[len(baseParts)-1]) && isInt(baseParts[len(baseParts)-2]) {
		lowerBound, _ := strconv.Atoi(baseParts[len(baseParts)-2])
		upperBound, _ := strconv.Atoi(baseParts[len(baseParts)-1])
		return strings.Join(baseParts[:len(baseParts)-2], "-"), lowerBound, upperBound, true
	}

	return baseName, 0, 0, false
}

func requiresMethodDefault(functionName string) bool {
	switch functionName {
	case "cartservice", "currencyservice", "productcatalogservice", "shippingservice":
		return true
	default:
		return false
	}
}

func isInt(value string) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}

func gRPCConnectionClose(conn *grpc.ClientConn) {
	if conn == nil {
		return
	}

	if err := conn.Close(); err != nil {
		logrus.Warnf("Error while closing gRPC connection - %s\n", err)
	}
}
