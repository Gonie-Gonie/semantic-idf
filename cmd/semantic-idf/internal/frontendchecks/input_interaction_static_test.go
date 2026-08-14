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

	jsonBinding := sliceBetween(views, "function bindJSONEditorControls", "function editJSONValueToken")
	for _, required := range []string{
		`elements.jsonStructuredView.addEventListener("click"`,
		`target.closest(".json-value-token")`,
		`target.closest(".json-object-summary")`,
	} {
		if !strings.Contains(jsonBinding, required) {
			t.Fatalf("JSON view delegated controls are missing %q", required)
		}
	}
	for _, removed := range []string{"jsonCollapseDepth", "jsonFocusObjectButton", "json-editor-tools"} {
		if strings.Contains(views, removed) {
			t.Fatalf("removed JSON display option remains: %q", removed)
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
		`bindDelegatedFieldEditor(elements.textObjectView, ".text-field-input", applyTextValue)`,
		`host.addEventListener("focusin"`,
		`host.addEventListener("focusout"`,
		`host.addEventListener("keydown"`,
	} {
		if !strings.Contains(textBinding, required) {
			t.Fatalf("formatted text delegated controls are missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`querySelectorAll(".text-field-input")`,
		`elements.textObjectView.addEventListener("click"`,
	} {
		if strings.Contains(textBinding, forbidden) {
			t.Fatalf("formatted text retains obsolete or per-field listener %q", forbidden)
		}
	}

	tableRender := sliceBetween(views, "export function renderFieldTable", "function limitObjectGroups")
	for _, required := range []string{
		"bindFieldTableControls();",
		`elements.fieldTable.addEventListener("click"`,
		`bindDelegatedFieldEditor(elements.fieldTable, ".field-value-input", applyTableValue)`,
		`target.closest(".object-orientation-button")`,
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

func TestSemanticViewUsesFixedDetailedAllPresentation(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	for _, required := range []string{"function semanticProjectionMode() {\n  return \"detailed\";", "function semanticProjectionFacet() {\n  return \"all\";"} {
		if !strings.Contains(views, required) {
			t.Fatalf("fixed Semantic presentation is missing %q", required)
		}
	}
	for _, removed := range []string{"semantic-mode-tabs", "semantic-filter-tabs", "semanticFocusObjectButton", "semanticFixDuplicatesButton"} {
		if strings.Contains(views, removed) {
			t.Fatalf("removed Semantic control remains: %q", removed)
		}
	}
}
