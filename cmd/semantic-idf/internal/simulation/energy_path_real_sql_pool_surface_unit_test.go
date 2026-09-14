package simulation

// Independent acceptance support. Original physical model plus hand SQL geometry and observed
// source records; no production qualification/reader helper creates a proof.
import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLPoolSurfaceHand(t *testing.T) (epathRealOracleEvidence, epathRealSQLModel, epathSQLFrames) {
	t.Helper()
	original := epathSQLPoolOriginalFixture(t)
	d := epathSQLPoolOriginalDeclaration()
	originals, err := epathSQLValidatePoolOriginal(original, []epathRealSQLPoolSystem{d})
	if err != nil {
		t.Fatal(err)
	}
	path := epathOracleUnitSQL(t)
	epathOracleEditSQL(t, path, `CREATE TABLE Zones(ZoneIndex INTEGER,ZoneName TEXT,Multiplier REAL,ListMultiplier REAL);
INSERT INTO Zones VALUES(1,'SPACE1-1',3,1);
CREATE TABLE Surfaces(SurfaceIndex INTEGER,SurfaceName TEXT,ClassName TEXT,ZoneIndex INTEGER,ExtBoundCond INTEGER,HeatTransferSurf INTEGER);
INSERT INTO Surfaces VALUES(19,'F1-1','Floor',1,-1,1)`)
	model := epathRealSQLModel{PoolSystems: []epathRealSQLPoolSystem{d}, Surface: epathRealSQLSurfaceModel{Mapping: "surface_class_boundary/v1", Sign: -1}}
	frames := epathSQLFrames{PoolSystems: []epathSQLPoolSourceFrames{{Original: originals[0]}}, Zones: map[string]epathSQLZone{"space1-1": {Name: "SPACE1-1", Multiplier: 3}}, Cells: map[string]*epathSQLCell{}, SourceIdentities: map[int]epathRealSQLSource{}, SourceRaw: map[int][]epathSQLQuantity{}, SourceEffective: map[int][]epathSQLQuantity{}, SourceZone: map[int]string{}, TraceSourceIdentities: map[int]epathSQLTraceSourceIdentity{}}
	// A real two-object displacement is parsed by the independent executed
	// owner helper. This is neither original +/-1 nor a mutated proof index.
	executed := "Output:Variable,*,Unrelated Test One,Monthly;\nOutput:Variable,*,Unrelated Test Two,Hourly;\n" + original
	plan := &PurposeRunPlan{BasicEnergyDetail: "energy_path"}
	var sources []epathRealSQLSource
	for keyIndex, key := range []string{"F1-1", "F2-1"} {
		for n, selection := range []struct{ name, unit, frequency string }{{"Surface Inside Face Convection Heat Gain Energy", "J", "Monthly"}, {"Surface Inside Face Convection Heat Gain Rate", "W", "Monthly"}, {"Surface Inside Face Convection Heat Gain Energy", "J", "Hourly"}, {"Surface Inside Face Convection Heat Gain Rate", "W", "Hourly"}} {
			source, _ := epathSQLPoolSourceHandObservation(700+keyIndex*4+n, selection.name, key, selection.unit, selection.frequency, false, 0.01)
			sources = append(sources, source)
			plan.OutputObjects = append(plan.OutputObjects, epathSQLPoolHandRequest(source, ""))
			executed += fmt.Sprintf("\nOutput:Variable,%s,%s,%s;", key, selection.name, selection.frequency)
		}
	}
	monthly, hourly := sources[0], sources[2]
	frames.SourceIdentities[700] = monthly
	frames.SourceIdentities[702] = hourly
	frames.SourceZone[700] = "space1-1"
	frames.SourceRaw[700] = make([]epathSQLQuantity, 12)
	frames.SourceEffective[700] = make([]epathSQLQuantity, 12)
	for month := 1; month <= 12; month++ {
		value := 0.0
		if month == 1 {
			value = -10
		}
		if month == 2 {
			value = 5
		}
		frames.SourceRaw[700][month-1] = epathSQLQuantity{Value: value}
		frames.SourceEffective[700][month-1] = epathSQLQuantity{Value: value * 3}
		frames.Cells[epathSQLKey(d.ZoneName, "surface:surface.ground_floors", month)] = &epathSQLCell{Zone: "space1-1", Family: "surface:surface.ground_floors", Category: "surface.ground_floors", Component: "sensible", Month: month, Raw: epathSQLQuantity{Value: -value}, Effective: epathSQLQuantity{Value: -value * 3}, BuildingVisible: true, SourceIDs: []int{700}}
	}
	frames.TraceSourceIdentities[702] = epathSQLTraceSourceIdentity{Source: hourly, Authority: monthly, NativeCompanion: &epathSQLHourlyCompanionIdentity{Source: hourly, Authority: monthly}}
	return epathRealOracleEvidence{sqlPath: path, originalText: original, executedText: executed, outputPlan: plan, Sources: sources}, model, frames
}

func epathSQLPoolSurfaceHandRecords(proof *epathSQLPoolSurfaceQualification) []EnergyDataSource {
	var out []EnergyDataSource
	for id := 700; id < 708; id++ {
		s := proof.Sources[id]
		source := EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", id), SourceType: "sql_report_data", Name: s.Name, KeyValue: s.Key, SourceUnit: s.Unit, Units: s.Unit, ReportingFrequency: s.Frequency, RawValue: -10, EffectiveValue: -30, EffectiveMultiplier: 3, observedValuePresence: 3}
		if s.Qualified {
			source.ZoneName = proof.Original.Declaration.ZoneName
			source.RelatedEntityIDs = []string{fmt.Sprintf("component:%d", proof.PoolIndex), fmt.Sprintf("component:%d", proof.SurfaceIndex)}
			source.Explanation = "Pool-bearing surface: the reported surface transfer is retained; this is not an isolated passive floor. Pool plant heat remains nonadditive context, not an additional Zone-air driver."
		}
		out = append(out, source)
	}
	return out
}

func TestEnergyPathRealSQLPoolSurfaceOriginalExecutedAndNumericIsolation(t *testing.T) {
	observed, model, frames := epathSQLPoolSurfaceHand(t)
	before, err := json.Marshal(frames)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := epathCompileSQLPoolSurfaceQualification(observed, model, frames)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(frames)
	if string(before) != string(after) {
		t.Fatal("qualification changed measured cells, source arrays, loads or multipliers")
	}
	if proof.SurfaceIndex != proof.Original.SurfaceObjectIndex+2 || proof.PoolIndex != proof.Original.Owners[0].ObjectIndex+2 || proof.AuthorityID != 700 || len(proof.Sources) != 8 {
		t.Fatal("original/executed/SQL/source indexes were conflated")
	}
	for month, want := range []float64{30, -15, 0} {
		cell := frames.Cells[epathSQLKey(model.PoolSystems[0].ZoneName, "surface:surface.ground_floors", month+1)]
		if cell.Effective.Value != want || !reflect.DeepEqual(cell.SourceIDs, []int{700}) {
			t.Fatal("pool-bearing floor was excluded, inverted again or lost known-zero source")
		}
	}
	sources := epathSQLPoolSurfaceHandRecords(proof)
	raw, _ := json.Marshal(sources)
	for reload := 0; reload < 2; reload++ {
		if err := epathSQLCheckPoolSurfaceQualification(sources, proof); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(sources)
		if string(raw) != string(encoded) {
			t.Fatal("metadata consumer changed candidate signed raw/effective fields or identities")
		}
		if err := json.Unmarshal(encoded, &sources); err != nil {
			t.Fatal(err)
		}
	}
	// Unselected Rate companions have no new scalar requirement here. If they
	// are present, their coupling metadata is checked; chosen E M/H cannot go.
	kept := []EnergyDataSource{sources[0], sources[2]}
	if err := epathSQLCheckPoolSurfaceQualification(kept, proof); err != nil {
		t.Fatal(err)
	}
	if p, err := epathCompileSQLPoolSurfaceQualification(epathRealOracleEvidence{}, epathRealSQLModel{}, epathSQLFrames{}); err != nil || p != nil {
		t.Fatal("legacy model unexpectedly acquired Pool qualification")
	}
}

func TestEnergyPathRealSQLPoolSurfaceRejectsOriginalNativeAndSelectionMutations(t *testing.T) {
	for _, mutation := range []string{"SQL outdoors", "SQL wall", "SQL foreign zone", "SQL multiplier", "SQL no heat transfer", "SQL duplicate floor", "SQL absent floor", "changed executed surface", "changed executed pool", "duplicate executed owner", "no executed", "missing request", "request wrong scope", "native NULL", "missing native Rate", "duplicate native identity", "native wrong unit", "Monthly removed", "cell excluded", "cell hidden", "cell context relabel", "Hourly removed"} {
		t.Run(mutation, func(t *testing.T) {
			observed, model, frames := epathSQLPoolSurfaceHand(t)
			switch mutation {
			case "SQL outdoors":
				epathOracleEditSQL(t, observed.sqlPath, `UPDATE Surfaces SET ExtBoundCond=0`)
			case "SQL wall":
				epathOracleEditSQL(t, observed.sqlPath, `UPDATE Surfaces SET ClassName='Wall'`)
			case "SQL foreign zone":
				epathOracleEditSQL(t, observed.sqlPath, `UPDATE Zones SET ZoneName='SPACE2-1'`)
			case "SQL multiplier":
				epathOracleEditSQL(t, observed.sqlPath, `UPDATE Zones SET Multiplier=9`)
			case "SQL no heat transfer":
				epathOracleEditSQL(t, observed.sqlPath, `UPDATE Surfaces SET HeatTransferSurf=0`)
			case "SQL duplicate floor":
				epathOracleEditSQL(t, observed.sqlPath, `INSERT INTO Surfaces SELECT * FROM Surfaces`)
			case "SQL absent floor":
				epathOracleEditSQL(t, observed.sqlPath, `DELETE FROM Surfaces`)
			case "changed executed surface":
				observed.executedText = epathSQLPoolMutatedOriginal(t, observed.executedText, func(doc *idf.Document) {
					epathSQLPoolUnitObject(t, doc, "BuildingSurface:Detailed", "F1-1").Fields[5].Value = "Outdoors"
				})
			case "changed executed pool":
				observed.executedText = epathSQLPoolMutatedOriginal(t, observed.executedText, func(doc *idf.Document) {
					epathSQLPoolUnitObject(t, doc, "SwimmingPool:Indoor", "Test Pool").Fields[1].Value = "F2-1"
				})
			case "duplicate executed owner":
				observed.executedText += "\nSwimmingPool:Indoor,Test Pool,F1-1;"
			case "no executed":
				observed.executedText = ""
			case "missing request":
				observed.outputPlan.OutputObjects = observed.outputPlan.OutputObjects[1:]
			case "request wrong scope":
				observed.outputPlan.OutputObjects[0].ScopeZoneName = "SPACE1-1"
			case "native NULL":
				observed.Sources[0].MissingRows = 1
			case "missing native Rate":
				observed.Sources = append(observed.Sources[:1], observed.Sources[2:]...)
			case "duplicate native identity":
				observed.Sources = append(observed.Sources, observed.Sources[0])
			case "native wrong unit":
				observed.Sources[0].SourceUnit = "W"
			case "Monthly removed":
				delete(frames.SourceRaw, 700)
			case "cell excluded":
				frames.Cells[epathSQLKey("SPACE1-1", "surface:surface.ground_floors", 1)].SourceIDs = nil
			case "cell hidden":
				frames.Cells[epathSQLKey("SPACE1-1", "surface:surface.ground_floors", 1)].BuildingVisible = false
			case "cell context relabel":
				frames.Cells[epathSQLKey("SPACE1-1", "surface:surface.ground_floors", 1)].Category = "context.pool"
			case "Hourly removed":
				delete(frames.TraceSourceIdentities, 702)
			}
			if _, err := epathCompileSQLPoolSurfaceQualification(observed, model, frames); err == nil {
				t.Fatalf("accepted %s", mutation)
			}
		})
	}
}

func TestEnergyPathRealSQLPoolSurfaceQualificationRejectsLostOrInventedCoupling(t *testing.T) {
	observed, model, frames := epathSQLPoolSurfaceHand(t)
	proof, err := epathCompileSQLPoolSurfaceQualification(observed, model, frames)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"missing explanation", "passive only", "not retained", "additive pool heat", "additional Zone driver", "lost parent", "wrong parent", "extra foreign parent", "duplicate parent", "original index", "SQL surface index", "wrong surface key", "wrong zone", "duplicate native", "missing selected zero", "missing Hourly", "nonrecipient label", "nonrecipient parent", "derived replacement", "Pool load label"} {
		t.Run(mutation, func(t *testing.T) {
			sources := epathSQLPoolSurfaceHandRecords(proof)
			s := &sources[0]
			switch mutation {
			case "missing explanation":
				s.Explanation = ""
			case "passive only":
				s.Explanation = strings.Replace(s.Explanation, "not an isolated passive", "isolated passive", 1)
			case "not retained":
				s.Explanation = strings.Replace(s.Explanation, "reported surface transfer is retained", "surface transfer was subtracted", 1)
			case "additive pool heat":
				s.Explanation = strings.Replace(s.Explanation, "nonadditive context", "additive heating", 1)
			case "additional Zone driver":
				s.Explanation = strings.Replace(s.Explanation, "not an additional Zone-air driver", "an additional Zone-air driver", 1)
			case "lost parent":
				s.RelatedEntityIDs = s.RelatedEntityIDs[1:]
			case "wrong parent":
				s.RelatedEntityIDs[0] = "component:9999"
			case "extra foreign parent":
				s.RelatedEntityIDs = append(s.RelatedEntityIDs, "component:9999")
			case "duplicate parent":
				s.RelatedEntityIDs = append(s.RelatedEntityIDs, s.RelatedEntityIDs[0])
			case "original index":
				s.RelatedEntityIDs[0] = fmt.Sprintf("component:%d", proof.Original.Owners[0].ObjectIndex)
			case "SQL surface index":
				s.RelatedEntityIDs[1] = "component:19"
			case "wrong surface key":
				s.KeyValue = "F2-1"
			case "wrong zone":
				s.ZoneName = "PLENUM-1"
			case "duplicate native":
				sources = append(sources, *s)
			case "missing selected zero":
				s.RawValue, s.EffectiveValue = 0, 0
				sources = sources[1:]
			case "missing Hourly":
				sources = append(sources[:2], sources[3:]...)
			case "nonrecipient label":
				sources[4].Explanation = s.Explanation
			case "nonrecipient parent":
				sources[4].RelatedEntityIDs = append([]string(nil), s.RelatedEntityIDs...)
			case "derived replacement":
				s.InputSourceIDs = []string{"sql-rdd-pool-water"}
				s.Formula = "surface - pool"
			case "Pool load label":
				sources = append(sources, EnergyDataSource{ID: "invented-pool-load", Name: "Indoor Pool Water Heating Energy", Explanation: s.Explanation})
			}
			if err := epathSQLCheckPoolSurfaceQualification(sources, proof); err == nil {
				t.Fatalf("accepted %s", mutation)
			}
		})
	}
}

func TestEnergyPathRealSQLPoolSurfaceNativeAllocationFormulaIsNotSourceTransform(t *testing.T) {
	observed, model, frames := epathSQLPoolSurfaceHand(t)
	proof, err := epathCompileSQLPoolSurfaceQualification(observed, model, frames)
	if err != nil {
		t.Fatal(err)
	}
	const proportional = "actual canonical service load * matching driver signed pressure / sum of matching signed pressures"
	for _, formula := range []string{proportional, "sum(distinct directional driver allocatedValue with this sole allocation source); each allocation: " + proportional} {
		for _, index := range []int{0, 4} { // Exact Pool recipient and ordinary peer.
			sources := epathSQLPoolSurfaceHandRecords(proof)
			s := &sources[index]
			s.AllocationApplied, s.Formula, s.AllocationFormula = true, formula, formula
			before, _ := json.Marshal(sources)
			if err := epathSQLCheckPoolSurfaceQualification(sources, proof); err != nil {
				t.Fatalf("native allocation explanation rejected: %v", err)
			}
			after, _ := json.Marshal(sources)
			if string(before) != string(after) {
				t.Fatal("qualification changed native signed quantities or allocation metadata")
			}
			for _, mutation := range []string{"no allocation", "mismatched explanation", "surface minus pool", "arbitrary extension", "source inputs"} {
				changed := append([]EnergyDataSource(nil), sources...)
				bad := &changed[index]
				switch mutation {
				case "no allocation":
					bad.AllocationApplied = false
				case "mismatched explanation":
					bad.AllocationFormula = proportional + " mismatched"
				case "surface minus pool":
					bad.Formula, bad.AllocationFormula = "surface - pool", "surface - pool"
				case "arbitrary extension":
					bad.Formula += "; subtract Pool water heating"
					bad.AllocationFormula = bad.Formula
				case "source inputs":
					bad.InputSourceIDs = []string{"sql-rdd-pool-water"}
				}
				if err := epathSQLCheckPoolSurfaceQualification(changed, proof); err == nil {
					t.Fatalf("accepted source-transform mutation %s", mutation)
				}
			}
		}
	}
}

func TestEnergyPathRealSQLPoolSurfaceMandatoryBindingCannotLoseProof(t *testing.T) {
	observed, model, frames := epathSQLPoolSurfaceHand(t)
	proof, err := epathCompileSQLPoolSurfaceQualification(observed, model, frames)
	if err != nil {
		t.Fatal(err)
	}
	if err := epathSQLModelPoolSurfaceChecks(proof, &epathSQLModelChecks{}); err == nil {
		t.Fatal("semantic check replaced an absent generic numeric source check")
	}
	source := proof.Sources[proof.AuthorityID]
	target := epathRealOracleTarget{Collection: "sources", Field: "rawValue", SourceName: source.Name, SourceKey: source.Key, SourceUnit: "J", Frequency: "Monthly", Unit: "kWh"}
	quantity := epathSQLQuantity{Value: -5}
	checks := epathSQLModelChecks{}
	if err := checks.add("drivers", "building", "", "annual", "source/"+source.Name+"/"+source.Key+"/rawValue", "kWh", &quantity, target, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	before := checks.Rows[0]
	if err := epathSQLModelPoolSurfaceChecks(proof, &checks); err != nil {
		t.Fatal(err)
	}
	if len(checks.Rows) != 2 || !reflect.DeepEqual(before, checks.Rows[0]) || !reflect.DeepEqual(checks.Rows[1].Quantity, before.Quantity) || checks.Rows[1].Quantity == before.Quantity {
		t.Fatal("mandatory semantic binding changed or aliased the original quantity check")
	}
	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{Sources: epathSQLPoolSurfaceHandRecords(proof)}}
	if err := epathSQLCheckPoolSurfaceConsumer(bundle, checks.Rows[1]); err != nil {
		t.Fatal(err)
	}
	if err := epathSQLCheckPoolSurfaceConsumer(bundle, checks.Rows[0]); err != nil {
		t.Fatal("ordinary source row incorrectly required new semantic metadata")
	}
	for _, mutation := range []string{"missing proof", "unmarked proof", "wrong target", "pruned zero", "optional", "missing numeric", "wrong scope"} {
		changed := checks.Rows[1]
		switch mutation {
		case "missing proof":
			changed.PoolSurface = nil
		case "unmarked proof":
			changed.Item.Key = before.Item.Key
		case "wrong target":
			changed.Item.Target.SourceKey = "F2-1"
		case "pruned zero":
			changed.Item.Target.AllowPrunedZero = true
		case "optional":
			changed.OptionalPresentation = true
		case "missing numeric":
			changed.Quantity = nil
		case "wrong scope":
			changed.Item.Scope = "zone"
		}
		if err := epathSQLCheckPoolSurfaceConsumer(bundle, changed); err == nil {
			t.Fatalf("accepted mandatory semantic mutation %s", mutation)
		}
	}
	if err := epathSQLModelPoolSurfaceChecks(proof, &checks); err == nil {
		t.Fatal("duplicate Pool floor semantic check admitted")
	}
}
