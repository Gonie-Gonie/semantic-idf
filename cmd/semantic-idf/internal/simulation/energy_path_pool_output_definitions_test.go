package simulation

import (
	"reflect"
	"strings"
	"testing"
)

func TestEnergyPathPoolNativeOutputDefinitionsKeepIndependentBoundaries(t *testing.T) {
	want := map[string][3]string{
		"pool.water_heating":           {"Indoor Pool Water Heating Energy", "Indoor Pool Water Heating Rate", ""},
		"boiler.heating_output":        {"Boiler Heating Energy", "Boiler Heating Rate", ""},
		"boiler.natural_gas":           {"Boiler NaturalGas Energy", "Boiler NaturalGas Rate", "natural_gas"},
		"boiler.ancillary_natural_gas": {"Boiler Ancillary NaturalGas Energy", "Boiler Ancillary NaturalGas Rate", "natural_gas"},
		"boiler.ancillary_electricity": {"Boiler Ancillary Electricity Energy", "Boiler Ancillary Electricity Rate", "electricity"},
		"pump.electricity":             {"Pump Electricity Energy", "Pump Electricity Rate", "electricity"},
	}
	definitions := energyPathPoolOutputDefinitions()
	seen := map[string]bool{}
	if len(definitions) != len(want) {
		t.Fatalf("native definition families=%d want%d", len(definitions), len(want))
	}
	for _, definition := range definitions {
		expected, exists := want[definition.ID]
		if !exists || seen[definition.ID] || [3]string{definition.EnergyName, definition.RateName, definition.Carrier} != expected {
			t.Fatalf("changed native family: %+v", definition)
		}
		seen[definition.ID] = true
		thermal := definition.ID == "pool.water_heating" || definition.ID == "boiler.heating_output"
		if thermal {
			if definition.Role != energyPathPoolThermalContext || definition.Carrier != "" || definition.EndUse != "" || definition.ServiceKind != "" {
				t.Fatalf("thermal transfer became purchased consumption or Zone-air service: %+v", definition)
			}
		} else if definition.Role != energyPathPoolPurchasedConstituent {
			t.Fatalf("purchased native constituent lost its distinct role: %+v", definition)
		}
		if definition.ID == "pump.electricity" {
			if definition.EndUse != "pumps" || definition.ServiceKind != "" {
				t.Fatalf("pump output name alone invented a heating/cooling recipient: %+v", definition)
			}
		} else if !thermal && (definition.EndUse != "heating" || definition.ServiceKind != "heating") {
			t.Fatalf("boiler constituent lost native Heating meter membership: %+v", definition)
		}
		for _, isRate := range []bool{false, true} {
			name, unit := definition.EnergyName, "J"
			if isRate {
				name, unit = definition.RateName, "W"
			}
			got, rate, found := energyPathPoolOutputDefinitionForName(" \t" + strings.ToLower(name) + " ")
			if !found || rate != isRate || !reflect.DeepEqual(got, definition) || got.nativeUnit(rate) != unit {
				t.Fatalf("exact native name/unit contract changed: %q, %+v, rate=%v found=%v", name, got, rate, found)
			}
		}
	}
	definitions[0].EnergyName = "mutated"
	if energyPathPoolOutputDefinitions()[0].EnergyName != "Indoor Pool Water Heating Energy" {
		t.Fatal("definition lookup exposed shared mutable state")
	}
}

func TestEnergyPathPoolOutputNamesDoNotAuthorizeOtherNativeBoundaries(t *testing.T) {
	if (energyPathPoolOutputDefinition{}).matchesOriginal("", "") {
		t.Fatal("absent definition acquired an original identity")
	}
	for _, name := range []string{"", "Indoor Pool Miscellaneous Equipment Energy", "Indoor Pool Miscellaneous Equipment Power", "Boiler Gas Energy", "Boiler NaturalGas Power", "Pump Fluid Heat Gain Energy", "Unknown Pump Electricity Energy", "Heating:NaturalGas"} {
		if _, _, found := energyPathPoolOutputDefinitionForName(name); found {
			t.Fatalf("unreviewed output accepted: %q", name)
		}
	}
	for _, definition := range energyPathPoolOutputDefinitions() {
		if !definition.matchesOriginal(" "+strings.ToLower(definition.ObjectType)+" ", " "+strings.ToLower(definition.FuelType)+" ") {
			t.Fatalf("exact native original rejected: %+v", definition)
		}
		if definition.matchesOriginal("Unknown:"+definition.ObjectType, definition.FuelType) {
			t.Fatalf("foreign typed original accepted: %+v", definition)
		}
		if definition.FuelType != "" {
			for _, fuel := range []string{"", "Electricity", "Propane", "FuelOilNo2"} {
				if definition.matchesOriginal(definition.ObjectType, fuel) {
					t.Fatalf("unreviewed or absent fuel borrowed NaturalGas proof: %+v / %q", definition, fuel)
				}
			}
		}
	}
}
