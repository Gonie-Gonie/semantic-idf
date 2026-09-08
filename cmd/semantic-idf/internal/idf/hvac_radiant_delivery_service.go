package idf

import "strings"

type hvacRadiantPlantBinding struct {
	ServiceKind  string
	PlantLoop    LoopRef
	SourceLabels []string
}

type hvacRadiantDeliveryServices struct {
	ZoneName       string
	EquipmentIndex int
	Bindings       []hvacRadiantPlantBinding
}

// ConstantFlow is one physical delivery on two different water circuits. The
// catalog's Design-object schema supplies separate H/C ports; neither its name
// nor the union of connected loops establishes which service a loop supplies.
func buildHVACRadiantDeliveryServices(ctx *hvacContext, loops []HVACLoop, relations []HVACZoneChain) map[string]hvacRadiantDeliveryServices {
	const kind = "ZoneHVAC:LowTemperatureRadiant:ConstantFlow"
	parents := ctx.objectsByType[normalizeFieldCatalogKey(kind)]
	if len(parents) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, object := range ctx.doc.Objects {
		counts[hvacObjectKey(object.Type, objectName(object))]++
	}
	owners := map[string][]string{}
	for _, relation := range relations {
		for _, equipment := range relation.ZoneEquipment {
			if strings.EqualFold(equipment.ObjectType, kind) {
				key := hvacComponentKey(equipment)
				owners[key] = append(owners[key], relation.ZoneName)
			}
		}
	}
	out := map[string]hvacRadiantDeliveryServices{}
	for _, parent := range parents {
		key := hvacObjectKey(parent.Type, objectName(parent))
		out[key] = hvacRadiantDeliveryServices{} // Unsupported/malformed native input must not fall through to Cartesian inference.
		zoneName, valid := hvacRadiantDeliveryOwner(ctx, parent, counts, owners[key])
		if !valid {
			continue
		}
		branches, valid := hvacRadiantDeliveryBranches(ctx, parent, counts, zoneName)
		if !valid {
			continue
		}
		bindings, valid := hvacRadiantBranchPlants(ctx, loops, parent, counts, branches)
		if valid {
			out[key] = hvacRadiantDeliveryServices{ZoneName: zoneName, EquipmentIndex: parent.Index, Bindings: bindings}
		}
	}
	return out
}

func hvacRadiantDeliveryOwner(ctx *hvacContext, parent Object, counts map[string]int, owners []string) (string, bool) {
	key := hvacObjectKey(parent.Type, objectName(parent))
	zone := fieldValueByCatalogName(parent, "Zone Name")
	design := fieldValueByCatalogName(parent, "Design Object")
	surfaceName := fieldValueByCatalogName(parent, "Surface Name or Radiant Surface Group Name")
	surfaceKey := hvacObjectKey("BuildingSurface:Detailed", surfaceName)
	if len(parent.Fields) < 22 || counts[key] != 1 || len(owners) != 1 || !strings.EqualFold(zone, owners[0]) || zone == "" ||
		counts[hvacObjectKey("Zone", zone)] != 1 || design == "" || counts[hvacObjectKey(parent.Type+":Design", design)] != 1 ||
		len(ctx.objectsByTypeName[hvacObjectKey(parent.Type+":Design", design)].Fields) < 2 || surfaceName == "" || counts[surfaceKey] != 1 ||
		counts[hvacObjectKey("ZoneHVAC:LowTemperatureRadiant:SurfaceGroup", surfaceName)] != 0 {
		return "", false
	}
	surface := ctx.objectsByTypeName[surfaceKey]
	if !strings.EqualFold(hvacFieldValue(surface, 3), zone) {
		return "", false
	}
	if spaceName := hvacFieldValue(surface, 4); spaceName != "" {
		spaceKey := hvacObjectKey("Space", spaceName)
		if counts[spaceKey] != 1 || !strings.EqualFold(hvacFieldValue(ctx.objectsByTypeName[spaceKey], 1), zone) {
			return "", false
		}
	}
	// A second emitter or a surface group cannot silently share this direct
	// surface. This does not infer services for those unsupported variants.
	for _, object := range ctx.doc.Objects {
		if object.Index == parent.Index {
			continue
		}
		surfaceField := -1
		switch normalizeFieldCatalogKey(object.Type) {
		case "zonehvac:lowtemperatureradiant:constantflow", "zonehvac:lowtemperatureradiant:variableflow":
			surfaceField = 4
		case "zonehvac:lowtemperatureradiant:electric":
			surfaceField = 3
		case "zonehvac:lowtemperatureradiant:surfacegroup":
			for index := 1; index < len(object.Fields); index += 2 {
				if strings.EqualFold(hvacFieldValue(object, index), surfaceName) {
					return "", false
				}
			}
		}
		if surfaceField >= 0 && (strings.EqualFold(objectName(object), objectName(parent)) || strings.EqualFold(hvacFieldValue(object, surfaceField), surfaceName)) {
			return "", false
		}
	}
	return zone, true
}

func hvacRadiantDeliveryBranches(ctx *hvacContext, parent Object, counts map[string]int, zone string) (map[string]string, bool) {
	ports := map[string]string{}
	seenNodes := map[string]bool{}
	for _, role := range []struct{ service, label string }{{"cooling", "Cooling"}, {"heating", "Heating"}} {
		inlet := fieldValueByCatalogName(parent, role.label+" Water Inlet Node Name")
		outlet := fieldValueByCatalogName(parent, role.label+" Water Outlet Node Name")
		for _, node := range []string{inlet, outlet} {
			key := normalizeName(node)
			if key == "" || seenNodes[key] {
				return nil, false
			}
			seenNodes[key] = true
		}
		ports[normalizeName(inlet)+"|"+normalizeName(outlet)] = role.service
	}
	key := hvacObjectKey(parent.Type, objectName(parent))
	branches, services := map[string]string{}, map[string]bool{}
	listName, listCount := "", 0
	for _, object := range ctx.doc.Objects {
		for field := 0; field+1 < len(object.Fields); field++ {
			if hvacObjectKey(hvacFieldValue(object, field), hvacFieldValue(object, field+1)) != key {
				continue
			}
			switch {
			case strings.EqualFold(object.Type, "ZoneHVAC:EquipmentList") && field >= 2 && (field-2)%6 == 0:
				listName, listCount = objectName(object), listCount+1
			case strings.EqualFold(object.Type, "Branch") && field >= 2 && (field-2)%4 == 0:
				pair := normalizeName(hvacFieldValue(object, field+2)) + "|" + normalizeName(hvacFieldValue(object, field+3))
				service := ports[pair]
				branchKey := hvacObjectKey("Branch", objectName(object))
				if service == "" || services[service] || counts[branchKey] != 1 {
					return nil, false
				}
				services[service], branches[branchKey] = true, service
			default:
				return nil, false
			}
		}
	}
	if listCount != 1 || counts[hvacObjectKey("ZoneHVAC:EquipmentList", listName)] != 1 || len(branches) != 2 {
		return nil, false
	}
	zoneConnections, listConnections := 0, 0
	for _, connection := range ctx.doc.Objects {
		if !strings.EqualFold(connection.Type, "ZoneHVAC:EquipmentConnections") {
			continue
		}
		if strings.EqualFold(hvacFieldValue(connection, 0), zone) {
			zoneConnections++
		}
		if strings.EqualFold(hvacFieldValue(connection, 1), listName) {
			listConnections++
			if !strings.EqualFold(hvacFieldValue(connection, 0), zone) {
				return nil, false
			}
		}
	}
	return branches, zoneConnections == 1 && listConnections == 1
}

func hvacRadiantBranchPlants(ctx *hvacContext, loops []HVACLoop, parent Object, counts map[string]int, branches map[string]string) ([]hvacRadiantPlantBinding, bool) {
	var bindings []hvacRadiantPlantBinding
	seen := map[string]bool{}
	for _, loop := range loops {
		for _, side := range []HVACLoopSide{loop.SupplySide, loop.DemandSide} {
			for _, branch := range side.Branches {
				for _, component := range branch.Components {
					if hvacComponentKey(component) != hvacObjectKey(parent.Type, objectName(parent)) {
						continue
					}
					branchKey := hvacObjectKey("Branch", branch.Name)
					service := branches[branchKey]
					if service == "" || seen[branchKey] || !component.Exists || component.ObjectIndex != parent.Index ||
						!strings.EqualFold(loop.Type, "PlantLoop") || !strings.EqualFold(side.Name, "demand") ||
						counts[hvacObjectKey(loop.Type, loop.Name)] != 1 || counts[hvacObjectKey("BranchList", side.BranchListName)] != 1 {
						return nil, false
					}
					seen[branchKey] = true
					plant := loopRefFromLoop(loop)
					bindings = append(bindings, hvacRadiantPlantBinding{ServiceKind: service, PlantLoop: plant, SourceLabels: hvacHydronicPlantSourceLabels(loops, counts, plant)})
				}
			}
		}
	}
	return bindings, len(seen) == len(branches)
}

func (entry hvacRadiantDeliveryServices) bindPath(path ZoneServicePath) (ZoneServicePath, bool) {
	if path.PathType != "radiant" || path.PlantLoop == nil || path.AirLoop != nil || path.CondenserLoop != nil || path.RefrigerantSystem != nil ||
		path.Delivery.ObjectIndex != entry.EquipmentIndex || !strings.EqualFold(path.ZoneName, entry.ZoneName) || len(path.Conditioning) != 0 || path.DeliveryWrapper != nil {
		return path, false
	}
	for _, binding := range entry.Bindings {
		if path.ServiceKind != binding.ServiceKind || !strings.EqualFold(path.PlantLoop.Type, binding.PlantLoop.Type) || !strings.EqualFold(path.PlantLoop.Name, binding.PlantLoop.Name) {
			continue
		}
		if source := path.SourceSystem; source != nil {
			valid := false
			for _, label := range binding.SourceLabels {
				valid = valid || source.Type == "source" && strings.EqualFold(source.Name, label)
			}
			if !valid {
				return path, false
			}
		}
		return path, true
	}
	return path, false
}
