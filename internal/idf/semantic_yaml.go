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
	Schema              string                       `json:"schema"`
	EnergyPlusVersion   string                       `json:"energyplusVersion,omitempty"`
	SourceFormat        string                       `json:"sourceFormat,omitempty"`
	Text                string                       `json:"text"`
	Lines               []SemanticYAMLLine           `json:"lines"`
	SourceNameConflicts []SemanticSourceNameConflict `json:"sourceNameConflicts,omitempty"`
	ObjectCount         int                          `json:"objectCount"`
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
}

type SemanticModelSource struct {
	ObjectCount       int
	NameConflicts     []SemanticSourceNameConflict
	OccurrenceIndex   map[int][]SemanticOccurrence
	ProjectedObjects  map[int]bool
	UnshownFieldCount map[int]int
}

type SemanticOccurrence struct {
	OccurrenceID   string
	SourceObjectID string
	Path           string
	RoleHere       string
	Class          string
	Name           string
}

func BuildSemanticYAMLProjection(doc Document, metadata SemanticYAMLMetadata) SemanticYAMLProjection {
	model := BuildSemanticModel(doc, metadata)
	return SemanticYAMLProjection{
		Schema:              model.Schema,
		EnergyPlusVersion:   model.EnergyPlusVersion,
		SourceFormat:        model.SourceFormat,
		Text:                RenderSemanticYAML(model),
		Lines:               BuildSemanticLines(model),
		SourceNameConflicts: model.Source.NameConflicts,
		ObjectCount:         model.Source.ObjectCount,
	}
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
	for objectIndex := range ctx.mapped {
		model.Source.ProjectedObjects[objectIndex] = true
		model.Source.UnshownFieldCount[objectIndex] = semanticUnshownFieldCount(ctx, objectIndex)
	}
	model.Source.OccurrenceIndex = builder.occurrences
	return model
}
