package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Literal native fixture, not production output or expected/candidate JSON.
// East predictions are +1 kW in January and -2 kW in February (-600 kWh
// signed annual); West/North are observed zero. Coil heat is 744 kWh in
// January only. Neither is the hand fan budget 100 kWh or Zone weights 1:2:1.
func epathSQLHeatOnlyContextUnitExtend(t *testing.T, o *epathRealOracleEvidence) {
	t.Helper()
	db, err := sql.Open("sqlite", o.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	insert, err := tx.Prepare(`INSERT INTO ReportData(ReportDataIndex,ReportDataDictionaryIndex,TimeIndex,Value) VALUES(?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer insert.Close()
	type literal struct{ name, key, unit string }
	specs := []literal{}
	for _, zone := range []string{"WEST ZONE", "EAST ZONE", "NORTH ZONE"} {
		for _, name := range []string{"Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate", "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate", "Zone System Predicted Sensible Load to Heating Setpoint Heat Transfer Rate", "Zone System Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate"} {
			specs = append(specs, literal{name, zone, "W"})
		}
	}
	specs = append(specs, literal{"Heating Coil Heating Energy", "FURNACE COIL", "J"}, literal{"Heating Coil Heating Rate", "FURNACE COIL", "W"})
	serial := 0
	for _, frequency := range []string{"Monthly", "Hourly"} {
		for _, s := range specs {
			id := 1000 + serial
			serial++
			kind := "Avg"
			if s.unit == "J" {
				kind = "Sum"
			}
			if _, err = tx.Exec(`INSERT INTO ReportDataDictionary(ReportDataDictionaryIndex,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup,Type,TimestepType) VALUES(?,?,?,0,?,?,'System',?,'HVAC System')`, id, s.name, s.key, frequency, s.unit, kind); err != nil {
				t.Fatal(err)
			}
			count := 12
			if frequency == "Hourly" {
				count = 8760
			}
			for n := 0; n < count; n++ {
				month, timeIndex, hours := n+1, 20001+n, 0
				if frequency == "Hourly" {
					date := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour)
					month, timeIndex, hours = int(date.Month()), n+1, 1
				} else {
					hours = time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day() * 24
				}
				watts := 0.0
				if s.key == "EAST ZONE" {
					if month == 1 {
						watts = 1000
					}
					if month == 2 {
						watts = -2000
					}
				}
				if s.key == "FURNACE COIL" && month == 1 {
					watts = 1000
				}
				value := watts
				if s.unit == "J" {
					value = watts * float64(hours) * 3600
				}
				if _, err = insert.Exec(1000000+serial*10000+n, id, timeIndex, value); err != nil {
					t.Fatal(err)
				}
			}
			o.executedText += "\nOutput:Variable," + s.key + "," + s.name + "," + frequency + ";"
			scopeZone := s.key
			if s.key == "FURNACE COIL" {
				scopeZone = ""
			}
			o.outputPlan.OutputObjects = append(o.outputPlan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", VariableName: s.name, KeyValue: s.key, ScopeZoneName: scopeZone, ReportingFrequency: frequency, State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Value: s.key}, {Value: s.name}, {Value: frequency}}})
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(o.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	o.Sources = observed.Sources
}

func epathSQLHeatOnlyContextUnitChecks(t *testing.T) (epathRealOracleEvidence, epathRealSQLModel, epathSQLModelChecks) {
	t.Helper()
	o, m, f := epathSQLHeatOnlyBindingUnit(t)
	epathSQLHeatOnlyContextUnitExtend(t, &o)
	var checks epathSQLModelChecks
	if err := epathSQLBindHeatOnlyChecks(o, m, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelHeatOnlyContextChecks(f, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 104 {
		t.Fatalf("finite 28-source/104-field census changed: %d", len(checks.Rows))
	}
	if err := epathSQLHeatOnlyContextCensus(checks); err != nil {
		t.Fatal(err)
	}
	return o, m, checks
}

func epathSQLHeatOnlyContextHandBundle(t *testing.T, c epathSQLModelCheck) PurposeResultBundle {
	t.Helper()
	p := c.HeatOnlyContext
	s := p.Spec
	component, application, method := "load.delivered.combined", "already_model_total", "integrate_rate_by_time_interval"
	if s.Unit == "J" {
		method = "sum_report_data"
	}
	if s.Zone != "" {
		component = "load.predicted.sensible"
		if strings.HasPrefix(s.Name, "Zone Predicted ") {
			application = "requires_zone_multiplier"
		}
	}
	a := EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", p.DictionaryIndex), SourceType: "sql_report_data", Name: s.Name, KeyValue: strings.ToUpper(s.Key), ZoneName: s.Zone, SourceUnit: s.Unit, Units: s.Unit, NormalizedUnit: "kWh", ReportingFrequency: s.Frequency, IndexGroup: "System", AggregationMethod: method, AggregationBasis: "model_total", RawValue: p.Scalar, EffectiveValue: p.Scalar, AllocatedValue: p.Scalar, EffectiveMultiplier: 1, MultiplierApplication: application, ObjectIndex: p.OutputIndex, DriverRole: "context", DriverCategory: "load." + s.Service, DriverComponent: component, HeatDirection: s.Service, InspectorSection: "Context", observedValuePresence: 3}
	if s.Zone != "" {
		a.ScopeDetails = []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: s.Zone, AggregationBasis: "model_total"}, RawValue: p.Scalar, EffectiveValue: p.Scalar, AllocatedValue: p.Scalar, EffectiveMultiplier: 1, MultiplierApplication: application, AggregationBasis: "model_total", inspectorDecodedFromJSON: true, inspectorValuePresence: 3}}
	}
	b := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}, Sources: []EnergyDataSource{a}}}
	if s.Frequency == "Hourly" {
		b.EnergyExplanation.Sources[0].HourlyEnergy = &EnergySourceHourlyEnergy{Unit: "kWh", Basis: "reported_source", Values: append([]float64(nil), p.Hourly...)}
		for n := 0; n < 8760; n++ {
			d := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Hour)
			b.EnergyExplanation.HourlyLabels = append(b.EnergyExplanation.HourlyLabels, fmt.Sprintf("%02d-%02d %02d:00", d.Month(), d.Day(), d.Hour()+1))
		}
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PurposeResultBundle
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestEnergyPathSQLHeatOnlyContextSignedZeroAndCoilBoundary(t *testing.T) {
	_, _, checks := epathSQLHeatOnlyContextUnitChecks(t)
	for _, c := range checks.Rows {
		p := c.HeatOnlyContext
		want := 0.0
		if strings.EqualFold(p.Spec.Zone, "East Zone") {
			want = -600
		}
		if p.Spec.Zone == "" {
			want = 744
		}
		if p.Scalar != want {
			t.Fatalf("independent signed/coil literal changed: %s=%g", p.Spec.Name, p.Scalar)
		}
		if err := epathCheckSQLHeatOnlyContextSource(epathSQLHeatOnlyContextHandBundle(t, c), c); err != nil {
			t.Fatal(err)
		}
		if c.Item.Scope == "zone" {
			b := epathSQLHeatOnlyContextHandBundle(t, c)
			b.EnergyExplanation.Sources[0].ScopeDetails = nil
			if err := epathCheckSQLHeatOnlyContextSource(b, c); err == nil {
				t.Fatalf("deleted requested Zone cache accepted: %s/%s/%s", c.Item.Zone, p.Spec.Name, p.Spec.Frequency)
			}
		}
		if p.Spec.Zone == "" && p.Spec.Frequency == "Monthly" {
			b := epathSQLHeatOnlyContextHandBundle(t, c)
			a := &b.EnergyExplanation.Sources[0]
			for _, zone := range []string{"West Zone", "East Zone", "North Zone"} {
				a.ScopeDetails = append(a.ScopeDetails, EnergyDataSourceScopeDetail{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone}, RawValue: 744, EffectiveValue: 744, AllocatedValue: 744, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", AggregationBasis: "model_total", inspectorDecodedFromJSON: true, inspectorValuePresence: 3})
			}
			if err := epathCheckSQLHeatOnlyContextSource(b, c); err != nil {
				t.Fatalf("unchanged source-context cache was mistaken for allocation: %v", err)
			}
		}
	}
	for _, c := range checks.Rows {
		if c.Item.Scope != "building" || c.Item.Target.Field != "rawValue" {
			continue
		}
		for _, mutation := range []string{"absolute", "wrong-zone", "missing-zero", "main-flow", "allocated", "cache-allocation", "fake-cache-zone", "wrong-units", "formula", "wrong-opener", "hourly-flatten"} {
			t.Run(fmt.Sprintf("%d/%s", c.HeatOnlyContext.DictionaryIndex, mutation), func(t *testing.T) {
				b := epathSQLHeatOnlyContextHandBundle(t, c)
				a := &b.EnergyExplanation.Sources[0]
				switch mutation {
				case "absolute":
					if a.RawValue >= 0 {
						return
					}
					a.RawValue = math.Abs(a.RawValue)
				case "wrong-zone":
					a.ZoneName = "Foreign"
				case "missing-zero":
					if a.RawValue != 0 {
						return
					}
					a.inspectorValuePresence = 0
				case "main-flow":
					a.DriverRole = "main_flow"
				case "allocated":
					a.AllocationApplied = true
				case "cache-allocation":
					a.ScopeDetails = []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "East Zone"}, AllocationApplied: true}}
				case "fake-cache-zone":
					a.ScopeDetails = []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Foreign"}}}
				case "wrong-units":
					a.SourceUnit = "J"
					if c.HeatOnlyContext.Spec.Unit == "J" {
						a.SourceUnit = "W"
					}
				case "formula":
					a.Formula = "coil / gas"
					a.InputSourceIDs = []string{"gas"}
				case "wrong-opener":
					n := 106
					a.ObjectIndex = &n
				case "hourly-flatten":
					if a.HourlyEnergy == nil {
						return
					}
					a.HourlyEnergy.Values[0]++
				}
				if err := epathCheckSQLHeatOnlyContextSource(b, c); err == nil {
					t.Fatal("bad native context accepted")
				}
			})
		}
	}
}

func TestEnergyPathSQLHeatOnlyContextNativeMissingDuplicateAndSignedMutations(t *testing.T) {
	baseline, baselineModel, _ := epathSQLHeatOnlyBindingUnit(t)
	epathSQLHeatOnlyContextUnitExtend(t, &baseline)
	data, err := os.ReadFile(baseline.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=1018 AND TimeIndex=745`,
		`INSERT INTO ReportData(ReportDataIndex,ReportDataDictionaryIndex,TimeIndex,Value) VALUES(99999999,1018,99999999,0)`,
		`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=1018 AND TimeIndex=745`,
		`UPDATE ReportData SET TimeIndex=746 WHERE ReportDataDictionaryIndex=1018 AND TimeIndex=745`,
		`UPDATE ReportData SET Value=ABS(Value) WHERE ReportDataDictionaryIndex=1018 AND TimeIndex=745`,
		`UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=1026 AND TimeIndex=745`,
		`UPDATE ReportDataDictionary SET KeyValue='OTHER COIL' WHERE ReportDataDictionaryIndex=1012`,
		`UPDATE ReportDataDictionary SET Type='Sum' WHERE ReportDataDictionaryIndex=1004`,
		`INSERT INTO ReportDataDictionary SELECT 9999,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup,Type,TimestepType,ScheduleName FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=1004`,
	} {
		o := epathSQLHeatOnlyUnitSQLCopy(t, baseline, data)
		m := epathSQLHeatOnlyClone(t, baselineModel)
		b, err := epathSQLCompileHeatOnlyBinding(o, m)
		if err != nil {
			t.Fatal(err)
		}
		epathOracleEditSQL(t, o.sqlPath, query)
		if _, err = epathSQLReadHeatOnlyContexts(b); err == nil {
			t.Fatalf("invalid native context accepted: %s", query)
		}
	}
}

func TestEnergyPathSQLHeatOnlyContextCensusAndCompilerDenyAmountPromotion(t *testing.T) {
	_, _, checks := epathSQLHeatOnlyContextUnitChecks(t)
	for _, mutation := range []string{"delete-all", "delete-one", "quantity", "native-proof", "scope"} {
		bad := epathSQLHeatOnlyClone(t, checks)
		switch mutation {
		case "delete-all":
			bad.Rows = nil
		case "delete-one":
			bad.Rows = bad.Rows[1:]
		case "quantity":
			bad.Rows[0].Quantity.Value++
		case "native-proof":
			bad.Rows[0].HeatOnlyContext.DictionaryIndex++
		case "scope":
			bad.Rows[0].Item.Zone = "Foreign"
		}
		if err := epathSQLHeatOnlyContextCensus(bad); err == nil {
			t.Fatal("missing/changed context proof accepted: " + mutation)
		}
	}
	id := checks.Rows[0].HeatOnlyContext.DictionaryIndex
	for _, f := range []epathSQLFrames{{SourceRaw: map[int][]epathSQLQuantity{id: {}}}, {LoadSourceIDs: map[string][]int{"heat": {id}}}, {SiteSources: map[string][]int{"gas": {id}}}, {Cells: map[string]*epathSQLCell{"pressure": {SourceIDs: []int{id}}}}} {
		copy := epathSQLModelChecks{HeatOnly: checks.HeatOnly}
		if err := epathSQLModelHeatOnlyContextChecks(f, &copy); err == nil {
			t.Fatal("source-only control/coil promoted to model amount")
		}
	}
}

func TestEnergyPathSQLHeatOnlyContextZeroAndDerivedGraphLeak(t *testing.T) {
	_, _, checks := epathSQLHeatOnlyContextUnitChecks(t)
	c := checks.Rows[0]
	id := fmt.Sprintf("sql-rdd-%d", c.HeatOnlyContext.DictionaryIndex)
	for _, kind := range []string{"zero-node", "zero-link", "renamed-derived"} {
		b := epathSQLHeatOnlyContextHandBundle(t, c)
		use := id
		if kind == "renamed-derived" {
			use = "renamed"
			b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: use, SourceType: "derived_formula", InputSourceIDs: []string{id}})
		}
		if kind == "zero-link" {
			b.EnergyExplanation.Links = []EnergyPathLink{{FromID: "x", ToID: "y", SourceIDs: []string{use}}}
		} else {
			b.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "invented", Level: "load", SourceIDs: []string{use}}}
		}
		if err := epathSQLCheckHeatOnlyContextGraph(b, checks); err == nil {
			t.Fatal("zero/derived context acquired graph authority")
		}
	}
}
