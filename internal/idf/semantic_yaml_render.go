package idf

import "strings"

func RenderSemanticYAML(model SemanticModel) string {
	lines := make([]string, 0, len(model.Nodes))
	for _, node := range model.Nodes {
		lines = append(lines, semanticNodeText(node))
	}
	return strings.Join(lines, "\n") + "\n"
}

func BuildSemanticLines(model SemanticModel) []SemanticYAMLLine {
	lines := make([]SemanticYAMLLine, 0, len(model.Nodes))
	for _, node := range model.Nodes {
		display := node.DisplayValue
		line := SemanticYAMLLine{
			Text:              semanticNodeText(node),
			Indent:            node.Indent,
			Key:               node.Key,
			Value:             display,
			DisplayValue:      display,
			PatchValue:        node.PatchValue,
			SourceValue:       node.SourceValue,
			ObjectIndex:       node.ObjectIndex,
			ObjectType:        node.ObjectType,
			ObjectName:        node.ObjectName,
			FieldIndex:        node.FieldIndex,
			SourceKind:        node.SourceKind,
			EditKind:          node.EditKind,
			Editable:          node.Editable,
			Role:              node.Role,
			EntityID:          node.EntityID,
			EntityKind:        node.EntityKind,
			OccurrenceID:      node.OccurrenceID,
			SemanticPath:      node.SemanticPath,
			SourceAnchor:      cloneSemanticSourceAnchor(node.SourceAnchor),
			ViewTargets:       append([]SemanticViewTarget(nil), node.ViewTargets...),
			PreferredView:     node.PreferredView,
			PreferredTargetID: node.PreferredTargetID,
		}
		lines = append(lines, line)
	}
	return lines
}

func semanticNodeText(node SemanticYAMLNode) string {
	if strings.TrimSpace(node.Raw) != "" {
		return semanticLineText(node.Indent, node.Raw)
	}
	key := strings.TrimSpace(node.Key)
	if key == "" {
		key = "value"
	}
	if node.ListItem {
		key = "- " + key
	}
	return semanticLineText(node.Indent, key+": "+yamlScalar(node.DisplayValue))
}

func (builder *semanticYAMLBuilder) addNode(node SemanticYAMLNode) {
	builder.model.Nodes = append(builder.model.Nodes, node)
}

func (builder *semanticYAMLBuilder) raw(indent int, raw string) {
	builder.addNode(SemanticYAMLNode{Indent: indent, Raw: raw, Role: "syntax", SourceKind: "summary", EditKind: "readonly"})
}

func (builder *semanticYAMLBuilder) kv(indent int, key string, value string) {
	builder.addNode(SemanticYAMLNode{
		Indent:       indent,
		Key:          key,
		DisplayValue: value,
		SourceKind:   "summary",
		EditKind:     "readonly",
		Role:         "metadata",
	})
}

func (builder *semanticYAMLBuilder) rawForObject(indent int, raw string, objectIndex int, objectType string, objectName string) {
	builder.addNode(SemanticYAMLNode{
		Indent:      indent,
		Raw:         raw,
		ObjectIndex: intPtr(objectIndex),
		ObjectType:  objectType,
		ObjectName:  objectName,
		SourceKind:  "derived",
		EditKind:    "readonly",
		Role:        "object",
	})
}

func (builder *semanticYAMLBuilder) kvForObject(indent int, key string, value string, objectIndex int, objectType string, objectName string) {
	builder.objectValue(indent, key, value, objectIndex, objectType, objectName, "derived", "readonly")
}

func (builder *semanticYAMLBuilder) objectKV(indent int, key string, value string, objectIndex int, objectType string, objectName string) {
	builder.objectValue(indent, key, value, objectIndex, objectType, objectName, "derived", "readonly")
}

func (builder *semanticYAMLBuilder) fieldKV(indent int, key string, value string, objectIndex int, objectType string, objectName string, fieldIndex int) {
	builder.fieldValue(indent, key, value, value, objectIndex, objectType, objectName, fieldIndex, true, "raw_field")
}

func (builder *semanticYAMLBuilder) fieldDisplayKV(indent int, key string, displayValue string, sourceValue string, objectIndex int, objectType string, objectName string, fieldIndex int) {
	builder.fieldValue(indent, key, displayValue, sourceValue, objectIndex, objectType, objectName, fieldIndex, false, "readonly")
}

func (builder *semanticYAMLBuilder) objectValue(indent int, key string, value string, objectIndex int, objectType string, objectName string, sourceKind string, editKind string) {
	listItem := strings.TrimSpace(key)
	builder.addNode(SemanticYAMLNode{
		Indent:       indent,
		Key:          strings.TrimPrefix(key, "- "),
		ListItem:     strings.HasPrefix(listItem, "- "),
		DisplayValue: value,
		ObjectIndex:  intPtr(objectIndex),
		ObjectType:   objectType,
		ObjectName:   objectName,
		SourceKind:   sourceKind,
		EditKind:     editKind,
		Role:         "object",
	})
}

func (builder *semanticYAMLBuilder) fieldValue(indent int, key string, displayValue string, sourceValue string, objectIndex int, objectType string, objectName string, fieldIndex int, editable bool, editKind string) {
	listItem := strings.TrimSpace(key)
	if builder.ctx != nil && objectIndex >= 0 && fieldIndex >= 0 {
		if builder.ctx.shownFields[objectIndex] == nil {
			builder.ctx.shownFields[objectIndex] = map[int]bool{}
		}
		builder.ctx.shownFields[objectIndex][fieldIndex] = true
	}
	builder.addNode(SemanticYAMLNode{
		Indent:       indent,
		Key:          strings.TrimPrefix(key, "- "),
		ListItem:     strings.HasPrefix(listItem, "- "),
		DisplayValue: displayValue,
		PatchValue:   sourceValue,
		SourceValue:  sourceValue,
		ObjectIndex:  intPtr(objectIndex),
		ObjectType:   objectType,
		ObjectName:   objectName,
		FieldIndex:   intPtr(fieldIndex),
		SourceKind:   "field",
		EditKind:     editKind,
		Editable:     editable,
		Role:         "field",
	})
}

func semanticNodePathLabel(node SemanticYAMLNode) string {
	raw := strings.TrimSpace(node.Raw)
	if raw == "semantic_energyplus_model:" {
		return "semantic_energyplus_model"
	}
	if strings.HasPrefix(raw, "- name:") {
		return strings.Trim(strings.TrimSpace(strings.TrimPrefix(raw, "- name:")), `"`)
	}
	if strings.HasPrefix(raw, "- ") {
		return strings.Trim(strings.TrimSpace(strings.TrimPrefix(raw, "- ")), `"`)
	}
	if strings.HasSuffix(raw, ":") {
		return strings.TrimSuffix(raw, ":")
	}
	if strings.HasSuffix(raw, ": {}") || strings.HasSuffix(raw, ": []") {
		return strings.TrimSpace(strings.Split(raw, ":")[0])
	}
	if node.Key == "name" {
		return node.DisplayValue
	}
	return ""
}

func semanticLineText(indent int, raw string) string {
	return strings.Repeat("  ", indent) + raw
}
