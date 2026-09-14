package simulation

// These tests read the hash-bound original IDF, never candidate/expected values.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func energyPathPoolOriginal(t *testing.T) idf.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneSwimmingPoolZoneMultipliers.idf"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "e8fe7fdc91c4127f55f645ac059323771fa4a92f16e86900953fa8d2ceff9301" {
		t.Fatalf("original fixture changed: %s", got)
	}
	doc, err := idf.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func energyPathPoolTestObject(t *testing.T, doc *idf.Document, kind, name string) *idf.Object {
	t.Helper()
	for at := range doc.Objects {
		object := &doc.Objects[at]
		if strings.EqualFold(object.Type, kind) && (name == "" || strings.EqualFold(energyPathPoolField(*object, 0), name)) {
			return object
		}
	}
	t.Fatalf("missing original %s/%s", kind, name)
	return nil
}

func energyPathPoolTestAppend(doc *idf.Document, object idf.Object) {
	object.Index = len(doc.Objects)
	object.Fields = append([]idf.Field(nil), object.Fields...)
	doc.Objects = append(doc.Objects, object)
}

func energyPathPoolTestLoop(t *testing.T, inventory energyPathPoolInventory, name string) energyPathPoolPlant {
	t.Helper()
	for _, loop := range inventory.Loops {
		if loop.Component.ObjectName == name {
			return loop
		}
	}
	t.Fatalf("missing loop inventory %s", name)
	return energyPathPoolPlant{}
}

func energyPathPoolTestSource(t *testing.T, inventory energyPathPoolInventory, name string) energyPathPoolSource {
	t.Helper()
	for _, source := range inventory.Sources {
		if source.Component.ObjectName == name {
			return source
		}
	}
	t.Fatalf("missing source evidence %s", name)
	return energyPathPoolSource{}
}

func TestEnergyPathPoolOriginalWaterInventoryPreservesAllDemands(t *testing.T) {
	doc := energyPathPoolOriginal(t)
	before := doc.String()
	got := energyPathNativePoolInventory(doc)
	if !got.HasNativePool || !got.SchemaReviewed || len(got.Pools) != 1 || len(got.Sources) != 4 || len(got.Loops) != 2 || len(got.UnresolvedPoolIndices) != 0 {
		t.Fatalf("original inventory: %+v", got)
	}
	pool := got.Pools[0]
	if !pool.resolved() || pool.Component.ObjectIndex != 301 || pool.Surface.ObjectIndex != 78 || pool.Surface.ObjectName != "F1-1" || pool.ZoneName != "SPACE1-1" || pool.Loop.ObjectIndex != 231 || pool.Branch.ObjectIndex != 254 {
		t.Fatalf("exact original pool/surface/Zone/water owner: %+v", pool)
	}
	for _, tc := range []struct {
		loop    string
		demands map[string]string
		sources map[string]int
		nonZone bool
	}{
		{"Hot Water Loop", map[string]string{
			"SPACE1-1 Zone Coil": "heating_coil", "SPACE2-1 Zone Coil": "heating_coil", "SPACE3-1 Zone Coil": "heating_coil",
			"SPACE4-1 Zone Coil": "heating_coil", "SPACE5-1 Zone Coil": "heating_coil", "OA Heating Coil 1": "heating_coil",
			"Main Heating Coil 1": "heating_coil", "Test Pool": "pool",
		}, map[string]int{"Central Boiler": 264, "HW Circ Pump": 267}, true},
		{"Chilled Water Loop", map[string]string{"Main Cooling Coil 1": "cooling_coil", "OA Cooling Coil 1": "cooling_coil"},
			map[string]int{"Central Chiller": 297, "CW Circ Pump": 300}, false},
	} {
		loop := energyPathPoolTestLoop(t, got, tc.loop)
		if !loop.WaterTopologyComplete || !loop.SupplyRosterComplete || !loop.DemandRosterComplete || loop.HasNonZoneDemand != tc.nonZone || loop.AirRoutesComplete {
			t.Fatalf("water completeness must not claim air routes for %s: %+v", tc.loop, loop)
		}
		if len(loop.Demands) != len(tc.demands) || len(loop.Sources) != len(tc.sources) {
			t.Fatalf("entire native roster must be retained for %s: %+v", tc.loop, loop)
		}
		seen := map[string]bool{}
		for _, demand := range loop.Demands {
			name := demand.Water.Component.ObjectName
			if demand.Kind != tc.demands[name] || seen[name] || !demand.Water.IdentityValid || !demand.Water.WaterOwnerValid || demand.AirRouteComplete || len(demand.RelatedPathIDs) != 0 {
				t.Fatalf("unproved/omitted/duplicate native demand: %+v", demand)
			}
			seen[name] = true
			if demand.Kind == "pool" {
				if !demand.NonZoneDemand || demand.ZoneName != "SPACE1-1" {
					t.Fatalf("pool is component transfer, not ZoneHVAC delivery: %+v", demand)
				}
			} else if demand.NonZoneDemand || demand.ZoneName != "" {
				t.Fatalf("no numeric/name-based coil owner inference: %+v", demand)
			}
		}
		for _, source := range loop.Sources {
			if !source.IdentityValid || !source.WaterOwnerValid || source.Component.ObjectIndex != tc.sources[source.Component.ObjectName] || source.Loop.ObjectIndex != loop.Component.ObjectIndex {
				t.Fatalf("exact source owner: %+v", source)
			}
		}
	}
	if boiler := energyPathPoolTestSource(t, got, "Central Boiler"); boiler.FuelType != "NaturalGas" || boiler.Kind != "boiler" {
		t.Fatalf("native fuel must be explicit, not inferred from names: %+v", boiler)
	}
	if doc.String() != before {
		t.Fatal("inventory mutated original IDF")
	}
	// Removing comments and changing representative Zone factors cannot change
	// model-total source identity, water ownership, or fabricate PLENUM service.
	for at := range doc.Objects {
		for field := range doc.Objects[at].Fields {
			doc.Objects[at].Fields[field].Comment = ""
		}
		if strings.EqualFold(doc.Objects[at].Type, "Zone") {
			doc.Objects[at].Fields[6].Value = "17"
		}
	}
	if after := energyPathNativePoolInventory(doc); !reflect.DeepEqual(after, got) {
		t.Fatal("comments or representative multiplier altered native component inventory")
	}
}

func TestEnergyPathPoolPresenceSurvivesRejectedSchemaOrOwner(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *idf.Document)
	}{
		{"unsupported version", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Version", "").Fields[0].Value = "26.1"
		}},
		{"duplicate version", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, *energyPathPoolTestObject(t, doc, "Version", ""))
		}},
		{"blank pool name", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "SwimmingPool:Indoor", "Test Pool").Fields[0].Value = ""
		}},
		{"truncated pool", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "SwimmingPool:Indoor", "Test Pool").Fields = nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := energyPathPoolOriginal(t)
			tc.mutate(t, &doc)
			got := energyPathNativePoolInventory(doc)
			if !energyPathHasNativePool(doc) || !got.HasNativePool || len(got.Pools) != 1 || len(got.UnresolvedPoolIndices) != 1 || got.Pools[0].resolved() {
				t.Fatalf("malformed target must not disable broad-boundary protection: %+v", got)
			}
		})
	}
	doc, err := idf.Parse("Version,25.1; Zone,Office; ZoneHVAC:Baseboard:Convective:Electric,Baseboard,,HeatingDesignCapacity,100;")
	if err != nil {
		t.Fatal(err)
	}
	if got := energyPathNativePoolInventory(doc); got.HasNativePool || got.Pools != nil || got.Sources != nil || got.Loops != nil {
		t.Fatalf("no new baseboard gate or mutation to non-pool models: %+v", got)
	}
}

func TestEnergyPathPoolSurfaceOwnerRejectsAmbiguity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *idf.Document)
	}{
		{"duplicate pool name", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, *energyPathPoolTestObject(t, doc, "SwimmingPool:Indoor", "Test Pool"))
		}},
		{"second pool same surface", func(t *testing.T, doc *idf.Document) {
			copy := *energyPathPoolTestObject(t, doc, "SwimmingPool:Indoor", "Test Pool")
			energyPathPoolTestAppend(doc, copy)
			doc.Objects[len(doc.Objects)-1].Fields[0].Value = "Other Pool"
		}},
		{"duplicate detailed surface", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, *energyPathPoolTestObject(t, doc, "BuildingSurface:Detailed", "F1-1"))
		}},
		{"duplicate legacy surface name", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, idf.Object{Type: "Floor:GroundContact", Fields: []idf.Field{{Value: "F1-1"}}})
		}},
		{"duplicate Zone", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, *energyPathPoolTestObject(t, doc, "Zone", "SPACE1-1"))
		}},
		{"missing Zone", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "BuildingSurface:Detailed", "F1-1").Fields[3].Value = "not a Zone"
		}},
		{"not floor", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "BuildingSurface:Detailed", "F1-1").Fields[1].Value = "Wall"
		}},
		{"unreviewed Space owner", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "BuildingSurface:Detailed", "F1-1").Fields[4].Value = "SPACE1-1"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := energyPathPoolOriginal(t)
			tc.mutate(t, &doc)
			got := energyPathNativePoolInventory(doc)
			if !got.HasNativePool || len(got.UnresolvedPoolIndices) == 0 || got.Pools[0].resolved() || got.Pools[0].SurfaceOwnerValid {
				t.Fatalf("ambiguous native surface owner accepted: %+v", got.Pools)
			}
			loop := energyPathPoolTestLoop(t, got, "Hot Water Loop")
			if !loop.HasNonZoneDemand || loop.DemandRosterComplete || loop.AirRoutesComplete {
				t.Fatalf("rejected pool cannot disappear from shared demand evidence: %+v", loop)
			}
		})
	}
}

func TestEnergyPathPoolWaterTopologyMutationsFailClosedWithoutLosingSources(t *testing.T) {
	for _, tc := range []struct {
		name, loop string
		mutate     func(*testing.T, *idf.Document)
	}{
		{"pool branch uses wrong outlet", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Branch", "Swimming Pool Branch").Fields[5].Value = "wrong outlet"
		}},
		{"pool branch old five-field stride", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			o := energyPathPoolTestObject(t, doc, "Branch", "Swimming Pool Branch")
			o.Fields = append(o.Fields, idf.Field{Value: "Active"})
		}},
		{"truncated branch retains demand identity", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			o := energyPathPoolTestObject(t, doc, "Branch", "Swimming Pool Branch")
			o.Fields = o.Fields[:5]
		}},
		{"duplicate branch object", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, *energyPathPoolTestObject(t, doc, "Branch", "Swimming Pool Branch"))
		}},
		{"duplicate BranchList member", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			o := energyPathPoolTestObject(t, doc, "BranchList", "Heating Demand Side Branches")
			o.Fields = append(o.Fields, idf.Field{Value: "Swimming Pool Branch"})
		}},
		{"second unreferenced BranchList owner", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, idf.Object{Type: "BranchList", Fields: []idf.Field{{Value: "Foreign List"}, {Value: "Swimming Pool Branch"}}})
		}},
		{"second loop BranchList owner", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			copy := *energyPathPoolTestObject(t, doc, "PlantLoop", "Hot Water Loop")
			energyPathPoolTestAppend(doc, copy)
			doc.Objects[len(doc.Objects)-1].Fields[0].Value = "Foreign Loop"
		}},
		{"splitter loses pool branch", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Connector:Splitter", "Heating Demand Splitter").Fields[9].Value = "Foreign Branch"
		}},
		{"mixer duplicate internal member", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Connector:Mixer", "Heating Demand Mixer").Fields[9].Value = "SPACE1-1 Reheat Branch"
		}},
		{"second connector list owner", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			copy := *energyPathPoolTestObject(t, doc, "ConnectorList", "Heating Demand Side Connectors")
			energyPathPoolTestAppend(doc, copy)
			doc.Objects[len(doc.Objects)-1].Fields[0].Value = "Foreign Connectors"
		}},
		{"off-stride connector alias still conflicts", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestAppend(doc, idf.Object{Type: "ConnectorList", Fields: []idf.Field{{Value: "Malformed Foreign Connectors"}, {Value: "junk"}, {Value: "Connector:Splitter"}, {Value: "Heating Demand Splitter"}}})
		}},
		{"air loop also claims water connector list", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "AirLoopHVAC", "VAV Sys 1").Fields[5].Value = "Heating Demand Side Connectors"
		}},
		{"loop demand inlet mismatch", "Hot Water Loop", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "PlantLoop", "Hot Water Loop").Fields[14].Value = "wrong outer node"
		}},
		{"chiller condenser ports cannot stand for chilled ports", "Chilled Water Loop", func(t *testing.T, doc *idf.Document) {
			chiller := energyPathPoolTestObject(t, doc, "Chiller:Electric", "Central Chiller")
			branch := energyPathPoolTestObject(t, doc, "Branch", "Central Chiller Branch")
			branch.Fields[4].Value, branch.Fields[5].Value = chiller.Fields[6].Value, chiller.Fields[7].Value
		}},
		{"unknown OA cooling demand is not omitted", "Chilled Water Loop", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Branch", "OA Cooling Coil Branch").Fields[2].Value = "Coil:Unreviewed:Water"
		}},
		{"CW splitter loses OA cooling", "Chilled Water Loop", func(t *testing.T, doc *idf.Document) {
			energyPathPoolTestObject(t, doc, "Connector:Splitter", "CW Demand Splitter").Fields[3].Value = "Cooling Coil Branch"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := energyPathPoolOriginal(t)
			tc.mutate(t, &doc)
			before := doc.String()
			got := energyPathNativePoolInventory(doc)
			loop := energyPathPoolTestLoop(t, got, tc.loop)
			if loop.WaterTopologyComplete || loop.AirRoutesComplete || (loop.SupplyRosterComplete && loop.DemandRosterComplete) {
				t.Fatalf("malformed original topology accepted: %+v", loop)
			}
			if !got.HasNativePool || len(got.Sources) != 4 {
				t.Fatalf("failure erased native presence/source evidence: %+v", got)
			}
			if tc.name == "unknown OA cooling demand is not omitted" {
				if len(loop.Demands) != 2 || loop.Demands[1].Kind != "unknown" || loop.Demands[1].Water.Component.ObjectName != "OA Cooling Coil 1" || loop.Demands[1].Water.Component.ObjectIndex != -1 {
					t.Fatalf("unknown demand identity was hidden: %+v", loop.Demands)
				}
			}
			if tc.name == "splitter loses pool branch" {
				for _, name := range []string{"Central Boiler", "HW Circ Pump"} {
					source := energyPathPoolTestSource(t, got, name)
					if !source.IdentityValid || !source.WaterOwnerValid {
						t.Fatalf("a bad demand connector must not erase independent native source identity: %+v", source)
					}
				}
			}
			if doc.String() != before {
				t.Fatal("mutation test input modified by inventory")
			}
		})
	}
}

func TestEnergyPathPoolReportingIdentityRejectsCrossTypeAliases(t *testing.T) {
	for _, tc := range []struct{ native, alias string }{
		{"Central Boiler", "Boiler:Steam"},
		{"HW Circ Pump", "Pump:ConstantSpeed"},
		{"CW Circ Pump", "HeaderedPumps:VariableSpeed"},
		{"Central Chiller", "Chiller:Electric:EIR"},
	} {
		t.Run(tc.alias, func(t *testing.T) {
			doc := energyPathPoolOriginal(t)
			energyPathPoolTestAppend(&doc, idf.Object{Type: tc.alias, Fields: []idf.Field{{Value: strings.ToLower(tc.native)}}})
			got := energyPathNativePoolInventory(doc)
			source := energyPathPoolTestSource(t, got, tc.native)
			if source.IdentityValid || source.WaterOwnerValid {
				t.Fatalf("SQL reporting key cannot distinguish native peer: %+v", source)
			}
			if len(got.Sources) != 4 || !got.HasNativePool {
				t.Fatal("rejected source disappeared instead of retaining evidence")
			}
		})
	}
}

func TestEnergyPathPoolOrphanDemandCannotDisablePresenceProtection(t *testing.T) {
	doc := energyPathPoolOriginal(t)
	// Remove the same pool member from all three reviewed rosters, leaving the
	// original pool and its water branch intact but unowned. Local HW water
	// topology can now be internally consistent; global unresolved pool evidence
	// must still forbid interpreting its seven visible coils as the whole plant.
	for _, tc := range []struct {
		kind, name string
		field      int
	}{
		{"BranchList", "Heating Demand Side Branches", 9},
		{"Connector:Splitter", "Heating Demand Splitter", 9},
		{"Connector:Mixer", "Heating Demand Mixer", 9},
	} {
		object := energyPathPoolTestObject(t, &doc, tc.kind, tc.name)
		if energyPathPoolField(*object, tc.field) != "Swimming Pool Branch" {
			t.Fatalf("wrong original mutation slot %s/%d", tc.name, tc.field)
		}
		object.Fields = append(object.Fields[:tc.field], object.Fields[tc.field+1:]...)
	}
	got := energyPathNativePoolInventory(doc)
	if !got.HasNativePool || len(got.UnresolvedPoolIndices) != 1 || got.UnresolvedPoolIndices[0] != 301 || got.Pools[0].WaterOwnerValid || got.Pools[0].resolved() {
		t.Fatalf("orphan pool must remain an explicit unresolved physical demand: %+v", got)
	}
	loop := energyPathPoolTestLoop(t, got, "Hot Water Loop")
	if len(loop.Demands) != 7 || loop.AirRoutesComplete {
		t.Fatalf("visible coil roster cannot be elevated to full service eligibility: %+v", loop)
	}
}
