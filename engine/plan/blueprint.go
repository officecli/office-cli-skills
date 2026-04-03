package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/officecli/officecli/engine"
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

func (w *Workflow) synthesizeFrameworkBlueprint(ctx context.Context, session *engine.PlanSession) (string, error) {
	if w == nil || w.llm == nil {
		return "", fmt.Errorf("llm unavailable")
	}
	attemptCtx, cancel := w.withTimeout(ctx, w.blueprintTimeout)
	response, err := w.llm.CompleteJSON(attemptCtx, []engine.LLMMessage{
		{Role: "system", Content: "你是一个资深办公文档结构设计师。请只返回一个合法 JSON 对象。"},
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
	sb.WriteString("请先为以下需求生成结构蓝图，只输出 JSON。\n\n")
	sb.WriteString("需求：")
	sb.WriteString(strings.TrimSpace(session.UserPrompt))
	sb.WriteString("\n")
	for _, answer := range session.Answers {
		question := findQuestion(session.Questions, answer.QuestionID)
		if question != nil {
			sb.WriteString("补充说明：")
			sb.WriteString(strings.TrimSpace(question.Question))
			sb.WriteString("：")
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
		return `输出格式：
{
  "documentType":"分析报告",
  "targetAudience":"管理层",
  "writingGoal":"说明结论与建议",
  "tone":"正式专业",
  "lengthHint":"约 3000 字",
  "contentGuideline":"先结论后分析，避免空话",
  "sections":[
    {
      "sectionIndex":1,
      "heading":"摘要",
      "purpose":"先给核心结论",
      "keyPoints":["结论","建议"],
      "lengthHint":"300 字"
    }
  ]
}

要求：
1. 章节顺序完整，适合直接扩写成正式文档。
2. 每节都要写清楚作用，不要只列标题。
3. contentGuideline 必须体现篇幅和表达质量要求。`
	case "xlsx":
		return `输出格式：
{
  "workbookType":"经营分析",
  "targetAudience":"管理层",
  "analysisGoal":"跟踪收入与预算偏差",
  "summaryStyle":"先摘要后明细",
  "contentGuideline":"字段口径统一，摘要与明细对应",
  "sheets":[
    {
      "sheetIndex":1,
      "name":"Summary",
      "purpose":"管理摘要",
      "columns":["月份","收入","预算偏差"],
      "notes":"保留核心 KPI"
    }
  ]
}

要求：
1. 先规划 workbook 和 sheet 职责，再补充字段设计。
2. notes 要说明该 sheet 的重点和限制。
3. contentGuideline 必须体现口径一致和摘要/明细关系。`
	default:
		return `输出格式：
{
  "presentationType":"项目汇报",
  "targetAudience":"管理层",
  "presentationPurpose":"同步核心结论与建议",
  "pageCount":6,
  "contentStyle":"结论先行",
  "visualEffect":"简洁可信",
  "contentGuideline":"每页只保留一个核心信息点，避免重复页",
  "slideOutline":[
    {
      "slideIndex":1,
      "purpose":"封面",
      "suggestedLayout":"title",
      "contentFormat":"paragraph",
      "maxItems":1,
      "contentRequirements":"说明主题与汇报对象",
      "visualSuggestion":"hero"
    }
  ]
}

要求：
1. 优先输出咨询风短 deck，默认 6-8 页。
2. slideOutline 必须体现页职责、叙事顺序、内容表达方式和信息上限。
3. contentGuideline 必须体现结论先行、避免重复和单页信息密度限制。`
	}
}

func buildFrameworkBlueprintMarkdown(documentType string, specJSON string) (string, error) {
	switch normalizeDocumentType(documentType) {
	case "docx":
		return buildDocumentBlueprintMarkdown(specJSON)
	case "xlsx":
		return buildWorkbookBlueprintMarkdown(specJSON)
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
	sb.WriteString("## 框架蓝图\n")
	overview := make([]string, 0, 7)
	if value := strings.TrimSpace(blueprint.PresentationType); value != "" {
		overview = append(overview, "- 演示类型："+value)
	}
	if value := strings.TrimSpace(blueprint.TargetAudience); value != "" {
		overview = append(overview, "- 受众："+value)
	}
	if value := strings.TrimSpace(blueprint.PresentationPurpose); value != "" {
		overview = append(overview, "- 输出目标："+value)
	}
	if blueprint.PageCount > 0 {
		overview = append(overview, "- 建议页数："+strconv.Itoa(blueprint.PageCount)+" 页")
	}
	if value := strings.TrimSpace(blueprint.ContentStyle); value != "" {
		overview = append(overview, "- 内容风格："+value)
	}
	if value := strings.TrimSpace(blueprint.VisualEffect); value != "" {
		overview = append(overview, "- 视觉方向："+value)
	}
	if value := strings.TrimSpace(blueprint.ContentGuideline); value != "" {
		overview = append(overview, "- 内容原则："+value)
	}
	if len(overview) > 0 {
		sb.WriteString("\n### 蓝图概览\n")
		sb.WriteString(strings.Join(overview, "\n"))
		sb.WriteString("\n")
	}
	if len(blueprint.SlideOutline) > 0 {
		sb.WriteString("\n### 页级规划\n")
		for idx, slide := range blueprint.SlideOutline {
			pageIndex := slide.SlideIndex
			if pageIndex <= 0 {
				pageIndex = idx + 1
			}
			sb.WriteString("- 第 ")
			sb.WriteString(strconv.Itoa(pageIndex))
			sb.WriteString(" 页：")
			sb.WriteString(strings.TrimSpace(fallbackText(slide.Purpose, "待定")))
			parts := make([]string, 0, 4)
			if value := strings.TrimSpace(slide.ContentFormat); value != "" {
				parts = append(parts, "表达 "+value)
			}
			if value := strings.TrimSpace(slide.SuggestedLayout); value != "" {
				parts = append(parts, "布局 "+value)
			}
			if slide.MaxItems > 0 {
				parts = append(parts, "信息上限 "+strconv.Itoa(slide.MaxItems)+" 项")
			}
			if value := strings.TrimSpace(slide.ContentRequirements); value != "" {
				parts = append(parts, "重点 "+value)
			}
			if value := strings.TrimSpace(slide.VisualSuggestion); value != "" {
				parts = append(parts, "视觉 "+value)
			}
			if len(parts) > 0 {
				sb.WriteString("（")
				sb.WriteString(strings.Join(parts, "；"))
				sb.WriteString("）")
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
	sb.WriteString("## 框架蓝图\n")
	overview := make([]string, 0, 6)
	if value := strings.TrimSpace(blueprint.DocumentType); value != "" {
		overview = append(overview, "- 文档类型："+value)
	}
	if value := strings.TrimSpace(blueprint.TargetAudience); value != "" {
		overview = append(overview, "- 受众："+value)
	}
	if value := strings.TrimSpace(blueprint.WritingGoal); value != "" {
		overview = append(overview, "- 输出目标："+value)
	}
	if value := strings.TrimSpace(blueprint.Tone); value != "" {
		overview = append(overview, "- 写作风格："+value)
	}
	if value := strings.TrimSpace(blueprint.LengthHint); value != "" {
		overview = append(overview, "- 篇幅建议："+value)
	}
	if value := strings.TrimSpace(blueprint.ContentGuideline); value != "" {
		overview = append(overview, "- 内容原则："+value)
	}
	if len(overview) > 0 {
		sb.WriteString("\n### 蓝图概览\n")
		sb.WriteString(strings.Join(overview, "\n"))
		sb.WriteString("\n")
	}
	if len(blueprint.Sections) > 0 {
		sb.WriteString("\n### 章节规划\n")
		for idx, section := range blueprint.Sections {
			sectionIndex := section.SectionIndex
			if sectionIndex <= 0 {
				sectionIndex = idx + 1
			}
			sb.WriteString("- 第 ")
			sb.WriteString(strconv.Itoa(sectionIndex))
			sb.WriteString(" 节：")
			sb.WriteString(fallbackText(section.Heading, "待定章节"))
			parts := make([]string, 0, 3)
			if value := strings.TrimSpace(section.Purpose); value != "" {
				parts = append(parts, "作用 "+value)
			}
			if len(section.KeyPoints) > 0 {
				parts = append(parts, "要点 "+strings.Join(section.KeyPoints, "、"))
			}
			if value := strings.TrimSpace(section.LengthHint); value != "" {
				parts = append(parts, "篇幅 "+value)
			}
			if len(parts) > 0 {
				sb.WriteString("（")
				sb.WriteString(strings.Join(parts, "；"))
				sb.WriteString("）")
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
	sb.WriteString("## 框架蓝图\n")
	overview := make([]string, 0, 5)
	if value := strings.TrimSpace(blueprint.WorkbookType); value != "" {
		overview = append(overview, "- 工作簿类型："+value)
	}
	if value := strings.TrimSpace(blueprint.TargetAudience); value != "" {
		overview = append(overview, "- 受众："+value)
	}
	if value := strings.TrimSpace(blueprint.AnalysisGoal); value != "" {
		overview = append(overview, "- 分析目标："+value)
	}
	if value := strings.TrimSpace(blueprint.SummaryStyle); value != "" {
		overview = append(overview, "- 输出方式："+value)
	}
	if value := strings.TrimSpace(blueprint.ContentGuideline); value != "" {
		overview = append(overview, "- 内容原则："+value)
	}
	if len(overview) > 0 {
		sb.WriteString("\n### 蓝图概览\n")
		sb.WriteString(strings.Join(overview, "\n"))
		sb.WriteString("\n")
	}
	if len(blueprint.Sheets) > 0 {
		sb.WriteString("\n### 工作簿规划\n")
		for idx, sheet := range blueprint.Sheets {
			sheetIndex := sheet.SheetIndex
			if sheetIndex <= 0 {
				sheetIndex = idx + 1
			}
			sb.WriteString("- Sheet ")
			sb.WriteString(strconv.Itoa(sheetIndex))
			sb.WriteString("（")
			sb.WriteString(fallbackText(sheet.Name, "未命名"))
			sb.WriteString("）：")
			sb.WriteString(fallbackText(sheet.Purpose, "待定职责"))
			parts := make([]string, 0, 2)
			if len(sheet.Columns) > 0 {
				parts = append(parts, "字段 "+strings.Join(sheet.Columns, "、"))
			}
			if value := strings.TrimSpace(sheet.Notes); value != "" {
				parts = append(parts, "说明 "+value)
			}
			if len(parts) > 0 {
				sb.WriteString("（")
				sb.WriteString(strings.Join(parts, "；"))
				sb.WriteString("）")
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
