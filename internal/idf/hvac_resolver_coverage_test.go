package idf

import "testing"

func TestHVACResolverCoverageMatrixRegistersChecklistFamilies(t *testing.T) {
	matrix := HVACResolverCoverageMatrix()
	if len(matrix) == 0 {
		t.Fatal("HVAC resolver coverage matrix is empty")
	}
	seen := map[string]bool{}
	for _, item := range matrix {
		if item.ObjectType == "" || item.Family == "" || item.Resolver == "" || item.Status == "" {
			t.Fatalf("incomplete matrix item: %#v", item)
		}
		if seen[item.ObjectType] {
			t.Fatalf("duplicate matrix object type %q", item.ObjectType)
		}
		seen[item.ObjectType] = true
		switch item.Status {
		case hvacResolverDone, hvacResolverGeneric, hvacResolverTodo:
		default:
			t.Fatalf("unknown resolver status %q for %s", item.Status, item.ObjectType)
		}
	}
	for _, objectType := range []string{
		"AirLoopHVAC",
		"PlantLoop",
		"CondenserLoop",
		"ZoneHVAC:EquipmentConnections",
		"ZoneHVAC:EquipmentList",
		"SpaceHVAC:EquipmentConnections",
		"SpaceHVAC:EquipmentList",
		"ZoneHVAC:AirDistributionUnit",
		"AirTerminal:SingleDuct:VAV:Reheat",
		"ZoneHVAC:FourPipeFanCoil",
		"Coil:Cooling:Water",
		"AirLoopHVAC:UnitarySystem",
		"Chiller:*",
		"Boiler:*",
		"CoolingTower:*",
		"HeatExchanger:*",
		"Fan:*",
		"Pump:*",
		"SetpointManager:*",
	} {
		if !seen[objectType] {
			t.Fatalf("coverage matrix missing checklist object type %q", objectType)
		}
	}
}
