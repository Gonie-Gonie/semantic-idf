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
