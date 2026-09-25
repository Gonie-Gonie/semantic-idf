package simulation

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLCentralSharedUnit(t *testing.T, blank, zero bool) (string, string, string, *PurposeRunPlan, epathRealSQLModel, []epathRealSQLSource, epathSQLFrames) {
	t.Helper()
	original := epathSQLCentralOriginalFixture(t)
	zones := []string{"SPACE1-1", "SPACE2-1", "SPACE3-1", "SPACE4-1", "SPACE5-1"}
	model := epathRealSQLModel{Precision: epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1}}
	frames := epathSQLFrames{Zones: map[string]epathSQLZone{}, Loads: map[string]epathSQLQuantity{}, LoadSourceIDs: map[string][]int{}, Site: map[string][]*epathSQLQuantity{}, SiteSources: map[string][]int{}, SourceRaw: map[int][]epathSQLQuantity{}, SourceIdentities: map[int]epathRealSQLSource{}}
	for _, zone := range append(append([]string(nil), zones...), "PLENUM-1") {
		frames.Zones[strings.ToLower(zone)] = epathSQLZone{Name: zone, Multiplier: 1}
	}
	plan := &PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
	outputs, decoys := "", ""
	var observed, all []epathRealSQLSource
	for n, service := range []string{"cooling", "heating"} {
		name, loop, meter := epathSQLCentralCoolingElectricity, "Chilled Water Loop", "Cooling:Electricity"
		values := [12]float64{15, .0014}
		if n == 1 {
			name, loop, meter, values = epathSQLCentralHeatingElectricity, "Hot Water Loop", "Heating:Electricity", [12]float64{30, .0026}
		}
		if zero {
			values = [12]float64{}
		}
		siteID := service + ".electricity"
		// The separate native Cooling electricity denominator is COP; Heating
		// retains the load/site boundary, not an asserted equipment efficiency.
		ratioKind := "load_to_site_energy"
		if service == "cooling" {
			ratioKind = "coefficient_of_performance"
		}
		member := epathRealSQLHVACSharedMember{ID: "central_heat_pump." + service + ".electricity:chillerbank", ObjectType: "CentralHeatPumpSystem", ObjectName: "ChillerBank", PlantLoopName: loop, ServedZones: append([]string(nil), zones...), Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}, Keys: []string{"ChillerBank"}}}
		model.HVACConsumptionPools = append(model.HVACConsumptionPools, epathRealSQLHVACConsumptionPool{SiteID: siteID, Shared: []epathRealSQLHVACSharedMember{member}})
		model.Site = append(model.Site, epathRealSQLSite{ID: siteID, EndUse: service, Carrier: "electricity", Source: epathRealSQLSelector{IsMeter: true, Keys: []string{""}, Alternatives: []epathRealSQLAlternative{{Name: meter, Unit: "J"}}}})
		model.Services = append(model.Services, epathRealSQLService{Service: service, SiteIDs: []string{siteID}, ServedZones: append([]string(nil), zones...), Basis: "service_path_allocation", FallbackBasis: "zone_load_allocation", RatioKind: ratioKind, FallbackRatioKind: ratioKind, CarrierReconciliationIDs: map[string]string{"electricity": "reconcile.zone_hvac_allocation." + service + ".electricity.annual"}})
		paid := epathSQLHVACConsumptionUnitSource(10+n, name, "CHILLERBANK", false, values)
		paid.IndexGroup = "System"
		observed, all = append(observed, paid), append(all, paid)
		broad := epathSQLHVACConsumptionUnitSource(1+n, meter, "", true, values)
		broad.IndexGroup = "Facility:Electricity:" + strings.ToUpper(service[:1]) + service[1:]
		all = append(all, broad)
		frames.SourceIdentities[broad.DictionaryIndex] = broad
		frames.SiteSources[siteID] = []int{broad.DictionaryIndex}
		monthly, err := epathSQLMonthly(broad, model.Precision)
		if err != nil {
			t.Fatal(err)
		}
		frames.SourceRaw[broad.DictionaryIndex] = monthly
		for _, q := range monthly {
			copy := q
			frames.Site[siteID] = append(frames.Site[siteID], &copy)
		}
		for z, zone := range append(append([]string(nil), zones...), "PLENUM-1") {
			value := float64(z + 1)
			if n == 1 {
				value = float64(5 - z)
			}
			if zone == "PLENUM-1" {
				value = 500
			}
			loads := [12]float64{}
			for m := range loads {
				loads[m] = value
			}
			load := epathSQLHVACConsumptionUnitSource(100+n*10+z, "Zone Air System Sensible "+strings.ToUpper(service[:1])+service[1:]+" Energy", zone, false, loads)
			all = append(all, load)
			frames.SourceIdentities[load.DictionaryIndex] = load
			quantities, err := epathSQLMonthly(load, model.Precision)
			if err != nil {
				t.Fatal(err)
			}
			for m, q := range quantities {
				key := epathSQLKey(zone, service, m+1)
				frames.Loads[key] = q.times(1)
				frames.LoadSourceIDs[key] = []int{load.DictionaryIndex}
			}
		}
		key := "ChillerBank"
		if blank {
			key = ""
		}
		outputs += fmt.Sprintf("\nOutput:Variable,%s,%s,Monthly;\n", key, name)
		decoys += fmt.Sprintf("\nOutput:Variable,ChillerBank,%s,Monthly,LIMITED;\n", name)
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", KeyValue: key, VariableName: name, ReportingFrequency: "Monthly", State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Name: "Key Value", Value: key}, {Name: "Variable Name", Value: name}, {Name: "Reporting Frequency", Value: "Monthly"}}})
	}
	if blank {
		original += outputs
		doc, err := idf.Parse(original)
		if err != nil {
			t.Fatal(err)
		}
		for n := range plan.OutputObjects {
			for _, object := range doc.Objects {
				if object.Type == "Output:Variable" && epathSQLSharedField(object, 0) == "" && epathSQLSharedField(object, 1) == plan.OutputObjects[n].VariableName && epathSQLSharedField(object, 2) == "Monthly" {
					index := object.Index
					plan.OutputObjects[n].ObjectIndex = &index
					plan.OutputObjects[n].State = "existing"
				}
			}
		}
		outputs = ""
	}
	executed := original + outputs + decoys
	for _, name := range []string{epathSQLCentralCoolingElectricity, epathSQLCentralHeatingElectricity} {
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", KeyValue: "ChillerBank", VariableName: name, ReportingFrequency: "Monthly", State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Name: "Key Value", Value: "ChillerBank"}, {Name: "Variable Name", Value: name}, {Name: "Reporting Frequency", Value: "Monthly"}, {Name: "Schedule Name", Value: "LIMITED"}}})
	}
	path := filepath.Join(t.TempDir(), "central-hand.sql")
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
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER,EnvironmentName TEXT,EnvironmentType INTEGER)`)
	exec(`INSERT INTO EnvironmentPeriods VALUES(3,'Annual hand values',3)`)
	exec(`CREATE TABLE Time(TimeIndex INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,Year INTEGER,"Interval" REAL,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER,SimulationDays INTEGER)`)
	exec(`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER,KeyValue TEXT,Name TEXT,Units TEXT,IsMeter INTEGER,ReportingFrequency TEXT,IndexGroup TEXT,Type TEXT,TimestepType TEXT,ScheduleName TEXT)`)
	exec(`CREATE TABLE ReportData(ReportDataIndex INTEGER,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`)
	for month := 1; month <= 12; month++ {
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
		exec(`INSERT INTO Time VALUES(?,?,?,24,0,2017,?,3,3,NULL,?)`, month, month, last.Day(), last.Day()*1440, last.YearDay())
	}
	for _, source := range all {
		step, meter := "Zone", 0
		if source.IndexGroup == "System" {
			step = "HVAC System"
		}
		if source.IsMeter {
			meter = 1
		}
		exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,?,'Monthly',?,'Sum',?,NULL)`, source.DictionaryIndex, source.KeyValue, source.Name, "J", meter, source.IndexGroup, step)
		for _, month := range source.Months {
			exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, source.DictionaryIndex*100+month.Month, month.Month, source.DictionaryIndex, *month.RawSum)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path, original, executed, plan, model, observed, frames
}

func TestEnergyPathSQLCentralSharedNativeMonthlyOnlyAndLiteralOpener(t *testing.T) {
	for _, blank := range []bool{false, true} {
		for _, zero := range []bool{false, true} {
			t.Run(fmt.Sprintf("blank=%t/zero=%t", blank, zero), func(t *testing.T) {
				path, original, executed, plan, model, observed, frames := epathSQLCentralSharedUnit(t, blank, zero)
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := epathSQLValidateHVACConsumptionOriginalModel(original, model.HVACConsumptionPools, executed); err != nil {
					t.Fatal(err)
				}
				if err := epathCompileSQLHVACConsumptionPoolFrames(path, observed, model, &frames, plan, original, executed); err != nil {
					t.Fatal(err)
				}
				if len(frames.HVACConsumptionPools) != 2 || len(frames.HVACSharedSourceIdentities) != 2 {
					t.Fatal("Central gained fake E/R companions or lost a paid role")
				}
				var checks epathSQLModelChecks
				if err := epathSQLModelHVACSharedSourceChecks(frames, &checks); err != nil {
					t.Fatal(err)
				}
				if len(checks.Rows) != 4 {
					t.Fatal("two native Monthly sources require four scalar checks")
				}
				for _, check := range checks.Rows {
					proof := check.HVACSharedSource
					want := 15.0014
					if proof.Source.DictionaryIndex == 11 {
						want = 30.0026
					}
					if zero {
						want = 0
					}
					if *proof.Source.EnergyKWh != want || !proof.Canonical || len(proof.Hourly) != 0 || (proof.ObjectIndex != nil) != blank {
						t.Fatalf("wrong native role/quantity/opener: %#v", proof)
					}
					bundle := epathSQLHVACSharedUnitBundle(*proof)
					if bundle.EnergyExplanation.Sources[0].RawValue != want || bundle.EnergyExplanation.Sources[0].EffectiveValue != want {
						t.Fatal("fresh native source scalar was rounded before transport")
					}
					// Saved V2 sources round the native annual sum once, not
					// each native month or each allocation share.
					transport := 15.001
					if proof.Source.DictionaryIndex == 11 {
						transport = 30.003
					}
					if zero {
						transport = 0
					}
					lo, hi := check.Quantity.bounds()
					if check.Quantity.Value != transport || lo != transport || hi != transport {
						t.Fatal("source transport widened or lost its single annual quantization")
					}
					bundle.EnergyExplanation.Schema = energyExplanationSchema
					bundle.EnergyExplanation.Scope = EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}
					if proof.ObjectIndex != nil {
						copy := *proof.ObjectIndex
						bundle.EnergyExplanation.Sources[0].ObjectIndex = &copy
					}
					nativeWire, err := json.Marshal(bundle)
					if err != nil {
						t.Fatal(err)
					}
					// Inspect literal original JSON without invoking the V2 reader.
					var literal struct {
						EnergyExplanation struct {
							Sources []struct {
								RawValue       float64 `json:"rawValue"`
								EffectiveValue float64 `json:"effectiveValue"`
							} `json:"sources"`
						} `json:"energyExplanation"`
					}
					if err := json.Unmarshal(nativeWire, &literal); err != nil || len(literal.EnergyExplanation.Sources) != 1 || literal.EnergyExplanation.Sources[0].RawValue != want || literal.EnergyExplanation.Sources[0].EffectiveValue != want {
						t.Fatal("original source JSON lost exact native annual scalars")
					}
					if err := json.Unmarshal(nativeWire, &bundle); err != nil {
						t.Fatal(err)
					}
					if bundle.EnergyExplanation.Sources[0].RawValue != transport || bundle.EnergyExplanation.Sources[0].EffectiveValue != transport {
						t.Fatal("saved-result reader did not apply exactly one annual 3dp transition")
					}
					for pass := 0; pass < 3; pass++ {
						if err := epathCheckSQLHVACSharedSource(bundle, check); err != nil {
							t.Fatal(err)
						}
						wire, err := json.Marshal(bundle)
						if err != nil {
							t.Fatal(err)
						}
						if err := json.Unmarshal(wire, &bundle); err != nil {
							t.Fatal(err)
						}
					}
					bundle.EnergyExplanation.Sources[0].EffectiveMultiplier = 3
					if err := epathCheckSQLHVACSharedSource(bundle, check); err == nil {
						t.Fatal("module-count multiplication accepted")
					}
					bundle.EnergyExplanation.Sources[0].EffectiveMultiplier = 1
					bundle.EnergyExplanation.Sources[0].inspectorValuePresence = 0
					if err := epathCheckSQLHVACSharedSource(bundle, check); err == nil {
						t.Fatal("absent scalar was substituted for observed zero")
					}
				}
				// Retained native quantities/owners do not alias caller declarations.
				*observed[0].Months[0].EnergyKWh = 999
				model.HVACConsumptionPools[0].Shared[0].ServedZones[0] = "PLENUM-1"
				if *frames.HVACSharedSourceIdentities[10].Source.Months[0].EnergyKWh == 999 || frames.HVACSharedSourceIdentities[10].Member.ServedZones[0] == "PLENUM-1" {
					t.Fatal("Central source proof aliases caller input")
				}
				if blank {
					*plan.OutputObjects[0].ObjectIndex = -1
					if *frames.HVACSharedSourceIdentities[10].ObjectIndex == -1 {
						t.Fatal("retained original opener aliases mutable plan")
					}
				}
				after, err := os.ReadFile(path)
				if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
					t.Fatal("native oracle modified SQL")
				}
			})
		}
	}
}

func TestEnergyPathSQLCentralSharedOnlyAllocationUsesDistinctServiceLoads(t *testing.T) {
	path, original, executed, plan, model, observed, frames := epathSQLCentralSharedUnit(t, true, false)
	if err := epathCompileSQLHVACConsumptionPoolFrames(path, observed, model, &frames, plan, original, executed); err != nil {
		t.Fatal(err)
	}
	if len(model.DirectHVACComponents) != 0 || len(frames.DirectHVAC) != 0 {
		t.Fatal("hand model must have no fake direct component")
	}
	var serviceChecks epathSQLModelChecks
	if err := epathSQLModelServiceChecks(frames, model, &serviceChecks); err != nil {
		t.Fatalf("full two-service carrier dispatcher rejected native shared-only pools: %v", err)
	}
	for n, service := range model.Services {
		pool, err := epathSQLCompileHVACConsumptionService(frames, model, service)
		if err != nil || pool == nil {
			t.Fatalf("shared-only service failed: %v", err)
		}
		for z, zone := range service.ServedZones {
			want := float64(z + 1)
			if n == 1 {
				want = float64(10 - 2*z)
			}
			part := pool.Direct.Monthly[0].Zones[strings.ToLower(zone)]["electricity"]
			if part.Direct.Value != 0 || part.ObservedDirect || part.Allocated.Value != want {
				t.Fatalf("%s/%s did not consume its own C/H pressure: %#v", service.Service, zone, part)
			}
		}
		if pool.Direct.Monthly[0].Zones["plenum-1"]["electricity"].Allocated.Value != 0 {
			t.Fatal("return-only plenum became a paid recipient")
		}
		if err := epathSQLValidateHVACConsumptionService(pool); err != nil {
			t.Fatal(err)
		}
		wantTiny := int64(1)
		if n == 1 {
			wantTiny = 3
		}
		if pool.Sources[1][10+n].BudgetMilliKWh != wantTiny {
			t.Fatal("native source budget did not round once before discrete shares")
		}
		var checks epathSQLModelChecks
		if err := epathSQLHVACConsumptionZoneServiceChecks(pool, []string{"space1-1", "space2-1", "space3-1", "space4-1", "space5-1", "plenum-1"}, &checks); err != nil {
			t.Fatal(err)
		}
		if err := epathSQLHVACConsumptionBuildingLedgerChecks(service, pool, "annual", &checks); err != nil {
			t.Fatal(err)
		}
		want := 15.001
		if n == 1 {
			want = 30.003
		}
		bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"}, Reconciliation: []EnergyReconciliation{{ID: service.CarrierReconciliationIDs["electricity"], Level: "allocation", Period: "annual", ServiceKind: service.Service, Basis: "service_path_allocation", Unit: "kWh", ExpectedValue: want, AllocatedValue: want, ExplainedValue: want}}}}
		for _, check := range checks.Rows {
			if check.Allocation != nil {
				if err := epathCheckSQLHVACConsumptionAllocation(bundle, check); err != nil {
					t.Fatal(err)
				}
			}
		}
		// The existing Zone source consumer must retain allocated-only detail,
		// never turn a native system input into a measured Zone quantity.
		proof := frames.HVACSharedSourceIdentities[10+n]
		zoned := epathSQLHVACSharedUnitBundle(proof)
		zoned.EnergyExplanation.Schema = energyExplanationSchema
		zoned.EnergyExplanation.Scope = EnergyExplanationScope{Kind: "building", AggregationBasis: "model_total"}
		amount := 1.0
		if n == 1 {
			amount = 10.001
		}
		zoned.EnergyExplanation.Sources[0].ScopeDetails = []EnergyDataSourceScopeDetail{{Scope: EnergyExplanationScope{Kind: "zone", ZoneName: "SPACE1-1", AggregationBasis: "model_total"}, AllocatedValue: amount, AllocationApplied: true, AggregationBasis: "model_total", inspectorScopedValuePresence: true}}
		for pass := 0; pass < 2; pass++ {
			if err := epathSQLHVACConsumptionScopedSources(zoned, pool, "space1-1"); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(zoned)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &zoned); err != nil {
				t.Fatal(err)
			}
		}
		zoned.EnergyExplanation.Sources[0].ScopeDetails[0].RawValue = amount
		if err := epathSQLHVACConsumptionScopedSources(zoned, pool, "space1-1"); err == nil {
			t.Fatal("shared native input became measured Zone raw")
		}
	}
	if direct, err := epathSQLCompileDirectHVACService(frames, model, model.Services[0]); err != nil || direct != nil {
		t.Fatal("default no-direct behavior was broadened")
	}
	delete(frames.Loads, epathSQLKey("SPACE2-1", "cooling", 3))
	if _, err := epathSQLCompileHVACConsumptionService(frames, model, model.Services[0]); err == nil {
		t.Fatal("missing canonical load became zero shared pressure")
	}
	f, m := epathSQLHVACConsumptionUnitFrames(t, [12]float64{}, [12]float64{}, [12]float64{1})
	m.DirectHVACComponents, f.DirectHVAC, f.DirectHVACSourceIdentities = nil, nil, nil
	if _, err := epathSQLCompileHVACConsumptionService(f, m, m.Services[0]); err == nil {
		t.Fatal("Boiler missing-direct contract was relaxed")
	}
}

func TestEnergyPathSQLCentralSharedRejectsMissingNativeOrWrongRole(t *testing.T) {
	for index, query := range []string{
		`DELETE FROM ReportData WHERE ReportDataDictionaryIndex=11 AND TimeIndex=2`,
		`UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=11 AND TimeIndex=2`,
		`UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=11 AND TimeIndex=2`,
		`INSERT INTO ReportData SELECT 9999,TimeIndex,ReportDataDictionaryIndex,Value FROM ReportData WHERE ReportDataDictionaryIndex=11 AND TimeIndex=2`,
		`INSERT INTO ReportData VALUES(9999,9999,11,0)`,
		`UPDATE ReportDataDictionary SET Type='Avg' WHERE ReportDataDictionaryIndex=11`,
		`UPDATE ReportDataDictionary SET TimestepType='Zone' WHERE ReportDataDictionaryIndex=11`,
		`UPDATE ReportDataDictionary SET IndexGroup='Zone' WHERE ReportDataDictionaryIndex=11`,
		`UPDATE ReportDataDictionary SET ScheduleName='LIMITED' WHERE ReportDataDictionaryIndex=11`,
		`UPDATE ReportDataDictionary SET ReportingFrequency='Hourly' WHERE ReportDataDictionaryIndex=11`,
		`UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=11`,
		`UPDATE Time SET Hour=23 WHERE TimeIndex=2`,
		`INSERT INTO ReportDataDictionary SELECT 99,KeyValue,Name,Units,IsMeter,ReportingFrequency,IndexGroup,Type,TimestepType,ScheduleName FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=11`,
	} {
		t.Run(fmt.Sprintf("native_%02d", index+1), func(t *testing.T) {
			path, original, executed, plan, model, observed, frames := epathSQLCentralSharedUnit(t, false, true)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(query); err != nil {
				db.Close()
				t.Fatal(err)
			}
			db.Close()
			if err := epathCompileSQLHVACConsumptionPoolFrames(path, observed, model, &frames, plan, original, executed); err == nil || len(frames.HVACConsumptionPools) != 0 || len(frames.HVACSharedSourceIdentities) != 0 {
				t.Fatal("unknown/contradictory paid native source was accepted or partially retained")
			}
		})
	}
}

func TestEnergyPathSQLCentralSharedRejectsDeletedDeclarationAndBorrowedOpener(t *testing.T) {
	path, original, executed, plan, model, observed, frames := epathSQLCentralSharedUnit(t, true, false)
	if err := epathSQLValidateHVACConsumptionOriginalModel(original, nil, executed); err == nil {
		t.Fatal("deleted-all Central source declarations accepted")
	}
	for _, mutate := range []func(*epathRealSQLHVACSharedMember){
		func(m *epathRealSQLHVACSharedMember) { m.PlantLoopName = "Chilled Water Condenser Loop" },
		func(m *epathRealSQLHVACSharedMember) { m.PlantLoopName = "Hot Water Loop" },
		func(m *epathRealSQLHVACSharedMember) {
			m.Source.Alternatives[0].Name = "Chiller Heater System Cooling Energy"
		},
		func(m *epathRealSQLHVACSharedMember) { m.Source.AllowAbsent = true },
		func(m *epathRealSQLHVACSharedMember) { m.ServedZones[4] = "PLENUM-1" },
	} {
		data, _ := json.Marshal(model.HVACConsumptionPools)
		var changed []epathRealSQLHVACConsumptionPool
		if err := json.Unmarshal(data, &changed); err != nil {
			t.Fatal(err)
		}
		mutate(&changed[0].Shared[0])
		if err := epathSQLValidateHVACConsumptionOriginalModel(original, changed, executed); err == nil {
			t.Fatal("wrong original paid circuit/recipient was accepted")
		}
	}
	for _, mutate := range []func(*PurposeRunPlan){
		func(p *PurposeRunPlan) { p.OutputObjects = p.OutputObjects[1:] },
		func(p *PurposeRunPlan) {
			p.OutputObjects[0].Fields = append(p.OutputObjects[0].Fields, idf.OutputFieldValue{Name: "Schedule Name", Value: "LIMITED"})
		},
		func(p *PurposeRunPlan) { *p.OutputObjects[0].ObjectIndex = *p.OutputObjects[1].ObjectIndex },
		func(p *PurposeRunPlan) { p.OutputObjects[0].PurposeIDs = nil },
		func(p *PurposeRunPlan) { p.OutputObjects = append(p.OutputObjects, p.OutputObjects[0]) },
	} {
		data, _ := json.Marshal(plan)
		var changed PurposeRunPlan
		if err := json.Unmarshal(data, &changed); err != nil {
			t.Fatal(err)
		}
		mutate(&changed)
		fresh := frames
		if err := epathCompileSQLHVACConsumptionPoolFrames(path, observed, model, &fresh, &changed, original, executed); err == nil {
			t.Fatal("unbound/filtered/foreign actual request accepted")
		}
	}
	boiler := epathSQLTestHVACSharedIdentity([12]float64{1})
	boiler.Companions = nil
	if err := epathSQLValidateHVACSharedSourceIdentity(boiler); err == nil {
		t.Fatal("Boiler companion contract was relaxed")
	}
	if !reflect.DeepEqual(frames.SourceIdentities[1].Name, "Cooling:Electricity") {
		t.Fatal("request failures changed source frames")
	}
}
