package idf

import (
	"fmt"
	"sort"
	"strings"
)

func (builder *thermalTopologyBuilder) addAirCouplings() {
	builder.addExplicitAirExchangeCouplings()
	builder.addAirBoundaryCouplings()
	builder.addAirflowNetworkCouplings()
	sort.SliceStable(builder.report.AirCouplings, func(i, j int) bool {
		return builder.report.AirCouplings[i].ID < builder.report.AirCouplings[j].ID
	})
}

func (builder *thermalTopologyBuilder) addExplicitAirExchangeCouplings() {
	for _, object := range builder.doc.Objects {
		lowerType := strings.ToLower(strings.TrimSpace(object.Type))
		switch lowerType {
		case "zonemixing", "zonecrossmixing":
			receivingName := geometryStringField(object, "Zone or Space Name")
			sourceName := geometryStringField(object, "Source Zone or Space Name")
			fromNodeID, fromFound := builder.thermalOwnedNodeID(sourceName)
			toNodeID, toFound := builder.thermalOwnedNodeID(receivingName)
			direction := "directed"
			kind := "zone_mixing"
			if lowerType == "zonecrossmixing" {
				direction = "bidirectional"
				kind = "zone_cross_mixing"
			}
			coupling := builder.newAirCoupling(object, fromNodeID, toNodeID, direction, kind)
			zone := builder.thermalZoneForOwnedName(receivingName)
			coupling.DesignFlowRate = thermalDesignFlowRate(object, zone, builder.peopleCountByZone[normalizeName(zone.Name)])
			if coupling.DesignFlowRate > 0 {
				coupling.Unit = "m3/s"
			}
			coupling.ScheduleName = geometryStringField(object, "Schedule Name")
			builder.appendAirCoupling(coupling, fromFound && toFound, fmt.Sprintf("Air exchange object %q cannot resolve source %q or target %q.", objectName(object), sourceName, receivingName))
		case "zonerefrigerationdoormixing":
			zone1Name := geometryStringField(object, "Zone 1 Name")
			zone2Name := geometryStringField(object, "Zone 2 Name")
			zone1ID, zone1Found := builder.thermalOwnedNodeID(zone1Name)
			zone2ID, zone2Found := builder.thermalOwnedNodeID(zone2Name)
			coupling := builder.newAirCoupling(object, zone1ID, zone2ID, "bidirectional", "refrigeration_door_mixing")
			coupling.ScheduleName = geometryStringField(object, "Schedule Name")
			builder.appendAirCoupling(coupling, zone1Found && zone2Found, fmt.Sprintf("Refrigeration door mixing %q cannot resolve both zones.", objectName(object)))
		case "zoneventilation:designflowrate", "zoneventilation:windandstackopenarea":
			ownedName := firstNonEmpty(
				geometryStringField(object, "Zone or ZoneList or Space or SpaceList Name"),
				geometryStringField(object, "Zone or Space Name"),
			)
			toNodeID, targetFound := builder.thermalOwnedNodeID(ownedName)
			anchors := []SemanticSourceAnchor{builder.sourceAnchor(object, nil, "")}
			fromNodeID := builder.addEnvironmentNode("outdoors", "Outdoors", anchors)
			coupling := builder.newAirCoupling(object, fromNodeID, toNodeID, "directed", "outdoor_ventilation")
			zone := builder.thermalZoneForOwnedName(ownedName)
			coupling.DesignFlowRate = thermalDesignFlowRate(object, zone, builder.peopleCountByZone[normalizeName(zone.Name)])
			if coupling.DesignFlowRate > 0 {
				coupling.Unit = "m3/s"
			}
			coupling.ScheduleName = firstNonEmpty(geometryStringField(object, "Schedule Name"), geometryStringField(object, "Opening Area Fraction Schedule Name"))
			builder.appendAirCoupling(coupling, targetFound, fmt.Sprintf("Ventilation object %q cannot resolve target %q.", objectName(object), ownedName))
		}
	}
}

func (builder *thermalTopologyBuilder) addAirBoundaryCouplings() {
	visited := map[string]bool{}
	for _, boundary := range builder.report.Boundaries {
		construction := builder.constructionByName[normalizeName(boundary.ConstructionName)]
		if construction.Kind != "air_boundary" || construction.ObjectIndex < 0 {
			continue
		}
		constructionObject, ok := builder.objectsByIndex[construction.ObjectIndex]
		if !ok || !strings.EqualFold(geometryStringField(constructionObject, "Air Exchange Method"), "SimpleMixing") {
			continue
		}
		key := firstNonEmpty(boundary.PairID, boundary.ID)
		if visited[key] {
			continue
		}
		visited[key] = true
		fromNodeID := firstNonEmpty(boundary.OwnerSpaceID, boundary.OwnerZoneID)
		toNodeID := boundary.TargetID
		coupling := builder.newAirCoupling(constructionObject, fromNodeID, toNodeID, "bidirectional", "construction_air_boundary")
		coupling.ID = "thermal-air-coupling:air-boundary:" + semanticStableHash(key, 20)
		coupling.EntityID = coupling.ID
		coupling.SurfaceID = boundary.SurfaceID
		coupling.ScheduleName = geometryStringField(constructionObject, "Simple Mixing Schedule Name")
		if airChanges, ok := geometryNumericField(constructionObject, "Simple Mixing Air Changes per Hour"); ok && airChanges > 0 {
			zone := builder.thermalZoneForNodeID(boundary.OwnerZoneID)
			if zone.Volume > 0 {
				coupling.DesignFlowRate = roundedNumber(airChanges*zone.Volume/3600, 6)
				coupling.Unit = "m3/s"
			}
		}
		coupling.SourceAnchors = appendUniqueThermalAnchors(coupling.SourceAnchors, boundary.SourceAnchors...)
		builder.appendAirCoupling(coupling, fromNodeID != "" && toNodeID != "" && boundary.RelationKind != "invalid", fmt.Sprintf("Air boundary %q cannot resolve both connected thermal nodes.", boundary.SurfaceName))
	}
}

func (builder *thermalTopologyBuilder) addAirflowNetworkCouplings() {
	afnZoneAnchors := map[string][]SemanticSourceAnchor{}
	for _, object := range builder.doc.Objects {
		if strings.EqualFold(object.Type, "AirflowNetwork:MultiZone:Zone") {
			zoneName := geometryStringField(object, "Zone Name")
			afnZoneAnchors[normalizeName(zoneName)] = append(afnZoneAnchors[normalizeName(zoneName)], builder.sourceAnchor(object, nil, ""))
		}
	}
	for _, object := range builder.doc.Objects {
		if !strings.EqualFold(object.Type, "AirflowNetwork:MultiZone:Surface") {
			continue
		}
		surfaceName := geometryStringField(object, "Surface Name")
		componentName := geometryStringField(object, "Leakage Component Name")
		boundary, surfaceID, found := builder.airflowNetworkBoundary(surfaceName)
		coupling := builder.newAirCoupling(object, "", "", "bidirectional", "airflow_network")
		coupling.ComponentName = componentName
		coupling.SurfaceID = surfaceID
		if found {
			coupling.FromNodeID = firstNonEmpty(boundary.OwnerSpaceID, boundary.OwnerZoneID)
			coupling.ToNodeID = boundary.TargetID
			coupling.SourceAnchors = appendUniqueThermalAnchors(coupling.SourceAnchors, boundary.SourceAnchors...)
			if zone := builder.thermalZoneForNodeID(boundary.OwnerZoneID); zone.Name != "" {
				coupling.SourceAnchors = appendUniqueThermalAnchors(coupling.SourceAnchors, afnZoneAnchors[normalizeName(zone.Name)]...)
			}
		}
		for _, candidate := range builder.documentIndex.ObjectsNamed(componentName) {
			if strings.HasPrefix(strings.ToLower(candidate.Type), "airflownetwork:multizone:") {
				coupling.SourceAnchors = appendUniqueThermalAnchors(coupling.SourceAnchors, builder.sourceAnchor(candidate, nil, ""))
				if strings.Contains(strings.ToLower(candidate.Type), "zoneexhaustfan") || strings.Contains(strings.ToLower(candidate.Type), "specifiedflowrate") {
					coupling.Direction = "directed"
				}
				break
			}
		}
		valid := found && coupling.FromNodeID != "" && coupling.ToNodeID != "" && boundary.RelationKind != "invalid"
		builder.appendAirCoupling(coupling, valid, fmt.Sprintf("AirflowNetwork surface %q cannot resolve heat-transfer surface or opening %q.", objectLabel(object), surfaceName))
	}
}

func (builder *thermalTopologyBuilder) airflowNetworkBoundary(surfaceName string) (ThermalBoundaryRecord, string, bool) {
	if boundaryIndexes := builder.boundaryIndexesByName[normalizeName(surfaceName)]; len(boundaryIndexes) == 1 {
		boundary := builder.report.Boundaries[boundaryIndexes[0]]
		return boundary, boundary.SurfaceID, true
	}
	if openingIndexes := builder.openingIndexesByName[normalizeName(surfaceName)]; len(openingIndexes) == 1 {
		opening := builder.report.Openings[openingIndexes[0]]
		if boundaryIndex, ok := builder.boundaryIndexBySurfaceID[opening.BaseSurfaceID]; ok {
			return builder.report.Boundaries[boundaryIndex], opening.WindowID, true
		}
	}
	return ThermalBoundaryRecord{}, "", false
}

func (builder *thermalTopologyBuilder) newAirCoupling(object Object, fromNodeID string, toNodeID string, direction string, kind string) ThermalAirCoupling {
	objectID := builder.registry.byObjectIndex[object.Index]
	entityID := "thermal-air-coupling:" + semanticStableHash(strings.Join([]string{objectID, kind}, "\x00"), 20)
	anchors := []SemanticSourceAnchor{builder.sourceAnchor(object, nil, "")}
	for _, fieldName := range []string{"Zone or Space Name", "Source Zone or Space Name", "Zone 1 Name", "Zone 2 Name", "Surface Name", "Leakage Component Name"} {
		if fieldIndex, ok := thermalFieldIndex(object, fieldName); ok {
			anchors = appendUniqueSemanticSourceAnchor(anchors, builder.sourceAnchor(object, intPtr(fieldIndex), fieldName))
		}
	}
	return ThermalAirCoupling{
		ID:            entityID,
		EntityID:      entityID,
		ObjectType:    object.Type,
		ObjectName:    objectName(object),
		ObjectIndex:   object.Index,
		FromNodeID:    fromNodeID,
		ToNodeID:      toNodeID,
		Direction:     direction,
		CouplingKind:  kind,
		SourceAnchors: anchors,
	}
}

func (builder *thermalTopologyBuilder) appendAirCoupling(coupling ThermalAirCoupling, valid bool, issueMessage string) {
	if !valid {
		if coupling.FromNodeID == "" {
			coupling.FromNodeID = builder.addUnresolvedNode("Unresolved air coupling source", coupling.SourceAnchors)
		}
		if coupling.ToNodeID == "" {
			coupling.ToNodeID = builder.addUnresolvedNode("Unresolved air coupling target", coupling.SourceAnchors)
		}
		builder.addAirCouplingIssue(&coupling, "air_coupling_target_missing", issueMessage)
	}
	builder.report.AirCouplings = append(builder.report.AirCouplings, coupling)
}

func (builder *thermalTopologyBuilder) addAirCouplingIssue(coupling *ThermalAirCoupling, code string, message string) {
	issueID := "topology-issue:" + semanticStableHash(strings.Join([]string{code, coupling.EntityID, message}, "\x00"), 20)
	coupling.DiagnosticIDs = appendUniqueString(coupling.DiagnosticIDs, issueID)
	builder.report.IssueLinks = append(builder.report.IssueLinks, ThermalTopologyIssueLink{
		ID:            issueID,
		Code:          code,
		Severity:      "warning",
		Message:       message,
		EntityID:      coupling.EntityID,
		AirCouplingID: coupling.ID,
		SourceAnchors: append([]SemanticSourceAnchor(nil), coupling.SourceAnchors...),
	})
}

func (builder *thermalTopologyBuilder) thermalOwnedNodeID(name string) (string, bool) {
	key := normalizeName(name)
	if nodeID := builder.spaceNodeIDByName[key]; nodeID != "" {
		return nodeID, true
	}
	if nodeID := builder.zoneNodeIDByName[key]; nodeID != "" {
		return nodeID, true
	}
	return "", false
}

func (builder *thermalTopologyBuilder) thermalZoneForOwnedName(name string) GeometryZone {
	key := normalizeName(name)
	for _, space := range builder.geometry.Spaces {
		if normalizeName(space.Name) == key {
			key = normalizeName(space.ZoneName)
			break
		}
	}
	for _, zone := range builder.geometry.Zones {
		if normalizeName(zone.Name) == key {
			return zone
		}
	}
	return GeometryZone{}
}

func (builder *thermalTopologyBuilder) thermalZoneForNodeID(nodeID string) GeometryZone {
	for _, zone := range builder.geometry.Zones {
		if builder.zoneNodeIDByName[normalizeName(zone.Name)] == nodeID {
			return zone
		}
	}
	return GeometryZone{}
}

func thermalDesignFlowRate(object Object, zone GeometryZone, peopleCount float64) float64 {
	method := strings.ToLower(strings.TrimSpace(geometryStringField(object, "Design Flow Rate Calculation Method")))
	switch method {
	case "flow/area", "flowperarea":
		if value, ok := geometryNumericField(object, "Flow Rate per Floor Area"); ok && zone.FloorArea > 0 {
			return roundedNumber(value*zone.FloorArea, 6)
		}
	case "airchanges/hour", "airchangesperhour", "ach":
		if value, ok := geometryNumericField(object, "Air Changes per Hour"); ok && zone.Volume > 0 {
			return roundedNumber(value*zone.Volume/3600, 6)
		}
	case "flow/person", "flowperperson":
		if value, ok := geometryNumericField(object, "Flow Rate per Person"); ok && peopleCount > 0 {
			return roundedNumber(value*peopleCount, 6)
		}
	default:
		if value, ok := geometryNumericField(object, "Design Flow Rate"); ok {
			return roundedNumber(value, 6)
		}
	}
	return 0
}

func thermalPeopleCounts(doc Document, geometry GeometryReport) map[string]float64 {
	context := newProfileContextWithGeometry(doc, geometry)
	return context.peopleCount
}

func appendUniqueThermalAnchors(values []SemanticSourceAnchor, candidates ...SemanticSourceAnchor) []SemanticSourceAnchor {
	for _, candidate := range candidates {
		values = appendUniqueSemanticSourceAnchor(values, candidate)
	}
	return values
}
