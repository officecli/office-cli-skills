package nonppt

import (
	"encoding/json"
	"fmt"
	"strings"
)

type ClassifyTargetMetadata struct {
	Scope          string `json:"scope,omitempty"`
	ParagraphIndex int    `json:"paragraphIndex,omitempty"`
	WorksheetIndex int    `json:"worksheetIndex,omitempty"`
	RangeRef       string `json:"rangeRef,omitempty"`
	ElementType    string `json:"elementType,omitempty"`
}

type ClassifyIntentResult struct {
	Action         string                 `json:"action"`
	ModifyIntent   string                 `json:"modifyIntent,omitempty"`
	TargetMetadata ClassifyTargetMetadata `json:"targetMetadata,omitempty"`
	Confidence     float64                `json:"confidence"`
	Reason         string                 `json:"reason,omitempty"`
}

type DOCXClassifyPromptInput struct {
	DocumentName     string
	Paragraphs       []string
	UserPrompt       string
	SelectionContext any
}

type XLSXClassifyPromptInput struct {
	DocumentName       string
	WorksheetSummaries []map[string]any
	UserPrompt         string
	SelectionContext   any
}

func BuildDOCXClassifyPrompt(input DOCXClassifyPromptInput) string {
	return fmt.Sprintf(`You are a classifier for commercial Word document editing requests.

Decide whether the user wants to edit the current document or regenerate a new one based on the paragraph context.

Input:
{
  "documentName": %q,
  "paragraphs": %s,
  "userRequest": %q,
  "selectionContext": %s
}

Rules:
- If the user asks to revise a paragraph, add a paragraph, or adjust the current document, prefer modify_current_document.
- Only use regenerate_new_document when the user explicitly asks to regenerate or create a new document.
- modifyIntent must be one of: replace_docx_paragraph, append_docx_paragraph, rewrite_docx_document.
- If the user explicitly refers to paragraph N, targetMetadata.paragraphIndex must return N.
- targetMetadata.scope must be one of: paragraph, document, selection.

Return only a JSON object that matches the schema.`,
		input.DocumentName,
		MustJSON(DOCXParagraphSummaries(input.Paragraphs)),
		input.UserPrompt,
		MustJSON(input.SelectionContext),
	)
}

func BuildXLSXClassifyPrompt(input XLSXClassifyPromptInput) string {
	return fmt.Sprintf(`You are a classifier for commercial Excel worksheet editing requests.

Decide whether the user wants to edit the current worksheet or regenerate a new document based on the worksheet structure.

Input:
{
  "documentName": %q,
  "worksheets": %s,
  "userRequest": %q,
  "selectionContext": %s
}

Rules:
- If the user asks to revise a cell, a column, a range, or the current worksheet content, prefer modify_current_document.
- Only use regenerate_new_document when the user explicitly asks to regenerate or create a new document.
- modifyIntent must be one of: update_xlsx_cells, append_xlsx_summary, rewrite_xlsx_sheet.
- targetMetadata.scope must be one of: worksheet, range, selection.

Return only a JSON object that matches the schema.`,
		input.DocumentName,
		MustJSON(input.WorksheetSummaries),
		input.UserPrompt,
		MustJSON(input.SelectionContext),
	)
}

func ParseAndValidateDOCXClassifyIntent(raw string) (*ClassifyIntentResult, error) {
	result, err := parseClassifyIntent(raw)
	if err != nil {
		return nil, err
	}
	if result.Action == "modify_current_document" {
		if !isAllowedDOCXIntent(result.ModifyIntent) {
			return nil, fmt.Errorf("unsupported classify intent %s", result.ModifyIntent)
		}
		if strings.TrimSpace(result.TargetMetadata.ElementType) == "" {
			return nil, fmt.Errorf("elementType is required")
		}
		if result.TargetMetadata.Scope == "paragraph" && result.TargetMetadata.ParagraphIndex < 1 {
			return nil, fmt.Errorf("paragraphIndex must be >= 1")
		}
	}
	return result, nil
}

func ParseAndValidateXLSXClassifyIntent(raw string) (*ClassifyIntentResult, error) {
	result, err := parseClassifyIntent(raw)
	if err != nil {
		return nil, err
	}
	if result.Action == "modify_current_document" {
		if !isAllowedXLSXIntent(result.ModifyIntent) {
			return nil, fmt.Errorf("unsupported classify intent %s", result.ModifyIntent)
		}
		if strings.TrimSpace(result.TargetMetadata.ElementType) == "" {
			return nil, fmt.Errorf("elementType is required")
		}
	}
	return result, nil
}

func parseClassifyIntent(raw string) (*ClassifyIntentResult, error) {
	var result ClassifyIntentResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse classify response: %w", err)
	}
	return &result, nil
}

func isAllowedDOCXIntent(intent string) bool {
	switch strings.TrimSpace(intent) {
	case "replace_docx_paragraph", "append_docx_paragraph", "rewrite_docx_document":
		return true
	default:
		return false
	}
}

func isAllowedXLSXIntent(intent string) bool {
	switch strings.TrimSpace(intent) {
	case "update_xlsx_cells", "append_xlsx_summary", "rewrite_xlsx_sheet":
		return true
	default:
		return false
	}
}
