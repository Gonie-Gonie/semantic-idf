package simulation

// Independent acceptance support. This is a negative semantic consumer, not a source classifier,
// sanitizer, allocator or replacement for the independent site/load ledgers.
import (
	"fmt"
	"strings"
)

type epathSQLPoolBoundaryProof struct {
	Native              epathSQLPoolSourceFrames
	Scope, Zone, Period string
}

func epathSQLPoolBoundaryKey(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// SourceFrames are produced only after the original-text compiler recomputes
// the physical proof. Recheck its consumed census here; no candidate flag can
// establish the non-Zone demand, parent ownership or five served recipients.
func epathSQLPoolBoundaryOriginalZones(original epathSQLPoolOriginalProof) (map[string]bool, error) {
	zones := map[string]bool{}
	if len(original.OriginalSHA256) != 64 || strings.Trim(original.OriginalSHA256, "0123456789abcdef") != "" || original.SurfaceObjectIndex < 0 || len(original.Owners) != 6 || len(original.ServedZones) != 5 || len(original.Declaration.ServedZones) != 5 || len(original.Declaration.Terminals) != 5 || len(original.HotWaterDemands) != 8 || len(original.ChilledWaterDemands) != 2 {
		return nil, fmt.Errorf("Pool denial has no complete original system/recipient/demand proof")
	}
	declared := map[string]bool{}
	for _, name := range original.Declaration.ServedZones {
		key := epathSQLPoolBoundaryKey(name)
		if key == "" || key == "*" || declared[key] {
			return nil, fmt.Errorf("Pool denial has an ambiguous declared Zone roster")
		}
		declared[key] = true
	}
	for _, name := range original.ServedZones {
		key := epathSQLPoolBoundaryKey(name)
		if !declared[key] || zones[key] {
			return nil, fmt.Errorf("Pool denial changed its original served recipients")
		}
		zones[key] = true
	}
	poolZone, plenum := epathSQLPoolBoundaryKey(original.Declaration.ZoneName), epathSQLPoolBoundaryKey(original.Declaration.ReturnPlenumZoneName)
	if !zones[poolZone] || plenum == "" || plenum == "*" || zones[plenum] {
		return nil, fmt.Errorf("Pool denial lost its exact surface Zone or distinct return plenum")
	}
	zones[plenum] = true
	var poolOwner epathSQLPoolOriginalOwner
	poolOwners := 0
	for _, owner := range original.Owners {
		if owner.ObjectType == "SwimmingPool:Indoor" {
			poolOwner, poolOwners = owner, poolOwners+1
		}
	}
	if poolOwners != 1 || poolOwner.ObjectIndex < 0 || !strings.EqualFold(poolOwner.ObjectName, original.Declaration.PoolName) || epathSQLPoolBoundaryKey(poolOwner.ZoneName) != poolZone || !strings.EqualFold(poolOwner.PlantLoopName, original.Declaration.HotWaterLoopName) {
		return nil, fmt.Errorf("Pool denial has no exact typed native source owner")
	}
	poolCount, heatingCount := 0, 0
	seen := map[int]bool{}
	for _, demand := range original.HotWaterDemands {
		if demand.ObjectIndex < 0 || seen[demand.ObjectIndex] || !strings.EqualFold(demand.PlantLoopName, original.Declaration.HotWaterLoopName) {
			return nil, fmt.Errorf("Pool denial has a missing, duplicate or foreign HW demand")
		}
		seen[demand.ObjectIndex] = true
		switch demand.ObjectType {
		case "SwimmingPool:Indoor":
			if demand.ObjectIndex != poolOwner.ObjectIndex || demand.ObjectType != poolOwner.ObjectType || demand.ObjectName != poolOwner.ObjectName || demand.ZoneName != poolOwner.ZoneName || demand.PlantLoopName != poolOwner.PlantLoopName {
				return nil, fmt.Errorf("Pool denial substituted its exact original pool demand/source owner")
			}
			poolCount++
		case "Coil:Heating:Water":
			heatingCount++
		default:
			return nil, fmt.Errorf("Pool denial has an unreviewed HW demand")
		}
	}
	if poolCount != 1 || heatingCount != 7 {
		return nil, fmt.Errorf("Pool denial lost its pool plus seven heating-coil boundary")
	}
	for _, demand := range original.ChilledWaterDemands {
		if demand.ObjectType != "Coil:Cooling:Water" || demand.ObjectIndex < 0 || seen[demand.ObjectIndex] || !strings.EqualFold(demand.PlantLoopName, original.Declaration.ChilledWaterLoopName) {
			return nil, fmt.Errorf("Pool denial has a missing, duplicate or foreign CW demand")
		}
		seen[demand.ObjectIndex] = true
	}
	return zones, nil
}

// Exact canonical tokens, not UI labels or a substring such as "heat". Both
// native endpoint identities and explicit service fields count: contradictory
// candidate metadata cannot re-label a heating endpoint as cooling.
func epathSQLPoolBoundaryHeatingID(id string) bool {
	key := epathSQLPoolBoundaryKey(id)
	for _, prefix := range []string{"load.heating", "end_use.heating", "energy.heating"} {
		if key == prefix || strings.HasPrefix(key, prefix+".") {
			return true
		}
	}
	return false
}

func epathSQLPoolBoundaryHeatingNode(node EnergyExplanationNode) bool {
	return epathSQLPoolBoundaryKey(node.ServiceKind) == "heating" || epathSQLPoolBoundaryKey(node.EndUse) == "heating" || epathSQLPoolBoundaryHeatingID(node.ID) || epathSQLPoolBoundaryHeatingID(node.Kind)
}

func epathSQLPoolBoundarySummary(summary EnergyExplanationSummary, nodes []EnergyExplanationNode, label string) error {
	heating := map[string]bool{}
	for _, node := range nodes {
		if epathSQLPoolBoundaryHeatingNode(node) {
			heating[node.ID] = true
		}
	}
	for _, items := range [][]EnergyExplanationSummaryItem{summary.Ratios, summary.DerivedKPIs} {
		for _, item := range items {
			denied := epathSQLPoolBoundaryKey(item.ServiceKind) == "heating" || epathSQLPoolBoundaryKey(item.EndUse) == "heating" || heating[item.DenominatorLabel] || heating[item.NumeratorLabel] || epathSQLPoolBoundaryHeatingID(item.DenominatorLabel) || epathSQLPoolBoundaryHeatingID(item.NumeratorLabel)
			for _, key := range []string{item.ID, item.Kind} {
				denied = denied || epathSQLPoolBoundaryKey(key) == "kpi.heating_cop" || epathSQLPoolBoundaryHeatingID(key)
			}
			if denied {
				return fmt.Errorf("Pool Heating ratio is semantically unproved in %s: %s", label, item.ID)
			}
		}
	}
	return nil
}

func epathSQLPoolBoundaryGraph(nodes []EnergyExplanationNode, links []EnergyPathLink, label string) error {
	heating := map[string]bool{}
	for _, node := range nodes {
		if epathSQLPoolBoundaryHeatingNode(node) {
			heating[node.ID] = true
		}
	}
	for _, link := range links {
		if epathSQLPoolBoundaryKey(link.Relation) != "load_to_end_use" {
			continue
		}
		if epathSQLPoolBoundaryKey(link.ServiceKind) == "heating" || heating[link.FromID] || heating[link.ToID] || epathSQLPoolBoundaryHeatingID(link.FromID) || epathSQLPoolBoundaryHeatingID(link.ToID) {
			// Includes zero-valued, no-ratio, missing-source, direct/allocation and
			// contradictory-kind links. None can establish a process/Zone split.
			return fmt.Errorf("Pool Heating conversion is semantically unproved in %s: %s", label, link.ID)
		}
	}
	return nil
}

func epathSQLCheckPoolBoundary(bundle PurposeResultBundle, proof *epathSQLPoolBoundaryProof) error {
	if proof == nil || (proof.Scope != "building" && proof.Scope != "zone") || (proof.Scope == "building" && proof.Zone != "") || (proof.Scope == "zone" && strings.TrimSpace(proof.Zone) == "") || !epathOracleValidPeriod(proof.Period) {
		return fmt.Errorf("Pool denial requires one exact original scope/Zone/period proof")
	}
	zones, err := epathSQLPoolBoundaryOriginalZones(proof.Native.Original)
	if err != nil {
		return err
	}
	if proof.Scope == "zone" && !zones[epathSQLPoolBoundaryKey(proof.Zone)] {
		return fmt.Errorf("Pool denial selected a foreign original Zone")
	}
	if err := epathSQLValidatePoolSourceFrames(proof.Native); err != nil {
		return fmt.Errorf("Pool denial lost its independent native parent/source proof: %w", err)
	}
	nodes, links, _, _, err := epathOracleGraph(bundle, proof.Scope, proof.Zone, proof.Period)
	if err != nil {
		return err
	}
	if err := epathSQLPoolBoundaryGraph(nodes, links, "selected "+proof.Scope+"/"+proof.Zone+"/"+proof.Period); err != nil {
		return err
	}

	result := bundle.EnergyExplanation
	periods := result.Periods
	var scopeSummary *EnergyExplanationSummary
	if proof.Scope == "zone" {
		for index := range result.ZoneResults {
			zone := &result.ZoneResults[index]
			if strings.EqualFold(zone.Scope.ZoneName, proof.Zone) {
				periods, scopeSummary = zone.Periods, &zone.Summary
			}
		}
	}
	// Inspect cached wrapper summaries only in their own scope/period, never
	// selecting the Building cache as a substitute for an absent Zone cache.
	checkCache := func(summary EnergyExplanationSummary, label string, ownScope bool) error {
		if len(summary.Ratios)+len(summary.DerivedKPIs) == 0 {
			return nil
		}
		if summary.Scope.Kind != "building" && summary.Scope.Kind != "zone" || summary.Scope.Kind == "building" && summary.Scope.ZoneName != "" || summary.Scope.Kind == "zone" && !zones[epathSQLPoolBoundaryKey(summary.Scope.ZoneName)] {
			return fmt.Errorf("Pool cached summary has an invalid original scope in %s", label)
		}
		if summary.Scope.Kind != proof.Scope || (proof.Scope == "building" && summary.Scope.ZoneName != "") || (proof.Scope == "zone" && !strings.EqualFold(summary.Scope.ZoneName, proof.Zone)) {
			if ownScope {
				return fmt.Errorf("Pool cached summary crossed its original scope in %s", label)
			}
			return nil
		}
		period := summary.Period
		if period == "" {
			period = "annual"
		}
		if !epathOracleValidPeriod(period) {
			return fmt.Errorf("Pool cached summary has an invalid original period in %s", label)
		}
		if period != proof.Period {
			return nil
		}
		return epathSQLPoolBoundarySummary(summary, nodes, label)
	}
	if err := checkCache(bundle.EnergyExplanationSummary, "bundle cached summary", false); err != nil {
		return err
	}
	if scopeSummary != nil {
		if err := checkCache(*scopeSummary, "Zone cached summary", true); err != nil {
			return err
		}
	}

	selectedCount := 0
	for _, period := range periods {
		if period.ID != proof.Period {
			continue
		}
		selectedCount++
		wantKind := "monthly"
		if proof.Period == "annual" {
			wantKind = "annual"
		}
		if selectedCount > 1 || period.Kind != wantKind {
			return fmt.Errorf("Pool denial found duplicate or conflicting selected period alias")
		}
		known := make([]string, 0, len(zones))
		for name := range zones {
			known = append(known, name)
		}
		if err := epathValidateOracleGraphRecords(period.Nodes, period.Links, period.Reconciliation, proof.Scope, proof.Zone, proof.Period, known...); err != nil {
			return err
		}
		if err := epathSQLPoolBoundaryGraph(period.Nodes, period.Links, "selected period/annual alias"); err != nil {
			return err
		}
		if period.Summary != nil {
			summary := *period.Summary
			if len(summary.Ratios)+len(summary.DerivedKPIs) > 0 {
				if summary.Scope.Kind != proof.Scope || (proof.Scope == "building" && summary.Scope.ZoneName != "") || (proof.Scope == "zone" && !strings.EqualFold(summary.Scope.ZoneName, proof.Zone)) || summary.Period != "" && summary.Period != proof.Period {
					return fmt.Errorf("Pool period cache changed its original scope/period")
				}
				if err := epathSQLPoolBoundarySummary(summary, period.Nodes, "selected period cached summary"); err != nil {
					return err
				}
			}
		}
	}
	// Annual uses the canonical root plus an optional explicit annual alias;
	// monthly selection has no root/annual fallback, even if every value is 0.
	if proof.Period != "annual" && selectedCount != 1 {
		return fmt.Errorf("Pool denial has no exact monthly graph")
	}
	return nil
}
