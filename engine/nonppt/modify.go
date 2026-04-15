package nonppt

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/officecli/officecli/pkg/officegen"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

type ModifyRequest struct {
	Intent      string
	Description string
	Target      ModifyTargetMetadata
}

type ModifyTargetMetadata struct {
	ParagraphIndex int
	WorksheetIndex int
}

type DocxOperation struct {
	Type       string
	NewText    string
	Paragraphs []string
}

type DocxModifyOperation struct {
	Intent         string
	ParagraphIndex int
	Operation      DocxOperation
}

type XLSXCellValue struct {
	Cell  string
	Value string
}

type XLSXOperation struct {
	Type        string
	CellUpdates []XLSXCellValue
	Rows        [][]string
}

type XLSXModifyOperation struct {
	Intent         string
	WorksheetIndex int
	Operation      XLSXOperation
}

func ApplyDOCXModification(req ModifyRequest, ooxmlBytes []byte, op *DocxModifyOperation) ([]byte, error) {
	switch req.Intent {
	case "replace_docx_paragraph":
		return ooxmledit.ReplaceDOCXParagraph(ooxmlBytes, firstPositive(op.ParagraphIndex, req.Target.ParagraphIndex), op.Operation.NewText)
	case "append_docx_paragraph":
		return ooxmledit.AppendDOCXParagraph(ooxmlBytes, firstPositive(op.ParagraphIndex, req.Target.ParagraphIndex), op.Operation.NewText)
	case "rewrite_docx_document":
		return RewriteDOCXDocument(ooxmlBytes, op.Operation.Paragraphs)
	default:
		return nil, fmt.Errorf("unsupported docx intent: %s", req.Intent)
	}
}

func ApplyXLSXModification(req ModifyRequest, ooxmlBytes []byte, op *XLSXModifyOperation) ([]byte, error) {
	switch req.Intent {
	case "update_xlsx_cells":
		updates := make([]ooxmledit.XLSXCellUpdate, 0, len(op.Operation.CellUpdates))
		for _, update := range op.Operation.CellUpdates {
			updates = append(updates, ooxmledit.XLSXCellUpdate{Cell: update.Cell, Value: update.Value})
		}
		return ooxmledit.UpdateXLSXCells(ooxmlBytes, firstPositive(op.WorksheetIndex, req.Target.WorksheetIndex, 1), updates)
	case "append_xlsx_summary":
		rows := op.Operation.Rows
		if len(rows) == 0 {
			rows = [][]string{{"Summary", req.Description}}
		}
		return AppendXLSXRows(ooxmlBytes, firstPositive(op.WorksheetIndex, req.Target.WorksheetIndex, 1), rows)
	case "rewrite_xlsx_sheet":
		return RewriteXLSXSheet(ooxmlBytes, firstPositive(op.WorksheetIndex, req.Target.WorksheetIndex, 1), op.Operation.Rows)
	default:
		return nil, fmt.Errorf("unsupported xlsx intent: %s", req.Intent)
	}
}

func RewriteDOCXDocument(ooxmlBytes []byte, paragraphs []string) ([]byte, error) {
	if len(paragraphs) == 0 {
		return nil, fmt.Errorf("paragraphs cannot be empty")
	}
	docParagraphs := make([]officegen.DocxParagraph, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		docParagraphs = append(docParagraphs, officegen.DocxParagraph{Text: paragraph})
	}
	docxBytes, err := officegen.NewDOCXGenerator().Generate(docParagraphs, officegen.DOCXOptions{Title: "Updated Document", Creator: "OfficeCLI"})
	if err != nil {
		return nil, err
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(docxBytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		return nil, err
	}
	return ooxmledit.ReplaceContentXML(ooxmlBytes, map[string]string{"word/document.xml": contentXMLs["word/document.xml"]})
}

func RewriteXLSXSheet(ooxmlBytes []byte, worksheetIndex int, rows [][]string) ([]byte, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("rows cannot be empty")
	}
	xlsxBytes, err := officegen.NewXLSXGenerator().Generate([]officegen.XlsxSheet{{Name: fmt.Sprintf("Sheet%d", worksheetIndex), Rows: rows}}, officegen.XLSXOptions{Title: "Updated Sheet", Creator: "OfficeCLI"})
	if err != nil {
		return nil, err
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(xlsxBytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		return nil, err
	}
	sheetPath := fmt.Sprintf("xl/worksheets/sheet%d.xml", worksheetIndex)
	return ooxmledit.ReplaceContentXML(ooxmlBytes, map[string]string{
		sheetPath:              contentXMLs["xl/worksheets/sheet1.xml"],
		"xl/sharedStrings.xml": contentXMLs["xl/sharedStrings.xml"],
	})
}

func AppendXLSXRows(ooxmlBytes []byte, worksheetIndex int, rows [][]string) ([]byte, error) {
	contentXMLs, err := ooxmledit.ExtractContentXML(ooxmlBytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		return nil, err
	}
	currentRows := XLSXRowsFromContent(contentXMLs)
	sheetKey := fmt.Sprintf("sheet%d", worksheetIndex)
	nextRows := append(currentRows[sheetKey], rows...)
	return RewriteXLSXSheet(ooxmlBytes, worksheetIndex, nextRows)
}

func XLSXRowsFromContent(contentXMLs map[string]string) map[string][][]string {
	sharedStrings := extractSharedStringsFromXML(contentXMLs["xl/sharedStrings.xml"])
	rowsBySheet := map[string][][]string{}
	sheetKeys := make([]string, 0)
	for path := range contentXMLs {
		if strings.HasPrefix(path, "xl/worksheets/sheet") {
			sheetKeys = append(sheetKeys, path)
		}
	}
	sort.Strings(sheetKeys)
	for _, path := range sheetKeys {
		rowsBySheet[strings.TrimSuffix(filepath.Base(path), ".xml")] = parseWorksheetRows(contentXMLs[path], sharedStrings)
	}
	return rowsBySheet
}

func parseWorksheetRows(sheetXML string, sharedStrings []string) [][]string {
	rowRe := regexp.MustCompile(`(?s)<row\b[^>]*>(.*?)</row>`)
	cellRe := regexp.MustCompile(`(?s)<c\b([^>]*)>(.*?)</c>`)
	vRe := regexp.MustCompile(`(?s)<v>(.*?)</v>`)
	tAttrRe := regexp.MustCompile(`\bt="([^"]+)"`)
	rows := make([][]string, 0)
	for _, rowMatch := range rowRe.FindAllStringSubmatch(sheetXML, -1) {
		rowCells := make([]string, 0)
		for _, cellMatch := range cellRe.FindAllStringSubmatch(rowMatch[1], -1) {
			cellType := ""
			if attrMatch := tAttrRe.FindStringSubmatch(cellMatch[1]); len(attrMatch) == 2 {
				cellType = attrMatch[1]
			}
			value := ""
			if valueMatch := vRe.FindStringSubmatch(cellMatch[2]); len(valueMatch) == 2 {
				value = valueMatch[1]
			}
			if cellType == "s" {
				if idx, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && idx >= 0 && idx < len(sharedStrings) {
					value = sharedStrings[idx]
				}
			}
			rowCells = append(rowCells, value)
		}
		if len(rowCells) > 0 {
			rows = append(rows, rowCells)
		}
	}
	return rows
}

func extractSharedStringsFromXML(sharedStringsXML string) []string {
	re := regexp.MustCompile(`(?s)<si>\s*<t[^>]*>(.*?)</t>\s*</si>`)
	matches := re.FindAllStringSubmatch(sharedStringsXML, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, htmlUnescape(match[1]))
	}
	return values
}

func htmlUnescape(value string) string {
	replacer := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'")
	return replacer.Replace(value)
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
