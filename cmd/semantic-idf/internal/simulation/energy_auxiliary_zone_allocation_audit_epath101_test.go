package simulation

import (
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEPATH101AuditFanUsesEachRelatedAirLoopZoneOnce(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("main.office.cooling", "Office", "cooling", "Main Air", "", ""),
		epath101AuditPath("main.office.heating", "Office", "heating", "Main Air", "", ""),
		epath101AuditPath("main.office.ventilation", "Office", "ventilation", "Main Air", "", ""),
		epath101AuditPath("main.lab.cooling", "Lab", "cooling", "Main Air", "", ""),
		epath101AuditPath("main.lab.heating", "Lab", "heating", "Main Air", "", ""),
		epath101AuditPath("main.lab.ventilation", "Lab", "ventilation", "Main Air", "", ""),
		epath101AuditPath("rogue.cooling", "Rogue", "cooling", "Unrelated Air", "", ""),
	}
	mainPaths := epath101AuditPathIDs(paths[:6])
	nodes := []EnergyExplanationNode{
		epath101AuditAuxiliary("fans", 100, "meter.fans", mainPaths),
		// One physical zone load can carry several same-loop path IDs. Its weight
		// must be added once, not once per cooling/heating/ventilation path.
		epath101AuditLoad("Office", "cooling", 60, "load.office", mainPaths[:3]),
		epath101AuditLoad("Lab", "cooling", 40, "load.lab", mainPaths[3:]),
		epath101AuditLoad("Rogue", "cooling", 1000, "load.rogue", []string{"rogue.cooling"}),
	}

	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "annual", "annual", false)
	epath101AuditAssertRecord(t, plan.Records, "fans", 100, 0, 100, 0)
	if record := epath101AuditRecord(plan.Records, "fans"); record != nil && record.Method != "air_loop_load_share" {
		t.Errorf("fan allocation method = %q, want air_loop_load_share", record.Method)
	}
	epath101AuditAssertZoneAllocation(t, plan.Edges, "Office", 60, "load.office", "meter.fans", mainPaths[:3])
	epath101AuditAssertZoneAllocation(t, plan.Edges, "Lab", 40, "load.lab", "meter.fans", mainPaths[3:])
	if edge := epath101AuditAllocationEdge(plan.Edges, "Rogue"); edge != nil {
		t.Errorf("unrelated AirLoop entered the fan denominator: %#v", edge)
	}
	if got := epath101AuditAllocationTotal(plan.Edges); got != 100 {
		t.Errorf("fan allocation total = %g, want the meter total 100; duplicate paths were likely counted", got)
	}
}

func TestEPATH101AuditFanAirflowPriorityRequiresCompleteEvidence(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("main.office.cooling", "Office", "cooling", "Main Air", "", ""),
		epath101AuditPath("main.lab.cooling", "Lab", "cooling", "Main Air", "", ""),
	}
	base := []EnergyExplanationNode{
		epath101AuditAuxiliary("fans", 100, "meter.fans", epath101AuditPathIDs(paths)),
		epath101AuditLoad("Office", "cooling", 90, "load.office", []string{"main.office.cooling"}),
		epath101AuditLoad("Lab", "cooling", 10, "load.lab", []string{"main.lab.cooling"}),
	}

	t.Run("complete airflow outranks load share", func(t *testing.T) {
		nodes := append([]EnergyExplanationNode(nil), base...)
		nodes = append(nodes,
			epath101AuditAirflow("Office", 30, "airflow.office", []string{"main.office.cooling"}),
			epath101AuditAirflow("Lab", 70, "airflow.lab", []string{"main.lab.cooling"}),
		)
		plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "annual", "annual", false)
		if record := epath101AuditRecord(plan.Records, "fans"); record == nil || record.Method != "airflow_share" {
			t.Errorf("complete-flow method = %#v, want airflow_share", record)
		}
		epath101AuditAssertZoneAllocation(t, plan.Edges, "Office", 30, "airflow.office", "meter.fans", []string{"main.office.cooling"})
		epath101AuditAssertZoneAllocation(t, plan.Edges, "Lab", 70, "airflow.lab", "meter.fans", []string{"main.lab.cooling"})
		for _, edge := range plan.Edges {
			if !strings.Contains(strings.ToLower(edge.Formula), "airflow") && !strings.Contains(strings.ToLower(edge.Formula), "air flow") && !strings.Contains(strings.ToLower(edge.Formula), "supply-air volume") {
				t.Errorf("complete supply-air-volume evidence did not disclose airflow basis: %#v", edge)
			}
		}
	})

	t.Run("partial airflow falls back wholly to load share", func(t *testing.T) {
		nodes := append([]EnergyExplanationNode(nil), base...)
		nodes = append(nodes, epath101AuditAirflow("Office", 30, "airflow.office", []string{"main.office.cooling"}))
		plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "annual", "annual", false)
		if record := epath101AuditRecord(plan.Records, "fans"); record == nil || record.Method != "air_loop_load_share" {
			t.Errorf("partial-flow fallback method = %#v, want air_loop_load_share", record)
		}
		epath101AuditAssertZoneAllocation(t, plan.Edges, "Office", 90, "load.office", "meter.fans", []string{"main.office.cooling"})
		epath101AuditAssertZoneAllocation(t, plan.Edges, "Lab", 10, "load.lab", "meter.fans", []string{"main.lab.cooling"})
		for _, edge := range plan.Edges {
			if stringSliceContains(edge.SourceIDs, "airflow.office") {
				t.Errorf("partial airflow evidence contaminated all-or-nothing fallback: %#v", edge)
			}
		}
	})
}

func TestEPATH101AuditCompleteAirflowExplanationSurvivesV1Upgrade(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("main.office.cooling", "Office", "cooling", "Main Air", "", ""),
		epath101AuditPath("main.lab.cooling", "Lab", "cooling", "Main Air", "", ""),
	}
	nodes := []EnergyExplanationNode{
		epath101AuditCarrier(100),
		epath101AuditAuxiliary("fans", 100, "meter.fans", epath101AuditPathIDs(paths)),
		epath101AuditLoad("Office", "cooling", 90, "load.office", []string{"main.office.cooling"}),
		epath101AuditLoad("Lab", "cooling", 10, "load.lab", []string{"main.lab.cooling"}),
		epath101AuditAirflow("Office", 30, "airflow.office", []string{"main.office.cooling"}),
		epath101AuditAirflow("Lab", 70, "airflow.lab", []string{"main.lab.cooling"}),
	}
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            nodes,
		Edges: []EnergyExplanationEdge{
			{ID: "facility-fans", FromID: "energy.carrier.electricity", ToID: "energy.end_use.fans.electricity", Value: 100, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.fans"}},
		},
		Sources: []EnergyDataSource{
			{ID: "meter.fans", SourceType: "sql_meter", IsMeter: true},
			{ID: "load.office", SourceType: "sql_variable", ZoneName: "Office"},
			{ID: "load.lab", SourceType: "sql_variable", ZoneName: "Lab"},
			{ID: "airflow.office", SourceType: "sql_variable", ZoneName: "Office"},
			{ID: "airflow.lab", SourceType: "sql_variable", ZoneName: "Lab"},
		},
		servicePathIndex: epath101AuditTopology(paths),
	})
	for _, want := range []struct {
		zone  string
		value float64
		flow  string
		other string
	}{
		{zone: "Office", value: 30, flow: "airflow.office", other: "airflow.lab"},
		{zone: "Lab", value: 70, flow: "airflow.lab", other: "airflow.office"},
	} {
		zone := epath101AuditZoneResult(result.ZoneResults, want.zone)
		if zone == nil {
			t.Fatalf("missing %s complete-airflow result: %#v", want.zone, result.AvailableZones)
		}
		node := epath101AuditNode(zone.Nodes, "end_use.fans."+metricID(want.zone))
		explanation := ""
		if node != nil {
			explanation = strings.ToLower(node.AllocationExplanation)
		}
		if node == nil || node.Value != want.value || node.Basis != "service_path_allocation" ||
			!strings.Contains(explanation, "airloop") || !strings.Contains(explanation, "supply-air volume") ||
			!stringSliceContains(node.SourceIDs, want.flow) || stringSliceContains(node.SourceIDs, want.other) {
			t.Errorf("%s airflow allocation explanation/provenance was lost during v1 upgrade: %#v", want.zone, node)
		}
		for _, rendered := range zone.Nodes {
			if strings.EqualFold(rendered.Level, "airflow") || energyPathZoneAuxiliaryIsSupplyAirflow(rendered) {
				t.Errorf("optional airflow evidence leaked into %s Energy Path stages: %#v", want.zone, rendered)
			}
		}
	}
}

func TestEPATH101AuditPumpPlantLoopServiceSeparation(t *testing.T) {
	tests := []struct {
		name       string
		service    string
		loopName   string
		decoy      string
		wantOffice float64
		wantLab    float64
	}{
		{name: "cooling plant", service: "cooling", loopName: "CHW Loop", decoy: "heating", wantOffice: 75, wantLab: 25},
		{name: "heating plant", service: "heating", loopName: "HW Loop", decoy: "cooling", wantOffice: 20, wantLab: 80},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := []energyPathAuxiliaryServicePath{
				epath101AuditPath("target.office", "Office", test.service, "", test.loopName, ""),
				epath101AuditPath("target.lab", "Lab", test.service, "", test.loopName, ""),
				epath101AuditPath("decoy.office", "Office", test.decoy, "", "Other Plant", ""),
				epath101AuditPath("decoy.lab", "Lab", test.decoy, "", "Other Plant", ""),
			}
			nodes := []EnergyExplanationNode{
				epath101AuditAuxiliary("pumps", 100, "meter.pumps", []string{"target.office", "target.lab"}),
				epath101AuditLoad("Office", test.service, test.wantOffice, "load.target.office", []string{"target.office"}),
				epath101AuditLoad("Lab", test.service, test.wantLab, "load.target.lab", []string{"target.lab"}),
				epath101AuditLoad("Office", test.decoy, 900, "load.decoy.office", []string{"decoy.office"}),
				epath101AuditLoad("Lab", test.decoy, 100, "load.decoy.lab", []string{"decoy.lab"}),
			}
			plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "annual", "annual", false)
			if record := epath101AuditRecord(plan.Records, "pumps"); record == nil || record.Method != "plant_loop_load_share" {
				t.Errorf("%s pump method = %#v, want plant_loop_load_share", test.service, record)
			}
			epath101AuditAssertZoneAllocation(t, plan.Edges, "Office", test.wantOffice, "load.target.office", "meter.pumps", []string{"target.office"})
			epath101AuditAssertZoneAllocation(t, plan.Edges, "Lab", test.wantLab, "load.target.lab", "meter.pumps", []string{"target.lab"})
			for _, edge := range plan.Edges {
				if stringSliceContains(edge.SourceIDs, "load.decoy.office") || stringSliceContains(edge.SourceIDs, "load.decoy.lab") ||
					stringSliceContains(edge.RelatedPathIDs, "decoy.office") || stringSliceContains(edge.RelatedPathIDs, "decoy.lab") {
					t.Errorf("%s pump allocation crossed service/PlantLoop: %#v", test.service, edge)
				}
			}
		})
	}
}

func TestEPATH101AuditHeatRejectionStaysOnRelatedCoolingCondenserLoop(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("tower.office.cooling", "Office", "cooling", "", "CHW Loop", "Tower Loop"),
		epath101AuditPath("tower.lab.cooling", "Lab", "cooling", "", "CHW Loop", "Tower Loop"),
		epath101AuditPath("tower.office.heating", "Office", "heating", "", "HW Loop", "Heating Condenser"),
		epath101AuditPath("other.rogue.cooling", "Rogue", "cooling", "", "Other CHW", "Other Condenser"),
	}
	nodes := []EnergyExplanationNode{
		epath101AuditAuxiliary("heat_rejection", 50, "meter.heat_rejection", []string{"tower.office.cooling", "tower.lab.cooling"}),
		epath101AuditLoad("Office", "cooling", 30, "load.office.cooling", []string{"tower.office.cooling"}),
		epath101AuditLoad("Lab", "cooling", 20, "load.lab.cooling", []string{"tower.lab.cooling"}),
		epath101AuditLoad("Office", "heating", 1000, "load.office.heating", []string{"tower.office.heating"}),
		epath101AuditLoad("Rogue", "cooling", 1000, "load.rogue.cooling", []string{"other.rogue.cooling"}),
	}
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "annual", "annual", false)
	if record := epath101AuditRecord(plan.Records, "heat_rejection"); record == nil || record.Method != "condenser_loop_load_share" {
		t.Errorf("heat rejection method = %#v, want condenser_loop_load_share", record)
	}
	epath101AuditAssertZoneAllocation(t, plan.Edges, "Office", 30, "load.office.cooling", "meter.heat_rejection", []string{"tower.office.cooling"})
	epath101AuditAssertZoneAllocation(t, plan.Edges, "Lab", 20, "load.lab.cooling", "meter.heat_rejection", []string{"tower.lab.cooling"})
	if edge := epath101AuditAllocationEdge(plan.Edges, "Rogue"); edge != nil {
		t.Errorf("cross-CondenserLoop target received heat rejection energy: %#v", edge)
	}
	for _, edge := range plan.Edges {
		if stringSliceContains(edge.SourceIDs, "load.office.heating") || stringSliceContains(edge.RelatedPathIDs, "tower.office.heating") {
			t.Errorf("heat rejection crossed into heating service on its CondenserLoop: %#v", edge)
		}
	}
}

func TestEPATH101AuditAmbiguousSharedAndDHWPlantRemainBuildingUnassigned(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("shared.office.cooling", "Office", "cooling", "", "Shared Plant", ""),
		epath101AuditPath("shared.lab.heating", "Lab", "heating", "", "Shared Plant", ""),
		epath101AuditPath("dhw.kitchen", "Kitchen", "service_water_heating", "", "DHW Plant", ""),
	}
	nodes := []EnergyExplanationNode{
		epath101AuditAuxiliary("pumps", 30, "meter.pumps", epath101AuditPathIDs(paths)),
		epath101AuditLoad("Office", "cooling", 60, "load.office", []string{"shared.office.cooling"}),
		epath101AuditLoad("Lab", "heating", 40, "load.lab", []string{"shared.lab.heating"}),
		epath101AuditLoad("Kitchen", "service_water_heating", 500, "load.kitchen", []string{"dhw.kitchen"}),
	}
	input := EnergyExplanationV1{
		Schema:                energyExplanationV1Schema,
		AllocationPolicy:      PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:                 nodes,
		servicePathIndex:      epath101AuditTopology(paths),
		canonicalMonthlyBasis: false,
	}
	result := UpgradeEnergyExplanationV1(input)
	if len(result.ZoneResults) != 3 {
		t.Fatalf("available zone results = %#v, want Office/Lab/Kitchen", result.AvailableZones)
	}
	for _, zone := range result.ZoneResults {
		if node := epath101AuditNode(zone.Nodes, "end_use.pumps."+metricID(zone.Scope.ZoneName)); node != nil && math.Abs(node.Value) > energyPathZoneHVACAllocationEpsilon {
			t.Errorf("ambiguous/DHW plant energy leaked into %s: %#v", zone.Scope.ZoneName, node)
		}
	}
	row := epath101AuditAuxiliaryReconciliation(result.Reconciliation, "pumps", "electricity", "annual")
	if row == nil || row.ExpectedValue != 30 || row.DirectValue != 0 || row.AllocatedValue != 0 || row.UnassignedValue != 30 || row.ResidualValue != 30 || row.Status == "balanced" || row.AllocationMethod != "unassigned" {
		t.Fatalf("ambiguous shared/DHW plant accounting = %#v", row)
	}
	if !epath101AuditHasWarning(result.Warnings, "unassigned_building_hvac_auxiliary_energy") {
		t.Errorf("unassigned auxiliary warning missing: %#v", result.Warnings)
	}
	for _, zone := range result.ZoneResults {
		quality := epath101AuditAuxiliaryReconciliation(zone.Reconciliation, "pumps", "electricity", "annual")
		if quality == nil || quality.ExpectedValue != 30 || quality.DirectValue != 0 || quality.AllocatedValue != 0 || quality.UnassignedValue != 30 || quality.AllocationMethod != "unassigned" ||
			!strings.Contains(strings.ToLower(quality.Label+" "+quality.Formula), "building") {
			t.Errorf("%s zone quality did not expose truthful model coverage: %#v", zone.Scope.ZoneName, quality)
		}
		if epath101AuditHasWarning(zone.Warnings, "unassigned_building_hvac_auxiliary_energy") {
			t.Errorf("Building-only unassigned warning leaked into %s: %#v", zone.Scope.ZoneName, zone.Warnings)
		}
	}
}

func TestEPATH101AuditDHWOnlyPumpIsEntirelyUnassigned(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("dhw.kitchen", "Kitchen", "service_water_heating", "", "DHW Plant", ""),
	}
	nodes := []EnergyExplanationNode{
		epath101AuditAuxiliary("pumps", 25, "meter.pumps", nil),
		epath101AuditLoad("Kitchen", "service_water_heating", 500, "load.kitchen.dhw", []string{"dhw.kitchen"}),
	}
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "annual", "annual", false)
	if len(plan.Edges) != 0 {
		t.Errorf("DHW-only pump was misclassified as space heating: %#v", plan.Edges)
	}
	record := epath101AuditRecord(plan.Records, "pumps")
	if record == nil || record.ExpectedValue != 25 || record.DirectValue != 0 || record.AllocatedValue != 0 || record.UnassignedValue != 25 || record.Method != "unassigned" {
		t.Errorf("DHW-only pump record = %#v, want 25 kWh entirely unassigned", record)
	}
}

func TestEPATH101AuditSamePlantLoopCoolingAndDHWIsAmbiguousAndUnassigned(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("shared.office.cooling", "Office", "cooling", "", "Shared CHW-DHW Plant", ""),
		epath101AuditPath("shared.kitchen.dhw", "Kitchen", "service_water_heating", "", "Shared CHW-DHW Plant", ""),
	}
	components := []idf.ComponentIndexItem{{
		Component: idf.ComponentRef{ObjectType: "Pump:VariableSpeed", ObjectName: "Shared Plant Pump"},
		Occurrences: []idf.ComponentOccurrence{{
			ContextType: "plant_branch",
			LoopName:    "Shared CHW-DHW Plant",
			LoopType:    "PlantLoop",
			BranchName:  "Shared Supply Branch",
		}},
		RelatedPathIDs:   []string{"shared.office.cooling", "shared.kitchen.dhw"},
		RelatedZoneNames: []string{"Office", "Kitchen"},
		RelatedLoopRefs:  []idf.LoopRef{{Name: "Shared CHW-DHW Plant", Type: "PlantLoop"}},
	}}
	resolvable := energyPathAuxiliaryResolvableInventory(components, paths)
	if resolvable["pumps"] {
		t.Errorf("production inventory resolver certified a mixed cooling+DHW pump as unambiguous: %#v", resolvable)
	}
	topology := energyServicePathIndex{
		auxiliaryPaths:      append([]energyPathAuxiliaryServicePath(nil), paths...),
		auxiliaryResolvable: resolvable,
	}
	nodes := []EnergyExplanationNode{
		epath101AuditCarrier(50),
		epath101AuditAuxiliary("pumps", 50, "meter.pumps.shared", epath101AuditPathIDs(paths)),
		epath101AuditLoad("Office", "cooling", 100, "load.office.cooling", []string{"shared.office.cooling"}),
		epath101AuditLoad("Kitchen", "service_water_heating", 500, "load.kitchen.dhw", []string{"shared.kitchen.dhw"}),
	}
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "annual", "annual", false)
	if len(plan.Edges) != 0 {
		t.Errorf("mixed cooling+DHW PlantLoop produced Zone pump allocations: %#v", plan.Edges)
	}
	record := epath101AuditRecord(plan.Records, "pumps")
	if record == nil || record.ExpectedValue != 50 || record.DirectValue != 0 || record.AllocatedValue != 0 || record.UnassignedValue != 50 || record.Method != "unassigned" ||
		!stringSliceContains(record.SourceIDs, "meter.pumps.shared") {
		t.Errorf("mixed cooling+DHW pump accounting = %#v, want entire 50 kWh unassigned", record)
	}

	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            nodes,
		Edges: []EnergyExplanationEdge{
			{ID: "facility-pumps", FromID: "energy.carrier.electricity", ToID: "energy.end_use.pumps.electricity", Value: 50, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.pumps.shared"}},
		},
		Sources:          []EnergyDataSource{{ID: "meter.pumps.shared", SourceType: "sql_meter", IsMeter: true}},
		servicePathIndex: topology,
	})
	for _, zone := range result.ZoneResults {
		if pump := epath101AuditNode(zone.Nodes, "end_use.pumps."+metricID(zone.Scope.ZoneName)); pump != nil && math.Abs(pump.Value) > energyPathZoneHVACAllocationEpsilon {
			t.Errorf("mixed cooling+DHW pump leaked into %s Zone result: %#v", zone.Scope.ZoneName, pump)
		}
	}
	row := epath101AuditAuxiliaryReconciliation(result.Reconciliation, "pumps", "electricity", "annual")
	if row == nil || row.ExpectedValue != 50 || row.AllocatedValue != 0 || row.UnassignedValue != 50 || row.ResidualValue != 50 || row.AllocationMethod != "unassigned" {
		t.Errorf("mixed cooling+DHW Building quality = %#v", row)
	}
	if !epath101AuditHasWarning(result.Warnings, "unassigned_building_hvac_auxiliary_energy") {
		t.Errorf("mixed cooling+DHW Building result omitted unassigned warning: %#v", result.Warnings)
	}
}

func TestEPATH101AuditDirectAuxiliaryWinsWithoutBroadMeterDoubleCount(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("main.office", "Office", "cooling", "Main Air", "", ""),
		epath101AuditPath("main.lab", "Lab", "cooling", "Main Air", "", ""),
	}
	nodes := []EnergyExplanationNode{
		epath101AuditCarrier(100),
		epath101AuditAuxiliary("fans", 100, "meter.fans", epath101AuditPathIDs(paths)),
		epath101AuditDirectAuxiliary("Office", "fans", 30, "component.fan.office", []string{"main.office"}),
		epath101AuditLoad("Office", "cooling", 60, "load.office", []string{"main.office"}),
		epath101AuditLoad("Lab", "cooling", 40, "load.lab", []string{"main.lab"}),
	}
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "annual", "annual", false)
	epath101AuditAssertRecord(t, plan.Records, "fans", 100, 30, 70, 0)
	if edge := epath101AuditAllocationEdge(plan.Edges, "Office"); edge != nil {
		t.Errorf("zone with exact component fan energy also received broad-meter allocation: %#v", edge)
	}
	epath101AuditAssertZoneAllocation(t, plan.Edges, "Lab", 70, "load.lab", "meter.fans", []string{"main.lab"})

	integrationNodes := append([]EnergyExplanationNode(nil), nodes[:2]...)
	integrationNodes = append(integrationNodes, nodes[3:]...)
	directSeries := energyExplanationSeries{
		Stage: "end_use", CanonicalKind: "energy.fans", Level: "energy", Kind: "energy.fans", Label: "Office exact fan",
		Unit: "kWh site", Carrier: "electricity", EndUse: "fans", ZoneName: "Office", Basis: "direct_zone_energy",
		SourceIDs: []string{"component.fan.office"}, AnnualSourceIDs: []string{"component.fan.office"}, Total: 30,
	}
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            integrationNodes,
		Edges: []EnergyExplanationEdge{
			{ID: "facility-fans", FromID: "energy.carrier.electricity", ToID: "energy.end_use.fans.electricity", Value: 100, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.fans"}},
		},
		Sources:             []EnergyDataSource{{ID: "component.fan.office", SourceType: "sql_variable", ZoneName: "Office"}},
		zoneDirectUseSeries: []energyExplanationSeries{directSeries},
		servicePathIndex:    epath101AuditTopology(paths),
	})
	office := epath101AuditZoneResult(result.ZoneResults, "Office")
	lab := epath101AuditZoneResult(result.ZoneResults, "Lab")
	if office == nil || lab == nil {
		t.Fatalf("missing direct/allocation zone results: %#v", result.AvailableZones)
	}
	if node := epath101AuditNode(office.Nodes, "end_use.fans.office"); node == nil || node.Value != 30 || node.Basis != "direct_zone_energy" {
		t.Errorf("Office exact fan branch = %#v", node)
	}
	if node := epath101AuditNode(lab.Nodes, "end_use.fans.lab"); node == nil || node.Value != 70 || node.Basis != "service_path_allocation" {
		t.Errorf("Lab allocated fan branch = %#v", node)
	}
	if building := epath101AuditNode(result.Nodes, "end_use.fans.building"); building == nil || building.Value != 100 {
		t.Errorf("Building broad fan meter double counted component evidence: %#v", building)
	}
}

func TestEPATH101AuditCanonicalAnnualAuxiliaryAllocationSumsMonths(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("main.office", "Office", "cooling", "Main Air", "", ""),
		epath101AuditPath("main.lab", "Lab", "cooling", "Main Air", "", ""),
	}
	graph := func(period string, fan, office, lab float64) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
		nodes := []EnergyExplanationNode{
			epath101AuditCarrier(fan),
			epath101AuditAuxiliary("fans", fan, "meter.fans."+period, epath101AuditPathIDs(paths)),
			epath101AuditLoad("Office", "cooling", office, "load.office."+period, []string{"main.office"}),
			epath101AuditLoad("Lab", "cooling", lab, "load.lab."+period, []string{"main.lab"}),
		}
		for index := range nodes {
			nodes[index].Period = period
		}
		return nodes, []EnergyExplanationEdge{{ID: "facility-fans." + period, FromID: "energy.carrier.electricity", ToID: "energy.end_use.fans.electricity", Value: fan, Unit: "kWh site", Period: period, Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.fans." + period}}}
	}
	annualNodes, annualEdges := graph("annual", 200, 20, 180) // deliberately conflicts with monthly shares
	m1Nodes, m1Edges := graph("M1", 100, 90, 10)
	m2Nodes, m2Edges := graph("M2", 100, 10, 90)
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema:                energyExplanationV1Schema,
		AllocationPolicy:      PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:                 annualNodes,
		Edges:                 annualEdges,
		Periods:               []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: m1Nodes, Edges: m1Edges}, {ID: "M2", Kind: "monthly", Nodes: m2Nodes, Edges: m2Edges}},
		canonicalMonthlyBasis: true,
		servicePathIndex:      epath101AuditTopology(paths),
	})
	for _, zoneName := range []string{"Office", "Lab"} {
		zone := epath101AuditZoneResult(result.ZoneResults, zoneName)
		if zone == nil {
			t.Fatalf("missing %s result", zoneName)
		}
		annual := epath101AuditNode(zone.Nodes, "end_use.fans."+metricID(zoneName))
		if annual == nil || annual.Value != 100 || annual.Basis != "service_path_allocation" {
			t.Errorf("%s annual fan allocation = %#v, want monthly sum 100", zoneName, annual)
		}
		if got := epath101AuditMonthlyNodeSum(zone.Periods, "end_use.fans."+metricID(zoneName)); got != 100 {
			t.Errorf("%s monthly fan sum = %g, want 100", zoneName, got)
		}
	}
	row := epath101AuditAuxiliaryReconciliation(result.Reconciliation, "fans", "electricity", "annual")
	if row == nil || row.ExpectedValue != 200 || row.AllocatedValue != 200 || row.UnassignedValue != 0 || row.ResidualValue != 0 {
		t.Errorf("annual fan reconciliation did not aggregate months: %#v", row)
	}
}

func TestEPATH101AuditAnnualOnlyDirectAuxiliaryWinsWithoutMonthlyFabrication(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("main.office", "Office", "cooling", "Main Air", "", ""),
		epath101AuditPath("main.lab", "Lab", "cooling", "Main Air", "", ""),
	}
	graph := func(period string, fan float64) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
		nodes := []EnergyExplanationNode{
			epath101AuditCarrier(fan),
			epath101AuditAuxiliary("fans", fan, "meter.fans."+period, epath101AuditPathIDs(paths)),
			epath101AuditLoad("Office", "cooling", 50, "load.office."+period, []string{"main.office"}),
			epath101AuditLoad("Lab", "cooling", 50, "load.lab."+period, []string{"main.lab"}),
		}
		for index := range nodes {
			nodes[index].Period = period
		}
		return nodes, []EnergyExplanationEdge{{ID: "facility-fans." + period, FromID: "energy.carrier.electricity", ToID: "energy.end_use.fans.electricity", Value: fan, Unit: "kWh site", Period: period, Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.fans." + period}}}
	}
	annualNodes, annualEdges := graph("annual", 200)
	m1Nodes, m1Edges := graph("M1", 100)
	m2Nodes, m2Edges := graph("M2", 100)
	direct := energyExplanationSeries{
		Stage: "end_use", CanonicalKind: "energy.fans", Level: "energy", Kind: "energy.fans", Label: "Office exact fan",
		Unit: "kWh site", Carrier: "electricity", EndUse: "fans", ZoneName: "Office", Basis: "direct_zone_energy",
		SourceIDs: []string{"direct.office.fans.annual"}, AnnualSourceIDs: []string{"direct.office.fans.annual"}, Total: 30,
	}
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema:                energyExplanationV1Schema,
		AllocationPolicy:      PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:                 annualNodes,
		Edges:                 annualEdges,
		Periods:               []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: m1Nodes, Edges: m1Edges}, {ID: "M2", Kind: "monthly", Nodes: m2Nodes, Edges: m2Edges}},
		Sources:               []EnergyDataSource{{ID: "direct.office.fans.annual", SourceType: "sql_variable", ZoneName: "Office"}},
		zoneDirectUseSeries:   []energyExplanationSeries{direct},
		canonicalMonthlyBasis: true,
		servicePathIndex:      epath101AuditTopology(paths),
	})
	office := epath101AuditZoneResult(result.ZoneResults, "Office")
	lab := epath101AuditZoneResult(result.ZoneResults, "Lab")
	if office == nil || lab == nil {
		t.Fatalf("missing annual-direct fan zones: %#v", result.AvailableZones)
	}
	officeAnnual := epath101AuditNode(office.Nodes, "end_use.fans.office")
	labAnnual := epath101AuditNode(lab.Nodes, "end_use.fans.lab")
	if officeAnnual == nil || officeAnnual.Value != 30 || officeAnnual.Basis != "direct_zone_energy" || !stringSliceContains(officeAnnual.SourceIDs, "direct.office.fans.annual") {
		t.Errorf("annual exact fan truth = %#v, want direct Office 30", officeAnnual)
	}
	if labAnnual == nil || labAnnual.Value != 170 || labAnnual.Basis != "service_path_allocation" {
		t.Errorf("annual fan remainder = %#v, want Lab allocation 170", labAnnual)
	}
	for _, zone := range []*EnergyExplanationZoneResult{office, lab} {
		for _, periodID := range []string{"M1", "M2"} {
			period := epath101AuditPeriod(zone.Periods, periodID)
			var node *EnergyExplanationNode
			if period != nil {
				node = epath101AuditNode(period.Nodes, "end_use.fans."+metricID(zone.Scope.ZoneName))
			}
			if node == nil || node.Value != 50 || node.Basis != "service_path_allocation" || stringSliceContains(node.SourceIDs, "direct.office.fans.annual") {
				t.Errorf("%s %s fabricated annual-only direct fan profile: %#v", zone.Scope.ZoneName, periodID, node)
			}
		}
	}
	row := epath101AuditAuxiliaryReconciliation(result.Reconciliation, "fans", "electricity", "annual")
	if row == nil || row.ExpectedValue != 200 || row.DirectValue != 30 || row.AllocatedValue != 170 || row.UnassignedValue != 0 || row.ResidualValue != 0 || !stringSliceContains(row.SourceIDs, "direct.office.fans.annual") {
		t.Errorf("annual-only direct auxiliary accounting = %#v", row)
	}
}

func TestEPATH101AuditAnnualDirectOverrideIsCarrierQualified(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("main.office", "Office", "cooling", "Main Air", "", ""),
		epath101AuditPath("main.lab", "Lab", "cooling", "Main Air", "", ""),
	}
	graph := func(period string, energy, officeLoad, labLoad float64) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
		nodes := []EnergyExplanationNode{
			epath101AuditCarrierFor("electricity", energy, "meter.facility.electricity."+period),
			epath101AuditCarrierFor("natural_gas", energy, "meter.facility.gas."+period),
			epath101AuditAuxiliaryForCarrier("fans", "electricity", energy, "meter.fans.electricity."+period, epath101AuditPathIDs(paths)),
			epath101AuditAuxiliaryForCarrier("fans", "natural_gas", energy, "meter.fans.gas."+period, epath101AuditPathIDs(paths)),
			epath101AuditLoad("Office", "cooling", officeLoad, "load.office."+period, []string{"main.office"}),
			epath101AuditLoad("Lab", "cooling", labLoad, "load.lab."+period, []string{"main.lab"}),
		}
		for index := range nodes {
			nodes[index].Period = period
		}
		edges := []EnergyExplanationEdge{
			{ID: "facility-fans-electricity." + period, FromID: "energy.carrier.electricity", ToID: "energy.end_use.fans.electricity", Value: energy, Unit: "kWh site", Period: period, Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.fans.electricity." + period}},
			{ID: "facility-fans-gas." + period, FromID: "energy.carrier.natural_gas", ToID: "energy.end_use.fans.natural_gas", Value: energy, Unit: "kWh site", Period: period, Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.fans.gas." + period}},
		}
		return nodes, edges
	}
	// Annual load evidence intentionally disagrees with the monthly sum. Only
	// the electricity branch has annual-only direct evidence and may use the
	// annual-authoritative 90/10 plan; natural gas must remain monthly-first.
	annualNodes, annualEdges := graph("annual", 200, 90, 10)
	m1Nodes, m1Edges := graph("M1", 100, 10, 90)
	m2Nodes, m2Edges := graph("M2", 100, 30, 70)
	direct := energyExplanationSeries{
		Stage: "end_use", CanonicalKind: "energy.fans", Level: "energy", Kind: "energy.fans", Label: "Office exact electric fan",
		Unit: "kWh site", Carrier: "electricity", EndUse: "fans", ZoneName: "Office", Basis: "direct_zone_energy",
		SourceIDs: []string{"direct.office.fans.electricity.annual"}, AnnualSourceIDs: []string{"direct.office.fans.electricity.annual"}, Total: 30,
	}
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            annualNodes,
		Edges:            annualEdges,
		Periods: []EnergyPeriod{
			{ID: "M1", Kind: "monthly", Nodes: m1Nodes, Edges: m1Edges},
			{ID: "M2", Kind: "monthly", Nodes: m2Nodes, Edges: m2Edges},
		},
		Sources: []EnergyDataSource{
			{ID: "direct.office.fans.electricity.annual", SourceType: "sql_variable", ZoneName: "Office"},
			{ID: "meter.fans.electricity.annual", SourceType: "sql_meter", IsMeter: true},
			{ID: "meter.fans.gas.annual", SourceType: "sql_meter", IsMeter: true},
			{ID: "meter.fans.electricity.M1", SourceType: "sql_meter", IsMeter: true},
			{ID: "meter.fans.electricity.M2", SourceType: "sql_meter", IsMeter: true},
			{ID: "meter.fans.gas.M1", SourceType: "sql_meter", IsMeter: true},
			{ID: "meter.fans.gas.M2", SourceType: "sql_meter", IsMeter: true},
		},
		zoneDirectUseSeries:   []energyExplanationSeries{direct},
		canonicalMonthlyBasis: true,
		servicePathIndex:      epath101AuditTopology(paths),
	})

	for _, want := range []struct {
		zone            string
		annualEndUse    float64
		annualGas       float64
		monthlyGas      map[string]float64
		forbiddenSource string
	}{
		{zone: "Office", annualEndUse: 70, annualGas: 40, monthlyGas: map[string]float64{"M1": 10, "M2": 30}, forbiddenSource: "meter.fans.gas.annual"},
		{zone: "Lab", annualEndUse: 330, annualGas: 160, monthlyGas: map[string]float64{"M1": 90, "M2": 70}, forbiddenSource: "meter.fans.gas.annual"},
	} {
		zone := epath101AuditZoneResult(result.ZoneResults, want.zone)
		if zone == nil {
			t.Fatalf("missing %s mixed-carrier fan result", want.zone)
		}
		endUseID := "end_use.fans." + metricID(want.zone)
		gasCarrierID := "carrier.natural_gas." + metricID(want.zone)
		endUse := epath101AuditNode(zone.Nodes, endUseID)
		gasCarrier := epath101AuditNode(zone.Nodes, gasCarrierID)
		gasLink := epath101AuditLink(zone.Links, endUseID, gasCarrierID)
		if endUse == nil || endUse.Value != want.annualEndUse {
			t.Errorf("%s mixed-carrier annual fan total = %#v, want %.6g", want.zone, endUse, want.annualEndUse)
		}
		if gasCarrier == nil || gasCarrier.Value != want.annualGas || gasLink == nil || gasLink.ToValue != want.annualGas || gasLink.Basis != "service_path_allocation" ||
			stringSliceContains(gasCarrier.SourceIDs, "direct.office.fans.electricity.annual") || stringSliceContains(gasLink.SourceIDs, "direct.office.fans.electricity.annual") ||
			stringSliceContains(gasCarrier.SourceIDs, want.forbiddenSource) || stringSliceContains(gasLink.SourceIDs, want.forbiddenSource) ||
			!stringSliceContains(gasCarrier.SourceIDs, "meter.fans.gas.M1") || !stringSliceContains(gasCarrier.SourceIDs, "meter.fans.gas.M2") ||
			!stringSliceContains(gasLink.SourceIDs, "meter.fans.gas.M1") || !stringSliceContains(gasLink.SourceIDs, "meter.fans.gas.M2") {
			t.Errorf("%s natural-gas fan branch was replaced by electric annual override: carrier=%#v link=%#v", want.zone, gasCarrier, gasLink)
		}
		for periodID, value := range want.monthlyGas {
			period := epath101AuditPeriod(zone.Periods, periodID)
			var periodCarrier *EnergyExplanationNode
			if period != nil {
				periodCarrier = epath101AuditNode(period.Nodes, gasCarrierID)
			}
			if periodCarrier == nil || periodCarrier.Value != value || stringSliceContains(periodCarrier.SourceIDs, "direct.office.fans.electricity.annual") {
				t.Errorf("%s %s natural-gas fan profile changed by electric annual direct evidence: %#v", want.zone, periodID, periodCarrier)
			}
		}
	}
	electricityRow := epath101AuditAuxiliaryReconciliation(result.Reconciliation, "fans", "electricity", "annual")
	gasRow := epath101AuditAuxiliaryReconciliation(result.Reconciliation, "fans", "natural_gas", "annual")
	if electricityRow == nil || electricityRow.DirectValue != 30 || electricityRow.AllocatedValue != 170 || electricityRow.UnassignedValue != 0 {
		t.Errorf("electricity annual-direct accounting = %#v", electricityRow)
	}
	if gasRow == nil || gasRow.DirectValue != 0 || gasRow.AllocatedValue != 200 || gasRow.ExpectedValue != 200 || gasRow.UnassignedValue != 0 ||
		stringSliceContains(gasRow.SourceIDs, "direct.office.fans.electricity.annual") || stringSliceContains(gasRow.SourceIDs, "meter.fans.gas.annual") ||
		!stringSliceContains(gasRow.SourceIDs, "meter.fans.gas.M1") || !stringSliceContains(gasRow.SourceIDs, "meter.fans.gas.M2") {
		t.Errorf("natural-gas monthly-first accounting was contaminated: %#v", gasRow)
	}
}

func TestEPATH101AuditProductionTopologyResolvesBroadAuxiliariesWithoutSeededPaths(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test source for production IDF fixture")
	}
	inputPath := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "frontend", "src", "samples", "RefBldgLargeOfficeNew2004_Chicago.idf"))
	index := buildEnergyServicePathIndex(inputPath)
	if len(index.auxiliaryPaths) == 0 {
		t.Fatalf("production ServiceModel yielded no auxiliary path topology for %s", inputPath)
	}

	for _, test := range []struct {
		endUse      string
		selectPaths func([]energyPathAuxiliaryServicePath) []energyPathAuxiliaryServicePath
	}{
		{endUse: "fans", selectPaths: epath101AuditResolvableAirLoopPaths},
		{endUse: "pumps", selectPaths: epath101AuditResolvablePlantLoopPaths},
		{endUse: "heat_rejection", selectPaths: epath101AuditResolvableCondenserLoopPaths},
	} {
		t.Run(test.endUse, func(t *testing.T) {
			paths := test.selectPaths(index.auxiliaryPaths)
			if len(paths) == 0 {
				t.Fatalf("production topology has no unambiguous %s service-path group", test.endUse)
			}
			nodes := []EnergyExplanationNode{epath101AuditAuxiliary(test.endUse, 100, "meter."+test.endUse, nil)}
			pathSet := map[string]bool{}
			for pathIndex, path := range paths {
				pathSet[path.ID] = true
				nodes = append(nodes, epath101AuditLoad(path.ZoneName, path.ServiceKind, float64(pathIndex+1), "load.production."+metricID(path.ZoneName)+"."+metricID(path.ServiceKind), []string{path.ID}))
			}
			if len(nodes[0].RelatedPathIDs) != 0 {
				t.Fatal("test accidentally seeded broad auxiliary related paths")
			}
			topology := index
			topology.auxiliaryPaths = append([]energyPathAuxiliaryServicePath(nil), paths...)
			plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "annual", "annual", false)
			if !index.auxiliaryResolvable[test.endUse] {
				if len(plan.Edges) != 0 {
					t.Errorf("unresolved production %s inventory was allocated: %#v", test.endUse, plan.Edges)
				}
				record := epath101AuditRecord(plan.Records, test.endUse)
				if record == nil || record.ExpectedValue != 100 || record.AllocatedValue != 0 || record.UnassignedValue != 100 || record.Method != "unassigned" {
					t.Errorf("unresolved production %s coverage = %#v", test.endUse, record)
				}
				return
			}
			if len(plan.Edges) == 0 || epath101AuditAllocationTotal(plan.Edges) != 100 {
				t.Fatalf("broad %s did not resolve through production topology: records=%#v edges=%#v", test.endUse, plan.Records, plan.Edges)
			}
			for _, edge := range plan.Edges {
				for _, pathID := range edge.RelatedPathIDs {
					if !pathSet[pathID] {
						t.Errorf("%s allocation escaped its resolvable production loop: %#v", test.endUse, edge)
					}
				}
			}
		})
	}
}

func TestEPATH101AuditEnergyPathPlanAddsNoHeavyAuxiliaryOutputs(t *testing.T) {
	doc := parsePurposePlanFixture(t, purposePlanFixtureIDF)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
		Purposes:          []SimulationPurposeID{SimulationPurposeBasicEnergy},
		BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath,
	})
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(output.ObjectType, "Output:Variable") {
			continue
		}
		name := normalizeEnergyOutputName(output.VariableName)
		if strings.Contains(name, "fan electricity") || strings.Contains(name, "pump electricity") ||
			(strings.Contains(name, "system node") && (strings.Contains(name, "mass flow") || strings.Contains(name, "volume flow"))) ||
			strings.Contains(name, "supply air volume") {
			t.Errorf("Energy Path added a heavy component/airflow output for auxiliary allocation: %#v", output)
		}
	}
}

func TestEPATH101AuditAuxiliaryAllocationKeepsCarrierSourceAndPathIsolation(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("main.office", "Office", "cooling", "Main Air", "", ""),
		epath101AuditPath("main.lab", "Lab", "cooling", "Main Air", "", ""),
		epath101AuditPath("other.rogue", "Rogue", "cooling", "Other Air", "", ""),
	}
	nodes := []EnergyExplanationNode{
		epath101AuditCarrier(100),
		epath101AuditCarrierFor("natural_gas", 900, "meter.facility.gas"),
		epath101AuditAuxiliary("fans", 100, "meter.fans.electricity", []string{"main.office", "main.lab"}),
		epath101AuditAuxiliaryForCarrier("fans", "natural_gas", 900, "meter.fans.gas", []string{"other.rogue"}),
		epath101AuditLoad("Office", "cooling", 40, "load.office", []string{"main.office"}),
		epath101AuditLoad("Lab", "cooling", 60, "load.lab", []string{"main.lab"}),
		epath101AuditLoad("Rogue", "cooling", 900, "load.rogue", []string{"other.rogue"}),
	}
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "annual", "annual", false)
	for _, want := range []struct {
		zone    string
		value   float64
		factor  float64
		load    string
		path    string
		sibling string
	}{
		{zone: "Office", value: 40, factor: .4, load: "load.office", path: "main.office", sibling: "load.lab"},
		{zone: "Lab", value: 60, factor: .6, load: "load.lab", path: "main.lab", sibling: "load.office"},
	} {
		edge := epath101AuditAllocationEdge(plan.Edges, want.zone)
		if edge == nil || edge.Value != want.value || edge.Basis != "service_path_allocation" || edge.RuleID != energyRelationshipRuleAllocatedAuxiliaryServicePath ||
			!stringSliceContains(edge.SourceIDs, "meter.fans.electricity") || !stringSliceContains(edge.SourceIDs, want.load) || stringSliceContains(edge.SourceIDs, want.sibling) || stringSliceContains(edge.SourceIDs, "load.rogue") ||
			stringSliceContains(edge.SourceIDs, "meter.fans.gas") ||
			!reflect.DeepEqual(edge.RelatedPathIDs, []string{want.path}) || !strings.Contains(edge.Formula, fmt.Sprintf("allocation factor %.6f", want.factor)) {
			t.Errorf("%s isolated %.0f%% fan allocation = %#v", want.zone, want.factor*100, edge)
		}
	}
	rogue := epath101AuditAllocationEdge(plan.Edges, "Rogue")
	if rogue == nil || rogue.FromID != "energy.end_use.fans.natural_gas" || rogue.Value != 900 || !stringSliceContains(rogue.SourceIDs, "meter.fans.gas") ||
		stringSliceContains(rogue.SourceIDs, "meter.fans.electricity") || !reflect.DeepEqual(rogue.RelatedPathIDs, []string{"other.rogue"}) {
		t.Errorf("carrier-qualified Rogue fan allocation = %#v", rogue)
	}

	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:            nodes,
		Edges: []EnergyExplanationEdge{
			{ID: "facility-fans-electricity", FromID: "energy.carrier.electricity", ToID: "energy.end_use.fans.electricity", Value: 100, Unit: "kWh site", Relation: "meter_enduse", SourceIDs: []string{"meter.fans.electricity"}},
			{ID: "facility-fans-gas", FromID: "energy.carrier.natural_gas", ToID: "energy.end_use.fans.natural_gas", Value: 900, Unit: "kWh site", Relation: "meter_enduse", SourceIDs: []string{"meter.fans.gas"}},
		},
		Sources: []EnergyDataSource{
			{ID: "meter.fans.electricity", SourceType: "sql_meter", IsMeter: true},
			{ID: "meter.fans.gas", SourceType: "sql_meter", IsMeter: true},
			{ID: "load.office", SourceType: "sql_variable", ZoneName: "Office"},
			{ID: "load.lab", SourceType: "sql_variable", ZoneName: "Lab"},
			{ID: "load.rogue", SourceType: "sql_variable", ZoneName: "Rogue"},
		},
		servicePathIndex: epath101AuditTopology(paths),
	})
	electricity := energyExplanationSourceByID(result.Sources, "meter.fans.electricity")
	epath101AuditAssertSourceScope(t, electricity, "Office", .4, 40)
	epath101AuditAssertSourceScope(t, electricity, "Lab", .6, 60)
	if detail := epath101AuditSourceScope(energyExplanationSourceByID(result.Sources, "meter.fans.gas"), "Office"); detail != nil && detail.AllocationApplied && math.Abs(detail.AllocatedValue) > energyPathZoneHVACAllocationEpsilon {
		t.Errorf("natural-gas fan source leaked into Office electricity allocation: %#v", detail)
	}
}

func TestEPATH101AuditDistinctSameCarrierFanMetersStayWithinOwnAirLoops(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("air_a.office", "Office", "cooling", "Air A", "", ""),
		epath101AuditPath("air_a.lab", "Lab", "cooling", "Air A", "", ""),
		epath101AuditPath("air_b.studio", "Studio", "cooling", "Air B", "", ""),
		epath101AuditPath("air_b.storage", "Storage", "cooling", "Air B", "", ""),
	}
	fanA := epath101AuditAuxiliary("fans", 100, "meter.fans.air_a", []string{"air_a.office", "air_a.lab"})
	fanA.ID += ".air_a"
	fanB := epath101AuditAuxiliary("fans", 200, "meter.fans.air_b", []string{"air_b.studio", "air_b.storage"})
	fanB.ID += ".air_b"
	nodes := []EnergyExplanationNode{
		fanA,
		fanB,
		epath101AuditLoad("Office", "cooling", 75, "load.air_a.office", []string{"air_a.office"}),
		epath101AuditLoad("Lab", "cooling", 25, "load.air_a.lab", []string{"air_a.lab"}),
		epath101AuditLoad("Studio", "cooling", 40, "load.air_b.studio", []string{"air_b.studio"}),
		epath101AuditLoad("Storage", "cooling", 60, "load.air_b.storage", []string{"air_b.storage"}),
	}
	plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, epath101AuditTopology(paths), "annual", "annual", false)
	wants := []struct {
		fromID         string
		zone           string
		value          float64
		meterSource    string
		loadSource     string
		pathID         string
		forbiddenMeter string
		forbiddenPath  string
	}{
		{fanA.ID, "Office", 75, "meter.fans.air_a", "load.air_a.office", "air_a.office", "meter.fans.air_b", "air_b.studio"},
		{fanA.ID, "Lab", 25, "meter.fans.air_a", "load.air_a.lab", "air_a.lab", "meter.fans.air_b", "air_b.storage"},
		{fanB.ID, "Studio", 80, "meter.fans.air_b", "load.air_b.studio", "air_b.studio", "meter.fans.air_a", "air_a.office"},
		{fanB.ID, "Storage", 120, "meter.fans.air_b", "load.air_b.storage", "air_b.storage", "meter.fans.air_a", "air_a.lab"},
	}
	seen := map[string]bool{}
	for _, want := range wants {
		var edge *EnergyExplanationEdge
		for index := range plan.Edges {
			if plan.Edges[index].FromID == want.fromID && strings.EqualFold(plan.Edges[index].ZoneName, want.zone) {
				edge = &plan.Edges[index]
				break
			}
		}
		if edge == nil || edge.Value != want.value || !stringSliceContains(edge.SourceIDs, want.meterSource) || !stringSliceContains(edge.SourceIDs, want.loadSource) ||
			stringSliceContains(edge.SourceIDs, want.forbiddenMeter) || !reflect.DeepEqual(edge.RelatedPathIDs, []string{want.pathID}) || stringSliceContains(edge.RelatedPathIDs, want.forbiddenPath) {
			t.Errorf("same-carrier source-local allocation %s -> %s = %#v, want %.6g on %s only", want.fromID, want.zone, edge, want.value, want.pathID)
		}
		seen[want.fromID+"|"+strings.ToLower(want.zone)] = edge != nil
	}
	for _, edge := range plan.Edges {
		key := edge.FromID + "|" + strings.ToLower(edge.ZoneName)
		if !seen[key] {
			t.Errorf("same-carrier fan meter crossed into another AirLoop target: %#v", edge)
		}
	}
	for _, want := range []struct {
		fromID string
		total  float64
	}{{fanA.ID, 100}, {fanB.ID, 200}} {
		total := 0.0
		for _, edge := range plan.Edges {
			if edge.FromID == want.fromID {
				total = roundedEnergyNumber(total + edge.Value)
			}
		}
		if total != want.total {
			t.Errorf("%s allocation total = %.6g, want its own meter total %.6g", want.fromID, total, want.total)
		}
	}
}

func TestEPATH101AuditAnnualAuxiliaryFormulaRecomputesMonthlyAggregateFactor(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("main.office", "Office", "cooling", "Main Air", "", ""),
		epath101AuditPath("main.lab", "Lab", "cooling", "Main Air", "", ""),
	}
	graph := func(period string, fan, office, lab float64) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
		nodes := []EnergyExplanationNode{
			epath101AuditCarrier(fan),
			epath101AuditAuxiliary("fans", fan, "meter.fans", epath101AuditPathIDs(paths)),
			epath101AuditLoad("Office", "cooling", office, "load.office."+period, []string{"main.office"}),
			epath101AuditLoad("Lab", "cooling", lab, "load.lab."+period, []string{"main.lab"}),
		}
		for index := range nodes {
			nodes[index].Period = period
		}
		return nodes, []EnergyExplanationEdge{{ID: "facility-fans." + period, FromID: "energy.carrier.electricity", ToID: "energy.end_use.fans.electricity", Value: fan, Unit: "kWh site", Period: period, Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.fans"}}}
	}
	annualNodes, annualEdges := graph("annual", 200, 100, 100)
	m1Nodes, m1Edges := graph("M1", 100, 90, 10)
	m2Nodes, m2Edges := graph("M2", 100, 10, 90)
	topology := epath101AuditTopology(paths)
	m1Plan := buildEnergyPathZoneAuxiliaryAllocationPlan(m1Nodes, nil, topology, "M1", "monthly", true)
	m2Plan := buildEnergyPathZoneAuxiliaryAllocationPlan(m2Nodes, nil, topology, "M2", "monthly", true)
	annualPlan := aggregateEnergyPathZoneAuxiliaryAllocationPlans([]energyPathZoneAuxiliaryAllocationPlan{m1Plan, m2Plan})
	annualEdge := epath101AuditAllocationEdge(annualPlan.Edges, "Office")
	if annualEdge == nil || annualEdge.Value != 100 || !strings.Contains(annualEdge.Formula, "allocation factor 0.500000") ||
		strings.Contains(annualEdge.Formula, "allocation factor 0.900000") || strings.Contains(annualEdge.Formula, "allocation factor 0.100000") {
		t.Errorf("annual auxiliary edge retained a stale monthly factor: %#v", annualEdge)
	}

	input := EnergyExplanationV1{
		Schema:                energyExplanationV1Schema,
		AllocationPolicy:      PurposeAllocationPolicyByServicePathLoadShare,
		Nodes:                 annualNodes,
		Edges:                 annualEdges,
		Periods:               []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: m1Nodes, Edges: m1Edges}, {ID: "M2", Kind: "monthly", Nodes: m2Nodes, Edges: m2Edges}},
		Sources:               []EnergyDataSource{{ID: "meter.fans", SourceType: "sql_meter", IsMeter: true}},
		canonicalMonthlyBasis: true,
		servicePathIndex:      topology,
		scope:                 EnergyExplanationScope{Kind: "zone", ZoneName: "Office", AggregationBasis: "model_total"},
	}
	result := UpgradeEnergyExplanationV1(input)
	source := energyExplanationSourceByID(result.Sources, "meter.fans")
	if source == nil || !source.AllocationApplied || source.AllocationFactor != .5 || source.AllocatedValue != 100 ||
		!strings.Contains(source.AllocationFormula, "allocation factor 0.500000") || strings.Contains(source.AllocationFormula, "allocation factor 0.900000") || strings.Contains(source.AllocationFormula, "allocation factor 0.100000") {
		t.Errorf("annual source inspector formula disagrees with AllocationFactor: %#v", source)
	}
}

func TestEPATH101AuditMalformedAuxiliaryDeliveredLoadNeverCreatesConversion(t *testing.T) {
	paths := []energyPathAuxiliaryServicePath{epath101AuditPath("main.office", "Office", "cooling", "Main Air", "", "")}
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema:           energyExplanationV1Schema,
		AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes: []EnergyExplanationNode{
			epath101AuditCarrier(10),
			epath101AuditAuxiliary("fans", 10, "meter.fans", []string{"main.office"}),
			epath101AuditLoad("Office", "cooling", 100, "load.office", []string{"main.office"}),
		},
		Edges: []EnergyExplanationEdge{
			{ID: "facility-fans", FromID: "energy.carrier.electricity", ToID: "energy.end_use.fans.electricity", Value: 10, Unit: "kWh site", Relation: "meter_enduse", SourceIDs: []string{"meter.fans"}},
			{ID: "malformed", FromID: "energy.end_use.fans.electricity", ToID: "load.cooling.office", Value: 100, Unit: "kWh thermal", Relation: "delivered_load", ServiceKind: "cooling", SourceIDs: []string{"poison.delivered"}},
		},
		servicePathIndex: epath101AuditTopology(paths),
	})
	zone := epath101AuditZoneResult(result.ZoneResults, "Office")
	if zone == nil {
		t.Fatalf("missing Office result: %#v", result.AvailableZones)
	}
	for _, link := range zone.Links {
		if link.Relation == "load_to_end_use" && strings.Contains(link.ToID, "end_use.fans") {
			t.Errorf("malformed/staging auxiliary edge became a delivered-load conversion: %#v", link)
		}
		if link.Relation == "auxiliary_allocation" {
			t.Errorf("private auxiliary evidence leaked into the public Sankey: %#v", link)
		}
	}
	fans := epath101AuditNode(zone.Nodes, "end_use.fans.office")
	carrier := epath101AuditNode(zone.Nodes, "carrier.electricity.office")
	link := epath101AuditLink(zone.Links, "end_use.fans.office", "carrier.electricity.office")
	if fans == nil || fans.Value != 10 || carrier == nil || carrier.Value != 10 || link == nil || link.Relation != "direct_end_use_to_carrier" || link.Basis != "service_path_allocation" {
		t.Errorf("auxiliary lower-lane branch was not preserved: fans=%#v carrier=%#v link=%#v", fans, carrier, link)
	}
}

func epath101AuditPath(id, zone, service, air, plant, condenser string) energyPathAuxiliaryServicePath {
	return energyPathAuxiliaryServicePath{ID: id, ZoneName: zone, ServiceKind: service, AirLoopName: air, PlantLoopName: plant, CondenserLoopName: condenser}
}

func epath101AuditTopology(paths []energyPathAuxiliaryServicePath) energyServicePathIndex {
	return energyServicePathIndex{
		auxiliaryPaths: append([]energyPathAuxiliaryServicePath(nil), paths...),
		auxiliaryResolvable: map[string]bool{
			"fans":           true,
			"pumps":          true,
			"heat_rejection": true,
		},
	}
}

func epath101AuditPathIDs(paths []energyPathAuxiliaryServicePath) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		out = append(out, path.ID)
	}
	return out
}

func epath101AuditCarrier(value float64) EnergyExplanationNode {
	return epath101AuditCarrierFor("electricity", value, "meter.facility")
}

func epath101AuditCarrierFor(carrier string, value float64, sourceID string) EnergyExplanationNode {
	return EnergyExplanationNode{ID: "energy.carrier." + carrier, Level: "energy", Kind: "energy.facility", Label: carrier, Value: value, Unit: "kWh site", Carrier: carrier, EndUse: "total", SourceIDs: []string{sourceID}}
}

func epath101AuditAuxiliary(endUse string, value float64, sourceID string, pathIDs []string) EnergyExplanationNode {
	return epath101AuditAuxiliaryForCarrier(endUse, "electricity", value, sourceID, pathIDs)
}

func epath101AuditAuxiliaryForCarrier(endUse, carrier string, value float64, sourceID string, pathIDs []string) EnergyExplanationNode {
	return EnergyExplanationNode{
		ID: "energy.end_use." + endUse + "." + carrier, Level: "energy", Kind: "energy." + endUse, Label: endUse,
		Value: value, Unit: "kWh site", Carrier: carrier, EndUse: endUse, Basis: "measured_meter",
		SourceIDs: []string{sourceID}, RelatedPathIDs: append([]string(nil), pathIDs...),
	}
}

func epath101AuditDirectAuxiliary(zone, endUse string, value float64, sourceID string, pathIDs []string) EnergyExplanationNode {
	return EnergyExplanationNode{
		ID: "energy.end_use." + endUse + ".electricity." + metricID(zone), Level: "energy", Kind: "energy." + endUse, Label: endUse,
		Value: value, Unit: "kWh site", ZoneName: zone, Carrier: "electricity", EndUse: endUse, Basis: "direct_zone_energy",
		SourceIDs: []string{sourceID}, RelatedPathIDs: append([]string(nil), pathIDs...),
	}
}

func epath101AuditLoad(zone, service string, value float64, sourceID string, pathIDs []string) EnergyExplanationNode {
	return EnergyExplanationNode{
		ID: "load." + service + "." + metricID(zone), Level: "load", Kind: "load.zone_" + service, Label: service + " load",
		Value: value, Unit: "kWh thermal", ZoneName: zone, ServiceKind: service, PathType: "zone", Basis: "measured_energy_variable",
		SourceIDs: []string{sourceID}, RelatedPathIDs: append([]string(nil), pathIDs...),
	}
}

func epath101AuditAirflow(zone string, value float64, sourceID string, pathIDs []string) EnergyExplanationNode {
	return EnergyExplanationNode{
		ID: "airflow.supply_air_volume." + metricID(zone), Level: "airflow", Kind: "airflow.supply_air_volume", Label: "Supply air volume",
		Value: value, Unit: "m3", ZoneName: zone, Basis: "measured_variable", SourceIDs: []string{sourceID}, RelatedPathIDs: append([]string(nil), pathIDs...),
	}
}

func epath101AuditRecord(records []energyPathZoneAuxiliaryAllocationRecord, endUse string) *energyPathZoneAuxiliaryAllocationRecord {
	for index := range records {
		if strings.EqualFold(records[index].EndUse, endUse) && strings.EqualFold(records[index].Carrier, "electricity") {
			return &records[index]
		}
	}
	return nil
}

func epath101AuditAssertRecord(t *testing.T, records []energyPathZoneAuxiliaryAllocationRecord, endUse string, expected, direct, allocated, unassigned float64) {
	t.Helper()
	record := epath101AuditRecord(records, endUse)
	if record == nil || record.ExpectedValue != expected || record.DirectValue != direct || record.AllocatedValue != allocated || record.UnassignedValue != unassigned || record.OvermappedValue != 0 {
		t.Errorf("%s allocation record = %#v, want expected/direct/allocated/unassigned %.6g/%.6g/%.6g/%.6g", endUse, record, expected, direct, allocated, unassigned)
	}
}

func epath101AuditAllocationEdge(edges []EnergyExplanationEdge, zone string) *EnergyExplanationEdge {
	for index := range edges {
		if strings.EqualFold(edges[index].Relation, "auxiliary_allocation") && strings.EqualFold(edges[index].ZoneName, zone) {
			return &edges[index]
		}
	}
	return nil
}

func epath101AuditAssertZoneAllocation(t *testing.T, edges []EnergyExplanationEdge, zone string, want float64, sourceID, meterID string, wantPaths []string) {
	t.Helper()
	edge := epath101AuditAllocationEdge(edges, zone)
	if edge == nil || edge.Value != want || edge.Basis != "service_path_allocation" || edge.RuleID != energyRelationshipRuleAllocatedAuxiliaryServicePath ||
		!stringSliceContains(edge.SourceIDs, sourceID) || !stringSliceContains(edge.SourceIDs, meterID) {
		t.Errorf("%s auxiliary allocation = %#v, want %.6g with sources %q/%q", zone, edge, want, sourceID, meterID)
		return
	}
	gotPaths := append([]string(nil), edge.RelatedPathIDs...)
	wantPaths = append([]string(nil), wantPaths...)
	sort.Strings(gotPaths)
	sort.Strings(wantPaths)
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Errorf("%s auxiliary path provenance = %#v, want %#v", zone, gotPaths, wantPaths)
	}
}

func epath101AuditAllocationTotal(edges []EnergyExplanationEdge) float64 {
	total := 0.0
	for _, edge := range edges {
		if strings.EqualFold(edge.Relation, "auxiliary_allocation") {
			total = roundedEnergyNumber(total + edge.Value)
		}
	}
	return total
}

func epath101AuditNode(nodes []EnergyExplanationNode, id string) *EnergyExplanationNode {
	for index := range nodes {
		if nodes[index].ID == id {
			return &nodes[index]
		}
	}
	return nil
}

func epath101AuditLink(links []EnergyPathLink, fromID, toID string) *EnergyPathLink {
	for index := range links {
		if links[index].FromID == fromID && links[index].ToID == toID {
			return &links[index]
		}
	}
	return nil
}

func epath101AuditZoneResult(results []EnergyExplanationZoneResult, zone string) *EnergyExplanationZoneResult {
	for index := range results {
		if strings.EqualFold(results[index].Scope.ZoneName, zone) {
			return &results[index]
		}
	}
	return nil
}

func epath101AuditPeriod(periods []EnergyPeriod, id string) *EnergyPeriod {
	for index := range periods {
		if strings.EqualFold(periods[index].ID, id) {
			return &periods[index]
		}
	}
	return nil
}

func epath101AuditMonthlyNodeSum(periods []EnergyPeriod, nodeID string) float64 {
	total := 0.0
	for _, period := range periods {
		if !strings.EqualFold(period.Kind, "monthly") {
			continue
		}
		if node := epath101AuditNode(period.Nodes, nodeID); node != nil {
			total = roundedEnergyNumber(total + node.Value)
		}
	}
	return total
}

func epath101AuditAuxiliaryReconciliation(rows []EnergyReconciliation, endUse, carrier, period string) *EnergyReconciliation {
	want := "reconcile.zone_auxiliary_allocation." + canonicalEnergyPathPart(endUse) + "." + canonicalEnergyPathPart(carrier) + "." + canonicalEnergyPathPart(period)
	for index := range rows {
		if rows[index].ID == want {
			return &rows[index]
		}
	}
	return nil
}

func epath101AuditHasWarning(warnings []EnergyWarning, code string) bool {
	for _, warning := range warnings {
		if strings.EqualFold(warning.Code, code) {
			return true
		}
	}
	return false
}

func epath101AuditSourceScope(source *EnergyDataSource, zone string) *EnergyDataSourceScopeDetail {
	if source == nil {
		return nil
	}
	for index := range source.ScopeDetails {
		if strings.EqualFold(source.ScopeDetails[index].Scope.Kind, "zone") && strings.EqualFold(source.ScopeDetails[index].Scope.ZoneName, zone) {
			return &source.ScopeDetails[index]
		}
	}
	return nil
}

func epath101AuditAssertSourceScope(t *testing.T, source *EnergyDataSource, zone string, factor, value float64) {
	t.Helper()
	detail := epath101AuditSourceScope(source, zone)
	if detail == nil || !detail.AllocationApplied || math.Abs(detail.AllocationFactor-factor) > energyPathZoneHVACAllocationEpsilon || math.Abs(detail.AllocatedValue-value) > energyPathZoneHVACAllocationEpsilon || detail.AggregationBasis != "model_total" {
		t.Errorf("%s source scope = %#v, want factor/value %.6g/%.6g model_total", zone, detail, factor, value)
	}
}

func epath101AuditResolvableAirLoopPaths(paths []energyPathAuxiliaryServicePath) []energyPathAuxiliaryServicePath {
	return epath101AuditPathsFromFirstLoopGroup(paths, func(path energyPathAuxiliaryServicePath) string {
		return strings.TrimSpace(path.AirLoopName)
	}, false)
}

func epath101AuditResolvablePlantLoopPaths(paths []energyPathAuxiliaryServicePath) []energyPathAuxiliaryServicePath {
	return epath101AuditPathsFromFirstLoopGroup(paths, func(path energyPathAuxiliaryServicePath) string {
		return strings.TrimSpace(path.PlantLoopName)
	}, true)
}

func epath101AuditResolvableCondenserLoopPaths(paths []energyPathAuxiliaryServicePath) []energyPathAuxiliaryServicePath {
	groups := map[string][]energyPathAuxiliaryServicePath{}
	order := []string{}
	for _, path := range paths {
		if energyCanonicalServiceKind(path.ServiceKind) != "cooling" || strings.TrimSpace(path.PlantLoopName) == "" || strings.TrimSpace(path.CondenserLoopName) == "" {
			continue
		}
		key := normalizePurposeToken(path.PlantLoopName) + "|" + normalizePurposeToken(path.CondenserLoopName)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], path)
	}
	sort.Strings(order)
	for _, key := range order {
		if selected := epath101AuditDistinctZoneServicePaths(groups[key]); len(selected) > 0 {
			return selected
		}
	}
	return nil
}

func epath101AuditPathsFromFirstLoopGroup(paths []energyPathAuxiliaryServicePath, loopKey func(energyPathAuxiliaryServicePath) string, requireSingleService bool) []energyPathAuxiliaryServicePath {
	groups := map[string][]energyPathAuxiliaryServicePath{}
	services := map[string]map[string]bool{}
	order := []string{}
	for _, path := range paths {
		loop := normalizePurposeToken(loopKey(path))
		service := energyCanonicalServiceKind(path.ServiceKind)
		if loop == "" || (service != "cooling" && service != "heating") {
			continue
		}
		if _, ok := groups[loop]; !ok {
			order = append(order, loop)
			services[loop] = map[string]bool{}
		}
		groups[loop] = append(groups[loop], path)
		services[loop][service] = true
	}
	sort.Strings(order)
	for _, loop := range order {
		if requireSingleService && len(services[loop]) != 1 {
			continue
		}
		if selected := epath101AuditDistinctZoneServicePaths(groups[loop]); len(selected) > 0 {
			return selected
		}
	}
	return nil
}

func epath101AuditDistinctZoneServicePaths(paths []energyPathAuxiliaryServicePath) []energyPathAuxiliaryServicePath {
	seen := map[string]bool{}
	out := []energyPathAuxiliaryServicePath{}
	sort.SliceStable(paths, func(i, j int) bool { return paths[i].ID < paths[j].ID })
	for _, path := range paths {
		key := strings.ToLower(strings.TrimSpace(path.ZoneName)) + "|" + energyCanonicalServiceKind(path.ServiceKind)
		if strings.TrimSpace(path.ZoneName) == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, path)
		if len(out) == 3 {
			break
		}
	}
	return out
}
