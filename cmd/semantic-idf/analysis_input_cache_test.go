package main

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestAnalysisInputCacheSharesConcurrentParsingAndIndex(t *testing.T) {
	var cache analysisInputCache
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	parse := func() (*epinput.Model, idf.Document, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return parseInputDocument(appMetricsIDF)
	}
	var wg sync.WaitGroup
	inputs := make([]*analysisInput, 8)
	indexes := make([]*idf.DocumentIndex, len(inputs))
	for i := range inputs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input, err := cache.load("same", parse)
			if err != nil {
				t.Errorf("load: %v", err)
				return
			}
			inputs[i], indexes[i] = input, input.documentIndex()
		}(i)
	}
	<-started
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("parse calls = %d, want 1", calls.Load())
	}
	for i := range inputs {
		if inputs[i] != inputs[0] || indexes[i] != indexes[0] || indexes[i] == nil {
			t.Fatal("concurrent stages did not share the parsed snapshot and index")
		}
	}
}

func TestAnalysisInputCacheEvictsLeastRecentlyUsedAndRetriesErrors(t *testing.T) {
	var cache analysisInputCache
	load := func(key string) *analysisInput {
		t.Helper()
		input, err := cache.load(key, func() (*epinput.Model, idf.Document, error) {
			return parseInputDocument(appMetricsIDF)
		})
		if err != nil {
			t.Fatal(err)
		}
		return input
	}
	first := load("first")
	oldest := load("second")
	load("third")
	if load("first") != first {
		t.Fatal("cache hit replaced the snapshot")
	}
	load("fourth")
	if cache.entries["second"] != nil || len(cache.entries) != maxAnalysisInputCacheEntries {
		t.Fatalf("unexpected eviction/size: %v", cache.order)
	}
	if load("second") == oldest {
		t.Fatal("evicted input was retained")
	}
	wantErr := errors.New("parse failed")
	if _, err := cache.load("error", func() (*epinput.Model, idf.Document, error) {
		return nil, idf.Document{}, wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("load error = %v", err)
	}
	if cache.entries["error"] != nil || len(cache.inflight) != 0 {
		t.Fatal("failed parse was retained")
	}
	load("error")
}

func TestAnalysisStagesReuseInputAndInvalidateAfterEdit(t *testing.T) {
	var app App // Concurrent initialization must also work for a zero-value App.
	var wg sync.WaitGroup
	for _, stage := range []string{"profile", "hvac", "output", "geometry"} {
		wg.Add(1)
		go func(stage string) {
			defer wg.Done()
			if _, err := app.AnalyzeInputStageText(appMetricsIDF, stage); err != nil {
				t.Errorf("%s: %v", stage, err)
			}
		}(stage)
	}
	wg.Wait()
	cache := &app.initializedAnalysisCache().inputs
	if len(cache.entries) != 1 {
		t.Fatalf("parsed documents = %d, want 1", len(cache.entries))
	}
	first := cache.entries[analysisTextHash(appMetricsIDF)]
	quick, err := app.AnalyzeInputQuickText(appMetricsIDF)
	if err != nil {
		t.Fatal(err)
	}
	if quick.Model != first.model {
		t.Fatal("quick result did not reuse the stages' input")
	}
	edited := appMetricsIDF + "\nZone, Another;\n"
	result, err := app.AnalyzeInputQuickText(edited)
	if err != nil {
		t.Fatal(err)
	}
	if result.Model == first.model || result.Report.ObjectCount != quick.Report.ObjectCount+1 {
		t.Fatal("edited input reused a stale parsed snapshot")
	}
}

// Isolate the preparation shared by Quick/Profile/HVAC/Geometry. Each fresh
// path reproduces the previous four parses and four index constructions.
func BenchmarkAnalysisInputPreparation(b *testing.B) {
	content, err := os.ReadFile("frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	if err != nil {
		b.Fatal(err)
	}
	text := string(content)
	key := analysisTextHash(text)
	for _, shared := range []bool{false, true} {
		b.Run(fmt.Sprintf("shared=%t", shared), func(b *testing.B) {
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				var cache analysisInputCache
				for stage := 0; stage < 4; stage++ {
					var input *analysisInput
					if shared {
						input, err = cache.load(key, func() (*epinput.Model, idf.Document, error) {
							return parseInputDocument(text)
						})
					} else {
						model, doc, parseErr := parseInputDocument(text)
						input, err = &analysisInput{model: model, doc: doc}, parseErr
					}
					if err != nil {
						b.Fatal(err)
					}
					input.documentIndex()
				}
			}
		})
	}
}
