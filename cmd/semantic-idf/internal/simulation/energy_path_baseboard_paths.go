package simulation

import (
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func energyPathNativeBaseboardPathKey(zone, equipment string) string {
	return normalizePurposeToken(zone) + "\x00" + normalizePurposeToken(equipment)
}

// Bind native consumption to the exact delivery component in this document's
// topology namespace. Output:Variable indices and another prepared document's
// component positions are not equipment identities.
func buildEnergyPathNativeBaseboardPaths(doc idf.Document, summaries []idf.ZoneServiceSummary) map[string][]string {
	out := map[string][]string{}
	for _, target := range energyPathBaseboardTargets(doc) {
		var paths []string
		for _, summary := range summaries {
			if summary.SpaceName != "" || summary.ServedSubject.SpaceName != "" ||
				!energyPathNativeBaseboardOwnerMatches(target.ZoneName, summary.ZoneName, summary.ServedSubject.ZoneName) {
				continue
			}
			for _, path := range summary.Paths {
				if path.ID == "" || path.SpaceName != "" || path.ServedSubject.SpaceName != "" || path.ServiceKind != "heating" || path.PathType != "baseboard" ||
					path.PlantLoop != nil || path.CondenserLoop != nil || path.AirLoop != nil || path.RefrigerantSystem != nil ||
					!strings.EqualFold(firstNonEmpty(path.ZoneName, path.ServedSubject.ZoneName, summary.ZoneName), target.ZoneName) ||
					!energyPathNativeBaseboardOwnerMatches(target.ZoneName, path.ZoneName, path.ServedSubject.ZoneName) ||
					!strings.EqualFold(path.Delivery.ObjectType, target.Component.ObjectType) || !strings.EqualFold(path.Delivery.ObjectName, target.KeyValue) ||
					path.Delivery.ID != target.Component.ID || path.Delivery.ObjectIndex != target.Component.ObjectIndex {
					continue
				}
				paths = append(paths, path.ID)
			}
		}
		if len(paths) == 1 {
			out[energyPathNativeBaseboardPathKey(target.ZoneName, target.KeyValue)] = paths
		}
	}
	return out
}

func energyPathNativeBaseboardOwnerMatches(owner string, declared ...string) bool {
	for _, zone := range declared {
		if strings.TrimSpace(zone) != "" && !strings.EqualFold(strings.TrimSpace(zone), owner) {
			return false
		}
	}
	return true
}

func qualifyEnergyPathNativeBaseboardNodes(nodes []EnergyExplanationNode, sources []EnergyDataSource, scope EnergyExplanationScope, paths map[string][]string) {
	local := energyPathNativeConsumptionSources(sources, scope)
	if len(local) == 0 {
		return
	}
	sourcePaths := map[string][]string{}
	for _, source := range sources {
		if local[source.ID] {
			sourcePaths[source.ID] = paths[energyPathNativeBaseboardPathKey(source.ZoneName, source.KeyValue)]
		}
	}
	for i := range nodes {
		node := &nodes[i]
		if !energyPathLegacyNodeIsDirectZoneEnergy(*node) || legacyEnergyNodeIsCarrier(*node) || !energyPathOnlyNativeBaseboardSources(node.SourceIDs, local) {
			continue
		}
		node.RelatedPathIDs = nil
		for _, id := range node.SourceIDs {
			if len(sourcePaths[id]) > 0 {
				node.RelatedPathIDs = appendUniqueStrings(node.RelatedPathIDs, sourcePaths[id]...)
			}
		}
		sort.Strings(node.RelatedPathIDs)
	}
}

func energyPathOnlyNativeBaseboardSources(ids []string, local map[string]bool) bool {
	if len(ids) == 0 {
		return false
	}
	for _, id := range ids {
		if !local[id] {
			return false
		}
	}
	return true
}
