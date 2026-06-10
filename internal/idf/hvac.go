package idf

import (
	"fmt"
	"sort"
	"strings"
)

type HVACReport struct {
	LoopCount           int                      `json:"loopCount"`
	AirLoopCount        int                      `json:"airLoopCount"`
	PlantLoopCount      int                      `json:"plantLoopCount"`
	CondenserLoopCount  int                      `json:"condenserLoopCount"`
	ZoneRelationCount   int                      `json:"zoneRelationCount"`
	NodeCount           int                      `json:"nodeCount"`
	WarningCount        int                      `json:"warningCount"`
	Loops               []HVACLoop               `json:"loops"`
	ZoneRelations       []HVACZoneChain          `json:"zoneRelations"`
	NodeUsages          []HVACNodeUsage          `json:"nodeUsages"`
	NodeOutputVariables []HVACNodeOutputVariable `json:"nodeOutputVariables,omitempty"`
	NodeOutputMonitors  []HVACNodeOutputMonitor  `json:"nodeOutputMonitors,omitempty"`
	NodeEdges           []HVACNodeEdge           `json:"nodeEdges,omitempty"`
	Warnings            []HVACWarning            `json:"warnings"`
}

type HVACLoop struct {
	ID                      string                  `json:"id"`
	Type                    string                  `json:"type"`
	Name                    string                  `json:"name"`
	ObjectIndex             int                     `json:"objectIndex"`
	OperationScheme         string                  `json:"operationScheme,omitempty"`
	AvailabilityManagerList string                  `json:"availabilityManagerList,omitempty"`
	SetpointNode            string                  `json:"setpointNode,omitempty"`
	SupplySide              HVACLoopSide            `json:"supplySide"`
	DemandSide              HVACLoopSide            `json:"demandSide"`
	RelatedZones            []string                `json:"relatedZones,omitempty"`
	RelatedLoops            []HVACCrossLoopRelation `json:"relatedLoops,omitempty"`
	Warnings                []HVACWarning           `json:"warnings,omitempty"`
}

type HVACLoopSide struct {
	Name               string          `json:"name"`
	InletNode          string          `json:"inletNode,omitempty"`
	OutletNode         string          `json:"outletNode,omitempty"`
	BranchListName     string          `json:"branchListName,omitempty"`
	ConnectorListName  string          `json:"connectorListName,omitempty"`
	Branches           []HVACBranch    `json:"branches,omitempty"`
	Connectors         []HVACConnector `json:"connectors,omitempty"`
	MissingBranchNames []string        `json:"missingBranchNames,omitempty"`
	Warnings           []HVACWarning   `json:"warnings,omitempty"`
}

type HVACBranch struct {
	Name        string          `json:"name"`
	ObjectIndex int             `json:"objectIndex"`
	InletNode   string          `json:"inletNode,omitempty"`
	OutletNode  string          `json:"outletNode,omitempty"`
	Components  []HVACComponent `json:"components,omitempty"`
	Warnings    []HVACWarning   `json:"warnings,omitempty"`
}

type HVACComponent struct {
	ObjectType              string          `json:"objectType"`
	ObjectName              string          `json:"objectName"`
	ObjectIndex             int             `json:"objectIndex,omitempty"`
	Exists                  bool            `json:"exists"`
	Family                  string          `json:"family,omitempty"`
	FamilyLabel             string          `json:"familyLabel,omitempty"`
	DisplayLabel            string          `json:"displayLabel,omitempty"`
	LoopName                string          `json:"loopName,omitempty"`
	ControlType             string          `json:"controlType,omitempty"`
	InletNode               string          `json:"inletNode,omitempty"`
	OutletNode              string          `json:"outletNode,omitempty"`
	WaterInletNode          string          `json:"waterInletNode,omitempty"`
	WaterOutletNode         string          `json:"waterOutletNode,omitempty"`
	InletFieldIndex         int             `json:"inletFieldIndex,omitempty"`
	OutletFieldIndex        int             `json:"outletFieldIndex,omitempty"`
	RoleHere                string          `json:"roleHere,omitempty"`
	CoolingSequence         string          `json:"coolingSequence,omitempty"`
	HeatingSequence         string          `json:"heatingSequence,omitempty"`
	CoolingFractionSchedule string          `json:"coolingFractionSchedule,omitempty"`
	HeatingFractionSchedule string          `json:"heatingFractionSchedule,omitempty"`
	NodeUsages              []HVACNodeUsage `json:"nodeUsages,omitempty"`
	RelatedLoopNames        []string        `json:"relatedLoopNames,omitempty"`
	EditableFields          []HVACEditField `json:"editableFields,omitempty"`
	RelationSource          string          `json:"relationSource,omitempty"`
	RelationConfidence      string          `json:"relationConfidence,omitempty"`
	RelationEvidence        []string        `json:"relationEvidence,omitempty"`
	ListedInZoneEquipment   bool            `json:"listedInZoneEquipment,omitempty"`
	ResolvedFromADU         bool            `json:"resolvedFromAirDistributionUnit,omitempty"`
	DistributionUnitName    string          `json:"distributionUnitName,omitempty"`
	OutletMatchesZoneInlet  bool            `json:"outletMatchesZoneInlet,omitempty"`
	InletOnAirLoopDemand    bool            `json:"inletOnAirLoopDemandPath,omitempty"`
}

type HVACConnector struct {
	Type               string        `json:"type"`
	Name               string        `json:"name"`
	ObjectIndex        int           `json:"objectIndex"`
	InletBranchName    string        `json:"inletBranchName,omitempty"`
	OutletBranchName   string        `json:"outletBranchName,omitempty"`
	BranchNames        []string      `json:"branchNames,omitempty"`
	MissingBranchNames []string      `json:"missingBranchNames,omitempty"`
	Warnings           []HVACWarning `json:"warnings,omitempty"`
}

type HVACNodeUsage struct {
	NodeName    string `json:"nodeName"`
	ObjectType  string `json:"objectType"`
	ObjectName  string `json:"objectName,omitempty"`
	ObjectIndex int    `json:"objectIndex"`
	FieldIndex  int    `json:"fieldIndex"`
	FieldName   string `json:"fieldName,omitempty"`
	Role        string `json:"role"`
}

type HVACNodeEdge struct {
	NodeName      string `json:"nodeName"`
	FromComponent string `json:"fromComponent,omitempty"`
	FromNode      string `json:"fromNode,omitempty"`
	ToNode        string `json:"toNode,omitempty"`
	ToComponent   string `json:"toComponent,omitempty"`
	Direction     string `json:"direction"`
}

type HVACZoneChain struct {
	ZoneName           string             `json:"zoneName"`
	ZoneObjectIndex    int                `json:"zoneObjectIndex,omitempty"`
	RelationSource     string             `json:"relationSource,omitempty"`
	Confidence         string             `json:"confidence,omitempty"`
	Evidence           []string           `json:"evidence,omitempty"`
	Nodes              HVACZoneNodes      `json:"nodes,omitempty"`
	AirLoopNames       []string           `json:"airLoopNames,omitempty"`
	AirLoopRelations   []HVACLoopRelation `json:"airLoopRelations,omitempty"`
	TerminalUnits      []HVACComponent    `json:"terminalUnits,omitempty"`
	ZoneEquipment      []HVACComponent    `json:"zoneEquipment,omitempty"`
	PlantLoopNames     []string           `json:"plantLoopNames,omitempty"`
	PlantLoopRelations []HVACLoopRelation `json:"plantLoopRelations,omitempty"`
	CondenserLoopNames []string           `json:"condenserLoopNames,omitempty"`
	PlantEquipment     []HVACComponent    `json:"plantEquipment,omitempty"`
	ServiceChains      []HVACServicePath  `json:"serviceChains,omitempty"`
	Warnings           []HVACWarning      `json:"warnings,omitempty"`
}

type HVACZoneNodes struct {
	AirNode      string               `json:"airNode,omitempty"`
	InletNodes   []string             `json:"inletNodes,omitempty"`
	ExhaustNodes []string             `json:"exhaustNodes,omitempty"`
	ReturnNodes  []string             `json:"returnNodes,omitempty"`
	Sources      []HVACZoneNodeSource `json:"sources,omitempty"`
}

type HVACZoneNodeSource struct {
	Role        string   `json:"role"`
	InputValue  string   `json:"inputValue,omitempty"`
	Nodes       []string `json:"nodes,omitempty"`
	SourceType  string   `json:"sourceType"`
	ObjectType  string   `json:"objectType,omitempty"`
	ObjectName  string   `json:"objectName,omitempty"`
	ObjectIndex int      `json:"objectIndex,omitempty"`
	Field       string   `json:"field,omitempty"`
}

type HVACLoopRelation struct {
	LoopName   string   `json:"loopName"`
	LoopType   string   `json:"loopType"`
	Source     string   `json:"source"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence,omitempty"`
}

type HVACServicePath struct {
	ZoneName        string   `json:"zoneName"`
	TerminalName    string   `json:"terminalName,omitempty"`
	AirLoopName     string   `json:"airLoopName,omitempty"`
	Component       string   `json:"component,omitempty"`
	PlantLoop       string   `json:"plantLoop,omitempty"`
	SourceComponent string   `json:"sourceComponent,omitempty"`
	Confidence      string   `json:"confidence,omitempty"`
	Evidence        string   `json:"evidence,omitempty"`
	SourceRelations []string `json:"sourceRelations,omitempty"`
}

type HVACCrossLoopRelation struct {
	ComponentType string `json:"componentType"`
	ComponentName string `json:"componentName"`
	LoopName      string `json:"loopName"`
	LoopType      string `json:"loopType"`
}

type HVACWarning struct {
	Severity           string   `json:"severity"`
	Category           string   `json:"category"`
	Code               string   `json:"code"`
	Message            string   `json:"message"`
	ObjectIndex        int      `json:"objectIndex,omitempty"`
	ObjectType         string   `json:"objectType,omitempty"`
	ObjectName         string   `json:"objectName,omitempty"`
	FieldIndex         int      `json:"fieldIndex,omitempty"`
	Field              string   `json:"field,omitempty"`
	Value              string   `json:"value,omitempty"`
	ExpectedNodes      []string `json:"expectedNodes,omitempty"`
	ActualNode         string   `json:"actualNode,omitempty"`
	SuggestedFixTarget string   `json:"suggestedFixTarget,omitempty"`
}

type HVACEditField struct {
	ObjectIndex     int                    `json:"objectIndex"`
	ObjectType      string                 `json:"objectType"`
	ObjectName      string                 `json:"objectName,omitempty"`
	FieldIndex      int                    `json:"fieldIndex"`
	FieldName       string                 `json:"fieldName,omitempty"`
	CurrentValue    string                 `json:"currentValue"`
	EditKind        string                 `json:"editKind"`
	ValueType       string                 `json:"valueType"`
	AllowAutosize   bool                   `json:"allowAutosize,omitempty"`
	SuggestedValues []FieldValueSuggestion `json:"suggestedValues,omitempty"`
	RequiresPreview bool                   `json:"requiresPreview"`
	Impact          string                 `json:"impact,omitempty"`
}

type HVACNodeOutputVariable struct {
	VariableName string `json:"variableName"`
	Units        string `json:"units,omitempty"`
	ReportType   string `json:"reportType,omitempty"`
	Category     string `json:"category,omitempty"`
	Description  string `json:"description,omitempty"`
	AppliesTo    string `json:"appliesTo,omitempty"`
	Advanced     bool   `json:"advanced,omitempty"`
}

type HVACNodeOutputMonitor struct {
	KeyValue           string `json:"keyValue"`
	VariableName       string `json:"variableName"`
	ReportingFrequency string `json:"reportingFrequency,omitempty"`
	ScheduleName       string `json:"scheduleName,omitempty"`
	ObjectIndex        int    `json:"objectIndex"`
	Wildcard           bool   `json:"wildcard,omitempty"`
}

type hvacContext struct {
	doc                Document
	objectsByTypeName  map[string]Object
	objectsByName      map[string][]Object
	objectsByType      map[string][]Object
	nodeLists          map[string][]string
	nodeUsages         []HVACNodeUsage
	nodeUsagesByName   map[string][]HVACNodeUsage
	branches           map[string]HVACBranch
	componentLoopNames map[string]map[string]string
	componentLoopTypes map[string]map[string]string
	warnings           []HVACWarning
}

func AnalyzeHVAC(doc Document) HVACReport {
	ctx := newHVACContext(doc)
	for _, branchObj := range ctx.objectsByType[normalizeFieldCatalogKey("Branch")] {
		branch := parseHVACBranch(ctx, branchObj)
		if branch.Name != "" {
			ctx.branches[normalizeName(branch.Name)] = branch
		}
	}

	var loops []HVACLoop
	for _, obj := range ctx.objectsByType[normalizeFieldCatalogKey("AirLoopHVAC")] {
		loop := parseAirLoopHVAC(ctx, obj)
		loops = append(loops, loop)
		registerLoopComponents(ctx, loop)
	}
	for _, obj := range ctx.objectsByType[normalizeFieldCatalogKey("PlantLoop")] {
		loop := parsePlantLoop(ctx, obj)
		loops = append(loops, loop)
		registerLoopComponents(ctx, loop)
	}
	for _, obj := range ctx.objectsByType[normalizeFieldCatalogKey("CondenserLoop")] {
		loop := parseCondenserLoop(ctx, obj)
		loops = append(loops, loop)
		registerLoopComponents(ctx, loop)
	}

	applyCrossLoopRelations(ctx, loops)
	relations := buildHVACZoneRelations(ctx, loops)
	applyLoopZoneRelations(loops, relations)
	report := HVACReport{
		Loops:               loops,
		ZoneRelations:       relations,
		NodeUsages:          append([]HVACNodeUsage(nil), ctx.nodeUsages...),
		NodeOutputVariables: HVACNodeOutputVariables(),
		NodeOutputMonitors:  hvacNodeOutputMonitors(doc),
		NodeEdges:           buildHVACNodeEdges(loops),
	}
	report.LoopCount = len(report.Loops)
	report.ZoneRelationCount = len(report.ZoneRelations)
	report.NodeCount = len(ctx.nodeUsagesByName)
	for _, loop := range report.Loops {
		switch loop.Type {
		case "AirLoopHVAC":
			report.AirLoopCount++
		case "PlantLoop":
			report.PlantLoopCount++
		case "CondenserLoop":
			report.CondenserLoopCount++
		}
	}
	report.Warnings = collectHVACWarnings(ctx, report)
	report.WarningCount = len(report.Warnings)
	sortHVACReport(&report)
	return report
}

func newHVACContext(doc Document) *hvacContext {
	ctx := &hvacContext{
		doc:                doc,
		objectsByTypeName:  map[string]Object{},
		objectsByName:      map[string][]Object{},
		objectsByType:      map[string][]Object{},
		nodeLists:          map[string][]string{},
		nodeUsagesByName:   map[string][]HVACNodeUsage{},
		branches:           map[string]HVACBranch{},
		componentLoopNames: map[string]map[string]string{},
		componentLoopTypes: map[string]map[string]string{},
	}
	for _, obj := range doc.Objects {
		typeKey := normalizeFieldCatalogKey(obj.Type)
		ctx.objectsByType[typeKey] = append(ctx.objectsByType[typeKey], obj)
		if name := objectName(obj); name != "" {
			ctx.objectsByTypeName[hvacObjectKey(obj.Type, name)] = obj
			ctx.objectsByName[normalizeName(name)] = append(ctx.objectsByName[normalizeName(name)], obj)
		}
	}
	for _, obj := range ctx.objectsByType[normalizeFieldCatalogKey("NodeList")] {
		name := objectName(obj)
		if name == "" {
			continue
		}
		for index := 1; index < len(obj.Fields); index++ {
			value := strings.TrimSpace(obj.Fields[index].Value)
			if value != "" {
				ctx.nodeLists[normalizeName(name)] = append(ctx.nodeLists[normalizeName(name)], value)
			}
		}
	}
	for _, obj := range doc.Objects {
		collectHVACNodeUsages(ctx, obj)
	}
	return ctx
}

func parseAirLoopHVAC(ctx *hvacContext, obj Object) HVACLoop {
	name := objectName(obj)
	loop := HVACLoop{
		ID:                      fmt.Sprintf("air:%s", normalizeName(name)),
		Type:                    "AirLoopHVAC",
		Name:                    name,
		ObjectIndex:             obj.Index,
		AvailabilityManagerList: fieldValueByCatalogName(obj, "Availability Manager List Name"),
		SupplySide: buildHVACLoopSide(ctx, "Supply", obj,
			fieldValueByCatalogName(obj, "Supply Side Inlet Node Name"),
			fieldValueByCatalogName(obj, "Supply Side Outlet Node Names"),
			fieldValueByCatalogName(obj, "Branch List Name"),
			fieldValueByCatalogName(obj, "Connector List Name")),
		DemandSide: HVACLoopSide{
			Name:       "Demand",
			InletNode:  fieldValueByCatalogName(obj, "Demand Side Inlet Node Names"),
			OutletNode: fieldValueByCatalogName(obj, "Demand Side Outlet Node Name"),
		},
	}
	if loop.ID == "air:" {
		loop.ID = fmt.Sprintf("air:%d", obj.Index)
	}
	return loop
}

func parsePlantLoop(ctx *hvacContext, obj Object) HVACLoop {
	name := objectName(obj)
	loop := HVACLoop{
		ID:                      fmt.Sprintf("plant:%s", normalizeName(name)),
		Type:                    "PlantLoop",
		Name:                    name,
		ObjectIndex:             obj.Index,
		OperationScheme:         fieldValueByCatalogName(obj, "Plant Equipment Operation Scheme Name"),
		AvailabilityManagerList: fieldValueByCatalogName(obj, "Availability Manager List Name"),
		SetpointNode:            fieldValueByCatalogName(obj, "Loop Temperature Setpoint Node Name"),
		SupplySide: buildHVACLoopSide(ctx, "Supply", obj,
			fieldValueByCatalogName(obj, "Plant Side Inlet Node Name"),
			fieldValueByCatalogName(obj, "Plant Side Outlet Node Name"),
			fieldValueByCatalogName(obj, "Plant Side Branch List Name"),
			fieldValueByCatalogName(obj, "Plant Side Connector List Name")),
		DemandSide: buildHVACLoopSide(ctx, "Demand", obj,
			fieldValueByCatalogName(obj, "Demand Side Inlet Node Name"),
			fieldValueByCatalogName(obj, "Demand Side Outlet Node Name"),
			fieldValueByCatalogName(obj, "Demand Side Branch List Name"),
			fieldValueByCatalogName(obj, "Demand Side Connector List Name")),
	}
	if loop.ID == "plant:" {
		loop.ID = fmt.Sprintf("plant:%d", obj.Index)
	}
	return loop
}

func parseCondenserLoop(ctx *hvacContext, obj Object) HVACLoop {
	name := objectName(obj)
	loop := HVACLoop{
		ID:                      fmt.Sprintf("condenser:%s", normalizeName(name)),
		Type:                    "CondenserLoop",
		Name:                    name,
		ObjectIndex:             obj.Index,
		OperationScheme:         fieldValueByCatalogName(obj, "Condenser Equipment Operation Scheme Name", "Plant Equipment Operation Scheme Name"),
		AvailabilityManagerList: fieldValueByCatalogName(obj, "Availability Manager List Name"),
		SetpointNode:            fieldValueByCatalogName(obj, "Condenser Loop Temperature Setpoint Node Name", "Loop Temperature Setpoint Node Name"),
		SupplySide: buildHVACLoopSide(ctx, "Supply", obj,
			fieldValueByCatalogName(obj, "Condenser Side Inlet Node Name", "Plant Side Inlet Node Name"),
			fieldValueByCatalogName(obj, "Condenser Side Outlet Node Name", "Plant Side Outlet Node Name"),
			fieldValueByCatalogName(obj, "Condenser Side Branch List Name", "Plant Side Branch List Name"),
			fieldValueByCatalogName(obj, "Condenser Side Connector List Name", "Plant Side Connector List Name")),
		DemandSide: buildHVACLoopSide(ctx, "Demand", obj,
			fieldValueByCatalogName(obj, "Demand Side Inlet Node Name"),
			fieldValueByCatalogName(obj, "Demand Side Outlet Node Name"),
			fieldValueByCatalogName(obj, "Demand Side Branch List Name"),
			fieldValueByCatalogName(obj, "Demand Side Connector List Name")),
	}
	if loop.ID == "condenser:" {
		loop.ID = fmt.Sprintf("condenser:%d", obj.Index)
	}
	return loop
}

func buildHVACLoopSide(ctx *hvacContext, sideName string, owner Object, inletNode string, outletNode string, branchListName string, connectorListName string) HVACLoopSide {
	side := HVACLoopSide{
		Name:              sideName,
		InletNode:         strings.TrimSpace(inletNode),
		OutletNode:        strings.TrimSpace(outletNode),
		BranchListName:    strings.TrimSpace(branchListName),
		ConnectorListName: strings.TrimSpace(connectorListName),
	}
	branchNames := branchNamesFromList(ctx, side.BranchListName, owner)
	for _, warning := range duplicateBranchNameWarnings(owner, side.BranchListName, branchNames) {
		side.Warnings = append(side.Warnings, warning)
	}
	knownBranches := map[string]bool{}
	for _, branchName := range branchNames {
		key := normalizeName(branchName)
		knownBranches[key] = true
		branch, ok := ctx.branches[key]
		if !ok {
			side.MissingBranchNames = append(side.MissingBranchNames, branchName)
			side.Warnings = append(side.Warnings, hvacWarningForObject(owner, "missing_branch", fmt.Sprintf("%s references missing Branch %q.", objectLabel(owner), branchName)))
			continue
		}
		branch = annotateHVACBranchContext(branch, owner.Type, sideName)
		side.Branches = append(side.Branches, branch)
	}
	side.Connectors = connectorsFromList(ctx, side.ConnectorListName, owner, knownBranches)
	for _, connector := range side.Connectors {
		side.Warnings = append(side.Warnings, connector.Warnings...)
	}
	side.Warnings = append(side.Warnings, validateHVACLoopSide(owner, side)...)
	return side
}

func annotateHVACBranchContext(branch HVACBranch, loopType string, sideName string) HVACBranch {
	for index := range branch.Components {
		branch.Components[index].RoleHere = hvacComponentContextRole(branch.Components[index], loopType, sideName)
	}
	return branch
}

func hvacComponentContextRole(component HVACComponent, loopType string, sideName string) string {
	family := component.Family
	if family == "" {
		family, _ = hvacComponentFamily(component.ObjectType)
	}
	if isHVACControlType(component.ObjectType) {
		return "control"
	}
	if family == "pipe" {
		return "bypass_pipe"
	}
	loopKey := strings.ToLower(strings.TrimSpace(loopType))
	sideKey := strings.ToLower(strings.TrimSpace(sideName))
	switch loopKey {
	case "airloophvac":
		if sideKey == "demand" {
			return "air_demand_distribution"
		}
		return "air_supply_component"
	case "plantloop":
		if sideKey == "demand" {
			return "plant_demand_load"
		}
		if isPlantSourceEquipmentType(component.ObjectType) {
			return "plant_supply_source"
		}
		return "plant_supply_component"
	case "condenserloop":
		if sideKey == "demand" {
			return "condenser_demand_load"
		}
		return "condenser_supply_reject"
	default:
		return "loop_component"
	}
}

func hvacZoneEquipmentRole(component HVACComponent) string {
	if isAirTerminalType(component.ObjectType) {
		return "zone_terminal"
	}
	return "zone_equipment"
}

func duplicateBranchNameWarnings(owner Object, branchListName string, branchNames []string) []HVACWarning {
	seen := map[string]bool{}
	var warnings []HVACWarning
	for _, branchName := range branchNames {
		key := normalizeName(branchName)
		if key == "" {
			continue
		}
		if seen[key] {
			warnings = append(warnings, hvacWarningForObject(owner, "duplicate_branch_in_branch_list",
				fmt.Sprintf("BranchList %q contains duplicate Branch %q.", branchListName, branchName)))
		}
		seen[key] = true
	}
	return warnings
}

func validateHVACLoopSide(owner Object, side HVACLoopSide) []HVACWarning {
	var warnings []HVACWarning
	if len(side.Branches) > 0 {
		first := side.Branches[0]
		last := side.Branches[len(side.Branches)-1]
		if side.InletNode != "" && first.InletNode != "" && !strings.EqualFold(side.InletNode, first.InletNode) {
			warnings = append(warnings, hvacWarningForObject(owner, "branch_list_order_mismatch",
				fmt.Sprintf("%s side first branch %q inlet node %q does not match loop side inlet node %q.", side.Name, first.Name, first.InletNode, side.InletNode)))
		}
		if side.OutletNode != "" && last.OutletNode != "" && !strings.EqualFold(side.OutletNode, last.OutletNode) {
			warnings = append(warnings, hvacWarningForObject(owner, "branch_list_order_mismatch",
				fmt.Sprintf("%s side last branch %q outlet node %q does not match loop side outlet node %q.", side.Name, last.Name, last.OutletNode, side.OutletNode)))
		}
	}
	for _, connector := range side.Connectors {
		if strings.EqualFold(connector.Type, "Connector:Splitter") && len(side.Branches) > 0 && connector.InletBranchName != "" && !strings.EqualFold(connector.InletBranchName, side.Branches[0].Name) {
			warnings = append(warnings, hvacWarningForObject(owner, "splitter_inlet_not_loop_inlet_branch",
				fmt.Sprintf("Splitter %q inlet branch %q is not the first branch in %s side BranchList.", connector.Name, connector.InletBranchName, side.Name)))
		}
		if strings.EqualFold(connector.Type, "Connector:Mixer") && len(side.Branches) > 0 && connector.OutletBranchName != "" && !strings.EqualFold(connector.OutletBranchName, side.Branches[len(side.Branches)-1].Name) {
			warnings = append(warnings, hvacWarningForObject(owner, "mixer_outlet_not_loop_outlet_branch",
				fmt.Sprintf("Mixer %q outlet branch %q is not the last branch in %s side BranchList.", connector.Name, connector.OutletBranchName, side.Name)))
		}
	}
	return warnings
}

func branchNamesFromList(ctx *hvacContext, branchListName string, owner Object) []string {
	if strings.TrimSpace(branchListName) == "" {
		return nil
	}
	obj, ok := ctx.objectsByTypeName[hvacObjectKey("BranchList", branchListName)]
	if !ok {
		ctx.warnings = append(ctx.warnings, hvacWarningForObject(owner, "missing_branch_list", fmt.Sprintf("%s references missing BranchList %q.", objectLabel(owner), branchListName)))
		return nil
	}
	var names []string
	for index := 1; index < len(obj.Fields); index++ {
		value := strings.TrimSpace(obj.Fields[index].Value)
		if value != "" {
			names = append(names, value)
		}
	}
	return names
}

func connectorsFromList(ctx *hvacContext, connectorListName string, owner Object, knownBranches map[string]bool) []HVACConnector {
	if strings.TrimSpace(connectorListName) == "" {
		return nil
	}
	obj, ok := ctx.objectsByTypeName[hvacObjectKey("ConnectorList", connectorListName)]
	if !ok {
		ctx.warnings = append(ctx.warnings, hvacWarningForObject(owner, "missing_connector_list", fmt.Sprintf("%s references missing ConnectorList %q.", objectLabel(owner), connectorListName)))
		return nil
	}
	var connectors []HVACConnector
	for index := 1; index+1 < len(obj.Fields); index += 2 {
		connectorType := strings.TrimSpace(obj.Fields[index].Value)
		connectorName := strings.TrimSpace(obj.Fields[index+1].Value)
		if connectorType == "" && connectorName == "" {
			continue
		}
		connectorObj, exists := ctx.objectsByTypeName[hvacObjectKey(connectorType, connectorName)]
		if !exists {
			connectors = append(connectors, HVACConnector{
				Type:        connectorType,
				Name:        connectorName,
				ObjectIndex: 0,
				Warnings:    []HVACWarning{hvacWarningForObject(obj, "missing_connector", fmt.Sprintf("ConnectorList %q references missing %s %q.", objectName(obj), connectorType, connectorName))},
			})
			continue
		}
		connectors = append(connectors, parseHVACConnector(connectorObj, knownBranches))
	}
	for _, warning := range validateConnectorListComposition(obj, connectors) {
		ctx.warnings = append(ctx.warnings, warning)
	}
	return connectors
}

func validateConnectorListComposition(obj Object, connectors []HVACConnector) []HVACWarning {
	var warnings []HVACWarning
	if len(connectors) > 2 {
		warnings = append(warnings, hvacWarningForObject(obj, "connector_list_too_many_connectors",
			fmt.Sprintf("ConnectorList %q has %d connectors; EnergyPlus loop sides allow at most two.", objectName(obj), len(connectors))))
	}
	if len(connectors) == 2 {
		hasSplitter := false
		hasMixer := false
		for _, connector := range connectors {
			hasSplitter = hasSplitter || strings.EqualFold(connector.Type, "Connector:Splitter")
			hasMixer = hasMixer || strings.EqualFold(connector.Type, "Connector:Mixer")
		}
		if !hasSplitter || !hasMixer {
			warnings = append(warnings, hvacWarningForObject(obj, "connector_list_invalid_composition",
				fmt.Sprintf("ConnectorList %q with two connectors must contain one splitter and one mixer.", objectName(obj))))
		}
	}
	return warnings
}

func parseHVACConnector(obj Object, knownBranches map[string]bool) HVACConnector {
	connector := HVACConnector{
		Type:        obj.Type,
		Name:        objectName(obj),
		ObjectIndex: obj.Index,
	}
	start := 2
	if strings.EqualFold(obj.Type, "Connector:Splitter") {
		connector.InletBranchName = hvacFieldValue(obj, 1)
	} else if strings.EqualFold(obj.Type, "Connector:Mixer") {
		connector.OutletBranchName = hvacFieldValue(obj, 1)
	}
	for index := start; index < len(obj.Fields); index++ {
		value := strings.TrimSpace(obj.Fields[index].Value)
		if value == "" {
			continue
		}
		connector.BranchNames = append(connector.BranchNames, value)
		if len(knownBranches) > 0 && !knownBranches[normalizeName(value)] {
			connector.MissingBranchNames = append(connector.MissingBranchNames, value)
			connector.Warnings = append(connector.Warnings, hvacWarningForObject(obj, "connector_branch_outside_branch_list",
				fmt.Sprintf("%s %q references Branch %q outside the selected BranchList.", obj.Type, objectName(obj), value)))
		}
	}
	return connector
}

func parseHVACBranch(ctx *hvacContext, obj Object) HVACBranch {
	branch := HVACBranch{
		Name:        objectName(obj),
		ObjectIndex: obj.Index,
	}
	for _, reference := range branchComponentReferences(ctx, obj) {
		componentType := strings.TrimSpace(obj.Fields[reference.TypeIndex].Value)
		componentName := strings.TrimSpace(obj.Fields[reference.NameIndex].Value)
		inletNode := strings.TrimSpace(obj.Fields[reference.InletIndex].Value)
		outletNode := strings.TrimSpace(obj.Fields[reference.OutletIndex].Value)
		if componentType == "" && componentName == "" && inletNode == "" && outletNode == "" {
			continue
		}
		component := newHVACComponent(ctx, componentType, componentName)
		component.InletNode = firstNonEmpty(component.InletNode, inletNode)
		component.OutletNode = firstNonEmpty(component.OutletNode, outletNode)
		component.InletFieldIndex = reference.InletIndex
		component.OutletFieldIndex = reference.OutletIndex
		if reference.ControlIndex >= 0 && reference.ControlIndex < len(obj.Fields) {
			component.ControlType = strings.TrimSpace(obj.Fields[reference.ControlIndex].Value)
		}
		if componentType != "" && componentName != "" && !component.Exists {
			branch.Warnings = append(branch.Warnings, hvacWarningForObject(obj, "missing_branch_component",
				fmt.Sprintf("Branch %q references missing %s %q.", branch.Name, componentType, componentName)))
		}
		if strings.EqualFold(componentType, "Connector:Splitter") || strings.EqualFold(componentType, "Connector:Mixer") {
			branch.Warnings = append(branch.Warnings, hvacWarningForObject(obj, "connector_inside_branch_component_list",
				fmt.Sprintf("Branch %q includes %s %q as a component; splitters and mixers belong in ConnectorList.", branch.Name, componentType, componentName)))
		}
		branch.Components = append(branch.Components, component)
	}
	for index := 0; index < len(branch.Components)-1; index++ {
		current := branch.Components[index]
		next := branch.Components[index+1]
		if current.OutletNode != "" && next.InletNode != "" && !strings.EqualFold(current.OutletNode, next.InletNode) {
			branch.Warnings = append(branch.Warnings, hvacWarningForObject(obj, "branch_node_sequence_mismatch",
				fmt.Sprintf("Branch %q component %q outlet node %q does not match next component %q inlet node %q.",
					branch.Name, componentLabel(current), current.OutletNode, componentLabel(next), next.InletNode)))
		}
	}
	if len(branch.Components) > 0 {
		branch.InletNode = branch.Components[0].InletNode
		branch.OutletNode = branch.Components[len(branch.Components)-1].OutletNode
	}
	return branch
}

type branchComponentReference struct {
	TypeIndex    int
	NameIndex    int
	InletIndex   int
	OutletIndex  int
	ControlIndex int
}

func branchComponentReferences(ctx *hvacContext, obj Object) []branchComponentReference {
	if references := branchComponentReferencesFromComments(obj); len(references) > 0 {
		return references
	}
	if references := branchComponentReferencesByStride(obj, 4); scoreBranchComponentReferences(ctx, obj, references) > 0 {
		return references
	}
	stride4 := branchComponentReferencesByStride(obj, 4)
	stride5 := branchComponentReferencesByStride(obj, 5)
	if scoreBranchComponentReferences(ctx, obj, stride5) > scoreBranchComponentReferences(ctx, obj, stride4) {
		return stride5
	}
	return stride4
}

func branchComponentReferencesFromComments(obj Object) []branchComponentReference {
	type branchComponentCommentGroup struct {
		typeIndex    int
		nameIndex    int
		inletIndex   int
		outletIndex  int
		controlIndex int
	}
	groups := map[int]branchComponentCommentGroup{}
	seenGroups := map[int]bool{}
	var order []int
	for index, field := range obj.Fields {
		comment := normalizeFieldName(field.Comment)
		group, role := branchComponentCommentRole(comment)
		if group == 0 || role == "" {
			continue
		}
		info := groups[group]
		if !seenGroups[group] {
			order = append(order, group)
			seenGroups[group] = true
			info = branchComponentCommentGroup{typeIndex: -1, nameIndex: -1, inletIndex: -1, outletIndex: -1, controlIndex: -1}
		}
		switch role {
		case "type":
			info.typeIndex = index
		case "name":
			info.nameIndex = index
		case "inlet":
			info.inletIndex = index
		case "outlet":
			info.outletIndex = index
		case "control":
			info.controlIndex = index
		}
		groups[group] = info
	}
	sort.Ints(order)
	var references []branchComponentReference
	for _, group := range order {
		info := groups[group]
		if info.typeIndex >= 0 && info.nameIndex >= 0 && info.inletIndex >= 0 && info.outletIndex >= 0 {
			references = append(references, branchComponentReference{
				TypeIndex:    info.typeIndex,
				NameIndex:    info.nameIndex,
				InletIndex:   info.inletIndex,
				OutletIndex:  info.outletIndex,
				ControlIndex: info.controlIndex,
			})
		}
	}
	return references
}

func branchComponentCommentRole(comment string) (int, string) {
	if !strings.Contains(comment, "component") {
		return 0, ""
	}
	group := firstPositiveInteger(comment)
	if group == 0 {
		return 0, ""
	}
	switch {
	case strings.Contains(comment, "object type"):
		return group, "type"
	case strings.Contains(comment, "inlet") && strings.Contains(comment, "node"):
		return group, "inlet"
	case strings.Contains(comment, "outlet") && strings.Contains(comment, "node"):
		return group, "outlet"
	case strings.Contains(comment, "control"):
		return group, "control"
	case strings.Contains(comment, "name") && !strings.Contains(comment, "node") && !strings.Contains(comment, "schedule"):
		return group, "name"
	default:
		return 0, ""
	}
}

func branchComponentReferencesByStride(obj Object, stride int) []branchComponentReference {
	if stride != 4 && stride != 5 {
		return nil
	}
	var references []branchComponentReference
	for index := 2; index+3 < len(obj.Fields); index += stride {
		if fieldsAreBlank(obj, index, index+1, index+2, index+3) {
			continue
		}
		controlIndex := -1
		if stride == 5 && index+4 < len(obj.Fields) {
			controlIndex = index + 4
		}
		references = append(references, branchComponentReference{
			TypeIndex:    index,
			NameIndex:    index + 1,
			InletIndex:   index + 2,
			OutletIndex:  index + 3,
			ControlIndex: controlIndex,
		})
	}
	return references
}

func scoreBranchComponentReferences(ctx *hvacContext, obj Object, references []branchComponentReference) int {
	score := 0
	for index, reference := range references {
		objectType := strings.TrimSpace(obj.Fields[reference.TypeIndex].Value)
		objectNameValue := strings.TrimSpace(obj.Fields[reference.NameIndex].Value)
		inletNode := strings.TrimSpace(obj.Fields[reference.InletIndex].Value)
		outletNode := strings.TrimSpace(obj.Fields[reference.OutletIndex].Value)
		if objectType == "" && objectNameValue == "" && inletNode == "" && outletNode == "" {
			continue
		}
		score++
		if isHVACComponentType(objectType) || isAirTerminalType(objectType) {
			score += 3
		}
		if _, ok := ctx.objectsByTypeName[hvacObjectKey(objectType, objectNameValue)]; ok {
			score += 5
		}
		if isBranchControlValue(objectType) {
			score -= 4
		}
		if inletNode != "" {
			score++
		}
		if outletNode != "" {
			score++
		}
		if index > 0 {
			previous := references[index-1]
			previousOutlet := strings.TrimSpace(obj.Fields[previous.OutletIndex].Value)
			if previousOutlet != "" && inletNode != "" && strings.EqualFold(previousOutlet, inletNode) {
				score += 2
			}
		}
	}
	return score
}

func fieldsAreBlank(obj Object, indexes ...int) bool {
	for _, index := range indexes {
		if index >= 0 && index < len(obj.Fields) && strings.TrimSpace(obj.Fields[index].Value) != "" {
			return false
		}
	}
	return true
}

func isBranchControlValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "active", "passive", "bypass":
		return true
	default:
		return false
	}
}

func newHVACComponent(ctx *hvacContext, objectType string, objectNameValue string) HVACComponent {
	family, familyLabel := hvacComponentFamily(objectType)
	component := HVACComponent{
		ObjectType:   strings.TrimSpace(objectType),
		ObjectName:   strings.TrimSpace(objectNameValue),
		ObjectIndex:  -1,
		Family:       family,
		FamilyLabel:  familyLabel,
		DisplayLabel: hvacComponentDisplayLabel(objectType),
	}
	if component.ObjectType == "" || component.ObjectName == "" {
		return component
	}
	obj, ok := ctx.objectsByTypeName[hvacObjectKey(component.ObjectType, component.ObjectName)]
	if !ok {
		return component
	}
	component.Exists = true
	component.ObjectIndex = obj.Index
	component.NodeUsages = nodeUsagesForObject(ctx, obj)
	component.EditableFields = editableHVACFields(ctx.doc, obj)
	for _, usage := range component.NodeUsages {
		switch usage.Role {
		case "water_inlet", "plant_inlet":
			component.WaterInletNode = firstNonEmpty(component.WaterInletNode, usage.NodeName)
		case "water_outlet", "plant_outlet":
			component.WaterOutletNode = firstNonEmpty(component.WaterOutletNode, usage.NodeName)
		case "inlet", "air_inlet", "zone_inlet", "condenser_inlet":
			component.InletNode = firstNonEmpty(component.InletNode, usage.NodeName)
		case "outlet", "air_outlet", "zone_outlet", "condenser_outlet":
			component.OutletNode = firstNonEmpty(component.OutletNode, usage.NodeName)
		}
	}
	if inlet := fieldValueByCatalogName(obj, "Air Inlet Node Name", "Inlet Node Name"); inlet != "" {
		component.InletNode = inlet
	}
	if outlet := fieldValueByCatalogName(obj, "Air Outlet Node Name", "Outlet Node Name"); outlet != "" {
		component.OutletNode = outlet
	}
	return component
}

func registerLoopComponents(ctx *hvacContext, loop HVACLoop) {
	for _, component := range loopComponents(loop) {
		key := hvacComponentKey(component)
		if key == "" {
			continue
		}
		if ctx.componentLoopNames[key] == nil {
			ctx.componentLoopNames[key] = map[string]string{}
		}
		if ctx.componentLoopTypes[key] == nil {
			ctx.componentLoopTypes[key] = map[string]string{}
		}
		ctx.componentLoopNames[key][loop.Name] = loop.Name
		ctx.componentLoopTypes[key][loop.Name] = loop.Type
	}
}

func applyCrossLoopRelations(ctx *hvacContext, loops []HVACLoop) {
	for loopIndex := range loops {
		relations := map[string]HVACCrossLoopRelation{}
		mutateLoopComponents(&loops[loopIndex], func(component *HVACComponent) {
			key := hvacComponentKey(*component)
			if key == "" {
				return
			}
			var relatedNames []string
			for loopName := range ctx.componentLoopNames[key] {
				if loopName == loops[loopIndex].Name {
					continue
				}
				relatedNames = append(relatedNames, loopName)
				relations[loopName+"|"+key] = HVACCrossLoopRelation{
					ComponentType: component.ObjectType,
					ComponentName: component.ObjectName,
					LoopName:      loopName,
					LoopType:      ctx.componentLoopTypes[key][loopName],
				}
			}
			sort.Strings(relatedNames)
			component.RelatedLoopNames = relatedNames
		})
		for _, relation := range relations {
			loops[loopIndex].RelatedLoops = append(loops[loopIndex].RelatedLoops, relation)
		}
		sort.Slice(loops[loopIndex].RelatedLoops, func(i, j int) bool {
			return loops[loopIndex].RelatedLoops[i].LoopName < loops[loopIndex].RelatedLoops[j].LoopName
		})
		loops[loopIndex].Warnings = append(loops[loopIndex].Warnings, waterCoilLoopWarnings(ctx, loops[loopIndex])...)
	}
}

func waterCoilLoopWarnings(ctx *hvacContext, loop HVACLoop) []HVACWarning {
	var warnings []HVACWarning
	for _, component := range loopComponents(loop) {
		if !isWaterCoilType(component.ObjectType) {
			continue
		}
		key := hvacComponentKey(component)
		if key == "" {
			continue
		}
		loopTypes := ctx.componentLoopTypes[key]
		switch loop.Type {
		case "AirLoopHVAC":
			if !containsMapValue(loopTypes, "PlantLoop") {
				warnings = append(warnings, hvacWarningForComponent(component, "water_coil_missing_plant_loop",
					fmt.Sprintf("Water coil %s is on AirLoop %q but was not found on any PlantLoop branch.", componentLabel(component), loop.Name)))
			}
		case "PlantLoop":
			if loopSideContainsComponent(loop.DemandSide, component) && !containsMapValue(loopTypes, "AirLoopHVAC") && !componentReferencedByZoneHVAC(ctx, key) {
				warning := hvacWarningForComponent(component, "plant_demand_component_without_air_or_zone_use",
					fmt.Sprintf("Plant demand component %s was not found on any AirLoop branch or ZoneHVAC equipment list.", componentLabel(component)))
				if strings.Contains(strings.ToLower(loop.Name), "service") || strings.Contains(strings.ToLower(loop.Name), "process") {
					warning.Severity = "notice"
				}
				warnings = append(warnings, warning)
			}
		}
	}
	return warnings
}

func buildHVACZoneRelations(ctx *hvacContext, loops []HVACLoop) []HVACZoneChain {
	var relations []HVACZoneChain
	for _, connectionObj := range ctx.objectsByType[normalizeFieldCatalogKey("ZoneHVAC:EquipmentConnections")] {
		relation := buildHVACZoneRelation(ctx, loops, connectionObj)
		if relation.ZoneName != "" {
			relations = append(relations, relation)
		}
	}
	sort.Slice(relations, func(i, j int) bool {
		return strings.ToLower(relations[i].ZoneName) < strings.ToLower(relations[j].ZoneName)
	})
	return relations
}

func buildHVACZoneRelation(ctx *hvacContext, loops []HVACLoop, connectionObj Object) HVACZoneChain {
	zoneName := fieldValueByCatalogName(connectionObj, "Zone Name")
	relation := HVACZoneChain{
		ZoneName:        zoneName,
		ZoneObjectIndex: zoneObjectIndex(ctx, zoneName),
	}
	equipmentListName := fieldValueByCatalogName(connectionObj, "Zone Conditioning Equipment List Name")
	zoneInletNodes := expandNodeOrNodeList(ctx, fieldValueByCatalogName(connectionObj, "Zone Air Inlet Node or NodeList Name"))
	zoneExhaustNodes := expandNodeOrNodeList(ctx, fieldValueByCatalogName(connectionObj, "Zone Air Exhaust Node or NodeList Name"))
	zoneReturnNodes := expandNodeOrNodeList(ctx, fieldValueByCatalogName(connectionObj, "Zone Return Air Node or NodeList Name"))
	relation.Nodes = HVACZoneNodes{
		AirNode:      fieldValueByCatalogName(connectionObj, "Zone Air Node Name"),
		InletNodes:   append([]string(nil), zoneInletNodes...),
		ExhaustNodes: append([]string(nil), zoneExhaustNodes...),
		ReturnNodes:  append([]string(nil), zoneReturnNodes...),
		Sources:      zoneNodeSources(ctx, connectionObj),
	}

	if equipmentListName != "" {
		equipmentList, ok := ctx.objectsByTypeName[hvacObjectKey("ZoneHVAC:EquipmentList", equipmentListName)]
		if !ok {
			relation.Warnings = append(relation.Warnings, hvacWarningForObject(connectionObj, "missing_zone_equipment_list",
				fmt.Sprintf("Zone %q references missing ZoneHVAC:EquipmentList %q.", zoneName, equipmentListName)))
		} else {
			relation.ZoneEquipment = equipmentFromZoneEquipmentList(ctx, equipmentList, &relation)
			relation.Evidence = append(relation.Evidence, "ZoneHVAC:EquipmentConnections -> ZoneHVAC:EquipmentList")
		}
	}

	for _, equipment := range relation.ZoneEquipment {
		for _, terminal := range terminalsForZoneEquipment(ctx, equipment) {
			if equipment.ListedInZoneEquipment {
				terminal.ListedInZoneEquipment = true
				terminal.RelationEvidence = appendUniqueString(terminal.RelationEvidence, "ZoneHVAC:EquipmentList")
			}
			if terminal.OutletNode != "" && stringSliceContainsFold(zoneInletNodes, terminal.OutletNode) {
				terminal.OutletMatchesZoneInlet = true
				terminal.RelationEvidence = appendUniqueString(terminal.RelationEvidence, "terminal outlet node matches zone inlet node")
			}
			if !componentSliceContains(relation.TerminalUnits, terminal) {
				relation.TerminalUnits = append(relation.TerminalUnits, terminal)
			}
			if terminal.OutletNode != "" && len(zoneInletNodes) > 0 && !stringSliceContainsFold(zoneInletNodes, terminal.OutletNode) {
				warning := hvacWarningForComponent(terminal, "terminal_not_connected_to_zone_inlet",
					fmt.Sprintf("Terminal %s outlet node %q is not in Zone %q inlet nodes.", componentLabel(terminal), terminal.OutletNode, zoneName))
				warning.ExpectedNodes = append([]string(nil), zoneInletNodes...)
				warning.ActualNode = terminal.OutletNode
				warning.SuggestedFixTarget = "ZoneHVAC:EquipmentConnections/Zone Air Inlet Node or NodeList Name"
				relation.Warnings = append(relation.Warnings, warning)
			}
		}
	}
	for _, terminal := range terminalsByZoneInlet(ctx, zoneInletNodes) {
		terminal.RelationSource = "node_match"
		terminal.RelationConfidence = "medium"
		terminal.OutletMatchesZoneInlet = true
		terminal.RelationEvidence = appendUniqueString(terminal.RelationEvidence, "terminal outlet node matches zone inlet node")
		if !componentSliceContains(relation.TerminalUnits, terminal) {
			relation.TerminalUnits = append(relation.TerminalUnits, terminal)
		}
	}

	airLoopRelations := inferAirLoopRelationsForZone(ctx, loops, relation.TerminalUnits, zoneInletNodes, zoneReturnNodes)
	airLoopNames := hvacLoopRelationNameSet(airLoopRelations)
	plantLoopNames := inferPlantLoopsForZone(ctx, loops, airLoopNames, relation.TerminalUnits, relation.ZoneEquipment)
	relation.AirLoopNames = sortedStringSet(airLoopNames)
	relation.AirLoopRelations = sortedHVACLoopRelations(airLoopRelations)
	relation.PlantLoopNames = sortedStringSet(plantLoopNames)
	relation.PlantLoopRelations = plantLoopRelationsForZone(loops, relation.PlantLoopNames, relation.AirLoopNames)
	relation.CondenserLoopNames = condenserLoopNamesForPlantLoops(loops, relation.PlantLoopNames)
	relation.PlantEquipment = plantSourceEquipmentForLoopNames(loops, relation.PlantLoopNames)
	if len(zoneReturnNodes) > 0 && len(relation.AirLoopNames) == 0 {
		relation.Warnings = append(relation.Warnings, hvacWarningForObject(connectionObj, "zone_return_without_airloop",
			fmt.Sprintf("Zone %q has return node(s) but no AirLoop relation could be inferred.", zoneName)))
	}
	annotateTerminalAirLoopEvidence(ctx, loops, &relation)
	relation.RelationSource, relation.Confidence = zoneRelationSourceConfidence(relation)
	relation.ServiceChains = buildServiceChains(relation)
	return relation
}

func equipmentFromZoneEquipmentList(ctx *hvacContext, equipmentList Object, relation *HVACZoneChain) []HVACComponent {
	var equipment []HVACComponent
	for _, reference := range zoneEquipmentReferences(ctx, equipmentList) {
		objectType := strings.TrimSpace(equipmentList.Fields[reference.TypeIndex].Value)
		objectNameValue := strings.TrimSpace(equipmentList.Fields[reference.NameIndex].Value)
		if objectType == "" && objectNameValue == "" {
			continue
		}
		component := newHVACComponent(ctx, objectType, objectNameValue)
		component.RoleHere = hvacZoneEquipmentRole(component)
		component.ListedInZoneEquipment = true
		component.RelationSource = "explicit"
		component.RelationConfidence = "high"
		component.RelationEvidence = appendUniqueString(component.RelationEvidence, "ZoneHVAC:EquipmentList")
		component.CoolingSequence = hvacFieldValue(equipmentList, reference.CoolingSequenceIndex)
		component.HeatingSequence = hvacFieldValue(equipmentList, reference.HeatingSequenceIndex)
		component.CoolingFractionSchedule = hvacFieldValue(equipmentList, reference.CoolingScheduleIndex)
		component.HeatingFractionSchedule = hvacFieldValue(equipmentList, reference.HeatingScheduleIndex)
		component.EditableFields = append(component.EditableFields, editableZoneEquipmentSequenceFields(ctx.doc, equipmentList, reference.TypeIndex)...)
		if !component.Exists {
			relation.Warnings = append(relation.Warnings, hvacWarningForObject(equipmentList, "missing_zone_equipment",
				fmt.Sprintf("ZoneHVAC:EquipmentList %q references missing %s %q.", objectName(equipmentList), objectType, objectNameValue)))
		}
		equipment = append(equipment, component)
	}
	return equipment
}

type zoneEquipmentReference struct {
	TypeIndex            int
	NameIndex            int
	CoolingSequenceIndex int
	HeatingSequenceIndex int
	CoolingScheduleIndex int
	HeatingScheduleIndex int
}

func zoneEquipmentReferences(ctx *hvacContext, equipmentList Object) []zoneEquipmentReference {
	if references := zoneEquipmentReferencesFromComments(equipmentList); len(references) > 0 {
		return references
	}
	start := bestZoneEquipmentListStartIndex(ctx, equipmentList)
	stride := bestZoneEquipmentListStride(ctx, equipmentList, start)
	var references []zoneEquipmentReference
	for index := start; index+1 < len(equipmentList.Fields); index += stride {
		references = append(references, zoneEquipmentReference{
			TypeIndex:            index,
			NameIndex:            index + 1,
			CoolingSequenceIndex: index + 2,
			HeatingSequenceIndex: index + 3,
			CoolingScheduleIndex: optionalZoneEquipmentIndex(equipmentList, index+4, stride),
			HeatingScheduleIndex: optionalZoneEquipmentIndex(equipmentList, index+5, stride),
		})
	}
	return references
}

func zoneEquipmentReferencesFromComments(equipmentList Object) []zoneEquipmentReference {
	typeIndexes := map[int]int{}
	nameIndexes := map[int]int{}
	var order []int
	for index, field := range equipmentList.Fields {
		comment := normalizeFieldName(field.Comment)
		if !strings.Contains(comment, "equipment") {
			continue
		}
		group := firstPositiveInteger(comment)
		if group == 0 {
			continue
		}
		switch {
		case strings.Contains(comment, "object type"):
			if _, exists := typeIndexes[group]; !exists {
				order = append(order, group)
			}
			typeIndexes[group] = index
		case zoneEquipmentNameComment(comment):
			nameIndexes[group] = index
		}
	}
	sort.Ints(order)
	var references []zoneEquipmentReference
	for _, group := range order {
		typeIndex, hasType := typeIndexes[group]
		nameIndex, hasName := nameIndexes[group]
		if hasType && hasName {
			references = append(references, zoneEquipmentReference{
				TypeIndex:            typeIndex,
				NameIndex:            nameIndex,
				CoolingSequenceIndex: typeIndex + 2,
				HeatingSequenceIndex: typeIndex + 3,
				CoolingScheduleIndex: typeIndex + 4,
				HeatingScheduleIndex: typeIndex + 5,
			})
		}
	}
	return references
}

func optionalZoneEquipmentIndex(equipmentList Object, index int, stride int) int {
	if stride >= 6 && index >= 0 && index < len(equipmentList.Fields) {
		return index
	}
	return -1
}

func zoneEquipmentNameComment(comment string) bool {
	return strings.Contains(comment, "name") &&
		!strings.Contains(comment, "object type") &&
		!strings.Contains(comment, "node") &&
		!strings.Contains(comment, "schedule") &&
		!strings.Contains(comment, "sequence") &&
		!strings.Contains(comment, "fraction") &&
		!strings.Contains(comment, "list")
}

func bestZoneEquipmentListStartIndex(ctx *hvacContext, equipmentList Object) int {
	start := 1
	bestScore := -1
	for _, candidate := range []int{1, 2} {
		score := scoreZoneEquipmentListStart(ctx, equipmentList, candidate)
		if score > bestScore {
			start = candidate
			bestScore = score
		}
	}
	return start
}

func bestZoneEquipmentListStride(ctx *hvacContext, equipmentList Object, start int) int {
	stride6 := scoreZoneEquipmentListStride(ctx, equipmentList, start, 6)
	stride4 := scoreZoneEquipmentListStride(ctx, equipmentList, start, 4)
	if stride6 >= stride4 {
		return 6
	}
	return 4
}

func scoreZoneEquipmentListStride(ctx *hvacContext, equipmentList Object, start int, stride int) int {
	score := 0
	for index := start; index+1 < len(equipmentList.Fields); index += stride {
		objectType := strings.TrimSpace(equipmentList.Fields[index].Value)
		objectNameValue := strings.TrimSpace(equipmentList.Fields[index+1].Value)
		if objectType == "" && objectNameValue == "" {
			continue
		}
		score++
		if isHVACComponentType(objectType) || isAirTerminalType(objectType) {
			score += 2
		}
		if _, ok := ctx.objectsByTypeName[hvacObjectKey(objectType, objectNameValue)]; ok {
			score += 3
		}
		if stride == 6 && index+5 < len(equipmentList.Fields) {
			if strings.TrimSpace(equipmentList.Fields[index+4].Value) != "" || strings.TrimSpace(equipmentList.Fields[index+5].Value) != "" {
				score++
			}
		}
	}
	return score
}

func scoreZoneEquipmentListStart(ctx *hvacContext, equipmentList Object, start int) int {
	score := 0
	for index := start; index+1 < len(equipmentList.Fields); index += 4 {
		objectType := strings.TrimSpace(equipmentList.Fields[index].Value)
		objectNameValue := strings.TrimSpace(equipmentList.Fields[index+1].Value)
		if objectType == "" && objectNameValue == "" {
			continue
		}
		score++
		if isHVACComponentType(objectType) || isAirTerminalType(objectType) {
			score += 2
		}
		if _, ok := ctx.objectsByTypeName[hvacObjectKey(objectType, objectNameValue)]; ok {
			score += 3
		}
	}
	return score
}

func firstPositiveInteger(value string) int {
	number := 0
	reading := false
	for _, char := range value {
		if char >= '0' && char <= '9' {
			reading = true
			number = number*10 + int(char-'0')
			continue
		}
		if reading {
			break
		}
	}
	return number
}

func terminalsByZoneInlet(ctx *hvacContext, zoneInletNodes []string) []HVACComponent {
	if len(zoneInletNodes) == 0 {
		return nil
	}
	var terminals []HVACComponent
	for _, obj := range ctx.doc.Objects {
		if !isAirTerminalType(obj.Type) {
			continue
		}
		component := newHVACComponent(ctx, obj.Type, objectName(obj))
		for _, terminal := range terminalsForZoneEquipment(ctx, component) {
			if terminal.OutletNode != "" && stringSliceContainsFold(zoneInletNodes, terminal.OutletNode) && !componentSliceContains(terminals, terminal) {
				terminals = append(terminals, terminal)
			}
		}
	}
	return terminals
}

func terminalsForZoneEquipment(ctx *hvacContext, equipment HVACComponent) []HVACComponent {
	if !isAirTerminalType(equipment.ObjectType) {
		return nil
	}
	if isAirDistributionUnitType(equipment.ObjectType) {
		if terminal, ok := resolveAirDistributionUnitTerminal(ctx, equipment); ok {
			return []HVACComponent{terminal}
		}
	}
	return []HVACComponent{equipment}
}

func resolveAirDistributionUnitTerminal(ctx *hvacContext, equipment HVACComponent) (HVACComponent, bool) {
	if !isAirDistributionUnitType(equipment.ObjectType) || !equipment.Exists {
		return HVACComponent{}, false
	}
	obj, ok := ctx.objectsByTypeName[hvacObjectKey(equipment.ObjectType, equipment.ObjectName)]
	if !ok {
		return HVACComponent{}, false
	}
	objectType := fieldValueByCatalogName(obj, "Air Terminal Object Type")
	objectNameValue := fieldValueByCatalogName(obj, "Air Terminal Name")
	if objectType == "" && len(obj.Fields) > 2 {
		objectType = strings.TrimSpace(obj.Fields[2].Value)
	}
	if objectNameValue == "" && len(obj.Fields) > 3 {
		objectNameValue = strings.TrimSpace(obj.Fields[3].Value)
	}
	if objectType == "" || objectNameValue == "" {
		return HVACComponent{}, false
	}
	terminal := newHVACComponent(ctx, objectType, objectNameValue)
	if equipment.OutletNode != "" {
		terminal.OutletNode = equipment.OutletNode
	}
	terminal.RoleHere = "zone_terminal"
	terminal.ResolvedFromADU = true
	terminal.DistributionUnitName = equipment.ObjectName
	terminal.RelationSource = "explicit"
	terminal.RelationConfidence = "high"
	terminal.RelationEvidence = appendUniqueString(terminal.RelationEvidence, "ZoneHVAC:AirDistributionUnit")
	return terminal, terminal.ObjectType != "" && terminal.ObjectName != ""
}

func zoneNodeSources(ctx *hvacContext, connectionObj Object) []HVACZoneNodeSource {
	var sources []HVACZoneNodeSource
	for _, item := range []struct {
		role string
		name string
	}{
		{role: "air_node", name: "Zone Air Node Name"},
		{role: "inlet_nodes", name: "Zone Air Inlet Node or NodeList Name"},
		{role: "exhaust_nodes", name: "Zone Air Exhaust Node or NodeList Name"},
		{role: "return_nodes", name: "Zone Return Air Node or NodeList Name"},
	} {
		value, fieldIndex, ok := fieldValueIndexByCatalogName(connectionObj, item.name)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		nodes := expandNodeOrNodeList(ctx, value)
		sourceType := "direct_field"
		sourceObjectType := connectionObj.Type
		sourceObjectName := objectName(connectionObj)
		sourceObjectIndex := connectionObj.Index
		if _, isList := ctx.nodeLists[normalizeName(value)]; isList {
			sourceType = "node_list_expansion"
			if listObj, ok := ctx.objectsByTypeName[hvacObjectKey("NodeList", value)]; ok {
				sourceObjectType = listObj.Type
				sourceObjectName = objectName(listObj)
				sourceObjectIndex = listObj.Index
			}
		}
		sources = append(sources, HVACZoneNodeSource{
			Role:        item.role,
			InputValue:  value,
			Nodes:       append([]string(nil), nodes...),
			SourceType:  sourceType,
			ObjectType:  sourceObjectType,
			ObjectName:  sourceObjectName,
			ObjectIndex: sourceObjectIndex,
			Field:       catalogFieldName(connectionObj, fieldIndex),
		})
	}
	return sources
}

func inferAirLoopRelationsForZone(ctx *hvacContext, loops []HVACLoop, terminals []HVACComponent, zoneInletNodes []string, zoneReturnNodes []string) []HVACLoopRelation {
	relations := map[string]HVACLoopRelation{}
	addRelation := func(loop HVACLoop, source string, confidence string, evidence string) {
		key := normalizeName(loop.Name)
		if key == "" {
			return
		}
		relation := relations[key]
		if relation.LoopName == "" {
			relation = HVACLoopRelation{
				LoopName:   loop.Name,
				LoopType:   loop.Type,
				Source:     source,
				Confidence: confidence,
			}
		}
		relation.Evidence = appendUniqueString(relation.Evidence, evidence)
		if confidenceRank(confidence) < confidenceRank(relation.Confidence) {
			relation.Confidence = confidence
			relation.Source = source
		}
		relations[key] = relation
	}

	for _, loop := range loops {
		if loop.Type != "AirLoopHVAC" {
			continue
		}
		demandNodes := airLoopDemandNodes(ctx, loop)
		for _, terminal := range terminals {
			if terminal.InletNode != "" && demandNodes[normalizeName(terminal.InletNode)] {
				addRelation(loop, "node_graph", "high", fmt.Sprintf("Terminal inlet node %q is on the air-loop demand path.", terminal.InletNode))
			}
			if terminal.OutletNode != "" && stringSliceContainsFold(zoneInletNodes, terminal.OutletNode) && anyNodeInSet(zoneInletNodes, demandNodes) {
				addRelation(loop, "zone_inlet_match", "medium", fmt.Sprintf("Terminal outlet node %q matches a zone inlet also present on the air-loop demand path.", terminal.OutletNode))
			}
		}
		if anyNodeInSet(zoneReturnNodes, demandNodes) {
			addRelation(loop, "zone_return_match", "medium", "Zone return node is present on the air-loop demand path.")
		}
		for _, component := range loopComponents(loop) {
			for _, terminal := range terminals {
				if strings.EqualFold(component.ObjectType, terminal.ObjectType) && strings.EqualFold(component.ObjectName, terminal.ObjectName) {
					addRelation(loop, "branch_component", "high", fmt.Sprintf("Terminal %s appears directly on an AirLoop branch.", componentLabel(terminal)))
				}
			}
		}
	}

	out := make([]HVACLoopRelation, 0, len(relations))
	for _, relation := range relations {
		out = append(out, relation)
	}
	return out
}

func inferAirLoopsForZone(ctx *hvacContext, loops []HVACLoop, terminals []HVACComponent, zoneInletNodes []string, zoneReturnNodes []string) map[string]bool {
	loopNames := map[string]bool{}
	for _, loop := range loops {
		if loop.Type != "AirLoopHVAC" {
			continue
		}
		demandNodes := airLoopDemandNodes(ctx, loop)
		for _, terminal := range terminals {
			if terminal.InletNode != "" && demandNodes[normalizeName(terminal.InletNode)] {
				loopNames[loop.Name] = true
			}
			if terminal.OutletNode != "" && stringSliceContainsFold(zoneInletNodes, terminal.OutletNode) && anyNodeInSet(zoneInletNodes, demandNodes) {
				loopNames[loop.Name] = true
			}
		}
		if anyNodeInSet(zoneReturnNodes, demandNodes) {
			loopNames[loop.Name] = true
		}
		for _, component := range loopComponents(loop) {
			for _, terminal := range terminals {
				if strings.EqualFold(component.ObjectType, terminal.ObjectType) && strings.EqualFold(component.ObjectName, terminal.ObjectName) {
					loopNames[loop.Name] = true
				}
			}
		}
	}
	return loopNames
}

func hvacLoopRelationNameSet(relations []HVACLoopRelation) map[string]bool {
	names := map[string]bool{}
	for _, relation := range relations {
		if relation.LoopName != "" {
			names[relation.LoopName] = true
		}
	}
	return names
}

func sortedHVACLoopRelations(relations []HVACLoopRelation) []HVACLoopRelation {
	out := append([]HVACLoopRelation(nil), relations...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LoopType != out[j].LoopType {
			return out[i].LoopType < out[j].LoopType
		}
		return strings.ToLower(out[i].LoopName) < strings.ToLower(out[j].LoopName)
	})
	return out
}

func plantLoopRelationsForZone(loops []HVACLoop, plantLoopNames []string, airLoopNames []string) []HVACLoopRelation {
	wanted := map[string]bool{}
	for _, name := range plantLoopNames {
		wanted[normalizeName(name)] = true
	}
	airLoops := map[string]bool{}
	for _, name := range airLoopNames {
		airLoops[normalizeName(name)] = true
	}
	relations := map[string]HVACLoopRelation{}
	for _, loop := range loops {
		if loop.Type == "PlantLoop" && wanted[normalizeName(loop.Name)] {
			relation := HVACLoopRelation{
				LoopName:   loop.Name,
				LoopType:   loop.Type,
				Source:     "component_reference",
				Confidence: "inferred",
				Evidence:   []string{"Zone equipment or terminal component references a component on this plant loop."},
			}
			for _, airLoopRelation := range loop.RelatedLoops {
				if airLoops[normalizeName(airLoopRelation.LoopName)] {
					relation.Source = "air_loop_cross_reference"
					relation.Confidence = "medium"
					relation.Evidence = appendUniqueString(relation.Evidence, fmt.Sprintf("AirLoop %q shares %s %q with this plant loop.", airLoopRelation.LoopName, airLoopRelation.ComponentType, airLoopRelation.ComponentName))
				}
			}
			relations[normalizeName(loop.Name)] = relation
		}
	}
	out := make([]HVACLoopRelation, 0, len(relations))
	for _, relation := range relations {
		out = append(out, relation)
	}
	return sortedHVACLoopRelations(out)
}

func condenserLoopNamesForPlantLoops(loops []HVACLoop, plantLoopNames []string) []string {
	plantWanted := map[string]bool{}
	for _, name := range plantLoopNames {
		plantWanted[normalizeName(name)] = true
	}
	condenserNames := map[string]bool{}
	for _, loop := range loops {
		if loop.Type != "PlantLoop" || !plantWanted[normalizeName(loop.Name)] {
			continue
		}
		for _, relation := range loop.RelatedLoops {
			if relation.LoopType == "CondenserLoop" {
				condenserNames[relation.LoopName] = true
			}
		}
	}
	return sortedStringSet(condenserNames)
}

func annotateTerminalAirLoopEvidence(ctx *hvacContext, loops []HVACLoop, relation *HVACZoneChain) {
	for terminalIndex := range relation.TerminalUnits {
		terminal := &relation.TerminalUnits[terminalIndex]
		for _, loop := range loops {
			if loop.Type != "AirLoopHVAC" || !stringSliceContainsFold(relation.AirLoopNames, loop.Name) {
				continue
			}
			demandNodes := airLoopDemandNodes(ctx, loop)
			if terminal.InletNode != "" && demandNodes[normalizeName(terminal.InletNode)] {
				terminal.InletOnAirLoopDemand = true
				terminal.RelationSource = firstNonEmpty(terminal.RelationSource, "node_graph")
				terminal.RelationConfidence = strongestConfidence(terminal.RelationConfidence, "high")
				terminal.RelationEvidence = appendUniqueString(terminal.RelationEvidence, "terminal inlet node is on AirLoop demand path")
			}
		}
		if terminal.OutletMatchesZoneInlet {
			terminal.RelationConfidence = strongestConfidence(terminal.RelationConfidence, "high")
		}
	}
}

func zoneRelationSourceConfidence(relation HVACZoneChain) (string, string) {
	switch {
	case len(relation.AirLoopRelations) > 0 && anyTerminalHasHighEvidence(relation.TerminalUnits):
		return "cross_confirmed", "high"
	case len(relation.ZoneEquipment) > 0:
		return "explicit_zone_equipment", "high"
	case len(relation.AirLoopRelations) > 0 || len(relation.PlantLoopRelations) > 0:
		return "inferred", "medium"
	default:
		return "unresolved", "low"
	}
}

func anyTerminalHasHighEvidence(terminals []HVACComponent) bool {
	for _, terminal := range terminals {
		if terminal.OutletMatchesZoneInlet && terminal.InletOnAirLoopDemand {
			return true
		}
	}
	return false
}

func confidenceRank(confidence string) int {
	switch confidence {
	case "high", "cross_confirmed":
		return 0
	case "medium", "inferred":
		return 1
	case "low", "unresolved":
		return 2
	default:
		return 3
	}
}

func strongestConfidence(current string, candidate string) string {
	if confidenceRank(candidate) < confidenceRank(current) {
		return candidate
	}
	return current
}

func inferPlantLoopsForZone(ctx *hvacContext, loops []HVACLoop, airLoopNames map[string]bool, terminals []HVACComponent, equipment []HVACComponent) map[string]bool {
	plantNames := map[string]bool{}
	for _, loop := range loops {
		if !airLoopNames[loop.Name] {
			continue
		}
		for _, relation := range loop.RelatedLoops {
			if relation.LoopType == "PlantLoop" {
				plantNames[relation.LoopName] = true
			}
		}
	}
	for _, component := range append(append([]HVACComponent{}, terminals...), equipment...) {
		keys := referencedHVACComponentKeysDepth(ctx, component, 3)
		if key := hvacComponentKey(component); key != "" {
			keys[key] = true
		}
		for key := range keys {
			for loopName, loopType := range ctx.componentLoopTypes[key] {
				if loopType == "PlantLoop" {
					plantNames[loopName] = true
				}
			}
		}
	}
	return plantNames
}

func referencedHVACComponentKeys(ctx *hvacContext, component HVACComponent) map[string]bool {
	return referencedHVACComponentKeysDepth(ctx, component, 1)
}

func referencedHVACComponentKeysDepth(ctx *hvacContext, component HVACComponent, depth int) map[string]bool {
	keys := map[string]bool{}
	visited := map[string]bool{}
	var visit func(HVACComponent, int)
	visit = func(current HVACComponent, remaining int) {
		if remaining < 0 || !current.Exists {
			return
		}
		currentKey := hvacComponentKey(current)
		if currentKey == "" || visited[currentKey] {
			return
		}
		visited[currentKey] = true
		obj, ok := ctx.objectsByTypeName[hvacObjectKey(current.ObjectType, current.ObjectName)]
		if !ok {
			return
		}
		for _, field := range obj.Fields {
			value := strings.TrimSpace(field.Value)
			if value == "" {
				continue
			}
			for _, candidate := range ctx.objectsByName[normalizeName(value)] {
				if !isHVACComponentType(candidate.Type) && !isAirTerminalType(candidate.Type) {
					continue
				}
				key := hvacObjectKey(candidate.Type, objectName(candidate))
				if key == "" || key == currentKey {
					continue
				}
				keys[key] = true
				if remaining > 0 {
					visit(newHVACComponent(ctx, candidate.Type, objectName(candidate)), remaining-1)
				}
			}
		}
	}
	visit(component, depth)
	return keys
}

func componentReferencedByZoneHVAC(ctx *hvacContext, wantedKey string) bool {
	if wantedKey == "" {
		return false
	}
	for _, obj := range ctx.doc.Objects {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(obj.Type)), "zonehvac:") && !isAirTerminalType(obj.Type) {
			continue
		}
		name := objectName(obj)
		if name == "" {
			continue
		}
		component := newHVACComponent(ctx, obj.Type, name)
		if keys := referencedHVACComponentKeysDepth(ctx, component, 4); keys[wantedKey] {
			return true
		}
	}
	return false
}

func plantSourceEquipmentForLoopNames(loops []HVACLoop, loopNames []string) []HVACComponent {
	wanted := map[string]bool{}
	for _, loopName := range loopNames {
		wanted[normalizeName(loopName)] = true
	}
	seen := map[string]bool{}
	var equipment []HVACComponent
	for _, loop := range loops {
		if loop.Type != "PlantLoop" || !wanted[normalizeName(loop.Name)] {
			continue
		}
		for _, component := range loop.SupplySideComponents() {
			if !isPlantSourceEquipmentType(component.ObjectType) {
				continue
			}
			key := hvacComponentKey(component)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			component.LoopName = loop.Name
			component.RoleHere = hvacComponentContextRole(component, loop.Type, "Supply")
			equipment = append(equipment, component)
		}
	}
	sort.Slice(equipment, func(i, j int) bool {
		if !strings.EqualFold(equipment[i].ObjectType, equipment[j].ObjectType) {
			return strings.ToLower(equipment[i].ObjectType) < strings.ToLower(equipment[j].ObjectType)
		}
		return strings.ToLower(equipment[i].ObjectName) < strings.ToLower(equipment[j].ObjectName)
	})
	return equipment
}

func (loop HVACLoop) SupplySideComponents() []HVACComponent {
	var components []HVACComponent
	for _, branch := range loop.SupplySide.Branches {
		components = append(components, branch.Components...)
	}
	return components
}

func buildServiceChains(relation HVACZoneChain) []HVACServicePath {
	var paths []HVACServicePath
	if len(relation.TerminalUnits) == 0 && len(relation.ZoneEquipment) == 0 {
		return nil
	}
	for _, terminal := range relation.TerminalUnits {
		if len(relation.AirLoopNames) == 0 && len(relation.PlantLoopNames) == 0 {
			paths = append(paths, HVACServicePath{
				ZoneName:        relation.ZoneName,
				TerminalName:    componentLabel(terminal),
				Confidence:      "medium",
				Evidence:        "terminal outlet matches zone inlet",
				SourceRelations: []string{"terminal_outlet_node", "zone_inlet_node"},
			})
			continue
		}
		for _, airLoopName := range append([]string{""}, relation.AirLoopNames...) {
			for _, plantLoopName := range append([]string{""}, relation.PlantLoopNames...) {
				if airLoopName == "" && plantLoopName == "" {
					continue
				}
				sourceComponents := plantSourceComponentsForServicePath(relation, plantLoopName)
				for _, sourceComponent := range sourceComponents {
					paths = append(paths, HVACServicePath{
						ZoneName:        relation.ZoneName,
						TerminalName:    componentLabel(terminal),
						AirLoopName:     airLoopName,
						PlantLoop:       plantLoopName,
						SourceComponent: sourceComponent,
						Confidence:      "inferred",
						Evidence:        "node/reference relation",
						SourceRelations: []string{"air_loop_demand_nodes", "zone_equipment_references", "plant_loop_component_references"},
					})
				}
			}
		}
	}
	if len(paths) == 0 {
		for _, equipment := range relation.ZoneEquipment {
			paths = append(paths, HVACServicePath{
				ZoneName:        relation.ZoneName,
				Component:       componentLabel(equipment),
				Confidence:      "high",
				Evidence:        "ZoneHVAC:EquipmentList",
				SourceRelations: []string{"zone_equipment_list"},
			})
		}
	}
	return paths
}

func plantSourceComponentsForServicePath(relation HVACZoneChain, plantLoopName string) []string {
	var labels []string
	if plantLoopName == "" {
		return []string{""}
	}
	for _, component := range relation.PlantEquipment {
		if component.LoopName != "" && !strings.EqualFold(component.LoopName, plantLoopName) {
			continue
		}
		labels = append(labels, componentLabel(component))
	}
	if len(labels) == 0 {
		return []string{""}
	}
	return labels
}

func airLoopDemandNodes(ctx *hvacContext, loop HVACLoop) map[string]bool {
	nodes := map[string]bool{}
	addNode := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			nodes[normalizeName(value)] = true
		}
	}
	addNode(loop.DemandSide.InletNode)
	addNode(loop.DemandSide.OutletNode)
	for _, obj := range ctx.objectsByType[normalizeFieldCatalogKey("AirLoopHVAC:SupplyPath")] {
		inlet := fieldValueByCatalogName(obj, "Supply Air Path Inlet Node Name")
		if !strings.EqualFold(inlet, loop.DemandSide.InletNode) {
			continue
		}
		for _, usage := range nodeUsagesForObject(ctx, obj) {
			addNode(usage.NodeName)
		}
		for index := 2; index+1 < len(obj.Fields); index += 2 {
			componentType := strings.TrimSpace(obj.Fields[index].Value)
			componentName := strings.TrimSpace(obj.Fields[index+1].Value)
			componentObj, ok := ctx.objectsByTypeName[hvacObjectKey(componentType, componentName)]
			if !ok {
				continue
			}
			for _, usage := range nodeUsagesForObject(ctx, componentObj) {
				addNode(usage.NodeName)
			}
		}
	}
	for _, obj := range ctx.objectsByType[normalizeFieldCatalogKey("AirLoopHVAC:ReturnPath")] {
		outlet := fieldValueByCatalogName(obj, "Return Air Path Outlet Node Name")
		if !strings.EqualFold(outlet, loop.DemandSide.OutletNode) {
			continue
		}
		for _, usage := range nodeUsagesForObject(ctx, obj) {
			addNode(usage.NodeName)
		}
		for index := 2; index+1 < len(obj.Fields); index += 2 {
			componentType := strings.TrimSpace(obj.Fields[index].Value)
			componentName := strings.TrimSpace(obj.Fields[index+1].Value)
			componentObj, ok := ctx.objectsByTypeName[hvacObjectKey(componentType, componentName)]
			if !ok {
				continue
			}
			for _, usage := range nodeUsagesForObject(ctx, componentObj) {
				addNode(usage.NodeName)
			}
		}
	}
	return nodes
}

func applyLoopZoneRelations(loops []HVACLoop, relations []HVACZoneChain) {
	zoneNamesByLoop := map[string]map[string]bool{}
	for _, relation := range relations {
		for _, loopName := range relation.AirLoopNames {
			if zoneNamesByLoop[loopName] == nil {
				zoneNamesByLoop[loopName] = map[string]bool{}
			}
			zoneNamesByLoop[loopName][relation.ZoneName] = true
		}
		for _, loopName := range relation.PlantLoopNames {
			if zoneNamesByLoop[loopName] == nil {
				zoneNamesByLoop[loopName] = map[string]bool{}
			}
			zoneNamesByLoop[loopName][relation.ZoneName] = true
		}
	}
	for index := range loops {
		loops[index].RelatedZones = sortedStringSet(zoneNamesByLoop[loops[index].Name])
	}
}

func collectHVACNodeUsages(ctx *hvacContext, obj Object) {
	for index, field := range obj.Fields {
		value := strings.TrimSpace(field.Value)
		if value == "" {
			continue
		}
		role := hvacNodeFieldRole(obj, index, field)
		if role == "" {
			continue
		}
		fieldName := catalogFieldName(obj, index)
		if fieldName == "" {
			fieldName = field.Comment
		}
		nodes := []string{value}
		if role == "node_list" || strings.Contains(role, "node_list") {
			if expanded := expandNodeOrNodeList(ctx, value); len(expanded) > 0 {
				nodes = expanded
			}
		}
		for _, nodeName := range nodes {
			usage := HVACNodeUsage{
				NodeName:    nodeName,
				ObjectType:  obj.Type,
				ObjectName:  objectName(obj),
				ObjectIndex: obj.Index,
				FieldIndex:  index,
				FieldName:   fieldName,
				Role:        role,
			}
			ctx.nodeUsages = append(ctx.nodeUsages, usage)
			ctx.nodeUsagesByName[normalizeName(nodeName)] = append(ctx.nodeUsagesByName[normalizeName(nodeName)], usage)
		}
	}
}

func hvacNodeFieldRole(obj Object, fieldIndex int, field Field) string {
	if strings.EqualFold(obj.Type, "NodeList") && fieldIndex > 0 {
		return "node_list_member"
	}
	catalogRole := catalogFieldRole(obj, fieldIndex)
	comment := normalizeFieldName(field.Comment)
	hasNodeWords := strings.Contains(comment, "node") && (strings.Contains(comment, "name") || strings.Contains(comment, "list"))
	if catalogRole != fieldRoleNodeRef && catalogRole != fieldRoleNodeListRef && !hasNodeWords {
		return ""
	}
	if strings.EqualFold(obj.Type, "OutdoorAir:Mixer") {
		switch {
		case strings.Contains(comment, "mixed"):
			return "oa_mixer_mixed_air"
		case strings.Contains(comment, "outdoor"):
			return "outdoor_air"
		case strings.Contains(comment, "relief"):
			return "relief_air"
		case strings.Contains(comment, "return"):
			return "return_air"
		}
	}
	listSuffix := ""
	if catalogRole == fieldRoleNodeListRef || strings.Contains(comment, "node list") || strings.Contains(comment, "nodelist") {
		listSuffix = "_node_list"
	}
	switch {
	case strings.Contains(comment, "condenser") && strings.Contains(comment, "inlet"):
		return "condenser_inlet" + listSuffix
	case strings.Contains(comment, "condenser") && strings.Contains(comment, "outlet"):
		return "condenser_outlet" + listSuffix
	case strings.Contains(comment, "water") && strings.Contains(comment, "inlet"):
		return "water_inlet" + listSuffix
	case strings.Contains(comment, "water") && strings.Contains(comment, "outlet"):
		return "water_outlet" + listSuffix
	case strings.Contains(comment, "plant") && strings.Contains(comment, "inlet"):
		return "plant_inlet" + listSuffix
	case strings.Contains(comment, "plant") && strings.Contains(comment, "outlet"):
		return "plant_outlet" + listSuffix
	case strings.Contains(comment, "return"):
		return "zone_return" + listSuffix
	case strings.Contains(comment, "exhaust"):
		return "zone_exhaust" + listSuffix
	case strings.Contains(comment, "zone") && strings.Contains(comment, "inlet"):
		return "zone_inlet" + listSuffix
	case strings.Contains(comment, "zone") && strings.Contains(comment, "outlet"):
		return "zone_outlet" + listSuffix
	case strings.Contains(comment, "air") && strings.Contains(comment, "inlet"):
		return "air_inlet" + listSuffix
	case strings.Contains(comment, "air") && strings.Contains(comment, "outlet"):
		return "air_outlet" + listSuffix
	case strings.Contains(comment, "inlet"):
		return "inlet" + listSuffix
	case strings.Contains(comment, "outlet"):
		return "outlet" + listSuffix
	case strings.Contains(comment, "setpoint"):
		return "setpoint" + listSuffix
	case listSuffix != "":
		return "node_list"
	default:
		return "node"
	}
}

func expandNodeOrNodeList(ctx *hvacContext, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if nodes := ctx.nodeLists[normalizeName(value)]; len(nodes) > 0 {
		return append([]string(nil), nodes...)
	}
	return []string{value}
}

func nodeUsagesForObject(ctx *hvacContext, obj Object) []HVACNodeUsage {
	var usages []HVACNodeUsage
	for _, usage := range ctx.nodeUsages {
		if usage.ObjectIndex == obj.Index {
			usages = append(usages, usage)
		}
	}
	return usages
}

func editableHVACFields(doc Document, obj Object) []HVACEditField {
	if !isHVACEditableObjectType(obj.Type) {
		return nil
	}
	var fields []HVACEditField
	for fieldIndex := range obj.Fields {
		editField, ok := hvacEditableFieldAt(doc, obj, fieldIndex)
		if ok {
			fields = append(fields, editField)
		}
	}
	return fields
}

func editableZoneEquipmentSequenceFields(doc Document, obj Object, equipmentStartIndex int) []HVACEditField {
	var fields []HVACEditField
	for _, fieldIndex := range []int{equipmentStartIndex + 2, equipmentStartIndex + 3} {
		if fieldIndex < 0 || fieldIndex >= len(obj.Fields) {
			continue
		}
		fieldName := "Equipment sequence"
		if fieldIndex == equipmentStartIndex+2 {
			fieldName = "Cooling sequence"
		}
		if fieldIndex == equipmentStartIndex+3 {
			fieldName = "Heating/no-load sequence"
		}
		fields = append(fields, HVACEditField{
			ObjectIndex:     obj.Index,
			ObjectType:      obj.Type,
			ObjectName:      objectName(obj),
			FieldIndex:      fieldIndex,
			FieldName:       fieldName,
			CurrentValue:    strings.TrimSpace(obj.Fields[fieldIndex].Value),
			EditKind:        "sequence",
			ValueType:       "integer",
			RequiresPreview: true,
			Impact:          "Changes ZoneHVAC equipment priority order.",
		})
	}
	return fields
}

func hvacEditableFieldAt(doc Document, obj Object, fieldIndex int) (HVACEditField, bool) {
	if fieldIndex < 0 || fieldIndex >= len(obj.Fields) || !isHVACEditableObjectType(obj.Type) {
		return HVACEditField{}, false
	}
	field := obj.Fields[fieldIndex]
	fieldName := catalogFieldName(obj, fieldIndex)
	if strings.TrimSpace(fieldName) == "" {
		fieldName = field.Comment
	}
	editKind, valueType, allowAutosize := classifyHVACEditableField(obj, fieldIndex, fieldName)
	if editKind == "" {
		return HVACEditField{}, false
	}
	editField := HVACEditField{
		ObjectIndex:     obj.Index,
		ObjectType:      obj.Type,
		ObjectName:      objectName(obj),
		FieldIndex:      fieldIndex,
		FieldName:       fieldName,
		CurrentValue:    strings.TrimSpace(field.Value),
		EditKind:        editKind,
		ValueType:       valueType,
		AllowAutosize:   allowAutosize,
		RequiresPreview: true,
		Impact:          hvacEditImpact(editKind),
	}
	editField.SuggestedValues = hvacEditSuggestions(doc, editField)
	return editField, true
}

func classifyHVACEditableField(obj Object, fieldIndex int, fieldName string) (string, string, bool) {
	normalized := normalizeFieldName(fieldName)
	if strings.EqualFold(obj.Type, "ZoneHVAC:EquipmentList") && fieldIndex > 0 {
		if strings.Contains(normalized, "sequence") || fieldIndex%4 == 3 || fieldIndex%4 == 0 {
			return "sequence", "integer", false
		}
	}
	switch {
	case strings.Contains(normalized, "availability") && strings.Contains(normalized, "schedule"):
		return "availability_schedule", "reference", false
	case strings.Contains(normalized, "schedule") && strings.Contains(normalized, "name"):
		return "schedule", "reference", false
	case strings.Contains(normalized, "flow rate") || strings.Contains(normalized, "air flow"):
		return "flow", "number", true
	case strings.Contains(normalized, "capacity"):
		return "capacity", "number", true
	case strings.Contains(normalized, "sequence"):
		return "sequence", "integer", false
	default:
		return "", "", false
	}
}

func hvacEditSuggestions(doc Document, field HVACEditField) []FieldValueSuggestion {
	switch field.EditKind {
	case "availability_schedule", "schedule":
		return objectNameSuggestionsByPredicate(doc, func(objectType string) bool {
			return isScheduleType(objectType)
		})
	case "flow", "capacity":
		if field.AllowAutosize {
			return []FieldValueSuggestion{{Value: "Autosize", Source: "catalog"}}
		}
	case "sequence":
		return []FieldValueSuggestion{
			{Value: "1", Source: "catalog"},
			{Value: "2", Source: "catalog"},
			{Value: "3", Source: "catalog"},
		}
	}
	return nil
}

func hvacEditImpact(editKind string) string {
	switch editKind {
	case "availability_schedule", "schedule":
		return "Changes when the component or equipment is available."
	case "flow":
		return "Changes design or maximum flow sizing for the selected HVAC object."
	case "capacity":
		return "Changes component capacity sizing or overrides Autosize."
	case "sequence":
		return "Changes ZoneHVAC equipment priority order."
	default:
		return "Changes an HVAC object field."
	}
}

func objectNameSuggestionsByPredicate(doc Document, match func(string) bool) []FieldValueSuggestion {
	var suggestions []FieldValueSuggestion
	for _, obj := range doc.Objects {
		if !match(obj.Type) {
			continue
		}
		if name := objectName(obj); name != "" {
			suggestions = append(suggestions, FieldValueSuggestion{Value: name, Label: obj.Type, Source: "document"})
		}
	}
	return uniqueFieldSuggestions(suggestions)
}

func hvacConnectionDiagnostics(doc Document) []Diagnostic {
	report := AnalyzeHVAC(doc)
	var diagnostics []Diagnostic
	for _, warning := range report.Warnings {
		diagnostics = append(diagnostics, Diagnostic{
			Severity:    warning.Severity,
			Category:    warning.Category,
			Code:        warning.Code,
			Message:     warning.Message,
			ObjectIndex: warning.ObjectIndex,
			ObjectType:  warning.ObjectType,
			ObjectName:  warning.ObjectName,
			FieldIndex:  warning.FieldIndex,
			Field:       warning.Field,
			Value:       warning.Value,
		})
	}
	return diagnostics
}

func collectHVACWarnings(ctx *hvacContext, report HVACReport) []HVACWarning {
	var warnings []HVACWarning
	warnings = append(warnings, ctx.warnings...)
	for _, loop := range report.Loops {
		warnings = append(warnings, loop.Warnings...)
		warnings = append(warnings, loop.SupplySide.Warnings...)
		warnings = append(warnings, loop.DemandSide.Warnings...)
		for _, branch := range append(append([]HVACBranch{}, loop.SupplySide.Branches...), loop.DemandSide.Branches...) {
			warnings = append(warnings, branch.Warnings...)
		}
		for _, connector := range append(append([]HVACConnector{}, loop.SupplySide.Connectors...), loop.DemandSide.Connectors...) {
			warnings = append(warnings, connector.Warnings...)
		}
	}
	for _, relation := range report.ZoneRelations {
		warnings = append(warnings, relation.Warnings...)
	}
	return uniqueHVACWarnings(warnings)
}

func sortHVACReport(report *HVACReport) {
	sort.Slice(report.Loops, func(i, j int) bool {
		if report.Loops[i].Type == report.Loops[j].Type {
			return strings.ToLower(report.Loops[i].Name) < strings.ToLower(report.Loops[j].Name)
		}
		return report.Loops[i].Type < report.Loops[j].Type
	})
	sort.Slice(report.NodeUsages, func(i, j int) bool {
		if !strings.EqualFold(report.NodeUsages[i].NodeName, report.NodeUsages[j].NodeName) {
			return strings.ToLower(report.NodeUsages[i].NodeName) < strings.ToLower(report.NodeUsages[j].NodeName)
		}
		return report.NodeUsages[i].ObjectIndex < report.NodeUsages[j].ObjectIndex
	})
	sort.Slice(report.Warnings, func(i, j int) bool {
		if report.Warnings[i].Severity != report.Warnings[j].Severity {
			return report.Warnings[i].Severity == DiagnosticError
		}
		if report.Warnings[i].Code != report.Warnings[j].Code {
			return report.Warnings[i].Code < report.Warnings[j].Code
		}
		return report.Warnings[i].ObjectIndex < report.Warnings[j].ObjectIndex
	})
}

func uniqueHVACWarnings(warnings []HVACWarning) []HVACWarning {
	seen := map[string]bool{}
	var out []HVACWarning
	for _, warning := range warnings {
		key := strings.Join([]string{
			warning.Severity,
			warning.Code,
			warning.Message,
			fmt.Sprint(warning.ObjectIndex),
			fmt.Sprint(warning.FieldIndex),
		}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, warning)
	}
	return out
}

func hvacWarningForObject(obj Object, code string, message string) HVACWarning {
	return HVACWarning{
		Severity:    DiagnosticWarning,
		Category:    "HVAC Connection",
		Code:        code,
		Message:     message,
		ObjectIndex: obj.Index,
		ObjectType:  obj.Type,
		ObjectName:  objectName(obj),
	}
}

func hvacWarningForComponent(component HVACComponent, code string, message string) HVACWarning {
	return HVACWarning{
		Severity:    DiagnosticWarning,
		Category:    "HVAC Connection",
		Code:        code,
		Message:     message,
		ObjectIndex: component.ObjectIndex,
		ObjectType:  component.ObjectType,
		ObjectName:  component.ObjectName,
	}
}

func zoneObjectIndex(ctx *hvacContext, zoneName string) int {
	for _, objectType := range []string{"Zone", "Space"} {
		obj, ok := ctx.objectsByTypeName[hvacObjectKey(objectType, zoneName)]
		if ok {
			return obj.Index
		}
	}
	return -1
}

func loopComponents(loop HVACLoop) []HVACComponent {
	var components []HVACComponent
	for _, side := range []HVACLoopSide{loop.SupplySide, loop.DemandSide} {
		for _, branch := range side.Branches {
			components = append(components, branch.Components...)
		}
	}
	return components
}

func buildHVACNodeEdges(loops []HVACLoop) []HVACNodeEdge {
	var edges []HVACNodeEdge
	for _, loop := range loops {
		for _, branch := range append(append([]HVACBranch{}, loop.SupplySide.Branches...), loop.DemandSide.Branches...) {
			for index, component := range branch.Components {
				label := componentLabel(component)
				if component.OutletNode != "" {
					edge := HVACNodeEdge{
						NodeName:      component.OutletNode,
						FromComponent: label,
						ToNode:        component.OutletNode,
						Direction:     "producer",
					}
					if index+1 < len(branch.Components) {
						edge.ToComponent = componentLabel(branch.Components[index+1])
					}
					edges = append(edges, edge)
				}
				if component.InletNode != "" {
					edges = append(edges, HVACNodeEdge{
						NodeName:    component.InletNode,
						FromNode:    component.InletNode,
						ToComponent: label,
						Direction:   "consumer",
					})
				}
			}
		}
	}
	return edges
}

func mutateLoopComponents(loop *HVACLoop, mutate func(*HVACComponent)) {
	for branchIndex := range loop.SupplySide.Branches {
		for componentIndex := range loop.SupplySide.Branches[branchIndex].Components {
			mutate(&loop.SupplySide.Branches[branchIndex].Components[componentIndex])
		}
	}
	for branchIndex := range loop.DemandSide.Branches {
		for componentIndex := range loop.DemandSide.Branches[branchIndex].Components {
			mutate(&loop.DemandSide.Branches[branchIndex].Components[componentIndex])
		}
	}
}

func loopSideContainsComponent(side HVACLoopSide, component HVACComponent) bool {
	key := hvacComponentKey(component)
	for _, branch := range side.Branches {
		for _, candidate := range branch.Components {
			if hvacComponentKey(candidate) == key {
				return true
			}
		}
	}
	return false
}

func hvacObjectKey(objectType string, objectNameValue string) string {
	return normalizeFieldCatalogKey(objectType) + "|" + normalizeName(objectNameValue)
}

func hvacComponentKey(component HVACComponent) string {
	if strings.TrimSpace(component.ObjectType) == "" || strings.TrimSpace(component.ObjectName) == "" {
		return ""
	}
	return hvacObjectKey(component.ObjectType, component.ObjectName)
}

func hvacFieldValue(obj Object, index int) string {
	if index < 0 || index >= len(obj.Fields) {
		return ""
	}
	return strings.TrimSpace(obj.Fields[index].Value)
}

func componentLabel(component HVACComponent) string {
	if component.ObjectName == "" {
		return component.ObjectType
	}
	if component.ObjectType == "" {
		return component.ObjectName
	}
	return component.ObjectType + " " + component.ObjectName
}

func hvacComponentFamily(objectType string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	switch {
	case strings.HasPrefix(lower, "fan:"):
		return "fan", "Fans"
	case strings.HasPrefix(lower, "coil:cooling"):
		return "cooling_coil", "Cooling coils"
	case strings.HasPrefix(lower, "coil:heating"):
		return "heating_coil", "Heating coils"
	case strings.HasPrefix(lower, "coil:"):
		return "coil", "Coils"
	case strings.HasPrefix(lower, "pump:"):
		return "pump", "Pumps"
	case strings.HasPrefix(lower, "chiller:"):
		return "chiller", "Chillers"
	case strings.HasPrefix(lower, "boiler:"):
		return "boiler", "Boilers"
	case strings.HasPrefix(lower, "coolingtower:"):
		return "cooling_tower", "Cooling towers"
	case strings.HasPrefix(lower, "heatpump:"):
		return "heat_pump", "Heat pumps"
	case strings.Contains(lower, "unitarysystem"):
		return "unitary_system", "Unitary systems"
	case strings.HasPrefix(lower, "waterheater:"):
		return "water_heater", "Water heaters"
	case strings.HasPrefix(lower, "thermalstorage:"):
		return "thermal_storage", "Thermal storage"
	case strings.HasPrefix(lower, "pipe:"):
		return "pipe", "Pipes"
	case strings.HasPrefix(lower, "outdoorair:") || strings.HasPrefix(lower, "airloophvac:outdoorairsystem"):
		return "outdoor_air", "Outdoor air systems"
	case strings.HasPrefix(lower, "controller:"):
		return "controller", "Controllers"
	case strings.HasPrefix(lower, "setpointmanager:"):
		return "setpoint_manager", "Setpoint managers"
	case strings.HasPrefix(lower, "availabilitymanager:"):
		return "availability_manager", "Availability managers"
	case strings.HasPrefix(lower, "districtcooling"):
		return "district_cooling", "District cooling"
	case strings.HasPrefix(lower, "districtheating"):
		return "district_heating", "District heating"
	case strings.HasPrefix(lower, "heat exchanger:") || strings.HasPrefix(lower, "heatexchanger:"):
		return "heat_exchanger", "Heat exchangers"
	case strings.HasPrefix(lower, "plantcomponent:"):
		return "plant_component", "Plant components"
	case isAirTerminalType(objectType):
		return "terminal", "Air terminals"
	case strings.HasPrefix(lower, "zonehvac:"):
		return "zone_hvac", "Zone HVAC"
	case strings.HasPrefix(lower, "airloophvac:"):
		return "air_distribution", "Air distribution"
	default:
		return "other", "Other HVAC"
	}
}

func hvacComponentDisplayLabel(objectType string) string {
	family, _ := hvacComponentFamily(objectType)
	switch family {
	case "fan":
		return "Fan"
	case "cooling_coil":
		return "Cooling Coil"
	case "heating_coil":
		return "Heating Coil"
	case "coil":
		return "Coil"
	case "pump":
		return "Pump"
	case "pipe":
		return "Pipe"
	case "chiller":
		return "Chiller"
	case "boiler":
		return "Boiler"
	case "cooling_tower":
		return "Cooling Tower"
	case "heat_pump":
		return "Heat Pump"
	case "water_heater":
		return "Water Heater"
	case "thermal_storage":
		return "Thermal Storage"
	case "heat_exchanger":
		return "Heat Exchanger"
	case "district_cooling":
		return "District Cooling"
	case "district_heating":
		return "District Heating"
	case "terminal":
		return "Air Terminal"
	case "zone_hvac":
		return "Zone HVAC"
	case "unitary_system":
		return "Unitary System"
	case "outdoor_air":
		return "Outdoor Air"
	case "controller":
		return "Controller"
	case "setpoint_manager":
		return "Setpoint Manager"
	case "availability_manager":
		return "Availability Manager"
	case "plant_component":
		return "Plant Component"
	case "air_distribution":
		return "Air Distribution"
	default:
		if strings.TrimSpace(objectType) == "" {
			return "Unknown HVAC object"
		}
		return "HVAC Component"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func containsMapValue(values map[string]string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func stringSliceContainsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(wanted)) {
			return true
		}
	}
	return false
}

func anyNodeInSet(nodes []string, nodeSet map[string]bool) bool {
	for _, node := range nodes {
		if nodeSet[normalizeName(node)] {
			return true
		}
	}
	return false
}

func sortedStringSet(values map[string]bool) []string {
	var out []string
	for value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func componentSliceContains(components []HVACComponent, wanted HVACComponent) bool {
	wantedKey := hvacComponentKey(wanted)
	for _, component := range components {
		if hvacComponentKey(component) == wantedKey && wantedKey != "" {
			return true
		}
	}
	return false
}

func isAirTerminalType(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	return strings.HasPrefix(lower, "airterminal:") ||
		strings.Contains(lower, "airterminal") ||
		isAirDistributionUnitType(lower)
}

func isAirDistributionUnitType(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	return lower == "zonehvac:airdistributionunit" || strings.Contains(lower, "airdistributionunit")
}

func isWaterCoilType(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	return strings.Contains(lower, "coil:cooling:water") ||
		strings.Contains(lower, "coil:heating:water") ||
		(strings.Contains(lower, "coil") && strings.Contains(lower, "water"))
}

func isHVACComponentType(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	return strings.HasPrefix(lower, "coil:") ||
		strings.HasPrefix(lower, "fan:") ||
		strings.HasPrefix(lower, "pump:") ||
		strings.HasPrefix(lower, "chiller:") ||
		strings.HasPrefix(lower, "boiler:") ||
		strings.HasPrefix(lower, "airterminal:") ||
		strings.HasPrefix(lower, "zonehvac:") ||
		strings.HasPrefix(lower, "airloophvac:") ||
		strings.HasPrefix(lower, "plantcomponent:") ||
		strings.HasPrefix(lower, "districtcooling") ||
		strings.HasPrefix(lower, "districtheating") ||
		strings.HasPrefix(lower, "coolingtower:") ||
		strings.HasPrefix(lower, "heatpump:") ||
		strings.Contains(lower, "unitarysystem") ||
		strings.HasPrefix(lower, "pipe:") ||
		strings.HasPrefix(lower, "outdoorair:") ||
		strings.HasPrefix(lower, "waterheater:") ||
		strings.HasPrefix(lower, "thermalstorage:") ||
		strings.HasPrefix(lower, "heat exchanger:") ||
		strings.HasPrefix(lower, "heatexchanger:") ||
		isHVACControlType(objectType)
}

func isHVACControlType(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	return strings.HasPrefix(lower, "controller:") ||
		strings.HasPrefix(lower, "setpointmanager:") ||
		strings.HasPrefix(lower, "availabilitymanager:")
}

func isPlantSourceEquipmentType(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	return strings.HasPrefix(lower, "chiller:") ||
		strings.HasPrefix(lower, "boiler:") ||
		strings.HasPrefix(lower, "districtcooling") ||
		strings.HasPrefix(lower, "districtheating") ||
		strings.HasPrefix(lower, "heatpump:") ||
		strings.HasPrefix(lower, "plantcomponent:") ||
		strings.HasPrefix(lower, "coolingtower:") ||
		strings.HasPrefix(lower, "waterheater:") ||
		strings.HasPrefix(lower, "thermalstorage:") ||
		strings.HasPrefix(lower, "heat exchanger:") ||
		strings.HasPrefix(lower, "heatexchanger:")
}

func isHVACEditableObjectType(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	return lower == "airloophvac" ||
		lower == "plantloop" ||
		lower == "zonehvac:equipmentlist" ||
		strings.HasPrefix(lower, "coil:") ||
		strings.HasPrefix(lower, "fan:") ||
		strings.HasPrefix(lower, "pump:") ||
		strings.HasPrefix(lower, "chiller:") ||
		strings.HasPrefix(lower, "boiler:") ||
		strings.HasPrefix(lower, "airterminal:") ||
		strings.HasPrefix(lower, "zonehvac:")
}
