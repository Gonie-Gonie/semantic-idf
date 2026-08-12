package idf

import (
	"fmt"
	"strings"
)

type Document struct {
	Objects []Object
}

type Object struct {
	Index  int     `json:"index"`
	Type   string  `json:"type"`
	Fields []Field `json:"fields"`
}

type Field struct {
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
}

func (d Document) String() string {
	var b strings.Builder
	for objectIndex, obj := range d.Objects {
		if len(obj.Fields) == 0 {
			fmt.Fprintf(&b, "%s;\n", obj.Type)
		} else {
			fmt.Fprintf(&b, "%s,\n", obj.Type)
			for i, field := range obj.Fields {
				terminator := ","
				if i == len(obj.Fields)-1 {
					terminator = ";"
				}
				line := fmt.Sprintf("  %s%s", field.Value, terminator)
				if field.Comment != "" {
					if len(line) < 32 {
						line += strings.Repeat(" ", 32-len(line))
					} else {
						line += "  "
					}
					line += "!- " + field.Comment
				}
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}

		if objectIndex != len(d.Objects)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (d Document) clone() Document {
	objects := make([]Object, len(d.Objects))
	for i, obj := range d.Objects {
		fields := make([]Field, len(obj.Fields))
		copy(fields, obj.Fields)
		objects[i] = Object{
			Index:  obj.Index,
			Type:   obj.Type,
			Fields: fields,
		}
	}
	return Document{Objects: objects}
}

func objectName(obj Object) string {
	if len(obj.Fields) == 0 || isNamelessType(obj.Type) {
		return ""
	}
	return strings.TrimSpace(obj.Fields[0].Value)
}

func normalizeName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
