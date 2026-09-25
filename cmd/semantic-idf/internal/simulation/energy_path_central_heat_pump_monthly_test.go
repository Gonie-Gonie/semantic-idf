package simulation

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func centralHeatPumpMonthlyFixture(t *testing.T) (string, *sql.DB, PurposeRunPlan, energyDriverBuildContext, []energyExplanationSeries) {
	t.Helper()
	doc := energyPathCentralHeatPumpOriginal(t)
	context := energyDriverBuildContext{Enabled: true, HasCentralHeatPump: true, CentralHeatPumpTargets: energyPathCentralHeatPumpOutputTargets(doc)}
	plan := PurposeRunPlan{BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
	var parents []energyExplanationSeries
	path := filepath.Join(t.TempDir(), "native-central.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER,KeyValue TEXT,Name TEXT,Units TEXT,ReportingFrequency TEXT,IsMeter INTEGER,Type TEXT,TimestepType TEXT,IndexGroup TEXT,ScheduleName TEXT)`)
	exec(`CREATE TABLE "Time"(TimeIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,IntervalType INTEGER,EnvironmentPeriodIndex INTEGER,WarmupFlag INTEGER)`)
	exec(`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER,EnvironmentType INTEGER)`)
	exec(`CREATE TABLE ReportData(ReportDataIndex INTEGER,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`)
	exec(`INSERT INTO EnvironmentPeriods VALUES(3,3),(2,1)`)
	for month := 1; month <= 12; month++ {
		days := time.Date(2017, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		exec(`INSERT INTO "Time" VALUES(?,2017,?,?,24,0,?,3,3,NULL)`, month, month, days, days*1440)
	}
	exec(`INSERT INTO "Time" VALUES(91,2017,1,31,24,0,44640,3,2,0)`)
	exec(`INSERT INTO "Time" VALUES(92,2017,1,31,24,0,44640,3,3,1)`)
	for index, service := range []string{"cooling", "heating"} {
		id, parent := 10+index, 30+index
		name := "Chiller Heater System " + strings.ToUpper(service[:1]) + service[1:] + " Electricity Energy"
		outputIndex := 800 + index
		plan.OutputObjects = append(plan.OutputObjects, PurposeOutputObject{ObjectType: "Output:Variable", KeyValue: "ChillerBank", VariableName: name, ReportingFrequency: "Monthly", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, ObjectIndex: &outputIndex})
		parentName := strings.ToUpper(service[:1]) + service[1:] + ":Electricity"
		definition, known := energyMeterAliasDefinitionForName(parentName)
		if !known {
			t.Fatalf("native parent is not a meter: %s", parentName)
		}
		parentBuilder := energyExplanationSeriesBuilder{dictionary: energyExplanationDictionary{row: sqlOutputDictionaryRow{index: parent, name: parentName, units: "J"}, isMeter: true, reportingFrequency: "Monthly", meter: &definition}, unit: "kWh"}
		parents = append(parents, canonicalEnergyExplanationSeries(energyExplanationSeriesForBuilder(&parentBuilder, fmt.Sprintf("sql-rdd-%d", parent))))
		exec(`INSERT INTO ReportDataDictionary VALUES(?,'CHILLERBANK',?,'J','Monthly',0,'Sum','HVAC System','System',NULL)`, id, name)
		exec(`INSERT INTO ReportDataDictionary VALUES(?,'',?,'J','Monthly',1,'Sum','Zone','Facility',NULL)`, parent, parentName)
		for month := 1; month <= 12; month++ {
			value := 4.0
			if service == "cooling" {
				value = 10
				if month == 1 {
					value = 0
				}
			}
			exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, id*100+month, month, id, value*3600000)
			exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, parent*100+month, month, parent, value*3600000)
		}
		// Excluded design and warmup rows must not inflate either paid budget.
		exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, id*100+91, 91, id, 999*3600000)
		exec(`INSERT INTO ReportData VALUES(?,?,?,?)`, id*100+92, 92, id, 999*3600000)
	}
	// A huge Rate/module/thermal observation cannot become a third budget.
	for index, name := range []string{"Chiller Heater System Cooling Electricity Rate", "Chiller Heater Cooling Electricity Energy Unit 1", "Chiller Heater System Cooling Energy"} {
		exec(`INSERT INTO ReportDataDictionary VALUES(?,'CHILLERBANK',?,'J','Monthly',0,'Sum','HVAC System','System',NULL)`, 60+index, name)
		exec(`INSERT INTO ReportData VALUES(?,1,?,3600000000000)`, 6000+index, 60+index)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path, db, plan, context, parents
}

func TestEnergyPathCentralHeatPumpMonthlyRequestsOnlyTwoPaidChannels(t *testing.T) {
	doc := energyPathCentralHeatPumpOriginal(t)
	full := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	paid := 0
	for _, output := range full.OutputObjects {
		if output.VariableName != "Chiller Heater System Cooling Electricity Energy" && output.VariableName != "Chiller Heater System Heating Electricity Energy" {
			continue
		}
		paid++
		if output.ReportingFrequency != "Monthly" || output.KeyValue != "ChillerBank" || output.ScopeZoneName != "" {
			t.Fatalf("actual plan hook mirrored/promoted the bounded request: %#v", output)
		}
	}
	if paid != 2 {
		t.Fatalf("actual plan hook emitted %d paid source requests", paid)
	}
	for _, scenario := range []string{"ordinary", "existing wildcard", "existing blank", "existing whitespace blank", "scheduled wildcard", "scheduled blank"} {
		t.Run(scenario, func(t *testing.T) {
			builder := newPurposePlanBuilder(doc, NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}))
			if scenario != "ordinary" {
				key := "*"
				if strings.Contains(scenario, "blank") {
					key = ""
				}
				if scenario == "existing whitespace blank" {
					key = " \t"
				}
				for _, definition := range energyPathCentralHeatPumpOutputDefinitions() {
					if definition.Role != energyPathCentralHeatPumpPurchased {
						continue
					}
					output := PurposeOutputObject{ObjectType: "Output:Variable", KeyValue: key, VariableName: definition.EnergyName, ReportingFrequency: "Monthly", Fields: []idf.OutputFieldValue{
						{Name: "Key Value", Value: key}, {Name: "Variable Name", Value: definition.EnergyName}, {Name: "Reporting Frequency", Value: "Monthly"},
					}}
					if strings.HasPrefix(scenario, "scheduled") {
						// An arbitrary scheduled object cannot satisfy the whole-month contract.
						output.Fields = append(output.Fields, idf.OutputFieldValue{Name: "Schedule Name", Value: "LIMITED"})
					}
					output.Signature = PurposeOutputSignature(output.ObjectType, output.Fields)
					builder.existing[output.Signature] = output
				}
			}
			builder.addEnergyPathCentralHeatPumpMonthlyOutputs()
			builder.addEnergyPathCentralHeatPumpMonthlyOutputs()
			if len(builder.objects) != 2 {
				t.Fatalf("minimum paid roster is two requests, not five definitions or count3 modules: %d", len(builder.objects))
			}
			seen := map[string]bool{}
			for _, output := range builder.objects {
				if output.ReportingFrequency != "Monthly" || output.ScopeZoneName != "" || !strings.Contains(output.VariableName, "System ") || !strings.HasSuffix(output.VariableName, " Electricity Energy") || seen[output.VariableName] {
					t.Fatalf("wrong native paid request: %#v", output)
				}
				seen[output.VariableName] = true
				wantKey := "ChillerBank"
				if scenario == "existing wildcard" {
					wantKey = "*"
				} else if strings.HasPrefix(scenario, "existing") && strings.Contains(scenario, "blank") {
					wantKey = ""
				}
				if output.KeyValue != wantKey {
					t.Fatalf("wrong unfiltered original/exact key reuse: %#v", output)
				}
			}
		})
	}
}

func TestEnergyPathCentralHeatPumpMonthlyNativeBudgetsRemainGlobalAndDistinct(t *testing.T) {
	path, db, plan, context, parents := centralHeatPumpMonthlyFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pools, sources := readEnergyPathCentralHeatPumpMonthlyPools(db, filepath.Base(path), &plan, context, parents)
	if len(pools) != 2 || len(sources) != 2 {
		t.Fatalf("wrong two-channel census: pools=%d sources=%d", len(pools), len(sources))
	}
	for index, pool := range pools {
		service, want := "cooling", 110.0
		if index == 1 {
			service, want = "heating", 48
		}
		if !pool.Valid || pool.ServiceKind != service || pool.Carrier != "electricity" || len(pool.Members) != 1 || len(pool.MeterSourceIDs) != 1 || pool.MeterSourceIDs[0] != fmt.Sprintf("sql-rdd-%d", 30+index) {
			t.Fatalf("paid service or parent collapsed: %#v", pool)
		}
		member, source := pool.Members[0], sources[index]
		if member.Series.Total != want || len(member.Series.Monthly) != 12 || len(member.RelatedPathIDs) != 0 || member.ZoneName != "" || member.Series.ZoneName != "" || member.Series.EffectiveMultiplier != 1 || member.Series.MultiplierApplication != "already_model_total" || member.Series.SourceIDs[0] != fmt.Sprintf("sql-rdd-%d", 10+index) {
			t.Fatalf("system budget became module/Zone/thermal quantity: %#v", member)
		}
		if source.RawValue != want || source.EffectiveValue != want || source.ZoneName != "" || source.EffectiveMultiplier != 1 || source.AggregationBasis != "model_total" || source.MultiplierApplication != "already_model_total" || source.DriverRole != energyDriverSourceRoleContext || source.HourlyEnergy != nil || source.ObjectIndex == nil || *source.ObjectIndex != 800+index || !energyDataSourceValueKnown(source, energySourceObservedRaw) {
			t.Fatalf("native scalar/navigation changed: %#v", source)
		}
		if index == 0 {
			if value, known := member.Series.Monthly[1]; !known || value != 0 {
				t.Fatal("native observed zero was lost")
			}
		}
	}
	// Exercise actual parser glue too: native pools are private context, not
	// additional additive end-use series or another generic Chiller alias.
	parsed, err := parseSimulationEnergyExplanationCanonicalSQL(path, &plan, context)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.HVACConsumptionPools) != 2 {
		t.Fatalf("canonical parser omitted distinct pools: %#v", parsed.HVACConsumptionPools)
	}
	for index, pool := range parsed.HVACConsumptionPools {
		if len(pool.MeterSourceIDs) != 1 || pool.MeterSourceIDs[0] != fmt.Sprintf("sql-rdd-%d", 30+index) {
			t.Fatalf("actual parser produced an unbound paid pool: %#v", pool)
		}
	}
	for _, item := range parsed.Series {
		if strings.Contains(item.SourceName, "Chiller Heater System") {
			t.Fatalf("native constituent entered additive classifier: %#v", item)
		}
	}
	for pass := 0; pass < 2; pass++ {
		wire, err := json.Marshal(sources)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(wire, &sources); err != nil {
			t.Fatal(err)
		}
		if sources[0].RawValue != 110 || sources[1].RawValue != 48 || !energyDataSourceValueKnown(sources[0], energySourceObservedRaw) || !energyDataSourceValueKnown(sources[1], energySourceObservedEffective) {
			t.Fatal("source JSON conflated services or lost scalar presence")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("native read changed SQL")
	}
}

func TestEnergyPathCentralHeatPumpMonthlyBlankNativeRequestOwner(t *testing.T) {
	path, db, _, context, parents := centralHeatPumpMonthlyFixture(t)
	doc := energyPathCentralHeatPumpOriginal(t)
	literal, err := idf.Parse(`Output:Variable,,Chiller Heater System Cooling Electricity Energy,Monthly;
Output:Variable,ChillerBank,Chiller Heater System Cooling Electricity Energy,Monthly,LIMITED;
Output:Variable, ,Chiller Heater System Heating Electricity Energy,Monthly;
Output:Variable,ChillerBank,Chiller Heater System Heating Electricity Energy,Monthly,LIMITED;`)
	if err != nil {
		t.Fatal(err)
	}
	wantIndices := map[string]int{}
	for _, object := range literal.Objects {
		object.Index = len(doc.Objects)
		if strings.TrimSpace(object.Fields[0].Value) == "" {
			wantIndices[object.Fields[1].Value] = object.Index
		}
		doc.Objects = append(doc.Objects, object)
	}
	before := doc.String()
	plan := BuildPurposeRunPlan(doc, SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath})
	paid := 0
	for _, output := range plan.OutputObjects {
		index, wanted := wantIndices[output.VariableName]
		if !wanted {
			continue
		}
		paid++
		if output.KeyValue != "" || output.ReportingFrequency != "Monthly" || output.State != PurposeOutputStateExisting || output.ObjectIndex == nil || *output.ObjectIndex != index || strings.TrimSpace(purposeFieldValue(output.Fields, "Schedule Name")) != "" {
			t.Fatalf("literal blank wildcard was rewritten/duplicated: %#v", output)
		}
	}
	if paid != 2 || doc.String() != before {
		t.Fatal("blank Monthly requests changed the original or paid channel census")
	}
	// A separately requested scheduled exact identity must neither authorize
	// the complete month nor displace the actual blank request's navigation.
	for _, output := range collectExistingPurposeOutputs(doc) {
		if purposeFieldValue(output.Fields, "Schedule Name") == "LIMITED" {
			output.PurposeIDs = []SimulationPurposeID{SimulationPurposeBasicEnergy}
			plan.OutputObjects = append(plan.OutputObjects, output)
		}
	}
	pools, sources := readEnergyPathCentralHeatPumpMonthlyPools(db, filepath.Base(path), &plan, context, parents)
	if len(pools) != 2 || len(sources) != 2 {
		t.Fatalf("blank wildcard lost the two native paid channels: %d/%d", len(pools), len(sources))
	}
	for index, want := range []float64{110, 48} {
		pool, source := pools[index], sources[index]
		if !pool.Valid || len(pool.Members) != 1 {
			t.Fatalf("blank wildcard changed native source identity: %#v", pool)
		}
		member := pool.Members[0]
		if source.ID != fmt.Sprintf("sql-rdd-%d", 10+index) || source.KeyValue != "CHILLERBANK" || source.Name != member.OutputName || source.ObjectIndex == nil || *source.ObjectIndex != wantIndices[source.Name] {
			t.Fatalf("blank native request lost its literal owner or SQL identity: %#v", source)
		}
		if source.RawValue != want || source.EffectiveValue != want || source.EffectiveMultiplier != 1 || source.MultiplierApplication != "already_model_total" || source.AggregationBasis != "model_total" || !energyDataSourceValueKnown(source, energySourceObservedRaw) || !energyDataSourceValueKnown(source, energySourceObservedEffective) || member.Series.Total != want || len(member.Series.Monthly) != 12 || member.Series.EffectiveMultiplier != 1 {
			t.Fatalf("blank wildcard changed 110/48 native budgets or multiplied three modules: %#v / %#v", source, member)
		}
	}
}

func TestEnergyPathCentralHeatPumpMonthlyUnknownCannotBorrowOtherService(t *testing.T) {
	for _, test := range []struct{ name, query string }{
		{"missing dictionary", `DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=11`},
		{"old timestep is not monthly", `UPDATE ReportDataDictionary SET ReportingFrequency='Zone Timestep' WHERE ReportDataDictionaryIndex=11`},
		{"rate is not energy", `UPDATE ReportDataDictionary SET Name='Chiller Heater System Heating Electricity Rate',Units='W' WHERE ReportDataDictionaryIndex=11`},
		{"wrong unit", `UPDATE ReportDataDictionary SET Units='kWh' WHERE ReportDataDictionaryIndex=11`},
		{"wrong meter", `UPDATE ReportDataDictionary SET IsMeter=1 WHERE ReportDataDictionaryIndex=11`},
		{"wrong type", `UPDATE ReportDataDictionary SET Type='Avg' WHERE ReportDataDictionaryIndex=11`},
		{"wrong timestep type", `UPDATE ReportDataDictionary SET TimestepType='Zone' WHERE ReportDataDictionaryIndex=11`},
		{"wrong index group", `UPDATE ReportDataDictionary SET IndexGroup='Zone' WHERE ReportDataDictionaryIndex=11`},
		{"scheduled", `UPDATE ReportDataDictionary SET ScheduleName='LIMITED' WHERE ReportDataDictionaryIndex=11`},
		{"duplicate dictionary", `INSERT INTO ReportDataDictionary SELECT 99,KeyValue,Name,Units,ReportingFrequency,IsMeter,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=11`},
		{"missing month", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=11 AND TimeIndex=2`},
		{"NULL month", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=11 AND TimeIndex=2`},
		{"negative month", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=11 AND TimeIndex=2`},
		{"duplicate month", `INSERT INTO ReportData SELECT 9999,TimeIndex,ReportDataDictionaryIndex,Value FROM ReportData WHERE ReportDataDictionaryIndex=11 AND TimeIndex=2`},
		{"orphan time", `INSERT INTO ReportData VALUES(9999,999,11,3600000)`},
		{"missing request", ""},
		{"scheduled blank request", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, db, plan, context, parents := centralHeatPumpMonthlyFixture(t)
			if test.name == "missing request" {
				plan.OutputObjects = plan.OutputObjects[:1]
			} else if test.name == "scheduled blank request" {
				plan.OutputObjects[1].KeyValue = " \t"
				plan.OutputObjects[1].Fields = []idf.OutputFieldValue{{Name: "Schedule Name", Value: "LIMITED"}}
			} else if _, err := db.Exec(test.query); err != nil {
				t.Fatal(err)
			}
			pools, sources := readEnergyPathCentralHeatPumpMonthlyPools(db, filepath.Base(path), &plan, context, parents)
			if len(pools) != 2 || len(pools[1].Members) != 1 || pools[0].Members[0].Series.Total != 110 {
				t.Fatal("missing Heating erased its cohort or changed Cooling")
			}
			if _, known := pools[1].Members[0].Series.Monthly[2]; known {
				t.Fatalf("bad Heating month borrowed a zero/other service: %#v", pools[1].Members[0])
			}
			for _, source := range sources {
				if source.Name == "Chiller Heater System Heating Electricity Energy" && (energyDataSourceValueKnown(source, energySourceObservedRaw) || energyDataSourceValueKnown(source, energySourceObservedEffective)) {
					t.Fatalf("incomplete Heating became known annual: %#v", source)
				}
			}
		})
	}
}

func TestEnergyPathCentralHeatPumpMonthlyKnownZeroAndUnresolvedRecipients(t *testing.T) {
	path, db, plan, context, parents := centralHeatPumpMonthlyFixture(t)
	if _, err := db.Exec(`UPDATE ReportData SET Value=0 WHERE ReportDataDictionaryIndex=11`); err != nil {
		t.Fatal(err)
	}
	pools, sources := readEnergyPathCentralHeatPumpMonthlyPools(db, filepath.Base(path), &plan, context, parents)
	if len(pools[1].Members[0].Series.Monthly) != 12 || sources[1].RawValue != 0 || !energyDataSourceValueKnown(sources[1], energySourceObservedRaw) {
		t.Fatal("observed zero Heating lost its native knownness")
	}
	for _, pool := range pools {
		if len(pool.Members[0].RelatedPathIDs) != 0 {
			t.Fatal("reporting identity fabricated Zone recipient authority")
		}
	}
	context.CentralHeatPumpTargets = nil
	pools, _ = readEnergyPathCentralHeatPumpMonthlyPools(db, filepath.Base(path), &plan, context, parents)
	if len(pools) != 2 || pools[0].Valid || pools[1].Valid || len(pools[0].MeterSourceIDs) != 1 || len(pools[1].MeterSourceIDs) != 1 {
		t.Fatal("unresolved original reopened generic broad-meter allocation")
	}
}

func TestEnergyPathCentralHeatPumpMonthlyCalendarDoesNotRepairEvidence(t *testing.T) {
	for _, query := range []string{
		`UPDATE "Time" SET "Interval"=40319 WHERE TimeIndex=2`,
		`UPDATE "Time" SET Hour=23 WHERE TimeIndex=2`,
		`UPDATE "Time" SET Day=29 WHERE TimeIndex=2`,
		`INSERT INTO "Time" SELECT 200,Year,Month,Day,Hour,Minute,"Interval",IntervalType,EnvironmentPeriodIndex,WarmupFlag FROM "Time" WHERE TimeIndex=2`,
	} {
		t.Run(query, func(t *testing.T) {
			path, db, plan, context, parents := centralHeatPumpMonthlyFixture(t)
			if _, err := db.Exec(query); err != nil {
				t.Fatal(err)
			}
			pools, sources := readEnergyPathCentralHeatPumpMonthlyPools(db, filepath.Base(path), &plan, context, parents)
			for _, pool := range pools {
				if _, known := pool.Members[0].Series.Monthly[2]; known {
					t.Fatal("invalid native calendar became a repaired Monthly budget")
				}
			}
			for _, source := range sources {
				if energyDataSourceValueKnown(source, energySourceObservedRaw) {
					t.Fatal("invalid calendar retained a known annual source")
				}
			}
		})
	}
}

func TestEnergyPathCentralHeatPumpMonthlyPoolsPreserveIndependentSibling(t *testing.T) {
	path, db, plan, context, parents := centralHeatPumpMonthlyFixture(t)
	pools, _ := readEnergyPathCentralHeatPumpMonthlyPools(db, filepath.Base(path), &plan, context, parents)
	nodes := []EnergyExplanationNode{}
	topology := energyServicePathIndex{byZoneService: map[string][]string{}}
	for index, service := range []string{"cooling", "heating"} {
		pathID := "actual." + service + ".Office"
		broad := 20.0
		if service == "heating" {
			broad = 10
		}
		nodes = append(nodes, epath100AuditEndUseNode(service, "electricity", broad, "M2", fmt.Sprintf("sql-rdd-%d", 30+index), []string{pathID}))
		nodes = append(nodes, epath100AuditLoadNode("Office", service, 100, "M2", "load."+service, []string{pathID}))
		topology.byZoneService[normalizePurposeToken("Office")+"|"+service] = []string{pathID}
	}
	pools = append(pools, energyPathHVACConsumptionPool{ID: "independent.boiler", ServiceKind: "heating", Carrier: "electricity", MeterSourceIDs: []string{"sql-rdd-31"}, Valid: true,
		Members: []energyPathHVACConsumptionMember{{ID: "boiler.ancillary", ObjectType: "Boiler:HotWater", ObjectName: "Separate Boiler", OutputName: energyPathBoilerAncillaryElectricityEnergy, RelatedPathIDs: []string{"actual.heating.Office"},
			Series: energyExplanationSeries{ServiceKind: "heating", Carrier: "electricity", Unit: "kWh", SourceIDs: []string{"boiler-source"}, MonthlySourceIDs: []string{"boiler-source"}, Monthly: map[int]float64{2: 2}, RawMonthly: map[int]float64{2: 2}, EffectiveMultiplier: 1, multiplierApplied: true}}}})
	allocation := buildEnergyPathZoneHVACAllocationPlan(nodes, nil, nil, "M2", "monthly", true, topology)
	allocation = reserveEnergyPathHVACConsumptionPools(allocation, nodes, topology, pools, "M2", "monthly", true)
	if len(allocation.Records) != 2 || len(allocation.Edges) != 1 || len(allocation.ConsumptionSourceAllocations) != 1 || allocation.ConsumptionSourceAllocations[0].MemberID != "boiler.ancillary" {
		t.Fatalf("native unassigned cohorts overwrote sibling allocation or fabricated Central recipients: %#v", allocation)
	}
	for _, record := range allocation.Records {
		wantBudget, wantAllocated := 20.0, 0.0
		if record.ServiceKind == "heating" {
			wantBudget, wantAllocated = 10, 2
		}
		if record.ExpectedValue != wantBudget || record.AllocatedValue != wantAllocated || record.UnassignedValue != wantBudget-wantAllocated {
			t.Fatalf("shared parent accounting changed: %#v", record)
		}
	}
}
