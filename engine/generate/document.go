package generate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/officecli/officecli/pkg/officegen"
)

type DOCXTarget = PromptTarget

type XLSXTarget = PromptTarget

type HTMLTarget = PromptTarget

func BuildDOCXPrompt(description string, target DOCXTarget) string {
	return fmt.Sprintf(`Generate a JSON structure for a Word document based on the following request.

Request: %s
%s

Return only valid JSON in exactly this shape:
{
  "title": "Document title",
  "sections": [
    {
      "heading": "Section heading",
      "level": 1,
      "paragraphs": ["Paragraph 1", "Paragraph 2"]
    }
  ]
}

Requirements:
- Build a complete document structure
- Include title, body, conclusion, and other relevant sections
- Keep the content professional and logically structured`, description, FormatDocumentPromptTarget(target))
}

func BuildDOCXBestSpecPrompt(description string, target DOCXTarget) string {
	return fmt.Sprintf(`First produce a structural blueprint for the following Word document request.

Request: %s
%s

Return JSON only:
{
  "title":"Document title",
  "goal":"Document goal",
  "audience":"Target readers",
  "tone":"Writing tone",
  "sections":[
    {"heading":"Section heading","summary":"Core content this section should cover"}
  ]
}

Requirements:
- Define the structure only, not the full prose
- Keep the section order complete and ready for expansion`, description, FormatDocumentPromptTarget(target))
}

func BuildDOCXBestDraftPrompt(description string, target DOCXTarget, spec string) string {
	return fmt.Sprintf(`Generate the final Word document JSON from the request and blueprint below.

Request: %s
%s

Blueprint:
%s

Return JSON in exactly this shape:
{
  "title": "Document title",
  "sections": [
    {
      "heading": "Section heading",
      "level": 1,
      "paragraphs": ["Paragraph 1", "Paragraph 2"]
    }
  ]
}

Requirements:
- Expand every section into delivery-ready prose
- Avoid filler and keep the information density high
- Do not output anything outside the JSON object`, description, FormatDocumentPromptTarget(target), spec)
}

func BuildXLSXPrompt(description string, target XLSXTarget) string {
	return fmt.Sprintf(`Generate a JSON structure for an Excel workbook based on the following request.

Request: %s
%s

Return only valid JSON in exactly this shape:
{
  "title": "Workbook title",
  "sheets": [
    {
      "name": "Sheet1",
      "headers": ["Column 1", "Column 2", "Column 3"],
      "rows": [
        ["Value 1", "Value 2", "Value 3"],
        ["Value 4", "Value 5", "Value 6"]
      ]
    }
  ]
}

Requirements:
- Generate meaningful data
- Include headers and data rows
- Keep the data format clean and consistent`, description, FormatDocumentPromptTarget(target))
}

func BuildXLSXBestSpecPrompt(description string, target XLSXTarget) string {
	return fmt.Sprintf(`First produce a workbook blueprint for the following Excel request.

Request: %s
%s

Return JSON only:
{
  "title":"Workbook title",
  "goal":"Analysis goal",
  "audience":"Target users",
  "analysisDimensions":["Dimension 1","Dimension 2"],
  "sheets":[
    {"name":"Sheet1","columns":["Column 1","Column 2"],"summary":"What analysis this sheet covers"}
  ]
}

Requirements:
- Plan the workbook structure before filling in data
- Align sheet names and columns with the request`, description, FormatDocumentPromptTarget(target))
}

func BuildXLSXBestDraftPrompt(description string, target XLSXTarget, spec string) string {
	return fmt.Sprintf(`Generate the final Excel JSON from the request and workbook blueprint below.

Request: %s
%s

Workbook blueprint:
%s

Return JSON in exactly this shape:
{
  "title": "Workbook title",
  "sheets": [
    {
      "name": "Sheet1",
      "headers": ["Column 1", "Column 2", "Column 3"],
      "rows": [
        ["Value 1", "Value 2", "Value 3"],
        ["Value 4", "Value 5", "Value 6"]
      ]
    }
  ]
}

Requirements:
- Match the blueprint's column definitions
- Provide at least 2 valid data rows per sheet
- Do not output anything outside the JSON object`, description, FormatDocumentPromptTarget(target), spec)
}

func BuildHTMLPrompt(description string, target HTMLTarget) string {
	return fmt.Sprintf(`Generate a JSON structure for a narrative HTML business report based on the following request.

Request: %s
%s

Return only valid JSON in exactly this shape:
{
  "title": "Report title",
  "subtitle": "One-sentence framing",
  "language": "en",
  "audience": "Business stakeholders",
  "summary": "Executive summary paragraph",
  "updatedAt": "2026-04-14",
  "kpis": [
    {"label":"Revenue","value":"$12.4M","change":"+8%% QoQ","note":"North America stayed ahead of plan"}
  ],
  "findings": ["Finding 1", "Finding 2", "Finding 3"],
  "sections": [
    {
      "title": "Demand momentum",
      "subtitle": "What changed and why it matters",
      "narrative": ["Paragraph 1", "Paragraph 2"],
      "takeaways": ["Takeaway 1", "Takeaway 2"],
      "charts": [
        {
          "type": "bar",
          "title": "Regional revenue",
          "subtitle": "Indexed comparison",
          "categories": ["North America", "Europe", "APAC"],
          "series": [{"name":"Revenue","values":[128,96,74]}],
          "unit": "index",
          "source": "Public company filings"
        }
      ],
      "table": {
        "title": "Supporting table",
        "headers": ["Region", "Revenue", "Growth"],
        "rows": [["North America", "128", "+12%%"]]
      }
    }
  ],
  "appendixTables": [
    {
      "title": "Appendix table",
      "headers": ["Column 1", "Column 2"],
      "rows": [["Value 1", "Value 2"]]
    }
  ]
}

Requirements:
- Write the content in fluent English unless another language is explicitly required
- Make it a narrative report for external business readers, not an admin dashboard
- Use chart types only from: bar, stacked_bar, line, area, donut, scatter, waterfall
- Include 3-5 KPIs, 3-5 findings, and 2-4 analysis sections
- Every section must combine narrative explanation with at least one chart or supporting table
- Keep the data internally consistent and presentation-ready`, description, FormatDocumentPromptTarget(target))
}

func BuildHTMLBestSpecPrompt(description string, target HTMLTarget) string {
	return fmt.Sprintf(`First produce a structural blueprint for the following HTML report request.

Request: %s
%s

Return JSON only:
{
  "title":"Report title",
  "audience":"Target readers",
  "reportStyle":"Narrative business review",
  "goal":"What the report should help the audience decide",
  "sections":[
    {"title":"Section title","purpose":"Why this section exists","chartIntent":"What the chart should prove"}
  ]
}

Requirements:
- Design a narrative HTML report for external readers
- Keep the structure compact, executive-friendly, and chart-driven
- Make every section decision-oriented`, description, FormatDocumentPromptTarget(target))
}

func BuildHTMLBestDraftPrompt(description string, target HTMLTarget, spec string) string {
	return fmt.Sprintf(`Generate the final HTML report JSON from the request and blueprint below.

Request: %s
%s

Blueprint:
%s

Return JSON in exactly the same shape as the HTML report schema.

Requirements:
- Keep the narrative sharp and business-facing
- Use rich charts and consistent numeric framing
- Do not output anything outside the JSON object`, description, FormatDocumentPromptTarget(target), spec)
}

func FormatDocumentPromptTarget(target PromptTarget) string {
	if IsEmptyPromptTarget(target) {
		return ""
	}
	parts := make([]string, 0, 4)
	if target.DocType != "" {
		parts = append(parts, "document_type="+target.DocType)
	}
	if target.Language != "" {
		parts = append(parts, "language="+target.Language)
	}
	if target.Style != "" {
		parts = append(parts, "style="+target.Style)
	}
	if target.Audience != "" {
		parts = append(parts, "audience="+target.Audience)
	}
	return "Additional requirements: " + strings.Join(parts, "; ")
}

func BuildDOCXFromJSON(content, fallbackDescription string) ([]byte, string, error) {
	content = RepairUnescapedQuotes(ExtractJSON(content))

	var llmResp struct {
		Title    string `json:"title"`
		Sections []struct {
			Heading    string   `json:"heading"`
			Level      int      `json:"level"`
			Paragraphs []string `json:"paragraphs"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(content), &llmResp); err != nil {
		return nil, "", fmt.Errorf("parse LLM response: %w", err)
	}

	paragraphs := []officegen.DocxParagraph{{Text: llmResp.Title, Level: 1, IsBold: true}}
	for _, sec := range llmResp.Sections {
		level := sec.Level
		if level <= 0 {
			level = 2
		}
		paragraphs = append(paragraphs, officegen.DocxParagraph{Text: sec.Heading, Level: level})
		for _, p := range sec.Paragraphs {
			paragraphs = append(paragraphs, officegen.DocxParagraph{Text: p, Level: 0})
		}
	}

	fileBytes, err := officegen.NewDOCXGenerator().Generate(paragraphs, officegen.DOCXOptions{Title: llmResp.Title, Creator: "ClaudeOffice"})
	if err != nil {
		return nil, "", fmt.Errorf("generate docx: %w", err)
	}

	title := llmResp.Title
	if title == "" {
		title = ExtractTitleFromDescription(fallbackDescription)
	}
	return fileBytes, fmt.Sprintf("%s.docx", SanitizeFileName(title)), nil
}

func BuildXLSXFromJSON(content, fallbackDescription string) ([]byte, string, error) {
	content = RepairUnescapedQuotes(ExtractJSON(content))

	var llmResp struct {
		Title  string `json:"title"`
		Sheets []struct {
			Name    string     `json:"name"`
			Headers []string   `json:"headers"`
			Rows    [][]string `json:"rows"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal([]byte(content), &llmResp); err != nil {
		return nil, "", fmt.Errorf("parse LLM response: %w", err)
	}

	sheets := make([]officegen.XlsxSheet, 0, len(llmResp.Sheets))
	for _, sh := range llmResp.Sheets {
		rows := make([][]string, 0, len(sh.Rows)+1)
		if len(sh.Headers) > 0 {
			rows = append(rows, sh.Headers)
		}
		rows = append(rows, sh.Rows...)
		sheets = append(sheets, officegen.XlsxSheet{Name: sh.Name, Rows: rows})
	}

	fileBytes, err := officegen.NewXLSXGenerator().Generate(sheets, officegen.XLSXOptions{Title: llmResp.Title, Creator: "ClaudeOffice"})
	if err != nil {
		return nil, "", fmt.Errorf("generate xlsx: %w", err)
	}

	title := llmResp.Title
	if title == "" {
		title = ExtractTitleFromDescription(fallbackDescription)
	}
	return fileBytes, fmt.Sprintf("%s.xlsx", SanitizeFileName(title)), nil
}

func BuildHTMLFromJSON(content, fallbackDescription string) ([]byte, string, error) {
	content = RepairUnescapedQuotes(ExtractJSON(content))

	var report officegen.HTMLReport
	if err := json.Unmarshal([]byte(content), &report); err != nil {
		return nil, "", fmt.Errorf("parse LLM response: %w", err)
	}

	fileBytes, err := officegen.BuildHTMLReport(report)
	if err != nil {
		return nil, "", fmt.Errorf("generate html: %w", err)
	}

	title := strings.TrimSpace(report.Title)
	if title == "" {
		title = ExtractTitleFromDescription(fallbackDescription)
	}
	return fileBytes, fmt.Sprintf("%s.html", SanitizeFileName(title)), nil
}
