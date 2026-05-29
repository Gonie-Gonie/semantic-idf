package idf

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

type GeometryReport struct {
	Zones        []GeometryZone    `json:"zones"`
	Surfaces     []GeometrySurface `json:"surfaces"`
	Windows      []GeometryWindow  `json:"windows"`
	Stories      []GeometryStory   `json:"stories"`
	Bounds       GeometryBounds    `json:"bounds"`
	ZoneCount    int               `json:"zoneCount"`
	SurfaceCount int               `json:"surfaceCount"`
	WindowCount  int               `json:"windowCount"`
}

type GeometryZone struct {
	ID          string           `json:"id"`
	ObjectIndex int              `json:"objectIndex"`
	Name        string           `json:"name"`
	StoryIndex  int              `json:"storyIndex"`
	FloorArea   float64          `json:"floorArea"`
	Volume      float64          `json:"volume"`
	MinZ        float64          `json:"minZ"`
	MaxZ        float64          `json:"maxZ"`
	SurfaceIDs  []string         `json:"surfaceIds"`
	WindowIDs   []string         `json:"windowIds"`
	Metrics     []GeometryMetric `json:"metrics"`
	Fields      []Field          `json:"fields"`
}

type GeometrySurface struct {
	ID              string           `json:"id"`
	ObjectIndex     int              `json:"objectIndex"`
	Name            string           `json:"name"`
	Type            string           `json:"type"`
	SurfaceType     string           `json:"surfaceType"`
	ZoneName        string           `json:"zoneName"`
	Construction    string           `json:"construction"`
	OutsideBoundary string           `json:"outsideBoundary"`
	StoryIndex      int              `json:"storyIndex"`
	Area            float64          `json:"area"`
	Azimuth         float64          `json:"azimuth"`
	Orientation     string           `json:"orientation"`
	MinZ            float64          `json:"minZ"`
	MaxZ            float64          `json:"maxZ"`
	Vertices        []GeometryPoint  `json:"vertices"`
	Metrics         []GeometryMetric `json:"metrics"`
	Fields          []Field          `json:"fields"`
}

type GeometryWindow struct {
	ID              string           `json:"id"`
	ObjectIndex     int              `json:"objectIndex"`
	Name            string           `json:"name"`
	Type            string           `json:"type"`
	SurfaceType     string           `json:"surfaceType"`
	BaseSurfaceID   string           `json:"baseSurfaceId,omitempty"`
	BaseSurfaceName string           `json:"baseSurfaceName"`
	ZoneName        string           `json:"zoneName,omitempty"`
	StoryIndex      int              `json:"storyIndex"`
	Area            float64          `json:"area"`
	Azimuth         float64          `json:"azimuth"`
	Orientation     string           `json:"orientation"`
	Vertices        []GeometryPoint  `json:"vertices"`
	Metrics         []GeometryMetric `json:"metrics"`
	Fields          []Field          `json:"fields"`
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

type geometryContext struct {
	buildingNorthAxis    float64
	hasBuildingNorthAxis bool
	vertexEntryDirection string
	zoneDirections       map[string]float64
	zoneMultipliers      map[string]float64
}

func AnalyzeGeometry(doc Document) GeometryReport {
	ctx := geometryContext{
		vertexEntryDirection: "counterclockwise",
		zoneDirections:       map[string]float64{},
		zoneMultipliers:      map[string]float64{},
	}
	report := GeometryReport{}
	zoneByName := map[string]int{}

	for _, obj := range doc.Objects {
		switch {
		case strings.EqualFold(obj.Type, "Building"):
			if value, ok := parseFloatField(findFieldByCommentWords(obj, "north", "axis")); ok {
				ctx.buildingNorthAxis = value
				ctx.hasBuildingNorthAxis = true
			}
		case strings.EqualFold(obj.Type, "GlobalGeometryRules"):
			if value := findFieldByCommentWords(obj, "vertex", "entry", "direction"); value != "" {
				ctx.vertexEntryDirection = strings.ToLower(strings.TrimSpace(value))
			}
		case strings.EqualFold(obj.Type, "Zone"):
			zone := geometryZoneFromObject(obj)
			zoneByName[normalizeName(zone.Name)] = len(report.Zones)
			report.Zones = append(report.Zones, zone)
			ctx.zoneDirections[normalizeName(zone.Name)] = numericFieldOrDefault(obj, 0, "direction", "relative", "north")
			ctx.zoneMultipliers[normalizeName(zone.Name)] = numericFieldOrDefault(obj, 1, "multiplier")
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
		report.addBounds(surface.Vertices)
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
		if !isFenestrationType(obj.Type) {
			continue
		}
		window, ok := geometryWindowFromObject(obj, ctx, surfaceByName)
		if !ok {
			continue
		}
		report.addBounds(window.Vertices)
		report.Windows = append(report.Windows, window)
		if index, ok := zoneByName[normalizeName(window.ZoneName)]; ok {
			report.Zones[index].WindowIDs = append(report.Zones[index].WindowIDs, window.ID)
		}
	}

	report.finalizeZones()
	report.assignStories()
	report.ZoneCount = len(report.Zones)
	report.SurfaceCount = len(report.Surfaces)
	report.WindowCount = len(report.Windows)
	return report
}

func geometryZoneFromObject(obj Object) GeometryZone {
	name := objectName(obj)
	if name == "" {
		name = obj.Type + " #" + strconv.Itoa(obj.Index)
	}
	volume, _ := findNumericFieldByCommentWords(obj, "volume")
	return GeometryZone{
		ID:          "zone-" + strconv.Itoa(obj.Index),
		ObjectIndex: obj.Index,
		Name:        name,
		StoryIndex:  -1,
		Volume:      volume,
		MinZ:        math.Inf(1),
		MaxZ:        math.Inf(-1),
		Fields:      append([]Field(nil), obj.Fields...),
	}
}

func geometrySurfaceFromObject(obj Object, ctx geometryContext) (GeometrySurface, bool) {
	vertices, ok := detailedVertices(obj)
	if !ok {
		return GeometrySurface{}, false
	}
	area, ok := polygonArea(vertices)
	if !ok {
		return GeometrySurface{}, false
	}
	zoneName := findFieldByCommentWords(obj, "zone", "name")
	area *= zoneMultiplierFor(ctx.zoneMultipliers, zoneName)
	points := geometryPoints(vertices)
	minZ, maxZ, _ := verticesZStats(vertices)
	azimuth, hasAzimuth := geometryAzimuth(obj, vertices, zoneName, ctx)
	orientation := ""
	if hasAzimuth {
		orientation = orientationFromAzimuth(azimuth)
	}
	surfaceType := buildingSurfaceType(obj)
	surface := GeometrySurface{
		ID:              "surface-" + strconv.Itoa(obj.Index),
		ObjectIndex:     obj.Index,
		Name:            objectName(obj),
		Type:            obj.Type,
		SurfaceType:     surfaceType,
		ZoneName:        zoneName,
		Construction:    findFieldByCommentWords(obj, "construction", "name"),
		OutsideBoundary: findFieldByCommentWords(obj, "outside", "boundary", "condition"),
		StoryIndex:      -1,
		Area:            roundedNumber(area, 3),
		Azimuth:         roundedNumber(azimuth, 2),
		Orientation:     orientation,
		MinZ:            roundedNumber(minZ, 3),
		MaxZ:            roundedNumber(maxZ, 3),
		Vertices:        points,
		Fields:          append([]Field(nil), obj.Fields...),
	}
	surface.Metrics = []GeometryMetric{
		geometryMetric("Area", surface.Area, "m2", 2),
		geometryMetric("Azimuth", surface.Azimuth, "deg", 1),
		geometryMetric("Orientation", surface.Orientation, "", 0),
		geometryMetric("Minimum Z", surface.MinZ, "m", 2),
		geometryMetric("Maximum Z", surface.MaxZ, "m", 2),
	}
	return surface, true
}

func geometryWindowFromObject(obj Object, ctx geometryContext, surfaces map[string]GeometrySurface) (GeometryWindow, bool) {
	vertices, ok := detailedVertices(obj)
	if !ok {
		return GeometryWindow{}, false
	}
	area, ok := polygonArea(vertices)
	if !ok {
		return GeometryWindow{}, false
	}
	baseName := findFieldByCommentWords(obj, "building", "surface", "name")
	if baseName == "" {
		baseName = findFieldByCommentWords(obj, "surface", "name")
	}
	base := surfaces[normalizeName(baseName)]
	multiplier := numericFieldOrDefault(obj, 1, "multiplier")
	area *= multiplier
	if base.ZoneName != "" {
		area *= zoneMultiplierFor(ctx.zoneMultipliers, base.ZoneName)
	}
	azimuth, hasAzimuth := geometryAzimuth(obj, vertices, base.ZoneName, ctx)
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
		BaseSurfaceID:   base.ID,
		BaseSurfaceName: baseName,
		ZoneName:        base.ZoneName,
		StoryIndex:      -1,
		Area:            roundedNumber(area, 3),
		Azimuth:         roundedNumber(azimuth, 2),
		Orientation:     orientation,
		Vertices:        geometryPoints(vertices),
		Fields:          append([]Field(nil), obj.Fields...),
	}
	window.Metrics = []GeometryMetric{
		geometryMetric("Area", window.Area, "m2", 2),
		geometryMetric("Azimuth", window.Azimuth, "deg", 1),
		geometryMetric("Orientation", window.Orientation, "", 0),
		geometryMetric("Multiplier", multiplier, "", 2),
	}
	return window, true
}

func geometryAzimuth(obj Object, vertices []point3, zoneName string, ctx geometryContext) (float64, bool) {
	if value, ok := parseFloatField(findFieldByCommentWords(obj, "azimuth")); ok {
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
	return normalizeDegrees(azimuth + geometryRotation(zoneName, ctx)), true
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
		return surface.Area
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
