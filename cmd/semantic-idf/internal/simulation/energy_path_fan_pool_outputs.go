package simulation

// Native AirLoop fan observations for the Energy Path allocation reader.
import (
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const energyPathAirLoopFanOutputName = "Air System Fan Electricity Energy"

// This is a request gate, not an allocation proof. Start with independently
// named AirLoops containing directly resolved supply-branch fans and an actual
// supply-terminal/Zone connection. The finite native HeatOnly extension uses
// an exported resolved child plus an actual heating service path, not a guess.
// SQL completeness, meter closure and eligible recipients remain reader gates.
func energyPathAirLoopFanOutputKeys(doc idf.Document) []string {
	counts, fanNames := map[string]int{}, map[string]int{}
	identity := func(kind, name string) string {
		return normalizePurposeToken(kind) + "|" + normalizePurposeToken(name)
	}
	for _, object := range doc.Objects {
		name := purposeObjectName(object)
		counts[identity(object.Type, name)]++
		if strings.HasPrefix(normalizePurposeToken(object.Type), "fan:") {
			fanNames[normalizePurposeToken(name)]++
		}
	}
	report := idf.AnalyzeHVAC(doc)
	completeFurnaces := map[int]bool{}
	for _, cohort := range idf.ResolveNativeHeatOnlyFurnaceCohorts(doc, report.ServiceModel.ZoneServices) {
		completeFurnaces[cohort.System.ObjectIndex] = cohort.Complete
	}
	// The same fan cannot authorize two loops or two branch occurrences.
	occurrences := map[string]int{}
	for _, loop := range report.Loops {
		for _, side := range []idf.HVACLoopSide{loop.SupplySide, loop.DemandSide} {
			for _, branch := range side.Branches {
				for _, component := range branch.Components {
					if strings.HasPrefix(normalizePurposeToken(component.ObjectType), "fan:") {
						occurrences[normalizePurposeToken(component.ObjectName)]++
					}
				}
			}
		}
	}
	var out []string
	coveredFans := map[string]bool{}
	for _, loop := range report.Loops {
		if !strings.EqualFold(loop.Type, "AirLoopHVAC") || strings.TrimSpace(loop.Name) == "" || loop.Name == "*" || counts[identity(loop.Type, loop.Name)] != 1 {
			continue
		}
		if len(loop.SupplySide.Branches) == 0 || len(loop.SupplySide.MissingBranchNames) != 0 || counts[identity("BranchList", loop.SupplySide.BranchListName)] != 1 {
			continue
		}
		valid, fans := true, 0
		for _, branch := range loop.SupplySide.Branches {
			if counts[identity("Branch", branch.Name)] != 1 {
				valid = false
			}
			for _, component := range branch.Components {
				if !component.Exists || counts[identity(component.ObjectType, component.ObjectName)] != 1 {
					valid = false
				}
				if strings.EqualFold(component.ObjectType, "AirLoopHVAC:Unitary:Furnace:HeatOnly") {
					fanName, owned := energyPathHeatOnlyFanOutputOwner(report, loop, component)
					if !owned || !completeFurnaces[component.ObjectIndex] || fanNames[normalizePurposeToken(fanName)] != 1 {
						valid = false
					} else {
						fans++
					}
					continue
				}
				if !strings.HasPrefix(normalizePurposeToken(component.ObjectType), "fan:") {
					continue
				}
				fans++
				fanKey := normalizePurposeToken(component.ObjectName)
				if fanNames[fanKey] != 1 || occurrences[fanKey] != 1 || component.InletNode == "" || component.OutletNode == "" || strings.EqualFold(component.ObjectType, "Fan:ZoneExhaust") {
					valid = false
				}
			}
		}
		if !valid || fans == 0 {
			continue
		}
		// Return-only coincidence is not a supplied Zone. Check the exact loop's
		// supply-path nodes against a resolved terminal reaching a real inlet.
		supplyNodes := map[string]bool{}
		for _, node := range loop.DemandGraph.Nodes {
			if node.PathType == "supply_path" && strings.TrimSpace(node.NodeName) != "" {
				supplyNodes[normalizePurposeToken(node.NodeName)] = true
			}
		}
		served := false
		for _, relation := range report.ZoneRelations {
			if counts[identity("Zone", relation.ZoneName)] != 1 {
				continue
			}
			for _, terminal := range relation.TerminalUnits {
				if terminal.Exists && terminal.OutletMatchesZoneInlet && counts[identity(terminal.ObjectType, terminal.ObjectName)] == 1 && supplyNodes[normalizePurposeToken(terminal.InletNode)] {
					served = true
				}
			}
		}
		if served {
			out = append(out, loop.Name)
			for _, branch := range loop.SupplySide.Branches {
				for _, component := range branch.Components {
					if strings.HasPrefix(normalizePurposeToken(component.ObjectType), "fan:") {
						coveredFans[normalizePurposeToken(component.ObjectName)] = true
					} else if fanName, owned := energyPathHeatOnlyFanOutputOwner(report, loop, component); owned {
						coveredFans[normalizePurposeToken(fanName)] = true
					}
				}
			}
		}
	}
	// A partial AirLoop census cannot partition the whole Fans meter. Do not
	// add heavy observations for mixed local/nested/exhaust or orphan fans;
	// those models keep their observed Building budget explicitly unassigned.
	// Existing user requests are untouched, and SQL closure remains mandatory.
	for name, count := range fanNames {
		if count != 1 || !coveredFans[name] {
			return nil
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

func (builder *purposePlanBuilder) addEnergyPathFanPoolOutputs() {
	keys := energyPathAirLoopFanOutputKeys(builder.doc)
	if len(keys) == 0 {
		return
	}
	// Keep native pools Building-wide even in a selected-Zone run: the reader
	// partitions the complete Fans meter before projecting the selected scope.
	// Reuse only unfiltered Hourly requests. Monthly, rate, physical-fan-key and
	// schedule-filtered outputs cannot satisfy the native hourly pool contract.
	covers := func(output PurposeOutputObject) bool {
		return strings.EqualFold(output.ObjectType, "Output:Variable") &&
			strings.EqualFold(output.VariableName, energyPathAirLoopFanOutputName) &&
			strings.EqualFold(output.ReportingFrequency, "Hourly") &&
			strings.TrimSpace(purposeFieldValue(output.Fields, "Schedule Name")) == ""
	}
	reuse := func(key string) bool {
		for index := range builder.objects {
			output := &builder.objects[index]
			if covers(*output) && strings.EqualFold(strings.TrimSpace(output.KeyValue), key) {
				output.PurposeIDs = normalizePurposeIDs(append(output.PurposeIDs, SimulationPurposeBasicEnergy))
				output.Reason = "Basic Energy Path"
				return true
			}
		}
		// Stable choice when literal declarations have equivalent identities,
		// for example omitted versus explicitly blank schedule fields.
		var matches []PurposeOutputObject
		for _, output := range builder.existing {
			if covers(output) && strings.EqualFold(strings.TrimSpace(output.KeyValue), key) {
				matches = append(matches, output)
			}
		}
		if len(matches) == 0 {
			return false
		}
		sort.Slice(matches, func(i, j int) bool { return matches[i].Signature < matches[j].Signature })
		output := matches[0]
		output.PurposeIDs = []SimulationPurposeID{SimulationPurposeBasicEnergy}
		output.Reason = "Basic Energy Path"
		builder.addObject(output)
		return true
	}
	// EnergyPlus treats a blank native Key Value as the wildcard default.
	// Preserve that literal declaration and its original navigation index.
	if reuse("*") || reuse("") {
		return
	}
	for _, key := range keys {
		if reuse(key) {
			continue
		}
		builder.addVariableWithReason(SimulationPurposeBasicEnergy, key, energyPathAirLoopFanOutputName, "Hourly", "heavy",
			"Native Hourly AirLoop fan allocation evidence; independently summed to months, never an additional Building end-use budget. Observation does not prove Zone ownership or meter closure.", "Basic Energy Path")
	}
}
