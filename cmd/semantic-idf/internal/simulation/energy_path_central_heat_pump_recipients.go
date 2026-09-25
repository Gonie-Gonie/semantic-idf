package simulation

import (
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Analyze once per original context. The IDF gate independently qualified the
// complete native air/water roster before emitting these source-local paths.
// A partial emitted cohort remains wholly unassigned, never renormalized.
func energyPathCentralHeatPumpRecipientPaths(doc idf.Document, targets []energyPathCentralHeatPumpOutputTarget) map[string][]string {
	if len(targets) == 0 {
		return nil
	}
	report := idf.AnalyzeHVAC(doc)
	return energyPathCentralHeatPumpBindRecipientPaths(doc, targets, report.ServiceModel.ZoneServices)
}

func energyPathCentralHeatPumpBindRecipientPaths(doc idf.Document, targets []energyPathCentralHeatPumpOutputTarget, summaries []idf.ZoneServiceSummary) map[string][]string {
	out := map[string][]string{}
	for _, target := range targets {
		if target.Definition.Role != energyPathCentralHeatPumpPurchased {
			continue
		}
		service := target.Definition.ServiceKind
		port := target.Binding.Cooling
		if service == "heating" {
			port = target.Binding.Heating
		}
		if _, ok := idf.NativeCentralHeatPumpServicePort(target.Binding, service, port.Loop.Type, port.Loop.Name); !ok {
			continue
		}
		var paths []string
		zones, terminals, ids := map[string]bool{}, map[int]bool{}, map[string]bool{}
		airIndex, valid := -1, true
		for _, summary := range summaries {
			for _, path := range summary.Paths {
				if path.SourceSystem == nil || !strings.EqualFold(path.SourceSystem.Name, target.System.ObjectType+" "+target.System.ObjectName) || path.ServiceKind != service {
					continue
				}
				zone := normalizePurposeToken(path.ZoneName)
				if path.ID == "" || ids[path.ID] || zone == "" || zones[zone] || terminals[path.Delivery.ObjectIndex] || path.SpaceName != "" || path.SourceSystem.Type != "source" || path.PathType != "central_air_with_plant" || path.PlantLoop == nil || path.PlantLoop.ObjectIndex != port.Loop.ObjectIndex || !strings.EqualFold(path.PlantLoop.Name, port.Loop.Name) || path.PlantLoop.Type != "PlantLoop" || path.AirLoop == nil || path.AirLoop.Type != "AirLoopHVAC" || path.CondenserLoop != nil || path.RefrigerantSystem != nil || path.DeliveryWrapper == nil || path.Delivery.ObjectType != "AirTerminal:SingleDuct:ConstantVolume:Reheat" || len(path.Conditioning) != 1 {
					valid = false
					continue
				}
				if airIndex >= 0 && airIndex != path.AirLoop.ObjectIndex {
					valid = false
				}
				airIndex = path.AirLoop.ObjectIndex
				zones[zone], terminals[path.Delivery.ObjectIndex] = true, true
				ids[path.ID] = true
				paths = append(paths, path.ID)
			}
		}
		if !valid || len(paths) == 0 || len(paths) != energyPathCentralHeatPumpSplitCount(doc, airIndex) {
			continue
		}
		sort.Strings(paths)
		out[target.Definition.ID+":"+target.System.ID] = paths
	}
	return out
}

// Census check only: ownership was proved by the native IDF path gate. This
// catches an incomplete downstream path list without inventing a Zone name.
func energyPathCentralHeatPumpSplitCount(doc idf.Document, airIndex int) int {
	f := energyPathBaseboardField
	var air idf.Object
	for _, object := range doc.Objects {
		if object.Index == airIndex && object.Type == "AirLoopHVAC" {
			air = object
		}
	}
	var supply idf.Object
	count := 0
	for _, object := range doc.Objects {
		if object.Type == "AirLoopHVAC:SupplyPath" && f(air, 8) != "" && strings.EqualFold(f(object, 1), f(air, 8)) {
			supply, count = object, count+1
		}
	}
	if count != 1 || len(supply.Fields) != 4 || !strings.EqualFold(f(supply, 2), "AirLoopHVAC:ZoneSplitter") {
		return 0
	}
	count, outlets := 0, 0
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, f(supply, 2)) && strings.EqualFold(f(object, 0), f(supply, 3)) {
			count, outlets = count+1, len(object.Fields)-2
		}
	}
	if count != 1 || outlets < 1 {
		return 0
	}
	return outlets
}
