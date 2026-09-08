package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
)

// The ordinary numeric reader deliberately excludes dictionaries without any
// ReportData. For the precision proof only, retain that missing-member fact.
// This must not change the numeric alias winner: an unavailable alias of an
// already selected physical surface is not another required observation.
func invalidateMissingEnergyDriverMonthlySurfaceShadows(db *sql.DB, dictionaries []energyExplanationDictionary, selected map[int]energySurfacePeriodSelection, context energyDriverBuildContext, categories map[string]*energyExplanationCategorySeriesBuilder) {
	if !context.Enabled || len(categories) == 0 {
		return
	}
	tracked := false
	for _, category := range categories {
		tracked = tracked || category.seriesBuilder.driverMonthlyShadow != nil
	}
	if !tracked {
		return
	}
	invalidateAll := func() {
		for _, category := range categories {
			if category.seriesBuilder.driverMonthlyShadow != nil {
				category.seriesBuilder.driverMonthlyShadow.invalid = true
			}
		}
	}
	columns, err := sqlTableColumns(db, "ReportDataDictionary")
	if err != nil {
		invalidateAll()
		return
	}
	rows, err := db.Query(fmt.Sprintf(`SELECT %s,%s,%s,%s FROM ReportDataDictionary`,
		sqlTextColumnExpr(columns, "KeyValue", "''"), sqlTextColumnExpr(columns, "Name", "''"),
		sqlTextColumnExpr(columns, "Units", "''"), sqlTextColumnExpr(columns, "ReportingFrequency", "''")))
	if err != nil {
		invalidateAll()
		return
	}
	defer rows.Close()
	selectedPhysical := map[string]bool{}
	for _, dictionary := range dictionaries {
		if selection, ok := selected[dictionary.row.index]; ok && selection.monthly {
			item := energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{dictionary: dictionary}, "")
			selectedPhysical[energyExplanationSeriesSelectionKey(item)] = true
		}
	}
	for rows.Next() {
		var key, name, unit, frequency string
		if err := rows.Scan(&key, &name, &unit, &frequency); err != nil {
			invalidateAll()
			return
		}
		if !strings.EqualFold(strings.TrimSpace(frequency), "Monthly") {
			continue
		}
		definition, ok := energyHeatAliasDefinitionForName(name)
		if !ok || !definition.SurfaceScoped || energyDriverSourcePolicyFor(name, definition.Kind).Role != energyDriverSourceRoleMainFlow {
			continue
		}
		if energyPathRadiantSurfaceIsContext(context, key) {
			continue
		}
		dictionary := energyExplanationDictionary{row: sqlOutputDictionaryRow{keyValue: key, name: name, units: unit}, heat: &definition, reportingFrequency: frequency}
		item := energyExplanationSeriesForBuilder(&energyExplanationSeriesBuilder{dictionary: dictionary}, "")
		if selectedPhysical[energyExplanationSeriesSelectionKey(item)] {
			continue
		}
		category, _ := context.SurfaceCategories.resolve(key)
		if builder := categories[energyExplanationCategoryBuilderKey(dictionary, category)]; builder != nil && builder.seriesBuilder.driverMonthlyShadow != nil {
			builder.seriesBuilder.driverMonthlyShadow.invalid = true
		}
	}
	if rows.Err() != nil {
		invalidateAll()
	}
}

// Private calculation evidence, not another reported value. A nil shadow is
// untracked legacy input; a nonnil shadow with no month is unavailable, not 0.
// Values begin as unrounded native-Zone Monthly SQL energy. Derived shadows
// already contain effective signed arithmetic and must not be multiplied twice.
type energyDriverMonthlyShadow struct {
	values    map[int]float64
	invalid   bool
	effective bool
}

func cloneEnergyDriverMonthlyShadow(input *energyDriverMonthlyShadow) *energyDriverMonthlyShadow {
	if input == nil {
		return nil
	}
	out := *input
	out.values = cloneEnergyExplanationPeriodValues(input.values)
	return &out
}

func accumulateEnergyDriverMonthlyShadow(builder *energyExplanationSeriesBuilder, row SQLSeriesRow, raw, hours float64) {
	shadow := builder.driverMonthlyShadow
	if shadow == nil || shadow.invalid {
		return
	}
	if !row.Month.Valid || row.Month.Int64 < 1 || row.Month.Int64 > 12 || !strings.EqualFold(strings.TrimSpace(builder.dictionary.reportingFrequency), "Monthly") {
		shadow.invalid = true
		return
	}
	value, unit := convertEnergySQLValue(raw, builder.dictionary.row.units)
	if energyExplanationIntegratesRate(builder.dictionary) {
		if hours <= 0 || !energyPathFinite(hours) {
			shadow.invalid = true
			return
		}
		switch normalizeUnitToken(builder.dictionary.row.units) {
		case "w":
			value, unit = raw*hours/1000, "kWh"
		case "kw":
			value, unit = raw*hours, "kWh"
		default:
			shadow.invalid = true
			return
		}
	}
	if unit != "kWh" || !energyPathFinite(value) {
		shadow.invalid = true
		return
	}
	if shadow.values == nil {
		shadow.values = map[int]float64{}
	}
	month := int(row.Month.Int64)
	shadow.values[month] += value
	if !energyPathFinite(shadow.values[month]) {
		shadow.invalid = true
	}
}

func signedEnergyDriverMonthlyShadow(item energyExplanationSeries, sign float64) *energyDriverMonthlyShadow {
	shadow := cloneEnergyDriverMonthlyShadow(item.driverMonthlyShadow)
	if shadow == nil {
		return nil
	}
	factor := 1.0
	if !shadow.effective {
		if !item.multiplierApplied || item.MultiplierApplication == energyMultiplierUnknown || !energyPathFinite(item.EffectiveMultiplier) || item.EffectiveMultiplier <= 0 {
			shadow.invalid = true
		} else {
			factor = item.EffectiveMultiplier
		}
	}
	shadow.effective = true
	for month, value := range shadow.values {
		shadow.values[month] = value * factor * sign
		if !energyPathFinite(shadow.values[month]) {
			shadow.invalid = true
		}
	}
	return shadow
}

// Only the intersection is known arithmetic. Missing a contributing month
// cannot be interpreted as a reported zero or filled from rounded metadata.
func combineEnergyDriverMonthlyShadow(left, right *energyDriverMonthlyShadow, first bool, sign float64) *energyDriverMonthlyShadow {
	if first {
		out := cloneEnergyDriverMonthlyShadow(right)
		if out != nil {
			for month, value := range out.values {
				out.values[month] = value * sign
			}
		}
		return out
	}
	if left == nil && right == nil {
		return nil
	}
	out := &energyDriverMonthlyShadow{effective: true, values: map[int]float64{}}
	if left == nil || right == nil || left.invalid || right.invalid {
		out.invalid = true
		return out
	}
	for month, value := range left.values {
		if other, ok := right.values[month]; ok {
			total := value + sign*other
			if !energyPathFinite(total) {
				out.invalid = true
				return out
			}
			out.values[month] = total
		}
	}
	return out
}

func applyEnergyDriverReconciliationMonthlyPrecision(vector *energyDriverVector) {
	if vector.monthlyShadow == nil {
		return
	}
	vector.monthly = nil
	if !vector.monthlyShadow.invalid {
		vector.monthly = cloneEnergyExplanationPeriodValues(vector.monthlyShadow.values)
	}
	// Canonical annual graphs are sums of completed months. Do not preserve an
	// independently rounded synthetic annual residual as another authority.
	vector.total = sumEnergyExplanationPeriodValues(vector.monthly)
}

func energyDriverMonthlyShadowHasObservation(shadow *energyDriverMonthlyShadow) bool {
	return shadow != nil && !shadow.invalid && len(shadow.values) > 0
}

func absoluteEnergyDriverMonthlyShadow(input *energyDriverMonthlyShadow) *energyDriverMonthlyShadow {
	out := cloneEnergyDriverMonthlyShadow(input)
	if out != nil {
		for month, value := range out.values {
			out.values[month] = math.Abs(value)
		}
	}
	return out
}
