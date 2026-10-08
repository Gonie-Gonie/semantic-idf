package simulation

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Optional field validation for TestPurposeSavedBundleReplay. Run with
// SEMANTIC_IDF_BUNDLE_REPLAY_PROGRESS=1 and -count=2 to observe first-use and
// same-input session estimates without launching an engine. This measures only
// post-processing; engine execution and desktop transport are deliberately absent.
type purposeSavedProgressReplay struct {
	t            *testing.T
	tracker      *simulationProgressTracker
	sqlPath      string
	priorSamples int
	mu           sync.Mutex
	phase        string
	events       []SimulationProgress
	lastRows     map[string]int64
	finishedRows map[string]int64
}

func newPurposeSavedProgressReplay(t *testing.T, directory, input string, plan []byte) *purposeSavedProgressReplay {
	t.Helper()
	inputBytes, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	sqlPath := filepath.Join(directory, "eplusout.sql")
	hash := sha256.New()
	_, _ = hash.Write(inputBytes)
	_, _ = hash.Write(plan)
	_, _ = hash.Write([]byte(simulationFileIdentity(sqlPath)))
	_, _ = hash.Write([]byte(strconv.Itoa(runtime.GOMAXPROCS(0))))
	key := fmt.Sprintf("saved-postprocessing:%x", hash.Sum(nil))
	replay := &purposeSavedProgressReplay{
		t: t, sqlPath: sqlPath, priorSamples: len(sessionSimulationDurations.samples(key)),
		lastRows: map[string]int64{}, finishedRows: map[string]int64{},
	}
	replay.tracker = newSimulationProgressTracker(replay.capture, time.Now, sessionSimulationDurations, true)
	replay.tracker.setKey(key)
	t.Cleanup(func() { replay.tracker.close(false) })
	t.Logf("post-processing telemetry: prior successful same-capture samples=%d; snapshot has warmed file caches", replay.priorSamples)
	return replay
}

func (replay *purposeSavedProgressReplay) observe(item SimulationProgress) {
	replay.mu.Lock()
	replay.phase = item.Phase
	replay.mu.Unlock()
	replay.tracker.observe(item)
}

func (replay *purposeSavedProgressReplay) work(work sqlWorkProgress) {
	replay.mu.Lock()
	phase := replay.phase
	if work.Unit == "rows" {
		if work.Completed < replay.lastRows[phase] {
			replay.t.Errorf("%s consumed rows decreased: %d -> %d", phase, replay.lastRows[phase], work.Completed)
		}
		replay.lastRows[phase] = work.Completed
		if work.Finished {
			replay.finishedRows[phase] = work.Completed
			replay.t.Logf("telemetry %s consumed SQL rows=%d (total remains unknown while reading)", phase, work.Completed)
		}
	}
	replay.mu.Unlock()
	replay.tracker.work(work.Completed, work.Total, work.Unit, work.Finished)
}

func (replay *purposeSavedProgressReplay) capture(item SimulationProgress) {
	replay.mu.Lock()
	defer replay.mu.Unlock()
	if len(replay.events) == 0 || replay.events[len(replay.events)-1].Phase != item.Phase {
		replay.t.Logf("telemetry phase=%s kind=%s elapsedMs=%s remainingMs=%s..%s basis=%s samples=%d",
			item.Phase, item.ProgressKind, purposeReplayOptionalMillis(item.ElapsedMS), purposeReplayOptionalMillis(item.RemainingLowMS), purposeReplayOptionalMillis(item.RemainingHighMS), item.EstimateBasis, item.EstimateSamples)
	}
	replay.events = append(replay.events, item)
}

func (replay *purposeSavedProgressReplay) finish(plan PurposeRunPlan) {
	replay.t.Helper()
	emitSimulationProgress(replay.observe, "saved-replay", "complete", "succeeded", "Saved post-processing complete", simulationProgressTotal, simulationProgressTotal, "")
	replay.tracker.close(true)
	replay.mu.Lock()
	events := append([]SimulationProgress(nil), replay.events...)
	replay.mu.Unlock()
	if len(events) == 0 {
		replay.t.Fatal("saved pipeline emitted no telemetry")
	}
	var previousSequence uint64
	var previousElapsed int64
	estimates := 0
	for _, event := range events {
		if event.Sequence <= previousSequence || event.ElapsedMS == nil || event.PhaseElapsedMS == nil || *event.ElapsedMS < previousElapsed || *event.PhaseElapsedMS < 0 {
			replay.t.Fatalf("invalid monotone telemetry clock/sequence: %+v", event)
		}
		previousSequence, previousElapsed = event.Sequence, *event.ElapsedMS
		if event.WorkCompleted != nil && (*event.WorkCompleted < 0 || (event.WorkTotal != nil && *event.WorkTotal > 0 && *event.WorkCompleted > *event.WorkTotal)) {
			replay.t.Errorf("invalid consumed-work bounds: %+v", event)
		}
		if event.Phase == "complete" {
			continue
		}
		if event.ProgressKind == "complete" || (event.OverallPercent != nil && *event.OverallPercent >= 100) {
			replay.t.Errorf("running post-processing claimed whole-run completion: %+v", event)
		}
		if event.RemainingMS != nil {
			estimates++
			if replay.priorSamples == 0 || event.EstimateBasis != "same_input_session" || event.EstimateSamples < 1 ||
				event.RemainingLowMS == nil || event.RemainingHighMS == nil || *event.RemainingLowMS < 0 || *event.RemainingMS < *event.RemainingLowMS || *event.RemainingMS > *event.RemainingHighMS {
				replay.t.Errorf("unsupported estimate or invalid range: %+v", event)
			}
		}
	}
	last := events[len(events)-1]
	if last.Phase != "complete" || last.ProgressKind != "complete" || last.OverallPercent == nil || *last.OverallPercent != 100 || last.RemainingMS == nil || *last.RemainingMS != 0 {
		replay.t.Fatalf("saved pipeline lost its final completion: %+v", last)
	}
	if replay.priorSamples > 0 && estimates == 0 {
		replay.t.Error("repeated saved pipeline never exposed its successful session observation")
	}
	replay.t.Logf("telemetry validated: events=%d estimates=%d priorSamples=%d finalElapsedMs=%d", len(events), estimates, replay.priorSamples, *last.ElapsedMS)
	reported := map[string]bool{}
	for _, event := range events {
		if event.Phase == "complete" || event.RemainingMS == nil || reported[event.Phase] {
			continue
		}
		reported[event.Phase] = true
		actual := *last.ElapsedMS - *event.ElapsedMS
		covered := *event.RemainingLowMS <= actual && actual <= *event.RemainingHighMS
		replay.t.Logf("ETA observation phase=%s predictedMs=%d intervalMs=%d..%d actualRemainingMs=%d covered=%v (diagnostic, no latency threshold)",
			event.Phase, *event.RemainingMS, *event.RemainingLowMS, *event.RemainingHighMS, actual, covered)
	}
	// Aggregate SQL independently after closing the timed tracker. This extra
	// read belongs to test validation, not the application's progress latency.
	replay.verifyConsumedRows(plan)
}

func purposeReplayOptionalMillis(value *int64) string {
	if value == nil {
		return "unknown"
	}
	return strconv.FormatInt(*value, 10)
}

func (replay *purposeSavedProgressReplay) verifyConsumedRows(plan PurposeRunPlan) {
	replay.t.Helper()
	db, err := openSimulationSQLiteReadOnly(replay.sqlPath)
	if err != nil {
		replay.t.Fatal(err)
	}
	defer db.Close()
	series, err := sqlOutputSeriesDictionaries(db, plan)
	if err != nil {
		replay.t.Fatal(err)
	}
	heatFlow, err := sqlOutputHeatFlowDictionaries(db)
	if err != nil {
		replay.t.Fatal(err)
	}
	for phase, dictionaries := range map[string][]sqlOutputDictionaryRow{"sql_series": series, "sql_heat_flow": heatFlow} {
		ids := make([]string, 0, len(dictionaries))
		for _, dictionary := range dictionaries {
			if phase == "sql_heat_flow" && strings.TrimSpace(dictionary.keyValue) == "" {
				continue
			}
			ids = append(ids, strconv.Itoa(dictionary.index))
		}
		if len(ids) == 0 {
			continue
		}
		var want int64
		if err := db.QueryRow("SELECT COUNT(*) FROM ReportData WHERE ReportDataDictionaryIndex IN (" + strings.Join(ids, ",") + ")").Scan(&want); err != nil {
			replay.t.Fatal(err)
		}
		if got := replay.finishedRows[phase]; got != want {
			replay.t.Errorf("%s terminal consumed rows=%d, independently counted SQL rows=%d", phase, got, want)
		}
		replay.t.Logf("%s independently verified SQL row count=%d", phase, want)
	}
}
