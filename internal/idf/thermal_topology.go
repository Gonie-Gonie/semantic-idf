package idf

import (
	"fmt"
	"sort"
	"strings"
)

const thermalTopologySchema = "semantic-idf.thermal-topology/v1"

type ThermalTopologyReport struct {
	Schema         string                       `json:"schema"`
	Nodes          []ThermalTopologyNode        `json:"nodes"`
	Boundaries     []ThermalBoundaryRecord      `json:"boundaries"`
	Connections    []ThermalConnectionAggregate `json:"connections"`
	Openings       []ThermalOpeningRecord       `json:"openings,omitempty"`
	AirCouplings   []ThermalAirCoupling         `json:"airCouplings,omitempty"`
	ZoneSignatures []ZoneThermalSignature       `json:"zoneSignatures"`
	Matrix         []ThermalMatrixCell          `json:"matrix,omitempty"`
	IssueLinks     []ThermalTopologyIssueLink   `json:"issueLinks,omitempty"`
	Stats          ThermalTopologyStats         `json:"stats"`
}

type ThermalTopologyNode struct {
	ID            string                 `json:"id"`
	EntityID      string                 `json:"entityId,omitempty"`
	Kind          string                 `json:"kind"`
	Label         string                 `json:"label"`
	ZoneName      string                 `json:"zoneName,omitempty"`
	SpaceName     string                 `json:"spaceName,omitempty"`
	StoryIndex    int                    `json:"storyIndex,omitempty"`
	ObjectType    string                 `json:"objectType,omitempty"`
	ObjectName    string                 `json:"objectName,omitempty"`
	ObjectIndex   *int                   `json:"objectIndex,omitempty"`
	PhysicalArea  float64                `json:"physicalArea,omitempty"`
	EffectiveArea float64                `json:"effectiveArea,omitempty"`
	FloorArea     float64                `json:"floorArea,omitempty"`
	Volume        float64                `json:"volume,omitempty"`
	Centroid      GeometryPoint          `json:"centroid,omitempty"`
	DiagnosticIDs []string               `json:"diagnosticIds,omitempty"`
	SourceAnchors []SemanticSourceAnchor `json:"sourceAnchors,omitempty"`
}

type ThermalBoundaryRecord struct {
	ID                 string `json:"id"`
	SurfaceID          string `json:"surfaceId"`
	SurfaceEntityID    string `json:"surfaceEntityId"`
	SurfaceObjectIndex int    `json:"surfaceObjectIndex"`
	SurfaceName        string `json:"surfaceName"`
	SurfaceType        string `json:"surfaceType"`

	OwnerZoneID  string `json:"ownerZoneId"`
	OwnerSpaceID string `json:"ownerSpaceId,omitempty"`

	BoundaryConditionRaw string `json:"boundaryConditionRaw"`
	BoundaryCondition    string `json:"boundaryCondition"`
	BoundaryObjectRaw    string `json:"boundaryObjectRaw,omitempty"`
	RelationKind         string `json:"relationKind"`

	TargetKind string `json:"targetKind"`
	TargetID   string `json:"targetId"`
	TargetName string `json:"targetName,omitempty"`

	CounterpartSurfaceID       string `json:"counterpartSurfaceId,omitempty"`
	CounterpartSurfaceEntityID string `json:"counterpartSurfaceEntityId,omitempty"`
	PairID                     string `json:"pairId,omitempty"`
	VirtualCounterpart         bool   `json:"virtualCounterpart,omitempty"`

	ConstructionName        string  `json:"constructionName,omitempty"`
	ConstructionObjectIndex *int    `json:"constructionObjectIndex,omitempty"`
	UValue                  float64 `json:"uValue,omitempty"`
	HasUValue               bool    `json:"hasUValue"`

	PhysicalGrossArea    float64 `json:"physicalGrossArea"`
	PhysicalOpeningArea  float64 `json:"physicalOpeningArea"`
	PhysicalOpaqueArea   float64 `json:"physicalOpaqueArea"`
	EffectiveGrossArea   float64 `json:"effectiveGrossArea"`
	EffectiveOpeningArea float64 `json:"effectiveOpeningArea"`
	EffectiveOpaqueArea  float64 `json:"effectiveOpaqueArea"`

	OpaqueUA  float64 `json:"opaqueUa,omitempty"`
	OpeningUA float64 `json:"openingUa,omitempty"`
	TotalUA   float64 `json:"totalUa,omitempty"`
	HasUA     bool    `json:"hasUa"`

	Orientation  string  `json:"orientation,omitempty"`
	Azimuth      float64 `json:"azimuth,omitempty"`
	SunExposure  string  `json:"sunExposure,omitempty"`
	WindExposure string  `json:"windExposure,omitempty"`

	OpeningIDs    []string               `json:"openingIds,omitempty"`
	DiagnosticIDs []string               `json:"diagnosticIds,omitempty"`
	SourceAnchors []SemanticSourceAnchor `json:"sourceAnchors,omitempty"`
}

type ThermalOpeningRecord struct {
	ID                   string                 `json:"id"`
	WindowID             string                 `json:"windowId"`
	EntityID             string                 `json:"entityId"`
	ObjectIndex          int                    `json:"objectIndex"`
	Name                 string                 `json:"name"`
	SurfaceType          string                 `json:"surfaceType"`
	BaseSurfaceID        string                 `json:"baseSurfaceId"`
	OwnerZoneID          string                 `json:"ownerZoneId"`
	OwnerSpaceID         string                 `json:"ownerSpaceId,omitempty"`
	CounterpartOpeningID string                 `json:"counterpartOpeningId,omitempty"`
	PairID               string                 `json:"pairId,omitempty"`
	ConstructionName     string                 `json:"constructionName,omitempty"`
	UValue               float64                `json:"uValue,omitempty"`
	HasUValue            bool                   `json:"hasUValue"`
	PhysicalArea         float64                `json:"physicalArea"`
	EffectiveArea        float64                `json:"effectiveArea"`
	UA                   float64                `json:"ua,omitempty"`
	HasUA                bool                   `json:"hasUa"`
	DiagnosticIDs        []string               `json:"diagnosticIds,omitempty"`
	SourceAnchors        []SemanticSourceAnchor `json:"sourceAnchors,omitempty"`
}

type ThermalAirCoupling struct {
	ID             string                 `json:"id"`
	EntityID       string                 `json:"entityId"`
	ObjectType     string                 `json:"objectType"`
	ObjectName     string                 `json:"objectName,omitempty"`
	ObjectIndex    int                    `json:"objectIndex"`
	FromNodeID     string                 `json:"fromNodeId"`
	ToNodeID       string                 `json:"toNodeId"`
	Direction      string                 `json:"direction"`
	CouplingKind   string                 `json:"couplingKind"`
	DesignFlowRate float64                `json:"designFlowRate,omitempty"`
	Unit           string                 `json:"unit,omitempty"`
	ScheduleName   string                 `json:"scheduleName,omitempty"`
	SurfaceID      string                 `json:"surfaceId,omitempty"`
	ComponentName  string                 `json:"componentName,omitempty"`
	DiagnosticIDs  []string               `json:"diagnosticIds,omitempty"`
	SourceAnchors  []SemanticSourceAnchor `json:"sourceAnchors,omitempty"`
}

type ThermalConnectionAggregate struct {
	ID                   string                 `json:"id"`
	FromNodeID           string                 `json:"fromNodeId"`
	ToNodeID             string                 `json:"toNodeId"`
	RelationKind         string                 `json:"relationKind"`
	BoundaryIDs          []string               `json:"boundaryIds"`
	OpeningIDs           []string               `json:"openingIds,omitempty"`
	AirCouplingIDs       []string               `json:"airCouplingIds,omitempty"`
	SurfaceCount         int                    `json:"surfaceCount"`
	OpeningCount         int                    `json:"openingCount"`
	PhysicalGrossArea    float64                `json:"physicalGrossArea"`
	PhysicalOpaqueArea   float64                `json:"physicalOpaqueArea"`
	PhysicalOpeningArea  float64                `json:"physicalOpeningArea"`
	EffectiveGrossArea   float64                `json:"effectiveGrossArea"`
	EffectiveOpaqueArea  float64                `json:"effectiveOpaqueArea"`
	EffectiveOpeningArea float64                `json:"effectiveOpeningArea"`
	OpaqueUA             float64                `json:"opaqueUa,omitempty"`
	OpeningUA            float64                `json:"openingUa,omitempty"`
	TotalUA              float64                `json:"totalUa,omitempty"`
	HasUA                bool                   `json:"hasUa"`
	Orientations         []string               `json:"orientations,omitempty"`
	DiagnosticIDs        []string               `json:"diagnosticIds,omitempty"`
	SourceAnchors        []SemanticSourceAnchor `json:"sourceAnchors,omitempty"`
}

type ZoneThermalSignature struct {
	ZoneID            string   `json:"zoneId"`
	ZoneName          string   `json:"zoneName"`
	SpaceIDs          []string `json:"spaceIds,omitempty"`
	ExteriorArea      float64  `json:"exteriorArea"`
	GroundArea        float64  `json:"groundArea"`
	InterzoneArea     float64  `json:"interzoneArea"`
	AdiabaticArea     float64  `json:"adiabaticArea"`
	OtherBoundaryArea float64  `json:"otherBoundaryArea"`
	ExteriorUA        float64  `json:"exteriorUa,omitempty"`
	GroundUA          float64  `json:"groundUa,omitempty"`
	InterzoneUA       float64  `json:"interzoneUa,omitempty"`
	TotalUA           float64  `json:"totalUa,omitempty"`
	WindowArea        float64  `json:"windowArea"`
	ExteriorWWR       float64  `json:"exteriorWwr,omitempty"`
	AdjacentZoneIDs   []string `json:"adjacentZoneIds,omitempty"`
	AirCoupledZoneIDs []string `json:"airCoupledZoneIds,omitempty"`
	ClosedShell       bool     `json:"closedShell"`
	OpenEdgeCount     int      `json:"openEdgeCount,omitempty"`
	DiagnosticIDs     []string `json:"diagnosticIds,omitempty"`
}

type ThermalMatrixCell struct {
	RowNodeID       string  `json:"rowNodeId"`
	ColumnNodeID    string  `json:"columnNodeId"`
	ConnectionID    string  `json:"connectionId,omitempty"`
	SurfaceCount    int     `json:"surfaceCount,omitempty"`
	Area            float64 `json:"area,omitempty"`
	UA              float64 `json:"ua,omitempty"`
	HasUA           bool    `json:"hasUa"`
	DiagnosticCount int     `json:"diagnosticCount,omitempty"`
}

type ThermalTopologyIssueLink struct {
	ID            string                 `json:"id"`
	Code          string                 `json:"code"`
	Severity      string                 `json:"severity"`
	Message       string                 `json:"message"`
	EntityID      string                 `json:"entityId,omitempty"`
	BoundaryID    string                 `json:"boundaryId,omitempty"`
	OpeningID     string                 `json:"openingId,omitempty"`
	AirCouplingID string                 `json:"airCouplingId,omitempty"`
	SourceAnchors []SemanticSourceAnchor `json:"sourceAnchors,omitempty"`
}

type ThermalTopologyStats struct {
	NodeCount            int `json:"nodeCount"`
	BoundaryCount        int `json:"boundaryCount"`
	ConnectionCount      int `json:"connectionCount"`
	OpeningCount         int `json:"openingCount"`
	AirCouplingCount     int `json:"airCouplingCount"`
	InvalidBoundaryCount int `json:"invalidBoundaryCount"`
	DiagnosticCount      int `json:"diagnosticCount"`
}

type thermalTopologyBuilder struct {
	doc                   Document
	geometry              GeometryReport
	documentIndex         *DocumentIndex
	registry              semanticSourceRegistry
	boundaryAdapter       thermalOutsideBoundaryAdapter
	objectsByIndex        map[int]Object
	nodeIndexByID         map[string]int
	zoneNodeIDByName      map[string]string
	spaceNodeIDByName     map[string]string
	boundaryIndexesByName map[string][]int
	report                ThermalTopologyReport
}

func BuildThermalTopology(doc Document, geometry GeometryReport, documentIndex *DocumentIndex) ThermalTopologyReport {
	if documentIndex == nil {
		documentIndex = NewDocumentIndex(doc)
	}
	builder := thermalTopologyBuilder{
		doc:                   doc,
		geometry:              geometry,
		documentIndex:         documentIndex,
		registry:              newSemanticSourceRegistry(doc),
		boundaryAdapter:       newThermalOutsideBoundaryAdapter(doc),
		objectsByIndex:        map[int]Object{},
		nodeIndexByID:         map[string]int{},
		zoneNodeIDByName:      map[string]string{},
		spaceNodeIDByName:     map[string]string{},
		boundaryIndexesByName: map[string][]int{},
		report: ThermalTopologyReport{
			Schema:         thermalTopologySchema,
			Nodes:          []ThermalTopologyNode{},
			Boundaries:     []ThermalBoundaryRecord{},
			Connections:    []ThermalConnectionAggregate{},
			ZoneSignatures: []ZoneThermalSignature{},
		},
	}
	for _, object := range doc.Objects {
		builder.objectsByIndex[object.Index] = object
	}
	builder.addOwnedNodes()
	builder.addBoundaryRecords()
	builder.resolveBoundaryRecords()
	builder.finalize()
	return builder.report
}

func (builder *thermalTopologyBuilder) addOwnedNodes() {
	for _, zone := range builder.geometry.Zones {
		object, ok := builder.objectsByIndex[zone.ObjectIndex]
		if !ok {
			continue
		}
		entityID := thermalSemanticEntityID(object, builder.registry)
		node := ThermalTopologyNode{
			ID:            entityID,
			EntityID:      entityID,
			Kind:          "zone",
			Label:         zone.Name,
			ZoneName:      zone.Name,
			StoryIndex:    zone.StoryIndex,
			ObjectType:    object.Type,
			ObjectName:    objectName(object),
			ObjectIndex:   intPtr(object.Index),
			FloorArea:     zone.FloorArea,
			Volume:        zone.Volume,
			Centroid:      thermalZoneCentroid(zone, builder.geometry),
			SourceAnchors: []SemanticSourceAnchor{builder.sourceAnchor(object, nil, "")},
		}
		builder.addNode(node)
		builder.zoneNodeIDByName[normalizeName(zone.Name)] = entityID
	}
	for _, space := range builder.geometry.Spaces {
		object, ok := builder.objectsByIndex[space.ObjectIndex]
		if !ok {
			continue
		}
		entityID := thermalSemanticEntityID(object, builder.registry)
		node := ThermalTopologyNode{
			ID:            entityID,
			EntityID:      entityID,
			Kind:          "space",
			Label:         space.Name,
			ZoneName:      space.ZoneName,
			SpaceName:     space.Name,
			ObjectType:    object.Type,
			ObjectName:    objectName(object),
			ObjectIndex:   intPtr(object.Index),
			Centroid:      thermalSpaceCentroid(space, builder.geometry),
			SourceAnchors: []SemanticSourceAnchor{builder.sourceAnchor(object, nil, "")},
		}
		builder.addNode(node)
		builder.spaceNodeIDByName[normalizeName(space.Name)] = entityID
	}
}

func (builder *thermalTopologyBuilder) addBoundaryRecords() {
	constructionByName := map[string]GeometryConstruction{}
	for _, construction := range builder.geometry.Constructions {
		constructionByName[normalizeName(construction.Name)] = construction
	}
	openingsByBaseSurface := map[string][]GeometryWindow{}
	for _, opening := range builder.geometry.Windows {
		openingsByBaseSurface[opening.BaseSurfaceID] = append(openingsByBaseSurface[opening.BaseSurfaceID], opening)
	}

	for _, surface := range builder.geometry.Surfaces {
		if surface.IsShading {
			continue
		}
		object, ok := builder.objectsByIndex[surface.ObjectIndex]
		if !ok || !isBuildingSurfaceType(object.Type) {
			continue
		}
		boundaryRaw := semanticGeometryOutsideBoundary(object)
		if boundaryRaw == "" {
			boundaryRaw = surface.OutsideBoundary
		}
		boundaryObjectRaw := geometryStringField(object, "Outside Boundary Condition Object")
		construction := constructionByName[normalizeName(surface.Construction)]
		physicalOpeningArea, effectiveOpeningArea := 0.0, 0.0
		openingIDs := make([]string, 0, len(openingsByBaseSurface[surface.ID]))
		for _, opening := range openingsByBaseSurface[surface.ID] {
			physicalOpeningArea += opening.PhysicalArea
			effectiveOpeningArea += opening.EffectiveArea
			openingIDs = append(openingIDs, "thermal-opening-"+strings.TrimPrefix(opening.ID, "window-"))
		}
		physicalOpaqueArea := thermalMax(0, surface.PhysicalArea-physicalOpeningArea)
		effectiveOpaqueArea := thermalMax(0, surface.EffectiveArea-effectiveOpeningArea)
		surfaceEntityID := thermalSemanticEntityID(object, builder.registry)
		boundary := ThermalBoundaryRecord{
			ID:                   "thermal-boundary:" + surfaceEntityID,
			SurfaceID:            surface.ID,
			SurfaceEntityID:      surfaceEntityID,
			SurfaceObjectIndex:   surface.ObjectIndex,
			SurfaceName:          surface.Name,
			SurfaceType:          surface.SurfaceType,
			OwnerZoneID:          builder.zoneNodeIDByName[normalizeName(surface.ZoneName)],
			OwnerSpaceID:         builder.spaceNodeIDByName[normalizeName(surface.SpaceName)],
			BoundaryConditionRaw: boundaryRaw,
			BoundaryCondition:    builder.boundaryAdapter.Canonicalize(boundaryRaw),
			BoundaryObjectRaw:    boundaryObjectRaw,
			RelationKind:         "invalid",
			TargetKind:           "unresolved_target",
			ConstructionName:     surface.Construction,
			HasUValue:            construction.HasThermalPerformance && construction.UValue > 0,
			UValue:               construction.UValue,
			PhysicalGrossArea:    surface.PhysicalArea,
			PhysicalOpeningArea:  roundedNumber(physicalOpeningArea, 3),
			PhysicalOpaqueArea:   roundedNumber(physicalOpaqueArea, 3),
			EffectiveGrossArea:   surface.EffectiveArea,
			EffectiveOpeningArea: roundedNumber(effectiveOpeningArea, 3),
			EffectiveOpaqueArea:  roundedNumber(effectiveOpaqueArea, 3),
			Orientation:          surface.Orientation,
			Azimuth:              surface.Azimuth,
			SunExposure:          geometryStringField(object, "Sun Exposure"),
			WindExposure:         geometryStringField(object, "Wind Exposure"),
			OpeningIDs:           openingIDs,
			SourceAnchors:        builder.boundarySourceAnchors(object),
		}
		if construction.ObjectIndex >= 0 && construction.Name != "" {
			boundary.ConstructionObjectIndex = intPtr(construction.ObjectIndex)
		}
		builder.report.Boundaries = append(builder.report.Boundaries, boundary)
		boundaryIndex := len(builder.report.Boundaries) - 1
		builder.boundaryIndexesByName[normalizeName(surface.Name)] = append(builder.boundaryIndexesByName[normalizeName(surface.Name)], boundaryIndex)
	}
}

func (builder *thermalTopologyBuilder) resolveBoundaryRecords() {
	for index := range builder.report.Boundaries {
		builder.resolveBoundaryRecord(index)
	}
}

func (builder *thermalTopologyBuilder) resolveBoundaryRecord(boundaryIndex int) {
	boundary := &builder.report.Boundaries[boundaryIndex]
	if boundary.OwnerZoneID == "" {
		builder.invalidateBoundary(boundary, "missing_boundary_target", fmt.Sprintf("Surface %q has no resolvable owner zone.", boundary.SurfaceName))
		return
	}
	switch boundary.BoundaryCondition {
	case "Outdoors":
		boundary.RelationKind = "exterior"
		boundary.TargetKind = "outdoors"
		boundary.TargetID = builder.addEnvironmentNode("outdoors", "Outdoors", boundary.SourceAnchors)
		builder.addOutdoorsExposureNode(*boundary)
	case "Ground":
		boundary.RelationKind = "ground"
		boundary.TargetKind = "ground"
		boundary.TargetID = builder.addEnvironmentNode("ground", "Ground", boundary.SourceAnchors)
	case "Foundation":
		builder.resolveNamedExternalObject(boundary, "foundation", "foundation", []string{"Foundation:Kiva"})
	case "GroundFCfactorMethod", "GroundSlabPreprocessorAverage", "GroundSlabPreprocessorCore", "GroundSlabPreprocessorPerimeter", "GroundBasementPreprocessorAverageWall", "GroundBasementPreprocessorAverageFloor", "GroundBasementPreprocessorUpperWall", "GroundBasementPreprocessorLowerWall":
		boundary.RelationKind = "ground_preprocessor"
		boundary.TargetKind = "ground_preprocessor"
		boundary.TargetID = builder.addEnvironmentNode("ground_preprocessor", boundary.BoundaryCondition, boundary.SourceAnchors)
	case "Adiabatic":
		boundary.RelationKind = "adiabatic_explicit"
		boundary.TargetKind = "adiabatic"
		boundary.TargetID = builder.addEnvironmentNode("adiabatic", "Adiabatic", boundary.SourceAnchors)
	case "Surface":
		builder.resolveSurfaceBoundary(boundaryIndex)
	case "Zone":
		builder.resolveImplicitOwnedBoundary(boundary, "zone")
	case "Space":
		builder.resolveImplicitOwnedBoundary(boundary, "space")
	case "OtherSideCoefficients":
		builder.resolveNamedExternalObject(boundary, "other_side_coefficients", "other_side_coefficients", []string{"SurfaceProperty:OtherSideCoefficients"})
	case "OtherSideConditionsModel":
		builder.resolveNamedExternalObject(boundary, "other_side_conditions_model", "other_side_conditions_model", []string{"SurfaceProperty:OtherSideConditionsModel"})
	default:
		builder.invalidateBoundary(boundary, "invalid_boundary_condition", fmt.Sprintf("Surface %q uses invalid outside boundary condition %q.", boundary.SurfaceName, boundary.BoundaryConditionRaw))
	}
	if boundary.RelationKind != "exterior" && (strings.EqualFold(boundary.SunExposure, "SunExposed") || strings.EqualFold(boundary.WindExposure, "WindExposed")) {
		builder.addBoundaryIssue(boundary, "boundary_exposure_rule_mismatch", "warning", "A non-outdoor boundary is marked as sun- or wind-exposed.")
	}
}

func (builder *thermalTopologyBuilder) resolveSurfaceBoundary(boundaryIndex int) {
	boundary := &builder.report.Boundaries[boundaryIndex]
	targetIndexes := builder.boundaryIndexesByName[normalizeName(boundary.BoundaryObjectRaw)]
	if len(targetIndexes) == 0 {
		builder.invalidateBoundary(boundary, "surface_counterpart_missing", fmt.Sprintf("Surface %q references missing counterpart %q.", boundary.SurfaceName, boundary.BoundaryObjectRaw))
		return
	}
	if len(targetIndexes) > 1 {
		builder.invalidateBoundary(boundary, "surface_counterpart_duplicate", fmt.Sprintf("Surface %q references duplicate counterpart name %q.", boundary.SurfaceName, boundary.BoundaryObjectRaw))
		return
	}
	target := &builder.report.Boundaries[targetIndexes[0]]
	if target.SurfaceEntityID == boundary.SurfaceEntityID {
		boundary.RelationKind = "adiabatic_self_reference"
		boundary.TargetKind = "adiabatic"
		boundary.TargetID = builder.addEnvironmentNode("adiabatic", "Adiabatic", boundary.SourceAnchors)
		boundary.VirtualCounterpart = true
		return
	}
	if target.BoundaryCondition != "Surface" || !strings.EqualFold(strings.TrimSpace(target.BoundaryObjectRaw), strings.TrimSpace(boundary.SurfaceName)) {
		builder.invalidateBoundary(boundary, "surface_counterpart_one_way", fmt.Sprintf("Surface pair %q and %q is not reciprocal.", boundary.SurfaceName, target.SurfaceName))
		return
	}
	boundary.RelationKind = "interzone_explicit_surface"
	boundary.CounterpartSurfaceID = target.SurfaceID
	boundary.CounterpartSurfaceEntityID = target.SurfaceEntityID
	boundary.PairID = thermalSurfacePairID(boundary.SurfaceEntityID, target.SurfaceEntityID)
	boundary.TargetKind, boundary.TargetID = thermalOwnedTarget(*target)
	boundary.TargetName = target.SurfaceName
}

func (builder *thermalTopologyBuilder) resolveImplicitOwnedBoundary(boundary *ThermalBoundaryRecord, kind string) {
	targets := builder.documentIndex.ObjectsNamed(boundary.BoundaryObjectRaw)
	matches := make([]Object, 0, len(targets))
	for _, target := range targets {
		if strings.EqualFold(target.Type, kind) {
			matches = append(matches, target)
		}
	}
	if len(matches) != 1 {
		builder.invalidateBoundary(boundary, "missing_boundary_target", fmt.Sprintf("Surface %q cannot resolve %s target %q.", boundary.SurfaceName, kind, boundary.BoundaryObjectRaw))
		return
	}
	targetID := thermalSemanticEntityID(matches[0], builder.registry)
	boundary.TargetKind = kind
	boundary.TargetID = targetID
	boundary.TargetName = objectName(matches[0])
	boundary.VirtualCounterpart = true
	if kind == "space" {
		boundary.RelationKind = "interspace_implicit"
	} else {
		boundary.RelationKind = "interzone_implicit_zone"
	}
}

func (builder *thermalTopologyBuilder) resolveNamedExternalObject(boundary *ThermalBoundaryRecord, relationKind string, targetKind string, objectTypes []string) {
	var matches []Object
	for _, object := range builder.documentIndex.ObjectsNamed(boundary.BoundaryObjectRaw) {
		for _, objectType := range objectTypes {
			if strings.EqualFold(object.Type, objectType) {
				matches = append(matches, object)
				break
			}
		}
	}
	if len(matches) != 1 {
		builder.invalidateBoundary(boundary, "missing_boundary_target", fmt.Sprintf("Surface %q cannot resolve %s target %q.", boundary.SurfaceName, boundary.BoundaryCondition, boundary.BoundaryObjectRaw))
		return
	}
	target := matches[0]
	targetEntityID := thermalSemanticEntityID(target, builder.registry)
	nodeID := "thermal-external:" + targetKind + ":" + strings.TrimPrefix(targetEntityID, "source-object:")
	anchors := append([]SemanticSourceAnchor(nil), boundary.SourceAnchors...)
	anchors = appendUniqueSemanticSourceAnchor(anchors, builder.sourceAnchor(target, nil, ""))
	builder.addNode(ThermalTopologyNode{
		ID:            nodeID,
		EntityID:      targetEntityID,
		Kind:          targetKind,
		Label:         firstNonEmpty(objectName(target), boundary.BoundaryCondition),
		ObjectType:    target.Type,
		ObjectName:    objectName(target),
		ObjectIndex:   intPtr(target.Index),
		SourceAnchors: anchors,
	})
	boundary.RelationKind = relationKind
	boundary.TargetKind = targetKind
	boundary.TargetID = nodeID
	boundary.TargetName = objectName(target)
}

func (builder *thermalTopologyBuilder) invalidateBoundary(boundary *ThermalBoundaryRecord, code string, message string) {
	boundary.RelationKind = "invalid"
	boundary.TargetKind = "unresolved_target"
	boundary.TargetName = boundary.BoundaryObjectRaw
	label := firstNonEmpty(boundary.BoundaryObjectRaw, boundary.BoundaryConditionRaw, "Unresolved target")
	boundary.TargetID = builder.addUnresolvedNode(label, boundary.SourceAnchors)
	builder.addBoundaryIssue(boundary, code, "warning", message)
}

func (builder *thermalTopologyBuilder) addBoundaryIssue(boundary *ThermalBoundaryRecord, code string, severity string, message string) {
	issueID := "topology-issue:" + semanticStableHash(strings.Join([]string{code, boundary.SurfaceEntityID, message}, "\x00"), 20)
	boundary.DiagnosticIDs = appendUniqueString(boundary.DiagnosticIDs, issueID)
	builder.report.IssueLinks = append(builder.report.IssueLinks, ThermalTopologyIssueLink{
		ID:            issueID,
		Code:          code,
		Severity:      severity,
		Message:       message,
		EntityID:      boundary.SurfaceEntityID,
		BoundaryID:    boundary.ID,
		SourceAnchors: append([]SemanticSourceAnchor(nil), boundary.SourceAnchors...),
	})
}

func (builder *thermalTopologyBuilder) addOutdoorsExposureNode(boundary ThermalBoundaryRecord) {
	kind, label := "outdoors", "Outdoors"
	if strings.EqualFold(boundary.SurfaceType, "Roof") || strings.EqualFold(boundary.SurfaceType, "RoofCeiling") || strings.EqualFold(boundary.SurfaceType, "Ceiling") {
		kind, label = "outdoors_roof", "Outdoors / Roof & Sky"
	} else {
		switch strings.ToLower(boundary.Orientation) {
		case "north", "east", "south", "west":
			kind = "outdoors_" + strings.ToLower(boundary.Orientation)
			label = "Outdoors / " + strings.ToUpper(boundary.Orientation[:1]) + strings.ToLower(boundary.Orientation[1:])
		}
	}
	if kind != "outdoors" {
		builder.addEnvironmentNode(kind, label, boundary.SourceAnchors)
	}
}

func (builder *thermalTopologyBuilder) addEnvironmentNode(kind string, label string, anchors []SemanticSourceAnchor) string {
	nodeID := "thermal-environment:" + semanticIDToken(kind)
	builder.addNode(ThermalTopologyNode{
		ID:            nodeID,
		Kind:          kind,
		Label:         label,
		SourceAnchors: append([]SemanticSourceAnchor(nil), anchors...),
	})
	return nodeID
}

func (builder *thermalTopologyBuilder) addUnresolvedNode(label string, anchors []SemanticSourceAnchor) string {
	nodeID := "thermal-unresolved:" + semanticStableHash(normalizeName(label), 16)
	builder.addNode(ThermalTopologyNode{ID: nodeID, Kind: "unresolved_target", Label: label, SourceAnchors: append([]SemanticSourceAnchor(nil), anchors...)})
	return nodeID
}

func (builder *thermalTopologyBuilder) addNode(node ThermalTopologyNode) {
	if index, ok := builder.nodeIndexByID[node.ID]; ok {
		existing := &builder.report.Nodes[index]
		for _, anchor := range node.SourceAnchors {
			existing.SourceAnchors = appendUniqueSemanticSourceAnchor(existing.SourceAnchors, anchor)
		}
		return
	}
	builder.nodeIndexByID[node.ID] = len(builder.report.Nodes)
	builder.report.Nodes = append(builder.report.Nodes, node)
}

func (builder *thermalTopologyBuilder) boundarySourceAnchors(object Object) []SemanticSourceAnchor {
	anchors := []SemanticSourceAnchor{builder.sourceAnchor(object, nil, "")}
	for _, fieldName := range []string{"Outside Boundary Condition", "Outside Boundary Condition Object"} {
		if fieldIndex, ok := thermalFieldIndex(object, fieldName); ok {
			anchors = appendUniqueSemanticSourceAnchor(anchors, builder.sourceAnchor(object, intPtr(fieldIndex), fieldName))
		}
	}
	return anchors
}

func (builder *thermalTopologyBuilder) sourceAnchor(object Object, fieldIndex *int, fieldName string) SemanticSourceAnchor {
	return SemanticSourceAnchor{
		ObjectID:    builder.registry.byObjectIndex[object.Index],
		ObjectIndex: intPtr(object.Index),
		ObjectType:  object.Type,
		ObjectName:  objectName(object),
		FieldIndex:  cloneIntPtr(fieldIndex),
		FieldName:   fieldName,
	}
}

func (builder *thermalTopologyBuilder) finalize() {
	sort.SliceStable(builder.report.Nodes, func(i, j int) bool { return builder.report.Nodes[i].ID < builder.report.Nodes[j].ID })
	sort.SliceStable(builder.report.Boundaries, func(i, j int) bool { return builder.report.Boundaries[i].ID < builder.report.Boundaries[j].ID })
	sort.SliceStable(builder.report.IssueLinks, func(i, j int) bool { return builder.report.IssueLinks[i].ID < builder.report.IssueLinks[j].ID })
	invalidCount := 0
	for _, boundary := range builder.report.Boundaries {
		if boundary.RelationKind == "invalid" {
			invalidCount++
		}
	}
	builder.report.Stats = ThermalTopologyStats{
		NodeCount:            len(builder.report.Nodes),
		BoundaryCount:        len(builder.report.Boundaries),
		ConnectionCount:      len(builder.report.Connections),
		OpeningCount:         len(builder.report.Openings),
		AirCouplingCount:     len(builder.report.AirCouplings),
		InvalidBoundaryCount: invalidCount,
		DiagnosticCount:      len(builder.report.IssueLinks),
	}
}

func thermalSemanticEntityID(object Object, registry semanticSourceRegistry) string {
	objectType := strings.TrimSpace(object.Type)
	typeKey := semanticIDToken(objectType)
	name := strings.TrimSpace(objectName(object))
	nameKey := semanticIDToken(name)
	objectID := registry.byObjectIndex[object.Index]
	entityID := semanticSourceObjectEntityID(objectID)
	switch {
	case strings.EqualFold(objectType, "Zone"):
		entityID = "zone:" + firstNonEmpty(nameKey, objectID)
	case strings.EqualFold(objectType, "Space"):
		zoneName := semanticSpaceFromObject(object).ZoneName
		entityID = "space:" + semanticIDToken(zoneName) + ":" + firstNonEmpty(nameKey, objectID)
	case isFenestrationType(objectType):
		entityID = "fenestration:" + typeKey + ":" + firstNonEmpty(nameKey, objectID)
	case isBuildingSurfaceType(objectType):
		entityID = "surface:" + typeKey + ":" + firstNonEmpty(nameKey, objectID)
	}
	if registry.duplicateName[object.Index] && !strings.HasPrefix(entityID, "source-object:") {
		entityID += ":source:" + strings.TrimPrefix(objectID, "obj-")
	}
	return entityID
}

func thermalFieldIndex(object Object, fieldName string) (int, bool) {
	wanted := normalizeFieldName(fieldName)
	for index := range object.Fields {
		if spec, ok := fieldSpecAt(object.Type, index); ok && normalizeFieldName(spec.Name) == wanted {
			return index, true
		}
	}
	words := strings.Fields(strings.ToLower(fieldName))
	for index, field := range object.Fields {
		comment := strings.ToLower(field.Comment)
		matched := true
		for _, word := range words {
			if !strings.Contains(comment, word) {
				matched = false
				break
			}
		}
		if matched {
			return index, true
		}
	}
	return -1, false
}

func canonicalOutsideBoundaryCondition(value string) string {
	return newThermalOutsideBoundaryAdapter(Document{}).Canonicalize(value)
}

type thermalOutsideBoundaryAdapter struct {
	EnergyPlusVersion string
	choices           map[string]string
}

func newThermalOutsideBoundaryAdapter(doc Document) thermalOutsideBoundaryAdapter {
	return thermalOutsideBoundaryAdapter{
		EnergyPlusVersion: thermalEnergyPlusVersion(doc),
		choices: map[string]string{
			"outdoors":                               "Outdoors",
			"outside":                                "Outdoors",
			"surface":                                "Surface",
			"zone":                                   "Zone",
			"otherzone":                              "Zone",
			"space":                                  "Space",
			"adiabatic":                              "Adiabatic",
			"ground":                                 "Ground",
			"foundation":                             "Foundation",
			"groundfcfactormethod":                   "GroundFCfactorMethod",
			"othersidecoefficients":                  "OtherSideCoefficients",
			"othersidecoefficient":                   "OtherSideCoefficients",
			"othersideconditionsmodel":               "OtherSideConditionsModel",
			"groundslabpreprocessoraverage":          "GroundSlabPreprocessorAverage",
			"groundslabpreprocessorcore":             "GroundSlabPreprocessorCore",
			"groundslabpreprocessorperimeter":        "GroundSlabPreprocessorPerimeter",
			"groundbasementpreprocessoraveragewall":  "GroundBasementPreprocessorAverageWall",
			"groundbasementpreprocessoraveragefloor": "GroundBasementPreprocessorAverageFloor",
			"groundbasementpreprocessorupperwall":    "GroundBasementPreprocessorUpperWall",
			"groundbasementpreprocessorlowerwall":    "GroundBasementPreprocessorLowerWall",
		},
	}
}

func (adapter thermalOutsideBoundaryAdapter) Canonicalize(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "_", "", "-", "", ":", "")
	key = replacer.Replace(key)
	return adapter.choices[key]
}

func thermalEnergyPlusVersion(doc Document) string {
	for _, object := range doc.Objects {
		if strings.EqualFold(object.Type, "Version") && len(object.Fields) > 0 {
			return strings.TrimSpace(object.Fields[0].Value)
		}
	}
	return "unknown"
}

func thermalSurfacePairID(surfaceEntityA string, surfaceEntityB string) string {
	values := []string{surfaceEntityA, surfaceEntityB}
	sort.Strings(values)
	return "thermal-pair:" + semanticStableHash(strings.Join(values, "\x00"), 20)
}

func thermalOwnedTarget(boundary ThermalBoundaryRecord) (string, string) {
	if boundary.OwnerSpaceID != "" {
		return "space", boundary.OwnerSpaceID
	}
	return "zone", boundary.OwnerZoneID
}

func thermalZoneCentroid(zone GeometryZone, geometry GeometryReport) GeometryPoint {
	points := make([]GeometryPoint, 0)
	for _, surface := range geometry.Surfaces {
		if !surface.IsShading && strings.EqualFold(surface.ZoneName, zone.Name) {
			points = append(points, surface.WorldVertices...)
		}
	}
	return thermalCentroid(points)
}

func thermalSpaceCentroid(space GeometrySpace, geometry GeometryReport) GeometryPoint {
	points := make([]GeometryPoint, 0)
	for _, surface := range geometry.Surfaces {
		if !surface.IsShading && strings.EqualFold(surface.SpaceName, space.Name) {
			points = append(points, surface.WorldVertices...)
		}
	}
	return thermalCentroid(points)
}

func thermalCentroid(points []GeometryPoint) GeometryPoint {
	if len(points) == 0 {
		return GeometryPoint{}
	}
	var centroid GeometryPoint
	for _, point := range points {
		centroid.X += point.X
		centroid.Y += point.Y
		centroid.Z += point.Z
	}
	centroid.X = roundedNumber(centroid.X/float64(len(points)), 4)
	centroid.Y = roundedNumber(centroid.Y/float64(len(points)), 4)
	centroid.Z = roundedNumber(centroid.Z/float64(len(points)), 4)
	return centroid
}

func thermalMax(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
