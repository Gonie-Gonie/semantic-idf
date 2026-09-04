package simulation

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const energyPathScopeFixtureIDF = purposePlanFixtureIDF + `
Space,
  Office Space,
  Office;

Space,
  Lab Space,
  Lab;

ZoneList,
  Shared Zones,
  Office,
  Lab;

SpaceList,
  Lab Spaces,
  Lab Space;

GasEquipment,
  Office Gas Equipment,
  Office,
  ,
  EquipmentLevel,
  100;

Boiler:HotWater,
  Central Gas Boiler,
  NaturalGas;

Coil:Heating:Fuel,
  Central Gas Heating Coil,
  NaturalGas;

InternalMass,
  Office Mass,
  Interior Furnishings,
  Office,
  ,
  20;

InternalMass,
  Shared Mass,
  Interior Furnishings,
  Shared Zones,
  ,
  30;

InternalMass,
  Lab Space Mass,
  Interior Furnishings,
  ,
  Lab Spaces,
  10;

InternalMass,
  Office Direct Space Mass,
  Interior Furnishings,
  ,
  Office Space,
  12;

ZoneHVAC:EquipmentConnections,
  Office,
  Office Equipment List,
  Office Inlet,
  ,
  Office Air Node,
  ;

ZoneHVAC:EquipmentList,
  Office Equipment List,
  SequentialLoad,
  ZoneHVAC:IdealLoadsAirSystem,
  Office Ideal Loads,
  1,
  1;

ZoneHVAC:IdealLoadsAirSystem,
  Office Ideal Loads;

ZoneHVAC:EquipmentConnections,
  Lab,
  Lab Equipment List,
  Lab Inlet,
  ,
  Lab Air Node,
  ;

ZoneHVAC:EquipmentList,
  Lab Equipment List,
  SequentialLoad,
  ZoneHVAC:IdealLoadsAirSystem,
  Lab Ideal Loads,
  1,
  1;

ZoneHVAC:IdealLoadsAirSystem,
  Lab Ideal Loads;

Coil:Cooling:DX:SingleSpeed,
  Central Cooling Coil;

BuildingSurface:Detailed,
  Office Wall,
  Wall,
  ,
  Office,
  ,
  Outdoors,
  ,
  SunExposed,
  WindExposed,
  ,
  4,
  0, 0, 0,
  4, 0, 0,
  4, 0, 3,
  0, 0, 3;

BuildingSurface:Detailed,
  Lab Wall,
  Wall,
  ,
  Lab,
  ,
  Outdoors,
  ,
  SunExposed,
  WindExposed,
  ,
  4,
  10, 0, 0,
  14, 0, 0,
  14, 0, 3,
  10, 0, 3;

BuildingSurface:Detailed,
  Office Wall Two,
  Wall,
  ,
  Office,
  ,
  Outdoors,
  ,
  SunExposed,
  WindExposed,
  ,
  4,
  0, 5, 0,
  4, 5, 0,
  4, 5, 3,
  0, 5, 3;

FenestrationSurface:Detailed,
  Office Window,
  Window,
  ,
  Office Wall,
  ,
  ,
  ,
  1,
  4,
  1, 0, 1,
  2, 0, 1,
  2, 0, 2,
  1, 0, 2;
`

const energyPathCurrentFuelFixtureSuffix = `
Coil:Heating:Fuel,
  Gasoline Heating Coil,
  Gasoline;

Coil:Heating:Fuel,
  Diesel Heating Coil,
  Diesel;

Coil:Heating:Fuel,
  Coal Heating Coil,
  Coal;

Coil:Heating:Fuel,
  Fuel Oil 1 Heating Coil,
  FuelOilNo1;

Coil:Heating:Fuel,
  Fuel Oil 2 Heating Coil,
  FuelOilNo2;

Coil:Heating:Fuel,
  Propane Heating Coil,
  Propane;

Coil:Heating:Fuel,
  Other Fuel 1 Heating Coil,
  OtherFuel1;

Coil:Heating:Fuel,
  Other Fuel 2 Heating Coil,
  OtherFuel2;

DistrictHeating:Water,
  Purchased Hot Water;

DistrictHeating:Steam,
  Purchased Steam;
`

func TestEPATH040BasicEnergyPathMonthlyFourStageOutputContract(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
	})

	if plan.BasicEnergyDetail != PurposeBasicEnergyDetailEnergyPath || plan.EstimatedFrames != 12 {
		t.Fatalf("default Basic Energy contract = detail %q / frames %d", plan.BasicEnergyDetail, plan.EstimatedFrames)
	}
	if !plan.RequiresDiscovery || findPurposeOutput(plan, "Output:VariableDictionary", "", "") == nil {
		t.Fatalf("default Energy Path must request post-run RDD/MDD discovery: requires=%t", plan.RequiresDiscovery)
	}
	for _, object := range plan.OutputObjects {
		if !purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) {
			continue
		}
		if object.Reason != "Basic Energy Path" {
			t.Errorf("reason for %s/%s/%s = %q", object.ObjectType, object.KeyValue, object.VariableName, object.Reason)
		}
		if purposeObjectIsSeries(object.ObjectType) && object.ReportingFrequency != "Monthly" {
			t.Errorf("frequency for %s/%s/%s = %q", object.ObjectType, object.KeyValue, object.VariableName, object.ReportingFrequency)
		}
	}

	requiredZoneVariables := []string{
		"Zone Gas Equipment NaturalGas Energy",
		"Zone Gas Equipment Gas Energy",
		"Zone Air System Sensible Heating Energy",
		"Zone Air System Sensible Cooling Energy",
		"Zone Air System Latent Heating Energy",
		"Zone Air System Latent Cooling Energy",
		"Zone People Convective Heating Energy",
		"Zone People Latent Gain Energy",
		"Zone Electric Equipment Convective Heating Energy",
		"Zone Electric Equipment Latent Gain Energy",
		"Zone Infiltration Sensible Heat Gain Energy",
		"Zone Infiltration Latent Heat Loss Energy",
		"Zone Ventilation Sensible Heat Loss Energy",
		"Zone Ventilation Latent Heat Gain Energy",
		"Zone Mixing Sensible Heat Gain Energy",
		"AFN Zone Mixing Latent Heat Loss Energy",
		"Zone Air Heat Balance Air Energy Storage Rate",
		"Zone Air Heat Balance Surface Convection Rate",
		"Zone Air Heat Balance Outdoor Air Transfer Rate",
		"Zone Air Heat Balance Internal Convective Heat Gain Rate",
	}
	for _, zoneName := range []string{"Office", "Lab"} {
		if findPurposeOutput(plan, "Output:Variable", zoneName, "Zone Lights Electricity Energy") == nil {
			t.Errorf("missing %s direct-use energy", zoneName)
		}
		for _, variable := range requiredZoneVariables {
			if findPurposeOutput(plan, "Output:Variable", zoneName, variable) == nil {
				t.Errorf("missing %s / %s", zoneName, variable)
			}
		}
	}
	for _, target := range []struct {
		key      string
		zoneName string
	}{
		{key: "Office Ideal Loads", zoneName: "Office"},
		{key: "Lab Ideal Loads", zoneName: "Lab"},
	} {
		for _, variable := range []string{"Zone Ideal Loads Supply Air Latent Heating Energy", "Zone Ideal Loads Supply Air Latent Cooling Energy"} {
			output := findPurposeOutput(plan, "Output:Variable", target.key, variable)
			if output == nil || output.ScopeZoneName != target.zoneName {
				t.Errorf("ideal-load target %s / %s = %#v", target.key, variable, output)
			}
		}
	}
	for _, surfaceName := range []string{
		"Office Wall", "Office Wall Two", "Office Window", "Lab Wall", "Office Mass", "Office Shared Mass", "Lab Shared Mass", "Lab Space Lab Space Mass",
		"Office Direct Space Mass",
	} {
		for _, variable := range []string{
			"Surface Inside Face Convection Heat Transfer Energy",
			"Surface Inside Face Convection Heat Transfer Rate",
		} {
			if findPurposeOutput(plan, "Output:Variable", surfaceName, variable) == nil {
				t.Errorf("missing surface Energy Path output %s / %s", surfaceName, variable)
			}
		}
	}
	if findPurposeOutput(plan, "Output:Variable", "*", "Zone Lights Electricity Energy") != nil ||
		findPurposeOutput(plan, "Output:Variable", "*", "Surface Inside Face Convection Heat Transfer Energy") != nil {
		t.Fatalf("building Energy Path should expand known zone/surface keys instead of wildcarding: %#v", plan.OutputObjects)
	}
	for _, meter := range []string{"Electricity:Facility", "Cooling:Electricity", "InteriorLights:Electricity"} {
		if findPurposeOutput(plan, "Output:Meter", meter, "") == nil {
			t.Errorf("missing carrier/end-use meter %q", meter)
		}
	}
	for _, meter := range []string{"NaturalGas:Facility", "Gas:Facility", "Heating:NaturalGas", "Heating:Gas"} {
		if findPurposeOutput(plan, "Output:Meter", meter, "") == nil {
			t.Errorf("missing version-compatible gas meter %q", meter)
		}
	}
	apply := PurposeRunPlanApplyRequest(plan)
	for _, meter := range []string{"NaturalGas:Facility", "Gas:Facility", "Heating:NaturalGas", "Heating:Gas"} {
		if !hasOutputObjectRequest(apply.AddObjects, "Output:Meter", meter, "") {
			t.Errorf("run-copy apply request omitted version-compatible gas meter %q", meter)
		}
	}
	for _, reversed := range []string{"Electricity:Cooling", "Electricity:InteriorLights"} {
		if findPurposeOutput(plan, "Output:Meter", reversed, "") != nil {
			t.Errorf("Energy Path requested noncanonical meter order %q", reversed)
		}
	}
}

func TestEPATH040MeterCatalogReadsCanonicalAndLegacyOrders(t *testing.T) {
	for _, pair := range [][2]string{
		{"Cooling:Electricity", "Electricity:Cooling"},
		{"InteriorLights:Electricity", "Electricity:InteriorLights"},
		{"Heating:NaturalGas", "NaturalGas:Heating"},
		{"Heating:Gas", "Gas:Heating"},
	} {
		canonical, canonicalOK := energyMeterAliasDefinitionForName(pair[0])
		legacy, legacyOK := energyMeterAliasDefinitionForName(pair[1])
		if !canonicalOK || !legacyOK || canonical.Kind != legacy.Kind || canonical.Carrier != legacy.Carrier || canonical.EndUse != legacy.EndUse {
			t.Errorf("meter aliases %q / %q = %#v / %#v", pair[0], pair[1], canonical, legacy)
		}
	}
}

func TestEPATH040CurrentFuelAndDistrictMetersReachPlanAndApply(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF+energyPathCurrentFuelFixtureSuffix)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	required := []string{
		"Gasoline:Facility", "Diesel:Facility", "Coal:Facility",
		"FuelOilNo1:Facility", "FuelOilNo2:Facility", "Propane:Facility", "OtherFuel1:Facility", "OtherFuel2:Facility",
		"DistrictHeatingWater:Facility", "DistrictHeating:Facility", "DistrictHeatingSteam:Facility", "Steam:Facility",
		"Heating:Gasoline", "Heating:Diesel", "Heating:Coal",
		"Heating:FuelOilNo1", "Heating:FuelOilNo2", "Heating:Propane", "Heating:OtherFuel1", "Heating:OtherFuel2",
		"Heating:DistrictHeatingWater", "Heating:DistrictHeating", "DistrictHeating:Heating",
		"Heating:DistrictHeatingSteam", "Heating:Steam", "Steam:Heating",
	}
	apply := PurposeRunPlanApplyRequest(plan)
	for _, meter := range required {
		output := findPurposeOutput(plan, "Output:Meter", meter, "")
		if output == nil || output.ReportingFrequency != "Monthly" || output.Reason != "Basic Energy Path" {
			t.Errorf("current/version fuel meter %q = %#v", meter, output)
		}
		if !hasOutputObjectRequest(apply.AddObjects, "Output:Meter", meter, "") {
			t.Errorf("run-copy apply request omitted fuel meter %q", meter)
		}
	}
	for _, reverseOnly := range []string{"Gasoline:Heating", "Diesel:Heating", "Coal:Heating", "FuelOilNo1:Heating", "FuelOilNo2:Heating", "Propane:Heating", "OtherFuel1:Heating", "OtherFuel2:Heating"} {
		if findPurposeOutput(plan, "Output:Meter", reverseOnly, "") != nil {
			t.Errorf("Energy Path requested noncanonical current meter order %q", reverseOnly)
		}
	}
	for _, test := range []struct {
		name    string
		carrier string
	}{
		{name: "DistrictHeatingWater:Facility", carrier: "district_heating"},
		{name: "DistrictHeating:Facility", carrier: "district_heating"},
		{name: "DistrictHeatingSteam:Facility", carrier: "steam"},
		{name: "Steam:Facility", carrier: "steam"},
		{name: "Heating:Gasoline", carrier: "gasoline"},
		{name: "Heating:Diesel", carrier: "diesel"},
		{name: "Heating:Coal", carrier: "coal"},
	} {
		definition, ok := energyMeterAliasDefinitionForName(test.name)
		if !ok || definition.Carrier != test.carrier {
			t.Errorf("meter catalog %q = %#v, found=%t", test.name, definition, ok)
		}
	}
}

func TestEPATH041LegacyLightReturnsOnlyCarrierAndEndUse(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF)
	lightPlan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
		BasicEnergyDetail: PurposeBasicEnergyDetailLight,
	})
	if lightPlan.RequiresDiscovery || findPurposeOutput(lightPlan, "Output:VariableDictionary", "", "") != nil {
		t.Fatalf("explicit legacy light unexpectedly enabled Energy Path discovery: %+v", lightPlan)
	}

	plan := PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailLight}
	legacy := buildEnergyExplanationResult([]energyExplanationSeries{
		{Level: "energy", Kind: "energy.electricity.total", Carrier: "electricity", EndUse: "total", MeterHierarchyLevel: "facility_total", Unit: "kWh", Total: 10, SourceIDs: []string{"energy"}},
		{Level: "energy", Kind: "energy.cooling", Carrier: "electricity", EndUse: "cooling", MeterHierarchyLevel: "broad_end_use", Unit: "kWh", Total: 8, SourceIDs: []string{"end-use"}},
		{Level: "load", Kind: "load.zone_cooling", ServiceKind: "cooling", ZoneName: "Office", Unit: "kWh", Total: 7, SourceIDs: []string{"load"}},
		{Level: "heat", Kind: "heat.infiltration", HeatCategory: "air_exchange", ZoneName: "Office", Unit: "kWh", Total: 6, SourceIDs: []string{"driver"}},
	}, []EnergyDataSource{{ID: "energy"}, {ID: "end-use"}, {ID: "load"}, {ID: "driver"}}, &plan)
	result := UpgradeEnergyExplanationV1(legacy)

	for _, node := range result.Nodes {
		if node.Level == "load" || node.Level == "driver" {
			t.Fatalf("explicit light leaked %q node: %#v", node.Level, node)
		}
	}
	if len(result.Sources) != 2 || energyExplanationSourceByID(result.Sources, "load") != nil || energyExplanationSourceByID(result.Sources, "driver") != nil {
		t.Fatalf("explicit light sources = %#v", result.Sources)
	}
	if result.Completeness.DeliveredLoad.Status != "not_requested" || result.Completeness.HeatDrivers.Status != "not_requested" {
		t.Fatalf("explicit light completeness = %#v", result.Completeness)
	}
}

func TestEPATH040EnergyPathAddsMonthlyAlongsideExistingAnnualOutputs(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF+`
Output:SQLite,
  SimpleAndTabular,
  JtoKWH;

Output:Meter,
  Electricity:Facility,
  Annual;

Output:Variable,
  Office,
  Zone Lights Electricity Energy,
  Annual;
`)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	for _, output := range []*PurposeOutputObject{
		findPurposeOutput(plan, "Output:Meter", "Electricity:Facility", ""),
		findPurposeOutput(plan, "Output:Variable", "Office", "Zone Lights Electricity Energy"),
	} {
		if output == nil || output.ReportingFrequency != "Monthly" || output.State == PurposeOutputStateConflict || output.Reason != "Basic Energy Path" {
			t.Fatalf("Monthly Energy Path conflict projection = %#v", output)
		}
	}
	if output := findPurposeOutput(plan, "Output:SQLite", "", ""); output == nil || output.Reason != "Basic Energy Path" {
		t.Fatalf("existing SQL output reason = %#v", output)
	}
	apply := PurposeRunPlanApplyRequest(plan)
	if !hasOutputObjectRequest(apply.AddObjects, "Output:Meter", "Electricity:Facility", "") ||
		!hasOutputObjectRequest(apply.AddObjects, "Output:Variable", "Office", "Zone Lights Electricity Energy") {
		t.Fatalf("add-missing Energy Path request did not retain Monthly additions: %#v", apply)
	}
}

func TestEPATH042EnergyPathScopeKeysAndHourlyDrilldownIsolation(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF)
	building := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	if building.ZoneMode != "all" || len(building.ZoneNames) != 0 {
		t.Fatalf("building plan scope metadata = %q / %#v", building.ZoneMode, building.ZoneNames)
	}
	for _, key := range []string{"Office", "Lab"} {
		if findPurposeOutput(building, "Output:Variable", key, "Zone Air System Sensible Cooling Energy") == nil {
			t.Errorf("building plan omitted zone %q", key)
		}
	}

	zone := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:           []SimulationPurposeID{SimulationPurposeBasicEnergy, SimulationPurposeZoneHeatFlow},
		ZoneHeatFlowDetail: PurposeZoneHeatFlowDetailSurface,
		Scope: SimulationPurposeScope{
			ZoneMode:  "selected",
			ZoneNames: []string{"Office"},
		},
	})
	if zone.ZoneMode != "selected" || len(zone.ZoneNames) != 1 || zone.ZoneNames[0] != "Office" {
		t.Fatalf("selected plan scope metadata = %q / %#v", zone.ZoneMode, zone.ZoneNames)
	}
	for _, variable := range []string{"Zone Lights Electricity Energy", "Zone Gas Equipment NaturalGas Energy", "Zone Gas Equipment Gas Energy", "Zone Air System Sensible Cooling Energy", "Zone People Latent Gain Energy"} {
		if findPurposeOutput(zone, "Output:Variable", "Office", variable) == nil {
			t.Errorf("selected zone plan omitted Office / %s", variable)
		}
		if findPurposeOutput(zone, "Output:Variable", "Lab", variable) != nil || findPurposeOutput(zone, "Output:Variable", "*", variable) != nil {
			t.Errorf("selected zone plan over-requested %s", variable)
		}
	}
	if output := findPurposeOutput(zone, "Output:Variable", "Office Ideal Loads", "Zone Ideal Loads Supply Air Latent Cooling Energy"); output == nil || output.ScopeZoneName != "Office" {
		t.Fatalf("selected Ideal Loads target = %#v", output)
	}
	if findPurposeOutput(zone, "Output:Variable", "Lab Ideal Loads", "Zone Ideal Loads Supply Air Latent Cooling Energy") != nil ||
		findPurposeOutput(zone, "Output:Variable", "Office", "Zone Ideal Loads Supply Air Latent Cooling Energy") != nil {
		t.Fatalf("selected zone plan should use only its Ideal Loads object key")
	}
	for _, variable := range []string{"Surface Inside Face Convection Heat Transfer Energy", "Surface Average Face Conduction Heat Transfer Energy"} {
		for _, key := range []string{"Office Wall", "Office Wall Two", "Office Mass", "Office Shared Mass", "Office Direct Space Mass"} {
			if findPurposeOutput(zone, "Output:Variable", key, variable) == nil {
				t.Errorf("selected zone plan omitted %s / %s", key, variable)
			}
		}
		for _, key := range []string{"Lab Wall", "Lab Shared Mass", "Lab Space Lab Space Mass"} {
			if findPurposeOutput(zone, "Output:Variable", key, variable) != nil {
				t.Errorf("selected zone plan emitted unrelated %s / %s", key, variable)
			}
		}
	}
	if findPurposeOutput(zone, "Output:Meter", "Cooling:Electricity", "") == nil ||
		findPurposeOutput(zone, "Output:Variable", "*", "Cooling Coil Total Cooling Energy") == nil {
		t.Fatalf("selected zone plan dropped central HVAC energy/load outputs")
	}

	monthly, hourly := 0, 0
	for _, object := range zone.OutputObjects {
		if object.ObjectType != "Output:Variable" || object.KeyValue != "Office" || object.VariableName != "Zone Air Heat Balance Surface Convection Rate" {
			continue
		}
		if object.ReportingFrequency == "Monthly" && purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) {
			monthly++
		}
		if object.ReportingFrequency == "Hourly" && purposeIDsContain(object.PurposeIDs, SimulationPurposeZoneHeatFlow) {
			hourly++
		}
	}
	if monthly != 1 || hourly != 1 || zone.EstimatedFrames != 8760 {
		t.Fatalf("monthly Energy Path/hourly drilldown = %d/%d, frames=%d", monthly, hourly, zone.EstimatedFrames)
	}
}

func TestEPATH042InternalMassGeneratedKeysResolveToOwningZone(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	for _, test := range []struct {
		key      string
		zoneName string
	}{
		{key: "Office Mass", zoneName: "Office"},
		{key: "Office Direct Space Mass", zoneName: "Office"},
		{key: "Office Shared Mass", zoneName: "Office"},
		{key: "Lab Shared Mass", zoneName: "Lab"},
		{key: "Lab Space Lab Space Mass", zoneName: "Lab"},
	} {
		category, warning := context.SurfaceCategories.resolve(test.key)
		if warning != nil || category.ZoneName != test.zoneName || category.Category != energyDriverCategoryStorageOther {
			t.Errorf("InternalMass key %q = category %#v / warning %#v", test.key, category, warning)
		}
	}
	if _, warning := context.SurfaceCategories.resolve("Shared Mass"); warning == nil {
		t.Fatal("ZoneList InternalMass raw object name must not masquerade as an expanded EnergyPlus report key")
	}
}

func TestEPATH040And042SQLBuildsFourStagesAndStreamsSurfaceCategories(t *testing.T) {
	doc := parsePurposePlanFixture(t, energyPathScopeFixtureIDF)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	createEnergyPathContractSQL(t, path)

	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	var categoryNode *EnergyExplanationNode
	categoryCount := 0
	for index := range legacy.Nodes {
		node := &legacy.Nodes[index]
		if node.Level == "heat" && node.DriverCategory == energyDriverCategoryExteriorWalls {
			categoryNode = node
			categoryCount++
		}
	}
	if categoryCount != 1 || categoryNode == nil || categoryNode.RawValue != 3 || categoryNode.EffectiveValue != 3 || categoryNode.SignedValue != -3 || categoryNode.Value != 0 || categoryNode.AllocatedValue != 0 || !categoryNode.AllocationApplied {
		t.Fatalf("streamed exterior-wall category = count %d / node %#v", categoryCount, categoryNode)
	}
	if !stringSliceContains(categoryNode.SourceIDs, "sql-rdd-1") || !stringSliceContains(categoryNode.SourceIDs, "sql-rdd-2") || stringSliceContains(categoryNode.SourceIDs, "sql-rdd-3") {
		t.Fatalf("category source union should retain two raw energy sources and exclude rate duplicate: %#v", categoryNode.SourceIDs)
	}
	for _, sourceID := range []string{"sql-rdd-1", "sql-rdd-2", "sql-rdd-3"} {
		source := energyExplanationSourceByID(legacy.Sources, sourceID)
		if source == nil || source.DriverCategory != energyDriverCategoryExteriorWalls || source.ZoneName != "Office" {
			t.Errorf("raw surface source %q = %#v", sourceID, source)
		}
	}

	annualLoad := energyPathTestNode(legacy.Nodes, "load", "cooling", "")
	if annualLoad == nil || annualLoad.Value != 1488 || !stringSliceContains(annualLoad.SourceIDs, "sql-rdd-5") || stringSliceContains(annualLoad.SourceIDs, "sql-rdd-4") {
		t.Fatalf("annual load must sum the selected Monthly contribution source = %#v", annualLoad)
	}
	month := energyPathTestPeriod(legacy.Periods, "M1")
	if month == nil {
		t.Fatalf("Monthly Rate fallback did not create M1: %#v", legacy.Periods)
	}
	monthlyLoad := energyPathTestNode(month.Nodes, "load", "cooling", "")
	if monthlyLoad == nil || monthlyLoad.Value != 1488 {
		t.Fatalf("Monthly Rate fallback should survive Annual Energy = %#v", monthlyLoad)
	}
	if energyPathTestPeriod(legacy.Periods, "M12") != nil {
		t.Fatalf("Annual/RunPeriod data leaked into M12: %#v", legacy.Periods)
	}
	annualStorage := energyPathTestDriverCategoryNode(legacy.Nodes, energyDriverCategoryStorageOther)
	monthlyStorage := energyPathTestDriverCategoryNode(month.Nodes, energyDriverCategoryStorageOther)
	if annualStorage == nil || annualStorage.Value != 1488 || annualStorage.AllocatedValue != 1488 || !annualStorage.AllocationApplied ||
		monthlyStorage == nil || monthlyStorage.Value != 1488 || monthlyStorage.AllocatedValue != 1488 || !monthlyStorage.AllocationApplied {
		t.Fatalf("surface period selection = annual %#v / M1 %#v", annualStorage, monthlyStorage)
	}
	var rawStorage *EnergyExplanationNode
	for index := range legacy.Nodes {
		if stringSliceContains(legacy.Nodes[index].SourceIDs, "sql-rdd-9") {
			rawStorage = &legacy.Nodes[index]
			break
		}
	}
	if rawStorage == nil || rawStorage.RawValue != 3720 || stringSliceContains(rawStorage.SourceIDs, "sql-rdd-8") {
		t.Fatalf("cross-frequency raw surface provenance = %#v", rawStorage)
	}
	if source := energyExplanationSourceByID(legacy.Sources, "sql-rdd-10"); source == nil || source.ZoneName != "Office" || source.Name != "Zone Gas Equipment NaturalGas Energy" {
		t.Fatalf("current zone direct-use source metadata = %#v", source)
	}
	for _, node := range legacy.Nodes {
		if node.MeterHierarchyLevel == "zone_direct_use" {
			t.Fatalf("zone direct-use source must not alter frozen v1 accounting graph: %#v", node)
		}
	}
	if energyPathTestNode(legacy.Nodes, "energy", "", "cooling") == nil {
		t.Fatalf("annual end-use energy fallback was not preserved: %#v", legacy.Nodes)
	}

	result := UpgradeEnergyExplanationV1(legacy)
	levels := map[string]int{}
	for _, node := range result.Nodes {
		levels[node.Level]++
	}
	for _, level := range []string{"driver", "load", "end_use", "carrier"} {
		if levels[level] == 0 {
			t.Errorf("default Basic Energy annual v2 result missing %s stage: %#v", level, result.Nodes)
		}
	}
}

func energyPathTestNode(nodes []EnergyExplanationNode, level string, serviceKind string, endUse string) *EnergyExplanationNode {
	for index := range nodes {
		node := &nodes[index]
		if node.Level == level && (serviceKind == "" || node.ServiceKind == serviceKind) && (endUse == "" || node.EndUse == endUse) {
			return node
		}
	}
	return nil
}

func energyPathTestPeriod(periods []EnergyPeriod, id string) *EnergyPeriod {
	for index := range periods {
		if periods[index].ID == id {
			return &periods[index]
		}
	}
	return nil
}

func energyPathTestDriverCategoryNode(nodes []EnergyExplanationNode, category string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].Level == "heat" && nodes[index].DriverCategory == category {
			return &nodes[index]
		}
	}
	return nil
}

func createEnergyPathContractSQL(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE ReportDataDictionary (
			ReportDataDictionaryIndex INTEGER PRIMARY KEY,
			KeyValue TEXT,
			Name TEXT,
			Units TEXT,
			IsMeter INTEGER,
			ReportingFrequency TEXT,
			IndexGroup TEXT
		)`,
		`CREATE TABLE "Time" (
			TimeIndex INTEGER PRIMARY KEY,
			Month INTEGER,
			Day INTEGER,
			Hour INTEGER,
			Minute INTEGER
		)`,
		`CREATE TABLE ReportData (
			ReportDataIndex INTEGER PRIMARY KEY,
			TimeIndex INTEGER,
			ReportDataDictionaryIndex INTEGER,
			Value REAL
		)`,
		`INSERT INTO ReportDataDictionary VALUES
			(1, 'Office Wall', 'Surface Inside Face Convection Heat Transfer Energy', 'J', 0, 'Monthly', 'Surface'),
			(2, 'Office Wall Two', 'Surface Inside Face Convection Heat Transfer Energy', 'J', 0, 'Monthly', 'Surface'),
			(3, 'Office Wall', 'Surface Inside Face Convection Heat Transfer Rate', 'W', 0, 'Monthly', 'Surface'),
			(4, 'Office', 'Zone Air System Sensible Cooling Energy', 'J', 0, 'Annual', 'Zone'),
			(5, 'Office', 'Zone Air System Sensible Cooling Rate', 'W', 0, 'Monthly', 'Zone'),
			(6, '', 'Electricity:Facility', 'J', 1, 'Monthly', 'Meter'),
			(7, '', 'Cooling:Electricity', 'J', 1, 'Annual', 'Meter'),
			(8, 'Office Mass', 'Surface Inside Face Convection Heat Transfer Energy', 'J', 0, 'Annual', 'Surface'),
			(9, 'Office Mass', 'Surface Inside Face Convection Heat Transfer Rate', 'W', 0, 'Monthly', 'Surface'),
			(10, 'Office', 'Zone Gas Equipment NaturalGas Energy', 'J', 0, 'Monthly', 'Zone')`,
		`INSERT INTO "Time" VALUES
			(1, 1, 1, 1, 0),
			(2, 12, 31, 24, 0)`,
		`INSERT INTO ReportData VALUES
			(1, 1, 1, 3600000.0),
			(2, 1, 2, 7200000.0),
			(3, 1, 3, 1000.0),
			(4, 2, 4, 36000000.0),
			(5, 1, 5, 2000.0),
			(6, 1, 6, 36000000.0),
			(7, 2, 7, 18000000.0),
			(8, 2, 8, 14400000.0),
			(9, 1, 9, 5000.0),
			(10, 1, 10, 3600000.0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("energy path SQL fixture statement failed: %v\n%s", err, statement)
		}
	}
}

func TestEPATH040MonthlyRateUsesMonthIntervalAcrossInterleavedFrequencies(t *testing.T) {
	for _, explicitInterval := range []bool{true, false} {
		name := "frequency_fallback"
		if explicitInterval {
			name = "explicit_time_interval"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "eplusout.sql")
			createEnergyPathInterleavedRateSQL(t, path, explicitInterval)
			if explicitInterval {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				intervals, err := sqlTimeIntervalHours(db)
				db.Close()
				if err != nil || intervals[4] != 744 {
					t.Fatalf("explicit Monthly Time.Interval = %#v, err=%v", intervals, err)
				}
			}

			plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
			result, err := parseSimulationEnergyExplanationSQL(path, plan)
			if err != nil {
				t.Fatal(err)
			}
			annual := energyPathTestNode(result.Nodes, "load", "cooling", "")
			month := energyPathTestPeriod(result.Periods, "M1")
			if month == nil {
				t.Fatalf("missing M1 period: %#v", result.Periods)
			}
			monthly := energyPathTestNode(month.Nodes, "load", "cooling", "")
			if annual == nil || annual.Value != 744 || monthly == nil || monthly.Value != 744 {
				t.Fatalf("1 kW Monthly rate integration = annual %#v / M1 %#v", annual, monthly)
			}
		})
	}
}

func createEnergyPathInterleavedRateSQL(t *testing.T, path string, explicitInterval bool) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	timeTable := `CREATE TABLE "Time" (
		TimeIndex INTEGER PRIMARY KEY,
		Month INTEGER,
		Day INTEGER,
		Hour INTEGER,
		Minute INTEGER,
		IntervalType TEXT
	)`
	timeValues := `INSERT INTO "Time" VALUES
		(1, 1, 31, 23, 0, 'Hourly'),
		(2, 1, 31, 24, 0, 'Hourly'),
		(3, 1, 31, 24, 0, 'Daily'),
		(4, 1, 31, 24, 0, 'Monthly')`
	if explicitInterval {
		timeTable = `CREATE TABLE "Time" (
			TimeIndex INTEGER PRIMARY KEY,
			Month INTEGER,
			Day INTEGER,
			Hour INTEGER,
			Minute INTEGER,
			Interval INTEGER,
			IntervalType INTEGER
		)`
		timeValues = `INSERT INTO "Time" VALUES
			(1, 1, 31, 23, 0, 60, 1),
			(2, 1, 31, 24, 0, 60, 1),
			(3, 1, 31, 24, 0, 1440, 2),
			(4, 1, 31, 24, 0, 44640, 3)`
	}
	statements := []string{
		`CREATE TABLE ReportDataDictionary (
			ReportDataDictionaryIndex INTEGER PRIMARY KEY,
			KeyValue TEXT,
			Name TEXT,
			Units TEXT,
			IsMeter INTEGER,
			ReportingFrequency TEXT,
			IndexGroup TEXT
		)`,
		timeTable,
		`CREATE TABLE ReportData (
			ReportDataIndex INTEGER PRIMARY KEY,
			TimeIndex INTEGER,
			ReportDataDictionaryIndex INTEGER,
			Value REAL
		)`,
		`INSERT INTO ReportDataDictionary VALUES
			(1, 'Office', 'Zone Infiltration Sensible Heat Gain Rate', 'W', 0, 'Hourly', 'Zone'),
			(2, 'Office', 'Zone Ventilation Sensible Heat Gain Rate', 'W', 0, 'Daily', 'Zone'),
			(3, 'Office', 'Zone Air System Sensible Cooling Rate', 'W', 0, 'Monthly', 'Zone')`,
		timeValues,
		`INSERT INTO ReportData VALUES
			(1, 1, 1, 100.0),
			(2, 2, 1, 100.0),
			(3, 3, 2, 100.0),
			(4, 4, 3, 1000.0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("interleaved Energy Path SQL fixture failed: %v\n%s", err, statement)
		}
	}
}

func TestEPATH041LargeModelWeightCountsMonthlySurfaceKeys(t *testing.T) {
	var source strings.Builder
	source.WriteString("Version, 24.1;\nZone, Office;\n")
	const surfaceCount = 850
	for index := 0; index < surfaceCount; index++ {
		fmt.Fprintf(&source, "BuildingSurface:Detailed, Wall %04d, Wall, , Office, , Outdoors, , SunExposed, WindExposed, , 4, 0,0,0, 1,0,0, 1,0,1, 0,0,1;\n", index)
	}
	doc := parsePurposePlanFixture(t, source.String())
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})

	surfaceSeries := 0
	for _, object := range plan.OutputObjects {
		if object.ObjectType == "Output:Variable" && strings.HasPrefix(object.KeyValue, "Wall ") && strings.Contains(object.VariableName, "Surface Inside Face Convection") {
			surfaceSeries++
		}
	}
	if surfaceSeries != surfaceCount*4 {
		t.Fatalf("monthly surface series = %d, want %d", surfaceSeries, surfaceCount*4)
	}
	if plan.EstimatedFrames != 12 || plan.EstimatedSeries < surfaceSeries || plan.EstimatedWeight != "Medium" {
		t.Fatalf("large Energy Path estimate = weight %q / series %d / frames %d", plan.EstimatedWeight, plan.EstimatedSeries, plan.EstimatedFrames)
	}
}
