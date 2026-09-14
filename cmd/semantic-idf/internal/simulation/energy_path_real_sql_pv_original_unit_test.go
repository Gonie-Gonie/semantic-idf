package simulation

// Test-only finite original proof. No SQL/candidate/expected scalar is used here.

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLPVOriginalDeclaration() epathRealSQLPVSystem {
	return epathRealSQLPVSystem{ID: "shop-pv-dc-storage", LoadCenterName: "PV Array Load Center", GeneratorListName: "Generator List", InverterName: "PV Inverter", StorageName: "Kibam", PerformanceName: "20percentEffPVhalfArea", Generators: []epathRealSQLPVGenerator{
		{Name: "PV:ZN_1_FLR_1_SEC_1_Ceiling", SurfaceName: "ZN_1_FLR_1_SEC_1_Ceiling", ZoneName: "ZN_1_FLR_1_SEC_1"},
		{Name: "PV:ZN_1_FLR_1_SEC_2_Ceiling", SurfaceName: "ZN_1_FLR_1_SEC_2_Ceiling", ZoneName: "ZN_1_FLR_1_SEC_2"},
		{Name: "PV:ZN_1_FLR_1_SEC_3_Ceiling", SurfaceName: "ZN_1_FLR_1_SEC_3_Ceiling", ZoneName: "ZN_1_FLR_1_SEC_3"},
		{Name: "PV:ZN_1_FLR_1_SEC_4_Ceiling", SurfaceName: "ZN_1_FLR_1_SEC_4_Ceiling", ZoneName: "ZN_1_FLR_1_SEC_4"},
		{Name: "PV:ZN_1_FLR_1_SEC_5_Ceiling", SurfaceName: "ZN_1_FLR_1_SEC_5_Ceiling", ZoneName: "ZN_1_FLR_1_SEC_5"},
	}}
}

func epathSQLPVOriginalFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/energy_path_real_models/models/25.1/ShopWithPVandBattery.idf")
	if err != nil {
		t.Fatal(err)
	}
	if s := fmt.Sprintf("%x", sha256.Sum256(b)); s != "9c3f9b637b1e04c2e4b8911854c36ffd6442ea86cfe8e165fccaaeb8b2be9cd0" {
		t.Fatalf("PV original hash changed %s", s)
	}
	return string(b)
}

func epathSQLPVUnitObject(t *testing.T, doc *idf.Document, typ, name string) *idf.Object {
	t.Helper()
	for i := range doc.Objects {
		o := &doc.Objects[i]
		if strings.EqualFold(o.Type, typ) && epathSQLSharedSame(epathSQLSharedField(*o, 0), name) {
			return o
		}
	}
	t.Fatalf("PV missing unit object %s/%s", typ, name)
	return nil
}

func TestEnergyPathSQLPVOriginalExactFiniteBoundary(t *testing.T) {
	original := epathSQLPVOriginalFixture(t)
	d := epathSQLPVOriginalDeclaration()
	p, err := epathSQLValidatePVOriginal(original, []epathRealSQLPVSystem{d})
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 1 || len(p[0].Owners) != 8 || len(p[0].PhysicalObjects) != 23 || !reflect.DeepEqual(p[0].Declaration, d) {
		t.Fatalf("PV physical proof incomplete: %#v", p)
	}
	for _, o := range p[0].Owners {
		if o.ObjectIndex < 0 || len(o.Fields) == 0 || o.ObjectName != o.Fields[0] {
			t.Fatalf("PV lost exact original owner: %#v", o)
		}
	}
	d.Generators[0].Name = "caller mutation"
	if p[0].Declaration.Generators[0].Name == d.Generators[0].Name {
		t.Fatal("PV proof aliases caller declaration")
	}
	if absent, err := epathSQLValidatePVOriginal("", nil); err != nil || absent != nil {
		t.Fatal("non-PV oracle acquired extra applicability")
	}
	if _, err := epathSQLValidatePVOriginal("", []epathRealSQLPVSystem{epathSQLPVOriginalDeclaration()}); err == nil {
		t.Fatal("present declaration accepted missing original")
	}
}

func TestEnergyPathSQLPVOriginalMutationDenials(t *testing.T) {
	original := epathSQLPVOriginalFixture(t)
	d := epathSQLPVOriginalDeclaration()
	type mutation struct {
		name  string
		typ   string
		owner string
		field int
		value string
	}
	cases := []mutation{
		{"AC changes this closure only", "ElectricLoadCenter:Distribution", d.LoadCenterName, 6, "AlternatingCurrentWithStorage"},
		{"wrong inverter", "ElectricLoadCenter:Distribution", d.LoadCenterName, 7, "missing"},
		{"blank storage selector", "ElectricLoadCenter:Distribution", d.LoadCenterName, 8, ""},
		{"transformer added", "ElectricLoadCenter:Distribution", d.LoadCenterName, 9, "transformer"},
		{"converter added", "ElectricLoadCenter:Distribution", d.LoadCenterName, 12, "converter"},
		{"battery Zone heat", "ElectricLoadCenter:Storage:Battery", d.StorageName, 2, d.Generators[0].ZoneName},
		{"inverter Zone heat", "ElectricLoadCenter:Inverter:LookUpTable", d.InverterName, 2, d.Generators[0].ZoneName},
		{"bad battery curve", "ElectricLoadCenter:Storage:Battery", d.StorageName, 12, "missing curve"},
		{"missing PV tuple", "ElectricLoadCenter:Generators", d.GeneratorListName, 1, "hidden"},
		{"wrong tuple type", "ElectricLoadCenter:Generators", d.GeneratorListName, 2, "Generator:WindTurbine"},
		{"integrated PV thermal", "Generator:Photovoltaic", d.Generators[0].Name, 4, "IntegratedSurfaceOutsideFace"},
		{"wrong PV surface", "Generator:Photovoltaic", d.Generators[0].Name, 1, d.Generators[1].SurfaceName},
		{"wrong surface Zone", "BuildingSurface:Detailed", d.Generators[0].SurfaceName, 3, d.Generators[1].ZoneName},
		{"changed multiplier", "Zone", d.Generators[0].ZoneName, 6, "3"},
		{"wrong version", "Version", "25.1", 0, "24.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := idf.Parse(original)
			if err != nil {
				t.Fatal(err)
			}
			o := epathSQLPVUnitObject(t, &doc, tc.typ, tc.owner)
			o.Fields[tc.field].Value = tc.value
			if _, err := epathSQLValidatePVOriginal(doc.String(), []epathRealSQLPVSystem{d}); err == nil {
				t.Fatal("mutated original accepted")
			}
		})
	}
	for _, tc := range []struct{ name, typ string }{{"same key other battery", "ElectricLoadCenter:Storage:Simple"}, {"duplicate native battery", "ElectricLoadCenter:Storage:Battery"}, {"hidden generator", "Generator:WindTurbine"}, {"blank unrelated distribution", "ElectricLoadCenter:Distribution"}, {"cross-type duplicate PV surface", "Shading:Building:Detailed"}} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := idf.Parse(original)
			if err != nil {
				t.Fatal(err)
			}
			name := d.StorageName
			if tc.typ == "ElectricLoadCenter:Distribution" {
				name = ""
			}
			if tc.typ == "Shading:Building:Detailed" {
				name = d.Generators[0].SurfaceName
			}
			doc.Objects = append(doc.Objects, idf.Object{Type: tc.typ, Fields: []idf.Field{{Value: name}}})
			if _, err := epathSQLValidatePVOriginal(doc.String(), []epathRealSQLPVSystem{d}); err == nil {
				t.Fatal("hidden/duplicate owner accepted")
			}
		})
	}
	for _, fieldCount := range []int{0, 1, 8, 14} {
		t.Run(fmt.Sprintf("truncated distribution %d", fieldCount), func(t *testing.T) {
			doc, err := idf.Parse(original)
			if err != nil {
				t.Fatal(err)
			}
			o := epathSQLPVUnitObject(t, &doc, "ElectricLoadCenter:Distribution", d.LoadCenterName)
			o.Fields = o.Fields[:fieldCount]
			if _, err := epathSQLValidatePVOriginal(doc.String(), []epathRealSQLPVSystem{d}); err == nil {
				t.Fatal("truncated distribution accepted")
			}
		})
	}
	bad := epathSQLPVOriginalDeclaration()
	bad.Generators[1] = bad.Generators[0]
	if _, err := epathSQLValidatePVOriginal(original, []epathRealSQLPVSystem{bad}); err == nil {
		t.Fatal("duplicate declaration accepted")
	}
}

func TestEnergyPathSQLPVExecutedOwnerBinding(t *testing.T) {
	original := epathSQLPVOriginalFixture(t)
	d := epathSQLPVOriginalDeclaration()
	proofs, err := epathSQLValidatePVOriginal(original, []epathRealSQLPVSystem{d})
	if err != nil {
		t.Fatal(err)
	}
	p := proofs[0]
	// A new harmless output object shifts every owner; original index +/- one
	// is not authority. Parsing each actual document supplies the index.
	executed := "Output:Variable,*,Site Outdoor Air Drybulb Temperature,Hourly;\n" + original
	owners, err := epathSQLBindPVExecuted(original, executed, p)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := idf.Parse(executed)
	if err != nil {
		t.Fatal(err)
	}
	for i, o := range owners {
		want := epathSQLPVUnitObject(t, &doc, o.ObjectType, o.ObjectName)
		if o.ObjectIndex != want.Index || o.ObjectIndex == p.Owners[i].ObjectIndex || !reflect.DeepEqual(o.Fields, p.Owners[i].Fields) {
			t.Fatalf("PV execution identity mismatch: %#v", o)
		}
	}
	bat := epathSQLPVUnitObject(t, &doc, "ElectricLoadCenter:Storage:Battery", d.StorageName)
	bat.Fields[6].Value = "999"
	if _, err := epathSQLBindPVExecuted(original, doc.String(), p); err == nil {
		t.Fatal("changed battery Ah capacity accepted across source binding")
	}
	p.Owners[1].ObjectIndex++
	if _, err := epathSQLBindPVExecuted(original, executed, p); err == nil {
		t.Fatal("forged original owner stamp accepted")
	}
	if _, err := epathSQLValidatePVOriginal("! misleading AC Storage and integrated PV comment\n"+original, []epathRealSQLPVSystem{d}); err != nil {
		t.Fatalf("comments changed physical classification: %v", err)
	}
}
