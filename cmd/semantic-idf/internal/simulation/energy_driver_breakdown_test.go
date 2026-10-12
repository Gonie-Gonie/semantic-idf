package simulation

import (
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEnergyDriverBreakdownSharedPurposeSQLBuilderRetainsMeasuredFamilies(t *testing.T) {
	dir := t.TempDir()
	inputPath, sqlPath := filepath.Join(dir, "model.idf"), filepath.Join(dir, "eplusout.sql")
	if err := os.WriteFile(inputPath, []byte(`Version,25.1;
Zone,Office,0,0,0,0,1,1;
People,Occupants,Office;
Material:NoMass,Insulation,Rough,2;
Construction,Wall Construction,Insulation;
BuildingSurface:Detailed,Wall,Wall,Wall Construction,Office,,Outdoors,,SunExposed,WindExposed,0.5,4,0,0,0,0,0,3,10,0,3,10,0,0;`), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(1,3)`,
		`CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,Interval REAL,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER)`,
		`INSERT INTO Time VALUES(1,2017,1,31,24,0,44640,3,1,NULL),(2,2017,2,28,24,0,40320,3,1,NULL)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,KeyValue TEXT,Name TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
		`CREATE TABLE Zones(ZoneIndex INTEGER PRIMARY KEY,ZoneName TEXT,Multiplier REAL,ListMultiplier REAL)`,
		`INSERT INTO Zones VALUES(1,'Office',1,1)`,
		`CREATE TABLE Surfaces(SurfaceIndex INTEGER PRIMARY KEY,SurfaceName TEXT,Area REAL,ZoneIndex INTEGER,ExtBoundCond INTEGER,HeatTransferSurf INTEGER)`,
		`INSERT INTO Surfaces VALUES(1,'Wall',30,1,0,1)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for i, item := range []struct {
		key, name, unit string
		kWh             float64
	}{
		{"Office", "Zone Air System Sensible Cooling Energy", "J", 100},
		{"Office", "Zone Air System Sensible Heating Energy", "J", 0},
		{"Office", "Zone People Convective Heating Energy", "J", 10},
		{"Office", "Zone People Total Heating Energy", "J", 15},
		// Native surface/air-storage signs are opposite room driver pressure.
		{"Wall", "Surface Inside Face Convection Heat Gain Energy", "J", -10},
		{"Office", "Zone Air Heat Balance Surface Convection Rate", "W", 30},
		{"Office", "Zone Air Heat Balance Air Energy Storage Rate", "W", -10},
	} {
		group := "Zone"
		if item.key == "Wall" {
			group = "Surface"
		}
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,0,'Monthly',?)`, i+1, item.key, item.name, item.unit, group); err != nil {
			t.Fatal(err)
		}
		for month, hours := range []float64{744, 672} {
			value := item.kWh * 3.6e6
			if item.unit == "W" {
				value = item.kWh * 1000 / hours
			}
			if _, err := db.Exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, i*2+month+1, month+1, i+1, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, AllocationPolicy: PurposeAllocationPolicyDirectOnly, ZoneMode: "all"}
	bundle := BuildPurposeResultBundle(&SimulationRunResult{InputPath: inputPath, PurposeRunPlan: plan, Files: []SimulationFileInfo{{Name: "eplusout.sql", Path: sqlPath, Kind: "sqlite"}}}, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, AllocationPolicy: PurposeAllocationPolicyDirectOnly})
	if len(bundle.EnergyExplanation.Nodes) == 0 {
		t.Fatalf("shared builder produced no graph: %+v", bundle.Completeness)
	}
	check := func(nodes []EnergyExplanationNode, multiplier float64) {
		t.Helper()
		var other *EnergyExplanationNode
		for i := range nodes {
			if nodes[i].Level == "driver" && nodes[i].DriverCategory == energyDriverCategoryStorageOther && nodes[i].ServiceKind == "cooling" {
				other = &nodes[i]
			}
		}
		if other == nil || other.Value != 60*multiplier || len(other.DriverBreakdown) != 2 {
			t.Fatalf("shared builder lost measured balance detail: %+v", other)
		}
		for _, part := range other.DriverBreakdown {
			want := map[string]float64{"air_storage": 20, "surface_balance": 40}[part.Component] * multiplier
			if part.Value != want || len(part.SourceIDs) == 0 {
				t.Fatalf("shared builder component=%+v, want=%v", part, want)
			}
		}
	}
	check(bundle.EnergyExplanation.Nodes, 2)
	for _, period := range bundle.EnergyExplanation.Periods {
		if period.ID == "M1" || period.ID == "M2" {
			check(period.Nodes, 1)
		}
	}
	encoded, err := json.Marshal(bundle.EnergyExplanation)
	if err != nil {
		t.Fatal(err)
	}
	var stored EnergyExplanationResult
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	check(stored.Nodes, 2)
}

func TestEnergyDriverBreakdownPreservesParentAllocationAndMonthlyIdentity(t *testing.T) {
	series := []energyExplanationSeries{}
	add := func(kind, category, source string, values map[int]float64) {
		item := energyExplanationSeries{Stage: "driver", Level: "heat", Kind: kind, CanonicalKind: kind, Label: kind, Unit: "kWh", ZoneName: "Office",
			DriverSourceRole: energyDriverSourceRoleMainFlow, DriverCategory: category, SourceIDs: []string{source}, Monthly: values, RawMonthly: values,
			EffectiveMultiplier: 1, multiplierApplied: true, heatSignMultiplier: 1}
		item.DriverComponent = energyDriverComponentForSeries(item)
		series = append(series, canonicalEnergyExplanationSeries(item))
	}
	add("heat.people", energyDriverCategoryPeople, "people", map[int]float64{1: 10, 2: 10})
	add("heat.storage_air", energyDriverCategoryStorageOther, "storage", map[int]float64{1: 10, 2: 30})
	add("heat.surface_reconciliation", energyDriverCategoryStorageOther, "surface", map[int]float64{1: 30, 2: 10})
	series = append(series, canonicalEnergyExplanationSeries(energyExplanationSeries{Stage: "load", Level: "load", Kind: "load.zone_cooling", CanonicalKind: "load.zone_cooling",
		Label: "Cooling load", Unit: "kWh", ZoneName: "Office", ServiceKind: "cooling", SourceIDs: []string{"load"}, Monthly: map[int]float64{1: 100, 2: 10}, RawMonthly: map[int]float64{1: 100, 2: 10}, EffectiveMultiplier: 1, multiplierApplied: true, canonicalLoadMetadata: true}))
	graphs := map[int]energyExplanationGraph{}
	for month := 1; month <= 2; month++ {
		graphs[month] = buildEnergyExplanationGraphForPeriod("M1", series, PurposeAllocationPolicyDirectOnly, func(item energyExplanationSeries) float64 { return item.Monthly[month] })
	}
	annual := aggregateEnergyExplanationMonthlyGraphs(graphs)
	var original EnergyExplanationNode
	for _, node := range annual.Nodes {
		if node.Level == "heat" && node.DriverCategory == energyDriverCategoryStorageOther {
			original = node
		}
	}
	if original.Value != 88 || original.RawValue != 80 || original.EffectiveValue != 80 {
		t.Fatalf("parent allocation changed: %+v", original)
	}
	want := map[string]float64{"air_storage": 26, "surface_balance": 62}
	for _, component := range original.DriverBreakdown {
		if component.Value != want[component.Component] || component.RawValue != 40 || component.EffectiveValue != 40 || len(component.SourceIDs) != 1 {
			t.Fatalf("monthly detail was reweighted annually: %+v", component)
		}
	}
	if len(original.DriverBreakdown) != 2 {
		t.Fatalf("lost independent balance components: %+v", original.DriverBreakdown)
	}
	canonical := upgradeEnergyExplanationNode(original, EnergyExplanationScope{Kind: "zone", ZoneName: "Office"})
	before := canonical
	normalizeEnergyExplanationV2Nodes([]EnergyExplanationNode{canonical}, EnergyExplanationScope{Kind: "zone", ZoneName: "Office"})
	if !reflect.DeepEqual(canonical, before) {
		t.Fatal("normalization mutated shared component detail")
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip EnergyExplanationNode
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	sanitizeEnergyExplanationDriverComponents(&roundTrip)
	roundTripJSON, _ := json.Marshal(roundTrip.DriverBreakdown)
	canonicalJSON, _ := json.Marshal(canonical.DriverBreakdown)
	if string(roundTripJSON) != string(canonicalJSON) {
		t.Fatalf("JSON lost known component values: %+v", roundTrip.DriverBreakdown)
	}
	clone := cloneEnergyExplanationNodes([]EnergyExplanationNode{canonical})
	clone[0].DriverBreakdown[0].SourceIDs[0] = "changed"
	clone[0].DriverBreakdown[0].Value = 999
	if !reflect.DeepEqual(canonical.DriverBreakdown, original.DriverBreakdown) {
		t.Fatal("clone shared component allocation or provenance")
	}
}

func TestEnergyDriverBreakdownFixedBudgetZeroAndMalformedStoredDetail(t *testing.T) {
	node := EnergyExplanationNode{Level: "driver", DriverCategory: energyDriverCategoryStorageOther, AllocationApplied: true, Value: .001, AllocatedValue: .001, RawValue: 2, EffectiveValue: 2, SourceIDs: []string{"a", "b"},
		DriverBreakdown: []EnergyExplanationDriverComponent{{Component: "air_storage", RawValue: 1, EffectiveValue: 1, SourceIDs: []string{"a"}}, {Component: "surface_balance", RawValue: 1, EffectiveValue: 1, SourceIDs: []string{"b"}}}}
	allocateEnergyExplanationDriverComponents(&node)
	if node.DriverBreakdown[0].Value != .001 || node.DriverBreakdown[1].Value != 0 {
		t.Fatalf("unstable milli-unit subdivision: %+v", node.DriverBreakdown)
	}
	sanitizeEnergyExplanationDriverComponents(&node)
	if len(node.DriverBreakdown) != 2 {
		t.Fatal("known-zero child was pruned")
	}
	for _, mutate := range []func(*EnergyExplanationNode){
		func(n *EnergyExplanationNode) { n.DriverBreakdown[0].Value = 2 },
		func(n *EnergyExplanationNode) { n.DriverBreakdown[0].EffectiveValue = math.NaN() },
		func(n *EnergyExplanationNode) { n.DriverBreakdown[0].RawValue = -1 },
		func(n *EnergyExplanationNode) { n.DriverBreakdown[0].SourceIDs = []string{"foreign"} },
		func(n *EnergyExplanationNode) { n.DriverBreakdown[1].Component = "air_storage" },
		func(n *EnergyExplanationNode) { n.DriverCategory = energyDriverCategoryPeople },
	} {
		copy := cloneEnergyExplanationNodes([]EnergyExplanationNode{node})[0]
		mutate(&copy)
		sanitizeEnergyExplanationDriverComponents(&copy)
		if copy.DriverBreakdown != nil || copy.Value != node.Value {
			t.Fatalf("invalid child acquired authority or changed parent: %+v", copy)
		}
	}
	setEnergyDriverAllocatedContribution(&node, 0)
	if len(node.DriverBreakdown) != 2 || node.DriverBreakdown[0].Value != 0 || node.DriverBreakdown[1].Value != 0 {
		t.Fatal("zero load erased original component pressures")
	}
	legacy := node
	legacy.DriverBreakdown = nil
	sanitizeEnergyExplanationDriverComponents(&legacy)
	if legacy.DriverBreakdown != nil {
		t.Fatal("legacy source metadata manufactured a breakdown")
	}
	for _, stored := range []string{
		`{"component":"unresolved_balance","value":0,"rawValue":0,"sourceIds":["a"]}`,
		`{"component":"unresolved_balance","value":0,"rawValue":0,"effectiveValue":null,"sourceIds":["a"]}`,
	} {
		var component EnergyExplanationDriverComponent
		if err := json.Unmarshal([]byte(stored), &component); err != nil {
			t.Fatal(err)
		}
		copy := EnergyExplanationNode{Level: "driver", DriverCategory: energyDriverCategoryStorageOther, AllocationApplied: true, SourceIDs: []string{"a"}, DriverBreakdown: []EnergyExplanationDriverComponent{component}}
		sanitizeEnergyExplanationDriverComponents(&copy)
		if copy.DriverBreakdown != nil {
			t.Fatal("missing/null component pressure acquired known-zero authority")
		}
	}
}

func TestEnergyDriverBreakdownInterzoneFoldKeepsCompletedBudget(t *testing.T) {
	scope := EnergyExplanationScope{Kind: "building"}
	load := &EnergyExplanationNode{ID: "load.cooling.building", Level: "load", Value: 20, Unit: "kWh", Period: "M1", SourceIDs: []string{"load"}}
	storage := &EnergyExplanationNode{ID: "driver.balance.storage_other.cooling.building", Level: "driver", DriverCategory: energyDriverCategoryStorageOther, Value: 5, AllocatedValue: 5, RawValue: 2, EffectiveValue: 2, SourceIDs: []string{"air"},
		DriverBreakdown: []EnergyExplanationDriverComponent{{Component: "air_storage", Value: 5, RawValue: 2, EffectiveValue: 2, SourceIDs: []string{"air"}}}}
	nodes := map[string]*EnergyExplanationNode{load.ID: load, storage.ID: storage}
	closeBuildingLoadsAfterInterzoneProjection(nodes, map[string]*EnergyPathLink{}, map[string]energyExplanationSuppressedTrace{"cooling": {suppressedValue: 7, retainedValue: 2, sourceIDs: []string{"transfer"}}}, scope)
	sanitizeEnergyExplanationDriverComponents(storage)
	if storage.Value != 10 || len(storage.DriverBreakdown) != 2 {
		t.Fatalf("interzone fold lost complete detail: %+v", storage)
	}
	for _, child := range storage.DriverBreakdown {
		if child.Component == "interzone_balance" && (child.Value != 5 || child.RawValue != 5 || child.EffectiveValue != 5) {
			t.Fatalf("interzone budget reallocated: %+v", child)
		}
	}
}
