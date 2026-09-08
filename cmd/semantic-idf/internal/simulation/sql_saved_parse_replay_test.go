package simulation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Opt-in performance diagnosis, not an independent numerical oracle. This
// records the SQL parser's complete output without changing its implementation,
// the default deadline, the saved input or any engine artifact.
func TestSQLSavedParseReplay(t *testing.T) {
	directory := strings.TrimSpace(os.Getenv("SEMANTIC_IDF_SQL_REPLAY_DIR"))
	if directory == "" {
		t.Skip("set SEMANTIC_IDF_SQL_REPLAY_DIR and SEMANTIC_IDF_SQL_REPLAY_OUTPUT for read-only SQL parser diagnosis")
	}
	output := strings.TrimSpace(os.Getenv("SEMANTIC_IDF_SQL_REPLAY_OUTPUT"))
	if output == "" {
		t.Fatal("explicit new .runtime result artifact required")
	}
	comparisonPath := strings.TrimSpace(os.Getenv("SEMANTIC_IDF_SQL_REPLAY_COMPARE"))
	var comparisonBytes []byte
	if comparisonPath != "" {
		var err error
		comparisonBytes, err = os.ReadFile(comparisonPath)
		if err != nil {
			t.Fatal(err)
		}
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	output, err = filepath.Abs(output)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := epathRealDirectories(t)
	relative, err := filepath.Rel(filepath.Join(root, ".runtime"), output)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		t.Fatal("SQL diagnosis must be written beneath repository .runtime")
	}
	before := epathReplayDirectorySnapshot(t, directory)
	t.Cleanup(func() {
		if !reflect.DeepEqual(before, epathReplayDirectorySnapshot(t, directory)) {
			t.Error("saved SQL/input/log/plan capture changed during parser diagnosis")
		}
	})
	sqlPath := filepath.Join(directory, "eplusout.sql")
	if state, ok := before["eplusout.sql"]; !ok || state.Size == 0 {
		t.Fatal("exact capture eplusout.sql is missing or empty")
	}
	planBytes, err := os.ReadFile(filepath.Join(directory, "semantic-idf-run-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plan PurposeRunPlan
	if err := json.Unmarshal(planBytes, &plan); err != nil || len(plan.Purposes) == 0 {
		t.Fatalf("actual flat run plan required: %v", err)
	}
	// Reserve both paths before spending time reading. O_EXCL prevents any
	// baseline replacement, including accidental reruns of this diagnostic.
	resultFile := purposeReplayArtifact(t, directory, output)
	defer resultFile.Close()
	metadataFile := purposeReplayArtifact(t, directory, output+".meta.json")
	defer metadataFile.Close()
	t.Logf("unlimited SQL parse starts: %s; compact walker with original-schema fallback; no engine or file fallback parser", sqlPath)
	started := time.Now()
	baseline, err := parseSimulationSQLWithContext(context.Background(), sqlPath, plan)
	unlimitedElapsed := time.Since(started)
	if err != nil || !sqlParseResultHasData(baseline) {
		t.Fatalf("unlimited SQL parse failed after %s: %v", unlimitedElapsed, err)
	}
	encoded, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	comparisonEqual := comparisonPath == "" || bytes.Equal(comparisonBytes, encoded)
	if _, err := resultFile.Write(encoded); err != nil {
		t.Fatal(err)
	}
	if err := resultFile.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("unlimited SQL parse: %s; series=%d; HeatFlow zones=%d frames=%d/%d source=%s; baseline bytes=%d SHA256=%x",
		unlimitedElapsed, len(baseline.Series), len(baseline.HeatFlow.Zones), baseline.HeatFlow.FrameCount, baseline.HeatFlow.OriginalFrameCount, baseline.HeatFlow.SourceFile, len(encoded), sha256.Sum256(encoded))
	if comparisonPath != "" {
		t.Logf("original unlimited baseline byte-exact comparison=%v; baseline=%s SHA256=%x", comparisonEqual, comparisonPath, sha256.Sum256(comparisonBytes))
	}
	t.Logf("default %s deadline comparison starts; errors remain errors", defaultSQLParseTimeout)
	started = time.Now()
	limited, limitedErr := parseSimulationSQL(sqlPath, plan)
	limitedElapsed := time.Since(started)
	status := "completed_sql_first"
	if limitedErr != nil {
		status = "error_fallback_discards_partial_sql"
		if errors.Is(limitedErr, context.DeadlineExceeded) {
			status = "deadline_fallback_discards_partial_sql"
		}
	}
	partialSeriesEqual := len(limited.Series) == 0 || reflect.DeepEqual(limited.Series, baseline.Series)
	partialHeatFlowEqual := len(limited.HeatFlow.Zones) == 0 || reflect.DeepEqual(limited.HeatFlow, baseline.HeatFlow)
	allEqual := reflect.DeepEqual(limited, baseline)
	captureUnchanged := reflect.DeepEqual(before, epathReplayDirectorySnapshot(t, directory))
	metadata := map[string]any{
		"schema": "semantic-idf.sql-parse-performance-replay/v1", "purpose": "performance_regression_not_numerical_oracle",
		"createdAt": time.Now().UTC().Format(time.RFC3339Nano), "sqlPath": sqlPath,
		"captureSnapshot": before, "captureUnchanged": captureUnchanged,
		"parser": "parseSimulationSQLWithContext", "walker": "compact_report_values_with_original_schema_fallback",
		"comparison": map[string]any{"path": comparisonPath, "bytes": len(comparisonBytes), "SHA256": fmt.Sprintf("%x", sha256.Sum256(comparisonBytes)), "byteExact": comparisonEqual, "requested": comparisonPath != ""},
		"output":     output, "outputSHA256": fmt.Sprintf("%x", sha256.Sum256(encoded)), "outputBytes": len(encoded),
		"unlimited": map[string]any{"elapsed": unlimitedElapsed.String(), "series": len(baseline.Series), "heatFlowZones": len(baseline.HeatFlow.Zones), "heatFlowFrames": baseline.HeatFlow.FrameCount, "heatFlowOriginalFrames": baseline.HeatFlow.OriginalFrameCount, "heatFlowSource": baseline.HeatFlow.SourceFile},
		"defaultDeadline": map[string]any{
			"limit": defaultSQLParseTimeout.String(), "elapsed": limitedElapsed.String(), "status": status,
			"error": fmt.Sprint(limitedErr), "series": len(limited.Series), "heatFlowZones": len(limited.HeatFlow.Zones), "heatFlowFrames": limited.HeatFlow.FrameCount,
			"returnedPartialData":          limitedErr != nil && sqlParseResultHasData(limited),
			"parseSQLResultsWouldKeepData": limitedErr == nil && sqlParseResultHasData(limited),
			"presentSeriesMatchUnlimited":  partialSeriesEqual, "presentHeatFlowMatchesUnlimited": partialHeatFlowEqual, "entireResultMatchesUnlimited": allEqual,
		},
	}
	if err := json.NewEncoder(metadataFile).Encode(metadata); err != nil {
		t.Fatal(err)
	}
	if err := metadataFile.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("default deadline: %s; status=%s err=%v series=%d HeatFlow zones=%d; valid SQL partial data discarded=%v",
		limitedElapsed, status, limitedErr, len(limited.Series), len(limited.HeatFlow.Zones), limitedErr != nil && sqlParseResultHasData(limited))
	t.Logf("capture SQL/input/log/plan unchanged=%v; metadata=%s.meta.json", captureUnchanged, output)
	if !captureUnchanged || !comparisonEqual || !partialSeriesEqual || !partialHeatFlowEqual || limitedErr == nil && !allEqual {
		t.Fatal("read-only or completed-parser equivalence contract failed; diagnostic preserved")
	}
}
