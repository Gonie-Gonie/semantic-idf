package idf

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
)

type ThermalTopologyProjectionOptions struct {
	Level            string `json:"level"`
	Metric           string `json:"metric"`
	Scope            string `json:"scope"`
	AreaBasis        string `json:"areaBasis"`
	StoryIndex       *int   `json:"storyIndex,omitempty"`
	SelectedEntityID string `json:"selectedEntityId,omitempty"`
	NeighborDepth    int    `json:"neighborDepth,omitempty"`
}

type ThermalTopologyGraphProjection struct {
	Schema          string                           `json:"schema"`
	SourceSchema    string                           `json:"sourceSchema"`
	SourceModelHash string                           `json:"sourceModelHash"`
	Options         ThermalTopologyProjectionOptions `json:"options"`
	Nodes           []ThermalTopologyGraphNode       `json:"nodes"`
	Edges           []ThermalTopologyGraphEdge       `json:"edges"`
}

type ThermalTopologyGraphNode struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	StoryIndex int    `json:"storyIndex,omitempty"`
}

type ThermalTopologyGraphEdge struct {
	ID             string   `json:"id"`
	FromNodeID     string   `json:"fromNodeId"`
	ToNodeID       string   `json:"toNodeId"`
	RelationKind   string   `json:"relationKind"`
	Metric         string   `json:"metric"`
	Value          float64  `json:"value,omitempty"`
	Unit           string   `json:"unit,omitempty"`
	ValueAvailable bool     `json:"valueAvailable"`
	BoundaryIDs    []string `json:"boundaryIds,omitempty"`
}

func NormalizeThermalTopologyProjectionOptions(options ThermalTopologyProjectionOptions) (ThermalTopologyProjectionOptions, error) {
	options.Level = strings.ToLower(strings.TrimSpace(options.Level))
	if options.Level == "" {
		options.Level = "zone"
	}
	if options.Level != "zone" && options.Level != "boundary" {
		return options, fmt.Errorf("unsupported topology level %q", options.Level)
	}
	options.Metric = strings.ToLower(strings.TrimSpace(options.Metric))
	if options.Metric == "" {
		options.Metric = "topology"
	}
	if !thermalTopologyStringIn(options.Metric, "topology", "area", "ua", "exposure", "qa", "air") {
		return options, fmt.Errorf("unsupported topology metric %q", options.Metric)
	}
	options.Scope = strings.ToLower(strings.TrimSpace(options.Scope))
	if options.Scope == "" {
		options.Scope = "building"
	}
	if !thermalTopologyStringIn(options.Scope, "building", "story", "selection", "neighbors") {
		return options, fmt.Errorf("unsupported topology scope %q", options.Scope)
	}
	options.AreaBasis = strings.ToLower(strings.TrimSpace(options.AreaBasis))
	if options.AreaBasis == "" {
		options.AreaBasis = "effective"
	}
	if options.AreaBasis != "effective" && options.AreaBasis != "physical" {
		return options, fmt.Errorf("unsupported topology area basis %q", options.AreaBasis)
	}
	if options.NeighborDepth <= 0 {
		options.NeighborDepth = 1
	}
	if options.NeighborDepth > 3 {
		options.NeighborDepth = 3
	}
	if (options.Scope == "selection" || options.Scope == "neighbors") && strings.TrimSpace(options.SelectedEntityID) == "" {
		return options, fmt.Errorf("topology scope %q requires a selected entity ID", options.Scope)
	}
	return options, nil
}

func ExportThermalTopologyJSON(report ThermalTopologyReport, options ThermalTopologyProjectionOptions) ([]byte, error) {
	normalized, err := NormalizeThermalTopologyProjectionOptions(options)
	if err != nil {
		return nil, err
	}
	report.AreaBasis = normalized.AreaBasis
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func ProjectThermalTopologyGraph(report ThermalTopologyReport, options ThermalTopologyProjectionOptions) (ThermalTopologyGraphProjection, error) {
	options, err := NormalizeThermalTopologyProjectionOptions(options)
	if err != nil {
		return ThermalTopologyGraphProjection{}, err
	}
	projection := ThermalTopologyGraphProjection{
		Schema:          "semantic-idf.thermal-topology-graph/v1",
		SourceSchema:    report.Schema,
		SourceModelHash: report.SourceModelHash,
		Options:         options,
	}
	visibleNodeIDs := thermalTopologyProjectionNodeIDs(report, options)
	for _, node := range report.Nodes {
		if visibleNodeIDs[node.ID] {
			projection.Nodes = append(projection.Nodes, ThermalTopologyGraphNode{ID: node.ID, Kind: node.Kind, Label: node.Label, StoryIndex: node.StoryIndex})
		}
	}
	if options.Level == "boundary" {
		projection = projectThermalTopologyBoundaries(report, projection, visibleNodeIDs, options)
	} else {
		projection = projectThermalTopologyConnections(report, projection, visibleNodeIDs, options)
	}
	sort.Slice(projection.Nodes, func(i, j int) bool { return projection.Nodes[i].ID < projection.Nodes[j].ID })
	sort.Slice(projection.Edges, func(i, j int) bool { return projection.Edges[i].ID < projection.Edges[j].ID })
	return projection, nil
}

func ExportThermalTopologyGraphML(report ThermalTopologyReport, options ThermalTopologyProjectionOptions) ([]byte, error) {
	projection, err := ProjectThermalTopologyGraph(report, options)
	if err != nil {
		return nil, err
	}
	var builder strings.Builder
	builder.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	builder.WriteString("<graphml xmlns=\"http://graphml.graphdrawing.org/xmlns\">\n")
	builder.WriteString("  <key id=\"kind\" for=\"node\" attr.name=\"kind\" attr.type=\"string\"/>\n")
	builder.WriteString("  <key id=\"label\" for=\"all\" attr.name=\"label\" attr.type=\"string\"/>\n")
	builder.WriteString("  <key id=\"relation\" for=\"edge\" attr.name=\"relation\" attr.type=\"string\"/>\n")
	builder.WriteString("  <key id=\"metric\" for=\"edge\" attr.name=\"metric\" attr.type=\"string\"/>\n")
	builder.WriteString("  <key id=\"value\" for=\"edge\" attr.name=\"value\" attr.type=\"double\"/>\n")
	builder.WriteString("  <key id=\"unit\" for=\"edge\" attr.name=\"unit\" attr.type=\"string\"/>\n")
	builder.WriteString("  <graph id=\"thermal-topology\" edgedefault=\"undirected\">\n")
	for _, node := range projection.Nodes {
		fmt.Fprintf(&builder, "    <node id=\"%s\"><data key=\"kind\">%s</data><data key=\"label\">%s</data></node>\n", thermalTopologyXMLEscape(node.ID), thermalTopologyXMLEscape(node.Kind), thermalTopologyXMLEscape(node.Label))
	}
	for _, edge := range projection.Edges {
		fmt.Fprintf(&builder, "    <edge id=\"%s\" source=\"%s\" target=\"%s\"><data key=\"relation\">%s</data><data key=\"metric\">%s</data>", thermalTopologyXMLEscape(edge.ID), thermalTopologyXMLEscape(edge.FromNodeID), thermalTopologyXMLEscape(edge.ToNodeID), thermalTopologyXMLEscape(edge.RelationKind), thermalTopologyXMLEscape(edge.Metric))
		if edge.ValueAvailable {
			fmt.Fprintf(&builder, "<data key=\"value\">%g</data><data key=\"unit\">%s</data>", edge.Value, thermalTopologyXMLEscape(edge.Unit))
		}
		builder.WriteString("</edge>\n")
	}
	builder.WriteString("  </graph>\n</graphml>\n")
	return []byte(builder.String()), nil
}

func ExportThermalTopologyDOT(report ThermalTopologyReport, options ThermalTopologyProjectionOptions) ([]byte, error) {
	projection, err := ProjectThermalTopologyGraph(report, options)
	if err != nil {
		return nil, err
	}
	var builder strings.Builder
	builder.WriteString("graph thermal_topology {\n")
	builder.WriteString("  graph [overlap=false, splines=true];\n")
	for _, node := range projection.Nodes {
		fmt.Fprintf(&builder, "  %s [label=%s, kind=%s];\n", strconv.Quote(node.ID), strconv.Quote(node.Label), strconv.Quote(node.Kind))
	}
	for _, edge := range projection.Edges {
		label := strings.ReplaceAll(edge.RelationKind, "_", " ")
		if edge.ValueAvailable {
			label += fmt.Sprintf(" | %.4g %s", edge.Value, edge.Unit)
		}
		fmt.Fprintf(&builder, "  %s -- %s [id=%s, label=%s];\n", strconv.Quote(edge.FromNodeID), strconv.Quote(edge.ToNodeID), strconv.Quote(edge.ID), strconv.Quote(label))
	}
	builder.WriteString("}\n")
	return []byte(builder.String()), nil
}

func projectThermalTopologyConnections(report ThermalTopologyReport, projection ThermalTopologyGraphProjection, visible map[string]bool, options ThermalTopologyProjectionOptions) ThermalTopologyGraphProjection {
	for _, connection := range report.Connections {
		if !visible[connection.FromNodeID] || !visible[connection.ToNodeID] {
			continue
		}
		value, unit, available := thermalTopologyConnectionMetric(connection, options)
		projection.Edges = append(projection.Edges, ThermalTopologyGraphEdge{
			ID: connection.ID, FromNodeID: connection.FromNodeID, ToNodeID: connection.ToNodeID,
			RelationKind: connection.RelationKind, Metric: options.Metric, Value: value, Unit: unit,
			ValueAvailable: available, BoundaryIDs: append([]string(nil), connection.BoundaryIDs...),
		})
	}
	return projection
}

func projectThermalTopologyBoundaries(report ThermalTopologyReport, projection ThermalTopologyGraphProjection, visible map[string]bool, options ThermalTopologyProjectionOptions) ThermalTopologyGraphProjection {
	addedNodes := map[string]bool{}
	for _, node := range projection.Nodes {
		addedNodes[node.ID] = true
	}
	for _, boundary := range report.Boundaries {
		if !visible[boundary.OwnerZoneID] && !visible[boundary.OwnerSpaceID] && !visible[boundary.TargetID] {
			continue
		}
		ownerID := firstNonEmpty(boundary.OwnerSpaceID, boundary.OwnerZoneID)
		if ownerID == "" || boundary.TargetID == "" {
			continue
		}
		if !addedNodes[boundary.ID] {
			projection.Nodes = append(projection.Nodes, ThermalTopologyGraphNode{ID: boundary.ID, Kind: "thermal_boundary", Label: boundary.SurfaceName})
			addedNodes[boundary.ID] = true
		}
		value, unit, available := thermalTopologyBoundaryMetric(boundary, options)
		projection.Edges = append(projection.Edges,
			ThermalTopologyGraphEdge{ID: "thermal-boundary-owner:" + boundary.ID, FromNodeID: ownerID, ToNodeID: boundary.ID, RelationKind: "boundary_source", Metric: options.Metric, Value: value, Unit: unit, ValueAvailable: available, BoundaryIDs: []string{boundary.ID}},
			ThermalTopologyGraphEdge{ID: "thermal-boundary-target:" + boundary.ID, FromNodeID: boundary.ID, ToNodeID: boundary.TargetID, RelationKind: boundary.RelationKind, Metric: options.Metric, Value: value, Unit: unit, ValueAvailable: available, BoundaryIDs: []string{boundary.ID}},
		)
	}
	return projection
}

func thermalTopologyProjectionNodeIDs(report ThermalTopologyReport, options ThermalTopologyProjectionOptions) map[string]bool {
	visible := map[string]bool{}
	if options.Scope == "building" {
		for _, node := range report.Nodes {
			visible[node.ID] = true
		}
		return visible
	}
	if options.Scope == "story" {
		storyIndex := 0
		if options.StoryIndex != nil {
			storyIndex = *options.StoryIndex
		}
		for _, node := range report.Nodes {
			if (node.Kind == "zone" || node.Kind == "space") && node.StoryIndex == storyIndex {
				visible[node.ID] = true
			}
		}
		thermalTopologyExpandConnectedNodeIDs(report, visible, 1)
		return visible
	}
	selected := strings.TrimSpace(options.SelectedEntityID)
	for _, node := range report.Nodes {
		if node.ID == selected || node.EntityID == selected {
			visible[node.ID] = true
		}
	}
	for _, connection := range report.Connections {
		if connection.ID == selected {
			visible[connection.FromNodeID] = true
			visible[connection.ToNodeID] = true
		}
	}
	for _, boundary := range report.Boundaries {
		if boundary.ID == selected || boundary.SurfaceID == selected || boundary.SurfaceEntityID == selected {
			visible[firstNonEmpty(boundary.OwnerSpaceID, boundary.OwnerZoneID)] = true
			visible[boundary.TargetID] = true
		}
	}
	depth := 0
	if options.Scope == "neighbors" {
		depth = options.NeighborDepth
	}
	thermalTopologyExpandConnectedNodeIDs(report, visible, depth)
	return visible
}

func thermalTopologyExpandConnectedNodeIDs(report ThermalTopologyReport, visible map[string]bool, depth int) {
	for step := 0; step < depth; step++ {
		next := map[string]bool{}
		for _, connection := range report.Connections {
			if visible[connection.FromNodeID] || visible[connection.ToNodeID] {
				next[connection.FromNodeID] = true
				next[connection.ToNodeID] = true
			}
		}
		for id := range next {
			visible[id] = true
		}
	}
	for _, connection := range report.Connections {
		if visible[connection.FromNodeID] && !visible[connection.ToNodeID] {
			visible[connection.ToNodeID] = true
		}
		if visible[connection.ToNodeID] && !visible[connection.FromNodeID] {
			visible[connection.FromNodeID] = true
		}
	}
}

func thermalTopologyConnectionMetric(connection ThermalConnectionAggregate, options ThermalTopologyProjectionOptions) (float64, string, bool) {
	switch options.Metric {
	case "area", "exposure":
		if options.AreaBasis == "physical" {
			return connection.PhysicalGrossArea, "m2", true
		}
		return connection.EffectiveGrossArea, "m2", true
	case "ua":
		if options.AreaBasis == "physical" {
			return connection.PhysicalTotalUA, "W/K", connection.HasPhysicalUA
		}
		return connection.TotalUA, "W/K", connection.HasUA
	case "qa":
		return float64(len(connection.DiagnosticIDs)), "issues", true
	case "air":
		return float64(len(connection.AirCouplingIDs)), "couplings", len(connection.AirCouplingIDs) > 0
	default:
		return float64(maxThermalTopologyInt(connection.SurfaceCount, len(connection.BoundaryIDs))), "boundaries", true
	}
}

func thermalTopologyBoundaryMetric(boundary ThermalBoundaryRecord, options ThermalTopologyProjectionOptions) (float64, string, bool) {
	switch options.Metric {
	case "area", "exposure":
		if options.AreaBasis == "physical" {
			return boundary.PhysicalGrossArea, "m2", true
		}
		return boundary.EffectiveGrossArea, "m2", true
	case "ua":
		return boundary.TotalUA, "W/K", boundary.HasUA
	case "qa":
		return float64(len(boundary.DiagnosticIDs)), "issues", true
	case "air":
		return 0, "couplings", false
	default:
		return 1, "boundary", true
	}
}

func thermalTopologyXMLEscape(value string) string {
	return html.EscapeString(value)
}

func thermalTopologyStringIn(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func maxThermalTopologyInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
