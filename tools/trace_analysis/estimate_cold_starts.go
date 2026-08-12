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

package main

import (
	"encoding/csv"
	"flag"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gocarina/gocsv"
	log "github.com/sirupsen/logrus"

	"github.com/vhive-serverless/loader/pkg/common"
	spec "github.com/vhive-serverless/loader/pkg/generator"
	trace "github.com/vhive-serverless/loader/pkg/trace"
)

var (
	tracePath       = flag.String("tracePath", "data/traces/", "Path to folder where the trace is located")
	outputFile      = flag.String("outputFile", "output.csv", "Path to output file")
	duration        = flag.Int("duration", 1440, "Duration of the traces in minutes")
	skip_duration   = flag.Int("skip_duration", 00, "Duration of the trace to skip")
	iatDistribution = flag.String("iatDistribution", "exponential", "IAT distribution, one of [exponential(_shift), uniform(_shift), equidistant(_shift)]")
	randSeed        = flag.Uint64("randSeed", 42, "Seed for the random number generator")
	keepalive       = flag.Int("keepalive", 6, "Keepalive period in seconds")
	typeFlag        = flag.String("type", "coldstart", "Type of analysis to perform, one of [coldstart, cpu, memory]")
	slowdown        = flag.Float64("slowdown", 1.0, "Slowdown factor for each invocation for the analysis")
	threads         = flag.Int("j", 12, "Number of threads to use for processing")
)

type coldStartRecord struct {
	Timestamp   float64 `csv:"timestamp"`
	FunctionNum int     `csv:"functionNum"`
}

func main() {
	flag.Parse()

	writer, written, functions := commonInit(*outputFile, *tracePath, *duration, *skip_duration, *iatDistribution, *randSeed)

	switch *typeFlag {
	case "coldstart":
		coldStarts(functions, *duration, *keepalive, written, writer, *threads)
	case "cpu":
		estimateCPUUsage(functions, *duration, *slowdown, written, writer, *threads)
	case "memory":
		estimateMemoryUsage(functions, *duration, *slowdown, *keepalive, written, writer, *threads)
	}
}

func parseIATDistribution(iat string) (common.IatDistribution, bool) {
	switch iat {
	case "exponential":
		return common.Exponential, false
	case "exponential_shift":
		return common.Exponential, true
	case "gamma":
		return common.Gamma, false
	case "gamma_shift":
		return common.Gamma, true
	case "uniform":
		return common.Uniform, false
	case "uniform_shift":
		return common.Uniform, true
	case "equidistant":
		return common.Equidistant, false
	default:
		log.Fatal("Unsupported IAT distribution.")
	}

	return common.Exponential, false
}

func commonInit(outputFilename string, tracePath string, duration, skip_duration int, iatDistribution string, randSeed uint64) (chan interface{}, *sync.WaitGroup, []*common.Function) {
	var allRecordsWritten sync.WaitGroup

	iatType, shift := parseIATDistribution(iatDistribution)

	writer := make(chan interface{}, 1000)

	traceInfo, err := os.Stat(tracePath)
	if err != nil {
		log.Fatalf("Unable to access trace path %q: %v", tracePath, err)
	}

	// Azure 2021 traces consist of one CSV file, whereas Azure 2019 traces
	// are directories containing invocations.csv, durations.csv, and memory.csv.
	// The Azure 2021 parser already creates a function specification from the
	// invocation timestamps and durations, so it must not be regenerated below.
	azure2021Trace := !traceInfo.IsDir()
	var functions []*common.Function
	if azure2021Trace {
		functions = trace.NewAzure2021Parser(tracePath, duration, "").Parse()
	} else {
		functions = trace.NewAzureParser(tracePath, duration, "", skip_duration).Parse()
	}

	// Azure2021 functions are initially collected in a map, so their parser
	// order is not stable.  Derive the persistent ID from the generated name
	// (without its random suffix), then sort before assigning function numbers.
	sortFunctionsByID(functions, azure2021Trace)
	writeFunctionMapping(outputFilename, functions, azure2021Trace)

	log.Infof("Traces contain the following %d functions:\n", len(functions))

	allRecordsWritten.Add(1)
	go func() {
		defer allRecordsWritten.Done()
		f, err := os.Create(outputFilename)
		if err != nil {
			log.Fatal(err)
		}
		_ = gocsv.MarshalChan(writer, gocsv.DefaultCSVWriter(f))
		f.Close()
	}()

	specGenerator := spec.NewSpecificationGenerator(randSeed)

	if !azure2021Trace {
		for i, function := range functions {
			spec := specGenerator.GenerateInvocationData(function, iatType, shift, common.MinuteGranularity)
			functions[i].Specification = spec
		}
	}

	return writer, &allRecordsWritten, functions
}

// functionID returns the identifier used to assign functionNum. Azure2021
// names end in a random numeric component; it is deliberately excluded so the
// same trace gets the same IDs in separate analyzer invocations.
func functionID(functionName string, azure2021Trace bool) string {
	if !azure2021Trace {
		return functionName
	}

	lastDash := strings.LastIndex(functionName, "-")
	if lastDash == -1 {
		return functionName
	}
	return functionName[:lastDash]
}

func sortFunctionsByID(functions []*common.Function, azure2021Trace bool) {
	sort.Slice(functions, func(i, j int) bool {
		return functionID(functions[i].Name, azure2021Trace) < functionID(functions[j].Name, azure2021Trace)
	})

	for i := 1; i < len(functions); i++ {
		if functionID(functions[i-1].Name, azure2021Trace) == functionID(functions[i].Name, azure2021Trace) {
			log.Fatalf("duplicate stable function ID %q; cannot assign reproducible function numbers", functionID(functions[i].Name, azure2021Trace))
		}
	}
}

func functionMappingFilename(outputFilename string) string {
	return outputFilename + ".function_map.csv"
}

func writeFunctionMapping(outputFilename string, functions []*common.Function, azure2021Trace bool) {
	f, err := os.Create(functionMappingFilename(outputFilename))
	if err != nil {
		log.Fatalf("unable to create function mapping: %v", err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	if err := writer.Write([]string{"functionNum", "functionId", "functionName"}); err != nil {
		log.Fatalf("unable to write function mapping header: %v", err)
	}
	for i, function := range functions {
		if err := writer.Write([]string{strconv.Itoa(i), functionID(function.Name, azure2021Trace), function.Name}); err != nil {
			log.Fatalf("unable to write function mapping: %v", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		log.Fatalf("unable to write function mapping: %v", err)
	}
}

func coldStarts(functions []*common.Function, duration int, keepalive int, allRecordsWritten *sync.WaitGroup, writer chan interface{}, threads int) {
	var allFunctionsProcessed sync.WaitGroup

	limiter := make(chan struct{}, threads)

	for i, function := range functions {
		allFunctionsProcessed.Add(1)
		limiter <- struct{}{}

		funcWriter := make(chan float64)
		go func() {
			defer allFunctionsProcessed.Done()
			for t, ok := <-funcWriter; ok; t, ok = <-funcWriter {
				writer <- coldStartRecord{
					t,
					i,
				}
			}
		}()

		go func() {
			defer func() { <-limiter }()
			defer close(funcWriter)

			timeline := generateFunctionTimelineCompressed(function, duration, *slowdown)
			instances := generateInstanceTimeline(timeline, keepalive)
			getColdStarts(instances, funcWriter)
		}()
	}
	allFunctionsProcessed.Wait()
	close(writer)
	allRecordsWritten.Wait()
}

func getColdStarts(timeline []TimelineEntry, writer chan float64) {
	for i, entry := range timeline {
		if i == 0 {
			if entry.Concurrency > 0 {
				writer <- entry.Timestamp
			}
		} else if entry.Concurrency > timeline[i-1].Concurrency {
			writer <- entry.Timestamp
		}
	}
}
