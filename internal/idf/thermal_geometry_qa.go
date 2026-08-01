package idf

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const thermalGeometryRuleVersion = "semantic-idf.geometry-qa/v1"

type SurfaceGeometryDescriptor struct {
	SurfaceID     string         `json:"surfaceId"`
	PlaneNormal   GeometryPoint  `json:"planeNormal"`
	PlaneOffset   float64        `json:"planeOffset"`
	Centroid      GeometryPoint  `json:"centroid"`
	Bounds        GeometryBounds `json:"bounds"`
	PhysicalArea  float64        `json:"physicalArea"`
	EdgeSignature []string       `json:"edgeSignature"`
}

type ThermalGeometryCheck struct {
	Status            string  `json:"status"`
	AreaDifferencePct float64 `json:"areaDifferencePct,omitempty"`
	OverlapRatio      float64 `json:"overlapRatio,omitempty"`
	NormalDot         float64 `json:"normalDot,omitempty"`
	PlaneDistance     float64 `json:"planeDistance,omitempty"`
	Message           string  `json:"message,omitempty"`
}

type GeometricAdjacencyObservation struct {
	SurfaceAID         string  `json:"surfaceAId"`
	SurfaceBID         string  `json:"surfaceBId"`
	OverlapRatio       float64 `json:"overlapRatio"`
	DeclaredConnection bool    `json:"declaredConnection"`
	ObservationKind    string  `json:"observationKind"`
}

type ThermalOpenEdge struct {
	SurfaceID string        `json:"surfaceId"`
	Start     GeometryPoint `json:"start"`
	End       GeometryPoint `json:"end"`
}

type ZoneEnclosureIntegrity struct {
	ZoneID               string            `json:"zoneId"`
	ZoneName             string            `json:"zoneName"`
	ClosedShell          bool              `json:"closedShell"`
	OpenEdgeCount        int               `json:"openEdgeCount"`
	NonManifoldEdgeCount int               `json:"nonManifoldEdgeCount"`
	ComputedVolume       float64           `json:"computedVolume,omitempty"`
	DeclaredVolume       float64           `json:"declaredVolume,omitempty"`
	HasDeclaredVolume    bool              `json:"hasDeclaredVolume"`
	VolumeDifferencePct  float64           `json:"volumeDifferencePct,omitempty"`
	OpenEdges            []ThermalOpenEdge `json:"openEdges,omitempty"`
	DiagnosticIDs        []string          `json:"diagnosticIds,omitempty"`
}

type thermalPoint2 struct {
	x float64
	y float64
}

func (builder *thermalTopologyBuilder) analyzeGeometryQA() {
	tolerance := thermalGeometryTolerance(builder.geometry.Bounds)
	builder.report.GeometryTolerance = roundedNumber(tolerance, 8)
	builder.report.GeometryRuleVersion = thermalGeometryRuleVersion
	descriptors := builder.buildSurfaceGeometryDescriptors(tolerance)
	builder.validateDeclaredGeometry(descriptors, tolerance)
	builder.buildAdjacencyObservations(descriptors, tolerance)
	builder.buildZoneEnclosures(tolerance)
}

func thermalGeometryTolerance(bounds GeometryBounds) float64 {
	if !bounds.OK {
		return 1e-5
	}
	dx := bounds.MaxX - bounds.MinX
	dy := bounds.MaxY - bounds.MinY
	dz := bounds.MaxZ - bounds.MinZ
	diagonal := math.Sqrt(dx*dx + dy*dy + dz*dz)
	return math.Max(1e-5, diagonal*1e-6)
}

func (builder *thermalTopologyBuilder) buildSurfaceGeometryDescriptors(tolerance float64) map[string]SurfaceGeometryDescriptor {
	descriptors := map[string]SurfaceGeometryDescriptor{}
	for _, surface := range builder.geometry.Surfaces {
		if surface.IsShading || len(surface.WorldVertices) < 3 {
			continue
		}
		vertices := geometryPoint3s(surface.WorldVertices)
		normal, ok := polygonNormal(vertices)
		if !ok {
			continue
		}
		magnitude := math.Sqrt(normal.x*normal.x + normal.y*normal.y + normal.z*normal.z)
		normal.x /= magnitude
		normal.y /= magnitude
		normal.z /= magnitude
		if strings.EqualFold(builder.geometry.VertexEntryDirection, "clockwise") {
			normal.x *= -1
			normal.y *= -1
			normal.z *= -1
		}
		centroid := thermalCentroid(surface.WorldVertices)
		bounds := thermalSurfaceBounds(surface.WorldVertices)
		edges := make([]string, 0, len(surface.WorldVertices))
		for index, start := range surface.WorldVertices {
			end := surface.WorldVertices[(index+1)%len(surface.WorldVertices)]
			edges = append(edges, thermalEdgeSignature(start, end, tolerance))
		}
		sort.Strings(edges)
		descriptor := SurfaceGeometryDescriptor{
			SurfaceID:     surface.ID,
			PlaneNormal:   GeometryPoint{X: roundedNumber(normal.x, 8), Y: roundedNumber(normal.y, 8), Z: roundedNumber(normal.z, 8)},
			PlaneOffset:   roundedNumber(normal.x*centroid.X+normal.y*centroid.Y+normal.z*centroid.Z, 8),
			Centroid:      centroid,
			Bounds:        bounds,
			PhysicalArea:  surface.PhysicalArea,
			EdgeSignature: edges,
		}
		descriptors[surface.ID] = descriptor
		builder.report.GeometryDescriptors = append(builder.report.GeometryDescriptors, descriptor)
	}
	sort.SliceStable(builder.report.GeometryDescriptors, func(i, j int) bool {
		return builder.report.GeometryDescriptors[i].SurfaceID < builder.report.GeometryDescriptors[j].SurfaceID
	})
	return descriptors
}

func thermalSurfaceBounds(points []GeometryPoint) GeometryBounds {
	var bounds GeometryBounds
	for _, point := range points {
		if !bounds.OK {
			bounds = GeometryBounds{MinX: point.X, MaxX: point.X, MinY: point.Y, MaxY: point.Y, MinZ: point.Z, MaxZ: point.Z, OK: true}
			continue
		}
		bounds.MinX = math.Min(bounds.MinX, point.X)
		bounds.MaxX = math.Max(bounds.MaxX, point.X)
		bounds.MinY = math.Min(bounds.MinY, point.Y)
		bounds.MaxY = math.Max(bounds.MaxY, point.Y)
		bounds.MinZ = math.Min(bounds.MinZ, point.Z)
		bounds.MaxZ = math.Max(bounds.MaxZ, point.Z)
	}
	return bounds
}

func thermalEdgeSignature(start GeometryPoint, end GeometryPoint, tolerance float64) string {
	startKey := thermalPointSignature(start, tolerance)
	endKey := thermalPointSignature(end, tolerance)
	if startKey > endKey {
		startKey, endKey = endKey, startKey
	}
	return startKey + "|" + endKey
}

func thermalPointSignature(point GeometryPoint, tolerance float64) string {
	return fmt.Sprintf("%d,%d,%d", thermalQuantize(point.X, tolerance), thermalQuantize(point.Y, tolerance), thermalQuantize(point.Z, tolerance))
}

func thermalQuantize(value float64, tolerance float64) int64 {
	if tolerance <= 0 {
		tolerance = 1e-5
	}
	return int64(math.Round(value / tolerance))
}

func (builder *thermalTopologyBuilder) validateDeclaredGeometry(descriptors map[string]SurfaceGeometryDescriptor, tolerance float64) {
	visited := map[string]bool{}
	for index := range builder.report.Boundaries {
		boundary := &builder.report.Boundaries[index]
		boundary.GeometryCheck = ThermalGeometryCheck{Status: "not_applicable"}
		if boundary.PairID == "" || visited[boundary.PairID] {
			continue
		}
		visited[boundary.PairID] = true
		counterpartIndex, ok := builder.boundaryIndexBySurfaceID[boundary.CounterpartSurfaceID]
		if !ok {
			continue
		}
		counterpart := &builder.report.Boundaries[counterpartIndex]
		descriptorA, hasA := descriptors[boundary.SurfaceID]
		descriptorB, hasB := descriptors[counterpart.SurfaceID]
		if !hasA || !hasB {
			continue
		}
		check := builder.compareDeclaredGeometry(*boundary, *counterpart, descriptorA, descriptorB, tolerance)
		boundary.GeometryCheck = check
		counterpart.GeometryCheck = check
		observationKind := "declared_and_geometrically_matched"
		if check.Status == "invalid" {
			observationKind = "declared_but_geometry_mismatched"
			builder.addGeometryCheckIssues(boundary, check, tolerance)
		}
		builder.report.AdjacencyObservations = append(builder.report.AdjacencyObservations, GeometricAdjacencyObservation{
			SurfaceAID:         boundary.SurfaceID,
			SurfaceBID:         counterpart.SurfaceID,
			OverlapRatio:       check.OverlapRatio,
			DeclaredConnection: true,
			ObservationKind:    observationKind,
		})
	}
}

func (builder *thermalTopologyBuilder) compareDeclaredGeometry(boundaryA ThermalBoundaryRecord, boundaryB ThermalBoundaryRecord, descriptorA SurfaceGeometryDescriptor, descriptorB SurfaceGeometryDescriptor, tolerance float64) ThermalGeometryCheck {
	normalDot := dotGeometryPoints(descriptorA.PlaneNormal, descriptorB.PlaneNormal)
	planeDistance := math.Abs(dotGeometryPoints(descriptorA.PlaneNormal, descriptorB.Centroid) - descriptorA.PlaneOffset)
	areaDifference := relativeDifference(descriptorA.PhysicalArea, descriptorB.PhysicalArea)
	overlapRatio := thermalPolygonOverlapRatio(builder.surfaceWorldVertices(boundaryA.SurfaceID), builder.surfaceWorldVertices(boundaryB.SurfaceID), descriptorA.PlaneNormal, tolerance)
	allowedPlaneDistance := tolerance * 10
	for _, constructionName := range []string{boundaryA.ConstructionName, boundaryB.ConstructionName} {
		if construction := builder.constructionByName[normalizeName(constructionName)]; construction.HasThickness {
			allowedPlaneDistance = math.Max(allowedPlaneDistance, construction.TotalThickness+tolerance*10)
		}
	}
	valid := normalDot <= -0.99 && planeDistance <= allowedPlaneDistance && areaDifference <= 0.01 && overlapRatio >= 0.99
	message := "Declared counterpart geometry is valid."
	if !valid {
		message = fmt.Sprintf("Declared counterpart geometry is invalid (tolerance %.8g m, allowed plane distance %.8g m).", tolerance, allowedPlaneDistance)
	}
	return ThermalGeometryCheck{
		Status:            map[bool]string{true: "valid", false: "invalid"}[valid],
		AreaDifferencePct: roundedNumber(areaDifference*100, 3),
		OverlapRatio:      roundedNumber(overlapRatio, 4),
		NormalDot:         roundedNumber(normalDot, 4),
		PlaneDistance:     roundedNumber(planeDistance, 8),
		Message:           message,
	}
}

func (builder *thermalTopologyBuilder) addGeometryCheckIssues(boundary *ThermalBoundaryRecord, check ThermalGeometryCheck, tolerance float64) {
	evidence := fmt.Sprintf(" Geometry QA tolerance: %.8g m.", tolerance)
	if check.AreaDifferencePct > 1 {
		builder.addBoundaryIssue(boundary, "surface_pair_area_mismatch", "warning", fmt.Sprintf("Surface pair area differs by %.3f%%.%s", check.AreaDifferencePct, evidence))
	}
	if check.PlaneDistance > tolerance*10 {
		builder.addBoundaryIssue(boundary, "surface_pair_plane_mismatch", "warning", fmt.Sprintf("Surface pair plane distance is %.8g m.%s", check.PlaneDistance, evidence))
	}
	if check.NormalDot > -0.99 {
		builder.addBoundaryIssue(boundary, "surface_pair_normal_mismatch", "warning", fmt.Sprintf("Surface pair normals have dot product %.4f.%s", check.NormalDot, evidence))
	}
	if check.OverlapRatio < 0.99 {
		builder.addBoundaryIssue(boundary, "surface_pair_overlap_mismatch", "warning", fmt.Sprintf("Surface pair overlap ratio is %.4f.%s", check.OverlapRatio, evidence))
	}
}

func dotGeometryPoints(a GeometryPoint, b GeometryPoint) float64 {
	return a.X*b.X + a.Y*b.Y + a.Z*b.Z
}

func (builder *thermalTopologyBuilder) surfaceWorldVertices(surfaceID string) []GeometryPoint {
	return builder.surfaceByID[surfaceID].WorldVertices
}

func thermalPolygonOverlapRatio(polygonA []GeometryPoint, polygonB []GeometryPoint, normal GeometryPoint, tolerance float64) float64 {
	if len(polygonA) < 3 || len(polygonB) < 3 {
		return 0
	}
	axis := dominantGeometryAxis(normal)
	projectedA := projectGeometryPolygon(polygonA, axis)
	projectedB := projectGeometryPolygon(polygonB, axis)
	areaA := math.Abs(thermalPolygonArea2(projectedA))
	areaB := math.Abs(thermalPolygonArea2(projectedB))
	if areaA <= tolerance*tolerance || areaB <= tolerance*tolerance {
		return 0
	}
	intersection := thermalConvexClip(projectedA, projectedB, tolerance)
	intersectionArea := math.Abs(thermalPolygonArea2(intersection))
	return math.Min(1, intersectionArea/math.Min(areaA, areaB))
}

func dominantGeometryAxis(normal GeometryPoint) int {
	ax, ay, az := math.Abs(normal.X), math.Abs(normal.Y), math.Abs(normal.Z)
	if ax >= ay && ax >= az {
		return 0
	}
	if ay >= az {
		return 1
	}
	return 2
}

func projectGeometryPolygon(points []GeometryPoint, dropAxis int) []thermalPoint2 {
	projected := make([]thermalPoint2, 0, len(points))
	for _, point := range points {
		switch dropAxis {
		case 0:
			projected = append(projected, thermalPoint2{x: point.Y, y: point.Z})
		case 1:
			projected = append(projected, thermalPoint2{x: point.X, y: point.Z})
		default:
			projected = append(projected, thermalPoint2{x: point.X, y: point.Y})
		}
	}
	return projected
}

func thermalPolygonArea2(points []thermalPoint2) float64 {
	if len(points) < 3 {
		return 0
	}
	area := 0.0
	for index, point := range points {
		next := points[(index+1)%len(points)]
		area += point.x*next.y - next.x*point.y
	}
	return area / 2
}

func thermalConvexClip(subject []thermalPoint2, clip []thermalPoint2, tolerance float64) []thermalPoint2 {
	output := append([]thermalPoint2(nil), subject...)
	orientation := 1.0
	if thermalPolygonArea2(clip) < 0 {
		orientation = -1
	}
	for index, edgeStart := range clip {
		edgeEnd := clip[(index+1)%len(clip)]
		input := output
		output = nil
		if len(input) == 0 {
			break
		}
		previous := input[len(input)-1]
		previousInside := thermalInsideClip(previous, edgeStart, edgeEnd, orientation, tolerance)
		for _, current := range input {
			currentInside := thermalInsideClip(current, edgeStart, edgeEnd, orientation, tolerance)
			if currentInside != previousInside {
				if intersection, ok := thermalLineIntersection(previous, current, edgeStart, edgeEnd); ok {
					output = append(output, intersection)
				}
			}
			if currentInside {
				output = append(output, current)
			}
			previous = current
			previousInside = currentInside
		}
	}
	return output
}

func thermalInsideClip(point thermalPoint2, edgeStart thermalPoint2, edgeEnd thermalPoint2, orientation float64, tolerance float64) bool {
	cross := (edgeEnd.x-edgeStart.x)*(point.y-edgeStart.y) - (edgeEnd.y-edgeStart.y)*(point.x-edgeStart.x)
	return orientation*cross >= -tolerance
}

func thermalLineIntersection(a thermalPoint2, b thermalPoint2, c thermalPoint2, d thermalPoint2) (thermalPoint2, bool) {
	dxAB, dyAB := b.x-a.x, b.y-a.y
	dxCD, dyCD := d.x-c.x, d.y-c.y
	denominator := dxAB*dyCD - dyAB*dxCD
	if math.Abs(denominator) <= 1e-12 {
		return thermalPoint2{}, false
	}
	t := ((c.x-a.x)*dyCD - (c.y-a.y)*dxCD) / denominator
	return thermalPoint2{x: a.x + t*dxAB, y: a.y + t*dyAB}, true
}

func (builder *thermalTopologyBuilder) buildAdjacencyObservations(descriptors map[string]SurfaceGeometryDescriptor, tolerance float64) {
	bucketSize := thermalAdjacencyBucketSize(builder.geometry.Bounds)
	buckets := map[string][]string{}
	for surfaceID, descriptor := range descriptors {
		surface := builder.surfaceByID[surfaceID]
		for _, key := range thermalAdjacencyBucketKeys(surface, descriptor, tolerance, bucketSize) {
			buckets[key] = append(buckets[key], surfaceID)
		}
	}
	seenPairs := map[string]bool{}
	for _, observation := range builder.report.AdjacencyObservations {
		seenPairs[thermalObservationPairKey(observation.SurfaceAID, observation.SurfaceBID)] = true
	}
	for _, surfaceIDs := range buckets {
		for left := 0; left < len(surfaceIDs); left++ {
			for right := left + 1; right < len(surfaceIDs); right++ {
				pairKey := thermalObservationPairKey(surfaceIDs[left], surfaceIDs[right])
				if seenPairs[pairKey] {
					continue
				}
				seenPairs[pairKey] = true
				surfaceA, surfaceB := builder.surfaceByID[surfaceIDs[left]], builder.surfaceByID[surfaceIDs[right]]
				if strings.EqualFold(surfaceA.ZoneName, surfaceB.ZoneName) {
					continue
				}
				descriptorA, descriptorB := descriptors[surfaceA.ID], descriptors[surfaceB.ID]
				if dotGeometryPoints(descriptorA.PlaneNormal, descriptorB.PlaneNormal) > -0.99 {
					continue
				}
				planeDistance := math.Abs(dotGeometryPoints(descriptorA.PlaneNormal, descriptorB.Centroid) - descriptorA.PlaneOffset)
				if planeDistance > tolerance*10 {
					continue
				}
				overlap := thermalPolygonOverlapRatio(surfaceA.WorldVertices, surfaceB.WorldVertices, descriptorA.PlaneNormal, tolerance)
				if overlap < 0.95 {
					continue
				}
				builder.report.AdjacencyObservations = append(builder.report.AdjacencyObservations, GeometricAdjacencyObservation{
					SurfaceAID:         surfaceA.ID,
					SurfaceBID:         surfaceB.ID,
					OverlapRatio:       roundedNumber(overlap, 4),
					DeclaredConnection: false,
					ObservationKind:    "geometrically_adjacent_but_thermally_disconnected",
				})
			}
		}
	}
	sort.SliceStable(builder.report.AdjacencyObservations, func(i, j int) bool {
		left := thermalObservationPairKey(builder.report.AdjacencyObservations[i].SurfaceAID, builder.report.AdjacencyObservations[i].SurfaceBID)
		right := thermalObservationPairKey(builder.report.AdjacencyObservations[j].SurfaceAID, builder.report.AdjacencyObservations[j].SurfaceBID)
		return left < right
	})
}

func thermalAdjacencyBucketSize(bounds GeometryBounds) float64 {
	if !bounds.OK {
		return 1
	}
	diagonal := math.Sqrt(math.Pow(bounds.MaxX-bounds.MinX, 2) + math.Pow(bounds.MaxY-bounds.MinY, 2) + math.Pow(bounds.MaxZ-bounds.MinZ, 2))
	return math.Max(0.5, diagonal/32)
}

func thermalAdjacencyBucketKeys(surface GeometrySurface, descriptor SurfaceGeometryDescriptor, tolerance float64, bucketSize float64) []string {
	normal := descriptor.PlaneNormal
	offset := descriptor.PlaneOffset
	if normal.X < -1e-9 || (math.Abs(normal.X) <= 1e-9 && normal.Y < -1e-9) || (math.Abs(normal.X) <= 1e-9 && math.Abs(normal.Y) <= 1e-9 && normal.Z < 0) {
		normal.X *= -1
		normal.Y *= -1
		normal.Z *= -1
		offset *= -1
	}
	axis := dominantGeometryAxis(normal)
	minU, maxU, minV, maxV := thermalProjectedBounds(descriptor.Bounds, axis)
	minCellU, maxCellU := int(math.Floor(minU/bucketSize)), int(math.Floor(maxU/bucketSize))
	minCellV, maxCellV := int(math.Floor(minV/bucketSize)), int(math.Floor(maxV/bucketSize))
	prefix := fmt.Sprintf("%d|%d,%d,%d|%d|%d", surface.StoryIndex, thermalQuantize(normal.X, 0.01), thermalQuantize(normal.Y, 0.01), thermalQuantize(normal.Z, 0.01), thermalQuantize(offset, tolerance*10), axis)
	keys := make([]string, 0, (maxCellU-minCellU+1)*(maxCellV-minCellV+1))
	for u := minCellU; u <= maxCellU; u++ {
		for v := minCellV; v <= maxCellV; v++ {
			keys = append(keys, fmt.Sprintf("%s|%d|%d", prefix, u, v))
		}
	}
	return keys
}

func thermalProjectedBounds(bounds GeometryBounds, dropAxis int) (float64, float64, float64, float64) {
	switch dropAxis {
	case 0:
		return bounds.MinY, bounds.MaxY, bounds.MinZ, bounds.MaxZ
	case 1:
		return bounds.MinX, bounds.MaxX, bounds.MinZ, bounds.MaxZ
	default:
		return bounds.MinX, bounds.MaxX, bounds.MinY, bounds.MaxY
	}
}

func thermalObservationPairKey(surfaceAID string, surfaceBID string) string {
	values := []string{surfaceAID, surfaceBID}
	sort.Strings(values)
	return strings.Join(values, "|")
}

func (builder *thermalTopologyBuilder) buildZoneEnclosures(tolerance float64) {
	for _, zone := range builder.geometry.Zones {
		edgeCount := map[string]int{}
		edgeDetails := map[string]ThermalOpenEdge{}
		var zoneSurfaces []GeometrySurface
		for _, surface := range builder.geometry.Surfaces {
			if surface.IsShading || !strings.EqualFold(surface.ZoneName, zone.Name) {
				continue
			}
			zoneSurfaces = append(zoneSurfaces, surface)
			for index, start := range surface.WorldVertices {
				end := surface.WorldVertices[(index+1)%len(surface.WorldVertices)]
				signature := thermalEdgeSignature(start, end, tolerance)
				edgeCount[signature]++
				edgeDetails[signature] = ThermalOpenEdge{SurfaceID: surface.ID, Start: start, End: end}
			}
		}
		enclosure := ZoneEnclosureIntegrity{
			ZoneID:            builder.zoneNodeIDByName[normalizeName(zone.Name)],
			ZoneName:          zone.Name,
			DeclaredVolume:    zone.DeclaredVolume,
			HasDeclaredVolume: zone.HasDeclaredVolume,
		}
		for signature, count := range edgeCount {
			switch {
			case count == 1:
				enclosure.OpenEdgeCount++
				enclosure.OpenEdges = append(enclosure.OpenEdges, edgeDetails[signature])
			case count > 2:
				enclosure.NonManifoldEdgeCount++
			}
		}
		enclosure.ClosedShell = len(zoneSurfaces) >= 4 && enclosure.OpenEdgeCount == 0 && enclosure.NonManifoldEdgeCount == 0
		if enclosure.ClosedShell {
			enclosure.ComputedVolume = roundedNumber(thermalEnclosedVolume(zoneSurfaces), 3)
			if enclosure.HasDeclaredVolume && enclosure.DeclaredVolume > 0 {
				enclosure.VolumeDifferencePct = roundedNumber(relativeDifference(enclosure.ComputedVolume, enclosure.DeclaredVolume)*100, 2)
			}
		}
		sort.SliceStable(enclosure.OpenEdges, func(i, j int) bool {
			return thermalEdgeSignature(enclosure.OpenEdges[i].Start, enclosure.OpenEdges[i].End, tolerance) < thermalEdgeSignature(enclosure.OpenEdges[j].Start, enclosure.OpenEdges[j].End, tolerance)
		})
		if enclosure.OpenEdgeCount > 0 {
			builder.addZoneEnclosureIssue(&enclosure, "zone_shell_open", fmt.Sprintf("Zone %q shell has %d open edges (tolerance %.8g m).", zone.Name, enclosure.OpenEdgeCount, tolerance))
		}
		if enclosure.NonManifoldEdgeCount > 0 {
			builder.addZoneEnclosureIssue(&enclosure, "zone_shell_non_manifold", fmt.Sprintf("Zone %q shell has %d non-manifold edges (tolerance %.8g m).", zone.Name, enclosure.NonManifoldEdgeCount, tolerance))
		}
		if enclosure.VolumeDifferencePct > 10 {
			builder.addZoneEnclosureIssue(&enclosure, "zone_volume_mismatch", fmt.Sprintf("Zone %q computed and declared volume differ by %.2f%%.", zone.Name, enclosure.VolumeDifferencePct))
		}
		builder.report.ZoneEnclosures = append(builder.report.ZoneEnclosures, enclosure)
	}
	sort.SliceStable(builder.report.ZoneEnclosures, func(i, j int) bool {
		return builder.report.ZoneEnclosures[i].ZoneID < builder.report.ZoneEnclosures[j].ZoneID
	})
}

func thermalEnclosedVolume(surfaces []GeometrySurface) float64 {
	var allPoints []GeometryPoint
	for _, surface := range surfaces {
		allPoints = append(allPoints, surface.WorldVertices...)
	}
	center := thermalCentroid(allPoints)
	volume := 0.0
	for _, surface := range surfaces {
		if len(surface.WorldVertices) < 3 {
			continue
		}
		origin := subtractGeometryPoint(surface.WorldVertices[0], center)
		for index := 1; index+1 < len(surface.WorldVertices); index++ {
			b := subtractGeometryPoint(surface.WorldVertices[index], center)
			c := subtractGeometryPoint(surface.WorldVertices[index+1], center)
			volume += math.Abs(dotGeometryPoints(origin, crossGeometryPoints(b, c))) / 6
		}
	}
	return volume
}

func subtractGeometryPoint(a GeometryPoint, b GeometryPoint) GeometryPoint {
	return GeometryPoint{X: a.X - b.X, Y: a.Y - b.Y, Z: a.Z - b.Z}
}

func crossGeometryPoints(a GeometryPoint, b GeometryPoint) GeometryPoint {
	return GeometryPoint{X: a.Y*b.Z - a.Z*b.Y, Y: a.Z*b.X - a.X*b.Z, Z: a.X*b.Y - a.Y*b.X}
}

func (builder *thermalTopologyBuilder) addZoneEnclosureIssue(enclosure *ZoneEnclosureIntegrity, code string, message string) {
	issueID := "topology-issue:" + semanticStableHash(strings.Join([]string{code, enclosure.ZoneID, message}, "\x00"), 20)
	enclosure.DiagnosticIDs = appendUniqueString(enclosure.DiagnosticIDs, issueID)
	anchors := []SemanticSourceAnchor{}
	if nodeIndex, ok := builder.nodeIndexByID[enclosure.ZoneID]; ok {
		anchors = append(anchors, builder.report.Nodes[nodeIndex].SourceAnchors...)
		builder.report.Nodes[nodeIndex].DiagnosticIDs = appendUniqueString(builder.report.Nodes[nodeIndex].DiagnosticIDs, issueID)
	}
	builder.report.IssueLinks = append(builder.report.IssueLinks, ThermalTopologyIssueLink{
		ID:            issueID,
		Code:          code,
		Severity:      "warning",
		Message:       message,
		EntityID:      enclosure.ZoneID,
		SourceAnchors: anchors,
	})
}
