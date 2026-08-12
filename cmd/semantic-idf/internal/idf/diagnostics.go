package idf

import (
	"fmt"
	"sort"
	"strings"
)

const (
	DiagnosticError   = "error"
	DiagnosticWarning = "warning"
	DiagnosticNotice  = "notice"
)

type Diagnostic struct {
	ID               string   `json:"id,omitempty"`
	Severity         string   `json:"severity"`
	Category         string   `json:"category"`
	Code             string   `json:"code"`
	Source           string   `json:"source,omitempty"`
	Confidence       string   `json:"confidence,omitempty"`
	Message          string   `json:"message"`
	ObjectIndex      int      `json:"objectIndex,omitempty"`
	ObjectType       string   `json:"objectType,omitempty"`
	ObjectName       string   `json:"objectName,omitempty"`
	FieldIndex       int      `json:"fieldIndex,omitempty"`
	Field            string   `json:"field,omitempty"`
	Value            string   `json:"value,omitempty"`
	Evidence         string   `json:"evidence,omitempty"`
	RelatedEntityIDs []string `json:"relatedEntityIds,omitempty"`
}

type diagnosticRefKind struct {
	code       string
	category   string
	label      string
	ownerTypes func(string) bool
	fieldMatch func(string) bool
}

var diagnosticReferenceKinds = []diagnosticRefKind{
	{
		code:       "missing_schedule_reference",
		category:   "Reference",
		label:      "schedule",
		ownerTypes: isScheduleType,
		fieldMatch: commentHasWords("schedule", "name"),
	},
	{
		code:     "missing_construction_reference",
		category: "Reference",
		label:    "construction",
		ownerTypes: func(objectType string) bool {
			return strings.HasPrefix(strings.ToLower(objectType), "construction")
		},
		fieldMatch: commentHasWords("construction", "name"),
	},
	{
		code:       "missing_material_reference",
		category:   "Reference",
		label:      "material",
		ownerTypes: isMaterialType,
		fieldMatch: commentHasWords("material", "name"),
	},
	{
		code:       "missing_zone_reference",
		category:   "Reference",
		label:      "zone",
		ownerTypes: isZoneReferenceTargetType,
		fieldMatch: commentHasWords("zone", "name"),
	},
}

func AnalyzeDiagnostics(doc Document) []Diagnostic {
	index := NewDocumentIndex(doc)
	return analyzeDiagnosticsWithReports(doc, AnalyzeHVACFromIndex(index), AnalyzeGeometryFromIndex(index))
}

func analyzeDiagnosticsWithReports(doc Document, hvac HVACReport, geometry GeometryReport) []Diagnostic {
	var diagnostics []Diagnostic
	diagnostics = append(diagnostics, requiredObjectDiagnostics(doc)...)
	diagnostics = append(diagnostics, duplicateNameDiagnostics(doc)...)
	diagnostics = append(diagnostics, referenceDiagnostics(doc)...)
	diagnostics = append(diagnostics, fieldCatalogDiagnostics(doc)...)
	diagnostics = append(diagnostics, orphanDiagnostics(doc)...)
	diagnostics = append(diagnostics, geometryDiagnosticsFromReport(doc, geometry)...)
	diagnostics = append(diagnostics, thermalTopologyDiagnostics(geometry.Topology)...)
	diagnostics = append(diagnostics, scheduleDiagnostics(doc)...)
	diagnostics = append(diagnostics, hvacNodeDiagnosticsForReport(hvac)...)
	diagnostics = append(diagnostics, hvacConnectionDiagnosticsForReport(hvac)...)
	diagnostics = mergeDiagnostics(diagnostics)
	for index := range diagnostics {
		if diagnostics[index].ID == "" {
			diagnostics[index].ID = semanticDiagnosticEntityID(diagnostics[index])
		}
	}

	sort.SliceStable(diagnostics, func(i, j int) bool {
		if diagnostics[i].Severity != diagnostics[j].Severity {
			return diagnosticSeverityRank(diagnostics[i].Severity) < diagnosticSeverityRank(diagnostics[j].Severity)
		}
		if diagnostics[i].Source != diagnostics[j].Source {
			return diagnostics[i].Source < diagnostics[j].Source
		}
		if diagnostics[i].Category != diagnostics[j].Category {
			return diagnostics[i].Category < diagnostics[j].Category
		}
		if diagnostics[i].ObjectIndex != diagnostics[j].ObjectIndex {
			return diagnostics[i].ObjectIndex < diagnostics[j].ObjectIndex
		}
		return diagnostics[i].Message < diagnostics[j].Message
	})
	return diagnostics
}

func requiredObjectDiagnostics(doc Document) []Diagnostic {
	required := []string{"Version", "Building", "Timestep", "RunPeriod", "SimulationControl"}
	present := map[string]bool{}
	for _, obj := range doc.Objects {
		present[strings.ToLower(obj.Type)] = true
	}

	var diagnostics []Diagnostic
	for _, objectType := range required {
		if !present[strings.ToLower(objectType)] {
			severity := DiagnosticError
			source := "energyplus_rule"
			evidence := "Required for a weather-file simulation run."
			if objectType == "RunPeriod" && hasSizingPeriodOnlyContext(doc) {
				severity = DiagnosticNotice
				source = "simulation_context"
				evidence = "Design-day or sizing-period objects are present, so the model may intentionally omit RunPeriod."
			}
			diagnostics = append(diagnostics, Diagnostic{
				Severity: severity,
				Category: "Required Object",
				Code:     "missing_required_object",
				Source:   source,
				Evidence: evidence,
				Message:  fmt.Sprintf("Missing required %s object.", objectType),
				Value:    objectType,
			})
		}
	}
	return diagnostics
}

func duplicateNameDiagnostics(doc Document) []Diagnostic {
	byTypeAndName := map[string][]Object{}
	for _, obj := range doc.Objects {
		name := objectName(obj)
		if name == "" {
			continue
		}
		key := strings.ToLower(obj.Type) + "\x00" + normalizeName(name)
		byTypeAndName[key] = append(byTypeAndName[key], obj)
	}

	var diagnostics []Diagnostic
	for _, objects := range byTypeAndName {
		if len(objects) < 2 {
			continue
		}
		for _, obj := range objects {
			diagnostics = append(diagnostics, diagnosticForObject(DiagnosticError, "Duplicate Name", "duplicate_name", obj,
				fmt.Sprintf("Duplicate %s name %q.", obj.Type, objectName(obj))).withSource("energyplus_rule", "Object names must be unique within the same object type."))
		}
	}
	return diagnostics
}

func referenceDiagnostics(doc Document) []Diagnostic {
	owners := map[string]map[string]bool{}
	for _, kind := range diagnosticReferenceKinds {
		owners[kind.code] = map[string]bool{}
	}

	for _, obj := range doc.Objects {
		name := objectName(obj)
		if name == "" {
			continue
		}
		for _, kind := range diagnosticReferenceKinds {
			if kind.ownerTypes(obj.Type) {
				owners[kind.code][normalizeName(name)] = true
			}
		}
	}

	var diagnostics []Diagnostic
	for _, obj := range doc.Objects {
		for fieldIndex, field := range obj.Fields {
			value := strings.TrimSpace(field.Value)
			if value == "" || strings.EqualFold(value, "autocalculate") {
				continue
			}
			for _, kind := range diagnosticReferenceKinds {
				if kind.ownerTypes(obj.Type) || diagnosticFieldReferenceKind(obj, fieldIndex, field) != kind.code {
					continue
				}
				if !owners[kind.code][normalizeName(value)] {
					diagnostics = append(diagnostics, Diagnostic{
						Severity:    DiagnosticError,
						Category:    kind.category,
						Code:        kind.code,
						Source:      "energyplus_rule",
						Confidence:  "high",
						Message:     fmt.Sprintf("Missing %s reference %q.", kind.label, value),
						ObjectIndex: obj.Index,
						ObjectType:  obj.Type,
						ObjectName:  objectName(obj),
						FieldIndex:  fieldIndex,
						Field:       catalogFieldName(obj, fieldIndex),
						Value:       value,
						Evidence:    "Field catalog role " + catalogFieldRole(obj, fieldIndex),
					})
				}
			}
		}
	}
	return diagnostics
}

func orphanDiagnostics(doc Document) []Diagnostic {
	unused := FindUnusedObjects(doc)
	diagnostics := make([]Diagnostic, 0, len(unused))
	for _, obj := range unused {
		diagnostics = append(diagnostics, Diagnostic{
			Severity:    DiagnosticNotice,
			Category:    "Model Cleanup",
			Code:        "orphan_object",
			Source:      "user_quality_check",
			Confidence:  "medium",
			Message:     fmt.Sprintf("%s %q is not referenced by other objects.", obj.Type, obj.Name),
			ObjectIndex: obj.Index,
			ObjectType:  obj.Type,
			ObjectName:  obj.Name,
			Evidence:    "No inbound references were found in parsed object fields.",
		})
	}
	return diagnostics
}

func geometryDiagnosticsFromReport(doc Document, geometry GeometryReport) []Diagnostic {
	geometryByObjectIndex := map[int]struct {
		physicalArea float64
		zoneName     string
	}{}
	for _, surface := range geometry.Surfaces {
		geometryByObjectIndex[surface.ObjectIndex] = struct {
			physicalArea float64
			zoneName     string
		}{physicalArea: surface.PhysicalArea, zoneName: surface.ZoneName}
	}
	for _, opening := range geometry.Windows {
		geometryByObjectIndex[opening.ObjectIndex] = struct {
			physicalArea float64
			zoneName     string
		}{physicalArea: opening.PhysicalArea, zoneName: opening.ZoneName}
	}
	zoneNames := map[string]bool{}
	for _, obj := range doc.Objects {
		if isZoneLikeType(obj.Type) {
			if name := objectName(obj); name != "" {
				zoneNames[normalizeName(name)] = true
			}
		}
	}

	var diagnostics []Diagnostic
	for _, obj := range doc.Objects {
		if !isBuildingSurfaceType(obj.Type) && !isFenestrationType(obj.Type) {
			continue
		}
		resolvedGeometry, hasGeometry := geometryByObjectIndex[obj.Index]
		if !hasGeometry {
			diagnostics = append(diagnostics, diagnosticForObject(DiagnosticWarning, "Geometry", "invalid_geometry", obj,
				fmt.Sprintf("%s %q could not be converted to a valid world-space polygon.", obj.Type, objectLabel(obj))).withSource("energyplus_rule", "Heat-transfer surfaces require a valid detailed or rectangular geometry definition."))
			diagnostics = append(diagnostics, diagnosticForObject(DiagnosticWarning, "Geometry", "zero_area", obj,
				fmt.Sprintf("%s %q has zero or invalid area.", obj.Type, objectLabel(obj))).withSource("energyplus_rule", "Surface polygons require a valid physical area."))
			continue
		}
		if resolvedGeometry.physicalArea <= 0 {
			diagnostics = append(diagnostics, diagnosticForObject(DiagnosticWarning, "Geometry", "zero_area", obj,
				fmt.Sprintf("%s %q has zero or invalid area.", obj.Type, objectLabel(obj))).withSource("energyplus_rule", "Surface polygons require a valid physical area."))
		}
		if isBuildingSurfaceType(obj.Type) {
			zoneName := resolvedGeometry.zoneName
			if zoneName == "" || !zoneNames[normalizeName(zoneName)] {
				diagnostics = append(diagnostics, diagnosticForObject(DiagnosticWarning, "Geometry", "surface_unconnected_zone", obj,
					fmt.Sprintf("Surface %q references missing zone %q.", objectLabel(obj), zoneName)).withSource("energyplus_rule", "BuildingSurface:Detailed Zone Name must resolve to a Zone or Space."))
			}
		}
	}
	return diagnostics
}

func thermalTopologyDiagnostics(topology ThermalTopologyReport) []Diagnostic {
	diagnostics := make([]Diagnostic, 0, len(topology.IssueLinks))
	for _, issue := range topology.IssueLinks {
		diagnostic := Diagnostic{
			ID:               issue.ID,
			Severity:         firstNonEmpty(issue.Severity, DiagnosticWarning),
			Category:         "Topology",
			Code:             issue.Code,
			Source:           "energyplus_rule",
			Confidence:       "high",
			Message:          issue.Message,
			Evidence:         thermalTopologyIssueEvidence(issue),
			RelatedEntityIDs: thermalTopologyIssueEntityIDs(issue),
		}
		if anchor, ok := preferredThermalIssueAnchor(issue); ok {
			if anchor.ObjectIndex != nil {
				diagnostic.ObjectIndex = *anchor.ObjectIndex
			}
			diagnostic.ObjectType = anchor.ObjectType
			diagnostic.ObjectName = anchor.ObjectName
			if anchor.FieldIndex != nil {
				diagnostic.FieldIndex = *anchor.FieldIndex
			}
			diagnostic.Field = anchor.FieldName
		}
		diagnostics = append(diagnostics, diagnostic)
	}
	return diagnostics
}

func preferredThermalIssueAnchor(issue ThermalTopologyIssueLink) (SemanticSourceAnchor, bool) {
	anchors := issue.SourceAnchors
	if len(anchors) == 0 {
		return SemanticSourceAnchor{}, false
	}
	preferredField := ""
	switch issue.Code {
	case thermalDiagnosticMissingBoundaryTarget, thermalDiagnosticSurfaceCounterpartMissing, thermalDiagnosticSurfaceCounterpartOneWay, thermalDiagnosticSurfaceCounterpartDuplicate, thermalDiagnosticSurfaceSelfReferenceInvalid, thermalDiagnosticSurfacePairZoneMismatch, thermalDiagnosticFenestrationCounterpartMissing, thermalDiagnosticFenestrationCounterpartOneWay:
		preferredField = "Outside Boundary Condition Object"
	case thermalDiagnosticInvalidBoundaryCondition, thermalDiagnosticBoundaryExposureRuleMismatch:
		preferredField = "Outside Boundary Condition"
	case thermalDiagnosticFenestrationBaseSurfaceMissing, thermalDiagnosticFenestrationZoneMismatch, thermalDiagnosticFenestrationAreaExceedsBase:
		preferredField = "Building Surface Name"
	case thermalDiagnosticSurfaceMissingConstruction, thermalDiagnosticSurfaceConstructionUnresolved, thermalDiagnosticSurfacePairConstructionMismatch, thermalDiagnosticSurfacePairLayerOrderMismatch, thermalDiagnosticFenestrationConstructionMismatch:
		preferredField = "Construction Name"
	}
	if preferredField != "" {
		for _, anchor := range anchors {
			if strings.EqualFold(anchor.FieldName, preferredField) {
				return anchor, true
			}
		}
	}
	for _, anchor := range anchors {
		if anchor.FieldIndex != nil {
			return anchor, true
		}
	}
	return anchors[0], true
}

func thermalTopologyIssueEntityIDs(issue ThermalTopologyIssueLink) []string {
	values := append([]string(nil), issue.RelatedEntityIDs...)
	for _, value := range []string{issue.EntityID, issue.BoundaryID, issue.OpeningID, issue.AirCouplingID} {
		if value != "" {
			values = appendUniqueString(values, value)
		}
	}
	return values
}

func thermalTopologyIssueEvidence(issue ThermalTopologyIssueLink) string {
	parts := []string{"Generated by authoritative thermal topology analysis."}
	if issue.EntityID != "" {
		parts = append(parts, "entity="+issue.EntityID)
	}
	if issue.BoundaryID != "" {
		parts = append(parts, "boundary="+issue.BoundaryID)
	}
	if issue.OpeningID != "" {
		parts = append(parts, "opening="+issue.OpeningID)
	}
	if issue.AirCouplingID != "" {
		parts = append(parts, "air_coupling="+issue.AirCouplingID)
	}
	return strings.Join(parts, " ")
}

func scheduleDiagnostics(doc Document) []Diagnostic {
	referenced := map[string]bool{}
	schedules := map[string]Object{}
	for _, obj := range doc.Objects {
		if isScheduleType(obj.Type) {
			if name := objectName(obj); name != "" {
				schedules[normalizeName(name)] = obj
			}
			continue
		}
		for _, field := range obj.Fields {
			if commentHasWords("schedule", "name")(field.Comment) && strings.TrimSpace(field.Value) != "" {
				referenced[normalizeName(field.Value)] = true
			}
		}
	}

	var diagnostics []Diagnostic
	for key := range referenced {
		schedule, ok := schedules[key]
		if !ok {
			continue
		}
		if _, supported := annualScheduleHours(schedule); !supported {
			diagnostics = append(diagnostics, diagnosticForObject(DiagnosticNotice, "Schedule", "unsupported_annual_hours", schedule,
				fmt.Sprintf("Referenced schedule %q cannot be evaluated for annual operating hours.", objectName(schedule))).withSource("analyzer_limitation", "Annual-hour parser currently supports Schedule:Constant and common Schedule:Compact forms."))
		}
	}
	return diagnostics
}

func hvacNodeDiagnostics(doc Document) []Diagnostic {
	return hvacNodeDiagnosticsForReport(AnalyzeHVAC(doc))
}

func hvacNodeDiagnosticsForReport(report HVACReport) []Diagnostic {
	nodeRefs := map[string][]HVACNodeUsage{}
	degree := map[string]int{}
	graph := map[string]map[string]bool{}
	var diagnostics []Diagnostic

	for _, usage := range report.NodeUsages {
		nodeRefs[normalizeName(usage.NodeName)] = append(nodeRefs[normalizeName(usage.NodeName)], usage)
	}
	for _, loop := range report.Loops {
		loopGraph := map[string]map[string]bool{}
		addHVACLoopSideDiagnosticEdges(loopGraph, degree, loop.SupplySide)
		addHVACLoopSideDiagnosticEdges(loopGraph, degree, loop.DemandSide)
		for node, edges := range loopGraph {
			if graph[node] == nil {
				graph[node] = map[string]bool{}
			}
			for edge := range edges {
				graph[node][edge] = true
			}
		}
		if components := graphComponentCount(loopGraph); components > 1 {
			diagnostics = append(diagnostics, Diagnostic{
				Severity:    DiagnosticNotice,
				Category:    "HVAC Node",
				Code:        "disconnected_node_graph",
				Source:      "heuristic_inference",
				Confidence:  "low",
				Message:     fmt.Sprintf("%s %q inferred node graph has %d disconnected components.", loop.Type, loop.Name, components),
				ObjectIndex: loop.ObjectIndex,
				ObjectType:  loop.Type,
				ObjectName:  loop.Name,
				Evidence:    "Computed per HVAC loop from typed branch component inlet/outlet nodes.",
			})
		}
	}

	for node, usages := range nodeRefs {
		if degree[node] == 0 && len(usages) == 1 && !isTerminalHVACBoundaryNode(usages[0]) {
			usage := usages[0]
			diagnostics = append(diagnostics, Diagnostic{
				Severity:    DiagnosticNotice,
				Category:    "HVAC Node",
				Code:        "unconnected_node",
				Source:      "heuristic_inference",
				Confidence:  "medium",
				Message:     fmt.Sprintf("Node %q appears only once and is not connected by inferred inlet/outlet links.", usage.NodeName),
				ObjectIndex: usage.ObjectIndex,
				ObjectType:  usage.ObjectType,
				ObjectName:  usage.ObjectName,
				FieldIndex:  usage.FieldIndex,
				Field:       usage.FieldName,
				Value:       usage.NodeName,
				Evidence:    "Single node usage with no inferred HVAC connection edge.",
			})
		}
	}

	return diagnostics
}

func addHVACLoopSideDiagnosticEdges(graph map[string]map[string]bool, degree map[string]int, side HVACLoopSide) {
	for _, branch := range side.Branches {
		for _, component := range branch.Components {
			addHVACDiagnosticNodeEdge(graph, degree, component.InletNode, component.OutletNode)
			addHVACDiagnosticNodeEdge(graph, degree, component.WaterInletNode, component.WaterOutletNode)
		}
	}
}

func addHVACDiagnosticNodeEdge(graph map[string]map[string]bool, degree map[string]int, fromNode string, toNode string) {
	from := normalizeName(fromNode)
	to := normalizeName(toNode)
	if from == "" || to == "" {
		return
	}
	degree[from]++
	degree[to]++
	if graph[from] == nil {
		graph[from] = map[string]bool{}
	}
	if graph[to] == nil {
		graph[to] = map[string]bool{}
	}
	graph[from][to] = true
	graph[to][from] = true
}

func graphComponentCount(graph map[string]map[string]bool) int {
	visited := map[string]bool{}
	components := 0
	for node := range graph {
		if visited[node] {
			continue
		}
		components++
		stack := []string{node}
		visited[node] = true
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for next := range graph[current] {
				if visited[next] {
					continue
				}
				visited[next] = true
				stack = append(stack, next)
			}
		}
	}
	return components
}

func diagnosticForObject(severity, category, code string, obj Object, message string) Diagnostic {
	return Diagnostic{
		Severity:    severity,
		Category:    category,
		Code:        code,
		Message:     message,
		ObjectIndex: obj.Index,
		ObjectType:  obj.Type,
		ObjectName:  objectName(obj),
	}
}

func (diagnostic Diagnostic) withSource(source string, evidence string) Diagnostic {
	diagnostic.Source = source
	if diagnostic.Confidence == "" {
		diagnostic.Confidence = diagnosticConfidenceForSource(source)
	}
	if diagnostic.Evidence == "" {
		diagnostic.Evidence = evidence
	}
	return diagnostic
}

func diagnosticConfidenceForSource(source string) string {
	switch source {
	case "energyplus_rule":
		return "high"
	case "simulation_context":
		return "medium"
	case "heuristic_inference":
		return "low"
	default:
		return "medium"
	}
}

func diagnosticSeverityRank(severity string) int {
	switch severity {
	case DiagnosticError:
		return 0
	case DiagnosticWarning:
		return 1
	case DiagnosticNotice:
		return 2
	default:
		return 3
	}
}

func mergeDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	seen := map[string]int{}
	var out []Diagnostic
	for _, diagnostic := range diagnostics {
		key := strings.Join([]string{
			diagnostic.Code,
			fmt.Sprint(diagnostic.ObjectIndex),
			fmt.Sprint(diagnostic.FieldIndex),
			normalizeName(diagnostic.Value),
		}, "|")
		if index, ok := seen[key]; ok {
			existing := out[index]
			if diagnosticSeverityRank(diagnostic.Severity) < diagnosticSeverityRank(existing.Severity) ||
				(existing.Source != "energyplus_rule" && diagnostic.Source == "energyplus_rule") {
				out[index] = diagnostic
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, diagnostic)
	}
	return out
}

func hasSizingPeriodOnlyContext(doc Document) bool {
	for _, obj := range doc.Objects {
		lower := normalizeFieldCatalogKey(obj.Type)
		if strings.HasPrefix(lower, "sizingperiod:") || strings.Contains(lower, "designday") {
			return true
		}
	}
	return false
}

func diagnosticFieldReferenceKind(obj Object, fieldIndex int, field Field) string {
	role := catalogFieldRole(obj, fieldIndex)
	switch role {
	case fieldRoleScheduleRef:
		return "missing_schedule_reference"
	case fieldRoleConstructionRef:
		return "missing_construction_reference"
	case fieldRoleZoneRef, fieldRoleSpaceRef, fieldRoleZoneSpaceRef, fieldRoleZoneSpaceTarget, fieldRoleZoneListRef:
		return "missing_zone_reference"
	}
	if role != "" {
		return ""
	}
	for _, kind := range diagnosticReferenceKinds {
		if kind.fieldMatch(field.Comment) {
			return kind.code
		}
	}
	return ""
}

func isTerminalHVACBoundaryNode(usage HVACNodeUsage) bool {
	role := strings.ToLower(usage.Role)
	return strings.Contains(role, "outdoor") ||
		strings.Contains(role, "relief") ||
		strings.Contains(role, "exhaust") ||
		strings.Contains(role, "zone_air") ||
		strings.Contains(role, "setpoint")
}

func commentHasWords(words ...string) func(string) bool {
	return func(comment string) bool {
		comment = strings.ToLower(comment)
		for _, word := range words {
			if !strings.Contains(comment, strings.ToLower(word)) {
				return false
			}
		}
		return true
	}
}

func isZoneLikeType(objectType string) bool {
	return strings.EqualFold(objectType, "Zone") || strings.EqualFold(objectType, "Space")
}

func isZoneReferenceTargetType(objectType string) bool {
	return isZoneLikeType(objectType) || strings.EqualFold(objectType, "ZoneList")
}

func isMaterialType(objectType string) bool {
	lower := strings.ToLower(objectType)
	return strings.HasPrefix(lower, "material") || strings.HasPrefix(lower, "windowmaterial")
}

func objectLabel(obj Object) string {
	if name := objectName(obj); name != "" {
		return name
	}
	return fmt.Sprintf("#%d", obj.Index+1)
}
