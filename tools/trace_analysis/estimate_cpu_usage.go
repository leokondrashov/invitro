package main

import (
	"sync"
	"time"

	"github.com/vhive-serverless/loader/pkg/common"
)

type cpuRecord struct {
	Timestamp float64 `csv:"timestamp"`
	Function  int     `csv:"function"`
	CPU       float64 `csv:"cpu"`
}

func estimateCPUUsage(functions []*common.Function, duration int, slowdown float64, allRecordsWritten *sync.WaitGroup, writer chan interface{}, threads int) {
	var allFunctionsProcessed sync.WaitGroup

	limiter := make(chan struct{}, threads)

	for i, function := range functions {
		allFunctionsProcessed.Add(1)
		limiter <- struct{}{}

		go func() {
			defer allFunctionsProcessed.Done()
			defer func() { <-limiter }()

			timeline := generateFunctionTimelineCompressed(function, duration, slowdown)
			avgTimeline := averageTimeline(timeline, time.Second)
			for _, entry := range avgTimeline {
				writer <- cpuRecord{entry.Timestamp, i, entry.Concurrency}
			}
		}()
	}
	allFunctionsProcessed.Wait()
	close(writer)
	allRecordsWritten.Wait()
}
