package simulation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func epathSQLHVACConsumptionRatioDeclaration(pool *epathSQLHVACConsumptionServiceFrames, zone, period, fallback string) string {
	positive := map[string]bool{}
	for _, month := range epathSQLPeriodMonths(period) {
		row := pool.Direct.Monthly[month-1]
		for _, carrier := range pool.Direct.Carriers {
			q := row.ByCarrier[carrier].Site
			if zone != "" {
				share := row.Zones[zone][carrier]
				q = share.Direct.add(share.Allocated)
			}
			if q.Value > 0 {
				positive[carrier] = true
			}
		}
	}
	if len(positive) == 1 && positive["natural_gas"] {
		return "efficiency"
	}
	if len(positive) > 1 || positive["electricity"] {
		return "load_to_site_energy"
	}
	return fallback
}

func epathSQLHVACConsumptionZoneProof(pool *epathSQLHVACConsumptionServiceFrames, zone, period string) (*epathSQLZoneServiceProof, map[string][12]epathSQLConversionProof, error) {
	if pool == nil || pool.frames.Zones[zone].Name == "" || !epathOracleValidPeriod(period) {
		return nil, nil, fmt.Errorf("invalid pool Zone/period")
	}
	frames, service := pool.frames, pool.service
	proof := &epathSQLZoneServiceProof{HVACConsumption: pool, DirectHVAC: true, Service: service.Service, Basis: service.Basis, Carriers: map[string]epathSQLQuantity{}, Branches: map[string]epathSQLDirectHVACBranchProof{}, LoadSources: map[string]epathRealSQLSource{}}
	pairs := map[string][12]epathSQLConversionProof{}
	for _, month := range epathSQLPeriodMonths(period) {
		row := pool.Direct.Monthly[month-1]
		load := frames.Loads[epathSQLKey(zone, service.Service, month)]
		ids := frames.LoadSourceIDs[epathSQLKey(zone, service.Service, month)]
		if len(ids) == 0 {
			return nil, nil, fmt.Errorf("pool Zone lacks canonical load authority")
		}
		for _, id := range ids {
			source, exists := frames.SourceIdentities[id]
			if !exists || source.DictionaryIndex != id || source.IsMeter || !strings.EqualFold(source.KeyValue, zone) || source.ReportingFrequency != "Monthly" || source.SourceUnit != "J" {
				return nil, nil, fmt.Errorf("foreign pool Zone load authority")
			}
			proof.LoadSources[fmt.Sprintf("sql-rdd-%d", id)] = source
		}
		directTotal, total := epathSQLQuantity{}, epathSQLQuantity{}
		for _, carrier := range pool.Direct.Carriers {
			share := row.Zones[zone][carrier]
			// Two independent entries can coexist on this same Zone/carrier.
			// A local observation never replaces the central source's own share.
			for _, direct := range []bool{true, false} {
				q, basis := share.Allocated, service.Basis
				originals, required := map[string]epathSQLOriginalSource{}, map[string]bool{}
				if direct {
					if !share.ObservedDirect {
						continue
					}
					q, basis = share.Direct, "direct_zone_energy"
					for id, source := range share.DirectSources {
						originals[id] = source
						if source.RDD != nil && source.RDD.Months[month-1].EnergyKWh != nil && *source.RDD.Months[month-1].EnergyKWh > 0 {
							required[id] = true
						}
					}
					directTotal = directTotal.add(q)
				} else {
					if !pool.PoolCarriers[carrier] && share.ObservedDirect {
						continue
					}
					for id, source := range pool.Direct.BroadSources[carrier] {
						originals[id] = source
						if q.Value > 0 && frames.SourceRaw[source.RDD.DictionaryIndex][month-1].Value > 0 {
							required[id] = true
						}
					}
					if pool.PoolCarriers[carrier] {
						for _, sourceShare := range pool.Sources[month-1] {
							if sourceShare.ZoneName != "" {
								continue
							}
							allocated, eligible := sourceShare.Shares[zone]
							if !eligible {
								continue
							}
							id := fmt.Sprintf("sql-rdd-%d", sourceShare.SourceID)
							originals[id] = sourceShare.Original
							if allocated > 0 {
								required[id] = true
							}
						}
						for id, source := range proof.LoadSources {
							originals[id] = epathSQLOriginalRDD(source)
						}
					} else {
						for _, other := range row.Zones {
							for id, source := range other[carrier].DirectSources {
								originals[id] = source
								if q.Value > 0 {
									required[id] = true
								}
							}
						}
					}
				}
				relation := "end_use_to_carrier"
				if load.Value == 0 {
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
		}
		basis := service.Basis
		if directTotal.Value > 0 && total.Value-directTotal.Value <= 1e-12 {
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
	}
	return proof, pairs, nil
}

func epathSQLHVACConsumptionZoneServiceChecks(pool *epathSQLHVACConsumptionServiceFrames, zones []string, checks *epathSQLModelChecks) error {
	if err := epathSQLValidateHVACConsumptionService(pool); err != nil {
		return err
	}
	for _, zone := range zones {
		for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
			proof, pairs, err := epathSQLHVACConsumptionZoneProof(pool, zone, period)
			if err != nil {
				return err
			}
			value := epathSQLQuantity{}
			for _, q := range proof.Carriers {
				value = value.add(q)
			}
			for _, field := range []string{"value", "allocatedValue"} {
				target := epathSQLNodeTarget("end_use", pool.service.Service, "", "site")
				target.Field, target.Basis, target.AllowPrunedZero = field, proof.Basis, true
				if err := checks.add("zoneAllocation", "zone", pool.frames.Zones[zone].Name, period, "hvac_zone/"+pool.service.Service+"/"+field, "kWh", &value, target, "", nil, nil); err != nil {
					return err
				}
				if field == "value" {
					checks.Rows[len(checks.Rows)-1].ZoneService = proof
				}
			}
			for _, basis := range []string{"direct_zone_energy", pool.service.Basis, pool.service.FallbackBasis} {
				from, to := epathSQLQuantity{}, epathSQLQuantity{}
				for _, month := range epathSQLPeriodMonths(period) {
					from = from.add(pairs[basis][month-1].From)
					to = to.add(pairs[basis][month-1].To)
				}
				declared := epathSQLHVACConsumptionRatioDeclaration(pool, zone, period, pool.service.RatioKind)
				kind, err := epathSQLConversionPeriodRatioKind(declared, period, pairs[basis])
				if err != nil {
					return fmt.Errorf("source-local %s/%s/%s ratio: %w", zone, period, basis, err)
				}
				var ratio *epathSQLQuantity
				if from.Value > 0 && to.Value > 0 {
					ratio = &epathSQLQuantity{Value: from.Value / to.Value}
				}
				target := epathRealOracleTarget{Collection: "links", Field: "pairedRatio", Relation: "load_to_end_use", Service: pool.service.Service, Basis: basis, FromUnit: "kWh", ToUnit: "kWh", RatioKind: kind, Aggregate: "sum"}
				if err := checks.add("ratios", "zone", pool.frames.Zones[zone].Name, period, "hvac_zone/"+pool.service.Service+"/"+basis, "ratio", ratio, target, "", nil, nil); err != nil {
					return err
				}
				checks.Rows[len(checks.Rows)-1].Conversion = &epathSQLConversionProof{From: from, To: to}
			}
		}
	}
	return nil
}

func epathCheckSQLHVACConsumptionZoneService(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	if check.ZoneService == nil || check.ZoneService.HVACConsumption == nil {
		return fmt.Errorf("missing source-local Zone proof")
	}
	pool := check.ZoneService.HVACConsumption
	if err := epathSQLValidateHVACConsumptionService(pool); err != nil {
		return err
	}
	want, _, err := epathSQLHVACConsumptionZoneProof(pool, strings.ToLower(check.Item.Zone), check.Item.Period)
	if err != nil || !reflect.DeepEqual(want, check.ZoneService) {
		return fmt.Errorf("Zone graph proof changed its original source-local arithmetic: %v", err)
	}
	if err := epathCheckSQLDirectHVACZoneService(bundle, check); err != nil {
		return err
	}
	if check.Item.Period == "annual" {
		return epathSQLHVACConsumptionScopedSources(bundle, pool, strings.ToLower(check.Item.Zone))
	}
	return nil
}

func epathSQLHVACConsumptionScopedSources(bundle PurposeResultBundle, pool *epathSQLHVACConsumptionServiceFrames, zone string) error {
	sources, err := epathSQLDirectHVACSourceMap(bundle)
	if err != nil {
		return err
	}
	amounts, eligible := map[int]int64{}, map[int]bool{}
	shared := map[int]bool{}
	for _, month := range pool.Sources {
		for id, row := range month {
			if row.ZoneName != "" {
				continue
			}
			shared[id] = true
			if value, exists := row.Shares[zone]; exists {
				eligible[id] = true
				amounts[id] += value
			}
		}
	}
	for id := range shared {
		source, exists := sources[fmt.Sprintf("sql-rdd-%d", id)]
		if !exists {
			return fmt.Errorf("native shared source missing even for measured zero")
		}
		count := 0
		for _, detail := range source.ScopeDetails {
			if detail.Scope.Kind != "zone" || !strings.EqualFold(detail.Scope.ZoneName, zone) {
				continue
			}
			count++
			encoded, err := json.Marshal(detail)
			if err != nil {
				return err
			}
			var wire map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &wire); err != nil {
				return err
			}
			_, rawKnown := wire["rawValue"]
			_, effectiveKnown := wire["effectiveValue"]
			if !eligible[id] || count != 1 || rawKnown || effectiveKnown || !detail.AllocationApplied || detail.RawValue != 0 || detail.EffectiveValue != 0 {
				return fmt.Errorf("shared source fabricated measured Zone input or recipient")
			}
			q := epathSQLVRFExactMilli(amounts[id])
			if err := epathCheckSQLModelQuantity(&detail.AllocatedValue, &q); err != nil {
				return err
			}
		}
		if eligible[id] && count != 1 {
			return fmt.Errorf("shared source lost allocated-only Zone detail")
		}
	}
	return nil
}
