package simulation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func heatOnlyIncompleteCohortUnit(t *testing.T, missing int) (idf.Document, idf.HVACReport) {
	t.Helper()
	doc := heatOnlyFanOutputOriginal(t)
	for _, name := range []string{"Zone1DirectAir", "Zone3DirectAir", "Zone2DirectAir"}[:missing] {
		heatOnlyFanOutputObject(t, &doc, "AirTerminal:SingleDuct:ConstantVolume:NoReheat", name).Fields[2].Value = "Disconnected " + name
	}
	return doc, idf.AnalyzeHVAC(doc)
}

func TestEnergyPathHeatOnlyCompleteCohortRequiredForBroadHeatingAndFanRequest(t *testing.T) {
	for missing := 0; missing <= 3; missing++ {
		t.Run(fmt.Sprintf("missing_%d", missing), func(t *testing.T) {
			doc, report := heatOnlyIncompleteCohortUnit(t, missing)
			path := filepath.Join(t.TempDir(), "Furnace.idf")
			if err := os.WriteFile(path, []byte(doc.String()), 0600); err != nil {
				t.Fatal(err)
			}
			topology := buildEnergyServicePathIndex(path)
			if topology.incompleteHeatOnly != (missing > 0) || len(topology.byService["heating"]) != 3-missing {
				t.Fatalf("allocation gate changed true path visibility: %+v", topology)
			}
			nodes := []EnergyExplanationNode{epath100AuditEndUseNode("heating", "natural_gas", 100, "M1", "literal.gas", topology.byService["heating"])}
			for _, zone := range []string{"West Zone", "EAST ZONE", "NORTH ZONE"} {
				nodes = append(nodes, epath100AuditLoadNode(zone, "heating", 1, "M1", "literal.load."+zone, topology.byZoneService[strings.ToLower(zone)+"|heating"]))
			}
			plan := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, nil, "M1", "monthly", true, topology)
			if len(plan.Records) != 1 {
				t.Fatalf("native broad budget lost its reconciliation: %+v", plan)
			}
			allocated, unassigned, requests := 100.0, 0.0, 1
			if missing > 0 {
				allocated, unassigned, requests = 0, 100, 0
			}
			if plan.Records[0].AllocatedValue != allocated || plan.Records[0].UnassignedValue != unassigned || len(energyPathAirLoopFanOutputKeys(doc)) != requests {
				t.Fatalf("incomplete cohort borrowed whole gas/fan authority: missing%d %+v", missing, plan)
			}
			if missing > 0 && len(plan.Edges) != 0 {
				t.Fatal("broad gas retained an allocation edge into a surviving sibling")
			}
			// The shared object/Zone report is not edited to achieve withholding.
			count := 0
			for _, zone := range report.ServiceModel.ZoneServices {
				for _, p := range zone.Paths {
					if p.ServiceKind == "heating" {
						count++
					}
				}
			}
			if count != 3-missing {
				t.Fatal("cohort gate erased valid source paths")
			}
		})
	}
}

func TestEnergyPathHeatOnlyIncompleteCohortKeepsDirectAndIndependentPaidSources(t *testing.T) {
	doc, report := heatOnlyIncompleteCohortUnit(t, 1)
	nodes, direct, topology, pool := epathHVACConsumptionPoolFixture()
	applyEnergyPathHeatOnlyCohortGuards(&topology, doc, report)
	plan := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, direct, "M1", "monthly", true, topology)
	if len(plan.Records) != 1 || plan.Records[0].DirectValue != 40 || plan.Records[0].AllocatedValue != 0 || plan.Records[0].UnassignedValue != 10 || len(plan.Edges) != 0 {
		t.Fatalf("incomplete broad remainder changed exact direct observations: %+v", plan)
	}
	// The literal independent boiler constituent retains its own five recipients;
	// it cannot authorize any Furnace share but must not lose its own budget.
	reserved := reserveEnergyPathHVACConsumptionPools(plan, nodes, topology, []energyPathHVACConsumptionPool{pool}, "M1", "monthly", true)
	epathHVACConsumptionPoolAssert(t, reserved, 50, 40, 10, 0, 0, map[string]float64{"SPACE1-1": 2, "SPACE2-1": 2, "SPACE3-1": 2, "SPACE4-1": 2, "SPACE5-1": 2})
}

func TestEnergyPathHeatOnlyIncompleteFanPoolDoesNotBorrowOtherLoopOrKnownZero(t *testing.T) {
	for _, scenario := range []string{"positive", "known-zero", "unknown-owner"} {
		t.Run(scenario, func(t *testing.T) {
			doc, report := heatOnlyIncompleteCohortUnit(t, 1)
			if scenario == "unknown-owner" {
				fanOutputAppend(t, &doc, "Branch,Unowned shared wrapper,,AirLoopHVAC:Unitary:Furnace:HeatOnly,Gas Furnace 1,In,Out;")
				report = idf.AnalyzeHVAC(doc)
			}
			value := 10.0
			if scenario == "known-zero" {
				value = 0
			}
			pools := []energyPathFanPool{
				{AirLoopName: "Typical Terminal Reheat 1", Monthly: map[int]float64{1: value}, Source: EnergyDataSource{ID: "literal.furnace.fan"}},
				{AirLoopName: "VAV_2", Monthly: map[int]float64{1: 20}, Source: EnergyDataSource{ID: "literal.other.fan"}},
			}
			topology := epathFanPoolTopology(2)
			for n := range topology.auxiliaryPaths {
				if topology.auxiliaryPaths[n].AirLoopName == "VAV_1" {
					topology.auxiliaryPaths[n].AirLoopName = pools[0].AirLoopName
				}
			}
			applyEnergyPathHeatOnlyCohortGuards(&topology, doc, report)
			nodes := epathFanPoolNodes(pools, "M1")
			plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true, pools)
			allocated, unassigned := 20.0, value
			if scenario == "unknown-owner" {
				allocated, unassigned = 0, value+20
			}
			if len(plan.Records) != 1 || plan.Records[0].AllocatedValue != allocated || plan.Records[0].UnassignedValue != unassigned {
				t.Fatalf("fan cohort escaped exact owner/knownness: %+v", plan)
			}
			for _, row := range plan.FanSourceAllocations {
				if row.SourceID != "literal.other.fan" || row.ZoneName != "Zone 2" && row.ZoneName != "Zone 3" {
					t.Fatalf("incomplete Furnace pool gained a fake Zone share: %+v", row)
				}
			}
			broad := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", true)
			if len(broad.Records) != 1 || broad.Records[0].AllocatedValue != 0 || broad.Records[0].UnassignedValue != value+20 {
				t.Fatalf("unmeasured broad fan allocation borrowed surviving paths: %+v", broad)
			}
		})
	}
}
