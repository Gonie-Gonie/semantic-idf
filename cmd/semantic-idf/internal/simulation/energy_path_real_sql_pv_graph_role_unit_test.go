package simulation

// UNINSTALLED DRAFT. Literal roles and zero-valued graph records deliberately
// contain no candidate-derived energy expectations. Synthetic compact identity
// frames exercise the cheap constructor, not native calendar/balance acceptance.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func epathSQLPVGraphRoleHand(t *testing.T) (epathSQLPVGraphRoleProof, PurposeResultBundle) {
	t.Helper()
	_, original := epathSQLPVSourceOriginal(t)
	core := epathSQLPVSourceFrames{Original: original, ExecutedSHA256: strings.Repeat("e", 64), SQLSHA256: strings.Repeat("a", 64), DictionaryCensusComplete: true, Sources: map[int]epathSQLPVSourceIdentity{}}
	cg := epathSQLPVCogenerationFrames{SQLSHA256: core.SQLSHA256, Membership: epathSQLPVCogenerationMembership{OriginalSHA256: original.OriginalSHA256, ExecutedSHA256: core.ExecutedSHA256, MTDSHA256: strings.Repeat("b", 64)}, Parents: map[string]epathSQLPVSourceIdentity{}, AncillaryIDs: map[string]int{}}
	specs, err := epathSQLPVSourceSpecs(original)
	if err != nil {
		t.Fatal(err)
	}
	specs = append(specs, epathSQLPVCogenerationParentSpec())
	b := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}}}
	for n, spec := range specs {
		for offset, f := range []string{"Monthly", "Hourly"} {
			id := 2000 + 2*n + offset
			source := epathRealSQLSource{DictionaryIndex: id, Name: spec.Name, KeyValue: spec.Key, IsMeter: spec.IsMeter, SourceUnit: "J", ReportingFrequency: f}
			native := epathSQLPVSourceIdentity{Spec: spec, Source: source, Dictionary: epathSQLPVNativeDictionary{Index: id, Name: spec.Name, Key: spec.Key, IsMeter: spec.IsMeter, Frequency: f, Unit: "J"}, RequestBound: true}
			if spec.ID == "cogeneration.electricity" {
				cg.Parents[f] = native
			} else {
				core.Sources[id] = native
			}
			if spec.ID == "inverter.ancillary" {
				cg.AncillaryIDs[f] = id
			}
			b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), SourceType: "sql_report_data", Name: spec.Name, KeyValue: spec.Key, IsMeter: spec.IsMeter, SourceUnit: "J", ReportingFrequency: f})
		}
	}
	p, err := epathSQLPVGraphRolesFromValidated(core, cg)
	if err != nil {
		t.Fatal(err)
	}
	// These IDs are the independent literal list above, not learned by looking
	// up the candidate's role/label/value: demand2032, produced2034, bought2036,
	// sold2038, parent2040, charge2020, ancillary2018, discharge2024.
	node := func(id, level, endUse, source string) EnergyExplanationNode {
		return EnergyExplanationNode{ID: id, Level: level, Kind: "energy." + endUse, EndUse: endUse, Carrier: "electricity", Unit: "kWh", ScaleDomain: "site", SourceIDs: []string{source}}
	}
	b.EnergyExplanation.Nodes = []EnergyExplanationNode{node("demand", "carrier", "total", "sql-rdd-2032"), node("purchased", "support", "electricity_purchased", "sql-rdd-2036"), node("produced", "support", "generators", "sql-rdd-2034"), node("sold", "support", "electricity_sold", "sql-rdd-2038"), node("input", "end_use", "other", "sql-rdd-2040"), node("charge", "support", "storage_charge", "sql-rdd-2020")}
	for _, x := range []struct{ node, source string }{{"purchased", "sql-rdd-2036"}, {"produced", "sql-rdd-2034"}, {"sold", "sql-rdd-2038"}, {"input", "sql-rdd-2040"}} {
		relation := "support_supply"
		if x.node == "input" {
			relation = "direct_end_use_to_carrier"
		}
		b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, EnergyPathLink{ID: x.node + "-demand", FromID: x.node, ToID: "demand", Relation: relation, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{x.source}})
	}
	b.EnergyExplanationSummary = EnergyExplanationSummary{Scope: EnergyExplanationScope{Kind: "building"}, Carriers: []EnergyExplanationSummaryItem{{ID: "demand", Level: "carrier", Carrier: "electricity", SourceIDs: []string{"sql-rdd-2032"}}}, EndUses: []EnergyExplanationSummaryItem{{ID: "input", Level: "end_use", EndUse: "other", SourceIDs: []string{"sql-rdd-2040"}}}}
	b.EnergyExplanation.Periods = []EnergyPeriod{{ID: "annual", Kind: "annual"}}
	for m := 1; m <= 12; m++ {
		b.EnergyExplanation.Periods = append(b.EnergyExplanation.Periods, EnergyPeriod{ID: fmt.Sprintf("M%d", m), Kind: "monthly"})
	}
	// Zero demand has no supply endpoint in the native builder. Keep support
	// records as explicit zeros, with no supply links; the CG test edge remains.
	b.EnergyExplanation.Links = b.EnergyExplanation.Links[3:]
	return p, b
}

func epathSQLPVGraphRoleCopy(t *testing.T, b PurposeResultBundle) PurposeResultBundle {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return copy
}

func TestEnergyPathSQLPVGraphRolesLiteralGlobalConsumptionAndIsolatedCharge(t *testing.T) {
	p, b := epathSQLPVGraphRoleHand(t)
	if err := epathSQLCheckPVGraphRoles(b, p); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		b = epathSQLPVGraphRoleCopy(t, b)
		if err := epathSQLCheckPVGraphRoles(b, p); err != nil {
			t.Fatal(err)
		}
	}
	// A negative native produced cell forbids positive/magnitude supply even
	// when both the graph value and the final annual source value are zero.
	b.EnergyExplanation.Nodes[2].Value = 0
	b.EnergyExplanation.Nodes[2].AllocatedValue = 0
	native := p.Identities["sql-rdd-2035"]
	native.HasNegative = true
	p.Identities["sql-rdd-2035"] = native
	p.seal = epathSQLPVGraphRoleSeal(p)
	if err := epathSQLCheckPVGraphRoles(b, p); err == nil {
		t.Fatal("Hourly negative production hidden behind Monthly support")
	}
	b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes[:2], b.EnergyExplanation.Nodes[3:]...)
	if err := epathSQLCheckPVGraphRoles(b, p); err != nil {
		t.Fatal("signed source-only produced context rejected", err)
	}
}

func TestEnergyPathSQLPVGraphRolesZeroValuedPromotionAndDuplicateDenials(t *testing.T) {
	p, base := epathSQLPVGraphRoleHand(t)
	for _, tc := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"charge supply", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, EnergyPathLink{ID: "bad", FromID: "charge", ToID: "demand", Relation: "support_supply", FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2020"}})
		}},
		{"charge hidden consumption", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[4].SourceIDs = append(b.EnergyExplanation.Nodes[4].SourceIDs, "sql-rdd-2020")
		}},
		{"DC duplicate production", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[2].SourceIDs = []string{"sql-rdd-2000"} }},
		{"storage discharge duplicate production", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[2].SourceIDs = []string{"sql-rdd-2024"} }},
		{"load center duplicate production", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[2].SourceIDs = []string{"sql-rdd-2028"} }},
		{"component thermal HVAC", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, EnergyExplanationNode{ID: "load", Level: "load", Kind: "load.heating", Unit: "kWh", ScaleDomain: "thermal", SourceIDs: []string{"sql-rdd-2026"}})
		}},
		{"hidden thermal breakdown", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[0].LoadBreakdown = []EnergyExplanationLoadComponent{{Component: "sensible", SourceIDs: []string{"sql-rdd-2030"}}}
		}},
		{"parent plus ancillary", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[4].SourceIDs = append(b.EnergyExplanation.Nodes[4].SourceIDs, "sql-rdd-2018")
		}},
		{"parent used as generation", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[2].SourceIDs = []string{"sql-rdd-2040"} }},
		{"demand used as production", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[2].SourceIDs = []string{"sql-rdd-2032"} }},
		{"production source missing", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[2].SourceIDs = nil }},
		{"M plus H generation", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[2].SourceIDs = append(b.EnergyExplanation.Nodes[2].SourceIDs, "sql-rdd-2035")
		}},
		{"second generation support", func(b *PurposeResultBundle) {
			n := b.EnergyExplanation.Nodes[2]
			n.ID = "duplicate"
			b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, n)
		}},
		{"second supply ribbon", func(b *PurposeResultBundle) {
			l := EnergyPathLink{ID: "first", FromID: "produced", ToID: "demand", Relation: "support_supply", FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2034"}}
			b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, l)
			l.ID = "duplicate"
			b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, l)
		}},
		{"empty-source non-supply support edge", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, EnergyPathLink{ID: "bad", FromID: "produced", ToID: "demand", Relation: "direct_end_use_to_carrier"})
		}},
		{"wrong supply authority", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, EnergyPathLink{ID: "bad", FromID: "produced", ToID: "demand", Relation: "support_supply", FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2032"}})
		}},
		{"fake zero path allocation", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[2].AllocationApplied = true
			b.EnergyExplanation.Nodes[2].RelatedPathIDs = []string{"fabricated"}
		}},
		{"fake source Zone scope", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Sources[20].ScopeDetails = []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}}}
		}},
		{"ancestry laundering", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: "renamed", SourceType: "derived", InputSourceIDs: []string{"sql-rdd-2020"}})
			b.EnergyExplanation.Nodes[4].SourceIDs = []string{"renamed"}
		}},
		{"ancestry cycle", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: "renamed", InputSourceIDs: []string{"renamed", "sql-rdd-2020"}})
		}},
		{"borrowed actual source identity", func(b *PurposeResultBundle) { b.EnergyExplanation.Sources[20].Name = "Lights Electricity Energy" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := epathSQLPVGraphRoleCopy(t, base)
			tc.mutate(&b)
			if err := epathSQLCheckPVGraphRoles(b, p); err == nil {
				t.Fatal("invalid zero-valued PV graph role accepted")
			}
		})
	}
}

func TestEnergyPathSQLPVGraphRolesAllOriginalWireScopesAndCaches(t *testing.T) {
	p, base := epathSQLPVGraphRoleHand(t)
	bad := EnergyExplanationNode{ID: "bad-zero", Level: "support", Kind: "energy.storage_charge", EndUse: "storage_charge", Carrier: "electricity", Unit: "kWh", ScaleDomain: "site", SourceIDs: []string{"sql-rdd-2020"}}
	cache := EnergyExplanationSummaryItem{ID: "bad-zero", Level: "end_use", EndUse: "other", SourceIDs: []string{"sql-rdd-2020"}}
	for _, tc := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"root cache", func(b *PurposeResultBundle) {
			b.EnergyExplanationSummary.EndUses = append(b.EnergyExplanationSummary.EndUses, cache)
		}},
		{"monthly cache", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[1].Summary = &EnergyExplanationSummary{EndUses: []EnergyExplanationSummaryItem{cache}}
		}},
		{"annual copy cache", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[0].Summary = &EnergyExplanationSummary{Loads: []EnergyExplanationSummaryItem{cache}}
		}},
		{"monthly graph", func(b *PurposeResultBundle) {
			n := bad
			n.Level = "end_use"
			b.EnergyExplanation.Periods[1].Nodes = []EnergyExplanationNode{n}
		}},
		{"Zone graph", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, Nodes: []EnergyExplanationNode{bad}}}
		}},
		{"Zone cache", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, Summary: EnergyExplanationSummary{EndUses: []EnergyExplanationSummaryItem{cache}}}}
		}},
		{"Zone monthly graph", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, Periods: []EnergyPeriod{{ID: "M1", Nodes: []EnergyExplanationNode{bad}}}}}
		}},
		{"Zone monthly cache", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, Periods: []EnergyPeriod{{ID: "M1", Summary: &EnergyExplanationSummary{EndUses: []EnergyExplanationSummaryItem{cache}}}}}}
		}},
		{"root Zone contribution", func(b *PurposeResultBundle) {
			b.EnergyExplanation.ZoneContributions = []EnergyExplanationSummaryItem{cache}
		}},
		{"monthly Zone contribution", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[1].ZoneContributions = []EnergyExplanationSummaryItem{cache}
		}},
		{"parent fake Zone allocation", func(b *PurposeResultBundle) {
			n := b.EnergyExplanation.Nodes[4]
			b.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}, Nodes: []EnergyExplanationNode{n}}}
		}},
		{"duplicate parent cache", func(b *PurposeResultBundle) {
			b.EnergyExplanationSummary.EndUses = append(b.EnergyExplanationSummary.EndUses, b.EnergyExplanationSummary.EndUses[0])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := epathSQLPVGraphRoleCopy(t, base)
			tc.mutate(&b)
			if err := epathSQLCheckPVGraphRoles(b, p); err == nil {
				t.Fatal("unchecked scope/cache permitted a zero-valued forbidden PV role")
			}
		})
	}
	if err := epathSQLCheckPVGraphRoles(base, epathSQLPVGraphRoleProof{}); err == nil {
		t.Fatal("missing mandatory prepared proof accepted")
	}
	delete(p.Identities, "sql-rdd-2020")
	if err := epathSQLCheckPVGraphRoles(base, p); err == nil {
		t.Fatal("mutated prepared native identity accepted")
	}
}
