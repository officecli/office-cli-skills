package runtime

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/officecli/officecli/pkg/officegen"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

var (
	workbookSheetRE = regexp.MustCompile(`(?s)<sheet\b[^>]*name="([^"]+)"[^>]*r:id="([^"]+)"[^>]*/?>`)
	workbookRelRE   = regexp.MustCompile(`(?s)<Relationship\b[^>]*Id="([^"]+)"[^>]*Target="([^"]+)"[^>]*/?>`)
	sharedStringRE  = regexp.MustCompile(`(?s)<si>\s*(?:<r>.*?</r>\s*)*<t[^>]*>(.*?)</t>.*?</si>`)
	rowRE           = regexp.MustCompile(`(?s)<row\b([^>]*)>(.*?)</row>`)
	cellRE          = regexp.MustCompile(`(?s)<c\b([^>]*)>(.*?)</c>`)
	valueRE         = regexp.MustCompile(`(?s)<v>(.*?)</v>`)
	inlineStringRE  = regexp.MustCompile(`(?s)<is>.*?<t[^>]*>(.*?)</t>.*?</is>`)
	cellTypeRE      = regexp.MustCompile(`\bt="([^"]+)"`)
	cellRefRE       = regexp.MustCompile(`\br="([A-Z]+)\d+"`)
	rowNumRE        = regexp.MustCompile(`\br="(\d+)"`)
	autoFilterRE    = regexp.MustCompile(`(?s)<autoFilter\b[^>]*ref="([A-Z]+)(\d+):([A-Z]+)(\d+)"[^>]*/?>`)
)

func loadWorkbookSheetsFromFile(path string) ([]officegen.XlsxSheet, error) {
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workbook: %w", err)
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(fileBytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		return nil, fmt.Errorf("extract workbook xml: %w", err)
	}
	return workbookSheetsFromContent(contentXMLs)
}

func workbookSheetsFromContent(contentXMLs map[string]string) ([]officegen.XlsxSheet, error) {
	sharedStrings := extractSharedStrings(contentXMLs["xl/sharedStrings.xml"])
	sheetOrder := extractWorkbookSheetOrder(contentXMLs["xl/workbook.xml"], contentXMLs["xl/_rels/workbook.xml.rels"])
	if len(sheetOrder) == 0 {
		paths := make([]string, 0)
		for path := range contentXMLs {
			if strings.HasPrefix(path, "xl/worksheets/sheet") && strings.HasSuffix(path, ".xml") {
				paths = append(paths, path)
			}
		}
		sort.Strings(paths)
		for _, path := range paths {
			sheetOrder = append(sheetOrder, workbookSheetRef{
				Name: firstWorkbookValue(strings.TrimSuffix(filepath.Base(path), ".xml"), "Sheet"),
				Path: path,
			})
		}
	}
	sheets := make([]officegen.XlsxSheet, 0, len(sheetOrder))
	for _, sheet := range sheetOrder {
		xmlBody := contentXMLs[sheet.Path]
		if strings.TrimSpace(xmlBody) == "" {
			continue
		}
		rows := parseWorksheetRows(xmlBody, sharedStrings)
		if len(rows) == 0 {
			continue
		}
		sheets = append(sheets, officegen.XlsxSheet{Name: firstWorkbookValue(sheet.Name, "Sheet"), Rows: rows})
	}
	if len(sheets) == 0 {
		return nil, fmt.Errorf("workbook does not contain readable worksheet rows")
	}
	return sheets, nil
}

type workbookSheetRef struct {
	Name string
	Path string
}

func extractWorkbookSheetOrder(workbookXML, relsXML string) []workbookSheetRef {
	relTargets := map[string]string{}
	for _, match := range workbookRelRE.FindAllStringSubmatch(relsXML, -1) {
		if len(match) != 3 {
			continue
		}
		target := strings.TrimSpace(match[2])
		target = strings.TrimPrefix(target, "/")
		if !strings.HasPrefix(target, "xl/") {
			target = filepath.ToSlash(filepath.Join("xl", target))
		}
		relTargets[strings.TrimSpace(match[1])] = target
	}
	out := make([]workbookSheetRef, 0)
	for _, match := range workbookSheetRE.FindAllStringSubmatch(workbookXML, -1) {
		if len(match) != 3 {
			continue
		}
		relID := strings.TrimSpace(match[2])
		target := relTargets[relID]
		if target == "" {
			continue
		}
		out = append(out, workbookSheetRef{
			Name: html.UnescapeString(strings.TrimSpace(match[1])),
			Path: filepath.ToSlash(target),
		})
	}
	return out
}

func extractSharedStrings(sharedStringsXML string) []string {
	matches := sharedStringRE.FindAllStringSubmatch(sharedStringsXML, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, html.UnescapeString(strings.TrimSpace(match[1])))
	}
	return values
}

func parseWorksheetRows(sheetXML string, sharedStrings []string) [][]string {
	startRow := 1
	if match := autoFilterRE.FindStringSubmatch(sheetXML); len(match) == 5 {
		if parsed, err := strconv.Atoi(strings.TrimSpace(match[2])); err == nil && parsed > 0 {
			startRow = parsed
		}
	}
	rows := make([][]string, 0)
	for _, rowMatch := range rowRE.FindAllStringSubmatch(sheetXML, -1) {
		if len(rowMatch) != 3 {
			continue
		}
		rowIndex := 0
		if idxMatch := rowNumRE.FindStringSubmatch(rowMatch[1]); len(idxMatch) == 2 {
			rowIndex, _ = strconv.Atoi(strings.TrimSpace(idxMatch[1]))
		}
		if rowIndex > 0 && rowIndex < startRow {
			continue
		}
		rowCells := make([]string, 0)
		for _, cellMatch := range cellRE.FindAllStringSubmatch(rowMatch[2], -1) {
			if len(cellMatch) != 3 {
				continue
			}
			attrs := cellMatch[1]
			body := cellMatch[2]
			cellIndex := len(rowCells)
			if refMatch := cellRefRE.FindStringSubmatch(attrs); len(refMatch) == 2 {
				cellIndex = excelColToIndex(refMatch[1])
			}
			for len(rowCells) < cellIndex {
				rowCells = append(rowCells, "")
			}
			rowCells = append(rowCells, parseCellValue(attrs, body, sharedStrings))
		}
		if len(rowCells) == 0 {
			continue
		}
		rows = append(rows, trimTrailingEmpty(rowCells))
	}
	return rows
}

func parseCellValue(attrs, body string, sharedStrings []string) string {
	cellType := ""
	if typeMatch := cellTypeRE.FindStringSubmatch(attrs); len(typeMatch) == 2 {
		cellType = strings.TrimSpace(typeMatch[1])
	}
	switch cellType {
	case "inlineStr":
		if textMatch := inlineStringRE.FindStringSubmatch(body); len(textMatch) == 2 {
			return html.UnescapeString(strings.TrimSpace(textMatch[1]))
		}
	case "s":
		if valueMatch := valueRE.FindStringSubmatch(body); len(valueMatch) == 2 {
			if idx, err := strconv.Atoi(strings.TrimSpace(valueMatch[1])); err == nil && idx >= 0 && idx < len(sharedStrings) {
				return sharedStrings[idx]
			}
		}
	default:
		if valueMatch := valueRE.FindStringSubmatch(body); len(valueMatch) == 2 {
			return strings.TrimSpace(valueMatch[1])
		}
	}
	return ""
}

func excelColToIndex(name string) int {
	value := 0
	for _, ch := range strings.ToUpper(strings.TrimSpace(name)) {
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

func trimTrailingEmpty(items []string) []string {
	last := len(items) - 1
	for last >= 0 && strings.TrimSpace(items[last]) == "" {
		last--
	}
	if last < 0 {
		return nil
	}
	return append([]string(nil), items[:last+1]...)
}

func buildWorkbookSummary(sheets []officegen.XlsxSheet) string {
	if len(sheets) == 0 {
		return "No readable worksheets were found."
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Workbook contains %d worksheet(s).\n", len(sheets)))
	for idx, sheet := range sheets {
		sb.WriteString(fmt.Sprintf("Sheet %d: %s\n", idx+1, firstWorkbookValue(sheet.Name, "Sheet")))
		if len(sheet.Rows) == 0 {
			sb.WriteString("- No rows\n")
			continue
		}
		headers := trimTrailingEmpty(sheet.Rows[0])
		if len(headers) > 0 {
			sb.WriteString("- Headers: ")
			sb.WriteString(strings.Join(headers, ", "))
			sb.WriteString("\n")
		}
		dataRows := maxInt(len(sheet.Rows)-1, 0)
		sb.WriteString(fmt.Sprintf("- Data rows: %d\n", dataRows))
		sampleLimit := len(sheet.Rows)
		if sampleLimit > 3 {
			sampleLimit = 3
		}
		for rowIdx := 1; rowIdx < sampleLimit; rowIdx++ {
			sb.WriteString(fmt.Sprintf("- Sample row %d: %s\n", rowIdx, strings.Join(trimTrailingEmpty(sheet.Rows[rowIdx]), " | ")))
		}
	}
	return strings.TrimSpace(sb.String())
}

func firstWorkbookValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maxInt(value int, fallback int) int {
	if value > fallback {
		return value
	}
	return fallback
}
