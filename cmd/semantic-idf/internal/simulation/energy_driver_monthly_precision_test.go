package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// These are original Monthly/J NORTH ZONE M8 observations from the immutable
// official FanCoilAutoSize capture, not candidate graph values. Other months
// in this small SQL fixture are explicitly observed zeros.
var driverPrecisionNorthJ = []float64{2350361815.7778316, 141392434.7538333, 511465301.42400056, 1697504079.5999966, 118617.1858992283}

var driverPrecisionNames = []string{
	"Zone Total Internal Convective Heating Energy", "Zone People Convective Heating Energy",
	"Zone Lights Convective Heating Energy", "Zone Electric Equipment Convective Heating Energy",
	"Zone Air System Sensible Heating Energy",
}

func TestEnergyDriverMonthlyPrecisionNorthHeatingUsesLoadOnlyFallback(t *testing.T) {
	rawResidual := (driverPrecisionNorthJ[0] - driverPrecisionNorthJ[1] - driverPrecisionNorthJ[2] - driverPrecisionNorthJ[3]) / 3600000
	if math.Abs(rawResidual) > 1e-9 {
		t.Fatalf("literal original heat balance no longer cancels: %.17g", rawResidual)
	}
	path := driverPrecisionSQL(t)
	doc := parsePurposePlanFixture(t, "Zone,NORTH ZONE,0,0,0,0,1,1;")
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for _, period := range []string{"M8", "annual"} {
		nodes, links := epath080Graph(t, result, "NORTH ZONE", period)
		internal := epath080NodeByID(nodes, epath080DriverNodeID(energyDriverCategoryInternalOther, "heating", "NORTH ZONE"))
		if internal != nil && (internal.Value != 0 || internal.AllocatedValue != 0) {
			t.Fatalf("%s rounded cancellation became synthetic heating pressure: %#v", period, internal)
		}
		storageID := epath080DriverNodeID(energyDriverCategoryStorageOther, "heating", "NORTH ZONE")
		storage := epath080NodeByID(nodes, storageID)
		link := epath080LinkByIDs(links, storageID, "load.heating.north_zone")
		if storage == nil || storage.Value != 0.033 || link == nil || link.FromValue != 0.033 || link.ToValue != 0.033 {
			t.Fatalf("%s lost existing 3dp delivered load / zero-pressure fallback: node=%#v link=%#v", period, storage, link)
		}
		if storage.RawValue != 0 || storage.EffectiveValue != 0 || storage.SignedValue != 0 ||
			!reflect.DeepEqual(storage.SourceIDs, []string{"sql-rdd-5"}) || !reflect.DeepEqual(link.SourceIDs, []string{"sql-rdd-5"}) {
			t.Fatalf("%s synthetic raw pressure is not an exact load-only fallback: node=%#v link=%#v", period, storage, link)
		}
		if storage.Period != period || storage.ZoneName != "NORTH ZONE" || storage.ServiceKind != "heating" || link.Period != period || link.ZoneName != "NORTH ZONE" || link.ServiceKind != "heating" {
			t.Fatalf("%s native fallback lost exact Zone/service/period provenance: node=%#v link=%#v", period, storage, link)
		}
		buildingNodes, buildingLinks := result.Nodes, result.Links
		if period != "annual" {
			p := energyExplanationPeriodByID(result.Periods, period)
			if p == nil {
				t.Fatal("missing Building M8")
			}
			buildingNodes, buildingLinks = p.Nodes, p.Links
		}
		buildingID := epath080DriverNodeID(energyDriverCategoryStorageOther, "heating", "building")
		building := epath080NodeByID(buildingNodes, buildingID)
		buildingLink := epath080LinkByIDs(buildingLinks, buildingID, "load.heating.building")
		if building == nil || building.Value != .033 || building.RawValue != 0 || building.EffectiveValue != 0 || building.SignedValue != 0 ||
			!reflect.DeepEqual(building.SourceIDs, []string{"sql-rdd-5"}) || buildingLink == nil || buildingLink.FromValue != .033 || buildingLink.ToValue != .033 || !reflect.DeepEqual(buildingLink.SourceIDs, []string{"sql-rdd-5"}) {
			t.Fatalf("%s Building is not the exact completed Zone fallback: node=%#v link=%#v", period, building, buildingLink)
		}
		// Building endpoints aggregate; this branch retains its one exact
		// contributing Zone rather than erasing physical ownership.
		if building.Period != period || building.ZoneName != "" || building.ServiceKind != "heating" || buildingLink.Period != period || buildingLink.ZoneName != "NORTH ZONE" || buildingLink.ServiceKind != "heating" {
			t.Fatalf("%s native fallback lost exact Building/service/period provenance: node=%#v link=%#v", period, building, buildingLink)
		}
	}
	driverPrecisionAssertNativeGraphPeriods(t, result)
}

func driverPrecisionAssertNativeGraphPeriods(t *testing.T, result EnergyExplanationResult) {
	t.Helper()
	if result.Scope.Kind != "building" || result.Scope.ZoneName != "" || len(result.ZoneResults) != 1 || result.ZoneResults[0].Scope.Kind != "zone" || result.ZoneResults[0].Scope.ZoneName != "NORTH ZONE" {
		t.Fatalf("native canonical scope wrappers changed: %+v", result.Scope)
	}
	bundle := PurposeResultBundle{EnergyExplanation: result}
	for _, scope := range []struct {
		kind, zone string
		periods    []EnergyPeriod
	}{
		{"building", "", result.Periods}, {"zone", "NORTH ZONE", result.ZoneResults[0].Periods},
	} {
		// Only M8 has nonzero observations in this minimal fixture. Preserve
		// the existing native graph roster instead of inventing zero months.
		if len(scope.periods) != 2 {
			t.Fatalf("%s requires exactly the native Annual and M8 contexts", scope.kind)
		}
		for _, period := range []string{"annual", "M8"} {
			wrapper := energyExplanationPeriodByID(scope.periods, period)
			kind := "monthly"
			if period == "annual" {
				kind = "annual"
			}
			if wrapper == nil || wrapper.ID != period || wrapper.Kind != kind {
				t.Fatalf("%s/%s lost its exact native Month/Annual wrapper: %#v", scope.kind, period, wrapper)
			}
			// No serialization/normalization repair between the native result
			// and this strict scope/period/endpoint contract check.
			if _, _, _, _, err := epathOracleGraph(bundle, scope.kind, scope.zone, period); err != nil {
				t.Fatalf("strict native graph %s/%s: %v", scope.kind, period, err)
			}
			if err := epathValidateOracleGraphRecords(wrapper.Nodes, wrapper.Links, wrapper.Reconciliation, scope.kind, scope.zone, period, "NORTH ZONE"); err != nil {
				t.Fatalf("strict duplicate annual/monthly wrapper %s/%s: %v", scope.kind, period, err)
			}
		}
	}
}

func TestEnergyDriverMonthlyPrecisionKnownZeroInvalidAndAliasWinner(t *testing.T) {
	for _, mutation := range []string{"known zero", "null month", "nonfinite month", "missing month", "duplicate month", "wrong unit", "monthly rate alias"} {
		t.Run(mutation, func(t *testing.T) {
			path := driverPrecisionSQL(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			queries := map[string]string{
				"null month":      `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=2 AND TimeIndex=8`,
				"nonfinite month": `UPDATE ReportData SET Value=1e999 WHERE ReportDataDictionaryIndex=2 AND TimeIndex=8`,
				"missing month":   `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=2 AND TimeIndex=8`,
				"duplicate month": `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(8,2,141392434.7538333)`,
				"wrong unit":      `UPDATE ReportDataDictionary SET Units='m3' WHERE ReportDataDictionaryIndex=2`,
			}
			if query := queries[mutation]; query != "" {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "monthly rate alias" {
				if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(6,'NORTH ZONE','Zone People Convective Heating Rate','W',0,'Monthly','Zone')`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,6,999999 FROM Time`); err != nil {
					t.Fatal(err)
				}
				for _, query := range []string{
					`INSERT INTO ReportDataDictionary VALUES(7,'NORTH ZONE','Zone People Convective Heating Energy','J',0,'Hourly','Zone')`,
					`INSERT INTO Time SELECT TimeIndex+100,Month,1,1,0,2017,60,1,3,0 FROM Time WHERE TimeIndex<100`,
					`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,7,999999999 FROM Time WHERE TimeIndex>100`,
				} {
					if _, err := db.Exec(query); err != nil {
						t.Fatal(err)
					}
				}
			}
			_ = db.Close()
			parsed := driverPrecisionParsed(t, path, idf.GeometryReport{})
			var people *energyExplanationSeries
			for i := range parsed.Series {
				if parsed.Series[i].sourceName == driverPrecisionNames[1] && parsed.Series[i].sourceFrequency == "Monthly" {
					people = &parsed.Series[i]
				}
			}
			if people == nil || people.driverMonthlyShadow == nil {
				t.Fatal("tracked original Monthly source lost private evidence")
			}
			if mutation == "known zero" || mutation == "monthly rate alias" {
				value, exists := people.driverMonthlyShadow.values[1]
				if people.driverMonthlyShadow.invalid || !exists || value != 0 {
					t.Fatal("reported Monthly0 lost known presence")
				}
				if got := people.driverMonthlyShadow.values[8]; math.Abs(got-driverPrecisionNorthJ[1]/3600000) > 1e-12 {
					t.Fatalf("original precise value changed: %.17g", got)
				}
				selected := preferredEnergyExplanationSeries(parsed.Series)
				selectedPeople := 0
				for _, item := range selected {
					if item.sourceName == driverPrecisionNames[1] {
						selectedPeople++
						if item.driverMonthlyShadow == nil || item.driverMonthlyShadow.values[8] != people.driverMonthlyShadow.values[8] || !reflect.DeepEqual(item.MonthlySourceIDs, []string{"sql-rdd-2"}) {
							t.Fatal("alias winner took another source's precision or membership")
						}
					}
				}
				if selectedPeople != 1 {
					t.Fatalf("expected exact Monthly Energy winner once, got%d", selectedPeople)
				}
			} else {
				if !people.driverMonthlyShadow.invalid {
					t.Fatal("invalid source gained exact authority")
				}
				source := energyExplanationSourceByID(parsed.Sources, "sql-rdd-2")
				if source == nil || source.Name != driverPrecisionNames[1] || source.KeyValue != "NORTH ZONE" || energyDataSourceValueKnown(*source, energySourceObservedRaw) {
					t.Fatalf("invalid original identity was removed or relabelled knownzero: %#v", source)
				}
				prepared := canonicalEnergyExplanationSeries(*people)
				prepared.multiplierApplied, prepared.EffectiveMultiplier, prepared.MultiplierApplication = true, 1, energyMultiplierRequiresZone
				vector := energyDriverSignedVector(prepared)
				applyEnergyDriverReconciliationMonthlyPrecision(&vector)
				if len(vector.monthly) != 0 || vector.total != 0 {
					t.Fatal("invalid/absent precision fell back to rounded values")
				}
			}
		})
	}
}

func TestEnergyDriverMonthlyPrecisionMissingSelectedSurfaceInvalidatesCategory(t *testing.T) {
	for _, mutation := range []string{"all NULL", "all missing", "one missing", "complete", "available energy winner", "available rate winner"} {
		t.Run(mutation, func(t *testing.T) {
			path := driverPrecisionSQL(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			for _, query := range []string{
				`INSERT INTO ReportDataDictionary VALUES(6,'Wall A','Surface Inside Face Convection Heat Gain Energy','J',0,'Monthly','Zone'),(7,'Wall B','Surface Inside Face Convection Heat Gain Energy','J',0,'Monthly','Zone')`,
				`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,6,3600000 FROM Time`,
				`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,7,7200000 FROM Time`,
			} {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			query := map[string]string{
				"all NULL":    `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=7`,
				"all missing": `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=7`,
				"one missing": `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=7 AND TimeIndex=8`,
			}[mutation]
			if query != "" {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "available energy winner" {
				if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(8,'Wall B','Surface Inside Face Convection Heat Gain Rate','W',0,'Monthly','Zone')`); err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "available rate winner" {
				for _, query := range []string{
					`UPDATE ReportDataDictionary SET Name='Surface Inside Face Convection Heat Gain Rate',Units='W' WHERE ReportDataDictionaryIndex=7`,
					`UPDATE ReportData SET Value=2000.0/(SELECT Interval/60.0 FROM Time WHERE Time.TimeIndex=ReportData.TimeIndex) WHERE ReportDataDictionaryIndex=7`,
					`INSERT INTO ReportDataDictionary VALUES(8,'Wall B','Surface Inside Face Convection Heat Gain Energy','J',0,'Monthly','Zone')`,
				} {
					if _, err := db.Exec(query); err != nil {
						t.Fatal(err)
					}
				}
			}
			_ = db.Close()
			geometry := idf.GeometryReport{Surfaces: []idf.GeometrySurface{
				{ID: "wall.a", Name: "Wall A", SurfaceType: "Wall", ZoneName: "NORTH ZONE", OutsideBoundary: "Outdoors"},
				{ID: "wall.b", Name: "Wall B", SurfaceType: "Wall", ZoneName: "NORTH ZONE", OutsideBoundary: "Outdoors"},
			}}
			parsed := driverPrecisionParsed(t, path, geometry)
			found := 0
			for _, item := range parsed.Series {
				if !item.parseCategoryAggregate {
					continue
				}
				found++
				valid := mutation == "complete" || strings.HasPrefix(mutation, "available ")
				if item.driverMonthlyShadow == nil || item.driverMonthlyShadow.invalid == valid {
					t.Fatalf("selected category validity ignores %s: %#v", mutation, item.driverMonthlyShadow)
				}
				if valid && math.Abs(item.driverMonthlyShadow.values[8]-3) > 1e-12 {
					t.Fatal("complete selected surface sum is not exact")
				}
			}
			if found != 1 {
				t.Fatalf("want one physical surface category, got%d", found)
			}
		})
	}
}

func TestEnergyDriverMonthlyPrecisionCloneDirectionAndUnknownMonth(t *testing.T) {
	item := energyExplanationSeries{Monthly: map[int]float64{8: 0}, multiplierApplied: true, EffectiveMultiplier: 10, MultiplierApplication: energyMultiplierRequiresZone, heatSignMultiplier: 1,
		driverMonthlyShadow: &energyDriverMonthlyShadow{values: map[int]float64{1: 0, 8: -.0004}}}
	vector := energyDriverSignedVector(item)
	if math.Abs(vector.monthlyShadow.values[8]+.004) > 1e-15 {
		t.Fatal("native shadow was rounded before multiplier")
	}
	vector.monthlyShadow.values[8] = 9
	if item.driverMonthlyShadow.values[8] != -.0004 {
		t.Fatal("signed vector mutated original shadow")
	}
	derived := energyExplanationSeries{driverMonthlyShadow: &energyDriverMonthlyShadow{values: map[int]float64{8: -.004}, effective: true}, multiplierApplied: true, EffectiveMultiplier: 10, MultiplierApplication: energyMultiplierRequiresZone}
	loss, _ := applyEnergyDriverDerivedDirection(derived, nil, "loss")
	if loss.driverMonthlyShadow.values[8] != .004 || derived.driverMonthlyShadow.values[8] != -.004 || energyDriverSignedVector(loss).monthlyShadow.values[8] != -.004 {
		t.Fatal("loss direction lost precision, mutated input, or applied multiplier twice")
	}
	left := energyDriverSignedVector(item)
	right := energyDriverSignedVector(energyExplanationSeries{multiplierApplied: true, EffectiveMultiplier: 1, MultiplierApplication: energyMultiplierRequiresZone, driverMonthlyShadow: &energyDriverMonthlyShadow{values: map[int]float64{1: 0}}})
	left.subtract(right)
	applyEnergyDriverReconciliationMonthlyPrecision(&left)
	if _, exists := left.monthly[8]; exists {
		t.Fatal("missing contributing month became knownzero or reused rounded observation")
	}
	if zero, exists := left.monthly[1]; !exists || zero != 0 {
		t.Fatal("intersection lost fully observed zero")
	}
	legacy := energyDriverVector{monthly: map[int]float64{8: .001}, total: .001}
	applyEnergyDriverReconciliationMonthlyPrecision(&legacy)
	if legacy.monthly[8] != .001 || legacy.total != .001 {
		t.Fatal("untracked legacy numerical boundary changed")
	}
}

func driverPrecisionParsed(t *testing.T, path string, geometry idf.GeometryReport) energyExplanationParseResult {
	t.Helper()
	doc := parsePurposePlanFixture(t, "Zone,NORTH ZONE,0,0,0,0,1,1;")
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, newEnergyDriverBuildContext(geometry, doc))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range parsed.Series {
		if strings.EqualFold(item.sourceFrequency, "Hourly") && item.driverMonthlyShadow != nil {
			t.Fatal("Hourly row acquired invented Monthly precision")
		}
	}
	return parsed
}

func driverPrecisionSQL(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "precision.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, query := range []string{
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,KeyValue TEXT,Name TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT)`,
		`CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,Year INTEGER,Interval REAL,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER)`,
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,3)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
	} {
		if _, err := tx.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for month := 1; month <= 12; month++ {
		days := time.Date(2017, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if _, err := tx.Exec(`INSERT INTO Time VALUES(?,?,?,24,0,2017,?,3,3,0)`, month, month, days, days*1440); err != nil {
			t.Fatal(err)
		}
	}
	for i, name := range driverPrecisionNames {
		if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(?,'NORTH ZONE',?,'J',0,'Monthly','Zone')`, i+1, name); err != nil {
			t.Fatal(err)
		}
		for month := 1; month <= 12; month++ {
			value := 0.0
			if month == 8 {
				value = driverPrecisionNorthJ[i]
			}
			if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, month, i+1, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(fmt.Errorf("precision fixture: %w", err))
	}
	return path
}
