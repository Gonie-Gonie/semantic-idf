package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

const maxSimulationProgressSnapshots = 16

type simulationProgressSnapshot struct {
	generation uint64
	updated    time.Time
	payload    []byte
}

// Keep only the latest small event per run. Polling never retains a run log or
// model/result data, and slow clients cannot hold up the runner.
type simulationProgressCache struct {
	sync.Mutex
	generation uint64
	runs       map[string]simulationProgressSnapshot
}

func (a *App) beginSimulationProgress(runID string) func(simulation.SimulationProgress) bool {
	c := &a.simulationProgressCache
	c.Lock()
	if c.runs == nil {
		c.runs = make(map[string]simulationProgressSnapshot)
	}
	c.generation++
	generation := c.generation
	if _, exists := c.runs[runID]; !exists && len(c.runs) >= maxSimulationProgressSnapshots {
		var oldest string
		var oldestTime time.Time
		for id, item := range c.runs {
			if oldestTime.IsZero() || item.updated.Before(oldestTime) {
				oldest, oldestTime = id, item.updated
			}
		}
		delete(c.runs, oldest)
	}
	c.runs[runID] = simulationProgressSnapshot{generation: generation, updated: time.Now()}
	c.Unlock()
	return func(item simulation.SimulationProgress) bool {
		if item.RunID != runID {
			return false
		}
		payload, err := json.Marshal(item)
		if err != nil {
			return false
		}
		c.Lock()
		defer c.Unlock()
		previous, ok := c.runs[runID]
		if !ok || previous.generation != generation {
			return false
		}
		c.runs[runID] = simulationProgressSnapshot{generation: generation, updated: time.Now(), payload: payload}
		return true
	}
}

func (a *App) simulationProgressBytes(runID string) []byte {
	c := &a.simulationProgressCache
	c.Lock()
	defer c.Unlock()
	item, ok := c.runs[runID]
	if !ok || time.Since(item.updated) > 30*time.Minute || item.payload == nil {
		return []byte("null")
	}
	// Payloads are immutable after insertion, so response writes need no lock.
	return item.payload
}

func serveSimulationProgress(w http.ResponseWriter, r *http.Request, app *App) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runID := r.URL.Query().Get("runId")
	if runID == "" || len(runID) > 256 {
		http.Error(w, "a runId of at most 256 bytes is required", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeSimulationResultBytes(w, app.simulationProgressBytes(runID))
}
