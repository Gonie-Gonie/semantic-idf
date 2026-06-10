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

Schedule:Constant,
  AlwaysOn,
  Fraction,
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
