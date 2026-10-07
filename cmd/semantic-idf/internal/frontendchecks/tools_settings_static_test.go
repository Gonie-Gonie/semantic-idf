package frontendchecks

import (
	"strings"
	"testing"
)

func readSettingsSource(t *testing.T) string {
	t.Helper()
	markup := readTestFile(t, "frontend/src/settings.html")
	if !strings.Contains(markup, `<script type="module" src="./js/settings.js"></script>`) {
		t.Fatal("Settings must load its feature module")
	}
	return markup + "\n" + readTestFile(t, "frontend/src/js/settings.js")
}
