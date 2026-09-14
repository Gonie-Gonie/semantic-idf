package simulation

import (
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func energyPathNativeWindowACPathKey(zone, equipment, sourceName string) string {
	return normalizePurposeToken(zone) + "\x00" + normalizePurposeToken(equipment) + "\x00" + normalizeEnergyOutputName(sourceName)
}

// Membership proves a native original owner; a nil value deliberately retains
// that membership when the separate service-path topology is unresolved.
func buildEnergyPathNativeWindowACPaths(doc idf.Document, summaries []idf.ZoneServiceSummary) map[string][]string {
	out := map[string][]string{}
	for _, target := range energyPathWindowACTargets(doc) {
		var paths []string
		for _, summary := range summaries {
			if summary.SpaceName != "" || summary.ServedSubject.SpaceName != "" || strings.EqualFold(summary.ServedSubject.Kind, "space") ||
				!energyPathNativeBaseboardOwnerMatches(target.ZoneName, summary.ZoneName, summary.ServedSubject.ZoneName) {
				continue
			}
			for _, path := range summary.Paths {
				if path.ID == "" || path.ServiceKind != "cooling" || path.PathType != "direct_zone_air" || path.SpaceName != "" || path.ServedSubject.SpaceName != "" || strings.EqualFold(path.ServedSubject.Kind, "space") ||
					path.AirLoop != nil || path.PlantLoop != nil || path.CondenserLoop != nil || path.RefrigerantSystem != nil ||
					!strings.EqualFold(firstNonEmpty(path.ZoneName, path.ServedSubject.ZoneName, summary.ZoneName), target.ZoneName) ||
					!energyPathNativeBaseboardOwnerMatches(target.ZoneName, path.ZoneName, path.ServedSubject.ZoneName) ||
					!strings.EqualFold(path.Delivery.ObjectType, target.Parent.ObjectType) || !strings.EqualFold(path.Delivery.ObjectName, target.Parent.ObjectName) || path.Delivery.ID != target.Parent.ID || path.Delivery.ObjectIndex != target.Parent.ObjectIndex {
					continue
				}
				paths = append(paths, path.ID)
			}
		}
		if len(paths) != 1 {
			paths = nil
		}
		for _, name := range []string{"Cooling Coil Electricity Energy", "Cooling Coil Crankcase Heater Electricity Energy"} {
			out[energyPathNativeWindowACPathKey(target.ZoneName, target.Coil.ObjectName, name)] = paths
		}
		out[energyPathNativeWindowACPathKey(target.ZoneName, target.Fan.ObjectName, "Fan Electricity Energy")] = paths
	}
	return out
}

func qualifyEnergyPathNativeWindowACNodes(nodes []EnergyExplanationNode, sources []EnergyDataSource, scope EnergyExplanationScope, paths map[string][]string) {
	if scope.Kind != "zone" || strings.TrimSpace(scope.ZoneName) == "" || len(paths) == 0 {
		return
	}
	local := map[string]bool{}
	sourcePaths := map[string][]string{}
	counts := map[string]int{}
	for _, source := range sources {
		counts[source.ID]++
	}
	for _, source := range sources {
		name := strings.ToLower(strings.TrimSpace(source.Name))
		if counts[source.ID] != 1 || source.ID == "" || source.SourceType != "sql_report_data" || source.IsMeter || !strings.EqualFold(source.ZoneName, scope.ZoneName) ||
			!strings.EqualFold(source.ReportingFrequency, "Monthly") || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" ||
			(name != "cooling coil electricity energy" && name != "cooling coil crankcase heater electricity energy" && name != "fan electricity energy") {
			continue
		}
		owned, exists := paths[energyPathNativeWindowACPathKey(source.ZoneName, source.KeyValue, source.Name)]
		if exists {
			local[source.ID], sourcePaths[source.ID] = true, owned
		}
	}
	for i := range nodes {
		node := &nodes[i]
		if !energyPathLegacyNodeIsDirectZoneEnergy(*node) || legacyEnergyNodeIsCarrier(*node) || !energyPathOnlyNativeBaseboardSources(node.SourceIDs, local) {
			continue
		}
		node.RelatedPathIDs = nil
		node.nativeWindowACPathQualified = true
		complete := true
		for _, sourceID := range node.SourceIDs {
			complete = complete && len(sourcePaths[sourceID]) > 0
		}
		if !complete {
			continue
		}
		for _, sourceID := range node.SourceIDs {
			node.RelatedPathIDs = appendUniqueStrings(node.RelatedPathIDs, sourcePaths[sourceID]...)
		}
		sort.Strings(node.RelatedPathIDs)
	}
}
