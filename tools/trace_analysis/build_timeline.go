package main

import (
	"time"

	"github.com/vhive-serverless/loader/pkg/common"
)

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
