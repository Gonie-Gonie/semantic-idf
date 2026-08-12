package idf

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

type geometryPolygonKind string

const (
	geometryPolygonDetailed    geometryPolygonKind = "detailed"
	geometryPolygonRectangular geometryPolygonKind = "rectangular"
)

type heatTransferSurfaceFamily struct {
	ObjectType        string
	PolygonKind       geometryPolygonKind
	SurfaceType       string
	BoundaryCondition string
	SecondDimension   string
}

type geometryShadingFamily struct {
	ObjectType  string
	PolygonKind geometryPolygonKind
	Scope       string
}

var heatTransferSurfaceFamilies = map[string]heatTransferSurfaceFamily{
	"buildingsurface:detailed": {ObjectType: "BuildingSurface:Detailed", PolygonKind: geometryPolygonDetailed},
	"wall:detailed":            {ObjectType: "Wall:Detailed", PolygonKind: geometryPolygonDetailed, SurfaceType: "Wall"},
	"roofceiling:detailed":     {ObjectType: "RoofCeiling:Detailed", PolygonKind: geometryPolygonDetailed, SurfaceType: "RoofCeiling"},
	"floor:detailed":           {ObjectType: "Floor:Detailed", PolygonKind: geometryPolygonDetailed, SurfaceType: "Floor"},
	"wall:exterior":            {ObjectType: "Wall:Exterior", PolygonKind: geometryPolygonRectangular, SurfaceType: "Wall", BoundaryCondition: "Outdoors", SecondDimension: "Height"},
	"wall:adiabatic":           {ObjectType: "Wall:Adiabatic", PolygonKind: geometryPolygonRectangular, SurfaceType: "Wall", BoundaryCondition: "Adiabatic", SecondDimension: "Height"},
	"wall:underground":         {ObjectType: "Wall:Underground", PolygonKind: geometryPolygonRectangular, SurfaceType: "Wall", BoundaryCondition: "Ground", SecondDimension: "Height"},
	"wall:interzone":           {ObjectType: "Wall:Interzone", PolygonKind: geometryPolygonRectangular, SurfaceType: "Wall", BoundaryCondition: "Surface", SecondDimension: "Height"},
	"roof":                     {ObjectType: "Roof", PolygonKind: geometryPolygonRectangular, SurfaceType: "Roof", BoundaryCondition: "Outdoors", SecondDimension: "Width"},
	"ceiling:adiabatic":        {ObjectType: "Ceiling:Adiabatic", PolygonKind: geometryPolygonRectangular, SurfaceType: "Ceiling", BoundaryCondition: "Adiabatic", SecondDimension: "Width"},
	"ceiling:interzone":        {ObjectType: "Ceiling:Interzone", PolygonKind: geometryPolygonRectangular, SurfaceType: "Ceiling", BoundaryCondition: "Surface", SecondDimension: "Width"},
	"floor:groundcontact":      {ObjectType: "Floor:GroundContact", PolygonKind: geometryPolygonRectangular, SurfaceType: "Floor", BoundaryCondition: "Ground", SecondDimension: "Width"},
	"floor:adiabatic":          {ObjectType: "Floor:Adiabatic", PolygonKind: geometryPolygonRectangular, SurfaceType: "Floor", BoundaryCondition: "Adiabatic", SecondDimension: "Width"},
	"floor:interzone":          {ObjectType: "Floor:Interzone", PolygonKind: geometryPolygonRectangular, SurfaceType: "Floor", BoundaryCondition: "Surface", SecondDimension: "Width"},
}

var geometryShadingFamilies = map[string]geometryShadingFamily{
	"shading:site":              {ObjectType: "Shading:Site", PolygonKind: geometryPolygonRectangular, Scope: "site"},
	"shading:building":          {ObjectType: "Shading:Building", PolygonKind: geometryPolygonRectangular, Scope: "building"},
	"shading:site:detailed":     {ObjectType: "Shading:Site:Detailed", PolygonKind: geometryPolygonDetailed, Scope: "site"},
	"shading:building:detailed": {ObjectType: "Shading:Building:Detailed", PolygonKind: geometryPolygonDetailed, Scope: "building"},
	"shading:zone:detailed":     {ObjectType: "Shading:Zone:Detailed", PolygonKind: geometryPolygonDetailed, Scope: "zone"},
}

func heatTransferSurfaceFamilyFor(objectType string) (heatTransferSurfaceFamily, bool) {
	family, ok := heatTransferSurfaceFamilies[strings.ToLower(strings.TrimSpace(objectType))]
	return family, ok
}

func heatTransferSurfaceObjectTypes() []string {
	types := make([]string, 0, len(heatTransferSurfaceFamilies))
	for _, family := range heatTransferSurfaceFamilies {
		types = append(types, family.ObjectType)
	}
	sort.Strings(types)
	return types
}

func geometryShadingFamilyFor(objectType string) (geometryShadingFamily, bool) {
	family, ok := geometryShadingFamilies[strings.ToLower(strings.TrimSpace(objectType))]
	return family, ok
}

func isGeometryShadingType(objectType string) bool {
	_, ok := geometryShadingFamilyFor(objectType)
	return ok
}

func heatTransferSurfaceVertices(obj Object, family heatTransferSurfaceFamily, ctx geometryContext) ([]point3, []point3, string, bool) {
	if family.PolygonKind == geometryPolygonDetailed {
		rawVertices, ok := detailedVertices(obj)
		if !ok {
			return nil, nil, "", false
		}
		zoneName := semanticGeometryFieldValue(obj, 3, "Zone Name")
		spaceName := semanticGeometrySpaceName(obj)
		if zoneName == "" && spaceName != "" {
			zoneName = ctx.spaceZones[normalizeName(spaceName)]
		}
		worldVertices := geometryWorldVerticesForCoordinateSystem(rawVertices, zoneName, ctx.coordinateSystem, ctx)
		return rawVertices, worldVertices, "computed_geometry", true
	}

	rawVertices, ok := rectangularSurfaceVertices(obj, family)
	if !ok {
		return nil, nil, "", false
	}
	zoneName := semanticGeometryFieldValue(obj, 2, "Zone Name")
	spaceName := semanticGeometrySpaceName(obj)
	if zoneName == "" && spaceName != "" {
		zoneName = ctx.spaceZones[normalizeName(spaceName)]
	}
	worldVertices := geometryWorldVerticesForCoordinateSystem(rawVertices, zoneName, ctx.rectangularCoordinateSystem, ctx)
	return rawVertices, worldVertices, "generated_rectangular_geometry", true
}

func rectangularSurfaceVertices(obj Object, family heatTransferSurfaceFamily) ([]point3, bool) {
	startX, okX := geometryNumericField(obj, "Starting X Coordinate")
	startY, okY := geometryNumericField(obj, "Starting Y Coordinate")
	startZ, okZ := geometryNumericField(obj, "Starting Z Coordinate")
	length, okLength := geometryNumericField(obj, "Length")
	secondDimension, okSecond := geometryNumericField(obj, family.SecondDimension)
	if !okX || !okY || !okZ || !okLength || !okSecond || length <= 0 || secondDimension <= 0 {
		return nil, false
	}
	azimuth, _ := geometryNumericField(obj, "Azimuth Angle")
	tilt, okTilt := geometryNumericField(obj, "Tilt Angle")
	if !okTilt {
		tilt = defaultRectangularSurfaceTilt(family.SurfaceType)
	}
	lengthAxis, secondAxis := rectangularSurfaceBasis(azimuth, tilt)
	origin := point3{x: startX, y: startY, z: startZ}
	secondCorner := addScaledGeometryPoint(origin, secondAxis, secondDimension)
	oppositeCorner := addScaledGeometryPoint(secondCorner, lengthAxis, length)
	lengthCorner := addScaledGeometryPoint(origin, lengthAxis, length)
	return []point3{origin, secondCorner, oppositeCorner, lengthCorner}, true
}

func rectangularSurfaceBasis(azimuth float64, tilt float64) (point3, point3) {
	azimuthRadians := azimuth * math.Pi / 180
	tiltRadians := tilt * math.Pi / 180
	lengthAxis := point3{x: math.Cos(azimuthRadians), y: -math.Sin(azimuthRadians)}
	secondAxis := point3{
		x: -math.Cos(tiltRadians) * math.Sin(azimuthRadians),
		y: -math.Cos(tiltRadians) * math.Cos(azimuthRadians),
		z: math.Sin(tiltRadians),
	}
	return lengthAxis, secondAxis
}

func defaultRectangularSurfaceTilt(surfaceType string) float64 {
	switch strings.ToLower(surfaceType) {
	case "floor":
		return 180
	case "roof", "roofceiling", "ceiling":
		return 0
	default:
		return 90
	}
}

func addScaledGeometryPoint(origin point3, direction point3, scale float64) point3 {
	return point3{
		x: origin.x + direction.x*scale,
		y: origin.y + direction.y*scale,
		z: origin.z + direction.z*scale,
	}
}

func geometryNumericField(obj Object, fieldName string) (float64, bool) {
	if value, ok := parseFloatField(fieldValueByCatalogName(obj, fieldName)); ok {
		return value, true
	}
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(fieldName, "-", " ")))
	return parseFloatField(findFieldByCommentWords(obj, words...))
}

func geometryShadingSurfaceFromObject(obj Object, ctx geometryContext, heatTransferSurfaces map[string]GeometrySurface) (GeometrySurface, bool) {
	family, supported := geometryShadingFamilyFor(obj.Type)
	if !supported {
		return GeometrySurface{}, false
	}

	var rawVertices []point3
	var ok bool
	verticesSource := "computed_geometry"
	if family.PolygonKind == geometryPolygonDetailed {
		rawVertices, ok = detailedVertices(obj)
	} else {
		rawVertices, ok = rectangularSurfaceVertices(obj, heatTransferSurfaceFamily{SurfaceType: "Shading", SecondDimension: "Height"})
		verticesSource = "generated_rectangular_geometry"
	}
	if !ok {
		return GeometrySurface{}, false
	}

	baseSurfaceName := geometryStringField(obj, "Base Surface Name")
	baseSurface := heatTransferSurfaces[normalizeName(baseSurfaceName)]
	zoneName := baseSurface.ZoneName
	coordinateSystem := ctx.coordinateSystem
	if family.PolygonKind == geometryPolygonRectangular {
		coordinateSystem = ctx.rectangularCoordinateSystem
	}
	worldVertices := geometryShadingWorldVertices(rawVertices, family.Scope, zoneName, coordinateSystem, ctx)
	physicalArea, ok := polygonArea(worldVertices)
	if !ok {
		return GeometrySurface{}, false
	}
	minZ, maxZ, _ := verticesZStats(worldVertices)
	azimuth, hasAzimuth := geometryAzimuthForCoordinateSystem(obj, worldVertices, zoneName, coordinateSystem, ctx)
	orientation := ""
	if hasAzimuth {
		orientation = orientationFromAzimuth(azimuth)
	}
	rawPoints := geometryPoints(rawVertices)
	worldPoints := geometryPoints(worldVertices)
	surface := GeometrySurface{
		ID:                "shading-" + strconv.Itoa(obj.Index),
		ObjectIndex:       obj.Index,
		Name:              objectName(obj),
		Type:              obj.Type,
		SurfaceType:       "Shading",
		ZoneName:          zoneName,
		StoryIndex:        -1,
		Area:              roundedNumber(physicalArea, 3),
		PhysicalArea:      roundedNumber(physicalArea, 3),
		EffectiveArea:     roundedNumber(physicalArea, 3),
		ZoneMultiplier:    1,
		SurfaceMultiplier: 1,
		AreaBasis:         "physical",
		IsShading:         true,
		Azimuth:           roundedNumber(azimuth, 2),
		Orientation:       orientation,
		MinZ:              roundedNumber(minZ, 3),
		MaxZ:              roundedNumber(maxZ, 3),
		RawVertices:       rawPoints,
		WorldVertices:     worldPoints,
		Vertices:          append([]GeometryPoint(nil), worldPoints...),
		VerticesSource:    verticesSource,
		Fields:            append([]Field(nil), obj.Fields...),
	}
	surface.Metrics = geometryAreaMetrics(surface.PhysicalArea, surface.EffectiveArea, 1, 1)
	return surface, true
}

func geometryShadingWorldVertices(rawVertices []point3, scope string, zoneName string, coordinateSystem string, ctx geometryContext) []point3 {
	if isWorldGeometryCoordinateSystem(coordinateSystem) || strings.EqualFold(scope, "site") {
		return append([]point3(nil), rawVertices...)
	}
	if strings.EqualFold(scope, "zone") {
		return geometryWorldVerticesForCoordinateSystem(rawVertices, zoneName, coordinateSystem, ctx)
	}
	worldVertices := make([]point3, len(rawVertices))
	for index, rawVertex := range rawVertices {
		worldVertices[index] = rotateGeometryPoint(rawVertex, ctx.buildingNorthAxis)
	}
	return worldVertices
}

func geometryStringField(obj Object, fieldName string) string {
	if value := fieldValueByCatalogName(obj, fieldName); value != "" {
		return strings.TrimSpace(value)
	}
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(fieldName, "-", " ")))
	return strings.TrimSpace(findFieldByCommentWords(obj, words...))
}

func heatTransferOpeningVertices(obj Object, base GeometrySurface, ctx geometryContext) ([]point3, []point3, string, bool) {
	if strings.EqualFold(obj.Type, "FenestrationSurface:Detailed") {
		rawVertices, ok := detailedVertices(obj)
		if !ok {
			return nil, nil, "", false
		}
		worldVertices := geometryWorldVerticesForCoordinateSystem(rawVertices, base.ZoneName, ctx.coordinateSystem, ctx)
		return rawVertices, worldVertices, "computed_geometry", true
	}

	startX, okX := geometryNumericField(obj, "Starting X Coordinate")
	startZ, okZ := geometryNumericField(obj, "Starting Z Coordinate")
	length, okLength := geometryNumericField(obj, "Length")
	height, okHeight := geometryNumericField(obj, "Height")
	if !okX || !okZ || !okLength || !okHeight || length <= 0 || height <= 0 || len(base.WorldVertices) < 3 {
		return nil, nil, "", false
	}

	worldBaseVertices := geometryPoint3s(base.WorldVertices)
	tilt := geometryPolygonTilt(worldBaseVertices, ctx.vertexEntryDirection)
	lengthAxis, heightAxis := rectangularSurfaceBasis(base.Azimuth, tilt)
	origin := geometryLowerLeftVertex(worldBaseVertices, lengthAxis, heightAxis)
	worldLowerLeft := addScaledGeometryPoint(addScaledGeometryPoint(origin, lengthAxis, startX), heightAxis, startZ)
	worldUpperLeft := addScaledGeometryPoint(worldLowerLeft, heightAxis, height)
	worldUpperRight := addScaledGeometryPoint(worldUpperLeft, lengthAxis, length)
	worldLowerRight := addScaledGeometryPoint(worldLowerLeft, lengthAxis, length)
	rawVertices := []point3{
		{x: startX, z: startZ},
		{x: startX, z: startZ + height},
		{x: startX + length, z: startZ + height},
		{x: startX + length, z: startZ},
	}
	return rawVertices, []point3{worldLowerLeft, worldUpperLeft, worldUpperRight, worldLowerRight}, "generated_rectangular_opening", true
}

func geometryPoint3s(points []GeometryPoint) []point3 {
	vertices := make([]point3, 0, len(points))
	for _, point := range points {
		vertices = append(vertices, point3{x: point.X, y: point.Y, z: point.Z})
	}
	return vertices
}

func geometryPolygonTilt(vertices []point3, vertexEntryDirection string) float64 {
	normal, ok := polygonNormal(vertices)
	if !ok {
		return 90
	}
	if strings.EqualFold(vertexEntryDirection, "clockwise") {
		normal.x *= -1
		normal.y *= -1
		normal.z *= -1
	}
	magnitude := math.Sqrt(normal.x*normal.x + normal.y*normal.y + normal.z*normal.z)
	if magnitude <= 1e-12 {
		return 90
	}
	return math.Acos(math.Max(-1, math.Min(1, normal.z/magnitude))) * 180 / math.Pi
}

func geometryLowerLeftVertex(vertices []point3, lengthAxis point3, heightAxis point3) point3 {
	if len(vertices) == 0 {
		return point3{}
	}
	best := vertices[0]
	bestScore := math.Inf(1)
	for _, vertex := range vertices {
		score := geometryDot(vertex, lengthAxis) + geometryDot(vertex, heightAxis)
		if score < bestScore {
			best = vertex
			bestScore = score
		}
	}
	return best
}

func geometryDot(left point3, right point3) float64 {
	return left.x*right.x + left.y*right.y + left.z*right.z
}
