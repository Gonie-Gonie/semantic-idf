package simulation

import (
	"reflect"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func TestEnergyPathCogenerationPurposeAddOnlyIndependentParent(t *testing.T) {
	for _, original := range []string{"", "Output:Meter,Cogeneration:Electricity,Timestep;", "Output:Meter,Cogeneration:*,Monthly; Output:Meter,Cogeneration:Electricity,Hourly;"} {
		t.Run(original, func(t *testing.T) {
			doc, err := idf.Parse("Version,25.1; ElectricLoadCenter:Inverter:LookUpTable,INV; " + original)
			if err != nil {
				t.Fatal(err)
			}
			before := doc.String()
			builder := newPurposePlanBuilder(doc, NormalizeSimulationPurposeRequest(&SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}))
			builder.addEnergyPathCogenerationOutputs()
			plan := builder.plan()
			if len(plan.OutputObjects) != 2 {
				t.Fatalf("expected two M/H intent requests, got %+v", plan.OutputObjects)
			}
			for _, output := range plan.OutputObjects {
				if output.ObjectType != "Output:Meter" || output.ReportingFrequency != "Monthly" && output.ReportingFrequency != "Hourly" {
					t.Fatalf("wrong parent request: %+v", output)
				}
				if original == "Output:Meter,Cogeneration:Electricity,Timestep;" && output.ObjectIndex != nil {
					t.Fatal("new frequency rewrites existing Timestep index")
				}
			}
			builder.addEnergyPathCogenerationOutputs()
			if !reflect.DeepEqual(builder.plan().OutputObjects, plan.OutputObjects) {
				t.Fatal("repeated parent request is not idempotent")
			}
			if doc.String() != before {
				t.Fatal("original output/physical objects changed")
			}
		})
	}
	for _, text := range []string{
		"Version,25.1;", "Version,24.2; ElectricLoadCenter:Inverter:LookUpTable,INV;",
		"Version,25.1; ElectricLoadCenter:Inverter:Simple,INV;",
		"Version,25.1; ElectricLoadCenter:Inverter:LookUpTable,INV; Meter:Custom,Cogeneration:Electricity,Electricity;",
	} {
		doc, err := idf.Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		builder := newPurposePlanBuilder(doc, NormalizeSimulationPurposeRequest(nil))
		builder.addEnergyPathCogenerationOutputs()
		if len(builder.plan().OutputObjects) != 0 {
			t.Fatal("unsupported or custom parent was requested as native")
		}
	}
}
