package idf

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/internal/tabular"
)

func ObjectTableSections(doc Document) []tabular.Section {
	groups := map[string][]Object{}
	order := make([]string, 0)
	for _, obj := range doc.Objects {
		if _, ok := groups[obj.Type]; !ok {
			order = append(order, obj.Type)
		}
		groups[obj.Type] = append(groups[obj.Type], obj)
	}
	sort.SliceStable(order, func(i, j int) bool {
		return strings.ToLower(order[i]) < strings.ToLower(order[j])
	})

	sections := make([]tabular.Section, 0, len(order))
	for _, objectType := range order {
		objects := groups[objectType]
		sort.SliceStable(objects, func(i, j int) bool {
			return objects[i].Index < objects[j].Index
		})
		sections = append(sections, objectTableSection(objectType, objects))
	}
	return sections
}

func objectTableSection(objectType string, objects []Object) tabular.Section {
	maxFields := 0
	for _, obj := range objects {
		if len(obj.Fields) > maxFields {
			maxFields = len(obj.Fields)
		}
	}

	headers := []string{"object_index", "object_type"}
	for fieldIndex := 0; fieldIndex < maxFields; fieldIndex++ {
		headers = append(headers, objectTableFieldHeader(objects, fieldIndex))
	}
	headers = uniqueTableHeaders(headers)

	rows := make([][]string, 0, len(objects))
	for _, obj := range objects {
		row := []string{fmt.Sprintf("%d", obj.Index+1), obj.Type}
		for fieldIndex := 0; fieldIndex < maxFields; fieldIndex++ {
			value := ""
			if fieldIndex < len(obj.Fields) {
				value = obj.Fields[fieldIndex].Value
			}
			row = append(row, value)
		}
		rows = append(rows, row)
	}
	return tabular.Section{Title: objectType, Headers: headers, Rows: rows}
}

func objectTableFieldHeader(objects []Object, fieldIndex int) string {
	for _, obj := range objects {
		if fieldIndex >= len(obj.Fields) {
			continue
		}
		key := semanticFieldKey(obj.Fields[fieldIndex], fieldIndex)
		if key != "" {
			return key
		}
	}
	return fmt.Sprintf("field_%d", fieldIndex+1)
}

func uniqueTableHeaders(headers []string) []string {
	seen := map[string]int{}
	out := make([]string, len(headers))
	for index, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			header = fmt.Sprintf("column_%d", index+1)
		}
		key := strings.ToLower(header)
		seen[key]++
		if seen[key] > 1 {
			header = fmt.Sprintf("%s_%d", header, seen[key])
		}
		out[index] = header
	}
	return out
}
