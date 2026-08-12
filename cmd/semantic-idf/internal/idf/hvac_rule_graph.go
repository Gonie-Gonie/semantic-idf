package idf

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	hvacRuleLoopSideBranchList             = "loop_side.branch_list"
	hvacRuleBranchListIncludesBranch       = "branch_list.includes_branch"
	hvacRuleBranchComponentOccurrence      = "branch.component_occurrence"
	hvacRuleComponentSourceOccurrence      = "component.source_occurrence"
	hvacRuleBranchComponentSeries          = "branch.component_series"
	hvacRuleBranchComponentInletNode       = "branch.component_inlet_node"
	hvacRuleBranchComponentOutletNode      = "branch.component_outlet_node"
	hvacRuleLoopSideInletToFirstBranch     = "loop_side.inlet_to_first_branch"
	hvacRuleLoopSideLastBranchToOutlet     = "loop_side.last_branch_to_outlet"
	hvacRuleConnectorSplitterInletBranch   = "connector.splitter_inlet_branch"
	hvacRuleConnectorSplitterOutletBranch  = "connector.splitter_outlet_branch"
	hvacRuleConnectorMixerInletBranch      = "connector.mixer_inlet_branch"
	hvacRuleConnectorMixerOutletBranch     = "connector.mixer_outlet_branch"
	hvacRuleAirLoopSupplyPathComponent     = "airloop.supply_path_component_sequence"
	hvacRuleAirLoopReturnPathComponent     = "airloop.return_path_component_sequence"
	hvacRuleAirLoopZoneSplitterToTerminal  = "airloop.zone_splitter_to_terminal_inlet"
	hvacRuleAirLoopZoneMixerFromZoneReturn = "airloop.zone_mixer_from_zone_return"
	hvacRuleZoneHasEquipmentConnections    = "zone.has_equipment_connections"
	hvacRuleZoneHasEquipmentList           = "zone.has_equipment_list"
	hvacRuleZoneHasAirNode                 = "zone.has_air_node"
	hvacRuleZoneHasInletNode               = "zone.has_inlet_node"
	hvacRuleZoneHasExhaustNode             = "zone.has_exhaust_node"
	hvacRuleZoneHasReturnNode              = "zone.has_return_node"
	hvacRuleZoneEquipmentListADU           = "zone.equipment_list_contains_adu"
	hvacRuleZoneEquipmentListTerminal      = "zone.equipment_list_contains_terminal"
	hvacRuleZoneEquipmentListEquipment     = "zone.equipment_list_contains_equipment"
	hvacRuleZoneADUResolvesTerminal        = "zone.adu_resolves_terminal"
	hvacRuleZoneADUOutletMatchesInlet      = "zone.adu_outlet_matches_zone_inlet"
	hvacRuleZoneTerminalOutletMatchesADU   = "zone.terminal_outlet_matches_adu_outlet"
	hvacRuleZoneTerminalOutletMatchesInlet = "zone.terminal_outlet_matches_zone_inlet"
	hvacRuleComponentReferencesComponent   = "component.references_component"
	hvacRuleComponentServesParent          = "component.reference_serves_parent"
	hvacRuleVRFSystemTerminalList          = "vrf.system_terminal_unit_list"
	hvacRuleVRFTerminalListContains        = "vrf.terminal_unit_list_contains_terminal"
	hvacRuleRadiantEquipmentSurface        = "radiant.equipment_surface"
	hvacRuleRadiantEquipmentSurfaceGroup   = "radiant.equipment_surface_group"
	hvacRuleRadiantSurfaceGroupSurface     = "radiant.surface_group_contains_surface"
	hvacRuleRadiantSurfaceBelongsToZone    = "radiant.surface_belongs_to_zone"
	hvacRulePlantComponentOnSupplyBranch   = "plant.component_on_supply_branch"
	hvacRulePlantComponentOnDemandBranch   = "plant.component_on_demand_branch"
	hvacRuleCondenserComponentOnDemand     = "condenser.component_on_demand_branch"
	hvacRuleCondenserComponentOnSupply     = "condenser.component_on_supply_branch"
	hvacRuleCrossLoopSameWaterCoil         = "crossloop.same_water_coil_air_and_plant"
	hvacRuleCrossLoopChillerCondenser      = "crossloop.chiller_chw_to_condenser"
)

type HVACRuleGraph struct {
	Nodes []HVACRuleNode `json:"nodes,omitempty"`
	Edges []HVACRuleEdge `json:"edges,omitempty"`
}

type HVACRuleNode struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	ObjectType  string `json:"objectType,omitempty"`
	ObjectName  string `json:"objectName,omitempty"`
	ObjectIndex int    `json:"objectIndex,omitempty"`
	Label       string `json:"label"`
	Medium      string `json:"medium,omitempty"`
	Role        string `json:"role,omitempty"`
}

type HVACRuleEdge struct {
	ID                 string   `json:"id"`
	RuleID             string   `json:"ruleId"`
	FromID             string   `json:"fromId"`
	ToID               string   `json:"toId"`
	EdgeKind           string   `json:"edgeKind"`
	Medium             string   `json:"medium,omitempty"`
	SourceObjectType   string   `json:"sourceObjectType,omitempty"`
	SourceObjectName   string   `json:"sourceObjectName,omitempty"`
	SourceObjectIndex  int      `json:"sourceObjectIndex"`
	SourceFieldIndexes []int    `json:"sourceFieldIndexes,omitempty"`
	NodeNames          []string `json:"nodeNames,omitempty"`
}

type hvacRuleGraphBuilder struct {
	ctx                  *hvacContext
	nodes                map[string]HVACRuleNode
	edges                map[string]HVACRuleEdge
	branchNodeIDs        map[string]string
	componentOccurrences map[string][]hvacRuleComponentOccurrence
	loopNodeIDs          map[string]string
}

type hvacRuleComponentOccurrence struct {
	ID        string
	LoopName  string
	LoopType  string
	SideName  string
	Component HVACComponent
}

func buildHVACRuleGraph(ctx *hvacContext, loops []HVACLoop, relations []HVACZoneChain) HVACRuleGraph {
	builder := &hvacRuleGraphBuilder{
		ctx:                  ctx,
		nodes:                map[string]HVACRuleNode{},
		edges:                map[string]HVACRuleEdge{},
		branchNodeIDs:        map[string]string{},
		componentOccurrences: map[string][]hvacRuleComponentOccurrence{},
		loopNodeIDs:          map[string]string{},
	}
	for _, loop := range loops {
		builder.addLoop(loop)
	}
	for _, relation := range relations {
		builder.addZoneRelation(relation)
	}
	builder.addVRFSystemEdges()
	builder.addRadiantSurfaceEdges()
	builder.addComponentReferenceEdges()
	builder.addCrossLoopEdges()
	return builder.graph()
}

func (b *hvacRuleGraphBuilder) graph() HVACRuleGraph {
	nodes := make([]HVACRuleNode, 0, len(b.nodes))
	for _, node := range b.nodes {
		nodes = append(nodes, node)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Kind != nodes[j].Kind {
			return nodes[i].Kind < nodes[j].Kind
		}
		if nodes[i].Role != nodes[j].Role {
			return nodes[i].Role < nodes[j].Role
		}
		return nodes[i].ID < nodes[j].ID
	})
	edges := make([]HVACRuleEdge, 0, len(b.edges))
	for _, edge := range b.edges {
		edges = append(edges, edge)
	}
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].RuleID != edges[j].RuleID {
			return edges[i].RuleID < edges[j].RuleID
		}
		return edges[i].ID < edges[j].ID
	})
	return HVACRuleGraph{Nodes: nodes, Edges: edges}
}

func (b *hvacRuleGraphBuilder) addLoop(loop HVACLoop) {
	loopObj, _ := b.objectByTypeName(loop.Type, loop.Name)
	loopID := hvacRuleLoopNodeID(loop)
	b.loopNodeIDs[hvacObjectKey(loop.Type, loop.Name)] = loopID
	b.addNode(HVACRuleNode{
		ID:          loopID,
		Kind:        "loop",
		ObjectType:  loop.Type,
		ObjectName:  loop.Name,
		ObjectIndex: loop.ObjectIndex,
		Label:       firstNonEmpty(loop.Name, loop.Type),
		Medium:      hvacRuleMediumForLoop(loop.Type),
		Role:        strings.ToLower(loop.Type),
	})
	b.addLoopSide(loop, loop.SupplySide, loopObj)
	b.addLoopSide(loop, loop.DemandSide, loopObj)
	if loop.Type == "AirLoopHVAC" {
		b.addAirLoopDemandGraph(loop, loopObj)
	}
}

func (b *hvacRuleGraphBuilder) addLoopSide(loop HVACLoop, side HVACLoopSide, loopObj Object) {
	if side.Name == "" {
		return
	}
	medium := hvacRuleMediumForLoop(loop.Type)
	loopID := hvacRuleLoopNodeID(loop)
	var branchListObj Object
	var branchListID string
	if obj, ok := b.objectByTypeName("BranchList", side.BranchListName); ok {
		branchListObj = obj
		branchListID = hvacRuleObjectNodeID("path", obj.Type, objectName(obj), obj.Index, side.Name)
		b.addObjectNode(branchListID, "path", "branch_list", medium, obj)
		b.addEdge(hvacRuleLoopSideBranchList, loopID, branchListID, "reference", medium, loopObj,
			[]int{fieldIndexOfValue(loopObj, side.BranchListName, 0)}, []string{side.BranchListName})
	}

	for branchIndex, branch := range side.Branches {
		branchID := b.addBranchNode(branch, side.Name, medium)
		if branchListID != "" {
			b.addEdge(hvacRuleBranchListIncludesBranch, branchListID, branchID, "reference", medium, branchListObj,
				[]int{fieldIndexOfValue(branchListObj, branch.Name, 1)}, []string{branch.Name})
		}
		if branchIndex == 0 && side.InletNode != "" {
			nodeID := b.addNodeName(side.InletNode, medium, "loop_side_inlet")
			b.addEdge(hvacRuleLoopSideInletToFirstBranch, nodeID, branchID, "flow", medium, loopObj,
				[]int{fieldIndexOfValue(loopObj, side.InletNode, 0)}, []string{side.InletNode})
		}
		if branchIndex == len(side.Branches)-1 && side.OutletNode != "" {
			nodeID := b.addNodeName(side.OutletNode, medium, "loop_side_outlet")
			b.addEdge(hvacRuleLoopSideLastBranchToOutlet, branchID, nodeID, "flow", medium, loopObj,
				[]int{fieldIndexOfValue(loopObj, side.OutletNode, 0)}, []string{side.OutletNode})
		}
		b.addBranchComponentEdges(loop, side, branch)
	}

	for _, connector := range side.Connectors {
		b.addConnectorEdges(connector, side, medium)
	}
}

func (b *hvacRuleGraphBuilder) addBranchComponentEdges(loop HVACLoop, side HVACLoopSide, branch HVACBranch) {
	medium := hvacRuleMediumForLoop(loop.Type)
	branchID := b.addBranchNode(branch, side.Name, medium)
	branchObj, _ := b.objectByTypeName("Branch", branch.Name)
	var previousID string
	var previousComponent HVACComponent
	for componentIndex, component := range branch.Components {
		sourceID, occurrenceID := b.addComponentNodes(component, loop, side.Name, branch, componentIndex, medium)
		b.addEdge(hvacRuleComponentSourceOccurrence, sourceID, occurrenceID, "occurrence", medium, branchObj,
			[]int{component.TypeFieldIndex, component.NameFieldIndex}, []string{component.ObjectName})
		b.addEdge(hvacRuleBranchComponentOccurrence, branchID, occurrenceID, "contains", medium, branchObj,
			[]int{component.TypeFieldIndex, component.NameFieldIndex}, []string{component.ObjectName})
		if branchRuleID := hvacRuleComponentOnLoopBranch(loop.Type, side.Name); branchRuleID != "" {
			b.addEdge(branchRuleID, sourceID, hvacRuleLoopNodeID(loop), "occurs_on", medium, branchObj,
				[]int{component.TypeFieldIndex, component.NameFieldIndex}, []string{component.ObjectName})
			if branchRuleID == hvacRulePlantComponentOnDemandBranch || branchRuleID == hvacRuleCondenserComponentOnDemand {
				b.addEdge(branchRuleID, hvacRuleLoopNodeID(loop), sourceID, "serves_demand", medium, branchObj,
					[]int{component.TypeFieldIndex, component.NameFieldIndex}, []string{component.ObjectName})
			}
		}
		if component.InletNode != "" {
			nodeID := b.addNodeName(component.InletNode, medium, "component_inlet")
			b.addEdge(hvacRuleBranchComponentInletNode, nodeID, occurrenceID, "flow", medium, branchObj,
				[]int{component.InletFieldIndex}, []string{component.InletNode})
		}
		if component.OutletNode != "" {
			nodeID := b.addNodeName(component.OutletNode, medium, "component_outlet")
			b.addEdge(hvacRuleBranchComponentOutletNode, occurrenceID, nodeID, "flow", medium, branchObj,
				[]int{component.OutletFieldIndex}, []string{component.OutletNode})
		}
		if previousID != "" {
			nodeNames := []string{}
			if previousComponent.OutletNode != "" && strings.EqualFold(previousComponent.OutletNode, component.InletNode) {
				nodeNames = append(nodeNames, component.InletNode)
			}
			b.addEdge(hvacRuleBranchComponentSeries, previousID, occurrenceID, "flow", medium, branchObj,
				[]int{previousComponent.OutletFieldIndex, component.InletFieldIndex}, nodeNames)
		}
		previousID = occurrenceID
		previousComponent = component
	}
}

func (b *hvacRuleGraphBuilder) addConnectorEdges(connector HVACConnector, side HVACLoopSide, medium string) {
	connectorObj, ok := b.objectByTypeName(connector.Type, connector.Name)
	if !ok {
		return
	}
	connectorID := hvacRuleObjectNodeID("connector", connectorObj.Type, objectName(connectorObj), connectorObj.Index, side.Name)
	b.addObjectNode(connectorID, "connector", strings.ToLower(side.Name), medium, connectorObj)
	if strings.EqualFold(connector.Type, "Connector:Splitter") {
		if connector.InletBranchName != "" {
			if branchID := b.branchNodeID(connector.InletBranchName); branchID != "" {
				b.addEdge(hvacRuleConnectorSplitterInletBranch, connectorID, branchID, "reference", medium, connectorObj,
					[]int{fieldIndexOfValue(connectorObj, connector.InletBranchName, 1)}, []string{connector.InletBranchName})
			}
		}
		for _, branchName := range connector.BranchNames {
			if branchID := b.branchNodeID(branchName); branchID != "" {
				b.addEdge(hvacRuleConnectorSplitterOutletBranch, connectorID, branchID, "reference", medium, connectorObj,
					[]int{fieldIndexOfValue(connectorObj, branchName, 2)}, []string{branchName})
			}
		}
		return
	}
	if strings.EqualFold(connector.Type, "Connector:Mixer") {
		for _, branchName := range connector.BranchNames {
			if branchID := b.branchNodeID(branchName); branchID != "" {
				b.addEdge(hvacRuleConnectorMixerInletBranch, branchID, connectorID, "reference", medium, connectorObj,
					[]int{fieldIndexOfValue(connectorObj, branchName, 2)}, []string{branchName})
			}
		}
		if connector.OutletBranchName != "" {
			if branchID := b.branchNodeID(connector.OutletBranchName); branchID != "" {
				b.addEdge(hvacRuleConnectorMixerOutletBranch, connectorID, branchID, "reference", medium, connectorObj,
					[]int{fieldIndexOfValue(connectorObj, connector.OutletBranchName, 1)}, []string{connector.OutletBranchName})
			}
		}
	}
}

func (b *hvacRuleGraphBuilder) addAirLoopDemandGraph(loop HVACLoop, loopObj Object) {
	medium := hvacRuleMediumForLoop(loop.Type)
	if loop.DemandGraph.SupplyPath != nil {
		b.addAirLoopDemandPath(loop, *loop.DemandGraph.SupplyPath, loopObj, medium)
	}
	if loop.DemandGraph.ReturnPath != nil {
		b.addAirLoopDemandPath(loop, *loop.DemandGraph.ReturnPath, loopObj, medium)
	}
	for _, edge := range loop.DemandGraph.Edges {
		fromID := b.addNodeName(edge.FromNode, medium, "airloop_demand")
		toID := b.addNodeName(edge.ToNode, medium, "airloop_demand")
		sourceObj, ok := b.objectByIndex(edge.ObjectIndex)
		if !ok {
			continue
		}
		ruleID := hvacRuleAirLoopDemandEdgeRule(edge.Role)
		if ruleID == "" {
			continue
		}
		b.addEdge(ruleID, fromID, toID, "flow", medium, sourceObj,
			fieldIndexesOfValues(sourceObj, []string{edge.FromNode, edge.ToNode}, 0), []string{edge.FromNode, edge.ToNode})
	}
}

func (b *hvacRuleGraphBuilder) addAirLoopDemandPath(loop HVACLoop, path AirLoopDemandPath, loopObj Object, medium string) {
	pathObj, ok := b.objectByTypeName(path.ObjectType, path.Name)
	if !ok {
		return
	}
	pathID := hvacRuleObjectNodeID("path", pathObj.Type, objectName(pathObj), pathObj.Index, path.PathType)
	b.addObjectNode(pathID, "path", path.PathType, medium, pathObj)
	b.addEdge(hvacRuleLoopSideBranchList, hvacRuleLoopNodeID(loop), pathID, "reference", medium, loopObj,
		[]int{fieldIndexOfValue(loopObj, path.InletNode, 0), fieldIndexOfValue(loopObj, path.OutletNode, 0)}, []string{path.InletNode, path.OutletNode})
	for index, component := range path.Components {
		hvacComponent := newHVACComponent(b.ctx, component.ObjectType, component.ObjectName)
		hvacComponent.ObjectIndex = component.ObjectIndex
		hvacComponent.TypeFieldIndex = component.SourceTypeFieldIndex
		hvacComponent.NameFieldIndex = component.SourceNameFieldIndex
		hvacComponent.RoleHere = component.Role
		branch := HVACBranch{Name: path.Name, ObjectIndex: path.ObjectIndex}
		_, occurrenceID := b.addComponentNodes(hvacComponent, loop, path.PathType, branch, index, medium)
		ruleID := hvacRuleAirLoopSupplyPathComponent
		if path.PathType == "return_path" {
			ruleID = hvacRuleAirLoopReturnPathComponent
		}
		b.addEdge(ruleID, pathID, occurrenceID, "sequence", medium, pathObj,
			[]int{component.SourceTypeFieldIndex, component.SourceNameFieldIndex}, []string{component.ObjectName})
	}
}

func (b *hvacRuleGraphBuilder) addZoneRelation(relation HVACZoneChain) {
	connectionObj, ok := b.zoneConnectionObject(relation)
	if !ok {
		return
	}
	subjectID := b.addZoneSubjectNode(relation)
	if subjectID == "" {
		return
	}
	connectionID := hvacRuleObjectNodeID("path", connectionObj.Type, objectName(connectionObj), connectionObj.Index, relation.RelationScope)
	b.addObjectNode(connectionID, "path", relation.RelationScope, "air", connectionObj)
	b.addEdge(hvacRuleZoneHasEquipmentConnections, subjectID, connectionID, "reference", "air", connectionObj,
		[]int{0}, []string{relation.ZoneName, relation.SpaceName})
	b.addZoneNodeEdges(subjectID, relation)
	b.addZoneEquipmentEdges(subjectID, connectionID, connectionObj, relation)
	b.addZoneTerminalEdges(subjectID, connectionObj, relation)
}

func (b *hvacRuleGraphBuilder) addZoneNodeEdges(subjectID string, relation HVACZoneChain) {
	for _, source := range relation.Nodes.Sources {
		ruleID := hvacRuleZoneNodeRule(source.Role)
		if ruleID == "" {
			continue
		}
		sourceObj, ok := b.objectByIndex(source.ObjectIndex)
		if !ok {
			sourceObj, _ = b.objectByTypeName(source.ObjectType, source.ObjectName)
		}
		fieldIndexes := []int{source.FieldIndex}
		if strings.EqualFold(source.SourceType, "node_list_expansion") {
			fieldIndexes = fieldIndexesOfValues(sourceObj, source.Nodes, 1)
		}
		for _, nodeName := range source.Nodes {
			nodeID := b.addNodeName(nodeName, "air", source.Role)
			b.addEdge(ruleID, subjectID, nodeID, "role", "air", sourceObj, fieldIndexes, []string{nodeName})
		}
	}
}

func (b *hvacRuleGraphBuilder) addZoneEquipmentEdges(subjectID string, connectionID string, connectionObj Object, relation HVACZoneChain) {
	equipmentListName := fieldValueByCatalogName(connectionObj, "Zone Conditioning Equipment List Name", "Space Conditioning Equipment List Name")
	equipmentListObj, ok := hvacEquipmentListByName(b.ctx, equipmentListName, "ZoneHVAC:EquipmentList", "SpaceHVAC:EquipmentList")
	if !ok {
		return
	}
	equipmentListID := hvacRuleObjectNodeID("path", equipmentListObj.Type, objectName(equipmentListObj), equipmentListObj.Index, relation.RelationScope)
	b.addObjectNode(equipmentListID, "path", "equipment_list", "air", equipmentListObj)
	b.addEdge(hvacRuleZoneHasEquipmentList, connectionID, equipmentListID, "reference", "air", connectionObj,
		[]int{fieldIndexOfValue(connectionObj, equipmentListName, 0)}, []string{equipmentListName})

	seen := map[string]bool{}
	for _, component := range relation.ZoneEquipment {
		key := hvacComponentKey(component)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		sourceID := b.addComponentSourceNode(component, "air")
		ruleID := hvacRuleZoneEquipmentListEquipment
		if isAirDistributionUnitType(component.ObjectType) {
			ruleID = hvacRuleZoneEquipmentListADU
		} else if isAirTerminalType(component.ObjectType) {
			ruleID = hvacRuleZoneEquipmentListTerminal
		}
		b.addEdge(ruleID, equipmentListID, sourceID, "reference", "air", equipmentListObj,
			[]int{component.TypeFieldIndex, component.NameFieldIndex}, []string{component.ObjectName})
		if !isAirTerminalType(component.ObjectType) && subjectID != "" {
			b.addEdge(ruleID, sourceID, subjectID, "serves", "air", equipmentListObj,
				[]int{component.TypeFieldIndex, component.NameFieldIndex}, []string{component.ObjectName})
		}
	}
	for _, terminal := range relation.TerminalUnits {
		if !terminal.ResolvedFromADU {
			continue
		}
		aduObj, ok := b.objectByTypeName("ZoneHVAC:AirDistributionUnit", terminal.DistributionUnitName)
		if !ok {
			continue
		}
		aduID := hvacRuleObjectNodeID("component", aduObj.Type, objectName(aduObj), aduObj.Index, "source")
		b.addObjectNode(aduID, "component", "source", "air", aduObj)
		terminalID := b.addComponentSourceNode(terminal, "air")
		b.addEdge(hvacRuleZoneADUResolvesTerminal, aduID, terminalID, "reference", "air", aduObj,
			fieldIndexesOfValues(aduObj, []string{terminal.ObjectType, terminal.ObjectName}, 1), []string{terminal.ObjectName})
		if terminal.DistributionUnitOutletNode != "" {
			nodeID := b.addNodeName(terminal.DistributionUnitOutletNode, "air", "zone_inlet")
			outletFieldIndex := terminal.DistributionUnitOutletFieldIndex
			if outletFieldIndex < 0 {
				outletFieldIndex = fieldIndexOfValue(aduObj, terminal.DistributionUnitOutletNode, 0)
			}
			b.addEdge(hvacRuleZoneADUOutletMatchesInlet, aduID, nodeID, "validation", "air", aduObj,
				[]int{outletFieldIndex}, []string{terminal.DistributionUnitOutletNode})
			b.addEdge(hvacRuleZoneADUOutletMatchesInlet, aduID, subjectID, "serves", "air", aduObj,
				[]int{outletFieldIndex}, []string{terminal.DistributionUnitOutletNode})
		}
		if terminal.TerminalObjectOutletNode != "" && terminal.DistributionUnitOutletNode != "" && strings.EqualFold(terminal.TerminalObjectOutletNode, terminal.DistributionUnitOutletNode) {
			terminalObj, ok := b.objectByTypeName(terminal.ObjectType, terminal.ObjectName)
			if ok {
				b.addEdge(hvacRuleZoneTerminalOutletMatchesADU, terminalID, aduID, "validation", "air", terminalObj,
					[]int{terminal.OutletFieldIndex}, []string{terminal.TerminalObjectOutletNode})
			}
		}
	}
}

func (b *hvacRuleGraphBuilder) addZoneTerminalEdges(subjectID string, connectionObj Object, relation HVACZoneChain) {
	for _, terminal := range relation.TerminalUnits {
		if !terminal.OutletMatchesZoneInlet || terminal.OutletNode == "" {
			continue
		}
		terminalObj, ok := b.objectByTypeName(terminal.ObjectType, terminal.ObjectName)
		if !ok {
			continue
		}
		terminalID := b.addComponentSourceNode(terminal, "air")
		nodeID := b.addNodeName(terminal.OutletNode, "air", "zone_inlet")
		b.addEdge(hvacRuleZoneTerminalOutletMatchesInlet, terminalID, nodeID, "validation", "air", terminalObj,
			[]int{terminal.OutletFieldIndex}, []string{terminal.OutletNode})
		b.addEdge(hvacRuleZoneTerminalOutletMatchesInlet, terminalID, subjectID, "serves", "air", terminalObj,
			[]int{terminal.OutletFieldIndex}, []string{terminal.OutletNode})
		if terminal.InletOnAirLoopDemand && terminal.InletNode != "" {
			for _, airLoopName := range relation.AirLoopNames {
				loopID := b.loopNodeID("AirLoopHVAC", airLoopName)
				if loopID == "" {
					continue
				}
				b.addEdge(hvacRuleAirLoopZoneSplitterToTerminal, loopID, terminalID, "serves", "air", terminalObj,
					[]int{terminal.InletFieldIndex}, []string{terminal.InletNode})
			}
		}
	}
}

func (b *hvacRuleGraphBuilder) addVRFSystemEdges() {
	for _, systemObj := range b.ctx.doc.Objects {
		if !isRefrigerantSystemType(systemObj.Type) {
			continue
		}
		listName, listFieldIndex, ok := vrfSystemTerminalUnitListName(systemObj)
		if !ok || listName == "" {
			continue
		}
		listObj, ok := b.objectByTypeName("ZoneTerminalUnitList", listName)
		if !ok {
			continue
		}
		system := newHVACComponent(b.ctx, systemObj.Type, objectName(systemObj))
		systemID := b.addComponentSourceNode(system, "refrigerant")
		listID := hvacRuleObjectNodeID("path", listObj.Type, objectName(listObj), listObj.Index, "zone_terminal_unit_list")
		b.addObjectNode(listID, "path", "zone_terminal_unit_list", "refrigerant", listObj)
		b.addEdge(hvacRuleVRFSystemTerminalList, systemID, listID, "reference", "refrigerant", systemObj,
			[]int{listFieldIndex}, []string{listName})

		for _, terminalRef := range zoneTerminalUnitReferences(listObj) {
			if terminalRef.Name == "" {
				continue
			}
			if _, ok := b.objectByTypeName("ZoneHVAC:TerminalUnit:VariableRefrigerantFlow", terminalRef.Name); !ok {
				continue
			}
			terminal := newHVACComponent(b.ctx, "ZoneHVAC:TerminalUnit:VariableRefrigerantFlow", terminalRef.Name)
			terminalID := b.addComponentSourceNode(terminal, "air")
			b.addEdge(hvacRuleVRFTerminalListContains, listID, terminalID, "reference", "refrigerant", listObj,
				[]int{terminalRef.FieldIndex}, []string{terminalRef.Name})
		}
	}
}

func (b *hvacRuleGraphBuilder) addRadiantSurfaceEdges() {
	for _, radiantObj := range b.ctx.doc.Objects {
		if !isLowTemperatureRadiantEquipmentType(radiantObj.Type) {
			continue
		}
		surfaceOrGroupName, fieldIndex, ok := radiantSurfaceOrGroupName(radiantObj)
		if !ok || surfaceOrGroupName == "" {
			continue
		}
		radiant := newHVACComponent(b.ctx, radiantObj.Type, objectName(radiantObj))
		radiantID := b.addComponentSourceNode(radiant, "radiant")
		if groupObj, ok := b.objectByTypeName("ZoneHVAC:LowTemperatureRadiant:SurfaceGroup", surfaceOrGroupName); ok {
			groupID := hvacRuleObjectNodeID("path", groupObj.Type, objectName(groupObj), groupObj.Index, "radiant_surface_group")
			b.addObjectNode(groupID, "path", "radiant_surface_group", "radiant", groupObj)
			b.addEdge(hvacRuleRadiantEquipmentSurfaceGroup, radiantID, groupID, "reference", "radiant", radiantObj,
				[]int{fieldIndex}, []string{surfaceOrGroupName})
			b.addRadiantSurfaceGroupSurfaceEdges(groupID, groupObj)
			continue
		}
		if surfaceObj, ok := b.objectByTypeName("BuildingSurface:Detailed", surfaceOrGroupName); ok {
			surfaceID := b.addRadiantSurfaceNode(surfaceObj)
			b.addEdge(hvacRuleRadiantEquipmentSurface, radiantID, surfaceID, "reference", "radiant", radiantObj,
				[]int{fieldIndex}, []string{surfaceOrGroupName})
			b.addRadiantSurfaceZoneEdge(surfaceID, surfaceObj)
		}
	}
}

func (b *hvacRuleGraphBuilder) addRadiantSurfaceGroupSurfaceEdges(groupID string, groupObj Object) {
	for _, surfaceRef := range radiantSurfaceGroupReferences(groupObj) {
		if surfaceRef.Name == "" {
			continue
		}
		surfaceObj, ok := b.objectByTypeName("BuildingSurface:Detailed", surfaceRef.Name)
		if !ok {
			continue
		}
		surfaceID := b.addRadiantSurfaceNode(surfaceObj)
		b.addEdge(hvacRuleRadiantSurfaceGroupSurface, groupID, surfaceID, "reference", "radiant", groupObj,
			[]int{surfaceRef.FieldIndex}, []string{surfaceRef.Name})
		b.addRadiantSurfaceZoneEdge(surfaceID, surfaceObj)
	}
}

func (b *hvacRuleGraphBuilder) addRadiantSurfaceNode(surfaceObj Object) string {
	surfaceID := hvacRuleObjectNodeID("surface", surfaceObj.Type, objectName(surfaceObj), surfaceObj.Index, "radiant_surface")
	b.addObjectNode(surfaceID, "surface", "radiant_surface", "radiant", surfaceObj)
	return surfaceID
}

func (b *hvacRuleGraphBuilder) addRadiantSurfaceZoneEdge(surfaceID string, surfaceObj Object) {
	zoneName, zoneFieldIndex, ok := fieldValueIndexByCatalogName(surfaceObj, "Zone Name")
	if !ok || zoneName == "" {
		return
	}
	zoneID := b.addZoneNode(zoneName)
	b.addEdge(hvacRuleRadiantSurfaceBelongsToZone, surfaceID, zoneID, "reference", "radiant", surfaceObj,
		[]int{zoneFieldIndex}, []string{zoneName})
}

func (b *hvacRuleGraphBuilder) addCrossLoopEdges() {
	for _, occurrences := range b.componentOccurrences {
		var air []hvacRuleComponentOccurrence
		var plantWater []hvacRuleComponentOccurrence
		var plantChillers []hvacRuleComponentOccurrence
		var condenser []hvacRuleComponentOccurrence
		for _, occurrence := range occurrences {
			if isWaterCoilType(occurrence.Component.ObjectType) {
				switch occurrence.LoopType {
				case "AirLoopHVAC":
					air = append(air, occurrence)
				case "PlantLoop":
					plantWater = append(plantWater, occurrence)
				}
			}
			if isChillerType(occurrence.Component.ObjectType) {
				switch occurrence.LoopType {
				case "PlantLoop":
					plantChillers = append(plantChillers, occurrence)
				case "CondenserLoop":
					condenser = append(condenser, occurrence)
				}
			}
		}
		for _, airOccurrence := range air {
			sourceObj, ok := b.objectByTypeName(airOccurrence.Component.ObjectType, airOccurrence.Component.ObjectName)
			if !ok {
				continue
			}
			for _, plantOccurrence := range plantWater {
				b.addEdge(hvacRuleCrossLoopSameWaterCoil, airOccurrence.ID, plantOccurrence.ID, "crossloop", "water", sourceObj,
					[]int{0}, []string{airOccurrence.Component.ObjectName})
				if airLoopID, plantLoopID := b.loopNodeID(airOccurrence.LoopType, airOccurrence.LoopName), b.loopNodeID(plantOccurrence.LoopType, plantOccurrence.LoopName); airLoopID != "" && plantLoopID != "" {
					b.addEdge(hvacRuleCrossLoopSameWaterCoil, plantLoopID, airLoopID, "crossloop", "water", sourceObj,
						[]int{0}, []string{airOccurrence.Component.ObjectName})
				}
			}
		}
		for _, plantOccurrence := range plantChillers {
			sourceObj, ok := b.objectByTypeName(plantOccurrence.Component.ObjectType, plantOccurrence.Component.ObjectName)
			if !ok {
				continue
			}
			for _, condenserOccurrence := range condenser {
				if !strings.EqualFold(plantOccurrence.Component.ObjectType, condenserOccurrence.Component.ObjectType) ||
					!strings.EqualFold(plantOccurrence.Component.ObjectName, condenserOccurrence.Component.ObjectName) {
					continue
				}
				b.addEdge(hvacRuleCrossLoopChillerCondenser, plantOccurrence.ID, condenserOccurrence.ID, "crossloop", "condenser_water", sourceObj,
					[]int{0}, []string{plantOccurrence.Component.ObjectName})
				if plantLoopID, condenserLoopID := b.loopNodeID(plantOccurrence.LoopType, plantOccurrence.LoopName), b.loopNodeID(condenserOccurrence.LoopType, condenserOccurrence.LoopName); plantLoopID != "" && condenserLoopID != "" {
					b.addEdge(hvacRuleCrossLoopChillerCondenser, plantLoopID, condenserLoopID, "crossloop", "condenser_water", sourceObj,
						[]int{0}, []string{plantOccurrence.Component.ObjectName})
				}
			}
		}
	}
}

func (b *hvacRuleGraphBuilder) addComponentReferenceEdges() {
	for _, reference := range b.ctx.componentReferences {
		if reference.RelationRole != "internal_component_reference" || !reference.TargetExists {
			continue
		}
		fromObj, ok := b.objectByIndex(reference.FromObjectIndex)
		if !ok || objectName(fromObj) == "" {
			continue
		}
		fromComponent := newHVACComponent(b.ctx, reference.FromObjectType, reference.FromObjectName)
		targetComponent := newHVACComponent(b.ctx, reference.TargetObjectType, reference.TargetObjectName)
		fromID := b.addComponentSourceNode(fromComponent, hvacRuleMediumForComponent(reference.FromObjectType))
		targetID := b.addComponentSourceNode(targetComponent, hvacRuleMediumForComponent(reference.TargetObjectType))
		fieldIndexes := []int{reference.TypeFieldIndex, reference.NameFieldIndex}
		b.addEdge(hvacRuleComponentReferencesComponent, fromID, targetID, "component_reference", hvacRuleMediumForComponent(reference.TargetObjectType), fromObj,
			fieldIndexes, []string{reference.TargetObjectName})
		if hvacReferenceTargetServesParent(reference) {
			b.addEdge(hvacRuleComponentServesParent, targetID, fromID, "serves_parent", hvacRuleMediumForComponent(reference.TargetObjectType), fromObj,
				fieldIndexes, []string{reference.TargetObjectName})
		}
	}
}

func (b *hvacRuleGraphBuilder) addBranchNode(branch HVACBranch, role string, medium string) string {
	branchObj, ok := b.objectByTypeName("Branch", branch.Name)
	id := hvacRuleObjectNodeID("branch", "Branch", branch.Name, branch.ObjectIndex, role)
	if ok {
		id = hvacRuleObjectNodeID("branch", branchObj.Type, objectName(branchObj), branchObj.Index, role)
		b.addObjectNode(id, "branch", role, medium, branchObj)
	} else {
		b.addNode(HVACRuleNode{
			ID:          id,
			Kind:        "branch",
			ObjectType:  "Branch",
			ObjectName:  branch.Name,
			ObjectIndex: branch.ObjectIndex,
			Label:       firstNonEmpty(branch.Name, "Branch"),
			Medium:      medium,
			Role:        strings.ToLower(role),
		})
	}
	if branch.Name != "" {
		b.branchNodeIDs[normalizeName(branch.Name)] = id
	}
	return id
}

func (b *hvacRuleGraphBuilder) addComponentNodes(component HVACComponent, loop HVACLoop, sideName string, branch HVACBranch, componentIndex int, medium string) (string, string) {
	sourceID := b.addComponentSourceNode(component, medium)
	occurrenceID := hvacRuleComponentOccurrenceNodeID(loop, sideName, branch, componentIndex, component)
	b.addNode(HVACRuleNode{
		ID:          occurrenceID,
		Kind:        "component",
		ObjectType:  component.ObjectType,
		ObjectName:  component.ObjectName,
		ObjectIndex: component.ObjectIndex,
		Label:       componentLabel(component),
		Medium:      medium,
		Role:        firstNonEmpty(component.RoleHere, strings.ToLower(sideName), "occurrence"),
	})
	key := hvacComponentKey(component)
	if key != "" {
		b.componentOccurrences[key] = append(b.componentOccurrences[key], hvacRuleComponentOccurrence{
			ID:        occurrenceID,
			LoopName:  loop.Name,
			LoopType:  loop.Type,
			SideName:  sideName,
			Component: component,
		})
	}
	return sourceID, occurrenceID
}

func (b *hvacRuleGraphBuilder) addComponentSourceNode(component HVACComponent, medium string) string {
	id := hvacRuleComponentSourceNodeID(component)
	b.addNode(HVACRuleNode{
		ID:          id,
		Kind:        "component",
		ObjectType:  component.ObjectType,
		ObjectName:  component.ObjectName,
		ObjectIndex: component.ObjectIndex,
		Label:       componentLabel(component),
		Medium:      medium,
		Role:        "source",
	})
	return id
}

func (b *hvacRuleGraphBuilder) addZoneSubjectNode(relation HVACZoneChain) string {
	if relation.SpaceName != "" {
		if obj, ok := b.objectByTypeName("Space", relation.SpaceName); ok {
			id := hvacRuleObjectNodeID("space", obj.Type, objectName(obj), obj.Index, "space")
			b.addObjectNode(id, "space", "space", "air", obj)
			if relation.ZoneName != "" {
				zoneID := b.addZoneNode(relation.ZoneName)
				if zoneID != "" {
					b.addEdge("space.belongs_to_zone", id, zoneID, "reference", "air", obj,
						[]int{fieldIndexOfValue(obj, relation.ZoneName, 0)}, []string{relation.ZoneName})
				}
			}
			return id
		}
		return hvacRuleNamedNodeID("space", relation.SpaceName, "space")
	}
	return b.addZoneNode(relation.ZoneName)
}

func (b *hvacRuleGraphBuilder) addZoneNode(zoneName string) string {
	if zoneName == "" {
		return ""
	}
	if obj, ok := b.objectByTypeName("Zone", zoneName); ok {
		id := hvacRuleObjectNodeID("zone", obj.Type, objectName(obj), obj.Index, "zone")
		b.addObjectNode(id, "zone", "zone", "air", obj)
		return id
	}
	id := hvacRuleNamedNodeID("zone", zoneName, "zone")
	b.addNode(HVACRuleNode{ID: id, Kind: "zone", ObjectType: "Zone", ObjectName: zoneName, Label: zoneName, Medium: "air", Role: "zone"})
	return id
}

func (b *hvacRuleGraphBuilder) addObjectNode(id string, kind string, role string, medium string, obj Object) {
	b.addNode(HVACRuleNode{
		ID:          id,
		Kind:        kind,
		ObjectType:  obj.Type,
		ObjectName:  objectName(obj),
		ObjectIndex: obj.Index,
		Label:       firstNonEmpty(objectName(obj), obj.Type),
		Medium:      medium,
		Role:        role,
	})
}

func (b *hvacRuleGraphBuilder) addNodeName(nodeName string, medium string, role string) string {
	if nodeName == "" {
		return ""
	}
	id := hvacRuleNamedNodeID("node", nodeName, role)
	b.addNode(HVACRuleNode{
		ID:     id,
		Kind:   "node",
		Label:  nodeName,
		Medium: medium,
		Role:   role,
	})
	return id
}

func (b *hvacRuleGraphBuilder) addNode(node HVACRuleNode) {
	if node.ID == "" {
		return
	}
	if node.Label == "" {
		node.Label = node.ID
	}
	if existing, ok := b.nodes[node.ID]; ok {
		if existing.ObjectIndex == 0 && node.ObjectIndex != 0 {
			existing.ObjectIndex = node.ObjectIndex
		}
		if existing.ObjectType == "" {
			existing.ObjectType = node.ObjectType
		}
		if existing.ObjectName == "" {
			existing.ObjectName = node.ObjectName
		}
		if existing.Medium == "" {
			existing.Medium = node.Medium
		}
		if existing.Role == "" {
			existing.Role = node.Role
		}
		b.nodes[node.ID] = existing
		return
	}
	b.nodes[node.ID] = node
}

func (b *hvacRuleGraphBuilder) addEdge(ruleID string, fromID string, toID string, edgeKind string, medium string, sourceObj Object, fieldIndexes []int, nodeNames []string) {
	if ruleID == "" || fromID == "" || toID == "" || sourceObj.Index < 0 {
		return
	}
	fields := cleanFieldIndexes(fieldIndexes)
	if len(fields) == 0 && len(sourceObj.Fields) > 0 {
		fields = []int{0}
	}
	edge := HVACRuleEdge{
		RuleID:             ruleID,
		FromID:             fromID,
		ToID:               toID,
		EdgeKind:           edgeKind,
		Medium:             medium,
		SourceObjectType:   sourceObj.Type,
		SourceObjectName:   objectName(sourceObj),
		SourceObjectIndex:  sourceObj.Index,
		SourceFieldIndexes: fields,
		NodeNames:          cleanStrings(nodeNames),
	}
	edge.ID = hvacRuleEdgeID(edge)
	b.edges[edge.ID] = edge
}

func (b *hvacRuleGraphBuilder) branchNodeID(branchName string) string {
	return b.branchNodeIDs[normalizeName(branchName)]
}

func (b *hvacRuleGraphBuilder) loopNodeID(loopType string, loopName string) string {
	return b.loopNodeIDs[hvacObjectKey(loopType, loopName)]
}

func (b *hvacRuleGraphBuilder) objectByTypeName(objectType string, objectNameValue string) (Object, bool) {
	if strings.TrimSpace(objectType) == "" || strings.TrimSpace(objectNameValue) == "" {
		return Object{}, false
	}
	obj, ok := b.ctx.objectsByTypeName[hvacObjectKey(objectType, objectNameValue)]
	return obj, ok
}

func (b *hvacRuleGraphBuilder) objectByIndex(index int) (Object, bool) {
	for _, obj := range b.ctx.doc.Objects {
		if obj.Index == index {
			return obj, true
		}
	}
	return Object{}, false
}

func (b *hvacRuleGraphBuilder) zoneConnectionObject(relation HVACZoneChain) (Object, bool) {
	if relation.SpaceName != "" {
		for _, obj := range b.ctx.objectsByType[normalizeFieldCatalogKey("SpaceHVAC:EquipmentConnections")] {
			if strings.EqualFold(fieldValueByCatalogName(obj, "Space Name"), relation.SpaceName) {
				return obj, true
			}
		}
	}
	for _, obj := range b.ctx.objectsByType[normalizeFieldCatalogKey("ZoneHVAC:EquipmentConnections")] {
		if strings.EqualFold(fieldValueByCatalogName(obj, "Zone Name"), relation.ZoneName) {
			return obj, true
		}
	}
	return Object{}, false
}

func hvacRuleLoopNodeID(loop HVACLoop) string {
	if loop.ID != "" {
		return "loop:" + loop.ID
	}
	return hvacRuleNamedNodeID("loop", loop.Name, loop.Type)
}

func hvacRuleComponentSourceNodeID(component HVACComponent) string {
	return hvacRuleObjectNodeID("component", component.ObjectType, component.ObjectName, component.ObjectIndex, "source")
}

func hvacRuleComponentOccurrenceNodeID(loop HVACLoop, sideName string, branch HVACBranch, componentIndex int, component HVACComponent) string {
	parts := []string{
		"component", "occurrence",
		normalizeName(loop.ID),
		normalizeName(sideName),
		strconv.Itoa(branch.ObjectIndex),
		strconv.Itoa(componentIndex),
		normalizeFieldCatalogKey(component.ObjectType),
		normalizeName(component.ObjectName),
	}
	return strings.Join(parts, ":")
}

func hvacRuleObjectNodeID(kind string, objectType string, objectNameValue string, objectIndex int, role string) string {
	if objectIndex >= 0 {
		return fmt.Sprintf("%s:%s:%d:%s", kind, normalizeFieldCatalogKey(objectType), objectIndex, normalizeName(role))
	}
	return hvacRuleNamedNodeID(kind, objectNameValue, role)
}

func hvacRuleNamedNodeID(kind string, name string, role string) string {
	return fmt.Sprintf("%s:%s:%s", kind, normalizeName(name), normalizeName(role))
}

func hvacRuleEdgeID(edge HVACRuleEdge) string {
	fields := make([]string, len(edge.SourceFieldIndexes))
	for index, value := range edge.SourceFieldIndexes {
		fields[index] = strconv.Itoa(value)
	}
	parts := []string{
		edge.RuleID,
		edge.FromID,
		edge.ToID,
		strconv.Itoa(edge.SourceObjectIndex),
		strings.Join(fields, "."),
		strings.Join(edge.NodeNames, "."),
	}
	return strings.Join(parts, "|")
}

func hvacRuleMediumForLoop(loopType string) string {
	switch strings.ToLower(strings.TrimSpace(loopType)) {
	case "airloophvac":
		return "air"
	case "condenserloop":
		return "condenser_water"
	case "plantloop":
		return "water"
	default:
		return ""
	}
}

func hvacRuleMediumForComponent(objectType string) string {
	switch {
	case isRefrigerantSystemType(objectType):
		return "refrigerant"
	case isAirTerminalType(objectType):
		return "air"
	case isWaterCoilType(objectType), isPlantSourceEquipmentType(objectType):
		return "water"
	default:
		return ""
	}
}

func hvacReferenceTargetServesParent(reference HVACComponentReference) bool {
	if isAirTerminalType(reference.FromObjectType) && isWaterCoilType(reference.TargetObjectType) {
		return true
	}
	return isDirectZoneEquipmentType(reference.FromObjectType) && isHVACComponentType(reference.TargetObjectType)
}

func hvacRuleAirLoopDemandEdgeRule(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "zone_splitter", "supply_plenum":
		return hvacRuleAirLoopZoneSplitterToTerminal
	case "zone_mixer", "return_plenum":
		return hvacRuleAirLoopZoneMixerFromZoneReturn
	default:
		return ""
	}
}

func hvacRuleComponentOnLoopBranch(loopType string, sideName string) string {
	switch {
	case strings.EqualFold(loopType, "PlantLoop") && strings.EqualFold(sideName, "Demand"):
		return hvacRulePlantComponentOnDemandBranch
	case strings.EqualFold(loopType, "PlantLoop"):
		return hvacRulePlantComponentOnSupplyBranch
	case strings.EqualFold(loopType, "CondenserLoop") && strings.EqualFold(sideName, "Demand"):
		return hvacRuleCondenserComponentOnDemand
	case strings.EqualFold(loopType, "CondenserLoop"):
		return hvacRuleCondenserComponentOnSupply
	default:
		return ""
	}
}

func hvacRuleZoneNodeRule(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "air_node":
		return hvacRuleZoneHasAirNode
	case "inlet_nodes":
		return hvacRuleZoneHasInletNode
	case "exhaust_nodes":
		return hvacRuleZoneHasExhaustNode
	case "return_nodes":
		return hvacRuleZoneHasReturnNode
	default:
		return ""
	}
}

func fieldIndexOfValue(obj Object, value string, start int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1
	}
	if start < 0 {
		start = 0
	}
	for index := start; index < len(obj.Fields); index++ {
		if strings.EqualFold(strings.TrimSpace(obj.Fields[index].Value), value) {
			return index
		}
	}
	return -1
}

func fieldIndexesOfValues(obj Object, values []string, start int) []int {
	var indexes []int
	for _, value := range values {
		if index := fieldIndexOfValue(obj, value, start); index >= 0 {
			indexes = append(indexes, index)
		}
	}
	return cleanFieldIndexes(indexes)
}

func cleanFieldIndexes(indexes []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, index := range indexes {
		if index < 0 || seen[index] {
			continue
		}
		seen[index] = true
		out = append(out, index)
	}
	sort.Ints(out)
	return out
}

func cleanStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := normalizeName(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}
