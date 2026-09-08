package simulation

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEPATH101FanCoilOriginalTypedPlantsAndLightweightRequests(t *testing.T) {
	doc, _, topology, report := epathFanCoilOriginal(t)
	counts := map[string]int{}
	for _, object := range doc.Objects {
		counts[strings.ToLower(object.Type)]++
	}
	for kind, want := range map[string]int{
		"zone": 3, "zonehvac:fourpipefancoil": 3, "fan:constantvolume": 3,
		"airloophvac": 0, "plantloop": 2, "pump:variablespeed": 2,
		"coil:cooling:water": 3, "coil:heating:water": 3,
		"districtcooling": 1, "districtheating:water": 1,
	} {
		if counts[kind] != want {
			t.Fatalf("original inventory %s=%d, want %d", kind, counts[kind], want)
		}
	}
	eligible := epathFanCoilAssertTypedPaths(t, topology)
	fanEligible := energyPathZoneAuxiliaryEligiblePaths("fans", nil, topology.auxiliaryPaths)
	if len(fanEligible) != 0 || topology.auxiliaryResolvable["fans"] {
		t.Fatal("three local fans without an AirLoop acquired central fan allocation authority")
	}
	if !topology.auxiliaryResolvable["pumps"] {
		t.Fatal("both original pumps must resolve to their distinct typed water loops")
	}
	localFans, pumps := map[string]bool{}, map[string]bool{}
	for _, item := range report.ServiceModel.Components {
		if item.Component.ObjectType == "ZoneHVAC:FourPipeFanCoil" {
			for _, ref := range item.InternalRefs {
				if ref.ObjectType == "Fan:ConstantVolume" {
					localFans[ref.ObjectName] = true
					if energyPathAuxiliaryComponentResolved("fans", item, fanEligible) {
						t.Fatal("local Fan Coil fan relabelled as an AirLoop fan")
					}
				}
			}
		}
		if item.Component.ObjectType == "Pump:VariableSpeed" {
			pumps[item.Component.ObjectName] = true
			if !energyPathAuxiliaryComponentResolved("pumps", item, eligible) {
				t.Fatalf("an original pump is unresolved: %#v", item)
			}
		}
	}
	if len(localFans) != 3 || !localFans["Zone1FanCoilFan"] || !localFans["Zone2FanCoilFan"] || !localFans["Zone3FanCoilFan"] ||
		len(pumps) != 2 || !pumps["ChW Circ Pump"] || !pumps["HW Circ Pump"] {
		t.Fatalf("original auxiliary inventory was dropped: fans=%v pumps=%v", localFans, pumps)
	}
	for _, scope := range []SimulationPurposeScope{{}, {ZoneMode: "selected", ZoneNames: []string{"West Zone"}}} {
		plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{
			Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath, Scope: scope,
		})
		meters := map[string]bool{}
		for _, output := range plan.OutputObjects {
			if strings.EqualFold(output.ObjectType, "Output:Meter") {
				meters[purposeFieldValue(output.Fields, "Key Name")] = true
			}
			if !strings.EqualFold(output.ObjectType, "Output:Variable") {
				continue
			}
			name := normalizeEnergyOutputName(output.VariableName)
			if strings.Contains(name, "fan electricity") || strings.Contains(name, "pump electricity") ||
				strings.Contains(name, "supply air volume") ||
				strings.Contains(name, "system node") && (strings.Contains(name, "mass flow") || strings.Contains(name, "volume flow")) {
				t.Fatalf("scope %s added a heavy auxiliary output: %#v", scope.ZoneMode, output)
			}
		}
		if !meters["Fans:Electricity"] || !meters["Pumps:Electricity"] {
			t.Fatalf("scope %s lost the broad auxiliary meters: %v", scope.ZoneMode, meters)
		}
	}
}

func TestEPATH101FanCoilBroadPumpAllocationKeepsFansUnassigned(t *testing.T) {
	_, _, topology, _ := epathFanCoilOriginal(t)
	epathFanCoilAssertTypedPaths(t, topology)
	nodes := epathFanCoilNodes(topology)
	for _, period := range []struct{ id, kind string }{{"annual", "annual"}, {"M1", "monthly"}} {
		plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, period.id, period.kind, false)
		epath101AuditAssertRecord(t, plan.Records, "fans", 30, 0, 0, 30)
		epath101AuditAssertRecord(t, plan.Records, "pumps", 100, 0, 100, 0)
		if len(plan.Records) != 2 || len(plan.Edges) != 3 || len(plan.FanSourceAllocations) != 0 {
			t.Fatalf("%s fabricated measured pools or extra allocation branches: %#v", period.id, plan)
		}
		if epath101AuditRecord(plan.Records, "fans").Method != "unassigned" || epath101AuditRecord(plan.Records, "pumps").Method != "plant_loop_load_share" {
			t.Fatalf("%s lost the direct/allocated/unassigned distinction: %#v", period.id, plan.Records)
		}
		for i, zone := range epathFanCoilZones() {
			edge := epath101AuditAllocationEdge(plan.Edges, zone)
			wantValue := []float64{35, 25, 40}[i] // 100 * (cooling + heating) / 200.
			paths := epathFanCoilZonePlantPaths(topology, zone)
			if edge == nil || edge.Value != wantValue || edge.Period != period.id || edge.ServiceKind != "hvac" ||
				edge.FromID != "energy.end_use.pumps.electricity" || edge.Basis != "service_path_allocation" ||
				edge.RuleID != energyRelationshipRuleAllocatedAuxiliaryServicePath || !strings.Contains(edge.Formula, "PlantLoop") {
				t.Fatalf("%s %s broad pump share is not an explicitly allocated combined-service value: %#v", period.id, zone, edge)
			}
			epathFanCoilAssertSet(t, edge.RelatedPathIDs, paths)
			epathFanCoilAssertSet(t, edge.SourceIDs, []string{"broad.pumps", "load.cooling." + zone, "load.heating." + zone})
		}
		epathFanCoilAssertSet(t, epath101AuditRecord(plan.Records, "fans").SourceIDs, []string{"broad.fans"})
	}
	// This is one observed building Pumps meter, not two invented individual
	// pump meters. No direct node or source-local pool is supplied to the test.
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{
		Schema: energyExplanationV1Schema, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare,
		Nodes: nodes, servicePathIndex: topology,
		Edges: []EnergyExplanationEdge{
			{ID: "facility-fans", FromID: "energy.carrier.electricity", ToID: "energy.end_use.fans.electricity", Value: 30, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"broad.fans"}},
			{ID: "facility-pumps", FromID: "energy.carrier.electricity", ToID: "energy.end_use.pumps.electricity", Value: 100, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"broad.pumps"}},
		},
		Sources: epathFanCoilSources(),
	})
	if len(result.ZoneResults) != 3 {
		t.Fatalf("lost original three Zone results: %v", result.AvailableZones)
	}
	for i, name := range epathFanCoilZones() {
		zone := epath101AuditZoneResult(result.ZoneResults, name)
		if zone == nil {
			t.Fatalf("missing Zone %s", name)
		}
		if fan := epath101AuditNode(zone.Nodes, "end_use.fans."+metricID(name)); fan != nil {
			t.Fatalf("unassigned local fan became a Zone observation: %#v", fan)
		}
		pump := epath101AuditNode(zone.Nodes, "end_use.pumps."+metricID(name))
		if pump == nil || pump.Value != []float64{35, 25, 40}[i] || pump.Basis != "service_path_allocation" || !pump.AllocationApplied {
			t.Fatalf("%s did not retain allocated (not direct) pump energy: %#v", name, pump)
		}
		epathFanCoilAssertSet(t, pump.SourceIDs, []string{"broad.pumps", "load.cooling." + name, "load.heating." + name})
		fanCoverage := epath101AuditAuxiliaryReconciliation(zone.Reconciliation, "fans", "electricity", "annual")
		pumpCoverage := epath101AuditAuxiliaryReconciliation(zone.Reconciliation, "pumps", "electricity", "annual")
		if fanCoverage == nil || fanCoverage.ExpectedValue != 30 || fanCoverage.DirectValue != 0 || fanCoverage.AllocatedValue != 0 || fanCoverage.UnassignedValue != 30 ||
			pumpCoverage == nil || pumpCoverage.ExpectedValue != 100 || pumpCoverage.DirectValue != 0 || pumpCoverage.AllocatedValue != 100 || pumpCoverage.UnassignedValue != 0 {
			t.Fatalf("%s lost model-wide allocation accounting: fan=%#v pump=%#v", name, fanCoverage, pumpCoverage)
		}
	}
	for _, source := range result.Sources {
		if source.IsMeter && source.ID != "broad.fans" && source.ID != "broad.pumps" && source.ID != "meter.facility" {
			t.Fatalf("invented an individual fan/pump meter: %#v", source)
		}
	}
}

func TestEPATH101FanCoilAmbiguousPlantCannotBeLaunderedByScope(t *testing.T) {
	_, _, original, report := epathFanCoilOriginal(t)
	epathFanCoilAssertTypedPaths(t, original)
	for _, unsupported := range []string{"heating", "service_water_heating", "ventilation", "unknown"} {
		t.Run(unsupported, func(t *testing.T) {
			topology := original
			topology.auxiliaryPaths = append([]energyPathAuxiliaryServicePath(nil), original.auxiliaryPaths...)
			// Genuine shared-loop evidence differs from the former Cartesian
			// Fan Coil bug: the same loop explicitly serves another service.
			for i := range topology.auxiliaryPaths {
				path := &topology.auxiliaryPaths[i]
				if path.PlantLoopName == "Hot Water Loop" {
					path.PlantLoopName = "Chilled Water Loop"
					path.ServiceKind = unsupported
				}
			}
			topology.auxiliaryResolvable = energyPathAuxiliaryResolvableInventory(report.ServiceModel.Components, topology.auxiliaryPaths)
			if topology.auxiliaryResolvable["pumps"] {
				t.Fatal("a genuinely mixed/unsupported plant was certified as resolved")
			}
			for _, restricted := range [][]string{nil, epathFanCoilZonePlantPaths(original, "West Zone")[:1]} {
				if len(energyPathZoneAuxiliaryEligiblePaths("pumps", restricted, topology.auxiliaryPaths)) != 0 {
					t.Fatalf("scope/path filter laundered %s ambiguity", unsupported)
				}
				nodes := epathFanCoilNodes(original)
				nodes[2].RelatedPathIDs = restricted
				plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "M1", "monthly", false)
				epath101AuditAssertRecord(t, plan.Records, "pumps", 100, 0, 0, 100)
				if len(plan.Edges) != 0 {
					t.Fatalf("ambiguous broad pump energy was allocated: %#v", plan.Edges)
				}
				epathFanCoilAssertSet(t, epath101AuditRecord(plan.Records, "pumps").SourceIDs, []string{"broad.pumps"})
			}
		})
	}
}

func TestEPATH101FanCoilEveryOriginalPumpMustResolve(t *testing.T) {
	_, _, original, report := epathFanCoilOriginal(t)
	eligible := epathFanCoilAssertTypedPaths(t, original)
	components := append([]idf.ComponentIndexItem(nil), report.ServiceModel.Components...)
	changed := 0
	for i := range components {
		if components[i].Component.ObjectType == "Pump:VariableSpeed" && components[i].Component.ObjectName == "HW Circ Pump" {
			// Keep the original pump in inventory, but with no established
			// ownership. The resolved CHW pump alone cannot license the pool.
			components[i].Occurrences = nil
			components[i].RelatedPathIDs = nil
			components[i].RelatedLoopRefs = nil
			if energyPathAuxiliaryComponentResolved("pumps", components[i], eligible) {
				t.Fatal("unbound original pump was treated as resolved")
			}
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("expected exact original HW pump once, got %d", changed)
	}
	topology := original
	topology.auxiliaryResolvable = energyPathAuxiliaryResolvableInventory(components, original.auxiliaryPaths)
	if topology.auxiliaryResolvable["pumps"] {
		t.Fatal("one resolved original pump hid another unresolved original pump")
	}
	nodes := epathFanCoilNodes(original)
	for _, scoped := range []bool{false, true} {
		if scoped {
			nodes[2].RelatedPathIDs = nil
			for _, path := range original.auxiliaryPaths {
				if path.PlantLoopName == "Chilled Water Loop" && path.ZoneName == "West Zone" {
					nodes[2].RelatedPathIDs = append(nodes[2].RelatedPathIDs, path.ID)
				}
			}
			if len(nodes[2].RelatedPathIDs) != 1 {
				t.Fatal("scope fixture did not identify the exact West cooling path")
			}
		}
		plan := buildEnergyPathZoneAuxiliaryAllocationPlan(nodes, nil, topology, "annual", "annual", false)
		epath101AuditAssertRecord(t, plan.Records, "pumps", 100, 0, 0, 100)
		if len(plan.Edges) != 0 || len(plan.FanSourceAllocations) != 0 {
			t.Fatalf("scope=%v hid an unresolved original pump: %#v", scoped, plan)
		}
	}
}

func epathFanCoilOriginal(t *testing.T) (idf.Document, string, energyServicePathIndex, idf.HVACReport) {
	t.Helper()
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "FanCoilAutoSize.idf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "d940e67912d5d4669773c11521b499ca671dae2d67ff8ce538c4a183ca0b43d3" {
		t.Fatalf("original Fan Coil bytes changed: %s", got)
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, after) {
			t.Error("read-only original model was changed")
		}
	})
	doc := parsePurposePlanFixture(t, string(data))
	return doc, path, buildEnergyServicePathIndex(path), idf.AnalyzeHVAC(doc)
}

func epathFanCoilZones() []string { return []string{"West Zone", "EAST ZONE", "NORTH ZONE"} }

func epathFanCoilAssertTypedPaths(t *testing.T, topology energyServicePathIndex) map[string]energyPathAuxiliaryServicePath {
	t.Helper()
	eligible := energyPathZoneAuxiliaryEligiblePaths("pumps", nil, topology.auxiliaryPaths)
	seen, ventilation := map[string]int{}, map[string]int{}
	for _, path := range topology.auxiliaryPaths {
		if path.AirLoopName != "" || path.CondenserLoopName != "" {
			t.Fatalf("local Fan Coil path acquired an unrelated loop: %#v", path)
		}
		if path.ServiceKind == "ventilation" {
			if path.PlantLoopName != "" {
				t.Fatalf("ventilation acquired a district plant: %#v", path)
			}
			ventilation[path.ZoneName]++
			continue
		}
		wantLoop := map[string]string{"cooling": "Chilled Water Loop", "heating": "Hot Water Loop"}[path.ServiceKind]
		if wantLoop == "" || path.PlantLoopName != wantLoop {
			t.Fatalf("crossed or plant-free water-coil service: %#v", path)
		}
		seen[path.ZoneName+"|"+path.ServiceKind]++
	}
	if len(eligible) != 6 || len(seen) != 6 || len(ventilation) != 3 {
		t.Fatalf("want exact 3 cooling + 3 heating + 3 local ventilation paths: eligible=%d seen=%v ventilation=%v", len(eligible), seen, ventilation)
	}
	for _, zone := range epathFanCoilZones() {
		if seen[zone+"|cooling"] != 1 || seen[zone+"|heating"] != 1 || ventilation[zone] != 1 {
			t.Fatalf("duplicate/missing original Zone service: %s %v %v", zone, seen, ventilation)
		}
	}
	return eligible
}

func epathFanCoilZonePlantPaths(topology energyServicePathIndex, zone string) []string {
	paths := []string{}
	for _, path := range topology.auxiliaryPaths {
		if path.ZoneName == zone && path.PlantLoopName != "" {
			paths = append(paths, path.ID)
		}
	}
	sort.Strings(paths)
	return paths
}

func epathFanCoilNodes(topology energyServicePathIndex) []EnergyExplanationNode {
	nodes := []EnergyExplanationNode{epath101AuditCarrier(130), epath101AuditAuxiliary("fans", 30, "broad.fans", nil), epath101AuditAuxiliary("pumps", 100, "broad.pumps", nil)}
	for i, zone := range epathFanCoilZones() {
		for _, service := range []string{"cooling", "heating"} {
			value := map[string][]float64{"cooling": {60, 30, 10}, "heating": {10, 20, 70}}[service][i]
			nodes = append(nodes, epath101AuditLoad(zone, service, value, "load."+service+"."+zone, topology.byZoneService[normalizePurposeToken(zone)+"|"+service]))
		}
	}
	return nodes
}

func epathFanCoilSources() []EnergyDataSource {
	sources := []EnergyDataSource{
		{ID: "meter.facility", Name: "Electricity:Facility", SourceType: "sql_meter", IsMeter: true},
		{ID: "broad.fans", Name: "Fans:Electricity", SourceType: "sql_meter", IsMeter: true},
		{ID: "broad.pumps", Name: "Pumps:Electricity", SourceType: "sql_meter", IsMeter: true},
	}
	for _, zone := range epathFanCoilZones() {
		for _, service := range []string{"cooling", "heating"} {
			sources = append(sources, EnergyDataSource{ID: "load." + service + "." + zone, SourceType: "sql_variable", ZoneName: zone})
		}
	}
	return sources
}

func epathFanCoilAssertSet(t *testing.T, actual, expected []string) {
	t.Helper()
	actual, expected = append([]string(nil), actual...), append([]string(nil), expected...)
	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("exact provenance mismatch: got %v want %v", actual, expected)
	}
}
