package simulation

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEnergyPathRealSQLDirectHVACOriginalContext(t *testing.T) {
	_, directory := epathRealDirectories(t)
	catalog := epathLoadRealCatalog(t, directory)
	var fixture epathRealFixture
	for _, candidate := range catalog.Fixtures {
		if candidate.ID == "ptac-25-1" {
			fixture = candidate
		}
	}
	if fixture.ID == "" {
		t.Fatal("reviewed original PTAC fixture missing")
	}
	recipe, err := epathLoadRealOracleRecipe(filepath.Join(directory, filepath.FromSlash(fixture.OraclePath)))
	if err != nil {
		t.Fatal(err)
	}
	evidence := epathRealRunEvidence{Fixture: fixture, CatalogDirectory: directory, ModelSHA256: fixture.ModelSHA256, OriginalInputPath: filepath.Join(directory, filepath.FromSlash(fixture.ModelPath))}
	if err := epathValidateRealSQLDirectHVACOriginal(evidence, recipe); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*epathRealRunEvidence){
		func(e *epathRealRunEvidence) { e.ModelSHA256 = "" },
		func(e *epathRealRunEvidence) { e.ModelSHA256 = strings.Repeat("0", 64) },
		func(e *epathRealRunEvidence) {
			e.ModelSHA256, e.Fixture.ModelSHA256 = strings.Repeat("0", 64), strings.Repeat("0", 64)
		},
		func(e *epathRealRunEvidence) { e.OriginalInputPath = filepath.Join(directory, "annual-model.idf") },
		func(e *epathRealRunEvidence) { e.CatalogDirectory = "" },
		func(e *epathRealRunEvidence) { e.Fixture.ModelPath = "" },
	} {
		bad := evidence
		mutate(&bad)
		if err := epathValidateRealSQLDirectHVACOriginal(bad, recipe); err == nil {
			t.Fatal("unbound original model was accepted")
		}
	}
	if err := epathValidateRealSQLDirectHVACOriginal(epathRealRunEvidence{}, epathRealOracleRecipe{}); err != nil {
		t.Fatal("unrelated non-direct recipe acquired an original ownership requirement")
	}
}
