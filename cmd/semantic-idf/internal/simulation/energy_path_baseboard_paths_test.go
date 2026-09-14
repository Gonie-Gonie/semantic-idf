package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathNativeBaseboardPathsUseOriginalDeliveryIdentity(t *testing.T) {
	doc := energyPathBaseboardOriginal(t)
	before := doc.String()
	report := idf.AnalyzeHVAC(doc)
	got := buildEnergyPathNativeBaseboardPaths(doc, report.ServiceModel.ZoneServices)
	want := map[string][]string{}
	for _, owner := range []struct {
		zone  string
		index int
	}{{"SPACE2-1", 167}, {"SPACE4-1", 168}} {
		object := directHVACFixtureObject(t, &doc, energyPathBaseboardElectricType, owner.zone+" Baseboard")
		if object.Index != owner.index {
			t.Fatalf("original component index=%d, want %d; do not borrow prepared-copy component positions", object.Index, owner.index)
		}
		want[energyPathNativeBaseboardPathKey(owner.zone, owner.zone+" Baseboard")] = []string{fmt.Sprintf("service-path:zone:%s:heating:baseboard:component:%d", strings.ToLower(owner.zone), owner.index)}
	}
	if !reflect.DeepEqual(got, want) || doc.String() != before {
		t.Fatalf("original native delivery paths=%v, want %v; original unchanged=%t", got, want, doc.String() == before)
	}
}

func TestEnergyPathNativeBaseboardPathsRejectMissingAndAmbiguousTopology(t *testing.T) {
	for _, mutation := range []string{"missing", "duplicate path", "wrong type", "wrong name", "wrong index", "wrong component ID", "wrong Zone", "wrong served Zone", "wrong summary Zone", "wrong summary subject Zone", "Space path", "Space subject", "Space summary", "Space summary subject", "cooling", "central path", "air loop", "plant loop", "condenser loop", "refrigerant system"} {
		t.Run(mutation, func(t *testing.T) {
			doc := energyPathBaseboardHand(t)
			summaries := idf.AnalyzeHVAC(doc).ServiceModel.ZoneServices
			var path *idf.ZoneServicePath
			var owner *idf.ZoneServiceSummary
			for i := range summaries {
				for j := range summaries[i].Paths {
					if summaries[i].Paths[j].Delivery.ObjectName == "Baseboard" {
						if path != nil {
							t.Fatal("control has more than one exact Baseboard path")
						}
						path, owner = &summaries[i].Paths[j], &summaries[i]
					}
				}
			}
			if path == nil || len(buildEnergyPathNativeBaseboardPaths(doc, summaries)) != 1 {
				t.Fatal("control lacks a uniquely typed Baseboard delivery")
			}
			switch mutation {
			case "missing":
				owner.Paths = nil
			case "duplicate path":
				owner.Paths = append(owner.Paths, *path)
			case "wrong type":
				path.Delivery.ObjectType = "ZoneHVAC:Baseboard:Convective:Electric"
			case "wrong name":
				path.Delivery.ObjectName = "Other Baseboard"
			case "wrong index":
				path.Delivery.ObjectIndex++
			case "wrong component ID":
				path.Delivery.ID = "component:foreign"
			case "wrong Zone":
				path.ZoneName = "Other"
			case "wrong served Zone":
				path.ServedSubject.ZoneName = "Other"
			case "wrong summary Zone":
				owner.ZoneName = "Other"
			case "wrong summary subject Zone":
				owner.ServedSubject.ZoneName = "Other"
			case "Space path":
				path.SpaceName = "Office Space"
			case "Space subject":
				path.ServedSubject.SpaceName = "Office Space"
			case "Space summary":
				owner.SpaceName = "Office Space"
			case "Space summary subject":
				owner.ServedSubject.SpaceName = "Office Space"
			case "cooling":
				path.ServiceKind = "cooling"
			case "central path":
				path.PathType = "central_air"
			case "air loop":
				path.AirLoop = &idf.LoopRef{}
			case "plant loop":
				path.PlantLoop = &idf.LoopRef{}
			case "condenser loop":
				path.CondenserLoop = &idf.LoopRef{}
			case "refrigerant system":
				path.RefrigerantSystem = &idf.SystemRef{}
			}
			if got := buildEnergyPathNativeBaseboardPaths(doc, summaries); len(got) != 0 {
				t.Fatalf("unresolved/ambiguous topology acquired an exact native path: %v", got)
			}
		})
	}
}

func TestEnergyPathNativeBaseboardNodePathsPreserveEverySource(t *testing.T) {
	doc := parsePurposePlanFixture(t, `Version,25.1;
Zone,Office,0,0,0,0,1,1;
ZoneHVAC:EquipmentConnections,Office,List,,,Office Air,;
ZoneHVAC:EquipmentList,List,SequentialLoad,ZoneHVAC:Baseboard:RadiantConvective:Electric,BB1,1,1,,,ZoneHVAC:Baseboard:RadiantConvective:Electric,BB2,2,2,,;
ZoneHVAC:Baseboard:RadiantConvective:Electric,BB1,Always,HeatingDesignCapacity,1000,,,.97,0,0;
ZoneHVAC:Baseboard:RadiantConvective:Electric,BB2,Always,HeatingDesignCapacity,2000,,,.97,0,0;`)
	paths := buildEnergyPathNativeBaseboardPaths(doc, idf.AnalyzeHVAC(doc).ServiceModel.ZoneServices)
	if len(paths) != 2 {
		t.Fatalf("two same-Zone Baseboards did not retain two exact identities: %v", paths)
	}
	sources := []EnergyDataSource{baseboardPathTestSource("native-1", "BB1", "Office"), baseboardPathTestSource("native-2", "BB2", "Office")}
	scope := EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}
	base := EnergyExplanationNode{ID: "energy.direct_zone.end_use.heating.electricity.office", Level: "energy", EndUse: "heating", Carrier: "electricity", ZoneName: "Office", Basis: "direct_zone_energy", Value: 30, RawValue: 30, EffectiveValue: 30, AllocatedValue: 30, DisplayValue: 30, Unit: "kWh", SourceIDs: []string{"native-1", "native-2"}, RelatedPathIDs: []string{"central-cooling", "central-gas"}}
	nodes := []EnergyExplanationNode{base}
	qualifyEnergyPathNativeBaseboardNodes(nodes, sources, scope, paths)
	want := append(append([]string(nil), paths[energyPathNativeBaseboardPathKey("Office", "BB1")]...), paths[energyPathNativeBaseboardPathKey("Office", "BB2")]...)
	sort.Strings(want)
	if !reflect.DeepEqual(nodes[0].RelatedPathIDs, want) || len(want) != 2 {
		t.Fatalf("same-Zone native contributors were partially truncated or borrowed central paths: %v, want %v", nodes[0].RelatedPathIDs, want)
	}
	nodes[0].RelatedPathIDs = base.RelatedPathIDs
	if !reflect.DeepEqual(nodes[0], base) {
		t.Fatal("path qualification changed numeric/source identity fields")
	}
	for _, mutation := range []string{"unknown topology", "unknown equipment key", "mixed foreign source", "Hourly", "Rate", "meter", "wrong source unit", "wrong normalized unit", "foreign Zone"} {
		t.Run(mutation, func(t *testing.T) {
			localSources := append([]EnergyDataSource(nil), sources...)
			localPaths := paths
			node := base
			clearPaths := false
			switch mutation {
			case "unknown topology":
				localPaths, clearPaths = nil, true
			case "unknown equipment key":
				localSources[0].KeyValue, localSources[1].KeyValue = "Missing 1", "Missing 2"
				clearPaths = true
			case "mixed foreign source":
				localSources[1].Name = "Heating Coil Electricity Energy"
			case "Hourly":
				localSources[1].ReportingFrequency = "Hourly"
			case "Rate":
				localSources[1].Name = "Baseboard Electricity Rate"
			case "meter":
				localSources[1].IsMeter = true
			case "wrong source unit":
				localSources[1].SourceUnit = "kJ"
			case "wrong normalized unit":
				localSources[1].NormalizedUnit = "W"
			case "foreign Zone":
				localSources[1].ZoneName = "Other"
			}
			localNodes := []EnergyExplanationNode{node}
			qualifyEnergyPathNativeBaseboardNodes(localNodes, localSources, scope, localPaths)
			wantNode := node
			if clearPaths {
				wantNode.RelatedPathIDs = nil
			}
			if !reflect.DeepEqual(localNodes[0], wantNode) {
				t.Fatalf("native path gate changed quantities or partially truncated mixed/foreign contributors: got %+v want %+v", localNodes[0], wantNode)
			}
		})
	}
}

func baseboardPathTestSource(id, key, zone string) EnergyDataSource {
	return EnergyDataSource{ID: id, SourceType: "sql_report_data", Name: "Baseboard Electricity Energy", KeyValue: key, ZoneName: zone, ReportingFrequency: "Monthly", SourceUnit: "J", NormalizedUnit: "kWh"}
}

func TestEnergyPathNativeBaseboardSQLPathsAndReloadPreserveAmounts(t *testing.T) {
	path, input, _, plan, context := mixedHeatingConsumptionSQLFixture(t)
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	legacy = applyEnergyExplanationV1ServicePathLoadShareAllocation(enrichEnergyExplanationWithServicePaths(legacy, input))
	if len(legacy.servicePathIndex.nativeBaseboardPaths) != 2 {
		t.Fatal("real purpose enrichment did not retain the original two-device path index")
	}
	// Deliberately remove only the exact path index in the control. The same
	// graph's site/thermal scalars and source IDs must remain byte-identical.
	withoutPaths := legacy
	withoutPaths.servicePathIndex.nativeBaseboardPaths = nil
	control := UpgradeEnergyExplanationV1(withoutPaths)
	result := UpgradeEnergyExplanationV1(legacy)
	if !reflect.DeepEqual(baseboardPathNumericSourceSnapshot(result), baseboardPathNumericSourceSnapshot(control)) {
		t.Fatal("exact equipment-path qualification changed numeric or source identity fields")
	}
	baseline := baseboardPathNumericSourceSnapshot(result)
	for reload := 0; reload < 3; reload++ {
		seenZones := map[string]bool{}
		for _, zone := range result.ZoneResults {
			objectIndex, sourceID, first := 0, "", 0.0
			switch zone.Scope.ZoneName {
			case "SPACE2-1":
				objectIndex, sourceID, first = 167, "sql-rdd-50", 10
			case "SPACE4-1":
				objectIndex, sourceID, first = 168, "sql-rdd-51", 30
			default:
				continue
			}
			seenZones[zone.Scope.ZoneName] = true
			wantPath := fmt.Sprintf("service-path:zone:%s:heating:baseboard:component:%d", strings.ToLower(zone.Scope.ZoneName), objectIndex)
			periods := append([]EnergyPeriod{{ID: "root annual", Nodes: zone.Nodes, Links: zone.Links}}, zone.Periods...)
			for _, period := range periods {
				factor := 0.0
				switch period.ID {
				case "root annual", "annual":
					factor = 3
				case "M1":
					factor = 1
				case "M2":
					factor = 2
				default:
					continue
				}
				count := 0
				for _, link := range period.Links {
					if !energyPathLinkIsCarrierSplit(link) || link.Basis != "direct_zone_energy" || !stringSliceContains(link.SourceIDs, sourceID) {
						continue
					}
					count++
					if !reflect.DeepEqual(link.RelatedPathIDs, []string{wantPath}) || !reflect.DeepEqual(link.SourceIDs, []string{sourceID}) || math.Abs(link.FromValue-first*factor) > 1e-9 || math.Abs(link.ToValue-first*factor) > 1e-9 {
						t.Errorf("reload %d %s/%s direct native electricity borrowed central paths or changed quantity/source: %+v", reload, zone.Scope.ZoneName, period.ID, link)
					}
				}
				if count != 1 {
					t.Errorf("reload %d %s/%s direct native branch count=%d, want one", reload, zone.Scope.ZoneName, period.ID, count)
				}
			}
		}
		if len(seenZones) != 2 || !reflect.DeepEqual(baseboardPathNumericSourceSnapshot(result), baseline) {
			t.Errorf("reload %d lost an original owner or changed numeric/source fields", reload)
		}
		if reload < 2 {
			wire, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var reopened EnergyExplanationResult
			if err := json.Unmarshal(wire, &reopened); err != nil {
				t.Fatal(err)
			}
			result = reopened
		}
	}
}

// Snapshot every graph amount and source identity, not just the narrowed
// Baseboard ribbon. Central gas/boiler contributions and Building quantities
// must remain unchanged as well. Paths are intentionally outside this snapshot.
func baseboardPathNumericSourceSnapshot(result EnergyExplanationResult) map[string]any {
	out := map[string]any{}
	graph := func(prefix string, nodes []EnergyExplanationNode, links []EnergyPathLink) {
		for _, node := range nodes {
			out[prefix+"/node/"+node.ID] = []any{node.Value, node.RawValue, node.EffectiveValue, node.AllocatedValue, node.DisplayValue, node.SignedValue, node.Multiplier, append([]string(nil), node.SourceIDs...)}
		}
		for _, link := range links {
			out[prefix+"/link/"+link.ID] = []any{link.FromValue, link.ToValue, link.Ratio, append([]string(nil), link.SourceIDs...)}
			if energyPathLinkIsCarrierSplit(link) && link.Basis != "direct_zone_energy" {
				out[prefix+"/unchanged-central-path/"+link.ID] = append([]string(nil), link.RelatedPathIDs...)
			}
		}
	}
	graph("building/root", result.Nodes, result.Links)
	for _, period := range result.Periods {
		graph("building/"+period.ID, period.Nodes, period.Links)
	}
	for _, zone := range result.ZoneResults {
		graph(zone.Scope.ZoneName+"/root", zone.Nodes, zone.Links)
		for _, period := range zone.Periods {
			graph(zone.Scope.ZoneName+"/"+period.ID, period.Nodes, period.Links)
		}
	}
	for _, source := range result.Sources {
		out["source/"+source.ID] = []any{source.Name, source.KeyValue, source.ZoneName, source.ReportingFrequency, source.SourceUnit, source.NormalizedUnit, source.RawValue, source.EffectiveValue, source.AllocatedValue, source.EffectiveMultiplier, source.AllocationFactor}
		for _, detail := range source.ScopeDetails {
			out["source/"+source.ID+"/scope/"+detail.Scope.Kind+"/"+detail.Scope.ZoneName] = []any{detail.RawValue, detail.EffectiveValue, detail.AllocatedValue, detail.EffectiveMultiplier, detail.AllocationFactor}
		}
	}
	return out
}
