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

func TestFieldCatalogExpandsExtensibleFieldSpecs(t *testing.T) {
	spec, ok := fieldSpecAt("Branch", 6)
	if !ok {
		t.Fatal("Branch second component object type field spec missing")
	}
	if spec.Name != "Component Object Type" || spec.Role != fieldRoleObjectTypeRef || spec.ExtensibleGroup != "components" || spec.Index != 6 {
		t.Fatalf("Branch repeated type spec = %#v", spec)
	}
	spec, ok = fieldSpecAt("ZoneHVAC:EquipmentList", 13)
	if !ok {
		t.Fatal("ZoneHVAC:EquipmentList repeated schedule field spec missing")
	}
	if spec.Name != "Sequential Heating Fraction Schedule Name" || spec.Role != fieldRoleScheduleRef || spec.ExtensibleGroup != "equipment" || spec.Index != 13 {
		t.Fatalf("Zone equipment repeated schedule spec = %#v", spec)
	}
	spec, ok = fieldSpecAt("AirLoopHVAC:SupplyPath", 5)
	if !ok {
		t.Fatal("AirLoopHVAC:SupplyPath repeated component name field spec missing")
	}
	if spec.Name != "Component Name" || spec.Role != fieldRoleObjectRef || spec.ExtensibleGroup != "components" || spec.Index != 5 {
		t.Fatalf("SupplyPath repeated component name spec = %#v", spec)
	}
}

func TestFieldCatalogCoversAirTerminalFamilies(t *testing.T) {
	tests := []struct {
		objectType string
		fieldIndex int
		fieldName  string
		role       string
	}{
		{
			objectType: "ZoneHVAC:AirDistributionUnit",
			fieldIndex: 2,
			fieldName:  "Air Terminal Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "AirTerminal:SingleDuct:VAV:Reheat",
			fieldIndex: 9,
			fieldName:  "Reheat Coil Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "AirTerminal:SingleDuct:VAV:Reheat",
			fieldIndex: 13,
			fieldName:  "Air Outlet Node Name",
			role:       fieldRoleNodeRef,
		},
		{
			objectType: "AirTerminal:SingleDuct:SeriesPIU:Reheat",
			fieldIndex: 10,
			fieldName:  "Reheat Coil Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "AirTerminal:DualDuct:VAV",
			fieldIndex: 4,
			fieldName:  "Cold Air Inlet Node Name",
			role:       fieldRoleNodeRef,
		},
		{
			objectType: "AirTerminal:SingleDuct:Mixer",
			fieldIndex: 1,
			fieldName:  "ZoneHVAC Unit Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:IdealLoadsAirSystem",
			fieldIndex: 2,
			fieldName:  "Zone Supply Air Node Name",
			role:       fieldRoleNodeRef,
		},
		{
			objectType: "ZoneHVAC:FourPipeFanCoil",
			fieldIndex: 14,
			fieldName:  "Cooling Coil Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:FourPipeFanCoil",
			fieldIndex: 20,
			fieldName:  "Heating Coil Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:UnitHeater",
			fieldIndex: 4,
			fieldName:  "Supply Air Fan Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:UnitHeater",
			fieldIndex: 8,
			fieldName:  "Heating Coil Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:PackagedTerminalAirConditioner",
			fieldIndex: 13,
			fieldName:  "Supply Air Fan Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:PackagedTerminalAirConditioner",
			fieldIndex: 18,
			fieldName:  "Cooling Coil Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:PackagedTerminalHeatPump",
			fieldIndex: 18,
			fieldName:  "Cooling Coil Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:PackagedTerminalHeatPump",
			fieldIndex: 22,
			fieldName:  "Supplemental Heating Coil Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:WaterToAirHeatPump",
			fieldIndex: 13,
			fieldName:  "Supply Air Fan Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:WaterToAirHeatPump",
			fieldIndex: 20,
			fieldName:  "Supplemental Heating Coil Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:UnitVentilator",
			fieldIndex: 17,
			fieldName:  "Heating Coil Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:UnitVentilator",
			fieldIndex: 21,
			fieldName:  "Cooling Coil Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:WindowAirConditioner",
			fieldIndex: 8,
			fieldName:  "Supply Air Fan Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:WindowAirConditioner",
			fieldIndex: 11,
			fieldName:  "DX Cooling Coil Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:EnergyRecoveryVentilator",
			fieldIndex: 5,
			fieldName:  "Supply Air Fan Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:Dehumidifier:DX",
			fieldIndex: 3,
			fieldName:  "Air Outlet Node Name",
			role:       fieldRoleNodeRef,
		},
		{
			objectType: "ZoneHVAC:Baseboard:Convective:Water",
			fieldIndex: 2,
			fieldName:  "Inlet Node Name",
			role:       fieldRoleNodeRef,
		},
		{
			objectType: "ZoneHVAC:Baseboard:RadiantConvective:Water",
			fieldIndex: 9,
			fieldName:  "Surface Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:Baseboard:RadiantConvective:Steam",
			fieldIndex: 8,
			fieldName:  "Surface Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:Baseboard:RadiantConvective:Electric",
			fieldIndex: 9,
			fieldName:  "Surface Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:CoolingPanel:RadiantConvective:Water",
			fieldIndex: 19,
			fieldName:  "Surface Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:LowTemperatureRadiant:VariableFlow",
			fieldIndex: 8,
			fieldName:  "Heating Water Inlet Node Name",
			role:       fieldRoleNodeRef,
		},
		{
			objectType: "ZoneHVAC:LowTemperatureRadiant:ConstantFlow",
			fieldIndex: 16,
			fieldName:  "Cooling Water Inlet Node Name",
			role:       fieldRoleNodeRef,
		},
		{
			objectType: "ZoneHVAC:LowTemperatureRadiant:Electric",
			fieldIndex: 2,
			fieldName:  "Zone Name",
			role:       fieldRoleZoneRef,
		},
		{
			objectType: "ZoneHVAC:HighTemperatureRadiant",
			fieldIndex: 16,
			fieldName:  "Surface Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:VentilatedSlab",
			fieldIndex: 30,
			fieldName:  "Fan Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:VentilatedSlab",
			fieldIndex: 32,
			fieldName:  "Heating Coil Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:OutdoorAirUnit",
			fieldIndex: 5,
			fieldName:  "Supply Fan Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:OutdoorAirUnit",
			fieldIndex: 17,
			fieldName:  "Outdoor Air Unit List Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:OutdoorAirUnit:EquipmentList",
			fieldIndex: 1,
			fieldName:  "Component Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:LowTemperatureRadiant:SurfaceGroup",
			fieldIndex: 1,
			fieldName:  "Surface Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow",
			fieldIndex: 13,
			fieldName:  "Supply Air Fan Object Type",
			role:       fieldRoleObjectTypeRef,
		},
		{
			objectType: "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow",
			fieldIndex: 18,
			fieldName:  "Cooling Coil Object Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "AirConditioner:VariableRefrigerantFlow",
			fieldIndex: 36,
			fieldName:  "Zone Terminal Unit List Name",
			role:       fieldRoleObjectRef,
		},
		{
			objectType: "ZoneTerminalUnitList",
			fieldIndex: 1,
			fieldName:  "Zone Terminal Unit Name",
			role:       fieldRoleObjectRef,
		},
	}

	for _, test := range tests {
		spec, ok := fieldSpecAt(test.objectType, test.fieldIndex)
		if !ok {
			t.Fatalf("%s field %d spec missing", test.objectType, test.fieldIndex)
		}
		if spec.Name != test.fieldName || spec.Role != test.role {
			t.Fatalf("%s field %d spec = %#v, want %s/%s", test.objectType, test.fieldIndex, spec, test.fieldName, test.role)
		}
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
