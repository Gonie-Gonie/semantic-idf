package simulation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathRealFixture struct {
	ID                   string   `json:"id"`
	Version              string   `json:"version"`
	ModelPath            string   `json:"modelPath"`
	ModelSHA256          string   `json:"modelSHA256"`
	EngineSourceFilename string   `json:"engineSourceFilename"`
	SourceURL            string   `json:"sourceURL"`
	LicensePath          string   `json:"licensePath"`
	ExpectedPath         string   `json:"expectedPath"`
	OraclePath           string   `json:"oraclePath,omitempty"`
	Types                []string `json:"types"`
	Weather              struct {
		Filename string `json:"filename"`
		SHA256   string `json:"sha256"`
	} `json:"weather"`
}

type epathRealCatalog struct {
	Schema   string             `json:"schema"`
	Fixtures []epathRealFixture `json:"fixtures"`
}

type epathRealRunEvidence struct {
	NumericalTrial    *epathRealNumericalTrial `json:"numericalTrial,omitempty"`
	Fixture           epathRealFixture         `json:"fixture"`
	Version           string                   `json:"version"`
	CatalogDirectory  string                   `json:"catalogDirectory"`
	RunDirectory      string                   `json:"runDirectory"`
	SQLPath           string                   `json:"sqlPath"`
	SQLSHA256         string                   `json:"sqlSHA256"`
	ManifestSHA256    string                   `json:"manifestSHA256"`
	ResultPath        string                   `json:"resultPath,omitempty"`
	ResultSHA256      string                   `json:"resultSHA256,omitempty"`
	OriginalInputPath string                   `json:"originalInputPath"`
	AnnualInputPath   string                   `json:"annualInputPath"`
	InputPath         string                   `json:"inputPath"`
	ModelSHA256       string                   `json:"modelSHA256"`
	AnnualSHA256      string                   `json:"annualSHA256"`
	ExecutedSHA256    string                   `json:"executedSHA256"`
	EngineSHA256      string                   `json:"engineSHA256"`
	EngineFilesSHA256 map[string]string        `json:"engineFilesSHA256"`
	WeatherSHA256     string                   `json:"weatherSHA256"`
	Run               *SimulationRunResult     `json:"run"`
	Bundle            PurposeResultBundle      `json:"bundle"`
	Discovery         OutputDiscoveryResult    `json:"discovery"`
}

// Capture never blesses checked-in expectations. A passing capture means only
// that the explicitly requested collection completed, not fixture acceptance.
func TestEnergyPathRealModelEvidence(t *testing.T) {
	if os.Getenv("EPATH_REAL_CAPTURE") != "1" {
		t.Skip("real EnergyPlus collection requires EPATH_REAL_CAPTURE=1; not acceptance")
	}
	if os.Getenv("EPATH_REAL_RUN") == "1" {
		t.Fatal("choose capture or acceptance explicitly, not both")
	}
	epathRunRealCatalog(t, false)
}

func TestEnergyPathRealModelAcceptance(t *testing.T) {
	if os.Getenv("EPATH_REAL_RUN") != "1" {
		t.Skip("real EnergyPlus acceptance requires EPATH_REAL_RUN=1")
	}
	if os.Getenv("EPATH_REAL_CAPTURE") == "1" {
		t.Fatal("choose capture or acceptance explicitly, not both")
	}
	epathRunRealCatalog(t, true)
}

// Re-check an already completed, hash-bound real run without invoking an
// engine, modifying artifacts, or writing/blessing expected values. Rebuilding
// the shared canonical payload is explicit because recipe-only iteration can
// use the preserved GUI payload directly.
func TestEnergyPathRealModelSavedEvidence(t *testing.T) {
	directory := strings.TrimSpace(os.Getenv("EPATH_REAL_VERIFY_DIR"))
	if directory == "" {
		t.Skip("saved-run verification requires EPATH_REAL_VERIFY_DIR")
	}
	if os.Getenv("EPATH_REAL_CAPTURE") == "1" || os.Getenv("EPATH_REAL_RUN") == "1" {
		t.Fatal("saved-run verification cannot be combined with an engine-run mode")
	}
	root, catalogDirectory := epathRealDirectories(t)
	directory, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	var evidence epathRealRunEvidence
	if err := json.Unmarshal(epathRequireRealFile(t, filepath.Join(directory, "run-evidence.json")), &evidence); err != nil {
		t.Fatal(err)
	}
	if !epathRealSamePath(evidence.RunDirectory, directory) {
		t.Fatal("saved evidence directory identity mismatch")
	}
	catalog := epathLoadRealCatalog(t, catalogDirectory)
	selected := epathSelectRealFixtures(t, catalog.Fixtures)
	var fixture *epathRealFixture
	for index := range selected {
		if selected[index].ID == evidence.Fixture.ID {
			fixture = &selected[index]
		}
	}
	if fixture == nil || !reflect.DeepEqual(*fixture, evidence.Fixture) || evidence.Version != fixture.Version {
		t.Fatal("saved evidence does not match the current exact selected catalog fixture")
	}
	if err := epathValidateSavedRealEvidence(root, catalogDirectory, evidence); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("EPATH_REAL_VERIFY_REBUILD") == "1" {
		projection, err := LoadEnergyPathProjection(EnergyPathProjectionRequest{ResultPath: evidence.SQLPath, InputPath: evidence.InputPath, Scope: "building", Period: "annual", Service: "all"})
		if err != nil {
			t.Fatal(err)
		}
		evidence.Bundle = projection.PurposeResults
		t.Log("Rebuilt with the shared saved-run loader; original captured payload/files were not changed")
	}
	observed := epathCollectRealSQLOracle(t, evidence)
	if os.Getenv("EPATH_REAL_VERIFY_ACCEPTANCE") == "1" {
		if evidence.NumericalTrial != nil {
			t.Fatal("numerical trial has no approved fixture/expected provenance; acceptance is not authorized")
		}
		manifest := epathLoadRealExpectedManifest(t, epathRealCatalogPath(t, catalogDirectory, fixture.ExpectedPath))
		epathAssertRealSQLOracle(t, evidence, manifest)
	} else {
		t.Logf("READ-ONLY CANDIDATE CHECK (not acceptance): status=%s checkedGroups=%v", observed.Status, observed.CheckedGroups)
	}
	if err := epathValidateSavedRealEvidence(root, catalogDirectory, evidence); err != nil {
		t.Fatal(err)
	}
}

func epathRealSamePath(left, right string) bool {
	left, leftErr := filepath.Abs(left)
	right, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func epathValidateSavedRealEvidence(root, catalogDirectory string, evidence epathRealRunEvidence) error {
	run := evidence.Run
	if run == nil || run.Status != "succeeded" || run.ExitCode != 0 || run.ERR.Severe != 0 || run.ERR.Fatal != 0 || evidence.Bundle.EnergyExplanation.Schema != energyExplanationSchema {
		return fmt.Errorf("saved evidence is not a completed successful GUI run")
	}
	parent := filepath.Join(root, ".runtime", "energy-path-acceptance", evidence.Version, evidence.Fixture.ID)
	if !epathRealSamePath(filepath.Dir(evidence.RunDirectory), parent) || !epathRealSamePath(run.OutputDirectory, evidence.RunDirectory) || filepath.Base(evidence.RunDirectory) != run.RunID {
		return fmt.Errorf("saved run must have its exact fixture-owned .runtime directory")
	}
	if !epathRealSamePath(evidence.OriginalInputPath, filepath.Join(catalogDirectory, filepath.FromSlash(evidence.Fixture.ModelPath))) || !epathRealSamePath(evidence.AnnualInputPath, filepath.Join(evidence.RunDirectory, "annual-model.idf")) || !epathRealSamePath(evidence.InputPath, filepath.Join(evidence.RunDirectory, "executed-model.idf")) || !epathRealSamePath(evidence.InputPath, run.InputPath) || !epathRealSamePath(filepath.Dir(evidence.SQLPath), evidence.RunDirectory) {
		return fmt.Errorf("saved model/SQL path bindings disagree")
	}
	if !strings.HasPrefix(run.EnergyPlusVersion, evidence.Version+".0") || evidence.ModelSHA256 != evidence.Fixture.ModelSHA256 || evidence.WeatherSHA256 != evidence.Fixture.Weather.SHA256 {
		return fmt.Errorf("saved fixture/engine/weather identity mismatch")
	}
	manifestPath := filepath.Join(evidence.RunDirectory, "semantic-idf-run.json")
	for path, want := range map[string]string{evidence.SQLPath: evidence.SQLSHA256, manifestPath: evidence.ManifestSHA256, evidence.OriginalInputPath: evidence.ModelSHA256, evidence.AnnualInputPath: evidence.AnnualSHA256, evidence.InputPath: evidence.ExecutedSHA256, run.WeatherPath: evidence.WeatherSHA256} {
		if len(want) != 64 {
			return fmt.Errorf("missing captured integrity digest for %s", path)
		}
		got, err := epathReadRealFileHash(path)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("captured evidence hash changed: %s", path)
		}
	}
	if evidence.ResultSHA256 != "" {
		if !epathRealSamePath(evidence.ResultPath, filepath.Join(evidence.RunDirectory, "simulation-result.json")) {
			return fmt.Errorf("full result snapshot path mismatch")
		}
		got, err := epathReadRealFileHash(evidence.ResultPath)
		if err != nil || got != evidence.ResultSHA256 {
			return fmt.Errorf("full result snapshot changed: %v", err)
		}
	} else if run.PurposeResults == nil {
		return fmt.Errorf("saved evidence requires a full result snapshot or the original inline result")
	}
	for _, name := range []string{"energyplus.exe", "Energy+.idd", "energyplusapi.dll"} {
		want := evidence.EngineFilesSHA256[name]
		got, err := epathReadRealFileHash(filepath.Join(filepath.Dir(run.EnergyPlusExecutablePath), name))
		if err != nil || len(want) != 64 || got != want {
			return fmt.Errorf("captured engine file changed or is unavailable: %s / %v", name, err)
		}
	}
	bytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest SimulationRunManifest
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return err
	}
	if manifest.RunID != run.RunID || manifest.Status != "succeeded" || manifest.InputHash != evidence.ExecutedSHA256 || !epathRealSamePath(manifest.InputPath, evidence.InputPath) || !epathRealSamePath(manifest.OutputDirectory, evidence.RunDirectory) || manifest.OutputPlan == nil || !reflect.DeepEqual(manifest.OutputPlan, run.PurposeRunPlan) {
		return fmt.Errorf("normal run manifest does not bind the saved input/plan/result")
	}
	exactSQL := 0
	for _, file := range manifest.ResultFiles {
		if epathRealSamePath(file.Path, evidence.SQLPath) && file.Kind == "sqlite" {
			exactSQL++
		}
	}
	if exactSQL != 1 {
		return fmt.Errorf("normal run manifest does not uniquely bind the selected SQL")
	}
	var request SimulationRunRequest
	bytes, err = os.ReadFile(filepath.Join(evidence.RunDirectory, "capture-request.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bytes, &request); err != nil {
		return err
	}
	if request.RunID != run.RunID || request.PurposeRequest == nil || !epathRealSamePath(request.InputPath, evidence.OriginalInputPath) || epathRealHash([]byte(request.Text)) != evidence.ExecutedSHA256 || !reflect.DeepEqual(request.PurposeRunPlan, manifest.OutputPlan) {
		return fmt.Errorf("captured request differs from the actual executed input/plan")
	}
	return nil
}

func TestEnergyPathRealModelCatalog(t *testing.T) {
	_, directory := epathRealDirectories(t)
	catalog := epathLoadRealCatalog(t, directory)
	types := map[string]bool{}
	for _, fixture := range catalog.Fixtures {
		data := epathRequireRealFile(t, epathRealCatalogPath(t, directory, fixture.ModelPath))
		if !strings.EqualFold(epathRealHash(data), fixture.ModelSHA256) {
			t.Fatalf("catalog original hash mismatch: %s", fixture.ID)
		}
		model, err := epinput.Parse(fixture.ModelPath, data)
		if err != nil {
			t.Fatal(err)
		}
		if model.Version.Raw != fixture.Version && model.Version.Raw != fixture.Version+".0" {
			t.Fatalf("catalog model version mismatch: %s", fixture.ID)
		}
		epathRequireRealFile(t, epathRealCatalogPath(t, directory, fixture.LicensePath))
		epathRealCatalogPath(t, directory, fixture.ExpectedPath)
		if fixture.OraclePath != "" {
			epathRealCatalogPath(t, directory, fixture.OraclePath)
		}
		if fixture.SourceURL == "" || fixture.EngineSourceFilename == "" || filepath.Base(fixture.Weather.Filename) != fixture.Weather.Filename || len(fixture.Weather.SHA256) != 64 {
			t.Fatalf("missing original/weather provenance: %s", fixture.ID)
		}
		for _, kind := range fixture.Types {
			types[kind] = true
		}
	}
	for _, kind := range []string{"large_office", "small_office", "ideal_loads", "ptac", "pthp", "fan_coil", "vrf", "radiant", "district_energy", "mixed_heating_fuels", "zone_group", "zone_multiplier", "pv_storage", "output_alias_discovery", "no_cooling", "no_heating", "simultaneous_heating_cooling"} {
		if !types[kind] {
			t.Errorf("catalog lacks required logical fixture type %q (catalog coverage is not runtime acceptance)", kind)
		}
	}
}

func TestEnergyPathRealModelAnnualPreparation(t *testing.T) {
	doc, err := idf.Parse("Version,25.1; SimulationControl,Yes,No,Yes,Yes,No,Yes,2; RunPeriod,Winter,1,14,,1,14,,Tuesday; RunPeriod,Summer,7,7,,7,7,,Tuesday; Timestep,4;")
	if err != nil {
		t.Fatal(err)
	}
	before := doc.String()
	annual, changes := epathRealAnnualDocument(t, doc)
	if before != doc.String() || len(changes) != 4 {
		t.Fatal("annual preparation mutated original or lost exact changes")
	}
	periods, controls := 0, 0
	for _, object := range annual.Objects {
		if strings.EqualFold(object.Type, "RunPeriod") {
			periods++
			for index, value := range []string{"Energy Path annual acceptance", "1", "1", "2017", "12", "31", "2017", "Sunday", "No", "No", "No", "Yes", "Yes", "No", "Hour24"} {
				if object.Fields[index].Value != value {
					t.Fatalf("annual field%d=%q want%q", index, object.Fields[index].Value, value)
				}
			}
		}
		if strings.EqualFold(object.Type, "SimulationControl") {
			controls++
			for index, value := range []string{"Yes", "No", "Yes", "No", "Yes", "Yes", "2"} {
				if object.Fields[index].Value != value {
					t.Fatalf("sizing/weather policy changed incorrectly: %#v", object)
				}
			}
		}
	}
	if periods != 1 || controls != 1 {
		t.Fatal("annual preparation did not create one calendar/environment")
	}
}

func epathRunRealCatalog(t *testing.T, acceptance bool) {
	t.Helper()
	root, catalogDirectory := epathRealDirectories(t)
	catalog := epathLoadRealCatalog(t, catalogDirectory)
	fixtures := epathSelectRealFixtures(t, catalog.Fixtures)
	for _, fixture := range fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			if acceptance {
				epathRequireRealFile(t, epathRealCatalogPath(t, catalogDirectory, fixture.OraclePath))
				epathRequireRealFile(t, epathRealCatalogPath(t, catalogDirectory, fixture.ExpectedPath))
			}
			evidence := epathRunRealFixture(t, root, catalogDirectory, fixture)
			// Keep the canonical bundle once in test evidence. The separate hashed
			// normal result remains the authoritative complete GUI payload.
			recorded := evidence
			recordedRun := *evidence.Run
			recordedRun.PurposeResults, recordedRun.Series = nil, nil
			recordedRun.HeatFlow = HeatFlowDataset{}
			recorded.Run = &recordedRun
			epathWriteRealJSON(t, filepath.Join(evidence.RunDirectory, "run-evidence.json"), recorded)
			observed := epathCollectRealSQLOracle(t, evidence)
			epathWriteRealJSON(t, filepath.Join(evidence.RunDirectory, "oracle-observations.json"), observed)
			if !acceptance {
				t.Logf("CAPTURE ONLY (not acceptance): %s; candidate observations remain under .runtime", evidence.RunDirectory)
				return
			}
			manifest := epathLoadRealExpectedManifest(t, epathRealCatalogPath(t, catalogDirectory, fixture.ExpectedPath))
			epathAssertRealSQLOracle(t, evidence, manifest)
		})
	}
}

func epathRealDirectories(t *testing.T) (string, string) {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve acceptance source path")
	}
	packageDirectory := filepath.Dir(source)
	for root := packageDirectory; ; root = filepath.Dir(root) {
		if info, err := os.Stat(filepath.Join(root, "go.mod")); err == nil && info.Mode().IsRegular() {
			return root, filepath.Join(packageDirectory, "testdata", "energy_path_real_models")
		}
		if filepath.Dir(root) == root {
			t.Fatal("repository go.mod not found")
		}
	}
}

func epathLoadRealCatalog(t *testing.T, directory string) epathRealCatalog {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog epathRealCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.Schema != "semantic-idf.energy-path-real-model-catalog/v1" || len(catalog.Fixtures) == 0 {
		t.Fatal("missing or unsupported real-model catalog")
	}
	seen := map[string]bool{}
	for _, fixture := range catalog.Fixtures {
		if fixture.ID == "" || seen[fixture.ID] || strings.ContainsAny(fixture.ID, `/\\:`) || fixture.Version == "" {
			t.Fatalf("invalid/duplicate fixture identity: %#v", fixture)
		}
		seen[fixture.ID] = true
	}
	return catalog
}

func epathSelectRealFixtures(t *testing.T, fixtures []epathRealFixture) []epathRealFixture {
	t.Helper()
	filter := func(name string, allowed map[string]bool) map[string]bool {
		raw, exists := os.LookupEnv(name)
		if !exists {
			return nil
		}
		selected := map[string]bool{}
		for _, token := range strings.Split(raw, ",") {
			token = strings.TrimSpace(token)
			if token == "" || !allowed[token] {
				t.Fatalf("%s contains unknown/empty selector %q", name, token)
			}
			selected[token] = true
		}
		return selected
	}
	ids, versions := map[string]bool{}, map[string]bool{}
	for _, fixture := range fixtures {
		ids[fixture.ID], versions[fixture.Version] = true, true
	}
	wantedIDs, wantedVersions := filter("EPATH_REAL_FIXTURES", ids), filter("EPATH_REAL_VERSIONS", versions)
	var selected []epathRealFixture
	for _, fixture := range fixtures {
		if (wantedIDs == nil || wantedIDs[fixture.ID]) && (wantedVersions == nil || wantedVersions[fixture.Version]) {
			selected = append(selected, fixture)
		}
	}
	if len(selected) == 0 {
		t.Fatal("explicit fixture/version filters matched no real models")
	}
	return selected
}

func epathRunRealFixture(t *testing.T, root, catalogDirectory string, fixture epathRealFixture, trials ...epathRealNumericalTrial) epathRealRunEvidence {
	t.Helper()
	var trial *epathRealNumericalTrial
	if len(trials) > 1 {
		t.Fatal("only one explicit numerical trial is supported")
	}
	if len(trials) == 1 {
		trial = &trials[0]
		if fixture.ID != "no-heating-25-1" || fixture.Version != "25.1" || trial.Algorithm != "RegulaFalsiThenBisection" || trial.SwitchAfter != 5 {
			t.Fatal("numerical trial is restricted to the reviewed noHeating hybrid-root experiment")
		}
	}
	inputPath := epathRealCatalogPath(t, catalogDirectory, fixture.ModelPath)
	original := epathRequireRealFile(t, inputPath)
	if !strings.EqualFold(epathRealHash(original), fixture.ModelSHA256) {
		t.Fatalf("original model hash mismatch: %s", inputPath)
	}
	model, err := epinput.Parse(inputPath, original)
	if err != nil {
		t.Fatal(err)
	}
	if model.Version.Raw != fixture.Version && model.Version.Raw != fixture.Version+".0" {
		t.Fatalf("fixture %s input version %q does not match %q", fixture.ID, model.Version.Raw, fixture.Version)
	}
	doc := epinput.ToIDFDocument(model)
	annual, changes := epathRealAnnualDocument(t, doc)
	if trial != nil {
		if epathRealHash([]byte(annual.String())) != trial.BaselineSHA256["annual-model.idf"] {
			t.Fatal("annual controls/physical model differ from the failed baseline before numerical trial")
		}
		var change map[string]any
		annual, change = epathNoHeatingHybridTrialDocument(t, annual)
		changes = append(changes, change)
	}
	annualText := annual.String()
	executable := strings.TrimSpace(os.Getenv("EPATH_REAL_ENGINE_" + strings.ReplaceAll(fixture.Version, ".", "_")))
	if executable == "" {
		executable = filepath.Join("C:/EnergyPlusV"+strings.ReplaceAll(fixture.Version, ".", "-")+"-0", "energyplus.exe")
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		t.Fatal(err)
	}
	engineHash := epathRealHash(epathRequireRealFile(t, executable))
	versionContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	versionOutput, versionErr := exec.CommandContext(versionContext, executable, "--version").CombinedOutput()
	cancel()
	if versionErr != nil || !strings.Contains(string(versionOutput), "Version "+fixture.Version+".0") {
		t.Fatalf("selected engine does not prove fixture version %s: %s / %v", fixture.Version, versionOutput, versionErr)
	}
	engineFiles := map[string]string{"energyplus.exe": engineHash}
	for _, name := range []string{"Energy+.idd", "energyplusapi.dll"} {
		engineFiles[name] = epathRealHash(epathRequireRealFile(t, filepath.Join(filepath.Dir(executable), name)))
	}
	weatherPath := strings.TrimSpace(os.Getenv("EPATH_REAL_WEATHER"))
	if weatherPath == "" {
		weatherPath = filepath.Join(filepath.Dir(executable), "WeatherData", fixture.Weather.Filename)
	}
	weatherPath, err = filepath.Abs(weatherPath)
	if err != nil {
		t.Fatal(err)
	}
	weatherHash := epathRealHash(epathRequireRealFile(t, weatherPath))
	if !strings.EqualFold(weatherHash, fixture.Weather.SHA256) {
		t.Fatalf("weather hash mismatch: %s", weatherPath)
	}
	if trial != nil && (strings.TrimSpace(string(versionOutput)) != strings.TrimSpace(trial.BaselineEngineVersion) || !reflect.DeepEqual(engineFiles, trial.BaselineEngineSHA256) || !epathRealSamePath(weatherPath, trial.BaselineWeatherPath) || weatherHash != trial.BaselineWeatherSHA256) {
		t.Fatal("numerical trial engine/version/weather differs from the documented failed baseline")
	}
	runID := "real-" + fixture.ID + "-" + time.Now().UTC().Format("20060102T150405.000000000")
	if trial != nil {
		runID = "numerical-trial-" + fixture.ID + "-hybrid-5-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	runDirectory := filepath.Join(root, ".runtime", "energy-path-acceptance", fixture.Version, fixture.ID, runID)
	if err := os.MkdirAll(filepath.Dir(runDirectory), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(runDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	annualPath := filepath.Join(runDirectory, "annual-model.idf")
	if err := os.WriteFile(annualPath, []byte(annualText), 0600); err != nil {
		t.Fatal(err)
	}
	epathWriteRealJSON(t, filepath.Join(runDirectory, "annualization.json"), changes)
	purpose := NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare, Scope: SimulationPurposeScope{ZoneMode: "all", PeriodMode: "full"}})
	plan := BuildPurposeRunPlan(annual, purpose)
	updated, preview := idf.ApplyOutput(annual, PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd))
	if !preview.CanApply {
		t.Fatalf("purpose output plan blocked: %#v", preview)
	}
	if trial != nil {
		withoutTrial := updated
		withoutTrial.Objects = nil
		for _, object := range updated.Objects {
			if !strings.EqualFold(object.Type, "HVACSystemRootFindingAlgorithm") {
				withoutTrial.Objects = append(withoutTrial.Objects, object)
			}
		}
		if len(updated.Objects) != len(withoutTrial.Objects)+1 || epathRealHash([]byte(withoutTrial.String())) != trial.BaselineSHA256["executed-model.idf"] {
			t.Fatal("executed trial differs from baseline by more than the one approved numerical object")
		}
	}
	epathWriteRealJSON(t, filepath.Join(runDirectory, "output-application.json"), preview)
	request := SimulationRunRequest{RunID: runID, InputPath: inputPath, Filename: "executed-model.idf", Text: updated.String(), EnergyPlusExecutablePath: executable, WeatherPath: weatherPath, OutputDirectory: runDirectory, PurposeRequest: &purpose, PurposeRunPlan: &plan, TemporaryOutputDiff: PurposeRunPlanTemporaryOutputDiff(plan), ResultMode: "sql_first", UseReadVarsESO: false, Silent: true}
	epathWriteRealJSON(t, filepath.Join(runDirectory, "capture-request.json"), request)
	provenance := map[string]any{"originalInputPath": inputPath, "originalSHA256": epathRealHash(original), "annualInputPath": annualPath, "annualSHA256": epathRealHash([]byte(annualText)), "executedInputPath": filepath.Join(runDirectory, request.Filename), "executedSHA256": epathRealHash([]byte(request.Text)), "weatherPath": weatherPath, "weatherSHA256": weatherHash, "engineVersionOutput": string(versionOutput), "engineFilesSHA256": engineFiles}
	if trial != nil {
		provenance["numericalTrial"] = trial
	}
	epathWriteRealJSON(t, filepath.Join(runDirectory, "provenance.json"), provenance)
	t.Logf("REAL RUN starting %s; artifacts=%s", fixture.ID, runDirectory)
	run, err := RunSimulation(request, func(progress SimulationProgress) { t.Logf("%s: %s %s", fixture.ID, progress.Phase, progress.Status) }, SimulationSettings{RunDirectory: filepath.Dir(runDirectory), EnergyPlusInstallations: []EnergyPlusInstallSetting{{Version: fixture.Version + ".0", ExecutablePath: executable, RootPath: filepath.Dir(executable)}}})
	if err != nil {
		t.Fatal(err)
	}
	if run == nil {
		t.Fatal("EnergyPlus returned no result")
	}
	epathWriteRealJSON(t, filepath.Join(runDirectory, "simulation-result.json"), run)
	if run.Status != "succeeded" || run.ExitCode != 0 || run.ERR.Fatal != 0 || run.ERR.Severe != 0 {
		t.Fatalf("EnergyPlus failed: status=%s exit=%d severe=%d fatal=%d error=%s; inspect %s", run.Status, run.ExitCode, run.ERR.Severe, run.ERR.Fatal, run.Error, runDirectory)
	}
	var sqlPath string
	for _, file := range run.Files {
		if strings.EqualFold(filepath.Ext(file.Path), ".sql") {
			if sqlPath != "" {
				t.Fatal("multiple SQL files in actual run")
			}
			sqlPath = file.Path
		}
	}
	if sqlPath == "" || run.PurposeResults == nil {
		t.Fatal("successful engine run lacks SQL or actual purpose bundle")
	}
	if info, err := os.Stat(sqlPath); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		t.Fatalf("actual SQL must be a nonempty regular file: %s / %v", sqlPath, err)
	}
	manifestHash := epathRealFileHash(t, filepath.Join(runDirectory, "semantic-idf-run.json"))
	sqlHash := epathRealFileHash(t, sqlPath)
	resultPath := filepath.Join(runDirectory, "simulation-result.json")
	resultHash := epathRealFileHash(t, resultPath)
	discovery, err := DiscoverAvailableOutputs(OutputDiscoveryRequest{Text: request.Text, OutputDirectory: runDirectory, SQLPath: sqlPath, PurposeRequest: &purpose})
	if err != nil {
		t.Fatal(err)
	}
	if epathRealHash(epathRequireRealFile(t, inputPath)) != epathRealHash(original) {
		t.Fatal("original fixture mutated by run")
	}
	return epathRealRunEvidence{NumericalTrial: trial, Fixture: fixture, Version: fixture.Version, CatalogDirectory: catalogDirectory, RunDirectory: runDirectory, SQLPath: sqlPath, SQLSHA256: sqlHash, ManifestSHA256: manifestHash, ResultPath: resultPath, ResultSHA256: resultHash, OriginalInputPath: inputPath, AnnualInputPath: annualPath, InputPath: run.InputPath, ModelSHA256: epathRealHash(original), AnnualSHA256: epathRealHash([]byte(annualText)), ExecutedSHA256: epathRealHash(epathRequireRealFile(t, run.InputPath)), EngineSHA256: engineHash, EngineFilesSHA256: engineFiles, WeatherSHA256: weatherHash, Run: run, Bundle: *run.PurposeResults, Discovery: discovery}
}

func epathRealAnnualDocument(t *testing.T, input idf.Document) (idf.Document, []map[string]any) {
	t.Helper()
	out := idf.Document{}
	var changes []map[string]any
	controlCount := 0
	var period *idf.Object
	for _, original := range input.Objects {
		object := original
		object.Fields = append([]idf.Field(nil), original.Fields...)
		switch strings.ToLower(strings.TrimSpace(object.Type)) {
		case "runperiod":
			if period == nil {
				copy := object
				period = &copy
			}
			changes = append(changes, map[string]any{"action": "remove", "before": original})
			continue
		case "simulationcontrol":
			controlCount++
			for len(object.Fields) < 5 {
				object.Fields = append(object.Fields, idf.Field{Value: "No"})
			}
			object.Fields[3].Value, object.Fields[4].Value = "No", "Yes"
			changes = append(changes, map[string]any{"action": "update", "before": original, "after": object})
		}
		out.Objects = append(out.Objects, object)
	}
	if controlCount > 1 {
		t.Fatal("multiple SimulationControl objects cannot define one annual run")
	}
	if controlCount == 0 {
		out.Objects = append(out.Objects, idf.Object{Type: "SimulationControl", Fields: []idf.Field{{Value: "No"}, {Value: "No"}, {Value: "No"}, {Value: "No"}, {Value: "Yes"}}})
	}
	if period == nil {
		period = &idf.Object{Type: "RunPeriod"}
	}
	for len(period.Fields) < 15 {
		period.Fields = append(period.Fields, idf.Field{})
	}
	// Fixed non-leap calendar, independent of the machine date. Jan 1, 2017 is
	// Sunday; weather holidays/DST are disabled, rain/snow remain weather-backed.
	for index, value := range []string{"Energy Path annual acceptance", "1", "1", "2017", "12", "31", "2017", "Sunday", "No", "No", "No", "Yes", "Yes", "No", "Hour24"} {
		period.Fields[index].Value = value
	}
	changes = append(changes, map[string]any{"action": "add", "after": *period, "rationale": "Fixed non-leap 2017 weather year; Sunday Jan 1; no weather holidays/DST; no design-day reporting."})
	out.Objects = append(out.Objects, *period)
	for index := range out.Objects {
		out.Objects[index].Index = index
	}
	return out, changes
}

func epathRealCatalogPath(t *testing.T, directory, relative string) string {
	t.Helper()
	if relative == "" || filepath.IsAbs(relative) {
		t.Fatalf("catalog path must be nonempty and relative: %q", relative)
	}
	path := filepath.Join(directory, filepath.FromSlash(relative))
	within, err := filepath.Rel(directory, path)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		t.Fatalf("catalog path escapes fixture directory: %q", relative)
	}
	return path
}

func epathRequireRealFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("explicit real fixture requires file %s: %v", path, err)
	}
	if len(data) == 0 {
		t.Fatalf("explicit real fixture file is empty: %s", path)
	}
	return data
}

func epathRealHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// Annual SQL can be large. Integrity hashing streams the actual closed file,
// and rejects replacement/modification during the read instead of allocating
// the complete database in memory.
func epathRealFileHash(t *testing.T, path string) string {
	t.Helper()
	hash, err := epathReadRealFileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func epathReadRealFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() == 0 {
		return "", fmt.Errorf("nonempty regular evidence file required: %s / %v", path, err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", fmt.Errorf("evidence file changed while hashing: %s", path)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func epathWriteRealJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(fmt.Errorf("write runtime evidence: %w", err))
	}
}
