package idf

import (
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type Report struct {
	ObjectCount     int              `json:"objectCount"`
	TypeCounts      []TypeCount      `json:"typeCounts"`
	Objects         []ObjectSummary  `json:"objects"`
	Schedules       []ScheduleInfo   `json:"schedules"`
	Zones           []ZoneInfo       `json:"zones"`
	HVACConnections []HVACConnection `json:"hvacConnections"`
	UnusedObjects   []NamedObject    `json:"unusedObjects"`
	Summary         SummaryReport    `json:"summary"`
	Output          OutputReport     `json:"output"`
	Profile         ProfileReport    `json:"profile"`
	HVAC            HVACReport       `json:"hvac"`
	Geometry        GeometryReport   `json:"geometry"`
	Diagnostics     []Diagnostic     `json:"diagnostics"`
}

type TypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type ObjectSummary struct {
	Index      int     `json:"index"`
	Type       string  `json:"type"`
	Name       string  `json:"name,omitempty"`
	FieldCount int     `json:"fieldCount"`
	Fields     []Field `json:"fields"`
}

type ScheduleInfo struct {
	Index      int    `json:"index"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	FieldCount int    `json:"fieldCount"`
}

type ZoneInfo struct {
	Index          int             `json:"index"`
	Name           string          `json:"name"`
	SurfaceCount   int             `json:"surfaceCount"`
	Surfaces       []RelatedObject `json:"surfaces,omitempty"`
	RelatedObjects []RelatedObject `json:"relatedObjects,omitempty"`
}

type HVACConnection struct {
	ObjectIndex int    `json:"objectIndex"`
	ObjectType  string `json:"objectType"`
	ObjectName  string `json:"objectName,omitempty"`
	FromNode    string `json:"fromNode"`
	ToNode      string `json:"toNode"`
}

type NamedObject struct {
	Index int    `json:"index"`
	Type  string `json:"type"`
	Name  string `json:"name"`
}

type RelatedObject struct {
	Index int    `json:"index"`
	Type  string `json:"type"`
	Name  string `json:"name,omitempty"`
	Role  string `json:"role,omitempty"`
}

type StageTimer func(stage string, elapsed time.Duration)

func AnalyzeOverview(doc Document) Report {
	return AnalyzeOverviewTimed(doc, nil)
}

func AnalyzeQuick(doc Document) Report {
	return AnalyzeQuickTimed(doc, nil)
}

func AnalyzeQuickTimed(doc Document, timer StageTimer) Report {
	return AnalyzeQuickFromIndex(NewDocumentIndex(doc), timer)
}

func AnalyzeQuickFromIndex(index *DocumentIndex, timer StageTimer) Report {
	if index == nil {
		return Report{}
	}
	doc := index.Doc
	var report Report
	timeAnalysisStage(timer, "core", func() {
		report = analyzeCore(doc)
	})
	timeAnalysisStage(timer, "summary", func() {
		report.Summary = AnalyzeSummaryQuick(doc)
	})
	timeAnalysisStage(timer, "output", func() {
		report.Output = AnalyzeOutputFromIndex(index)
	})
	return report
}

func AnalyzeOverviewTimed(doc Document, timer StageTimer) Report {
	index := NewDocumentIndex(doc)
	var report Report
	timeAnalysisStage(timer, "core", func() {
		report = analyzeCore(doc)
	})
	timeAnalysisStage(timer, "summary", func() {
		report.Summary = AnalyzeSummary(doc)
	})
	timeAnalysisStage(timer, "output", func() {
		report.Output = AnalyzeOutputFromIndex(index)
	})
	timeAnalysisStage(timer, "profile", func() {
		report.Profile = AnalyzeProfileFromIndex(index)
	})
	timeAnalysisStage(timer, "hvac", func() {
		report.HVAC = AnalyzeHVACFromIndex(index)
	})
	return report
}

func Analyze(doc Document) Report {
	return AnalyzeTimed(doc, nil)
}

func AnalyzeTimed(doc Document, timer StageTimer) Report {
	index := NewDocumentIndex(doc)
	var report Report
	timeAnalysisStage(timer, "core", func() {
		report = analyzeCore(doc)
	})
	var unusedObjects []NamedObject
	var summary SummaryReport
	var output OutputReport
	var profile ProfileReport
	var hvac HVACReport
	var geometry GeometryReport
	var diagnostics []Diagnostic

	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxAnalysisWorkers())
	run := func(work func()) {
		wg.Add(1)
		go func() {
			sem <- struct{}{}
			defer func() {
				<-sem
				wg.Done()
			}()
			work()
		}()
	}
	run(func() {
		timeAnalysisStage(timer, "unused", func() {
			unusedObjects = FindUnusedObjects(doc)
		})
	})
	run(func() {
		timeAnalysisStage(timer, "summary", func() {
			summary = AnalyzeSummary(doc)
		})
	})
	run(func() {
		timeAnalysisStage(timer, "output", func() {
			output = AnalyzeOutputFromIndex(index)
		})
	})
	run(func() {
		timeAnalysisStage(timer, "profile", func() {
			profile = AnalyzeProfileFromIndex(index)
		})
	})
	run(func() {
		timeAnalysisStage(timer, "hvac", func() {
			hvac = AnalyzeHVACFromIndex(index)
		})
	})
	run(func() {
		timeAnalysisStage(timer, "geometry", func() {
			geometry = AnalyzeGeometryFromIndex(index)
		})
	})
	run(func() {
		timeAnalysisStage(timer, "diagnostics", func() {
			diagnostics = AnalyzeDiagnosticsFromIndex(index)
		})
	})
	wg.Wait()

	report.UnusedObjects = unusedObjects
	report.Summary = summary
	report.Output = output
	report.Profile = profile
	report.HVAC = hvac
	report.Geometry = geometry
	report.Diagnostics = diagnostics
	return report
}

func timeAnalysisStage(timer StageTimer, stage string, work func()) {
	start := time.Now()
	work()
	if timer != nil {
		timer(stage, time.Since(start))
	}
}

func MaxAnalysisWorkers() int {
	cpus := runtime.NumCPU()
	if cpus < 1 {
		return 1
	}
	if cpus > 4 {
		return 4
	}
	return cpus
}

func analyzeCore(doc Document) Report {
	report := Report{ObjectCount: len(doc.Objects)}
	typeCounts := map[string]int{}
	zoneSurfaces := map[string][]RelatedObject{}
	zoneRelatedObjects := map[string][]RelatedObject{}

	for _, obj := range doc.Objects {
		typeCounts[obj.Type]++
		name := objectName(obj)
		report.Objects = append(report.Objects, ObjectSummary{
			Index:      obj.Index,
			Type:       obj.Type,
			Name:       name,
			FieldCount: len(obj.Fields),
			Fields:     append([]Field(nil), obj.Fields...),
		})

		if isScheduleType(obj.Type) && name != "" {
			report.Schedules = append(report.Schedules, ScheduleInfo{
				Index:      obj.Index,
				Type:       obj.Type,
				Name:       name,
				FieldCount: len(obj.Fields),
			})
		}

		if strings.EqualFold(obj.Type, "Zone") && name != "" {
			report.Zones = append(report.Zones, ZoneInfo{Index: obj.Index, Name: name})
		}

		if strings.EqualFold(obj.Type, "BuildingSurface:Detailed") {
			if zoneName := findFieldByCommentWords(obj, "zone", "name"); zoneName != "" {
				zoneSurfaces[normalizeName(zoneName)] = append(zoneSurfaces[normalizeName(zoneName)], relatedObject(obj, "surface"))
			}
		} else if !strings.EqualFold(obj.Type, "Zone") {
			if zoneName := findFieldByCommentWords(obj, "zone", "name"); zoneName != "" {
				zoneRelatedObjects[normalizeName(zoneName)] = append(zoneRelatedObjects[normalizeName(zoneName)], relatedObject(obj, "zone reference"))
			}
		}

		report.HVACConnections = append(report.HVACConnections, extractHVACConnections(obj)...)
	}

	for i := range report.Zones {
		zoneKey := normalizeName(report.Zones[i].Name)
		report.Zones[i].Surfaces = zoneSurfaces[zoneKey]
		report.Zones[i].SurfaceCount = len(report.Zones[i].Surfaces)
		report.Zones[i].RelatedObjects = zoneRelatedObjects[zoneKey]
	}

	for objectType, count := range typeCounts {
		report.TypeCounts = append(report.TypeCounts, TypeCount{Type: objectType, Count: count})
	}
	sort.Slice(report.TypeCounts, func(i, j int) bool {
		if report.TypeCounts[i].Count == report.TypeCounts[j].Count {
			return report.TypeCounts[i].Type < report.TypeCounts[j].Type
		}
		return report.TypeCounts[i].Count > report.TypeCounts[j].Count
	})

	return report
}

func FindUnusedObjects(doc Document) []NamedObject {
	owners := map[string][]NamedObject{}
	references := map[string]int{}

	for _, obj := range doc.Objects {
		name := objectName(obj)
		if name != "" && isUnusedCandidateType(obj.Type) && !isProtectedType(obj.Type) {
			key := normalizeName(name)
			owners[key] = append(owners[key], NamedObject{Index: obj.Index, Type: obj.Type, Name: name})
		}

		start := 0
		if name != "" {
			start = 1
		}
		for _, field := range obj.Fields[start:] {
			value := normalizeName(field.Value)
			if value != "" {
				references[value]++
			}
		}
	}

	var unused []NamedObject
	for key, namedObjects := range owners {
		if references[key] == 0 {
			unused = append(unused, namedObjects...)
		}
	}

	sort.Slice(unused, func(i, j int) bool {
		return unused[i].Index < unused[j].Index
	})
	return unused
}

func isScheduleType(objectType string) bool {
	return strings.HasPrefix(strings.ToLower(objectType), "schedule:")
}

func findFieldByComment(obj Object, commentNeedle string) string {
	commentNeedle = strings.ToLower(commentNeedle)
	for _, field := range obj.Fields {
		if strings.Contains(strings.ToLower(field.Comment), commentNeedle) {
			return strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func findFieldByCommentWords(obj Object, words ...string) string {
	for _, field := range obj.Fields {
		comment := strings.ToLower(field.Comment)
		matched := true
		for _, word := range words {
			if !strings.Contains(comment, strings.ToLower(word)) {
				matched = false
				break
			}
		}
		if matched {
			return strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func relatedObject(obj Object, role string) RelatedObject {
	return RelatedObject{
		Index: obj.Index,
		Type:  obj.Type,
		Name:  objectName(obj),
		Role:  role,
	}
}

type nodeField struct {
	value string
	role  string
}

func extractHVACConnections(obj Object) []HVACConnection {
	var nodes []nodeField
	for _, field := range obj.Fields {
		comment := strings.ToLower(field.Comment)
		value := strings.TrimSpace(field.Value)
		if value == "" || !strings.Contains(comment, "node") || !strings.Contains(comment, "name") {
			continue
		}

		role := "node"
		switch {
		case strings.Contains(comment, "inlet"):
			role = "inlet"
		case strings.Contains(comment, "outlet"):
			role = "outlet"
		}
		nodes = append(nodes, nodeField{value: value, role: role})
	}

	name := objectName(obj)
	var connections []HVACConnection
	var inlets []string
	var outlets []string
	for _, node := range nodes {
		switch node.role {
		case "inlet":
			inlets = append(inlets, node.value)
		case "outlet":
			outlets = append(outlets, node.value)
		}
	}

	if len(inlets) > 0 && len(outlets) > 0 {
		for _, inlet := range inlets {
			for _, outlet := range outlets {
				if inlet == outlet {
					continue
				}
				connections = append(connections, HVACConnection{
					ObjectIndex: obj.Index,
					ObjectType:  obj.Type,
					ObjectName:  name,
					FromNode:    inlet,
					ToNode:      outlet,
				})
			}
		}
		return connections
	}

	for i := 0; i < len(nodes)-1; i++ {
		if nodes[i].value == nodes[i+1].value {
			continue
		}
		connections = append(connections, HVACConnection{
			ObjectIndex: obj.Index,
			ObjectType:  obj.Type,
			ObjectName:  name,
			FromNode:    nodes[i].value,
			ToNode:      nodes[i+1].value,
		})
	}
	return connections
}

func isNamelessType(objectType string) bool {
	switch strings.ToLower(objectType) {
	case "version",
		"simulationcontrol",
		"building",
		"timestep",
		"runperiod",
		"globalgeometryrules",
		"shadowcalculation",
		"heatbalancealgorithm",
		"surfaceconvectionalgorithm:inside",
		"surfaceconvectionalgorithm:outside",
		"zoneairheatbalancealgorithm",
		"zoneaircontaminantbalance":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(objectType), "output:") ||
			strings.HasPrefix(strings.ToLower(objectType), "outputcontrol:") ||
			strings.HasPrefix(strings.ToLower(objectType), "meter:")
	}
}

func isProtectedType(objectType string) bool {
	if isNamelessType(objectType) {
		return true
	}

	switch strings.ToLower(objectType) {
	case "scheduletypelimits":
		return true
	default:
		return false
	}
}

func isUnusedCandidateType(objectType string) bool {
	objectType = strings.ToLower(objectType)
	return strings.HasPrefix(objectType, "schedule:") ||
		strings.HasPrefix(objectType, "material") ||
		strings.HasPrefix(objectType, "windowmaterial") ||
		strings.HasPrefix(objectType, "construction") ||
		strings.HasPrefix(objectType, "curve:") ||
		strings.HasPrefix(objectType, "table:")
}
