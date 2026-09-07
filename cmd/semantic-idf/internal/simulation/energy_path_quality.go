package simulation

import (
	"fmt"
	"math"
	"strings"
)

// BuildEnergyPathQuality separates run-level requested-output availability from
// selected-period graph accounting. Sources do not contain period availability;
// finding one must never claim that every month contains that observation.
func BuildEnergyPathQuality(result EnergyExplanationResult, period string) *EnergyPathQuality {
	period = strings.TrimSpace(period)
	nodes, links, rows := result.Nodes, result.Links, result.Reconciliation
	selected := period == ""
	for _, candidate := range result.Periods {
		if period != "" && strings.EqualFold(candidate.ID, period) {
			nodes, links, rows = candidate.Nodes, candidate.Links, candidate.Reconciliation
			selected = true
			break
		}
	}
	graphPeriod := energyPathQualityGraphPeriod(nodes, rows)
	if !selected {
		selected = strings.EqualFold(graphPeriod, period) || (strings.EqualFold(period, "annual") && graphPeriod == "")
		if !selected {
			nodes, links, rows = nil, nil, nil
		}
	}
	if period == "" {
		period = graphPeriod
	}
	quality := &EnergyPathQuality{
		Drivers: energyPathQualityThermalLevel(result.Completeness.HeatDrivers, "driver", "Drivers", result.Nodes),
		Loads:   energyPathQualityThermalLevel(result.Completeness.DeliveredLoad, "load", "Loads", result.Nodes),
	}
	quality.EndUses = energyPathQualitySiteLevel(result, "end_use", "End uses")
	quality.Carriers = energyPathQualitySiteLevel(result, "carrier", "Carriers")
	quality.DriverToLoadClosedPct, quality.DriverToLoadStatus = energyPathQualityClosure(nodes, links, "load", quality.Loads.Status)
	quality.EndUseToCarrierClosedPct, quality.EndUseToCarrierStatus = energyPathQualityClosure(nodes, links, "carrier", quality.Carriers.Status)
	quality.Ratios = energyPathQualityRatios(nodes, links, result.Sources, quality.Loads.Status)
	quality.ZoneAllocatedPct, quality.UnassignedPct, quality.ZoneAllocationStatus = energyPathQualityZoneAllocation(rows, period)
	if !selected {
		quality.DriverToLoadStatus, quality.EndUseToCarrierStatus = "unavailable", "unavailable"
		quality.Ratios = EnergyCompletenessLevel{Level: "ratio", Status: "unavailable", Message: "The requested period has no graph; annual values are not substituted."}
	}
	return quality
}

func energyPathQualityGraphPeriod(nodes []EnergyExplanationNode, rows []EnergyReconciliation) string {
	periods := map[string]bool{}
	for _, node := range nodes {
		if period := strings.ToLower(strings.TrimSpace(node.Period)); period != "" {
			periods[period] = true
		}
	}
	if len(periods) == 0 {
		for _, row := range rows {
			periods[strings.ToLower(firstNonEmpty(strings.TrimSpace(row.Period), "annual"))] = true
		}
	}
	if len(periods) == 1 {
		for period := range periods {
			return period
		}
	}
	return ""
}

func energyPathQualityThermalLevel(input EnergyCompletenessLevel, level, label string, nodes []EnergyExplanationNode) EnergyCompletenessLevel {
	input.Level = level
	if input.Status != "" {
		input.Message = label + ": run-level requested-output availability. " + input.Message
		return input
	}
	for _, node := range nodes {
		if node.Level == level && node.ScaleDomain == "thermal" && node.Basis != "residual" {
			input.Found++
		}
	}
	input.Status = "unavailable"
	input.Message = label + ": requested-output coverage is unavailable in this stored result."
	if input.Found > 0 {
		input.Status = "partial"
	}
	return input
}

// Facility totals and carrier-qualified end uses are separate source groups.
// Alias alternatives count once; supply and raw water never enter either stage.
func energyPathQualitySiteLevel(result EnergyExplanationResult, level, label string) EnergyCompletenessLevel {
	expected, found := map[string]bool{}, map[string]bool{}
	notRequested, notApplicable := false, false
	for _, entry := range result.Completeness.SourceAvailability {
		if entry.Level != "energy" || energyExplanationNameIsWaterMeter(entry.Name) || energyExplanationNameIsSupplyContext(entry.Name) {
			continue
		}
		entryLevel, group := energyPathQualityEnergyGroup(entry.Name)
		if entryLevel != "" && entryLevel != level {
			continue
		}
		if entry.Status == "not_requested" || entry.Status == "not_applicable" {
			// Only the actual whole-stage sentinel applies to both site
			// stages. An unknown resource name is not plan-level evidence.
			if entryLevel == "" && strings.TrimSpace(entry.Name) != "" && !strings.EqualFold(strings.TrimSpace(entry.Name), "not requested by current output plan") {
				continue
			}
			notRequested = notRequested || entry.Status == "not_requested"
			notApplicable = notApplicable || entry.Status == "not_applicable"
			continue
		}
		if entryLevel != level || group == "" {
			continue
		}
		expected[group] = true
		if entry.Status == "found" {
			found[group] = true
		}
	}
	out := energyCompletenessLevel(level, len(found), len(expected), label)
	if len(expected) == 0 {
		out.Status = "unavailable"
		switch {
		case notRequested:
			out.Status = "not_requested"
		case notApplicable:
			out.Status = "not_applicable"
		case result.Completeness.EnergyUse.Status == "not_requested" || result.Completeness.EnergyUse.Status == "not_applicable":
			out.Status = result.Completeness.EnergyUse.Status
		default:
			for _, source := range result.Sources {
				if entryLevel, group := energyPathQualityEnergyGroup(firstNonEmpty(source.Name, source.KeyValue)); entryLevel == level && group != "" {
					found[group] = true
				}
			}
			out.Found = len(found)
			for _, node := range result.Nodes {
				if node.Level == level && node.ScaleDomain == "site" {
					out.Status = "partial"
				}
			}
			if out.Found > 0 {
				out.Status = "partial"
			} else if result.Completeness.EnergyUse.Status == "missing" {
				out.Status = "missing"
			}
		}
		out.Message = label + ": requested-output coverage is unavailable in this stored result."
		if out.Status == "not_requested" || out.Status == "not_applicable" {
			out.Message = label + ": not requested or not applicable according to the output-plan metadata."
		}
	}
	if level == "carrier" && out.Status != "not_requested" && out.Status != "not_applicable" {
		for _, node := range result.Nodes {
			if node.Level == "carrier" && node.ScaleDomain == "site" && !energyPathQualityReportedCarrier(node) {
				out.Status = "partial"
				out.Message = "Carriers: observed/allocated subtotals do not establish complete carrier output coverage."
				break
			}
		}
	}
	out.Message = "Run-level requested-output availability. " + out.Message
	return out
}

func energyPathQualityEnergyGroup(name string) (string, string) {
	definition, ok := energyMeterAliasDefinitionForName(name)
	if !ok {
		definition, ok = energyVariableAliasDefinitionForName(name)
	}
	if !ok {
		definition, ok = energyMeterEndUseCarrierDefinitionForName(name)
	}
	if !ok || canonicalEnergyPathCarrier(definition.Carrier) == "water" || energyExplanationIsSupportEndUse(energyExplanationSeries{Level: "energy", Kind: definition.Kind, EndUse: definition.EndUse}) {
		return "", ""
	}
	level := "end_use"
	if definition.FacilityTotal || definition.HierarchyLevel == "facility_total" {
		level = "carrier"
	}
	return level, energyMeterDefinitionGroupKey(definition)
}

func energyPathQualityReportedCarrier(node EnergyExplanationNode) bool {
	if node.Level != "carrier" || node.ScaleDomain != "site" || node.ZoneName != "" {
		return false
	}
	basis := canonicalEnergyPathBasis(node.Basis, "")
	return basis != "reported_end_use_subtotal" && basis != "direct_zone_energy" && energyPathAllocationBasisRank(basis) == 0 &&
		!strings.EqualFold(node.MeterHierarchyLevel, "observed_end_use_subtotal") && !strings.EqualFold(node.MeterHierarchyLevel, "zone_direct_subtotal") &&
		(strings.EqualFold(node.MeterHierarchyLevel, "facility_total") || basis == "reported_meter" || basis == "reported_variable" || basis == "integrated_rate")
}

// Closure is 100 * max(0, 1 - sum(abs(expected-incoming))/sum(expected)).
// Absolute errors are taken per endpoint before summing, so excess and deficit
// cannot cancel across services or carriers. Residual display nodes do not count.
func energyPathQualityClosure(nodes []EnergyExplanationNode, links []EnergyPathLink, level, availability string) (float64, string) {
	byID := make(map[string]EnergyExplanationNode, len(nodes))
	expected, incoming := map[string]float64{}, map[string]float64{}
	partialSubtotal := false
	for _, node := range nodes {
		byID[node.ID] = node
		if node.Level != level {
			continue
		}
		if level == "carrier" && !energyPathQualityReportedCarrier(node) {
			partialSubtotal = partialSubtotal || node.ScaleDomain == "site"
			continue
		}
		if level == "load" && node.ScaleDomain != "thermal" {
			continue
		}
		nodeValue := energyExplanationEffectiveNodeValue(node)
		if level == "carrier" {
			// Match canonical carrier reconciliation, not retained raw/effective
			// source context which may cover a broader observation.
			nodeValue = math.Abs(node.Value)
		}
		value, ok := energyPathQualityEnergyValue(nodeValue, node.Unit)
		if ok {
			expected[node.ID] = value
		}
	}
	for _, link := range links {
		if _, ok := expected[link.ToID]; !ok {
			continue
		}
		from, to := byID[link.FromID], byID[link.ToID]
		if level == "load" {
			service := energyCanonicalServiceKind(firstNonEmpty(to.ServiceKind, energyExplanationKindSuffix(to.Kind)))
			if link.Relation != "driver_to_load" || from.Level != "driver" || from.ScaleDomain != "thermal" ||
				(service != "cooling" && service != "heating") || energyCanonicalServiceKind(from.ServiceKind) != service || !strings.EqualFold(from.ZoneName, to.ZoneName) {
				continue
			}
		} else if (link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier") || from.Level != "end_use" || from.ScaleDomain != "site" {
			continue
		}
		value, ok := energyPathQualityEnergyValue(link.ToValue, link.ToUnit)
		fromValue, fromOK := energyPathQualityEnergyValue(link.FromValue, link.FromUnit)
		if ok && fromOK && math.Abs(fromValue-value) <= 1e-6*math.Max(1, value) {
			incoming[link.ToID] += value
		}
	}
	total, errorSum := 0.0, 0.0
	overmapped := false
	for id, value := range expected {
		total += value
		errorSum += math.Abs(value - incoming[id])
		overmapped = overmapped || incoming[id] > value+1e-6*math.Max(1, value)
	}
	if total == 0 {
		if partialSubtotal {
			return 0, "partial"
		}
		if availability == "not_requested" || availability == "not_applicable" {
			return 0, availability
		}
		return 0, "unavailable"
	}
	pct := roundedEnergyNumber(100 * math.Max(0, 1-errorSum/total))
	status := "complete"
	if partialSubtotal || errorSum > 1e-6*math.Max(1, total) {
		status = "partial"
	}
	if overmapped {
		status = "overmapped"
	}
	return pct, status
}

func energyPathQualityEnergyValue(value float64, unit string) (float64, bool) {
	if !energyPathFinite(value) || value < 0 {
		return 0, false
	}
	base := energyPathConversionUnitBase(unit)
	factor, ok := energyPathEnergyUnitNormalization(base)
	return value * factor, ok && base != ""
}

func energyPathQualityRatios(nodes []EnergyExplanationNode, links []EnergyPathLink, sources []EnergyDataSource, loadStatus string) EnergyCompletenessLevel {
	if loadStatus == "not_requested" || loadStatus == "not_applicable" {
		return EnergyCompletenessLevel{Level: "ratio", Status: loadStatus, Message: "Load-to-end-use ratios were not requested by this output plan."}
	}
	byID := map[string]EnergyExplanationNode{}
	candidates, found, knownSources, carrierTrace := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, source := range sources {
		knownSources[source.ID] = source.ID != ""
	}
	hasTrace := func(ids []string) bool {
		for _, id := range ids {
			if knownSources[id] {
				return true
			}
		}
		return false
	}
	keyFor := func(node EnergyExplanationNode) string {
		service := energyCanonicalServiceKind(firstNonEmpty(node.ServiceKind, node.EndUse, energyExplanationKindSuffix(node.Kind)))
		if service != "cooling" && service != "heating" {
			return ""
		}
		return strings.ToLower(strings.TrimSpace(node.ZoneName)) + "|" + service
	}
	for _, node := range nodes {
		byID[node.ID] = node
		if (node.Level == "load" && node.ScaleDomain == "thermal") || (node.Level == "end_use" && node.ScaleDomain == "site") {
			if key := keyFor(node); key != "" {
				candidates[key] = true
			}
		}
	}
	for _, link := range links {
		from, to := byID[link.FromID], byID[link.ToID]
		if (link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier") && from.Level == "end_use" && to.Level == "carrier" &&
			from.ScaleDomain == "site" && to.ScaleDomain == "site" && energyPathPositiveFinite(link.ToValue) && hasTrace(link.SourceIDs) {
			carrierTrace[from.ID] = true
		}
	}
	for _, link := range links {
		from, to := byID[link.FromID], byID[link.ToID]
		if link.Relation != "load_to_end_use" || from.Level != "load" || to.Level != "end_use" || from.ScaleDomain != "thermal" || to.ScaleDomain != "site" ||
			!energyPathConversionValuesValid(link) || !energyPathPositiveFinite(link.Ratio) || link.RatioKind == "" || !hasTrace(link.SourceIDs) || !carrierTrace[to.ID] {
			continue
		}
		if key := keyFor(from); key != "" && key == keyFor(to) {
			found[key] = true
		}
	}
	out := energyCompletenessLevel("ratio", len(found), len(candidates), "Load-to-end-use ratios")
	out.Message = fmt.Sprintf("Selected-period ratio availability: %d/%d cooling/heating service paths have a valid, source-traced ratio; thermal and site energy are not a conservation boundary.", out.Found, out.Total)
	return out
}

// Allocation rows express building-wide zone coverage, including when copied
// into a selected zone. Direct + allocated is the assigned fraction; explicit
// unassigned values retain non-cancelling monthly gaps in annual accounting.
func energyPathQualityZoneAllocation(rows []EnergyReconciliation, period string) (float64, float64, string) {
	if period == "" {
		return 0, 0, "unavailable"
	}
	expected, assigned, unassigned := 0.0, 0.0, 0.0
	status := "complete"
	seen := map[string]bool{}
	for _, row := range rows {
		id := strings.ToLower(strings.TrimSpace(row.ID))
		if row.Level != "allocation" || !strings.EqualFold(firstNonEmpty(row.Period, "annual"), period) || seen[id] ||
			(!strings.HasPrefix(id, "reconcile.zone_hvac_allocation.") && !strings.HasPrefix(id, "reconcile.zone_auxiliary_allocation.")) {
			continue
		}
		value, ok := energyPathQualityEnergyValue(row.ExpectedValue, row.Unit)
		direct, directOK := energyPathQualityEnergyValue(row.DirectValue, row.Unit)
		allocated, allocatedOK := energyPathQualityEnergyValue(row.AllocatedValue, row.Unit)
		missing, missingOK := energyPathQualityEnergyValue(row.UnassignedValue, row.Unit)
		if !ok || !directOK || !allocatedOK || !missingOK {
			continue
		}
		seen[id] = true
		expected += value
		assigned += direct + allocated
		unassigned += missing
		if row.Status == "overmapped" || row.OvermappedValue > 1e-6 || direct+allocated > value+1e-6 {
			status = "overmapped"
		} else if status != "overmapped" && (row.Status == "partial" || missing > 1e-6 || math.Abs(value-direct-allocated) > 1e-6) {
			status = "partial"
		}
	}
	if expected <= 0 {
		return 0, 0, "unavailable"
	}
	return roundedEnergyNumber(100 * assigned / expected), roundedEnergyNumber(100 * unassigned / expected), status
}
