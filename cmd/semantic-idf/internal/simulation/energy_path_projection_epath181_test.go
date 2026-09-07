package simulation

import (
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func epath181BackendFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	sqlPath, inputPath := filepath.Join(dir, "보고서 # 100%.sql"), filepath.Join(dir, "original model.idf")
	createEnergyPathContractSQL(t, sqlPath)
	if err := os.WriteFile(inputPath, []byte(energyPathScopeFixtureIDF), 0600); err != nil {
		t.Fatal(err)
	}
	return sqlPath, inputPath
}

func epath181BackendSnapshot(t *testing.T, dir string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[entry.Name()] = sha256.Sum256(data)
	}
	return out
}

func TestEPATH181ReadOnlySQLiteNeverCreatesMutatesOrIgnoresWAL(t *testing.T) {
	sqlPath, _ := epath181BackendFixture(t)
	dir := filepath.Dir(sqlPath)
	before := epath181BackendSnapshot(t, dir)
	missing := filepath.Join(dir, "missing.sql")
	if db, err := openSimulationSQLiteReadOnly(missing); err == nil {
		db.Close()
		t.Fatal("missing SQL opened")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("missing SQL was created")
	}
	db, err := openSimulationSQLiteReadOnly(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	var queryOnly, count int
	if err := db.QueryRow("PRAGMA query_only").Scan(&queryOnly); err != nil || queryOnly != 1 {
		t.Fatalf("query_only=%d err=%v", queryOnly, err)
	}
	if err := db.QueryRow("SELECT count(*) FROM ReportData").Scan(&count); err != nil || count != 10 {
		t.Fatalf("URL-escaped SQL not read exactly: %d %v", count, err)
	}
	if _, err := db.Exec("CREATE TABLE forbidden(value REAL)"); err == nil {
		t.Fatal("read-only connection accepted write")
	}
	db.Close()
	if after := epath181BackendSnapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatal("read-only SQLite changed file bytes or created sidecars")
	}
	writer, err := sql.Open("sqlite", sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	for _, statement := range []string{"PRAGMA journal_mode=WAL", "PRAGMA wal_autocheckpoint=0", "INSERT INTO ReportData VALUES(11,1,1,999)"} {
		if _, err := writer.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	before = epath181BackendSnapshot(t, dir)
	if reader, err := openSimulationSQLiteReadOnly(sqlPath); err == nil {
		reader.Close()
		t.Fatal("active WAL opened without a safe snapshot")
	} else if !strings.Contains(err.Error(), "checkpoint") {
		t.Fatalf("WAL error lacks action: %v", err)
	}
	if after := epath181BackendSnapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatal("WAL rejection mutated sidecars")
	}
	writer.Close()
	before = epath181BackendSnapshot(t, dir)
	if reader, err := openSimulationSQLiteReadOnly(sqlPath); err == nil {
		reader.Close()
		t.Fatal("closed WAL mode can recreate sidecars; must reject")
	}
	if after := epath181BackendSnapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatal("closed WAL rejection created artifacts")
	}
}

func TestEPATH181LoaderCanonicalParityProvenanceAndNoWrites(t *testing.T) {
	sqlPath, inputPath := epath181BackendFixture(t)
	before := epath181BackendSnapshot(t, filepath.Dir(sqlPath))
	request := EnergyPathProjectionRequest{ResultPath: sqlPath, InputPath: inputPath}
	loaded, err := LoadEnergyPathProjection(request)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Provenance == nil || loaded.Provenance.InputVerified || loaded.Provenance.OutputPlanKnown || len(loaded.Provenance.Warnings) == 0 || loaded.Provenance.SQLPath != sqlPath || loaded.Provenance.InputPath != inputPath {
		t.Fatalf("unverified bare SQL identity: %+v", loaded.Provenance)
	}
	info, _ := os.Stat(sqlPath)
	run := SimulationRunResult{InputPath: inputPath, OutputDirectory: filepath.Dir(sqlPath), Files: []SimulationFileInfo{{Name: filepath.Base(sqlPath), Path: sqlPath, Kind: "sqlite", Size: info.Size()}}}
	expected := BuildPurposeResultBundle(&run, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	want, _ := json.Marshal(expected)
	for repeat := 0; repeat < 5; repeat++ {
		got, err := LoadEnergyPathProjection(request)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(got.PurposeResults)
		if string(encoded) != string(want) {
			t.Fatalf("repeat %d differs from direct canonical builder", repeat)
		}
	}
	if loaded.View.Quality == nil || loaded.View.Quality.Drivers.Total != 0 || loaded.View.Quality.Carriers.Total != 0 || loaded.View.Quality.Carriers.Status == "complete" {
		t.Fatalf("invented output plan coverage: %+v", loaded.View.Quality)
	}
	if after := epath181BackendSnapshot(t, filepath.Dir(sqlPath)); !reflect.DeepEqual(before, after) {
		t.Fatal("loader changed inputs or SQL artifacts")
	}
}

func TestEPATH181ManifestBindingHashAndExactSQLSelection(t *testing.T) {
	sqlPath, inputPath := epath181BackendFixture(t)
	dir := filepath.Dir(sqlPath)
	input, _ := os.ReadFile(inputPath)
	hash := sha256.Sum256(input)
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, AllocationPolicy: PurposeAllocationPolicyDirectOnly}
	manifest := SimulationRunManifest{InputPath: inputPath, InputHash: hex.EncodeToString(hash[:]), OutputPlan: plan, ResultFiles: []SimulationFileInfo{{Name: filepath.Base(sqlPath), Path: sqlPath, Kind: "sqlite"}}}
	writeManifest := func() {
		t.Helper()
		data, _ := json.Marshal(manifest)
		if err := os.WriteFile(filepath.Join(dir, "semantic-idf-run.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest()
	loaded, err := LoadEnergyPathProjection(EnergyPathProjectionRequest{ResultPath: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Provenance.InputVerified || !loaded.Provenance.OutputPlanKnown || loaded.PurposeResults.EnergyExplanation.AllocationPolicy != PurposeAllocationPolicyDirectOnly {
		t.Fatalf("verified plan not retained: %+v", loaded.Provenance)
	}
	manifest.InputHash = ""
	writeManifest()
	if _, err := LoadEnergyPathProjection(EnergyPathProjectionRequest{ResultPath: dir}); err == nil {
		t.Fatal("implicit unhashed model accepted")
	}
	if _, err := LoadEnergyPathProjection(EnergyPathProjectionRequest{ResultPath: dir, InputPath: inputPath}); err != nil {
		t.Fatalf("explicit unverified pairing rejected: %v", err)
	}
	manifest.InputHash = strings.Repeat("0", 64)
	writeManifest()
	if _, err := LoadEnergyPathProjection(EnergyPathProjectionRequest{ResultPath: sqlPath, InputPath: inputPath}); err == nil {
		t.Fatal("recorded input hash mismatch accepted")
	}
	manifest.InputHash = hex.EncodeToString(hash[:])
	writeManifest()
	sibling := filepath.Join(dir, "other.sql")
	createEnergyPathContractSQL(t, sibling)
	if _, err := LoadEnergyPathProjection(EnergyPathProjectionRequest{ResultPath: dir, InputPath: inputPath}); err == nil {
		t.Fatal("ambiguous directory chose first SQL")
	}
	loaded, err = LoadEnergyPathProjection(EnergyPathProjectionRequest{ResultPath: sibling, InputPath: inputPath})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Provenance.InputVerified || loaded.Provenance.OutputPlanKnown || loaded.Provenance.SQLPath != sibling {
		t.Fatal("unrelated SQL borrowed sibling manifest or substituted SQL")
	}
	corrupt := filepath.Join(dir, "broken.sql")
	if err := os.WriteFile(corrupt, []byte("not SQLite"), 0600); err != nil {
		t.Fatal(err)
	}
	before := epath181BackendSnapshot(t, dir)
	if _, err := LoadEnergyPathProjection(EnergyPathProjectionRequest{ResultPath: corrupt, InputPath: inputPath}); err == nil {
		t.Fatal("corrupt SQL returned successful empty graph")
	}
	if !reflect.DeepEqual(before, epath181BackendSnapshot(t, dir)) {
		t.Fatal("failed exact SQL read changed files")
	}
}

func TestEPATH181LoaderUsesExistingEpJSONInputReader(t *testing.T) {
	sqlPath, _ := epath181BackendFixture(t)
	inputPath := filepath.Join(filepath.Dir(sqlPath), "reported model.epJSON")
	model := []byte(`{"Version":{"Version 1":{"version_identifier":"24.2"}},"Building":{"Example":{"north_axis":0}},"Zone":{"Office":{"multiplier":1}}}`)
	if err := os.WriteFile(inputPath, model, 0600); err != nil {
		t.Fatal(err)
	}
	before := epath181BackendSnapshot(t, filepath.Dir(sqlPath))
	loaded, err := LoadEnergyPathProjection(EnergyPathProjectionRequest{ResultPath: sqlPath, InputPath: inputPath})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PurposeResults.EnergyExplanation.Schema != energyExplanationSchema || len(loaded.View.Nodes) == 0 || loaded.Provenance.InputPath != inputPath {
		t.Fatal("epJSON did not use shared canonical result builder")
	}
	if !reflect.DeepEqual(before, epath181BackendSnapshot(t, filepath.Dir(sqlPath))) {
		t.Fatal("epJSON read created a converted IDF or changed files")
	}
}

func TestEPATH181ProjectionEmptyExactMonthlyAndImmutableServiceSelection(t *testing.T) {
	annual := EnergyExplanationNode{ID: "load.annual", Level: "load", Kind: "load.cooling", ServiceKind: "cooling", ScaleDomain: "thermal", Unit: "kWh", Value: 900, Period: "annual"}
	monthly := annual
	monthly.ID = "load.month"
	monthly.Value = 40
	monthly.Period = "M1"
	site := EnergyExplanationNode{ID: "use.month", Level: "end_use", Kind: "end_use.cooling", EndUse: "cooling", ServiceKind: "cooling", ScaleDomain: "site", Unit: "kWh", Value: 10, Period: "M1"}
	heat := annual
	heat.ID = "heat.month"
	heat.Kind = "load.heating"
	heat.ServiceKind = "heating"
	heat.Period = "M1"
	heat.Value = 85
	link := EnergyPathLink{ID: "conversion", FromID: monthly.ID, ToID: site.ID, Relation: "load_to_end_use", ServiceKind: "cooling", Period: "M1", FromValue: 40, ToValue: 10, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"reported"}}
	quality := &EnergyPathQuality{DriverToLoadStatus: "partial", DriverToLoadClosedPct: 80}
	graph := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Nodes: []EnergyExplanationNode{annual}, Periods: []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: []EnergyExplanationNode{monthly, site, heat}, Links: []EnergyPathLink{link}, Quality: quality}, {ID: "M3", Kind: "monthly"}}}
	bundle := PurposeResultBundle{EnergyExplanation: graph}
	before, _ := json.Marshal(bundle)
	projected, err := ProjectEnergyPath(bundle, EnergyPathSelection{Period: "M1", Service: "cooling"})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.View.Nodes) != 2 || len(projected.View.Links) != 1 || projected.View.Links[0].FromValue != 40 || projected.View.Summary.Period != "M1" || projected.View.Quality.DriverToLoadClosedPct != 80 {
		t.Fatalf("wrong selected period/service: %+v", projected.View)
	}
	after, _ := json.Marshal(bundle)
	canonical, _ := json.Marshal(projected.PurposeResults)
	if string(before) != string(after) || string(before) != string(canonical) {
		t.Fatal("selection mutated original canonical payload")
	}
	empty, err := ProjectEnergyPath(bundle, EnergyPathSelection{Period: "M3"})
	if err != nil || len(empty.View.Nodes) != 0 || len(empty.View.Summary.Loads) != 0 {
		t.Fatalf("explicit empty month fell back: %+v %v", empty.View, err)
	}
	if _, err := ProjectEnergyPath(bundle, EnergyPathSelection{Period: "M2"}); err == nil {
		t.Fatal("absent month fell back to annual")
	}
	graph.Nodes = []EnergyExplanationNode{monthly, site}
	graph.Links = []EnergyPathLink{link}
	graph.Periods = nil
	if got, err := ProjectEnergyPath(PurposeResultBundle{EnergyExplanation: graph}, EnergyPathSelection{Period: "M1"}); err != nil || len(got.View.Nodes) != 2 {
		t.Fatalf("exact monthly top-level graph unavailable: %v", err)
	}
	graph.Nodes[0].Period = ""
	if _, err := ProjectEnergyPath(PurposeResultBundle{EnergyExplanation: graph}, EnergyPathSelection{Period: "M1"}); err == nil {
		t.Fatal("unproven top-level month accepted")
	}
}

func TestEPATH181CSVQualityUnknownAndTraceUnits(t *testing.T) {
	projection := EnergyPathProjection{Selection: EnergyPathSelection{Scope: "building", Period: "M1", Service: "all"}, View: EnergyPathProjectionView{Quality: &EnergyPathQuality{Drivers: EnergyCompletenessLevel{Status: "partial", Found: 0, Total: 2}, Carriers: EnergyCompletenessLevel{Status: "unavailable"}, DriverToLoadStatus: "partial", DriverToLoadClosedPct: 0, EndUseToCarrierStatus: "unavailable", ZoneAllocationStatus: "not_requested"}, Sources: []EnergyDataSource{{ID: "source", Name: "Cooling", SourceUnit: "J", NormalizedUnit: "kWh", RawValue: 999}}}}
	encoded, err := EnergyPathProjectionCSV(projection, true)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(encoded)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string][]string{}
	for _, row := range rows[1:] {
		byKey[row[1]+"|"+row[2]] = row
	}
	if row := byKey["drivers|output_availability"]; row[23] != "0" || row[24] != "2" {
		t.Fatalf("reported zero count lost: %v", row)
	}
	if row := byKey["carriers|output_availability"]; row[23] != "" || row[24] != "" {
		t.Fatalf("unknown count fabricated: %v", row)
	}
	if row := byKey["|driver_to_load_closed_pct"]; row[3] != "0" {
		t.Fatalf("reported zero closure lost: %v", row)
	}
	if row := byKey["|end_use_to_carrier_closed_pct"]; row[3] != "" {
		t.Fatalf("unknown closure fabricated: %v", row)
	}
	if row := byKey["|Cooling"]; row[25] != "J" || row[26] != "kWh" || row[9] != "" || row[3] != "" {
		t.Fatalf("source units/period scalar misrepresented: %v", row)
	}
}

func TestEPATH181CSVCarrierClosureUsesFullScopeDenominator(t *testing.T) {
	for _, measured := range []bool{false, true} {
		node := EnergyExplanationNode{ID: "facility", Level: "carrier", ScaleDomain: "site", Carrier: "electricity", Unit: "kWh", Value: 100, Basis: "reported_end_use_subtotal"}
		if measured {
			node.Basis = "reported_meter"
			node.MeterHierarchyLevel = "facility_total"
		}
		quality := &EnergyPathQuality{EndUseToCarrierStatus: "partial", EndUseToCarrierClosedPct: 0}
		graph := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Nodes: []EnergyExplanationNode{node}, Quality: quality}
		projection, err := ProjectEnergyPath(PurposeResultBundle{EnergyExplanation: graph}, EnergyPathSelection{Service: "cooling"})
		if err != nil {
			t.Fatal(err)
		}
		// The unrelated carrier is absent from this service view, but the
		// canonical full-scope quality denominator must still govern CSV.
		if len(projection.View.Nodes) != 0 {
			t.Fatal("fixture carrier should be outside filtered service")
		}
		encoded, err := EnergyPathProjectionCSV(projection, false)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := csv.NewReader(strings.NewReader(encoded)).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows[1:] {
			if row[2] != "end_use_to_carrier_closed_pct" {
				continue
			}
			found = true
			if measured && row[3] != "0" {
				t.Fatalf("known true zero erased: %v", row)
			}
			if !measured && (row[3] != "" || row[27] == "") {
				t.Fatalf("unmeasured subtotal became 0%%: %v", row)
			}
			if row[22] != "partial" {
				t.Fatal("canonical quality status changed")
			}
		}
		if !found {
			t.Fatal("missing closure quality row")
		}
	}
}
