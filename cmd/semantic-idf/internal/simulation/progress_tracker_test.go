package simulation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type simulationProgressTestClock struct{ milliseconds atomic.Int64 }

func (c *simulationProgressTestClock) now() time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(c.milliseconds.Load()) * time.Millisecond)
}
func (c *simulationProgressTestClock) advance(milliseconds int64) { c.milliseconds.Add(milliseconds) }

func TestSimulationProgressTrackerColdMeasuredAndOverrun(t *testing.T) {
	cache := &simulationDurationCache{}
	clock := &simulationProgressTestClock{}
	var events []SimulationProgress
	capture := func(item SimulationProgress) { events = append(events, item) }
	first := newSimulationProgressTracker(capture, clock.now, cache, false)
	first.setKey("same-configuration")
	emitSimulationProgress(first.observe, "first", "execute", "running", "Engine", 4, 9, "")
	clock.advance(10000)
	emitSimulationProgress(first.observe, "first", "sql_series", "running", "SQL", 6, 9, "")
	first.work(4096, 0, "rows", true)
	clock.advance(30000)
	emitSimulationProgress(first.observe, "first", "energy_drivers", "running", "Drivers", 8, 9, "")
	clock.advance(20000)
	emitSimulationProgress(first.observe, "first", "complete", "succeeded", "Done", 9, 9, "")
	first.close(true)
	for index, event := range events {
		if event.Sequence != uint64(index+1) {
			t.Fatal("cold event sequence", events)
		}
		if event.Phase != "complete" && (event.RemainingMS != nil || event.OverallPercent != nil || event.ProgressKind == "estimated") {
			t.Fatal("invented cold-start estimate", event)
		}
	}
	if len(cache.samples("same-configuration")) != 1 {
		t.Fatal("successful run was not sampled")
	}
	if events[2].WorkTotal == nil || *events[2].WorkTotal != 0 || *events[2].WorkCompleted != 4096 {
		t.Fatal("unknown SQL denominator was lost", events[2])
	}
	encoded, err := json.Marshal(events[2])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"remainingMs"`)) || !bytes.Contains(encoded, []byte(`"workTotal":0`)) {
		t.Fatal("unknown and known-zero protocol conflated", string(encoded))
	}
	events = nil
	clock = &simulationProgressTestClock{}
	second := newSimulationProgressTracker(capture, clock.now, cache, false)
	second.setKey("same-configuration")
	emitSimulationProgress(second.observe, "second", "execute", "running", "Engine", 4, 9, "")
	measured := events[len(events)-1]
	if measured.RemainingMS == nil || *measured.RemainingMS != 60000 || measured.EstimateSamples != 1 || measured.EstimateBasis != "same_input_session" || measured.ProgressKind != "estimated" || *measured.RemainingLowMS != 30000 || *measured.RemainingHighMS != 120000 {
		t.Fatal("first empirical estimate not broad and labeled", measured)
	}
	clock.advance(12000)
	emitSimulationProgress(second.observe, "second", "sql_series", "running", "SQL", 6, 9, "")
	second.work(8192, 0, "rows", true)
	measured = events[len(events)-1]
	if *measured.PhaseElapsedMS != 0 || *measured.ElapsedMS != 12000 || *measured.RemainingMS != 50000 || measured.ProgressKind != "work" || measured.OverallPercent == nil || *measured.OverallPercent > 95 || *measured.OverallPercent == measured.Percent {
		t.Fatal("actual stage boundary did not calibrate whole-run ETA", measured)
	}
	clock.advance(30000)
	emitSimulationProgress(second.observe, "second", "energy_drivers", "running", "Drivers", 8, 9, "")
	clock.advance(20000)
	second.publish(true)
	if last := events[len(events)-1]; last.RemainingMS != nil || last.OverallPercent != nil || last.ProgressKind != "indeterminate" {
		t.Fatal("zero remaining advertised before phase completion", last)
	}
	clock.advance(21000)
	second.publish(true)
	if last := events[len(events)-1]; last.RemainingMS != nil || last.RemainingHighMS != nil || last.OverallPercent != nil {
		t.Fatal("overrun froze an estimate", last)
	}
	second.close(false)
	if len(cache.samples("same-configuration")) != 1 {
		t.Fatal("failed/incomplete run polluted samples")
	}
}

func TestSimulationProgressTrackerSerializesCallbacksAndBoundsSession(t *testing.T) {
	cache := &simulationDurationCache{}
	var active atomic.Int32
	var overlap atomic.Bool
	var previous uint64
	tracker := newSimulationProgressTracker(func(item SimulationProgress) {
		if active.Add(1) != 1 {
			overlap.Store(true)
		}
		if item.Sequence <= previous {
			t.Errorf("nonmonotone sequence %d after %d", item.Sequence, previous)
		}
		previous = item.Sequence
		runtime.Gosched()
		active.Add(-1)
	}, nil, cache, false)
	emitSimulationProgress(tracker.observe, "concurrent", "sql_series", "running", "Reading", 6, 9, "")
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for count := 0; count < 20; count++ {
				tracker.work(int64(count), 0, "rows", true)
				tracker.publish(true)
			}
		}()
	}
	workers.Wait()
	emitSimulationProgress(tracker.observe, "concurrent", "complete", "succeeded", "Done", 9, 9, "")
	tracker.close(false)
	if overlap.Load() || active.Load() != 0 {
		t.Fatal("callback delivery overlapped or survived close")
	}
	for key := 0; key < maxSimulationDurationKeys+10; key++ {
		for sample := 0; sample < maxSimulationDurationSamples+4; sample++ {
			cache.add(fmt.Sprint(key), simulationDurationSample{TotalMS: 1000})
		}
	}
	if len(cache.entries) != maxSimulationDurationKeys || len(cache.order) != maxSimulationDurationKeys || len(cache.samples("0")) != 0 || len(cache.samples(fmt.Sprint(maxSimulationDurationKeys+9))) != maxSimulationDurationSamples {
		t.Fatal("unbounded session sample cache")
	}
}

func TestSimulationProgressConcurrentRunInvalidatesHistoryAndPointerIsolation(t *testing.T) {
	cache := &simulationDurationCache{}
	cache.add("serial", simulationDurationSample{TotalMS: 10000, Phases: map[string]int64{"execute": 10000}, Order: []string{"execute"}})
	clock := &simulationProgressTestClock{}
	var events []SimulationProgress
	first := newSimulationProgressTracker(func(item SimulationProgress) {
		events = append(events, copySimulationProgress(item))
		if item.WorkCompleted != nil {
			*item.WorkCompleted = 999999
		}
		if item.WorkTotal != nil {
			*item.WorkTotal = 999999
		}
	}, clock.now, cache, false)
	unregisterFirst := registerSimulationDurationConcurrency(first, 1)
	defer unregisterFirst()
	first.setKey("serial")
	emitSimulationProgress(first.observe, "first", "execute", "running", "Engine", 4, 9, "")
	if events[len(events)-1].RemainingMS == nil {
		t.Fatal("serial sample not usable before contention")
	}
	first.work(4096, 0, "rows", true)
	clock.advance(1000)
	first.publish(true)
	if last := events[len(events)-1]; *last.WorkCompleted != 4096 || *last.WorkTotal != 0 {
		t.Fatal("callback mutation corrupted later telemetry", last)
	}
	second := newSimulationProgressTracker(nil, clock.now, cache, false)
	unregisterSecond := registerSimulationDurationConcurrency(second, 1)
	second.setKey("serial")
	clock.advance(1000)
	first.publish(true)
	if last := events[len(events)-1]; last.RemainingMS != nil || last.OverallPercent != nil {
		t.Fatal("serial ETA remained valid during overlapping execution", last)
	}
	first.close(true)
	second.close(true)
	unregisterSecond()
	if len(cache.samples("serial")) != 1 {
		t.Fatal("contended observations polluted serial signature")
	}
	// A declared two-worker batch remains eligible while at most two workers
	// are active; its different concurrency signature cannot reuse serial data.
	unregisterFirst()
	one := newSimulationProgressTracker(nil, clock.now, cache, false)
	two := newSimulationProgressTracker(nil, clock.now, cache, false)
	removeOne := registerSimulationDurationConcurrency(one, 2)
	removeTwo := registerSimulationDurationConcurrency(two, 2)
	one.setKey("batch-two")
	two.setKey("batch-two")
	if one.key != "batch-two" || two.key != "batch-two" {
		t.Fatal("normal bounded batch disabled its own measurements")
	}
	clock.advance(1000)
	one.close(true)
	two.close(true)
	removeOne()
	removeTwo()
}

func TestSimulationProgressBatchRowUpdatesCoalesceAcrossWorkers(t *testing.T) {
	clock := &simulationProgressTestClock{}
	events := 0
	tracker := newSimulationProgressTracker(func(SimulationProgress) { events++ }, clock.now, &simulationDurationCache{}, false)
	event := func(worker, rows int, phase string) SimulationProgress {
		return SimulationProgress{RunID: "batch", Phase: "item_progress", Status: "running", ItemRunID: fmt.Sprint(worker), ItemPhase: phase, ProgressKind: "work", WorkCompleted: int64Pointer(int64(rows)), WorkTotal: int64Pointer(0), WorkUnit: "rows"}
	}
	for worker := 0; worker < 48; worker++ {
		tracker.observe(event(worker, 0, "sql_series"))
	}
	if events != 48 || len(tracker.itemPhases) != 48 {
		t.Fatal("worker stage transitions were suppressed", events)
	}
	for round := 0; round < 10; round++ {
		for worker := 0; worker < 48; worker++ {
			tracker.observe(event(worker, (round+1)*4096, "sql_series"))
		}
	}
	if events != 48 {
		t.Fatal("alternating worker row updates bypassed root throttle", events)
	}
	clock.advance(500)
	for worker := 0; worker < 48; worker++ {
		tracker.observe(event(worker, 50000, "sql_series"))
	}
	if events != 49 {
		t.Fatal("root coalescing did not publish one latest row snapshot", events)
	}
	for worker := 0; worker < 48; worker++ {
		tracker.observe(event(worker, 50000, "complete"))
	}
	if events != 97 || len(tracker.itemPhases) != 0 {
		t.Fatal("worker completion did not flush and release child stage map", events, len(tracker.itemPhases))
	}
	tracker.close(false)
}

func TestSimulationProgressSignatureSeparatesInputPurposeEngineWeatherAndConcurrency(t *testing.T) {
	directory := t.TempDir()
	engine, weather, input := filepath.Join(directory, "energyplus.exe"), filepath.Join(directory, "weather.epw"), filepath.Join(directory, "model.idf")
	for path, text := range map[string]string{engine: "engine", weather: "weather", input: "Version,24.2;"} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	request := SimulationRunRequest{InputPath: input, PurposeRequest: &SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}}
	result := &SimulationRunResult{EnergyPlusExecutablePath: engine, EnergyPlusVersion: "24.2", WeatherPath: weather}
	settings := DefaultSettings()
	base := simulationDurationKey(request, result, settings, 1)
	if base == "" || strings.Contains(base, "model") {
		t.Fatal("non-digest key", base)
	}
	textRequest := request
	textRequest.Text = "Version,24.2;"
	if simulationDurationKey(textRequest, result, settings, 1) != base {
		t.Fatal("same source content has different key")
	}
	changed := textRequest
	changed.Text += "Zone,Office;"
	if simulationDurationKey(changed, result, settings, 1) == base {
		t.Fatal("input edits reused samples")
	}
	purpose := *request.PurposeRequest
	purpose.Scope.ZoneMode = "selected"
	purpose.Scope.ZoneNames = []string{"Office"}
	changed = request
	changed.PurposeRequest = &purpose
	if simulationDurationKey(changed, result, settings, 1) == base {
		t.Fatal("scope reused samples")
	}
	changed = request
	purpose = *request.PurposeRequest
	purpose.ZoneHeatFlowDetail = PurposeZoneHeatFlowDetailSurface
	changed.PurposeRequest = &purpose
	if simulationDurationKey(changed, result, settings, 1) == base {
		t.Fatal("detail reused samples")
	}
	if simulationDurationKey(request, result, settings, 4) == base {
		t.Fatal("batch concurrency reused serial samples")
	}
	oldProcs := runtime.GOMAXPROCS(0)
	runtime.GOMAXPROCS(oldProcs + 1)
	changedProcs := simulationDurationKey(request, result, settings, 1)
	runtime.GOMAXPROCS(oldProcs)
	if changedProcs == base {
		t.Fatal("runtime CPU concurrency reused samples")
	}
	result.EnergyPlusVersion = "25.1"
	if simulationDurationKey(request, result, settings, 1) == base {
		t.Fatal("engine version reused samples")
	}
	result.EnergyPlusVersion = "24.2"
	if err := os.WriteFile(weather, []byte("different weather contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if simulationDurationKey(request, result, settings, 1) == base {
		t.Fatal("weather fingerprint reused samples")
	}
}

func TestSimulationProgressStreamingCaptureContinuesPastLimit(t *testing.T) {
	var captured strings.Builder
	var lines []string
	text := strings.Repeat("x", maxCapturedOutputBytes+100) + "\r\nWarming up {3}\r\nStarting Simulation at 01/01 for Annual\nContinuing Simulation at 01/21"
	copyLimitedWithProgress(&captured, strings.NewReader(text), maxCapturedOutputBytes, func(line string) { lines = append(lines, line) })
	if captured.Len() != maxCapturedOutputBytes || !reflect.DeepEqual(lines, []string{"Warming up {3}", "Starting Simulation at 01/01 for Annual", "Continuing Simulation at 01/21"}) {
		t.Fatal("bounded capture suppressed live trailing progress", captured.Len(), lines)
	}
	clock := &simulationProgressTestClock{}
	var events []SimulationProgress
	tracker := newSimulationProgressTracker(func(event SimulationProgress) { events = append(events, event) }, clock.now, &simulationDurationCache{}, false)
	emitSimulationProgress(tracker.observe, "engine", "execute", "running", "Engine", 4, 9, "")
	clock.advance(1000)
	tracker.engineLine(lines[0])
	clock.advance(1000)
	tracker.engineLine(lines[1])
	last := events[len(events)-1]
	if last.Message != lines[1] || last.Phase != "execute" || last.WorkTotal != nil || last.RemainingMS != nil || *last.PhaseElapsedMS != 2000 {
		t.Fatal("stdout fabricated engine percent or reset phase time", last)
	}
	tracker.close(false)
}

func TestSimulationProgressCommandStreamsWithoutTruncatingDiagnostics(t *testing.T) {
	if os.Getenv("SEMANTIC_IDF_TEST_PROGRESS_COMMAND") == "1" {
		fmt.Fprintln(os.Stdout, strings.Repeat("x", maxCapturedOutputBytes+10))
		fmt.Fprintln(os.Stdout, "Warming up {4}")
		fmt.Fprintln(os.Stdout, "Continuing Simulation at 02/10")
		fmt.Fprintln(os.Stderr, "synthetic stderr diagnostic")
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestSimulationProgressCommandStreamsWithoutTruncatingDiagnostics$")
	command.Env = append(os.Environ(), "SEMANTIC_IDF_TEST_PROGRESS_COMMAND=1")
	var lines []string
	stdout, stderr, exitCode, err := runCommandCapturedWithProgress(command, func(line string) { lines = append(lines, line) })
	if err != nil || exitCode != 0 || len(stdout) != maxCapturedOutputBytes || !strings.Contains(stderr, "synthetic stderr diagnostic") || !stringSliceContains(lines, "Warming up {4}") || !stringSliceContains(lines, "Continuing Simulation at 02/10") {
		t.Fatal("engine progress pipe/capture regression", err, exitCode, len(stdout), stderr, lines)
	}
}

func TestSimulationProgressSQLReadersKeepResultsAndOriginalFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "original.sql")
	createTestEnergyPlusSQL(t, path)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantSeries, err := parseSimulationSQLSeriesForPlan(path, PurposeRunPlan{})
	if err != nil {
		t.Fatal(err)
	}
	wantHeat, err := parseSimulationHeatFlowSQL(path)
	if err != nil {
		t.Fatal(err)
	}
	var work []sqlWorkProgress
	observer := func(value sqlWorkProgress) { work = append(work, value) }
	gotSeries, err := parseSimulationSQLSeriesForPlanWithProgress(path, PurposeRunPlan{}, observer)
	if err != nil {
		t.Fatal(err)
	}
	gotHeat, err := parseSimulationHeatFlowSQLWithProgress(path, observer)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotSeries) == 0 || len(gotHeat.Zones) == 0 || !reflect.DeepEqual(gotSeries, wantSeries) || !reflect.DeepEqual(gotHeat, wantHeat) {
		t.Fatal("instrumentation changed SQL result values")
	}
	rowFinishes, seriesFinishes := 0, 0
	for _, value := range work {
		if value.Unit == "rows" {
			if value.Total != 0 {
				t.Fatal("unknown sparse observation count became exact", value)
			}
			if value.Finished {
				rowFinishes++
				if value.Completed != 8 {
					t.Fatal("actual raw rows not counted", value)
				}
			}
		}
		if value.Unit == "series" && value.Finished {
			seriesFinishes++
			if value.Total <= 0 || value.Completed != value.Total {
				t.Fatal("known series denominator incorrect", value)
			}
		}
	}
	if rowFinishes != 2 || seriesFinishes != 1 {
		t.Fatal("both initial readers must report consumed work", work)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("SQL progress modified source database", err)
	}
}

func TestSimulationProgressSQLCompactAndFallbackCounters(t *testing.T) {
	for _, keyType := range []string{"INTEGER", "TEXT"} {
		t.Run(keyType, func(t *testing.T) {
			db, path := compactReportDataFixture(t,
				"CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex "+keyType+")",
				"CREATE TABLE Time (TimeIndex "+keyType+")",
				"CREATE TABLE ReportData (TimeIndex "+keyType+", ReportDataDictionaryIndex "+keyType+", Value REAL)",
				`INSERT INTO ReportDataDictionary VALUES(1)`, `INSERT INTO Time VALUES(1)`,
				`WITH RECURSIVE observations(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM observations WHERE n<8200) INSERT INTO ReportData SELECT 1,1,CASE WHEN n%2=0 THEN NULL ELSE 42 END FROM observations`,
			)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want, err := QueryReportData(db, SQLSeriesQuery{})
			if err != nil {
				t.Fatal(err)
			}
			var got []SQLSeriesRow
			var work []sqlWorkProgress
			err = walkReportDataCompactWithProgress(db, SQLSeriesQuery{}, func(row SQLSeriesRow) error { got = append(got, row); return nil }, func(value sqlWorkProgress) { work = append(work, value) })
			if err != nil || !reflect.DeepEqual(got, want) || len(got) != 8200 {
				t.Fatal("counted walker changed NULL rows/fallback semantics", err, len(got))
			}
			if len(work) != 4 || work[1].Completed != 4096 || work[2].Completed != 8192 || work[3].Completed != 8200 || !work[3].Finished {
				t.Fatal("row observer did not use sparse batches and actual final count", work)
			}
			for _, value := range work {
				if value.Total != 0 || value.Unit != "rows" {
					t.Fatal("fabricated count denominator", value)
				}
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("counted walker changed SQL bytes", err)
			}
		})
	}
}

func TestSimulationProgressRunnerAndParallelBatchTelemetry(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEMANTIC_IDF_TEST_PURPOSE_ENGINE", "1")
	directory := t.TempDir()
	sqlPath := filepath.Join(directory, "eplusout.sql")
	createTestEnergyPlusSQL(t, sqlPath)
	before, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	var events []SimulationProgress
	result, err := RunSimulation(SimulationRunRequest{RunID: "  progress/fixture  ", Text: "Version,24.2;", Filename: "input.idf", OutputDirectory: directory, EnergyPlusExecutablePath: executable}, func(item SimulationProgress) { events = append(events, item) }, SimulationSettings{})
	if err != nil || result == nil || result.Status != "succeeded" {
		t.Fatal("synthetic runner failed", result, err)
	}
	if result.RunID != NormalizeSimulationRunID("  progress/fixture  ") {
		t.Fatal("transport and runner identity differ", result.RunID)
	}
	scanned := map[string]int64{}
	for index, event := range events {
		if event.RunID != result.RunID || event.Sequence != uint64(index+1) || event.ElapsedMS == nil || event.PhaseElapsedMS == nil {
			t.Fatal("runner telemetry lost identity/order/timing", event)
		}
		if event.WorkUnit == "rows" && event.WorkCompleted != nil {
			scanned[event.Phase] = max(scanned[event.Phase], *event.WorkCompleted)
		}
		if event.Phase != "complete" && event.ProgressKind == "complete" {
			t.Fatal("backend advertised early whole-run completion", event)
		}
	}
	last := events[len(events)-1]
	if scanned["sql_series"] != 8 || scanned["sql_heat_flow"] != 8 || last.ProgressKind != "complete" || last.OverallPercent == nil || *last.OverallPercent != 100 {
		t.Fatal("actual SQL work/terminal telemetry absent", scanned, last)
	}
	after, err := os.ReadFile(sqlPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("runner telemetry changed sourceSQL", err)
	}
	paths := []string{writeBatchEngineInput(t, t.TempDir(), "first.idf", "Version,24.2;"), writeBatchEngineInput(t, t.TempDir(), "second.idf", "Version,24.2;")}
	events = nil
	batch, err := RunMultipleSimulations(MultiSimulationRequest{RunID: "batch-progress", InputPaths: paths, EnergyPlusExecutablePath: executable, WorkerCount: 2}, func(item SimulationProgress) { events = append(events, item) }, SimulationSettings{RunDirectory: t.TempDir()})
	if err != nil || batch.Completed != 2 || batch.Succeeded != 2 {
		t.Fatal("parallel synthetic batch failed", batch, err)
	}
	children := map[string]bool{}
	terminals := 0
	for index, event := range events {
		if event.RunID != batch.RunID || event.Sequence != uint64(index+1) {
			t.Fatal("parallel events leaked child run identity or ordering", event)
		}
		if event.ItemRunID != "" {
			children[event.ItemRunID] = true
			if event.Phase != "item_progress" || event.ItemPhase == "" || event.ItemPath == "" || event.Active < 1 || event.Queued < 0 || event.Status != "running" || event.RemainingMS != nil || event.ProgressKind == "complete" {
				t.Fatal("child progress appeared as root terminal/ETA", event)
			}
		}
		if event.Phase == "complete" {
			terminals++
		}
	}
	if len(children) != 2 || terminals != 1 || events[len(events)-1].ProgressKind != "complete" {
		t.Fatal("parallel workers did not retain distinct live progress", children, terminals)
	}
}
