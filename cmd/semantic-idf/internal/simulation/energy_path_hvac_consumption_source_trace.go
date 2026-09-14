package simulation

import (
	"sort"
	"strings"
)

// Run after nested Zone source details have been assembled. A measured boiler
// total remains a native object observation at the top level, but is not a raw
// measurement for each recipient Zone. Only the exact per-source allocation is
// copied into a Zone detail; generic edge-wide factors cannot supply that proof.
func applyEnergyPathHVACConsumptionSourceAllocations(sources []EnergyDataSource, plan energyPathZoneHVACAllocationPlan, pools []energyPathHVACConsumptionPool, scope EnergyExplanationScope) []EnergyDataSource {
	sharedSources := map[string]bool{}
	sharedMembers := []energyPathHVACConsumptionMember{}
	for _, pool := range pools {
		for _, member := range pool.Members {
			if strings.TrimSpace(member.ZoneName) != "" {
				continue
			}
			sharedMembers = append(sharedMembers, member)
			ids := appendUniqueStrings(nil, member.Series.MonthlySourceIDs...)
			ids = appendUniqueStrings(ids, member.Series.AnnualSourceIDs...)
			if len(ids) == 0 {
				ids = appendUniqueStrings(ids, member.Series.SourceIDs...)
			}
			for _, id := range ids {
				sharedSources[id] = true
			}
		}
	}
	if len(sharedMembers) == 0 {
		return sources
	}
	contextSources := map[string]bool{}
	for _, source := range sources {
		if sharedSources[source.ID] {
			continue
		}
		for _, member := range sharedMembers {
			if normalizePurposeToken(source.KeyValue) != normalizePurposeToken(member.ObjectName) {
				continue
			}
			energyName := normalizePurposeToken(member.OutputName)
			rateName := strings.TrimSuffix(energyName, " energy") + " rate"
			name := normalizePurposeToken(source.Name)
			if name == energyName || strings.HasSuffix(energyName, " energy") && name == rateName {
				contextSources[source.ID] = true
			}
		}
	}
	bySource := map[string]map[string]energyPathHVACConsumptionSourceAllocation{}
	for _, row := range plan.ConsumptionSourceAllocations {
		if scope.Kind == "zone" && !strings.EqualFold(scope.ZoneName, row.ZoneName) {
			continue
		}
		for _, id := range row.SourceIDs {
			if !sharedSources[id] {
				continue
			}
			if bySource[id] == nil {
				bySource[id] = map[string]energyPathHVACConsumptionSourceAllocation{}
			}
			zone := energyPathZoneHVACZoneKey(row.ZoneName)
			current := bySource[id][zone]
			current.ZoneName = row.ZoneName
			current.ObservedValue = roundedEnergyNumber(current.ObservedValue + row.ObservedValue)
			current.AllocatedValue = roundedEnergyNumber(current.AllocatedValue + row.AllocatedValue)
			current.LoadSourceIDs = appendUniqueStrings(current.LoadSourceIDs, row.LoadSourceIDs...)
			current.RelatedPathIDs = appendUniqueStrings(current.RelatedPathIDs, row.RelatedPathIDs...)
			bySource[id][zone] = current
		}
	}
	out := append([]EnergyDataSource(nil), sources...)
	for i := range out {
		source := &out[i]
		if !sharedSources[source.ID] && !contextSources[source.ID] {
			continue
		}
		// Remove inferred/duplicated Zone details, including those for an
		// unserved or unobserved Zone. Keep native Building observations intact.
		details := []EnergyDataSourceScopeDetail{}
		for _, detail := range source.ScopeDetails {
			if detail.Scope.Kind != "zone" {
				details = append(details, detail)
			}
		}
		// Native component observations do not establish an allocated scalar.
		// Clear the generic source fallback in every scope, including Building
		// under direct-only policy. Only actual per-source Zone rows below can
		// establish an allocation; RawValue/EffectiveValue stay untouched.
		source.AllocatedValue, source.AllocationFactor, source.AllocationApplied = 0, 0, false
		source.AllocationExplanation = "Native shared-component observation; only explicit source-local Zone allocation records establish allocated consumption."
		source.AllocationFormula = ""
		if contextSources[source.ID] {
			// An Hourly energy observation or a native Rate companion is useful
			// component context, but is not the Monthly pool's allocation budget.
			source.ScopeDetails = details
			source.AllocationExplanation = "Native shared-component context only; the independently observed Monthly energy source is the allocation budget. No individual-Zone raw/effective consumption or allocated share is inferred from this companion."
			source.AllocationFormula = ""
			continue
		}
		zoneKeys := []string{}
		for zone := range bySource[source.ID] {
			zoneKeys = append(zoneKeys, zone)
		}
		sort.Strings(zoneKeys)
		for _, key := range zoneKeys {
			row := bySource[source.ID][key]
			factor := 0.0
			if energyDataSourceValueKnown(*source, energySourceObservedEffective) && source.EffectiveValue > 0 {
				factor = row.AllocatedValue / source.EffectiveValue
			}
			details = append(details, EnergyDataSourceScopeDetail{
				Scope:             EnergyExplanationScope{Kind: "zone", ZoneName: row.ZoneName, AggregationBasis: scope.AggregationBasis},
				AllocationApplied: true, AllocationFactor: factor, AllocatedValue: row.AllocatedValue, AggregationBasis: scope.AggregationBasis,
				inspectorScopedValuePresence: true,
			})
			if scope.Kind == "zone" {
				source.AllocationApplied, source.AllocationFactor, source.AllocatedValue = true, factor, row.AllocatedValue
				source.InputSourceIDs = appendUniqueStrings(append([]string(nil), source.InputSourceIDs...), row.LoadSourceIDs...)
			}
		}
		if scope.Kind == "zone" {
			source.AllocationExplanation = "Raw/effective values describe this native shared component, not an individually measured Zone input. The Zone amount is its independently allocated source-local share; missing or ineligible shares remain unavailable."
			source.AllocationFormula = "sum of completed monthly allocations of this exact component within its original recipient service paths"
		}
		source.ScopeDetails = details
	}
	return out
}
