package simulation

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRunMultipleSimulationsSerializesBatchCompletionProgress(t *testing.T) {
	directory := t.TempDir()
	paths := make([]string, 8)
	for index := range paths {
		paths[index] = writeBatchEngineInput(t, directory, fmt.Sprintf("model-%02d.idf", index), "Version, 24.2;\nZone, Office;\n")
	}

	var callbackActive atomic.Int32
	var callbackOverlap atomic.Bool
	var progressMu sync.Mutex
	executeCompleted := make([]int, 0, len(paths))
	result, err := RunMultipleSimulations(MultiSimulationRequest{
		RunID:                    "serialized-progress",
		InputPaths:               paths,
		EnergyPlusExecutablePath: filepath.Join(directory, "missing-energyplus"),
		WorkerCount:              4,
	}, func(progress SimulationProgress) {
		if progress.Phase != "execute" {
			return
		}
		if callbackActive.Add(1) != 1 {
			callbackOverlap.Store(true)
		}
		for range 100 {
			runtime.Gosched()
		}
		progressMu.Lock()
		executeCompleted = append(executeCompleted, progress.Completed)
		progressMu.Unlock()
		callbackActive.Add(-1)
	}, SimulationSettings{})
	if err != nil {
		t.Fatalf("RunMultipleSimulations returned error: %v", err)
	}
	if result.Completed != len(paths) || result.Failed != len(paths) {
		t.Fatalf("batch completion = %d/%d failed, want %d/%d", result.Completed, result.Failed, len(paths), len(paths))
	}
	if callbackOverlap.Load() {
		t.Fatal("batch completion progress callbacks overlapped")
	}
	if len(executeCompleted) != len(paths) {
		t.Fatalf("execute progress events = %d, want %d", len(executeCompleted), len(paths))
	}
	for index, completed := range executeCompleted {
		if completed != index+1 {
			t.Fatalf("execute progress event %d completed = %d, want %d", index, completed, index+1)
		}
	}
}
