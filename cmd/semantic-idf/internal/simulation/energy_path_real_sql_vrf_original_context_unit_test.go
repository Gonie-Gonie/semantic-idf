package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnergyPathRealSQLVRFOriginalContext(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "original.idf")
	data := []byte("Zone, Original Owner;\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	evidence := epathRealRunEvidence{CatalogDirectory: directory, OriginalInputPath: path, ModelSHA256: hex.EncodeToString(digest[:])}
	evidence.Fixture.ModelPath, evidence.Fixture.ModelSHA256 = "original.idf", evidence.ModelSHA256
	recipe := epathRealOracleRecipe{SQLModel: &epathRealSQLModel{NativeVRFSystems: []epathRealSQLVRFSystem{{}}}}
	observed := epathRealOracleEvidence{}
	if err := epathBindRealSQLVRFOriginal(evidence, recipe, &observed); err != nil || observed.originalText != string(data) {
		t.Fatalf("exact original bytes not bound: %v", err)
	}
	for name, mutate := range map[string]func(*epathRealRunEvidence){
		"different original path": func(e *epathRealRunEvidence) { e.OriginalInputPath = filepath.Join(directory, "executed.idf") },
		"different catalog path":  func(e *epathRealRunEvidence) { e.Fixture.ModelPath = "other.idf" },
		"missing catalog":         func(e *epathRealRunEvidence) { e.CatalogDirectory = "" },
		"missing hash":            func(e *epathRealRunEvidence) { e.ModelSHA256 = "" },
		"different catalog hash":  func(e *epathRealRunEvidence) { e.Fixture.ModelSHA256 = strings.Repeat("0", 64) },
		"altered original bytes": func(e *epathRealRunEvidence) {
			e.ModelSHA256 = strings.Repeat("0", 64)
			e.Fixture.ModelSHA256 = e.ModelSHA256
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := evidence
			mutate(&bad)
			unbound := epathRealOracleEvidence{}
			if err := epathBindRealSQLVRFOriginal(bad, recipe, &unbound); err == nil || unbound.originalText != "" {
				t.Fatalf("unproved original accepted: %v", err)
			}
		})
	}
	if err := epathBindRealSQLVRFOriginal(evidence, recipe, nil); err == nil {
		t.Fatal("nil evidence destination accepted")
	}
	if err := epathBindRealSQLVRFOriginal(epathRealRunEvidence{}, epathRealOracleRecipe{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLBindVRFFrames(epathRealOracleEvidence{}, epathRealSQLModel{}, nil); err == nil {
		t.Fatal("nil frame destination accepted")
	}
}
