package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"unicode"

	webassets "github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/frontend"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestManualKoreanMetricDescriptionsMatchRegistry(t *testing.T) {
	data, err := fs.ReadFile(webassets.Assets, "manual/metric-guides.ko.json")
	if err != nil {
		t.Fatal(err)
	}
	var overlay struct {
		Version int `json:"version"`
		Guides  map[string]struct {
			Original    map[string]string `json:"original"`
			Translation map[string]string `json:"translation"`
		} `json:"guides"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if !json.Valid(data) {
		t.Fatal("Korean metric overlay is not a single valid JSON document")
	}
	if err := decoder.Decode(&overlay); err != nil {
		t.Fatal(err)
	}
	registry := idf.MetricGuides()
	if overlay.Version != 1 || len(overlay.Guides) != len(registry) || len(registry) != 59 {
		t.Fatalf("expected version 1 and all 59 canonical metric IDs, got version %d and %d entries", overlay.Version, len(overlay.Guides))
	}
	for _, guide := range registry {
		entry, ok := overlay.Guides[guide.ID]
		if !ok {
			t.Fatalf("missing Korean metric description for %s", guide.ID)
		}
		original := map[string]string{
			"name": guide.Name, "category": guide.Category, "source": guide.Source,
			"method": guide.Method, "assumptions": guide.Assumptions, "missingData": guide.MissingData,
		}
		for _, fields := range []map[string]string{entry.Original, entry.Translation} {
			if len(fields) != len(original) {
				t.Fatalf("%s must contain exactly the six descriptive fields; IDs, units and values cannot be localized", guide.ID)
			}
			for field, value := range fields {
				if _, allowed := original[field]; !allowed || strings.TrimSpace(value) == "" {
					t.Fatalf("%s has an unknown or empty descriptive field %q", guide.ID, field)
				}
			}
		}
		for field, value := range original {
			if entry.Original[field] != value {
				t.Fatalf("%s.%s source changed; review and update the Korean translation with its original snapshot", guide.ID, field)
			}
		}
		for _, field := range []string{"source", "method", "assumptions", "missingData"} {
			if !strings.ContainsFunc(entry.Translation[field], func(r rune) bool { return unicode.Is(unicode.Hangul, r) }) {
				t.Fatalf("%s.%s lacks a Korean explanation", guide.ID, field)
			}
		}
	}
}
