package simulation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnergyPathRealSavedSnapshotModeRequiresExplicitAcceptance(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
		ok   bool
	}{
		{"ordinary test skip", nil, true},
		{"preserved capture", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture"}, true},
		{"explicit rebuild", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture", "EPATH_REAL_VERIFY_REBUILD": "1"}, true},
		{"explicit acceptance", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture", "EPATH_REAL_VERIFY_ACCEPTANCE": "1"}, true},
		{"snapshot acceptance", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture", "EPATH_REAL_VERIFY_SNAPSHOT": "snapshot.json", "EPATH_REAL_VERIFY_ACCEPTANCE": "1"}, true},
		{"snapshot alone", map[string]string{"EPATH_REAL_VERIFY_SNAPSHOT": "snapshot.json"}, false},
		{"snapshot without capture", map[string]string{"EPATH_REAL_VERIFY_SNAPSHOT": "snapshot.json", "EPATH_REAL_VERIFY_ACCEPTANCE": "1"}, false},
		{"snapshot not acceptance", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture", "EPATH_REAL_VERIFY_SNAPSHOT": "snapshot.json"}, false},
		{"nonexact opt in", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture", "EPATH_REAL_VERIFY_SNAPSHOT": "snapshot.json", "EPATH_REAL_VERIFY_ACCEPTANCE": "true"}, false},
		{"snapshot and rebuild", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture", "EPATH_REAL_VERIFY_SNAPSHOT": "snapshot.json", "EPATH_REAL_VERIFY_ACCEPTANCE": "1", "EPATH_REAL_VERIFY_REBUILD": "1"}, false},
		{"snapshot and engine", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture", "EPATH_REAL_VERIFY_SNAPSHOT": "snapshot.json", "EPATH_REAL_VERIFY_ACCEPTANCE": "1", "EPATH_REAL_RUN": "1"}, false},
		{"snapshot and capture", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture", "EPATH_REAL_VERIFY_SNAPSHOT": "snapshot.json", "EPATH_REAL_VERIFY_ACCEPTANCE": "1", "EPATH_REAL_CAPTURE": "1"}, false},
		{"legacy verification and engine", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture", "EPATH_REAL_RUN": "1"}, false},
		{"legacy verification and capture", map[string]string{"EPATH_REAL_VERIFY_DIR": "capture", "EPATH_REAL_CAPTURE": "1"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			mode, err := epathRealSavedVerificationOptions(func(key string) string { return test.env[key] })
			if (err == nil) != test.ok {
				t.Fatalf("mode=%+v, error=%v", mode, err)
			}
			if test.ok && mode.Snapshot != test.env["EPATH_REAL_VERIFY_SNAPSHOT"] {
				t.Fatal("explicit snapshot selection was discarded")
			}
		})
	}
}

// These files are synthetic, isolated reader inputs, not approved acceptance
// evidence. No engine/SQL/expected manifest is created or opened by this test.
func TestEnergyPathRealSavedSnapshotUsesOriginalWireAndIntegrityGuards(t *testing.T) {
	for _, mutation := range []string{"", "candidate changed", "capture changed", "production changed", "SQL binding changed", "executed input changed", "engine changed", "weather changed", "missing sidecar", "bound invalid wire", "bound invalid graph"} {
		t.Run(mutation, func(t *testing.T) {
			root := t.TempDir()
			write := func(path string, data []byte) {
				t.Helper()
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			capturePath, productionPath := filepath.Join(root, "run-evidence.json"), filepath.Join(root, "source.go")
			write(capturePath, []byte("unchanged original capture"))
			write(productionPath, []byte("package synthetic\n"))
			evidence := epathRealRunEvidence{
				RunDirectory: root, SQLPath: filepath.Join(root, "does-not-exist.sql"),
				SQLSHA256: strings.Repeat("a", 64), ExecutedSHA256: strings.Repeat("b", 64), EngineSHA256: strings.Repeat("c", 64), WeatherSHA256: strings.Repeat("d", 64),
				Bundle: PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Nodes: []EnergyExplanationNode{{ID: "original-capture-not-snapshot", Value: 99}}}},
			}
			wire := epathOracleWireFixture()
			graph := wire["energyExplanation"].(map[string]any)
			graph["scope"] = map[string]any{"kind": "building"}
			graph["nodes"] = []any{map[string]any{"id": "snapshot-value", "value": 1.23456789, "rawValue": 0}}
			if mutation == "bound invalid wire" {
				graph["nodes"].([]any)[0].(map[string]any)["value"] = nil
			}
			if mutation == "bound invalid graph" {
				graph["links"] = []any{map[string]any{"id": "unrepaired-invalid-link", "fromId": "snapshot-value", "toId": "missing", "fromValue": -2, "toValue": 4, "fromUnit": "J", "toUnit": "kWh", "ratio": 999}}
			}
			data, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "candidate.json")
			write(path, data)
			provenance, err := epathOracleSnapshotProvenanceFor(root, evidence)
			if err != nil {
				t.Fatal(err)
			}
			provenance.CandidateSHA256 = epathRealHash(data)
			sidecar, err := json.Marshal(provenance)
			if err != nil {
				t.Fatal(err)
			}
			if mutation != "missing sidecar" {
				write(path+".provenance.json", sidecar)
			}
			switch mutation {
			case "candidate changed":
				write(path, append(data, '\n'))
			case "capture changed":
				write(capturePath, []byte("changed capture"))
			case "production changed":
				write(productionPath, []byte("package changed\n"))
			case "SQL binding changed":
				evidence.SQLSHA256 = strings.Repeat("e", 64)
			case "executed input changed":
				evidence.ExecutedSHA256 = strings.Repeat("e", 64)
			case "engine changed":
				evidence.EngineSHA256 = strings.Repeat("e", 64)
			case "weather changed":
				evidence.WeatherSHA256 = strings.Repeat("e", 64)
			}
			before := evidence.Bundle
			mode := epathRealSavedVerificationMode{Directory: root, Snapshot: path, Acceptance: true}
			bundle, err := mode.bundle(root, evidence)
			wantSuccess := mutation == "" || mutation == "bound invalid graph"
			if (err == nil) != wantSuccess {
				t.Fatalf("snapshot mutation %q: %v", mutation, err)
			}
			if !reflect.DeepEqual(evidence.Bundle, before) {
				t.Fatal("reader modified the preserved capture bundle")
			}
			if !wantSuccess {
				return
			}
			node := bundle.EnergyExplanation.Nodes[0]
			if node.ID != "snapshot-value" || node.Value != 1.23456789 || !node.inspectorDecodedFromJSON || node.inspectorValuePresence&1 == 0 || node.inspectorValuePresence&2 != 0 {
				t.Fatalf("snapshot was rebuilt/normalized or lost known0/unknown distinction: %+v", node)
			}
			if mutation == "bound invalid graph" {
				link := bundle.EnergyExplanation.Links[0]
				if link.ToID != "missing" || link.FromValue != -2 || link.Ratio != 999 {
					t.Fatal("original invalid graph was silently repaired")
				}
				if err := epathValidateOracleGraphRecords(bundle.EnergyExplanation.Nodes, bundle.EnergyExplanation.Links, nil, "building", "", "annual"); err == nil {
					t.Fatal("signed snapshot integrity was mistaken for graph acceptance")
				}
			}
			for file, want := range map[string]string{path: provenance.CandidateSHA256, path + ".provenance.json": epathRealHash(sidecar), capturePath: provenance.CaptureSHA256} {
				if got, err := epathOracleHashFile(file); err != nil || got != want {
					t.Fatalf("reader changed %s: %v", file, err)
				}
			}
		})
	}
}

func TestEnergyPathRealSavedSnapshotCannotBypassModeOrReplaceLegacyCapture(t *testing.T) {
	evidence := epathRealRunEvidence{Bundle: PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: "original-capture"}}}
	for _, mode := range []epathRealSavedVerificationMode{
		{Snapshot: "missing.json", Acceptance: true},
		{Directory: "capture", Snapshot: "missing.json"},
		{Directory: "capture", Snapshot: "missing.json", Acceptance: true, Rebuild: true},
	} {
		if _, err := mode.bundle(t.TempDir(), evidence); err == nil || !strings.Contains(err.Error(), "explicit saved acceptance") {
			t.Fatalf("invalid mode reached snapshot I/O or rebuild: %v", err)
		}
	}
	bundle, err := (epathRealSavedVerificationMode{}).bundle(t.TempDir(), evidence)
	if err != nil || !reflect.DeepEqual(bundle, evidence.Bundle) {
		t.Fatalf("ordinary saved replay stopped preserving its captured bundle: %v", err)
	}
}
