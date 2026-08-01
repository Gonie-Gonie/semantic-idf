package idf

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	thermalGeometryTransformVersion = "semantic-idf.geometry-transform/v1"
	thermalGeometryCacheEntries     = 24
)

type thermalGeometryCacheKey struct {
	DocumentTextHash         string
	SchemaAdapterVersion     string
	TopologySchemaVersion    string
	GeometryTransformVersion string
}

func newThermalGeometryCacheKey(index *DocumentIndex) thermalGeometryCacheKey {
	if index == nil {
		index = NewDocumentIndex(Document{})
	}
	adapterVersion := fieldCatalogAdapter(thermalEnergyPlusVersion(index.Doc)).AdapterVersion
	return thermalGeometryCacheKey{
		DocumentTextHash:         semanticStableHash(index.Doc.String(), 32),
		SchemaAdapterVersion:     adapterVersion,
		TopologySchemaVersion:    thermalTopologySchema,
		GeometryTransformVersion: thermalGeometryTransformVersion,
	}
}

func (key thermalGeometryCacheKey) String() string {
	return semanticStableHash(strings.Join([]string{
		key.DocumentTextHash,
		key.SchemaAdapterVersion,
		key.TopologySchemaVersion,
		key.GeometryTransformVersion,
	}, "\x00"), 32)
}

type thermalWorldGeometryDescriptor struct {
	CoordinateSystem            string
	RectangularCoordinateSystem string
	VertexEntryDirection        string
	StartingVertexPosition      string
	Bounds                      GeometryBounds
	SurfaceCount                int
	WindowCount                 int
}

type thermalGeometrySharedArtifacts struct {
	ConstructionIndex       map[string]GeometryConstruction
	WorldGeometryDescriptor thermalWorldGeometryDescriptor
	SpatialAdjacencyIndex   map[string][]string
}

type GeometryAnalysisTiming struct {
	CacheHit          bool
	GeometryTransform time.Duration
	ThermalTopology   time.Duration
}

type thermalGeometryCacheEntry struct {
	key       thermalGeometryCacheKey
	report    GeometryReport
	artifacts thermalGeometrySharedArtifacts
	timing    GeometryAnalysisTiming
}

type thermalGeometryCacheFlight struct {
	done  chan struct{}
	entry thermalGeometryCacheEntry
}

type thermalGeometryAnalysisCache struct {
	mu         sync.Mutex
	entries    map[string]thermalGeometryCacheEntry
	flights    map[string]*thermalGeometryCacheFlight
	order      []string
	maxEntries int
	buildCount int
}

func newThermalGeometryAnalysisCache(maxEntries int) *thermalGeometryAnalysisCache {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &thermalGeometryAnalysisCache{
		entries:    make(map[string]thermalGeometryCacheEntry),
		flights:    make(map[string]*thermalGeometryCacheFlight),
		maxEntries: maxEntries,
	}
}

var sharedThermalGeometryCache = newThermalGeometryAnalysisCache(thermalGeometryCacheEntries)

func (cache *thermalGeometryAnalysisCache) getOrCompute(index *DocumentIndex) (thermalGeometryCacheEntry, bool) {
	key := newThermalGeometryCacheKey(index)
	cacheID := key.String()

	cache.mu.Lock()
	if entry, ok := cache.entries[cacheID]; ok {
		cache.touchLocked(cacheID)
		cache.mu.Unlock()
		entry.timing.CacheHit = true
		entry.timing.GeometryTransform = 0
		entry.timing.ThermalTopology = 0
		return entry, true
	}
	if flight, ok := cache.flights[cacheID]; ok {
		cache.mu.Unlock()
		<-flight.done
		entry := flight.entry
		entry.timing.CacheHit = true
		entry.timing.GeometryTransform = 0
		entry.timing.ThermalTopology = 0
		return entry, true
	}
	flight := &thermalGeometryCacheFlight{done: make(chan struct{})}
	cache.flights[cacheID] = flight
	cache.mu.Unlock()

	report, transformDuration, topologyDuration := analyzeGeometryWithIndexMeasured(index.Doc, index)
	entry := thermalGeometryCacheEntry{
		key:       key,
		report:    report,
		artifacts: buildThermalGeometrySharedArtifacts(report),
		timing: GeometryAnalysisTiming{
			GeometryTransform: transformDuration,
			ThermalTopology:   topologyDuration,
		},
	}

	cache.mu.Lock()
	cache.buildCount++
	cache.entries[cacheID] = entry
	cache.touchLocked(cacheID)
	for len(cache.order) > cache.maxEntries {
		evictedID := cache.order[0]
		cache.order = cache.order[1:]
		delete(cache.entries, evictedID)
	}
	flight.entry = entry
	delete(cache.flights, cacheID)
	close(flight.done)
	cache.mu.Unlock()
	return entry, false
}

func (cache *thermalGeometryAnalysisCache) touchLocked(cacheID string) {
	for index, existingID := range cache.order {
		if existingID != cacheID {
			continue
		}
		cache.order = append(cache.order[:index], cache.order[index+1:]...)
		break
	}
	cache.order = append(cache.order, cacheID)
}

func buildThermalGeometrySharedArtifacts(report GeometryReport) thermalGeometrySharedArtifacts {
	adjacency := make(map[string][]string)
	for _, connection := range report.Topology.Connections {
		appendThermalAdjacency(adjacency, connection.FromNodeID, connection.ToNodeID)
		appendThermalAdjacency(adjacency, connection.ToNodeID, connection.FromNodeID)
	}
	for _, observation := range report.Topology.AdjacencyObservations {
		appendThermalAdjacency(adjacency, observation.SurfaceAID, observation.SurfaceBID)
		appendThermalAdjacency(adjacency, observation.SurfaceBID, observation.SurfaceAID)
	}
	for entityID := range adjacency {
		sort.Strings(adjacency[entityID])
	}
	return thermalGeometrySharedArtifacts{
		ConstructionIndex: thermalConstructionIndex(report.Constructions),
		WorldGeometryDescriptor: thermalWorldGeometryDescriptor{
			CoordinateSystem:            report.CoordinateSystem,
			RectangularCoordinateSystem: report.RectangularCoordinateSystem,
			VertexEntryDirection:        report.VertexEntryDirection,
			StartingVertexPosition:      report.StartingVertexPosition,
			Bounds:                      report.Bounds,
			SurfaceCount:                report.SurfaceCount,
			WindowCount:                 report.WindowCount,
		},
		SpatialAdjacencyIndex: adjacency,
	}
}

func appendThermalAdjacency(index map[string][]string, sourceID string, targetID string) {
	if sourceID == "" || targetID == "" || sourceID == targetID {
		return
	}
	index[sourceID] = appendUniqueStrings(index[sourceID], targetID)
}

func analyzeGeometryFromIndexCached(index *DocumentIndex) (GeometryReport, GeometryAnalysisTiming) {
	if index == nil {
		return GeometryReport{}, GeometryAnalysisTiming{}
	}
	entry, _ := sharedThermalGeometryCache.getOrCompute(index)
	return entry.report, entry.timing
}

func AnalyzeGeometryFromIndexTimed(index *DocumentIndex, timer StageTimer) GeometryReport {
	report, timing := analyzeGeometryFromIndexCached(index)
	if timer != nil {
		timer("geometry_transform", timing.GeometryTransform)
		timer("topology", timing.ThermalTopology)
	}
	return report
}
