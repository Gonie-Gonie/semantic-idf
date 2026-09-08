package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// These are independently completed meter/Zone-month quantities, not values
// selected from the candidate's consumption ribbons.
type epathSQLSiteFlowProof struct {
	PairedCertain          epathSQLQuantity
	PairedChoices          []epathSQLQuantity
	EndUse, Carrier, Basis string
	Total, NodeTotal       epathSQLQuantity
	Relations              map[string]epathSQLQuantity
	Sources, NodeSources   map[string]epathRealSQLSource
	Required               map[string]map[string]bool
	AllowedCarriers        map[string]bool
	MonthlyExclusive       bool
}

func epathSQLSiteFlowMonth(frames epathSQLFrames, model epathRealSQLModel, site epathRealSQLSite, month int) (map[string]epathSQLQuantity, error) {
	values := frames.Site[site.ID]
	if len(values) != 12 || values[month-1] == nil || !values[month-1].valid() || values[month-1].Value < 0 {
		return nil, fmt.Errorf("site flow needs a known nonnegative meter %s/M%d", site.ID, month)
	}
	q := values[month-1].positive()
	out := map[string]epathSQLQuantity{"end_use_to_carrier": {}, "direct_end_use_to_carrier": q}
	var selected *epathRealSQLService
	for i := range model.Services {
		for _, id := range model.Services[i].SiteIDs {
			if id == site.ID {
				if selected != nil || model.Services[i].Service != site.EndUse {
					return nil, fmt.Errorf("ambiguous site flow service membership %s", site.ID)
				}
				selected = &model.Services[i]
			}
		}
	}
	if selected == nil {
		if site.EndUse == "cooling" || site.EndUse == "heating" {
			return nil, fmt.Errorf("thermal end use lacks reviewed service membership %s", site.ID)
		}
		return out, nil
	}
	zones, err := epathSQLDeclaredZones(frames, selected.ServedZones)
	if err != nil {
		return nil, err
	}
	load := epathSQLQuantity{}
	for zone := range zones {
		value, ok := frames.Loads[epathSQLKey(zone, selected.Service, month)]
		if !ok || !value.valid() || value.Value < 0 {
			return nil, fmt.Errorf("site flow has unknown served load %s/M%d", zone, month)
		}
		load = load.add(value)
	}
	low, high := load.bounds()
	_, siteHigh := q.bounds()
	if low > 0 {
		out["end_use_to_carrier"], out["direct_end_use_to_carrier"] = q, epathSQLQuantity{}
	} else if high > 0 && siteHigh > 0 {
		// A proven display interval crossing zero may remove the paired load.
		// Both branch choices are bounded by the same meter; their combined
		// quantity below must still equal that independently observed meter.
		paired, direct := 0.0, q.Value
		if load.Value > 0 && q.Value > 0 {
			paired, direct = q.Value, 0
		}
		out["end_use_to_carrier"] = epathSQLBounded(paired, 0, siteHigh)
		out["direct_end_use_to_carrier"] = epathSQLBounded(direct, 0, siteHigh)
	}
	return out, nil
}

func epathSQLModelSiteFlowChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	periods := []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"}
	for _, period := range periods {
		proofs := map[string]*epathSQLSiteFlowProof{}
		nodeTotals := map[string]epathSQLQuantity{}
		nodeSources := map[string]map[string]epathRealSQLSource{}
		carriers := map[string]map[string]bool{}
		for _, site := range model.Site {
			if site.Facility {
				continue
			}
			if site.EndUse == "" || site.Carrier == "" || len(frames.SiteSources[site.ID]) == 0 {
				return fmt.Errorf("site flow lacks exact reviewed meter identity %s", site.ID)
			}
			key := site.EndUse + "/" + site.Carrier
			p := proofs[key]
			if p == nil {
				p = &epathSQLSiteFlowProof{EndUse: site.EndUse, Carrier: site.Carrier, Basis: "reported_meter", Relations: map[string]epathSQLQuantity{"end_use_to_carrier": {}, "direct_end_use_to_carrier": {}}, Sources: map[string]epathRealSQLSource{}, Required: map[string]map[string]bool{"end_use_to_carrier": {}, "direct_end_use_to_carrier": {}, "node": {}}}
				proofs[key] = p
				p.MonthlyExclusive = period != "annual"
			} else {
				p.MonthlyExclusive = false // Distinct reviewed pools may choose distinct branches.
			}
			if nodeSources[site.EndUse] == nil {
				nodeSources[site.EndUse], carriers[site.EndUse] = map[string]epathRealSQLSource{}, map[string]bool{}
			}
			carriers[site.EndUse][site.Carrier] = true
			for _, id := range frames.SiteSources[site.ID] {
				original, ok := frames.SourceIdentities[id]
				if !ok || original.DictionaryIndex != id || !original.IsMeter {
					return fmt.Errorf("site flow has an unbound original meter %d", id)
				}
				sourceID := fmt.Sprintf("sql-rdd-%d", id)
				p.Sources[sourceID], nodeSources[site.EndUse][sourceID] = original, original
				months, err := epathSQLMonthly(original, model.Precision)
				if err != nil {
					return err
				}
				for _, month := range epathSQLPeriodMonths(period) {
					split, err := epathSQLSiteFlowMonth(frames, model, site, month)
					if err != nil {
						return err
					}
					if !months[month-1].includesZero() {
						p.Required["node"][sourceID] = true
						for relation, q := range split {
							if !q.includesZero() {
								p.Required[relation][sourceID] = true
							}
						}
					}
				}
			}
			for _, month := range epathSQLPeriodMonths(period) {
				split, err := epathSQLSiteFlowMonth(frames, model, site, month)
				if err != nil {
					return err
				}
				for relation, q := range split {
					p.Relations[relation] = p.Relations[relation].add(q)
				}
				value := frames.Site[site.ID][month-1].positive()
				_, pairedHigh := split["end_use_to_carrier"].bounds()
				_, directHigh := split["direct_end_use_to_carrier"].bounds()
				if pairedHigh > 0 && directHigh > 0 {
					p.PairedChoices = append(p.PairedChoices, value)
				} else {
					p.PairedCertain = p.PairedCertain.add(split["end_use_to_carrier"])
				}
				p.Total = p.Total.add(value)
				nodeTotals[site.EndUse] = nodeTotals[site.EndUse].add(value)
			}
		}
		keys := []string{}
		for key := range proofs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			p := proofs[key]
			p.NodeTotal, p.NodeSources, p.AllowedCarriers = nodeTotals[p.EndUse], nodeSources[p.EndUse], carriers[p.EndUse]
			for _, relation := range []string{"end_use_to_carrier", "direct_end_use_to_carrier"} {
				for _, field := range []string{"fromValue", "toValue"} {
					q := p.Relations[relation]
					target := epathRealOracleTarget{Collection: "links", Field: field, Relation: relation, Basis: p.Basis, FromUnit: "kWh", ToUnit: "kWh", Aggregate: "sum"}
					if err := checks.add("endUses", "building", "", period, "site_flow/"+key+"/"+relation+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
						return err
					}
					checks.Rows[len(checks.Rows)-1].SiteFlow = p
				}
			}
		}
	}
	return nil
}

func epathSQLSiteFlowMatches(nodes map[string]EnergyExplanationNode, link EnergyPathLink, p *epathSQLSiteFlowProof) bool {
	return nodes[link.FromID].Level == "end_use" && nodes[link.FromID].EndUse == p.EndUse && nodes[link.ToID].Carrier == p.Carrier && (link.Relation == "end_use_to_carrier" || link.Relation == "direct_end_use_to_carrier")
}

// Reuse only the already independently compiled fan shares, never candidate
// fan values. Exact Hourly source ownership comes from the reviewed pool roster.
func epathSQLModelFanFlowChecks(observed epathRealOracleEvidence, frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	if len(model.FanPools) == 0 {
		return nil
	}
	pools, err := epathSQLFanPoolObservations(observed.Sources, model.FanPools)
	if err != nil {
		return err
	}
	siteCarriers := map[string]string{}
	for _, site := range model.Site {
		if !site.Facility && site.EndUse == "fans" {
			if site.Carrier == "" || siteCarriers[site.ID] != "" {
				return fmt.Errorf("ambiguous reviewed fan carrier")
			}
			siteCarriers[site.ID] = site.Carrier
		}
	}
	prior := append([]epathSQLModelCheck(nil), checks.Rows...)
	seen := map[string]bool{}
	for _, check := range prior {
		if check.Item.Scope != "zone" || check.Item.Target.Collection != "nodes" || check.Item.Target.Category != "fans" || check.Item.Target.Field != "value" || !strings.HasSuffix(check.Want.Key, "|fan_pools/value") {
			continue
		}
		identity := strings.ToLower(check.Item.Zone) + "/" + check.Item.Period
		if seen[identity] || check.Quantity == nil || !check.Quantity.valid() || check.Quantity.Value < 0 {
			return fmt.Errorf("missing/duplicate independently compiled fan share")
		}
		seen[identity] = true
		p := &epathSQLSiteFlowProof{EndUse: "fans", Basis: "service_path_allocation", Total: *check.Quantity, NodeTotal: *check.Quantity, Relations: map[string]epathSQLQuantity{"direct_end_use_to_carrier": *check.Quantity, "end_use_to_carrier": {}}, Sources: map[string]epathRealSQLSource{}, Required: map[string]map[string]bool{"node": {}, "direct_end_use_to_carrier": {}, "end_use_to_carrier": {}}, AllowedCarriers: map[string]bool{}}
		for _, pool := range pools {
			for _, zone := range pool.Declaration.ServedZones {
				if !strings.EqualFold(zone, check.Item.Zone) {
					continue
				}
				carrier := siteCarriers[pool.Declaration.SiteID]
				if carrier == "" || p.Carrier != "" && p.Carrier != carrier {
					return fmt.Errorf("fan share lacks a unique independently proven carrier")
				}
				p.Carrier, p.AllowedCarriers[carrier] = carrier, true
				id := fmt.Sprintf("sql-rdd-%d", pool.Source.DictionaryIndex)
				p.Sources[id] = pool.Source
				if !check.Quantity.includesZero() {
					p.Required["node"][id], p.Required["direct_end_use_to_carrier"][id] = true, true
				}
				ids := frames.SiteSources[pool.Declaration.SiteID]
				if len(ids) == 0 {
					return fmt.Errorf("fan trace lacks its independently observed broad meter")
				}
				for _, dictionaryID := range ids {
					source, exists := frames.SourceIdentities[dictionaryID]
					if !exists || !source.IsMeter || source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" {
						return fmt.Errorf("fan trace has an invalid broad meter identity")
					}
					sourceID := fmt.Sprintf("sql-rdd-%d", dictionaryID)
					p.Sources[sourceID] = source
					if !check.Quantity.includesZero() {
						p.Required["node"][sourceID], p.Required["direct_end_use_to_carrier"][sourceID] = true, true
					}
				}
			}
		}
		if p.Carrier == "" {
			if check.Quantity.Value != 0 {
				return fmt.Errorf("positive fan allocation escapes every reviewed fan pool")
			}
			// No served pool means a required zero/absent fan end use. The
			// independent scalar obligation remains in the original checks.
			continue
		}
		p.NodeSources = map[string]epathRealSQLSource{}
		for id, source := range p.Sources {
			p.NodeSources[id] = source
		}
		// Load observations justify this Zone's weighting, not additional fan
		// consumption. Preserve that exact context only on the allocated node.
		for _, month := range epathSQLPeriodMonths(check.Item.Period) {
			for _, service := range []string{"cooling", "heating"} {
				key := epathSQLKey(check.Item.Zone, service, month)
				load, exists := frames.Loads[key]
				if !exists || !load.valid() || load.Value < 0 {
					return fmt.Errorf("fan trace lacks known selected Zone load context")
				}
				_, high := load.bounds()
				if high <= 0 {
					continue
				}
				ids := frames.LoadSourceIDs[key]
				if len(ids) == 0 {
					return fmt.Errorf("fan trace lacks independently bound positive load sources")
				}
				for _, dictionaryID := range ids {
					source, exists := frames.SourceIdentities[dictionaryID]
					if !exists || source.IsMeter || !strings.EqualFold(source.KeyValue, check.Item.Zone) || source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" {
						return fmt.Errorf("fan weight trace escapes the exact observed Zone load")
					}
					id := fmt.Sprintf("sql-rdd-%d", dictionaryID)
					p.NodeSources[id] = source
					if !load.includesZero() && !check.Quantity.includesZero() {
						p.Required["node"][id] = true
					}
				}
			}
		}
		for _, relation := range []string{"direct_end_use_to_carrier", "end_use_to_carrier"} {
			for _, field := range []string{"fromValue", "toValue"} {
				q := p.Relations[relation]
				target := epathRealOracleTarget{Collection: "links", Field: field, Relation: relation, Basis: p.Basis, FromUnit: "kWh", ToUnit: "kWh", Aggregate: "sum"}
				if err := checks.add("zoneAllocation", "zone", check.Item.Zone, check.Item.Period, "fan_flow/"+relation+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].SiteFlow = p
			}
		}
	}
	for zone := range frames.Zones {
		for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
			if !seen[zone+"/"+period] {
				return fmt.Errorf("required independent fan share is missing for %s/%s", zone, period)
			}
		}
	}
	return nil
}

func epathCheckSQLSiteFlow(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p := check.SiteFlow
	if p == nil || p.EndUse == "" || p.Carrier == "" || p.Basis == "" || !p.Total.valid() || !p.NodeTotal.valid() || check.Quantity == nil || !check.Quantity.valid() {
		return fmt.Errorf("invalid independent site-flow proof")
	}
	want, ok := p.Relations[check.Item.Target.Relation]
	if !ok || check.Item.Target.Collection != "links" || check.Item.Target.Basis != p.Basis || check.Quantity.Value != want.Value || check.Item.Target.FromUnit != "kWh" || check.Item.Target.ToUnit != "kWh" {
		return fmt.Errorf("site-flow selector contradicts its independent branch")
	}
	nodes, links, _, _, err := epathOracleGraph(bundle, check.Item.Scope, check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	byID, sources := map[string]EnergyExplanationNode{}, map[string]EnergyDataSource{}
	var selected *EnergyExplanationNode
	for i := range nodes {
		node := nodes[i]
		byID[node.ID] = node
		if node.Level == "end_use" && node.EndUse == p.EndUse {
			if selected != nil || !strings.EqualFold(node.ZoneName, check.Item.Zone) || node.Period != check.Item.Period || node.Basis != p.Basis || node.Unit != "kWh" || node.ScaleDomain != "site" || node.AggregationBasis != "model_total" || node.ServiceKind != "" && node.ServiceKind != p.EndUse {
				return fmt.Errorf("duplicate or mistyped site end-use endpoint")
			}
			selected = &nodes[i]
		}
	}
	for _, source := range bundle.EnergyExplanation.Sources {
		if _, duplicate := sources[source.ID]; duplicate {
			return fmt.Errorf("duplicate site-flow source identity")
		}
		sources[source.ID] = source
	}
	if selected == nil {
		if p.NodeTotal.includesZero() {
			return nil
		}
		return fmt.Errorf("missing independent site end-use endpoint")
	}
	if err := epathCheckSQLModelQuantity(&selected.Value, &p.NodeTotal); err != nil {
		return err
	}
	if err := epathSQLZoneServiceVerifySources(selected.SourceIDs, sources, p.NodeSources, p.Required["node"]); err != nil {
		return err
	}
	seen := map[string]bool{}
	total, pairedTotal := 0.0, 0.0
	for _, link := range links {
		if link.FromID != selected.ID {
			continue
		}
		to := byID[link.ToID]
		if !p.AllowedCarriers[to.Carrier] || to.Level != "carrier" || to.Unit != "kWh" || to.ScaleDomain != "site" || to.AggregationBasis != "model_total" || !strings.EqualFold(to.ZoneName, check.Item.Zone) || to.Period != check.Item.Period || (link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier") {
			return fmt.Errorf("site end use has an unrelated consumption branch")
		}
		if to.Carrier != p.Carrier {
			continue
		}
		if seen[link.Relation] || !strings.EqualFold(link.ZoneName, check.Item.Zone) || link.Period != check.Item.Period || link.Basis != p.Basis || link.FromUnit != "kWh" || link.ToUnit != "kWh" || link.FromValue != link.ToValue || !epathOracleFinite(link.FromValue) || link.FromValue <= 0 || link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" || link.ServiceKind != "" && !strings.EqualFold(link.ServiceKind, p.EndUse) {
			return fmt.Errorf("duplicate/invalid paired site-domain branch")
		}
		q := p.Relations[link.Relation]
		if err := epathCheckSQLModelQuantity(&link.FromValue, &q); err != nil {
			return err
		}
		if err := epathSQLZoneServiceVerifySources(link.SourceIDs, sources, p.Sources, p.Required[link.Relation]); err != nil {
			return err
		}
		if len(link.SourceIDs) == 0 {
			return fmt.Errorf("consumption branch has no original site evidence")
		}
		seen[link.Relation], total = true, total+link.FromValue
		if link.Relation == "end_use_to_carrier" {
			pairedTotal += link.FromValue
		}
	}
	if p.MonthlyExclusive && len(seen) > 1 {
		return fmt.Errorf("one completed monthly meter cannot be fractionally split between exclusive branch choices")
	}
	if err := epathSQLSiteFlowChoiceContains(p, pairedTotal); err != nil {
		return err
	}
	for relation, q := range p.Relations {
		if !seen[relation] && !q.includesZero() {
			return fmt.Errorf("missing independently positive site branch %s", relation)
		}
	}
	return epathCheckSQLModelQuantity(&total, &p.Total)
}

// Each completed pool-month chooses zero or its full paired site quantity.
// A convex envelope would permit an unsupported fractional redistribution.
func epathSQLSiteFlowChoiceContains(p *epathSQLSiteFlowProof, actual float64) error {
	low, high := p.PairedCertain.bounds()
	options := [][2]float64{{low, high}}
	for _, choice := range p.PairedChoices {
		if !choice.valid() || choice.Value < 0 {
			return fmt.Errorf("invalid monthly site branch choice")
		}
		cl, ch := choice.bounds()
		next := append([][2]float64(nil), options...)
		for _, interval := range options {
			next = append(next, [2]float64{interval[0] + cl, interval[1] + ch})
		}
		sort.Slice(next, func(i, j int) bool { return next[i][0] < next[j][0] })
		options = nil
		for _, interval := range next {
			if len(options) > 0 && interval[0] <= options[len(options)-1][1] {
				options[len(options)-1][1] = math.Max(options[len(options)-1][1], interval[1])
			} else {
				options = append(options, interval)
			}
		}
		if len(options) > 65536 {
			return fmt.Errorf("unresolved independent site branch choice union is too large")
		}
	}
	for _, interval := range options {
		slack := 1e-8 * math.Max(1, math.Max(math.Abs(interval[0]), math.Abs(interval[1])))
		if actual >= interval[0]-slack && actual <= interval[1]+slack {
			return nil
		}
	}
	return fmt.Errorf("annual paired energy is not a sum of permitted completed-month branch choices")
}
