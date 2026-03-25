package deployment

import (
	"bytes"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-cmd/cmd"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
)

const (
	bareMetalLbGateway = "10.200.3.4.sslip.io" // Address of the bare-metal load balancer.
	namespace          = "default"
)

var (
	urlRegex = regexp.MustCompile("at URL:\nhttp://([^\n]+)")
)

type knativeDeployer struct {
	reuseFunctions bool
}

type knativeDeploymentConfiguration struct {
	IsPartiallyPanic  bool
	EndpointPort      int
	AutoscalingMetric string
	ReuseFunctions    bool
}

func newKnativeDeployer() *knativeDeployer {
	return &knativeDeployer{}
}

func newKnativeDeployerConfiguration(cfg *config.Configuration) knativeDeploymentConfiguration {
	return knativeDeploymentConfiguration{
		IsPartiallyPanic:  cfg.LoaderConfiguration.IsPartiallyPanic,
		EndpointPort:      cfg.LoaderConfiguration.EndpointPort,
		AutoscalingMetric: cfg.LoaderConfiguration.AutoscalingMetric,
		ReuseFunctions:    cfg.LoaderConfiguration.ReuseDeployedFunctions,
	}
}

func (d *knativeDeployer) Deploy(cfg *config.Configuration) {
	knativeConfig := newKnativeDeployerConfiguration(cfg)
	d.reuseFunctions = knativeConfig.ReuseFunctions

	queue := make(chan struct{}, runtime.NumCPU()) // message queue as a sync method
	deployed := sync.WaitGroup{}
	deployed.Add(len(cfg.Functions))

	for i := 0; i < len(cfg.Functions); i++ {
		index := i
		go func(function *common.Function) {
			queue <- struct{}{}

			defer deployed.Done()
			defer func() { <-queue }()

			knativeDeploySingleFunction(
				function,
				function.YAMLPath,
				knativeConfig.IsPartiallyPanic,
				knativeConfig.EndpointPort,
				knativeConfig.AutoscalingMetric,
				knativeConfig.ReuseFunctions,
			)
		}(cfg.Functions[index])
	}

	deployed.Wait()
	start := time.Now()
	cmd := exec.Command("kubectl", "annotate", "--overwrite", "PodAutoscaler", "-n", "default", "--all", fmt.Sprintf("autoscaling.knative.dev/min-scale=%d", cfg.LoaderConfiguration.MinScale))
	err := cmd.Start()
	if err != nil {
		log.Fatalf("Failed to execute script: %v", err)
	}
	elapsed := time.Since(start)
	log.Printf("Script executed in %s", elapsed)
}

func (d *knativeDeployer) Clean() {
	if d.reuseFunctions {
		log.Info("ReuseDeployedFunctions is enabled - skipping Knative cleanup to preserve functions.")
		return
	}

	cmd := exec.Command("kn", "service", "delete", "--all")

	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Errorf("Unable to delete Knative services - %s", err)
	}

	preDepCmd := exec.Command("kubectl", "delete", "services", "--all", "--force", "--grace-period=0")
	preDepCmd.Stdout = &out
	if err := preDepCmd.Run(); err != nil {
		log.Error("Unable to clean up predeployment files")
	}
	preDepCmd = exec.Command("kubectl", "delete", "deployment", "--all", "--force", "--grace-period=0")
	preDepCmd.Stdout = &out
	if err := preDepCmd.Run(); err != nil {
		log.Error("Unable to clean up predeployment files")
	}
	preDepCmd = exec.Command("kubectl", "delete", "jobs", "--all", "--force", "--grace-period=0")
	preDepCmd.Stdout = &out
	if err := preDepCmd.Run(); err != nil {
		log.Error("Unable to clean up predeployment files")
	}
}

func knativeDeploySingleFunction(function *common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int, autoscalingMetric string, reuseFunctions bool) bool {
	panicWindow := "\"10.0\""
	panicThreshold := "\"200.0\""
	if isPartiallyPanic {
		panicWindow = "\"100.0\""
		panicThreshold = "\"1000.0\""
	}
	autoscalingTarget := 100 // default for concurrency
	if autoscalingMetric == "rps" {
		autoscalingTarget = int(math.Round(1000.0 / function.RuntimeStats.Average))
		// for rps mode use the average runtime in milliseconds to determine how many requests a pod can process per
		// second, then round to an integer as that is what the knative config expects
	}

	deployName := function.Name
	if reuseFunctions {
		existingName, found, err := findKnativeServiceByCanonicalName(function.Name)
		if err != nil {
			log.Warnf("Failed to discover existing Knative service for %s: %v", function.Name, err)
		} else if found {
			deployName = existingName
			if err := updateKnativeServiceScales(deployName, function.InitialScale); err != nil {
				log.Warnf("Failed to update scales for existing function %s: %v", deployName, err)
				return false
			}

			function.Endpoint = fmt.Sprintf("%s.%s.%s:%d", deployName, namespace, bareMetalLbGateway, endpointPort)
			log.Debugf("Reused existing function %s on %s", deployName, function.Endpoint)
			return true
		}
	}

	for _, path := range function.PredeploymentPath {
		envCmd := cmd.NewCmd("kubectl", "apply", "-f", path)
		status := <-envCmd.Start()

		for _, line := range status.Stdout {
			fmt.Println("Predeployment command response is " + line)
		}
	}
	cmd := exec.Command(
		"bash",
		"./pkg/driver/deployment/knative.sh",
		yamlPath,
		deployName,

		strconv.Itoa(function.CPURequestsMilli)+"m",
		strconv.Itoa(function.CPULimitsMilli)+"m",
		strconv.Itoa(function.MemoryRequestsMiB)+"Mi",
		strconv.Itoa(function.InitialScale),
		panicWindow,
		panicThreshold,

		wrapString(autoscalingMetric),
		wrapString(strconv.Itoa(autoscalingTarget)),

		wrapString(strconv.Itoa(function.ColdStartBusyLoopMs)),
	)

	stdoutStderr, err := cmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))
	if err != nil {
		// TODO: there should be a toggle to turn off deployment because if this is fatal then we cannot test the thing locally
		log.Warnf("Failed to deploy function %s: %v\n%s\n", function.Name, err, stdoutStderr)
		return false
	}
	if endpointMatch := urlRegex.FindStringSubmatch(string(stdoutStderr)); len(endpointMatch) > 1 && endpointMatch[1] != function.Endpoint {
		endpoint := endpointMatch[1]
		// TODO: check when this situation happens
		log.Debugf("Update function endpoint to %s\n", endpoint)
		function.Endpoint = endpoint
	} else {
		function.Endpoint = fmt.Sprintf("%s.%s.%s", deployName, namespace, bareMetalLbGateway)
	}
	// adding port to the endpoint
	function.Endpoint = fmt.Sprintf("%s:%d", function.Endpoint, endpointPort)
	log.Debugf("Deployed function on %s\n", function.Endpoint)

	return true
}

func wrapString(value string) string {
	return "\"" + value + "\""
}

func canonicalFunctionName(functionName string) string {
	parts := strings.Split(functionName, "-")
	if len(parts) <= 1 {
		return functionName
	}

	return strings.Join(parts[:len(parts)-1], "-")
}

func findKnativeServiceByCanonicalName(functionName string) (string, bool, error) {
	canonicalName := canonicalFunctionName(functionName)

	cmd := exec.Command("kubectl", "get", "ksvc", "-n", namespace, "-o", "custom-columns=NAME:.metadata.name", "--no-headers")
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("kubectl get ksvc failed: %w (%s)", err, strings.TrimSpace(string(stdoutStderr)))
	}

	var matches []string
	for _, line := range strings.Split(string(stdoutStderr), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}

		if name == canonicalName || strings.HasPrefix(name, canonicalName+"-") {
			matches = append(matches, name)
		}
	}

	if len(matches) == 0 {
		return "", false, nil
	}

	// Prefer exact canonical names first, then stable deterministic selection.
	sort.Strings(matches)
	for _, name := range matches {
		if name == canonicalName {
			return name, true, nil
		}
	}

	return matches[0], true, nil
}

func updateKnativeServiceScales(serviceName string, scale int) error {
	scaleValue := strconv.Itoa(scale)
	// Some Knative versions may reject changing initial-scale for existing services.
	patchWithMinOnly := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{"autoscaling.knative.dev/min-scale":"%s"}}}}}`,
		scaleValue,
	)
	cmd := exec.Command(
		"kubectl",
		"patch",
		"ksvc",
		serviceName,
		"-n",
		namespace,
		"--type=merge",
		"-p",
		patchWithMinOnly,
	)

	stdoutStderrMinOnly, errMinOnly := cmd.CombinedOutput()
	if errMinOnly != nil {
		return fmt.Errorf(
			"kubectl patch failed (min-only: %s)",
			strings.TrimSpace(string(stdoutStderrMinOnly)),
		)
	}

	return nil
}
