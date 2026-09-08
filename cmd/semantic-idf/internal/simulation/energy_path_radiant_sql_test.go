package simulation

import (
	"database/sql"
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathRadiantSQLMonthlyAliasesUseOriginalOwnerAndModelTotal(t *testing.T) {
	for _, service := range []string{"Cooling", "Heating"} {
		for _, measure := range []string{"Energy", "Rate"} {
			t.Run(service+measure, func(t *testing.T) {
				name, unit := "Zone Radiant HVAC "+service+" "+measure, "J"
				values, monthly, total := []any{3600000.0, 7200000.0}, []float64{1, 2}, 3.0
				if measure == "Rate" {
					// Two independently chosen mean rates, integrated over the
					// actual January/February 2017 intervals (744 and 672 hours).
					unit, values, monthly, total = "W", []any{10.0, 20.0}, []float64{7.44, 13.44}, 20.88
				}
				doc := energyPathRadiantHandDocument(t)
				before := doc.String()
				plan := radiantSQLPlan(doc)
				// The executed request is not an equipment ownership authority.
				for i := range plan.OutputObjects {
					if plan.OutputObjects[i].KeyValue == "Radiant" {
						plan.OutputObjects[i].ScopeZoneName = "Other"
					}
				}
				context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
				factor, ok := context.Multipliers.resolve("Office")
				if !ok || factor.effectiveMultiplier() != 7 {
					t.Fatal("hand fixture must exercise an actual Zone multiplier of seven")
				}
				parsed := radiantSQLRead(t, radiantSQLFixture(t, name, unit, "RADIANT", values), &plan, context)
				if len(parsed.Series) != 1 || parsed.Series[0].sourceFrequency != "Monthly" || parsed.Series[0].SourceKey != "RADIANT" || parsed.Series[0].Total != total {
					t.Fatalf("exact native Monthly alias was not read: %#v", parsed.Series)
				}
				for month, want := range monthly {
					if got, found := parsed.Series[0].Monthly[month+1]; !found || got != want {
						t.Fatalf("M%d native J/W conversion = %g (%v), want %g", month+1, got, found, want)
					}
				}
				result := buildEnergyExplanationResultWithDriverContext(parsed.Series, parsed.Sources, &plan, context)
				source := radiantSQLSource(t, result.Sources)
				if source.Name != name || source.KeyValue != "RADIANT" || source.SourceUnit != unit || source.NormalizedUnit != "kWh" || source.ReportingFrequency != "Monthly" ||
					source.ZoneName != "Office" || source.RawValue != total || source.EffectiveValue != total || source.EffectiveMultiplier != 1 || source.MultiplierApplication != energyMultiplierAlreadyModelTotal ||
					!energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) ||
					!stringSliceContains(source.RelatedEntityIDs, context.RadiantLoads[0].Component.ID) {
					t.Fatalf("native radiant identity, original owner, or model-total quantity changed: %#v", source)
				}
				loads := 0
				for _, node := range result.Nodes {
					if node.Level != "load" {
						continue
					}
					loads++
					if node.ZoneName != "Office" || node.ServiceKind != strings.ToLower(service) || node.Value != total || node.ThermalBoundary != "active_surface_source" ||
						len(node.SourceIDs) != 1 || node.SourceIDs[0] != "sql-rdd-101" {
						t.Fatalf("owned surface source was re-scaled, relabeled air delivery, or contaminated: %#v", node)
					}
				}
				if loads != 1 || doc.String() != before {
					t.Fatalf("loads=%d (want one); original document preserved=%v", loads, doc.String() == before)
				}
			})
		}
	}
}

func TestEnergyPathRadiantSQLKnownZeroIsNotMissingOrInvalid(t *testing.T) {
	for _, measure := range []string{"Energy", "Rate"} {
		unit := "J"
		if measure == "Rate" {
			unit = "W"
		}
		for _, test := range []struct {
			name   string
			values []any
			change string
			known  bool
		}{
			{"literal_zero", []any{0.0, 0.0}, "", true},
			{"all_null", []any{nil, nil}, "", false},
			{"one_null", []any{0.0, nil}, "", false},
			{"one_missing", []any{0.0, 0.0}, "DELETE FROM ReportData WHERE TimeIndex=2", false},
			{"all_missing", []any{0.0, 0.0}, "DELETE FROM ReportData", false},
			{"duplicate_record", []any{0.0, 0.0}, "INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(2,101,0)", false},
			{"duplicate_month_axis", []any{0.0, 0.0}, "UPDATE Time SET Month=1,Day=31,Interval=44640 WHERE TimeIndex=2", false},
			{"nonfinite", []any{0.0, math.Inf(1)}, "", false},
		} {
			t.Run(measure+"/"+test.name, func(t *testing.T) {
				doc := energyPathRadiantHandDocument(t)
				plan := radiantSQLPlan(doc)
				context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
				name := "Zone Radiant HVAC Heating " + measure
				path := radiantSQLFixture(t, name, unit, "RADIANT", test.values)
				if test.change != "" {
					radiantSQLExec(t, path, test.change)
				}
				parsed := radiantSQLRead(t, path, &plan, context)
				source := radiantSQLSource(t, parsed.Sources)
				if source.Name != name || source.KeyValue != "RADIANT" || source.SourceUnit != unit || source.ReportingFrequency != "Monthly" || energyDataSourceValueKnown(source, energySourceObservedRaw) != test.known {
					t.Fatalf("dictionary identity or independent raw knownness lost: %#v", source)
				}
				if test.known {
					if len(parsed.Series) != 1 || len(parsed.Series[0].Monthly) != 2 || parsed.Series[0].Total != 0 {
						t.Fatalf("fully observed zero was pruned at the SQL boundary: %#v", parsed.Series)
					}
					for _, month := range []int{1, 2} {
						if value, found := parsed.Series[0].Monthly[month]; !found || value != 0 {
							t.Fatalf("known-zero M%d is absent or nonzero", month)
						}
					}
				} else if len(parsed.Series) != 0 {
					t.Fatalf("invalid/partial radiant observation became a load series: %#v", parsed.Series)
				}
				result := buildEnergyExplanationResultWithDriverContext(parsed.Series, parsed.Sources, &plan, context)
				source = radiantSQLSource(t, result.Sources)
				if energyDataSourceValueKnown(source, energySourceObservedRaw) != test.known || energyDataSourceValueKnown(source, energySourceObservedEffective) != test.known {
					t.Fatalf("build changed unknown/zero semantics: %#v", source)
				}
				fields := radiantSQLSourceWire(t, UpgradeEnergyExplanationV1(result))
				for _, field := range []string{"rawValue", "effectiveValue"} {
					value := strings.TrimSpace(string(fields[field]))
					if test.known && value != "0" || !test.known && value != "" && value != "null" {
						t.Fatalf("%s on actual v2 wire = %q, known=%v", field, value, test.known)
					}
				}
				if !test.known {
					for _, node := range result.Nodes {
						if node.Level == "load" {
							t.Fatalf("invalid source acquired a numeric load node: %#v", node)
						}
					}
				}
			})
		}
	}
}

func TestEnergyPathRadiantSQLPlanScopeCannotLaunderUnownedKey(t *testing.T) {
	for _, key := range []string{"Office", "Other", "Unknown Radiant", ""} {
		t.Run(key, func(t *testing.T) {
			doc := energyPathRadiantHandDocument(t)
			plan := radiantSQLPlan(doc)
			for i := range plan.OutputObjects {
				if strings.HasPrefix(plan.OutputObjects[i].VariableName, "Zone Radiant HVAC") {
					plan.OutputObjects[i].KeyValue = key
					plan.OutputObjects[i].ScopeZoneName = "Office"
				}
			}
			context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
			parsed := radiantSQLRead(t, radiantSQLFixture(t, "Zone Radiant HVAC Heating Energy", "J", key, []any{3600000.0, 7200000.0}), &plan, context)
			if len(parsed.Series) != 1 {
				t.Fatal("fixture must first read a real reported quantity before ownership filtering")
			}
			result := buildEnergyExplanationResultWithDriverContext(parsed.Series, parsed.Sources, &plan, context)
			source := radiantSQLSource(t, result.Sources)
			if source.KeyValue != key || source.RawValue != 3 || source.ZoneName != "" || source.InspectorSection != energyDriverInspectorSectionContext {
				t.Fatalf("unowned measured source was lost or given a forged Zone: %#v", source)
			}
			for _, node := range result.Nodes {
				if node.Level == "load" {
					t.Fatalf("scoped request invented radiant equipment ownership: %#v", node)
				}
			}
			found := false
			for _, warning := range result.Warnings {
				found = found || warning.Code == "radiant_load_owner_unresolved"
			}
			if !found {
				t.Fatal("unresolved original radiant ownership was not explained")
			}
		})
	}
}

func radiantSQLPlan(doc idf.Document) PurposeRunPlan {
	return BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
}

func radiantSQLRead(t *testing.T, path string, plan *PurposeRunPlan, context energyDriverBuildContext) energyExplanationParseResult {
	t.Helper()
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, plan, context)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func radiantSQLSource(t *testing.T, sources []EnergyDataSource) EnergyDataSource {
	t.Helper()
	var found []EnergyDataSource
	for _, source := range sources {
		if source.ID == "sql-rdd-101" {
			found = append(found, source)
		}
	}
	if len(found) != 1 {
		t.Fatalf("exact original radiant source count=%d, want one", len(found))
	}
	return found[0]
}

func radiantSQLSourceWire(t *testing.T, result EnergyExplanationResult) map[string]json.RawMessage {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Sources []map[string]json.RawMessage `json:"sources"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	for _, fields := range wire.Sources {
		if string(fields["id"]) == `"sql-rdd-101"` {
			return fields
		}
	}
	t.Fatal("source identity vanished on the actual v2 wire")
	return nil
}

func radiantSQLFixture(t *testing.T, name, unit, key string, values []any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "radiant.sql")
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
	// Same minimal schema as the existing precision/owned-component SQL
	// regressions. Only two actual Monthly rows exist; no twelve-month fill.
	for _, statement := range []string{
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,KeyValue TEXT,Name TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT)`,
		`CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,Year INTEGER,Interval REAL,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER)`,
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,3)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
		`INSERT INTO Time VALUES(1,1,31,24,0,2017,44640,3,3,NULL),(2,2,28,24,0,2017,40320,3,3,NULL)`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(101,?,?,?,0,'Monthly','Zone')`, key, name, unit); err != nil {
		t.Fatal(err)
	}
	for i, value := range values {
		if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,101,?)`, i+1, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func radiantSQLExec(t *testing.T, path, statement string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(statement); err != nil {
		t.Fatal(err)
	}
}
