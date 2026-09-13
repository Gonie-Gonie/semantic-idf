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

// The returned bytes belong to this completed request even if a newer request
// has taken over the workspace cache. They are immutable and can be written
// directly to the HTTP response without serializing the large result again.
func (a *App) rememberSimulationResultForRequest(sequence uint64, text string, result *simulation.SimulationRunResult) []byte {
	return a.rememberSimulationResultForTransport(sequence, text, result, false)
}

func (a *App) rememberSimulationResultForTransport(sequence uint64, text string, result *simulation.SimulationRunResult, compact bool) []byte {
	if result == nil {
		return nil
	}
	canCache := a != nil && strings.TrimSpace(text) != "" && result.RunID != ""
	if !canCache && !compact {
		return nil
	}
	var wire any = result
	if compact {
		wire = simulation.CompactResultTransport(result)
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return nil
	}
	if !canCache {
		return payload
	}
	cache := &a.simulationWorkspaceCache
	cache.mu.Lock()
	defer cache.mu.Unlock()
	// A slow older run may finish after the user starts a newer document/run.
	// Its completion cannot evict that newer workspace result.
	if sequence != cache.requestSequence {
		return payload
	}
	cache.textHash = analysisTextHash(text)
	cache.runID = result.RunID
	cache.payload = payload
	return payload
}

// GetCachedSimulationResult is a read-only, exact workspace lookup. A miss does
// not load output files, parse SQL, analyze an input, or start EnergyPlus.
func (a *App) GetCachedSimulationResult(textHash, runID string) (json.RawMessage, error) {
	payload := a.cachedSimulationResultBytes(textHash, runID)
	// Public callers may mutate their copy; the HTTP handler instead writes the
	// immutable bytes directly and avoids this full-payload allocation.
	return append(json.RawMessage(nil), payload...), nil
}

func (a *App) cachedSimulationResultBytes(textHash, runID string) []byte {
	if a == nil || textHash == "" || runID == "" {
		return nil
	}
	cache := &a.simulationWorkspaceCache
	cache.mu.RLock()
	if cache.textHash != textHash || cache.runID != runID || len(cache.payload) == 0 {
		cache.mu.RUnlock()
		return nil
	}
	// Cached bytes are never mutated, including after replacement. Retaining this
	// slice is safe after unlocking and does not block a new run during transfer.
	// Do not decode it through stored-result migration or aggregation hooks.
	payload := cache.payload
	cache.mu.RUnlock()
	return payload
}
