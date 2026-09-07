package simulation

import (
	"fmt"
	"math"
	"strings"
)

func epathSQLRatio(numerator, denominator epathSQLQuantity) (*epathSQLQuantity, error) {
	if denominator.Value <= 0 {
		return nil, nil
	}
	if denominator.Value-denominator.Error <= 0 {
		return nil, fmt.Errorf("uncertain positive ratio denominator")
	}
	value := numerator.Value / denominator.Value
	low := math.Max(0, numerator.Value-numerator.Error) / (denominator.Value + denominator.Error)
	high := (numerator.Value + numerator.Error) / (denominator.Value - denominator.Error)
	return &epathSQLQuantity{value, math.Max(value-low, high-value) + .00051}, nil
}
func epathSQLDeclaredZones(frames epathSQLFrames, names []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, name := range names {
		key := strings.ToLower(name)
		if out[key] || frames.Zones[key].Name == "" {
			return nil, fmt.Errorf("unknown/duplicate served Zone %q", name)
		}
		out[key] = true
	}
	return out, nil
}
func epathSQLSiteSum(frames epathSQLFrames, ids []string, month int) (epathSQLQuantity, error) {
	out := epathSQLQuantity{}
	seen := map[string]bool{}
	for _, id := range ids {
		values, ok := frames.Site[id]
		if !ok || seen[id] || values[month-1] == nil {
			return out, fmt.Errorf("unknown/duplicate service site source %s", id)
		}
		seen[id] = true
		out = out.add(*values[month-1])
	}
	if len(ids) == 0 {
		return out, fmt.Errorf("service has no exact site source")
	}
	return out, nil
}
func epathSQLAllocationID(annualID, period string) (string, error) {
	if !strings.HasSuffix(annualID, ".annual") {
		return "", fmt.Errorf("allocation ID requires explicit annual fixture identity")
	}
	return strings.TrimSuffix(annualID, ".annual") + "." + strings.ToLower(period), nil
}

func epathSQLModelServiceChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	periods := []string{"annual"}
	for month := 1; month <= 12; month++ {
		periods = append(periods, fmt.Sprintf("M%d", month))
	}
	allExpected, allAssigned, allUnassigned := [12]epathSQLQuantity{}, [12]epathSQLQuantity{}, [12]epathSQLQuantity{}
	serviceSeen := map[string]bool{}
	for _, service := range model.Services {
		if (service.Service != "cooling" && service.Service != "heating") || serviceSeen[service.Service] || service.Basis == "" || service.FallbackBasis == "" || service.RatioKind == "" || service.FallbackRatioKind == "" {
			return fmt.Errorf("invalid/duplicate conversion service declaration")
		}
		serviceSeen[service.Service] = true
		served, err := epathSQLDeclaredZones(frames, service.ServedZones)
		if err != nil {
			return err
		}
		if len(served) == 0 {
			return fmt.Errorf("service path requires explicit served Zone membership")
		}
		monthSite, monthAssigned, monthUnassigned := [12]epathSQLQuantity{}, [12]epathSQLQuantity{}, [12]epathSQLQuantity{}
		branchNumerator, branchDenominator := map[string][12]epathSQLQuantity{}, map[string][12]epathSQLQuantity{}
		for month := 1; month <= 12; month++ {
			consumption, err := epathSQLSiteSum(frames, service.SiteIDs, month)
			if err != nil {
				return err
			}
			monthSite[month-1] = consumption
			pathLoad, totalLoad := epathSQLQuantity{}, epathSQLQuantity{}
			for zone := range frames.Zones {
				value := frames.Loads[epathSQLKey(zone, service.Service, month)]
				totalLoad = totalLoad.add(value)
				if served[zone] {
					pathLoad = pathLoad.add(value)
				}
			}
			basis, load := service.Basis, pathLoad
			if pathLoad.Value == 0 {
				basis, load = service.FallbackBasis, totalLoad
			}
			if load.Value > 0 {
				monthAssigned[month-1] = consumption
			} else {
				monthUnassigned[month-1] = consumption
			}
			if load.Value > 0 && consumption.Value > 0 {
				num, den := branchNumerator[basis], branchDenominator[basis]
				num[month-1] = load
				den[month-1] = consumption
				branchNumerator[basis], branchDenominator[basis] = num, den
			}
			allExpected[month-1] = allExpected[month-1].add(consumption)
			allAssigned[month-1] = allAssigned[month-1].add(monthAssigned[month-1])
			allUnassigned[month-1] = allUnassigned[month-1].add(monthUnassigned[month-1])
		}
		for _, period := range periods {
			for _, basis := range []string{service.Basis, service.FallbackBasis} {
				num, den := epathSQLQuantity{}, epathSQLQuantity{}
				for _, month := range epathSQLPeriodMonths(period) {
					num = num.add(branchNumerator[basis][month-1])
					den = den.add(branchDenominator[basis][month-1])
				}
				ratio, err := epathSQLRatio(num, den)
				if err != nil {
					// Continue the first diagnostic through every other group, but
					// fail unconditionally until the presentation-pruning interval
					// policy is reviewed. Never relabel observed positive input zero.
					key := service.Service + "/" + period + "/" + basis
					checks.Unresolved = append(checks.Unresolved, epathSQLModelFailure{"ratios", key, "unresolved SQL precision interval " + key + ": " + err.Error()})
					ratio = &epathSQLQuantity{Value: num.Value / den.Value}
				}
				kind := service.RatioKind
				if basis == service.FallbackBasis {
					kind = service.FallbackRatioKind
				}
				target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: service.Service, Basis: basis, FromUnit: "kWh", ToUnit: "kWh", RatioKind: kind, Aggregate: "sum"}
				if err := checks.add("ratios", "building", "", period, service.Service+"/"+basis, "ratio", ratio, target, "", nil, nil); err != nil {
					return err
				}
			}
			id, err := epathSQLAllocationID(service.ReconciliationID, period)
			if err != nil {
				return err
			}
			for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
				q := epathSQLQuantity{}
				for _, month := range epathSQLPeriodMonths(period) {
					switch field {
					case "expectedValue":
						q = q.add(monthSite[month-1])
					case "allocatedValue":
						q = q.add(monthAssigned[month-1])
					case "unassignedValue":
						q = q.add(monthUnassigned[month-1])
					}
				}
				target := epathRealOracleTarget{Collection: "reconciliation", ID: id, Level: "allocation", Field: field, Unit: "kWh"}
				if err := checks.add("zoneAllocation", "building", "", period, service.Service+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
					return err
				}
			}
		}
	}
	if len(serviceSeen) != 2 {
		return fmt.Errorf("both declared conversion services are required")
	}
	for _, aux := range model.Auxiliaries {
		if aux.Weight != "cooling_plus_heating" && aux.Weight != "unassigned" {
			return fmt.Errorf("unsupported auxiliary weight requires an independent observation implementation")
		}
		served, err := epathSQLDeclaredZones(frames, aux.ServedZones)
		if err != nil {
			return err
		}
		if aux.Weight == "unassigned" && len(served) != 0 || aux.Weight != "unassigned" && len(served) == 0 {
			return fmt.Errorf("auxiliary applicability contradicts weight policy")
		}
		assigned, unassigned, expected := [12]epathSQLQuantity{}, [12]epathSQLQuantity{}, [12]epathSQLQuantity{}
		for month := 1; month <= 12; month++ {
			value, err := epathSQLSiteSum(frames, []string{aux.SiteID}, month)
			if err != nil {
				return err
			}
			expected[month-1] = value
			weight := 0.0
			for zone := range served {
				weight += frames.Loads[epathSQLKey(zone, "cooling", month)].Value + frames.Loads[epathSQLKey(zone, "heating", month)].Value
			}
			if aux.Weight != "unassigned" && weight > 0 {
				assigned[month-1] = value
			} else {
				unassigned[month-1] = value
			}
			allExpected[month-1] = allExpected[month-1].add(value)
			allAssigned[month-1] = allAssigned[month-1].add(assigned[month-1])
			allUnassigned[month-1] = allUnassigned[month-1].add(unassigned[month-1])
		}
		for _, period := range periods {
			id, err := epathSQLAllocationID(aux.ReconciliationID, period)
			if err != nil {
				return err
			}
			for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
				q := epathSQLQuantity{}
				for _, month := range epathSQLPeriodMonths(period) {
					switch field {
					case "expectedValue":
						q = q.add(expected[month-1])
					case "allocatedValue":
						q = q.add(assigned[month-1])
					case "unassignedValue":
						q = q.add(unassigned[month-1])
					}
				}
				target := epathRealOracleTarget{Collection: "reconciliation", ID: id, Level: "allocation", Field: field, Unit: "kWh", AllocationMethod: aux.AllocationMethod}
				if err := checks.add("zoneAllocation", "building", "", period, aux.SiteID+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
					return err
				}
			}
		}
	}
	for _, period := range periods {
		expected, assigned, unassigned := epathSQLQuantity{}, epathSQLQuantity{}, epathSQLQuantity{}
		for _, month := range epathSQLPeriodMonths(period) {
			expected = expected.add(allExpected[month-1])
			assigned = assigned.add(allAssigned[month-1])
			unassigned = unassigned.add(allUnassigned[month-1])
		}
		for _, field := range []string{"zoneAllocatedPct", "unassignedPct"} {
			num := assigned
			if field == "unassignedPct" {
				num = unassigned
			}
			ratio, err := epathSQLRatio(num, expected)
			if err != nil {
				return err
			}
			if ratio != nil {
				scaled := ratio.times(100)
				ratio = &scaled
			}
			if err := checks.add("zoneAllocation", "building", "", period, field, "%", ratio, epathRealOracleTarget{Collection: "quality", Field: field}, "", nil, nil); err != nil {
				return err
			}
		}
	}
	return nil
}
