package simulation

import (
	"fmt"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Native v24.2 SetupOutputVariable channels. Fan/pump power and flow are not
// PLR, and shaft/electric power is not total equipment efficiency.
func hvacOperatingVariables(component idf.HVACOperatingSource, doc *idf.Document) []string {
	typ := normalizePurposeToken(component.ObjectType)
	switch {
	case strings.HasPrefix(typ, "fan:"):
		variables := []string{"Fan Electricity Rate", "Fan Electricity Energy", "Fan Air Mass Flow Rate"}
		if typ == "fan:onoff" {
			variables = append(variables, "Fan Runtime Fraction")
		}
		return variables
	case strings.HasPrefix(typ, "pump:") || strings.HasPrefix(typ, "headeredpumps:"):
		return []string{"Pump Electricity Rate", "Pump Electricity Energy", "Pump Mass Flow Rate", "Pump Shaft Power"}
	case typ == "chiller:electric:eir" || typ == "chiller:electric:reformulatedeir":
		return []string{"Chiller Evaporator Cooling Rate", "Chiller Evaporator Cooling Energy", "Chiller Electricity Rate", "Chiller Electricity Energy", "Chiller COP", "Chiller Part Load Ratio", "Chiller Cycling Ratio"}
	case typ == "boiler:hotwater":
		variables := []string{"Boiler Heating Rate", "Boiler Heating Energy", "Boiler Ancillary Electricity Rate", "Boiler Part Load Ratio", "Boiler Efficiency"}
		if fuel := hvacOperatingBoilerFuel(component, doc); fuel != "" {
			variables = append(variables, "Boiler "+fuel+" Rate", "Boiler "+fuel+" Energy")
		}
		return variables
	}
	return nil
}

var hvacOperatingBoilerFuels = []string{"Electricity", "NaturalGas", "Propane", "FuelOilNo1", "FuelOilNo2", "Coal", "Diesel", "Gasoline", "OtherFuel1", "OtherFuel2"}

func hvacOperatingBoilerFuel(component idf.HVACOperatingSource, doc *idf.Document) string {
	if doc == nil || !strings.EqualFold(component.ObjectType, "Boiler:HotWater") {
		return ""
	}
	var fuel string
	matches := 0
	for _, object := range doc.Objects {
		if !strings.EqualFold(object.Type, component.ObjectType) || len(object.Fields) < 2 || !strings.EqualFold(strings.TrimSpace(object.Fields[0].Value), component.ObjectName) {
			continue
		}
		matches++
		if component.ObjectIndex >= 0 && object.Index != component.ObjectIndex {
			continue
		}
		for _, candidate := range hvacOperatingBoilerFuels {
			if strings.EqualFold(strings.TrimSpace(object.Fields[1].Value), candidate) {
				fuel = candidate
			}
		}
	}
	if matches != 1 {
		return ""
	}
	return fuel
}

func hvacOperatingBoilerFuelVariable(variable string) bool {
	for _, fuel := range hvacOperatingBoilerFuels {
		if strings.EqualFold(variable, "Boiler "+fuel+" Rate") || strings.EqualFold(variable, "Boiler "+fuel+" Energy") {
			return true
		}
	}
	return false
}

func hvacOperatingSignals(profile idf.HVACOperatingProfile, doc *idf.Document) []HVACControlSignalBinding {
	component := hvacRuntimeComponent(profile)
	var out []HVACControlSignalBinding
	for _, variable := range hvacOperatingVariables(component, doc) {
		if component.ObjectName == "" {
			continue
		}
		out = append(out, HVACControlSignalBinding{
			ID:             fmt.Sprintf("%d/%s/equipment/operating/%s/%s", profile.ComponentIndex, profile.LoopID, normalizePurposeToken(component.ObjectName), normalizePurposeToken(variable)),
			ComponentIndex: profile.ComponentIndex, LoopID: profile.LoopID, Graph: "equipment", Role: "operating", Domain: "continuous", Source: component,
			KeyValue: component.ObjectName, VariableName: variable, ReportingFrequency: "Timestep",
		})
	}
	return out
}

func hvacComponentReportsVariable(component idf.HVACComponent, variable string, doc *idf.Document) bool {
	if !hvacOperatingBoilerFuelVariable(variable) {
		return hvacComponentTypeReportsVariable(component.ObjectType, variable)
	}
	source := idf.HVACOperatingSource{ObjectIndex: component.ObjectIndex, ObjectType: component.ObjectType, ObjectName: component.ObjectName}
	fuel := hvacOperatingBoilerFuel(source, doc)
	return fuel != "" && (strings.EqualFold(variable, "Boiler "+fuel+" Rate") || strings.EqualFold(variable, "Boiler "+fuel+" Energy"))
}

func hvacOperatingTemplateVariables(doc idf.Document) []string {
	var variables []string
	for _, typ := range []string{"Fan:OnOff", "Pump:VariableSpeed", "Chiller:Electric:EIR", "Boiler:HotWater"} {
		for _, variable := range hvacOperatingVariables(idf.HVACOperatingSource{ObjectType: typ}, nil) {
			variables = appendUniquePurposeString(variables, variable)
		}
	}
	for _, object := range doc.Objects {
		if !strings.EqualFold(object.Type, "HVACTemplate:Plant:Boiler") || len(object.Fields) < 5 {
			continue
		}
		typ := strings.TrimSpace(object.Fields[1].Value)
		if !strings.EqualFold(typ, "HotWaterBoiler") && !strings.EqualFold(typ, "CondensingHotWaterBoiler") {
			continue
		}
		for _, fuel := range hvacOperatingBoilerFuels {
			if strings.EqualFold(strings.TrimSpace(object.Fields[4].Value), fuel) {
				variables = appendUniquePurposeString(variables, "Boiler "+fuel+" Rate")
				variables = appendUniquePurposeString(variables, "Boiler "+fuel+" Energy")
			}
		}
	}
	return variables
}

// Called after required-control validation. Elective channels never introduce
// another file or a longer time union into that validation, and absent or
// ambiguous outputs create neither a placeholder nor an acquisition error.
func attachHVACOperatingObservations(result *HVACLoopRunResult, profiles []idf.HVACOperatingProfile, doc *idf.Document, byOutput map[string][]SimulationSeries, byNativeID map[string]SimulationSeries) {
	outputKey := func(key, name, frequency string) string {
		return normalizePurposeToken(key) + "\x00" + normalizePurposeToken(name) + "\x00" + purposeControlFrequency(frequency)
	}
	pooled := map[string]string{}
	for _, series := range result.ControlSeries {
		key, name := hvacSeriesIdentity(series)
		pooled[series.SourceID] = outputKey(key, name, purposeSeriesFrequency(series))
	}
	for _, profile := range profiles {
		for _, binding := range hvacOperatingSignals(profile, doc) {
			identity := outputKey(binding.KeyValue, binding.VariableName, binding.ReportingFrequency)
			matched := byOutput[identity]
			if len(matched) != 1 {
				continue
			}
			item := matched[0]
			if item.SourceID == "" {
				item.SourceID = "hvac-control:" + normalizePurposeToken(item.File) + ":" + normalizePurposeToken(binding.KeyValue) + ":" + normalizePurposeToken(binding.VariableName) + ":" + purposeControlFrequency(binding.ReportingFrequency)
			}
			if previous, exists := pooled[item.SourceID]; exists && previous != identity {
				continue
			}
			if item.NativeIntervalsSourceID != "" {
				owner, exists := byNativeID[normalizePurposeToken(item.File)+"\x00"+item.NativeIntervalsSourceID]
				if exists && simulationPointAxisEqual(item.Points, owner.Points) {
					item.NativeIntervals = owner.NativeIntervals
				}
				item.NativeIntervalsSourceID = ""
			}
			binding.SeriesID, binding.Status = item.SourceID, "collected"
			result.ControlSignals = append(result.ControlSignals, binding)
			if _, exists := pooled[item.SourceID]; !exists {
				pooled[item.SourceID] = identity
				result.ControlSeries = append(result.ControlSeries, item)
			}
		}
	}
}
