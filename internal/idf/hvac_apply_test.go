package idf

import "testing"

func TestPreviewApplyHVACAcceptsSafeCapacityEdit(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Coil:Cooling:Water", Fields: []Field{
			{Value: "Cooling Coil", Comment: "Name"},
			{Value: "Autosize", Comment: "Design Water Flow Rate"},
			{Value: "Autosize", Comment: "Design Total Cooling Capacity"},
		}},
	}}

	preview := PreviewApplyHVAC(doc, HVACApplyRequest{Changes: []HVACFieldEditRequest{
		{ObjectIndex: 0, FieldIndex: 2, Value: "12000"},
	}})
	if !preview.CanApply {
		t.Fatalf("preview.CanApply = false, warnings = %#v", preview.Warnings)
	}
	if len(preview.Changes) != 1 || preview.Changes[0].After != "12000" {
		t.Fatalf("preview changes = %#v, want one 12000 change", preview.Changes)
	}

	updated, applied := ApplyHVAC(doc, HVACApplyRequest{Changes: []HVACFieldEditRequest{
		{ObjectIndex: 0, FieldIndex: 2, Value: "12000"},
	}})
	if !applied.CanApply {
		t.Fatalf("applied.CanApply = false, warnings = %#v", applied.Warnings)
	}
	if got := updated.Objects[0].Fields[2].Value; got != "12000" {
		t.Fatalf("updated capacity = %q, want 12000", got)
	}
}

func TestPreviewApplyHVACRejectsUnsafeField(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Building", Fields: []Field{
			{Value: "Building", Comment: "Name"},
		}},
	}}

	preview := PreviewApplyHVAC(doc, HVACApplyRequest{Changes: []HVACFieldEditRequest{
		{ObjectIndex: 0, FieldIndex: 0, Value: "New Building"},
	}})
	if preview.CanApply {
		t.Fatalf("preview.CanApply = true, want false")
	}
	if !hasHVACWarningCode(preview.Warnings, "unsafe_hvac_field") {
		t.Fatalf("warnings = %#v, want unsafe_hvac_field", preview.Warnings)
	}
}

func TestPreviewApplyHVACRejectsInvalidNumber(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Fan:ConstantVolume", Fields: []Field{
			{Value: "Supply Fan", Comment: "Name"},
			{Value: "Always On", Comment: "Availability Schedule Name"},
			{Value: "0.7", Comment: "Fan Total Efficiency"},
			{Value: "500", Comment: "Pressure Rise"},
			{Value: "Autosize", Comment: "Maximum Flow Rate"},
		}},
	}}

	preview := PreviewApplyHVAC(doc, HVACApplyRequest{Changes: []HVACFieldEditRequest{
		{ObjectIndex: 0, FieldIndex: 4, Value: "fast"},
	}})
	if preview.CanApply {
		t.Fatalf("preview.CanApply = true, want false")
	}
	if !hasHVACWarningCode(preview.Warnings, "invalid_hvac_number") {
		t.Fatalf("warnings = %#v, want invalid_hvac_number", preview.Warnings)
	}
}

func TestApplyHVACAddsNodeOutputVariable(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "NodeList", Fields: []Field{
			{Value: "Supply Nodes", Comment: "Name"},
			{Value: "Supply Outlet Node", Comment: "Node 1 Name"},
		}},
	}}

	updated, preview := ApplyHVAC(doc, HVACApplyRequest{OutputVariables: []HVACOutputVariableRequest{
		{
			KeyValue:           "Supply Outlet Node",
			VariableName:       "System Node Temperature",
			ReportingFrequency: "Timestep",
		},
	}})
	if !preview.CanApply {
		t.Fatalf("preview warnings = %#v", preview.Warnings)
	}
	if len(updated.Objects) != 2 {
		t.Fatalf("object count = %d, want 2", len(updated.Objects))
	}
	output := updated.Objects[1]
	if output.Type != "Output:Variable" {
		t.Fatalf("new object type = %s, want Output:Variable", output.Type)
	}
	if got := output.Fields[0].Value; got != "Supply Outlet Node" {
		t.Fatalf("key value = %q, want node name", got)
	}
	if got := output.Fields[1].Value; got != "System Node Temperature" {
		t.Fatalf("variable = %q, want System Node Temperature", got)
	}
	if got := output.Fields[2].Value; got != "Timestep" {
		t.Fatalf("frequency = %q, want Timestep", got)
	}
}

func TestApplyHVACSkipsDuplicateNodeOutputVariable(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Output:Variable", Fields: []Field{
			{Value: "Supply Outlet Node", Comment: "Key Value"},
			{Value: "System Node Temperature", Comment: "Variable Name"},
			{Value: "Hourly", Comment: "Reporting Frequency"},
		}},
	}}

	updated, preview := ApplyHVAC(doc, HVACApplyRequest{OutputVariables: []HVACOutputVariableRequest{
		{
			KeyValue:     "Supply Outlet Node",
			VariableName: "System Node Temperature",
		},
	}})
	if !preview.CanApply {
		t.Fatalf("preview warnings = %#v", preview.Warnings)
	}
	if len(updated.Objects) != 1 {
		t.Fatalf("object count = %d, want no duplicate", len(updated.Objects))
	}
	if len(preview.Changes) != 1 || preview.Changes[0].Action != "no_change" {
		t.Fatalf("changes = %#v, want no_change", preview.Changes)
	}
}
