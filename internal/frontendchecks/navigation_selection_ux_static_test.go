package frontendchecks

import (
	"strings"
	"testing"
)

func TestResultPanelPrimarySelectionUsesExactOccurrenceOrTarget(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/panel-navigation-adapters.js")
	refresh := sliceBetween(
		content,
		"export function refreshResultPanelSelectionStyles",
		"function createResultPanelNavigationAdapter",
	)
	for _, required := range []string{
		"selection?.occurrenceId",
		"item.dataset.occurrenceId",
		`item.classList.toggle("semantic-selected"`,
		`item.classList.toggle("semantic-related"`,
		`item.setAttribute("aria-current", "location")`,
		`item.removeAttribute("aria-current")`,
	} {
		if !strings.Contains(refresh, required) {
			t.Fatalf("result-panel exact-primary contract is missing %q", required)
		}
	}

	// Entity equality alone denotes semantic context, not an exact visual
	// primary. This was the branch that outlined a Summary category and every
	// one of its rows at the same time.
	if strings.Contains(refresh, "(selectedEntityId && itemEntityId === selectedEntityId) ||") {
		t.Fatal("same-entity result items must not all become primary selections")
	}
}
