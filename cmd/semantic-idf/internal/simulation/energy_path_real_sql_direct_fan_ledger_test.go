package simulation

import (
	"fmt"
	"math"
	"strings"
)

// Presentation is calculated from original Monthly observations, not from a
// candidate ledger. Annual rounding debt retains each month's sign separately.
func epathSQLDirectFanLedgerPresentation(fans *epathSQLDirectFanFrames, period string) (EnergyReconciliation, map[string]epathSQLOriginalSource, error) {
	out := EnergyReconciliation{}
	if fans == nil || !epathOracleValidPeriod(period) || fans.Precision.DecimalPlaces != 3 {
		return out, nil, fmt.Errorf("invalid native fan ledger precision/context")
	}
	allowed := map[string]epathSQLOriginalSource{}
	meterProof := epathSQLOriginalRDD(fans.MeterSource)
	meterKey, err := epathSQLOriginalKey(meterProof)
	if err != nil {
		return out, nil, err
	}
	allowed[meterKey] = meterProof
	meterMonths, err := epathSQLMonthly(fans.MeterSource, fans.Precision)
	if err != nil {
		return out, nil, err
	}
	directMonths := [12]float64{}
	family := ""
	round := func(v float64) float64 { return math.Round(v*1000) / 1000 }
	for zone, sources := range fans.Sources {
		zoneMonthly := [12]epathSQLQuantity{}
		for _, identity := range sources {
			proof, err := epathSQLOriginalDirectHVAC(identity)
			if err != nil {
				return out, nil, err
			}
			key, err := epathSQLOriginalKey(proof)
			if err != nil {
				return out, nil, err
			}
			if _, exists := allowed[key]; exists {
				return out, nil, fmt.Errorf("duplicate native fan ledger source")
			}
			allowed[key] = proof
			if !epathSQLSupportedDirectFanFamily(identity.FamilyID) || family != "" && family != identity.FamilyID || identity.SiteID != fans.SiteID {
				return out, nil, fmt.Errorf("foreign native fan ledger role")
			}
			family = identity.FamilyID
			monthly, err := epathSQLMonthly(identity.Source, identity.Precision)
			if err != nil {
				return out, nil, err
			}
			for i, q := range monthly {
				zoneMonthly[i] = zoneMonthly[i].add(q.positive())
				directMonths[i] = round(directMonths[i] + round(q.Value))
			}
		}
		for i, q := range zoneMonthly {
			if !epathSQLZoneCarrierQuantityEqual(q, fans.Zones[zone][i]) {
				return out, nil, fmt.Errorf("fan ledger roster escaped native Zone source sum")
			}
		}
	}
	for _, month := range epathSQLPeriodMonths(period) {
		i := month - 1
		if !epathSQLZoneCarrierQuantityEqual(meterMonths[i].positive(), fans.Meter[i]) {
			return out, nil, fmt.Errorf("fan meter proof changed")
		}
		expected, direct := round(meterMonths[i].Value), directMonths[i]
		unassigned, overmapped := math.Max(0, round(expected-direct)), math.Max(0, round(direct-expected))
		out.ExpectedValue = round(out.ExpectedValue + expected)
		out.DirectValue = round(out.DirectValue + direct)
		out.UnassignedValue = round(out.UnassignedValue + unassigned)
		out.OvermappedValue = round(out.OvermappedValue + overmapped)
		if expected == 0 && direct == 0 {
			continue
		} // Fully observed zeros may prune the entire monthly row.
		method := "direct_only"
		if unassigned > 0 || direct == 0 {
			method = "unassigned"
		}
		if out.AllocationMethod == "" {
			out.AllocationMethod = method
		} else if out.AllocationMethod != method {
			out.AllocationMethod = "mixed"
		}
	}
	return out, allowed, nil
}

func epathCheckSQLDirectFanAllocation(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	if check.Allocation == nil || check.Allocation.DirectFan == nil || check.Item.Scope != "building" || check.Item.Zone != "" || check.Item.Target.ID != "reconcile.zone_auxiliary_allocation.fans.electricity."+strings.ToLower(check.Item.Period) {
		return fmt.Errorf("native fan ledger escaped exact building identity")
	}
	want, allowed, err := epathSQLDirectFanLedgerPresentation(check.Allocation.DirectFan, check.Item.Period)
	if err != nil {
		return err
	}
	actualSources, err := epathSQLDirectHVACSourceMap(bundle)
	if err != nil {
		return err
	}
	required := map[string]bool{}
	for key, proof := range allowed {
		required[key] = true
		source, exists := actualSources[fmt.Sprintf("sql-rdd-%d", proof.RDD.DictionaryIndex)]
		if !exists || !epathSQLOriginalSourceMatches(source, proof, check.Item.Period) {
			return fmt.Errorf("native fan ledger missing/misqualified original source even at measured zero")
		}
	}
	_, _, rows, _, err := epathOracleGraph(bundle, "building", "", check.Item.Period)
	if err != nil {
		return err
	}
	count := 0
	for _, row := range rows {
		if row.ID != check.Item.Target.ID {
			continue
		}
		count++
		if row.Level != "allocation" || row.Period != check.Item.Period || row.ServiceKind != "fans" || row.ZoneName != "" || row.Basis != "service_path_allocation" || row.Unit != "kWh" || row.AllocationMethod != want.AllocationMethod {
			return fmt.Errorf("native fan ledger has invalid method/identity")
		}
		for _, pair := range [][2]float64{{row.ExpectedValue, want.ExpectedValue}, {row.DirectValue, want.DirectValue}, {row.AllocatedValue, 0}, {row.UnassignedValue, want.UnassignedValue}, {row.OvermappedValue, want.OvermappedValue}} {
			if !epathSQLZoneCarrierQuantityEqual(epathSQLQuantity{Value: pair[0]}, epathSQLQuantity{Value: pair[1]}) {
				return fmt.Errorf("native fan ledger lost source-local rounding or monthly overlap: got %g want %g", pair[0], pair[1])
			}
		}
		if err := epathSQLVerifyOriginalSources(row.SourceIDs, actualSources, allowed, required, check.Item.Period); err != nil {
			return err
		}
	}
	if count > 1 || count == 0 && (want.ExpectedValue != 0 || want.DirectValue != 0) {
		return fmt.Errorf("missing/duplicate native fan ledger")
	}
	return nil
}
