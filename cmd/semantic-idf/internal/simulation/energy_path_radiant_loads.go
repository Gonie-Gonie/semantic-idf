package simulation

import "strings"

// Native low-temperature radiant energy is a model-total fluid/surface source,
// not a measured Zone-air load. Retain its exact dictionary identity while
// binding its Zone only through the validated original equipment ownership.
func bindEnergyPathRadiantLoadSeries(series []energyExplanationSeries, sources []EnergyDataSource, context energyDriverBuildContext) ([]energyExplanationSeries, []EnergyDataSource, []EnergyWarning) {
	if !context.Enabled {
		return series, sources, nil
	}
	owners := map[string]energyPathRadiantLoadTarget{}
	for _, target := range context.RadiantLoads {
		owners[normalizePurposeToken(target.KeyValue)] = target
	}
	sourceIndex := map[string]int{}
	for i := range sources {
		sourceIndex[sources[i].ID] = i
	}
	out := make([]energyExplanationSeries, 0, len(series))
	warnings := []EnergyWarning{}
	for _, item := range series {
		if item.Level != "load" && item.Stage != "load" || !energyPathIsRadiantLoadVariable(firstNonEmpty(item.SourceName, item.sourceName)) {
			out = append(out, item)
			continue
		}
		key := firstNonEmpty(item.SourceKey, item.sourceKeyValue)
		target, owned := owners[normalizePurposeToken(key)]
		definition, _ := energyLoadAliasDefinitionForName(firstNonEmpty(item.SourceName, item.sourceName))
		explanation := "Reported fluid heat inserted into or removed from an active radiant surface. Includes Zone and ZoneList multipliers; storage and exchange with other surfaces mean this is not same-period Zone-air heat delivery."
		if !owned {
			explanation = "Radiant output retained as context only: its equipment key has no unique validated original Zone and active-surface owner. It is not a Zone-air load or a new Zone."
			warnings = appendEnergyDriverWarning(warnings, EnergyWarning{Severity: "warning", Code: "radiant_load_owner_unresolved", Message: explanation + " Key: " + strings.TrimSpace(key)})
		}
		for _, sourceID := range item.SourceIDs {
			if i, ok := sourceIndex[sourceID]; ok {
				sources[i].Explanation = explanation
				if owned {
					sources[i].ZoneName = target.ZoneName
					sources[i].RelatedEntityIDs = appendUniqueStrings(sources[i].RelatedEntityIDs, target.Component.ID)
				} else {
					sources[i].ZoneName = ""
					// The output's thermal meaning is known even when its owner
					// is not. It leaves the candidate list below, so the later
					// load-selection annotator cannot supply this context metadata.
					sources[i].DriverRole = energyDriverSourceRoleContext
					sources[i].DriverCategory = "load." + definition.ServiceKind
					sources[i].DriverComponent = "load.active_surface_source.combined"
					sources[i].HeatDirection = definition.ServiceKind
					sources[i].InspectorSection = energyDriverInspectorSectionContext
				}
			}
		}
		if !owned {
			continue
		}
		item.ZoneName = target.ZoneName
		item.ThermalBoundary = energyPathThermalBoundaryActiveSurfaceSource
		item.RelatedEntityIDs = appendUniqueStrings(item.RelatedEntityIDs, target.Component.ID)
		out = append(out, item)
	}
	return out, sources, warnings
}

func energyPathRadiantSelectionWarnings(candidates, selected []energyExplanationSeries) []EnergyWarning {
	surfaceByZoneService := map[string]bool{}
	for _, item := range candidates {
		if item.ThermalBoundary == energyPathThermalBoundaryActiveSurfaceSource && energyLoadSeriesHasMaterialValue(item) {
			surfaceByZoneService[normalizePurposeToken(item.ZoneName)+"|"+item.ServiceKind] = true
		}
	}
	var warnings []EnergyWarning
	for _, item := range selected {
		if item.Stage != "load" || item.ThermalBoundary != "" || !surfaceByZoneService[normalizePurposeToken(item.ZoneName)+"|"+item.ServiceKind] {
			continue
		}
		warnings = appendEnergyDriverWarning(warnings, EnergyWarning{Severity: "warning", Code: "radiant_surface_boundary_not_selected",
			Message: "The selected " + item.ServiceKind + " load in " + item.ZoneName + " does not include the separately reported active radiant surface source. Air-system and surface-source observations are different measurement boundaries; this selection is not proof of total combined HVAC delivery."})
	}
	return warnings
}
