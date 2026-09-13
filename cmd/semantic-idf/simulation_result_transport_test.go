package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/simulation"
)

func TestSimulationHTTPResultUsesExactSnapshot(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	app := NewApp()
	purpose := simulation.NormalizeSimulationPurposeRequest(nil)
	request := simulation.SimulationRunRequest{
		RunID: "http-result", Text: appMetricsIDF, Filename: "office.idf",
		PurposeRequest: &purpose,
		// Exercise preparation and the actual endpoint without starting an engine.
		EnergyPlusExecutablePath: filepath.Join(t.TempDir(), "absent-energyplus.exe"),
	}
	requestBytes, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	handler := appAssetHandler(app)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/simulation-run", bytes.NewReader(requestBytes)))
	if response.Code != http.StatusOK {
		t.Fatalf("run response: %d %s", response.Code, response.Body)
	}
	var result simulation.SimulationRunResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.RunID != request.RunID || result.Status != "missing_energyplus" {
		t.Fatalf("run response lost the result contract: %s, %v", response.Body, err)
	}
	want, err := app.GetCachedSimulationResult(analysisTextHash(request.Text), request.RunID)
	if err != nil || !bytes.Equal(response.Body.Bytes(), want) {
		t.Fatalf("HTTP and workspace snapshots differ: %v", err)
	}
	if response.Header().Get("Content-Length") != strconv.Itoa(len(want)) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected snapshot headers: %v", response.Header())
	}
	cacheResponse := httptest.NewRecorder()
	handler.ServeHTTP(cacheResponse, httptest.NewRequest(http.MethodPost, "/api/simulation-result-cache", strings.NewReader(`{"textHash":"`+analysisTextHash(request.Text)+`","runId":"`+request.RunID+`"}`)))
	if cacheResponse.Code != http.StatusOK || !bytes.Equal(cacheResponse.Body.Bytes(), want) {
		t.Fatalf("HTTP restore changed fresh result bytes: %d %s", cacheResponse.Code, cacheResponse.Body)
	}
}

func TestSimulationHTTPResponseSurvivesNewerWorkspaceRun(t *testing.T) {
	app := NewApp()
	older := app.beginSimulationResultRequest()
	newer := app.beginSimulationResultRequest()
	newerResult := &simulation.SimulationRunResult{RunID: "newer", Status: "succeeded"}
	app.rememberSimulationResultForRequest(newer, appMetricsIDF, newerResult)
	olderResult := &simulation.SimulationRunResult{
		RunID: "older", Status: "succeeded",
		Series: []simulation.SimulationSeries{{File: "eplusout.sql", Column: "T", Points: []simulation.SimulationPoint{{X: 1, Label: "01/01 01:00", Value: 0}, {X: 2, Label: "01/01 02:00", Value: 21.375}}}},
	}
	want, err := json.Marshal(olderResult)
	if err != nil {
		t.Fatal(err)
	}
	payload := app.rememberSimulationResultForRequest(older, appMetricsIDF, olderResult)
	olderResult.Series[0].Points[1].Value = 999
	response := httptest.NewRecorder()
	if err := writeSimulationResultBytes(response, payload); err != nil || !bytes.Equal(response.Body.Bytes(), want) {
		t.Fatalf("older request lost or changed its own response after cache replacement: %v", err)
	}
	if cached, _ := app.GetCachedSimulationResult(analysisTextHash(appMetricsIDF), "newer"); cached == nil {
		t.Fatal("older response evicted newer workspace result")
	}
	if cached, _ := app.GetCachedSimulationResult(analysisTextHash(appMetricsIDF), "older"); cached != nil {
		t.Fatal("older response replaced the workspace result")
	}
}

func TestSimulationHTTPTransportCapabilityRequiresHandler(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	app := NewApp()
	before, err := app.GetSimulationEnvironment()
	if err != nil || before.ResultHTTPAvailable {
		t.Fatalf("unconfigured backend advertised an HTTP result endpoint: %+v, %v", before, err)
	}
	_ = appAssetHandler(app)
	after, err := app.GetSimulationEnvironment()
	if err != nil || !after.ResultHTTPAvailable {
		t.Fatalf("configured backend did not advertise its HTTP result endpoint: %+v, %v", after, err)
	}
}

func TestSimulationHTTPCompactSnapshotPreservesSourceAndRestore(t *testing.T) {
	app := NewApp()
	points := make([]simulation.SimulationPoint, 256)
	for index := range points {
		points[index] = simulation.SimulationPoint{X: index + 1, Label: "hour " + strconv.Itoa(index+1), Value: float64(index) / 10}
	}
	result := &simulation.SimulationRunResult{
		RunID: "compact", Status: "succeeded",
		Series: []simulation.SimulationSeries{{File: "eplusout.sql", Column: "T", Points: points}},
	}
	before, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	payload := app.rememberSimulationResultForTransport(app.beginSimulationResultRequest(), appMetricsIDF, result, true)
	after, err := json.Marshal(result)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("compact snapshot changed the typed result: %v", err)
	}
	var wire struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil || wire.Schema != "semantic-idf.simulation-transfer/v1" {
		t.Fatalf("compact snapshot not encoded: %s, %v", payload, err)
	}
	result.Series[0].Points[10].Value = 999
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/simulation-result-cache", strings.NewReader(`{"textHash":"`+analysisTextHash(appMetricsIDF)+`","runId":"compact"}`))
	request.Header.Set("Accept", compactSimulationResultMediaType)
	appAssetHandler(app).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), payload) {
		t.Fatalf("compact restore changed the original wire snapshot: %d", response.Code)
	}
	if !acceptsCompactSimulationResult(request) {
		t.Fatal("explicit compact transport was not accepted")
	}
	request.Header.Set("Accept", "application/json")
	if acceptsCompactSimulationResult(request) {
		t.Fatal("ordinary JSON callers were opted into a different transport")
	}
}
