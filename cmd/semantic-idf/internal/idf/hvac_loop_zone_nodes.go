package idf

// Carry the resolved EquipmentConnections ports into the standalone loop
// topology sent with simulation results. Splitter outlet order is not evidence
// of a zone connection (a terminal can lie between the splitter and the zone).
func attachAirLoopZoneNodes(loop *HVACLoop, relations []HVACZoneChain) {
	if loop.Type != "AirLoopHVAC" {
		return
	}
	pathNodes := airLoopDemandGraphNodeSet(loop.DemandGraph)
	for _, relation := range relations {
		if !stringSliceContainsFold(relation.AirLoopNames, loop.Name) {
			continue
		}
		inlets := map[string]bool{}
		for _, terminal := range relation.TerminalUnits {
			if pathNodes[normalizeName(terminal.InletNode)] {
				inlets[normalizeName(terminal.OutletNode)] = true
			}
		}
		for _, source := range relation.Nodes.Sources {
			role := map[string]string{"inlet_nodes": "zone_inlet", "return_nodes": "zone_return"}[source.Role]
			if role == "" {
				continue
			}
			for _, name := range source.Nodes {
				connected := pathNodes[normalizeName(name)]
				if role == "zone_inlet" {
					connected = connected || inlets[normalizeName(name)]
				}
				if !connected {
					continue
				}
				addAirLoopDemandNode(&loop.DemandGraph.Nodes, AirLoopDemandNode{
					NodeName: name, Role: role, ZoneName: relation.ZoneName,
					PathType:   map[string]string{"zone_inlet": "supply", "zone_return": "return"}[role],
					ObjectType: source.ObjectType, ObjectName: source.ObjectName,
					ObjectIndex: source.ObjectIndex, FieldIndex: source.FieldIndex, FieldName: source.Field,
				})
			}
		}
	}
	sortAirLoopDemandGraph(&loop.DemandGraph)
}
