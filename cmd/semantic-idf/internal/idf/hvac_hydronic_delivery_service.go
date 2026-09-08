package idf

import "strings"

type hvacHydronicDeliveryBinding struct {
	ServiceKind  string
	PlantLoop    LoopRef
	Coil         ComponentRef
	SourceLabels []string
}

type hvacHydronicDeliveryServices struct {
	Bindings      []hvacHydronicDeliveryBinding
	LocalServices map[string]bool
	LocalCoils    map[string]ComponentRef
}

// A native fan coil's two water coils can belong to different plants. Its
// delivery-level service union must not be multiplied by its plant-loop union.
// Keep incomplete/legacy shapes without native coil fields on the existing
// path, and do no extra indexing for models without this equipment type.
func buildHVACHydronicDeliveryServices(ctx *hvacContext, loops []HVACLoop, relations []HVACZoneChain) map[string]hvacHydronicDeliveryServices {
	if len(ctx.objectsByType[normalizeFieldCatalogKey("ZoneHVAC:FourPipeFanCoil")]) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, obj := range ctx.doc.Objects {
		counts[hvacObjectKey(obj.Type, objectName(obj))]++
	}
	owners := map[string]int{}
	for _, relation := range relations {
		for _, equipment := range relation.ZoneEquipment {
			owners[hvacComponentKey(equipment)]++
		}
	}
	out := map[string]hvacHydronicDeliveryServices{}
	for _, obj := range ctx.objectsByType[normalizeFieldCatalogKey("ZoneHVAC:FourPipeFanCoil")] {
		_, _, hasCooling := fieldValueIndexByCatalogName(obj, "Cooling Coil Object Type")
		_, _, hasHeating := fieldValueIndexByCatalogName(obj, "Heating Coil Object Type")
		if !hasCooling && !hasHeating {
			continue
		}
		key := hvacObjectKey(obj.Type, objectName(obj))
		entry := hvacHydronicDeliveryServices{LocalServices: map[string]bool{}, LocalCoils: map[string]ComponentRef{}}
		out[key] = entry // An unresolved native declaration is not a fallback.
		if counts[key] != 1 || owners[key] != 1 {
			continue
		}
		for _, role := range []struct{ service, prefix string }{{"cooling", "Cooling"}, {"heating", "Heating"}} {
			coilType := fieldValueByCatalogName(obj, role.prefix+" Coil Object Type")
			coilName := fieldValueByCatalogName(obj, role.prefix+" Coil Name")
			water := role.service == "cooling" && (strings.EqualFold(coilType, "Coil:Cooling:Water") || strings.EqualFold(coilType, "Coil:Cooling:Water:DetailedGeometry")) ||
				role.service == "heating" && strings.EqualFold(coilType, "Coil:Heating:Water")
			coilKey := hvacObjectKey(coilType, coilName)
			if !water {
				// FourPipeFanCoil also supports an electric heating coil. It is
				// local heating, never a service supplied by the cooling plant.
				if role.service == "heating" && strings.EqualFold(coilType, "Coil:Heating:Electric") && counts[coilKey] == 1 &&
					hvacHydronicCoilReferencesMatch(ctx, key, coilKey, false) {
					entry.LocalServices["heating"] = true
					entry.LocalCoils["heating"] = componentRefFromHVACComponent(newHVACComponent(ctx, coilType, coilName))
				}
				continue
			}
			if counts[coilKey] != 1 || !hvacHydronicCoilReferencesMatch(ctx, key, coilKey, true) {
				continue
			}
			coil := ctx.objectsByTypeName[coilKey]
			if plant, ok := hvacHydronicCoilPlant(ctx, loops, counts, coil); ok {
				entry.Bindings = append(entry.Bindings, hvacHydronicDeliveryBinding{
					ServiceKind: role.service, PlantLoop: plant,
					Coil:         componentRefFromHVACComponent(newHVACComponent(ctx, coilType, coilName)),
					SourceLabels: hvacHydronicPlantSourceLabels(loops, counts, plant),
				})
			}
		}
		mixerType := fieldValueByCatalogName(obj, "Outdoor Air Mixer Object Type")
		mixerName := fieldValueByCatalogName(obj, "Outdoor Air Mixer Name")
		if strings.EqualFold(mixerType, "OutdoorAir:Mixer") && counts[hvacObjectKey(mixerType, mixerName)] == 1 {
			entry.LocalServices["ventilation"] = true
		}
		out[key] = entry
	}
	return out
}

// The legitimate hydronic double reference is one delivery plus one Branch.
// Another delivery, detached Branch, or duplicated reference is not ownership.
func hvacHydronicCoilReferencesMatch(ctx *hvacContext, deliveryKey, coilKey string, water bool) bool {
	deliveryRefs, branchRefs := 0, 0
	for _, ref := range ctx.componentReferences {
		if hvacObjectKey(ref.TargetObjectType, ref.TargetObjectName) != coilKey {
			continue
		}
		if !ref.TargetExists {
			return false
		}
		switch {
		case hvacObjectKey(ref.FromObjectType, ref.FromObjectName) == deliveryKey:
			deliveryRefs++
		case strings.EqualFold(ref.FromObjectType, "Branch"):
			branchRefs++
		default:
			return false
		}
	}
	return deliveryRefs == 1 && (water && branchRefs == 1 || !water && branchRefs == 0)
}

func hvacHydronicCoilPlant(ctx *hvacContext, loops []HVACLoop, counts map[string]int, coil Object) (LoopRef, bool) {
	inlet := fieldValueByCatalogName(coil, "Water Inlet Node Name")
	outlet := fieldValueByCatalogName(coil, "Water Outlet Node Name")
	if inlet == "" || outlet == "" || strings.EqualFold(inlet, outlet) {
		return LoopRef{}, false
	}
	var matched LoopRef
	occurrences, valid := 0, false
	key := hvacObjectKey(coil.Type, objectName(coil))
	for _, loop := range loops {
		for _, side := range []HVACLoopSide{loop.SupplySide, loop.DemandSide} {
			for _, branch := range side.Branches {
				for _, component := range branch.Components {
					if hvacComponentKey(component) != key {
						continue
					}
					occurrences++
					branchObject := ctx.objectsByTypeName[hvacObjectKey("Branch", branch.Name)]
					valid = strings.EqualFold(loop.Type, "PlantLoop") && strings.EqualFold(side.Name, "demand") &&
						counts[hvacObjectKey(loop.Type, loop.Name)] == 1 && counts[hvacObjectKey("BranchList", side.BranchListName)] == 1 &&
						counts[hvacObjectKey("Branch", branch.Name)] == 1 &&
						strings.EqualFold(component.WaterInletNode, inlet) && strings.EqualFold(component.WaterOutletNode, outlet) &&
						strings.EqualFold(hvacFieldValue(branchObject, component.InletFieldIndex), inlet) &&
						strings.EqualFold(hvacFieldValue(branchObject, component.OutletFieldIndex), outlet)
					matched = loopRefFromLoop(loop)
				}
			}
		}
	}
	return matched, occurrences == 1 && valid
}

func (entry hvacHydronicDeliveryServices) bindPath(path ZoneServicePath) (ZoneServicePath, bool) {
	if path.AirLoop != nil || path.CondenserLoop != nil || path.RefrigerantSystem != nil {
		return path, false
	}
	if path.PlantLoop == nil {
		if !entry.LocalServices[path.ServiceKind] || path.PathType != "direct_zone_air" || path.SourceSystem != nil {
			return path, false
		}
		if coil, known := entry.LocalCoils[path.ServiceKind]; known {
			return hvacHydronicBindConditioning(path, coil)
		}
		return path, len(path.Conditioning) == 0
	}
	for _, binding := range entry.Bindings {
		if path.PathType == "direct_zone_hydronic" && path.ServiceKind == binding.ServiceKind && strings.EqualFold(path.PlantLoop.Type, binding.PlantLoop.Type) &&
			strings.EqualFold(path.PlantLoop.Name, binding.PlantLoop.Name) {
			if source := path.SourceSystem; source != nil {
				matched := false
				for _, label := range binding.SourceLabels {
					matched = matched || source.Type == "source" && strings.EqualFold(source.Name, label)
				}
				if !matched {
					return path, false
				}
			}
			return hvacHydronicBindConditioning(path, binding.Coil)
		}
	}
	return path, false
}

func hvacHydronicBindConditioning(path ZoneServicePath, coil ComponentRef) (ZoneServicePath, bool) {
	for _, existing := range path.Conditioning {
		if existing.ObjectIndex != coil.ObjectIndex || !strings.EqualFold(existing.ObjectType, coil.ObjectType) || !strings.EqualFold(existing.ObjectName, coil.ObjectName) {
			return path, false
		}
	}
	// The legacy chain runs first and path IDs deliberately ignore metadata.
	// Bind the complete conditioning metadata to the original typed coil before
	// first-wins deduplication, never append it beside an unrelated legacy coil.
	path.Conditioning = []ComponentRef{coil}
	return path, true
}

func hvacHydronicPlantSourceLabels(loops []HVACLoop, counts map[string]int, plant LoopRef) []string {
	var labels []string
	for _, loop := range loops {
		if !strings.EqualFold(loop.Type, plant.Type) || !strings.EqualFold(loop.Name, plant.Name) {
			continue
		}
		for _, branch := range loop.SupplySide.Branches {
			for _, component := range branch.Components {
				if component.Exists && counts[hvacComponentKey(component)] == 1 && isPlantSourceEquipmentType(component.ObjectType) {
					labels = appendUniqueString(labels, componentLabel(component))
				}
			}
		}
	}
	return labels
}
