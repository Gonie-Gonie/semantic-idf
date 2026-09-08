package simulation

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Replay the complete post-engine pipeline of an interrupted run, including
// combined purposes. Opt-in only: never launch EnergyPlus or repair the capture.
func TestPurposeSavedBundleReplay(t *testing.T) {
	directory := os.Getenv("SEMANTIC_IDF_BUNDLE_REPLAY_DIR")
	if directory == "" {
		t.Skip("set SEMANTIC_IDF_BUNDLE_REPLAY_DIR and SEMANTIC_IDF_BUNDLE_REPLAY_INPUT for a read-only saved-run replay")
	}
	input := os.Getenv("SEMANTIC_IDF_BUNDLE_REPLAY_INPUT")
	if input == "" {
		t.Fatal("explicit saved input path required")
	}
	before := epathReplayDirectorySnapshot(t, directory)
	t.Cleanup(func() {
		if !reflect.DeepEqual(before, epathReplayDirectorySnapshot(t, directory)) {
			t.Error("saved capture changed during read-only replay")
		}
	})
	content, err := os.ReadFile(filepath.Join(directory, "semantic-idf-run-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plan PurposeRunPlan
	if err := json.Unmarshal(content, &plan); err != nil {
		t.Fatal(err)
	}
	request := SimulationPurposeRequest{
		Purposes: plan.Purposes, AllocationPolicy: plan.AllocationPolicy,
		BasicEnergyDetail: plan.BasicEnergyDetail, ZoneHeatFlowDetail: plan.ZoneHeatFlowDetail,
		Scope: SimulationPurposeScope{ZoneMode: plan.ZoneMode, ZoneNames: plan.ZoneNames,
			PeriodMode: plan.PeriodMode, PeriodStart: plan.PeriodStart, PeriodEnd: plan.PeriodEnd},
	}
	result := SimulationRunResult{InputPath: input, OutputDirectory: directory, PurposeRunPlan: &plan}
	started := time.Now()
	readSimulationOutputsWithProgress(&result, "saved-replay", func(progress SimulationProgress) {
		t.Logf("%s after %s", progress.Phase, time.Since(started))
	}, input)
	t.Logf("read outputs: %s; series=%d, heat-flow zones=%d", time.Since(started), len(result.Series), len(result.HeatFlow.Zones))
	if !result.ERR.Completed || result.ERR.Severe != 0 || result.ERR.Fatal != 0 {
		t.Fatalf("capture did not complete successfully: %+v", result.ERR)
	}
	// An optional bounded CPU profile is available before a long replay finishes.
	// Keep it outside the capture, and never overwrite an earlier diagnosis.
	if profilePath := os.Getenv("SEMANTIC_IDF_BUNDLE_REPLAY_PROFILE"); profilePath != "" {
		file := purposeReplayArtifact(t, directory, profilePath)
		if err := pprof.StartCPUProfile(file); err != nil {
			file.Close()
			t.Fatal(err)
		}
		var once sync.Once
		stop := func() { once.Do(func() { pprof.StopCPUProfile(); file.Close() }) }
		timer := time.AfterFunc(45*time.Second, stop)
		t.Cleanup(func() { timer.Stop(); stop() })
	}
	started = time.Now()
	bundle := buildPurposeResultBundleWithProgress(&result, request, func(phase, message string) {
		t.Logf("%s after %s: %s", phase, time.Since(started), message)
	})
	t.Logf("build complete bundle: %s", time.Since(started))
	if purposeIDsContain(plan.Purposes, SimulationPurposeBasicEnergy) && len(bundle.EnergyExplanation.Nodes) == 0 {
		t.Fatal("missing energy explanation")
	}
	if purposeIDsContain(plan.Purposes, SimulationPurposeZoneHeatFlow) && plan.ZoneHeatFlowDetail == PurposeZoneHeatFlowDetailSurface && !bundle.ThermalTopology.Available {
		t.Fatalf("missing thermal topology: %s", bundle.ThermalTopology.UnavailableReason)
	}
	started = time.Now()
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("serialize bundle: %s; bytes=%d", time.Since(started), len(encoded))
	t.Logf("bundle SHA256: %x", sha256.Sum256(encoded))
	if output := os.Getenv("SEMANTIC_IDF_BUNDLE_REPLAY_OUTPUT"); output != "" {
		file := purposeReplayArtifact(t, directory, output)
		_, writeErr := file.Write(encoded)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("save diagnostic result: %v / %v", writeErr, closeErr)
		}
	}
	if baseline := os.Getenv("SEMANTIC_IDF_BUNDLE_REPLAY_COMPARE"); baseline != "" {
		want, err := os.ReadFile(baseline)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, encoded) {
			// Existing reconciliation unions collect source IDs from Go maps.
			// Their membership is unordered; nothing else may be normalized,
			// rounded, migrated, dropped or reordered by this comparison.
			if !reflect.DeepEqual(purposeReplayComparableJSON(t, want), purposeReplayComparableJSON(t, encoded)) {
				t.Fatalf("bundle changed beyond source-ID ordering: baseline SHA256=%x, current SHA256=%x", sha256.Sum256(want), sha256.Sum256(encoded))
			}
			t.Log("all values, metadata and provenance membership exactly match baseline; only unordered source-ID lists differ")
		}
	}
}

func purposeReplayComparableJSON(t *testing.T, data []byte) any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber() // Preserve exact decimal tokens, not rounded float64s.
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	var visit func(any)
	visit = func(value any) {
		switch item := value.(type) {
		case map[string]any:
			for key, child := range item {
				if ids, ok := child.([]any); key == "sourceIds" && ok {
					for _, id := range ids {
						if _, ok := id.(string); !ok {
							t.Fatal("non-string source ID in replay comparison")
						}
					}
					sort.Slice(ids, func(i, j int) bool { return ids[i].(string) < ids[j].(string) })
				}
				visit(child)
			}
		case []any:
			for _, child := range item {
				visit(child)
			}
		}
	}
	visit(value)
	return value
}

func TestPurposeReplayComparisonOnlyIgnoresSourceIDOrder(t *testing.T) {
	baseline := purposeReplayComparableJSON(t, []byte(`{"value":0.000000000000000001,"sourceIds":["b","a","a"],"values":[1,2],"rawValue":0}`))
	for _, test := range []struct {
		name, wire string
		equal      bool
	}{
		{"source order", `{"value":0.000000000000000001,"sourceIds":["a","b","a"],"values":[1,2],"rawValue":0}`, true},
		{"tiny quantity", `{"value":0.000000000000000002,"sourceIds":["a","b","a"],"values":[1,2],"rawValue":0}`, false},
		{"source removed", `{"value":0.000000000000000001,"sourceIds":["a","b"],"values":[1,2],"rawValue":0}`, false},
		{"value order", `{"value":0.000000000000000001,"sourceIds":["a","b","a"],"values":[2,1],"rawValue":0}`, false},
		{"zero absent", `{"value":0.000000000000000001,"sourceIds":["a","b","a"],"values":[1,2]}`, false},
		{"zero null", `{"value":0.000000000000000001,"sourceIds":["a","b","a"],"values":[1,2],"rawValue":null}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if equal := reflect.DeepEqual(baseline, purposeReplayComparableJSON(t, []byte(test.wire))); equal != test.equal {
				t.Fatalf("comparison equal=%v, want %v", equal, test.equal)
			}
		})
	}
}

func purposeReplayArtifact(t *testing.T, capture, output string) *os.File {
	t.Helper()
	absoluteOutput, err := filepath.Abs(output)
	if err != nil {
		t.Fatal(err)
	}
	absoluteCapture, err := filepath.Abs(capture)
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(absoluteCapture, absoluteOutput)
	if err != nil || (!filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		t.Fatal("diagnostic artifact must be outside the preserved capture")
	}
	file, err := os.OpenFile(absoluteOutput, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	return file
}
