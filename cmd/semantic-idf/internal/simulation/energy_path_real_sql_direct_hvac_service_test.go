package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// One independent carrier-qualified monthly allocation is consumed by the
// Building ledger and every Zone proof. Direct presence is not Value>0: a
// reported zero still excludes that owner from the remainder denominator.
type epathSQLDirectHVACShare struct {
	Direct, Allocated epathSQLQuantity
	ObservedDirect    bool
	DirectSources     map[string]epathSQLOriginalSource
}
type epathSQLDirectHVACServiceMonth struct {
	Site, Direct, Allocated, Unassigned epathSQLQuantity
	Zones                               map[string]map[string]epathSQLDirectHVACShare
}
type epathSQLDirectHVACServiceFrames struct {
	Monthly      [12]epathSQLDirectHVACServiceMonth
	Carriers     []string
	BroadSources map[string]map[string]epathSQLOriginalSource
}
type epathSQLDirectHVACBranchProof struct {
	Basis, Carrier, Relation string
	Quantity                 epathSQLQuantity
	Originals                map[string]epathSQLOriginalSource
	Required                 map[string]bool
}

func epathSQLCompileDirectHVACService(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService) (*epathSQLDirectHVACServiceFrames, error) {
	if len(model.DirectHVACComponents) == 0 {
		if len(frames.DirectHVAC) > 0 {
			return nil, fmt.Errorf("undeclared direct HVAC frames")
		}
		return nil, nil
	}
	declared := map[string]bool{}
	declaredMembers := map[string]map[string]bool{}
	for _, component := range model.DirectHVACComponents {
		if component.Service != service.Service {
			continue
		}
		for _, owner := range component.Owners {
			for month := 1; month <= 12; month++ {
				key := epathSQLDirectHVACKey(owner.ZoneName, component.Service, component.Carrier, month)
				declared[key] = true
				if declaredMembers[key] == nil {
					declaredMembers[key] = map[string]bool{}
				}
				member := component.ID + "|" + strings.ToLower(owner.KeyValue)
				if declaredMembers[key][member] {
					return nil, fmt.Errorf("duplicate direct cohort member declaration")
				}
				declaredMembers[key][member] = true
			}
		}
	}
	if len(declared) == 0 {
		return nil, nil
	}
	served, err := epathSQLDeclaredZones(frames, service.ServedZones)
	if err != nil || len(served) == 0 {
		return nil, fmt.Errorf("direct service requires exact served owners: %v", err)
	}
	annual, err := epathSQLServiceIsAnnualOnly(frames, service)
	if err != nil || annual {
		return nil, fmt.Errorf("direct HVAC requires compatible Monthly site pools: %v", err)
	}
	out := &epathSQLDirectHVACServiceFrames{BroadSources: map[string]map[string]epathSQLOriginalSource{}}
	sites := map[string]epathRealSQLSite{}
	for _, site := range model.Site {
		if sites[site.ID].ID != "" {
			return nil, fmt.Errorf("duplicate direct service site")
		}
		sites[site.ID] = site
	}
	byCarrier := map[string][]string{}
	seenSites, seenSources := map[string]bool{}, map[int]bool{}
	for _, id := range service.SiteIDs {
		site, ok := sites[id]
		if !ok || seenSites[id] || site.Facility || site.EndUse != service.Service || site.Carrier == "" || len(frames.SiteSources[id]) == 0 {
			return nil, fmt.Errorf("direct service has invalid broad site %s", id)
		}
		seenSites[id] = true
		byCarrier[site.Carrier] = append(byCarrier[site.Carrier], id)
		if out.BroadSources[site.Carrier] == nil {
			out.BroadSources[site.Carrier] = map[string]epathSQLOriginalSource{}
		}
		for _, index := range frames.SiteSources[id] {
			source, ok := frames.SourceIdentities[index]
			if !ok || index <= 0 || source.DictionaryIndex != index || seenSources[index] || !source.IsMeter || source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" || len(frames.SourceRaw[index]) != 12 {
				return nil, fmt.Errorf("invalid original broad direct-service source %d", index)
			}
			seenSources[index] = true
			out.BroadSources[site.Carrier][fmt.Sprintf("sql-rdd-%d", index)] = epathSQLOriginalRDD(source)
		}
	}
	for carrier := range byCarrier {
		out.Carriers = append(out.Carriers, carrier)
	}
	sort.Strings(out.Carriers)
	if len(out.Carriers) == 0 {
		return nil, fmt.Errorf("direct service has no carriers")
	}
	used := map[string]bool{}
	for month := 1; month <= 12; month++ {
		row := epathSQLDirectHVACServiceMonth{Zones: map[string]map[string]epathSQLDirectHVACShare{}}
		for zone := range frames.Zones {
			load, ok := frames.Loads[epathSQLKey(zone, service.Service, month)]
			if !ok || !load.valid() || load.Value < 0 {
				return nil, fmt.Errorf("unknown direct service Zone load %s/M%d", zone, month)
			}
			row.Zones[zone] = map[string]epathSQLDirectHVACShare{}
		}
		for _, carrier := range out.Carriers {
			pool, err := epathSQLSiteSum(frames, byCarrier[carrier], month)
			if err != nil || !pool.valid() || pool.Value < 0 {
				return nil, fmt.Errorf("invalid direct site pool: %v", err)
			}
			direct, denominator := epathSQLQuantity{}, epathSQLQuantity{}
			for zone := range frames.Zones {
				key := epathSQLDirectHVACKey(zone, service.Service, carrier, month)
				observation, exists := frames.DirectHVAC[key]
				share := epathSQLDirectHVACShare{DirectSources: map[string]epathSQLOriginalSource{}}
				if exists != declared[key] {
					return nil, fmt.Errorf("missing/undeclared direct observation %s", key)
				}
				if exists {
					if !served[zone] || !observation.Present || !observation.Quantity.valid() || observation.Quantity.Value < 0 || len(observation.SourceIDs) == 0 {
						return nil, fmt.Errorf("invalid/unowned direct observation %s", key)
					}
					used[key] = true
					share.ObservedDirect = true
					share.Direct = observation.Quantity.positive()
					members := map[string]bool{}
					sourceSum := epathSQLQuantity{}
					for _, index := range observation.SourceIDs {
						identity, ok := frames.DirectHVACSourceIdentities[index]
						if !ok || identity.Service != service.Service || identity.Carrier != carrier || !strings.EqualFold(identity.Owner.ZoneName, zone) || !seenSites[identity.SiteID] || sites[identity.SiteID].Carrier != carrier {
							return nil, fmt.Errorf("direct source owner/service/carrier mismatch %d", index)
						}
						original, err := epathSQLOriginalDirectHVAC(identity)
						if err != nil {
							return nil, err
						}
						member := identity.FamilyID + "|" + strings.ToLower(identity.Owner.KeyValue)
						if !declaredMembers[key][member] || members[member] {
							return nil, fmt.Errorf("undeclared/duplicate original direct cohort member")
						}
						members[member] = true
						quantities, err := epathSQLMonthly(identity.Source, identity.Precision)
						if err != nil {
							return nil, err
						}
						sourceSum = sourceSum.add(quantities[month-1].positive())
						id := fmt.Sprintf("sql-rdd-%d", index)
						if _, duplicate := share.DirectSources[id]; duplicate {
							return nil, fmt.Errorf("duplicate direct constituent source")
						}
						share.DirectSources[id] = original
					}
					if len(members) != len(declaredMembers[key]) || !epathSQLZoneCarrierQuantityEqual(sourceSum, share.Direct) {
						return nil, fmt.Errorf("direct frame does not equal every independently observed cohort constituent")
					}
					direct = direct.add(share.Direct)
				} else if served[zone] {
					denominator = denominator.add(frames.Loads[epathSQLKey(zone, service.Service, month)])
				}
				row.Zones[zone][carrier] = share
			}
			if direct.Value-pool.Value > 1e-10*math.Max(1, pool.Value) {
				return nil, fmt.Errorf("observed direct consumption exceeds the exact carrier meter")
			}
			remainder := pool.add(direct.times(-1)).positive()
			allocated, unassigned := epathSQLQuantity{}, epathSQLQuantity{}
			if denominator.Value == 0 {
				_, high := denominator.bounds()
				if high != 0 {
					return nil, fmt.Errorf("uncertain unobserved-owner load denominator")
				}
				unassigned = remainder
			} else {
				for zone := range frames.Zones {
					share := row.Zones[zone][carrier]
					if !served[zone] || share.ObservedDirect {
						continue
					}
					q, err := epathSQLShare(remainder, frames.Loads[epathSQLKey(zone, service.Service, month)], denominator, model.Precision)
					if err != nil {
						return nil, err
					}
					share.Allocated = q
					row.Zones[zone][carrier] = share
					allocated = allocated.add(q)
				}
			}
			row.Site = row.Site.add(pool)
			row.Direct = row.Direct.add(direct)
			row.Allocated = row.Allocated.add(allocated)
			row.Unassigned = row.Unassigned.add(unassigned)
		}
		out.Monthly[month-1] = row
	}
	if len(used) != len(declared) {
		return nil, fmt.Errorf("unbound direct cohort carrier or owner")
	}
	return out, nil
}

func epathSQLDirectHVACBindBuildingSources(frames epathSQLFrames, service epathRealSQLService, served map[string]bool, direct *epathSQLDirectHVACServiceFrames, period string, proof *epathSQLConversionProof) error {
	proof.DirectHVACSources = map[string]epathSQLOriginalSource{}
	proof.DirectHVACRequired = map[string]bool{}
	for _, sources := range direct.BroadSources {
		for id, source := range sources {
			proof.DirectHVACSources[id] = source
			for _, month := range epathSQLPeriodMonths(period) {
				if source.RDD != nil && frames.SourceRaw[source.RDD.DictionaryIndex][month-1].Value > 0 {
					proof.DirectHVACRequired[id] = true
				}
			}
		}
	}
	for zone := range served {
		for _, month := range epathSQLPeriodMonths(period) {
			if frames.Loads[epathSQLKey(zone, service.Service, month)].Value <= 0 {
				continue
			}
			ids := frames.LoadSourceIDs[epathSQLKey(zone, service.Service, month)]
			if len(ids) == 0 {
				return fmt.Errorf("Building direct-service conversion lacks exact served load provenance")
			}
			for _, index := range ids {
				source, ok := frames.SourceIdentities[index]
				if !ok || source.DictionaryIndex != index || source.IsMeter || !strings.EqualFold(source.KeyValue, zone) {
					return fmt.Errorf("invalid direct-service Building load source")
				}
				id := fmt.Sprintf("sql-rdd-%d", index)
				proof.DirectHVACSources[id] = epathSQLOriginalRDD(source)
				proof.DirectHVACRequired[id] = true
			}
		}
	}
	return nil
}

func epathCheckSQLDirectHVACBuildingSources(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof := check.Conversion
	if proof == nil || proof.DirectHVACSources == nil || proof.DirectHVACRequired == nil || check.Item.Scope != "building" || check.Item.Zone != "" || check.Item.Target.Relation != "load_to_end_use" {
		return fmt.Errorf("invalid direct-service Building conversion source proof")
	}
	_, links, _, _, err := epathOracleGraph(bundle, "building", "", check.Item.Period)
	if err != nil {
		return err
	}
	sources, err := epathSQLDirectHVACSourceMap(bundle)
	if err != nil {
		return err
	}
	used := map[string]bool{}
	for _, link := range links {
		if link.Relation != check.Item.Target.Relation || link.ServiceKind != check.Item.Target.Service || link.Basis != check.Item.Target.Basis {
			continue
		}
		leaves, err := epathSQLOriginalSourceLeaves(link.SourceIDs, sources, proof.DirectHVACSources, check.Item.Period)
		if err != nil {
			return err
		}
		for id := range leaves {
			used[id] = true
		}
	}
	if proof.From.includesZero() || proof.To.includesZero() {
		return nil
	}
	for id, required := range proof.DirectHVACRequired {
		if required && !used[id] {
			return fmt.Errorf("Building conversion lost exact independently required source %s", id)
		}
	}
	return nil
}

func epathSQLDirectHVACZoneServiceChecks(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService, direct *epathSQLDirectHVACServiceFrames, zones []string, checks *epathSQLModelChecks) error {
	for _, zone := range zones {
		for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
			proof := &epathSQLZoneServiceProof{DirectHVAC: true, Service: service.Service, Basis: service.Basis, Carriers: map[string]epathSQLQuantity{}, Branches: map[string]epathSQLDirectHVACBranchProof{}, LoadSources: map[string]epathRealSQLSource{}}
			pairs := map[string][12]epathSQLConversionProof{}
			for _, carrier := range direct.Carriers {
				proof.Carriers[carrier] = epathSQLQuantity{}
			}
			for _, month := range epathSQLPeriodMonths(period) {
				row := direct.Monthly[month-1]
				monthDirect, total := epathSQLQuantity{}, epathSQLQuantity{}
				for _, carrier := range direct.Carriers {
					share := row.Zones[zone][carrier]
					basis, q := service.Basis, share.Allocated
					originals := map[string]epathSQLOriginalSource{}
					required := map[string]bool{}
					if share.ObservedDirect {
						basis, q = "direct_zone_energy", share.Direct
						for id, source := range share.DirectSources {
							originals[id] = source
							if source.DirectHVAC != nil && source.DirectHVAC.Source.Months[month-1].EnergyKWh != nil && *source.DirectHVAC.Source.Months[month-1].EnergyKWh > 0 {
								required[id] = true
							}
						}
						monthDirect = monthDirect.add(q)
					} else {
						for id, source := range direct.BroadSources[carrier] {
							originals[id] = source
							if q.Value > 0 && frames.SourceRaw[source.RDD.DictionaryIndex][month-1].Value > 0 {
								required[id] = true
							}
						}
						// Exact same-carrier direct observations justify the pool
						// subtraction; they are context, never extra Zone consumption.
						for _, other := range row.Zones {
							for id, source := range other[carrier].DirectSources {
								originals[id] = source
								if q.Value > 0 {
									required[id] = true
								}
							}
						}
					}
					relation := "end_use_to_carrier"
					if frames.Loads[epathSQLKey(zone, service.Service, month)].Value == 0 {
						relation = "direct_end_use_to_carrier"
					}
					key := basis + "|" + carrier + "|" + relation
					branch := proof.Branches[key]
					if branch.Originals == nil {
						branch = epathSQLDirectHVACBranchProof{Basis: basis, Carrier: carrier, Relation: relation, Originals: map[string]epathSQLOriginalSource{}, Required: map[string]bool{}}
					}
					branch.Quantity = branch.Quantity.add(q)
					for id, source := range originals {
						branch.Originals[id] = source
					}
					for id := range required {
						branch.Required[id] = true
					}
					proof.Branches[key] = branch
					proof.Carriers[carrier] = proof.Carriers[carrier].add(q)
					total = total.add(q)
				}
				load := frames.Loads[epathSQLKey(zone, service.Service, month)]
				basis := service.Basis
				if monthDirect.Value > 0 {
					basis = "direct_zone_energy"
					proof.Basis = basis
				}
				if load.Value > 0 && total.Value > 0 {
					from, to := load.positive(), total.positive()
					if from.includesZero() || to.includesZero() {
						from, to = from.optionalPresentation(), to.optionalPresentation()
					}
					values := pairs[basis]
					values[month-1] = epathSQLConversionProof{From: from, To: to}
					pairs[basis] = values
				}
				ids := frames.LoadSourceIDs[epathSQLKey(zone, service.Service, month)]
				if len(ids) == 0 {
					return fmt.Errorf("direct service is missing its independent Zone-load authority")
				}
				for _, index := range ids {
					source, ok := frames.SourceIdentities[index]
					if !ok || index <= 0 || source.DictionaryIndex != index || source.IsMeter || !strings.EqualFold(source.KeyValue, zone) || source.ReportingFrequency != "Monthly" || source.SourceUnit != "J" {
						return fmt.Errorf("direct service contains a foreign/invalid Zone load")
					}
					proof.LoadSources[fmt.Sprintf("sql-rdd-%d", index)] = source
				}
			}
			value := epathSQLQuantity{}
			for _, q := range proof.Carriers {
				value = value.add(q)
			}
			for _, field := range []string{"value", "allocatedValue"} {
				target := epathSQLNodeTarget("end_use", service.Service, "", "site")
				target.Field, target.Basis, target.AllowPrunedZero = field, proof.Basis, true
				if err := checks.add("zoneAllocation", "zone", frames.Zones[zone].Name, period, "hvac_zone/"+service.Service+"/"+field, "kWh", &value, target, "", nil, nil); err != nil {
					return err
				}
				if field == "value" {
					checks.Rows[len(checks.Rows)-1].ZoneService = proof
				}
			}
			for _, basis := range []string{"direct_zone_energy", service.Basis, service.FallbackBasis} {
				from, to := epathSQLQuantity{}, epathSQLQuantity{}
				for _, month := range epathSQLPeriodMonths(period) {
					from = from.add(pairs[basis][month-1].From)
					to = to.add(pairs[basis][month-1].To)
				}
				kind := service.RatioKind
				if basis == service.FallbackBasis {
					kind = service.FallbackRatioKind
				}
				kind, err := epathSQLConversionPeriodRatioKind(kind, period, pairs[basis])
				var exact *epathSQLConversionProof
				if err != nil && basis == "direct_zone_energy" && period != "annual" {
					month := epathSQLPeriodMonths(period)[0]
					exact, err = epathSQLDirectHVACDisplayedPair(frames, model, zone, service.Service, month)
					if err == nil {
						kind, err = epathSQLConversionRatioKind(service.RatioKind, exact.From, exact.To)
					}
				}
				if err != nil {
					return fmt.Errorf("direct %s/%s/%s/%s ratio kind: %w", zone, service.Service, period, basis, err)
				}
				var ratio *epathSQLQuantity
				if from.Value > 0 && to.Value > 0 {
					ratio = &epathSQLQuantity{Value: from.Value / to.Value}
				}
				target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: service.Service, Basis: basis, FromUnit: "kWh", ToUnit: "kWh", RatioKind: kind, Aggregate: "sum"}
				if err := checks.add("ratios", "zone", frames.Zones[zone].Name, period, "hvac_zone/"+service.Service+"/"+basis, "ratio", ratio, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].Conversion = &epathSQLConversionProof{From: from, To: to, ExactPresentation: exact}
			}
		}
	}
	return nil
}

func epathSQLDirectHVACSourceMap(bundle PurposeResultBundle) (map[string]EnergyDataSource, error) {
	out := map[string]EnergyDataSource{}
	for _, source := range bundle.EnergyExplanation.Sources {
		if _, duplicate := out[source.ID]; source.ID == "" || duplicate {
			return nil, fmt.Errorf("duplicate/empty original source")
		}
		out[source.ID] = source
	}
	return out, nil
}
func epathSQLDirectHVACZoneCoveredLink(nodes []EnergyExplanationNode, link EnergyPathLink, proof *epathSQLZoneServiceProof) bool {
	if proof == nil || !proof.DirectHVAC || link.Relation != "end_use_to_carrier" && link.Relation != "direct_end_use_to_carrier" || link.ServiceKind != "" && link.ServiceKind != proof.Service || link.FromUnit != "kWh" || link.ToUnit != "kWh" {
		return false
	}
	var from, to EnergyExplanationNode
	for _, node := range nodes {
		if node.ID == link.FromID {
			from = node
		}
		if node.ID == link.ToID {
			to = node
		}
	}
	branch, ok := proof.Branches[link.Basis+"|"+to.Carrier+"|"+link.Relation]
	return ok && branch.Basis == link.Basis && branch.Carrier == to.Carrier && branch.Relation == link.Relation && from.Level == "end_use" && from.EndUse == proof.Service && (from.ServiceKind == "" || from.ServiceKind == proof.Service) && from.Basis == proof.Basis && to.Level == "carrier" && from.ScaleDomain == "site" && to.ScaleDomain == "site" && from.Unit == "kWh" && to.Unit == "kWh" && strings.EqualFold(from.ZoneName, to.ZoneName) && strings.EqualFold(link.ZoneName, from.ZoneName) && from.Period == to.Period && link.Period == from.Period
}

func epathCheckSQLDirectHVACZoneService(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof := check.ZoneService
	if proof == nil || !proof.DirectHVAC || proof.AnnualTabular || check.Item.Scope != "zone" || check.Item.Target.Collection != "nodes" || check.Item.Target.Level != "end_use" || check.Item.Target.Category != proof.Service || check.Item.Target.Basis != proof.Basis || check.Item.Target.Field != "value" || check.Quantity == nil || len(proof.LoadSources) == 0 || len(proof.Carriers) == 0 {
		return fmt.Errorf("invalid exact direct-first Zone service proof")
	}
	byCarrier := map[string]epathSQLQuantity{}
	sum := epathSQLQuantity{}
	allowed := map[string]epathSQLOriginalSource{}
	required := map[string]bool{}
	for key, branch := range proof.Branches {
		if key != branch.Basis+"|"+branch.Carrier+"|"+branch.Relation || branch.Relation != "end_use_to_carrier" && branch.Relation != "direct_end_use_to_carrier" || branch.Basis != "direct_zone_energy" && branch.Basis != "service_path_allocation" || branch.Carrier == "" || !branch.Quantity.valid() || branch.Quantity.Value < 0 || len(branch.Originals) == 0 {
			return fmt.Errorf("invalid carrier-qualified direct/allocated branch")
		}
		byCarrier[branch.Carrier] = byCarrier[branch.Carrier].add(branch.Quantity)
		for id, source := range branch.Originals {
			if branch.Basis == "direct_zone_energy" && (source.DirectHVAC == nil || source.DirectHVAC.Service != proof.Service || source.DirectHVAC.Carrier != branch.Carrier || !strings.EqualFold(source.DirectHVAC.Owner.ZoneName, check.Item.Zone)) {
				return fmt.Errorf("direct branch has a broad or foreign-owner source")
			}
			allowed[id] = source
		}
		for id, must := range branch.Required {
			if _, ok := branch.Originals[id]; !ok || !must {
				return fmt.Errorf("invalid independently required branch source")
			}
			required[id] = true
		}
	}
	if len(byCarrier) != len(proof.Carriers) {
		return fmt.Errorf("missing direct-first carrier branch proof")
	}
	for carrier, q := range proof.Carriers {
		if !epathSQLZoneCarrierQuantityEqual(q, byCarrier[carrier]) {
			return fmt.Errorf("carrier branch quantities contradict end-use proof")
		}
		sum = sum.add(q)
	}
	if !epathSQLZoneCarrierQuantityEqual(sum, *check.Quantity) {
		return fmt.Errorf("direct-first proof contradicts selected quantity")
	}
	loadRequired := map[string]bool{}
	for id, source := range proof.LoadSources {
		if source.IsMeter || !strings.EqualFold(source.KeyValue, check.Item.Zone) {
			return fmt.Errorf("foreign direct-service load source")
		}
		allowed[id] = epathSQLOriginalRDD(source)
		loadRequired[id] = true
	}
	sources, err := epathSQLDirectHVACSourceMap(bundle)
	if err != nil {
		return err
	}
	nodes, links, _, _, err := epathOracleGraph(bundle, "zone", check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	uses := map[string]bool{}
	for _, node := range nodes {
		if node.Level != "end_use" || node.EndUse != proof.Service && node.ServiceKind != proof.Service {
			continue
		}
		if node.EndUse != proof.Service || node.ServiceKind != "" && node.ServiceKind != proof.Service || node.Basis != proof.Basis || node.Unit != "kWh" || node.ScaleDomain != "site" || !strings.EqualFold(node.ZoneName, check.Item.Zone) || node.Period != check.Item.Period {
			return fmt.Errorf("direct-first node metadata contradicts exact context")
		}
		uses[node.ID] = true
		if err := epathSQLVerifyOriginalSources(node.SourceIDs, sources, allowed, required, check.Item.Period); err != nil {
			return err
		}
	}
	if len(uses) > 1 || len(uses) == 0 && !sum.includesZero() {
		return fmt.Errorf("missing/duplicate direct-first canonical service node")
	}
	counts := map[string]int{}
	for _, link := range links {
		if !uses[link.FromID] && !uses[link.ToID] {
			continue
		}
		if link.Relation == "source_correspondence" {
			continue
		}
		if link.Relation == "load_to_end_use" && uses[link.ToID] && link.ServiceKind == proof.Service {
			union := map[string]bool{}
			for id := range required {
				union[id] = true
			}
			for id := range loadRequired {
				union[id] = true
			}
			if err := epathSQLVerifyOriginalSources(link.SourceIDs, sources, allowed, union, check.Item.Period); err != nil {
				return err
			}
			continue
		}
		if !epathSQLDirectHVACZoneCoveredLink(nodes, link, proof) {
			return fmt.Errorf("unreviewed direct-first carrier link %s", link.ID)
		}
		carrier := ""
		for _, node := range nodes {
			if node.ID == link.ToID {
				carrier = node.Carrier
			}
		}
		key := link.Basis + "|" + carrier + "|" + link.Relation
		branch := proof.Branches[key]
		counts[key]++
		branchAllowed := map[string]epathSQLOriginalSource{}
		for id, source := range branch.Originals {
			branchAllowed[id] = source
		}
		if err := epathSQLVerifyOriginalSources(link.SourceIDs, sources, branchAllowed, branch.Required, check.Item.Period); err != nil {
			return err
		}
		if counts[key] != 1 || link.FromValue != link.ToValue || link.FromValue < 0 || link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" {
			return fmt.Errorf("duplicate/nonconserving direct-first site branch")
		}
		if err := epathCheckSQLModelQuantity(&link.FromValue, &branch.Quantity); err != nil {
			return err
		}
	}
	for key, branch := range proof.Branches {
		if counts[key] == 0 && !branch.Quantity.includesZero() {
			return fmt.Errorf("missing required direct-first carrier branch %s", key)
		}
	}
	return nil
}
