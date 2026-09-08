package simulation

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Literal hand arithmetic, with no production system/allocator/builder calls.
// One Zone has multiplier ten; its independently observed native load is one
// tenth of the effective weight. All consumption is already model-total.
func epathSQLVRFAllocationUnitFrames(t *testing.T) (epathSQLFrames, epathSQLVRFSystemFrame) {
	t.Helper()
	precision := epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}
	frame := epathSQLVRFSystemFrame{Precision: precision, Declaration: epathRealSQLVRFSystem{
		OutdoorUnit:      epathRealSQLVRFObject{ObjectType: "AirConditioner:VariableRefrigerantFlow", ObjectName: "Outdoor"},
		TerminalUnitList: epathRealSQLVRFObject{ObjectType: "ZoneTerminalUnitList", ObjectName: "Terminals"},
		CoolingSiteID:    "cooling.electricity", HeatingSiteID: "heating.electricity", Frequency: "Monthly", AggregationBasis: "model_total", AllocationPolicy: "per_constituent_millikwh_largest_remainder_v1",
	}}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{}, Loads: map[string]epathSQLQuantity{}, LoadSourceIDs: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}}
	keys := []string{}
	for index := 1; index <= 5; index++ {
		zone, terminal := fmt.Sprintf("SPACE%d-1", index), fmt.Sprintf("TU%d", index)
		keys = append(keys, terminal)
		frame.Declaration.Terminals = append(frame.Declaration.Terminals, epathRealSQLVRFTerminal{TerminalUnit: epathRealSQLVRFObject{ObjectType: "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow", ObjectName: terminal}, ZoneName: zone})
		factor := 1.0
		if index == 1 {
			factor = 10
		}
		frames.Zones[strings.ToLower(zone)] = epathSQLZone{Name: zone, Multiplier: factor}
		for _, service := range []string{"cooling", "heating"} {
			id, name := 300+index*2, "Zone Air System Sensible Cooling Energy"
			if service == "heating" {
				id, name = id+1, "Zone Air System Sensible Heating Energy"
			}
			var values [12]float64
			for month := 1; month <= 12; month++ {
				weight := index
				if month%2 == 0 {
					weight = 6 - index
				}
				if service == "heating" {
					weight = 6 - weight
				}
				values[month-1] = float64(10*weight*month) / factor
			}
			source := epathSQLVRFAllocationUnitSource(id, name, zone, values)
			frames.SourceIdentities[id] = source
			for month, value := range values {
				key := epathSQLKey(zone, service, month+1)
				frames.Loads[key] = epathSQLVRFAllocationUnitQuantity(value).times(factor)
				frames.LoadSourceIDs[key] = []int{id}
			}
		}
	}
	selector := func(name string, shared bool) epathRealSQLSelector {
		selected := append([]string(nil), keys...)
		if shared {
			selected = []string{"Outdoor"}
		}
		return epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}, Keys: selected}
	}
	frame.Declaration.Sources = epathRealSQLVRFSelectors{
		LocalCooling: selector("Zone VRF Air Terminal Cooling Electricity Energy", false), LocalHeating: selector("Zone VRF Air Terminal Heating Electricity Energy", false),
		SharedCooling: selector("VRF Heat Pump Cooling Electricity Energy", true), SharedCrankcase: selector("VRF Heat Pump Crankcase Heater Electricity Energy", true),
		SharedHeating: selector("VRF Heat Pump Heating Electricity Energy", true), SharedDefrost: selector("VRF Heat Pump Defrost Electricity Energy", true),
	}
	for _, item := range []struct {
		role, service, name string
		shared              bool
		coefficients        []float64
	}{
		{"local_cooling", "cooling", "Zone VRF Air Terminal Cooling Electricity Energy", false, []float64{0, 2, 3, 4, 5}},
		{"local_heating", "heating", "Zone VRF Air Terminal Heating Electricity Energy", false, []float64{1, 0, 3, 4, 5}},
		{"shared_cooling", "cooling", "VRF Heat Pump Cooling Electricity Energy", true, []float64{100}},
		{"shared_crankcase", "cooling", "VRF Heat Pump Crankcase Heater Electricity Energy", true, []float64{10}},
		{"shared_heating", "heating", "VRF Heat Pump Heating Electricity Energy", true, []float64{200}},
		{"shared_defrost", "heating", "VRF Heat Pump Defrost Electricity Energy", true, []float64{20}},
	} {
		for index, coefficient := range item.coefficients {
			zone, key := fmt.Sprintf("SPACE%d-1", index+1), fmt.Sprintf("TU%d", index+1)
			if item.shared {
				zone, key = "", "Outdoor"
			}
			var values [12]float64
			for month := 1; month <= 12; month++ {
				values[month-1] = coefficient * float64(month)
			}
			source := epathSQLVRFAllocationUnitSource(100+len(frame.Sources), item.name, key, values)
			itemFrame := epathSQLVRFSourceFrame{Role: item.role, Service: item.service, ZoneName: zone, Shared: item.shared, Source: source, EquipmentObjectIndex: index + 1}
			for month, value := range values {
				itemFrame.Months[month] = epathSQLVRFAllocationUnitQuantity(value)
			}
			frame.Sources = append(frame.Sources, itemFrame)
		}
	}
	return frames, frame
}

func epathSQLVRFAllocationUnitQuantity(value float64) epathSQLQuantity {
	if value == 0 {
		return epathSQLQuantity{}
	}
	return epathSQLQuantity{Value: value, Error: .001}
}

func epathSQLVRFAllocationUnitSource(id int, name, key string, values [12]float64) epathRealSQLSource {
	source := epathRealSQLSource{DictionaryIndex: id, Name: name, KeyValue: key, ReportingFrequency: "Monthly", SourceUnit: "J", Rows: 12}
	raw, energy := 0.0, 0.0
	for month, value := range values {
		joules := value * 3600000
		source.Months = append(source.Months, epathRealSQLMonth{Month: month + 1, Rows: 1, RawSum: epathOracleNumber(joules), EnergyKWh: epathOracleNumber(value)})
		raw += joules
		energy += value
	}
	source.RawSum, source.EnergyKWh = epathOracleNumber(raw), epathOracleNumber(energy)
	return source
}

func epathSQLVRFAllocationUnitReplaceSource(frame *epathSQLVRFSystemFrame, role, zone string, values [12]float64) {
	for index := range frame.Sources {
		source := &frame.Sources[index]
		if source.Role != role || source.ZoneName != zone {
			continue
		}
		source.Source = epathSQLVRFAllocationUnitSource(source.Source.DictionaryIndex, source.Source.Name, source.Source.KeyValue, values)
		for month, value := range values {
			source.Months[month] = epathSQLVRFAllocationUnitQuantity(value)
		}
	}
}

func epathSQLVRFAllocationUnitReplaceLoad(frames *epathSQLFrames, zone, service string, effective [12]float64) {
	id := frames.LoadSourceIDs[epathSQLKey(zone, service, 1)][0]
	factor := frames.Zones[strings.ToLower(zone)].Multiplier
	var native [12]float64
	for month, value := range effective {
		native[month] = value / factor
		frames.Loads[epathSQLKey(zone, service, month+1)] = epathSQLVRFAllocationUnitQuantity(native[month]).times(factor)
	}
	source := frames.SourceIdentities[id]
	frames.SourceIdentities[id] = epathSQLVRFAllocationUnitSource(id, source.Name, source.KeyValue, native)
}

func TestEnergyPathRealSQLVRFAllocationMonthlyAndAnnual(t *testing.T) {
	frames, system := epathSQLVRFAllocationUnitFrames(t)
	beforeFrames, beforeSystem := epathSQLVRFAllocationUnitFrames(t)
	proof, err := epathSQLCompileVRFAllocation(frames, system)
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.Months) != 12 || len(proof.Annual.Sources) != 14 || len(proof.Zones) != 5 {
		t.Fatalf("incomplete proof: %#v", proof)
	}
	for index, zone := range proof.Zones {
		for service, totals := range map[string][]int64{"cooling": {1804000, 1916000, 1950000, 1984000, 2018000}, "heating": {3334000, 3344000, 3666000, 3832000, 3998000}} {
			row := proof.Annual.Zones[zone][service]
			if got := row.DirectMilliKWh + row.AllocatedMilliKWh; got != totals[index] {
				t.Fatalf("%s/%s annual=%d want %d", zone, service, got, totals[index])
			}
			if len(row.PairMonths) != 12 || len(row.LoadSourceIDs) != 1 {
				t.Fatalf("lost known monthly load/trace: %#v", row)
			}
			monthly := int64(0)
			for month := 1; month <= 12; month++ {
				m := proof.Months[month].Zones[zone][service]
				monthly += m.DirectMilliKWh + m.AllocatedMilliKWh
			}
			if monthly != totals[index] {
				t.Fatalf("annual did not sum month-first allocations")
			}
		}
	}
	knownZero := proof.Months[1].Zones["SPACE1-1"]["cooling"]
	if knownZero.DirectMilliKWh != 0 || knownZero.AllocatedMilliKWh != 7334 || len(knownZero.SourceShares) != 3 || knownZero.SourceShares[100].Native != 0 {
		t.Fatalf("zero local input lost its distinct proof: %#v", knownZero)
	}
	for month, state := range proof.Months {
		for _, source := range state.Sources {
			total := source.UnassignedMilliKWh
			for _, share := range source.Shares {
				total += share.DisplayMilliKWh
			}
			if total != source.BudgetMilliKWh {
				t.Fatalf("M%d source %d does not close exactly", month, source.SourceID)
			}
		}
	}
	if !reflect.DeepEqual(frames, beforeFrames) || !reflect.DeepEqual(system, beforeSystem) {
		t.Fatal("allocation mutated its independent original frames")
	}
}

func TestEnergyPathRealSQLVRFAllocationPolicyBAndSourceRounding(t *testing.T) {
	shares, unassigned, err := epathSQLVRFMilliShares(2, map[string]float64{"A": 8, "B": 2, "C": 0, "D": 0, "E": 0}, []string{"E", "D", "C", "B", "A"})
	if err != nil || unassigned != 0 || shares["A"] != 2 || shares["B"] != 0 {
		t.Fatalf("budget-first .0016 kWh must give [2,0], not native-quota [1,1]: %v %v", shares, err)
	}
	shares, _, err = epathSQLVRFMilliShares(2, map[string]float64{"a": 1, "B": 1, "C": 1}, []string{"a", "C", "B"})
	if err != nil || shares["B"] != 1 || shares["C"] != 1 || shares["a"] != 0 {
		t.Fatalf("canonical lexical tie break changed: %v %v", shares, err)
	}
	frames, system := epathSQLVRFAllocationUnitFrames(t)
	var first, second [12]float64
	for month := range first {
		first[month], second[month] = .0006, .0004
	}
	epathSQLVRFAllocationUnitReplaceSource(&system, "shared_cooling", "", first)
	epathSQLVRFAllocationUnitReplaceSource(&system, "shared_crankcase", "", second)
	proof, err := epathSQLCompileVRFAllocation(frames, system)
	if err != nil {
		t.Fatal(err)
	}
	main, auxiliary := proof.Annual.Sources[110], proof.Annual.Sources[111]
	if main.BudgetMilliKWh != 12 || auxiliary.BudgetMilliKWh != 0 || math.Abs(main.Native.Value-.0006*12) > 1e-17 || math.Abs(auxiliary.Native.Value-.0004*12) > 1e-17 {
		t.Fatalf("source/month rounding collapsed or native observation replaced: %#v %#v", main, auxiliary)
	}
	// Two independently rounded-down sources cannot donate their fractional
	// native amounts to each other, even though their combined raw sum rounds up.
	epathSQLVRFAllocationUnitReplaceSource(&system, "shared_cooling", "", second)
	proof, err = epathSQLCompileVRFAllocation(frames, system)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Months[1].Sources[110].BudgetMilliKWh+proof.Months[1].Sources[111].BudgetMilliKWh != 0 {
		t.Fatal("main and auxiliary were merged before rounding")
	}
	// Source order and original TU-list order are irrelevant to ownership/ties.
	sort.Slice(system.Sources, func(i, j int) bool {
		return system.Sources[i].Source.DictionaryIndex > system.Sources[j].Source.DictionaryIndex
	})
	sort.Slice(system.Declaration.Terminals, func(i, j int) bool {
		return system.Declaration.Terminals[i].ZoneName > system.Declaration.Terminals[j].ZoneName
	})
	reordered, err := epathSQLCompileVRFAllocation(frames, system)
	if err != nil || !reflect.DeepEqual(proof.Months, reordered.Months) || !reflect.DeepEqual(proof.Annual, reordered.Annual) {
		t.Fatalf("source/list order changed allocation: %v", err)
	}
}

func TestEnergyPathRealSQLVRFAllocationZeroSiteRetainsPhysicalLoad(t *testing.T) {
	frames, system := epathSQLVRFAllocationUnitFrames(t)
	for _, source := range append([]epathSQLVRFSourceFrame(nil), system.Sources...) {
		if source.Service != "heating" {
			continue
		}
		var values [12]float64
		for month, q := range source.Months {
			values[month] = q.Value
		}
		values[6] = 0
		epathSQLVRFAllocationUnitReplaceSource(&system, source.Role, source.ZoneName, values)
	}
	proof, err := epathSQLCompileVRFAllocation(frames, system)
	if err != nil {
		t.Fatal(err)
	}
	for _, zone := range proof.Zones {
		july, annual := proof.Months[7].Zones[zone]["heating"], proof.Annual.Zones[zone]["heating"]
		if july.Load.Value <= 0 || july.DirectMilliKWh != 0 || july.AllocatedMilliKWh != 0 || len(july.PairMonths) != 0 || july.PairedLoad.Value != 0 {
			t.Fatalf("known-zero site became unknown or lost physical load: %#v", july)
		}
		if len(annual.PairMonths) != 11 || annual.PairedLoad.Value != annual.Load.Value-july.Load.Value {
			t.Fatalf("annual pair failed month-first omission: %#v", annual)
		}
	}
	// A fully observed zero denominator leaves the separate outdoor pools
	// unassigned; it does not erase local input or invent equal ownership.
	for _, zone := range proof.Zones {
		epathSQLVRFAllocationUnitReplaceLoad(&frames, zone, "cooling", [12]float64{})
	}
	proof, err = epathSQLCompileVRFAllocation(frames, system)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{110, 111} {
		row := proof.Months[1].Sources[id]
		if row.UnassignedMilliKWh != row.BudgetMilliKWh || row.UnassignedNative != row.Native.Value {
			t.Fatal("zero denominator hid an original positive pool")
		}
	}
	if proof.Months[1].Zones["SPACE2-1"]["cooling"].DirectMilliKWh != 2000 {
		t.Fatal("zero thermal load erased separately measured local input")
	}
}

func TestEnergyPathRealSQLVRFAllocationUnroundedWeightAndDisplayedLoad(t *testing.T) {
	frames, system := epathSQLVRFAllocationUnitFrames(t)
	for index := 1; index <= 5; index++ {
		var load [12]float64
		for month := range load {
			if index == 1 {
				load[month] = .0006 // native .00006 * ten: source and effective displays zero
			} else if index == 2 {
				load[month] = .0004
			}
		}
		epathSQLVRFAllocationUnitReplaceLoad(&frames, fmt.Sprintf("SPACE%d-1", index), "cooling", load)
	}
	var main [12]float64
	for month := range main {
		main[month] = .001
	}
	epathSQLVRFAllocationUnitReplaceSource(&system, "shared_cooling", "", main)
	epathSQLVRFAllocationUnitReplaceSource(&system, "shared_crankcase", "", [12]float64{})
	proof, err := epathSQLCompileVRFAllocation(frames, system)
	if err != nil {
		t.Fatal(err)
	}
	row := proof.Months[1].Zones["SPACE1-1"]["cooling"]
	if row.Load.Value != .0006 || row.LoadDisplayMilliKWh != 0 || row.AllocatedMilliKWh != 1 || math.Abs(row.SourceShares[110].Native-.0006) > 1e-18 {
		t.Fatalf("displayed load changed unrounded denominator or was promoted: %#v", row)
	}
	if row.PairedLoad.Value != 0 || row.PairedLoadDisplayMilliKWh != 0 || len(row.PairMonths) != 0 {
		t.Fatal("positive native but displayed-zero load acquired a thermal pair")
	}
	annual := proof.Annual.Zones["SPACE1-1"]["cooling"]
	if annual.Load.Value <= 0 || annual.PairedLoad.Value != 0 || annual.LoadDisplayMilliKWh != 0 || annual.AllocatedMilliKWh != 12 {
		t.Fatal("annual native load or source-month display allocations were lost")
	}
	if proof.Months[1].Zones["SPACE1-1"]["heating"].DirectMilliKWh != 1000 {
		t.Fatal("model-total VRF consumption was multiplied again")
	}
}

func TestEnergyPathRealSQLVRFAllocationRejectsUnboundFrames(t *testing.T) {
	for name, mutate := range map[string]func(*epathSQLFrames, *epathSQLVRFSystemFrame){
		"missing_source":   func(_ *epathSQLFrames, s *epathSQLVRFSystemFrame) { s.Sources = s.Sources[1:] },
		"duplicate_source": func(_ *epathSQLFrames, s *epathSQLVRFSystemFrame) { s.Sources = append(s.Sources, s.Sources[0]) },
		"wrong_service":    func(_ *epathSQLFrames, s *epathSQLVRFSystemFrame) { s.Sources[11].Service = "heating" },
		"wrong_owner":      func(_ *epathSQLFrames, s *epathSQLVRFSystemFrame) { s.Sources[0].ZoneName = "PLENUM" },
		"wrong_frequency": func(_ *epathSQLFrames, s *epathSQLVRFSystemFrame) {
			s.Sources[0].Source.ReportingFrequency = "Timestep"
		},
		"wrong_unit": func(_ *epathSQLFrames, s *epathSQLVRFSystemFrame) { s.Sources[0].Source.SourceUnit = "W" },
		"missing_month": func(_ *epathSQLFrames, s *epathSQLVRFSystemFrame) {
			s.Sources[0].Source.Months = s.Sources[0].Source.Months[:11]
		},
		"null_month":            func(_ *epathSQLFrames, s *epathSQLVRFSystemFrame) { s.Sources[0].Source.Months[0].EnergyKWh = nil },
		"negative_source":       func(_ *epathSQLFrames, s *epathSQLVRFSystemFrame) { s.Sources[0].Months[0].Value = -1 },
		"forged_display_center": func(_ *epathSQLFrames, s *epathSQLVRFSystemFrame) { s.Sources[10].Months[0].Value += .0001 },
		"missing_zero_load": func(f *epathSQLFrames, _ *epathSQLVRFSystemFrame) {
			delete(f.Loads, epathSQLKey("SPACE1-1", "cooling", 1))
		},
		"missing_load_trace": func(f *epathSQLFrames, _ *epathSQLVRFSystemFrame) {
			delete(f.LoadSourceIDs, epathSQLKey("SPACE1-1", "cooling", 1))
		},
		"duplicate_load_trace": func(f *epathSQLFrames, _ *epathSQLVRFSystemFrame) {
			key := epathSQLKey("SPACE1-1", "cooling", 1)
			f.LoadSourceIDs[key] = append(f.LoadSourceIDs[key], f.LoadSourceIDs[key][0])
		},
		"wrong_load_zone": func(f *epathSQLFrames, _ *epathSQLVRFSystemFrame) {
			source := f.SourceIdentities[302]
			source.KeyValue = "SPACE2-1"
			f.SourceIdentities[302] = source
		},
		"wrong_load_service": func(f *epathSQLFrames, _ *epathSQLVRFSystemFrame) {
			source := f.SourceIdentities[302]
			source.Name = "Zone Air System Sensible Heating Energy"
			f.SourceIdentities[302] = source
		},
		"load_multiplier_twice": func(f *epathSQLFrames, _ *epathSQLVRFSystemFrame) {
			key := epathSQLKey("SPACE1-1", "cooling", 1)
			f.Loads[key] = f.Loads[key].times(10)
		},
		"load_candidate_center": func(f *epathSQLFrames, _ *epathSQLVRFSystemFrame) {
			key := epathSQLKey("SPACE1-1", "cooling", 1)
			q := f.Loads[key]
			q.Value += .0001
			f.Loads[key] = q
		},
	} {
		t.Run(name, func(t *testing.T) {
			frames, system := epathSQLVRFAllocationUnitFrames(t)
			mutate(&frames, &system)
			if _, err := epathSQLCompileVRFAllocation(frames, system); err == nil {
				t.Fatal("unbound source/load proof accepted")
			}
		})
	}
	for _, value := range []float64{-1, math.Inf(1), math.NaN(), float64(epathSQLVRFMaxExactMilli)} {
		if _, err := epathSQLVRFMilliBudget(value); err == nil {
			t.Fatalf("invalid budget accepted: %v", value)
		}
	}
	for name, weights := range map[string]map[string]float64{"missing": {"A": 1}, "negative": {"A": 1, "B": -1}, "nan": {"A": 1, "B": math.NaN()}} {
		if _, _, err := epathSQLVRFMilliShares(1, weights, []string{"A", "B"}); err == nil {
			t.Fatalf("invalid weights accepted: %s", name)
		}
	}
}
