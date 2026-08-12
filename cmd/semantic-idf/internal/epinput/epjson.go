package epinput

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type orderedMember struct {
	Key   string
	Value any
}

type orderedObject []orderedMember

func (object orderedObject) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	writeJSONValue(&b, object)
	return []byte(b.String()), nil
}

func ParseEPJSON(content []byte) (*Model, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	value, err := parseOrderedJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, err
	}

	root, ok := value.(orderedObject)
	if !ok {
		return nil, fmt.Errorf("epJSON input must be a JSON object")
	}

	model := &Model{Format: FormatEPJSON}
	objectIndex := 0
	for _, typeMember := range root {
		instances, ok := typeMember.Value.(orderedObject)
		if !ok {
			return nil, fmt.Errorf("epJSON object group %q must contain named objects", typeMember.Key)
		}
		for _, instanceMember := range instances {
			instanceFields, ok := instanceMember.Value.(orderedObject)
			if !ok {
				return nil, fmt.Errorf("epJSON object %q/%q must contain field object", typeMember.Key, instanceMember.Key)
			}
			fields, metadata := parseEPJSONFields(instanceFields)
			model.Objects = append(model.Objects, InputObject{
				Type:        typeMember.Key,
				Name:        instanceMember.Key,
				NameSource:  NameSourceEPJSONInstance,
				Fields:      fields,
				Metadata:    metadata,
				SourceIndex: sourceIndexFromMetadata(metadata, objectIndex),
			})
			objectIndex++
		}
	}

	sort.SliceStable(model.Objects, func(i, j int) bool {
		return model.Objects[i].SourceIndex < model.Objects[j].SourceIndex
	})
	model.Version = DetectVersion(model.Objects)
	if err := EnsureSupportedVersion(model); err != nil {
		return nil, err
	}
	return model, nil
}

func parseEPJSONFields(values orderedObject) ([]Field, map[string]any) {
	var fields []Field
	metadata := map[string]any{}
	for _, member := range values {
		if strings.HasPrefix(member.Key, "idf_") {
			metadata[member.Key] = member.Value
			continue
		}
		fields = append(fields, Field{
			Key:     member.Key,
			Value:   member.Value,
			Comment: keyToComment(member.Key),
		})
	}
	if len(metadata) == 0 {
		metadata = nil
	}
	return fields, metadata
}

func sourceIndexFromMetadata(metadata map[string]any, fallback int) int {
	if metadata == nil {
		return fallback
	}
	switch v := metadata["idf_order"].(type) {
	case json.Number:
		if order, err := v.Int64(); err == nil && order > 0 {
			return int(order) - 1
		}
	case float64:
		if v > 0 {
			return int(v) - 1
		}
	case int:
		if v > 0 {
			return v - 1
		}
	}
	return fallback
}

func WriteEPJSON(model *Model) (string, error) {
	if model == nil {
		return "{}\n", nil
	}

	grouped := map[string][]InputObject{}
	typeOrder := make([]string, 0)
	for _, object := range model.Objects {
		if _, ok := grouped[object.Type]; !ok {
			typeOrder = append(typeOrder, object.Type)
		}
		grouped[object.Type] = append(grouped[object.Type], object)
	}

	var b strings.Builder
	b.WriteString("{\n")
	for typeIndex, objectType := range typeOrder {
		if typeIndex > 0 {
			b.WriteString(",\n")
		}
		writeJSONString(&b, objectType)
		b.WriteString(": {\n")

		objects := grouped[objectType]
		for objectIndex, object := range objects {
			if objectIndex > 0 {
				b.WriteString(",\n")
			}
			b.WriteString("    ")
			writeJSONString(&b, jsonObjectName(object, objectIndex))
			b.WriteString(": {\n")
			writeEPJSONFields(&b, object)
			b.WriteString("\n    }")
		}
		b.WriteString("\n  }")
	}
	b.WriteString("\n}\n")
	return b.String(), nil
}

func writeEPJSONFields(b *strings.Builder, object InputObject) {
	wrote := false
	for _, field := range object.Fields {
		if wrote {
			b.WriteString(",\n")
		}
		b.WriteString("      ")
		writeJSONString(b, field.Key)
		b.WriteString(": ")
		writeJSONValue(b, field.Value)
		wrote = true
	}

	metadata := object.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	if _, ok := metadata["idf_order"]; !ok {
		metadata["idf_order"] = object.SourceIndex + 1
	}

	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if wrote {
			b.WriteString(",\n")
		}
		b.WriteString("      ")
		writeJSONString(b, key)
		b.WriteString(": ")
		writeJSONValue(b, metadata[key])
		wrote = true
	}
}

func jsonObjectName(object InputObject, fallbackIndex int) string {
	if object.Name != "" {
		return object.Name
	}
	return fmt.Sprintf("%s %d", object.Type, fallbackIndex+1)
}

func writeJSONString(b *strings.Builder, value string) {
	encoded, _ := json.Marshal(value)
	b.Write(encoded)
}

func writeJSONValue(b *strings.Builder, value any) {
	switch v := value.(type) {
	case orderedObject:
		b.WriteString("{")
		for i, member := range v {
			if i > 0 {
				b.WriteString(", ")
			}
			writeJSONString(b, member.Key)
			b.WriteString(": ")
			writeJSONValue(b, member.Value)
		}
		b.WriteString("}")
		return
	case []any:
		b.WriteString("[")
		for i, item := range v {
			if i > 0 {
				b.WriteString(", ")
			}
			writeJSONValue(b, item)
		}
		b.WriteString("]")
		return
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		writeJSONString(b, valueToString(value))
		return
	}
	b.Write(encoded)
}

func parseOrderedJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}

	switch t := token.(type) {
	case json.Delim:
		switch t {
		case '{':
			var object orderedObject
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("JSON object key must be a string")
				}
				value, err := parseOrderedJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				object = append(object, orderedMember{Key: key, Value: value})
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return object, nil
		case '[':
			var array []any
			for decoder.More() {
				value, err := parseOrderedJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				array = append(array, value)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return array, nil
		default:
			return nil, fmt.Errorf("unexpected JSON delimiter %q", t)
		}
	default:
		return t, nil
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("trailing content after JSON document")
}
