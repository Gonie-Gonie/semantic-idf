package simulation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func epathSQLSimpleVentilationTransportUnit(t *testing.T) (epathRealRunEvidence, epathRealOracleEvidence, epathRealSQLModel, epathSQLFrames) {
	t.Helper()
	native, model, frames := epathSQLSimpleVentilationZeroServiceUnit(t)
	model.OriginalZoneMultiplierProof = epathSQLOriginalMultiplierContract
	catalog, runDir := t.TempDir(), t.TempDir()
	original, executed := filepath.Join(catalog, "VentilationSimpleTest.idf"), filepath.Join(runDir, "executed-model.idf")
	for file, text := range map[string]string{original: native.originalText, executed: native.executedText} {
		if err := os.WriteFile(file, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	evidence := epathRealRunEvidence{Version: "25.1", CatalogDirectory: catalog, RunDirectory: runDir, OriginalInputPath: original, InputPath: executed, ModelSHA256: epathRealHash([]byte(native.originalText)), ExecutedSHA256: epathRealHash([]byte(native.executedText))}
	evidence.Fixture = epathRealFixture{ID: "no-heating-ventilation-25-1", Version: "25.1", ModelPath: "VentilationSimpleTest.idf", ModelSHA256: evidence.ModelSHA256}
	return evidence, native, model, frames
}

// Same shared transport used by capture, saved diagnostics and reviewed replay.
// Neither the SQL reader nor the declaration invents the original model text.
func TestEnergyPathSQLSimpleVentilationExternalOriginalTransportReachesNativeBoundary(t *testing.T) {
	evidence, native, model, frames := epathSQLSimpleVentilationTransportUnit(t)
	observed, err := epathReadRealSQLOracle(native.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	observed.outputPlan = native.outputPlan
	if observed.originalText != "" || observed.executedText != "" {
		t.Fatal("SQL reader invented original input")
	}
	db, err := epathOpenOracleSQL(native.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := epathSQLValidateOriginalZoneMultipliers(db, observed.originalText, model.OriginalZoneMultiplierProof); err == nil {
		t.Fatal("missing original became a waiver")
	}
	if err := epathBindRealSQLVRFOriginal(evidence, epathRealOracleRecipe{SQLModel: &model}, &observed); err != nil {
		t.Fatal(err)
	}
	if observed.originalText != native.originalText || observed.executedText != native.executedText || observed.sqlPath != native.sqlPath || observed.outputPlan != native.outputPlan {
		t.Fatal("actual catalog/run bytes or native evidence changed")
	}
	if err := epathSQLValidateOriginalZoneMultipliers(db, observed.originalText, model.OriginalZoneMultiplierProof); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLSimpleVentilationZeroServices(frames, model, observed); err != nil {
		t.Fatal(err)
	}
	// Removing the recipe cohort cannot prevent external identity from loading
	// its actual documents; the existing finite native gate still rejects it.
	empty := epathRealSQLModel{}
	var bound epathRealOracleEvidence
	if err := epathBindRealSQLVRFOriginal(evidence, epathRealOracleRecipe{SQLModel: &empty}, &bound); err != nil || bound.originalText != native.originalText || bound.executedText != native.executedText {
		t.Fatalf("external fixture lost documents after declaration deletion: %v", err)
	}
	if err := epathSQLSimpleVentilationZeroServices(frames, empty, bound); err == nil {
		t.Fatal("empty declaration borrowed finite native permission")
	}
}

func TestEnergyPathSQLSimpleVentilationOriginalTransportRejectsUnboundDocuments(t *testing.T) {
	evidence, _, model, _ := epathSQLSimpleVentilationTransportUnit(t)
	for name, edit := range map[string]func(*epathRealRunEvidence){
		"missing original hash": func(e *epathRealRunEvidence) { e.ModelSHA256 = "" },
		"changed original digest": func(e *epathRealRunEvidence) {
			e.ModelSHA256 = strings.Repeat("0", 64)
			e.Fixture.ModelSHA256 = e.ModelSHA256
		},
		"catalog digest differs":            func(e *epathRealRunEvidence) { e.Fixture.ModelSHA256 = strings.Repeat("0", 64) },
		"foreign catalog path":              func(e *epathRealRunEvidence) { e.Fixture.ModelPath = "foreign.idf" },
		"missing executed digest":           func(e *epathRealRunEvidence) { e.ExecutedSHA256 = "" },
		"changed executed digest":           func(e *epathRealRunEvidence) { e.ExecutedSHA256 = strings.Repeat("0", 64) },
		"unbound executed directory":        func(e *epathRealRunEvidence) { e.RunDirectory = e.CatalogDirectory },
		"original substituted for executed": func(e *epathRealRunEvidence) { e.InputPath = e.OriginalInputPath },
	} {
		t.Run(name, func(t *testing.T) {
			bad := evidence
			edit(&bad)
			var observed epathRealOracleEvidence
			if err := epathBindRealSQLVRFOriginal(bad, epathRealOracleRecipe{SQLModel: &model}, &observed); err == nil {
				t.Fatal("unbound actual source accepted")
			}
			if observed.originalText != "" || observed.executedText != "" {
				t.Fatal("failed bind published partial documents")
			}
		})
	}
}

func TestEnergyPathSQLOriginalMultiplierDeclarationTransportsOriginalWithoutFixtureDispatch(t *testing.T) {
	evidence, native, _, _ := epathSQLSimpleVentilationTransportUnit(t)
	evidence.Fixture.ID, evidence.Fixture.Version = "literal-hand", ""
	// A generic declared original multiplier proof requires only original text.
	// Do not impose a new executed-input requirement on unrelated legacy models.
	evidence.InputPath, evidence.RunDirectory, evidence.ExecutedSHA256 = "", "", ""
	model := epathRealSQLModel{OriginalZoneMultiplierProof: epathSQLOriginalMultiplierContract}
	var bound epathRealOracleEvidence
	if err := epathBindRealSQLVRFOriginal(evidence, epathRealOracleRecipe{SQLModel: &model}, &bound); err != nil || bound.originalText != native.originalText || bound.executedText != "" {
		t.Fatalf("explicit original proof lost authoritative input: %v", err)
	}
	bad := evidence
	bad.ModelSHA256 = ""
	if err := epathBindRealSQLVRFOriginal(bad, epathRealOracleRecipe{SQLModel: &model}, &epathRealOracleEvidence{}); err == nil {
		t.Fatal("generic declaration bypassed original provenance")
	}
	// Explicit finite hand fan declarations request both documents even without
	// the external fixture ID. Missing executed evidence must fail.
	model = epathRealSQLModel{DirectHVACComponents: []epathRealSQLDirectHVACComponent{epathSQLSimpleVentilationUnitDeclaration()}}
	if err := epathBindRealSQLVRFOriginal(evidence, epathRealOracleRecipe{SQLModel: &model}, &epathRealOracleEvidence{}); err == nil {
		t.Fatal("finite hand fan declaration lost executed provenance")
	}
	for _, fixture := range []epathRealFixture{{}, {ID: "no-heating-ventilation-25-1", Version: "24.2"}, {ID: "no-heating-25-1", Version: "25.1"}} {
		var unchanged epathRealOracleEvidence
		if err := epathBindRealSQLVRFOriginal(epathRealRunEvidence{Fixture: fixture}, epathRealOracleRecipe{SQLModel: &epathRealSQLModel{}}, &unchanged); err != nil || unchanged.originalText != "" || unchanged.executedText != "" {
			t.Fatalf("unrelated empty model gained finite fixture requirements: %v", err)
		}
	}
}
