package idf

import (
	"math"
	"testing"
)

func TestProfileAirflowUsesEffectiveZoneBasisAndExteriorWallArea(t *testing.T) {
	doc, err := Parse(`
Zone,
  Test Zone,                !- Name
  0,                        !- Direction of Relative North
  0,                        !- X Origin
  0,                        !- Y Origin
  0,                        !- Z Origin
  1,                        !- Type
  2,                        !- Multiplier
  3,                        !- Ceiling Height
  300;                      !- Volume

BuildingSurface:Detailed,
  Test Floor,               !- Name
  Floor,                    !- Surface Type
  Floor Construction,       !- Construction Name
  Test Zone,                !- Zone Name
  Ground,                   !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  NoSun,                    !- Sun Exposure
  NoWind,                   !- Wind Exposure
  0.5,                      !- View Factor to Ground
  4,                        !- Number of Vertices
  0, 0, 0,
  10, 0, 0,
  10, 10, 0,
  0, 10, 0;

BuildingSurface:Detailed,
  Test Wall,                !- Name
  Wall,                     !- Surface Type
  Wall Construction,        !- Construction Name
  Test Zone,                !- Zone Name
  Outdoors,                 !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  SunExposed,               !- Sun Exposure
  WindExposed,              !- Wind Exposure
  0.5,                      !- View Factor to Ground
  4,                        !- Number of Vertices
  0, 0, 0,
  10, 0, 0,
  10, 0, 3,
  0, 0, 3;

BuildingSurface:Detailed,
  Test Roof,                !- Name
  Roof,                     !- Surface Type
  Roof Construction,        !- Construction Name
  Test Zone,                !- Zone Name
  Outdoors,                 !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  SunExposed,               !- Sun Exposure
  WindExposed,              !- Wind Exposure
  0.5,                      !- View Factor to Ground
  4,                        !- Number of Vertices
  0, 0, 3,
  0, 10, 3,
  10, 10, 3,
  10, 0, 3;

ZoneInfiltration:DesignFlowRate,
  Flow Zone,                !- Name
  Test Zone,                !- Zone or ZoneList or Space or SpaceList Name
  ,                         !- Schedule Name
  Flow/Zone,                !- Design Flow Rate Calculation Method
  0.1;                      !- Design Flow Rate

ZoneInfiltration:DesignFlowRate,
  ACH Input,                !- Name
  Test Zone,                !- Zone or ZoneList or Space or SpaceList Name
  ,                         !- Schedule Name
  AirChanges/Hour,          !- Design Flow Rate Calculation Method
  ,                         !- Design Flow Rate
  ,                         !- Flow Rate per Floor Area
  ,                         !- Flow Rate per Exterior Surface Area
  0.5;                      !- Air Changes per Hour

ZoneInfiltration:DesignFlowRate,
  Wall Input,               !- Name
  Test Zone,                !- Zone or ZoneList or Space or SpaceList Name
  ,                         !- Schedule Name
  Flow/ExteriorWallArea,    !- Design Flow Rate Calculation Method
  ,                         !- Design Flow Rate
  ,                         !- Flow Rate per Floor Area
  0.001;                    !- Flow Rate per Exterior Surface Area
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := AnalyzeProfile(doc)
	zone := profileZoneByName(t, report, "Test Zone")
	if !closeProfileValue(zone.FloorArea, 100, 1e-9) || !closeProfileValue(zone.Volume, 300, 1e-9) {
		t.Fatalf("representative zone geometry = area %v, volume %v; want 100 m2 and 300 m3", zone.FloorArea, zone.Volume)
	}

	flowZone := profileItemByName(t, zone, "Flow Zone")
	assertProfileMetricValue(t, flowZone, "flow", 0.1, 1e-9)
	assertProfileMetricValue(t, flowZone, "ach", 1.2, 1e-9)

	achInput := profileItemByName(t, zone, "ACH Input")
	assertProfileMetricValue(t, achInput, "flow", 1.0/24.0, 1e-4)
	assertProfileMetricValue(t, achInput, "ach", 0.5, 1e-9)

	wallInput := profileItemByName(t, zone, "Wall Input")
	// A representative profile is invariant to Zone Multiplier. The physical
	// wall is 30 m2, and the 100 m2 roof must not be included.
	assertProfileMetricValue(t, wallInput, "flow", 0.03, 1e-9)
}

func TestProfileOutdoorAirSupportsSumMaximumAndEnergyPlusDefaults(t *testing.T) {
	doc, err := Parse(`
Zone,
  OA Zone,                  !- Name
  0,                        !- Direction of Relative North
  0,                        !- X Origin
  0,                        !- Y Origin
  0,                        !- Z Origin
  1,                        !- Type
  1,                        !- Multiplier
  3,                        !- Ceiling Height
  300;                      !- Volume

BuildingSurface:Detailed,
  OA Floor,                 !- Name
  Floor,                    !- Surface Type
  Floor Construction,       !- Construction Name
  OA Zone,                  !- Zone Name
  Ground,                   !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  NoSun,                    !- Sun Exposure
  NoWind,                   !- Wind Exposure
  0.5,                      !- View Factor to Ground
  4,                        !- Number of Vertices
  0, 0, 0,
  10, 0, 0,
  10, 10, 0,
  0, 10, 0;

People,
  OA People,                !- Name
  OA Zone,                  !- Zone or ZoneList or Space or SpaceList Name
  ,                         !- Number of People Schedule Name
  People/Area,              !- Number of People Calculation Method
  ,                         !- Number of People
  0.1;                      !- People per Floor Area

DesignSpecification:OutdoorAir,
  OA Sum,                   !- Name
  Sum,                      !- Outdoor Air Method
  0.01,                     !- Outdoor Air Flow per Person
  0.001,                    !- Outdoor Air Flow per Zone Floor Area
  0.05,                     !- Outdoor Air Flow per Zone
  0.6;                      !- Outdoor Air Flow Air Changes per Hour

DesignSpecification:OutdoorAir,
  OA Maximum,               !- Name
  Maximum,                  !- Outdoor Air Method
  0.01,                     !- Outdoor Air Flow per Person
  0.001,                    !- Outdoor Air Flow per Zone Floor Area
  0.05,                     !- Outdoor Air Flow per Zone
  0.6;                      !- Outdoor Air Flow Air Changes per Hour

DesignSpecification:OutdoorAir,
  OA Default,               !- Name
  ;                         !- Outdoor Air Method

Sizing:Zone,
  OA Zone,                  !- Zone or ZoneList Name
  ,                         !- Zone Cooling Design Supply Air Temperature Input Method
  ,                         !- Zone Cooling Design Supply Air Temperature
  ,                         !- Zone Cooling Design Supply Air Temperature Difference
  ,                         !- Zone Heating Design Supply Air Temperature Input Method
  ,                         !- Zone Heating Design Supply Air Temperature
  ,                         !- Zone Heating Design Supply Air Temperature Difference
  ,                         !- Zone Cooling Design Supply Air Humidity Ratio
  ,                         !- Zone Heating Design Supply Air Humidity Ratio
  OA Sum;                   !- Design Specification Outdoor Air Object Name

Sizing:Zone,
  OA Zone,                  !- Zone or ZoneList Name
  , , , , , , , ,
  OA Maximum;               !- Design Specification Outdoor Air Object Name

Sizing:Zone,
  OA Zone,                  !- Zone or ZoneList Name
  , , , , , , , ,
  OA Default;               !- Design Specification Outdoor Air Object Name
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	zone := profileZoneByName(t, AnalyzeProfile(doc), "OA Zone")
	assertProfileMetricValue(t, profileItemByName(t, zone, "OA Sum"), "flow", 0.3, 5e-4)
	assertProfileMetricValue(t, profileItemByName(t, zone, "OA Maximum"), "flow", 0.1, 1e-9)
	assertProfileMetricValue(t, profileItemByName(t, zone, "OA Default"), "flow", 0.0944, 1e-9)
}

func TestProfileMarksWeatherDependentAirflowAsUnresolved(t *testing.T) {
	doc, err := Parse(`
Zone,
  Weather Zone,             !- Name
  0, 0, 0, 0, 1, 1, 3, 300;

ZoneInfiltration:EffectiveLeakageArea,
  Leakage,                  !- Name
  Weather Zone,             !- Zone or Space Name
  ,                         !- Schedule Name
  500,                      !- Effective Air Leakage Area
  0.000145,                 !- Stack Coefficient
  0.000174;                 !- Wind Coefficient

ZoneInfiltration:FlowCoefficient,
  Crack,                    !- Name
  Weather Zone,             !- Zone or Space Name
  ,                         !- Schedule Name
  0.05,                     !- Flow Coefficient
  0.089,                    !- Stack Coefficient
  0.67,                     !- Pressure Exponent
  0.156,                    !- Wind Coefficient
  0.9;                      !- Shelter Factor

ZoneVentilation:WindandStackOpenArea,
  Window Ventilation,       !- Name
  Weather Zone,             !- Zone or Space Name
  2.0;                      !- Opening Area
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	zone := profileZoneByName(t, AnalyzeProfile(doc), "Weather Zone")
	for _, objectName := range []string{"Leakage", "Crack", "Window Ventilation"} {
		item := profileItemByName(t, zone, objectName)
		metric := selectProfileMetric(item.Normalized, "flow")
		if metric.Status != metricStatusMissing {
			t.Fatalf("%s flow status = %q, want missing", objectName, metric.Status)
		}
		found := false
		for _, warning := range item.Warnings {
			found = found || warning.Code == "weather_dependent_airflow"
		}
		if !found {
			t.Fatalf("%s warnings = %#v, want weather_dependent_airflow", objectName, item.Warnings)
		}
		sourceMetricID := map[string]string{
			"Leakage":            "effective_leakage_area",
			"Crack":              "flow_coefficient",
			"Window Ventilation": "opening_area",
		}[objectName]
		sourceMetric := selectProfileMetric(item.Normalized, sourceMetricID)
		wantSourceStatus := metricStatusOK
		if objectName == "Window Ventilation" {
			wantSourceStatus = metricStatusPartial
		}
		if sourceMetric.Status != wantSourceStatus {
			t.Fatalf("%s source metric = %#v, want %s model input", objectName, sourceMetric, wantSourceStatus)
		}
		if objectName == "Crack" && (sourceMetric.Unit != "m3/s-Pa^n" || !closeProfileValue(sourceMetric.Value, 0.05, 1e-12)) {
			t.Fatalf("flow coefficient source metric = %#v, want 0.05 m3/s-Pa^n", sourceMetric)
		}
	}
}

func TestProfileDoesNotSumIncompatibleWeatherModelParameters(t *testing.T) {
	doc, err := Parse(`
Zone, Weather Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
ZoneInfiltration:FlowCoefficient, Crack A, Weather Zone, , 0.05, 0.089, 0.67, 0.156, 0.9;
ZoneInfiltration:FlowCoefficient, Crack B, Weather Zone, , 0.03, 0.089, 0.50, 0.156, 0.9;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := profileZoneByName(t, AnalyzeProfile(doc), "Weather Zone")
	dimension := profileDimensionSummary(zone.Dimensions, ProfileDimensionInfiltration)
	if dimension == nil || dimension.Status != metricStatusMissing || dimension.DisplayValue != "—" {
		t.Fatalf("incompatible flow coefficients = %#v, want unavailable aggregate", dimension)
	}
}

func TestProfileWeatherModelSignaturesUseCompleteEnergyPlusBasis(t *testing.T) {
	doc, err := Parse(`
Zone, Weather Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
ZoneInfiltration:FlowCoefficient, Default Exponent, Weather Zone, , 0.05, 0.089, , 0.156, 0.9;
ZoneInfiltration:FlowCoefficient, Explicit Exponent, Weather Zone, , 0.03, 0.089, 0.67, 0.156, 0.9;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := profileZoneByName(t, AnalyzeProfile(doc), "Weather Zone")
	dimension := profileDimensionSummary(zone.Dimensions, ProfileDimensionInfiltration)
	if dimension == nil || dimension.MetricID != "flow_coefficient" || dimension.Status != metricStatusOK {
		t.Fatalf("compatible default/explicit exponent aggregate = %#v, want resolved flow coefficient", dimension)
	}
	if !closeProfileValue(dimension.Value, 0.08, 1e-12) {
		t.Fatalf("flow coefficient aggregate = %v, want 0.08", dimension.Value)
	}

	doc, err = Parse(`
Zone, Weather Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
ZoneInfiltration:FlowCoefficient, Stack A, Weather Zone, , 0.05, 0.089, 0.67, 0.156, 0.9;
ZoneInfiltration:FlowCoefficient, Stack B, Weather Zone, , 0.03, 0.078, 0.67, 0.156, 0.9;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone = profileZoneByName(t, AnalyzeProfile(doc), "Weather Zone")
	dimension = profileDimensionSummary(zone.Dimensions, ProfileDimensionInfiltration)
	if dimension == nil || dimension.Status != metricStatusMissing {
		t.Fatalf("different stack-coefficient aggregate = %#v, want unavailable", dimension)
	}
}

func TestProfileMarksIncompleteWeatherModelsPartial(t *testing.T) {
	doc, err := Parse(`
Zone, Weather Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
ZoneInfiltration:EffectiveLeakageArea, Incomplete ELA, Weather Zone, , 500;
ZoneInfiltration:FlowCoefficient, Incomplete Crack, Weather Zone, , 0.05, 0.089, , 0.156;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := profileZoneByName(t, AnalyzeProfile(doc), "Weather Zone")
	for _, testCase := range []struct {
		name     string
		metricID string
	}{
		{name: "Incomplete ELA", metricID: "effective_leakage_area"},
		{name: "Incomplete Crack", metricID: "flow_coefficient"},
	} {
		item := profileItemByName(t, zone, testCase.name)
		metric := selectProfileMetric(item.Normalized, testCase.metricID)
		if metric.Status != metricStatusPartial {
			t.Fatalf("%s source metric = %#v, want partial", testCase.name, metric)
		}
		found := false
		for _, warning := range item.Warnings {
			found = found || warning.Code == "incomplete_weather_airflow_model"
		}
		if !found {
			t.Fatalf("%s warnings = %#v, want incomplete_weather_airflow_model", testCase.name, item.Warnings)
		}
	}
}

func TestProfileWindAndStackAggregationUsesModelAndControlSignature(t *testing.T) {
	doc, err := Parse(`
Zone, Weather Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
ZoneVentilation:WindandStackOpenArea, North Opening, Weather Zone, 1, , Autocalculate, 0, 1, Autocalculate;
ZoneVentilation:WindandStackOpenArea, East Opening, Weather Zone, 1, , Autocalculate, 90, 1, Autocalculate;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := profileZoneByName(t, AnalyzeProfile(doc), "Weather Zone")
	dimension := profileDimensionSummary(zone.Dimensions, ProfileDimensionVentilation)
	if dimension == nil || dimension.Status != metricStatusMissing {
		t.Fatalf("differently oriented openings = %#v, want unavailable equivalent opening aggregate", dimension)
	}
}

func TestProfileDesignFlowWeatherModifiersAreNominalPartial(t *testing.T) {
	doc, err := Parse(`
Zone, Weather Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
ZoneInfiltration:DesignFlowRate,
  Weather Infiltration, Weather Zone, , Flow/Zone,
  0.1, , , , 0.606, 0.03636, 0.1177, 0;
ZoneVentilation:DesignFlowRate,
  Weather Ventilation, Weather Zone, , Flow/Zone,
  0.2, , , , Natural, , , 0.606, 0.02, 0.000598, 0;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	report := AnalyzeProfile(doc)
	zone := profileZoneByName(t, report, "Weather Zone")
	for _, name := range []string{"Weather Infiltration", "Weather Ventilation"} {
		item := profileItemByName(t, zone, name)
		if metric := selectProfileMetric(item.Normalized, "flow"); metric.Status != metricStatusPartial {
			t.Fatalf("%s flow metric = %#v, want nominal partial", name, metric)
		}
		found := false
		for _, warning := range item.Warnings {
			found = found || warning.Code == "weather_modified_design_flow_basis"
		}
		if !found {
			t.Fatalf("%s warnings = %#v, want weather_modified_design_flow_basis", name, item.Warnings)
		}
	}
	for _, dimensionID := range []string{ProfileDimensionInfiltration, ProfileDimensionVentilation} {
		dimension := profileDimensionSummary(zone.Dimensions, dimensionID)
		if dimension == nil || dimension.Status != metricStatusPartial {
			t.Fatalf("%s dimension = %#v, want partial", dimensionID, dimension)
		}
		series := findProfileSeries(report.GraphDataset.Series, "zone", "Weather Zone", dimensionID)
		if series == nil || series.Status != metricStatusPartial {
			t.Fatalf("%s graph series = %#v, want partial", dimensionID, series)
		}
	}
}

func TestProfileOutdoorAirResolvesMechanicalVentilationAndSpaceList(t *testing.T) {
	doc, err := Parse(`
Zone,
  Space Zone,               !- Name
  0, 0, 0, 0, 1, 1, 3, 300;

Space,
  Office Space,             !- Name
  Space Zone,               !- Zone Name
  ,                         !- Ceiling Height
  180,                      !- Volume
  60;                       !- Floor Area

Space,
  Meeting Space,            !- Name
  Space Zone,               !- Zone Name
  ,                         !- Ceiling Height
  120,                      !- Volume
  40;                       !- Floor Area

People,
  Office People,            !- Name
  Office Space,             !- Zone or ZoneList or Space or SpaceList Name
  ,                         !- Number of People Schedule Name
  People,                   !- Number of People Calculation Method
  6;                        !- Number of People

People,
  Meeting People,           !- Name
  Meeting Space,            !- Zone or ZoneList or Space or SpaceList Name
  ,                         !- Number of People Schedule Name
  People,                   !- Number of People Calculation Method
  8;                        !- Number of People

DesignSpecification:OutdoorAir,
  Office OA,                !- Name
  Sum,                      !- Outdoor Air Method
  0.01,                     !- Outdoor Air Flow per Person
  0.001;                    !- Outdoor Air Flow per Zone Floor Area

DesignSpecification:OutdoorAir,
  Meeting OA,               !- Name
  Flow/Area,                !- Outdoor Air Method
  ,                         !- Outdoor Air Flow per Person
  0.002;                    !- Outdoor Air Flow per Zone Floor Area

DesignSpecification:OutdoorAir:SpaceList,
  Space OA List,            !- Name
  Office Space,             !- Space 1 Name
  Office OA,                !- Space 1 Design Specification Outdoor Air Object Name
  Meeting Space,            !- Space 2 Name
  Meeting OA;               !- Space 2 Design Specification Outdoor Air Object Name

Controller:MechanicalVentilation,
  Mechanical Ventilation,   !- Name
  ,                         !- Availability Schedule Name
  No,                       !- Demand Controlled Ventilation
  ZoneSum,                  !- System Outdoor Air Method
  1.0,                      !- Zone Maximum Outdoor Air Fraction
  Space Zone,               !- Zone or ZoneList 1 Name
  Space OA List,            !- Design Specification Outdoor Air Object Name 1
  ;                         !- Design Specification Zone Air Distribution Object Name 1
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	zone := profileZoneByName(t, AnalyzeProfile(doc), "Space Zone")
	// Office OA uses only Office Space: 6*0.01 + 60*0.001 = 0.12 m3/s.
	assertProfileMetricValue(t, profileItemByName(t, zone, "Office OA"), "flow", 0.12, 5e-5)
	// Meeting OA uses only Meeting Space: 40*0.002 = 0.08 m3/s.
	assertProfileMetricValue(t, profileItemByName(t, zone, "Meeting OA"), "flow", 0.08, 5e-5)
}

func TestProfileOutdoorAirDoesNotHideUnresolvedPeople(t *testing.T) {
	doc, err := Parse(`
Zone, Test Zone, 0, 0, 0, 1, 1, 3, 0, 0;
People, Broken People, Test Zone, , People/Area, , 0.1;
DesignSpecification:OutdoorAir, Test OA, Flow/Person, 0.01;
Sizing:Zone, Test Zone, , , , , , , , , Test OA;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := profileZoneByName(t, AnalyzeProfile(doc), "Test Zone")
	item := profileItemByName(t, zone, "Test OA")
	flow := selectProfileMetric(item.Normalized, "flow")
	if flow.Status != metricStatusMissing {
		t.Fatalf("outdoor air flow = %#v, want missing because configured People is unresolved", flow)
	}
	if !profileWarningCode(item.Warnings, "missing_people_reference") {
		t.Fatalf("outdoor air warnings = %#v, want missing_people_reference", item.Warnings)
	}
}

func TestProfileOutdoorAirResolvesDirectZoneEquipmentReference(t *testing.T) {
	doc, err := Parse(`
Zone, Test Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
People, Test People, Test Zone, , People, 10;
DesignSpecification:OutdoorAir, Test OA, Flow/Person, 0.01;
ZoneHVAC:IdealLoadsAirSystem,
  Ideal Loads,              !- Name
  ,                         !- Availability Schedule Name
  Supply Node,              !- Zone Supply Air Node Name
  Exhaust Node,             !- Zone Exhaust Air Node Name
  Inlet Node,               !- System Inlet Air Node Name
  , , , , , , , , , , , , , , ,
  Test OA;                  !- Design Specification Outdoor Air Object Name
ZoneHVAC:EquipmentList,
  Test Equipment, SequentialLoad,
  ZoneHVAC:IdealLoadsAirSystem, Ideal Loads, 1, 1, , ;
ZoneHVAC:EquipmentConnections,
  Test Zone, Test Equipment, Supply Node, Exhaust Node, Zone Air Node, Return Node;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := profileZoneByName(t, AnalyzeProfile(doc), "Test Zone")
	assertProfileMetricValue(t, profileItemByName(t, zone, "Test OA"), "flow", 0.1, 1e-12)
}

func TestProfileOutdoorAirDirectSpaceEquipmentUsesSpaceBasis(t *testing.T) {
	doc, err := Parse(`
Zone, Test Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Space, Office Space, Test Zone, 3, 75, 25;
Space, Support Space, Test Zone, 3, 225, 75;
People, Office People, Office Space, , People, 5;
DesignSpecification:OutdoorAir, Test OA, Sum, 0.01, 0.001;
ZoneHVAC:IdealLoadsAirSystem,
  Ideal Loads, , Supply Node, Exhaust Node, Inlet Node,
  , , , , , , , , , , , , , , ,
  Test OA;
SpaceHVAC:EquipmentList,
  Space Equipment, SequentialLoad,
  AirTerminal:SingleDuct:ConstantVolume:NoReheat, Office Terminal, 1, 1, , ;
AirTerminal:SingleDuct:ConstantVolume:NoReheat,
  Office Terminal, , Inlet Node, Supply Node, Autosize, Test OA, DesignOccupancy;
SpaceHVAC:EquipmentConnections,
  Office Space, Space Equipment, Supply Node, Exhaust Node, Space Air Node, Return Node;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := profileZoneByName(t, AnalyzeProfile(doc), "Test Zone")
	item := profileItemByName(t, zone, "Test OA")
	assertProfileMetricValue(t, item, "flow", 0.075, 1e-12)
	if item.SourceTargetKind != "Space" || item.SourceTarget != "Office Space" {
		t.Fatalf("OA source target = %q %q, want Office Space", item.SourceTargetKind, item.SourceTarget)
	}
}

func TestProfileZoneVentilationRepeatsDirectInputsForEachSpace(t *testing.T) {
	doc, err := Parse(`
Zone, Multi Space Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Space, Office A, Multi Space Zone, 3, 120, 40;
Space, Office B, Multi Space Zone, 3, 180, 60;
ZoneVentilation:DesignFlowRate,
  Direct Ventilation, Multi Space Zone, , Flow/Zone, 0.2;
ZoneVentilation:WindandStackOpenArea,
  Operable Windows, Multi Space Zone, 2, , Autocalculate, 90, 1.5, Autocalculate;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	zone := profileZoneByName(t, AnalyzeProfile(doc), "Multi Space Zone")
	direct := profileItemByName(t, zone, "Direct Ventilation")
	flow := selectProfileMetric(direct.Normalized, "flow")
	if flow.Status == metricStatusMissing || !closeProfileValue(flow.Value, 0.4, 1e-12) {
		t.Fatalf("Zone Flow/Zone expansion = %#v, want 0.2 m3/s repeated for two Spaces", flow)
	}
	windows := profileItemByName(t, zone, "Operable Windows")
	opening := selectProfileMetric(windows.Normalized, "opening_area")
	if opening.Status != metricStatusPartial || !closeProfileValue(opening.Value, 4, 1e-12) {
		t.Fatalf("Zone WindAndStack opening expansion = %#v, want 2 m2 repeated for two Spaces and marked operational partial", opening)
	}
}

func profileWarningCode(warnings []ProfileWarning, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

func profileZoneByName(t *testing.T, report ProfileReport, name string) ZoneProfile {
	t.Helper()
	for _, zone := range report.ZoneProfiles {
		if zone.ZoneName == name {
			return zone
		}
	}
	t.Fatalf("zone %q not found in %#v", name, report.ZoneProfiles)
	return ZoneProfile{}
}

func profileItemByName(t *testing.T, zone ZoneProfile, name string) ProfileItem {
	t.Helper()
	for _, item := range zone.Items {
		if item.ObjectName == name {
			return item
		}
	}
	t.Fatalf("profile item %q not found in zone %q: %#v", name, zone.ZoneName, zone.Items)
	return ProfileItem{}
}

func assertProfileMetricValue(t *testing.T, item ProfileItem, metricID string, want, tolerance float64) {
	t.Helper()
	metric := selectProfileMetric(item.Normalized, metricID)
	if metric.Status != metricStatusOK || !closeProfileValue(metric.Value, want, tolerance) {
		t.Fatalf("%s %s = %#v, want %v +/- %v", item.ObjectName, metricID, metric, want, tolerance)
	}
}

func closeProfileValue(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance
}
