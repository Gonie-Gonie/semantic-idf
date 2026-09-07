package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestEPATH160SimulationWorkspaceCachePreservesExactWireResult(t *testing.T) {
	app := NewApp()
	text := "Version,24.1;\r\nZone,Office;\r\n"
	result := &simulation.SimulationRunResult{
		RunID: "epath160-run", Status: "succeeded",
		Series: []simulation.SimulationSeries{{File: "eplusout.sql", Column: "Cooling", Points: []simulation.SimulationPoint{{Value: 0}, {Value: 42.5}}}},
		PurposeResults: &simulation.PurposeResultBundle{EnergyExplanation: simulation.EnergyExplanationResult{
			// Deliberately sparse fresh wire values must not acquire defaults from
			// stored-result normalization on a read-only workspace lookup.
			Nodes: []simulation.EnergyExplanationNode{{ID: "driver.zero", Level: "driver", Value: 0, Unit: "kWh"}},
		}},
	}
	want, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	app.rememberSimulationResult(text, result)
	result.Series[0].Points[1].Value = 999
	result.PurposeResults.EnergyExplanation.Nodes[0].Value = 123
	got, err := app.GetCachedSimulationResult(analysisTextHash(strings.ReplaceAll(text, "\r\n", "\n")), result.RunID)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("fresh wire result changed: err=%v\ngot=%s\nwant=%s", err, got, want)
	}
	got[0] = '!'
	again, err := app.GetCachedSimulationResult(analysisTextHash(text), result.RunID)
	if err != nil || !bytes.Equal(again, want) {
		t.Fatalf("caller mutation corrupted cached bytes: %v", err)
	}
	for _, query := range [][2]string{{"", result.RunID}, {analysisTextHash(text), ""}, {analysisTextHash(text + "!"), result.RunID}, {analysisTextHash(text), "another-run"}} {
		if miss, err := app.GetCachedSimulationResult(query[0], query[1]); err != nil || miss != nil {
			t.Fatalf("inexact workspace identity returned a result: %v, %s, %v", query, miss, err)
		}
	}
	app.rememberSimulationResult(text, &simulation.SimulationRunResult{RunID: "newer-run", Status: "failed"})
	if old, _ := app.GetCachedSimulationResult(analysisTextHash(text), result.RunID); old != nil {
		t.Fatal("cache retained more than the most recent single-file result")
	}
	if cached, _ := app.GetCachedSimulationResult(analysisTextHash(text), "newer-run"); cached == nil {
		t.Fatal("most recent result unavailable")
	}
	if restarted, _ := NewApp().GetCachedSimulationResult(analysisTextHash(text), "newer-run"); restarted != nil {
		t.Fatal("process-local cache invented a result after restart")
	}
}

func TestEPATH160SimulationWorkspaceCacheHTTPIsReadOnlyAndObjectShaped(t *testing.T) {
	app := NewApp()
	app.rememberSimulationResult(appMetricsIDF, &simulation.SimulationRunResult{RunID: "wire-run", Status: "succeeded"})
	handler := appAssetHandler(app)
	for _, item := range []struct {
		method, body string
		status       int
		runID        string
	}{
		{http.MethodPost, `{"textHash":"` + analysisTextHash(appMetricsIDF) + `","runId":"wire-run"}`, http.StatusOK, "wire-run"},
		{http.MethodPost, `{"textHash":"missing","runId":"wire-run"}`, http.StatusOK, ""},
		{http.MethodPost, `{`, http.StatusBadRequest, ""},
		{http.MethodGet, "", http.StatusMethodNotAllowed, ""},
		{http.MethodPost, `{"textHash":"` + strings.Repeat("x", 5000) + `"}`, http.StatusBadRequest, ""},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(item.method, "/api/simulation-result-cache", strings.NewReader(item.body)))
		if response.Code != item.status {
			t.Fatalf("HTTP status=%d, want=%d: %s", response.Code, item.status, response.Body)
		}
		if item.status != http.StatusOK {
			continue
		}
		if item.runID == "" {
			if strings.TrimSpace(response.Body.String()) != "null" {
				t.Fatalf("cache miss=%s, want null", response.Body)
			}
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload["runId"] != item.runID {
			t.Fatalf("bridge must return a result object, not a JSON string/byte array: %s, %v", response.Body, err)
		}
	}
}

func TestEPATH160SimulationWorkspaceCacheConcurrentReadsAreIndependent(t *testing.T) {
	app := NewApp()
	app.rememberSimulationResult(appMetricsIDF, &simulation.SimulationRunResult{RunID: "parallel", Status: "succeeded"})
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := app.GetCachedSimulationResult(analysisTextHash(appMetricsIDF), "parallel")
			if err != nil || !json.Valid(got) {
				t.Errorf("concurrent lookup invalid: %v", err)
				return
			}
			got[0] = '!'
		}()
	}
	group.Wait()
}

func TestEPATH160RunEntryCachesOriginalInputBeforePurposeOutputs(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	app := NewApp()
	purpose := simulation.NormalizeSimulationPurposeRequest(nil)
	request := simulation.SimulationRunRequest{
		RunID: "purpose-cache", Text: appMetricsIDF, Filename: "office.idf",
		PurposeRequest: &purpose,
		// An explicit absent executable exercises the real entry without ever
		// launching EnergyPlus or reading output data.
		EnergyPlusExecutablePath: filepath.Join(t.TempDir(), "absent-energyplus.exe"),
	}
	prepared, err := preparePurposeSimulationRequest(request)
	if err != nil || prepared.Text == request.Text {
		t.Fatalf("fixture must inject purpose output requests: %v", err)
	}
	result, err := app.RunPurposeSimulationText(request)
	if err != nil || result == nil || result.Status != "missing_energyplus" {
		t.Fatalf("real non-executing run entry failed: %+v, %v", result, err)
	}
	want, _ := json.Marshal(result)
	got, err := app.GetCachedSimulationResult(analysisTextHash(request.Text), request.RunID)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("real entry did not cache original document result: %s, %v", got, err)
	}
	if wrong, _ := app.GetCachedSimulationResult(analysisTextHash(prepared.Text), request.RunID); wrong != nil {
		t.Fatal("cache identity used the output-injected run copy instead of user input")
	}
}

func TestEPATH160SlowOlderRunCannotEvictNewerWorkspaceResult(t *testing.T) {
	app := NewApp()
	older := app.beginSimulationResultRequest()
	newer := app.beginSimulationResultRequest()
	app.rememberSimulationResultForRequest(newer, appMetricsIDF, &simulation.SimulationRunResult{RunID: "newer", Status: "succeeded"})
	app.rememberSimulationResultForRequest(older, appMetricsIDF, &simulation.SimulationRunResult{RunID: "older", Status: "succeeded"})
	if got, _ := app.GetCachedSimulationResult(analysisTextHash(appMetricsIDF), "newer"); got == nil {
		t.Fatal("slow older completion evicted the active workspace result")
	}
	if got, _ := app.GetCachedSimulationResult(analysisTextHash(appMetricsIDF), "older"); got != nil {
		t.Fatal("slow older completion became the cached workspace result")
	}
}
