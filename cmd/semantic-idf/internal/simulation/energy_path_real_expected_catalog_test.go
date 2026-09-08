package simulation

import "testing"

// This offline integrity check does not claim a new engine acceptance run.
// Once explicitly approved, an expected artifact may not silently disappear
// back into the catalog's not-yet-approved placeholder state.
func TestEnergyPathRealApprovedExpectedCatalog(t *testing.T) {
	_, directory := epathRealDirectories(t)
	catalog := epathLoadRealCatalog(t, directory)
	approved := map[string]int{"large-office-25-1": 46224, "small-office-25-1": 17675, "ideal-loads-25-1": 17121, "ptac-25-1": 17021, "pthp-25-1": 17113, "fan-coil-25-1": 8966, "vrf-25-1": 16360, "radiant-25-1": 9615}
	for _, fixture := range catalog.Fixtures {
		count, required := approved[fixture.ID]
		if !required {
			continue
		}
		manifest := epathLoadRealExpectedManifest(t, epathRealCatalogPath(t, directory, fixture.ExpectedPath))
		if manifest.FixtureID != fixture.ID || manifest.Version != fixture.Version || manifest.ModelSHA256 != fixture.ModelSHA256 || manifest.WeatherSHA256 != fixture.Weather.SHA256 || len(manifest.Metrics) != count {
			t.Fatalf("approved artifact lost its exact fixture/provenance/metric set: %s", fixture.ID)
		}
		delete(approved, fixture.ID)
	}
	if len(approved) != 0 {
		t.Fatalf("approved fixtures disappeared from catalog: %v", approved)
	}
}
