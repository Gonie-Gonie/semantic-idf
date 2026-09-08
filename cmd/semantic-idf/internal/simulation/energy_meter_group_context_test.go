package simulation

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnergyMeterGroupContextClassification(t *testing.T) {
	for _, name := range []string{"Electricity:Building", "Electricity:HVAC", "NaturalGas:Plant", " DistrictCooling : HVAC ", "Gas:Building"} {
		t.Run(name, func(t *testing.T) {
			def, ok := energyMeterAliasOrOtherDefinitionForName(name)
			if !ok || def.HierarchyLevel != "meter_group" || def.EndUse == "other" || def.FacilityTotal {
				t.Fatalf("meter group must be a non-additive subtotal, not a new end use: %#v", def)
			}
			item := canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "energy", Kind: def.Kind, EndUse: def.EndUse, Carrier: def.Carrier, MeterHierarchyLevel: def.HierarchyLevel})
			if item.Stage != "context" || energyExplanationMultiplierRequirement(item) != energyMultiplierAlreadyModelTotal {
				t.Fatalf("subtotal must retain model-total context: %#v", item)
			}
		})
	}
	for _, name := range []string{"SomeCustomMeter:Electricity", "Electricity:BuildingExtension", "Building:Electricity"} {
		def, ok := energyMeterAliasOrOtherDefinitionForName(name)
		if !ok || def.EndUse != "other" || def.HierarchyLevel != "broad_end_use" {
			t.Fatalf("an arbitrary custom name must not become a native group: %s %#v", name, def)
		}
	}
	for _, name := range []string{"Electricity:Facility", "Cooling:Electricity", "InteriorLights:Electricity"} {
		def, ok := energyMeterAliasOrOtherDefinitionForName(name)
		if !ok || def.HierarchyLevel == "meter_group" {
			t.Fatalf("existing physical meter classification changed: %s %#v", name, def)
		}
	}
}

func TestEnergyMeterGroupContextSQLDoesNotDoubleCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "group-meters.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,KeyValue TEXT,Name TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT)`,
		`CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	names := []string{"Electricity:Facility", "Electricity:Building", "Electricity:HVAC", "Electricity:Plant", "InteriorLights:Electricity", "InteriorEquipment:Electricity", "Cooling:Electricity"}
	monthly := []float64{10, 7, 3, 0, 5, 2, 3}
	for index, name := range names {
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(?, '', ?, 'J', 1, 'Monthly', 'Meter')`, index+1, name); err != nil {
			t.Fatal(err)
		}
	}
	for month := 1; month <= 12; month++ {
		if _, err := db.Exec(`INSERT INTO Time VALUES(?, ?, 1, 1, 0)`, month, month); err != nil {
			t.Fatal(err)
		}
		for index, value := range monthly {
			if _, err := db.Exec(`INSERT INTO ReportData VALUES(?, ?, ?, ?)`, (month-1)*len(names)+index+1, month, index+1, value*3600000); err != nil {
				t.Fatal(err)
			}
		}
	}
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
	context := energyDriverBuildContext{Enabled: true}
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, context)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Series) != 7 || len(parsed.Sources) != 7 {
		t.Fatal("subtotal observations were discarded")
	}
	result := UpgradeEnergyExplanationV1(buildEnergyExplanationResultWithDriverContext(parsed.Series, parsed.Sources, plan, context))
	for _, period := range result.Periods {
		want := 10.0
		if period.ID == "annual" {
			want *= 12
		}
		carrier, endUse := 0.0, 0.0
		for _, node := range period.Nodes {
			if node.Level == "carrier" {
				carrier += node.Value
			}
			if node.Level == "end_use" {
				if node.EndUse == "other" {
					t.Fatalf("%s invented an Other branch from a group subtotal", period.ID)
				}
				endUse += node.Value
			}
			for _, id := range node.SourceIDs {
				if id == "sql-rdd-2" || id == "sql-rdd-3" || id == "sql-rdd-4" {
					t.Fatal("non-additive group source entered physical flow")
				}
			}
		}
		if carrier != want || endUse != want {
			t.Fatalf("%s double counted subtotals: carrier=%v endUse=%v want=%v", period.ID, carrier, endUse, want)
		}
	}
	for index := 1; index <= 3; index++ {
		id, found := fmt.Sprintf("sql-rdd-%d", index+1), false
		for _, source := range result.Sources {
			if source.ID != id {
				continue
			}
			found = true
			if source.Name != names[index] || source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" || source.InspectorSection != "context" || source.RawValue != 12*monthly[index] || source.EffectiveValue != 12*monthly[index] || !strings.Contains(source.Explanation, "subtotal") {
				t.Fatalf("group source evidence was lost, including observed zero: %#v", source)
			}
		}
		if !found {
			t.Fatalf("original source %s disappeared", id)
		}
	}
}
