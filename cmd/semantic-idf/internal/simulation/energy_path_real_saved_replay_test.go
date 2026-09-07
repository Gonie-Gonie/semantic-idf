package simulation

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// This explicit saved-run acceptance never launches an engine, creates output
// files, updates captured results, or blesses an expected-value manifest.
func TestEnergyPathRealLargeOfficeSavedReplay(t *testing.T) {
	directory := strings.TrimSpace(os.Getenv("EPATH_REAL_REPLAY_DIR"))
	if directory == "" {
		t.Skip("actual saved LargeOffice replay requires EPATH_REAL_REPLAY_DIR")
	}
	if os.Getenv("EPATH_REAL_CAPTURE") == "1" || os.Getenv("EPATH_REAL_RUN") == "1" {
		t.Fatal("saved replay cannot be combined with an engine-run mode")
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	before := epathReplayDirectorySnapshot(t, directory)
	defer func() {
		after := epathReplayDirectorySnapshot(t, directory)
		if !reflect.DeepEqual(before, after) {
			t.Error("saved replay changed original artifact files, hashes, timestamps or directory membership")
		}
	}()

	var evidence epathRealRunEvidence
	epathReplayReadJSON(t, filepath.Join(directory, "run-evidence.json"), &evidence)
	root, catalogDirectory := epathRealDirectories(t)
	if !epathRealSamePath(directory, evidence.RunDirectory) || evidence.Version != "25.1" || evidence.Fixture.ID != "large-office-25-1" {
		t.Fatal("this bounded replay requires the exact captured official 25.1 LargeOffice run")
	}
	matches := 0
	for _, fixture := range epathLoadRealCatalog(t, catalogDirectory).Fixtures {
		if fixture.ID == evidence.Fixture.ID && reflect.DeepEqual(fixture, evidence.Fixture) {
			matches++
		}
	}
	if matches != 1 {
		t.Fatal("captured fixture does not uniquely match the unchanged official model catalog")
	}
	if err := epathValidateSavedRealEvidence(root, catalogDirectory, evidence); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := epathValidateSavedRealEvidence(root, catalogDirectory, evidence); err != nil {
			t.Error(err)
		}
	}()

	var request SimulationRunRequest
	epathReplayReadJSON(t, filepath.Join(directory, "capture-request.json"), &request)
	if request.PurposeRequest == nil || request.PurposeRunPlan == nil ||
		request.PurposeRequest.BasicEnergyDetail != PurposeBasicEnergyDetailEnergyPath ||
		epathRealHash([]byte(request.Text)) != evidence.ExecutedSHA256 {
		t.Fatal("discovery request is not bound to the exact executed Energy Path model")
	}

	t.Log("Rebuilding once through LoadEnergyPathProjection from the hash-bound saved SQL")
	projection, err := LoadEnergyPathProjection(EnergyPathProjectionRequest{
		ResultPath: evidence.SQLPath, InputPath: evidence.InputPath,
		Scope: "building", Period: "annual", Service: "all",
	})
	if err != nil {
		t.Fatal(err)
	}
	provenance := projection.Provenance
	if provenance == nil || !provenance.InputVerified || !provenance.OutputPlanKnown ||
		provenance.InputHash != evidence.ExecutedSHA256 || len(provenance.Warnings) != 0 {
		t.Fatalf("saved loader lost verified input/output-plan provenance: %+v", provenance)
	}
	epathReplayAssertSameFile(t, provenance.SQLPath, evidence.SQLPath)
	epathReplayAssertSameFile(t, provenance.InputPath, evidence.InputPath)
	if projection.Selection != (EnergyPathSelection{Scope: "building", Period: "annual", Service: "all"}) ||
		projection.View.Scope.Kind != "building" || projection.View.Period != "annual" || projection.View.Service != "all" {
		t.Fatalf("unexpected shared-loader selection/view: %+v / %+v", projection.Selection, projection.View.Scope)
	}
	explanation := projection.PurposeResults.EnergyExplanation
	if explanation.Schema != energyExplanationSchema || explanation.Scope.Kind != "building" {
		t.Fatal("shared loader did not return canonical Building Energy Path")
	}
	epathReplayAssertQuality(t, "canonical Building", explanation.Quality)
	epathReplayAssertQuality(t, "selected view", projection.View.Quality)
	epathRealAssertReportedAvailability(t, explanation.Completeness, 12, 18, 9, 14)
	epathReplayAssertPeriods(t, "Building", explanation.Periods)
	epathReplayAssertLoads(t, "Building annual", explanation.Nodes)
	epathReplayAssertLoads(t, "selected annual", projection.View.Nodes)
	if len(explanation.ZoneResults) != 19 {
		t.Fatalf("canonical Zones = %d, want 19", len(explanation.ZoneResults))
	}
	zones := map[string]bool{}
	for _, zone := range explanation.ZoneResults {
		name := strings.ToLower(strings.TrimSpace(zone.Scope.ZoneName))
		if zone.Scope.Kind != "zone" || name == "" || zones[name] {
			t.Fatalf("ambiguous/invalid Zone scope: %+v", zone.Scope)
		}
		zones[name] = true
		epathReplayAssertPeriods(t, zone.Scope.ZoneName, zone.Periods)
	}
	t.Log("Actual shared replay: Drivers 12/18, Loads 9/14, End uses 9/10, Carriers 2/2; two canonical loads, 13 periods, 19 Zones")

	// Inspect actual observations independently of saved discovery or candidate
	// quality. There is no synthetic SQL and no substitution of a dictionary row
	// from another key. Explicit observed zero remains a present observation.
	db, err := openSimulationSQLiteReadOnly(evidence.SQLPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	weather, err := epathReadOracleWeather(db)
	if err != nil {
		t.Fatal(err)
	}
	if weather.Year != 2017 || len(weather.Months) != 12 || weather.CoverageBasis == "" {
		t.Fatalf("actual full annual weather coverage is not proven: %+v", weather)
	}
	surfaces := epathReplayMonthlyDictionary(t, db, "Surface Inside Face Convection Heat Gain Energy")
	people := epathReplayMonthlyDictionary(t, db, "Zone People Convective Heating Energy")
	missingTransfer := epathReplayMonthlyDictionary(t, db, "Surface Inside Face Convection Heat Transfer Energy")
	if len(surfaces) != 158 || len(people) != 16 || len(missingTransfer) != 0 {
		t.Fatalf("actual Monthly SQL key counts surfaceGain=%d people=%d unavailableTransfer=%d", len(surfaces), len(people), len(missingTransfer))
	}
	if people["BASEMENT"].total <= 0 || surfaces["BASEMENT_WALL_EAST"].total <= 0 {
		t.Fatal("actual BASEMENT People / east-wall SQL evidence is absent or not positive")
	}
	for _, key := range []string{"GROUNDFLOOR_PLENUM", "MIDFLOOR_PLENUM", "TOPFLOOR_PLENUM"} {
		if _, exists := people[key]; exists {
			t.Fatalf("plenum unexpectedly acquired actual People observations: %s", key)
		}
	}

	discovery, err := DiscoverAvailableOutputs(OutputDiscoveryRequest{
		Text: request.Text, PurposeRequest: request.PurposeRequest,
		OutputDirectory: directory, SQLPath: evidence.SQLPath,
		RDDPath: filepath.Join(directory, "eplusout.rdd"), MDDPath: filepath.Join(directory, "eplusout.mdd"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"GROUNDFLOOR_PLENUM", "MIDFLOOR_PLENUM", "TOPFLOOR_PLENUM"} {
		resolution := epathReplayResolution(t, discovery, key, "Zone People Convective Heating Energy")
		if resolution.Status != "fallback" || resolution.ResolvedName != "" || resolution.ResolutionBasis != "" {
			t.Errorf("missing exact plenum borrowed a different Zone: %+v", resolution)
		}
	}
	for key, observation := range people {
		resolution := epathReplayResolution(t, discovery, key, observation.name)
		if resolution.Status != "available" || resolution.ResolvedName != observation.name ||
			resolution.ResolutionBasis != "exact" || !strings.Contains(resolution.Source, "sql") || resolution.Units != "J" {
			t.Errorf("reported People exact key was not preserved: %+v", resolution)
		}
		epathReplayAssertOriginalSource(t, explanation.Sources, observation)
	}
	for key, observation := range surfaces {
		resolution := epathReplayResolution(t, discovery, key, "Surface Inside Face Convection Heat Transfer Energy")
		if resolution.Status != "alias" || resolution.ResolvedName != observation.name ||
			resolution.ResolutionBasis != "version_alias" || !strings.Contains(resolution.Source, "sql") ||
			resolution.Units != "J" || !strings.EqualFold(resolution.KeyValue, key) {
			t.Errorf("surface version alias left its exact observed key: %+v", resolution)
		}
		epathReplayAssertOriginalSource(t, explanation.Sources, observation)
	}
	t.Log("Actual discovery: all 158 surface aliases retain their reported key/name/J/Monthly; 16 People keys exact, three absent plenums unresolved")
}

func epathReplayAssertQuality(t *testing.T, label string, quality *EnergyPathQuality) {
	t.Helper()
	if quality == nil {
		t.Fatalf("%s quality missing", label)
	}
	for _, check := range []struct {
		name         string
		actual       EnergyCompletenessLevel
		found, total int
	}{
		{"Drivers", quality.Drivers, 12, 18}, {"Loads", quality.Loads, 9, 14},
		{"End uses", quality.EndUses, 9, 10}, {"Carriers", quality.Carriers, 2, 2},
	} {
		if check.actual.Found != check.found || check.actual.Total != check.total {
			t.Errorf("%s %s = %d/%d, want %d/%d", label, check.name, check.actual.Found, check.actual.Total, check.found, check.total)
		}
	}
}

func epathReplayAssertPeriods(t *testing.T, label string, periods []EnergyPeriod) {
	t.Helper()
	if len(periods) != 13 {
		t.Fatalf("%s periods = %d, want 13", label, len(periods))
	}
	expected := map[string]bool{"annual": false}
	for month := 1; month <= 12; month++ {
		expected[fmt.Sprintf("M%d", month)] = false
	}
	for _, period := range periods {
		seen, exists := expected[period.ID]
		if !exists || seen {
			t.Fatalf("%s has extra/duplicate period %q", label, period.ID)
		}
		expected[period.ID] = true
	}
	for id, seen := range expected {
		if !seen {
			t.Errorf("%s lacks %s", label, id)
		}
	}
}

func epathReplayAssertLoads(t *testing.T, label string, nodes []EnergyExplanationNode) {
	t.Helper()
	services := map[string]bool{}
	count := 0
	for _, node := range nodes {
		if node.Level != "load" {
			continue
		}
		count++
		if node.ServiceKind != "cooling" && node.ServiceKind != "heating" || services[node.ServiceKind] || node.ScaleDomain != "thermal" {
			t.Errorf("%s ambiguous/nonthermal canonical load: %+v", label, node)
		}
		services[node.ServiceKind] = true
	}
	if count != 2 || !services["cooling"] || !services["heating"] {
		t.Fatalf("%s canonical load count = %d (%v), want cooling/heating only", label, count, services)
	}
}

type epathReplayObservation struct {
	key, name, unit      string
	rows, values, months int
	total                float64
}

func epathReplayMonthlyDictionary(t *testing.T, db *sql.DB, name string) map[string]epathReplayObservation {
	t.Helper()
	rows, err := db.Query(`SELECT d.KeyValue, d.Name, d.Units, count(r.ReportDataIndex),
 count(r.Value), count(DISTINCT tm.Month), sum(r.Value)
 FROM ReportDataDictionary d
 JOIN ReportData r ON r.ReportDataDictionaryIndex=d.ReportDataDictionaryIndex
 JOIN "Time" tm ON tm.TimeIndex=r.TimeIndex
 JOIN EnvironmentPeriods env ON env.EnvironmentPeriodIndex=tm.EnvironmentPeriodIndex
 WHERE d.IsMeter=0 AND d.Name=? AND d.ReportingFrequency='Monthly'
 AND env.EnvironmentType=3 AND tm.IntervalType=3
 AND (tm.WarmupFlag=0 OR tm.WarmupFlag IS NULL)
 GROUP BY d.ReportDataDictionaryIndex,d.KeyValue,d.Name,d.Units`, name)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]epathReplayObservation{}
	for rows.Next() {
		var observation epathReplayObservation
		var total sql.NullFloat64
		if err := rows.Scan(&observation.key, &observation.name, &observation.unit, &observation.rows, &observation.values, &observation.months, &total); err != nil {
			t.Fatal(err)
		}
		if !total.Valid || math.IsNaN(total.Float64) || math.IsInf(total.Float64, 0) {
			t.Fatalf("Monthly SQL total is null/nonfinite: %+v", observation)
		}
		observation.total = total.Float64
		key := strings.ToUpper(strings.TrimSpace(observation.key))
		if _, duplicate := out[key]; duplicate || key == "" || observation.name != name || observation.unit != "J" || observation.rows != 12 || observation.values != 12 || observation.months != 12 {
			t.Fatalf("ambiguous/nonreported Monthly SQL observation: %+v", observation)
		}
		out[key] = observation
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func epathReplayResolution(t *testing.T, discovery OutputDiscoveryResult, key, canonicalName string) PurposeOutputResolution {
	t.Helper()
	matches := []PurposeOutputResolution{}
	for _, resolution := range discovery.PurposeOutputResolutions {
		if strings.EqualFold(resolution.ObjectType, "Output:Variable") &&
			strings.EqualFold(resolution.KeyValue, key) && resolution.CanonicalName == canonicalName &&
			resolution.ReportingFrequency == "Monthly" {
			matches = append(matches, resolution)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("exact Monthly resolution %s / %s has %d matches", key, canonicalName, len(matches))
	}
	return matches[0]
}

func epathReplayAssertOriginalSource(t *testing.T, sources []EnergyDataSource, observation epathReplayObservation) {
	t.Helper()
	matches := 0
	for _, source := range sources {
		if source.Name == observation.name && strings.EqualFold(source.KeyValue, observation.key) && source.ReportingFrequency == "Monthly" {
			matches++
			if source.IsMeter || source.Units != observation.unit || source.NormalizedUnit != "kWh" ||
				source.SourceUnit != "" && source.SourceUnit != observation.unit {
				t.Errorf("reported source unit/identity was rewritten: %+v", source)
			}
		}
	}
	if matches != 1 {
		t.Errorf("original reported source %s / %s has %d matches", observation.key, observation.name, matches)
	}
}

func epathReplayAssertSameFile(t *testing.T, actual, expected string) {
	t.Helper()
	left, err := os.Stat(actual)
	if err != nil {
		t.Fatal(err)
	}
	right, err := os.Stat(expected)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(left, right) {
		t.Fatalf("different actual files: %s / %s", actual, expected)
	}
}

func epathReplayReadJSON(t *testing.T, path string, value any) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(value); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("trailing JSON or read error in %s: %v", path, err)
	}
}

type epathReplayFileState struct {
	Size    int64
	ModTime int64
	Hash    string
}

// Include empty logs and all directory entries. A read-only database access
// must not create a journal/WAL/SHM or change any saved discovery/result file.
func epathReplayDirectorySnapshot(t *testing.T, directory string) map[string]epathReplayFileState {
	t.Helper()
	snapshot := map[string]epathReplayFileState{}
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			snapshot[relative+"/"] = epathReplayFileState{ModTime: info.ModTime().UnixNano()}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("nonregular captured artifact: %s", path)
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
			return fmt.Errorf("artifact changed during integrity read: %s / %v", path, err)
		}
		snapshot[relative] = epathReplayFileState{Size: info.Size(), ModTime: info.ModTime().UnixNano(), Hash: hex.EncodeToString(hash.Sum(nil))}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
