package idf

import (
	"fmt"
	"strconv"
	"strings"
)

type HVACApplyRequest struct {
	Changes         []HVACFieldEditRequest      `json:"changes"`
	OutputVariables []HVACOutputVariableRequest `json:"outputVariables,omitempty"`
}

type HVACFieldEditRequest struct {
	ObjectIndex int    `json:"objectIndex"`
	FieldIndex  int    `json:"fieldIndex"`
	Value       string `json:"value"`
	Reason      string `json:"reason,omitempty"`
}

type HVACOutputVariableRequest struct {
	KeyValue           string `json:"keyValue"`
	VariableName       string `json:"variableName"`
	ReportingFrequency string `json:"reportingFrequency,omitempty"`
	ScheduleName       string `json:"scheduleName,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type HVACApplyPreview struct {
	CanApply bool              `json:"canApply"`
	Changes  []HVACApplyChange `json:"changes"`
	Warnings []HVACWarning     `json:"warnings,omitempty"`
}

type HVACApplyChange struct {
	Action       string `json:"action"`
	ObjectIndex  int    `json:"objectIndex"`
	ObjectType   string `json:"objectType,omitempty"`
	ObjectName   string `json:"objectName,omitempty"`
	FieldIndex   int    `json:"fieldIndex"`
	FieldName    string `json:"fieldName,omitempty"`
	EditKind     string `json:"editKind,omitempty"`
	Before       string `json:"before,omitempty"`
	After        string `json:"after,omitempty"`
	Message      string `json:"message"`
	RequiresSave bool   `json:"requiresSave"`
}

func PreviewApplyHVAC(doc Document, request HVACApplyRequest) HVACApplyPreview {
	_, preview := applyHVAC(doc, request, false)
	return preview
}

func ApplyHVAC(doc Document, request HVACApplyRequest) (Document, HVACApplyPreview) {
	return applyHVAC(doc, request, true)
}

func applyHVAC(doc Document, request HVACApplyRequest, mutate bool) (Document, HVACApplyPreview) {
	updated := doc.clone()
	preview := HVACApplyPreview{CanApply: true}
	if len(request.Changes) == 0 && len(request.OutputVariables) == 0 {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, HVACWarning{
			Severity: DiagnosticWarning,
			Category: "HVAC Apply",
			Code:     "missing_hvac_changes",
			Message:  "No HVAC changes were selected.",
		})
		return updated, preview
	}

	seen := map[string]bool{}
	for _, change := range request.Changes {
		key := fmt.Sprintf("%d:%d", change.ObjectIndex, change.FieldIndex)
		if seen[key] {
			preview.CanApply = false
			preview.Warnings = append(preview.Warnings, HVACWarning{
				Severity: DiagnosticError,
				Category: "HVAC Apply",
				Code:     "duplicate_hvac_change",
				Message:  fmt.Sprintf("Field %s was selected more than once.", key),
			})
			continue
		}
		seen[key] = true
		applyHVACFieldChange(&updated, doc, change, mutate, &preview)
	}
	for _, outputVariable := range request.OutputVariables {
		applyHVACOutputVariable(&updated, doc, outputVariable, mutate, &preview)
	}
	return updated, preview
}

func applyHVACFieldChange(updated *Document, original Document, request HVACFieldEditRequest, mutate bool, preview *HVACApplyPreview) {
	if request.ObjectIndex < 0 || request.ObjectIndex >= len(original.Objects) {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, HVACWarning{
			Severity: DiagnosticError,
			Category: "HVAC Apply",
			Code:     "hvac_object_out_of_range",
			Message:  fmt.Sprintf("Object index %d is out of range.", request.ObjectIndex),
		})
		return
	}
	obj := original.Objects[request.ObjectIndex]
	if request.FieldIndex < 0 || request.FieldIndex >= len(obj.Fields) {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, hvacWarningForObject(obj, "hvac_field_out_of_range",
			fmt.Sprintf("Field index %d is out of range for %s.", request.FieldIndex, objectLabel(obj))))
		return
	}
	editField, ok := hvacEditableFieldAt(original, obj, request.FieldIndex)
	if !ok {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, hvacWarningForObject(obj, "unsafe_hvac_field",
			fmt.Sprintf("%s field %d is not in the safe HVAC edit set.", objectLabel(obj), request.FieldIndex+1)))
		return
	}
	nextValue := strings.TrimSpace(request.Value)
	currentValue := strings.TrimSpace(obj.Fields[request.FieldIndex].Value)
	if currentValue == nextValue {
		preview.Changes = append(preview.Changes, HVACApplyChange{
			Action:       "no_change",
			ObjectIndex:  obj.Index,
			ObjectType:   obj.Type,
			ObjectName:   objectName(obj),
			FieldIndex:   request.FieldIndex,
			FieldName:    editField.FieldName,
			EditKind:     editField.EditKind,
			Before:       currentValue,
			After:        nextValue,
			Message:      fmt.Sprintf("%s stays at %q.", editField.FieldName, currentValue),
			RequiresSave: false,
		})
		return
	}

	if warnings := validateHVACEditValue(original, obj, editField, nextValue); len(warnings) > 0 {
		for _, warning := range warnings {
			if warning.Severity == DiagnosticError {
				preview.CanApply = false
			}
			preview.Warnings = append(preview.Warnings, warning)
		}
		if !preview.CanApply {
			return
		}
	}

	preview.Changes = append(preview.Changes, HVACApplyChange{
		Action:       "update_field",
		ObjectIndex:  obj.Index,
		ObjectType:   obj.Type,
		ObjectName:   objectName(obj),
		FieldIndex:   request.FieldIndex,
		FieldName:    editField.FieldName,
		EditKind:     editField.EditKind,
		Before:       currentValue,
		After:        nextValue,
		Message:      fmt.Sprintf("Update %s on %s from %q to %q.", editField.FieldName, objectLabel(obj), currentValue, nextValue),
		RequiresSave: true,
	})
	if mutate {
		updated.Objects[request.ObjectIndex].Fields[request.FieldIndex].Value = nextValue
	}
}

func applyHVACOutputVariable(updated *Document, original Document, request HVACOutputVariableRequest, mutate bool, preview *HVACApplyPreview) {
	keyValue := strings.TrimSpace(request.KeyValue)
	variableName := strings.TrimSpace(request.VariableName)
	frequency := strings.TrimSpace(request.ReportingFrequency)
	scheduleName := strings.TrimSpace(request.ScheduleName)
	if frequency == "" {
		frequency = defaultHVACOutputFrequency
	}
	if keyValue == "" {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, HVACWarning{
			Severity: DiagnosticError,
			Category: "HVAC Apply",
			Code:     "missing_output_key_value",
			Message:  "Output:Variable monitor needs a node name as the key value.",
		})
		return
	}
	if !validHVACNodeOutputVariable(variableName) {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, HVACWarning{
			Severity: DiagnosticError,
			Category: "HVAC Apply",
			Code:     "unsupported_node_output_variable",
			Message:  fmt.Sprintf("%q is not in the built-in EnergyPlus node output variable list.", variableName),
			Value:    variableName,
		})
		return
	}
	if variable, ok := hvacNodeOutputVariableByName(variableName); ok && variable.Advanced {
		preview.Warnings = append(preview.Warnings, HVACWarning{
			Severity: DiagnosticWarning,
			Category: "HVAC Apply",
			Code:     "advanced_node_output_variable",
			Message:  fmt.Sprintf("%q may require Diagnostics,DisplayAdvancedReportVariable to appear in EnergyPlus output.", variableName),
			Value:    variableName,
		})
	}
	if !validHVACOutputFrequency(frequency) {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, HVACWarning{
			Severity: DiagnosticError,
			Category: "HVAC Apply",
			Code:     "unsupported_output_frequency",
			Message:  fmt.Sprintf("%q is not a valid Output:Variable reporting frequency.", frequency),
			Value:    frequency,
		})
		return
	}
	frequency = canonicalHVACOutputFrequency(frequency)
	if existing, ok := findExistingOutputVariable(original, keyValue, variableName, frequency, scheduleName); ok {
		preview.Changes = append(preview.Changes, HVACApplyChange{
			Action:       "no_change",
			ObjectIndex:  existing.Index,
			ObjectType:   existing.Type,
			ObjectName:   objectName(existing),
			FieldName:    "Output:Variable",
			EditKind:     "output_variable",
			After:        variableName,
			Message:      fmt.Sprintf("Output:Variable for %q / %q at %s already exists.", keyValue, variableName, frequency),
			RequiresSave: false,
		})
		return
	}
	objectIndex := len(updated.Objects)
	preview.Changes = append(preview.Changes, HVACApplyChange{
		Action:       "add_output_variable",
		ObjectIndex:  objectIndex,
		ObjectType:   "Output:Variable",
		FieldName:    "Output:Variable",
		EditKind:     "output_variable",
		After:        variableName,
		Message:      fmt.Sprintf("Add Output:Variable for node %q: %s at %s.", keyValue, variableName, frequency),
		RequiresSave: true,
	})
	if !mutate {
		return
	}
	fields := []Field{
		{Value: keyValue, Comment: "Key Value"},
		{Value: variableName, Comment: "Variable Name"},
		{Value: frequency, Comment: "Reporting Frequency"},
	}
	if scheduleName != "" {
		fields = append(fields, Field{Value: scheduleName, Comment: "Schedule Name"})
	}
	updated.Objects = append(updated.Objects, Object{
		Index:  objectIndex,
		Type:   "Output:Variable",
		Fields: fields,
	})
}

func findExistingOutputVariable(doc Document, keyValue string, variableName string, frequency string, scheduleName string) (Object, bool) {
	for _, obj := range doc.Objects {
		if !strings.EqualFold(obj.Type, "Output:Variable") {
			continue
		}
		existingFrequency := hvacFieldValue(obj, 2)
		if existingFrequency == "" {
			existingFrequency = defaultHVACOutputFrequency
		}
		if strings.EqualFold(hvacFieldValue(obj, 0), keyValue) &&
			strings.EqualFold(hvacFieldValue(obj, 1), variableName) &&
			strings.EqualFold(canonicalHVACOutputFrequency(existingFrequency), canonicalHVACOutputFrequency(frequency)) &&
			strings.EqualFold(hvacFieldValue(obj, 3), scheduleName) {
			return obj, true
		}
	}
	return Object{}, false
}

func validateHVACEditValue(doc Document, obj Object, editField HVACEditField, value string) []HVACWarning {
	var warnings []HVACWarning
	if strings.TrimSpace(value) == "" {
		return append(warnings, HVACWarning{
			Severity:    DiagnosticWarning,
			Category:    "HVAC Apply",
			Code:        "blank_hvac_value",
			Message:     fmt.Sprintf("%s will be cleared.", editField.FieldName),
			ObjectIndex: obj.Index,
			ObjectType:  obj.Type,
			ObjectName:  objectName(obj),
			FieldIndex:  editField.FieldIndex,
			Field:       editField.FieldName,
			Value:       value,
		})
	}
	switch editField.ValueType {
	case "number":
		if editField.AllowAutosize && isFlexibleSizingValue(value) {
			return warnings
		}
		if _, ok := parseFloatField(value); !ok {
			warnings = append(warnings, HVACWarning{
				Severity:    DiagnosticError,
				Category:    "HVAC Apply",
				Code:        "invalid_hvac_number",
				Message:     fmt.Sprintf("%s expects a numeric value or Autosize.", editField.FieldName),
				ObjectIndex: obj.Index,
				ObjectType:  obj.Type,
				ObjectName:  objectName(obj),
				FieldIndex:  editField.FieldIndex,
				Field:       editField.FieldName,
				Value:       value,
			})
		}
	case "integer":
		if parsed, ok := parseIntField(value); !ok || parsed < 1 {
			warnings = append(warnings, HVACWarning{
				Severity:    DiagnosticError,
				Category:    "HVAC Apply",
				Code:        "invalid_hvac_integer",
				Message:     fmt.Sprintf("%s expects a positive integer sequence.", editField.FieldName),
				ObjectIndex: obj.Index,
				ObjectType:  obj.Type,
				ObjectName:  objectName(obj),
				FieldIndex:  editField.FieldIndex,
				Field:       editField.FieldName,
				Value:       value,
			})
		}
	case "reference":
		if strings.Contains(editField.EditKind, "schedule") && !hvacScheduleExists(doc, value) {
			warnings = append(warnings, HVACWarning{
				Severity:    DiagnosticWarning,
				Category:    "HVAC Apply",
				Code:        "missing_hvac_schedule",
				Message:     fmt.Sprintf("Schedule %q was not found; EnergyPlus may reject this reference.", value),
				ObjectIndex: obj.Index,
				ObjectType:  obj.Type,
				ObjectName:  objectName(obj),
				FieldIndex:  editField.FieldIndex,
				Field:       editField.FieldName,
				Value:       value,
			})
		}
	}
	return warnings
}

func hvacScheduleExists(doc Document, value string) bool {
	for _, obj := range doc.Objects {
		if isScheduleType(obj.Type) && strings.EqualFold(objectName(obj), strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func parseIntField(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
