package simulation

import (
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func simpleVentilationHandDocument(t *testing.T) idf.Document {
	t.Helper()
	return parsePurposePlanFixture(t, `Version,25.1;
Zone,Natural,0,0,0,0,1,1;
Zone,Intake,0,0,0,0,1,4;
Zone,Exhaust,0,0,0,0,1,2;
ZoneList,Repeated,Exhaust;
ZoneGroup,RepeatedGroup,Repeated,4;
Schedule:Constant,Always,,1;
ZoneVentilation:DesignFlowRate,Natural inlet,Natural,Always,Flow/Zone,1,,,,Natural,0,1,1,0,0,0;
ZoneVentilation:DesignFlowRate,Intake inlet,Intake,Always,Flow/Zone,1,,,,Intake,400,0.9,1,0,0,0;
ZoneVentilation:DesignFlowRate,Exhaust outlet,Exhaust,Always,Flow/Zone,1,,,,Exhaust,400,0.8,1,0,0,0;
ZoneVentilation:WindandStackOpenArea,Exhaust opening,Exhaust,0.5,Always,0.2,0,1,0.5;`)
}

func TestEnergyPathSimpleVentilationOriginalUncontrolledThreeAggregates(t *testing.T) {
	// The vendored official original is required on every checkout. Never
	// consult a machine-local installation or silently skip missing input.
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "VentilationSimpleTest.idf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "e38887ebcbfec13df596bcd77ed1d5965a65a4bd1e0ad026672f449d8fbe1074" {
		t.Fatalf("official original changed: %s", got)
	}
	doc := parsePurposePlanFixture(t, string(data))
	before := doc.String()
	targets := energyPathSimpleVentilationTargets(doc)
	if len(targets) != 3 {
		t.Fatalf("Zone source census=%+v", targets)
	}
	want := map[string][]int{"ZONE 1": {34, 38}, "ZONE 2": {39, 43}, "ZONE 3": {44, 48, 49}}
	for _, target := range targets {
		got := []int{target.Zone.ObjectIndex}
		for _, member := range target.Members {
			got = append(got, member.ObjectIndex)
		}
		if !reflect.DeepEqual(got, want[target.ZoneName]) {
			t.Fatalf("original reporting owner/member identity=%s %v", target.ZoneName, got)
		}
	}
	for _, object := range doc.Objects {
		lower := strings.ToLower(object.Type)
		for _, prefix := range []string{"airloophvac", "plantloop", "zonehvac:", "zonecontrol:", "coil:", "fan:", "pump:"} {
			if strings.HasPrefix(lower, prefix) {
				t.Fatalf("uncontrolled original gained %s", object.Type)
			}
		}
	}
	for _, summary := range idf.AnalyzeHVAC(doc).ServiceModel.ZoneServices {
		for _, path := range summary.Paths {
			if path.ServiceKind == "heating" || path.ServiceKind == "cooling" {
				t.Fatalf("uncontrolled original invented conditioning path: %+v", path)
			}
		}
	}
	if doc.String() != before {
		t.Fatal("source inventory modified original physics")
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(data, after) {
		t.Fatal("original bytes modified")
	}
}

func TestEnergyPathSimpleVentilationTypedAggregationDefaultsAndAdverseInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*idf.Document)
		count  int
	}{
		{"all four objects are only three sources", nil, 3},
		{"blank native natural pressure efficiency defaults", func(doc *idf.Document) {
			o := directHVACFixtureObject(t, doc, "ZoneVentilation:DesignFlowRate", "Natural inlet")
			o.Fields[8].Value = ""
			o.Fields[9].Value = ""
			o.Fields[10].Value = ""
		}, 3},
		{"blank type is natural even when name says heater", func(doc *idf.Document) {
			o := directHVACFixtureObject(t, doc, "ZoneVentilation:DesignFlowRate", "Natural inlet")
			o.Fields[0].Value = "Gas heating fan"
			o.Fields[8].Value = ""
		}, 3},
		{"comment cannot change owner", func(doc *idf.Document) {
			o := directHVACFixtureObject(t, doc, "ZoneVentilation:DesignFlowRate", "Intake inlet")
			o.Fields[0].Comment = "Zone or ZoneList or Space or SpaceList Name"
		}, 3},
		{"case insensitive native owner", func(doc *idf.Document) {
			o := directHVACFixtureObject(t, doc, "ZoneVentilation:DesignFlowRate", "Intake inlet")
			o.Fields[1].Value = " intake "
		}, 3},
		{"quadrature suppresses this reporting family", func(doc *idf.Document) {
			doc.Objects = append(doc.Objects, parsePurposePlanFixture(t, `ZoneAirBalance:OutdoorAir,Balance,Intake,Quadrature;`).Objects...)
		}, 2},
		{"none balance preserves family", func(doc *idf.Document) {
			doc.Objects = append(doc.Objects, parsePurposePlanFixture(t, `ZoneAirBalance:OutdoorAir,Balance,Intake,None;`).Objects...)
		}, 3},
		{"negative pressure", func(doc *idf.Document) {
			directHVACFixtureObject(t, doc, "ZoneVentilation:DesignFlowRate", "Intake inlet").Fields[9].Value = "-1"
		}, 2},
		{"zero efficiency", func(doc *idf.Document) {
			directHVACFixtureObject(t, doc, "ZoneVentilation:DesignFlowRate", "Intake inlet").Fields[10].Value = "0"
		}, 2},
		{"nonfinite pressure", func(doc *idf.Document) {
			directHVACFixtureObject(t, doc, "ZoneVentilation:DesignFlowRate", "Intake inlet").Fields[9].Value = "NaN"
		}, 2},
		{"wrong native type", func(doc *idf.Document) {
			directHVACFixtureObject(t, doc, "ZoneVentilation:DesignFlowRate", "Intake inlet").Fields[8].Value = "Heating"
		}, 2},
		{"duplicate global ventilation name invalidates both owners", func(doc *idf.Document) {
			directHVACFixtureObject(t, doc, "ZoneVentilation:WindandStackOpenArea", "Exhaust opening").Fields[0].Value = "Intake inlet"
		}, 1},
		{"unknown Zone declines cohort", func(doc *idf.Document) {
			directHVACFixtureObject(t, doc, "ZoneVentilation:DesignFlowRate", "Intake inlet").Fields[1].Value = "missing"
		}, 0},
		{"ambiguous Zone declines cohort", func(doc *idf.Document) {
			doc.Objects = append(doc.Objects, *directHVACFixtureObject(t, doc, "Zone", "Intake"))
		}, 0},
		{"unreviewed ZoneList selector never partially binds", func(doc *idf.Document) {
			directHVACFixtureObject(t, doc, "ZoneVentilation:DesignFlowRate", "Intake inlet").Fields[1].Value = "Repeated"
		}, 0},
		{"selector cross namespace collision", func(doc *idf.Document) {
			doc.Objects = append(doc.Objects, parsePurposePlanFixture(t, `ZoneList,Intake,Exhaust;`).Objects...)
		}, 0},
		{"unreviewed version not promoted", func(doc *idf.Document) { directHVACFixtureObject(t, doc, "Version", "25.1").Fields[0].Value = "22.1" }, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := simpleVentilationHandDocument(t)
			if tc.mutate != nil {
				tc.mutate(&doc)
			}
			before := doc.String()
			got := energyPathSimpleVentilationTargets(doc)
			if len(got) != tc.count {
				t.Fatalf("targets=%+v want%d", got, tc.count)
			}
			if doc.String() != before {
				t.Fatal("inventory rewrote input")
			}
		})
	}
	if got := energyPathSimpleVentilationTargets(idf.Document{}); got != nil {
		t.Fatal("unrelated model failed fast empty return")
	}
}

func TestEnergyPathSimpleVentilationNativeMeterFactorOneNotZoneExpansion(t *testing.T) {
	doc := simpleVentilationHandDocument(t)
	index := buildEnergyEffectiveMultiplierIndex(doc)
	for _, zone := range []string{"Natural", "Intake", "Exhaust"} {
		item := canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "energy", Kind: "energy.fans", EndUse: "fans", Carrier: "electricity", Unit: "kWh", ZoneName: zone, MeterHierarchyLevel: "zone_direct_use", Basis: "direct_zone_energy", Total: 6, Monthly: map[int]float64{1: 2, 2: 4}, directComponentID: energyPathSimpleVentilationFanID, sourceName: energyPathSimpleVentilationFanName, sourceKeyValue: zone, sourceFrequency: "Monthly"})
		factor, application, known := energyExplanationMultiplierForSeries(item, index)
		if !known || factor != 1 || application != energyMultiplierAlreadyModelTotal {
			t.Fatalf("native report acquired Zone expansion: %s %g %s", zone, factor, application)
		}
		series, _, _ := applyEnergyExplanationMultipliers([]energyExplanationSeries{item}, nil, index)
		if series[0].Total != 6 || series[0].Monthly[1] != 2 || series[0].Monthly[2] != 4 {
			t.Fatalf("native observations scaled: %+v", series[0])
		}
		load := energyExplanationSeries{Stage: "load", Level: "load", Kind: "load.zone_cooling", ZoneName: zone, sourceName: "Zone Air System Sensible Cooling Energy"}
		q, _, _ := energyExplanationMultiplierForSeries(load, index)
		want := map[string]float64{"Natural": 1, "Intake": 4, "Exhaust": 8}[zone]
		if math.Abs(q-want) > 0 {
			t.Fatalf("separate canonical Zone load factor changed: %s %g", zone, q)
		}
	}
}

func TestEnergyPathSimpleVentilationSourceQualificationNeverPartiallyTruncates(t *testing.T) {
	scope := EnergyExplanationScope{Kind: "zone", ZoneName: "Intake"}
	source := EnergyDataSource{ID: "native", SourceType: "sql_report_data", Name: energyPathSimpleVentilationFanName, KeyValue: "Intake", ZoneName: "Intake", ReportingFrequency: "Monthly", SourceUnit: "J", NormalizedUnit: "kWh"}
	node := EnergyExplanationNode{ID: "direct", Level: "end_use", Kind: "energy.fans", EndUse: "fans", ZoneName: "Intake", Basis: "direct_zone_energy", SourceIDs: []string{"native"}}
	if !energyPathNodeHasOnlySimpleVentilationSources(node, []EnergyDataSource{source}, scope) {
		t.Fatal("exact source did not qualify")
	}
	foreign := source
	foreign.ID = "foreign"
	foreign.Name = "Fan Electricity Energy"
	node.SourceIDs = append(node.SourceIDs, "foreign")
	if energyPathNodeHasOnlySimpleVentilationSources(node, []EnergyDataSource{source, foreign}, scope) {
		t.Fatal("mixed source node was partially qualified")
	}
	node.SourceIDs = []string{"native"}
	if energyPathNodeHasOnlySimpleVentilationSources(node, []EnergyDataSource{source, source}, scope) {
		t.Fatal("duplicate source ID qualified")
	}
	source.KeyValue = "Natural"
	if energyPathNodeHasOnlySimpleVentilationSources(node, []EnergyDataSource{source}, scope) {
		t.Fatal("foreign Zone source qualified")
	}
}
