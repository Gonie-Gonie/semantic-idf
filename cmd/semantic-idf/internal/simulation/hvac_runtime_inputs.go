package simulation

import (
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// These inputs explain a native manager's observed action. They are not a
// replacement implementation of its timer, adaptive history or solver logic.
type hvacRuntimeReasonInput struct {
	role, parameter, key, variable string
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
