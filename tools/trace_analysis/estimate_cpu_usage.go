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

func estimateCPUUsage(functions []*common.Function, duration int, slowdown float64, allRecordsWritten *sync.WaitGroup, writer chan interface{}) {
	var allFunctionsProcessed sync.WaitGroup

	limiter := make(chan struct{}, 12)

	for i, function := range functions {
		allFunctionsProcessed.Add(1)
		limiter <- struct{}{}

		funcWriter := make(chan AvgTimelineEntry)
		go func() {
			for t, ok := <-funcWriter; ok; t, ok = <-funcWriter {
				writer <- cpuRecord{
					t.Timestamp,
					i,
					t.Concurrency,
				}
			}
		}()

		go func() {
			defer allFunctionsProcessed.Done()
			defer func() { <-limiter }()
			defer close(funcWriter)

			timeline := generateFunctionTimelineCompressed(function, duration, slowdown)
			avgTimeline := averageTimeline(timeline, time.Second)
			for _, entry := range avgTimeline {
				funcWriter <- entry
			}
		}()
	}
	allFunctionsProcessed.Wait()
	close(writer)
	allRecordsWritten.Wait()
}
