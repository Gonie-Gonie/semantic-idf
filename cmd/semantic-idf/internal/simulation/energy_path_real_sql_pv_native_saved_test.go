package simulation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Read-only native electrical diagnostic. No candidate scalar, expected
// manifest, runtime rebuild, engine run or acceptance artifact participates.
// The separate all-eight-group consumer review remains mandatory afterward.
func TestEnergyPathRealSQLPVNativeSavedCapture(t *testing.T) {
	directory := strings.TrimSpace(os.Getenv("EPATH_REAL_PV_NATIVE_DIR"))
	if directory == "" {
		t.Skip("explicit saved PV capture required for native electrical diagnostic")
	}
	if os.Getenv("EPATH_REAL_RUN") == "1" || os.Getenv("EPATH_REAL_CAPTURE") == "1" {
		t.Fatal("native saved diagnostic cannot start the engine or approve a fixture")
	}
	root, catalog := epathRealDirectories(t)
	var evidence epathRealRunEvidence
	// Only the Bundle's schema is used by the existing capture-integrity gate.
	// Intercept its quantities to avoid production source/graph JSON repair.
	type plainEvidence epathRealRunEvidence
	wire := struct {
		*plainEvidence
		Bundle struct {
			EnergyExplanation struct {
				Schema string `json:"schema"`
			} `json:"energyExplanation"`
		} `json:"bundle"`
	}{plainEvidence: (*plainEvidence)(&evidence)}
	file, err := os.Open(filepath.Join(directory, "run-evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewDecoder(file).Decode(&wire)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	evidence.Bundle.EnergyExplanation.Schema = wire.Bundle.EnergyExplanation.Schema
	if !epathRealSamePath(directory, evidence.RunDirectory) || evidence.Fixture.ID != "pv-storage-25-1" || evidence.Version != "25.1" {
		t.Fatal("native electrical diagnostic requires the exact saved Shop25.1 capture")
	}
	if err := epathValidateSavedRealEvidence(root, catalog, evidence); err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(evidence.SQLPath)
	if err != nil {
		t.Fatal(err)
	}
	model := epathRealSQLModel{PVSystems: []epathRealSQLPVSystem{epathSQLPVOriginalDeclaration()}, Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}}
	observed.outputPlan = evidence.Run.PurposeRunPlan
	if err := epathBindRealSQLVRFOriginal(evidence, epathRealOracleRecipe{SQLModel: &model}, &observed); err != nil {
		t.Fatal(err)
	}
	var frames epathSQLFrames
	if err := epathSQLBindPVSources(observed, model, &frames); err != nil {
		t.Fatal(err)
	}
	core := frames.PVSystems[0]
	cg, err := epathCompileSQLPVCogenerationFrames(observed, core, filepath.Join(directory, "eplusout.mtd"))
	if err != nil {
		t.Fatal(err)
	}
	required := &epathRealSQLPVCogeneration{SystemID: model.PVSystems[0].ID, Boundary: "shop-25.1-native-cogeneration", MTDFile: "eplusout.mtd"}
	if err := epathSQLValidatePVCogenerationExternalEvidence(required, model.PVSystems, observed, core, cg); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLValidatePVNativeBalances(core, cg); err != nil {
		t.Fatal(err)
	}
	for _, id := range epathSQLPVSourceIDs(core) {
		s := core.Sources[id]
		t.Logf("NATIVE %s/%s id=%d rows=%d kWh=%.12f negativeCell=%t", s.Spec.ID, s.Dictionary.Frequency, id, len(s.Rows), s.NativeAnnualKWh, s.HasNegativeValue)
	}
	for _, frequency := range []string{"Monthly", "Hourly"} {
		s := cg.Parents[frequency]
		t.Logf("NATIVE Cogeneration/%s id=%d rows=%d kWh=%.12f", frequency, s.Dictionary.Index, len(s.Rows), s.NativeAnnualKWh)
	}
	if err := epathValidateSavedRealEvidence(root, catalog, evidence); err != nil {
		t.Fatal(err)
	}
	if hash, err := epathSQLPVFileSHA(cg.MTDPath); err != nil || hash != cg.Membership.MTDSHA256 {
		t.Fatalf("native MTD changed during read-only diagnostic: %v", err)
	}
	t.Logf("NATIVE ONLY, NOT ACCEPTANCE: core40 + parent2; nine equations at12months/8760hours and annual sum; SQL=%s MTD=%s", core.SQLSHA256, cg.Membership.MTDSHA256)
}
