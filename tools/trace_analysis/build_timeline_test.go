package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/vhive-serverless/loader/pkg/common"
)

func injectInvocationData(function *common.Function, IAT common.IATMatrix, runtimeSpec common.RuntimeSpecificationMatrix) {
	function.Specification = &common.FunctionSpecification{}
	function.Specification.IAT = IAT
	function.Specification.RuntimeSpecification = runtimeSpec

	function.InvocationStats = &common.FunctionInvocationStats{}
	function.InvocationStats.Invocations = make([]int, len(IAT))
	for i := range function.InvocationStats.Invocations {
		if len(IAT[i]) == 0 {
			continue
		}
		function.InvocationStats.Invocations[i] = len(IAT[i]) - 1
	}
}

func TestGenerateTimeline(t *testing.T) {
	tests := []struct {
		name        string
		iat         common.IATMatrix
		runtimeSpec common.RuntimeSpecificationMatrix
		testFunc    func([]int) bool
		granularity time.Duration
	}{
		{
			name: "single inv",
			iat: common.IATMatrix{
				[]float64{0, 60000000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 1,
						Memory:  1,
					},
				},
			},
			testFunc: func(timeline []int) bool {
				return (len(timeline) == 1*60_000+60_000) && timeline[0] == 1 && timeline[1] == 0 && timeline[60] == 0 && timeline[60_000] == 0
			},
			granularity: time.Millisecond,
		},
		{
			name: "single inv, 0.1ms granularity",
			iat: common.IATMatrix{
				[]float64{0, 60000000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 1,
						Memory:  1,
					},
				},
			},
			testFunc: func(timeline []int) bool {
				return (len(timeline) == 1*600_000+600_000) && timeline[0] == 1 && timeline[9] == 1 && timeline[10] == 0
			},
			granularity: time.Millisecond / 10,
		},
		{
			name: "single long inv",
			iat: common.IATMatrix{
				[]float64{0, 60000000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 1000,
						Memory:  1,
					},
				},
			},
			testFunc: func(timeline []int) bool {
				return timeline[0] == 1 && timeline[1] == 1 && timeline[999] == 1 && timeline[1000] == 0
			},
			granularity: time.Millisecond,
		},
		{
			name: "single long inv, 0.1ms granularity",
			iat: common.IATMatrix{
				[]float64{0, 60000000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 1000,
						Memory:  1,
					},
				},
			},
			testFunc: func(timeline []int) bool {
				return timeline[0] == 1 && timeline[1] == 1 && timeline[9999] == 1 && timeline[10000] == 0
			},
			granularity: time.Millisecond / 10,
		},
		{
			name: "two inv",
			iat: common.IATMatrix{
				[]float64{0, 10000, 60000000 - 10000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 1,
						Memory:  1,
					},
					{
						Runtime: 1,
						Memory:  1,
					},
				},
			},
			testFunc: func(timeline []int) bool {
				return timeline[0] == 1 && timeline[1] == 0 && timeline[9] == 0 && timeline[10] == 1 && timeline[11] == 0
			},
			granularity: time.Millisecond,
		},
		{
			name: "two inv, 0.1ms granularity",
			iat: common.IATMatrix{
				[]float64{0, 10000, 60000000 - 10000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 1,
						Memory:  1,
					},
					{
						Runtime: 1,
						Memory:  1,
					},
				},
			},
			testFunc: func(timeline []int) bool {
				return timeline[0] == 1 && timeline[10] == 0 && timeline[99] == 0 && timeline[100] == 1 && timeline[110] == 0
			},
			granularity: time.Millisecond / 10,
		},
		{
			name: "two overlapping inv",
			iat: common.IATMatrix{
				[]float64{0, 10000, 60000000 - 10000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 100,
						Memory:  1,
					},
					{
						Runtime: 100,
						Memory:  1,
					},
				},
			},
			testFunc: func(timeline []int) bool {
				return timeline[0] == 1 && timeline[9] == 1 && timeline[10] == 2 && timeline[99] == 2 && timeline[100] == 1 && timeline[110] == 0
			},
			granularity: time.Millisecond,
		},
		{
			name: "two overlapping inv, 0.1ms granularity",
			iat: common.IATMatrix{
				[]float64{0, 10000, 60000000 - 10000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 100,
						Memory:  1,
					},
					{
						Runtime: 100,
						Memory:  1,
					},
				},
			},
			testFunc: func(timeline []int) bool {
				return timeline[0] == 1 && timeline[99] == 1 && timeline[100] == 2 && timeline[999] == 2 && timeline[1000] == 1 && timeline[1100] == 0
			},
			granularity: time.Millisecond / 10,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			function := &common.Function{}
			injectInvocationData(function, test.iat, test.runtimeSpec)

			timeline := generateFunctionTimeline(function, 1, test.granularity)
			if !test.testFunc(timeline) {
				t.Errorf("Test failed")
				fmt.Printf("timeline: %v\n", timeline[:100])
			}
		})
	}
}

func TestGenerateTimelineCompressed(t *testing.T) {
	eps := 1e-9
	tests := []struct {
		name        string
		slowdown    float64
		iat         common.IATMatrix
		runtimeSpec common.RuntimeSpecificationMatrix
		expected    []TimelineEntry
	}{
		{
			name:     "single inv",
			slowdown: 1,
			iat: common.IATMatrix{
				[]float64{0, 60000000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 1,
						Memory:  1,
					},
				},
			},
			expected: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1e-3,
					Concurrency: 0,
				},
			},
		},
		{
			name:     "single inv, slowed down",
			slowdown: 1.5,
			iat: common.IATMatrix{
				[]float64{0, 60000000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 1,
						Memory:  1,
					},
				},
			},
			expected: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1.5e-3,
					Concurrency: 0,
				},
			},
		},
		{
			name:     "single long inv",
			slowdown: 1,
			iat: common.IATMatrix{
				[]float64{0, 60000000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 1000,
						Memory:  1,
					},
				},
			},
			expected: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1,
					Concurrency: 0,
				},
			},
		},
		{
			name:     "two inv",
			slowdown: 1,
			iat: common.IATMatrix{
				[]float64{0, 10000, 60000000 - 10000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 1,
						Memory:  1,
					},
					{
						Runtime: 1,
						Memory:  1,
					},
				},
			},
			expected: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1e-3,
					Concurrency: 0,
				},
				{
					Timestamp:   10e-3,
					Concurrency: 1,
				},
				{
					Timestamp:   11e-3,
					Concurrency: 0,
				},
			},
		},
		{
			name:     "two overlapping inv",
			slowdown: 1,
			iat: common.IATMatrix{
				[]float64{0, 10000, 60000000 - 10000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 100,
						Memory:  1,
					},
					{
						Runtime: 100,
						Memory:  1,
					},
				},
			},
			expected: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   10e-3,
					Concurrency: 2,
				},
				{
					Timestamp:   100e-3,
					Concurrency: 1,
				},
				{
					Timestamp:   110e-3,
					Concurrency: 0,
				},
			},
		},
		{
			name:     "two simultaneous inv",
			slowdown: 1,
			iat: common.IATMatrix{
				[]float64{0, 0, 60000000 - 10000},
			},
			runtimeSpec: common.RuntimeSpecificationMatrix{
				[]common.RuntimeSpecification{
					{
						Runtime: 100,
						Memory:  1,
					},
					{
						Runtime: 100,
						Memory:  1,
					},
				},
			},
			expected: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   0,
					Concurrency: 2,
				},
				{
					Timestamp:   100e-3,
					Concurrency: 1,
				},
				{
					Timestamp:   100e-3,
					Concurrency: 0,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			function := &common.Function{}
			injectInvocationData(function, test.iat, test.runtimeSpec)

			timeline := generateFunctionTimelineCompressed(function, 1, test.slowdown)
			if len(timeline) != len(test.expected) {
				t.Errorf("Wrong timeline length: %v, expected %v", timeline, test.expected)
			}
			for i, entry := range timeline {
				if math.Abs(entry.Timestamp-test.expected[i].Timestamp) > eps || entry.Concurrency != test.expected[i].Concurrency {
					t.Errorf("Wrong entry at %v: %v, expected %v", i, entry, test.expected[i])
				}
			}
		})
	}
}

func TestAverageTimeline(t *testing.T) {
	eps := 1e-9

	tests := []struct {
		name     string
		timeline []TimelineEntry
		expected []AvgTimelineEntry
	}{
		{
			name:     "empty timeline",
			timeline: []TimelineEntry{},
			expected: []AvgTimelineEntry{},
		},
		{
			name: "long single inv",
			timeline: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1,
					Concurrency: 0,
				},
			},
		},
		{
			name: "short single inv",
			timeline: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1e-3,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1e-3,
				},
			},
		},
		{
			name: "late short single inv",
			timeline: []TimelineEntry{
				{
					Timestamp:   10,
					Concurrency: 1,
				},
				{
					Timestamp:   10 + 1e-3,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   10,
					Concurrency: 1e-3,
				},
			},
		},
		{
			name: "spill",
			timeline: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1.5,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1,
					Concurrency: 0.5,
				},
			},
		},
		{
			name: "two overlapping inv",
			timeline: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   0.5,
					Concurrency: 2,
				},
				{
					Timestamp:   1,
					Concurrency: 1,
				},
				{
					Timestamp:   1.5,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1.5,
				},
				{
					Timestamp:   1,
					Concurrency: 0.5,
				},
			},
		},
		{
			name: "two non-overlapping inv",
			timeline: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   0.1,
					Concurrency: 0,
				},
				{
					Timestamp:   0.5,
					Concurrency: 1,
				},
				{
					Timestamp:   0.6,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 0.2,
				},
			},
		},
		{
			name: "two simultaneous inv",
			timeline: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   0,
					Concurrency: 2,
				},
				{
					Timestamp:   1.5,
					Concurrency: 1,
				},
				{
					Timestamp:   1.5,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 2,
				},
				{
					Timestamp:   1,
					Concurrency: 1,
				},
			},
		},
		{
			name: "empty granularity",
			timeline: []TimelineEntry{
				{
					Timestamp:   0.1,
					Concurrency: 1,
				},
				{
					Timestamp:   0.9,
					Concurrency: 0,
				},
				{
					Timestamp:   2.1,
					Concurrency: 1,
				},
				{
					Timestamp:   2.9,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 0.8,
				},
				{
					Timestamp:   1,
					Concurrency: 0,
				},
				{
					Timestamp:   2,
					Concurrency: 0.8,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := averageTimeline(test.timeline, time.Second)

			if len(result) != len(test.expected) {
				t.Errorf("Wrong result length: %v, expected %v", result, test.expected)
			}

			for i, entry := range result {
				if math.Abs(entry.Timestamp-test.expected[i].Timestamp) > eps || math.Abs(entry.Concurrency-test.expected[i].Concurrency) > eps {
					t.Errorf("Wrong entry at %v: %v, expected %v", i, entry, test.expected[i])
				}
			}
		})
	}
}

func TestAverageTimelineGranularity(t *testing.T) {
	eps := 1e-9

	tests := []struct {
		name        string
		timeline    []TimelineEntry
		expected    []AvgTimelineEntry
		granularity time.Duration
	}{
		{
			name:        "empty timeline",
			timeline:    []TimelineEntry{},
			expected:    []AvgTimelineEntry{},
			granularity: time.Second,
		},
		{
			name: "single inv",
			timeline: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1,
					Concurrency: 0,
				},
			},
			granularity: time.Second,
		},
		{
			name: "single inv, 0.1s granularity",
			timeline: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   0.1,
					Concurrency: 1,
				},
				{
					Timestamp:   0.2,
					Concurrency: 1,
				},
				{
					Timestamp:   0.3,
					Concurrency: 1,
				},
				{
					Timestamp:   0.4,
					Concurrency: 1,
				}, {
					Timestamp:   0.5,
					Concurrency: 1,
				},
				{
					Timestamp:   0.6,
					Concurrency: 1,
				},
				{
					Timestamp:   0.7,
					Concurrency: 1,
				},
				{
					Timestamp:   0.8,
					Concurrency: 1,
				},
				{
					Timestamp:   0.9,
					Concurrency: 1,
				},
				{
					Timestamp:   1,
					Concurrency: 0,
				},
			},
			granularity: time.Second / 10,
		},
		{
			name: "single inv, 10s",
			timeline: []TimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 1,
				},
				{
					Timestamp:   1,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 0.1,
				},
			},
			granularity: 10 * time.Second,
		},
		{
			name: "single inv, 10s, off granularity",
			timeline: []TimelineEntry{
				{
					Timestamp:   2,
					Concurrency: 1,
				},
				{
					Timestamp:   3,
					Concurrency: 0,
				},
			},
			expected: []AvgTimelineEntry{
				{
					Timestamp:   0,
					Concurrency: 0.1,
				},
			},
			granularity: 10 * time.Second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := averageTimeline(test.timeline, test.granularity)

			if len(result) != len(test.expected) {
				t.Errorf("Wrong result length: %v, expected %v", result, test.expected)
			}

			for i, entry := range result {
				if math.Abs(entry.Timestamp-test.expected[i].Timestamp) > eps || math.Abs(entry.Concurrency-test.expected[i].Concurrency) > eps {
					t.Errorf("Wrong entry at %v: %v, expected %v", i, entry, test.expected[i])
				}
			}
		})
	}
}
