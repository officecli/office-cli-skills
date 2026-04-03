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
	return fmt.Sprintf(`你是一个商业化 Word 文档编辑意图分类器。

请基于段落上下文判断用户是在修改当前文档，还是要求重新生成新文档。

输入上下文：
{
  "documentName": %q,
  "paragraphs": %s,
  "userRequest": %q,
  "selectionContext": %s
}

判定规则：
- 用户要求改某一段、补一段、调整当前文档内容，优先判定为 modify_current_document
- 只有明确提出“重新生成”“新建一份”时，才判定为 regenerate_new_document
- modifyIntent 只允许输出：replace_docx_paragraph, append_docx_paragraph, rewrite_docx_document
- 如果用户明确提到“第N段”，targetMetadata.paragraphIndex 必须返回 N
- targetMetadata.scope 只能是 paragraph、document、selection

只返回符合 schema 的 JSON 对象。`,
		input.DocumentName,
		MustJSON(DOCXParagraphSummaries(input.Paragraphs)),
		input.UserPrompt,
		MustJSON(input.SelectionContext),
	)
}

func BuildXLSXClassifyPrompt(input XLSXClassifyPromptInput) string {
	return fmt.Sprintf(`你是一个商业化 Excel 表格编辑意图分类器。

请基于工作表结构判断用户是在修改当前表格，还是要求重新生成新文档。

输入上下文：
{
  "documentName": %q,
  "worksheets": %s,
  "userRequest": %q,
  "selectionContext": %s
}

判定规则：
- 用户要求改某个单元格、某一列、某个范围、当前表格内容，优先判定为 modify_current_document
- 只有明确提出“重新生成”“新建一份”时，才判定为 regenerate_new_document
- modifyIntent 只允许输出：update_xlsx_cells, append_xlsx_summary, rewrite_xlsx_sheet
- targetMetadata.scope 只能是 worksheet、range、selection

只返回符合 schema 的 JSON 对象。`,
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
