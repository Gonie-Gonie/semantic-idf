package idf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFieldCatalogFindsFieldsWithoutComments(t *testing.T) {
	obj := Object{
		Type: "AirLoopHVAC",
		Fields: []Field{
			{Value: "Main Air Loop"},
			{Value: "Controllers"},
			{Value: "Availability"},
			{Value: "autosize"},
			{Value: "Main Branches"},
			{Value: "Main Connectors"},
			{Value: "Supply Inlet"},
		},
	}

	if got := fieldValueByCatalogName(obj, "Branch List Name"); got != "Main Branches" {
		t.Fatalf("Branch List Name = %q, want Main Branches", got)
	}
	if got := catalogFieldRole(obj, 6); got != fieldRoleNodeRef {
		t.Fatalf("field role = %q, want node ref", got)
	}
}

func TestFieldCatalogCarriesReferenceTargetMetadata(t *testing.T) {
	spec, ok := fieldSpecAt("BuildingSurface:Detailed", 2)
	if !ok {
		t.Fatal("BuildingSurface:Detailed construction field spec missing")
	}
	if spec.Role != fieldRoleConstructionRef || spec.TargetClass != "construction" || spec.TargetCollection != "constructions" || spec.RelationshipType != "uses" {
		t.Fatalf("construction field metadata = %#v", spec)
	}
	spec, ok = fieldSpecAt("Space", 1)
	if !ok {
		t.Fatal("Space zone field spec missing")
	}
	if spec.Role != fieldRoleZoneRef || spec.TargetClass != "zone" || spec.TargetCollection != "zones" {
		t.Fatalf("space zone metadata = %#v", spec)
	}
}

func TestLoadEnergyPlusSchemaAdapterFile(t *testing.T) {
	root := t.TempDir()
	schemaPath := filepath.Join(root, "Energy+.schema.epJSON")
	if err := os.WriteFile(schemaPath, []byte(`{
  "properties": {
    "Custom:Object": {
      "required": ["name"],
      "legacy_idd": {
        "fields": ["name", "mode", "flow_rate"],
        "field_info": {
          "name": {"field_name": "Name"},
          "mode": {"field_name": "Operating Mode"},
          "flow_rate": {"field_name": "Design Flow Rate"}
        }
      },
      "properties": {
        "name": {"type": "string"},
        "flow_rate": {"type": "number", "units": "m3/s", "autosizable": true},
        "mode": {"type": "string", "enum": ["A", "B"]}
      }
    }
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter, err := loadEnergyPlusSchemaAdapterFile("26.1", schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	spec := adapter.Objects[normalizeFieldCatalogKey("Custom:Object")]
	if spec.Type != "Custom:Object" || adapter.AdapterVersion != "epjson-schema/1" {
		t.Fatalf("adapter/spec = %#v / %#v", adapter, spec)
	}
	if len(spec.Fields) != 3 {
		t.Fatalf("field count = %d, want 3", len(spec.Fields))
	}
	if spec.Fields[0].Name != "Name" || spec.Fields[1].Name != "Operating Mode" || spec.Fields[2].Name != "Design Flow Rate" {
		t.Fatalf("schema field order/names = %#v, want legacy IDD order and field names", spec.Fields)
	}
	var flow FieldSpec
	for _, field := range spec.Fields {
		if field.Name == "Design Flow Rate" {
			flow = field
		}
	}
	if !flow.Numeric || !flow.AllowAutosize || flow.Units != "m3/s" {
		t.Fatalf("flow metadata = %#v", flow)
	}
}

func TestFieldCatalogFallsBackToComments(t *testing.T) {
	obj := Object{
		Type: "Custom:Object",
		Fields: []Field{
			{Value: "Obj", Comment: "Name"},
			{Value: "Node A", Comment: "Air Inlet Node Name"},
		},
	}

	if got := fieldValueByCatalogName(obj, "Air Inlet Node Name"); got != "Node A" {
		t.Fatalf("comment fallback = %q, want Node A", got)
	}
}

func TestSuggestFieldValuesUsesCatalogAndDocument(t *testing.T) {
	doc := Document{Objects: []Object{
		{
			Index: 0,
			Type:  "BranchList",
			Fields: []Field{
				{Value: "Main Branches"},
			},
		},
		{
			Index: 1,
			Type:  "AirLoopHVAC",
			Fields: []Field{
				{Value: "Main Air Loop"},
				{Value: ""},
				{Value: ""},
				{Value: "Autosize"},
				{Value: ""},
			},
		},
	}}

	suggestions := SuggestFieldValues(doc, 1, 4)
	if !hasSuggestionValue(suggestions, "Main Branches") {
		t.Fatalf("suggestions = %#v, want Main Branches", suggestions)
	}
}

func TestFieldCatalogDiagnosticsValidatesChoicesAndNumbers(t *testing.T) {
	doc := Document{Objects: []Object{
		{
			Index: 0,
			Type:  "PlantLoop",
			Fields: []Field{
				{Value: "Hot Water Loop"},
				{Value: "Coffee"},
				{Value: ""},
				{Value: ""},
				{Value: "Loop Setpoint"},
				{Value: "warm"},
			},
		},
	}}

	diagnostics := fieldCatalogDiagnostics(doc)
	if !hasDiagnosticCode(diagnostics, "invalid_choice") {
		t.Fatalf("diagnostics = %#v, want invalid_choice", diagnostics)
	}
	if !hasDiagnosticCode(diagnostics, "invalid_number") {
		t.Fatalf("diagnostics = %#v, want invalid_number", diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Source != "energyplus_rule" || diagnostic.Confidence == "" {
			t.Fatalf("diagnostic metadata = %#v, want energyplus_rule with confidence", diagnostic)
		}
	}
}

func hasSuggestionValue(suggestions []FieldValueSuggestion, value string) bool {
	for _, suggestion := range suggestions {
		if suggestion.Value == value {
			return true
		}
	}
	return false
}

func hasDiagnosticCode(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
