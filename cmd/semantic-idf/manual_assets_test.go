package main

import (
	"encoding/json"
	"io/fs"
	"path"
	"reflect"
	"regexp"
	"strings"
	"testing"

	webassets "github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/frontend"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestManualAssetsMatchCatalogAndSections(t *testing.T) {
	var manifest struct {
		Version         int    `json:"version"`
		DefaultLanguage string `json:"defaultLanguage"`
		Chapters        []struct {
			ID    string            `json:"id"`
			Title map[string]string `json:"title"`
			File  map[string]string `json:"file"`
		} `json:"chapters"`
	}
	read := func(name string) []byte {
		t.Helper()
		data, err := fs.ReadFile(webassets.Assets, "manual/"+name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if err := json.Unmarshal(read("manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || manifest.DefaultLanguage != "en" || len(manifest.Chapters) == 0 {
		t.Fatal("unsupported or empty manual manifest")
	}
	sectionPattern := regexp.MustCompile(`(?m)^#{1,4} .+ \{#([a-z][a-z0-9-]*)\}\s*$`)
	linkPattern := regexp.MustCompile(`\[[^\]\n]+\]\(([^)\s]+)\)`)
	sections := map[string]map[string]bool{}
	bodies := map[string]string{}
	ids := map[string]bool{}
	for _, chapter := range manifest.Chapters {
		if chapter.ID == "" || ids[chapter.ID] {
			t.Fatalf("empty/duplicate chapter ID %q", chapter.ID)
		}
		ids[chapter.ID] = true
		var english []string
		for _, language := range []string{"en", "ko"} {
			filename := chapter.File[language]
			if chapter.Title[language] == "" || !fs.ValidPath(filename) || path.Base(filename) != filename || !strings.HasSuffix(filename, "."+language+".md") {
				t.Fatalf("chapter %q lacks a local %s source/title", chapter.ID, language)
			}
			body := string(read(filename))
			if !strings.HasPrefix(body, "# ") {
				t.Fatalf("%s needs a chapter heading", filename)
			}
			// Example Markdown inside a code fence is content, not navigation.
			var plain []string
			fence := ""
			for _, line := range strings.Split(body, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
					marker := trimmed[:3]
					if fence == "" {
						fence = marker
					} else if fence == marker {
						fence = ""
					}
					continue
				}
				if fence == "" {
					plain = append(plain, line)
				}
			}
			if fence != "" {
				t.Fatalf("%s has an unclosed code fence", filename)
			}
			bodies[filename] = strings.Join(plain, "\n")
			found := []string{}
			sections[filename] = map[string]bool{}
			for _, match := range sectionPattern.FindAllStringSubmatch(bodies[filename], -1) {
				id := match[1]
				if sections[filename][id] {
					t.Fatalf("%s duplicates section %q", filename, id)
				}
				sections[filename][id] = true
				found = append(found, id)
			}
			if len(found) == 0 {
				t.Fatalf("%s has no stable section destinations", filename)
			}
			if language == "en" {
				english = found
			} else if !reflect.DeepEqual(english, found) {
				t.Fatalf("chapter %q translation section IDs/order differ\nEnglish: %v\nKorean: %v", chapter.ID, english, found)
			}
		}
	}
	for filename, body := range bodies {
		for _, match := range linkPattern.FindAllStringSubmatch(body, -1) {
			target := match[1]
			if strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "http://") {
				continue
			}
			destination, anchor, _ := strings.Cut(target, "#")
			if destination == "" {
				destination = filename
			} else {
				destination = strings.TrimPrefix(destination, "./")
			}
			if _, ok := sections[destination]; !ok {
				t.Fatalf("%s links outside the authored manual: %s", filename, target)
			}
			if anchor != "" && !sections[destination][anchor] {
				t.Fatalf("%s links to missing section: %s", filename, target)
			}
		}
	}
	for _, language := range []string{"en", "ko"} {
		if !sections["metrics."+language+".md"]["metric-catalog"] {
			t.Fatalf("%s metrics lacks the live/offline catalog destination", language)
		}
	}
	var catalog []idf.MetricGuide
	if err := json.Unmarshal(read("metric-guides.json"), &catalog); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(catalog, idf.MetricGuides()) {
		t.Fatal("bundled metric definitions differ from the Go registry; run go run ./cmd/semantic-idf/internal/manualcatalog from the repo root")
	}
}
