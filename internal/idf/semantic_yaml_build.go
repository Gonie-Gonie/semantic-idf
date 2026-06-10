package idf

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type semanticYAMLBuilder struct {
	model       *SemanticModel
	ctx         *semanticContext
	occurrences map[int][]SemanticOccurrence
}

func buildSemanticProjectionNodes(builder *semanticYAMLBuilder, ctx *semanticContext, metadata SemanticYAMLMetadata) {
	if builder.occurrences == nil {
		builder.occurrences = map[int][]SemanticOccurrence{}
	}
	builder.raw(0, "semantic_energyplus_model:")
	builder.kv(1, "schema", semanticYAMLSchema)
	if strings.TrimSpace(metadata.EnergyPlusVersion) != "" {
		builder.kv(1, "energyplus_version", strings.TrimSpace(metadata.EnergyPlusVersion))
	} else {
		builder.kv(1, "energyplus_version", "unknown")
	}
	builder.raw(1, "compatibility:")
	builder.kv(2, "energyplus_version", blankAs(metadata.EnergyPlusVersion, "unknown"))
	builder.kv(2, "adapter_version", ctx.adapter.AdapterVersion)
	if ctx.adapter.SourcePath != "" {
		builder.kv(2, "schema_source", ctx.adapter.SourcePath)
	} else {
		builder.kv(2, "schema_source", "manual_fallback_catalog")
	}
	builder.kv(1, "yaml_profile", "strict-yaml-1.2-json-compatible")

	builder.raw(1, "project:")
	builder.kv(2, "source_format", blankAs(metadata.SourceFormat, "unknown"))
	builder.kv(2, "object_count", fmt.Sprintf("%d", len(ctx.doc.Objects)))
	builder.kv(2, "semantic_policy", "semantic_view_over_idf_object_registry")

	writeSemanticObjectLibrary(builder, ctx, "simulation", []string{"Version", "SimulationControl", "Timestep"})
	writeSemanticObjectLibrary(builder, ctx, "site", []string{"Site:Location", "SizingPeriod:DesignDay", "SizingPeriod:WeatherFileDays", "SizingPeriod:WeatherFileConditionType", "RunPeriod"})
	writeSemanticObjectLibrary(builder, ctx, "building", []string{"Building", "GlobalGeometryRules"})
	writeSemanticSchedules(builder, ctx)
	writeSemanticMaterials(builder, ctx)
	writeSemanticConstructions(builder, ctx)
	writeSemanticShading(builder, ctx)
	writeSemanticTargetGroups(builder, ctx)
	writeSemanticZones(builder, ctx)
	writeSemanticAirflows(builder, ctx)
	writeSemanticHVAC(builder, ctx)
	writeSemanticOutputs(builder, ctx)
	writeSemanticSourceNameConflicts(builder, builder.model.Source.NameConflicts)
	writeSemanticMiscellaneous(builder, ctx)
	writeSemanticSourcePreservation(builder, ctx)
}

type semanticContext struct {
	doc              Document
	adapter          EnergyPlusSchemaAdapter
	objectByIndex    map[int]Object
	mapped           map[int]bool
	geometry         GeometryReport
	hvac             HVACReport
	output           OutputReport
	surfacesByZone   map[string][]GeometrySurface
	windowsBySurface map[string][]GeometryWindow
	loadsByZone      map[string]map[string][]Object
	loadsBySpace     map[string]map[string][]Object
	controlsByZone   map[string][]Object
	outputsByTarget  map[string][]OutputObjectSummary
	wildcardOutputs  []OutputObjectSummary
	scheduleRefs     map[string][]semanticScheduleUse
	zoneLists        map[string][]string
	spaceLists       map[string][]string
	zoneGroups       []Object
	spacesByZone     map[string][]semanticSpace
	spaceByName      map[string]semanticSpace
	shownFields      map[int]map[int]bool
}

type semanticScheduleUse struct {
	ScheduleName  string
	Path          string
	Object        string
	Field         string
	Role          string
	SourceObject  string
	ExpandedViews []string
}

type semanticSpace struct {
	Name        string
	ZoneName    string
	Source      string
	ObjectIndex int
	FloorArea   string
	Volume      string
	SpaceType   string
	Tags        []string
}

func buildSemanticContext(doc Document, metadata SemanticYAMLMetadata) *semanticContext {
	adapter, err := loadEnergyPlusSchemaAdapter(metadata.EnergyPlusVersion,
		"resources",
		filepath.Join("..", "..", "resources"),
	)
	if err != nil {
		adapter = fieldCatalogAdapter(metadata.EnergyPlusVersion)
	}
	ctx := &semanticContext{
		doc:              doc,
		adapter:          adapter,
		objectByIndex:    map[int]Object{},
		mapped:           map[int]bool{},
		geometry:         AnalyzeGeometry(doc),
		hvac:             AnalyzeHVAC(doc),
		output:           AnalyzeOutput(doc),
		surfacesByZone:   map[string][]GeometrySurface{},
		windowsBySurface: map[string][]GeometryWindow{},
		loadsByZone:      map[string]map[string][]Object{},
		loadsBySpace:     map[string]map[string][]Object{},
		controlsByZone:   map[string][]Object{},
		outputsByTarget:  map[string][]OutputObjectSummary{},
		scheduleRefs:     map[string][]semanticScheduleUse{},
		zoneLists:        map[string][]string{},
		spaceLists:       map[string][]string{},
		spacesByZone:     map[string][]semanticSpace{},
		spaceByName:      map[string]semanticSpace{},
		shownFields:      map[int]map[int]bool{},
	}
	for _, obj := range doc.Objects {
		ctx.objectByIndex[obj.Index] = obj
		if strings.EqualFold(obj.Type, "ZoneList") {
			if name := objectName(obj); name != "" {
				ctx.zoneLists[normalizeName(name)] = semanticListMembers(obj)
				ctx.mark(obj.Index)
			}
			continue
		}
		if strings.EqualFold(obj.Type, "SpaceList") {
			if name := objectName(obj); name != "" {
				ctx.spaceLists[normalizeName(name)] = semanticListMembers(obj)
				ctx.mark(obj.Index)
			}
			continue
		}
		if strings.EqualFold(obj.Type, "ZoneGroup") {
			ctx.zoneGroups = append(ctx.zoneGroups, obj)
			ctx.mark(obj.Index)
			continue
		}
		if strings.EqualFold(obj.Type, "Space") {
			space := semanticSpaceFromObject(obj)
			if space.Name != "" {
				ctx.spacesByZone[normalizeName(space.ZoneName)] = append(ctx.spacesByZone[normalizeName(space.ZoneName)], space)
				ctx.spaceByName[normalizeName(space.Name)] = space
				ctx.mark(obj.Index)
			}
		}
	}
	for _, surface := range ctx.geometry.Surfaces {
		ctx.surfacesByZone[normalizeName(surface.ZoneName)] = append(ctx.surfacesByZone[normalizeName(surface.ZoneName)], surface)
		if surface.SpaceName != "" && ctx.spaceByName[normalizeName(surface.SpaceName)].Name == "" {
			space := semanticSpace{Name: surface.SpaceName, ZoneName: surface.ZoneName, Source: "referenced_by_surface", ObjectIndex: -1}
			ctx.spacesByZone[normalizeName(surface.ZoneName)] = append(ctx.spacesByZone[normalizeName(surface.ZoneName)], space)
			ctx.spaceByName[normalizeName(space.Name)] = space
		}
	}
	for _, window := range ctx.geometry.Windows {
		ctx.windowsBySurface[normalizeName(window.BaseSurfaceName)] = append(ctx.windowsBySurface[normalizeName(window.BaseSurfaceName)], window)
	}
	for _, summary := range ctx.output.Existing {
		key := normalizeName(summary.KeyValue)
		if key == "*" {
			if semanticOutputWildcardFamily(summary) == "zone_wildcard" {
				ctx.wildcardOutputs = append(ctx.wildcardOutputs, summary)
			}
		} else if key != "" {
			ctx.outputsByTarget[key] = append(ctx.outputsByTarget[key], summary)
		}
	}
	for _, obj := range doc.Objects {
		for _, ref := range semanticScheduleReferences(obj) {
			ref.Path = semanticObjectReferencePath(obj)
			ref.Object = semanticObjectLabel(obj)
			ref.SourceObject = fmt.Sprintf("obj-%d", obj.Index)
			ctx.scheduleRefs[normalizeName(ref.ScheduleName)] = append(ctx.scheduleRefs[normalizeName(ref.ScheduleName)], ref)
		}
		targetName, targetKind, bucket, ok := semanticTargetAttachment(ctx, obj)
		if ok {
			if targetKind == "space" || targetKind == "space_list" {
				for _, target := range semanticSpaceTargets(ctx, targetName) {
					key := normalizeName(target)
					if ctx.loadsBySpace[key] == nil {
						ctx.loadsBySpace[key] = map[string][]Object{}
					}
					ctx.loadsBySpace[key][bucket] = append(ctx.loadsBySpace[key][bucket], obj)
				}
				continue
			}
			for _, target := range semanticZoneTargets(ctx, targetName) {
				key := normalizeName(target)
				if ctx.loadsByZone[key] == nil {
					ctx.loadsByZone[key] = map[string][]Object{}
				}
				ctx.loadsByZone[key][bucket] = append(ctx.loadsByZone[key][bucket], obj)
			}
			continue
		}
		if zoneName := semanticControlZone(obj); zoneName != "" {
			ctx.controlsByZone[normalizeName(zoneName)] = append(ctx.controlsByZone[normalizeName(zoneName)], obj)
		}
	}
	for _, zone := range semanticZones(ctx) {
		key := normalizeName(zone.Name)
		if len(ctx.spacesByZone[key]) == 0 {
			ctx.spacesByZone[key] = append(ctx.spacesByZone[key], semanticSpace{
				Name:        zone.Name,
				ZoneName:    zone.Name,
				Source:      "inferred_default",
				ObjectIndex: -1,
			})
		}
	}
	return ctx
}

func (ctx *semanticContext) mark(objectIndex int) {
	if objectIndex >= 0 {
		ctx.mapped[objectIndex] = true
	}
}

func writeSemanticObjectLibrary(builder *semanticYAMLBuilder, ctx *semanticContext, section string, objectTypes []string) {
	objects := semanticObjectsForTypes(ctx.doc, objectTypes)
	if len(objects) == 0 {
		builder.raw(1, section+": {}")
		return
	}
	builder.raw(1, section+":")
	for _, obj := range objects {
		ctx.mark(obj.Index)
		writeSemanticReferenceObject(builder, 2, obj)
	}
}

type semanticCompactScheduleInterval struct {
	Time            string
	TimeFieldIndex  int
	Value           string
	ValueFieldIndex int
}

type semanticCompactScheduleRule struct {
	Through           string
	ThroughFieldIndex int
	DaySelector       string
	DayFieldIndex     int
	Intervals         []semanticCompactScheduleInterval
}

func writeSemanticSchedules(builder *semanticYAMLBuilder, ctx *semanticContext) {
	objects := semanticObjectsForTypes(ctx.doc, semanticObjectTypesWithPrefix(ctx.doc, "Schedule:"))
	typeLimits := semanticObjectsForTypes(ctx.doc, []string{"ScheduleTypeLimits"})
	if len(objects) == 0 && len(typeLimits) == 0 {
		builder.raw(1, "schedules: {}")
		return
	}
	builder.raw(1, "schedules:")
	if len(typeLimits) > 0 {
		builder.raw(2, "type_limits:")
		for _, obj := range typeLimits {
			ctx.mark(obj.Index)
			writeSemanticReferenceObject(builder, 3, obj)
		}
	}
	if len(objects) == 0 {
		return
	}
	builder.raw(2, "definitions:")
	for _, obj := range objects {
		ctx.mark(obj.Index)
		switch {
		case strings.EqualFold(obj.Type, "Schedule:Constant"):
			writeSemanticConstantSchedule(builder, ctx, 3, obj)
		case strings.EqualFold(obj.Type, "Schedule:Compact"):
			if rules, ok := semanticCompactScheduleRules(obj); ok {
				writeSemanticCompactSchedule(builder, ctx, 3, obj, rules)
			} else {
				writeSemanticRawFallbackSchedule(builder, ctx, 3, obj, "compact_schedule_parse_failed")
			}
		default:
			writeSemanticReferenceObject(builder, 3, obj)
		}
	}
}

func writeSemanticScheduleHeader(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, obj Object) string {
	name := objectName(obj)
	if name != "" {
		builder.fieldKV(indent, "- name", name, obj.Index, obj.Type, name, 0)
		builder.kvForObject(indent+1, "class", obj.Type, obj.Index, obj.Type, name)
	} else {
		builder.objectKV(indent, "- class", obj.Type, obj.Index, obj.Type, name)
	}
	builder.kvForObject(indent+1, "source_object_index", fmt.Sprintf("%d", obj.Index), obj.Index, obj.Type, name)
	if value, fieldIndex, ok := semanticScheduleField(obj, 1, "Schedule Type Limits Name", "Schedule Type Limits"); ok {
		builder.fieldKV(indent+1, "type_limits", value, obj.Index, obj.Type, name, fieldIndex)
	}
	if hours, ok := annualScheduleHours(obj); ok {
		builder.kvForObject(indent+1, "active_hours_per_year", semanticNumber(hours), obj.Index, obj.Type, name)
	}
	if len(ctx.scheduleRefs[normalizeName(name)]) > 0 {
		builder.rawForObject(indent+1, "used_by:", obj.Index, obj.Type, name)
		for _, use := range sortedSemanticScheduleUses(ctx.scheduleRefs[normalizeName(name)]) {
			builder.rawForObject(indent+2, "- path: "+yamlScalar(use.Path), obj.Index, obj.Type, name)
			builder.kvForObject(indent+3, "object", use.Object, obj.Index, obj.Type, name)
			builder.kvForObject(indent+3, "field", use.Field, obj.Index, obj.Type, name)
			builder.kvForObject(indent+3, "role", use.Role, obj.Index, obj.Type, name)
			builder.kvForObject(indent+3, "source_object", use.SourceObject, obj.Index, obj.Type, name)
			if len(use.ExpandedViews) > 0 {
				builder.rawForObject(indent+3, "expanded_views:", obj.Index, obj.Type, name)
				for _, path := range use.ExpandedViews {
					builder.rawForObject(indent+4, "- "+yamlScalar(path), obj.Index, obj.Type, name)
				}
			}
		}
	}
	return name
}

func writeSemanticConstantSchedule(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, obj Object) {
	name := writeSemanticScheduleHeader(builder, ctx, indent, obj)
	if value, fieldIndex, ok := semanticScheduleField(obj, 2, "Hourly Value"); ok {
		builder.fieldKV(indent+1, "default", value, obj.Index, obj.Type, name, fieldIndex)
	}
}

func writeSemanticCompactSchedule(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, obj Object, rules []semanticCompactScheduleRule) {
	name := writeSemanticScheduleHeader(builder, ctx, indent, obj)
	builder.kvForObject(indent+1, "parse_status", "parsed", obj.Index, obj.Type, name)
	builder.rawForObject(indent+1, "rules:", obj.Index, obj.Type, name)
	for _, rule := range rules {
		if rule.Through != "" && rule.ThroughFieldIndex >= 0 {
			builder.fieldKV(indent+2, "- through", rule.Through, obj.Index, obj.Type, name, rule.ThroughFieldIndex)
		} else {
			builder.objectKV(indent+2, "- through", "unspecified", obj.Index, obj.Type, name)
		}
		if rule.DaySelector != "" && rule.DayFieldIndex >= 0 {
			builder.fieldKV(indent+3, "for", rule.DaySelector, obj.Index, obj.Type, name, rule.DayFieldIndex)
		}
		if len(rule.Intervals) == 0 {
			continue
		}
		builder.rawForObject(indent+3, "until:", obj.Index, obj.Type, name)
		for _, interval := range rule.Intervals {
			builder.fieldKV(indent+4, "- time", interval.Time, obj.Index, obj.Type, name, interval.TimeFieldIndex)
			builder.fieldKV(indent+5, "value", interval.Value, obj.Index, obj.Type, name, interval.ValueFieldIndex)
		}
	}
}

func writeSemanticRawFallbackSchedule(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, obj Object, reason string) {
	name := writeSemanticScheduleHeader(builder, ctx, indent, obj)
	builder.kvForObject(indent+1, "parse_status", "raw_fallback", obj.Index, obj.Type, name)
	builder.kvForObject(indent+1, "reason", reason, obj.Index, obj.Type, name)
	builder.rawForObject(indent+1, "raw_fields:", obj.Index, obj.Type, name)
	for fieldIndex, field := range obj.Fields {
		if fieldIndex == 0 && name != "" {
			continue
		}
		builder.fieldKV(indent+2, semanticFieldKey(field, fieldIndex), field.Value, obj.Index, obj.Type, name, fieldIndex)
	}
}

func semanticScheduleField(obj Object, fallbackIndex int, names ...string) (string, int, bool) {
	if value, fieldIndex, ok := semanticFieldValue(obj, names...); ok {
		return value, fieldIndex, true
	}
	if fallbackIndex >= 0 && fallbackIndex < len(obj.Fields) {
		value := strings.TrimSpace(obj.Fields[fallbackIndex].Value)
		if value != "" {
			return value, fallbackIndex, true
		}
	}
	return "", -1, false
}

func semanticCompactScheduleRules(obj Object) ([]semanticCompactScheduleRule, bool) {
	if len(obj.Fields) <= 2 {
		return nil, false
	}
	var rules []semanticCompactScheduleRule
	through := ""
	throughFieldIndex := -1
	for fieldIndex := 2; fieldIndex < len(obj.Fields); {
		directive, value, ok := semanticCompactScheduleDirective(obj.Fields[fieldIndex].Value)
		if !ok {
			fieldIndex++
			continue
		}
		switch directive {
		case "through":
			through = value
			throughFieldIndex = fieldIndex
			fieldIndex++
		case "for":
			rule := semanticCompactScheduleRule{
				Through:           through,
				ThroughFieldIndex: throughFieldIndex,
				DaySelector:       value,
				DayFieldIndex:     fieldIndex,
			}
			fieldIndex++
			for fieldIndex < len(obj.Fields) {
				nextDirective, nextValue, nextOK := semanticCompactScheduleDirective(obj.Fields[fieldIndex].Value)
				if nextOK && (nextDirective == "through" || nextDirective == "for") {
					break
				}
				if !nextOK || nextDirective != "until" {
					fieldIndex++
					continue
				}
				if fieldIndex+1 >= len(obj.Fields) {
					return nil, false
				}
				rule.Intervals = append(rule.Intervals, semanticCompactScheduleInterval{
					Time:            nextValue,
					TimeFieldIndex:  fieldIndex,
					Value:           strings.TrimSpace(obj.Fields[fieldIndex+1].Value),
					ValueFieldIndex: fieldIndex + 1,
				})
				fieldIndex += 2
			}
			rules = append(rules, rule)
		default:
			fieldIndex++
		}
	}
	return rules, len(rules) > 0
}

func semanticCompactScheduleDirective(value string) (string, string, bool) {
	parts := strings.SplitN(strings.TrimSpace(value), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	directive := strings.ToLower(strings.TrimSpace(parts[0]))
	switch directive {
	case "through", "for", "until":
		return directive, strings.TrimSpace(parts[1]), true
	default:
		return "", "", false
	}
}

func writeSemanticMaterials(builder *semanticYAMLBuilder, ctx *semanticContext) {
	materials := semanticMaterialObjects(ctx.doc)
	if len(materials) == 0 {
		builder.raw(1, "materials: {}")
		return
	}
	builder.raw(1, "materials:")
	for _, obj := range materials {
		ctx.mark(obj.Index)
		name := objectName(obj)
		builder.fieldKV(2, "- name", name, obj.Index, obj.Type, name, 0)
		builder.kvForObject(3, "class", obj.Type, obj.Index, obj.Type, name)
		builder.kvForObject(3, "material_kind", semanticMaterialKind(obj.Type), obj.Index, obj.Type, name)
		builder.kvForObject(3, "source_object_index", fmt.Sprintf("%d", obj.Index), obj.Index, obj.Type, name)
		for _, fieldName := range []string{"Roughness"} {
			key := semanticFieldKeyFromName(fieldName)
			semanticFieldByNames(builder, 3, key, obj, "", fieldName)
		}
		for _, item := range []struct {
			field string
			unit  string
		}{
			{"Thickness", "m"},
			{"Conductivity", "W/m-K"},
			{"Density", "kg/m3"},
			{"Specific Heat", "J/kg-K"},
			{"Thermal Resistance", "m2-K/W"},
			{"U-Factor", "W/m2-K"},
		} {
			if value, fieldIndex, ok := semanticFieldValue(obj, item.field); ok {
				builder.fieldDisplayKV(3, semanticFieldKeyFromName(item.field), strings.TrimSpace(value+" "+item.unit), value, obj.Index, obj.Type, name, fieldIndex)
			}
		}
		usedBy := semanticMaterialUsedBy(ctx.doc, name)
		if len(usedBy) > 0 {
			builder.rawForObject(3, "used_by:", obj.Index, obj.Type, name)
			for _, item := range usedBy {
				builder.rawForObject(4, "- "+yamlScalar(item), obj.Index, obj.Type, name)
			}
		}
	}
}

func semanticMaterialKind(objectType string) string {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	switch {
	case lower == "material":
		return "mass"
	case strings.Contains(lower, "nomass"):
		return "no_mass"
	case strings.Contains(lower, "airgap"):
		return "air_gap"
	case strings.HasPrefix(lower, "windowmaterial:"):
		return "window_material"
	default:
		return "complex"
	}
}

func writeSemanticConstructions(builder *semanticYAMLBuilder, ctx *semanticContext) {
	performance := semanticPerformanceConstructionObjects(ctx.doc)
	fenestration := semanticFenestrationConstructionObjects(ctx.doc)
	if len(ctx.geometry.Constructions) == 0 && len(fenestration) == 0 && len(performance) == 0 {
		builder.raw(1, "constructions: {}")
		return
	}
	builder.raw(1, "constructions:")
	builder.raw(2, "opaque:")
	if len(ctx.geometry.Constructions) == 0 {
		builder.raw(3, "[]")
	}
	for _, construction := range ctx.geometry.Constructions {
		ctx.mark(construction.ObjectIndex)
		name := construction.Name
		builder.fieldKV(3, "- name", name, construction.ObjectIndex, construction.ObjectType, name, 0)
		builder.kvForObject(4, "class", construction.ObjectType, construction.ObjectIndex, construction.ObjectType, name)
		builder.kvForObject(4, "source_object_index", fmt.Sprintf("%d", construction.ObjectIndex), construction.ObjectIndex, construction.ObjectType, name)
		usedBy := semanticConstructionUsedBy(ctx, name)
		if len(usedBy) > 0 {
			builder.rawForObject(4, "used_by:", construction.ObjectIndex, construction.ObjectType, name)
			for _, item := range usedBy {
				builder.rawForObject(5, "- "+yamlScalar(item), construction.ObjectIndex, construction.ObjectType, name)
			}
		}
		if len(construction.Layers) > 0 {
			builder.rawForObject(4, "layers:", construction.ObjectIndex, construction.ObjectType, name)
			for layerIndex, layer := range construction.Layers {
				if layer.ObjectIndex >= 0 {
					ctx.mark(layer.ObjectIndex)
				}
				layerName := blankAs(layer.Name, "unnamed_layer")
				builder.rawForObject(5, "- name: "+yamlScalar(layerName), construction.ObjectIndex, construction.ObjectType, name)
				builder.kvForObject(6, "order", fmt.Sprintf("%d", layerIndex+1), construction.ObjectIndex, construction.ObjectType, name)
				if layer.ObjectType != "" {
					builder.kvForObject(6, "class", layer.ObjectType, construction.ObjectIndex, construction.ObjectType, name)
					builder.kvForObject(6, "material_kind", semanticMaterialKind(layer.ObjectType), construction.ObjectIndex, construction.ObjectType, name)
				}
				if layer.HasThickness {
					builder.kvForObject(6, "thickness", semanticQuantity(layer.Thickness, "m"), construction.ObjectIndex, construction.ObjectType, name)
				}
				if layer.ThermalResistance > 0 {
					builder.kvForObject(6, "thermal_resistance", semanticQuantity(layer.ThermalResistance, "m2-K/W"), construction.ObjectIndex, construction.ObjectType, name)
				}
				if layer.Conductivity > 0 {
					builder.kvForObject(6, "conductivity", semanticQuantity(layer.Conductivity, "W/m-K"), construction.ObjectIndex, construction.ObjectType, name)
				}
			}
		}
	}
	if len(performance) > 0 {
		builder.raw(2, "performance_based:")
		for _, obj := range performance {
			ctx.mark(obj.Index)
			writeSemanticReferenceObject(builder, 3, obj)
		}
	}
	if len(fenestration) == 0 {
		builder.raw(2, "fenestration: []")
		return
	}
	builder.raw(2, "fenestration:")
	for _, obj := range fenestration {
		ctx.mark(obj.Index)
		writeSemanticReferenceObject(builder, 3, obj)
	}
}

func writeSemanticShading(builder *semanticYAMLBuilder, ctx *semanticContext) {
	site := semanticObjectsWithTypePrefix(ctx.doc, "Shading:Site")
	building := semanticObjectsWithTypePrefix(ctx.doc, "Shading:Building")
	zone := semanticObjectsWithTypePrefix(ctx.doc, "Shading:Zone")
	if len(site) == 0 && len(building) == 0 && len(zone) == 0 {
		builder.raw(1, "shading: {}")
		return
	}
	builder.raw(1, "shading:")
	writeSemanticObjectList(builder, ctx, 2, "site", site)
	writeSemanticObjectList(builder, ctx, 2, "building", building)
	writeSemanticObjectList(builder, ctx, 2, "zone", zone)
}

func writeSemanticObjectList(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, key string, objects []Object) {
	if len(objects) == 0 {
		builder.raw(indent, key+": []")
		return
	}
	builder.raw(indent, key+":")
	for _, obj := range objects {
		ctx.mark(obj.Index)
		writeSemanticReferenceObject(builder, indent+1, obj)
	}
}

func writeSemanticTargetGroups(builder *semanticYAMLBuilder, ctx *semanticContext) {
	writeSemanticListObjects(builder, ctx, "zone_lists", "ZoneList")
	writeSemanticListObjects(builder, ctx, "space_lists", "SpaceList")
	if len(ctx.zoneGroups) == 0 {
		builder.raw(1, "zone_groups: []")
		return
	}
	builder.raw(1, "zone_groups:")
	for _, obj := range ctx.zoneGroups {
		ctx.mark(obj.Index)
		name := objectName(obj)
		builder.fieldKV(2, "- name", name, obj.Index, obj.Type, name, 0)
		builder.kvForObject(3, "class", obj.Type, obj.Index, obj.Type, name)
		if value, fieldIndex, ok := semanticFieldValue(obj, "Zone List Name"); ok {
			builder.fieldKV(3, "zone_list", value, obj.Index, obj.Type, name, fieldIndex)
		}
		if value, fieldIndex, ok := semanticFieldValue(obj, "Zone List Multiplier", "Multiplier"); ok {
			builder.fieldKV(3, "multiplier", value, obj.Index, obj.Type, name, fieldIndex)
		}
	}
}

func writeSemanticListObjects(builder *semanticYAMLBuilder, ctx *semanticContext, section string, objectType string) {
	objects := semanticObjectsForTypes(ctx.doc, []string{objectType})
	if len(objects) == 0 {
		builder.raw(1, section+": []")
		return
	}
	builder.raw(1, section+":")
	for _, obj := range objects {
		ctx.mark(obj.Index)
		name := objectName(obj)
		builder.fieldKV(2, "- name", name, obj.Index, obj.Type, name, 0)
		builder.kvForObject(3, "class", obj.Type, obj.Index, obj.Type, name)
		builder.rawForObject(3, "members:", obj.Index, obj.Type, name)
		for index := 1; index < len(obj.Fields); index++ {
			value := strings.TrimSpace(obj.Fields[index].Value)
			if value == "" {
				continue
			}
			memberKind := "zone"
			if strings.EqualFold(objectType, "SpaceList") {
				memberKind = "space"
			}
			builder.fieldKV(4, "- name", value, obj.Index, obj.Type, name, index)
			builder.kvForObject(5, "kind", memberKind, obj.Index, obj.Type, name)
		}
	}
}

func writeSemanticZones(builder *semanticYAMLBuilder, ctx *semanticContext) {
	zones := semanticZones(ctx)
	if len(zones) == 0 {
		builder.raw(1, "zones: []")
		return
	}
	builder.raw(1, "zones:")
	for _, zone := range zones {
		ctx.mark(zone.ObjectIndex)
		zoneObj := ctx.objectByIndex[zone.ObjectIndex]
		zoneName := zone.Name
		if zoneName == "" {
			zoneName = objectName(zoneObj)
		}
		builder.fieldKV(2, "- name", zoneName, zone.ObjectIndex, "Zone", zoneName, 0)
		builder.kvForObject(3, "class", "Zone", zone.ObjectIndex, "Zone", zoneName)
		builder.rawForObject(3, "source:", zone.ObjectIndex, "Zone", zoneName)
		builder.kvForObject(4, "object_index", fmt.Sprintf("%d", zone.ObjectIndex), zone.ObjectIndex, "Zone", zoneName)
		builder.kvForObject(4, "object_type", "Zone", zone.ObjectIndex, "Zone", zoneName)
		writeSemanticZoneGeometry(builder, ctx, zoneName)
		writeSemanticSpaces(builder, ctx, zoneName)
		writeSemanticZoneLoads(builder, ctx, zoneName)
		writeSemanticZoneControls(builder, ctx, zoneName)
		writeSemanticZoneHVAC(builder, ctx, zoneName)
		writeSemanticAttachedOutputs(builder, 3, "outputs", ctx.outputsByTarget[normalizeName(zoneName)])
		writeSemanticAttachedOutputs(builder, 3, "inherited_outputs", ctx.wildcardOutputs)
	}
}

func writeSemanticZoneGeometry(builder *semanticYAMLBuilder, ctx *semanticContext, zoneName string) {
	surfaces := ctx.surfacesByZone[normalizeName(zoneName)]
	zoneObj, hasZoneObj := semanticZoneObject(ctx, zoneName)
	if len(surfaces) == 0 && !hasZoneObj {
		return
	}
	builder.raw(3, "geometry:")
	if hasZoneObj {
		if value, fieldIndex, ok := semanticFieldValue(zoneObj, "Direction of Relative North"); ok {
			builder.fieldKV(4, "relative_north", value, zoneObj.Index, zoneObj.Type, zoneName, fieldIndex)
		}
		origin := []string{}
		for _, name := range []string{"X Origin", "Y Origin", "Z Origin"} {
			if value, _, ok := semanticFieldValue(zoneObj, name); ok {
				origin = append(origin, value)
			} else {
				origin = append(origin, "0")
			}
		}
		builder.rawForObject(4, "origin: ["+strings.Join(origin, ", ")+"]", zoneObj.Index, zoneObj.Type, zoneName)
		if value, fieldIndex, ok := semanticFieldValue(zoneObj, "Multiplier"); ok {
			builder.fieldKV(4, "multiplier", value, zoneObj.Index, zoneObj.Type, zoneName, fieldIndex)
		}
		if value, fieldIndex, ok := semanticFieldValue(zoneObj, "Floor Area"); ok {
			builder.fieldKV(4, "floor_area", value, zoneObj.Index, zoneObj.Type, zoneName, fieldIndex)
		}
		if value, fieldIndex, ok := semanticFieldValue(zoneObj, "Volume"); ok {
			builder.fieldKV(4, "volume", value, zoneObj.Index, zoneObj.Type, zoneName, fieldIndex)
		}
		if value, fieldIndex, ok := semanticFieldValue(zoneObj, "Part of Total Floor Area"); ok {
			builder.fieldKV(4, "part_of_total_floor_area", value, zoneObj.Index, zoneObj.Type, zoneName, fieldIndex)
		}
	}
	builder.raw(4, "global_rules:")
	builder.kv(5, "coordinate_system", blankAs(ctx.geometry.CoordinateSystem, "relative"))
	builder.kv(5, "vertex_entry_direction", blankAs(ctx.geometry.VertexEntryDirection, "counterclockwise"))
	builder.kv(5, "starting_vertex_position", blankAs(ctx.geometry.StartingVertexPosition, "upperleftcorner"))
	if len(surfaces) == 0 {
		return
	}
	builder.raw(4, "surfaces:")
	for _, surface := range surfaces {
		ctx.mark(surface.ObjectIndex)
		builder.fieldKV(5, "- name", surface.Name, surface.ObjectIndex, surface.Type, surface.Name, 0)
		builder.kvForObject(6, "class", surface.Type, surface.ObjectIndex, surface.Type, surface.Name)
		builder.kvForObject(6, "type", surface.SurfaceType, surface.ObjectIndex, surface.Type, surface.Name)
		if surface.ZoneName != "" {
			semanticFieldByNames(builder, 6, "zone", ctx.objectByIndex[surface.ObjectIndex], surface.ZoneName, "Zone Name")
		}
		if surface.SpaceName != "" {
			semanticFieldByNames(builder, 6, "space", ctx.objectByIndex[surface.ObjectIndex], surface.SpaceName, "Space Name")
		}
		semanticFieldByNames(builder, 6, "construction", ctx.objectByIndex[surface.ObjectIndex], surface.Construction, "Construction Name")
		builder.rawForObject(6, "boundary:", surface.ObjectIndex, surface.Type, surface.Name)
		semanticFieldByNames(builder, 7, "condition", ctx.objectByIndex[surface.ObjectIndex], surface.OutsideBoundary, "Outside Boundary Condition")
		semanticFieldByNames(builder, 7, "object", ctx.objectByIndex[surface.ObjectIndex], "", "Outside Boundary Condition Object")
		writeSemanticSurfaceBoundaryResolution(builder, ctx, 7, surface)
		builder.rawForObject(6, "exposure:", surface.ObjectIndex, surface.Type, surface.Name)
		semanticFieldByNames(builder, 7, "sun", ctx.objectByIndex[surface.ObjectIndex], "", "Sun Exposure")
		semanticFieldByNames(builder, 7, "wind", ctx.objectByIndex[surface.ObjectIndex], "", "Wind Exposure")
		writeSemanticBoundaryExposureValidation(builder, ctx, 7, surface)
		semanticFieldByNames(builder, 6, "view_factor_to_ground", ctx.objectByIndex[surface.ObjectIndex], "", "View Factor to Ground")
		builder.rawForObject(6, "vertices:", surface.ObjectIndex, surface.Type, surface.Name)
		builder.kvForObject(7, "source", blankAs(surface.VerticesSource, "computed_geometry"), surface.ObjectIndex, surface.Type, surface.Name)
		builder.rawForObject(7, "value: "+semanticVertices(surface.Vertices), surface.ObjectIndex, surface.Type, surface.Name)
		builder.rawForObject(6, "computed:", surface.ObjectIndex, surface.Type, surface.Name)
		builder.kvForObject(7, "area", semanticQuantity(surface.Area, "m2"), surface.ObjectIndex, surface.Type, surface.Name)
		if surface.Orientation != "" {
			builder.kvForObject(7, "orientation", surface.Orientation, surface.ObjectIndex, surface.Type, surface.Name)
		}
		builder.kvForObject(7, "azimuth", semanticQuantity(surface.Azimuth, "deg"), surface.ObjectIndex, surface.Type, surface.Name)
		windows := ctx.windowsBySurface[normalizeName(surface.Name)]
		if len(windows) > 0 {
			builder.rawForObject(6, "fenestration:", surface.ObjectIndex, surface.Type, surface.Name)
			for _, window := range windows {
				ctx.mark(window.ObjectIndex)
				builder.fieldKV(7, "- name", window.Name, window.ObjectIndex, window.Type, window.Name, 0)
				builder.kvForObject(8, "class", window.Type, window.ObjectIndex, window.Type, window.Name)
				builder.kvForObject(8, "type", window.SurfaceType, window.ObjectIndex, window.Type, window.Name)
				semanticFieldByNames(builder, 8, "construction", ctx.objectByIndex[window.ObjectIndex], window.Construction, "Construction Name")
				semanticFieldByNames(builder, 8, "base_surface", ctx.objectByIndex[window.ObjectIndex], window.BaseSurfaceName, "Building Surface Name", "Surface Name")
				builder.kvForObject(8, "base_surface_resolution", semanticBaseSurfaceResolution(ctx, window), window.ObjectIndex, window.Type, window.Name)
				builder.rawForObject(8, "raw:", window.ObjectIndex, window.Type, window.Name)
				builder.kvForObject(9, "multiplier", semanticNumber(window.Multiplier), window.ObjectIndex, window.Type, window.Name)
				builder.rawForObject(8, "vertices:", window.ObjectIndex, window.Type, window.Name)
				builder.kvForObject(9, "source", blankAs(window.VerticesSource, "computed_geometry"), window.ObjectIndex, window.Type, window.Name)
				builder.rawForObject(9, "value: "+semanticVertices(window.Vertices), window.ObjectIndex, window.Type, window.Name)
				builder.rawForObject(8, "computed:", window.ObjectIndex, window.Type, window.Name)
				builder.rawForObject(9, "area:", window.ObjectIndex, window.Type, window.Name)
				builder.kvForObject(10, "value", semanticQuantity(window.Area, "m2"), window.ObjectIndex, window.Type, window.Name)
				builder.kvForObject(10, "includes_multiplier", fmt.Sprintf("%t", window.AreaIncludesMultiplier), window.ObjectIndex, window.Type, window.Name)
				if window.Orientation != "" {
					builder.kvForObject(9, "orientation", window.Orientation, window.ObjectIndex, window.Type, window.Name)
				}
				builder.kvForObject(9, "azimuth", semanticQuantity(window.Azimuth, "deg"), window.ObjectIndex, window.Type, window.Name)
				writeSemanticAttachedOutputs(builder, 8, "outputs", ctx.outputsByTarget[normalizeName(window.Name)])
			}
		}
		writeSemanticAttachedOutputs(builder, 6, "outputs", ctx.outputsByTarget[normalizeName(surface.Name)])
	}
}

func writeSemanticSpaces(builder *semanticYAMLBuilder, ctx *semanticContext, zoneName string) {
	spaces := ctx.spacesByZone[normalizeName(zoneName)]
	if len(spaces) == 0 {
		return
	}
	builder.raw(3, "spaces:")
	for _, space := range spaces {
		objectIndex := space.ObjectIndex
		objectType := "Space"
		if objectIndex < 0 {
			objectIndex = semanticZoneObjectIndex(ctx, zoneName)
			objectType = "computed/default_space"
		} else {
			ctx.mark(objectIndex)
		}
		builder.rawForObject(4, "- name: "+yamlScalar(space.Name), objectIndex, objectType, space.Name)
		builder.kvForObject(5, "zone_name", blankAs(space.ZoneName, zoneName), objectIndex, objectType, space.Name)
		builder.kvForObject(5, "source", blankAs(space.Source, "idf_object"), objectIndex, objectType, space.Name)
		if space.FloorArea != "" {
			builder.kvForObject(5, "floor_area", space.FloorArea, objectIndex, objectType, space.Name)
		}
		if space.Volume != "" {
			builder.kvForObject(5, "volume", space.Volume, objectIndex, objectType, space.Name)
		}
		if space.SpaceType != "" {
			builder.kvForObject(5, "space_type", space.SpaceType, objectIndex, objectType, space.Name)
		}
		writeSemanticSpaceLoads(builder, ctx, 5, space.Name)
		writeSemanticSpaceHVAC(builder, ctx, 5, space.Name)
	}
}

func writeSemanticSpaceHVAC(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, spaceName string) {
	var objects []Object
	for _, obj := range ctx.doc.Objects {
		lower := strings.ToLower(strings.TrimSpace(obj.Type))
		if !strings.HasPrefix(lower, "spacehvac:") {
			continue
		}
		value, _, ok := semanticFieldValue(obj, "Space Name")
		if ok && strings.EqualFold(value, spaceName) {
			objects = append(objects, obj)
		}
	}
	if len(objects) == 0 {
		return
	}
	builder.raw(indent, "hvac:")
	for _, obj := range objects {
		ctx.mark(obj.Index)
		writeSemanticReferenceObject(builder, indent+1, obj)
	}
}

func writeSemanticZoneLoads(builder *semanticYAMLBuilder, ctx *semanticContext, zoneName string) {
	buckets := ctx.loadsByZone[normalizeName(zoneName)]
	outdoorAirSpecs := semanticOutdoorAirSpecsForZone(ctx, zoneName)
	if len(buckets) == 0 && len(outdoorAirSpecs) == 0 {
		return
	}
	writeSemanticZoneObjectBuckets(builder, ctx, 3, "loads", []string{"people", "lights", "electric_equipment", "gas_equipment", "hot_water_equipment", "steam_equipment", "other_equipment"}, buckets)
	writeSemanticZoneObjectBuckets(builder, ctx, 3, "thermal_mass", []string{"internal_mass"}, buckets)
	writeSemanticZoneObjectBuckets(builder, ctx, 3, "behavior", []string{"thermal_capacitance", "air_balance"}, buckets)
	writeSemanticAirExchange(builder, ctx, 3, buckets, outdoorAirSpecs)
}

func writeSemanticAirExchange(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, buckets map[string][]Object, outdoorAirSpecs []Object) {
	order := []string{"infiltration", "ventilation", "mixing", "cross_mixing"}
	hasAirExchange := len(outdoorAirSpecs) > 0
	for _, bucket := range order {
		hasAirExchange = hasAirExchange || len(buckets[bucket]) > 0
	}
	if !hasAirExchange {
		return
	}
	builder.raw(indent, "air_exchange:")
	for _, bucket := range order {
		objects := buckets[bucket]
		if len(objects) == 0 {
			continue
		}
		builder.raw(indent+1, bucket+":")
		for _, obj := range objects {
			ctx.mark(obj.Index)
			writeSemanticLoadObject(builder, indent+2, obj)
		}
	}
	if len(outdoorAirSpecs) > 0 {
		builder.raw(indent+1, "outdoor_air_specs:")
		for _, obj := range outdoorAirSpecs {
			ctx.mark(obj.Index)
			writeSemanticReferenceObject(builder, indent+2, obj)
		}
	}
}

func writeSemanticSpaceLoads(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, spaceName string) {
	buckets := ctx.loadsBySpace[normalizeName(spaceName)]
	if len(buckets) == 0 {
		return
	}
	writeSemanticZoneObjectBuckets(builder, ctx, indent, "loads", []string{"people", "lights", "electric_equipment", "gas_equipment", "hot_water_equipment", "steam_equipment", "other_equipment"}, buckets)
	writeSemanticZoneObjectBuckets(builder, ctx, indent, "thermal_mass", []string{"internal_mass"}, buckets)
	writeSemanticAirExchange(builder, ctx, indent, buckets, nil)
}

func writeSemanticZoneObjectBuckets(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, section string, order []string, buckets map[string][]Object) {
	hasAny := false
	for _, bucket := range order {
		if len(buckets[bucket]) > 0 {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return
	}
	builder.raw(indent, section+":")
	for _, bucket := range order {
		objects := buckets[bucket]
		if len(objects) == 0 {
			continue
		}
		builder.raw(indent+1, bucket+":")
		for _, obj := range objects {
			ctx.mark(obj.Index)
			writeSemanticLoadObject(builder, indent+2, obj)
		}
	}
}

func writeSemanticLoadObject(builder *semanticYAMLBuilder, indent int, obj Object) {
	name := objectName(obj)
	builder.fieldKV(indent, "- name", name, obj.Index, obj.Type, name, 0)
	builder.kvForObject(indent+1, "class", obj.Type, obj.Index, obj.Type, name)
	if value, fieldIndex, ok := semanticFieldValue(obj, "Zone or ZoneList or Space or SpaceList Name", "Zone or ZoneList Name", "Space or SpaceList Name", "Zone Name", "Space Name"); ok {
		key := "zone"
		if builder.ctx != nil {
			_, targetKind, _, _ := semanticTargetAttachment(builder.ctx, obj)
			if targetKind == "space" || targetKind == "space_list" {
				key = "space"
			}
		}
		builder.fieldKV(indent+1, key, value, obj.Index, obj.Type, name, fieldIndex)
	}
	if value, fieldIndex, ok := semanticFieldValue(obj, "Number of People Schedule Name", "Schedule Name"); ok {
		builder.fieldKV(indent+1, "schedule", value, obj.Index, obj.Type, name, fieldIndex)
	}
	if value, fieldIndex, ok := semanticFieldValue(obj, "Activity Level Schedule Name"); ok {
		builder.fieldKV(indent+1, "activity_schedule", value, obj.Index, obj.Type, name, fieldIndex)
	}
	if displayValue, sourceValue, fieldIndex, ok := semanticLoadLevel(obj); ok {
		builder.fieldDisplayKV(indent+1, "level", displayValue, sourceValue, obj.Index, obj.Type, name, fieldIndex)
	}
	writeSemanticLoadMethodFields(builder, indent+1, obj)
	if target, _, ok := semanticZoneAttachment(obj); ok {
		if members := builder.ctx.zoneLists[normalizeName(target)]; len(members) > 1 {
			builder.kvForObject(indent+1, "same_source_object_id", fmt.Sprintf("obj-%d", obj.Index), obj.Index, obj.Type, name)
			builder.kvForObject(indent+1, "expanded_from", "ZoneList/"+target, obj.Index, obj.Type, name)
			writeSemanticDuplicatedAs(builder, indent+1, obj.Index, obj.Type, name, "zone_group_expanded_load", semanticZoneGroupOccurrencePaths(members, obj))
		}
	}
	writeSemanticAttachedOutputs(builder, indent+1, "outputs", builder.ctx.outputsByTarget[normalizeName(name)])
}

func writeSemanticLoadMethodFields(builder *semanticYAMLBuilder, indent int, obj Object) {
	method, methodIndex, hasMethod := semanticFieldValue(obj, "Calculation Method", "Design Flow Rate Calculation Method")
	if hasMethod {
		builder.fieldKV(indent, "calculation_method", method, obj.Index, obj.Type, objectName(obj), methodIndex)
	}
	switch strings.ToLower(strings.TrimSpace(obj.Type)) {
	case "people":
		semanticFieldByNames(builder, indent, "number_of_people", obj, "", "Number of People")
		semanticFieldByNames(builder, indent, "people_per_floor_area", obj, "", "People per Zone Floor Area")
		semanticFieldByNames(builder, indent, "floor_area_per_person", obj, "", "Zone Floor Area per Person")
	case "lights":
		semanticFieldByNames(builder, indent, "lighting_level", obj, "", "Lighting Level")
		semanticFieldByNames(builder, indent, "watts_per_zone_floor_area", obj, "", "Watts per Zone Floor Area")
		semanticFieldByNames(builder, indent, "watts_per_person", obj, "", "Watts per Person")
	case "electricequipment", "gasequipment", "hotwaterequipment", "steamequipment", "otherequipment":
		semanticFieldByNames(builder, indent, "design_level", obj, "", "Design Level")
		semanticFieldByNames(builder, indent, "power_density", obj, "", "Watts per Zone Floor Area", "Power per Zone Floor Area")
		semanticFieldByNames(builder, indent, "area_per_power", obj, "", "Zone Floor Area per Unit of Power")
	default:
		lower := strings.ToLower(strings.TrimSpace(obj.Type))
		if strings.HasPrefix(lower, "zoneinfiltration:") || strings.HasPrefix(lower, "zoneventilation:") || strings.HasPrefix(lower, "zonemixing") || strings.HasPrefix(lower, "zonecrossmixing") {
			semanticFieldByNames(builder, indent, "design_flow_rate", obj, "", "Design Flow Rate")
			semanticFieldByNames(builder, indent, "flow_rate_per_zone_floor_area", obj, "", "Flow Rate per Zone Floor Area")
			semanticFieldByNames(builder, indent, "flow_rate_per_person", obj, "", "Flow Rate per Person")
			semanticFieldByNames(builder, indent, "air_changes_per_hour", obj, "", "Air Changes per Hour")
			semanticFieldByNames(builder, indent, "effective_leakage_area", obj, "", "Effective Air Leakage Area")
			semanticFieldByNames(builder, indent, "flow_coefficient", obj, "", "Flow Coefficient")
		}
	}
}

func writeSemanticZoneControls(builder *semanticYAMLBuilder, ctx *semanticContext, zoneName string) {
	controls := ctx.controlsByZone[normalizeName(zoneName)]
	if len(controls) == 0 {
		return
	}
	builder.raw(3, "controls:")
	buckets := map[string][]Object{}
	order := []string{"thermostat", "daylighting", "humidistat", "other"}
	for _, obj := range controls {
		bucket := semanticControlBucket(obj.Type)
		buckets[bucket] = append(buckets[bucket], obj)
	}
	for _, bucket := range order {
		objects := buckets[bucket]
		if len(objects) == 0 {
			continue
		}
		builder.raw(4, bucket+":")
		for _, obj := range objects {
			ctx.mark(obj.Index)
			name := objectName(obj)
			builder.fieldKV(5, "- name", name, obj.Index, obj.Type, name, 0)
			builder.kvForObject(6, "class", obj.Type, obj.Index, obj.Type, name)
			if value, fieldIndex, ok := semanticFieldValue(obj, "Zone Name"); ok {
				builder.fieldKV(6, "zone", value, obj.Index, obj.Type, name, fieldIndex)
			}
		}
	}
}

func writeSemanticZoneHVAC(builder *semanticYAMLBuilder, ctx *semanticContext, zoneName string) {
	var relation HVACZoneChain
	found := false
	for _, candidate := range ctx.hvac.ZoneRelations {
		if strings.EqualFold(candidate.ZoneName, zoneName) {
			relation = candidate
			found = true
			break
		}
	}
	if !found || (len(relation.TerminalUnits) == 0 && len(relation.ZoneEquipment) == 0 && len(relation.AirLoopNames) == 0 && len(relation.PlantLoopNames) == 0) {
		return
	}
	builder.raw(3, "hvac:")
	if relation.RelationSource != "" {
		builder.kv(4, "relation_source", relation.RelationSource)
	}
	if relation.Confidence != "" {
		builder.kv(4, "confidence", relation.Confidence)
	}
	writeSemanticStringList(builder, 4, "evidence", relation.Evidence)
	if relation.Nodes.AirNode != "" || len(relation.Nodes.InletNodes) > 0 || len(relation.Nodes.ExhaustNodes) > 0 || len(relation.Nodes.ReturnNodes) > 0 {
		builder.raw(4, "nodes:")
		if relation.Nodes.AirNode != "" {
			builder.kv(5, "air_node", relation.Nodes.AirNode)
		}
		writeSemanticStringList(builder, 5, "inlet_nodes", relation.Nodes.InletNodes)
		writeSemanticStringList(builder, 5, "exhaust_nodes", relation.Nodes.ExhaustNodes)
		writeSemanticStringList(builder, 5, "return_nodes", relation.Nodes.ReturnNodes)
		if len(relation.Nodes.Sources) > 0 {
			builder.raw(5, "sources:")
			for _, source := range relation.Nodes.Sources {
				builder.raw(6, "- role: "+yamlScalar(source.Role))
				if source.InputValue != "" {
					builder.kv(7, "input", source.InputValue)
				}
				builder.kv(7, "source_type", source.SourceType)
				writeSemanticStringList(builder, 7, "nodes", source.Nodes)
			}
		}
	}
	if len(relation.AirLoopNames) > 0 {
		builder.raw(4, "air_loops:")
		for _, name := range relation.AirLoopNames {
			builder.raw(5, "- "+yamlScalar(name))
		}
	}
	if len(relation.AirLoopRelations) > 0 {
		builder.raw(4, "air_loop_relations:")
		for _, loopRelation := range relation.AirLoopRelations {
			builder.raw(5, "- loop: "+yamlScalar(loopRelation.LoopName))
			builder.kv(6, "source", loopRelation.Source)
			builder.kv(6, "confidence", loopRelation.Confidence)
			writeSemanticStringList(builder, 6, "evidence", loopRelation.Evidence)
		}
	}
	if len(relation.PlantLoopRelations) > 0 {
		builder.raw(4, "plant_loop_relations:")
		for _, loopRelation := range relation.PlantLoopRelations {
			builder.raw(5, "- loop: "+yamlScalar(loopRelation.LoopName))
			builder.kv(6, "source", loopRelation.Source)
			builder.kv(6, "confidence", loopRelation.Confidence)
			writeSemanticStringList(builder, 6, "evidence", loopRelation.Evidence)
		}
	}
	writeSemanticStringList(builder, 4, "condenser_loops", relation.CondenserLoopNames)
	if len(relation.TerminalUnits) > 0 {
		builder.raw(4, "terminals:")
		for _, component := range relation.TerminalUnits {
			ctx.mark(component.ObjectIndex)
			writeSemanticHVACComponent(builder, 5, component, "zone_terminal")
		}
	}
	if len(relation.ZoneEquipment) > 0 {
		builder.raw(4, "equipment:")
		for _, component := range relation.ZoneEquipment {
			ctx.mark(component.ObjectIndex)
			writeSemanticHVACComponent(builder, 5, component, "zone_equipment")
		}
	}
	if len(relation.ServiceChains) > 0 {
		builder.raw(4, "service_chains:")
		for _, chain := range relation.ServiceChains {
			builder.raw(5, "- zone: "+yamlScalar(chain.ZoneName))
			if chain.TerminalName != "" {
				builder.kv(6, "terminal", chain.TerminalName)
			}
			if chain.AirLoopName != "" {
				builder.kv(6, "air_loop", chain.AirLoopName)
			}
			if chain.PlantLoop != "" {
				builder.kv(6, "plant_loop", chain.PlantLoop)
			}
			if chain.SourceComponent != "" {
				builder.kv(6, "source_component", chain.SourceComponent)
			}
			if chain.Component != "" {
				builder.kv(6, "component", chain.Component)
			}
			builder.kv(6, "confidence", blankAs(chain.Confidence, "inferred"))
			builder.kv(6, "evidence", chain.Evidence)
			writeSemanticStringList(builder, 6, "source_relations", chain.SourceRelations)
		}
	}
}

func writeSemanticStringList(builder *semanticYAMLBuilder, indent int, key string, values []string) {
	if len(values) == 0 {
		return
	}
	builder.raw(indent, key+":")
	for _, value := range values {
		builder.raw(indent+1, "- "+yamlScalar(value))
	}
}

func writeSemanticHVAC(builder *semanticYAMLBuilder, ctx *semanticContext) {
	if len(ctx.hvac.Loops) == 0 && len(ctx.hvac.NodeUsages) == 0 {
		builder.raw(1, "hvac: {}")
		return
	}
	builder.raw(1, "hvac:")
	writeSemanticHVACLoops(builder, ctx, "air_loops", "AirLoopHVAC")
	writeSemanticHVACLoops(builder, ctx, "plant_loops", "PlantLoop")
	writeSemanticHVACLoops(builder, ctx, "condenser_loops", "CondenserLoop")
	writeSemanticHVACEquipmentCatalog(builder, ctx)
	writeSemanticHVACNodes(builder, ctx)
}

func writeSemanticAirflows(builder *semanticYAMLBuilder, ctx *semanticContext) {
	var afn []Object
	for _, obj := range ctx.doc.Objects {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(obj.Type)), "airflownetwork:") {
			afn = append(afn, obj)
		}
	}
	if len(afn) == 0 {
		builder.raw(1, "airflows: {}")
		return
	}
	builder.raw(1, "airflows:")
	builder.raw(2, "airflow_network:")
	for _, obj := range afn {
		ctx.mark(obj.Index)
		writeSemanticReferenceObject(builder, 3, obj)
	}
}

func writeSemanticHVACLoops(builder *semanticYAMLBuilder, ctx *semanticContext, key string, loopType string) {
	var loops []HVACLoop
	for _, loop := range ctx.hvac.Loops {
		if strings.EqualFold(loop.Type, loopType) {
			loops = append(loops, loop)
		}
	}
	if len(loops) == 0 {
		return
	}
	builder.raw(2, key+":")
	for _, loop := range loops {
		ctx.mark(loop.ObjectIndex)
		builder.fieldKV(3, "- name", loop.Name, loop.ObjectIndex, loop.Type, loop.Name, 0)
		builder.kvForObject(4, "class", loop.Type, loop.ObjectIndex, loop.Type, loop.Name)
		if loop.OperationScheme != "" || loop.AvailabilityManagerList != "" || loop.SetpointNode != "" {
			builder.rawForObject(4, "controls:", loop.ObjectIndex, loop.Type, loop.Name)
			if loop.OperationScheme != "" {
				builder.kvForObject(5, "operation_scheme", loop.OperationScheme, loop.ObjectIndex, loop.Type, loop.Name)
			}
			if loop.AvailabilityManagerList != "" {
				builder.kvForObject(5, "availability_managers", loop.AvailabilityManagerList, loop.ObjectIndex, loop.Type, loop.Name)
			}
			if loop.SetpointNode != "" {
				builder.rawForObject(5, "setpoints:", loop.ObjectIndex, loop.Type, loop.Name)
				builder.kvForObject(6, "- node", loop.SetpointNode, loop.ObjectIndex, loop.Type, loop.Name)
			}
		}
		builder.rawForObject(4, "computed:", loop.ObjectIndex, loop.Type, loop.Name)
		builder.rawForObject(5, "summary_path: "+yamlScalar(semanticHVACSummaryPath(loop)), loop.ObjectIndex, loop.Type, loop.Name)
		writeSemanticHVACSide(builder, ctx, 4, "supply_side", loop.SupplySide)
		writeSemanticHVACSide(builder, ctx, 4, "demand_side", loop.DemandSide)
	}
}

func writeSemanticHVACSide(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, key string, side HVACLoopSide) {
	if side.Name == "" && len(side.Branches) == 0 {
		return
	}
	builder.raw(indent, key+":")
	if side.InletNode != "" {
		builder.kv(indent+1, "inlet_node", side.InletNode)
	}
	if side.OutletNode != "" {
		builder.kv(indent+1, "outlet_node", side.OutletNode)
	}
	if side.BranchListName != "" {
		builder.kv(indent+1, "branch_list", side.BranchListName)
	}
	if side.ConnectorListName != "" {
		builder.kv(indent+1, "connector_list", side.ConnectorListName)
	}
	if len(side.Branches) > 0 {
		builder.raw(indent+1, "branches:")
		for _, branch := range side.Branches {
			ctx.mark(branch.ObjectIndex)
			builder.rawForObject(indent+2, "- name: "+yamlScalar(branch.Name), branch.ObjectIndex, "Branch", branch.Name)
			if branch.InletNode != "" {
				builder.kvForObject(indent+3, "inlet_node", branch.InletNode, branch.ObjectIndex, "Branch", branch.Name)
			}
			if branch.OutletNode != "" {
				builder.kvForObject(indent+3, "outlet_node", branch.OutletNode, branch.ObjectIndex, "Branch", branch.Name)
			}
			if len(branch.Components) > 0 {
				builder.rawForObject(indent+3, "components:", branch.ObjectIndex, "Branch", branch.Name)
				for _, component := range branch.Components {
					ctx.mark(component.ObjectIndex)
					writeSemanticHVACComponent(builder, indent+4, component, "loop_component")
				}
			}
		}
	}
	if len(side.Connectors) > 0 {
		builder.raw(indent+1, "connectors:")
		for _, connector := range side.Connectors {
			ctx.mark(connector.ObjectIndex)
			builder.rawForObject(indent+2, "- name: "+yamlScalar(connector.Name), connector.ObjectIndex, connector.Type, connector.Name)
			builder.kvForObject(indent+3, "class", connector.Type, connector.ObjectIndex, connector.Type, connector.Name)
			if connector.InletBranchName != "" {
				builder.kvForObject(indent+3, "inlet_branch", connector.InletBranchName, connector.ObjectIndex, connector.Type, connector.Name)
			}
			if connector.OutletBranchName != "" {
				builder.kvForObject(indent+3, "outlet_branch", connector.OutletBranchName, connector.ObjectIndex, connector.Type, connector.Name)
			}
			if len(connector.BranchNames) > 0 {
				builder.rawForObject(indent+3, "branches:", connector.ObjectIndex, connector.Type, connector.Name)
				for _, branchName := range connector.BranchNames {
					builder.rawForObject(indent+4, "- "+yamlScalar(branchName), connector.ObjectIndex, connector.Type, connector.Name)
				}
			}
		}
	}
}

func writeSemanticHVACComponent(builder *semanticYAMLBuilder, indent int, component HVACComponent, role string) {
	name := component.ObjectName
	objectIndex := component.ObjectIndex
	builder.rawForObject(indent, "- name: "+yamlScalar(name), objectIndex, component.ObjectType, name)
	builder.kvForObject(indent+1, "class", component.ObjectType, objectIndex, component.ObjectType, name)
	if component.Family != "" {
		builder.kvForObject(indent+1, "family", component.Family, objectIndex, component.ObjectType, name)
	}
	if component.FamilyLabel != "" {
		builder.kvForObject(indent+1, "family_label", component.FamilyLabel, objectIndex, component.ObjectType, name)
	}
	if component.DisplayLabel != "" {
		builder.kvForObject(indent+1, "display_label", component.DisplayLabel, objectIndex, component.ObjectType, name)
	}
	roleHere := firstNonEmpty(component.RoleHere, role)
	builder.kvForObject(indent+1, "role_here", roleHere, objectIndex, component.ObjectType, name)
	if component.RelationSource != "" || component.RelationConfidence != "" || len(component.RelationEvidence) > 0 {
		builder.rawForObject(indent+1, "relation:", objectIndex, component.ObjectType, name)
		if component.RelationSource != "" {
			builder.kvForObject(indent+2, "source", component.RelationSource, objectIndex, component.ObjectType, name)
		}
		if component.RelationConfidence != "" {
			builder.kvForObject(indent+2, "confidence", component.RelationConfidence, objectIndex, component.ObjectType, name)
		}
		writeSemanticStringList(builder, indent+2, "evidence", component.RelationEvidence)
	}
	if component.CoolingSequence != "" || component.HeatingSequence != "" {
		builder.rawForObject(indent+1, "sequence:", objectIndex, component.ObjectType, name)
		if component.CoolingSequence != "" {
			builder.kvForObject(indent+2, "cooling", component.CoolingSequence, objectIndex, component.ObjectType, name)
		}
		if component.HeatingSequence != "" {
			builder.kvForObject(indent+2, "heating_or_no_load", component.HeatingSequence, objectIndex, component.ObjectType, name)
		}
	}
	if component.CoolingFractionSchedule != "" || component.HeatingFractionSchedule != "" {
		builder.rawForObject(indent+1, "sequential_fraction_schedules:", objectIndex, component.ObjectType, name)
		if component.CoolingFractionSchedule != "" {
			builder.kvForObject(indent+2, "cooling_fraction_schedule", component.CoolingFractionSchedule, objectIndex, component.ObjectType, name)
		}
		if component.HeatingFractionSchedule != "" {
			builder.kvForObject(indent+2, "heating_fraction_schedule", component.HeatingFractionSchedule, objectIndex, component.ObjectType, name)
		}
	}
	if component.InletNode != "" {
		builder.kvForObject(indent+1, "inlet_node", component.InletNode, objectIndex, component.ObjectType, name)
	}
	if component.OutletNode != "" {
		builder.kvForObject(indent+1, "outlet_node", component.OutletNode, objectIndex, component.ObjectType, name)
	}
	if component.WaterInletNode != "" {
		builder.kvForObject(indent+1, "water_inlet_node", component.WaterInletNode, objectIndex, component.ObjectType, name)
	}
	if component.WaterOutletNode != "" {
		builder.kvForObject(indent+1, "water_outlet_node", component.WaterOutletNode, objectIndex, component.ObjectType, name)
	}
	if !component.Exists {
		builder.kvForObject(indent+1, "exists", "false", objectIndex, component.ObjectType, name)
		builder.kvForObject(indent+1, "reason", "unresolved_component_reference", objectIndex, component.ObjectType, name)
	}
	if objectIndex >= 0 {
		writeSemanticDuplicatedAs(builder, indent+1, objectIndex, component.ObjectType, name, roleHere, []string{"hvac/equipment_catalog/" + blankAs(name, component.ObjectType)})
	}
}

func semanticHVACSummaryPath(loop HVACLoop) string {
	var parts []string
	for _, side := range []HVACLoopSide{loop.SupplySide, loop.DemandSide} {
		if side.InletNode != "" {
			parts = append(parts, side.InletNode)
		}
		for _, branch := range side.Branches {
			for _, component := range branch.Components {
				parts = append(parts, componentLabel(component))
				if component.OutletNode != "" {
					parts = append(parts, component.OutletNode)
				}
			}
		}
	}
	return strings.Join(sortedUniqueConsecutive(parts), " -> ")
}

func sortedUniqueConsecutive(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1] == value {
			continue
		}
		out = append(out, value)
	}
	return out
}

func writeSemanticDuplicatedAs(builder *semanticYAMLBuilder, indent int, objectIndex int, objectType string, objectName string, role string, alsoShownIn []string) {
	builder.rawForObject(indent, "duplicated_as:", objectIndex, objectType, objectName)
	builder.kvForObject(indent+1, "group", "obj-"+fmt.Sprintf("%d", objectIndex), objectIndex, objectType, objectName)
	builder.kvForObject(indent+1, "role_here", role, objectIndex, objectType, objectName)
	builder.rawForObject(indent+1, "also_shown_in:", objectIndex, objectType, objectName)
	for _, path := range sortedUniqueStrings(alsoShownIn) {
		builder.rawForObject(indent+2, "- "+yamlScalar(path), objectIndex, objectType, objectName)
	}
	builder.kvForObject(indent+1, "sync_policy", "edit_once_sync_all", objectIndex, objectType, objectName)
}

func semanticZoneGroupOccurrencePaths(members []string, obj Object) []string {
	_, bucket, _ := semanticZoneAttachment(obj)
	section := "loads"
	if semanticAirExchangeBucket(bucket) {
		section = "air_exchange"
	}
	name := blankAs(objectName(obj), obj.Type)
	paths := make([]string, 0, len(members))
	for _, zone := range members {
		paths = append(paths, "zones/"+zone+"/"+section+"/"+bucket+"/"+name)
	}
	return paths
}

func writeSemanticHVACEquipmentCatalog(builder *semanticYAMLBuilder, ctx *semanticContext) {
	seen := map[int]HVACComponent{}
	for _, loop := range ctx.hvac.Loops {
		for _, component := range loopComponents(loop) {
			if component.ObjectIndex >= 0 && component.Exists {
				seen[component.ObjectIndex] = component
			}
		}
	}
	for _, relation := range ctx.hvac.ZoneRelations {
		for _, component := range append(append([]HVACComponent{}, relation.TerminalUnits...), append(relation.ZoneEquipment, relation.PlantEquipment...)...) {
			if component.ObjectIndex >= 0 && component.Exists {
				seen[component.ObjectIndex] = component
			}
		}
	}
	if len(seen) == 0 {
		return
	}
	byBucket := map[string][]HVACComponent{}
	for _, component := range seen {
		bucket := semanticHVACEquipmentBucket(component.ObjectType)
		byBucket[bucket] = append(byBucket[bucket], component)
	}
	builder.raw(2, "equipment_catalog:")
	for _, bucket := range []string{
		"fans",
		"coils",
		"pumps",
		"pipes",
		"chillers",
		"boilers",
		"cooling_towers",
		"heat_pumps",
		"water_heaters",
		"thermal_storage",
		"heat_exchangers",
		"district_energy",
		"terminals",
		"unitary_systems",
		"outdoor_air",
		"controllers",
		"setpoint_managers",
		"availability_managers",
		"plant_components",
		"zone_hvac",
		"air_distribution",
		"other",
	} {
		components := byBucket[bucket]
		if len(components) == 0 {
			continue
		}
		sort.SliceStable(components, func(i, j int) bool {
			return components[i].ObjectIndex < components[j].ObjectIndex
		})
		builder.raw(3, bucket+":")
		for _, component := range components {
			ctx.mark(component.ObjectIndex)
			writeSemanticHVACComponent(builder, 4, component, "equipment_catalog")
		}
	}
}

func semanticHVACEquipmentBucket(objectType string) string {
	family, _ := hvacComponentFamily(objectType)
	switch {
	case family == "fan":
		return "fans"
	case strings.Contains(family, "coil"):
		return "coils"
	case family == "pump":
		return "pumps"
	case family == "pipe":
		return "pipes"
	case family == "chiller":
		return "chillers"
	case family == "boiler":
		return "boilers"
	case family == "cooling_tower":
		return "cooling_towers"
	case family == "heat_pump":
		return "heat_pumps"
	case family == "water_heater":
		return "water_heaters"
	case family == "thermal_storage":
		return "thermal_storage"
	case family == "heat_exchanger":
		return "heat_exchangers"
	case family == "district_cooling" || family == "district_heating":
		return "district_energy"
	case family == "terminal":
		return "terminals"
	case family == "unitary_system":
		return "unitary_systems"
	case family == "outdoor_air":
		return "outdoor_air"
	case family == "controller":
		return "controllers"
	case family == "setpoint_manager":
		return "setpoint_managers"
	case family == "availability_manager":
		return "availability_managers"
	case family == "plant_component":
		return "plant_components"
	case family == "zone_hvac":
		return "zone_hvac"
	case family == "air_distribution":
		return "air_distribution"
	default:
		return "other"
	}
}

func writeSemanticHVACNodes(builder *semanticYAMLBuilder, ctx *semanticContext) {
	if len(ctx.hvac.NodeUsages) == 0 {
		return
	}
	builder.raw(2, "nodes:")
	byName := map[string][]HVACNodeUsage{}
	for _, usage := range ctx.hvac.NodeUsages {
		byName[normalizeName(usage.NodeName)] = append(byName[normalizeName(usage.NodeName)], usage)
	}
	keys := make([]string, 0, len(byName))
	for key := range byName {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		usages := byName[key]
		if len(usages) == 0 {
			continue
		}
		nodeName := usages[0].NodeName
		builder.raw(3, "- name: "+yamlScalar(nodeName))
		builder.raw(4, "defined_by: []")
		var listedBy []HVACNodeUsage
		var usedBy []HVACNodeUsage
		for _, usage := range usages {
			if usage.Role == "node_list_member" {
				listedBy = append(listedBy, usage)
			} else {
				usedBy = append(usedBy, usage)
			}
		}
		if len(listedBy) > 0 {
			builder.raw(4, "listed_by:")
			for _, usage := range listedBy {
				label := usage.ObjectType
				if usage.ObjectName != "" {
					label += " " + usage.ObjectName
				}
				builder.rawForObject(5, "- "+yamlScalar(label), usage.ObjectIndex, usage.ObjectType, usage.ObjectName)
			}
		}
		builder.kv(4, "node_kind", semanticHVACNodeKind(usedBy))
		builder.kv(4, "expected_degree", semanticHVACNodeExpectedDegree(usedBy))
		if len(usedBy) == 0 {
			builder.raw(4, "used_by: []")
			continue
		}
		builder.raw(4, "used_by:")
		for _, usage := range usedBy {
			label := usage.ObjectType
			if usage.ObjectName != "" {
				label += " " + usage.ObjectName
			}
			label += " / " + usage.Role
			builder.rawForObject(5, "- "+yamlScalar(label), usage.ObjectIndex, usage.ObjectType, usage.ObjectName)
		}
	}
}

func semanticHVACNodeKind(usages []HVACNodeUsage) string {
	for _, usage := range usages {
		if strings.Contains(usage.Role, "outdoor") || strings.Contains(usage.Role, "relief") {
			return "outdoor_or_relief"
		}
	}
	if len(usages) <= 1 {
		return "one_sided"
	}
	return "connected"
}

func semanticHVACNodeExpectedDegree(usages []HVACNodeUsage) string {
	switch semanticHVACNodeKind(usages) {
	case "outdoor_or_relief":
		return "one_or_more"
	case "one_sided":
		return "check_context"
	default:
		return "producer_and_consumer"
	}
}

func writeSemanticOutputs(builder *semanticYAMLBuilder, ctx *semanticContext) {
	builder.raw(1, "outputs:")
	builder.raw(2, "files:")
	writeSemanticOutputFileStatus(builder, ctx, 3, "csv", "")
	writeSemanticOutputFileStatus(builder, ctx, 3, "sqlite", "Output:SQLite")
	writeSemanticOutputFileStatus(builder, ctx, 3, "json", "Output:JSON")
	var variables []OutputObjectSummary
	var meters []OutputObjectSummary
	var summaryReports []OutputObjectSummary
	var tableStyles []OutputObjectSummary
	var monthlyTables []OutputObjectSummary
	var annualTables []OutputObjectSummary
	var timeBinTables []OutputObjectSummary
	var wildcard []OutputObjectSummary
	var unresolved []OutputObjectSummary
	for _, item := range ctx.output.Existing {
		ctx.mark(item.ObjectIndex)
		switch strings.ToLower(item.ObjectType) {
		case "output:variable":
			variables = append(variables, item)
			if strings.TrimSpace(item.KeyValue) == "*" {
				wildcard = append(wildcard, item)
			} else if semanticOutputAttachmentResolution(ctx, item) == "unresolved_after_rdd" {
				unresolved = append(unresolved, item)
			}
		case "output:meter", "output:meter:meterfileonly", "output:meter:cumulative", "output:meter:cumulativemeterfileonly":
			meters = append(meters, item)
		case "output:table:summaryreports":
			summaryReports = append(summaryReports, item)
		case "outputcontrol:table:style":
			tableStyles = append(tableStyles, item)
		case "output:table:monthly":
			monthlyTables = append(monthlyTables, item)
		case "output:table:annual":
			annualTables = append(annualTables, item)
		case "output:table:timebins":
			timeBinTables = append(timeBinTables, item)
		}
	}
	writeSemanticOutputList(builder, 2, "variables", variables, true)
	writeSemanticOutputList(builder, 2, "meters", meters, true)
	writeSemanticOutputList(builder, 2, "wildcard", wildcard, true)
	writeSemanticOutputList(builder, 2, "unresolved", unresolved, true)
	if len(summaryReports) > 0 || len(tableStyles) > 0 || len(monthlyTables) > 0 || len(annualTables) > 0 || len(timeBinTables) > 0 {
		builder.raw(2, "tabular:")
		writeSemanticOutputList(builder, 3, "summary_reports", summaryReports, false)
		writeSemanticOutputList(builder, 3, "monthly", monthlyTables, false)
		writeSemanticOutputList(builder, 3, "annual", annualTables, false)
		writeSemanticOutputList(builder, 3, "time_bins", timeBinTables, false)
		if len(tableStyles) > 0 {
			builder.raw(3, "style:")
			for _, item := range tableStyles {
				builder.rawForObject(4, "- source: "+yamlScalar(item.ObjectType), item.ObjectIndex, item.ObjectType, item.ObjectName)
				for _, field := range item.Fields {
					if strings.TrimSpace(field.Value) != "" {
						builder.fieldKV(5, semanticFieldKeyFromName(field.Name), field.Value, item.ObjectIndex, item.ObjectType, item.ObjectName, field.Index)
					}
				}
			}
		}
	}
	writeSemanticHeatFlowOutputGroup(builder, 2, variables)
}

func writeSemanticOutputFileStatus(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, key string, requestObjectType string) {
	controlEnabled, state := semanticOutputControlFilesState(ctx.doc, key)
	requested := true
	requestSource := "default"
	if requestObjectType != "" {
		requested = semanticOutputObjectExists(ctx.output, requestObjectType)
		requestSource = requestObjectType
	}
	enabled := controlEnabled
	if requestObjectType != "" {
		enabled = requested && controlEnabled
	}
	builder.raw(indent, key+":")
	builder.kv(indent+1, "enabled", fmt.Sprintf("%t", enabled))
	builder.kv(indent+1, "requested", fmt.Sprintf("%t", requested))
	builder.kv(indent+1, "disabled", fmt.Sprintf("%t", !controlEnabled))
	builder.kv(indent+1, "state", state)
	builder.kv(indent+1, "source", "OutputControl:Files")
	builder.kv(indent+1, "request_source", requestSource)
}

func writeSemanticAttachedOutputs(builder *semanticYAMLBuilder, indent int, key string, outputs []OutputObjectSummary) {
	writeSemanticOutputList(builder, indent, key, outputs, false)
}

func writeSemanticOutputList(builder *semanticYAMLBuilder, indent int, key string, outputs []OutputObjectSummary, includeTarget bool) {
	if len(outputs) == 0 {
		return
	}
	builder.raw(indent, key+":")
	for _, output := range outputs {
		label := semanticOutputLabel(output, includeTarget)
		switch key {
		case "wildcard":
			builder.rawForObject(indent+1, "- request: "+yamlScalar(label), output.ObjectIndex, output.ObjectType, output.ObjectName)
			builder.kvForObject(indent+2, "scope", semanticOutputWildcardFamily(output), output.ObjectIndex, output.ObjectType, output.ObjectName)
			builder.kvForObject(indent+2, "attachment_resolution", semanticOutputAttachmentResolution(builder.ctx, output), output.ObjectIndex, output.ObjectType, output.ObjectName)
		case "unresolved":
			builder.rawForObject(indent+1, "- request: "+yamlScalar(label), output.ObjectIndex, output.ObjectType, output.ObjectName)
			builder.kvForObject(indent+2, "attachment_resolution", semanticOutputAttachmentResolution(builder.ctx, output), output.ObjectIndex, output.ObjectType, output.ObjectName)
			builder.kvForObject(indent+2, "reason", "target_key_not_resolved", output.ObjectIndex, output.ObjectType, output.ObjectName)
		default:
			builder.rawForObject(indent+1, "- "+yamlScalar(label), output.ObjectIndex, output.ObjectType, output.ObjectName)
			if strings.EqualFold(output.ObjectType, "Output:Variable") {
				builder.kvForObject(indent+2, "attachment_resolution", semanticOutputAttachmentResolution(builder.ctx, output), output.ObjectIndex, output.ObjectType, output.ObjectName)
			}
		}
	}
}

func writeSemanticSourceNameConflicts(builder *semanticYAMLBuilder, duplicates []SemanticSourceNameConflict) {
	if len(duplicates) == 0 {
		builder.raw(1, "source_name_conflicts: []")
		return
	}
	builder.raw(1, "source_name_conflicts:")
	for _, group := range duplicates {
		builder.raw(2, "- group: "+yamlScalar(group.Group))
		builder.kv(3, "object_type", group.ObjectType)
		builder.kv(3, "name", group.Name)
		builder.raw(3, "object_indexes:")
		for _, index := range group.ObjectIndexes {
			builder.raw(4, "- "+fmt.Sprintf("%d", index))
		}
		builder.kv(3, "suggested_action", "rename_later_duplicates")
	}
}

func writeSemanticMiscellaneous(builder *semanticYAMLBuilder, ctx *semanticContext) {
	var unmapped []Object
	for _, obj := range ctx.doc.Objects {
		if !ctx.mapped[obj.Index] {
			unmapped = append(unmapped, obj)
		}
	}
	if len(unmapped) == 0 {
		builder.raw(1, "miscellaneous:")
		builder.raw(2, "other: []")
		return
	}
	builder.raw(1, "miscellaneous:")
	builder.raw(2, "other:")
	for _, obj := range unmapped {
		name := objectName(obj)
		builder.objectKV(3, "- class", obj.Type, obj.Index, obj.Type, name)
		if name != "" {
			builder.fieldKV(4, "name", name, obj.Index, obj.Type, name, 0)
		}
		builder.kvForObject(4, "reason", semanticMiscReason(obj.Type), obj.Index, obj.Type, name)
		if suggested := semanticSuggestedSection(obj.Type); suggested != "" {
			builder.kvForObject(4, "suggested_section", suggested, obj.Index, obj.Type, name)
		}
		builder.rawForObject(4, "source:", obj.Index, obj.Type, name)
		builder.kvForObject(5, "object_index", fmt.Sprintf("%d", obj.Index), obj.Index, obj.Type, name)
		builder.kvForObject(5, "object_type", obj.Type, obj.Index, obj.Type, name)
		builder.rawForObject(4, "fields:", obj.Index, obj.Type, name)
		for fieldIndex, field := range obj.Fields {
			if fieldIndex == 0 && name != "" {
				continue
			}
			builder.fieldKV(5, semanticFieldKey(field, fieldIndex), field.Value, obj.Index, obj.Type, name, fieldIndex)
		}
		builder.kvForObject(4, "export_policy", "preserve_exactly", obj.Index, obj.Type, name)
	}
}

func ApplySemanticDuplicateNameFixes(doc Document) (Document, []SemanticDuplicateFix) {
	updated := doc.clone()
	seenByType := map[string]map[string]bool{}
	reservedByType := semanticReservedNamesByType(updated)
	nextCountByTypeName := map[string]int{}
	var fixes []SemanticDuplicateFix

	for index := range updated.Objects {
		obj := &updated.Objects[index]
		name := objectName(*obj)
		if strings.TrimSpace(name) == "" || len(obj.Fields) == 0 {
			continue
		}
		typeKey := normalizeName(obj.Type)
		if seenByType[typeKey] == nil {
			seenByType[typeKey] = map[string]bool{}
		}
		nameKey := normalizeName(name)
		groupKey := typeKey + "/" + nameKey
		if !seenByType[typeKey][nameKey] {
			seenByType[typeKey][nameKey] = true
			nextCountByTypeName[groupKey] = 2
			continue
		}
		nextName := semanticUniqueName(name, reservedByType[typeKey], nextCountByTypeName[groupKey])
		nextCountByTypeName[groupKey]++
		obj.Fields[0].Value = nextName
		seenByType[typeKey][normalizeName(nextName)] = true
		reservedByType[typeKey][normalizeName(nextName)] = true
		fixes = append(fixes, SemanticDuplicateFix{
			ObjectIndex: obj.Index,
			ObjectType:  obj.Type,
			Before:      name,
			After:       nextName,
		})
	}
	reindexObjects(&updated)
	return updated, fixes
}

func semanticReservedNamesByType(doc Document) map[string]map[string]bool {
	reserved := map[string]map[string]bool{}
	for _, obj := range doc.Objects {
		name := objectName(obj)
		if strings.TrimSpace(name) == "" {
			continue
		}
		typeKey := normalizeName(obj.Type)
		if reserved[typeKey] == nil {
			reserved[typeKey] = map[string]bool{}
		}
		reserved[typeKey][normalizeName(name)] = true
	}
	return reserved
}

func semanticUniqueName(base string, existing map[string]bool, start int) string {
	for index := start; ; index++ {
		candidate := fmt.Sprintf("%s %d", strings.TrimSpace(base), index)
		if !existing[normalizeName(candidate)] {
			return candidate
		}
	}
}

func writeSemanticSourcePreservation(builder *semanticYAMLBuilder, ctx *semanticContext) {
	builder.raw(1, "source_preservation:")
	builder.kv(2, "object_order", "preserved")
	builder.kv(2, "field_order", "preserved")
	builder.kv(2, "comments", "best_effort_from_current_parser")
	builder.kv(2, "mode", "internal_projection")
	builder.kv(2, "editable_scope", "visible_raw_fields_only")
	builder.kv(2, "roundtrip_scope", "app_state_patch_not_standalone_yaml_import")
	builder.kv(2, "source_registry", "internal_idf_document")
	builder.kv(2, "unmapped_policy", "miscellaneous_preserve_exactly")
	entries := []Object{}
	for _, obj := range ctx.doc.Objects {
		if !ctx.mapped[obj.Index] {
			continue
		}
		if semanticUnshownFieldCount(ctx, obj.Index) > 0 {
			entries = append(entries, obj)
		}
	}
	if len(entries) == 0 {
		builder.raw(2, "mapped_object_unshown_fields: []")
		return
	}
	builder.raw(2, "mapped_object_unshown_fields:")
	for _, obj := range entries {
		count := semanticUnshownFieldCount(ctx, obj.Index)
		name := objectName(obj)
		label := obj.Type
		if name != "" {
			label += " " + name
		}
		builder.rawForObject(3, "- object: "+yamlScalar(label), obj.Index, obj.Type, name)
		builder.kvForObject(4, "source_object_index", fmt.Sprintf("%d", obj.Index), obj.Index, obj.Type, name)
		builder.kvForObject(4, "unshown_field_count", fmt.Sprintf("%d", count), obj.Index, obj.Type, name)
		builder.rawForObject(4, "fields:", obj.Index, obj.Type, name)
		for fieldIndex, field := range obj.Fields {
			if strings.TrimSpace(field.Value) == "" || ctx.shownFields[obj.Index][fieldIndex] {
				continue
			}
			fieldName := catalogFieldName(obj, fieldIndex)
			if fieldName == "" {
				fieldName = field.Comment
			}
			builder.rawForObject(5, "- index: "+fmt.Sprintf("%d", fieldIndex), obj.Index, obj.Type, name)
			builder.kvForObject(6, "name", blankAs(fieldName, fmt.Sprintf("Field %d", fieldIndex+1)), obj.Index, obj.Type, name)
			builder.kvForObject(6, "value", field.Value, obj.Index, obj.Type, name)
		}
	}
}

func semanticUnshownFieldCount(ctx *semanticContext, objectIndex int) int {
	obj, ok := ctx.objectByIndex[objectIndex]
	if !ok {
		return 0
	}
	shown := ctx.shownFields[objectIndex]
	count := 0
	for index, field := range obj.Fields {
		if strings.TrimSpace(field.Value) == "" {
			continue
		}
		if shown[index] {
			continue
		}
		count++
	}
	return count
}

func semanticObjectTypesWithPrefix(doc Document, prefix string) []string {
	seen := map[string]bool{}
	var out []string
	for _, obj := range doc.Objects {
		if strings.HasPrefix(strings.ToLower(obj.Type), strings.ToLower(prefix)) && !seen[strings.ToLower(obj.Type)] {
			seen[strings.ToLower(obj.Type)] = true
			out = append(out, obj.Type)
		}
	}
	sort.Strings(out)
	return out
}

func semanticObjectsForTypes(doc Document, objectTypes []string) []Object {
	wanted := map[string]bool{}
	for _, objectType := range objectTypes {
		wanted[normalizeName(objectType)] = true
	}
	var objects []Object
	for _, obj := range doc.Objects {
		if wanted[normalizeName(obj.Type)] {
			objects = append(objects, obj)
		}
	}
	return objects
}

func semanticObjectsWithTypePrefix(doc Document, prefix string) []Object {
	var objects []Object
	for _, obj := range doc.Objects {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(obj.Type)), strings.ToLower(strings.TrimSpace(prefix))) {
			objects = append(objects, obj)
		}
	}
	return objects
}

func semanticMaterialObjects(doc Document) []Object {
	var objects []Object
	for _, obj := range doc.Objects {
		if isGeometryMaterialType(obj.Type) {
			objects = append(objects, obj)
		}
	}
	return objects
}

func semanticFenestrationConstructionObjects(doc Document) []Object {
	var objects []Object
	for _, obj := range doc.Objects {
		lower := strings.ToLower(strings.TrimSpace(obj.Type))
		if strings.HasPrefix(lower, "construction:") && !semanticIsPerformanceConstructionType(lower) {
			objects = append(objects, obj)
		}
	}
	return objects
}

func semanticPerformanceConstructionObjects(doc Document) []Object {
	var objects []Object
	for _, obj := range doc.Objects {
		if semanticIsPerformanceConstructionType(strings.ToLower(strings.TrimSpace(obj.Type))) {
			objects = append(objects, obj)
		}
	}
	return objects
}

func semanticIsPerformanceConstructionType(lower string) bool {
	return strings.Contains(lower, "ffactorgroundfloor") ||
		strings.Contains(lower, "cfactorundergroundwall")
}

func semanticFieldKeyFromName(name string) string {
	return semanticFieldKey(Field{Comment: name}, 0)
}

func semanticMaterialUsedBy(doc Document, materialName string) []string {
	var out []string
	key := normalizeName(materialName)
	for _, obj := range doc.Objects {
		if !strings.EqualFold(obj.Type, "Construction") || objectName(obj) == "" {
			continue
		}
		for fieldIndex := 1; fieldIndex < len(obj.Fields); fieldIndex++ {
			if normalizeName(obj.Fields[fieldIndex].Value) == key {
				out = append(out, "constructions/opaque/"+objectName(obj))
			}
		}
	}
	return sortedUniqueStrings(out)
}

func semanticConstructionUsedBy(ctx *semanticContext, constructionName string) []string {
	var out []string
	key := normalizeName(constructionName)
	for _, surface := range ctx.geometry.Surfaces {
		if normalizeName(surface.Construction) == key {
			out = append(out, "zones/"+surface.ZoneName+"/geometry/surfaces/"+surface.Name)
		}
	}
	for _, window := range ctx.geometry.Windows {
		if normalizeName(window.Construction) == key {
			out = append(out, "zones/"+window.ZoneName+"/geometry/fenestration/"+window.Name)
		}
	}
	return sortedUniqueStrings(out)
}

func writeSemanticReferenceObject(builder *semanticYAMLBuilder, indent int, obj Object) {
	name := objectName(obj)
	if name != "" {
		builder.fieldKV(indent, "- name", name, obj.Index, obj.Type, name, 0)
		builder.kvForObject(indent+1, "class", obj.Type, obj.Index, obj.Type, name)
	} else {
		builder.objectKV(indent, "- class", obj.Type, obj.Index, obj.Type, name)
	}
	builder.kvForObject(indent+1, "source_object_index", fmt.Sprintf("%d", obj.Index), obj.Index, obj.Type, name)
	for fieldIndex, field := range obj.Fields {
		if fieldIndex == 0 && name != "" {
			continue
		}
		key := semanticFieldKey(field, fieldIndex)
		builder.fieldKV(indent+1, key, field.Value, obj.Index, obj.Type, name, fieldIndex)
	}
}

func semanticZones(ctx *semanticContext) []GeometryZone {
	if len(ctx.geometry.Zones) > 0 {
		return ctx.geometry.Zones
	}
	var zones []GeometryZone
	for _, obj := range ctx.doc.Objects {
		if strings.EqualFold(obj.Type, "Zone") {
			zones = append(zones, GeometryZone{ObjectIndex: obj.Index, Name: objectName(obj)})
		}
	}
	return zones
}

func semanticZoneObject(ctx *semanticContext, zoneName string) (Object, bool) {
	for _, obj := range ctx.doc.Objects {
		if strings.EqualFold(obj.Type, "Zone") && strings.EqualFold(objectName(obj), zoneName) {
			return obj, true
		}
	}
	return Object{}, false
}

func semanticZoneObjectIndex(ctx *semanticContext, zoneName string) int {
	if obj, ok := semanticZoneObject(ctx, zoneName); ok {
		return obj.Index
	}
	return -1
}

func semanticOutdoorAirSpecsForZone(ctx *semanticContext, zoneName string) []Object {
	dsoaByName := map[string]Object{}
	for _, obj := range ctx.doc.Objects {
		if strings.EqualFold(obj.Type, "DesignSpecification:OutdoorAir") {
			dsoaByName[normalizeName(objectName(obj))] = obj
		}
	}
	if len(dsoaByName) == 0 {
		return nil
	}
	zoneKey := normalizeName(zoneName)
	seen := map[int]bool{}
	var out []Object
	for _, obj := range ctx.doc.Objects {
		target, _, ok := semanticFieldValue(obj, "Zone or ZoneList Name", "Zone Name")
		if !ok {
			continue
		}
		targetMatches := normalizeName(target) == zoneKey
		if !targetMatches {
			for _, member := range ctx.zoneLists[normalizeName(target)] {
				if normalizeName(member) == zoneKey {
					targetMatches = true
					break
				}
			}
		}
		if !targetMatches {
			continue
		}
		for index, field := range obj.Fields {
			fieldName := catalogFieldName(obj, index)
			if fieldName == "" {
				fieldName = field.Comment
			}
			normalized := normalizeFieldName(fieldName)
			if !strings.Contains(normalized, "outdoor air") || !strings.Contains(normalized, "name") {
				continue
			}
			if spec, ok := dsoaByName[normalizeName(field.Value)]; ok && !seen[spec.Index] {
				seen[spec.Index] = true
				out = append(out, spec)
			}
		}
	}
	return out
}

func writeSemanticSurfaceBoundaryResolution(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, surface GeometrySurface) {
	if !strings.EqualFold(surface.OutsideBoundary, "Surface") {
		return
	}
	counterpart, _, _ := semanticFieldValue(ctx.objectByIndex[surface.ObjectIndex], "Outside Boundary Condition Object")
	resolution := "missing"
	if counterpart != "" {
		resolution = "one_way"
		for _, other := range ctx.geometry.Surfaces {
			if strings.EqualFold(other.Name, counterpart) {
				reverse, _, _ := semanticFieldValue(ctx.objectByIndex[other.ObjectIndex], "Outside Boundary Condition Object")
				if strings.EqualFold(reverse, surface.Name) {
					resolution = "resolved"
				}
				break
			}
		}
	}
	builder.kvForObject(indent, "counterpart", blankAs(counterpart, "missing"), surface.ObjectIndex, surface.Type, surface.Name)
	builder.kvForObject(indent, "resolution", resolution, surface.ObjectIndex, surface.Type, surface.Name)
}

func writeSemanticBoundaryExposureValidation(builder *semanticYAMLBuilder, ctx *semanticContext, indent int, surface GeometrySurface) {
	obj := ctx.objectByIndex[surface.ObjectIndex]
	sun, _, _ := semanticFieldValue(obj, "Sun Exposure")
	wind, _, _ := semanticFieldValue(obj, "Wind Exposure")
	rule := "not_checked"
	switch strings.ToLower(strings.TrimSpace(surface.OutsideBoundary)) {
	case "outdoors":
		if strings.EqualFold(sun, "SunExposed") && strings.EqualFold(wind, "WindExposed") {
			rule = "ok"
		} else {
			rule = "check_outdoors_exposure"
		}
	case "surface", "adiabatic", "ground":
		if strings.EqualFold(sun, "NoSun") && strings.EqualFold(wind, "NoWind") {
			rule = "ok"
		} else {
			rule = "check_non_outdoor_exposure"
		}
	}
	builder.rawForObject(indent, "validation:", surface.ObjectIndex, surface.Type, surface.Name)
	builder.kvForObject(indent+1, "exposure_rule", rule, surface.ObjectIndex, surface.Type, surface.Name)
}

func semanticBaseSurfaceResolution(ctx *semanticContext, window GeometryWindow) string {
	if window.BaseSurfaceName == "" {
		return "missing"
	}
	matches := 0
	for _, surface := range ctx.geometry.Surfaces {
		if strings.EqualFold(surface.Name, window.BaseSurfaceName) {
			matches++
		}
	}
	switch matches {
	case 0:
		return "missing"
	case 1:
		return "resolved"
	default:
		return "ambiguous"
	}
}

func semanticFieldByNames(builder *semanticYAMLBuilder, indent int, key string, obj Object, fallback string, names ...string) {
	if value, fieldIndex, ok := semanticFieldValue(obj, names...); ok {
		builder.fieldKV(indent, key, value, obj.Index, obj.Type, objectName(obj), fieldIndex)
		return
	}
	if strings.TrimSpace(fallback) != "" {
		builder.kvForObject(indent, key, fallback, obj.Index, obj.Type, objectName(obj))
	}
}

func semanticScheduleReferences(obj Object) []semanticScheduleUse {
	if strings.HasPrefix(strings.ToLower(obj.Type), "schedule:") {
		return nil
	}
	var refs []semanticScheduleUse
	for index, field := range obj.Fields {
		value := strings.TrimSpace(field.Value)
		if value == "" {
			continue
		}
		fieldName := catalogFieldName(obj, index)
		if fieldName == "" {
			fieldName = field.Comment
		}
		normalized := normalizeFieldName(fieldName)
		if strings.Contains(normalized, "schedule") && strings.Contains(normalized, "name") {
			refs = append(refs, semanticScheduleUse{
				ScheduleName: value,
				Field:        blankAs(fieldName, fmt.Sprintf("Field %d", index+1)),
				Role:         catalogFieldRole(obj, index),
			})
		}
	}
	return sortedSemanticScheduleUses(refs)
}

func semanticObjectReferencePath(obj Object) string {
	name := objectName(obj)
	zoneName, bucket, ok := semanticZoneAttachment(obj)
	if ok {
		section := "loads"
		if semanticAirExchangeBucket(bucket) {
			section = "air_exchange"
		}
		return "zones/" + blankAs(zoneName, "unresolved_zone") + "/" + section + "/" + bucket + "/" + blankAs(name, obj.Type)
	}
	if zoneName := semanticControlZone(obj); zoneName != "" {
		return "zones/" + zoneName + "/controls/" + semanticControlBucket(obj.Type) + "/" + blankAs(name, obj.Type)
	}
	if name != "" {
		return strings.ToLower(obj.Type) + "/" + name
	}
	return strings.ToLower(obj.Type) + "/object-" + fmt.Sprintf("%d", obj.Index)
}

func semanticObjectLabel(obj Object) string {
	name := objectName(obj)
	if name == "" {
		return obj.Type
	}
	return obj.Type + " " + name
}

func sortedSemanticScheduleUses(values []semanticScheduleUse) []semanticScheduleUse {
	seen := map[string]bool{}
	out := make([]semanticScheduleUse, 0, len(values))
	for _, value := range values {
		key := normalizeName(value.ScheduleName) + "|" + value.Path + "|" + value.Field + "|" + value.Object
		if seen[key] || strings.TrimSpace(value.ScheduleName) == "" {
			continue
		}
		seen[key] = true
		if value.Role == "" {
			value.Role = "schedule_ref"
		}
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Field < out[j].Field
	})
	return out
}

func semanticAirExchangeBucket(bucket string) bool {
	switch bucket {
	case "infiltration", "ventilation", "mixing", "cross_mixing":
		return true
	default:
		return false
	}
}

func semanticListMembers(obj Object) []string {
	var values []string
	for index := 1; index < len(obj.Fields); index++ {
		value := strings.TrimSpace(obj.Fields[index].Value)
		if value != "" {
			values = append(values, value)
		}
	}
	return sortedUniqueStrings(values)
}

func semanticSpaceFromObject(obj Object) semanticSpace {
	name := objectName(obj)
	zoneName, _, _ := semanticFieldValue(obj, "Zone Name")
	floorArea, _, _ := semanticFieldValue(obj, "Floor Area")
	volume, _, _ := semanticFieldValue(obj, "Volume")
	spaceType, _, _ := semanticFieldValue(obj, "Space Type")
	return semanticSpace{
		Name:        name,
		ZoneName:    zoneName,
		Source:      "idf_object",
		ObjectIndex: obj.Index,
		FloorArea:   floorArea,
		Volume:      volume,
		SpaceType:   spaceType,
	}
}

func semanticZoneTargets(ctx *semanticContext, target string) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	if members := ctx.zoneLists[normalizeName(target)]; len(members) > 0 {
		return members
	}
	return []string{target}
}

func semanticSpaceTargets(ctx *semanticContext, target string) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	if members := ctx.spaceLists[normalizeName(target)]; len(members) > 0 {
		return members
	}
	return []string{target}
}

func semanticFieldValue(obj Object, names ...string) (string, int, bool) {
	if field, index, ok := fieldByCatalogName(obj, names...); ok && strings.TrimSpace(field.Value) != "" {
		return strings.TrimSpace(field.Value), index, true
	}
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[normalizeFieldName(name)] = true
	}
	for index, field := range obj.Fields {
		if wanted[normalizeFieldName(field.Comment)] && strings.TrimSpace(field.Value) != "" {
			return strings.TrimSpace(field.Value), index, true
		}
	}
	for _, index := range semanticFallbackFieldIndexes(obj.Type, names...) {
		if index >= 0 && index < len(obj.Fields) && strings.TrimSpace(obj.Fields[index].Value) != "" {
			return strings.TrimSpace(obj.Fields[index].Value), index, true
		}
	}
	return "", -1, false
}

func semanticFallbackFieldIndexes(objectType string, names ...string) []int {
	lower := strings.ToLower(objectType)
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[normalizeFieldName(name)] = true
	}
	has := func(name string) bool { return wanted[normalizeFieldName(name)] }
	switch {
	case lower == "people":
		switch {
		case has("Zone or ZoneList Name"), has("Zone Name"):
			return []int{1}
		case has("Number of People Schedule Name"), has("Schedule Name"):
			return []int{2}
		case has("Number of People"), has("People"):
			return []int{4}
		case has("Activity Level Schedule Name"):
			return []int{6}
		}
	case lower == "lights":
		switch {
		case has("Zone or ZoneList Name"), has("Zone Name"):
			return []int{1}
		case has("Schedule Name"):
			return []int{2}
		case has("Lighting Level"), has("Design Level"):
			return []int{4}
		}
	case lower == "electricequipment" || lower == "gasequipment":
		switch {
		case has("Zone or ZoneList Name"), has("Zone Name"):
			return []int{1}
		case has("Schedule Name"):
			return []int{2}
		case has("Design Level"):
			return []int{4}
		}
	case strings.HasPrefix(lower, "zoneinfiltration:") || strings.HasPrefix(lower, "zoneventilation:") || strings.HasPrefix(lower, "zonemixing"):
		switch {
		case has("Zone or ZoneList Name"), has("Zone Name"):
			return []int{1}
		case has("Schedule Name"):
			return []int{2}
		case has("Design Flow Rate"):
			return []int{4}
		}
	case lower == "zonecontrol:thermostat":
		switch {
		case has("Zone Name"):
			return []int{1}
		case has("Control Type Schedule Name"), has("Schedule Name"):
			return []int{2}
		}
	}
	return nil
}

func semanticZoneAttachment(obj Object) (string, string, bool) {
	targetName, _, bucket, ok := semanticTargetAttachment(nil, obj)
	return targetName, bucket, ok
}

func semanticTargetAttachment(ctx *semanticContext, obj Object) (string, string, string, bool) {
	bucket := ""
	switch lower := strings.ToLower(obj.Type); {
	case lower == "people":
		bucket = "people"
	case lower == "lights":
		bucket = "lights"
	case lower == "electricequipment":
		bucket = "electric_equipment"
	case lower == "gasequipment":
		bucket = "gas_equipment"
	case lower == "hotwaterequipment":
		bucket = "hot_water_equipment"
	case lower == "steamequipment":
		bucket = "steam_equipment"
	case lower == "otherequipment":
		bucket = "other_equipment"
	case lower == "internalmass":
		bucket = "internal_mass"
	case lower == "zonecapacitancemultiplier:researchspecial":
		bucket = "thermal_capacitance"
	case lower == "zoneairmassflowconservation":
		bucket = "air_balance"
	case strings.HasPrefix(lower, "zoneinfiltration:"):
		bucket = "infiltration"
	case strings.HasPrefix(lower, "zoneventilation:"):
		bucket = "ventilation"
	case strings.HasPrefix(lower, "zonemixing"):
		bucket = "mixing"
	case strings.HasPrefix(lower, "zonecrossmixing"):
		bucket = "cross_mixing"
	default:
		return "", "", "", false
	}
	targetName, _, ok := semanticFieldValue(obj,
		"Zone or ZoneList or Space or SpaceList Name",
		"Zone or ZoneList Name",
		"Space or SpaceList Name",
		"Zone Name",
		"Space Name",
	)
	if !ok {
		return "", "", "", false
	}
	targetKind := "zone"
	if ctx != nil {
		switch {
		case len(ctx.spaceLists[normalizeName(targetName)]) > 0:
			targetKind = "space_list"
		case ctx.spaceByName[normalizeName(targetName)].Name != "":
			targetKind = "space"
		case len(ctx.zoneLists[normalizeName(targetName)]) > 0:
			targetKind = "zone_list"
		}
	}
	return targetName, targetKind, bucket, true
}

func semanticControlZone(obj Object) string {
	if !strings.HasPrefix(strings.ToLower(obj.Type), "zonecontrol:") {
		return ""
	}
	zoneName, _, _ := semanticFieldValue(obj, "Zone Name")
	return zoneName
}

func semanticControlBucket(objectType string) string {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	switch {
	case strings.Contains(lower, "thermostat") || strings.Contains(lower, "thermostatsetpoint"):
		return "thermostat"
	case strings.Contains(lower, "daylighting"):
		return "daylighting"
	case strings.Contains(lower, "humidistat"):
		return "humidistat"
	default:
		return "other"
	}
}

func semanticLoadLevel(obj Object) (string, string, int, bool) {
	switch strings.ToLower(obj.Type) {
	case "people":
		value, index, ok := semanticFieldValue(obj, "Number of People", "People")
		if ok {
			return strings.TrimSpace(value + " persons"), value, index, true
		}
	case "lights":
		value, index, ok := semanticFieldValue(obj, "Lighting Level", "Design Level")
		if ok {
			return strings.TrimSpace(value + " W"), value, index, true
		}
	case "electricequipment", "gasequipment":
		value, index, ok := semanticFieldValue(obj, "Design Level")
		if ok {
			return strings.TrimSpace(value + " W"), value, index, true
		}
	default:
		value, index, ok := semanticFieldValue(obj, "Design Flow Rate")
		if ok {
			return strings.TrimSpace(value + " m3/s"), value, index, true
		}
	}
	return "", "", -1, false
}

func semanticVertices(points []GeometryPoint) string {
	if len(points) == 0 {
		return "[]"
	}
	values := make([]string, 0, len(points))
	for _, point := range points {
		values = append(values, fmt.Sprintf("[%s,%s,%s]", semanticNumber(point.X), semanticNumber(point.Y), semanticNumber(point.Z)))
	}
	return "[" + strings.Join(values, ", ") + "]"
}

func semanticNumber(value float64) string {
	text := fmt.Sprintf("%.4f", value)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "-0" || text == "" {
		return "0"
	}
	return text
}

func semanticQuantity(value float64, unit string) string {
	return strings.TrimSpace(semanticNumber(value) + " " + unit)
}

func sortedUniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func semanticOutputObjectExists(report OutputReport, objectType string) bool {
	for _, item := range report.Existing {
		if strings.EqualFold(item.ObjectType, objectType) {
			return true
		}
	}
	return false
}

func semanticOutputControlFilesEnabled(doc Document, fileKind string) bool {
	enabled, _ := semanticOutputControlFilesState(doc, fileKind)
	return enabled
}

func semanticOutputControlFilesState(doc Document, fileKind string) (bool, string) {
	for _, obj := range doc.Objects {
		if !strings.EqualFold(obj.Type, "OutputControl:Files") {
			continue
		}
		wanted := normalizeFieldName("Output " + fileKind)
		for index, field := range obj.Fields {
			name := normalizeFieldName(catalogFieldName(obj, index))
			if name == "" {
				name = normalizeFieldName(field.Comment)
			}
			if name != wanted && !strings.Contains(name, normalizeFieldName(fileKind)) {
				continue
			}
			return semanticYesNoValue(field.Value), "explicit"
		}
	}
	return semanticOutputControlFilesDefault(fileKind), "default"
}

func semanticOutputControlFilesDefault(fileKind string) bool {
	switch strings.ToLower(strings.TrimSpace(fileKind)) {
	case "csv":
		return false
	case "mtr", "eso", "eio", "tabular", "sqlite", "json":
		return true
	default:
		return false
	}
}

func semanticYesNoValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "1", "on":
		return true
	default:
		return false
	}
}

func semanticOutputTargetExists(ctx *semanticContext, keyValue string) bool {
	key := normalizeName(keyValue)
	if key == "" || key == "*" {
		return true
	}
	for _, obj := range ctx.doc.Objects {
		if normalizeName(objectName(obj)) == key {
			return true
		}
	}
	return false
}

func semanticOutputAttachmentResolution(ctx *semanticContext, output OutputObjectSummary) string {
	key := strings.TrimSpace(output.KeyValue)
	if key == "" || key == "*" {
		if key == "*" {
			return semanticOutputWildcardFamily(output)
		}
		return "wildcard"
	}
	if strings.EqualFold(key, "Environment") || strings.EqualFold(key, "Environment:*") {
		return "environment"
	}
	if ctx != nil && semanticOutputTargetExists(ctx, key) {
		return "resolved"
	}
	return "unresolved_after_rdd"
}

func semanticOutputWildcardFamily(output OutputObjectSummary) string {
	name := strings.ToLower(strings.TrimSpace(output.VariableName))
	switch {
	case strings.Contains(name, "zone "):
		return "zone_wildcard"
	case strings.Contains(name, "system node") || strings.Contains(name, " node "):
		return "node_wildcard"
	case strings.Contains(name, "surface"):
		return "surface_wildcard"
	case strings.Contains(name, "equipment"):
		return "equipment_wildcard"
	default:
		return "unknown_wildcard"
	}
}

func semanticOutputLabel(output OutputObjectSummary, includeTarget bool) string {
	frequency := blankAs(output.ReportingFrequency, defaultOutputFrequency)
	switch strings.ToLower(output.ObjectType) {
	case "output:variable":
		if includeTarget && strings.TrimSpace(output.KeyValue) != "" && strings.TrimSpace(output.KeyValue) != "*" {
			return fmt.Sprintf("[%s] %s :: %s", frequency, output.KeyValue, output.VariableName)
		}
		return fmt.Sprintf("[%s] %s", frequency, output.VariableName)
	case "output:meter", "output:meter:meterfileonly":
		return fmt.Sprintf("[%s] %s", frequency, output.KeyValue)
	default:
		return output.Summary
	}
}

func writeSemanticHeatFlowOutputGroup(builder *semanticYAMLBuilder, indent int, variables []OutputObjectSummary) {
	var heatFlow []OutputObjectSummary
	for _, item := range variables {
		if strings.Contains(strings.ToLower(item.VariableName), "zone air heat balance") {
			heatFlow = append(heatFlow, item)
		}
	}
	if len(heatFlow) == 0 {
		return
	}
	builder.raw(indent, "groups:")
	builder.raw(indent+1, "heat_flow_ledger:")
	builder.kv(indent+2, "frequency", standardHeatFlowFrequency)
	completeness, missing, consistent := semanticHeatFlowLedgerCompleteness(heatFlow)
	builder.kv(indent+2, "completeness", completeness)
	builder.kv(indent+2, "frequency_consistent", fmt.Sprintf("%t", consistent))
	if len(missing) > 0 {
		builder.raw(indent+2, "missing_variables:")
		for _, variable := range missing {
			builder.raw(indent+3, "- "+yamlScalar(variable))
		}
	}
	writeSemanticOutputList(builder, indent+2, "variables", heatFlow, false)
}

func semanticHeatFlowLedgerCompleteness(variables []OutputObjectSummary) (string, []string, bool) {
	required := []string{
		"Zone Air Heat Balance Internal Convective Heat Gain Rate",
		"Zone Air Heat Balance Surface Convection Rate",
		"Zone Air Heat Balance Interzone Air Transfer Rate",
		"Zone Air Heat Balance Outdoor Air Transfer Rate",
		"Zone Air Heat Balance System Air Transfer Rate",
		"Zone Air Heat Balance System Convective Heat Gain Rate",
		"Zone Air Heat Balance Air Energy Storage Rate",
		"Zone Air Heat Balance Deviation Rate",
	}
	seen := map[string]bool{}
	consistent := true
	for _, variable := range variables {
		seen[normalizeName(variable.VariableName)] = true
		if variable.ReportingFrequency != "" && !strings.EqualFold(variable.ReportingFrequency, standardHeatFlowFrequency) {
			consistent = false
		}
	}
	var missing []string
	for _, variable := range required {
		if !seen[normalizeName(variable)] {
			missing = append(missing, variable)
		}
	}
	if len(missing) == 0 && consistent {
		return "complete", nil, true
	}
	if len(missing) < len(required) {
		return "partial", missing, consistent
	}
	return "incomplete", missing, consistent
}

func semanticSectionForType(objectType string) string {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	switch {
	case lower == "version" ||
		lower == "simulationcontrol" ||
		lower == "timestep" ||
		strings.Contains(lower, "algorithm"):
		return "simulation"
	case strings.HasPrefix(lower, "site:") ||
		strings.HasPrefix(lower, "sizingperiod:") ||
		lower == "runperiod":
		return "site"
	case lower == "building" ||
		lower == "globalgeometryrules" ||
		strings.HasPrefix(lower, "shading:"):
		return "building"
	case strings.HasPrefix(lower, "schedule:"):
		return "schedules"
	case lower == "construction" ||
		strings.HasPrefix(lower, "construction:") ||
		strings.HasPrefix(lower, "material") ||
		strings.HasPrefix(lower, "windowmaterial"):
		return "constructions"
	case lower == "zone" ||
		lower == "space" ||
		strings.Contains(lower, "surface") ||
		strings.HasPrefix(lower, "wall:") ||
		lower == "window" ||
		lower == "door" ||
		lower == "people" ||
		lower == "lights" ||
		strings.Contains(lower, "equipment") ||
		lower == "internalmass" ||
		strings.HasPrefix(lower, "designspecification:outdoorair") ||
		strings.HasPrefix(lower, "zoneinfiltration:") ||
		strings.HasPrefix(lower, "zoneventilation:") ||
		strings.HasPrefix(lower, "zonecontrol:") ||
		strings.HasPrefix(lower, "thermostatsetpoint:"):
		return "zones"
	case strings.Contains(lower, "hvac") ||
		strings.HasPrefix(lower, "airloop") ||
		strings.HasPrefix(lower, "plantloop") ||
		strings.HasPrefix(lower, "condenserloop") ||
		strings.HasPrefix(lower, "branch") ||
		strings.HasPrefix(lower, "connector") ||
		strings.HasPrefix(lower, "node") ||
		strings.HasPrefix(lower, "coil:") ||
		strings.HasPrefix(lower, "fan:") ||
		strings.HasPrefix(lower, "pump:") ||
		strings.HasPrefix(lower, "boiler:") ||
		strings.HasPrefix(lower, "chiller:") ||
		strings.HasPrefix(lower, "controller:") ||
		strings.HasPrefix(lower, "setpointmanager:") ||
		strings.HasPrefix(lower, "pipe:"):
		return "hvac"
	case strings.HasPrefix(lower, "airflownetwork:"):
		return "airflows"
	case strings.HasPrefix(lower, "output:") ||
		strings.HasPrefix(lower, "outputcontrol:") ||
		strings.HasPrefix(lower, "meter:"):
		return "outputs"
	default:
		return "miscellaneous"
	}
}

func semanticMiscReason(objectType string) string {
	section := semanticSuggestedSection(objectType)
	switch section {
	case "":
		return "unmapped_object_type"
	case "miscellaneous":
		return "raw_preservation_only"
	default:
		return "known_type_not_projected_yet"
	}
}

func semanticSuggestedSection(objectType string) string {
	section := semanticSectionForType(objectType)
	if section == "miscellaneous" {
		return ""
	}
	return section
}

func semanticSourceNameConflicts(doc Document) []SemanticSourceNameConflict {
	type item struct {
		objectType string
		name       string
		indexes    []int
	}
	byKey := map[string]*item{}
	for _, obj := range doc.Objects {
		name := objectName(obj)
		if name == "" {
			continue
		}
		key := normalizeName(obj.Type) + "/" + normalizeName(name)
		if byKey[key] == nil {
			byKey[key] = &item{objectType: obj.Type, name: name}
		}
		byKey[key].indexes = append(byKey[key].indexes, obj.Index)
	}
	var groups []SemanticSourceNameConflict
	for _, item := range byKey {
		if len(item.indexes) < 2 {
			continue
		}
		sort.Ints(item.indexes)
		groups = append(groups, SemanticSourceNameConflict{
			Group:         semanticDuplicateGroupID(item.objectType, item.name),
			ObjectType:    item.objectType,
			Name:          item.name,
			ObjectIndexes: append([]int(nil), item.indexes...),
			SyncPolicy:    "rename_later_duplicates",
			AutoFixable:   true,
		})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].ObjectType != groups[j].ObjectType {
			return strings.ToLower(groups[i].ObjectType) < strings.ToLower(groups[j].ObjectType)
		}
		return strings.ToLower(groups[i].Name) < strings.ToLower(groups[j].Name)
	})
	return groups
}

func semanticDuplicateGroupID(objectType string, name string) string {
	parts := strings.Fields(strings.ToLower(objectType + " " + name))
	raw := strings.Join(parts, "-")
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	value := strings.Trim(b.String(), "-")
	if value == "" {
		return "duplicate-object"
	}
	return "duplicate-" + value
}

func semanticFieldKey(field Field, fieldIndex int) string {
	key := strings.TrimSpace(field.Comment)
	if key == "" {
		key = fmt.Sprintf("field_%d", fieldIndex+1)
	}
	key = strings.TrimSpace(strings.Split(key, "{")[0])
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(key) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return fmt.Sprintf("field_%d", fieldIndex+1)
	}
	return out
}

func yamlScalar(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "null"
	}
	lower := strings.ToLower(value)
	if lower == "true" || lower == "false" {
		return lower
	}
	if lower == "yes" || lower == "no" || lower == "on" || lower == "off" || lower == "null" {
		return quoteYAMLString(value)
	}
	if strings.ContainsAny(value, ",:[]{}#*!|>&%@`\"'") || strings.Contains(value, "  ") {
		return quoteYAMLString(value)
	}
	return value
}

func quoteYAMLString(value string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

func blankAs(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func intPtr(value int) *int {
	return &value
}
