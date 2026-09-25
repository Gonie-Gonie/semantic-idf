package simulation

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLCentralOriginalFixture(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/energy_path_real_models/models/25.1/CentralChillerHeaterSystem_Simultaneous_Cooling_Heating.idf")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "84d389f1c40ab098eea05105664dd71d54c58509a4d812bf6d5dfe4e933c28f0" {
		t.Fatal("literal Central original bytes changed")
	}
	return string(raw)
}

func TestEnergyPathSQLCentralOriginalFiveRecipientsAndExecutedPhysicalBinding(t *testing.T) {
	original := epathSQLCentralOriginalFixture(t)
	proof, err := epathSQLValidateCentralOriginal(original)
	if err != nil {
		t.Fatal(err)
	}
	if proof.System != 269 || proof.CoolingLoop != 274 || proof.HeatingLoop != 275 || proof.SourceLoop != 276 || proof.AirLoop != 178 || len(proof.Recipients) != 5 || len(proof.Objects) != 312 {
		t.Fatalf("wrong literal native roster: %#v", proof)
	}
	for n, r := range proof.Recipients {
		if r.ZoneName != fmt.Sprintf("SPACE%d-1", n+1) || r.Terminal != 149+n || r.ADU != 154+n || r.EquipmentList != 159+n || r.Connection != 164+n || r.HeatingCoil != 171+n {
			t.Fatalf("wrong literal recipient: %#v", r)
		}
	}
	executed := "Output:Variable,ChillerBank,Chiller Heater System Cooling Electricity Energy,Monthly;\n" + original
	objects, err := epathSQLBindCentralExecuted(original, executed, proof)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 312 || objects[0].ObjectIndex != proof.Objects[0].ObjectIndex+1 {
		t.Fatal("executed output insertion did not retain separate physical navigation indexes")
	}
	proof.Recipients[0].ZoneName = "PLENUM-1"
	if _, err := epathSQLBindCentralExecuted(original, executed, proof); err == nil {
		t.Fatal("caller-mutated original proof accepted")
	}
}

func TestEnergyPathSQLCentralOriginalRejectsBrokenNativeRoster(t *testing.T) {
	original := epathSQLCentralOriginalFixture(t)
	for _, test := range []struct {
		typ, name string
		field     int
	}{
		{"CentralHeatPumpSystem", "ChillerBank", 6},
		{"Branch", "Big Chiller Condenser Branch", 4},
		{"Connector:Splitter", "Reheat Splitter", 6},
		{"Branch", "Zone 5 Reheat Branch", 4},
		{"Fan:ConstantVolume", "Supply Fan 1", 8},
		{"Controller:WaterCoil", "Main Cooling Coil Controller", 4},
		{"Controller:WaterCoil", "Main Cooling Coil Controller", 5},
		{"AirLoopHVAC:SupplyPath", "TermReheatSupplyPath", 1},
		{"AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter", 6},
		{"AirLoopHVAC:ReturnPlenum", "Return-Plenum-1", 9},
		{"AirLoopHVAC:ReturnPath", "ReturnAirPath1", 1},
		{"ZoneHVAC:EquipmentList", "Zone5Equipment", 3},
		{"ZoneHVAC:EquipmentConnections", "SPACE5-1", 5},
		{"NodeList", "Zone5Inlets", 1},
	} {
		t.Run(fmt.Sprintf("%s/%s/%d", test.typ, test.name, test.field), func(t *testing.T) {
			mutated := epathSQLPoolMutatedOriginal(t, original, func(doc *idf.Document) {
				epathSQLPoolUnitObject(t, doc, test.typ, test.name).Fields[test.field].Value = "Disconnected original"
			})
			if _, err := epathSQLValidateCentralOriginal(mutated); err == nil {
				t.Fatal("disconnected original became five-recipient authority")
			}
		})
	}
}

func TestEnergyPathSQLCentralExecutedCannotEditUnrelatedPhysicalOrControlFields(t *testing.T) {
	original := epathSQLCentralOriginalFixture(t)
	proof, err := epathSQLValidateCentralOriginal(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		typ, name string
		field     int
		value     string
	}{
		{"ChillerHeaterPerformance:Electric:EIR", "ChillerHeaterModule", 2, "1.6"},
		{"Pump:VariableSpeed", "HW Circ Pump", 5, "2100"},
		{"CentralHeatPumpSystem", "ChillerBank", 8, "461"},
		{"Controller:WaterCoil", "Main Cooling Coil Controller", 6, "0.2"},
		{"Zone", "PLENUM-1", 6, "2"},
	} {
		t.Run(test.typ+"/"+test.name, func(t *testing.T) {
			executed := epathSQLPoolMutatedOriginal(t, original, func(doc *idf.Document) {
				epathSQLPoolUnitObject(t, doc, test.typ, test.name).Fields[test.field].Value = test.value
			})
			if _, err := epathSQLBindCentralExecuted(original, executed, proof); err == nil {
				t.Fatal("executed physical/control edit borrowed original ownership proof")
			}
		})
	}
}

func TestEnergyPathSQLCentralExternalFixtureCannotDropOriginalBinding(t *testing.T) {
	original := epathSQLCentralOriginalFixture(t)
	root := t.TempDir()
	originalPath, runPath := filepath.Join(root, "original.idf"), filepath.Join(root, "executed.idf")
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(originalPath, original)
	write(runPath, original)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(original)))
	evidence := epathRealRunEvidence{Fixture: epathRealFixture{ID: "simultaneous-25-1", Version: "25.1", ModelPath: "original.idf", ModelSHA256: digest}, CatalogDirectory: root, RunDirectory: root, OriginalInputPath: originalPath, InputPath: runPath, ModelSHA256: digest, ExecutedSHA256: digest}
	// Applicability is external fixture identity, not a removable optional
	// recipient selector. Empty model declarations cannot bypass this binding.
	recipe := epathRealOracleRecipe{SQLModel: &epathRealSQLModel{}}
	var observed epathRealOracleEvidence
	if err := epathBindRealSQLVRFOriginal(evidence, recipe, &observed); err != nil {
		t.Fatal(err)
	}
	if observed.originalText != original || observed.executedText != original {
		t.Fatal("external finite fixture was not bound")
	}
	mutated := epathSQLPoolMutatedOriginal(t, original, func(doc *idf.Document) {
		epathSQLPoolUnitObject(t, doc, "Controller:WaterCoil", "Main Cooling Coil Controller").Fields[4].Value = "Broken sensor"
	})
	write(runPath, mutated)
	evidence.ExecutedSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(mutated)))
	var rejected epathRealOracleEvidence
	if err := epathBindRealSQLVRFOriginal(evidence, recipe, &rejected); err == nil || rejected.originalText != "" || rejected.executedText != "" {
		t.Fatal("provenance-consistent executed edit escaped independent physical binding")
	}
}
