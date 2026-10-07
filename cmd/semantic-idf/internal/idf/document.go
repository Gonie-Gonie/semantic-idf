package idf

import "strings"

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
	b.Grow(d.serializedSize())
	const padding = "                                "
	for objectIndex, obj := range d.Objects {
		b.WriteString(obj.Type)
		if len(obj.Fields) == 0 {
			b.WriteString(";\n")
		} else {
			b.WriteString(",\n")
			for i, field := range obj.Fields {
				b.WriteString("  ")
				b.WriteString(field.Value)
				if i == len(obj.Fields)-1 {
					b.WriteByte(';')
				} else {
					b.WriteByte(',')
				}
				if field.Comment != "" {
					lineLength := len(field.Value) + 3
					if lineLength < len(padding) {
						b.WriteString(padding[:len(padding)-lineLength])
					} else {
						b.WriteString("  ")
					}
					b.WriteString("!- ")
					b.WriteString(field.Comment)
				}
				b.WriteByte('\n')
			}
		}

		if objectIndex != len(d.Objects)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (d Document) serializedSize() int {
	size := 0
	for objectIndex, object := range d.Objects {
		size += len(object.Type) + 2
		if objectIndex > 0 {
			size++
		}
		for _, field := range object.Fields {
			lineLength := len(field.Value) + 3
			if field.Comment != "" {
				if lineLength < 32 {
					lineLength = 32
				} else {
					lineLength += 2
				}
				lineLength += 3 + len(field.Comment)
			}
			size += lineLength + 1
		}
	}
	return size
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
