package simulation

import (
	"fmt"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type energySurfaceCategoryIndex struct {
	BySurfaceKey map[string]energySurfaceCategory
}

type energySurfaceCategory struct {
	SurfaceID           string
	EntityID            string
	ZoneName            string
	SpaceName           string
	SurfaceType         string
	BoundaryKind        string
	Category            string
	EffectiveMultiplier float64
	RelatedEntityIDs    []string
}

type energyDriverBuildContext struct {
	Enabled              bool
	SurfaceCategories    energySurfaceCategoryIndex
	AirCouplings         energyAirCouplingIndex
	Multipliers          energyEffectiveMultiplierIndex
	GeometryWarning      *EnergyWarning
	DirectHVACComponents []energyPathDirectHVACComponentTarget
	VRFSystems           []energyPathVRFSystem
	RadiantLoads         []energyPathRadiantLoadTarget
}

func newEnergyDriverBuildContext(report idf.GeometryReport, documents ...idf.Document) energyDriverBuildContext {
	index := buildEnergySurfaceCategoryIndex(report)
	multipliers := energyEffectiveMultiplierIndex{}
	var directHVACComponents []energyPathDirectHVACComponentTarget
	var vrfSystems []energyPathVRFSystem
	var radiantLoads []energyPathRadiantLoadTarget
	if len(documents) > 0 {
		addEnergyInternalMassCategories(&index, documents[0], report)
		multipliers = buildEnergyEffectiveMultiplierIndex(documents[0])
		directHVACComponents = energyPathDirectHVACComponentTargets(documents[0])
		vrfSystems = energyPathVRFSystems(documents[0])
		radiantLoads = energyPathRadiantLoadTargets(documents[0])
	}
	return energyDriverBuildContext{
		Enabled:              true,
		SurfaceCategories:    index,
		AirCouplings:         buildEnergyAirCouplingIndex(report.Topology),
		Multipliers:          multipliers,
		DirectHVACComponents: directHVACComponents,
		VRFSystems:           vrfSystems,
		RadiantLoads:         radiantLoads,
	}
}

func prepareEnergyDriverSeries(series []energyExplanationSeries, sources []EnergyDataSource, context energyDriverBuildContext) ([]energyExplanationSeries, []EnergyDataSource, []EnergyWarning) {
	if !context.Enabled {
		return series, sources, nil
	}
	sourceIndex := make(map[string]int, len(sources))
	for index := range sources {
		sourceIndex[sources[index].ID] = index
	}
	out := make([]energyExplanationSeries, 0, len(series))
	warnings := []EnergyWarning{}
	geometryWarningUsed := false
	for _, item := range series {
		if item.Level != "heat" {
			out = append(out, item)
			continue
		}

		policy := energyDriverSourcePolicyFor(item.sourceName, item.Kind)
		if item.parseCategoryAggregate {
			policy.Category = item.DriverCategory
			policy.Label = energyDriverCategoryLabel(item.DriverCategory)
			policy.Role = energyDriverSourceRoleMainFlow
			policy.Explanation = energyDriverSurfaceExplanation
		}
		var surface energySurfaceCategory
		if item.SurfaceScoped {
			var warning *EnergyWarning
			surface, warning = context.SurfaceCategories.resolve(item.sourceKeyValue)
			policy.Category = surface.Category
			policy.Label = energyDriverCategoryLabel(surface.Category)
			item.ZoneName = surface.ZoneName
			item.RelatedEntityIDs = appendUniqueStrings(item.RelatedEntityIDs, surface.RelatedEntityIDs...)
			if policy.Role == energyDriverSourceRoleMainFlow {
				policy.Explanation = energyDriverSurfaceExplanation
			}
			if warning != nil {
				warnings = appendEnergyDriverWarning(warnings, *warning)
				if context.GeometryWarning != nil && !geometryWarningUsed {
					warnings = appendEnergyDriverWarning(warnings, *context.GeometryWarning)
					geometryWarningUsed = true
				}
			}
		}
		applyEnergyPathRadiantSurfaceContext(&item, &policy, context)
		if policy.Category == energyDriverCategoryInterzoneAir {
			coupling := context.AirCouplings.resolve(item.sourceKeyValue)
			pairwise := coupling.Pairwise && energyDriverInterzoneSourceCanBePairwise(item)
			item.RelatedEntityIDs = appendUniqueStrings(item.RelatedEntityIDs, coupling.RelatedEntityIDs...)
			// An exact topology match can establish the receiving Zone even when
			// this output family is aggregate-only. PairID is intentionally not
			// copied unless the heat source itself is explicitly pairwise.
			if coupling.PairID != "" && coupling.ZoneName != "" || item.ZoneName == "" {
				item.ZoneName = coupling.ZoneName
			}
			if pairwise {
				item.interzonePairID = coupling.PairID
			}
			// Source rows remain visible at Zone scope. Only a signed pairwise net
			// derived below is eligible for Building main-flow presentation.
			item.driverZoneOnly = true
		}
		item.DriverCategory = canonicalEnergyDriverCategory(policy.Category)
		item.DriverSourceRole = policy.Role
		item.DriverExplanation = policy.Explanation
		item.DriverComponent = energyDriverComponentForSeries(item)
		item.Label = energyDriverCategoryLabel(item.DriverCategory)

		for _, sourceID := range item.SourceIDs {
			index, ok := sourceIndex[sourceID]
			if !ok {
				continue
			}
			source := &sources[index]
			source.DriverRole = policy.Role
			source.DriverCategory = item.DriverCategory
			source.DriverComponent = item.DriverComponent
			source.HeatDirection = energyDriverHeatDirection(item)
			source.Explanation = policy.Explanation
			if !item.parseCategoryAggregate {
				source.RawValue = roundedEnergyNumber(item.Total)
			}
			source.ZoneName = item.ZoneName
			if policy.Role != energyDriverSourceRoleMainFlow {
				source.InspectorSection = energyDriverInspectorSectionContext
			} else if source.InspectorSection == "" {
				source.InspectorSection = energyDriverInspectorSectionBreakdown
			}
			if item.SurfaceScoped {
				source.RelatedEntityIDs = appendUniqueStrings(source.RelatedEntityIDs, surface.RelatedEntityIDs...)
			}
		}
		if item.parseCategorySource {
			// The streaming category aggregate carries this row's value and
			// source ID. Keeping the original series would count it twice.
			continue
		}
		out = append(out, canonicalEnergyExplanationSeries(item))
	}
	return out, sources, warnings
}

func energyDriverInterzoneSourceCanBePairwise(item energyExplanationSeries) bool {
	name := normalizeEnergyOutputName(firstNonEmpty(item.SourceName, item.sourceName, item.Kind))
	// EnergyPlus Zone Mixing and Zone Air Heat Balance Interzone Air outputs are
	// receiving-zone aggregates. An exact topology-name match alone does not
	// turn either family into a pairwise heat measurement.
	if item.Kind == "heat.interzone_air" ||
		strings.Contains(name, "zone mixing ") || strings.Contains(name, "zone cross mixing ") ||
		strings.Contains(name, "zone crossmixing ") || strings.Contains(name, "zone refrigeration door mixing ") ||
		strings.Contains(name, "zone air heat balance interzone air") {
		return false
	}
	return true
}

func appendEnergyDriverWarningsForPeriod(existing []EnergyWarning, warnings []EnergyWarning, period string) []EnergyWarning {
	out := append([]EnergyWarning(nil), existing...)
	for _, warning := range warnings {
		warning.Period = period
		out = appendEnergyDriverWarning(out, warning)
	}
	return out
}

func appendEnergyDriverWarning(warnings []EnergyWarning, warning EnergyWarning) []EnergyWarning {
	for _, existing := range warnings {
		if existing.Code == warning.Code && existing.Message == warning.Message && existing.Period == warning.Period {
			return warnings
		}
	}
	return append(warnings, warning)
}

func buildEnergySurfaceCategoryIndex(report idf.GeometryReport) energySurfaceCategoryIndex {
	index := energySurfaceCategoryIndex{BySurfaceKey: map[string]energySurfaceCategory{}}
	boundaryBySurfaceID := make(map[string]idf.ThermalBoundaryRecord, len(report.Topology.Boundaries))
	openingByWindowID := make(map[string]idf.ThermalOpeningRecord, len(report.Topology.Openings))
	surfaceByID := make(map[string]idf.GeometrySurface, len(report.Surfaces))
	connectionIDsByBoundaryID := map[string][]string{}
	connectionIDsByOpeningID := map[string][]string{}
	for _, surface := range report.Surfaces {
		surfaceByID[normalizeEnergySurfaceKey(surface.ID)] = surface
	}
	for _, boundary := range report.Topology.Boundaries {
		boundaryBySurfaceID[normalizeEnergySurfaceKey(boundary.SurfaceID)] = boundary
	}
	for _, opening := range report.Topology.Openings {
		openingByWindowID[normalizeEnergySurfaceKey(opening.WindowID)] = opening
	}
	for _, connection := range report.Topology.Connections {
		for _, boundaryID := range connection.BoundaryIDs {
			key := normalizeEnergySurfaceKey(boundaryID)
			connectionIDsByBoundaryID[key] = appendUniqueStrings(connectionIDsByBoundaryID[key], connection.ID)
		}
		for _, openingID := range connection.OpeningIDs {
			key := normalizeEnergySurfaceKey(openingID)
			connectionIDsByOpeningID[key] = appendUniqueStrings(connectionIDsByOpeningID[key], connection.ID)
		}
	}

	for _, surface := range report.Surfaces {
		boundary := boundaryBySurfaceID[normalizeEnergySurfaceKey(surface.ID)]
		boundaryKind := firstNonEmpty(boundary.RelationKind, boundary.BoundaryCondition, surface.OutsideBoundary)
		entityID := firstNonEmpty(boundary.SurfaceEntityID, surface.ID)
		category := energySurfaceCategory{
			SurfaceID:           surface.ID,
			EntityID:            entityID,
			ZoneName:            surface.ZoneName,
			SpaceName:           surface.SpaceName,
			SurfaceType:         surface.SurfaceType,
			BoundaryKind:        boundaryKind,
			Category:            energySurfaceDriverCategory(surface.SurfaceType, surface.Type, boundaryKind, surface.OutsideBoundary),
			EffectiveMultiplier: energyGeometryMultiplier(surface.ZoneMultiplier, surface.SurfaceMultiplier),
		}
		category.RelatedEntityIDs = appendUniqueStrings(category.RelatedEntityIDs, surface.ID, entityID)
		category.RelatedEntityIDs = appendUniqueStrings(category.RelatedEntityIDs, connectionIDsByBoundaryID[normalizeEnergySurfaceKey(boundary.ID)]...)
		addEnergySurfaceCategoryKeys(&index, category, surface.Name, surface.ID, entityID, boundary.SurfaceName, boundary.ID)
	}

	for _, window := range report.Windows {
		opening := openingByWindowID[normalizeEnergySurfaceKey(window.ID)]
		baseSurface := surfaceByID[normalizeEnergySurfaceKey(window.BaseSurfaceID)]
		entityID := firstNonEmpty(opening.EntityID, window.ID)
		boundaryKind := "fenestration"
		if base := boundaryBySurfaceID[normalizeEnergySurfaceKey(window.BaseSurfaceID)]; base.RelationKind != "" {
			boundaryKind = base.RelationKind
		}
		category := energySurfaceCategory{
			SurfaceID:           window.ID,
			EntityID:            entityID,
			ZoneName:            firstNonEmpty(window.ZoneName, baseSurface.ZoneName),
			SpaceName:           baseSurface.SpaceName,
			SurfaceType:         window.SurfaceType,
			BoundaryKind:        boundaryKind,
			Category:            energyDriverCategoryWindowsDoors,
			EffectiveMultiplier: energyGeometryMultiplier(window.ZoneMultiplier, window.SurfaceMultiplier),
		}
		category.RelatedEntityIDs = appendUniqueStrings(category.RelatedEntityIDs, window.ID, entityID, window.BaseSurfaceID)
		category.RelatedEntityIDs = appendUniqueStrings(category.RelatedEntityIDs, connectionIDsByOpeningID[normalizeEnergySurfaceKey(opening.ID)]...)
		if len(connectionIDsByOpeningID[normalizeEnergySurfaceKey(opening.ID)]) == 0 {
			base := boundaryBySurfaceID[normalizeEnergySurfaceKey(window.BaseSurfaceID)]
			category.RelatedEntityIDs = appendUniqueStrings(category.RelatedEntityIDs, connectionIDsByBoundaryID[normalizeEnergySurfaceKey(base.ID)]...)
		}
		addEnergySurfaceCategoryKeys(&index, category, window.Name, window.ID, entityID, opening.Name, opening.ID)
	}
	return index
}

func addEnergyInternalMassCategories(index *energySurfaceCategoryIndex, doc idf.Document, report idf.GeometryReport) {
	for _, object := range doc.Objects {
		if !strings.EqualFold(strings.TrimSpace(object.Type), "InternalMass") {
			continue
		}
		for _, target := range purposeInternalMassSurfaceTargets(doc, report, object) {
			zoneName := ""
			if len(target.ZoneNames) == 1 {
				zoneName = target.ZoneNames[0]
			}
			entityID := fmt.Sprintf("internal-mass-%d", object.Index)
			category := energySurfaceCategory{
				SurfaceID:           entityID,
				EntityID:            entityID,
				ZoneName:            zoneName,
				SurfaceType:         "InternalMass",
				BoundaryKind:        "internal_mass",
				Category:            energyDriverCategoryStorageOther,
				EffectiveMultiplier: 1,
				RelatedEntityIDs:    []string{entityID},
			}
			addEnergySurfaceCategoryKeys(index, category, target.Name, entityID)
		}
	}
}

func (index energySurfaceCategoryIndex) resolve(surfaceKey string) (energySurfaceCategory, *EnergyWarning) {
	if category, ok := index.BySurfaceKey[normalizeEnergySurfaceKey(surfaceKey)]; ok {
		return category, nil
	}
	key := strings.TrimSpace(surfaceKey)
	fallback := energySurfaceCategory{
		SurfaceID:           key,
		EntityID:            key,
		BoundaryKind:        "unresolved",
		Category:            energyDriverCategoryStorageOther,
		EffectiveMultiplier: 1,
		RelatedEntityIDs:    appendUniqueStrings(nil, key),
	}
	warning := &EnergyWarning{
		Severity: "warning",
		Code:     "energy_driver_surface_unresolved",
		Message:  fmt.Sprintf("Surface output key %q could not be resolved through GeometryReport; its value is retained in Other / storage.", key),
	}
	return fallback, warning
}

func addEnergySurfaceCategoryKeys(index *energySurfaceCategoryIndex, category energySurfaceCategory, keys ...string) {
	if index == nil {
		return
	}
	if index.BySurfaceKey == nil {
		index.BySurfaceKey = map[string]energySurfaceCategory{}
	}
	for _, value := range keys {
		key := normalizeEnergySurfaceKey(value)
		if key == "" {
			continue
		}
		if _, exists := index.BySurfaceKey[key]; !exists {
			index.BySurfaceKey[key] = category
		}
	}
}

func normalizeEnergySurfaceKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func energyGeometryMultiplier(values ...float64) float64 {
	multiplier := 1.0
	for _, value := range values {
		if value != 0 {
			multiplier *= value
		}
	}
	return multiplier
}

func energySurfaceDriverCategory(surfaceType string, objectType string, boundaryKind string, outsideBoundary string) string {
	typeKey := normalizeEnergyOutputName(strings.Join([]string{surfaceType, objectType}, " "))
	boundaryKey := normalizeEnergyOutputName(strings.Join([]string{boundaryKind, outsideBoundary}, " "))
	switch {
	case strings.Contains(typeKey, "internal mass") || strings.Contains(boundaryKey, "adiabatic"):
		return energyDriverCategoryStorageOther
	case strings.Contains(typeKey, "window") || strings.Contains(typeKey, "door") || strings.Contains(typeKey, "fenestration") || strings.Contains(typeKey, "glass"):
		return energyDriverCategoryWindowsDoors
	case strings.Contains(boundaryKey, "interzone") || strings.Contains(boundaryKey, "interspace") ||
		strings.Contains(boundaryKey, "counterpart") || strings.Contains(boundaryKey, "surface") ||
		boundaryKey == "zone" || boundaryKey == "space" ||
		strings.Contains(boundaryKey, " zone ") || strings.HasSuffix(boundaryKey, " zone") ||
		strings.Contains(boundaryKey, " space ") || strings.HasSuffix(boundaryKey, " space"):
		return energyDriverCategoryInterzoneSurfaces
	case strings.Contains(typeKey, "floor") && (strings.Contains(boundaryKey, "ground") || strings.Contains(boundaryKey, "foundation")):
		return energyDriverCategoryGroundFloors
	case strings.Contains(typeKey, "wall") && energySurfaceBoundaryIsExternal(boundaryKey):
		return energyDriverCategoryExteriorWalls
	case (strings.Contains(typeKey, "roof") || strings.Contains(typeKey, "ceiling")) && energySurfaceBoundaryIsExternal(boundaryKey):
		return energyDriverCategoryRoofs
	case strings.Contains(typeKey, "floor") && energySurfaceBoundaryIsExternal(boundaryKey):
		return energyDriverCategoryGroundFloors
	default:
		return energyDriverCategoryStorageOther
	}
}

func energySurfaceBoundaryIsExternal(boundaryKey string) bool {
	return strings.Contains(boundaryKey, "exterior") || strings.Contains(boundaryKey, "outdoor") ||
		strings.Contains(boundaryKey, "external") || strings.Contains(boundaryKey, "other side") ||
		strings.Contains(boundaryKey, "other_side") || strings.Contains(boundaryKey, "other-side") ||
		strings.Contains(boundaryKey, "otherside")
}
