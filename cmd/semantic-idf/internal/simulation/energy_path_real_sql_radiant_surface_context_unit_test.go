package simulation

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func epathSQLRadiantSurfaceContextUnitFixture(t *testing.T) (string, epathRealSQLModel, string) {
	t.Helper()
	path, model := epathSQLModelUnitFixture(t)
	original, binding := epathSQLRadiantHandFixture()
	epathOracleEditSQL(t, path, `UPDATE Zones SET Multiplier=7,ListMultiplier=3;
UPDATE ReportDataDictionary SET Name='Zone Radiant HVAC Cooling Energy',KeyValue='Radiant' WHERE ReportDataDictionaryIndex=10;
UPDATE ReportDataDictionary SET Name='Zone Radiant HVAC Heating Energy',KeyValue='Radiant' WHERE ReportDataDictionaryIndex=11;
UPDATE Surfaces SET SurfaceName='Passive Floor',ClassName='Floor',ExtBoundCond=-1 WHERE SurfaceIndex=1;
UPDATE ReportDataDictionary SET Name='Surface Inside Face Convection Heat Gain Energy',KeyValue='Passive Floor' WHERE ReportDataDictionaryIndex=12;
INSERT INTO Surfaces VALUES(3,'Floor','Floor',1,-1,1);
INSERT INTO ReportDataDictionary VALUES(15,'Surface Inside Face Convection Heat Gain Energy','Floor',0,'Monthly','J','Surface');
INSERT INTO ReportDataDictionary VALUES(16,'Zone Air Heat Balance Surface Convection Rate','Office',0,'Monthly','W','Zone');
INSERT INTO ReportData SELECT 15000+TimeIndex,15,TimeIndex,CASE WHEN Month=2 THEN 0 ELSE -500*3600000 END FROM "Time" WHERE TimeIndex BETWEEN 1 AND 12;
INSERT INTO ReportData SELECT 16000+TimeIndex,16,TimeIndex,CASE WHEN Month=2 THEN 0 ELSE 200*1000/("Interval"/60) END FROM "Time" WHERE TimeIndex BETWEEN 1 AND 12;`)
	model.Surface.RadiantContext = &epathRealSQLRadiantSurfaceContext{Policy: "original_direct_surface_context/v1"}
	model.Surface.Source.Keys = []string{"Passive Floor", "Wall B", "Floor"}
	model.Surface.Source.Alternatives = append([]epathRealSQLAlternative{{Name: "Surface Inside Face Convection Heat Gain Energy", Unit: "J"}}, model.Surface.Source.Alternatives...)
	model.Families = append(model.Families, epathRealSQLFamily{ID: "surface.context", Keys: []string{"Office"}, Category: "balance.storage_other", Component: "sensible", Role: "context", Terms: []epathRealSQLTerm{{Sign: 1, Source: epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Zone Air Heat Balance Surface Convection Rate", Unit: "W"}}}}}})
	for i := range model.Loads {
		name := "Zone Radiant HVAC Cooling Energy"
		if model.Loads[i].Service == "heating" {
			name = "Zone Radiant HVAC Heating Energy"
		}
		copy := binding
		copy.Owners = append([]epathRealSQLRadiantOwner(nil), binding.Owners...)
		model.Loads[i].Component, model.Loads[i].NativeRadiant = "combined", &copy
		model.Loads[i].Source = epathRealSQLSelector{Keys: []string{"Radiant"}, Alternatives: []epathRealSQLAlternative{{Name: name, Unit: "J"}}}
	}
	return path, model, original
}

func epathSQLRadiantSurfaceContextUnitFrames(t *testing.T, path string, model epathRealSQLModel, original string) epathSQLFrames {
	t.Helper()
	observed, err := epathReadRealSQLOracle(path)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := epathCompileSQLModelFrames(path, observed.Sources, model, original)
	if err != nil {
		t.Fatal(err)
	}
	return frames
}

func TestEnergyPathRealSQLRadiantSurfaceContextPreservesRawAndPassivePeer(t *testing.T) {
	path, model, original := epathSQLRadiantSurfaceContextUnitFixture(t)
	frames := epathSQLRadiantSurfaceContextUnitFrames(t, path, model, original)
	if len(frames.RadiantSurfaceContextIdentities) != 2 {
		t.Fatalf("context roster=%d, want one active surface and one Zone aggregate", len(frames.RadiantSurfaceContextIdentities))
	}
	for month := 1; month <= 12; month++ {
		passive := frames.Cells[epathSQLKey("Office", "surface:surface.ground_floors", month)]
		if passive == nil || math.Abs(passive.Raw.Value-10) > 1e-12 || math.Abs(passive.Effective.Value-210) > 1e-12 || !reflect.DeepEqual(passive.SourceIDs, []int{12}) || math.Abs(passive.Allocated["cooling"].Value-30.0/11) > 1e-12 {
			t.Fatalf("M%d active floor changed the same-category passive pressure/weight: %#v", month, passive)
		}
		if frames.Cells[epathSQLKey("Office", "surface.context", month)] != nil || frames.Cells[epathSQLKey("Office", "surface.balance", month)] != nil {
			t.Fatal("active surface aggregate became a physical or synthetic storage cell")
		}
		rawSurface, rawAggregate := -500.0, 200.0
		if month == 2 {
			rawSurface, rawAggregate = 0, 0
		}
		for id, want := range map[int]float64{15: rawSurface, 16: rawAggregate} {
			proof := frames.RadiantSurfaceContextIdentities[id]
			if math.Abs(proof.Raw[month-1].Value-want) > 1e-10 || math.Abs(proof.Effective[month-1].Value-want*21) > 1e-10 || frames.SourceZone[id] != "office" {
				t.Fatalf("M%d raw context source %d lost value/sign/owner or factor7*3: %#v", month, id, proof)
			}
			if month == 2 && (proof.Raw[1].Error != 0 || proof.Effective[1].Error != 0 || proof.Source.Months[1].Rows != 1) {
				t.Fatal("observed zero became unknown or acquired a fabricated pressure allowance")
			}
		}
		if math.Abs(frames.Loads[epathSQLKey("Office", "cooling", month)].Value-3) > 1e-12 {
			t.Fatal("surface context changed the already-model-total native load")
		}
	}
	for _, cell := range frames.Cells {
		cell.SourceIDs = append(cell.SourceIDs, 15)
		break
	}
	if epathSQLValidateRadiantSurfaceContextFrames(frames) == nil {
		t.Fatal("active source was permitted to enter a pressure cell after compilation")
	}
}

func TestEnergyPathRealSQLRadiantSurfaceContextRejectsMissingAndReusedEvidence(t *testing.T) {
	for _, mutation := range []string{"missing policy", "unknown policy", "missing original", "foreign H owner", "missing active selector", "SQL boundary", "missing month", "null month", "duplicate month", "wrong unit", "aggregate omitted", "aggregate pressure", "aggregate subtraction", "indirect subtraction"} {
		t.Run(mutation, func(t *testing.T) {
			path, model, original := epathSQLRadiantSurfaceContextUnitFixture(t)
			switch mutation {
			case "missing policy":
				model.Surface.RadiantContext = nil
			case "unknown policy":
				model.Surface.RadiantContext.Policy = "skip_ground_floor"
			case "missing original":
				original = ""
			case "foreign H owner":
				model.Loads[1].NativeRadiant.Owners[0].SurfaceName = "Other Floor"
			case "missing active selector":
				model.Surface.Source.Keys = model.Surface.Source.Keys[:2]
			case "SQL boundary":
				epathOracleEditSQL(t, path, `UPDATE Surfaces SET ExtBoundCond=0 WHERE SurfaceIndex=3`)
			case "missing month":
				epathOracleEditSQL(t, path, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=15 AND TimeIndex=2`)
			case "null month":
				epathOracleEditSQL(t, path, `UPDATE ReportData SET Value=NULL WHERE ReportDataDictionaryIndex=15 AND TimeIndex=2`)
			case "duplicate month":
				epathOracleEditSQL(t, path, `INSERT INTO ReportData VALUES(99999,15,2,0)`)
			case "wrong unit":
				epathOracleEditSQL(t, path, `UPDATE ReportDataDictionary SET Units='m3' WHERE ReportDataDictionaryIndex=15`)
			case "aggregate omitted":
				model.Families = model.Families[:1]
			case "aggregate pressure":
				model.Families[1].Role = "pressure"
			case "aggregate subtraction":
				model.Families[1].Subtract = []string{"surface.total"}
			case "indirect subtraction":
				model.Families = append(model.Families, epathRealSQLFamily{ID: "pretend.storage", Keys: []string{"Office"}, Category: "balance.storage_other", Component: "sensible", Role: "pressure", Subtract: []string{"surface.context"}})
			}
			observed, err := epathReadRealSQLOracle(path)
			if err != nil {
				if mutation == "duplicate month" && strings.Contains(err.Error(), "duplicate ReportData observation dictionary=15 TimeIndex=2") {
					return // The original SQL reader rejects this exact duplicate before frame compilation.
				}
				t.Fatal(err)
			}
			if _, err := epathCompileSQLModelFrames(path, observed.Sources, model, original); err == nil {
				t.Fatal("unbound/unknown/reused active surface context was accepted")
			}
		})
	}
}

func TestEnergyPathRealSQLRadiantSurfaceContextAbsentAggregateAndLegacyNoop(t *testing.T) {
	path, model, original := epathSQLRadiantSurfaceContextUnitFixture(t)
	epathOracleEditSQL(t, path, `DELETE FROM ReportData WHERE ReportDataDictionaryIndex=16; DELETE FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=16`)
	model.Families = model.Families[:1]
	frames := epathSQLRadiantSurfaceContextUnitFrames(t, path, model, original)
	if len(frames.RadiantSurfaceContextIdentities) != 1 || frames.SourceRaw[16] != nil {
		t.Fatal("unobserved aggregate was invented as a zero-valued source")
	}
	legacyPath, legacy := epathSQLModelUnitFixture(t)
	observed, err := epathReadRealSQLOracle(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	one, err := epathCompileSQLModelFrames(legacyPath, observed.Sources, legacy)
	if err != nil {
		t.Fatal(err)
	}
	two, err := epathCompileSQLModelFrames(legacyPath, observed.Sources, legacy, original)
	if err != nil || !reflect.DeepEqual(one, two) || one.RadiantSurfaceContextIdentities != nil {
		t.Fatalf("no declaration changed existing source/frame values or metadata: %v", err)
	}
}

func epathSQLRadiantSurfaceContextUnitCandidate(proof epathSQLRadiantSurfaceContextIdentity) EnergyDataSource {
	source := EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", proof.Source.DictionaryIndex), SourceType: "sql_report_data", Name: proof.Source.Name, KeyValue: proof.Source.KeyValue, Units: proof.Source.SourceUnit, SourceUnit: proof.Source.SourceUnit, NormalizedUnit: "kWh", ReportingFrequency: "Monthly", ZoneName: proof.Owner.Owner.ZoneName, DriverRole: "context", DriverCategory: proof.Category, DriverComponent: "surface.sensible", InspectorSection: "Context", AggregationMethod: "sum_report_data", MultiplierApplication: "requires_zone_multiplier", EffectiveMultiplier: 21, RelatedEntityIDs: []string{fmt.Sprintf("surface-%d", proof.Owner.SurfaceIndex), "surface:buildingsurface%3adetailed:floor"}}
	if proof.Kind == "zone_aggregate" {
		source.DriverComponent = "surface.reconciliation"
	}
	if proof.Source.SourceUnit == "W" {
		source.AggregationMethod = "integrate_rate_by_time_interval"
	}
	return source
}

func TestEnergyPathRealSQLRadiantSurfaceContextStableOriginalSurfaceSurvivesIndexShift(t *testing.T) {
	for name, want := range map[string]string{
		"Floor":          "surface:buildingsurface%3adetailed:floor",
		" Zn001:Flr001 ": "surface:buildingsurface%3adetailed:zn001%3aflr001",
		"A/B % C.D_E-F":  "surface:buildingsurface%3adetailed:a%2fb%20%25%20c.d_e-f",
	} {
		if got := epathSQLRadiantStableSurfaceID(name); got != want {
			t.Fatalf("original typed surface identity=%q, want literal %q", got, want)
		}
	}
	path, model, original := epathSQLRadiantSurfaceContextUnitFixture(t)
	frames := epathSQLRadiantSurfaceContextUnitFrames(t, path, model, original)
	proof := frames.RadiantSurfaceContextIdentities[15]
	if proof.Owner.SurfaceIndex <= 0 {
		t.Fatal("hand original surface must have an earlier object to remove")
	}
	// Removing one earlier object when annualizing changes only the runtime
	// geometry alias. Never change the independently bound original object
	// index, source key, owner, multiplier or monthly quantities to match it.
	source := epathSQLRadiantSurfaceContextUnitCandidate(proof)
	source.RelatedEntityIDs = []string{fmt.Sprintf("surface-%d", proof.Owner.SurfaceIndex-1), "surface:buildingsurface%3adetailed:floor"}
	if !epathSQLRadiantSurfaceContextSourceMatches(source, proof) {
		t.Fatal("annualization index shift invalidated the same original typed surface")
	}
	source.RelatedEntityIDs = []string{"surface:buildingsurface%3adetailed:floor"}
	if !epathSQLRadiantSurfaceContextSourceMatches(source, proof) {
		t.Fatal("transient object-index alias became original ownership authority")
	}
	for name, related := range map[string][]string{
		"index only":         {fmt.Sprintf("surface-%d", proof.Owner.SurfaceIndex)},
		"foreign type":       {"surface:wall%3adetailed:floor"},
		"foreign name":       {"surface:buildingsurface%3adetailed:other%20floor"},
		"duplicate stable":   {"surface:buildingsurface%3adetailed:floor", "surface:buildingsurface%3adetailed:floor"},
		"ambiguous surfaces": {"surface:buildingsurface%3adetailed:floor", "surface:buildingsurface%3adetailed:other%20floor"},
	} {
		t.Run(name, func(t *testing.T) {
			mutant := source
			mutant.RelatedEntityIDs = related
			if epathSQLRadiantSurfaceContextSourceMatches(mutant, proof) {
				t.Fatal("transient, foreign or ambiguous surface identity replaced the original source binding")
			}
		})
	}
}

func TestEnergyPathRealSQLRadiantSurfaceContextMetadataCannotMasquerade(t *testing.T) {
	path, model, original := epathSQLRadiantSurfaceContextUnitFixture(t)
	frames := epathSQLRadiantSurfaceContextUnitFrames(t, path, model, original)
	for _, id := range []int{15, 16} {
		proof := frames.RadiantSurfaceContextIdentities[id]
		source := epathSQLRadiantSurfaceContextUnitCandidate(proof)
		if !epathSQLRadiantSurfaceContextSourceMatches(source, proof) {
			t.Fatalf("literal independent valid metadata rejected: %#v", source)
		}
		for name, mutate := range map[string]func(*EnergyDataSource){
			"main flow":        func(s *EnergyDataSource) { s.DriverRole = "main_flow" },
			"other owner":      func(s *EnergyDataSource) { s.ZoneName = "Other" },
			"other surface":    func(s *EnergyDataSource) { s.KeyValue = "Passive Floor" },
			"wrong unit":       func(s *EnergyDataSource) { s.SourceUnit = "kWh" },
			"wrong frequency":  func(s *EnergyDataSource) { s.ReportingFrequency = "Hourly" },
			"wrong multiplier": func(s *EnergyDataSource) { s.EffectiveMultiplier = 1 },
			"wrapper":          func(s *EnergyDataSource) { s.InputSourceIDs = []string{"original"} },
			"derived formula":  func(s *EnergyDataSource) { s.Formula = "sum(original)" },
			"allocation":       func(s *EnergyDataSource) { s.AllocationApplied = true },
		} {
			t.Run(fmt.Sprintf("%d/%s", id, name), func(t *testing.T) {
				mutant := source
				mutate(&mutant)
				if epathSQLRadiantSurfaceContextSourceMatches(mutant, proof) {
					t.Fatal("mutant context source passed original metadata proof")
				}
			})
		}
		if id == 15 {
			source.RelatedEntityIDs = []string{"surface-999"}
			if epathSQLRadiantSurfaceContextSourceMatches(source, proof) {
				t.Fatal("active source borrowed another surface topology index")
			}
		}
	}
}
