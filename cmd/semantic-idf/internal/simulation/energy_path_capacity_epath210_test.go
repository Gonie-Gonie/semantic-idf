package simulation

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Synthetic monthly capacity and source-preservation evidence, not a real
// EnergyPlus physical-model approval or a timing SLA. Setup is outside the
// measured parser/build stages. The production upgrade precomputes all Zones.
func TestEPATH210ThousandSurfaceMonthlyCapacity(t *testing.T) {
	const surfaceCount, zoneCount = 1000, 20
	path, doc := epath210CapacityFixture(t, surfaceCount, zoneCount)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wantHash := sha256.Sum256(before)
	sqlBytes := len(before)
	before = nil
	setupStart := time.Now()
	geometry := idf.AnalyzeGeometry(doc)
	context := newEnergyDriverBuildContext(geometry, doc)
	if len(geometry.Surfaces) != surfaceCount {
		t.Fatalf("geometry surfaces = %d", len(geometry.Surfaces))
	}
	contextElapsed := time.Since(setupStart)
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
	runtime.GC()
	var initial, built, retained runtime.MemStats
	runtime.ReadMemStats(&initial)
	started := time.Now()
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, context)
	parseElapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	buildStart := time.Now()
	legacy := buildEnergyExplanationResultWithDriverContext(parsed.Series, parsed.Sources, plan, context)
	result := UpgradeEnergyExplanationV1(legacy)
	buildElapsed := time.Since(buildStart)
	encodeStart := time.Now()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	encodeElapsed := time.Since(encodeStart)
	runtime.ReadMemStats(&built)
	runtime.GC()
	runtime.ReadMemStats(&retained)
	runtime.KeepAlive(parsed)
	runtime.KeepAlive(legacy)
	runtime.KeepAlive(result)
	runtime.KeepAlive(encoded)
	t.Logf("%s/%s %s GOMAXPROCS=%d: surfaces=%d zones=%d months=12 dictionaries=%d rows=%d SQL=%d bytes", runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.GOMAXPROCS(0), surfaceCount, zoneCount, surfaceCount+zoneCount*2+3, (surfaceCount+zoneCount*2+3)*12, sqlBytes)
	t.Logf("context=%s canonical SQL parse=%s graph build including precomputed Zones=%s JSON=%s; allocated=%d bytes; live heap before=%d after-GC=%d bytes (not peak/RSS); JSON=%d bytes sources=%d periods=%d ZoneResults=%d", contextElapsed, parseElapsed, buildElapsed, encodeElapsed, built.TotalAlloc-initial.TotalAlloc, initial.HeapAlloc, retained.HeapAlloc, len(encoded), len(result.Sources), len(result.Periods), len(result.ZoneResults))
	if len(result.ZoneResults) != zoneCount || len(result.Periods) != 13 {
		t.Fatalf("missing precomputed monthly/Zone results: %d / %d", len(result.Periods), len(result.ZoneResults))
	}
	if len(parsed.HourlyLabels) != 0 || len(parsed.HVACConsumptionPools) != 0 || len(parsed.VRFConsumption) != 0 || len(parsed.FanPools) != 0 {
		t.Fatal("monthly-only surface fixture invented hourly/shared equipment evidence")
	}
	aggregates := map[string]energyExplanationSeries{}
	for _, series := range parsed.Series {
		if !series.parseCategoryAggregate {
			continue
		}
		if _, exists := aggregates[series.ZoneName]; exists {
			t.Fatalf("duplicate Zone surface category %s", series.ZoneName)
		}
		aggregates[series.ZoneName] = series
	}
	if len(aggregates) != zoneCount {
		t.Fatalf("category count = %d", len(aggregates))
	}
	for zone := 0; zone < zoneCount; zone++ {
		name := fmt.Sprintf("Capacity Zone %02d", zone)
		series := aggregates[name]
		want := 0.0
		for offset := 0; offset < surfaceCount/zoneCount; offset++ {
			want += epath210SurfaceValue(zone*(surfaceCount/zoneCount)+offset+1, 1)
		}
		if series.DriverCategory != energyDriverCategoryExteriorWalls || len(series.SourceIDs) != surfaceCount/zoneCount || len(series.Monthly) != 12 || series.Total != want*78 {
			t.Fatalf("category/source/annual mismatch for %s: %#v", name, series)
		}
		epath210ExactSources(t, series.SourceIDs, zone*(surfaceCount/zoneCount)+1, surfaceCount/zoneCount)
		for month := 1; month <= 12; month++ {
			if series.Monthly[month] != want*float64(month) {
				t.Fatalf("%s M%d = %g, want %g", name, month, series.Monthly[month], want*float64(month))
			}
		}
	}
	for pass := 0; pass < 3; pass++ {
		if pass > 0 {
			wire, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var decoded EnergyExplanationResult
			if err := json.Unmarshal(wire, &decoded); err != nil {
				t.Fatal(err)
			}
			result = decoded
		}
		epath210CapacityGraphs(t, result, surfaceCount, zoneCount)
		epath210CapacitySources(t, result.Sources, surfaceCount, zoneCount)
		zeros := 0
		for _, source := range result.Sources {
			if !strings.HasPrefix(source.KeyValue, "Capacity Surface ") {
				continue
			}
			if !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) {
				t.Fatalf("pass%d lost observed presence %s", pass, source.ID)
			}
			if source.RawValue == 0 {
				zeros++
				if source.EffectiveValue != 0 {
					t.Fatal("known zero changed effective value")
				}
			}
		}
		if zeros != surfaceCount/3 {
			t.Fatalf("pass%d known zero sources=%d", pass, zeros)
		}
	}
	// Source indexes are usable for original row retrieval; the graph never
	// needs to carry those raw rows or duplicate dictionary metadata per month.
	db, err := openSimulationSQLiteReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{1, 2, 3, surfaceCount} {
		rows, err := QueryReportData(db, SQLSeriesQuery{DictionaryIndexes: []int{index}})
		if err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
		if len(rows) != 12 {
			_ = db.Close()
			t.Fatalf("source %d raw rows = %d", index, len(rows))
		}
		for i, row := range rows {
			if row.DictionaryIndex != index || !row.Value.Valid || !row.Month.Valid || row.Month.Int64 != int64(i+1) || row.Value.Float64 != epath210SurfaceValue(index, i+1)*3_600_000 {
				_ = db.Close()
				t.Fatalf("raw observation changed: %#v", row)
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	var wire any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	epath210NoRawRowPayload(t, wire)
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(after) != wantHash || len(after) != sqlBytes {
		t.Fatalf("read-only capacity check changed SQL: %v", err)
	}
}

func epath210CapacitySources(t *testing.T, sources []EnergyDataSource, surfaceCount, zoneCount int) {
	t.Helper()
	seen := map[string]bool{}
	physical := 0
	for _, source := range sources {
		if seen[source.ID] {
			t.Fatalf("duplicate source metadata %s", source.ID)
		}
		seen[source.ID] = true
		if source.HourlyEnergy != nil {
			t.Fatal("monthly-only SQL fabricated hourly evidence")
		}
		if !strings.HasPrefix(source.KeyValue, "Capacity Surface ") {
			continue
		}
		var number int
		if _, err := fmt.Sscanf(source.KeyValue, "Capacity Surface %d", &number); err != nil {
			t.Fatal(err)
		}
		if number < 1 || number > surfaceCount || source.ID != fmt.Sprintf("sql-rdd-%d", number) || source.SourceUnit != "J" || source.NormalizedUnit != "kWh" || source.RawValue != epath210SurfaceValue(number, 1)*78 || source.EffectiveValue != source.RawValue || source.ZoneName != fmt.Sprintf("Capacity Zone %02d", (number-1)/(surfaceCount/zoneCount)) || !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) {
			t.Fatalf("source authority/owner/presence changed: %#v", source)
		}
		physical++
	}
	if physical != surfaceCount {
		t.Fatalf("physical sources = %d", physical)
	}
}

func epath210ExactSources(t *testing.T, ids []string, first, count int) {
	t.Helper()
	if len(ids) != count {
		t.Fatalf("source count=%d, want%d", len(ids), count)
	}
	want := map[string]bool{}
	for index := first; index < first+count; index++ {
		want[fmt.Sprintf("sql-rdd-%d", index)] = true
	}
	for _, id := range ids {
		if !want[id] {
			t.Fatalf("unexpected/duplicate physical source %s", id)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatal("missing physical source")
	}
}

func epath210CapacityGraphs(t *testing.T, result EnergyExplanationResult, surfaces, zones int) {
	t.Helper()
	byZone := map[string]EnergyExplanationZoneResult{}
	for _, zone := range result.ZoneResults {
		if _, duplicate := byZone[zone.Scope.ZoneName]; duplicate {
			t.Fatalf("duplicate precomputed Zone %s", zone.Scope.ZoneName)
		}
		byZone[zone.Scope.ZoneName] = zone
	}
	if len(byZone) != zones {
		t.Fatalf("precomputed Zone count=%d", len(byZone))
	}
	checkLoads := func(nodes []EnergyExplanationNode, factor float64, firstSource int, sourceCount int) {
		seen := map[string]bool{}
		for _, node := range nodes {
			if node.Level != "load" || node.ServiceKind != "heating" && node.ServiceKind != "cooling" {
				continue
			}
			if seen[node.ServiceKind] || node.Unit != "kWh" || node.Value != 1000*factor {
				t.Fatalf("load changed: %#v", node)
			}
			seen[node.ServiceKind] = true
			if sourceCount == 1 {
				offset := 0
				if node.ServiceKind == "cooling" {
					offset = 1
				}
				epath210ExactSources(t, node.SourceIDs, firstSource+offset, 1)
			}
		}
		if len(seen) != 2 {
			t.Fatal("missing heating/cooling graph loads")
		}
	}
	for _, period := range epath210CapacityPeriods(t, result.Periods) {
		checkLoads(period.Period.Nodes, float64(zones)*period.Factor, 0, zones)
	}
	checkLoads(result.Nodes, float64(zones*78), 0, zones)
	for number := 0; number < zones; number++ {
		name := fmt.Sprintf("Capacity Zone %02d", number)
		zone, exists := byZone[name]
		if !exists || zone.Scope.Kind != "zone" || len(zone.Periods) != 13 {
			t.Fatalf("missing complete precomputed %s", name)
		}
		first, count := number*(surfaces/zones)+1, surfaces/zones
		pressure := 0.0
		for index := first; index < first+count; index++ {
			pressure += epath210SurfaceValue(index, 1)
		}
		service := "heating"
		if pressure < 0 {
			service = "cooling"
		}
		check := func(nodes []EnergyExplanationNode, factor float64) {
			checkLoads(nodes, factor, surfaces+number*2+1, 1)
			found := false
			for _, node := range nodes {
				if node.Level != "driver" || node.DriverCategory != energyDriverCategoryExteriorWalls || node.ServiceKind != service {
					continue
				}
				if found || node.Unit != "kWh" || node.Value != 1000*factor || node.RawValue != math.Abs(pressure)*factor || node.EffectiveValue != node.RawValue {
					t.Fatalf("%s driver changed: %#v", name, node)
				}
				found = true
				epath210ExactSources(t, node.SourceIDs, first, count)
			}
			if !found {
				t.Fatalf("missing %s surface driver", name)
			}
		}
		for _, period := range epath210CapacityPeriods(t, zone.Periods) {
			check(period.Period.Nodes, period.Factor)
		}
		check(zone.Nodes, 78)
	}
}

type epath210CapacityPeriod struct {
	Period EnergyPeriod
	Factor float64
}

func epath210CapacityPeriods(t *testing.T, periods []EnergyPeriod) []epath210CapacityPeriod {
	t.Helper()
	// The persisted contract includes an annual period as well as the twelve
	// monthly periods; annual top-level nodes are a convenience mirror.
	seen := map[string]bool{}
	var out []epath210CapacityPeriod
	for _, period := range periods {
		if seen[period.ID] {
			t.Fatalf("duplicate period %s", period.ID)
		}
		seen[period.ID] = true
		factor := 78.0
		if period.ID == "annual" {
			if period.Kind != "annual" {
				t.Fatal("annual period lost kind")
			}
		} else {
			var month int
			if _, err := fmt.Sscanf(period.ID, "M%d", &month); err != nil || month < 1 || month > 12 || period.ID != fmt.Sprintf("M%d", month) || period.Kind != "monthly" {
				t.Fatalf("invalid monthly period %s", period.ID)
			}
			factor = float64(month)
		}
		out = append(out, epath210CapacityPeriod{period, factor})
	}
	if len(seen) != 13 || !seen["annual"] {
		t.Fatalf("incomplete annual/monthly period census: %v", seen)
	}
	return out
}

func epath210SurfaceValue(index, month int) float64 {
	if index%3 == 0 {
		return 0
	}
	sign := 1.0
	if index%2 == 0 {
		sign = -1
	}
	return sign * float64((index%7+1)*month)
}

func epath210NoRawRowPayload(t *testing.T, value any) {
	t.Helper()
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			switch strings.ToLower(key) {
			case "reportdata", "reportdataindex", "reportdatadictionaryindex", "timeindex", "dictionaryindex":
				t.Fatalf("raw SQL row field in graph JSON: %s", key)
			}
			epath210NoRawRowPayload(t, child)
		}
	case []any:
		for _, child := range item {
			epath210NoRawRowPayload(t, child)
		}
	}
}

func epath210CapacityFixture(t *testing.T, surfaces, zones int) (string, idf.Document) {
	t.Helper()
	var input strings.Builder
	input.WriteString("Version,25.1;\nMaterial:NoMass,Capacity Insulation,Rough,2;\nConstruction,Capacity Wall,Capacity Insulation;\n")
	for zone := 0; zone < zones; zone++ {
		fmt.Fprintf(&input, "Zone,Capacity Zone %02d,0,0,0,0,1,1,3,150,50;\n", zone)
	}
	for index := 1; index <= surfaces; index++ {
		zone := (index - 1) / (surfaces / zones)
		x := index * 2
		fmt.Fprintf(&input, "BuildingSurface:Detailed,Capacity Surface %d,Wall,Capacity Wall,Capacity Zone %02d,,Outdoors,,SunExposed,WindExposed,0.5,4,%d,0,0,%d,0,3,%d,0,3,%d,0,0;\n", index, zone, x, x, x+1, x+1)
	}
	doc, err := idf.Parse(input.String())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "capacity.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, query := range []string{
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,KeyValue TEXT,Name TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT)`,
		`CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,Interval REAL,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER)`,
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,3)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	dictionary, err := tx.Prepare(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J',?,'Monthly',?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer dictionary.Close()
	row, err := tx.Prepare(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer row.Close()
	for month := 1; month <= 12; month++ {
		days := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day()
		if _, err := tx.Exec(`INSERT INTO Time VALUES(?,2017,?,?,24,0,?,3,3,0)`, month, month, days, days*1440); err != nil {
			t.Fatal(err)
		}
	}
	add := func(index int, key, name, group string, meter bool, value func(int) float64) {
		if _, err := dictionary.Exec(index, key, name, meter, group); err != nil {
			t.Fatal(err)
		}
		for month := 1; month <= 12; month++ {
			if _, err := row.Exec(month, index, value(month)*3_600_000); err != nil {
				t.Fatal(err)
			}
		}
	}
	for index := 1; index <= surfaces; index++ {
		add(index, fmt.Sprintf("Capacity Surface %d", index), "Surface Inside Face Convection Heat Gain Energy", "Surface", false, func(month int) float64 { return epath210SurfaceValue(index, month) })
	}
	for zone := 0; zone < zones; zone++ {
		for service, name := range []string{"Zone Air System Sensible Heating Energy", "Zone Air System Sensible Cooling Energy"} {
			add(surfaces+zone*2+service+1, fmt.Sprintf("Capacity Zone %02d", zone), name, "Zone", false, func(month int) float64 { return float64(1000 * month) })
		}
	}
	for offset, name := range []string{"Electricity:Facility", "Heating:Electricity", "Cooling:Electricity"} {
		factor := 1000.0
		if offset == 0 {
			factor = 2000
		}
		add(surfaces+zones*2+offset+1, "", name, "Facility", true, func(month int) float64 { return factor * float64(month) })
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return path, doc
}
