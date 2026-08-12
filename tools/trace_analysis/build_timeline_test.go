package main

import (
	"math"
	"testing"
	"time"

	"github.com/vhive-serverless/loader/pkg/common"
)

func functionWithInvocations(iat common.IATArray, runtime []common.RuntimeSpecification) *common.Function {
	return &common.Function{Specification: &common.FunctionSpecification{
		IAT:                  iat,
		RuntimeSpecification: runtime,
	}}
}

func TestGenerateTimelineUsesFlattenedIAT(t *testing.T) {
	tests := []struct {
		name        string
		granularity time.Duration
		iat         common.IATArray
		runtime     []common.RuntimeSpecification
		assert      func(*testing.T, []int)
	}{
		{"single invocation", time.Millisecond, []float64{0}, []common.RuntimeSpecification{{Runtime: 1}}, func(t *testing.T, got []int) {
			if got[0] != 1 || got[1] != 0 {
				t.Fatal("unexpected timeline")
			}
		}},
		{"single invocation, 0.1ms", time.Millisecond / 10, []float64{0}, []common.RuntimeSpecification{{Runtime: 1}}, func(t *testing.T, got []int) {
			if got[0] != 1 || got[9] != 1 || got[10] != 0 {
				t.Fatal("unexpected timeline")
			}
		}},
		{"long invocation", time.Millisecond, []float64{0}, []common.RuntimeSpecification{{Runtime: 1000}}, func(t *testing.T, got []int) {
			if got[0] != 1 || got[999] != 1 || got[1000] != 0 {
				t.Fatal("unexpected timeline")
			}
		}},
		{"long invocation, 0.1ms", time.Millisecond / 10, []float64{0}, []common.RuntimeSpecification{{Runtime: 1000}}, func(t *testing.T, got []int) {
			if got[0] != 1 || got[9999] != 1 || got[10000] != 0 {
				t.Fatal("unexpected timeline")
			}
		}},
		{"two invocations", time.Millisecond, []float64{0, 10_000}, []common.RuntimeSpecification{{Runtime: 1}, {Runtime: 1}}, func(t *testing.T, got []int) {
			if got[0] != 1 || got[10] != 1 || got[11] != 0 {
				t.Fatal("unexpected timeline")
			}
		}},
		{"two invocations, 0.1ms", time.Millisecond / 10, []float64{0, 10_000}, []common.RuntimeSpecification{{Runtime: 1}, {Runtime: 1}}, func(t *testing.T, got []int) {
			if got[0] != 1 || got[100] != 1 || got[110] != 0 {
				t.Fatal("unexpected timeline")
			}
		}},
		{"overlapping invocations", time.Millisecond, []float64{0, 10_000}, []common.RuntimeSpecification{{Runtime: 100}, {Runtime: 100}}, func(t *testing.T, got []int) {
			if got[0] != 1 || got[10] != 2 || got[100] != 1 || got[110] != 0 {
				t.Fatal("unexpected timeline")
			}
		}},
		{"overlapping invocations, 0.1ms", time.Millisecond / 10, []float64{0, 10_000}, []common.RuntimeSpecification{{Runtime: 100}, {Runtime: 100}}, func(t *testing.T, got []int) {
			if got[0] != 1 || got[100] != 2 || got[1000] != 1 || got[1100] != 0 {
				t.Fatal("unexpected timeline")
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assert(t, generateFunctionTimeline(functionWithInvocations(test.iat, test.runtime), 1, test.granularity))
		})
	}
}

func TestFlattenedIATKeepsInitialEmptyMinutes(t *testing.T) {
	function := functionWithInvocations(
		common.IATArray{122_000_000},
		[]common.RuntimeSpecification{{Runtime: 1, Memory: 1}},
	)

	compressed := generateFunctionTimelineCompressed(function, 3, 1)
	if len(compressed) != 2 || compressed[0].Timestamp != 122 {
		t.Fatalf("first invocation timestamp = %v, want 122", compressed)
	}

	timeline := generateFunctionTimeline(function, 3, time.Millisecond)
	if timeline[122_000] != 1 {
		t.Fatalf("expected invocation at 122 seconds, got %d", timeline[122_000])
	}
}

func TestGenerateTimelineCompressed(t *testing.T) {
	tests := []struct {
		name     string
		slowdown float64
		iat      common.IATArray
		runtime  []common.RuntimeSpecification
		want     []TimelineEntry
	}{
		{"single invocation", 1, []float64{0}, []common.RuntimeSpecification{{Runtime: 1}}, []TimelineEntry{{0, 1}, {0.001, 0}}},
		{"single invocation, slowed", 1.5, []float64{0}, []common.RuntimeSpecification{{Runtime: 1}}, []TimelineEntry{{0, 1}, {0.0015, 0}}},
		{"long invocation", 1, []float64{0}, []common.RuntimeSpecification{{Runtime: 1000}}, []TimelineEntry{{0, 1}, {1, 0}}},
		{"two invocations", 1, []float64{0, 10_000}, []common.RuntimeSpecification{{Runtime: 1}, {Runtime: 1}}, []TimelineEntry{{0, 1}, {0.001, 0}, {0.01, 1}, {0.011, 0}}},
		{"overlapping invocations", 1, []float64{0, 10_000}, []common.RuntimeSpecification{{Runtime: 100}, {Runtime: 100}}, []TimelineEntry{{0, 1}, {0.01, 2}, {0.1, 1}, {0.11, 0}}},
		{"simultaneous invocations", 1, []float64{0, 0}, []common.RuntimeSpecification{{Runtime: 100}, {Runtime: 100}}, []TimelineEntry{{0, 1}, {0, 2}, {0.1, 1}, {0.1, 0}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := generateFunctionTimelineCompressed(functionWithInvocations(test.iat, test.runtime), 1, test.slowdown)
			if len(got) != len(test.want) {
				t.Fatalf("timeline length = %d, want %d", len(got), len(test.want))
			}
			for i := range test.want {
				if got[i].Concurrency != test.want[i].Concurrency || math.Abs(got[i].Timestamp-test.want[i].Timestamp) > 1e-9 {
					t.Errorf("entry %d = %+v, want %+v", i, got[i], test.want[i])
				}
			}
		})
	}
}

func TestAverageTimeline(t *testing.T) {
	tests := []struct {
		name     string
		timeline []TimelineEntry
		want     []AvgTimelineEntry
	}{
		{"empty timeline", []TimelineEntry{}, []AvgTimelineEntry{}},
		{"long single invocation", []TimelineEntry{{0, 1}, {1, 0}}, []AvgTimelineEntry{{0, 1}, {1, 0}}},
		{"short single invocation", []TimelineEntry{{0, 1}, {0.001, 0}}, []AvgTimelineEntry{{0, 0.001}}},
		{"late short invocation", []TimelineEntry{{10, 1}, {10.001, 0}}, []AvgTimelineEntry{{10, 0.001}}},
		{"spill", []TimelineEntry{{0, 1}, {1.5, 0}}, []AvgTimelineEntry{{0, 1}, {1, 0.5}}},
		{"overlapping invocations", []TimelineEntry{{0, 1}, {0.5, 2}, {1, 1}, {1.5, 0}}, []AvgTimelineEntry{{0, 1.5}, {1, 0.5}}},
		{"non-overlapping invocations", []TimelineEntry{{0, 1}, {0.1, 0}, {0.5, 1}, {0.6, 0}}, []AvgTimelineEntry{{0, 0.2}}},
		{"simultaneous invocations", []TimelineEntry{{0, 1}, {0, 2}, {1.5, 1}, {1.5, 0}}, []AvgTimelineEntry{{0, 2}, {1, 1}}},
		{"empty granularity", []TimelineEntry{{0.1, 1}, {0.9, 0}, {2.1, 1}, {2.9, 0}}, []AvgTimelineEntry{{0, 0.8}, {1, 0}, {2, 0.8}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertAverageTimeline(t, averageTimeline(test.timeline, time.Second), test.want)
		})
	}
}

func TestAverageTimelineGranularity(t *testing.T) {
	tests := []struct {
		name        string
		timeline    []TimelineEntry
		granularity time.Duration
		want        []AvgTimelineEntry
	}{
		{"empty timeline", []TimelineEntry{}, time.Second, []AvgTimelineEntry{}},
		{"one-second invocation", []TimelineEntry{{0, 1}, {1, 0}}, time.Second, []AvgTimelineEntry{{0, 1}, {1, 0}}},
		{"tenths of a second", []TimelineEntry{{0, 1}, {1, 0}}, time.Second / 10, []AvgTimelineEntry{{0, 1}, {0.1, 1}, {0.2, 1}, {0.3, 1}, {0.4, 1}, {0.5, 1}, {0.6, 1}, {0.7, 1}, {0.8, 1}, {0.9, 1}, {1, 0}}},
		{"ten-second window", []TimelineEntry{{0, 1}, {1, 0}}, 10 * time.Second, []AvgTimelineEntry{{0, 0.1}}},
		{"off-window invocation", []TimelineEntry{{2, 1}, {3, 0}}, 10 * time.Second, []AvgTimelineEntry{{0, 0.1}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertAverageTimeline(t, averageTimeline(test.timeline, test.granularity), test.want)
		})
	}
}

func assertAverageTimeline(t *testing.T, got, want []AvgTimelineEntry) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("average timeline length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if math.Abs(got[i].Timestamp-want[i].Timestamp) > 1e-9 || math.Abs(got[i].Concurrency-want[i].Concurrency) > 1e-9 {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
