package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Availability is compiled from the executed requests and observed SQL, while
// ratio availability depends on independently validated PRESENT graph pairs.
// Dependencies contain no quality checks, so quality never proves itself.
type epathSQLQualityProof struct {
	Field        string
	RunLevel     EnergyCompletenessLevel
	LoadStatus   string
	Dependencies []epathSQLModelCheck
	Originals    map[int]epathRealSQLSource
	LoadSources  map[string][]int
	SiteSources  map[string]map[string][]int
	SiteValues   map[string]map[string]epathSQLQuantity
}

func epathSQLModelQualityChecks(observed epathRealOracleEvidence, frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	var seed epathSQLModelChecks
	if err := epathSQLModelAvailabilityChecks(observed.Sources, observed.outputPlan, model, &seed); err != nil {
		return err
	}
	levels := map[string]EnergyCompletenessLevel{}
	for _, check := range seed.Rows {
		if check.Want.Found == nil || check.Want.Total == nil {
			return fmt.Errorf("availability seed lacks independent counts")
		}
		levels[check.Item.Target.Field] = EnergyCompletenessLevel{Status: check.Want.Status, Found: *check.Want.Found, Total: *check.Want.Total}
	}
	zones := []string{""}
	for key, zone := range frames.Zones {
		if key != strings.ToLower(zone.Name) || zone.Name == "" {
			return fmt.Errorf("quality requires exact observed Zone identities")
		}
		zones = append(zones, zone.Name)
	}
	sort.Strings(zones)
	prior := append([]epathSQLModelCheck(nil), checks.Rows...)
	for _, zone := range zones {
		scope := "building"
		if zone != "" {
			scope = "zone"
		}
		for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
			for _, field := range []string{"drivers", "loads", "endUses", "carriers", "ratios", "driverToLoadClosedPct", "endUseToCarrierClosedPct", "zoneAllocatedPct", "unassignedPct"} {
				proof := &epathSQLQualityProof{Field: field, RunLevel: levels[field], LoadStatus: levels["loads"].Status}
				if epathSQLQualityAccounting(field) {
					group := "zoneAllocation"
					if field == "driverToLoadClosedPct" || field == "endUseToCarrierClosedPct" {
						group = "completeness"
						proof.RunLevel = levels["loads"]
						if field == "endUseToCarrierClosedPct" {
							proof.RunLevel = levels["carriers"]
						}
					}
					proof.Dependencies = epathSQLQualityAccountingDependencies(prior, field, scope, zone, period)
					if err := checks.add(group, scope, zone, period, field, "%", nil, epathRealOracleTarget{Collection: "quality", Field: field}, "unavailable", nil, nil); err != nil {
						return err
					}
					checks.Rows[len(checks.Rows)-1].Quality = proof
					continue
				}
				if field == "ratios" {
					proof.Originals = frames.SourceIdentities
					proof.LoadSources, proof.SiteSources, proof.SiteValues = map[string][]int{}, map[string]map[string][]int{}, map[string]map[string]epathSQLQuantity{}
					for _, check := range prior {
						if check.Item.Scope != scope || !strings.EqualFold(check.Item.Zone, zone) || check.Item.Period != period || check.Item.Target.Collection == "quality" {
							continue
						}
						target := check.Item.Target
						if target.Collection == "nodes" && target.Field == "value" && (target.Level == "load" || target.Level == "end_use" || target.Level == "carrier") || check.Conversion != nil || check.SiteFlow != nil && target.Field == "fromValue" && target.Relation == "end_use_to_carrier" && (check.SiteFlow.EndUse == "cooling" || check.SiteFlow.EndUse == "heating") {
							proof.Dependencies = append(proof.Dependencies, check)
						}
					}
					for _, service := range []string{"cooling", "heating"} {
						proof.SiteSources[service], proof.SiteValues[service] = map[string][]int{}, map[string]epathSQLQuantity{}
						for current := range frames.Zones {
							if zone != "" && !strings.EqualFold(current, zone) {
								continue
							}
							for _, month := range epathSQLPeriodMonths(period) {
								ids := frames.LoadSourceIDs[epathSQLKey(current, service, month)]
								if err := epathSQLDriverOriginalIDs(frames.SourceIdentities, ids); err != nil {
									return err
								}
								proof.LoadSources[service] = epathSQLDictionaryUnion(proof.LoadSources[service], ids)
							}
						}
						for _, site := range model.Site {
							if site.Facility || site.EndUse != service {
								continue
							}
							ids := frames.SiteSources[site.ID]
							if err := epathSQLDriverOriginalIDs(frames.SourceIdentities, ids); err != nil {
								return err
							}
							proof.SiteSources[service][site.Carrier] = epathSQLDictionaryUnion(proof.SiteSources[service][site.Carrier], ids)
							if scope == "building" {
								for _, month := range epathSQLPeriodMonths(period) {
									values := frames.Site[site.ID]
									if len(values) != 12 || values[month-1] == nil || !values[month-1].valid() || values[month-1].Value < 0 {
										return fmt.Errorf("quality has unknown site observations for %s/%s", site.ID, period)
									}
									proof.SiteValues[service][site.Carrier] = proof.SiteValues[service][site.Carrier].add(*values[month-1])
								}
							}
						}
						if scope == "zone" {
							for _, dependency := range proof.Dependencies {
								if dependency.ZoneService != nil && dependency.ZoneService.Service == service {
									for carrier, q := range dependency.ZoneService.Carriers {
										proof.SiteValues[service][carrier] = q
									}
								}
							}
						}
					}
				}
				group, status := "completeness", proof.RunLevel.Status
				found, total := proof.RunLevel.Found, proof.RunLevel.Total
				var q *epathSQLQuantity
				if field == "ratios" {
					group, status, found, total = "ratios", "unavailable", 0, 0
				} else if scope == "zone" && (field == "endUses" || field == "carriers") {
					// Run-level observed site inventory establishes presence, never
					// a Zone requested-output denominator or a measured Zone share.
					if found > 0 {
						status = "partial"
					}
					found, total = 0, 0
				} else if total > 0 {
					q = &epathSQLQuantity{Value: float64(found)}
				}
				if err := checks.add(group, scope, zone, period, field, "count", q, epathRealOracleTarget{Collection: "quality", Field: field}, status, &found, &total); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].Quality = proof
			}
		}
	}
	return nil
}

func epathCheckSQLModelQuality(bundle PurposeResultBundle, check epathSQLModelCheck) (epathRealOracleMetric, error) {
	want := check.Want
	proof := check.Quality
	if proof == nil || proof.Field != check.Item.Target.Field || check.Item.Target.Collection != "quality" || (!epathSQLQualityAccounting(proof.Field) && check.Item.Unit != "count") || (epathSQLQualityAccounting(proof.Field) && check.Item.Unit != "%") {
		return want, fmt.Errorf("invalid independent quality proof")
	}
	nodes, links, rows, quality, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
	if err != nil {
		return want, err
	}
	if quality == nil {
		return want, fmt.Errorf("missing scope-period quality record")
	}
	if epathSQLQualityAccounting(proof.Field) {
		if proof.Field == "zoneAllocatedPct" || proof.Field == "unassignedPct" {
			want.Value, want.Status, err = epathSQLQualityAllocation(bundle, check)
		} else {
			want.Value, want.Status, err = epathSQLQualityClosure(bundle, check, nodes, links)
		}
		if err != nil {
			return want, err
		}
		// Zone allocation is the Building-wide observation, even in a Zone
		// whose local reconciliation does not repeat any allocation rows.
		readNodes, readRows := nodes, rows
		if proof.Field == "zoneAllocatedPct" || proof.Field == "unassignedPct" {
			readNodes, _, readRows, _, err = epathOracleGraph(bundle, "building", "", check.Item.Period)
			if err != nil {
				return want, err
			}
		}
		actual, err := epathReadStrictOracleQuality(readNodes, readRows, quality, check.Item.Target, want)
		if err != nil {
			return want, err
		}
		return want, epathCompareOracleNumber(actual, want.Value, .00000001, 0)
	}
	if proof.Field == "ratios" {
		found, total, status, err := epathSQLQualityRatioCounts(bundle, check, nodes, links)
		if err != nil {
			return want, err
		}
		want.Found, want.Total, want.Status, want.Value = &found, &total, status, nil
		if total > 0 && status != "not_requested" && status != "not_applicable" {
			want.Value = epathOracleNumber(float64(found))
		}
	}
	levels := map[string]EnergyCompletenessLevel{"drivers": quality.Drivers, "loads": quality.Loads, "endUses": quality.EndUses, "carriers": quality.Carriers, "ratios": quality.Ratios}
	names := map[string]string{"drivers": "driver", "loads": "load", "endUses": "end_use", "carriers": "carrier", "ratios": "ratio"}
	level, ok := levels[proof.Field]
	if !ok || level.Level != names[proof.Field] {
		return want, fmt.Errorf("quality level does not match its declared field")
	}
	actual, err := epathReadStrictOracleQuality(nodes, rows, quality, check.Item.Target, want)
	if err != nil {
		return want, err
	}
	return want, epathCompareOracleNumber(actual, want.Value, 0, 0)
}

func epathSQLQualityAccounting(field string) bool {
	return field == "driverToLoadClosedPct" || field == "endUseToCarrierClosedPct" || field == "zoneAllocatedPct" || field == "unassignedPct"
}

func epathSQLQualityAccountingDependencies(prior []epathSQLModelCheck, field, scope, zone, period string) []epathSQLModelCheck {
	out := []epathSQLModelCheck{}
	allocation := field == "zoneAllocatedPct" || field == "unassignedPct"
	if allocation {
		scope, zone = "building", ""
	}
	for _, check := range prior {
		if check.Quality != nil || check.Item.Target.Collection == "quality" || check.Item.Scope != scope || check.Item.Zone != zone || check.Item.Period != period {
			continue
		}
		target := check.Item.Target
		if allocation {
			if check.Allocation != nil && target.Field == "expectedValue" {
				out = append(out, check)
			}
		} else if field == "driverToLoadClosedPct" {
			if target.Collection == "nodes" && target.Field == "value" && (target.Level == "load" || target.Level == "driver") || check.DriverLink != nil && target.Field == "fromValue" {
				out = append(out, check)
			}
		} else if target.Collection == "nodes" && target.Field == "value" && (target.Level == "carrier" || target.Level == "end_use") || check.SiteFlow != nil && target.Field == "fromValue" && target.Relation == "end_use_to_carrier" {
			out = append(out, check)
		}
	}
	return out
}

// Dependencies prove the SQL quantities first. The calculation below may then
// use their actual rounded presentation, never candidate quality or row status.
func epathSQLQualityCheckDependency(bundle PurposeResultBundle, dependency epathSQLModelCheck) error {
	if dependency.Quality != nil || dependency.Item.Target.Collection == "quality" {
		return fmt.Errorf("quality cannot depend on another quality result")
	}
	var err error
	switch {
	case dependency.Allocation != nil:
		err = epathCheckSQLModelAllocation(bundle, dependency)
	case dependency.DriverLink != nil:
		err = epathCheckSQLModelDriverLink(bundle, dependency)
	case dependency.SiteFlow != nil:
		err = epathCheckSQLSiteFlow(bundle, dependency)
	case dependency.ZoneCarrier != nil:
		err = epathCheckSQLZoneCarrier(bundle, dependency)
	default:
		var actual *float64
		actual, err = epathReadOracleCandidate(bundle, dependency.Item, dependency.Want)
		if err == nil {
			err = epathCheckSQLModelPresentation(bundle, dependency, actual)
		}
	}
	if err == nil && dependency.ZoneService != nil {
		err = epathCheckSQLZoneServiceEndpoints(bundle, dependency)
	}
	if err == nil && dependency.DirectUse != nil {
		err = epathCheckSQLDirectUseEndpoints(bundle, dependency)
	}
	return err
}

func epathSQLQualityClosure(bundle PurposeResultBundle, check epathSQLModelCheck, nodes []EnergyExplanationNode, links []EnergyPathLink) (*float64, string, error) {
	p := check.Quality
	fail := func(message string) (*float64, string, error) {
		return nil, "", fmt.Errorf("independent closure: %s", message)
	}
	if len(p.Dependencies) == 0 {
		return fail("no independently observed graph roster")
	}
	for _, dependency := range p.Dependencies {
		if dependency.Item.Scope != check.Item.Scope || dependency.Item.Zone != check.Item.Zone || dependency.Item.Period != check.Item.Period {
			return fail("prerequisite outside selected scope/period")
		}
		if dependency.Item.Target.Collection == "nodes" && (dependency.Quantity == nil || !dependency.Quantity.valid() || dependency.Quantity.Value < 0) {
			return fail("unknown node observation is not evidence for pruned zero")
		}
		if err := epathSQLQualityCheckDependency(bundle, dependency); err != nil {
			return nil, "", fmt.Errorf("closure prerequisite %s: %w", dependency.Want.Key, err)
		}
	}
	thermal := p.Field == "driverToLoadClosedPct"
	level, domain := "carrier", "site"
	if thermal {
		level, domain = "load", "thermal"
	}
	byID, expected, incoming := map[string]EnergyExplanationNode{}, map[string]float64{}, map[string]float64{}
	partialSubtotal := false
	canonical := map[string]bool{}
	for _, node := range nodes {
		byID[node.ID] = node
		if node.Level != level {
			continue
		}
		covered := false
		for _, dependency := range p.Dependencies {
			target := dependency.Item.Target
			covered = covered || target.Collection == "nodes" && target.Field == "value" && epathOracleNodeMatches(node, target)
		}
		if !covered || node.ScaleDomain != domain || node.Unit != "kWh" || !epathOracleFinite(node.Value) || node.Value < 0 {
			return fail("endpoint lacks a valid independent quantity/domain proof")
		}
		key := node.Carrier
		if thermal {
			key = node.ServiceKind
			if key != "cooling" && key != "heating" {
				return fail("load lacks exact thermal service")
			}
		} else {
			if node.ZoneName != "" || node.Basis != "reported_meter" && node.Basis != "reported_variable" && node.Basis != "integrated_rate" || node.MeterHierarchyLevel == "observed_end_use_subtotal" || node.MeterHierarchyLevel == "zone_direct_subtotal" {
				partialSubtotal = true
				continue
			}
		}
		if key == "" || canonical[key] {
			return fail("ambiguous canonical closure denominator")
		}
		canonical[key] = true
		expected[node.ID] = node.Value
	}
	for _, link := range links {
		relevant := thermal && link.Relation == "driver_to_load" || !thermal && (link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier")
		if !relevant {
			continue // A drawn residual is not independently explained energy.
		}
		from, fromOK := byID[link.FromID]
		to, toOK := byID[link.ToID]
		if !fromOK || !toOK || to.Level != level || from.ScaleDomain != domain || to.ScaleDomain != domain || from.Unit != "kWh" || to.Unit != "kWh" || link.FromUnit != "kWh" || link.ToUnit != "kWh" || !epathOracleFinite(link.FromValue) || !epathOracleFinite(link.ToValue) || link.FromValue < 0 || link.ToValue < 0 || math.Abs(link.FromValue-link.ToValue) > 1e-9 || link.Ratio != 0 || link.RatioKind != "" {
			return fail("malformed same-domain physical branch")
		}
		if thermal && (from.Level != "driver" || from.ServiceKind != to.ServiceKind || link.ServiceKind != to.ServiceKind || from.ZoneName != to.ZoneName) || !thermal && from.Level != "end_use" {
			return fail("physical branch contradicts endpoint identity")
		}
		if _, eligible := expected[to.ID]; !eligible {
			continue // Verified Zone subtotals have no facility denominator.
		}
		covered := false
		for _, dependency := range p.Dependencies {
			covered = covered || thermal && dependency.DriverLink != nil && epathSQLDriverLinkMatches(byID, link, dependency.DriverLink) || !thermal && dependency.SiteFlow != nil && epathSQLSiteFlowMatches(byID, link, dependency.SiteFlow)
		}
		if !covered {
			return fail("physical branch lacks independent endpoint/source proof")
		}
		incoming[to.ID] += link.ToValue
	}
	total, errorSum, overmapped := 0.0, 0.0, false
	keys := []string{}
	for id := range expected {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		value := expected[id]
		total += value
		errorSum += math.Abs(value - incoming[id])
		overmapped = overmapped || incoming[id] > value+1e-6*math.Max(1, value)
	}
	if total == 0 {
		status := "unavailable"
		if partialSubtotal {
			status = "partial"
		} else if p.RunLevel.Status == "not_requested" || p.RunLevel.Status == "not_applicable" {
			status = p.RunLevel.Status
		}
		return nil, status, nil
	}
	status := "complete"
	if partialSubtotal || errorSum > 1e-6*math.Max(1, total) {
		status = "partial"
	}
	if overmapped {
		status = "overmapped"
	}
	return epathOracleNumber(math.Round(100*math.Max(0, 1-errorSum/total)*1000) / 1000), status, nil
}

func epathSQLQualityAllocation(bundle PurposeResultBundle, check epathSQLModelCheck) (*float64, string, error) {
	_, _, rows, _, err := epathOracleGraph(bundle, "building", "", check.Item.Period)
	if err != nil {
		return nil, "", err
	}
	declared := map[string]epathSQLModelCheck{}
	for _, dependency := range check.Quality.Dependencies {
		id := dependency.Item.Target.ID
		if dependency.Allocation == nil || dependency.Item.Scope != "building" || dependency.Item.Zone != "" || dependency.Item.Period != check.Item.Period || id == "" || dependency.Item.Target.Field != "expectedValue" || declared[id].Allocation != nil {
			return nil, "", fmt.Errorf("allocation quality requires unique exact Building-period proofs")
		}
		if err := epathSQLQualityCheckDependency(bundle, dependency); err != nil {
			return nil, "", fmt.Errorf("allocation quality prerequisite %s: %w", id, err)
		}
		declared[id] = dependency
	}
	expected, assigned, unassigned, status := 0.0, 0.0, 0.0, "complete"
	for _, row := range rows {
		if row.Level != "allocation" {
			continue
		}
		if declared[row.ID].Allocation == nil || row.Period != check.Item.Period || row.Unit != "kWh" {
			return nil, "", fmt.Errorf("unproved allocation row would change the quality denominator")
		}
		expected += row.ExpectedValue
		assigned += row.DirectValue + row.AllocatedValue
		unassigned += row.UnassignedValue
		// Status is independently recomputed from the four proven displayed
		// quantities, not copied from row.Status or quality.ZoneAllocationStatus.
		if row.DirectValue+row.AllocatedValue > row.ExpectedValue+1e-6 {
			status = "overmapped"
		} else if status != "overmapped" && (row.UnassignedValue > 1e-6 || math.Abs(row.ExpectedValue-row.DirectValue-row.AllocatedValue) > 1e-6) {
			status = "partial"
		}
	}
	if expected <= 0 {
		return nil, "unavailable", nil
	}
	numerator := assigned
	if check.Quality.Field == "unassignedPct" {
		numerator = unassigned
	}
	return epathOracleNumber(math.Round(100*numerator/expected*1000) / 1000), status, nil
}

func epathSQLQualityNodeService(node EnergyExplanationNode) (string, error) {
	service := node.ServiceKind
	if node.Level == "end_use" {
		if node.EndUse != "cooling" && node.EndUse != "heating" {
			if service == "cooling" || service == "heating" {
				return "", fmt.Errorf("end-use service contradicts its typed category")
			}
			return "", nil
		}
		if service != "" && service != node.EndUse {
			return "", fmt.Errorf("end-use has conflicting service evidence")
		}
		service = node.EndUse
	}
	if node.Level != "load" && node.Level != "end_use" {
		return "", nil
	}
	if service != "cooling" && service != "heating" {
		return "", fmt.Errorf("load has no reviewed service identity")
	}
	return service, nil
}

func epathSQLQualityRatioCounts(bundle PurposeResultBundle, check epathSQLModelCheck, nodes []EnergyExplanationNode, links []EnergyPathLink) (int, int, string, error) {
	proof := check.Quality
	fail := func(err error) (int, int, string, error) { return 0, 0, "", err }
	if len(proof.Dependencies) == 0 || len(proof.Originals) == 0 {
		return fail(fmt.Errorf("ratio quality lacks independent graph/source proofs"))
	}
	for _, dependency := range proof.Dependencies {
		if dependency.Quality != nil || dependency.Item.Target.Collection == "quality" || dependency.Item.Scope != check.Item.Scope || !strings.EqualFold(dependency.Item.Zone, check.Item.Zone) || dependency.Item.Period != check.Item.Period {
			return fail(fmt.Errorf("quality dependency is recursive or outside the exact graph context"))
		}
		var err error
		if dependency.SiteFlow != nil {
			err = epathCheckSQLSiteFlow(bundle, dependency)
		} else if dependency.Conversion != nil {
			err = epathCheckSQLModelConversion(bundle, dependency)
		} else {
			var actual *float64
			actual, err = epathReadOracleCandidate(bundle, dependency.Item, dependency.Want)
			if err == nil {
				err = epathCheckSQLModelPresentation(bundle, dependency, actual)
			}
		}
		if err == nil && dependency.ZoneService != nil {
			err = epathCheckSQLZoneServiceEndpoints(bundle, dependency)
		}
		if err == nil && dependency.DirectUse != nil {
			err = epathCheckSQLDirectUseEndpoints(bundle, dependency)
		}
		if err != nil {
			return fail(fmt.Errorf("ratio graph prerequisite %s: %w", dependency.Want.Key, err))
		}
	}
	byID, services, candidates := map[string]EnergyExplanationNode{}, map[string]string{}, map[string]bool{}
	for _, node := range nodes {
		byID[node.ID] = node
		service, err := epathSQLQualityNodeService(node)
		if err != nil {
			return fail(err)
		}
		if service == "" {
			continue
		}
		domain := "thermal"
		if node.Level == "end_use" {
			domain = "site"
		}
		if node.ScaleDomain != domain || node.Unit != "kWh" || !epathOracleFinite(node.Value) || node.Value < 0 {
			return fail(fmt.Errorf("invalid ratio candidate endpoint %s", node.ID))
		}
		covered := false
		for _, dependency := range proof.Dependencies {
			target := dependency.Item.Target
			covered = covered || target.Collection == "nodes" && target.Field == "value" && target.ScaleDomain == domain && epathOracleNodeMatches(node, target)
		}
		if !covered {
			return fail(fmt.Errorf("ratio candidate endpoint %s has no independent node proof", node.ID))
		}
		services[node.ID] = service
		candidates[strings.ToLower(node.ZoneName)+"|"+service] = true
	}
	sources := map[string]EnergyDataSource{}
	for _, source := range bundle.EnergyExplanation.Sources {
		if _, duplicate := sources[source.ID]; source.ID == "" || duplicate {
			return fail(fmt.Errorf("ambiguous ratio source dictionary"))
		}
		sources[source.ID] = source
	}
	carrierTrace := map[string]bool{}
	carrierSums := map[string]map[string]float64{}
	for _, link := range links {
		if link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier" || services[link.FromID] == "" {
			continue
		}
		from, to := byID[link.FromID], byID[link.ToID]
		service := services[from.ID]
		ids, known := proof.SiteSources[service][to.Carrier]
		_, quantityKnown := proof.SiteValues[service][to.Carrier]
		if !known || !quantityKnown || to.Level != "carrier" || to.ScaleDomain != "site" || to.Unit != "kWh" || link.FromUnit != "kWh" || link.ToUnit != "kWh" || link.FromValue != link.ToValue || !epathOracleFinite(link.ToValue) || link.ToValue < 0 || link.Ratio != 0 || link.RatioKind != "" {
			return fail(fmt.Errorf("invalid ratio carrier evidence %s", link.ID))
		}
		if _, err := epathSQLQualitySourceLeaves(link.SourceIDs, sources, proof.Originals, ids); err != nil {
			return fail(err)
		}
		if carrierSums[service] == nil {
			carrierSums[service] = map[string]float64{}
		}
		carrierSums[service][to.Carrier] += link.ToValue
		carrierTrace[from.ID] = carrierTrace[from.ID] || link.ToValue > 0
	}
	for service, carriers := range proof.SiteValues {
		for carrier, q := range carriers {
			// Annual paired/direct relations may cover disjoint months. Their
			// combined site consumption, not each individual branch, equals the
			// independently observed/allocated total. SiteFlow/ZoneService proofs
			// above separately bind each relation's physical quantity and source.
			value := carrierSums[service][carrier]
			if err := epathCheckSQLModelQuantity(&value, &q); err != nil {
				return fail(fmt.Errorf("ratio carrier quantity %s/%s: %w", service, carrier, err))
			}
		}
	}
	found := map[string]bool{}
	for _, link := range links {
		if link.Relation != "load_to_end_use" {
			continue
		}
		from, to := byID[link.FromID], byID[link.ToID]
		service := services[from.ID]
		matched := false
		for _, dependency := range proof.Dependencies {
			matched = matched || dependency.Conversion != nil && epathSQLCoverageLinkMatches(link, dependency.Item.Target)
		}
		if !matched || service == "" || service != services[to.ID] || from.Level != "load" || to.Level != "end_use" || !strings.EqualFold(from.ZoneName, to.ZoneName) {
			return fail(fmt.Errorf("conversion is not an independently proved service pair: %s", link.ID))
		}
		siteIDs := []int{}
		for _, ids := range proof.SiteSources[service] {
			siteIDs = epathSQLDictionaryUnion(siteIDs, ids)
		}
		allowed := epathSQLDictionaryUnion(siteIDs, proof.LoadSources[service])
		leaves, err := epathSQLQualitySourceLeaves(link.SourceIDs, sources, proof.Originals, allowed)
		if err != nil {
			return fail(err)
		}
		fromLeaves, err := epathSQLQualitySourceLeaves(from.SourceIDs, sources, proof.Originals, proof.LoadSources[service])
		if err != nil {
			return fail(err)
		}
		toLeaves, err := epathSQLQualitySourceLeaves(to.SourceIDs, sources, proof.Originals, allowed)
		if err != nil {
			return fail(err)
		}
		loadTrace, siteTrace := false, false
		for _, id := range proof.LoadSources[service] {
			loadTrace = loadTrace || leaves[id] && fromLeaves[id]
		}
		for _, id := range siteIDs {
			siteTrace = siteTrace || leaves[id] && toLeaves[id]
		}
		if !loadTrace || !siteTrace {
			return fail(fmt.Errorf("conversion lacks exact independently owned load/site source trace"))
		}
		if link.FromValue > 0 && link.ToValue > 0 && carrierTrace[to.ID] {
			found[strings.ToLower(from.ZoneName)+"|"+service] = true
		}
	}
	if proof.LoadStatus == "not_requested" || proof.LoadStatus == "not_applicable" {
		return 0, 0, proof.LoadStatus, nil
	}
	status := "complete"
	if len(candidates) == 0 {
		status = "not_applicable"
	} else if len(found) == 0 {
		status = "missing"
	} else if len(found) < len(candidates) {
		status = "partial"
	}
	return len(found), len(candidates), status, nil
}

func epathSQLQualitySourceLeaves(ids []string, sources map[string]EnergyDataSource, originals map[int]epathRealSQLSource, allowed []int) (map[int]bool, error) {
	if len(ids) == 0 || len(allowed) == 0 {
		return nil, fmt.Errorf("ratio trace lacks original source evidence")
	}
	leaves, visiting, visited := map[int]bool{}, map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("cyclic ratio source inputs")
		}
		if visited[id] {
			return nil
		}
		source, exists := sources[id]
		if !exists {
			return fmt.Errorf("missing ratio source %s", id)
		}
		visiting[id] = true
		if len(source.InputSourceIDs) > 0 {
			for _, input := range source.InputSourceIDs {
				if err := visit(input); err != nil {
					return err
				}
			}
		} else {
			count := 0
			for _, originalID := range allowed {
				original, ok := originals[originalID]
				if ok && source.SourceType == "sql_report_data" && source.IsMeter == original.IsMeter && strings.EqualFold(source.Name, original.Name) && strings.EqualFold(source.KeyValue, original.KeyValue) && source.SourceUnit == original.SourceUnit && source.NormalizedUnit == "kWh" && strings.EqualFold(source.ReportingFrequency, original.ReportingFrequency) {
					leaves[originalID], count = true, count+1
				}
			}
			if count != 1 {
				return fmt.Errorf("ratio source %s has %d permitted original identities", id, count)
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return leaves, nil
}
