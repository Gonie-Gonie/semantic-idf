package epinput

import (
	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

func ParseIDF(content string) (*Model, error) {
	doc, err := idf.Parse(content)
	if err != nil {
		return nil, err
	}
	model := FromIDFDocument(doc, FormatIDF)
	if err := EnsureSupportedVersion(model); err != nil {
		return nil, err
	}
	return model, nil
}

func FromIDFDocument(doc idf.Document, format Format) *Model {
	model := &Model{Format: format}
	typeCounts := map[string]int{}

	for _, object := range doc.Objects {
		used := map[string]int{}
		fields := make([]Field, 0, len(object.Fields))
		for fieldIndex, field := range object.Fields {
			fields = append(fields, Field{
				Key:     fieldKey(field.Comment, fieldIndex, used),
				Value:   field.Value,
				Comment: field.Comment,
			})
		}

		typeIndex := typeCounts[object.Type]
		typeCounts[object.Type]++
		name, remaining, nameSource := objectInstanceName(object.Type, fields, typeIndex)
		remaining = collapseVertexArrayFields(object.Type, remaining)
		model.Objects = append(model.Objects, InputObject{
			Type:        object.Type,
			Name:        name,
			NameSource:  nameSource,
			Fields:      remaining,
			Metadata:    map[string]any{"idf_order": object.Index + 1},
			SourceIndex: object.Index,
		})
	}

	model.Version = DetectVersion(model.Objects)
	return model
}

func ToIDFDocument(model *Model) idf.Document {
	if model == nil {
		return idf.Document{}
	}

	doc := idf.Document{Objects: make([]idf.Object, 0, len(model.Objects))}
	for objectIndex, object := range model.Objects {
		fields := make([]idf.Field, 0, len(object.Fields)+1)
		if shouldWriteObjectName(object) {
			fields = append(fields, idf.Field{Value: object.Name, Comment: "Name"})
		}
		for _, field := range object.Fields {
			if expanded, ok := expandVertexArrayField(object.Type, field); ok {
				fields = append(fields, expanded...)
				continue
			}
			comment := field.Comment
			if comment == "" {
				comment = keyToComment(field.Key)
			}
			fields = append(fields, idf.Field{
				Value:   valueToString(field.Value),
				Comment: comment,
			})
		}
		doc.Objects = append(doc.Objects, idf.Object{
			Index:  objectIndex,
			Type:   object.Type,
			Fields: fields,
		})
	}
	return doc
}

func WriteDocumentLikeOriginal(doc idf.Document, original *Model) string {
	if original != nil && original.Format == FormatEPJSON {
		model := FromIDFDocument(doc, FormatEPJSON)
		output, err := Write(model, FormatEPJSON)
		if err == nil {
			return output
		}
	}
	return doc.String()
}

func WriteIDF(model *Model) string {
	return ToIDFDocument(model).String()
}

func shouldWriteObjectName(object InputObject) bool {
	if isNamelessObjectType(object.Type) {
		return false
	}
	return object.NameSource == NameSourceExplicitField || object.NameSource == NameSourceEPJSONInstance
}
