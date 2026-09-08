package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// A purchased, model-total meter is allocated, never multiplied into another
// observation. Sources justify consumption; NodeSources additionally justify
// this Zone's load weight. Neither map invents an individual pump meter.
type epathSQLAuxiliaryZoneProof struct {
	SiteID, EndUse, Carrier, ZoneName, Period, Basis, Weight, AllocationMethod string
	Value                                                                      epathSQLQuantity
	Sources, NodeSources                                                       map[string]epathSQLOriginalSource
	Required, NodeRequired                                                     map[string]bool
	Owned                                                                      bool
}

func epathSQLAuxiliaryZoneKey(siteID, zone, period string) string {
	return siteID + "\x00" + strings.ToLower(zone) + "\x00" + period
}

// Deliberately finite support. Fan pools have their own measured pool proof;
// unassigned auxiliaries create no Zone value. Unknown allocation policies do
// not gain permission merely by appearing in a recipe or a candidate.
func epathSQLAuxiliaryZoneProofs(frames epathSQLFrames, model epathRealSQLModel) (map[string]*epathSQLAuxiliaryZoneProof, error) {
	out := map[string]*epathSQLAuxiliaryZoneProof{}
	sites, fanSites := map[string]epathRealSQLSite{}, map[string]bool{}
	for _, site := range model.Site {
		if site.ID == "" || sites[site.ID].ID != "" {
			return nil, fmt.Errorf("auxiliary requires unique reviewed site identities")
		}
		sites[site.ID] = site
	}
	for _, pool := range model.FanPools {
		fanSites[pool.SiteID] = true
	}
	zones := []string{}
	for key, zone := range frames.Zones {
		if key == "" || key != strings.ToLower(zone.Name) || !epathOracleFinite(zone.Multiplier) || zone.Multiplier <= 0 {
			return nil, fmt.Errorf("auxiliary requires exact original Zone identities and factors")
		}
		zones = append(zones, key)
	}
	sort.Strings(zones)
	seen, endUses := map[string]bool{}, map[string]bool{}
	for _, auxiliary := range model.Auxiliaries {
		if auxiliary.SiteID == "" || seen[auxiliary.SiteID] {
			return nil, fmt.Errorf("duplicate/empty auxiliary site declaration")
		}
		seen[auxiliary.SiteID] = true
		if auxiliary.Weight == "unassigned" {
			if len(auxiliary.ServedZones) != 0 || auxiliary.AllocationMethod != "unassigned" {
				return nil, fmt.Errorf("unassigned auxiliary cannot invent served ownership")
			}
			continue
		}
		if fanSites[auxiliary.SiteID] {
			continue // Existing exact fan-pool proof remains the sole authority.
		}
		site, exists := sites[auxiliary.SiteID]
		if !exists || site.Facility || site.EndUse != "pumps" || site.Carrier != "electricity" || site.Tabular != nil || auxiliary.Weight != "cooling_plus_heating" || auxiliary.WeightSource != nil || auxiliary.AllocationMethod != "plant_loop_load_share" || endUses[site.EndUse] {
			return nil, fmt.Errorf("allocated auxiliary lacks an explicit supported site/carrier/weight proof: %s", auxiliary.SiteID)
		}
		endUses[site.EndUse] = true
		selector := epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Pumps:Electricity", Unit: "J"}}, Keys: []string{""}, IsMeter: true}
		if !reflect.DeepEqual(site.Source, selector) {
			return nil, fmt.Errorf("pump scalar requires its exact reviewed broad meter selector")
		}
		annual, err := epathSQLSiteIsAnnual(frames, site.ID)
		if err != nil || annual {
			return nil, fmt.Errorf("allocated pump scalar requires original Monthly observations: %v", err)
		}
		if _, err := epathSQLAllocationID(auxiliary.ReconciliationID, "annual"); err != nil {
			return nil, err
		}
		served, err := epathSQLDeclaredZones(frames, auxiliary.ServedZones)
		if err != nil || len(served) == 0 || len(zones) == 0 {
			return nil, fmt.Errorf("allocated auxiliary requires exact nonempty served membership: %v", err)
		}
		ids := frames.SiteSources[site.ID]
		if len(ids) != 1 || len(frames.Site[site.ID]) != 12 {
			return nil, fmt.Errorf("pump allocation requires one original twelve-month broad meter")
		}
		meter, exists := frames.SourceIdentities[ids[0]]
		if !exists || meter.DictionaryIndex != ids[0] || ids[0] <= 0 || !meter.IsMeter || meter.Name != "Pumps:Electricity" || meter.KeyValue != "" || meter.ReportingFrequency != "Monthly" || meter.SourceUnit != "J" || meter.MissingRows != 0 {
			return nil, fmt.Errorf("pump site has a contradictory original Monthly/J meter identity")
		}
		original, err := epathSQLMonthly(meter, model.Precision)
		if err != nil {
			return nil, err
		}
		for month, bucket := range meter.Months {
			value := frames.Site[site.ID][month]
			if bucket.MissingRows != 0 || value == nil || !value.valid() || value.Value < 0 || !epathSQLZoneCarrierQuantityEqual(*value, original[month]) {
				return nil, fmt.Errorf("pump model-total meter changed, was multiplied, or lost a monthly observation")
			}
		}
		shares := map[string][12]epathSQLQuantity{}
		for month := 1; month <= 12; month++ {
			weights, denominator := map[string]epathSQLQuantity{}, epathSQLQuantity{}
			for _, zone := range zones {
				for _, service := range []string{"cooling", "heating"} {
					load, exists := frames.Loads[epathSQLKey(zone, service, month)]
					if !exists || !load.valid() || load.Value < 0 {
						return nil, fmt.Errorf("auxiliary weight lacks a known %s/%s/M%d load", zone, service, month)
					}
					weights[zone] = weights[zone].add(load)
				}
				if served[zone] {
					denominator = denominator.add(weights[zone])
				}
			}
			if denominator.Value == 0 {
				_, high := denominator.bounds()
				if high != 0 {
					return nil, fmt.Errorf("uncertain auxiliary denominator cannot prove unassigned energy")
				}
				continue // The Building ledger keeps the pool unassigned.
			}
			for _, zone := range zones {
				if !served[zone] {
					continue
				}
				q, err := epathSQLShare(*frames.Site[site.ID][month-1], weights[zone], denominator, model.Precision)
				if err != nil {
					return nil, fmt.Errorf("auxiliary %s/%s/M%d: %w", site.ID, zone, month, err)
				}
				values := shares[zone]
				values[month-1] = q
				shares[zone] = values
			}
		}
		meterID := fmt.Sprintf("sql-rdd-%d", meter.DictionaryIndex)
		for _, zone := range zones {
			for _, period := range epathSQLZoneCarrierPeriods() {
				p := &epathSQLAuxiliaryZoneProof{SiteID: site.ID, EndUse: site.EndUse, Carrier: site.Carrier, ZoneName: frames.Zones[zone].Name, Period: period, Basis: "service_path_allocation", Weight: auxiliary.Weight, AllocationMethod: auxiliary.AllocationMethod, Owned: served[zone], Sources: map[string]epathSQLOriginalSource{meterID: epathSQLOriginalRDD(meter)}, NodeSources: map[string]epathSQLOriginalSource{meterID: epathSQLOriginalRDD(meter)}, Required: map[string]bool{}, NodeRequired: map[string]bool{}}
				for _, month := range epathSQLPeriodMonths(period) {
					part := shares[zone][month-1]
					p.Value = p.Value.add(part)
					_, high := part.bounds()
					if high <= 0 {
						continue
					}
					if !part.includesZero() {
						p.Required[meterID], p.NodeRequired[meterID] = true, true
					}
					for _, service := range []string{"cooling", "heating"} {
						key := epathSQLKey(zone, service, month)
						load := frames.Loads[key]
						_, loadHigh := load.bounds()
						if loadHigh <= 0 {
							continue
						}
						if len(frames.LoadSourceIDs[key]) == 0 {
							return nil, fmt.Errorf("auxiliary weight has no original load trace")
						}
						for _, id := range frames.LoadSourceIDs[key] {
							source, exists := frames.SourceIdentities[id]
							if !exists || source.DictionaryIndex != id || id <= 0 || source.IsMeter || !strings.EqualFold(source.KeyValue, p.ZoneName) || source.ReportingFrequency != "Monthly" || source.SourceUnit != "J" {
								return nil, fmt.Errorf("auxiliary weight source escapes original Zone/Monthly/J ownership")
							}
							id := fmt.Sprintf("sql-rdd-%d", id)
							p.NodeSources[id] = epathSQLOriginalRDD(source)
							if !part.includesZero() && !load.includesZero() {
								p.NodeRequired[id] = true
							}
						}
					}
				}
				out[epathSQLAuxiliaryZoneKey(site.ID, zone, period)] = p
			}
		}
	}
	return out, nil
}

func epathSQLModelAuxiliaryZoneChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	proofs, err := epathSQLAuxiliaryZoneProofs(frames, model)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(proofs))
	for key := range proofs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		p := proofs[key]
		for _, field := range []string{"value", "allocatedValue"} {
			target := epathSQLNodeTarget("end_use", p.EndUse, "", "site")
			target.Field, target.Basis, target.AggregationBasis, target.Aggregate, target.AllowPrunedZero = field, p.Basis, "model_total", "", true
			if err := checks.add("zoneAllocation", "zone", p.ZoneName, p.Period, "auxiliary/"+p.SiteID+"/"+field, "kWh", &p.Value, target, "", nil, nil); err != nil {
				return err
			}
			checks.Rows[len(checks.Rows)-1].AuxiliaryZone = p
		}
	}
	return nil
}

func epathSQLAuxiliaryZoneCheckMatches(check epathSQLModelCheck, expected *epathSQLAuxiliaryZoneProof) bool {
	p, target := check.AuxiliaryZone, check.Item.Target
	if p == nil || expected == nil || !reflect.DeepEqual(p, expected) {
		return false
	}
	key := strings.Join([]string{"zoneAllocation", "zone", strings.ToLower(p.ZoneName), p.Period, "auxiliary/" + p.SiteID + "/" + target.Field}, "|")
	return check.Item.Key == key && check.Want.Key == key && check.Item.Group == "zoneAllocation" && check.Want.Group == "zoneAllocation" && check.Item.Scope == "zone" && check.Want.Scope == "zone" && check.Item.Zone == p.ZoneName && check.Want.Zone == p.ZoneName && check.Item.Period == p.Period && check.Want.Period == p.Period && check.Item.Unit == "kWh" && check.Want.Unit == "kWh" && check.Want.Status == "" && check.Want.Found == nil && check.Want.Total == nil && check.Quantity != nil && epathSQLZoneCarrierQuantityEqual(*check.Quantity, p.Value) && check.Want.Value != nil && *check.Want.Value == p.Value.Value && target.Collection == "nodes" && target.Level == "end_use" && target.Category == p.EndUse && target.Service == "" && target.ScaleDomain == "site" && target.AggregationBasis == "model_total" && target.Basis == p.Basis && target.Unit == "kWh" && target.Aggregate == "" && (target.Field == "value" || target.Field == "allocatedValue")
}

func epathCheckSQLAuxiliaryZone(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p := check.AuxiliaryZone
	if !epathSQLAuxiliaryZoneCheckMatches(check, p) || p.SiteID == "" || p.EndUse != "pumps" || p.Carrier != "electricity" || p.Basis != "service_path_allocation" || p.Weight != "cooling_plus_heating" || p.AllocationMethod != "plant_loop_load_share" || !p.Value.valid() || p.Value.Value < 0 || !epathOracleValidPeriod(p.Period) || !p.Owned && !p.Value.includesZero() {
		return fmt.Errorf("invalid exact independent allocated auxiliary scalar proof")
	}
	nodes, _, _, _, err := epathOracleGraph(bundle, "zone", p.ZoneName, p.Period)
	if err != nil {
		return err
	}
	var selected *EnergyExplanationNode
	for i := range nodes {
		node := &nodes[i]
		if node.Level != "end_use" || node.EndUse != p.EndUse {
			continue
		}
		if selected != nil || !p.Owned || !strings.EqualFold(node.ZoneName, p.ZoneName) || node.Period != p.Period || node.Unit != "kWh" || node.ScaleDomain != "site" || node.AggregationBasis != "model_total" || node.Basis != p.Basis || !node.AllocationApplied || node.Carrier != "" || node.ServiceKind != "" && node.ServiceKind != p.EndUse {
			return fmt.Errorf("duplicate, unowned, direct, or mistyped allocated auxiliary node")
		}
		selected = node
	}
	if selected == nil {
		if p.Value.includesZero() {
			return nil
		}
		return fmt.Errorf("missing positive allocated auxiliary node")
	}
	sources := map[string]EnergyDataSource{}
	for _, source := range bundle.EnergyExplanation.Sources {
		if _, exists := sources[source.ID]; exists {
			return fmt.Errorf("duplicate allocated auxiliary source identity")
		}
		sources[source.ID] = source
	}
	if err := epathSQLVerifyOriginalSources(selected.SourceIDs, sources, p.NodeSources, p.NodeRequired, p.Period); err != nil {
		return err
	}
	actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
	if err != nil {
		return err
	}
	if actual == nil {
		return fmt.Errorf("present allocated auxiliary scalar is unknown, not pruned zero")
	}
	return epathCheckSQLModelQuantity(actual, &p.Value)
}
