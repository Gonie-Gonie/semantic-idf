package simulation

import (
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func addEnergyFloorAreaFixture(t *testing.T, path, values string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE Zones (ZoneName TEXT, FloorArea REAL, Multiplier REAL, ListMultiplier REAL, IsPartOfTotalArea INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO Zones VALUES ` + values); err != nil {
		t.Fatal(err)
	}
}

func TestEnergyFloorAreasSQLUsesEffectiveAreasAndExcludesNonTotalZones(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	addEnergyFloorAreaFixture(t, path, `('Office', 100.25, 2, 3, 1), ('Lab', 40, 1, 1, 1), ('Plenum', 500, 2, 1, 0)`)
	areas, available := readEnergyFloorAreasSQL(path)
	if !available || areas.building != 641.5 || areas.zones["office"] != 601.5 || areas.zones["plenum"] != 1000 {
		t.Fatalf("effective floor areas = %#v, available %v", areas, available)
	}
	// SQL geometry is authoritative even when a caller's input has a different area.
	doc := parsePurposePlanFixture(t, "Zone,Office,0,0,0,0,1,1,3,300,999;")
	got := energyFloorAreasForRun(&SimulationRunResult{Files: []SimulationFileInfo{{Kind: "sqlite", Path: path}}}, &doc)
	if got.building != areas.building {
		t.Fatalf("input area overrode executed SQL geometry: %#v", got)
	}
}

func TestEnergyFloorAreasSQLDoesNotInventMissingDenominators(t *testing.T) {
	for _, test := range []struct {
		name, values string
		wantBuilding float64
	}{
		{"missing included area", `('Office', 100, 1, 1, 1), ('Missing', NULL, 1, 1, 1)`, 0},
		{"invalid multiplier", `('Office', 100, 1, 1, 1), ('Missing', 10, 0, 1, 1)`, 0},
		{"unknown inclusion", `('Office', 100, 1, 1, 1), ('Missing', 10, 1, 1, NULL)`, 0},
		{"excluded missing area", `('Office', 100, 1, 1, 1), ('Missing', NULL, 1, 1, 0)`, 100},
		{"known zero floor", `('Office', 100, 1, 1, 1), ('Empty', 0, 1, 1, 1)`, 100},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "eplusout.sql")
			addEnergyFloorAreaFixture(t, path, test.values)
			areas, available := readEnergyFloorAreasSQL(path)
			if !available || areas.building != test.wantBuilding || areas.zones["office"] != 100 {
				t.Fatalf("areas = %#v, available %v", areas, available)
			}
		})
	}
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if normalized := normalizeEnergyExplanationScope(EnergyExplanationScope{FloorAreaM2: value}); normalized.FloorAreaM2 != 0 {
			t.Fatalf("invalid area %v survived scope normalization", value)
		}
	}
}

func TestEnergyFloorAreasInputFallbackUsesDeclaredAreaAndZoneGroup(t *testing.T) {
	doc := parsePurposePlanFixture(t, `
Version,24.2;
Zone,Office,0,0,0,0,1,2,3,300,100,,,Yes;
Zone,Lab,0,0,0,0,1,1,3,120,40,,,Yes;
Zone,Plenum,0,0,0,0,1,1,3,1500,500,,,No;
ZoneList,Repeated,Office;
ZoneGroup,Repeated Offices,Repeated,3;
`)
	areas := energyFloorAreasForRun(&SimulationRunResult{}, &doc)
	if areas.building != 640 || areas.zones["office"] != 600 || areas.zones["lab"] != 40 || areas.zones["plenum"] != 500 {
		t.Fatalf("input fallback areas = %#v", areas)
	}
	unknown := parsePurposePlanFixture(t, "Zone,Unknown;")
	if got := energyFloorAreasForRun(&SimulationRunResult{}, &unknown); got.building != 0 || got.zones["unknown"] != 0 {
		t.Fatalf("unknown geometry produced an area: %#v", got)
	}
}

func TestEnergyFloorAreasBundleCarriesRunAreaAcrossScopesAndJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	createEnergyPathContractSQL(t, path)
	addEnergyFloorAreaFixture(t, path, `('Office', 100.25, 2, 3, 1), ('Lab', 40, 1, 1, 1), ('Plenum', 500, 2, 1, 0)`)
	input := filepath.Join(dir, "input.idf")
	if err := os.WriteFile(input, []byte(energyPathScopeFixtureIDF), 0600); err != nil {
		t.Fatal(err)
	}
	result := &SimulationRunResult{InputPath: input, Files: []SimulationFileInfo{{Name: "eplusout.sql", Kind: "sqlite", Path: path}}}
	bundle := BuildPurposeResultBundle(result, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	assertEnergyBundleFloorAreas(t, bundle)
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PurposeResultBundle
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	assertEnergyBundleFloorAreas(t, decoded)
	if data, err := json.Marshal(EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}); err != nil || strings.Contains(string(data), "floorAreaM2") {
		t.Fatalf("unknown area was serialized: %s (%v)", data, err)
	}
}

func assertEnergyBundleFloorAreas(t *testing.T, bundle PurposeResultBundle) {
	t.Helper()
	explanation := bundle.EnergyExplanation
	if explanation.Scope.FloorAreaM2 != 641.5 || bundle.EnergyExplanationSummary.Scope.FloorAreaM2 != 641.5 {
		t.Fatalf("building scopes lost floor area: %#v / %#v", explanation.Scope, bundle.EnergyExplanationSummary.Scope)
	}
	for _, period := range explanation.Periods {
		if period.Summary != nil && period.Summary.Scope.FloorAreaM2 != 641.5 {
			t.Fatalf("building period %s lost floor area: %#v", period.ID, period.Summary.Scope)
		}
	}
	found := false
	for _, zone := range explanation.ZoneResults {
		if !strings.EqualFold(zone.Scope.ZoneName, "Office") {
			continue
		}
		found = true
		if zone.Scope.FloorAreaM2 != 601.5 || zone.Summary.Scope.FloorAreaM2 != 601.5 {
			t.Fatalf("Office scopes lost floor area: %#v / %#v", zone.Scope, zone.Summary.Scope)
		}
		for _, period := range zone.Periods {
			if period.Summary != nil && period.Summary.Scope.FloorAreaM2 != 601.5 {
				t.Fatalf("Office period %s lost floor area: %#v", period.ID, period.Summary.Scope)
			}
		}
	}
	if !found {
		t.Fatal("bundle did not include Office scope")
	}
}
