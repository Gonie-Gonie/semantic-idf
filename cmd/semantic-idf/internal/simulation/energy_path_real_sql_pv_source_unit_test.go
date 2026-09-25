package simulation

// Literal native SQL, never an engine run or a production/candidate fixture.
import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathSQLPVSourceLiteral struct {
	id, name, key string
	meter         bool
	power         float64
}

// Independent literal output-name census and distinct signed hand powers.
// These are NOT a claim that the artificial electrical circuit balances.
func epathSQLPVSourceLiterals() []epathSQLPVSourceLiteral {
	d := epathSQLPVOriginalDeclaration()
	return []epathSQLPVSourceLiteral{
		{"pv.dc.1", "Generator Produced DC Electricity Energy", d.Generators[0].Name, false, 1},
		{"pv.dc.2", "Generator Produced DC Electricity Energy", d.Generators[1].Name, false, 2},
		{"pv.dc.3", "Generator Produced DC Electricity Energy", d.Generators[2].Name, false, 3},
		{"pv.dc.4", "Generator Produced DC Electricity Energy", d.Generators[3].Name, false, 4},
		{"pv.dc.5", "Generator Produced DC Electricity Energy", d.Generators[4].Name, false, 5},
		{"inverter.dc_input", "Inverter DC Input Electricity Energy", d.InverterName, false, 11},
		{"inverter.ac_output", "Inverter AC Output Electricity Energy", d.InverterName, false, 10},
		{"inverter.loss", "Inverter Conversion Loss Energy", d.InverterName, false, .0034},
		{"inverter.loss_decrement", "Inverter Conversion Loss Decrement Energy", d.InverterName, false, -.0034},
		{"inverter.ancillary", "Inverter Ancillary AC Electricity Energy", d.InverterName, false, 0},
		{"storage.charge", "Electric Storage Charge Energy", d.StorageName, false, 7},
		{"storage.decrement", "Electric Storage Production Decrement Energy", d.StorageName, false, -7},
		{"storage.discharge", "Electric Storage Discharge Energy", d.StorageName, false, 6},
		{"storage.thermal", "Electric Storage Thermal Loss Energy", d.StorageName, false, -2.5},
		{"distribution.electricity", "Electric Load Center Produced Electricity Energy", d.LoadCenterName, false, 8},
		{"distribution.thermal", "Electric Load Center Produced Thermal Energy", d.LoadCenterName, false, -1.5},
		{"facility.demand", "Electricity:Facility", "", true, 9},
		{"facility.produced", "ElectricityProduced:Facility", "", true, -.25},
		{"facility.purchased", "ElectricityPurchased:Facility", "", true, 9.25},
		{"facility.sold", "ElectricitySurplusSold:Facility", "", true, 0},
	}
}

func epathSQLPVSourceHandRequests(literals []epathSQLPVSourceLiteral) (string, PurposeRunPlan) {
	var output strings.Builder
	plan := PurposeRunPlan{BasicEnergyDetail: "energy_path"}
	for _, literal := range literals {
		for _, frequency := range []string{"Monthly", "Hourly"} {
			fields := []string{literal.key, literal.name, frequency}
			typ := "Output:Variable"
			name, key := literal.name, literal.key
			if literal.meter {
				fields = []string{literal.name, frequency}
				typ = "Output:Meter"
				name, key = "", literal.name
			}
			fmt.Fprintf(&output, "%s,%s;\n", typ, strings.Join(fields, ","))
			request := PurposeOutputObject{ObjectType: typ, VariableName: name, KeyValue: key, ReportingFrequency: frequency, State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}}
			for _, field := range fields {
				request.Fields = append(request.Fields, idf.OutputFieldValue{Value: field})
			}
			plan.OutputObjects = append(plan.OutputObjects, request)
		}
	}
	return output.String(), plan
}

func epathSQLPVSourceSQL(t *testing.T, literals []epathSQLPVSourceLiteral, mutations ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "independent-pv-native.sql")
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
		`CREATE TABLE EnvironmentPeriods(EnvironmentPeriodIndex INTEGER PRIMARY KEY,EnvironmentName TEXT,EnvironmentType INTEGER)`,
		`CREATE TABLE Time(TimeIndex INTEGER PRIMARY KEY,EnvironmentPeriodIndex INTEGER,Year INTEGER,Month INTEGER,Day INTEGER,Hour INTEGER,Minute INTEGER,"Interval" REAL,IntervalType INTEGER,SimulationDays INTEGER,WarmupFlag INTEGER)`,
		`CREATE TABLE ReportDataDictionary(ReportDataDictionaryIndex INTEGER PRIMARY KEY,Name TEXT,KeyValue TEXT,IsMeter INTEGER,ReportingFrequency TEXT,Units TEXT,Type TEXT,TimestepType TEXT,IndexGroup TEXT,ScheduleName TEXT)`,
		`CREATE TABLE ReportData(ReportDataIndex INTEGER PRIMARY KEY,TimeIndex INTEGER,ReportDataDictionaryIndex INTEGER,Value REAL)`,
		`INSERT INTO EnvironmentPeriods VALUES(1,'design',1),(3,'controlled weather',3)`,
		`INSERT INTO Time VALUES(99991,1,2017,1,1,1,0,60,1,1,0),(99992,3,2017,1,1,1,0,60,1,1,1)`,
	} {
		if _, err := tx.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	statement, err := tx.Prepare(`INSERT INTO Time VALUES(?,3,2017,?,?,?,?,?,?,?,NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	for month := 1; month <= 12; month++ {
		last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
		if _, err := statement.Exec(month, month, last.Day(), 24, 0, last.Day()*1440, 3, last.YearDay()); err != nil {
			statement.Close()
			t.Fatal(err)
		}
	}
	for slot := 0; slot < 8760; slot++ {
		date := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(slot) * time.Hour)
		if _, err := statement.Exec(100+slot, int(date.Month()), date.Day(), date.Hour()+1, 0, 60, 1, date.YearDay()); err != nil {
			statement.Close()
			t.Fatal(err)
		}
	}
	if err := statement.Close(); err != nil {
		t.Fatal(err)
	}
	for n, literal := range literals {
		for offset, frequency := range []string{"Monthly", "Hourly"} {
			id := 2000 + 2*n + offset
			meter := 0
			step, group := "HVAC System", "System"
			var key any = strings.ToUpper(literal.key)
			if literal.meter {
				meter = 1
				step = "Zone"
				group = "Facility:" + strings.TrimSuffix(literal.name, ":Facility")
				key = nil
			}
			if _, err := tx.Exec(`INSERT INTO ReportDataDictionary VALUES(?,?,?,?,?,'J','Sum',?,?,NULL)`, id, literal.name, key, meter, frequency, step, group); err != nil {
				t.Fatal(err)
			}
			intervalType := 3
			if frequency == "Hourly" {
				intervalType = 1
			}
			if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,?,?*"Interval"/60*3600000 FROM Time WHERE EnvironmentPeriodIndex=3 AND WarmupFlag IS NULL AND IntervalType=?`, id, literal.power, intervalType); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(`INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(99991,?,999999999999),(99992,?,-999999999999)`, id, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, mutation := range mutations {
		if _, err := tx.Exec(mutation); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func epathSQLPVSourceOriginal(t *testing.T) (string, epathSQLPVOriginalProof) {
	t.Helper()
	original := epathSQLPVOriginalFixture(t)
	proofs, err := epathSQLValidatePVOriginal(original, []epathRealSQLPVSystem{epathSQLPVOriginalDeclaration()})
	if err != nil || len(proofs) != 1 {
		t.Fatalf("original proof: %v", err)
	}
	return original, proofs[0]
}

func TestEnergyPathSQLPVNativeSourceCompilerFortyOriginalBoundObservations(t *testing.T) {
	original, proof := epathSQLPVSourceOriginal(t)
	literals := epathSQLPVSourceLiterals()
	path := epathSQLPVSourceSQL(t, literals)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	requests, plan := epathSQLPVSourceHandRequests(literals)
	observed.originalText, observed.executedText, observed.outputPlan = original, requests+original, &plan
	frame, err := epathCompileSQLPVSourceFrames(observed, proof, epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(frame.Sources) != 40 || len(frame.Original.Owners) != 8 || len(frame.Original.PhysicalObjects) != 23 || len(frame.ExecutedOwners) != 8 {
		t.Fatal("native source/physical census changed")
	}
	if frame.SQLSHA256 != fmt.Sprintf("%x", sha256.Sum256(before)) || frame.ExecutedSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(observed.executedText))) {
		t.Fatal("SQL/executed bytes were not separately bound")
	}
	executed, err := idf.Parse(observed.executedText)
	if err != nil {
		t.Fatal(err)
	}
	for n, literal := range literals {
		for offset, frequency := range []string{"Monthly", "Hourly"} {
			identity, found := frame.Sources[2000+2*n+offset]
			if !found || identity.Spec.ID != literal.id || identity.Spec.Name != literal.name || !strings.EqualFold(identity.Spec.Key, literal.key) || identity.Spec.IsMeter != literal.meter || identity.Source.ReportingFrequency != frequency || identity.Source.SourceUnit != "J" {
				t.Fatalf("literal native registry changed: %+v", identity)
			}
			if identity.Source.RawSum == nil || identity.Source.EnergyKWh == nil || !epathSQLPVNativeNear(identity.NativeAnnualKWh, literal.power*8760) || identity.HasNegativeValue != (literal.power < 0) {
				t.Fatalf("source native signed/zero annual changed: %+v", identity.Source)
			}
			if identity.OutputObjectIndex != nil || identity.EffectiveMultiplier != 1 || identity.AggregationBasis != "model_total" || !identity.RequestBound {
				t.Fatal("temporary output or native multiplier changed")
			}
			if literal.meter {
				if identity.OriginalOwnerIndex != nil || identity.ExecutedOwnerIndex != nil || !identity.Dictionary.KeyWasNULL {
					t.Fatal("native NULL-key Facility meter acquired owner")
				}
			} else {
				want := epathSQLPVUnitObject(t, &executed, identity.Spec.Owner.ObjectType, identity.Spec.Owner.ObjectName)
				if identity.OriginalOwnerIndex == nil || identity.ExecutedOwnerIndex == nil || *identity.OriginalOwnerIndex != identity.Spec.Owner.ObjectIndex || *identity.ExecutedOwnerIndex != want.Index || *identity.ExecutedOwnerIndex == *identity.OriginalOwnerIndex {
					t.Fatal("executed owner was guessed from original/output index")
				}
			}
			for month := 1; month <= 12; month++ {
				hours := float64(time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Day() * 24)
				if !epathSQLPVNativeNear(identity.NativeMonthlyKWh[month-1], literal.power*hours) {
					t.Fatalf("native M%d changed", month)
				}
			}
		}
	}
	if err := epathSQLValidatePVSourceFrames(frame); err != nil {
		t.Fatal(err)
	}
	// Mutate only a copy of one retained row/summary; no SQL/candidate change.
	for _, mode := range []string{"multiplier", "original_index", "executed_index", "request", "precision", "census", "wrong_sign_flag", "raw_summary", "source_role", "duplicate_period"} {
		t.Run(mode, func(t *testing.T) {
			changed := frame
			changed.Sources = map[int]epathSQLPVSourceIdentity{}
			for id, value := range frame.Sources {
				changed.Sources[id] = value
			}
			identity := changed.Sources[2000]
			switch mode {
			case "multiplier":
				identity.EffectiveMultiplier = 3
			case "original_index":
				wrong := *identity.OriginalOwnerIndex + 1
				identity.OriginalOwnerIndex = &wrong
			case "executed_index":
				wrong := *identity.OriginalOwnerIndex
				identity.ExecutedOwnerIndex = &wrong
			case "request":
				identity.RequestBound = false
			case "precision":
				changed.Precision.DecimalPlaces = 2
			case "census":
				changed.DictionaryCensusComplete = false
			case "wrong_sign_flag":
				identity.HasNegativeValue = true
			case "raw_summary":
				identity.NativeAnnualKWh++
			case "source_role":
				identity.Spec.Role = "thermal_context"
			case "duplicate_period":
				identity.Rows = append([]epathSQLPVNativeRow(nil), identity.Rows...)
				identity.Rows[1] = identity.Rows[0]
			}
			changed.Sources[2000] = identity
			if err := epathSQLValidatePVSourceFrames(changed); err == nil {
				t.Fatal("altered native proof accepted")
			}
		})
	}
	bad := observed
	doc, err := idf.Parse(observed.executedText)
	if err != nil {
		t.Fatal(err)
	}
	battery := epathSQLPVUnitObject(t, &doc, "ElectricLoadCenter:Storage:Battery", proof.Declaration.StorageName)
	battery.Fields[6].Value = "999"
	bad.executedText = doc.String()
	if _, err := epathCompileSQLPVSourceFrames(bad, proof, frame.Precision); err == nil {
		t.Fatal("changed physical battery accepted by source compiler")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("independent source compiler changed SQL")
	}
}

func TestEnergyPathSQLPVNativeUnknownAndMetadataAreNotZero(t *testing.T) {
	_, proof := epathSQLPVSourceOriginal(t)
	specs, err := epathSQLPVSourceSpecs(proof)
	if err != nil {
		t.Fatal(err)
	}
	spec := specs[10]
	literal := epathSQLPVSourceLiterals()[10]
	cases := []struct{ name, sql string }{
		{"NULL", `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=2000 AND TimeIndex=1`},
		{"missing month", `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=2000 AND TimeIndex=1`},
		{"duplicate row", `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) SELECT TimeIndex,ReportDataDictionaryIndex,Value FROM ReportData WHERE ReportDataDictionaryIndex=2000 AND TimeIndex=1`},
		{"duplicate dictionary", `INSERT INTO ReportDataDictionary SELECT 2999,Name,KeyValue,IsMeter,ReportingFrequency,Units,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=2000`},
		{"NULL component key", `UPDATE ReportDataDictionary SET KeyValue=NULL WHERE ReportDataDictionaryIndex=2000`},
		{"foreign component key", `UPDATE ReportDataDictionary SET KeyValue='Other Battery' WHERE ReportDataDictionaryIndex=2000`},
		{"meter masquerade", `UPDATE ReportDataDictionary SET IsMeter=1 WHERE ReportDataDictionaryIndex=2000`},
		{"wrong units", `UPDATE ReportDataDictionary SET Units='W' WHERE ReportDataDictionaryIndex=2000`},
		{"wrong aggregation", `UPDATE ReportDataDictionary SET Type='Avg' WHERE ReportDataDictionaryIndex=2000`},
		{"wrong native timestep", `UPDATE ReportDataDictionary SET TimestepType='Zone' WHERE ReportDataDictionaryIndex=2000`},
		{"scheduled observation", `UPDATE ReportDataDictionary SET ScheduleName='Filtered' WHERE ReportDataDictionaryIndex=2000`},
		{"negative purchased transfer", `UPDATE ReportData SET Value=-1 WHERE ReportDataDictionaryIndex=2000 AND TimeIndex=1`},
		{"orphan", `INSERT INTO ReportData(TimeIndex,ReportDataDictionaryIndex,Value) VALUES(999999,2000,1)`},
		{"wrong period axis", `UPDATE ReportData SET TimeIndex=100 WHERE ReportDataDictionaryIndex=2000 AND TimeIndex=1`},
		{"wrong cumulative days", `UPDATE Time SET SimulationDays=30 WHERE TimeIndex=1`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := epathSQLPVSourceSQL(t, []epathSQLPVSourceLiteral{literal}, tc.sql)
			db, err := epathOpenOracleSQL(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			weather, err := epathReadOracleWeather(db)
			if err != nil {
				return
			}
			dictionary, err := epathSQLPVReadDictionary(db, spec, "Monthly")
			if err != nil {
				return
			}
			rows, err := epathSQLPVReadRows(db, dictionary)
			if err != nil {
				return
			}
			source, _, _, _, err := epathSQLPVNativeSummary(spec, dictionary, weather, rows)
			if err == nil {
				t.Fatalf("invalid native observation became known source: %+v", source)
			}
			if source.RawSum != nil || source.EnergyKWh != nil {
				t.Fatal("failed observation retained a scalar proof")
			}
		})
	}
}

func TestEnergyPathSQLPVNativeSignRulesCannotClampOrCrossCancel(t *testing.T) {
	_, proof := epathSQLPVSourceOriginal(t)
	specs, err := epathSQLPVSourceSpecs(proof)
	if err != nil {
		t.Fatal(err)
	}
	weather := epathRealSQLWeather{Year: 2017, EnvironmentIndex: 3, Months: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, CoverageBasis: "monthly_intervals_and_cumulative_simulation_days"}
	for _, index := range []int{8, 11, 13, 15, 17} {
		spec := specs[index]
		d := epathSQLPVNativeDictionary{Index: 1, Name: spec.Name, Key: spec.Key, IsMeter: spec.IsMeter, Frequency: "Monthly", Unit: "J", Type: "Sum", TimestepType: "HVAC System"}
		if spec.IsMeter {
			d.TimestepType = "Zone"
		}
		var rows []epathSQLPVNativeRow
		for month := 1; month <= 12; month++ {
			last := time.Date(2017, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
			value := -3600000.0
			if spec.Sign == "signed" && month > 1 {
				value = 3600000
			}
			rows = append(rows, epathSQLPVNativeRow{TimeIndex: month, EnvironmentIndex: 3, Year: 2017, Month: month, Day: last.Day(), Hour: 24, IntervalType: 3, SimulationDays: last.YearDay(), IntervalMinutes: float64(last.Day() * 1440), NativeJ: value})
		}
		source, _, energy, negative, err := epathSQLPVNativeSummary(spec, d, weather, rows)
		if err != nil {
			t.Fatal(err)
		}
		if !negative || energy[0] != -1 || source.EnergyKWh == nil {
			t.Fatal("signed/decrement observation was clamped")
		}
		if spec.Sign == "signed" && *source.EnergyKWh != 10 {
			t.Fatal("positive signed total lost its negative period")
		}
		if spec.Sign == "nonpositive" {
			rows[1].NativeJ = 1
			if _, _, _, _, err := epathSQLPVNativeSummary(spec, d, weather, rows); err == nil {
				t.Fatal("negative annual decrement hid forbidden positive month")
			}
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if epathSQLPVSignValid("signed", value) {
			t.Fatal("signed context accepted nonfinite")
		}
	}
}

func TestEnergyPathSQLPVSourceOutputOpenerUsesRawOriginalCensus(t *testing.T) {
	spec := epathSQLPVNativeSpec{Name: "Electric Storage Charge Energy", Key: "Kibam"}
	parse := func(text string) idf.Document {
		doc, err := idf.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	line := "Output:Variable,Kibam,Electric Storage Charge Energy,Monthly;\n"
	old := parse(line)
	run := parse("Output:Variable,*,Site Outdoor Air Drybulb Temperature,Hourly;\n" + line)
	request := PurposeOutputObject{ObjectType: "Output:Variable", VariableName: spec.Name, KeyValue: spec.Key, ReportingFrequency: "Monthly", State: "existing", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, ObjectIndex: new(int), Fields: []idf.OutputFieldValue{{Value: "Kibam"}, {Value: spec.Name}, {Value: "Monthly"}}}
	plan := PurposeRunPlan{BasicEnergyDetail: "energy_path", OutputObjects: []PurposeOutputObject{request}}
	opener, err := epathSQLPVRequestBinding(old, run, &plan, spec, "Monthly")
	if err != nil || opener == nil || *opener != 0 {
		t.Fatalf("original opener was confused with shifted execution index: %v/%v", opener, err)
	}
	duplicate := parse(line + line)
	opener, err = epathSQLPVRequestBinding(duplicate, duplicate, &plan, spec, "Monthly")
	if err != nil || opener != nil {
		t.Fatal("deduplicated plan hid duplicate original declarations")
	}
	wildcard := parse(line + "Output:Variable,*,Electric Storage Charge Energy,Monthly;")
	opener, err = epathSQLPVRequestBinding(wildcard, wildcard, &plan, spec, "Monthly")
	if err != nil || opener != nil {
		t.Fatal("exact+wildcard original ambiguity guessed opener")
	}
	if _, err := epathSQLPVRequestBinding(duplicate, old, &plan, spec, "Monthly"); err == nil {
		t.Fatal("executed request erased an original")
	}
	bad := plan
	bad.OutputObjects = append([]PurposeOutputObject(nil), plan.OutputObjects...)
	wrong := 1
	bad.OutputObjects[0].ObjectIndex = &wrong
	if _, err := epathSQLPVRequestBinding(old, run, &bad, spec, "Monthly"); err == nil {
		t.Fatal("executed output index masqueraded as original opener")
	}
	if !reflect.DeepEqual(plan.OutputObjects[0].Fields, request.Fields) || *plan.OutputObjects[0].ObjectIndex != 0 {
		t.Fatal("request binding mutated caller")
	}
}
