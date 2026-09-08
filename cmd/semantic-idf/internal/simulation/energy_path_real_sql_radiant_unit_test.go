package simulation

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLRadiantHandFixture() (string, epathRealSQLRadiantLoadBinding) {
	// Literal independent IDF. No production owner helper, purpose builder,
	// classifier, canonical load selector, multiplier or graph is called.
	text := `Version,25.1;
Zone,Office,0,0,0,0,1,7;
Zone,Other,0,0,0,0,1,1;
ZoneList,Repeated Offices,Office;
ZoneGroup,Office Group,Repeated Offices,3;
BuildingSurface:Detailed,Floor,Floor,Slab,Office,,Ground,,NoSun,NoWind,1,3,0,0,0,1,0,0,0,1,0;
BuildingSurface:Detailed,Other Floor,Floor,Slab,Other,,Ground,,NoSun,NoWind,1,3,0,0,0,1,0,0,0,1,0;
ZoneHVAC:EquipmentConnections,Office,List,,,Office Air,;
ZoneHVAC:EquipmentList,List,SequentialLoad,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,Radiant,1,1,,;
ZoneHVAC:LowTemperatureRadiant:ConstantFlow,Radiant,Design,Always,Office,Floor,400,.0004,,75000,50,HW In,HW Out,H1,H2,H3,H4,CW In,CW Out,C1,C2,C3,C4,,;
ZoneHVAC:LowTemperatureRadiant:ConstantFlow:Design,Design,ConvectionOnly,.012,.016,.35,MeanAirTemperature,.8,.87,.1,SimpleOff,1;
Branch,Heating Branch,,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,Radiant,HW In,HW Out;
Branch,Cooling Branch,,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,Radiant,CW In,CW Out;`
	binding := epathRealSQLRadiantLoadBinding{Frequency: "Monthly", AggregationBasis: "model_total", ThermalBoundary: "active_surface_source", Owners: []epathRealSQLRadiantOwner{{EquipmentType: "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", EquipmentName: "Radiant", DesignName: "Design", SurfaceName: "Floor", ZoneName: "Office"}}}
	return text, binding
}

func epathSQLRadiantUnitObject(t *testing.T, doc *idf.Document, kind, name string) *idf.Object {
	t.Helper()
	for i := range doc.Objects {
		if doc.Objects[i].Type == kind && len(doc.Objects[i].Fields) > 0 && doc.Objects[i].Fields[0].Value == name {
			return &doc.Objects[i]
		}
	}
	t.Fatalf("missing original unit object %s/%s", kind, name)
	return nil
}

func TestEnergyPathRealSQLRadiantOriginalAndHandOwnership(t *testing.T) {
	text, binding := epathSQLRadiantHandFixture()
	owners, err := epathSQLRadiantOriginalOwners(text, binding)
	if err != nil || len(owners) != 1 {
		t.Fatalf("independent hand ownership: %v %#v", err, owners)
	}
	owner := owners["radiant"]
	if owner.Owner.ZoneName != "Office" || owner.Owner.SurfaceName != "Floor" || owner.ZoneMultiplier != 7 || owner.ZoneListMultiplier != 3 || owner.EquipmentIndex == owner.SurfaceIndex || owner.DesignIndex == owner.ZoneIndex {
		t.Fatalf("hand original typed owner/multiplier changed: %#v", owner)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "RadLoTempCFloHeatCool.idf"))
	if err != nil {
		t.Fatal(err)
	}
	binding.Owners = []epathRealSQLRadiantOwner{
		{EquipmentType: "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", EquipmentName: "West Zone Radiant Floor", DesignName: "West Zone Radiant Floor Design", SurfaceName: "Zn001:Flr001", ZoneName: "West Zone"},
		{EquipmentType: "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", EquipmentName: "East Zone Radiant Floor", DesignName: "East Zone Radiant Floor Design", SurfaceName: "Zn002:Flr001", ZoneName: "EAST ZONE"},
		{EquipmentType: "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", EquipmentName: "North Zone Radiant Floor", DesignName: "North Zone Radiant Floor Design", SurfaceName: "Zn003:Flr001", ZoneName: "NORTH ZONE"},
	}
	owners, err = epathSQLRadiantOriginalOwners(string(data), binding)
	if err != nil || len(owners) != 3 {
		t.Fatalf("literal original radiant owners: %v %#v", err, owners)
	}
	for _, declared := range binding.Owners {
		owner := owners[strings.ToLower(declared.EquipmentName)]
		if owner.Owner != declared || owner.ZoneMultiplier != 1 || owner.ZoneListMultiplier != 1 {
			t.Fatalf("original literal owner mismatch: %#v", owner)
		}
	}
	binding.Owners = binding.Owners[:2]
	if _, err := epathSQLRadiantOriginalOwners(string(data), binding); err == nil {
		t.Fatal("declaration omitted the independently present third radiant owner")
	}
}

func TestEnergyPathRealSQLRadiantRejectsOriginalOwnershipMutants(t *testing.T) {
	for _, test := range []struct {
		name, kind, object string
		field              int
		value              string
	}{
		{"explicit_zone", "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", "Radiant", 3, "Other"},
		{"surface_owner", "BuildingSurface:Detailed", "Floor", 3, "Other"},
		{"foreign_surface", "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", "Radiant", 4, "Other Floor"},
		{"wrong_design", "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", "Radiant", 1, "Missing"},
		{"unowned_equipment", "ZoneHVAC:EquipmentList", "List", 3, "Missing"},
		{"wrong_typed_equipment", "ZoneHVAC:EquipmentList", "List", 2, "ZoneHVAC:LowTemperatureRadiant:VariableFlow"},
		{"foreign_list_owner", "ZoneHVAC:EquipmentConnections", "Office", 0, "Other"},
		{"missing_connection", "ZoneHVAC:EquipmentConnections", "Office", 1, "Missing"},
		{"crossed_branch", "Branch", "Heating Branch", 5, "CW Out"},
		{"missing_port", "ZoneHVAC:LowTemperatureRadiant:ConstantFlow", "Radiant", 10, ""},
		{"invalid_multiplier", "Zone", "Office", 6, "0"},
		{"noninteger_multiplier", "Zone", "Office", 6, "2.5"},
		{"invalid_group_multiplier", "ZoneGroup", "Office Group", 2, "NaN"},
	} {
		t.Run(test.name, func(t *testing.T) {
			text, binding := epathSQLRadiantHandFixture()
			doc, err := idf.Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			epathSQLRadiantUnitObject(t, &doc, test.kind, test.object).Fields[test.field].Value = test.value
			if _, err := epathSQLRadiantOriginalOwners(doc.String(), binding); err == nil {
				t.Fatal("mutated original ownership was accepted")
			}
		})
	}
	for _, kindAndName := range [][2]string{{"Zone", "Office"}, {"ZoneHVAC:LowTemperatureRadiant:ConstantFlow", "Radiant"}, {"ZoneHVAC:LowTemperatureRadiant:ConstantFlow:Design", "Design"}, {"BuildingSurface:Detailed", "Floor"}, {"ZoneHVAC:EquipmentList", "List"}, {"ZoneHVAC:EquipmentConnections", "Office"}, {"ZoneGroup", "Office Group"}, {"Branch", "Heating Branch"}} {
		t.Run("duplicate_"+kindAndName[0], func(t *testing.T) {
			text, binding := epathSQLRadiantHandFixture()
			doc, err := idf.Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			duplicate := *epathSQLRadiantUnitObject(t, &doc, kindAndName[0], kindAndName[1])
			duplicate.Index = len(doc.Objects)
			doc.Objects = append(doc.Objects, duplicate)
			if _, err := epathSQLRadiantOriginalOwners(doc.String(), binding); err == nil {
				t.Fatal("duplicate original identity accepted")
			}
		})
	}
	for _, extra := range []string{
		"ZoneHVAC:EquipmentList,Foreign List,SequentialLoad,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,Radiant,1,1,,;",
		"Unsupported:Wrapper,Wrapper,ZoneHVAC:LowTemperatureRadiant:ConstantFlow,Radiant;",
		"ZoneHVAC:LowTemperatureRadiant:Electric,Radiant,Always,Other,Other Floor;",
		"ZoneHVAC:LowTemperatureRadiant:SurfaceGroup,Shared,Floor,1;",
		"ZoneList,Second List,Office; ZoneGroup,Second Group,Second List,2;",
	} {
		text, binding := epathSQLRadiantHandFixture()
		if _, err := epathSQLRadiantOriginalOwners(text+extra, binding); err == nil {
			t.Fatalf("foreign/shared original context accepted: %s", extra)
		}
	}
}

func epathSQLRadiantUnitSource() epathRealSQLSource {
	source := epathRealSQLSource{DictionaryIndex: 101, Name: "Zone Radiant HVAC Cooling Energy", KeyValue: "RADIANT", ReportingFrequency: "Monthly", SourceUnit: "J", Rows: 12}
	raw, energy := 0.0, 0.0
	for month := 1; month <= 12; month++ {
		value := float64(month)
		if month == 2 {
			value = 0
		}
		source.Months = append(source.Months, epathRealSQLMonth{Month: month, Rows: 1, RawSum: epathOracleNumber(value * 3600000), EnergyKWh: epathOracleNumber(value)})
		raw += value * 3600000
		energy += value
	}
	source.RawSum, source.EnergyKWh = epathOracleNumber(raw), epathOracleNumber(energy)
	return source
}

func TestEnergyPathRealSQLRadiantNativeQuantityNeverMultipliedAgain(t *testing.T) {
	text, binding := epathSQLRadiantHandFixture()
	owners, err := epathSQLRadiantOriginalOwners(text, binding)
	if err != nil {
		t.Fatal(err)
	}
	source := epathSQLRadiantUnitSource()
	before, _ := json.Marshal(source)
	precision := epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}
	proof, err := epathSQLRadiantLoadObservation(source, "cooling", owners["radiant"], epathSQLZone{Name: "Office", Multiplier: 21}, precision)
	if err != nil {
		t.Fatal(err)
	}
	var sum float64
	for month, raw := range proof.Raw {
		want := float64(month + 1)
		if month == 1 {
			want = 0
		}
		if raw.Value != want || !reflect.DeepEqual(raw, proof.Effective[month]) || month == 1 && (raw.Error != 0 || raw.Value != 0) || month != 1 && raw.Error != .001 {
			t.Fatalf("native model-total J was multiplied/rounded or original zero became unknown: %#v", proof)
		}
		sum += raw.Value
	}
	if sum != 76 || proof.AppliedMultiplier != 1 || proof.AggregationBasis != "model_total" || proof.ThermalBoundary != "active_surface_source" || proof.Source.DictionaryIndex != 101 {
		t.Fatalf("independent native identity/annual sum changed: %#v", proof)
	}
	after, _ := json.Marshal(source)
	if string(before) != string(after) {
		t.Fatal("source proof mutated original SQL observations")
	}
	for _, multiplier := range []float64{0, 1, 7, 63, math.NaN()} {
		if _, err := epathSQLRadiantLoadObservation(source, "cooling", owners["radiant"], epathSQLZone{Name: "Office", Multiplier: multiplier}, precision); err == nil {
			t.Fatal("SQL multiplier contradiction was ignored")
		}
	}
}

func TestEnergyPathRealSQLRadiantRejectsUnknownOrMisboundSources(t *testing.T) {
	text, binding := epathSQLRadiantHandFixture()
	owners, err := epathSQLRadiantOriginalOwners(text, binding)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*epathRealSQLSource){
		func(s *epathRealSQLSource) { s.DictionaryIndex = 0 },
		func(s *epathRealSQLSource) { s.KeyValue = "Office" },
		func(s *epathRealSQLSource) { s.Name = "Zone Air System Sensible Cooling Energy" },
		func(s *epathRealSQLSource) { s.Name = "Zone Radiant HVAC Heating Energy" },
		func(s *epathRealSQLSource) { s.IsMeter = true },
		func(s *epathRealSQLSource) { s.ReportingFrequency = "Zone Timestep" },
		func(s *epathRealSQLSource) { s.SourceUnit = "W" },
		func(s *epathRealSQLSource) { s.Months = s.Months[:11] },
		func(s *epathRealSQLSource) { s.Months[1].Rows = 2 },
		func(s *epathRealSQLSource) { s.Months[1].MissingRows = 1 },
		func(s *epathRealSQLSource) { s.Months[1].EnergyKWh = nil },
		func(s *epathRealSQLSource) { s.Months[1].RawSum = nil },
		func(s *epathRealSQLSource) { s.Months[1].EnergyKWh = epathOracleNumber(math.NaN()) },
		func(s *epathRealSQLSource) { s.Months[1].RawSum = epathOracleNumber(-1) },
		func(s *epathRealSQLSource) { s.Months[0].EnergyKWh = epathOracleNumber(21) },
		func(s *epathRealSQLSource) { s.EnergyKWh = epathOracleNumber(1596) },
	} {
		source := epathSQLRadiantUnitSource()
		mutate(&source)
		if _, err := epathSQLRadiantLoadObservation(source, "cooling", owners["radiant"], epathSQLZone{Name: "Office", Multiplier: 21}, epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 2, ContributionStages: 2}); err == nil {
			t.Fatal("unknown or misbound source was made a known native load")
		}
	}
}

func TestEnergyPathRealSQLRadiantFramesUseExplicitNativeBranch(t *testing.T) {
	path, model := epathSQLModelUnitFixture(t)
	text, binding := epathSQLRadiantHandFixture()
	epathOracleEditSQL(t, path, `UPDATE Zones SET Multiplier=7,ListMultiplier=3;
UPDATE ReportDataDictionary SET Name='Zone Radiant HVAC Cooling Energy',KeyValue='Radiant' WHERE ReportDataDictionaryIndex=10;
UPDATE ReportDataDictionary SET Name='Zone Radiant HVAC Heating Energy',KeyValue='Radiant' WHERE ReportDataDictionaryIndex=11;
INSERT INTO Surfaces VALUES(3,'Floor','Floor',1,-1,1);
INSERT INTO ReportDataDictionary VALUES(15,'Surface Inside Face Convection Heat Gain Energy','Floor',0,'Monthly','J','Surface');
INSERT INTO ReportData SELECT 15000+TimeIndex,15,TimeIndex,0 FROM "Time" WHERE TimeIndex BETWEEN 1 AND 12;`)
	model.Surface.Source.Keys = append(model.Surface.Source.Keys, "Floor")
	model.Surface.RadiantContext = &epathRealSQLRadiantSurfaceContext{Policy: "original_direct_surface_context/v1"}
	model.Surface.Source.Alternatives = append([]epathRealSQLAlternative{{Name: "Surface Inside Face Convection Heat Gain Energy", Unit: "J"}}, model.Surface.Source.Alternatives...)
	for i := range model.Loads {
		name := "Zone Radiant HVAC Cooling Energy"
		if model.Loads[i].Service == "heating" {
			name = "Zone Radiant HVAC Heating Energy"
		}
		model.Loads[i].Component, model.Loads[i].NativeRadiant = "combined", &binding
		model.Loads[i].Source = epathRealSQLSelector{Keys: []string{"Radiant"}, Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}}
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model, text)
	if err != nil {
		t.Fatal(err)
	}
	for month := 1; month <= 12; month++ {
		cooling := frames.Loads[epathSQLKey("Office", "cooling", month)]
		heating := frames.Loads[epathSQLKey("Office", "heating", month)]
		if math.Abs(cooling.Value-3) > 2*(math.Nextafter(3, math.Inf(1))-3) || heating.Value != 0 || heating.Error != 0 || !reflect.DeepEqual(frames.SourceRaw[10], frames.SourceEffective[10]) || !reflect.DeepEqual(frames.LoadSourceIDs[epathSQLKey("Office", "cooling", month)], []int{10}) {
			t.Fatalf("frame load/source was multiplied again, unknown healed, or reassigned: %#v %#v", cooling, heating)
		}
	}
	if len(frames.RadiantLoadSourceIdentities) != 2 {
		t.Fatal("missing native source identities")
	}
	if _, err := epathCompileSQLModelFrames(path, observed.Sources, model); err == nil {
		t.Fatal("native load compiled without exact original input")
	}
	model.Loads[0].NativeRadiant = nil
	if _, err := epathCompileSQLModelFrames(path, observed.Sources, model, text); err == nil {
		t.Fatal("combined equipment-key source entered legacy Zone-key load branch")
	}
}
