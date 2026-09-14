package simulation

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const hotWaterEquipmentHandIDF = `Version,25.1;
Zone,Office,0,0,0,0,1,4;
Zone,Lab,0,0,0,0,1,2;
ZoneList,Repeated Office,Office;
ZoneGroup,Office Group,Repeated Office,8;
ZoneList,All Zones,Office,Lab;
Schedule:Constant,Always,,1;
HotWaterEquipment,Hot Water Use,All Zones,Always,EquipmentLevel,100,,,0.2,0.1,0.5,General;
`

func TestEnergyPathHotWaterEquipmentPlanRequestsPurchasedEquipmentNotHeating(t *testing.T) {
	doc := parsePurposePlanFixture(t, hotWaterEquipmentHandIDF)
	original := doc.String()
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	for _, name := range []string{"DistrictHeatingWater:Facility", "DistrictHeating:Facility", "InteriorEquipment:DistrictHeatingWater", "InteriorEquipment:DistrictHeating"} {
		for _, frequency := range []string{"Monthly", "Hourly"} {
			outputs := epathDistrictPurposeOutputs(plan, "Output:Meter", name, "", frequency)
			if len(outputs) != 1 || !purposeIDsContain(outputs[0].PurposeIDs, SimulationPurposeBasicEnergy) {
				t.Fatalf("missing/duplicate purchased-equipment request %s/%s: %#v", name, frequency, outputs)
			}
		}
	}
	for _, output := range plan.OutputObjects {
		if output.ObjectType == "Output:Meter" && strings.Contains(strings.ToLower(output.KeyValue), "heating:") {
			// DistrictHeating:Facility is the legitimate pre-23.1 resource name.
			if !strings.EqualFold(output.KeyValue, "DistrictHeating:Facility") {
				t.Fatalf("HotWaterEquipment invented space-heating consumption: %#v", output)
			}
		}
	}
	for _, zone := range []string{"Office", "Lab"} {
		for _, frequency := range []string{"Monthly", "Hourly"} {
			if rows := epathDistrictPurposeOutputs(plan, "Output:Variable", zone, "Zone Hot Water Equipment District Heating Energy", frequency); len(rows) != 1 {
				t.Fatalf("representative Zone direct-use request missing: %s/%s", zone, frequency)
			}
		}
	}
	if doc.String() != original {
		t.Fatal("output plan rewrote the original physical input")
	}
}

func TestEnergyPathHotWaterEquipmentMeterAliasesRemainInteriorEquipment(t *testing.T) {
	for _, name := range []string{"InteriorEquipment:DistrictHeatingWater", "InteriorEquipment:DistrictHeating", "DistrictHeatingWater:InteriorEquipment", "DistrictHeating:InteriorEquipment"} {
		definition, ok := energyMeterAliasDefinitionForName(name)
		if !ok || definition.Kind != "energy.interior_equipment" || definition.EndUse != "interior_equipment" || definition.Carrier != "district_heating" || definition.HierarchyLevel != "broad_end_use" || definition.FacilityTotal {
			t.Fatalf("district equipment meter lost its physical boundary: %s %#v", name, definition)
		}
	}
}

// These are hand observations, not an engine acceptance fixture. Office raw10
// has native Zone4 x Group8; Lab raw5 has Zone2. Both model-total meters are330,
// not a second consuming branch or a quantity to multiply by32 again.
func hotWaterEquipmentSQLFixture(t *testing.T, resource string) (idf.Document, string, PurposeRunPlan) {
	t.Helper()
	doc := parsePurposePlanFixture(t, hotWaterEquipmentHandIDF)
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}})
	path := directHVACSQLFixture(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{`DELETE FROM ReportData`, `DELETE FROM ReportDataDictionary`} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		id        int
		key, name string
		meter     int
		value     float64
	}{
		{1, "", resource + ":Facility", 1, 330},
		{2, "", "InteriorEquipment:" + resource, 1, 330},
		{3, "Office", "Zone Hot Water Equipment District Heating Energy", 0, 10},
		{4, "Lab", "Zone Hot Water Equipment District Heating Energy", 0, 5},
		{5, "Office", "Zone Air System Sensible Heating Energy", 0, 0},
		{6, "Lab", "Zone Air System Sensible Heating Energy", 0, 0},
		{7, "Office", "Zone Air System Sensible Cooling Energy", 0, 0},
		{8, "Lab", "Zone Air System Sensible Cooling Energy", 0, 0},
	} {
		if _, err := db.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,'J',?,'Monthly','Zone')`, row.id, row.key, row.name, row.meter); err != nil {
			t.Fatal(err)
		}
		for month := 1; month <= 2; month++ {
			value := row.value
			if month == 2 {
				value = 0
			}
			if _, err := db.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(?,?,?)`, month, row.id, value*3600000); err != nil {
				t.Fatal(err)
			}
		}
	}
	return doc, path, plan
}

func TestEnergyPathHotWaterEquipmentSQLDirectUseAndModelTotalMeterCloseOnce(t *testing.T) {
	for _, resource := range []string{"DistrictHeatingWater", "DistrictHeating"} {
		t.Run(resource, func(t *testing.T) {
			doc, path, plan := hotWaterEquipmentSQLFixture(t, resource)
			legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
			if err != nil {
				t.Fatal(err)
			}
			result := UpgradeEnergyExplanationV1(legacy)
			for reload := 0; reload < 3; reload++ {
				assertMixedHeatingNode(t, result.Nodes, "carrier", "district_heating", 330)
				assertMixedHeatingNode(t, result.Nodes, "end_use", "equipment", 330)
				for _, node := range result.Nodes {
					if node.Level == "end_use" && node.EndUse == "heating" && node.Value > 0 {
						t.Fatalf("purchased equipment became space heating: %#v", node)
					}
				}
				for _, link := range result.Links {
					if link.Relation == "load_to_end_use" && link.ToValue > 0 {
						t.Fatalf("direct interior equipment invented HVAC thermal conversion: %#v", link)
					}
				}
				for _, row := range []struct {
					id                     string
					raw, effective, factor float64
				}{
					{"sql-rdd-1", 330, 330, 1}, {"sql-rdd-2", 330, 330, 1},
					{"sql-rdd-3", 10, 320, 32}, {"sql-rdd-4", 5, 10, 2},
				} {
					source := energyExplanationSourceByID(result.Sources, row.id)
					if source == nil || source.RawValue != row.raw || source.EffectiveValue != row.effective || source.EffectiveMultiplier != row.factor || !energyDataSourceValueKnown(*source, energySourceObservedRaw) || !energyDataSourceValueKnown(*source, energySourceObservedEffective) {
						t.Fatalf("purchased/representative boundary changed across reload%d: %s %#v", reload, row.id, source)
					}
				}
				zones := map[string]bool{}
				for _, zone := range result.ZoneResults {
					want, found := map[string]float64{"Office": 320, "Lab": 10}[zone.Scope.ZoneName]
					if !found || zones[zone.Scope.ZoneName] {
						t.Fatalf("unexpected/duplicate equipment Zone %q", zone.Scope.ZoneName)
					}
					zones[zone.Scope.ZoneName] = true
					assertMixedHeatingNode(t, zone.Nodes, "carrier", "district_heating", want)
					assertMixedHeatingNode(t, zone.Nodes, "end_use", "equipment", want)
				}
				if len(zones) != 2 {
					t.Fatalf("missing direct-use Zone: %v", zones)
				}
				if reload == 2 {
					break
				}
				wire, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var stored EnergyExplanationResult
				if err := json.Unmarshal(wire, &stored); err != nil {
					t.Fatal(err)
				}
				result = stored
			}
		})
	}
}

func TestEnergyPathHotWaterEquipmentSQLMissingIsNotMeasuredZero(t *testing.T) {
	for _, mutation := range []string{
		`UPDATE ReportData SET Value=0`,
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex IN(1,2)`,
		`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=4`,
	} {
		t.Run(mutation, func(t *testing.T) {
			doc, path, plan := hotWaterEquipmentSQLFixture(t, "DistrictHeatingWater")
			epathOracleEditSQL(t, path, mutation)
			legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
			if err != nil {
				t.Fatal(err)
			}
			result := UpgradeEnergyExplanationV1(legacy)
			for _, id := range []string{"sql-rdd-1", "sql-rdd-2", "sql-rdd-3", "sql-rdd-4"} {
				source := energyExplanationSourceByID(result.Sources, id)
				if mutation == `UPDATE ReportData SET Value=0` {
					if source == nil || source.RawValue != 0 || source.EffectiveValue != 0 || !energyDataSourceValueKnown(*source, energySourceObservedRaw) || !energyDataSourceValueKnown(*source, energySourceObservedEffective) {
						t.Fatalf("native measured zero lost: %s %#v", id, source)
					}
				} else if strings.HasPrefix(mutation, "DELETE") && (id == "sql-rdd-1" || id == "sql-rdd-2") || strings.Contains(mutation, "NULL") && id == "sql-rdd-4" {
					if source != nil && (energyDataSourceValueKnown(*source, energySourceObservedRaw) || energyDataSourceValueKnown(*source, energySourceObservedEffective)) {
						t.Fatalf("missing/NULL purchased evidence became known: %s %#v", id, source)
					}
				}
			}
		})
	}
}

func TestEnergyPathHotWaterEquipmentSQLAliasesAndAnnualFallbackAreNotAdditive(t *testing.T) {
	doc, path, plan := hotWaterEquipmentSQLFixture(t, "DistrictHeatingWater")
	// Unequal shadow observations expose both summation and incorrect priority.
	// Only the canonical Monthly330 is authoritative. An annual table is not
	// permission to overwrite native monthly measurements or fill their zeros.
	epathOracleEditSQL(t, path, `
INSERT INTO ReportDataDictionary VALUES
 (201,'','InteriorEquipment:DistrictHeating','J',1,'Monthly','Meter'),
 (202,'','DistrictHeatingWater:InteriorEquipment','J',1,'Monthly','Meter'),
 (203,'','DistrictHeating:InteriorEquipment','J',1,'Monthly','Meter'),
 (204,'','DistrictHeating:Facility','J',1,'Monthly','Meter');
INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES
 (1,201,3600000000),(2,201,0),(1,202,7200000000),(2,202,0),
 (1,203,10800000000),(2,203,0),(1,204,14400000000),(2,204,0);
CREATE TABLE TabularDataWithStrings(TabularDataIndex INTEGER,ReportName TEXT,ReportForString TEXT,TableName TEXT,RowName TEXT,ColumnName TEXT,Units TEXT,Value TEXT);
INSERT INTO TabularDataWithStrings VALUES
 (1,'AnnualBuildingUtilityPerformanceSummary','Entire Facility','End Uses','Interior Equipment','District Heating Water','kWh','1999.00'),
 (2,'AnnualBuildingUtilityPerformanceSummary','Entire Facility','End Uses','Total End Uses','District Heating Water','kWh','2999.00');`)
	legacy, err := parseSimulationEnergyExplanationSQLWithDriverContext(path, &plan, newEnergyDriverBuildContext(idf.AnalyzeGeometry(doc), doc))
	if err != nil {
		t.Fatal(err)
	}
	result := UpgradeEnergyExplanationV1(legacy)
	for reload := 0; reload < 3; reload++ {
		assertMixedHeatingNode(t, result.Nodes, "carrier", "district_heating", 330)
		assertMixedHeatingNode(t, result.Nodes, "end_use", "equipment", 330)
		for _, period := range result.Periods {
			if period.ID == "M1" {
				assertMixedHeatingNode(t, period.Nodes, "carrier", "district_heating", 330)
				assertMixedHeatingNode(t, period.Nodes, "end_use", "equipment", 330)
			}
			if period.ID == "M2" {
				for _, node := range period.Nodes {
					if (node.Level == "carrier" || node.Level == "end_use") && node.Value != 0 {
						t.Fatalf("annual/shadow alias filled native Monthly zero: %#v", node)
					}
				}
			}
		}
		if reload == 2 {
			break
		}
		wire, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var stored EnergyExplanationResult
		if err := json.Unmarshal(wire, &stored); err != nil {
			t.Fatal(err)
		}
		result = stored
	}
}
