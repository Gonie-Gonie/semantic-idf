package idf

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

type GeometryReport struct {
	Zones                       []GeometryZone         `json:"zones"`
	Spaces                      []GeometrySpace        `json:"spaces,omitempty"`
	Surfaces                    []GeometrySurface      `json:"surfaces"`
	Windows                     []GeometryWindow       `json:"windows"`
	Constructions               []GeometryConstruction `json:"constructions,omitempty"`
	Stories                     []GeometryStory        `json:"stories"`
	Bounds                      GeometryBounds         `json:"bounds"`
	CoordinateSystem            string                 `json:"coordinateSystem,omitempty"`
	RectangularCoordinateSystem string                 `json:"rectangularCoordinateSystem,omitempty"`
	VertexEntryDirection        string                 `json:"vertexEntryDirection,omitempty"`
	StartingVertexPosition      string                 `json:"startingVertexPosition,omitempty"`
	ZoneCount                   int                    `json:"zoneCount"`
	SurfaceCount                int                    `json:"surfaceCount"`
	WindowCount                 int                    `json:"windowCount"`
	Topology                    ThermalTopologyReport  `json:"topology"`
}

type GeometrySpace struct {
	ID          string `json:"id"`
	ObjectIndex int    `json:"objectIndex"`
	Name        string `json:"name"`
	ZoneName    string `json:"zoneName"`
}

type GeometryZone struct {
	ID                string           `json:"id"`
	ObjectIndex       int              `json:"objectIndex"`
	Name              string           `json:"name"`
	StoryIndex        int              `json:"storyIndex"`
	FloorArea         float64          `json:"floorArea"`
	Volume            float64          `json:"volume"`
	DeclaredVolume    float64          `json:"declaredVolume,omitempty"`
	HasDeclaredVolume bool             `json:"hasDeclaredVolume"`
	MinZ              float64          `json:"minZ"`
	MaxZ              float64          `json:"maxZ"`
	SurfaceIDs        []string         `json:"surfaceIds"`
	WindowIDs         []string         `json:"windowIds"`
	Metrics           []GeometryMetric `json:"metrics"`
	Fields            []Field          `json:"fields"`
}

type GeometrySurface struct {
	ID                string           `json:"id"`
	ObjectIndex       int              `json:"objectIndex"`
	Name              string           `json:"name"`
	Type              string           `json:"type"`
	SurfaceType       string           `json:"surfaceType"`
	ZoneName          string           `json:"zoneName"`
	SpaceName         string           `json:"spaceName,omitempty"`
	Construction      string           `json:"construction"`
	OutsideBoundary   string           `json:"outsideBoundary"`
	StoryIndex        int              `json:"storyIndex"`
	Area              float64          `json:"area"`
	PhysicalArea      float64          `json:"physicalArea"`
	EffectiveArea     float64          `json:"effectiveArea"`
	ZoneMultiplier    float64          `json:"zoneMultiplier,omitempty"`
	SurfaceMultiplier float64          `json:"surfaceMultiplier,omitempty"`
	AreaBasis         string           `json:"areaBasis"`
	IsShading         bool             `json:"isShading,omitempty"`
	Azimuth           float64          `json:"azimuth"`
	Orientation       string           `json:"orientation"`
	MinZ              float64          `json:"minZ"`
	MaxZ              float64          `json:"maxZ"`
	RawVertices       []GeometryPoint  `json:"rawVertices,omitempty"`
	WorldVertices     []GeometryPoint  `json:"worldVertices"`
	Vertices          []GeometryPoint  `json:"vertices"`
	VerticesSource    string           `json:"verticesSource"`
	Metrics           []GeometryMetric `json:"metrics"`
	Fields            []Field          `json:"fields"`
}

type GeometryWindow struct {
	ID                     string           `json:"id"`
	ObjectIndex            int              `json:"objectIndex"`
	Name                   string           `json:"name"`
	Type                   string           `json:"type"`
	SurfaceType            string           `json:"surfaceType"`
	Construction           string           `json:"construction,omitempty"`
	BaseSurfaceID          string           `json:"baseSurfaceId,omitempty"`
	BaseSurfaceName        string           `json:"baseSurfaceName"`
	ZoneName               string           `json:"zoneName,omitempty"`
	StoryIndex             int              `json:"storyIndex"`
	Area                   float64          `json:"area"`
	PhysicalArea           float64          `json:"physicalArea"`
	EffectiveArea          float64          `json:"effectiveArea"`
	ZoneMultiplier         float64          `json:"zoneMultiplier,omitempty"`
	SurfaceMultiplier      float64          `json:"surfaceMultiplier,omitempty"`
	AreaBasis              string           `json:"areaBasis"`
	AreaIncludesMultiplier bool             `json:"areaIncludesMultiplier"`
	Multiplier             float64          `json:"multiplier,omitempty"`
	Azimuth                float64          `json:"azimuth"`
	Orientation            string           `json:"orientation"`
	RawVertices            []GeometryPoint  `json:"rawVertices,omitempty"`
	WorldVertices          []GeometryPoint  `json:"worldVertices"`
	Vertices               []GeometryPoint  `json:"vertices"`
	VerticesSource         string           `json:"verticesSource"`
	Metrics                []GeometryMetric `json:"metrics"`
	Fields                 []Field          `json:"fields"`
}

type GeometryStory struct {
	Index      int      `json:"index"`
	Name       string   `json:"name"`
	Elevation  float64  `json:"elevation"`
	ZoneIDs    []string `json:"zoneIds"`
	SurfaceIDs []string `json:"surfaceIds"`
	WindowIDs  []string `json:"windowIds"`
}

type GeometryPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type GeometryBounds struct {
	MinX float64 `json:"minX"`
	MaxX float64 `json:"maxX"`
	MinY float64 `json:"minY"`
	MaxY float64 `json:"maxY"`
	MinZ float64 `json:"minZ"`
	MaxZ float64 `json:"maxZ"`
	OK   bool    `json:"ok"`
}

type GeometryMetric struct {
	Name         string `json:"name"`
	Value        any    `json:"value,omitempty"`
	DisplayValue string `json:"displayValue"`
	Unit         string `json:"unit,omitempty"`
}

type GeometryConstruction struct {
	Name                  string                  `json:"name"`
	ObjectType            string                  `json:"objectType"`
	ObjectIndex           int                     `json:"objectIndex"`
	Kind                  string                  `json:"kind"`
	Layers                []GeometryMaterialLayer `json:"layers"`
	TotalThickness        float64                 `json:"totalThickness,omitempty"`
	HasThickness          bool                    `json:"hasThickness"`
	ThermalResistance     float64                 `json:"thermalResistance,omitempty"`
	UValue                float64                 `json:"uValue,omitempty"`
	ArealHeatCapacity     float64                 `json:"arealHeatCapacity,omitempty"`
	HasThermalPerformance bool                    `json:"hasThermalPerformance"`
	HasArealHeatCapacity  bool                    `json:"hasArealHeatCapacity"`
}

type GeometryMaterialLayer struct {
	Name              string  `json:"name"`
	ObjectType        string  `json:"objectType,omitempty"`
	ObjectIndex       int     `json:"objectIndex,omitempty"`
	Thickness         float64 `json:"thickness,omitempty"`
	HasThickness      bool    `json:"hasThickness"`
	ThermalResistance float64 `json:"thermalResistance,omitempty"`
	UFactor           float64 `json:"uFactor,omitempty"`
	Conductivity      float64 `json:"conductivity,omitempty"`
	Density           float64 `json:"density,omitempty"`
	SpecificHeat      float64 `json:"specificHeat,omitempty"`
	ArealHeatCapacity float64 `json:"arealHeatCapacity,omitempty"`
}

type geometryContext struct {
	buildingNorthAxis           float64
	hasBuildingNorthAxis        bool
	coordinateSystem            string
	rectangularCoordinateSystem string
	startingVertexPosition      string
	vertexEntryDirection        string
	zoneDirections              map[string]float64
	zoneMultipliers             map[string]float64
	zoneOrigins                 map[string]point3
	spaceZones                  map[string]string
}

func AnalyzeGeometry(doc Document) GeometryReport {
	return analyzeGeometryWithIndex(doc, NewDocumentIndex(doc))
}

func analyzeGeometryWithIndex(doc Document, documentIndex *DocumentIndex) GeometryReport {
	ctx := geometryContext{
		coordinateSystem:            "relative",
		rectangularCoordinateSystem: "relative",
		startingVertexPosition:      "upperleftcorner",
		vertexEntryDirection:        "counterclockwise",
		zoneDirections:              map[string]float64{},
		zoneMultipliers:             map[string]float64{},
		zoneOrigins:                 map[string]point3{},
		spaceZones:                  map[string]string{},
	}
	report := GeometryReport{}
	zoneByName := map[string]int{}

	for _, obj := range doc.Objects {
		switch {
		case strings.EqualFold(obj.Type, "Building"):
			valueText := fieldValueByCatalogName(obj, "North Axis")
			if valueText == "" {
				valueText = findFieldByCommentWords(obj, "north", "axis")
			}
			if value, ok := parseFloatField(valueText); ok {
				ctx.buildingNorthAxis = value
				ctx.hasBuildingNorthAxis = true
			}
		case strings.EqualFold(obj.Type, "GlobalGeometryRules"):
			if value := findFieldByCommentWords(obj, "vertex", "entry", "direction"); value != "" {
				ctx.vertexEntryDirection = strings.ToLower(strings.TrimSpace(value))
			} else if len(obj.Fields) > 1 && strings.TrimSpace(obj.Fields[1].Value) != "" {
				ctx.vertexEntryDirection = strings.ToLower(strings.TrimSpace(obj.Fields[1].Value))
			}
			if value := findFieldByCommentWords(obj, "coordinate", "system"); value != "" {
				ctx.coordinateSystem = strings.ToLower(strings.TrimSpace(value))
			} else if len(obj.Fields) > 2 && strings.TrimSpace(obj.Fields[2].Value) != "" {
				ctx.coordinateSystem = strings.ToLower(strings.TrimSpace(obj.Fields[2].Value))
			}
			if value := findFieldByCommentWords(obj, "starting", "vertex", "position"); value != "" {
				ctx.startingVertexPosition = strings.ToLower(strings.TrimSpace(value))
			} else if len(obj.Fields) > 0 && strings.TrimSpace(obj.Fields[0].Value) != "" {
				ctx.startingVertexPosition = strings.ToLower(strings.TrimSpace(obj.Fields[0].Value))
			}
			if value := findFieldByCommentWords(obj, "rectangular", "surface", "coordinate", "system"); value != "" {
				ctx.rectangularCoordinateSystem = strings.ToLower(strings.TrimSpace(value))
			} else if len(obj.Fields) > 4 && strings.TrimSpace(obj.Fields[4].Value) != "" {
				ctx.rectangularCoordinateSystem = strings.ToLower(strings.TrimSpace(obj.Fields[4].Value))
			}
		case strings.EqualFold(obj.Type, "Zone"):
			zone := geometryZoneFromObject(obj)
			zoneByName[normalizeName(zone.Name)] = len(report.Zones)
			report.Zones = append(report.Zones, zone)
			zoneKey := normalizeName(zone.Name)
			ctx.zoneDirections[zoneKey] = geometryNumericFieldOrDefault(obj, 0, "Direction of Relative North")
			ctx.zoneMultipliers[zoneKey] = geometryNumericFieldOrDefault(obj, 1, "Multiplier")
			ctx.zoneOrigins[zoneKey] = point3{
				x: geometryNumericFieldOrDefault(obj, 0, "X Origin"),
				y: geometryNumericFieldOrDefault(obj, 0, "Y Origin"),
				z: geometryNumericFieldOrDefault(obj, 0, "Z Origin"),
			}
		case strings.EqualFold(obj.Type, "Space"):
			space := geometrySpaceFromObject(obj)
			report.Spaces = append(report.Spaces, space)
			ctx.spaceZones[normalizeName(space.Name)] = space.ZoneName
		}
	}

	for _, obj := range doc.Objects {
		if !isBuildingSurfaceType(obj.Type) {
			continue
		}
		surface, ok := geometrySurfaceFromObject(obj, ctx)
		if !ok {
			continue
		}
		report.addBounds(surface.WorldVertices)
		report.Surfaces = append(report.Surfaces, surface)
		if index, ok := zoneByName[normalizeName(surface.ZoneName)]; ok {
			zone := &report.Zones[index]
			zone.SurfaceIDs = append(zone.SurfaceIDs, surface.ID)
			zone.FloorArea += floorAreaContribution(surface)
			updateZoneZ(zone, surface.MinZ, surface.MaxZ)
		}
	}

	surfaceByName := map[string]GeometrySurface{}
	for _, surface := range report.Surfaces {
		if surface.Name != "" {
			surfaceByName[normalizeName(surface.Name)] = surface
		}
	}

	for _, obj := range doc.Objects {
		if !isGeometryShadingType(obj.Type) {
			continue
		}
		shading, ok := geometryShadingSurfaceFromObject(obj, ctx, surfaceByName)
		if !ok {
			continue
		}
		report.addBounds(shading.WorldVertices)
		report.Surfaces = append(report.Surfaces, shading)
	}

	for _, obj := range doc.Objects {
		if !isFenestrationType(obj.Type) {
			continue
		}
		window, ok := geometryWindowFromObject(obj, ctx, surfaceByName)
		if !ok {
			continue
		}
		report.addBounds(window.WorldVertices)
		report.Windows = append(report.Windows, window)
		if index, ok := zoneByName[normalizeName(window.ZoneName)]; ok {
			report.Zones[index].WindowIDs = append(report.Zones[index].WindowIDs, window.ID)
		}
	}

	report.finalizeZones()
	report.assignStories()
	report.Constructions = geometryConstructionsFromDocument(doc)
	report.CoordinateSystem = ctx.coordinateSystem
	report.RectangularCoordinateSystem = ctx.rectangularCoordinateSystem
	report.VertexEntryDirection = ctx.vertexEntryDirection
	report.StartingVertexPosition = ctx.startingVertexPosition
	report.ZoneCount = len(report.Zones)
	report.SurfaceCount = len(report.Surfaces)
	report.WindowCount = len(report.Windows)
	report.Topology = BuildThermalTopology(doc, report, documentIndex)
	return report
}

func geometrySpaceFromObject(obj Object) GeometrySpace {
	name := objectName(obj)
	if name == "" {
		name = obj.Type + " #" + strconv.Itoa(obj.Index)
	}
	zoneName := fieldValueByCatalogName(obj, "Zone Name")
	if zoneName == "" && len(obj.Fields) > 1 {
		zoneName = strings.TrimSpace(obj.Fields[1].Value)
	}
	return GeometrySpace{
		ID:          "space-" + strconv.Itoa(obj.Index),
		ObjectIndex: obj.Index,
		Name:        name,
		ZoneName:    zoneName,
	}
}

func geometryZoneFromObject(obj Object) GeometryZone {
	name := objectName(obj)
	if name == "" {
		name = obj.Type + " #" + strconv.Itoa(obj.Index)
	}
	volumeText := fieldValueByCatalogName(obj, "Volume")
	if volumeText == "" {
		volumeText = findFieldByCommentWords(obj, "volume")
	}
	volume, hasDeclaredVolume := parseFloatField(volumeText)
	if strings.EqualFold(strings.TrimSpace(volumeText), "autocalculate") {
		volume = 0
		hasDeclaredVolume = false
	}
	return GeometryZone{
		ID:                "zone-" + strconv.Itoa(obj.Index),
		ObjectIndex:       obj.Index,
		Name:              name,
		StoryIndex:        -1,
		Volume:            volume,
		DeclaredVolume:    volume,
		HasDeclaredVolume: hasDeclaredVolume,
		MinZ:              math.Inf(1),
		MaxZ:              math.Inf(-1),
		Fields:            append([]Field(nil), obj.Fields...),
	}
}

func geometrySurfaceFromObject(obj Object, ctx geometryContext) (GeometrySurface, bool) {
	family, supported := heatTransferSurfaceFamilyFor(obj.Type)
	if !supported {
		return GeometrySurface{}, false
	}
	rawVertices, worldVertices, verticesSource, ok := heatTransferSurfaceVertices(obj, family, ctx)
	if !ok {
		return GeometrySurface{}, false
	}
	zoneName := semanticGeometryFieldValue(obj, 3, "Zone Name")
	spaceName := semanticGeometrySpaceName(obj)
	if zoneName == "" && spaceName != "" {
		zoneName = ctx.spaceZones[normalizeName(spaceName)]
	}
	polygonPhysicalArea, ok := polygonArea(worldVertices)
	if !ok {
		return GeometrySurface{}, false
	}
	outsideBoundary := semanticGeometryOutsideBoundary(obj)
	if outsideBoundary == "" {
		outsideBoundary = family.BoundaryCondition
	}
	zoneMultiplier := zoneMultiplierFor(ctx.zoneMultipliers, zoneName)
	surfaceMultiplier := geometryNumericFieldOrDefault(obj, 1, "Multiplier")
	physicalArea := polygonPhysicalArea * surfaceMultiplier
	effectiveArea := physicalArea * zoneMultiplier
	rawPoints := geometryPoints(rawVertices)
	worldPoints := geometryPoints(worldVertices)
	minZ, maxZ, _ := verticesZStats(worldVertices)
	surfaceCoordinateSystem := ctx.coordinateSystem
	if family.PolygonKind == geometryPolygonRectangular {
		surfaceCoordinateSystem = ctx.rectangularCoordinateSystem
	}
	azimuth, hasAzimuth := geometryAzimuthForCoordinateSystem(obj, worldVertices, zoneName, surfaceCoordinateSystem, ctx)
	orientation := ""
	if hasAzimuth {
		orientation = orientationFromAzimuth(azimuth)
	}
	surfaceType := family.SurfaceType
	if surfaceType == "" {
		surfaceType = buildingSurfaceType(obj)
	}
	surface := GeometrySurface{
		ID:              "surface-" + strconv.Itoa(obj.Index),
		ObjectIndex:     obj.Index,
		Name:            objectName(obj),
		Type:            obj.Type,
		SurfaceType:     surfaceType,
		ZoneName:        zoneName,
		SpaceName:       spaceName,
		Construction:    geometryStringField(obj, "Construction Name"),
		OutsideBoundary: outsideBoundary,
		StoryIndex:      -1,
		// Area remains an EffectiveArea alias for one compatibility release.
		Area:              roundedNumber(effectiveArea, 3),
		PhysicalArea:      roundedNumber(physicalArea, 3),
		EffectiveArea:     roundedNumber(effectiveArea, 3),
		ZoneMultiplier:    zoneMultiplier,
		SurfaceMultiplier: surfaceMultiplier,
		AreaBasis:         "effective",
		Azimuth:           roundedNumber(azimuth, 2),
		Orientation:       orientation,
		MinZ:              roundedNumber(minZ, 3),
		MaxZ:              roundedNumber(maxZ, 3),
		RawVertices:       rawPoints,
		WorldVertices:     worldPoints,
		// Vertices remains a WorldVertices alias for one compatibility release.
		Vertices:       append([]GeometryPoint(nil), worldPoints...),
		VerticesSource: verticesSource,
		Fields:         append([]Field(nil), obj.Fields...),
	}
	surface.Metrics = geometryAreaMetrics(surface.PhysicalArea, surface.EffectiveArea, surface.ZoneMultiplier, surface.SurfaceMultiplier)
	surface.Metrics = append(surface.Metrics,
		geometryMetric("Azimuth", surface.Azimuth, "deg", 1),
		geometryMetric("Orientation", surface.Orientation, "", 0),
		geometryMetric("Minimum Z", surface.MinZ, "m", 2),
		geometryMetric("Maximum Z", surface.MaxZ, "m", 2),
	)
	return surface, true
}

func geometryWindowFromObject(obj Object, ctx geometryContext, surfaces map[string]GeometrySurface) (GeometryWindow, bool) {
	baseName := geometryStringField(obj, "Building Surface Name")
	if baseName == "" {
		baseName = findFieldByCommentWords(obj, "surface", "name")
	}
	base := surfaces[normalizeName(baseName)]
	if base.ID == "" {
		return GeometryWindow{}, false
	}
	rawVertices, worldVertices, verticesSource, ok := heatTransferOpeningVertices(obj, base, ctx)
	if !ok {
		return GeometryWindow{}, false
	}
	basePhysicalArea, ok := polygonArea(worldVertices)
	if !ok {
		return GeometryWindow{}, false
	}
	openingMultiplier := geometryNumericFieldOrDefault(obj, 1, "Multiplier")
	physicalArea := basePhysicalArea * openingMultiplier
	zoneMultiplier := zoneMultiplierFor(ctx.zoneMultipliers, base.ZoneName)
	effectiveArea := physicalArea * zoneMultiplier
	azimuth, hasAzimuth := geometryAzimuth(obj, worldVertices, base.ZoneName, ctx)
	if !hasAzimuth && base.Orientation != "" {
		azimuth = base.Azimuth
	}
	orientation := ""
	if hasAzimuth || base.Orientation != "" {
		orientation = orientationFromAzimuth(azimuth)
	}
	window := GeometryWindow{
		ID:              "window-" + strconv.Itoa(obj.Index),
		ObjectIndex:     obj.Index,
		Name:            objectName(obj),
		Type:            obj.Type,
		SurfaceType:     fenestrationSurfaceType(obj),
		Construction:    geometryStringField(obj, "Construction Name"),
		BaseSurfaceID:   base.ID,
		BaseSurfaceName: baseName,
		ZoneName:        base.ZoneName,
		StoryIndex:      -1,
		// Area remains an EffectiveArea alias for one compatibility release.
		Area:                   roundedNumber(effectiveArea, 3),
		PhysicalArea:           roundedNumber(physicalArea, 3),
		EffectiveArea:          roundedNumber(effectiveArea, 3),
		ZoneMultiplier:         zoneMultiplier,
		SurfaceMultiplier:      openingMultiplier,
		AreaBasis:              "effective",
		AreaIncludesMultiplier: openingMultiplier != 1,
		Multiplier:             openingMultiplier,
		Azimuth:                roundedNumber(azimuth, 2),
		Orientation:            orientation,
		RawVertices:            geometryPoints(rawVertices),
		WorldVertices:          geometryPoints(worldVertices),
		// Vertices remains a WorldVertices alias for one compatibility release.
		Vertices:       geometryPoints(worldVertices),
		VerticesSource: verticesSource,
		Fields:         append([]Field(nil), obj.Fields...),
	}
	window.Metrics = geometryAreaMetrics(window.PhysicalArea, window.EffectiveArea, window.ZoneMultiplier, window.SurfaceMultiplier)
	window.Metrics = append(window.Metrics,
		geometryMetric("Azimuth", window.Azimuth, "deg", 1),
		geometryMetric("Orientation", window.Orientation, "", 0),
	)
	return window, true
}

func geometryAreaMetrics(physicalArea float64, effectiveArea float64, zoneMultiplier float64, surfaceMultiplier float64) []GeometryMetric {
	metrics := []GeometryMetric{
		geometryMetric("Physical area", physicalArea, "m2", 2),
	}
	if math.Abs(effectiveArea-physicalArea) > 1e-9 {
		metrics = append(metrics, geometryMetric("Model total area", effectiveArea, "m2", 2))
	}
	metrics = append(metrics,
		geometryMetric("Zone multiplier", zoneMultiplier, "", 2),
		geometryMetric("Surface multiplier", surfaceMultiplier, "", 2),
		geometryMetric("Area basis", "Area is a compatibility alias of EffectiveArea", "", 0),
	)
	return metrics
}

func geometryNumericFieldOrDefault(obj Object, fallback float64, fieldName string) float64 {
	if value, ok := parseFloatField(fieldValueByCatalogName(obj, fieldName)); ok {
		return value
	}
	words := strings.Fields(strings.ToLower(strings.ReplaceAll(fieldName, "-", " ")))
	if value, ok := parseFloatField(findFieldByCommentWords(obj, words...)); ok {
		return value
	}
	return fallback
}

func geometryWorldVertices(rawVertices []point3, zoneName string, ctx geometryContext) []point3 {
	return geometryWorldVerticesForCoordinateSystem(rawVertices, zoneName, ctx.coordinateSystem, ctx)
}

func geometryWorldVerticesForCoordinateSystem(rawVertices []point3, zoneName string, coordinateSystem string, ctx geometryContext) []point3 {
	worldVertices := append([]point3(nil), rawVertices...)
	if isWorldGeometryCoordinateSystem(coordinateSystem) {
		return worldVertices
	}

	zoneKey := normalizeName(zoneName)
	zoneOrigin := ctx.zoneOrigins[zoneKey]
	worldZoneOrigin := rotateGeometryPoint(zoneOrigin, ctx.buildingNorthAxis)
	totalRotation := ctx.buildingNorthAxis + ctx.zoneDirections[zoneKey]
	for index, rawVertex := range rawVertices {
		rotatedVertex := rotateGeometryPoint(rawVertex, totalRotation)
		worldVertices[index] = point3{
			x: worldZoneOrigin.x + rotatedVertex.x,
			y: worldZoneOrigin.y + rotatedVertex.y,
			z: worldZoneOrigin.z + rotatedVertex.z,
		}
	}
	return worldVertices
}

func rotateGeometryPoint(point point3, clockwiseDegrees float64) point3 {
	radians := clockwiseDegrees * math.Pi / 180
	cosine := math.Cos(radians)
	sine := math.Sin(radians)
	return point3{
		x: point.x*cosine - point.y*sine,
		y: point.x*sine + point.y*cosine,
		z: point.z,
	}
}

func isWorldGeometryCoordinateSystem(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, " ", "")
	return strings.HasPrefix(normalized, "world")
}

func semanticGeometryFieldValue(obj Object, fallbackIndex int, names ...string) string {
	if value := fieldValueByCatalogName(obj, names...); value != "" {
		return value
	}
	for _, name := range names {
		words := strings.Fields(strings.ToLower(strings.ReplaceAll(name, "-", " ")))
		if value := findFieldByCommentWords(obj, words...); value != "" {
			return value
		}
	}
	if fallbackIndex >= 0 && fallbackIndex < len(obj.Fields) {
		return strings.TrimSpace(obj.Fields[fallbackIndex].Value)
	}
	return ""
}

func semanticGeometrySpaceName(obj Object) string {
	value := fieldValueByCatalogName(obj, "Space Name")
	if value == "" {
		value = findFieldByCommentWords(obj, "space", "name")
	}
	if isSurfaceBoundaryCondition(value) {
		return ""
	}
	return strings.TrimSpace(value)
}

func semanticGeometryOutsideBoundary(obj Object) string {
	if value := findFieldByCommentWords(obj, "outside", "boundary", "condition"); value != "" {
		return value
	}
	if value := fieldValueByCatalogName(obj, "Outside Boundary Condition"); value != "" && isSurfaceBoundaryCondition(value) {
		return value
	}
	for _, index := range []int{4, 5} {
		if index < len(obj.Fields) && isSurfaceBoundaryCondition(obj.Fields[index].Value) {
			return strings.TrimSpace(obj.Fields[index].Value)
		}
	}
	return ""
}

func isSurfaceBoundaryCondition(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "outdoors", "ground", "surface", "adiabatic", "zone", "otherzone", "space", "foundation",
		"groundfcfactormethod", "ground fc factor method", "ground slab preprocessor average", "groundslabpreprocessoraverage",
		"ground slab preprocessor core", "groundslabpreprocessorcore", "ground slab preprocessor perimeter", "groundslabpreprocessorperimeter",
		"ground basement preprocessor average wall", "groundbasementpreprocessoraveragewall",
		"ground basement preprocessor average floor", "groundbasementpreprocessoraveragefloor",
		"ground basement preprocessor upper wall", "groundbasementpreprocessorupperwall",
		"ground basement preprocessor lower wall", "groundbasementpreprocessorlowerwall",
		"other side coefficients", "othersidecoefficients", "other side conditions model", "othersideconditionsmodel":
		return true
	default:
		return false
	}
}

func geometryAzimuth(obj Object, vertices []point3, zoneName string, ctx geometryContext) (float64, bool) {
	return geometryAzimuthForCoordinateSystem(obj, vertices, zoneName, ctx.coordinateSystem, ctx)
}

func geometryAzimuthForCoordinateSystem(obj Object, vertices []point3, zoneName string, coordinateSystem string, ctx geometryContext) (float64, bool) {
	valueText := geometryStringField(obj, "Azimuth Angle")
	if value, ok := parseFloatField(valueText); ok {
		if isWorldGeometryCoordinateSystem(coordinateSystem) {
			return normalizeDegrees(value), true
		}
		return normalizeDegrees(value + geometryRotation(zoneName, ctx)), true
	}
	normal, ok := polygonNormal(vertices)
	if !ok {
		return 0, false
	}
	if strings.EqualFold(ctx.vertexEntryDirection, "clockwise") {
		normal.x *= -1
		normal.y *= -1
		normal.z *= -1
	}
	if math.Hypot(normal.x, normal.y) <= 1e-9 {
		return 0, false
	}
	azimuth := math.Atan2(normal.x, normal.y) * 180 / math.Pi
	return normalizeDegrees(azimuth), true
}

func geometryRotation(zoneName string, ctx geometryContext) float64 {
	rotation := 0.0
	if ctx.hasBuildingNorthAxis {
		rotation += ctx.buildingNorthAxis
	}
	if zoneName != "" {
		rotation += ctx.zoneDirections[normalizeName(zoneName)]
	}
	return rotation
}

func geometryPoints(vertices []point3) []GeometryPoint {
	points := make([]GeometryPoint, 0, len(vertices))
	for _, vertex := range vertices {
		points = append(points, GeometryPoint{
			X: roundedNumber(vertex.x, 4),
			Y: roundedNumber(vertex.y, 4),
			Z: roundedNumber(vertex.z, 4),
		})
	}
	return points
}

func floorAreaContribution(surface GeometrySurface) float64 {
	if strings.EqualFold(surface.SurfaceType, "Floor") {
		return surface.EffectiveArea
	}
	return 0
}

func updateZoneZ(zone *GeometryZone, minZ float64, maxZ float64) {
	if math.IsInf(zone.MinZ, 0) {
		zone.MinZ = minZ
	} else {
		zone.MinZ = math.Min(zone.MinZ, minZ)
	}
	if math.IsInf(zone.MaxZ, 0) {
		zone.MaxZ = maxZ
	} else {
		zone.MaxZ = math.Max(zone.MaxZ, maxZ)
	}
}

func (report *GeometryReport) finalizeZones() {
	for index := range report.Zones {
		zone := &report.Zones[index]
		if math.IsInf(zone.MinZ, 0) {
			zone.MinZ = 0
		}
		if math.IsInf(zone.MaxZ, 0) {
			zone.MaxZ = zone.MinZ
		}
		height := math.Max(0, zone.MaxZ-zone.MinZ)
		if zone.Volume == 0 && zone.FloorArea > 0 && height > 0 {
			zone.Volume = zone.FloorArea * height
		}
		zone.FloorArea = roundedNumber(zone.FloorArea, 3)
		zone.Volume = roundedNumber(zone.Volume, 3)
		zone.MinZ = roundedNumber(zone.MinZ, 3)
		zone.MaxZ = roundedNumber(zone.MaxZ, 3)
		zone.Metrics = []GeometryMetric{
			geometryMetric("Floor area", zone.FloorArea, "m2", 2),
			geometryMetric("Volume", zone.Volume, "m3", 2),
			geometryMetric("Minimum Z", zone.MinZ, "m", 2),
			geometryMetric("Maximum Z", zone.MaxZ, "m", 2),
			geometryMetric("Surface count", len(zone.SurfaceIDs), "", 0),
			geometryMetric("Window count", len(zone.WindowIDs), "", 0),
		}
	}
}

func (report *GeometryReport) assignStories() {
	elevations := report.storyElevations()
	for index, elevation := range elevations {
		report.Stories = append(report.Stories, GeometryStory{
			Index:     index,
			Name:      "Level " + strconv.Itoa(index+1),
			Elevation: roundedNumber(elevation, 3),
		})
	}
	if len(report.Stories) == 0 {
		report.Stories = append(report.Stories, GeometryStory{Index: 0, Name: "Level 1", Elevation: 0})
		elevations = []float64{0}
	}

	zoneStoryByName := map[string]int{}
	for index := range report.Zones {
		storyIndex := nearestStoryIndex(report.Zones[index].MinZ, elevations)
		report.Zones[index].StoryIndex = storyIndex
		zoneStoryByName[normalizeName(report.Zones[index].Name)] = storyIndex
		report.Stories[storyIndex].ZoneIDs = append(report.Stories[storyIndex].ZoneIDs, report.Zones[index].ID)
	}
	for index := range report.Surfaces {
		storyIndex, ok := zoneStoryByName[normalizeName(report.Surfaces[index].ZoneName)]
		if !ok {
			storyIndex = nearestStoryIndex(report.Surfaces[index].MinZ, elevations)
		}
		report.Surfaces[index].StoryIndex = storyIndex
		report.Stories[storyIndex].SurfaceIDs = append(report.Stories[storyIndex].SurfaceIDs, report.Surfaces[index].ID)
	}
	for index := range report.Windows {
		storyIndex, ok := zoneStoryByName[normalizeName(report.Windows[index].ZoneName)]
		if !ok {
			storyIndex = nearestStoryIndex(avgPointZ(report.Windows[index].Vertices), elevations)
		}
		report.Windows[index].StoryIndex = storyIndex
		report.Stories[storyIndex].WindowIDs = append(report.Stories[storyIndex].WindowIDs, report.Windows[index].ID)
	}
}

func (report GeometryReport) storyElevations() []float64 {
	var elevations []float64
	for _, surface := range report.Surfaces {
		if strings.EqualFold(surface.SurfaceType, "Floor") {
			elevations = appendUniqueElevation(elevations, surface.MinZ)
		}
	}
	if len(elevations) == 0 {
		for _, zone := range report.Zones {
			elevations = appendUniqueElevation(elevations, zone.MinZ)
		}
	}
	sort.Float64s(elevations)
	return elevations
}

func appendUniqueElevation(elevations []float64, value float64) []float64 {
	for _, existing := range elevations {
		if math.Abs(existing-value) <= 0.5 {
			return elevations
		}
	}
	return append(elevations, value)
}

func nearestStoryIndex(value float64, elevations []float64) int {
	if len(elevations) == 0 {
		return 0
	}
	bestIndex := 0
	bestDistance := math.Abs(value - elevations[0])
	for index, elevation := range elevations[1:] {
		distance := math.Abs(value - elevation)
		if distance < bestDistance {
			bestIndex = index + 1
			bestDistance = distance
		}
	}
	return bestIndex
}

func avgPointZ(points []GeometryPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	var total float64
	for _, point := range points {
		total += point.Z
	}
	return total / float64(len(points))
}

func (report *GeometryReport) addBounds(points []GeometryPoint) {
	for _, point := range points {
		if !report.Bounds.OK {
			report.Bounds = GeometryBounds{MinX: point.X, MaxX: point.X, MinY: point.Y, MaxY: point.Y, MinZ: point.Z, MaxZ: point.Z, OK: true}
			continue
		}
		report.Bounds.MinX = math.Min(report.Bounds.MinX, point.X)
		report.Bounds.MaxX = math.Max(report.Bounds.MaxX, point.X)
		report.Bounds.MinY = math.Min(report.Bounds.MinY, point.Y)
		report.Bounds.MaxY = math.Max(report.Bounds.MaxY, point.Y)
		report.Bounds.MinZ = math.Min(report.Bounds.MinZ, point.Z)
		report.Bounds.MaxZ = math.Max(report.Bounds.MaxZ, point.Z)
	}
}

func geometryMetric(name string, value any, unit string, precision int) GeometryMetric {
	display := ""
	switch v := value.(type) {
	case float64:
		display = formatSummaryNumber(v, precision)
	case int:
		display = strconv.Itoa(v)
	case string:
		display = v
	default:
		display = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strconv.FormatFloat(toFloat(value), 'f', precision, 64), "0"), "."))
	}
	if display == "" {
		display = "N/A"
	}
	return GeometryMetric{Name: name, Value: value, DisplayValue: display, Unit: unit}
}

func geometryConstructionsFromDocument(doc Document) []GeometryConstruction {
	materials := geometryMaterialsByName(doc)
	var constructions []GeometryConstruction
	for _, obj := range doc.Objects {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(obj.Type)), "construction") {
			continue
		}
		construction := GeometryConstruction{
			Name:        objectName(obj),
			ObjectType:  obj.Type,
			ObjectIndex: obj.Index,
			Kind:        geometryConstructionKind(obj.Type),
		}
		for _, index := range geometryConstructionLayerFieldIndexes(obj) {
			layerName := strings.TrimSpace(obj.Fields[index].Value)
			if layerName == "" {
				continue
			}
			layer, ok := materials[normalizeName(layerName)]
			if !ok {
				layer = GeometryMaterialLayer{Name: layerName, ObjectIndex: -1}
			}
			construction.Layers = append(construction.Layers, layer)
			if layer.HasThickness {
				construction.TotalThickness += layer.Thickness
				construction.HasThickness = true
			}
		}
		if construction.Kind == "layer_based_opaque" {
			for _, layer := range construction.Layers {
				if strings.HasPrefix(strings.ToLower(layer.ObjectType), "windowmaterial:") {
					construction.Kind = "layer_based_window"
					break
				}
			}
		}
		applyDirectGeometryConstructionPerformance(&construction, obj)
		construction.TotalThickness = roundedNumber(construction.TotalThickness, 4)
		finalizeGeometryConstructionPerformance(&construction)
		constructions = append(constructions, construction)
	}
	return constructions
}

func geometryMaterialsByName(doc Document) map[string]GeometryMaterialLayer {
	materials := map[string]GeometryMaterialLayer{}
	for _, obj := range doc.Objects {
		if !isGeometryMaterialType(obj.Type) {
			continue
		}
		name := objectName(obj)
		if name == "" {
			continue
		}
		layer := GeometryMaterialLayer{
			Name:        name,
			ObjectType:  obj.Type,
			ObjectIndex: obj.Index,
		}
		if thickness, ok := geometryNumericField(obj, "Thickness"); ok {
			layer.Thickness = roundedNumber(thickness, 4)
			layer.HasThickness = true
		}
		if resistance, ok := geometryNumericField(obj, "Thermal Resistance"); ok {
			layer.ThermalResistance = roundedNumber(resistance, 4)
		}
		if uFactor, ok := geometryNumericField(obj, "U-Factor"); ok {
			layer.UFactor = roundedNumber(uFactor, 4)
		}
		if conductivity, ok := geometryNumericField(obj, "Conductivity"); ok {
			layer.Conductivity = roundedNumber(conductivity, 4)
		}
		if density, ok := geometryNumericField(obj, "Density"); ok {
			layer.Density = roundedNumber(density, 3)
		}
		if specificHeat, ok := geometryNumericField(obj, "Specific Heat"); ok {
			layer.SpecificHeat = roundedNumber(specificHeat, 2)
		}
		if layer.HasThickness && layer.Density > 0 && layer.SpecificHeat > 0 {
			layer.ArealHeatCapacity = roundedNumber(layer.Thickness*layer.Density*layer.SpecificHeat, 1)
		}
		materials[normalizeName(name)] = layer
	}
	return materials
}

func geometryConstructionKind(objectType string) string {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	switch {
	case lower == "construction:internalsource":
		return "internal_source"
	case lower == "construction:cfactorundergroundwall":
		return "c_factor"
	case lower == "construction:ffactorgroundfloor":
		return "f_factor"
	case lower == "construction:complexfenestrationstate":
		return "complex_fenestration"
	case strings.Contains(lower, "window"):
		return "layer_based_window"
	default:
		return "layer_based_opaque"
	}
}

func geometryConstructionLayerFieldIndexes(obj Object) []int {
	lower := strings.ToLower(strings.TrimSpace(obj.Type))
	if lower == "construction:cfactorundergroundwall" || lower == "construction:ffactorgroundfloor" || lower == "construction:complexfenestrationstate" {
		return nil
	}
	start := 1
	if lower == "construction:internalsource" {
		start = 6
	}
	indexes := make([]int, 0, len(obj.Fields)-start)
	for index := start; index < len(obj.Fields); index++ {
		indexes = append(indexes, index)
	}
	return indexes
}

func applyDirectGeometryConstructionPerformance(construction *GeometryConstruction, obj Object) {
	switch construction.Kind {
	case "c_factor":
		if cFactor, ok := geometryNumericField(obj, "C-Factor"); ok && cFactor > 0 {
			construction.UValue = roundedNumber(cFactor, 4)
			construction.ThermalResistance = roundedNumber(1/cFactor, 4)
			construction.HasThermalPerformance = true
		}
	case "f_factor":
		fFactor, hasF := geometryNumericField(obj, "F-Factor")
		area, hasArea := geometryNumericField(obj, "Area")
		perimeter, hasPerimeter := geometryNumericField(obj, "Perimeter Exposed")
		if hasF && hasArea && hasPerimeter && fFactor > 0 && area > 0 && perimeter > 0 {
			construction.UValue = roundedNumber(fFactor*perimeter/area, 4)
			construction.ThermalResistance = roundedNumber(1/construction.UValue, 4)
			construction.HasThermalPerformance = true
		}
	}
}

func finalizeGeometryConstructionPerformance(construction *GeometryConstruction) {
	for _, layer := range construction.Layers {
		resistance := geometryLayerThermalResistance(layer)
		if resistance > 0 {
			construction.ThermalResistance += resistance
			construction.HasThermalPerformance = true
		}
		if layer.ArealHeatCapacity > 0 {
			construction.ArealHeatCapacity += layer.ArealHeatCapacity
			construction.HasArealHeatCapacity = true
		}
	}
	if construction.HasThermalPerformance {
		construction.ThermalResistance = roundedNumber(construction.ThermalResistance, 4)
		if construction.ThermalResistance > 0 {
			construction.UValue = roundedNumber(1/construction.ThermalResistance, 4)
		}
	}
	if construction.HasArealHeatCapacity {
		construction.ArealHeatCapacity = roundedNumber(construction.ArealHeatCapacity, 1)
	}
}

func geometryLayerThermalResistance(layer GeometryMaterialLayer) float64 {
	if layer.ThermalResistance > 0 {
		return layer.ThermalResistance
	}
	if layer.UFactor > 0 {
		return 1 / layer.UFactor
	}
	if layer.HasThickness && layer.Thickness > 0 && layer.Conductivity > 0 {
		return layer.Thickness / layer.Conductivity
	}
	return 0
}

func isGeometryMaterialType(objectType string) bool {
	lower := strings.ToLower(strings.TrimSpace(objectType))
	return lower == "material" ||
		strings.HasPrefix(lower, "material:") ||
		strings.HasPrefix(lower, "windowmaterial:")
}

func toFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	default:
		return 0
	}
}
