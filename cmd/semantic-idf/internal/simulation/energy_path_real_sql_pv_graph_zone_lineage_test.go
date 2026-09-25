package simulation

// Finite Shop allowance: a Zone subtotal retains the global Facility budget as
// lineage, never as its observed amount. All scalar/source/endpoint authorities
// remain the independently compiled generic SQL consumers. No production helper
// or candidate quantity supplies an expected value here.
import (
	"fmt"
	"strings"
)

func epathSQLPVZoneBudgetKey(zone, period, id string) string {
	return strings.ToLower(zone) + "\x00" + period + "\x00" + id
}

func epathSQLPVZoneBudgetIDs(a, b []string) bool {
	seen := map[string]bool{}
	for _, id := range a {
		if id == "" || seen[id] {
			return false
		}
		seen[id] = true
	}
	if len(a) != len(b) {
		return false
	}
	for _, id := range b {
		if !seen[id] {
			return false
		}
		delete(seen, id)
	}
	return len(seen) == 0
}

// Demand must occur directly, once, at Monthly frequency. A derived wrapper,
// Hourly duplicate, CG input, component, transfer or supply identity is not an
// alternate way to acquire this permission, even with a zero numeric value.
func (w epathSQLPVGraphRoleWalk) zoneBudgetLineage(ids []string) bool {
	demand := 0
	for _, id := range ids {
		if p, ok := w.proof.Identities[id]; ok {
			if p.SpecID != "facility.demand" || p.Frequency != "Monthly" {
				return false
			}
			demand++
		} else if roles, err := w.roles([]string{id}); err != nil || len(roles) != 0 {
			return false
		}
	}
	return demand == 1 && len(ids) > 1 && epathSQLPVZoneBudgetIDs(ids, ids)
}

func (w epathSQLPVGraphRoleWalk) zoneBudgetNode(n EnergyExplanationNode, period string) (bool, error) {
	if !w.zoneBudgetLineage(n.SourceIDs) || n.Level != "carrier" || n.Carrier != "electricity" || n.EndUse != "total" || n.ZoneName == "" || n.ServiceKind != "" || n.PathType != "" || n.LoopName != "" || n.Unit != "kWh" || n.ScaleDomain != "site" || n.AggregationBasis != "model_total" || n.Period != period || !n.AllocationApplied || len(n.RelatedPathIDs) == 0 || n.AllocationExplanation == "" || (n.Basis != "direct_zone_energy" && n.Basis != "service_path_allocation") {
		return false, nil
	}
	if w.zoneBudgetChecks == nil {
		return false, fmt.Errorf("Zone Facility lineage requires independent subtotal and branch proofs")
	}
	fields, parts, fanFlows := map[string]bool{}, map[string]bool{}, map[string]bool{}
	allowed := map[string]bool{}
	for _, check := range w.zoneBudgetChecks.Rows {
		if check.Item.Scope != "zone" || !strings.EqualFold(check.Item.Zone, n.ZoneName) || check.Item.Period != period {
			continue
		}
		var err error
		if p := check.ZoneCarrier; p != nil && p.Carrier == "electricity" {
			field := check.Item.Target.Field
			if fields[field] || !w.zoneBudgetChecks.Keys[check.Item.Key] || check.Item.Key != check.Want.Key || p.Unavailable || p.Basis != n.Basis {
				return false, fmt.Errorf("Zone Facility lineage lost exact scalar proof roster")
			}
			fields[field] = true
			err = epathCheckSQLZoneCarrier(w.zoneBudgetBundle, check)
			if err == nil {
				actual := n.Value
				if field == "allocatedValue" {
					actual = n.AllocatedValue
				}
				err = epathCheckSQLModelQuantity(&actual, &p.Value) // also checks the duplicate annual graph
			}
		}
		// The finite Shop direct-use, two service, and fan consumers already
		// validate their source sets and branch endpoints. Require their roster;
		// a removed proof cannot turn a matching carrier scalar into acceptance.
		if check.Item.Target.Field == "value" {
			if p := check.DirectUse; p != nil && (p.EndUse == "lighting" || p.EndUse == "equipment") {
				parts[p.EndUse] = true
				err = epathCheckSQLDirectUseEndpoints(w.zoneBudgetBundle, check)
				for id := range p.Carriers["electricity"].Sources {
					allowed[id] = true
				}
			}
			if p := check.ZoneService; p != nil && (p.Service == "cooling" || p.Service == "heating") {
				parts[p.Service] = true
				err = epathCheckSQLZoneServiceEndpoints(w.zoneBudgetBundle, check)
				for id := range p.CarrierSources["electricity"] {
					allowed[id] = true
				}
				for id := range p.LoadSources {
					allowed[id] = true
				}
			}
			if strings.HasSuffix(check.Item.Key, "|fan_pools/value") && check.Item.Target.Level == "end_use" && check.Item.Target.Category == "fans" {
				parts["fans"] = true
				var actual *float64
				actual, err = epathReadOracleCandidate(w.zoneBudgetBundle, check.Item, check.Want)
				if err == nil {
					err = epathCheckSQLModelPresentation(w.zoneBudgetBundle, check, actual)
				}
			}
		}
		if p := check.SiteFlow; p != nil && p.EndUse == "fans" && p.Carrier == "electricity" {
			fanFlows[check.Item.Target.Relation+"/"+check.Item.Target.Field] = true
			err = epathCheckSQLSiteFlow(w.zoneBudgetBundle, check)
			for id := range p.NodeSources {
				allowed[id] = true
			}
		}
		if err != nil {
			return false, fmt.Errorf("Zone Facility lineage prerequisite: %w", err)
		}
	}
	if len(fields) != 2 || !fields["value"] || !fields["allocatedValue"] || len(parts) != 5 || len(fanFlows) != 4 {
		return false, fmt.Errorf("Zone Facility lineage lacks complete scalar/source/endpoint proof roster")
	}
	for _, id := range n.SourceIDs {
		if w.proof.Identities[id].SpecID == "facility.demand" {
			continue
		}
		if !allowed[id] {
			return false, fmt.Errorf("Zone Facility subtotal borrowed unproved source %s", id)
		}
	}
	w.zoneBudgetNodes[epathSQLPVZoneBudgetKey(n.ZoneName, period, n.ID)] = n
	return true, nil
}

func (w epathSQLPVGraphRoleWalk) zoneBudgetRow(row EnergyReconciliation, period string, nodes []EnergyExplanationNode) (bool, error) {
	if !w.zoneBudgetLineage(row.SourceIDs) || row.Level != "energy" || row.ZoneName == "" || row.Period != period || row.ServiceKind != "" || row.AllocationMethod != "" || row.AllocatedValue != 0 || row.Unit != "kWh" || row.Status != "partial" {
		return false, nil
	}
	var selected *EnergyExplanationNode
	for _, n := range nodes {
		if n.Carrier != "electricity" || n.Level != "carrier" || !strings.EqualFold(n.ZoneName, row.ZoneName) {
			continue
		}
		proven, ok := w.zoneBudgetNodes[epathSQLPVZoneBudgetKey(n.ZoneName, period, n.ID)]
		if !ok || selected != nil || row.Basis != proven.Basis || !epathSQLPVZoneBudgetIDs(row.SourceIDs, proven.SourceIDs) {
			return false, nil
		}
		selected = &proven
	}
	if selected == nil || w.zoneBudgetChecks == nil {
		return false, nil
	}
	fields := map[string]bool{}
	for _, check := range w.zoneBudgetChecks.Rows {
		p := check.Reconciliation
		if check.Item.Scope != "zone" || !strings.EqualFold(check.Item.Zone, row.ZoneName) || check.Item.Period != period || p == nil || !epathSQLReconciliationMatches(row, p) {
			continue
		}
		field := check.Item.Target.Field
		if fields[field] || !w.zoneBudgetChecks.Keys[check.Item.Key] || check.Item.Key != check.Want.Key {
			return false, fmt.Errorf("Zone Facility row lost exact proof roster")
		}
		fields[field] = true
		if err := epathCheckSQLModelReconciliation(w.zoneBudgetBundle, check); err != nil {
			return false, err
		}
		// The generic reader selects canonical annual; inspect this supplied
		// duplicate annual row too rather than trusting its sibling's amount.
		for field, actual := range map[string]float64{"expectedValue": row.ExpectedValue, "explainedValue": row.ExplainedValue, "residualValue": row.ResidualValue} {
			if err := epathCheckSQLModelQuantity(&actual, p.fields()[field]); err != nil {
				return false, err
			}
		}
	}
	if len(fields) != 3 || !fields["expectedValue"] || !fields["explainedValue"] || !fields["residualValue"] {
		return false, fmt.Errorf("Zone Facility row lacks independent whole-row proof")
	}
	return true, nil
}

func (w epathSQLPVGraphRoleWalk) zoneBudgetItem(item EnergyExplanationSummaryItem, collection string, context []string) bool {
	if collection != "carriers" || len(context) != 1 {
		return false
	}
	n, ok := w.zoneBudgetNodes[epathSQLPVZoneBudgetKey(item.ZoneName, context[0], item.ID)]
	// Generic coverage does not consume cached summary magnitudes. Bind the
	// displayed subtotal explicitly to the independently checked node. RawValue
	// is only clone consistency here, NOT independently proved magnitude: its
	// constructor retains parent raw demand plus merged direct raw contributions.
	return ok && item.Level == n.Level && item.Kind == n.Kind && item.ZoneName == n.ZoneName && item.ServiceKind == "" && item.PathType == "" && item.Carrier == n.Carrier && item.EndUse == n.EndUse && item.Unit == n.Unit && item.ScaleDomain == n.ScaleDomain && item.AggregationBasis == n.AggregationBasis && item.Basis == n.Basis && item.Value == n.Value && item.AllocatedValue == n.AllocatedValue && item.RawValue == n.RawValue && epathSQLPVZoneBudgetIDs(item.SourceIDs, n.SourceIDs)
}
