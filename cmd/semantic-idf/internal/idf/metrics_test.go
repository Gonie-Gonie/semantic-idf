package idf

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

const metricsFixtureIDF = `
Version,
  24.1;                    !- Version Identifier

GlobalGeometryRules,
  UpperLeftCorner,         !- Starting Vertex Position
  CounterClockWise,        !- Vertex Entry Direction
  World;                   !- Coordinate System

Building,
  Metrics Test Building,   !- Name
  0;                       !- North Axis

Schedule:Constant,
  AlwaysOn,                !- Name
  Fraction,                !- Schedule Type Limits Name
  1;                       !- Hourly Value

Schedule:Compact,
  HalfDay,                 !- Name
  Fraction,                !- Schedule Type Limits Name
  Through: 12/31,          !- Field 1
  For: AllDays,            !- Field 2
  Until: 12:00,            !- Field 3
  1,                       !- Field 4
  Until: 24:00,            !- Field 5
  0;                       !- Field 6

Zone,
  Zone 1,                  !- Name
  0,                       !- Direction of Relative North
  0,                       !- X Origin
  0,                       !- Y Origin
  0,                       !- Z Origin
  1,                       !- Type
  1,                       !- Multiplier
  3,                       !- Ceiling Height
  600;                     !- Volume

BuildingSurface:Detailed,
  Zone 1 Floor,            !- Name
  Floor,                   !- Surface Type
  Floor Construction,      !- Construction Name
  Zone 1,                  !- Zone Name
  Ground,                  !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  NoSun,                   !- Sun Exposure
  NoWind,                  !- Wind Exposure
  0.5,                     !- View Factor to Ground
  4,                       !- Number of Vertices
  0,                       !- Vertex 1 X-coordinate
  0,                       !- Vertex 1 Y-coordinate
  0,                       !- Vertex 1 Z-coordinate
  10,                      !- Vertex 2 X-coordinate
  0,                       !- Vertex 2 Y-coordinate
  0,                       !- Vertex 2 Z-coordinate
  10,                      !- Vertex 3 X-coordinate
  20,                      !- Vertex 3 Y-coordinate
  0,                       !- Vertex 3 Z-coordinate
  0,                       !- Vertex 4 X-coordinate
  20,                      !- Vertex 4 Y-coordinate
  0;                       !- Vertex 4 Z-coordinate

BuildingSurface:Detailed,
  Zone 1 Roof,             !- Name
  Roof,                    !- Surface Type
  Roof Construction,       !- Construction Name
  Zone 1,                  !- Zone Name
  Outdoors,                !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  SunExposed,              !- Sun Exposure
  WindExposed,             !- Wind Exposure
  0.5,                     !- View Factor to Ground
  4,                       !- Number of Vertices
  0,                       !- Vertex 1 X-coordinate
  20,                      !- Vertex 1 Y-coordinate
  3,                       !- Vertex 1 Z-coordinate
  10,                      !- Vertex 2 X-coordinate
  20,                      !- Vertex 2 Y-coordinate
  3,                       !- Vertex 2 Z-coordinate
  10,                      !- Vertex 3 X-coordinate
  0,                       !- Vertex 3 Y-coordinate
  3,                       !- Vertex 3 Z-coordinate
  0,                       !- Vertex 4 X-coordinate
  0,                       !- Vertex 4 Y-coordinate
  3;                       !- Vertex 4 Z-coordinate

BuildingSurface:Detailed,
  South Wall,              !- Name
  Wall,                    !- Surface Type
  Wall Construction,       !- Construction Name
  Zone 1,                  !- Zone Name
  Outdoors,                !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  SunExposed,              !- Sun Exposure
  WindExposed,             !- Wind Exposure
  0.5,                     !- View Factor to Ground
  4,                       !- Number of Vertices
  0,                       !- Vertex 1 X-coordinate
  0,                       !- Vertex 1 Y-coordinate
  0,                       !- Vertex 1 Z-coordinate
  10,                      !- Vertex 2 X-coordinate
  0,                       !- Vertex 2 Y-coordinate
  0,                       !- Vertex 2 Z-coordinate
  10,                      !- Vertex 3 X-coordinate
  0,                       !- Vertex 3 Y-coordinate
  3,                       !- Vertex 3 Z-coordinate
  0,                       !- Vertex 4 X-coordinate
  0,                       !- Vertex 4 Y-coordinate
  3;                       !- Vertex 4 Z-coordinate

BuildingSurface:Detailed,
  East Wall,               !- Name
  Wall,                    !- Surface Type
  Wall Construction,       !- Construction Name
  Zone 1,                  !- Zone Name
  Outdoors,                !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  SunExposed,              !- Sun Exposure
  WindExposed,             !- Wind Exposure
  0.5,                     !- View Factor to Ground
  4,                       !- Number of Vertices
  10,                      !- Vertex 1 X-coordinate
  0,                       !- Vertex 1 Y-coordinate
  0,                       !- Vertex 1 Z-coordinate
  10,                      !- Vertex 2 X-coordinate
  20,                      !- Vertex 2 Y-coordinate
  0,                       !- Vertex 2 Z-coordinate
  10,                      !- Vertex 3 X-coordinate
  20,                      !- Vertex 3 Y-coordinate
  3,                       !- Vertex 3 Z-coordinate
  10,                      !- Vertex 4 X-coordinate
  0,                       !- Vertex 4 Y-coordinate
  3;                       !- Vertex 4 Z-coordinate

BuildingSurface:Detailed,
  North Wall,              !- Name
  Wall,                    !- Surface Type
  Wall Construction,       !- Construction Name
  Zone 1,                  !- Zone Name
  Outdoors,                !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  SunExposed,              !- Sun Exposure
  WindExposed,             !- Wind Exposure
  0.5,                     !- View Factor to Ground
  4,                       !- Number of Vertices
  10,                      !- Vertex 1 X-coordinate
  20,                      !- Vertex 1 Y-coordinate
  0,                       !- Vertex 1 Z-coordinate
  0,                       !- Vertex 2 X-coordinate
  20,                      !- Vertex 2 Y-coordinate
  0,                       !- Vertex 2 Z-coordinate
  0,                       !- Vertex 3 X-coordinate
  20,                      !- Vertex 3 Y-coordinate
  3,                       !- Vertex 3 Z-coordinate
  10,                      !- Vertex 4 X-coordinate
  20,                      !- Vertex 4 Y-coordinate
  3;                       !- Vertex 4 Z-coordinate

BuildingSurface:Detailed,
  West Wall,               !- Name
  Wall,                    !- Surface Type
  Wall Construction,       !- Construction Name
  Zone 1,                  !- Zone Name
  Outdoors,                !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  SunExposed,              !- Sun Exposure
  WindExposed,             !- Wind Exposure
  0.5,                     !- View Factor to Ground
  4,                       !- Number of Vertices
  0,                       !- Vertex 1 X-coordinate
  20,                      !- Vertex 1 Y-coordinate
  0,                       !- Vertex 1 Z-coordinate
  0,                       !- Vertex 2 X-coordinate
  0,                       !- Vertex 2 Y-coordinate
  0,                       !- Vertex 2 Z-coordinate
  0,                       !- Vertex 3 X-coordinate
  0,                       !- Vertex 3 Y-coordinate
  3,                       !- Vertex 3 Z-coordinate
  0,                       !- Vertex 4 X-coordinate
  20,                      !- Vertex 4 Y-coordinate
  3;                       !- Vertex 4 Z-coordinate

FenestrationSurface:Detailed,
  South Window,            !- Name
  Window,                  !- Surface Type
  Window Construction,     !- Construction Name
  South Wall,              !- Building Surface Name
  ,                        !- Outside Boundary Condition Object
  0.5,                     !- View Factor to Ground
  ,                        !- Frame and Divider Name
  1,                       !- Multiplier
  4,                       !- Number of Vertices
  4,                       !- Vertex 1 X-coordinate
  0,                       !- Vertex 1 Y-coordinate
  1,                       !- Vertex 1 Z-coordinate
  6,                       !- Vertex 2 X-coordinate
  0,                       !- Vertex 2 Y-coordinate
  1,                       !- Vertex 2 Z-coordinate
  6,                       !- Vertex 3 X-coordinate
  0,                       !- Vertex 3 Y-coordinate
  2,                       !- Vertex 3 Z-coordinate
  4,                       !- Vertex 4 X-coordinate
  0,                       !- Vertex 4 Y-coordinate
  2;                       !- Vertex 4 Z-coordinate

FenestrationSurface:Detailed,
  East Window,             !- Name
  Window,                  !- Surface Type
  Window Construction,     !- Construction Name
  East Wall,               !- Building Surface Name
  ,                        !- Outside Boundary Condition Object
  0.5,                     !- View Factor to Ground
  ,                        !- Frame and Divider Name
  1,                       !- Multiplier
  4,                       !- Number of Vertices
  10,                      !- Vertex 1 X-coordinate
  8,                       !- Vertex 1 Y-coordinate
  1,                       !- Vertex 1 Z-coordinate
  10,                      !- Vertex 2 X-coordinate
  12,                      !- Vertex 2 Y-coordinate
  1,                       !- Vertex 2 Z-coordinate
  10,                      !- Vertex 3 X-coordinate
  12,                      !- Vertex 3 Y-coordinate
  2,                       !- Vertex 3 Z-coordinate
  10,                      !- Vertex 4 X-coordinate
  8,                       !- Vertex 4 Y-coordinate
  2;                       !- Vertex 4 Z-coordinate

People,
  Zone People,             !- Name
  Zone 1,                  !- Zone or ZoneList Name
  HalfDay,                 !- Number of People Schedule Name
  People,                  !- Number of People Calculation Method
  10;                      !- Number of People

Lights,
  Zone Lights,             !- Name
  Zone 1,                  !- Zone or ZoneList Name
  HalfDay,                 !- Schedule Name
  LightingLevel,           !- Design Level Calculation Method
  1000;                    !- Lighting Level

ElectricEquipment,
  Zone Equipment,          !- Name
  Zone 1,                  !- Zone or ZoneList Name
  HalfDay,                 !- Schedule Name
  EquipmentLevel,          !- Design Level Calculation Method
  2000;                    !- Design Level

ThermostatSetpoint:DualSetpoint,
  Dual Setpoint,           !- Name
  AlwaysOn,                !- Heating Setpoint Temperature Schedule Name
  AlwaysOn;                !- Cooling Setpoint Temperature Schedule Name

ZoneControl:Thermostat,
  Zone Thermostat,         !- Name
  Zone 1,                  !- Zone or ZoneList Name
  AlwaysOn,                !- Control Type Schedule Name
  ThermostatSetpoint:DualSetpoint, !- Control 1 Object Type
  Dual Setpoint;           !- Control 1 Name

Fan:ConstantVolume,
  Supply Fan,              !- Name
  AlwaysOn,                !- Availability Schedule Name
  0.7,                     !- Fan Total Efficiency
  500,                     !- Pressure Rise
  1.0,                     !- Maximum Flow Rate
  0.9,                     !- Motor Efficiency
  1.0,                     !- Motor In Airstream Fraction
  Inlet Node,              !- Air Inlet Node Name
  Outlet Node;             !- Air Outlet Node Name
`

const metricsSkylightMissingBaseFixtureIDF = `
Version,
  24.1;

GlobalGeometryRules,
  UpperLeftCorner,
  CounterClockWise,
  World;

Zone,
  Zone 1;

BuildingSurface:Detailed,
  Zone Roof,                !- Name
  Roof,                     !- Surface Type
  Roof Construction,        !- Construction Name
  Zone 1,                   !- Zone Name
  Outdoors,                 !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  SunExposed,               !- Sun Exposure
  WindExposed,              !- Wind Exposure
  0.5,                      !- View Factor to Ground
  4,                        !- Number of Vertices
  0,                        !- Vertex 1 X-coordinate
  0,                        !- Vertex 1 Y-coordinate
  3,                        !- Vertex 1 Z-coordinate
  10,                       !- Vertex 2 X-coordinate
  0,                        !- Vertex 2 Y-coordinate
  3,                        !- Vertex 2 Z-coordinate
  10,                       !- Vertex 3 X-coordinate
  10,                       !- Vertex 3 Y-coordinate
  3,                        !- Vertex 3 Z-coordinate
  0,                        !- Vertex 4 X-coordinate
  10,                       !- Vertex 4 Y-coordinate
  3;                        !- Vertex 4 Z-coordinate

FenestrationSurface:Detailed,
  Orphan Roof Window,       !- Name
  Window,                   !- Surface Type
  Window Construction,      !- Construction Name
  Missing Roof Surface,     !- Building Surface Name
  ,                         !- Outside Boundary Condition Object
  0.5,                      !- View Factor to Ground
  ,                         !- Frame and Divider Name
  1,                        !- Multiplier
  4,                        !- Number of Vertices
  2,                        !- Vertex 1 X-coordinate
  2,                        !- Vertex 1 Y-coordinate
  3,                        !- Vertex 1 Z-coordinate
  4,                        !- Vertex 2 X-coordinate
  2,                        !- Vertex 2 Y-coordinate
  3,                        !- Vertex 2 Z-coordinate
  4,                        !- Vertex 3 X-coordinate
  4,                        !- Vertex 3 Y-coordinate
  3,                        !- Vertex 3 Z-coordinate
  2,                        !- Vertex 4 X-coordinate
  4,                        !- Vertex 4 Y-coordinate
  3;                        !- Vertex 4 Z-coordinate
`

func TestMetricRegistryAndGuideCoverage(t *testing.T) {
	definitions := MetricDefinitions()
	guides := MetricGuides()
	if len(definitions) != 59 {
		t.Fatalf("definition count = %d, want 59", len(definitions))
	}
	if len(guides) != len(definitions) {
		t.Fatalf("guide count = %d, want %d", len(guides), len(definitions))
	}

	seen := map[string]bool{}
	for index, definition := range definitions {
		if definition.ID == "" || definition.Category == "" || definition.Name == "" {
			t.Fatalf("definition %d has empty required metadata: %#v", index, definition)
		}
		if definition.Source == "" || definition.Method == "" || definition.Assumptions == "" || definition.MissingData == "" {
			t.Fatalf("definition %s is missing guide metadata", definition.ID)
		}
		if seen[definition.ID] {
			t.Fatalf("duplicate definition id %q", definition.ID)
		}
		seen[definition.ID] = true
		if guides[index].ID != definition.ID {
			t.Fatalf("guide %d id = %q, want %q", index, guides[index].ID, definition.ID)
		}
	}
}

func TestAnalyzeMetricsCoreMetricsAndExports(t *testing.T) {
	doc, err := Parse(metricsFixtureIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	if metrics.MetricCount != 59 {
		t.Fatalf("metrics metric count = %d, want 59", metrics.MetricCount)
	}
	if got := countMetrics(metrics); got != 59 {
		t.Fatalf("rendered metric count = %d, want 59", got)
	}
	if got := metricByID(t, metrics, "building_name").DisplayValue; got != "Metrics Test Building" {
		t.Fatalf("building name = %q, want Metrics Test Building", got)
	}

	assertMetricClose(t, metrics, "gross_floor_area_m2", 200, 0.001)
	assertMetricClose(t, metrics, "conditioned_floor_area_m2", 200, 0.001)
	assertMetricClose(t, metrics, "unconditioned_floor_area_m2", 0, 0.001)
	assertMetricClose(t, metrics, "total_zone_volume_m3", 600, 0.001)
	assertMetricClose(t, metrics, "average_floor_height_m", 3, 0.001)
	assertMetricClose(t, metrics, "building_long_side_m", 20, 0.001)
	assertMetricClose(t, metrics, "building_short_side_m", 10, 0.001)
	assertMetricClose(t, metrics, "footprint_aspect_ratio", 2, 0.001)
	assertMetricClose(t, metrics, "bounding_box_area_m2", 200, 0.001)
	assertMetricClose(t, metrics, "exterior_wall_area_m2", 180, 0.001)
	assertMetricClose(t, metrics, "window_area_m2", 6, 0.001)
	assertMetricClose(t, metrics, "total_wwr_percent", 3.3, 0.05)
	assertMetricClose(t, metrics, "east_wwr_percent", 6.7, 0.05)
	assertMetricClose(t, metrics, "south_wwr_percent", 6.7, 0.05)
	assertMetricClose(t, metrics, "north_wwr_percent", 0, 0.001)
	assertMetricClose(t, metrics, "west_wwr_percent", 0, 0.001)
	eastWWR := metricByID(t, metrics, "east_wwr_percent")
	if eastWWR.Source != "surface_azimuth" || eastWWR.Confidence != "computed" || !stringSliceContainsFold(eastWWR.Badges, "orientation") {
		t.Fatalf("east WWR metadata = source %q confidence %q badges %#v, want orientation metadata", eastWWR.Source, eastWWR.Confidence, eastWWR.Badges)
	}
	if !strings.Contains(eastWWR.Evidence, "computed_normal") || !strings.Contains(eastWWR.Evidence, "base_surface") {
		t.Fatalf("east WWR evidence = %q, want azimuth source metrics", eastWWR.Evidence)
	}
	assertMetricClose(t, metrics, "total_lighting_power_w", 1000, 0.001)
	assertMetricClose(t, metrics, "average_lighting_power_density_w_per_m2", 5, 0.001)
	assertMetricClose(t, metrics, "total_equipment_power_w", 2000, 0.001)
	assertMetricClose(t, metrics, "average_equipment_power_density_w_per_m2", 10, 0.001)
	assertMetricClose(t, metrics, "total_people", 10, 0.001)
	assertMetricClose(t, metrics, "people_density_per_100m2", 5, 0.001)
	if got := metricByID(t, metrics, "internal_load_method_coverage").DisplayValue; got != "resolved:3/3, unresolved_method_count:0" {
		t.Fatalf("internal load coverage = %q, want all core loads resolved", got)
	}
	assertMetricClose(t, metrics, "model_operating_hours_h", 8760, 0.001)
	assertMetricClose(t, metrics, "average_schedule_operating_hours_h", 6570, 0.001)

	if got := metricByID(t, metrics, "supported_schedule_count").Value; got != 2 {
		t.Fatalf("supported schedule count = %#v, want 2", got)
	}
	if got := metricByID(t, metrics, "conditioned_zone_count").Value; got != 1 {
		t.Fatalf("conditioned zone count = %#v, want 1", got)
	}
	if got := metricByID(t, metrics, "conditioned_zone_evidence_breakdown").DisplayValue; !strings.Contains(got, "by_thermostat:1") {
		t.Fatalf("conditioned zone evidence = %q, want thermostat count", got)
	}
	if got := metricByID(t, metrics, "conditioned_floor_area_m2").Confidence; got != "inferred" {
		t.Fatalf("conditioned floor confidence = %q, want inferred", got)
	}
	if got := metricByID(t, metrics, "unconditioned_floor_area_m2").Visibility; got != "advanced" {
		t.Fatalf("unconditioned floor visibility = %q, want advanced", got)
	}
	if got := metricByID(t, metrics, "hvac_node_connection_count").Value; got != 0 {
		t.Fatalf("hvac node connection count = %#v, want 0 typed loop edges", got)
	}
	if got := metricByID(t, metrics, "model_operating_hours_h").Name; got != "Representative operating hours" {
		t.Fatalf("model operating hours label = %q", got)
	}
	if got := metricByID(t, metrics, "average_schedule_operating_hours_h").Visibility; got != "advanced" {
		t.Fatalf("average schedule visibility = %q, want advanced", got)
	}
	if got := metricByID(t, metrics, "geometry_coverage_percent").Confidence; got == "" {
		t.Fatalf("geometry coverage confidence is empty")
	}

	jsonText, err := ExportMetricsJSON(metrics)
	if err != nil {
		t.Fatalf("ExportMetricsJSON() error = %v", err)
	}
	var exported map[string]map[string]struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Value  any    `json:"value"`
		Unit   string `json:"unit"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(jsonText), &exported); err != nil {
		t.Fatalf("metrics JSON did not parse: %v\n%s", err, jsonText)
	}
	if exported["Geometry & Areas"]["gross_floor_area_m2"].Name != "Gross floor area" {
		t.Fatalf("metrics JSON missing categorized gross floor area metric: %#v", exported["Geometry & Areas"]["gross_floor_area_m2"])
	}

	csvText, err := ExportMetricsCSV(metrics)
	if err != nil {
		t.Fatalf("ExportMetricsCSV() error = %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(csvText)).ReadAll()
	if err != nil {
		t.Fatalf("metrics CSV did not parse: %v\n%s", err, csvText)
	}
	if len(records) != 60 {
		t.Fatalf("CSV rows = %d, want 60", len(records))
	}
	if len(records[0]) != 2 || records[0][0] != "name" || records[0][1] != "value" {
		t.Fatalf("CSV header = %#v, want name,value", records[0])
	}
	csvValues := map[string]string{}
	csvBaseNames := map[string]bool{}
	for index, record := range records {
		if len(record) != 2 {
			t.Fatalf("CSV row %d has %d columns, want 2: %#v", index, len(record), record)
		}
		if index == 0 {
			continue
		}
		if strings.Contains(record[0], " / ") {
			t.Fatalf("CSV row %d includes category in name: %#v", index, record)
		}
		if strings.Contains(record[1], " m2") || strings.Contains(record[1], " %") {
			t.Fatalf("CSV row %d includes unit in value: %#v", index, record)
		}
		if !strings.Contains(record[0], " [") || !strings.HasSuffix(record[0], "]") {
			t.Fatalf("CSV row %d does not include bracketed unit: %#v", index, record)
		}
		baseName, _, ok := strings.Cut(record[0], " [")
		if !ok {
			t.Fatalf("CSV row %d has unparsable bracketed unit: %#v", index, record)
		}
		if csvBaseNames[baseName] {
			t.Fatalf("duplicate CSV metric base name %q", baseName)
		}
		csvBaseNames[baseName] = true
		if _, exists := csvValues[record[0]]; exists {
			t.Fatalf("duplicate CSV metric name %q", record[0])
		}
		csvValues[record[0]] = record[1]
	}
	if _, ok := csvValues["object_count [-]"]; !ok {
		t.Fatalf("CSV missing unitless object count name with [-] unit")
	}
	if got := csvValues["gross_floor_area [m2]"]; got != "200.0" {
		t.Fatalf("CSV gross floor area = %q, want 200.0", got)
	}
	if got := csvValues["total_wwr [%]"]; got != "3.3" {
		t.Fatalf("CSV total WWR = %q, want 3.3", got)
	}
}

func TestAnalyzeMetricsPreservesNegativeBuildingNorthAxis(t *testing.T) {
	doc, err := Parse(`
Building,
  Signed Rotation,
  -22.5;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metric := metricByID(t, AnalyzeMetrics(doc), "building_north_axis_deg")
	if metric.Status != metricStatusOK || metric.Value != -22.5 || metric.DisplayValue != "-22.5" {
		t.Fatalf("negative building north axis = %#v, want available -22.5 deg", metric)
	}
}

func TestAnalyzeMetricsMarksInterpolatedCompactScheduleHoursPartial(t *testing.T) {
	for _, interpolation := range []string{"Linear", "Average"} {
		t.Run(interpolation, func(t *testing.T) {
			doc, err := Parse(fmt.Sprintf(`
Schedule:Compact,
  Interpolated Schedule,
  Fraction,
  Through: 12/31,
  For: AllDays,
  Interpolate: %s,
  Until: 12:00, 1,
  Until: 24:00, 0;
Zone, Schedule Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Lights,
  Scheduled Lights,
  Schedule Zone,
  Interpolated Schedule, !- Schedule Name
  Watts/Area,
  ,
  10;
`, interpolation))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			report := AnalyzeMetrics(doc)
			for _, id := range []string{
				"supported_schedule_count",
				"unsupported_schedule_count",
				"model_operating_hours_h",
				"average_schedule_operating_hours_h",
				"profile_coverage_percent",
			} {
				metric := metricByID(t, report, id)
				if metric.Status != metricStatusPartial {
					t.Fatalf("%s status = %q, want partial for Interpolate: %s", id, metric.Status, interpolation)
				}
				if !strings.Contains(metric.Evidence, "step approximations:1") {
					t.Fatalf("%s evidence = %q, want explicit interpolation step-approximation count", id, metric.Evidence)
				}
			}
			if got := metricByID(t, report, "supported_schedule_count").Value; got != 1 {
				t.Fatalf("supported schedule count = %#v, want one usable approximated schedule", got)
			}
			if got := metricByID(t, report, "unsupported_schedule_count").Value; got != 0 {
				t.Fatalf("unsupported schedule count = %#v, want zero wholly unsupported schedules", got)
			}
			assertMetricClose(t, report, "model_operating_hours_h", 4380, 0.001)
			assertMetricClose(t, report, "average_schedule_operating_hours_h", 4380, 0.001)
			assertMetricClose(t, report, "profile_coverage_percent", 100, 0.001)
			if got := metricByID(t, report, "schedule_count").Status; got != metricStatusOK {
				t.Fatalf("direct schedule object count status = %q, want ok", got)
			}
		})
	}
}

func TestAnalyzeMetricsUsesExplicitSpaceVolumesWhenZoneVolumeIsAutocalculate(t *testing.T) {
	for _, test := range []struct {
		name       string
		multiplier float64
		want       float64
	}{
		{name: "single zone instance", multiplier: 1, want: 300},
		{name: "multiplied zone", multiplier: 2, want: 600},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc, err := Parse(fmt.Sprintf(`
Zone,
  Volume Zone,
  0,
  0, 0, 0,
  1,
  %g,
  Autocalculate,
  Autocalculate;
Space,
  West Space,
  Volume Zone,
  Autocalculate,
  120,
  Autocalculate;
Space,
  East Space,
  Volume Zone,
  Autocalculate,
  180,
  Autocalculate;
	`, test.multiplier))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			metric := metricByID(t, AnalyzeMetrics(doc), "total_zone_volume_m3")
			value, ok := metric.Value.(float64)
			if metric.Status != metricStatusOK || !ok || value != test.want {
				t.Fatalf("space-derived zone volume = %#v, want available %.0f m3", metric, test.want)
			}
		})
	}
}

func TestAnalyzeMetricsResolvesAreaPerPersonFromCanonicalField(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office,                  !- Name
  0,                       !- Direction of Relative North
  0,                       !- X Origin
  0,                       !- Y Origin
  0,                       !- Z Origin
  1,                       !- Type
  2,                       !- Multiplier
  3,                       !- Ceiling Height
  557.4,                   !- Volume
  185.8;                   !- Floor Area

BuildingSurface:Detailed,
  Office Floor,            !- Name
  Floor,                   !- Surface Type
  Floor Construction,      !- Construction Name
  Office,                  !- Zone Name
  Ground,                  !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  NoSun,                   !- Sun Exposure
  NoWind,                  !- Wind Exposure
  0.5,                     !- View Factor to Ground
  4,                       !- Number of Vertices
  0, 0, 0,
  18.58, 0, 0,
  18.58, 10, 0,
  0, 10, 0;

People,
  Office People,           !- Name
  Office,
  ,
  Area/Person,
  ,
  ,
  18.58;

GasEquipment,
  Office Gas,              !- Name
  Office,                  !- Zone or ZoneList or Space or SpaceList Name
  ,                        !- Schedule Name
  Power/Area,              !- Design Level Calculation Method
  ,                        !- Design Level
  2;                       !- Power per Floor Area
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	metrics := AnalyzeMetrics(doc)
	assertMetricClose(t, metrics, "gross_floor_area_m2", 371.6, 0.001)
	people := metricByID(t, metrics, "total_people")
	if people.Status != metricStatusOK {
		t.Fatalf("total_people status = %q, want %q (metric %#v)", people.Status, metricStatusOK, people)
	}
	if value, ok := people.Value.(float64); !ok || math.Abs(value-20) > 0.001 {
		t.Fatalf("total_people = %#v, want 20 from the 2x zone multiplier", people.Value)
	}
	assertMetricClose(t, metrics, "total_equipment_power_w", 743.2, 0.001)
	coverage := metricByID(t, metrics, "internal_load_method_coverage")
	if coverage.DisplayValue != "resolved:2/2, unresolved_method_count:0" {
		t.Fatalf("internal load coverage = %q, want canonical Area/Person resolution", coverage.DisplayValue)
	}
}

func TestAnalyzeMetricsUsesActualPeopleForWattsPerPersonByZone(t *testing.T) {
	doc, err := Parse(`
Zone, Zone A, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Zone, Zone B, 0, 0, 0, 0, 1, 1, 3, 300, 100;
People, A People, Zone A, , People, 10;
People, B People, Zone B, , People, 30;
Lights, A Lights, Zone A, , Watts/Person, , , 5;
Lights, B Lights, Zone B, , Watts/Person, , , 20;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	assertMetricClose(t, metrics, "total_people", 40, 0.001)
	assertMetricClose(t, metrics, "total_lighting_power_w", 650, 0.001)
}

func TestAnalyzeMetricsSpaceListLoadSemanticsAndPeopleAllocation(t *testing.T) {
	doc, err := Parse(`
Zone, Office, 0, 0, 0, 0, 1, 2, 3, 0, 0;
Space, Small, Office, 3, 150, 50;
Space, Large, Office, 3, 450, 150;
SpaceList, Offices, Small, Large;
People, Space People, Offices, , People, 5;
Lights, Per Person Lights, Offices, , Watts/Person, , , 10;
GasEquipment, Absolute Gas, Offices, , EquipmentLevel, 100;
ElectricEquipment, Area Equipment, Offices, , Watts/Area, , 2;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	assertMetricClose(t, metrics, "gross_floor_area_m2", 400, 0.001)
	// People and direct absolute loads are repeated once per Space and then
	// multiplied by the Zone multiplier: (5 + 5) * 2 = 20 people.
	assertMetricClose(t, metrics, "total_people", 20, 0.001)
	assertMetricClose(t, metrics, "total_lighting_power_w", 200, 0.001)
	// Gas: (100 + 100) * 2. Electric: 2 W/m2 * 400 model-total m2.
	assertMetricClose(t, metrics, "total_equipment_power_w", 1200, 0.001)
	assertMetricClose(t, metrics, "total_zone_volume_m3", 1200, 0.001)
}

func TestAnalyzeMetricsMarksMixedZoneListTargetPartial(t *testing.T) {
	doc, err := Parse(`
Zone, Valid Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
ZoneList, Mixed Zones, Valid Zone, Missing Zone;
People, Mixed People, Mixed Zones, , People, 5;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	people := metricByID(t, metrics, "total_people")
	if people.Status != metricStatusPartial {
		t.Fatalf("total_people status = %q, want partial for an unresolved ZoneList member", people.Status)
	}
	if value, ok := people.Value.(float64); !ok || math.Abs(value-5) > 1e-9 {
		t.Fatalf("total_people = %#v, want the known valid-zone contribution", people.Value)
	}
}

func TestAnalyzeMetricsKeepsAllMissingSpaceListTargetUnavailable(t *testing.T) {
	doc, err := Parse(`
Zone, Valid Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
SpaceList, Broken Spaces, Missing Space A, Missing Space B;
Lights, Orphan Lights, Broken Spaces, , LightingLevel, 100;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	if got := metricByID(t, metrics, "total_lighting_power_w"); got.Status != metricStatusMissing {
		t.Fatalf("total_lighting_power_w = %#v, want unavailable for an all-unresolved SpaceList", got)
	}
	if got := metricByID(t, metrics, "internal_load_method_coverage").DisplayValue; got != "resolved:0/1, unresolved_method_count:1" {
		t.Fatalf("internal load coverage = %q, want unresolved SpaceList load", got)
	}
}

func TestAnalyzeMetricsAppliesZoneGroupAndCommentlessFields(t *testing.T) {
	doc, err := Parse(`
Building, Commentless Building, 25;
Zone, Repeated Zone, 15, 0, 0, 0, 1, 2, 3, 0, 100;
ZoneList, Repeated Zones, Repeated Zone;
ZoneGroup, Floors, Repeated Zones, 3;
People, People, Repeated Zone, , People, 4;
Lights, Lights, Repeated Zone, , LightingLevel, 100;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	if got := metricByID(t, metrics, "building_name").DisplayValue; got != "Commentless Building" {
		t.Fatalf("building_name = %q", got)
	}
	assertMetricClose(t, metrics, "building_north_axis_deg", 25, 0.001)
	assertMetricClose(t, metrics, "gross_floor_area_m2", 600, 0.001)
	assertMetricClose(t, metrics, "total_people", 24, 0.001)
	assertMetricClose(t, metrics, "total_lighting_power_w", 600, 0.001)
	// Zone Volume is zero/autocalculated, so model-total area x height is used.
	assertMetricClose(t, metrics, "total_zone_volume_m3", 1800, 0.001)
}

func TestAnalyzeMetricsCommentlessSpaceAttachedFloor(t *testing.T) {
	doc, err := Parse(`
Zone, Space Zone, 0, 0, 0, 0, 1, 2, 3, 0, 0;
Space, Main Space, Space Zone, 3, 0, 0;
BuildingSurface:Detailed,
  Space Floor, Floor, Floor Construction, , Main Space, Ground, , NoSun, NoWind, 0.5, 4,
  0, 0, 0,
  10, 0, 0,
  10, 5, 0,
  0, 5, 0;
People, Space People, Main Space, , People/Area, , 0.1;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	assertMetricClose(t, metrics, "gross_floor_area_m2", 100, 0.001)
	assertMetricClose(t, metrics, "total_people", 10, 0.001)
}

func TestAnalyzeMetricsResolvedZeroPeopleMakesZeroWattsPerPerson(t *testing.T) {
	doc, err := Parse(`
Zone, Empty Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
People, Empty People, Empty Zone, , People, 0;
Lights, Empty Lights, Empty Zone, , Watts/Person, , , 12;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	assertMetricClose(t, metrics, "total_people", 0, 0.001)
	lighting := metricByID(t, metrics, "total_lighting_power_w")
	if lighting.Status != metricStatusOK {
		t.Fatalf("lighting status = %q, want ok for resolved zero people", lighting.Status)
	}
	assertMetricClose(t, metrics, "total_lighting_power_w", 0, 0.001)
}

func TestAnalyzeMetricsReconcilesExplicitSpaceAndZoneRemainderArea(t *testing.T) {
	doc, err := Parse(`
Zone, Zone With Remainder, 0, 0, 0, 0, 1, 1, 3, 0, 200;
Space, Explicit Space, Zone With Remainder, 3, 75, 25;
BuildingSurface:Detailed,
  Explicit Floor, Floor, Floor Construction, Zone With Remainder, Explicit Space, Ground, , NoSun, NoWind, 0.5, 4,
  0, 0, 0, 5, 0, 0, 5, 5, 0, 0, 5, 0;
BuildingSurface:Detailed,
  Remainder Floor, Floor, Floor Construction, Zone With Remainder, , Ground, , NoSun, NoWind, 0.5, 4,
  5, 0, 0, 20, 0, 0, 20, 5, 0, 5, 5, 0;
People, Explicit People, Explicit Space, , People/Area, , 0.1;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	assertMetricClose(t, metrics, "gross_floor_area_m2", 200, 0.001)
	// 25 m2 explicit + 75 m2 remainder is scaled to the declared 200 m2 Zone:
	// the explicit Space becomes 50 m2 and therefore holds five people.
	assertMetricClose(t, metrics, "total_people", 5, 0.001)
}

func TestAnalyzeMetricsAllocatesZonePeopleAcrossImplicitRemainder(t *testing.T) {
	doc, err := Parse(`
Zone, Zone With Remainder, 0, 0, 0, 0, 1, 1, 3, 300, 100;
Space, Explicit Space, Zone With Remainder, 3, 75, 25;
BuildingSurface:Detailed,
  Explicit Floor, Floor, Floor Construction, Zone With Remainder, Explicit Space, Ground, , NoSun, NoWind, 0.5, 4,
  0, 0, 0, 5, 0, 0, 5, 5, 0, 0, 5, 0;
BuildingSurface:Detailed,
  Remainder Floor, Floor, Floor Construction, Zone With Remainder, , Ground, , NoSun, NoWind, 0.5, 4,
  5, 0, 0, 20, 0, 0, 20, 5, 0, 5, 5, 0;
People, Zone People, Zone With Remainder, , People, 20;
Lights, Space Lights, Explicit Space, , Watts/Person, , , 10;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	assertMetricClose(t, metrics, "total_people", 20, 0.001)
	assertMetricClose(t, metrics, "total_lighting_power_w", 50, 0.001)
}

func TestAnalyzeMetricsKeepsZoneVolumeIndependentFromSpaceOverride(t *testing.T) {
	doc, err := Parse(`
Zone, Volume Zone, 0, 0, 0, 0, 1, 1, 3, 0, 100;
Space, Explicit Space, Volume Zone, 3, 150, 25;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	assertMetricClose(t, metrics, "total_zone_volume_m3", 300, 0.001)
}

func TestAnalyzeMetricsMarksWattsPerPersonPartialWhenPeopleCoverageIsPartial(t *testing.T) {
	doc, err := Parse(`
Zone, Mixed Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100;
People, Valid People, Mixed Zone, , People, 10;
People, Broken People, Mixed Zone, , People/Area, , ;
Lights, Mixed Lights, Mixed Zone, , Watts/Person, , , 5;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	assertMetricClose(t, metrics, "total_lighting_power_w", 50, 0.001)
	if got := metricByID(t, metrics, "total_lighting_power_w").Status; got != metricStatusPartial {
		t.Fatalf("lighting status = %q, want partial when People coverage is incomplete", got)
	}
}

func TestFormatMetricNumberPreservesEngineeringPrecision(t *testing.T) {
	tests := []struct {
		name      string
		value     float64
		precision int
		want      string
	}{
		{name: "large area", value: 4978.588, precision: 2, want: "4978.59"},
		{name: "four displayed digits", value: 12.345, precision: 3, want: "12.345"},
		{name: "small ratio keeps precision", value: 0.123, precision: 3, want: "0.123"},
		{name: "outdoor air per person remains nonzero", value: 0.0125, precision: 6, want: "0.0125"},
		{name: "power density keeps meaningful decimals", value: 10.76, precision: 3, want: "10.76"},
		{name: "integer numeric keeps one decimal", value: 5, precision: 2, want: "5.0"},
		{name: "count precision stays integer", value: 42.2, precision: 0, want: "42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMetricNumber(tt.value, tt.precision); got != tt.want {
				t.Fatalf("formatMetricNumber(%v, %d) = %q, want %q", tt.value, tt.precision, got, tt.want)
			}
		})
	}
}

func TestAnalyzeMetricsExcludesZonesNotPartOfTotalFloorArea(t *testing.T) {
	doc, err := Parse(`
Zone,
  Included Zone, 0, 0, 0, 0, 1, 1, 3, 300, 100, , , Yes;
Zone,
  Excluded Zone, 0, 0, 0, 0, 1, 1, 3, 150, 50, , , No;
ZoneControl:Thermostat,
  Included Thermostat,       !- Name
  Included Zone;             !- Zone or ZoneList Name
ZoneControl:Thermostat,
  Excluded Thermostat,       !- Name
  Excluded Zone;             !- Zone or ZoneList Name
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := AnalyzeMetrics(doc)
	assertMetricClose(t, report, "gross_floor_area_m2", 100, 0.001)
	assertMetricClose(t, report, "conditioned_floor_area_m2", 100, 0.001)
	assertMetricClose(t, report, "unconditioned_floor_area_m2", 0, 0.001)
	assertMetricClose(t, report, "total_zone_volume_m3", 450, 0.001)
}

func TestAnalyzeMetricsBoundsUseWorldGeometryCoordinates(t *testing.T) {
	doc, err := Parse(`
GlobalGeometryRules,
  UpperLeftCorner,          !- Starting Vertex Position
  CounterClockWise,        !- Vertex Entry Direction
  Relative;                !- Coordinate System
Zone, Z1, 0, 0, 0, 0, 1;
Zone, Z2, 0, 100, 0, 0, 1;
BuildingSurface:Detailed,
  Z1 Floor,                !- Name
  Floor,                   !- Surface Type
  ,                        !- Construction Name
  Z1,                      !- Zone Name
  ,                        !- Space Name
  Ground,                  !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  NoSun,                   !- Sun Exposure
  NoWind,                  !- Wind Exposure
  ,                        !- View Factor to Ground
  4,                       !- Number of Vertices
  0,0,0, 10,0,0, 10,10,0, 0,10,0;
BuildingSurface:Detailed,
  Z2 Floor,                !- Name
  Floor,                   !- Surface Type
  ,                        !- Construction Name
  Z2,                      !- Zone Name
  ,                        !- Space Name
  Ground,                  !- Outside Boundary Condition
  ,                        !- Outside Boundary Condition Object
  NoSun,                   !- Sun Exposure
  NoWind,                  !- Wind Exposure
  ,                        !- View Factor to Ground
  4,                       !- Number of Vertices
  0,0,0, 10,0,0, 10,10,0, 0,10,0;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := AnalyzeMetrics(doc)
	assertMetricClose(t, report, "bounding_box_area_m2", 1100, 0.001)
	assertMetricClose(t, report, "building_long_side_m", 110, 0.001)
	assertMetricClose(t, report, "building_short_side_m", 10, 0.001)
}

func TestAnalyzeMetricsWorldVertexAzimuthIgnoresRelativeNorthRotations(t *testing.T) {
	doc, err := Parse(`
Building,
  Rotated Metadata,
  90;                      !- North Axis
GlobalGeometryRules,
  UpperLeftCorner,         !- Starting Vertex Position
  CounterClockWise,        !- Vertex Entry Direction
  World;                   !- Coordinate System
Zone,
  World Zone,
  90,                      !- Direction of Relative North
  100, 100, 0,
  1, 1, 3, 300, 100;
BuildingSurface:Detailed,
  World South Wall, Wall, , World Zone, , Outdoors, , SunExposed, WindExposed, , 4,
  0,0,0, 10,0,0, 10,0,3, 0,0,3;
FenestrationSurface:Detailed,
  World South Window, Window, , World South Wall, , , , 1, 4,
  2,0,1, 4,0,1, 4,0,2, 2,0,2;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	report := AnalyzeMetrics(doc)
	assertMetricClose(t, report, "south_wwr_percent", 6.7, 0.05)
	if got := metricByID(t, report, "north_wwr_percent").Status; got != metricStatusMissing {
		t.Fatalf("north WWR status = %q, want missing because world-coordinate vertices must ignore relative-north rotations", got)
	}
}

func TestAnalyzeMetricsQuickDefersHeavyReadinessMetrics(t *testing.T) {
	doc, err := Parse(metricsFixtureIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	quick := AnalyzeMetricsQuick(doc)
	if quick.MetricCount != 59 {
		t.Fatalf("quick metrics metric count = %d, want 59", quick.MetricCount)
	}
	if got := metricByID(t, quick, "building_name").DisplayValue; got != "Metrics Test Building" {
		t.Fatalf("quick building name = %q, want Metrics Test Building", got)
	}
	for _, id := range []string{"hvac_node_connection_count", "hvac_rule_edge_count", "diagnostics_by_source", "output_readiness_percent"} {
		if got := metricByID(t, quick, id).Status; got != metricStatusMissing {
			t.Fatalf("quick %s status = %q, want missing until heavy stages run", id, got)
		}
	}

	full := AnalyzeMetrics(doc)
	if got := metricByID(t, full, "hvac_node_connection_count").Status; got == metricStatusMissing {
		t.Fatalf("full hvac_node_connection_count should be computed")
	}
	if got := metricByID(t, full, "diagnostics_by_source").Status; got == metricStatusMissing {
		t.Fatalf("full diagnostics_by_source should be computed")
	}
}

func TestAnalyzeMetricsSkylightBaseSurfaceResolution(t *testing.T) {
	doc, err := Parse(metricsSkylightMissingBaseFixtureIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	metrics := AnalyzeMetrics(doc)
	skylight := metricByID(t, metrics, "skylight_roof_ratio_percent")
	if skylight.Status != metricStatusPartial || skylight.Confidence != "partial" || skylight.Source != "base_surface_resolution" {
		t.Fatalf("skylight metadata = status %q confidence %q source %q, want partial base surface resolution", skylight.Status, skylight.Confidence, skylight.Source)
	}
	if !strings.Contains(skylight.Evidence, "unresolved:1") || !stringSliceContainsFold(skylight.Badges, "base-surface") {
		t.Fatalf("skylight evidence/badges = %q %#v, want unresolved base surface evidence", skylight.Evidence, skylight.Badges)
	}
}

func TestAnalyzeMetricsConditionedZoneEvidenceBreakdown(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Equipment Zone"}}},
		{Index: 1, Type: "Zone", Fields: []Field{{Value: "Thermostat Zone"}}},
		{Index: 2, Type: "Zone", Fields: []Field{{Value: "Zone HVAC Zone"}}},
		{Index: 3, Type: "Zone", Fields: []Field{{Value: "Space HVAC Zone"}}},
		{Index: 4, Type: "Space", Fields: []Field{
			{Value: "Open Office", Comment: "Name"},
			{Value: "Space HVAC Zone", Comment: "Zone Name"},
		}},
		{Index: 5, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Equipment Zone", Comment: "Zone Name"},
		}},
		{Index: 6, Type: "ZoneControl:Thermostat", Fields: []Field{
			{Value: "Thermostat Zone", Comment: "Zone Name"},
		}},
		{Index: 7, Type: "ZoneHVAC:IdealLoadsAirSystem", Fields: []Field{
			{Value: "Zone HVAC Zone", Comment: "Zone Name"},
		}},
		{Index: 8, Type: "SpaceHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Open Office", Comment: "Space Name"},
		}},
	}}

	metrics := AnalyzeMetrics(doc)
	if got := metricByID(t, metrics, "conditioned_zone_count").Value; got != 4 {
		t.Fatalf("conditioned zone count = %#v, want 4", got)
	}
	breakdown := metricByID(t, metrics, "conditioned_zone_evidence_breakdown")
	for _, expected := range []string{
		"by_equipment_connections:1",
		"by_zone_hvac:1",
		"by_thermostat:1",
		"by_space_hvac:1",
	} {
		if !strings.Contains(breakdown.DisplayValue, expected) {
			t.Fatalf("conditioned evidence = %q, want %s", breakdown.DisplayValue, expected)
		}
	}
	if breakdown.Confidence != "inferred" {
		t.Fatalf("conditioned evidence confidence = %q, want inferred", breakdown.Confidence)
	}
}

func TestAnalyzeMetricsInternalLoadMethodCoverageReportsUnresolved(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office", Comment: "Name"}}},
		{Index: 1, Type: "Lights", Fields: []Field{
			{Value: "Office Lights", Comment: "Name"},
			{Value: "Office", Comment: "Zone Name"},
			{Value: "AlwaysOn", Comment: "Schedule Name"},
			{Value: "Watts/Area", Comment: "Design Level Calculation Method"},
			{Value: "", Comment: "Lighting Level"},
			{Value: "8", Comment: "Watts per Zone Floor Area"},
		}},
	}}

	metrics := AnalyzeMetrics(doc)
	coverage := metricByID(t, metrics, "internal_load_method_coverage")
	if coverage.DisplayValue != "resolved:0/1, unresolved_method_count:1" {
		t.Fatalf("internal load coverage = %q, want unresolved Watts/Area object", coverage.DisplayValue)
	}
	if coverage.Status != metricStatusPartial {
		t.Fatalf("internal load coverage status = %q, want partial", coverage.Status)
	}
}

func TestAnalyzeMetricsSeparatesBoundingBoxAreaFromFootprint(t *testing.T) {
	doc, err := Parse(`
Zone,
  Box Zone;

BuildingSurface:Detailed,
  Roof Surface,             !- Name
  Roof,                     !- Surface Type
  ,                         !- Construction Name
  Box Zone,                 !- Zone Name
  Outdoors,                 !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  SunExposed,               !- Sun Exposure
  WindExposed,              !- Wind Exposure
  0,                        !- View Factor to Ground
  4,                        !- Number of Vertices
  0,                        !- Vertex 1 X-coordinate
  0,                        !- Vertex 1 Y-coordinate
  3,                        !- Vertex 1 Z-coordinate
  10,                       !- Vertex 2 X-coordinate
  0,                        !- Vertex 2 Y-coordinate
  3,                        !- Vertex 2 Z-coordinate
  10,                       !- Vertex 3 X-coordinate
  8,                        !- Vertex 3 Y-coordinate
  3,                        !- Vertex 3 Z-coordinate
  0,                        !- Vertex 4 X-coordinate
  8,                        !- Vertex 4 Y-coordinate
  3;                        !- Vertex 4 Z-coordinate
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	metrics := AnalyzeMetrics(doc)
	footprint := metricByID(t, metrics, "footprint_area_m2")
	if footprint.Status != metricStatusMissing {
		t.Fatalf("footprint status = %q, want missing when only bounding box is available", footprint.Status)
	}
	assertMetricClose(t, metrics, "bounding_box_area_m2", 80, 0.001)
	boundingBox := metricByID(t, metrics, "bounding_box_area_m2")
	if boundingBox.Visibility != "advanced" || boundingBox.Confidence != "inferred" {
		t.Fatalf("bounding box metadata = visibility %q confidence %q, want advanced/inferred", boundingBox.Visibility, boundingBox.Confidence)
	}
}

func TestAnalyzeMetricsSeparatesGrossAndNetEnvelopeArea(t *testing.T) {
	doc, err := Parse(`
Zone,
  Perimeter;

BuildingSurface:Detailed,
  South Wall,               !- Name
  Wall,                     !- Surface Type
  ,                         !- Construction Name
  Perimeter,                !- Zone Name
  Outdoors,                 !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  SunExposed,               !- Sun Exposure
  WindExposed,              !- Wind Exposure
  0,                        !- View Factor to Ground
  4,                        !- Number of Vertices
  0,                        !- Vertex 1 X-coordinate
  0,                        !- Vertex 1 Y-coordinate
  0,                        !- Vertex 1 Z-coordinate
  10,                       !- Vertex 2 X-coordinate
  0,                        !- Vertex 2 Y-coordinate
  0,                        !- Vertex 2 Z-coordinate
  10,                       !- Vertex 3 X-coordinate
  0,                        !- Vertex 3 Y-coordinate
  3,                        !- Vertex 3 Z-coordinate
  0,                        !- Vertex 4 X-coordinate
  0,                        !- Vertex 4 Y-coordinate
  3;                        !- Vertex 4 Z-coordinate

FenestrationSurface:Detailed,
  South Window,             !- Name
  Window,                   !- Surface Type
  ,                         !- Construction Name
  South Wall,               !- Building Surface Name
  ,                         !- Outside Boundary Condition Object
  0.5,                      !- View Factor to Ground
  ,                         !- Frame and Divider Name
  1,                        !- Multiplier
  4,                        !- Number of Vertices
  2,                        !- Vertex 1 X-coordinate
  0,                        !- Vertex 1 Y-coordinate
  1,                        !- Vertex 1 Z-coordinate
  4,                        !- Vertex 2 X-coordinate
  0,                        !- Vertex 2 Y-coordinate
  1,                        !- Vertex 2 Z-coordinate
  4,                        !- Vertex 3 X-coordinate
  0,                        !- Vertex 3 Y-coordinate
  2,                        !- Vertex 3 Z-coordinate
  2,                        !- Vertex 4 X-coordinate
  0,                        !- Vertex 4 Y-coordinate
  2;                        !- Vertex 4 Z-coordinate
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	metrics := AnalyzeMetrics(doc)
	if got := metricByID(t, metrics, "envelope_area_m2").Name; got != "Gross envelope area" {
		t.Fatalf("envelope metric name = %q, want Gross envelope area", got)
	}
	assertMetricClose(t, metrics, "envelope_area_m2", 30, 0.001)
	assertMetricClose(t, metrics, "net_opaque_envelope_area_m2", 28, 0.001)
}

func countMetrics(metrics MetricsReport) int {
	count := 0
	for _, category := range metrics.Categories {
		count += len(category.Metrics)
	}
	return count
}

func TestMetricsCSVNameNormalizesUnits(t *testing.T) {
	unitless := Metric{ID: "object_count"}
	if got := metricsCSVName(metricsCSVVariableName(unitless), metricsCSVUnitLabel(unitless.Unit)); got != "object_count [-]" {
		t.Fatalf("unitless CSV metric name = %q, want object_count [-]", got)
	}

	bracketed := Metric{ID: "total_wwr_percent", Unit: "[%]"}
	if got := metricsCSVName(metricsCSVVariableName(bracketed), metricsCSVUnitLabel(bracketed.Unit)); got != "total_wwr [%]" {
		t.Fatalf("bracketed-unit CSV metric name = %q, want total_wwr [%%]", got)
	}
}

func metricByID(t *testing.T, metrics MetricsReport, id string) Metric {
	t.Helper()
	for _, category := range metrics.Categories {
		for _, metric := range category.Metrics {
			if metric.ID == id {
				return metric
			}
		}
	}
	t.Fatalf("metric %q not found", id)
	return Metric{}
}

func assertMetricClose(t *testing.T, metrics MetricsReport, id string, want float64, tolerance float64) {
	t.Helper()
	metric := metricByID(t, metrics, id)
	got, ok := metric.Value.(float64)
	if !ok {
		t.Fatalf("metric %s value = %#v (%T), want float64", id, metric.Value, metric.Value)
	}
	if math.Abs(got-want) > tolerance {
		t.Fatalf("metric %s = %v, want %v +/- %v", id, got, want, tolerance)
	}
}
