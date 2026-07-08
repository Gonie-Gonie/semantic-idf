package epinput

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

var vertexFieldPattern = regexp.MustCompile(`^vertex_(\d+)_(x|y|z)_coordinate$`)

type vertexFields map[string]Field

func collapseVertexArrayFields(objectType string, fields []Field) []Field {
	if !usesVerticesArray(objectType) {
		return fields
	}

	vertices := map[int]vertexFields{}
	firstVertexIndex := -1
	vertexFieldIndexes := map[int]bool{}

	for index, field := range fields {
		matches := vertexFieldPattern.FindStringSubmatch(field.Key)
		if matches == nil {
			continue
		}
		vertexIndex, err := strconv.Atoi(matches[1])
		if err != nil || vertexIndex <= 0 {
			continue
		}
		if firstVertexIndex == -1 {
			firstVertexIndex = index
		}
		if vertices[vertexIndex] == nil {
			vertices[vertexIndex] = vertexFields{}
		}
		vertices[vertexIndex][matches[2]] = field
		vertexFieldIndexes[index] = true
	}

	if firstVertexIndex < 0 || !hasCompleteVertices(vertices) {
		return fields
	}

	var out []Field
	verticesWritten := false
	for index, field := range fields {
		if vertexFieldIndexes[index] {
			if !verticesWritten {
				out = append(out, Field{
					Key:     "vertices",
					Value:   orderedVertices(vertices),
					Comment: "Vertices",
				})
				verticesWritten = true
			}
			continue
		}
		out = append(out, field)
	}
	return out
}

func expandVertexArrayField(objectType string, field Field) ([]idf.Field, bool) {
	if !usesVerticesArray(objectType) || field.Key != "vertices" {
		return nil, false
	}

	items, ok := field.Value.([]any)
	if !ok {
		return nil, false
	}

	expanded := make([]idf.Field, 0, len(items)*3)
	for index, item := range items {
		vertex := orderedVertexValues(item)
		if vertex == nil {
			return nil, false
		}
		ordinal := index + 1
		expanded = append(expanded,
			idf.Field{Value: valueToString(vertex["x"]), Comment: fmt.Sprintf("Vertex %d X-coordinate", ordinal)},
			idf.Field{Value: valueToString(vertex["y"]), Comment: fmt.Sprintf("Vertex %d Y-coordinate", ordinal)},
			idf.Field{Value: valueToString(vertex["z"]), Comment: fmt.Sprintf("Vertex %d Z-coordinate", ordinal)},
		)
	}
	return expanded, true
}

func usesVerticesArray(objectType string) bool {
	switch strings.ToLower(objectType) {
	case "buildingsurface:detailed",
		"wall:detailed",
		"roofceiling:detailed",
		"floor:detailed",
		"shading:site:detailed",
		"shading:building:detailed",
		"shading:zone:detailed":
		return true
	default:
		return false
	}
}

func hasCompleteVertices(vertices map[int]vertexFields) bool {
	if len(vertices) == 0 {
		return false
	}
	for index, values := range vertices {
		if index <= 0 || values["x"].Key == "" || values["y"].Key == "" || values["z"].Key == "" {
			return false
		}
	}
	return true
}

func orderedVertices(vertices map[int]vertexFields) []any {
	indexes := make([]int, 0, len(vertices))
	for index := range vertices {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	out := make([]any, 0, len(indexes))
	for _, index := range indexes {
		values := vertices[index]
		out = append(out, orderedObject{
			{Key: "vertex_x_coordinate", Value: coordinateValue(values["x"].Value)},
			{Key: "vertex_y_coordinate", Value: coordinateValue(values["y"].Value)},
			{Key: "vertex_z_coordinate", Value: coordinateValue(values["z"].Value)},
		})
	}
	return out
}

func coordinateValue(value any) any {
	text := strings.TrimSpace(valueToString(value))
	if text == "" {
		return ""
	}
	if _, err := strconv.ParseFloat(text, 64); err == nil {
		return json.Number(text)
	}
	return value
}

func orderedVertexValues(value any) map[string]any {
	out := map[string]any{}
	switch vertex := value.(type) {
	case orderedObject:
		for _, member := range vertex {
			axis := vertexAxis(member.Key)
			if axis != "" {
				out[axis] = member.Value
			}
		}
	case map[string]any:
		for key, value := range vertex {
			axis := vertexAxis(key)
			if axis != "" {
				out[axis] = value
			}
		}
	default:
		return nil
	}
	if _, ok := out["x"]; !ok {
		return nil
	}
	if _, ok := out["y"]; !ok {
		return nil
	}
	if _, ok := out["z"]; !ok {
		return nil
	}
	return out
}

func vertexAxis(key string) string {
	switch strings.ToLower(key) {
	case "vertex_x_coordinate":
		return "x"
	case "vertex_y_coordinate":
		return "y"
	case "vertex_z_coordinate":
		return "z"
	default:
		return ""
	}
}
