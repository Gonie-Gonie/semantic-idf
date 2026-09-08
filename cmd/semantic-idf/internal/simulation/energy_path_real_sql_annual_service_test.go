package simulation

import (
	"fmt"
	"strings"
)

// Temporal authority belongs to the independently observed site sources, not
// to the candidate graph or a caller's guessed monthly profile. Combining
// annual-only and monthly carriers within one service requires a separately
// reviewed mixed-temporal allocation proof; it must never silently fall back.
func epathSQLServiceIsAnnualOnly(frames epathSQLFrames, service epathRealSQLService) (bool, error) {
	if len(service.SiteIDs) == 0 {
		return false, fmt.Errorf("service has no exact site source")
	}
	seen, annualCount := map[string]bool{}, 0
	for _, id := range service.SiteIDs {
		if seen[id] {
			return false, fmt.Errorf("duplicate service site source %s", id)
		}
		seen[id] = true
		if _, err := epathSQLSitePeriod(frames, id, "annual"); err != nil {
			return false, err
		}
		if _, annual := frames.SiteAnnual[id]; annual {
			annualCount++
		}
	}
	if annualCount != 0 && annualCount != len(service.SiteIDs) {
		return false, fmt.Errorf("mixed annual-only/monthly service requires an independent temporal allocation proof")
	}
	return annualCount > 0, nil
}

func epathSQLAnnualServedLoad(frames epathSQLFrames, served map[string]bool, service string) (epathSQLQuantity, error) {
	load := epathSQLQuantity{}
	if len(served) == 0 || service != "cooling" && service != "heating" {
		return load, fmt.Errorf("annual service requires explicit served loads")
	}
	for zone, member := range served {
		if !member || frames.Zones[zone].Name == "" {
			return load, fmt.Errorf("invalid annual served Zone membership")
		}
		for month := 1; month <= 12; month++ {
			q, ok := frames.Loads[epathSQLKey(zone, service, month)]
			if !ok || !q.valid() || q.Value < 0 {
				return load, fmt.Errorf("annual service requires all twelve known served load observations")
			}
			load = load.add(q.positive())
		}
	}
	return load, nil
}

func epathSQLAnnualBuildingServiceChecks(frames epathSQLFrames, service epathRealSQLService, served map[string]bool, checks *epathSQLModelChecks) error {
	load, err := epathSQLAnnualServedLoad(frames, served, service.Service)
	if err != nil {
		return err
	}
	if _, high := load.bounds(); load.Value == 0 && high != 0 {
		return fmt.Errorf("uncertain annual served denominator")
	}
	consumption := epathSQLQuantity{}
	for _, id := range service.SiteIDs {
		value, err := epathSQLSitePeriod(frames, id, "annual")
		if err != nil || value == nil {
			return fmt.Errorf("annual service consumption is unknown: %s: %v", id, err)
		}
		consumption = consumption.add(value.positive())
	}
	for _, basis := range []string{service.Basis, service.FallbackBasis} {
		from, to := epathSQLQuantity{}, epathSQLQuantity{}
		kind := service.FallbackRatioKind
		if basis == service.Basis {
			kind = service.RatioKind
			if load.Value > 0 && consumption.Value > 0 {
				from, to = load, consumption
			}
		}
		kind, err = epathSQLConversionRatioKind(kind, from, to)
		if err != nil {
			return err
		}
		var ratio *epathSQLQuantity
		if from.Value > 0 && to.Value > 0 {
			ratio = &epathSQLQuantity{Value: from.Value / to.Value}
		}
		target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: service.Service, Basis: basis, FromUnit: "kWh", ToUnit: "kWh", RatioKind: kind, Aggregate: "sum"}
		if err := checks.add("ratios", "building", "", "annual", service.Service+"/"+basis, "ratio", ratio, target, "", nil, nil); err != nil {
			return err
		}
		checks.Rows[len(checks.Rows)-1].Conversion = &epathSQLConversionProof{From: from, To: to}
		for month := 1; month <= 12; month++ {
			if err := checks.add("ratios", "building", "", fmt.Sprintf("M%d", month), service.Service+"/"+basis, "ratio", nil, target, "unavailable", nil, nil); err != nil {
				return err
			}
			checks.Rows[len(checks.Rows)-1].AnnualServiceAbsent = true
		}
	}
	assigned, unassigned := epathSQLQuantity{}, consumption
	if load.Value > 0 {
		assigned, unassigned = consumption, epathSQLQuantity{}
	}
	start := len(checks.Rows)
	for _, item := range []struct {
		field string
		value epathSQLQuantity
	}{{"expectedValue", consumption}, {"directValue", epathSQLQuantity{}}, {"allocatedValue", assigned}, {"unassignedValue", unassigned}} {
		target := epathRealOracleTarget{Collection: "reconciliation", ID: service.ReconciliationID, Level: "allocation", Field: item.field, Unit: "kWh"}
		if err := checks.add("zoneAllocation", "building", "", "annual", service.Service+"/"+item.field, "kWh", &item.value, target, "", nil, nil); err != nil {
			return err
		}
	}
	if err := checks.bindAllocationProof(start); err != nil {
		return err
	}
	for month := 1; month <= 12; month++ {
		period := fmt.Sprintf("M%d", month)
		id, err := epathSQLAllocationID(service.ReconciliationID, period)
		if err != nil {
			return err
		}
		for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
			target := epathRealOracleTarget{Collection: "reconciliation", ID: id, Level: "allocation", Field: field, Unit: "kWh"}
			if err := checks.add("zoneAllocation", "building", "", period, service.Service+"/"+field, "kWh", nil, target, "unavailable", nil, nil); err != nil {
				return err
			}
			checks.Rows[len(checks.Rows)-1].AnnualServiceAbsent = true
		}
	}
	return nil
}

// An absent monthly observation is not a reported zero. Even an empty-valued
// allocation row or a zero conversion link would falsely claim monthly evidence.
func epathCheckSQLAnnualServiceAbsent(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	if !check.AnnualServiceAbsent || check.Quantity != nil || check.Want.Status != "unavailable" || check.Item.Scope != "building" || check.Item.Period == "annual" || !epathOracleValidPeriod(check.Item.Period) {
		return fmt.Errorf("invalid annual-only monthly absence proof")
	}
	_, links, records, _, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	target := check.Item.Target
	switch target.Collection {
	case "links":
		if target.Relation != "load_to_end_use" || target.Service != "cooling" && target.Service != "heating" {
			return fmt.Errorf("invalid monthly conversion absence target")
		}
		for _, link := range links {
			if link.Relation == target.Relation && strings.EqualFold(link.ServiceKind, target.Service) {
				return fmt.Errorf("annual-only consumption invented a monthly conversion")
			}
		}
	case "reconciliation":
		if target.ID == "" || target.Level != "allocation" {
			return fmt.Errorf("invalid monthly allocation absence target")
		}
		for _, record := range records {
			if record.ID == target.ID {
				return fmt.Errorf("annual-only consumption invented a monthly allocation record")
			}
		}
	default:
		return fmt.Errorf("unsupported annual service absence target")
	}
	return nil
}
