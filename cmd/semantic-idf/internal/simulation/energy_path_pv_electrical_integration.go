package simulation

import (
	"database/sql"
	"fmt"
	"strings"
)

// Exact native electrical families opt into this reporting boundary only when
// the original inventory/reader opted in. Component totals are not extra
// Facility production or consumption, including legacy Timestep companions.
func filterEnergyPathPVElectricalSeries(input []energyExplanationSeries, evidence energyPathPVElectricalEvidence, plans ...*PurposeRunPlan) ([]energyExplanationSeries, []EnergyWarning) {
	if !evidence.Inventory.HasOriginal || !evidence.Inventory.SchemaReviewed || len(evidence.Observations) == 0 {
		return input, nil
	}
	definitions := map[string]energyPathPVElectricalDefinition{}
	for _, definition := range energyPathPVElectricalDefinitions() {
		definitions[energyPathPVElectricalKey(definition.Name)] = definition
	}
	snapshots := map[string]EnergyDataSource{}
	counts := map[string]int{}
	for _, source := range evidence.SourceSnapshots {
		snapshots[source.ID] = source
		counts[source.ID]++
	}
	observed := map[string]bool{}
	observations := map[string]energyPathPVElectricalObservation{}
	negative := map[string]bool{}
	for _, observation := range evidence.Observations {
		if observation.Status == "observed" && len(observation.SourceIDs) == 1 {
			observed[observation.SourceIDs[0]] = true
			negative[observation.SourceIDs[0]] = observation.HasNegativeValue
			observations[observation.SourceIDs[0]] = observation
		}
	}
	var out []energyExplanationSeries
	var warnings []EnergyWarning
	for _, item := range input {
		name := firstNonEmpty(item.SourceName, item.sourceName)
		definition, native := definitions[energyPathPVElectricalKey(name)]
		// Generic meter aliases may replace an invalid native Name with its
		// foreign KeyValue. Actual dictionary ownership precedes that alias.
		owned := false
		for _, id := range item.SourceIDs {
			if source, exists := snapshots[id]; exists {
				owned = true
				definition, native = definitions[energyPathPVElectricalKey(source.Name)]
				break
			}
		}
		if owned && !native {
			continue
		}
		if !native {
			out = append(out, item)
			continue
		}
		if !definition.IsMeter && definition.ID != "storage.charge" {
			continue // Native constituent/subtotal/transfer remains source-only.
		}
		// Charge outside the new M/H observation contract retains its existing
		// independent durable non-consumption qualifier, never a supply ribbon.
		if !owned && definition.ID == "storage.charge" && !strings.EqualFold(item.sourceFrequency, "Monthly") && !strings.EqualFold(item.sourceFrequency, "Hourly") {
			out = append(out, item)
			continue
		}
		valid := len(item.SourceIDs) == 1
		var source EnergyDataSource
		if valid {
			id := item.SourceIDs[0]
			source = snapshots[id]
			valid = counts[id] == 1 && observed[id] && energyDataSourceValueKnown(source, energySourceObservedRaw) &&
				strings.EqualFold(source.Name, name) && strings.EqualFold(source.ReportingFrequency, item.sourceFrequency) &&
				source.IsMeter == definition.IsMeter && strings.EqualFold(source.KeyValue, firstNonEmpty(item.SourceKey, item.sourceKeyValue))
			valid = valid && (item.SourceKey == "" || strings.EqualFold(source.KeyValue, item.SourceKey)) && (item.sourceKeyValue == "" || strings.EqualFold(source.KeyValue, item.sourceKeyValue))
			valid = valid && (item.SourceName == "" || strings.EqualFold(source.Name, item.SourceName)) && (item.sourceName == "" || strings.EqualFold(source.Name, item.sourceName))
		}
		if !valid {
			warnings = appendEnergyDriverWarning(warnings, EnergyWarning{Severity: "warning", Code: "native_electrical_budget_unverified", Message: fmt.Sprintf("%s (%s) remains source context: no unique complete Monthly/Hourly native observation authorizes this graph budget.", name, item.sourceFrequency)})
			continue
		}
		if definition.ID == "facility.produced" && (source.RawValue < 0 || negative[source.ID]) {
			warnings = appendEnergyDriverWarning(warnings, EnergyWarning{Severity: "warning", Code: "native_signed_production_context", Message: "Signed net produced electricity is retained as measured source context. A series containing negative periods is not converted into positive supply or an annual sum of positive periods."})
			continue
		}
		// ID/status validation alone cannot sanitize a generic aggregate that
		// included design days or warmup. Replace EVERY temporal value from
		// native validated cells, preserving only the classification metadata.
		var plan *PurposeRunPlan
		if len(plans) > 0 {
			plan = plans[0]
		}
		item = energyPathPVElectricalNativeSeriesValues(item, observations[source.ID], plan)
		item.RawTotal, item.Total = source.RawValue, source.RawValue
		out = append(out, item)
	}
	return out, warnings
}

// Record actual removed generic component series, not graph absence or a
// source-name census. These older frequencies have no new observation proof.
func energyPathPVElectricalRemovedLegacyComponentSourceIDs(before, after []energyExplanationSeries, evidence energyPathPVElectricalEvidence) []string {
	if !evidence.Inventory.HasOriginal || !evidence.Inventory.SchemaReviewed || len(evidence.Observations) == 0 {
		return nil
	}
	kept := map[string]bool{}
	for _, item := range after {
		for _, id := range item.SourceIDs {
			kept[id] = true
		}
	}
	definitions := map[string]energyPathPVElectricalDefinition{}
	for _, definition := range energyPathPVElectricalDefinitions() {
		definitions[energyPathPVElectricalKey(definition.Name)] = definition
	}
	var removed []string
	for _, item := range before {
		if strings.EqualFold(item.sourceFrequency, "Monthly") || strings.EqualFold(item.sourceFrequency, "Hourly") {
			continue
		}
		definition, relevant := definitions[energyPathPVElectricalKey(firstNonEmpty(item.SourceName, item.sourceName))]
		if !relevant || definition.IsMeter || definition.ID == "storage.charge" {
			continue
		}
		for _, id := range item.SourceIDs {
			if !kept[id] {
				removed = appendUniqueStrings(removed, id)
			}
		}
	}
	return removed
}

func energyPathPVElectricalNativeSeriesValues(item energyExplanationSeries, observation energyPathPVElectricalObservation, plan *PurposeRunPlan) energyExplanationSeries {
	builder := energyExplanationSeriesBuilder{}
	dictionary := energyExplanationDictionary{reportingFrequency: observation.Intent.Frequency}
	start, end, selected := energyExplanationSelectedRangeDays(plan)
	if observation.Intent.Frequency == "Monthly" {
		for month, value := range observation.MonthlyValues {
			// A monthly cell cannot locate energy within a selected day range.
			accumulateEnergyExplanationSeriesBuilder(&builder, SQLSeriesRow{Month: sql.NullInt64{Int64: int64(month), Valid: true}}, value, "kWh", dictionary, 0, 0, false)
		}
	} else {
		for _, cell := range observation.HourlyValues {
			row := SQLSeriesRow{Month: sql.NullInt64{Int64: int64(cell.Month), Valid: true}, Day: sql.NullInt64{Int64: int64(cell.Day), Valid: true}, Hour: sql.NullInt64{Int64: int64(cell.Hour), Valid: true}}
			accumulateEnergyExplanationSeriesBuilder(&builder, row, cell.Value, "kWh", dictionary, start, end, selected)
		}
	}
	item.RawTotal, item.Total = builder.total, builder.total
	item.Monthly, item.RawMonthly = builder.monthly, cloneEnergyPathPVElectricalValues(builder.monthly)
	item.Daily, item.RawDaily = builder.daily, cloneEnergyPathPVElectricalValues(builder.daily)
	item.Hourly, item.RawHourly = builder.hourly, cloneEnergyPathPVElectricalValues(builder.hourly)
	item.SelectedRange, item.RawSelectedRange, item.HasSelectedRange = builder.selectedRange, builder.selectedRange, builder.hasSelectedRange
	item.AnnualSourceIDs, item.MonthlySourceIDs, item.DailySourceIDs, item.HourlySourceIDs, item.SelectedRangeSourceIDs = nil, nil, nil, nil, nil
	return item
}

func cloneEnergyPathPVElectricalValues(input map[int]float64) map[int]float64 {
	if input == nil {
		return nil
	}
	out := make(map[int]float64, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func energyPathPVElectricalSeriesHasNegativePeriod(item energyExplanationSeries) bool {
	if item.Total < 0 || item.HasSelectedRange && item.SelectedRange < 0 {
		return true
	}
	for _, values := range []map[int]float64{item.Monthly, item.Daily, item.Hourly} {
		for _, value := range values {
			if value < 0 {
				return true
			}
		}
	}
	return false
}

// Evidence remains native/unrounded. Only public source copies are marked and
// rounded under the existing scalar display contract. No graph value is read.
func applyEnergyPathProtectedPVElectricalSources(existing []EnergyDataSource, evidence energyPathPVElectricalEvidence) []EnergyDataSource {
	if len(evidence.SourceSnapshots) == 0 {
		return existing
	}
	out := applyEnergyPathPVElectricalSourceObservations(existing, evidence)
	owned := map[string]bool{}
	for _, source := range evidence.SourceSnapshots {
		owned[source.ID] = true
	}
	for index := range out {
		if owned[out[index].ID] {
			out[index] = protectEnergyPathPVSourceObservation(out[index])
		}
	}
	return out
}
