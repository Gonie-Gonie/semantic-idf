package simulation

// Keep explicit non-Zone service boundaries separate from observed energy.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	energyPathBoundaryNonZoneDemand      = "non_zone_demand_unquantified"
	energyPathBoundaryTopologyIncomplete = "demand_topology_incomplete"
)

// This is a semantic restriction, never a positive allocation proof. Native
// purchased quantities remain in their existing sources and ledgers. Source
// IDs, when present, are actual observed dictionary references; an absent
// consumer is not 0. An incomplete boundary may carry only original topology
// identity when no native source exists (for example, a tabular-only meter).
// This finite contract requires an actual original SwimmingPool:Indoor object.
// topology_incomplete means its source/plant applicability is unresolved, not
// that an arbitrary foreign demand may be relabeled as a Pool. A different
// unknown demand requires its own inventory evidence/contract at the caller.
type energyPathServiceBoundaryRestriction struct {
	Reason            string   `json:"reason"`
	EndUse            string   `json:"endUse"`
	ServiceKind       string   `json:"serviceKind"`
	Carrier           string   `json:"carrier"`
	PlantLoopName     string   `json:"plantLoopName,omitempty"`
	DemandObjectType  string   `json:"demandObjectType"`
	DemandObjectName  string   `json:"demandObjectName,omitempty"`
	ConsumerSourceIDs []string `json:"consumerSourceIds,omitempty"`
	MeterSourceIDs    []string `json:"meterSourceIds,omitempty"`
}

func energyPathBoundaryToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func energyPathBoundarySQLSourceIDValid(id string) bool {
	if !strings.HasPrefix(id, "sql-rdd-") {
		return false
	}
	part := strings.TrimPrefix(id, "sql-rdd-")
	number, err := strconv.ParseInt(part, 10, 64)
	return err == nil && number > 0 && strconv.FormatInt(number, 10) == part
}

func energyPathBoundarySourceIDs(input []string) []string {
	seen := map[string]bool{}
	for _, id := range input {
		// Keep even an empty/invalid token until validation: silently dropping
		// malformed metadata could turn an explicit restriction into absence.
		seen[energyPathBoundaryToken(id)] = true
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func normalizeEnergyPathServiceBoundaryRestriction(input energyPathServiceBoundaryRestriction) energyPathServiceBoundaryRestriction {
	out := input
	out.Reason = energyPathBoundaryToken(input.Reason)
	out.EndUse = energyPathBoundaryToken(input.EndUse)
	out.ServiceKind = energyPathBoundaryToken(input.ServiceKind)
	out.Carrier = energyPathBoundaryToken(input.Carrier)
	out.PlantLoopName = energyPathBoundaryToken(input.PlantLoopName)
	out.DemandObjectType = energyPathBoundaryToken(input.DemandObjectType)
	out.DemandObjectName = energyPathBoundaryToken(input.DemandObjectName)
	out.ConsumerSourceIDs = energyPathBoundarySourceIDs(input.ConsumerSourceIDs)
	out.MeterSourceIDs = energyPathBoundarySourceIDs(input.MeterSourceIDs)
	return out
}

func cloneEnergyPathServiceBoundaryRestrictions(input []energyPathServiceBoundaryRestriction) []energyPathServiceBoundaryRestriction {
	if input == nil {
		return nil
	}
	out := append([]energyPathServiceBoundaryRestriction{}, input...)
	for index := range out {
		out[index].ConsumerSourceIDs = append([]string(nil), input[index].ConsumerSourceIDs...)
		out[index].MeterSourceIDs = append([]string(nil), input[index].MeterSourceIDs...)
	}
	return out
}

func energyPathServiceBoundaryRestrictionKey(input energyPathServiceBoundaryRestriction) string {
	// Encoding a tuple avoids delimiter collisions even in invalid metadata.
	data, _ := json.Marshal([]string{input.Reason, input.EndUse, input.ServiceKind, input.Carrier, input.PlantLoopName, input.DemandObjectType, input.DemandObjectName})
	return string(data)
}

func unionEnergyPathServiceBoundaryRestrictions(inputs ...[]energyPathServiceBoundaryRestriction) []energyPathServiceBoundaryRestriction {
	byKey := map[string]energyPathServiceBoundaryRestriction{}
	for _, input := range inputs {
		for _, item := range input {
			next := normalizeEnergyPathServiceBoundaryRestriction(item)
			key := energyPathServiceBoundaryRestrictionKey(next)
			if previous, exists := byKey[key]; exists {
				next.ConsumerSourceIDs = energyPathBoundarySourceIDs(append(previous.ConsumerSourceIDs, next.ConsumerSourceIDs...))
				next.MeterSourceIDs = energyPathBoundarySourceIDs(append(previous.MeterSourceIDs, next.MeterSourceIDs...))
			}
			byKey[key] = next
		}
	}
	if len(byKey) == 0 {
		return nil
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]energyPathServiceBoundaryRestriction, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func validateEnergyPathServiceBoundaryRestriction(input energyPathServiceBoundaryRestriction) error {
	r := normalizeEnergyPathServiceBoundaryRestriction(input)
	if r.Reason != energyPathBoundaryNonZoneDemand && r.Reason != energyPathBoundaryTopologyIncomplete {
		return fmt.Errorf("unknown service boundary restriction reason")
	}
	if r.ServiceKind != "heating" && r.ServiceKind != "cooling" || r.EndUse != r.ServiceKind && r.EndUse != "pumps" {
		return fmt.Errorf("service boundary restriction has conflicting service/end use")
	}
	reviewedCarrier := r.Carrier == "electricity" || r.Carrier == "natural_gas"
	topologyOnlyCarrier := r.Reason == energyPathBoundaryTopologyIncomplete && r.Carrier != "" && canonicalEnergyPathPart(r.Carrier) == r.Carrier && len(r.ConsumerSourceIDs) == 0
	if !reviewedCarrier && !topologyOnlyCarrier || r.EndUse == "pumps" && r.Carrier != "electricity" {
		return fmt.Errorf("unreviewed pool service carrier")
	}
	if r.DemandObjectType != "swimmingpool:indoor" {
		return fmt.Errorf("service boundary needs the exact native non-Zone demand type")
	}
	if r.DemandObjectName == "" {
		return fmt.Errorf("service boundary needs an actual original Pool identity")
	}
	if r.Reason == energyPathBoundaryNonZoneDemand && r.PlantLoopName == "" {
		return fmt.Errorf("resolved ownership restriction lacks original plant/demand identity")
	}
	if len(r.ConsumerSourceIDs)+len(r.MeterSourceIDs) == 0 && r.Reason != energyPathBoundaryTopologyIncomplete {
		return fmt.Errorf("service boundary has no observed source reference")
	}
	seen := map[string]bool{}
	for _, ids := range [][]string{r.ConsumerSourceIDs, r.MeterSourceIDs} {
		for _, id := range ids {
			if !energyPathBoundarySQLSourceIDValid(id) || seen[id] {
				return fmt.Errorf("invalid or cross-role native pool source reference")
			}
			seen[id] = true
		}
	}
	return nil
}

func energyPathServiceBoundaryEndUse(node EnergyExplanationNode) string {
	if !strings.EqualFold(node.Level, "energy") && !strings.EqualFold(node.Level, "end_use") || legacyEnergyNodeIsCarrier(node) || legacyEnergyNodeIsSupport(node) {
		return ""
	}
	return canonicalEnergyPathEndUse(firstNonEmpty(node.EndUse, energyExplanationKindSuffix(node.Kind)))
}

// One deny predicate for all graph and summary consumers. Invalid explicit
// restrictions still deny conversion; validation errors must not become an
// empty slice. Only original/source validation may create positive authority.
func energyPathServiceBoundaryDeniesConversion(node EnergyExplanationNode) bool {
	endUse := energyPathServiceBoundaryEndUse(node)
	return (endUse == "heating" || endUse == "cooling") && len(node.serviceBoundaryRestrictions) > 0
}

// The caller supplies one scope's qualified legacy contributors BEFORE value
// pruning. No positive-value or source-knownness test is permitted here: zero
// and unknown restricted contributors must survive canonical metadata merging.
func collectEnergyPathServiceBoundaryCensus(nodes []EnergyExplanationNode) map[string][]energyPathServiceBoundaryRestriction {
	out := map[string][]energyPathServiceBoundaryRestriction{}
	for _, node := range nodes {
		endUse := energyPathServiceBoundaryEndUse(node)
		if endUse == "" || len(node.serviceBoundaryRestrictions) == 0 {
			continue
		}
		out[endUse] = unionEnergyPathServiceBoundaryRestrictions(out[endUse], node.serviceBoundaryRestrictions)
	}
	return out
}

func collectEnergyPathServiceBoundaryCensusForScope(nodes []EnergyExplanationNode, edges []EnergyExplanationEdge, scope EnergyExplanationScope, allocationPolicy string) map[string][]energyPathServiceBoundaryRestriction {
	for _, node := range nodes {
		if len(node.serviceBoundaryRestrictions) > 0 {
			return collectEnergyPathServiceBoundaryCensus(scopedEnergyExplanationLegacyNodes(nodes, edges, scope, allocationPolicy))
		}
	}
	// Do not rebuild allocation projections merely to prove an empty census in
	// existing models. Return a writable map for later restricted-only months.
	return map[string][]energyPathServiceBoundaryRestriction{}
}

func applyEnergyPathServiceBoundaryCensus(nodes []EnergyExplanationNode, census map[string][]energyPathServiceBoundaryRestriction) []EnergyExplanationNode {
	if len(census) == 0 {
		return nodes
	}
	out := append([]EnergyExplanationNode(nil), nodes...)
	for index := range out {
		endUse := energyPathServiceBoundaryEndUse(out[index])
		out[index].serviceBoundaryRestrictions = unionEnergyPathServiceBoundaryRestrictions(out[index].serviceBoundaryRestrictions, census[endUse])
	}
	return out
}

// Call before an annual-fallback same-ID skip. Preserve monthly numerical/source
// precedence exactly; transfer only restriction metadata from an annual node.
func mergeEnergyPathServiceBoundaryNodeMetadata(kept, fallback []EnergyExplanationNode) []EnergyExplanationNode {
	byID := map[string][]energyPathServiceBoundaryRestriction{}
	for _, node := range fallback {
		if node.ID != "" && len(node.serviceBoundaryRestrictions) > 0 {
			byID[node.ID] = unionEnergyPathServiceBoundaryRestrictions(byID[node.ID], node.serviceBoundaryRestrictions)
		}
	}
	if len(byID) == 0 {
		return kept
	}
	out := append([]EnergyExplanationNode(nil), kept...)
	for index := range out {
		out[index].serviceBoundaryRestrictions = unionEnergyPathServiceBoundaryRestrictions(out[index].serviceBoundaryRestrictions, byID[out[index].ID])
	}
	return out
}

// Use actual compiled consuming-source IDs of an exact projection, NOT its
// entire SourceIDs trace (which can also contain the shared broad meter).
// Missing/invalid consumer proof keeps the restriction. A broad-meter-only
// trace cannot prove that CW consumption is disjoint from an HW restriction.
func projectEnergyPathServiceBoundaryRestrictions(input []energyPathServiceBoundaryRestriction, consumerSourceIDs []string, exactConsumers bool) []energyPathServiceBoundaryRestriction {
	if !exactConsumers || len(consumerSourceIDs) == 0 {
		return unionEnergyPathServiceBoundaryRestrictions(input)
	}
	selected := map[string]bool{}
	for _, id := range consumerSourceIDs {
		id = energyPathBoundaryToken(id)
		if !energyPathBoundarySQLSourceIDValid(id) || selected[id] {
			return unionEnergyPathServiceBoundaryRestrictions(input)
		}
		selected[id] = true
	}
	out := []energyPathServiceBoundaryRestriction{}
	for _, r := range unionEnergyPathServiceBoundaryRestrictions(input) {
		keep := validateEnergyPathServiceBoundaryRestriction(r) != nil || len(r.ConsumerSourceIDs) == 0
		for _, id := range r.MeterSourceIDs {
			keep = keep || selected[id]
		}
		for _, id := range r.ConsumerSourceIDs {
			keep = keep || selected[id]
		}
		if keep {
			out = append(out, r)
		}
	}
	return unionEnergyPathServiceBoundaryRestrictions(out)
}

// The canonical target is authoritative. Omitting a restricted source from a
// link's trace cannot bypass an aggregate whose denominator includes it.
func filterEnergyPathServiceBoundaryConversions(nodes []EnergyExplanationNode, links []EnergyPathLink) []EnergyPathLink {
	denied := map[string]bool{}
	for _, node := range nodes {
		if energyPathServiceBoundaryDeniesConversion(node) {
			denied[node.ID] = true
		}
	}
	if len(denied) == 0 {
		return links
	}
	if links == nil {
		return nil
	}
	out := make([]EnergyPathLink, 0, len(links))
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(link.Relation), "load_to_end_use") && denied[link.ToID] {
			continue
		}
		link.SourceIDs = append([]string(nil), link.SourceIDs...)
		link.RelatedPathIDs = append([]string(nil), link.RelatedPathIDs...)
		out = append(out, link)
	}
	return out
}

// Apply after legacy DerivedKPIs aliases and at cached-summary return sites.
// No load, end-use, carrier, driver, completeness or energy quantity is rebuilt.
func filterEnergyPathServiceBoundarySummary(summary EnergyExplanationSummary, nodes []EnergyExplanationNode) EnergyExplanationSummary {
	deniedServices, deniedEndpoints := map[string]bool{}, map[string]bool{}
	for _, node := range nodes {
		if energyPathServiceBoundaryDeniesConversion(node) {
			deniedServices[energyPathServiceBoundaryEndUse(node)] = true
			deniedEndpoints[node.ID] = true
		}
	}
	if len(deniedServices) == 0 {
		return summary
	}
	filter := func(items []EnergyExplanationSummaryItem) []EnergyExplanationSummaryItem {
		if items == nil {
			return nil
		}
		out := make([]EnergyExplanationSummaryItem, 0, len(items))
		for _, item := range items {
			service := energyPathBoundaryToken(item.ServiceKind)
			blocked := deniedServices[service] || deniedEndpoints[item.DenominatorLabel]
			// These are exact legacy KPI identities, not labels/UI text.
			// A contradictory ID must not override an already denied service.
			for _, key := range []string{item.Kind, item.ID} {
				switch key {
				case "kpi.heating_cop":
					blocked = blocked || deniedServices["heating"]
				case "kpi.cooling_cop":
					blocked = blocked || deniedServices["cooling"]
				}
			}
			if blocked {
				continue
			}
			item.SourceIDs = append([]string(nil), item.SourceIDs...)
			out = append(out, item)
		}
		return out
	}
	summary.Ratios = filter(summary.Ratios)
	summary.DerivedKPIs = filter(summary.DerivedKPIs)
	return summary
}

func mergeEnergyPathServiceBoundaryCensus(target map[string][]energyPathServiceBoundaryRestriction, incoming map[string][]energyPathServiceBoundaryRestriction) {
	for endUse, restrictions := range incoming {
		target[endUse] = unionEnergyPathServiceBoundaryRestrictions(target[endUse], restrictions)
	}
}

// A period with only a zero/unknown restricted contributor may have no visible
// end-use node at all. Keep its scope-qualified metadata in the annual census
// without adding a dummy zero node or changing monthly quantity precedence.
func applyEnergyPathServiceBoundaryAnnualCensus(result *EnergyExplanationResult, census map[string][]energyPathServiceBoundaryRestriction) {
	if result == nil || len(census) == 0 {
		return
	}
	result.Nodes = applyEnergyPathServiceBoundaryCensus(result.Nodes, census)
	result.Periods = append([]EnergyPeriod(nil), result.Periods...)
	for index := range result.Periods {
		period := &result.Periods[index]
		if strings.EqualFold(period.Kind, "annual") || strings.EqualFold(period.ID, "annual") {
			period.Nodes = applyEnergyPathServiceBoundaryCensus(period.Nodes, census)
		}
	}
}

func filterEnergyPathServiceBoundaryPeriod(period EnergyPeriod) (EnergyPeriod, bool) {
	links := filterEnergyPathServiceBoundaryConversions(period.Nodes, period.Links)
	changed := len(links) != len(period.Links)
	period.Links = links
	if period.Summary != nil {
		summary := filterEnergyPathServiceBoundarySummary(*period.Summary, period.Nodes)
		if len(summary.Ratios) != len(period.Summary.Ratios) || len(summary.DerivedKPIs) != len(period.Summary.DerivedKPIs) {
			changed = true
			period.Summary = &summary
		}
	}
	return period, changed
}

// Last semantic gate after allocation and annual overrides and for cached or
// stored results. Every scope is filtered against its own qualified nodes;
// a Building restriction never leaks into an excluded/disjoint Zone scope.
func filterEnergyPathServiceBoundaryResult(result *EnergyExplanationResult) bool {
	if result == nil {
		return false
	}
	links := filterEnergyPathServiceBoundaryConversions(result.Nodes, result.Links)
	changed := len(links) != len(result.Links)
	result.Links = links
	periods := result.Periods
	periodsCopied := false
	for index, period := range result.Periods {
		filtered, periodChanged := filterEnergyPathServiceBoundaryPeriod(period)
		if periodChanged {
			if !periodsCopied {
				periods = append([]EnergyPeriod(nil), result.Periods...)
				periodsCopied = true
			}
			periods[index] = filtered
			changed = true
		}
	}
	result.Periods = periods
	zones := result.ZoneResults
	zonesCopied := false
	for index, zone := range result.ZoneResults {
		zoneLinks := filterEnergyPathServiceBoundaryConversions(zone.Nodes, zone.Links)
		zoneSummary := filterEnergyPathServiceBoundarySummary(zone.Summary, zone.Nodes)
		zoneChanged := len(zoneLinks) != len(zone.Links) || len(zoneSummary.Ratios) != len(zone.Summary.Ratios) || len(zoneSummary.DerivedKPIs) != len(zone.Summary.DerivedKPIs)
		zone.Links, zone.Summary = zoneLinks, zoneSummary
		periodsCopied := false
		for periodIndex, period := range zone.Periods {
			filtered, periodChanged := filterEnergyPathServiceBoundaryPeriod(period)
			if periodChanged {
				if !periodsCopied {
					zone.Periods = append([]EnergyPeriod(nil), zone.Periods...)
					periodsCopied = true
				}
				zone.Periods[periodIndex] = filtered
				zoneChanged = true
			}
		}
		if zoneChanged {
			if !zonesCopied {
				zones = append([]EnergyExplanationZoneResult(nil), result.ZoneResults...)
				zonesCopied = true
			}
			zones[index] = zone
			changed = true
		}
	}
	result.ZoneResults = zones
	return changed
}
