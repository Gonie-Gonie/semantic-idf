package simulation

import (
	"database/sql"
	"encoding/json"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type thermalInputFixture struct {
	path     string
	geometry idf.GeometryReport
	doc      idf.Document
	context  energyDriverBuildContext
}

// The RunPeriod crosses a month boundary: Jan 31 and Feb 1-2, not two full
// calendar months. Monthly SQL Interval deliberately retains calendar lengths.
func newThermalInputFixture(t *testing.T) thermalInputFixture {
	t.Helper()
	doc := parsePurposePlanFixture(t, "Version,25.1; Zone,Office,0,0,0,0,1,2; Zone,Lab,0,0,0,0,1,1; ZoneList,Repeated,Office; ZoneGroup,Repeat,Repeated,3;")
	geometry := idf.GeometryReport{
		Zones: []idf.GeometryZone{{ID: "z1", Name: "Office"}, {ID: "z2", Name: "Lab"}},
		Surfaces: []idf.GeometrySurface{
			{ID: "s1", Name: "Office Wall", ZoneName: "Office", SurfaceType: "Wall", OutsideBoundary: "Outdoors", Area: 9999, PhysicalArea: 14},
			{ID: "s2", Name: "Lab Wall", ZoneName: "Lab", SurfaceType: "Wall", OutsideBoundary: "Outdoors", Area: 9999, PhysicalArea: 8},
			{ID: "s3", Name: "Ground", ZoneName: "Office", SurfaceType: "Floor", OutsideBoundary: "Ground"},
			{ID: "s4", Name: "Partition", ZoneName: "Office", SurfaceType: "Wall", OutsideBoundary: "Surface"},
			{ID: "s5", Name: "Adiabatic", ZoneName: "Office", SurfaceType: "Wall", OutsideBoundary: "Adiabatic"},
		},
		Windows: []idf.GeometryWindow{{ID: "w1", Name: "Window", ZoneName: "Office", BaseSurfaceID: "s1", Multiplier: 1, PhysicalArea: 4}},
	}
	f := thermalInputFixture{path: filepath.Join(t.TempDir(), "eplusout.sql"), geometry: geometry, doc: doc, context: newEnergyDriverBuildContext(geometry, doc)}
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,3),(1,1)`,
		`CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,Interval REAL,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,KeyValue TEXT,Name TEXT,Units TEXT,ReportingFrequency TEXT,IsMeter INTEGER)`,
		`CREATE TABLE ReportData(TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
		`CREATE TABLE Zones(ZoneIndex INTEGER PRIMARY KEY,ZoneName TEXT,Multiplier REAL,ListMultiplier REAL)`,
		`INSERT INTO Zones VALUES(1,'Office',2,3),(2,'Lab',1,1)`,
		`CREATE TABLE Surfaces(SurfaceIndex INTEGER PRIMARY KEY,SurfaceName TEXT,Area REAL,ZoneIndex INTEGER,ExtBoundCond INTEGER,HeatTransferSurf INTEGER)`,
		`INSERT INTO Surfaces VALUES(1,'Office Wall',10,1,0,1),(2,'Lab Wall',8,2,0,1),(3,'Window',4,1,0,1),(4,'Ground',100,1,-1,1),(5,'Partition',100,1,2,1),(6,'Adiabatic',100,1,6,1)`,
		`INSERT INTO Time VALUES(101,2017,1,31,24,0,44640,3,3,NULL),(102,2017,2,28,24,0,40320,3,3,NULL)`,
	} {
		exec(q)
	}
	start := time.Date(2017, 1, 31, 0, 0, 0, 0, time.UTC)
	for day := 0; day < 3; day++ {
		date := start.AddDate(0, 0, day)
		exec(`INSERT INTO Time VALUES(?,2017,?,?,24,0,1440,2,3,0)`, day+1, int(date.Month()), date.Day())
		for hour := 1; hour <= 24; hour++ {
			exec(`INSERT INTO Time VALUES(?,2017,?,?,?,0,60,1,3,0)`, 1000+day*24+hour, int(date.Month()), date.Day(), hour)
		}
	}
	// Design-day records are not weather-run coverage and must not poison it.
	exec(`INSERT INTO Time VALUES(9000,2017,2,1,24,0,1440,2,1,0)`)
	dictionary := func(index int, key, name, unit, freq string) {
		exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,?,0)`, index, key, name, unit, freq)
	}
	for i, key := range []string{"Office Wall", "Lab Wall", "Window"} {
		dictionary(i+1, key, "Surface Outside Face Incident Solar Radiation Rate per Area", "W/m2", "Daily")
		value := 100.0
		if key == "Lab Wall" {
			value = 50
		}
		for day := 1; day <= 3; day++ {
			exec(`INSERT INTO ReportData VALUES(?,?,?)`, day, i+1, value)
		}
		exec(`INSERT INTO ReportData VALUES(9000,?,999999)`, i+1)
	}
	for kind, definition := range energyPathThermalInputDefinitions {
		if definition.incident {
			continue
		}
		keys := []string{"Office Wall", "Lab Wall"}
		if definition.internal {
			keys = []string{"Office", "Lab"}
		}
		for z, key := range keys {
			index := 10 + kind*2 + z
			dictionary(index, key, definition.variable, "J", "Monthly")
			vals := [2]float64{0, 0}
			if definition.kind == "input.exterior_convection" {
				if z == 0 {
					vals = [2]float64{-3.6e6, 7.2e6}
				} else {
					vals = [2]float64{0, -3.6e6}
				}
			}
			if definition.kind == "input.surface_storage" && z == 0 {
				vals = [2]float64{1.8e6, -.9e6}
			}
			if definition.internal {
				if z == 0 {
					vals = [2]float64{36e6, 72e6}
				} else {
					vals = [2]float64{7.2e6, 0}
				}
			}
			for m, value := range vals {
				exec(`INSERT INTO ReportData VALUES(?,?,?)`, 101+m, index, value)
			}
		}
	}
	// Monthly averages are intentionally enormous. They cannot replace the
	// observed Daily integration, or be added to it as a second physical source.
	dictionary(80, "Office Wall", "Surface Outside Face Incident Solar Radiation Rate per Area", "W/m2", "Monthly")
	exec(`INSERT INTO ReportData VALUES(101,80,999999),(102,80,999999)`)
	for i, key := range []string{"Ground", "Partition", "Adiabatic"} {
		dictionary(90+i, key, "Surface Outside Face Incident Solar Radiation Rate per Area", "W/m2", "Daily")
		for day := 1; day <= 3; day++ {
			exec(`INSERT INTO ReportData VALUES(?,?,999999)`, day, 90+i)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f thermalInputFixture) read(t *testing.T) ([]energyPathThermalInputValue, []EnergyDataSource, []int) {
	t.Helper()
	values, sources, months, err := readEnergyPathThermalInputs(f.path, nil, f.context, &f.geometry)
	if err != nil {
		t.Fatal(err)
	}
	return values, sources, months
}

func thermalInputAssertValue(t *testing.T, values []energyPathThermalInputValue, months []int, zone, period, kind, category string, want float64, observed bool) {
	t.Helper()
	for _, value := range energyPathThermalInputGraphValues(values, months, zone, period) {
		if value.kind == kind && value.category == category {
			if !observed || math.Abs(value.effective-want) > 1e-9 {
				t.Fatalf("%s/%s/%s/%s = %.12g; want %.12g observed=%v", zone, period, kind, category, value.effective, want, observed)
			}
			return
		}
	}
	if observed {
		t.Fatalf("missing %s/%s/%s/%s, want %.12g", zone, period, kind, category, want)
	}
}

func TestEnergyPathThermalInputsNativePeriodsMultipliersAndContext(t *testing.T) {
	f := newThermalInputFixture(t)
	beforeGeometry, _ := json.Marshal(f.geometry)
	values, sources, months := f.read(t)
	if !reflect.DeepEqual(months, []int{1, 2}) {
		t.Fatalf("periods=%v", months)
	}
	wall := energyDriverCategoryExteriorWalls
	thermalInputAssertValue(t, values, months, "Office", "M1", "input.solar_incident", wall, 144, true)
	thermalInputAssertValue(t, values, months, "Office", "M2", "input.solar_incident", wall, 288, true)
	thermalInputAssertValue(t, values, months, "", "Annual", "input.solar_incident", wall, 460.8, true)
	thermalInputAssertValue(t, values, months, "", "annual", "input.solar_incident", wall, 460.8, true)
	thermalInputAssertValue(t, values, months, "Office", "M1", "input.solar_incident", energyDriverCategoryWindowsDoors, 57.6, true)
	thermalInputAssertValue(t, values, months, "", "Annual", "input.exterior_convection", wall, 5, true)
	thermalInputAssertValue(t, values, months, "Office", "M1", "input.surface_storage", wall, 3, true)
	thermalInputAssertValue(t, values, months, "Office", "M2", "input.surface_storage", wall, -1.5, true)
	thermalInputAssertValue(t, values, months, "", "Annual", "input.internal_gains", "", 182, true)
	thermalInputAssertValue(t, values, months, "Lab", "M1", "input.exterior_convection", wall, 0, true)
	afterGeometry, _ := json.Marshal(f.geometry)
	if string(beforeGeometry) != string(afterGeometry) {
		t.Fatal("shared geometry mutated")
	}
	for _, source := range sources {
		if source.HourlyEnergy != nil || !strings.HasPrefix(source.ID, "sql-input-rdd-") || source.DriverRole != "context" {
			t.Fatalf("unsafe source=%+v", source)
		}
		if source.KeyValue == "Office Wall" && source.Name == energyPathThermalInputDefinitions[0].variable {
			if source.RawValue != 72 || source.EffectiveValue != 432 || source.EffectiveMultiplier != 6 {
				t.Fatalf("solar source=%+v", source)
			}
		}
	}
	driver := EnergyExplanationNode{ID: "driver-wall", Level: "driver", Kind: "driver.surface.exterior_walls", DriverCategory: wall, Value: 77, AllocatedValue: 77, Unit: "kWh", ScaleDomain: "thermal", Period: "Annual", SourceIDs: []string{"core"}}
	internal := EnergyExplanationNode{ID: "driver-people", Level: "driver", DriverCategory: energyDriverCategoryPeople, Value: 5, Unit: "kWh", ScaleDomain: "thermal", Period: "Annual", SourceIDs: []string{"people"}}
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Nodes: []EnergyExplanationNode{driver, internal}, Periods: []EnergyPeriod{{ID: "M1", Nodes: []EnergyExplanationNode{driver, internal}}, {ID: "M2", Nodes: []EnergyExplanationNode{driver, internal}}}, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, Nodes: []EnergyExplanationNode{driver, internal}, Periods: []EnergyPeriod{{ID: "M1", Nodes: []EnergyExplanationNode{driver, internal}}}}}}
	result.Periods = append(result.Periods, EnergyPeriod{ID: "annual", Kind: "annual", Nodes: []EnergyExplanationNode{driver, internal}})
	for i := range result.Periods {
		for j := range result.Periods[i].Nodes {
			result.Periods[i].Nodes[j].Period = result.Periods[i].ID
		}
	}
	for i := range result.ZoneResults {
		z := &result.ZoneResults[i]
		for j := range z.Nodes {
			z.Nodes[j].ZoneName = z.Scope.ZoneName
		}
		for p := range z.Periods {
			for j := range z.Periods[p].Nodes {
				z.Periods[p].Nodes[j].ZoneName = z.Scope.ZoneName
				z.Periods[p].Nodes[j].Period = z.Periods[p].ID
			}
		}
	}
	appendEnergyPathThermalInputGraphs(&result, values, months)
	if !reflect.DeepEqual(result.Nodes[:2], []EnergyExplanationNode{driver, internal}) {
		t.Fatal("boundary input changed core drivers")
	}
	ids := map[string]bool{}
	for _, node := range result.Nodes {
		if node.Level == "input" {
			ids[node.ID] = true
			if node.Value < 0 || node.ScaleDomain != "boundary" || node.Basis != "reported_boundary" {
				t.Fatalf("node=%+v", node)
			}
		}
	}
	for _, node := range result.Periods[0].Nodes {
		if node.Level == "input" && !ids[node.ID] {
			t.Fatalf("period changed physical node identity: %s", node.ID)
		}
	}
	if len(result.Periods[2].Nodes) != len(result.Nodes) || len(result.Periods[2].Links) != len(result.Links) {
		t.Fatal("lowercase annual wrapper lost root thermal input graph")
	}
	for _, link := range result.Links {
		if link.Relation != "input_to_driver" || link.Basis != "boundary_context" || link.Ratio != 0 || len(link.SourceIDs) < 2 {
			t.Fatalf("context became flow/ratio: %+v", link)
		}
		if link.ToID == driver.ID && link.ToValue != 77 {
			t.Fatal("driver allocation overwritten")
		}
	}
	result.Sources = append(sources, EnergyDataSource{ID: "core"}, EnergyDataSource{ID: "people"})
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, raw := range decoded["sources"].([]any) {
		source := raw.(map[string]any)
		if source["name"] == "Surface Outside Face Net Thermal Radiation Heat Gain Energy" {
			if _, ok := source["rawValue"]; !ok {
				t.Fatal("observed zero source lost presence")
			}
		}
	}
	zero, cancellation := false, false
	for _, raw := range decoded["nodes"].([]any) {
		node := raw.(map[string]any)
		if node["level"] != "input" {
			continue
		}
		if node["kind"] == "input.exterior_longwave" {
			zero = node["signedValue"] == float64(0) && node["rawValue"] == float64(0) && node["effectiveValue"] == float64(0)
		}
		if node["kind"] == "input.exterior_convection" {
			// Office raw +1 and Lab raw -1 cancel; applying their proven
			// multipliers first leaves +5, which must survive JSON transport.
			cancellation = node["rawValue"] == float64(0) && node["signedValue"] == float64(5) && node["effectiveValue"] == float64(5)
		}
	}
	if !zero || !cancellation {
		t.Fatalf("lost zero/cancellation boundary values: zero=%v cancellation=%v", zero, cancellation)
	}
	var roundTrip EnergyExplanationResult
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	contextLinks := 0
	for _, link := range roundTrip.Links {
		if link.Relation == "input_to_driver" {
			contextLinks++
		}
	}
	if contextLinks == 0 {
		t.Fatal("valid boundary context lost all source-proven links on JSON round-trip")
	}
	for _, node := range roundTrip.Nodes {
		if node.Level == "input" && node.Kind == "input.exterior_convection" && node.SignedValue != 5 {
			t.Fatalf("boundary normalization changed signed input: %+v", node)
		}
	}
}

func TestEnergyPathThermalInputsMissingAmbiguousAndWrongBasisFailClosed(t *testing.T) {
	for _, test := range []struct {
		name, query, kind, category string
		zone                        string
		period                      string
	}{
		{"missing Daily row", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=1 AND TimeIndex=3`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"BLOB orphan cannot fill missing observation", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=1 AND TimeIndex=3; INSERT INTO ReportData VALUES(x'33',1,100)`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"null Daily row", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=1 AND TimeIndex=3`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"duplicate Daily row", `INSERT INTO ReportData VALUES(3,1,100)`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"native Daily hole", `DELETE FROM Time WHERE TimeIndex=2`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"short Daily interval", `UPDATE Time SET Interval=720 WHERE TimeIndex=3`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"invalid calendar", `UPDATE Time SET Day=30 WHERE TimeIndex=3`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"Monthly density cannot stand in", `DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=1`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"unproved area", `UPDATE Surfaces SET Area=NULL WHERE SurfaceName='Office Wall'`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"wrong SQL owner", `UPDATE Surfaces SET ZoneIndex=2 WHERE SurfaceName='Office Wall'`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"wrong executed multiplier", `UPDATE Zones SET Multiplier=99 WHERE ZoneName='Office'`, "input.internal_gains", "", "Office", "M2"},
		{"missing native Monthly energy", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=12 AND TimeIndex=102`, "input.exterior_convection", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"known density with wrong unit", `UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=1`, "input.solar_incident", energyDriverCategoryExteriorWalls, "Office", "M2"},
		{"duplicate Monthly timestamp", `INSERT INTO Time VALUES(999,2017,2,2,24,0,40320,3,3,NULL)`, "input.internal_gains", "", "Office", "M2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newThermalInputFixture(t)
			db, err := sql.Open("sqlite", f.path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(test.query); err != nil {
				t.Fatal(err)
			}
			db.Close()
			values, _, months := f.read(t)
			thermalInputAssertValue(t, values, months, test.zone, test.period, test.kind, test.category, 0, false)
			thermalInputAssertValue(t, values, months, "", test.period, test.kind, test.category, 0, false)
			thermalInputAssertValue(t, values, months, "", "Annual", test.kind, test.category, 0, false)
		})
	}
	// A missing Month is not allowed to erase an independently observed zero.
	f := newThermalInputFixture(t)
	db, _ := sql.Open("sqlite", f.path)
	db.Exec(`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=12 AND TimeIndex=102`)
	db.Close()
	values, _, months := f.read(t)
	thermalInputAssertValue(t, values, months, "Lab", "M1", "input.exterior_convection", energyDriverCategoryExteriorWalls, 0, true)
	thermalInputAssertValue(t, values, months, "Office", "M1", "input.exterior_convection", energyDriverCategoryExteriorWalls, -6, true)
	thermalInputAssertValue(t, values, months, "Office", "M2", "input.solar_incident", energyDriverCategoryExteriorWalls, 288, true)
	// Executed SQL net Area already includes the opening multiplier. These
	// exact areas reproduce the native 22.1 multiplier=2 smoke finding, even
	// though the compatibility geometry.Area deliberately has another basis.
	f.geometry.Windows[0].Multiplier = 2
	db, _ = sql.Open("sqlite", f.path)
	if _, err := db.Exec(`ALTER TABLE Surfaces ADD COLUMN GrossArea REAL; UPDATE Surfaces SET Area=20,GrossArea=10 WHERE SurfaceName='Window'; UPDATE Surfaces SET Area=49.67728,GrossArea=69.67728 WHERE SurfaceName='Office Wall'`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	values, _, months = f.read(t)
	thermalInputAssertValue(t, values, months, "Office", "M1", "input.solar_incident", energyDriverCategoryWindowsDoors, 288, true)
	thermalInputAssertValue(t, values, months, "Office", "M1", "input.solar_incident", energyDriverCategoryExteriorWalls, 715.352832, true)
	t.Run("duplicate dictionary index has no zero authority", func(t *testing.T) {
		f := newThermalInputFixture(t)
		db, err := sql.Open("sqlite", f.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`ALTER TABLE ReportDataDictionary RENAME TO OriginalDictionary; CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER,KeyValue TEXT,Name TEXT,Units TEXT,ReportingFrequency TEXT,IsMeter INTEGER); INSERT INTO ReportDataDictionary SELECT * FROM OriginalDictionary; INSERT INTO ReportDataDictionary VALUES(1,'Lab Wall','Surface Outside Face Incident Solar Radiation Rate per Area','W/m2','Daily',0)`); err != nil {
			t.Fatal(err)
		}
		db.Close()
		if _, _, _, err := readEnergyPathThermalInputs(f.path, nil, f.context, &f.geometry); err == nil {
			t.Fatal("duplicate SQL dictionary index acquired known-zero authority")
		}
	})
}

func TestEnergyPathThermalInputStoredContextGuards(t *testing.T) {
	input := EnergyExplanationNode{ID: "input-wall", Level: "input", Kind: "input.exterior_convection", DriverCategory: energyDriverCategoryExteriorWalls,
		ScaleDomain: "boundary", Basis: "reported_boundary", Value: 6, SignedValue: -6, RawValue: -1, EffectiveValue: -6, Unit: "kWh", Period: "annual", SourceIDs: []string{"sql-input-rdd-12"}}
	driver := EnergyExplanationNode{ID: "wall", Level: "driver", DriverCategory: energyDriverCategoryExteriorWalls, ScaleDomain: "thermal", Value: 3, Unit: "kWh", Period: "annual", SourceIDs: []string{"core"}}
	source := EnergyDataSource{ID: "sql-input-rdd-12", Name: "Surface Outside Face Convection Heat Gain Energy", DriverRole: "context", AggregationBasis: "reported_boundary", NormalizedUnit: "kWh", RawValue: -1, EffectiveValue: -6, EffectiveMultiplier: 6, observedValuePresence: energySourceObservedRaw | energySourceObservedEffective}
	valid := EnergyPathLink{ID: "boundary", FromID: input.ID, ToID: driver.ID, Relation: "input_to_driver", Basis: "boundary_context", FromValue: -6, ToValue: 3, FromUnit: "kWh", ToUnit: "kWh", Period: "annual", SourceIDs: []string{source.ID, "core"}}
	other := EnergyPathLink{ID: "unchanged", Relation: "driver_to_load"}
	scope := EnergyExplanationScope{Kind: "building"}
	for _, test := range []struct {
		name   string
		mutate func(*EnergyExplanationNode, *EnergyExplanationNode, *EnergyDataSource, *EnergyPathLink)
	}{
		{"wrong signed value", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) { l.FromValue = 6 }},
		{"wrong allocation", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) { l.ToValue = 4 }},
		{"cross period", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) { l.Period = "M1" }},
		{"cross zone", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) { l.ZoneName = "Office" }},
		{"not thermal", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) { d.ScaleDomain = "site" }},
		{"not boundary", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) { i.ScaleDomain = "thermal" }},
		{"causal ratio", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) { l.Ratio = .5 }},
		{"ratio label", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) { l.RatioLabel = "COP" }},
		{"missing endpoint source", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) {
			l.SourceIDs = []string{s.ID}
		}},
		{"phantom source", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) {
			l.SourceIDs = []string{s.ID, "core", "unknown"}
		}},
		{"no raw authority", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) { s.observedValuePresence = 0 }},
		{"wrong native quantity", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) {
			s.Name = "Zone People Total Heating Energy"
		}},
		{"stored missing signed zero", func(i, d *EnergyExplanationNode, s *EnergyDataSource, l *EnergyPathLink) {
			i.inspectorDecodedFromJSON = true
			i.inspectorValuePresence = 0
			i.Value = 0
			i.SignedValue = 0
			l.FromValue = 0
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			i, d, s, l := input, driver, source, valid
			test.mutate(&i, &d, &s, &l)
			got := filterEnergyPathThermalInputLinks([]EnergyExplanationNode{i, d}, []EnergyPathLink{l, other}, []EnergyDataSource{s, {ID: "core"}}, scope, "annual")
			if len(got) != 1 || got[0].ID != other.ID {
				t.Fatalf("invalid context survived: %+v", got)
			}
		})
	}
	got := filterEnergyPathThermalInputLinks([]EnergyExplanationNode{input, driver}, []EnergyPathLink{valid, other}, []EnergyDataSource{source, {ID: "core"}}, scope, "annual")
	if len(got) != 2 || got[0].FromValue != -6 {
		t.Fatalf("valid signed context lost: %+v", got)
	}
	got = filterEnergyPathThermalInputLinks([]EnergyExplanationNode{input, driver}, []EnergyPathLink{valid, valid}, []EnergyDataSource{source, {ID: "core"}}, scope, "annual")
	if len(got) != 0 {
		t.Fatalf("ambiguous duplicate context survived: %+v", got)
	}
}

func TestEnergyPathThermalInputsHourlyFallbackAndDailyAuthority(t *testing.T) {
	f := newThermalInputFixture(t)
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(81,'Office Wall','Surface Outside Face Incident Solar Radiation Rate per Area','W/m2','Hourly',0); INSERT INTO ReportData SELECT TimeIndex,81,1000 FROM Time WHERE IntervalType=1 AND EnvironmentPeriodIndex=3`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	values, _, months := f.read(t)
	thermalInputAssertValue(t, values, months, "Office", "M2", "input.solar_incident", energyDriverCategoryExteriorWalls, 288, true)
	db, _ = sql.Open("sqlite", f.path)
	db.Exec(`DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=1`)
	db.Close()
	values, _, months = f.read(t)
	thermalInputAssertValue(t, values, months, "Office", "M2", "input.solar_incident", energyDriverCategoryExteriorWalls, 2880, true)
	db, _ = sql.Open("sqlite", f.path)
	db.Exec(`UPDATE Time SET Interval=30 WHERE TimeIndex=1072`)
	db.Close()
	values, _, months = f.read(t)
	thermalInputAssertValue(t, values, months, "Office", "M2", "input.solar_incident", energyDriverCategoryExteriorWalls, 0, false)
}

func TestEnergyPathThermalInputRequestsUseNativeDailyAndMonthlyOnly(t *testing.T) {
	f := newThermalInputFixture(t)
	builder := newPurposePlanBuilder(f.doc, NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}}))
	builder.geometry = f.geometry
	builder.addEnergyPathThermalInputOutputs([]string{"Office", "Lab"})
	builder.addEnergyPathHourlyOutputs()
	incident, internal, storage := 0, 0, 0
	for _, object := range builder.objects {
		if !energyPathThermalInputVariable(object.VariableName) {
			continue
		}
		if object.ReportingFrequency == "Hourly" {
			t.Fatalf("boundary context created extra Hourly roster: %+v", object)
		}
		if strings.Contains(object.VariableName, "Incident Solar") {
			incident++
			if object.ReportingFrequency != "Daily" {
				t.Fatal("unsafe monthly solar average")
			}
		} else if object.ReportingFrequency != "Monthly" {
			t.Fatal("expected native Monthly energy")
		}
		if strings.Contains(object.VariableName, "Internal Total") {
			internal++
		}
		if object.VariableName == "Surface Heat Storage Energy" {
			storage++
		}
		if object.KeyValue == "Ground" || object.KeyValue == "Partition" || object.KeyValue == "Adiabatic" {
			t.Fatalf("non-exterior thermal input requested: %+v", object)
		}
	}
	if incident != 3 || internal != 2 || storage != 2 {
		t.Fatalf("roster incident=%d internal=%d storage=%d", incident, internal, storage)
	}
	for _, existing := range []string{"", "Output:Diagnostics,DisplayExtraWarnings;"} {
		doc := parsePurposePlanFixture(t, f.doc.String()+existing)
		request := NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy, SimulationPurposeIntegrity}})
		plan := BuildPurposeRunPlan(doc, request)
		updated, preview := idf.ApplyOutput(doc, PurposeRunPlanApplyRequest(plan))
		if !preview.CanApply {
			t.Fatalf("diagnostics apply rejected: %+v", preview)
		}
		count := 0
		flags := map[string]bool{}
		for _, object := range updated.Objects {
			if strings.EqualFold(object.Type, "Output:Diagnostics") {
				count++
				for _, field := range object.Fields {
					flags[field.Value] = true
				}
			}
		}
		if count != 1 || !flags["DisplayAdvancedReportVariables"] || !flags["DisplayExtraWarnings"] {
			t.Fatalf("unique merged diagnostics count=%d flags=%v", count, flags)
		}
	}
}
