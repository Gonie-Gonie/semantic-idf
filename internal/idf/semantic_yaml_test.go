package idf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildSemanticYAMLProjectionPreservesObjectMetadata(t *testing.T) {
	doc, err := Parse(`
Version,
  26.1;

Zone,
  Office;

People,
  Office People,
  Office,
  AlwaysOn,
  People,
  3;
`)
	if err != nil {
		t.Fatal(err)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{EnergyPlusVersion: "26.1", SourceFormat: "idf"})
	if projection.Schema != semanticYAMLSchema {
		t.Fatalf("schema = %q", projection.Schema)
	}
	if projection.ObjectCount != 3 {
		t.Fatalf("object count = %d, want 3", projection.ObjectCount)
	}
	if !strings.Contains(projection.Text, "semantic_energyplus_model:") ||
		!strings.Contains(projection.Text, "schema: eplus-semantic/0.2") ||
		!strings.Contains(projection.Text, "zones:") ||
		!strings.Contains(projection.Text, "loads:") ||
		!strings.Contains(projection.Text, "people:") ||
		!strings.Contains(projection.Text, "source_preservation:") {
		t.Fatalf("projection text missing expected sections:\n%s", projection.Text)
	}

	foundEditableName := false
	for _, line := range projection.Lines {
		if line.Editable && line.ObjectIndex != nil && *line.ObjectIndex == 1 && line.FieldIndex != nil && *line.FieldIndex == 0 && line.Value == "Office" {
			foundEditableName = true
		}
	}
	if !foundEditableName {
		t.Fatalf("editable Zone name line not found in %#v", projection.Lines)
	}
}

func TestSemanticYAMLParsesAsYAML(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

People,
  Office People,
  Office,
  AlwaysOn,
  People,
  3;

Schedule:Compact,
  AlwaysOn,
  Fraction,
  Through: 12/31,
  For: AllDays,
  Until: 24:00,
  1;
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(projection.Text), &parsed); err != nil {
		t.Fatalf("semantic YAML is not parseable:\n%s\n%v", projection.Text, err)
	}
	if problems := validateSemanticYAMLContract(parsed); len(problems) > 0 {
		t.Fatalf("semantic YAML contract problems: %v\n%s", problems, projection.Text)
	}
}

func TestSemanticYAMLBasicProjectionPreservesCompactLineSet(t *testing.T) {
	text, err := os.ReadFile("../../frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	if err != nil {
		t.Fatalf("read reference sample: %v", err)
	}
	doc, err := Parse(string(text))
	if err != nil {
		t.Fatalf("parse reference sample: %v", err)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	if projection.BasicVisibleLineCount != len(basicSemanticYAMLLines(projection.Lines)) {
		t.Fatalf("basic line count = %d, helper returned %d", projection.BasicVisibleLineCount, len(basicSemanticYAMLLines(projection.Lines)))
	}
	if projection.BasicVisibleLineCount < 150 {
		t.Fatalf("basic semantic visible lines = %d, want the complete compact fixture", projection.BasicVisibleLineCount)
	}
}

func TestSemanticYAMLQuantityDisplayIsReadonlyPatchSafe(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

People,
  Office People,
  Office,
  AlwaysOn,
  People,
  3;
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, line := range projection.Lines {
		if line.Key == "level" && line.ObjectType == "People" {
			if line.DisplayValue != "3 persons" || line.PatchValue != "3" || line.SourceValue != "3" {
				t.Fatalf("level line values = %#v", line)
			}
			if line.Editable {
				t.Fatalf("quantity display line must be readonly: %#v", line)
			}
			return
		}
	}
	t.Fatalf("people level line not found in %#v", projection.Lines)
}

func TestSemanticYAMLCompactQuantitiesAreReadonlyBeyondPeople(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

Lights,
  Office Lights,
  Office,
  AlwaysOn,
  LightingLevel,
  500;

ElectricEquipment,
  Office Plug Loads,
  Office,
  AlwaysOn,
  EquipmentLevel,
  750;

ZoneInfiltration:DesignFlowRate,
  Office Infiltration,
  Office,
  AlwaysOn,
  Flow/Zone,
  0.2;
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	want := map[string]string{
		"Lights":                          "500 W",
		"ElectricEquipment":               "750 W",
		"ZoneInfiltration:DesignFlowRate": "0.2 m3/s",
	}
	for objectType, display := range want {
		found := false
		for _, line := range projection.Lines {
			if line.Key == "level" && line.ObjectType == objectType {
				found = true
				if line.DisplayValue != display || line.Editable {
					t.Fatalf("%s level line = %#v, want display %q readonly", objectType, line, display)
				}
			}
		}
		if !found {
			t.Fatalf("%s readonly level line not found:\n%s", objectType, projection.Text)
		}
	}
}

func TestSemanticYAMLGroupsZoneLoadsAndOutputs(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

People,
  Office People,
  Office,
  AlwaysOn,
  People,
  3;

Output:Variable,
  Office,
  Zone Mean Air Temperature,
  Hourly;
`)
	if err != nil {
		t.Fatal(err)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{
		"zones:",
		"- name: Office",
		"loads:",
		"people:",
		"schedule: AlwaysOn",
		"level: 3 persons",
		"outputs:",
		"- \"[Hourly] Zone Mean Air Temperature\"",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic YAML missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestSemanticYAMLShowsCompactScheduleAsRules(t *testing.T) {
	doc, err := Parse(`
Schedule:Compact,
  OfficeSched,
  Fraction,
  Through: 12/31,
  For: Weekdays,
  Until: 08:00,
  0,
  Until: 18:00,
  1,
  Until: 24:00,
  0;
`)
	if err != nil {
		t.Fatal(err)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{
		"schedules:",
		"- name: OfficeSched",
		"class: \"Schedule:Compact\"",
		"type_limits: Fraction",
		"rules:",
		"- through: 12/31",
		"for: Weekdays",
		"- time: \"08:00\"",
		"value: 0",
		"- time: \"18:00\"",
		"value: 1",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic YAML missing %q:\n%s", expected, projection.Text)
		}
	}

	foundEditableUntil := false
	for _, line := range projection.Lines {
		if line.Editable && line.ObjectIndex != nil && *line.ObjectIndex == 0 && line.FieldIndex != nil && *line.FieldIndex == 6 && line.Value == "18:00" {
			foundEditableUntil = true
		}
	}
	if !foundEditableUntil {
		t.Fatalf("editable compact schedule time line not found in %#v", projection.Lines)
	}
}

func TestSemanticYAMLSummarizesScheduleUsedBy(t *testing.T) {
	doc, err := Parse(`
Schedule:Constant,
  AlwaysOn,
  Fraction,
  1;

Zone,
  Office;

People,
  Office People,            !- Name
  Office,                   !- Zone or ZoneList Name
  AlwaysOn,                 !- Number of People Schedule Name
  People,                   !- Number of People Calculation Method
  3;                        !- Number of People

Lights,
  Office Lights,            !- Name
  Office,                   !- Zone or ZoneList Name
  AlwaysOn,                 !- Schedule Name
  LightingLevel,            !- Design Level Calculation Method
  500;                      !- Lighting Level

ElectricEquipment,
  Office Plug Loads,        !- Name
  Office,                   !- Zone or ZoneList Name
  AlwaysOn,                 !- Schedule Name
  EquipmentLevel,           !- Design Level Calculation Method
  750;                      !- Design Level

ZoneControl:Thermostat,
  Office Thermostat,        !- Name
  Office,                   !- Zone or ZoneList Name
  AlwaysOn,                 !- Control Type Schedule Name
  ThermostatSetpoint:DualSetpoint, !- Control 1 Object Type
  Office Setpoint;          !- Control 1 Name
`)
	if err != nil {
		t.Fatal(err)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{
		"used_by_count: 4",
		"used_by_groups:",
		"used_by_examples:",
		"used_by_truncated_count: 1",
		"object: \"ZoneControl:Thermostat Office Thermostat\"",
		"object: ElectricEquipment Office Plug Loads",
		"object: Lights Office Lights",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic schedule used_by summary missing %q:\n%s", expected, projection.Text)
		}
	}
	if strings.Contains(projection.Text, "source_object: obj-") || strings.Contains(projection.Text, "expanded_views:") {
		t.Fatalf("semantic schedule used_by summary should not include full debug references:\n%s", projection.Text)
	}
}

func TestSemanticYAMLShowsSurfaceVerticesAsZoneGeometry(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

BuildingSurface:Detailed,
  Office Wall,              !- Name
  Wall,                     !- Surface Type
  ExtWall,                  !- Construction Name
  Office,                   !- Zone Name
  Outdoors,                 !- Outside Boundary Condition
  ,                         !- Outside Boundary Condition Object
  SunExposed,               !- Sun Exposure
  WindExposed,              !- Wind Exposure
  0.5,                      !- View Factor to Ground
  4,                        !- Number of Vertices
  0,0,3,                    !- Vertex 1
  4,0,3,                    !- Vertex 2
  4,0,0,                    !- Vertex 3
  0,0,0;                    !- Vertex 4
`)
	if err != nil {
		t.Fatal(err)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{
		"geometry:",
		"surfaces:",
		"- name: Office Wall",
		"type: Wall",
		"construction: ExtWall",
		"vertices:",
		"source: computed_geometry",
		"value: [[0,0,3], [4,0,3], [4,0,0], [0,0,0]]",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic YAML missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestSemanticYAMLPreservesUnmappedObjectsInMiscellaneous(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

Parametric:SetValueForRun,
  Some Value,
  3.14;
`)
	if err != nil {
		t.Fatal(err)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{
		"miscellaneous:",
		"other:",
		"class: \"Parametric:SetValueForRun\"",
		"reason: unmapped_object_type",
		"export_policy: preserve_exactly",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic YAML missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestSemanticDuplicateNameFixesRenameLaterDuplicates(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

Zone,
  Office;

Zone,
  Office 2;
`)
	if err != nil {
		t.Fatal(err)
	}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	if len(projection.SourceNameConflicts) != 1 {
		t.Fatalf("source name conflicts = %#v", projection.SourceNameConflicts)
	}
	if !strings.Contains(projection.Text, "source_name_conflicts:") {
		t.Fatalf("semantic YAML should report name conflicts separately:\n%s", projection.Text)
	}

	updated, fixes := ApplySemanticDuplicateNameFixes(doc)
	if len(fixes) != 1 {
		t.Fatalf("fixes = %#v", fixes)
	}
	if fixes[0].Before != "Office" || fixes[0].After != "Office 3" {
		t.Fatalf("fix = %#v, want Office -> Office 3", fixes[0])
	}
	if objectName(updated.Objects[1]) != "Office 3" {
		t.Fatalf("updated name = %q", objectName(updated.Objects[1]))
	}
}

func TestSemanticYAMLShowsAirAndPlantLoops(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "AirLoopHVAC", Fields: []Field{
			{Value: "Main Air Loop"}, {Value: ""}, {Value: ""}, {Value: "Autosize"}, {Value: "Air Branches"}, {Value: ""},
			{Value: "Air Supply Inlet"}, {Value: "Air Demand Outlet"}, {Value: "Air Demand Inlet"}, {Value: "Air Supply Outlet"},
		}},
		{Index: 1, Type: "BranchList", Fields: []Field{{Value: "Air Branches"}, {Value: "Main Air Branch"}}},
		{Index: 2, Type: "Branch", Fields: []Field{
			{Value: "Main Air Branch"}, {Value: ""}, {Value: "Fan:ConstantVolume"}, {Value: "Supply Fan"}, {Value: "Air Supply Inlet"}, {Value: "Air Supply Outlet"},
		}},
		{Index: 3, Type: "Fan:ConstantVolume", Fields: []Field{{Value: "Supply Fan", Comment: "Name"}, {Value: "Air Supply Inlet", Comment: "Air Inlet Node Name"}, {Value: "Air Supply Outlet", Comment: "Air Outlet Node Name"}}},
		{Index: 4, Type: "PlantLoop", Fields: []Field{
			{Value: "Chilled Water Loop"}, {Value: "Water"}, {Value: ""}, {Value: ""}, {Value: "CHW Setpoint"}, {Value: "15"}, {Value: "5"}, {Value: "Autosize"}, {Value: "0"}, {Value: "Autosize"},
			{Value: "Plant Supply Inlet"}, {Value: "Plant Supply Outlet"}, {Value: "Plant Supply Branches"}, {Value: ""}, {Value: "Plant Demand Inlet"}, {Value: "Plant Demand Outlet"}, {Value: ""}, {Value: ""},
		}},
		{Index: 5, Type: "BranchList", Fields: []Field{{Value: "Plant Supply Branches"}, {Value: "Plant Supply Branch"}}},
		{Index: 6, Type: "Branch", Fields: []Field{
			{Value: "Plant Supply Branch"}, {Value: ""}, {Value: "Pump:VariableSpeed"}, {Value: "CHW Pump"}, {Value: "Plant Supply Inlet"}, {Value: "Plant Supply Outlet"},
		}},
		{Index: 7, Type: "Pump:VariableSpeed", Fields: []Field{{Value: "CHW Pump", Comment: "Name"}, {Value: "Plant Supply Inlet", Comment: "Inlet Node Name"}, {Value: "Plant Supply Outlet", Comment: "Outlet Node Name"}}},
	}}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{
		"hvac:",
		"air_loops:",
		"- name: Main Air Loop",
		"plant_loops:",
		"- name: Chilled Water Loop",
		"equipment_catalog:",
		"nodes:",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic HVAC YAML missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestSemanticYAMLOutputFilesAttachmentsAndWildcards(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

Lights,
  Office Lights,
  Office,
  Lighting,
  LightingLevel,
  500;

OutputControl:Files,
  Yes,                      !- Output CSV
  No,                       !- Output MTR
  No,                       !- Output ESO
  No,                       !- Output EIO
  No,                       !- Output Tabular
  No,                       !- Output SQLite
  Yes;                      !- Output JSON

OutputControl:Table:Style,
  HTML,
  JtoKWH;

Output:Variable,
  Office Lights,
  Zone Lights Electricity Energy,
  Hourly;

Output:Variable,
  *,
  Zone Mean Air Temperature,
  Hourly;

Output:Variable,
  Missing Object,
  Zone Mean Air Temperature,
  Hourly;
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{
		"csv:",
		"enabled: true",
		"json:",
		"request_source: \"Output:JSON\"",
		"tabular:",
		"style:",
		"- \"[Hourly] Zone Lights Electricity Energy\"",
		"inherited_outputs:",
		"scope: zone_wildcard",
		"unresolved:",
		"attachment_resolution: unresolved_after_rdd",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic output YAML missing %q:\n%s", expected, projection.Text)
		}
	}
	if strings.Contains(projection.Text, "csv:\n        enabled: true\n        source: \"OutputControl:Table:Style\"") {
		t.Fatalf("csv must not be inferred from OutputControl:Table:Style:\n%s", projection.Text)
	}
}

func TestSemanticYAMLExpandsZoneListLoads(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office 1;

Zone,
  Office 2;

ZoneList,
  Offices,
  Office 1,
  Office 2;

Lights,
  Shared Lights,
  Offices,
  Lighting,
  LightingLevel,
  500;
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{
		"zone_lists:",
		"- name: Offices",
		"- name: Office 1",
		"- name: Office 2",
		"zone: Offices",
		"role_here: zone_group_expanded_load",
		"also_shown_in:",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic ZoneList YAML missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestSemanticYAMLHVACDuplicatedAsUsesOccurrencePaths(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Supply Inlet"},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: "Office Return Node"},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "AirTerminal:SingleDuct:VAV:Reheat"},
			{Value: "Office VAV"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 3, Type: "AirTerminal:SingleDuct:VAV:Reheat", Fields: []Field{
			{Value: "Office VAV", Comment: "Name"},
			{Value: "Office Terminal Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Office Supply Inlet", Comment: "Air Outlet Node Name"},
			{Value: "Coil:Heating:Water", Comment: "Reheat Coil Object Type"},
			{Value: "Office Reheat Coil", Comment: "Reheat Coil Name"},
		}},
		{Index: 4, Type: "PlantLoop", Fields: []Field{
			{Value: "Heating Water Loop"},
			{Value: "Water"},
			{Value: ""},
			{Value: ""},
			{Value: "HW Setpoint"},
			{Value: "80"},
			{Value: "20"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Autosize"},
			{Value: "HW Supply Inlet"},
			{Value: "HW Supply Outlet"},
			{Value: ""},
			{Value: ""},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
			{Value: "HW Demand Branches"},
			{Value: ""},
		}},
		{Index: 5, Type: "BranchList", Fields: []Field{
			{Value: "HW Demand Branches"},
			{Value: "Reheat Coil Branch"},
		}},
		{Index: 6, Type: "Branch", Fields: []Field{
			{Value: "Reheat Coil Branch"},
			{Value: ""},
			{Value: "Coil:Heating:Water"},
			{Value: "Office Reheat Coil"},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
		}},
		{Index: 7, Type: "Coil:Heating:Water", Fields: []Field{
			{Value: "Office Reheat Coil", Comment: "Name"},
			{Value: "HW Demand Inlet", Comment: "Water Inlet Node Name"},
			{Value: "HW Demand Outlet", Comment: "Water Outlet Node Name"},
		}},
	}}

	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{
		"- hvac/equipment_catalog/terminals/Office VAV",
		"- zones/Office/hvac/equipment/Office VAV",
		"- zones/Office/hvac/terminals/Office VAV",
		"- hvac/equipment_catalog/coils/Office Reheat Coil",
		"- hvac/plant_loops/Heating Water Loop/demand_side/branches/Reheat Coil Branch/components/Office Reheat Coil",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic duplicated_as occurrence paths missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestSemanticYAMLShowsSpacesAndDoesNotExpandSpaceListAsZones(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

Space,
  Open Office,
  Office,
  OfficeType,
  45,
  135;

SpaceList,
  Office Spaces,
  Open Office;

People,
  Space People,
  Office Spaces,
  AlwaysOn,
  People,
  7;
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{EnergyPlusVersion: "26.1"})
	for _, expected := range []string{
		"space_lists:",
		"- name: Office Spaces",
		"spaces:",
		"- name: Open Office",
		"zone_name: Office",
		"loads:",
		"people:",
		"space: Office Spaces",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic Space YAML missing %q:\n%s", expected, projection.Text)
		}
	}
	if strings.Contains(projection.Text, "zones:\n    - name: Office Spaces") {
		t.Fatalf("SpaceList must not be emitted as a zone:\n%s", projection.Text)
	}
}

func TestSemanticYAMLOutputResolutionEnvironmentWildcardsAndFileDefaults(t *testing.T) {
	doc, err := Parse(`
Zone,
  Office;

Output:Variable,
  Environment,
  Site Outdoor Air Drybulb Temperature,
  Hourly;

Output:Variable,
  *,
  System Node Temperature,
  Hourly;

Output:Variable,
  *,
  Zone Mean Air Temperature,
  Hourly;
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{})
	for _, expected := range []string{
		"attachment_resolution: environment",
		"scope: node_wildcard",
		"scope: zone_wildcard",
		"sqlite:",
		"state: default",
		"requested: false",
		"disabled: false",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic output YAML missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestSemanticYAMLContractValidationFixture(t *testing.T) {
	doc, err := Parse(`
Version,
  26.1;

Zone,
  Office;

ScheduleTypeLimits,
  Fraction,
  0,
  1,
  Continuous;

Schedule:Compact,
  AlwaysOn,
  Fraction,
  Through: 12/31,
  For: AllDays,
  Until: 24:00,
  1;

People,
  Office People,
  Office,
  AlwaysOn,
  People,
  3;
`)
	if err != nil {
		t.Fatal(err)
	}
	projection := BuildSemanticYAMLProjection(doc, SemanticYAMLMetadata{EnergyPlusVersion: "26.1", SourceFormat: "idf"})
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(projection.Text), &parsed); err != nil {
		t.Fatalf("semantic YAML is not parseable:\n%s\n%v", projection.Text, err)
	}
	if problems := validateSemanticYAMLContract(parsed); len(problems) > 0 {
		t.Fatalf("semantic YAML contract problems: %v\n%s", problems, projection.Text)
	}
	for _, expected := range []string{
		"compatibility:",
		"adapter_version:",
		"type_limits:",
		"definitions:",
		"parse_status:",
	} {
		if !strings.Contains(projection.Text, expected) {
			t.Fatalf("semantic contract fixture missing %q:\n%s", expected, projection.Text)
		}
	}
}

func TestSemanticYAMLGoldenSnapshotExactMatch(t *testing.T) {
	projection := BuildSemanticYAMLProjection(Document{}, SemanticYAMLMetadata{})
	const want = `semantic_energyplus_model:
  schema: eplus-semantic/0.2
  energyplus_version: unknown
  compatibility:
    energyplus_version: unknown
    adapter_version: manual-catalog-fallback/0.3
    schema_source: manual_fallback_catalog
  yaml_profile: strict-yaml-1.2-json-compatible
  project:
    source_format: unknown
    object_count: 0
    semantic_policy: semantic_view_over_idf_object_registry
  simulation: {}
  site: {}
  building: {}
  schedules: {}
  materials: {}
  constructions: {}
  shading: {}
  zone_lists: []
  space_lists: []
  zone_groups: []
  zones: []
  airflows: {}
  hvac: {}
  outputs:
    files:
      csv:
        enabled: false
        requested: true
        disabled: true
        state: default
        source: "OutputControl:Files"
        request_source: default
      sqlite:
        enabled: false
        requested: false
        disabled: false
        state: default
        source: "OutputControl:Files"
        request_source: "Output:SQLite"
      json:
        enabled: false
        requested: false
        disabled: false
        state: default
        source: "OutputControl:Files"
        request_source: "Output:JSON"
  source_name_conflicts: []
  miscellaneous:
    other: []
  source_preservation:
    object_order: preserved
    field_order: preserved
    comments: best_effort_from_current_parser
    mode: internal_projection
    editable_scope: visible_raw_fields_only
    roundtrip_scope: app_state_patch_not_standalone_yaml_import
    source_registry: internal_idf_document
    unmapped_policy: miscellaneous_preserve_exactly
    mapped_object_unshown_fields: []
`
	if projection.Text != want {
		t.Fatalf("semantic YAML golden mismatch\nwant:\n%s\ngot:\n%s", want, projection.Text)
	}
}

func TestSemanticYAMLGoldenFilesExistAndParse(t *testing.T) {
	dir := filepath.Join("testdata", "semantic_yaml")
	for index := 1; index <= 10; index++ {
		path := filepath.Join(dir, semanticGoldenFilename(index))
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("golden file %s missing: %v", path, err)
		}
		var parsed map[string]any
		if err := yaml.Unmarshal(content, &parsed); err != nil {
			t.Fatalf("golden file %s is not YAML: %v", path, err)
		}
		if _, ok := parsed["semantic_energyplus_model"]; !ok {
			t.Fatalf("golden file %s missing semantic_energyplus_model root", path)
		}
	}
}

func semanticGoldenFilename(index int) string {
	names := []string{
		"01_minimal_zone.golden.yaml",
		"02_zone_loads_outputs.golden.yaml",
		"03_surface_geometry.golden.yaml",
		"04_schedule_compact.golden.yaml",
		"05_airloop_vav.golden.yaml",
		"06_plantloop_chw.golden.yaml",
		"07_output_files_and_wildcards.golden.yaml",
		"08_unknown_misc.golden.yaml",
		"09_source_name_conflict.golden.yaml",
		"10_zonelist_spacelist.golden.yaml",
	}
	return names[index-1]
}

func validateSemanticYAMLContract(parsed map[string]any) []string {
	var problems []string
	root, ok := parsed["semantic_energyplus_model"].(map[string]any)
	if !ok {
		return []string{"missing semantic_energyplus_model root"}
	}
	for _, key := range []string{"schema", "compatibility", "project", "zones", "outputs", "source_preservation"} {
		if _, ok := root[key]; !ok {
			problems = append(problems, "missing "+key)
		}
	}
	if compatibility, ok := root["compatibility"].(map[string]any); ok {
		for _, key := range []string{"energyplus_version", "adapter_version", "schema_source"} {
			if _, ok := compatibility[key]; !ok {
				problems = append(problems, "missing compatibility."+key)
			}
		}
	}
	return problems
}
