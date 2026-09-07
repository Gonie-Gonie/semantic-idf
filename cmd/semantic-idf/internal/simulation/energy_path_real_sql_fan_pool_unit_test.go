package simulation

import (
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func epathFanPoolUnitSQL(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "independent-fan-pools.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentName TEXT,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(1,'FULL WEATHER 2017',3),(2,'DESIGN DAY',1)`,
		`CREATE TABLE "Time"(TimeIndex INTEGER PRIMARY KEY,EnvironmentPeriodIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,WarmupFlag INTEGER,IntervalType INTEGER,SimulationDays INTEGER)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,Name TEXT,KeyValue TEXT,IsMeter INTEGER,ReportingFrequency TEXT,Units TEXT,IndexGroup TEXT)`,
		`INSERT INTO ReportDataDictionary VALUES(101,'Air System Fan Electricity Energy','LOOP A',0,'Hourly','J','System'),(202,'Air System Fan Electricity Energy','LOOP B',0,'Hourly','J','System'),(303,'Fans:Electricity','',1,'Monthly','J','Facility')`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,ReportDataDictionaryIndex INTEGER,TimeIndex INTEGER,Value REAL)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	insertTime, err := tx.Prepare(`INSERT INTO "Time" VALUES(?,1,2017,?,?,?,0,60,NULL,1,?)`)
	if err != nil {
		t.Fatal(err)
	}
	insertValue, err := tx.Prepare(`INSERT INTO ReportData VALUES(?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := 1; index <= 8760; index++ {
		date := start.Add(time.Duration(index-1) * time.Hour)
		hours := time.Date(2017, date.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day() * 24
		if _, err := insertTime.Exec(index, date.Month(), date.Day(), date.Hour()+1, date.YearDay()); err != nil {
			t.Fatal(err)
		}
		for pool, monthly := range []float64{100, 300} {
			if _, err := insertValue.Exec(index+pool*10000, 101*(pool+1), index, monthly*3600000/float64(hours)); err != nil {
				t.Fatal(err)
			}
		}
	}
	insertTime.Close()
	insertValue.Close()
	for month := 1; month <= 12; month++ {
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
		if _, err := tx.Exec(`INSERT INTO "Time" VALUES(?,1,2017,?,?,24,0,?,NULL,3,?)`, 20000+month, month, last.Day(), last.Day()*1440, last.YearDay()); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO ReportData VALUES(?,303,?,?)`, 30000+month, 20000+month, 400*3600000); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func epathFanPoolUnitInputs(t *testing.T, path string) (epathRealOracleEvidence, epathSQLFrames, []epathRealSQLFanPool, epathRealSQLPrecision) {
	t.Helper()
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{}, Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{"fans": {}}}
	for _, zone := range []string{"A", "B", "C", "D", "Unserved"} {
		frames.Zones[strings.ToLower(zone)] = epathSQLZone{Name: zone, Multiplier: 1}
		for month := 1; month <= 12; month++ {
			weight := 1.0
			if zone == "B" || zone == "C" {
				weight = 3
			}
			if month == 2 && zone == "A" {
				weight = 3
			}
			if month == 2 && zone == "B" {
				weight = 1
			}
			if zone == "Unserved" {
				weight = 10000
			}
			frames.Loads[epathSQLKey(zone, "cooling", month)] = epathSQLQuantity{Value: weight}
			frames.Loads[epathSQLKey(zone, "heating", month)] = epathSQLQuantity{}
		}
	}
	for month := 0; month < 12; month++ {
		frames.Site["fans"] = append(frames.Site["fans"], &epathSQLQuantity{Value: 400})
	}
	pools := []epathRealSQLFanPool{
		{SiteID: "fans", Name: "Air System Fan Electricity Energy", Key: "LOOP A", Frequency: "Hourly", Unit: "J", ServedZones: []string{"A", "B"}},
		{SiteID: "fans", Name: "Air System Fan Electricity Energy", Key: "LOOP B", Frequency: "Hourly", Unit: "J", ServedZones: []string{"C", "D"}},
	}
	return observed, frames, pools, epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}
}

func TestEnergyPathRealSQLFanPoolIndependentPartition(t *testing.T) {
	path := epathFanPoolUnitSQL(t)
	observed, frames, pools, precision := epathFanPoolUnitInputs(t, path)
	before, err := epathReadRealFileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelFanPoolChecks(observed, frames, pools, precision, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 5*13+2*2 {
		t.Fatalf("missing monthly/annual Zone or exact-source checks: %d", len(checks.Rows))
	}
	wants := map[string]float64{"A/M1": 25, "B/M1": 75, "C/M1": 225, "D/M1": 75, "A/M2": 75, "B/M2": 25, "A/annual": 350, "B/annual": 850, "C/annual": 2700, "D/annual": 900, "Unserved/annual": 0}
	for _, check := range checks.Rows {
		if check.Item.Target.Collection == "sources" {
			want := 1200.0
			if check.Item.Target.SourceKey == "LOOP B" {
				want = 3600
			}
			if check.Item.Target.SourceName != "Air System Fan Electricity Energy" || check.Item.Target.SourceUnit != "J" || check.Item.Target.Frequency != "Hourly" || check.Want.Value == nil || math.Abs(*check.Want.Value-want) > 1e-7 {
				t.Fatalf("pool source annual is not its independent observation: %+v", check)
			}
			continue
		}
		key := check.Want.Zone + "/" + check.Want.Period
		if want, ok := wants[key]; ok {
			if check.Want.Value == nil || math.Abs(*check.Want.Value-want) > 1e-7 {
				t.Fatalf("%s=%v, want %g", key, check.Want.Value, want)
			}
			delete(wants, key)
		}
		if check.Want.Zone == "A" && check.Want.Period == "M1" {
			if err := epathCheckSQLModelQuantity(epathOracleNumber(50), check.Quantity); err == nil {
				t.Fatal("wrong 400-kWh global-load share passed 100-kWh pool expectation")
			}
		}
	}
	if len(wants) != 0 {
		t.Fatalf("missing checks: %v", wants)
	}
	for _, check := range checks.Rows {
		if check.Want.Zone != "A" || check.Want.Period != "M1" {
			continue
		}
		// Canonical end-use nodes aggregate carriers. The electricity identity
		// belongs to the outgoing carrier endpoint, not a required node field.
		// Values here are independently established above, not candidate copies.
		fan := EnergyExplanationNode{ID: "arbitrary-fan-id", Level: "end_use", EndUse: "fans", Value: 25, Unit: "kWh", ScaleDomain: "site", Basis: "service_path_allocation", ZoneName: "A", Period: "M1"}
		carrier := EnergyExplanationNode{ID: "arbitrary-carrier-id", Level: "carrier", Carrier: "electricity", Value: 25, Unit: "kWh", ScaleDomain: "site", ZoneName: "A", Period: "M1"}
		link := EnergyPathLink{ID: "arbitrary-link-id", FromID: fan.ID, ToID: carrier.ID, Relation: "direct_end_use_to_carrier", FromValue: 25, ToValue: 25, FromUnit: "kWh", ToUnit: "kWh", Basis: "service_path_allocation", ZoneName: "A", Period: "M1"}
		makeBundle := func(node EnergyExplanationNode) PurposeResultBundle {
			return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "A"}, Periods: []EnergyPeriod{{ID: "M1", Nodes: []EnergyExplanationNode{node, carrier}, Links: []EnergyPathLink{link}}}}}}}
		}
		bundle := makeBundle(fan)
		actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
		if err != nil {
			t.Fatal(err)
		}
		if err := epathCheckSQLModelPresentation(bundle, check, actual); err != nil {
			t.Fatalf("carrier-agnostic canonical fan node falsely unknown: %v", err)
		}
		for _, mutate := range []func(*EnergyExplanationNode){
			func(n *EnergyExplanationNode) { n.EndUse = "pumps" },
			func(n *EnergyExplanationNode) { n.Basis = "zone_load_allocation" },
			func(n *EnergyExplanationNode) { n.Unit = "MJ" },
			func(n *EnergyExplanationNode) { n.ScaleDomain = "thermal" },
			func(n *EnergyExplanationNode) { n.Value = 50 },
		} {
			wrong := fan
			mutate(&wrong)
			bundle := makeBundle(wrong)
			actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
			if err == nil && epathCheckSQLModelPresentation(bundle, check, actual) == nil {
				t.Fatal("canonical aggregate selector accepted wrong category/basis/domain/unit/quantity")
			}
		}
	}
	after, err := epathReadRealFileHash(path)
	if err != nil || before != after {
		t.Fatal("independent fan oracle changed input SQL")
	}
	// Different source, pool and served-Zone order cannot alter expectations.
	for left, right := 0, len(observed.Sources)-1; left < right; left, right = left+1, right-1 {
		observed.Sources[left], observed.Sources[right] = observed.Sources[right], observed.Sources[left]
	}
	pools[0], pools[1] = pools[1], pools[0]
	var reversed epathSQLModelChecks
	if err := epathSQLModelFanPoolChecks(observed, frames, pools, precision, &reversed); err != nil {
		t.Fatal(err)
	}
	byKey := map[string]float64{}
	for _, check := range checks.Rows {
		byKey[check.Want.Key] = *check.Want.Value
	}
	for _, check := range reversed.Rows {
		if math.Abs(byKey[check.Want.Key]-*check.Want.Value) > 1e-10 {
			t.Fatal("input order changed independent pool allocation")
		}
	}
}

func TestEnergyPathRealSQLFanPoolRejectsUnprovenInputs(t *testing.T) {
	path := epathFanPoolUnitSQL(t)
	for _, test := range []struct {
		name   string
		mutate func(*epathRealOracleEvidence, *epathSQLFrames, *[]epathRealSQLFanPool)
	}{
		{"missing pool", func(_ *epathRealOracleEvidence, _ *epathSQLFrames, p *[]epathRealSQLFanPool) { *p = (*p)[:1] }},
		{"duplicate pool", func(_ *epathRealOracleEvidence, _ *epathSQLFrames, p *[]epathRealSQLFanPool) {
			*p = append(*p, (*p)[0])
		}},
		{"overlapping zone", func(_ *epathRealOracleEvidence, _ *epathSQLFrames, p *[]epathRealSQLFanPool) {
			(*p)[1].ServedZones[0] = "A"
		}},
		{"unknown zone", func(_ *epathRealOracleEvidence, _ *epathSQLFrames, p *[]epathRealSQLFanPool) {
			(*p)[0].ServedZones[0] = "Missing"
		}},
		{"unsupported unit", func(_ *epathRealOracleEvidence, _ *epathSQLFrames, p *[]epathRealSQLFanPool) { (*p)[0].Unit = "W" }},
		{"monthly masquerading as hourly", func(_ *epathRealOracleEvidence, _ *epathSQLFrames, p *[]epathRealSQLFanPool) {
			(*p)[0].Frequency = "Monthly"
		}},
		{"wrong exact key", func(_ *epathRealOracleEvidence, _ *epathSQLFrames, p *[]epathRealSQLFanPool) {
			(*p)[0].Key = "OTHER LOOP"
		}},
		{"duplicate dictionary", func(o *epathRealOracleEvidence, _ *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			o.Sources = append(o.Sources, o.Sources[0])
		}},
		{"missing source month", func(o *epathRealOracleEvidence, _ *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			o.Sources[0].Months = o.Sources[0].Months[:11]
		}},
		{"duplicate source month", func(o *epathRealOracleEvidence, _ *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			o.Sources[0].Months[1] = o.Sources[0].Months[0]
		}},
		{"null source", func(o *epathRealOracleEvidence, _ *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			o.Sources[0].RawSum = nil
		}},
		{"wrong broad source identity", func(o *epathRealOracleEvidence, _ *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			o.Sources[2].Name = "Lights:Electricity"
		}},
		{"null monthly source", func(o *epathRealOracleEvidence, _ *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			o.Sources[0].Months[0].RawSum = nil
		}},
		{"nonfinite source", func(o *epathRealOracleEvidence, _ *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			o.Sources[0].Months[0].RawSum = epathOracleNumber(math.NaN())
		}},
		{"unknown broad meter", func(_ *epathRealOracleEvidence, f *epathSQLFrames, _ *[]epathRealSQLFanPool) { f.Site["fans"][0] = nil }},
		{"wrong broad meter", func(_ *epathRealOracleEvidence, f *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			f.Site["fans"][0].Value = 401
		}},
		{"zero broad meter", func(_ *epathRealOracleEvidence, f *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			f.Site["fans"][0].Value = 0
		}},
		{"missing load not zero", func(_ *epathRealOracleEvidence, f *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			delete(f.Loads, epathSQLKey("A", "cooling", 1))
		}},
		{"unknown load", func(_ *epathRealOracleEvidence, f *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			f.Loads[epathSQLKey("A", "cooling", 1)] = epathSQLQuantity{Value: math.NaN()}
		}},
		{"zero served denominator", func(_ *epathRealOracleEvidence, f *epathSQLFrames, _ *[]epathRealSQLFanPool) {
			f.Loads[epathSQLKey("A", "cooling", 1)], f.Loads[epathSQLKey("B", "cooling", 1)] = epathSQLQuantity{}, epathSQLQuantity{}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			observed, frames, pools, precision := epathFanPoolUnitInputs(t, path)
			test.mutate(&observed, &frames, &pools)
			if err := epathSQLModelFanPoolChecks(observed, frames, pools, precision, &epathSQLModelChecks{}); err == nil {
				t.Fatal("unproven fan attribution accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLFanPoolZeroPartitionGate(t *testing.T) {
	path := epathFanPoolUnitSQL(t)
	epathOracleEditSQL(t, path, `UPDATE ReportData SET Value=0`)
	observed, frames, pools, precision := epathFanPoolUnitInputs(t, path)
	for _, month := range frames.Site["fans"] {
		month.Value = 0
	}
	if err := epathSQLModelFanPoolChecks(observed, frames, pools, precision, &epathSQLModelChecks{}); err == nil {
		t.Fatal("zero pool partition silently accepted as positive operation evidence")
	}
}

func TestEnergyPathRealSQLFanPoolHourlyCalendarGuard(t *testing.T) {
	base := epathFanPoolUnitSQL(t)
	observed, _, pools, _ := epathFanPoolUnitInputs(t, base)
	items, err := epathSQLFanPoolObservations(observed.Sources, pools)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE ReportData SET TimeIndex=2 WHERE ReportDataDictionaryIndex=101 AND TimeIndex=1`,
		`UPDATE "Time" SET Hour=2 WHERE TimeIndex=1`,
		`UPDATE "Time" SET WarmupFlag=1 WHERE TimeIndex=1`,
		`UPDATE "Time" SET EnvironmentPeriodIndex=2 WHERE TimeIndex=1`,
		`UPDATE "Time" SET "Interval"=30 WHERE TimeIndex=1`,
		`UPDATE "Time" SET IntervalType=0 WHERE TimeIndex=1`,
		`UPDATE "Time" SET Year=2018 WHERE TimeIndex=1`,
		`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=101 AND TimeIndex=1`,
		`UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=101 AND TimeIndex=1`,
		`UPDATE ReportDataDictionary SET KeyValue='WRONG LOOP' WHERE ReportDataDictionaryIndex=101`,
	} {
		t.Run(statement, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.sql")
			if err := os.WriteFile(path, bytes, 0600); err != nil {
				t.Fatal(err)
			}
			epathOracleEditSQL(t, path, statement)
			if err := epathSQLFanPoolAuditHours(path, items); err == nil {
				t.Fatal("row count/total alone concealed missing, duplicate or invalid hour")
			}
		})
	}
}

// SQL-only opt-in compiles independent checks, not a new candidate or expected
// manifest. Production integration is validated later through the same hook.
func TestEnergyPathRealSQLFanPoolSavedObservations(t *testing.T) {
	directory := strings.TrimSpace(os.Getenv("EPATH_REAL_FAN_POOL_DIR"))
	if directory == "" {
		t.Skip("explicit saved fan-pool SQL required; no engine or snapshot writes")
	}
	if os.Getenv("EPATH_REAL_RUN") == "1" || os.Getenv("EPATH_REAL_CAPTURE") == "1" {
		t.Fatal("fan-pool observation cannot launch an engine")
	}
	root, catalog := epathRealDirectories(t)
	var evidence epathRealRunEvidence
	file, err := os.Open(filepath.Join(directory, "run-evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = json.NewDecoder(file).Decode(&evidence)
	file.Close()
	if err != nil || !epathRealSamePath(directory, evidence.RunDirectory) || evidence.Fixture.ID != "large-office-25-1" || evidence.Version != "25.1" {
		t.Fatalf("exact captured 25.1 LargeOffice is required: %v", err)
	}
	if err := epathValidateSavedRealEvidence(root, catalog, evidence); err != nil {
		t.Fatal(err)
	}
	before := epathReplayDirectorySnapshot(t, directory)
	defer func() {
		if !reflect.DeepEqual(before, epathReplayDirectorySnapshot(t, directory)) {
			t.Error("fan-pool observation changed saved run artifacts")
		}
	}()
	recipe, err := epathLoadRealOracleRecipe(filepath.Join(catalog, filepath.FromSlash(evidence.Fixture.OraclePath)))
	if err != nil || recipe.SQLModel == nil || len(recipe.SQLModel.FanPools) != 4 {
		t.Fatalf("four reviewed fan pools required: %v", err)
	}
	observed, err := epathReadRealSQLOracle(evidence.SQLPath)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(evidence.SQLPath, observed.Sources, *recipe.SQLModel)
	if err != nil {
		t.Fatal(err)
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelFanPoolChecks(observed, frames, recipe.SQLModel.FanPools, recipe.SQLModel.Precision, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 19*13+4*2 {
		t.Fatalf("missing exact Zone/source checks: %d", len(checks.Rows))
	}
	for _, check := range checks.Rows {
		if check.Item.Target.Collection == "sources" && check.Item.Target.Field == "rawValue" || check.Want.Scope == "zone" && check.Want.Period == "annual" {
			t.Logf("independent %s = %.9f kWh", check.Want.Key, *check.Want.Value)
		}
	}
	t.Log("SQL-only pool proof: 4 × 8760 unique full-year hours, monthly pool sum closes Fans meter; no candidate acceptance or expected-file writes")
}
