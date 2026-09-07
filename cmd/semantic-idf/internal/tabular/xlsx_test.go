package tabular

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"testing"
)

func TestWorkbookPreservesTraceJSONWhitespaceAndUnicode(t *testing.T) {
	// Literal CR must be a character reference: XML parsers otherwise normalize
	// CR and CRLF, changing a trace's original JSON even with xml:space=preserve.
	values := []string{"{\r\n  \"value\": null,\r\n  \"unknown\": \"🧪 열 & < >\"\r\n}", "\r", "\n", "a\rb", " =SUM(A1:A2) ", "tab\tend"}
	var workbook bytes.Buffer
	if err := WriteWorkbookXLSX(&workbook, []WorkbookSheet{{Name: "Energy Path JSON", Sections: []Section{{Title: "trace", Headers: []string{"json"}, Rows: [][]string{values}}}}}); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(workbook.Bytes()), int64(workbook.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range archive.File {
		if file.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		var sheet struct {
			Rows []struct {
				Cells []struct {
					Type string `xml:"t,attr"`
					Text string `xml:"is>t"`
				} `xml:"c"`
			} `xml:"sheetData>row"`
		}
		if err := xml.Unmarshal(data, &sheet); err != nil {
			t.Fatal(err)
		}
		if len(sheet.Rows) != 3 || len(sheet.Rows[2].Cells) != len(values) {
			t.Fatalf("unexpected worksheet dimensions: %s", data)
		}
		for index, cell := range sheet.Rows[2].Cells {
			if cell.Type != "inlineStr" || cell.Text != values[index] {
				t.Errorf("trace cell %d changed: got %q (%s), want %q", index, cell.Text, cell.Type, values[index])
			}
		}
		return
	}
	t.Fatal("worksheet missing from workbook archive")
}
