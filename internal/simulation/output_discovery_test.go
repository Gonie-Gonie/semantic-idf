package simulation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverAvailableOutputsFromSQLRDDMDDAndPurposeFallback(t *testing.T) {
	dir := t.TempDir()
	sqlPath := filepath.Join(dir, "eplusout.sql")
	createTestEnergySQL(t, sqlPath)
	rddPath := filepath.Join(dir, "eplusout.rdd")
	if err := os.WriteFile(rddPath, []byte("Zone,Average,Zone Mean Air Temperature [C]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mddPath := filepath.Join(dir, "eplusout.mdd")
	if err := os.WriteFile(mddPath, []byte("Electricity:Facility [J]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := DiscoverAvailableOutputs(OutputDiscoveryRequest{
		Text:            purposePlanFixtureIDF,
		OutputDirectory: dir,
		PurposeRequest:  &SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeComfort}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !discoveryHas(result.Items, "Output:Variable", "ZONE ONE", "Zone Lights Electricity Energy", "available") {
		t.Fatalf("missing SQL variable catalog item: %#v", result.Items)
	}
	if !discoveryHas(result.Items, "Output:Variable", "Zone", "Zone Mean Air Temperature", "available") {
		t.Fatalf("missing RDD variable catalog item: %#v", result.Items)
	}
	if !discoveryHas(result.Items, "Output:Meter", "", "Electricity:Facility", "available") {
		t.Fatalf("missing MDD meter catalog item: %#v", result.Items)
	}
	if !discoveryHas(result.Items, "Output:Variable", "*", "Zone Thermostat Heating Setpoint Temperature", "fallback") {
		t.Fatalf("missing purpose fallback catalog item: %#v", result.Items)
	}
	if result.Counts["available"] == 0 || result.Counts["fallback"] == 0 {
		t.Fatalf("counts = %#v", result.Counts)
	}
}

func TestDiscoverOutputsFromMDDParsesMeterMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.mdd")
	if err := os.WriteFile(path, []byte("Electricity:Facility [J]\nNaturalGas:Heating:Plant [J]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := discoverOutputsFromMDD(path)
	if err != nil {
		t.Fatal(err)
	}
	facility, ok := discoveryFind(items, "Output:Meter", "", "Electricity:Facility", "available")
	if !ok {
		t.Fatalf("missing facility meter: %#v", items)
	}
	if facility.ResourceType != "Electricity" || facility.EndUseCategory != "Facility" || facility.MeterGroup != "" {
		t.Fatalf("facility metadata = %#v", facility)
	}
	plant, ok := discoveryFind(items, "Output:Meter", "", "NaturalGas:Heating:Plant", "available")
	if !ok {
		t.Fatalf("missing grouped meter: %#v", items)
	}
	if plant.ResourceType != "NaturalGas" || plant.EndUseCategory != "Heating" || plant.MeterGroup != "Plant" {
		t.Fatalf("grouped metadata = %#v", plant)
	}
}

func TestDiscoverAvailableOutputsMarksPurposeMeterAvailableFromMDD(t *testing.T) {
	dir := t.TempDir()
	mddPath := filepath.Join(dir, "eplusout.mdd")
	if err := os.WriteFile(mddPath, []byte("Electricity:Facility [J]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := DiscoverAvailableOutputs(OutputDiscoveryRequest{
		Text:    purposePlanFixtureIDF,
		MDDPath: mddPath,
		PurposeRequest: &SimulationPurposeRequest{
			Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	item, ok := discoveryFind(result.Items, "Output:Meter", "Electricity:Facility", "Electricity:Facility", "available")
	if !ok {
		t.Fatalf("purpose meter should be available from MDD: %#v", result.Items)
	}
	if item.ResourceType != "Electricity" || item.EndUseCategory != "Facility" {
		t.Fatalf("available purpose meter metadata = %#v", item)
	}
}

func TestDiscoverOutputsCachedInvalidatesOnFileChange(t *testing.T) {
	outputDiscoveryCache.Lock()
	outputDiscoveryCache.items = map[string]outputDiscoveryCacheEntry{}
	outputDiscoveryCache.Unlock()

	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.rdd")
	if err := os.WriteFile(path, []byte("First Variable"), 0o644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	loader := func(path string) ([]OutputDiscoveryItem, error) {
		calls++
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return []OutputDiscoveryItem{{ObjectType: "Output:Variable", Name: string(content), Status: "available"}}, nil
	}

	items, err := discoverOutputsCached("rdd", path, loader)
	if err != nil {
		t.Fatal(err)
	}
	items[0].Name = "mutated"
	cachedItems, err := discoverOutputsCached("rdd", path, loader)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || cachedItems[0].Name != "First Variable" {
		t.Fatalf("cache hit = calls %d items %#v", calls, cachedItems)
	}
	if err := os.WriteFile(path, []byte("Second Variable With New Size"), 0o644); err != nil {
		t.Fatal(err)
	}
	updatedItems, err := discoverOutputsCached("rdd", path, loader)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || updatedItems[0].Name != "Second Variable With New Size" {
		t.Fatalf("cache miss = calls %d items %#v", calls, updatedItems)
	}
}

func TestDiscoverAvailableOutputsMarksPurposeAlias(t *testing.T) {
	dir := t.TempDir()
	rddPath := filepath.Join(dir, "eplusout.rdd")
	if err := os.WriteFile(rddPath, []byte("Zone,Average,Zone Air Temperature [C]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := DiscoverAvailableOutputs(OutputDiscoveryRequest{
		Text:    purposePlanFixtureIDF,
		RDDPath: rddPath,
		PurposeRequest: &SimulationPurposeRequest{
			Purposes: []SimulationPurposeID{SimulationPurposeZoneHeatFlow},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	item, ok := discoveryFind(result.Items, "Output:Variable", "*", "Zone Mean Air Temperature", "alias")
	if !ok || item.AliasOf != "Zone Air Temperature" {
		t.Fatalf("missing alias item: %#v", result.Items)
	}
	if result.Counts["alias"] == 0 {
		t.Fatalf("alias count missing: %#v", result.Counts)
	}
}

func TestDiscoverAvailableOutputsMarksPurposeMeterAlias(t *testing.T) {
	dir := t.TempDir()
	mddPath := filepath.Join(dir, "eplusout.mdd")
	if err := os.WriteFile(mddPath, []byte("Gas:Facility [J]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := DiscoverAvailableOutputs(OutputDiscoveryRequest{
		Text: purposePlanFixtureIDF + `
GasEquipment,
  Office Gas Equipment,
  Office,
  ,
  EquipmentLevel,
  100;
`,
		MDDPath: mddPath,
		PurposeRequest: &SimulationPurposeRequest{
			Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	item, ok := discoveryFind(result.Items, "Output:Meter", "NaturalGas:Facility", "NaturalGas:Facility", "alias")
	if !ok || item.AliasOf != "Gas:Facility" {
		t.Fatalf("missing meter alias item: %#v", result.Items)
	}
	if !discoveryHas(result.Items, "Output:Meter", "Electricity:Facility", "Electricity:Facility", "fallback") {
		t.Fatalf("purpose meter fallback should use meter name instead of empty variable name: %#v", result.Items)
	}
}

func TestDiscoverAvailableOutputsMarksUndiscoveredCustomOutputMissing(t *testing.T) {
	result, err := DiscoverAvailableOutputs(OutputDiscoveryRequest{
		Text: purposePlanFixtureIDF,
		PurposeRequest: &SimulationPurposeRequest{
			Purposes: []SimulationPurposeID{SimulationPurposeCustomOutputs},
			Scope: SimulationPurposeScope{CustomOutputs: []PurposeCustomOutput{{
				ObjectType:         "Output:Variable",
				KeyValue:           "*",
				VariableName:       "Not A Real Output Variable",
				ReportingFrequency: "Hourly",
			}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !discoveryHas(result.Items, "Output:Variable", "*", "Not A Real Output Variable", "missing") {
		t.Fatalf("custom output should be marked missing when no catalog entry exists: %#v", result.Items)
	}
	if result.Counts["missing"] == 0 {
		t.Fatalf("missing count not reported: %#v", result.Counts)
	}
}

func discoveryHas(items []OutputDiscoveryItem, objectType string, keyValue string, name string, status string) bool {
	_, ok := discoveryFind(items, objectType, keyValue, name, status)
	return ok
}

func discoveryFind(items []OutputDiscoveryItem, objectType string, keyValue string, name string, status string) (OutputDiscoveryItem, bool) {
	for _, item := range items {
		if item.ObjectType == objectType && item.KeyValue == keyValue && item.Name == name && item.Status == status {
			return item, true
		}
	}
	return OutputDiscoveryItem{}, false
}
