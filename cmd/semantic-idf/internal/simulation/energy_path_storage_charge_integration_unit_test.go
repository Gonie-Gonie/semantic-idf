package simulation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestEnergyPathStorageChargeOriginalIndexCannotGainNativeProof(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		doc := storageBoundaryDraftDocument(t)
		storage := storageBoundaryDraftObject(&doc, "ElectricLoadCenter:Storage:Battery")
		storage.Index = -1
		if duplicate {
			storage.Index = storageBoundaryDraftObject(&doc, "ElectricLoadCenter:Distribution").Index
		}
		item := storageBoundaryDraftSeries("Battery", map[int]float64{1: 10})
		qualified, boundary := qualifyEnergyPathStorageChargeSeries(item, energyPathBuildStorageChargeInventory(doc), storageBoundaryDraftSources("Battery"))
		if boundary == nil || boundary.State != energyPathStorageChargeUnresolved || qualified.Total != 10 || qualified.Stage != "support" {
			t.Fatalf("invalid original index gained native proof: %+v", boundary)
		}
		qualified.storageChargeBoundary = boundary
		if len(energyPathStorageChargeWarnings([]energyExplanationSeries{qualified})) == 0 {
			t.Fatal("invalid original reference lost its diagnostic")
		}
	}
}

func TestEnergyPathStorageChargeAvailabilityIsKeyAndFrequencyLocal(t *testing.T) {
	inventory := energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t))
	second := inventory.ByKey["battery"]
	second.SourceKey = "AC Battery"
	inventory.ByKey["ac battery"] = second
	plan := &PurposeRunPlan{}
	for _, frequency := range []string{"Monthly", "Hourly"} {
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", VariableName: "Electric Storage Charge Energy", KeyValue: "*", ReportingFrequency: frequency, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	}
	sources := storageBoundaryDraftSources("Battery")
	sources[0].observedValuePresence = energySourceObservedRaw // An actual measured zero.
	unknown := sources[0]
	unknown.ID, unknown.KeyValue, unknown.observedValuePresence = "sql-rdd-52", "AC Battery", 0
	sources = append(sources, unknown)
	entries := energyPathStorageChargeAvailability(plan, inventory, sources)
	if len(entries) != 4 {
		t.Fatalf("key/frequency census collapsed: %+v", entries)
	}
	found := 0
	for _, entry := range entries {
		if entry.Level != "context" {
			t.Fatal("charge request entered energy denominator")
		}
		if entry.Status == "found" {
			found++
			if entry.Name != "Electric Storage Charge Energy [Battery; Monthly]" || !reflect.DeepEqual(entry.SourceIDs, []string{"sql-rdd-51"}) {
				t.Fatalf("zero substituted for another key/frequency: %+v", entry)
			}
		}
	}
	if found != 1 {
		t.Fatalf("measured-zero/unknown count = %d", found)
	}
	duplicate := sources[0]
	duplicate.ID = "sql-rdd-53"
	for _, entry := range energyPathStorageChargeAvailability(plan, inventory, append(sources, duplicate)) {
		if entry.Status == "found" {
			t.Fatal("duplicate native reporting dictionary gained availability")
		}
	}
	if got := energyPathStorageChargeAvailability(plan, energyPathStorageChargeInventory{}, sources); got != nil {
		t.Fatal("no-original compatibility acquired new availability")
	}
}

func TestEnergyPathStorageChargePrunedMetadataDoesNotChangeBudget(t *testing.T) {
	boundary := energyPathStorageChargeBoundaryForKey(energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t)), "Battery")
	positive := EnergyExplanationNode{ID: "charge", Level: "support", Value: 10, SourceIDs: []string{"sql-rdd-51"}, storageChargeBoundaries: []energyPathStorageChargeBoundary{*boundary}}
	zero := cloneEnergyPathStorageChargeBoundary(boundary)
	zero.SourceKey, zero.SourceIDs = "AC Battery", []string{"sql-rdd-52"}
	series := []energyExplanationSeries{{Total: 0, storageChargeBoundary: zero}}
	nodes := carryEnergyPathStorageChargePrunedMetadata([]EnergyExplanationNode{positive}, series, func(item energyExplanationSeries) float64 { return item.Total })
	if len(nodes) != 1 || nodes[0].Value != 10 || len(nodes[0].storageChargeBoundaries) != 2 || !reflect.DeepEqual(nodes[0].SourceIDs, []string{"sql-rdd-51"}) {
		t.Fatalf("metadata altered numerical contributors: %+v", nodes)
	}
	if len(carryEnergyPathStorageChargePrunedMetadata(nil, series, func(item energyExplanationSeries) float64 { return item.Total })) != 0 {
		t.Fatal("metadata invented a zero-valued observation node")
	}
	kept := []EnergyExplanationNode{{ID: "charge", Level: "support", Value: 5}}
	merged := mergeEnergyPathStorageChargeNodeMetadata(kept, nodes)
	if merged[0].Value != 5 || len(merged[0].storageChargeBoundaries) != 2 || len(merged[0].SourceIDs) != 0 || len(kept[0].storageChargeBoundaries) != 0 {
		t.Fatal("annual fallback metadata changed monthly amount or caller")
	}
}

func TestEnergyPathStorageChargeStoredDenialDoesNotMutateCaller(t *testing.T) {
	boundary := energyPathStorageChargeBoundaryForKey(energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t)), "Battery")
	// Unknown future metadata is still a denial, without guessed source IDs.
	boundary.State, boundary.SourceIDs = "unreviewed_future_state", nil
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"},
		Nodes: []EnergyExplanationNode{
			{ID: "carrier.electricity.building", Level: "carrier", Kind: "energy.electricity.total", Carrier: "electricity", Value: 100, Unit: "kWh", Period: "annual"},
			{ID: "support.storage_charge.building", Level: "support", Kind: "energy.storage_charge", EndUse: "storage_charge", Carrier: "electricity", Value: 10, Unit: "kWh", Period: "annual", storageChargeBoundaries: []energyPathStorageChargeBoundary{*boundary}},
		},
		Links: []EnergyPathLink{{ID: "stale", FromID: "support.storage_charge.building", ToID: "carrier.electricity.building", Relation: "support_supply", FromValue: 10, ToValue: 10, FromUnit: "kWh", ToUnit: "kWh"}},
	}
	// Exact snapshot: the general canonical clone normalizes nil string slices
	// to empty ones, which is not evidence that the writer mutated its caller.
	beforeNodes, beforeLinks := append([]EnergyExplanationNode(nil), result.Nodes...), append([]EnergyPathLink(nil), result.Links...)
	beforeNodes[1].storageChargeBoundaries = []energyPathStorageChargeBoundary{*cloneEnergyPathStorageChargeBoundary(boundary)}
	if !reflect.DeepEqual(result.Nodes, beforeNodes) {
		t.Fatal("snapshot differs before serialization")
	}
	for reopen := 0; reopen < 2; reopen++ {
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if reopen == 0 && (!reflect.DeepEqual(result.Nodes, beforeNodes) || !reflect.DeepEqual(result.Links, beforeLinks)) {
			t.Fatal("writer changed caller-owned graph")
		}
		if !strings.Contains(string(raw), "storageChargeBoundaries") {
			t.Fatal("writer dropped explicit denial")
		}
		var decoded EnergyExplanationResult
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		for _, link := range decoded.Links {
			if link.FromID == "support.storage_charge.building" || link.ToID == "support.storage_charge.building" {
				t.Fatal("stored explicit boundary recovered a flow")
			}
		}
		warning := false
		for _, item := range decoded.Warnings {
			warning = warning || item.Code == "storage_charge_boundary_unresolved"
		}
		if !warning {
			t.Fatal("invalid present boundary lost diagnostic")
		}
		result = decoded
	}
}

func TestEnergyPathStorageChargePreferredCompanionCannotBorrowNativeProof(t *testing.T) {
	inventory := energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t))
	monthly := storageBoundaryDraftSeries("Battery", map[int]float64{1: 10})
	hourly := storageBoundaryDraftSeries("Battery", nil)
	hourly.Total, hourly.Hourly, hourly.sourceFrequency = 12, map[int]float64{1: 12}, "Hourly"
	hourly.SourceIDs, hourly.AnnualSourceIDs, hourly.HourlySourceIDs = []string{"sql-rdd-52"}, []string{"sql-rdd-52"}, []string{"sql-rdd-52"}
	sources := storageBoundaryDraftSources("Battery")
	badCompanion := sources[0]
	badCompanion.ID, badCompanion.ReportingFrequency, badCompanion.SourceUnit, badCompanion.Units = "sql-rdd-52", "Hourly", "kWh", "kWh"
	sources = append(sources, badCompanion)
	items := []energyExplanationSeries{monthly, hourly}
	for index := range items {
		items[index], items[index].storageChargeBoundary = qualifyEnergyPathStorageChargeSeries(items[index], inventory, sources)
		items[index].storageChargeSelectionSuffix = energyPathStorageChargeSelectionSuffix(items[index], inventory)
		items[index] = canonicalEnergyExplanationSeries(items[index])
	}
	if items[0].storageChargeBoundary.State != energyPathStorageChargeNative || items[1].storageChargeBoundary.State != energyPathStorageChargeUnresolved {
		t.Fatal("test did not establish different source qualifications")
	}
	selected := preferredEnergyExplanationSeries(items)
	if len(selected) != 1 || selected[0].storageChargeBoundary.State != energyPathStorageChargeUnresolved || selected[0].Monthly[1] != 10 || selected[0].Hourly[1] != 12 || len(selected[0].storageChargeBoundary.SourceIDs) != 2 {
		t.Fatalf("companion borrowed native proof or changed quantities: %+v", selected)
	}
	if items[0].storageChargeBoundary.State != energyPathStorageChargeNative || len(items[0].storageChargeBoundary.SourceIDs) != 1 {
		t.Fatal("preference changed original source proof")
	}
}

func TestEnergyPathStorageChargeAnnualOnlyContextSurvivesMonthlyFallback(t *testing.T) {
	boundary := energyPathStorageChargeBoundaryForKey(energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t)), "Battery")
	boundary.SourceIDs = []string{"sql-rdd-51"}
	carrier := EnergyExplanationNode{ID: "carrier.electricity.building", Level: "carrier", Carrier: "electricity", Value: 100, RawValue: 100, EffectiveValue: 100, AllocatedValue: 100, Unit: "kWh", SourceIDs: []string{"sql-rdd-1"}}
	charge := EnergyExplanationNode{ID: "support.storage_charge.building", Level: "support", EndUse: "storage_charge", Carrier: "electricity", Value: 13, RawValue: 13, EffectiveValue: 13, AllocatedValue: 13, Unit: "kWh", SourceIDs: []string{"sql-rdd-51"}, storageChargeBoundaries: []energyPathStorageChargeBoundary{*boundary}}
	result := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}, Nodes: []EnergyExplanationNode{carrier, charge}, Periods: []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: []EnergyExplanationNode{carrier}}}}
	applyCanonicalMonthlyBasisToEnergyPathResult(&result)
	count := 0
	for _, node := range result.Nodes {
		if node.ID == charge.ID {
			count++
			if node.Value != 13 || len(node.storageChargeBoundaries) != 1 || !reflect.DeepEqual(node.SourceIDs, charge.SourceIDs) {
				t.Fatal("annual-only context changed")
			}
		}
	}
	if count != 1 {
		t.Fatal("annual-only native charge disappeared behind unrelated Monthly outputs")
	}
	for _, period := range result.Periods {
		if period.Kind == "monthly" {
			for _, node := range period.Nodes {
				if len(node.storageChargeBoundaries) > 0 {
					t.Fatal("annual-only charge fabricated a monthly observation")
				}
			}
		}
	}
}
