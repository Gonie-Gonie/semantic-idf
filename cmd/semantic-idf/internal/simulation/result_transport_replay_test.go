package simulation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Opt-in timing of a preserved run; never launches EnergyPlus or changes its
// files. The output is the exact result JSON used by the desktop transport.
func TestSimulationResultTransportSavedReplay(t *testing.T) {
	directory := os.Getenv("SIMULATION_TRANSPORT_REPLAY_DIR")
	if directory == "" {
		t.Skip("set SIMULATION_TRANSPORT_REPLAY_DIR and SIMULATION_TRANSPORT_REPLAY_INPUT for saved-run transport timing")
	}
	input := os.Getenv("SIMULATION_TRANSPORT_REPLAY_INPUT")
	if input == "" {
		t.Fatal("explicit preserved input path required")
	}
	before := epathReplayDirectorySnapshot(t, directory)
	t.Cleanup(func() {
		if !reflect.DeepEqual(before, epathReplayDirectorySnapshot(t, directory)) {
			t.Error("saved capture changed during transport replay")
		}
	})
	data, err := os.ReadFile(filepath.Join(directory, "semantic-idf-run-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plan PurposeRunPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	request := SimulationPurposeRequest{
		Purposes: plan.Purposes, AllocationPolicy: plan.AllocationPolicy,
		BasicEnergyDetail: plan.BasicEnergyDetail, ZoneHeatFlowDetail: plan.ZoneHeatFlowDetail,
		Scope: SimulationPurposeScope{ZoneMode: plan.ZoneMode, ZoneNames: plan.ZoneNames,
			PeriodMode: plan.PeriodMode, PeriodStart: plan.PeriodStart, PeriodEnd: plan.PeriodEnd},
	}
	result := SimulationRunResult{RunID: "transport-saved-replay", Status: "succeeded", InputPath: input,
		OutputDirectory: directory, PurposeRunPlan: &plan}
	started := time.Now()
	readSimulationOutputsWithProgress(&result, result.RunID, func(progress SimulationProgress) {
		t.Logf("%s after %s", progress.Phase, time.Since(started))
	}, input)
	t.Logf("read outputs: %s; series=%d", time.Since(started), len(result.Series))
	if !result.ERR.Completed || result.ERR.Severe != 0 || result.ERR.Fatal != 0 {
		t.Fatalf("capture did not complete successfully: %+v", result.ERR)
	}
	started = time.Now()
	bundle := buildPurposeResultBundleWithProgress(&result, request, nil)
	result.PurposeResults = &bundle
	t.Logf("build bundle: %s; loops=%d", time.Since(started), len(bundle.HVACLoops))
	for _, loop := range bundle.HVACLoops {
		points := 0
		for _, series := range loop.Series {
			points += len(series.Points)
		}
		t.Logf("loop %s: node series=%d, points=%d, components=%d", loop.Name, len(loop.Series), points, len(loop.Components))
	}
	started = time.Now()
	transport := CompactResultTransport(&result)
	t.Logf("compact column preparation: %s", time.Since(started))
	if wire, ok := transport.(*ResultTransport); ok {
		started = time.Now()
		samples := validateResultTransportBindings(t, &result, wire)
		t.Logf("verified exact X, label, value bits for %d samples across %d bindings: %s", samples, len(wire.PointBindings), time.Since(started))
	}
	started = time.Now()
	payload, err := json.Marshal(transport)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("compact cache / direct HTTP serialization: %s; bytes=%d", time.Since(started), len(payload))
	if wire, ok := transport.(*ResultTransport); ok {
		points := 0
		for _, set := range wire.PointSets {
			points += len(set.Values)
		}
		t.Logf("unique point sets=%d; timelines=%d; bindings=%d; unique samples=%d", len(wire.PointSets), len(wire.Timelines), len(wire.PointBindings), points)
	}
	if output := os.Getenv("SIMULATION_TRANSPORT_REPLAY_OUTPUT"); output != "" {
		file := purposeReplayArtifact(t, directory, output)
		_, writeErr := file.Write(payload)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("save transport result: %v / %v", writeErr, closeErr)
		}
	}
	if os.Getenv("SIMULATION_TRANSPORT_REPLAY_LEGACY") == "" {
		return
	}
	started = time.Now()
	callback, err := json.Marshal(map[string]any{"result": &result, "callbackid": "transport-replay"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("previous extra Wails result serialization: %s; bytes=%d", time.Since(started), len(callback))
	started = time.Now()
	scriptString, err := json.Marshal(string(callback))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("previous extra Windows Wails callback escaping: %s; bytes=%d (excludes WebView Eval)", time.Since(started), len(scriptString))
}
