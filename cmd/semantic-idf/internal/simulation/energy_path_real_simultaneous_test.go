package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// These are reviewed native dictionary identities, not production classifiers,
// energy sums, aliases, or hard-coded RDD indices. A ten-minute joint interval
// demonstrates time-aligned operation, not an unobserved internal solver step.
var epathRealSimultaneousSources = [...]struct{ Name, Unit string }{
	{"Chiller Heater System Cooling Mass Flow Rate", "kg/s"},
	{"Chiller Heater System Heating Mass Flow Rate", "kg/s"},
	{"Chiller Heater System Cooling Inlet Temperature", "C"},
	{"Chiller Heater System Cooling Outlet Temperature", "C"},
	{"Chiller Heater System Heating Inlet Temperature", "C"},
	{"Chiller Heater System Heating Outlet Temperature", "C"},
}

type epathRealSimultaneousInterval struct {
	TimeIndex                                                int
	Year, Month, Day, Hour, Minute                           int
	IntervalMinutes                                          float64
	CoolingMassFlow, HeatingMassFlow                         float64
	CoolingInlet, CoolingOutlet, HeatingInlet, HeatingOutlet float64
}

type epathRealSimultaneousObservation struct {
	JointIntervals                         int
	Intervals                              int
	MinIntervalMinutes, MaxIntervalMinutes float64
	First                                  *epathRealSimultaneousInterval
}

func epathReadRealSimultaneousSQL(path string) (epathRealSimultaneousObservation, error) {
	var out epathRealSimultaneousObservation
	db, err := epathOpenOracleSQL(path)
	if err != nil {
		return out, err
	}
	defer db.Close()
	ids := make([]any, len(epathRealSimultaneousSources))
	for index, source := range epathRealSimultaneousSources {
		// Include spelling/whitespace variants in the ambiguity check, but never
		// silently choose one as an alias or substitute another reporting period.
		rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,KeyValue,IsMeter,ReportingFrequency,Units,"Type",TimestepType
FROM ReportDataDictionary WHERE lower(trim(Name))=lower(?) AND lower(trim(KeyValue))='chillerbank'
AND lower(trim(ReportingFrequency))='zone timestep'`, source.Name)
		if err != nil {
			return out, err
		}
		count := 0
		for rows.Next() {
			var id, meter int
			var name, key, frequency, unit, aggregation, timestep string
			if err := rows.Scan(&id, &name, &key, &meter, &frequency, &unit, &aggregation, &timestep); err != nil {
				rows.Close()
				return out, err
			}
			count++
			if name != source.Name || key != "CHILLERBANK" || frequency != "Zone Timestep" || unit != source.Unit || meter != 0 || aggregation != "Avg" || timestep != "HVAC System" {
				rows.Close()
				return out, fmt.Errorf("simultaneous source has contradictory native identity: %s", source.Name)
			}
			ids[index] = id
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		if count != 1 {
			return out, fmt.Errorf("simultaneous source requires one exact identity, found%d: %s", count, source.Name)
		}
	}
	// Reject repeated observations before joining; a duplicate may otherwise
	// multiply joins and turn a single observed interval into several successes.
	var duplicateTime, duplicateSource int
	err = db.QueryRow(`SELECT TimeIndex,ReportDataDictionaryIndex FROM ReportData
WHERE ReportDataDictionaryIndex IN (?,?,?,?,?,?) GROUP BY TimeIndex,ReportDataDictionaryIndex HAVING count(*)<>1 LIMIT 1`, ids...).Scan(&duplicateTime, &duplicateSource)
	if err == nil {
		return out, fmt.Errorf("duplicate simultaneous observation at TimeIndex%d/source%d", duplicateTime, duplicateSource)
	}
	if err != sql.ErrNoRows {
		return out, err
	}
	query := `WITH observed_times AS (SELECT DISTINCT TimeIndex FROM ReportData WHERE ReportDataDictionaryIndex IN (?,?,?,?,?,?))
SELECT t.TimeIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",cm.Value,hm.Value,ci.Value,co.Value,hi.Value,ho.Value
FROM observed_times o JOIN "Time" t ON t.TimeIndex=o.TimeIndex
JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
LEFT JOIN ReportData cm ON cm.TimeIndex=t.TimeIndex AND cm.ReportDataDictionaryIndex=?
LEFT JOIN ReportData hm ON hm.TimeIndex=t.TimeIndex AND hm.ReportDataDictionaryIndex=?
LEFT JOIN ReportData ci ON ci.TimeIndex=t.TimeIndex AND ci.ReportDataDictionaryIndex=?
LEFT JOIN ReportData co ON co.TimeIndex=t.TimeIndex AND co.ReportDataDictionaryIndex=?
LEFT JOIN ReportData hi ON hi.TimeIndex=t.TimeIndex AND hi.ReportDataDictionaryIndex=?
LEFT JOIN ReportData ho ON ho.TimeIndex=t.TimeIndex AND ho.ReportDataDictionaryIndex=?
WHERE e.EnvironmentType=3 AND t.WarmupFlag=0 AND t.IntervalType=-1 ORDER BY t.TimeIndex`
	args := append(append([]any(nil), ids...), ids...)
	rows, err := db.Query(query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var sample epathRealSimultaneousInterval
		var values [6]sql.NullFloat64
		if err := rows.Scan(&sample.TimeIndex, &sample.Year, &sample.Month, &sample.Day, &sample.Hour, &sample.Minute, &sample.IntervalMinutes,
			&values[0], &values[1], &values[2], &values[3], &values[4], &values[5]); err != nil {
			return out, err
		}
		date := time.Date(sample.Year, time.Month(sample.Month), sample.Day, 0, 0, 0, 0, time.UTC)
		if sample.Year != 2017 || int(date.Month()) != sample.Month || date.Day() != sample.Day || sample.Hour < 0 || sample.Hour > 24 || sample.Minute < 0 || sample.Minute > 59 || (sample.Hour == 24 && sample.Minute != 0) || !epathFiniteSimultaneous(sample.IntervalMinutes) || sample.IntervalMinutes <= 0 || sample.IntervalMinutes > 60 {
			return out, fmt.Errorf("invalid simultaneous weather calendar/interval at TimeIndex%d", sample.TimeIndex)
		}
		for index, value := range values {
			if !value.Valid || !epathFiniteSimultaneous(value.Float64) {
				return out, fmt.Errorf("missing/nonfinite same-time observation at TimeIndex%d: %s", sample.TimeIndex, epathRealSimultaneousSources[index].Name)
			}
		}
		sample.CoolingMassFlow, sample.HeatingMassFlow = values[0].Float64, values[1].Float64
		sample.CoolingInlet, sample.CoolingOutlet = values[2].Float64, values[3].Float64
		sample.HeatingInlet, sample.HeatingOutlet = values[4].Float64, values[5].Float64
		if out.JointIntervals == 0 || sample.IntervalMinutes < out.MinIntervalMinutes {
			out.MinIntervalMinutes = sample.IntervalMinutes
		}
		if sample.IntervalMinutes > out.MaxIntervalMinutes {
			out.MaxIntervalMinutes = sample.IntervalMinutes
		}
		out.JointIntervals++
		if sample.CoolingMassFlow > 0 && sample.HeatingMassFlow > 0 && sample.CoolingInlet-sample.CoolingOutlet > 0.01 && sample.HeatingOutlet-sample.HeatingInlet > 0.01 {
			out.Intervals++
			if out.First == nil {
				copy := sample
				out.First = &copy
			}
		}
	}
	return out, rows.Err()
}

func epathFiniteSimultaneous(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func TestEnergyPathRealSimultaneousSavedSQL(t *testing.T) {
	if os.Getenv("EPATH_REAL_SIMULTANEOUS") != "1" {
		t.Skip("saved same-TimeIndex proof requires EPATH_REAL_SIMULTANEOUS=1; never runs EnergyPlus")
	}
	if os.Getenv("EPATH_REAL_CAPTURE") == "1" || os.Getenv("EPATH_REAL_RUN") == "1" || os.Getenv("EPATH_REAL_NUMERICAL_TRIAL") == "1" {
		t.Fatal("saved simultaneous verification must be isolated from engine workflows")
	}
	root, catalogDirectory := epathRealDirectories(t)
	directory := filepath.Join(root, ".runtime", "energy-path-acceptance", "25.1", "simultaneous-25-1", "real-simultaneous-25-1-20260907T163311.967451100")
	sqlPath := epathValidateRealSimultaneousProvenance(t, catalogDirectory, directory)
	epathAssertRealAnnualWeatherSQL(t, sqlPath)
	observed, err := epathReadRealSimultaneousSQL(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	if observed.JointIntervals != 52560 || observed.Intervals != 7624 || observed.First == nil || observed.MinIntervalMinutes != 10 || observed.MaxIntervalMinutes != 10 {
		t.Fatalf("reviewed same-time operation count changed: %+v", observed)
	}
	first := observed.First
	if first.TimeIndex != 13299 || first.Year != 2017 || first.Month != 4 || first.Day != 3 || first.Hour != 8 || first.Minute != 0 || first.IntervalMinutes != 10 {
		t.Fatalf("reviewed first interval changed: %+v", first)
	}
	for label, pair := range map[string][2]float64{
		"cooling mass flow": {first.CoolingMassFlow, 0.780015906240348}, "heating mass flow": {first.HeatingMassFlow, 0.937404375},
		"cooling inlet": {first.CoolingInlet, 14.43397073083102}, "cooling outlet": {first.CoolingOutlet, 6.67},
		"heating inlet": {first.HeatingInlet, 56.73719845414952}, "heating outlet": {first.HeatingOutlet, 60},
	} {
		if math.Abs(pair[0]-pair[1]) > 1e-9 {
			t.Errorf("first interval %s=%g, expected%g", label, pair[0], pair[1])
		}
	}
	t.Logf("SAME-TIME OPERATION PROOF ONLY: %d of%d complete weather intervals, first=%+v; no energy aggregation or eight-group acceptance implied", observed.Intervals, observed.JointIntervals, first)
}

// Validate the original successful normal capture without manufacturing a new
// run-evidence or typed PurposeResultBundle. The original collector could fail
// after the normal engine result and manifest had already been saved.
func epathValidateRealSimultaneousProvenance(t *testing.T, catalogDirectory, directory string) string {
	t.Helper()
	var fixture epathRealFixture
	for _, item := range epathLoadRealCatalog(t, catalogDirectory).Fixtures {
		if item.ID == "simultaneous-25-1" && item.Version == "25.1" {
			fixture = item
		}
	}
	if fixture.ID == "" {
		t.Fatal("exact reviewed simultaneous catalog fixture is missing")
	}
	var provenance struct {
		OriginalInputPath, OriginalSHA256, AnnualInputPath, AnnualSHA256, ExecutedInputPath, ExecutedSHA256 string
		WeatherPath, WeatherSHA256, EngineVersionOutput                                                     string
		EngineFilesSHA256                                                                                   map[string]string
	}
	epathDecodeSimultaneousMetadata(t, filepath.Join(directory, "provenance.json"), &provenance)
	var manifest SimulationRunManifest
	epathDecodeSimultaneousMetadata(t, filepath.Join(directory, "semantic-idf-run.json"), &manifest)
	var request SimulationRunRequest
	epathDecodeSimultaneousMetadata(t, filepath.Join(directory, "capture-request.json"), &request)
	sqlPath := filepath.Join(directory, "eplusout.sql")
	original := epathRealCatalogPath(t, catalogDirectory, fixture.ModelPath)
	if !epathRealSamePath(provenance.OriginalInputPath, original) || provenance.OriginalSHA256 != fixture.ModelSHA256 || !epathRealSamePath(provenance.AnnualInputPath, filepath.Join(directory, "annual-model.idf")) || !epathRealSamePath(provenance.ExecutedInputPath, filepath.Join(directory, "executed-model.idf")) || provenance.WeatherSHA256 != fixture.Weather.SHA256 || filepath.Base(provenance.WeatherPath) != fixture.Weather.Filename || !strings.HasPrefix(provenance.EngineVersionOutput, "EnergyPlus, Version 25.1.0-") {
		t.Fatal("saved simultaneous original/executed/weather/version provenance contradicts catalog")
	}
	if manifest.Status != "succeeded" || manifest.RunID != filepath.Base(directory) || manifest.EnergyPlusVersion != "25.1.0" || !epathRealSamePath(manifest.OutputDirectory, directory) || !epathRealSamePath(manifest.InputPath, provenance.ExecutedInputPath) || manifest.InputHash != provenance.ExecutedSHA256 || !epathRealSamePath(manifest.WeatherPath, provenance.WeatherPath) || manifest.OutputPlan == nil || request.PurposeRequest == nil || request.RunID != manifest.RunID || request.Filename != "executed-model.idf" || !epathRealSamePath(request.InputPath, original) || !epathRealSamePath(request.OutputDirectory, directory) || !epathRealSamePath(request.EnergyPlusExecutablePath, manifest.EnergyPlusExecutablePath) || !epathRealSamePath(request.WeatherPath, provenance.WeatherPath) || epathRealHash([]byte(request.Text)) != provenance.ExecutedSHA256 || !reflect.DeepEqual(request.PurposeRunPlan, manifest.OutputPlan) {
		t.Fatal("normal simultaneous manifest/request does not bind the actual executed input/plan")
	}
	exactSQL := 0
	for _, file := range manifest.ResultFiles {
		if file.Kind == "sqlite" && epathRealSamePath(file.Path, sqlPath) {
			exactSQL++
			info, err := os.Stat(sqlPath)
			if err != nil || !info.Mode().IsRegular() || info.Size() != file.Size {
				t.Fatalf("normal simultaneous SQL file binding changed: %v", err)
			}
		}
	}
	if exactSQL != 1 {
		t.Fatal("normal simultaneous manifest does not uniquely bind exact SQL")
	}
	wants := map[string]string{original: provenance.OriginalSHA256, provenance.AnnualInputPath: provenance.AnnualSHA256, provenance.ExecutedInputPath: provenance.ExecutedSHA256, provenance.WeatherPath: provenance.WeatherSHA256}
	for _, name := range []string{"energyplus.exe", "Energy+.idd", "energyplusapi.dll"} {
		wants[filepath.Join(filepath.Dir(manifest.EnergyPlusExecutablePath), name)] = provenance.EngineFilesSHA256[name]
	}
	for path, want := range wants {
		if len(want) != 64 || epathRealFileHash(t, path) != want {
			t.Fatalf("captured simultaneous source/engine hash mismatch: %s", path)
		}
	}
	for _, name := range []string{"eplusout.sql", "eplusout.err", "semantic-idf-run.json", "capture-request.json", "provenance.json", "annualization.json"} {
		path := filepath.Join(directory, name)
		wants[path] = epathRealFileHash(t, path)
	}
	t.Cleanup(func() {
		for path, want := range wants {
			if got, err := epathReadRealFileHash(path); err != nil || got != want {
				t.Errorf("read-only simultaneous proof changed capture: %s / %v", path, err)
			}
		}
	})
	// Historical digests are checked when the original capture provided them;
	// absent optional evidence is not replaced with invented historical hashes.
	evidencePath := filepath.Join(directory, "run-evidence.json")
	if _, err := os.Stat(evidencePath); err == nil {
		var captured struct{ SQLSHA256, ManifestSHA256 string }
		epathDecodeSimultaneousMetadata(t, evidencePath, &captured)
		if captured.SQLSHA256 != wants[sqlPath] || captured.ManifestSHA256 != wants[filepath.Join(directory, "semantic-idf-run.json")] {
			t.Fatal("original simultaneous SQL/manifest capture digests changed")
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	errSummary := parseERRFile(filepath.Join(directory, "eplusout.err"))
	if errSummary.Severe != 0 || errSummary.Fatal != 0 {
		t.Fatal("simultaneous capture has engine Severe/Fatal errors")
	}
	return sqlPath
}

func epathDecodeSimultaneousMetadata(t *testing.T, path string, target any) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("metadata has trailing JSON: %s / %v", path, err)
	}
}
