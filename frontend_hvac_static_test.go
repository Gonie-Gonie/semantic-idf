package main

import (
	"os"
	"strings"
	"testing"
)

func TestFrontendHVACDefaultUICopyAvoidsDebugAndLegacyTerms(t *testing.T) {
	files := []string{
		"frontend/src/js/hvac-views.js",
		"frontend/src/js/i18n.js",
		"frontend/src/js/state.js",
	}
	forbidden := []string{
		"Rule edges",
		"Rule trace",
		"Rule path",
		"Terminal / Equipment",
		"Plant / Condenser",
		"terminal:direct",
		"Zone relations",
		"hvac.inferred",
		"Inferred",
		"Cross-loop",
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		for _, term := range forbidden {
			if strings.Contains(text, term) {
				t.Fatalf("%s contains forbidden HVAC default UI copy %q", file, term)
			}
		}
	}
}

func TestFrontendHVACStartsOnZoneServices(t *testing.T) {
	content, err := os.ReadFile("frontend/src/js/state.js")
	if err != nil {
		t.Fatalf("read state.js: %v", err)
	}
	if !strings.Contains(string(content), `activeHVACView: "services"`) {
		t.Fatalf("state.js should default HVAC to Zone Services view")
	}
}
