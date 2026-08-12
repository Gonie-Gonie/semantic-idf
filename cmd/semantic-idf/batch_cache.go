package main

import (
	"container/list"
	"crypto/sha256"
	"os"
	"sync"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const maxBatchInputCacheEntries = 64

type batchInputCacheKey struct {
	Path string
	Hash [sha256.Size]byte
}

type batchInputCacheEntry struct {
	key   batchInputCacheKey
	model *epinput.Model
	doc   idf.Document
}

type batchInputCacheLoad struct {
	done  chan struct{}
	model *epinput.Model
	doc   idf.Document
	err   error
}

var globalBatchInputCache = newBatchInputCache()

type batchInputCache struct {
	mu       sync.Mutex
	entries  map[batchInputCacheKey]*list.Element
	order    *list.List
	inflight map[batchInputCacheKey]*batchInputCacheLoad
}

func newBatchInputCache() *batchInputCache {
	return &batchInputCache{
		entries:  make(map[batchInputCacheKey]*list.Element, maxBatchInputCacheEntries),
		order:    list.New(),
		inflight: make(map[batchInputCacheKey]*batchInputCacheLoad),
	}
}

func parseCachedBatchInput(path string) (*epinput.Model, idf.Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, idf.Document{}, err
	}
	key := batchInputCacheKey{Path: path, Hash: batchContentHash(content)}
	return globalBatchInputCache.load(key, func() (*epinput.Model, idf.Document, error) {
		model, err := epinput.Parse(path, content)
		if err != nil {
			return nil, idf.Document{}, err
		}
		return model, epinput.ToIDFDocument(model), nil
	})
}

func (cache *batchInputCache) load(key batchInputCacheKey, loader func() (*epinput.Model, idf.Document, error)) (*epinput.Model, idf.Document, error) {
	cache.mu.Lock()
	cache.ensureInitializedLocked()
	if element, ok := cache.entries[key]; ok {
		cache.order.MoveToBack(element)
		entry := element.Value.(*batchInputCacheEntry)
		cache.mu.Unlock()
		return entry.model, entry.doc, nil
	}
	if pending := cache.inflight[key]; pending != nil {
		cache.mu.Unlock()
		<-pending.done
		return pending.model, pending.doc, pending.err
	}
	pending := &batchInputCacheLoad{done: make(chan struct{})}
	cache.inflight[key] = pending
	cache.mu.Unlock()

	pending.model, pending.doc, pending.err = loader()

	cache.mu.Lock()
	if pending.err == nil {
		cache.storeLocked(key, pending.model, pending.doc)
	}
	delete(cache.inflight, key)
	close(pending.done)
	cache.mu.Unlock()
	return pending.model, pending.doc, pending.err
}

func (cache *batchInputCache) ensureInitializedLocked() {
	if cache.entries == nil {
		cache.entries = make(map[batchInputCacheKey]*list.Element, maxBatchInputCacheEntries)
	}
	if cache.order == nil {
		cache.order = list.New()
	}
	if cache.inflight == nil {
		cache.inflight = make(map[batchInputCacheKey]*batchInputCacheLoad)
	}
}

func (cache *batchInputCache) storeLocked(key batchInputCacheKey, model *epinput.Model, doc idf.Document) {
	if element, ok := cache.entries[key]; ok {
		entry := element.Value.(*batchInputCacheEntry)
		entry.model = model
		entry.doc = doc
		cache.order.MoveToBack(element)
		return
	}
	entry := &batchInputCacheEntry{key: key, model: model, doc: doc}
	cache.entries[key] = cache.order.PushBack(entry)
	for cache.order.Len() > maxBatchInputCacheEntries {
		oldest := cache.order.Front()
		delete(cache.entries, oldest.Value.(*batchInputCacheEntry).key)
		cache.order.Remove(oldest)
	}
}

func batchContentHash(content []byte) [sha256.Size]byte {
	return sha256.Sum256(content)
}
