package main

import (
	"fmt"
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

func TestGetColdStarts(t *testing.T) {
	tests := []struct {
		name      string
		timeline  []int
		keepalive int
		expected  []coldStartRecord
	}{
		{
			name:      "no cold starts",
			timeline:  []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			keepalive: 1,
			expected:  []coldStartRecord{},
		},
		{
			name:      "initial cold start",
			timeline:  []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			keepalive: 1,
			expected: []coldStartRecord{
				{Timestamp: 0, FunctionNum: 0},
			},
		},
		{
			name:      "cold starts",
			timeline:  []int{1, 2, 2, 2, 1, 1, 1, 1, 1, 1},
			keepalive: 3,
			expected: []coldStartRecord{
				{Timestamp: 0, FunctionNum: 0},
				{Timestamp: 1, FunctionNum: 0},
			},
		},
		{
			name:      "no cold starts during keepalive",
			timeline:  []int{1, 2, 1, 1, 1, 1, 2, 1, 1, 1},
			keepalive: 5,
			expected: []coldStartRecord{
				{Timestamp: 0, FunctionNum: 0},
				{Timestamp: 1, FunctionNum: 0},
			},
		},
		{
			name:      "cold start after keepalive",
			timeline:  []int{1, 2, 1, 1, 1, 1, 1, 2, 1, 1}, // scaled down after 6th point
			keepalive: 5,
			expected: []coldStartRecord{
				{Timestamp: 0, FunctionNum: 0},
				{Timestamp: 1, FunctionNum: 0},
				{Timestamp: 7, FunctionNum: 0},
			},
		},
		{
			name:      "cold start during keepalive",
			timeline:  []int{1, 2, 1, 2, 3, 1, 1, 1, 1, 1},
			keepalive: 5,
			expected: []coldStartRecord{
				{Timestamp: 0, FunctionNum: 0},
				{Timestamp: 1, FunctionNum: 0},
				{Timestamp: 4, FunctionNum: 0},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := make(chan interface{}, 100)
			go func() {
				defer close(writer)
				getColdStarts(test.timeline, test.keepalive, 0, writer)
			}()

			for _, expected := range test.expected {
				record, ok := <-writer
				if !ok {
					t.Errorf("Expected %v, got nothing", expected)
				} else if record != expected {
					t.Errorf("Expected %v, got %v", expected, record)
				}
			}
			if record, ok := <-writer; ok {
				t.Errorf("Expected nothing, got %v", record)
			}
		})
	}
}
