package simulation

import (
	"encoding/json"
	"strings"
	"testing"
)

// Hand zero observations and explicit original identities. No real candidate
// numbers, production allocator, or candidate-to-expected copying is used.
func epathSQLPVZoneBudgetHand(t *testing.T) (epathSQLPVGraphRoleProof, PurposeResultBundle, epathSQLModelChecks) {
	t.Helper()
	p, b := epathSQLPVGraphRoleHand(t)
	zero := epathSQLQuantity{}
	checks := epathSQLModelChecks{PVHVACRequired: true}
	add := func(group, key string, target epathRealOracleTarget) *epathSQLModelCheck {
		if err := checks.add(group, "zone", "A", "M1", key, "kWh", &zero, target, "", nil, nil); err != nil {
			t.Fatal(err)
		}
		return &checks.Rows[len(checks.Rows)-1]
	}
	direct := epathRealSQLSource{DictionaryIndex: 3000, Name: "Zone Lights Electricity Energy", KeyValue: "A", SourceUnit: "J", ReportingFrequency: "Monthly"}
	meter := epathRealSQLSource{DictionaryIndex: 3001, Name: "Heating:Electricity", IsMeter: true, SourceUnit: "J", ReportingFrequency: "Monthly"}
	b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources,
		EnergyDataSource{ID: "sql-rdd-3000", Name: direct.Name, KeyValue: "A", SourceType: "sql_report_data", SourceUnit: "J", ReportingFrequency: "Monthly"},
		EnergyDataSource{ID: "sql-rdd-3001", Name: meter.Name, IsMeter: true, SourceType: "sql_report_data", SourceUnit: "J", ReportingFrequency: "Monthly"})
	for _, endUse := range []string{"lighting", "equipment"} {
		target := epathSQLNodeTarget("end_use", endUse, "", "site")
		target.Basis, target.AllowPrunedZero = "direct_zone_energy", true
		add("endUses", endUse, target).DirectUse = &epathSQLDirectUseProof{EndUse: endUse, Multiplier: 1, Carriers: map[string]epathSQLDirectUseCarrier{"electricity": {Sources: map[string]epathRealSQLSource{"sql-rdd-3000": direct}}}}
	}
	for _, service := range []string{"cooling", "heating"} {
		target := epathSQLNodeTarget("end_use", service, "", "site")
		target.Basis, target.AllowPrunedZero = "service_path_allocation", true
		add("endUses", service, target).ZoneService = &epathSQLZoneServiceProof{Service: service, Basis: "service_path_allocation", Carriers: map[string]epathSQLQuantity{"electricity": {}}, CarrierSources: map[string]map[string]epathRealSQLSource{"electricity": {"sql-rdd-3001": meter}}, RequiredSites: map[string]map[string]bool{"electricity": {}}, LoadSources: map[string]epathRealSQLSource{"sql-rdd-3000": direct}}
	}
	fan := epathSQLNodeTarget("end_use", "fans", "", "site")
	fan.Basis, fan.AllowPrunedZero = "service_path_allocation", true
	add("zoneAllocation", "fan_pools/value", fan)
	flow := &epathSQLSiteFlowProof{EndUse: "fans", Carrier: "electricity", Basis: "service_path_allocation", Relations: map[string]epathSQLQuantity{"direct_end_use_to_carrier": {}, "end_use_to_carrier": {}}, NodeSources: map[string]epathSQLOriginalSource{}}
	for _, relation := range []string{"direct_end_use_to_carrier", "end_use_to_carrier"} {
		for _, field := range []string{"fromValue", "toValue"} {
			target := epathRealOracleTarget{Collection: "links", Relation: relation, Field: field, Basis: flow.Basis, FromUnit: "kWh", ToUnit: "kWh", Aggregate: "sum"}
			add("zoneAllocation", "fan/"+relation+"/"+field, target).SiteFlow = flow
		}
	}
	carrier := &epathSQLZoneCarrierProof{Carrier: "electricity", Basis: "direct_zone_energy", ZoneName: "A", Period: "M1"}
	for _, field := range []string{"value", "allocatedValue"} {
		target := epathSQLNodeTarget("carrier", "electricity", "", "site")
		target.Field, target.Basis, target.AggregationBasis, target.Aggregate = field, carrier.Basis, "model_total", ""
		add("carriers", "zone_subtotal/electricity/"+field, target).ZoneCarrier = carrier
	}
	recon := &epathSQLReconciliationProof{ID: "row", Level: "energy", ZoneName: "A", Period: "M1", Basis: carrier.Basis, Unit: "kWh", Status: "partial"}
	for _, field := range []string{"expectedValue", "explainedValue", "residualValue"} {
		target := epathRealOracleTarget{Collection: "reconciliation", ID: "row", Level: "energy", Field: field, Basis: recon.Basis, Unit: "kWh", Status: "partial"}
		add("residuals", "zone_subtotal/electricity/"+field, target).Reconciliation = recon
	}
	// Optional allocatedValue:0 is known only when explicitly present on the
	// original wire. An in-memory zero carrier is not evidence of that field.
	nodes, err := epathDecodeOriginalOracleNodes([]json.RawMessage{json.RawMessage(`{"id":"zone-carrier","level":"carrier","kind":"energy.electricity.total","carrier":"electricity","endUse":"total","period":"M1","zoneName":"A","unit":"kWh","scaleDomain":"site","basis":"direct_zone_energy","aggregationBasis":"model_total","value":0,"allocatedValue":0,"allocationApplied":true,"allocationExplanation":"literal branch subtotal","relatedPathIds":["literal-path"],"sourceIds":["sql-rdd-2032","sql-rdd-3000"]}`)})
	if err != nil {
		t.Fatal(err)
	}
	n := nodes[0]
	row := EnergyReconciliation{ID: "row", Level: "energy", Period: "M1", ZoneName: "A", Unit: "kWh", Basis: n.Basis, Status: "partial", SourceIDs: []string{"sql-rdd-3000", "sql-rdd-2032"}}
	item := EnergyExplanationSummaryItem{ID: n.ID, Level: "carrier", Kind: n.Kind, Carrier: "electricity", EndUse: "total", ZoneName: "A", Unit: "kWh", ScaleDomain: "site", Basis: n.Basis, AggregationBasis: "model_total", SourceIDs: []string{"sql-rdd-2032", "sql-rdd-3000"}}
	summary := &EnergyExplanationSummary{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "A"}, Carriers: []EnergyExplanationSummaryItem{item}}
	b.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "A"}, Periods: []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: []EnergyExplanationNode{n}, Reconciliation: []EnergyReconciliation{row}, Summary: summary}}}}
	return p, b, checks
}

func TestEnergyPathSQLPVZoneFacilityBudgetLineageLiteral(t *testing.T) {
	p, b, checks := epathSQLPVZoneBudgetHand(t)
	if err := epathSQLCheckPVGraphRoles(b, p, checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLCheckRequiredPVGraph(b, checks, epathSQLPVCogenerationValidatedSources{graph: &p}); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLCheckPVGraphRoles(b, p); err == nil {
		t.Fatal("metadata-only lineage accepted without independent proof consumers")
	}
	for _, mutate := range []struct {
		name  string
		apply func(*EnergyPeriod)
	}{
		{"full budget as subtotal", func(p *EnergyPeriod) { p.Nodes[0].Value = 1 }},
		{"allocation scalar", func(p *EnergyPeriod) { p.Nodes[0].AllocatedValue = 1 }},
		{"wrong allocation role", func(p *EnergyPeriod) { p.Nodes[0].Basis = "zone_load_allocation" }},
		{"Zone path source", func(p *EnergyPeriod) { p.Nodes[0].PathType = "central_air" }},
		{"Hourly parent", func(p *EnergyPeriod) { p.Nodes[0].SourceIDs[0] = "sql-rdd-2033" }},
		{"CG parent", func(p *EnergyPeriod) { p.Nodes[0].SourceIDs[0] = "sql-rdd-2040" }},
		{"component parent", func(p *EnergyPeriod) { p.Nodes[0].SourceIDs[0] = "sql-rdd-2018" }},
		{"supply parent", func(p *EnergyPeriod) { p.Nodes[0].SourceIDs[0] = "sql-rdd-2036" }},
		{"extra CG", func(p *EnergyPeriod) { p.Nodes[0].SourceIDs = append(p.Nodes[0].SourceIDs, "sql-rdd-2040") }},
		{"Facility-only observation", func(p *EnergyPeriod) { p.Nodes[0].SourceIDs = p.Nodes[0].SourceIDs[:1] }},
		{"reconciliation quantity", func(p *EnergyPeriod) { p.Reconciliation[0].ExplainedValue = 1 }},
		{"cached quantity", func(p *EnergyPeriod) { p.Summary.Carriers[0].Value = 1 }},
		{"cached allocation", func(p *EnergyPeriod) { p.Summary.Carriers[0].AllocatedValue = 1 }},
		{"cached source", func(p *EnergyPeriod) { p.Summary.Carriers[0].SourceIDs[1] = "sql-rdd-2040" }},
		{"additive edge", func(p *EnergyPeriod) {
			p.Links = []EnergyPathLink{{ID: "bad", FromID: "zone-carrier", ToID: "zone-carrier", Relation: "direct_end_use_to_carrier", SourceIDs: []string{"sql-rdd-2032"}}}
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			p, b, checks := epathSQLPVZoneBudgetHand(t)
			mutate.apply(&b.EnergyExplanation.ZoneResults[0].Periods[0])
			if err := epathSQLCheckPVGraphRoles(b, p, checks); err == nil {
				t.Fatal("invalid lineage accepted")
			}
		})
	}
	for i := range checks.Rows {
		t.Run(checks.Rows[i].Item.Key, func(t *testing.T) {
			p, b, changed := epathSQLPVZoneBudgetHand(t)
			delete(changed.Keys, changed.Rows[i].Item.Key)
			changed.Rows = append(changed.Rows[:i], changed.Rows[i+1:]...)
			if err := epathSQLCheckPVGraphRoles(b, p, changed); err == nil {
				t.Fatal("deleted scalar/source/endpoint proof accepted")
			}
		})
	}
}

func TestEnergyPathSQLPVCoverageRejectsExternalDeclarationDrift(t *testing.T) {
	model := &epathRealSQLModel{PVSystems: []epathRealSQLPVSystem{{}}}
	report := epathSQLModelCoverage(PurposeResultBundle{}, epathSQLModelChecks{}, model)
	if len(report.Failures) == 0 || report.Failures[0].Key != "pv_native_source/registry" {
		t.Fatal("external PV declaration mismatch was not rejected before registry reuse")
	}
}

func TestEnergyPathSQLPVZoneFacilityDuplicateAnnualCannotBorrowCanonical(t *testing.T) {
	p, b, checks := epathSQLPVZoneBudgetHand(t)
	checks.Keys = map[string]bool{}
	for i := range checks.Rows {
		c := &checks.Rows[i]
		c.Item.Period, c.Want.Period = "annual", "annual"
		c.Item.Key = strings.Replace(c.Item.Key, "|M1|", "|annual|", 1)
		c.Want.Key = c.Item.Key
		checks.Keys[c.Item.Key] = true
		if c.ZoneCarrier != nil {
			c.ZoneCarrier.Period = "annual"
		}
		if c.Reconciliation != nil {
			c.Reconciliation.Period = "annual"
		}
	}
	z := &b.EnergyExplanation.ZoneResults[0]
	z.Periods[0].ID, z.Periods[0].Kind = "annual", "annual"
	z.Periods[0].Nodes[0].Period, z.Periods[0].Reconciliation[0].Period = "annual", "annual"
	z.Nodes = append([]EnergyExplanationNode(nil), z.Periods[0].Nodes...)
	z.Reconciliation = append([]EnergyReconciliation(nil), z.Periods[0].Reconciliation...)
	if err := epathSQLCheckPVGraphRoles(b, p, checks); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"node", "expected", "explained", "residual", "summary"} {
		t.Run(field, func(t *testing.T) {
			copy := epathSQLPVGraphRoleCopy(t, b)
			period := &copy.EnergyExplanation.ZoneResults[0].Periods[0]
			// Production carrier marshaling omits allocatedValue:0. Preserve the
			// independently declared literal wire node rather than allowing an
			// unrelated omitted-field failure to mask the intended mutation.
			copy.EnergyExplanation.ZoneResults[0].Nodes[0] = b.EnergyExplanation.ZoneResults[0].Nodes[0]
			period.Nodes[0] = b.EnergyExplanation.ZoneResults[0].Periods[0].Nodes[0]
			switch field {
			case "node":
				period.Nodes[0].Value = 1
			case "expected":
				period.Reconciliation[0].ExpectedValue = 1
			case "explained":
				period.Reconciliation[0].ExplainedValue = 1
			case "residual":
				period.Reconciliation[0].ResidualValue = 1
			case "summary":
				period.Summary.Carriers[0].Value = 1
			}
			if err := epathSQLCheckPVGraphRoles(copy, p, checks); err == nil {
				t.Fatal("duplicate annual borrowed valid canonical quantities")
			}
		})
	}
}
