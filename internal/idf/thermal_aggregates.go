package idf

import (
	"sort"
	"strings"
)

type thermalConnectionContribution struct {
	fromNodeID           string
	toNodeID             string
	relationKind         string
	boundaryIDs          []string
	openingIDs           []string
	physicalGrossArea    float64
	physicalOpaqueArea   float64
	physicalOpeningArea  float64
	effectiveGrossArea   float64
	effectiveOpaqueArea  float64
	effectiveOpeningArea float64
	opaqueUA             float64
	openingUA            float64
	totalUA              float64
	hasUA                bool
	physicalOpaqueUA     float64
	physicalOpeningUA    float64
	physicalTotalUA      float64
	hasPhysicalUA        bool
	orientations         []string
	diagnosticIDs        []string
	sourceAnchors        []SemanticSourceAnchor
}

func (builder *thermalTopologyBuilder) buildConnectionAggregates() {
	builder.report.Connections = []ThermalConnectionAggregate{}
	openingsByID := map[string]ThermalOpeningRecord{}
	for _, opening := range builder.report.Openings {
		openingsByID[opening.ID] = opening
	}
	connectionIndexByKey := map[string]int{}
	uaCompleteByKey := map[string]bool{}
	physicalUACompleteByKey := map[string]bool{}
	visitedPairs := map[string]bool{}
	for _, boundary := range builder.report.Boundaries {
		contribution := builder.connectionContributionForBoundary(boundary, openingsByID, visitedPairs)
		if contribution.fromNodeID == "" || contribution.toNodeID == "" {
			continue
		}
		key := thermalConnectionKey(contribution.fromNodeID, contribution.toNodeID, contribution.relationKind)
		connectionIndex, exists := connectionIndexByKey[key]
		if !exists {
			fromNodeID, toNodeID := thermalSortedNodeIDs(contribution.fromNodeID, contribution.toNodeID)
			builder.report.Connections = append(builder.report.Connections, ThermalConnectionAggregate{
				ID:           "thermal-connection:" + semanticStableHash(key, 20),
				FromNodeID:   fromNodeID,
				ToNodeID:     toNodeID,
				RelationKind: contribution.relationKind,
				QAOnly:       contribution.relationKind == "invalid",
				BoundaryIDs:  []string{},
			})
			connectionIndex = len(builder.report.Connections) - 1
			connectionIndexByKey[key] = connectionIndex
			uaCompleteByKey[key] = true
			physicalUACompleteByKey[key] = true
		}
		aggregate := &builder.report.Connections[connectionIndex]
		aggregate.BoundaryIDs = appendUniqueStrings(aggregate.BoundaryIDs, contribution.boundaryIDs...)
		aggregate.OpeningIDs = appendUniqueStrings(aggregate.OpeningIDs, contribution.openingIDs...)
		aggregate.SurfaceCount++
		aggregate.PhysicalGrossArea += contribution.physicalGrossArea
		aggregate.PhysicalOpaqueArea += contribution.physicalOpaqueArea
		aggregate.PhysicalOpeningArea += contribution.physicalOpeningArea
		aggregate.EffectiveGrossArea += contribution.effectiveGrossArea
		aggregate.EffectiveOpaqueArea += contribution.effectiveOpaqueArea
		aggregate.EffectiveOpeningArea += contribution.effectiveOpeningArea
		aggregate.OpaqueUA += contribution.opaqueUA
		aggregate.OpeningUA += contribution.openingUA
		aggregate.TotalUA += contribution.totalUA
		aggregate.PhysicalOpaqueUA += contribution.physicalOpaqueUA
		aggregate.PhysicalOpeningUA += contribution.physicalOpeningUA
		aggregate.PhysicalTotalUA += contribution.physicalTotalUA
		uaCompleteByKey[key] = uaCompleteByKey[key] && contribution.hasUA
		physicalUACompleteByKey[key] = physicalUACompleteByKey[key] && contribution.hasPhysicalUA
		aggregate.Orientations = appendUniqueStrings(aggregate.Orientations, contribution.orientations...)
		aggregate.DiagnosticIDs = appendUniqueStrings(aggregate.DiagnosticIDs, contribution.diagnosticIDs...)
		aggregate.SourceAnchors = appendUniqueThermalAnchors(aggregate.SourceAnchors, contribution.sourceAnchors...)
	}

	for _, coupling := range builder.report.AirCouplings {
		fromNodeID := builder.compactThermalNodeID(coupling.FromNodeID)
		toNodeID := builder.compactThermalNodeID(coupling.ToNodeID)
		if fromNodeID == "" || toNodeID == "" {
			continue
		}
		key := thermalConnectionKey(fromNodeID, toNodeID, "air_coupling")
		connectionIndex, exists := connectionIndexByKey[key]
		if !exists {
			fromNodeID, toNodeID = thermalSortedNodeIDs(fromNodeID, toNodeID)
			builder.report.Connections = append(builder.report.Connections, ThermalConnectionAggregate{
				ID:           "thermal-connection:" + semanticStableHash(key, 20),
				FromNodeID:   fromNodeID,
				ToNodeID:     toNodeID,
				RelationKind: "air_coupling",
				BoundaryIDs:  []string{},
			})
			connectionIndex = len(builder.report.Connections) - 1
			connectionIndexByKey[key] = connectionIndex
		}
		aggregate := &builder.report.Connections[connectionIndex]
		aggregate.AirCouplingIDs = appendUniqueString(aggregate.AirCouplingIDs, coupling.ID)
		aggregate.DiagnosticIDs = appendUniqueStrings(aggregate.DiagnosticIDs, coupling.DiagnosticIDs...)
		aggregate.SourceAnchors = appendUniqueThermalAnchors(aggregate.SourceAnchors, coupling.SourceAnchors...)
	}

	for index := range builder.report.Connections {
		aggregate := &builder.report.Connections[index]
		aggregate.OpeningCount = len(aggregate.OpeningIDs)
		aggregate.PhysicalGrossArea = roundedNumber(aggregate.PhysicalGrossArea, 3)
		aggregate.PhysicalOpaqueArea = roundedNumber(aggregate.PhysicalOpaqueArea, 3)
		aggregate.PhysicalOpeningArea = roundedNumber(aggregate.PhysicalOpeningArea, 3)
		aggregate.EffectiveGrossArea = roundedNumber(aggregate.EffectiveGrossArea, 3)
		aggregate.EffectiveOpaqueArea = roundedNumber(aggregate.EffectiveOpaqueArea, 3)
		aggregate.EffectiveOpeningArea = roundedNumber(aggregate.EffectiveOpeningArea, 3)
		key := thermalConnectionKey(aggregate.FromNodeID, aggregate.ToNodeID, aggregate.RelationKind)
		aggregate.HasUA = aggregate.RelationKind != "air_coupling" && uaCompleteByKey[key]
		aggregate.HasPhysicalUA = aggregate.RelationKind != "air_coupling" && physicalUACompleteByKey[key]
		if aggregate.HasUA {
			aggregate.OpaqueUA = roundedNumber(aggregate.OpaqueUA, 4)
			aggregate.OpeningUA = roundedNumber(aggregate.OpeningUA, 4)
			aggregate.TotalUA = roundedNumber(aggregate.TotalUA, 4)
		} else {
			aggregate.OpaqueUA, aggregate.OpeningUA, aggregate.TotalUA = 0, 0, 0
		}
		if aggregate.HasPhysicalUA {
			aggregate.PhysicalOpaqueUA = roundedNumber(aggregate.PhysicalOpaqueUA, 4)
			aggregate.PhysicalOpeningUA = roundedNumber(aggregate.PhysicalOpeningUA, 4)
			aggregate.PhysicalTotalUA = roundedNumber(aggregate.PhysicalTotalUA, 4)
		} else {
			aggregate.PhysicalOpaqueUA, aggregate.PhysicalOpeningUA, aggregate.PhysicalTotalUA = 0, 0, 0
		}
		sort.Strings(aggregate.BoundaryIDs)
		sort.Strings(aggregate.OpeningIDs)
		sort.Strings(aggregate.AirCouplingIDs)
		sort.Strings(aggregate.Orientations)
		sort.Strings(aggregate.DiagnosticIDs)
	}
	sort.SliceStable(builder.report.Connections, func(i, j int) bool { return builder.report.Connections[i].ID < builder.report.Connections[j].ID })
}

func (builder *thermalTopologyBuilder) connectionContributionForBoundary(boundary ThermalBoundaryRecord, openingsByID map[string]ThermalOpeningRecord, visitedPairs map[string]bool) thermalConnectionContribution {
	if boundary.PairID != "" {
		if visitedPairs[boundary.PairID] {
			return thermalConnectionContribution{}
		}
		visitedPairs[boundary.PairID] = true
	}
	contribution := thermalConnectionContribution{
		fromNodeID:           builder.compactThermalNodeID(boundary.OwnerZoneID),
		toNodeID:             builder.compactThermalNodeID(boundary.TargetID),
		relationKind:         boundary.RelationKind,
		boundaryIDs:          []string{boundary.ID},
		openingIDs:           thermalUniqueOpeningIDs(boundary.OpeningIDs, openingsByID),
		physicalGrossArea:    boundary.PhysicalGrossArea,
		physicalOpaqueArea:   boundary.PhysicalOpaqueArea,
		physicalOpeningArea:  boundary.PhysicalOpeningArea,
		effectiveGrossArea:   boundary.EffectiveGrossArea,
		effectiveOpaqueArea:  boundary.EffectiveOpaqueArea,
		effectiveOpeningArea: boundary.EffectiveOpeningArea,
		opaqueUA:             boundary.OpaqueUA,
		openingUA:            boundary.OpeningUA,
		totalUA:              boundary.TotalUA,
		hasUA:                boundary.HasUA,
		orientations:         []string{boundary.Orientation},
		diagnosticIDs:        append([]string(nil), boundary.DiagnosticIDs...),
		sourceAnchors:        append([]SemanticSourceAnchor(nil), boundary.SourceAnchors...),
	}
	contribution.physicalOpaqueUA, contribution.physicalOpeningUA, contribution.physicalTotalUA, contribution.hasPhysicalUA = thermalBoundaryUAForAreaBasis(boundary, openingsByID, "physical")
	if boundary.RelationKind == "invalid" {
		contribution.opaqueUA, contribution.openingUA, contribution.totalUA = 0, 0, 0
		contribution.physicalOpaqueUA, contribution.physicalOpeningUA, contribution.physicalTotalUA = 0, 0, 0
		contribution.hasUA = false
		contribution.hasPhysicalUA = false
	}
	if boundary.RelationKind == "adiabatic_explicit" || boundary.RelationKind == "adiabatic_self_reference" {
		contribution.opaqueUA, contribution.openingUA, contribution.totalUA = 0, 0, 0
		contribution.physicalOpaqueUA, contribution.physicalOpeningUA, contribution.physicalTotalUA = 0, 0, 0
		contribution.hasUA = true
		contribution.hasPhysicalUA = true
	}
	if boundary.PairID == "" {
		return contribution
	}
	counterpartIndex, ok := builder.boundaryIndexBySurfaceID[boundary.CounterpartSurfaceID]
	if !ok {
		contribution.hasUA = false
		contribution.hasPhysicalUA = false
		return contribution
	}
	counterpart := builder.report.Boundaries[counterpartIndex]
	contribution.boundaryIDs = appendUniqueString(contribution.boundaryIDs, counterpart.ID)
	contribution.diagnosticIDs = appendUniqueStrings(contribution.diagnosticIDs, counterpart.DiagnosticIDs...)
	contribution.sourceAnchors = appendUniqueThermalAnchors(contribution.sourceAnchors, counterpart.SourceAnchors...)
	pairUACompatible := boundary.ConstructionStatus != "mismatch" && boundary.ConstructionStatus != "missing_construction" && boundary.HasUA && counterpart.HasUA && relativeDifference(boundary.TotalUA, counterpart.TotalUA) <= 0.01
	contribution.hasUA = contribution.hasUA && pairUACompatible
	physicalOpaqueUA, physicalOpeningUA, physicalTotalUA, counterpartHasPhysicalUA := thermalBoundaryUAForAreaBasis(counterpart, openingsByID, "physical")
	_ = physicalOpaqueUA
	_ = physicalOpeningUA
	contribution.hasPhysicalUA = contribution.hasPhysicalUA && counterpartHasPhysicalUA && relativeDifference(contribution.physicalTotalUA, physicalTotalUA) <= 0.01
	if !thermalPairedOpeningsAreComplete(boundary, counterpart, openingsByID) {
		contribution.hasUA = false
		contribution.hasPhysicalUA = false
	}
	return contribution
}

func thermalUniqueOpeningIDs(openingIDs []string, openingsByID map[string]ThermalOpeningRecord) []string {
	seenPairs := map[string]bool{}
	var result []string
	for _, openingID := range openingIDs {
		opening, ok := openingsByID[openingID]
		if !ok {
			continue
		}
		key := firstNonEmpty(opening.PairID, opening.ID)
		if seenPairs[key] {
			continue
		}
		seenPairs[key] = true
		result = append(result, opening.ID)
	}
	return result
}

func thermalPairedOpeningsAreComplete(boundary ThermalBoundaryRecord, counterpart ThermalBoundaryRecord, openingsByID map[string]ThermalOpeningRecord) bool {
	if len(boundary.OpeningIDs) == 0 && len(counterpart.OpeningIDs) == 0 {
		return true
	}
	if len(boundary.OpeningIDs) != len(counterpart.OpeningIDs) {
		return false
	}
	for _, openingID := range boundary.OpeningIDs {
		opening, ok := openingsByID[openingID]
		if !ok || opening.PairID == "" || opening.CounterpartOpeningID == "" {
			return false
		}
	}
	return true
}

func thermalConnectionKey(fromNodeID string, toNodeID string, relationKind string) string {
	fromNodeID, toNodeID = thermalSortedNodeIDs(fromNodeID, toNodeID)
	return strings.Join([]string{relationKind, fromNodeID, toNodeID}, "\x00")
}

func thermalSortedNodeIDs(left string, right string) (string, string) {
	if left > right {
		return right, left
	}
	return left, right
}

func (builder *thermalTopologyBuilder) compactThermalNodeID(nodeID string) string {
	index, ok := builder.nodeIndexByID[nodeID]
	if !ok {
		return nodeID
	}
	node := builder.report.Nodes[index]
	if node.Kind == "space" {
		return builder.zoneNodeIDByName[normalizeName(node.ZoneName)]
	}
	return nodeID
}

func (builder *thermalTopologyBuilder) buildZoneSignatures(areaBasis string) {
	builder.report.ZoneSignatures = []ZoneThermalSignature{}
	openingsByID := map[string]ThermalOpeningRecord{}
	for _, opening := range builder.report.Openings {
		openingsByID[opening.ID] = opening
	}
	enclosureByZoneID := map[string]ZoneEnclosureIntegrity{}
	for _, enclosure := range builder.report.ZoneEnclosures {
		enclosureByZoneID[enclosure.ZoneID] = enclosure
	}
	for _, zone := range builder.geometry.Zones {
		zoneID := builder.zoneNodeIDByName[normalizeName(zone.Name)]
		signature := ZoneThermalSignature{ZoneID: zoneID, ZoneName: zone.Name, AreaBasis: areaBasis}
		for _, space := range builder.geometry.Spaces {
			if strings.EqualFold(space.ZoneName, zone.Name) {
				signature.SpaceIDs = appendUniqueString(signature.SpaceIDs, builder.spaceNodeIDByName[normalizeName(space.Name)])
			}
		}
		totalArea, coveredArea, completeTotalUA := 0.0, 0.0, 0.0
		exteriorComplete, groundComplete, interzoneComplete := true, true, true
		exteriorSeen, groundSeen, interzoneSeen := false, false, false
		exteriorWallGrossArea, exteriorWallOpeningArea := 0.0, 0.0
		for _, boundary := range builder.report.Boundaries {
			if boundary.OwnerZoneID != zoneID {
				continue
			}
			grossArea := boundary.EffectiveGrossArea
			openingArea := boundary.EffectiveOpeningArea
			if strings.EqualFold(areaBasis, "physical") {
				grossArea = boundary.PhysicalGrossArea
				openingArea = boundary.PhysicalOpeningArea
			}
			_, _, boundaryUA, hasUA := thermalBoundaryUAForAreaBasis(boundary, openingsByID, areaBasis)
			transferable := boundary.RelationKind != "adiabatic_explicit" && boundary.RelationKind != "adiabatic_self_reference" && boundary.RelationKind != "invalid"
			if transferable {
				totalArea += grossArea
				if hasUA {
					coveredArea += grossArea
					completeTotalUA += boundaryUA
				}
			}
			signature.DiagnosticIDs = appendUniqueStrings(signature.DiagnosticIDs, boundary.DiagnosticIDs...)
			switch boundary.RelationKind {
			case "exterior":
				signature.ExteriorArea += grossArea
				exteriorSeen = true
				exteriorComplete = exteriorComplete && hasUA
				if hasUA {
					signature.ExteriorUA += boundaryUA
				}
				if strings.EqualFold(boundary.SurfaceType, "Wall") {
					exteriorWallGrossArea += grossArea
					exteriorWallOpeningArea += openingArea
				}
			case "ground", "foundation", "ground_preprocessor":
				signature.GroundArea += grossArea
				groundSeen = true
				groundComplete = groundComplete && hasUA
				if hasUA {
					signature.GroundUA += boundaryUA
				}
			case "interzone_explicit_surface", "interzone_implicit_zone", "interspace_implicit":
				signature.InterzoneArea += grossArea
				interzoneSeen = true
				interzoneComplete = interzoneComplete && hasUA
				if hasUA {
					signature.InterzoneUA += boundaryUA
				}
				targetZoneID := builder.compactThermalNodeID(boundary.TargetID)
				if targetZoneID != "" && targetZoneID != zoneID {
					signature.AdjacentZoneIDs = appendUniqueString(signature.AdjacentZoneIDs, targetZoneID)
				}
			case "adiabatic_explicit", "adiabatic_self_reference":
				signature.AdiabaticArea += grossArea
			default:
				signature.OtherBoundaryArea += grossArea
			}
			signature.WindowArea += openingArea
		}
		if !exteriorSeen || !exteriorComplete {
			signature.ExteriorUA = 0
		}
		if !groundSeen || !groundComplete {
			signature.GroundUA = 0
		}
		if !interzoneSeen || !interzoneComplete {
			signature.InterzoneUA = 0
		}
		if totalArea > 0 {
			signature.UACoverage = roundedNumber(coveredArea/totalArea, 4)
			signature.HasTotalUA = mathNearlyEqual(coveredArea, totalArea, 1e-6)
		}
		if signature.HasTotalUA {
			signature.TotalUA = roundedNumber(completeTotalUA, 4)
		}
		if exteriorWallGrossArea > 0 {
			signature.ExteriorWWR = roundedNumber(exteriorWallOpeningArea/exteriorWallGrossArea, 4)
		}
		for _, coupling := range builder.report.AirCouplings {
			fromNodeID := builder.compactThermalNodeID(coupling.FromNodeID)
			toNodeID := builder.compactThermalNodeID(coupling.ToNodeID)
			if fromNodeID == zoneID && strings.HasPrefix(toNodeID, "zone:") {
				signature.AirCoupledZoneIDs = appendUniqueString(signature.AirCoupledZoneIDs, toNodeID)
			}
			if toNodeID == zoneID && strings.HasPrefix(fromNodeID, "zone:") {
				signature.AirCoupledZoneIDs = appendUniqueString(signature.AirCoupledZoneIDs, fromNodeID)
			}
		}
		if enclosure, ok := enclosureByZoneID[zoneID]; ok {
			signature.ClosedShell = enclosure.ClosedShell
			signature.OpenEdgeCount = enclosure.OpenEdgeCount
			signature.NonManifoldEdgeCount = enclosure.NonManifoldEdgeCount
			signature.ComputedVolume = enclosure.ComputedVolume
			signature.DeclaredVolume = enclosure.DeclaredVolume
			signature.VolumeDifferencePct = enclosure.VolumeDifferencePct
			signature.DiagnosticIDs = appendUniqueStrings(signature.DiagnosticIDs, enclosure.DiagnosticIDs...)
		}
		signature.ExteriorArea = roundedNumber(signature.ExteriorArea, 3)
		signature.GroundArea = roundedNumber(signature.GroundArea, 3)
		signature.InterzoneArea = roundedNumber(signature.InterzoneArea, 3)
		signature.AdiabaticArea = roundedNumber(signature.AdiabaticArea, 3)
		signature.OtherBoundaryArea = roundedNumber(signature.OtherBoundaryArea, 3)
		signature.WindowArea = roundedNumber(signature.WindowArea, 3)
		signature.ExteriorUA = roundedNumber(signature.ExteriorUA, 4)
		signature.GroundUA = roundedNumber(signature.GroundUA, 4)
		signature.InterzoneUA = roundedNumber(signature.InterzoneUA, 4)
		sort.Strings(signature.SpaceIDs)
		sort.Strings(signature.AdjacentZoneIDs)
		sort.Strings(signature.AirCoupledZoneIDs)
		sort.Strings(signature.DiagnosticIDs)
		builder.report.ZoneSignatures = append(builder.report.ZoneSignatures, signature)
	}
	sort.SliceStable(builder.report.ZoneSignatures, func(i, j int) bool {
		return builder.report.ZoneSignatures[i].ZoneID < builder.report.ZoneSignatures[j].ZoneID
	})
}

func (builder *thermalTopologyBuilder) buildThermalMatrix(areaBasis string) {
	builder.report.Matrix = []ThermalMatrixCell{}
	for _, connection := range builder.report.Connections {
		area := connection.EffectiveGrossArea
		ua := connection.TotalUA
		hasUA := connection.HasUA
		if strings.EqualFold(areaBasis, "physical") {
			area = connection.PhysicalGrossArea
			ua = connection.PhysicalTotalUA
			hasUA = connection.HasPhysicalUA
		}
		cell := ThermalMatrixCell{
			RowNodeID:       connection.FromNodeID,
			ColumnNodeID:    connection.ToNodeID,
			ConnectionID:    connection.ID,
			SurfaceCount:    connection.SurfaceCount,
			Area:            area,
			UA:              ua,
			HasUA:           hasUA,
			DiagnosticCount: len(connection.DiagnosticIDs),
		}
		builder.report.Matrix = append(builder.report.Matrix, cell)
		if connection.FromNodeID != connection.ToNodeID {
			mirror := cell
			mirror.RowNodeID, mirror.ColumnNodeID = cell.ColumnNodeID, cell.RowNodeID
			builder.report.Matrix = append(builder.report.Matrix, mirror)
		}
	}
	sort.SliceStable(builder.report.Matrix, func(i, j int) bool {
		left := builder.report.Matrix[i].RowNodeID + "\x00" + builder.report.Matrix[i].ColumnNodeID + "\x00" + builder.report.Matrix[i].ConnectionID
		right := builder.report.Matrix[j].RowNodeID + "\x00" + builder.report.Matrix[j].ColumnNodeID + "\x00" + builder.report.Matrix[j].ConnectionID
		return left < right
	})
}

func mathNearlyEqual(a float64, b float64, tolerance float64) bool {
	if a > b {
		return a-b <= tolerance
	}
	return b-a <= tolerance
}
