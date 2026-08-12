package idf

import (
	"fmt"
	"strings"
	"testing"
)

// nominal_outdoor_air_profile means that the plotted time series is a
// nominal/design outdoor-air requirement, not an EnergyPlus-exact operating
// flow.  In particular, a DesignSpecification:OutdoorAir per-person component
// can depend on the owner's CurrentOccupancy/DesignOccupancy mode, the People
// schedule, mechanical-ventilation control method, or IAQ state.  Until those
// operating inputs are evaluated together, a numeric curve must remain partial.
const nominalOutdoorAirProfileWarningCode = "nominal_outdoor_air_profile"

func TestProfileActualTimeSeriesMarksNominalOutdoorAirMethodsPartial(t *testing.T) {
	testCases := []struct {
		name   string
		method string
	}{
		{name: "flow per current occupant", method: "Flow/Person"},
		{name: "sum with current-occupancy component", method: "Sum"},
		{name: "maximum with current-occupancy component", method: "Maximum"},
		{name: "proportional by occupancy schedule", method: "ProportionalControlBasedOnOccupancySchedule"},
		{name: "proportional by design occupancy", method: "ProportionalControlBasedOnDesignOccupancy"},
		{name: "indoor air quality procedure", method: "IndoorAirQualityProcedure"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			doc, err := Parse(fmt.Sprintf(`
Zone, OA Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Space, OA Space, OA Zone, 3, 300, 100;
Schedule:Constant, Occupancy Fraction, , 0.25;
Schedule:Constant, OA Fraction, , 0.80;
People,
  OA People,                !- Name
  OA Space,                 !- Zone or ZoneList or Space or SpaceList Name
  Occupancy Fraction,       !- Number of People Schedule Name
  People,                   !- Number of People Calculation Method
  10;                       !- Number of People
DesignSpecification:OutdoorAir,
  Test OA,                  !- Name
  %s,                       !- Outdoor Air Method
  0.01,                     !- Outdoor Air Flow per Person
  0.0001,                   !- Outdoor Air Flow per Zone Floor Area
  0.002,                    !- Outdoor Air Flow per Zone
  0.01,                     !- Outdoor Air Flow Air Changes per Hour
  OA Fraction;              !- Outdoor Air Flow Rate Fraction Schedule Name
SpaceHVAC:EquipmentList,
  OA Space Equipment, SequentialLoad,
  AirTerminal:SingleDuct:ConstantVolume:NoReheat, OA Terminal, 1, 1, , ;
AirTerminal:SingleDuct:ConstantVolume:NoReheat,
  OA Terminal,              !- Name
  ,                         !- Availability Schedule Name
  OA Inlet Node,            !- Air Inlet Node Name
  OA Supply Node,           !- Air Outlet Node Name
  Autosize,                 !- Maximum Air Flow Rate
  Test OA,                  !- Design Specification Outdoor Air Object Name
  CurrentOccupancy;         !- Per Person Ventilation Rate Mode
SpaceHVAC:EquipmentConnections,
  OA Space, OA Space Equipment, OA Supply Node, OA Exhaust Node, OA Space Air Node, OA Return Node;
`, testCase.method))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			report := AnalyzeProfile(doc)
			zone := profileZoneByName(t, report, "OA Zone")
			item := profileItemByName(t, zone, "Test OA")
			if flow := selectProfileMetric(item.Normalized, "flow"); flow.Status == metricStatusMissing || flow.Value <= 0 {
				t.Fatalf("%s nominal outdoor-air flow = %#v, want a usable design value", testCase.method, flow)
			}
			series := findProfileSeries(report.GraphDataset.Series, "zone", "OA Zone", ProfileDimensionOutdoorAir)
			if series == nil {
				t.Fatalf("%s outdoor-air graph series is missing", testCase.method)
			}
			if series.Status != metricStatusPartial {
				t.Fatalf("%s graph status = %q, want partial because the curve does not evaluate actual occupancy/owner control", testCase.method, series.Status)
			}
			warning := actualFidelityWarning(series.Warnings, nominalOutdoorAirProfileWarningCode)
			if warning == nil {
				t.Fatalf("%s graph warnings = %#v, want %s", testCase.method, series.Warnings, nominalOutdoorAirProfileWarningCode)
			}
			message := strings.ToLower(warning.Message)
			if warning.Severity != "warning" || !strings.Contains(message, "nominal") || !strings.Contains(message, "actual") {
				t.Fatalf("%s warning = %#v, want warning severity explaining nominal versus actual operation", testCase.method, warning)
			}
		})
	}
}

// weather_modified_design_flow_basis means that the displayed/graphable value
// is V_design (and its availability schedule), while EnergyPlus can gate or
// modify actual ZoneVentilation:DesignFlowRate using temperature and wind
// controls.  A valid control is therefore not an error, but it makes an
// unsimulated Actual Time Profile partial.
func TestProfileActualTimeSeriesMarksVentilationControlsPartial(t *testing.T) {
	testCases := []struct {
		name                       string
		minimumIndoorTemperature   string
		minimumIndoorScheduleName  string
		maximumWindSpeed           string
		expectedControlDescription string
	}{
		{
			name:                       "non-default temperature limit",
			minimumIndoorTemperature:   "18",
			maximumWindSpeed:           "40",
			expectedControlDescription: "Minimum Indoor Temperature",
		},
		{
			name:                       "temperature control schedule",
			minimumIndoorScheduleName:  "Minimum Indoor Limit",
			maximumWindSpeed:           "40",
			expectedControlDescription: "Minimum Indoor Temperature Schedule Name",
		},
		{
			name:                       "non-default wind limit",
			maximumWindSpeed:           "5",
			expectedControlDescription: "Maximum Wind Speed",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			doc, err := Parse(fmt.Sprintf(`
Zone, Control Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Schedule:Constant, Ventilation Availability, , 1;
Schedule:Constant, Minimum Indoor Limit, , 18;
ZoneVentilation:DesignFlowRate,
  Controlled Ventilation,     !- Name
  Control Zone,               !- Zone or ZoneList or Space or SpaceList Name
  Ventilation Availability,   !- Schedule Name
  Flow/Zone,                  !- Design Flow Rate Calculation Method
  0.20,                       !- Design Flow Rate
  ,                           !- Flow Rate per Floor Area
  ,                           !- Flow Rate per Person
  ,                           !- Air Changes per Hour
  Natural,                    !- Ventilation Type
  ,                           !- Fan Pressure Rise
  ,                           !- Fan Total Efficiency
  1,                          !- Constant Term Coefficient
  0,                          !- Temperature Term Coefficient
  0,                          !- Velocity Term Coefficient
  0,                          !- Velocity Squared Term Coefficient
  %s,                         !- Minimum Indoor Temperature
  %s,                         !- Minimum Indoor Temperature Schedule Name
  ,                           !- Maximum Indoor Temperature
  ,                           !- Maximum Indoor Temperature Schedule Name
  ,                           !- Delta Temperature
  ,                           !- Delta Temperature Schedule Name
  ,                           !- Minimum Outdoor Temperature
  ,                           !- Minimum Outdoor Temperature Schedule Name
  ,                           !- Maximum Outdoor Temperature
  ,                           !- Maximum Outdoor Temperature Schedule Name
  %s,                         !- Maximum Wind Speed
  Outdoor;                    !- Density Basis
`, testCase.minimumIndoorTemperature, testCase.minimumIndoorScheduleName, testCase.maximumWindSpeed))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			report := AnalyzeProfile(doc)
			zone := profileZoneByName(t, report, "Control Zone")
			item := profileItemByName(t, zone, "Controlled Ventilation")
			flow := selectProfileMetric(item.Normalized, "flow")
			if flow.Status != metricStatusPartial {
				t.Fatalf("flow metric = %#v, want nominal design value marked partial for %s", flow, testCase.expectedControlDescription)
			}
			dimension := profileDimensionSummary(zone.Dimensions, ProfileDimensionVentilation)
			if dimension == nil || dimension.Status != metricStatusPartial {
				t.Fatalf("ventilation dimension = %#v, want partial for %s", dimension, testCase.expectedControlDescription)
			}
			series := findProfileSeries(report.GraphDataset.Series, "zone", "Control Zone", ProfileDimensionVentilation)
			if series == nil || series.Status != metricStatusPartial {
				t.Fatalf("ventilation graph series = %#v, want partial for %s", series, testCase.expectedControlDescription)
			}
			warning := actualFidelityWarning(series.Warnings, "weather_modified_design_flow_basis")
			if warning == nil {
				t.Fatalf("graph warnings = %#v, want weather_modified_design_flow_basis for %s", series.Warnings, testCase.expectedControlDescription)
			}
			if warning.Severity != "warning" || !strings.Contains(warning.Message, testCase.expectedControlDescription) {
				t.Fatalf("control warning = %#v, want warning severity naming %q", warning, testCase.expectedControlDescription)
			}
		})
	}
}

func TestProfileActualTimeSeriesMarksCompactInterpolationApproximationPartial(t *testing.T) {
	doc, err := Parse(`
Zone, Interpolation Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Schedule:Compact,
  Interpolated Lights,       !- Name
  ,                          !- Schedule Type Limits Name
  Through: 12/31,
  For: AllDays,
  Interpolate: Linear,
  Until: 12:00, 0,
  Until: 24:00, 1;
Lights,
  Interpolated Load,         !- Name
  Interpolation Zone,        !- Zone or ZoneList or Space or SpaceList Name
  Interpolated Lights,       !- Schedule Name
  Watts/Area,                !- Design Level Calculation Method
  ,                          !- Lighting Level
  10;                        !- Watts per Floor Area
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := AnalyzeProfile(doc)
	series := findProfileSeries(report.GraphDataset.Series, "zone", "Interpolation Zone", ProfileDimensionLighting)
	if series == nil || series.Status != metricStatusPartial {
		t.Fatalf("interpolated schedule series = %#v, want partial step approximation", series)
	}
	warning := actualFidelityWarning(series.Warnings, "schedule_profile_fallback")
	if warning == nil || !strings.Contains(warning.Message, "Interpolate: Linear") || !strings.Contains(strings.ToLower(warning.Message), "step approximation") {
		t.Fatalf("interpolated schedule warnings = %#v, want explicit step-approximation warning", series.Warnings)
	}
	if len(series.Values) < 24 || series.Values[0] != 0 || series.Values[23] != 10 {
		t.Fatalf("step approximation values = %#v, want preserved usable 0/10 design series", series.Values[:min(24, len(series.Values))])
	}
}

func TestProfileActualTimeSeriesMarksMissingSchedulePartial(t *testing.T) {
	doc, err := Parse(`
Zone, Missing Schedule Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Lights,
  Missing Schedule Load, !- Name
  Missing Schedule Zone, !- Zone or ZoneList or Space or SpaceList Name
  Missing Schedule,      !- Schedule Name
  Watts/Area,            !- Design Level Calculation Method
  ,                      !- Lighting Level
  10;                    !- Watts per Floor Area
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := AnalyzeProfile(doc)
	series := findProfileSeries(report.GraphDataset.Series, "zone", "Missing Schedule Zone", ProfileDimensionLighting)
	if series == nil || series.Status != metricStatusPartial {
		t.Fatalf("missing-schedule series = %#v, want partial design-level fallback", series)
	}
	warning := actualFidelityWarning(series.Warnings, "missing_schedule_summary")
	if warning == nil || !strings.Contains(strings.ToLower(warning.Message), "could not be resolved") {
		t.Fatalf("missing-schedule warnings = %#v, want explicit unresolved-schedule reason", series.Warnings)
	}
}

func TestProfileScheduleHashIncludesSeasonalRuleContent(t *testing.T) {
	doc, err := Parse(`
Schedule:Compact,
  Seasonal A,
  ,
  Through: 6/30,
  For: AllDays,
  Until: 24:00, 0,
  Through: 12/31,
  For: AllDays,
  Until: 24:00, 1;
Schedule:Compact,
  Seasonal B,
  ,
  Through: 6/30,
  For: AllDays,
  Until: 24:00, 0,
  Through: 12/31,
  For: AllDays,
  Until: 24:00, 0.5;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := AnalyzeProfile(doc)
	if len(report.Schedules) != 2 {
		t.Fatalf("schedule count = %d, want 2", len(report.Schedules))
	}
	if report.Schedules[0].ContentHash == report.Schedules[1].ContentHash {
		t.Fatalf("seasonally different schedules share hash %q", report.Schedules[0].ContentHash)
	}
	if report.Schedules[0].AnnualStats.EquivalentFullHours == report.Schedules[1].AnnualStats.EquivalentFullHours {
		t.Fatalf("seasonally different schedules have equal annual stats: %#v / %#v", report.Schedules[0].AnnualStats, report.Schedules[1].AnnualStats)
	}
}

func actualFidelityWarning(warnings []ProfileWarning, code string) *ProfileWarning {
	for index := range warnings {
		if warnings[index].Code == code {
			return &warnings[index]
		}
	}
	return nil
}
