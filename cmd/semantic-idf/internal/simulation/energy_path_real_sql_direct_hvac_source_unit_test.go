package simulation

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLDirectHVACSourceUnitFixture(t *testing.T) (string, epathRealSQLModel, *PurposeRunPlan) {
	t.Helper()
	path, model := epathSQLModelUnitFixture(t)
	plan := &PurposeRunPlan{}
	items := []struct {
		id, name, key, component, service, carrier string
		value                                      float64
	}{
		{"cooling.coil.electricity", "Cooling Coil Electricity Energy", "Unrelated DX name", "Coil:Cooling:DX:SingleSpeed", "cooling", "electricity", 2},
		{"cooling.coil.crankcase_electricity", "Cooling Coil Crankcase Heater Electricity Energy", "Unrelated DX name", "Coil:Cooling:DX:SingleSpeed", "cooling", "electricity", .25},
		{"heating.coil.natural_gas", "Heating Coil NaturalGas Energy", "Unrelated fuel name", "Coil:Heating:Fuel", "heating", "natural_gas", 3},
		{"heating.coil.ancillary_natural_gas", "Heating Coil Ancillary NaturalGas Energy", "Unrelated fuel name", "Coil:Heating:Fuel", "heating", "natural_gas", .125},
		{"heating.coil.electricity", "Heating Coil Electricity Energy", "Unrelated fuel name", "Coil:Heating:Fuel", "heating", "electricity", 0},
	}
	for offset, item := range items {
		id := 40 + offset
		epathOracleEditSQL(t, path, fmt.Sprintf(`INSERT INTO ReportDataDictionary VALUES(%d,'%s','%s',0,'Monthly','J','System');
INSERT INTO ReportData SELECT 30000+%d*20+t.Month,%d,t.TimeIndex,CASE WHEN t.Month=1 THEN %.17g*3600000 ELSE 0 END FROM Time t WHERE t.IntervalType=3;`, id, item.name, item.key, id, id, item.value))
		model.DirectHVACComponents = append(model.DirectHVACComponents, epathRealSQLDirectHVACComponent{ID: item.id, Service: item.service, Carrier: item.carrier, SiteID: item.service + "." + item.carrier, Frequency: "Monthly", AggregationBasis: "model_total", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: item.name, Unit: "J"}}, Keys: []string{item.key}}, Owners: []epathRealSQLDirectHVACOwner{{KeyValue: item.key, ZoneName: "Office", EquipmentType: "ZoneHVAC:PackagedTerminalAirConditioner", EquipmentName: "Native package", ComponentType: item.component}}})
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", VariableName: item.name, KeyValue: item.key, ReportingFrequency: "Monthly", ScopeZoneName: "Office", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	}
	for offset, site := range []struct {
		id, name, service, carrier string
		value                      float64
	}{{"cooling.electricity", "Cooling:Electricity", "cooling", "electricity", 2.25}, {"heating.natural_gas", "Heating:NaturalGas", "heating", "natural_gas", 3.125}, {"heating.electricity", "Heating:Electricity", "heating", "electricity", 0}} {
		id := 50 + offset
		if offset == 0 {
			// The shared raw-reader fixture already owns dictionary 1 with
			// this exact Monthly meter. Reuse its twelve weather rows; keep
			// the separate RunPeriod and warmup/design-day records intact.
			epathOracleEditSQL(t, path, fmt.Sprintf(`UPDATE ReportData SET Value=CASE WHEN TimeIndex=1 THEN %.17g*3600000 ELSE 0 END WHERE ReportDataDictionaryIndex=1 AND TimeIndex BETWEEN 1 AND 12;`, site.value))
		} else {
			epathOracleEditSQL(t, path, fmt.Sprintf(`INSERT INTO ReportDataDictionary VALUES(%d,'%s','',1,'Monthly','J','Facility');
INSERT INTO ReportData SELECT 40000+%d*20+t.Month,%d,t.TimeIndex,CASE WHEN t.Month=1 THEN %.17g*3600000 ELSE 0 END FROM Time t WHERE t.IntervalType=3;`, id, site.name, id, id, site.value))
		}
		model.Site = append(model.Site, epathRealSQLSite{ID: site.id, EndUse: site.service, Carrier: site.carrier, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: site.name, Unit: "J"}}, Keys: []string{"*"}, IsMeter: true}})
	}
	return path, model, plan
}

func epathSQLDirectHVACPTHPSourceUnitFixture(t *testing.T) (string, epathRealSQLModel, *PurposeRunPlan) {
	t.Helper()
	path, model, plan := epathSQLDirectHVACSourceUnitFixture(t)
	// PTHP has no DX-cooling crankcase output in the reviewed RDD/MTD.
	epathOracleEditSQL(t, path, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=41;
DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=41;
UPDATE ReportData SET Value=2*3600000 WHERE ReportDataDictionaryIndex=1 AND TimeIndex=1;
UPDATE ReportData SET Value=.375*3600000 WHERE ReportDataDictionaryIndex=44 AND TimeIndex=1;
UPDATE ReportData SET Value=5*3600000 WHERE ReportDataDictionaryIndex=52 AND TimeIndex=1;`)
	model.DirectHVACComponents = append(model.DirectHVACComponents[:1], model.DirectHVACComponents[2:]...)
	plan.OutputObjects = append(plan.OutputObjects[:1], plan.OutputObjects[2:]...)
	for index := range model.DirectHVACComponents {
		model.DirectHVACComponents[index].Owners[0].EquipmentType = "ZoneHVAC:PackagedTerminalHeatPump"
	}
	for offset, item := range []struct {
		id, name string
		value    float64
	}{
		{"heating.coil.dx_electricity", "Heating Coil Electricity Energy", 4},
		{"heating.coil.defrost_electricity", "Heating Coil Defrost Electricity Energy", .5},
		{"heating.coil.crankcase_electricity", "Heating Coil Crankcase Heater Electricity Energy", .125},
	} {
		id, key := 80+offset, "Unrelated heat-pump name"
		epathOracleEditSQL(t, path, fmt.Sprintf(`INSERT INTO ReportDataDictionary VALUES(%d,'%s','%s',0,'Monthly','J','System');
INSERT INTO ReportData SELECT 50000+%d*20+t.Month,%d,t.TimeIndex,CASE WHEN t.Month=1 THEN %.17g*3600000 ELSE 0 END FROM Time t WHERE t.IntervalType=3;`, id, item.name, key, id, id, item.value))
		model.DirectHVACComponents = append(model.DirectHVACComponents, epathRealSQLDirectHVACComponent{ID: item.id, Service: "heating", Carrier: "electricity", SiteID: "heating.electricity", Frequency: "Monthly", AggregationBasis: "model_total", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: item.name, Unit: "J"}}, Keys: []string{key}}, Owners: []epathRealSQLDirectHVACOwner{{KeyValue: key, ZoneName: "Office", EquipmentType: "ZoneHVAC:PackagedTerminalHeatPump", EquipmentName: "Native package", ComponentType: "Coil:Heating:DX:SingleSpeed"}}})
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", VariableName: item.name, KeyValue: key, ReportingFrequency: "Monthly", ScopeZoneName: "Office", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	}
	return path, model, plan
}

func epathSQLDirectHVACOriginalUnitText(heatPump bool) string {
	objectType, fields := "ZoneHVAC:PackagedTerminalAirConditioner", make([]string, 21)
	fields[15], fields[16], fields[17], fields[18] = "Coil:Heating:Fuel", "Unrelated fuel name", "Coil:Cooling:DX:SingleSpeed", "Unrelated DX name"
	if heatPump {
		objectType, fields = "ZoneHVAC:PackagedTerminalHeatPump", make([]string, 27)
		fields[15], fields[16], fields[18], fields[19], fields[21], fields[22] = "Coil:Heating:DX:SingleSpeed", "Unrelated heat-pump name", "Coil:Cooling:DX:SingleSpeed", "Unrelated DX name", "Coil:Heating:Fuel", "Unrelated fuel name"
	}
	fields[0], fields[2], fields[3] = "Native package", "Package inlet", "Package outlet"
	return "Zone,Office;\n" + objectType + "," + strings.Join(fields, ",") + ";\n" +
		"Coil:Cooling:DX:SingleSpeed,Unrelated DX name;\nCoil:Heating:Fuel,Unrelated fuel name,,NaturalGas;\n" +
		"Coil:Heating:DX:SingleSpeed,Unrelated heat-pump name;\n" +
		"ZoneHVAC:EquipmentList,Native list,SequentialLoad," + objectType + ",Native package,1,1,,;\n" +
		"ZoneHVAC:EquipmentConnections,Office,Native list;\n" +
		"AirTerminal:SingleDuct:Mixer,DOAS terminal," + objectType + ",Native package,Package inlet,Primary inlet,Secondary inlet,InletSide;\n"
}

func TestEnergyPathRealSQLDirectHVACPTHPSourceRolesAndSameName(t *testing.T) {
	path, model, plan := epathSQLDirectHVACPTHPSourceUnitFixture(t)
	if err := epathSQLValidateDirectHVACOriginalModel(epathSQLDirectHVACOriginalUnitText(true), model); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLValidateDirectHVACRequests(plan, model); err != nil {
		t.Fatal(err)
	}
	base := model
	base.DirectHVACComponents = nil
	frames, observed := epathSQLDirectHVACSourceUnitFrames(t, path, base)
	beforeSQL := epathRealFileHash(t, path)
	before, _ := json.Marshal([]any{frames.Loads, frames.Cells, frames.Site, frames.SourceRaw, frames.SourceEffective, frames.LoadSourceIDs})
	if err := epathCompileSQLDirectHVACSources(path, observed.Sources, model, &frames); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal([]any{frames.Loads, frames.Cells, frames.Site, frames.SourceRaw, frames.SourceEffective, frames.LoadSourceIDs})
	if string(before) != string(after) || epathRealFileHash(t, path) != beforeSQL {
		t.Fatal("PTHP consumption altered original SQL or thermal/site authorities")
	}
	if len(frames.DirectHVACSourceIdentities) != 7 || len(frames.DirectHVAC) != 36 || frames.Zones["office"].Multiplier != 6 {
		t.Fatal("PTHP needs seven independently measured roles and a nonunit multiplier counterexample")
	}
	for _, test := range []struct {
		service, carrier string
		value            float64
		ids              []int
	}{{"cooling", "electricity", 2, []int{40}}, {"heating", "electricity", 5, []int{44, 80, 81, 82}}, {"heating", "natural_gas", 3.125, []int{42, 43}}} {
		for month := 1; month <= 12; month++ {
			want := test.value
			if month > 1 {
				want = 0
			}
			got := frames.DirectHVAC[epathSQLDirectHVACKey("Office", test.service, test.carrier, month)]
			if !got.Present || math.Abs(got.Quantity.Value-want) > 1e-14 || !reflect.DeepEqual(got.SourceIDs, test.ids) || want == 0 && got.Quantity.Error != 0 {
				t.Fatalf("PTHP constituent summation/type/known-zero mismatch: %+v", got)
			}
		}
	}
	if frames.DirectHVACSourceIdentities[44].Source.Name != frames.DirectHVACSourceIdentities[80].Source.Name || frames.DirectHVACSourceIdentities[44].Owner.ComponentType == frames.DirectHVACSourceIdentities[80].Owner.ComponentType {
		t.Fatal("fixture lost same-name DX/Fuel distinction")
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectHVACSourceChecks(frames, &checks); err != nil || len(checks.Rows) != 28 {
		t.Fatalf("seven exact source raw/effective scope proofs: %v/%d", err, len(checks.Rows))
	}
	for _, id := range []int{44, 80, 81, 82} {
		identity := frames.DirectHVACSourceIdentities[id]
		if err := epathSQLMatchDirectHVACSource(epathSQLDirectHVACSourceUnitCandidate(identity), identity); err != nil {
			t.Fatal(err)
		}
	}
	dx, fuel := frames.DirectHVACSourceIdentities[80], frames.DirectHVACSourceIdentities[44]
	wrong := epathSQLDirectHVACSourceUnitCandidate(dx)
	wrong.KeyValue = fuel.Source.KeyValue
	if err := epathSQLMatchDirectHVACSource(wrong, dx); err == nil {
		t.Fatal("same variable name let Fuel consumption masquerade as the DX original")
	}
	dx.Owner.ComponentType = "Coil:Heating:Fuel"
	if err := epathSQLValidateDirectHVACSourceIdentity(dx); err == nil {
		t.Fatal("same-name DX proof accepted a substituted Fuel type")
	}
	for index := range plan.OutputObjects {
		output := &plan.OutputObjects[index]
		if output.VariableName != "Heating Coil Electricity Energy" || output.KeyValue != "Unrelated heat-pump name" {
			continue
		}
		output.ScopeZoneName = "Other Zone"
		if err := epathSQLValidateDirectHVACRequests(plan, model); err == nil {
			t.Fatal("a valid same-name Fuel request concealed the DX request's foreign Zone")
		}
		break
	}
}

func TestEnergyPathRealSQLDirectHVACPTHPRejectsPartialAndCrossTyped(t *testing.T) {
	for _, test := range []struct {
		name, sql string
		mutate    func(*epathRealSQLModel)
	}{
		{"missing defrost month", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=81 AND TimeIndex=1`, nil},
		{"NULL DX main", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=80 AND TimeIndex=1`, nil},
		{"negative defrost", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=81 AND TimeIndex=1`, nil},
		{"duplicate same-name DX identity", `INSERT INTO ReportDataDictionary SELECT 90,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=80`, nil},
		{"unreviewed same-name owner", `INSERT INTO ReportDataDictionary SELECT 90,Name,'Other DX',IsMeter,ReportingFrequency,Units,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=80`, nil},
		{"wrong DX units", `UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=80`, nil},
		{"missing exact role", "", func(m *epathRealSQLModel) { m.DirectHVACComponents = m.DirectHVACComponents[:6] }},
		{"DX role assigned Fuel type", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[4].Owners[0].ComponentType = "Coil:Heating:Fuel" }},
		{"different package type", "", func(m *epathRealSQLModel) {
			m.DirectHVACComponents[4].Owners[0].EquipmentType = "ZoneHVAC:PackagedTerminalAirConditioner"
		}},
		{"same-name key claimed by two roles", "", func(m *epathRealSQLModel) {
			m.DirectHVACComponents[4].Source.Keys[0] = "Unrelated fuel name"
			m.DirectHVACComponents[4].Owners[0].KeyValue = "Unrelated fuel name"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, model, _ := epathSQLDirectHVACPTHPSourceUnitFixture(t)
			if test.sql != "" {
				epathOracleEditSQL(t, path, test.sql)
			}
			if test.mutate != nil {
				test.mutate(&model)
			}
			observed, err := epathReadRealSQLOracle(path)
			if err != nil {
				return
			}
			base := model
			base.DirectHVACComponents = nil
			frames, err := epathCompileSQLModelFrames(path, observed.Sources, base)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(frames)
			if err := epathCompileSQLDirectHVACSources(path, observed.Sources, model, &frames); err == nil {
				t.Fatal("PTHP missing/ambiguous typed original was accepted")
			}
			after, _ := json.Marshal(frames)
			if string(before) != string(after) {
				t.Fatal("invalid PTHP source published a partial direct cohort")
			}
		})
	}
}

func TestEnergyPathRealSQLDirectHVACOriginalTypedOwnership(t *testing.T) {
	_, model, _ := epathSQLDirectHVACPTHPSourceUnitFixture(t)
	encoded, _ := json.Marshal(model)
	object := func(t *testing.T, doc *idf.Document, objectType string) *idf.Object {
		t.Helper()
		for index := range doc.Objects {
			if doc.Objects[index].Type == objectType {
				return &doc.Objects[index]
			}
		}
		t.Fatalf("missing handwritten object %s", objectType)
		return nil
	}
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *idf.Document, *epathRealSQLModel)
	}{
		{"swap declared DX and Fuel keys", func(_ *testing.T, _ *idf.Document, m *epathRealSQLModel) {
			for index := range m.DirectHVACComponents {
				declaration := &m.DirectHVACComponents[index]
				switch declaration.Owners[0].ComponentType {
				case "Coil:Heating:DX:SingleSpeed":
					declaration.Owners[0].KeyValue, declaration.Source.Keys[0] = "Unrelated fuel name", "Unrelated fuel name"
				case "Coil:Heating:Fuel":
					declaration.Owners[0].KeyValue, declaration.Source.Keys[0] = "Unrelated heat-pump name", "Unrelated heat-pump name"
				}
			}
		}},
		{"wrong original coil type", func(t *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			object(t, d, "Coil:Heating:DX:SingleSpeed").Type = "Coil:Heating:Fuel"
		}},
		{"wrong original Fuel", func(t *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			object(t, d, "Coil:Heating:Fuel").Fields[2].Value = "Propane"
		}},
		{"different native role field", func(t *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			parent := object(t, d, "ZoneHVAC:PackagedTerminalHeatPump")
			parent.Fields[15], parent.Fields[21] = parent.Fields[21], parent.Fields[15]
			parent.Fields[16], parent.Fields[22] = parent.Fields[22], parent.Fields[16]
		}},
		{"duplicate original coil", func(t *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			d.Objects = append(d.Objects, *object(t, d, "Coil:Heating:DX:SingleSpeed"))
		}},
		{"another typed coil parent", func(_ *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			d.Objects = append(d.Objects, idf.Object{Type: "OtherComponent", Fields: []idf.Field{{Value: "Shared reference"}, {Value: "Coil:Heating:DX:SingleSpeed"}, {Value: "Unrelated heat-pump name"}}})
		}},
		{"duplicate EquipmentList owner", func(t *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			copy := *object(t, d, "ZoneHVAC:EquipmentList")
			copy.Fields = append([]idf.Field(nil), copy.Fields...)
			copy.Fields[0].Value = "Other list"
			d.Objects = append(d.Objects, copy)
		}},
		{"foreign Zone owner", func(t *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			object(t, d, "ZoneHVAC:EquipmentConnections").Fields[0].Value = "Other Zone"
		}},
		{"missing original Zone", func(t *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			object(t, d, "Zone").Fields[0].Value = "Other Zone"
		}},
		{"duplicate Zone connection", func(t *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			d.Objects = append(d.Objects, *object(t, d, "ZoneHVAC:EquipmentConnections"))
		}},
		{"wrong mixer connection", func(t *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			object(t, d, "AirTerminal:SingleDuct:Mixer").Fields[3].Value = "Other inlet"
		}},
		{"blank mixer connection", func(t *testing.T, d *idf.Document, _ *epathRealSQLModel) {
			object(t, d, "AirTerminal:SingleDuct:Mixer").Fields[3].Value = ""
			object(t, d, "ZoneHVAC:PackagedTerminalHeatPump").Fields[2].Value = ""
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var model epathRealSQLModel
			if err := json.Unmarshal(encoded, &model); err != nil {
				t.Fatal(err)
			}
			doc, err := idf.Parse(epathSQLDirectHVACOriginalUnitText(true))
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, &doc, &model)
			if err := epathSQLValidateDirectHVACOriginalModel(doc.String(), model); err == nil {
				t.Fatal("recipe-declared type/key was accepted without its true original physical owner")
			}
		})
	}
}

func TestEnergyPathRealSQLDirectHVACOriginalNativePTACAndPTHP(t *testing.T) {
	for _, heatPump := range []bool{false, true} {
		name, packageType := "DOAToPTAC.idf", "ZoneHVAC:PackagedTerminalAirConditioner"
		_, model, _ := epathSQLDirectHVACSourceUnitFixture(t)
		if heatPump {
			name, packageType = "DOAToPTHP.idf", "ZoneHVAC:PackagedTerminalHeatPump"
			_, model, _ = epathSQLDirectHVACPTHPSourceUnitFixture(t)
		}
		data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", name))
		if err != nil {
			t.Fatal(err)
		}
		owners := 0
		for index := range model.DirectHVACComponents {
			declaration := &model.DirectHVACComponents[index]
			component := declaration.Owners[0].ComponentType
			declaration.Owners, declaration.Source.Keys = nil, nil
			for zoneIndex := 1; zoneIndex <= 5; zoneIndex++ {
				zone := fmt.Sprintf("SPACE%d-1", zoneIndex)
				parent, key := zone+" PTAC", zone+" PTAC CCoil"
				if component == "Coil:Heating:Fuel" {
					key = zone + " Heating Coil"
				}
				if heatPump {
					parent = zone + " Heat Pump"
					switch component {
					case "Coil:Cooling:DX:SingleSpeed":
						key = zone + " HP Cooling Mode"
					case "Coil:Heating:DX:SingleSpeed":
						key = zone + " HP Heating Mode"
					case "Coil:Heating:Fuel":
						key = zone + " HP Supp Coil"
					}
				}
				declaration.Source.Keys = append(declaration.Source.Keys, key)
				declaration.Owners = append(declaration.Owners, epathRealSQLDirectHVACOwner{KeyValue: key, ZoneName: zone, EquipmentType: packageType, EquipmentName: parent, ComponentType: component})
				owners++
			}
		}
		want := 25
		if heatPump {
			want = 35
		}
		if owners != want {
			t.Fatal("actual original needs every parent-specific component/owner binding")
		}
		if err := epathSQLValidateDirectHVACOriginalModel(string(data), model); err != nil {
			t.Fatalf("original %s exact %d role owners: %v", name, owners, err)
		}
	}
}

func epathSQLDirectHVACSourceUnitFrames(t *testing.T, path string, model epathRealSQLModel) (epathSQLFrames, epathRealOracleEvidence) {
	t.Helper()
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model)
	if err != nil {
		t.Fatal(err)
	}
	return frames, observed
}

func TestEnergyPathRealSQLDirectHVACSourceFrames(t *testing.T) {
	path, model, plan := epathSQLDirectHVACSourceUnitFixture(t)
	before := epathRealFileHash(t, path)
	if err := epathSQLValidateDirectHVACRequests(plan, model); err != nil {
		t.Fatal(err)
	}
	base := model
	base.DirectHVACComponents = nil
	frames, observed := epathSQLDirectHVACSourceUnitFrames(t, path, base)
	prior, err := json.Marshal([]any{frames.Loads, frames.Cells, frames.Site, frames.SourceRaw, frames.SourceEffective, frames.LoadSourceIDs})
	if err != nil {
		t.Fatal(err)
	}
	if err := epathCompileSQLDirectHVACSources(path, observed.Sources, model, &frames); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal([]any{frames.Loads, frames.Cells, frames.Site, frames.SourceRaw, frames.SourceEffective, frames.LoadSourceIDs})
	if string(prior) != string(after) {
		t.Fatal("direct observations changed thermal/Building authorities or generic source quantities")
	}
	if len(frames.DirectHVAC) != 36 || len(frames.DirectHVACSourceIdentities) != 5 {
		t.Fatalf("incomplete source frame: %d/%d", len(frames.DirectHVAC), len(frames.DirectHVACSourceIdentities))
	}
	for month := 1; month <= 12; month++ {
		for _, test := range []struct {
			service, carrier string
			value            float64
			ids              []int
		}{{"cooling", "electricity", 2.25, []int{40, 41}}, {"heating", "natural_gas", 3.125, []int{42, 43}}, {"heating", "electricity", 0, []int{44}}} {
			want := test.value
			if month != 1 {
				want = 0
			}
			got := frames.DirectHVAC[epathSQLDirectHVACKey("Office", test.service, test.carrier, month)]
			// The two original J-to-kWh conversions and their addition can
			// differ from the hand-written decimal by a few binary ULPs.
			// This is not the separate three-decimal presentation allowance.
			ulp := math.Nextafter(want, math.Inf(1)) - want
			quantityMatches := math.Abs(got.Quantity.Value-want) <= 4*ulp
			if want == 0 {
				quantityMatches = got.Quantity.Value == 0
			}
			if !got.Present || !quantityMatches || !reflect.DeepEqual(got.SourceIDs, test.ids) {
				t.Fatalf("wrong original cohort M%d: %+v want%g", month, got, want)
			}
			if want == 0 && (got.Quantity.Error != 0 || !got.Quantity.includesZero()) {
				t.Fatal("observed zero acquired invented uncertainty")
			}
		}
	}
	if frames.Zones["office"].Multiplier != 6 {
		t.Fatal("fixture needs a nonunit Zone multiplier")
	}
	if epathRealFileHash(t, path) != before {
		t.Fatal("read-only source compiler changed SQL")
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectHVACSourceChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 20 {
		t.Fatalf("source proof rows=%d want 5x2x2", len(checks.Rows))
	}
}

func TestEnergyPathRealSQLDirectHVACSourceRejectsMissingAndSpoofed(t *testing.T) {
	for _, test := range []struct {
		name, sql string
		mutate    func(*epathRealSQLModel)
	}{
		{"missing Monthly row", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=40 AND TimeIndex=1`, nil},
		{"null Monthly row", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=40 AND TimeIndex=1`, nil},
		{"duplicate Monthly row", `INSERT INTO ReportData SELECT 99999,40,TimeIndex,Value FROM ReportData WHERE ReportDataDictionaryIndex=40 AND TimeIndex=1`, nil},
		{"negative Monthly row", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=40 AND TimeIndex=1`, nil},
		{"unobserved duplicate", `INSERT INTO ReportDataDictionary SELECT 80,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=40`, nil},
		{"case duplicate", `INSERT INTO ReportDataDictionary SELECT 80,lower(Name),lower(KeyValue),IsMeter,ReportingFrequency,Units,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=40`, nil},
		{"undeclared owner", `INSERT INTO ReportDataDictionary SELECT 80,Name,'Central unknown coil',IsMeter,ReportingFrequency,Units,IndexGroup FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=40`, nil},
		{"missing constituent", "", func(m *epathRealSQLModel) { m.DirectHVACComponents = m.DirectHVACComponents[:4] }},
		{"wrong carrier", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Carrier = "natural_gas" }},
		{"wrong source unit", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Source.Alternatives[0].Unit = "W" }},
		{"package electric masquerade", "", func(m *epathRealSQLModel) {
			m.DirectHVACComponents[0].Source.Alternatives[0].Name = "Zone Packaged Terminal Air Conditioner Electricity Energy"
		}},
		{"unknown Zone", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Owners[0].ZoneName = "PLENUM" }},
		{"different package owner", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Owners[0].EquipmentName = "Other package" }},
		{"wrong component type", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Owners[0].ComponentType = "Coil:Heating:Fuel" }},
		{"double multiplier declaration", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].AggregationBasis = "per_zone" }},
		{"unknown treated absent", "", func(m *epathRealSQLModel) { m.DirectHVACComponents[0].Source.AllowAbsent = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, model, _ := epathSQLDirectHVACSourceUnitFixture(t)
			if test.sql != "" {
				epathOracleEditSQL(t, path, test.sql)
			}
			if test.mutate != nil {
				test.mutate(&model)
			}
			observed, err := epathReadRealSQLOracle(path)
			if err != nil {
				return
			} // The independent raw reader may reject invalid SQL first.
			base := model
			base.DirectHVACComponents = nil
			frames, err := epathCompileSQLModelFrames(path, observed.Sources, base)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(frames)
			if err := epathCompileSQLDirectHVACSources(path, observed.Sources, model, &frames); err == nil {
				t.Fatal("invalid direct ownership/observation accepted")
			}
			after, _ := json.Marshal(frames)
			if string(before) != string(after) {
				t.Fatal("failed direct compilation published partial frame")
			}
		})
	}
}

func epathSQLDirectHVACSourceUnitCandidate(identity epathSQLDirectHVACSourceIdentity) EnergyDataSource {
	q, _ := epathSQLDirectHVACAnnualQuantity(identity)
	return EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", identity.Source.DictionaryIndex), SourceType: "sql_report_data", Name: identity.Source.Name, KeyValue: identity.Source.KeyValue, Units: "J", SourceUnit: "J", NormalizedUnit: "kWh", ReportingFrequency: "Monthly", AggregationMethod: "sum_report_data", ZoneName: identity.Owner.ZoneName, AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", RawValue: q.Value, EffectiveValue: q.Value, inspectorDecodedFromJSON: true, inspectorValuePresence: 3, ScopeDetails: []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: identity.Owner.ZoneName}, RawValue: q.Value, EffectiveValue: q.Value, AggregationBasis: "model_total", EffectiveMultiplier: 1, MultiplierApplication: "already_model_total", inspectorDecodedFromJSON: true, inspectorValuePresence: 3, inspectorScopedValuePresence: true}}}
}

func TestEnergyPathRealSQLDirectHVACSourceProofAndRequests(t *testing.T) {
	path, model, plan := epathSQLDirectHVACSourceUnitFixture(t)
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectHVACSourceChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}}}}}
	for id := 40; id <= 44; id++ {
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, epathSQLDirectHVACSourceUnitCandidate(frames.DirectHVACSourceIdentities[id]))
	}
	for _, check := range checks.Rows {
		if err := epathCheckSQLDirectHVACSource(bundle, check); err != nil {
			t.Fatalf("%s: %v", check.Want.Key, err)
		}
	}
	identity := frames.DirectHVACSourceIdentities[40]
	for _, test := range []struct {
		name   string
		mutate func(*EnergyDataSource)
	}{
		{"wrong Zone", func(s *EnergyDataSource) { s.ZoneName = "Other" }},
		{"wrong key", func(s *EnergyDataSource) { s.KeyValue = "Lighting" }},
		{"wrong carrier family", func(s *EnergyDataSource) { s.Name = "Heating Coil NaturalGas Energy" }},
		{"wrong frequency", func(s *EnergyDataSource) { s.ReportingFrequency = "Hourly" }},
		{"zone remultiplied", func(s *EnergyDataSource) { s.EffectiveMultiplier = 6 }},
		{"wrong aggregation", func(s *EnergyDataSource) { s.AggregationBasis = "per_zone" }},
		{"wrapper masquerade", func(s *EnergyDataSource) { s.InputSourceIDs = []string{"other"} }},
		{"wrong ID", func(s *EnergyDataSource) { s.ID = "derived-40" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := epathSQLDirectHVACSourceUnitCandidate(identity)
			test.mutate(&s)
			if epathSQLMatchDirectHVACSource(s, identity) == nil {
				t.Fatal("spoofed source metadata accepted")
			}
		})
	}
	original, err := epathSQLOriginalDirectHVAC(identity)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := epathSQLOriginalKey(original)
	source := epathSQLDirectHVACSourceUnitCandidate(identity)
	allowed := map[string]epathSQLOriginalSource{key: original}
	if _, err := epathSQLOriginalSourceLeaves([]string{key}, map[string]EnergyDataSource{key: source}, allowed, "M1"); err != nil {
		t.Fatal(err)
	}
	source.InputSourceIDs = []string{"other"}
	if _, err := epathSQLOriginalSourceLeaves([]string{key}, map[string]EnergyDataSource{key: source}, allowed, "M1"); err == nil {
		t.Fatal("direct source escaped exact owner validation through wrapper recursion")
	}
	for _, mutate := range []func(*PurposeRunPlan){
		func(p *PurposeRunPlan) { p.OutputObjects = p.OutputObjects[1:] },
		func(p *PurposeRunPlan) { p.OutputObjects[0].ScopeZoneName = "Other" },
		func(p *PurposeRunPlan) { p.OutputObjects[0].KeyValue = "Office" },
		func(p *PurposeRunPlan) { p.OutputObjects[0].ReportingFrequency = "Hourly" },
		func(p *PurposeRunPlan) { p.OutputObjects = append(p.OutputObjects, p.OutputObjects[0]) },
	} {
		copyPlan := *plan
		copyPlan.OutputObjects = append([]PurposeOutputObject(nil), plan.OutputObjects...)
		mutate(&copyPlan)
		if epathSQLValidateDirectHVACRequests(&copyPlan, model) == nil {
			t.Fatal("missing/spoofed/duplicate executed request accepted")
		}
	}
	zero := frames.DirectHVACSourceIdentities[44]
	zero.Source.Months = append([]epathRealSQLMonth(nil), zero.Source.Months...)
	zero.Source.Months[0].EnergyKWh = epathOracleNumber(math.SmallestNonzeroFloat64)
	if epathSQLValidateDirectHVACSourceIdentity(zero) == nil {
		t.Fatal("exact source zero became nonzero")
	}
}

func TestEnergyPathRealSQLDirectHVACSourceExactOwnedZeroWithoutScopeDetails(t *testing.T) {
	path, model, _ := epathSQLDirectHVACSourceUnitFixture(t)
	frames, _ := epathSQLDirectHVACSourceUnitFrames(t, path, model)
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectHVACSourceChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	var raw, effective, positive epathSQLModelCheck
	for _, check := range checks.Rows {
		if check.Item.Scope != "zone" {
			continue
		}
		if check.DirectHVACSource.Source.DictionaryIndex == 44 {
			if check.Item.Target.Field == "rawValue" {
				raw = check
			} else {
				effective = check
			}
		} else if check.DirectHVACSource.Source.DictionaryIndex == 40 && check.Item.Target.Field == "rawValue" {
			positive = check
		}
	}
	if raw.DirectHVACSource == nil || effective.DirectHVACSource == nil || positive.DirectHVACSource == nil {
		t.Fatal("fixture lost its zero/positive source obligations")
	}
	fieldsFor := func(identity epathSQLDirectHVACSourceIdentity) map[string]any {
		source := epathSQLDirectHVACSourceUnitCandidate(identity)
		source.ScopeDetails = nil
		data, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		fields := map[string]any{}
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		// Original v2 wire explicitly contains both measured zero scalars.
		fields["rawValue"], fields["effectiveValue"] = source.RawValue, source.EffectiveValue
		return fields
	}
	decode := func(fields map[string]any) PurposeResultBundle {
		data, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		var source EnergyDataSource
		if err := json.Unmarshal(data, &source); err != nil {
			t.Fatal(err)
		}
		return PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Sources: []EnergyDataSource{source}, ZoneResults: []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "Office"}}}}}
	}
	for _, check := range []epathSQLModelCheck{raw, effective} {
		bundle := decode(fieldsFor(*check.DirectHVACSource))
		before, _ := json.Marshal(bundle.EnergyExplanation.Sources[0])
		if err := epathCheckSQLDirectHVACSource(bundle, check); err != nil {
			t.Fatalf("exact explicit owned zero was unknown: %v", err)
		}
		after, _ := json.Marshal(bundle.EnergyExplanation.Sources[0])
		if string(before) != string(after) || len(bundle.EnergyExplanation.Sources[0].ScopeDetails) != 0 {
			t.Fatal("source proof fabricated a scope detail or changed original wire data")
		}
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any, *epathSQLModelCheck)
	}{
		{"missing raw", func(f map[string]any, _ *epathSQLModelCheck) { delete(f, "rawValue") }},
		{"null raw", func(f map[string]any, _ *epathSQLModelCheck) { f["rawValue"] = nil }},
		{"nonzero root", func(f map[string]any, _ *epathSQLModelCheck) { f["rawValue"] = .000001 }},
		{"wrong Zone source", func(f map[string]any, _ *epathSQLModelCheck) { f["zoneName"] = "Other" }},
		{"blank Zone source", func(f map[string]any, _ *epathSQLModelCheck) { delete(f, "zoneName") }},
		{"wrong requested Zone", func(_ map[string]any, c *epathSQLModelCheck) { c.Item.Zone = "Other" }},
		{"wrong model basis", func(f map[string]any, _ *epathSQLModelCheck) { f["aggregationBasis"] = "per_zone" }},
		{"wrong multiplier", func(f map[string]any, _ *epathSQLModelCheck) { f["effectiveMultiplier"] = 6 }},
		{"monthly context", func(_ map[string]any, c *epathSQLModelCheck) { c.Item.Period = "M1" }},
		{"wrong reporting frequency", func(f map[string]any, _ *epathSQLModelCheck) { f["reportingFrequency"] = "Annual" }},
		{"malformed detail", func(f map[string]any, _ *epathSQLModelCheck) { f["scopeDetails"] = []any{map[string]any{}} }},
		{"foreign detail", func(f map[string]any, _ *epathSQLModelCheck) {
			f["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Other"}, "rawValue": 0, "effectiveValue": 0}}
		}},
		{"same owner missing detail value", func(f map[string]any, _ *epathSQLModelCheck) {
			f["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Office"}}}
		}},
		{"same owner null detail value", func(f map[string]any, _ *epathSQLModelCheck) {
			f["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Office"}, "rawValue": nil}}
		}},
		{"same owner contradictory detail", func(f map[string]any, _ *epathSQLModelCheck) {
			f["scopeDetails"] = []any{map[string]any{"scope": map[string]any{"kind": "zone", "zoneName": "Office"}, "rawValue": 1}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fields, check := fieldsFor(*raw.DirectHVACSource), raw
			test.mutate(fields, &check)
			if err := epathCheckSQLDirectHVACSource(decode(fields), check); err == nil {
				t.Fatal("unknown/foreign/contradictory source became an owned zero")
			}
		})
	}
	for _, missing := range []bool{true, false} {
		fields := fieldsFor(*effective.DirectHVACSource)
		if missing {
			delete(fields, "effectiveValue")
		} else {
			fields["effectiveValue"] = nil
		}
		bundle := decode(fields)
		if err := epathCheckSQLDirectHVACSource(bundle, effective); err == nil {
			t.Fatal("known raw zero fabricated unknown effective zero")
		}
		if err := epathCheckSQLDirectHVACSource(bundle, raw); err != nil {
			t.Fatal("independently explicit raw zero was lost with missing effective scalar")
		}
	}
	bundle := decode(fieldsFor(*raw.DirectHVACSource))
	bundle.EnergyExplanation.Sources[0].inspectorDecodedFromJSON = false
	if err := epathCheckSQLDirectHVACSource(bundle, raw); err == nil {
		t.Fatal("undecoded metadata fabricated explicit wire zero")
	}
	bundle = decode(fieldsFor(*raw.DirectHVACSource))
	bundle.EnergyExplanation.Schema = "semantic-idf.energy-explanation/v1"
	if err := epathCheckSQLDirectHVACSource(bundle, raw); err == nil {
		t.Fatal("legacy wire received v2-only source proof fallback")
	}
	if err := epathCheckSQLDirectHVACSource(decode(fieldsFor(*positive.DirectHVACSource)), positive); err == nil {
		t.Fatal("nonzero source bypassed the existing strict scope-detail reader")
	}
}
