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

func discoveryHas(items []OutputDiscoveryItem, objectType string, keyValue string, name string, status string) bool {
	for _, item := range items {
		if item.ObjectType == objectType && item.KeyValue == keyValue && item.Name == name && item.Status == status {
			return true
		}
	}
	return false
}
