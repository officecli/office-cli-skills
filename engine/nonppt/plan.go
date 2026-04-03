package nonppt

import "strings"

type SelectionContext struct {
	HasTextSelection bool
}

type ModifyTargetMetadataWithScope struct {
	Scope          string
	ElementType    string
	ParagraphIndex int
}

type OfficeActionPlanStep struct {
	Type           string
	Text           string
	ParagraphIndex int
}

type OfficeActionPlan struct {
	Scope   string
	Summary string
	Actions []OfficeActionPlanStep
}

type DOCXPlanInput struct {
	ModifyIntent string
	Target       ModifyTargetMetadataWithScope
	Selection    *SelectionContext
	Operation    *DocxModifyOperation
}

type XLSXPlanInput struct {
	Selection *SelectionContext
	Operation *XLSXModifyOperation
}

func CompileDOCXOfficeActionPlan(input DOCXPlanInput) *OfficeActionPlan {
	if input.Operation == nil {
		return nil
	}
	newText := strings.TrimSpace(input.Operation.Operation.NewText)
	if newText == "" {
		return nil
	}
	if input.Selection != nil && input.Selection.HasTextSelection &&
		(input.Target.Scope == "selection" || input.Target.ElementType == "text") {
		return &OfficeActionPlan{
			Scope:   "selection",
			Summary: "replace selected document text",
			Actions: []OfficeActionPlanStep{{Type: "replace_selected_text", Text: newText}},
		}
	}

	paragraphIndex := firstPositive(input.Operation.ParagraphIndex, input.Target.ParagraphIndex)
	if paragraphIndex < 1 {
		return nil
	}

	switch input.ModifyIntent {
	case "replace_docx_paragraph":
		return &OfficeActionPlan{
			Scope:   "paragraph",
			Summary: "replace document paragraph",
			Actions: []OfficeActionPlanStep{{Type: "replace_document_paragraph", Text: newText, ParagraphIndex: paragraphIndex}},
		}
	case "append_docx_paragraph":
		return &OfficeActionPlan{
			Scope:   "paragraph",
			Summary: "append document paragraph",
			Actions: []OfficeActionPlanStep{{Type: "append_document_paragraph", Text: newText, ParagraphIndex: paragraphIndex}},
		}
	default:
		return nil
	}
}

func CompileXLSXOfficeActionPlan(input XLSXPlanInput) *OfficeActionPlan {
	if input.Selection == nil || !input.Selection.HasTextSelection || input.Operation == nil {
		return nil
	}
	if len(input.Operation.Operation.CellUpdates) != 1 {
		return nil
	}
	return &OfficeActionPlan{
		Scope:   "selection",
		Summary: "replace selected spreadsheet text",
		Actions: []OfficeActionPlanStep{{Type: "replace_selected_text", Text: input.Operation.Operation.CellUpdates[0].Value}},
	}
}
