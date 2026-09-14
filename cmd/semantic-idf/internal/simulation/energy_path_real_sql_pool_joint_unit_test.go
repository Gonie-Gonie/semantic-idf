package simulation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// Hand allocation matrix. No production graph builder, allocator or candidate.
func epathSQLPoolJointHandBundle(t *testing.T, consumer *epathSQLPoolPumpConsumer, allocations map[string][12]float64) PurposeResultBundle {
	t.Helper()
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}}
	byPeriod := map[string]float64{}
	for _, name := range append(append([]string(nil), consumer.Native.Original.ServedZones...), consumer.Native.Original.Declaration.ReturnPlenumZoneName) {
		key := strings.ToLower(name)
		monthly := allocations[key]
		annual := 0.0
		for _, value := range monthly {
			annual += value
		}
		annual = math.Round(annual*1000) / 1000
		wrapper := EnergyExplanationZoneResult{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: name, AggregationBasis: "model_total"}}
		node := func(period string, value float64) []EnergyExplanationNode {
			if value == 0 {
				return nil
			}
			return []EnergyExplanationNode{{ID: "end_use.pumps.service_path_allocation", Level: "end_use", Kind: "end_use.pumps", EndUse: "pumps", Label: "Pumps", Value: value, AllocatedValue: value, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", AggregationBasis: "model_total", Basis: "service_path_allocation", Period: period, ZoneName: name, inspectorDecodedFromJSON: true, inspectorValuePresence: 4}}
		}
		wrapper.Nodes = node("annual", annual)
		wrapper.Periods = append(wrapper.Periods, EnergyPeriod{ID: "annual", Kind: "annual", Nodes: node("annual", annual)})
		byPeriod["annual"] += annual
		for month, value := range monthly {
			period := fmt.Sprintf("M%d", month+1)
			wrapper.Periods = append(wrapper.Periods, EnergyPeriod{ID: period, Kind: "monthly", Nodes: node(period, value)})
			byPeriod[period] += value
		}
		result.ZoneResults = append(result.ZoneResults, wrapper)
		result.AvailableZones = append(result.AvailableZones, name)
	}
	parent := consumer.Native.Parents["Pumps:Electricity"]
	for _, period := range epathSQLZoneCarrierPeriods() {
		expected := 0.0
		for _, month := range epathSQLPeriodMonths(period) {
			expected += math.Round(parent.Monthly[month-1].Value*1000) / 1000
		}
		expected = math.Round(expected*1000) / 1000
		allocated := math.Round(byPeriod[period]*1000) / 1000
		method := "unassigned"
		if allocated > 0 {
			method = "service_load_share"
		}
		id, err := epathSQLAllocationID(consumer.Auxiliary.ReconciliationID, period)
		if err != nil {
			t.Fatal(err)
		}
		row := EnergyReconciliation{ID: id, Level: "allocation", Period: period, Unit: "kWh", ExpectedValue: expected, AllocatedValue: allocated, UnassignedValue: math.Round((expected-allocated)*1000) / 1000, ResidualValue: math.Round((expected-allocated)*1000) / 1000, AllocationMethod: method}
		if period == "annual" {
			result.Reconciliation = []EnergyReconciliation{row}
			result.Periods = append(result.Periods, EnergyPeriod{ID: period, Kind: "annual", Reconciliation: []EnergyReconciliation{row}})
		} else {
			result.Periods = append(result.Periods, EnergyPeriod{ID: period, Kind: "monthly", Reconciliation: []EnergyReconciliation{row}})
		}
	}
	for id, identity := range consumer.Native.Sources {
		value := math.Round(*identity.Source.EnergyKWh*1000) / 1000
		source := EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), Name: identity.Source.Name, KeyValue: identity.Source.KeyValue, ReportingFrequency: identity.Source.ReportingFrequency, SourceUnit: identity.Source.SourceUnit, NormalizedUnit: "kWh", RawValue: value, EffectiveValue: value, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", AggregationBasis: "model_total", observedValuePresence: 3}
		if id == consumer.Native.Families["pump.cw.electricity"].CanonicalID {
			for _, zone := range consumer.Native.Original.ServedZones {
				key := strings.ToLower(zone)
				eligible := false
				for month := 1; month <= 12; month++ {
					if math.Round(consumer.Frames.Loads[epathSQLKey(zone, "cooling", month)].Value*1000) > 0 {
						eligible = true
					}
				}
				if !eligible {
					continue
				}
				total := 0.0
				for _, amount := range allocations[key] {
					total += amount
				}
				total = math.Round(total*1000) / 1000
				factor := 0.0
				if value > 0 {
					factor = total / value
				}
				source.ScopeDetails = append(source.ScopeDetails, EnergyDataSourceScopeDetail{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: zone, AggregationBasis: "model_total"}, AggregationBasis: "model_total", AllocatedValue: total, AllocationFactor: factor, AllocationApplied: true, inspectorScopedValuePresence: true})
			}
		}
		result.Sources = append(result.Sources, source)
	}
	return PurposeResultBundle{EnergyExplanation: result}
}

func epathSQLPoolJointFixture(t *testing.T) (PurposeResultBundle, *epathSQLPoolPumpConsumer) {
	t.Helper()
	frames, model, aux := epathSQLPoolPumpConsumerHandFrames(t)
	consumer, err := epathSQLPoolPumpConsumerFor(frames, model, aux)
	if err != nil {
		t.Fatal(err)
	}
	allocations := map[string][12]float64{}
	for index, zone := range aux.ServedZones {
		monthly := [12]float64{}
		for month := 1; month <= 12; month++ {
			hours := float64(time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day() * 24)
			if index == 0 {
				monthly[month-1] = 5 * hours / 3
			}
			if index == 1 {
				monthly[month-1] = 10 * hours / 3
			}
		}
		allocations[strings.ToLower(zone)] = monthly
	}
	return epathSQLPoolJointHandBundle(t, consumer, allocations), consumer
}

func epathSQLPoolJointCWSource(t *testing.T, bundle *PurposeResultBundle, consumer *epathSQLPoolPumpConsumer) *EnergyDataSource {
	t.Helper()
	id := fmt.Sprintf("sql-rdd-%d", consumer.Native.Families["pump.cw.electricity"].CanonicalID)
	for i := range bundle.EnergyExplanation.Sources {
		if bundle.EnergyExplanation.Sources[i].ID == id {
			return &bundle.EnergyExplanation.Sources[i]
		}
	}
	t.Fatal("hand CW source missing")
	return nil
}

func epathSQLPoolJointOriginalWire(t *testing.T, bundle PurposeResultBundle) []byte {
	t.Helper()
	type plainNode EnergyExplanationNode
	type plainSource EnergyDataSource
	nodes := func(input []EnergyExplanationNode) []plainNode {
		out := []plainNode{}
		for _, node := range input {
			out = append(out, plainNode(node))
		}
		return out
	}
	periods := func(input []EnergyPeriod) []map[string]any {
		out := []map[string]any{}
		for _, period := range input {
			out = append(out, map[string]any{"id": period.ID, "kind": period.Kind, "nodes": nodes(period.Nodes), "links": period.Links, "reconciliation": period.Reconciliation})
		}
		return out
	}
	zones := []map[string]any{}
	for _, zone := range bundle.EnergyExplanation.ZoneResults {
		zones = append(zones, map[string]any{"scope": zone.Scope, "nodes": nodes(zone.Nodes), "links": zone.Links, "reconciliation": zone.Reconciliation, "periods": periods(zone.Periods)})
	}
	sources := []map[string]any{}
	for _, source := range bundle.EnergyExplanation.Sources {
		encoded, err := json.Marshal(plainSource(source))
		if err != nil {
			t.Fatal(err)
		}
		wire := map[string]any{}
		if err := json.Unmarshal(encoded, &wire); err != nil {
			t.Fatal(err)
		}
		// Actual global zero is a known observation; scope zeros deliberately
		// do not inherit that presence and are left untouched.
		wire["rawValue"], wire["effectiveValue"] = source.RawValue, source.EffectiveValue
		sources = append(sources, wire)
	}
	result := bundle.EnergyExplanation
	wire := map[string]any{"energyExplanation": map[string]any{"schema": result.Schema, "scope": result.Scope, "nodes": nodes(result.Nodes), "links": result.Links, "reconciliation": result.Reconciliation, "periods": periods(result.Periods), "zoneResults": zones, "availableZones": result.AvailableZones, "sources": sources}}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestEnergyPathRealSQLPoolJointBudgetAndScopeHandGraph(t *testing.T) {
	bundle, consumer := epathSQLPoolJointFixture(t)
	if err := epathSQLCheckPoolPumpJointBudget(bundle, consumer); err != nil {
		t.Fatal(err)
	}
	if len(epathSQLPoolJointCWSource(t, &bundle, consumer).ScopeDetails) != 2 {
		t.Fatal("known-zero primary Zones became source recipients")
	}
	// Serialize without compatibility repair; missing raw/effective in the
	// allocated-only detail remains unknown after the original-wire decoder.
	data := epathSQLPoolJointOriginalWire(t, bundle)
	decoded, err := epathDecodeOriginalOracleCandidate(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := epathSQLCheckPoolPumpJointBudget(decoded, consumer); err != nil {
		t.Fatal(err)
	}
}

func TestEnergyPathRealSQLPoolJointRejectsGraphAndScopeMutations(t *testing.T) {
	for _, mutation := range []string{"monthly joint", "annual Zone", "annual Building alias", "annual Zone alias", "node value", "node allocated null", "wrong basis", "missing positive", "duplicate Zone", "missing month", "foreign month", "scope amount", "scope factor", "scope raw zero", "scope null raw number", "missing scope", "duplicate scope", "plenum scope", "zero primary scope", "HW scope", "Hourly scope", "native CW doubled", "native source missing", "nil proof"} {
		t.Run(mutation, func(t *testing.T) {
			bundle, consumer := epathSQLPoolJointFixture(t)
			zone := &bundle.EnergyExplanation.ZoneResults[0]
			source := epathSQLPoolJointCWSource(t, &bundle, consumer)
			switch mutation {
			case "monthly joint":
				zone.Periods[1].Nodes[0].Value += .001
				zone.Periods[1].Nodes[0].AllocatedValue += .001
			case "annual Zone":
				zone.Nodes[0].Value += .001
				zone.Nodes[0].AllocatedValue += .001
			case "annual Building alias":
				bundle.EnergyExplanation.Periods[0].Reconciliation[0].AllocatedValue += .001
			case "annual Zone alias":
				zone.Periods[0].Nodes[0].Value += .001
				zone.Periods[0].Nodes[0].AllocatedValue += .001
			case "node value":
				zone.Nodes[0].Value += 1
			case "node allocated null":
				zone.Nodes[0].inspectorValuePresence = 0
			case "wrong basis":
				zone.Nodes[0].Basis = "direct_zone_energy"
			case "missing positive":
				zone.Periods[1].Nodes = nil
			case "duplicate Zone":
				bundle.EnergyExplanation.ZoneResults = append(bundle.EnergyExplanation.ZoneResults, *zone)
			case "missing month":
				zone.Periods = zone.Periods[:12]
			case "foreign month":
				zone.Periods[1].ID = "M13"
			case "scope amount":
				source.ScopeDetails[0].AllocatedValue += .001
			case "scope factor":
				source.ScopeDetails[0].AllocationFactor += .0001
			case "scope raw zero":
				source.ScopeDetails[0].inspectorDecodedFromJSON = true
				source.ScopeDetails[0].inspectorValuePresence = 1
			case "scope null raw number":
				source.ScopeDetails[0].RawValue = 1
			case "missing scope":
				source.ScopeDetails = source.ScopeDetails[1:]
			case "duplicate scope":
				source.ScopeDetails = append(source.ScopeDetails, source.ScopeDetails[0])
			case "plenum scope":
				source.ScopeDetails[0].Scope.ZoneName = consumer.Native.Original.Declaration.ReturnPlenumZoneName
			case "zero primary scope":
				source.ScopeDetails[0].Scope.ZoneName = consumer.Native.Original.ServedZones[3]
			case "HW scope", "Hourly scope":
				id := consumer.Native.Families["pump.hw.electricity"].CanonicalID
				if mutation == "Hourly scope" {
					id = consumer.Native.Families["pump.cw.electricity"].CompanionIDs[1]
				}
				for i := range bundle.EnergyExplanation.Sources {
					if bundle.EnergyExplanation.Sources[i].ID == fmt.Sprintf("sql-rdd-%d", id) {
						bundle.EnergyExplanation.Sources[i].ScopeDetails = []EnergyDataSourceScopeDetail{source.ScopeDetails[0]}
					}
				}
			case "native CW doubled":
				source.RawValue *= 2
				source.EffectiveValue *= 2
			case "native source missing":
				bundle.EnergyExplanation.Sources = bundle.EnergyExplanation.Sources[1:]
			case "nil proof":
				consumer = nil
			}
			if err := epathSQLCheckPoolPumpJointBudget(bundle, consumer); err == nil {
				t.Fatal("joint/source-scope mutation accepted")
			}
		})
	}
}

// Rebuild native hand rows and summaries, not production output. These setters
// retain the exact native calendar, units, source ownership and E/R semantics.
func epathSQLPoolJointSetObservation(source epathRealSQLSource, rows []epathSQLPoolNativeRow, energy [12]float64) (epathRealSQLSource, []epathSQLPoolNativeRow) {
	out := source
	out.RawSum, out.EnergyKWh = epathOracleNumber(0), epathOracleNumber(0)
	out.Months = nil
	copyRows := append([]epathSQLPoolNativeRow(nil), rows...)
	months := make([]epathRealSQLMonth, 12)
	for m := range months {
		months[m] = epathRealSQLMonth{Month: m + 1, RawSum: epathOracleNumber(0), EnergyKWh: epathOracleNumber(0)}
	}
	for i := range copyRows {
		row := &copyRows[i]
		minutes := float64(time.Date(2017, time.Month(row.Month+1), 0, 0, 0, 0, 0, time.UTC).Day() * 24 * 60)
		row.EnergyKWh = energy[row.Month-1] * row.IntervalMinutes / minutes
		row.NativeValue = row.EnergyKWh * 3600000
		if source.SourceUnit == "W" {
			row.NativeValue = energy[row.Month-1] * 60000 / minutes
		}
		bucket := &months[row.Month-1]
		bucket.Rows++
		*bucket.RawSum += row.NativeValue
		*bucket.EnergyKWh += row.EnergyKWh
	}
	for _, bucket := range months {
		*out.RawSum += *bucket.RawSum
		*out.EnergyKWh += *bucket.EnergyKWh
		out.Months = append(out.Months, bucket)
	}
	return out, copyRows
}

func epathSQLPoolJointNativePumpBudgets(t *testing.T, frames *epathSQLFrames, cwEnergy, hwEnergy [12]float64) {
	t.Helper()
	native := &frames.PoolSystems[0]
	for familyID, energy := range map[string][12]float64{"pump.cw.electricity": cwEnergy, "pump.hw.electricity": hwEnergy} {
		family := native.Families[familyID]
		for _, id := range append([]int{family.CanonicalID}, family.CompanionIDs...) {
			identity := native.Sources[id]
			identity.Source, identity.Rows = epathSQLPoolJointSetObservation(identity.Source, identity.Rows, energy)
			for m, bucket := range identity.Source.Months {
				identity.NativeRawMonthly[m] = *bucket.RawSum
				identity.NativeEnergyMonthly[m] = *bucket.EnergyKWh
			}
			native.Sources[id] = identity
			if id == family.CanonicalID {
				var err error
				family.Monthly, err = epathSQLMonthly(identity.Source, native.Precision)
				if err != nil {
					t.Fatal(err)
				}
				family.Annual, err = epathSQLPoolSourceQuantity(identity, native.Weather)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		native.Families[familyID] = family
	}
	energy := [12]float64{}
	for m := range energy {
		energy[m] = cwEnergy[m] + hwEnergy[m]
	}
	parent := native.Parents["Pumps:Electricity"]
	parent.Source, parent.Rows = epathSQLPoolJointSetObservation(parent.Source, parent.Rows, energy)
	for m, bucket := range parent.Source.Months {
		parent.NativeMonthly[m] = *bucket.EnergyKWh
	}
	var err error
	parent.Monthly, err = epathSQLMonthly(parent.Source, native.Precision)
	if err != nil {
		t.Fatal(err)
	}
	native.Parents["Pumps:Electricity"] = parent
	frames.SourceIdentities[parent.Source.DictionaryIndex] = parent.Source
	frames.Site["pumps.electricity"] = nil
	for _, q := range parent.Monthly {
		copy := q
		frames.Site["pumps.electricity"] = append(frames.Site["pumps.electricity"], &copy)
	}
}

func TestEnergyPathRealSQLPoolJointKnownZeroCWRequiresRecipientDetails(t *testing.T) {
	frames, model, aux := epathSQLPoolPumpConsumerHandFrames(t)
	hw := [12]float64{}
	for m := range hw {
		hw[m] = 2 * float64(time.Date(2017, time.Month(m+2), 0, 0, 0, 0, 0, time.UTC).Day()*24)
	}
	epathSQLPoolJointNativePumpBudgets(t, &frames, [12]float64{}, hw)
	consumer, err := epathSQLPoolPumpConsumerFor(frames, model, aux)
	if err != nil {
		t.Fatal(err)
	}
	bundle := epathSQLPoolJointHandBundle(t, consumer, map[string][12]float64{})
	if err := epathSQLCheckPoolPumpJointBudget(bundle, consumer); err != nil {
		t.Fatal(err)
	}
	zone := &bundle.EnergyExplanation.ZoneResults[0]
	zone.Nodes = []EnergyExplanationNode{{ID: "end_use.pumps.service_path_allocation", Level: "end_use", Kind: "end_use.pumps", EndUse: "pumps", AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", AggregationBasis: "model_total", Basis: "service_path_allocation", Period: "annual", ZoneName: zone.Scope.ZoneName, inspectorDecodedFromJSON: true, inspectorValuePresence: 4}}
	if err := epathSQLCheckPoolPumpJointBudget(bundle, consumer); err != nil {
		t.Fatalf("explicit present-zero pump rejected: %v", err)
	}
	zone.Nodes[0].inspectorValuePresence = 0
	if err := epathSQLCheckPoolPumpJointBudget(bundle, consumer); err == nil {
		t.Fatal("present-zero allocatedValue null was accepted")
	}
	zone.Nodes = nil
	source := epathSQLPoolJointCWSource(t, &bundle, consumer)
	if len(source.ScopeDetails) != 2 || source.ScopeDetails[0].AllocatedValue != 0 || !source.ScopeDetails[0].AllocationApplied {
		t.Fatal("hand known-zero recipient proof absent")
	}
	source.ScopeDetails = nil
	if err := epathSQLCheckPoolPumpJointBudget(bundle, consumer); err == nil {
		t.Fatal("deleted allocated-zero source scopes accepted")
	}
}

func TestEnergyPathRealSQLPoolJointRejectsThreeCopiesOfSingleQuantum(t *testing.T) {
	frames, model, aux := epathSQLPoolPumpConsumerHandFrames(t)
	cw := [12]float64{}
	for m := range cw {
		cw[m] = .001
	}
	epathSQLPoolJointNativePumpBudgets(t, &frames, cw, [12]float64{})
	for index, zone := range aux.ServedZones {
		power := 0.0
		if index == 0 {
			power = .34
		}
		if index == 1 || index == 2 {
			power = .33
		}
		id := frames.LoadSourceIDs[epathSQLKey(zone, "cooling", 1)][0]
		source, _ := epathSQLPoolSourceHandObservation(id, "Zone Air System Sensible Cooling Energy", zone, "J", "Monthly", false, power)
		frames.SourceIdentities[id] = source
		monthly, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		for month, q := range monthly {
			frames.Loads[epathSQLKey(zone, "cooling", month+1)] = q.times(3)
		}
	}
	consumer, err := epathSQLPoolPumpConsumerFor(frames, model, aux)
	if err != nil {
		t.Fatal(err)
	}
	shares := map[string][12]float64{}
	shares[strings.ToLower(aux.ServedZones[0])] = cw
	bundle := epathSQLPoolJointHandBundle(t, consumer, shares)
	if err := epathSQLCheckPoolPumpJointBudget(bundle, consumer); err != nil {
		t.Fatal(err)
	}
	calculation, err := epathSQLPoolPumpMathFromSources(consumer.Native, consumer.Frames, consumer.Model)
	if err != nil {
		t.Fatal(err)
	}
	// Each forged scalar still lies inside its own independently required
	// Hamilton envelope. Keep the Building budget at .001, not .003.
	for index := 1; index < 3; index++ {
		zone := &bundle.EnergyExplanation.ZoneResults[index]
		zone.Periods[1].Nodes = []EnergyExplanationNode{{ID: "end_use.pumps.service_path_allocation", Level: "end_use", Kind: "end_use.pumps", EndUse: "pumps", Value: .001, AllocatedValue: .001, AllocationApplied: true, Unit: "kWh", ScaleDomain: "site", AggregationBasis: "model_total", Basis: "service_path_allocation", Period: "M1", ZoneName: zone.Scope.ZoneName, inspectorDecodedFromJSON: true, inspectorValuePresence: 4}}
	}
	for index := 0; index < 3; index++ {
		q := calculation.Months[0].Shares[strings.ToLower(aux.ServedZones[index])]
		value := .001
		if err := epathCheckSQLModelQuantity(&value, &q); err != nil {
			t.Fatalf("test must isolate joint, not individual rejection: %v", err)
		}
	}
	err = epathSQLCheckPoolPumpJointBudget(bundle, consumer)
	if err == nil || !strings.Contains(err.Error(), "Zone pump sum 3 differs from Building allocated budget 1") {
		t.Fatalf("three copies of one native quantum escaped joint guard: %v", err)
	}
}

func TestEnergyPathRealSQLPoolJointRejectsInvalidDisplayQuantum(t *testing.T) {
	for _, value := range []float64{-1, math.NaN(), math.Inf(1), .0001, 1e-12, 9007199254741} {
		if _, err := epathSQLPoolPumpJointMilli(value); err == nil {
			t.Fatalf("invalid display quantum accepted: %.17g", value)
		}
	}
	for _, value := range []float64{0, .001, .003, 12345.678} {
		if _, err := epathSQLPoolPumpJointMilli(value); err != nil {
			t.Fatal(err)
		}
	}
}
