package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestBatchInputCacheCoalescesConcurrentLoads(t *testing.T) {
	cache := newBatchInputCache()
	key := batchInputCacheKey{Path: "same.idf", Hash: batchContentHash([]byte("same content"))}
	model := &epinput.Model{}
	doc := idf.Document{Objects: []idf.Object{{Type: "Version"}}}
	var loaderCalls atomic.Int32
	loaderEntered := make(chan struct{}, 1)
	releaseLoader := make(chan struct{})
	loader := func() (*epinput.Model, idf.Document, error) {
		loaderCalls.Add(1)
		loaderEntered <- struct{}{}
		<-releaseLoader
		return model, doc, nil
	}

	const callers = 12
	models := make([]*epinput.Model, callers)
	documents := make([]idf.Document, callers)
	errors := make([]error, callers)
	start := make(chan struct{})
	var callersDone sync.WaitGroup
	callersDone.Add(callers)
	for index := range callers {
		go func() {
			defer callersDone.Done()
			<-start
			models[index], documents[index], errors[index] = cache.load(key, loader)
		}()
	}
	close(start)
	<-loaderEntered
	close(releaseLoader)
	callersDone.Wait()

	if loaderCalls.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loaderCalls.Load())
	}
	for index := range callers {
		if errors[index] != nil {
			t.Fatalf("caller %d error = %v", index, errors[index])
		}
		if models[index] != model || len(documents[index].Objects) != 1 {
			t.Fatalf("caller %d cache result was not shared", index)
		}
	}
}

func TestBatchInputCacheEvictsLeastRecentlyUsedEntry(t *testing.T) {
	cache := newBatchInputCache()
	keys := make([]batchInputCacheKey, maxBatchInputCacheEntries+1)
	for index := range maxBatchInputCacheEntries {
		keys[index] = batchInputCacheKey{
			Path: fmt.Sprintf("model-%d.idf", index),
			Hash: batchContentHash([]byte(fmt.Sprintf("content-%d", index))),
		}
		if _, _, err := cache.load(keys[index], emptyBatchCacheLoader); err != nil {
			t.Fatalf("seed cache entry %d: %v", index, err)
		}
	}

	var touchedLoaderCalls atomic.Int32
	if _, _, err := cache.load(keys[0], func() (*epinput.Model, idf.Document, error) {
		touchedLoaderCalls.Add(1)
		return nil, idf.Document{}, fmt.Errorf("unexpected reload")
	}); err != nil {
		t.Fatalf("touch most-recent entry: %v", err)
	}
	if touchedLoaderCalls.Load() != 0 {
		t.Fatal("touching a cached entry invoked its loader")
	}

	keys[maxBatchInputCacheEntries] = batchInputCacheKey{Path: "new.idf", Hash: batchContentHash([]byte("new content"))}
	if _, _, err := cache.load(keys[maxBatchInputCacheEntries], emptyBatchCacheLoader); err != nil {
		t.Fatalf("add eviction entry: %v", err)
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.order.Len() != maxBatchInputCacheEntries || len(cache.entries) != maxBatchInputCacheEntries {
		t.Fatalf("cache size = %d/%d, want %d", cache.order.Len(), len(cache.entries), maxBatchInputCacheEntries)
	}
	if _, ok := cache.entries[keys[0]]; !ok {
		t.Fatal("recently used entry was evicted")
	}
	if _, ok := cache.entries[keys[1]]; ok {
		t.Fatal("least recently used entry was retained")
	}
}

func emptyBatchCacheLoader() (*epinput.Model, idf.Document, error) {
	return &epinput.Model{}, idf.Document{}, nil
}
