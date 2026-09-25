package simulation

import (
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathSQLHeatOnlyOriginalDetachesRecipientDeclaration(t *testing.T) {
	text, declaration := epathSQLHeatOnlyOriginalFixture(t)
	proof, err := epathSQLValidateHeatOnlyFurnaceOriginal(text, declaration)
	if err != nil {
		t.Fatal(err)
	}
	before := proof.Declaration.ServedZones[0]
	declaration.ServedZones[0] = "forged caller recipient"
	if proof.Declaration.ServedZones[0] != before {
		t.Fatal("original HeatOnly proof aliases the caller's recipient declaration")
	}
}

func TestEnergyPathSQLHeatOnlyOriginalRejectsExtraPhysicalDeliveryOrSource(t *testing.T) {
	text, declaration := epathSQLHeatOnlyOriginalFixture(t)
	// Even disconnected extra HVAC objects invalidate this finite original
	// census. A SingleCooling thermostat is still allowed as control context.
	for _, kind := range []string{
		"ZoneHVAC:IdealLoadsAirSystem",
		"ZoneHVAC:UnitHeater",
		"AirTerminal:SingleDuct:VAV:NoReheat",
		"AirLoopHVAC:UnitarySystem",
		"CoilSystem:Cooling:DX",
		"AirConditioner:VariableRefrigerantFlow",
		"HeatPump:PlantLoop:EIR:Cooling",
		"EvaporativeCooler:Direct:CelDekPad",
	} {
		t.Run(kind, func(t *testing.T) {
			doc, err := idf.Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			doc.Objects = append(doc.Objects, idf.Object{Type: kind, Index: len(doc.Objects), Fields: []idf.Field{{Value: "unreviewed physical owner"}}})
			if _, err := epathSQLValidateHeatOnlyFurnaceOriginal(doc.String(), declaration); err == nil {
				t.Fatal("finite no-cooling proof accepted an extra physical delivery/source")
			}
		})
	}
}
