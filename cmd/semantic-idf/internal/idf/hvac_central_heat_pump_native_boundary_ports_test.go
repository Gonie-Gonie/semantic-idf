package idf

import (
	"fmt"
	"testing"
)

func TestNativeCentralHeatPumpActualPipePumpPortsCannotBorrowBranchDeclarations(t *testing.T) {
	for _, target := range [][2]string{
		{"Pump:VariableSpeed", "Circ Pump"},
		{"Pump:VariableSpeed", "HW Circ Pump"},
		{"Pipe:Adiabatic", "Supply Side Outlet Pipe"},
		{"Pipe:Adiabatic", "Heating Supply Outlet"},
		{"Pipe:Adiabatic", "Condenser Demand Inlet Pipe"},
		{"Pipe:Adiabatic", "Condenser Demand Outlet Pipe"},
		{"DistrictCooling", "Purchased Cooling"},
		{"DistrictHeating:Water", "Purchased Heating"},
	} {
		for _, field := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/field%d", target[1], field), func(t *testing.T) {
				doc := nativeCentralHeatPumpOriginal(t)
				// Keep every Branch, connector, module and coil byte unchanged:
				// only the actual component's native water port is disconnected.
				centralHeatPumpTestObject(t, &doc, target[0], target[1]).Fields[field].Value = "Disconnected actual native port"
				bindings := ResolveNativeCentralHeatPumpBindings(doc)
				if len(bindings) != 1 || !bindings[0].ReportingIdentityValid || !bindings[0].NativeDefinitionValid || bindings[0].PortBindingsComplete {
					t.Fatalf("native source identity must survive but disconnected boundary must deny service eligibility: %+v", bindings)
				}
				assertCentralHeatPumpOriginalRoutes(t, AnalyzeHVAC(doc), 0, 0)
			})
		}
	}
}

func TestNativeCentralHeatPumpUnknownWaterBranchKindCannotBorrowKnownPosition(t *testing.T) {
	doc := nativeCentralHeatPumpOriginal(t)
	// The object still uniquely exists and its two fields match the Branch,
	// but this finite cohort has no reviewed Pipe:Indoor port/ownership proof.
	centralHeatPumpTestObject(t, &doc, "Pipe:Adiabatic", "Supply Side Outlet Pipe").Type = "Pipe:Indoor"
	centralHeatPumpTestObject(t, &doc, "Branch", "Cooling Supply Outlet").Fields[2].Value = "Pipe:Indoor"
	bindings := ResolveNativeCentralHeatPumpBindings(doc)
	if len(bindings) != 1 || !bindings[0].ReportingIdentityValid || !bindings[0].NativeDefinitionValid || bindings[0].PortBindingsComplete {
		t.Fatalf("unknown component acquired complete physical eligibility: %+v", bindings)
	}
	assertCentralHeatPumpOriginalRoutes(t, AnalyzeHVAC(doc), 0, 0)
}
