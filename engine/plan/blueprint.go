package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/officecli/officecli-internal/engine"
)

type frameworkBlueprint struct {
	PresentationType    string                   `json:"presentationType"`
	TargetAudience      string                   `json:"targetAudience"`
	PresentationPurpose string                   `json:"presentationPurpose"`
	PageCount           int                      `json:"pageCount"`
	ContentStyle        string                   `json:"contentStyle"`
	VisualEffect        string                   `json:"visualEffect"`
	ContentGuideline    string                   `json:"contentGuideline"`
	SlideOutline        []frameworkBlueprintPage `json:"slideOutline"`
}

type frameworkBlueprintPage struct {
	SlideIndex          int    `json:"slideIndex"`
	Purpose             string `json:"purpose"`
	SuggestedLayout     string `json:"suggestedLayout"`
	ContentFormat       string `json:"contentFormat"`
	MaxItems            int    `json:"maxItems"`
	ContentRequirements string `json:"contentRequirements"`
	VisualSuggestion    string `json:"visualSuggestion"`
}

type documentBlueprint struct {
	DocumentType     string                     `json:"documentType"`
	TargetAudience   string                     `json:"targetAudience"`
	WritingGoal      string                     `json:"writingGoal"`
	Tone             string                     `json:"tone"`
	LengthHint       string                     `json:"lengthHint"`
	ContentGuideline string                     `json:"contentGuideline"`
	Sections         []documentBlueprintSection `json:"sections"`
}

type documentBlueprintSection struct {
	SectionIndex int      `json:"sectionIndex"`
	Heading      string   `json:"heading"`
	Purpose      string   `json:"purpose"`
	KeyPoints    []string `json:"keyPoints"`
	LengthHint   string   `json:"lengthHint"`
}

type workbookBlueprint struct {
	WorkbookType     string                   `json:"workbookType"`
	TargetAudience   string                   `json:"targetAudience"`
	AnalysisGoal     string                   `json:"analysisGoal"`
	SummaryStyle     string                   `json:"summaryStyle"`
	ContentGuideline string                   `json:"contentGuideline"`
	Sheets           []workbookBlueprintSheet `json:"sheets"`
}

type workbookBlueprintSheet struct {
	SheetIndex int      `json:"sheetIndex"`
	Name       string   `json:"name"`
	Purpose    string   `json:"purpose"`
	Columns    []string `json:"columns"`
	Notes      string   `json:"notes"`
}

type reportBlueprint struct {
	ReportType       string                   `json:"reportType"`
	TargetAudience   string                   `json:"targetAudience"`
	StoryGoal        string                   `json:"storyGoal"`
	ChartDensity     string                   `json:"chartDensity"`
	ContentGuideline string                   `json:"contentGuideline"`
	Sections         []reportBlueprintSection `json:"sections"`
}

type reportBlueprintSection struct {
	SectionIndex int      `json:"sectionIndex"`
	Title        string   `json:"title"`
	Purpose      string   `json:"purpose"`
	ChartIntent  string   `json:"chartIntent"`
	Takeaways    []string `json:"takeaways"`
}

func (w *Workflow) synthesizeFrameworkBlueprint(ctx context.Context, session *engine.PlanSession) (string, error) {
	if w == nil || w.llm == nil {
		return "", fmt.Errorf("llm unavailable")
	}
	attemptCtx, cancel := w.withTimeout(ctx, w.blueprintTimeout)
	response, err := w.llm.CompleteJSON(attemptCtx, []engine.LLMMessage{
		{Role: "system", Content: "You are a senior office-document structure designer. Return one valid JSON object only."},
		{Role: "user", Content: buildFrameworkBlueprintPrompt(session)},
	})
	cancel()
	if err != nil {
		return "", err
	}
	return buildFrameworkBlueprintMarkdown(session.DocumentType, response)
}

func buildFrameworkBlueprintPrompt(session *engine.PlanSession) string {
	var sb strings.Builder
	sb.WriteString("Generate a framework blueprint for the request below. Return JSON only.\n\n")
	sb.WriteString("Request: ")
	sb.WriteString(strings.TrimSpace(session.UserPrompt))
	sb.WriteString("\n")
	for _, answer := range session.Answers {
		question := findQuestion(session.Questions, answer.QuestionID)
		if question != nil {
			sb.WriteString("Additional note: ")
			sb.WriteString(strings.TrimSpace(question.Question))
			sb.WriteString(": ")
			sb.WriteString(strings.TrimSpace(answer.Answer))
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")
	sb.WriteString(buildBlueprintSchemaPrompt(session.DocumentType))
	return sb.String()
}

func buildBlueprintSchemaPrompt(documentType string) string {
	switch normalizeDocumentType(documentType) {
	case "docx":
		return `Output format:
{
  "documentType":"Analytical report",
  "targetAudience":"Leadership",
  "writingGoal":"Explain conclusions and recommendations",
  "tone":"Formal and professional",
  "lengthHint":"Around 3000 words",
  "contentGuideline":"Lead with conclusions before analysis and avoid filler",
  "sections":[
    {
      "sectionIndex":1,
      "heading":"Executive summary",
      "purpose":"Present the core conclusion first",
      "keyPoints":["Conclusion","Recommendation"],
      "lengthHint":"300 words"
    }
  ]
}

Requirements:
1. Keep the section order complete so the result can be expanded into a formal document directly.
2. Explain the role of each section clearly instead of listing headings only.
3. contentGuideline must include expectations for length and expression quality.`
	case "xlsx":
		return `Output format:
{
  "workbookType":"Business analysis",
  "targetAudience":"Leadership",
  "analysisGoal":"Track revenue and budget variance",
  "summaryStyle":"Summary first, details second",
  "contentGuideline":"Keep field definitions consistent and align summary with detail",
  "sheets":[
    {
      "sheetIndex":1,
      "name":"Summary",
      "purpose":"Executive summary",
      "columns":["Month","Revenue","Budget variance"],
      "notes":"Keep only the core KPIs"
    }
  ]
}

Requirements:
1. Define workbook and sheet responsibilities before finalizing columns.
2. notes must describe the focus and limits of the sheet.
3. contentGuideline must reflect consistency of definitions and the relationship between summary and detail.`
	case "report":
		return `Output format:
{
  "reportType":"Business review",
  "targetAudience":"Board and investors",
  "storyGoal":"Explain the latest performance shift and what should happen next",
  "chartDensity":"Balanced",
  "contentGuideline":"Lead with the headline, keep every section evidence-based, and end with a decision implication",
  "sections":[
    {
      "sectionIndex":1,
      "title":"Executive summary",
      "purpose":"Frame the headline change and why it matters",
      "chartIntent":"Show the single most important comparison first",
      "takeaways":["Performance shift","Decision implication"]
    }
  ]
}

Requirements:
1. Design a narrative workbook-backed report for external business readers rather than an admin dashboard.
2. Each section must define both the story purpose and what the chart needs to prove from workbook data.
3. contentGuideline must reflect chart discipline, narrative clarity, and external readability.`
	default:
		return `Output format:
{
  "presentationType":"Project update",
  "targetAudience":"Leadership",
  "presentationPurpose":"Communicate core conclusions and recommendations",
  "pageCount":6,
  "contentStyle":"Conclusion-first",
  "visualEffect":"Clean and credible",
  "contentGuideline":"Keep one core point per slide and avoid repetitive slides",
  "slideOutline":[
    {
      "slideIndex":1,
      "purpose":"Cover",
      "suggestedLayout":"title",
      "contentFormat":"paragraph",
      "maxItems":1,
      "contentRequirements":"State the theme and audience",
      "visualSuggestion":"hero"
    }
  ]
}

Requirements:
1. Favor a concise consulting-style deck, typically 6-8 slides.
2. slideOutline must capture slide purpose, narrative order, expression style, and information cap.
3. contentGuideline must reflect conclusion-first structure, anti-repetition, and per-slide density limits.`
	}
}

func buildFrameworkBlueprintMarkdown(documentType string, specJSON string) (string, error) {
	switch normalizeDocumentType(documentType) {
	case "docx":
		return buildDocumentBlueprintMarkdown(specJSON)
	case "xlsx":
		return buildWorkbookBlueprintMarkdown(specJSON)
	case "report":
		return buildReportBlueprintMarkdown(specJSON)
	default:
		return buildPresentationBlueprintMarkdown(specJSON)
	}
}

func buildPresentationBlueprintMarkdown(specJSON string) (string, error) {
	var blueprint frameworkBlueprint
	if err := json.Unmarshal([]byte(specJSON), &blueprint); err != nil {
		return "", fmt.Errorf("decode framework blueprint: %w", err)
	}
	var sb strings.Builder
	sb.WriteString("## Framework Blueprint\n")
	overview := make([]string, 0, 7)
	if value := strings.TrimSpace(blueprint.PresentationType); value != "" {
		overview = append(overview, "- Presentation type: "+value)
	}
	if value := strings.TrimSpace(blueprint.TargetAudience); value != "" {
		overview = append(overview, "- Audience: "+value)
	}
	if value := strings.TrimSpace(blueprint.PresentationPurpose); value != "" {
		overview = append(overview, "- Goal: "+value)
	}
	if blueprint.PageCount > 0 {
		overview = append(overview, "- Suggested pages: "+strconv.Itoa(blueprint.PageCount))
	}
	if value := strings.TrimSpace(blueprint.ContentStyle); value != "" {
		overview = append(overview, "- Content style: "+value)
	}
	if value := strings.TrimSpace(blueprint.VisualEffect); value != "" {
		overview = append(overview, "- Visual direction: "+value)
	}
	if value := strings.TrimSpace(blueprint.ContentGuideline); value != "" {
		overview = append(overview, "- Content guideline: "+value)
	}
	if len(overview) > 0 {
		sb.WriteString("\n### Overview\n")
		sb.WriteString(strings.Join(overview, "\n"))
		sb.WriteString("\n")
	}
	if len(blueprint.SlideOutline) > 0 {
		sb.WriteString("\n### Slide Plan\n")
		for idx, slide := range blueprint.SlideOutline {
			pageIndex := slide.SlideIndex
			if pageIndex <= 0 {
				pageIndex = idx + 1
			}
			sb.WriteString("- Slide ")
			sb.WriteString(strconv.Itoa(pageIndex))
			sb.WriteString(": ")
			sb.WriteString(strings.TrimSpace(fallbackText(slide.Purpose, "TBD")))
			parts := make([]string, 0, 4)
			if value := strings.TrimSpace(slide.ContentFormat); value != "" {
				parts = append(parts, "format "+value)
			}
			if value := strings.TrimSpace(slide.SuggestedLayout); value != "" {
				parts = append(parts, "layout "+value)
			}
			if slide.MaxItems > 0 {
				parts = append(parts, "max items "+strconv.Itoa(slide.MaxItems))
			}
			if value := strings.TrimSpace(slide.ContentRequirements); value != "" {
				parts = append(parts, "focus "+value)
			}
			if value := strings.TrimSpace(slide.VisualSuggestion); value != "" {
				parts = append(parts, "visual "+value)
			}
			if len(parts) > 0 {
				sb.WriteString(" (")
				sb.WriteString(strings.Join(parts, "; "))
				sb.WriteString(")")
			}
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

func buildDocumentBlueprintMarkdown(specJSON string) (string, error) {
	var blueprint documentBlueprint
	if err := json.Unmarshal([]byte(specJSON), &blueprint); err != nil {
		return "", fmt.Errorf("decode document blueprint: %w", err)
	}
	var sb strings.Builder
	sb.WriteString("## Framework Blueprint\n")
	overview := make([]string, 0, 6)
	if value := strings.TrimSpace(blueprint.DocumentType); value != "" {
		overview = append(overview, "- Document type: "+value)
	}
	if value := strings.TrimSpace(blueprint.TargetAudience); value != "" {
		overview = append(overview, "- Audience: "+value)
	}
	if value := strings.TrimSpace(blueprint.WritingGoal); value != "" {
		overview = append(overview, "- Goal: "+value)
	}
	if value := strings.TrimSpace(blueprint.Tone); value != "" {
		overview = append(overview, "- Tone: "+value)
	}
	if value := strings.TrimSpace(blueprint.LengthHint); value != "" {
		overview = append(overview, "- Length hint: "+value)
	}
	if value := strings.TrimSpace(blueprint.ContentGuideline); value != "" {
		overview = append(overview, "- Content guideline: "+value)
	}
	if len(overview) > 0 {
		sb.WriteString("\n### Overview\n")
		sb.WriteString(strings.Join(overview, "\n"))
		sb.WriteString("\n")
	}
	if len(blueprint.Sections) > 0 {
		sb.WriteString("\n### Section Plan\n")
		for idx, section := range blueprint.Sections {
			sectionIndex := section.SectionIndex
			if sectionIndex <= 0 {
				sectionIndex = idx + 1
			}
			sb.WriteString("- Section ")
			sb.WriteString(strconv.Itoa(sectionIndex))
			sb.WriteString(": ")
			sb.WriteString(fallbackText(section.Heading, "TBD"))
			parts := make([]string, 0, 3)
			if value := strings.TrimSpace(section.Purpose); value != "" {
				parts = append(parts, "purpose "+value)
			}
			if len(section.KeyPoints) > 0 {
				parts = append(parts, "key points "+strings.Join(section.KeyPoints, ", "))
			}
			if value := strings.TrimSpace(section.LengthHint); value != "" {
				parts = append(parts, "length "+value)
			}
			if len(parts) > 0 {
				sb.WriteString(" (")
				sb.WriteString(strings.Join(parts, "; "))
				sb.WriteString(")")
			}
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

func buildWorkbookBlueprintMarkdown(specJSON string) (string, error) {
	var blueprint workbookBlueprint
	if err := json.Unmarshal([]byte(specJSON), &blueprint); err != nil {
		return "", fmt.Errorf("decode workbook blueprint: %w", err)
	}
	var sb strings.Builder
	sb.WriteString("## Framework Blueprint\n")
	overview := make([]string, 0, 5)
	if value := strings.TrimSpace(blueprint.WorkbookType); value != "" {
		overview = append(overview, "- Workbook type: "+value)
	}
	if value := strings.TrimSpace(blueprint.TargetAudience); value != "" {
		overview = append(overview, "- Audience: "+value)
	}
	if value := strings.TrimSpace(blueprint.AnalysisGoal); value != "" {
		overview = append(overview, "- Analysis goal: "+value)
	}
	if value := strings.TrimSpace(blueprint.SummaryStyle); value != "" {
		overview = append(overview, "- Output style: "+value)
	}
	if value := strings.TrimSpace(blueprint.ContentGuideline); value != "" {
		overview = append(overview, "- Content guideline: "+value)
	}
	if len(overview) > 0 {
		sb.WriteString("\n### Overview\n")
		sb.WriteString(strings.Join(overview, "\n"))
		sb.WriteString("\n")
	}
	if len(blueprint.Sheets) > 0 {
		sb.WriteString("\n### Workbook Plan\n")
		for idx, sheet := range blueprint.Sheets {
			sheetIndex := sheet.SheetIndex
			if sheetIndex <= 0 {
				sheetIndex = idx + 1
			}
			sb.WriteString("- Sheet ")
			sb.WriteString(strconv.Itoa(sheetIndex))
			sb.WriteString(" (")
			sb.WriteString(fallbackText(sheet.Name, "Unnamed"))
			sb.WriteString("): ")
			sb.WriteString(fallbackText(sheet.Purpose, "TBD"))
			parts := make([]string, 0, 2)
			if len(sheet.Columns) > 0 {
				parts = append(parts, "columns "+strings.Join(sheet.Columns, ", "))
			}
			if value := strings.TrimSpace(sheet.Notes); value != "" {
				parts = append(parts, "notes "+value)
			}
			if len(parts) > 0 {
				sb.WriteString(" (")
				sb.WriteString(strings.Join(parts, "; "))
				sb.WriteString(")")
			}
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

func buildReportBlueprintMarkdown(specJSON string) (string, error) {
	var blueprint reportBlueprint
	if err := json.Unmarshal([]byte(specJSON), &blueprint); err != nil {
		return "", fmt.Errorf("decode report blueprint: %w", err)
	}
	var sb strings.Builder
	sb.WriteString("## Framework Blueprint\n")
	overview := make([]string, 0, 5)
	if value := strings.TrimSpace(blueprint.ReportType); value != "" {
		overview = append(overview, "- Report type: "+value)
	}
	if value := strings.TrimSpace(blueprint.TargetAudience); value != "" {
		overview = append(overview, "- Audience: "+value)
	}
	if value := strings.TrimSpace(blueprint.StoryGoal); value != "" {
		overview = append(overview, "- Story goal: "+value)
	}
	if value := strings.TrimSpace(blueprint.ChartDensity); value != "" {
		overview = append(overview, "- Chart density: "+value)
	}
	if value := strings.TrimSpace(blueprint.ContentGuideline); value != "" {
		overview = append(overview, "- Content guideline: "+value)
	}
	if len(overview) > 0 {
		sb.WriteString("\n### Overview\n")
		sb.WriteString(strings.Join(overview, "\n"))
		sb.WriteString("\n")
	}
	if len(blueprint.Sections) > 0 {
		sb.WriteString("\n### Section Plan\n")
		for idx, section := range blueprint.Sections {
			sectionIndex := section.SectionIndex
			if sectionIndex <= 0 {
				sectionIndex = idx + 1
			}
			sb.WriteString("- Section ")
			sb.WriteString(strconv.Itoa(sectionIndex))
			sb.WriteString(": ")
			sb.WriteString(fallbackText(section.Title, "Untitled"))
			parts := make([]string, 0, 3)
			if value := strings.TrimSpace(section.Purpose); value != "" {
				parts = append(parts, "purpose "+value)
			}
			if value := strings.TrimSpace(section.ChartIntent); value != "" {
				parts = append(parts, "chart proves "+value)
			}
			if len(section.Takeaways) > 0 {
				parts = append(parts, "takeaways "+strings.Join(section.Takeaways, ", "))
			}
			if len(parts) > 0 {
				sb.WriteString(" (")
				sb.WriteString(strings.Join(parts, "; "))
				sb.WriteString(")")
			}
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

func fallbackText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
