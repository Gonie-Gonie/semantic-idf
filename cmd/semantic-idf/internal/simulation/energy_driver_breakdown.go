package simulation

import (
	"bytes"
	"encoding/json"
	"math"
	"sort"
	"strings"
)

func (component *EnergyExplanationDriverComponent) UnmarshalJSON(data []byte) error {
	type plainComponent EnergyExplanationDriverComponent
	var decoded plainComponent
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields struct {
		Value          json.RawMessage `json:"value"`
		RawValue       json.RawMessage `json:"rawValue"`
		EffectiveValue json.RawMessage `json:"effectiveValue"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*component = EnergyExplanationDriverComponent(decoded)
	component.decodedFromJSON = true
	for i, field := range []json.RawMessage{fields.Value, fields.RawValue, fields.EffectiveValue} {
		if len(field) > 0 && !bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
			component.valuePresence |= 1 << i
		}
	}
	return nil
}

func energyDriverBreakdownLabel(component string) string {
	switch component {
	case "air_storage":
		return "Air energy storage"
	case "surface_balance":
		return "Surface heat-balance difference"
	case "outdoor_air_balance":
		return "Outdoor-air heat-balance difference"
	case "internal_gain_balance":
		return "Internal-gain heat-balance difference"
	case "other_internal_gains":
		return "Other internal gains"
	case "interzone_balance":
		return "Interzone balance"
	case "surface_exchange":
		return "Other surface heat exchange"
	case "unresolved_balance":
		return "Unresolved load balance"
	default:
		return ""
	}
}

func energyDriverComponentBreakdownForSeries(item energyExplanationSeries, raw, effective float64) []EnergyExplanationDriverComponent {
	if item.DriverCategory != energyDriverCategoryStorageOther && item.DriverCategory != energyDriverCategoryInternalOther && item.DriverCategory != energyDriverCategoryInterzoneAir && item.DriverCategory != energyDriverCategoryInterzoneSurfaces {
		return nil
	}
	component := "unresolved_balance"
	switch {
	case item.Kind == "heat.storage_air" || item.DriverComponent == "zone_air_storage":
		component = "air_storage"
	case item.Kind == "heat.surface_reconciliation" || strings.HasPrefix(item.DriverComponent, "surface.reconciliation_gap"):
		component = "surface_balance"
	case item.Kind == "heat.outdoor_air_reconciliation" || strings.HasPrefix(item.DriverComponent, "outdoor_air.reconciliation_difference"):
		component = "outdoor_air_balance"
	case item.Kind == "heat.internal_other_reconciliation" || strings.HasPrefix(item.DriverComponent, "internal.other.reconciliation_gap"):
		component = "internal_gain_balance"
	case item.DriverCategory == energyDriverCategoryInternalOther:
		component = "other_internal_gains"
	case item.Kind == "heat.interzone_building_residual" || strings.HasPrefix(item.DriverComponent, "interzone_air.") || item.DriverCategory == energyDriverCategoryInterzoneSurfaces || item.DriverCategory == energyDriverCategoryInterzoneAir:
		component = "interzone_balance"
	case item.Kind == "heat.surface_inside_face_convection":
		component = "surface_exchange"
	}
	return []EnergyExplanationDriverComponent{{Component: component, Label: energyDriverBreakdownLabel(component), RawValue: roundedEnergyNumber(raw), EffectiveValue: roundedEnergyNumber(effective), SourceIDs: appendUniqueStrings(item.SourceIDs, item.DriverInputSourceIDs...)}}
}

func cloneEnergyExplanationDriverComponents(input []EnergyExplanationDriverComponent) []EnergyExplanationDriverComponent {
	if len(input) == 0 {
		return nil
	}
	out := append([]EnergyExplanationDriverComponent(nil), input...)
	for i := range out {
		out[i].SourceIDs = appendUniqueStrings(nil, input[i].SourceIDs...)
	}
	return out
}

func mergeEnergyExplanationDriverComponents(current, next []EnergyExplanationDriverComponent) []EnergyExplanationDriverComponent {
	out := cloneEnergyExplanationDriverComponents(current)
	indexes := map[string]int{}
	for i, component := range out {
		indexes[component.Component] = i
	}
	for _, component := range next {
		if i, found := indexes[component.Component]; found {
			out[i].Value = roundedEnergyNumber(out[i].Value + component.Value)
			out[i].RawValue = roundedEnergyNumber(out[i].RawValue + component.RawValue)
			out[i].EffectiveValue = roundedEnergyNumber(out[i].EffectiveValue + component.EffectiveValue)
			out[i].SourceIDs = appendUniqueStrings(out[i].SourceIDs, component.SourceIDs...)
		} else {
			component.SourceIDs = appendUniqueStrings(nil, component.SourceIDs...)
			indexes[component.Component] = len(out)
			out = append(out, component)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Component < out[j].Component })
	return out
}

// The canonical parent allocation remains the authority. Subdivide only that
// fixed budget using its already signed, same-service pressure components. This
// cannot alter any load, source allocation, COP, closure or approved node ID.
func allocateEnergyExplanationDriverComponents(node *EnergyExplanationNode) {
	if node == nil || len(node.DriverBreakdown) == 0 {
		return
	}
	node.DriverBreakdown = cloneEnergyExplanationDriverComponents(node.DriverBreakdown)
	targets := make([]energyPathZoneAuxiliaryTarget, len(node.DriverBreakdown))
	for i, component := range node.DriverBreakdown {
		targets[i] = energyPathZoneAuxiliaryTarget{NodeID: component.Component, Value: component.EffectiveValue}
	}
	shares := energyPathFanPoolShares(node.AllocatedValue, targets)
	if len(shares) != len(targets) {
		// Unrepresentable inputs cannot gain a guessed numerical breakdown.
		node.DriverBreakdown = nil
		return
	}
	for i, value := range shares {
		node.DriverBreakdown[i].Value = value
	}
}

func scaleEnergyExplanationDriverComponents(input []EnergyExplanationDriverComponent, factor float64) []EnergyExplanationDriverComponent {
	out := cloneEnergyExplanationDriverComponents(input)
	for i := range out {
		out[i].Value = roundedEnergyNumber(out[i].Value * factor)
		out[i].RawValue = roundedEnergyNumber(out[i].RawValue * factor)
		out[i].EffectiveValue = roundedEnergyNumber(out[i].EffectiveValue * factor)
	}
	return out
}

// Stored optional detail is usable only as a complete, finite subdivision of
// the already authoritative parent. Unknown legacy detail remains absent.
func sanitizeEnergyExplanationDriverComponents(node *EnergyExplanationNode) {
	if len(node.DriverBreakdown) == 0 {
		return
	}
	valid := node.Level == "driver" && node.DriverCategory == energyDriverCategoryStorageOther && node.AllocationApplied
	seen, sourceSet := map[string]bool{}, map[string]bool{}
	for _, sourceID := range node.SourceIDs {
		sourceSet[sourceID] = true
	}
	value, raw, effective := 0.0, 0.0, 0.0
	for _, component := range node.DriverBreakdown {
		valid = valid && !seen[component.Component] && energyDriverBreakdownLabel(component.Component) != "" && len(component.SourceIDs) > 0 &&
			(!component.decodedFromJSON || component.valuePresence == 7) &&
			energyPathFinite(component.Value) && energyPathFinite(component.RawValue) && energyPathFinite(component.EffectiveValue) &&
			component.Value >= 0 && component.RawValue >= 0 && component.EffectiveValue >= 0
		seen[component.Component] = true
		for _, sourceID := range component.SourceIDs {
			valid = valid && sourceSet[sourceID]
		}
		value += component.Value
		raw += component.RawValue
		effective += component.EffectiveValue
	}
	valid = valid && math.Abs(roundedEnergyNumber(value)-node.Value) <= 1e-9 && math.Abs(roundedEnergyNumber(raw)-node.RawValue) <= 1e-9 && math.Abs(roundedEnergyNumber(effective)-node.EffectiveValue) <= 1e-9
	if !valid {
		node.DriverBreakdown = nil
		return
	}
	node.DriverBreakdown = cloneEnergyExplanationDriverComponents(node.DriverBreakdown)
	for i := range node.DriverBreakdown {
		node.DriverBreakdown[i].Label = energyDriverBreakdownLabel(node.DriverBreakdown[i].Component)
	}
}
