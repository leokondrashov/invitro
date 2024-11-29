package main

import (
	"testing"
)

func TestGetColdStarts(t *testing.T) {
	tests := []struct {
		name      string
		timeline  []int
		keepalive int
		expected  []int
	}{
		{
			name:      "no cold starts",
			timeline:  []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			keepalive: 1,
			expected:  []int{},
		},
		{
			name:      "initial cold start",
			timeline:  []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			keepalive: 1,
			expected:  []int{0},
		},
		{
			name:      "cold starts",
			timeline:  []int{1, 2, 2, 2, 1, 1, 1, 1, 1, 1},
			keepalive: 3,
			expected:  []int{0, 1},
		},
		{
			name:      "no cold starts during keepalive",
			timeline:  []int{1, 2, 1, 1, 1, 1, 2, 1, 1, 1},
			keepalive: 5,
			expected:  []int{0, 1},
		},
		{
			name:      "cold start after keepalive",
			timeline:  []int{1, 2, 1, 1, 1, 1, 1, 2, 1, 1}, // scaled down after 6th point
			keepalive: 5,
			expected:  []int{0, 1, 7},
		},
		{
			name:      "cold start during keepalive",
			timeline:  []int{1, 2, 1, 2, 3, 1, 1, 1, 1, 1},
			keepalive: 5,
			expected:  []int{0, 1, 4},
		},
		{
			name:      "cold start staircase",
			timeline:  []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			keepalive: 5,
			expected:  []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		},
		{
			name:      "cold start big step",
			timeline:  []int{1, 1, 1, 1, 1, 6, 6, 6, 6, 6},
			keepalive: 5,
			expected:  []int{0, 5, 5, 5, 5, 5},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := make(chan int, 100)
			go func() {
				defer close(writer)
				getColdStarts(test.timeline, test.keepalive, writer)
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
