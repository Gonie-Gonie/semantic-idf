package simulation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnergyPathSQLHeatOnlyExternalFixtureRequiresDocumentsBeforeDeclaration(t *testing.T) {
	originalText, _ := epathSQLHeatOnlyOriginalFixture(t)
	executedText := "Output:SQLite,SimpleAndTabular;\n" + originalText
	catalog, runDir := t.TempDir(), t.TempDir()
	original, executed := filepath.Join(catalog, "Furnace.idf"), filepath.Join(runDir, "executed-model.idf")
	for path, text := range map[string]string{original: originalText, executed: executedText} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	evidence := epathRealRunEvidence{
		Version: "25.1", CatalogDirectory: catalog, RunDirectory: runDir,
		OriginalInputPath: original, InputPath: executed,
		ModelSHA256: epathRealHash([]byte(originalText)), ExecutedSHA256: epathRealHash([]byte(executedText)),
	}
	evidence.Fixture = epathRealFixture{ID: "no-cooling-25-1", Version: "25.1", ModelPath: "Furnace.idf", ModelSHA256: evidence.ModelSHA256}
	recipe := epathRealOracleRecipe{SQLModel: &epathRealSQLModel{}}
	var observed epathRealOracleEvidence
	if err := epathBindRealSQLVRFOriginal(evidence, recipe, &observed); err != nil {
		t.Fatal(err)
	}
	if observed.originalText != originalText || observed.executedText != executedText || observed.sqlPath != "" {
		t.Fatal("external Furnace identity did not bind both actual documents without SQL")
	}
	if err := epathSQLBindHeatOnlyChecks(observed, *recipe.SQLModel, &epathSQLModelChecks{}); err == nil || !strings.Contains(err.Error(), "mandatory independent declaration") {
		t.Fatalf("deleted HeatOnly declaration escaped actual original detection: %v", err)
	}
	// Explicit hand declarations still request both documents without requiring
	// a real catalog fixture ID; this test does not assert their native proof.
	hand := evidence
	hand.Fixture.ID, hand.Fixture.Version = "hand", ""
	declared := epathRealOracleRecipe{SQLModel: &epathRealSQLModel{HeatOnlyFurnaces: []epathRealSQLHeatOnlyFurnace{{}}}}
	var bound epathRealOracleEvidence
	if err := epathBindRealSQLVRFOriginal(hand, declared, &bound); err != nil || bound.originalText != originalText || bound.executedText != executedText {
		t.Fatalf("explicit hand declaration lost original/executed transport: %v", err)
	}
	// Neither a partial fixture-name match nor an unrelated empty hand model
	// gains new document requirements.
	for _, fixture := range []epathRealFixture{{}, {ID: "no-cooling-25-1", Version: "24.2"}, {ID: "hand", Version: "25.1"}} {
		var untouched epathRealOracleEvidence
		if err := epathBindRealSQLVRFOriginal(epathRealRunEvidence{Fixture: fixture}, recipe, &untouched); err != nil || untouched.originalText != "" || untouched.executedText != "" {
			t.Fatalf("unrelated empty hand model gained document requirements: %v", err)
		}
	}
}
