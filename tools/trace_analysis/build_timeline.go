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
	minuteIndex, invocationIndex := 0, 0
	sum := 0.0

	IAT, runtimeSpecification := function.Specification.IAT, function.Specification.RuntimeSpecification

	maxTime := duration*60*int(time.Second/granularity) + common.MaxExecTimeMilli*int(time.Millisecond/granularity)
	concurrency := make([]int, maxTime)

	for {
		if minuteIndex >= duration {
			break
		} else if function.InvocationStats.Invocations[minuteIndex] == 0 {
			minuteIndex++
			invocationIndex = 0
			sum = 0.0
			continue
		}

		sum += IAT[minuteIndex][invocationIndex] / 1e6

		duration := runtimeSpecification[minuteIndex][invocationIndex].Runtime * int(time.Millisecond/granularity)
		// fmt.Println(sum)
		startTime := minuteIndex*int(time.Minute/granularity) + int(sum*float64(time.Second/granularity))
		// log.Infof("Function %s, order %d, minute %d, invocation %d, start time %d, duration %d", function.Name, orderNum, minuteIndex, invocationIndex, startTime, duration)
		for i := startTime; i < startTime+duration; i++ {
			concurrency[i]++
		}
		// log.Infof("%v", concurrency[startTime:startTime+duration])

		invocationIndex++
		if function.InvocationStats.Invocations[minuteIndex] == invocationIndex {
			minuteIndex++
			invocationIndex = 0
			sum = 0.0
		}
	}

	return concurrency
}

func totalInvocations(IAT common.IATMatrix) int {
	total := 0
	for _, minute := range IAT {
		total += len(minute)
	}
	return total
}

func generateFunctionTimelineCompressed(function *common.Function, duration int, slowdown float64) []TimelineEntry {
	minuteIndex, invocationIndex := 0, 0
	sum := 0.0

	IAT, runtimeSpecification := function.Specification.IAT, function.Specification.RuntimeSpecification
	timeline := make([]TimelineEntry, 0, 2*totalInvocations(IAT))

	for {
		if minuteIndex >= duration {
			break
		} else if function.InvocationStats.Invocations[minuteIndex] == 0 {
			minuteIndex++
			invocationIndex = 0
			sum = 0.0
			continue
		}

		sum += IAT[minuteIndex][invocationIndex] / float64(time.Second/time.Microsecond)

		duration := float64(runtimeSpecification[minuteIndex][invocationIndex].Runtime) / float64(time.Second/time.Millisecond) * slowdown
		startTime := float64(minuteIndex*int(time.Minute/time.Second)) + sum
		timeline = append(timeline, TimelineEntry{
			Timestamp:   startTime,
			Concurrency: 1,
		})
		timeline = append(timeline, TimelineEntry{
			Timestamp:   startTime + duration,
			Concurrency: -1,
		})

		invocationIndex++
		if function.InvocationStats.Invocations[minuteIndex] == invocationIndex {
			minuteIndex++
			invocationIndex = 0
			sum = 0.0
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
