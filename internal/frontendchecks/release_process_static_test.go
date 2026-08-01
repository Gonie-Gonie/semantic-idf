package frontendchecks

import (
	"strings"
	"testing"
)

func TestReleaseScriptReadsTrackedTextAsUTF8(t *testing.T) {
	script := readTestFile(t, "scripts/release.ps1")
	for lineNumber, line := range strings.Split(script, "\n") {
		if !strings.Contains(line, "Get-Content") || !strings.Contains(line, "-Raw") {
			continue
		}
		if !strings.Contains(strings.ToLower(line), "-encoding utf8") {
			t.Fatalf("release script raw text read on line %d must specify UTF-8: %s", lineNumber+1, line)
		}
	}
}
