package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/officecli/officecli/pkg/officegen"
)

type DOCXTarget = PromptTarget
type XLSXTarget = PromptTarget
type ReportTarget = PromptTarget

type jsonScalarString string

type xlsxSummaryInput struct {
	Label jsonScalarString `json:"label"`
	Value jsonScalarString `json:"value"`
}

type xlsxSheetSpecInput struct {
	Name       string                 `json:"name"`
	Purpose    string                 `json:"purpose,omitempty"`
	Columns    []officegen.XlsxColumn `json:"columns,omitempty"`
	Rows       [][]jsonScalarString   `json:"rows,omitempty"`
	Summary    []xlsxSummaryInput     `json:"summary,omitempty"`
	Freeze     string                 `json:"freeze,omitempty"`
	AutoFilter bool                   `json:"autoFilter,omitempty"`
	ShowTotals bool                   `json:"showTotals,omitempty"`
}

type xlsxWorkbookSpecInput struct {
	Title    string                   `json:"title"`
	Subtitle string                   `json:"subtitle,omitempty"`
	Theme    *officegen.DocumentTheme `json:"theme,omitempty"`
	Sheets   []xlsxSheetSpecInput     `json:"sheets,omitempty"`
}

func (s *jsonScalarString) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}

	text, err := stringifyJSONScalar(value)
	if err != nil {
		return err
	}
	*s = jsonScalarString(text)
	return nil
}

func stringifyJSONScalar(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	default:
		return "", fmt.Errorf("expected JSON scalar, got %T", value)
	}
}

func BuildDOCXPrompt(description string, target DOCXTarget) string {
	return fmt.Sprintf(`Generate a JSON structure for a high-quality Word document.

Request: %s
%s

Return JSON only in exactly this shape:
{
  "title": "Document title",
  "subtitle": "One-sentence framing",
  "theme": {"preset": "executive"},
  "blocks": [
    {"type":"heading","level":1,"text":"Executive Summary"},
    {"type":"paragraph","text":"Dense professional prose."},
    {"type":"callout","title":"Decision","text":"What matters most."},
    {"type":"bullets","items":["Point 1","Point 2"]},
    {"type":"table","title":"Operating plan","columns":["Workstream","Owner","Next step"],"rows":[["A","B","C"]]}
  ]
}

Requirements:
- Build a polished business document, not a loose note dump
- Use 5-10 blocks with clear structure
- Prefer a mix of headings, paragraphs, bullets, and one callout when useful
- Include a table whenever the request benefits from comparison, plan tracking, or structured evidence
- Keep wording concise, professional, and publication-ready`, description, FormatDocumentPromptTarget(target))
}

func BuildDOCXBestSpecPrompt(description string, target DOCXTarget) string {
	return fmt.Sprintf(`First produce a structural blueprint for a polished Word document.

Request: %s
%s

Return JSON only:
{
  "title":"Document title",
  "subtitle":"One-sentence framing",
  "theme":{"preset":"executive"},
  "sections":[
    {"heading":"Section heading","goal":"Why this section exists","preferredBlocks":["paragraph","bullets","table"]}
  ]
}

Requirements:
- Design the structure only, not the full prose
- Keep the section order complete and implementable
- Make block choices explicit when a table, bullets, or callout would improve readability`, description, FormatDocumentPromptTarget(target))
}

func BuildDOCXBestDraftPrompt(description string, target DOCXTarget, spec string) string {
	return fmt.Sprintf(`Generate the final Word document JSON from the request and blueprint below.

Request: %s
%s

Blueprint:
%s

Return JSON in the same schema as the DOCX document payload.

Requirements:
- Expand the blueprint into delivery-ready content
- Preserve strong structure and information density
- Do not output anything outside the JSON object`, description, FormatDocumentPromptTarget(target), spec)
}

func BuildXLSXPrompt(description string, target XLSXTarget) string {
	return fmt.Sprintf(`Generate a JSON structure for a professional Excel workbook.

Request: %s
%s

Return JSON only in exactly this shape:
{
  "title": "Workbook title",
  "subtitle": "One-sentence framing",
  "theme": {"preset": "analysis"},
  "sheets": [
    {
      "name":"Executive Summary",
      "purpose":"What this sheet helps answer",
      "summary":[{"label":"Top KPI","value":"$12.4M"}],
      "columns":[
        {"label":"Region","type":"string"},
        {"label":"Revenue","type":"currency"},
        {"label":"YoY","type":"percent"}
      ],
      "rows":[
        ["North America","1280000","12%%"],
        ["Europe","960000","8%%"]
      ],
      "showTotals": true
    }
  ]
}

Requirements:
- Build a workbook that feels analysis-ready, not raw CSV dumped into Excel
- Use typed columns: string, number, currency, percent, date, bool, or formula
- Keep every row aligned with the column definitions
- Include summary items when the request suggests KPI framing
- Use 1-3 sheets unless the request clearly needs more`, description, FormatDocumentPromptTarget(target))
}

func BuildXLSXBestSpecPrompt(description string, target XLSXTarget) string {
	return fmt.Sprintf(`First produce a blueprint for a professional Excel workbook.

Request: %s
%s

Return JSON only:
{
  "title":"Workbook title",
  "subtitle":"Workbook framing",
  "theme":{"preset":"analysis"},
  "sheets":[
    {
      "name":"Sheet1",
      "purpose":"What analysis this sheet covers",
      "summary":[{"label":"KPI","value":"Target metric"}],
      "columns":[{"label":"Column 1","type":"string"},{"label":"Column 2","type":"currency"}],
      "showTotals": true
    }
  ]
}

Requirements:
- Plan the workbook structure before filling in data
- Keep the sheet list compact and decision-oriented
- Use column types that match the intended analysis`, description, FormatDocumentPromptTarget(target))
}

func BuildXLSXBestDraftPrompt(description string, target XLSXTarget, spec string) string {
	return fmt.Sprintf(`Generate the final Excel workbook JSON from the request and workbook blueprint below.

Request: %s
%s

Workbook blueprint:
%s

Return JSON in the same schema as the XLSX workbook payload.

Requirements:
- Match the blueprint's sheet goals and column definitions
- Provide at least 2 valid data rows per sheet when the sheet is tabular
- Do not output anything outside the JSON object`, description, FormatDocumentPromptTarget(target), spec)
}

func BuildWorkbookReportPrompt(description string, target ReportTarget, workbookSummary, baseReportJSON string) string {
	return fmt.Sprintf(`Generate a JSON structure for a narrative business report from workbook data.

Request: %s
%s

Workbook summary:
%s

Base report draft JSON:
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
- Treat the workbook as the source of truth. Do not invent metrics, categories, findings, or conclusions unsupported by the workbook data.
- Keep all chart categories, chart series values, supporting tables, and appendix tables aligned with the workbook-backed base draft.
- This is a report generation workflow, not a generic HTML page generation task. HTML is only the delivery format.
- Make it a narrative report for external business readers, not an admin dashboard
- Use chart types only from: bar, stacked_bar, line, area, donut, scatter, waterfall
- Include 3-5 KPIs, 3-5 findings, and 2-4 analysis sections
- Every section must combine narrative explanation with at least one chart or supporting table
- Focus edits on title, subtitle, summary, findings, section narrative, and takeaways while preserving workbook-backed evidence`, description, FormatDocumentPromptTarget(target), workbookSummary, baseReportJSON)
}

func BuildReportBestSpecPrompt(description string, target ReportTarget) string {
	return fmt.Sprintf(`First produce a structural blueprint for the following workbook-backed report request.

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
- Design a narrative report for external readers that will be backed by workbook data
- Keep the structure compact, executive-friendly, and chart-driven
- Make every section decision-oriented`, description, FormatDocumentPromptTarget(target))
}

func BuildReportBestDraftPrompt(description string, target ReportTarget, spec string) string {
	return fmt.Sprintf(`Generate the final workbook-backed report JSON from the request and blueprint below.

Request: %s
%s

Blueprint:
%s

Return JSON in exactly the same shape as the report schema.

Requirements:
- Keep the narrative sharp and business-facing
- Keep charts and numeric framing evidence-based and workbook-consistent
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
	fileBytes, fileName, _, _, err := BuildDOCXArtifactFromJSON(content, fallbackDescription, "", false)
	return fileBytes, fileName, err
}

func BuildDOCXArtifactFromJSON(content, fallbackDescription, style string, localPreview bool) ([]byte, string, []byte, []byte, error) {
	spec, title, err := parseDOCXSpec(content, fallbackDescription)
	if err != nil {
		return nil, "", nil, nil, err
	}
	fileBytes, err := officegen.NewDOCXGenerator().GenerateSpec(spec, officegen.DOCXOptions{
		Title:   title,
		Creator: "OfficeCLI",
		Style:   style,
	})
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("generate docx: %w", err)
	}
	var previewHTML []byte
	var previewJSON []byte
	if localPreview {
		previewJSON, _ = officegen.BuildDOCXPreviewJSON(spec, style)
		previewHTML = officegen.BuildDOCXPreviewHTML(spec, style)
	}
	return fileBytes, fmt.Sprintf("%s.docx", SanitizeFileName(title)), previewHTML, previewJSON, nil
}

func BuildXLSXFromJSON(content, fallbackDescription string) ([]byte, string, error) {
	fileBytes, fileName, _, _, err := BuildXLSXArtifactFromJSON(content, fallbackDescription, "", false)
	return fileBytes, fileName, err
}

func BuildXLSXArtifactFromJSON(content, fallbackDescription, style string, localPreview bool) ([]byte, string, []byte, []byte, error) {
	spec, title, err := parseXLSXSpec(content, fallbackDescription)
	if err != nil {
		return nil, "", nil, nil, err
	}
	fileBytes, err := officegen.NewXLSXGenerator().GenerateWorkbook(spec, officegen.XLSXOptions{
		Title:   title,
		Creator: "OfficeCLI",
		Style:   style,
	})
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("generate xlsx: %w", err)
	}
	var previewHTML []byte
	var previewJSON []byte
	if localPreview {
		previewJSON, _ = officegen.BuildXLSXPreviewJSON(spec, style)
		previewHTML = officegen.BuildXLSXPreviewHTML(spec, style)
	}
	return fileBytes, fmt.Sprintf("%s.xlsx", SanitizeFileName(title)), previewHTML, previewJSON, nil
}

func parseDOCXSpec(content, fallbackDescription string) (officegen.DocxDocumentSpec, string, error) {
	content = RepairUnescapedQuotes(ExtractJSON(content))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return officegen.DocxDocumentSpec{}, "", fmt.Errorf("parse LLM response: %w", err)
	}

	var spec officegen.DocxDocumentSpec
	if _, ok := raw["blocks"]; ok {
		if err := json.Unmarshal([]byte(content), &spec); err != nil {
			return officegen.DocxDocumentSpec{}, "", fmt.Errorf("parse docx spec: %w", err)
		}
	} else {
		var legacy struct {
			Title    string `json:"title"`
			Subtitle string `json:"subtitle"`
			Sections []struct {
				Heading    string   `json:"heading"`
				Level      int      `json:"level"`
				Paragraphs []string `json:"paragraphs"`
			} `json:"sections"`
		}
		if err := json.Unmarshal([]byte(content), &legacy); err != nil {
			return officegen.DocxDocumentSpec{}, "", fmt.Errorf("parse legacy docx payload: %w", err)
		}
		spec.Title = strings.TrimSpace(legacy.Title)
		spec.Subtitle = strings.TrimSpace(legacy.Subtitle)
		for _, sec := range legacy.Sections {
			level := sec.Level
			if level <= 0 {
				level = 2
			}
			spec.Blocks = append(spec.Blocks, officegen.DocxBlock{
				Type:  "heading",
				Level: level,
				Text:  strings.TrimSpace(sec.Heading),
			})
			for _, paragraph := range sec.Paragraphs {
				spec.Blocks = append(spec.Blocks, officegen.DocxBlock{
					Type: "paragraph",
					Text: strings.TrimSpace(paragraph),
				})
			}
		}
	}
	title := strings.TrimSpace(spec.Title)
	if title == "" {
		title = ExtractTitleFromDescription(fallbackDescription)
		spec.Title = title
	}
	return spec, title, nil
}

func parseXLSXSpec(content, fallbackDescription string) (officegen.XlsxWorkbookSpec, string, error) {
	content = RepairUnescapedQuotes(ExtractJSON(content))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return officegen.XlsxWorkbookSpec{}, "", fmt.Errorf("parse LLM response: %w", err)
	}

	var spec officegen.XlsxWorkbookSpec
	if hasXLSXSemanticSheets(raw) {
		var input xlsxWorkbookSpecInput
		if err := json.Unmarshal([]byte(content), &input); err != nil {
			return officegen.XlsxWorkbookSpec{}, "", fmt.Errorf("parse xlsx spec: %w", err)
		}
		spec = workbookSpecFromInput(input)
	} else {
		var legacy struct {
			Title  string `json:"title"`
			Sheets []struct {
				Name    string               `json:"name"`
				Headers []string             `json:"headers"`
				Rows    [][]jsonScalarString `json:"rows"`
			} `json:"sheets"`
		}
		if err := json.Unmarshal([]byte(content), &legacy); err != nil {
			return officegen.XlsxWorkbookSpec{}, "", fmt.Errorf("parse legacy xlsx payload: %w", err)
		}
		spec.Title = strings.TrimSpace(legacy.Title)
		for _, sheet := range legacy.Sheets {
			columns := make([]officegen.XlsxColumn, 0, len(sheet.Headers))
			for _, header := range sheet.Headers {
				columns = append(columns, officegen.XlsxColumn{Label: strings.TrimSpace(header)})
			}
			spec.Sheets = append(spec.Sheets, officegen.XlsxSheetSpec{
				Name:       strings.TrimSpace(sheet.Name),
				Columns:    columns,
				Rows:       stringifyXLSXRows(sheet.Rows),
				AutoFilter: true,
			})
		}
	}
	title := strings.TrimSpace(spec.Title)
	if title == "" {
		title = ExtractTitleFromDescription(fallbackDescription)
		spec.Title = title
	}
	return spec, title, nil
}

func hasXLSXSemanticSheets(raw map[string]json.RawMessage) bool {
	var envelopes []map[string]json.RawMessage
	if data, ok := raw["sheets"]; ok {
		if err := json.Unmarshal(data, &envelopes); err == nil && len(envelopes) > 0 {
			if _, ok := envelopes[0]["columns"]; ok {
				return true
			}
		}
	}
	return false
}

func workbookSpecFromInput(input xlsxWorkbookSpecInput) officegen.XlsxWorkbookSpec {
	spec := officegen.XlsxWorkbookSpec{
		Title:    input.Title,
		Subtitle: input.Subtitle,
		Theme:    input.Theme,
		Sheets:   make([]officegen.XlsxSheetSpec, 0, len(input.Sheets)),
	}
	for _, sheet := range input.Sheets {
		spec.Sheets = append(spec.Sheets, officegen.XlsxSheetSpec{
			Name:       sheet.Name,
			Purpose:    sheet.Purpose,
			Columns:    append([]officegen.XlsxColumn(nil), sheet.Columns...),
			Rows:       stringifyXLSXRows(sheet.Rows),
			Summary:    stringifyXLSXSummary(sheet.Summary),
			Freeze:     sheet.Freeze,
			AutoFilter: sheet.AutoFilter,
			ShowTotals: sheet.ShowTotals,
		})
	}
	return spec
}

func stringifyXLSXRows(rows [][]jsonScalarString) [][]string {
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		cells := make([]string, 0, len(row))
		for _, cell := range row {
			cells = append(cells, string(cell))
		}
		out = append(out, cells)
	}
	return out
}

func stringifyXLSXSummary(items []xlsxSummaryInput) []officegen.XlsxSummaryItem {
	out := make([]officegen.XlsxSummaryItem, 0, len(items))
	for _, item := range items {
		out = append(out, officegen.XlsxSummaryItem{
			Label: string(item.Label),
			Value: string(item.Value),
		})
	}
	return out
}

func BuildReportFromJSON(content, fallbackDescription string) ([]byte, string, error) {
	content = RepairUnescapedQuotes(ExtractJSON(content))

	var report officegen.Report
	if err := json.Unmarshal([]byte(content), &report); err != nil {
		return nil, "", fmt.Errorf("parse LLM response: %w", err)
	}

	fileBytes, err := officegen.BuildReport(report)
	if err != nil {
		return nil, "", fmt.Errorf("generate html: %w", err)
	}

	title := strings.TrimSpace(report.Title)
	if title == "" {
		title = ExtractTitleFromDescription(fallbackDescription)
	}
	return fileBytes, fmt.Sprintf("%s.html", SanitizeFileName(title)), nil
}

func BuildReportPrompt(description string, target ReportTarget) string {
	return BuildWorkbookReportPrompt(description, target, "No workbook summary provided.", "{}")
}
