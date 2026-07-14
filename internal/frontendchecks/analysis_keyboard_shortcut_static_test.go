package frontendchecks

import (
	"strings"
	"testing"
)

func TestAnalysisPanelLocalActivationLeavesModifiedEnterForGlobalShortcut(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/main.js")
	handler := sliceBetween(
		content,
		`elements.analysisPanel.addEventListener("keydown"`,
		`window.addEventListener("idfAnalyzer:settingsChanged"`,
	)
	for _, guard := range []string{"event.altKey", "event.ctrlKey", "event.metaKey", "event.shiftKey"} {
		if !strings.Contains(handler, guard) {
			t.Fatalf("analysis-panel local activation must ignore modified Enter/Space: missing %q", guard)
		}
	}
	guardIndex := strings.Index(handler, "event.altKey")
	preventDefaultIndex := strings.Index(handler, "event.preventDefault()")
	if guardIndex < 0 || preventDefaultIndex < 0 || guardIndex > preventDefaultIndex {
		t.Fatal("modified activation must return before the analysis panel consumes the key event")
	}
}
