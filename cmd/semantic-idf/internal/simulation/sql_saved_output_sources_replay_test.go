package simulation

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Opt-in integration replay of source selection, not a numerical acceptance
// oracle. The comparison must be a preserved, complete SQL parse of this exact
// capture, rather than a previous run that silently fell back to CSV/ESO.
func TestSQLSavedOutputSourcesReplay(t *testing.T) {
	directory := strings.TrimSpace(os.Getenv("SEMANTIC_IDF_SQL_REPLAY_DIR"))
	if directory == "" {
		t.Skip("set SEMANTIC_IDF_SQL_REPLAY_DIR and SEMANTIC_IDF_SQL_REPLAY_COMPARE for read-only source-selection replay")
	}
	comparison := strings.TrimSpace(os.Getenv("SEMANTIC_IDF_SQL_REPLAY_COMPARE"))
	if comparison == "" {
		t.Fatal("a preserved unlimited SQL baseline and its metadata are required")
	}
	before := epathReplayDirectorySnapshot(t, directory)
	t.Cleanup(func() {
		if !reflect.DeepEqual(before, epathReplayDirectorySnapshot(t, directory)) {
			t.Error("saved capture changed during read-only source-selection replay")
		}
	})
	want, err := os.ReadFile(comparison)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile(comparison + ".meta.json")
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		CaptureSnapshot  map[string]epathReplayFileState `json:"captureSnapshot"`
		CaptureUnchanged bool                            `json:"captureUnchanged"`
		OutputSHA256     string                          `json:"outputSHA256"`
	}
	if err := json.Unmarshal(metadata, &proof); err != nil {
		t.Fatal(err)
	}
	if !proof.CaptureUnchanged || !reflect.DeepEqual(proof.CaptureSnapshot, before) || proof.OutputSHA256 != fmt.Sprintf("%x", sha256.Sum256(want)) {
		t.Fatal("baseline metadata does not match the exact capture and result bytes")
	}
	var expected map[string]json.RawMessage
	if err := json.Unmarshal(want, &expected); err != nil {
		t.Fatal(err)
	}
	if len(expected["series"]) == 0 || len(expected["heatFlow"]) == 0 {
		t.Fatal("comparison requires both complete SQL series and heat-flow results")
	}
	planBytes, err := os.ReadFile(filepath.Join(directory, "semantic-idf-run-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plan PurposeRunPlan
	if err := json.Unmarshal(planBytes, &plan); err != nil || len(plan.Purposes) == 0 {
		t.Fatalf("actual flat run plan required: %v", err)
	}
	result := SimulationRunResult{OutputDirectory: directory, PurposeRunPlan: &plan}
	started := time.Now()
	readSimulationOutputs(&result)
	t.Logf("initial outputs: %s; sources=%v; series=%d; heat-flow zones=%d frames=%d/%d source=%s",
		time.Since(started), result.ResultSources, len(result.Series), len(result.HeatFlow.Zones), result.HeatFlow.FrameCount, result.HeatFlow.OriginalFrameCount, result.HeatFlow.SourceFile)
	if !result.ERR.Completed || result.ERR.Severe != 0 || result.ERR.Fatal != 0 {
		t.Fatalf("capture is not a successful engine run: %+v", result.ERR)
	}
	if len(result.ResultSources) == 0 || result.ResultSources[0] != "sql" {
		t.Errorf("SQL-first result selection failed: %v", result.ResultSources)
	}
	for _, source := range result.ResultSources {
		if source == "eso" {
			t.Error("complete SQL result incorrectly triggered ESO fallback")
		}
	}
	for field, value := range map[string]any{"series": result.Series, "heatFlow": result.HeatFlow} {
		actual, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(actual, expected[field]) {
			t.Errorf("%s differs from the complete SQL baseline; no numeric, metadata or ordering normalization is allowed", field)
		}
	}
	baselineAfter, err := os.ReadFile(comparison)
	if err != nil || !bytes.Equal(baselineAfter, want) {
		t.Fatal("preserved baseline changed during source-selection replay")
	}
}
