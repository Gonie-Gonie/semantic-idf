package frontendchecks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Translation contracts follow the catalogs, independently of their engine.
// Read them here as well as in browser checks so Go records their dependencies.
func readTranslationSource(t *testing.T) string {
	t.Helper()
	parts := []string{readTestFile(t, "frontend/src/js/i18n.js")}
	paths, err := filepath.Glob(repoPath("frontend/src/js/locales/*.js"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("discover translation sources: %v", err)
	}
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read translation source: %v", err)
		}
		parts = append(parts, string(contents))
	}
	return strings.Join(parts, "\n")
}

func TestLocalizationCatalogCoverageAndPlaceholders(t *testing.T) {
	catalogs := make(map[string]map[string]string)
	for _, language := range []string{"en", "ko", "ja", "hi", "es", "fr"} {
		text := strings.TrimSpace(readTestFile(t, "frontend/src/js/locales/"+language+".js"))
		if !strings.HasPrefix(text, "export default Object.freeze(") || !strings.HasSuffix(text, ");") {
			t.Fatalf("%s catalog must export a frozen string map", language)
		}
		text = strings.TrimSuffix(strings.TrimPrefix(text, "export default Object.freeze("), ");")
		var messages map[string]string
		if err := json.Unmarshal([]byte(text), &messages); err != nil {
			t.Fatalf("parse %s catalog: %v", language, err)
		}
		catalogs[language] = messages
	}
	placeholders := regexp.MustCompile(`\{[a-zA-Z0-9_]+\}`)
	tokens := func(value string) string {
		values := placeholders.FindAllString(value, -1)
		sort.Strings(values)
		return strings.Join(values, ",")
	}
	for language, messages := range catalogs {
		for key, value := range messages {
			original, ok := catalogs["en"][key]
			if !ok {
				t.Errorf("%s has an unknown key %s", language, key)
			}
			if strings.TrimSpace(value) == "" || strings.Contains(value, "\uFFFD") || strings.Contains(value, "??") {
				t.Errorf("%s has an empty or corrupted translation for %s: %q", language, key, value)
			}
			if ok && tokens(original) != tokens(value) {
				t.Errorf("%s key %s changes placeholders: %q -> %q", language, key, original, value)
			}
		}
	}
	for key := range catalogs["en"] {
		if _, ok := catalogs["ko"][key]; !ok {
			t.Errorf("Korean catalog is missing %s", key)
		}
	}
	// Technical names and data-dependent keys have explicit English fallbacks.
	// Literal interface keys must always have an authoritative English entry.
	keyPattern := regexp.MustCompile(`\b(?:t|localizedMessage)\(\s*["']([^"']+)["']`)
	attributePattern := regexp.MustCompile(`data-i18n(?:-title|-placeholder|-aria-label)?=["']([^"']+)["']`)
	err := filepath.WalkDir(repoPath("frontend/src"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if strings.Contains(filepath.ToSlash(path), "/vendor/") || (filepath.Ext(path) != ".js" && filepath.Ext(path) != ".html") {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, pattern := range []*regexp.Regexp{keyPattern, attributePattern} {
			for _, match := range pattern.FindAllStringSubmatch(string(contents), -1) {
				if _, ok := catalogs["en"][match[1]]; !ok {
					t.Errorf("%s uses unregistered interface key %s", filepath.Base(path), match[1])
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
