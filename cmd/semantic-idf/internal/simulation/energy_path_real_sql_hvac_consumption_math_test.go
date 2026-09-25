package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type epathSQLHVACConsumptionSourceShare struct {
	SourceID           int
	SiteID, ZoneName   string
	Native             epathSQLQuantity
	BudgetMilliKWh     int64
	Shares             map[string]int64
	UnassignedMilliKWh int64
	Original           epathSQLOriginalSource
}

type epathSQLHVACConsumptionServiceFrames struct {
	Direct            epathSQLDirectHVACServiceFrames
	Sources           [12]map[int]epathSQLHVACConsumptionSourceShare
	PoolCarriers      map[string]bool
	Overmapped        [12]map[string]epathSQLQuantity
	centralCoolingCOP bool // Recomputed finite native/source qualification; not serialized authority.
	frames            epathSQLFrames
	model             epathRealSQLModel
	service           epathRealSQLService
}

// The packaged-terminal compiler remains the control for untouched carriers.
// Every bound carrier below is replaced using original consuming components;
// none of the old whole-direct-Zone remainder shares is consumed by this proof.
func epathSQLCompileHVACConsumptionService(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService) (*epathSQLHVACConsumptionServiceFrames, error) {
	if len(model.HVACConsumptionPools) == 0 {
		if len(frames.HVACConsumptionPools) != 0 {
			return nil, fmt.Errorf("undeclared original HVAC consumption pool frames")
		}
		return nil, nil
	}
	if len(model.HVACConsumptionPools) != len(frames.HVACConsumptionPools) {
		return nil, fmt.Errorf("missing original HVAC consumption pool frame")
	}
	sites := map[string]epathRealSQLSite{}
	for _, site := range model.Site {
		if site.ID == "" || sites[site.ID].ID != "" {
			return nil, fmt.Errorf("duplicate/empty original HVAC site identity")
		}
		sites[site.ID] = site
	}
	selected := []epathSQLHVACConsumptionPoolFrame{}
	seenSites, seenMembers, seenSources := map[string]bool{}, map[string]bool{}, map[int]bool{}
	poolCarriers := map[string]bool{}
	for i, pool := range frames.HVACConsumptionPools {
		if !reflect.DeepEqual(pool.Declaration, model.HVACConsumptionPools[i]) {
			return nil, fmt.Errorf("original pool frame differs from reviewed recipe")
		}
		if err := epathSQLValidateHVACConsumptionPoolFrame(pool); err != nil {
			return nil, err
		}
		site, exists := sites[pool.Declaration.SiteID]
		if !exists || site.Facility || site.EndUse == "" || site.Carrier == "" || seenSites[site.ID] {
			return nil, fmt.Errorf("invalid/duplicate native pool site binding")
		}
		seenSites[site.ID] = true
		for _, member := range pool.Shared {
			if member.Member.ObjectType == epathSQLCentralSharedType {
				if err := epathSQLCentralSharedSite(member.Member, site, frames); err != nil {
					return nil, err
				}
			}
			if seenMembers[member.Member.ID] || seenSources[member.Source.DictionaryIndex] {
				return nil, fmt.Errorf("duplicated native shared member or original source")
			}
			seenMembers[member.Member.ID], seenSources[member.Source.DictionaryIndex] = true, true
		}
		if site.EndUse != service.Service {
			continue
		}
		if !epathSQLHVACConsumptionHasString(service.SiteIDs, site.ID) || poolCarriers[site.Carrier] {
			return nil, fmt.Errorf("native service pool needs one exact site per carrier")
		}
		poolCarriers[site.Carrier] = true
		selected = append(selected, pool)
	}
	if len(selected) == 0 {
		return nil, nil
	}
	if len(model.NativeVRFSystems) > 0 {
		return nil, fmt.Errorf("native VRF coexistence needs its separately reviewed source roster")
	}
	base, err := epathSQLCompileDirectHVACService(frames, model, service, epathSQLCentralSharedOnlyService(frames, model, service, selected))
	if err != nil || base == nil {
		return nil, fmt.Errorf("native mixed pool needs independently validated direct component frames: %v", err)
	}
	served, err := epathSQLDeclaredZones(frames, service.ServedZones)
	if err != nil || len(served) == 0 {
		return nil, fmt.Errorf("native mixed pool requires reviewed served Zones: %v", err)
	}
	out := &epathSQLHVACConsumptionServiceFrames{Direct: *base, PoolCarriers: poolCarriers, frames: frames, model: model, service: service, centralCoolingCOP: epathSQLCentralCoolingCOP(frames, model, service, selected)}
	for month := 1; month <= 12; month++ {
		out.Sources[month-1] = map[int]epathSQLHVACConsumptionSourceShare{}
		out.Overmapped[month-1] = map[string]epathSQLQuantity{}
		row := &out.Direct.Monthly[month-1]
		for _, pool := range selected {
			site := sites[pool.Declaration.SiteID]
			// Avoid claiming that a sibling site with the same carrier is part
			// of this exact original broad meter's consuming-component roster.
			for _, id := range service.SiteIDs {
				if id != site.ID && sites[id].Carrier == site.Carrier {
					return nil, fmt.Errorf("ambiguous native same-carrier broad site")
				}
			}
			directMilli, allocatedMilli := int64(0), int64(0)
			for zone := range frames.Zones {
				current := row.Zones[zone][site.Carrier]
				current.Allocated = epathSQLVRFExactMilli(0)
				current.Direct = epathSQLVRFExactMilli(0)
				for id, original := range current.DirectSources {
					if original.DirectHVAC == nil || original.DirectHVAC.SiteID != site.ID || original.RDD == nil || seenSources[original.RDD.DictionaryIndex] {
						return nil, fmt.Errorf("direct pool member has a foreign/shared source identity: %s", id)
					}
					values, err := epathSQLMonthly(*original.RDD, model.Precision)
					if err != nil {
						return nil, err
					}
					budget, err := epathSQLVRFMilliBudget(values[month-1].Value)
					if err != nil {
						return nil, err
					}
					directMilli, err = epathSQLVRFMilliAdd(directMilli, budget)
					if err != nil {
						return nil, err
					}
					current.Direct = current.Direct.add(epathSQLVRFExactMilli(budget))
					out.Sources[month-1][original.RDD.DictionaryIndex] = epathSQLHVACConsumptionSourceShare{SourceID: original.RDD.DictionaryIndex, SiteID: site.ID, ZoneName: frames.Zones[zone].Name, Native: values[month-1], BudgetMilliKWh: budget, Shares: map[string]int64{zone: budget}, Original: original}
				}
				row.Zones[zone][site.Carrier] = current
			}
			for _, member := range pool.Shared {
				if err := epathSQLValidateHVACSharedSourceIdentity(member); err != nil {
					return nil, err
				}
				if !reflect.DeepEqual(member.Precision, model.Precision) {
					return nil, fmt.Errorf("native shared source changed reviewed precision")
				}
				values, err := epathSQLMonthly(member.Source, member.Precision)
				if err != nil {
					return nil, err
				}
				budget, err := epathSQLVRFMilliBudget(values[month-1].Value)
				if err != nil {
					return nil, err
				}
				weights, names, keys := map[string]float64{}, []string{}, map[string]string{}
				for _, name := range member.Member.ServedZones {
					zone := strings.ToLower(strings.TrimSpace(name))
					if !served[zone] || frames.Zones[zone].Name != name || keys[name] != "" {
						return nil, fmt.Errorf("shared source has a foreign/duplicate original recipient")
					}
					_, loadID, err := epathSQLVRFAllocationLoad(frames, model.Precision, name, service.Service, month)
					if err != nil {
						return nil, err
					}
					displayed, err := epathSQLVRFDisplayedLoad(frames, name, loadID, month)
					if err != nil {
						return nil, err
					}
					// This pool weights canonical displayed Zone load nodes:
					// native Monthly 3dp, original multiplier once, effective 3dp.
					keys[name] = zone
					if displayed > 0 {
						weights[name] = float64(displayed)
						names = append(names, name)
					}
				}
				sort.Strings(names)
				shares, remainder := map[string]int64{}, budget
				if len(names) > 0 {
					shares, remainder, err = epathSQLVRFMilliShares(budget, weights, names)
					if err != nil {
						return nil, err
					}
				}
				original := epathSQLOriginalRDD(member.Source)
				proof := epathSQLHVACConsumptionSourceShare{SourceID: member.Source.DictionaryIndex, SiteID: site.ID, Native: values[month-1], BudgetMilliKWh: budget, Shares: map[string]int64{}, UnassignedMilliKWh: remainder, Original: original}
				for name, share := range shares {
					zone := keys[name]
					proof.Shares[zone] = share
					current := row.Zones[zone][site.Carrier]
					current.Allocated = current.Allocated.add(epathSQLVRFExactMilli(share))
					row.Zones[zone][site.Carrier] = current
					allocatedMilli, err = epathSQLVRFMilliAdd(allocatedMilli, share)
					if err != nil {
						return nil, err
					}
				}
				out.Sources[month-1][member.Source.DictionaryIndex] = proof
			}
			ledger := row.ByCarrier[site.Carrier]
			meterIDs := frames.SiteSources[site.ID]
			if len(meterIDs) != 1 {
				return nil, fmt.Errorf("native consumption pool requires one exact original broad meter")
			}
			meter, exists := frames.SourceIdentities[meterIDs[0]]
			if !exists || meter.DictionaryIndex != meterIDs[0] || !meter.IsMeter || meter.ReportingFrequency != "Monthly" || meter.SourceUnit != "J" {
				return nil, fmt.Errorf("native pool broad meter identity changed")
			}
			meterValues, err := epathSQLMonthly(meter, model.Precision)
			if err != nil {
				return nil, err
			}
			if !epathSQLFanPoolNear(meterValues[month-1].Value, ledger.Site.Value) {
				return nil, fmt.Errorf("native pool broad meter frame changed original observation")
			}
			siteMilli, err := epathSQLVRFMilliBudget(meterValues[month-1].Value)
			if err != nil {
				return nil, err
			}
			ledger.Site = epathSQLVRFExactMilli(siteMilli)
			ledger.Direct, ledger.Allocated = epathSQLVRFExactMilli(directMilli), epathSQLVRFExactMilli(allocatedMilli)
			consumed, err := epathSQLVRFMilliAdd(directMilli, allocatedMilli)
			if err != nil {
				return nil, err
			}
			ledger.Unassigned = epathSQLVRFExactMilli(max(int64(0), siteMilli-consumed))
			out.Overmapped[month-1][site.Carrier] = epathSQLVRFExactMilli(max(int64(0), consumed-siteMilli))
			row.ByCarrier[site.Carrier] = ledger
		}
		row.Site, row.Direct, row.Allocated, row.Unassigned = epathSQLQuantity{}, epathSQLQuantity{}, epathSQLQuantity{}, epathSQLQuantity{}
		for _, carrier := range out.Direct.Carriers {
			ledger := row.ByCarrier[carrier]
			row.Site = row.Site.add(ledger.Site)
			row.Direct = row.Direct.add(ledger.Direct)
			row.Allocated = row.Allocated.add(ledger.Allocated)
			row.Unassigned = row.Unassigned.add(ledger.Unassigned)
		}
	}
	return out, nil
}

func epathSQLValidateHVACConsumptionService(proof *epathSQLHVACConsumptionServiceFrames) error {
	if proof == nil || len(proof.PoolCarriers) == 0 {
		return fmt.Errorf("missing independent native source-pool proof")
	}
	fresh, err := epathSQLCompileHVACConsumptionService(proof.frames, proof.model, proof.service)
	if err != nil || fresh == nil || fresh.centralCoolingCOP != proof.centralCoolingCOP || !epathSQLHVACConsumptionSameDirect(fresh.Direct, proof.Direct) || !reflect.DeepEqual(fresh.Sources, proof.Sources) || !reflect.DeepEqual(fresh.PoolCarriers, proof.PoolCarriers) || !reflect.DeepEqual(fresh.Overmapped, proof.Overmapped) {
		return fmt.Errorf("native consumption allocation differs from its original source-local arithmetic: %v", err)
	}
	return nil
}

// Untouched continuous carrier ledgers can add a map's quantities in a
// different order. Reuse the existing oracle's floating arithmetic equality,
// never a candidate tolerance; discrete source budgets remain exact above.
func epathSQLHVACConsumptionSameDirect(a, b epathSQLDirectHVACServiceFrames) bool {
	if !reflect.DeepEqual(a.Carriers, b.Carriers) || !reflect.DeepEqual(a.BroadSources, b.BroadSources) {
		return false
	}
	for n, left := range a.Monthly {
		right := b.Monthly[n]
		if len(left.ByCarrier) != len(right.ByCarrier) || len(left.Zones) != len(right.Zones) {
			return false
		}
		for _, pair := range [][2]epathSQLQuantity{{left.Site, right.Site}, {left.Direct, right.Direct}, {left.Allocated, right.Allocated}, {left.Unassigned, right.Unassigned}} {
			if !epathSQLZoneCarrierQuantityEqual(pair[0], pair[1]) {
				return false
			}
		}
		for carrier, x := range left.ByCarrier {
			y, exists := right.ByCarrier[carrier]
			if !exists {
				return false
			}
			for _, pair := range [][2]epathSQLQuantity{{x.Site, y.Site}, {x.Direct, y.Direct}, {x.Allocated, y.Allocated}, {x.Unassigned, y.Unassigned}} {
				if !epathSQLZoneCarrierQuantityEqual(pair[0], pair[1]) {
					return false
				}
			}
		}
		for zone, parts := range left.Zones {
			other, exists := right.Zones[zone]
			if !exists || len(parts) != len(other) {
				return false
			}
			for carrier, x := range parts {
				y, exists := other[carrier]
				if !exists || x.ObservedDirect != y.ObservedDirect || !reflect.DeepEqual(x.DirectSources, y.DirectSources) || !epathSQLZoneCarrierQuantityEqual(x.Direct, y.Direct) || !epathSQLZoneCarrierQuantityEqual(x.Allocated, y.Allocated) {
					return false
				}
			}
		}
	}
	return true
}

func epathSQLHVACConsumptionHasString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
