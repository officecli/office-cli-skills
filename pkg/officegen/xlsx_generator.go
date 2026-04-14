package officegen

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
)

// XlsxSheet represents a worksheet.
type XlsxSheet struct {
	Name string     `json:"name"` // Worksheet name.
	Rows [][]string `json:"rows"` // Row data; each row is a slice of cell strings.
}

// XLSXOptions configures XLSX generation.
type XLSXOptions struct {
	Title   string // Document title.
	Creator string // Document creator.
}

// XLSXGenerator builds XLSX files.
type XLSXGenerator struct{}

// NewXLSXGenerator creates an XLSX generator instance.
func NewXLSXGenerator() *XLSXGenerator {
	return &XLSXGenerator{}
}

// Generate builds an XLSX file and returns its bytes.
func (g *XLSXGenerator) Generate(sheets []XlsxSheet, opts XLSXOptions) ([]byte, error) {
	if len(sheets) == 0 {
		return nil, fmt.Errorf("sheets cannot be empty")
	}

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// Collect all strings into sharedStrings.
	ssTable, ssIndex, ssTotalCount := g.buildSharedStrings(sheets)

	files := g.buildFiles(sheets, ssTable, ssIndex, ssTotalCount, opts)

	for path, content := range files {
		f, err := w.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create file %s: %w", path, err)
		}
		_, err = f.Write([]byte(content))
		if err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

func (g *XLSXGenerator) buildSharedStrings(sheets []XlsxSheet) ([]string, map[string]int, int) {
	var table []string
	index := make(map[string]int)
	totalCount := 0

	for _, sheet := range sheets {
		for _, row := range sheet.Rows {
			for _, cell := range row {
				totalCount++
				if _, ok := index[cell]; !ok {
					index[cell] = len(table)
					table = append(table, cell)
				}
			}
		}
	}

	return table, index, totalCount
}

func (g *XLSXGenerator) buildFiles(sheets []XlsxSheet, ssTable []string, ssIndex map[string]int, ssTotalCount int, opts XLSXOptions) map[string]string {
	files := map[string]string{
		"[Content_Types].xml":        g.generateContentTypes(len(sheets)),
		"_rels/.rels":                xlsxRootRels,
		"docProps/core.xml":          g.generateCoreXML(opts),
		"docProps/app.xml":           xlsxAppXML,
		"xl/workbook.xml":            g.generateWorkbookXML(sheets),
		"xl/_rels/workbook.xml.rels": g.generateWorkbookRels(len(sheets)),
		"xl/styles.xml":              xlsxStylesXML,
		"xl/sharedStrings.xml":       g.generateSharedStringsXML(ssTable, ssTotalCount),
		"xl/theme/theme1.xml":        officeThemeXML,
	}

	// Each sheet gets its own XML file.
	for i, sheet := range sheets {
		sheetNum := i + 1
		sheetPath := fmt.Sprintf("xl/worksheets/sheet%d.xml", sheetNum)
		files[sheetPath] = g.generateSheetXML(sheet, ssIndex)
	}

	return files
}

func (g *XLSXGenerator) generateCoreXML(opts XLSXOptions) string {
	title := opts.Title
	if title == "" {
		title = "Untitled Spreadsheet"
	}
	creator := opts.Creator
	if creator == "" {
		creator = "officecli"
	}
	createdTime := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:dcterms="http://purl.org/dc/terms/"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
    <dc:title>%s</dc:title>
    <dc:creator>%s</dc:creator>
    <dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created>
</cp:coreProperties>`, escapeXML(title), escapeXML(creator), createdTime)
}

func (g *XLSXGenerator) generateContentTypes(sheetCount int) string {
	sheetOverrides := ""
	for i := 1; i <= sheetCount; i++ {
		sheetOverrides += fmt.Sprintf(`    <Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
`, i)
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
    <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
    <Default Extension="xml" ContentType="application/xml"/>
    <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
%s    <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
    <Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
    <Override PartName="/xl/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>
    <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
    <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`, sheetOverrides)
}

func (g *XLSXGenerator) generateWorkbookXML(sheets []XlsxSheet) string {
	var sheetList strings.Builder
	for i, sheet := range sheets {
		name := sheet.Name
		if name == "" {
			name = fmt.Sprintf("Sheet%d", i+1)
		}
		sheetList.WriteString(fmt.Sprintf(`        <sheet name="%s" sheetId="%d" r:id="rId%d"/>
`, escapeXML(name), i+1, i+1))
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
    <bookViews>
        <workbookView xWindow="0" yWindow="0" windowWidth="16384" windowHeight="8192"/>
    </bookViews>
    <sheets>
%s    </sheets>
</workbook>`, sheetList.String())
}

func (g *XLSXGenerator) generateWorkbookRels(sheetCount int) string {
	var rels strings.Builder
	for i := 1; i <= sheetCount; i++ {
		rels.WriteString(fmt.Sprintf(`    <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>
`, i, i))
	}
	rels.WriteString(fmt.Sprintf(`    <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
`, sheetCount+1))
	rels.WriteString(fmt.Sprintf(`    <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>
`, sheetCount+2))
	rels.WriteString(fmt.Sprintf(`    <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>
`, sheetCount+3))

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
%s</Relationships>`, rels.String())
}

func (g *XLSXGenerator) generateSharedStringsXML(table []string, totalCount int) string {
	var items strings.Builder
	for _, s := range table {
		items.WriteString(fmt.Sprintf(`    <si><t>%s</t></si>
`, escapeXML(s)))
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">
%s</sst>`, totalCount, len(table), items.String())
}

// colName converts a zero-based column index to an Excel column name.
func colName(col int) string {
	name := ""
	for col >= 0 {
		name = string(rune('A'+col%26)) + name
		col = col/26 - 1
	}
	return name
}

func (g *XLSXGenerator) generateSheetXML(sheet XlsxSheet, ssIndex map[string]int) string {
	var rows strings.Builder

	// Compute the data range for the dimension tag.
	maxCol := 0
	for _, row := range sheet.Rows {
		if len(row) > maxCol {
			maxCol = len(row)
		}
	}
	rowCount := len(sheet.Rows)

	// Generate the dimension ref.
	dimensionRef := "A1"
	if rowCount > 0 && maxCol > 0 {
		dimensionRef = fmt.Sprintf("A1:%s%d", colName(maxCol-1), rowCount)
	}

	for r, row := range sheet.Rows {
		rowNum := r + 1
		var cells strings.Builder
		for c, cellValue := range row {
			cellRef := fmt.Sprintf("%s%d", colName(c), rowNum)
			idx := ssIndex[cellValue]
			cells.WriteString(fmt.Sprintf(`            <c r="%s" t="s"><v>%d</v></c>
`, cellRef, idx))
		}
		rows.WriteString(fmt.Sprintf(`        <row r="%d">
%s        </row>
`, rowNum, cells.String()))
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
    <dimension ref="%s"/>
    <sheetViews>
        <sheetView workbookViewId="0"/>
    </sheetViews>
    <sheetData>
%s    </sheetData>
</worksheet>`, dimensionRef, rows.String())
}

// ---- Static templates ----

const xlsxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
    <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
    <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
    <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const xlsxAppXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
    <Application>officecli XLSX Generator</Application>
</Properties>`

const xlsxStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
    <fonts count="2">
        <font>
            <sz val="11"/>
            <name val="Microsoft YaHei"/>
        </font>
        <font>
            <b/>
            <sz val="11"/>
            <name val="Microsoft YaHei"/>
        </font>
    </fonts>
    <fills count="2">
        <fill><patternFill patternType="none"/></fill>
        <fill><patternFill patternType="gray125"/></fill>
    </fills>
    <borders count="1">
        <border>
            <left/><right/><top/><bottom/><diagonal/>
        </border>
    </borders>
    <cellStyleXfs count="1">
        <xf numFmtId="0" fontId="0" fillId="0" borderId="0"/>
    </cellStyleXfs>
    <cellXfs count="1">
        <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
    </cellXfs>
    <cellStyles count="1">
        <cellStyle name="Normal" xfId="0" builtinId="0"/>
    </cellStyles>
</styleSheet>`

// xlsxThemeXML lives in theme.go as officeThemeXML and is shared across formats.
