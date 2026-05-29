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
	return fmt.Sprintf(`You are a professional PPT editing assistant. Return the modification instruction in JSON only.

Intent: %s
Target slide: %d
Target element type: %s
User instruction: %s

Current target element content:
%s

Output format:
{
  "intent": "%s",
  "slideIndex": %d,
  "operation": {
    "type": "replace_title | append_bullets | replace_paragraph | update_table_cell",
    "newTitle": "(only for replace_slide_title)",
    "newBullets": ["(only for append_slide_bullets)"],
    "newParagraph": "(only for replace_body_paragraph)",
    "cellUpdates": [{"row": 1, "col": 1, "value": "(only for update_table_cells)"}]
  }
}

Constraints:
- The intent field must exactly match the input intent
- slideIndex must match the target slide
- Fill only the operation fields relevant to the intent and leave the others empty`,
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
	return fmt.Sprintf(`You are a professional single-slide PPT rewrite assistant. Return JSON only, not XML.

Intent: %s
Target slide: %d
User instruction: %s
Current slide text summary: %s

Output format:
{
  "intent": "%s",
  "slideIndex": %d,
  "operation": {
    "type": "rewrite_slide",
    "layout": "title | content",
    "title": "New title",
    "subtitle": "Only for title layout",
    "points": ["Only for content layout, 2-5 items"],
    "sections": [{"heading": "Primary heading", "detail": "Secondary detail"}],
    "bgColor": "E8F5E9",
    "bgColor2": "C8E6C9"
  }
}

Constraints:
- Rewrite only this slide
- Preserve the current slide's topic, semantics, facts, and main structure unless the user explicitly asks for a new topic
- If the user asks for English, Chinese, translation, or another language conversion, translate the current content instead of inventing a new topic
- Do not fabricate unrelated industries, companies, reports, numbers, sections, or conclusions
- If the user asks for English, title / subtitle / points / sections must be in English
- If the user asks for a green background, provide green-toned bgColor / bgColor2 values
- layout must be either title or content
- intent and slideIndex must exactly match the input`,
		input.Intent,
		input.SlideIndex,
		input.Description,
		slideSummary,
		input.Intent,
		input.SlideIndex,
	)
}
