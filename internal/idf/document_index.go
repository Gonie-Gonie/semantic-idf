package idf

import "strings"

type DocumentIndex struct {
	Doc               Document
	ObjectsByType     map[string][]Object
	ObjectsByName     map[string][]Object
	ObjectsByTypeName map[string]Object
	Zones             []Object
	Schedules         []Object
}

func NewDocumentIndex(doc Document) *DocumentIndex {
	index := &DocumentIndex{
		Doc:               doc,
		ObjectsByType:     map[string][]Object{},
		ObjectsByName:     map[string][]Object{},
		ObjectsByTypeName: map[string]Object{},
	}
	for _, object := range doc.Objects {
		typeKey := normalizeIndexKey(object.Type)
		index.ObjectsByType[typeKey] = append(index.ObjectsByType[typeKey], object)
		name := objectName(object)
		if name != "" {
			nameKey := normalizeIndexKey(name)
			index.ObjectsByName[nameKey] = append(index.ObjectsByName[nameKey], object)
			index.ObjectsByTypeName[typeNameIndexKey(object.Type, name)] = object
		}
		if strings.EqualFold(object.Type, "Zone") {
			index.Zones = append(index.Zones, object)
		}
		if isScheduleType(object.Type) {
			index.Schedules = append(index.Schedules, object)
		}
	}
	return index
}

func (index *DocumentIndex) ObjectsOfType(typeName string) []Object {
	if index == nil {
		return nil
	}
	return append([]Object(nil), index.ObjectsByType[normalizeIndexKey(typeName)]...)
}

func (index *DocumentIndex) ObjectsNamed(name string) []Object {
	if index == nil {
		return nil
	}
	return append([]Object(nil), index.ObjectsByName[normalizeIndexKey(name)]...)
}

func (index *DocumentIndex) ObjectByTypeName(typeName, name string) (Object, bool) {
	if index == nil {
		return Object{}, false
	}
	object, ok := index.ObjectsByTypeName[typeNameIndexKey(typeName, name)]
	return object, ok
}

func AnalyzeProfileFromIndex(index *DocumentIndex) ProfileReport {
	if index == nil {
		return ProfileReport{}
	}
	return AnalyzeProfile(index.Doc)
}

func AnalyzeHVACFromIndex(index *DocumentIndex) HVACReport {
	if index == nil {
		return HVACReport{}
	}
	return AnalyzeHVAC(index.Doc)
}

func AnalyzeOutputFromIndex(index *DocumentIndex) OutputReport {
	if index == nil {
		return OutputReport{}
	}
	return AnalyzeOutput(index.Doc)
}

func AnalyzeDiagnosticsFromIndex(index *DocumentIndex) []Diagnostic {
	if index == nil {
		return nil
	}
	return AnalyzeDiagnostics(index.Doc)
}

func AnalyzeGeometryFromIndex(index *DocumentIndex) GeometryReport {
	if index == nil {
		return GeometryReport{}
	}
	return AnalyzeGeometry(index.Doc)
}

func typeNameIndexKey(typeName, name string) string {
	return normalizeIndexKey(typeName) + "\x00" + normalizeIndexKey(name)
}

func normalizeIndexKey(value string) string {
	return normalizeName(value)
}
