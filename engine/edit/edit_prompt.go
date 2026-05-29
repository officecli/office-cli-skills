package edit

import (
	"fmt"
	"strings"
)

type PPTXEditPromptInput struct {
	Prompt        string
	SlidePreviews []SlidePreview
	Language      string
	Style         string
}

type SlidePreview struct {
	SlideIndex  int
	TextPreview string
}

type DOCXEditPromptInput struct {
	Prompt             string
	ParagraphPreviews  []ParagraphPreview
	Language           string
	Style              string
}

type ParagraphPreview struct {
	ParagraphIndex int
	Text           string
}

type XLSXEditPromptInput struct {
	Prompt          string
	SheetPreviews   []SheetPreview
	Language        string
	Style           string
}

type SheetPreview struct {
	SheetName string
	Rows      [][]string
}

var pptxOpTypes = []string{
	"replace_slide_title",
	"append_slide_bullets",
	"replace_body_paragraph",
	"update_table_cells",
	"rewrite_entire_slide",
}

var docxOpTypes = []string{
	"replace_docx_paragraph",
	"append_docx_paragraph",
	"rewrite_docx_document",
}

var xlsxOpTypes = []string{
	"update_xlsx_cells",
	"append_xlsx_summary",
	"rewrite_xlsx_sheet",
}

func PPTXOpTypes() []string { return append([]string(nil), pptxOpTypes...) }
func DOCXOpTypes() []string { return append([]string(nil), docxOpTypes...) }
func XLSXOpTypes() []string { return append([]string(nil), xlsxOpTypes...) }

func BuildPPTXEditPrompt(in PPTXEditPromptInput) string {
	var sb strings.Builder
	sb.WriteString(`You are a professional PowerPoint editing assistant. Given a user request and the current slide contents, return a JSON object with an array of edit operations.

User request: `)
	sb.WriteString(in.Prompt)
	sb.WriteString("\n")
	if in.Language != "" {
		sb.WriteString("Language: ")
		sb.WriteString(in.Language)
		sb.WriteString("\n")
	}
	if in.Style != "" {
		sb.WriteString("Style: ")
		sb.WriteString(in.Style)
		sb.WriteString("\n")
	}
	sb.WriteString("\nCurrent slides:\n")
	for _, sp := range in.SlidePreviews {
		fmt.Fprintf(&sb, "  Slide %d: %s\n", sp.SlideIndex, sp.TextPreview)
	}
	sb.WriteString(fmt.Sprintf(`
Allowed op_type values: %s

Response format (strict JSON):
{
  "ops": [
    {
      "op_type": "<one of the allowed op_type values>",
      "target": {"slide": <slide_index>},
      "payload": {
        "new_title": "(for replace_slide_title)",
        "new_bullets": ["(for append_slide_bullets)"],
        "new_paragraph": "(for replace_body_paragraph)",
        "cell_updates": [{"row":1,"col":1,"value":"x"}],
        "new_slide_xml": "(for rewrite_entire_slide)"
      }
    }
  ],
  "__needs_rewrite": []
}

Rules:
- Use only the allowed op_type values listed above
- Each op must include a target with the slide index
- Fill only payload fields relevant to the op_type
- If the edit cannot be expressed with these operations, add the slide reference to __needs_rewrite (e.g. "slide:3")
- __needs_rewrite entries must be at most 3
`, strings.Join(pptxOpTypes, ", ")))
	return sb.String()
}

func BuildDOCXEditPrompt(in DOCXEditPromptInput) string {
	var sb strings.Builder
	sb.WriteString(`You are a professional Word document editing assistant. Given a user request and the current document paragraphs, return a JSON object with an array of edit operations.

User request: `)
	sb.WriteString(in.Prompt)
	sb.WriteString("\n")
	if in.Language != "" {
		sb.WriteString("Language: ")
		sb.WriteString(in.Language)
		sb.WriteString("\n")
	}
	if in.Style != "" {
		sb.WriteString("Style: ")
		sb.WriteString(in.Style)
		sb.WriteString("\n")
	}
	sb.WriteString("\nCurrent paragraphs:\n")
	for _, pp := range in.ParagraphPreviews {
		fmt.Fprintf(&sb, "  Paragraph %d: %s\n", pp.ParagraphIndex, pp.Text)
	}
	sb.WriteString(fmt.Sprintf(`
Allowed op_type values: %s

Response format (strict JSON):
{
  "ops": [
    {
      "op_type": "<one of the allowed op_type values>",
      "target": {"paragraph": <paragraph_index>},
      "payload": {
        "new_text": "(for replace_docx_paragraph or append_docx_paragraph)",
        "paragraphs": ["(for rewrite_docx_document)"]
      }
    }
  ],
  "__needs_rewrite": []
}

Rules:
- Use only the allowed op_type values listed above
- Each op must include a target with the paragraph index
- Fill only payload fields relevant to the op_type
- If the edit cannot be expressed with these operations, add the section reference to __needs_rewrite (e.g. "docx:section:intro")
- __needs_rewrite entries must be at most 3
`, strings.Join(docxOpTypes, ", ")))
	return sb.String()
}

func BuildXLSXEditPrompt(in XLSXEditPromptInput) string {
	var sb strings.Builder
	sb.WriteString(`You are a professional Excel worksheet editing assistant. Given a user request and the current sheet contents, return a JSON object with an array of edit operations.

User request: `)
	sb.WriteString(in.Prompt)
	sb.WriteString("\n")
	if in.Language != "" {
		sb.WriteString("Language: ")
		sb.WriteString(in.Language)
		sb.WriteString("\n")
	}
	if in.Style != "" {
		sb.WriteString("Style: ")
		sb.WriteString(in.Style)
		sb.WriteString("\n")
	}
	sb.WriteString("\nCurrent sheets:\n")
	for _, sp := range in.SheetPreviews {
		fmt.Fprintf(&sb, "  Sheet %q:\n", sp.SheetName)
		for i, row := range sp.Rows {
			fmt.Fprintf(&sb, "    Row %d: %s\n", i+1, strings.Join(row, " | "))
		}
	}
	sb.WriteString(fmt.Sprintf(`
Allowed op_type values: %s

Response format (strict JSON):
{
  "ops": [
    {
      "op_type": "<one of the allowed op_type values>",
      "target": {"sheet": "<sheet_name>", "worksheet_index": <1-based index>},
      "payload": {
        "cell_updates": [{"cell":"B2","value":"150"}],
        "rows": [["col1","col2"]]
      }
    }
  ],
  "__needs_rewrite": []
}

Rules:
- Use only the allowed op_type values listed above
- Each op must include a target with the sheet name or worksheet index
- Fill only payload fields relevant to the op_type
- If the edit cannot be expressed with these operations, add the sheet reference to __needs_rewrite (e.g. "xlsx:sheet:Sheet1")
- __needs_rewrite entries must be at most 3
`, strings.Join(xlsxOpTypes, ", ")))
	return sb.String()
}
