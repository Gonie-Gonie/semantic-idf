package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"sort"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// These deliberately non-cancelling inputs are handwritten native-Zone kWh.
// They exercise the ordinary SQL reader and multiplier pass, not a shadow
// constructed with an already-applied multiplier or candidate-derived values.
func TestEnergyDriverMonthlyPrecisionMultiplierGenuineSignedResidual(t *testing.T) {
	for _, tc := range []struct {
		name       string
		residual   float64
		multiplier int
		want       float64
	}{
		{"positive before multiplier", .0004, 10, .004},
		{"negative before multiplier", -.0004, 10, -.004},
		{"positive final rounding", .0046, 1, .005},
		{"negative final rounding", -.0046, 1, -.005},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := driverPrecisionMultiplierResult(t, tc.multiplier, [4]float64{100 + tc.residual, 10, 20, 70})
			driverPrecisionAssertSignedResidual(t, result, tc.want)
		})
	}
}

func TestEnergyDriverMonthlyPrecisionMultiplierEntireTinyAggregateAndKnownZero(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values [4]float64
		want   float64
	}{
		{"tiny aggregate minus observed zero", [4]float64{.0004, 0, 0, 0}, .004},
		{"observed zero minus tiny detail", [4]float64{0, .0004, 0, 0}, -.004},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := driverPrecisionMultiplierResult(t, 10, tc.values)
			for _, id := range []string{"sql-rdd-1", "sql-rdd-2", "sql-rdd-3", "sql-rdd-4"} {
				source := driverPrecisionMultiplierSource(t, result, id)
				if source.RawValue != 0 {
					t.Fatalf("%s unexpectedly bypassed the existing native 3dp display boundary: %.17g", id, source.RawValue)
				}
			}
			driverPrecisionAssertSignedResidual(t, result, tc.want)
		})
	}
}

func TestEnergyDriverMonthlyPrecisionMultiplierLeavesSiteRoundingUnchanged(t *testing.T) {
	unit := driverPrecisionMultiplierResult(t, 1, [4]float64{100.0004, 10, 20, 70})
	weighted := driverPrecisionMultiplierResult(t, 10, [4]float64{100.0004, 10, 20, 70})
	for _, result := range []EnergyExplanationResult{unit, weighted} {
		for _, id := range []string{"sql-rdd-7", "sql-rdd-8"} {
			source := driverPrecisionMultiplierSource(t, result, id)
			if source.RawValue != 17.891 || source.EffectiveValue != 17.891 || source.EffectiveMultiplier != 1 {
				t.Fatalf("%s site meter changed its independent model-total 3dp boundary: %#v", id, source)
			}
		}
		for _, period := range []string{"M8", "annual"} {
			nodes, _ := epath080Graph(t, result, "", period)
			foundEndUse, foundCarrier := 0, 0
			for _, node := range nodes {
				if node.Level == "end_use" && node.EndUse == "equipment" {
					foundEndUse++
					if node.Value != 17.891 {
						t.Fatalf("%s end use changed from ordinary rounded meter: %#v", period, node)
					}
				}
				if node.Level == "carrier" && node.Carrier == "electricity" {
					foundCarrier++
					if node.Value != 17.891 {
						t.Fatalf("%s facility changed from ordinary rounded meter: %#v", period, node)
					}
				}
			}
			if foundEndUse != 1 || foundCarrier != 1 {
				t.Fatalf("%s expected one exact meter end-use/carrier, got %d/%d", period, foundEndUse, foundCarrier)
			}
		}
	}
}

func driverPrecisionAssertSignedResidual(t *testing.T, result EnergyExplanationResult, want float64) {
	t.Helper()
	sourceID := "derived-driver-internal_reconciliation_sensible-north_zone"
	source := driverPrecisionMultiplierSource(t, result, sourceID)
	operands := append([]string(nil), source.InputSourceIDs...)
	sort.Strings(operands)
	if source.EffectiveValue != want || source.SourceType != "derived_formula" ||
		!reflect.DeepEqual(operands, []string{"sql-rdd-1", "sql-rdd-2", "sql-rdd-3", "sql-rdd-4"}) {
		t.Fatalf("real signed residual lost magnitude, sign, or original operands: want %.3f, got %#v", want, source)
	}
	service := "cooling"
	if want < 0 {
		service = "heating"
	}
	for _, zone := range []string{"NORTH ZONE", ""} {
		for _, period := range []string{"M8", "annual"} {
			nodes, _ := epath080Graph(t, result, zone, period)
			found := 0
			for _, node := range nodes {
				if node.Level != "driver" {
					continue
				}
				ownsResidual := false
				for _, id := range node.SourceIDs {
					ownsResidual = ownsResidual || id == sourceID
				}
				if !ownsResidual {
					continue
				}
				found++
				// The reviewed presentation taxonomy folds internal.other into
				// storage_other, potentially with other same-direction pressure.
				// Its exact residual is checked above, before that category sum.
				if node.DriverCategory != energyDriverCategoryStorageOther || node.ServiceKind != service ||
					node.EffectiveValue < math.Abs(want) || node.SignedValue*want <= 0 || node.Value <= 0 {
					t.Fatalf("%q/%s lost genuine signed %.3f pressure or direction: %#v", zone, period, want, node)
				}
			}
			if found != 1 {
				t.Fatalf("%q/%s expected one visible driver with exact residual source, got %d", zone, period, found)
			}
		}
	}
}

func driverPrecisionMultiplierSource(t *testing.T, result EnergyExplanationResult, id string) EnergyDataSource {
	t.Helper()
	var out EnergyDataSource
	found := 0
	for _, source := range result.Sources {
		if source.ID == id {
			out, found = source, found+1
		}
	}
	if found != 1 {
		t.Fatalf("expected one source %q, got %d", id, found)
	}
	return out
}

func driverPrecisionMultiplierResult(t *testing.T, multiplier int, values [4]float64) EnergyExplanationResult {
	t.Helper()
	path := driverPrecisionSQL(t)
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
	for i, value := range values {
		if _, err := tx.Exec(`UPDATE ReportData SET Value=? WHERE TimeIndex=8 AND ReportDataDictionaryIndex=?`, value*3600000, i+1); err != nil {
			t.Fatal(err)
		}
	}
	// Both delivered services are substantial so the test can observe a genuine
	// tiny pressure after allocation without turning presentation pruning into
	// a source-knownness assumption. Every other month remains observed zero.
	if _, err := tx.Exec(`UPDATE ReportData SET Value=3600000000 WHERE TimeIndex=8 AND ReportDataDictionaryIndex=5`); err != nil {
		t.Fatal(err)
	}
	for _, source := range []struct {
		id      int
		name    string
		key     string
		isMeter int
		kWh     float64
	}{
		{6, "Zone Air System Sensible Cooling Energy", "NORTH ZONE", 0, 1000},
		{7, "InteriorEquipment:Electricity", "", 1, 17.891234},
		{8, "Electricity:Facility", "", 1, 17.891234},
	} {
		if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J',?,'Monthly','Zone')`, source.id, source.key, source.name, source.isMeter); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,?,CASE WHEN Month=8 THEN ? ELSE 0 END FROM Time`, source.id, source.kWh*3600000); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	doc := parsePurposePlanFixture(t, fmt.Sprintf("Zone,NORTH ZONE,0,0,0,0,1,%d;", multiplier))
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	return UpgradeEnergyExplanationV1(legacy)
}
