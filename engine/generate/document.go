package generate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/officecli/officecli/pkg/officegen"
)

type DOCXTarget = PromptTarget

type XLSXTarget = PromptTarget

func BuildDOCXPrompt(description string, target DOCXTarget) string {
	return fmt.Sprintf(`请根据以下需求生成一个Word文档的JSON结构。

需求：%s
%s

请严格按照以下JSON格式输出（不要包含任何其他内容）：
{
  "title": "文档标题",
  "sections": [
    {
      "heading": "章节标题",
      "level": 1,
      "paragraphs": ["段落内容1", "段落内容2"]
    }
  ]
}

要求：
- 生成结构完整的文档
- 包含标题、正文、结论等部分
- 内容专业、逻辑清晰`, description, FormatDocumentPromptTarget(target))
}

func BuildDOCXBestSpecPrompt(description string, target DOCXTarget) string {
	return fmt.Sprintf(`请先为以下 Word 文档需求生成“结构蓝图”。

需求：%s
%s

请只输出 JSON：
{
  "title":"文档标题",
  "goal":"文档目标",
  "audience":"读者对象",
  "tone":"写作风格",
  "sections":[
    {"heading":"章节标题","summary":"该章节要覆盖的核心内容"}
  ]
}

要求：
- 只定义结构，不写完整正文
- 章节顺序要完整，适合后续扩写`, description, FormatDocumentPromptTarget(target))
}

func BuildDOCXBestDraftPrompt(description string, target DOCXTarget, spec string) string {
	return fmt.Sprintf(`请根据以下需求和结构蓝图，生成最终 Word 文档 JSON。

需求：%s
%s

结构蓝图：
%s

请严格按照以下 JSON 输出：
{
  "title": "文档标题",
  "sections": [
    {
      "heading": "章节标题",
      "level": 1,
      "paragraphs": ["段落内容1", "段落内容2"]
    }
  ]
}

要求：
- 每个章节都要补全成可直接交付的正文
- 内容避免空话，信息密度高
- 不要输出 JSON 之外的任何文字`, description, FormatDocumentPromptTarget(target), spec)
}

func BuildXLSXPrompt(description string, target XLSXTarget) string {
	return fmt.Sprintf(`请根据以下需求生成一个Excel表格的JSON结构。

需求：%s
%s

请严格按照以下JSON格式输出（不要包含任何其他内容）：
{
  "title": "表格标题",
  "sheets": [
    {
      "name": "Sheet1",
      "headers": ["列1", "列2", "列3"],
      "rows": [
        ["数据1", "数据2", "数据3"],
        ["数据4", "数据5", "数据6"]
      ]
    }
  ]
}

要求：
- 生成有意义的数据
- 包含表头和数据行
- 数据格式规范`, description, FormatDocumentPromptTarget(target))
}

func BuildXLSXBestSpecPrompt(description string, target XLSXTarget) string {
	return fmt.Sprintf(`请先为以下 Excel 需求生成“工作簿蓝图”。

需求：%s
%s

请只输出 JSON：
{
  "title":"表格标题",
  "goal":"分析目标",
  "audience":"使用对象",
  "analysisDimensions":["维度1","维度2"],
  "sheets":[
    {"name":"Sheet1","columns":["列1","列2"],"summary":"该工作表负责什么分析"}
  ]
}

要求：
- 先规划工作表结构，再补数据
- 工作表命名和列设计要与需求匹配`, description, FormatDocumentPromptTarget(target))
}

func BuildXLSXBestDraftPrompt(description string, target XLSXTarget, spec string) string {
	return fmt.Sprintf(`请根据以下需求和工作簿蓝图，生成最终 Excel JSON。

需求：%s
%s

工作簿蓝图：
%s

请严格按照以下 JSON 输出：
{
  "title": "表格标题",
  "sheets": [
    {
      "name": "Sheet1",
      "headers": ["列1", "列2", "列3"],
      "rows": [
        ["数据1", "数据2", "数据3"],
        ["数据4", "数据5", "数据6"]
      ]
    }
  ]
}

要求：
- 数据要和蓝图中的列定义一致
- 每个 sheet 至少提供 2 行有效数据
- 不要输出 JSON 之外的任何文字`, description, FormatDocumentPromptTarget(target), spec)
}

func FormatDocumentPromptTarget(target PromptTarget) string {
	if IsEmptyPromptTarget(target) {
		return ""
	}
	parts := make([]string, 0, 4)
	if target.DocType != "" {
		parts = append(parts, "文档类型="+target.DocType)
	}
	if target.Language != "" {
		parts = append(parts, "语言="+target.Language)
	}
	if target.Style != "" {
		parts = append(parts, "风格="+target.Style)
	}
	if target.Audience != "" {
		parts = append(parts, "受众="+target.Audience)
	}
	return "补充要求：" + strings.Join(parts, "；")
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
