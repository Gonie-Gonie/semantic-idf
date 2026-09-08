package simulation

import (
	"fmt"
	"sort"
	"strings"
)

// Monthly contains already-effective canonical service loads. Presence, not a
// nonzero value, proves observation; the allocator never applies Zone factors.
type energyPathVRFLoadObservation struct {
	ZoneName, ServiceKind string
	Monthly               map[int]float64
	InvalidMonths         map[int]bool
	SourceIDs             []string
}

type energyPathVRFAllocationPlan struct {
	Zones    []energyPathVRFZoneAllocation
	Services []energyPathVRFServiceAllocation
	Sources  []energyPathVRFSourceAllocation
	Issues   []string
}

type energyPathVRFZoneAllocation struct {
	SystemID, ZoneName, ServiceKind, Carrier, PeriodID string
	Month                                              int // 0 is the sum of actual months.
	Months                                             []int
	DirectValue, AllocatedValue, TotalValue, LoadValue float64
	DirectKnown, AllocatedKnown, TotalKnown, LoadKnown bool
	SharedKnown                                        bool
	ConsumptionSourceIDs, LoadSourceIDs                []string
	WeightSourceIDs                                    []string
}

type energyPathVRFServiceAllocation struct {
	SystemID, ServiceKind, Carrier, PeriodID                  string
	Month                                                     int
	Months                                                    []int
	DirectValue, SharedValue, AllocatedValue, UnassignedValue float64
	DirectKnown, SharedKnown, AllocatedKnown, UnassignedKnown bool
	LoadValue                                                 float64
	LoadKnown                                                 bool
	ConsumptionSourceIDs, LoadSourceIDs, Reasons              []string
}

// ObservedValue is the original source amount, not an individual-Zone
// measurement for Shared rows. Target retains the original (possibly blank)
// ownership scope; ZoneName is only the allocation recipient. Repeated shared
// observations across recipients are context, never additive source totals.
type energyPathVRFSourceAllocation struct {
	SystemID, ZoneName, ServiceKind, Carrier, PeriodID string
	Month                                              int
	Months                                             []int
	Target                                             energyPathVRFConsumptionTarget
	Source                                             EnergyDataSource
	Shared                                             bool
	ObservedValue, AllocatedValue                      float64
	ObservedKnown, AllocatedKnown                      bool
	LoadSourceIDs                                      []string
	Method                                             string
}

func buildEnergyPathVRFAllocationPlan(cohorts []energyPathVRFConsumptionCohort, loads []energyPathVRFLoadObservation) energyPathVRFAllocationPlan {
	plan := energyPathVRFAllocationPlan{}
	loadIndex := map[string][]energyPathVRFLoadObservation{}
	for _, load := range loads {
		key := energyPathVRFName(load.ZoneName) + "|" + energyPathVRFName(load.ServiceKind)
		loadIndex[key] = append(loadIndex[key], load)
	}
	systemCounts, sourceCounts := map[string]int{}, map[string]int{}
	ordered := append([]energyPathVRFConsumptionCohort(nil), cohorts...)
	for _, cohort := range ordered {
		systemCounts[vrfAllocationSystemKey(cohort.System)]++
		for _, observation := range cohort.Observations {
			if observation.Source.ID != "" {
				sourceCounts[observation.Source.ID]++
			}
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return vrfAllocationSystemKey(ordered[i].System) < vrfAllocationSystemKey(ordered[j].System)
	})
	for _, cohort := range ordered {
		key := vrfAllocationSystemKey(cohort.System)
		owners, targets, months, err := vrfAllocationRoster(cohort)
		if systemCounts[key] != 1 {
			err = fmt.Errorf("duplicate original system")
		}
		if err != nil {
			plan.Issues = append(plan.Issues, key+": "+err.Error())
			continue
		}
		observed := map[string][]energyPathVRFConsumptionObservation{}
		for _, observation := range cohort.Observations {
			identity := vrfAllocationTargetKey(observation.Target)
			observed[identity] = append(observed[identity], observation)
		}
		for _, service := range []string{"cooling", "heating"} {
			monthly := energyPathVRFAllocationPlan{}
			for _, month := range months {
				vrfAllocationMonth(&monthly, cohort, owners, targets, observed, sourceCounts, loadIndex, service, month)
			}
			plan.Zones = append(plan.Zones, monthly.Zones...)
			plan.Services = append(plan.Services, monthly.Services...)
			plan.Sources = append(plan.Sources, monthly.Sources...)
			vrfAllocationAnnual(&plan, monthly, months)
		}
	}
	plan.Issues = vrfAllocationIDs(plan.Issues)
	return plan
}

func vrfAllocationSystemKey(system energyPathVRFSystem) string {
	return energyPathVRFName(system.OutdoorUnit.ObjectType) + "|" + energyPathVRFName(system.OutdoorUnit.ObjectName)
}

func vrfAllocationTargetKey(target energyPathVRFConsumptionTarget) string {
	return target.Definition.ID + "|" + energyPathVRFName(target.KeyValue) + "|" + energyPathVRFName(target.ZoneName)
}

// Validate the complete original roster, never a scope-filtered list. Physical
// IDF connections remain the context builder's authority; this boundary rejects
// malformed/duplicated typed identities rather than changing that ownership.
func vrfAllocationRoster(cohort energyPathVRFConsumptionCohort) ([]string, []energyPathVRFConsumptionTarget, []int, error) {
	system := cohort.System
	if system.OutdoorUnit.ID == "" || !strings.EqualFold(system.OutdoorUnit.ObjectType, energyPathVRFOutdoorType) ||
		strings.TrimSpace(system.OutdoorUnit.ObjectName) == "" || len(system.Terminals) == 0 {
		return nil, nil, nil, fmt.Errorf("missing original system identity or owners")
	}
	owners, terminalKeys := map[string]string{}, map[string]bool{}
	expected := map[string]energyPathVRFConsumptionTarget{}
	for _, terminal := range system.Terminals {
		zone, name := energyPathVRFName(terminal.ZoneName), energyPathVRFName(terminal.Terminal.ObjectName)
		if zone == "" || name == "" || owners[zone] != "" || terminalKeys[name] || !strings.EqualFold(terminal.Terminal.ObjectType, energyPathVRFTerminalType) {
			return nil, nil, nil, fmt.Errorf("ambiguous original terminal ownership")
		}
		owners[zone], terminalKeys[name] = strings.TrimSpace(terminal.ZoneName), true
	}
	for _, definition := range energyPathVRFConsumptionDefinitions() {
		if definition.Shared {
			target := energyPathVRFConsumptionTarget{Definition: definition, KeyValue: system.OutdoorUnit.ObjectName, ObjectIndex: system.OutdoorUnit.ObjectIndex}
			expected[vrfAllocationTargetKey(target)] = target
			continue
		}
		for _, terminal := range system.Terminals {
			target := energyPathVRFConsumptionTarget{Definition: definition, KeyValue: terminal.Terminal.ObjectName, ZoneName: terminal.ZoneName, ObjectIndex: terminal.Terminal.ObjectIndex}
			expected[vrfAllocationTargetKey(target)] = target
		}
	}
	seen := map[string]bool{}
	for _, target := range system.Targets {
		key := vrfAllocationTargetKey(target)
		want, ok := expected[key]
		if !ok || seen[key] || !vrfAllocationSameTarget(target, want) {
			return nil, nil, nil, fmt.Errorf("invalid original consumption target roster")
		}
		seen[key] = true
	}
	if len(seen) != len(expected) {
		return nil, nil, nil, fmt.Errorf("incomplete original consumption target roster")
	}
	for _, observation := range cohort.Observations {
		want, ok := expected[vrfAllocationTargetKey(observation.Target)]
		if !ok || !vrfAllocationSameTarget(observation.Target, want) {
			return nil, nil, nil, fmt.Errorf("observation outside original consumption target roster")
		}
	}
	ownerList := make([]string, 0, len(owners))
	for _, owner := range owners {
		ownerList = append(ownerList, owner)
	}
	sort.Slice(ownerList, func(i, j int) bool { return energyPathVRFName(ownerList[i]) < energyPathVRFName(ownerList[j]) })
	targets := append([]energyPathVRFConsumptionTarget(nil), system.Targets...)
	sort.Slice(targets, func(i, j int) bool { return vrfAllocationTargetKey(targets[i]) < vrfAllocationTargetKey(targets[j]) })
	months := []int{}
	for month, actual := range cohort.Months {
		if !actual {
			continue
		}
		if month < 1 || month > 12 {
			return nil, nil, nil, fmt.Errorf("invalid actual Monthly axis")
		}
		months = append(months, month)
	}
	sort.Ints(months)
	if len(months) == 0 {
		return nil, nil, nil, fmt.Errorf("missing actual Monthly axis")
	}
	return ownerList, targets, months, nil
}

func vrfAllocationSameTarget(actual, expected energyPathVRFConsumptionTarget) bool {
	return vrfAllocationTargetKey(actual) == vrfAllocationTargetKey(expected) && actual.ObjectIndex == expected.ObjectIndex &&
		actual.Definition.Shared == expected.Definition.Shared && strings.EqualFold(actual.Definition.ObjectType, expected.Definition.ObjectType) &&
		actual.Definition.Energy.EndUse == expected.Definition.Energy.EndUse && actual.Definition.Energy.Carrier == expected.Definition.Energy.Carrier
}

func vrfAllocationObservation(items []energyPathVRFConsumptionObservation, target energyPathVRFConsumptionTarget, counts map[string]int, month int) (EnergyDataSource, float64, bool) {
	if len(items) != 1 {
		return EnergyDataSource{}, 0, false
	}
	item, source := items[0], items[0].Source
	definition, recognized := energyPathVRFConsumptionDefinitionForName(source.Name)
	value, present := item.Monthly[month]
	known := item.Requested && !item.Invalid && !item.InvalidMonths[month] && present && energyPathFinite(value) && value >= 0 &&
		source.ID != "" && counts[source.ID] == 1 && source.SourceType == "sql_report_data" && !source.IsMeter &&
		strings.EqualFold(strings.TrimSpace(source.ReportingFrequency), "Monthly") && strings.EqualFold(strings.TrimSpace(source.SourceUnit), "J") &&
		strings.EqualFold(strings.TrimSpace(source.NormalizedUnit), "kWh") && recognized && definition.ID == target.Definition.ID &&
		energyPathVRFName(source.KeyValue) == energyPathVRFName(target.KeyValue) && energyPathVRFName(source.ZoneName) == energyPathVRFName(target.ZoneName)
	if !known {
		return source, 0, false
	}
	return source, value, true
}

func vrfAllocationLoad(items []energyPathVRFLoadObservation, month int) (float64, []string, bool) {
	if len(items) != 1 || items[0].InvalidMonths[month] || len(items[0].SourceIDs) == 0 {
		return 0, nil, false
	}
	for _, id := range items[0].SourceIDs {
		if strings.TrimSpace(id) == "" {
			return 0, nil, false
		}
	}
	value, present := items[0].Monthly[month]
	if !present || value < 0 || !energyPathFinite(value) {
		return 0, nil, false
	}
	return value, vrfAllocationIDs(items[0].SourceIDs), true
}

func vrfAllocationMonth(plan *energyPathVRFAllocationPlan, cohort energyPathVRFConsumptionCohort, owners []string, targets []energyPathVRFConsumptionTarget,
	observed map[string][]energyPathVRFConsumptionObservation, sourceCounts map[string]int, loads map[string][]energyPathVRFLoadObservation, service string, month int) {
	systemID, period := cohort.System.OutdoorUnit.ID, fmt.Sprintf("M%d", month)
	record := energyPathVRFServiceAllocation{SystemID: systemID, ServiceKind: service, Carrier: "electricity", PeriodID: period,
		Month: month, Months: []int{month}, DirectKnown: true, SharedKnown: true, LoadKnown: true}
	zones := make([]energyPathVRFZoneAllocation, len(owners))
	for i, owner := range owners {
		value, ids, known := vrfAllocationLoad(loads[energyPathVRFName(owner)+"|"+service], month)
		zones[i] = energyPathVRFZoneAllocation{SystemID: systemID, ZoneName: owner, ServiceKind: service, Carrier: "electricity", PeriodID: period,
			Month: month, Months: []int{month}, LoadValue: value, LoadKnown: known, LoadSourceIDs: ids}
		record.LoadValue += value
		record.LoadKnown = record.LoadKnown && known
		record.LoadSourceIDs = append(record.LoadSourceIDs, ids...)
	}
	if !energyPathFinite(record.LoadValue) {
		record.LoadValue, record.LoadKnown = 0, false
	}
	record.LoadSourceIDs = vrfAllocationIDs(record.LoadSourceIDs)
	sources := []energyPathVRFSourceAllocation{}
	for _, target := range targets {
		if target.Definition.Energy.EndUse != service {
			continue
		}
		source, value, known := vrfAllocationObservation(observed[vrfAllocationTargetKey(target)], target, sourceCounts, month)
		if known {
			record.ConsumptionSourceIDs = append(record.ConsumptionSourceIDs, source.ID)
		}
		if target.Definition.Shared {
			record.SharedValue += value
			record.SharedKnown = record.SharedKnown && known
		} else {
			record.DirectValue += value
			record.DirectKnown = record.DirectKnown && known
		}
		for i := range zones {
			if !target.Definition.Shared && !strings.EqualFold(zones[i].ZoneName, target.ZoneName) {
				continue
			}
			trace := energyPathVRFSourceAllocation{SystemID: systemID, ZoneName: zones[i].ZoneName, ServiceKind: service, Carrier: "electricity", PeriodID: period,
				Month: month, Months: []int{month}, Target: target, Source: source, Shared: target.Definition.Shared, ObservedValue: value, ObservedKnown: known}
			if !target.Definition.Shared {
				zones[i].DirectValue, zones[i].DirectKnown = value, known
				trace.Method = "direct_vrf_terminal_energy"
				if known {
					zones[i].ConsumptionSourceIDs = append(zones[i].ConsumptionSourceIDs, source.ID)
				}
			} else {
				trace.Method = "vrf_system_service_load_share"
			}
			sources = append(sources, trace)
		}
	}
	if !energyPathFinite(record.DirectValue) {
		record.DirectValue, record.DirectKnown = 0, false
	}
	if !energyPathFinite(record.SharedValue) {
		record.SharedValue, record.SharedKnown = 0, false
	}
	// Both shared components form one service cohort. A valid sibling remains
	// observed context but cannot shrink an incomplete shared consumption pool.
	canAllocate := record.SharedKnown && record.LoadKnown && (record.LoadValue > 0 || record.SharedValue == 0)
	record.AllocatedKnown, record.UnassignedKnown = canAllocate, record.SharedKnown
	record.UnassignedValue = record.SharedValue
	if canAllocate {
		// These are unrounded quotas of the complete measured pool. The service
		// pool is conserved; display-quantum apportionment belongs to projection.
		record.AllocatedValue, record.UnassignedValue = record.SharedValue, 0
	}
	for i := range zones {
		zone := &zones[i]
		zone.SharedKnown, zone.AllocatedKnown = record.SharedKnown, canAllocate
		zone.WeightSourceIDs = append([]string(nil), record.LoadSourceIDs...)
		for j := range sources {
			trace := &sources[j]
			if !trace.Shared || trace.ZoneName != zone.ZoneName {
				continue
			}
			trace.AllocatedKnown = canAllocate
			if canAllocate {
				if record.LoadValue > 0 {
					trace.AllocatedValue = trace.ObservedValue * (zone.LoadValue / record.LoadValue)
				}
				trace.LoadSourceIDs = append([]string(nil), record.LoadSourceIDs...)
				zone.AllocatedValue += trace.AllocatedValue
				zone.ConsumptionSourceIDs = append(zone.ConsumptionSourceIDs, trace.Source.ID)
			}
		}
		zone.TotalValue = zone.DirectValue + zone.AllocatedValue
		zone.TotalKnown = zone.DirectKnown && zone.AllocatedKnown && energyPathFinite(zone.TotalValue)
		if !energyPathFinite(zone.TotalValue) {
			zone.TotalValue = 0
		}
		zone.ConsumptionSourceIDs = vrfAllocationIDs(zone.ConsumptionSourceIDs)
	}
	if !cohort.Requested {
		record.Reasons = append(record.Reasons, "unrequested_cohort")
	}
	if !record.DirectKnown {
		record.Reasons = append(record.Reasons, "missing_or_invalid_local")
	}
	if !record.SharedKnown {
		record.Reasons = append(record.Reasons, "missing_or_invalid_shared")
	}
	if !record.LoadKnown {
		record.Reasons = append(record.Reasons, "missing_or_invalid_owner_load")
	} else if record.LoadValue == 0 && record.SharedValue > 0 {
		record.Reasons = append(record.Reasons, "zero_service_load")
	}
	record.ConsumptionSourceIDs = vrfAllocationIDs(record.ConsumptionSourceIDs)
	plan.Zones, plan.Services, plan.Sources = append(plan.Zones, zones...), append(plan.Services, record), append(plan.Sources, sources...)
}

func vrfAllocationIDs(ids []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id], out = true, append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Annual is exclusively a sum of the actual observed axis. Unknown months
// retain their incomplete flags; partial observed sums never become a complete
// annual observation, and a three-month run is not invented into twelve months.
func vrfAllocationAnnual(plan *energyPathVRFAllocationPlan, monthly energyPathVRFAllocationPlan, months []int) {
	zones := map[string]energyPathVRFZoneAllocation{}
	for _, next := range monthly.Zones {
		current, exists := zones[next.ZoneName]
		if !exists {
			current = next
			current.Month, current.PeriodID, current.Months = 0, "annual", append([]int(nil), months...)
		} else {
			current.DirectValue += next.DirectValue
			current.AllocatedValue += next.AllocatedValue
			current.TotalValue += next.TotalValue
			current.LoadValue += next.LoadValue
			current.DirectKnown = current.DirectKnown && next.DirectKnown
			current.SharedKnown = current.SharedKnown && next.SharedKnown
			current.AllocatedKnown = current.AllocatedKnown && next.AllocatedKnown
			current.TotalKnown = current.TotalKnown && next.TotalKnown
			current.LoadKnown = current.LoadKnown && next.LoadKnown
			current.ConsumptionSourceIDs = vrfAllocationIDs(append(append([]string(nil), current.ConsumptionSourceIDs...), next.ConsumptionSourceIDs...))
			current.LoadSourceIDs = vrfAllocationIDs(append(append([]string(nil), current.LoadSourceIDs...), next.LoadSourceIDs...))
			current.WeightSourceIDs = vrfAllocationIDs(append(append([]string(nil), current.WeightSourceIDs...), next.WeightSourceIDs...))
		}
		current.DirectValue, current.DirectKnown = vrfAllocationFiniteSum(current.DirectValue, current.DirectKnown)
		current.AllocatedValue, current.AllocatedKnown = vrfAllocationFiniteSum(current.AllocatedValue, current.AllocatedKnown)
		current.TotalValue, current.TotalKnown = vrfAllocationFiniteSum(current.TotalValue, current.TotalKnown)
		current.LoadValue, current.LoadKnown = vrfAllocationFiniteSum(current.LoadValue, current.LoadKnown)
		zones[next.ZoneName] = current
	}
	zoneKeys := []string{}
	for key := range zones {
		zoneKeys = append(zoneKeys, key)
	}
	sort.Strings(zoneKeys)
	for _, key := range zoneKeys {
		plan.Zones = append(plan.Zones, zones[key])
	}
	var service energyPathVRFServiceAllocation
	for i, next := range monthly.Services {
		if i == 0 {
			service = next
			service.Month, service.PeriodID, service.Months = 0, "annual", append([]int(nil), months...)
		} else {
			service.DirectValue += next.DirectValue
			service.SharedValue += next.SharedValue
			service.AllocatedValue += next.AllocatedValue
			service.UnassignedValue += next.UnassignedValue
			service.LoadValue += next.LoadValue
			service.DirectKnown = service.DirectKnown && next.DirectKnown
			service.SharedKnown = service.SharedKnown && next.SharedKnown
			service.AllocatedKnown = service.AllocatedKnown && next.AllocatedKnown
			service.UnassignedKnown = service.UnassignedKnown && next.UnassignedKnown
			service.LoadKnown = service.LoadKnown && next.LoadKnown
			service.ConsumptionSourceIDs = vrfAllocationIDs(append(append([]string(nil), service.ConsumptionSourceIDs...), next.ConsumptionSourceIDs...))
			service.LoadSourceIDs = vrfAllocationIDs(append(append([]string(nil), service.LoadSourceIDs...), next.LoadSourceIDs...))
			service.Reasons = vrfAllocationIDs(append(append([]string(nil), service.Reasons...), next.Reasons...))
		}
	}
	service.DirectValue, service.DirectKnown = vrfAllocationFiniteSum(service.DirectValue, service.DirectKnown)
	service.SharedValue, service.SharedKnown = vrfAllocationFiniteSum(service.SharedValue, service.SharedKnown)
	service.AllocatedValue, service.AllocatedKnown = vrfAllocationFiniteSum(service.AllocatedValue, service.AllocatedKnown)
	service.UnassignedValue, service.UnassignedKnown = vrfAllocationFiniteSum(service.UnassignedValue, service.UnassignedKnown)
	service.LoadValue, service.LoadKnown = vrfAllocationFiniteSum(service.LoadValue, service.LoadKnown)
	plan.Services = append(plan.Services, service)
	sources := map[string]energyPathVRFSourceAllocation{}
	for _, next := range monthly.Sources {
		key := vrfAllocationTargetKey(next.Target) + "|" + next.ZoneName
		current, exists := sources[key]
		if !exists {
			current = next
			current.Month, current.PeriodID, current.Months = 0, "annual", append([]int(nil), months...)
		} else {
			current.ObservedValue += next.ObservedValue
			current.AllocatedValue += next.AllocatedValue
			current.ObservedKnown = current.ObservedKnown && next.ObservedKnown
			current.AllocatedKnown = current.AllocatedKnown && next.AllocatedKnown
			current.LoadSourceIDs = vrfAllocationIDs(append(append([]string(nil), current.LoadSourceIDs...), next.LoadSourceIDs...))
		}
		current.ObservedValue, current.ObservedKnown = vrfAllocationFiniteSum(current.ObservedValue, current.ObservedKnown)
		current.AllocatedValue, current.AllocatedKnown = vrfAllocationFiniteSum(current.AllocatedValue, current.AllocatedKnown)
		sources[key] = current
	}
	sourceKeys := []string{}
	for key := range sources {
		sourceKeys = append(sourceKeys, key)
	}
	sort.Strings(sourceKeys)
	for _, key := range sourceKeys {
		plan.Sources = append(plan.Sources, sources[key])
	}
}

func vrfAllocationFiniteSum(value float64, known bool) (float64, bool) {
	if !energyPathFinite(value) {
		return 0, false
	}
	return value, known
}
