package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func epathSQLDirectHVACSourceUnitFixture(t *testing.T) (string, epathRealSQLModel, *PurposeRunPlan) {
	t.Helper()
	path, model := epathSQLModelUnitFixture(t)
	plan := &PurposeRunPlan{}
	items := []struct {
		id, name, key, component, service, carrier string
		value                                      float64
	}{
		{"cooling.coil.electricity", "Cooling Coil Electricity Energy", "Unrelated DX name", "Coil:Cooling:DX:SingleSpeed", "cooling", "electricity", 2},
		{"cooling.coil.crankcase_electricity", "Cooling Coil Crankcase Heater Electricity Energy", "Unrelated DX name", "Coil:Cooling:DX:SingleSpeed", "cooling", "electricity", .25},
		{"heating.coil.natural_gas", "Heating Coil NaturalGas Energy", "Unrelated fuel name", "Coil:Heating:Fuel", "heating", "natural_gas", 3},
		{"heating.coil.ancillary_natural_gas", "Heating Coil Ancillary NaturalGas Energy", "Unrelated fuel name", "Coil:Heating:Fuel", "heating", "natural_gas", .125},
		{"heating.coil.electricity", "Heating Coil Electricity Energy", "Unrelated fuel name", "Coil:Heating:Fuel", "heating", "electricity", 0},
	}
	for offset, item := range items {
		id := 40 + offset
		epathOracleEditSQL(t, path, fmt.Sprintf(`INSERT INTO ReportDataDictionary VALUES(%d,'%s','%s',0,'Monthly','J','System');
INSERT INTO ReportData SELECT 30000+%d*20+t.Month,%d,t.TimeIndex,CASE WHEN t.Month=1 THEN %.17g*3600000 ELSE 0 END FROM Time t WHERE t.IntervalType=3;`, id, item.name, item.key, id, id, item.value))
		model.DirectHVACComponents = append(model.DirectHVACComponents, epathRealSQLDirectHVACComponent{ID: item.id, Service: item.service, Carrier: item.carrier, SiteID: item.service + "." + item.carrier, Frequency: "Monthly", AggregationBasis: "model_total", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: item.name, Unit: "J"}}, Keys: []string{item.key}}, Owners: []epathRealSQLDirectHVACOwner{{KeyValue: item.key, ZoneName: "Office", EquipmentType: "ZoneHVAC:PackagedTerminalAirConditioner", EquipmentName: "Native package", ComponentType: item.component}}})
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", VariableName: item.name, KeyValue: item.key, ReportingFrequency: "Monthly", ScopeZoneName: "Office", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	}
	for offset, site := range []struct {
		id, name, service, carrier string
		value                      float64
	}{{"cooling.electricity", "Cooling:Electricity", "cooling", "electricity", 2.25}, {"heating.natural_gas", "Heating:NaturalGas", "heating", "natural_gas", 3.125}, {"heating.electricity", "Heating:Electricity", "heating", "electricity", 0}} {
		id := 50 + offset
		if offset == 0 {
			// The shared raw-reader fixture already owns dictionary 1 with
			// this exact Monthly meter. Reuse its twelve weather rows; keep
			// the separate RunPeriod and warmup/design-day records intact.
			epathOracleEditSQL(t, path, fmt.Sprintf(`UPDATE ReportData SET Value=CASE WHEN TimeIndex=1 THEN %.17g*3600000 ELSE 0 END WHERE ReportDataDictionaryIndex=1 AND TimeIndex BETWEEN 1 AND 12;`, site.value))
		} else {
			epathOracleEditSQL(t, path, fmt.Sprintf(`INSERT INTO ReportDataDictionary VALUES(%d,'%s','',1,'Monthly','J','Facility');
INSERT INTO ReportData SELECT 40000+%d*20+t.Month,%d,t.TimeIndex,CASE WHEN t.Month=1 THEN %.17g*3600000 ELSE 0 END FROM Time t WHERE t.IntervalType=3;`, id, site.name, id, id, site.value))
		}
		model.Site = append(model.Site, epathRealSQLSite{ID: site.id, EndUse: site.service, Carrier: site.carrier, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: site.name, Unit: "J"}}, Keys: []string{"*"}, IsMeter: true}})
	}
	return path, model, plan
}

func epathSQLDirectHVACSourceUnitFrames(t *testing.T, path string, model epathRealSQLModel) (epathSQLFrames, epathRealOracleEvidence) {
	t.Helper()
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	return frames, observed
}

func TestEnergyPathRealSQLDirectHVACSourceFrames(t *testing.T) {
	path, model, plan := epathSQLDirectHVACSourceUnitFixture(t)
	before := epathRealFileHash(t, path)
	if err := epathSQLValidateDirectHVACRequests(plan, model); err != nil {
		t.Fatal(err)
	}
	base := model
	base.DirectHVACComponents = nil
	frames, observed := epathSQLDirectHVACSourceUnitFrames(t, path, base)
	prior, err := json.Marshal([]any{frames.Loads, frames.Cells, frames.Site, frames.SourceRaw, frames.SourceEffective, frames.LoadSourceIDs})
	if err != nil {
		t.Fatal(err)
	}
	if err := epathCompileSQLDirectHVACSources(path, observed.Sources, model, &frames); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal([]any{frames.Loads, frames.Cells, frames.Site, frames.SourceRaw, frames.SourceEffective, frames.LoadSourceIDs})
	if string(prior) != string(after) {
		t.Fatal("direct observations changed thermal/Building authorities or generic source quantities")
	}
	if len(frames.DirectHVAC) != 36 || len(frames.DirectHVACSourceIdentities) != 5 {
		t.Fatalf("incomplete source frame: %d/%d", len(frames.DirectHVAC), len(frames.DirectHVACSourceIdentities))
	}
	for month := 1; month <= 12; month++ {
		for _, test := range []struct {
			service, carrier string
			value            float64
			ids              []int
		}{{"cooling", "electricity", 2.25, []int{40, 41}}, {"heating", "natural_gas", 3.125, []int{42, 43}}, {"heating", "electricity", 0, []int{44}}} {
			want := test.value
			if month != 1 {
				want = 0
			}
			got := frames.DirectHVAC[epathSQLDirectHVACKey("Office", test.service, test.carrier, month)]
			// The two original J-to-kWh conversions and their addition can
			// differ from the hand-written decimal by a few binary ULPs.
			// This is not the separate three-decimal presentation allowance.
			ulp := math.Nextafter(want, math.Inf(1)) - want
			quantityMatches := math.Abs(got.Quantity.Value-want) <= 4*ulp
			if want == 0 {
				quantityMatches = got.Quantity.Value == 0
			}
			if !got.Present || !quantityMatches || !reflect.DeepEqual(got.SourceIDs, test.ids) {
				t.Fatalf("wrong original cohort M%d: %+v want%g", month, got, want)
			}
			if want == 0 && (got.Quantity.Error != 0 || !got.Quantity.includesZero()) {
				t.Fatal("observed zero acquired invented uncertainty")
			}
		}
	}
	if frames.Zones["office"].Multiplier != 6 {
		t.Fatal("fixture needs a nonunit Zone multiplier")
	}
	if epathRealFileHash(t, path) != before {
		t.Fatal("read-only source compiler changed SQL")
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectHVACSourceChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 20 {
		t.Fatalf("source proof rows=%d want 5x2x2", len(checks.Rows))
	}
}

func TestEnergyPathRealSQLDirectHVACSourceRejectsMissingAndSpoofed(t *testing.T) {
	for _, test := range []struct {
		name, sql string
		mutate    func(*epathRealSQLModel)
	}{
		{"missing Monthly row", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=40 AND TimeIndex=1`, nil},
		{"null Monthly row", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=40 AND TimeIndex=1`, nil},
		{"duplicate Monthly row", `INSERT INTO ReportData SELECT 99999,40,TimeIndex,Value FROM ReportData WHERE ReportDataDictionaryIndex=40 AND TimeIndex=1`, nil},
		{"negative Monthly row", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=40 AND TimeIndex=1`, nil},
		{"unobserved duplicate", `INSERT INTO ReportDataDictionary SELECT 80,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=40`, nil},
		{"case duplicate", `INSERT INTO ReportDataDictionary SELECT 80,lower(Name),lower(KeyValue),IsMeter,ReportingFrequency,Units,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=40`, nil},
		{"undeclared owner", `INSERT INTO ReportDataDictionary SELECT 80,Name,'Central unknown coil',IsMeter,ReportingFrequency,Units,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=40`, nil},
		{"missing constituent", "", func(m *epathRealSQLModel) { m.DirectHVACComponents = m.DirectHVACComponents[:4] }},
		{"wrong carrier", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Carrier = "natural_gas" }},
		{"wrong source unit", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Source.Alternatives[0].Unit = "W" }},
		{"package electric masquerade", "", func(m *epathRealSQLModel) {
			m.DirectHVACComponents[0].Source.Alternatives[0].Name = "Zone Packaged Terminal Air Conditioner Electricity Energy"
		}},
		{"unknown Zone", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Owners[0].ZoneName = "PLENUM" }},
		{"different package owner", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Owners[0].EquipmentName = "Other package" }},
		{"wrong component type", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Owners[0].ComponentType = "Coil:Heating:Fuel" }},
		{"double multiplier declaration", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].AggregationBasis = "per_zone" }},
		{"unknown treated absent", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Source.AllowAbsent = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, model, _ := epathSQLDirectHVACSourceUnitFixture(t)
			if test.sql != "" {
				epathOracleEditSQL(t, path, test.sql)
			}
			if test.mutate != nil {
				test.mutate(&model)
			}
			observed, err := epathReadRealSQLOracle(path)
			if err != nil {
				return
			} // The independent raw reader may reject invalid SQL first.
			base := model
			base.DirectHVACComponents = nil
			frames, err := epathCompileSQLModelFrames(path, observed.Sources, base)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(frames)
			if err := epathCompileSQLDirectHVACSources(path, observed.Sources, model, &frames); err == nil {
				t.Fatal("invalid direct ownership/observation accepted")
			}
			after, _ := json.Marshal(frames)
			if string(before) != string(after) {
				t.Fatal("failed direct compilation published partial frame")
			}
		})
	}
}

func epathSQLDirectHVACSourceUnitCandidate(identity epathSQLDirectHVACSourceIdentity) EnergyDataSource {
	q, _ := epathSQLDirectHVACAnnualQuantity(identity)
	return EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", identity.Source.DictionaryIndex), SourceType: "sql_report_data", Name: identity.Source.Name, KeyValue: identity.Source.KeyValue, Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", AggregationMethod: "sum_report_data", ZoneName: identity.Owner.ZoneName, AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", RawValue: q.Value, EffectiveValue: q.Value, inspectorDecodedFromJSON: true, inspectorValuePresence: 3, ScopeDetails: []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: identity.Owner.ZoneName}, RawValue: q.Value, EffectiveValue: q.Value, AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", inspectorDecodedFromJSON: true, inspectorValuePresence: 3, inspectorScopedValuePresence: true}}}
}

func TestEnergyPathRealSQLDirectHVACSourceProofAndRequests(t *testing.T) {
	path, model, plan := epathSQLDirectHVACSourceUnitFixture(t)
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectHVACSourceChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}}}}}
	for id := 40; id <= 44; id++ {
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, epathSQLDirectHVACSourceUnitCandidate(frames.DirectHVACSourceIdentities[id]))
	}
	for _, check := range checks.Rows {
		if err := epathCheckSQLDirectHVACSource(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Want.Key, err)
		}
	}
	identity := frames.DirectHVACSourceIdentities[40]
	for _, test := range []struct {
		name   string
		mutate func(*EnergyDataSource)
	}{
		{"wrong Zone", func(s *EnergyDataSource) { s.ZoneName = "Other" }},
		{"wrong key", func(s *EnergyDataSource) { s.KeyValue = "Lighting" }},
		{"wrong carrier family", func(s *EnergyDataSource) { s.Name = "Heating Coil NaturalGas Energy" }},
		{"wrong frequency", func(s *EnergyDataSource) { s.ReportingFrequency = "Hourly" }},
		{"zone remultiplied", func(s *EnergyDataSource) { s.EffectiveMultiplier = 6 }},
		{"wrong aggregation", func(s *EnergyDataSource) { s.AggregationBasis = "per_zone" }},
		{"wrapper masquerade", func(s *EnergyDataSource) { s.InputSourceIDs = []string{"other"} }},
		{"wrong ID", func(s *EnergyDataSource) { s.ID = "derived-40" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := epathSQLDirectHVACSourceUnitCandidate(identity)
			test.mutate(&s)
			if epathSQLMatchDirectHVACSource(s, identity) == nil {
				t.Fatal("spoofed source metadata accepted")
			}
		})
	}
	original, err := epathSQLOriginalDirectHVAC(identity)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := epathSQLOriginalKey(original)
	source := epathSQLDirectHVACSourceUnitCandidate(identity)
	allowed := map[string]epathSQLOriginalSource{key: original}
	if _, err := epathSQLOriginalSourceLeaves([]string{key}, map[string]EnergyDataSource{key: source}, allowed, "M1"); err != nil {
		t.Fatal(err)
	}
	source.InputSourceIDs = []string{"other"}
	if _, err := epathSQLOriginalSourceLeaves([]string{key}, map[string]EnergyDataSource{key: source}, allowed, "M1"); err == nil {
		t.Fatal("direct source escaped exact owner validation through wrapper recursion")
	}
	for _, mutate := range []func(*PurposeRunPlan){
		func(p *PurposeRunPlan) { p.OutputObjects = p.OutputObjects[1:] },
		func(p *PurposeRunPlan) { p.OutputObjects[0].ScopeZoneName = "Other" },
		func(p *PurposeRunPlan) { p.OutputObjects[0].KeyValue = "Office" },
		func(p *PurposeRunPlan) { p.OutputObjects[0].ReportingFrequency = "Hourly" },
		func(p *PurposeRunPlan) { p.OutputObjects = append(p.OutputObjects, p.OutputObjects[0]) },
	} {
		copyPlan := *plan
		copyPlan.OutputObjects = append([]PurposeOutputObject(nil), plan.OutputObjects...)
		mutate(&copyPlan)
		if epathSQLValidateDirectHVACRequests(&copyPlan, model) == nil {
			t.Fatal("missing/spoofed/duplicate executed request accepted")
		}
	}
	zero := frames.DirectHVACSourceIdentities[44]
	zero.Source.Months = append([]epathRealSQLMonth(nil), zero.Source.Months...)
	zero.Source.Months[0].EnergyKWh = epathOracleNumber(math.SmallestNonzeroFloat64)
	if epathSQLValidateDirectHVACSourceIdentity(zero) == nil {
		t.Fatal("exact source zero became nonzero")
	}
}

func TestEnergyPathRealSQLDirectHVACSourceExactOwnedZeroWithoutScopeDetails(t *testing.T) {
	path, model, _ := epathSQLDirectHVACSourceUnitFixture(t)
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectHVACSourceChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	var raw, effective, positive epathSQLModelCheck
	for _, check := range checks.Rows {
		if check.Item.Scope != "zone" {
			continue
		}
		if check.DirectHVACSource.Source.DictionaryIndex == 44 {
			if check.Item.Target.Field == "rawValue" {
				raw = check
			} else {
				effective = check
			}
		} else if check.DirectHVACSource.Source.DictionaryIndex == 40 && check.Item.Target.Field == "rawValue" {
			positive = check
		}
	}
	if raw.DirectHVACSource == nil || effective.DirectHVACSource == nil || positive.DirectHVACSource == nil {
		t.Fatal("fixture lost its zero/positive source obligations")
	}
	fieldsFor := func(identity epathSQLDirectHVACSourceIdentity) map[string]any {
		source := epathSQLDirectHVACSourceUnitCandidate(identity)
		source.ScopeDetails = nil
		data, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		fields := map[string]any{}
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		// Original v2 wire explicitly contains both measured zero scalars.
		fields["rawValue"], fields["effectiveValue"] = source.RawValue, source.EffectiveValue
		return fields
	}
	decode := func(fields map[string]any) PurposeResultBundle {
		data, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		var source EnergyDataSource
		if err := json.Unmarshal(data, &source); err != nil {
			t.Fatal(err)
		}
		return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Sources: []EnergyDataSource{source}, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}}}}}
	}
	for _, check := range []epathSQLModelCheck{raw, effective} {
		bundle := decode(fieldsFor(*check.DirectHVACSource))
		before, _ := json.Marshal(bundle.EnergyExplanation.Sources[0])
		if err := epathCheckSQLDirectHVACSource(bundle, check); err != nil {
			t.Fatalf("exact explicit owned zero was unknown: %v", err)
		}
		after, _ := json.Marshal(bundle.EnergyExplanation.Sources[0])
		if string(before) != string(after) || len(bundle.EnergyExplanation.Sources[0].ScopeDetails) != 0 {
			t.Fatal("source proof fabricated a scope detail or changed original wire data")
		}
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any, *epathSQLModelCheck)
	}{
		{"missing raw", func(f map[string]any, _ *epathSQLModelCheck) { delete(f, "rawValue") }},
		{"null raw", func(f map[string]any, _ *epathSQLModelCheck) { f["rawValue"] = nil }},
		{"nonzero root", func(f map[string]any, _ *epathSQLModelCheck) { f["rawValue"] = .000001 }},
		{"wrong Zone source", func(f map[string]any, _ *epathSQLModelCheck) { f["zoneName"] = "Other" }},
		{"blank Zone source", func(f map[string]any, _ *epathSQLModelCheck) { delete(f, "zoneName") }},
		{"wrong requested Zone", func(_ map[string]any, c *epathSQLModelCheck) { c.Item.Zone = "Other" }},
		{"wrong model basis", func(f map[string]any, _ *epathSQLModelCheck) { f["aggregationBasis"] = "per_zone" }},
		{"wrong multiplier", func(f map[string]any, _ *epathSQLModelCheck) { f["effectiveMultiplier"] = 6 }},
		{"monthly context", func(_ map[string]any, c *epathSQLModelCheck) { c.Item.Period = "M1" }},
		{"wrong reporting frequency", func(f map[string]any, _ *epathSQLModelCheck) { f["reportingFrequency"] = "Annual" }},
		{"malformed detail", func(f map[string]any, _ *epathSQLModelCheck) { f["scopeDetails"] = []any{map[string]any{}} }},
		{"foreign detail", func(f map[string]any, _ *epathSQLModelCheck) {
			f["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Other"}, "rawValue": 0, "effectiveValue": 0}}
		}},
		{"same owner missing detail value", func(f map[string]any, _ *epathSQLModelCheck) {
			f["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Office"}}}
		}},
		{"same owner null detail value", func(f map[string]any, _ *epathSQLModelCheck) {
			f["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Office"}, "rawValue": nil}}
		}},
		{"same owner contradictory detail", func(f map[string]any, _ *epathSQLModelCheck) {
			f["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Office"}, "rawValue": 1}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fields, check := fieldsFor(*raw.DirectHVACSource), raw
			test.mutate(fields, &check)
			if err := epathCheckSQLDirectHVACSource(decode(fields), check); err == nil {
				t.Fatal("unknown/foreign/contradictory source became an owned zero")
			}
		})
	}
	for _, missing := range []bool{true, false} {
		fields := fieldsFor(*effective.DirectHVACSource)
		if missing {
			delete(fields, "effectiveValue")
		} else {
			fields["effectiveValue"] = nil
		}
		bundle := decode(fields)
		if err := epathCheckSQLDirectHVACSource(bundle, effective); err == nil {
			t.Fatal("known raw zero fabricated unknown effective zero")
		}
		if err := epathCheckSQLDirectHVACSource(bundle, raw); err != nil {
			t.Fatal("independently explicit raw zero was lost with missing effective scalar")
		}
	}
	bundle := decode(fieldsFor(*raw.DirectHVACSource))
	bundle.EnergyExplanation.Sources[0].inspectorDecodedFromJSON = false
	if err := epathCheckSQLDirectHVACSource(bundle, raw); err == nil {
		t.Fatal("undecoded metadata fabricated explicit wire zero")
	}
	bundle = decode(fieldsFor(*raw.DirectHVACSource))
	bundle.EnergyExplanation.Schema = "semantic-idf.energy-explanation/v1"
	if err := epathCheckSQLDirectHVACSource(bundle, raw); err == nil {
		t.Fatal("legacy wire received v2-only source proof fallback")
	}
	if err := epathCheckSQLDirectHVACSource(decode(fieldsFor(*positive.DirectHVACSource)), positive); err == nil {
		t.Fatal("nonzero source bypassed the existing strict scope-detail reader")
	}
}
