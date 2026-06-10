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
