package officegen

import (
	"archive/zip"
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// XlsxSheet represents the legacy worksheet model.
type XlsxSheet struct {
	Name string     `json:"name"`
	Rows [][]string `json:"rows"`
}

// XlsxColumn describes one logical workbook column.
type XlsxColumn struct {
	Label string `json:"label"`
	Type  string `json:"type,omitempty"`
	Width int    `json:"width,omitempty"`
}

// XlsxSummaryItem stores one summary key/value row rendered above the table.
type XlsxSummaryItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// XlsxSheetSpec is the semantic input model for one worksheet.
type XlsxSheetSpec struct {
	Name       string            `json:"name"`
	Purpose    string            `json:"purpose,omitempty"`
	Columns    []XlsxColumn      `json:"columns,omitempty"`
	Rows       [][]string        `json:"rows,omitempty"`
	Summary    []XlsxSummaryItem `json:"summary,omitempty"`
	Freeze     string            `json:"freeze,omitempty"`
	AutoFilter bool              `json:"autoFilter,omitempty"`
	ShowTotals bool              `json:"showTotals,omitempty"`
}

// XlsxWorkbookSpec is the semantic workbook model.
type XlsxWorkbookSpec struct {
	Title    string          `json:"title"`
	Subtitle string          `json:"subtitle,omitempty"`
	Theme    *DocumentTheme  `json:"theme,omitempty"`
	Sheets   []XlsxSheetSpec `json:"sheets,omitempty"`
}

// XLSXOptions configures XLSX generation.
type XLSXOptions struct {
	Title   string
	Creator string
	Style   string
	Theme   *DocumentTheme
}

// XLSXGenerator builds XLSX files.
type XLSXGenerator struct{}

type xlsxCellKind string

const (
	xlsxCellString  xlsxCellKind = "string"
	xlsxCellNumber  xlsxCellKind = "number"
	xlsxCellBool    xlsxCellKind = "bool"
	xlsxCellDate    xlsxCellKind = "date"
	xlsxCellFormula xlsxCellKind = "formula"
)

const (
	xlsxStyleDefault = iota
	xlsxStyleTitle
	xlsxStyleSubtitle
	xlsxStyleHeader
	xlsxStyleNumber
	xlsxStyleCurrency
	xlsxStylePercent
	xlsxStyleDate
	xlsxStyleSummaryLabel
	xlsxStyleSummaryValue
	xlsxStyleTotalLabel
)

type xlsxCellData struct {
	Kind    xlsxCellKind
	Text    string
	Number  float64
	Bool    bool
	Formula string
	Style   int
}

type xlsxRenderedSheet struct {
	Name          string
	Rows          [][]xlsxCellData
	MaxCols       int
	ColumnWidths  []float64
	AutoFilterRef string
	Freeze        string
}

// NewXLSXGenerator creates an XLSX generator instance.
func NewXLSXGenerator() *XLSXGenerator {
	return &XLSXGenerator{}
}

// Generate keeps the legacy row-array API for modify and report flows.
func (g *XLSXGenerator) Generate(sheets []XlsxSheet, opts XLSXOptions) ([]byte, error) {
	if len(sheets) == 0 {
		return nil, fmt.Errorf("sheets cannot be empty")
	}
	return g.GenerateWorkbook(legacyWorkbookSpecFromSheets(sheets), opts)
}

// GenerateWorkbook builds an XLSX file from the semantic workbook model.
func (g *XLSXGenerator) GenerateWorkbook(spec XlsxWorkbookSpec, opts XLSXOptions) ([]byte, error) {
	spec = normalizeWorkbookSpec(spec)
	if len(spec.Sheets) == 0 {
		return nil, fmt.Errorf("workbook spec cannot be empty")
	}

	theme := ResolveDocumentTheme(opts.Style, spec.Theme)
	if opts.Theme != nil {
		theme = MergeDocumentTheme(theme, opts.Theme)
	}
	rendered := g.renderSheets(spec, theme)
	sharedTable, sharedIndex, totalCount := buildWorkbookSharedStrings(rendered)
	files := g.buildFiles(spec, opts, theme, rendered, sharedTable, sharedIndex, totalCount)

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for path, content := range files {
		f, err := w.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create file %s: %w", path, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}
	return buf.Bytes(), nil
}

func legacyWorkbookSpecFromSheets(sheets []XlsxSheet) XlsxWorkbookSpec {
	spec := XlsxWorkbookSpec{Sheets: make([]XlsxSheetSpec, 0, len(sheets))}
	for _, sheet := range sheets {
		name := strings.TrimSpace(sheet.Name)
		if name == "" {
			name = "Sheet"
		}
		var columns []XlsxColumn
		rows := append([][]string(nil), sheet.Rows...)
		if len(rows) > 0 {
			header := rows[0]
			columns = make([]XlsxColumn, 0, len(header))
			for _, item := range header {
				columns = append(columns, XlsxColumn{Label: strings.TrimSpace(item)})
			}
			if len(rows) > 1 {
				rows = append([][]string(nil), rows[1:]...)
			} else {
				rows = nil
			}
		}
		spec.Sheets = append(spec.Sheets, XlsxSheetSpec{
			Name:       name,
			Columns:    columns,
			Rows:       rows,
			AutoFilter: true,
		})
	}
	return spec
}

func normalizeWorkbookSpec(spec XlsxWorkbookSpec) XlsxWorkbookSpec {
	spec.Title = strings.TrimSpace(spec.Title)
	spec.Subtitle = strings.TrimSpace(spec.Subtitle)
	sheets := make([]XlsxSheetSpec, 0, len(spec.Sheets))
	for idx, sheet := range spec.Sheets {
		sheet.Name = strings.TrimSpace(sheet.Name)
		if sheet.Name == "" {
			sheet.Name = fmt.Sprintf("Sheet%d", idx+1)
		}
		sheet.Purpose = strings.TrimSpace(sheet.Purpose)
		sheet.Freeze = strings.ToUpper(strings.TrimSpace(sheet.Freeze))
		cols := make([]XlsxColumn, 0, len(sheet.Columns))
		for _, col := range sheet.Columns {
			label := strings.TrimSpace(col.Label)
			if label == "" {
				continue
			}
			cols = append(cols, XlsxColumn{
				Label: label,
				Type:  normalizeXLSXColumnType(col.Type),
				Width: col.Width,
			})
		}
		rows := make([][]string, 0, len(sheet.Rows))
		for _, row := range sheet.Rows {
			cells := make([]string, 0, len(row))
			for _, cell := range row {
				cells = append(cells, strings.TrimSpace(cell))
			}
			rows = append(rows, cells)
		}
		sheet.Columns = cols
		sheet.Rows = rows
		sheet.Summary = normalizeSheetSummary(sheet.Summary)
		if !sheet.AutoFilter {
			sheet.AutoFilter = len(sheet.Columns) > 0
		}
		if !sheet.ShowTotals {
			sheet.ShowTotals = shouldShowTotals(sheet)
		}
		if len(sheet.Columns) == 0 && len(sheet.Rows) > 0 {
			inferred := make([]XlsxColumn, 0, len(sheet.Rows[0]))
			for idx, label := range sheet.Rows[0] {
				inferred = append(inferred, XlsxColumn{
					Label: firstNonEmpty(strings.TrimSpace(label), fmt.Sprintf("Column %d", idx+1)),
				})
			}
			sheet.Columns = inferred
			if len(sheet.Rows) > 1 {
				sheet.Rows = append([][]string(nil), sheet.Rows[1:]...)
			} else {
				sheet.Rows = nil
			}
		}
		sheets = append(sheets, sheet)
	}
	spec.Sheets = sheets
	return spec
}

func normalizeSheetSummary(items []XlsxSummaryItem) []XlsxSummaryItem {
	out := make([]XlsxSummaryItem, 0, len(items))
	for _, item := range items {
		label := strings.TrimSpace(item.Label)
		value := strings.TrimSpace(item.Value)
		if label == "" && value == "" {
			continue
		}
		out = append(out, XlsxSummaryItem{Label: label, Value: value})
	}
	return out
}

func normalizeXLSXColumnType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "number", "numeric", "integer":
		return "number"
	case "currency", "money", "amount":
		return "currency"
	case "percent", "percentage", "ratio":
		return "percent"
	case "date", "datetime":
		return "date"
	case "bool", "boolean":
		return "bool"
	case "formula":
		return "formula"
	default:
		return "string"
	}
}

func shouldShowTotals(sheet XlsxSheetSpec) bool {
	if len(sheet.Rows) < 2 {
		return false
	}
	for idx, col := range sheet.Columns {
		if idx >= len(sheet.Columns) {
			break
		}
		switch normalizeXLSXColumnType(col.Type) {
		case "number", "currency", "percent":
			return true
		default:
		}
	}
	return false
}

func (g *XLSXGenerator) renderSheets(spec XlsxWorkbookSpec, theme DocumentTheme) []xlsxRenderedSheet {
	rendered := make([]xlsxRenderedSheet, 0, len(spec.Sheets))
	for _, sheet := range spec.Sheets {
		rendered = append(rendered, renderSheetSpec(spec, sheet, theme))
	}
	return rendered
}

func renderSheetSpec(workbook XlsxWorkbookSpec, sheet XlsxSheetSpec, theme DocumentTheme) xlsxRenderedSheet {
	rows := make([][]xlsxCellData, 0)
	maxCols := maxInt(len(sheet.Columns), 2)

	appendRow := func(items ...xlsxCellData) {
		if len(items) > maxCols {
			maxCols = len(items)
		}
		row := make([]xlsxCellData, len(items))
		copy(row, items)
		rows = append(rows, row)
	}

	appendRow(xlsxCellData{Kind: xlsxCellString, Text: sheet.Name, Style: xlsxStyleTitle})
	if sheet.Purpose != "" || workbook.Subtitle != "" {
		appendRow(xlsxCellData{Kind: xlsxCellString, Text: firstNonEmpty(sheet.Purpose, workbook.Subtitle), Style: xlsxStyleSubtitle})
	}
	for _, item := range sheet.Summary {
		appendRow(
			xlsxCellData{Kind: xlsxCellString, Text: item.Label, Style: xlsxStyleSummaryLabel},
			xlsxCellData{Kind: xlsxCellString, Text: item.Value, Style: xlsxStyleSummaryValue},
		)
	}
	rows = append(rows, nil)

	headerRowIndex := len(rows) + 1
	header := make([]xlsxCellData, 0, len(sheet.Columns))
	for _, col := range sheet.Columns {
		header = append(header, xlsxCellData{Kind: xlsxCellString, Text: col.Label, Style: xlsxStyleHeader})
	}
	appendRow(header...)

	for _, dataRow := range sheet.Rows {
		cells := make([]xlsxCellData, 0, len(sheet.Columns))
		for idx, col := range sheet.Columns {
			value := ""
			if idx < len(dataRow) {
				value = dataRow[idx]
			}
			cells = append(cells, buildSheetCell(value, col.Type))
		}
		appendRow(cells...)
	}

	dataEndRow := headerRowIndex + len(sheet.Rows)
	if sheet.ShowTotals && len(sheet.Rows) > 0 {
		totals := make([]xlsxCellData, 0, len(sheet.Columns))
		for idx, col := range sheet.Columns {
			if idx == 0 {
				totals = append(totals, xlsxCellData{Kind: xlsxCellString, Text: "Total", Style: xlsxStyleTotalLabel})
				continue
			}
			columnLetter := columnName(idx)
			switch normalizeXLSXColumnType(col.Type) {
			case "number", "currency", "percent":
				style := styleForColumnType(col.Type)
				totals = append(totals, xlsxCellData{
					Kind:    xlsxCellFormula,
					Formula: fmt.Sprintf("SUBTOTAL(9,%s%d:%s%d)", columnLetter, headerRowIndex+1, columnLetter, headerRowIndex+len(sheet.Rows)),
					Style:   style,
				})
			default:
				totals = append(totals, xlsxCellData{Kind: xlsxCellString, Text: "", Style: xlsxStyleDefault})
			}
		}
		appendRow(totals...)
	}

	filterLastCol := columnName(maxInt(len(sheet.Columns), 1) - 1)
	autoFilterRef := ""
	if sheet.AutoFilter && len(sheet.Columns) > 0 {
		autoFilterRef = fmt.Sprintf("A%d:%s%d", headerRowIndex, filterLastCol, maxInt(dataEndRow, headerRowIndex))
	}

	freeze := sheet.Freeze
	if freeze == "" && len(sheet.Columns) > 0 {
		freeze = fmt.Sprintf("A%d", headerRowIndex+1)
	}

	return xlsxRenderedSheet{
		Name:          sheet.Name,
		Rows:          rows,
		MaxCols:       maxCols,
		ColumnWidths:  estimateSheetColumnWidths(sheet, maxCols),
		AutoFilterRef: autoFilterRef,
		Freeze:        freeze,
	}
}

func estimateSheetColumnWidths(sheet XlsxSheetSpec, maxCols int) []float64 {
	widths := make([]float64, maxCols)
	for idx := range widths {
		widths[idx] = 12
	}
	for idx, col := range sheet.Columns {
		if idx >= len(widths) {
			break
		}
		if col.Width > 0 {
			widths[idx] = float64(col.Width)
			continue
		}
		widths[idx] = math.Max(widths[idx], estimateCellWidth(col.Label))
	}
	for _, row := range sheet.Rows {
		for idx, cell := range row {
			if idx >= len(widths) {
				break
			}
			widths[idx] = math.Max(widths[idx], estimateCellWidth(cell))
		}
	}
	for idx := range widths {
		if widths[idx] < 10 {
			widths[idx] = 10
		}
		if widths[idx] > 34 {
			widths[idx] = 34
		}
	}
	return widths
}

func estimateCellWidth(value string) float64 {
	width := 0.0
	for _, ch := range strings.TrimSpace(value) {
		if ch <= 127 {
			width += 1
		} else {
			width += 1.7
		}
	}
	return width + 2
}

func buildSheetCell(value, columnType string) xlsxCellData {
	value = strings.TrimSpace(value)
	cellType := normalizeXLSXColumnType(columnType)
	switch cellType {
	case "currency":
		if n, ok := parseNumericCell(value); ok {
			return xlsxCellData{Kind: xlsxCellNumber, Number: n, Style: xlsxStyleCurrency}
		}
	case "percent":
		if n, ok := parsePercentCell(value); ok {
			return xlsxCellData{Kind: xlsxCellNumber, Number: n, Style: xlsxStylePercent}
		}
	case "number":
		if n, ok := parseNumericCell(value); ok {
			return xlsxCellData{Kind: xlsxCellNumber, Number: n, Style: xlsxStyleNumber}
		}
	case "date":
		if n, ok := parseDateCell(value); ok {
			return xlsxCellData{Kind: xlsxCellDate, Number: n, Style: xlsxStyleDate}
		}
	case "bool":
		if b, ok := parseBoolCell(value); ok {
			return xlsxCellData{Kind: xlsxCellBool, Bool: b, Style: xlsxStyleDefault}
		}
	case "formula":
		if strings.HasPrefix(value, "=") {
			return xlsxCellData{Kind: xlsxCellFormula, Formula: strings.TrimPrefix(value, "="), Style: xlsxStyleNumber}
		}
	}

	if strings.HasPrefix(value, "=") {
		return xlsxCellData{Kind: xlsxCellFormula, Formula: strings.TrimPrefix(value, "="), Style: xlsxStyleNumber}
	}
	if b, ok := parseBoolCell(value); ok {
		return xlsxCellData{Kind: xlsxCellBool, Bool: b, Style: xlsxStyleDefault}
	}
	if n, ok := parsePercentCell(value); ok && strings.HasSuffix(value, "%") {
		return xlsxCellData{Kind: xlsxCellNumber, Number: n, Style: xlsxStylePercent}
	}
	if n, ok := parseNumericCell(value); ok {
		return xlsxCellData{Kind: xlsxCellNumber, Number: n, Style: xlsxStyleNumber}
	}
	if n, ok := parseDateCell(value); ok {
		return xlsxCellData{Kind: xlsxCellDate, Number: n, Style: xlsxStyleDate}
	}
	return xlsxCellData{Kind: xlsxCellString, Text: value, Style: xlsxStyleDefault}
}

func styleForColumnType(columnType string) int {
	switch normalizeXLSXColumnType(columnType) {
	case "currency":
		return xlsxStyleCurrency
	case "percent":
		return xlsxStylePercent
	case "date":
		return xlsxStyleDate
	default:
		return xlsxStyleNumber
	}
}

func parseNumericCell(value string) (float64, bool) {
	cleaned := strings.NewReplacer(",", "", "$", "", "￥", "", "¥", "").Replace(strings.TrimSpace(value))
	if cleaned == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parsePercentCell(value string) (float64, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	if strings.HasSuffix(trimmed, "%") {
		n, ok := parseNumericCell(strings.TrimSuffix(trimmed, "%"))
		if !ok {
			return 0, false
		}
		return n / 100, true
	}
	return parseNumericCell(trimmed)
}

func parseBoolCell(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "y", "1":
		return true, true
	case "false", "no", "n", "0":
		return false, true
	default:
		return false, false
	}
}

func parseDateCell(value string) (float64, bool) {
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
	}
	trimmed := strings.TrimSpace(value)
	for _, format := range formats {
		parsed, err := time.Parse(format, trimmed)
		if err == nil {
			base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
			return parsed.UTC().Sub(base).Hours() / 24, true
		}
	}
	return 0, false
}

func buildWorkbookSharedStrings(sheets []xlsxRenderedSheet) ([]string, map[string]int, int) {
	table := make([]string, 0)
	index := make(map[string]int)
	total := 0
	for _, sheet := range sheets {
		for _, row := range sheet.Rows {
			for _, cell := range row {
				if cell.Kind != xlsxCellString {
					continue
				}
				total++
				if _, ok := index[cell.Text]; ok {
					continue
				}
				index[cell.Text] = len(table)
				table = append(table, cell.Text)
			}
		}
	}
	return table, index, total
}

func (g *XLSXGenerator) buildFiles(spec XlsxWorkbookSpec, opts XLSXOptions, theme DocumentTheme, sheets []xlsxRenderedSheet, sharedTable []string, sharedIndex map[string]int, totalCount int) map[string]string {
	title := opts.Title
	if strings.TrimSpace(title) == "" {
		title = spec.Title
	}
	files := map[string]string{
		"[Content_Types].xml":        g.generateContentTypes(len(sheets)),
		"_rels/.rels":                xlsxRootRels,
		"docProps/core.xml":          g.generateCoreXML(title, opts.Creator),
		"docProps/app.xml":           xlsxAppXML,
		"xl/workbook.xml":            g.generateWorkbookXML(sheets),
		"xl/_rels/workbook.xml.rels": g.generateWorkbookRels(len(sheets)),
		"xl/styles.xml":              g.generateStylesXML(theme),
		"xl/sharedStrings.xml":       g.generateSharedStringsXML(sharedTable, totalCount),
		"xl/theme/theme1.xml":        officeThemeXML,
	}
	for idx, sheet := range sheets {
		path := fmt.Sprintf("xl/worksheets/sheet%d.xml", idx+1)
		files[path] = g.generateSheetXML(sheet, sharedIndex)
	}
	return files
}

func (g *XLSXGenerator) generateCoreXML(title, creator string) string {
	if strings.TrimSpace(title) == "" {
		title = "Untitled Spreadsheet"
	}
	if strings.TrimSpace(creator) == "" {
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
	var sheetOverrides strings.Builder
	for i := 1; i <= sheetCount; i++ {
		sheetOverrides.WriteString(fmt.Sprintf(`    <Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
`, i))
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
</Types>`, sheetOverrides.String())
}

func (g *XLSXGenerator) generateWorkbookXML(sheets []xlsxRenderedSheet) string {
	var sheetList strings.Builder
	for idx, sheet := range sheets {
		sheetList.WriteString(fmt.Sprintf(`        <sheet name="%s" sheetId="%d" r:id="rId%d"/>
`, escapeXML(sheet.Name), idx+1, idx+1))
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
	for _, value := range table {
		items.WriteString(fmt.Sprintf(`    <si><t>%s</t></si>
`, escapeXML(value)))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">
%s</sst>`, totalCount, len(table), items.String())
}

func (g *XLSXGenerator) generateStylesXML(theme DocumentTheme) string {
	primary := normalizeHexColor(theme.PrimaryColor, "1D4ED8")
	accentSoft := normalizeHexColor(theme.AccentSoft, "DBEAFE")
	border := normalizeHexColor(theme.BorderColor, "CBD5E1")
	text := normalizeHexColor(theme.TextColor, "0F172A")
	font := escapeXML(theme.FontFamily)
	numFmts := `<numFmts count="3">
        <numFmt numFmtId="164" formatCode="&quot;$&quot;#,##0.00"/>
        <numFmt numFmtId="165" formatCode="0.0%%"/>
        <numFmt numFmtId="166" formatCode="yyyy-mm-dd"/>
    </numFmts>`
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
    %s
    <fonts count="4">
        <font><sz val="11"/><color rgb="FF%s"/><name val="%s"/></font>
        <font><b/><sz val="11"/><color rgb="FF%s"/><name val="%s"/></font>
        <font><b/><sz val="14"/><color rgb="FF%s"/><name val="%s"/></font>
        <font><i/><sz val="11"/><color rgb="FF64748B"/><name val="%s"/></font>
    </fonts>
    <fills count="5">
        <fill><patternFill patternType="none"/></fill>
        <fill><patternFill patternType="gray125"/></fill>
        <fill><patternFill patternType="solid"><fgColor rgb="FFF8FAFC"/><bgColor indexed="64"/></patternFill></fill>
        <fill><patternFill patternType="solid"><fgColor rgb="FF%s"/><bgColor indexed="64"/></patternFill></fill>
        <fill><patternFill patternType="solid"><fgColor rgb="FF%s"/><bgColor indexed="64"/></patternFill></fill>
    </fills>
    <borders count="2">
        <border><left/><right/><top/><bottom/><diagonal/></border>
        <border>
            <left style="thin"><color rgb="FF%s"/></left>
            <right style="thin"><color rgb="FF%s"/></right>
            <top style="thin"><color rgb="FF%s"/></top>
            <bottom style="thin"><color rgb="FF%s"/></bottom>
            <diagonal/>
        </border>
    </borders>
    <cellStyleXfs count="1">
        <xf numFmtId="0" fontId="0" fillId="0" borderId="0"/>
    </cellStyleXfs>
    <cellXfs count="11">
        <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
        <xf numFmtId="0" fontId="2" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1"/>
        <xf numFmtId="0" fontId="3" fillId="0" borderId="0" xfId="0" applyFont="1"/>
        <xf numFmtId="0" fontId="1" fillId="3" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1"/>
        <xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1"/>
        <xf numFmtId="164" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1"/>
        <xf numFmtId="165" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1"/>
        <xf numFmtId="166" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1"/>
        <xf numFmtId="0" fontId="1" fillId="4" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1"/>
        <xf numFmtId="0" fontId="2" fillId="4" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1"/>
        <xf numFmtId="0" fontId="1" fillId="3" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1"/>
    </cellXfs>
    <cellStyles count="1">
        <cellStyle name="Normal" xfId="0" builtinId="0"/>
    </cellStyles>
</styleSheet>`, numFmts, text, font, text, font, primary, font, font, primary, accentSoft, border, border, border, border)
}

func (g *XLSXGenerator) generateSheetXML(sheet xlsxRenderedSheet, sharedIndex map[string]int) string {
	maxCols := maxInt(sheet.MaxCols, 1)
	lastCol := columnName(maxCols - 1)
	lastRow := maxInt(len(sheet.Rows), 1)
	dimensionRef := fmt.Sprintf("A1:%s%d", lastCol, lastRow)

	var cols strings.Builder
	if len(sheet.ColumnWidths) > 0 {
		cols.WriteString("    <cols>\n")
		for idx, width := range sheet.ColumnWidths {
			cols.WriteString(fmt.Sprintf("        <col min=\"%d\" max=\"%d\" width=\"%.2f\" customWidth=\"1\"/>\n", idx+1, idx+1, width))
		}
		cols.WriteString("    </cols>\n")
	}

	var sheetViews strings.Builder
	sheetViews.WriteString("    <sheetViews>\n        <sheetView workbookViewId=\"0\">")
	if sheet.Freeze != "" {
		col, row := splitCellRef(sheet.Freeze)
		xSplit := maxInt(columnIndex(col), 0)
		ySplit := maxInt(row-1, 0)
		activePane := "bottomLeft"
		if xSplit > 0 && ySplit > 0 {
			activePane = "bottomRight"
		} else if xSplit > 0 {
			activePane = "topRight"
		}
		sheetViews.WriteString(fmt.Sprintf(`<pane xSplit="%d" ySplit="%d" topLeftCell="%s" activePane="%s" state="frozen"/>`, xSplit, ySplit, sheet.Freeze, activePane))
	}
	sheetViews.WriteString("</sheetView>\n    </sheetViews>\n")

	var rows strings.Builder
	rows.WriteString("    <sheetData>\n")
	for rowIdx, row := range sheet.Rows {
		if row == nil {
			rows.WriteString(fmt.Sprintf("        <row r=\"%d\"></row>\n", rowIdx+1))
			continue
		}
		rows.WriteString(fmt.Sprintf("        <row r=\"%d\">\n", rowIdx+1))
		for colIdx, cell := range row {
			ref := fmt.Sprintf("%s%d", columnName(colIdx), rowIdx+1)
			rows.WriteString(renderWorkbookCell(ref, cell, sharedIndex))
		}
		rows.WriteString("        </row>\n")
	}
	rows.WriteString("    </sheetData>\n")

	autoFilter := ""
	if sheet.AutoFilterRef != "" {
		autoFilter = fmt.Sprintf("    <autoFilter ref=\"%s\"/>\n", sheet.AutoFilterRef)
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
    <dimension ref="%s"/>
%s%s%s</worksheet>`, dimensionRef, sheetViews.String(), cols.String(), rows.String()+autoFilter)
}

func renderWorkbookCell(ref string, cell xlsxCellData, sharedIndex map[string]int) string {
	styleAttr := ""
	if cell.Style > 0 {
		styleAttr = fmt.Sprintf(` s="%d"`, cell.Style)
	}
	switch cell.Kind {
	case xlsxCellBool:
		val := "0"
		if cell.Bool {
			val = "1"
		}
		return fmt.Sprintf("            <c r=\"%s\" t=\"b\"%s><v>%s</v></c>\n", ref, styleAttr, val)
	case xlsxCellNumber, xlsxCellDate:
		return fmt.Sprintf("            <c r=\"%s\"%s><v>%s</v></c>\n", ref, styleAttr, strconv.FormatFloat(cell.Number, 'f', -1, 64))
	case xlsxCellFormula:
		return fmt.Sprintf("            <c r=\"%s\"%s><f>%s</f></c>\n", ref, styleAttr, escapeXML(cell.Formula))
	default:
		idx := sharedIndex[cell.Text]
		return fmt.Sprintf("            <c r=\"%s\" t=\"s\"%s><v>%d</v></c>\n", ref, styleAttr, idx)
	}
}

func columnName(index int) string {
	if index < 0 {
		return "A"
	}
	name := ""
	for index >= 0 {
		name = string(rune('A'+index%26)) + name
		index = index/26 - 1
	}
	return name
}

func columnIndex(name string) int {
	name = strings.ToUpper(strings.TrimSpace(name))
	value := 0
	for _, ch := range name {
		if ch < 'A' || ch > 'Z' {
			continue
		}
		value = value*26 + int(ch-'A'+1)
	}
	if value <= 0 {
		return 0
	}
	return value - 1
}

func splitCellRef(value string) (string, int) {
	letters := strings.Builder{}
	digits := strings.Builder{}
	for _, ch := range strings.ToUpper(strings.TrimSpace(value)) {
		if ch >= 'A' && ch <= 'Z' {
			letters.WriteRune(ch)
			continue
		}
		if ch >= '0' && ch <= '9' {
			digits.WriteRune(ch)
		}
	}
	row, _ := strconv.Atoi(digits.String())
	if row <= 0 {
		row = 1
	}
	col := letters.String()
	if col == "" {
		col = "A"
	}
	return col, row
}

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
