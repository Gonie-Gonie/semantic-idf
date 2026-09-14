package simulation

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

func epathSQLPoolPumpConsumerHandFrames(t *testing.T) (epathSQLFrames, epathRealSQLModel, epathRealSQLAuxiliary) {
	t.Helper()
	original, err := epathSQLValidatePoolOriginal(epathSQLPoolOriginalFixture(t), []epathRealSQLPoolSystem{epathSQLPoolOriginalDeclaration()})
	if err != nil || len(original) != 1 {
		t.Fatalf("original Pool fixture: %v", err)
	}
	native := epathSQLPoolSourceHandFramesForOriginal(t, original[0])
	parent := native.Parents["Pumps:Electricity"]
	aux := epathRealSQLAuxiliary{SiteID: "pumps.electricity", ServedZones: append([]string(nil), original[0].ServedZones...), Weight: "cooling", AllocationMethod: "service_load_share", ReconciliationID: "reconcile.allocation.pumps.electricity.annual"}
	model := epathRealSQLModel{OriginalZoneMultiplierProof: epathSQLOriginalMultiplierContract, Precision: native.Precision, PoolSystems: []epathRealSQLPoolSystem{original[0].Declaration}, Auxiliaries: []epathRealSQLAuxiliary{aux}, Site: []epathRealSQLSite{{ID: aux.SiteID, EndUse: "pumps", Carrier: "electricity", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Pumps:Electricity", Unit: "J"}}, Keys: []string{""}, IsMeter: true}}}}
	frames := epathSQLFrames{PoolSystems: []epathSQLPoolSourceFrames{native}, Zones: map[string]epathSQLZone{}, Loads: map[string]epathSQLQuantity{}, LoadSourceIDs: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{parent.Source.DictionaryIndex: parent.Source}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{aux.SiteID: {parent.Source.DictionaryIndex}}}
	for _, q := range parent.Monthly {
		copy := q
		frames.Site[aux.SiteID] = append(frames.Site[aux.SiteID], &copy)
	}
	names := append(append([]string(nil), original[0].ServedZones...), original[0].Declaration.ReturnPlenumZoneName)
	for n, name := range names {
		zone := strings.ToLower(name)
		frames.Zones[zone] = epathSQLZone{Name: name, Multiplier: 3}
		power := 0.0
		if n == 0 {
			power = 1
		}
		if n == 1 {
			power = 2
		}
		if n == 5 {
			power = 10
		}
		source, _ := epathSQLPoolSourceHandObservation(400+n, "Zone Air System Sensible Cooling Energy", name, "J", "Monthly", false, power)
		frames.SourceIdentities[source.DictionaryIndex] = source
		values, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		for month, q := range values {
			key := epathSQLKey(zone, "cooling", month+1)
			frames.Loads[key], frames.LoadSourceIDs[key] = q.times(3), []int{source.DictionaryIndex}
		}
	}
	return frames, model, aux
}

func TestEnergyPathRealSQLPoolAuxiliaryNativeCWNotBroadMeter(t *testing.T) {
	frames, model, aux := epathSQLPoolPumpConsumerHandFrames(t)
	consumer, err := epathSQLPoolPumpConsumerFor(frames, model, aux)
	if err != nil || consumer == nil {
		t.Fatalf("native Pool consumer: %v", err)
	}
	proofs, err := epathSQLPoolPumpAuxiliaryProofs(consumer)
	if err != nil {
		t.Fatal(err)
	}
	if len(proofs) != 6*13 {
		t.Fatal("original recipient/plenum or monthly/annual census changed")
	}
	first := proofs[epathSQLAuxiliaryZoneKey(aux.SiteID, aux.ServedZones[0], "annual")]
	if err := epathSQLValidatePoolPumpAuxiliaryProof(first); err != nil {
		t.Fatal(err)
	}
	cwID := fmt.Sprintf("sql-rdd-%d", consumer.Native.Families["pump.cw.electricity"].CanonicalID)
	parentID := fmt.Sprintf("sql-rdd-%d", consumer.Native.Parents["Pumps:Electricity"].Source.DictionaryIndex)
	if len(first.Sources) != 1 || first.Sources[parentID].RDD == nil || !first.Required[parentID] || len(first.NodeSources) != 3 || first.NodeSources[cwID].RDD == nil || !first.NodeRequired[cwID] {
		t.Fatal("CW recipient lost the distinct broad-meter provenance, native CW budget or primary load proof")
	}
	want := 5.0 * 8760 / 3
	if err := epathCheckSQLModelQuantity(&want, &first.Value); err != nil {
		t.Fatal(err)
	}
	plenum := proofs[epathSQLAuxiliaryZoneKey(aux.SiteID, consumer.Native.Original.Declaration.ReturnPlenumZoneName, "annual")]
	if plenum.Owned || plenum.Value.Value != 0 {
		t.Fatal("positive return-plenum load became a CW denominator member")
	}
	checks := epathSQLModelChecks{}
	if err := epathSQLPoolPumpBuildingLedgerChecks(consumer, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 13*4 {
		t.Fatal("missing native broad/HW/CW ledger fields")
	}
	for _, check := range checks.Rows {
		if check.Item.Period != "annual" {
			continue
		}
		values := map[string]float64{"expectedValue": 7 * 8760, "directValue": 0, "allocatedValue": 5 * 8760, "unassignedValue": 2 * 8760}
		want := values[check.Item.Target.Field]
		if check.Allocation.NativePoolPump == nil {
			t.Fatal("ledger lacks source-local native proof")
		}
		if err := epathCheckSQLModelQuantity(&want, check.Quantity); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnergyPathRealSQLPoolAuxiliaryRejectsWrongScopeOrWeights(t *testing.T) {
	for _, mutation := range []string{"undeclared", "missing native", "broad replaced", "broad identity", "foreign site", "HW recipient", "whole load weight", "wrong method", "latent only", "double multiplier", "missing zero", "duplicate served", "foreign source"} {
		t.Run(mutation, func(t *testing.T) {
			frames, model, aux := epathSQLPoolPumpConsumerHandFrames(t)
			zone := strings.ToLower(aux.ServedZones[0])
			key := epathSQLKey(zone, "cooling", 1)
			switch mutation {
			case "undeclared":
				model.PoolSystems = nil
			case "missing native":
				frames.PoolSystems = nil
			case "broad replaced":
				copy := *frames.Site[aux.SiteID][0]
				copy.Value++
				frames.Site[aux.SiteID][0] = &copy
			case "broad identity":
				frames.SiteSources[aux.SiteID] = []int{999}
			case "foreign site":
				model.Site[0].Carrier = "natural_gas"
			case "HW recipient":
				aux.ServedZones[0] = frames.PoolSystems[0].Original.Declaration.ReturnPlenumZoneName
			case "whole load weight":
				aux.Weight = "cooling_plus_heating"
			case "wrong method":
				aux.AllocationMethod = "plant_loop_load_share"
			case "latent only":
				id := frames.LoadSourceIDs[key][0]
				source := frames.SourceIdentities[id]
				source.Name = "Zone Air System Latent Cooling Energy"
				frames.SourceIdentities[id] = source
			case "double multiplier":
				frames.Loads[key] = frames.Loads[key].times(3)
			case "missing zero":
				delete(frames.Loads, epathSQLKey(aux.ServedZones[3], "cooling", 1))
			case "duplicate served":
				aux.ServedZones[1] = aux.ServedZones[0]
			case "foreign source":
				id := frames.LoadSourceIDs[key][0]
				source := frames.SourceIdentities[id]
				source.KeyValue = "Foreign"
				frames.SourceIdentities[id] = source
			}
			if _, err := epathSQLPoolPumpConsumerFor(frames, model, aux); err == nil {
				t.Fatal("unproved native CW consumer accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLPoolAuxiliaryProjectionMutation(t *testing.T) {
	frames, model, aux := epathSQLPoolPumpConsumerHandFrames(t)
	consumer, err := epathSQLPoolPumpConsumerFor(frames, model, aux)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"quantity", "source", "required", "method", "unowned", "period", "missing proof"} {
		t.Run(mutation, func(t *testing.T) {
			p, err := epathSQLPoolPumpProjection(consumer, aux.ServedZones[0], "M1")
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "quantity":
				p.Value.Value++
			case "source":
				p.Sources = map[string]epathSQLOriginalSource{}
			case "required":
				p.NodeRequired = map[string]bool{}
			case "method":
				p.AllocationMethod = "plant_loop_load_share"
			case "unowned":
				p.Owned = false
			case "period":
				p.Period = "M13"
			case "missing proof":
				p.NativePool = nil
			}
			if err := epathSQLValidatePoolPumpAuxiliaryProof(p); err == nil {
				t.Fatal("mutated native CW scalar proof accepted")
			}
		})
	}
	keys := []string{}
	for key := range consumer.Frames.Loads {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) != 72 {
		t.Fatal("positive-only projection discarded primary observed-zero roster")
	}
}

func TestEnergyPathRealSQLPoolAuxiliaryMethodUsesMonthlyIntegerBudgets(t *testing.T) {
	input := epathSQLPoolMathFixture()
	for month := 0; month < 12; month++ {
		input.Broad[month], input.HotWater[month], input.ChilledWater[month] = &epathSQLQuantity{Value: .0004, Error: .0005}, &epathSQLQuantity{}, &epathSQLQuantity{Value: .0004, Error: .0005}
	}
	calculation, err := epathSQLCompilePoolPumpMath(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, period := range epathSQLZoneCarrierPeriods() {
		method, err := epathSQLPoolPumpAllocationMethod(calculation, period)
		if err != nil || method != "unassigned" {
			t.Fatalf("%s fabricated allocation from subquantum months: %s/%v", period, method, err)
		}
	}
	input.Broad[5], input.ChilledWater[5] = &epathSQLQuantity{Value: .0006, Error: .0005}, &epathSQLQuantity{Value: .0006, Error: .0005}
	calculation, err = epathSQLCompilePoolPumpMath(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, period := range []string{"M6", "annual"} {
		method, err := epathSQLPoolPumpAllocationMethod(calculation, period)
		if err != nil || method != "service_load_share" {
			t.Fatal("positive monthly integer allocation was erased")
		}
	}
}
