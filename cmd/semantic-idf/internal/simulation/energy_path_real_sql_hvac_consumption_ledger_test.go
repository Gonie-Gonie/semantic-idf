package simulation

import (
	"fmt"
	"math"
)

type epathSQLHVACConsumptionLedgerProof struct {
	Service             *epathSQLHVACConsumptionServiceFrames
	Carrier, Period, ID string
}

func epathSQLHVACConsumptionLedgerFields(p *epathSQLHVACConsumptionLedgerProof) (map[string]epathSQLQuantity, error) {
	if p == nil || p.Service == nil || !epathOracleValidPeriod(p.Period) {
		return nil, fmt.Errorf("missing native pool ledger proof")
	}
	if err := epathSQLValidateHVACConsumptionService(p.Service); err != nil {
		return nil, err
	}
	if !epathSQLHVACConsumptionHasString(p.Service.Direct.Carriers, p.Carrier) {
		return nil, fmt.Errorf("foreign pool ledger carrier")
	}
	id, err := epathSQLAllocationID(p.Service.service.CarrierReconciliationIDs[p.Carrier], p.Period)
	if err != nil || id != p.ID {
		return nil, fmt.Errorf("pool ledger identity changed")
	}
	fields := map[string]epathSQLQuantity{}
	for _, month := range epathSQLPeriodMonths(p.Period) {
		row := p.Service.Direct.Monthly[month-1].ByCarrier[p.Carrier]
		parts := map[string]epathSQLQuantity{"expectedValue": row.Site, "directValue": row.Direct, "allocatedValue": row.Allocated, "unassignedValue": row.Unassigned, "overmappedValue": p.Service.Overmapped[month-1][p.Carrier]}
		for field, q := range parts {
			if !q.valid() || q.Value < 0 {
				return nil, fmt.Errorf("unknown/negative native pool ledger field")
			}
			fields[field] = fields[field].add(q)
		}
		residual := row.Site.add(row.Direct.times(-1)).add(row.Allocated.times(-1))
		fields["residualValue"] = fields["residualValue"].add(residual)
		if math.Abs(residual.Value-row.Unassigned.Value+parts["overmappedValue"].Value) > 1e-10*math.Max(1, row.Site.Value) {
			return nil, fmt.Errorf("native pool ledger does not conserve its displayed meter")
		}
	}
	return fields, nil
}

func epathSQLHVACConsumptionBuildingLedgerChecks(service epathRealSQLService, pool *epathSQLHVACConsumptionServiceFrames, period string, checks *epathSQLModelChecks) error {
	if pool == nil || service.ReconciliationID != "" || len(service.CarrierReconciliationIDs) != len(pool.Direct.Carriers) {
		return fmt.Errorf("source-local ledger requires explicit carrier partitions")
	}
	for _, carrier := range pool.Direct.Carriers {
		id, err := epathSQLAllocationID(service.CarrierReconciliationIDs[carrier], period)
		if err != nil {
			return err
		}
		p := &epathSQLHVACConsumptionLedgerProof{Service: pool, Carrier: carrier, Period: period, ID: id}
		fields, err := epathSQLHVACConsumptionLedgerFields(p)
		if err != nil {
			return err
		}
		start := len(checks.Rows)
		for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
			q := fields[field]
			target := epathRealOracleTarget{Collection: "reconciliation", ID: id, Level: "allocation", Field: field, Unit: "kWh"}
			if err := checks.add("zoneAllocation", "building", "", period, service.Service+"/"+carrier+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
				return err
			}
		}
		if err := checks.bindAllocationProof(start); err != nil {
			return err
		}
		checks.Rows[start].Allocation.HVACConsumption = p
	}
	return nil
}

func epathCheckSQLHVACConsumptionAllocation(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	if check.Allocation == nil {
		return fmt.Errorf("missing source-local allocation check")
	}
	p := check.Allocation.HVACConsumption
	fields, err := epathSQLHVACConsumptionLedgerFields(p)
	if err != nil {
		return err
	}
	if check.Item.Scope != "building" || check.Item.Zone != "" || check.Item.Period != p.Period || check.Item.Target.ID != p.ID || check.Item.Target.Collection != "reconciliation" || check.Item.Target.Level != "allocation" || check.Item.Target.Unit != "kWh" || check.Item.Group != "zoneAllocation" {
		return fmt.Errorf("source-local ledger escaped exact context")
	}
	for field, q := range check.Allocation.fields() {
		if q == nil || !epathSQLZoneCarrierQuantityEqual(*q, fields[field]) {
			return fmt.Errorf("changed source-local allocation scalar %s", field)
		}
	}
	q, exists := fields[check.Item.Target.Field]
	if !exists || check.Quantity == nil || !epathSQLZoneCarrierQuantityEqual(*check.Quantity, q) {
		return fmt.Errorf("changed selected source-local scalar")
	}
	_, _, rows, _, err := epathOracleGraph(bundle, "building", "", p.Period)
	if err != nil {
		return err
	}
	var found *EnergyReconciliation
	for i := range rows {
		if rows[i].ID == p.ID {
			if found != nil {
				return fmt.Errorf("duplicate source-local ledger")
			}
			found = &rows[i]
		}
	}
	if found == nil {
		for _, value := range fields {
			if value.Value != 0 {
				return fmt.Errorf("missing nonzero source-local ledger")
			}
		}
		return nil
	}
	if found.Level != "allocation" || found.Period != p.Period || found.ZoneName != "" || found.Unit != "kWh" || found.ServiceKind != p.Service.service.Service {
		return fmt.Errorf("source-local ledger metadata changed")
	}
	actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
	if err != nil || actual == nil {
		return fmt.Errorf("source-local ledger target mismatch: %v", err)
	}
	for field, value := range map[string]float64{"expectedValue": found.ExpectedValue, "directValue": found.DirectValue, "allocatedValue": found.AllocatedValue, "unassignedValue": found.UnassignedValue, "overmappedValue": found.OvermappedValue, "residualValue": found.ResidualValue} {
		q := fields[field]
		if err := epathCheckSQLModelQuantity(&value, &q); err != nil {
			return fmt.Errorf("source-local %s: %w", field, err)
		}
	}
	return nil
}
