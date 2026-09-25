package simulation

import (
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// These native system outputs have one non-Zone reporting key. They are not
// direct Zone equipment, generic Chiller aliases, or module-count-scaled input.
// The table does not itself request outputs, read SQL, or authorize allocation.
type energyPathCentralHeatPumpOutputDefinition struct {
	ID, EnergyName, RateName, Role, ServiceKind, EndUse, Carrier string
}

const (
	energyPathCentralHeatPumpPurchased = "purchased_constituent"
	energyPathCentralHeatPumpThermal   = "nonadditive_thermal_context"
)

func energyPathCentralHeatPumpOutputDefinitions() []energyPathCentralHeatPumpOutputDefinition {
	return []energyPathCentralHeatPumpOutputDefinition{
		{ID: "central_heat_pump.cooling.electricity", EnergyName: "Chiller Heater System Cooling Electricity Energy", RateName: "Chiller Heater System Cooling Electricity Rate", Role: energyPathCentralHeatPumpPurchased, ServiceKind: "cooling", EndUse: "cooling", Carrier: "electricity"},
		{ID: "central_heat_pump.heating.electricity", EnergyName: "Chiller Heater System Heating Electricity Energy", RateName: "Chiller Heater System Heating Electricity Rate", Role: energyPathCentralHeatPumpPurchased, ServiceKind: "heating", EndUse: "heating", Carrier: "electricity"},
		{ID: "central_heat_pump.cooling_transfer", EnergyName: "Chiller Heater System Cooling Energy", RateName: "Chiller Heater System Cooling Rate", Role: energyPathCentralHeatPumpThermal},
		{ID: "central_heat_pump.heating_transfer", EnergyName: "Chiller Heater System Heating Energy", RateName: "Chiller Heater System Heating Rate", Role: energyPathCentralHeatPumpThermal},
		{ID: "central_heat_pump.source_transfer", EnergyName: "Chiller Heater System Source Heat Transfer Energy", RateName: "Chiller Heater System Source Heat Transfer Rate", Role: energyPathCentralHeatPumpThermal},
	}
}

type energyPathCentralHeatPumpOutputTarget struct {
	System     idf.ComponentRef
	Definition energyPathCentralHeatPumpOutputDefinition
	Binding    idf.NativeCentralHeatPumpBinding
}

// Source identity must survive an unresolved physical route. This is a global
// context roster: never set ScopeZoneName/System.ZoneName, directComponentID,
// or effective multiplier from a served Zone, and never treat count 3 as factor3.
func energyPathCentralHeatPumpOutputTargets(doc idf.Document) []energyPathCentralHeatPumpOutputTarget {
	var targets []energyPathCentralHeatPumpOutputTarget
	for _, binding := range idf.ResolveNativeCentralHeatPumpBindings(doc) {
		if !binding.ReportingIdentityValid {
			continue
		}
		for _, definition := range energyPathCentralHeatPumpOutputDefinitions() {
			targets = append(targets, energyPathCentralHeatPumpOutputTarget{System: binding.System, Definition: definition, Binding: binding})
		}
	}
	return targets
}

// This is only the exact dictionary FAMILY gate. The caller must separately
// verify target membership in the original-derived roster, unique dictionary,
// original output opener, Type=Sum/TimestepType=HVAC System, weather axis,
// duplicate rows, finite nonnegative known values and source-local parent
// membership. Matching identity cannot turn absent/NULL into observed zero.
func energyPathCentralHeatPumpMonthlyBudgetIdentity(target energyPathCentralHeatPumpOutputTarget, name, key, unit, frequency string, isMeter bool) bool {
	if !target.Binding.ReportingIdentityValid || !strings.EqualFold(target.System.ObjectType, "CentralHeatPumpSystem") || !strings.EqualFold(target.Binding.System.ObjectType, target.System.ObjectType) ||
		target.System.ID == "" || target.System.ID != target.Binding.System.ID || target.System.ObjectIndex != target.Binding.System.ObjectIndex ||
		!strings.EqualFold(target.System.ObjectName, target.Binding.System.ObjectName) || target.System.ObjectName == "" || isMeter ||
		!strings.EqualFold(strings.TrimSpace(key), target.System.ObjectName) || !strings.EqualFold(strings.TrimSpace(unit), "J") ||
		!strings.EqualFold(strings.TrimSpace(frequency), "Monthly") {
		return false
	}
	for _, definition := range energyPathCentralHeatPumpOutputDefinitions() {
		if definition.Role == energyPathCentralHeatPumpPurchased && definition == target.Definition && strings.EqualFold(strings.TrimSpace(name), definition.EnergyName) {
			return true
		}
	}
	return false
}
