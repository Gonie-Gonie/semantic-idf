package simulation

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strings"
)

// These are independent SQL/display proofs, not canonical graph records. Native
// quantities are never replaced by the millikWh presentation allocation.
type epathSQLVRFAllocatedShare struct {
	Native          float64
	DisplayMilliKWh int64
}

type epathSQLVRFSourceAllocation struct {
	SourceID                int
	Role, Service, ZoneName string
	Shared                  bool
	Native                  epathSQLQuantity
	BudgetMilliKWh          int64
	Shares                  map[string]epathSQLVRFAllocatedShare
	UnassignedNative        float64
	UnassignedMilliKWh      int64
}

type epathSQLVRFZoneAllocation struct {
	ZoneName, Service                 string
	Load                              epathSQLQuantity
	LoadDisplayMilliKWh               int64
	LoadSourceIDs                     []int
	SourceShares                      map[int]epathSQLVRFAllocatedShare
	NativeDirect, NativeAllocated     float64
	DirectMilliKWh, AllocatedMilliKWh int64
	PairedLoad                        epathSQLQuantity
	PairedLoadDisplayMilliKWh         int64
	PairedSiteNative                  float64
	PairedSiteMilliKWh                int64
	PairMonths                        []int
}

type epathSQLVRFAllocationMonth struct {
	Sources map[int]epathSQLVRFSourceAllocation
	Zones   map[string]map[string]epathSQLVRFZoneAllocation
}

type epathSQLVRFAllocationProof struct {
	System epathSQLVRFSystemFrame
	Zones  []string
	Months map[int]epathSQLVRFAllocationMonth
	Annual epathSQLVRFAllocationMonth
}

// The source layer binds the original IDF/plan/SQL identities. Revalidate its
// immutable frame here, then bind each weight to its selected original Monthly
// sensible source. No production allocator or candidate number is consulted.
func epathSQLCompileVRFAllocation(frames epathSQLFrames, system epathSQLVRFSystemFrame) (epathSQLVRFAllocationProof, error) {
	if err := epathSQLValidateVRFSystemFrame(system); err != nil {
		return epathSQLVRFAllocationProof{}, err
	}
	out := epathSQLVRFAllocationProof{System: system, Months: map[int]epathSQLVRFAllocationMonth{}}
	for _, terminal := range system.Declaration.Terminals {
		out.Zones = append(out.Zones, terminal.ZoneName)
	}
	// The tie break is the declared original canonical ZoneName, not map order,
	// terminal-list order, source ID, or a case-folded display label.
	sort.Strings(out.Zones)
	newMonth := func() epathSQLVRFAllocationMonth {
		value := epathSQLVRFAllocationMonth{Sources: map[int]epathSQLVRFSourceAllocation{}, Zones: map[string]map[string]epathSQLVRFZoneAllocation{}}
		for _, zone := range out.Zones {
			value.Zones[zone] = map[string]epathSQLVRFZoneAllocation{}
			for _, service := range []string{"cooling", "heating"} {
				value.Zones[zone][service] = epathSQLVRFZoneAllocation{ZoneName: zone, Service: service, SourceShares: map[int]epathSQLVRFAllocatedShare{}}
			}
		}
		return value
	}
	out.Annual = newMonth()
	for month := 1; month <= 12; month++ {
		current := newMonth()
		weights := map[string]map[string]float64{"cooling": {}, "heating": {}}
		for _, zone := range out.Zones {
			for _, service := range []string{"cooling", "heating"} {
				load, id, err := epathSQLVRFAllocationLoad(frames, system.Precision, zone, service, month)
				if err != nil {
					return epathSQLVRFAllocationProof{}, err
				}
				row := current.Zones[zone][service]
				row.Load, row.LoadSourceIDs = load, []int{id}
				row.LoadDisplayMilliKWh, err = epathSQLVRFDisplayedLoad(frames, zone, id, month)
				if err != nil {
					return epathSQLVRFAllocationProof{}, err
				}
				current.Zones[zone][service] = row
				weights[service][zone] = load.Value
			}
		}
		for _, source := range system.Sources {
			q := source.Months[month-1]
			owner := source.ZoneName
			if !source.Shared {
				for _, zone := range out.Zones {
					if strings.EqualFold(zone, source.ZoneName) {
						owner = zone
					}
				}
			}
			budget, err := epathSQLVRFMilliBudget(q.Value)
			if err != nil {
				return epathSQLVRFAllocationProof{}, err
			}
			row := epathSQLVRFSourceAllocation{SourceID: source.Source.DictionaryIndex, Role: source.Role, Service: source.Service, ZoneName: owner, Shared: source.Shared, Native: q, BudgetMilliKWh: budget, Shares: map[string]epathSQLVRFAllocatedShare{}}
			if source.Shared {
				shares, unassigned, err := epathSQLVRFMilliShares(budget, weights[source.Service], out.Zones)
				if err != nil {
					return epathSQLVRFAllocationProof{}, err
				}
				denominator := 0.0
				for _, zone := range out.Zones {
					denominator += weights[source.Service][zone]
				}
				if !epathOracleFinite(denominator) {
					return epathSQLVRFAllocationProof{}, fmt.Errorf("VRF load denominator overflow")
				}
				row.UnassignedMilliKWh = unassigned
				if denominator == 0 {
					row.UnassignedNative = q.Value
				}
				for _, zone := range out.Zones {
					native := 0.0
					if denominator > 0 {
						native = q.Value * (weights[source.Service][zone] / denominator)
					}
					row.Shares[zone] = epathSQLVRFAllocatedShare{Native: native, DisplayMilliKWh: shares[zone]}
				}
			} else {
				row.Shares[owner] = epathSQLVRFAllocatedShare{Native: q.Value, DisplayMilliKWh: budget}
			}
			current.Sources[row.SourceID] = row
			annual := out.Annual.Sources[row.SourceID]
			if annual.SourceID == 0 {
				annual = epathSQLVRFSourceAllocation{SourceID: row.SourceID, Role: row.Role, Service: row.Service, ZoneName: row.ZoneName, Shared: row.Shared, Shares: map[string]epathSQLVRFAllocatedShare{}}
			}
			annual.Native = annual.Native.add(row.Native)
			annual.BudgetMilliKWh, err = epathSQLVRFMilliAdd(annual.BudgetMilliKWh, row.BudgetMilliKWh)
			if err != nil {
				return epathSQLVRFAllocationProof{}, err
			}
			annual.UnassignedNative += row.UnassignedNative
			annual.UnassignedMilliKWh += row.UnassignedMilliKWh // bounded by the checked budget sum
			for zone, share := range row.Shares {
				previous := annual.Shares[zone]
				previous.Native += share.Native
				previous.DisplayMilliKWh += share.DisplayMilliKWh
				annual.Shares[zone] = previous
				zoneRow := current.Zones[zone][row.Service]
				zoneRow.SourceShares[row.SourceID] = share
				if row.Shared {
					zoneRow.NativeAllocated += share.Native
					zoneRow.AllocatedMilliKWh, err = epathSQLVRFMilliAdd(zoneRow.AllocatedMilliKWh, share.DisplayMilliKWh)
				} else {
					zoneRow.NativeDirect += share.Native
					zoneRow.DirectMilliKWh, err = epathSQLVRFMilliAdd(zoneRow.DirectMilliKWh, share.DisplayMilliKWh)
				}
				if err != nil {
					return epathSQLVRFAllocationProof{}, err
				}
				current.Zones[zone][row.Service] = zoneRow
			}
			out.Annual.Sources[row.SourceID] = annual
		}
		for _, zone := range out.Zones {
			for _, service := range []string{"cooling", "heating"} {
				row := current.Zones[zone][service]
				site, err := epathSQLVRFMilliAdd(row.DirectMilliKWh, row.AllocatedMilliKWh)
				if err != nil {
					return epathSQLVRFAllocationProof{}, err
				}
				if row.LoadDisplayMilliKWh > 0 && site > 0 {
					row.PairedLoad, row.PairedSiteNative, row.PairedSiteMilliKWh = row.Load, row.NativeDirect+row.NativeAllocated, site
					row.PairedLoadDisplayMilliKWh = row.LoadDisplayMilliKWh
					row.PairMonths = []int{month}
				}
				current.Zones[zone][service] = row
				annual := out.Annual.Zones[zone][service]
				annual.Load = annual.Load.add(row.Load)
				annual.PairedLoad = annual.PairedLoad.add(row.PairedLoad)
				annual.NativeDirect += row.NativeDirect
				annual.NativeAllocated += row.NativeAllocated
				annual.PairedSiteNative += row.PairedSiteNative
				annual.LoadDisplayMilliKWh, err = epathSQLVRFMilliAdd(annual.LoadDisplayMilliKWh, row.LoadDisplayMilliKWh)
				if err == nil {
					annual.PairedLoadDisplayMilliKWh, err = epathSQLVRFMilliAdd(annual.PairedLoadDisplayMilliKWh, row.PairedLoadDisplayMilliKWh)
				}
				if err == nil {
					annual.DirectMilliKWh, err = epathSQLVRFMilliAdd(annual.DirectMilliKWh, row.DirectMilliKWh)
				}
				if err == nil {
					annual.AllocatedMilliKWh, err = epathSQLVRFMilliAdd(annual.AllocatedMilliKWh, row.AllocatedMilliKWh)
				}
				if err == nil {
					annual.PairedSiteMilliKWh, err = epathSQLVRFMilliAdd(annual.PairedSiteMilliKWh, row.PairedSiteMilliKWh)
				}
				if err != nil {
					return epathSQLVRFAllocationProof{}, err
				}
				if _, err := epathSQLVRFMilliAdd(annual.DirectMilliKWh, annual.AllocatedMilliKWh); err != nil {
					return epathSQLVRFAllocationProof{}, err
				}
				annual.PairMonths = append(annual.PairMonths, row.PairMonths...)
				for _, id := range row.LoadSourceIDs {
					if !epathSQLVRFHasID(annual.LoadSourceIDs, id) {
						annual.LoadSourceIDs = append(annual.LoadSourceIDs, id)
					}
				}
				for id, share := range row.SourceShares {
					previous := annual.SourceShares[id]
					previous.Native += share.Native
					previous.DisplayMilliKWh += share.DisplayMilliKWh
					annual.SourceShares[id] = previous
				}
				out.Annual.Zones[zone][service] = annual
			}
		}
		out.Months[month] = current
	}
	return out, nil
}

// This presentation proof is separate from the unrounded weight: selected
// native source 3dp -> resolved Zone multiplier once -> effective 3dp.
func epathSQLVRFDisplayedLoad(frames epathSQLFrames, zone string, id, month int) (int64, error) {
	source := frames.SourceIdentities[id]
	if month < 1 || month > 12 || len(source.Months) != 12 || source.Months[month-1].EnergyKWh == nil {
		return 0, fmt.Errorf("unknown original VRF displayed load")
	}
	native, err := epathSQLVRFMilliBudget(*source.Months[month-1].EnergyKWh)
	if err != nil {
		return 0, err
	}
	return epathSQLVRFMilliBudget(float64(native) / 1000 * frames.Zones[strings.ToLower(zone)].Multiplier)
}

func epathSQLVRFAllocationLoad(frames epathSQLFrames, precision epathRealSQLPrecision, zone, service string, month int) (epathSQLQuantity, int, error) {
	key := epathSQLKey(zone, service, month)
	q, found := frames.Loads[key]
	owner, owned := frames.Zones[strings.ToLower(zone)]
	ids := frames.LoadSourceIDs[key]
	if !found || !owned || !strings.EqualFold(owner.Name, zone) || !epathOracleFinite(owner.Multiplier) || owner.Multiplier <= 0 || !q.valid() || q.Value < 0 || len(ids) != 1 {
		return epathSQLQuantity{}, 0, fmt.Errorf("VRF weight has missing/invalid selected Zone load: %s", key)
	}
	id := ids[0]
	source, known := frames.SourceIdentities[id]
	name := "Zone Air System Sensible Cooling Energy"
	if service == "heating" {
		name = "Zone Air System Sensible Heating Energy"
	}
	if !known || id <= 0 || source.DictionaryIndex != id || source.Name != name || source.IsMeter || source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" || !strings.EqualFold(source.KeyValue, zone) || source.MissingRows != 0 {
		return epathSQLQuantity{}, 0, fmt.Errorf("VRF weight has contradictory original source: %s", key)
	}
	values, err := epathSQLMonthly(source, precision)
	if err != nil {
		return epathSQLQuantity{}, 0, err
	}
	for _, bucket := range source.Months {
		if bucket.MissingRows != 0 || *bucket.EnergyKWh < 0 {
			return epathSQLQuantity{}, 0, fmt.Errorf("VRF weight has unobserved/negative month: %s", key)
		}
	}
	if !reflect.DeepEqual(q, values[month-1].times(owner.Multiplier)) {
		return epathSQLQuantity{}, 0, fmt.Errorf("VRF weight differs from original source times Zone multiplier once: %s", key)
	}
	return q, id, nil
}

const epathSQLVRFMaxExactMilli int64 = 1<<53 - 1

func epathSQLVRFMilliBudget(value float64) (int64, error) {
	if !epathOracleFinite(value) || value < 0 {
		return 0, fmt.Errorf("VRF source display requires a finite nonnegative observation")
	}
	budget := math.Round(value * 1000)
	if !epathOracleFinite(budget) || budget > float64(epathSQLVRFMaxExactMilli) {
		return 0, fmt.Errorf("VRF source display exceeds exact integer range")
	}
	return int64(budget), nil
}

func epathSQLVRFMilliAdd(left, right int64) (int64, error) {
	if left < 0 || right < 0 || left > epathSQLVRFMaxExactMilli-right {
		return 0, fmt.Errorf("VRF display total exceeds exact nonnegative integer range")
	}
	return left + right, nil
}

// Exact rational quotas make the independent tie decision insensitive to the
// evaluation order of floating-point additions. The budget is rounded first;
// this deliberately does not round a continuous source*share quota instead.
func epathSQLVRFMilliShares(budget int64, weights map[string]float64, zones []string) (map[string]int64, int64, error) {
	if budget < 0 || budget > epathSQLVRFMaxExactMilli || len(zones) == 0 || len(zones) != len(weights) {
		return nil, 0, fmt.Errorf("invalid VRF allocation budget/owner set")
	}
	seen := map[string]bool{}
	total := new(big.Rat)
	for _, zone := range zones {
		value, found := weights[zone]
		if zone == "" || seen[strings.ToLower(zone)] || !found || !epathOracleFinite(value) || value < 0 {
			return nil, 0, fmt.Errorf("invalid VRF allocation owner/weight")
		}
		seen[strings.ToLower(zone)] = true
		total.Add(total, new(big.Rat).SetFloat64(value))
	}
	out := map[string]int64{}
	if total.Sign() == 0 {
		for _, zone := range zones {
			out[zone] = 0
		}
		return out, budget, nil
	}
	type remainder struct {
		zone     string
		fraction *big.Rat
	}
	remainders := make([]remainder, 0, len(zones))
	remaining := budget
	for _, zone := range zones {
		quota := new(big.Rat).Mul(new(big.Rat).SetInt64(budget), new(big.Rat).SetFloat64(weights[zone]))
		quota.Quo(quota, total)
		floor := new(big.Int).Quo(quota.Num(), quota.Denom()).Int64()
		out[zone] = floor
		remaining -= floor
		remainders = append(remainders, remainder{zone: zone, fraction: new(big.Rat).Sub(quota, new(big.Rat).SetInt64(floor))})
	}
	if remaining < 0 || remaining >= int64(len(zones)) {
		return nil, 0, fmt.Errorf("invalid VRF largest-remainder closure")
	}
	sort.Slice(remainders, func(i, j int) bool {
		comparison := remainders[i].fraction.Cmp(remainders[j].fraction)
		if comparison != 0 {
			return comparison > 0
		}
		return remainders[i].zone < remainders[j].zone
	})
	for index := int64(0); index < remaining; index++ {
		out[remainders[index].zone]++
	}
	return out, 0, nil
}

func epathSQLVRFHasID(ids []int, wanted int) bool {
	for _, id := range ids {
		if id == wanted {
			return true
		}
	}
	return false
}
