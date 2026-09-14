package simulation

// Legacy context protection. Denial-only protection for actual component SourceIDs
// removed from the graph outside the reviewed Monthly/Hourly reader contract.
// Root-owned marker/glue changes are listed in the accompanying note.
import (
	"encoding/json"
	"fmt"
	"math"
)

// The caller supplies IDs recorded while removing actual generic component
// series. No name, source-role, nonzero value, or missing M/H source is a grant.
// Existing measured-source protection is never downgraded to this legacy mode.
func protectEnergyPathPVLegacyContextSources(existing []EnergyDataSource, removedSourceIDs []string) []EnergyDataSource {
	removed := map[string]bool{}
	for _, id := range removedSourceIDs {
		if id != "" {
			removed[id] = true
		}
	}
	if len(removed) == 0 {
		return existing
	}
	out := append([]EnergyDataSource(nil), existing...)
	for index := range out {
		if !removed[out[index].ID] || energyPathPVSourceObservationProtected(out[index]) {
			continue
		}
		out[index].pvObservationProtection = &energyPathPVObservationProtection{
			Schema: energyPathPVObservationProtectionSchema, LegacyContextOnly: true,
		}
		out[index] = energyPathPVLegacyContextPresentation(out[index])
	}
	return out
}

// Keep the old scalar values, precision, presence, units, multiplier and source
// identity exactly as supplied. This mode proves no native observation. Only
// graph-derived allocation/Zone state is removed and future backfill denied.
func energyPathPVLegacyContextPresentation(source EnergyDataSource) EnergyDataSource {
	if source.pvObservationProtection == nil || !source.pvObservationProtection.LegacyContextOnly {
		return source
	}
	source.pvObservationProtection = cloneEnergyPathPVObservationProtection(source.pvObservationProtection)
	if source.ObjectIndex != nil {
		index := *source.ObjectIndex
		source.ObjectIndex = &index
	}
	if source.HourlyEnergy != nil {
		hourly := *source.HourlyEnergy
		hourly.Values = append([]float64(nil), hourly.Values...)
		source.HourlyEnergy = &hourly
	}
	source.ZoneName = ""
	source.DriverRole, source.InspectorSection = energyDriverSourceRoleContext, energyDriverInspectorSectionContext
	source.AllocationApplied, source.AllocationFactor, source.AllocatedValue = false, 0, 0
	// Do not erase a native/source formula unrelated to graph allocation.
	if source.AllocationFormula != "" && source.Formula == source.AllocationFormula {
		source.Formula = ""
	}
	source.AllocationExplanation, source.AllocationFormula = "", ""
	source.ScopeDetails = nil
	return source
}

// Preserve decoded field presence exactly. Before the first JSON, a legacy
// nonzero value with no runtime presence mask follows the old omitempty writer;
// emitting that already-existing number is not native validation. Real known
// zero is retained, while unknown zero never receives a field or presence bit.
func energyPathPVLegacyContextScalar(source EnergyDataSource, bit uint8) *float64 {
	var value float64
	switch bit {
	case energySourceObservedRaw:
		value = source.RawValue
	case energySourceObservedEffective:
		value = source.EffectiveValue
	default:
		return nil
	}
	if source.inspectorDecodedFromJSON && source.inspectorValuePresence&bit == 0 {
		return nil
	}
	if !energyDataSourceValueKnown(source, bit) && value == 0 {
		return nil
	}
	return &value
}

func marshalEnergyPathPVLegacyContextSource(source EnergyDataSource) ([]byte, error) {
	source = energyPathPVLegacyContextPresentation(source)
	// Match the old JSON writer's rejection, not silent omission of NaN/Inf.
	for _, value := range []float64{source.RawValue, source.EffectiveValue} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("non-finite legacy electrical source scalar")
		}
	}
	type plainSource EnergyDataSource
	return json.Marshal(struct {
		plainSource
		RawValue                    *float64                           `json:"rawValue,omitempty"`
		EffectiveValue              *float64                           `json:"effectiveValue,omitempty"`
		NativeElectricalObservation *energyPathPVObservationProtection `json:"nativeElectricalObservation"`
	}{plainSource(source), energyPathPVLegacyContextScalar(source, energySourceObservedRaw),
		energyPathPVLegacyContextScalar(source, energySourceObservedEffective), source.pvObservationProtection})
}
