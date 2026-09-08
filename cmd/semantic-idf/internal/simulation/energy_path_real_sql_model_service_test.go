package simulation

import (
	"fmt"
	"math"
	"strings"
)

type epathSQLConversionProof struct{ From, To epathSQLQuantity }

type epathSQLAllocationProof struct {
	Expected, Direct, Allocated, Unassigned *epathSQLQuantity
}

func (proof *epathSQLAllocationProof) fields() map[string]*epathSQLQuantity {
	return map[string]*epathSQLQuantity{"expectedValue": proof.Expected, "directValue": proof.Direct, "allocatedValue": proof.Allocated, "unassignedValue": proof.Unassigned}
}

// Bind all four independently computed quantities together. Zero crossing is
// a whole-row presentation possibility, never permission to repair one missing
// number or to turn an unknown SQL source into a zero-valued observation.
func (checks *epathSQLModelChecks) bindAllocationProof(start int) error {
	if start < 0 || len(checks.Rows)-start != 4 {
		return fmt.Errorf("allocation proof requires exactly four fields")
	}
	proof := &epathSQLAllocationProof{}
	first := checks.Rows[start].Item
	seen := map[string]bool{}
	for index := start; index < len(checks.Rows); index++ {
		check := &checks.Rows[index]
		if check.Quantity == nil || !check.Quantity.valid() || check.Quantity.Value < 0 || check.Item.Target.ID != first.Target.ID || check.Item.Scope != first.Scope || check.Item.Zone != first.Zone || check.Item.Period != first.Period || seen[check.Item.Target.Field] {
			return fmt.Errorf("unknown/contradictory allocation proof")
		}
		q := check.Quantity.positive()
		check.Quantity = &q
		seen[check.Item.Target.Field] = true
		switch check.Item.Target.Field {
		case "expectedValue":
			proof.Expected = &q
		case "directValue":
			proof.Direct = &q
		case "allocatedValue":
			proof.Allocated = &q
		case "unassignedValue":
			proof.Unassigned = &q
		default:
			return fmt.Errorf("invalid whole allocation field")
		}
		check.Allocation = proof
	}
	return nil
}

func epathSQLRatio(numerator, denominator epathSQLQuantity) (*epathSQLQuantity, error) {
	if !numerator.valid() || !denominator.valid() || numerator.Value < 0 || denominator.Value < 0 {
		return nil, fmt.Errorf("invalid ratio interval")
	}
	if denominator.Value <= 0 {
		return nil, nil
	}
	numLow, numHigh := numerator.bounds()
	denLow, denHigh := denominator.bounds()
	if denLow <= 0 {
		return nil, fmt.Errorf("uncertain positive ratio denominator")
	}
	value := numerator.Value / denominator.Value
	low := math.Max(0, numLow) / denHigh
	high := numHigh / denLow
	q := epathSQLBounded(value, math.Max(0, low), high)
	return &q, nil
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
		branchKindPairs := map[string][12]epathSQLConversionProof{}
		for month := 1; month <= 12; month++ {
			consumption, err := epathSQLSiteSum(frames, service.SiteIDs, month)
			if err != nil {
				return err
			}
			monthSite[month-1] = consumption
			pathLoad := epathSQLQuantity{}
			for zone := range frames.Zones {
				value := frames.Loads[epathSQLKey(zone, service.Service, month)]
				if served[zone] {
					pathLoad = pathLoad.add(value)
				}
			}
			basis, load := service.Basis, pathLoad
			// Explicit reviewed ServedZones means known topology. A zero load
			// on those paths does not authorize expanding the denominator to
			// passive plenums or other unserved Zones. The fallback selector
			// below remains an absence guard, not an invented allocation path.
			if load.Value > 0 {
				monthAssigned[month-1] = consumption
			} else {
				monthUnassigned[month-1] = consumption
			}
			if load.Value > 0 && consumption.Value > 0 {
				num, den := branchNumerator[basis], branchDenominator[basis]
				from, to := load.positive(), consumption.positive()
				kindPairs := branchKindPairs[basis]
				kindPairs[month-1] = epathSQLConversionProof{From: from, To: to}
				branchKindPairs[basis] = kindPairs
				if from.includesZero() || to.includesZero() {
					from, to = from.optionalPresentation(), to.optionalPresentation()
				}
				num[month-1] = from
				den[month-1] = to
				branchNumerator[basis], branchDenominator[basis] = num, den
			}
		}
		for _, period := range periods {
			for _, basis := range []string{service.Basis, service.FallbackBasis} {
				num, den := epathSQLQuantity{}, epathSQLQuantity{}
				for _, month := range epathSQLPeriodMonths(period) {
					num = num.add(branchNumerator[basis][month-1])
					den = den.add(branchDenominator[basis][month-1])
				}
				// Retain the exact SQL quotient in evidence. Candidate validation
				// uses independently bounded paired quantities, so a denominator
				// crossing presentation zero does not require an infinite tolerance.
				var ratio *epathSQLQuantity
				if den.Value > 0 && num.Value > 0 {
					ratio = &epathSQLQuantity{Value: num.Value / den.Value}
				}
				kind := service.RatioKind
				if basis == service.FallbackBasis {
					kind = service.FallbackRatioKind
				}
				kind, err = epathSQLConversionPeriodRatioKind(kind, period, branchKindPairs[basis])
				if err != nil {
					return fmt.Errorf("%s/%s/%s ratio kind: %w", service.Service, basis, period, err)
				}
				target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: service.Service, Basis: basis, FromUnit: "kWh", ToUnit: "kWh", RatioKind: kind, Aggregate: "sum"}
				if err := checks.add("ratios", "building", "", period, service.Service+"/"+basis, "ratio", ratio, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].Conversion = &epathSQLConversionProof{From: num, To: den}
			}
			id, err := epathSQLAllocationID(service.ReconciliationID, period)
			if err != nil {
				return err
			}
			allocationStart := len(checks.Rows)
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
			if err := checks.bindAllocationProof(allocationStart); err != nil {
				return err
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
			poolValue, err := epathSQLFanAllocatedMonth(*checks, model, aux, month, value)
			if err != nil {
				return err
			}
			weight := 0.0
			for zone := range served {
				weight += frames.Loads[epathSQLKey(zone, "cooling", month)].Value + frames.Loads[epathSQLKey(zone, "heating", month)].Value
			}
			if aux.Weight != "unassigned" && weight > 0 {
				assigned[month-1] = value
				if poolValue != nil {
					assigned[month-1] = *poolValue
				}
			} else {
				if poolValue != nil && poolValue.Value > 0 {
					return fmt.Errorf("audited positive fan allocation lost its served load")
				}
				unassigned[month-1] = value
			}
		}
		for _, period := range periods {
			id, err := epathSQLAllocationID(aux.ReconciliationID, period)
			if err != nil {
				return err
			}
			allocationStart := len(checks.Rows)
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
			if err := checks.bindAllocationProof(allocationStart); err != nil {
				return err
			}
		}
	}
	// Accounting percentages/status now belong to the all-context quality
	// proof, after the independent allocation rows have been validated.
	return nil
}
