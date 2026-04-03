package nonppt

import (
	"encoding/json"
	"fmt"
)

type PromptTargetMetadata struct {
	Scope          string `json:"scope,omitempty"`
	ElementType    string `json:"elementType,omitempty"`
	ParagraphIndex int    `json:"paragraphIndex,omitempty"`
	WorksheetIndex int    `json:"worksheetIndex,omitempty"`
	WorksheetName  string `json:"worksheetName,omitempty"`
	RangeRef       string `json:"rangeRef,omitempty"`
}

type DOCXModifyPromptInput struct {
	Intent           string
	Description      string
	Target           PromptTargetMetadata
	Paragraphs       []string
	DefaultParagraph int
}

type XLSXModifyPromptInput struct {
	Intent             string
	Description        string
	Target             PromptTargetMetadata
	WorksheetSummaries []map[string]any
	DefaultWorksheet   int
}

func BuildDOCXModifyPrompt(input DOCXModifyPromptInput) string {
	return fmt.Sprintf(`你是专业的 Word 文档修改助手。请严格按 JSON 输出修改指令。

修改意图：%s
目标元数据：%s
用户指令：%s
当前段落：%s

输出格式：
{
  "intent":"%s",
  "paragraphIndex": %d,
  "operation": {
    "type":"replace_paragraph | append_paragraph | rewrite_document",
    "newText":"新的内容",
    "paragraphs":["整文重写时填写"]
  }
}
`,
		input.Intent,
		MustJSON(input.Target),
		input.Description,
		MustJSON(DOCXParagraphSummaries(input.Paragraphs)),
		input.Intent,
		input.DefaultParagraph,
	)
}

func BuildXLSXModifyPrompt(input XLSXModifyPromptInput) string {
	return fmt.Sprintf(`你是专业的 Excel 表格修改助手。请严格按 JSON 输出修改指令。

修改意图：%s
目标元数据：%s
用户指令：%s
当前工作表：%s

输出格式：
{
  "intent":"%s",
  "worksheetIndex": %d,
  "operation": {
    "type":"update_cells | append_summary | rewrite_sheet",
    "cellUpdates":[{"cell":"B2","value":"150"}],
    "rows":[["列1","列2"]]
  }
}
`,
		input.Intent,
		MustJSON(input.Target),
		input.Description,
		MustJSON(input.WorksheetSummaries),
		input.Intent,
		input.DefaultWorksheet,
	)
}

func ParseDOCXModifyOperation(raw string) (*DocxModifyOperation, error) {
	var op DocxModifyOperation
	if err := json.Unmarshal([]byte(raw), &op); err != nil {
		return nil, fmt.Errorf("parse llm response: %w", err)
	}
	return &op, nil
}

func ParseXLSXModifyOperation(raw string) (*XLSXModifyOperation, error) {
	var op XLSXModifyOperation
	if err := json.Unmarshal([]byte(raw), &op); err != nil {
		return nil, fmt.Errorf("parse llm response: %w", err)
	}
	return &op, nil
}

func DOCXParagraphSummaries(paragraphs []string) []map[string]any {
	items := make([]map[string]any, 0, len(paragraphs))
	for idx, paragraph := range paragraphs {
		items = append(items, map[string]any{
			"paragraphIndex": idx + 1,
			"summary":        paragraph,
		})
	}
	return items
}

func mustJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func MustJSON(value any) string {
	return mustJSON(value)
}
