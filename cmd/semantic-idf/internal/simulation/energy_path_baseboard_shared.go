package simulation

import (
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type energyPathSharedHeatingElectricTarget struct {
	Component      idf.ComponentRef
	RelatedPathIDs []string
}

func energyPathSharedHeatingElectricTargets(doc idf.Document) []energyPathSharedHeatingElectricTarget {
	return energyPathBaseboardPlantTargets(doc, "Boiler:HotWater", "PlantLoop", "heating", 10, 11)
}

func energyPathBaseboardHeatRejectionTargets(doc idf.Document) []energyPathSharedHeatingElectricTarget {
	return energyPathBaseboardPlantTargets(doc, "CoolingTower:SingleSpeed", "CondenserLoop", "cooling", 1, 2)
}

// This bounded mixed-system capture does not add outputs to other accepted
// fixtures. A native consuming component may have an unknown recipient roster;
// retain that target with no paths so the reader can report it unassigned.
func energyPathBaseboardPlantTargets(doc idf.Document, objectType, loopType, service string, inletField, outletField int) []energyPathSharedHeatingElectricTarget {
	if !energyPathHasNativeBaseboard(doc) {
		return nil
	}
	index := newEnergyPathBaseboardOriginalIndex(doc)
	report := idf.AnalyzeHVAC(doc)
	condenserByPlant := energyPathCondenserLoopsByPlant(report.Loops)
	var targets []energyPathSharedHeatingElectricTarget
	for _, object := range doc.Objects {
		if !strings.EqualFold(strings.TrimSpace(object.Type), objectType) {
			continue
		}
		name := energyPathBaseboardField(object, 0)
		key := energyPathDirectHVACComponentKey(object.Type, name)
		if _, unique := index.unique(object.Type, name); !unique {
			continue
		}
		// The SQL name/key cannot distinguish another native variant with the
		// same name, including an unsupported steam boiler or tower variant.
		family := "boiler:"
		if strings.HasPrefix(strings.ToLower(objectType), "coolingtower:") {
			family = "coolingtower:"
		}
		nameCount := 0
		for _, peer := range doc.Objects {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(peer.Type)), family) && strings.EqualFold(energyPathBaseboardField(peer, 0), name) {
				nameCount++
			}
		}
		if nameCount != 1 {
			continue
		}
		inlet, outlet := energyPathBaseboardField(object, inletField), energyPathBaseboardField(object, outletField)
		if inlet == "" || outlet == "" || strings.EqualFold(inlet, outlet) {
			continue
		}
		branchCount, valid := 0, true
		branchName := ""
		for _, reference := range index.references[key] {
			owner := doc.Objects[reference.object]
			ownerName := energyPathBaseboardField(owner, 0)
			if _, unique := index.unique(owner.Type, ownerName); !unique {
				valid = false
			}
			switch strings.ToLower(strings.TrimSpace(owner.Type)) {
			case "branch":
				branchCount++
				branchName = ownerName
				valid = valid && reference.field >= 2 &&
					strings.EqualFold(energyPathBaseboardField(owner, reference.field+2), inlet) && strings.EqualFold(energyPathBaseboardField(owner, reference.field+3), outlet)
			case "plantequipmentlist", "condenserequipmentlist":
				valid = valid && reference.field >= 1 && (reference.field-1)%2 == 0
			default:
				valid = false
			}
		}
		if !valid || branchCount != 1 {
			continue
		}
		var ownerLoop idf.HVACLoop
		occurrences := 0
		for _, loop := range report.Loops {
			for _, side := range []idf.HVACLoopSide{loop.SupplySide, loop.DemandSide} {
				for _, branch := range side.Branches {
					for _, component := range branch.Components {
						if energyPathDirectHVACComponentKey(component.ObjectType, component.ObjectName) != key {
							continue
						}
						occurrences++
						_, loopUnique := index.unique(loop.Type, loop.Name)
						valid = valid && loopUnique && strings.EqualFold(loop.Type, loopType) && strings.EqualFold(side.Name, "supply") &&
							strings.EqualFold(branch.Name, branchName) && component.Exists && component.ObjectIndex == object.Index &&
							strings.EqualFold(component.InletNode, inlet) && strings.EqualFold(component.OutletNode, outlet)
						ownerLoop = loop
					}
				}
			}
		}
		if !valid || occurrences != 1 {
			continue
		}
		// AnalyzeHVAC may normalize repeated BranchList names. Prove the
		// original branch has exactly one list/loop owner before using paths.
		branchList, listUnique := index.unique("BranchList", ownerLoop.SupplySide.BranchListName)
		branchReferences, listReferences := 0, 0
		for _, original := range doc.Objects {
			if strings.EqualFold(original.Type, "BranchList") {
				for field := 1; field < len(original.Fields); field++ {
					if strings.EqualFold(energyPathBaseboardField(original, field), branchName) {
						branchReferences++
						valid = valid && original.Index == branchList.Index
					}
				}
			}
			if strings.EqualFold(original.Type, "PlantLoop") || strings.EqualFold(original.Type, "CondenserLoop") {
				for _, field := range []int{12, 16} {
					if strings.EqualFold(energyPathBaseboardField(original, field), ownerLoop.SupplySide.BranchListName) {
						listReferences++
						valid = valid && original.Index == ownerLoop.ObjectIndex && field == 12
					}
				}
			}
		}
		if !valid || !listUnique || branchReferences != 1 || listReferences != 1 {
			continue
		}
		target := energyPathSharedHeatingElectricTarget{Component: energyPathBaseboardComponentRef(object)}
		for _, summary := range report.ServiceModel.ZoneServices {
			for _, path := range summary.Paths {
				if path.ID == "" || energyCanonicalServiceKind(path.ServiceKind) != service || path.SpaceName != "" {
					continue
				}
				zoneName := firstNonEmpty(path.ZoneName, path.ServedSubject.ZoneName, summary.ZoneName)
				_, zoneUnique := index.unique("Zone", zoneName)
				_, deliveryUnique := index.unique(path.Delivery.ObjectType, path.Delivery.ObjectName)
				if !zoneUnique || !deliveryUnique {
					continue
				}
				connected := path.PlantLoop != nil && strings.EqualFold(path.PlantLoop.Type, ownerLoop.Type) && strings.EqualFold(path.PlantLoop.Name, ownerLoop.Name)
				if loopType == "CondenserLoop" {
					connected = path.CondenserLoop != nil && strings.EqualFold(path.CondenserLoop.Name, ownerLoop.Name)
					if !connected && path.PlantLoop != nil {
						peers := condenserByPlant[normalizePurposeToken(path.PlantLoop.Name)]
						connected = len(peers) == 1 && strings.EqualFold(peers[0], ownerLoop.Name)
					}
				}
				if connected {
					target.RelatedPathIDs = appendUniqueStrings(target.RelatedPathIDs, path.ID)
				}
			}
		}
		sort.Strings(target.RelatedPathIDs)
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		return normalizePurposeToken(targets[i].Component.ObjectName) < normalizePurposeToken(targets[j].Component.ObjectName)
	})
	return targets
}

func (builder *purposePlanBuilder) addEnergyPathBaseboardSharedOutputs() {
	for _, target := range energyPathSharedHeatingElectricTargets(builder.doc) {
		for _, name := range []string{"Boiler Ancillary Electricity Energy", "Boiler Ancillary Electricity Rate"} {
			builder.addVariableWithReasonAndScopeZone(SimulationPurposeBasicEnergy, target.Component.ObjectName, name, "Monthly", "medium",
				"Native central boiler ancillary consumption, a separate Heating:Electricity constituent; exact plant recipients or explicit unassigned context, never a local baseboard subtotal.", "Basic Energy Path", "")
		}
	}
	if len(energyPathBaseboardHeatRejectionTargets(builder.doc)) > 0 {
		builder.addObject(PurposeOutputObject{ObjectType: "Output:Meter", Fields: []idf.OutputFieldValue{{Name: "Key Name", Value: "HeatRejection:Electricity"}, {Name: "Reporting Frequency", Value: "Monthly"}},
			PurposeIDs: []SimulationPurposeID{SimulationPurposeBasicEnergy}, Weight: "medium", Reason: "Basic Energy Path", Description: "Native heat-rejection electricity of the validated cooling tower; not a facility residual or additional overlapping subcategory."})
	}
}
