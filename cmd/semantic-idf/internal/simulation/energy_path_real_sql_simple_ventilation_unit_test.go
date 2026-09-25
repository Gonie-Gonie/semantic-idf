package simulation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func epathSQLSimpleVentilationUnitDeclaration() epathRealSQLDirectHVACComponent {
	d := epathRealSQLDirectHVACComponent{ID: epathSQLSimpleVentilationFanFamily, Service: "fans", Carrier: "electricity", SiteID: "fans.electricity", Frequency: "Monthly", AggregationBasis: "model_total", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: epathSQLSimpleVentilationFanName, Unit: "J"}}}}
	for _, zone := range []string{"ZONE 1", "ZONE 2", "ZONE 3"} {
		d.Source.Keys = append(d.Source.Keys, zone)
		d.Owners = append(d.Owners, epathRealSQLDirectHVACOwner{KeyValue: zone, ZoneName: zone, EquipmentType: "Zone", EquipmentName: zone, ComponentType: "Zone"})
	}
	return d
}

func epathSQLSimpleVentilationUnitOriginal() string {
	return `Version,25.1;
Zone,ZONE 1,0,0,0,0,1,1;
Zone,ZONE 2,0,0,0,0,1,1;
Zone,ZONE 3,0,0,0,0,1,1;
Schedule:Constant,Constant,,1;
ZoneVentilation:DesignFlowRate,ZONE 1 Ventl 1,ZONE 1,Constant,Flow/Zone,1,,,,Natural,0,1;
ZoneVentilation:DesignFlowRate,ZONE 2 Ventl 1,ZONE 2,Constant,Flow/Zone,1,,,,Intake,400,0.9;
ZoneVentilation:DesignFlowRate,ZONE 3 Ventl 1,ZONE 3,Constant,Flow/Zone,1,,,,Exhaust,400,0.8;
ZoneVentilation:WindandStackOpenArea,ZONE 3 Ventl 2,ZONE 3,0.5,Constant,0.2,0,1,0.5;`
}

func TestEnergyPathRealSQLSimpleVentilationOriginalExactCohort(t *testing.T) {
	path := filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "VentilationSimpleTest.idf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if epathRealHash(data) != "e38887ebcbfec13df596bcd77ed1d5965a65a4bd1e0ad026672f449d8fbe1074" {
		t.Fatal("exact vendored original changed")
	}
	model := epathRealSQLModel{DirectHVACComponents: []epathRealSQLDirectHVACComponent{epathSQLSimpleVentilationUnitDeclaration()}}
	for _, text := range []string{string(data), epathSQLSimpleVentilationUnitOriginal()} {
		if err := epathSQLValidateDirectHVACOriginalModel(text, model); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(data, after) {
		t.Fatal("original proof changed source bytes")
	}
	// Existing packaged fan family remains exact, distinct and unchanged.
	name, component, service, carrier, ok := epathSQLDirectHVACTaxonomy(epathSQLDirectFanFamily)
	if !ok || name != "Fan Electricity Energy" || component != "Fan:OnOff" || service != "fans" || carrier != "electricity" {
		t.Fatal("existing native fan taxonomy changed")
	}
}

func TestEnergyPathRealSQLSimpleVentilationOriginalRejectsHiddenOrPartialOwners(t *testing.T) {
	base := epathSQLSimpleVentilationUnitOriginal()
	for _, tc := range []struct{ name, old, replacement string }{
		{"missing Zone 3 second member", "ZoneVentilation:WindandStackOpenArea,ZONE 3 Ventl 2,ZONE 3,0.5,Constant,0.2,0,1,0.5;", ""},
		{"second member moved", "ZONE 3 Ventl 2,ZONE 3", "ZONE 3 Ventl 2,ZONE 2"},
		{"member clone alias", "ZONE 3 Ventl 2,ZONE 3", "Other opening,ZONE 3"},
		{"member name duplicate", "ZONE 3 Ventl 2,ZONE 3", "ZONE 2 Ventl 1,ZONE 3"},
		{"unreviewed version", "Version,25.1", "Version,24.2"},
		{"Zone multiplier changed", "Zone,ZONE 2,0,0,0,0,1,1", "Zone,ZONE 2,0,0,0,0,1,4"},
		{"wrong physical fan role", "Intake,400,0.9", "Natural,400,0.9"},
		{"nonfinite pressure", "Intake,400,0.9", "Intake,NaN,0.9"},
		{"negative pressure", "Intake,400,0.9", "Intake,-1,0.9"},
		{"zero efficiency", "Intake,400,0.9", "Intake,400,0"},
		{"foreign owner", "ZONE 2 Ventl 1,ZONE 2", "ZONE 2 Ventl 1,Other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := strings.Replace(base, tc.old, tc.replacement, 1)
			if text == base {
				t.Fatal("ineffective test mutation")
			}
			if err := epathSQLValidateDirectHVACOriginalModel(text, epathRealSQLModel{DirectHVACComponents: []epathRealSQLDirectHVACComponent{epathSQLSimpleVentilationUnitDeclaration()}}); err == nil {
				t.Fatal("invalid original accepted")
			}
		})
	}
	for _, extra := range []string{
		"Zone,ZONE 2,0,0,0,0,1,1;", "Version,25.1;", "Zone,Other,0,0,0,0,1,1;",
		"ZoneList,ZONE 2,ZONE 3;", "Space,ZONE 2,ZONE 3;", "ZoneGroup,Repeated,List,2;",
		"ZoneAirBalance:OutdoorAir,Balance,ZONE 2,Quadrature;",
		"ZoneVentilation:DesignFlowRate,Hidden,ZONE 2,Constant,Flow/Zone,1,,,,Intake,400,0.9;",
		"ZoneVentilation:DesignFlowRate,ZONE 2 Ventl 1,ZONE 2,Constant,Flow/Zone,1,,,,Intake,400,0.9;",
		"Fan:SystemModel,Disconnected fan;", "ZoneEarthtube,Hidden,ZONE 2;", "Coil:Heating:Electric,Hidden;",
	} {
		if err := epathSQLValidateDirectHVACOriginalModel(base+"\n"+extra, epathRealSQLModel{DirectHVACComponents: []epathRealSQLDirectHVACComponent{epathSQLSimpleVentilationUnitDeclaration()}}); err == nil {
			t.Fatalf("hidden/ambiguous physical source accepted: %s", extra)
		}
	}
	for _, mutate := range []func(*epathRealSQLDirectHVACComponent){
		func(d *epathRealSQLDirectHVACComponent) { d.Owners = d.Owners[:2]; d.Source.Keys = d.Source.Keys[:2] },
		func(d *epathRealSQLDirectHVACComponent) { d.Owners[1].EquipmentName = "ZONE 2 Ventl 1" },
		func(d *epathRealSQLDirectHVACComponent) {
			d.Owners[1].KeyValue = "ZONE 2 Ventl 1"
			d.Source.Keys[1] = "ZONE 2 Ventl 1"
		},
		func(d *epathRealSQLDirectHVACComponent) { d.Owners[1].ZoneName = "ZONE 3" },
		func(d *epathRealSQLDirectHVACComponent) { d.Owners[1].ComponentType = "Fan:OnOff" },
		func(d *epathRealSQLDirectHVACComponent) { d.Source.AllowAbsent = true },
	} {
		d := epathSQLSimpleVentilationUnitDeclaration()
		mutate(&d)
		if _, err := epathSQLDirectHVACOwners(d); err == nil {
			t.Fatal("partial/foreign declaration accepted")
		}
	}
}

// Literal private SQLite observations, not an engine capture: Monthly values
// are 0, 2 and 3 kWh in all twelve months. Broad Fans is 5 each month. Nothing
// in this fixture proves actual VentilationSimpleTest heating or fan amounts.
func epathSQLSimpleVentilationUnitSQL(t *testing.T) (string, epathRealSQLModel, PurposeRunPlan) {
	t.Helper()
	path := epathOracleUnitSQL(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	exec := func(query string) {
		t.Helper()
		if _, err := tx.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	exec(`DELETE FROM ReportData; DELETE FROM ReportDataDictionary;
ALTER TABLE ReportDataDictionary ADD COLUMN Type TEXT;
ALTER TABLE ReportDataDictionary ADD COLUMN TimestepType TEXT;
ALTER TABLE ReportDataDictionary ADD COLUMN ScheduleName TEXT;`)
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}, DirectHVACComponents: []epathRealSQLDirectHVACComponent{epathSQLSimpleVentilationUnitDeclaration()},
		Site:        []epathRealSQLSite{{ID: "fans.electricity", EndUse: "fans", Carrier: "electricity", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Fans:Electricity", Unit: "J"}}, Keys: []string{"*"}, IsMeter: true}}},
		Auxiliaries: []epathRealSQLAuxiliary{{SiteID: "fans.electricity", ServedZones: []string{"ZONE 1", "ZONE 2", "ZONE 3"}, Weight: "native_direct", AllocationMethod: "direct_only", ReconciliationID: "reconcile.zone_auxiliary_allocation.fans.electricity.annual"}}}
	plan := PurposeRunPlan{}
	for i, value := range []int{0, 2, 3} {
		id, zone := 40+i, fmt.Sprintf("ZONE %d", i+1)
		exec(fmt.Sprintf(`INSERT INTO ReportDataDictionary VALUES(%d,'Zone Ventilation Fan Electricity Energy','%s',0,'Monthly','J','Zone','Sum','HVAC System',''); INSERT INTO ReportData SELECT %d*100+TimeIndex,%d,TimeIndex,%d*3600000 FROM Time WHERE TimeIndex BETWEEN 1 AND 12;`, id, zone, id, id, value))
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", VariableName: epathSQLSimpleVentilationFanName, KeyValue: zone, ReportingFrequency: "Monthly", ScopeZoneName: zone, PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	}
	exec(`INSERT INTO ReportDataDictionary VALUES(50,'Fans:Electricity','',1,'Monthly','J','Facility','Sum','Zone',''); INSERT INTO ReportData SELECT 5000+TimeIndex,50,TimeIndex,5*3600000 FROM Time WHERE TimeIndex BETWEEN 1 AND 12;`)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path, model, plan
}

func epathSQLSimpleVentilationUnitFrames(path string, model epathRealSQLModel) (epathSQLFrames, error) {
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}, SourceZone: map[int]string{}}
	for _, zone := range []string{"ZONE 1", "ZONE 2", "ZONE 3"} {
		frames.Zones[strings.ToLower(zone)] = epathSQLZone{Name: zone, Multiplier: 1}
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		return frames, err
	}
	for _, source := range observed.Sources {
		if source.DictionaryIndex != 50 {
			continue
		}
		values, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			return frames, err
		}
		for _, value := range values {
			q := value
			frames.Site["fans.electricity"] = append(frames.Site["fans.electricity"], &q)
		}
		frames.SiteSources["fans.electricity"] = []int{50}
		frames.SourceIdentities[50] = source
	}
	err = epathCompileSQLDirectHVACSources(path, observed.Sources, model, &frames)
	return frames, err
}

func TestEnergyPathRealSQLSimpleVentilationNativeFramesAndConsumers(t *testing.T) {
	path, model, plan := epathSQLSimpleVentilationUnitSQL(t)
	if err := epathSQLValidateDirectHVACRequests(&plan, model); err != nil {
		t.Fatal(err)
	}
	before := epathRealFileHash(t, path)
	frames, err := epathSQLSimpleVentilationUnitFrames(path, model)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames.DirectHVACSourceIdentities) != 3 || len(frames.DirectHVAC) != 36 {
		t.Fatal("four members must remain three actual sources / 36 monthly observations")
	}
	for month := 1; month <= 12; month++ {
		zero := frames.DirectHVAC[epathSQLDirectHVACKey("ZONE 1", "fans", "electricity", month)]
		if !zero.Present || zero.Quantity.Value != 0 || zero.Quantity.Error != 0 || !reflect.DeepEqual(zero.SourceIDs, []int{40}) {
			t.Fatal("actual hand-observed zero lost source/knownness")
		}
	}
	// Direct fan native meter contribution never takes an extra Zone factor.
	frames.Zones["zone 2"] = epathSQLZone{Name: "ZONE 2", Multiplier: 8}
	fans, err := epathSQLCompileDirectFans(frames, model)
	if err != nil {
		t.Fatal(err)
	}
	ledger, allowed, err := epathSQLDirectFanLedgerPresentation(fans, "annual")
	if err != nil || ledger.ExpectedValue != 60 || ledger.DirectValue != 60 || ledger.AllocatedValue != 0 || ledger.UnassignedValue != 0 || ledger.OvermappedValue != 0 || ledger.AllocationMethod != "direct_only" || len(allowed) != 4 {
		t.Fatalf("native source/parent double count, multiplier or allocation: %#v %v", ledger, err)
	}
	for _, tc := range []struct {
		zone  string
		value float64
	}{{"ZONE 1", 0}, {"ZONE 2", 24}, {"ZONE 3", 36}} {
		proof := &epathSQLDirectFanProof{Zone: tc.zone, Period: "annual", SiteID: "fans.electricity", Value: epathSQLQuantity{Value: tc.value}, Sources: fans.Sources[strings.ToLower(tc.zone)]}
		for _, identity := range proof.Sources {
			q, _ := epathSQLDirectHVACAnnualQuantity(identity)
			proof.Value.Error += q.Error
		}
		if err := epathSQLDirectFanProofQuantity(proof); err != nil {
			t.Fatal(err)
		}
	}
	var checks epathSQLModelChecks
	if err := epathSQLModelDirectHVACSourceChecks(frames, &checks); err != nil || len(checks.Rows) != 12 {
		t.Fatalf("three exact source x two fields x two scopes: %d %v", len(checks.Rows), err)
	}
	if err := epathSQLModelDirectFanChecks(frames, model, &checks); err != nil || len(checks.Rows) != 220 {
		t.Fatalf("existing source/Zone/ledger consumers not installed: %d %v", len(checks.Rows), err)
	}
	if epathRealFileHash(t, path) != before {
		t.Fatal("oracle changed native SQL")
	}
	// The V2 source writer preserves known zeros; the legacy writer intentionally omits them.
	source := epathSQLDirectHVACSourceUnitCandidate(frames.DirectHVACSourceIdentities[40])
	for pass := 0; pass < 3; pass++ {
		if err := epathSQLMatchDirectHVACSource(source, frames.DirectHVACSourceIdentities[40]); err != nil {
			t.Fatal(err)
		}
		if source.RawValue != 0 || source.EffectiveValue != 0 || !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) {
			t.Fatal("literal zero source became unknown")
		}
		wire, err := json.Marshal(energyPathSourceWire{source})
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(wire, &source); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnergyPathRealSQLSimpleVentilationNativeAdversarial(t *testing.T) {
	for _, query := range []string{
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=40`,
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=41 AND TimeIndex=2`,
		`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=40 AND TimeIndex=1`,
		`UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=41 AND TimeIndex=1`,
		`INSERT INTO ReportData VALUES(99999,41,1,7200000)`,
		`UPDATE ReportDataDictionary SET KeyValue='ZONE 2 Ventl 1' WHERE ReportDataDictionaryIndex=41`,
		`UPDATE ReportDataDictionary SET ReportingFrequency='Hourly' WHERE ReportDataDictionaryIndex=40`,
		`UPDATE ReportDataDictionary SET Units='kJ' WHERE ReportDataDictionaryIndex=41`,
		`UPDATE ReportDataDictionary SET IsMeter=1 WHERE ReportDataDictionaryIndex=41`,
		`UPDATE ReportDataDictionary SET Type='Avg' WHERE ReportDataDictionaryIndex=41`,
		`UPDATE ReportDataDictionary SET TimestepType='Zone' WHERE ReportDataDictionaryIndex=41`,
		`UPDATE ReportDataDictionary SET ScheduleName='Reporting filter' WHERE ReportDataDictionaryIndex=41`,
		`INSERT INTO ReportDataDictionary SELECT 90,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup,Type,TimestepType,ScheduleName FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=41`,
		`INSERT INTO ReportDataDictionary SELECT 90,Name,'Other',IsMeter,ReportingFrequency,Units,IndexGroup,Type,TimestepType,ScheduleName FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=41`,
	} {
		path, model, _ := epathSQLSimpleVentilationUnitSQL(t)
		epathOracleEditSQL(t, path, query)
		frames, err := epathSQLSimpleVentilationUnitFrames(path, model)
		if err == nil || len(frames.DirectHVAC) != 0 || len(frames.DirectHVACSourceIdentities) != 0 {
			t.Fatalf("bad native evidence accepted or partially committed: %s / %v", query, err)
		}
	}
	for _, mode := range []string{"missing", "Hourly", "wildcard", "wrong scope", "not Basic", "duplicate"} {
		_, model, plan := epathSQLSimpleVentilationUnitSQL(t)
		switch mode {
		case "missing":
			plan.OutputObjects = plan.OutputObjects[1:]
		case "Hourly":
			plan.OutputObjects[0].ReportingFrequency = "Hourly"
		case "wildcard":
			plan.OutputObjects[0].KeyValue = "*"
		case "wrong scope":
			plan.OutputObjects[0].ScopeZoneName = "ZONE 2"
		case "not Basic":
			plan.OutputObjects[0].PurposeIDs = []SimulationPurposeID{SimulationPurposeComfort}
		case "duplicate":
			plan.OutputObjects = append(plan.OutputObjects, plan.OutputObjects[0])
		}
		if err := epathSQLValidateDirectHVACRequests(&plan, model); err == nil {
			t.Fatalf("request %s accepted", mode)
		}
	}
}

func TestEnergyPathRealSQLSimpleVentilationRejectsForeignFanCohorts(t *testing.T) {
	path, model, _ := epathSQLSimpleVentilationUnitSQL(t)
	frames, err := epathSQLSimpleVentilationUnitFrames(path, model)
	if err != nil {
		t.Fatal(err)
	}
	fans, err := epathSQLCompileDirectFans(frames, model)
	if err != nil {
		t.Fatal(err)
	}
	foreign := frames.DirectHVACSourceIdentities[41]
	foreign.FamilyID, foreign.Owner.EquipmentType, foreign.Owner.EquipmentName, foreign.Owner.ComponentType = epathSQLDirectFanFamily, epathSQLWindowACType, "Foreign package", "Fan:OnOff"
	foreign.Owner.KeyValue, foreign.Source.KeyValue, foreign.Source.Name, foreign.Source.DictionaryIndex = "Foreign fan", "Foreign fan", "Fan Electricity Energy", 90
	frames.DirectHVACSourceIdentities[90] = foreign
	if _, err := epathSQLCompileDirectFans(frames, model); err == nil {
		t.Fatal("mixed family escaped exact declared fan cohort")
	}
	delete(frames.DirectHVACSourceIdentities, 90)
	without := model
	without.DirectHVACComponents = nil
	if _, err := epathSQLCompileDirectFans(frames, without); err == nil {
		t.Fatal("retained ventilation source escaped deleted family declaration")
	}
	duplicate := model
	duplicate.DirectHVACComponents = append(append([]epathRealSQLDirectHVACComponent(nil), model.DirectHVACComponents...), model.DirectHVACComponents[0])
	if _, err := epathSQLCompileDirectFans(frames, duplicate); err == nil {
		t.Fatal("duplicate family declaration accepted")
	}
	fans.Sources["zone 2"]["sql-rdd-90"] = foreign
	if _, _, err := epathSQLDirectFanLedgerPresentation(fans, "annual"); err == nil {
		t.Fatal("mixed source-family ledger accepted")
	}
	proof := &epathSQLDirectFanProof{Zone: "ZONE 2", Period: "annual", SiteID: "fans.electricity", Value: epathSQLQuantity{Value: 48}, Sources: fans.Sources["zone 2"]}
	if err := epathSQLDirectFanProofQuantity(proof); err == nil {
		t.Fatal("mixed consumer source-family proof accepted")
	}
}
