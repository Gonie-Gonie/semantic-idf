package simulation

import (
	"fmt"
	"sort"
	"strings"
)

// Quantities come only from reviewed SQL pool identities and their declared
// served-Zone loads. Carrier endpoints are not inferred from candidate totals.
type epathSQLZoneServiceProof struct {
	Service, Basis string
	Carriers       map[string]epathSQLQuantity
	CarrierSources map[string]map[string]epathRealSQLSource
	RequiredSites  map[string]map[string]bool
	LoadSources    map[string]epathRealSQLSource
}

func epathSQLModelZoneServiceChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	if model.Precision.DecimalPlaces != 3 || model.Precision.SourceStages < 1 || model.Precision.SourceStages > 3 || model.Precision.ContributionStages < 1 || model.Precision.ContributionStages > 3 {
		return fmt.Errorf("Zone service proof requires reviewed source/contribution precision")
	}
	zones := []string{}
	for key, zone := range frames.Zones {
		if key == "" || key != strings.ToLower(zone.Name) {
			return fmt.Errorf("invalid exact Zone identity in service frames")
		}
		zones = append(zones, key)
	}
	sort.Strings(zones)
	if len(zones) == 0 {
		return fmt.Errorf("Zone service proof requires observed Zones")
	}
	sites := map[string]epathRealSQLSite{}
	for _, site := range model.Site {
		if site.ID == "" || sites[site.ID].ID != "" {
			return fmt.Errorf("duplicate/empty declared site identity")
		}
		sites[site.ID] = site
	}
	seen := map[string]bool{}
	for _, service := range model.Services {
		if (service.Service != "cooling" && service.Service != "heating") || seen[service.Service] || service.Basis != "service_path_allocation" || service.FallbackBasis != "zone_load_allocation" || service.RatioKind == "" || service.FallbackRatioKind == "" {
			return fmt.Errorf("unsupported/duplicate reviewed Zone service")
		}
		seen[service.Service] = true
		served, err := epathSQLDeclaredZones(frames, service.ServedZones)
		if err != nil || len(served) == 0 {
			return fmt.Errorf("Zone service requires exact nonempty served membership: %v", err)
		}
		poolIDs := append([]string(nil), service.SiteIDs...)
		sort.Strings(poolIDs)
		carriers, used := map[string]bool{}, map[string]bool{}
		carrierSources := map[string]map[string]epathRealSQLSource{}
		sourceOwners := map[int]string{}
		for _, id := range poolIDs {
			site, ok := sites[id]
			values := frames.Site[id]
			if !ok || used[id] || site.Facility || site.EndUse != service.Service || site.Carrier == "" || len(values) != 12 {
				return fmt.Errorf("service %s has ambiguous/incompatible site identity %s", service.Service, id)
			}
			used[id], carriers[site.Carrier] = true, true
			if len(frames.SiteSources[id]) == 0 {
				return fmt.Errorf("site %s lacks original SQL source identities", id)
			}
			if carrierSources[site.Carrier] == nil {
				carrierSources[site.Carrier] = map[string]epathRealSQLSource{}
			}
			for _, index := range frames.SiteSources[id] {
				source, ok := frames.SourceIdentities[index]
				if !ok || source.DictionaryIndex != index || index <= 0 || sourceOwners[index] != "" || source.Name == "" || source.SourceUnit == "" || !strings.EqualFold(source.ReportingFrequency, "Monthly") || len(frames.SourceRaw[index]) != 12 {
					return fmt.Errorf("unknown/duplicate original site-source identity for %s", id)
				}
				for _, q := range frames.SourceRaw[index] {
					if !q.valid() || q.Value < 0 {
						return fmt.Errorf("site source %d has unknown/negative monthly observation", index)
					}
				}
				sourceOwners[index] = id
				carrierSources[site.Carrier][fmt.Sprintf("sql-rdd-%d", index)] = source
			}
			for _, value := range values {
				if value == nil || !value.valid() || value.Value < 0 {
					return fmt.Errorf("service site %s has missing/nonfinite/negative monthly energy", id)
				}
			}
		}
		if len(poolIDs) == 0 {
			return fmt.Errorf("service has no reviewed site pool")
		}
		// Each carrier remains separate through monthly allocation. Annual
		// values sum these completed shares, never annual load fractions.
		allocated := map[string]map[string][12]epathSQLQuantity{}
		fromValues, toValues := map[string][12]epathSQLQuantity{}, map[string][12]epathSQLQuantity{}
		for _, zone := range zones {
			allocated[zone] = map[string][12]epathSQLQuantity{}
		}
		for month := 1; month <= 12; month++ {
			denominator := epathSQLQuantity{}
			for _, zone := range zones {
				load, ok := frames.Loads[epathSQLKey(zone, service.Service, month)]
				if !ok || !load.valid() || load.Value < 0 {
					return fmt.Errorf("unknown monthly %s load for %s M%d is not zero", service.Service, zone, month)
				}
				if served[zone] {
					denominator = denominator.add(load)
				}
			}
			if denominator.Value == 0 {
				_, high := denominator.bounds()
				if high != 0 {
					return fmt.Errorf("uncertain served-load denominator cannot prove unassigned energy")
				}
				// The independently checked Building allocation ledger retains
				// the entire pool as unassigned. No plenum fallback is authorized.
				continue
			}
			for _, zone := range zones {
				if !served[zone] {
					continue
				}
				load := frames.Loads[epathSQLKey(zone, service.Service, month)]
				total := epathSQLQuantity{}
				for _, id := range poolIDs {
					share, err := epathSQLShare(*frames.Site[id][month-1], load, denominator, model.Precision)
					if err != nil {
						return fmt.Errorf("%s/%s/M%d: %w", service.Service, zone, month, err)
					}
					carrier := sites[id].Carrier
					values := allocated[zone][carrier]
					values[month-1] = values[month-1].add(share)
					allocated[zone][carrier] = values
					total = total.add(share)
				}
				if load.Value > 0 && total.Value > 0 {
					from, to := load.positive(), total.positive()
					if from.includesZero() || to.includesZero() {
						from, to = from.optionalPresentation(), to.optionalPresentation()
					}
					fromMonths, toMonths := fromValues[zone], toValues[zone]
					fromMonths[month-1], toMonths[month-1] = from, to
					fromValues[zone], toValues[zone] = fromMonths, toMonths
				}
			}
		}
		for _, zone := range zones {
			for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
				proof := &epathSQLZoneServiceProof{Service: service.Service, Basis: service.Basis, Carriers: map[string]epathSQLQuantity{}, CarrierSources: carrierSources, RequiredSites: map[string]map[string]bool{}, LoadSources: map[string]epathRealSQLSource{}}
				value, from, to := epathSQLQuantity{}, epathSQLQuantity{}, epathSQLQuantity{}
				carrierNames := []string{}
				for carrier := range carriers {
					carrierNames = append(carrierNames, carrier)
				}
				sort.Strings(carrierNames)
				for _, carrier := range carrierNames {
					q := epathSQLQuantity{}
					proof.RequiredSites[carrier] = map[string]bool{}
					for _, month := range epathSQLPeriodMonths(period) {
						q = q.add(allocated[zone][carrier][month-1])
						if allocated[zone][carrier][month-1].Value > 0 {
							for id, source := range carrierSources[carrier] {
								if frames.SourceRaw[source.DictionaryIndex][month-1].Value > 0 {
									proof.RequiredSites[carrier][id] = true
								}
							}
						}
					}
					proof.Carriers[carrier] = q
					value = value.add(q)
				}
				for _, month := range epathSQLPeriodMonths(period) {
					from = from.add(fromValues[zone][month-1])
					to = to.add(toValues[zone][month-1])
					ids := frames.LoadSourceIDs[epathSQLKey(zone, service.Service, month)]
					if len(ids) == 0 {
						return fmt.Errorf("service %s/%s M%d lacks original Zone-load source", service.Service, zone, month)
					}
					seenLoad := map[int]bool{}
					for _, index := range ids {
						source, ok := frames.SourceIdentities[index]
						if !ok || index <= 0 || source.DictionaryIndex != index || seenLoad[index] || source.IsMeter || !strings.EqualFold(source.KeyValue, zone) || source.Name == "" || source.SourceUnit == "" || !strings.EqualFold(source.ReportingFrequency, "Monthly") {
							return fmt.Errorf("invalid original Zone/service load source %d", index)
						}
						seenLoad[index] = true
						proof.LoadSources[fmt.Sprintf("sql-rdd-%d", index)] = source
					}
				}
				for _, field := range []string{"value", "allocatedValue"} {
					target := epathSQLNodeTarget("end_use", service.Service, "", "site")
					target.Field, target.Basis, target.AllowPrunedZero = field, service.Basis, true
					if err := checks.add("zoneAllocation", "zone", frames.Zones[zone].Name, period, "hvac_zone/"+service.Service+"/"+field, "kWh", &value, target, "", nil, nil); err != nil {
						return err
					}
					if field == "value" {
						checks.Rows[len(checks.Rows)-1].ZoneService = proof
					}
				}
				for _, basis := range []string{service.Basis, service.FallbackBasis} {
					pair := &epathSQLConversionProof{From: from, To: to}
					kind := service.RatioKind
					if basis == service.FallbackBasis {
						pair, kind = &epathSQLConversionProof{}, service.FallbackRatioKind
					}
					var ratio *epathSQLQuantity
					if pair.From.Value > 0 && pair.To.Value > 0 {
						ratio = &epathSQLQuantity{Value: pair.From.Value / pair.To.Value}
					}
					target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: service.Service, Basis: basis, FromUnit: "kWh", ToUnit: "kWh", RatioKind: kind, Aggregate: "sum"}
					if err := checks.add("ratios", "zone", frames.Zones[zone].Name, period, "hvac_zone/"+service.Service+"/"+basis, "ratio", ratio, target, "", nil, nil); err != nil {
						return err
					}
					checks.Rows[len(checks.Rows)-1].Conversion = pair
				}
			}
		}
	}
	if len(seen) != 2 {
		return fmt.Errorf("Zone service proof requires both reviewed services")
	}
	return nil
}

// This predicate is shared with required-record coverage. It covers only the
// declared carrier and both exact site-domain endpoints, not all outgoing links
// of a broadly matched end-use node.
func epathSQLZoneServiceCoveredLink(nodes []EnergyExplanationNode, link EnergyPathLink, proof *epathSQLZoneServiceProof) bool {
	if proof == nil || link.Relation != "end_use_to_carrier" || link.ServiceKind != "" && link.ServiceKind != proof.Service || link.Basis != proof.Basis || link.FromUnit != "kWh" || link.ToUnit != "kWh" {
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
	_, declared := proof.Carriers[to.Carrier]
	return declared && from.Level == "end_use" && from.EndUse == proof.Service && (from.ServiceKind == "" || from.ServiceKind == proof.Service) && from.Basis == proof.Basis && to.Level == "carrier" && from.ScaleDomain == "site" && to.ScaleDomain == "site" && from.Unit == "kWh" && to.Unit == "kWh" && strings.EqualFold(from.ZoneName, to.ZoneName) && from.Period == to.Period
}

func epathSQLZoneServiceVerifySources(ids []string, actual map[string]EnergyDataSource, allowed map[string]epathRealSQLSource, required map[string]bool) error {
	seen := map[string]bool{}
	for _, id := range ids {
		original, ok := allowed[id]
		source, exists := actual[id]
		if !ok || !exists || seen[id] || source.SourceType != "sql_report_data" || source.IsMeter != original.IsMeter || !strings.EqualFold(source.Name, original.Name) || !strings.EqualFold(source.KeyValue, original.KeyValue) || source.SourceUnit != original.SourceUnit || source.NormalizedUnit != "kWh" || !strings.EqualFold(source.ReportingFrequency, original.ReportingFrequency) {
			return fmt.Errorf("Zone service trace is not an approved exact SQL source: %s", id)
		}
		seen[id] = true
	}
	for id, mustExist := range required {
		if mustExist && !seen[id] {
			return fmt.Errorf("Zone service trace lost required original SQL source: %s", id)
		}
	}
	return nil
}

func epathCheckSQLZoneServiceEndpoints(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof := check.ZoneService
	if proof == nil || check.Item.Scope != "zone" || (proof.Service != "cooling" && proof.Service != "heating") || proof.Basis != "service_path_allocation" || check.Item.Target.Collection != "nodes" || check.Item.Target.Category != proof.Service || check.Item.Target.Basis != proof.Basis || len(proof.Carriers) == 0 || check.Quantity == nil || !check.Quantity.valid() {
		return fmt.Errorf("exact Zone service proof required")
	}
	expected := epathSQLQuantity{}
	for _, q := range proof.Carriers {
		if !q.valid() || q.Value < 0 {
			return fmt.Errorf("unknown/invalid carrier allocation proof")
		}
		expected = expected.add(q)
	}
	if err := epathCheckSQLModelQuantity(&expected.Value, check.Quantity); err != nil {
		return fmt.Errorf("carrier proof contradicts selected end-use quantity: %w", err)
	}
	nodes, links, _, _, err := epathOracleGraph(bundle, "zone", check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	uses, byID := map[string]bool{}, map[string]EnergyExplanationNode{}
	for _, node := range nodes {
		byID[node.ID] = node
		if node.Level != "end_use" || node.EndUse != proof.Service && node.ServiceKind != proof.Service {
			continue
		}
		if node.EndUse != proof.Service || node.ServiceKind != "" && node.ServiceKind != proof.Service || node.Basis != proof.Basis || node.Unit != "kWh" || node.ScaleDomain != "site" {
			return fmt.Errorf("unreviewed Zone service node identity/basis %s", node.ID)
		}
		uses[node.ID] = true
	}
	sources := map[string]EnergyDataSource{}
	for _, source := range bundle.EnergyExplanation.Sources {
		if _, duplicate := sources[source.ID]; source.ID == "" || duplicate {
			return fmt.Errorf("ambiguous carrier-branch source identity")
		}
		sources[source.ID] = source
	}
	if len(proof.LoadSources) == 0 || len(proof.CarrierSources) != len(proof.Carriers) || len(proof.RequiredSites) != len(proof.Carriers) {
		return fmt.Errorf("Zone service lacks independently bound SQL source sets")
	}
	union, requiredSite, requiredConversion := map[string]epathRealSQLSource{}, map[string]bool{}, map[string]bool{}
	for carrier := range proof.Carriers {
		if len(proof.CarrierSources[carrier]) == 0 {
			return fmt.Errorf("carrier %s lacks original SQL source identities", carrier)
		}
		for id, source := range proof.CarrierSources[carrier] {
			if _, duplicate := union[id]; duplicate {
				return fmt.Errorf("SQL source ambiguously owns multiple carriers")
			}
			union[id] = source
		}
		for id, required := range proof.RequiredSites[carrier] {
			if _, ok := proof.CarrierSources[carrier][id]; !ok || !required {
				return fmt.Errorf("required SQL source contradicts declared carrier pool")
			}
			requiredSite[id], requiredConversion[id] = true, true
		}
	}
	for id, source := range proof.LoadSources {
		if _, duplicate := union[id]; duplicate || source.IsMeter || !strings.EqualFold(source.KeyValue, check.Item.Zone) {
			return fmt.Errorf("Zone-load source contradicts exact ownership")
		}
		union[id] = source
		requiredConversion[id] = true
	}
	for id := range uses {
		// End-use amounts are site quantities. Their trace may retain the
		// matching Zone load as allocation context, but never a foreign load.
		if err := epathSQLZoneServiceVerifySources(byID[id].SourceIDs, sources, union, requiredSite); err != nil {
			return err
		}
	}
	fromSums, toSums, counts := map[string]float64{}, map[string]float64{}, map[string]int{}
	for _, link := range links {
		if !uses[link.FromID] && !uses[link.ToID] {
			continue
		}
		if link.Relation == "source_correspondence" {
			continue
		}
		if link.Relation == "load_to_end_use" && uses[link.ToID] && link.ServiceKind == proof.Service && link.Basis == proof.Basis {
			if err := epathSQLZoneServiceVerifySources(link.SourceIDs, sources, union, requiredConversion); err != nil {
				return err
			}
			continue // Both independently bounded conversion quantities are checked separately.
		}
		if !epathSQLZoneServiceCoveredLink(nodes, link, proof) || len(link.SourceIDs) == 0 {
			return fmt.Errorf("unreviewed Zone service carrier branch %s", link.ID)
		}
		carrier := byID[link.ToID].Carrier
		if err := epathSQLZoneServiceVerifySources(link.SourceIDs, sources, proof.CarrierSources[carrier], proof.RequiredSites[carrier]); err != nil {
			return err
		}
		if !epathOracleFinite(link.FromValue) || !epathOracleFinite(link.ToValue) || link.FromValue < 0 || link.ToValue < 0 || link.FromValue != link.ToValue || link.Ratio != 0 || link.RatioKind != "" || link.RatioLabel != "" {
			return fmt.Errorf("invalid site/site branch quantity or fabricated conversion ratio")
		}
		fromSums[carrier] += link.FromValue
		toSums[carrier] += link.ToValue
		counts[carrier]++
	}
	for carrier, q := range proof.Carriers {
		if counts[carrier] == 0 {
			if !q.includesZero() {
				return fmt.Errorf("missing required %s service carrier branch", carrier)
			}
			continue
		}
		for _, sum := range []float64{fromSums[carrier], toSums[carrier]} {
			if err := epathCheckSQLModelQuantity(&sum, &q); err != nil {
				return fmt.Errorf("%s carrier-specific Zone allocation: %w", carrier, err)
			}
		}
	}
	return nil
}
