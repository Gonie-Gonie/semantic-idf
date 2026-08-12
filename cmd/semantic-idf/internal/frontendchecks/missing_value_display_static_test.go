package frontendchecks

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeMissingValuesUseEmDash(t *testing.T) {
	root := repoPath(".")
	allowedCompatibilityParsers := map[string][]string{
		"app.go": {`strings.EqualFold(buildingName, "N/A")`},
		"internal/simulation/purpose.go": {`"n/a"`},
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" || entry.Name() == "vendor" || entry.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), "_test.go") || strings.HasSuffix(entry.Name(), ".rej") {
			return nil
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".go", ".js", ".html", ".css":
		default:
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(content)
		relativePath, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		relativePath = filepath.ToSlash(relativePath)
		for _, parser := range allowedCompatibilityParsers[relativePath] {
			text = strings.Replace(text, parser, "", 1)
		}
		if strings.Contains(strings.ToUpper(text), "N/A") || strings.Contains(text, "해당 없음") {
			t.Errorf("%s contains a user-visible N/A sentinel; render an em dash and keep the reason in status/help text", relativePath)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan runtime sources: %v", err)
	}

	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	if count := strings.Count(i18n, `"common.notAvailable": "—"`); count != 2 {
		t.Fatalf("common.notAvailable must be an em dash in the English and Korean dictionaries, got %d entries", count)
	}
}
