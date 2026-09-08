package idf

import "strings"

type hvacAirLoopConditioning struct {
	ServiceKind string
	Components  []ComponentRef
	TraceIDs    []string
}

// Native air-side DX and fuel/electric coils do not have the PlantLoop or VRF
// source path used by buildServiceChainsFromRuleGraph. Recognize them only on
// an unambiguous, continuous supply branch, not from an equipment/terminal name.
// Water/plant conditioning remains with the existing plant-backed path builder.
func buildHVACAirLoopConditioning(ctx *hvacContext, loops []HVACLoop, graph HVACRuleGraph) map[string][]hvacAirLoopConditioning {
	out := map[string][]hvacAirLoopConditioning{}
	counts := map[string]int{}
	for _, obj := range ctx.doc.Objects {
		counts[hvacObjectKey(obj.Type, objectName(obj))]++
	}
	for _, loop := range loops {
		if !strings.EqualFold(loop.Type, "AirLoopHVAC") || counts[hvacObjectKey(loop.Type, loop.Name)] != 1 {
			continue
		}
		side := loop.SupplySide
		// A single connected branch is explicit evidence. Do not guess a route
		// through incomplete/parallel branches or silently broaden recipients.
		if len(side.Branches) != 1 || side.ConnectorListName != "" || len(side.Connectors) != 0 || len(side.MissingBranchNames) != 0 ||
			side.InletNode == "" || side.OutletNode == "" ||
			counts[hvacObjectKey("BranchList", side.BranchListName)] != 1 {
			continue
		}
		branch := side.Branches[0]
		if len(branch.Components) == 0 || counts[hvacObjectKey("Branch", branch.Name)] != 1 {
			continue
		}
		branchObject := ctx.objectsByTypeName[hvacObjectKey("Branch", branch.Name)]
		connected := true
		lastNode := side.InletNode
		seenComponents := map[string]bool{}
		for _, component := range branch.Components {
			if counts[hvacComponentKey(component)] != 1 || component.InletNode == "" || component.OutletNode == "" ||
				!strings.EqualFold(lastNode, component.InletNode) || seenComponents[hvacComponentKey(component)] ||
				!strings.EqualFold(hvacFieldValue(branchObject, component.InletFieldIndex), component.InletNode) ||
				!strings.EqualFold(hvacFieldValue(branchObject, component.OutletFieldIndex), component.OutletNode) {
				connected = false
				break
			}
			seenComponents[hvacComponentKey(component)] = true
			lastNode = component.OutletNode
		}
		if !connected || !strings.EqualFold(lastNode, side.OutletNode) {
			continue
		}
		byService := map[string]*hvacAirLoopConditioning{}
		for _, component := range branch.Components {
			service, refs, trace := hvacNativeAirConditioning(ctx, graph, counts, component)
			if service == "" {
				continue
			}
			entry := byService[service]
			if entry == nil {
				entry = &hvacAirLoopConditioning{ServiceKind: service}
				byService[service] = entry
			}
			for _, ref := range refs {
				entry.Components = appendUniqueComponentRef(entry.Components, ref)
			}
			entry.TraceIDs = appendUniqueStrings(entry.TraceIDs, trace...)
		}
		for _, service := range []string{"cooling", "heating"} {
			if entry := byService[service]; entry != nil {
				out[normalizeName(loop.Name)] = append(out[normalizeName(loop.Name)], *entry)
			}
		}
	}
	return out
}

func hvacNativeAirConditioning(ctx *hvacContext, graph HVACRuleGraph, counts map[string]int, component HVACComponent) (string, []ComponentRef, []string) {
	lower := normalizeFieldCatalogKey(component.ObjectType)
	obj := ctx.objectsByTypeName[hvacComponentKey(component)]
	if lower == "coilsystem:cooling:dx" {
		if !hvacAirConditioningNodesMatch(obj, component, "DX Cooling Coil System Inlet Node Name", "DX Cooling Coil System Outlet Node Name") {
			return "", nil, nil
		}
		var children []HVACComponentReference
		for _, ref := range ctx.componentReferencesByFromKey[hvacComponentKey(component)] {
			if strings.HasPrefix(normalizeFieldCatalogKey(ref.TargetObjectType), "coil:") {
				children = append(children, ref)
			}
		}
		if len(children) != 1 || !children[0].TargetExists ||
			!strings.HasPrefix(normalizeFieldCatalogKey(children[0].TargetObjectType), "coil:cooling:dx:") ||
			counts[hvacObjectKey(children[0].TargetObjectType, children[0].TargetObjectName)] != 1 {
			return "", nil, nil
		}
		child := newHVACComponent(ctx, children[0].TargetObjectType, children[0].TargetObjectName)
		childObj := ctx.objectsByTypeName[hvacComponentKey(child)]
		if !hvacAirConditioningNodesMatch(childObj, component, "Air Inlet Node Name", "Air Outlet Node Name") {
			return "", nil, nil
		}
		for _, edge := range graph.Edges {
			if edge.RuleID == hvacRuleComponentReferencesComponent && edge.FromID == hvacRuleComponentSourceNodeID(component) && edge.ToID == hvacRuleComponentSourceNodeID(child) {
				return "cooling", []ComponentRef{componentRefFromHVACComponent(component), componentRefFromHVACComponent(child)},
					[]string{hvacRuleBranchComponentOccurrence, hvacRuleComponentSourceOccurrence, edge.RuleID}
			}
		}
		return "", nil, nil
	}
	service := ""
	switch {
	case strings.HasPrefix(lower, "coil:cooling:dx:"):
		service = "cooling"
	case strings.HasPrefix(lower, "coil:heating:dx:"), lower == "coil:heating:fuel", lower == "coil:heating:gas", lower == "coil:heating:electric":
		service = "heating"
	}
	if service == "" || !hvacAirConditioningNodesMatch(obj, component, "Air Inlet Node Name", "Air Outlet Node Name") {
		return "", nil, nil
	}
	return service, []ComponentRef{componentRefFromHVACComponent(component)}, []string{hvacRuleBranchComponentOccurrence, hvacRuleComponentSourceOccurrence}
}

func hvacAirConditioningNodesMatch(obj Object, component HVACComponent, inletField, outletField string) bool {
	inlet := fieldValueByCatalogName(obj, inletField)
	outlet := fieldValueByCatalogName(obj, outletField)
	return inlet != "" && outlet != "" && strings.EqualFold(inlet, component.InletNode) && strings.EqualFold(outlet, component.OutletNode)
}

func hvacAirConditioningDeliveryTrace(ctx *hvacContext, loops []HVACLoop, graph HVACRuleGraph, relation HVACZoneChain, terminal HVACComponent, airLoopName string) ([]string, bool) {
	if !hvacAirConditioningUniqueObject(ctx, terminal.ObjectType, terminal.ObjectName) ||
		!hvacAirConditioningUniqueObject(ctx, "Zone", relation.ZoneName) ||
		(terminal.ResolvedFromADU && !hvacAirConditioningUniqueObject(ctx, "ZoneHVAC:AirDistributionUnit", terminal.DistributionUnitName)) {
		return nil, false
	}
	// Existing relation graph edges can represent the Zone-wide union of its
	// AirLoops. They are not sufficient to assign each individual terminal.
	// Bind this terminal's actual inlet to exactly one physical demand graph.
	inlet, matchedLoops, selectedLoop := normalizeName(terminal.InletNode), 0, false
	for _, loop := range loops {
		if !strings.EqualFold(loop.Type, "AirLoopHVAC") || inlet == "" {
			continue
		}
		nodes := airLoopDemandGraphNodeSet(airLoopDemandGraphForLoop(ctx, loop))
		if nodes[inlet] {
			matchedLoops++
			selectedLoop = selectedLoop || strings.EqualFold(loop.Name, airLoopName)
		}
	}
	if matchedLoops != 1 || !selectedLoop {
		return nil, false
	}
	loopID := hvacRuleLoopNodeIDForName(graph, "AirLoopHVAC", airLoopName)
	terminalID := hvacRuleComponentSourceNodeID(terminal)
	subjectID := hvacRuleSubjectNodeIDForRelation(graph, relation)
	upstream, upstreamOK := hvacRuleGraphPath(graph, loopID, terminalID)
	downstream, downstreamOK := hvacRuleGraphPath(graph, terminalID, subjectID)
	if !upstreamOK || !downstreamOK || loopID == "" || subjectID == "" {
		return nil, false
	}
	return appendUniqueStrings(hvacRulePathRuleIDs(upstream), hvacRulePathRuleIDs(downstream)...), true
}

func hvacAirConditioningUniqueObject(ctx *hvacContext, objectType, name string) bool {
	count := 0
	for _, obj := range ctx.objectsByName[normalizeName(name)] {
		if strings.EqualFold(obj.Type, objectType) {
			count++
		}
	}
	return count == 1
}
