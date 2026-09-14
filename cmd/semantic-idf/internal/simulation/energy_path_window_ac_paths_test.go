package simulation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathWindowACOriginalPathsBindAllNativeSourceFamilies(t *testing.T) {
	doc, _ := windowACOriginal(t)
	before := doc.String()
	targets := energyPathWindowACTargets(doc)
	paths := buildEnergyPathNativeWindowACPaths(doc, idf.AnalyzeHVAC(doc).ServiceModel.ZoneServices)
	if len(paths) != 27 {
		t.Fatalf("original source/path registry=%d want27", len(paths))
	}
	for _, target := range targets {
		want := []string{fmt.Sprintf("service-path:zone:%s:cooling:direct_zone_air:component:%d", strings.ToLower(target.ZoneName), target.Parent.ObjectIndex)}
		for _, pair := range [][2]string{{target.Coil.ObjectName, "Cooling Coil Electricity Energy"}, {target.Coil.ObjectName, "Cooling Coil Crankcase Heater Electricity Energy"}, {target.Fan.ObjectName, "Fan Electricity Energy"}} {
			got := paths[energyPathNativeWindowACPathKey(target.ZoneName, pair[0], pair[1])]
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("original parent path %s/%s=%v want%v", target.ZoneName, pair[1], got, want)
			}
		}
	}
	if doc.String() != before {
		t.Fatal("path registry changed original")
	}
}

func TestEnergyPathWindowACPathsKeepOwnerButClearUnresolvedTopology(t *testing.T) {
	for _, mutation := range []string{"missing", "duplicate path", "wrong type", "wrong name", "wrong component ID", "wrong index", "wrong Zone", "wrong served Zone", "wrong summary Zone", "wrong summary subject Zone", "Space path", "Space subject", "Space summary", "Space summary subject", "heating", "central path", "air loop", "plant loop", "condenser loop", "refrigerant system"} {
		t.Run(mutation, func(t *testing.T) {
			doc := windowACHandDocument(t)
			targets := energyPathWindowACTargets(doc)
			if len(targets) != 4 {
				t.Fatal("hand fixture lacks four native packages")
			}
			target := targets[0]
			summaries := idf.AnalyzeHVAC(doc).ServiceModel.ZoneServices
			var path *idf.ZoneServicePath
			var owner *idf.ZoneServiceSummary
			for i := range summaries {
				for j := range summaries[i].Paths {
					if summaries[i].Paths[j].Delivery.ID == target.Parent.ID {
						if path != nil {
							t.Fatal("control path is ambiguous")
						}
						path, owner = &summaries[i].Paths[j], &summaries[i]
					}
				}
			}
			if path == nil {
				t.Fatal("control native path missing")
			}
			switch mutation {
			case "missing":
				owner.Paths = nil
			case "duplicate path":
				owner.Paths = append(owner.Paths, *path)
			case "wrong type":
				path.Delivery.ObjectType = "ZoneHVAC:PackagedTerminalAirConditioner"
			case "wrong name":
				path.Delivery.ObjectName = "Foreign"
			case "wrong component ID":
				path.Delivery.ID = "component:foreign"
			case "wrong index":
				path.Delivery.ObjectIndex++
			case "wrong Zone":
				path.ZoneName = "Foreign"
			case "wrong served Zone":
				path.ServedSubject.ZoneName = "Foreign"
			case "wrong summary Zone":
				owner.ZoneName = "Foreign"
			case "wrong summary subject Zone":
				owner.ServedSubject.ZoneName = "Foreign"
			case "Space path":
				path.SpaceName = "Space"
			case "Space subject":
				path.ServedSubject.SpaceName = "Space"
			case "Space summary":
				owner.SpaceName = "Space"
			case "Space summary subject":
				owner.ServedSubject.SpaceName = "Space"
			case "heating":
				path.ServiceKind = "heating"
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
			got := buildEnergyPathNativeWindowACPaths(doc, summaries)
			if len(got) != 12 {
				t.Fatalf("native source membership lost: %v", got)
			}
			unresolved := 0
			for _, ids := range got {
				if len(ids) == 0 {
					unresolved++
				}
			}
			if unresolved != 3 {
				t.Fatalf("wrong topology changed unrelated owners: %v", got)
			}
			for _, pair := range [][2]string{{target.Coil.ObjectName, "Cooling Coil Electricity Energy"}, {target.Coil.ObjectName, "Cooling Coil Crankcase Heater Electricity Energy"}, {target.Fan.ObjectName, "Fan Electricity Energy"}} {
				if ids, exists := got[energyPathNativeWindowACPathKey(target.ZoneName, pair[0], pair[1])]; !exists || len(ids) != 0 {
					t.Fatalf("unresolved native source borrowed a path: %v", ids)
				}
			}
		})
	}
}

func windowACPathSource(id, key, name string) EnergyDataSource {
	return EnergyDataSource{ID: id, SourceType: "sql_report_data", Name: name, KeyValue: key, ZoneName: "Office", ReportingFrequency: "Monthly", SourceUnit: "J", NormalizedUnit: "kWh"}
}

func TestEnergyPathWindowACPathQualificationIsWholeSourceLocalAndNumericInvariant(t *testing.T) {
	paths := map[string][]string{
		energyPathNativeWindowACPathKey("Office", "Coil A", "Cooling Coil Electricity Energy"):                  {"window-a"},
		energyPathNativeWindowACPathKey("Office", "Coil A", "Cooling Coil Crankcase Heater Electricity Energy"): {"window-a"},
		energyPathNativeWindowACPathKey("Office", "Coil B", "Cooling Coil Electricity Energy"):                  {"window-b"},
		energyPathNativeWindowACPathKey("Office", "Fan A", "Fan Electricity Energy"):                            {"window-a"},
	}
	sources := []EnergyDataSource{windowACPathSource("main", "Coil A", "Cooling Coil Electricity Energy"), windowACPathSource("crankcase", "Coil A", "Cooling Coil Crankcase Heater Electricity Energy"), windowACPathSource("second", "Coil B", "Cooling Coil Electricity Energy")}
	base := EnergyExplanationNode{ID: "energy.direct_zone.end_use.cooling.electricity.office", Level: "energy", EndUse: "cooling", Carrier: "electricity", ZoneName: "Office", Basis: "direct_zone_energy", Value: 30, RawValue: 30, EffectiveValue: 30, AllocatedValue: 30, DisplayValue: 30, Unit: "kWh", SourceIDs: []string{"main", "crankcase", "second"}, RelatedPathIDs: []string{"central-cooling", "central-gas"}}
	scope := EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}
	nodes := []EnergyExplanationNode{base}
	qualifyEnergyPathNativeWindowACNodes(nodes, sources, scope, paths)
	if !nodes[0].nativeWindowACPathQualified || !reflect.DeepEqual(nodes[0].RelatedPathIDs, []string{"window-a", "window-b"}) {
		t.Fatalf("same-Zone native package union lost: %+v", nodes[0])
	}
	nodes[0].RelatedPathIDs = base.RelatedPathIDs
	nodes[0].nativeWindowACPathQualified = false
	if !reflect.DeepEqual(nodes[0], base) {
		t.Fatal("qualification changed amounts or source IDs")
	}
	for _, mutation := range []string{"one unresolved topology", "foreign source", "wrong family equipment key", "duplicate source ID", "Hourly", "Rate", "meter", "wrong unit", "wrong normalized unit", "foreign Zone", "Building scope"} {
		t.Run(mutation, func(t *testing.T) {
			localSources := append([]EnergyDataSource(nil), sources...)
			localPaths := map[string][]string{}
			for key, value := range paths {
				localPaths[key] = value
			}
			localScope := scope
			want := base
			switch mutation {
			case "one unresolved topology":
				localPaths[energyPathNativeWindowACPathKey("Office", "Coil B", "Cooling Coil Electricity Energy")] = nil
				want.RelatedPathIDs = nil
				want.nativeWindowACPathQualified = true
			case "foreign source":
				localSources[2].Name = "Heating Coil Electricity Energy"
			case "wrong family equipment key":
				localSources[2].KeyValue = "Fan A"
			case "duplicate source ID":
				localSources = append(localSources, localSources[0])
			case "Hourly":
				localSources[2].ReportingFrequency = "Hourly"
			case "Rate":
				localSources[2].Name = "Cooling Coil Electricity Rate"
			case "meter":
				localSources[2].IsMeter = true
			case "wrong unit":
				localSources[2].SourceUnit = "kJ"
			case "wrong normalized unit":
				localSources[2].NormalizedUnit = "W"
			case "foreign Zone":
				localSources[2].ZoneName = "Other"
			case "Building scope":
				localScope.Kind = "building"
			}
			nodes := []EnergyExplanationNode{base}
			qualifyEnergyPathNativeWindowACNodes(nodes, localSources, localScope, localPaths)
			if !reflect.DeepEqual(nodes[0], want) {
				t.Fatalf("mixed/unresolved input changed quantities, IDs or partially truncated provenance: %+v want%+v", nodes[0], want)
			}
		})
	}
	// A native fan retains the owning cooling-only package route without
	// merging its electricity into the coil's cooling end-use subtotal.
	fan := base
	fan.EndUse = "fans"
	fan.SourceIDs = []string{"fan"}
	fanSources := []EnergyDataSource{windowACPathSource("fan", "Fan A", "Fan Electricity Energy")}
	fanNodes := []EnergyExplanationNode{fan}
	qualifyEnergyPathNativeWindowACNodes(fanNodes, fanSources, scope, paths)
	sort.Strings(fanNodes[0].RelatedPathIDs)
	if !fanNodes[0].nativeWindowACPathQualified || !reflect.DeepEqual(fanNodes[0].RelatedPathIDs, []string{"window-a"}) || fanNodes[0].EndUse != "fans" || fanNodes[0].Value != base.Value {
		t.Fatalf("native fan ownership/end-use changed: %+v", fanNodes[0])
	}
}
