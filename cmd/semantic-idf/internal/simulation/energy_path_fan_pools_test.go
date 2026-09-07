package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func epathFanPoolSQL(t *testing.T, poolCount int) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,KeyValue TEXT,Name TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
		`CREATE TABLE "Time"(TimeIndex INTEGER PRIMARY KEY,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" INTEGER,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER)`,
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentName TEXT,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(1,'Sizing',1),(3,'Annual weather',3)`,
		`INSERT INTO ReportDataDictionary VALUES(1,'Fans:Electricity','Fans:Electricity','J',1,'Monthly','Facility'),(2,'Electricity:Facility','Electricity:Facility','J',1,'Monthly','Facility')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	insertTime, err := tx.Prepare(`INSERT INTO "Time" VALUES(?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer insertTime.Close()
	insertValue, err := tx.Prepare(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer insertValue.Close()
	for pool := 0; pool < poolCount; pool++ {
		if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J',0,'Hourly','HVAC System')`, 100+pool, fmt.Sprintf("VAV_%d", pool+1), "Air System Fan Electricity Energy"); err != nil {
			t.Fatal(err)
		}
		for zone := 0; zone < 2; zone++ {
			if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,'Zone Air System Sensible Cooling Energy','J',0,'Monthly','Zone')`, 200+pool*2+zone, fmt.Sprintf("Zone %d", pool*2+zone)); err != nil {
				t.Fatal(err)
			}
		}
	}
	timeID := 0
	for month := 1; month <= 12; month++ {
		days := time.Date(2017, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		for day := 1; day <= days; day++ {
			for hour := 1; hour <= 24; hour++ {
				timeID++
				if _, err := insertTime.Exec(timeID, 2017, month, day, hour, 0, 60, 1, 3, nil); err != nil {
					t.Fatal(err)
				}
				for pool := 0; pool < poolCount; pool++ {
					if _, err := insertValue.Exec(timeID, 100+pool, float64(pool+1)*12345.6789); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		monthlyID := 10000 + month
		if _, err := insertTime.Exec(monthlyID, 2017, month, days, 24, 0, days*24*60, 3, 3, nil); err != nil {
			t.Fatal(err)
		}
		total := float64(poolCount*(poolCount+1)/2) * 12345.6789 * float64(days*24)
		for _, dictionary := range []int{1, 2} {
			if _, err := insertValue.Exec(monthlyID, dictionary, total); err != nil {
				t.Fatal(err)
			}
		}
		for pool := 0; pool < poolCount; pool++ {
			for zone := 0; zone < 2; zone++ {
				if _, err := insertValue.Exec(monthlyID, 200+pool*2+zone, float64(zone+1)*3600000); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	// Foreign environment, warmup and wrong interval rows must not enter pools.
	for index, tuple := range [][3]int{{1, 1, 0}, {3, 1, 1}, {3, 2, 0}} {
		id := 20000 + index
		if _, err := insertTime.Exec(id, 2017, 1, 1, 1, 0, 60, tuple[1], tuple[0], tuple[2]); err != nil {
			t.Fatal(err)
		}
		for pool := 0; pool < poolCount; pool++ {
			if _, err := insertValue.Exec(id, 100+pool, 1e12); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return db, path
}

func epathFanPoolTopology(count int) energyServicePathIndex {
	paths := []energyPathAuxiliaryServicePath{}
	for pool := 0; pool < count; pool++ {
		for zone := 0; zone < 2; zone++ {
			paths = append(paths, epath101AuditPath(fmt.Sprintf("path.%d.%d", pool, zone), fmt.Sprintf("Zone %d", pool*2+zone), "cooling", fmt.Sprintf("VAV_%d", pool+1), "", ""))
		}
	}
	return epath101AuditTopology(paths)
}

func epathFanPoolNodes(pools []energyPathFanPool, period string) []EnergyExplanationNode {
	total := 0.0
	for _, pool := range pools {
		value, _ := energyPathFanPoolValue(pool, period)
		total += value
	}
	nodes := []EnergyExplanationNode{epath101AuditAuxiliary("fans", roundedEnergyNumber(total), "broad.fans", nil)}
	for pool := range pools {
		for zone := 0; zone < 2; zone++ {
			nodes = append(nodes, epath101AuditLoad(fmt.Sprintf("Zone %d", pool*2+zone), "cooling", float64(zone+1), fmt.Sprintf("load.%d.%d", pool, zone), []string{fmt.Sprintf("path.%d.%d", pool, zone)}))
		}
	}
	return nodes
}

func TestEnergyPathReportedFanPoolsHourlySQLPrivateAndExact(t *testing.T) {
	for _, count := range []int{2, 4} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			db, path := epathFanPoolSQL(t, count)
			plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
			pools, err := readEnergyPathFanPools(db, "eplusout.sql", plan)
			if err != nil {
				t.Fatal(err)
			}
			if len(pools) != count {
				t.Fatalf("pool count=%d", len(pools))
			}
			for i, pool := range pools {
				if pool.Invalid || len(pool.Monthly) != 12 {
					t.Fatalf("incomplete pool: %#v", pool)
				}
				for month, value := range pool.Monthly {
					days := time.Date(2017, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
					want := float64(i+1) * 12345.6789 * float64(days*24) / 3600000
					if math.Abs(value-want) > 1e-9 {
						t.Fatalf("pool %d month%d=%g want raw-then-summed %g", i, month, value, want)
					}
				}
			}
			without, err := readEnergyPathFanPools(db, "eplusout.sql", &PurposeRunPlan{})
			if err != nil || len(without) != 0 {
				t.Fatal("legacy plan gained pool ingestion")
			}
			legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, plan, energyDriverBuildContext{Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(legacy.auxiliaryFanPools) != count {
				t.Fatal("private pools were not carried through SQL builder")
			}
			wire, _ := json.Marshal(legacy)
			if strings.Contains(string(wire), "Air System Fan") || strings.Contains(string(wire), "sql-rdd-100") {
				t.Fatal("fan pools leaked into frozen v1 wire/default sources")
			}
			legacy.AllocationPolicy = PurposeAllocationPolicyByServicePathLoadShare
			legacy.servicePathIndex = epathFanPoolTopology(count)
			before, _ := json.Marshal(legacy)
			result := UpgradeEnergyExplanationV1(legacy)
			baseline := legacy
			baseline.auxiliaryFanPools = nil
			baselineResult := UpgradeEnergyExplanationV1(baseline)
			if !reflect.DeepEqual(result.Nodes, baselineResult.Nodes) || !reflect.DeepEqual(result.Links, baselineResult.Links) {
				t.Fatal("pool evidence changed Building graph values/default nodes or doubled the broad Fans meter")
			}
			after, _ := json.Marshal(legacy)
			if string(before) != string(after) {
				t.Fatal("v2 pool integration mutated v1")
			}
			fans := 0
			for _, node := range result.Nodes {
				if node.Level == "end_use" && node.EndUse == "fans" {
					fans++
					if len(node.SourceIDs) != 1 || node.SourceIDs[0] != "sql-rdd-1" {
						t.Fatalf("Building fan node became a pool observation: %#v", node)
					}
				}
			}
			if fans != 1 {
				t.Fatalf("default graph gained %d fan nodes", fans)
			}
			poolSource := energyExplanationSourceByID(result.Sources, "sql-rdd-100")
			if poolSource == nil || poolSource.KeyValue != "VAV_1" || poolSource.SourceUnit != "J" || poolSource.ReportingFrequency != "Hourly" || poolSource.NormalizedUnit != "kWh" {
				t.Fatalf("lost actual pool provenance: %#v", poolSource)
			}
			for _, detail := range poolSource.ScopeDetails {
				if detail.RawValue != 0 || detail.EffectiveValue != 0 || detail.AllocatedValue <= 0 || !detail.AllocationApplied {
					t.Fatalf("pool fabricated measured Zone values: %#v", detail)
				}
				if detail.Scope.ZoneName != "Zone 0" && detail.Scope.ZoneName != "Zone 1" {
					t.Fatalf("source escaped exact AirLoop: %#v", detail)
				}
			}
			if len(poolSource.ScopeDetails) != 2 {
				t.Fatalf("source-local scope details absent: %#v", poolSource.ScopeDetails)
			}
		})
	}
}

func TestEnergyPathReportedFanPoolsAllocationBoundaries(t *testing.T) {
	db, _ := epathFanPoolSQL(t, 4)
	pools, err := readEnergyPathFanPools(db, "eplusout.sql", &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	if err != nil {
		t.Fatal(err)
	}
	nodes := epathFanPoolNodes(pools, "M1")
	topology := epathFanPoolTopology(4)
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true, pools)
	if len(plan.Edges) != 8 || len(plan.FanSourceAllocations) != 8 {
		t.Fatalf("source-local allocation missing: %#v", plan)
	}
	for _, edge := range plan.Edges {
		var poolIndex, zone int
		if _, err := fmt.Sscanf(edge.RelatedPathIDs[0], "path.%d.%d", &poolIndex, &zone); err != nil {
			t.Fatal(err)
		}
		if !stringSliceContains(edge.SourceIDs, fmt.Sprintf("sql-rdd-%d", 100+poolIndex)) {
			t.Fatalf("wrong source pool: %#v", edge)
		}
		for other := 0; other < 4; other++ {
			if other != poolIndex && stringSliceContains(edge.SourceIDs, fmt.Sprintf("sql-rdd-%d", 100+other)) {
				t.Fatal("cross-loop source leakage")
			}
		}
		want := energyPathZoneHVACProportionalValues(pools[poolIndex].Monthly[1], []float64{1, 2})[zone]
		if edge.Value != want {
			t.Fatalf("loop %d zone %d got %g want own pool share %g", poolIndex, zone, edge.Value, want)
		}
	}
	partial := append([]energyPathFanPool(nil), pools...)
	partial[3].Monthly = map[int]float64{}
	partialPlan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true, partial)
	if len(partialPlan.Edges) != 6 || partialPlan.Records[0].UnassignedValue <= 0 {
		t.Fatal("missing pool was zero-filled or globally redistributed")
	}
	for _, mutate := range []func([]energyPathFanPool){
		func(p []energyPathFanPool) { p[0].Invalid = true },
		func(p []energyPathFanPool) { p[0].Monthly = map[int]float64{1: 100000} },
	} {
		changed := append([]energyPathFanPool(nil), pools...)
		mutate(changed)
		failed := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true, changed)
		if len(failed.Edges) != 0 || failed.Records[0].UnassignedValue != nodes[0].Value {
			t.Fatal("invalid/oversum pool silently fell back to global allocation")
		}
	}
	unknown := append([]energyPathFanPool(nil), pools...)
	unknown[0].AirLoopName = "VAV 1"
	unresolved := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true, unknown)
	if len(unresolved.Edges) != 6 || unresolved.Records[0].UnassignedValue <= 0 {
		t.Fatal("punctuation/fuzzy key substituted an unrelated AirLoop")
	}
	reverse := append([]energyPathFanPool(nil), pools...)
	for a, b := 0, len(reverse)-1; a < b; a, b = a+1, b-1 {
		reverse[a], reverse[b] = reverse[b], reverse[a]
	}
	reversePlan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true, reverse)
	if !reflect.DeepEqual(plan.Edges, reversePlan.Edges) {
		t.Fatal("pool allocation depends on input order")
	}
}

func TestEnergyPathReportedFanPoolsSQLUnknownAndKnownZero(t *testing.T) {
	for _, test := range []struct {
		name, sql        string
		unknown, invalid bool
	}{
		{"zero", `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=100`, false, false},
		{"NULL", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=100 AND TimeIndex=1`, true, false},
		{"missing hour", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=100 AND TimeIndex=1`, true, false},
		{"duplicate hour", `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(1,100,1)`, true, false},
		{"negative", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=100 AND TimeIndex=1`, true, false},
		{"duplicate dictionary", `INSERT INTO ReportDataDictionary VALUES(999,'vav_1','Air System Fan Electricity Energy','J',0,'Hourly','HVAC System')`, true, true},
		{"mixed year", `UPDATE "Time" SET Year=2018 WHERE TimeIndex=1`, true, false},
		{"incomplete weather month", `DELETE FROM "Time" WHERE TimeIndex=1`, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, _ := epathFanPoolSQL(t, 2)
			if _, err := db.Exec(test.sql); err != nil {
				t.Fatal(err)
			}
			pools, err := readEnergyPathFanPools(db, "eplusout.sql", &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
			if err != nil {
				t.Fatal(err)
			}
			value, known := energyPathFanPoolValue(pools[0], "M1")
			if known == test.unknown || pools[0].Invalid != test.invalid {
				t.Fatalf("known/invalid=%t/%t", known, pools[0].Invalid)
			}
			if test.name == "zero" && value != 0 {
				t.Fatalf("reported zero lost: %g", value)
			}
			if test.unknown {
				if _, known := energyPathFanPoolValue(pools[0], "annual"); known {
					t.Fatal("incomplete monthly pool fabricated annual scalar")
				}
			}
		})
	}
}

func TestEnergyPathReportedFanPoolsSeparateReportingFrequencies(t *testing.T) {
	db, _ := epathFanPoolSQL(t, 2)
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
	want, err := readEnergyPathFanPools(db, "eplusout.sql", plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO ReportDataDictionary VALUES(999,'vav_1','Air System Fan Electricity Energy','J',0,'Monthly','HVAC System')`,
		`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,999,1e15 FROM "Time" WHERE IntervalType=3`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	got, err := readEnergyPathFanPools(db, "eplusout.sql", plan)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("a separate Monthly dictionary invalidated or changed the unique observed Hourly pools")
	}
}

func TestEnergyPathReportedFanPoolsExactOwnerAndLocalTopology(t *testing.T) {
	pools := []energyPathFanPool{
		{Source: EnergyDataSource{ID: "pool.1"}, AirLoopName: "VAV_1", Monthly: map[int]float64{1: 12}},
		{Source: EnergyDataSource{ID: "pool.2"}, AirLoopName: "VAV_2", Monthly: map[int]float64{1: 24}},
	}
	nodes := epathFanPoolNodes(pools, "M1")
	topology := epathFanPoolTopology(2)
	baseline := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true, pools)
	if len(baseline.Edges) != 4 || baseline.Records[0].AllocatedValue != 36 {
		t.Fatalf("source-local baseline missing: %#v", baseline)
	}
	t.Run("owner path mask", func(t *testing.T) {
		restricted := append([]EnergyExplanationNode(nil), nodes...)
		restricted[0].RelatedPathIDs = []string{"path.0.0", "path.0.1"}
		got := buildEnergyPathZoneAuxiliaryAllocationPlan(restricted, nil, topology, "M1", "monthly", true, pools)
		if len(got.Edges) != 2 || got.Records[0].AllocatedValue != 12 || got.Records[0].UnassignedValue != 24 {
			t.Fatalf("pool escaped the broad owner's explicit path mask: %#v", got)
		}
		for _, edge := range got.Edges {
			if stringSliceContains(edge.SourceIDs, "pool.2") || !stringSliceContains(edge.SourceIDs, "pool.1") || !stringSliceContains(restricted[0].RelatedPathIDs, edge.RelatedPathIDs[0]) {
				t.Fatalf("unrelated pool/path provenance leaked: %#v", edge)
			}
		}
	})
	t.Run("unrelated unresolved equipment", func(t *testing.T) {
		unresolved := topology
		unresolved.auxiliaryResolvable = map[string]bool{"fans": false}
		// This flag describes incomplete global fan equipment discovery, not
		// the already observed exact AirLoop pools and their explicit paths.
		got := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, unresolved, "M1", "monthly", true, pools)
		if !reflect.DeepEqual(got.Edges, baseline.Edges) || !reflect.DeepEqual(got.FanSourceAllocations, baseline.FanSourceAllocations) {
			t.Fatal("unrelated unresolved fan equipment vetoed exact source-local measured pools")
		}
		withoutPools := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, unresolved, "M1", "monthly", true)
		if len(withoutPools.Edges) != 0 || withoutPools.Records[0].UnassignedValue != 36 {
			t.Fatal("global unmeasured fan allocation lost its unresolved-equipment guard")
		}
		if unresolved.auxiliaryResolvable["fans"] {
			t.Fatal("source-local allocation mutated the global topology guard")
		}
	})
}

func TestEnergyPathReportedFanPoolsRoundingAndSourceScopeProof(t *testing.T) {
	pools := make([]energyPathFanPool, 4)
	for i := range pools {
		pools[i] = energyPathFanPool{Source: EnergyDataSource{ID: fmt.Sprintf("pool.%d", i), SourceType: "sql_report_data"}, AirLoopName: fmt.Sprintf("VAV_%d", i+1), Monthly: map[int]float64{1: float64(i+1) + .0004}}
	}
	nodes := epathFanPoolNodes(pools, "M1")
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epathFanPoolTopology(4), "M1", "monthly", true, pools)
	if len(plan.Records) != 1 || plan.Records[0].poolRoundingBound != .0025 || plan.Records[0].UnassignedValue != 0 || plan.Records[0].OvermappedValue != 0 {
		t.Fatalf("counted rounding accounting missing: %#v", plan.Records)
	}
	rows, _ := appendEnergyPathZoneAuxiliaryAllocationRecords(nil, nil, plan.Records, "M1", true)
	if len(rows) != 1 || rows[0].ResidualValue != .002 || rows[0].Status != "balanced" || !strings.Contains(rows[0].Formula, "counted 3dp bound") {
		t.Fatalf("rounding erased actual residual or its explanation: %#v", rows)
	}
	if pools[0].Monthly[1] != 1.0004 {
		t.Fatal("rounding accounting overwrote observed pool energy")
	}
	nodes[0].Value = 10.005
	nodes[0].EffectiveValue = 10.005
	partial := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epathFanPoolTopology(4), "M1", "monthly", true, pools)
	if partial.Records[0].poolRoundingBound != 0 || partial.Records[0].UnassignedValue != .005 {
		t.Fatal("arbitrary relative tolerance hid unassigned remainder")
	}
	targets := []energyPathZoneAuxiliaryTarget{}
	for i := 0; i < 5; i++ {
		targets = append(targets, energyPathZoneAuxiliaryTarget{NodeID: fmt.Sprint(i), ZoneName: fmt.Sprint(i), Value: 1})
	}
	shares := energyPathFanPoolShares(.0025, targets)
	sum := 0.0
	for _, value := range shares {
		if value < 0 {
			t.Fatal("tiny pool emitted negative allocation")
		}
		sum = roundedEnergyNumber(sum + value)
	}
	if len(shares) != 5 || sum != .003 {
		t.Fatalf("tiny pool escaped its fixed 3dp budget: %v", shares)
	}
	pool := energyPathFanPool{Source: EnergyDataSource{ID: "zero.pool", SourceType: "sql_report_data", observedValuePresence: energySourceObservedRaw | energySourceObservedEffective}, Monthly: map[int]float64{}}
	for month := 1; month <= 12; month++ {
		pool.Monthly[month] = 0
	}
	building := appendEnergyPathFanPoolSources(nil, []energyPathFanPool{pool}, energyPathZoneAuxiliaryAllocationPlan{}, EnergyExplanationScope{Kind: "building"})
	wire, _ := json.Marshal(energyPathSourceWire{building[0]})
	if !strings.Contains(string(wire), `"rawValue":0`) || !strings.Contains(string(wire), `"effectiveValue":0`) {
		t.Fatalf("observed pool zero became unknown: %s", wire)
	}
	// Deliberately carry a Building proof in the original source; the scoped
	// allocation must explicitly clear it instead of inventing Zone raw zero.
	pool.Source.inspectorDecodedFromJSON = true
	pool.Source.inspectorValuePresence = energySourceObservedRaw | energySourceObservedEffective
	scoped := appendEnergyPathFanPoolSources(nil, []energyPathFanPool{pool}, energyPathZoneAuxiliaryAllocationPlan{FanSourceAllocations: []energyPathFanSourceAllocation{{SourceID: "zero.pool", ZoneName: "Office", Value: 0, Method: "air_loop_load_share"}}}, EnergyExplanationScope{Kind: "zone", ZoneName: "Office"})
	wire, _ = json.Marshal(energyPathSourceWire{scoped[0]})
	if strings.Contains(string(wire), `"rawValue"`) || strings.Contains(string(wire), `"effectiveValue"`) || !strings.Contains(string(wire), `"allocationApplied":true`) {
		t.Fatalf("Building pool proof invented raw individual-Zone energy: %s", wire)
	}
}
