package idf

import (
	"fmt"
	"strings"
)

type OutputApplyRequest struct {
	Preset                  string                `json:"preset,omitempty"`
	PresetMode              string                `json:"presetMode,omitempty"`
	PresetRecommendationIDs []string              `json:"presetRecommendationIds,omitempty"`
	AddRecommendations      []string              `json:"addRecommendations,omitempty"`
	AddObjects              []OutputObjectRequest `json:"addObjects,omitempty"`
	Updates                 []OutputFieldUpdate   `json:"updates,omitempty"`
	RemoveObjectIndexes     []int                 `json:"removeObjectIndexes,omitempty"`
}

type OutputObjectRequest struct {
	ObjectType string             `json:"objectType"`
	Fields     []OutputFieldValue `json:"fields"`
	Reason     string             `json:"reason,omitempty"`
}

type OutputFieldUpdate struct {
	ObjectIndex int    `json:"objectIndex"`
	FieldIndex  int    `json:"fieldIndex"`
	Value       string `json:"value"`
}

type OutputApplyPreview struct {
	CanApply bool                `json:"canApply"`
	Changes  []OutputApplyChange `json:"changes"`
	Warnings []Diagnostic        `json:"warnings,omitempty"`
}

type OutputApplyChange struct {
	Action       string `json:"action"`
	ObjectIndex  int    `json:"objectIndex"`
	ObjectType   string `json:"objectType,omitempty"`
	FieldIndex   int    `json:"fieldIndex,omitempty"`
	FieldName    string `json:"fieldName,omitempty"`
	Before       string `json:"before,omitempty"`
	After        string `json:"after,omitempty"`
	Message      string `json:"message"`
	RequiresSave bool   `json:"requiresSave"`
}

func PreviewApplyOutput(doc Document, request OutputApplyRequest) OutputApplyPreview {
	_, preview := applyOutput(doc, request, false)
	return preview
}

func ApplyOutput(doc Document, request OutputApplyRequest) (Document, OutputApplyPreview) {
	return applyOutput(doc, request, true)
}

func applyOutput(doc Document, request OutputApplyRequest, mutate bool) (Document, OutputApplyPreview) {
	request = expandOutputPresetRequest(doc, request)
	updated := doc.clone()
	preview := OutputApplyPreview{CanApply: true}
	if len(request.AddRecommendations) == 0 && len(request.AddObjects) == 0 && len(request.Updates) == 0 && len(request.RemoveObjectIndexes) == 0 {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, outputApplyDiagnostic(DiagnosticWarning, "missing_output_changes", "No output changes were selected."))
		return updated, preview
	}

	recommendations := map[string]OutputRecommendation{}
	for _, item := range outputRecommendations(doc, AnalyzeOutput(doc).Existing) {
		recommendations[item.ID] = item
	}
	for _, id := range request.AddRecommendations {
		item, ok := recommendations[strings.TrimSpace(id)]
		if !ok {
			preview.CanApply = false
			preview.Warnings = append(preview.Warnings, outputApplyDiagnostic(DiagnosticError, "unknown_output_recommendation", fmt.Sprintf("Unknown output recommendation %q.", id)))
			continue
		}
		applyOutputAddObject(&updated, doc, OutputObjectRequest{ObjectType: item.ObjectType, Fields: item.Fields, Reason: item.Label}, mutate, &preview)
	}
	for _, add := range request.AddObjects {
		applyOutputAddObject(&updated, doc, add, mutate, &preview)
	}
	for _, update := range request.Updates {
		applyOutputFieldUpdate(&updated, doc, update, mutate, &preview)
	}
	for _, objectIndex := range request.RemoveObjectIndexes {
		applyOutputRemoveObject(&updated, doc, objectIndex, mutate, &preview)
	}
	if mutate {
		reindexObjects(&updated)
	}
	return updated, preview
}

func applyOutputAddObject(updated *Document, original Document, request OutputObjectRequest, mutate bool, preview *OutputApplyPreview) {
	objectType := strings.TrimSpace(request.ObjectType)
	if !isOutputManagementType(objectType) {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, outputApplyDiagnostic(DiagnosticError, "unsupported_output_object", fmt.Sprintf("%q is not an Output:* or OutputControl:* object.", objectType)))
		return
	}
	fields := normalizeOutputRequestFields(objectType, request.Fields)
	if len(fields) == 0 {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, outputApplyDiagnostic(DiagnosticError, "missing_output_fields", fmt.Sprintf("%s needs at least one field.", objectType)))
		return
	}
	if warnings := validateOutputFields(objectType, fields); len(warnings) > 0 {
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
	signature := outputObjectSignature(objectType, fields)
	for _, obj := range original.Objects {
		if !isOutputManagementType(obj.Type) {
			continue
		}
		if outputObjectSignature(obj.Type, outputFieldValues(obj)) == signature {
			preview.Changes = append(preview.Changes, OutputApplyChange{
				Action:       "no_change",
				ObjectIndex:  obj.Index,
				ObjectType:   obj.Type,
				Message:      fmt.Sprintf("%s already exists.", outputObjectLabel(objectType, fields)),
				RequiresSave: false,
			})
			return
		}
	}
	if applyOutputMergeUniqueObject(updated, original, objectType, fields, mutate, preview) {
		return
	}
	objectIndex := len(updated.Objects)
	preview.Changes = append(preview.Changes, OutputApplyChange{
		Action:       "add_output",
		ObjectIndex:  objectIndex,
		ObjectType:   objectType,
		After:        outputObjectLabel(objectType, fields),
		Message:      fmt.Sprintf("Add %s.", outputObjectLabel(objectType, fields)),
		RequiresSave: true,
	})
	if !mutate {
		return
	}
	updated.Objects = append(updated.Objects, Object{
		Index:  objectIndex,
		Type:   objectType,
		Fields: outputFieldsForObject(objectType, fields),
	})
}

func applyOutputMergeUniqueObject(updated *Document, original Document, objectType string, fields []OutputFieldValue, mutate bool, preview *OutputApplyPreview) bool {
	typeKey := normalizeFieldCatalogKey(objectType)
	switch typeKey {
	case "output:table:summaryreports", "output:diagnostics":
		return applyOutputMergeExtensibleUniqueObject(updated, original, objectType, fields, mutate, preview)
	case "output:sqlite", "output:json", "outputcontrol:files", "outputcontrol:table:style":
		if existing, ok := firstOutputObjectOfType(original, objectType); ok {
			preview.Changes = append(preview.Changes, OutputApplyChange{
				Action:       "no_change",
				ObjectIndex:  existing.Index,
				ObjectType:   existing.Type,
				After:        outputObjectLabel(objectType, fields),
				Message:      fmt.Sprintf("%s already has a unique %s object; not adding a duplicate.", objectLabel(existing), objectType),
				RequiresSave: false,
			})
			return true
		}
	}
	return false
}

func applyOutputMergeExtensibleUniqueObject(updated *Document, original Document, objectType string, fields []OutputFieldValue, mutate bool, preview *OutputApplyPreview) bool {
	existing, ok := firstOutputObjectOfType(original, objectType)
	if !ok {
		return false
	}
	existingValues := map[string]bool{}
	for _, field := range existing.Fields {
		if value := normalizeName(field.Value); value != "" {
			existingValues[value] = true
		}
	}
	missing := make([]OutputFieldValue, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field.Value)
		if value == "" || existingValues[normalizeName(value)] {
			continue
		}
		missing = append(missing, OutputFieldValue{Name: outputExtensibleFieldName(objectType, len(existing.Fields)+len(missing)), Value: value})
		existingValues[normalizeName(value)] = true
	}
	if len(missing) == 0 {
		preview.Changes = append(preview.Changes, OutputApplyChange{
			Action:       "no_change",
			ObjectIndex:  existing.Index,
			ObjectType:   existing.Type,
			After:        outputObjectLabel(objectType, fields),
			Message:      fmt.Sprintf("%s already includes %s.", objectLabel(existing), outputObjectLabel(objectType, fields)),
			RequiresSave: false,
		})
		return true
	}
	for index, field := range missing {
		preview.Changes = append(preview.Changes, OutputApplyChange{
			Action:       "update_field",
			ObjectIndex:  existing.Index,
			ObjectType:   existing.Type,
			FieldIndex:   len(existing.Fields) + index,
			FieldName:    field.Name,
			After:        field.Value,
			Message:      fmt.Sprintf("Add %s to %s.", field.Value, objectLabel(existing)),
			RequiresSave: true,
		})
	}
	if mutate {
		if updatedIndex, ok := objectIndexByOriginalIndex(*updated, existing.Index); ok {
			for _, field := range missing {
				updated.Objects[updatedIndex].Fields = append(updated.Objects[updatedIndex].Fields, Field{
					Value:   field.Value,
					Comment: field.Name,
				})
			}
		}
	}
	return true
}

func firstOutputObjectOfType(doc Document, objectType string) (Object, bool) {
	typeKey := normalizeFieldCatalogKey(objectType)
	for _, obj := range doc.Objects {
		if normalizeFieldCatalogKey(obj.Type) == typeKey {
			return obj, true
		}
	}
	return Object{}, false
}

func objectIndexByOriginalIndex(doc Document, objectIndex int) (int, bool) {
	for index, obj := range doc.Objects {
		if obj.Index == objectIndex {
			return index, true
		}
	}
	return -1, false
}

func outputExtensibleFieldName(objectType string, fieldIndex int) string {
	switch normalizeFieldCatalogKey(objectType) {
	case "output:table:summaryreports":
		return fmt.Sprintf("Report %d Name", fieldIndex+1)
	case "output:diagnostics":
		return fmt.Sprintf("Key %d", fieldIndex+1)
	default:
		return outputDefaultFieldName(objectType, fieldIndex)
	}
}

func applyOutputFieldUpdate(updated *Document, original Document, request OutputFieldUpdate, mutate bool, preview *OutputApplyPreview) {
	if request.ObjectIndex < 0 || request.ObjectIndex >= len(original.Objects) {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, outputApplyDiagnostic(DiagnosticError, "output_object_out_of_range", fmt.Sprintf("Object index %d is out of range.", request.ObjectIndex)))
		return
	}
	obj := original.Objects[request.ObjectIndex]
	if !isOutputManagementType(obj.Type) {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, outputApplyDiagnostic(DiagnosticError, "not_output_object", fmt.Sprintf("%s is not an output-management object.", objectLabel(obj))))
		return
	}
	if request.FieldIndex < 0 || request.FieldIndex >= len(obj.Fields) {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, outputApplyDiagnostic(DiagnosticError, "output_field_out_of_range", fmt.Sprintf("Field index %d is out of range for %s.", request.FieldIndex, objectLabel(obj))))
		return
	}
	fieldName := outputFieldName(obj, request.FieldIndex)
	nextValue := strings.TrimSpace(request.Value)
	currentValue := strings.TrimSpace(obj.Fields[request.FieldIndex].Value)
	if currentValue == nextValue {
		preview.Changes = append(preview.Changes, OutputApplyChange{
			Action:       "no_change",
			ObjectIndex:  obj.Index,
			ObjectType:   obj.Type,
			FieldIndex:   request.FieldIndex,
			FieldName:    fieldName,
			Before:       currentValue,
			After:        nextValue,
			Message:      fmt.Sprintf("%s stays at %q.", fieldName, currentValue),
			RequiresSave: false,
		})
		return
	}
	updateFields := outputFieldValues(obj)
	for index := range updateFields {
		if index == request.FieldIndex {
			updateFields[index].Value = nextValue
		}
	}
	if warnings := validateOutputFields(obj.Type, updateFields); len(warnings) > 0 {
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
	preview.Changes = append(preview.Changes, OutputApplyChange{
		Action:       "update_field",
		ObjectIndex:  obj.Index,
		ObjectType:   obj.Type,
		FieldIndex:   request.FieldIndex,
		FieldName:    fieldName,
		Before:       currentValue,
		After:        nextValue,
		Message:      fmt.Sprintf("Update %s on %s from %q to %q.", fieldName, objectLabel(obj), currentValue, nextValue),
		RequiresSave: true,
	})
	if mutate {
		updated.Objects[request.ObjectIndex].Fields[request.FieldIndex].Value = nextValue
	}
}

func applyOutputRemoveObject(updated *Document, original Document, objectIndex int, mutate bool, preview *OutputApplyPreview) {
	if objectIndex < 0 || objectIndex >= len(original.Objects) {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, outputApplyDiagnostic(DiagnosticError, "output_remove_out_of_range", fmt.Sprintf("Object index %d is out of range.", objectIndex)))
		return
	}
	obj := original.Objects[objectIndex]
	if !isOutputManagementType(obj.Type) {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, outputApplyDiagnostic(DiagnosticError, "remove_not_output_object", fmt.Sprintf("%s is not an output-management object.", objectLabel(obj))))
		return
	}
	preview.Changes = append(preview.Changes, OutputApplyChange{
		Action:       "remove_output",
		ObjectIndex:  obj.Index,
		ObjectType:   obj.Type,
		Before:       summarizeOutputObject(obj).Summary,
		Message:      fmt.Sprintf("Remove %s.", objectLabel(obj)),
		RequiresSave: true,
	})
	if !mutate {
		return
	}
	objects := updated.Objects[:0]
	for _, candidate := range updated.Objects {
		if candidate.Index == objectIndex {
			continue
		}
		objects = append(objects, candidate)
	}
	updated.Objects = objects
}

func normalizeOutputRequestFields(objectType string, fields []OutputFieldValue) []OutputFieldValue {
	out := make([]OutputFieldValue, 0, len(fields))
	for index, field := range fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			name = outputDefaultFieldName(objectType, index)
		}
		value := strings.TrimSpace(field.Value)
		if outputTypeUsesFrequency(objectType) && normalizeFieldName(name) == "reporting frequency" {
			value = canonicalOutputFrequency(value)
		}
		out = append(out, OutputFieldValue{Name: name, Value: value})
	}
	return out
}

func outputDefaultFieldName(objectType string, fieldIndex int) string {
	if spec, ok := fieldSpecAt(objectType, fieldIndex); ok && spec.Name != "" {
		return spec.Name
	}
	return fmt.Sprintf("Field %d", fieldIndex+1)
}

func outputFieldsForObject(objectType string, values []OutputFieldValue) []Field {
	fields := make([]Field, 0, len(values))
	for index, field := range values {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			name = outputDefaultFieldName(objectType, index)
		}
		fields = append(fields, Field{Value: strings.TrimSpace(field.Value), Comment: name})
	}
	return fields
}

func validateOutputFields(objectType string, fields []OutputFieldValue) []Diagnostic {
	var warnings []Diagnostic
	for index, field := range fields {
		value := strings.TrimSpace(field.Value)
		name := strings.TrimSpace(field.Name)
		if name == "" {
			name = outputDefaultFieldName(objectType, index)
		}
		choices := outputFieldChoices(objectType, index)
		if len(choices) > 0 && value != "" && !stringInSet(value, choices) {
			warnings = append(warnings, outputApplyDiagnostic(DiagnosticError, "invalid_output_choice", fmt.Sprintf("%s expects one of: %s.", name, strings.Join(choices, ", "))))
		}
		if outputTypeUsesFrequency(objectType) && normalizeFieldName(name) == "reporting frequency" {
			if value == "" {
				continue
			}
			if !validOutputFrequency(value) {
				warnings = append(warnings, outputApplyDiagnostic(DiagnosticError, "invalid_output_frequency", fmt.Sprintf("%q is not a valid reporting frequency.", value)))
			}
			if strings.EqualFold(value, "Detailed") || strings.EqualFold(value, "Timestep") {
				warnings = append(warnings, outputApplyDiagnostic(DiagnosticWarning, "high_volume_output", fmt.Sprintf("%s frequency can create large output files.", canonicalOutputFrequency(value))))
			}
		}
	}
	return warnings
}

func outputObjectLabel(objectType string, fields []OutputFieldValue) string {
	key := outputFieldValue(fields, "Key Value", "Key Name")
	variable := outputFieldValue(fields, "Variable Name")
	frequency := outputFieldValue(fields, "Reporting Frequency")
	switch strings.ToLower(strings.TrimSpace(objectType)) {
	case "output:variable":
		return fmt.Sprintf("%s %s / %s / %s", objectType, blankAsWildcard(key), variable, canonicalOutputFrequency(frequency))
	case "output:meter", "output:meter:meterfileonly":
		return fmt.Sprintf("%s %s / %s", objectType, key, canonicalOutputFrequency(frequency))
	default:
		values := make([]string, 0, len(fields))
		for _, field := range fields {
			if strings.TrimSpace(field.Value) != "" {
				values = append(values, strings.TrimSpace(field.Value))
			}
		}
		if len(values) == 0 {
			return objectType
		}
		return fmt.Sprintf("%s %s", objectType, strings.Join(values, ", "))
	}
}

func outputApplyDiagnostic(severity string, code string, message string) Diagnostic {
	return Diagnostic{
		Severity: severity,
		Category: "Output Apply",
		Code:     code,
		Message:  message,
	}
}
