package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

func energyPathHasStorageChargeBoundary(result EnergyExplanationResult) bool {
	if len(energyPathStorageChargeNodeBoundaries(result.Nodes)) > 0 {
		return true
	}
	for _, period := range result.Periods {
		if len(energyPathStorageChargeNodeBoundaries(period.Nodes)) > 0 {
			return true
		}
	}
	for _, zone := range result.ZoneResults {
		if len(energyPathStorageChargeNodeBoundaries(zone.Nodes)) > 0 {
			return true
		}
		for _, period := range zone.Periods {
			if len(energyPathStorageChargeNodeBoundaries(period.Nodes)) > 0 {
				return true
			}
		}
	}
	return false
}

// The writer receives a value copy, but its slices still alias the caller.
// Clone before the existing stored-graph sanitation rebuilds accounting.
func energyPathStorageChargeResultForWrite(result EnergyExplanationResult) EnergyExplanationResult {
	if !energyPathHasStorageChargeBoundary(result) {
		return result
	}
	clonePeriods := func(input []EnergyPeriod) []EnergyPeriod {
		out := append([]EnergyPeriod(nil), input...)
		for index := range out {
			out[index].Nodes = cloneEnergyExplanationNodes(out[index].Nodes)
			out[index].Links = append([]EnergyPathLink(nil), out[index].Links...)
			out[index].Warnings = append([]EnergyWarning(nil), out[index].Warnings...)
		}
		return out
	}
	result.Nodes = cloneEnergyExplanationNodes(result.Nodes)
	result.Links = append([]EnergyPathLink(nil), result.Links...)
	result.Warnings = append([]EnergyWarning(nil), result.Warnings...)
	result.Periods = clonePeriods(result.Periods)
	result.ZoneResults = append([]EnergyExplanationZoneResult(nil), result.ZoneResults...)
	for index := range result.ZoneResults {
		zone := &result.ZoneResults[index]
		zone.Nodes = cloneEnergyExplanationNodes(zone.Nodes)
		zone.Links = append([]EnergyPathLink(nil), zone.Links...)
		zone.Warnings = append([]EnergyWarning(nil), zone.Warnings...)
		zone.Periods = clonePeriods(zone.Periods)
	}
	sanitizeEnergyExplanationV2Result(&result)
	return result
}

// A display-zero/unknown constituent has no additive node. If a charge
// context is visible for this period, retain its deny-only record there so
// the subsequent canonical merge cannot lose a separate AC/DC key. Neither
// values nor numerical SourceIDs are added by this metadata-only union.
func carryEnergyPathStorageChargePrunedMetadata(nodes []EnergyExplanationNode, series []energyExplanationSeries, valueFor func(energyExplanationSeries) float64) []EnergyExplanationNode {
	var pruned []energyPathStorageChargeBoundary
	for _, item := range series {
		if item.storageChargeBoundary != nil && roundedEnergyNumber(valueFor(item)) == 0 {
			pruned = unionEnergyPathStorageChargeBoundaries(pruned, []energyPathStorageChargeBoundary{*item.storageChargeBoundary})
		}
	}
	if len(pruned) == 0 {
		return nodes
	}
	for index := range nodes {
		if len(nodes[index].storageChargeBoundaries) > 0 {
			nodes[index].storageChargeBoundaries = unionEnergyPathStorageChargeBoundaries(nodes[index].storageChargeBoundaries, pruned)
			break
		}
	}
	return nodes
}

func energyPathStorageChargeNodeWarnings(warnings []EnergyWarning, nodes []EnergyExplanationNode, period string) []EnergyWarning {
	for _, node := range nodes {
		for _, boundary := range node.storageChargeBoundaries {
			items := energyPathStorageChargeWarnings([]energyExplanationSeries{{storageChargeBoundary: &boundary}})
			for _, warning := range items {
				warning.Period = period
				warnings = appendEnergyDriverWarning(warnings, warning)
			}
		}
	}
	return warnings
}

// A standalone period also passes through this gate, not only the outer result.
// A cached summary is rebuilt from clean nodes/links, never by subtracting an
// assumed charge share from canonical Other.
func filterEnergyPathStorageChargePeriod(period EnergyPeriod) (EnergyPeriod, bool) {
	boundaries := energyPathStorageChargeNodeBoundaries(period.Nodes)
	if len(boundaries) == 0 {
		return period, false
	}
	before := period
	period.Edges = filterEnergyPathStorageChargeLegacyEdges(period.Edges, boundaries)
	period.Links = filterEnergyPathStorageChargeLinks(period.Links, boundaries)
	period.Warnings = energyPathStorageChargeNodeWarnings(append([]EnergyWarning(nil), period.Warnings...), period.Nodes, period.ID)
	if period.Summary != nil {
		old := period.Summary
		period.Nodes, period.Links, period.Reconciliation, _ = sanitizeEnergyExplanationV2Graph(period.Nodes, period.Links, period.Reconciliation, nil, period.ID, old.Scope)
		summary := buildEnergyExplanationSummaryV2(EnergyExplanationResult{
			Schema: energyExplanationSchema, Scope: old.Scope, AllocationPolicy: old.AllocationPolicy,
			Nodes: period.Nodes, Links: period.Links, Reconciliation: period.Reconciliation,
			Completeness: old.Completeness, Warnings: period.Warnings, ZoneContributions: period.ZoneContributions,
		})
		summary.Period = period.ID
		period.Summary = &summary
	}
	return period, !reflect.DeepEqual(before, period)
}

// Boundary identity belongs to the original charge node, never to an arbitrary
// source-ID intersection with a merged Other node.
func energyPathStorageChargeNodeBoundaries(nodes []EnergyExplanationNode) map[string]*energyPathStorageChargeBoundary {
	var out map[string]*energyPathStorageChargeBoundary
	for _, node := range nodes {
		if len(node.storageChargeBoundaries) == 0 {
			continue
		}
		if out == nil {
			out = map[string]*energyPathStorageChargeBoundary{}
		}
		out[node.ID] = cloneEnergyPathStorageChargeBoundary(&node.storageChargeBoundaries[0])
	}
	return out
}

func mergeEnergyPathStorageChargeNodeMetadata(kept, fallback []EnergyExplanationNode) []EnergyExplanationNode {
	byID := map[string][]energyPathStorageChargeBoundary{}
	for _, node := range fallback {
		if len(node.storageChargeBoundaries) > 0 {
			byID[node.ID] = unionEnergyPathStorageChargeBoundaries(byID[node.ID], node.storageChargeBoundaries)
		}
	}
	if len(byID) == 0 {
		return kept
	}
	out := append([]EnergyExplanationNode(nil), kept...)
	for index := range out {
		out[index].storageChargeBoundaries = unionEnergyPathStorageChargeBoundaries(out[index].storageChargeBoundaries, byID[out[index].ID])
	}
	return out
}

func energyPathStorageChargeWarnings(series []energyExplanationSeries) []EnergyWarning {
	var warnings []EnergyWarning
	for _, item := range series {
		boundary := item.storageChargeBoundary
		if boundary == nil {
			continue
		}
		err := validateEnergyPathStorageChargeBoundary(boundary)
		if err == nil && boundary.State == energyPathStorageChargeNative {
			continue
		}
		message := fmt.Sprintf("Storage charge %q remains non-flow context: %s. It is not Facility consumption.", boundary.SourceKey, boundary.Reason)
		if err != nil {
			message += " Invalid boundary metadata: " + err.Error()
		}
		warnings = appendEnergyDriverWarning(warnings, EnergyWarning{Severity: "warning", Code: "storage_charge_boundary_unresolved", Message: message})
	}
	return warnings
}

func energyPathStorageChargePreferredBoundary(selected energyExplanationSeries, candidates []energyExplanationSeries) *energyPathStorageChargeBoundary {
	boundary := cloneEnergyPathStorageChargeBoundary(selected.storageChargeBoundary)
	if boundary == nil {
		return nil
	}
	selectedIDs := map[string]bool{}
	for _, sourceID := range selected.SourceIDs {
		selectedIDs[sourceID] = true
	}
	// A valid Monthly source cannot lend its proof to an unproved companion
	// selected for another period. Downgrade the combined record, not its data.
	for _, item := range candidates {
		if !energyExplanationSourcesIntersect(item.SourceIDs, selectedIDs) {
			continue
		}
		if item.storageChargeBoundary == nil || item.storageChargeBoundary.State != energyPathStorageChargeNative || validateEnergyPathStorageChargeBoundary(item.storageChargeBoundary) != nil {
			if boundary.State == energyPathStorageChargeNative {
				boundary.State, boundary.Reason = energyPathStorageChargeUnresolved, "unproved_native_charge_reporting_identity"
			}
		}
	}
	boundary.SourceIDs = appendUniqueStrings(nil, selected.SourceIDs...)
	return boundary
}

// An explicitly supplied original opts exact Charge requests into a separate
// context census. A missing/unbound owner is unresolved, never consumption.
// All other output names and the omitted-original compatibility path are intact.
func partitionEnergyPathStorageChargeOutputs(names []string, inventory energyPathStorageChargeInventory) []string {
	if !inventory.HasOriginal {
		return names
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if energyPathIsStorageChargeOutputName(name) {
			continue
		}
		out = append(out, name)
	}
	return out
}

func energyPathIsStorageChargeOutputName(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "Electric Storage Charge Energy") || strings.EqualFold(strings.TrimSpace(name), "Electric Storage Charge Power")
}

func energyPathStorageChargeAvailability(plan *PurposeRunPlan, inventory energyPathStorageChargeInventory, sources []EnergyDataSource) []EnergySourceAvailabilityEntry {
	if plan == nil || !inventory.HasOriginal {
		return nil
	}
	type request struct{ name, key, frequency string }
	requests := map[string]request{}
	for _, object := range plan.OutputObjects {
		if !purposeIDsContain(object.PurposeIDs, SimulationPurposeBasicEnergy) || !strings.EqualFold(object.ObjectType, "Output:Variable") {
			continue
		}
		name := firstNonEmpty(object.VariableName, purposeOutputVariableName(object.Fields))
		if !energyPathIsStorageChargeOutputName(name) {
			continue
		}
		key := strings.TrimSpace(firstNonEmpty(object.KeyValue, purposeOutputKeyValue(object.Fields)))
		frequency := canonicalPurposeFrequency(firstNonEmpty(object.ReportingFrequency, purposeOutputFrequency(object.ObjectType, object.Fields)))
		keys := map[string]string{}
		if key != "" && key != "*" {
			keys[energyPathStorageBoundaryKey(key)] = key
		} else {
			for originalKey := range inventory.ByKey {
				keys[originalKey] = originalKey
			}
			// Preserve independently observed unbound keys as unresolved context.
			for _, source := range sources {
				if strings.EqualFold(source.Name, name) && strings.TrimSpace(source.KeyValue) != "" {
					keys[energyPathStorageBoundaryKey(source.KeyValue)] = source.KeyValue
				}
			}
			if len(keys) == 0 {
				keys["*"] = "*"
			}
		}
		for normalized, actual := range keys {
			identity := energyPathStorageBoundaryKey(name) + "\x00" + normalized + "\x00" + frequency
			requests[identity] = request{name, actual, frequency}
		}
	}
	identities := make([]string, 0, len(requests))
	for identity := range requests {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	out := make([]EnergySourceAvailabilityEntry, 0, len(identities))
	for _, identity := range identities {
		request := requests[identity]
		entry := EnergySourceAvailabilityEntry{Name: fmt.Sprintf("%s [%s; %s]", request.name, request.key, request.frequency), Level: "context", Status: "missing"}
		matches, observed := 0, false
		for _, source := range sources {
			if source.SourceType != "sql_report_data" || !strings.EqualFold(source.Name, request.name) || !strings.EqualFold(source.KeyValue, request.key) || !strings.EqualFold(canonicalPurposeFrequency(source.ReportingFrequency), request.frequency) {
				continue
			}
			matches++
			entry.SourceIDs = appendUniqueStrings(entry.SourceIDs, source.ID)
			unit := "J"
			if strings.EqualFold(request.name, "Electric Storage Charge Power") {
				unit = "W"
			}
			observed = !source.IsMeter && source.SourceUnit == unit && energyDataSourceValueKnown(source, energySourceObservedRaw)
		}
		if matches == 1 && observed {
			entry.Status = "found"
		}
		sort.Strings(entry.SourceIDs)
		out = append(out, entry)
	}
	return out
}
