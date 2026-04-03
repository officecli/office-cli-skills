package ooxmledit

import (
	"fmt"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type XLSXCellUpdate struct {
	Cell  string
	Value string
}

var (
	xlsxSharedStringRegex = regexp.MustCompile(`(?s)<si>\s*<t[^>]*>(.*?)</t>\s*</si>`)
	xlsxDimensionRegex    = regexp.MustCompile(`(?s)<dimension\s+ref="([^"]+)"\s*/>`)
)

func UpdateXLSXCells(ooxmlBytes []byte, worksheetIndex int, updates []XLSXCellUpdate) ([]byte, error) {
	if worksheetIndex < 1 {
		return nil, fmt.Errorf("worksheetIndex must be >= 1, got %d", worksheetIndex)
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("cell updates cannot be empty")
	}
	contentXMLs, err := ExtractContentXML(ooxmlBytes, FileTypeXLSX)
	if err != nil {
		return nil, err
	}
	sheetPath := fmt.Sprintf("xl/worksheets/sheet%d.xml", worksheetIndex)
	sheetXML, ok := contentXMLs[sheetPath]
	if !ok {
		return nil, fmt.Errorf("worksheet %d not found", worksheetIndex)
	}
	sharedStringsXML := contentXMLs["xl/sharedStrings.xml"]
	sharedStrings := extractSharedStrings(sharedStringsXML)

	for _, update := range updates {
		if strings.TrimSpace(update.Cell) == "" {
			return nil, fmt.Errorf("cell ref is required")
		}
		idx := slices.Index(sharedStrings, update.Value)
		if idx < 0 {
			sharedStrings = append(sharedStrings, update.Value)
			idx = len(sharedStrings) - 1
		}
		nextCellXML := fmt.Sprintf(`<c r="%s" t="s"><v>%d</v></c>`, strings.ToUpper(strings.TrimSpace(update.Cell)), idx)
		var replaced bool
		sheetXML, replaced = replaceOrInsertSheetCell(sheetXML, strings.ToUpper(strings.TrimSpace(update.Cell)), nextCellXML)
		if !replaced {
			return nil, fmt.Errorf("failed to update cell %s", update.Cell)
		}
	}

	sharedStringsXML = buildSharedStringsXML(sharedStrings)
	modifiedXMLs := map[string]string{
		sheetPath:              sheetXML,
		"xl/sharedStrings.xml": sharedStringsXML,
	}
	return ReplaceContentXML(ooxmlBytes, modifiedXMLs)
}

func extractSharedStrings(sharedStringsXML string) []string {
	matches := xlsxSharedStringRegex.FindAllStringSubmatch(sharedStringsXML, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, html.UnescapeString(match[1]))
	}
	return values
}

func buildSharedStringsXML(values []string) string {
	var items strings.Builder
	for _, value := range values {
		items.WriteString(fmt.Sprintf(`<si><t>%s</t></si>`, escapeXML(value)))
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">%s</sst>`, len(values), len(values), items.String())
}

func replaceOrInsertSheetCell(sheetXML, cellRef, nextCellXML string) (string, bool) {
	cellPattern := regexp.MustCompile(`(?s)<c\b[^>]*r="` + regexp.QuoteMeta(cellRef) + `"\b[^>]*>.*?</c>|<c\b[^>]*r="` + regexp.QuoteMeta(cellRef) + `"\b[^>]*/>`)
	if cellPattern.MatchString(sheetXML) {
		return cellPattern.ReplaceAllString(sheetXML, nextCellXML), true
	}

	rowNumber, err := rowNumberFromCellRef(cellRef)
	if err != nil {
		return sheetXML, false
	}
	rowPattern := regexp.MustCompile(`(?s)(<row\b[^>]*r="` + strconv.Itoa(rowNumber) + `"\b[^>]*>)(.*?)(</row>)`)
	if rowPattern.MatchString(sheetXML) {
		return rowPattern.ReplaceAllString(sheetXML, `${1}${2}`+nextCellXML+`${3}`), true
	}

	insertBefore := "</sheetData>"
	rowXML := fmt.Sprintf(`<row r="%d">%s</row>`, rowNumber, nextCellXML)
	if strings.Contains(sheetXML, insertBefore) {
		sheetXML = strings.Replace(sheetXML, insertBefore, rowXML+insertBefore, 1)
		if updated, ok := updateSheetDimension(sheetXML, cellRef); ok {
			sheetXML = updated
		}
		return sheetXML, true
	}
	return sheetXML, false
}

func rowNumberFromCellRef(cellRef string) (int, error) {
	digits := regexp.MustCompile(`\d+`).FindString(cellRef)
	if digits == "" {
		return 0, fmt.Errorf("invalid cell ref %s", cellRef)
	}
	return strconv.Atoi(digits)
}

func updateSheetDimension(sheetXML, cellRef string) (string, bool) {
	match := xlsxDimensionRegex.FindStringSubmatch(sheetXML)
	if len(match) != 2 {
		return sheetXML, false
	}
	current := match[1]
	if current == "" {
		return sheetXML, false
	}
	parts := strings.Split(current, ":")
	if len(parts) == 1 {
		parts = []string{parts[0], parts[0]}
	}
	end := parts[1]
	if compareCellRef(cellRef, end) <= 0 {
		return sheetXML, true
	}
	next := parts[0] + ":" + cellRef
	return strings.Replace(sheetXML, match[0], `<dimension ref="`+next+`"/>`, 1), true
}

func compareCellRef(left, right string) int {
	leftCol, leftRow := splitCellRef(left)
	rightCol, rightRow := splitCellRef(right)
	if leftRow != rightRow {
		return leftRow - rightRow
	}
	return leftCol - rightCol
}

func splitCellRef(cellRef string) (int, int) {
	colPart := regexp.MustCompile(`[A-Z]+`).FindString(strings.ToUpper(cellRef))
	row, _ := rowNumberFromCellRef(cellRef)
	col := 0
	for _, ch := range colPart {
		col = col*26 + int(ch-'A'+1)
	}
	return col, row
}
