package simulation

// Native physical bounds are independent of the source's later one-stage3dp
// transport. No graph/candidate value participates in these nine equations.
import (
	"fmt"
	"math"
	"sort"
	"time"
)

func epathSQLPVBalanceNativeQuantity(value float64) epathSQLQuantity {
	q := epathSQLQuantity{Value: value}
	// Same native, pre-presentation envelope as independent Pool source proof.
	// Actual zero remains exact; sign/knownness is separately source-validated.
	if value != 0 {
		q.Error = 1e-10 * math.Max(1, math.Abs(value))
	}
	return q
}

type epathSQLPVNativeEquation struct {
	ID               string
	Terms            map[string]float64
	FacilityDeadband bool
}

func epathSQLPVNativeEquations() []epathSQLPVNativeEquation {
	return []epathSQLPVNativeEquation{
		{ID: "pv_subtotal", Terms: map[string]float64{"pv.dc.1": 1, "pv.dc.2": 1, "pv.dc.3": 1, "pv.dc.4": 1, "pv.dc.5": 1, "distribution.electricity": -1}},
		{ID: "dc_storage_bus", Terms: map[string]float64{"inverter.dc_input": 1, "pv.dc.1": -1, "pv.dc.2": -1, "pv.dc.3": -1, "pv.dc.4": -1, "pv.dc.5": -1, "storage.discharge": -1, "storage.charge": 1}},
		{ID: "inverter_conversion", Terms: map[string]float64{"inverter.ac_output": 1, "inverter.dc_input": -1, "inverter.loss": 1}},
		{ID: "storage_decrement", Terms: map[string]float64{"storage.decrement": 1, "storage.charge": 1}},
		{ID: "inverter_decrement", Terms: map[string]float64{"inverter.loss_decrement": 1, "inverter.loss": 1}},
		{ID: "produced_membership", Terms: map[string]float64{"facility.produced": 1, "pv.dc.1": -1, "pv.dc.2": -1, "pv.dc.3": -1, "pv.dc.4": -1, "pv.dc.5": -1, "storage.discharge": -1, "storage.decrement": -1, "inverter.loss_decrement": -1}},
		{ID: "net_ac_supply", Terms: map[string]float64{"facility.produced": 1, "inverter.ac_output": -1}},
		{ID: "cogeneration_sole_member", Terms: map[string]float64{"cogeneration.electricity": 1, "inverter.ancillary": -1}},
		{ID: "facility_purchase_sale", Terms: map[string]float64{"facility.demand": 1, "facility.purchased": -1, "facility.produced": -1, "facility.sold": 1}, FacilityDeadband: true},
	}
}

func epathSQLPVCheckNativeBalancePeriod(values map[string]epathSQLQuantity, coveredSeconds float64) error {
	if !epathOracleFinite(coveredSeconds) || coveredSeconds <= 0 || coveredSeconds > 366*86400 {
		return fmt.Errorf("PV balance has invalid native covered duration")
	}
	for _, equation := range epathSQLPVNativeEquations() {
		sum := epathSQLQuantity{}
		// Stable term order avoids map-dependent accumulation and diagnostics.
		keys := make([]string, 0, len(equation.Terms))
		for key := range equation.Terms {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			q, found := values[key]
			if !found || !q.valid() {
				return fmt.Errorf("PV %s lacks finite known native source %s", equation.ID, key)
			}
			if key == "storage.decrement" || key == "inverter.loss_decrement" {
				if q.Value > 0 {
					return fmt.Errorf("PV signed native decrement became positive")
				}
			} else if key != "facility.produced" && q.Value < 0 {
				return fmt.Errorf("PV nonnegative native source %s became negative", key)
			}
			sum = sum.add(q.times(equation.Terms[key]))
		}
		if equation.FacilityDeadband {
			// EPSM v25.1 lines496/504: |net power|<0.0001W is set to0.
			// This is a physical reporting deadband, not a source rounding rule.
			limit := 0.0001 * coveredSeconds / 3600000
			sum = sum.add(epathSQLQuantity{Value: 0, Error: limit})
		}
		if !sum.includesZero() {
			low, high := sum.bounds()
			return fmt.Errorf("PV native %s residual=%g interval=[%g,%g] excludes zero", equation.ID, sum.Value, low, high)
		}
	}
	return nil
}

type epathSQLPVBalanceCell struct {
	TimeIndex, Month, Day, Hour int
	Seconds                     float64
	Quantity                    epathSQLQuantity
}

func epathSQLPVBalanceCells(identity epathSQLPVSourceIdentity) (map[int]epathSQLPVBalanceCell, error) {
	out := map[int]epathSQLPVBalanceCell{}
	for _, row := range identity.Rows {
		slot := row.Month - 1
		if identity.Dictionary.Frequency == "Hourly" {
			date := time.Date(row.Year, time.Month(row.Month), row.Day, 0, 0, 0, 0, time.UTC)
			slot = (date.YearDay()-1)*24 + row.Hour - 1
		}
		if _, duplicate := out[slot]; duplicate {
			return nil, fmt.Errorf("PV balance duplicate native slot")
		}
		out[slot] = epathSQLPVBalanceCell{TimeIndex: row.TimeIndex, Month: row.Month, Day: row.Day, Hour: row.Hour, Seconds: row.IntervalMinutes * 60, Quantity: epathSQLPVBalanceNativeQuantity(row.NativeJ / 3600000)}
	}
	return out, nil
}

func epathSQLValidatePVNativeBalances(core epathSQLPVSourceFrames, cg epathSQLPVCogenerationFrames) error {
	// This is the full compile/evaluate/registry boundary. Call once, not80
	// times for each source's independently checked raw/effective scalar.
	if err := epathSQLValidatePVSourceFrames(core); err != nil {
		return err
	}
	if err := epathSQLValidatePVCogenerationFrames(core, cg); err != nil {
		return err
	}
	for _, frequency := range []string{"Monthly", "Hourly"} {
		families := map[string]map[int]epathSQLPVBalanceCell{}
		for _, identity := range core.Sources {
			if identity.Dictionary.Frequency != frequency {
				continue
			}
			if _, duplicate := families[identity.Spec.ID]; duplicate {
				return fmt.Errorf("PV balance duplicate source family")
			}
			cells, err := epathSQLPVBalanceCells(identity)
			if err != nil {
				return err
			}
			families[identity.Spec.ID] = cells
		}
		parent, exists := cg.Parents[frequency]
		if !exists {
			return fmt.Errorf("PV balance absent native Cogeneration parent")
		}
		cells, err := epathSQLPVBalanceCells(parent)
		if err != nil {
			return err
		}
		families[parent.Spec.ID] = cells
		count := 12
		if frequency == "Hourly" {
			count = 8760
		}
		anchor, exists := families["facility.demand"]
		if !exists || len(anchor) != count {
			return fmt.Errorf("PV balance has no complete native Facility calendar")
		}
		annual := map[string]epathSQLQuantity{}
		annualSeconds := 0.0
		for slot := 0; slot < count; slot++ {
			calendar, exists := anchor[slot]
			if !exists {
				return fmt.Errorf("PV balance has a missing native calendar slot")
			}
			values := map[string]epathSQLQuantity{}
			for family, points := range families {
				point, exists := points[slot]
				if !exists || len(points) != count || point.TimeIndex != calendar.TimeIndex || point.Month != calendar.Month || point.Day != calendar.Day || point.Hour != calendar.Hour || point.Seconds != calendar.Seconds {
					return fmt.Errorf("PV balance %s %s borrowed another native time axis", frequency, family)
				}
				values[family] = point.Quantity
				if frequency == "Monthly" {
					annual[family] = annual[family].add(point.Quantity)
				}
			}
			if err := epathSQLPVCheckNativeBalancePeriod(values, calendar.Seconds); err != nil {
				return fmt.Errorf("%s slot%d: %w", frequency, slot, err)
			}
			if frequency == "Monthly" {
				annualSeconds += calendar.Seconds
			}
		}
		if frequency == "Monthly" {
			if err := epathSQLPVCheckNativeBalancePeriod(annual, annualSeconds); err != nil {
				return fmt.Errorf("annual=sum(native12months): %w", err)
			}
		}
	}
	return nil
}
