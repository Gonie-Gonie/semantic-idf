package simulation

import (
	"fmt"
	"sort"
	"strings"
)

// This arithmetic is independent of the application allocator. Callers bind
// all three purchased observations and every effective primary cooling load to
// original/native source proofs. Neither a recipe flag nor a candidate source
// ID can establish those identities. A nil month is unavailable, never zero.
type epathSQLPoolPumpMathInput struct {
	Broad, HotWater, ChilledWater [12]*epathSQLQuantity
	Cooling                       map[string][12]*epathSQLQuantity
	ServedZones                   []string
	Precision                     epathRealSQLPrecision
}

type epathSQLPoolPumpMonthMath struct {
	Expected, HotWaterObserved, ChilledWaterObserved epathSQLQuantity
	Allocated, Unassigned, Denominator               epathSQLQuantity
	Weights, Shares                                  map[string]epathSQLQuantity
}

type epathSQLPoolPumpMath struct {
	Months                                            [12]epathSQLPoolPumpMonthMath
	AnnualExpected, AnnualAllocated, AnnualUnassigned epathSQLQuantity
	AnnualShares                                      map[string]epathSQLQuantity
}

func epathSQLCompilePoolPumpMath(input epathSQLPoolPumpMathInput) (epathSQLPoolPumpMath, error) {
	out := epathSQLPoolPumpMath{AnnualShares: map[string]epathSQLQuantity{}}
	if input.Precision.DecimalPlaces != 3 || input.Precision.SourceStages < 1 || input.Precision.SourceStages > 3 || input.Precision.ContributionStages < 1 || input.Precision.ContributionStages > 3 {
		return out, fmt.Errorf("Pool pump math lacks reviewed serialization-stage bounds")
	}
	served, zones := map[string]bool{}, []string{}
	for name := range input.Cooling {
		if name == "" || name != strings.ToLower(strings.TrimSpace(name)) {
			return out, fmt.Errorf("Pool pump load identities must be exact normalized original Zone keys")
		}
		zones = append(zones, name)
	}
	sort.Strings(zones)
	for _, name := range input.ServedZones {
		key := strings.ToLower(strings.TrimSpace(name))
		if _, exists := input.Cooling[key]; key == "" || key == "*" || served[key] || !exists {
			return out, fmt.Errorf("Pool pump served roster is missing, duplicated or foreign")
		}
		served[key] = true
	}
	if len(served) == 0 {
		return out, fmt.Errorf("Pool pump allocation requires a complete nonempty original recipient roster")
	}
	for month := 0; month < 12; month++ {
		row := epathSQLPoolPumpMonthMath{Weights: map[string]epathSQLQuantity{}, Shares: map[string]epathSQLQuantity{}}
		for _, q := range []*epathSQLQuantity{input.Broad[month], input.HotWater[month], input.ChilledWater[month]} {
			if q == nil || !q.valid() || q.Value < 0 {
				return out, fmt.Errorf("Pool pump M%d has an unavailable/negative purchased observation", month+1)
			}
		}
		row.Expected, row.HotWaterObserved, row.ChilledWaterObserved = *input.Broad[month], *input.HotWater[month], *input.ChilledWater[month]
		// The separately audited native meter/constituent closure is still
		// required. This verifies frame consistency; it does not derive CW by
		// subtracting HW from a meter or authorize a relaxed native tolerance.
		gap := row.Expected.add(row.HotWaterObserved.times(-1)).add(row.ChilledWaterObserved.times(-1))
		if !gap.includesZero() {
			return out, fmt.Errorf("Pool pump M%d observed constituent partition differs from its broad meter", month+1)
		}
		for _, zone := range zones {
			q := input.Cooling[zone][month]
			if q == nil || !q.valid() || q.Value < 0 {
				return out, fmt.Errorf("Pool pump %s/M%d lacks a known primary cooling observation", zone, month+1)
			}
			row.Weights[zone] = *q
			row.Shares[zone] = epathSQLQuantity{}
			if served[zone] {
				row.Denominator = row.Denominator.add(*q)
			}
		}
		if !row.Denominator.valid() {
			return out, fmt.Errorf("Pool pump M%d denominator overflow", month+1)
		}
		if row.Denominator.Value == 0 {
			_, high := row.Denominator.bounds()
			if high != 0 {
				return out, fmt.Errorf("Pool pump M%d zero denominator is uncertain, not proved zero", month+1)
			}
		} else {
			// Hamilton integer-quota apportionment differs from a proportional
			// share by less than a whole 0.001 kWh quantum, not merely the half
			// quantum of nearest rounding. Keep the generic helper unchanged;
			// this independently derived Pool-only bound must hold even for a
			// caller whose ordinary contribution-stage declaration is one.
			apportionmentPrecision := input.Precision
			if apportionmentPrecision.ContributionStages < 2 {
				apportionmentPrecision.ContributionStages = 2
			}
			for _, zone := range zones {
				if !served[zone] {
					continue
				}
				share, err := epathSQLShare(row.ChilledWaterObserved, row.Weights[zone], row.Denominator, apportionmentPrecision)
				if err != nil || !share.valid() || share.Value < 0 {
					return out, fmt.Errorf("Pool pump %s/M%d invalid source-local share: %v", zone, month+1, err)
				}
				row.Shares[zone] = share
				row.Allocated = row.Allocated.add(share)
			}
			// The apportionment preserves its integer budget jointly. Do not
			// inflate the total's error by summing independent recipient bounds.
			row.Allocated = row.ChilledWaterObserved
		}
		row.Unassigned = row.Expected.add(row.Allocated.times(-1)).positive()
		if !row.Allocated.valid() || row.Allocated.Value < 0 || !row.Unassigned.valid() {
			return out, fmt.Errorf("Pool pump M%d source-local ledger overflow", month+1)
		}
		out.Months[month] = row
		out.AnnualExpected = out.AnnualExpected.add(row.Expected)
		out.AnnualAllocated = out.AnnualAllocated.add(row.Allocated)
		out.AnnualUnassigned = out.AnnualUnassigned.add(row.Unassigned)
		for _, zone := range zones {
			out.AnnualShares[zone] = out.AnnualShares[zone].add(row.Shares[zone])
			if !out.AnnualShares[zone].valid() {
				return out, fmt.Errorf("Pool pump %s annual allocation overflow", zone)
			}
		}
		if !out.AnnualExpected.valid() || !out.AnnualAllocated.valid() || !out.AnnualUnassigned.valid() {
			return out, fmt.Errorf("Pool pump annual ledger overflow")
		}
	}
	return out, nil
}

// Bind math to independently compiled SQL/original evidence. In particular,
// one unserved plenum remains a measured load but is not a denominator member;
// every served Zone includes a genuine primary observation even when it is 0.
func epathSQLPoolPumpMathFromSources(pool epathSQLPoolSourceFrames, frames epathSQLFrames, model epathRealSQLModel) (epathSQLPoolPumpMath, error) {
	var empty epathSQLPoolPumpMath
	if model.OriginalZoneMultiplierProof != epathSQLOriginalMultiplierContract {
		return empty, fmt.Errorf("Pool pump math requires independently proved original Zone and Group multipliers")
	}
	if pool.Precision != model.Precision {
		return empty, fmt.Errorf("Pool native and model precision contracts differ")
	}
	if _, err := epathSQLPoolBoundaryOriginalZones(pool.Original); err != nil {
		return empty, err
	}
	if err := epathSQLValidatePoolSourceFrames(pool); err != nil {
		return empty, err
	}
	input := epathSQLPoolPumpMathInput{Cooling: map[string][12]*epathSQLQuantity{}, ServedZones: append([]string(nil), pool.Original.ServedZones...), Precision: model.Precision}
	parent := pool.Parents["Pumps:Electricity"]
	hw, cw := pool.Families["pump.hw.electricity"], pool.Families["pump.cw.electricity"]
	if len(parent.Monthly) != 12 || len(hw.Monthly) != 12 || len(cw.Monthly) != 12 {
		return empty, fmt.Errorf("Pool pump source-local budgets lack complete native months")
	}
	for month := 0; month < 12; month++ {
		b, h, c := parent.Monthly[month], hw.Monthly[month], cw.Monthly[month]
		input.Broad[month], input.HotWater[month], input.ChilledWater[month] = &b, &h, &c
	}
	wantedZones := map[string]bool{}
	for _, name := range append(append([]string(nil), pool.Original.ServedZones...), pool.Original.Declaration.ReturnPlenumZoneName) {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || wantedZones[key] {
			return empty, fmt.Errorf("Pool original served/plenum census is ambiguous")
		}
		wantedZones[key] = true
	}
	if len(wantedZones) != len(frames.Zones) {
		return empty, fmt.Errorf("Pool native Zone census differs from original proof")
	}
	for zone, identity := range frames.Zones {
		if !wantedZones[zone] || zone == "" || zone != strings.ToLower(identity.Name) || !epathOracleFinite(identity.Multiplier) || identity.Multiplier <= 0 {
			return empty, fmt.Errorf("Pool pump cooling denominator has invalid original Zone identity or factor")
		}
		var values [12]*epathSQLQuantity
		for month := 1; month <= 12; month++ {
			key := epathSQLKey(zone, "cooling", month)
			q, exists := frames.Loads[key]
			ids := frames.LoadSourceIDs[key]
			if !exists || len(ids) != 1 {
				return empty, fmt.Errorf("Pool pump cooling denominator lost its exact primary observation")
			}
			source, found := frames.SourceIdentities[ids[0]]
			if !found || source.Name != "Zone Air System Sensible Cooling Energy" || source.IsMeter || source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" || !strings.EqualFold(source.KeyValue, identity.Name) || source.DictionaryIndex != ids[0] {
				return empty, fmt.Errorf("Pool pump primary load substituted a humidity detail, wrong owner or foreign quantity")
			}
			original, err := epathSQLMonthly(source, model.Precision)
			if err != nil || len(original) != 12 {
				return empty, fmt.Errorf("Pool pump primary load has invalid native months: %v", err)
			}
			want := original[month-1].times(identity.Multiplier)
			if !epathSQLZoneCarrierQuantityEqual(q, want) {
				return empty, fmt.Errorf("Pool pump load no longer equals its native observation times one original effective multiplier")
			}
			values[month-1] = &q
		}
		input.Cooling[zone] = values
	}
	return epathSQLCompilePoolPumpMath(input)
}
