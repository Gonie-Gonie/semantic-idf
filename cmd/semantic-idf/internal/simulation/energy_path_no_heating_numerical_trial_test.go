package simulation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathRealNumericalTrial struct {
	Label                 string            `json:"label"`
	Algorithm             string            `json:"algorithm"`
	SwitchAfter           int               `json:"switchAfter"`
	BaselineDirectory     string            `json:"baselineDirectory"`
	BaselineSHA256        map[string]string `json:"baselineSHA256"`
	BaselineEngineVersion string            `json:"baselineEngineVersion"`
	BaselineEngineSHA256  map[string]string `json:"baselineEngineSHA256"`
	BaselineWeatherPath   string            `json:"baselineWeatherPath"`
	BaselineWeatherSHA256 string            `json:"baselineWeatherSHA256"`
}

// This is an explicitly requested numerical experiment, not a fixture default,
// engine setting, automatic recovery, or acceptance of changed expected values.
func TestEnergyPathNoHeatingHybridSolverTrial(t *testing.T) {
	if os.Getenv("EPATH_REAL_NUMERICAL_TRIAL") != "1" {
		t.Skip("isolated numerical trial requires EPATH_REAL_NUMERICAL_TRIAL=1")
	}
	if os.Getenv("EPATH_REAL_CAPTURE") == "1" || os.Getenv("EPATH_REAL_RUN") == "1" {
		t.Fatal("numerical trial must be separate from normal capture/acceptance")
	}
	root, catalogDirectory := epathRealDirectories(t)
	catalog := epathLoadRealCatalog(t, catalogDirectory)
	var fixture epathRealFixture
	for _, item := range catalog.Fixtures {
		if item.ID == "no-heating-25-1" {
			fixture = item
		}
	}
	if fixture.ID == "" {
		t.Fatal("exact official noHeating fixture is missing")
	}
	baseline := strings.TrimSpace(os.Getenv("EPATH_REAL_BASELINE_DIR"))
	if baseline == "" {
		t.Fatal("explicit failed baseline directory required")
	}
	baseline, err := filepath.Abs(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if !epathRealSamePath(filepath.Dir(baseline), filepath.Join(root, ".runtime", "energy-path-acceptance", "25.1", fixture.ID)) || !strings.HasPrefix(filepath.Base(baseline), "real-"+fixture.ID+"-") {
		t.Fatal("baseline is not the exact fixture-owned failed runtime capture")
	}
	var previous struct {
		Status string     `json:"status"`
		ERR    ERRSummary `json:"err"`
	}
	if err := json.Unmarshal(epathRequireRealFile(t, filepath.Join(baseline, "simulation-result.json")), &previous); err != nil {
		t.Fatal(err)
	}
	if previous.Status != "failed" || previous.ERR.Severe != 2 || previous.ERR.Fatal != 0 {
		t.Fatal("baseline does not match the documented two-Severe/no-Fatal failure")
	}
	expectedSevere := map[string]bool{"WEST DATA CENTER IEC": false, "EAST DATA CENTER IEC": false}
	severeCount := 0
	for _, issue := range previous.ERR.Issues {
		if issue.Severity != "severe" {
			continue
		}
		severeCount++
		// parseERRFile retains the severity token from the original ERR line.
		message := strings.TrimSpace(strings.TrimPrefix(issue.Message, "Severe"))
		matched := false
		for name, seen := range expectedSevere {
			if message == "CalcIndirectResearchSpecialEvapCooler: calculate secondary air mass flow failed for Indirect Evaporative Cooler Research Special = "+name && !seen {
				expectedSevere[name] = true
				matched = true
				break
			}
		}
		if !matched {
			t.Fatal("baseline has a different or repeated severe error; trial is not applicable")
		}
	}
	if severeCount != 2 || !expectedSevere["WEST DATA CENTER IEC"] || !expectedSevere["EAST DATA CENTER IEC"] {
		t.Fatal("baseline does not contain both exact documented cooler errors")
	}
	hashes := map[string]string{}
	for _, name := range []string{"annual-model.idf", "executed-model.idf", "eplusout.sql", "capture-request.json", "simulation-result.json", "semantic-idf-run.json"} {
		hashes[name] = epathRealFileHash(t, filepath.Join(baseline, name))
	}
	for _, name := range []string{"eplusout.err", "provenance.json", "annualization.json"} {
		path := filepath.Join(baseline, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			t.Fatal(err)
		}
		hashes[name] = epathRealFileHash(t, path)
	}
	defer func() {
		for name, want := range hashes {
			if got, err := epathReadRealFileHash(filepath.Join(baseline, name)); err != nil || got != want {
				t.Errorf("failed baseline changed during trial: %s / %v", name, err)
			}
		}
	}()
	var provenance struct {
		EngineVersionOutput string            `json:"engineVersionOutput"`
		EngineFilesSHA256   map[string]string `json:"engineFilesSHA256"`
		WeatherPath         string            `json:"weatherPath"`
		WeatherSHA256       string            `json:"weatherSHA256"`
	}
	if err := json.Unmarshal(epathRequireRealFile(t, filepath.Join(baseline, "provenance.json")), &provenance); err != nil {
		t.Fatal(err)
	}
	trial := epathRealNumericalTrial{Label: "Isolated noHeating numerical trial; not approved fixture acceptance", Algorithm: "RegulaFalsiThenBisection", SwitchAfter: 5, BaselineDirectory: baseline, BaselineSHA256: hashes, BaselineEngineVersion: provenance.EngineVersionOutput, BaselineEngineSHA256: provenance.EngineFilesSHA256, BaselineWeatherPath: provenance.WeatherPath, BaselineWeatherSHA256: provenance.WeatherSHA256}
	evidence := epathRunRealFixture(t, root, catalogDirectory, fixture, trial)
	// One authoritative normal result is separately hashed; preserve its entire
	// canonical bundle once in the test evidence, not twice.
	recorded := evidence
	recordedRun := *evidence.Run
	recordedRun.PurposeResults, recordedRun.Series = nil, nil
	recordedRun.HeatFlow = HeatFlowDataset{}
	recorded.Run = &recordedRun
	epathWriteRealJSON(t, filepath.Join(evidence.RunDirectory, "run-evidence.json"), recorded)
	observed, err := epathReadRealSQLOracle(evidence.SQLPath)
	if err != nil {
		t.Fatal(err)
	}
	epathWriteRealJSON(t, filepath.Join(evidence.RunDirectory, "oracle-observations.json"), observed)
	if err := epathValidateSavedRealEvidence(root, catalogDirectory, evidence); err != nil {
		t.Fatal(err)
	}
	t.Logf("NUMERICAL TRIAL ONLY: severe=%d fatal=%d year=%d months=%v; artifacts=%s", evidence.Run.ERR.Severe, evidence.Run.ERR.Fatal, observed.Weather.Year, observed.Weather.Months, evidence.RunDirectory)
	t.Logf("originalSHA256=%s annualSHA256=%s executedSHA256=%s SQLSHA256=%s", evidence.ModelSHA256, evidence.AnnualSHA256, evidence.ExecutedSHA256, evidence.SQLSHA256)
}

func epathNoHeatingHybridTrialDocument(t *testing.T, input idf.Document) (idf.Document, map[string]any) {
	t.Helper()
	for _, object := range input.Objects {
		if strings.EqualFold(object.Type, "HVACSystemRootFindingAlgorithm") {
			t.Fatal("trial cannot replace an existing model solver choice")
		}
	}
	out := input
	out.Objects = append([]idf.Object(nil), input.Objects...)
	object := idf.Object{Type: "HVACSystemRootFindingAlgorithm", Index: len(out.Objects), Fields: []idf.Field{{Value: "RegulaFalsiThenBisection"}, {Value: "5"}}}
	out.Objects = append(out.Objects, object)
	return out, map[string]any{"action": "add", "kind": "isolated_numerical_trial", "after": object, "rationale": "The official25.1 indirect evaporative-cooler reported non-convergence with a500-iteration cap and0.01C tolerance; the message does not prove the actual iteration count. Switch false-position to bisection after5 iterations; retain the same equations, iteration cap/tolerance, Timestep4, weather, schedules and equipment. This is not fixture acceptance.", "source": "https://github.com/NatLabRockies/EnergyPlus/blob/v25.1.0/src/EnergyPlus/General.cc#L197"}
}

func TestEnergyPathNoHeatingHybridPreparationIsOneNumericalObjectOnly(t *testing.T) {
	doc, err := idf.Parse("Version,25.1; Timestep,4; RunPeriod,Annual,1,1,2017,12,31,2017,Sunday; Schedule:Constant,AlwaysOn,,1; EvaporativeCooler:Indirect:ResearchSpecial,Cooler,AlwaysOn;")
	if err != nil {
		t.Fatal(err)
	}
	before := doc.String()
	prepared, change := epathNoHeatingHybridTrialDocument(t, doc)
	if doc.String() != before || len(prepared.Objects) != len(doc.Objects)+1 || !reflect.DeepEqual(prepared.Objects[:len(doc.Objects)], doc.Objects) {
		t.Fatal("numerical preparation changed original controls/physical model")
	}
	last := prepared.Objects[len(doc.Objects)]
	if last.Type != "HVACSystemRootFindingAlgorithm" || len(last.Fields) != 2 || last.Fields[0].Value != "RegulaFalsiThenBisection" || last.Fields[1].Value != "5" || change["kind"] != "isolated_numerical_trial" {
		t.Fatal("numerical experiment differs from approved one-object change")
	}
}
