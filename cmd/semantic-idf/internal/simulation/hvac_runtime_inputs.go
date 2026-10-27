package simulation

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// These inputs explain a native manager's observed action. They are not a
// replacement implementation of its timer, adaptive history or solver logic.
type hvacRuntimeReasonInput struct {
	role, parameter, key, variable string
}

// Tank thermostat quantities are distinct from plant dispatch and fluid-port
// temperatures. Native tank temperatures and duties below are reported interval
// quantities; they never reconstruct the solver's internal hysteresis state.
func hvacRuntimeWaterHeaterObservations(component idf.HVACOperatingSource, doc *idf.Document) []HVACControlSignalBinding {
	if doc == nil {
		return nil
	}
	var object *idf.Object
	for i := range doc.Objects {
		candidate := &doc.Objects[i]
		if candidate.Index == component.ObjectIndex && strings.EqualFold(candidate.Type, component.ObjectType) && len(candidate.Fields) > 0 && strings.EqualFold(strings.TrimSpace(candidate.Fields[0].Value), component.ObjectName) {
			if object != nil {
				return nil
			}
			object = candidate
		}
	}
	if object == nil {
		return nil
	}
	field := func(index int) string {
		if index >= len(object.Fields) {
			return ""
		}
		return strings.TrimSpace(object.Fields[index].Value)
	}
	var observations []HVACControlSignalBinding
	add := func(role, domain, parameter, key, variable string) {
		if key == "" {
			return
		}
		observations = append(observations, HVACControlSignalBinding{Graph: "onOff", NodeID: "demand", Role: role, Domain: domain, Source: component, ParameterName: parameter, KeyValue: key, VariableName: variable})
	}
	target := func(index int, name string) {
		add("setpoint", "temperature_schedule", name, field(index), "Schedule Value")
	}
	switch strings.ToLower(object.Type) {
	case "waterheater:mixed":
		add("sensor", "continuous", "Tank temperature (interval average)", component.ObjectName, "Water Heater Tank Temperature")
		target(2, "Setpoint Temperature Schedule Name")
	case "waterheater:stratified":
		target(8, "Heater 1 Setpoint Temperature Schedule Name")
		target(12, "Heater 2 Setpoint Temperature Schedule Name")
		for _, number := range hvacRuntimeStratifiedTemperatureNodes(*object) {
			add("sensor", "continuous", "Tank temperature node (interval average)", component.ObjectName, fmt.Sprintf("Water Heater Temperature Node %d", number))
		}
	case "waterheater:heatpump:pumpedcondenser", "waterheater:heatpump:wrappedcondenser":
		typeIndex, nameIndex := 16, 17
		if strings.EqualFold(object.Type, "WaterHeater:HeatPump:WrappedCondenser") {
			typeIndex, nameIndex = 15, 16
		}
		tankType, tankName := field(typeIndex), field(nameIndex)
		if !strings.EqualFold(tankType, "WaterHeater:Mixed") && !strings.EqualFold(tankType, "WaterHeater:Stratified") {
			return nil
		}
		matches := 0
		for _, tank := range doc.Objects {
			if strings.EqualFold(tank.Type, tankType) && len(tank.Fields) > 0 && strings.EqualFold(strings.TrimSpace(tank.Fields[0].Value), tankName) {
				matches++
			}
		}
		if matches != 1 {
			return nil
		}
		// SetupWaterHeaterOutputVars registers these with HPWH.Name, unlike
		// backup-element tank temperature/runtime outputs owned by Tank.Name.
		add("sensor", "continuous", "Heat pump control tank temperature (interval average)", component.ObjectName, "Water Heater Heat Pump Control Tank Temperature")
		target(2, "Compressor Setpoint Temperature Schedule Name")
	}
	return observations
}

// Native SetupStratifiedNodes divides vertical/Other tanks into equal-height
// nodes numbered top-down, then assigns each element to (lower, upper]. Reuse
// that exact simple geometry only when its numeric inputs are declared. The
// horizontal solver uses a nonlinear geometry; retain its native node identities
// instead of assigning an approximate heater sensor or substituting tank mean.
func hvacRuntimeStratifiedTemperatureNodes(object idf.Object) []int {
	field := func(index int) string {
		if index >= len(object.Fields) {
			return ""
		}
		return strings.TrimSpace(object.Fields[index].Value)
	}
	count := 1 // Native 24.2 IDD default for Number of Nodes.
	if value := field(51); value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil || parsed < 1 || parsed > 12 || math.Trunc(parsed) != parsed {
			return nil
		}
		count = int(parsed)
	}
	all := make([]int, count)
	for i := range all {
		all[i] = i + 1
	}
	shape := field(4)
	if shape != "" && !strings.EqualFold(shape, "VerticalCylinder") && !strings.EqualFold(shape, "Other") {
		return all
	}
	height, err := strconv.ParseFloat(field(3), 64)
	if err != nil || height <= 0 || math.IsNaN(height) || math.IsInf(height, 0) {
		return all
	}
	selected := map[int]bool{}
	for _, index := range []int{11, 15} {
		value := field(index)
		if value == "" {
			value = "0"
		} // Native numeric blank height is zero.
		position, err := strconv.ParseFloat(value, 64)
		if err != nil || position < 0 || position > height || math.IsNaN(position) || math.IsInf(position, 0) {
			return all
		}
		upper := height
		for number := 1; number <= count; number++ {
			lower := upper - height/float64(count)
			if number == count {
				lower = -1
			}
			if position <= upper && position > lower {
				selected[number] = true
				break
			}
			upper = lower
		}
	}
	if len(selected) == 0 {
		return all
	}
	var nodes []int
	for number := 1; number <= count; number++ {
		if selected[number] {
			nodes = append(nodes, number)
		}
	}
	return nodes
}

func hvacRuntimeHasPlantDispatch(profile idf.HVACOperatingProfile, loop idf.HVACLoop, component idf.HVACOperatingSource) bool {
	if !hvacWaterLoop(loop.Type) || profile.LoopID != loop.ID {
		return false
	}
	declared := false
	for _, node := range profile.OnOff.Nodes {
		declared = declared || node.Code == "plant_dispatch"
	}
	if !declared {
		return false
	}
	matches := 0
	for _, branch := range loop.SupplySide.Branches {
		for _, candidate := range branch.Components {
			if candidate.ObjectIndex == component.ObjectIndex && strings.EqualFold(candidate.ObjectType, component.ObjectType) && strings.EqualFold(candidate.ObjectName, component.ObjectName) {
				matches++
			}
		}
	}
	return matches == 1
}

func hvacRuntimeFollowSystemNodeInput(node idf.HVACControlNode, graph idf.HVACControlSequence, doc *idf.Document) (idf.HVACControlParameter, string) {
	reference, _ := hvacRuntimeParameter(node.Parameters, "Reference Node Name")
	var temperatureType string
	for _, source := range graph.Nodes {
		if source.Code == "setpoint_manager" && source.Source == node.Source {
			if p, ok := hvacRuntimeParameter(source.Parameters, "Reference Temperature Type"); ok {
				temperatureType = p.Value
			}
		}
	}
	if doc != nil {
		for _, obj := range doc.Objects {
			if obj.Index != node.Source.ObjectIndex || !strings.EqualFold(obj.Type, node.Source.ObjectType) || len(obj.Fields) == 0 || !strings.EqualFold(obj.Fields[0].Value, node.Source.ObjectName) {
				continue
			}
			if len(obj.Fields) > 2 {
				reference = idf.HVACControlParameter{Name: "Reference Node Name", Value: strings.TrimSpace(obj.Fields[2].Value), ObjectIndex: obj.Index, FieldIndex: 2}
			}
			if len(obj.Fields) > 3 {
				temperatureType = strings.TrimSpace(obj.Fields[3].Value)
			}
			break
		}
	}
	switch strings.ToLower(strings.TrimSpace(temperatureType)) {
	case "", "nodedrybulb": // Native default: NodeDryBulb.
		return reference, "System Node Temperature"
	case "nodewetbulb":
		return reference, "System Node Wetbulb Temperature"
	default:
		return reference, "" // Required unsupported binding; never guess.
	}
}

func hvacRuntimeReasonInputs(node idf.HVACControlNode, loop idf.HVACLoop, doc *idf.Document) []hvacRuntimeReasonInput {
	var out []hvacRuntimeReasonInput
	add := func(role, field, key, variable string) {
		if strings.TrimSpace(key) != "" {
			out = append(out, hvacRuntimeReasonInput{role, field, key, variable})
		}
	}
	parameter := func(index int, name string) idf.HVACControlParameter {
		if p, ok := hvacRuntimeParameter(node.Parameters, name); ok {
			return p
		}
		// Older catalogs may leave these native fields unnamed. Preserve the
		// exact source reference and verified native position in that case.
		if doc != nil {
			for _, obj := range doc.Objects {
				if obj.Index == node.Source.ObjectIndex && strings.EqualFold(obj.Type, node.Source.ObjectType) && len(obj.Fields) > index && len(obj.Fields) > 0 && strings.EqualFold(obj.Fields[0].Value, node.Source.ObjectName) {
					return idf.HVACControlParameter{Name: name, Value: strings.TrimSpace(obj.Fields[index].Value), ObjectIndex: obj.Index, FieldIndex: index}
				}
			}
		}
		return idf.HVACControlParameter{Name: name}
	}
	zones := func(field string, names []string, heat, cool, control bool) {
		for _, zone := range names {
			add("sensor", field, zone, "Zone Thermostat Air Temperature")
			if control {
				add("sensor", field, zone, "Zone Thermostat Control Type")
			}
			if heat {
				add("setpoint", field, zone, "Zone Thermostat Heating Setpoint Temperature")
			}
			if cool {
				add("setpoint", field, zone, "Zone Thermostat Cooling Setpoint Temperature")
			}
		}
	}
	explicitZones := func(index int, name string, heat, cool, control bool) {
		p := parameter(index, name)
		zones(p.Name, hvacRuntimeZones(doc, p.Value), heat, cool, control)
	}
	typ := strings.ToLower(node.Source.ObjectType)
	if node.Code == "terminal_load_and_operating_mode" {
		switch typ {
		case "airconditioner:variablerefrigerantflow", "airconditioner:variablerefrigerantflow:fluidtemperaturecontrol", "airconditioner:variablerefrigerantflow:fluidtemperaturecontrol:hr":
			add("demand", "Terminal cooling request", node.Source.ObjectName, "VRF Heat Pump Terminal Unit Cooling Load Rate")
			add("demand", "Terminal heating request", node.Source.ObjectName, "VRF Heat Pump Terminal Unit Heating Load Rate")
		}
	}
	if node.Code == "zone_thermostat" {
		p := parameter(1, "Zone or ZoneList Name")
		names := hvacRuntimeZones(doc, p.Value)
		zones(p.Name, names, true, true, true)
		for _, name := range names {
			// These are the zone's requests before individual equipment
			// priority/dispatch, not the selected coil's assigned load.
			add("demand", p.Name, name, "Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate")
			add("demand", p.Name, name, "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate")
		}
	}
	if node.Code == "plant_dispatch" && hvacWaterLoop(loop.Type) {
		switch typ {
		case "plantequipmentoperation:heatingload":
			add("demand", "Loop heating demand", loop.Name, "Plant Supply Side Heating Demand Rate")
		case "plantequipmentoperation:coolingload":
			add("demand", "Loop cooling demand", loop.Name, "Plant Supply Side Cooling Demand Rate")
		case "plantequipmentoperation:componentsetpoint":
			if p, ok := hvacRuntimeParameter(node.Parameters, "Demand Calculation Node Name"); ok {
				add("sensor", p.Name, p.Value, "System Node Temperature")
				add("sensor", p.Name, p.Value, "System Node Mass Flow Rate")
			}
			if p, ok := hvacRuntimeParameter(node.Parameters, "Setpoint Node Name"); ok {
				scheme, _ := hvacRuntimeParameter(node.Parameters, "Plant Loop Demand Calculation Scheme")
				if strings.EqualFold(scheme.Value, "DualSetpointDeadband") {
					operation, _ := hvacRuntimeParameter(node.Parameters, "Operation Type")
					if strings.EqualFold(operation.Value, "Cooling") || strings.EqualFold(operation.Value, "Dual") {
						add("setpoint", p.Name, p.Value, "System Node Setpoint High Temperature")
					}
					if strings.EqualFold(operation.Value, "Heating") || strings.EqualFold(operation.Value, "Dual") {
						add("setpoint", p.Name, p.Value, "System Node Setpoint Low Temperature")
					}
				} else {
					add("setpoint", p.Name, p.Value, "System Node Setpoint Temperature")
				}
			}
		}
	}
	if strings.HasPrefix(typ, "plantequipmentoperation:outdoor") {
		variable := map[string]string{
			"plantequipmentoperation:outdoordrybulb":            "Site Outdoor Air Drybulb Temperature",
			"plantequipmentoperation:outdoorwetbulb":            "Site Outdoor Air Wetbulb Temperature",
			"plantequipmentoperation:outdoorrelativehumidity":   "Site Outdoor Air Relative Humidity",
			"plantequipmentoperation:outdoordewpoint":           "Site Outdoor Air Dewpoint Temperature",
			"plantequipmentoperation:outdoordrybulbdifference":  "Site Outdoor Air Drybulb Temperature",
			"plantequipmentoperation:outdoorwetbulbdifference":  "Site Outdoor Air Wetbulb Temperature",
			"plantequipmentoperation:outdoordewpointdifference": "Site Outdoor Air Dewpoint Temperature",
		}[typ]
		if variable != "" {
			add("sensor", "Outdoor dispatch condition", "Environment", variable)
			if strings.HasSuffix(typ, "difference") {
				p := parameter(1, "Reference Temperature Node Name")
				add("sensor", p.Name, p.Value, "System Node Temperature")
			}
		}
	}
	if node.Code == "availability_manager" {
		for _, p := range node.Parameters {
			if strings.Contains(strings.ToLower(p.Name), "node") {
				add("sensor", p.Name, p.Value, "System Node Temperature")
			}
		}
		switch typ {
		case "availabilitymanager:nightcycle":
			control := strings.ToLower(parameter(3, "Control Type").Value)
			switch control {
			case "cycleonany", "cycleonanyzonefansonly":
				zones("Served zones", loop.RelatedZones, true, true, true)
			case "cycleoncontrolzone":
				explicitZones(7, "Control Zone or Zone List Name", true, true, true)
			case "cycleonanycoolingorheatingzone":
				explicitZones(8, "Cooling Control Zone or Zone List Name", false, true, true)
				explicitZones(9, "Heating Control Zone or Zone List Name", true, false, true)
				explicitZones(10, "Heating Zone Fans Only Zone or Zone List Name", true, false, true)
			case "cycleonanycoolingzone":
				explicitZones(8, "Cooling Control Zone or Zone List Name", false, true, true)
			case "cycleonanyheatingzone":
				explicitZones(9, "Heating Control Zone or Zone List Name", true, false, true)
			case "cycleonanyheatingzonefansonly":
				explicitZones(10, "Heating Zone Fans Only Zone or Zone List Name", true, false, true)
			}
		case "availabilitymanager:optimumstart":
			switch strings.ToLower(parameter(3, "Control Type").Value) {
			case "", "controlzone":
				explicitZones(4, "Control Zone Name", true, true, true)
			case "maximumofzonelist":
				explicitZones(5, "Zone List Name", true, true, true)
			}
		case "availabilitymanager:nightventilation":
			explicitZones(7, "Control Zone Name", false, false, false)
			zones("Served zones", loop.RelatedZones, false, false, false)
			add("sensor", "Outdoor air temperature", "Environment", "Site Outdoor Air Drybulb Temperature")
		case "availabilitymanager:hybridventilation":
			// Keep only declared zone inputs; its different status domain and
			// control-mode schedules must not become a generic on/off gate.
			explicitZones(2, "Control Zone Name", true, true, true)
		}
	}
	if node.Code != "setpoint_manager" {
		return out
	}
	zoneDemand := func(field string, names []string, total, heat, cool bool) {
		zones(field, names, false, false, false)
		for _, zone := range names {
			if total {
				add("demand", field, zone, "Zone Predicted Sensible Load to Setpoint Heat Transfer Rate")
			}
			if heat {
				add("demand", field, zone, "Zone Predicted Sensible Load to Heating Setpoint Heat Transfer Rate")
			}
			if cool {
				add("demand", field, zone, "Zone Predicted Sensible Load to Cooling Setpoint Heat Transfer Rate")
			}
		}
	}
	switch typ {
	case "setpointmanager:singlezone:reheat", "setpointmanager:singlezone:heating", "setpointmanager:singlezone:cooling":
		p := parameter(4, "Control Zone Name")
		reheat := typ == "setpointmanager:singlezone:reheat"
		zoneDemand(p.Name, hvacRuntimeZones(doc, p.Value), reheat, reheat || typ == "setpointmanager:singlezone:heating", reheat || typ == "setpointmanager:singlezone:cooling")
	case "setpointmanager:warmest", "setpointmanager:coldest", "setpointmanager:warmesttemperatureflow", "setpointmanager:multizone:heating:average", "setpointmanager:multizone:cooling:average":
		index := 2
		if strings.HasPrefix(typ, "setpointmanager:multizone:") {
			index = 1
		}
		// A provider naming a different loop must not borrow this occurrence's
		// zones merely because the same equipment participates in both loops.
		p := parameter(index, "HVAC Air Loop Name")
		if strings.EqualFold(p.Value, loop.Name) && p.Value != "" {
			heat := typ == "setpointmanager:coldest" || typ == "setpointmanager:multizone:heating:average"
			zoneDemand(p.Name, loop.RelatedZones, true, heat, !heat)
		}
	case "setpointmanager:outdoorairreset":
		add("sensor", "Outdoor air temperature", "Environment", "Site Outdoor Air Drybulb Temperature")
	case "setpointmanager:followoutdoorairtemperature":
		variable := "Site Outdoor Air Wetbulb Temperature"
		if strings.EqualFold(parameter(2, "Reference Temperature Type").Value, "OutdoorAirDryBulb") {
			variable = "Site Outdoor Air Drybulb Temperature"
		}
		add("sensor", "Reference Temperature Type", "Environment", variable)
	case "setpointmanager:followgroundtemperature":
		variable := map[string]string{"site:groundtemperature:buildingsurface": "Site Ground Temperature", "site:groundtemperature:shallow": "Site Surface Ground Temperature", "site:groundtemperature:deep": "Site Deep Ground Temperature", "site:groundtemperature:fcfactormethod": "Site Simple Factor Model Ground Temperature"}[strings.ToLower(parameter(2, "Reference Ground Temperature Object Type").Value)]
		if variable != "" {
			add("sensor", "Reference Ground Temperature Object Type", "Environment", variable)
		}
	}
	return out
}
