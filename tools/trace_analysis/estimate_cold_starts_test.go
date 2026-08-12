package main

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCommonInitAcceptsAzure2021InvocationFile(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "output.csv")
	tracePath := filepath.Join("..", "..", "pkg", "trace", "test_data", "Azure2021", "Azure2021_30.csv")

	writer, written, functions := commonInit(outputPath, tracePath, 3, 0, "exponential", 42)
	close(writer)
	written.Wait()

	if len(functions) != 14 {
		t.Fatalf("expected 14 Azure 2021 functions, got %d", len(functions))
	}
	for i := 1; i < len(functions); i++ {
		previous := functionID(functions[i-1].Name, true)
		current := functionID(functions[i].Name, true)
		if previous >= current {
			t.Fatalf("functions were not sorted by stable ID: %q >= %q", previous, current)
		}
	}

	mappingFile, err := os.Open(functionMappingFilename(outputPath))
	if err != nil {
		t.Fatalf("open function mapping: %v", err)
	}
	defer mappingFile.Close()
	mapping, err := csv.NewReader(mappingFile).ReadAll()
	if err != nil {
		t.Fatalf("read function mapping: %v", err)
	}
	if len(mapping) != len(functions)+1 {
		t.Fatalf("expected %d mapping rows, got %d", len(functions)+1, len(mapping))
	}
	if got, want := strings.Join(mapping[0], ","), "functionNum,functionId,functionName"; got != want {
		t.Fatalf("mapping header = %q, want %q", got, want)
	}
	for i, function := range functions {
		row := mapping[i+1]
		if row[0] != strconv.Itoa(i) || row[1] != functionID(function.Name, true) || row[2] != function.Name {
			t.Fatalf("mapping row %d does not match function", i)
		}
	}

	for _, function := range functions {
		if function.Specification == nil || len(function.Specification.IAT) == 0 {
			t.Fatal("Azure 2021 function specification was not preserved")
		}
	}
}

func TestAzure2021FunctionNumbersAreStableAcrossParserRuns(t *testing.T) {
	tracePath := filepath.Join("..", "..", "pkg", "trace", "test_data", "Azure2021", "Azure2021_30.csv")
	temporaryDir := t.TempDir()

	writer1, written1, functions1 := commonInit(filepath.Join(temporaryDir, "first.csv"), tracePath, 3, 0, "exponential", 42)
	close(writer1)
	written1.Wait()
	writer2, written2, functions2 := commonInit(filepath.Join(temporaryDir, "second.csv"), tracePath, 3, 0, "exponential", 42)
	close(writer2)
	written2.Wait()

	if len(functions1) != len(functions2) {
		t.Fatalf("function count differs across parser runs: %d vs %d", len(functions1), len(functions2))
	}
	for i := range functions1 {
		if got, want := functionID(functions1[i].Name, true), functionID(functions2[i].Name, true); got != want {
			t.Fatalf("functionNum %d maps to %q in first run and %q in second", i, got, want)
		}
	}
}

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
