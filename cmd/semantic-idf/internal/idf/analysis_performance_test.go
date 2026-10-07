package idf

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// Replicate the closed-shell fixture into separate, source-ordered zones so the
// benchmarks exercise model-size scaling rather than coincident geometry.
func analysisBenchmarkDocument(tb testing.TB, zoneCount int) Document {
	tb.Helper()
	base, err := Parse(metricsFixtureIDF)
	if err != nil {
		tb.Fatal(err)
	}
	var doc Document
	for zone := 0; zone < zoneCount; zone++ {
		names := map[string]string{}
		for _, object := range base.Objects {
			if name := objectName(object); name != "" {
				names[name] = fmt.Sprintf("%s %d", name, zone)
			}
		}
		for _, original := range base.Objects {
			if zone > 0 && isNamelessType(original.Type) {
				continue
			}
			object := original
			object.Index = len(doc.Objects)
			object.Fields = append([]Field(nil), original.Fields...)
			for index := range object.Fields {
				field := &object.Fields[index]
				if name, ok := names[field.Value]; ok {
					field.Value = name
				}
				if strings.Contains(field.Comment, "X-coordinate") {
					value, err := strconv.ParseFloat(field.Value, 64)
					if err != nil {
						tb.Fatal(err)
					}
					field.Value = strconv.FormatFloat(value+float64(zone)*30, 'f', -1, 64)
				}
			}
			doc.Objects = append(doc.Objects, object)
		}
	}
	return doc
}

func BenchmarkDocumentString(b *testing.B) {
	doc := analysisBenchmarkDocument(b, 80)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = doc.String()
	}
}

func BenchmarkAnalyzeWarm(b *testing.B) {
	doc := analysisBenchmarkDocument(b, 80)
	_ = AnalyzeGeometry(doc)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = Analyze(doc)
	}
}

func BenchmarkThermalTopology(b *testing.B) {
	doc := analysisBenchmarkDocument(b, 80)
	index := NewDocumentIndex(doc)
	geometry := AnalyzeGeometryFromIndex(index)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = BuildThermalTopology(doc, geometry, index)
	}
}

func BenchmarkAnalyzeHVACFromIndex(b *testing.B) {
	index := NewDocumentIndex(analysisBenchmarkDocument(b, 80))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = AnalyzeHVACFromIndex(index)
	}
}
