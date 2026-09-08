package simulation

import (
	"fmt"
	"reflect"
	"strings"
)

// Annual tabular observations have no monthly energy axis. Allocation uses
// only the reviewed owners' annual loads; the twelve monthly obligations are
// explicit unavailable/absence proofs, not fabricated zero observations.
func epathSQLAnnualZoneServiceChecks(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService, served map[string]bool, zones []string, sites map[string]epathRealSQLSite, checks *epathSQLModelChecks) error {
	denominator, err := epathSQLAnnualServedLoad(frames, served, service.Service)
	if err != nil {
		return err
	}
	pools := map[string]epathSQLQuantity{}
	originals := map[string]map[string]epathSQLOriginalSource{}
	used := map[string]bool{}
	for _, id := range service.SiteIDs {
		site, ok := sites[id]
		observation, observed := frames.SiteAnnual[id]
		if !ok || !observed || site.Facility || site.EndUse != service.Service || site.Carrier == "" {
			return fmt.Errorf("annual Zone service has contradictory source ownership: %s", id)
		}
		original, err := epathSQLOriginalTabular(site, observation)
		if err != nil {
			return err
		}
		key, err := epathSQLOriginalKey(original)
		if err != nil || used[key] {
			return fmt.Errorf("duplicate/invalid annual Zone service source: %s", id)
		}
		used[key] = true
		q, err := epathSQLSitePeriod(frames, id, "annual")
		if err != nil || q == nil || !q.valid() || q.Value < 0 {
			return fmt.Errorf("unknown annual Zone site pool: %s", id)
		}
		pools[site.Carrier] = pools[site.Carrier].add(*q)
		if originals[site.Carrier] == nil {
			originals[site.Carrier] = map[string]epathSQLOriginalSource{}
		}
		originals[site.Carrier][key] = original
	}
	for _, zone := range zones {
		load, err := epathSQLAnnualServedLoad(frames, map[string]bool{zone: true}, service.Service)
		if err != nil {
			return err
		}
		loadSources := map[string]epathRealSQLSource{}
		loadDetails := map[string]epathSQLLoadDetailIdentity{}
		for month := 1; month <= 12; month++ {
			ids := frames.LoadSourceIDs[epathSQLKey(zone, service.Service, month)]
			if len(ids) == 0 {
				return fmt.Errorf("annual Zone load has no observed source")
			}
			for _, id := range ids {
				source, ok := frames.SourceIdentities[id]
				if !ok || id <= 0 || source.DictionaryIndex != id || source.IsMeter || !strings.EqualFold(source.KeyValue, zone) || !strings.EqualFold(source.ReportingFrequency, "Monthly") {
					return fmt.Errorf("invalid annual Zone load owner")
				}
				loadSources[fmt.Sprintf("sql-rdd-%d", id)] = source
			}
			for _, id := range frames.LoadDetailSourceIDs[epathSQLKey(zone, service.Service, month)] {
				detail, found := frames.LoadDetailIdentities[id]
				if !found || epathSQLValidateLoadDetailIdentity(detail) != nil || detail.Source.DictionaryIndex != id || !strings.EqualFold(detail.ZoneName, zone) || detail.Service != service.Service || !reflect.DeepEqual(frames.SourceIdentities[id], detail.Source) {
					return fmt.Errorf("invalid annual Zone non-additive load detail owner/source")
				}
				key := fmt.Sprintf("sql-rdd-%d", id)
				if _, primary := loadSources[key]; primary {
					return fmt.Errorf("non-additive detail overlaps selected primary load")
				}
				loadDetails[key] = detail
			}
		}
		for _, period := range epathSQLZoneCarrierPeriods() {
			proof := &epathSQLZoneServiceProof{Service: service.Service, Basis: service.Basis, AnnualTabular: true, Unavailable: period != "annual", Unowned: !served[zone], Carriers: map[string]epathSQLQuantity{}, AnnualCarrierSources: originals, RequiredSites: map[string]map[string]bool{}, LoadSources: loadSources}
			if !proof.Unavailable {
				proof.LoadDetails = loadDetails
			}
			value, pair := epathSQLQuantity{}, epathSQLConversionProof{}
			for carrier, pool := range pools {
				proof.RequiredSites[carrier] = map[string]bool{}
				if proof.Unavailable {
					continue
				}
				share := epathSQLQuantity{}
				if !proof.Unowned {
					if denominator.Value == 0 {
						_, high := denominator.bounds()
						if high != 0 {
							return fmt.Errorf("uncertain annual served denominator")
						}
					} else {
						share, err = epathSQLShare(pool, load, denominator, model.Precision)
						if err != nil {
							return err
						}
					}
				}
				proof.Carriers[carrier] = share
				value = value.add(share)
				if !share.includesZero() {
					for key := range originals[carrier] {
						proof.RequiredSites[carrier][key] = true
					}
				}
			}
			if !proof.Unavailable && !proof.Unowned && load.Value > 0 && value.Value > 0 {
				pair = epathSQLConversionProof{From: load.positive(), To: value.positive()}
				if pair.From.includesZero() || pair.To.includesZero() {
					pair.From = pair.From.optionalPresentation()
					pair.To = pair.To.optionalPresentation()
				}
			}
			for _, field := range []string{"value", "allocatedValue"} {
				target := epathSQLNodeTarget("end_use", service.Service, "", "site")
				target.Field, target.Basis, target.AllowPrunedZero = field, service.Basis, !proof.Unavailable
				var q *epathSQLQuantity
				if !proof.Unavailable {
					q = &value
				}
				if err := checks.add("zoneAllocation", "zone", frames.Zones[zone].Name, period, "hvac_zone/"+service.Service+"/"+field, "kWh", q, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].ZoneService = proof
			}
			for _, basis := range []string{service.Basis, service.FallbackBasis} {
				selected, kind := pair, service.RatioKind
				if basis == service.FallbackBasis {
					selected = epathSQLConversionProof{}
					kind = service.FallbackRatioKind
				}
				kind, err = epathSQLConversionRatioKind(kind, selected.From, selected.To)
				if err != nil {
					return err
				}
				var ratio *epathSQLQuantity
				if selected.From.Value > 0 && selected.To.Value > 0 {
					ratio = &epathSQLQuantity{Value: selected.From.Value / selected.To.Value}
				}
				target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: service.Service, Basis: basis, FromUnit: "kWh", ToUnit: "kWh", RatioKind: kind, Aggregate: "sum"}
				if err := checks.add("ratios", "zone", frames.Zones[zone].Name, period, "hvac_zone/"+service.Service+"/"+basis, "ratio", ratio, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].Conversion = &selected
			}
		}
	}
	return nil
}

func epathCheckSQLAnnualZoneService(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p, t := check.ZoneService, check.Item.Target
	if p == nil || !p.AnnualTabular || check.Item.Scope != "zone" || check.Item.Zone == "" || (p.Service != "heating" && p.Service != "cooling") || p.Basis != "service_path_allocation" || t.Collection != "nodes" || t.Level != "end_use" || t.Category != p.Service || t.Basis != p.Basis || t.Unit != "kWh" || t.ScaleDomain != "site" || (t.Field != "value" && t.Field != "allocatedValue") || p.Unavailable != (check.Item.Period != "annual") || len(p.AnnualCarrierSources) == 0 || len(p.LoadSources) == 0 {
		return fmt.Errorf("invalid annual-only Zone service proof")
	}
	all := map[string]epathSQLOriginalSource{}
	details := map[string]epathSQLOriginalSource{}
	if p.Unavailable && len(p.LoadDetails) != 0 {
		return fmt.Errorf("monthly unavailable service gained annual-only trace allowance")
	}
	for id, detail := range p.LoadDetails {
		if !strings.EqualFold(detail.ZoneName, check.Item.Zone) || detail.Service != p.Service {
			return fmt.Errorf("annual service non-additive detail has wrong Zone/service")
		}
		original, err := epathSQLOriginalLoadDetail(detail)
		if err != nil {
			return err
		}
		key, err := epathSQLOriginalKey(original)
		if err != nil || key != id {
			return fmt.Errorf("annual detail lost its exact original identity")
		}
		if _, primary := p.LoadSources[id]; primary {
			return fmt.Errorf("annual detail cannot replace selected canonical load authority")
		}
		details[id] = original
	}
	required := map[string]bool{}
	for carrier, originals := range p.AnnualCarrierSources {
		if carrier == "" || len(originals) == 0 {
			return fmt.Errorf("empty annual carrier source")
		}
		for key, original := range originals {
			actualKey, err := epathSQLOriginalKey(original)
			if err != nil || actualKey != key || original.Tabular == nil || all[key].Tabular != nil || original.site.Carrier != carrier || original.site.EndUse != p.Service || original.site.Facility {
				return fmt.Errorf("invalid/duplicate annual original source")
			}
			if !p.Unavailable && !p.Carriers[carrier].includesZero() && !original.Tabular.Quantity.includesZero() && !p.RequiredSites[carrier][key] {
				return fmt.Errorf("positive annual carrier lost its required original source proof")
			}
			all[key] = original
		}
		for key, must := range p.RequiredSites[carrier] {
			if !must || originals[key].Tabular == nil {
				return fmt.Errorf("unbound required annual source")
			}
			required[key] = true
		}
	}
	if p.Unavailable {
		if check.Quantity != nil || check.Want.Value != nil || check.Want.Status != "unavailable" || len(p.Carriers) != 0 {
			return fmt.Errorf("monthly unreported energy was converted to zero")
		}
	} else {
		sum := epathSQLQuantity{}
		if len(p.Carriers) != len(p.AnnualCarrierSources) {
			return fmt.Errorf("incomplete annual carrier proof")
		}
		for carrier, q := range p.Carriers {
			if p.AnnualCarrierSources[carrier] == nil || !q.valid() || q.Value < 0 {
				return fmt.Errorf("invalid annual carrier quantity")
			}
			sum = sum.add(q)
		}
		if check.Quantity == nil || !epathSQLZoneCarrierQuantityEqual(sum, *check.Quantity) {
			return fmt.Errorf("annual carrier sum contradicts selected quantity")
		}
		if p.Unowned && (sum.Value != 0 || !sum.includesZero()) {
			return fmt.Errorf("unowned Zone gained an annual allocation")
		}
	}
	nodes, links, rows, _, err := epathOracleGraph(bundle, "zone", check.Item.Zone, check.Item.Period)
	if err != nil {
		return err
	}
	byID := map[string]EnergyExplanationNode{}
	uses := map[string]bool{}
	for _, n := range nodes {
		byID[n.ID] = n
		if n.Level == "end_use" && (n.EndUse == p.Service || n.ServiceKind == p.Service) {
			uses[n.ID] = true
		}
	}
	if len(uses) > 1 {
		return fmt.Errorf("duplicate canonical annual Zone service end use")
	}
	if p.Unavailable || p.Unowned {
		if len(uses) > 0 {
			return fmt.Errorf("unreported/unowned service gained an end-use node")
		}
		for _, l := range links {
			if l.Relation == "load_to_end_use" && (l.ServiceKind == p.Service || byID[l.FromID].ServiceKind == p.Service || byID[l.ToID].EndUse == p.Service) {
				return fmt.Errorf("unreported/unowned service gained a conversion")
			}
		}
		for _, r := range rows {
			if r.Level == "allocation" && (r.ServiceKind == p.Service || strings.Contains(r.ID, "zone_hvac_allocation."+p.Service+".")) {
				return fmt.Errorf("unreported/unowned service gained allocation accounting")
			}
		}
		return nil
	}
	sources := map[string]EnergyDataSource{}
	for _, s := range bundle.EnergyExplanation.Sources {
		if s.ID == "" || sources[s.ID].ID != "" {
			return fmt.Errorf("duplicate original source ID")
		}
		sources[s.ID] = s
	}
	conversionRequired := map[string]bool{}
	for k, v := range required {
		conversionRequired[k] = v
	}
	for id, source := range p.LoadSources {
		if source.IsMeter || !strings.EqualFold(source.KeyValue, check.Item.Zone) {
			return fmt.Errorf("annual load ownership contradiction")
		}
		original := epathSQLOriginalRDD(source)
		key, err := epathSQLOriginalKey(original)
		if err != nil || key != id {
			return fmt.Errorf("invalid annual load source")
		}
		all[key] = original
		conversionRequired[key] = true
	}
	for id, original := range details {
		if _, exists := all[id]; exists {
			return fmt.Errorf("duplicate annual trace source classes")
		}
		all[id] = original
	}
	for id := range uses {
		n := byID[id]
		if n.EndUse != p.Service || n.ServiceKind != "" && n.ServiceKind != p.Service || n.Basis != p.Basis || n.Unit != "kWh" || n.ScaleDomain != "site" || n.Period != check.Item.Period || !strings.EqualFold(n.ZoneName, check.Item.Zone) {
			return fmt.Errorf("annual service node metadata contradiction")
		}
		if err := epathSQLVerifyOriginalSources(n.SourceIDs, sources, all, required, "annual"); err != nil {
			return err
		}
	}
	sums := map[string]float64{}
	counts := map[string]int{}
	for _, l := range links {
		if !uses[l.FromID] && !uses[l.ToID] {
			continue
		}
		if l.Relation == "source_correspondence" {
			continue
		}
		if l.Relation == "load_to_end_use" && uses[l.ToID] && l.ServiceKind == p.Service && l.Basis == p.Basis {
			if err := epathSQLVerifyOriginalSources(l.SourceIDs, sources, all, conversionRequired, "annual"); err != nil {
				return err
			}
			continue
		}
		if !epathSQLZoneServiceCoveredLink(nodes, l, p) {
			return fmt.Errorf("unreviewed annual carrier branch")
		}
		carrier := byID[l.ToID].Carrier
		if err := epathSQLVerifyOriginalSources(l.SourceIDs, sources, p.AnnualCarrierSources[carrier], p.RequiredSites[carrier], "annual"); err != nil {
			return err
		}
		if !epathOracleFinite(l.FromValue) || l.FromValue < 0 || l.FromValue != l.ToValue || l.Ratio != 0 || l.RatioKind != "" || l.RatioLabel != "" {
			return fmt.Errorf("invalid annual site branch quantities")
		}
		sums[carrier] += l.FromValue
		counts[carrier]++
	}
	for carrier, q := range p.Carriers {
		if counts[carrier] == 0 && q.includesZero() {
			continue
		}
		v := sums[carrier]
		if err := epathCheckSQLModelQuantity(&v, &q); err != nil {
			return err
		}
	}
	return nil
}
