package idf

import "strings"

type hvacExtensibleFieldGroup []FieldSpec

func hvacExtensibleFieldGroups(obj Object, groupName string) []hvacExtensibleFieldGroup {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return nil
	}
	objectSpec, ok := objectFieldCatalog[normalizeFieldCatalogKey(obj.Type)]
	if !ok {
		return nil
	}
	start := -1
	size := 0
	for _, field := range objectSpec.Fields {
		if field.ExtensibleGroup == groupName && field.ExtensibleGroupSize > 0 {
			if start < 0 || field.Index < start {
				start = field.Index
			}
			size = field.ExtensibleGroupSize
		}
	}
	if start < 0 || size <= 0 {
		return nil
	}
	var groups []hvacExtensibleFieldGroup
	for index := start; index < len(obj.Fields); index += size {
		var group hvacExtensibleFieldGroup
		for offset := 0; offset < size && index+offset < len(obj.Fields); offset++ {
			spec, ok := fieldSpecAt(obj.Type, index+offset)
			if !ok || spec.ExtensibleGroup != groupName {
				continue
			}
			group = append(group, spec)
		}
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

func hvacGroupFieldIndex(group hvacExtensibleFieldGroup, match func(FieldSpec) bool) int {
	for _, spec := range group {
		if match(spec) {
			return spec.Index
		}
	}
	return -1
}

func hvacGroupFieldIndexByName(group hvacExtensibleFieldGroup, names ...string) int {
	normalized := map[string]bool{}
	for _, name := range names {
		normalized[normalizeFieldName(name)] = true
	}
	return hvacGroupFieldIndex(group, func(spec FieldSpec) bool {
		return normalized[normalizeFieldName(spec.Name)]
	})
}

func hvacGroupFieldIndexByRole(group hvacExtensibleFieldGroup, role string) int {
	return hvacGroupFieldIndex(group, func(spec FieldSpec) bool {
		return spec.Role == role
	})
}
