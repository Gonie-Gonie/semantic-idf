package simulation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	simulationProgressInterval   = 500 * time.Millisecond
	maxSimulationDurationKeys    = 64
	maxSimulationDurationSamples = 8
)

type simulationDurationSample struct {
	TotalMS int64
	Phases  map[string]int64
	Order   []string
}

// Duration observations live only in this process. The key is a digest, never a
// stored model, path or execution ledger; both key and sample counts are bounded.
type simulationDurationCache struct {
	mu      sync.Mutex
	entries map[string][]simulationDurationSample
	order   []string
}

var sessionSimulationDurations = &simulationDurationCache{}

var simulationDurationConcurrency = struct {
	sync.Mutex
	active map[*simulationProgressTracker]int
}{active: map[*simulationProgressTracker]int{}}

// Independent overlapping runs cannot calibrate an idle-machine signature.
// A declared batch worker limit is stable context and already part of the key.
func registerSimulationDurationConcurrency(tracker *simulationProgressTracker, concurrency int) func() {
	simulationDurationConcurrency.Lock()
	simulationDurationConcurrency.active[tracker] = max(1, concurrency)
	active := len(simulationDurationConcurrency.active)
	for current, expected := range simulationDurationConcurrency.active {
		if active > expected {
			current.mu.Lock()
			current.sampleAllowed, current.key = false, ""
			current.mu.Unlock()
		}
	}
	simulationDurationConcurrency.Unlock()
	return func() {
		simulationDurationConcurrency.Lock()
		delete(simulationDurationConcurrency.active, tracker)
		simulationDurationConcurrency.Unlock()
	}
}

func (c *simulationDurationCache) samples(key string) []simulationDurationSample {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]simulationDurationSample(nil), c.entries[key]...)
}

func (c *simulationDurationCache) add(key string, sample simulationDurationSample) {
	if key == "" || sample.TotalMS <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string][]simulationDurationSample{}
	}
	if _, exists := c.entries[key]; !exists {
		if len(c.order) >= maxSimulationDurationKeys {
			delete(c.entries, c.order[0])
			c.order = c.order[1:]
		}
		c.order = append(c.order, key)
	}
	values := append(c.entries[key], sample)
	if len(values) > maxSimulationDurationSamples {
		values = values[len(values)-maxSimulationDurationSamples:]
	}
	c.entries[key] = values
}

type simulationProgressTracker struct {
	mu                                   sync.Mutex
	deliveryMu                           sync.Mutex
	callback                             func(SimulationProgress)
	now                                  func() time.Time
	cache                                *simulationDurationCache
	key                                  string
	started, phaseStarted, lastPublished time.Time
	current                              SimulationProgress
	phaseDurations                       map[string]int64
	phaseOrder                           []string
	itemPhases                           map[string]string
	sequence                             uint64
	closed                               bool
	sampleAllowed                        bool
	stop                                 chan struct{}
	done                                 chan struct{}
}

func newSimulationProgressTracker(callback func(SimulationProgress), now func() time.Time, cache *simulationDurationCache, heartbeat bool) *simulationProgressTracker {
	if now == nil {
		now = time.Now
	}
	if cache == nil {
		cache = sessionSimulationDurations
	}
	started := now()
	t := &simulationProgressTracker{callback: callback, now: now, cache: cache, started: started, phaseStarted: started,
		phaseDurations: map[string]int64{}, sampleAllowed: true, stop: make(chan struct{}), done: make(chan struct{})}
	if heartbeat && callback != nil {
		go func() {
			defer close(t.done)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					t.publish(true)
				case <-t.stop:
					return
				}
			}
		}()
	} else {
		close(t.done)
	}
	return t
}

func (t *simulationProgressTracker) setKey(key string) {
	t.mu.Lock()
	if t.sampleAllowed {
		t.key = key
	}
	t.mu.Unlock()
}

// observe is explicit per-run plumbing. No database/path global observer can
// leak progress between parallel runs or side-effect-free saved-result readers.
func (t *simulationProgressTracker) observe(item SimulationProgress) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	now := t.now()
	transition := item.Phase != t.current.Phase
	activityChanged := false
	if item.ItemRunID != "" {
		if t.itemPhases == nil {
			t.itemPhases = map[string]string{}
		}
		previous, found := t.itemPhases[item.ItemRunID]
		activityChanged = !found || previous != item.ItemPhase
		if item.ItemPhase == "complete" {
			delete(t.itemPhases, item.ItemRunID)
		} else {
			t.itemPhases[item.ItemRunID] = item.ItemPhase
		}
	}
	completionChanged := item.ItemRunID == "" && item.Completed != t.current.Completed
	if transition {
		if _, repeated := t.phaseDurations[item.Phase]; repeated {
			t.key, t.sampleAllowed = "", false
		}
		if t.current.Phase != "" {
			t.phaseDurations[t.current.Phase] += nonnegativeMS(now.Sub(t.phaseStarted))
		}
		t.phaseStarted = now
		t.phaseOrder = append(t.phaseOrder, item.Phase)
	}
	if item.ProgressKind == "" {
		item.ProgressKind = "indeterminate"
	}
	if item.Phase == "complete" && item.ItemRunID == "" {
		item.ProgressKind = "complete"
	}
	t.current = copySimulationProgress(item)
	t.mu.Unlock()
	t.publish(transition || activityChanged || completionChanged || item.ProgressKind == "complete")
}

func (t *simulationProgressTracker) work(completed, total int64, unit string, force bool) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	force = force || unit != t.current.WorkUnit
	t.current.WorkCompleted, t.current.WorkTotal = int64Pointer(completed), int64Pointer(total)
	t.current.WorkUnit, t.current.ProgressKind = unit, "work"
	t.mu.Unlock()
	t.publish(force)
}

func (t *simulationProgressTracker) engineLine(line string) {
	line = strings.TrimSpace(line)
	if !(strings.HasPrefix(line, "Warming up") || strings.HasPrefix(line, "Starting Simulation") ||
		strings.HasPrefix(line, "Continuing Simulation") || strings.HasPrefix(line, "Initializing") ||
		strings.HasPrefix(line, "Beginning") || strings.HasPrefix(line, "Adjusting") ||
		strings.HasPrefix(line, "Writing") || strings.HasPrefix(line, "Reporting")) {
		return
	}
	if len(line) > 240 {
		line = line[:240]
	}
	t.mu.Lock()
	if t.closed || t.current.Phase != "execute" {
		t.mu.Unlock()
		return
	}
	t.current.Message = line
	t.mu.Unlock()
	t.publish(false)
}

// Callback delivery is serialized separately from state mutation: a callback
// can read tracker state, and workers/heartbeat cannot emit out-of-order events.
func (t *simulationProgressTracker) publish(force bool) {
	t.deliveryMu.Lock()
	defer t.deliveryMu.Unlock()
	t.mu.Lock()
	if t.closed || t.current.Phase == "" {
		t.mu.Unlock()
		return
	}
	now := t.now()
	if !force && !t.lastPublished.IsZero() && now.Sub(t.lastPublished) < simulationProgressInterval {
		t.mu.Unlock()
		return
	}
	item := copySimulationProgress(t.current)
	item.ElapsedMS = int64Pointer(nonnegativeMS(now.Sub(t.started)))
	if item.ItemRunID == "" {
		item.PhaseElapsedMS = int64Pointer(nonnegativeMS(now.Sub(t.phaseStarted)))
	}
	t.sequence++
	item.Sequence = t.sequence
	t.lastPublished = now
	key := t.key
	finished := make(map[string]bool, len(t.phaseDurations))
	for phase := range t.phaseDurations {
		finished[phase] = true
	}
	t.mu.Unlock()
	if item.ProgressKind == "complete" {
		item.RemainingMS, item.RemainingLowMS, item.RemainingHighMS = int64Pointer(0), int64Pointer(0), int64Pointer(0)
		complete := 100.0
		item.OverallPercent = &complete
	} else {
		applySimulationDurationEstimate(&item, t.cache.samples(key), finished)
	}
	if t.callback != nil {
		t.callback(item)
	}
}

func (t *simulationProgressTracker) close(succeeded bool) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	close(t.stop)
	now := t.now()
	if t.current.Phase != "" && t.current.Phase != "complete" {
		t.phaseDurations[t.current.Phase] += nonnegativeMS(now.Sub(t.phaseStarted))
	}
	key := t.key
	sample := simulationDurationSample{TotalMS: nonnegativeMS(now.Sub(t.started)), Phases: t.phaseDurations, Order: append([]string(nil), t.phaseOrder...)}
	t.mu.Unlock()
	<-t.done
	// Wait for any already-started callback before the public call returns.
	t.deliveryMu.Lock()
	t.deliveryMu.Unlock()
	if succeeded {
		t.cache.add(key, sample)
	}
}

func applySimulationDurationEstimate(item *SimulationProgress, samples []simulationDurationSample, finished map[string]bool) {
	if len(samples) == 0 || item.PhaseElapsedMS == nil || item.ElapsedMS == nil {
		return
	}
	remaining := []int64{}
	for _, sample := range samples {
		phaseMS, found := sample.Phases[item.Phase]
		if !found || phaseMS <= 0 {
			continue
		}
		budget := phaseMS
		seenCurrent := false
		for _, phase := range sample.Order {
			if phase == item.Phase {
				seenCurrent = true
				continue
			}
			if seenCurrent && !finished[phase] {
				budget += sample.Phases[phase]
			}
		}
		remaining = append(remaining, budget-*item.PhaseElapsedMS)
	}
	if len(remaining) == 0 {
		return
	}
	sort.Slice(remaining, func(i, j int) bool { return remaining[i] < remaining[j] })
	median := remaining[len(remaining)/2]
	// Intervals refer to the observed current-stage plus future-stage budget,
	// not elapsed whole-run time. One observation intentionally stays broad.
	lowFactor, highFactor := 0.5, 2.0
	if len(remaining) >= 3 {
		lowFactor, highFactor = 0.75, 1.5
	}
	low := int64(float64(remaining[0]+*item.PhaseElapsedMS)*lowFactor) - *item.PhaseElapsedMS
	high := int64(float64(remaining[len(remaining)-1]+*item.PhaseElapsedMS)*highFactor) - *item.PhaseElapsedMS
	if high <= 0 {
		return
	} // An exceeded observation is unknown, never a frozen 99%.
	if median <= 0 {
		return
	}
	item.RemainingMS = int64Pointer(median)
	item.RemainingLowMS = int64Pointer(max(int64(0), low))
	item.RemainingHighMS = int64Pointer(high)
	item.EstimateBasis, item.EstimateSamples = "same_input_session", len(remaining)
	if item.ProgressKind == "indeterminate" {
		item.ProgressKind = "estimated"
	}
	denominator := *item.ElapsedMS + median
	if denominator > 0 {
		percent := math.Min(95, math.Max(0, float64(*item.ElapsedMS)/float64(denominator)*100))
		item.OverallPercent = &percent
	}
}

func nonnegativeMS(duration time.Duration) int64 { return max(int64(0), duration.Milliseconds()) }
func int64Pointer(value int64) *int64            { return &value }

func copySimulationProgress(item SimulationProgress) SimulationProgress {
	copyInt := func(value *int64) *int64 {
		if value == nil {
			return nil
		}
		return int64Pointer(*value)
	}
	item.WorkCompleted, item.WorkTotal = copyInt(item.WorkCompleted), copyInt(item.WorkTotal)
	item.RemainingMS, item.RemainingLowMS, item.RemainingHighMS = copyInt(item.RemainingMS), copyInt(item.RemainingLowMS), copyInt(item.RemainingHighMS)
	item.ElapsedMS, item.PhaseElapsedMS = copyInt(item.ElapsedMS), copyInt(item.PhaseElapsedMS)
	if item.OverallPercent != nil {
		value := *item.OverallPercent
		item.OverallPercent = &value
	}
	return item
}

func simulationDurationKey(request SimulationRunRequest, result *SimulationRunResult, settings SimulationSettings, concurrency int) string {
	hash := sha256.New()
	if request.Text != "" {
		io.WriteString(hash, request.Text)
	} else {
		input, err := os.Open(request.InputPath)
		if err != nil {
			return ""
		}
		_, err = io.Copy(hash, input)
		input.Close()
		if err != nil {
			return ""
		}
	}
	inputDigest := hex.EncodeToString(hash.Sum(nil))
	configuration := struct {
		Input, Engine, Version, Weather string
		Purpose                         *SimulationPurposeRequest
		Plan                            *PurposeRunPlan
		Mode, ResultMode                string
		Standard, ReadVars              bool
		Concurrency, CPUs               int
		WorkerFraction                  float64
		MaxWorkers                      int
	}{inputDigest, simulationFileIdentity(result.EnergyPlusExecutablePath), result.EnergyPlusVersion,
		simulationFileIdentity(result.WeatherPath), request.PurposeRequest, request.PurposeRunPlan,
		request.StandardOutputMode, request.ResultMode, request.StandardOutput, request.UseReadVarsESO,
		max(1, concurrency), runtime.GOMAXPROCS(0), settings.WorkerFraction, settings.MaxWorkers}
	data, err := json.Marshal(configuration)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func simulationFileIdentity(path string) string {
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err == nil {
		return fmt.Sprintf("%s\x00%d\x00%d", storagePathKey(path), info.Size(), info.ModTime().UnixNano())
	}
	return storagePathKey(path)
}

// Stream all stdout bytes while preserving the existing bounded diagnostic
// capture. Long/malformed lines and unterminated output stay memory-bounded.
type simulationEngineLineReader struct {
	partial []byte
	discard bool
	observe func(string)
}

func (r *simulationEngineLineReader) write(data []byte) {
	for _, value := range data {
		if value == '\n' || value == '\r' {
			if !r.discard && len(r.partial) > 0 && r.observe != nil {
				r.observe(string(r.partial))
			}
			r.partial, r.discard = r.partial[:0], false
		} else if !r.discard {
			if len(r.partial) >= 4096 {
				r.partial = r.partial[:0]
				r.discard = true
			} else {
				r.partial = append(r.partial, value)
			}
		}
	}
}
func (r *simulationEngineLineReader) finish() { r.write([]byte{'\n'}) }
