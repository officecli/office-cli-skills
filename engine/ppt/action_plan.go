package ppt

import (
	"fmt"
	"strings"
)

type TargetMetadata struct {
	SlideIndex  int
	ElementType string
}

type SelectionContext struct {
	SlideIndex       int
	HasTextSelection bool
}

type ModifyOperationPayload struct {
	Type         string
	NewTitle     string
	NewBullets   []string
	NewParagraph string
	NewSlideXML  string
	CellUpdates  []CellUpdate
}

type ModifyOperation struct {
	Intent     string
	SlideIndex int
	Operation  ModifyOperationPayload
}

type OfficeActionPlanStep struct {
	Type       string
	Text       string
	Bullets    []string
	SlideIndex int
	TargetRole string
}

type OfficeActionPlan struct {
	Scope   string
	Summary string
	Actions []OfficeActionPlanStep
}

func CompileOfficeActionPlan(target TargetMetadata, op *ModifyOperation, selection *SelectionContext, selectionTargeted bool) (*OfficeActionPlan, error) {
	if op == nil {
		return nil, fmt.Errorf("office action plan operation is required")
	}
	if selectionTargeted && HasOfficeTextSelection(selection) {
		if plan, err := compileOfficeSelectedTextActionPlan(target, op, selection); err != nil {
			return nil, err
		} else if plan != nil {
			return plan, nil
		}
	}
	switch op.Intent {
	case "replace_slide_title":
		return &OfficeActionPlan{Scope: "slide", Summary: "replace slide title", Actions: []OfficeActionPlanStep{{Type: "replace_slide_title", Text: op.Operation.NewTitle, SlideIndex: target.SlideIndex, TargetRole: "title"}}}, nil
	case "append_slide_bullets":
		return &OfficeActionPlan{Scope: "slide", Summary: "append slide bullets", Actions: []OfficeActionPlanStep{{Type: "append_slide_bullets", Bullets: append([]string(nil), op.Operation.NewBullets...), SlideIndex: target.SlideIndex, TargetRole: "body"}}}, nil
	case "replace_body_paragraph":
		return &OfficeActionPlan{Scope: "slide", Summary: "replace body paragraph", Actions: []OfficeActionPlanStep{{Type: "replace_shape_text", Text: op.Operation.NewParagraph, SlideIndex: target.SlideIndex, TargetRole: "body"}}}, nil
	default:
		return nil, fmt.Errorf("unsupported office runtime intent %s", op.Intent)
	}
}

func compileOfficeSelectedTextActionPlan(target TargetMetadata, op *ModifyOperation, selection *SelectionContext) (*OfficeActionPlan, error) {
	if op == nil {
		return nil, fmt.Errorf("office action plan operation is required")
	}
	if !HasOfficeTextSelection(selection) {
		return nil, nil
	}
	var text string
	switch op.Intent {
	case "replace_slide_title":
		text = strings.TrimSpace(op.Operation.NewTitle)
	case "replace_body_paragraph":
		text = strings.TrimSpace(op.Operation.NewParagraph)
	default:
		return nil, nil
	}
	if text == "" {
		return nil, fmt.Errorf("selected text replacement requires text")
	}
	slideIndex := target.SlideIndex
	if selection != nil && selection.SlideIndex > 0 {
		slideIndex = selection.SlideIndex
	}
	return &OfficeActionPlan{Scope: "selection", Summary: "replace selected text", Actions: []OfficeActionPlanStep{{Type: "replace_selected_text", Text: text, SlideIndex: slideIndex, TargetRole: TargetRoleFromElementType(target.ElementType)}}}, nil
}

func HasOfficeTextSelection(selection *SelectionContext) bool {
	return selection != nil && selection.HasTextSelection
}

func TargetRoleFromElementType(elementType string) string {
	switch strings.TrimSpace(elementType) {
	case "title":
		return "title"
	case "body":
		return "body"
	case "slide":
		return "shape"
	default:
		return ""
	}
}

func OfficePromptTargetsTextSelection(prompt string) bool {
	normalized := strings.TrimSpace(strings.ToLower(prompt))
	if normalized == "" {
		return false
	}
	for _, keyword := range []string{"currently selected", "selected text", "selection", "highlighted text"} {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}
	return false
}
