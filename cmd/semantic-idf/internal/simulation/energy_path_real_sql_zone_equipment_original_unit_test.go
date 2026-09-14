package simulation

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLZoneGroupOriginalUnit(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "MultiStory.idf"))
	if err != nil {
		t.Fatal(err)
	}
	if epathRealHash(data) != "c7165328a5f3a90aa81ac3f95928600079cf9f7a109fd615ccc6686caf518ba2" {
		t.Fatal("original MultiStory bytes changed")
	}
	return string(data)
}

func TestEnergyPathRealSQLOriginalZoneGroupFactorsMatchSeparately(t *testing.T) {
	original := epathSQLZoneGroupOriginalUnit(t)
	want := map[string]epathSQLOriginalZoneFactors{
		"gnd west zone": {1, 1}, "gnd center zone": {4, 1}, "gnd east zone": {1, 1},
		"mid west zone": {1, 8}, "mid center zone": {4, 8}, "mid east zone": {1, 8},
		"top west zone": {1, 1}, "top center zone": {4, 1}, "top east zone": {1, 1},
	}
	got, err := epathSQLOriginalZoneMultipliers(original)
	if err != nil || len(got) != len(want) {
		t.Fatalf("original proof: %v %v", got, err)
	}
	for zone, factors := range want {
		if got[zone] != factors {
			t.Fatalf("%s original factors %v, want %v", zone, got[zone], factors)
		}
	}
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE Zones(ZoneName TEXT,Multiplier REAL,ListMultiplier REAL)`); err != nil {
		t.Fatal(err)
	}
	for zone, factors := range want {
		if _, err := db.Exec(`INSERT INTO Zones VALUES(?,?,?)`, strings.ToUpper(zone), factors.Zone, factors.List); err != nil {
			t.Fatal(err)
		}
	}
	if err := epathSQLValidateOriginalZoneMultipliers(db, original, epathSQLOriginalMultiplierContract); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE Zones SET Multiplier=8,ListMultiplier=4 WHERE ZoneName='MID CENTER ZONE'`); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLValidateOriginalZoneMultipliers(db, original, epathSQLOriginalMultiplierContract); err == nil {
		t.Fatal("equal product concealed exchanged original factors")
	}
	if err := epathSQLValidateOriginalZoneMultipliers(db, "", epathSQLOriginalMultiplierContract); err == nil {
		t.Fatal("missing original silently waived declared proof")
	}
	if err := epathSQLValidateOriginalZoneMultipliers(db, "", ""); err != nil {
		t.Fatal("legacy frames without declared proof were changed")
	}
	for _, extra := range []string{
		"ZoneGroup,Other,Mid Floor List,8;",
		"ZoneList,OtherList,Mid Center Zone;ZoneGroup,Other,OtherList,2;",
		"ZoneList,ForeignList,Missing;ZoneGroup,Other,ForeignList,2;",
		"ZoneGroup,Bad,MissingList,2;",
		"ZoneList,ZeroList,Top West Zone;ZoneGroup,Zero,ZeroList,0;",
	} {
		if _, err := epathSQLOriginalZoneMultipliers(original + extra); err == nil {
			t.Fatalf("accepted ambiguous factors: %s", extra)
		}
	}
}

func TestEnergyPathRealSQLOriginalWindowACAndConvectiveOwners(t *testing.T) {
	original := epathSQLZoneGroupOriginalUnit(t)
	doc, err := idf.Parse(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, floor := range []string{"Gnd", "Mid", "Top"} {
		for _, position := range []string{"West", "Center", "East"} {
			prefix := floor + " " + position
			baseboard := epathRealSQLDirectHVACOwner{ZoneName: prefix + " Zone", EquipmentType: epathSQLConvectiveBaseboardType, EquipmentName: prefix + " Baseboard", ComponentType: epathSQLConvectiveBaseboardType, KeyValue: prefix + " Baseboard"}
			if err := epathSQLValidateConvectiveBaseboardOriginalOwner(doc, baseboard); err != nil {
				t.Fatalf("%s: %v", prefix, err)
			}
			if err := epathSQLValidateNativeBaseboardOriginalOwner(doc, baseboard); err == nil {
				t.Fatal("convective type passed radiant recipient validator")
			}
			for _, kind := range []string{"Coil:Cooling:DX:SingleSpeed", "Fan:OnOff"} {
				name := prefix + " OA DX Coil"
				if kind == "Fan:OnOff" {
					name = prefix + " OA Fan"
				}
				owner := epathRealSQLDirectHVACOwner{ZoneName: prefix + " Zone", EquipmentType: epathSQLWindowACType, EquipmentName: prefix + " Window AC", ComponentType: kind, KeyValue: name}
				if err := epathSQLValidateWindowACOriginalOwner(doc, owner); err != nil {
					t.Fatalf("%s/%s: %v", prefix, kind, err)
				}
				owner.ZoneName = "Foreign Zone"
				if err := epathSQLValidateWindowACOriginalOwner(doc, owner); err == nil {
					t.Fatal("Window AC source borrowed foreign Zone")
				}
			}
		}
	}
	owner := epathRealSQLDirectHVACOwner{ZoneName: "Gnd West Zone", EquipmentType: epathSQLWindowACType, EquipmentName: "Gnd West Window AC", ComponentType: "Coil:Cooling:DX:SingleSpeed", KeyValue: "Gnd West OA DX Coil"}
	for _, typ := range []string{"Coil:Heating:Fuel", "Coil:Heating:Water", "Coil:Cooling:Water"} {
		valid, err := idf.Parse(original + typ + ",Gnd West OA DX Coil;")
		if err != nil {
			t.Fatal(err)
		}
		if err := epathSQLValidateWindowACOriginalOwner(valid, owner); err != nil {
			t.Fatalf("different output family was falsely a reporting collision: %v", err)
		}
	}
	for _, typ := range []string{"Coil:Cooling:DX:VariableSpeed", "Coil:Cooling:DX:SingleSpeed:ThermalStorage", "Coil:Cooling:WaterToAirHeatPump:EquationFit", "Coil:Cooling:DX"} {
		invalid, err := idf.Parse(original + typ + ",Gnd West OA DX Coil;")
		if err != nil {
			t.Fatal(err)
		}
		if err := epathSQLValidateWindowACOriginalOwner(invalid, owner); err == nil {
			t.Fatal("same-output physical type reporting collision was hidden")
		}
	}
	for _, mutation := range []struct {
		typ   string
		index int
		value string
	}{
		{epathSQLWindowACType, 10, "Coil:Heating:Fuel"},
		{epathSQLWindowACType, 11, "Gnd Center OA DX Coil"},
		{epathSQLWindowACType, 5, "Foreign outlet"},
		{epathSQLWindowACType, 13, "DrawThrough"},
		{"Fan:OnOff", 7, "Foreign fan inlet"},
	} {
		changed, err := idf.Parse(original)
		if err != nil {
			t.Fatal(err)
		}
		for i := range changed.Objects {
			if strings.EqualFold(changed.Objects[i].Type, mutation.typ) && strings.HasPrefix(epathSQLBaseboardField(changed.Objects[i], 0), "Gnd West") {
				changed.Objects[i].Fields[mutation.index].Value = mutation.value
				break
			}
		}
		if err := epathSQLValidateWindowACOriginalOwner(changed, owner); err == nil {
			t.Fatalf("accepted broken native topology: %+v", mutation)
		}
	}
}

func TestEnergyPathRealSQLConvectiveOriginalDefaultsAndSelectedCapacity(t *testing.T) {
	owner := epathRealSQLDirectHVACOwner{ZoneName: "Office", EquipmentType: epathSQLConvectiveBaseboardType, EquipmentName: "Baseboard", ComponentType: epathSQLConvectiveBaseboardType, KeyValue: "Baseboard"}
	wrap := func(fields string) idf.Document {
		text := "Zone,Office;ZoneHVAC:Baseboard:Convective:Electric," + fields + ";ZoneHVAC:EquipmentList,Equipment,SequentialLoad,ZoneHVAC:Baseboard:Convective:Electric,Baseboard,1,1,,;ZoneHVAC:EquipmentConnections,Office,Equipment;"
		doc, err := idf.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	for _, fields := range []string{
		"Baseboard,,HeatingDesignCapacity,100",
		"Baseboard,,,Autosize",
		"Baseboard,,,0,,,",
		"Baseboard,,heatingdesigncapacity,aUtOsIzE,,,",
		"Baseboard,,capacityperfloorarea,,10,,",
		"Baseboard,,fractionofautosizedheatingcapacity,,,0,",
		"Baseboard,,FractionOfAutosizedHeatingCapacity,,,1,0.95",
		"Baseboard,,,1.2e3,,,9.5e-1",
	} {
		if err := epathSQLValidateConvectiveBaseboardOriginalOwner(wrap(fields), owner); err != nil {
			t.Fatalf("valid native default %s: %v", fields, err)
		}
	}
	for _, fields := range []string{
		"Baseboard,,HeatingDesignCapacity,,,,",
		"Baseboard,,Unknown,100,,,1",
		"Baseboard,,HeatingDesignCapacity,-1,,,1",
		"Baseboard,,CapacityPerFloorArea,,0,,1",
		"Baseboard,,CapacityPerFloorArea,100,,,1",
		"Baseboard,,FractionOfAutosizedHeatingCapacity,,,-1,1",
		"Baseboard,,FractionOfAutosizedHeatingCapacity,,,Autosize,1",
		"Baseboard,,HeatingDesignCapacity,100,,,0",
		"Baseboard,,HeatingDesignCapacity,100,,,NaN",
		"Baseboard,,HeatingDesignCapacity,100,,,1,0.2",
		"Baseboard,,,0x1p2,,,1",
		"Baseboard,,,1_000,,,1",
		"Baseboard,,,100,,,0x1p0",
		"Baseboard,,,100,,,1_0e-1",
	} {
		if err := epathSQLValidateConvectiveBaseboardOriginalOwner(wrap(fields), owner); err == nil {
			t.Fatalf("invalid native default accepted: %s", fields)
		}
	}
}

func TestEnergyPathRealSQLOriginalZoneGroupDefaultOne(t *testing.T) {
	for _, group := range []string{"ZoneGroup,Group,List;", "ZoneGroup,Group,List,;", "ZoneGroup,Group,List,1;"} {
		factors, err := epathSQLOriginalZoneMultipliers("Zone,Office,,,,,,4;ZoneList,List,Office;" + group)
		if err != nil || factors["office"] != (epathSQLOriginalZoneFactors{Zone: 4, List: 1}) {
			t.Fatalf("native default factor: %s %+v %v", group, factors, err)
		}
	}
	for _, value := range []string{"0", "-1", "NaN", "Inf", "1.5", "invalid"} {
		if _, err := epathSQLOriginalZoneMultipliers("Zone,Office;ZoneList,List,Office;ZoneGroup,Group,List," + value + ";"); err == nil {
			t.Fatalf("invalid original Group factor accepted: %s", value)
		}
	}
}

func TestEnergyPathRealSQLConvectiveContextUsesTypedNonRadiantBoundary(t *testing.T) {
	for _, frequency := range []string{"Monthly", "Hourly"} {
		path, model, plan, observed := epathSQLBaseboardContextUnit(t, "Baseboard Total Heating Energy", frequency, func(int) float64 { return 9.5 })
		model.BaseboardContexts[0].OwnerType = epathSQLConvectiveBaseboardType
		original := "Zone,Office;ZoneHVAC:Baseboard:Convective:Electric,Baseboard,,HeatingDesignCapacity,Autosize,,,0.95;ZoneHVAC:EquipmentList,Equipment,SequentialLoad,ZoneHVAC:Baseboard:Convective:Electric,Baseboard,1,1,,;ZoneHVAC:EquipmentConnections,Office,Equipment;"
		frames := epathSQLBaseboardContextUnitFrames()
		frames.Zones["office"] = epathSQLZone{Name: "Office", Multiplier: 32}
		if err := epathCompileSQLBaseboardContexts(path, original, &plan, observed, model, &frames); err != nil {
			t.Fatal(err)
		}
		identity := frames.BaseboardContextIdentities[40]
		if identity.OwnerType != epathSQLConvectiveBaseboardType || identity.RecipientQualification != nil {
			t.Fatal("convective context acquired radiant recipients")
		}
		count := 12.0
		if frequency == "Hourly" {
			count = 8760
		}
		if identity.ReportedScalar != 9.5*count {
			t.Fatal("native model-total context multiplied by ZoneGroup again")
		}
		identity.RecipientQualification = &epathSQLBaseboardRecipientQualification{}
		if err := epathSQLValidateBaseboardContextIdentity(identity); err == nil {
			t.Fatal("convective source accepted fabricated radiant qualification")
		}
	}
}
