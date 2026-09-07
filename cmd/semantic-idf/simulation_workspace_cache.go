package main

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

// Keep only the most recent single-file result for auxiliary-page round trips.
// The desktop process survives a webview document reload; browser workspace
// snapshots need only an exact input hash and run ID, never the large payload.
type simulationWorkspaceCache struct {
	mu              sync.RWMutex
	textHash        string
	runID           string
	payload         []byte
	requestSequence uint64
}

func (a *App) rememberSimulationResult(text string, result *simulation.SimulationRunResult) {
	a.rememberSimulationResultForRequest(a.beginSimulationResultRequest(), text, result)
}

func (a *App) beginSimulationResultRequest() uint64 {
	if a == nil {
		return 0
	}
	cache := &a.simulationWorkspaceCache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.requestSequence++
	return cache.requestSequence
}

func (a *App) rememberSimulationResultForRequest(sequence uint64, text string, result *simulation.SimulationRunResult) {
	if a == nil || strings.TrimSpace(text) == "" || result == nil || result.RunID == "" {
		return
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return
	}
	cache := &a.simulationWorkspaceCache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	// A slow older run may finish after the user starts a newer document/run.
	// Its completion cannot evict that newer workspace result.
	if sequence != cache.requestSequence {
		return
	}
	cache.textHash = analysisTextHash(text)
	cache.runID = result.RunID
	cache.payload = payload
}

// GetCachedSimulationResult is a read-only, exact workspace lookup. A miss does
// not load output files, parse SQL, analyze an input, or start EnergyPlus.
func (a *App) GetCachedSimulationResult(textHash, runID string) (json.RawMessage, error) {
	if a == nil || textHash == "" || runID == "" {
		return nil, nil
	}
	cache := &a.simulationWorkspaceCache
	cache.mu.RLock()
	if cache.textHash != textHash || cache.runID != runID || len(cache.payload) == 0 {
		cache.mu.RUnlock()
		return nil, nil
	}
	// Return independent wire bytes. Decoding through SimulationRunResult would
	// invoke stored-result migration or aggregation hooks and could change a fresh
	// result during navigation. RawMessage is serialized as the original object
	// by both Wails and the HTTP bridge, not as a quoted JSON string.
	payload := append(json.RawMessage(nil), cache.payload...)
	cache.mu.RUnlock()
	return payload, nil
}
