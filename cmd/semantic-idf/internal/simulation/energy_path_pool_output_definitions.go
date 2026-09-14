package simulation

import "strings"

// These are native v25.1 names, not a whole-model capability declaration.
// Original typed reporting identity and actual fuel qualify output requests.
// Full water/air ownership and demand census are separate allocation gates;
// an incomplete service route must not hide an independently named observation.
// In particular, two Pump:VariableSpeed targets retain distinct native keys.
type energyPathPoolOutputDefinition struct {
	ID          string
	ObjectType  string
	FuelType    string
	EnergyName  string
	RateName    string
	Role        string
	EndUse      string
	ServiceKind string
	Carrier     string
}

const (
	energyPathPoolThermalContext       = "thermal_context"
	energyPathPoolPurchasedConstituent = "purchased_constituent"
)

func energyPathPoolOutputDefinitions() []energyPathPoolOutputDefinition {
	return []energyPathPoolOutputDefinition{
		{ID: "pool.water_heating", ObjectType: "SwimmingPool:Indoor",
			EnergyName: "Indoor Pool Water Heating Energy", RateName: "Indoor Pool Water Heating Rate",
			Role: energyPathPoolThermalContext},
		{ID: "boiler.heating_output", ObjectType: "Boiler:HotWater", FuelType: "NaturalGas",
			EnergyName: "Boiler Heating Energy", RateName: "Boiler Heating Rate",
			Role: energyPathPoolThermalContext},
		{ID: "boiler.natural_gas", ObjectType: "Boiler:HotWater", FuelType: "NaturalGas",
			EnergyName: "Boiler NaturalGas Energy", RateName: "Boiler NaturalGas Rate",
			Role: energyPathPoolPurchasedConstituent, EndUse: "heating", ServiceKind: "heating", Carrier: "natural_gas"},
		{ID: "boiler.ancillary_natural_gas", ObjectType: "Boiler:HotWater", FuelType: "NaturalGas",
			EnergyName: "Boiler Ancillary NaturalGas Energy", RateName: "Boiler Ancillary NaturalGas Rate",
			Role: energyPathPoolPurchasedConstituent, EndUse: "heating", ServiceKind: "heating", Carrier: "natural_gas"},
		{ID: "boiler.ancillary_electricity", ObjectType: "Boiler:HotWater", FuelType: "NaturalGas",
			EnergyName: "Boiler Ancillary Electricity Energy", RateName: "Boiler Ancillary Electricity Rate",
			Role: energyPathPoolPurchasedConstituent, EndUse: "heating", ServiceKind: "heating", Carrier: "electricity"},
		{ID: "pump.electricity", ObjectType: "Pump:VariableSpeed",
			EnergyName: "Pump Electricity Energy", RateName: "Pump Electricity Rate",
			Role: energyPathPoolPurchasedConstituent, EndUse: "pumps", Carrier: "electricity"},
	}
}

// Exact name discovery alone never proves source ownership or knownness. Rate
// companions are non-additive even for a purchased constituent. Thermal output
// never acquires an end use, service conversion or purchased carrier here.
func energyPathPoolOutputDefinitionForName(name string) (definition energyPathPoolOutputDefinition, isRate, found bool) {
	name = strings.TrimSpace(name)
	for _, definition := range energyPathPoolOutputDefinitions() {
		if strings.EqualFold(name, definition.EnergyName) {
			return definition, false, true
		}
		if strings.EqualFold(name, definition.RateName) {
			return definition, true, true
		}
	}
	return energyPathPoolOutputDefinition{}, false, false
}

func (definition energyPathPoolOutputDefinition) matchesOriginal(objectType, fuelType string) bool {
	return definition.ID != "" && definition.ObjectType != "" &&
		strings.EqualFold(strings.TrimSpace(objectType), definition.ObjectType) &&
		(definition.FuelType == "" || strings.EqualFold(strings.TrimSpace(fuelType), definition.FuelType))
}

func (definition energyPathPoolOutputDefinition) nativeUnit(isRate bool) string {
	if isRate {
		return "W"
	}
	return "J"
}
