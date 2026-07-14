package frontendchecks

import (
	"strings"
	"testing"
)

func TestSemanticBasicViewUsesLazySectionsWithoutHardLineBudget(t *testing.T) {
	views := readTestFile(t, "frontend/src/js/views/input-views.js")
	if strings.Contains(views, "SEMANTIC_BASIC_LINE_BUDGET") || strings.Contains(views, "slice(0, 250)") {
		t.Fatal("Basic Semantic Text must not discard navigation targets at a hard line budget")
	}
	for _, required := range []string{
		"function materializedBasicSemanticLines",
		"semanticExpandedSectionIds",
		"data-semantic-section-id",
		"semanticSectionEntityCount",
		"aria-expanded",
	} {
		if !strings.Contains(views, required) {
			t.Fatalf("lazy semantic section contract missing %q", required)
		}
	}

	state := readTestFile(t, "frontend/src/js/state.js")
	if !strings.Contains(state, `semanticExpandedSectionIds: new Set(["project"])`) {
		t.Fatal("Semantic Text should initially materialize only the compact project summary")
	}
}
