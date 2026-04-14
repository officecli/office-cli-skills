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
	return fmt.Sprintf(`You are a professional Word document editing assistant. Return only JSON instructions.

Edit intent: %s
Target metadata: %s
User request: %s
Current paragraphs: %s

Output schema:
{
  "intent":"%s",
  "paragraphIndex": %d,
  "operation": {
    "type":"replace_paragraph | append_paragraph | rewrite_document",
    "newText":"new content",
    "paragraphs":["fill this only for full-document rewrites"]
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
	return fmt.Sprintf(`You are a professional Excel worksheet editing assistant. Return only JSON instructions.

Edit intent: %s
Target metadata: %s
User request: %s
Current worksheets: %s

Output schema:
{
  "intent":"%s",
  "worksheetIndex": %d,
  "operation": {
    "type":"update_cells | append_summary | rewrite_sheet",
    "cellUpdates":[{"cell":"B2","value":"150"}],
    "rows":[["Column 1","Column 2"]]
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
