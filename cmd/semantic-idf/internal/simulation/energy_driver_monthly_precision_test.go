package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyDriverGrossDirectionsPreserveNativeIntervalBeforeEverySum(t *testing.T) {
	for _, activity := range []struct {
		name       string
		gain, loss float64
	}{{"small", 10, 5}, {"large", 110, 105}} {
		t.Run(activity.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gross.sql")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			statements := []string{
				`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentType INTEGER)`,
				`INSERT INTO EnvironmentPeriods VALUES(3,3)`,
				`CREATE TABLE "Time"(TimeIndex INTEGER PRIMARY KEY,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER)`,
				`INSERT INTO "Time" VALUES(1,2025,1,1,0,15,15,0,3,0),(2,2025,1,1,0,30,15,0,3,0),(3,2025,1,1,0,45,15,0,3,0),(4,2025,1,1,1,0,15,0,3,0),(999,2025,1,1,24,0,1440,2,3,0),(1000,2025,1,1,24,0,1440,3,3,0)`,
				`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,KeyValue TEXT,Name TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT)`,
				`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
				`INSERT INTO ReportDataDictionary VALUES(1,'Wall A','Surface Inside Face Convection Heat Gain Energy','J',0,'Monthly','Zone'),(2,'Wall A','Surface Inside Face Convection Heat Gain Energy','J',0,'Zone Timestep','Zone'),(3,'Wall B','Surface Inside Face Convection Heat Gain Energy','J',0,'Monthly','Zone'),(4,'Wall B','Surface Inside Face Convection Heat Gain Energy','J',0,'Zone Timestep','Zone'),(5,'Office','Zone Air Heat Balance Air Energy Storage Rate','W',0,'Monthly','Zone'),(6,'Office','Zone Air Heat Balance Air Energy Storage Rate','W',0,'Zone Timestep','Zone'),(7,'Office','Zone Air System Sensible Cooling Energy','J',0,'Monthly','Zone'),(8,'Office','Zone Air System Sensible Heating Energy','J',0,'Monthly','Zone')`,
				`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(1000,1,-18000000),(1000,3,0),(1000,5,0),(1000,7,3600000),(1000,8,3600000),(1,4,10800000),(2,4,-10800000),(3,4,0),(4,4,0),(1,6,40000),(2,6,-20000),(3,6,20000),(4,6,-40000)`,
				fmt.Sprintf(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(1,2,%.17g),(2,2,%.17g),(3,2,0),(4,2,0)`, -activity.gain*3600000, activity.loss*3600000),
			}
			for _, statement := range statements {
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			for index := 5; index <= 96; index++ {
				minutes := index * 15
				if _, err := db.Exec(`INSERT INTO "Time" VALUES(?,2025,1,1,?,?,15,0,3,0)`, index, minutes/60, minutes%60); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,2,0),(?,4,0),(?,6,0)`, index, index, index); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if activity.name == "small" {
				t.Run("dictionary metadata preserves interval arithmetic", func(t *testing.T) {
					assertEnergyDriverDirectionalDictionaryArithmetic(t, path)
				})
			}
			report := idf.GeometryReport{Zones: []idf.GeometryZone{{Name: "Office"}}, Surfaces: []idf.GeometrySurface{{ID: "a", Name: "Wall A", ZoneName: "Office", SurfaceType: "Wall", OutsideBoundary: "Outdoors"}, {ID: "b", Name: "Wall B", ZoneName: "Office", SurfaceType: "Wall", OutsideBoundary: "Outdoors"}}}
			plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
			legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, plan, newEnergyDriverBuildContext(report))
			if err != nil {
				t.Fatal(err)
			}
			result := UpgradeEnergyExplanationV1(legacy)
			for _, direction := range []struct {
				name, service string
				value         float64
			}{{"gain", "cooling", activity.gain + 3}, {"loss", "heating", activity.loss + 3}} {
				id := "driver.surface.exterior_walls." + direction.service + ".building." + direction.name
				node := energyPathV2NodeByID(result.Nodes, id)
				if node == nil || node.HeatDirection != direction.name || node.HeatDirectionBasis != "native_interval" || node.EffectiveValue != direction.value || node.RawValue != direction.value || node.AllocatedValue <= 0 || node.AllocatedValue >= direction.value {
					t.Fatalf("native directions collapsed: id=%s node=%#v result=%#v", id, node, result.Nodes)
				}
				if !stringSliceContains(node.SourceIDs, "sql-rdd-2") || !stringSliceContains(node.SourceIDs, "sql-rdd-4") || stringSliceContains(node.SourceIDs, "sql-rdd-1") {
					t.Fatalf("gross source identity=%#v", node)
				}
			}
			for _, direction := range []struct{ name, service string }{{"gain", "cooling"}, {"loss", "heating"}} {
				storage := energyPathV2NodeByID(result.Nodes, "driver.balance.storage_other."+direction.service+".building."+direction.name)
				if storage == nil || storage.EffectiveValue != 15 || len(storage.DriverBreakdown) != 1 || storage.DriverBreakdown[0].Component != "air_storage" {
					t.Fatalf("storage/release interval split=%#v", storage)
				}
			}
			for _, service := range []string{"cooling", "heating"} {
				sum := 0.0
				for _, link := range result.Links {
					if link.Relation == "driver_to_load" && link.ServiceKind == service {
						sum += link.FromValue
					}
				}
				if math.Abs(sum-1) > 1e-9 {
					t.Fatalf("%s allocation no longer closes: %.17g", service, sum)
				}
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var restored EnergyExplanationResult
			if err := json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			gain := energyPathV2NodeByID(restored.Nodes, "driver.surface.exterior_walls.cooling.building.gain")
			if gain == nil || gain.HeatDirection != "gain" || gain.HeatDirectionBasis != "native_interval" || gain.EffectiveValue != activity.gain+3 {
				t.Fatalf("roundtrip lost gross evidence: %#v", gain)
			}
			if activity.name == "small" {
				original, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				for name, mutations := range map[string][]string{
					"native Zone Timestep EachCall encoding":              {`UPDATE "Time" SET IntervalType=-1 WHERE IntervalType=0`, `INSERT INTO "Time" VALUES(200000,2025,1,1,0,5,5,-1,3,0)`, `INSERT INTO ReportDataDictionary VALUES(500,'Wall A','Surface Inside Face Convection Heat Gain Energy','J',0,'HVAC System Timestep','Zone')`, `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(200000,500,360000000000)`},
					"missing first native interval":                       {`DELETE FROM "Time" WHERE TimeIndex=1`},
					"native union without independent endpoint authority": {`DELETE FROM "Time" WHERE IntervalType=2`},
					"missing last native interval":                        {`DELETE FROM "Time" WHERE TimeIndex=96`},
					"missing selected surface interval":                   {`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=4 AND TimeIndex=2`},
					"null selected surface interval":                      {`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=4 AND TimeIndex=2`},
					"duplicate selected surface interval":                 {`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,ReportDataDictionaryIndex,Value FROM ReportData WHERE ReportDataDictionaryIndex=4 AND TimeIndex=2`},
					"hourly fallback":                                     {`DELETE FROM ReportData WHERE TimeIndex BETWEEN 25 AND 96`, `DELETE FROM "Time" WHERE TimeIndex BETWEEN 25 AND 96`, `UPDATE "Time" SET Hour=TimeIndex,Minute=0,"Interval"=60,IntervalType=1 WHERE TimeIndex BETWEEN 1 AND 24`, `UPDATE ReportDataDictionary SET ReportingFrequency='Hourly' WHERE ReportingFrequency='Zone Timestep'`},
				} {
					t.Run(name, func(t *testing.T) {
						mutatedPath := filepath.Join(t.TempDir(), "mutated.sql")
						if err := os.WriteFile(mutatedPath, original, 0600); err != nil {
							t.Fatal(err)
						}
						mutated, err := sql.Open("sqlite", mutatedPath)
						if err != nil {
							t.Fatal(err)
						}
						for _, mutation := range mutations {
							if _, err := mutated.Exec(mutation); err != nil {
								t.Fatal(err)
							}
						}
						if err := mutated.Close(); err != nil {
							t.Fatal(err)
						}
						parsed, err := parseSimulationEnergyExplanationSQLWithDriverContext(mutatedPath, plan, newEnergyDriverBuildContext(report))
						if err != nil {
							t.Fatal(err)
						}
						upgraded := UpgradeEnergyExplanationV1(parsed)
						wall := energyPathV2NodeByID(upgraded.Nodes, "driver.surface.exterior_walls.cooling.building.gain")
						if name == "hourly fallback" || name == "native Zone Timestep EachCall encoding" {
							if wall == nil || wall.EffectiveValue != 13 || wall.HeatDirectionBasis != "native_interval" {
								t.Fatalf("complete native Hourly fallback lost gross evidence: %#v", wall)
							}
						} else if wall != nil {
							t.Fatalf("incomplete native evidence acquired gross authority: %#v", wall)
						}
					})
				}
			}
		})
	}
}

func assertEnergyDriverDirectionalDictionaryArithmetic(t *testing.T, path string) {
	t.Helper()
	db, err := openSimulationSQLiteReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, test := range []struct {
		name, source, unit, frequency string
		convert                       func(float64) float64
		ignored, nonHeat              bool
	}{
		{"surface J reversal", "Surface Inside Face Convection Heat Gain Energy", "J", "Zone Timestep", func(value float64) float64 { return (value * (1.0 / 3600000)) * -1 }, false, false},
		{"storage W reversal", "Zone Air Heat Balance Air Energy Storage Rate", " W ", "Timestep", func(value float64) float64 { return (value * .25 / 1000) * -1 }, false, false},
		{"surface kW reversal", "Surface Inside Face Convection Heat Gain Rate", "k_W", "Zone Timestep", func(value float64) float64 { return (value * .25) * -1 }, false, false},
		{"conduction kJ", "Surface Inside Face Conduction Heat Transfer Energy", " k_J ", "Zone Timestep", func(value float64) float64 { return value * (1.0 / 3600) }, false, false},
		{"conduction GJ", "Surface Inside Face Conduction Heat Transfer Energy", "GJ", "Zone Timestep", func(value float64) float64 { return value * 277.7777777778 }, false, false},
		{"conduction Wh", "Surface Inside Face Conduction Heat Transfer Energy", "Wh", "Zone Timestep", func(value float64) float64 { return value * (1.0 / 1000) }, false, false},
		{"conduction kWh", "Surface Inside Face Conduction Heat Transfer Energy", "kWh", "Zone Timestep", func(value float64) float64 { return value }, false, false},
		{"unsupported unit", "Surface Inside Face Conduction Heat Transfer Energy", "unknown", "Zone Timestep", nil, false, false},
		{"unsupported rate unit", "Zone Air Heat Balance Air Energy Storage Rate", "Watt", "Zone Timestep", nil, false, false},
		{"monthly excluded", "Surface Inside Face Conduction Heat Transfer Energy", "J", "Monthly", nil, true, false},
		{"explicit gain excluded", "Zone Windows Total Heat Gain Energy", "J", "Zone Timestep", nil, true, false},
		{"nonheat excluded", "Surface Inside Face Conduction Heat Transfer Energy", "J", "Zone Timestep", nil, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition, exists := energyHeatAliasDefinitionForName(test.source)
			if !exists {
				t.Fatalf("native heat alias is missing: %s", test.source)
			}
			// Reusing the source index across independently interpreted reads
			// must never borrow another run's units, frequency or physical sign.
			dictionary := energyExplanationDictionary{row: sqlOutputDictionaryRow{index: 700, keyValue: "Office", name: test.source, units: test.unit}, reportingFrequency: test.frequency, heat: &definition}
			if test.nonHeat {
				dictionary.heat = nil
			}
			conversion := prepareEnergyExplanationValueConversion(dictionary)
			dictionary.valueConversion = &conversion
			collector := newEnergyDriverDirectionalCollector(db, true)
			if collector == nil {
				t.Fatal("native interval collector is missing")
			}
			collector.observe(SQLSeriesRow{DictionaryIndex: 700, TimeIndex: -1}, dictionary)
			if len(collector.series) != 0 {
				t.Fatal("an excluded timestamp created directional observation state")
			}
			values := []float64{12345.6789012345, -98765.4321098765, 0, math.Copysign(0, -1), 1.0000000000000002e-8, -1e17, 1e17}
			gain, loss := 0.0, 0.0
			for index := int64(1); index <= 96; index++ {
				frame := collector.times[index]
				value := values[(index-1)%int64(len(values))]
				collector.observe(SQLSeriesRow{DictionaryIndex: 700, TimeIndex: index, Month: sql.NullInt64{Int64: int64(frame.month), Valid: true}, Day: sql.NullInt64{Int64: int64(frame.day), Valid: true}, Hour: sql.NullInt64{Int64: int64(frame.hour), Valid: true}, Minute: sql.NullInt64{Int64: int64(frame.minute), Valid: true}, IntervalType: frame.intervalType, Value: sql.NullFloat64{Float64: value, Valid: true}}, dictionary)
				if test.convert != nil {
					signed := test.convert(value)
					gain += math.Max(signed, 0)
					loss += math.Max(-signed, 0)
				}
			}
			gross := collector.values(dictionary)
			if test.ignored {
				if gross != nil || len(collector.series) != 0 {
					t.Fatal("an excluded dictionary created native gross evidence")
				}
				return
			}
			if gross == nil || !reflect.DeepEqual(gross.sourceIDs, []string{"sql-rdd-700"}) {
				t.Fatal("native dictionary/source identity was lost")
			}
			if test.convert == nil {
				if gross.observed[1] || len(gross.gain) != 0 || len(gross.loss) != 0 {
					t.Fatal("invalid native units became zero or valid directional evidence")
				}
				return
			}
			if !gross.observed[1] || math.Float64bits(gross.gain[1]) != math.Float64bits(gain) || math.Float64bits(gross.loss[1]) != math.Float64bits(loss) {
				t.Fatalf("native gross arithmetic/order changed: gain %.17g want %.17g; loss %.17g want %.17g; observed=%t", gross.gain[1], gain, gross.loss[1], loss, gross.observed[1])
			}
		})
	}
}

func TestEnergyDriverGrossAnnualRetainsZeroAllocationAndObservedZeroMonths(t *testing.T) {
	storage := reviewEnergyHeatSeries(t, "Office", "Zone Air Heat Balance Air Energy Storage Rate", -11, "storage")
	storage.Monthly = map[int]float64{1: -5, 2: -6, 3: 0}
	storage.driverDirections = &energyDriverDirectionalValues{gain: map[int]float64{1: 10, 2: 20, 3: 0}, loss: map[int]float64{1: 5, 2: 14, 3: 0}, observed: map[int]bool{1: true, 2: true, 3: true}, sourceIDs: []string{"storage"}}
	cooling, heating := reviewEnergyLoadSeries("Office", "cooling", 1, "cooling"), reviewEnergyLoadSeries("Office", "heating", 1, "heating")
	cooling.Monthly, heating.Monthly = map[int]float64{1: 1, 2: 0, 3: 0}, map[int]float64{1: 1, 2: 0, 3: 0}
	series := []energyExplanationSeries{storage, cooling, heating}
	sources := reviewEnergySources(series)
	sources[0].SourceType, sources[0].ReportingFrequency = "sql_report_data", "Zone Timestep"
	legacy := buildEnergyExplanationResultWithDriverContext(series, sources, &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}, newEnergyDriverBuildContext(idf.GeometryReport{Zones: []idf.GeometryZone{{Name: "Office"}}}))
	result := UpgradeEnergyExplanationV1(legacy)
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var restored EnergyExplanationResult
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		direction, service string
		annual, second     float64
	}{{"gain", "cooling", 30, 20}, {"loss", "heating", 19, 14}} {
		id := "driver.balance.storage_other." + expected.service + ".building." + expected.direction
		annual := energyPathV2NodeByID(result.Nodes, id)
		second := energyPathV2NodeByID(energyExplanationPeriodByID(result.Periods, "M2").Nodes, id)
		zero := energyPathV2NodeByID(energyExplanationPeriodByID(result.Periods, "M3").Nodes, id)
		if annual == nil || annual.EffectiveValue != expected.annual || annual.AllocatedValue != 1 || second == nil || second.EffectiveValue != expected.second || second.AllocatedValue != 0 || !second.AllocationApplied {
			t.Fatalf("zero-load month erased gross observation: annual=%#v second=%#v", annual, second)
		}
		if zero == nil || zero.EffectiveValue != 0 || zero.RawValue != 0 || zero.SignedValue != 0 || zero.HeatDirection != expected.direction || zero.ServiceKind != expected.service {
			t.Fatalf("observed zero became missing or wrong-direction: %#v", zero)
		}
		storedZero := energyPathV2NodeByID(energyExplanationPeriodByID(restored.Periods, "M3").Nodes, id)
		if storedZero == nil || storedZero.HeatDirection != expected.direction || storedZero.inspectorValuePresence&11 != 11 {
			t.Fatalf("stored zero lost physical direction/presence: %#v", storedZero)
		}
		for _, link := range energyExplanationPeriodByID(result.Periods, "M2").Links {
			if link.Relation == "driver_to_load" && link.FromID == id {
				t.Fatalf("zero allocation created a gross-energy main-flow ribbon: %#v", link)
			}
		}
	}
}

func TestEnergyDriverGrossNativeDirectionDoesNotAuthorizeDerivedNetRemainder(t *testing.T) {
	native := reviewEnergyHeatSeries(t, "Office", "Zone Other Equipment Convective Heating Energy", 10, "native")
	native.Stage, native.DriverSourceRole = "driver", energyDriverSourceRoleMainFlow
	native.DriverCategory = energyDriverCategoryInternalOther
	derived := native
	derived.SourceIDs, derived.Kind = []string{"derived"}, "heat.internal_other_reconciliation"
	derived.Total, derived.Monthly = -5, map[int]float64{1: -5}
	sources := []EnergyDataSource{{ID: "native", SourceType: "sql_report_data", Name: "Zone Other Equipment Convective Heating Energy", ReportingFrequency: "Monthly"}, {ID: "derived", SourceType: "derived_formula", Name: "Internal reconciliation"}}
	split := splitEnergyDriverDirectionalSeries([]energyExplanationSeries{native, derived}, sources)
	if split[0].heatDirection != "gain" || split[0].heatDirectionBasis != "native_direction" || split[1].heatDirection != "" || split[1].heatDirectionBasis != "net_only" {
		t.Fatalf("derived sign received native direction authority: %#v", split)
	}
	intermediate := buildEnergyExplanationGraphForPeriod("M1", split, "reported", func(item energyExplanationSeries) float64 { return item.Monthly[1] }, sources)
	if len(intermediate.Nodes) != 2 {
		t.Fatalf("intermediate graph merged native direction with derived net: %#v", intermediate.Nodes)
	}
	scope := EnergyExplanationScope{Kind: "building"}
	nativeNode := EnergyExplanationNode{ID: "native", Level: "heat", Kind: native.Kind, DriverCategory: energyDriverCategoryInternalOther, ServiceKind: "cooling", Value: 10, RawValue: 10, EffectiveValue: 10, SignedValue: 10, HeatDirection: "gain", HeatDirectionBasis: "native_direction", SourceIDs: []string{"native"}}
	netNode := nativeNode
	netNode.ID, netNode.HeatDirection, netNode.HeatDirectionBasis, netNode.SourceIDs = "derived", "", "net_only", []string{"derived"}
	nodes := map[string]*EnergyExplanationNode{}
	mergeEnergyExplanationV2Node(nodes, upgradeEnergyExplanationNode(nativeNode, scope))
	mergeEnergyExplanationV2Node(nodes, upgradeEnergyExplanationNode(netNode, scope))
	if len(nodes) != 2 || nodes["driver.internal.other.cooling.building.gain"] == nil || nodes["driver.internal.other.cooling.building"] == nil {
		t.Fatalf("native gross observation merged with net-only remainder: %#v", nodes)
	}
	forged := nativeNode
	forged.Level, forged.SourceIDs = "driver", []string{"wall-net"}
	stored := []EnergyExplanationNode{nativeNode, forged}
	stored[0].Level = "driver"
	sources = append(sources, EnergyDataSource{ID: "wall-net", SourceType: "sql_report_data", Name: "Surface Inside Face Convection Heat Gain Energy", ReportingFrequency: "Monthly"})
	qualifyStoredEnergyDriverDirectionSources(stored, sources)
	if stored[0].HeatDirection != "gain" || stored[1].HeatDirection != "" || stored[1].HeatDirectionBasis != "net_only" {
		t.Fatalf("stored metadata bypassed native source sign proof: %#v", stored)
	}
}

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
