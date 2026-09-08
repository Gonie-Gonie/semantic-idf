package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Coverage is separate from numeric success. A matching scalar assertion is not
// evidence that every primary record, or the other end of a link, was checked.
type epathSQLModelCoverageRecord struct {
	Scope, Zone, Period, Collection, ID, Role, Reason string
	RequiredFields                                    []string
	Selectors                                         map[string][]string
}

type epathSQLModelCoverageReport struct {
	Records  []epathSQLModelCoverageRecord
	Failures []epathSQLModelFailure
}

type epathSQLCoverageContext struct {
	scope, zone, period string
	nodes               []EnergyExplanationNode
	links               []EnergyPathLink
	rows                []EnergyReconciliation
	quality             *EnergyPathQuality
}

func epathSQLCoverageContextKey(scope, zone, period string) string {
	return scope + "/" + strings.ToLower(zone) + "/" + period
}

func (out *epathSQLModelCoverageReport) fail(group, key, message string) {
	out.Failures = append(out.Failures, epathSQLModelFailure{Group: group, Key: key, Message: message})
}

// Call on the original-wire candidate. This function neither repairs graphs nor
// invokes a production classifier or builder. Structural failures use the group
// "coverage" and invalidate acceptance as a whole, not just one numeric group.
func epathSQLModelCoverage(bundle PurposeResultBundle, checks epathSQLModelChecks) epathSQLModelCoverageReport {
	out := epathSQLModelCoverageReport{}
	byContext := map[string][]epathSQLModelCheck{}
	seenKeys := map[string]bool{}
	for _, check := range checks.Rows {
		key := check.Want.Key
		if key == "" || seenKeys[key] || !checks.Keys[key] || check.Item.Key != key {
			out.fail("coverage", key, "missing/duplicate required selector identity or registry entry")
			continue
		}
		seenKeys[key] = true
		if check.Item.Scope != check.Want.Scope || check.Item.Zone != check.Want.Zone || check.Item.Period != check.Want.Period || check.Item.Group != check.Want.Group || check.Item.Unit != check.Want.Unit {
			out.fail("coverage", key, "selector and independently compiled expectation have contradictory context")
			continue
		}
		if err := epathValidateOracleMetricIdentity(check.Want); err != nil {
			out.fail("coverage", key, err.Error())
			continue
		}
		targetError := epathValidateOracleTarget(check.Item.Target, check.Item.Unit)
		if check.NativeVRFSource != nil {
			targetError = epathSQLVRFSourceTarget(check)
		}
		if err := targetError; err != nil {
			out.fail("coverage", key, err.Error())
			continue
		}
		context := epathSQLCoverageContextKey(check.Item.Scope, check.Item.Zone, check.Item.Period)
		byContext[context] = append(byContext[context], check)
	}
	for key, required := range checks.Keys {
		if !required || !seenKeys[key] {
			out.fail("coverage", key, "required compiled selector removed, including absent/pruned-record obligations")
		}
	}
	contexts := epathSQLCoverageContexts(bundle, &out)
	seenContexts := map[string]bool{}
	for _, context := range contexts {
		key := epathSQLCoverageContextKey(context.scope, context.zone, context.period)
		seenContexts[key] = true
		epathSQLCoverageRecords(bundle, context, byContext[key], byContext, &out)
	}
	for key := range byContext {
		if !seenContexts[key] {
			out.fail("coverage", key, "required selector context is absent from the candidate; no annual fallback")
		}
	}
	sort.Slice(out.Failures, func(i, j int) bool {
		return out.Failures[i].Key+out.Failures[i].Message < out.Failures[j].Key+out.Failures[j].Message
	})
	return out
}

func epathSQLCoverageContexts(bundle PurposeResultBundle, out *epathSQLModelCoverageReport) []epathSQLCoverageContext {
	result := bundle.EnergyExplanation
	if result.Schema != energyExplanationSchema || result.Scope.Kind != "building" || result.Scope.ZoneName != "" {
		out.fail("coverage", "root", "coverage requires the original canonical Building wrapper")
		return nil
	}
	zones := append([]string(nil), result.AvailableZones...)
	for _, zone := range result.ZoneResults {
		zones = append(zones, zone.Scope.ZoneName)
	}
	var contexts []epathSQLCoverageContext
	appendScope := func(annual epathSQLCoverageContext, periods []EnergyPeriod) {
		key := epathSQLCoverageContextKey(annual.scope, annual.zone, "annual")
		if err := epathValidateOracleGraphRecords(annual.nodes, annual.links, annual.rows, annual.scope, annual.zone, "annual", zones...); err != nil {
			out.fail("coverage", key, err.Error())
		}
		contexts = append(contexts, annual)
		seen := map[string]bool{}
		for _, period := range periods {
			periodKey := epathSQLCoverageContextKey(annual.scope, annual.zone, period.ID)
			if !epathOracleValidPeriod(period.ID) || seen[period.ID] {
				out.fail("coverage", periodKey, "invalid/duplicate candidate period wrapper")
				continue
			}
			seen[period.ID] = true
			context := epathSQLCoverageContext{annual.scope, annual.zone, period.ID, period.Nodes, period.Links, period.Reconciliation, period.Quality}
			if err := epathValidateOracleGraphRecords(context.nodes, context.links, context.rows, context.scope, context.zone, context.period, zones...); err != nil {
				out.fail("coverage", periodKey, err.Error())
			}
			if period.ID == "annual" {
				if !epathSQLCoverageSameAnnual(annual, context) {
					out.fail("coverage", periodKey, "wrapper annual records/quality contradict periods.annual; neither copy may hide the other")
				}
				continue
			}
			contexts = append(contexts, context)
		}
	}
	appendScope(epathSQLCoverageContext{"building", "", "annual", result.Nodes, result.Links, result.Reconciliation, result.Quality}, result.Periods)
	seenZones := map[string]bool{}
	for _, zone := range result.ZoneResults {
		name := strings.ToLower(zone.Scope.ZoneName)
		if zone.Scope.Kind != "zone" || strings.TrimSpace(name) == "" || seenZones[name] {
			out.fail("coverage", "zone/"+name, "invalid/duplicate Zone wrapper")
			continue
		}
		seenZones[name] = true
		appendScope(epathSQLCoverageContext{"zone", zone.Scope.ZoneName, "annual", zone.Nodes, zone.Links, zone.Reconciliation, zone.Quality}, zone.Periods)
	}
	for _, zone := range result.AvailableZones {
		if !seenZones[strings.ToLower(zone)] {
			out.fail("coverage", "zone/"+zone, "advertised Zone has no candidate wrapper")
		}
	}
	return contexts
}

func epathSQLCoverageSameAnnual(a, b epathSQLCoverageContext) bool {
	// Record order and source-ID order are not physical differences. Preserve
	// duplicates and every scalar, basis, period and source identity; never round.
	nodes := func(input []EnergyExplanationNode) []EnergyExplanationNode {
		out := append([]EnergyExplanationNode{}, input...)
		for index := range out {
			out[index].SourceIDs = append([]string(nil), out[index].SourceIDs...)
			sort.Strings(out[index].SourceIDs)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out
	}
	links := func(input []EnergyPathLink) []EnergyPathLink {
		out := append([]EnergyPathLink{}, input...)
		for index := range out {
			out[index].SourceIDs = append([]string(nil), out[index].SourceIDs...)
			sort.Strings(out[index].SourceIDs)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out
	}
	rows := func(input []EnergyReconciliation) []EnergyReconciliation {
		out := append([]EnergyReconciliation{}, input...)
		for index := range out {
			out[index].SourceIDs = append([]string(nil), out[index].SourceIDs...)
			sort.Strings(out[index].SourceIDs)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
		return out
	}
	return reflect.DeepEqual(nodes(a.nodes), nodes(b.nodes)) && reflect.DeepEqual(links(a.links), links(b.links)) && reflect.DeepEqual(rows(a.rows), rows(b.rows)) && reflect.DeepEqual(a.quality, b.quality)
}

func epathSQLCoverageRecords(bundle PurposeResultBundle, context epathSQLCoverageContext, checks []epathSQLModelCheck, all map[string][]epathSQLModelCheck, out *epathSQLModelCoverageReport) {
	byID := map[string]EnergyExplanationNode{}
	for _, node := range context.nodes {
		byID[node.ID] = node
	}
	// Reuse each proof's actual validation and exact record predicate. Merely
	// carrying a non-nil proof, or checking one same-service scalar, is not
	// permission to mark all of that service's links as checked.
	proofValid := map[string]bool{}
	qualityMetrics := map[string]epathRealOracleMetric{}
	for _, check := range checks {
		validators := []func() error{}
		if check.Reconciliation != nil {
			validators = append(validators, func() error { return epathCheckSQLModelReconciliation(bundle, check) })
		}
		if check.Allocation != nil && check.Allocation.RadiantCarrier != nil {
			validators = append(validators, func() error { return epathCheckSQLModelAllocation(bundle, check) })
		}
		if check.SiteFlow != nil {
			validators = append(validators, func() error { return epathCheckSQLSiteFlow(bundle, check) })
		}
		if check.SiteResidual != nil {
			validators = append(validators, func() error { return epathCheckSQLSiteResidual(bundle, check) })
		}
		if check.ZoneCarrier != nil {
			validators = append(validators, func() error { return epathCheckSQLZoneCarrier(bundle, check) })
		}
		if check.Quality != nil {
			validators = append(validators, func() error {
				metric, err := epathCheckSQLModelQuality(bundle, check)
				if err == nil {
					qualityMetrics[check.Want.Key] = metric
				}
				return err
			})
		}
		if check.DriverLink != nil {
			validators = append(validators, func() error { return epathCheckSQLModelDriverLink(bundle, check) })
		}
		if check.ZoneService != nil {
			validators = append(validators, func() error { return epathCheckSQLZoneServiceEndpoints(bundle, check) })
		}
		if check.DirectUse != nil {
			validators = append(validators, func() error { return epathCheckSQLDirectUseEndpoints(bundle, check) })
		}
		if check.AuxiliaryZone != nil {
			validators = append(validators, func() error { return epathCheckSQLAuxiliaryZone(bundle, check) })
		}
		if check.Conversion != nil {
			validators = append(validators, func() error { return epathCheckSQLModelConversion(bundle, check) })
		}
		if check.OriginalSource != nil {
			validators = append(validators, func() error { return epathCheckSQLModelOriginalSource(bundle, check) })
		}
		if check.LoadDetail != nil {
			validators = append(validators, func() error { return epathCheckSQLLoadDetailSource(bundle, check) })
		}
		if check.RadiantSurfaceContext != nil {
			validators = append(validators, func() error { return epathCheckSQLRadiantSurfaceContextSource(bundle, check) })
		}
		if check.TraceSource != nil {
			validators = append(validators, func() error { return epathCheckSQLTemporalTraceSource(bundle, check) })
		}
		if check.DirectHVACSource != nil {
			validators = append(validators, func() error { return epathCheckSQLDirectHVACSource(bundle, check) })
		}
		if check.NativeVRFSource != nil {
			validators = append(validators, func() error { return epathCheckSQLVRFSource(bundle, check) })
		}
		if check.NativeRadiantSource != nil {
			validators = append(validators, func() error { return epathCheckSQLRadiantLoadSource(bundle, check) })
		}
		if check.NativeRadiantLoad != nil {
			validators = append(validators, func() error { return epathCheckSQLRadiantLoadNode(bundle, check) })
		}
		if check.AnnualServiceAbsent {
			validators = append(validators, func() error { return epathCheckSQLAnnualServiceAbsent(bundle, check) })
		}
		if len(validators) == 0 {
			continue
		}
		valid := true
		for _, validate := range validators {
			if err := validate(); err != nil {
				valid = false
				out.fail(check.Want.Group, check.Want.Key, "custom proof cannot authorize record coverage: "+err.Error())
			}
		}
		proofValid[check.Want.Key] = valid
	}
	add := func(collection, id, group, role, reason string, fields []string, selected map[string][]string) {
		record := epathSQLModelCoverageRecord{context.scope, context.zone, context.period, collection, id, role, reason, fields, selected}
		out.Records = append(out.Records, record)
		for _, field := range fields {
			if len(selected[field]) == 0 {
				key := epathSQLCoverageContextKey(context.scope, context.zone, context.period) + "/" + collection + "/" + id + "/" + field
				out.fail(group, key, "unchecked primary record field: "+field+"; record="+id+"; "+reason)
			}
		}
	}
	canonicalSite := map[[2]string]string{}
	for _, node := range context.nodes {
		// Building reported meters have one canonical end-use category (across
		// carriers) or one carrier total. A sum selector alone would also cover
		// an added zero record, or two records splitting the original value.
		// Driver components, allocation bases, Zone subtotals and support are
		// deliberately outside this narrow identity/cardinality constraint.
		if context.scope == "building" && node.Basis == "reported_meter" && node.ScaleDomain == "site" {
			category := ""
			if node.Level == "end_use" {
				category = node.EndUse
			} else if node.Level == "carrier" {
				category = node.Carrier
			}
			if category != "" {
				identity := [2]string{node.Level, category}
				if first, duplicate := canonicalSite[identity]; duplicate {
					key := epathSQLCoverageContextKey(context.scope, context.zone, context.period) + "/nodes/" + node.ID + "/canonical-cardinality"
					out.fail("coverage", key, fmt.Sprintf("duplicate Building reported-meter canonical category %s/%s: %s and %s; a matching aggregate is not per-record proof", node.Level, category, first, node.ID))
				} else {
					canonicalSite[identity] = node.ID
				}
			}
		}
		group, role, reason, fields := epathSQLCoverageNodeRole(node, context.links, byID)
		selected := map[string][]string{}
		for _, check := range checks {
			target := check.Item.Target
			if check.ZoneCarrier != nil && !proofValid[check.Want.Key] {
				continue
			}
			if check.AuxiliaryZone != nil && !proofValid[check.Want.Key] {
				continue
			}
			if check.SiteResidual != nil && !proofValid[check.Want.Key] {
				continue
			}
			if target.Collection != "nodes" || !epathOracleNodeMatches(node, target) || target.Unit != node.Unit || target.ScaleDomain != node.ScaleDomain || target.AggregationBasis != "" && target.AggregationBasis != node.AggregationBasis {
				continue
			}
			field := target.Field
			if field == "loadBreakdown" {
				field += "/" + target.Component
			}
			selected[field] = append(selected[field], check.Want.Key)
		}
		add("nodes", node.ID, group, role, reason, fields, selected)
	}
	for _, link := range context.links {
		group, role, reason, fields := epathSQLCoverageLinkRole(link, byID)
		selected := map[string][]string{}
		for _, check := range checks {
			target := check.Item.Target
			if proofValid[check.Want.Key] {
				fields := []string{}
				switch {
				case check.SiteResidual != nil && epathSQLSiteResidualMatches(byID, link, check.SiteResidual):
					if target.Collection == "links" {
						fields = append(fields, target.Field)
					}
				case check.SiteFlow != nil && epathSQLSiteFlowMatches(byID, link, check.SiteFlow) && link.Relation == target.Relation:
					fields = append(fields, target.Field)
				case check.DriverLink != nil && epathSQLDriverLinkMatches(byID, link, check.DriverLink):
					fields = append(fields, target.Field)
				case check.ZoneService != nil && epathSQLZoneServiceCoveredLink(context.nodes, link, check.ZoneService):
					fields = append(fields, "fromValue", "toValue")
				case check.DirectUse != nil && epathSQLDirectUseLinkMatches(link, byID, check.DirectUse):
					fields = append(fields, "fromValue", "toValue")
				}
				for _, field := range fields {
					selected[field] = append(selected[field], check.Want.Key)
				}
			}
			if !epathSQLCoverageLinkMatches(link, target) {
				continue
			}
			if target.Field == "pairedRatio" && check.Conversion != nil && proofValid[check.Want.Key] {
				// This proof checks both original endpoints as well as their
				// quotient; a standalone aggregate ratio does not.
				for _, field := range []string{"fromValue", "toValue", "pairedRatio"} {
					selected[field] = append(selected[field], check.Want.Key)
				}
			} else if target.ID != "" || target.FromID != "" && target.ToID != "" {
				selected[target.Field] = append(selected[target.Field], check.Want.Key)
			}
		}
		add("links", link.ID, group, role, reason, fields, selected)
	}
	for _, row := range context.rows {
		group, fields := "residuals", []string{"expectedValue", "explainedValue", "residualValue"}
		if row.Level == "allocation" {
			group, fields = "zoneAllocation", []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"}
		}
		selected := map[string][]string{}
		candidates := checks
		role, reason := "primary", "exact reconciliation row"
		if context.scope == "zone" && row.Level == "allocation" && row.ZoneName == "" {
			// A copied Building ledger is context, but still requires the
			// actual Building row and its complete independent scalar checks.
			_, _, buildingRows, _, err := epathOracleGraph(bundle, "building", "", context.period)
			for _, building := range buildingRows {
				if err == nil && reflect.DeepEqual(row, building) {
					candidates = all[epathSQLCoverageContextKey("building", "", context.period)]
					role, reason = "referenced-accounting", "byte-equivalent Building allocation context; inherits only its exact checked fields"
				}
			}
		}
		for _, check := range candidates {
			target := check.Item.Target
			if check.Allocation != nil && check.Allocation.RadiantCarrier != nil {
				valid, checked := proofValid[check.Want.Key]
				if !checked {
					// A copied Building ledger is still proved against its
					// original Building context, not merely its repeated ID.
					valid = epathCheckSQLModelAllocation(bundle, check) == nil
					proofValid[check.Want.Key] = valid
				}
				if !valid {
					continue
				}
			}
			if check.Reconciliation != nil {
				if proofValid[check.Want.Key] && epathSQLReconciliationMatches(row, check.Reconciliation) {
					selected[target.Field] = append(selected[target.Field], check.Want.Key)
				}
				continue
			}
			if target.Collection == "reconciliation" && target.ID == row.ID && target.Level == row.Level && target.Unit == row.Unit && epathSQLCoverageMatch(target.Basis, row.Basis) && epathSQLCoverageMatch(target.Service, row.ServiceKind) && epathSQLCoverageMatch(target.AllocationMethod, row.AllocationMethod) && epathSQLCoverageMatch(target.Status, row.Status) {
				selected[target.Field] = append(selected[target.Field], check.Want.Key)
			}
		}
		add("reconciliation", row.ID, group, role, reason, fields, selected)
	}
	for _, field := range []string{"drivers", "loads", "endUses", "carriers", "ratios", "driverToLoadClosedPct", "endUseToCarrierClosedPct", "zoneAllocatedPct", "unassignedPct"} {
		selected := map[string][]string{}
		for _, check := range checks {
			if check.Quality != nil {
				metric, ok := qualityMetrics[check.Want.Key]
				if ok && proofValid[check.Want.Key] && context.quality != nil && check.Item.Target.Field == field && metric.Status != "" && (metric.Unit == "%" || metric.Found != nil && metric.Total != nil) {
					selected[field] = append(selected[field], check.Want.Key)
				}
				continue
			}
			if context.quality == nil || check.Item.Target.Collection != "quality" || check.Item.Target.Field != field || check.Want.Status == "" {
				continue
			}
			if field == "drivers" || field == "loads" || field == "endUses" || field == "carriers" || field == "ratios" {
				if check.Want.Status == "" || check.Want.Found == nil || check.Want.Total == nil {
					continue // Percentage alone does not prove count/unknown status.
				}
			}
			selected[field] = append(selected[field], check.Want.Key)
		}
		group := "completeness"
		if field == "ratios" {
			group = "ratios"
		} else if field == "zoneAllocatedPct" || field == "unassignedPct" {
			group = "zoneAllocation"
		} else if strings.HasSuffix(field, "ClosedPct") {
			group = "residuals"
		}
		add("quality", field, group, "primary", "scope-period quality requires its own numeric/unknown and count/status assertion", []string{field}, selected)
	}
}

func epathSQLCoverageMatch(filter, value string) bool { return filter == "" || filter == value }

func epathSQLCoverageLinkMatches(link EnergyPathLink, target epathRealOracleTarget) bool {
	return target.Collection == "links" && epathSQLCoverageMatch(target.ID, link.ID) && epathSQLCoverageMatch(target.FromID, link.FromID) && epathSQLCoverageMatch(target.ToID, link.ToID) && target.Relation == link.Relation && epathSQLCoverageMatch(target.Service, link.ServiceKind) && epathSQLCoverageMatch(target.Basis, link.Basis) && target.FromUnit == link.FromUnit && target.ToUnit == link.ToUnit && (target.Field == "pairedRatio" || epathSQLCoverageMatch(target.RatioKind, link.RatioKind))
}

func epathSQLCoverageNodeRole(node EnergyExplanationNode, links []EnergyPathLink, nodes map[string]EnergyExplanationNode) (string, string, string, []string) {
	switch node.Level {
	case "driver":
		return "drivers", "primary", "thermal driver including every distinct basis record", []string{"value", "rawValue", "effectiveValue"}
	case "load":
		return "loads", "primary", "delivered load and independently known/unknown components", []string{"value", "loadBreakdown/sensible", "loadBreakdown/latent"}
	case "end_use":
		return "endUses", "primary", "site end use, including direct and allocated uses", []string{"value"}
	case "carrier":
		return "carriers", "primary", "reported carrier or explicit Zone subtotal", []string{"value"}
	case "support":
		if epathSQLCoverageSupport(node) {
			return "carriers", "context", "typed purchased/produced/sold/storage support; never counted as conserved consumption", nil
		}
	case "residual":
		if node.ScaleDomain == "site" {
			return "residuals", "primary", "displayed site residual is a numeric branch, not exempt context", []string{"value"}
		}
		for _, link := range links {
			if link.FromID == node.ID && link.Relation == "residual" && node.ScaleDomain == "thermal" && node.Basis == "residual" && nodes[link.ToID].Level == "load" && nodes[link.ToID].ScaleDomain == "thermal" {
				return "residuals", "context", "retained legacy thermal residual-to-load reconciliation, not a primary driver", nil
			}
		}
	}
	return "coverage", "unclassified", "unrecognized auxiliary node is not an automatic exemption", []string{"explicit-reviewed-role"}
}

func epathSQLCoverageSupport(node EnergyExplanationNode) bool {
	if node.Level != "support" || node.ScaleDomain != "site" {
		return false
	}
	switch node.EndUse {
	case "electricity_purchased", "purchased", "generators", "electricity_sold", "sold", "storage_charge", "storage_discharge":
		return node.Carrier != "" && node.Unit != ""
	}
	return false
}

func epathSQLCoverageLinkRole(link EnergyPathLink, nodes map[string]EnergyExplanationNode) (string, string, string, []string) {
	from, to := nodes[link.FromID], nodes[link.ToID]
	validUnits := link.FromUnit != "" && link.FromUnit == from.Unit && link.ToUnit != "" && link.ToUnit == to.Unit
	if validUnits {
		switch link.Relation {
		case "driver_to_load":
			if from.Level == "driver" && to.Level == "load" && from.ScaleDomain == "thermal" && to.ScaleDomain == "thermal" {
				return "drivers", "primary", "allocated thermal contribution at both endpoints", []string{"fromValue", "toValue"}
			}
		case "load_to_end_use":
			if from.Level == "load" && to.Level == "end_use" && from.ScaleDomain == "thermal" && to.ScaleDomain == "site" {
				return "ratios", "primary", "paired conversion requires both endpoints and ratio availability", []string{"fromValue", "toValue", "pairedRatio"}
			}
		case "end_use_to_carrier", "direct_end_use_to_carrier":
			if from.Level == "end_use" && to.Level == "carrier" && from.ScaleDomain == "site" && to.ScaleDomain == "site" {
				return "endUses", "primary", "each carrier-specific consumption branch requires both endpoint checks", []string{"fromValue", "toValue"}
			}
		case "residual":
			if from.Level == "residual" && to.Level == "carrier" && from.ScaleDomain == "site" && to.ScaleDomain == "site" {
				return "residuals", "primary", "additive site residual branch", []string{"fromValue", "toValue"}
			}
			if from.Level == "residual" && to.Level == "load" && from.ScaleDomain == "thermal" && to.ScaleDomain == "thermal" && from.Basis == "residual" {
				return "residuals", "context", "retained legacy thermal reconciliation relation", nil
			}
		case "source_correspondence":
			if from.Level == "driver" && to.Level == "end_use" && from.ScaleDomain == "thermal" && to.ScaleDomain == "site" {
				return "drivers", "non-flow", "typed source correspondence has independent thermal/site quantities; not a flow exemption for either node", nil
			}
		case "support_supply":
			if epathSQLCoverageSupport(from) && to.Level == "carrier" && to.ScaleDomain == "site" || epathSQLCoverageSupport(to) && from.Level == "carrier" && from.ScaleDomain == "site" {
				return "carriers", "context", "typed support/carrier supply relation; not consumption", nil
			}
		}
	}
	return "coverage", "unclassified", fmt.Sprintf("relation %q has no valid reviewed endpoint/unit role", link.Relation), []string{"explicit-reviewed-role"}
}
