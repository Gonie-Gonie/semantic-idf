package simulation

import "fmt"

// The reviewed combustion declaration is a policy, not a promise that every
// period has load/fuel <= 1. Classify only from independently bounded paired
// quantities; a candidate's label or quotient is never an input.
func epathSQLConversionRatioKind(declared string, from, to epathSQLQuantity) (string, error) {
	if declared == "" || !from.valid() || !to.valid() || from.Value < 0 || to.Value < 0 {
		return "", fmt.Errorf("invalid independent conversion-kind evidence")
	}
	if declared != "efficiency" && declared != "load_to_fuel" {
		return declared, nil
	}
	fromLow, fromHigh := from.bounds()
	toLow, toHigh := to.bounds()
	if fromLow < 0 || toLow < 0 {
		return "", fmt.Errorf("conversion-kind bounds must be nonnegative")
	}
	if fromHigh == 0 || toHigh == 0 {
		return declared, nil // No positive pair is possible; ratio stays unavailable.
	}
	const threshold = 1 + 1e-9
	if fromLow > threshold*toHigh {
		return "load_to_fuel", nil
	}
	if toLow > 0 && fromHigh <= threshold*toLow {
		return "efficiency", nil
	}
	return "", fmt.Errorf("independent load/fuel bounds straddle classification threshold: thermal [%g,%g], site [%g,%g]", fromLow, fromHigh, toLow, toHigh)
}

// A monthly pair may be absent because one endpoint rounds to zero. If it is
// present, its original independent bounds still determine the kind. Annual
// aggregation must also include the possibility that each such whole monthly
// pair was omitted; restoring every raw lower bound would hide that uncertainty.
func epathSQLConversionPeriodRatioKind(declared, period string, pairs [12]epathSQLConversionProof) (string, error) {
	if !epathOracleValidPeriod(period) {
		return "", fmt.Errorf("invalid conversion-kind period %q", period)
	}
	from, to := epathSQLQuantity{}, epathSQLQuantity{}
	for _, month := range epathSQLPeriodMonths(period) {
		pair := pairs[month-1]
		if !pair.From.valid() || !pair.To.valid() || pair.From.Value < 0 || pair.To.Value < 0 {
			return "", fmt.Errorf("unknown/invalid monthly conversion-kind interval")
		}
		f, t := pair.From.positive(), pair.To.positive()
		if period == "annual" && (f.includesZero() || t.includesZero()) {
			f, t = f.optionalPresentation(), t.optionalPresentation()
		}
		from, to = from.add(f), to.add(t)
	}
	return epathSQLConversionRatioKind(declared, from, to)
}
