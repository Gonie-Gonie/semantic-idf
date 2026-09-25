package simulation

import (
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathSQLHeatOnlyContextRequestScopeKeepsActualOpenerAndDefaults(t *testing.T) {
	text, declaration := epathSQLHeatOnlyOriginalFixture(t)
	proof, err := epathSQLValidateHeatOnlyFurnaceOriginal(text, declaration)
	if err != nil {
		t.Fatal(err)
	}
	specs, err := epathSQLHeatOnlyContextSpecs(&epathSQLHeatOnlyBinding{Inputs: epathSQLHeatOnlyInputs{Furnaces: []epathRealSQLHeatOnlyFurnace{declaration}}, Original: proof, Executed: proof})
	if err != nil {
		t.Fatal(err)
	}
	parse := func(text string) idf.Document {
		doc, err := idf.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	for _, s := range specs {
		if s.Zone == "" {
			continue
		}
		spec := epathSQLPVNativeSpec{Name: s.Name, Key: s.Key}
		line := "\nOutput:Variable," + s.Key + "," + s.Name + "," + s.Frequency + ";\n"
		request := PurposeOutputObject{ObjectType: "Output:Variable", VariableName: s.Name, KeyValue: s.Key, ScopeZoneName: s.Zone, ReportingFrequency: s.Frequency, State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Value: s.Key}, {Value: s.Name}, {Value: s.Frequency}}}
		plan := PurposeRunPlan{BasicEnergyDetail: "energy_path", OutputObjects: []PurposeOutputObject{request}}
		original, run := parse(text), parse(text+line)
		if opener, err := epathSQLPVRequestBinding(original, run, &plan, spec, s.Frequency, s.Zone); err != nil || opener != nil {
			t.Fatalf("actual temporary Zone request lost binding: %v %v", opener, err)
		}
		if _, err := epathSQLPVRequestBinding(original, run, &plan, spec, s.Frequency); err == nil {
			t.Fatal("default PV request guard accepted scoped metadata")
		}
		// Original raw opener is authoritative even when execution inserts earlier objects.
		old := parse(text + line)
		index := old.Objects[len(old.Objects)-1].Index
		plan.OutputObjects[0].State, plan.OutputObjects[0].ObjectIndex = "existing", &index
		shifted := parse("Output:Variable,*,Site Outdoor Air Drybulb Temperature,Hourly;\n" + text + line)
		if opener, err := epathSQLPVRequestBinding(old, shifted, &plan, spec, s.Frequency, s.Zone); err != nil || opener == nil || *opener != index {
			t.Fatalf("exact original opener lost: %v %v", opener, err)
		}
		wrong := index + 1
		plan.OutputObjects[0].ObjectIndex = &wrong
		if _, err := epathSQLPVRequestBinding(old, shifted, &plan, spec, s.Frequency, s.Zone); err == nil {
			t.Fatal("shifted executed opener impersonated original")
		}
		// Actual unscoped wildcard requests remain valid without inventing a Zone opener.
		wild := strings.Replace(line, "Output:Variable,"+s.Key+",", "Output:Variable,*,", 1)
		request.ScopeZoneName, request.KeyValue, request.Fields[0].Value = "", "*", "*"
		plan.OutputObjects[0] = request
		if opener, err := epathSQLPVRequestBinding(original, parse(text+wild), &plan, spec, s.Frequency, s.Zone); err != nil || opener != nil {
			t.Fatalf("actual unscoped wildcard request rejected: %v %v", opener, err)
		}
	}
}

func TestEnergyPathSQLHeatOnlyContextRequestScopeRejectsBorrowedMetadata(t *testing.T) {
	spec := epathSQLPVNativeSpec{Name: "Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate", Key: "WEST ZONE"}
	line := "Output:Variable,WEST ZONE," + spec.Name + ",Monthly;"
	parse := func(text string) idf.Document {
		doc, err := idf.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	for _, mutation := range []string{"wrong-zone", "wrong-metadata-key", "scoped-wildcard", "borrowed-executed-wildcard", "scheduled", "wrong-purpose", "temporary-opener", "missing-executed", "wrong-native-key", "ambiguous-scope"} {
		t.Run(mutation, func(t *testing.T) {
			request := PurposeOutputObject{ObjectType: "Output:Variable", VariableName: spec.Name, KeyValue: spec.Key, ScopeZoneName: spec.Key, ReportingFrequency: "Monthly", State: "temporary", PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Fields: []idf.OutputFieldValue{{Value: spec.Key}, {Value: spec.Name}, {Value: "Monthly"}}}
			runText, zone := line, spec.Key
			scopes := []string{zone}
			switch mutation {
			case "wrong-zone":
				request.ScopeZoneName = "EAST ZONE"
			case "wrong-metadata-key":
				request.KeyValue = "EAST ZONE"
			case "scoped-wildcard":
				request.KeyValue, request.Fields[0].Value = "*", "*"
				runText = strings.Replace(line, "WEST ZONE", "*", 1)
			case "borrowed-executed-wildcard":
				runText = strings.Replace(line, "WEST ZONE", "*", 1)
			case "scheduled":
				request.Fields = append(request.Fields, idf.OutputFieldValue{Value: "FILTER"})
				runText = strings.Replace(line, "Monthly;", "Monthly,FILTER;", 1)
			case "wrong-purpose":
				request.PurposeIDs = nil
			case "temporary-opener":
				request.ObjectIndex = new(int)
			case "missing-executed":
				runText = "Version,25.1;"
			case "wrong-native-key":
				scopes[0] = "EAST ZONE"
			case "ambiguous-scope":
				scopes = append(scopes, zone)
			}
			plan := PurposeRunPlan{BasicEnergyDetail: "energy_path", OutputObjects: []PurposeOutputObject{request}}
			if _, err := epathSQLPVRequestBinding(parse("Version,25.1;"), parse(runText), &plan, spec, "Monthly", scopes...); err == nil {
				t.Fatalf("borrowed request metadata accepted: %s", mutation)
			}
		})
	}
}
