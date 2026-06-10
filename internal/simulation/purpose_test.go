package simulation

import (
	"strings"
	"testing"

	"github.com/Gonie-Gonie/idf-analyzer/internal/idf"
)

const purposePlanFixtureIDF = `
Version, 24.1;

Zone,
  Office;

Zone,
  Lab;

Lights,
  Office Lights,
  Office,
  ,
  LightingLevel,
  100;

ElectricEquipment,
  Office Equipment,
  Office,
  ,
  EquipmentLevel,
  100;
`

func TestBuildPurposeRunPlanBasicEnergy(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
Output:SQLite,
  SimpleAndTabular,
  JtoKWH;
`)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
	})

	if !plan.RequiresSQL {
		t.Fatalf("plan should require SQL")
	}
	sql := findPurposeOutput(plan, "Output:SQLite", "", "")
	if sql == nil {
		t.Fatalf("missing SQL output in %#v", plan.OutputObjects)
	}
	if sql.State != PurposeOutputStateExisting {
		t.Fatalf("SQL state = %q, want existing", sql.State)
	}
	if findPurposeOutput(plan, "Output:Meter", "Electricity:Facility", "") == nil {
		t.Fatalf("missing Electricity:Facility meter in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "*", "Zone Lights Electricity Energy") == nil {
		t.Fatalf("missing zone lights energy output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "*", "Zone Air Heat Balance Surface Convection Rate") != nil {
		t.Fatalf("basic energy plan should not include zone heat-flow outputs: %#v", plan.OutputObjects)
	}
	if plan.EstimatedFrames != 12 {
		t.Fatalf("estimated frames = %d, want 12 for monthly energy", plan.EstimatedFrames)
	}
}

func TestBuildPurposeRunPlanZoneHeatFlowSelectedZones(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
		Scope: SimulationPurposeScope{
			ZoneMode:  "selected",
			ZoneNames: []string{"Office"},
		},
	})

	if findPurposeOutput(plan, "Output:Variable", "Office", "Zone Mean Air Temperature") == nil {
		t.Fatalf("missing selected-zone temperature output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "*", "Zone Mean Air Temperature") != nil {
		t.Fatalf("selected-zone plan should not use wildcard zone key: %#v", plan.OutputObjects)
	}
	if plan.EstimatedFrames != 8760 {
		t.Fatalf("estimated frames = %d, want hourly full-year estimate", plan.EstimatedFrames)
	}
	if !purposePlanHasWarning(plan, "zone_scope_selected") {
		t.Fatalf("expected selected-zone scope warning in %#v", plan.Warnings)
	}
}

func TestBuildPurposeRunPlanHVACLoopCheckSelectedAirLoopNodes(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
AirLoopHVAC,
  Main Air Loop,
  ,
  ,
  Autosize,
  Air Branches,
  ,
  Air Supply Inlet,
  Air Demand Outlet,
  Air Demand Inlet,
  Air Supply Outlet;

BranchList,
  Air Branches,
  Main Air Branch;

Branch,
  Main Air Branch,
  ,
  Fan:ConstantVolume,
  Supply Fan,
  Air Supply Inlet,
  Fan Outlet;

Fan:ConstantVolume,
  Supply Fan,
  Air Supply Inlet,
  Fan Outlet;
`)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck},
		Scope: SimulationPurposeScope{
			LoopMode:     "selected",
			AirLoopNames: []string{"Main Air Loop"},
		},
	})

	if findPurposeOutput(plan, "Output:Variable", "Air Supply Inlet", "System Node Temperature") == nil {
		t.Fatalf("missing selected loop node output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "Fan Outlet", "System Node Mass Flow Rate") == nil {
		t.Fatalf("missing component outlet node output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "*", "System Node Temperature") != nil {
		t.Fatalf("selected loop plan should not use wildcard node key: %#v", plan.OutputObjects)
	}
	if !purposePlanHasWarning(plan, "hvac_scope_selected") {
		t.Fatalf("expected selected HVAC scope warning in %#v", plan.Warnings)
	}
}

func TestBuildPurposeRunPlanEstimatesFramesFromRunPeriod(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
Timestep,
  6;

RunPeriod,
  One Week,
  1,
  1,
  ,
  1,
  7;
`)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
	})

	if plan.EstimatedFrames != 168 {
		t.Fatalf("estimated frames = %d, want 168 hourly frames for one week", plan.EstimatedFrames)
	}

	timestepPlan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeCustomOutputs},
		Scope: SimulationPurposeScope{CustomOutputs: []PurposeCustomOutput{{
			ObjectType:         "Output:Variable",
			KeyValue:           "*",
			VariableName:       "Zone Mean Air Temperature",
			ReportingFrequency: "Timestep",
		}}},
	})
	if timestepPlan.EstimatedFrames != 1008 {
		t.Fatalf("timestep estimated frames = %d, want 1008", timestepPlan.EstimatedFrames)
	}
}

func TestBuildPurposeRunPlanMergesDuplicateOutputsAcrossPurposes(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy, SimulationPurposeZoneHeatFlow, SimulationPurposeIntegrity},
	})

	sqlCount := 0
	for _, object := range plan.OutputObjects {
		if strings.EqualFold(object.ObjectType, "Output:SQLite") {
			sqlCount++
			if len(object.PurposeIDs) != 3 {
				t.Fatalf("SQL purpose IDs = %#v, want all selected purposes", object.PurposeIDs)
			}
		}
	}
	if sqlCount != 1 {
		t.Fatalf("SQL output count = %d, want 1", sqlCount)
	}
	if findPurposeOutput(plan, "Output:VariableDictionary", "", "") == nil {
		t.Fatalf("integrity plan should include variable dictionary output: %#v", plan.OutputObjects)
	}
}

func TestBuildPurposeRunPlanMarksFrequencyConflict(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
Output:Variable,
  *,
  Zone Mean Air Temperature,
  Monthly;
`)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
	})

	output := findPurposeOutput(plan, "Output:Variable", "*", "Zone Mean Air Temperature")
	if output == nil {
		t.Fatalf("missing requested zone mean air temperature output")
	}
	if output.State != PurposeOutputStateConflict {
		t.Fatalf("output state = %q, want conflict", output.State)
	}
	if !purposePlanHasWarning(plan, "frequency_conflict") {
		t.Fatalf("expected frequency conflict warning in %#v", plan.Warnings)
	}
}

func TestBuildPurposeRunPlanPreservesExistingFrequency(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
Output:Variable,
  *,
  Zone Mean Air Temperature,
  Monthly;
`)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:        []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
		FrequencyPolicy: PurposeFrequencyPolicyPreserve,
	})

	output := findPurposeOutput(plan, "Output:Variable", "*", "Zone Mean Air Temperature")
	if output == nil {
		t.Fatalf("missing preserved zone mean air temperature output")
	}
	if output.State != PurposeOutputStateExisting || output.ReportingFrequency != "Monthly" {
		t.Fatalf("preserved output = %+v, want existing Monthly", output)
	}
	if !purposePlanHasWarning(plan, "frequency_preserved") {
		t.Fatalf("expected frequency preserved warning in %#v", plan.Warnings)
	}
}

func TestBuildPurposeRunPlanHighestResolutionAddsRequestedFrequency(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
Output:Variable,
  *,
  Zone Mean Air Temperature,
  Monthly;
`)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:        []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
		FrequencyPolicy: PurposeFrequencyPolicyHighestResolution,
	})

	output := findPurposeOutput(plan, "Output:Variable", "*", "Zone Mean Air Temperature")
	if output == nil {
		t.Fatalf("missing promoted zone mean air temperature output")
	}
	if output.State != PurposeOutputStateTemporary || output.ReportingFrequency != "Hourly" {
		t.Fatalf("promoted output = %+v, want temporary Hourly", output)
	}
	if !purposePlanHasWarning(plan, "frequency_promoted") {
		t.Fatalf("expected frequency promoted warning in %#v", plan.Warnings)
	}
}

func parsePurposePlanFixture(t *testing.T, text string) idf.Document {
	t.Helper()
	doc, err := idf.Parse(text)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return doc
}

func findPurposeOutput(plan PurposeRunPlan, objectType string, keyValue string, variableName string) *PurposeOutputObject {
	for index, object := range plan.OutputObjects {
		if !strings.EqualFold(object.ObjectType, objectType) {
			continue
		}
		if keyValue != "" && object.KeyValue != keyValue {
			continue
		}
		if variableName != "" && object.VariableName != variableName {
			continue
		}
		return &plan.OutputObjects[index]
	}
	return nil
}

func purposePlanHasWarning(plan PurposeRunPlan, code string) bool {
	for _, warning := range plan.Warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
