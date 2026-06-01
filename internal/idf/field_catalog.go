package idf

import (
	"fmt"
	"sort"
	"strings"
)

type FieldSpec struct {
	Index         int
	Name          string
	Role          string
	Choices       []string
	Numeric       bool
	AllowAutosize bool
}

type ObjectSpec struct {
	Type   string
	Fields []FieldSpec
}

type FieldValueSuggestion struct {
	Value  string `json:"value"`
	Label  string `json:"label,omitempty"`
	Source string `json:"source,omitempty"`
}

const (
	fieldRoleName          = "name"
	fieldRoleObjectRef     = "object_ref"
	fieldRoleObjectTypeRef = "object_type_ref"
	fieldRoleNodeRef       = "node_ref"
	fieldRoleNodeListRef   = "node_list_ref"
	fieldRoleBranchRef     = "branch_ref"
	fieldRoleBranchListRef = "branch_list_ref"
	fieldRoleConnectorRef  = "connector_ref"
	fieldRoleZoneRef       = "zone_ref"
)

var objectFieldCatalog = map[string]ObjectSpec{
	"airloophvac": catalogObject("AirLoopHVAC",
		field("Name", fieldRoleName),
		field("Controller List Name", fieldRoleObjectRef),
		field("Availability Manager List Name", fieldRoleObjectRef),
		numericField("Design Supply Air Flow Rate", true),
		field("Branch List Name", fieldRoleBranchListRef),
		field("Connector List Name", fieldRoleConnectorRef),
		field("Supply Side Inlet Node Name", fieldRoleNodeRef),
		field("Demand Side Outlet Node Name", fieldRoleNodeRef),
		field("Demand Side Inlet Node Names", fieldRoleNodeRef),
		field("Supply Side Outlet Node Names", fieldRoleNodeRef),
	),
	"plantloop": catalogObject("PlantLoop",
		field("Name", fieldRoleName),
		choiceField("Fluid Type", "", "Water", "Steam", "UserDefined", "Glycol"),
		field("User Defined Fluid Type", ""),
		field("Plant Equipment Operation Scheme Name", fieldRoleObjectRef),
		field("Loop Temperature Setpoint Node Name", fieldRoleNodeRef),
		numericField("Maximum Loop Temperature", false),
		numericField("Minimum Loop Temperature", false),
		numericField("Maximum Loop Flow Rate", true),
		numericField("Minimum Loop Flow Rate", false),
		numericField("Plant Loop Volume", true),
		field("Plant Side Inlet Node Name", fieldRoleNodeRef),
		field("Plant Side Outlet Node Name", fieldRoleNodeRef),
		field("Plant Side Branch List Name", fieldRoleBranchListRef),
		field("Plant Side Connector List Name", fieldRoleConnectorRef),
		field("Demand Side Inlet Node Name", fieldRoleNodeRef),
		field("Demand Side Outlet Node Name", fieldRoleNodeRef),
		field("Demand Side Branch List Name", fieldRoleBranchListRef),
		field("Demand Side Connector List Name", fieldRoleConnectorRef),
	),
	"branchlist": catalogObject("BranchList",
		field("Name", fieldRoleName),
	),
	"branch": catalogObject("Branch",
		field("Name", fieldRoleName),
		field("Pressure Drop Curve Name", fieldRoleObjectRef),
	),
	"connectorlist": catalogObject("ConnectorList",
		field("Name", fieldRoleName),
	),
	"connector:splitter": catalogObject("Connector:Splitter",
		field("Name", fieldRoleName),
		field("Inlet Branch Name", fieldRoleBranchRef),
	),
	"connector:mixer": catalogObject("Connector:Mixer",
		field("Name", fieldRoleName),
		field("Outlet Branch Name", fieldRoleBranchRef),
	),
	"airloophvac:supplypath": catalogObject("AirLoopHVAC:SupplyPath",
		field("Name", fieldRoleName),
		field("Supply Air Path Inlet Node Name", fieldRoleNodeRef),
	),
	"airloophvac:returnpath": catalogObject("AirLoopHVAC:ReturnPath",
		field("Name", fieldRoleName),
		field("Return Air Path Outlet Node Name", fieldRoleNodeRef),
	),
	"airloophvac:zonesplitter": catalogObject("AirLoopHVAC:ZoneSplitter",
		field("Name", fieldRoleName),
		field("Inlet Node Name", fieldRoleNodeRef),
	),
	"airloophvac:zonemixer": catalogObject("AirLoopHVAC:ZoneMixer",
		field("Name", fieldRoleName),
		field("Outlet Node Name", fieldRoleNodeRef),
	),
	"zonehvac:equipmentconnections": catalogObject("ZoneHVAC:EquipmentConnections",
		field("Zone Name", fieldRoleZoneRef),
		field("Zone Conditioning Equipment List Name", fieldRoleObjectRef),
		field("Zone Air Inlet Node or NodeList Name", fieldRoleNodeListRef),
		field("Zone Air Exhaust Node or NodeList Name", fieldRoleNodeListRef),
		field("Zone Air Node Name", fieldRoleNodeRef),
		field("Zone Return Air Node or NodeList Name", fieldRoleNodeListRef),
	),
	"zonehvac:equipmentlist": catalogObject("ZoneHVAC:EquipmentList",
		field("Name", fieldRoleName),
	),
	"nodelist": catalogObject("NodeList",
		field("Name", fieldRoleName),
	),
	"output:variable": catalogObject("Output:Variable",
		field("Key Value", ""),
		field("Variable Name", ""),
		choiceField("Reporting Frequency", "", "Detailed", "Timestep", "Hourly", "Daily", "Monthly", "RunPeriod", "Annual"),
		field("Schedule Name", fieldRoleObjectRef),
	),
	"output:meter": catalogObject("Output:Meter",
		field("Key Name", ""),
		choiceField("Reporting Frequency", "", "Detailed", "Timestep", "Hourly", "Daily", "Monthly", "RunPeriod", "Annual"),
	),
	"output:meter:meterfileonly": catalogObject("Output:Meter:MeterFileOnly",
		field("Key Name", ""),
		choiceField("Reporting Frequency", "", "Detailed", "Timestep", "Hourly", "Daily", "Monthly", "RunPeriod", "Annual"),
	),
	"output:variabledictionary": catalogObject("Output:VariableDictionary",
		choiceField("Key Field", "", "IDF", "regular", "Regular", "Name", "None"),
	),
	"output:table:summaryreports": catalogObject("Output:Table:SummaryReports",
		field("Report 1 Name", ""),
	),
	"output:sqlite": catalogObject("Output:SQLite",
		choiceField("Option Type", "", "Simple", "SimpleAndTabular"),
		choiceField("Unit Conversion for Tabular Data", "", "None", "JtoKWH", "JtoMJ", "JtoGJ", "InchPound"),
	),
	"outputcontrol:table:style": catalogObject("OutputControl:Table:Style",
		choiceField("Column Separator", "", "Tab", "Fixed", "Comma", "HTML", "XML", "All"),
		choiceField("Unit Conversion", "", "None", "JtoKWH", "JtoMJ", "JtoGJ", "InchPound"),
	),
	"output:diagnostics": catalogObject("Output:Diagnostics",
		field("Key 1", ""),
	),
}

func catalogObject(objectType string, fields ...FieldSpec) ObjectSpec {
	for index := range fields {
		fields[index].Index = index
	}
	return ObjectSpec{Type: objectType, Fields: fields}
}

func field(name string, role string) FieldSpec {
	return FieldSpec{Name: name, Role: role}
}

func choiceField(name string, role string, choices ...string) FieldSpec {
	return FieldSpec{Name: name, Role: role, Choices: choices}
}

func numericField(name string, allowAutosize bool) FieldSpec {
	return FieldSpec{Name: name, Numeric: true, AllowAutosize: allowAutosize}
}

func fieldSpecAt(objectType string, fieldIndex int) (FieldSpec, bool) {
	spec, ok := objectFieldCatalog[normalizeFieldCatalogKey(objectType)]
	if !ok || fieldIndex < 0 || fieldIndex >= len(spec.Fields) {
		return FieldSpec{}, false
	}
	return spec.Fields[fieldIndex], true
}

func fieldByCatalogName(obj Object, names ...string) (Field, int, bool) {
	wanted := map[string]bool{}
	for _, name := range names {
		key := normalizeFieldName(name)
		if key != "" {
			wanted[key] = true
		}
	}
	if len(wanted) == 0 {
		return Field{}, -1, false
	}
	for index, field := range obj.Fields {
		if spec, ok := fieldSpecAt(obj.Type, index); ok && wanted[normalizeFieldName(spec.Name)] {
			return field, index, true
		}
		if wanted[normalizeFieldName(field.Comment)] {
			return field, index, true
		}
	}
	return Field{}, -1, false
}

func fieldValueByCatalogName(obj Object, names ...string) string {
	field, _, ok := fieldByCatalogName(obj, names...)
	if !ok {
		return ""
	}
	return strings.TrimSpace(field.Value)
}

func catalogFieldName(obj Object, fieldIndex int) string {
	if spec, ok := fieldSpecAt(obj.Type, fieldIndex); ok && spec.Name != "" {
		return spec.Name
	}
	if fieldIndex >= 0 && fieldIndex < len(obj.Fields) {
		return strings.TrimSpace(obj.Fields[fieldIndex].Comment)
	}
	return ""
}

func catalogFieldRole(obj Object, fieldIndex int) string {
	if spec, ok := fieldSpecAt(obj.Type, fieldIndex); ok {
		return spec.Role
	}
	return ""
}

func SuggestFieldValues(doc Document, objectIndex int, fieldIndex int) []FieldValueSuggestion {
	if objectIndex < 0 || objectIndex >= len(doc.Objects) {
		return nil
	}
	obj := doc.Objects[objectIndex]
	if fieldIndex < 0 || fieldIndex >= len(obj.Fields) {
		return nil
	}
	spec, _ := fieldSpecAt(obj.Type, fieldIndex)
	suggestions := suggestionsForFieldSpec(doc, spec)
	if len(suggestions) == 0 {
		suggestions = suggestionsForComment(doc, obj.Fields[fieldIndex].Comment)
	}
	return uniqueFieldSuggestions(suggestions)
}

func fieldCatalogDiagnostics(doc Document) []Diagnostic {
	var diagnostics []Diagnostic
	for _, obj := range doc.Objects {
		for fieldIndex, field := range obj.Fields {
			value := strings.TrimSpace(field.Value)
			if value == "" {
				continue
			}
			spec, hasSpec := fieldSpecAt(obj.Type, fieldIndex)
			if !hasSpec {
				spec = FieldSpec{
					Name: catalogFieldName(obj, fieldIndex),
					Role: roleFromFieldComment(field.Comment),
				}
			}
			diagnostics = append(diagnostics, validateCatalogField(doc, obj, fieldIndex, field, spec)...)
		}
	}
	return diagnostics
}

func validateCatalogField(doc Document, obj Object, fieldIndex int, field Field, spec FieldSpec) []Diagnostic {
	value := strings.TrimSpace(field.Value)
	if value == "" {
		return nil
	}
	fieldName := catalogFieldName(obj, fieldIndex)
	if fieldName == "" {
		fieldName = spec.Name
	}
	if len(spec.Choices) > 0 && !stringInSet(value, spec.Choices) {
		return []Diagnostic{{
			Severity:    DiagnosticWarning,
			Category:    "Field Value",
			Code:        "invalid_choice",
			Message:     fmt.Sprintf("%s expects one of: %s.", fieldName, strings.Join(spec.Choices, ", ")),
			ObjectIndex: obj.Index,
			ObjectType:  obj.Type,
			ObjectName:  objectName(obj),
			FieldIndex:  fieldIndex,
			Field:       fieldName,
			Value:       value,
		}}
	}
	if spec.Numeric && !(spec.AllowAutosize && isFlexibleSizingValue(value)) {
		if _, ok := parseFloatField(value); !ok {
			return []Diagnostic{{
				Severity:    DiagnosticError,
				Category:    "Field Value",
				Code:        "invalid_number",
				Message:     fmt.Sprintf("%s expects a numeric value.", fieldName),
				ObjectIndex: obj.Index,
				ObjectType:  obj.Type,
				ObjectName:  objectName(obj),
				FieldIndex:  fieldIndex,
				Field:       fieldName,
				Value:       value,
			}}
		}
	}
	if spec.Role == "" {
		return nil
	}
	if referenceRoleAllowsImplicitValue(spec.Role) {
		return nil
	}
	if !referenceExistsForRole(doc, spec.Role, value) {
		return []Diagnostic{{
			Severity:    DiagnosticError,
			Category:    "Reference",
			Code:        "missing_catalog_reference",
			Message:     fmt.Sprintf("%s references missing value %q.", fieldName, value),
			ObjectIndex: obj.Index,
			ObjectType:  obj.Type,
			ObjectName:  objectName(obj),
			FieldIndex:  fieldIndex,
			Field:       fieldName,
			Value:       value,
		}}
	}
	return nil
}

func suggestionsForFieldSpec(doc Document, spec FieldSpec) []FieldValueSuggestion {
	var suggestions []FieldValueSuggestion
	for _, choice := range spec.Choices {
		suggestions = append(suggestions, FieldValueSuggestion{Value: choice, Source: "catalog"})
	}
	if spec.Numeric && spec.AllowAutosize {
		suggestions = append(suggestions, FieldValueSuggestion{Value: "Autosize", Source: "catalog"})
	}
	switch spec.Role {
	case fieldRoleZoneRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "Zone", "Space")...)
	case fieldRoleBranchRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "Branch")...)
	case fieldRoleBranchListRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "BranchList")...)
	case fieldRoleConnectorRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "ConnectorList", "Connector:Splitter", "Connector:Mixer")...)
	case fieldRoleNodeListRef:
		suggestions = append(suggestions, objectNameSuggestions(doc, "NodeList")...)
		suggestions = append(suggestions, nodeNameSuggestions(doc)...)
	case fieldRoleNodeRef:
		suggestions = append(suggestions, nodeNameSuggestions(doc)...)
	case fieldRoleObjectTypeRef:
		suggestions = append(suggestions, objectTypeSuggestions(doc)...)
	case fieldRoleObjectRef:
		suggestions = append(suggestions, namedObjectSuggestions(doc)...)
	}
	return suggestions
}

func suggestionsForComment(doc Document, comment string) []FieldValueSuggestion {
	role := roleFromFieldComment(comment)
	if role == "" {
		return nil
	}
	return suggestionsForFieldSpec(doc, FieldSpec{Role: role})
}

func roleFromFieldComment(comment string) string {
	normalized := normalizeFieldName(comment)
	switch {
	case strings.Contains(normalized, "node list"):
		return fieldRoleNodeListRef
	case strings.Contains(normalized, "node") && strings.Contains(normalized, "name"):
		return fieldRoleNodeRef
	case strings.Contains(normalized, "branch list"):
		return fieldRoleBranchListRef
	case strings.Contains(normalized, "branch") && strings.Contains(normalized, "name"):
		return fieldRoleBranchRef
	case strings.Contains(normalized, "connector list"):
		return fieldRoleConnectorRef
	case strings.Contains(normalized, "zone") && strings.Contains(normalized, "name"):
		return fieldRoleZoneRef
	case strings.Contains(normalized, "schedule") && strings.Contains(normalized, "name"):
		return fieldRoleObjectRef
	default:
		return ""
	}
}

func objectNameSuggestions(doc Document, types ...string) []FieldValueSuggestion {
	allowed := map[string]bool{}
	for _, objectType := range types {
		allowed[normalizeFieldCatalogKey(objectType)] = true
	}
	var suggestions []FieldValueSuggestion
	for _, obj := range doc.Objects {
		if len(allowed) > 0 && !allowed[normalizeFieldCatalogKey(obj.Type)] {
			continue
		}
		if name := objectName(obj); name != "" {
			suggestions = append(suggestions, FieldValueSuggestion{Value: name, Label: obj.Type, Source: "document"})
		}
	}
	return suggestions
}

func namedObjectSuggestions(doc Document) []FieldValueSuggestion {
	return objectNameSuggestions(doc)
}

func objectTypeSuggestions(doc Document) []FieldValueSuggestion {
	seen := map[string]bool{}
	var suggestions []FieldValueSuggestion
	for _, obj := range doc.Objects {
		if seen[strings.ToLower(obj.Type)] {
			continue
		}
		seen[strings.ToLower(obj.Type)] = true
		suggestions = append(suggestions, FieldValueSuggestion{Value: obj.Type, Source: "document"})
	}
	return suggestions
}

func nodeNameSuggestions(doc Document) []FieldValueSuggestion {
	seen := map[string]bool{}
	var suggestions []FieldValueSuggestion
	for _, obj := range doc.Objects {
		for index, field := range obj.Fields {
			if !fieldLooksLikeNodeRef(obj, index, field) {
				continue
			}
			value := strings.TrimSpace(field.Value)
			if value == "" || seen[normalizeName(value)] {
				continue
			}
			seen[normalizeName(value)] = true
			suggestions = append(suggestions, FieldValueSuggestion{Value: value, Label: obj.Type, Source: "document"})
		}
	}
	return suggestions
}

func fieldLooksLikeNodeRef(obj Object, fieldIndex int, field Field) bool {
	role := catalogFieldRole(obj, fieldIndex)
	if role == fieldRoleNodeRef || role == fieldRoleNodeListRef {
		return true
	}
	comment := normalizeFieldName(field.Comment)
	return strings.Contains(comment, "node") && strings.Contains(comment, "name")
}

func referenceExistsForRole(doc Document, role string, value string) bool {
	switch role {
	case fieldRoleZoneRef:
		return namedObjectExists(doc, value, "Zone", "Space")
	case fieldRoleBranchRef:
		return namedObjectExists(doc, value, "Branch")
	case fieldRoleBranchListRef:
		return namedObjectExists(doc, value, "BranchList")
	case fieldRoleConnectorRef:
		return namedObjectExists(doc, value, "ConnectorList", "Connector:Splitter", "Connector:Mixer")
	case fieldRoleObjectRef:
		return namedObjectExists(doc, value)
	default:
		return true
	}
}

func namedObjectExists(doc Document, value string, types ...string) bool {
	key := normalizeName(value)
	allowed := map[string]bool{}
	for _, objectType := range types {
		allowed[normalizeFieldCatalogKey(objectType)] = true
	}
	for _, obj := range doc.Objects {
		if len(allowed) > 0 && !allowed[normalizeFieldCatalogKey(obj.Type)] {
			continue
		}
		if normalizeName(objectName(obj)) == key {
			return true
		}
	}
	return false
}

func referenceRoleAllowsImplicitValue(role string) bool {
	return role == fieldRoleNodeRef || role == fieldRoleNodeListRef || role == fieldRoleObjectTypeRef
}

func uniqueFieldSuggestions(values []FieldValueSuggestion) []FieldValueSuggestion {
	seen := map[string]bool{}
	var out []FieldValueSuggestion
	for _, suggestion := range values {
		if strings.TrimSpace(suggestion.Value) == "" {
			continue
		}
		key := normalizeName(suggestion.Value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, suggestion)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return strings.ToLower(out[i].Value) < strings.ToLower(out[j].Value)
	})
	return out
}

func stringInSet(value string, choices []string) bool {
	for _, choice := range choices {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(choice)) {
			return true
		}
	}
	return false
}

func isFlexibleSizingValue(value string) bool {
	return strings.EqualFold(value, "autosize") || strings.EqualFold(value, "autocalculate")
}

func normalizeFieldCatalogKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeFieldName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.Join(strings.Fields(value), " ")
	return value
}
