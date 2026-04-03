package ppt

import "testing"

func TestCompileOfficeActionPlan_ReplaceTitle(t *testing.T) {
	plan, err := CompileOfficeActionPlan(TargetMetadata{SlideIndex: 2, ElementType: "title"}, &ModifyOperation{
		Intent:    "replace_slide_title",
		Operation: ModifyOperationPayload{NewTitle: "新标题"},
	}, nil, false)
	if err != nil {
		t.Fatalf("CompileOfficeActionPlan: %v", err)
	}
	if plan == nil || len(plan.Actions) != 1 || plan.Actions[0].Type != "replace_slide_title" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestCompileOfficeActionPlan_UsesSelectedTextWhenPromptTargetsSelection(t *testing.T) {
	plan, err := CompileOfficeActionPlan(TargetMetadata{SlideIndex: 3, ElementType: "body"}, &ModifyOperation{
		Intent:    "replace_body_paragraph",
		Operation: ModifyOperationPayload{NewParagraph: "替换选中文本"},
	}, &SelectionContext{HasTextSelection: true, SlideIndex: 5}, true)
	if err != nil {
		t.Fatalf("CompileOfficeActionPlan: %v", err)
	}
	if plan == nil || plan.Scope != "selection" || plan.Actions[0].Type != "replace_selected_text" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.Actions[0].SlideIndex != 5 {
		t.Fatalf("slide index = %d", plan.Actions[0].SlideIndex)
	}
}

func TestOfficePromptTargetsTextSelection(t *testing.T) {
	if !OfficePromptTargetsTextSelection("请修改当前选中的文字") {
		t.Fatal("expected selection-targeted prompt")
	}
	if OfficePromptTargetsTextSelection("请修改第二页标题") {
		t.Fatal("expected non-selection prompt")
	}
}
