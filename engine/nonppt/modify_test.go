package nonppt

import (
	"strings"
	"testing"

	"github.com/officecli/officecli/pkg/officegen"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

func TestApplyDOCXModification_ReplacesParagraph(t *testing.T) {
	base, err := officegen.NewDOCXGenerator().Generate([]officegen.DocxParagraph{
		{Text: "First paragraph"},
		{Text: "Second paragraph"},
	}, officegen.DOCXOptions{Title: "test", Creator: "test"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	modified, err := ApplyDOCXModification(ModifyRequest{
		Intent: "replace_docx_paragraph",
		Target: ModifyTargetMetadata{ParagraphIndex: 2},
	}, base, &DocxModifyOperation{
		ParagraphIndex: 2,
		Operation:      DocxOperation{NewText: "Second paragraph rewritten"},
	})
	if err != nil {
		t.Fatalf("ApplyDOCXModification: %v", err)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(modified, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["word/document.xml"], "Second paragraph rewritten") {
		t.Fatalf("document xml = %q, want replaced text", contentXMLs["word/document.xml"])
	}
}

func TestApplyXLSXModification_UpdatesCells(t *testing.T) {
	base, err := officegen.NewXLSXGenerator().Generate([]officegen.XlsxSheet{{
		Name: "Sheet1",
		Rows: [][]string{{"Region", "Amount"}, {"East", "100"}, {"South", "200"}},
	}}, officegen.XLSXOptions{Title: "test", Creator: "test"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	modified, err := ApplyXLSXModification(ModifyRequest{
		Intent:      "update_xlsx_cells",
		Target:      ModifyTargetMetadata{WorksheetIndex: 1},
		Description: "update amounts",
	}, base, &XLSXModifyOperation{
		WorksheetIndex: 1,
		Operation:      XLSXOperation{CellUpdates: []XLSXCellValue{{Cell: "B2", Value: "150"}, {Cell: "B3", Value: "260"}}},
	})
	if err != nil {
		t.Fatalf("ApplyXLSXModification: %v", err)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(modified, ooxmledit.FileTypeXLSX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	shared := contentXMLs["xl/sharedStrings.xml"]
	if !strings.Contains(shared, "150") || !strings.Contains(shared, "260") {
		t.Fatalf("shared strings = %q, want updated values", shared)
	}
}
