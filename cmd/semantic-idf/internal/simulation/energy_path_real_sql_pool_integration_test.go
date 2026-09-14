package simulation

import (
	"fmt"
	"reflect"
	"strings"
)

func epathSQLModelPoolSourceChecks(frames epathSQLFrames, checks *epathSQLModelChecks) error {
	for _, native := range frames.PoolSystems {
		rows, err := epathSQLPoolSourceRequirements(native)
		if err != nil {
			return err
		}
		for _, row := range rows {
			q, proof := row.Quantity, row.Proof
			if err := checks.add(row.Group, row.Scope, row.Zone, row.Period, row.Key, row.Unit, &q, row.Target, "", nil, nil); err != nil {
				return err
			}
			checks.Rows[len(checks.Rows)-1].PoolSource = &proof
		}
	}
	return nil
}

func epathSQLCheckPoolPumpConsumer(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	if check.Allocation == nil || check.Allocation.NativePoolPump == nil {
		return fmt.Errorf("required Pool pump budget lost its independent native proof")
	}
	proof := check.Allocation.NativePoolPump
	fields, method, err := epathSQLPoolPumpLedgerFields(proof)
	if err != nil {
		return err
	}
	field := check.Item.Target.Field
	q, exists := fields[field]
	if !exists {
		return fmt.Errorf("Pool pump consumer changed its exact quantity")
	}
	q = q.positive()
	target := epathRealOracleTarget{Collection: "reconciliation", ID: proof.ID, Level: "allocation", Field: field, Unit: "kWh", AllocationMethod: method}
	want := epathSQLModelChecks{}
	key := "pool_native_pump/" + proof.Consumer.Auxiliary.SiteID + "/" + field
	if err := want.add("zoneAllocation", "building", "", proof.Period, key, "kWh", &q, target, "", nil, nil); err != nil {
		return err
	}
	canonical := want.Rows[0]
	if !reflect.DeepEqual(check.Item, canonical.Item) || !reflect.DeepEqual(check.Want, canonical.Want) || !reflect.DeepEqual(check.Quantity, canonical.Quantity) || check.OptionalPresentation || check.AnnualServiceAbsent {
		return fmt.Errorf("Pool pump consumer escaped its exact independently compiled budget")
	}
	// The required annual expected-value selector cannot pass without proving
	// all 13 actual Zone/Building budgets and their original source scope data.
	if proof.Period == "annual" && field == "expectedValue" {
		return epathSQLCheckPoolPumpJointBudget(bundle, proof.Consumer)
	}
	return nil
}

func epathSQLCheckPoolSourceConsumer(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof := check.PoolSource
	if proof == nil {
		return fmt.Errorf("required Pool native source lost its independently compiled proof")
	}
	q, err := epathSQLPoolSourceProofQuantity(*proof)
	if err != nil {
		return err
	}
	s := proof.Identity.Source
	key := fmt.Sprintf("pool_native_source/%s/%s/%s/%s", proof.Identity.Spec.ID, s.Name, s.ReportingFrequency, proof.Field)
	target := epathRealOracleTarget{Collection: "sources", Field: proof.Field, SourceName: s.Name, SourceKey: s.KeyValue, SourceUnit: s.SourceUnit, Frequency: s.ReportingFrequency, Unit: "kWh"}
	want := epathSQLModelChecks{}
	if err := want.add(proof.Identity.Spec.SourceGroup, "building", "", "annual", key, "kWh", &q, target, "", nil, nil); err != nil {
		return err
	}
	canonical := want.Rows[0]
	if !reflect.DeepEqual(check.Item, canonical.Item) || !reflect.DeepEqual(check.Want, canonical.Want) || !reflect.DeepEqual(check.Quantity, canonical.Quantity) || check.OptionalPresentation || check.AnnualServiceAbsent {
		return fmt.Errorf("Pool native source proof escaped its exact selector, context or native quantity")
	}
	return epathSQLCheckPoolSourceChart(bundle, proof)
}

func epathSQLBindPoolSources(observed epathRealOracleEvidence, model epathRealSQLModel, frames *epathSQLFrames) error {
	if len(model.PoolSystems) == 0 {
		return nil
	}
	if frames == nil || len(frames.PoolSystems) != 0 {
		return fmt.Errorf("Pool requires fresh independently compiled model frames")
	}
	originals, err := epathSQLValidatePoolOriginal(observed.originalText, model.PoolSystems)
	if err != nil {
		return err
	}
	if len(originals) != 1 {
		return fmt.Errorf("Pool original proof must bind one finite system")
	}
	native, err := epathCompileSQLPoolSourceFrames(observed, originals[0], model.Precision)
	if err != nil {
		return err
	}
	frames.PoolSystems = []epathSQLPoolSourceFrames{native}
	return nil
}

// A semantic nil is neither a missing SQL observation nor permission to skip
// an ordinary quantity. Validate its exact selector and every native boundary.
func epathSQLCheckPoolBoundaryConsumer(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p := check.PoolBoundary
	semantic := check.Want.Status == epathSQLPoolHeatingUnquantified
	if p == nil {
		if semantic {
			return fmt.Errorf("semantic Heating denial lost its required native proof")
		}
		return nil
	}
	if p.Scope != check.Item.Scope || !strings.EqualFold(p.Zone, check.Item.Zone) || p.Period != check.Item.Period || check.Want.Scope != check.Item.Scope || check.Want.Zone != check.Item.Zone || check.Want.Period != check.Item.Period || check.Want.Unit != check.Item.Unit || check.Want.Group != check.Item.Group {
		return fmt.Errorf("Pool boundary proof escaped its exact required consumer context")
	}
	if semantic {
		want := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: "heating", FromUnit: "kWh", ToUnit: "kWh", RatioKind: "load_to_site_energy", Aggregate: "sum"}
		if check.Item.Group != "ratios" || check.Item.Unit != "ratio" || check.Quantity != nil || check.Want.Value != nil || check.Conversion != nil || !reflect.DeepEqual(check.Item.Target, want) || check.AnnualServiceAbsent {
			return fmt.Errorf("Pool semantic denial was replaced by an unavailable or numeric conversion")
		}
	} else if check.Item.Group != "zoneAllocation" || check.Item.Unit != "kWh" || check.Quantity == nil || !check.Quantity.valid() || check.Want.Status != "" {
		return fmt.Errorf("Pool boundary cannot exempt a measured allocation quantity")
	}
	return epathSQLCheckPoolBoundary(bundle, p)
}
