package main

import (
	"math"
	"testing"
)

func TestGetColdStarts(t *testing.T) {
	eps := 1e-9
	tests := []struct {
		name     string
		timeline []TimelineEntry
		expected []float64
	}{
		{
			name:     "no cold starts",
			timeline: []TimelineEntry{},
			expected: []float64{},
		},
		{
			name: "initial cold start",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 2, Concurrency: 0},
			},
			expected: []float64{0},
		},
		{
			name: "stairs",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 2},
				{Timestamp: 2, Concurrency: 1},
				{Timestamp: 3, Concurrency: 0},
			},
			expected: []float64{0, 1},
		},
		{
			name: "consecutive cold starts",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 1, Concurrency: 0},
				{Timestamp: 2, Concurrency: 1},
				{Timestamp: 3, Concurrency: 0},
			},
			expected: []float64{0, 2},
		},
		{
			name: "simultaneous cold starts",
			timeline: []TimelineEntry{
				{Timestamp: 0, Concurrency: 1},
				{Timestamp: 0, Concurrency: 2},
				{Timestamp: 2, Concurrency: 1},
				{Timestamp: 2, Concurrency: 0},
			},
			expected: []float64{0, 0},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := make(chan float64, 100)
			go func() {
				defer close(writer)
				getColdStarts(test.timeline, writer)
			}()

			for _, expected := range test.expected {
				record, ok := <-writer
				if !ok {
					t.Errorf("Expected %v, got nothing", expected)
				} else if math.Abs(record-expected) > eps {
					t.Errorf("Expected %v, got %v", expected, record)
				}
			}
			if record, ok := <-writer; ok {
				t.Errorf("Expected nothing, got %v", record)
			}
		})
	}
}
