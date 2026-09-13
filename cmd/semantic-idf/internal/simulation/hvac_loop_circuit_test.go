package simulation

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestHVACReferenceResultsSeparateAirWaterAndCondenserCircuits(t *testing.T) {
	input := filepath.Join("..", "..", "frontend", "src", "samples", "RefBldgLargeOfficeNew2004_Chicago.idf")
	doc, err := simulationDocumentFromInput(input)
	if err != nil {
		t.Fatal(err)
	}
	series := []SimulationSeries{}
	seen := map[string]bool{}
	for _, usage := range idf.AnalyzeHVAC(doc).NodeUsages {
		key := strings.ToUpper(usage.NodeName)
		if seen[key] {
			continue
		}
		seen[key] = true
		for _, property := range [][2]string{{"System Node Temperature", "C"}, {"System Node Mass Flow Rate", "kg/s"}, {"System Node Setpoint Temperature", "C"}, {"System Node Relative Humidity", "%"}, {"System Node Humidity Ratio", "kg/kg"}} {
			series = append(series, hvacResultTestSeries(key, property[0], property[1]))
		}
	}
	series = append(series, hvacResultTestSeries("VAV_5_COOLC", "Cooling Coil Total Cooling Rate", "W"), hvacResultTestSeries("COOLSYS1 CHILLER 1", "Chiller COP", "W/W"))
	request := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeHVACLoopCheck}}
	results := buildHVACLoopRunResultsWithDocument(series, request, &doc)
	type expected struct {
		component       string
		present, absent []string
	}
	want := map[string]expected{
		"VAV_5":         {"VAV_5_CoolC", []string{"VAV_5_OA-VAV_5_CoolCNode", "VAV_5_CoolC-VAV_5_HeatCNode"}, []string{"VAV_5_CoolCDemand Inlet Node", "VAV_5_CoolCDemand Outlet Node"}},
		"CoolSys1":      {"VAV_5_CoolC", []string{"VAV_5_CoolCDemand Inlet Node", "VAV_5_CoolCDemand Outlet Node"}, []string{"VAV_5_OA-VAV_5_CoolCNode", "VAV_5_CoolC-VAV_5_HeatCNode", "CoolSys1 Chiller Water Inlet Node 1", "CoolSys1 Chiller Water Outlet Node 1"}},
		"TowerWaterSys": {"CoolSys1 Chiller 1", []string{"CoolSys1 Chiller Water Inlet Node 1", "CoolSys1 Chiller Water Outlet Node 1"}, []string{"CoolSys1 Pump-CoolSys1 ChillerNode 1", "CoolSys1 Supply Equipment Outlet Node 1"}},
	}
	for _, result := range results {
		nodes := map[string]bool{}
		for _, item := range result.Series {
			nodes[strings.ToUpper(item.KeyValue)] = true
			if hvacWaterLoop(result.LoopType) && strings.Contains(item.Name, "Humidity") {
				t.Fatalf("%s exposes humidity on a water circuit: %s", result.Name, item.Column)
			}
		}
		expect, ok := want[result.Name]
		if !ok {
			continue
		}
		for _, name := range expect.present {
			if !nodes[strings.ToUpper(name)] {
				t.Fatalf("%s lost actual circuit node %s", result.Name, name)
			}
		}
		for _, name := range expect.absent {
			if nodes[strings.ToUpper(name)] {
				t.Fatalf("%s includes other circuit node %s", result.Name, name)
			}
		}
		found := false
		for _, component := range result.Components {
			if !strings.EqualFold(component.ComponentName, expect.component) {
				continue
			}
			found = true
			for _, port := range component.NodePorts {
				if !strings.EqualFold(port.NodeName, expect.present[0]) && !strings.EqualFold(port.NodeName, expect.present[1]) {
					t.Fatalf("%s / %s exposes another circuit's port: %+v", result.Name, component.ComponentName, port)
				}
			}
		}
		if !found {
			t.Fatalf("%s lost shared equipment observations", result.Name)
		}
		delete(want, result.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing loops: %v", want)
	}
	request.Scope = SimulationPurposeScope{PlantLoopNames: []string{"CoolSys1"}, ComponentIDs: []string{"Coil:Cooling:Water:VAV_5_CoolC"}}
	plan := BuildPurposeRunPlan(doc, request)
	if findPurposeOutput(plan, "Output:Variable", "VAV_5_CoolCDemand Inlet Node", "System Node Temperature") == nil || findPurposeOutput(plan, "Output:Variable", "VAV_5_OA-VAV_5_CoolCNode", "System Node Temperature") != nil {
		t.Fatal("selected water coil plan requested the air circuit")
	}
	if findPurposeOutput(plan, "Output:Variable", "VAV_5_CoolCDemand Inlet Node", "System Node Relative Humidity") != nil || findPurposeOutput(plan, "Output:Variable", "VAV_5_CoolCDemand Inlet Node", "System Node Humidity Ratio") != nil {
		t.Fatal("selected water circuit requested humidity outputs")
	}
}
