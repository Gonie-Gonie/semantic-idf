package simulation

// Hand-authored boundaries; no candidate-derived numerical expectations.
import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func storageBoundaryDraftDocument(t *testing.T) idf.Document {
	t.Helper()
	doc, err := idf.Parse(`
Version,25.1;
ElectricLoadCenter:Distribution, DC Center, PV List, TrackElectrical,,,,DirectCurrentWithInverterDCStorage, PV Inverter, Battery,,,;
ElectricLoadCenter:Generators, PV List, PV One, Generator:Photovoltaic, 1000,,;
Generator:Photovoltaic, PV One, Roof, PhotovoltaicPerformance:Simple, Performance, Decoupled;
PhotovoltaicPerformance:Simple, Performance, .5, Fixed, .2;
BuildingSurface:Detailed, Roof, Roof, Construction, Zone, ,Outdoors;
ElectricLoadCenter:Inverter:LookUpTable, PV Inverter, Always On,,0,1000,100,.9,.9,.9,.9,.9,.9;
ElectricLoadCenter:Storage:Battery, Battery, Always On,,0;
`)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
func storageBoundaryDraftClone(doc idf.Document) idf.Document {
	out := idf.Document{Objects: append([]idf.Object(nil), doc.Objects...)}
	for i := range out.Objects {
		out.Objects[i].Fields = append([]idf.Field(nil), out.Objects[i].Fields...)
	}
	return out
}
func storageBoundaryDraftObject(doc *idf.Document, kind string) *idf.Object {
	for i := range doc.Objects {
		if strings.EqualFold(doc.Objects[i].Type, kind) {
			return &doc.Objects[i]
		}
	}
	return nil
}
func storageBoundaryDraftSeries(key string, monthly map[int]float64) energyExplanationSeries {
	return canonicalEnergyExplanationSeries(energyExplanationSeries{Level: "energy", Kind: "energy.storage_charge", Label: "Storage charge", Carrier: "electricity", EndUse: "storage_charge", MeterHierarchyLevel: "broad_end_use", Unit: "kWh", SourceIDs: []string{"sql-rdd-51"}, sourceName: "Electric Storage Charge Energy", sourceKeyValue: key, sourceFrequency: "Monthly", Total: 10, Monthly: monthly})
}
func storageBoundaryDraftSources(key string) []EnergyDataSource {
	return []EnergyDataSource{{ID: "sql-rdd-51", SourceType: "sql_report_data", Name: "Electric Storage Charge Energy", KeyValue: key, Units: "J", SourceUnit: "J", ReportingFrequency: "Monthly"}}
}

func TestEnergyPathStorageChargeBoundaryOriginalDCFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/energy_path_real_models/models/25.1/ShopWithPVandBattery.idf")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "9c3f9b637b1e04c2e4b8911854c36ffd6442ea86cfe8e165fccaaeb8b2be9cd0" {
		t.Fatalf("original changed: %s", got)
	}
	doc, err := idf.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	inventory := energyPathBuildStorageChargeInventory(doc)
	if !inventory.HasOriginal || len(inventory.ByKey) != 1 {
		t.Fatalf("original storage inventory %#v", inventory)
	}
	b := energyPathStorageChargeBoundaryForKey(inventory, "KIBAM")
	if b == nil || b.State != energyPathStorageChargeNative || validateEnergyPathStorageChargeBoundary(b) != nil {
		t.Fatalf("original native boundary %#v", b)
	}
	counts := map[string]int{}
	for _, ref := range b.Objects {
		counts[ref.ObjectType]++
	}
	if len(b.Objects) != 2 || counts["ElectricLoadCenter:Storage:Battery"] != 1 || counts["ElectricLoadCenter:Distribution"] != 1 {
		t.Fatalf("native charge proof must not pretend to certify PV/inverter topology: %#v", counts)
	}
	for _, name := range []string{"Kibam", "PV Inverter"} {
		prefix := "ElectricLoadCenter:Storage:"
		if name == "PV Inverter" {
			prefix = "ElectricLoadCenter:Inverter:"
		}
		objects := energyPathStorageBoundaryObjects(doc, prefix, name)
		if len(objects) != 1 || energyPathStorageBoundaryField(objects[0], 2) != "" {
			t.Fatal("original blank Zone ownership changed")
		}
	}
}

func TestEnergyPathStorageChargeBoundaryAllNativeModelsAndBuses(t *testing.T) {
	for _, kind := range []string{"ElectricLoadCenter:Storage:Simple", "ElectricLoadCenter:Storage:Battery", "ElectricLoadCenter:Storage:LiIonNMCBattery"} {
		for _, bus := range []string{"AlternatingCurrentWithStorage", "DirectCurrentWithInverterACStorage", energyPathStorageChargeDCCanonicalBus} {
			t.Run(kind+"/"+bus, func(t *testing.T) {
				doc := storageBoundaryDraftDocument(t)
				storageBoundaryDraftObject(&doc, "ElectricLoadCenter:Storage:Battery").Type = kind
				center := storageBoundaryDraftObject(&doc, "ElectricLoadCenter:Distribution")
				center.Fields[6].Value = bus
				// These hand records prove reporting ownership, not engine-valid battery
				// parameters or an inverter equation. AC storage needs no PV/inverter.
				if bus == "AlternatingCurrentWithStorage" {
					center.Fields[1].Value = ""
					center.Fields[7].Value = ""
				}
				kept := []idf.Object{}
				for _, o := range doc.Objects {
					if o.Type == "Version" || o.Type == "ElectricLoadCenter:Distribution" || o.Type == kind {
						kept = append(kept, o)
					}
				}
				doc.Objects = kept
				inventory := energyPathBuildStorageChargeInventory(doc)
				item := storageBoundaryDraftSeries("BATTERY", map[int]float64{1: 10})
				actual, b := qualifyEnergyPathStorageChargeSeries(item, inventory, storageBoundaryDraftSources("Battery"))
				if b == nil || b.State != energyPathStorageChargeNative || b.BusType != bus || actual.Level != "support" || actual.Stage != "support" || actual.Total != item.Total || len(b.Objects) != 2 || validateEnergyPathStorageChargeBoundary(b) != nil {
					t.Fatalf("shared native reporting boundary rejected: %#v", b)
				}
			})
		}
	}
	// Other generator, PV integration, inverter and converter/transformer
	// choices cannot change the common storage Charge registration.
	doc := storageBoundaryDraftDocument(t)
	storageBoundaryDraftObject(&doc, "Generator:Photovoltaic").Fields[4].Value = "IntegratedSurfaceOutsideFace"
	storageBoundaryDraftObject(&doc, "ElectricLoadCenter:Inverter:LookUpTable").Type = "ElectricLoadCenter:Inverter:Simple"
	storageBoundaryDraftObject(&doc, "ElectricLoadCenter:Distribution").Fields[9].Value = "Transformer"
	converter := idf.Object{Type: "ElectricLoadCenter:Storage:Converter", Index: 100, Fields: []idf.Field{{Value: "Battery"}}}
	doc.Objects = append(doc.Objects, converter)
	b := energyPathStorageChargeBoundaryForKey(energyPathBuildStorageChargeInventory(doc), "Battery")
	if b == nil || b.State != energyPathStorageChargeNative {
		t.Fatal("unrelated electrical/thermal proof was required for the native reporting boundary")
	}
}

func TestEnergyPathStorageChargeBoundaryAmbiguityFailsClosed(t *testing.T) {
	base := storageBoundaryDraftDocument(t)
	mutations := []struct {
		name string
		edit func(*idf.Document)
	}{
		{"missing storage association", func(d *idf.Document) {
			storageBoundaryDraftObject(d, "ElectricLoadCenter:Distribution").Fields[8].Value = "Absent"
		}},
		{"unsupported native storage", func(d *idf.Document) {
			storageBoundaryDraftObject(d, "ElectricLoadCenter:Storage:Battery").Type = "ElectricLoadCenter:Storage:Unreviewed"
		}},
		{"duplicate battery", func(d *idf.Document) {
			object := *storageBoundaryDraftObject(d, "ElectricLoadCenter:Storage:Battery")
			object.Index = 100
			d.Objects = append(d.Objects, object)
		}},
		{"same-key different native model", func(d *idf.Document) {
			object := *storageBoundaryDraftObject(d, "ElectricLoadCenter:Storage:Battery")
			object.Index = 100
			object.Type = "ElectricLoadCenter:Storage:Simple"
			d.Objects = append(d.Objects, object)
		}},
		{"duplicate load-center name", func(d *idf.Document) {
			object := *storageBoundaryDraftObject(d, "ElectricLoadCenter:Distribution")
			object.Index = 100
			d.Objects = append(d.Objects, object)
		}},
		{"AC and DC reuse one battery", func(d *idf.Document) {
			object := *storageBoundaryDraftObject(d, "ElectricLoadCenter:Distribution")
			object.Fields = append([]idf.Field(nil), object.Fields...)
			object.Index = 100
			object.Fields[0].Value = "AC"
			object.Fields[6].Value = "AlternatingCurrentWithStorage"
			d.Objects = append(d.Objects, object)
		}},
		{"blank bus", func(d *idf.Document) {
			storageBoundaryDraftObject(d, "ElectricLoadCenter:Distribution").Fields[6].Value = ""
		}},
		{"unreviewed bus", func(d *idf.Document) {
			storageBoundaryDraftObject(d, "ElectricLoadCenter:Distribution").Fields[6].Value = "UnknownStorageBus"
		}},
		{"unreviewed version", func(d *idf.Document) { storageBoundaryDraftObject(d, "Version").Fields[0].Value = "22.1" }},
		{"missing version", func(d *idf.Document) { storageBoundaryDraftObject(d, "Version").Type = "OtherObject" }},
		{"duplicate version", func(d *idf.Document) {
			object := *storageBoundaryDraftObject(d, "Version")
			object.Index = 100
			d.Objects = append(d.Objects, object)
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			doc := storageBoundaryDraftClone(base)
			mutation.edit(&doc)
			b := energyPathStorageChargeBoundaryForKey(energyPathBuildStorageChargeInventory(doc), "BATTERY")
			if b == nil || b.State != energyPathStorageChargeUnresolved || !energyPathStorageChargeIsNonFlow(b) {
				t.Fatalf("bad native boundary became consumption authorization: %#v", b)
			}
		})
	}
}

func TestEnergyPathStorageChargeBoundaryMetadataDoesNotUseComments(t *testing.T) {
	doc := storageBoundaryDraftDocument(t)
	for i := range doc.Objects {
		for j := range doc.Objects[i].Fields {
			doc.Objects[i].Fields[j].Comment = "Electrical Buss Type: AlternatingCurrentWithStorage"
		}
	}
	b := energyPathStorageChargeBoundaryForKey(energyPathBuildStorageChargeInventory(doc), "Battery")
	if b == nil || b.State != energyPathStorageChargeNative {
		t.Fatal("editable comments changed native positional boundary")
	}
}

func TestEnergyPathStorageChargeBoundaryPreservesObservationsAndKnownness(t *testing.T) {
	inventory := energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t))
	for _, months := range []map[int]float64{nil, {}, {1: 0}, {1: .0000001}, {1: 3, 2: 7}} {
		item := storageBoundaryDraftSeries("Battery", months)
		original := item
		qualified, b := qualifyEnergyPathStorageChargeSeries(item, inventory, storageBoundaryDraftSources("Battery"))
		if b == nil || b.State != energyPathStorageChargeNative || qualified.Stage != "support" || qualified.Level != "support" {
			t.Fatal("missing explicit non-flow routing")
		}
		if qualified.Total != original.Total || qualified.RawTotal != original.RawTotal || !reflect.DeepEqual(qualified.Monthly, original.Monthly) || !reflect.DeepEqual(qualified.RawMonthly, original.RawMonthly) || !reflect.DeepEqual(qualified.SourceIDs, original.SourceIDs) {
			t.Fatal("boundary changed numerical authority/presence")
		}
		qualified.Stage, qualified.Level, qualified.ZoneName, qualified.SourceClass = original.Stage, original.Level, original.ZoneName, original.SourceClass
		if !reflect.DeepEqual(qualified, original) || !reflect.DeepEqual(item, original) {
			t.Fatal("boundary changed unrelated physical series")
		}
		b.SourceIDs[0] = "mutated"
		if item.SourceIDs[0] != "sql-rdd-51" {
			t.Fatal("boundary aliases source slice")
		}
	}
}

func TestEnergyPathStorageChargeBoundaryReportingIdentityIsNotPresenceOrConsumption(t *testing.T) {
	inventory := energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t))
	item := storageBoundaryDraftSeries("Battery", map[int]float64{1: 0})
	for _, frequency := range []string{"Monthly", "Hourly"} {
		t.Run(frequency, func(t *testing.T) {
			input := item
			input.sourceFrequency = frequency
			sources := storageBoundaryDraftSources("Battery")
			sources[0].ReportingFrequency = frequency
			actual, b := qualifyEnergyPathStorageChargeSeries(input, inventory, sources)
			if b == nil || b.State != energyPathStorageChargeNative || actual.SourceClass != "storage_charge_context" || !reflect.DeepEqual(actual.Monthly, input.Monthly) {
				t.Fatal("exact native frequency or known zero changed")
			}
			// No scalar is made known by this identity proof: the deliberately empty
			// source presence mask remains empty, distinct from a measured zero.
			if sources[0].observedValuePresence != 0 || sources[0].RawValue != 0 {
				t.Fatal("identity qualification fabricated an observation")
			}
		})
	}
	for _, mutation := range []struct {
		name string
		edit func([]EnergyDataSource) []EnergyDataSource
	}{
		{"absent", func(s []EnergyDataSource) []EnergyDataSource { return nil }},
		{"wrong ID", func(s []EnergyDataSource) []EnergyDataSource { s[0].ID = "sql-rdd-52"; return s }},
		{"wrong key", func(s []EnergyDataSource) []EnergyDataSource { s[0].KeyValue = "Another battery"; return s }},
		{"wrong name", func(s []EnergyDataSource) []EnergyDataSource {
			s[0].Name = "Electric Storage Discharge Energy"
			return s
		}},
		{"wrong unit", func(s []EnergyDataSource) []EnergyDataSource { s[0].SourceUnit = "W"; return s }},
		{"wrong frequency", func(s []EnergyDataSource) []EnergyDataSource { s[0].ReportingFrequency = "Hourly"; return s }},
		{"unknown frequency", func(s []EnergyDataSource) []EnergyDataSource { s[0].ReportingFrequency = "Invented"; return s }},
		{"meter", func(s []EnergyDataSource) []EnergyDataSource { s[0].IsMeter = true; return s }},
		{"tabular", func(s []EnergyDataSource) []EnergyDataSource { s[0].SourceType = "sql_tabular"; return s }},
		{"duplicate ID", func(s []EnergyDataSource) []EnergyDataSource { return append(s, s[0]) }},
		{"duplicate native identity", func(s []EnergyDataSource) []EnergyDataSource {
			other := s[0]
			other.ID = "sql-rdd-52"
			return append(s, other)
		}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			actual, b := qualifyEnergyPathStorageChargeSeries(item, inventory, mutation.edit(storageBoundaryDraftSources("Battery")))
			if b == nil || b.State != energyPathStorageChargeUnresolved || b.Reason != "unproved_native_charge_reporting_identity" || actual.Stage != "support" || actual.Level != "support" || actual.Total != item.Total || !reflect.DeepEqual(actual.Monthly, item.Monthly) {
				t.Fatalf("bad reporting proof restored consumption/changed value: %#v", b)
			}
		})
	}
	for _, name := range []string{"Electric Storage Discharge Energy", "Electric Storage Production Decrement Energy", "Inverter Ancillary AC Electricity Energy", "Converter Ancillary AC Electricity Energy", "Electricity:Facility"} {
		other := item
		other.SourceName = name
		other.sourceName = name
		actual, b := qualifyEnergyPathStorageChargeSeries(other, inventory, nil)
		if b != nil || !reflect.DeepEqual(actual, other) {
			t.Fatalf("charge-only rule changed distinct supply/consumption source %s", name)
		}
	}
}

func TestEnergyPathStorageChargeBoundaryLegacyAndMixedNativeKeysRemainDistinct(t *testing.T) {
	base := storageBoundaryDraftDocument(t)
	item := storageBoundaryDraftSeries("Battery", map[int]float64{1: 1})
	legacy := energyPathStorageChargeInventory{}
	actual, b := qualifyEnergyPathStorageChargeSeries(item, legacy, nil)
	if b != nil || !reflect.DeepEqual(actual, item) || energyPathStorageChargeSelectionSuffix(item, legacy) != "" {
		t.Fatal("EPATH111/no-original legacy changed")
	}
	for _, text := range []string{"", "! explicitly supplied comment-only original\n"} {
		empty, err := idf.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		inventory := energyPathBuildStorageChargeInventory(empty)
		actual, b := qualifyEnergyPathStorageChargeSeries(item, inventory, nil)
		if !inventory.HasOriginal || b == nil || b.State != energyPathStorageChargeUnresolved || actual.Level != "support" || actual.Stage != "support" || actual.Total != item.Total || energyPathStorageChargeSelectionSuffix(item, inventory) == "" {
			t.Fatal("explicit empty original became omitted-original consumption compatibility")
		}
	}
	doc := storageBoundaryDraftClone(base)
	ac := *storageBoundaryDraftObject(&doc, "ElectricLoadCenter:Distribution")
	ac.Fields = append([]idf.Field(nil), ac.Fields...)
	ac.Index = 100
	ac.Fields[0].Value = "AC Center"
	ac.Fields[1].Value = ""
	ac.Fields[6].Value = "AlternatingCurrentWithStorage"
	ac.Fields[7].Value = ""
	ac.Fields[8].Value = "AC Battery"
	battery := *storageBoundaryDraftObject(&doc, "ElectricLoadCenter:Storage:Battery")
	battery.Fields = append([]idf.Field(nil), battery.Fields...)
	battery.Index = 101
	battery.Fields[0].Value = "AC Battery"
	doc.Objects = append(doc.Objects, ac, battery)
	inventory := energyPathBuildStorageChargeInventory(doc)
	acOriginal := storageBoundaryDraftSeries("AC Battery", map[int]float64{1: 0, 2: 2})
	acOriginal.SourceIDs = []string{"sql-rdd-52"}
	acOriginal.AnnualSourceIDs = []string{"sql-rdd-52"}
	acOriginal.MonthlySourceIDs = []string{"sql-rdd-52"}
	acSource := storageBoundaryDraftSources("AC Battery")[0]
	acSource.ID = "sql-rdd-52"
	sources := append(storageBoundaryDraftSources("Battery"), acSource)
	dcSeries, dcBoundary := qualifyEnergyPathStorageChargeSeries(item, inventory, sources)
	acSeries, acBoundary := qualifyEnergyPathStorageChargeSeries(acOriginal, inventory, sources)
	if dcBoundary == nil || dcBoundary.State != energyPathStorageChargeNative || acBoundary == nil || acBoundary.State != energyPathStorageChargeNative || acSeries.Level != "support" || dcSeries.Level != "support" || acSeries.Total != acOriginal.Total || !reflect.DeepEqual(acSeries.Monthly, acOriginal.Monthly) {
		t.Fatal("separate AC/DC non-consumption boundaries were conflated")
	}
	if energyPathStorageChargeSelectionSuffix(dcSeries, inventory) == energyPathStorageChargeSelectionSuffix(acSeries, inventory) {
		t.Fatal("DC alias preference can steal AC energy")
	}
	if !reflect.DeepEqual(dcBoundary.SourceIDs, []string{"sql-rdd-51"}) || !reflect.DeepEqual(acBoundary.SourceIDs, []string{"sql-rdd-52"}) {
		t.Fatal("per-key reporting proof attached a foreign source")
	}
	unknown := energyPathStorageChargeBoundaryForKey(inventory, "unowned battery")
	if unknown == nil || unknown.State != energyPathStorageChargeUnresolved {
		t.Fatal("unknown native key was guessed consumption")
	}
	// Merely having an original, but no storage declaration, does not make an
	// otherwise exact native charge source into positive consumption evidence.
	missing := energyPathBuildStorageChargeInventory(idf.Document{Objects: []idf.Object{{Type: "Version", Fields: []idf.Field{{Value: "25.1"}}}}})
	if b := energyPathStorageChargeBoundaryForKey(missing, "Battery"); b == nil || b.State != energyPathStorageChargeUnresolved || len(b.Objects) != 0 {
		t.Fatal("missing owner invented an association or restored legacy consumption")
	}
	// The canonical support node may aggregate AC and DC quantities for display,
	// but must keep both per-key boundaries even when one month's value is zero.
	joined := unionEnergyPathStorageChargeBoundaries([]energyPathStorageChargeBoundary{*dcBoundary}, []energyPathStorageChargeBoundary{*acBoundary})
	if len(joined) != 2 || !energyPathStorageChargeBoundariesAreNonFlow(joined) {
		t.Fatal("canonical support merge lost native AC/DC constituents")
	}
	for reopen := 0; reopen < 2; reopen++ {
		data, err := json.Marshal(joined)
		if err != nil {
			t.Fatal(err)
		}
		var decoded []energyPathStorageChargeBoundary
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		annual := unionEnergyPathStorageChargeBoundaries(decoded, joined)
		if !reflect.DeepEqual(annual, joined) || len(annual) != 2 {
			t.Fatal("monthly/annual same-ID merge or reload erased a constituent")
		}
		joined = annual
	}
}

func TestEnergyPathStorageChargeBoundaryOriginalIndexPresenceAfterTwoJSON(t *testing.T) {
	base := energyPathStorageChargeBoundaryForKey(energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t)), "Battery")
	// Index zero is legitimate, but only when explicitly present and different
	// from the other original object's index. No original+offset inference.
	base.Objects[0].ObjectIndex = 0
	base.Objects[1].ObjectIndex = 1
	encoded, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		replace string
		valid   bool
	}{
		{"distinct zero", `"objectIndex":0`, true},
		{"missing", `"unrelatedField":0`, false},
		{"null", `"objectIndex":null`, false},
		{"negative", `"objectIndex":-1`, false},
		{"duplicate", `"objectIndex":1`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(strings.Replace(string(encoded), `"objectIndex":0`, tc.replace, 1))
			for reopen := 0; reopen < 2; reopen++ {
				var decoded *energyPathStorageChargeBoundary
				if err := json.Unmarshal(raw, &decoded); err != nil {
					t.Fatal(err)
				}
				if decoded == nil || !energyPathStorageChargeIsNonFlow(decoded) {
					t.Fatal("malformed present metadata disappeared into legacy")
				}
				if valid := validateEnergyPathStorageChargeBoundary(decoded) == nil; valid != tc.valid {
					t.Fatalf("index metadata validity=%v want=%v: %s", valid, tc.valid, raw)
				}
				if tc.name == "missing" || tc.name == "null" {
					if !decoded.Objects[0].objectIndexMissing {
						t.Fatal("missing index silently became original object zero")
					}
				}
				if tc.valid && (decoded.Objects[0].objectIndexMissing || decoded.Objects[0].ObjectIndex != 0) {
					t.Fatal("explicit original index zero lost presence")
				}
				links := []EnergyPathLink{{FromID: "charge", ToID: "carrier", Relation: "end_use_to_carrier", FromValue: 10, ToValue: 10}}
				if len(filterEnergyPathStorageChargeLinks(links, map[string]*energyPathStorageChargeBoundary{"charge": decoded})) != 0 || links[0].FromValue != 10 {
					t.Fatal("invalid metadata restored a flow or changed the observed input")
				}
				raw, err = json.Marshal(decoded)
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestEnergyPathStorageChargeBoundaryBothFlowDirectionsDeniedAfterJSON(t *testing.T) {
	inventory := energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t))
	b := energyPathStorageChargeBoundaryForKey(inventory, "Battery")
	legacy := []EnergyExplanationEdge{
		{ID: "consumption", FromID: "carrier", ToID: "charge", Relation: "end_use", Value: 10},
		{ID: "invented production", FromID: "carrier", ToID: "charge", Relation: "onsite_production", Value: 10},
		{ID: "real cooling", FromID: "carrier", ToID: "cooling", Relation: "end_use", Value: 30},
		{ID: "real purchased", FromID: "carrier", ToID: "purchased", Relation: "purchased_electricity", Value: 40},
	}
	links := []EnergyPathLink{
		{ID: "consumption", FromID: "charge", ToID: "carrier", Relation: "end_use_to_carrier", FromValue: 10, ToValue: 10},
		{ID: "invented production", FromID: "charge", ToID: "carrier", Relation: "support_supply", FromValue: 10, ToValue: 10},
		{ID: "real cooling", FromID: "cooling", ToID: "carrier", Relation: "end_use_to_carrier", FromValue: 30, ToValue: 30},
		{ID: "real purchased", FromID: "purchased", ToID: "carrier", Relation: "support_supply", FromValue: 40, ToValue: 40},
	}
	for i := 0; i < 3; i++ {
		boundaries := map[string]*energyPathStorageChargeBoundary{"charge": b}
		if !reflect.DeepEqual(filterEnergyPathStorageChargeLegacyEdges(legacy, boundaries), legacy[2:]) || !reflect.DeepEqual(filterEnergyPathStorageChargeLinks(links, boundaries), links[2:]) {
			t.Fatal("non-flow boundary lost or unrelated consumption/supply modified")
		}
		// No SourceIDs appear on any edge: missing provenance cannot bypass deny.
		encoded, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		var decoded energyPathStorageChargeBoundary
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		b = &decoded
		if err := validateEnergyPathStorageChargeBoundary(b); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(filterEnergyPathStorageChargeLegacyEdges(legacy, nil), legacy) || !reflect.DeepEqual(filterEnergyPathStorageChargeLinks(links, nil), links) {
		t.Fatal("legacy path changed without native proof")
	}
	b.State = "unknown future state"
	b.SourceIDs = nil
	if validateEnergyPathStorageChargeBoundary(b) == nil || !energyPathStorageChargeIsNonFlow(b) {
		t.Fatal("malformed explicit boundary restored a positive flow assumption")
	}
}

func TestEnergyPathStorageChargeBoundaryStableCloneAndNilWire(t *testing.T) {
	b := energyPathStorageChargeBoundaryForKey(energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t)), "Battery")
	before := cloneEnergyPathStorageChargeBoundary(b)
	duplicate := append(append([]energyPathStorageBoundaryObject(nil), b.Objects...), b.Objects...)
	if !reflect.DeepEqual(energyPathStableStorageBoundaryObjects(duplicate), b.Objects) {
		t.Fatal("duplicate evidence is unstable")
	}
	clone := cloneEnergyPathStorageChargeBoundary(b)
	clone.Objects[0].ObjectName = "changed"
	clone.SourceIDs = append(clone.SourceIDs, "sql-rdd-999")
	if !reflect.DeepEqual(b, before) {
		t.Fatal("metadata clone mutated original")
	}
	wire := struct {
		Value    float64                          `json:"value"`
		Boundary *energyPathStorageChargeBoundary `json:"storageChargeBoundary,omitempty"`
	}{Value: 0}
	encoded, err := json.Marshal(wire)
	if err != nil || string(encoded) != "{\"value\":0}" {
		t.Fatalf("nil extension changed legacy byte shape: %s %v", encoded, err)
	}
}

func TestEnergyPathStorageChargeBoundaryUnionAndIDsKeepConstituents(t *testing.T) {
	b := energyPathStorageChargeBoundaryForKey(energyPathBuildStorageChargeInventory(storageBoundaryDraftDocument(t)), "Battery")
	b.SourceIDs = []string{"sql-rdd-52"}
	later := cloneEnergyPathStorageChargeBoundary(b)
	later.SourceIDs = []string{"sql-rdd-51"}
	left, right := []energyPathStorageChargeBoundary{*b}, []energyPathStorageChargeBoundary{*later}
	joined := unionEnergyPathStorageChargeBoundaries(left, right)
	if !reflect.DeepEqual(joined, unionEnergyPathStorageChargeBoundaries(right, left)) || len(joined) != 1 || !reflect.DeepEqual(joined[0].SourceIDs, []string{"sql-rdd-51", "sql-rdd-52"}) {
		t.Fatal("monthly/annual evidence union is not stable")
	}
	joined[0].Objects[0].ObjectName = "changed"
	if left[0].Objects[0].ObjectName == "changed" || right[0].Objects[0].ObjectName == "changed" {
		t.Fatal("evidence union mutated input")
	}
	if unionEnergyPathStorageChargeBoundaries(nil, nil) != nil || energyPathStorageChargeBoundariesAreNonFlow(nil) {
		t.Fatal("nil changes legacy semantics")
	}
	if !energyPathStorageChargeBoundariesAreNonFlow([]energyPathStorageChargeBoundary{{}}) {
		t.Fatal("invalid present metadata became authorization")
	}
	one, two := cloneEnergyPathStorageChargeBoundary(b), cloneEnergyPathStorageChargeBoundary(b)
	one.SourceKey = "A-B"
	two.SourceKey = "A_B"
	item := storageBoundaryDraftSeries("Battery", nil)
	if energyPathStorageChargeNodeID(item, one) == energyPathStorageChargeNodeID(item, two) {
		t.Fatal("lossy source-key slug merged different storage")
	}
}
