package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestSimulationProgressHTTPIsolatedLatestAndReadOnly(t *testing.T) {
	app := NewApp()
	first := app.beginSimulationProgress("first")
	second := app.beginSimulationProgress("second")
	first(simulation.SimulationProgress{RunID: "first", Phase: "execute", Status: "running"})
	second(simulation.SimulationProgress{RunID: "second", Phase: "parse_sql", Status: "running"})
	first(simulation.SimulationProgress{RunID: "first", Phase: "complete", Status: "succeeded"})
	handler := appAssetHandler(app)
	for _, test := range []struct{ id, phase string }{{"first", "complete"}, {"second", "parse_sql"}, {"missing", ""}} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/simulation/progress?runId="+test.id, nil))
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("poll response: %d %v", response.Code, response.Header())
		}
		var item *simulation.SimulationProgress
		if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
		if test.phase == "" {
			if item != nil {
				t.Fatalf("unknown run exposed another run: %+v", item)
			}
		} else if item == nil || item.RunID != test.id || item.Phase != test.phase {
			t.Fatalf("wrong run or old event: %+v", item)
		}
	}
	for _, test := range []struct {
		method, url string
		code        int
	}{
		{http.MethodPost, "/api/simulation/progress?runId=first", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/simulation/progress", http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.url, nil))
		if response.Code != test.code {
			t.Fatalf("poll validation: %d, want %d", response.Code, test.code)
		}
	}
}

func TestSimulationProgressReusedRunIDRejectsOldWriter(t *testing.T) {
	app := NewApp()
	old := app.beginSimulationProgress("reused")
	old(simulation.SimulationProgress{RunID: "reused", Phase: "execute"})
	newer := app.beginSimulationProgress("reused")
	newer(simulation.SimulationProgress{RunID: "reused", Phase: "prepare"})
	if old(simulation.SimulationProgress{RunID: "reused", Phase: "complete"}) {
		t.Fatal("late old run replaced current progress")
	}
	if newer(simulation.SimulationProgress{RunID: "another", Phase: "complete"}) {
		t.Fatal("callback wrote another run's telemetry")
	}
	var item simulation.SimulationProgress
	if err := json.Unmarshal(app.simulationProgressBytes("reused"), &item); err != nil || item.Phase != "prepare" {
		t.Fatalf("late event corrupted progress: %+v %v", item, err)
	}
}

func TestSimulationProgressPreparationAndBatchCancellationTerminate(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	app := NewApp()
	// A preparation failure occurs before EnergyPlus or the runner's tracker.
	_, err := app.RunSimulationText(simulation.SimulationRunRequest{RunID: "bad-input", InputPath: filepath.Join(t.TempDir(), "missing.idf")})
	if err == nil {
		t.Fatal("missing input unexpectedly prepared")
	}
	var failure simulation.SimulationProgress
	if err := json.Unmarshal(app.simulationProgressBytes("bad-input"), &failure); err != nil || failure.Status != "failed" || failure.Phase != "request_failed" {
		t.Fatalf("preparation left progress running: %+v %v", failure, err)
	}
	batch, err := app.RunMultipleSimulations(simulation.MultiSimulationRequest{RunID: " batch/empty "})
	if err != nil || batch == nil || !batch.Canceled {
		t.Fatalf("empty batch: %+v %v", batch, err)
	}
	response := httptest.NewRecorder()
	appAssetHandler(app).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/simulation/progress?runId="+url.QueryEscape(batch.RunID), nil))
	var canceled simulation.SimulationProgress
	if err := json.Unmarshal(response.Body.Bytes(), &canceled); err != nil || canceled.Status != "canceled" || canceled.RunID != batch.RunID {
		t.Fatalf("normalized batch cancellation did not reach polling: %s %v", response.Body, err)
	}
}

func TestSimulationProgressConcurrentPollingRemainsBounded(t *testing.T) {
	app := NewApp()
	var workers sync.WaitGroup
	for i := 0; i < 40; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			id := fmt.Sprintf("run-%d", i)
			publish := app.beginSimulationProgress(id)
			for j := 0; j < 30; j++ {
				publish(simulation.SimulationProgress{RunID: id, Phase: "execute", Completed: j})
				var item *simulation.SimulationProgress
				if err := json.Unmarshal(app.simulationProgressBytes(id), &item); err != nil || (item != nil && item.RunID != id) {
					t.Errorf("concurrent poll mixed runs: %+v %v", item, err)
				}
			}
		}(i)
	}
	workers.Wait()
	app.simulationProgressCache.Lock()
	defer app.simulationProgressCache.Unlock()
	if len(app.simulationProgressCache.runs) > maxSimulationProgressSnapshots {
		t.Fatal("progress polling retained an unbounded run history")
	}
}
