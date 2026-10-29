package simulation

import (
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// A run's loop membership comes from the input that was actually executed. The
// current editor may already contain a different model when the run is viewed.
func buildHVACLoopRunResultsWithDocument(series []SimulationSeries, request SimulationPurposeRequest, doc *idf.Document) []HVACLoopRunResult {
	return buildHVACLoopRunResultsWithDocumentWithExecutor(series, request, doc, nil)
}

func buildHVACLoopRunResultsWithDocumentWithExecutor(series []SimulationSeries, request SimulationPurposeRequest, doc *idf.Document, executor *resultBuildExecutor) []HVACLoopRunResult {
	if doc == nil {
		return buildHVACLoopRunResults(series, request)
	}
	report := idf.AnalyzeHVAC(*doc)
	if len(report.Loops) == 0 && len(report.OperatingProfiles) == 0 {
		return buildHVACLoopRunResults(series, request)
	}
	loops := make([]idf.HVACLoop, 0, len(report.Loops))
	for _, loop := range report.Loops {
		if hvacResultLoopInScope(loop, request.Scope) {
			loops = append(loops, loop)
		}
	}
	local := hvacRuntimeLocalProfiles(report, request.Scope)
	count := len(loops)
	if len(local) > 0 {
		count++
	}
	if count == 0 {
		return []HVACLoopRunResult{}
	}
	context := hvacLoopResultBuildContext{
		series: series, request: request, doc: doc, report: report,
		selectedComponents: purposeComponentIDSet(request.Scope.ComponentIDs),
		runtime:            newHVACRuntimeSeriesIndex(series),
	}
	// Each job owns one result slot. Merge in executed-input loop order after
	// all jobs finish, retaining the local-equipment group at the end.
	slots, included := make([]HVACLoopRunResult, count), make([]bool, count)
	build := func(index int) {
		if index == len(loops) {
			slots[index], included[index] = context.buildLocal(local), true
			return
		}
		slots[index], included[index] = context.buildLoop(loops[index])
	}
	if executor == nil {
		for index := range slots {
			build(index)
		}
	} else {
		executor.forEach(count, build)
	}
	results := make([]HVACLoopRunResult, 0, count)
	for index, result := range slots {
		if included[index] {
			results = append(results, result)
		}
	}
	return results
}

// Prepared once and read-only while individual loops build their owned output.
type hvacLoopResultBuildContext struct {
	series             []SimulationSeries
	request            SimulationPurposeRequest
	doc                *idf.Document
	report             idf.HVACReport
	selectedComponents map[string]bool
	runtime            hvacRuntimeSeriesIndex
}

func (context *hvacLoopResultBuildContext) buildLoop(loop idf.HVACLoop) (HVACLoopRunResult, bool) {
	nodes := map[string]bool{}
	components := map[string][]idf.HVACComponent{}
	if len(context.selectedComponents) == 0 {
		for _, node := range purposeHVACLoopNodes(loop) {
			nodes[normalizePurposeToken(node)] = true
		}
	}
	for _, component := range hvacLoopScopedMeasurementComponents(loop, context.report, context.selectedComponents) {
		components[normalizePurposeToken(component.ObjectName)] = append(components[normalizePurposeToken(component.ObjectName)], component)
		for _, node := range purposeHVACComponentNodes(component) {
			nodes[normalizePurposeToken(node)] = true
		}
	}
	if len(context.selectedComponents) > 0 && len(components) == 0 {
		return HVACLoopRunResult{}, false
	}
	matched := []SimulationSeries{}
	for _, item := range context.series {
		key, variable := hvacSeriesIdentity(item)
		if purposeSeriesMatchesVariables(item.Column, hvacLoopCheckNodeVariables()) && nodes[normalizePurposeToken(key)] {
			if hvacWaterLoop(loop.Type) && hvacHumidityVariable(variable) {
				continue
			}
			matched = append(matched, item)
			continue
		}
		for _, component := range components[normalizePurposeToken(key)] {
			if hvacComponentReportsVariable(component, variable, context.doc) {
				matched = append(matched, item)
				break
			}
		}
	}
	built := buildHVACLoopRunResults(matched, context.request)
	result := HVACLoopRunResult{}
	if len(built) > 0 {
		result = built[0]
	}
	result.Name, result.LoopType, result.Topology = loop.Name, loop.Type, &loop
	result.Components = hvacTypedComponentSummaries(matched, components, context.doc)
	// Derivations depend on the individual loop type, never the wildcard run.
	result.DerivedMetrics = buildHVACLoopDerivedMetrics(result.NodeSummaries, result.LoopType)
	result.Status, result.StatusMessage = classifyHVACLoopStatus(result.NodeSummaries, result.DerivedMetrics, result.Alerts)
	attachHVACRuntimeControlsWithIndex(&result, hvacRuntimeProfiles(context.report, loop, context.request.Scope), loop, context.doc, context.runtime)
	return result, true
}

func (context *hvacLoopResultBuildContext) buildLocal(profiles []idf.HVACOperatingProfile) HVACLoopRunResult {
	result := HVACLoopRunResult{Name: "Zone / local equipment", LoopType: "ZoneHVAC", Status: "info", StatusMessage: "Local equipment controls from the executed input; no physical loop is implied."}
	if legacy := buildHVACLoopRunResults(hvacRuntimeLocalHourlySeries(profiles, context.doc, context.series), context.request); len(legacy) > 0 {
		result.Series, result.Components, result.NodeSummaries = legacy[0].Series, legacy[0].Components, legacy[0].NodeSummaries
	}
	attachHVACRuntimeControlsWithIndex(&result, profiles, idf.HVACLoop{}, context.doc, context.runtime)
	return result
}

// Follow only the analyzer's typed, resolved HVAC component references. This
// includes measured fans/coils inside wrapper equipment without treating names
// or unrelated zone equipment as evidence of loop ownership.
func hvacLoopScopedMeasurementComponents(loop idf.HVACLoop, report idf.HVACReport, selected map[string]bool) []idf.HVACComponent {
	identity := func(objectType, name string) string {
		return normalizePurposeToken(objectType) + "\x00" + normalizePurposeToken(name)
	}
	components := map[string]idf.HVACComponent{}
	circuitNodes := map[string]map[string]bool{}
	queue := []string{}
	add := func(component idf.HVACComponent) {
		key := identity(component.ObjectType, component.ObjectName)
		if _, exists := components[key]; exists || strings.TrimSpace(component.ObjectName) == "" {
			return
		}
		components[key] = component
		queue = append(queue, key)
	}
	for _, component := range purposeHVACLoopComponents(loop) {
		key := identity(component.ObjectType, component.ObjectName)
		if circuitNodes[key] == nil {
			circuitNodes[key] = map[string]bool{}
		}
		for _, name := range []string{component.InletNode, component.OutletNode} {
			if name != "" {
				circuitNodes[key][normalizePurposeToken(name)] = true
			}
		}
		add(component)
	}
	for _, path := range []*idf.AirLoopDemandPath{loop.DemandGraph.SupplyPath, loop.DemandGraph.ReturnPath} {
		if path == nil {
			continue
		}
		for _, component := range path.Components {
			add(idf.HVACComponent{ObjectType: component.ObjectType, ObjectName: component.ObjectName, ObjectIndex: component.ObjectIndex})
		}
	}
	loopNodes := purposeNameSet(purposeHVACLoopNodes(loop))
	if loop.Type == "AirLoopHVAC" {
		for _, zone := range report.ZoneRelations {
			for _, terminal := range zone.TerminalUnits {
				if loopNodes[normalizePurposeToken(terminal.InletNode)] {
					add(terminal)
				}
			}
		}
	}
	references := map[string][]idf.HVACComponentReference{}
	for _, reference := range report.ComponentReferences {
		if reference.TargetExists {
			key := identity(reference.FromObjectType, reference.FromObjectName)
			references[key] = append(references[key], reference)
		}
	}
	for index := 0; index < len(queue); index++ {
		for _, reference := range references[queue[index]] {
			add(idf.HVACComponent{ObjectType: reference.TargetObjectType, ObjectName: reference.TargetObjectName, ObjectIndex: reference.TargetObjectIndex, Exists: true})
		}
	}
	allowed := map[string]bool{}
	if len(selected) > 0 {
		selectedQueue := []string{}
		for key, component := range components {
			if purposeHVACComponentSelected(component, selected) {
				allowed[key] = true
				selectedQueue = append(selectedQueue, key)
			}
		}
		for index := 0; index < len(selectedQueue); index++ {
			for _, reference := range references[selectedQueue[index]] {
				key := identity(reference.TargetObjectType, reference.TargetObjectName)
				if !allowed[key] {
					allowed[key] = true
					selectedQueue = append(selectedQueue, key)
				}
			}
		}
	}
	usages := map[string][]idf.HVACNodeUsage{}
	for _, usage := range report.NodeUsages {
		key := identity(usage.ObjectType, usage.ObjectName)
		usages[key] = append(usages[key], usage)
	}
	out := []idf.HVACComponent{}
	for key, component := range components {
		if len(selected) > 0 && !allowed[key] {
			continue
		}
		component.NodeUsages = append(append([]idf.HVACNodeUsage(nil), component.NodeUsages...), usages[key]...)
		component = hvacComponentOnLoopCircuit(component, loop.Type, circuitNodes[key], loopNodes)
		component.SourceOwnerName, component.SourceOwnerType = "", ""
		parents := map[string]idf.HVACComponent{}
		for parentKey, parent := range components {
			if parentKey == key {
				continue
			}
			for _, reference := range references[parentKey] {
				if identity(reference.TargetObjectType, reference.TargetObjectName) == key {
					parents[parentKey] = parent
				}
			}
		}
		if len(parents) == 1 {
			for _, parent := range parents {
				component.SourceOwnerType, component.SourceOwnerName = parent.ObjectType, parent.ObjectName
			}
		}
		out = append(out, component)
	}
	sort.Slice(out, func(i, j int) bool {
		return identity(out[i].ObjectType, out[i].ObjectName) < identity(out[j].ObjectType, out[j].ObjectName)
	})
	return out
}

func hvacWaterLoop(loopType string) bool {
	return strings.EqualFold(loopType, "PlantLoop") || strings.EqualFold(loopType, "CondenserLoop")
}

func hvacHumidityVariable(variable string) bool {
	return strings.Contains(strings.ToLower(variable), "humidity")
}

// Scope node observations to this occurrence of a shared device. In particular,
// WaterInletNode alone cannot choose a chiller's condenser vs. evaporator circuit.
func hvacComponentOnLoopCircuit(component idf.HVACComponent, loopType string, circuit, loopNodes map[string]bool) idf.HVACComponent {
	allowed := func(name, role string) bool {
		if hvacWaterLoop(loopType) {
			if len(circuit) > 0 {
				return circuit[normalizePurposeToken(name)]
			}
			return loopNodes[normalizePurposeToken(name)]
		}
		return !strings.HasPrefix(role, "water_") && !strings.HasPrefix(role, "plant_") && !strings.HasPrefix(role, "condenser_")
	}
	usages := []idf.HVACNodeUsage{}
	for _, usage := range component.NodeUsages {
		if allowed(usage.NodeName, usage.Role) {
			usages = append(usages, usage)
		}
	}
	component.NodeUsages = usages
	for _, port := range []struct {
		name *string
		role string
	}{{&component.InletNode, "inlet"}, {&component.OutletNode, "outlet"}, {&component.WaterInletNode, "water_inlet"}, {&component.WaterOutletNode, "water_outlet"}} {
		if !allowed(*port.name, port.role) {
			*port.name = ""
		}
	}
	return component
}

func hvacTypedComponentSummaries(series []SimulationSeries, components map[string][]idf.HVACComponent, doc *idf.Document) []HVACComponentRunSummary {
	out := []HVACComponentRunSummary{}
	for key, owners := range components {
		for _, owner := range owners {
			matched := []SimulationSeries{}
			for _, item := range series {
				itemKey, variable := hvacSeriesIdentity(item)
				if normalizePurposeToken(itemKey) != key || !hvacSeriesIsHourly(item) || !hvacComponentReportsVariable(owner, variable, doc) {
					continue
				}
				matchingOwners := 0
				for _, candidate := range owners {
					if hvacComponentReportsVariable(candidate, variable, doc) {
						matchingOwners++
					}
				}
				if matchingOwners == 1 {
					matched = append(matched, item)
				}
			}
			built := buildHVACComponentRunSummaries(matched)
			if len(built) == 0 {
				continue
			}
			summary := built[0]
			summary.ComponentType = owner.ObjectType
			summary.ParentComponentName, summary.ParentComponentType = owner.SourceOwnerName, owner.SourceOwnerType
			seenPorts := map[string]bool{}
			addPort := func(node, role string) {
				node = strings.TrimSpace(node)
				key := normalizePurposeToken(node) + "\x00" + role
				if node == "" || seenPorts[key] {
					return
				}
				seenPorts[key] = true
				summary.NodePorts = append(summary.NodePorts, HVACComponentNodePort{NodeName: node, Role: role})
				if strings.Contains(role, "inlet") {
					summary.InletNodes = appendUniquePurposeString(summary.InletNodes, node)
				}
				if strings.Contains(role, "outlet") {
					summary.OutletNodes = appendUniquePurposeString(summary.OutletNodes, node)
				}
			}
			for _, usage := range owner.NodeUsages {
				addPort(usage.NodeName, usage.Role)
			}
			addPort(owner.InletNode, "inlet")
			addPort(owner.OutletNode, "outlet")
			addPort(owner.WaterInletNode, "water_inlet")
			addPort(owner.WaterOutletNode, "water_outlet")
			out = append(out, summary)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return normalizePurposeToken(out[i].ComponentName+"\x00"+out[i].ComponentType) < normalizePurposeToken(out[j].ComponentName+"\x00"+out[j].ComponentType)
	})
	return out
}

func hvacResultLoopInScope(loop idf.HVACLoop, scope SimulationPurposeScope) bool {
	selectedNames := map[string]map[string]bool{
		"AirLoopHVAC": purposeNameSet(scope.AirLoopNames), "PlantLoop": purposeNameSet(scope.PlantLoopNames),
		"CondenserLoop": purposeNameSet(scope.CondenserLoopNames),
	}
	if len(scope.AirLoopNames)+len(scope.PlantLoopNames)+len(scope.CondenserLoopNames) > 0 {
		return purposeHVACLoopSelected(loop, selectedNames)
	}
	return !strings.EqualFold(scope.LoopMode, "selected") || len(scope.ComponentIDs) > 0
}

func hvacComponentTypeReportsVariable(objectType, variable string) bool {
	if strings.EqualFold(strings.TrimSpace(variable), "Fan Runtime Fraction") {
		return strings.EqualFold(strings.TrimSpace(objectType), "Fan:OnOff")
	}
	for _, output := range hvacLoopCheckComponentOutputsForType(objectType) {
		if strings.EqualFold(strings.TrimSpace(variable), output.VariableName) {
			return true
		}
	}
	return false
}

func hvacSeriesIdentity(series SimulationSeries) (string, string) {
	key, variable := splitPurposeSeriesColumn(series.Column)
	if series.KeyValue != "" {
		key = series.KeyValue
	}
	if series.Name != "" {
		variable = series.Name
	}
	return key, variable
}

func hvacSeriesIsHourly(series SimulationSeries) bool {
	frequency := strings.TrimSpace(series.ReportingFrequency)
	if frequency == "" {
		if index := strings.LastIndex(series.Column, "("); index >= 0 && strings.HasSuffix(series.Column, ")") {
			frequency = strings.TrimSuffix(series.Column[index+1:], ")")
		}
	}
	return frequency == "" || strings.EqualFold(frequency, "Hourly")
}
