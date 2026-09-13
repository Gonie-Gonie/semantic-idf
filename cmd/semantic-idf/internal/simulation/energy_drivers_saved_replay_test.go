package simulation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime/pprof"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Opt-in replay of only the Energy drivers stage. Uses the executed input and
// existing SQL; never runs EnergyPlus or modifies the preserved run directory.
func TestEnergyDriversSavedReplay(t *testing.T) {
	directory := os.Getenv("ENERGY_DRIVERS_REPLAY_DIR")
	if directory == "" {
		t.Skip("set ENERGY_DRIVERS_REPLAY_DIR and ENERGY_DRIVERS_REPLAY_INPUT for a saved driver-stage replay")
	}
	input := os.Getenv("ENERGY_DRIVERS_REPLAY_INPUT")
	if input == "" {
		t.Fatal("explicit executed input required")
	}
	before := epathReplayDirectorySnapshot(t, directory)
	t.Cleanup(func() {
		if !reflect.DeepEqual(before, epathReplayDirectorySnapshot(t, directory)) {
			t.Error("saved driver replay changed the captured run")
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
	doc, err := simulationDocumentFromInput(input)
	if err != nil {
		t.Fatal(err)
	}
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	stop := func() {}
	if path := os.Getenv("ENERGY_DRIVERS_REPLAY_PROFILE"); path != "" {
		file := purposeReplayArtifact(t, directory, path)
		if err := pprof.StartCPUProfile(file); err != nil {
			file.Close()
			t.Fatal(err)
		}
		var once sync.Once
		stop = func() { once.Do(func() { pprof.StopCPUProfile(); file.Close() }) }
		timer := time.AfterFunc(45*time.Second, stop)
		t.Cleanup(func() { timer.Stop(); stop() })
	}
	started := time.Now()
	explanation, err := parseSimulationEnergyExplanationSQLWithDriverContext(filepath.Join(directory, "eplusout.sql"), &plan, context)
	elapsed := time.Since(started)
	stop()
	if err != nil {
		t.Fatal(err)
	}
	if len(explanation.Nodes) == 0 || len(explanation.Sources) == 0 {
		t.Fatal("saved driver stage returned no energy results")
	}
	t.Logf("Energy drivers: %s; nodes=%d, sources=%d, hourly labels=%d", elapsed, len(explanation.Nodes), len(explanation.Sources), len(explanation.HourlyLabels))
	encoded, err := json.Marshal(explanation)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Complete driver-stage JSON: %d bytes", len(encoded))
	if output := os.Getenv("ENERGY_DRIVERS_REPLAY_OUTPUT"); output != "" {
		file := purposeReplayArtifact(t, directory, output)
		_, writeErr := file.Write(encoded)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("write driver evidence: %v / %v", writeErr, closeErr)
		}
	}
	if baseline := os.Getenv("ENERGY_DRIVERS_REPLAY_COMPARE"); baseline != "" {
		want, err := os.ReadFile(baseline)
		if err != nil {
			t.Fatal(err)
		}
		compareEnergyDriversReplay(t, want, encoded)
	}
}

func TestEnergyDriversSavedComparison(t *testing.T) {
	before, after := os.Getenv("ENERGY_DRIVERS_REPLAY_COMPARE"), os.Getenv("ENERGY_DRIVERS_REPLAY_OUTPUT")
	if before == "" || after == "" {
		t.Skip("set baseline and candidate driver-stage JSON paths to compare existing replays")
	}
	want, err := os.ReadFile(before)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(after)
	if err != nil {
		t.Fatal(err)
	}
	compareEnergyDriversReplay(t, want, got)
}

func compareEnergyDriversReplay(t *testing.T, want, got []byte) {
	t.Helper()
	if !reflect.DeepEqual(energyDriversReplayComparable(t, want), energyDriversReplayComparable(t, got)) {
		t.Fatal("driver values or metadata changed")
	}
	t.Log("All numeric tokens, hourly observations and metadata match exactly; only unordered source-ID lists and v1 reconciliation record order are ignored")
}

func energyDriversReplayComparable(t *testing.T, encoded []byte) any {
	value := purposeReplayComparableJSON(t, encoded)
	var visit func(any)
	visit = func(value any) {
		switch item := value.(type) {
		case map[string]any:
			for key, child := range item {
				if list, ok := child.([]any); ok {
					switch key {
					case "inputSourceIds":
						// Derived-source unions also originate in Go map iteration.
						for _, id := range list {
							if _, ok := id.(string); !ok {
								t.Fatal("invalid source ID")
							}
						}
						sort.Slice(list, func(i, j int) bool { return list[i].(string) < list[j].(string) })
					case "reconciliation":
						// The v1 builder iterates the carrier map. Compare these
						// records by ID, retaining duplicate counts and their
						// relative order as well as all quantities and fields.
						for _, row := range list {
							record, ok := row.(map[string]any)
							if !ok {
								t.Fatal("invalid reconciliation record")
							}
							id, ok := record["id"].(string)
							if !ok || id == "" {
								t.Fatal("missing reconciliation ID")
							}
						}
						sort.SliceStable(list, func(i, j int) bool {
							return list[i].(map[string]any)["id"].(string) < list[j].(map[string]any)["id"].(string)
						})
					}
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

func TestEnergyDriversComparisonPreservesObservations(t *testing.T) {
	want := energyDriversReplayComparable(t, []byte(`{"reconciliation":[{"id":"b","value":0.001},{"id":"a","value":0}],"inputSourceIds":["b","a"],"values":[1,2]}`))
	for _, test := range []struct {
		wire  string
		equal bool
	}{
		{`{"reconciliation":[{"id":"a","value":0},{"id":"b","value":0.001}],"inputSourceIds":["a","b"],"values":[1,2]}`, true},
		{`{"reconciliation":[{"id":"a","value":0},{"id":"b","value":0.002}],"inputSourceIds":["a","b"],"values":[1,2]}`, false},
		{`{"reconciliation":[{"id":"a","value":0},{"id":"b","value":0.001}],"inputSourceIds":["a"],"values":[1,2]}`, false},
		{`{"reconciliation":[{"id":"a","value":0},{"id":"b","value":0.001}],"inputSourceIds":["a","b"],"values":[2,1]}`, false},
	} {
		if reflect.DeepEqual(want, energyDriversReplayComparable(t, []byte(test.wire))) != test.equal {
			t.Fatal("replay comparison lost values, observations or source membership")
		}
	}
}
