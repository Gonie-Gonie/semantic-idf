package simulation

// This marker protects an observation from graph backfill.
// It grants neither native identity, numerical presence, nor accounting flow.
import (
	"bytes"
	"encoding/json"
	"math"
)

const energyPathPVObservationProtectionSchema = "semantic-idf.native-electrical-observation/v1"

type energyPathPVObservationProtection struct {
	Schema            string `json:"schema"`
	Invalid           bool   `json:"invalid,omitempty"`
	LegacyContextOnly bool   `json:"legacyContextOnly,omitempty"`
}

func cloneEnergyPathPVObservationProtection(input *energyPathPVObservationProtection) *energyPathPVObservationProtection {
	if input == nil {
		return nil
	}
	copy := *input
	return &copy
}

// Call with the raw field only when present. Explicit null, unknown schemas,
// wrong types and future fields remain a denial; they never revert to legacy.
func decodeEnergyPathPVObservationProtection(raw json.RawMessage) *energyPathPVObservationProtection {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	out := &energyPathPVObservationProtection{}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		out.Invalid = true
		return out
	}
	if err := json.Unmarshal(fields["schema"], &out.Schema); err != nil {
		out.Invalid = true
	}
	if rawInvalid, present := fields["invalid"]; present {
		if bytes.Equal(bytes.TrimSpace(rawInvalid), []byte("null")) || json.Unmarshal(rawInvalid, &out.Invalid) != nil {
			out.Invalid = true
		}
	}
	if rawLegacy, present := fields["legacyContextOnly"]; present {
		out.LegacyContextOnly = true
		var legacy bool
		if bytes.Equal(bytes.TrimSpace(rawLegacy), []byte("null")) || json.Unmarshal(rawLegacy, &legacy) != nil {
			out.Invalid = true
		} else {
			out.LegacyContextOnly = legacy
		}
	}
	for name := range fields {
		if name != "schema" && name != "invalid" && name != "legacyContextOnly" {
			out.Invalid = true
		}
	}
	if out.Schema != energyPathPVObservationProtectionSchema {
		out.Invalid = true
	}
	return out
}

func energyPathPVSourceObservationProtected(source EnergyDataSource) bool {
	return source.pvObservationProtection != nil
}

// Root applies this ONLY to C's exact revalidated SourceSnapshots, including
// explicitly unknown snapshots. It is never inferred from a source's name,
// DriverRole, value, graph usage, or factor. Inventory proof remains separate.
func protectEnergyPathPVSourceObservation(source EnergyDataSource) EnergyDataSource {
	source.pvObservationProtection = &energyPathPVObservationProtection{Schema: energyPathPVObservationProtectionSchema}
	return energyPathPVSourceObservationPresentation(source)
}

// Native evidence retains unrounded sums. The public snapshot uses the existing
// v2 three-decimal scalar transport once, never a sum of rounded Hourly points.
// Presence is checked independently for Raw and Effective, including real zero.
func energyPathPVSourceObservationPresentation(source EnergyDataSource) EnergyDataSource {
	if !energyPathPVSourceObservationProtected(source) {
		return source
	}
	if source.pvObservationProtection.LegacyContextOnly {
		return energyPathPVLegacyContextPresentation(source)
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
	for _, field := range []struct {
		value *float64
		bit   uint8
	}{{&source.RawValue, energySourceObservedRaw}, {&source.EffectiveValue, energySourceObservedEffective}} {
		if !energyDataSourceValueKnown(source, field.bit) || math.IsNaN(*field.value) || math.IsInf(*field.value, 0) {
			*field.value = 0
			source.observedValuePresence &^= field.bit
			source.inspectorValuePresence &^= field.bit
			continue
		}
		*field.value = roundedEnergyNumber(*field.value)
	}
	// These quantities belong to this measured source, not to an allocated node
	// or a selected Zone. Request ObjectIndex and native identity are retained.
	source.ZoneName, source.DriverCategory, source.DriverComponent, source.HeatDirection = "", "", "", ""
	source.DriverRole, source.InspectorSection = energyDriverSourceRoleContext, energyDriverInspectorSectionContext
	source.AggregationBasis, source.MultiplierApplication, source.EffectiveMultiplier = "model_total", "already_model_total", 1
	source.AllocationApplied, source.AllocationFactor, source.AllocatedValue = false, 0, 0
	source.AllocationExplanation, source.AllocationFormula, source.Formula = "", "", ""
	source.InputSourceIDs, source.RelatedEntityIDs, source.ScopeDetails = nil, nil, nil
	return source
}

func energyPathPVProtectedSourceScalar(source EnergyDataSource, bit uint8) *float64 {
	if !energyPathPVSourceObservationProtected(source) || !energyDataSourceValueKnown(source, bit) {
		return nil
	}
	var value float64
	switch bit {
	case energySourceObservedRaw:
		value = source.RawValue
	case energySourceObservedEffective:
		value = source.EffectiveValue
	default:
		return nil
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	value = roundedEnergyNumber(value)
	return &value
}

func marshalEnergyPathPVProtectedSource(source EnergyDataSource) ([]byte, error) {
	if !energyPathPVSourceObservationProtected(source) {
		return json.Marshal(source)
	}
	if source.pvObservationProtection.LegacyContextOnly {
		return marshalEnergyPathPVLegacyContextSource(source)
	}
	source = energyPathPVSourceObservationPresentation(source)
	type plainSource EnergyDataSource
	return json.Marshal(struct {
		plainSource
		RawValue                    *float64                           `json:"rawValue,omitempty"`
		EffectiveValue              *float64                           `json:"effectiveValue,omitempty"`
		NativeElectricalObservation *energyPathPVObservationProtection `json:"nativeElectricalObservation"`
	}{plainSource(source), energyPathPVProtectedSourceScalar(source, energySourceObservedRaw), energyPathPVProtectedSourceScalar(source, energySourceObservedEffective), source.pvObservationProtection})
}

// The frozen V1 writer omits measured zeros. Override only marked source rows;
// every unmarked result takes the exact former default writer byte path. Do not
// add a global EnergyDataSource.MarshalJSON: its anonymous embedding inside
// existing source wrappers would promote that method over their scalar fields.
func (result EnergyExplanationV1) MarshalJSON() ([]byte, error) {
	type plainV1 EnergyExplanationV1
	protected := false
	for _, source := range result.Sources {
		protected = protected || energyPathPVSourceObservationProtected(source)
	}
	if !protected {
		return json.Marshal(plainV1(result))
	}
	sources := make([]json.RawMessage, len(result.Sources))
	for index, source := range result.Sources {
		raw, err := marshalEnergyPathPVProtectedSource(source)
		if err != nil {
			return nil, err
		}
		sources[index] = raw
	}
	return json.Marshal(struct {
		plainV1
		Sources []json.RawMessage `json:"sources,omitempty"`
	}{plainV1(result), sources})
}
