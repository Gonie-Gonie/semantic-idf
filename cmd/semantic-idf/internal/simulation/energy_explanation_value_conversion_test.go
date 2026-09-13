package simulation

import (
	"math"
	"testing"
)

func TestPreparedEnergyConversionMatchesOriginalArithmetic(t *testing.T) {
	for _, unit := range []string{"J", "kJ", "MJ", "GJ", "Wh", "kWh", " W ", " k_W ", "", "unknown", "C", "kg/s"} {
		for _, name := range []string{"Surface Inside Face Convection Heat Gain Energy", "  Surface  Inside Face Convection Heat Gain RATE ", "Heating Energy"} {
			for _, family := range []string{"heat", "load", "energy", "meter", "unknown"} {
				original := energyExplanationDictionary{row: sqlOutputDictionaryRow{name: name, units: unit}}
				switch family {
				case "heat":
					original.heat = &energyHeatAliasDefinition{}
				case "load":
					original.load = &energyLoadAliasDefinition{}
				case "energy":
					original.energy = &energyMeterAliasDefinition{}
				case "meter":
					original.meter = &energyMeterAliasDefinition{}
				}
				prepared := original
				conversion := prepareEnergyExplanationValueConversion(original)
				prepared.valueConversion = &conversion
				if energyExplanationIntegratesRate(original) != energyExplanationIntegratesRate(prepared) {
					t.Fatalf("rate classification changed: %s / %s / %s", family, name, unit)
				}
				for _, value := range []float64{0, math.Copysign(0, -1), .0005, -.0005, 1234.56789, -1234.56789, math.MaxFloat64, math.SmallestNonzeroFloat64, math.Inf(1), math.Inf(-1), math.NaN()} {
					for _, hours := range []float64{1, .25, 730, 8760, 0, -1, math.NaN(), math.Inf(1)} {
						want, wantUnit := energyExplanationSQLValue(value, original, hours)
						got, gotUnit := energyExplanationSQLValue(value, prepared, hours)
						if math.Float64bits(want) != math.Float64bits(got) || wantUnit != gotUnit {
							t.Fatalf("conversion changed: %s / %s / %s, value=%g hours=%g: got %.17g %q, want %.17g %q", family, name, unit, value, hours, got, gotUnit, want, wantUnit)
						}
					}
				}
			}
		}
	}
}
