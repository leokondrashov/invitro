package main

import (
	"math"
	"testing"
)

func TestMaxOverRange(t *testing.T) {
	eps := 1e-9
	tests := []struct {
		name     string
		timeline []TimelineEntry
		start    float64
		end      float64
		expected int
	}{
		{
			name:     "no entries",
			timeline: []TimelineEntry{},
			start:    0,
			end:      1,
			expected: 0,
		},
		{
			name: "single entry",
			timeline: []TimelineEntry{
				{Timestamp: 0.5, Concurrency: 1},
			},
			start:    0,
			end:      1,
			expected: 1,
		},
		{
			name: "single entry before range",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
			},
			start:    1,
			end:      2,
			expected: 1,
		},
		{
			name: "single entry after range",
			timeline: []TimelineEntry{
				{Timestamp: 2, Concurrency: 1},
			},
			start:    0,
			end:      1,
			expected: 0,
		},
		{
			name: "two entries before range",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 0.5, Concurrency: 0},
			},
			start:    1,
			end:      2,
			expected: 0,
		},
		{
			name: "entry on start",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 0},
			},
			start:    1,
			end:      2,
			expected: 1,
		},
		{
			name: "entry right before start",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 0},
			},
			start:    1 + eps,
			end:      2,
			expected: 0,
		},
		{
			name: "entry on end",
			timeline: []TimelineEntry{
				{Timestamp: 1, Concurrency: 1},
				{Timestamp: 2, Concurrency: 0},
			},
			start:    0,
			end:      1,
			expected: 0,
		},
		{
			name: "two entries inside range",
			timeline: []TimelineEntry{
				{Timestamp: 0.1, Concurrency: 1},
				{Timestamp: 0.9, Concurrency: 0},
			},
			start:    0,
			end:      1,
			expected: 1,
		},
		{
			name: "range inside two entries",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 0},
			},
			start:    0.1,
			end:      0.9,
			expected: 1,
		},
		{
			name: "range between two invocations",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 0},
				{Timestamp: 2, Concurrency: 1},
				{Timestamp: 3, Concurrency: 0},
			},
			start:    1.1,
			end:      1.9,
			expected: 0,
		},
		{
			name: "double step",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 2},
				{Timestamp: 2, Concurrency: 1},
				{Timestamp: 3, Concurrency: 0},
			},
			start:    0,
			end:      4,
			expected: 2,
		},
		{
			name: "double step after",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 2},
				{Timestamp: 2, Concurrency: 1},
				{Timestamp: 3, Concurrency: 0},
			},
			start:    2 + eps,
			end:      4,
			expected: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := maxOverRange(test.timeline, test.start, test.end)
			if result != test.expected {
				t.Errorf("expected %d, got %d", test.expected, result)
			}
		})
	}
}

func TestGenerateInstanceTimeline(t *testing.T) {
	eps := 1e-9
	tests := []struct {
		name      string
		timeline  []TimelineEntry
		keepalive int
		expected  []TimelineEntry
	}{
		{
			name:      "no entries",
			timeline:  []TimelineEntry{},
			keepalive: 1,
			expected:  []TimelineEntry{},
		},
		{
			name: "single scale up",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 0},
			},
			keepalive: 1,
			expected: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 2, Concurrency: 0},
			},
		},
		{
			name: "scale up after keepalive",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1 - eps, Concurrency: 0},
				{Timestamp: 2, Concurrency: 1},
			},
			keepalive: 1,
			expected: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 2 - eps, Concurrency: 0},
				{Timestamp: 2, Concurrency: 1},
			},
		},
		{
			name: "no scale up during keepalive",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 0},
				{Timestamp: 2, Concurrency: 1},
				{Timestamp: 3, Concurrency: 0},
			},
			keepalive: 2,
			expected: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 5, Concurrency: 0},
			},
		},
		{
			name: "scale up during keepalive",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 2},
				{Timestamp: 2, Concurrency: 1},
				{Timestamp: 2, Concurrency: 0},
			},
			keepalive: 2,
			expected: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 2},
				{Timestamp: 4, Concurrency: 1},
				{Timestamp: 4, Concurrency: 0},
			},
		},
		{
			name: "scale up staircase",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 2},
				{Timestamp: 2, Concurrency: 3},
				{Timestamp: 3, Concurrency: 4},
				{Timestamp: 4, Concurrency: 5},
				{Timestamp: 5, Concurrency: 4},
				{Timestamp: 6, Concurrency: 3},
				{Timestamp: 7, Concurrency: 2},
				{Timestamp: 8, Concurrency: 1},
				{Timestamp: 9, Concurrency: 0},
			},
			keepalive: 2,
			expected: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 2},
				{Timestamp: 2, Concurrency: 3},
				{Timestamp: 3, Concurrency: 4},
				{Timestamp: 4, Concurrency: 5},
				{Timestamp: 7, Concurrency: 4},
				{Timestamp: 8, Concurrency: 3},
				{Timestamp: 9, Concurrency: 2},
				{Timestamp: 10, Concurrency: 1},
				{Timestamp: 11, Concurrency: 0},
			},
		},
		{
			name: "scale up big step",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 4, Concurrency: 2},
				{Timestamp: 4, Concurrency: 3},
				{Timestamp: 4, Concurrency: 4},
				{Timestamp: 4, Concurrency: 5},
				{Timestamp: 4, Concurrency: 6},
				{Timestamp: 5, Concurrency: 5},
				{Timestamp: 5, Concurrency: 4},
				{Timestamp: 5, Concurrency: 3},
				{Timestamp: 5, Concurrency: 2},
				{Timestamp: 5, Concurrency: 1},
				{Timestamp: 5, Concurrency: 0},
			},
			keepalive: 2,
			expected: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 4, Concurrency: 2},
				{Timestamp: 4, Concurrency: 3},
				{Timestamp: 4, Concurrency: 4},
				{Timestamp: 4, Concurrency: 5},
				{Timestamp: 4, Concurrency: 6},
				{Timestamp: 7, Concurrency: 5},
				{Timestamp: 7, Concurrency: 4},
				{Timestamp: 7, Concurrency: 3},
				{Timestamp: 7, Concurrency: 2},
				{Timestamp: 7, Concurrency: 1},
				{Timestamp: 7, Concurrency: 0},
			},
		},
		{
			name: "partial scale down",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 2},
				{Timestamp: 2, Concurrency: 1},
				{Timestamp: 2, Concurrency: 0},
				{Timestamp: 3, Concurrency: 1},
				{Timestamp: 4, Concurrency: 0},
			},
			keepalive: 4,
			expected: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 2},
				{Timestamp: 6, Concurrency: 1},
				{Timestamp: 8, Concurrency: 0},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := generateInstanceTimeline(test.timeline, test.keepalive)
			if len(result) != len(test.expected) {
				t.Errorf("expected %d entries, got %d: %v vs %v", len(test.expected), len(result), test.expected, result)
			} else {
				for i, entry := range result {
					if math.Abs(entry.Timestamp-test.expected[i].Timestamp) > eps || entry.Concurrency != test.expected[i].Concurrency {
						t.Errorf("expected %v, got %v", test.expected[i], entry)
					}
				}
			}
		})
	}
}
