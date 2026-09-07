package simulation

import (
	"slices"
	"sort"
	"strings"
)

// refreshEnergyPathConversionLinks validates the thermal/site boundary after a
// stored graph is read. The measured dual values are retained: an annual link
// can cover only the months for which both load and equipment energy exist.
func refreshEnergyPathConversionLinks(nodes []EnergyExplanationNode, links []EnergyPathLink) []EnergyPathLink {
	nodeByID := make(map[string]EnergyExplanationNode, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}
	carriersByEndUse := map[string][]string{}
	for _, link := range links {
		if link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier" {
			continue
		}
		endUse, carrier := nodeByID[link.FromID], nodeByID[link.ToID]
		if endUse.Level != "end_use" || carrier.Level != "carrier" || !energyPathPositiveFinite(link.ToValue) {
			continue
		}
		carriersByEndUse[endUse.ID] = appendUniqueStrings(carriersByEndUse[endUse.ID], canonicalEnergyPathCarrier(energyPathV2CarrierEvidence(carrier)))
	}
	out := make([]EnergyPathLink, 0, len(links))
	for _, original := range links {
		if original.Relation != "load_to_end_use" {
			out = append(out, original)
			continue
		}
		load, endUse := nodeByID[original.FromID], nodeByID[original.ToID]
		if slices.Contains(endUse.Badges, "filtered_carrier_splits") {
			// A conversion that included rejected carrier data cannot be
			// reinterpreted as the surviving fuel alone. Its source records stay
			// inspectable, but no new thermal allocation or ratio is invented.
			continue
		}
		service := energyCanonicalServiceKind(firstNonEmpty(load.ServiceKind, energyExplanationKindSuffix(load.Kind)))
		endUseKind := canonicalEnergyPathEndUse(firstNonEmpty(endUse.EndUse, energyExplanationKindSuffix(endUse.Kind)))
		if load.Level != "load" || endUse.Level != "end_use" ||
			(service != "cooling" && service != "heating") || service != endUseKind ||
			(original.ServiceKind != "" && energyCanonicalServiceKind(original.ServiceKind) != service) ||
			!strings.EqualFold(load.ScaleDomain, "thermal") || !strings.EqualFold(endUse.ScaleDomain, "site") ||
			!strings.EqualFold(strings.TrimSpace(load.ZoneName), strings.TrimSpace(endUse.ZoneName)) {
			continue
		}
		fromUnit, toUnit := energyPathConversionUnitBase(load.Unit), energyPathConversionUnitBase(endUse.Unit)
		if fromUnit == "" || fromUnit != toUnit {
			continue
		}
		link := original
		link.ServiceKind = service
		link.FromUnit, link.ToUnit = load.Unit, endUse.Unit
		link.SourceIDs = appendUniqueStrings(nil, original.SourceIDs...)
		// Partial-period links must not inherit annual endpoint sources from
		// months with no paired observation. Only a full endpoint observation
		// can safely recover missing provenance from both canonical nodes.
		if len(link.SourceIDs) == 0 && link.FromValue == energyExplanationEffectiveNodeValue(load) && link.ToValue == energyExplanationEffectiveNodeValue(endUse) {
			link.SourceIDs = appendUniqueStrings(link.SourceIDs, load.SourceIDs...)
			link.SourceIDs = appendUniqueStrings(link.SourceIDs, endUse.SourceIDs...)
		}
		sort.Strings(link.SourceIDs)
		// Carrier identities are private build metadata, so reconstruct them
		// from the retained meter splits instead of trusting a stale ratio label.
		load.ServiceKind = service
		endUse.EndUse = endUseKind
		endUse.endUseCarriers = appendUniqueStrings(nil, carriersByEndUse[endUse.ID]...)
		if len(endUse.endUseCarriers) == 0 && endUse.Carrier != "" {
			endUse.endUseCarriers = []string{canonicalEnergyPathCarrier(endUse.Carrier)}
		}
		setEnergyPathConversionRatioKind(&link, &load, &endUse)
		finalizeEnergyPathLinkRatio(&link)
		out = append(out, link)
	}
	return out
}
