package simulation

import (
	"encoding/json"
	"testing"
)

func TestEPATH181ProjectionRejectsAmbiguousCanonicalIdentity(t *testing.T) {
	for _, variant := range []string{"duplicate node", "duplicate link", "empty node", "empty link", "missing endpoint", "duplicate Zone", "case-folded Zone", "root and nested Zone"} {
		t.Run(variant, func(t *testing.T) {
			bundle := epath181IdentityBundle()
			selection := EnergyPathSelection{}
			switch variant {
			case "duplicate node":
				bundle.EnergyExplanation.Nodes = append(bundle.EnergyExplanation.Nodes, bundle.EnergyExplanation.Nodes[0])
			case "duplicate link":
				bundle.EnergyExplanation.Links = append(bundle.EnergyExplanation.Links, bundle.EnergyExplanation.Links[0])
			case "empty node":
				bundle.EnergyExplanation.Nodes[0].ID = ""
			case "empty link":
				bundle.EnergyExplanation.Links[0].ID = ""
			case "missing endpoint":
				bundle.EnergyExplanation.Links[0].ToID = "absent"
			default:
				zone := epath181IdentityZone(bundle, "Office")
				selection = EnergyPathSelection{Scope: "zone", Zone: "Office"}
				bundle.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{zone, zone}
				if variant == "case-folded Zone" {
					bundle.EnergyExplanation.ZoneResults[1].Scope.ZoneName = "OFFICE"
				}
				if variant == "root and nested Zone" {
					bundle.EnergyExplanation.Scope = zone.Scope
					bundle.EnergyExplanation.Nodes = zone.Nodes
					bundle.EnergyExplanation.Links = zone.Links
					bundle.EnergyExplanationSummary = zone.Summary
					bundle.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{zone}
				}
			}
			before, _ := json.Marshal(bundle)
			if _, err := ProjectEnergyPath(bundle, selection); err == nil {
				t.Fatalf("%s was silently resolved to one canonical identity", variant)
			}
			after, _ := json.Marshal(bundle)
			if string(before) != string(after) {
				t.Fatal("failed selection mutated its input bundle")
			}
		})
	}
}

func TestEPATH181ProjectionDoesNotReuseContradictorySummaryContext(t *testing.T) {
	for _, variant := range []string{"empty month annual summary", "month different month summary", "empty month stale same-context summary", "Zone Building summary", "Zone different Zone summary"} {
		t.Run(variant, func(t *testing.T) {
			bundle := epath181IdentityBundle()
			selection := EnergyPathSelection{Period: "M3"}
			stale := bundle.EnergyExplanationSummary
			stale.Loads = []EnergyExplanationSummaryItem{{ID: "stale.annual", Level: "load", Value: 900, Unit: "kWh"}}
			bundle.EnergyExplanation.Periods = []EnergyPeriod{{ID: "M3", Kind: "monthly", Nodes: []EnergyExplanationNode{}, Links: []EnergyPathLink{}, Summary: &stale}}
			switch variant {
			case "month different month summary":
				stale.Period = "M2"
			case "empty month stale same-context summary":
				stale.Period = "M3"
			case "Zone Building summary", "Zone different Zone summary":
				selection = EnergyPathSelection{Scope: "zone", Zone: "Office"}
				zone := epath181IdentityZone(bundle, "Office")
				zone.Summary = stale
				if variant == "Zone different Zone summary" {
					zone.Summary.Scope = EnergyExplanationScope{Kind: "zone", ZoneName: "Lab"}
				}
				bundle.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{zone}
			}
			before, _ := json.Marshal(bundle)
			got, err := ProjectEnergyPath(bundle, selection)
			if err != nil {
				t.Fatalf("valid selected graph should rebuild its stale summary: %v", err)
			}
			if got.View.Summary.Period != got.Selection.Period || got.View.Summary.Scope != got.View.Scope {
				t.Fatalf("%s retained the unselected summary context: %+v", variant, got.View.Summary)
			}
			if selection.Scope == "zone" {
				if len(got.View.Summary.Loads) != 1 || got.View.Summary.Loads[0].Value != 40 || got.View.Summary.Loads[0].ID != "load.cool" || got.View.Summary.Scope.ZoneName != "Office" {
					t.Fatalf("%s substituted unselected 900 for actual Office40: %+v", variant, got.View.Summary)
				}
			} else if len(got.View.Nodes) != 0 || len(got.View.Summary.Loads)+len(got.View.Summary.EndUses)+len(got.View.Summary.Carriers) != 0 {
				t.Fatalf("%s reused stale values for explicit empty M3: %+v", variant, got.View.Summary)
			}
			after, _ := json.Marshal(bundle)
			if string(before) != string(after) {
				t.Fatal("context validation mutated canonical summary")
			}
		})
	}
}

func TestEPATH181ProjectionRejectsExplicitScopeAndServiceContradictions(t *testing.T) {
	for _, variant := range []string{"Zone node mismatch", "Zone link mismatch", "link service mismatch", "load service mismatch", "end-use service mismatch"} {
		t.Run(variant, func(t *testing.T) {
			bundle := epath181IdentityBundle()
			selection := EnergyPathSelection{Service: "cooling"}
			switch variant {
			case "Zone node mismatch", "Zone link mismatch":
				selection = EnergyPathSelection{Scope: "zone", Zone: "Office"}
				zone := epath181IdentityZone(bundle, "Office")
				if variant == "Zone node mismatch" {
					zone.Nodes[0].ZoneName = "Lab"
				} else {
					zone.Links[0].ZoneName = "Lab"
				}
				bundle.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{zone}
			case "link service mismatch":
				bundle.EnergyExplanation.Links[0].ServiceKind = "heating"
				selection.Service = "heating"
			case "load service mismatch":
				bundle.EnergyExplanation.Nodes[0].ServiceKind = "heating"
			case "end-use service mismatch":
				bundle.EnergyExplanation.Nodes[1].ServiceKind = "heating"
			}
			if got, err := ProjectEnergyPath(bundle, selection); err == nil {
				t.Fatalf("%s exposed a contradictory selected graph: %+v", variant, got.View)
			}
		})
	}
}

func TestEPATH181ProjectionKeepsSameCarrierStorageContextWithoutAddingConsumption(t *testing.T) {
	bundle := epath181IdentityBundle()
	graph := &bundle.EnergyExplanation
	graph.Nodes[2].Value = 13
	graph.Nodes = append(graph.Nodes,
		EnergyExplanationNode{ID: "use.other", Level: "end_use", Kind: "end_use.other", EndUse: "other", ScaleDomain: "site", Period: "annual", Unit: "kWh", Value: 3, SourceIDs: []string{"charge"}},
		EnergyExplanationNode{ID: "support.charge", Level: "support", Kind: "support.storage_charge", EndUse: "storage_charge", Carrier: "electricity", Period: "annual", Unit: "kWh", Value: 3, SourceIDs: []string{"charge"}},
		EnergyExplanationNode{ID: "support.other-carrier", Level: "support", Kind: "support.purchased", EndUse: "purchased", Carrier: "natural_gas", Period: "annual", Unit: "kWh", Value: 9, SourceIDs: []string{"other-supply"}},
	)
	graph.Links = append(graph.Links, EnergyPathLink{ID: "charge.consumption", FromID: "use.other", ToID: "carrier.electricity", Relation: "direct_end_use_to_carrier", Period: "annual", FromValue: 3, ToValue: 3, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"charge"}})
	graph.Sources = append(graph.Sources, EnergyDataSource{ID: "charge", Name: "Reported storage charge", NormalizedUnit: "kWh"}, EnergyDataSource{ID: "other-supply", Name: "Other carrier supply", NormalizedUnit: "kWh"})
	bundle.EnergyExplanationSummary = buildEnergyExplanationSummaryV2(*graph)
	before, _ := json.Marshal(bundle)
	projected, err := ProjectEnergyPath(bundle, EnergyPathSelection{Service: "cooling"})
	if err != nil {
		t.Fatal(err)
	}
	var charge *EnergyExplanationNode
	for index := range projected.View.Nodes {
		node := &projected.View.Nodes[index]
		if node.ID == "support.other-carrier" || node.ID == "use.other" {
			t.Fatalf("Cooling selection pulled an unrelated carrier or direct-use branch into the view: %+v", node)
		}
		if node.ID == "support.charge" {
			charge = node
		}
	}
	if charge == nil || charge.Level != "support" || charge.Value != 3 || charge.EndUse != "storage_charge" || len(charge.SourceIDs) != 1 || charge.SourceIDs[0] != "charge" {
		t.Fatalf("same-carrier isolated charge lost its exact context: %+v", charge)
	}
	if len(projected.View.Links) != 2 || projected.View.Links[0].ID != "conversion" || projected.View.Links[1].ID != "consumption" {
		t.Fatalf("support retention created/reassigned a physical or supply link: %+v", projected.View.Links)
	}
	if len(projected.View.Summary.EndUses) != 1 || projected.View.Summary.EndUses[0].ID != "use.cool" || projected.View.Summary.EndUses[0].Value != 10 {
		t.Fatalf("support charge inflated selected consumption: %+v", projected.View.Summary.EndUses)
	}
	if len(projected.View.Summary.Carriers) != 1 || projected.View.Summary.Carriers[0].Value != 13 {
		t.Fatalf("service context changed the original shared carrier total: %+v", projected.View.Summary.Carriers)
	}
	after, _ := json.Marshal(bundle)
	canonical, _ := json.Marshal(projected.PurposeResults)
	if string(before) != string(after) || string(before) != string(canonical) {
		t.Fatal("support selection moved or duplicated original charge consumption")
	}
}

func epath181IdentityBundle() PurposeResultBundle {
	sources := []EnergyDataSource{{ID: "thermal", Name: "Reported cooling", NormalizedUnit: "kWh"}, {ID: "site", Name: "Cooling:Electricity", NormalizedUnit: "kWh"}}
	nodes := []EnergyExplanationNode{
		{ID: "load.cool", Level: "load", Kind: "load.cooling", ServiceKind: "cooling", ScaleDomain: "thermal", Period: "annual", Unit: "kWh", Value: 40, SourceIDs: []string{"thermal"}},
		{ID: "use.cool", Level: "end_use", Kind: "end_use.cooling", EndUse: "cooling", ServiceKind: "cooling", ScaleDomain: "site", Period: "annual", Unit: "kWh", Value: 10, SourceIDs: []string{"site"}},
		{ID: "carrier.electricity", Level: "carrier", Kind: "carrier.electricity", Carrier: "electricity", ScaleDomain: "site", Period: "annual", Unit: "kWh", Value: 10, SourceIDs: []string{"site"}},
	}
	links := []EnergyPathLink{
		{ID: "conversion", FromID: "load.cool", ToID: "use.cool", Relation: "load_to_end_use", ServiceKind: "cooling", Period: "annual", FromValue: 40, ToValue: 10, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"thermal", "site"}},
		{ID: "consumption", FromID: "use.cool", ToID: "carrier.electricity", Relation: "end_use_to_carrier", ServiceKind: "cooling", Period: "annual", FromValue: 10, ToValue: 10, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"site"}},
	}
	graph := EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}, Nodes: nodes, Links: links, Sources: sources}
	return PurposeResultBundle{EnergyExplanation: graph, EnergyExplanationSummary: buildEnergyExplanationSummaryV2(graph)}
}

func epath181IdentityZone(bundle PurposeResultBundle, name string) EnergyExplanationZoneResult {
	graph := bundle.EnergyExplanation
	graph.Scope = EnergyExplanationScope{Kind: "zone", ZoneName: name, AggregationBasis: "model_total"}
	graph.Nodes = cloneEnergyExplanationNodes(graph.Nodes)
	graph.Links = append([]EnergyPathLink(nil), graph.Links...)
	for index := range graph.Nodes {
		graph.Nodes[index].ZoneName = name
	}
	for index := range graph.Links {
		graph.Links[index].ZoneName = name
	}
	return EnergyExplanationZoneResult{Scope: graph.Scope, Nodes: graph.Nodes, Links: graph.Links, Summary: buildEnergyExplanationSummaryV2(graph)}
}
