package simulation

import (
	"reflect"
	"strings"
	"sync"
	"testing"
)

// This is the pre-index lookup algorithm, deliberately independent of the
// production indexes. Catalog iteration order defines the expected answer.
func linearEnergyAliasLookup[T any](name string, definitions []T, aliases func(T) []string) (T, bool) {
	key := normalizeEnergyOutputName(name)
	for _, definition := range definitions {
		for _, alias := range aliases(definition) {
			if normalizeEnergyOutputName(alias) == key {
				return definition, true
			}
		}
	}
	var zero T
	return zero, false
}

func checkEnergyAliasIndexAgainstLinear[T any](t *testing.T, definitions []T, aliases func(T) []string, lookup func(string) (T, bool)) {
	t.Helper()
	names := []string{"", "  ", "Not a known output", "Private Surface 01", "Unknown:Electricity", "Heating:UnknownFuel"}
	for _, definition := range definitions {
		for _, alias := range aliases(definition) {
			names = append(names, alias, strings.ToUpper(alias), " \t"+alias+"\n", strings.ReplaceAll(alias, " ", "\t\u00a0\u2003 "), alias+" unknown suffix")
		}
	}
	for _, name := range names {
		want, wantOK := linearEnergyAliasLookup(name, definitions, aliases)
		got, gotOK := lookup(name)
		if gotOK != wantOK || !reflect.DeepEqual(got, want) {
			t.Fatalf("lookup %q differs from catalog-order reference: got %#v/%v, want %#v/%v", name, got, gotOK, want, wantOK)
		}
	}
}

func TestEnergyAliasIndexesMatchEveryCatalogEntry(t *testing.T) {
	t.Run("meter", func(t *testing.T) {
		checkEnergyAliasIndexAgainstLinear(t, energyMeterAliasCatalog(), func(d energyMeterAliasDefinition) []string { return d.Aliases }, energyMeterAliasDefinitionForName)
	})
	t.Run("variable then direct use", func(t *testing.T) {
		checkEnergyAliasIndexAgainstLinear(t, append(energyVariableAliasCatalog(), energyPathDirectUseVariableAliasCatalog()...), func(d energyMeterAliasDefinition) []string { return d.Aliases }, energyVariableAliasDefinitionForName)
	})
	t.Run("load", func(t *testing.T) {
		checkEnergyAliasIndexAgainstLinear(t, energyLoadAliasCatalog(), func(d energyLoadAliasDefinition) []string { return d.Aliases }, energyLoadAliasDefinitionForName)
	})
	t.Run("heat", func(t *testing.T) {
		checkEnergyAliasIndexAgainstLinear(t, energyHeatAliasCatalog(), func(d energyHeatAliasDefinition) []string { return d.Aliases }, energyHeatAliasDefinitionForName)
	})
}

func TestEnergyAliasIndexesPreserveFirstDefinitionAndOwnCatalogSlices(t *testing.T) {
	meters := []energyMeterAliasDefinition{
		{Kind: "first", Aliases: []string{" X ", "x"}, LegacyAliases: []string{}, OutputRequestAliases: []string{"legacy"}},
		{Kind: "second", Aliases: []string{"x", "y"}},
	}
	loads := []energyLoadAliasDefinition{{Kind: "first", Aliases: []string{" X ", "x"}, LegacyAliases: []string{}}, {Kind: "second", Aliases: []string{"x", "y"}}}
	heat := []energyHeatAliasDefinition{{Kind: "first", Aliases: []string{" X ", "x"}, OutputRequestAliases: []string{}}, {Kind: "second", Aliases: []string{"x", "y"}}}
	meterIndex, loadIndex, heatIndex := indexEnergyMeterAliases(meters), indexEnergyLoadAliases(loads), indexEnergyHeatAliases(heat)
	if len(meterIndex) != 2 || len(loadIndex) != 2 || len(heatIndex) != 2 || meterIndex["x"].Kind != "first" || loadIndex["x"].Kind != "first" || heatIndex["x"].Kind != "first" || meterIndex["y"].Kind != "second" || loadIndex["y"].Kind != "second" || heatIndex["y"].Kind != "second" {
		t.Fatal("normalized duplicate aliases must keep the first catalog definition")
	}
	meters[0].Aliases[0], meters[0].OutputRequestAliases[0] = "mutated", "mutated"
	loads[0].Aliases[0], heat[0].Aliases[0] = "mutated", "mutated"
	if meterIndex["x"].Aliases[0] != " X " || meterIndex["x"].OutputRequestAliases[0] != "legacy" || loadIndex["x"].Aliases[0] != " X " || heatIndex["x"].Aliases[0] != " X " {
		t.Fatal("index retained mutable constructor inputs")
	}
	if meterIndex["x"].LegacyAliases == nil || loadIndex["x"].LegacyAliases == nil || heatIndex["x"].OutputRequestAliases == nil || meterIndex["y"].LegacyAliases != nil || heatIndex["y"].OutputRequestAliases != nil {
		t.Fatal("index construction changed nil versus non-nil empty slices")
	}
	for _, names := range [][]string{nil, {}, {"one", "two"}} {
		copy := cloneEnergyAliasNames(names)
		if !reflect.DeepEqual(copy, names) {
			t.Fatal("clone changed nil/empty/value semantics")
		}
		if len(copy) > 0 {
			copy[0] = "changed"
			if names[0] == "changed" {
				t.Fatal("clone aliases its input")
			}
		}
	}
}

func TestEnergyAliasLookupReturnsIndependentSlices(t *testing.T) {
	mutate := func(names []string) {
		for index := range names {
			names[index] = "mutated returned alias"
		}
	}
	for _, pair := range []struct {
		definitions []energyMeterAliasDefinition
		lookup      func(string) (energyMeterAliasDefinition, bool)
	}{
		{energyMeterAliasCatalog(), energyMeterAliasDefinitionForName},
		{append(energyVariableAliasCatalog(), energyPathDirectUseVariableAliasCatalog()...), energyVariableAliasDefinitionForName},
	} {
		for _, expected := range pair.definitions {
			for _, name := range expected.Aliases {
				got, _ := pair.lookup(name)
				mutate(got.Aliases)
				mutate(got.LegacyAliases)
				mutate(got.OutputRequestAliases)
				got, _ = pair.lookup(name)
				want, _ := linearEnergyAliasLookup(name, pair.definitions, func(d energyMeterAliasDefinition) []string { return d.Aliases })
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("meter/variable lookup %q exposed shared mutable slices", name)
				}
			}
		}
	}
	for _, expected := range energyLoadAliasCatalog() {
		for _, name := range expected.Aliases {
			got, _ := energyLoadAliasDefinitionForName(name)
			mutate(got.Aliases)
			mutate(got.LegacyAliases)
			got, _ = energyLoadAliasDefinitionForName(name)
			want, _ := linearEnergyAliasLookup(name, energyLoadAliasCatalog(), func(d energyLoadAliasDefinition) []string { return d.Aliases })
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("load lookup %q exposed shared mutable slices", name)
			}
		}
	}
	for _, expected := range energyHeatAliasCatalog() {
		for _, name := range expected.Aliases {
			got, _ := energyHeatAliasDefinitionForName(name)
			mutate(got.Aliases)
			mutate(got.OutputRequestAliases)
			got, _ = energyHeatAliasDefinitionForName(name)
			want, _ := linearEnergyAliasLookup(name, energyHeatAliasCatalog(), func(d energyHeatAliasDefinition) []string { return d.Aliases })
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("heat lookup %q exposed shared mutable slices", name)
			}
		}
	}
}

func TestEnergyAliasIndexesSupportParallelReadsWithoutGrowing(t *testing.T) {
	before := [4]int{len(energyAliasDefinitions.meter), len(energyAliasDefinitions.variable), len(energyAliasDefinitions.load), len(energyAliasDefinitions.heat)}
	var workers sync.WaitGroup
	for worker := 0; worker < 12; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for iteration := 0; iteration < 200; iteration++ {
				meter, meterOK := energyMeterAliasDefinitionForName("Gas:Facility")
				variable, variableOK := energyVariableAliasDefinitionForName("Zone Lights Electric Energy")
				load, loadOK := energyLoadAliasDefinitionForName("Zone Air System Sensible Cooling Energy")
				heat, heatOK := energyHeatAliasDefinitionForName("Zone Infiltration Sensible Heat Gain Rate")
				if !meterOK || !variableOK || !loadOK || !heatOK || meter.Carrier != "natural_gas" || variable.HierarchyLevel != "zone_direct_use" || load.ServiceKind != "cooling" || heat.Kind != "heat.infiltration" {
					t.Error("parallel lookup changed a definition")
					return
				}
				meter.Aliases[0], variable.Aliases[0], load.Aliases[0], heat.Aliases[0] = "local", "local", "local", "local"
				unknown := "private model source " + strings.Repeat("x", iteration)
				_, a := energyMeterAliasDefinitionForName(unknown)
				_, b := energyVariableAliasDefinitionForName(unknown)
				_, c := energyLoadAliasDefinitionForName(unknown)
				_, d := energyHeatAliasDefinitionForName(unknown)
				if a || b || c || d {
					t.Error("unknown name became a catalog alias")
					return
				}
			}
		}()
	}
	workers.Wait()
	after := [4]int{len(energyAliasDefinitions.meter), len(energyAliasDefinitions.variable), len(energyAliasDefinitions.load), len(energyAliasDefinitions.heat)}
	if before != after {
		t.Fatal("arbitrary input names expanded the fixed catalog indexes")
	}
}

var benchmarkEnergyAliasFound bool

func BenchmarkEnergyAliasHeatLookup(b *testing.B) {
	for _, name := range []string{"AFN Zone Mixing Latent Heat Gain Rate", "Unrecognized Surface A-19"} {
		b.Run(name+"/linear", func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				_, benchmarkEnergyAliasFound = linearEnergyAliasLookup(name, energyHeatAliasCatalog(), func(d energyHeatAliasDefinition) []string { return d.Aliases })
			}
		})
		b.Run(name+"/indexed", func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				_, benchmarkEnergyAliasFound = energyHeatAliasDefinitionForName(name)
			}
		})
	}
}

func BenchmarkEnergyAliasMeterLookup(b *testing.B) {
	for _, name := range []string{"Heating:Steam", "Surface Inside Face Convection Heat Gain Energy"} {
		b.Run(name+"/linear", func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				_, benchmarkEnergyAliasFound = linearEnergyAliasLookup(name, energyMeterAliasCatalog(), func(d energyMeterAliasDefinition) []string { return d.Aliases })
			}
		})
		b.Run(name+"/indexed", func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				_, benchmarkEnergyAliasFound = energyMeterAliasDefinitionForName(name)
			}
		})
	}
}
