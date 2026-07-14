package idf

import (
	"strings"
)

const semanticYAMLSchema = "eplus-semantic/0.2"

type SemanticYAMLMetadata struct {
	EnergyPlusVersion string
	SourceFormat      string
}

type SemanticYAMLProjection struct {
	Schema                string                       `json:"schema"`
	EnergyPlusVersion     string                       `json:"energyplusVersion,omitempty"`
	SourceFormat          string                       `json:"sourceFormat,omitempty"`
	Text                  string                       `json:"text"`
	Lines                 []SemanticYAMLLine           `json:"lines"`
	Navigation            SemanticNavigationIndex      `json:"navigation"`
	BasicVisibleLineCount int                          `json:"basicVisibleLineCount"`
	SourceNameConflicts   []SemanticSourceNameConflict `json:"sourceNameConflicts,omitempty"`
	ObjectCount           int                          `json:"objectCount"`
}

type SemanticYAMLLine struct {
	Text         string `json:"text"`
	Indent       int    `json:"indent"`
	Key          string `json:"key,omitempty"`
	Value        string `json:"value,omitempty"`
	DisplayValue string `json:"displayValue,omitempty"`
	PatchValue   string `json:"patchValue,omitempty"`
	SourceValue  string `json:"sourceValue,omitempty"`

	ObjectIndex *int   `json:"objectIndex,omitempty"`
	ObjectType  string `json:"objectType,omitempty"`
	ObjectName  string `json:"objectName,omitempty"`
	FieldIndex  *int   `json:"fieldIndex,omitempty"`

	SourceKind string `json:"sourceKind,omitempty"`
	EditKind   string `json:"editKind,omitempty"`
	Editable   bool   `json:"editable,omitempty"`
	Role       string `json:"role,omitempty"`

	EntityID          string                `json:"entityId,omitempty"`
	EntityKind        string                `json:"entityKind,omitempty"`
	OccurrenceID      string                `json:"occurrenceId,omitempty"`
	SemanticPath      string                `json:"semanticPath,omitempty"`
	SourceAnchor      *SemanticSourceAnchor `json:"sourceAnchor,omitempty"`
	ViewTargets       []SemanticViewTarget  `json:"viewTargets,omitempty"`
	PreferredView     string                `json:"preferredView,omitempty"`
	PreferredTargetID string                `json:"preferredTargetId,omitempty"`
}

type SemanticSourceNameConflict struct {
	Group         string `json:"group"`
	ObjectType    string `json:"objectType"`
	Name          string `json:"name"`
	ObjectIndexes []int  `json:"objectIndexes"`
	SyncPolicy    string `json:"syncPolicy"`
	AutoFixable   bool   `json:"autoFixable"`
}

type SemanticDuplicateFix struct {
	ObjectIndex int    `json:"objectIndex"`
	ObjectType  string `json:"objectType"`
	Before      string `json:"before"`
	After       string `json:"after"`
}

type SemanticModel struct {
	Schema            string
	EnergyPlusVersion string
	SourceFormat      string
	Source            SemanticModelSource
	Nodes             []SemanticYAMLNode
	Navigation        SemanticNavigationIndex
}

type SemanticYAMLNode struct {
	Indent       int
	Raw          string
	Key          string
	ListItem     bool
	DisplayValue string
	PatchValue   string
	SourceValue  string

	ObjectIndex *int
	ObjectType  string
	ObjectName  string
	FieldIndex  *int

	SourceKind string
	EditKind   string
	Editable   bool
	Role       string

	EntityID          string
	EntityKind        string
	OccurrenceID      string
	SemanticPath      string
	SourceAnchor      *SemanticSourceAnchor
	ViewTargets       []SemanticViewTarget
	PreferredView     string
	PreferredTargetID string
}

type SemanticModelSource struct {
	ObjectCount       int
	NameConflicts     []SemanticSourceNameConflict
	OccurrenceIndex   map[int][]SemanticOccurrence
	ProjectedObjects  map[int]bool
	UnshownFieldCount map[int]int
}

type SemanticOccurrence struct {
	OccurrenceID      string                `json:"occurrenceId"`
	EntityID          string                `json:"entityId"`
	SourceObjectID    string                `json:"sourceObjectId,omitempty"`
	Path              string                `json:"path"`
	RoleHere          string                `json:"roleHere,omitempty"`
	ContextKind       string                `json:"contextKind,omitempty"`
	PreferredView     string                `json:"preferredView,omitempty"`
	PreferredTargetID string                `json:"preferredTargetId,omitempty"`
	Class             string                `json:"class,omitempty"`
	Name              string                `json:"name,omitempty"`
	SourceAnchor      *SemanticSourceAnchor `json:"sourceAnchor,omitempty"`
	ViewTargets       []SemanticViewTarget  `json:"viewTargets,omitempty"`
	LineIndexes       []int                 `json:"lineIndexes,omitempty"`
}

func BuildSemanticYAMLProjection(doc Document, metadata SemanticYAMLMetadata) SemanticYAMLProjection {
	model := BuildSemanticModel(doc, metadata)
	lines := BuildSemanticLines(model)
	return SemanticYAMLProjection{
		Schema:                model.Schema,
		EnergyPlusVersion:     model.EnergyPlusVersion,
		SourceFormat:          model.SourceFormat,
		Text:                  RenderSemanticYAML(model),
		Lines:                 lines,
		Navigation:            model.Navigation,
		BasicVisibleLineCount: len(basicSemanticYAMLLines(lines)),
		SourceNameConflicts:   model.Source.NameConflicts,
		ObjectCount:           model.Source.ObjectCount,
	}
}

func basicSemanticYAMLLines(lines []SemanticYAMLLine) []SemanticYAMLLine {
	hiddenBlocks := map[string]bool{
		"duplicated_as":       true,
		"also_shown_in":       true,
		"sync_policy":         true,
		"source_relations":    true,
		"source_preservation": true,
		"raw":                 true,
		"computed":            true,
		"vertices":            true,
	}
	keepKeys := map[string]bool{
		"schema":          true,
		"name":            true,
		"class":           true,
		"type":            true,
		"family":          true,
		"family_label":    true,
		"display_label":   true,
		"role_here":       true,
		"source":          true,
		"confidence":      true,
		"status":          true,
		"value":           true,
		"zone":            true,
		"space":           true,
		"schedule":        true,
		"air_loop":        true,
		"plant_loop":      true,
		"air_loops":       true,
		"plant_loops":     true,
		"condenser_loops": true,
		"terminal_units":  true,
		"zone_equipment":  true,
		"outputs":         true,
		"diagnostics":     true,
	}
	var out []SemanticYAMLLine
	hideUntilIndent := -1
	for _, line := range lines {
		indent := line.Indent
		if hideUntilIndent >= 0 && indent > hideUntilIndent {
			continue
		}
		hideUntilIndent = -1
		key := strings.TrimSpace(line.Key)
		if hiddenBlocks[key] {
			hideUntilIndent = indent
			continue
		}
		text := strings.TrimLeft(line.Text, " \t")
		if semanticYAMLBasicKeepsSyntax(line) {
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(text, "- name:") && indent <= 4 && semanticYAMLBasicKeepsObjectName(line) {
			out = append(out, line)
			continue
		}
		if semanticYAMLLineHasValue(line) && indent <= 4 && keepKeys[key] && semanticYAMLBasicKeepsValueLine(line) {
			out = append(out, line)
		}
	}
	return out
}

func semanticYAMLBasicKeepsSyntax(line SemanticYAMLLine) bool {
	if line.Text == "semantic_energyplus_model:" {
		return true
	}
	if line.Role != "syntax" {
		return false
	}
	if line.Indent <= 1 {
		return true
	}
	if line.Indent != 2 {
		return false
	}
	key := semanticYAMLLineKeyToken(line)
	switch key {
	case "definitions", "zones", "air_loops", "plant_loops", "condenser_loops", "zone_relations", "files", "variables", "meters", "diagnostics":
		return true
	default:
		return false
	}
}

func semanticYAMLBasicKeepsObjectName(line SemanticYAMLLine) bool {
	objectType := strings.ToLower(strings.TrimSpace(line.ObjectType))
	switch {
	case objectType == "zone" || objectType == "space":
		return true
	case objectType == "airloophvac" || objectType == "plantloop" || objectType == "condenserloop":
		return true
	default:
		return false
	}
}

func semanticYAMLBasicKeepsValueLine(line SemanticYAMLLine) bool {
	if line.SourceKind == "summary" && line.Indent <= 2 {
		return true
	}
	if semanticYAMLBasicKeepsObjectName(line) {
		return true
	}
	key := semanticYAMLLineKeyToken(line)
	switch key {
	case "source", "confidence", "status", "value", "air_loops", "plant_loops", "condenser_loops", "terminal_units", "zone_equipment", "outputs", "diagnostics":
		return line.Indent <= 4
	default:
		return false
	}
}

func semanticYAMLLineKeyToken(line SemanticYAMLLine) string {
	if key := strings.TrimSpace(line.Key); key != "" {
		return key
	}
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line.Text), "- "))
	if index := strings.Index(text, ":"); index >= 0 {
		return strings.TrimSpace(text[:index])
	}
	return text
}

func semanticYAMLLineHasValue(line SemanticYAMLLine) bool {
	return strings.TrimSpace(line.Key) != "" &&
		(line.Editable ||
			line.DisplayValue != "" ||
			line.Value != "" ||
			line.Role == "metadata" ||
			line.Role == "object" ||
			line.Role == "field")
}

func BuildSemanticModel(doc Document, metadata SemanticYAMLMetadata) SemanticModel {
	model := SemanticModel{
		Schema:            semanticYAMLSchema,
		EnergyPlusVersion: strings.TrimSpace(metadata.EnergyPlusVersion),
		SourceFormat:      strings.TrimSpace(metadata.SourceFormat),
		Source: SemanticModelSource{
			ObjectCount:       len(doc.Objects),
			NameConflicts:     semanticSourceNameConflicts(doc),
			OccurrenceIndex:   map[int][]SemanticOccurrence{},
			ProjectedObjects:  map[int]bool{},
			UnshownFieldCount: map[int]int{},
		},
	}
	ctx := buildSemanticContext(doc, metadata)
	builder := &semanticYAMLBuilder{model: &model, ctx: ctx}
	buildSemanticProjectionNodes(builder, ctx, metadata)
	buildSemanticNavigation(doc, ctx, &model)
	for objectIndex := range ctx.mapped {
		model.Source.ProjectedObjects[objectIndex] = true
		model.Source.UnshownFieldCount[objectIndex] = semanticUnshownFieldCount(ctx, objectIndex)
	}
	return model
}
