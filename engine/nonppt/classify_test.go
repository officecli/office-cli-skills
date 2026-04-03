package nonppt

import (
	"strings"
	"testing"
)

func TestBuildDOCXClassifyPrompt_IncludesParagraphContext(t *testing.T) {
	prompt := BuildDOCXClassifyPrompt(DOCXClassifyPromptInput{
		DocumentName:     "项目方案",
		Paragraphs:       []string{"第一段原文", "第二段原文"},
		UserPrompt:       "把第二段改成更正式的语气",
		SelectionContext: map[string]any{"selectionKind": "text"},
	})
	for _, needle := range []string{"项目方案", "第一段原文", "第二段原文", "把第二段改成更正式的语气", "selectionKind"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}

func TestBuildXLSXClassifyPrompt_IncludesWorksheetContext(t *testing.T) {
	prompt := BuildXLSXClassifyPrompt(XLSXClassifyPromptInput{
		DocumentName:       "销售报表",
		WorksheetSummaries: []map[string]any{{"worksheetKey": "sheet1", "rows": [][]string{{"区域", "金额"}, {"华东", "100"}}}},
		UserPrompt:         "把华东金额调高",
		SelectionContext:   map[string]any{"selectionKind": "cell"},
	})
	for _, needle := range []string{"销售报表", "sheet1", "华东", "100", "把华东金额调高"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}

func TestParseDOCXClassifyIntentResult_ParsesAndValidates(t *testing.T) {
	result, err := ParseAndValidateDOCXClassifyIntent(`{"action":"modify_current_document","modifyIntent":"replace_docx_paragraph","targetMetadata":{"scope":"paragraph","paragraphIndex":2,"elementType":"paragraph"},"confidence":0.95,"reason":"用户明确要求修改第二段内容"}`)
	if err != nil {
		t.Fatalf("ParseAndValidateDOCXClassifyIntent: %v", err)
	}
	if result.ModifyIntent != "replace_docx_paragraph" || result.TargetMetadata.ParagraphIndex != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseXLSXClassifyIntentResult_ParsesAndValidates(t *testing.T) {
	result, err := ParseAndValidateXLSXClassifyIntent(`{"action":"modify_current_document","modifyIntent":"update_xlsx_cells","targetMetadata":{"scope":"worksheet","worksheetIndex":1,"elementType":"cells"},"confidence":0.88,"reason":"用户要求更新表格数值"}`)
	if err != nil {
		t.Fatalf("ParseAndValidateXLSXClassifyIntent: %v", err)
	}
	if result.ModifyIntent != "update_xlsx_cells" || result.TargetMetadata.WorksheetIndex != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
