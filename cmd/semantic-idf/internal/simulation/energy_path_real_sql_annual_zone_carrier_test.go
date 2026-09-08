package simulation

import (
	"fmt"
	"strings"
)

// No monthly subtotal exists for an exclusively annual source. Keep the
// unknown metric and require actual absence, including zero-valued records.
func epathSQLZoneCarrierAbsentChecks(zone, carrier, period string, checks *epathSQLModelChecks) error {
	proof := &epathSQLZoneCarrierProof{Carrier: carrier, Basis: "service_path_allocation", ZoneName: zone, Period: period, Unavailable: true}
	for _, field := range []string{"value", "allocatedValue", "expectedValue", "explainedValue", "residualValue"} {
		group := "carriers"
		target := epathSQLNodeTarget("carrier", carrier, "", "site")
		target.Field, target.Basis, target.AggregationBasis, target.Aggregate = field, proof.Basis, "model_total", ""
		if field != "value" && field != "allocatedValue" {
			group = "residuals"
			target = epathRealOracleTarget{Collection: "reconciliation", ID: "reconcile.energy." + carrier + "." + period + "." + epathSQLZoneCarrierToken(zone), Level: "energy", Field: field, Unit: "kWh", Basis: proof.Basis}
		}
		if err := checks.add(group, "zone", zone, period, "zone_subtotal/"+carrier+"/"+field, "kWh", nil, target, "unavailable", nil, nil); err != nil {
			return err
		}
		checks.Rows[len(checks.Rows)-1].ZoneCarrier = proof
	}
	return nil
}

func epathCheckSQLZoneCarrierAbsent(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p, t := check.ZoneCarrier, check.Item.Target
	if p == nil || !p.Unavailable || p.Carrier == "" || p.ZoneName == "" || p.Period == "annual" || !epathOracleValidPeriod(p.Period) || p.Basis != "service_path_allocation" || check.Item.Scope != "zone" || check.Item.Zone != p.ZoneName || check.Item.Period != p.Period || check.Item.Unit != "kWh" || check.Quantity != nil || check.Want.Value != nil || check.Want.Status != "unavailable" || t.Unit != "kWh" || t.Basis != p.Basis {
		return fmt.Errorf("invalid unavailable Zone carrier proof")
	}
	if t.Collection == "nodes" {
		if t.Level != "carrier" || t.Category != p.Carrier || t.ScaleDomain != "site" || t.AggregationBasis != "model_total" || t.Aggregate != "" || (t.Field != "value" && t.Field != "allocatedValue") {
			return fmt.Errorf("unavailable carrier selector contradiction")
		}
	} else if t.Collection == "reconciliation" {
		if t.ID != "reconcile.energy."+p.Carrier+"."+p.Period+"."+epathSQLZoneCarrierToken(p.ZoneName) || t.Level != "energy" || (t.Field != "expectedValue" && t.Field != "explainedValue" && t.Field != "residualValue") {
			return fmt.Errorf("unavailable accounting selector contradiction")
		}
	} else {
		return fmt.Errorf("unsupported unavailable carrier collection")
	}
	nodes, links, rows, _, err := epathOracleGraph(bundle, "zone", p.ZoneName, p.Period)
	if err != nil {
		return err
	}
	byID := map[string]EnergyExplanationNode{}
	for _, n := range nodes {
		byID[n.ID] = n
		if n.Level == "carrier" && n.Carrier == p.Carrier {
			return fmt.Errorf("unreported monthly carrier was synthesized, even if zero")
		}
	}
	for _, l := range links {
		if byID[l.ToID].Carrier == p.Carrier || byID[l.FromID].Carrier == p.Carrier {
			return fmt.Errorf("unreported monthly carrier gained a flow")
		}
	}
	for _, r := range rows {
		if r.Level == "energy" && strings.HasPrefix(r.ID, "reconcile.energy."+p.Carrier+".") {
			return fmt.Errorf("unreported monthly carrier gained reconciliation")
		}
	}
	return nil
}
