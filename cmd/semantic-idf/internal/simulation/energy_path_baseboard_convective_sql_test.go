package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Synthetic observations are deliberately not the real-model oracle. The
// unchanged official model supplies typed owners and factors; hand quantities
// make duplicate energy, multiplier and native context contamination obvious.
func energyPathConvectiveBaseboardSQLFixture(t *testing.T) (string, string, idf.Document, PurposeRunPlan, energyDriverBuildContext) {
	t.Helper()
	doc, input := energyPathConvectiveBaseboardOriginal(t)
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	if len(context.BaseboardTargets) != 9 {
		t.Fatalf("native targets=%+v", context.BaseboardTargets)
	}
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare})
	path := directHVACSQLFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`DELETE FROM ReportData`, `DELETE FROM ReportDataDictionary`,
		`INSERT INTO "Time" (TimeIndex,Month,Day,Hour,Minute,Year,"Interval",IntervalType,EnvironmentPeriodIndex,WarmupFlag) VALUES(3,1,1,1,0,2017,60,1,3,0),(4,2,1,1,0,2017,60,1,3,0)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	add := func(id int, key, name, unit, frequency string, meter int, first float64) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,?,?,'HVAC')`, id, key, name, unit, meter, frequency); err != nil {
			t.Fatal(err)
		}
		for month := 1; month <= 2; month++ {
			value := first * float64(month)
			timeIndex := month
			if frequency == "Hourly" {
				timeIndex += 2
			}
			if unit == "J" {
				value *= 3600000
			} else {
				hours := 1.0
				if frequency == "Monthly" {
					hours = 744
					if month == 2 {
						hours = 672
					}
				}
				value = value * 1000 / hours
			}
			if _, err := db.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, timeIndex, id, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	add(1, "", "Electricity:Facility", "J", "Monthly", 1, 450)
	add(2, "", "Heating:Electricity", "J", "Monthly", 1, 450)
	for i, target := range context.BaseboardTargets {
		first := 10 * float64(i+1)
		for field, name := range []string{"Baseboard Electricity Energy", "Baseboard Electricity Rate", "Baseboard Total Heating Energy", "Baseboard Total Heating Rate"} {
			unit := "J"
			if strings.HasSuffix(name, " Rate") {
				unit = "W"
			}
			value := first
			if field >= 2 {
				value = 900
			}
			add(100+i*10+field, target.KeyValue, name, unit, "Monthly", 0, value)
			add(104+i*10+field, target.KeyValue, name, unit, "Hourly", 0, value)
		}
		add(300+i, target.ZoneName, "Zone Air System Sensible Heating Energy", "J", "Monthly", 0, 100)
	}
	return path, input, doc, plan, context
}

func TestEnergyPathConvectiveBaseboardSQLNativeValuesFactorOncePathsAndReload(t *testing.T) {
	path, input, _, plan, context := energyPathConvectiveBaseboardSQLFixture(t)
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.hvacConsumptionPools) != 1 || !legacy.hvacConsumptionPools[0].Valid || len(legacy.hvacConsumptionPools[0].Members) != 9 {
		t.Fatalf("native local pool=%+v", legacy.hvacConsumptionPools)
	}
	legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(enrichEnergyExplanationWithServicePaths(legacy, input))
	if len(legacy.servicePathIndex.nativeBaseboardPaths) != 9 {
		t.Fatal("original nine exact baseboard paths missing")
	}
	result := UpgradeEnergyExplanationV1(legacy)
	baseline := baseboardPathNumericSourceSnapshot(result)
	for reload := 0; reload < 3; reload++ {
		assertMixedHeatingNode(t, result.Nodes, "carrier", "electricity", 1350)
		assertMixedHeatingNode(t, result.Nodes, "end_use", "heating", 1350)
		seen := map[string]bool{}
		for _, zone := range result.ZoneResults {
			owner := -1
			for i, target := range context.BaseboardTargets {
				if target.ZoneName == zone.Scope.ZoneName {
					owner = i
					break
				}
			}
			if owner < 0 {
				t.Fatalf("equipment or unknown key became a Zone: %s", zone.Scope.ZoneName)
			}
			seen[zone.Scope.ZoneName] = true
			target := context.BaseboardTargets[owner]
			first := 10 * float64(owner+1)
			factor := energyPathConvectiveBaseboardZoneFactors()[target.ZoneName]
			wantPath := fmt.Sprintf("service-path:zone:%s:heating:baseboard:component:%d", strings.ToLower(target.ZoneName), target.Component.ObjectIndex)
			wantSource := fmt.Sprintf("sql-rdd-%d", 100+owner*10)
			periods := append([]EnergyPeriod{{ID: "root", Nodes: zone.Nodes, Links: zone.Links}}, zone.Periods...)
			for _, period := range periods {
				monthFactor := 0.0
				switch period.ID {
				case "root", "annual":
					monthFactor = 3
				case "M1":
					monthFactor = 1
				case "M2":
					monthFactor = 2
				default:
					continue
				}
				assertMixedHeatingNode(t, period.Nodes, "carrier", "electricity", first*monthFactor)
				assertMixedHeatingNode(t, period.Nodes, "load", "heating", 100*factor*monthFactor)
				count := 0
				for _, link := range period.Links {
					if !energyPathLinkIsCarrierSplit(link) || link.Basis != "direct_zone_energy" || !stringSliceContains(link.SourceIDs, wantSource) {
						continue
					}
					count++
					if !reflect.DeepEqual(link.SourceIDs, []string{wantSource}) || !reflect.DeepEqual(link.RelatedPathIDs, []string{wantPath}) || math.Abs(link.FromValue-first*monthFactor) > 1e-9 || math.Abs(link.ToValue-first*monthFactor) > 1e-9 {
						t.Errorf("reload%d %s/%s direct native source, path or amount changed: %+v", reload, target.ZoneName, period.ID, link)
					}
				}
				if count != 1 {
					t.Errorf("reload%d %s/%s native branch count=%d want1", reload, target.ZoneName, period.ID, count)
				}
			}
		}
		if len(seen) != 9 {
			t.Fatalf("original Zone count=%d", len(seen))
		}
		nativeSources, zoneSources := 0, 0
		for _, source := range result.Sources {
			for i, target := range context.BaseboardTargets {
				if source.ID == fmt.Sprintf("sql-rdd-%d", 300+i) {
					zoneSources++
					factor := energyPathConvectiveBaseboardZoneFactors()[target.ZoneName]
					if source.RawValue != 300 || source.EffectiveValue != 300*factor || source.EffectiveMultiplier != factor {
						t.Errorf("Zone representative load was not multiplied exactly once: %+v", source)
					}
				}
				for field := 0; field < 8; field++ {
					if source.ID != fmt.Sprintf("sql-rdd-%d", 100+i*10+field) {
						continue
					}
					nativeSources++
					first := 10 * float64(i+1)
					if field%4 >= 2 {
						first = 900
					}
					if math.Abs(source.RawValue-first*3) > 1e-9 || math.Abs(source.EffectiveValue-first*3) > 1e-9 || source.EffectiveMultiplier != 1 || source.MultiplierApplication != energyMultiplierAlreadyModelTotal || !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) {
						t.Errorf("reload%d native model total was multiplied or lost: %+v", reload, source)
					}
					if field%4 != 0 && source.DriverRole != energyDriverSourceRoleContext {
						t.Errorf("native TotalHeating/rate became additive: %+v", source)
					}
					if field%4 >= 2 && source.DriverComponent != "load.baseboard_response.convective" {
						t.Errorf("convective response classification=%+v", source)
					}
					if field >= 4 && (source.HourlyEnergy == nil || !reflect.DeepEqual(source.HourlyEnergy.Values, []float64{first, first * 2})) {
						t.Errorf("native Hourly companion changed or fabricated: %+v", source)
					}
				}
			}
		}
		if nativeSources != 72 || zoneSources != 9 {
			t.Errorf("native/source roster=%d/%d want72/9", nativeSources, zoneSources)
		}
		if !reflect.DeepEqual(baseboardPathNumericSourceSnapshot(result), baseline) {
			t.Errorf("reload%d altered numeric or source identities", reload)
		}
		wire, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var reopened EnergyExplanationResult
		if err := json.Unmarshal(wire, &reopened); err != nil {
			t.Fatal(err)
		}
		result = reopened
	}
}

func TestEnergyPathConvectiveBaseboardSQLMissingNULLAndZeroRemainDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		known       bool
	}{
		{"native zero", `UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=100`, true},
		{"NULL", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=100`, false},
		{"missing observations", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=100`, false},
		{"wrong native unit", `UPDATE ReportDataDictionary SET Units='kJ' WHERE ReportDataDictionaryIndex=100`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, _, _, plan, context := energyPathConvectiveBaseboardSQLFixture(t)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.query); err != nil {
				db.Close()
				t.Fatal(err)
			}
			db.Close()
			legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
			if err != nil {
				t.Fatal(err)
			}
			if len(legacy.hvacConsumptionPools) != 1 {
				t.Fatal("missing observation bypassed native pool gate")
			}
			found := 0
			for _, member := range legacy.hvacConsumptionPools[0].Members {
				if member.ObjectName != context.BaseboardTargets[0].KeyValue {
					continue
				}
				found++
				value, _, _, known := energyPathHVACConsumptionPeriodValue(member.Series, "M1", "monthly", true)
				if known != tc.known || known && value != 0 {
					t.Errorf("native knownness=%g/%t want0/%t", value, known, tc.known)
				}
			}
			if found != 1 {
				t.Fatal("original typed member disappeared")
			}
			for _, source := range legacy.Sources {
				if source.ID == "sql-rdd-100" && energyDataSourceValueKnown(source, energySourceObservedRaw) != tc.known {
					t.Errorf("unknown consumption fabricated native zero: %+v", source)
				}
			}
			// Hourly electricity, rate and TotalHeating remain available, but
			// cannot fill an absent Monthly direct consumption observation.
		})
	}
}

func TestEnergyPathConvectiveBaseboardNativePathCannotBorrowCoolingOrAnotherOwner(t *testing.T) {
	for _, mutation := range []string{"none", "duplicate", "cooling", "wrong type", "wrong component", "other Zone", "central air loop"} {
		t.Run(mutation, func(t *testing.T) {
			doc, _ := energyPathConvectiveBaseboardOriginal(t)
			targets := energyPathBaseboardTargets(doc)
			summaries := idf.AnalyzeHVAC(doc).ServiceModel.ZoneServices
			target := targets[0]
			var path *idf.ZoneServicePath
			var summary *idf.ZoneServiceSummary
			for i := range summaries {
				for j := range summaries[i].Paths {
					if summaries[i].Paths[j].Delivery.ObjectType == energyPathBaseboardConvectiveElectricType && summaries[i].Paths[j].Delivery.ObjectName == target.KeyValue {
						path = &summaries[i].Paths[j]
						summary = &summaries[i]
					}
				}
			}
			if path == nil {
				t.Fatal("typed original path missing")
			}
			switch mutation {
			case "duplicate":
				summary.Paths = append(summary.Paths, *path)
			case "cooling":
				path.ServiceKind = "cooling"
			case "wrong type":
				path.Delivery.ObjectType = "ZoneHVAC:WindowAirConditioner"
			case "wrong component":
				path.Delivery.ObjectIndex++
				path.Delivery.ID = fmt.Sprintf("component:%d", path.Delivery.ObjectIndex)
			case "other Zone":
				path.ZoneName = targets[1].ZoneName
			case "central air loop":
				path.AirLoop = &idf.LoopRef{ID: "foreign"}
			}
			got := buildEnergyPathNativeBaseboardPaths(doc, summaries)
			key := energyPathNativeBaseboardPathKey(target.ZoneName, target.KeyValue)
			want := 9
			if mutation != "none" {
				want = 8
				if len(got[key]) != 0 {
					t.Fatalf("%s got a valid native path: %+v", mutation, got[key])
				}
			}
			if len(got) != want {
				t.Fatalf("%s changed unrelated exact owner paths: %d want%d", mutation, len(got), want)
			}
		})
	}
}

func TestEnergyPathConvectiveBaseboardSQLContextCannotReplaceMissingZoneAir(t *testing.T) {
	path, _, _, plan, context := energyPathConvectiveBaseboardSQLFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM ReportData WHERE ReportDataDictionaryIndex BETWEEN 300 AND 308`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for _, node := range result.Nodes {
		if node.Level == "load" {
			t.Fatalf("native context fabricated a Building load: %+v", node)
		}
	}
	for _, zone := range result.ZoneResults {
		for _, node := range zone.Nodes {
			if node.Level == "load" {
				t.Fatalf("native convective context filled missing Zone-air load: %+v", node)
			}
		}
	}
	count := 0
	for _, source := range result.Sources {
		if strings.HasPrefix(source.Name, "Baseboard Total Heating ") {
			count++
			if source.DriverRole != energyDriverSourceRoleContext || source.DriverComponent != "load.baseboard_response.convective" || !energyDataSourceValueKnown(source, energySourceObservedRaw) || math.Abs(source.RawValue-2700) > 1e-9 {
				t.Fatalf("native source evidence lost while excluding fallback: %+v", source)
			}
		}
	}
	if count != 36 {
		t.Fatalf("native TotalHeating E/R M/H source count=%d want36", count)
	}
}
