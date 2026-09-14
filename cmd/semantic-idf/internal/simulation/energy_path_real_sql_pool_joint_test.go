package simulation

// Independent, cross-scope conservation of presented native CW allocations.
// Individual proportional envelopes do not prove their joint integer budget.
// This checker does not run the production allocator or repair the candidate.
import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

func epathSQLPoolPumpJointMilli(value float64) (int64, error) {
	if !epathOracleFinite(value) || value < 0 {
		return 0, fmt.Errorf("invalid presented Pool pump quantity")
	}
	scaled := value * 1000
	units := math.Round(scaled)
	if !epathOracleFinite(scaled) || units > 1<<53 || units == 0 && value != 0 {
		return 0, fmt.Errorf("Pool pump quantity is not an exact nonnegative display quantum")
	}
	// Only binary representation error, never another display/SQL error budget.
	spacing := math.Nextafter(math.Abs(scaled), math.Inf(1)) - math.Abs(scaled)
	if math.Abs(scaled-units) > 4*spacing {
		return 0, fmt.Errorf("Pool pump quantity has subquantum precision")
	}
	return int64(units), nil
}

func epathSQLPoolPumpJointAdd(a, b int64) (int64, error) {
	if a < 0 || b < 0 || a > 1<<53-b {
		return 0, fmt.Errorf("Pool pump joint integer budget overflow")
	}
	return a + b, nil
}

func epathSQLPoolPumpJointPeriodCensus(periods []EnergyPeriod) error {
	seen := map[string]bool{}
	for _, period := range periods {
		if !epathOracleValidPeriod(period.ID) || seen[period.ID] || period.ID == "annual" && period.Kind != "annual" || period.ID != "annual" && period.Kind != "monthly" {
			return fmt.Errorf("Pool joint proof has duplicate/foreign/mistyped period")
		}
		seen[period.ID] = true
	}
	for month := 1; month <= 12; month++ {
		if !seen[fmt.Sprintf("M%d", month)] {
			return fmt.Errorf("Pool joint proof lacks an original monthly graph")
		}
	}
	return nil
}

func epathSQLPoolPumpJointZoneValue(nodes []EnergyExplanationNode, zone, period string, owned bool, want epathSQLQuantity) (int64, error) {
	count, units := 0, int64(0)
	for _, node := range nodes {
		// The category OR canonical identity catches a contradictory disguise.
		canonical := node.ID == "end_use.pumps" || strings.HasPrefix(node.ID, "end_use.pumps.") || node.Kind == "end_use.pumps" || node.Kind == "energy.pumps"
		if node.EndUse != "pumps" && !canonical {
			continue
		}
		count++
		if count != 1 || !owned || node.Level != "end_use" || node.EndUse != "pumps" || node.Period != period || !strings.EqualFold(node.ZoneName, zone) || node.Unit != "kWh" || node.ScaleDomain != "site" || node.AggregationBasis != "model_total" || node.Basis != "service_path_allocation" || !node.AllocationApplied || node.Carrier != "" || node.ServiceKind != "" && node.ServiceKind != "pumps" {
			return 0, fmt.Errorf("Pool joint proof found duplicate/unowned/mistyped pump node")
		}
		if node.inspectorDecodedFromJSON && node.inspectorValuePresence&4 == 0 {
			return 0, fmt.Errorf("present Pool pump allocatedValue is absent/null, not zero")
		}
		value, err := epathSQLPoolPumpJointMilli(node.Value)
		if err != nil {
			return 0, err
		}
		allocated, err := epathSQLPoolPumpJointMilli(node.AllocatedValue)
		if err != nil {
			return 0, err
		}
		if value != allocated {
			return 0, fmt.Errorf("Pool pump node value and allocatedValue disagree")
		}
		if err := epathCheckSQLModelQuantity(&node.Value, &want); err != nil {
			return 0, err
		}
		units = value
	}
	if count == 0 && !want.includesZero() {
		return 0, fmt.Errorf("Pool joint proof lost a required positive pump node")
	}
	return units, nil
}

func epathSQLPoolPumpJointLedgerValue(rows []EnergyReconciliation, consumer *epathSQLPoolPumpConsumer, calculation epathSQLPoolPumpMath, period string) (int64, error) {
	id, err := epathSQLAllocationID(consumer.Auxiliary.ReconciliationID, period)
	if err != nil {
		return 0, err
	}
	method, err := epathSQLPoolPumpAllocationMethod(calculation, period)
	if err != nil {
		return 0, err
	}
	expected, allocated := epathSQLQuantity{}, epathSQLQuantity{}
	for _, month := range epathSQLPeriodMonths(period) {
		expected = expected.add(calculation.Months[month-1].Expected)
		allocated = allocated.add(calculation.Months[month-1].Allocated)
	}
	count, units := 0, int64(0)
	for _, row := range rows {
		if row.ID != id {
			continue
		}
		count++
		if count != 1 || row.Level != "allocation" || row.Period != period || row.ZoneName != "" || row.Unit != "kWh" || row.AllocationMethod != method {
			return 0, fmt.Errorf("Pool joint proof has an ambiguous or mistyped Building ledger")
		}
		if err := epathCheckSQLModelQuantity(&row.AllocatedValue, &allocated); err != nil {
			return 0, err
		}
		units, err = epathSQLPoolPumpJointMilli(row.AllocatedValue)
		if err != nil {
			return 0, err
		}
	}
	if count == 0 && (!expected.includesZero() || !allocated.includesZero()) {
		return 0, fmt.Errorf("Pool joint proof lacks a non-prunable Building ledger")
	}
	return units, nil
}

func epathSQLCheckPoolPumpJointBudget(bundle PurposeResultBundle, consumer *epathSQLPoolPumpConsumer) error {
	if consumer == nil || len(consumer.Model.PoolSystems) != 1 || !reflect.DeepEqual(consumer.Model.PoolSystems[0], consumer.Native.Original.Declaration) {
		return fmt.Errorf("Pool joint proof requires its exact original/native consumer")
	}
	aux := consumer.Auxiliary
	if aux.SiteID == "" || aux.Weight != "cooling" || aux.AllocationMethod != "service_load_share" || aux.WeightSource != nil {
		return fmt.Errorf("Pool joint proof changed its source-local policy")
	}
	served, err := epathSQLDeclaredZones(consumer.Frames, aux.ServedZones)
	if err != nil || len(served) != len(consumer.Native.Original.ServedZones) {
		return fmt.Errorf("Pool joint proof changed original recipient census")
	}
	for _, zone := range consumer.Native.Original.ServedZones {
		if !served[strings.ToLower(zone)] {
			return fmt.Errorf("Pool joint proof borrowed a foreign Zone")
		}
	}
	calculation, err := epathSQLPoolPumpMathFromSources(consumer.Native, consumer.Frames, consumer.Model)
	if err != nil {
		return err
	}
	result := bundle.EnergyExplanation
	if result.Schema != energyExplanationSchema || result.Scope.Kind != "building" || result.Scope.ZoneName != "" {
		return fmt.Errorf("Pool joint proof requires original Building wrapper")
	}
	if err := epathSQLPoolPumpJointPeriodCensus(result.Periods); err != nil {
		return err
	}
	zones, seenZones := []string{}, map[string]bool{}
	for _, wrapper := range result.ZoneResults {
		key := strings.ToLower(wrapper.Scope.ZoneName)
		if wrapper.Scope.Kind != "zone" || seenZones[key] || consumer.Frames.Zones[key].Name == "" {
			return fmt.Errorf("Pool joint proof has duplicate/foreign Zone wrapper")
		}
		seenZones[key] = true
		if err := epathSQLPoolPumpJointPeriodCensus(wrapper.Periods); err != nil {
			return err
		}
	}
	if len(seenZones) != len(consumer.Frames.Zones) {
		return fmt.Errorf("Pool joint proof lacks one of the six original Zones")
	}
	for _, identity := range consumer.Frames.Zones {
		zones = append(zones, identity.Name)
	}
	sort.Strings(zones)
	zoneAnnual, zoneMonths, building := map[string]int64{}, map[string]int64{}, map[string]int64{}
	for _, period := range epathSQLZoneCarrierPeriods() {
		_, _, rows, _, err := epathOracleGraph(bundle, "building", "", period)
		if err != nil {
			return err
		}
		ledger, err := epathSQLPoolPumpJointLedgerValue(rows, consumer, calculation, period)
		if err != nil {
			return err
		}
		building[period] = ledger
		sum := int64(0)
		for _, zone := range zones {
			key := strings.ToLower(zone)
			want := epathSQLQuantity{}
			for _, month := range epathSQLPeriodMonths(period) {
				want = want.add(calculation.Months[month-1].Shares[key])
			}
			nodes, _, _, _, err := epathOracleGraph(bundle, "zone", zone, period)
			if err != nil {
				return err
			}
			units, err := epathSQLPoolPumpJointZoneValue(nodes, zone, period, served[key], want)
			if err != nil {
				return fmt.Errorf("%s/%s: %w", zone, period, err)
			}
			sum, err = epathSQLPoolPumpJointAdd(sum, units)
			if err != nil {
				return err
			}
			if period == "annual" {
				zoneAnnual[key] = units
			} else {
				zoneMonths[key], err = epathSQLPoolPumpJointAdd(zoneMonths[key], units)
				if err != nil {
					return err
				}
			}
		}
		if sum != ledger {
			return fmt.Errorf("Pool %s Zone pump sum %d differs from Building allocated budget %d milli-kWh", period, sum, ledger)
		}
	}
	monthlyBuilding := int64(0)
	for month := 1; month <= 12; month++ {
		monthlyBuilding, err = epathSQLPoolPumpJointAdd(monthlyBuilding, building[fmt.Sprintf("M%d", month)])
		if err != nil {
			return err
		}
	}
	if monthlyBuilding != building["annual"] {
		return fmt.Errorf("Pool annual Building allocation is not its completed monthly sum")
	}
	for _, zone := range zones {
		key := strings.ToLower(zone)
		if zoneAnnual[key] != zoneMonths[key] {
			return fmt.Errorf("Pool annual Zone allocation is not its completed monthly sum")
		}
	}
	// Explicit annual aliases are independent stored graphs. Do not let the
	// root-only annual selector hide contradictory cached annual pump amounts.
	for _, period := range result.Periods {
		if period.ID == "annual" {
			units, err := epathSQLPoolPumpJointLedgerValue(period.Reconciliation, consumer, calculation, "annual")
			if err != nil {
				return err
			}
			if units != building["annual"] {
				return fmt.Errorf("Pool Building annual alias changed its allocated budget")
			}
		}
	}
	for _, wrapper := range result.ZoneResults {
		for _, period := range wrapper.Periods {
			if period.ID == "annual" {
				key := strings.ToLower(wrapper.Scope.ZoneName)
				units, err := epathSQLPoolPumpJointZoneValue(period.Nodes, wrapper.Scope.ZoneName, "annual", served[key], calculation.AnnualShares[key])
				if err != nil {
					return err
				}
				if units != zoneAnnual[key] {
					return fmt.Errorf("Pool Zone annual alias changed its allocation")
				}
			}
		}
	}
	return epathSQLCheckPoolPumpJointScopes(bundle, consumer, calculation, zoneAnnual, building["annual"])
}

func epathSQLCheckPoolPumpJointScopes(bundle PurposeResultBundle, consumer *epathSQLPoolPumpConsumer, calculation epathSQLPoolPumpMath, annual map[string]int64, budget int64) error {
	cw := consumer.Native.Families["pump.cw.electricity"]
	canonicalID := fmt.Sprintf("sql-rdd-%d", cw.CanonicalID)
	nativeIDs := map[string]epathSQLPoolSourceIdentity{}
	for id, identity := range consumer.Native.Sources {
		nativeIDs[fmt.Sprintf("sql-rdd-%d", id)] = identity
	}
	sources := map[string]EnergyDataSource{}
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID == "" {
			return fmt.Errorf("Pool joint source has no ID")
		}
		if _, duplicate := sources[source.ID]; duplicate {
			return fmt.Errorf("duplicate Pool joint source ID")
		}
		sources[source.ID] = source
	}
	for id, identity := range nativeIDs {
		source, exists := sources[id]
		if !exists {
			return fmt.Errorf("Pool joint proof lost a native source/companion")
		}
		if source.Name != identity.Source.Name || !strings.EqualFold(source.KeyValue, identity.Source.KeyValue) || source.ReportingFrequency != identity.Source.ReportingFrequency || source.SourceUnit != identity.Source.SourceUnit || source.NormalizedUnit != "kWh" {
			return fmt.Errorf("Pool joint scope borrowed a different native source")
		}
		if id != canonicalID {
			for _, detail := range source.ScopeDetails {
				if detail.Scope.Kind == "zone" {
					return fmt.Errorf("HW/boiler/pool/companion acquired an unsupported Zone allocation")
				}
			}
		}
	}
	source := sources[canonicalID]
	if source.IsMeter || source.ZoneName != "" || source.AllocationApplied || source.AllocatedValue != 0 || source.AllocationFactor != 0 || source.EffectiveMultiplier != 1 || source.MultiplierApplication != "already_model_total" || source.AggregationBasis != "model_total" || source.RawValue != source.EffectiveValue {
		return fmt.Errorf("CW native global quantity became a multiplied/allocated Zone observation")
	}
	presence := source.observedValuePresence
	if source.inspectorDecodedFromJSON {
		presence = source.inspectorValuePresence
	}
	if presence&3 != 3 {
		return fmt.Errorf("CW global observation lost raw/effective knownness")
	}
	global := epathSQLQuantity{Value: cw.Annual.Value}
	if global.Value != 0 {
		global.Error = .0005 + cw.Annual.Error
	}
	if err := epathCheckSQLModelQuantity(&source.EffectiveValue, &global); err != nil {
		return err
	}
	if _, err := epathSQLPoolPumpJointMilli(source.EffectiveValue); err != nil {
		return err
	}
	eligible := map[string]bool{}
	for _, zone := range consumer.Native.Original.ServedZones {
		key := strings.ToLower(zone)
		for _, month := range calculation.Months {
			// Source detail rows follow actual positive primary load targets,
			// including known-zero CW budgets. All-known-zero primary Zones are
			// still denominator evidence, but do not have recipient rows.
			if math.Round(month.Weights[key].Value*1000) > 0 {
				eligible[key] = true
			}
		}
	}
	seen, sum := map[string]bool{}, int64(0)
	for _, detail := range source.ScopeDetails {
		if detail.Scope.Kind == "building" {
			if detail.Scope.ZoneName != "" {
				return fmt.Errorf("CW Building scope carries a Zone")
			}
			continue
		}
		key := strings.ToLower(detail.Scope.ZoneName)
		if detail.Scope.Kind != "zone" || !eligible[key] || seen[key] || detail.Scope.AggregationBasis != "model_total" || detail.AggregationBasis != "model_total" {
			return fmt.Errorf("CW detail has a duplicate/foreign/known-zero-primary scope")
		}
		seen[key] = true
		if !(detail.inspectorDecodedFromJSON || detail.inspectorScopedValuePresence) || detail.inspectorValuePresence&3 != 0 || detail.RawValue != 0 || detail.EffectiveValue != 0 || detail.EffectiveMultiplier != 0 || detail.MultiplierApplication != "" || !detail.AllocationApplied {
			return fmt.Errorf("CW allocated-only scope fabricated measured raw/effective values or lost known zero")
		}
		units, err := epathSQLPoolPumpJointMilli(detail.AllocatedValue)
		if err != nil {
			return err
		}
		if units != annual[key] {
			return fmt.Errorf("CW annual source detail differs from completed Zone graph allocation")
		}
		want := calculation.AnnualShares[key]
		if err := epathCheckSQLModelQuantity(&detail.AllocatedValue, &want); err != nil {
			return err
		}
		factor := 0.0
		if source.EffectiveValue > 0 {
			factor = detail.AllocatedValue / source.EffectiveValue
		}
		if !epathOracleFinite(detail.AllocationFactor) || detail.AllocationFactor < 0 || factor == 0 && detail.AllocationFactor != 0 || math.Abs(detail.AllocationFactor-factor) > 1e-12*math.Max(1, math.Abs(factor)) {
			return fmt.Errorf("CW detail factor does not use its own native global energy")
		}
		sum, err = epathSQLPoolPumpJointAdd(sum, units)
		if err != nil {
			return err
		}
	}
	if len(seen) != len(eligible) {
		return fmt.Errorf("CW source lost a required positive-primary recipient detail, including allocated zero")
	}
	if sum != budget {
		return fmt.Errorf("CW source scope sum differs from the joint Building allocation")
	}
	return nil
}
