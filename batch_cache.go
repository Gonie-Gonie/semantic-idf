package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

const maxBatchInputCacheEntries = 64

type batchInputCacheKey struct {
	Path string
	Hash string
}

type batchInputCacheEntry struct {
	key       batchInputCacheKey
	model     *epinput.Model
	doc       idf.Document
	touchedAt time.Time
}

var globalBatchInputCache = &batchInputCache{
	entries: map[batchInputCacheKey]*batchInputCacheEntry{},
}

type batchInputCache struct {
	mu      sync.Mutex
	entries map[batchInputCacheKey]*batchInputCacheEntry
	order   []batchInputCacheKey
}

func parseCachedBatchInput(path string) (*epinput.Model, idf.Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, idf.Document{}, err
	}
	key := batchInputCacheKey{Path: path, Hash: batchContentHash(content)}
	if model, doc, ok := globalBatchInputCache.lookup(key); ok {
		return model, doc, nil
	}
	model, err := epinput.Parse(path, content)
	if err != nil {
		return nil, idf.Document{}, err
	}
	doc := epinput.ToIDFDocument(model)
	globalBatchInputCache.store(key, model, doc)
	return model, doc, nil
}

func (cache *batchInputCache) lookup(key batchInputCacheKey) (*epinput.Model, idf.Document, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.entries[key]
	if !ok {
		return nil, idf.Document{}, false
	}
	entry.touchedAt = time.Now()
	cache.rememberLocked(key)
	return entry.model, entry.doc, true
}

func (cache *batchInputCache) store(key batchInputCacheKey, model *epinput.Model, doc idf.Document) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.entries[key] = &batchInputCacheEntry{
		key:       key,
		model:     model,
		doc:       doc,
		touchedAt: time.Now(),
	}
	cache.rememberLocked(key)
	for len(cache.order) > maxBatchInputCacheEntries {
		oldest := cache.order[0]
		cache.order = cache.order[1:]
		delete(cache.entries, oldest)
	}
}

func (cache *batchInputCache) rememberLocked(key batchInputCacheKey) {
	next := cache.order[:0]
	for _, existing := range cache.order {
		if existing != key {
			next = append(next, existing)
		}
	}
	cache.order = append(next, key)
}

func batchContentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
