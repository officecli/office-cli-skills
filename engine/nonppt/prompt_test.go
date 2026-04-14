package nonppt

import (
	"strings"
	"testing"
)

func TestBuildDOCXModifyPrompt_IncludesIntentTargetAndParagraphSummaries(t *testing.T) {
	prompt := BuildDOCXModifyPrompt(DOCXModifyPromptInput{
		Intent:           "replace_docx_paragraph",
		Description:      "Make the second paragraph more formal",
		Target:           PromptTargetMetadata{ParagraphIndex: 2, Scope: "paragraph", ElementType: "paragraph"},
		Paragraphs:       []string{"First paragraph", "Second paragraph"},
		DefaultParagraph: 2,
	})
	for _, needle := range []string{"replace_docx_paragraph", "Make the second paragraph more formal", "paragraphIndex", "First paragraph", "Second paragraph"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}

func TestBuildXLSXModifyPrompt_IncludesWorksheetSummaries(t *testing.T) {
	prompt := BuildXLSXModifyPrompt(XLSXModifyPromptInput{
		Intent:             "update_xlsx_cells",
		Description:        "Update the amount to the latest value",
		Target:             PromptTargetMetadata{WorksheetIndex: 1, Scope: "worksheet", ElementType: "cells"},
		WorksheetSummaries: []map[string]any{{"worksheetKey": "sheet1", "rows": [][]string{{"Region", "Amount"}, {"East", "100"}}}},
		DefaultWorksheet:   1,
	})
	for _, needle := range []string{"update_xlsx_cells", "Update the amount to the latest value", "sheet1", "East", "100"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}

func TestParseDOCXModifyOperation_ParsesJSON(t *testing.T) {
	op, err := ParseDOCXModifyOperation(`{"intent":"replace_docx_paragraph","paragraphIndex":2,"operation":{"type":"replace_paragraph","newText":"New paragraph"}}`)
	if err != nil {
		t.Fatalf("ParseDOCXModifyOperation: %v", err)
	}
	if op.ParagraphIndex != 2 || op.Operation.NewText != "New paragraph" {
		t.Fatalf("unexpected op: %+v", op)
	}
}

func TestParseXLSXModifyOperation_ParsesJSON(t *testing.T) {
	op, err := ParseXLSXModifyOperation(`{"intent":"update_xlsx_cells","worksheetIndex":1,"operation":{"type":"update_cells","cellUpdates":[{"cell":"B2","value":"150"}]}}`)
	if err != nil {
		t.Fatalf("ParseXLSXModifyOperation: %v", err)
	}
	if op.WorksheetIndex != 1 || len(op.Operation.CellUpdates) != 1 || op.Operation.CellUpdates[0].Value != "150" {
		t.Fatalf("unexpected op: %+v", op)
	}
}
