package main

import (
	"sync"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// The twelve result slots normally hold four stages for three documents. Keep
// at most three parsed snapshots as well, rather than retaining every edit.
const maxAnalysisInputCacheEntries = 3

// Analysis reads these snapshots; editing APIs continue to parse their own
// documents. The index is built on demand and shared by concurrent stages.
type analysisInput struct {
	model     *epinput.Model
	doc       idf.Document
	indexOnce sync.Once
	index     *idf.DocumentIndex
}

func (input *analysisInput) documentIndex() *idf.DocumentIndex {
	input.indexOnce.Do(func() { input.index = idf.NewDocumentIndex(input.doc) })
	return input.index
}

type analysisInputFlight struct {
	done  chan struct{}
	input *analysisInput
	err   error
}

type analysisInputCache struct {
	mu       sync.Mutex
	entries  map[string]*analysisInput
	inflight map[string]*analysisInputFlight
	order    []string
}

func (cache *analysisInputCache) load(textHash string, parse func() (*epinput.Model, idf.Document, error)) (*analysisInput, error) {
	cache.mu.Lock()
	if input := cache.entries[textHash]; input != nil {
		cache.rememberLocked(textHash)
		cache.mu.Unlock()
		return input, nil
	}
	if pending := cache.inflight[textHash]; pending != nil {
		cache.mu.Unlock()
		<-pending.done
		return pending.input, pending.err
	}
	if cache.inflight == nil {
		cache.inflight = make(map[string]*analysisInputFlight)
	}
	pending := &analysisInputFlight{done: make(chan struct{})}
	cache.inflight[textHash] = pending
	cache.mu.Unlock()

	model, doc, err := parse()
	if err == nil {
		pending.input = &analysisInput{model: model, doc: doc}
	}
	pending.err = err

	cache.mu.Lock()
	if err == nil {
		if cache.entries == nil {
			cache.entries = make(map[string]*analysisInput)
		}
		cache.entries[textHash] = pending.input
		cache.rememberLocked(textHash)
	}
	delete(cache.inflight, textHash)
	close(pending.done)
	cache.mu.Unlock()
	return pending.input, pending.err
}

func (cache *analysisInputCache) rememberLocked(textHash string) {
	order := cache.order[:0]
	for _, key := range cache.order {
		if key != textHash {
			order = append(order, key)
		}
	}
	cache.order = append(order, textHash)
	for len(cache.order) > maxAnalysisInputCacheEntries {
		delete(cache.entries, cache.order[0])
		cache.order = cache.order[1:]
	}
}

func (a *App) initializedAnalysisCache() *AnalysisCache {
	a.analysisCacheOnce.Do(func() {
		if a.analysisCache == nil {
			a.analysisCache = NewAnalysisCache(defaultAnalysisCacheEntries)
		}
	})
	return a.analysisCache
}

func (a *App) analysisInputForText(textHash, text string) (*analysisInput, error) {
	return a.initializedAnalysisCache().inputs.load(textHash, func() (*epinput.Model, idf.Document, error) {
		return parseInputDocument(text)
	})
}
