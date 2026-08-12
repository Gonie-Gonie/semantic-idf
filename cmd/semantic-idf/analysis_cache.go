package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const (
	defaultAnalysisCacheEntries = 12
	defaultAnalysisSettingsHash = "default"
)

type AnalysisTiming struct {
	Mode        string           `json:"mode,omitempty"`
	CacheHit    bool             `json:"cacheHit"`
	QueueWaitMS int64            `json:"queueWaitMs,omitempty"`
	TotalMS     int64            `json:"totalMs"`
	ParseMS     int64            `json:"parseMs,omitempty"`
	AnalyzeMS   int64            `json:"analyzeMs,omitempty"`
	SemanticMS  int64            `json:"semanticMs,omitempty"`
	EPJSONMS    int64            `json:"epjsonMs,omitempty"`
	Stages      map[string]int64 `json:"stages,omitempty"`
}

type analysisCacheKey struct {
	TextHash          string
	Format            string
	EnergyPlusVersion string
	AnalyzerVersion   string
	Mode              string
	SettingsHash      string
}

type analysisTextModeKey struct {
	TextHash string
	Mode     string
}

type analysisCacheEntry struct {
	key       analysisCacheKey
	result    *InputAnalysisResult
	touchedAt time.Time
}

type analysisFlight struct {
	done   chan struct{}
	result *InputAnalysisResult
	err    error
}

type AnalysisCache struct {
	mu            sync.Mutex
	maxEntries    int
	entries       map[analysisCacheKey]*analysisCacheEntry
	textModeIndex map[analysisTextModeKey]analysisCacheKey
	inFlight      map[analysisCacheKey]*analysisFlight
	order         []analysisCacheKey
}

func NewAnalysisCache(maxEntries int) *AnalysisCache {
	if maxEntries <= 0 {
		maxEntries = defaultAnalysisCacheEntries
	}
	return &AnalysisCache{
		maxEntries:    maxEntries,
		entries:       map[analysisCacheKey]*analysisCacheEntry{},
		textModeIndex: map[analysisTextModeKey]analysisCacheKey{},
		inFlight:      map[analysisCacheKey]*analysisFlight{},
	}
}

func (c *AnalysisCache) LookupTextMode(textHash, mode string) (*InputAnalysisResult, bool) {
	if c == nil || textHash == "" || mode == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	indexKey := analysisTextModeKey{TextHash: textHash, Mode: mode}
	key, ok := c.textModeIndex[indexKey]
	if !ok {
		return nil, false
	}
	entry, ok := c.entries[key]
	if !ok {
		delete(c.textModeIndex, indexKey)
		return nil, false
	}
	c.touchLocked(key, entry)
	return entry.result, true
}

func (c *AnalysisCache) GetOrCompute(key analysisCacheKey, compute func() (*InputAnalysisResult, error)) (*InputAnalysisResult, bool, time.Duration, error) {
	if c == nil {
		result, err := compute()
		return result, false, 0, err
	}

	waitStart := time.Now()
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		c.touchLocked(key, entry)
		c.mu.Unlock()
		return entry.result, true, 0, nil
	}
	if flight, ok := c.inFlight[key]; ok {
		c.mu.Unlock()
		<-flight.done
		return flight.result, flight.err == nil, time.Since(waitStart), flight.err
	}
	flight := &analysisFlight{done: make(chan struct{})}
	c.inFlight[key] = flight
	c.mu.Unlock()

	result, err := compute()

	c.mu.Lock()
	if err == nil && result != nil {
		c.storeLocked(key, result)
	}
	flight.result = result
	flight.err = err
	delete(c.inFlight, key)
	close(flight.done)
	c.mu.Unlock()

	return result, false, 0, err
}

func (c *AnalysisCache) Store(key analysisCacheKey, result *InputAnalysisResult) {
	if c == nil || result == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.storeLocked(key, result)
}

func (c *AnalysisCache) storeLocked(key analysisCacheKey, result *InputAnalysisResult) {
	c.entries[key] = &analysisCacheEntry{
		key:       key,
		result:    result,
		touchedAt: time.Now(),
	}
	c.textModeIndex[analysisTextModeKey{TextHash: key.TextHash, Mode: key.Mode}] = key
	c.rememberLocked(key)
}

func (c *AnalysisCache) touchLocked(key analysisCacheKey, entry *analysisCacheEntry) {
	entry.touchedAt = time.Now()
	c.rememberLocked(key)
}

func (c *AnalysisCache) rememberLocked(key analysisCacheKey) {
	nextOrder := c.order[:0]
	for _, existing := range c.order {
		if existing != key {
			nextOrder = append(nextOrder, existing)
		}
	}
	c.order = append(nextOrder, key)

	for len(c.order) > c.maxEntries {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
		for indexKey, indexedKey := range c.textModeIndex {
			if indexedKey == oldest {
				delete(c.textModeIndex, indexKey)
			}
		}
	}
}

func analysisTextHash(text string) string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func analysisDurationMS(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	return duration.Milliseconds()
}

func cloneInputAnalysisResult(result *InputAnalysisResult) *InputAnalysisResult {
	if result == nil {
		return nil
	}
	clone := *result
	if result.Timing != nil {
		timing := *result.Timing
		if result.Timing.Stages != nil {
			timing.Stages = map[string]int64{}
			for key, value := range result.Timing.Stages {
				timing.Stages[key] = value
			}
		}
		clone.Timing = &timing
	}
	return &clone
}
