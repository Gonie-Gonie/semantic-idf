package idf

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func profileSetpointTestReport(t *testing.T, input string) ProfileReport {
	t.Helper()
	doc, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	return AnalyzeProfile(doc)
}

func profileSetpointTestSeries(t *testing.T, report ProfileReport, scope, zone, dimension string) ProfileGraphSeries {
	t.Helper()
	for _, series := range report.GraphDataset.Series {
		if series.ScopeType == scope && series.ZoneName == zone && series.Dimension == dimension {
			return series
		}
	}
	t.Fatalf("missing %s / %s / %s series", scope, zone, dimension)
	return ProfileGraphSeries{}
}

func profileSetpointTestValue(t *testing.T, values []*float64, hour int, expected *float64) {
	t.Helper()
	if len(values) != 8760 {
		t.Fatalf("annual length=%d", len(values))
	}
	if expected == nil {
		if values[hour] != nil {
			t.Fatalf("hour %d=%v, want gap", hour, *values[hour])
		}
		return
	}
	if values[hour] == nil || math.Abs(*values[hour]-*expected) > 1e-10 {
		t.Fatalf("hour %d=%v, want %g", hour, values[hour], *expected)
	}
}

func TestProfileThermostatSetpointsFollowActiveModesAndZoneLists(t *testing.T) {
	report := profileSetpointTestReport(t, `
Zone, A;
Zone, B;
ZoneList, Both, A, B;
Schedule:Compact, Mode, , Through: 12/31, For: AllDays, Until: 06:00, 0, Until: 12:00, 1, Until: 18:00, 4, Until: 24:00, 2;
Schedule:Constant, Heat, , 18;
Schedule:Constant, Cool, , 26;
Schedule:Constant, Dual Heat, , 20;
Schedule:Constant, Dual Cool, , 24;
ThermostatSetpoint:SingleHeating, H, Heat;
ThermostatSetpoint:SingleCooling, C, Cool;
ThermostatSetpoint:DualSetpoint, D, Dual Heat, Dual Cool;
ThermostatSetpoint:SingleHeating, Unbound, Heat;
ZoneControl:Thermostat, Both thermostat, Both, Mode, ThermostatSetpoint:SingleHeating, H, ThermostatSetpoint:SingleCooling, C, ThermostatSetpoint:DualSetpoint, D;
`)
	if report.ItemCount != 4 || report.GroupCount != 1 {
		t.Fatalf("items=%d groups=%d", report.ItemCount, report.GroupCount)
	}
	heat18, heat20, cool24, cool26 := 18.0, 20.0, 24.0, 26.0
	for _, zone := range []string{"A", "B"} {
		heat := profileSetpointTestSeries(t, report, "zone", zone, ProfileDimensionHeatingSetpoint)
		cool := profileSetpointTestSeries(t, report, "zone", zone, ProfileDimensionCoolingSetpoint)
		profileSetpointTestValue(t, heat.AnnualValueProfile, 0, nil)
		profileSetpointTestValue(t, heat.AnnualValueProfile, 6, &heat18)
		profileSetpointTestValue(t, cool.AnnualValueProfile, 6, nil)
		profileSetpointTestValue(t, heat.AnnualValueProfile, 12, &heat20)
		profileSetpointTestValue(t, cool.AnnualValueProfile, 12, &cool24)
		profileSetpointTestValue(t, heat.AnnualValueProfile, 18, nil)
		profileSetpointTestValue(t, cool.AnnualValueProfile, 18, &cool26)
		if heat.DesignValue != 19 || cool.DesignValue != 25 || heat.Unit != "degC" || heat.MetricID != "temperature" {
			t.Fatalf("incorrect temperature summary: heat=%#v cool=%#v", heat, cool)
		}
		if len(heat.Values) != 0 || len(heat.AnnualMultiplierProfile) != 0 || heat.AnnualContribution != 0 || heat.EquivalentFullHours != 0 {
			t.Fatal("temperature was exposed as additive/design-scaled load")
		}
		payload, err := json.Marshal(heat)
		if err != nil || !strings.Contains(string(payload), `"annualValueProfile":[null,null`) {
			t.Fatal("inactive temperature is not a wire null gap")
		}
		zoneProfile := findZoneProfile(report.ZoneProfiles, zone)
		for _, item := range zoneProfile.Items {
			if item.ObjectType != "ZoneControl:Thermostat" || item.ObjectName != "Both thermostat" || item.SourceTarget != "Both" || item.SourceTargetKind != "ZoneList" || item.CloneEligible {
				t.Fatalf("source identity or apply eligibility lost: %#v", item)
			}
		}
	}
	groupHeat := profileSetpointTestSeries(t, report, "group", "", ProfileDimensionHeatingSetpoint)
	profileSetpointTestValue(t, groupHeat.AnnualValueProfile, 6, &heat18)
	for _, hint := range report.Outliers {
		if profileIsSetpointDimension(hint.Dimension) {
			t.Fatalf("load QA applied to temperature: %#v", hint)
		}
	}
	for _, candidate := range report.ParameterCandidates {
		if profileIsSetpointDimension(candidate.Dimension) {
			t.Fatalf("load parameter apply suggested for temperature: %#v", candidate)
		}
	}
}

func TestProfileThermostatSetpointsPreserveZeroAndScheduleYearCalendar(t *testing.T) {
	report := profileSetpointTestReport(t, `
Zone, A;
Schedule:Constant, Mode, , 3;
Schedule:Day:Interval, Winter Day, , No, 06:30, 0, 24:00, 20;
Schedule:Day:Hourly, Summer Day, , -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2, -2;
Schedule:Week:Daily, Winter Week, Winter Day, Winter Day, Winter Day, Winter Day, Winter Day, Winter Day, Winter Day, Winter Day, Winter Day, Winter Day, Winter Day, Winter Day;
Schedule:Week:Compact, Summer Week, AllDays, Summer Day;
Schedule:Year, Seasonal, , Winter Week, 1,1,2,28, Summer Week,3,1,12,31;
ThermostatSetpoint:SingleHeatingOrCooling, Both, Seasonal;
ZoneControl:Thermostat, T, A, Mode, ThermostatSetpoint:SingleHeatingOrCooling, Both;
`)
	zero, ten, twenty, negative := 0.0, 10.0, 20.0, -2.0
	for _, dimension := range []string{ProfileDimensionHeatingSetpoint, ProfileDimensionCoolingSetpoint} {
		series := profileSetpointTestSeries(t, report, "zone", "A", dimension)
		profileSetpointTestValue(t, series.AnnualValueProfile, 0, &zero)
		profileSetpointTestValue(t, series.AnnualValueProfile, 6, &ten)
		profileSetpointTestValue(t, series.AnnualValueProfile, 7, &twenty)
		profileSetpointTestValue(t, series.AnnualValueProfile, 58*24+7, &twenty)
		profileSetpointTestValue(t, series.AnnualValueProfile, 59*24, &negative)
		if len(series.DayValueProfile) != 72 || len(series.WeekValueProfile) != 168 || len(series.MonthValueProfile) != 12 || len(series.DurationValueProfile) != 8760 {
			t.Fatalf("incomplete view profiles: %#v", series)
		}
		payload, err := json.Marshal(series)
		if err != nil || !strings.Contains(string(payload), `"annualValueProfile":[0,0,0`) {
			t.Fatalf("actual 0°C was not retained in wire data: %v", err)
		}
	}
}

func TestProfileThermostatSetpointsRejectAmbiguousAndUnsupportedSources(t *testing.T) {
	base := `Zone, A;
Schedule:Constant, Mode, , 1;
Schedule:Constant, Heat, , 20;
ThermostatSetpoint:SingleHeating, H, Heat;
ZoneControl:Thermostat, T, A, Mode, ThermostatSetpoint:SingleHeating, H;
`
	cases := map[string]string{
		"duplicate schedule":       base + "Schedule:Constant, Heat, , 23;",
		"duplicate setpoint":       base + "ThermostatSetpoint:SingleHeating, H, Heat;",
		"duplicate controller":     base + "ZoneControl:Thermostat, Other, A, Mode, ThermostatSetpoint:SingleHeating, H;",
		"duplicate zone":           base + "Zone, A;",
		"external override":        base + "EnergyManagementSystem:Actuator, Act, Heat, Schedule:Constant, Schedule Value;",
		"interpolated schedule":    strings.Replace(base, "Schedule:Constant, Heat, , 20;", "Schedule:Compact, Heat, , Through:12/31, For:AllDays, Interpolate:Linear, Until:24:00, 20;", 1),
		"missing schedule":         strings.Replace(base, "Schedule:Constant, Heat, , 20;", "", 1),
		"missing control schedule": strings.Replace(base, "Schedule:Constant, Mode, , 1;", "", 1),
		"invalid control mode":     strings.Replace(base, "Schedule:Constant, Mode, , 1;", "Schedule:Constant, Mode, , 1.5;", 1),
		"file schedule":            strings.Replace(base, "Schedule:Constant, Heat, , 20;", "Schedule:File, Heat, , missing.csv, 1, 0, 8760, Comma, No;", 1),
		"unsupported setpoint":     strings.Replace(base, "ThermostatSetpoint:SingleHeating, H, Heat;", "ThermostatSetpoint:SingleHeating, Other, Heat;", 1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			report := profileSetpointTestReport(t, input)
			series := profileSetpointTestSeries(t, report, "zone", "A", ProfileDimensionHeatingSetpoint)
			if series.Status != metricStatusMissing || len(series.Warnings) == 0 {
				t.Fatalf("missing evidence not distinguished: %#v", series)
			}
			for hour, value := range series.AnnualValueProfile {
				if value != nil {
					t.Fatalf("unknown source became %g°C at hour%d", *value, hour)
				}
			}
		})
	}
	listReport := profileSetpointTestReport(t, strings.Replace(base, "ZoneControl:Thermostat, T, A, Mode,", "ZoneList, Both, A;\nZoneList, Both, A;\nZoneControl:Thermostat, T, Both, Mode,", 1))
	series := profileSetpointTestSeries(t, listReport, "zone", "A", ProfileDimensionHeatingSetpoint)
	if series.Status != metricStatusMissing || series.AnnualValueProfile[0] != nil {
		t.Fatal("ambiguous ZoneList selected a native owner")
	}
}

func TestProfileThermostatSetpointsSeparateControlSchedulesForGrouping(t *testing.T) {
	report := profileSetpointTestReport(t, `
Zone,A;
Zone,B;
Zone,C;
Schedule:Constant, Mode A, , 1;
Schedule:Constant, Mode B, , 1;
Schedule:Compact, Mode C, , Through:12/31, For:AllDays, Until:12:00, 0, Until:24:00,1;
Schedule:Constant, Heat, ,20;
ThermostatSetpoint:SingleHeating,H,Heat;
ZoneControl:Thermostat, TA,A,Mode A,ThermostatSetpoint:SingleHeating,H;
ZoneControl:Thermostat, TB,B,Mode B,ThermostatSetpoint:SingleHeating,H;
ZoneControl:Thermostat, TC,C,Mode C,ThermostatSetpoint:SingleHeating,H;
`)
	if report.GroupCount != 3 {
		t.Fatalf("name grouping=%d, want3", report.GroupCount)
	}
	settings := report.DefaultSettings
	settings.ScheduleCompareMode = "resolved"
	groups := buildDefaultProfileGroups(report.ZoneProfiles, settings)
	if len(groups) != 2 {
		t.Fatalf("resolved grouping=%d, want equivalent modes merged and inactivehours separate", len(groups))
	}
	first := profileSetpointTestSeries(t, report, "zone", "A", ProfileDimensionHeatingSetpoint)
	third := profileSetpointTestSeries(t, report, "zone", "C", ProfileDimensionHeatingSetpoint)
	if first.DesignValue != third.DesignValue || first.ScheduleHash == third.ScheduleHash {
		t.Fatal("control schedule mask omitted from grouping signature")
	}
}

func TestProfileThermostatSetpointsIncludeStagedBaseSchedules(t *testing.T) {
	report := profileSetpointTestReport(t, `
Zone,A;
Schedule:Constant, Heat, ,21;
Schedule:Constant, Cool, ,25;
ZoneControl:Thermostat:StagedDualSetpoint, Stage T,A,1,Heat,1.1,-0.5,,,,1,Cool,1.1,0.5;
`)
	heat, cool := 21.0, 25.0
	profileSetpointTestValue(t, profileSetpointTestSeries(t, report, "zone", "A", ProfileDimensionHeatingSetpoint).AnnualValueProfile, 0, &heat)
	profileSetpointTestValue(t, profileSetpointTestSeries(t, report, "zone", "A", ProfileDimensionCoolingSetpoint).AnnualValueProfile, 0, &cool)
}
