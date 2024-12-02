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
	"flag"
	"os"
	"slices"
	"sync"
	"time"

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
	iatDistribution = flag.String("iatDistribution", "exponential", "IAT distribution, one of [exponential(_shift), uniform(_shift), equidistant(_shift)]")
	randSeed        = flag.Uint64("randSeed", 42, "Seed for the random number generator")
	keepalive       = flag.Int("keepalive", 6, "Keepalive period in seconds")
	typeFlag        = flag.String("type", "coldstart", "Type of analysis to perform, one of [coldstart, cpu, memory]")
	slowdown        = flag.Float64("slowdown", 1.0, "Slowdown factor for each invocation for the analysis")
	threads         = flag.Int("j", 12, "Number of threads to use for processing")
)

type coldStartRecord struct {
	Timestamp   int `csv:"timestamp"`
	FunctionNum int `csv:"functionNum"`
}

func main() {
	flag.Parse()

	writer, written, functions := commonInit(*outputFile, *tracePath, *duration, *iatDistribution, *randSeed)

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

func commonInit(outputFilename string, tracePath string, duration int, iatDistribution string, randSeed uint64) (chan interface{}, *sync.WaitGroup, []*common.Function) {
	var allRecordsWritten sync.WaitGroup

	iatType, shift := parseIATDistribution(iatDistribution)

	writer := make(chan interface{}, 1000)

	traceParser := trace.NewAzureParser(tracePath, duration)
	functions := traceParser.Parse("Knative")

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

	for i, function := range functions {
		spec := specGenerator.GenerateInvocationData(function, iatType, shift, common.MinuteGranularity)
		functions[i].Specification = spec
	}

	return writer, &allRecordsWritten, functions
}

func coldStarts(functions []*common.Function, duration int, keepalive int, allRecordsWritten *sync.WaitGroup, writer chan interface{}, threads int) {
	var allFunctionsProcessed sync.WaitGroup

	granularity := time.Millisecond
	limiter := make(chan struct{}, threads)

	for i, function := range functions {
		allFunctionsProcessed.Add(1)
		limiter <- struct{}{}

		funcWriter := make(chan int)
		go func() {
			for t, ok := <-funcWriter; ok; t, ok = <-funcWriter {
				writer <- coldStartRecord{
					t,
					i,
				}
			}
		}()

		go func() {
			defer allFunctionsProcessed.Done()
			defer func() { <-limiter }()
			defer close(funcWriter)

			timeline := generateFunctionTimeline(function, duration, granularity)
			getColdStarts(timeline, keepalive*int(time.Second/granularity), funcWriter)
		}()
	}
	allFunctionsProcessed.Wait()
	close(writer)
	allRecordsWritten.Wait()
}

func getColdStarts(concurrency []int, keepalive int, writer chan int) {
	capacity := 0
	for i, c := range concurrency {
		if i == 0 {
			capacity = 0
		} else if c <= concurrency[i-1] {
			continue
		} else {
			capacity = slices.Max(concurrency[max(0, i-keepalive):i])
		}
		for ; capacity < c; capacity++ {
			writer <- i
		}
	}
}
