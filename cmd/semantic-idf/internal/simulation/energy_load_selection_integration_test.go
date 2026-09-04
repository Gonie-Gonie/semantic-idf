package simulation

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestEPATH070OutputPlanRequestsCanonicalAndContextLoadFamilies(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})

	for _, target := range []struct {
		key      string
		zoneName string
		variable string
	}{
		{key: "Office", zoneName: "Office", variable: "Zone Air System Sensible Cooling Energy"},
		{key: "Office", zoneName: "Office", variable: "Zone Air System Latent Cooling Energy"},
		{key: "Office", zoneName: "Office", variable: "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate"},
		{key: "Office", zoneName: "Office", variable: "Zone System Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate"},
		{key: "Office", zoneName: "Office", variable: "Zone Baseboard Total Heating Energy"},
		{key: "Office Ideal Loads", zoneName: "Office", variable: "Zone Ideal Loads Zone Total Cooling Energy"},
		{key: "Office Ideal Loads", zoneName: "Office", variable: "Zone Ideal Loads Zone Total Cooling Rate"},
		{key: "Office Ideal Loads", zoneName: "Office", variable: "Zone Ideal Loads Zone Sensible Cooling Energy"},
		{key: "Office Ideal Loads", zoneName: "Office", variable: "Zone Ideal Loads Zone Latent Cooling Energy"},
		{key: "Lab Ideal Loads", zoneName: "Lab", variable: "Zone Ideal Loads Zone Total Heating Energy"},
	} {
		output := findPurposeOutput(plan, "Output:Variable", target.key, target.variable)
		if output == nil || output.ReportingFrequency != "Monthly" || output.ScopeZoneName != target.zoneName {
			t.Errorf("EPATH-070 output target %s / %s = %#v", target.key, target.variable, output)
		}
	}
	if findPurposeOutput(plan, "Output:Variable", "Office", "Zone Ideal Loads Zone Total Cooling Energy") != nil {
		t.Fatal("Ideal Loads object-scoped load was incorrectly requested with the owning zone key")
	}
}

func TestEPATH070SQLCanonicalLoadSelectionAndRawBreakdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createTestEnergySQL(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES
		(23, 'Office', 'Zone Air System Sensible Cooling Energy', 'J'),
		(24, 'Office', 'Zone Air System Latent Cooling Energy', 'J'),
		(25, 'Office', 'Zone Ideal Loads Zone Total Cooling Energy', 'J'),
		(26, 'Office', 'Zone Radiant HVAC Cooling Energy', 'J'),
		(27, 'Cooling Coil A', 'Cooling Coil Total Cooling Energy', 'J'),
		(28, 'Cooling Loop', 'Plant Loop Cooling Demand Energy', 'J'),
		(29, 'Office', 'Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate', 'W')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportData VALUES
		(7, 1, 23, 28800000.0),
		(8, 1, 24, 7200000.0),
		(9, 1, 25, 180000000.0),
		(10, 1, 26, 108000000.0),
		(11, 1, 27, 360000000.0),
		(12, 1, 28, 720000000.0),
		(13, 1, 29, -15.0)`); err != nil {
		t.Fatal(err)
	}

	variables := []string{
		"Zone Air System Sensible Cooling Energy",
		"Zone Air System Latent Cooling Energy",
		"Zone Ideal Loads Zone Total Cooling Energy",
		"Zone Radiant HVAC Cooling Energy",
		"Cooling Coil Total Cooling Energy",
		"Plant Loop Cooling Demand Energy",
		"Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate",
	}
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
	for _, variable := range variables {
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{
			ObjectType: "Output:Variable", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, KeyValue: "*", VariableName: variable,
		})
	}
	context := energyDriverBuildContext{Enabled: true, Multipliers: unitEnergyMultiplierIndex("Office")}
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, plan, context)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	load := energyPathV2NodeByID(result.Nodes, "load.cooling.building")
	if load == nil || load.Value != 10 || load.ThermalComponent != "combined" {
		t.Fatalf("SQL canonical cooling load = %#v, want sensible 8 + latent 2", load)
	}
	for _, sourceID := range []string{"sql-rdd-23", "sql-rdd-24"} {
		if !stringSliceContains(load.SourceIDs, sourceID) {
			t.Errorf("canonical load lost raw component %s: %#v", sourceID, load.SourceIDs)
		}
		source := energyExplanationSourceByID(result.Sources, sourceID)
		if source == nil || source.DriverRole != energyDriverSourceRoleMainFlow || source.InspectorSection != energyDriverInspectorSectionBreakdown {
			t.Errorf("selected SQL component metadata %s = %#v", sourceID, source)
		}
	}
	for _, sourceID := range []string{"sql-rdd-25", "sql-rdd-26", "sql-rdd-27", "sql-rdd-28", "sql-rdd-29"} {
		if stringSliceContains(load.SourceIDs, sourceID) {
			t.Errorf("lower/predicted SQL source entered canonical load: %s / %#v", sourceID, load.SourceIDs)
		}
		source := energyExplanationSourceByID(result.Sources, sourceID)
		if source == nil || source.DriverRole != energyDriverSourceRoleContext {
			t.Errorf("lower/predicted SQL source was not retained as context: %s / %#v", sourceID, source)
		}
	}
}
