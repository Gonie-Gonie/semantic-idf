package simulation

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLSimpleVentilationZeroServiceUnit(t *testing.T) (epathRealOracleEvidence, epathRealSQLModel, epathSQLFrames) {
	t.Helper()
	path, model, plan := epathSQLSimpleVentilationUnitSQL(t)
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
	exec(`CREATE TABLE Zones(ZoneName TEXT,Multiplier REAL,ListMultiplier REAL); INSERT INTO Zones VALUES('ZONE 1',1,1),('ZONE 2',1,1),('ZONE 3',1,1);`)
	for s, service := range []string{"cooling", "heating"} {
		word := "Cooling"
		if service == "heating" {
			word = "Heating"
		}
		load := epathRealSQLLoad{Service: service, Component: "sensible", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Zone Air System Sensible " + word + " Energy", Unit: "J"}}, Keys: []string{"ZONE 1", "ZONE 2", "ZONE 3"}}}
		model.Loads = append(model.Loads, load)
		for z := 0; z < 3; z++ {
			id := 61 + s*3 + z
			exec(fmt.Sprintf(`INSERT INTO ReportDataDictionary VALUES(%d,'Zone Air System Sensible %s Energy','ZONE %d',0,'Monthly','J','System','Sum','HVAC System',NULL); INSERT INTO ReportData SELECT %d*100+TimeIndex,%d,TimeIndex,0 FROM Time WHERE TimeIndex BETWEEN 1 AND 12;`, id, word, z+1, id, id))
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	observed.originalText = epathSQLSimpleVentilationUnitOriginal()
	observed.executedText = observed.originalText
	plan.BasicEnergyDetail = "energy_path"
	for _, name := range []string{"Heating:Electricity", "Cooling:Electricity", "Heating:NaturalGas"} {
		observed.executedText += "\nOutput:Meter," + name + ",Monthly;"
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Meter", KeyValue: name, ReportingFrequency: "Monthly", State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Value: name}, {Value: "Monthly"}}})
	}
	observed.outputPlan = &plan
	frames, err := epathSQLSimpleVentilationUnitFrames(path, model)
	if err != nil {
		t.Fatal(err)
	}
	frames.Loads = map[string]epathSQLQuantity{}
	frames.LoadSourceIDs = map[string][]int{}
	frames.SourceRaw = map[int][]epathSQLQuantity{}
	frames.SourceEffective = map[int][]epathSQLQuantity{}
	for _, s := range observed.Sources {
		if s.DictionaryIndex < 61 || s.DictionaryIndex > 66 {
			continue
		}
		q, e := epathSQLMonthly(s, model.Precision)
		if e != nil {
			t.Fatal(e)
		}
		zone := strings.ToLower(s.KeyValue)
		service := "cooling"
		if s.Name == "Zone Air System Sensible Heating Energy" {
			service = "heating"
		}
		frames.SourceIdentities[s.DictionaryIndex] = s
		frames.SourceRaw[s.DictionaryIndex] = q
		frames.SourceZone[s.DictionaryIndex] = zone
		effective := make([]epathSQLQuantity, 12)
		for m, v := range q {
			key := epathSQLKey(zone, service, m+1)
			effective[m] = v.times(1)
			frames.Loads[key] = effective[m]
			frames.LoadSourceIDs[key] = []int{s.DictionaryIndex}
		}
		frames.SourceEffective[s.DictionaryIndex] = effective
	}
	return observed, model, frames
}

func TestEnergyPathSQLSimpleVentilationZeroServicePreservesDirectFansWithoutRatios(t *testing.T) {
	observed, model, frames := epathSQLSimpleVentilationZeroServiceUnit(t)
	// A thermal ventilation observation may be nonzero without becoming paid
	// Heating/Cooling. The gate must not confuse it with purchased conditioning.
	epathOracleEditSQL(t, observed.sqlPath, `INSERT INTO ReportDataDictionary VALUES(81,'Zone Ventilation Sensible Heat Loss Energy','ZONE 1',0,'Monthly','J','Zone','Sum','HVAC System',NULL); INSERT INTO ReportData SELECT 8100+TimeIndex,81,TimeIndex,10*3600000 FROM Time WHERE TimeIndex BETWEEN 1 AND 12;`)
	before, err := epathReadRealFileHash(observed.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	checks := epathSQLModelChecks{}
	if err = epathSQLModelServiceChecks(frames, model, &checks, observed); err != nil {
		t.Fatal(err)
	}
	if err = epathSQLModelZoneServiceChecks(frames, model, &checks, observed); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 0 {
		t.Fatal("no-conditioning service gate fabricated service or ratio observations")
	}
	if err = epathSQLModelDirectFanChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	annualZones := 0
	// Sum the literal native monthly Joules with the J-to-kWh conversion;
	// retain exact equality without assuming 2 kWh has an exact binary product.
	joulesToKWh := float64(1.0 / 3600000)
	for _, check := range checks.Rows {
		if check.Conversion != nil || check.ZoneService != nil || check.Item.Target.Service == "heating" || check.Item.Target.Service == "cooling" {
			t.Fatal("direct ventilation fan gained a conditioning service")
		}
		if check.Want.Scope == "zone" && check.Want.Period == "annual" && check.Item.Target.Field == "value" {
			annualZones++
			monthlyJoules, knownZone := map[string]float64{"zone 1": 0, "zone 2": 7200000, "zone 3": 10800000}[strings.ToLower(check.Want.Zone)]
			if !knownZone {
				t.Fatalf("foreign fan owner %q", check.Want.Zone)
			}
			want := 0.0
			for month := 0; month < 12; month++ {
				want += monthlyJoules * joulesToKWh
			}
			if check.DirectFan == nil || check.Quantity == nil || check.Quantity.Value != want || check.Item.Target.Basis != "direct_zone_energy" {
				t.Fatalf("literal native fan 0/24/36 annual observations lost direct ownership: zone=%q target=%+v quantity=%+v proof=%+v", check.Want.Zone, check.Item.Target, check.Quantity, check.DirectFan)
			}
		}
	}
	if len(checks.Rows) != 208 || annualZones != 3 {
		t.Fatalf("existing full native fan consumers lost: %d rows, %d annual Zones", len(checks.Rows), annualZones)
	}
	if after, e := epathReadRealFileHash(observed.sqlPath); e != nil || after != before {
		t.Fatal("zero-service compiler mutated native SQL")
	}
}

func TestEnergyPathSQLSimpleVentilationZeroServiceKeepsOrdinaryAndHeatOnlyGates(t *testing.T) {
	observed, model, frames := epathSQLSimpleVentilationZeroServiceUnit(t)
	if err := epathSQLModelServiceChecks(frames, model, &epathSQLModelChecks{}); err == nil || !strings.Contains(err.Error(), "both declared conversion services") {
		t.Fatalf("zero services without actual evidence escaped ordinary gate: %v", err)
	}
	if err := epathSQLModelZoneServiceChecks(frames, model, &epathSQLModelChecks{}); err == nil || !strings.Contains(err.Error(), "Zone service proof requires both reviewed services") {
		t.Fatalf("zero Zone services without actual evidence escaped ordinary gate: %v", err)
	}
	model.DirectHVACComponents = nil
	if err := epathSQLModelServiceChecks(frames, model, &epathSQLModelChecks{}, observed); err == nil {
		t.Fatal("ordinary empty service model borrowed ventilation exception")
	}
	if err := epathSQLModelZoneServiceChecks(frames, model, &epathSQLModelChecks{}, observed); err == nil {
		t.Fatal("ordinary empty Zone service model borrowed ventilation exception")
	}
	if err := epathSQLModelServiceChecks(frames, model, &epathSQLModelChecks{}, observed, observed); err == nil {
		t.Fatal("ambiguous evidence was accepted")
	}
	if err := epathSQLModelZoneServiceChecks(frames, model, &epathSQLModelChecks{}, observed, observed); err == nil {
		t.Fatal("ambiguous Zone evidence was accepted")
	}
	// The separately frozen HeatOnly wrapper keeps its existing one-Heating
	// route, regardless of the compiler now passing one optional evidence value.
	heat, heatModel, heatFrames := epathSQLHeatOnlyServiceUnit(t)
	checks := epathSQLModelChecks{}
	if err := epathSQLBindHeatOnlyChecks(heat, heatModel, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelFanPoolChecks(heat, heatFrames, heatModel.FanPools, heatModel.Precision, &checks, checks.HeatOnly); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelServiceChecks(heatFrames, heatModel, &checks, heat); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelZoneServiceChecks(heatFrames, heatModel, &checks, heat); err != nil {
		t.Fatal(err)
	}
}

func TestEnergyPathSQLSimpleVentilationZeroServiceRejectsMissingAndFalseProof(t *testing.T) {
	for _, mutation := range []string{"missing-original", "missing-executed", "missing-member", "changed-executed-physical", "missing-load-declaration", "missing-zero-load", "uncertain-zero", "foreign-load-source", "missing-fan-cohort", "missing-native-fan", "native-fan-value", "native-fan-orphan", "native-positive", "native-missing", "native-duplicate", "native-orphan", "native-foreign-owner", "native-wrong-step", "paid-meter-even-zero", "paid-coil-even-zero", "missing-request"} {
		t.Run(mutation, func(t *testing.T) {
			observed, model, frames := epathSQLSimpleVentilationZeroServiceUnit(t)
			switch mutation {
			case "missing-original":
				observed.originalText = ""
			case "missing-executed":
				observed.executedText = ""
			case "missing-member":
				observed.originalText = strings.Replace(observed.originalText, "ZoneVentilation:WindandStackOpenArea,ZONE 3 Ventl 2,ZONE 3,0.5,Constant,0.2,0,1,0.5;", "", 1)
			case "changed-executed-physical":
				observed.executedText = strings.Replace(observed.executedText, "Intake,400,0.9", "Intake,401,0.9", 1)
			case "missing-load-declaration":
				model.Loads = model.Loads[:1]
			case "missing-zero-load":
				delete(frames.Loads, epathSQLKey("ZONE 1", "heating", 2))
			case "uncertain-zero":
				frames.Loads[epathSQLKey("ZONE 1", "cooling", 2)] = epathSQLQuantity{Error: 1e-12}
			case "foreign-load-source":
				frames.LoadSourceIDs[epathSQLKey("ZONE 1", "cooling", 2)] = []int{62}
			case "missing-fan-cohort":
				frames.DirectHVACSourceIdentities = nil
			case "missing-native-fan":
				epathOracleEditSQL(t, observed.sqlPath, `DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=40`)
			case "native-fan-value":
				epathOracleEditSQL(t, observed.sqlPath, `UPDATE ReportData SET Value=1 WHERE ReportDataDictionaryIndex=40 AND TimeIndex=2`)
			case "native-fan-orphan":
				epathOracleEditSQL(t, observed.sqlPath, `INSERT INTO ReportData VALUES(99999,40,99999,0)`)
			case "native-positive":
				epathOracleEditSQL(t, observed.sqlPath, `UPDATE ReportData SET Value=0.000000001 WHERE ReportDataDictionaryIndex=61 AND TimeIndex=2`)
			case "native-missing":
				epathOracleEditSQL(t, observed.sqlPath, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=64 AND TimeIndex=2`)
			case "native-duplicate":
				epathOracleEditSQL(t, observed.sqlPath, `INSERT INTO ReportData VALUES(99999,61,2,0)`)
			case "native-orphan":
				epathOracleEditSQL(t, observed.sqlPath, `INSERT INTO ReportData VALUES(99999,61,99999,0)`)
			case "native-foreign-owner":
				epathOracleEditSQL(t, observed.sqlPath, `UPDATE ReportDataDictionary SET KeyValue='FOREIGN' WHERE ReportDataDictionaryIndex=61`)
			case "native-wrong-step":
				epathOracleEditSQL(t, observed.sqlPath, `UPDATE ReportDataDictionary SET TimestepType='Zone' WHERE ReportDataDictionaryIndex=61`)
			case "paid-meter-even-zero":
				epathOracleEditSQL(t, observed.sqlPath, `INSERT INTO ReportDataDictionary VALUES(91,'Heating:Electricity','',1,'Monthly','J','Facility','Sum','Zone',NULL); INSERT INTO ReportData SELECT 9100+TimeIndex,91,TimeIndex,0 FROM Time WHERE TimeIndex BETWEEN 1 AND 12;`)
			case "paid-coil-even-zero":
				epathOracleEditSQL(t, observed.sqlPath, `INSERT INTO ReportDataDictionary VALUES(92,'Cooling Coil Electricity Energy','HIDDEN',0,'Monthly','J','System','Sum','HVAC System',NULL); INSERT INTO ReportData SELECT 9200+TimeIndex,92,TimeIndex,0 FROM Time WHERE TimeIndex BETWEEN 1 AND 12;`)
			case "missing-request":
				observed.executedText = strings.Replace(observed.executedText, "Output:Meter,Cooling:Electricity,Monthly;", "", 1)
			}
			// Frames deliberately retain the valid native zeros; actual SQL anomalies
			// must still be found at this service-count boundary, without stale hashes.
			if err := epathSQLModelServiceChecks(frames, model, &epathSQLModelChecks{}, observed); err == nil {
				t.Fatalf("unproved zero-service exception accepted: %s", mutation)
			}
			if err := epathSQLModelZoneServiceChecks(frames, model, &epathSQLModelChecks{}, observed); err == nil {
				t.Fatalf("unproved zero-Zone-service exception accepted: %s", mutation)
			}
		})
	}
}
