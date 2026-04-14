package nonppt

import (
	"strings"
	"testing"
)

func TestBuildDOCXClassifyPrompt_IncludesParagraphContext(t *testing.T) {
	prompt := BuildDOCXClassifyPrompt(DOCXClassifyPromptInput{
		DocumentName:     "Project Proposal",
		Paragraphs:       []string{"Original first paragraph", "Original second paragraph"},
		UserPrompt:       "Rewrite the second paragraph in a more formal tone",
		SelectionContext: map[string]any{"selectionKind": "text"},
	})
	for _, needle := range []string{"Project Proposal", "Original first paragraph", "Original second paragraph", "Rewrite the second paragraph in a more formal tone", "selectionKind"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}

func TestBuildXLSXClassifyPrompt_IncludesWorksheetContext(t *testing.T) {
	prompt := BuildXLSXClassifyPrompt(XLSXClassifyPromptInput{
		DocumentName:       "Sales Report",
		WorksheetSummaries: []map[string]any{{"worksheetKey": "sheet1", "rows": [][]string{{"Region", "Amount"}, {"East", "100"}}}},
		UserPrompt:         "Increase the amount for East",
		SelectionContext:   map[string]any{"selectionKind": "cell"},
	})
	for _, needle := range []string{"Sales Report", "sheet1", "East", "100", "Increase the amount for East"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}

func TestParseDOCXClassifyIntentResult_ParsesAndValidates(t *testing.T) {
	result, err := ParseAndValidateDOCXClassifyIntent(`{"action":"modify_current_document","modifyIntent":"replace_docx_paragraph","targetMetadata":{"scope":"paragraph","paragraphIndex":2,"elementType":"paragraph"},"confidence":0.95,"reason":"The user explicitly asked to revise the second paragraph"}`)
	if err != nil {
		t.Fatalf("ParseAndValidateDOCXClassifyIntent: %v", err)
	}
	if result.ModifyIntent != "replace_docx_paragraph" || result.TargetMetadata.ParagraphIndex != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestParseXLSXClassifyIntentResult_ParsesAndValidates(t *testing.T) {
	result, err := ParseAndValidateXLSXClassifyIntent(`{"action":"modify_current_document","modifyIntent":"update_xlsx_cells","targetMetadata":{"scope":"worksheet","worksheetIndex":1,"elementType":"cells"},"confidence":0.88,"reason":"The user asked to update worksheet values"}`)
	if err != nil {
		t.Fatalf("ParseAndValidateXLSXClassifyIntent: %v", err)
	}
	if result.ModifyIntent != "update_xlsx_cells" || result.TargetMetadata.WorksheetIndex != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
