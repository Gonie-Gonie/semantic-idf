package idf

import (
	"encoding/csv"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

const summaryFixtureIDF = `
Version,
  24.1;                    !- Version Identifier

GlobalGeometryRules,
  UpperLeftCorner,         !- Starting Vertex Position
  CounterClockWise,        !- Vertex Entry Direction
  World;                   !- Coordinate System

Building,
  Summary Test Building,   !- Name
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

func TestSummaryRegistryAndGuideCoverage(t *testing.T) {
	definitions := SummaryDefinitions()
	guides := SummaryGuides()
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

func TestAnalyzeSummaryCoreMetricsAndExports(t *testing.T) {
	doc, err := Parse(summaryFixtureIDF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	summary := AnalyzeSummary(doc)
	if summary.MetricCount != 59 {
		t.Fatalf("summary metric count = %d, want 59", summary.MetricCount)
	}
	if got := countMetrics(summary); got != 59 {
		t.Fatalf("rendered metric count = %d, want 59", got)
	}
	if got := metricByID(t, summary, "building_name").DisplayValue; got != "Summary Test Building" {
		t.Fatalf("building name = %q, want Summary Test Building", got)
	}

	assertMetricClose(t, summary, "gross_floor_area_m2", 200, 0.001)
	assertMetricClose(t, summary, "conditioned_floor_area_m2", 200, 0.001)
	assertMetricClose(t, summary, "unconditioned_floor_area_m2", 0, 0.001)
	assertMetricClose(t, summary, "total_zone_volume_m3", 600, 0.001)
	assertMetricClose(t, summary, "average_floor_height_m", 3, 0.001)
	assertMetricClose(t, summary, "building_long_side_m", 20, 0.001)
	assertMetricClose(t, summary, "building_short_side_m", 10, 0.001)
	assertMetricClose(t, summary, "footprint_aspect_ratio", 2, 0.001)
	assertMetricClose(t, summary, "bounding_box_area_m2", 200, 0.001)
	assertMetricClose(t, summary, "exterior_wall_area_m2", 180, 0.001)
	assertMetricClose(t, summary, "window_area_m2", 6, 0.001)
	assertMetricClose(t, summary, "total_wwr_percent", 3.3, 0.05)
	assertMetricClose(t, summary, "east_wwr_percent", 6.7, 0.05)
	assertMetricClose(t, summary, "south_wwr_percent", 6.7, 0.05)
	assertMetricClose(t, summary, "north_wwr_percent", 0, 0.001)
	assertMetricClose(t, summary, "west_wwr_percent", 0, 0.001)
	assertMetricClose(t, summary, "total_lighting_power_w", 1000, 0.001)
	assertMetricClose(t, summary, "average_lighting_power_density_w_per_m2", 5, 0.001)
	assertMetricClose(t, summary, "total_equipment_power_w", 2000, 0.001)
	assertMetricClose(t, summary, "average_equipment_power_density_w_per_m2", 10, 0.001)
	assertMetricClose(t, summary, "total_people", 10, 0.001)
	assertMetricClose(t, summary, "people_density_per_100m2", 5, 0.001)
	if got := metricByID(t, summary, "internal_load_method_coverage").DisplayValue; got != "resolved:3/3, unresolved_method_count:0" {
		t.Fatalf("internal load coverage = %q, want all core loads resolved", got)
	}
	assertMetricClose(t, summary, "model_operating_hours_h", 8760, 0.001)
	assertMetricClose(t, summary, "average_schedule_operating_hours_h", 6570, 0.001)

	if got := metricByID(t, summary, "supported_schedule_count").Value; got != 2 {
		t.Fatalf("supported schedule count = %#v, want 2", got)
	}
	if got := metricByID(t, summary, "conditioned_zone_count").Value; got != 1 {
		t.Fatalf("conditioned zone count = %#v, want 1", got)
	}
	if got := metricByID(t, summary, "conditioned_zone_evidence_breakdown").DisplayValue; !strings.Contains(got, "by_thermostat:1") {
		t.Fatalf("conditioned zone evidence = %q, want thermostat count", got)
	}
	if got := metricByID(t, summary, "conditioned_floor_area_m2").Confidence; got != "inferred" {
		t.Fatalf("conditioned floor confidence = %q, want inferred", got)
	}
	if got := metricByID(t, summary, "unconditioned_floor_area_m2").Visibility; got != "advanced" {
		t.Fatalf("unconditioned floor visibility = %q, want advanced", got)
	}
	if got := metricByID(t, summary, "hvac_node_connection_count").Value; got != 0 {
		t.Fatalf("hvac node connection count = %#v, want 0 typed loop edges", got)
	}
	if got := metricByID(t, summary, "model_operating_hours_h").Name; got != "Representative operating hours" {
		t.Fatalf("model operating hours label = %q", got)
	}
	if got := metricByID(t, summary, "average_schedule_operating_hours_h").Visibility; got != "advanced" {
		t.Fatalf("average schedule visibility = %q, want advanced", got)
	}
	if got := metricByID(t, summary, "geometry_coverage_percent").Confidence; got == "" {
		t.Fatalf("geometry coverage confidence is empty")
	}

	jsonText, err := ExportSummaryJSON(summary)
	if err != nil {
		t.Fatalf("ExportSummaryJSON() error = %v", err)
	}
	var exported map[string]map[string]struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Value  any    `json:"value"`
		Unit   string `json:"unit"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(jsonText), &exported); err != nil {
		t.Fatalf("summary JSON did not parse: %v\n%s", err, jsonText)
	}
	if exported["Geometry & Areas"]["gross_floor_area_m2"].Name != "Gross floor area" {
		t.Fatalf("summary JSON missing categorized gross floor area metric: %#v", exported["Geometry & Areas"]["gross_floor_area_m2"])
	}

	csvText, err := ExportSummaryCSV(summary)
	if err != nil {
		t.Fatalf("ExportSummaryCSV() error = %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(csvText)).ReadAll()
	if err != nil {
		t.Fatalf("summary CSV did not parse: %v\n%s", err, csvText)
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
	if got := csvValues["gross_floor_area [m2]"]; got != "200" {
		t.Fatalf("CSV gross floor area = %q, want 200", got)
	}
	if got := csvValues["total_wwr [%]"]; got != "3.3" {
		t.Fatalf("CSV total WWR = %q, want 3.3", got)
	}
}

func TestAnalyzeSummaryConditionedZoneEvidenceBreakdown(t *testing.T) {
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

	summary := AnalyzeSummary(doc)
	if got := metricByID(t, summary, "conditioned_zone_count").Value; got != 4 {
		t.Fatalf("conditioned zone count = %#v, want 4", got)
	}
	breakdown := metricByID(t, summary, "conditioned_zone_evidence_breakdown")
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

func TestAnalyzeSummaryInternalLoadMethodCoverageReportsUnresolved(t *testing.T) {
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

	summary := AnalyzeSummary(doc)
	coverage := metricByID(t, summary, "internal_load_method_coverage")
	if coverage.DisplayValue != "resolved:0/1, unresolved_method_count:1" {
		t.Fatalf("internal load coverage = %q, want unresolved Watts/Area object", coverage.DisplayValue)
	}
	if coverage.Status != summaryStatusPartial {
		t.Fatalf("internal load coverage status = %q, want partial", coverage.Status)
	}
}

func TestAnalyzeSummarySeparatesBoundingBoxAreaFromFootprint(t *testing.T) {
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

	summary := AnalyzeSummary(doc)
	footprint := metricByID(t, summary, "footprint_area_m2")
	if footprint.Status != summaryStatusMissing {
		t.Fatalf("footprint status = %q, want missing when only bounding box is available", footprint.Status)
	}
	assertMetricClose(t, summary, "bounding_box_area_m2", 80, 0.001)
	boundingBox := metricByID(t, summary, "bounding_box_area_m2")
	if boundingBox.Visibility != "advanced" || boundingBox.Confidence != "inferred" {
		t.Fatalf("bounding box metadata = visibility %q confidence %q, want advanced/inferred", boundingBox.Visibility, boundingBox.Confidence)
	}
}

func TestAnalyzeSummarySeparatesGrossAndNetEnvelopeArea(t *testing.T) {
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

	summary := AnalyzeSummary(doc)
	if got := metricByID(t, summary, "envelope_area_m2").Name; got != "Gross envelope area" {
		t.Fatalf("envelope metric name = %q, want Gross envelope area", got)
	}
	assertMetricClose(t, summary, "envelope_area_m2", 30, 0.001)
	assertMetricClose(t, summary, "net_opaque_envelope_area_m2", 28, 0.001)
}

func countMetrics(summary SummaryReport) int {
	count := 0
	for _, category := range summary.Categories {
		count += len(category.Metrics)
	}
	return count
}

func TestSummaryCSVMetricNameNormalizesUnits(t *testing.T) {
	unitless := SummaryMetric{ID: "object_count"}
	if got := summaryCSVMetricName(summaryCSVVariableName(unitless), summaryCSVUnitLabel(unitless.Unit)); got != "object_count [-]" {
		t.Fatalf("unitless CSV metric name = %q, want object_count [-]", got)
	}

	bracketed := SummaryMetric{ID: "total_wwr_percent", Unit: "[%]"}
	if got := summaryCSVMetricName(summaryCSVVariableName(bracketed), summaryCSVUnitLabel(bracketed.Unit)); got != "total_wwr [%]" {
		t.Fatalf("bracketed-unit CSV metric name = %q, want total_wwr [%%]", got)
	}
}

func metricByID(t *testing.T, summary SummaryReport, id string) SummaryMetric {
	t.Helper()
	for _, category := range summary.Categories {
		for _, metric := range category.Metrics {
			if metric.ID == id {
				return metric
			}
		}
	}
	t.Fatalf("metric %q not found", id)
	return SummaryMetric{}
}

func assertMetricClose(t *testing.T, summary SummaryReport, id string, want float64, tolerance float64) {
	t.Helper()
	metric := metricByID(t, summary, id)
	got, ok := metric.Value.(float64)
	if !ok {
		t.Fatalf("metric %s value = %#v (%T), want float64", id, metric.Value, metric.Value)
	}
	if math.Abs(got-want) > tolerance {
		t.Fatalf("metric %s = %v, want %v +/- %v", id, got, want, tolerance)
	}
}
