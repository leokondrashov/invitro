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
