package main

import (
	"math"
	"slices"
	"time"

	"github.com/vhive-serverless/loader/pkg/common"
)

type TimelineEntry struct {
	Timestamp   float64
	Concurrency int
}

type AvgTimelineEntry struct {
	Timestamp   float64 `csv:"timestamp"`
	Concurrency float64 `csv:"concurrency"`
}

func generateFunctionTimeline(function *common.Function, duration int, granularity time.Duration) []int {
	IAT, runtimeSpecification := function.Specification.IAT, function.Specification.RuntimeSpecification

	maxTime := duration*60*int(time.Second/granularity) + common.MaxExecTimeMilli*int(time.Millisecond/granularity)
	concurrency := make([]int, maxTime)

	iatIndex := 0
	for minuteIndex, invocationCount := range perMinuteCounts(function) {
		if minuteIndex >= duration || iatIndex >= len(IAT) || iatIndex >= len(runtimeSpecification) {
			break
		}

		sum := 0.0
		for range invocationCount {
			if iatIndex >= len(IAT) || iatIndex >= len(runtimeSpecification) {
				break
			}
			sum += IAT[iatIndex] / 1e6
			runtime := runtimeSpecification[iatIndex].Runtime * int(time.Millisecond/granularity)
			startTime := minuteIndex*int(time.Minute/granularity) + int(sum*float64(time.Second/granularity))
			for i := startTime; i < startTime+runtime && i < len(concurrency); i++ {
				if i >= 0 {
					concurrency[i]++
				}
			}
			iatIndex++
		}
	}

	return concurrency
}

func perMinuteCounts(function *common.Function) []int {
	if len(function.Specification.PerMinuteCount) != 0 {
		return function.Specification.PerMinuteCount
	}
	return []int{len(function.Specification.IAT)}
}

func generateFunctionTimelineCompressed(function *common.Function, duration int, slowdown float64) []TimelineEntry {
	IAT, runtimeSpecification := function.Specification.IAT, function.Specification.RuntimeSpecification
	timeline := make([]TimelineEntry, 0, 2*len(IAT))

	iatIndex := 0
	for minuteIndex, invocationCount := range perMinuteCounts(function) {
		if minuteIndex >= duration || iatIndex >= len(IAT) || iatIndex >= len(runtimeSpecification) {
			break
		}

		sum := 0.0
		for range invocationCount {
			if iatIndex >= len(IAT) || iatIndex >= len(runtimeSpecification) {
				break
			}
			sum += IAT[iatIndex] / float64(time.Second/time.Microsecond)
			runtime := float64(runtimeSpecification[iatIndex].Runtime) / float64(time.Second/time.Millisecond) * slowdown
			startTime := float64(minuteIndex*int(time.Minute/time.Second)) + sum
			timeline = append(timeline, TimelineEntry{Timestamp: startTime, Concurrency: 1})
			timeline = append(timeline, TimelineEntry{Timestamp: startTime + runtime, Concurrency: -1})
			iatIndex++
		}
	}

	slices.SortFunc(timeline, func(i, j TimelineEntry) int {
		if i.Timestamp < j.Timestamp {
			return -1
		} else if i.Timestamp > j.Timestamp {
			return 1
		} else {
			return i.Concurrency - j.Concurrency
		}
	})

	concurrency := 0
	for i := 0; i < len(timeline); i++ {
		concurrency += timeline[i].Concurrency
		timeline[i].Concurrency = concurrency
	}

	return timeline
}

// Timeline should be already sorted by timestamp and compressed
func averageTimeline(timeline []TimelineEntry, granularity time.Duration) []AvgTimelineEntry {
	if len(timeline) == 0 {
		return []AvgTimelineEntry{}
	}
	minTime := timeline[0].Timestamp
	maxTime := timeline[len(timeline)-1].Timestamp
	// assert.Greater(maxTime, 0, "maxTime should be greater than 0")
	// assert.Equal(timeline[len(timeline)-1].Concurrency, 0, "last concurrency should be equal 0")
	avg := 0.0
	currentTime := math.Floor(minTime/granularity.Seconds()) * granularity.Seconds()
	intervalEnd := currentTime + granularity.Seconds()
	prevTimestamp := currentTime
	i := 0
	concurrency := 0

	avgTimeline := make([]AvgTimelineEntry, 0, int(maxTime/granularity.Seconds())+1)

	for currentTime <= maxTime {
		for i < len(timeline) && timeline[i].Timestamp <= intervalEnd {
			avg += float64(concurrency) * (timeline[i].Timestamp - prevTimestamp)
			concurrency = timeline[i].Concurrency
			prevTimestamp = timeline[i].Timestamp
			i++
		}
		avg += float64(concurrency) * (intervalEnd - prevTimestamp) // last interval in this granularity
		prevTimestamp = intervalEnd

		avgTimeline = append(avgTimeline, AvgTimelineEntry{
			currentTime,
			avg / granularity.Seconds(),
		})
		currentTime = intervalEnd
		intervalEnd += granularity.Seconds()
		avg = 0.0
	}

	return avgTimeline
}
