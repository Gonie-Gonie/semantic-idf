package frontendchecks

import (
	"strings"
	"testing"
)

func TestFrontendEnergyPathDriverTaxonomyOrderAndZeroGuard(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	for _, required := range []string{
		`const ENERGY_PATH_DRIVER_ORDER`,
		`"surface.exterior_walls": 0`,
		`"surface.roofs": 1`,
		`"surface.ground_floors": 2`,
		`"surface.windows_doors": 3`,
		`"air.infiltration": 5`,
		`"air.mechanical_ventilation": 6`,
		`"internal.people": 8`,
		`"internal.lighting": 9`,
		`"internal.equipment": 10`,
		`"balance.storage_other": 12`,
		`compareEnergyPathStageNodes(stage, left, right)`,
		`stage.level !== "driver" || Math.abs(Number(node.value) || 0) > 0`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("Energy Path driver display contract missing %q", required)
		}
	}
	if strings.Contains(view, `data-simulation-energy-node-limit`) {
		t.Fatal("Energy Path must not expose a user-adjustable node limit")
	}
}
