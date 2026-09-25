package simulation

// Independent finite native graph transport. The values below come
// only from an already validated native frame, never from candidate amounts.
import (
	"fmt"
	"math"
)

type epathSQLPVGraphAmounts struct {
	Monthly      [12]epathSQLQuantity
	GraphAnnual  epathSQLQuantity
	SourceAnnual epathSQLQuantity // distinct once-rounded native annual source.
}

func epathSQLPVGraphRound(q epathSQLQuantity) (epathSQLQuantity, error) {
	if !q.valid() {
		return epathSQLQuantity{}, fmt.Errorf("PV graph native interval invalid")
	}
	lo, hi := q.bounds()
	round := func(x float64) float64 { return math.Round(x*1000) / 1000 }
	out := epathSQLBounded(round(q.Value), round(lo), round(hi))
	if !out.valid() {
		return out, fmt.Errorf("PV graph rounded native interval invalid")
	}
	return out, nil
}

func epathSQLPVGraphNativeAmounts(s epathSQLPVSourceIdentity) (epathSQLPVGraphAmounts, error) {
	var out epathSQLPVGraphAmounts
	// Existing native floating summation bounds, followed by the established
	// factor-one/3dp monthly stage; subsequent 3dp copies are idempotent.
	var err error
	out.SourceAnnual, err = epathSQLPVGraphRound(epathSQLPVBalanceNativeQuantity(s.NativeAnnualKWh))
	if err != nil {
		return out, err
	}
	for m, value := range s.NativeMonthlyKWh {
		out.Monthly[m], err = epathSQLPVGraphRound(epathSQLPVBalanceNativeQuantity(value))
		if err != nil {
			return out, err
		}
		// The v2 canonical annual graph sums monthly nodes/ribbons. Propagate
		// the reviewed per-merge rounding exactly; do not use SourceAnnual.
		out.GraphAnnual, err = epathSQLPVGraphRound(out.GraphAnnual.add(out.Monthly[m]))
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func (a epathSQLPVGraphAmounts) period(period string) (epathSQLQuantity, error) {
	if period == "annual" {
		if !a.GraphAnnual.valid() {
			return epathSQLQuantity{}, fmt.Errorf("invalid native annual graph quantity")
		}
		return a.GraphAnnual, nil
	}
	for m := 1; m <= 12; m++ {
		if period == fmt.Sprintf("M%d", m) {
			q := a.Monthly[m-1]
			if !q.valid() {
				return q, fmt.Errorf("invalid native monthly graph quantity")
			}
			return q, nil
		}
	}
	return epathSQLQuantity{}, fmt.Errorf("finite PV graph scalar requires exact annual/M1..M12 period")
}

func epathSQLPVGraphContains(q epathSQLQuantity, value float64) bool {
	lo, hi := q.bounds()
	return q.valid() && epathOracleFinite(value) && value == math.Round(value*1000)/1000 && value >= lo && value <= hi
}

func (w epathSQLPVGraphRoleWalk) supportAmounts(nodes []EnergyExplanationNode, links []EnergyPathLink, period string, zone bool) error {
	if zone {
		return nil
	} // Role guard forbids every native support in Zone scope.
	var demand epathSQLQuantity
	demandFound := false
	for _, p := range w.proof.Identities {
		if p.SpecID == "facility.demand" && p.Frequency == "Monthly" {
			var err error
			demand, err = p.Amounts.period(period)
			if err != nil {
				return err
			}
			demandFound = true
		}
	}
	if !demandFound {
		return fmt.Errorf("PV support lacks independent Facility demand presence")
	}
	demandLow, demandHigh := demand.bounds()
	for _, role := range []string{"storage.charge", "facility.purchased", "facility.produced", "facility.sold"} {
		var nativeID string
		var native epathSQLPVGraphIdentity
		for id, p := range w.proof.Identities {
			if p.SpecID == role && p.Frequency == "Monthly" {
				if nativeID != "" {
					return fmt.Errorf("duplicate independent Monthly support role")
				}
				nativeID, native = id, p
			}
		}
		if nativeID == "" {
			return fmt.Errorf("missing independent Monthly support role")
		}
		q, err := native.Amounts.period(period)
		if err != nil {
			return err
		}
		if role == "facility.produced" && w.negativeProduced {
			continue
		} // Structural gate already requires source-only.
		lo, hi := q.bounds()
		if lo < 0 || hi < 0 {
			return fmt.Errorf("nonnegative support has negative native transport interval")
		}
		count, edgeCount := 0, 0
		nodeID := ""
		for _, n := range nodes {
			if epathSQLPVGraphSupport(n.EndUse, n.Kind) != role {
				continue
			}
			count++
			nodeID = n.ID
			for _, field := range []struct {
				name  string
				value float64
			}{{"value", n.Value}, {"rawValue", n.RawValue}, {"effectiveValue", n.EffectiveValue}, {"allocatedValue", n.AllocatedValue}} {
				if !epathSQLPVGraphContains(q, field.value) {
					return fmt.Errorf("PV %s/%s %s escaped independent native transport", role, period, field.name)
				}
			}
			// These legacy optional display fields are absent (zero) or carry
			// that same native quantity. Zero grants no separate magnitude.
			for _, v := range []float64{n.DisplayValue, n.SignedValue} {
				if v != 0 && !epathSQLPVGraphContains(q, v) {
					return fmt.Errorf("PV support optional display/signed scalar changed")
				}
			}
			if n.Multiplier != 0 && n.Multiplier != 1 {
				return fmt.Errorf("global native support multiplied")
			}
		}
		if count > 1 || lo > 0 && count != 1 {
			return fmt.Errorf("positive native %s/%s missing or duplicated support node", role, period)
		}
		for _, l := range links {
			if l.Relation != "support_supply" || l.FromID != nodeID || nodeID == "" {
				continue
			}
			edgeCount++
			if !epathSQLPVGraphContains(q, l.FromValue) || !epathSQLPVGraphContains(q, l.ToValue) {
				return fmt.Errorf("PV %s/%s support ribbon escaped native transport", role, period)
			}
		}
		if role == "storage.charge" {
			if edgeCount != 0 {
				return fmt.Errorf("charge gained numeric flow")
			}
			continue
		}
		if edgeCount > 1 || lo > 0 && demandLow > 0 && edgeCount != 1 || demandHigh == 0 && edgeCount != 0 {
			return fmt.Errorf("native %s/%s support ribbon presence contradicts support/demand observation", role, period)
		}
		// Zero intervals may be pruned or retained as explicit zero records.
		// Missing a positive node/ribbon cannot be excused by zero candidates.
	}
	return nil
}
