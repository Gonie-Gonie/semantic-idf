package simulation

// Hash-bound native original plus independent structural mutations.
// No candidate, SQL scalar, expected file or AnalyzeHVAC result proves ownership.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func energyPathPoolAirTestApply(t *testing.T, doc idf.Document) (energyPathPoolInventory, []energyPathPoolAirRoute) {
	t.Helper()
	inventory := energyPathNativePoolInventory(doc)
	routes := applyEnergyPathPoolAirRoutes(doc, &inventory)
	return inventory, routes
}

func energyPathPoolAirTestRoute(t *testing.T, routes []energyPathPoolAirRoute, name string) energyPathPoolAirRoute {
	t.Helper()
	for _, route := range routes {
		if energyPathPoolEqual(route.Coil.ObjectName, name) {
			return route
		}
	}
	t.Fatalf("missing preserved demand route for %s", name)
	return energyPathPoolAirRoute{}
}

func TestEnergyPathPoolAirOriginalCWEligibleAndHWPoolBoundaryPreserved(t *testing.T) {
	doc := energyPathPoolOriginal(t)
	before := doc.String()
	waterBefore := energyPathNativePoolInventory(doc)
	got, routes := energyPathPoolAirTestApply(t, doc)
	if doc.String() != before {
		t.Fatal("native original changed")
	}
	if len(routes) != 9 {
		t.Fatalf("all seven heating and two cooling coil demands retained: %d", len(routes))
	}
	wantZones := []string{"SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1"}
	for _, loop := range got.Loops {
		if !loop.WaterTopologyComplete || !loop.SupplyRosterComplete || !loop.DemandRosterComplete {
			t.Fatalf("air binder weakened water evidence: %+v", loop)
		}
		if energyPathPoolEqual(loop.Component.ObjectName, "Chilled Water Loop") {
			if !loop.AirRoutesComplete || loop.HasNonZoneDemand || len(loop.Demands) != 2 {
				t.Fatalf("CW full native service path should be eligible: %+v", loop)
			}
		} else if energyPathPoolEqual(loop.Component.ObjectName, "Hot Water Loop") {
			if loop.AirRoutesComplete || !loop.HasNonZoneDemand || len(loop.Demands) != 8 {
				t.Fatalf("pool must keep HW total service incomplete: %+v", loop)
			}
		}
		for _, demand := range loop.Demands {
			if demand.Kind == "pool" {
				if demand.AirRouteComplete || len(demand.RelatedPathIDs) != 0 || demand.ZoneName != "SPACE1-1" || !demand.NonZoneDemand {
					t.Fatalf("pool became a fake ZoneHVAC path: %+v", demand)
				}
				continue
			}
			if !demand.AirRouteComplete || len(demand.RelatedPathIDs) != 0 {
				t.Fatalf("native route must precede optional path binding: %+v", demand)
			}
		}
	}
	for _, route := range routes {
		if !route.Complete || route.AirLoop.ObjectIndex != 206 || route.AirLoop.ObjectName != "VAV Sys 1" || route.PlantLoop.ObjectType != "PlantLoop" || len(route.Issues) != 0 || len(route.Trace) < 20 {
			t.Fatalf("full independent route identity/trace: %+v", route)
		}
		if route.Position == "terminal_reheat" {
			if len(route.ServedZoneNames) != 1 || len(route.Deliveries) != 1 || !energyPathPoolAirSameRef(route.Deliveries[0].ReheatCoil, route.Coil) {
				t.Fatalf("reheat coil recipient must remain local: %+v", route)
			}
		} else if !reflect.DeepEqual(route.ServedZoneNames, wantZones) || len(route.Deliveries) != 5 {
			t.Fatalf("OA/main coils must serve exact five-Zone union: %+v", route)
		}
		for _, zone := range route.ServedZoneNames {
			if zone == "PLENUM-1" {
				t.Fatal("positive native PLENUM load is not a supply recipient")
			}
		}
	}
	for _, name := range []string{"OA Cooling Coil 1", "Main Cooling Coil 1"} {
		route := energyPathPoolAirTestRoute(t, routes, name)
		if route.ServiceKind != "cooling" || route.PlantLoop.ObjectIndex != 268 {
			t.Fatalf("CW source-local ownership: %+v", route)
		}
	}
	// The binder's only inventory mutations are the named air-route fields.
	for loop := range got.Loops {
		got.Loops[loop].AirRoutesComplete = false
		for demand := range got.Loops[loop].Demands {
			got.Loops[loop].Demands[demand].AirRouteComplete = false
			got.Loops[loop].Demands[demand].RelatedPathIDs = nil
		}
	}
	if !reflect.DeepEqual(got, waterBefore) {
		t.Fatal("water/source/pool presence evidence mutated by air binder")
	}
}

func TestEnergyPathPoolAirOriginalBuildContextKeepsActualCWNavigationEligible(t *testing.T) {
	doc := energyPathPoolOriginal(t)
	before := doc.String()
	context := newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc)
	if !context.Enabled || !context.PoolInventory.HasNativePool || !context.PoolInventory.SchemaReviewed {
		t.Fatal("actual original build context did not retain native Pool evidence")
	}
	inventory := context.PoolInventory
	cw := energyPathPoolTestLoop(t, inventory, "Chilled Water Loop")
	if !cw.WaterTopologyComplete || !cw.SupplyRosterComplete || !cw.DemandRosterComplete || !cw.AirRoutesComplete || cw.HasNonZoneDemand || len(cw.Demands) != 2 {
		t.Fatalf("actual context lost complete independent CW topology: %+v", cw)
	}

	// This is the actual original navigation output, not the hand-framed
	// helper used in isolated binder tests. It may only identify already-proved
	// routes; every expected Zone/terminal below comes from the fixed original.
	hvac := idf.AnalyzeHVAC(doc)
	navigation := map[string][]idf.ZoneServicePath{}
	for _, summary := range hvac.ServiceModel.ZoneServices {
		for _, path := range summary.Paths {
			navigation[path.ID] = append(navigation[path.ID], path)
		}
	}
	wantTerminals := map[string]int{"SPACE1-1": 192, "SPACE2-1": 193, "SPACE3-1": 194, "SPACE4-1": 195, "SPACE5-1": 196}
	checkFiveActualPaths := func(label string, ids []string) {
		t.Helper()
		if len(ids) != 5 {
			t.Fatalf("%s: want five existing original service paths, got %v; CW demands=%+v; actual navigation=%+v", label, ids, cw.Demands, hvac.ServiceModel.ZoneServices)
		}
		seenIDs, seenZones := map[string]bool{}, map[string]bool{}
		for _, id := range ids {
			if id == "" || seenIDs[id] || len(navigation[id]) != 1 {
				t.Fatalf("%s: duplicate/fabricated/ambiguous actual navigation ID %q", label, id)
			}
			seenIDs[id] = true
			path := navigation[id][0]
			terminalIndex, exists := wantTerminals[path.ZoneName]
			if !exists || seenZones[path.ZoneName] || path.ServiceKind != "cooling" || path.AirLoop == nil || path.PlantLoop == nil ||
				path.AirLoop.ObjectIndex != 206 || path.AirLoop.Type != "AirLoopHVAC" || path.AirLoop.Name != "VAV Sys 1" ||
				path.PlantLoop.ObjectIndex != 268 || path.PlantLoop.Type != "PlantLoop" || path.PlantLoop.Name != "Chilled Water Loop" ||
				path.Delivery.ObjectType != "AirTerminal:SingleDuct:VAV:Reheat" || path.Delivery.ObjectIndex != terminalIndex {
				t.Fatalf("%s: source eligibility borrowed a foreign service/Zone/terminal/loop: %+v", label, path)
			}
			seenZones[path.ZoneName] = true
		}
		if len(seenZones) != len(wantTerminals) || seenZones["PLENUM-1"] {
			t.Fatalf("%s: exact original supplied Zone union changed: %v", label, seenZones)
		}
	}
	seenCoils := map[int]bool{}
	for _, demand := range cw.Demands {
		if demand.Kind != "cooling_coil" || !demand.AirRouteComplete || (demand.Water.Component.ObjectIndex != 216 && demand.Water.Component.ObjectIndex != 218) || seenCoils[demand.Water.Component.ObjectIndex] {
			t.Fatalf("actual context dropped/duplicated an OA/main native cooling demand: %+v", demand)
		}
		seenCoils[demand.Water.Component.ObjectIndex] = true
		checkFiveActualPaths(demand.Water.Component.ObjectName, demand.RelatedPathIDs)
	}
	cwPump := energyPathPoolTestSource(t, inventory, "CW Circ Pump")
	if cwPump.Component.ObjectIndex != 300 || cwPump.Loop.ObjectIndex != 268 {
		t.Fatalf("actual original CW pump ownership: %+v", cwPump)
	}
	checkFiveActualPaths("CW pump final eligible source paths", energyPathPoolEligibleSourcePaths(inventory, cwPump.Component, "cooling"))
	if paths := energyPathPoolEligibleSourcePaths(inventory, cwPump.Component, "heating"); len(paths) != 0 {
		t.Fatalf("CW pump gained Heating paths: %v", paths)
	}
	for _, name := range []string{"HW Circ Pump", "Central Boiler"} {
		source := energyPathPoolTestSource(t, inventory, name)
		if paths := energyPathPoolEligibleSourcePaths(inventory, source.Component, "heating"); len(paths) != 0 {
			t.Fatalf("%s: Pool-bearing HW must remain unassigned despite real air navigation: %v", name, paths)
		}
	}
	if doc.String() != before {
		t.Fatal("actual geometry/context/navigation analysis changed the hash-bound original")
	}
}

func TestEnergyPathPoolAirOriginalRejectsEveryBrokenNativeRoute(t *testing.T) {
	tests := []struct {
		name, kind, object string
		field              int
		value              string
	}{
		{"outside inlet not declared", "Coil:Heating:Water", "OA Heating Coil 1", 6, "Undeclared Outside"},
		{"OA heating outlet disconnected", "Coil:Heating:Water", "OA Heating Coil 1", 7, "Wrong OA Heating Out"},
		{"OA cooling inlet disconnected", "Coil:Cooling:Water", "OA Cooling Coil 1", 11, "Wrong OA Cooling In"},
		{"OA cooling outlet disconnected", "Coil:Cooling:Water", "OA Cooling Coil 1", 12, "Wrong OA Cooling Out"},
		{"mixer outside disconnected", "OutdoorAir:Mixer", "OA Mixing Box 1", 2, "Wrong Mixer Outside"},
		{"mixer return disconnected", "OutdoorAir:Mixer", "OA Mixing Box 1", 4, "Wrong Mixer Return"},
		{"mixer output disconnected", "OutdoorAir:Mixer", "OA Mixing Box 1", 1, "Wrong Mixer Output"},
		{"OA controller relief disconnected", "Controller:OutdoorAir", "OA Controller 1", 1, "Wrong Relief"},
		{"OA controller return disconnected", "Controller:OutdoorAir", "OA Controller 1", 2, "Wrong Return"},
		{"OA controller mixed disconnected", "Controller:OutdoorAir", "OA Controller 1", 3, "Wrong Mixed"},
		{"OA actuator skips pretreat", "Controller:OutdoorAir", "OA Controller 1", 4, "OA Mixing Box 1 Inlet Node"},
		{"OA cooling sensor disconnected", "Controller:WaterCoil", "OA CC Controller 1", 4, "Wrong Sensor"},
		{"OA heating actuator disconnected", "Controller:WaterCoil", "OA HC Controller 1", 5, "Wrong Actuator"},
		{"central cooling sensor disconnected", "Controller:WaterCoil", "Central Cooling Coil Controller 1", 4, "Wrong Sensor"},
		{"central heating actuator disconnected", "Controller:WaterCoil", "Central Heating Coil Controller 1", 5, "Wrong Actuator"},
		{"cooling controller wrong action", "Controller:WaterCoil", "OA CC Controller 1", 2, "Normal"},
		{"wrong controller type", "AirLoopHVAC:ControllerList", "OA Sys 1 Controllers", 3, "Controller:OutdoorAir"},
		{"OA list wrong order", "AirLoopHVAC:OutdoorAirSystem:EquipmentList", "OA Sys 1 Equipment", 1, "Coil:Cooling:Water"},
		{"main branch component wrong type", "Branch", "VAV Sys 1 Main Branch", 6, "Coil:Heating:Water"},
		{"main branch continuity broken", "Branch", "VAV Sys 1 Main Branch", 8, "Wrong Main In"},
		{"main cooling native inlet", "Coil:Cooling:Water", "Main Cooling Coil 1", 11, "Wrong Coil In"},
		{"main heating native outlet", "Coil:Heating:Water", "Main Heating Coil 1", 7, "Wrong Coil Out"},
		{"fan native outlet", "Fan:VariableVolume", "Supply Fan 1", 16, "Wrong Fan Out"},
		{"AirLoop supply inlet", "AirLoopHVAC", "VAV Sys 1", 6, "Wrong Supply In"},
		{"AirLoop demand outlet", "AirLoopHVAC", "VAV Sys 1", 7, "Wrong Demand Out"},
		{"AirLoop demand inlet", "AirLoopHVAC", "VAV Sys 1", 8, "Wrong Demand In"},
		{"AirLoop supply outlet", "AirLoopHVAC", "VAV Sys 1", 9, "Wrong Supply Out"},
		{"supply path inlet", "AirLoopHVAC:SupplyPath", "Zone Supply Air Path 1", 1, "Wrong SupplyPath In"},
		{"splitter inlet", "AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter 1", 1, "Wrong Splitter In"},
		{"splitter outlet absent terminal", "AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter 1", 2, "Wrong Terminal In"},
		{"duplicate splitter outlet", "AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter 1", 3, "SPACE1-1 ATU In Node"},
		{"terminal inlet disconnected", "AirTerminal:SingleDuct:VAV:Reheat", "SPACE1-1 VAV Reheat", 3, "Wrong Terminal In"},
		{"terminal damper outlet disconnected", "AirTerminal:SingleDuct:VAV:Reheat", "SPACE1-1 VAV Reheat", 2, "Wrong Damper Out"},
		{"terminal outlet disconnected", "AirTerminal:SingleDuct:VAV:Reheat", "SPACE1-1 VAV Reheat", 13, "Wrong Terminal Out"},
		{"ADU outlet disconnected", "ZoneHVAC:AirDistributionUnit", "SPACE1-1 ATU", 1, "Wrong ADU Out"},
		{"ADU terminal missing", "ZoneHVAC:AirDistributionUnit", "SPACE1-1 ATU", 3, "Missing Terminal"},
		{"Zone inlet disconnected", "ZoneHVAC:EquipmentConnections", "SPACE1-1", 2, "Wrong Zone In"},
		{"Zone return disconnected", "ZoneHVAC:EquipmentConnections", "SPACE1-1", 5, "Wrong Zone Return"},
		{"Zone unknown owner", "ZoneHVAC:EquipmentConnections", "SPACE1-1", 0, "Unknown Zone"},
		{"Zone repeated air node", "ZoneHVAC:EquipmentConnections", "SPACE1-1", 4, "SPACE1-1 In Node"},
		{"return path outlet", "AirLoopHVAC:ReturnPath", "ReturnAirPath1", 1, "Wrong ReturnPath Out"},
		{"plenum outlet", "AirLoopHVAC:ReturnPlenum", "Return-Plenum-1", 3, "Wrong Plenum Out"},
		{"plenum Zone missing", "AirLoopHVAC:ReturnPlenum", "Return-Plenum-1", 1, "Unknown Plenum Zone"},
		{"plenum duplicate return", "AirLoopHVAC:ReturnPlenum", "Return-Plenum-1", 6, "SPACE1-1 Out Node"},
		{"plenum missing return", "AirLoopHVAC:ReturnPlenum", "Return-Plenum-1", 5, "Wrong Return Node"},
		{"plenum induced outlet unreviewed", "AirLoopHVAC:ReturnPlenum", "Return-Plenum-1", 4, "Induced Outlet"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := energyPathPoolOriginal(t)
			object := energyPathPoolTestObject(t, &doc, tc.kind, tc.object)
			object.Fields[tc.field].Value = tc.value
			inventory, routes := energyPathPoolAirTestApply(t, doc)
			if energyPathPoolTestLoop(t, inventory, "Chilled Water Loop").AirRoutesComplete {
				t.Fatal("broken complete shared air path still made CW eligible")
			}
			for _, route := range routes {
				if route.Complete || len(route.RelatedPathIDs) != 0 {
					t.Fatalf("invalid native topology was partially authorized: %+v", route)
				}
			}
		})
	}
}

func TestEnergyPathPoolAirRejectsDuplicateAndAdditionalOwners(t *testing.T) {
	for _, tc := range []struct{ kind, name string }{
		{"AirLoopHVAC", "VAV Sys 1"}, {"BranchList", "VAV Sys 1 Branches"}, {"Branch", "VAV Sys 1 Main Branch"},
		{"AirLoopHVAC:OutdoorAirSystem", "OA Sys 1"}, {"AirLoopHVAC:OutdoorAirSystem:EquipmentList", "OA Sys 1 Equipment"},
		{"AirLoopHVAC:ControllerList", "OA Sys 1 Controllers"}, {"Controller:OutdoorAir", "OA Controller 1"}, {"Controller:WaterCoil", "OA CC Controller 1"},
		{"Coil:Cooling:Water", "OA Cooling Coil 1"}, {"Coil:Heating:Water", "Main Heating Coil 1"},
		{"AirLoopHVAC:SupplyPath", "Zone Supply Air Path 1"}, {"AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter 1"},
		{"AirTerminal:SingleDuct:VAV:Reheat", "SPACE1-1 VAV Reheat"}, {"ZoneHVAC:AirDistributionUnit", "SPACE1-1 ATU"},
		{"ZoneHVAC:EquipmentList", "SPACE1-1 Eq"}, {"ZoneHVAC:EquipmentConnections", "SPACE1-1"},
		{"AirLoopHVAC:ReturnPath", "ReturnAirPath1"}, {"AirLoopHVAC:ReturnPlenum", "Return-Plenum-1"},
	} {
		for _, rename := range []bool{false, true} {
			// An unreferenced, differently named coil is not a second typed
			// ownership claim. Duplicate identities are still tested below.
			if rename && strings.HasPrefix(tc.kind, "Coil:") {
				continue
			}
			t.Run(fmt.Sprintf("%s/%s/rename=%t", tc.kind, tc.name, rename), func(t *testing.T) {
				doc := energyPathPoolOriginal(t)
				copy := *energyPathPoolTestObject(t, &doc, tc.kind, tc.name)
				copy.Fields = append([]idf.Field(nil), copy.Fields...)
				if rename {
					copy.Fields[0].Value += " competing owner"
				}
				energyPathPoolTestAppend(&doc, copy)
				inventory, _ := energyPathPoolAirTestApply(t, doc)
				if energyPathPoolTestLoop(t, inventory, "Chilled Water Loop").AirRoutesComplete {
					t.Fatal("competing native ownership should fail closed")
				}
			})
		}
	}
}

func energyPathPoolAirTestAppendObject(doc *idf.Document, kind string, fields ...string) {
	object := idf.Object{Type: kind}
	for _, value := range fields {
		object.Fields = append(object.Fields, idf.Field{Value: value})
	}
	energyPathPoolTestAppend(doc, object)
}

func TestEnergyPathPoolAirNodeListNativeExpansionAndNoNameHeuristics(t *testing.T) {
	for _, mode := range []string{"single-selector", "outdoor-union", "outdoor-single-alias", "renamed-all", "blank-default-controller-action"} {
		t.Run(mode, func(t *testing.T) {
			doc := energyPathPoolOriginal(t)
			switch mode {
			case "single-selector":
				for _, target := range []struct {
					kind, name string
					field      int
				}{
					{"AirLoopHVAC", "VAV Sys 1", 8}, {"AirLoopHVAC", "VAV Sys 1", 9},
					{"ZoneHVAC:EquipmentConnections", "SPACE1-1", 2}, {"ZoneHVAC:EquipmentConnections", "SPACE1-1", 5},
				} {
					object := energyPathPoolTestObject(t, &doc, target.kind, target.name)
					node := object.Fields[target.field].Value
					alias := fmt.Sprintf("Reviewed one-node alias %d", len(doc.Objects))
					object.Fields[target.field].Value = alias
					energyPathPoolAirTestAppendObject(&doc, "NodeList", alias, node)
				}
			case "outdoor-union":
				energyPathPoolAirTestAppendObject(&doc, "OutdoorAir:NodeList", "Outside Air Inlet Node 1", "OutsideAirInletNodes")
			case "outdoor-single-alias":
				for at := range doc.Objects {
					if energyPathPoolEqual(doc.Objects[at].Type, "OutdoorAir:NodeList") {
						doc.Objects[at].Type = "OutdoorAir:Node"
					}
				}
			case "renamed-all":
				// Native names and matching reference values change consistently;
				// no literal fixture object/Zone spelling may authorize ownership.
				names := map[string]string{}
				for _, object := range doc.Objects {
					kind := energyPathPoolToken(object.Type)
					named := strings.HasPrefix(kind, "airloop") || strings.HasPrefix(kind, "zonehvac:") || strings.HasPrefix(kind, "airterminal:") || strings.HasPrefix(kind, "controller:") || strings.HasPrefix(kind, "coil:") || strings.HasPrefix(kind, "fan:") ||
						kind == "zone" || kind == "branch" || kind == "branchlist" || kind == "plantloop" || kind == "outdoorair:mixer" || kind == "nodelist"
					if named && len(object.Fields) > 0 && object.Fields[0].Value != "" {
						name := object.Fields[0].Value
						if _, exists := names[energyPathPoolToken(name)]; !exists {
							names[energyPathPoolToken(name)] = "Renamed " + name
						}
					}
				}
				for at := range doc.Objects {
					for field := range doc.Objects[at].Fields {
						if value, ok := names[energyPathPoolToken(doc.Objects[at].Fields[field].Value)]; ok {
							doc.Objects[at].Fields[field].Value = value
						}
					}
				}
			case "blank-default-controller-action":
				for at := range doc.Objects {
					if energyPathPoolEqual(doc.Objects[at].Type, "Controller:WaterCoil") {
						doc.Objects[at].Fields[2].Value = ""
					}
				}
			}
			inventory, routes := energyPathPoolAirTestApply(t, doc)
			completeCW := 0
			for _, loop := range inventory.Loops {
				if loop.AirRoutesComplete && len(loop.Demands) == 2 && !loop.HasNonZoneDemand {
					completeCW++
				}
			}
			if completeCW != 1 || len(routes) != 9 {
				t.Fatalf("valid native equivalent route rejected: CW=%d routes=%+v", completeCW, routes)
			}
		})
	}
	for _, mode := range []string{"nested", "duplicate-members", "outdoor-single-overlap", "duplicate-list", "multimember-single", "extra-terminal", "extra-OA-component", "extra-controller", "extra-return"} {
		t.Run(mode, func(t *testing.T) {
			doc := energyPathPoolOriginal(t)
			switch mode {
			case "nested":
				energyPathPoolTestObject(t, &doc, "NodeList", "OutsideAirInletNodes").Fields[1].Value = "Inner list"
				energyPathPoolAirTestAppendObject(&doc, "NodeList", "Inner list", "Outside Air Inlet Node 1")
			case "duplicate-members":
				list := energyPathPoolTestObject(t, &doc, "NodeList", "OutsideAirInletNodes")
				list.Fields = append(list.Fields, list.Fields[1])
			case "outdoor-single-overlap":
				energyPathPoolAirTestAppendObject(&doc, "OutdoorAir:Node", "Outside Air Inlet Node 1", "-1")
			case "duplicate-list":
				energyPathPoolAirTestAppendObject(&doc, "NodeList", "OutsideAirInletNodes", "Outside Air Inlet Node 1")
			case "multimember-single":
				energyPathPoolTestObject(t, &doc, "AirLoopHVAC", "VAV Sys 1").Fields[8].Value = "Many demand nodes"
				energyPathPoolAirTestAppendObject(&doc, "NodeList", "Many demand nodes", "Zone Eq In Node", "Another Inlet")
			case "extra-terminal":
				object := energyPathPoolTestObject(t, &doc, "AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter 1")
				object.Fields = append(object.Fields, idf.Field{Value: "Extra unowned terminal inlet"})
			case "extra-OA-component":
				object := energyPathPoolTestObject(t, &doc, "AirLoopHVAC:OutdoorAirSystem:EquipmentList", "OA Sys 1 Equipment")
				object.Fields = append(object.Fields, idf.Field{Value: "Coil:Heating:Water"}, idf.Field{Value: "Unexpected extra coil"})
			case "extra-controller":
				object := energyPathPoolTestObject(t, &doc, "AirLoopHVAC:ControllerList", "OA Sys 1 Controllers")
				object.Fields = append(object.Fields, idf.Field{Value: "Controller:WaterCoil"}, idf.Field{Value: "Unexpected extra controller"})
			case "extra-return":
				object := energyPathPoolTestObject(t, &doc, "AirLoopHVAC:ReturnPlenum", "Return-Plenum-1")
				object.Fields = append(object.Fields, idf.Field{Value: "Extra unowned return node"})
			}
			inventory, _ := energyPathPoolAirTestApply(t, doc)
			if energyPathPoolTestLoop(t, inventory, "Chilled Water Loop").AirRoutesComplete {
				t.Fatal("invalid native selector/roster was accepted")
			}
		})
	}
}

func TestEnergyPathPoolAirUnknownDemandAndOrphanEvidenceRemain(t *testing.T) {
	doc := energyPathPoolOriginal(t)
	inventory := energyPathNativePoolInventory(doc)
	for loop := range inventory.Loops {
		if inventory.Loops[loop].Component.ObjectName == "Chilled Water Loop" {
			inventory.Loops[loop].Demands = append(inventory.Loops[loop].Demands, energyPathPoolDemand{Kind: "unknown", Water: energyPathPoolWaterComponent{Component: idf.ComponentRef{ObjectType: "Unreviewed:Demand", ObjectName: "Unknown", ObjectIndex: -1}}})
		}
	}
	inventory.UnresolvedPoolIndices = []int{301}
	poolCount, sourceCount := len(inventory.Pools), len(inventory.Sources)
	applyEnergyPathPoolAirRoutes(doc, &inventory)
	if energyPathPoolTestLoop(t, inventory, "Chilled Water Loop").AirRoutesComplete {
		t.Fatal("unknown additional CW demand removed from completeness")
	}
	if !inventory.HasNativePool || !reflect.DeepEqual(inventory.UnresolvedPoolIndices, []int{301}) || len(inventory.Pools) != poolCount || len(inventory.Sources) != sourceCount {
		t.Fatal("global unresolved/orphan evidence was dropped")
	}
	for loop := range inventory.Loops {
		for demand := range inventory.Loops[loop].Demands {
			inventory.Loops[loop].Demands[demand].RelatedPathIDs = []string{"stale path"}
		}
	}
	inventory.SchemaReviewed = false
	if routes := applyEnergyPathPoolAirRoutes(doc, &inventory); len(routes) != 0 {
		t.Fatal("unreviewed schema produced native routes")
	}
	for _, loop := range inventory.Loops {
		for _, demand := range loop.Demands {
			if demand.AirRouteComplete || len(demand.RelatedPathIDs) != 0 {
				t.Fatal("unreviewed schema kept stale path authorization")
			}
		}
	}
}

func energyPathPoolAirTestSummaries(routes []energyPathPoolAirRoute) []idf.ZoneServiceSummary {
	var summaries []idf.ZoneServiceSummary
	for _, route := range routes {
		if !route.Complete {
			continue
		}
		for _, delivery := range route.Deliveries {
			zone := delivery.Zone.ObjectName
			path := idf.ZoneServicePath{ID: fmt.Sprintf("hand-path:%d:%d", route.Coil.ObjectIndex, delivery.Zone.ObjectIndex), ZoneName: zone, ServiceKind: route.ServiceKind, PathType: "central_air", AirLoop: &idf.LoopRef{Type: route.AirLoop.ObjectType, Name: route.AirLoop.ObjectName, ObjectIndex: route.AirLoop.ObjectIndex}, PlantLoop: &idf.LoopRef{Type: route.PlantLoop.ObjectType, Name: route.PlantLoop.ObjectName, ObjectIndex: route.PlantLoop.ObjectIndex}, Conditioning: []idf.ComponentRef{route.Coil}, Delivery: delivery.Terminal}
			summaries = append(summaries, idf.ZoneServiceSummary{ZoneName: zone, Paths: []idf.ZoneServicePath{path}})
		}
	}
	return summaries
}

func TestEnergyPathPoolAirNavigationCannotAuthorizeOriginalOwnership(t *testing.T) {
	doc := energyPathPoolOriginal(t)
	inventory, routes := energyPathPoolAirTestApply(t, doc)
	summaries := energyPathPoolAirTestSummaries(routes)
	bindEnergyPathPoolAirRoutePaths(routes, summaries, &inventory)
	for _, route := range routes {
		if len(route.RelatedPathIDs) != len(route.ServedZoneNames) {
			t.Fatalf("exact post-proof binding missing: %+v", route)
		}
	}
	for _, loop := range inventory.Loops {
		for _, demand := range loop.Demands {
			if demand.AirRouteComplete && len(demand.RelatedPathIDs) == 0 {
				t.Fatal("post-proof demand path metadata missing")
			}
		}
	}
	for _, mode := range []string{"wrong-Zone", "wrong-AirLoop", "wrong-delivery-index", "wrong-coil-index", "foreign-PlantLoop", "missing-one-Zone", "duplicate-path-ID", "incomplete-native"} {
		t.Run(mode, func(t *testing.T) {
			inventory, rows := energyPathPoolAirTestApply(t, doc)
			var routeIndex int
			for i := range rows {
				if rows[i].Coil.ObjectName == "OA Cooling Coil 1" {
					routeIndex = i
				}
			}
			target := rows[routeIndex]
			paths := energyPathPoolAirTestSummaries([]energyPathPoolAirRoute{target})
			switch mode {
			case "wrong-Zone":
				paths[0].Paths[0].ZoneName = "PLENUM-1"
			case "wrong-AirLoop":
				paths[0].Paths[0].AirLoop.ObjectIndex++
			case "wrong-delivery-index":
				paths[0].Paths[0].Delivery.ObjectIndex++
			case "wrong-coil-index":
				paths[0].Paths[0].Conditioning[0].ObjectIndex++
			case "foreign-PlantLoop":
				paths[0].Paths[0].PlantLoop.Name = "Hot Water Loop"
			case "missing-one-Zone":
				paths = paths[1:]
			case "duplicate-path-ID":
				paths[1].Paths[0].ID = paths[0].Paths[0].ID
			case "incomplete-native":
				rows[routeIndex].Complete = false
			}
			bindEnergyPathPoolAirRoutePaths(rows, paths, &inventory)
			if len(rows[routeIndex].RelatedPathIDs) != 0 {
				t.Fatal("partial/foreign navigation fabricated an original route")
			}
		})
	}
}

func TestEnergyPathPoolAirOAReusesOnlyProvedSameLoopMainNavigation(t *testing.T) {
	doc := energyPathPoolOriginal(t)
	for _, service := range []string{"cooling", "heating"} {
		for _, mode := range []string{"valid", "different-native-anchor-loop", "foreign-path-loop", "nil-path-loop", "wrong-main-coil", "wrong-service", "wrong-AirLoop", "wrong-terminal", "missing-recipient", "native-incomplete"} {
			t.Run(service+"/"+mode, func(t *testing.T) {
				inventory, rows := energyPathPoolAirTestApply(t, doc)
				var oaIndex int
				var main energyPathPoolAirRoute
				for index, row := range rows {
					if row.Position == "outside_air" && row.ServiceKind == service {
						oaIndex = index
					}
					if row.Position == "main_branch" && row.ServiceKind == service {
						main = row
					}
				}
				if main.Coil.ID == "" || !energyPathPoolAirSameRef(rows[oaIndex].NavigationAnchor, main.Coil) || !energyPathPoolAirSameRef(rows[oaIndex].NavigationAnchorPlantLoop, main.PlantLoop) {
					t.Fatal("native OA/main same-plant anchor was not independently proved")
				}
				// Only existing MAIN-coil navigation is supplied. No fabricated OA
				// path appears in this independently hand-framed navigation roster.
				paths := energyPathPoolAirTestSummaries([]energyPathPoolAirRoute{main})
				before, err := json.Marshal(paths)
				if err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "different-native-anchor-loop":
					rows[oaIndex].NavigationAnchorPlantLoop.ObjectIndex++
				case "foreign-path-loop":
					paths[0].Paths[0].PlantLoop.ObjectIndex++
				case "nil-path-loop":
					paths[0].Paths[0].PlantLoop = nil
				case "wrong-main-coil":
					paths[0].Paths[0].Conditioning[0].ObjectIndex++
				case "wrong-service":
					paths[0].Paths[0].ServiceKind = "ventilation"
				case "wrong-AirLoop":
					paths[0].Paths[0].AirLoop.ObjectIndex++
				case "wrong-terminal":
					paths[0].Paths[0].Delivery.ObjectIndex++
				case "missing-recipient":
					paths = paths[1:]
				case "native-incomplete":
					rows[oaIndex].Complete = false
				}
				bindEnergyPathPoolAirRoutePaths(rows, paths, &inventory)
				got := rows[oaIndex]
				if mode != "valid" {
					if len(got.RelatedPathIDs) != 0 || len(got.NavigationBindings) != 0 {
						t.Fatalf("fallback borrowed unproved or partial navigation: %+v", got)
					}
					return
				}
				after, err := json.Marshal(paths)
				if err != nil {
					t.Fatal(err)
				}
				if len(got.RelatedPathIDs) != 5 || len(got.NavigationBindings) != 5 || string(after) != string(before) {
					t.Fatalf("fallback navigation count or caller state changed: %+v", got)
				}
				for _, binding := range got.NavigationBindings {
					if binding.Mode != "proved_oa_via_same_loop_main_coil" || !energyPathPoolAirSameRef(binding.Anchor, main.Coil) || len(binding.PathIDs) != 1 {
						t.Fatalf("fallback omitted native anchor/mode trace: %+v", binding)
					}
					found := false
					for _, summary := range paths {
						for _, path := range summary.Paths {
							found = found || path.ID == binding.PathIDs[0]
						}
					}
					if !found {
						t.Fatal("binder created a fake navigation ID")
					}
				}
			})
		}
	}
}
