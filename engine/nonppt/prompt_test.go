package nonppt

import (
	"strings"
	"testing"
)

func TestBuildDOCXModifyPrompt_IncludesIntentTargetAndParagraphSummaries(t *testing.T) {
	prompt := BuildDOCXModifyPrompt(DOCXModifyPromptInput{
		Intent:           "replace_docx_paragraph",
		Description:      "把第二段改正式一点",
		Target:           PromptTargetMetadata{ParagraphIndex: 2, Scope: "paragraph", ElementType: "paragraph"},
		Paragraphs:       []string{"第一段", "第二段"},
		DefaultParagraph: 2,
	})
	for _, needle := range []string{"replace_docx_paragraph", "把第二段改正式一点", "paragraphIndex", "第一段", "第二段"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}

func TestBuildXLSXModifyPrompt_IncludesWorksheetSummaries(t *testing.T) {
	prompt := BuildXLSXModifyPrompt(XLSXModifyPromptInput{
		Intent:             "update_xlsx_cells",
		Description:        "把金额更新到最新值",
		Target:             PromptTargetMetadata{WorksheetIndex: 1, Scope: "worksheet", ElementType: "cells"},
		WorksheetSummaries: []map[string]any{{"worksheetKey": "sheet1", "rows": [][]string{{"区域", "金额"}, {"华东", "100"}}}},
		DefaultWorksheet:   1,
	})
	for _, needle := range []string{"update_xlsx_cells", "把金额更新到最新值", "sheet1", "华东", "100"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}

func TestParseDOCXModifyOperation_ParsesJSON(t *testing.T) {
	op, err := ParseDOCXModifyOperation(`{"intent":"replace_docx_paragraph","paragraphIndex":2,"operation":{"type":"replace_paragraph","newText":"新段落"}}`)
	if err != nil {
		t.Fatalf("ParseDOCXModifyOperation: %v", err)
	}
	if op.ParagraphIndex != 2 || op.Operation.NewText != "新段落" {
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
