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

// These literals are independent of the production role/target catalog. The
// original MTD puts crankcase in cooling and defrost in heating.
type vrfSQLConstituent struct {
	id                       int
	key, name, zone, service string
	coefficient              float64
}

func vrfSQLConstituents() []vrfSQLConstituent {
	out := []vrfSQLConstituent{}
	for owner := 1; owner <= 5; owner++ {
		cooling, heating := float64(owner), float64(owner)
		if owner == 1 {
			cooling = 0
		}
		if owner == 2 {
			heating = 0
		}
		key, zone := fmt.Sprintf("TU%d", owner), fmt.Sprintf("SPACE%d-1", owner)
		out = append(out,
			vrfSQLConstituent{100 + 2*owner, key, "Zone VRF Air Terminal Cooling Electricity Energy", zone, "cooling", cooling},
			vrfSQLConstituent{101 + 2*owner, key, "Zone VRF Air Terminal Heating Electricity Energy", zone, "heating", heating})
	}
	return append(out,
		vrfSQLConstituent{201, "VRF HEAT PUMP", "VRF Heat Pump Cooling Electricity Energy", "", "cooling", 100},
		vrfSQLConstituent{202, "VRF HEAT PUMP", "VRF Heat Pump Crankcase Heater Electricity Energy", "", "cooling", 10},
		vrfSQLConstituent{203, "VRF HEAT PUMP", "VRF Heat Pump Heating Electricity Energy", "", "heating", 200},
		vrfSQLConstituent{204, "VRF HEAT PUMP", "VRF Heat Pump Defrost Electricity Energy", "", "heating", 20})
}

func vrfSQLDocument(t *testing.T) idf.Document { t.Helper(); return energyPathVRFDocument(t) }

func vrfSQLPlan(doc idf.Document) PurposeRunPlan {
	return BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
}

// Reuse only the existing tiny SQLite schema, not its PTAC data or expectations.
// Monthly consumption coefficients sum to C124/H233/Fans50/Facility407.
// Odd/even monthly load weights deliberately reverse so annual allocation must
// sum monthly shares rather than recompute one annual load share.
func vrfSQLFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eplusout.sql")
	epath094AuditCreateZoneDirectSQL(t, path)
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
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{`DELETE FROM ReportData`, `DELETE FROM ReportDataDictionary`, `DELETE FROM "Time"`} {
		exec(query)
	}
	for _, query := range []string{
		`ALTER TABLE "Time" ADD COLUMN Year INTEGER`,
		`ALTER TABLE "Time" ADD COLUMN "Interval" REAL`,
		`ALTER TABLE "Time" ADD COLUMN IntervalType INTEGER`,
		`ALTER TABLE "Time" ADD COLUMN EnvironmentPeriodIndex INTEGER`,
		`ALTER TABLE "Time" ADD COLUMN WarmupFlag INTEGER`,
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY, EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,3)`,
	} {
		exec(query)
	}
	for month := 1; month <= 12; month++ {
		days := time.Date(2017, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		exec(`INSERT INTO "Time" (TimeIndex,Month,Day,Hour,Minute,Year,"Interval",IntervalType,EnvironmentPeriodIndex,WarmupFlag) VALUES(?,?,?,24,0,2017,?,3,3,NULL)`, month, month, days, days*1440)
	}
	add := func(index int, key, name string, meter bool, coefficient float64) {
		t.Helper()
		flag, group := 0, "HVAC"
		if meter {
			flag, group = 1, "Meter"
		}
		exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J',?,'Monthly',?)`, index, key, name, flag, group)
		for month := 1; month <= 12; month++ {
			exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, month, index, coefficient*float64(month)*3600000)
		}
	}
	for _, row := range []struct {
		id          int
		name        string
		coefficient float64
	}{
		{1, "Electricity:Facility", 407}, {2, "Cooling:Electricity", 124}, {3, "Heating:Electricity", 233}, {4, "Fans:Electricity", 50},
	} {
		add(row.id, "", row.name, true, row.coefficient)
	}
	for _, row := range vrfSQLConstituents() {
		add(row.id, row.key, row.name, false, row.coefficient)
	}
	for owner := 1; owner <= 5; owner++ {
		zone := fmt.Sprintf("SPACE%d-1", owner)
		add(300+owner*2, zone, "Zone Air System Sensible Cooling Energy", false, 10*float64(owner))
		add(301+owner*2, zone, "Zone Air System Sensible Heating Energy", false, 10*float64(6-owner))
		for month := 2; month <= 12; month += 2 {
			exec(`UPDATE ReportData SET Value=? WHERE TimeIndex=? AND ReportDataDictionaryIndex=?`, 10*float64(6-owner)*float64(month)*3600000, month, 300+owner*2)
			exec(`UPDATE ReportData SET Value=? WHERE TimeIndex=? AND ReportDataDictionaryIndex=?`, 10*float64(owner)*float64(month)*3600000, month, 301+owner*2)
		}
	}
	add(390, "PLENUM-1", "Zone Air System Sensible Cooling Energy", false, 0)
	add(391, "PLENUM-1", "Zone Air System Sensible Heating Energy", false, 0)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func vrfSQLRead(t *testing.T, path string, doc idf.Document, plan *PurposeRunPlan) energyExplanationParseResult {
	t.Helper()
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func vrfSQLCohort(t *testing.T, parsed energyExplanationParseResult) energyPathVRFConsumptionCohort {
	t.Helper()
	if len(parsed.VRFConsumption) != 1 {
		t.Fatalf("VRF cohorts=%d, want one original shared system", len(parsed.VRFConsumption))
	}
	return parsed.VRFConsumption[0]
}

func vrfSQLObservation(t *testing.T, cohort energyPathVRFConsumptionCohort, id int) energyPathVRFConsumptionObservation {
	t.Helper()
	var found []energyPathVRFConsumptionObservation
	for _, observation := range cohort.Observations {
		if observation.Source.ID == fmt.Sprintf("sql-rdd-%d", id) {
			found = append(found, observation)
		}
	}
	if len(found) != 1 {
		t.Fatalf("original source %d observations=%d, want1", id, len(found))
	}
	return found[0]
}

func vrfSQLClose(t *testing.T, label string, actual, want float64) {
	t.Helper()
	if math.IsNaN(actual) || math.IsInf(actual, 0) || math.Abs(actual-want) > 1e-10 {
		t.Errorf("%s=%0.15g want %0.15g", label, actual, want)
	}
}

func TestEnergyPathVRFSQLFourteenObservedConstituents(t *testing.T) {
	doc := vrfSQLDocument(t)
	before := doc.String()
	plan := vrfSQLPlan(doc)
	parsed := vrfSQLRead(t, vrfSQLFixture(t), doc, &plan)
	cohort := vrfSQLCohort(t, parsed)
	if !cohort.Requested || len(cohort.Months) != 12 || len(cohort.Observations) != 14 || len(cohort.System.Terminals) != 5 {
		t.Fatalf("incomplete original cohort %#v", cohort)
	}
	for _, want := range vrfSQLConstituents() {
		got := vrfSQLObservation(t, cohort, want.id)
		if !got.Requested || got.Invalid || len(got.InvalidMonths) != 0 || len(got.Monthly) != 12 {
			t.Errorf("valid identity %d was rejected: %#v", want.id, got)
		}
		if !strings.EqualFold(got.Target.KeyValue, want.key) || got.Target.ZoneName != want.zone || got.Target.Definition.Shared != (want.zone == "") || got.Target.Definition.Energy.EndUse != want.service || got.Target.Definition.Energy.Carrier != "electricity" {
			t.Errorf("source %d original ownership/carrier/service changed: %#v", want.id, got.Target)
		}
		if got.Source.Name != want.name || !strings.EqualFold(got.Source.KeyValue, want.key) || got.Source.ReportingFrequency != "Monthly" || got.Source.SourceUnit != "J" || got.Source.NormalizedUnit != "kWh" {
			t.Errorf("source %d metadata %#v", want.id, got.Source)
		}
		if got.Source.SourceType != "sql_report_data" || got.Source.IsMeter || got.Source.ZoneName != want.zone {
			t.Errorf("source %d lost original variable/owner provenance: %#v", want.id, got.Source)
		}
		// Generated Monthly requests have no original input object index.
		// The equipment index belongs to Target, never to the Output jump.
		if got.Source.ObjectIndex != nil {
			t.Errorf("source %d invented an Output request index %d from physical equipment %d", want.id, *got.Source.ObjectIndex, got.Target.ObjectIndex)
		}
		if !energyDataSourceValueKnown(got.Source, energySourceObservedRaw) || !energyDataSourceValueKnown(got.Source, energySourceObservedEffective) {
			t.Errorf("observed source %d lost knownness (including reported zero)", want.id)
		}
		vrfSQLClose(t, "source annual", got.Source.RawValue, want.coefficient*78)
		vrfSQLClose(t, "source effective annual", got.Source.EffectiveValue, want.coefficient*78)
		for month := 1; month <= 12; month++ {
			value, exists := got.Monthly[month]
			if !cohort.Months[month] || !exists {
				t.Errorf("source %d lost actual month %d (zero is present)", want.id, month)
			}
			vrfSQLClose(t, "native month", value, want.coefficient*float64(month))
		}
	}
	for _, item := range parsed.Series {
		for _, id := range item.SourceIDs {
			for _, want := range vrfSQLConstituents() {
				if id == fmt.Sprintf("sql-rdd-%d", want.id) {
					t.Errorf("constituent %s was also promoted into ordinary additive series", id)
				}
			}
		}
	}
	if before != doc.String() {
		t.Fatal("reader mutated original model")
	}
}

func TestEnergyPathVRFSQLSourceIndexIsExactMonthlyRequestNotEquipment(t *testing.T) {
	doc := vrfSQLDocument(t)
	path := vrfSQLFixture(t)
	physicalIndex := directHVACFixtureObject(t, &doc, "AirConditioner:VariableRefrigerantFlow", "VRF Heat Pump").Index
	for _, tc := range []struct {
		name               string
		monthlyIndex       bool
		otherFrequency     bool
		conflictingMonthly bool
	}{
		{name: "exact Monthly request", monthlyIndex: true},
		{name: "other frequency cannot override Monthly", monthlyIndex: true, otherFrequency: true},
		{name: "unindexed Monthly cannot borrow Timestep", otherFrequency: true},
		{name: "conflicting Monthly requests rejected", monthlyIndex: true, conflictingMonthly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := vrfSQLPlan(doc)
			requestPosition := -1
			for i, output := range plan.OutputObjects {
				if output.ObjectType == "Output:Variable" && strings.EqualFold(output.KeyValue, "VRF Heat Pump") &&
					output.VariableName == "VRF Heat Pump Cooling Electricity Energy" && output.ReportingFrequency == "Monthly" {
					if requestPosition != -1 {
						t.Fatal("fixture has duplicate original Monthly requests")
					}
					requestPosition = i
				}
			}
			if requestPosition < 0 {
				t.Fatal("fixture has no exact generated Monthly request")
			}
			monthlyIndex, timestepIndex, conflictIndex := 12345, 54321, 23456
			if tc.monthlyIndex {
				plan.OutputObjects[requestPosition].ObjectIndex = &monthlyIndex
			}
			if tc.otherFrequency {
				timestep := plan.OutputObjects[requestPosition]
				timestep.ReportingFrequency, timestep.ObjectIndex = "Timestep", &timestepIndex
				// Put the other-frequency request first to expose first-match bugs.
				plan.OutputObjects = append(plan.OutputObjects, timestep)
				copy(plan.OutputObjects[1:], plan.OutputObjects[:len(plan.OutputObjects)-1])
				plan.OutputObjects[0] = timestep
				requestPosition++
			}
			if tc.conflictingMonthly {
				duplicate := plan.OutputObjects[requestPosition]
				duplicate.ObjectIndex = &conflictIndex
				plan.OutputObjects = append(plan.OutputObjects, duplicate)
			}
			parsed := vrfSQLRead(t, path, doc, &plan)
			cohort := vrfSQLCohort(t, parsed)
			if tc.conflictingMonthly {
				if cohort.Requested || len(cohort.Observations) != 13 {
					t.Fatal("ambiguous Output request became a complete observed constituent")
				}
				for _, source := range parsed.Sources {
					if source.ID == "sql-rdd-201" {
						t.Fatal("ambiguous request acquired a source jump")
					}
				}
				return
			}
			got := vrfSQLObservation(t, cohort, 201)
			if !cohort.Requested || got.Target.ObjectIndex != physicalIndex || got.Target.ObjectIndex == monthlyIndex {
				t.Fatal("request index replaced original physical equipment ownership")
			}
			if tc.monthlyIndex {
				if got.Source.ObjectIndex == nil || *got.Source.ObjectIndex != 12345 {
					t.Fatalf("source index=%v, want exact Monthly request 12345", got.Source.ObjectIndex)
				}
				monthlyIndex = 99999
				if *got.Source.ObjectIndex != 12345 {
					t.Fatal("source retained a mutable pointer into the output plan")
				}
			} else if got.Source.ObjectIndex != nil {
				t.Fatalf("unindexed Monthly borrowed equipment/Timestep index %d", *got.Source.ObjectIndex)
			}
			vrfSQLClose(t, "unchanged observed annual", got.Source.RawValue, 7800)
			vrfSQLClose(t, "unchanged original January", got.Monthly[1], 100)
		})
	}
}

func vrfSQLMutate(t *testing.T, path string, statements ...string) {
	t.Helper()
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
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func vrfSQLAssertUnknown(t *testing.T, source EnergyDataSource) {
	t.Helper()
	if energyDataSourceValueKnown(source, energySourceObservedRaw) || energyDataSourceValueKnown(source, energySourceObservedEffective) {
		t.Errorf("incomplete/invalid source acquired known annual value: %#v", source)
	}
}

func TestEnergyPathVRFSQLNativePrecisionKnownZeroAndModelTotal(t *testing.T) {
	doc := vrfSQLDocument(t)
	plan := vrfSQLPlan(doc)
	path := vrfSQLFixture(t)
	vrfSQLMutate(t, path,
		`UPDATE ReportData SET Value=1476 WHERE ReportDataDictionaryIndex=103 AND TimeIndex=1`,
		`UPDATE ReportData SET Value=1512 WHERE ReportDataDictionaryIndex=103 AND TimeIndex=2`)
	baseline := vrfSQLCohort(t, vrfSQLRead(t, path, doc, &plan))
	precision := vrfSQLObservation(t, baseline, 103)
	vrfSQLClose(t, "unrounded month1", precision.Monthly[1], .00041)
	vrfSQLClose(t, "unrounded month2", precision.Monthly[2], .00042)
	vrfSQLClose(t, "source annual rounded once", precision.Source.RawValue, 75.001)
	zero := vrfSQLObservation(t, baseline, 102)
	if !energyDataSourceValueKnown(zero.Source, energySourceObservedRaw) || zero.Source.RawValue != 0 || len(zero.Monthly) != 12 {
		t.Fatal("twelve observed zeros became missing")
	}
	directHVACFixtureObject(t, &doc, "Zone", "SPACE1-1").Fields[6].Value = "7"
	changed := vrfSQLCohort(t, vrfSQLRead(t, path, doc, &plan))
	for _, row := range vrfSQLConstituents() {
		a, b := vrfSQLObservation(t, baseline, row.id), vrfSQLObservation(t, changed, row.id)
		if !reflect.DeepEqual(a.Monthly, b.Monthly) || a.Source.RawValue != b.Source.RawValue || a.Source.EffectiveValue != b.Source.EffectiveValue {
			t.Errorf("model-total source %d multiplied again", row.id)
		}
		if b.Source.AggregationBasis != "model_total" || b.Source.EffectiveMultiplier != 1 {
			t.Errorf("source %d lost modeled-total metadata: %#v", row.id, b.Source)
		}
	}
	vrfSQLMutate(t, path, `UPDATE ReportData SET Value=1440 WHERE ReportDataDictionaryIndex=102 AND TimeIndex=1`)
	tiny := vrfSQLObservation(t, vrfSQLCohort(t, vrfSQLRead(t, path, doc, &plan)), 102)
	vrfSQLClose(t, "tiny native remains positive", tiny.Monthly[1], .0004)
	if !energyDataSourceValueKnown(tiny.Source, energySourceObservedRaw) {
		t.Error("sub-presentation positive native source lost observation proof")
	}
}

func TestEnergyPathVRFSQLInvalidMonthlyPreservesOtherObservations(t *testing.T) {
	doc := vrfSQLDocument(t)
	plan := vrfSQLPlan(doc)
	for _, tc := range []struct {
		name  string
		id    int
		query string
	}{
		{"missing known-zero month", 102, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=102 AND TimeIndex=7`},
		{"missing outdoor month", 201, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=201 AND TimeIndex=7`},
		{"null outdoor month", 201, `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=201 AND TimeIndex=7`},
		{"duplicate outdoor month", 201, `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(7,201,1)`},
		{"negative outdoor month", 201, `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=201 AND TimeIndex=7`},
		{"infinite outdoor month", 201, `UPDATE ReportData SET Value=1e999 WHERE ReportDataDictionaryIndex=201 AND TimeIndex=7`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := vrfSQLFixture(t)
			vrfSQLMutate(t, path, tc.query)
			cohort := vrfSQLCohort(t, vrfSQLRead(t, path, doc, &plan))
			if !cohort.Requested || len(cohort.Observations) != 14 {
				t.Fatal("bad data must not erase an original dictionary or its exact request")
			}
			bad := vrfSQLObservation(t, cohort, tc.id)
			if bad.Invalid {
				t.Error("one bad monthly observation must not invalidate the original identity and its other valid months")
			}
			if !bad.InvalidMonths[7] {
				t.Errorf("bad month7 not marked invalid: %#v", bad)
			}
			vrfSQLAssertUnknown(t, bad.Source)
			coefficient := 100.0
			if tc.id == 102 {
				coefficient = 0
			}
			value, ok := bad.Monthly[6]
			if !ok || bad.InvalidMonths[6] {
				t.Error("unrelated observed month was discarded")
			}
			vrfSQLClose(t, "valid month6", value, coefficient*6)
			for _, id := range []int{202, 203, 204} {
				other := vrfSQLObservation(t, cohort, id)
				if other.Invalid || len(other.InvalidMonths) != 0 || len(other.Monthly) != 12 || !energyDataSourceValueKnown(other.Source, energySourceObservedRaw) {
					t.Errorf("valid sibling %d was lost: %#v", id, other)
				}
			}
		})
	}
}

func TestEnergyPathVRFSQLDictionaryAbsenceNullAndDuplicateStayDistinct(t *testing.T) {
	doc := vrfSQLDocument(t)
	plan := vrfSQLPlan(doc)
	for _, tc := range []struct {
		name       string
		statements []string
		dictionary bool
	}{
		{"all null", []string{`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=201`}, true},
		{"no reported rows", []string{`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=201`}, true},
		{"no dictionary", []string{`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=201`, `DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=201`}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := vrfSQLFixture(t)
			vrfSQLMutate(t, path, tc.statements...)
			parsed := vrfSQLRead(t, path, doc, &plan)
			cohort := vrfSQLCohort(t, parsed)
			if tc.dictionary {
				got := vrfSQLObservation(t, cohort, 201)
				vrfSQLAssertUnknown(t, got.Source)
				for month := 1; month <= 12; month++ {
					if !got.InvalidMonths[month] {
						t.Errorf("unknown month%d became reported zero", month)
					}
				}
				if len(got.Monthly) != 0 {
					t.Error("unobserved values were invented")
				}
			} else {
				if len(cohort.Observations) != 13 {
					t.Errorf("absent dictionary must not be invented: observations%d", len(cohort.Observations))
				}
				for _, got := range cohort.Observations {
					if got.Source.ID == "sql-rdd-201" || got.Target.Definition.ID == "cooling.vrf.outdoor_electricity" {
						t.Error("missing original dictionary was synthesized")
					}
				}
				for _, source := range parsed.Sources {
					if source.ID == "sql-rdd-201" {
						t.Error("missing dictionary became a global source")
					}
				}
			}
			if !energyDataSourceValueKnown(vrfSQLObservation(t, cohort, 202).Source, energySourceObservedRaw) {
				t.Error("observed crankcase context was removed")
			}
		})
	}
	t.Run("two native identities cannot sum", func(t *testing.T) {
		path := vrfSQLFixture(t)
		vrfSQLMutate(t, path,
			`INSERT INTO ReportDataDictionary SELECT 8201,KeyValue,Name,Units,IsMeter,ReportingFrequency,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=201`,
			`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,8201,Value FROM ReportData WHERE ReportDataDictionaryIndex=201`)
		cohort := vrfSQLCohort(t, vrfSQLRead(t, path, doc, &plan))
		if len(cohort.Observations) != 15 {
			t.Error("duplicate native identities should remain individually inspectable")
		}
		for _, id := range []int{201, 8201} {
			got := vrfSQLObservation(t, cohort, id)
			if !got.Invalid {
				t.Errorf("duplicate identity%d remained usable", id)
			}
			vrfSQLAssertUnknown(t, got.Source)
		}
	})
}

func TestEnergyPathVRFSQLRequiresExactRequestAndReportingIdentity(t *testing.T) {
	doc := vrfSQLDocument(t)
	path := vrfSQLFixture(t)
	for _, kind := range []string{"nil plan", "missing request", "wrong zone", "wrong purpose", "wrong frequency", "wrong key", "shared scoped as local"} {
		t.Run(kind, func(t *testing.T) {
			plan := vrfSQLPlan(doc)
			planPtr := &plan
			if kind == "nil plan" {
				planPtr = nil
			} else {
				matched := false
				for i := range plan.OutputObjects {
					output := &plan.OutputObjects[i]
					key, name := "TU1", "Zone VRF Air Terminal Cooling Electricity Energy"
					if kind == "shared scoped as local" {
						key, name = "VRF Heat Pump", "VRF Heat Pump Cooling Electricity Energy"
					}
					if !strings.EqualFold(output.KeyValue, key) || output.VariableName != name {
						continue
					}
					matched = true
					switch kind {
					case "missing request":
						plan.OutputObjects = append(plan.OutputObjects[:i], plan.OutputObjects[i+1:]...)
					case "wrong zone", "shared scoped as local":
						output.ScopeZoneName = "SPACE2-1"
					case "wrong purpose":
						output.PurposeIDs = []SimulationPurposeID{SimulationPurposeZoneHeatFlow}
					case "wrong frequency":
						output.ReportingFrequency = "Timestep"
					case "wrong key":
						output.KeyValue = "TU2"
					}
					break
				}
				if !matched {
					t.Fatal("fixture did not mutate an actual request")
				}
			}
			parsed := vrfSQLRead(t, path, doc, planPtr)
			for _, cohort := range parsed.VRFConsumption {
				if cohort.Requested {
					t.Errorf("%s laundered into complete exact14 request proof", kind)
				}
				if kind == "missing request" {
					if len(cohort.Observations) != 13 {
						t.Fatalf("one unrequested target changed %d observations, want 13 valid siblings", len(cohort.Observations))
					}
					for _, observation := range cohort.Observations {
						if observation.Source.ID == "sql-rdd-102" {
							t.Error("missing exact target request acquired an observation")
						}
						if !observation.Requested || observation.Invalid || len(observation.InvalidMonths) != 0 ||
							len(observation.Monthly) != 12 || !energyDataSourceValueKnown(observation.Source, energySourceObservedRaw) {
							t.Errorf("unrelated missing request invalidated observed sibling %s", observation.Source.ID)
						}
					}
				}
			}
		})
	}
	for _, tc := range []struct{ name, query string }{
		{"wrong unit", `UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=201`},
		{"wrong meter flag", `UPDATE ReportDataDictionary SET IsMeter=1 WHERE ReportDataDictionaryIndex=201`},
		{"wrong carrier name", `UPDATE ReportDataDictionary SET Name='VRF Heat Pump Cooling NaturalGas Energy' WHERE ReportDataDictionaryIndex=201`},
		{"wrong service constituent", `UPDATE ReportDataDictionary SET Name='VRF Heat Pump Heating Electricity Energy' WHERE ReportDataDictionaryIndex=202`},
		{"unrelated owner key", `UPDATE ReportDataDictionary SET KeyValue='ALIEN VRF' WHERE ReportDataDictionaryIndex=201`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := vrfSQLFixture(t)
			vrfSQLMutate(t, path, tc.query)
			plan := vrfSQLPlan(doc)
			cohort := vrfSQLCohort(t, vrfSQLRead(t, path, doc, &plan))
			wantedID := "sql-rdd-201"
			if tc.name == "wrong service constituent" {
				wantedID = "sql-rdd-202"
			}
			for _, observation := range cohort.Observations {
				if observation.Source.ID != wantedID {
					continue
				}
				if !observation.Invalid {
					t.Errorf("wrong reporting identity is still usable: %#v", observation)
				}
				vrfSQLAssertUnknown(t, observation.Source)
			}
		})
	}
}

func TestEnergyPathVRFSQLActualMonthlyAxisNotAssumedAnnualCalendar(t *testing.T) {
	doc := vrfSQLDocument(t)
	plan := vrfSQLPlan(doc)
	path := vrfSQLFixture(t)
	vrfSQLMutate(t, path,
		`DELETE FROM ReportData WHERE TimeIndex>3`, `DELETE FROM "Time" WHERE TimeIndex>3`,
		`UPDATE "Time" SET Year=2024`, `UPDATE "Time" SET Day=29,"Interval"=41760 WHERE Month=2`)
	cohort := vrfSQLCohort(t, vrfSQLRead(t, path, doc, &plan))
	if len(cohort.Months) != 3 || len(cohort.Observations) != 14 || !cohort.Requested {
		t.Fatal("legitimate three-month leap-year run was rejected")
	}
	for _, row := range vrfSQLConstituents() {
		got := vrfSQLObservation(t, cohort, row.id)
		if got.Invalid || len(got.InvalidMonths) != 0 || len(got.Monthly) != 3 || !energyDataSourceValueKnown(got.Source, energySourceObservedRaw) {
			t.Errorf("fully observed short source%d became unknown", row.id)
		}
		vrfSQLClose(t, "actual run total", got.Source.RawValue, row.coefficient*6)
	}
	vrfSQLMutate(t, path, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=102 AND TimeIndex=2`)
	missing := vrfSQLObservation(t, vrfSQLCohort(t, vrfSQLRead(t, path, doc, &plan)), 102)
	if !missing.InvalidMonths[2] {
		t.Error("one missing observed-zero month was masked by scalar zero")
	}
	vrfSQLAssertUnknown(t, missing.Source)

	for _, tc := range []struct {
		name       string
		statements []string
	}{
		{"unknown year", []string{`UPDATE "Time" SET Year=NULL WHERE TimeIndex=2`}},
		{"invalid interval", []string{`UPDATE "Time" SET "Interval"=0 WHERE TimeIndex=2`}},
		{"duplicate native month", []string{`INSERT INTO "Time" SELECT 50,Month,Day,Hour,Minute,Year,"Interval",IntervalType,EnvironmentPeriodIndex,WarmupFlag FROM "Time" WHERE TimeIndex=2`}},
		{"no environment schema", []string{`DROP TABLE EnvironmentPeriods`}},
		{"sizing rows without weather axis", []string{`INSERT INTO EnvironmentPeriods VALUES(2,2)`, `UPDATE "Time" SET EnvironmentPeriodIndex=2`}},
		{"warmup observations without weather axis", []string{`UPDATE "Time" SET WarmupFlag=1`}},
		{"wrong interval type", []string{`UPDATE "Time" SET IntervalType=-1 WHERE TimeIndex=2`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := vrfSQLFixture(t)
			vrfSQLMutate(t, path, tc.statements...)
			parsed := vrfSQLRead(t, path, doc, &plan)
			for _, cohort := range parsed.VRFConsumption {
				for _, observation := range cohort.Observations {
					vrfSQLAssertUnknown(t, observation.Source)
				}
			}
		})
	}
	t.Run("exclude extra sizing and warmup without corrupting weather", func(t *testing.T) {
		path := vrfSQLFixture(t)
		vrfSQLMutate(t, path,
			`INSERT INTO EnvironmentPeriods VALUES(2,2)`,
			`INSERT INTO "Time" SELECT 80,Month,Day,Hour,Minute,Year,"Interval",IntervalType,2,0 FROM "Time" WHERE TimeIndex=2`,
			`INSERT INTO "Time" SELECT 81,Month,Day,Hour,Minute,Year,"Interval",IntervalType,3,1 FROM "Time" WHERE TimeIndex=2`,
			`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(80,201,999999999),(81,201,888888888)`)
		cohort := vrfSQLCohort(t, vrfSQLRead(t, path, doc, &plan))
		got := vrfSQLObservation(t, cohort, 201)
		if len(cohort.Months) != 12 || got.Invalid || len(got.InvalidMonths) != 0 || !energyDataSourceValueKnown(got.Source, energySourceObservedRaw) {
			t.Fatal("excluded non-weather records corrupted a complete weather observation")
		}
		vrfSQLClose(t, "weather annual only", got.Source.RawValue, 7800)
		vrfSQLClose(t, "weather February only", got.Monthly[2], 200)
	})
}

func TestEnergyPathVRFSQLMonthlyWinsWithoutDiscardingOriginalFanTimestep(t *testing.T) {
	doc := vrfSQLDocument(t)
	plan := vrfSQLPlan(doc)
	path := vrfSQLFixture(t)
	baseline := vrfSQLCohort(t, vrfSQLRead(t, path, doc, &plan))
	statements := []string{
		`INSERT INTO ReportDataDictionary VALUES(501,'TU1 VRF SUPPLY FAN','Fan Electricity Rate','W',0,'Zone Timestep','HVAC')`,
		`INSERT INTO ReportDataDictionary VALUES(502,'TU1','Zone VRF Air Terminal Cooling Electricity Energy','J',0,'Zone Timestep','HVAC')`,
		`INSERT INTO ReportDataDictionary VALUES(503,'VRF HEAT PUMP','VRF Heat Pump Cooling Electricity Energy','J',0,'Zone Timestep','HVAC')`,
	}
	for index, power := range []int{100, 300, 500, 700} {
		ti := 1001 + index
		statements = append(statements,
			fmt.Sprintf(`INSERT INTO "Time" (TimeIndex,Month,Day,Hour,Minute,Year,"Interval",IntervalType,EnvironmentPeriodIndex,WarmupFlag) VALUES(%d,1,1,1,%d,2017,15,-1,3,0)`, ti, 15*(index+1)),
			fmt.Sprintf(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(%d,501,%d),(%d,502,900000),(%d,503,1800000)`, ti, power, ti, ti))
	}
	vrfSQLMutate(t, path, statements...)
	parsed := vrfSQLRead(t, path, doc, &plan)
	changed := vrfSQLCohort(t, parsed)
	if len(changed.Observations) != 14 || len(changed.Months) != 12 {
		t.Fatal("Timestep aliases were promoted into extra Monthly constituents")
	}
	for _, row := range vrfSQLConstituents() {
		a, b := vrfSQLObservation(t, baseline, row.id), vrfSQLObservation(t, changed, row.id)
		if !reflect.DeepEqual(a.Monthly, b.Monthly) || a.Source.RawValue != b.Source.RawValue || a.Invalid != b.Invalid || !reflect.DeepEqual(a.InvalidMonths, b.InvalidMonths) {
			t.Errorf("timestep alias changed Monthly authority %d", row.id)
		}
	}
	series, err := parseSimulationSQLSeries(path)
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, item := range series {
		if item.KeyValue != "TU1 VRF SUPPLY FAN" || item.Name != "Fan Electricity Rate" {
			continue
		}
		seen = true
		// SimulationSeries.RowCount is the shared display axis (12 Monthly
		// frames plus four timestep frames), not this source's observation count.
		if item.ReportingFrequency != "Zone Timestep" || item.RowCount != 16 || len(item.Points) != 4 || item.Column != "TU1 VRF SUPPLY FAN:Fan Electricity Rate [W]" || item.IsMeter == nil || *item.IsMeter {
			t.Errorf("original fan reporting identity or samples lost: %#v", item)
		}
		vrfSQLClose(t, "fan W min", item.Min, 100)
		vrfSQLClose(t, "fan W max", item.Max, 700)
		vrfSQLClose(t, "fan W mean", item.Average, 400)
		for index, want := range []float64{100, 300, 500, 700} {
			if len(item.Points) > index {
				vrfSQLClose(t, "original fan W sample", item.Points[index].Value, want)
			}
		}
	}
	if !seen {
		t.Fatal("Monthly VRF consumption ingestion discarded unrelated original Fan W")
	}
	// This is only four original samples, not proof of a complete annual fan
	// energy pool. Neither their .4kWh integral nor the VRF aliases may become
	// additive ordinary site series in this reader-only slice.
	for _, item := range parsed.Series {
		for _, id := range item.SourceIDs {
			if id == "sql-rdd-501" || id == "sql-rdd-502" || id == "sql-rdd-503" {
				t.Errorf("unverified Timestep context became ordinary site energy: %s", id)
			}
		}
	}
}
