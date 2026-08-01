package frontendchecks

import (
	"strings"
	"testing"
)

func TestTopologyViewSwitchSupportsThermalAndNormalizesInvalidModes(t *testing.T) {
	index := readTestFile(t, "frontend/src/index.html")
	for _, required := range []string{
		`data-geometry-mode="3d"`,
		`data-geometry-mode="plan"`,
		`data-geometry-mode="thermal"`,
		`data-i18n-title="topology.thermalTooltip"`,
		`id="thermalTopologyGraph"`,
		`id="thermalTopologyAreaBasis"`,
		`value="effective"`,
		`value="physical"`,
	} {
		if !strings.Contains(index, required) {
			t.Fatalf("topology view switch is missing %q", required)
		}
	}

	state := readTestFile(t, "frontend/src/js/state.js")
	for _, required := range []string{
		`Object.freeze(["3d", "plan", "thermal"])`,
		`return geometryModes.includes(mode) ? mode : "3d"`,
	} {
		if !strings.Contains(state, required) {
			t.Fatalf("geometry mode normalization is missing %q", required)
		}
	}
}

func TestTopologyAreaBasisDefaultsToEffectiveAndGeometryLabelsUsePhysicalArea(t *testing.T) {
	state := readTestFile(t, "frontend/src/js/state.js")
	if !strings.Contains(state, `thermalTopologyAreaBasis: "effective"`) {
		t.Fatal("thermal topology area basis must default to effective model-total area")
	}

	view := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	for _, required := range []string{
		`surface.physicalArea ?? surface.area`,
		`windowItem.physicalArea ?? windowItem.area`,
		`"physicalGrossArea" : "effectiveGrossArea"`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("area basis rendering contract is missing %q", required)
		}
	}
}

func TestTopologyModeHistoryPreservesSharedSelection(t *testing.T) {
	loader := readTestFile(t, "frontend/src/js/geometry-loader.js")
	setter := sliceBetween(loader, "export function setGeometryMode", "export function setGeometryStory")
	for _, required := range []string{"normalizeGeometryMode(mode)", "recordViewHistory()", "state.geometryMode = nextMode"} {
		if !strings.Contains(setter, required) {
			t.Fatalf("geometry mode setter is missing %q", required)
		}
	}
	for _, forbidden := range []string{"selectedGeometryId =", "selectedGeometryKind =", "globalSelection =", "analyze(", "backend."} {
		if strings.Contains(setter, forbidden) {
			t.Fatalf("mode switching must preserve the shared selection; found %q", forbidden)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/geometry-view.js")
	restore := sliceBetween(view, "export async function restoreGeometryNavigationContext", "export function preferredGeometrySemanticOccurrence")
	for _, required := range []string{
		`state.geometryMode = normalizeGeometryMode(snapshot.mode)`,
		`state.selectedGeometryKind = normalizeGeometryKind(snapshot.selectedKind)`,
		`state.selectedGeometryId = String(snapshot.selectedId || "")`,
	} {
		if !strings.Contains(restore, required) {
			t.Fatalf("topology history restore is missing %q", required)
		}
	}
}
