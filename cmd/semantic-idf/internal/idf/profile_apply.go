package idf

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type ProfileApplyRequest struct {
	SourceObjectIndexes   []int    `json:"sourceObjectIndexes"`
	SourceZoneNames       []string `json:"sourceZoneNames,omitempty"`
	TargetZoneNames       []string `json:"targetZoneNames"`
	Dimensions            []string `json:"dimensions,omitempty"`
	Mode                  string   `json:"mode"`
	ReplaceExistingPolicy string   `json:"replaceExistingPolicy"`
	NameSuffix            string   `json:"nameSuffix"`
	AllowZoneListEdit     bool     `json:"allowZoneListEdit"`
}

type ProfileApplyPreview struct {
	CanApply bool                 `json:"canApply"`
	Mode     string               `json:"mode"`
	Changes  []ProfileApplyChange `json:"changes"`
	Warnings []ProfileWarning     `json:"warnings,omitempty"`
}

type ProfileApplyChange struct {
	Action            string `json:"action"`
	Dimension         string `json:"dimension,omitempty"`
	SourceObjectIndex int    `json:"sourceObjectIndex,omitempty"`
	ObjectIndex       int    `json:"objectIndex,omitempty"`
	ObjectType        string `json:"objectType,omitempty"`
	ObjectName        string `json:"objectName,omitempty"`
	TargetZoneName    string `json:"targetZoneName,omitempty"`
	ZoneListName      string `json:"zoneListName,omitempty"`
	Message           string `json:"message"`
}

func PreviewApplyProfile(doc Document, request ProfileApplyRequest) ProfileApplyPreview {
	_, preview := applyProfile(doc, request, false)
	return preview
}

func ApplyProfile(doc Document, request ProfileApplyRequest) (Document, ProfileApplyPreview) {
	return applyProfile(doc, request, true)
}

func applyProfile(doc Document, request ProfileApplyRequest, mutate bool) (Document, ProfileApplyPreview) {
	request = normalizeProfileApplyRequest(request)
	updated := doc.clone()
	preview := ProfileApplyPreview{CanApply: true, Mode: request.Mode}
	if len(request.SourceObjectIndexes) == 0 {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, ProfileWarning{Severity: "warning", Code: "missing_source_profile", Message: "No source profile objects were selected."})
		return updated, preview
	}
	if len(request.TargetZoneNames) == 0 {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, ProfileWarning{Severity: "warning", Code: "missing_target_zone", Message: "Select at least one target zone."})
		return updated, preview
	}

	if request.Mode == "shared" {
		applySharedProfile(&updated, request, mutate, &preview)
	} else {
		applyClonedProfile(&updated, request, mutate, &preview)
	}
	if mutate {
		reindexObjects(&updated)
	}
	return updated, preview
}

func normalizeProfileApplyRequest(request ProfileApplyRequest) ProfileApplyRequest {
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	if request.Mode != "shared" {
		request.Mode = "clone"
	}
	request.ReplaceExistingPolicy = strings.ToLower(strings.TrimSpace(request.ReplaceExistingPolicy))
	switch request.ReplaceExistingPolicy {
	case "keep", "replace", "duplicate":
	default:
		request.ReplaceExistingPolicy = "replace"
	}
	if strings.TrimSpace(request.NameSuffix) == "" {
		request.NameSuffix = " Profile Copy"
	}
	request.SourceObjectIndexes = uniqueInts(request.SourceObjectIndexes)
	request.TargetZoneNames = cleanProfileNames(request.TargetZoneNames)
	request.Dimensions = cleanProfileNames(request.Dimensions)
	return request
}

func applyClonedProfile(doc *Document, request ProfileApplyRequest, mutate bool, preview *ProfileApplyPreview) {
	sourceObjects := sourceProfileObjects(*doc, request)
	existingNames := existingObjectNames(*doc)
	removeIndexes := map[int]bool{}

	if request.ReplaceExistingPolicy == "replace" {
		for _, targetZone := range request.TargetZoneNames {
			for _, existing := range directProfileObjectsForZone(*doc, targetZone, request.Dimensions) {
				removeIndexes[existing.Index] = true
				preview.Changes = append(preview.Changes, ProfileApplyChange{
					Action:         "replace",
					Dimension:      profileDimensionForObject(existing.Type),
					ObjectIndex:    existing.Index,
					ObjectType:     existing.Type,
					ObjectName:     objectName(existing),
					TargetZoneName: targetZone,
					Message:        fmt.Sprintf("Replace existing %s in %s.", existing.Type, targetZone),
				})
			}
		}
	}

	if mutate && len(removeIndexes) > 0 {
		var kept []Object
		for _, obj := range doc.Objects {
			if !removeIndexes[obj.Index] {
				kept = append(kept, obj)
			}
		}
		doc.Objects = kept
	}

	for _, targetZone := range request.TargetZoneNames {
		for _, source := range sourceObjects {
			dimension := profileDimensionForObject(source.Type)
			if request.ReplaceExistingPolicy == "keep" && len(directProfileObjectsForZone(*doc, targetZone, []string{dimension})) > 0 {
				preview.Changes = append(preview.Changes, ProfileApplyChange{
					Action:            "keep",
					Dimension:         dimension,
					SourceObjectIndex: source.Index,
					ObjectType:        source.Type,
					ObjectName:        objectName(source),
					TargetZoneName:    targetZone,
					Message:           fmt.Sprintf("Keep existing %s profile in %s; source object is not cloned.", profileDimensionLabel(dimension), targetZone),
				})
				continue
			}
			clone := cloneProfileObjectForZone(source, targetZone, request.NameSuffix, existingNames)
			preview.Changes = append(preview.Changes, ProfileApplyChange{
				Action:            "clone",
				Dimension:         dimension,
				SourceObjectIndex: source.Index,
				ObjectType:        clone.Type,
				ObjectName:        objectName(clone),
				TargetZoneName:    targetZone,
				Message:           fmt.Sprintf("Clone %s as %q for %s.", source.Type, objectName(clone), targetZone),
			})
			if mutate {
				doc.Objects = append(doc.Objects, clone)
				existingNames[normalizeName(objectName(clone))] = true
			}
		}
	}
}

func applySharedProfile(doc *Document, request ProfileApplyRequest, mutate bool, preview *ProfileApplyPreview) {
	if !request.AllowZoneListEdit {
		preview.CanApply = false
		preview.Warnings = append(preview.Warnings, ProfileWarning{Severity: "warning", Code: "zonelist_edit_disabled", Message: "Shared mode requires ZoneList editing to be enabled."})
		return
	}
	sourceObjects := sourceProfileObjects(*doc, request)
	zoneListIndexes := map[int]bool{}
	for _, source := range sourceObjects {
		target := profileTargetName(source)
		zoneListIndex := findNamedObjectIndex(*doc, "ZoneList", target)
		if zoneListIndex < 0 {
			preview.Warnings = append(preview.Warnings, profileWarning("warning", "shared_mode_needs_zonelist", "Source profile object does not reference a ZoneList; use clone mode for this object.", "", profileDimensionForObject(source.Type), source))
			continue
		}
		zoneListIndexes[zoneListIndex] = true
		preview.Changes = append(preview.Changes, ProfileApplyChange{
			Action:            "shared_zone_list",
			Dimension:         profileDimensionForObject(source.Type),
			SourceObjectIndex: source.Index,
			ObjectType:        source.Type,
			ObjectName:        objectName(source),
			ZoneListName:      target,
			Message:           fmt.Sprintf("Add target zones to shared ZoneList %q used by %s.", target, objectName(source)),
		})
	}
	for zoneListIndex := range zoneListIndexes {
		for _, targetZone := range request.TargetZoneNames {
			if zoneListHasZone(doc.Objects[zoneListIndex], targetZone) {
				continue
			}
			preview.Changes = append(preview.Changes, ProfileApplyChange{
				Action:         "add_zone_to_zonelist",
				ObjectIndex:    doc.Objects[zoneListIndex].Index,
				ObjectType:     doc.Objects[zoneListIndex].Type,
				ObjectName:     objectName(doc.Objects[zoneListIndex]),
				TargetZoneName: targetZone,
				ZoneListName:   objectName(doc.Objects[zoneListIndex]),
				Message:        fmt.Sprintf("Add %s to ZoneList %q.", targetZone, objectName(doc.Objects[zoneListIndex])),
			})
			if mutate {
				doc.Objects[zoneListIndex].Fields = append(doc.Objects[zoneListIndex].Fields, Field{
					Value:   targetZone,
					Comment: fmt.Sprintf("Zone %d Name", len(doc.Objects[zoneListIndex].Fields)),
				})
			}
		}
		for _, impacted := range objectsReferencingTarget(*doc, objectName(doc.Objects[zoneListIndex])) {
			preview.Changes = append(preview.Changes, ProfileApplyChange{
				Action:       "impact",
				Dimension:    profileDimensionForObject(impacted.Type),
				ObjectIndex:  impacted.Index,
				ObjectType:   impacted.Type,
				ObjectName:   objectName(impacted),
				ZoneListName: objectName(doc.Objects[zoneListIndex]),
				Message:      fmt.Sprintf("%s also references shared ZoneList %q.", objectName(impacted), objectName(doc.Objects[zoneListIndex])),
			})
		}
	}
}

func sourceProfileObjects(doc Document, request ProfileApplyRequest) []Object {
	indexes := map[int]bool{}
	for _, index := range request.SourceObjectIndexes {
		indexes[index] = true
	}
	dimensions := stringSet(request.Dimensions)
	var out []Object
	for _, obj := range doc.Objects {
		if !indexes[obj.Index] {
			continue
		}
		dimension := profileDimensionForObject(obj.Type)
		if dimension == "" || strings.EqualFold(obj.Type, "DesignSpecification:OutdoorAir") {
			continue
		}
		if len(dimensions) > 0 && !dimensions[dimension] {
			continue
		}
		out = append(out, obj)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func directProfileObjectsForZone(doc Document, zoneName string, dimensions []string) []Object {
	dimensionSet := stringSet(dimensions)
	var out []Object
	for _, obj := range doc.Objects {
		dimension := profileDimensionForObject(obj.Type)
		if dimension == "" || strings.EqualFold(obj.Type, "DesignSpecification:OutdoorAir") {
			continue
		}
		if len(dimensionSet) > 0 && !dimensionSet[dimension] {
			continue
		}
		target := profileTargetName(obj)
		if strings.EqualFold(strings.TrimSpace(target), strings.TrimSpace(zoneName)) {
			out = append(out, obj)
		}
	}
	return out
}

func cloneProfileObjectForZone(source Object, targetZone string, suffix string, existingNames map[string]bool) Object {
	clone := source
	clone.Index = 0
	clone.Fields = append([]Field(nil), source.Fields...)
	if len(clone.Fields) > 0 && !isNamelessType(clone.Type) {
		baseName := strings.TrimSpace(clone.Fields[0].Value)
		if baseName == "" {
			baseName = clone.Type
		}
		clone.Fields[0].Value = uniqueProfileObjectName(fmt.Sprintf("%s%s %s", baseName, suffix, targetZone), existingNames)
	}
	if targetIndex := profileTargetFieldIndex(clone); targetIndex >= 0 {
		clone.Fields[targetIndex].Value = targetZone
	}
	return clone
}

func profileTargetFieldIndex(obj Object) int {
	for index, field := range obj.Fields {
		comment := strings.ToLower(field.Comment)
		if strings.Contains(comment, "zone") && strings.Contains(comment, "name") {
			return index
		}
		if strings.Contains(comment, "zone") && strings.Contains(comment, "zonelist") && strings.Contains(comment, "name") {
			return index
		}
		if strings.Contains(comment, "space") && strings.Contains(comment, "name") {
			return index
		}
		if strings.Contains(comment, "space") && strings.Contains(comment, "spacelist") && strings.Contains(comment, "name") {
			return index
		}
	}
	return -1
}

func uniqueProfileObjectName(base string, existing map[string]bool) string {
	name := strings.TrimSpace(base)
	if name == "" {
		name = "Profile Object"
	}
	candidate := name
	for index := 2; existing[normalizeName(candidate)]; index++ {
		candidate = name + " " + strconv.Itoa(index)
	}
	existing[normalizeName(candidate)] = true
	return candidate
}

func existingObjectNames(doc Document) map[string]bool {
	out := map[string]bool{}
	for _, obj := range doc.Objects {
		if name := objectName(obj); name != "" {
			out[normalizeName(name)] = true
		}
	}
	return out
}

func findNamedObjectIndex(doc Document, objectType string, name string) int {
	for index, obj := range doc.Objects {
		if strings.EqualFold(obj.Type, objectType) && strings.EqualFold(objectName(obj), name) {
			return index
		}
	}
	return -1
}

func zoneListHasZone(obj Object, zoneName string) bool {
	for _, field := range obj.Fields[1:] {
		if strings.EqualFold(strings.TrimSpace(field.Value), strings.TrimSpace(zoneName)) {
			return true
		}
	}
	return false
}

func objectsReferencingTarget(doc Document, target string) []Object {
	var out []Object
	for _, obj := range doc.Objects {
		if profileDimensionForObject(obj.Type) == "" {
			continue
		}
		if strings.EqualFold(profileTargetName(obj), target) {
			out = append(out, obj)
		}
	}
	return out
}

func reindexObjects(doc *Document) {
	for index := range doc.Objects {
		doc.Objects[index].Index = index
	}
}

func uniqueInts(values []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, value := range values {
		if value < 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}
