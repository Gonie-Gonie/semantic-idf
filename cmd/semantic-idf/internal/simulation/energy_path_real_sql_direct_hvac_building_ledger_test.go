package simulation

import (
	"fmt"
	"math"
)

func epathSQLDirectHVACRequireCarrierLedger(service epathRealSQLService, direct *epathSQLDirectHVACServiceFrames) error {
	if direct == nil {
		if len(service.CarrierReconciliationIDs) > 0 {
			return fmt.Errorf("carrier ledger declaration requires an independently compiled direct service")
		}
		return nil
	}
	positive := map[string]bool{}
	for _, row := range direct.Monthly {
		for carrier, part := range row.ByCarrier {
			if part.Site.Value > 0 {
				positive[carrier] = true
			}
		}
	}
	if len(positive) > 1 && len(service.CarrierReconciliationIDs) == 0 {
		return fmt.Errorf("multiple originally observed positive carriers cannot share one allocation row")
	}
	return nil
}

// A combined heating conversion has one paired denominator, but its electric
// and gas allocation rows are distinct. Explicit recipe IDs bind each original
// carrier partition; never compare the combined quantity with one fuel's row.
func epathSQLDirectHVACBuildingLedgerChecks(service epathRealSQLService, direct *epathSQLDirectHVACServiceFrames, period string, checks *epathSQLModelChecks) error {
	if direct == nil || len(direct.Carriers) == 0 || service.ReconciliationID != "" || len(service.CarrierReconciliationIDs) != len(direct.Carriers) {
		return fmt.Errorf("carrier allocation ledger requires every independently compiled direct-service carrier")
	}
	seen := map[string]bool{}
	for _, carrier := range direct.Carriers {
		wantID := "reconcile.zone_hvac_allocation." + service.Service + "." + carrier + ".annual"
		if carrier == "" || seen[carrier] || service.CarrierReconciliationIDs[carrier] != wantID {
			return fmt.Errorf("carrier allocation ID lacks its exact service/carrier identity")
		}
		seen[carrier] = true
	}
	for _, month := range epathSQLPeriodMonths(period) {
		row := direct.Monthly[month-1]
		if len(row.ByCarrier) != len(seen) {
			return fmt.Errorf("carrier allocation ledger lost an observed monthly partition")
		}
		sum := epathSQLDirectHVACCarrierLedger{}
		for _, carrier := range direct.Carriers {
			part, exists := row.ByCarrier[carrier]
			if !exists || !part.Site.valid() || !part.Direct.valid() || !part.Allocated.valid() || !part.Unassigned.valid() || part.Site.Value < 0 || part.Direct.Value < 0 || part.Allocated.Value < 0 || part.Unassigned.Value < 0 {
				return fmt.Errorf("unknown/negative carrier allocation partition")
			}
			zoneDirect, zoneAllocated := epathSQLQuantity{}, epathSQLQuantity{}
			for _, shares := range row.Zones {
				share, exists := shares[carrier]
				if !exists {
					return fmt.Errorf("carrier allocation ledger lost a Zone partition")
				}
				zoneDirect = zoneDirect.add(share.Direct)
				zoneAllocated = zoneAllocated.add(share.Allocated)
			}
			if !epathSQLZoneCarrierQuantityEqual(zoneDirect, part.Direct) || !epathSQLZoneCarrierQuantityEqual(zoneAllocated, part.Allocated) {
				return fmt.Errorf("carrier allocation ledger differs from its independently computed Zone shares")
			}
			if math.Abs(part.Site.Value-part.Direct.Value-part.Allocated.Value-part.Unassigned.Value) > 1e-10*math.Max(1, part.Site.Value) {
				return fmt.Errorf("carrier allocation partition does not close against its original pool")
			}
			sum.Site = sum.Site.add(part.Site)
			sum.Direct = sum.Direct.add(part.Direct)
			sum.Allocated = sum.Allocated.add(part.Allocated)
			sum.Unassigned = sum.Unassigned.add(part.Unassigned)
		}
		if !epathSQLZoneCarrierQuantityEqual(sum.Site, row.Site) || !epathSQLZoneCarrierQuantityEqual(sum.Direct, row.Direct) || !epathSQLZoneCarrierQuantityEqual(sum.Allocated, row.Allocated) || !epathSQLZoneCarrierQuantityEqual(sum.Unassigned, row.Unassigned) {
			return fmt.Errorf("carrier allocation partitions disagree with the independent service total")
		}
	}
	for _, carrier := range direct.Carriers {
		id, err := epathSQLAllocationID(service.CarrierReconciliationIDs[carrier], period)
		if err != nil {
			return err
		}
		start := len(checks.Rows)
		for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
			q := epathSQLQuantity{}
			for _, month := range epathSQLPeriodMonths(period) {
				part := direct.Monthly[month-1].ByCarrier[carrier]
				switch field {
				case "expectedValue":
					q = q.add(part.Site)
				case "directValue":
					q = q.add(part.Direct)
				case "allocatedValue":
					q = q.add(part.Allocated)
				case "unassignedValue":
					q = q.add(part.Unassigned)
				}
			}
			target := epathRealOracleTarget{Collection: "reconciliation", ID: id, Level: "allocation", Field: field, Unit: "kWh"}
			if err := checks.add("zoneAllocation", "building", "", period, service.Service+"/"+carrier+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
				return err
			}
		}
		if err := checks.bindAllocationProof(start); err != nil {
			return err
		}
	}
	return nil
}
