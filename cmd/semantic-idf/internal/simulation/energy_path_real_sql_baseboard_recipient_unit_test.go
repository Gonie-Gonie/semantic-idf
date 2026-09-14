package simulation

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLRecipientUnitSource(index int, name, key, unit, frequency string) epathRealSQLSource {
	return epathRealSQLSource{DictionaryIndex: index, Name: name, KeyValue: key, SourceUnit: unit, ReportingFrequency: frequency}
}

func epathSQLRecipientUnitCandidate(proof *epathSQLBaseboardRecipientQualification) []EnergyDataSource {
	var out []EnergyDataSource
	for index, observed := range proof.Sources {
		source := EnergyDataSource{ID: fmt.Sprintf("sql-rdd-%d", index), SourceType: "sql_report_data", Name: observed.Name, KeyValue: observed.Key, Units: observed.Unit, SourceUnit: observed.Unit, ReportingFrequency: observed.Frequency, RawValue: -1.25, EffectiveValue: -8.75}
		if len(observed.References) > 0 {
			_, aggregate := epathSQLBaseboardRecipientOutput(observed.Name)
			source.Explanation = "Not isolated passive conduction or a measured baseboard-only gain. Declared fractions are not operation; no heat is added or subtracted."
			if aggregate {
				source.Explanation = "Includes environmental and HVAC responses; not an isolated passive or baseboard-only contribution."
			}
			parents := map[string]bool{}
			for _, ref := range observed.References {
				source.ZoneName = ref.ZoneName
				source.Explanation += fmt.Sprintf(" Non-additive recipient configuration: %s %q [component:%d, object %d] -> surface %q [object %d], Zone %q; radiant fraction=%g; recipient share of radiant output=%g.", epathSQLNativeBaseboardType, ref.ParentName, ref.ParentIndex, ref.ParentIndex, ref.SurfaceName, ref.SurfaceIndex, ref.ZoneName, ref.RadiantFraction, ref.RecipientFraction)
				id := fmt.Sprintf("component:%d", ref.ParentIndex)
				if !parents[id] {
					source.RelatedEntityIDs = append(source.RelatedEntityIDs, id)
					parents[id] = true
				}
			}
		}
		out = append(out, source)
	}
	return out
}

func TestEnergyPathRealSQLBaseboardRecipientOriginalRosterAndExecutedIndices(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "energy_path_real_models", "models", "25.1", "5ZoneElectricBaseboard.idf"))
	if err != nil {
		t.Fatal(err)
	}
	original, err := idf.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	// A run-copy's indexes are deliberately different without changing any
	// physical object. Never assume the original parent index is a UI opener.
	executed, err := idf.Parse("Timestep,4;\n" + string(data))
	if err != nil {
		t.Fatal(err)
	}
	declarations := []epathRealSQLBaseboardContext{{OwnerName: "SPACE2-1 BASEBOARD", ZoneName: "SPACE2-1"}, {OwnerName: "SPACE4-1 BASEBOARD", ZoneName: "SPACE4-1"}}
	var observed []epathRealSQLSource
	for _, object := range original.Objects {
		if object.Type != "BuildingSurface:Detailed" && object.Type != "FenestrationSurface:Detailed" {
			continue
		}
		for _, frequency := range []string{"Monthly", "Hourly"} {
			for _, pair := range [][2]string{{"Surface Inside Face Convection Heat Gain Energy", "J"}, {"Surface Inside Face Convection Heat Gain Rate", "W"}} {
				observed = append(observed, epathSQLRecipientUnitSource(len(observed)+1, pair[0], epathSQLBaseboardField(object, 0), pair[1], frequency))
			}
		}
	}
	for _, object := range original.Objects {
		if object.Type != "Zone" {
			continue
		}
		for _, frequency := range []string{"Monthly", "Hourly"} {
			observed = append(observed, epathSQLRecipientUnitSource(len(observed)+1, "Zone Air Heat Balance Surface Convection Rate", epathSQLBaseboardField(object, 0), "W", frequency))
		}
	}
	proof, err := epathSQLCompileBaseboardRecipientQualification(original, executed, observed, declarations)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"RIGHT-1": .3, "C2-1": .1, "SB25": .1, "SB23": .1, "SB21": .1, "LEFT-1": .3, "C4-1": .1, "SB45": .1, "SB41": .1, "SB43": .1}
	seen, surfaces, aggregates := map[string]bool{}, 0, 0
	for _, source := range proof.Sources {
		_, aggregate := epathSQLBaseboardRecipientOutput(source.Name)
		if len(source.References) == 0 {
			continue
		}
		if aggregate {
			aggregates++
			continue
		}
		surfaces++
		if len(source.References) != 1 {
			t.Fatal("surface has ambiguous original parent")
		}
		ref := source.References[0]
		parent, _ := epathSQLBaseboardOne(original, epathSQLNativeBaseboardType, ref.ParentName)
		surface, _ := epathSQLBaseboardOne(original, "BuildingSurface:Detailed", ref.SurfaceName)
		if want[ref.SurfaceName] != ref.RecipientFraction || ref.RadiantFraction != .2 || ref.ParentIndex != parent.Index+1 || ref.SurfaceIndex != surface.Index+1 {
			t.Fatalf("original recipient/index mismatch: %+v", ref)
		}
		seen[ref.SurfaceName] = true
	}
	if len(proof.Sources) != 196 || len(seen) != 10 || surfaces != 40 || aggregates != 4 {
		t.Fatalf("recipient census changed: total=%d physical=%d surface=%d aggregate=%d", len(proof.Sources), len(seen), surfaces, aggregates)
	}
	sources := epathSQLRecipientUnitCandidate(proof)
	before := fmt.Sprintf("%#v", sources)
	if err := epathSQLCheckBaseboardRecipientQualification(sources, proof); err != nil {
		t.Fatal(err)
	}
	if before != fmt.Sprintf("%#v", sources) {
		t.Fatal("metadata validation mutated candidate values")
	}
	changed, err := idf.Parse(executed.String())
	if err != nil {
		t.Fatal(err)
	}
	for i := range changed.Objects {
		if changed.Objects[i].Type == "BuildingSurface:Detailed" && epathSQLBaseboardField(changed.Objects[i], 0) == "RIGHT-1" {
			changed.Objects[i].Fields[0].Value = "WRONG-RIGHT"
		}
	}
	if _, err := epathSQLCompileBaseboardRecipientQualification(original, changed, observed, declarations); err == nil {
		t.Fatal("changed executed surface escaped original binding")
	}
}

func TestEnergyPathRealSQLBaseboardRecipientMetadataMutationsAndConsumedGate(t *testing.T) {
	path, model, plan, observed := epathSQLBaseboardContextUnit(t, "Baseboard Total Heating Energy", "Monthly", func(int) float64 { return 1 })
	observed = append(observed, epathSQLRecipientUnitSource(51, "Surface Inside Face Convection Heat Gain Energy", "Floor", "J", "Monthly"), epathSQLRecipientUnitSource(52, "Surface Inside Face Convection Heat Gain Energy", "Other Floor", "J", "Monthly"), epathSQLRecipientUnitSource(53, "Zone Air Heat Balance Surface Convection Rate", "Office", "W", "Hourly"))
	frames := epathSQLBaseboardContextUnitFrames()
	beforeRaw, beforeEffective, beforeCells, beforeLoads := fmt.Sprintf("%#v", frames.SourceRaw), fmt.Sprintf("%#v", frames.SourceEffective), fmt.Sprintf("%#v", frames.Cells), fmt.Sprintf("%#v", frames.Loads)
	if err := epathCompileSQLBaseboardContexts(path, energyPathBaseboardHand(t).String(), &plan, observed, model, &frames); err != nil {
		t.Fatal(err)
	}
	identity := frames.BaseboardContextIdentities[40]
	checks := epathSQLModelChecks{}
	if err := epathSQLModelBaseboardContextSourceChecks(frames, &checks); err != nil {
		t.Fatal(err)
	}
	makeBundle := func() PurposeResultBundle {
		bundle := epathSQLBaseboardContextUnitBundle(identity)
		bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources, epathSQLRecipientUnitCandidate(identity.RecipientQualification)...)
		return bundle
	}
	for _, check := range checks.Rows {
		if err := epathCheckSQLBaseboardContextSource(makeBundle(), check); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*EnergyDataSource)
	}{
		{"lost recipient", func(s *EnergyDataSource) { s.Explanation = "" }},
		{"wrong parent", func(s *EnergyDataSource) {
			s.Explanation = strings.ReplaceAll(s.Explanation, `"Baseboard"`, `"Foreign Baseboard"`)
		}},
		{"wrong component index", func(s *EnergyDataSource) {
			s.Explanation = strings.ReplaceAll(s.Explanation, "component:", "component:9")
		}},
		{"wrong surface", func(s *EnergyDataSource) {
			s.Explanation = strings.ReplaceAll(s.Explanation, `surface "Floor"`, `surface "Other Floor"`)
		}},
		{"wrong fraction", func(s *EnergyDataSource) {
			s.Explanation = strings.ReplaceAll(s.Explanation, "output=0.7", "output=0.6")
		}},
		{"missing parent ID", func(s *EnergyDataSource) { s.RelatedEntityIDs = nil }},
		{"passive-only claim", func(s *EnergyDataSource) {
			s.Explanation = strings.ReplaceAll(s.Explanation, "Not isolated passive", "Is isolated passive")
		}},
		{"wrong source frequency", func(s *EnergyDataSource) { s.ReportingFrequency = "Hourly" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle := makeBundle()
			for i := range bundle.EnergyExplanation.Sources {
				if bundle.EnergyExplanation.Sources[i].ID == "sql-rdd-51" {
					tc.mutate(&bundle.EnergyExplanation.Sources[i])
				}
			}
			if err := epathCheckSQLBaseboardContextSource(bundle, checks.Rows[0]); err == nil {
				t.Fatal("recipient mutation passed the consumed source proof gate")
			}
		})
	}
	bundle := makeBundle()
	var parentTrace EnergyDataSource
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID == "sql-rdd-51" {
			parentTrace = source
		}
	}
	for i := range bundle.EnergyExplanation.Sources {
		if bundle.EnergyExplanation.Sources[i].ID == "sql-rdd-52" {
			bundle.EnergyExplanation.Sources[i].Explanation = parentTrace.Explanation
			bundle.EnergyExplanation.Sources[i].RelatedEntityIDs = append([]string(nil), parentTrace.RelatedEntityIDs...)
		}
	}
	if err := epathCheckSQLBaseboardContextSource(bundle, checks.Rows[0]); err == nil {
		t.Fatal("nonrecipient mislabeled as a baseboard recipient")
	}
	bundle = makeBundle()
	for i, source := range bundle.EnergyExplanation.Sources {
		if source.ID == "sql-rdd-53" {
			bundle.EnergyExplanation.Sources = append(bundle.EnergyExplanation.Sources[:i], bundle.EnergyExplanation.Sources[i+1:]...)
			break
		}
	}
	if err := epathCheckSQLBaseboardContextSource(bundle, checks.Rows[0]); err == nil {
		t.Fatal("omitted native aggregate metadata passed")
	}
	if !reflect.DeepEqual([]string{beforeRaw, beforeEffective, beforeCells, beforeLoads}, []string{fmt.Sprintf("%#v", frames.SourceRaw), fmt.Sprintf("%#v", frames.SourceEffective), fmt.Sprintf("%#v", frames.Cells), fmt.Sprintf("%#v", frames.Loads)}) {
		t.Fatal("recipient metadata acquired numeric frames")
	}
}
