package simulation

import (
	"fmt"
	"math"
	"reflect"
	"testing"
)

func epathSQLLoadDetailUnitFixture(t *testing.T) (string, epathRealSQLModel) {
	t.Helper()
	path, model := epathSQLModelUnitFixture(t)
	// The independent Office Zone multiplier is six. Equipment observations
	// are already model-total and must remain 7/11, not 42/66 or doubled aliases.
	for offset, service := range []string{"Cooling", "Heating"} {
		for measure, unit := range map[string]string{"Energy": "J", "Rate": "W"} {
			id := 40 + offset*2
			if measure == "Rate" {
				id++
			}
			name := "Zone Ideal Loads Supply Air Latent " + service + " " + measure
			value := 7 + offset*4
			expression := fmt.Sprintf("%d*3600000.0", value)
			if unit == "W" {
				expression = fmt.Sprintf("%d*60000.0/t.Interval", value)
			}
			epathOracleEditSQL(t, path, fmt.Sprintf(`INSERT INTO ReportDataDictionary VALUES(%d,'%s','Office Ideal',0,'Monthly','%s','System');
INSERT INTO ReportData SELECT 20000+%d*20+t.Month,%d,t.TimeIndex,CASE WHEN t.Month=1 THEN %s ELSE 0 END FROM Time t WHERE t.IntervalType=3;`, id, name, unit, id, id, expression))
			serviceKey := "cooling"
			if offset == 1 {
				serviceKey = "heating"
			}
			model.NonAdditiveLoadDetails = append(model.NonAdditiveLoadDetails, epathRealSQLNonAdditiveLoadDetail{ZoneName: "Office", Service: serviceKey, OwnerName: "Office Ideal", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: unit}}, Keys: []string{"Office Ideal"}}})
		}
	}
	return path, model
}

func epathSQLLoadDetailUnitSources() []EnergyDataSource {
	sources := []EnergyDataSource{}
	for offset, service := range []string{"Cooling", "Heating"} {
		for quantity, unit := range map[string]string{"Energy": "J", "Rate": "W"} {
			id, value := 40+offset*2, float64(7+offset*4)
			method := "sum_report_data"
			if unit == "W" {
				id++
				method = "integrate_rate_by_time_interval"
			}
			serviceKey, component := "cooling", "load.dehumidification"
			if offset == 1 {
				serviceKey, component = "heating", "load.humidification"
			}
			sources = append(sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), Name: "Zone Ideal Loads Supply Air Latent " + service + " " + quantity, KeyValue: "Office Ideal", SourceType: "sql_report_data", Units: unit, SourceUnit: unit, NormalizedUnit: "kWh", ReportingFrequency: "Monthly", ZoneName: "Office", RawValue: value, EffectiveValue: value, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", AllocationFactor: 1, AggregationBasis: "model_total", AggregationMethod: method, DriverRole: "context", DriverCategory: "load." + serviceKey, DriverComponent: component, HeatDirection: serviceKey, InspectorSection: "Breakdown", inspectorDecodedFromJSON: true, inspectorValuePresence: 3, ScopeDetails: []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, RawValue: value, EffectiveValue: value, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", inspectorDecodedFromJSON: true, inspectorValuePresence: 3}}})
		}
	}
	return sources
}

func epathSQLLoadDetailUnitBundle(sources []EnergyDataSource) PurposeResultBundle {
	return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Sources: sources, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}}}}}
}

func epathSQLLoadDetailUnitChecks(t *testing.T, frames epathSQLFrames, observed []epathRealSQLSource, model epathRealSQLModel) epathSQLModelChecks {
	t.Helper()
	all := epathSQLModelChecks{}
	if err := epathSQLModelSourceChecks(frames, observed, model, &all); err != nil {
		t.Fatal(err)
	}
	checks := epathSQLModelChecks{}
	for _, check := range all.Rows {
		if check.LoadDetail != nil {
			checks.Rows = append(checks.Rows, check)
		}
	}
	if len(checks.Rows) != 16 {
		t.Fatalf("detail original source obligations = %d, want four sources x raw/effective x Building/Zone", len(checks.Rows))
	}
	return checks
}

func TestEnergyPathRealSQLNonAdditiveDetailIndependentFrames(t *testing.T) {
	path, model := epathSQLLoadDetailUnitFixture(t)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	without := model
	without.NonAdditiveLoadDetails = nil
	baseline, err := epathCompileSQLModelFrames(path, observed.Sources, without)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(frames.Loads, baseline.Loads) || !reflect.DeepEqual(frames.Cells, baseline.Cells) || !reflect.DeepEqual(frames.LoadSourceIDs, baseline.LoadSourceIDs) {
		t.Fatal("inspector detail changed primary load quantities, pressure, allocation or selected source authority")
	}
	if len(frames.LoadDetailIdentities) != 4 || len(frames.LoadDetailSourceIDs) != 24 {
		t.Fatal("missing exact owner/service/month detail coverage")
	}
	for month := 1; month <= 12; month++ {
		if !reflect.DeepEqual(frames.LoadDetailSourceIDs[epathSQLKey("Office", "cooling", month)], []int{40, 41}) || !reflect.DeepEqual(frames.LoadDetailSourceIDs[epathSQLKey("Office", "heating", month)], []int{42, 43}) {
			t.Fatal("Energy and Rate must remain separate original observations")
		}
		for id := 40; id <= 43; id++ {
			want := float64(0)
			if month == 1 {
				want = 7
				if id >= 42 {
					want = 11
				}
			}
			if math.Abs(frames.SourceRaw[id][month-1].Value-want) > 1e-12 || math.Abs(frames.SourceEffective[id][month-1].Value-want) > 1e-12 || want == 0 && (frames.SourceRaw[id][month-1].Value != 0 || frames.SourceEffective[id][month-1].Value != 0) {
				t.Fatalf("detail %d/M%d was doubled, Zone-multiplied, or a real zero was lost: raw=%+v effective=%+v want=%g", id, month, frames.SourceRaw[id][month-1], frames.SourceEffective[id][month-1], want)
			}
		}
	}
	checks := epathSQLLoadDetailUnitChecks(t, frames, observed.Sources, model)
	bundle := epathSQLLoadDetailUnitBundle(epathSQLLoadDetailUnitSources())
	var report epathRealOracleEvidence
	if failures := epathEvaluateSQLModelChecks(&report, bundle, checks); len(failures) > 0 {
		t.Fatalf("valid independently observed detail sources rejected: %+v", failures)
	}
	// Selected sensible annual quantity is still 12*3*6=216, not 216+7+7.
	loads := epathSQLModelChecks{}
	if err := epathSQLModelLoadDriverChecks(frames, model, &loads); err != nil {
		t.Fatal(err)
	}
	var loadCheck epathSQLModelCheck
	for _, check := range loads.Rows {
		if check.Item.Scope == "building" && check.Item.Period == "annual" && check.Item.Target.Level == "load" && check.Item.Target.Field == "value" && check.Item.Target.Service == "cooling" {
			loadCheck = check
		}
	}
	bundle.EnergyExplanation.Nodes = []EnergyExplanationNode{{ID: "load.cooling.building", Level: "load", ServiceKind: "cooling", Value: 216, Unit: "kWh", ScaleDomain: "thermal", Basis: "reported_variable", Period: "annual"}}
	for _, added := range []float64{0, 7, 14} {
		bundle.EnergyExplanation.Nodes[0].Value = 216 + added
		actual, err := epathReadOracleCandidate(bundle, loadCheck.Item, loadCheck.Want)
		if err == nil {
			err = epathCheckSQLModelPresentation(bundle, loadCheck, actual)
		}
		if (added == 0) != (err == nil) {
			t.Fatalf("non-additive quantity mutation +%g: %v", added, err)
		}
	}
}

func TestEnergyPathRealSQLNonAdditiveDetailRejectDeclarationAndSQL(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*epathRealSQLModel)
		sql    string
	}{
		{name: "wrong owner", mutate: func(m *epathRealSQLModel) { m.NonAdditiveLoadDetails[0].OwnerName = "Other Equipment" }},
		{name: "unknown Zone", mutate: func(m *epathRealSQLModel) { m.NonAdditiveLoadDetails[0].ZoneName = "Plenum" }},
		{name: "wrong service", mutate: func(m *epathRealSQLModel) { m.NonAdditiveLoadDetails[0].Service = "ventilation" }},
		{name: "opposite service", mutate: func(m *epathRealSQLModel) { m.NonAdditiveLoadDetails[0].Service = "heating" }},
		{name: "wrong unit", mutate: func(m *epathRealSQLModel) { m.NonAdditiveLoadDetails[0].Source.Alternatives[0].Unit = "kWh" }},
		{name: "optional absence", mutate: func(m *epathRealSQLModel) { m.NonAdditiveLoadDetails[0].Source.AllowAbsent = true }},
		{name: "wildcard", mutate: func(m *epathRealSQLModel) { m.NonAdditiveLoadDetails[0].Source.Keys = []string{"*"} }},
		{name: "Energy Rate alternatives", mutate: func(m *epathRealSQLModel) {
			m.NonAdditiveLoadDetails[0].Source.Alternatives = append(m.NonAdditiveLoadDetails[0].Source.Alternatives, m.NonAdditiveLoadDetails[1].Source.Alternatives[0])
		}},
		{name: "duplicate declaration", mutate: func(m *epathRealSQLModel) {
			m.NonAdditiveLoadDetails = append(m.NonAdditiveLoadDetails, m.NonAdditiveLoadDetails[0])
		}},
		{name: "wrong frequency", sql: `UPDATE ReportDataDictionary SET ReportingFrequency='Annual' WHERE ReportDataDictionaryIndex IN(40,41)`},
		{name: "missing month", sql: `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=40 AND TimeIndex=2`},
		{name: "NULL month", sql: `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=40 AND TimeIndex=2`},
		{name: "duplicate month", sql: `INSERT INTO ReportData VALUES(99999,40,2,0)`},
		{name: "unknown rate interval", sql: `UPDATE Time SET Interval=NULL WHERE TimeIndex=2`},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, model := epathSQLLoadDetailUnitFixture(t)
			if test.mutate != nil {
				test.mutate(&model)
			}
			if test.sql != "" {
				epathOracleEditSQL(t, path, test.sql)
			}
			observed, err := epathReadRealSQLOracle(path)
			if err != nil {
				return
			} // Invalid SQL frequency/interval is rejected before frame compilation.
			if _, err = epathCompileSQLModelFrames(path, observed.Sources, model); err == nil {
				t.Fatal("invalid non-additive detail was accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLNonAdditiveDetailRejectCandidateMetadataAndQuantity(t *testing.T) {
	path, model := epathSQLLoadDetailUnitFixture(t)
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	checks := epathSQLLoadDetailUnitChecks(t, frames, observed.Sources, model)
	for _, test := range []struct {
		name   string
		mutate func(*EnergyDataSource)
	}{
		{"main_flow disguise", func(s *EnergyDataSource) { s.DriverRole = "main_flow" }},
		{"wrong owner", func(s *EnergyDataSource) { s.KeyValue = "Other Ideal" }},
		{"wrong Zone", func(s *EnergyDataSource) { s.ZoneName = "Plenum" }},
		{"wrong service", func(s *EnergyDataSource) { s.DriverCategory = "load.heating"; s.HeatDirection = "heating" }},
		{"numeric latent disguise", func(s *EnergyDataSource) { s.DriverComponent = "load.delivered.latent" }},
		{"wrong section", func(s *EnergyDataSource) { s.InspectorSection = "Main flow" }},
		{"wrong unit", func(s *EnergyDataSource) { s.SourceUnit = "W" }},
		{"wrong frequency", func(s *EnergyDataSource) { s.ReportingFrequency = "Annual" }},
		{"fake derived input", func(s *EnergyDataSource) { s.InputSourceIDs = []string{"sql-rdd-10"} }},
		{"Zone multiplier reapplied", func(s *EnergyDataSource) { s.EffectiveMultiplier = 6; s.EffectiveValue *= 6 }},
		{"Energy Rate double count", func(s *EnergyDataSource) { s.RawValue *= 2; s.EffectiveValue *= 2 }},
		{"missing numeric wire", func(s *EnergyDataSource) { s.inspectorValuePresence = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			sources := epathSQLLoadDetailUnitSources()
			for i := range sources {
				if sources[i].ID == "sql-rdd-40" {
					test.mutate(&sources[i])
				}
			}
			bundle := epathSQLLoadDetailUnitBundle(sources)
			var report epathRealOracleEvidence
			if failures := epathEvaluateSQLModelChecks(&report, bundle, checks); len(failures) == 0 {
				t.Fatal("invalid candidate detail passed quantity/metadata checks")
			}
		})
	}
}
