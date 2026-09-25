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

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLHeatOnlyBindingUnit(t *testing.T) (epathRealOracleEvidence, epathRealSQLModel, epathSQLFrames) {
	t.Helper()
	original, d := epathSQLHeatOnlyOriginalFixture(t)
	path := epathFanPoolUnitSQL(t)
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
	for _, query := range []string{
		`CREATE TABLE Zones(ZoneName TEXT,Multiplier REAL,ListMultiplier REAL)`,
		`INSERT INTO Zones VALUES('WEST ZONE',1,1),('EAST ZONE',1,1),('NORTH ZONE',1,1)`,
		`ALTER TABLE ReportDataDictionary ADD COLUMN Type TEXT DEFAULT 'Sum'`,
		`ALTER TABLE ReportDataDictionary ADD COLUMN TimestepType TEXT DEFAULT 'HVAC System'`,
		`ALTER TABLE ReportDataDictionary ADD COLUMN ScheduleName TEXT`,
		`UPDATE ReportDataDictionary SET TimestepType='Zone',IndexGroup='Facility:Electricity:Fans' WHERE ReportDataDictionaryIndex=303`,
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=202`,
		`DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=202`,
		`UPDATE ReportDataDictionary SET KeyValue='TYPICAL TERMINAL REHEAT 1' WHERE ReportDataDictionaryIndex=101`,
		`UPDATE ReportData SET Value=0 WHERE TimeIndex IN (SELECT TimeIndex FROM Time WHERE Month<>1)`,
		`UPDATE ReportData SET Value=100*3600000 WHERE ReportDataDictionaryIndex=303 AND TimeIndex=20001`,
	} {
		if _, err = tx.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	observed.originalText = original
	observed.executedText = "Output:Variable,Typical Terminal Reheat 1,Air System Fan Electricity Energy,Hourly;\nOutput:Meter,Fans:Electricity,Monthly;\nOutput:SQLite,SimpleAndTabular;\n" + original
	observed.outputPlan = &PurposeRunPlan{BasicEnergyDetail: "energy_path", OutputObjects: []PurposeOutputObject{
		{ObjectType: "Output:Variable", VariableName: "Air System Fan Electricity Energy", KeyValue: "Typical Terminal Reheat 1", ReportingFrequency: "Hourly", State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Value: "Typical Terminal Reheat 1"}, {Value: "Air System Fan Electricity Energy"}, {Value: "Hourly"}}},
		{ObjectType: "Output:Meter", KeyValue: "Fans:Electricity", ReportingFrequency: "Monthly", State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Value: "Fans:Electricity"}, {Value: "Monthly"}}},
	}}
	model := epathRealSQLModel{HeatOnlyFurnaces: []epathRealSQLHeatOnlyFurnace{d}, Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1},
		FanPools:    []epathRealSQLFanPool{{SiteID: "fans.electricity", Name: "Air System Fan Electricity Energy", Key: d.AirLoopName, Frequency: "Hourly", Unit: "J", ServedZones: append([]string(nil), d.ServedZones...)}},
		Services:    []epathRealSQLService{{Service: "heating", SiteIDs: []string{"heating.electricity", "heating.natural_gas"}, ServedZones: append([]string(nil), d.ServedZones...), Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: "efficiency", FallbackRatioKind: "efficiency", CarrierReconciliationIDs: map[string]string{"electricity": "reconcile.zone_hvac_allocation.heating.electricity.annual", "natural_gas": "reconcile.zone_hvac_allocation.heating.natural_gas.annual"}}},
		Auxiliaries: []epathRealSQLAuxiliary{{SiteID: "fans.electricity", ServedZones: append([]string(nil), d.ServedZones...), Weight: "cooling_plus_heating", AllocationMethod: "air_loop_load_share", ReconciliationID: "reconcile.zone_auxiliary_allocation.fans.annual"}}}
	for _, s := range []struct{ id, name, endUse, carrier string }{{"fans.electricity", "Fans:Electricity", "fans", "electricity"}, {"heating.electricity", "Heating:Electricity", "heating", "electricity"}, {"heating.natural_gas", "Heating:NaturalGas", "heating", "natural_gas"}} {
		model.Site = append(model.Site, epathRealSQLSite{ID: s.id, EndUse: s.endUse, Carrier: s.carrier, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: s.name, Unit: "J"}}, Keys: []string{""}, IsMeter: true}})
	}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{}, Loads: map[string]epathSQLQuantity{}, Site: map[string][]*epathSQLQuantity{"fans.electricity": {}}}
	for month := 1; month <= 12; month++ {
		budget := 0.0
		if month == 1 {
			budget = 100
		}
		frames.Site["fans.electricity"] = append(frames.Site["fans.electricity"], &epathSQLQuantity{Value: budget})
		for _, zone := range d.ServedZones {
			frames.Zones[strings.ToLower(zone)] = epathSQLZone{Name: zone, Multiplier: 1}
			weight := 0.0
			if month == 1 {
				weight = 1
				if strings.EqualFold(zone, "East Zone") {
					weight = 2
				}
			}
			frames.Loads[epathSQLKey(zone, "heating", month)] = epathSQLQuantity{Value: weight}
			frames.Loads[epathSQLKey(zone, "cooling", month)] = epathSQLQuantity{}
		}
	}
	return observed, model, frames
}

func epathSQLHeatOnlyClone[T any](t *testing.T, value T) T {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Copy only closed literal unit-fixture SQL, never captures or expectations.
// Every mutation owns its file and mutable evidence; private input bindings
// are retained explicitly rather than lost through an evidence JSON clone.
func epathSQLHeatOnlyUnitSQLCopy(t *testing.T, baseline epathRealOracleEvidence, data []byte) epathRealOracleEvidence {
	t.Helper()
	out := baseline
	out.sqlPath = filepath.Join(t.TempDir(), "heat-only-copy.sql")
	if err := os.WriteFile(out.sqlPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	out.Sources = epathSQLHeatOnlyClone(t, baseline.Sources)
	out.Weather = epathSQLHeatOnlyClone(t, baseline.Weather)
	plan := epathSQLHeatOnlyClone(t, *baseline.outputPlan)
	out.outputPlan = &plan
	return out
}

func TestEnergyPathSQLHeatOnlyBindingOriginalExecutedAndFiniteInputs(t *testing.T) {
	observed, model, _ := epathSQLHeatOnlyBindingUnit(t)
	b, err := epathSQLCompileHeatOnlyBinding(observed, model)
	if err != nil {
		t.Fatal(err)
	}
	if b.Original.OriginalSHA256 == b.Executed.OriginalSHA256 || b.Original.Owners[0].ObjectIndex == b.Executed.Owners[0].ObjectIndex || len(b.Original.ServedZones) != 3 {
		t.Fatal("original/executed index or controller/recipient identity was conflated")
	}
	if b.Native.HourlyID != 101 || b.Native.MonthlyMeterID != 303 || b.Native.Months[0].NonZero != 744 || b.Native.Months[1].Hours != 672 || b.Native.Months[1].NonZero != 0 || b.Native.Months[1].BroadJ != 0 {
		t.Fatalf("literal active/known-zero native proof changed: %+v", b.Native)
	}
	model.HeatOnlyFurnaces[0].ServedZones[0] = "Aliased"
	model.FanPools[0].ServedZones[0] = "Aliased"
	model.Services[0].CarrierReconciliationIDs["electricity"] = "Aliased"
	if b.Inputs.Furnaces[0].ServedZones[0] != "West Zone" || b.Inputs.FanPools[0].ServedZones[0] != "West Zone" || b.Inputs.Services[0].CarrierReconciliationIDs["electricity"] == "Aliased" {
		t.Fatal("HeatOnly binding retained mutable declaration aliases")
	}
	if err = epathSQLValidateHeatOnlyBinding(b, nil); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"controller-only", "missing-executed", "added-cooling", "added-loose-boiler", "changed-physical", "foreign-pool", "changed-service", "deleted-furnace", "borrowed-system", "missing-plan", "scheduled-output", "foreign-request"} {
		t.Run(mutation, func(t *testing.T) {
			o, m := observed, epathSQLHeatOnlyModelFrom(epathSQLHeatOnlyClone(t, b.Inputs))
			switch mutation {
			case "controller-only":
				m.Services[0].ServedZones = []string{"East Zone"}
			case "missing-executed":
				o.executedText = ""
			case "added-cooling":
				o.executedText += "\nZoneHVAC:IdealLoadsAirSystem,Hidden;"
			case "added-loose-boiler":
				o.originalText += "\nBoiler:HotWater,Hidden;"
				o.executedText += "\nBoiler:HotWater,Hidden;"
			case "changed-physical":
				doc, e := idf.Parse(o.executedText)
				if e != nil {
					t.Fatal(e)
				}
				for i := range doc.Objects {
					if strings.EqualFold(doc.Objects[i].Type, "Fan:OnOff") {
						doc.Objects[i].Fields[3].Value = "601"
					}
				}
				o.executedText = doc.String()
			case "foreign-pool":
				m.FanPools[0].Key = "Other Loop"
			case "changed-service":
				m.Services[0].RatioKind = "thermal_efficiency"
			case "deleted-furnace":
				m.HeatOnlyFurnaces = nil
			case "borrowed-system":
				m.AirLoopFans = []epathRealSQLAirLoopFan{{SiteID: "fans.electricity"}}
			case "missing-plan":
				o.outputPlan = nil
			case "scheduled-output":
				o.executedText = strings.Replace(o.executedText, "Energy,Hourly;", "Energy,Hourly,OnlySometimes;", 1)
			case "foreign-request":
				copy := epathSQLHeatOnlyClone(t, *o.outputPlan)
				copy.OutputObjects[0].KeyValue = "East Zone"
				o.outputPlan = &copy
			}
			if _, err := epathSQLCompileHeatOnlyBinding(o, m); err == nil {
				t.Fatal("unproved HeatOnly binding accepted")
			}
		})
	}
	for _, mutation := range []string{"proof-index", "native-zero-flag", "different-month", "sql-hash", "changed-retained-text"} {
		bad := epathSQLHeatOnlyClone(t, *b)
		switch mutation {
		case "proof-index":
			bad.Executed.Owners[0].ObjectIndex = b.Original.Owners[0].ObjectIndex
		case "native-zero-flag":
			bad.Native.Months[0].NonZero = 0
		case "different-month":
			bad.Native.Months[1] = bad.Native.Months[0]
		case "sql-hash":
			bad.SQLSHA256 = "forged"
		case "changed-retained-text":
			bad.ExecutedText += "\n! changed bytes"
		}
		if err := epathSQLValidateHeatOnlyBinding(&bad, nil); err == nil {
			t.Fatalf("retained proof mutation accepted: %s", mutation)
		}
	}
}

func TestEnergyPathSQLHeatOnlyFanZeroOptInDoesNotRelaxGenericGate(t *testing.T) {
	observed, model, frames := epathSQLHeatOnlyBindingUnit(t)
	b, err := epathSQLCompileHeatOnlyBinding(observed, model)
	if err != nil {
		t.Fatal(err)
	}
	if err = epathSQLModelFanPoolChecks(observed, frames, model.FanPools, model.Precision, &epathSQLModelChecks{}); err == nil {
		t.Fatal("generic fan positive-budget guard was relaxed")
	}
	checks := epathSQLModelChecks{HeatOnly: b}
	if err = epathSQLModelFanPoolChecks(observed, frames, model.FanPools, model.Precision, &checks, b); err != nil {
		t.Fatal(err)
	}
	for _, row := range checks.Rows {
		if row.Want.Scope != "zone" {
			continue
		}
		if row.Want.Period == "M2" {
			lo, hi := row.Quantity.bounds()
			if row.Quantity.Value != 0 || row.Quantity.Error != 0 || lo != 0 || hi != 0 {
				t.Fatal("inactive fan acquired uncertainty or denominator")
			}
		}
		if row.Want.Period == "M1" {
			want := 25.0
			if strings.EqualFold(row.Want.Zone, "East Zone") {
				want = 50
			}
			if row.Quantity.Value < want-1e-8 || row.Quantity.Value > want+1e-8 {
				t.Fatalf("literal 25/50/25 native pool allocation changed: %+v", row.Quantity)
			}
		}
	}
	for _, mutation := range []string{"missing-load", "unknown-load", "tiny-site", "zero-proof-removed", "foreign-key", "positive-zero-denominator"} {
		t.Run(mutation, func(t *testing.T) {
			_, _, f := epathSQLHeatOnlyBindingUnit(t)
			p := append([]epathRealSQLFanPool(nil), model.FanPools...)
			binding := b
			switch mutation {
			case "missing-load":
				delete(f.Loads, epathSQLKey("West Zone", "cooling", 2))
			case "unknown-load":
				f.Loads[epathSQLKey("West Zone", "heating", 2)] = epathSQLQuantity{Error: -1}
			case "tiny-site":
				f.Site["fans.electricity"][1].Value = 1e-12
			case "zero-proof-removed":
				binding = nil
			case "foreign-key":
				p[0].Key = "Other"
			case "positive-zero-denominator":
				for _, zone := range model.FanPools[0].ServedZones {
					f.Loads[epathSQLKey(zone, "heating", 1)] = epathSQLQuantity{}
				}
			}
			if err := epathSQLModelFanPoolChecks(observed, f, p, model.Precision, &epathSQLModelChecks{}, binding); err == nil {
				t.Fatal("unproved inactive/positive pool accepted")
			}
		})
	}
}

func TestEnergyPathSQLHeatOnlyZeroNativeRejectsIncompleteAndNearZero(t *testing.T) {
	for _, query := range []string{
		`INSERT INTO ReportData(ReportDataIndex,ReportDataDictionaryIndex,TimeIndex,Value) VALUES(99999999,101,99999999,0)`,
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=101 AND TimeIndex=745`,
		`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=101 AND TimeIndex=745`,
		`UPDATE ReportData SET TimeIndex=746 WHERE ReportDataDictionaryIndex=101 AND TimeIndex=745`,
		`UPDATE Time SET Hour=2 WHERE TimeIndex=745`,
		`UPDATE ReportData SET Value=0.000000001 WHERE ReportDataDictionaryIndex=101 AND TimeIndex=745`,
		`UPDATE ReportData SET Value=0.000000001 WHERE ReportDataDictionaryIndex=303 AND TimeIndex=20002`,
		`UPDATE ReportData SET Value=CASE TimeIndex WHEN 745 THEN 1 ELSE -1 END WHERE ReportDataDictionaryIndex=101 AND TimeIndex IN (745,746)`,
		`INSERT INTO ReportDataDictionary(ReportDataDictionaryIndex,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup) VALUES(999,'Air System Fan Electricity Energy','TYPICAL TERMINAL REHEAT 1',0,'Hourly','J','System')`,
		`UPDATE ReportDataDictionary SET Type='Avg' WHERE ReportDataDictionaryIndex=101`,
		`UPDATE ReportDataDictionary SET TimestepType='Zone' WHERE ReportDataDictionaryIndex=101`,
		`UPDATE Zones SET Multiplier=2 WHERE ZoneName='EAST ZONE'`,
	} {
		observed, model, _ := epathSQLHeatOnlyBindingUnit(t)
		epathOracleEditSQL(t, observed.sqlPath, query)
		if _, err := epathSQLCompileHeatOnlyBinding(observed, model); err == nil {
			t.Fatalf("invalid actual native zero proof accepted: %s", query)
		}
	}
}

func TestEnergyPathSQLHeatOnlyRetainedRegistryAndZeroFlow(t *testing.T) {
	observed, model, frames := epathSQLHeatOnlyBindingUnit(t)
	var checks epathSQLModelChecks
	epathSQLHeatOnlyContextUnitExtend(t, &observed)
	if err := epathSQLBindHeatOnlyChecks(observed, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelFanPoolChecks(observed, frames, model.FanPools, model.Precision, &checks, checks.HeatOnly); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelHeatOnlyContextChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	// Literal selector-only fixtures exercise retained binding, not service
	// arithmetic. The ordinary typed service consumers remain separate gates.
	for _, zone := range model.FanPools[0].ServedZones {
		for _, period := range []string{"annual", "M1", "M2", "M3", "M4", "M5", "M6", "M7", "M8", "M9", "M10", "M11", "M12"} {
			for _, field := range []string{"value", "allocatedValue"} {
				target := epathSQLNodeTarget("end_use", "heating", "", "site")
				target.Field, target.Basis, target.AllowPrunedZero = field, "service_path_allocation", true
				q := epathSQLQuantity{}
				if err := checks.add("zoneAllocation", "zone", zone, period, "hvac_zone/heating/"+field, "kWh", &q, target, "", nil, nil); err != nil {
					t.Fatal(err)
				}
				if field == "value" {
					checks.Rows[len(checks.Rows)-1].ZoneService = &epathSQLZoneServiceProof{Service: "heating", Basis: "service_path_allocation"}
				}
			}
		}
	}
	if err := epathSQLSealHeatOnlyChecks(&checks, model); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLPrepareHeatOnlyChecks(checks, &model); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"binding", "row", "stamp", "typed-service", "external-declaration"} {
		c := checks
		m := epathSQLHeatOnlyModelFrom(epathSQLHeatOnlyClone(t, checks.HeatOnly.Inputs))
		c.Rows = append([]epathSQLModelCheck(nil), checks.Rows...)
		switch mutation {
		case "binding":
			c.HeatOnly = nil
		case "row":
			c.Rows = c.Rows[1:]
		case "stamp":
			c.Rows[0].HeatOnlyBound = false
		case "typed-service":
			for i := range c.Rows {
				if c.Rows[i].ZoneService != nil {
					c.Rows[i].ZoneService = nil
					break
				}
			}
		case "external-declaration":
			m.HeatOnlyFurnaces = nil
		}
		if err := epathSQLPrepareHeatOnlyChecks(c, &m); err == nil {
			t.Fatalf("retained HeatOnly deletion accepted: %s", mutation)
		}
	}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Periods: []EnergyPeriod{{ID: "M2"}}}}
	if err := epathSQLCheckHeatOnlyInactiveFanGraph(bundle, checks); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"building", "zone"} {
		bad := bundle
		bad.EnergyExplanation.Periods = append([]EnergyPeriod(nil), bundle.EnergyExplanation.Periods...)
		nodes := []EnergyExplanationNode{{ID: "fan", Level: "end_use", EndUse: "fans", Unit: "kWh", ScaleDomain: "site", Period: "M2"}, {ID: "carrier", Level: "carrier", Carrier: "electricity", Unit: "kWh", ScaleDomain: "site", Period: "M2"}}
		links := []EnergyPathLink{{ID: "fake", FromID: "fan", ToID: "carrier", Relation: "direct_end_use_to_carrier", FromUnit: "kWh", ToUnit: "kWh", Period: "M2", SourceIDs: []string{fmt.Sprintf("sql-rdd-%d", checks.HeatOnly.Native.HourlyID)}}}
		if scope == "zone" {
			for i := range nodes {
				nodes[i].ZoneName = "West Zone"
			}
			links[0].ZoneName = "West Zone"
			bad.EnergyExplanation.AvailableZones = []string{"West Zone"}
			bad.EnergyExplanation.ZoneResults = []EnergyExplanationZoneResult{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "West Zone"}, Periods: []EnergyPeriod{{ID: "M2", Nodes: nodes, Links: links}}}}
		} else {
			bad.EnergyExplanation.Periods[0].Nodes = nodes
			bad.EnergyExplanation.Periods[0].Links = links
		}
		if err := epathSQLCheckHeatOnlyInactiveFanGraph(bad, checks); err == nil {
			t.Fatal("zero-valued fabricated fan path accepted")
		}
	}
	before := checks.HeatOnly.Inputs
	model.FanPools[0].ServedZones = []string{"East Zone"}
	if !reflect.DeepEqual(before, checks.HeatOnly.Inputs) {
		t.Fatal("registry inputs aliased external model")
	}
}
