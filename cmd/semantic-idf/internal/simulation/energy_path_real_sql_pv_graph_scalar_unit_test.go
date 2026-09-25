package simulation

// Literal native period inputs and literal graph expectations. None of the
// expected quantities below is copied from the candidate or a production API.
import (
	"testing"
)

func epathSQLPVGraphHandAmounts(t *testing.T, p *epathSQLPVGraphRoleProof, spec string, months [12]float64) {
	t.Helper()
	annual := 0.0
	for _, v := range months {
		annual += v
	}
	amounts, err := epathSQLPVGraphNativeAmounts(epathSQLPVSourceIdentity{NativeMonthlyKWh: months, NativeAnnualKWh: annual})
	if err != nil {
		t.Fatal(err)
	}
	for id, identity := range p.Identities {
		if identity.SpecID == spec {
			identity.Amounts = amounts
			p.Identities[id] = identity
		}
	}
	p.seal = epathSQLPVGraphRoleSeal(*p)
}

func epathSQLPVGraphHandNodeValue(n *EnergyExplanationNode, value float64) {
	n.Value = value
	n.RawValue = value
	n.EffectiveValue = value
	n.AllocatedValue = value
	n.Multiplier = 1
}

func epathSQLPVGraphScalarHand(t *testing.T) (epathSQLPVGraphRoleProof, PurposeResultBundle) {
	t.Helper()
	p, b := epathSQLPVGraphRoleHand(t)
	epathSQLPVGraphHandAmounts(t, &p, "facility.demand", [12]float64{10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10})
	epathSQLPVGraphHandAmounts(t, &p, "facility.purchased", [12]float64{.0004, 1.2344, 2.3454})
	epathSQLPVGraphHandAmounts(t, &p, "storage.charge", [12]float64{.5, .5, .5})
	epathSQLPVGraphHandNodeValue(&b.EnergyExplanation.Nodes[0], 120)
	epathSQLPVGraphHandNodeValue(&b.EnergyExplanation.Nodes[1], 3.579)
	epathSQLPVGraphHandNodeValue(&b.EnergyExplanation.Nodes[5], 1.5)
	b.EnergyExplanation.Links = []EnergyPathLink{{ID: "purchase", FromID: "purchased", ToID: "demand", Relation: "support_supply", FromUnit: "kWh", ToUnit: "kWh", FromValue: 3.579, ToValue: 3.579, SourceIDs: []string{"sql-rdd-2036"}}}
	b.EnergyExplanation.Periods[0].Nodes = append([]EnergyExplanationNode(nil), b.EnergyExplanation.Nodes...)
	b.EnergyExplanation.Periods[0].Links = append([]EnergyPathLink(nil), b.EnergyExplanation.Links...)
	for m := 1; m <= 12; m++ {
		demand := b.EnergyExplanation.Nodes[0]
		epathSQLPVGraphHandNodeValue(&demand, 10)
		b.EnergyExplanation.Periods[m].Nodes = []EnergyExplanationNode{demand}
		if m <= 3 {
			charge := b.EnergyExplanation.Nodes[5]
			epathSQLPVGraphHandNodeValue(&charge, .5)
			b.EnergyExplanation.Periods[m].Nodes = append(b.EnergyExplanation.Periods[m].Nodes, charge)
		}
		if m == 2 || m == 3 {
			value := 1.234
			if m == 3 {
				value = 2.345
			}
			purchase := b.EnergyExplanation.Nodes[1]
			epathSQLPVGraphHandNodeValue(&purchase, value)
			b.EnergyExplanation.Periods[m].Nodes = append(b.EnergyExplanation.Periods[m].Nodes, purchase)
			edge := b.EnergyExplanation.Links[0]
			edge.FromValue = value
			edge.ToValue = value
			b.EnergyExplanation.Periods[m].Links = []EnergyPathLink{edge}
		}
	}
	return p, b
}

func TestEnergyPathSQLPVGraphScalarNativeMonthlyAndAnnualTransport(t *testing.T) {
	p, b := epathSQLPVGraphScalarHand(t)
	if err := epathSQLCheckPVGraphRoles(b, p); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		b = epathSQLPVGraphRoleCopy(t, b)
		if err := epathSQLCheckPVGraphRoles(b, p); err != nil {
			t.Fatal(err)
		}
	}
	a := p.Identities["sql-rdd-2036"].Amounts
	if a.SourceAnnual.Value != 3.580 || a.GraphAnnual.Value != 3.579 || a.Monthly[0].Value != 0 || a.Monthly[1].Value != 1.234 || a.Monthly[2].Value != 2.345 {
		t.Fatal("once-annual and monthly-graph transport conflated")
	}
	// Exact native zero remains exact; a half-ulp presentation boundary only
	// inherits the already established native summation interval.
	zero, err := epathSQLPVGraphRound(epathSQLPVBalanceNativeQuantity(0))
	if err != nil || zero.Value != 0 || zero.Error != 0 {
		t.Fatal("known native zero gained uncertainty")
	}
	half, err := epathSQLPVGraphRound(epathSQLPVBalanceNativeQuantity(1.2345))
	if err != nil || !epathSQLPVGraphContains(half, 1.234) || !epathSQLPVGraphContains(half, 1.235) || epathSQLPVGraphContains(half, 1.233) || epathSQLPVGraphContains(half, 1.2344) {
		t.Fatal("native boundary transport is not the literal two-outcome interval")
	}
	// Explicit zero support/ribbon is legal when the independently observed
	// demand endpoint exists; pruning the same display-zero source is legal.
	n := b.EnergyExplanation.Nodes[1]
	epathSQLPVGraphHandNodeValue(&n, 0)
	b.EnergyExplanation.Periods[1].Nodes = append(b.EnergyExplanation.Periods[1].Nodes, n)
	b.EnergyExplanation.Periods[1].Links = []EnergyPathLink{{ID: "zero", FromID: "purchased", ToID: "demand", Relation: "support_supply", FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2036"}}}
	if err := epathSQLCheckPVGraphRoles(b, p); err != nil {
		t.Fatal("zero-prune alternative rejected", err)
	}
}

func TestEnergyPathSQLPVGraphScalarWrongPeriodMagnitudeAndPruningDenials(t *testing.T) {
	p, base := epathSQLPVGraphScalarHand(t)
	for _, tc := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"annual once-source substituted", func(b *PurposeResultBundle) { epathSQLPVGraphHandNodeValue(&b.EnergyExplanation.Nodes[1], 3.580) }},
		{"annual graph value", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].Value = 3.580 }},
		{"raw scalar", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].RawValue = 0 }},
		{"effective scalar", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].EffectiveValue = 0 }},
		{"allocated display scalar", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].AllocatedValue = 0 }},
		{"legacy display scalar", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].DisplayValue = 99 }},
		{"multiplier", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].Multiplier = 2 }},
		{"ribbon from", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].FromValue = 3.580 }},
		{"ribbon to", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].ToValue = 0 }},
		{"positive root node pruned", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes[:1], b.EnergyExplanation.Nodes[2:]...)
			b.EnergyExplanation.Links = nil
		}},
		{"positive root ribbon pruned", func(b *PurposeResultBundle) { b.EnergyExplanation.Links = nil }},
		{"annual copy missing positive", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[0].Nodes = nil
			b.EnergyExplanation.Periods[0].Links = nil
		}},
		{"monthly node pruned", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[2].Nodes = b.EnergyExplanation.Periods[2].Nodes[:2]
			b.EnergyExplanation.Periods[2].Links = nil
		}},
		{"monthly ribbon pruned", func(b *PurposeResultBundle) { b.EnergyExplanation.Periods[2].Links = nil }},
		{"monthly replaced with annual", func(b *PurposeResultBundle) {
			epathSQLPVGraphHandNodeValue(&b.EnergyExplanation.Periods[2].Nodes[2], 3.579)
		}},
		{"isolated charge amount", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[5].Value = 0 }},
		{"isolated positive charge pruned", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods[1].Nodes = b.EnergyExplanation.Periods[1].Nodes[:1]
		}},
		{"entire required month pruned", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Periods = append(b.EnergyExplanation.Periods[:2], b.EnergyExplanation.Periods[3:]...)
		}},
		{"Hourly cannot replace Monthly graph source", func(b *PurposeResultBundle) { b.EnergyExplanation.Nodes[1].SourceIDs = []string{"sql-rdd-2037"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := epathSQLPVGraphRoleCopy(t, base)
			tc.mutate(&b)
			if err := epathSQLCheckPVGraphRoles(b, p); err == nil {
				t.Fatal("native period transport/presence contradiction accepted")
			}
		})
	}
}

func TestEnergyPathSQLPVGraphScalarResidualLineageAndCanonicalOther(t *testing.T) {
	p, b := epathSQLPVGraphRoleHand(t)
	b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: "unrelated-consumption", SourceType: "sql_report_data"})
	b.EnergyExplanation.Nodes[4].SourceIDs = append(b.EnergyExplanation.Nodes[4].SourceIDs, "unrelated-consumption")
	b.EnergyExplanation.Links[0].SourceIDs = append(b.EnergyExplanation.Links[0].SourceIDs, "unrelated-consumption")
	// A fallback carrier split retains the legacy non-HVAC end-use token;
	// it is not an HVAC allocation service or a ratio/path authorization.
	b.EnergyExplanation.Links[0].ServiceKind = "other"
	ids := []string{"sql-rdd-2032", "sql-rdd-2040", "unrelated-consumption"}
	b.EnergyExplanation.Nodes = append(b.EnergyExplanation.Nodes, EnergyExplanationNode{ID: "residual", Level: "residual", Basis: "residual", Carrier: "electricity", Unit: "kWh", ScaleDomain: "site", SourceIDs: ids})
	b.EnergyExplanation.Links = append(b.EnergyExplanation.Links, EnergyPathLink{ID: "residual", FromID: "residual", ToID: "demand", Relation: "residual", Basis: "residual", FromUnit: "kWh", ToUnit: "kWh", SourceIDs: ids})
	b.EnergyExplanation.Reconciliation = []EnergyReconciliation{{ID: "energy", Level: "energy", SourceIDs: ids}, {ID: "unrelated", Level: "allocation", ServiceKind: "heating", AllocatedValue: 12, SourceIDs: []string{"unrelated-consumption"}}}
	b.EnergyExplanationSummary.Residuals = []EnergyExplanationSummaryItem{{ID: "residual", Level: "residual", Basis: "residual", Carrier: "electricity", SourceIDs: ids}}
	if err := epathSQLCheckPVGraphRoles(b, p); err != nil {
		t.Fatal("consumption/residual ancestry confused with another amount", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*PurposeResultBundle)
	}{
		{"ancillary hidden in residual", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[6].SourceIDs = append(b.EnergyExplanation.Nodes[6].SourceIDs, "sql-rdd-2018")
		}},
		{"parent zero allocation ledger", func(b *PurposeResultBundle) { b.EnergyExplanation.Reconciliation[0].Level = "allocation" }},
		{"parent borrowed HVAC ledger", func(b *PurposeResultBundle) { b.EnergyExplanation.Reconciliation[0].ServiceKind = "heating" }},
		{"parent borrowed HVAC link", func(b *PurposeResultBundle) { b.EnergyExplanation.Links[0].ServiceKind = "heating" }},
		{"parent derived duplicate ancestry", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Sources = append(b.EnergyExplanation.Sources, EnergyDataSource{ID: "again", InputSourceIDs: []string{"sql-rdd-2040"}})
			b.EnergyExplanation.Links[0].SourceIDs = append(b.EnergyExplanation.Links[0].SourceIDs, "again")
		}},
		{"parent both observation frequencies", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Links[0].SourceIDs = append(b.EnergyExplanation.Links[0].SourceIDs, "sql-rdd-2041")
		}},
		{"demand mixed lineage as amount", func(b *PurposeResultBundle) {
			b.EnergyExplanation.Nodes[0].SourceIDs = append(b.EnergyExplanation.Nodes[0].SourceIDs, "unrelated-consumption")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := epathSQLPVGraphRoleCopy(t, b)
			tc.mutate(&copy)
			if err := epathSQLCheckPVGraphRoles(copy, p); err == nil {
				t.Fatal("lineage exception authorized forbidden role/quantity")
			}
		})
	}
}
