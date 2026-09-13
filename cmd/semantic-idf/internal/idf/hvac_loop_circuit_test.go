package idf

import (
	"os"
	"strings"
	"testing"
)

func TestHVACReferenceBranchOccurrencesUseTheirOwnCircuit(t *testing.T) {
	text, err := os.ReadFile("../../frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(string(text))
	if err != nil {
		t.Fatal(err)
	}
	report := AnalyzeHVAC(doc)
	objects := map[int]Object{}
	for _, object := range doc.Objects {
		objects[object.Index] = object
	}
	circuits := map[string]map[string]string{}
	for _, loop := range report.Loops {
		for _, side := range []HVACLoopSide{loop.SupplySide, loop.DemandSide} {
			for _, branch := range side.Branches {
				source := objects[branch.ObjectIndex]
				for _, component := range branch.Components {
					inlet, outlet := source.Fields[component.InletFieldIndex].Value, source.Fields[component.OutletFieldIndex].Value
					if component.InletNode != inlet || component.OutletNode != outlet {
						t.Fatalf("%s / %s: ports %q -> %q, Branch specifies %q -> %q", loop.Name, component.ObjectName, component.InletNode, component.OutletNode, inlet, outlet)
					}
					if circuits[component.ObjectName] == nil {
						circuits[component.ObjectName] = map[string]string{}
					}
					circuits[component.ObjectName][loop.Name] = inlet + "|" + outlet
				}
			}
		}
	}
	for _, pair := range [][3]string{{"VAV_5_CoolC", "VAV_5", "CoolSys1"}, {"VAV_5_HeatC", "VAV_5", "HeatSys1"}, {"CoolSys1 Chiller 1", "CoolSys1", "TowerWaterSys"}} {
		ports := circuits[pair[0]]
		if ports[pair[1]] == "" || ports[pair[2]] == "" || ports[pair[1]] == ports[pair[2]] {
			t.Fatalf("shared equipment lost distinct circuits: %v: %v", pair, ports)
		}
	}
	for _, relation := range report.ZoneRelations {
		for _, loopName := range relation.AirLoopNames {
			loop := findHVACTestingLoop(report, loopName)
			if loop == nil {
				t.Fatal(loopName)
			}
			for role, names := range map[string][]string{"zone_inlet": relation.Nodes.InletNodes, "zone_return": relation.Nodes.ReturnNodes} {
				for _, name := range names {
					found := false
					for _, node := range loop.DemandGraph.Nodes {
						found = found || strings.EqualFold(node.NodeName, name) && node.Role == role && node.ZoneName == relation.ZoneName
					}
					if !found {
						t.Fatalf("%s: %s %s lacks explicit zone %s in result topology", loopName, role, name, relation.ZoneName)
					}
				}
			}
		}
	}
}

func TestHVACZonePortsStayOnTheirConnectedAirLoop(t *testing.T) {
	relation := HVACZoneChain{ZoneName: "Shared zone", AirLoopNames: []string{"A", "B"},
		TerminalUnits: []HVACComponent{{InletNode: "A terminal inlet", OutletNode: "A zone inlet"}, {InletNode: "B terminal inlet", OutletNode: "B zone inlet"}},
		Nodes: HVACZoneNodes{Sources: []HVACZoneNodeSource{
			{Role: "inlet_nodes", Nodes: []string{"A zone inlet", "B zone inlet", "Unconnected inlet"}},
			{Role: "return_nodes", Nodes: []string{"A return", "B return"}},
		}},
	}
	for _, name := range []string{"A", "B"} {
		loop := HVACLoop{Name: name, Type: "AirLoopHVAC", DemandGraph: AirLoopDemandGraph{Nodes: []AirLoopDemandNode{
			{NodeName: name + " terminal inlet"}, {NodeName: name + " return"},
		}}}
		attachAirLoopZoneNodes(&loop, []HVACZoneChain{relation})
		count := 0
		for _, node := range loop.DemandGraph.Nodes {
			if node.ZoneName == "" {
				continue
			}
			count++
			if node.ZoneName != relation.ZoneName || node.NodeName != name+" zone inlet" && node.NodeName != name+" return" {
				t.Fatalf("zone membership broadened to an unrelated port: %+v", node)
			}
		}
		if count != 2 {
			t.Fatalf("%s zone ports = %d, want 2", name, count)
		}
	}
}
