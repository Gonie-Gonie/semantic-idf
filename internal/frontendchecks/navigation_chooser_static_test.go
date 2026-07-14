package frontendchecks

import (
	"regexp"
	"strings"
	"testing"
)

func TestNavigationChooserSupportsOccurrenceAndViewTargetAmbiguity(t *testing.T) {
	content := readTestFile(t, "frontend/src/js/navigation-chooser.js")
	for _, name := range []string{"chooseSemanticOccurrence", "chooseViewTarget", "closeNavigationChooser"} {
		assertJSExport(t, content, name)
	}
	for _, required := range []string{
		"payload.occurrences",
		"occurrence.occurrenceId",
		"occurrence.path",
		"payload.targets",
		"target.targetId",
		"target.targetKind",
		`setAttribute("role", "listbox")`,
		`setAttribute("role", "option")`,
		`addEventListener("cancel"`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("navigation chooser is missing %q", required)
		}
	}
	if regexp.MustCompile(`(?i)\b(?:analyze|analyzeinput|scheduleanalyze)\s*\(`).MatchString(content) {
		t.Fatal("navigation choice must not start analysis")
	}

	main := readTestFile(t, "frontend/src/js/main.js")
	for _, required := range []string{"chooseSemanticOccurrence,", "chooseViewTarget,"} {
		if !strings.Contains(main, required) {
			t.Fatalf("selection controller is not configured with %q", required)
		}
	}
	build := readTestFile(t, "scripts/frontend-build.ps1")
	if !strings.Contains(build, `"navigation-chooser.js"`) {
		t.Fatal("frontend readiness manifest is missing navigation-chooser.js")
	}
}
