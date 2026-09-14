package simulation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnergyPathRealSQLBaseboardExecutedOriginalBinding(t *testing.T) {
	catalog := t.TempDir()
	runDir := t.TempDir()
	original := filepath.Join(catalog, "original.idf")
	executed := filepath.Join(runDir, "executed-model.idf")
	originalText := "Version,25.1;\nRunPeriod,Original;\nZone,A;"
	executedText := "Version,25.1;\nZone,A;\nOutput:SQLite,SimpleAndTabular;"
	if err := os.WriteFile(original, []byte(originalText), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executed, []byte(executedText), 0600); err != nil {
		t.Fatal(err)
	}
	evidence := epathRealRunEvidence{CatalogDirectory: catalog, RunDirectory: runDir, OriginalInputPath: original, InputPath: executed, ModelSHA256: epathRealHash([]byte(originalText)), ExecutedSHA256: epathRealHash([]byte(executedText))}
	evidence.Fixture.ModelPath = "original.idf"
	evidence.Fixture.ModelSHA256 = evidence.ModelSHA256
	recipe := epathRealOracleRecipe{SQLModel: &epathRealSQLModel{BaseboardContexts: []epathRealSQLBaseboardContext{{ZoneName: "A"}}}}
	var observed epathRealOracleEvidence
	if err := epathBindRealSQLVRFOriginal(evidence, recipe, &observed); err != nil {
		t.Fatal(err)
	}
	if observed.originalText != originalText || observed.executedText != executedText {
		t.Fatal("original ownership and executed index documents were conflated")
	}
	for name, mutate := range map[string]func(*epathRealRunEvidence){
		"missing executed hash":     func(e *epathRealRunEvidence) { e.ExecutedSHA256 = "" },
		"changed executed hash":     func(e *epathRealRunEvidence) { e.ExecutedSHA256 = strings.Repeat("0", 64) },
		"original used as executed": func(e *epathRealRunEvidence) { e.InputPath = e.OriginalInputPath },
		"unbound run directory":     func(e *epathRealRunEvidence) { e.RunDirectory = catalog },
		"missing original hash":     func(e *epathRealRunEvidence) { e.ModelSHA256 = "" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := evidence
			mutate(&bad)
			var fresh epathRealOracleEvidence
			if err := epathBindRealSQLVRFOriginal(bad, recipe, &fresh); err == nil {
				t.Fatal("unbound source document accepted")
			}
			if fresh.originalText != "" || fresh.executedText != "" {
				t.Fatal("partial document escaped a failed bind")
			}
		})
	}
}
