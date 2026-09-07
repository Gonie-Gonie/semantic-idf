package simulation

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// This normal, engine-independent test checks the actual original IDF objects,
// not filenames, comments or catalog tags alone. It does not establish annual
// EnergyPlus output totals, runtime overlap or successful output discovery.
func TestEnergyPathRealFixtureTypes(t *testing.T) {
	_, directory := epathRealDirectories(t)
	catalog := epathLoadRealCatalog(t, directory)
	for _, fixture := range catalog.Fixtures {
		t.Run(fixture.ID, func(t *testing.T) {
			path := epathRealCatalogPath(t, directory, fixture.ModelPath)
			original := epathRequireRealFile(t, path)
			if !strings.EqualFold(epathRealHash(original), fixture.ModelSHA256) {
				t.Fatal("type validation requires the catalog's unmodified official model")
			}
			model, err := epinput.Parse(path, original)
			if err != nil {
				t.Fatal(err)
			}
			structure := epathRealStructure{t: t, byType: map[string][]idf.Object{}}
			for _, object := range epinput.ToIDFDocument(model).Objects {
				key := strings.ToLower(strings.TrimSpace(object.Type))
				structure.byType[key] = append(structure.byType[key], object)
			}
			structure.require("Zone", 1)
			for _, kind := range fixture.Types {
				t.Run(kind, func(t *testing.T) {
					s := structure
					s.t = t
					s.assertKind(kind)
				})
			}
		})
	}
}

type epathRealStructure struct {
	t      *testing.T
	byType map[string][]idf.Object
}

func (s epathRealStructure) objects(kind string) []idf.Object {
	return s.byType[strings.ToLower(kind)]
}

func (s epathRealStructure) require(kind string, minimum int) {
	s.t.Helper()
	if got := len(s.objects(kind)); got < minimum {
		s.t.Fatalf("actual %s objects = %d, require at least %d", kind, got, minimum)
	}
}

func (s epathRealStructure) named(kind, name string) bool {
	for _, object := range s.objects(kind) {
		if name != "" && strings.EqualFold(epathRealTypeField(object, 0), name) {
			return true
		}
	}
	return false
}

func epathRealTypeField(object idf.Object, index int) string {
	if index >= len(object.Fields) {
		return ""
	}
	return strings.TrimSpace(object.Fields[index].Value)
}

func epathRealTypePositive(value string, minimum float64) bool {
	number, err := strconv.ParseFloat(value, 64)
	return err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) && number > minimum
}

func (s epathRealStructure) fieldEquals(kind string, index int, value string) bool {
	for _, object := range s.objects(kind) {
		if strings.EqualFold(epathRealTypeField(object, index), value) {
			return true
		}
	}
	return false
}

func (s epathRealStructure) noPrefixes(prefixes ...string) {
	s.t.Helper()
	for kind, objects := range s.byType {
		for _, prefix := range prefixes {
			if strings.HasPrefix(kind, strings.ToLower(prefix)) && len(objects) > 0 {
				s.t.Errorf("unexpected physical equipment type %s (%d objects)", kind, len(objects))
			}
		}
	}
}

func (s epathRealStructure) sizedPlant(kind string) {
	s.t.Helper()
	// EnergyPlus 22.1--25.1 IDD: Sizing:Plant A1 = loop name, A2 =
	// Heating/Cooling/Condenser. The reference must resolve to an actual loop.
	for _, object := range s.objects("Sizing:Plant") {
		if strings.EqualFold(epathRealTypeField(object, 1), kind) && s.named("PlantLoop", epathRealTypeField(object, 0)) {
			return
		}
	}
	s.t.Fatalf("no actual PlantLoop with typed %s Sizing:Plant reference", kind)
}

func (s epathRealStructure) branchPair(kind, name, inlet, outlet string) bool {
	// EnergyPlus 25.1 IDD Branch A1/A2 precede extensible groups of four:
	// component object type, component name, inlet node, outlet node.
	for _, branch := range s.objects("Branch") {
		for i := 2; i+3 < len(branch.Fields); i += 4 {
			if strings.EqualFold(epathRealTypeField(branch, i), kind) &&
				strings.EqualFold(epathRealTypeField(branch, i+1), name) &&
				inlet != "" && strings.EqualFold(epathRealTypeField(branch, i+2), inlet) &&
				outlet != "" && strings.EqualFold(epathRealTypeField(branch, i+3), outlet) {
				return true
			}
		}
	}
	return false
}

func (s epathRealStructure) zoneMultiplier() bool {
	// EnergyPlus 25.1 IDD Zone: A1 name, N1..N5 direction/origin/type,
	// N6 multiplier => zero-based field 6. Do not confuse it with Type.
	for _, zone := range s.objects("Zone") {
		if epathRealTypePositive(epathRealTypeField(zone, 6), 1) {
			return true
		}
	}
	return false
}

func (s epathRealStructure) zoneGroupMultiplier() bool {
	// EnergyPlus 25.1 IDD ZoneGroup: A1 name, A2 ZoneList, N1 multiplier.
	for _, group := range s.objects("ZoneGroup") {
		if !epathRealTypePositive(epathRealTypeField(group, 2), 1) {
			continue
		}
		for _, list := range s.objects("ZoneList") {
			if !strings.EqualFold(epathRealTypeField(list, 0), epathRealTypeField(group, 1)) || len(list.Fields) < 2 {
				continue
			}
			valid := true
			for i := 1; i < len(list.Fields); i++ {
				valid = valid && s.named("Zone", epathRealTypeField(list, i))
			}
			if valid {
				return true
			}
		}
	}
	return false
}

func (s epathRealStructure) assertKind(kind string) {
	s.t.Helper()
	switch kind {
	case "large_office":
		if got := len(s.objects("Zone")); got != 19 {
			s.t.Fatalf("Large Office requires actual 19 Zone objects, got %d", got)
		}
		s.require("AirLoopHVAC", 4)
		s.require("PlantLoop", 3)
		s.require("Coil:Cooling:Water", 1)
		s.require("Coil:Heating:Water", 1)
		s.require("Chiller:Electric:ReformulatedEIR", 1)
		s.require("Boiler:HotWater", 1)
		s.sizedPlant("Cooling")
		s.sizedPlant("Heating")
	case "small_office":
		s.require("Coil:Cooling:DX:SingleSpeed", 1)
		s.require("Coil:Heating:Fuel", 1)
		if !s.fieldEquals("Coil:Heating:Fuel", 2, "NaturalGas") {
			s.t.Fatal("small office fuel coil must declare actual NaturalGas (IDD A3)")
		}
	case "ideal_loads":
		s.require("ZoneHVAC:IdealLoadsAirSystem", 1)
		s.noPrefixes("Coil:", "PlantLoop", "Boiler:", "Chiller:", "District")
		if len(s.objects("AirLoopHVAC")) != 0 {
			s.t.Fatal("IdealLoads-only model has an actual AirLoopHVAC system")
		}
		// AirLoopHVAC:ReturnPlenum is an allowed passive return-air
		// container, not an AirLoopHVAC system or a heating/cooling coil.
		for objectType := range s.byType {
			if strings.HasPrefix(objectType, "zonehvac:") && objectType != "zonehvac:idealloadsairsystem" && objectType != "zonehvac:equipmentlist" && objectType != "zonehvac:equipmentconnections" {
				s.t.Errorf("IdealLoads-only model has another zone HVAC type: %s", objectType)
			}
		}
	case "ptac_pthp":
		if len(s.objects("ZoneHVAC:PackagedTerminalAirConditioner"))+len(s.objects("ZoneHVAC:PackagedTerminalHeatPump")) == 0 {
			s.t.Fatal("neither actual PTAC nor PTHP object is present")
		}
	case "ptac":
		s.require("ZoneHVAC:PackagedTerminalAirConditioner", 1)
		s.require("Coil:Cooling:DX:SingleSpeed", 1)
	case "pthp":
		s.require("ZoneHVAC:PackagedTerminalHeatPump", 1)
		s.require("Coil:Cooling:DX:SingleSpeed", 1)
		s.require("Coil:Heating:DX:SingleSpeed", 1)
	case "fan_coil":
		s.require("ZoneHVAC:FourPipeFanCoil", 1)
		s.require("Coil:Cooling:Water", 1)
		s.require("Coil:Heating:Water", 1)
		s.sizedPlant("Cooling")
		s.sizedPlant("Heating")
	case "vrf":
		s.require("AirConditioner:VariableRefrigerantFlow", 1)
		s.require("ZoneHVAC:TerminalUnit:VariableRefrigerantFlow", 1)
		s.require("Coil:Cooling:DX:VariableRefrigerantFlow", 1)
		s.require("Coil:Heating:DX:VariableRefrigerantFlow", 1)
	case "radiant":
		s.require("ZoneHVAC:LowTemperatureRadiant:ConstantFlow", 1)
		s.require("PlantLoop", 2)
		for _, radiant := range s.objects("ZoneHVAC:LowTemperatureRadiant:ConstantFlow") {
			// 25.1 IDD: Design/Zone references at 1/3; heating water nodes
			// at 10/11; cooling water nodes at 16/17. Require both actual
			// Branch pairs, not just a suggestive HeatCool filename.
			if !s.named("ZoneHVAC:LowTemperatureRadiant:ConstantFlow:Design", epathRealTypeField(radiant, 1)) || !s.named("Zone", epathRealTypeField(radiant, 3)) {
				s.t.Fatal("radiant design or served Zone does not resolve")
			}
			for _, pair := range [][2]int{{10, 11}, {16, 17}} {
				if !s.branchPair(radiant.Type, epathRealTypeField(radiant, 0), epathRealTypeField(radiant, pair[0]), epathRealTypeField(radiant, pair[1])) {
					s.t.Fatalf("radiant %q lacks actual heating/cooling Branch pair at %v", epathRealTypeField(radiant, 0), pair)
				}
			}
		}
	case "district_energy":
		s.require("DistrictCooling", 1)
		s.require("DistrictHeating:Water", 1)
		s.sizedPlant("Cooling")
		s.sizedPlant("Heating")
	case "mixed_heating_fuels":
		s.require("ZoneHVAC:Baseboard:RadiantConvective:Electric", 1)
		s.require("Coil:Heating:Water", 1)
		if !s.fieldEquals("Boiler:HotWater", 1, "NaturalGas") {
			s.t.Fatal("mixed heating model lacks a NaturalGas boiler (IDD A2)")
		}
		s.sizedPlant("Heating")
	case "zone_multiplier_or_group":
		if !s.zoneMultiplier() && !s.zoneGroupMultiplier() {
			s.t.Fatal("no actual Zone or resolved ZoneGroup multiplier greater than one")
		}
	case "zone_multiplier":
		if !s.zoneMultiplier() {
			s.t.Fatal("Zone multiplier field is not greater than one")
		}
	case "zone_group":
		if !s.zoneGroupMultiplier() {
			s.t.Fatal("no multiplier > 1 ZoneGroup resolves through a ZoneList to actual Zones")
		}
	case "pv_storage":
		s.require("Generator:Photovoltaic", 1)
		s.require("ElectricLoadCenter:Storage:Battery", 1)
		connected := false
		for _, distribution := range s.objects("ElectricLoadCenter:Distribution") {
			// 25.1 IDD fields 1/8 reference generator list/electrical storage.
			connected = connected || (s.named("ElectricLoadCenter:Generators", epathRealTypeField(distribution, 1)) && s.named("ElectricLoadCenter:Storage:Battery", epathRealTypeField(distribution, 8)))
		}
		if !connected {
			s.t.Fatal("PV generator list and actual Battery do not share a distribution")
		}
	case "no_cooling":
		s.require("AirLoopHVAC:Unitary:Furnace:HeatOnly", 1)
		s.require("Coil:Heating:Fuel", 1)
		s.noPrefixes("Coil:Cooling:", "CoilSystem:Cooling:", "Chiller:", "DistrictCooling", "AirConditioner:", "ZoneHVAC:IdealLoads", "ZoneHVAC:PackagedTerminal", "ZoneHVAC:WindowAirConditioner", "PlantLoop")
		// Furnace contains a SingleCooling thermostat schedule. A control
		// object is not physical cooling equipment or a reported cooling load.
	case "no_heating":
		s.require("Coil:Cooling:DX:SingleSpeed", 1)
		s.require("Coil:Cooling:Water", 1)
		s.noPrefixes("Coil:Heating:", "CoilSystem:Heating:", "Boiler:", "DistrictHeating", "ZoneHVAC:Baseboard:", "ZoneHVAC:LowTemperatureRadiant:", "ZoneHVAC:IdealLoads", "ZoneHVAC:PackagedTerminalHeatPump", "AirConditioner:VariableRefrigerantFlow", "CentralHeatPumpSystem")
	case "simultaneous_heating_cooling":
		s.require("CentralHeatPumpSystem", 1)
		s.require("ChillerHeaterPerformance:Electric:EIR", 1)
		s.sizedPlant("Cooling")
		s.sizedPlant("Heating")
		for _, system := range s.objects("CentralHeatPumpSystem") {
			// 25.1 IDD fields 10/11 reference the first performance module;
			// cooling/source/heating water pairs are 2/3, 4/5 and 6/7.
			if !strings.EqualFold(epathRealTypeField(system, 10), "ChillerHeaterPerformance:Electric:EIR") || !s.named("ChillerHeaterPerformance:Electric:EIR", epathRealTypeField(system, 11)) {
				s.t.Fatal("CentralHeatPumpSystem has no actual referenced chiller/heater performance module")
			}
			for _, pair := range [][2]int{{2, 3}, {4, 5}, {6, 7}} {
				if !s.branchPair(system.Type, epathRealTypeField(system, 0), epathRealTypeField(system, pair[0]), epathRealTypeField(system, pair[1])) {
					s.t.Fatalf("chiller/heater lacks actual water Branch pair %v", pair)
				}
			}
		}
		s.t.Log("physical simultaneous-capable topology only; interval overlap remains an actual-SQL acceptance obligation")
	case "output_alias_discovery":
		s.t.Skip("excluded from static type proof: requires actual non-exact RDD/MDD resolution and reported SQL evidence")
	default:
		s.t.Fatalf("catalog type %q lacks an explicit structural check or documented runtime-only exclusion", kind)
	}
}
