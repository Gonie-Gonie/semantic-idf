package tabular

import (
	"archive/zip"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type Section struct {
	Title   string
	Headers []string
	Rows    [][]string
}

type WorkbookSheet struct {
	Name     string
	Sections []Section
}

func WriteOneSheetXLSX(w io.Writer, sheetName string, sections []Section) error {
	return WriteWorkbookXLSX(w, []WorkbookSheet{{Name: sheetName, Sections: sections}})
}

func WriteWorkbookXLSX(w io.Writer, sheets []WorkbookSheet) error {
	if len(sheets) == 0 {
		sheets = []WorkbookSheet{{Name: "Sheet1"}}
	}
	archive := zip.NewWriter(w)
	files := map[string]string{
		"[Content_Types].xml":        contentTypesXMLForSheetCount(len(sheets)),
		"_rels/.rels":                packageRelsXML(),
		"docProps/app.xml":           appPropsXMLForSheets(sheets),
		"docProps/core.xml":          corePropsXML(),
		"xl/workbook.xml":            workbookXMLForSheets(sheets),
		"xl/_rels/workbook.xml.rels": workbookRelsXMLForSheets(sheets),
		"xl/styles.xml":              stylesXML(),
	}
	order := []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"docProps/app.xml",
		"docProps/core.xml",
		"xl/workbook.xml",
		"xl/_rels/workbook.xml.rels",
		"xl/styles.xml",
	}
	for index, sheet := range sheets {
		path := fmt.Sprintf("xl/worksheets/sheet%d.xml", index+1)
		files[path] = worksheetXML(sheet.Name, sheet.Sections)
		order = append(order, path)
	}
	for _, name := range order {
		file, err := archive.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(file, files[name]); err != nil {
			return err
		}
	}
	return archive.Close()
}

func worksheetXML(sheetName string, sections []Section) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	writeColumns(&b, sections)
	b.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`)
	b.WriteString(`<sheetData>`)
	rowIndex := 1
	for sectionIndex, section := range sections {
		if sectionIndex > 0 {
			rowIndex++
		}
		width := max(1, len(section.Headers))
		writeRow(&b, rowIndex, []cell{{Value: "[" + section.Title + "]", Style: 1}}, width)
		rowIndex++
		headerCells := make([]cell, 0, len(section.Headers))
		for _, header := range section.Headers {
			headerCells = append(headerCells, cell{Value: header, Style: 2})
		}
		writeRow(&b, rowIndex, headerCells, 0)
		rowIndex++
		for _, row := range section.Rows {
			cells := make([]cell, 0, len(row))
			for _, value := range row {
				cells = append(cells, cell{Value: value, Style: 3})
			}
			writeRow(&b, rowIndex, cells, 0)
			rowIndex++
		}
	}
	b.WriteString(`</sheetData>`)
	b.WriteString(`<pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/>`)
	b.WriteString(`</worksheet>`)
	return b.String()
}

type cell struct {
	Value string
	Style int
}

func writeColumns(b *strings.Builder, sections []Section) {
	width := maxSectionWidth(sections)
	if width == 0 {
		return
	}
	b.WriteString(`<cols>`)
	for index := 1; index <= width; index++ {
		columnWidth := 18.0
		if index == 1 {
			columnWidth = 14.0
		}
		fmt.Fprintf(b, `<col min="%d" max="%d" width="%.1f" customWidth="1"/>`, index, index, columnWidth)
	}
	b.WriteString(`</cols>`)
}

func writeRow(b *strings.Builder, rowIndex int, cells []cell, fillWidth int) {
	b.WriteString(`<row r="` + strconv.Itoa(rowIndex) + `">`)
	for columnIndex, cell := range cells {
		writeCell(b, columnIndex+1, rowIndex, cell)
	}
	for columnIndex := len(cells) + 1; columnIndex <= fillWidth; columnIndex++ {
		writeCell(b, columnIndex, rowIndex, cell{Style: 1})
	}
	b.WriteString(`</row>`)
}

func writeCell(b *strings.Builder, columnIndex int, rowIndex int, c cell) {
	ref := columnName(columnIndex) + strconv.Itoa(rowIndex)
	style := ""
	if c.Style > 0 {
		style = ` s="` + strconv.Itoa(c.Style) + `"`
	}
	b.WriteString(`<c r="` + ref + `" t="inlineStr"` + style + `><is><t`)
	if needsPreserveSpace(c.Value) {
		b.WriteString(` xml:space="preserve"`)
	}
	b.WriteString(`>`)
	b.WriteString(xmlEscape(c.Value))
	b.WriteString(`</t></is></c>`)
}

func maxSectionWidth(sections []Section) int {
	width := 0
	for _, section := range sections {
		width = max(width, len(section.Headers))
		for _, row := range section.Rows {
			width = max(width, len(row))
		}
	}
	return width
}

func columnName(index int) string {
	if index < 1 {
		return "A"
	}
	var chars []byte
	for index > 0 {
		index--
		chars = append([]byte{byte('A' + index%26)}, chars...)
		index /= 26
	}
	return string(chars)
}

func contentTypesXML() string {
	return contentTypesXMLForSheetCount(1)
}

func contentTypesXMLForSheetCount(sheetCount int) string {
	var worksheets strings.Builder
	for index := 1; index <= max(1, sheetCount); index++ {
		fmt.Fprintf(&worksheets, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, index)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>` +
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
		worksheets.String() +
		`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
		`</Types>`
}

func packageRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>` +
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>` +
		`</Relationships>`
}

func workbookXML(sheetName string) string {
	return workbookXMLForSheets([]WorkbookSheet{{Name: sheetName}})
}

func workbookXMLForSheets(sheets []WorkbookSheet) string {
	var sheetXML strings.Builder
	used := map[string]int{}
	for index, sheet := range sheets {
		name := uniqueExcelSheetName(sheet.Name, used)
		fmt.Fprintf(&sheetXML, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlEscape(name), index+1, index+1)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		`<sheets>` + sheetXML.String() + `</sheets>` +
		`</workbook>`
}

func workbookRelsXML() string {
	return workbookRelsXMLForSheets([]WorkbookSheet{{Name: "Sheet1"}})
}

func workbookRelsXMLForSheets(sheets []WorkbookSheet) string {
	var rels strings.Builder
	for index := range sheets {
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, index+1, index+1)
	}
	styleID := len(sheets) + 1
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		rels.String() +
		fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, styleID) +
		`</Relationships>`
}

func stylesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<fonts count="3"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><color rgb="FFFFFFFF"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
		`<fills count="4"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF1F2937"/><bgColor indexed="64"/></patternFill></fill><fill><patternFill patternType="solid"><fgColor rgb="FFD9EAF7"/><bgColor indexed="64"/></patternFill></fill></fills>` +
		`<borders count="2"><border><left/><right/><top/><bottom/><diagonal/></border><border><left style="thin"><color rgb="FFB8C2CC"/></left><right style="thin"><color rgb="FFB8C2CC"/></right><top style="thin"><color rgb="FFB8C2CC"/></top><bottom style="thin"><color rgb="FFB8C2CC"/></bottom><diagonal/></border></borders>` +
		`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
		`<cellXfs count="4"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="2" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1"/><xf numFmtId="0" fontId="2" fillId="3" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1"/><xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1"/></cellXfs>` +
		`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
		`</styleSheet>`
}

func appPropsXML(sheetName string) string {
	return appPropsXMLForSheets([]WorkbookSheet{{Name: sheetName}})
}

func appPropsXMLForSheets(sheets []WorkbookSheet) string {
	var titles strings.Builder
	used := map[string]int{}
	for _, sheet := range sheets {
		titles.WriteString(`<vt:lpstr>`)
		titles.WriteString(xmlEscape(uniqueExcelSheetName(sheet.Name, used)))
		titles.WriteString(`</vt:lpstr>`)
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">` +
		`<Application>SemanticIDF</Application><DocSecurity>0</DocSecurity><ScaleCrop>false</ScaleCrop>` +
		fmt.Sprintf(`<HeadingPairs><vt:vector size="2" baseType="variant"><vt:variant><vt:lpstr>Worksheets</vt:lpstr></vt:variant><vt:variant><vt:i4>%d</vt:i4></vt:variant></vt:vector></HeadingPairs>`, max(1, len(sheets))) +
		fmt.Sprintf(`<TitlesOfParts><vt:vector size="%d" baseType="lpstr">%s</vt:vector></TitlesOfParts>`, max(1, len(sheets)), titles.String()) +
		`</Properties>`
}

func corePropsXML() string {
	now := time.Now().UTC().Format(time.RFC3339)
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<dc:creator>SemanticIDF</dc:creator><cp:lastModifiedBy>SemanticIDF</cp:lastModifiedBy>` +
		`<dcterms:created xsi:type="dcterms:W3CDTF">` + now + `</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">` + now + `</dcterms:modified>` +
		`</cp:coreProperties>`
}

func excelSheetName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "Sheet1"
	}
	for _, bad := range []string{":", "\\", "/", "?", "*", "[", "]"} {
		value = strings.ReplaceAll(value, bad, " ")
	}
	value = strings.TrimSpace(value)
	if len(value) > 31 {
		value = value[:31]
	}
	if value == "" {
		return "Sheet1"
	}
	return value
}

func uniqueExcelSheetName(value string, used map[string]int) string {
	name := excelSheetName(value)
	count := used[strings.ToLower(name)]
	used[strings.ToLower(name)] = count + 1
	if count == 0 {
		return name
	}
	suffix := fmt.Sprintf(" %d", count+1)
	limit := 31 - len(suffix)
	if len(name) > limit {
		name = name[:limit]
	}
	return name + suffix
}

func needsPreserveSpace(value string) bool {
	return strings.HasPrefix(value, " ") || strings.HasSuffix(value, " ") || strings.Contains(value, "\n")
}

func xmlEscape(value string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(value)
}
