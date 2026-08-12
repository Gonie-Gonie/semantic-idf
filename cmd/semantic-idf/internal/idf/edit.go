package idf

import "fmt"

func UpdateField(doc Document, objectIndex int, fieldIndex int, value string) (Document, error) {
	if objectIndex < 0 || objectIndex >= len(doc.Objects) {
		return doc, fmt.Errorf("object index %d out of range", objectIndex)
	}
	if fieldIndex < 0 || fieldIndex >= len(doc.Objects[objectIndex].Fields) {
		return doc, fmt.Errorf("field index %d out of range for object %d", fieldIndex, objectIndex)
	}

	updated := doc.clone()
	updated.Objects[objectIndex].Fields[fieldIndex].Value = value
	return updated, nil
}

func RemoveUnusedObjects(doc Document) (Document, []NamedObject) {
	unused := FindUnusedObjects(doc)
	if len(unused) == 0 {
		return doc.clone(), nil
	}

	removeIndexes := map[int]bool{}
	for _, obj := range unused {
		removeIndexes[obj.Index] = true
	}

	var objects []Object
	for _, obj := range doc.Objects {
		if removeIndexes[obj.Index] {
			continue
		}
		obj.Index = len(objects)
		objects = append(objects, obj)
	}
	return Document{Objects: objects}, unused
}
