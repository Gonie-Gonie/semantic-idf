package frontendchecks

import (
	"strings"
	"testing"
)

func TestDynamicInputViewsUseFixedDelegatedEventBindings(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	for _, stateFlag := range []string{
		"semanticControlsBound",
		"jsonEditorControlsBound",
		"formattedTextControlsBound",
		"fieldTableControlsBound",
	} {
		if !strings.Contains(views, stateFlag) {
			t.Fatalf("dynamic input views need a one-time binding guard %q", stateFlag)
		}
	}

	jsonBinding := sliceBetween(views, "function bindJSONEditorControls", "function focusSelectedJSONObject")
	for _, required := range []string{
		`elements.jsonStructuredView.addEventListener("change"`,
		`elements.jsonStructuredView.addEventListener("click"`,
		`target.closest(".json-value-token")`,
		`target.closest(".json-object-summary")`,
	} {
		if !strings.Contains(jsonBinding, required) {
			t.Fatalf("JSON view delegated controls are missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`querySelectorAll(".json-object-summary")`,
		`querySelectorAll(".json-value-token")`,
	} {
		if strings.Contains(jsonBinding, forbidden) {
			t.Fatalf("JSON view must not allocate a listener per rendered object/value: %q", forbidden)
		}
	}

	textBinding := sliceBetween(views, "function bindFormattedTextControls", "function fieldSuggestionListID")
	for _, required := range []string{
		`elements.textObjectView.addEventListener("click"`,
		`bindDelegatedFieldEditor(elements.textObjectView, ".text-field-input", applyTextValue)`,
		`host.addEventListener("focusin"`,
		`host.addEventListener("focusout"`,
		`host.addEventListener("keydown"`,
	} {
		if !strings.Contains(textBinding, required) {
			t.Fatalf("formatted text delegated controls are missing %q", required)
		}
	}
	if strings.Contains(textBinding, `querySelectorAll(".text-field-input")`) {
		t.Fatal("formatted text must not allocate a listener per rendered field")
	}

	tableRender := sliceBetween(views, "export function renderFieldTable", "function limitObjectGroups")
	for _, required := range []string{
		"bindFieldTableControls();",
		`elements.fieldTable.addEventListener("click"`,
		`bindDelegatedFieldEditor(elements.fieldTable, ".field-value-input", applyTableValue)`,
		`target.closest(".object-orientation-button")`,
		`target.closest("[data-table-object-index]")`,
	} {
		if !strings.Contains(tableRender, required) {
			t.Fatalf("field table delegated controls are missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`querySelectorAll(".field-value-input")`,
		`querySelectorAll("[data-table-object-index]")`,
		`querySelectorAll(".object-orientation-button")`,
	} {
		if strings.Contains(tableRender, forbidden) {
			t.Fatalf("field table must not allocate a listener per rendered row/cell: %q", forbidden)
		}
	}
}
