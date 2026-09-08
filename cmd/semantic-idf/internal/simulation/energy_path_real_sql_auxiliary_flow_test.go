package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// A plant-loop load share is not direct equipment metering. The historical
// direct_end_use_to_carrier relation identifies an unpaired site-domain flow;
// its service_path_allocation basis and original broad meter remain explicit.
func epathSQLModelAuxiliaryFlowChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	if checks == nil {
		return fmt.Errorf("auxiliary flow requires the independently compiled scalar ledger")
	}
	expected, err := epathSQLAuxiliaryZoneProofs(frames, model)
	if err != nil {
		return err
	}
	seen := map[string]map[string]bool{}
	for _, check := range checks.Rows {
		p := check.AuxiliaryZone
		if p == nil {
			continue
		}
		key := epathSQLAuxiliaryZoneKey(p.SiteID, p.ZoneName, p.Period)
		want := expected[key]
		if !epathSQLAuxiliaryZoneCheckMatches(check, want) {
			return fmt.Errorf("auxiliary flow has an unbound or changed independent scalar proof: %s", key)
		}
		field := check.Item.Target.Field
		target := epathSQLNodeTarget("end_use", p.EndUse, "", "site")
		target.Field, target.Basis, target.AggregationBasis, target.Aggregate, target.AllowPrunedZero = field, p.Basis, "model_total", "", true
		identity := strings.Join([]string{"zoneAllocation", "zone", strings.ToLower(p.ZoneName), p.Period, "auxiliary/" + p.SiteID + "/" + field}, "|")
		if !reflect.DeepEqual(check.Item.Target, target) || check.Item.Group != "zoneAllocation" || check.Item.Key != identity ||
			check.Want.Key != identity || check.Want.Group != "zoneAllocation" || check.Want.Scope != "zone" || check.Want.Zone != p.ZoneName || check.Want.Period != p.Period || check.Want.Unit != "kWh" || check.Want.Status != "" {
			return fmt.Errorf("auxiliary flow scalar selector contradicts its exact compiled context")
		}
		if seen[key] == nil {
			seen[key] = map[string]bool{}
		}
		if seen[key][field] {
			return fmt.Errorf("duplicate independently compiled auxiliary scalar %s/%s", key, field)
		}
		seen[key][field] = true
	}
	keys := make([]string, 0, len(expected))
	for key, p := range expected {
		if len(seen[key]) != 2 || !seen[key]["value"] || !seen[key]["allocatedValue"] {
			return fmt.Errorf("auxiliary flow requires both independent scalar fields for %s", key)
		}
		if p.Period == "annual" {
			sum := epathSQLQuantity{}
			for month := 1; month <= 12; month++ {
				part := expected[epathSQLAuxiliaryZoneKey(p.SiteID, p.ZoneName, fmt.Sprintf("M%d", month))]
				if part == nil || !part.Value.valid() || part.Value.Value < 0 {
					return fmt.Errorf("annual auxiliary flow lacks twelve completed monthly shares")
				}
				sum = sum.add(part.Value)
			}
			if !epathSQLZoneCarrierQuantityEqual(sum, p.Value) {
				return fmt.Errorf("annual auxiliary flow is not the sum of completed monthly quantities and bounds")
			}
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		original := expected[key]
		p := &epathSQLSiteFlowProof{
			EndUse: original.EndUse, Carrier: original.Carrier, Basis: original.Basis,
			Total: original.Value, NodeTotal: original.Value, MonthlyExclusive: true,
			Relations: map[string]epathSQLQuantity{"direct_end_use_to_carrier": original.Value, "end_use_to_carrier": {}},
			Sources:   original.Sources, NodeSources: original.NodeSources,
			Required:        map[string]map[string]bool{"direct_end_use_to_carrier": original.Required, "end_use_to_carrier": {}, "node": original.NodeRequired},
			AllowedCarriers: map[string]bool{original.Carrier: true},
		}
		for _, relation := range []string{"direct_end_use_to_carrier", "end_use_to_carrier"} {
			for _, field := range []string{"fromValue", "toValue"} {
				q := p.Relations[relation]
				target := epathRealOracleTarget{Collection: "links", Field: field, Relation: relation, Basis: p.Basis, FromUnit: "kWh", ToUnit: "kWh", Aggregate: "sum"}
				if err := checks.add("zoneAllocation", "zone", original.ZoneName, original.Period, "auxiliary_flow/"+original.SiteID+"/"+relation+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].SiteFlow = p
			}
		}
	}
	return nil
}
