package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLBaseboardContextUnit(t *testing.T, name, frequency string, value func(int) float64) (string, epathRealSQLModel, PurposeRunPlan, []epathRealSQLSource) {
	t.Helper()
	unit, _, _, _, err := epathSQLBaseboardContextClass(name, frequency)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "baseboard.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER,EnvironmentName TEXT,EnvironmentType INTEGER)`,
		`INSERT INTO EnvironmentPeriods VALUES(3,'Annual',3)`,
		`CREATE TABLE "Time"(TimeIndex INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,Year INTEGER,"Interval" REAL,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER,SimulationDays INTEGER)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER,KeyValue TEXT,Name TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
	} {
		if _, err := db.Exec(query); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(40,'Baseboard',?,?,0,?,'HVAC')`, name, unit, frequency); err != nil {
		db.Close()
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	count := 12
	if frequency == "Hourly" {
		count = 8760
	}
	for index := 0; index < count; index++ {
		stamp := time.Date(2017, time.Month(index+2), 0, 0, 0, 0, 0, time.UTC)
		hour, minutes, intervalType := 24, float64(stamp.Day()*1440), 3
		if frequency == "Hourly" {
			stamp = time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index) * time.Hour)
			hour, minutes, intervalType = stamp.Hour()+1, 60, 1
		}
		if _, err := tx.Exec(`INSERT INTO "Time" VALUES(?,?,?,?,?,2017,?,?,3,NULL,?)`, index+1, int(stamp.Month()), stamp.Day(), hour, 0, minutes, intervalType, stamp.YearDay()); err != nil {
			t.Fatal(err)
		}
		native := value(index) * 3600000
		if unit == "W" {
			native = value(index) * 1000 / (minutes / 60)
		}
		if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,40,?)`, index+1, native); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}, BaseboardContexts: []epathRealSQLBaseboardContext{{ZoneName: "Office", OwnerName: "Baseboard", Service: "heating", Frequency: frequency, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: unit}}, Keys: []string{"Baseboard"}}}}}
	plan := PurposeRunPlan{BasicEnergyDetail: "energy_path", OutputObjects: []PurposeOutputObject{energyPathBaseboardTestRequest(name, "Baseboard", "Monthly", "Office", -1)}}
	if frequency == "Hourly" {
		plan.OutputObjects = append(plan.OutputObjects, energyPathBaseboardTestRequest(name, "Baseboard", "Hourly", "Office", -1))
	}
	return path, model, plan, observed.Sources
}

func epathSQLBaseboardContextUnitFrames() epathSQLFrames {
	return epathSQLFrames{Zones: map[string]epathSQLZone{"office": {Name: "Office", Multiplier: 7}}, SourceIdentities: map[int]epathRealSQLSource{}}
}

func epathSQLBaseboardContextUnitMonthlyCompanion(frames *epathSQLFrames) {
	source := epathRealSQLSource{DictionaryIndex: 42, Name: "Baseboard Electricity Energy", KeyValue: "Baseboard", ReportingFrequency: "Monthly", SourceUnit: "J", Rows: 12, RawSum: epathOracleNumber(43200000), EnergyKWh: epathOracleNumber(12)}
	for month := 1; month <= 12; month++ {
		source.Months = append(source.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: epathOracleNumber(3600000), EnergyKWh: epathOracleNumber(1)})
	}
	frames.DirectHVACSourceIdentities = map[int]epathSQLDirectHVACSourceIdentity{42: {FamilyID: "heating.baseboard.electricity", Service: "heating", Carrier: "electricity", SiteID: "electricHeating", AggregationBasis: "model_total", Owner: epathSQLBaseboardOwner("Office", "Baseboard"), Source: source, Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}}}
}

func epathSQLBaseboardContextUnitBundle(identity epathSQLBaseboardContextIdentity) PurposeResultBundle {
	unit, category, component, effective, _ := epathSQLBaseboardContextClass(identity.Source.Name, identity.Source.ReportingFrequency)
	method := "sum_report_data"
	if unit == "W" {
		method = "integrate_rate_by_time_interval"
	}
	source := EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", identity.Source.DictionaryIndex), SourceType: "sql_report_data", Name: identity.Source.Name, KeyValue: identity.OwnerName, ZoneName: identity.ZoneName, Units: unit, SourceUnit: unit, NormalizedUnit: "kWh", ReportingFrequency: identity.Source.ReportingFrequency, AggregationMethod: method, AggregationBasis: "model_total", ObjectIndex: identity.ObjectIndex, RawValue: identity.ReportedScalar, inspectorDecodedFromJSON: true, inspectorValuePresence: 1}
	// Full LoadProjection normalizes native component observations to model
	// total even when the Hourly source is not an additive series.
	source.EffectiveValue = identity.ReportedScalar
	source.inspectorValuePresence |= 2
	source.EffectiveMultiplier = 1
	source.MultiplierApplication = "already_model_total"
	if effective {
		source.EffectiveValue = identity.ReportedScalar
		source.inspectorValuePresence |= 2
		source.DriverRole = "context"
		source.DriverCategory = category
		source.DriverComponent = component
		source.InspectorSection = "Context"
		source.HeatDirection = "heating"
		source.EffectiveMultiplier = 1
		source.MultiplierApplication = "already_model_total"
	}
	if identity.Source.ReportingFrequency == "Hourly" {
		source.HourlyEnergy = &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source", Values: append([]float64(nil), identity.HourlyValues...)}
	}
	return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: "semantic-idf.energy-explanation/v2", Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}, Sources: []EnergyDataSource{source}, HourlyLabels: append([]string(nil), identity.HourlyLabels...)}}
}

func TestEnergyPathRealSQLBaseboardContextMonthlySignsZerosAndNonAdditivity(t *testing.T) {
	for _, name := range []string{"Baseboard Total Heating Energy", "Baseboard Total Heating Rate", "Baseboard Electricity Rate"} {
		t.Run(name, func(t *testing.T) {
			sign := -1.0
			if name == "Baseboard Electricity Rate" {
				sign = 1
			}
			path, model, plan, observed := epathSQLBaseboardContextUnit(t, name, "Monthly", func(index int) float64 {
				if index == 0 {
					return sign * 1.2504
				}
				if index == 1 {
					return 0
				}
				return .00049
			})
			frames := epathSQLBaseboardContextUnitFrames()
			beforeLoads := frames.Loads
			if err := epathCompileSQLBaseboardContexts(path, energyPathBaseboardHand(t).String(), &plan, observed, model, &frames); err != nil {
				t.Fatal(err)
			}
			identity := frames.BaseboardContextIdentities[40]
			if identity.ReportedScalar != sign*1.25 || identity.Source.EnergyKWh == nil || math.Abs(*identity.Source.EnergyKWh-(sign*1.2504+.0049)) > 1e-12 || !reflect.DeepEqual(frames.Loads, beforeLoads) || len(frames.DirectHVAC) != 0 || len(frames.SourceIdentities) != 0 {
				t.Fatalf("context changed native scalar/rounding or became additive: %+v", identity)
			}
			checks := epathSQLModelChecks{}
			if err := epathSQLModelBaseboardContextSourceChecks(frames, &checks); err != nil {
				t.Fatal(err)
			}
			if len(checks.Rows) != 4 {
				t.Fatalf("missing exact raw/effective Building/Zone checks: %d", len(checks.Rows))
			}
			bundle := epathSQLBaseboardContextUnitBundle(identity)
			for _, check := range checks.Rows {
				if err := epathCheckSQLBaseboardContextSource(bundle, check); err != nil {
					t.Fatal(err)
				}
			}
			for _, tc := range []struct {
				name   string
				mutate func(*PurposeResultBundle)
			}{
				{"wrong source sign", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].RawValue *= -1 }},
				{"multiplier twice", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].EffectiveValue *= 7 }},
				{"NULL scalar", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].inspectorValuePresence &^= 1 }},
				{"wrong frequency opener", func(b *PurposeResultBundle) { index := 42; b.EnergyExplanation.Sources[0].ObjectIndex = &index }},
				{"additive role", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[0].DriverRole = "main_flow" }},
				{"graph input", func(b *PurposeResultBundle) {
					b.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "bad", SourceIDs: []string{"sql-rdd-40"}}}
				}},
				{"derived graph input", func(b *PurposeResultBundle) {
					b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: "derived", InputSourceIDs: []string{"sql-rdd-40"}})
					b.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "bad", SourceIDs: []string{"derived"}}}
				}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					copy := epathSQLBaseboardContextUnitBundle(identity)
					tc.mutate(&copy)
					if err := epathCheckSQLBaseboardContextSource(copy, checks.Rows[0]); err == nil {
						t.Fatal("context proof accepted source mutation")
					}
				})
			}
		})
	}
}

func TestEnergyPathRealSQLBaseboardContextNativeHourlyQuantizationAndElectricCompanion(t *testing.T) {
	for _, name := range []string{"Baseboard Total Heating Energy", "Baseboard Total Heating Rate", "Baseboard Electricity Rate", "Baseboard Electricity Energy"} {
		t.Run(name, func(t *testing.T) {
			path, model, plan, observed := epathSQLBaseboardContextUnit(t, name, "Hourly", func(index int) float64 {
				if index%2 == 0 {
					return .00049
				}
				return .00051
			})
			frames := epathSQLBaseboardContextUnitFrames()
			if name == "Baseboard Electricity Energy" {
				if err := epathCompileSQLBaseboardContexts(path, energyPathBaseboardHand(t).String(), &plan, observed, model, &frames); err == nil {
					t.Fatal("Hourly electricity became a companion without its measured Monthly authority")
				}
				epathSQLBaseboardContextUnitMonthlyCompanion(&frames)
			}
			if err := epathCompileSQLBaseboardContexts(path, energyPathBaseboardHand(t).String(), &plan, observed, model, &frames); err != nil {
				t.Fatal(err)
			}
			identity := frames.BaseboardContextIdentities[40]
			if identity.ReportedScalar != 4.38 || len(identity.HourlyValues) != 8760 || identity.HourlyValues[0] != 0 || identity.HourlyValues[1] != .001 || identity.HourlyLabels[0] != "01-01 01:00" || identity.HourlyLabels[8759] != "12-31 24:00" || identity.ObjectIndex != nil {
				t.Fatal("native 8760-row chart lost rounding/axis/ambiguous opener proof")
			}
			checks := epathSQLModelChecks{}
			if err := epathSQLModelBaseboardContextSourceChecks(frames, &checks); err != nil {
				t.Fatal(err)
			}
			wantChecks := 4
			if name == "Baseboard Electricity Energy" {
				wantChecks = 2
			}
			if len(checks.Rows) != wantChecks {
				t.Fatal("direct Hourly companion changed the raw-only metric roster")
			}
			bundle := epathSQLBaseboardContextUnitBundle(identity)
			for _, check := range checks.Rows {
				if err := epathCheckSQLBaseboardContextSource(bundle, check); err != nil {
					t.Fatal(err)
				}
			}
			if name == "Baseboard Electricity Energy" {
				for _, mutation := range []string{"missing effective", "multiplied twice", "allocated companion", "graph input"} {
					bad := epathSQLBaseboardContextUnitBundle(identity)
					switch mutation {
					case "missing effective":
						bad.EnergyExplanation.Sources[0].inspectorValuePresence &^= 2
					case "multiplied twice":
						bad.EnergyExplanation.Sources[0].EffectiveValue *= 7
					case "allocated companion":
						bad.EnergyExplanation.Sources[0].AllocationApplied = true
					case "graph input":
						bad.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "bad-hourly-load", SourceIDs: []string{bad.EnergyExplanation.Sources[0].ID}}}
					}
					if err := epathCheckSQLBaseboardContextSource(bad, checks.Rows[0]); err == nil {
						t.Fatalf("accepted Hourly direct %s", mutation)
					}
				}
			}
			bundle.EnergyExplanation.Sources[0].HourlyEnergy.Values[0] = .00049
			if err := epathCheckSQLBaseboardContextSource(bundle, checks.Rows[0]); err == nil {
				t.Fatal("chart silently switched from explicit transport grid to native lossless claim")
			}
			bundle = epathSQLBaseboardContextUnitBundle(identity)
			bundle.EnergyExplanation.HourlyLabels[0] = "01-01 00:00"
			if err := epathCheckSQLBaseboardContextSource(bundle, checks.Rows[0]); err == nil {
				t.Fatal("chart shifted native end-of-hour convention")
			}
		})
	}
}

func TestEnergyPathRealSQLBaseboardContextRejectsMissingDuplicateForeignAndWrongPlan(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		mutate      func(*epathRealSQLModel, *PurposeRunPlan)
	}{
		{"missing row", `DELETE FROM ReportData WHERE TimeIndex=1`, nil},
		{"NULL row", `UPDATE ReportData SET Value=NULL WHERE TimeIndex=1`, nil},
		{"duplicate dictionary without rows", `INSERT INTO ReportDataDictionary VALUES(41,'Baseboard','Baseboard Total Heating Energy','J',0,'Monthly','HVAC')`, nil},
		{"wrong Time interval", `UPDATE "Time" SET "Interval"=60 WHERE TimeIndex=1`, nil},
		{"warmup row", `UPDATE "Time" SET WarmupFlag=1 WHERE TimeIndex=1`, nil},
		{"foreign owner declaration", "", func(m *epathRealSQLModel, p *PurposeRunPlan) { m.BaseboardContexts[0].ZoneName = "Other" }},
		{"Monthly electricity must stay direct", "", func(m *epathRealSQLModel, p *PurposeRunPlan) {
			m.BaseboardContexts[0].Source.Alternatives[0].Name = "Baseboard Electricity Energy"
		}},
		{"wrong unit", "", func(m *epathRealSQLModel, p *PurposeRunPlan) {
			m.BaseboardContexts[0].Source.Alternatives[0].Unit = "W"
		}},
		{"absent allowed", "", func(m *epathRealSQLModel, p *PurposeRunPlan) { m.BaseboardContexts[0].Source.AllowAbsent = true }},
		{"wrong purpose owner", "", func(m *epathRealSQLModel, p *PurposeRunPlan) { p.OutputObjects[0].ScopeZoneName = "Other" }},
		{"duplicate declaration", "", func(m *epathRealSQLModel, p *PurposeRunPlan) {
			m.BaseboardContexts = append(m.BaseboardContexts, m.BaseboardContexts[0])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, model, plan, observed := epathSQLBaseboardContextUnit(t, "Baseboard Total Heating Energy", "Monthly", func(int) float64 { return 0 })
			if tc.query != "" {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(tc.query); err != nil {
					t.Fatal(err)
				}
				db.Close()
			}
			if tc.mutate != nil {
				tc.mutate(&model, &plan)
			}
			frames := epathSQLBaseboardContextUnitFrames()
			if err := epathCompileSQLBaseboardContexts(path, energyPathBaseboardHand(t).String(), &plan, observed, model, &frames); err == nil || len(frames.BaseboardContextIdentities) != 0 {
				t.Fatal("invalid context committed partial source proof")
			}
		})
	}
	path, model, plan, observed := epathSQLBaseboardContextUnit(t, "Baseboard Total Heating Energy", "Monthly", func(int) float64 { return 0 })
	frames := epathSQLBaseboardContextUnitFrames()
	if err := epathCompileSQLBaseboardContexts(path, energyPathBaseboardHand(t).String(), &plan, observed, model, &frames); err != nil {
		t.Fatal(err)
	}
	checks := epathSQLModelChecks{}
	if err := epathSQLModelBaseboardContextSourceChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	bundle := epathSQLBaseboardContextUnitBundle(frames.BaseboardContextIdentities[40])
	if err := epathCheckSQLBaseboardContextSource(bundle, checks.Rows[0]); err != nil {
		t.Fatal(err)
	}
	bundle.EnergyExplanation.Sources[0].inspectorValuePresence = 0
	if err := epathCheckSQLBaseboardContextSource(bundle, checks.Rows[0]); err == nil {
		t.Fatal("native zero became missing/NULL")
	}
}

func TestEnergyPathRealSQLBaseboardContextOpenerNeverBorrowsOtherFrequency(t *testing.T) {
	doc := energyPathBaseboardHand(t)
	declaration := epathRealSQLBaseboardContext{ZoneName: "Office", OwnerName: "Baseboard", Service: "heating", Frequency: "Hourly", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Baseboard Total Heating Energy", Unit: "J"}}, Keys: []string{"Baseboard"}}}
	plan := PurposeRunPlan{BasicEnergyDetail: "energy_path", OutputObjects: []PurposeOutputObject{energyPathBaseboardTestRequest("Baseboard Total Heating Energy", "Baseboard", "Monthly", "Office", 42), energyPathBaseboardTestRequest("Baseboard Total Heating Energy", "Baseboard", "Hourly", "Office", -1)}}
	if index, err := epathSQLBaseboardContextOpener(doc, &plan, declaration); err != nil || index != nil {
		t.Fatalf("two untouched original Hourly wildcards should prove unknown, not Monthly42: %v/%v", index, err)
	}
	// A genuine exact original Hourly request outranks wildcard ambiguity.
	energyPathBaseboardDuplicate(&doc, idf.Object{Type: "Output:Variable", Fields: []idf.Field{{Value: "Baseboard"}, {Value: "Baseboard Total Heating Energy"}, {Value: "Hourly"}}})
	index := 71
	plan.OutputObjects[1].ObjectIndex = &index
	if got, err := epathSQLBaseboardContextOpener(doc, &plan, declaration); err != nil || got == nil || *got != 71 {
		t.Fatalf("exact actual-frequency opener lost: %v/%v", got, err)
	}
}

func TestEnergyPathRealSQLBaseboardContextTemporaryOpenerUsesHashBoundExecutedModel(t *testing.T) {
	path, model, plan, observed := epathSQLBaseboardContextUnit(t, "Baseboard Total Heating Energy", "Monthly", func(int) float64 { return .00049 })
	original := energyPathBaseboardHand(t)
	executed := parsePurposePlanFixture(t, "Timestep,4;\n"+original.String())
	monthlyIndex := len(executed.Objects)
	energyPathBaseboardDuplicate(&executed, idf.Object{Type: "Output:Variable", Fields: []idf.Field{{Value: "Baseboard"}, {Value: "Baseboard Total Heating Energy"}, {Value: "Monthly"}}})
	hourlyIndex := len(executed.Objects)
	energyPathBaseboardDuplicate(&executed, idf.Object{Type: "Output:Variable", Fields: []idf.Field{{Value: "Baseboard"}, {Value: "Baseboard Total Heating Energy"}, {Value: "Hourly"}}})
	plan.OutputObjects = append(plan.OutputObjects, energyPathBaseboardTestRequest("Baseboard Total Heating Energy", "Baseboard", "Hourly", "Office", -1))
	if plan.OutputObjects[0].ObjectIndex != nil || plan.OutputObjects[1].ObjectIndex != nil {
		t.Fatal("fixture lost pre-application temporary plan")
	}
	frames := epathSQLBaseboardContextUnitFrames()
	if err := epathCompileSQLBaseboardContexts(path, original.String(), &plan, observed, model, &frames, executed.String()); err != nil {
		t.Fatal(err)
	}
	identity := frames.BaseboardContextIdentities[40]
	if identity.ObjectIndex == nil || *identity.ObjectIndex != monthlyIndex || identity.ReportedScalar != 0 {
		t.Fatal("temporary Monthly output did not bind its exact executed object while preserving observed rounded zero")
	}
	checks := epathSQLModelChecks{}
	if err := epathSQLModelBaseboardContextSourceChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	bundle := epathSQLBaseboardContextUnitBundle(identity)
	if err := epathCheckSQLBaseboardContextSource(bundle, checks.Rows[0]); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []int{-1, monthlyIndex - 1, hourlyIndex} {
		bad := epathSQLBaseboardContextUnitBundle(identity)
		bad.EnergyExplanation.Sources[0].ObjectIndex = nil
		if wrong >= 0 {
			value := wrong
			bad.EnergyExplanation.Sources[0].ObjectIndex = &value
		}
		if err := epathCheckSQLBaseboardContextSource(bad, checks.Rows[0]); err == nil {
			t.Fatalf("accepted missing, unshifted or wrong-frequency opener %d", wrong)
		}
	}
	for _, tc := range []struct {
		name    string
		mutate  func(*idf.Document, *PurposeRunPlan)
		unknown bool
	}{
		{"wrong exact key", func(d *idf.Document, p *PurposeRunPlan) { d.Objects[monthlyIndex].Fields[0].Value = "Other" }, false},
		{"wrong actual frequency", func(d *idf.Document, p *PurposeRunPlan) { d.Objects[monthlyIndex].Fields[2].Value = "Daily" }, false},
		{"plan borrows Hourly index", func(d *idf.Document, p *PurposeRunPlan) {
			value := hourlyIndex
			p.OutputObjects[0].ObjectIndex = &value
		}, false},
		{"duplicate executed exact remains unknown", func(d *idf.Document, p *PurposeRunPlan) { energyPathBaseboardDuplicate(d, d.Objects[monthlyIndex]) }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := parsePurposePlanFixture(t, executed.String())
			captured := plan
			captured.OutputObjects = append([]PurposeOutputObject(nil), plan.OutputObjects...)
			tc.mutate(&run, &captured)
			index, err := epathSQLBaseboardContextOpener(original, &captured, model.BaseboardContexts[0], run)
			if tc.unknown {
				if err != nil || index != nil {
					t.Fatalf("duplicate opener became unique: %v/%v", index, err)
				}
			} else if err == nil {
				t.Fatal("accepted contradicted executed output/plan identity")
			}
		})
	}
	hourly := model.BaseboardContexts[0]
	hourly.Frequency = "Hourly"
	if index, err := epathSQLBaseboardContextOpener(original, &plan, hourly, executed); err != nil || index == nil || *index != hourlyIndex {
		t.Fatalf("executed exact Hourly output failed to outrank preserved wildcard ambiguity: %v/%v", index, err)
	}
	withoutExact := parsePurposePlanFixture(t, executed.String())
	withoutExact.Objects[hourlyIndex].Fields[0].Value = "Other"
	if index, err := epathSQLBaseboardContextOpener(original, &plan, hourly, withoutExact); err != nil || index != nil {
		t.Fatalf("two preserved original wildcards became a unique opener: %v/%v", index, err)
	}
	for i, object := range withoutExact.Objects {
		if object.Type == "Output:Variable" && epathSQLBaseboardField(object, 0) == "*" && epathSQLBaseboardField(object, 1) == "Baseboard Total Heating Energy" {
			withoutExact.Objects[i].Fields[0].Value = "Other"
			break
		}
	}
	if _, err := epathSQLBaseboardContextOpener(original, &plan, hourly, withoutExact); err == nil {
		t.Fatal("lost original wildcard was accepted as a now-unique opener")
	}
}
