package ppt

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/officecli/officecli/pkg/ooxmledit"
)

type ModifyPromptInput struct {
	Intent      string
	Description string
	Target      TargetMetadata
	SlideXML    string
}

type RewriteSlidePromptInput struct {
	Intent      string
	Description string
	SlideIndex  int
	SlideXML    string
}

func ExtractTargetSlideXML(ooxmlBytes []byte, slideIndex int) (string, error) {
	contentXMLs, err := ooxmledit.ExtractContentXML(ooxmlBytes, ooxmledit.FileTypePPTX)
	if err != nil {
		return "", fmt.Errorf("extract content: %w", err)
	}

	slidePath := fmt.Sprintf("ppt/slides/slide%d.xml", slideIndex)
	slideXML, ok := contentXMLs[slidePath]
	if !ok {
		return "", fmt.Errorf("slide %d not found", slideIndex)
	}
	return slideXML, nil
}

func BuildModifyPrompt(input ModifyPromptInput) string {
	slidePreview := input.SlideXML
	if len(slidePreview) > 500 {
		slidePreview = slidePreview[:500]
	}
	return fmt.Sprintf(`你是专业的PPT修改助手。请严格按以下JSON格式输出修改指令。

修改意图：%s
目标幻灯片：第%d页
目标元素类型：%s
用户指令：%s

当前目标元素内容：
%s

输出格式（严格遵守）：
{
  "intent": "%s",
  "slideIndex": %d,
  "operation": {
    "type": "replace_title | append_bullets | replace_paragraph | update_table_cell",
    "newTitle": "（仅 replace_slide_title 时填写）",
    "newBullets": ["（仅 append_slide_bullets 时填写）"],
    "newParagraph": "（仅 replace_body_paragraph 时填写）",
    "cellUpdates": [{"row": 1, "col": 1, "value": "（仅 update_table_cells 时填写）"}]
  }
}

约束：
- intent 字段必须与输入的修改意图完全一致
- slideIndex 必须与目标幻灯片一致
- 只填写与 intent 对应的 operation 字段，其余置为空字符串或空数组`,
		input.Intent,
		input.Target.SlideIndex,
		input.Target.ElementType,
		input.Description,
		slidePreview,
		input.Intent,
		input.Target.SlideIndex,
	)
}

func ParseModifyOperation(raw string) (*ModifyOperation, error) {
	var op ModifyOperation
	if err := json.Unmarshal([]byte(raw), &op); err != nil {
		return nil, fmt.Errorf("parse llm response: %w", err)
	}
	return &op, nil
}

func ValidateModifyOperation(op *ModifyOperation, expectedIntent string, expectedSlideIndex int) error {
	if op == nil {
		return fmt.Errorf("modify operation is required")
	}
	if strings.TrimSpace(op.Intent) != strings.TrimSpace(expectedIntent) {
		return fmt.Errorf("llm returned wrong intent: expected %s, got %s", expectedIntent, op.Intent)
	}
	if expectedSlideIndex > 0 && op.SlideIndex != expectedSlideIndex {
		return fmt.Errorf("llm returned wrong slideIndex: expected %d, got %d", expectedSlideIndex, op.SlideIndex)
	}
	return nil
}

func BuildRewriteSlideFallbackPrompt(input RewriteSlidePromptInput) string {
	slideSummary := ooxmledit.ExtractSlideTextSummary(input.SlideXML)
	return fmt.Sprintf(`你是专业的PPT单页改写助手。请只返回 JSON，不要返回 XML。

修改意图：%s
目标幻灯片：第%d页
用户指令：%s
当前目标页文本摘要：%s

输出格式（严格遵守）：
{
  "intent": "%s",
  "slideIndex": %d,
  "operation": {
    "type": "rewrite_slide",
    "layout": "title | content",
    "title": "新的标题",
    "subtitle": "仅 title 布局时填写",
    "points": ["仅 content 布局时填写，2-5 条"],
    "sections": [{"heading": "一级标题", "detail": "二级说明"}],
    "bgColor": "E8F5E9",
    "bgColor2": "C8E6C9"
  }
}

约束：
- 只改这一页
- 保持当前页的主题、语义、事实和主要结构，除非用户明确要求改成全新主题
- 如果用户只是要求改成英文、中文、翻译或其他语言转换，请翻译当前页已有内容，不要替换成新的主题
- 不要凭空新增与当前页无关的行业、公司、报告、数字、章节或结论
- 如果用户要求英文，就把 title / subtitle / points / sections 改成英文
- 如果用户要求绿色背景，请提供绿色系 bgColor / bgColor2
- layout 只能是 title 或 content
- intent 和 slideIndex 必须与输入完全一致`,
		input.Intent,
		input.SlideIndex,
		input.Description,
		slideSummary,
		input.Intent,
		input.SlideIndex,
	)
}
