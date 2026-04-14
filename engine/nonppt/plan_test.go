package nonppt

import "testing"

func TestCompileDOCXOfficeActionPlan_ForSelectionReplacement(t *testing.T) {
	plan := CompileDOCXOfficeActionPlan(DOCXPlanInput{
		ModifyIntent: "replace_docx_paragraph",
		Target:       ModifyTargetMetadataWithScope{Scope: "selection", ElementType: "text"},
		Selection:    &SelectionContext{HasTextSelection: true},
		Operation:    &DocxModifyOperation{Operation: DocxOperation{NewText: "Replacement for the selected text"}},
	})
	if plan == nil || plan.Scope != "selection" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Type != "replace_selected_text" {
		t.Fatalf("unexpected actions: %+v", plan)
	}
	if plan.Actions[0].Text != "Replacement for the selected text" {
		t.Fatalf("text = %q", plan.Actions[0].Text)
	}
}

func TestCompileDOCXOfficeActionPlan_ForParagraphAppend(t *testing.T) {
	plan := CompileDOCXOfficeActionPlan(DOCXPlanInput{
		ModifyIntent: "append_docx_paragraph",
		Target:       ModifyTargetMetadataWithScope{ParagraphIndex: 3},
		Operation:    &DocxModifyOperation{ParagraphIndex: 3, Operation: DocxOperation{NewText: "Appended paragraph"}},
	})
	if plan == nil || plan.Scope != "paragraph" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.Actions[0].Type != "append_document_paragraph" || plan.Actions[0].ParagraphIndex != 3 {
		t.Fatalf("unexpected action: %+v", plan.Actions[0])
	}
}

func TestCompileXLSXOfficeActionPlan_ForSelectionReplacement(t *testing.T) {
	plan := CompileXLSXOfficeActionPlan(XLSXPlanInput{
		Selection: &SelectionContext{HasTextSelection: true},
		Operation: &XLSXModifyOperation{Operation: XLSXOperation{CellUpdates: []XLSXCellValue{{Cell: "B2", Value: "150"}}}},
	})
	if plan == nil || plan.Scope != "selection" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Text != "150" {
		t.Fatalf("unexpected actions: %+v", plan.Actions)
	}
}
