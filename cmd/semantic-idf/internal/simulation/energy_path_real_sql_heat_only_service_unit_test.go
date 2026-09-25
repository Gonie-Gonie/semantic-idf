package simulation

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Literal native hand fixture: January Zone delivery 1/2/1 kWh, gas 200 kWh,
// electric Heating 0 and fan 100 kWh. None of these boundaries is equated.
func epathSQLHeatOnlyServiceUnit(t *testing.T) (epathRealOracleEvidence, epathRealSQLModel, epathSQLFrames) {
	t.Helper()
	observed, model, frames := epathSQLHeatOnlyBindingUnit(t)
	db, err := sql.Open("sqlite", observed.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		id        int
		name, key string
		meter     int
		january   float64
	}
	sources := []row{{401, "Heating:Electricity", "", 1, 0}, {402, "Heating:NaturalGas", "", 1, 200}}
	for i, zone := range []string{"WEST ZONE", "EAST ZONE", "NORTH ZONE"} {
		weight := 1.0
		if i == 1 {
			weight = 2
		}
		sources = append(sources, row{411 + i, "Zone Air System Sensible Heating Energy", zone, 0, weight}, row{421 + i, "Zone Air System Sensible Cooling Energy", zone, 0, 0})
	}
	for _, s := range sources {
		step, group := "HVAC System", "System"
		if s.meter == 1 {
			step = "Zone"
			group = map[string]string{"Heating:Electricity": "Facility:Electricity:Heating", "Heating:NaturalGas": "Facility:NaturalGas:Heating"}[s.name]
		}
		if _, err = tx.Exec(`INSERT INTO ReportDataDictionary(ReportDataDictionaryIndex,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup,Type,TimestepType,ScheduleName) VALUES(?,?,?,?,'Monthly','J',?,'Sum',?,NULL)`, s.id, s.name, s.key, s.meter, group, step); err != nil {
			t.Fatal(err)
		}
		for month := 1; month <= 12; month++ {
			value := 0.0
			if month == 1 {
				value = s.january * 3600000
			}
			if _, err = tx.Exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, 900000+s.id*100+month, s.id, 20000+month, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	refreshed, err := epathReadRealSQLOracle(observed.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	observed.Sources, observed.Weather = refreshed.Sources, refreshed.Weather
	// Keep the absent-Cooling request last for its existing deletion regression.
	for _, name := range []string{"Heating:Electricity", "Heating:NaturalGas", "Cooling:Electricity"} {
		observed.executedText = "Output:Meter," + name + ",Monthly;\n" + observed.executedText
		observed.outputPlan.OutputObjects = append(observed.outputPlan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Meter", KeyValue: name, ReportingFrequency: "Monthly", State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Value: name}, {Value: "Monthly"}}})
	}
	for _, service := range []string{"cooling", "heating"} {
		word := "Cooling"
		if service == "heating" {
			word = "Heating"
		}
		model.Loads = append(model.Loads, epathRealSQLLoad{Service: service, Component: "sensible", Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Zone Air System Sensible " + word + " Energy", Unit: "J"}}, Keys: append([]string(nil), model.HeatOnlyFurnaces[0].ServedZones...)}})
	}
	frames.SourceIdentities = map[int]epathRealSQLSource{}
	frames.SourceRaw = map[int][]epathSQLQuantity{}
	frames.SourceEffective = map[int][]epathSQLQuantity{}
	frames.SourceZone = map[int]string{}
	frames.SiteSources = map[string][]int{}
	frames.LoadSourceIDs = map[string][]int{}
	for _, s := range observed.Sources {
		if s.ReportingFrequency != "Monthly" {
			continue
		}
		q, e := epathSQLMonthly(s, model.Precision)
		if e != nil {
			t.Fatal(e)
		}
		frames.SourceIdentities[s.DictionaryIndex] = s
		frames.SourceRaw[s.DictionaryIndex] = q
		if s.IsMeter {
			site := map[string]string{"Fans:Electricity": "fans.electricity", "Heating:Electricity": "heating.electricity", "Heating:NaturalGas": "heating.natural_gas"}[s.Name]
			if site == "" {
				t.Fatalf("unexpected hand meter %s", s.Name)
			}
			frames.SiteSources[site] = []int{s.DictionaryIndex}
			frames.Site[site] = nil
			frames.SourceEffective[s.DictionaryIndex] = q
			for _, v := range q {
				value := v
				frames.Site[site] = append(frames.Site[site], &value)
			}
			continue
		}
		zone := strings.ToLower(s.KeyValue)
		service := "cooling"
		if s.Name == "Zone Air System Sensible Heating Energy" {
			service = "heating"
		}
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

func TestEnergyPathSQLHeatOnlySingleServiceCompilesBothScopes(t *testing.T) {
	observed, model, frames := epathSQLHeatOnlyServiceUnit(t)
	checks := epathSQLModelChecks{}
	if err := epathSQLBindHeatOnlyChecks(observed, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelFanPoolChecks(observed, frames, model.FanPools, model.Precision, &checks, checks.HeatOnly); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelZoneServiceChecks(frames, model, &checks); err != nil {
		t.Fatal(err)
	}
	compiled, err := epathSQLCompileHeatOnlyService(frames, model, model.Services[0], checks.HeatOnly)
	if err != nil {
		t.Fatal(err)
	}
	for m, month := range compiled.Monthly {
		for zone, carriers := range month.Zones {
			for carrier, share := range carriers {
				want := 0.0
				if m == 0 && carrier == "natural_gas" {
					want = 50
					if zone == "east zone" {
						want = 100
					}
				}
				if share.ObservedDirect || share.Direct.Value != 0 || len(share.DirectSources) != 0 || share.Allocated.Value != want {
					t.Fatalf("shared native service became a direct observation or wrong 50/100/50 share: M%d %s %s %+v", m+1, zone, carrier, share)
				}
			}
		}
	}
	ratios, zoneValues, gasLedger := 0, 0, 0
	// Keep exact machine comparisons, derived from the literal native Joules
	// and unit conversion, not decimal constants rounded differently by Go.
	joulesToKWh := float64(1.0 / 3600000)
	wantDelivered := float64(3600000)*joulesToKWh + float64(7200000)*joulesToKWh + float64(3600000)*joulesToKWh
	wantGas := float64(720000000) * joulesToKWh
	for _, check := range checks.Rows {
		if check.Item.Target.Service == "cooling" || check.Item.Target.Category == "cooling" || check.ZoneService != nil && check.ZoneService.Service == "cooling" {
			t.Fatal("fake Cooling service was compiled")
		}
		if check.Want.Scope == "building" && check.Want.Period == "M1" && check.Item.Target.Field == "pairedRatio" && check.Item.Target.Basis == "service_path_allocation" {
			ratios++
			if check.Quantity == nil || check.Quantity.Value != wantDelivered/wantGas || check.Conversion == nil || check.Item.Target.RatioKind != "efficiency" || check.Conversion.From.Value != wantDelivered || check.Conversion.To.Value != wantGas {
				t.Fatalf("independent delivered-load/fuel efficiency policy or literal boundary changed: target=%+v quantity=%+v conversion=%+v", check.Item.Target, check.Quantity, check.Conversion)
			}
		}
		if check.Want.Scope == "zone" && check.Want.Period == "M1" && check.ZoneService != nil {
			zoneValues++
			want := 50.0
			if strings.EqualFold(check.Want.Zone, "East Zone") {
				want = 100
			}
			if check.Quantity == nil || check.Quantity.Value != want || check.ZoneService.Carriers["natural_gas"].Value != want || check.ZoneService.Carriers["electricity"].Value != 0 {
				t.Fatal("Zone carrier proof lost the literal native shares")
			}
			for _, branch := range check.ZoneService.Branches {
				if branch.Basis != "service_path_allocation" {
					t.Fatal("shared-only branch claimed direct ownership")
				}
				for _, source := range branch.Originals {
					if source.DirectHVAC != nil {
						t.Fatal("invented measured direct source")
					}
				}
			}
		}
		if check.Want.Scope == "building" && check.Want.Period == "M1" && check.Item.Target.ID == "reconcile.zone_hvac_allocation.heating.natural_gas.m1" && check.Item.Target.Field == "allocatedValue" {
			gasLedger++
			if check.Quantity == nil || check.Quantity.Value != 200 {
				t.Fatal("Building carrier allocation lost native gas closure")
			}
		}
	}
	if ratios != 1 || zoneValues != 3 || gasLedger != 1 {
		t.Fatalf("missing complete service consumers: ratio %d, Zones %d, gas ledger %d", ratios, zoneValues, gasLedger)
	}
}

func TestEnergyPathSQLHeatOnlySingleServiceDoesNotRelaxRegularModel(t *testing.T) {
	_, model, frames := epathSQLHeatOnlyServiceUnit(t)
	// Reach the existing final service-count gate through the ordinary single-
	// ledger route, with no HeatOnly binding and no claimed DirectHVAC source.
	model.HeatOnlyFurnaces = nil
	model.Services[0].CarrierReconciliationIDs = nil
	model.Services[0].ReconciliationID = "reconcile.zone_hvac_allocation.heating.annual"
	if direct, err := epathSQLCompileDirectHVACService(frames, model, model.Services[0]); err != nil || direct != nil {
		t.Fatalf("default constructor opt-in changed: %v %v", direct, err)
	}
	if err := epathSQLModelServiceChecks(frames, model, &epathSQLModelChecks{}); err == nil || !strings.Contains(err.Error(), "both declared conversion services are required") {
		t.Fatalf("ordinary missing Cooling did not fail at the service gate: %v", err)
	}
	if err := epathSQLModelZoneServiceChecks(frames, model, &epathSQLModelChecks{}); err == nil || !strings.Contains(err.Error(), "Zone service proof requires both reviewed services") {
		t.Fatalf("ordinary missing Cooling did not fail at the Zone service gate: %v", err)
	}
}

func TestEnergyPathSQLHeatOnlySingleServiceRequiresNativeAbsenceAndKnownZero(t *testing.T) {
	baseline, baselineModel, baselineFrames := epathSQLHeatOnlyServiceUnit(t)
	data, err := os.ReadFile(baseline.sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"missing-binding", "missing-cooling-declaration", "missing-zone-month", "uncertain-zero", "foreign-source", "native-nonzero", "native-missing-month", "native-orphan-time", "native-duplicate-month", "paid-cooling-even-zero", "missing-cooling-request"} {
		t.Run(mutation, func(t *testing.T) {
			observed := epathSQLHeatOnlyUnitSQLCopy(t, baseline, data)
			model := epathSQLHeatOnlyClone(t, baselineModel)
			frames := epathSQLHeatOnlyClone(t, baselineFrames)
			switch mutation {
			case "missing-cooling-declaration":
				model.Loads = model.Loads[1:]
			case "missing-zone-month":
				delete(frames.Loads, epathSQLKey("West Zone", "cooling", 2))
			case "uncertain-zero":
				frames.Loads[epathSQLKey("West Zone", "cooling", 2)] = epathSQLQuantity{Error: 1e-12}
			case "foreign-source":
				frames.LoadSourceIDs[epathSQLKey("West Zone", "cooling", 2)] = []int{422}
			case "native-nonzero":
				epathOracleEditSQL(t, observed.sqlPath, `UPDATE ReportData SET Value=0.000000001 WHERE ReportDataDictionaryIndex=421 AND TimeIndex=20002`)
			case "native-missing-month":
				epathOracleEditSQL(t, observed.sqlPath, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=421 AND TimeIndex=20002`)
			case "native-orphan-time":
				epathOracleEditSQL(t, observed.sqlPath, `INSERT INTO ReportData VALUES(9999999,421,9999999,0)`)
			case "native-duplicate-month":
				epathOracleEditSQL(t, observed.sqlPath, `INSERT INTO ReportData VALUES(9999999,421,20002,0)`)
			case "paid-cooling-even-zero":
				epathOracleEditSQL(t, observed.sqlPath, `INSERT INTO ReportDataDictionary(ReportDataDictionaryIndex,Name,KeyValue,IsMeter,ReportingFrequency,Units,IndexGroup) VALUES(499,'Cooling:Electricity','',1,'Monthly','J','Facility')`)
			case "missing-cooling-request":
				observed.executedText = strings.Replace(observed.executedText, "Output:Meter,Cooling:Electricity,Monthly;\n", "", 1)
				observed.outputPlan.OutputObjects = observed.outputPlan.OutputObjects[:len(observed.outputPlan.OutputObjects)-1]
			}
			// Rebind after the native mutation: a stale file hash is not this test's
			// rejection reason. Frames deliberately remain the old zero observations.
			binding, err := epathSQLCompileHeatOnlyBinding(observed, model)
			if err != nil {
				t.Fatal(err)
			}
			if mutation == "missing-binding" {
				binding = nil
			}
			if err = epathSQLHeatOnlySingleService(frames, model, binding); err == nil {
				t.Fatalf("unproved Heating-only service accepted: %s", mutation)
			}
		})
	}
}
