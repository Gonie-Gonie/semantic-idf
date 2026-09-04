package simulation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
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
		Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
		BasicEnergyDetail: PurposeBasicEnergyDetailHeatDrivers,
	})

	if !plan.RequiresSQL {
		t.Fatalf("plan should require SQL")
	}
	if plan.AllocationPolicy != PurposeAllocationPolicyDirectOnly {
		t.Fatalf("allocation policy = %q, want %q", plan.AllocationPolicy, PurposeAllocationPolicyDirectOnly)
	}
	if plan.BasicEnergyDetail != PurposeBasicEnergyDetailHeatDrivers {
		t.Fatalf("basic energy detail = %q, want %q", plan.BasicEnergyDetail, PurposeBasicEnergyDetailHeatDrivers)
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
	zoneEnergy := findPurposeOutput(plan, "Output:Variable", "*", "Zone Lights Electricity Energy")
	if zoneEnergy == nil {
		t.Fatalf("missing zone lights energy output in %#v", plan.OutputObjects)
	}
	if zoneEnergy.Reason != "Basic Energy Explain output" || !strings.Contains(zoneEnergy.Description, "Basic Energy Explain") {
		t.Fatalf("zone energy reason/description = %q / %q", zoneEnergy.Reason, zoneEnergy.Description)
	}
	for _, variable := range []string{
		"Zone Air System Sensible Cooling Rate",
		"Zone Ideal Loads Zone Sensible Heating Energy",
		"Zone Ideal Loads Supply Air Latent Cooling Energy",
		"Zone Ideal Loads Outdoor Air Total Heating Energy",
		"Zone Radiant HVAC Cooling Energy",
		"Cooling Coil Total Cooling Energy",
		"Plant Supply Side Cooling Demand Rate",
		"Plant Supply Side Unmet Demand Rate",
	} {
		if findPurposeOutput(plan, "Output:Variable", "*", variable) == nil {
			t.Fatalf("missing delivered-load output %q in %#v", variable, plan.OutputObjects)
		}
	}
	heatDriver := findPurposeOutput(plan, "Output:Variable", "*", "Zone Air Heat Balance Surface Convection Rate")
	if heatDriver == nil || heatDriver.ReportingFrequency != "Monthly" {
		t.Fatalf("missing monthly heat-driver output in %#v", plan.OutputObjects)
	}
	if heatDriver.Reason != "Basic Energy Heat Drivers output" || !strings.Contains(heatDriver.Description, "Basic Energy Heat Drivers") {
		t.Fatalf("heat driver reason/description = %q / %q", heatDriver.Reason, heatDriver.Description)
	}
	fanHeat := findPurposeOutput(plan, "Output:Variable", "*", "Fan Air Heat Gain Energy")
	if fanHeat == nil || fanHeat.ReportingFrequency != "Monthly" || fanHeat.Reason != "Basic Energy Heat Drivers output" {
		t.Fatalf("missing monthly fan heat-driver output in %#v", plan.OutputObjects)
	}
	for _, variable := range []string{
		"Zone People Total Heating Energy",
		"Zone Windows Total Heat Gain Energy",
		"Zone Windows Total Transmitted Solar Radiation Energy",
		"Zone Infiltration Sensible Heat Loss Energy",
		"Zone Ventilation Sensible Heat Gain Rate",
	} {
		detailedHeatDriver := findPurposeOutput(plan, "Output:Variable", "*", variable)
		if detailedHeatDriver == nil || detailedHeatDriver.ReportingFrequency != "Monthly" || detailedHeatDriver.Reason != "Basic Energy Heat Drivers output" {
			t.Fatalf("missing detailed heat-driver output %q in %#v", variable, plan.OutputObjects)
		}
	}
	if plan.EstimatedFrames != 12 {
		t.Fatalf("estimated frames = %d, want 12 for monthly energy", plan.EstimatedFrames)
	}
}

func TestBuildPurposeRunPlanBasicEnergyDefaultsToEnergyPath(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
	})

	if plan.BasicEnergyDetail != PurposeBasicEnergyDetailEnergyPath {
		t.Fatalf("basic energy detail = %q, want %q", plan.BasicEnergyDetail, PurposeBasicEnergyDetailEnergyPath)
	}
	if findPurposeOutput(plan, "Output:Meter", "Electricity:Facility", "") == nil {
		t.Fatalf("Energy Path default should include top-level meters: %#v", plan.OutputObjects)
	}
	if output := findPurposeOutput(plan, "Output:Variable", "Office", "Zone Lights Electricity Energy"); output == nil {
		t.Fatalf("Energy Path default should include zone direct-use energy: %#v", plan.OutputObjects)
	}
	if output := findPurposeOutput(plan, "Output:Variable", "Lab", "Zone Air Heat Balance Surface Convection Rate"); output == nil {
		t.Fatalf("Energy Path default should include every zone's heat-driver reconciliation output: %#v", plan.OutputObjects)
	}
	for _, object := range plan.OutputObjects {
		if purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) && object.Reason != "Basic Energy Path" {
			t.Fatalf("Energy Path output reason = %q for %+v", object.Reason, object)
		}
		if purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) && purposeObjectIsSeries(object.ObjectType) && object.ReportingFrequency != "Monthly" {
			t.Fatalf("Energy Path output frequency = %q for %+v", object.ReportingFrequency, object)
		}
	}
	if plan.EstimatedFrames != 12 {
		t.Fatalf("estimated frames = %d, want monthly Energy Path frames", plan.EstimatedFrames)
	}
}

func TestBuildPurposeRunPlanBasicEnergyDetailTiers(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	light := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
		BasicEnergyDetail: PurposeBasicEnergyDetailLight,
	})
	if light.BasicEnergyDetail != PurposeBasicEnergyDetailLight {
		t.Fatalf("light detail = %q", light.BasicEnergyDetail)
	}
	if findPurposeOutput(light, "Output:Meter", "Electricity:Facility", "") == nil {
		t.Fatalf("light tier should still include top-level meters: %#v", light.OutputObjects)
	}
	if output := findPurposeOutput(light, "Output:Variable", "*", "Zone Lights Electricity Energy"); output != nil {
		t.Fatalf("light tier should not include explain output: %+v", output)
	}
	if output := findPurposeOutput(light, "Output:Variable", "*", "Zone Air Heat Balance Surface Convection Rate"); output != nil {
		t.Fatalf("light tier should not include heat-driver output: %+v", output)
	}

	explain := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
		BasicEnergyDetail: PurposeBasicEnergyDetailExplain,
	})
	if explain.BasicEnergyDetail != PurposeBasicEnergyDetailExplain {
		t.Fatalf("explain detail = %q", explain.BasicEnergyDetail)
	}
	if findPurposeOutput(explain, "Output:Variable", "*", "Zone Lights Electricity Energy") == nil {
		t.Fatalf("explain tier should include zone energy output: %#v", explain.OutputObjects)
	}
	if findPurposeOutput(explain, "Output:Variable", "*", "Zone Air System Sensible Cooling Rate") == nil {
		t.Fatalf("explain tier should include delivered-load output: %#v", explain.OutputObjects)
	}
	if output := findPurposeOutput(explain, "Output:Variable", "*", "Zone Air Heat Balance Surface Convection Rate"); output != nil {
		t.Fatalf("explain tier should not include heat-driver output: %+v", output)
	}
}

func TestBuildPurposeRunPlanBasicEnergyHighestResolutionUsesHourlyDetail(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
		BasicEnergyDetail: PurposeBasicEnergyDetailHeatDrivers,
		FrequencyPolicy:   PurposeFrequencyPolicyHighestResolution,
	})

	for _, variable := range []string{
		"Zone Lights Electricity Energy",
		"Zone Air System Sensible Cooling Rate",
		"Fan Air Heat Gain Energy",
		"Zone Air Heat Balance Surface Convection Rate",
		"Zone Infiltration Sensible Heat Loss Energy",
	} {
		output := findPurposeOutput(plan, "Output:Variable", "*", variable)
		if output == nil || output.ReportingFrequency != "Hourly" {
			t.Fatalf("highest-resolution Basic Energy output %q = %+v", variable, output)
		}
	}
	if plan.EstimatedFrames != 8760 {
		t.Fatalf("estimated frames = %d, want hourly Basic Energy frames", plan.EstimatedFrames)
	}
}

func TestBuildPurposeRunPlanBasicEnergyWithZoneHeatFlowKeepsEnergyMonthly(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy, SimulationPurposeZoneHeatFlow},
		Scope: SimulationPurposeScope{
			ZoneMode:  "selected",
			ZoneNames: []string{"Office"},
		},
	})

	for _, object := range plan.OutputObjects {
		if !purposeObjectIsSeries(object.ObjectType) {
			continue
		}
		if purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) && object.ReportingFrequency != "Monthly" {
			t.Fatalf("Basic Energy output should stay monthly: %+v", object)
		}
		if strings.EqualFold(object.ReportingFrequency, "Hourly") && purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) {
			t.Fatalf("Hourly output should not be owned by Basic Energy: %+v", object)
		}
	}

	var monthlyEnergyPath, hourlyDrilldown *PurposeOutputObject
	for index := range plan.OutputObjects {
		output := &plan.OutputObjects[index]
		if output.ObjectType != "Output:Variable" || output.KeyValue != "Office" || output.VariableName != "Zone Air Heat Balance Surface Convection Rate" {
			continue
		}
		if output.ReportingFrequency == "Monthly" {
			monthlyEnergyPath = output
		}
		if output.ReportingFrequency == "Hourly" {
			hourlyDrilldown = output
		}
	}
	if monthlyEnergyPath == nil || !purposeIDsContain(monthlyEnergyPath.PurposeIDs, SimulationPurposeBasicEnergy) || purposeIDsContain(monthlyEnergyPath.PurposeIDs, SimulationPurposeZoneHeatFlow) {
		t.Fatalf("monthly Energy Path output = %+v", monthlyEnergyPath)
	}
	if hourlyDrilldown == nil || !purposeIDsContain(hourlyDrilldown.PurposeIDs, SimulationPurposeZoneHeatFlow) || purposeIDsContain(hourlyDrilldown.PurposeIDs, SimulationPurposeBasicEnergy) {
		t.Fatalf("hourly heat-flow drilldown output = %+v", hourlyDrilldown)
	}
	if plan.EstimatedFrames != 8760 {
		t.Fatalf("estimated frames = %d, want hourly Zone Heat Flow frames", plan.EstimatedFrames)
	}
}

func TestBuildPurposeRunPlanBasicEnergyAllocationPolicy(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:         []SimulationPurposeID{SimulationPurposeBasicEnergy},
		AllocationPolicy: PurposeAllocationPolicyByZoneLoadShare,
	})

	if plan.AllocationPolicy != PurposeAllocationPolicyByZoneLoadShare {
		t.Fatalf("allocation policy = %q, want %q", plan.AllocationPolicy, PurposeAllocationPolicyByZoneLoadShare)
	}
}

func TestBuildPurposeRunPlanPreservesPeriodScope(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
		Scope: SimulationPurposeScope{
			PeriodMode:  "custom",
			PeriodStart: "01-01",
			PeriodEnd:   "01-31",
		},
	})

	if plan.PeriodMode != "custom" || plan.PeriodStart != "01-01" || plan.PeriodEnd != "01-31" {
		t.Fatalf("period scope = %q %q %q", plan.PeriodMode, plan.PeriodStart, plan.PeriodEnd)
	}
}

func TestBuildPurposeRunPlanBasicEnergyExtendedEndUseMeters(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
Refrigeration:Compressor,
  Case Compressor;

HeatExchanger:AirToAir:FlatPlate,
  Heat Recovery HX;

Boiler:HotWater,
  Fuel Oil Boiler,
  FuelOilNo1;

DistrictCooling,
  Campus District Cooling;

DistrictHeating,
  Campus District Heating;

Generator:Photovoltaic,
  Roof PV;

ElectricLoadCenter:Storage:Simple,
  Battery;

OtherEquipment,
  Steam Process,
  Steam;
`)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
	})

	if findPurposeOutput(plan, "Output:Meter", "Refrigeration:Electricity", "") == nil {
		t.Fatalf("missing refrigeration meter in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Meter", "HeatRecovery:Electricity", "") == nil {
		t.Fatalf("missing heat recovery meter in %#v", plan.OutputObjects)
	}
	for _, meter := range []string{
		"FuelOilNo1:Facility",
		"Steam:Facility",
		"ElectricityProduced:Facility",
		"Cooling:DistrictCooling",
		"Heating:DistrictHeating",
	} {
		if findPurposeOutput(plan, "Output:Meter", meter, "") == nil {
			t.Fatalf("missing extended energy meter %q in %#v", meter, plan.OutputObjects)
		}
	}
	for _, variable := range []string{
		"Electric Storage Charge Energy",
		"Electric Storage Discharge Energy",
	} {
		output := findPurposeOutput(plan, "Output:Variable", "*", variable)
		if output == nil || output.ReportingFrequency != "Monthly" || output.Reason != "Basic Energy Path" {
			t.Fatalf("missing storage energy output %q in %#v", variable, plan.OutputObjects)
		}
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

func TestBuildPurposeRunPlanZoneHeatFlowVisibleZones(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
		Scope: SimulationPurposeScope{
			ZoneMode:  "visible",
			ZoneNames: []string{"Lab"},
		},
	})

	if findPurposeOutput(plan, "Output:Variable", "Lab", "Zone Mean Air Temperature") == nil {
		t.Fatalf("missing visible-zone temperature output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "*", "Zone Mean Air Temperature") != nil {
		t.Fatalf("visible-zone plan should not use wildcard zone key: %#v", plan.OutputObjects)
	}
	if !purposePlanHasWarning(plan, "zone_scope_visible") {
		t.Fatalf("expected visible-zone scope warning in %#v", plan.Warnings)
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
	if findPurposeOutput(plan, "Output:Variable", "Supply Fan", "Fan Electricity Rate") == nil {
		t.Fatalf("missing selected loop fan operation output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "*", "System Node Temperature") != nil {
		t.Fatalf("selected loop plan should not use wildcard node key: %#v", plan.OutputObjects)
	}
	if !purposePlanHasWarning(plan, "hvac_scope_selected") {
		t.Fatalf("expected selected HVAC scope warning in %#v", plan.Warnings)
	}
}

func TestBuildPurposeRunPlanHVACLoopCheckSelectedComponent(t *testing.T) {
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
  Fan Outlet,
  Coil:Cooling:Water,
  Cooling Coil,
  Fan Outlet,
  Coil Outlet;
`)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck},
		Scope: SimulationPurposeScope{
			LoopMode:     "selected",
			AirLoopNames: []string{"Main Air Loop"},
			ComponentIDs: []string{"Coil:Cooling:Water:Cooling Coil"},
		},
	})

	if findPurposeOutput(plan, "Output:Variable", "Cooling Coil", "Cooling Coil Total Cooling Rate") == nil {
		t.Fatalf("missing selected component operation output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "Supply Fan", "Fan Electricity Rate") != nil {
		t.Fatalf("component-scoped plan should not include unselected fan output: %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "Fan Outlet", "System Node Temperature") == nil {
		t.Fatalf("missing selected component inlet node output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "Air Supply Inlet", "System Node Temperature") != nil {
		t.Fatalf("component-scoped plan should not include loop-wide inlet node: %#v", plan.OutputObjects)
	}
}

func TestBuildPurposeRunPlanComfortSelectedZones(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeComfort},
		Scope: SimulationPurposeScope{
			ZoneMode:  "selected",
			ZoneNames: []string{"Lab"},
		},
	})

	if findPurposeOutput(plan, "Output:Variable", "Lab", "Zone Mean Air Temperature") == nil {
		t.Fatalf("missing selected-zone comfort output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "Lab", "Zone Air Relative Humidity") == nil {
		t.Fatalf("missing selected-zone humidity comfort output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "Lab", "Zone Air System Sensible Heating Rate") == nil {
		t.Fatalf("missing selected-zone heating-rate comfort output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Table:SummaryReports", "", "") == nil {
		t.Fatalf("missing comfort summary report output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "*", "Zone Mean Air Temperature") != nil {
		t.Fatalf("selected comfort plan should not use wildcard zone key: %#v", plan.OutputObjects)
	}
	if !purposePlanHasWarning(plan, "comfort_scope_selected") {
		t.Fatalf("expected comfort selected-zone warning in %#v", plan.Warnings)
	}
}

func TestBuildPurposeRunPlanComfortFilteredZones(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeComfort},
		Scope: SimulationPurposeScope{
			ZoneMode:  "filtered",
			ZoneNames: []string{"Office"},
		},
	})

	if findPurposeOutput(plan, "Output:Variable", "Office", "Zone Mean Air Temperature") == nil {
		t.Fatalf("missing filtered-zone comfort output in %#v", plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:Variable", "*", "Zone Mean Air Temperature") != nil {
		t.Fatalf("filtered comfort plan should not use wildcard zone key: %#v", plan.OutputObjects)
	}
	if !purposePlanHasWarning(plan, "comfort_scope_filtered") {
		t.Fatalf("expected comfort filtered-zone warning in %#v", plan.Warnings)
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
	monthlyEnergyPathCount := 0
	hourlyDrilldownCount := 0
	for _, object := range plan.OutputObjects {
		if object.ObjectType == "Output:Variable" && object.VariableName == "Zone Air Heat Balance Surface Convection Rate" {
			switch object.ReportingFrequency {
			case "Monthly":
				monthlyEnergyPathCount++
				if !purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) || purposeIDsContain(object.PurposeIDs, SimulationPurposeZoneHeatFlow) {
					t.Fatalf("monthly Energy Path ownership = %#v", object.PurposeIDs)
				}
			case "Hourly":
				hourlyDrilldownCount++
				if !purposeIDsContain(object.PurposeIDs, SimulationPurposeZoneHeatFlow) || purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) {
					t.Fatalf("hourly drilldown ownership = %#v", object.PurposeIDs)
				}
			}
		}
	}
	if monthlyEnergyPathCount != 2 || hourlyDrilldownCount != 1 {
		t.Fatalf("heat-driver counts = monthly %d / hourly %d, want one monthly per zone and one wildcard hourly in %#v", monthlyEnergyPathCount, hourlyDrilldownCount, plan.OutputObjects)
	}
	if findPurposeOutput(plan, "Output:VariableDictionary", "", "") == nil {
		t.Fatalf("integrity plan should include variable dictionary output: %#v", plan.OutputObjects)
	}
}

func TestBuildPurposeRunPlanGoldenSnapshots(t *testing.T) {
	hvacDoc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
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
	cases := []struct {
		name    string
		doc     idf.Document
		request SimulationPurposeRequest
		want    string
	}{
		{
			name: "basic_energy_heat_drivers",
			doc:  parsePurposePlanFixture(t, purposePlanFixtureIDF),
			request: SimulationPurposeRequest{
				Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
				BasicEnergyDetail: PurposeBasicEnergyDetailHeatDrivers,
			},
			want: `requires_sql=true requires_discovery=false weight=Light frames=12
output|Output:SQLite||||temporary|basic_energy
output|Output:Meter|Electricity:Facility||Monthly|temporary|basic_energy
output|Output:Meter|Electricity:InteriorEquipment||Monthly|temporary|basic_energy
output|Output:Meter|Electricity:InteriorLights||Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Infiltration Latent Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Infiltration Latent Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Infiltration Latent Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Infiltration Latent Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Infiltration Sensible Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Infiltration Sensible Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Infiltration Sensible Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Infiltration Sensible Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Mixing Latent Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Mixing Latent Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Mixing Latent Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Mixing Latent Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Mixing Sensible Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Mixing Sensible Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Mixing Sensible Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Mixing Sensible Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Ventilation Latent Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Ventilation Latent Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Ventilation Latent Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Ventilation Latent Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Ventilation Sensible Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Ventilation Sensible Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Ventilation Sensible Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|AFN Zone Ventilation Sensible Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Latent Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Latent Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Latent Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Latent Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Sensible Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Sensible Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Sensible Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Sensible Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Total Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Total Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Total Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Air System Outdoor Air Total Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Cond Loop Demand Not Distributed|Monthly|temporary|basic_energy
output|Output:Variable|*|Cooling Coil Sensible Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Cooling Coil Total Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Cooling Coil Total Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Fan Air Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Fan Air Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Latent Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Latent Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Latent Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Latent Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Sensible Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Sensible Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Sensible Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Sensible Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Total Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Total Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Total Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Heat Exchanger Total Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Heating Coil Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Heating Coil Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Plant Loop Cooling Demand Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Plant Loop Heating Demand Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Plant Supply Side Cooling Demand Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Plant Supply Side Heating Demand Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Plant Supply Side Not Distributed Demand Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Plant Supply Side Unmet Demand Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air Heat Balance Air Energy Storage Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air Heat Balance Deviation Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air Heat Balance Internal Convective Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air Heat Balance Interzone Air Transfer Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air Heat Balance Outdoor Air Transfer Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air Heat Balance Surface Convection Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air Heat Balance System Air Transfer Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air Heat Balance System Convective Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air System Sensible Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air System Sensible Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air System Sensible Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Air System Sensible Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Combined Outdoor Air Latent Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Combined Outdoor Air Latent Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Combined Outdoor Air Latent Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Combined Outdoor Air Latent Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Combined Outdoor Air Sensible Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Combined Outdoor Air Sensible Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Combined Outdoor Air Sensible Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Combined Outdoor Air Sensible Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Convective Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Convective Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Electricity Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Latent Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Latent Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Lost Heat Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Lost Heat Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Radiant Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Radiant Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Total Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Electric Equipment Total Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Convective Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Convective Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Gas Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Latent Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Latent Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Lost Heat Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Lost Heat Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Radiant Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Radiant Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Total Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Gas Equipment Total Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Hot Water Equipment Convective Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Hot Water Equipment Convective Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Hot Water Equipment Latent Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Hot Water Equipment Latent Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Latent Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Latent Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Latent Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Latent Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Sensible Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Sensible Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Sensible Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Sensible Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Total Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Total Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Total Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Heat Recovery Total Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Latent Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Latent Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Latent Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Latent Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Sensible Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Sensible Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Sensible Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Sensible Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Total Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Total Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Total Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Outdoor Air Total Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Supply Air Latent Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Supply Air Latent Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Supply Air Latent Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Supply Air Latent Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Supply Air Total Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Supply Air Total Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Zone Latent Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Zone Latent Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Zone Latent Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Zone Latent Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Zone Sensible Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ideal Loads Zone Sensible Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Infiltration Latent Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Infiltration Latent Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Infiltration Latent Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Infiltration Latent Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Infiltration Sensible Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Infiltration Sensible Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Infiltration Sensible Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Infiltration Sensible Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone IT Equipment Convective Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone IT Equipment Convective Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone IT Equipment Latent Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone IT Equipment Latent Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Convective Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Convective Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Electricity Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Radiant Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Radiant Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Return Air Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Return Air Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Total Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Total Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Visible Radiation Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Lights Visible Radiation Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Mixing Latent Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Mixing Latent Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Mixing Latent Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Mixing Latent Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Mixing Sensible Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Mixing Sensible Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Mixing Sensible Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Mixing Sensible Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Equipment Convective Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Equipment Convective Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Equipment Latent Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Equipment Latent Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Equipment Radiant Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Equipment Radiant Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Equipment Total Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Equipment Total Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Internal Convective Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Internal Convective Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Internal Latent Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Other Internal Latent Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone People Convective Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone People Convective Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone People Latent Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone People Latent Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone People Radiant Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone People Radiant Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone People Sensible Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone People Sensible Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone People Total Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone People Total Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Radiant HVAC Cooling Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Radiant HVAC Cooling Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Radiant HVAC Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Radiant HVAC Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Steam Equipment Convective Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Steam Equipment Convective Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Steam Equipment Latent Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Steam Equipment Latent Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Total Internal Convective Heating Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Total Internal Convective Heating Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Total Internal Latent Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Total Internal Latent Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Transmitted Solar Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ventilation Latent Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ventilation Latent Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ventilation Latent Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ventilation Latent Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ventilation Sensible Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ventilation Sensible Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ventilation Sensible Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Ventilation Sensible Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Windows Total Heat Gain Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Windows Total Heat Gain Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Windows Total Heat Loss Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Windows Total Heat Loss Rate|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Windows Total Transmitted Solar Radiation Energy|Monthly|temporary|basic_energy
output|Output:Variable|*|Zone Windows Total Transmitted Solar Radiation Rate|Monthly|temporary|basic_energy
`,
		},
		{
			name: "zone_heat_flow_selected",
			doc:  parsePurposePlanFixture(t, purposePlanFixtureIDF),
			request: SimulationPurposeRequest{
				Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
				Scope: SimulationPurposeScope{
					ZoneMode:  "selected",
					ZoneNames: []string{"Office"},
				},
			},
			want: `requires_sql=true requires_discovery=false weight=Medium frames=8760
warning|info|zone_scope_selected|zone_heat_flow
output|Output:SQLite||||temporary|zone_heat_flow
output|Output:Variable|Office|Zone Air Heat Balance Air Energy Storage Rate|Hourly|temporary|zone_heat_flow
output|Output:Variable|Office|Zone Air Heat Balance Deviation Rate|Hourly|temporary|zone_heat_flow
output|Output:Variable|Office|Zone Air Heat Balance Internal Convective Heat Gain Rate|Hourly|temporary|zone_heat_flow
output|Output:Variable|Office|Zone Air Heat Balance Interzone Air Transfer Rate|Hourly|temporary|zone_heat_flow
output|Output:Variable|Office|Zone Air Heat Balance Outdoor Air Transfer Rate|Hourly|temporary|zone_heat_flow
output|Output:Variable|Office|Zone Air Heat Balance Surface Convection Rate|Hourly|temporary|zone_heat_flow
output|Output:Variable|Office|Zone Air Heat Balance System Air Transfer Rate|Hourly|temporary|zone_heat_flow
output|Output:Variable|Office|Zone Air Heat Balance System Convective Heat Gain Rate|Hourly|temporary|zone_heat_flow
output|Output:Variable|Office|Zone Mean Air Temperature|Hourly|temporary|zone_heat_flow
`,
		},
		{
			name: "hvac_loop_selected",
			doc:  hvacDoc,
			request: SimulationPurposeRequest{
				Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck},
				Scope: SimulationPurposeScope{
					LoopMode:     "selected",
					AirLoopNames: []string{"Main Air Loop"},
				},
			},
			want: `requires_sql=true requires_discovery=false weight=Heavy frames=8760
warning|info|hvac_scope_selected|hvac_loop_check
warning|warning|output_weight_heavy|hvac_loop_check
output|Output:SQLite||||temporary|hvac_loop_check
output|Output:Variable|Air Demand Inlet|System Node Enthalpy|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Demand Inlet|System Node Humidity Ratio|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Demand Inlet|System Node Mass Flow Rate|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Demand Inlet|System Node Setpoint Temperature|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Demand Inlet|System Node Temperature|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Demand Outlet|System Node Enthalpy|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Demand Outlet|System Node Humidity Ratio|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Demand Outlet|System Node Mass Flow Rate|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Demand Outlet|System Node Setpoint Temperature|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Demand Outlet|System Node Temperature|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Supply Inlet|System Node Enthalpy|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Supply Inlet|System Node Humidity Ratio|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Supply Inlet|System Node Mass Flow Rate|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Supply Inlet|System Node Setpoint Temperature|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Supply Inlet|System Node Temperature|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Supply Outlet|System Node Enthalpy|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Supply Outlet|System Node Humidity Ratio|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Supply Outlet|System Node Mass Flow Rate|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Supply Outlet|System Node Setpoint Temperature|Hourly|temporary|hvac_loop_check
output|Output:Variable|Air Supply Outlet|System Node Temperature|Hourly|temporary|hvac_loop_check
output|Output:Variable|Fan Outlet|System Node Enthalpy|Hourly|temporary|hvac_loop_check
output|Output:Variable|Fan Outlet|System Node Humidity Ratio|Hourly|temporary|hvac_loop_check
output|Output:Variable|Fan Outlet|System Node Mass Flow Rate|Hourly|temporary|hvac_loop_check
output|Output:Variable|Fan Outlet|System Node Setpoint Temperature|Hourly|temporary|hvac_loop_check
output|Output:Variable|Fan Outlet|System Node Temperature|Hourly|temporary|hvac_loop_check
output|Output:Variable|Supply Fan|Fan Electricity Energy|Hourly|temporary|hvac_loop_check
output|Output:Variable|Supply Fan|Fan Electricity Rate|Hourly|temporary|hvac_loop_check
`,
		},
		{
			name:    "integrity",
			doc:     parsePurposePlanFixture(t, purposePlanFixtureIDF),
			request: SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeIntegrity}},
			want: `requires_sql=true requires_discovery=false weight=Light frames=1
output|Output:SQLite||||temporary|integrity_check
output|Output:Diagnostics||||temporary|integrity_check
output|Output:Table:SummaryReports||||temporary|integrity_check
output|Output:VariableDictionary||||temporary|integrity_check
output|OutputControl:Table:Style||||temporary|integrity_check
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := snapshotPurposeRunPlan(BuildPurposeRunPlan(tc.doc, tc.request))
			if got != tc.want {
				t.Fatalf("purpose plan snapshot mismatch\nwant:\n%s\ngot:\n%s", tc.want, got)
			}
		})
	}
}

func TestBuildPurposeRunPlanLargeModelWeightWarning(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("Version, 24.1;\n\n")
	var zones []string
	for index := 1; index <= 80; index++ {
		name := fmt.Sprintf("Zone %02d", index)
		zones = append(zones, name)
		fmt.Fprintf(&builder, "Zone,\n  %s;\n\n", name)
	}
	doc := parsePurposePlanFixture(t, builder.String())

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
		Scope: SimulationPurposeScope{
			ZoneMode:  "selected",
			ZoneNames: zones,
		},
	})

	if plan.EstimatedWeight != "Very Heavy" {
		t.Fatalf("estimated weight = %q, want Very Heavy (series=%d frames=%d)", plan.EstimatedWeight, plan.EstimatedSeries, plan.EstimatedFrames)
	}
	if !purposePlanHasWarning(plan, "output_weight_very_heavy") {
		t.Fatalf("expected very-heavy output weight warning in %#v", plan.Warnings)
	}
}

func TestBuildPurposeRunPlanDiscoveryAddsDictionaryOutput(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:         []SimulationPurposeID{SimulationPurposeBasicEnergy},
		DiscoveryAllowed: true,
	})

	if !plan.RequiresDiscovery {
		t.Fatalf("plan should require discovery when discovery is allowed")
	}
	dictionary := findPurposeOutput(plan, "Output:VariableDictionary", "", "")
	if dictionary == nil {
		t.Fatalf("discovery plan should include variable dictionary output: %#v", plan.OutputObjects)
	}
	if dictionary.State != PurposeOutputStateTemporary {
		t.Fatalf("dictionary output state = %q, want temporary", dictionary.State)
	}
	if !purposeIDsContain(dictionary.PurposeIDs, SimulationPurposeBasicEnergy) {
		t.Fatalf("dictionary purpose IDs = %#v, want basic energy", dictionary.PurposeIDs)
	}
	if !purposePlanHasWarning(plan, "discovery_dictionary_requested") {
		t.Fatalf("expected discovery dictionary warning in %#v", plan.Warnings)
	}
}

func TestBuildPurposeRunPlanMarksPersistedOutputs(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:       []SimulationPurposeID{SimulationPurposeBasicEnergy},
		PersistOutputs: true,
	})

	meter := findPurposeOutput(plan, "Output:Meter", "Electricity:Facility", "")
	if meter == nil {
		t.Fatalf("missing facility meter in %#v", plan.OutputObjects)
	}
	if meter.State != PurposeOutputStateWillPersist {
		t.Fatalf("meter state = %q, want will persist", meter.State)
	}
	sql := findPurposeOutput(plan, "Output:SQLite", "", "")
	if sql == nil || sql.State != PurposeOutputStateWillPersist {
		t.Fatalf("sql output = %#v, want will persist", sql)
	}
}

func TestPurposeRunPlanTemporaryOutputDiffIncludesOnlyTemporaryOutputs(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
Output:Variable,
  *,
  Zone Mean Air Temperature,
  Hourly;
`)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
	})

	diff := PurposeRunPlanTemporaryOutputDiff(plan)
	if !strings.Contains(diff, "+++ purpose-run-copy.idf") || !strings.Contains(diff, "+Output:SQLite,") {
		t.Fatalf("diff does not include expected added outputs:\n%s", diff)
	}
	if strings.Contains(diff, "+  Zone Mean Air Temperature") {
		t.Fatalf("diff includes existing output:\n%s", diff)
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
	if output.ObjectIndex == nil {
		t.Fatalf("conflict output should reference existing object index: %#v", output)
	}
}

func TestPurposeRunPlanApplyRequestModes(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF+`
Output:Variable,
  *,
  Zone Mean Air Temperature,
  Monthly;
`)

	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
	})
	conflict := findPurposeOutput(plan, "Output:Variable", "*", "Zone Mean Air Temperature")
	if conflict == nil || conflict.ObjectIndex == nil {
		t.Fatalf("missing conflict output with object index: %#v", plan.OutputObjects)
	}

	defaultRequest := PurposeRunPlanApplyRequest(plan)
	if hasOutputObjectRequest(defaultRequest.AddObjects, "Output:Variable", "*", "Zone Mean Air Temperature") {
		t.Fatalf("default apply request should not add conflicting purpose output: %#v", defaultRequest)
	}

	keepRequest := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd)
	if !hasOutputObjectRequest(keepRequest.AddObjects, "Output:Variable", "*", "Zone Mean Air Temperature") {
		t.Fatalf("keep-existing apply request should add conflicting purpose output: %#v", keepRequest)
	}

	replaceRequest := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeReplaceConflicts)
	if len(replaceRequest.Updates) != 1 || replaceRequest.Updates[0].ObjectIndex != *conflict.ObjectIndex || replaceRequest.Updates[0].Value != "Hourly" {
		t.Fatalf("replace-conflicting apply request = %#v", replaceRequest)
	}

	removeRequest := PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeRemovePurpose)
	if len(removeRequest.RemoveObjectIndexes) != 1 || removeRequest.RemoveObjectIndexes[0] != *conflict.ObjectIndex {
		t.Fatalf("remove-purpose apply request = %#v", removeRequest)
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

func snapshotPurposeRunPlan(plan PurposeRunPlan) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "requires_sql=%t requires_discovery=%t weight=%s frames=%d\n", plan.RequiresSQL, plan.RequiresDiscovery, plan.EstimatedWeight, plan.EstimatedFrames)
	for _, warning := range plan.Warnings {
		fmt.Fprintf(&builder, "warning|%s|%s|%s\n", warning.Severity, warning.Code, warning.PurposeID)
	}
	for _, object := range plan.OutputObjects {
		fmt.Fprintf(
			&builder,
			"output|%s|%s|%s|%s|%s|%s\n",
			object.ObjectType,
			object.KeyValue,
			object.VariableName,
			object.ReportingFrequency,
			object.State,
			purposeIDsSnapshot(object.PurposeIDs),
		)
	}
	return builder.String()
}

func purposeIDsSnapshot(values []SimulationPurposeID) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return strings.Join(out, ",")
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

func hasOutputObjectRequest(objects []idf.OutputObjectRequest, objectType string, keyValue string, variableName string) bool {
	for _, object := range objects {
		if !strings.EqualFold(object.ObjectType, objectType) {
			continue
		}
		if keyValue != "" && purposeFieldValue(object.Fields, "Key Value", "Key Name") != keyValue {
			continue
		}
		if variableName != "" && purposeFieldValue(object.Fields, "Variable Name") != variableName {
			continue
		}
		return true
	}
	return false
}

func purposePlanHasWarning(plan PurposeRunPlan, code string) bool {
	for _, warning := range plan.Warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}
