package simulation

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Projection-only boundary test. Native physical ownership and SQL observations
// are independently exercised by the WindowAC reader tests. A legacy AirLoop
// pool does not prove nonoverlap with another direct observation: retain the
// native local fan and an explicit unassigned remainder, never an invented
// central share or a second copy of the broad meter.
func TestEnergyPathNativeFanCoexistsWithUnresolvedSharedPool(t *testing.T) {
	const zoneA, zoneB = "Office", "Lab"
	paths := []energyPathAuxiliaryServicePath{
		epath101AuditPath("central.office", zoneA, "cooling", "Main Air", "", ""),
		epath101AuditPath("central.lab", zoneB, "cooling", "Main Air", "", ""),
	}
	topology := epath101AuditTopology(paths)
	topology.nativeWindowACPaths = map[string][]string{
		energyPathNativeWindowACPathKey(zoneA, "Local Fan", "Fan Electricity Energy"): {"window.ac.office"},
	}
	makeGraph := func(factor float64) ([]EnergyExplanationNode, []EnergyExplanationEdge) {
		nodes := []EnergyExplanationNode{
			epath101AuditCarrier(10 * factor),
			epath101AuditAuxiliary("fans", 10*factor, "meter.fans", epath101AuditPathIDs(paths)),
			epath101AuditLoad(zoneA, "cooling", 10*factor, "load.office", []string{"central.office", "window.ac.office"}),
			epath101AuditLoad(zoneB, "cooling", 20*factor, "load.lab", []string{"central.lab"}),
		}
		edges := []EnergyExplanationEdge{{ID: "facility.fans", FromID: nodes[0].ID, ToID: nodes[1].ID, Value: 10 * factor, Unit: "kWh site", Relation: "meter_enduse", Basis: "measured_meter", SourceIDs: []string{"meter.fans"}, RelatedPathIDs: []string{"central.office", "central.lab"}}}
		return nodes, edges
	}
	nodes, edges := makeGraph(3)
	m1, e1 := makeGraph(1)
	m2, e2 := makeGraph(2)
	direct := energyExplanationSeries{Stage: "end_use", CanonicalKind: "energy.fans", Level: "energy", Kind: "energy.fans", Label: "Native local fan", Unit: "kWh", Carrier: "electricity", EndUse: "fans", ZoneName: zoneA, Basis: "direct_zone_energy", MeterHierarchyLevel: "zone_direct_use", SourceIDs: []string{"native.fan"}, AnnualSourceIDs: []string{"native.fan"}, MonthlySourceIDs: []string{"native.fan"}, Total: 9, Monthly: map[int]float64{1: 3, 2: 6}, directComponentID: "fans.zone_equipment.electricity", sourceName: "Fan Electricity Energy", sourceKeyValue: "Local Fan", sourceFrequency: "Monthly"}
	source := EnergyDataSource{ID: "native.fan", SourceType: "sql_report_data", Name: "Fan Electricity Energy", KeyValue: "Local Fan", ZoneName: zoneA, ReportingFrequency: "Monthly", Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", RawValue: 9, EffectiveValue: 9, EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", observedValuePresence: energySourceObservedRaw | energySourceObservedEffective}
	pool := energyPathFanPool{Source: EnergyDataSource{ID: "shared.pool", SourceType: "sql_report_data", Name: "Air System Fan Electricity Energy", KeyValue: "Main Air", ReportingFrequency: "Hourly", SourceUnit: "J", NormalizedUnit: "kWh"}, AirLoopName: "Main Air", Monthly: map[int]float64{1: 7, 2: 14}}
	// Explicit observed zeros complete the source's annual observation. The
	// projection fixture intentionally exposes only its two nonzero periods.
	for month := 3; month <= 12; month++ {
		pool.Monthly[month] = 0
	}
	result := UpgradeEnergyExplanationV1(EnergyExplanationV1{Schema: energyExplanationV1Schema, AllocationPolicy: PurposeAllocationPolicyByServicePathLoadShare, Nodes: nodes, Edges: edges, Periods: []EnergyPeriod{{ID: "M1", Kind: "monthly", Nodes: m1, Edges: e1}, {ID: "M2", Kind: "monthly", Nodes: m2, Edges: e2}}, Sources: []EnergyDataSource{source}, canonicalMonthlyBasis: true, zoneDirectUseSeries: []energyExplanationSeries{direct}, servicePathIndex: topology, auxiliaryFanPools: []energyPathFanPool{pool}})
	for pass := 0; pass < 3; pass++ {
		if pass > 0 {
			wire, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var decoded EnergyExplanationResult
			if err := json.Unmarshal(wire, &decoded); err != nil {
				t.Fatal(err)
			}
			result = decoded
		}
		building := epath101AuditNode(result.Nodes, "end_use.fans.building")
		if building == nil || building.Value != 30 {
			t.Fatalf("broad Building fan authority changed: %#v", building)
		}
		row := epath101AuditAuxiliaryReconciliation(result.Reconciliation, "fans", "electricity", "annual")
		if row == nil || row.ExpectedValue != 30 || row.DirectValue != 9 || row.AllocatedValue != 0 || row.UnassignedValue != 21 || row.OvermappedValue != 0 {
			t.Fatalf("unknown shared overlap was fabricated or direct energy lost: %#v", row)
		}
		office := epath101AuditZoneResult(result.ZoneResults, zoneA)
		lab := epath101AuditZoneResult(result.ZoneResults, zoneB)
		if office == nil || lab == nil {
			t.Fatal("missing precomputed Zone")
		}
		periods := append([]EnergyPeriod{{ID: "annual", Nodes: office.Nodes, Links: office.Links}}, office.Periods...)
		seenPeriods := map[string]bool{}
		for _, period := range periods {
			want, known := map[string]float64{"annual": 9, "M1": 3, "M2": 6}[period.ID]
			if !known || seenPeriods[period.ID] {
				t.Fatalf("unexpected/duplicate Office period %s", period.ID)
			}
			seenPeriods[period.ID] = true
			fan := epath101AuditNode(period.Nodes, "end_use.fans.office")
			if fan == nil || fan.Value != want || fan.Basis != "direct_zone_energy" {
				t.Fatalf("native local fan changed: %#v", fan)
			}
			count := 0
			for _, link := range period.Links {
				if link.ToID == fan.ID && link.Relation == "load_to_end_use" {
					t.Fatal("fan created thermal conversion")
				}
				if link.FromID != fan.ID || link.Relation != "direct_end_use_to_carrier" && link.Relation != "end_use_to_carrier" {
					continue
				}
				count++
				if link.FromValue != want || link.ToValue != want || link.Basis != "direct_zone_energy" || !reflect.DeepEqual(link.SourceIDs, []string{"native.fan"}) || !reflect.DeepEqual(link.RelatedPathIDs, []string{"window.ac.office"}) {
					t.Fatalf("native fan borrowed shared identity/path: %#v", link)
				}
			}
			if count != 1 {
				t.Fatalf("native fan carrier branches=%d", count)
			}
		}
		if len(seenPeriods) != 3 {
			t.Fatalf("missing Office periods: %v", seenPeriods)
		}
		for _, node := range lab.Nodes {
			if node.Level == "end_use" && node.EndUse == "fans" && node.Value > 0 {
				t.Fatal("unproven overlapping shared pool became Lab consumption")
			}
		}
		sharedSources := 0
		for _, item := range result.Sources {
			if item.ID != "shared.pool" {
				continue
			}
			sharedSources++
			if item.RawValue != 21 || item.EffectiveValue != 21 || !energyDataSourceValueKnown(item, energySourceObservedRaw) || !energyDataSourceValueKnown(item, energySourceObservedEffective) || item.AllocationApplied || item.AllocatedValue != 0 || len(item.ScopeDetails) != 0 {
				t.Fatalf("shared observation lost or fabricated Zone observation: %#v", item)
			}
		}
		if sharedSources != 1 {
			t.Fatalf("shared source census=%d", sharedSources)
		}
	}
}
