package main

import (
	"slices"
	"sync"
	"time"

	"github.com/vhive-serverless/loader/pkg/common"
)

func estimateMemoryUsage(functions []*common.Function, duration int, slowdown float64, keepalive int, allRecordsWritten *sync.WaitGroup, writer chan interface{}, threads int) {
	var allFunctionsProcessed sync.WaitGroup

	limiter := make(chan struct{}, threads)

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
			instanceTimeline := generateInstanceTimeline(timeline, keepalive)
			avgTimeline := averageTimeline(instanceTimeline, time.Second)
			for _, entry := range avgTimeline {
				funcWriter <- entry
			}
		}()
	}
	allFunctionsProcessed.Wait()
	close(writer)
	allRecordsWritten.Wait()
}

// compute max over a range [start, end)
func maxOverRange(timeline []TimelineEntry, start, end float64) int {
	max := 0
	cmp := func(e TimelineEntry, t float64) int {
		if e.Timestamp < t {
			return -1
		} else if e.Timestamp > t {
			return 1
		} else {
			return 0
		}
	}
	low, _ := slices.BinarySearchFunc(timeline, start, cmp)
	if low > 0 {
		low--
	}
	high, _ := slices.BinarySearchFunc(timeline, end, cmp)
	for i := low; i < high; i++ {
		if timeline[i].Concurrency > max {
			max = timeline[i].Concurrency
		}
	}
	return max
}

func generateInstanceTimeline(timeline []TimelineEntry, keepalive int) []TimelineEntry {
	instanceTimeline := make([]TimelineEntry, 0)
	eps := 1e-9

	capacity := 0
	for i, entry := range timeline {
		if i != 0 && entry.Concurrency <= timeline[i-1].Concurrency {
			// looking into the future, whether we would need the same or greater capacity over keepalive
			// if not, we can add scale down event in future
			futureCapacity := maxOverRange(timeline, entry.Timestamp+eps, entry.Timestamp+float64(keepalive))
			if futureCapacity <= entry.Concurrency {
				instanceTimeline = append(instanceTimeline, TimelineEntry{entry.Timestamp + float64(keepalive), entry.Concurrency})
			}
		} else {
			capacity = maxOverRange(timeline, entry.Timestamp-float64(keepalive), entry.Timestamp)
			if capacity < entry.Concurrency {
				instanceTimeline = append(instanceTimeline, entry)
			}
		}
	}

	// slices.SortFunc(instanceTimeline, func(i, j TimelineEntry) int {
	// 	if i.Timestamp < j.Timestamp {
	// 		return -1
	// 	} else if i.Timestamp > j.Timestamp {
	// 		return 1
	// 	} else {
	// 		return i.Concurrency - j.Concurrency
	// 	}
	// })

	return instanceTimeline
}
