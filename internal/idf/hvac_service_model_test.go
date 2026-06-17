package idf

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestHVACServiceModelReferenceLargeOfficeZonePaths(t *testing.T) {
	text, err := os.ReadFile("../../frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	if err != nil {
		t.Fatalf("read reference sample: %v", err)
	}
	doc, err := Parse(string(text))
	if err != nil {
		t.Fatalf("parse reference sample: %v", err)
	}

	report := AnalyzeHVAC(doc)
	coreBottom := findHVACTestingZoneService(report.ServiceModel, "Core_bottom")
	if coreBottom == nil {
		t.Fatalf("Core_bottom zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	if !hasZoneServicePath(coreBottom.Paths, "cooling", "central_air_with_plant", "CoolSys1", "VAV_1", "Core_bottom VAV Box") {
		t.Fatalf("Core_bottom paths = %#v, want cooling CoolSys1 -> VAV_1 -> terminal path", coreBottom.Paths)
	}
	if !hasZoneServicePath(coreBottom.Paths, "heating", "central_air_with_plant", "HeatSys1", "VAV_1", "Core_bottom VAV Box") {
		t.Fatalf("Core_bottom paths = %#v, want heating HeatSys1 -> VAV_1 -> terminal path", coreBottom.Paths)
	}
	if !hasZoneServicePath(coreBottom.Paths, "ventilation", "central_air", "", "VAV_1", "Core_bottom VAV Box") {
		t.Fatalf("Core_bottom paths = %#v, want ventilation VAV_1 -> terminal path", coreBottom.Paths)
	}
	for _, path := range coreBottom.Paths {
		if path.DeliveryEquipment.DeliveryType == "fan_coil" || path.DeliveryEquipment.DeliveryType == "zone_direct_unit" {
			t.Fatalf("Core_bottom VAV path misclassified as direct zone equipment: %#v", path)
		}
		if path.ServiceKind == "heating" && path.AirLoop == nil {
			t.Fatalf("Core_bottom heating service missing air loop context: %#v", path)
		}
	}
}

func TestHVACServiceModelBuildsNavigationIndex(t *testing.T) {
	text, err := os.ReadFile("../../frontend/src/samples/RefBldgLargeOfficeNew2004_Chicago.idf")
	if err != nil {
		t.Fatalf("read reference sample: %v", err)
	}
	doc, err := Parse(string(text))
	if err != nil {
		t.Fatalf("parse reference sample: %v", err)
	}

	report := AnalyzeHVAC(doc)
	coreBottom := findHVACTestingZoneService(report.ServiceModel, "Core_bottom")
	if coreBottom == nil {
		t.Fatalf("Core_bottom zone service not found")
	}
	path := findZoneServicePath(coreBottom.Paths, "cooling", "central_air_with_plant", "CoolSys1", "VAV_1", "Core_bottom VAV Box")
	if path == nil {
		t.Fatalf("Core_bottom cooling service path not found: %#v", coreBottom.Paths)
	}
	navigation := report.ServiceModel.Navigation
	if len(navigation.Entities) == 0 || len(navigation.Links) == 0 {
		t.Fatalf("navigation index is empty: %#v", navigation)
	}

	zoneID := "zone:" + normalizeName("Core_bottom")
	loopID := "loop:" + loopRefKey("PlantLoop", "CoolSys1")
	componentID := navigationComponentID(path.Delivery)
	pathEntityID := navigationPathID(path.ID)
	for _, assertion := range []struct {
		name   string
		values []string
		want   string
	}{
		{"zone reverse index", navigation.ByZone[zoneID], path.ID},
		{"loop reverse index", navigation.ByLoop[loopID], path.ID},
		{"component reverse index", navigation.ByComponent[componentID], path.ID},
		{"path entity index", navigation.ByPath[path.ID], pathEntityID},
	} {
		if !stringSliceContains(assertion.values, assertion.want) {
			t.Fatalf("%s = %#v, want %s", assertion.name, assertion.values, assertion.want)
		}
	}
	if strings.HasPrefix(componentID, "component:") && strings.TrimPrefix(componentID, "component:") == path.Delivery.ID {
		t.Fatalf("navigation component id still mirrors object-index-only id: %s", componentID)
	}
	if findNavigationEntity(navigation.Entities, pathEntityID, "service_path") == nil {
		t.Fatalf("navigation entities missing path entity %s", pathEntityID)
	}
	if findNavigationLink(navigation.Links, pathEntityID, zoneID, "serves") == nil {
		t.Fatalf("navigation links missing path -> zone serve link")
	}
	if findNavigationLink(navigation.Links, pathEntityID, loopID, "uses_loop") == nil {
		t.Fatalf("navigation links missing path -> loop link")
	}
	system := findSystemSummary(report.ServiceModel.Systems, "PlantLoop", "CoolSys1")
	if system == nil {
		t.Fatalf("systems missing CoolSys1 summary: %#v", report.ServiceModel.Systems)
	}
	if !stringSliceContains(system.RelatedPathIDs, path.ID) || !stringSliceContains(system.RelatedZoneNames, "Core_bottom") {
		t.Fatalf("CoolSys1 system summary missing service reverse refs: %#v", system)
	}
	component := findComponentIndexItem(report.ServiceModel.Components, path.Delivery.ObjectType, path.Delivery.ObjectName)
	if component == nil {
		t.Fatalf("components missing delivery component %#v", path.Delivery)
	}
	if !stringSliceContains(component.RelatedPathIDs, path.ID) || !stringSliceContains(component.RelatedZoneNames, "Core_bottom") {
		t.Fatalf("component index missing related paths/zones: %#v", component)
	}
}

func TestHVACNavigationComponentIDUsesTypeAndName(t *testing.T) {
	left := navigationComponentID(ComponentRef{ObjectType: "Coil:Cooling:Water", ObjectName: "Shared Name", ObjectIndex: 10})
	right := navigationComponentID(ComponentRef{ObjectType: "Fan:ConstantVolume", ObjectName: "Shared Name", ObjectIndex: 11})
	if left == right {
		t.Fatalf("different component types collided: %s", left)
	}
	for _, id := range []string{left, right} {
		if strings.HasPrefix(strings.TrimPrefix(id, "component:"), "10") || strings.HasPrefix(strings.TrimPrefix(id, "component:"), "11") {
			t.Fatalf("component navigation id should not be object-index-only: %s", id)
		}
		if !strings.Contains(id, normalizeName("Shared Name")) {
			t.Fatalf("component navigation id should include normalized name: %s", id)
		}
	}
}

func TestHVACServiceModelExternalFixtureMatrix(t *testing.T) {
	fixtures := []struct {
		name       string
		path       string
		assertions []func(*testing.T, HVACServiceModel)
	}{
		{
			name: "heatfloor radiant plant",
			path: "testdata/hvac_external/heatfloor.idf",
			assertions: []func(*testing.T, HVACServiceModel){
				func(t *testing.T, model HVACServiceModel) {
					if !hasAnyServicePath(model, "radiant_heating", "radiant", "radiant_floor") {
						t.Fatalf("heatfloor service model missing radiant floor heating path: %#v", model.ZoneServices)
					}
				},
				func(t *testing.T, model HVACServiceModel) {
					if hasServicePathWithAirLoop(model, "radiant") {
						t.Fatalf("radiant fixture should not force radiant paths through an air loop: %#v", model.ZoneServices)
					}
				},
			},
		},
		{
			name: "office vrf erv",
			path: "testdata/hvac_external/Office_1_default.idf",
			assertions: []func(*testing.T, HVACServiceModel){
				func(t *testing.T, model HVACServiceModel) {
					if !hasAnyServicePath(model, "cooling", "direct_zone_refrigerant", "vrf_indoor") {
						t.Fatalf("office fixture missing VRF cooling service path: %#v", model.ZoneServices)
					}
				},
				func(t *testing.T, model HVACServiceModel) {
					if !hasAnyServicePath(model, "heating", "direct_zone_refrigerant", "vrf_indoor") {
						t.Fatalf("office fixture missing VRF heating service path: %#v", model.ZoneServices)
					}
				},
				func(t *testing.T, model HVACServiceModel) {
					if !hasAnyServicePath(model, "ventilation", "ventilation_only", "erv") {
						t.Fatalf("office fixture missing ERV ventilation service path: %#v", model.ZoneServices)
					}
				},
			},
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			text, err := os.ReadFile(fixture.path)
			if os.IsNotExist(err) {
				t.Skipf("external fixture %s is not checked in; run TestExternalHVACFixtureMatrix with local fixture paths for full coverage", fixture.path)
			}
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			doc, err := Parse(string(text))
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			report := AnalyzeHVAC(doc)
			if len(report.ServiceModel.ZoneServices) == 0 {
				t.Fatalf("%s produced no zone services", fixture.path)
			}
			for _, assert := range fixture.assertions {
				assert(t, report.ServiceModel)
			}
		})
	}
}

func TestHVACServiceModelKeepsFanCoilAsDirectZoneDelivery(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office", Comment: "Zone Name"},
			{Value: "Office Equipment", Comment: "Zone Conditioning Equipment List Name"},
			{Value: "Office Supply Inlet", Comment: "Zone Air Inlet Node or NodeList Name"},
			{Value: "", Comment: "Zone Air Exhaust Node or NodeList Name"},
			{Value: "Office Zone Air Node", Comment: "Zone Air Node Name"},
			{Value: "", Comment: "Zone Return Air Node or NodeList Name"},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment", Comment: "Name"},
			{Value: "ZoneHVAC:FourPipeFanCoil", Comment: "Zone Equipment 1 Object Type"},
			{Value: "Office FPFC", Comment: "Zone Equipment 1 Name"},
			{Value: "1", Comment: "Zone Equipment 1 Cooling Sequence"},
			{Value: "1", Comment: "Zone Equipment 1 Heating or No-Load Sequence"},
		}},
		{Index: 3, Type: "ZoneHVAC:FourPipeFanCoil", Fields: []Field{
			{Value: "Office FPFC", Comment: "Name"},
			{Value: "", Comment: "Availability Schedule Name"},
			{Value: "Autosize", Comment: "Maximum Supply Air Flow Rate"},
			{Value: "Office Supply Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Office Zone Air Node", Comment: "Air Outlet Node Name"},
			{Value: "HW Inlet", Comment: "Hot Water Inlet Node Name"},
			{Value: "HW Outlet", Comment: "Hot Water Outlet Node Name"},
			{Value: "CHW Inlet", Comment: "Chilled Water Inlet Node Name"},
			{Value: "CHW Outlet", Comment: "Chilled Water Outlet Node Name"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	if !hasZoneServicePath(office.Paths, "cooling", "direct_zone_air", "", "", "Office FPFC") {
		t.Fatalf("Office paths = %#v, want fan coil cooling as direct zone air", office.Paths)
	}
	if !hasZoneServicePath(office.Paths, "heating", "direct_zone_air", "", "", "Office FPFC") {
		t.Fatalf("Office paths = %#v, want fan coil heating as direct zone air", office.Paths)
	}
	for _, path := range office.Paths {
		if path.AirLoop != nil {
			t.Fatalf("fan coil path gained an air loop: %#v", path)
		}
		if path.DeliveryEquipment.DeliveryType != "fan_coil" || path.DeliveryEquipment.RequiresAirLoop {
			t.Fatalf("fan coil delivery classification = %#v", path.DeliveryEquipment)
		}
	}
}

func TestHVACServiceModelBuildsHydronicFanCoilPlantPath(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office", Comment: "Zone Name"},
			{Value: "Office Equipment", Comment: "Zone Conditioning Equipment List Name"},
			{Value: "Office Supply Inlet", Comment: "Zone Air Inlet Node or NodeList Name"},
			{Value: "", Comment: "Zone Air Exhaust Node or NodeList Name"},
			{Value: "Office Zone Air Node", Comment: "Zone Air Node Name"},
			{Value: "", Comment: "Zone Return Air Node or NodeList Name"},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment", Comment: "Name"},
			{Value: "ZoneHVAC:FourPipeFanCoil", Comment: "Zone Equipment 1 Object Type"},
			{Value: "Office FPFC", Comment: "Zone Equipment 1 Name"},
			{Value: "1", Comment: "Zone Equipment 1 Cooling Sequence"},
			{Value: "1", Comment: "Zone Equipment 1 Heating or No-Load Sequence"},
		}},
		{Index: 3, Type: "ZoneHVAC:FourPipeFanCoil", Fields: []Field{
			{Value: "Office FPFC", Comment: "Name"},
			{Value: "", Comment: "Availability Schedule Name"},
			{Value: "Autosize", Comment: "Maximum Supply Air Flow Rate"},
			{Value: "Office Supply Inlet", Comment: "Air Inlet Node Name"},
			{Value: "Office Zone Air Node", Comment: "Air Outlet Node Name"},
			{Value: "HW Inlet", Comment: "Hot Water Inlet Node Name"},
			{Value: "HW Outlet", Comment: "Hot Water Outlet Node Name"},
			{Value: "CHW Inlet", Comment: "Chilled Water Inlet Node Name"},
			{Value: "CHW Outlet", Comment: "Chilled Water Outlet Node Name"},
		}},
		{Index: 4, Type: "PlantLoop", Fields: []Field{
			{Value: "Office Water Loop"},
			{Value: "Water"},
			{Value: ""},
			{Value: ""},
			{Value: "Loop Setpoint"},
			{Value: "80"},
			{Value: "20"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Autosize"},
			{Value: "Supply Inlet"},
			{Value: "Supply Outlet"},
			{Value: ""},
			{Value: ""},
			{Value: "Demand Inlet"},
			{Value: "Demand Outlet"},
			{Value: "Demand Branches"},
			{Value: ""},
		}},
		{Index: 5, Type: "BranchList", Fields: []Field{
			{Value: "Demand Branches"},
			{Value: "Fan Coil Branch"},
		}},
		{Index: 6, Type: "Branch", Fields: []Field{
			{Value: "Fan Coil Branch"},
			{Value: ""},
			{Value: "ZoneHVAC:FourPipeFanCoil"},
			{Value: "Office FPFC"},
			{Value: "HW Inlet"},
			{Value: "HW Outlet"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	for _, serviceKind := range []string{"cooling", "heating"} {
		path := findZoneServicePath(office.Paths, serviceKind, "direct_zone_hydronic", "Office Water Loop", "", "Office FPFC")
		if path == nil {
			t.Fatalf("Office paths = %#v, want %s PlantLoop -> FanCoil -> Zone path", office.Paths, serviceKind)
		}
		if path.AirLoop != nil {
			t.Fatalf("hydronic fan coil path gained an air loop: %#v", path)
		}
		if path.DeliveryEquipment.DeliveryType != "fan_coil" || !path.DeliveryEquipment.CanUsePlantLoop {
			t.Fatalf("fan coil delivery classification = %#v", path.DeliveryEquipment)
		}
	}
}

func TestHVACServiceModelClassifiesPackagedLocalEquipment(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Supply Inlet"},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "ZoneHVAC:PackagedTerminalHeatPump"},
			{Value: "Office PTHP"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 3, Type: "ZoneHVAC:PackagedTerminalHeatPump", Fields: []Field{
			{Value: "Office PTHP"},
			{Value: ""},
			{Value: "Autosize"},
			{Value: "Office Supply Inlet"},
			{Value: "Office Zone Air Node"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	for _, serviceKind := range []string{"cooling", "heating"} {
		path := findZoneServicePath(office.Paths, serviceKind, "direct_zone_air", "", "", "Office PTHP")
		if path == nil {
			t.Fatalf("Office paths = %#v, want %s packaged heat pump path", office.Paths, serviceKind)
		}
		if path.DeliveryEquipment.DeliveryType != "pthp" || path.DeliveryEquipment.RequiresAirLoop {
			t.Fatalf("PTHP delivery classification = %#v", path.DeliveryEquipment)
		}
		if path.SourceSystem == nil || path.SourceSystem.Name == "" {
			t.Fatalf("PTHP path missing local source system: %#v", path)
		}
		if strings.Contains(strings.ToLower(path.Delivery.DisplayFamily), "terminal") {
			t.Fatalf("PTHP delivery label still looks terminal-like: %#v", path.Delivery)
		}
	}
}

func TestHVACServiceModelClassifiesWindowAndEvaporativeCoolerAsLocalEquipment(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Supply Inlet"},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "SequentialLoad"},
			{Value: "ZoneHVAC:WindowAirConditioner"},
			{Value: "Office Window AC"},
			{Value: "1"},
			{Value: "1"},
			{Value: "ZoneHVAC:EvaporativeCoolerUnit"},
			{Value: "Office Evap Cooler"},
			{Value: "2"},
			{Value: "2"},
		}},
		{Index: 3, Type: "ZoneHVAC:WindowAirConditioner", Fields: []Field{
			{Value: "Office Window AC"},
			{Value: ""},
			{Value: "Autosize"},
			{Value: "Office Supply Inlet"},
			{Value: "Office Zone Air Node"},
		}},
		{Index: 4, Type: "ZoneHVAC:EvaporativeCoolerUnit", Fields: []Field{
			{Value: "Office Evap Cooler"},
			{Value: ""},
			{Value: "Autosize"},
			{Value: "Office Supply Inlet"},
			{Value: "Office Zone Air Node"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	for _, expected := range []struct {
		name         string
		deliveryType string
		source       string
	}{
		{"Office Window AC", "window_ac", "Local DX"},
		{"Office Evap Cooler", "evaporative_cooler", "Local evaporative cooling"},
	} {
		path := findZoneServicePath(office.Paths, "cooling", "direct_zone_air", "", "", expected.name)
		if path == nil {
			t.Fatalf("Office paths = %#v, want cooling path for %s", office.Paths, expected.name)
		}
		if path.AirLoop != nil || path.PlantLoop != nil {
			t.Fatalf("%s path gained loop context: %#v", expected.name, path)
		}
		if path.DeliveryEquipment.DeliveryType != expected.deliveryType || path.DeliveryEquipment.RequiresAirLoop {
			t.Fatalf("%s delivery classification = %#v", expected.name, path.DeliveryEquipment)
		}
		if path.SourceSystem == nil || path.SourceSystem.Name != expected.source {
			t.Fatalf("%s source system = %#v, want %q", expected.name, path.SourceSystem, expected.source)
		}
	}
}

func TestHVACServiceModelPackagedLocalPathGoldenJSON(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Supply Inlet"},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "ZoneHVAC:PackagedTerminalHeatPump"},
			{Value: "Office PTHP"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 3, Type: "ZoneHVAC:PackagedTerminalHeatPump", Fields: []Field{
			{Value: "Office PTHP"},
			{Value: ""},
			{Value: "Autosize"},
			{Value: "Office Supply Inlet"},
			{Value: "Office Zone Air Node"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	gotBytes, err := json.MarshalIndent(servicePathGoldenRows(office.Paths), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	const want = `[
  {
    "serviceKind": "cooling",
    "pathType": "direct_zone_air",
    "deliveryType": "pthp",
    "delivery": "Office PTHP",
    "source": "Local DX"
  },
  {
    "serviceKind": "heating",
    "pathType": "direct_zone_air",
    "deliveryType": "pthp",
    "delivery": "Office PTHP",
    "source": "Local electric/gas/heat pump"
  }
]`
	if string(gotBytes) != want {
		t.Fatalf("packaged local service path JSON mismatch\nwant:\n%s\n\ngot:\n%s", want, string(gotBytes))
	}
}

func TestHVACServiceModelBuildsRadiantPlantPath(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: ""},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "ZoneHVAC:LowTemperatureRadiant:VariableFlow"},
			{Value: "Office Radiant"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 3, Type: "ZoneHVAC:LowTemperatureRadiant:VariableFlow", Fields: []Field{
			{Value: "Office Radiant"},
			{Value: "Radiant Design"},
			{Value: ""},
			{Value: "Office"},
			{Value: "Office Radiant Surfaces"},
			{Value: "100"},
			{Value: "Autosize"},
			{Value: "Autosize"},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
		}},
		{Index: 4, Type: "PlantLoop", Fields: []Field{
			{Value: "Heating Water Loop"},
			{Value: "Water"},
			{Value: ""},
			{Value: ""},
			{Value: "HW Setpoint"},
			{Value: "80"},
			{Value: "20"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Autosize"},
			{Value: "HW Supply Inlet"},
			{Value: "HW Supply Outlet"},
			{Value: "HW Supply Branches"},
			{Value: ""},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
			{Value: "HW Demand Branches"},
			{Value: ""},
		}},
		{Index: 5, Type: "BranchList", Fields: []Field{
			{Value: "HW Demand Branches"},
			{Value: "Radiant Branch"},
		}},
		{Index: 6, Type: "Branch", Fields: []Field{
			{Value: "Radiant Branch"},
			{Value: ""},
			{Value: "ZoneHVAC:LowTemperatureRadiant:VariableFlow"},
			{Value: "Office Radiant"},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	path := findZoneServicePath(office.Paths, "radiant_heating", "radiant", "Heating Water Loop", "", "Office Radiant")
	if path == nil {
		t.Fatalf("Office paths = %#v, want radiant plant path", office.Paths)
	}
	if path.AirLoop != nil {
		t.Fatalf("radiant path gained an air loop: %#v", path)
	}
	if path.DeliveryEquipment.DeliveryType != "radiant_floor" {
		t.Fatalf("radiant delivery classification = %#v", path.DeliveryEquipment)
	}
}

func TestHVACServiceModelLinksLoopSupportingCouplingsToServicePaths(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: ""},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "ZoneHVAC:LowTemperatureRadiant:VariableFlow"},
			{Value: "Office Radiant"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 3, Type: "ZoneHVAC:LowTemperatureRadiant:VariableFlow", Fields: []Field{
			{Value: "Office Radiant"},
			{Value: "Radiant Design"},
			{Value: ""},
			{Value: "Office"},
			{Value: "Office Radiant Surface"},
			{Value: "100"},
			{Value: "Autosize"},
			{Value: "Autosize"},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
		}},
		{Index: 4, Type: "PlantLoop", Fields: []Field{
			{Value: "Heating Water Loop"},
			{Value: "Water"},
			{Value: ""},
			{Value: "TES Operation"},
			{Value: "HW Setpoint"},
			{Value: "80"},
			{Value: "20"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Autosize"},
			{Value: "HW Supply Inlet"},
			{Value: "HW Supply Outlet"},
			{Value: "HW Supply Branches"},
			{Value: ""},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
			{Value: "HW Demand Branches"},
			{Value: ""},
		}},
		{Index: 5, Type: "BranchList", Fields: []Field{
			{Value: "HW Supply Branches"},
			{Value: "Storage Branch"},
		}},
		{Index: 6, Type: "Branch", Fields: []Field{
			{Value: "Storage Branch"},
			{Value: ""},
			{Value: "ThermalStorage:Ice:Simple"},
			{Value: "Ice Tank"},
			{Value: "TES Inlet"},
			{Value: "TES Outlet"},
		}},
		{Index: 7, Type: "BranchList", Fields: []Field{
			{Value: "HW Demand Branches"},
			{Value: "Radiant Branch"},
		}},
		{Index: 8, Type: "Branch", Fields: []Field{
			{Value: "Radiant Branch"},
			{Value: ""},
			{Value: "ZoneHVAC:LowTemperatureRadiant:VariableFlow"},
			{Value: "Office Radiant"},
			{Value: "HW Demand Inlet"},
			{Value: "HW Demand Outlet"},
		}},
		{Index: 9, Type: "ThermalStorage:Ice:Simple", Fields: []Field{{Value: "Ice Tank"}}},
		{Index: 10, Type: "PlantEquipmentOperation:ThermalEnergyStorage", Fields: []Field{{Value: "TES Operation"}}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	path := findZoneServicePath(office.Paths, "radiant_heating", "radiant", "Heating Water Loop", "", "Office Radiant")
	if path == nil {
		t.Fatalf("Office paths = %#v, want radiant service path", office.Paths)
	}
	if len(path.SupportingCouplings) == 0 {
		t.Fatalf("radiant path missing supporting loop couplings: %#v", path)
	}
	storage := findSystemCouplingByObject(report.ServiceModel.Couplings, "ThermalStorage:Ice:Simple", "Ice Tank")
	if storage == nil {
		t.Fatalf("couplings = %#v, want Ice Tank thermal storage coupling", report.ServiceModel.Couplings)
	}
	if !stringSliceContains(path.SupportingCouplings, storage.ID) {
		t.Fatalf("path supporting couplings = %#v, want %s", path.SupportingCouplings, storage.ID)
	}
	if !hasLoopRef(storage.ConnectedLoops, "PlantLoop", "Heating Water Loop") {
		t.Fatalf("storage connected loops = %#v, want Heating Water Loop", storage.ConnectedLoops)
	}
	if path.Delivery.ObjectName == "Ice Tank" || componentRefsContainObject(path.Conditioning, "ThermalStorage:Ice:Simple", "Ice Tank") {
		t.Fatalf("supporting storage leaked into primary service path: %#v", path)
	}
	couplingEntityID := navigationCouplingID(storage.ID)
	if !stringSliceContains(report.ServiceModel.Navigation.ByCoupling[couplingEntityID], path.ID) {
		t.Fatalf("navigation coupling reverse index = %#v, want %s", report.ServiceModel.Navigation.ByCoupling[couplingEntityID], path.ID)
	}
	if findNavigationLink(report.ServiceModel.Navigation.Links, navigationPathID(path.ID), couplingEntityID, "supported_by") == nil {
		t.Fatalf("navigation links missing path -> supporting coupling link")
	}
	if !hasSystemCoupling(report.ServiceModel.Couplings, "PlantEquipmentOperation:ThermalEnergyStorage", "operation_scheme", "thermal_storage_operation") {
		t.Fatalf("couplings = %#v, want thermal storage operation scheme coupling", report.ServiceModel.Couplings)
	}
}

func TestHVACServiceModelBuildsVRFRefrigerantPath(t *testing.T) {
	vrfSystemFields := make([]Field, 37)
	vrfSystemFields[0] = Field{Value: "Office VRF Outdoor"}
	vrfSystemFields[36] = Field{Value: "Office VRF Terminal List"}
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Inlet"},
			{Value: ""},
			{Value: "Office Air Node"},
			{Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "SequentialLoad"},
			{Value: "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow"},
			{Value: "Office VRF Terminal"},
			{Value: "1"},
			{Value: "1"},
			{Value: ""},
			{Value: ""},
		}},
		{Index: 3, Type: "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow", Fields: []Field{
			{Value: "Office VRF Terminal"},
			{Value: "Always On"},
			{Value: "Office VRF Inlet"},
			{Value: "Office Inlet"},
		}},
		{Index: 4, Type: "AirConditioner:VariableRefrigerantFlow", Fields: vrfSystemFields},
		{Index: 5, Type: "ZoneTerminalUnitList", Fields: []Field{
			{Value: "Office VRF Terminal List"},
			{Value: "Office VRF Terminal"},
		}},
	}}

	report := AnalyzeHVAC(doc)
	office := findHVACTestingZoneService(report.ServiceModel, "Office")
	if office == nil {
		t.Fatalf("Office zone service not found: %#v", report.ServiceModel.ZoneServices)
	}
	for _, serviceKind := range []string{"cooling", "heating"} {
		path := findZoneServicePath(office.Paths, serviceKind, "direct_zone_refrigerant", "", "", "Office VRF Terminal")
		if path == nil {
			t.Fatalf("Office paths = %#v, want %s VRF refrigerant path", office.Paths, serviceKind)
		}
		if path.RefrigerantSystem == nil || path.RefrigerantSystem.Name != "Office VRF Outdoor" {
			t.Fatalf("VRF path refrigerant system = %#v", path.RefrigerantSystem)
		}
		if path.PlantLoop != nil || path.AirLoop != nil {
			t.Fatalf("VRF path gained plant/air loop: %#v", path)
		}
	}
}

func TestHVACServiceModelClassifiesSupportingSystemCouplings(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "ThermalStorage:Ice:Simple", Fields: []Field{{Value: "Ice Tank"}}},
		{Index: 1, Type: "ThermalStorage:ChilledWater:Mixed", Fields: []Field{{Value: "CHW Tank"}}},
		{Index: 2, Type: "CoolingTower:SingleSpeed", Fields: []Field{{Value: "Tower"}}},
		{Index: 3, Type: "Generator:Photovoltaic", Fields: []Field{{Value: "PV Array"}}},
		{Index: 4, Type: "ElectricLoadCenter:Storage:Simple", Fields: []Field{{Value: "Battery"}}},
		{Index: 5, Type: "WaterHeater:Mixed", Fields: []Field{{Value: "DHW Heater"}}},
		{Index: 6, Type: "WaterUse:Equipment", Fields: []Field{{Value: "Lavatory"}}},
		{Index: 7, Type: "SetpointManager:Scheduled", Fields: []Field{{Value: "SAT Reset"}}},
		{Index: 8, Type: "FaultModel:TemperatureSensorOffset:OutdoorAir", Fields: []Field{{Value: "OA Sensor Fault"}}},
		{Index: 9, Type: "EnergyManagementSystem:Program", Fields: []Field{{Value: "EMS Program"}}},
		{Index: 10, Type: "PythonPlugin:Instance", Fields: []Field{{Value: "Python Plugin"}}},
		{Index: 11, Type: "ExternalInterface:Schedule", Fields: []Field{{Value: "External Schedule"}}},
		{Index: 12, Type: "PlantEquipmentOperation:ThermalEnergyStorage", Fields: []Field{{Value: "TES Operation"}}},
		{Index: 13, Type: "FluidCooler:SingleSpeed", Fields: []Field{{Value: "Fluid Cooler"}}},
		{Index: 14, Type: "EvaporativeFluidCooler:SingleSpeed", Fields: []Field{{Value: "Evap Fluid Cooler"}}},
		{Index: 15, Type: "GroundHeatExchanger:Vertical", Fields: []Field{{Value: "Ground HX"}}},
		{Index: 16, Type: "PipingSystem:Underground:Domain", Fields: []Field{{Value: "Underground Domain"}}},
		{Index: 17, Type: "HeatExchanger:FluidToFluid", Fields: []Field{{Value: "Loop HX"}}},
		{Index: 18, Type: "Generator:FuelCell", Fields: []Field{{Value: "Fuel Cell"}}},
		{Index: 19, Type: "Generator:FuelCell:ExhaustGasToWaterHeatExchanger", Fields: []Field{{Value: "Fuel Cell HX"}}},
		{Index: 20, Type: "Generator:FuelSupply", Fields: []Field{{Value: "Fuel Supply"}}},
		{Index: 21, Type: "Controller:WaterCoil", Fields: []Field{{Value: "Water Coil Controller"}}},
		{Index: 22, Type: "AvailabilityManagerAssignmentList", Fields: []Field{{Value: "Availability Managers"}}},
	}}

	report := AnalyzeHVAC(doc)
	couplings := report.ServiceModel.Couplings
	for _, expected := range []struct {
		objectType   string
		couplingType string
		role         string
	}{
		{"ThermalStorage:Ice:Simple", "thermal_storage", "ice_storage"},
		{"ThermalStorage:ChilledWater:Mixed", "thermal_storage", "chilled_water_storage"},
		{"CoolingTower:SingleSpeed", "heat_rejection", "cooling_tower"},
		{"Generator:Photovoltaic", "generator", "pv"},
		{"ElectricLoadCenter:Storage:Simple", "electric_storage", "battery"},
		{"WaterHeater:Mixed", "service_water", "water_heater"},
		{"WaterUse:Equipment", "service_water", "water_use"},
		{"SetpointManager:Scheduled", "control_overlay", "setpoint_manager"},
		{"FaultModel:TemperatureSensorOffset:OutdoorAir", "fault_overlay", "fault_model"},
		{"EnergyManagementSystem:Program", "control_overlay", "ems_external"},
		{"PythonPlugin:Instance", "control_overlay", "ems_external"},
		{"ExternalInterface:Schedule", "control_overlay", "ems_external"},
		{"PlantEquipmentOperation:ThermalEnergyStorage", "operation_scheme", "thermal_storage_operation"},
		{"FluidCooler:SingleSpeed", "heat_rejection", "fluid_cooler"},
		{"EvaporativeFluidCooler:SingleSpeed", "heat_rejection", "fluid_cooler"},
		{"GroundHeatExchanger:Vertical", "source_sink", "ground_hx"},
		{"PipingSystem:Underground:Domain", "source_sink", "ground_hx"},
		{"HeatExchanger:FluidToFluid", "heat_recovery", "fluid_heat_exchanger"},
		{"Generator:FuelCell", "generator", "fuel_cell"},
		{"Generator:FuelCell:ExhaustGasToWaterHeatExchanger", "heat_recovery", "fuel_cell_heat_recovery"},
		{"Generator:FuelSupply", "source_sink", "fuel_supply"},
		{"Controller:WaterCoil", "control_overlay", "controller"},
		{"AvailabilityManagerAssignmentList", "control_overlay", "availability_manager"},
	} {
		if !hasSystemCoupling(couplings, expected.objectType, expected.couplingType, expected.role) {
			t.Fatalf("couplings = %#v, missing %#v", couplings, expected)
		}
	}
	if !hasEnergyNetwork(report.ServiceModel, "electric_network") {
		t.Fatalf("networks = %#v, want electric network", report.ServiceModel.Networks)
	}
	if !hasEnergyNetwork(report.ServiceModel, "service_water") {
		t.Fatalf("networks = %#v, want service water network", report.ServiceModel.Networks)
	}
}

func TestHVACServiceModelKeepsCondenserHeatRejectionOutOfZoneServices(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "CondenserLoop", Fields: []Field{
			{Value: "Condenser Loop"},
			{Value: "Water"},
			{Value: ""},
			{Value: ""},
			{Value: "Cnd Setpoint"},
			{Value: "35"},
			{Value: "5"},
			{Value: "Autosize"},
			{Value: "0"},
			{Value: "Autosize"},
			{Value: "Cnd Supply Inlet"},
			{Value: "Cnd Supply Outlet"},
			{Value: "Cnd Branches"},
			{Value: ""},
		}},
		{Index: 1, Type: "BranchList", Fields: []Field{
			{Value: "Cnd Branches"},
			{Value: "Tower Branch"},
		}},
		{Index: 2, Type: "Branch", Fields: []Field{
			{Value: "Tower Branch"},
			{Value: ""},
			{Value: "CoolingTower:SingleSpeed"},
			{Value: "Heat Rejection Tower"},
			{Value: "Cnd Supply Inlet"},
			{Value: "Cnd Supply Outlet"},
		}},
		{Index: 3, Type: "CoolingTower:SingleSpeed", Fields: []Field{{Value: "Heat Rejection Tower"}}},
	}}

	report := AnalyzeHVAC(doc)
	coupling := findSystemCoupling(report.ServiceModel.Couplings, "CoolingTower:SingleSpeed", "heat_rejection", "cooling_tower")
	if coupling == nil {
		t.Fatalf("couplings = %#v, want cooling tower heat rejection coupling", report.ServiceModel.Couplings)
	}
	if !hasConnectedLoop(*coupling, "CondenserLoop", "Condenser Loop") {
		t.Fatalf("cooling tower coupling = %#v, want connected condenser loop", coupling)
	}
	for _, zone := range report.ServiceModel.ZoneServices {
		for _, path := range zone.Paths {
			if strings.EqualFold(path.Delivery.ObjectType, "CoolingTower:SingleSpeed") {
				t.Fatalf("cooling tower leaked into zone service path: %#v", path)
			}
		}
	}
}

func TestHVACServiceModelSeparatesServiceWaterNetworkFromZoneServices(t *testing.T) {
	doc := Document{Objects: []Object{
		{Index: 0, Type: "Zone", Fields: []Field{{Value: "Office"}}},
		{Index: 1, Type: "ZoneHVAC:EquipmentConnections", Fields: []Field{
			{Value: "Office"},
			{Value: "Office Equipment"},
			{Value: "Office Supply Inlet"},
			{Value: ""},
			{Value: "Office Zone Air Node"},
			{Value: ""},
		}},
		{Index: 2, Type: "ZoneHVAC:EquipmentList", Fields: []Field{
			{Value: "Office Equipment"},
			{Value: "ZoneHVAC:PackagedTerminalAirConditioner"},
			{Value: "Office PTAC"},
			{Value: "1"},
			{Value: "1"},
		}},
		{Index: 3, Type: "ZoneHVAC:PackagedTerminalAirConditioner", Fields: []Field{
			{Value: "Office PTAC"},
			{Value: ""},
			{Value: "Autosize"},
			{Value: "Office Supply Inlet"},
			{Value: "Office Zone Air Node"},
		}},
		{Index: 4, Type: "WaterHeater:Mixed", Fields: []Field{{Value: "DHW Heater"}}},
		{Index: 5, Type: "WaterUse:Equipment", Fields: []Field{{Value: "Lavatory"}}},
		{Index: 6, Type: "WaterUse:Connections", Fields: []Field{{Value: "Lavatory Connections"}}},
	}}

	report := AnalyzeHVAC(doc)
	if !hasEnergyNetwork(report.ServiceModel, "service_water") {
		t.Fatalf("networks = %#v, want service water network", report.ServiceModel.Networks)
	}
	for _, expected := range []struct {
		objectType string
		role       string
	}{
		{"WaterHeater:Mixed", "water_heater"},
		{"WaterUse:Equipment", "water_use"},
		{"WaterUse:Connections", "water_use"},
	} {
		if !hasSystemCoupling(report.ServiceModel.Couplings, expected.objectType, "service_water", expected.role) {
			t.Fatalf("couplings = %#v, missing service water %#v", report.ServiceModel.Couplings, expected)
		}
	}
	for _, zone := range report.ServiceModel.ZoneServices {
		for _, path := range zone.Paths {
			if path.ServiceKind == "service_water" || path.PathType == "service_water" {
				t.Fatalf("service water leaked into HVAC zone service: %#v", path)
			}
			if strings.HasPrefix(normalizeFieldCatalogKey(path.Delivery.ObjectType), "water") {
				t.Fatalf("water object leaked into HVAC delivery: %#v", path)
			}
		}
	}
}

func TestHVACServiceModelSeparatesTrueAirTerminalsFromZoneEquipment(t *testing.T) {
	if !isTrueAirTerminalType("AirTerminal:SingleDuct:VAV:Reheat") {
		t.Fatal("VAV reheat air terminal was not recognized")
	}
	for _, objectType := range []string{
		"ZoneHVAC:AirDistributionUnit",
		"ZoneHVAC:FourPipeFanCoil",
		"ZoneHVAC:IdealLoadsAirSystem",
		"AirLoopHVAC:UnitarySystem",
	} {
		if isTrueAirTerminalType(objectType) {
			t.Fatalf("%s was misclassified as a true air terminal", objectType)
		}
	}
}

func TestHVACFrontendDefaultCopyHidesRuleGraphVocabulary(t *testing.T) {
	for _, path := range []string{
		"../../frontend/src/js/views/hvac-views.js",
		"../../frontend/src/js/i18n.js",
		"../../frontend/src/styles/hvac.css",
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(content)
		for _, forbidden := range []string{
			"Rule edges",
			"Rule trace",
			"Rule path",
			"Terminal / Equipment",
			"Plant / Condenser",
			"Zone relations",
			"Cross-loop",
			"hvac.inferred",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains default-HVAC forbidden copy %q", path, forbidden)
			}
		}
	}
}

func findHVACTestingZoneService(model HVACServiceModel, zoneName string) *ZoneServiceSummary {
	for index := range model.ZoneServices {
		if strings.EqualFold(model.ZoneServices[index].ZoneName, zoneName) {
			return &model.ZoneServices[index]
		}
	}
	return nil
}

func hasZoneServicePath(paths []ZoneServicePath, serviceKind string, pathType string, plantLoop string, airLoop string, deliveryNeedle string) bool {
	return findZoneServicePath(paths, serviceKind, pathType, plantLoop, airLoop, deliveryNeedle) != nil
}

func hasAnyServicePath(model HVACServiceModel, serviceKind string, pathType string, deliveryType string) bool {
	for _, summary := range model.ZoneServices {
		for _, path := range summary.Paths {
			if path.ServiceKind == serviceKind && path.PathType == pathType && path.DeliveryEquipment.DeliveryType == deliveryType {
				return true
			}
		}
	}
	return false
}

func hasServicePathWithAirLoop(model HVACServiceModel, pathType string) bool {
	for _, summary := range model.ZoneServices {
		for _, path := range summary.Paths {
			if path.PathType == pathType && path.AirLoop != nil {
				return true
			}
		}
	}
	return false
}

func findZoneServicePath(paths []ZoneServicePath, serviceKind string, pathType string, plantLoop string, airLoop string, deliveryNeedle string) *ZoneServicePath {
	for index := range paths {
		path := paths[index]
		if serviceKind != "" && path.ServiceKind != serviceKind {
			continue
		}
		if pathType != "" && path.PathType != pathType {
			continue
		}
		if plantLoop != "" {
			if path.PlantLoop == nil || !strings.EqualFold(path.PlantLoop.Name, plantLoop) {
				continue
			}
		}
		if airLoop != "" {
			if path.AirLoop == nil || !strings.EqualFold(path.AirLoop.Name, airLoop) {
				continue
			}
		}
		deliveryName := path.Delivery.ObjectName + " " + path.Delivery.DisplayName + " " + path.DeliveryEquipment.Component.ObjectName
		if deliveryNeedle != "" && !strings.Contains(strings.ToLower(deliveryName), strings.ToLower(deliveryNeedle)) {
			continue
		}
		return &paths[index]
	}
	return nil
}

type servicePathGoldenRow struct {
	ServiceKind       string `json:"serviceKind"`
	PathType          string `json:"pathType"`
	DeliveryType      string `json:"deliveryType"`
	Delivery          string `json:"delivery"`
	Source            string `json:"source,omitempty"`
	PlantLoop         string `json:"plantLoop,omitempty"`
	AirLoop           string `json:"airLoop,omitempty"`
	RefrigerantSystem string `json:"refrigerantSystem,omitempty"`
}

func servicePathGoldenRows(paths []ZoneServicePath) []servicePathGoldenRow {
	rows := make([]servicePathGoldenRow, 0, len(paths))
	for _, path := range paths {
		row := servicePathGoldenRow{
			ServiceKind:  path.ServiceKind,
			PathType:     path.PathType,
			DeliveryType: path.DeliveryEquipment.DeliveryType,
			Delivery:     path.Delivery.ObjectName,
		}
		if path.SourceSystem != nil {
			row.Source = path.SourceSystem.Name
		}
		if path.PlantLoop != nil {
			row.PlantLoop = path.PlantLoop.Name
		}
		if path.AirLoop != nil {
			row.AirLoop = path.AirLoop.Name
		}
		if path.RefrigerantSystem != nil {
			row.RefrigerantSystem = path.RefrigerantSystem.Name
		}
		rows = append(rows, row)
	}
	return rows
}

func hasSystemCoupling(couplings []SystemCoupling, objectType string, couplingType string, role string) bool {
	return findSystemCoupling(couplings, objectType, couplingType, role) != nil
}

func findSystemCoupling(couplings []SystemCoupling, objectType string, couplingType string, role string) *SystemCoupling {
	for index := range couplings {
		coupling := couplings[index]
		if strings.EqualFold(coupling.Object.ObjectType, objectType) && coupling.CouplingType == couplingType && coupling.Role == role {
			return &couplings[index]
		}
	}
	return nil
}

func findSystemCouplingByObject(couplings []SystemCoupling, objectType string, objectName string) *SystemCoupling {
	for index := range couplings {
		coupling := couplings[index]
		if strings.EqualFold(coupling.Object.ObjectType, objectType) && strings.EqualFold(coupling.Object.ObjectName, objectName) {
			return &couplings[index]
		}
	}
	return nil
}

func hasLoopRef(refs []LoopRef, loopType string, loopName string) bool {
	for _, ref := range refs {
		if strings.EqualFold(ref.Type, loopType) && strings.EqualFold(ref.Name, loopName) {
			return true
		}
	}
	return false
}

func hasConnectedLoop(coupling SystemCoupling, loopType string, loopName string) bool {
	return hasLoopRef(coupling.ConnectedLoops, loopType, loopName)
}

func componentRefsContainObject(refs []ComponentRef, objectType string, objectName string) bool {
	for _, ref := range refs {
		if strings.EqualFold(ref.ObjectType, objectType) && strings.EqualFold(ref.ObjectName, objectName) {
			return true
		}
	}
	return false
}

func hasEnergyNetwork(model HVACServiceModel, networkType string) bool {
	for _, network := range model.Networks {
		if network.NetworkType == networkType {
			return true
		}
	}
	return false
}

func findNavigationEntity(entities []HVACNavigationEntity, id string, kind string) *HVACNavigationEntity {
	for index := range entities {
		if entities[index].ID == id && (kind == "" || entities[index].Kind == kind) {
			return &entities[index]
		}
	}
	return nil
}

func findNavigationLink(links []HVACNavigationLink, fromID string, toID string, kind string) *HVACNavigationLink {
	for index := range links {
		if links[index].FromID == fromID && links[index].ToID == toID && links[index].Kind == kind {
			return &links[index]
		}
	}
	return nil
}

func findSystemSummary(systems []SystemSummary, systemType string, name string) *SystemSummary {
	for index := range systems {
		if strings.EqualFold(systems[index].Type, systemType) && strings.EqualFold(systems[index].Name, name) {
			return &systems[index]
		}
	}
	return nil
}

func findComponentIndexItem(items []ComponentIndexItem, objectType string, objectName string) *ComponentIndexItem {
	for index := range items {
		component := items[index].Component
		if strings.EqualFold(component.ObjectType, objectType) && strings.EqualFold(component.ObjectName, objectName) {
			return &items[index]
		}
	}
	return nil
}
