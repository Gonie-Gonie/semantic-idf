package simulation

import "strings"

type energyPathBaseboardContextBinding struct {
	Target         energyPathBaseboardTarget
	Name           string
	Units          string
	IntegratesRate bool
	ObjectIndex    *int
}

func energyPathIsBaseboardContextVariable(name string) bool {
	switch normalizeEnergyOutputName(name) {
	case "baseboard total heating energy", "baseboard total heating rate", "baseboard electricity rate":
		return true
	default:
		return false
	}
}

// The original typed owner and exact native name/unit are independent of any
// request. A Monthly Basic Energy request additionally proves the intended
// owner scope. Its index is NOT the opener for an Hourly source.
func energyPathBaseboardContextDictionaryScope(dictionary energyExplanationDictionary, plan *PurposeRunPlan, targets []energyPathBaseboardTarget) (energyPathBaseboardContextBinding, bool) {
	var binding energyPathBaseboardContextBinding
	name := strings.TrimSpace(dictionary.row.name)
	if !energyExplanationPlanUsesEnergyPath(plan) || dictionary.isMeter || !energyPathIsBaseboardContextVariable(name) ||
		(!strings.EqualFold(strings.TrimSpace(dictionary.reportingFrequency), "Monthly") && !strings.EqualFold(strings.TrimSpace(dictionary.reportingFrequency), "Hourly")) {
		return binding, false
	}
	rate := strings.HasSuffix(normalizeEnergyOutputName(name), " rate")
	unit := "J"
	if rate {
		unit = "W"
	}
	if !strings.EqualFold(strings.TrimSpace(dictionary.row.units), unit) {
		return binding, false
	}
	count := 0
	for _, target := range targets {
		if strings.EqualFold(strings.TrimSpace(target.KeyValue), strings.TrimSpace(dictionary.row.keyValue)) && strings.TrimSpace(target.ZoneName) != "" {
			binding.Target = target
			count++
		}
	}
	if count != 1 {
		return energyPathBaseboardContextBinding{}, false
	}
	requested := false
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(strings.TrimSpace(output.ObjectType), "Output:Variable") ||
			!strings.EqualFold(strings.TrimSpace(output.KeyValue), strings.TrimSpace(dictionary.row.keyValue)) ||
			!strings.EqualFold(strings.TrimSpace(output.VariableName), name) || !strings.EqualFold(strings.TrimSpace(output.ReportingFrequency), "Monthly") {
			continue
		}
		if !purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) || !strings.EqualFold(strings.TrimSpace(output.ScopeZoneName), binding.Target.ZoneName) {
			return energyPathBaseboardContextBinding{}, false
		}
		requested = true
	}
	if !requested {
		return energyPathBaseboardContextBinding{}, false
	}
	binding.Name, binding.Units, binding.IntegratesRate = name, unit, rate
	binding.ObjectIndex = energyPathBaseboardContextOutputIndex(dictionary, plan, binding.Target)
	return binding, true
}

func energyPathBaseboardContextOutputIndex(dictionary energyExplanationDictionary, plan *PurposeRunPlan, target energyPathBaseboardTarget) *int {
	exact, wildcard := map[int]bool{}, map[int]bool{}
	for _, output := range plan.OutputObjects {
		if output.ObjectIndex == nil || !strings.EqualFold(strings.TrimSpace(output.ObjectType), "Output:Variable") ||
			!purposeIDsContain(output.PurposeIDs, SimulationPurposeBasicEnergy) || !energyExplanationOutputFrequencyMatchesDictionary(output, dictionary) ||
			!strings.EqualFold(strings.TrimSpace(output.VariableName), strings.TrimSpace(dictionary.row.name)) {
			continue
		}
		key := strings.TrimSpace(output.KeyValue)
		if (key == "*" || key == "") && (output.ScopeZoneName == "" || strings.EqualFold(strings.TrimSpace(output.ScopeZoneName), target.ZoneName)) {
			wildcard[*output.ObjectIndex] = true
		} else if strings.EqualFold(key, strings.TrimSpace(dictionary.row.keyValue)) && strings.EqualFold(strings.TrimSpace(output.ScopeZoneName), target.ZoneName) {
			exact[*output.ObjectIndex] = true
		}
	}
	// The run plan deduplicates request signatures; it cannot establish a
	// unique original opener when the untouched IDF contains two wildcards.
	for _, output := range target.OriginalOutputs {
		if !strings.EqualFold(output.Name, strings.TrimSpace(dictionary.row.name)) || !strings.EqualFold(output.Frequency, strings.TrimSpace(dictionary.reportingFrequency)) {
			continue
		}
		if output.KeyValue == "*" || output.KeyValue == "" {
			wildcard[output.ObjectIndex] = true
		} else if strings.EqualFold(output.KeyValue, strings.TrimSpace(dictionary.row.keyValue)) {
			exact[output.ObjectIndex] = true
		}
	}
	indices := exact
	if len(indices) == 0 {
		indices = wildcard
	}
	if len(indices) != 1 {
		return nil
	}
	for index := range indices {
		value := index
		return &value
	}
	return nil
}

func energyPathBaseboardContextSeriesForBuilder(builder *energyExplanationSeriesBuilder, sourceID string) energyExplanationSeries {
	binding := builder.dictionary.baseboardContext
	if binding == nil {
		return energyExplanationSeries{}
	}
	return energyExplanationSeries{Level: "context", Stage: "context", Kind: "context.baseboard", Label: binding.Name, Unit: builder.unit,
		ZoneName: binding.Target.ZoneName, ServiceKind: "heating", Basis: "measured_energy_variable", SourceIDs: []string{sourceID},
		Total: roundedEnergyNumber(builder.total), Monthly: roundedEnergyExplanationMonthly(builder.monthly), Daily: roundedEnergyExplanationDaily(builder.daily), Hourly: roundedEnergyExplanationHourly(builder.hourly),
		SelectedRange: roundedEnergyNumber(builder.selectedRange), HasSelectedRange: builder.hasSelectedRange,
		sourceKeyValue: strings.TrimSpace(builder.dictionary.row.keyValue), sourceName: strings.TrimSpace(builder.dictionary.row.name), sourceFrequency: strings.TrimSpace(builder.dictionary.reportingFrequency), sourceIsRate: binding.IntegratesRate,
		RelatedEntityIDs: []string{binding.Target.Component.ID}}
}

// Remove EVERY native context series before load selection, including zero and
// invalid/all-NULL rows. Source knownness comes exclusively from the SQL reader;
// a placeholder builder must never turn an absent observation into measured0.
func bindEnergyPathBaseboardContextSeries(series []energyExplanationSeries, sources []EnergyDataSource, context energyDriverBuildContext) ([]energyExplanationSeries, []EnergyDataSource, []EnergyWarning) {
	if !context.Enabled {
		return series, sources, nil
	}
	owners := map[string][]energyPathBaseboardTarget{}
	for _, target := range context.BaseboardTargets {
		key := normalizePurposeToken(target.KeyValue)
		owners[key] = append(owners[key], target)
	}
	sourceIndex := map[string]int{}
	for index, source := range sources {
		sourceIndex[source.ID] = index
	}
	out := make([]energyExplanationSeries, 0, len(series))
	var warnings []EnergyWarning
	for _, item := range series {
		name := firstNonEmpty(item.SourceName, item.sourceName)
		if !energyPathIsBaseboardContextVariable(name) {
			out = append(out, item)
			continue
		}
		targets := owners[normalizePurposeToken(firstNonEmpty(item.SourceKey, item.sourceKeyValue))]
		owned := len(targets) == 1
		for _, sourceID := range item.SourceIDs {
			index, exists := sourceIndex[sourceID]
			if !exists {
				continue
			}
			source := &sources[index]
			source.DriverRole, source.InspectorSection = energyDriverSourceRoleContext, energyDriverInspectorSectionContext
			source.DriverCategory, source.DriverComponent, source.HeatDirection = "load.heating", "load.baseboard_response.combined", "heating"
			source.Explanation = "Native baseboard LoadMet response is already included in Zone Air System Sensible heating/cooling. Retained as non-additive equipment context; not an additional Zone load, not a passive surface decomposition, and not generated heat divided by efficiency."
			if normalizeEnergyOutputName(name) == "baseboard electricity rate" {
				source.DriverCategory, source.DriverComponent = "energy.heating", "energy.baseboard_electricity_rate"
				source.Explanation = "Native baseboard electricity rate retained as non-additive context. Monthly Electricity Energy is the direct consumption authority; integrating this companion does not create additional site energy."
			}
			source.Formula, source.InputSourceIDs = "", nil
			if owned {
				source.ZoneName = targets[0].ZoneName
				source.RelatedEntityIDs = appendUniqueStrings(source.RelatedEntityIDs, targets[0].Component.ID)
			} else {
				source.ZoneName = ""
				source.Explanation = "Native baseboard output retained as non-additive context without a unique original equipment owner; not a Zone load or direct consumption proof."
			}
			source.EffectiveMultiplier, source.MultiplierApplication = 1, energyMultiplierAlreadyModelTotal
			if energyDataSourceValueKnown(*source, energySourceObservedRaw) {
				source.EffectiveValue = source.RawValue
				source.observedValuePresence |= energySourceObservedEffective
			}
		}
		if !owned {
			warnings = appendEnergyDriverWarning(warnings, EnergyWarning{Severity: "warning", Code: "baseboard_context_owner_unresolved", Message: "Native baseboard context has no unique original owner: " + firstNonEmpty(item.SourceKey, item.sourceKeyValue)})
		}
	}
	return out, sources, warnings
}
