package simulation

import (
	"fmt"
	"sort"
	"strings"
)

// This shadow contains only the small, original VRF owner load roster. It is
// recorded during the canonical SQL walk, not by rescanning ReportData. The
// ordinary load selector remains authoritative; this evidence validates and
// supplies the unrounded monthly values of the sources it actually selected.
type energyPathVRFLoadEvidence struct {
	ZoneName      string
	ServiceKind   string
	Monthly       map[int]float64
	InvalidMonths map[int]bool
	Invalid       bool
}

type energyPathVRFLoadCollector struct {
	axis       map[int64]bool
	byID       map[int]*energyPathVRFLoadEvidence
	identities map[string][]int
	seen       map[int]map[int]bool
}

func newEnergyPathVRFLoadCollector(dictionaries []energyExplanationDictionary, systems []energyPathVRFSystem, plan *PurposeRunPlan, axis map[int64]bool) *energyPathVRFLoadCollector {
	if len(systems) == 0 || !energyExplanationPlanUsesEnergyPath(plan) {
		return nil
	}
	collector := &energyPathVRFLoadCollector{axis: axis, byID: map[int]*energyPathVRFLoadEvidence{}, identities: map[string][]int{}, seen: map[int]map[int]bool{}}
	owners := map[string]string{}
	for _, system := range systems {
		for _, terminal := range system.Terminals {
			owners[energyPathVRFName(terminal.ZoneName)] = terminal.ZoneName
		}
	}
	for _, dictionary := range dictionaries {
		if dictionary.load == nil || dictionary.isMeter || !strings.EqualFold(dictionary.load.Scope, "zone") ||
			!strings.EqualFold(dictionary.reportingFrequency, "Monthly") || !strings.EqualFold(strings.TrimSpace(dictionary.row.units), "J") {
			continue
		}
		zone := owners[energyPathVRFName(dictionary.row.keyValue)]
		service, ok := energyLoadCanonicalSelectionService(dictionary.load.ServiceKind)
		if zone == "" || !ok || !energyPathVRFLoadRequested(dictionary, plan, zone) {
			continue
		}
		id := dictionary.row.index
		collector.byID[id] = &energyPathVRFLoadEvidence{ZoneName: zone, ServiceKind: service,
			Monthly: map[int]float64{}, InvalidMonths: map[int]bool{}, Invalid: len(axis) == 0}
		collector.seen[id] = map[int]bool{}
		identity := energyPathVRFName(dictionary.row.keyValue) + "\x00" + normalizeEnergyOutputName(dictionary.row.name)
		collector.identities[identity] = append(collector.identities[identity], id)
	}
	for _, ids := range collector.identities {
		if len(ids) > 1 {
			for _, id := range ids {
				collector.byID[id].Invalid = true
			}
		}
	}
	return collector
}

func energyPathVRFLoadRequested(dictionary energyExplanationDictionary, plan *PurposeRunPlan, zone string) bool {
	requested := false
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(strings.TrimSpace(output.ObjectType), "Output:Variable") ||
			!strings.EqualFold(strings.TrimSpace(output.KeyValue), strings.TrimSpace(dictionary.row.keyValue)) ||
			!strings.EqualFold(strings.TrimSpace(output.VariableName), strings.TrimSpace(dictionary.row.name)) ||
			!strings.EqualFold(strings.TrimSpace(output.ReportingFrequency), "Monthly") {
			continue
		}
		if !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) || !strings.EqualFold(strings.TrimSpace(output.ScopeZoneName), zone) {
			return false
		}
		requested = true
	}
	return requested
}

func (collector *energyPathVRFLoadCollector) observe(row SQLSeriesRow) {
	if collector == nil {
		return
	}
	item := collector.byID[row.DictionaryIndex]
	if item == nil || !collector.axis[row.TimeIndex] {
		// Design-day and warmup rows cannot fill a Weather Monthly observation.
		return
	}
	if !row.Month.Valid || row.Month.Int64 < 1 || row.Month.Int64 > 12 {
		item.Invalid = true
		return
	}
	month := int(row.Month.Int64)
	if collector.seen[row.DictionaryIndex][month] {
		item.InvalidMonths[month] = true
	}
	collector.seen[row.DictionaryIndex][month] = true
	if !row.Value.Valid || !energyPathFinite(row.Value.Float64) || row.Value.Float64 < 0 {
		item.InvalidMonths[month] = true
		return
	}
	value := row.Value.Float64 / 3.6e6
	if !energyPathFinite(value) {
		item.InvalidMonths[month] = true
		return
	}
	// Duplicate values are not summed and a valid sibling cannot repair NULL.
	if !item.InvalidMonths[month] {
		item.Monthly[month] = value
	}
}

func (collector *energyPathVRFLoadCollector) evidence() map[string]energyPathVRFLoadEvidence {
	if collector == nil || len(collector.byID) == 0 {
		return nil
	}
	result := make(map[string]energyPathVRFLoadEvidence, len(collector.byID))
	for id, item := range collector.byID {
		result[fmt.Sprintf("sql-rdd-%d", id)] = *item
	}
	return result
}

func energyPathVRFSelectedLoadSeries(series []energyExplanationSeries, systems []energyPathVRFSystem) []energyExplanationSeries {
	if len(systems) == 0 {
		return nil
	}
	owners := map[string]bool{}
	for _, system := range systems {
		for _, terminal := range system.Terminals {
			owners[energyPathVRFName(terminal.ZoneName)] = true
		}
	}
	var selected []energyExplanationSeries
	for _, item := range series {
		if item.Stage == "load" && owners[energyPathVRFName(item.ZoneName)] {
			selected = append(selected, item)
		}
	}
	return selected
}

// Build from the canonical selection's monthly primary source IDs, not all
// inspector context sources. Reported site electricity is already model-total;
// only these delivered-load weights use the source's resolved multiplier.
func buildEnergyPathVRFLoadObservations(selected []energyExplanationSeries, sources []EnergyDataSource, evidence map[string]energyPathVRFLoadEvidence, cohorts []energyPathVRFConsumptionCohort) []energyPathVRFLoadObservation {
	if len(cohorts) == 0 {
		return nil
	}
	months, owners := map[int]bool{}, map[string]bool{}
	for _, cohort := range cohorts {
		for month, present := range cohort.Months {
			if present {
				months[month] = true
			}
		}
		for _, terminal := range cohort.System.Terminals {
			owners[energyPathVRFName(terminal.ZoneName)] = true
		}
	}
	sourceByID := map[string]EnergyDataSource{}
	for _, source := range sources {
		sourceByID[source.ID] = source
	}
	var out []energyPathVRFLoadObservation
	for _, original := range selected {
		item := canonicalEnergyExplanationSeries(original)
		service, ok := energyLoadCanonicalSelectionService(item.ServiceKind)
		if item.Stage != "load" || !ok || !owners[energyPathVRFName(item.ZoneName)] {
			continue
		}
		ids := appendUniqueStrings(nil, energyExplanationPeriodSourceIDs(item.MonthlySourceIDs, item.SourceIDs)...)
		observation := energyPathVRFLoadObservation{ZoneName: item.ZoneName, ServiceKind: service,
			Monthly: map[int]float64{}, InvalidMonths: map[int]bool{}, SourceIDs: ids}
		for month := range months {
			value, valid := 0.0, len(ids) > 0
			for _, id := range ids {
				reported, found := evidence[id]
				source, sourceFound := sourceByID[id]
				raw, observed := reported.Monthly[month]
				factor := source.EffectiveMultiplier
				if !found || !sourceFound || reported.Invalid || reported.InvalidMonths[month] || !observed ||
					!strings.EqualFold(reported.ZoneName, item.ZoneName) || reported.ServiceKind != service ||
					!energyPathFinite(raw) || raw < 0 || !energyPathFinite(factor) || factor <= 0 ||
					source.MultiplierApplication == "" || source.MultiplierApplication == energyMultiplierUnknown {
					valid = false
					continue
				}
				value += raw * factor
			}
			if valid && energyPathFinite(value) {
				observation.Monthly[month] = value
			} else {
				observation.InvalidMonths[month] = true
			}
		}
		out = append(out, observation)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return energyPathVRFName(out[i].ZoneName)+"\x00"+out[i].ServiceKind < energyPathVRFName(out[j].ZoneName)+"\x00"+out[j].ServiceKind
	})
	return out
}
