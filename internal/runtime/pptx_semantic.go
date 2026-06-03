package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	generateengine "github.com/officecli/officecli/engine/generate"
	"github.com/officecli/officecli/pkg/officegen"
)

type semanticPPTXDeck struct {
	Title                 string                   `json:"title"`
	Subtitle              string                   `json:"subtitle,omitempty"`
	StylePreset           string                   `json:"stylePreset,omitempty"`
	ReferenceStyleSummary string                   `json:"referenceStyleSummary,omitempty"`
	Theme                 *officegen.DocumentTheme `json:"theme,omitempty"`
	Slides                []semanticPPTXSlide      `json:"slides"`
}

type semanticPPTXSlide struct {
	Role            string              `json:"role,omitempty"`
	Layout          string              `json:"layout,omitempty"`
	Variant         string              `json:"variant,omitempty"`
	Headline        string              `json:"headline"`
	Takeaway        string              `json:"takeaway,omitempty"`
	StyleIntent     string              `json:"styleIntent,omitempty"`
	Density         string              `json:"density,omitempty"`
	VisualTreatment string              `json:"visualTreatment,omitempty"`
	Blocks          []semanticPPTXBlock `json:"blocks,omitempty"`
	Visual          *semanticPPTXVisual `json:"visual,omitempty"`
	Source          string              `json:"source,omitempty"`
	BgColor         string              `json:"bgColor,omitempty"`
	BgColor2        string              `json:"bgColor2,omitempty"`
}

type semanticPPTXBlock struct {
	Type     string                   `json:"type"`
	Text     string                   `json:"text,omitempty"`
	Items    []string                 `json:"items,omitempty"`
	Sections []officegen.SlideSection `json:"sections,omitempty"`
	Metrics  []officegen.MetricCard   `json:"metrics,omitempty"`
	Chart    *officegen.ChartData     `json:"chart,omitempty"`
}

type semanticPPTXVisual struct {
	Kind     string `json:"kind,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
	Position string `json:"position,omitempty"`
}

func parsePPTXPayload(content, fallback, requestedStyle string, enableImages bool) (*pptxPayload, error) {
	content = generatePPTJSON(content)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("parse llm response: %w", err)
	}

	if isSemanticPPTXPayload(raw) {
		var deck semanticPPTXDeck
		if err := json.Unmarshal([]byte(content), &deck); err != nil {
			return nil, fmt.Errorf("parse pptx deck spec: %w", err)
		}
		payload := convertSemanticDeckToPayload(deck, fallback, requestedStyle, enableImages)
		return &payload, nil
	}

	var payload pptxPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return nil, fmt.Errorf("parse llm response: %w", err)
	}
	return &payload, nil
}

func generatePPTJSON(content string) string {
	return generateengine.RepairUnescapedQuotes(generateengine.ExtractJSON(content))
}

func isSemanticPPTXPayload(raw map[string]json.RawMessage) bool {
	var slides []map[string]json.RawMessage
	if data, ok := raw["slides"]; !ok {
		return false
	} else if err := json.Unmarshal(data, &slides); err != nil || len(slides) == 0 {
		return false
	}
	first := slides[0]
	if _, ok := first["headline"]; ok {
		return true
	}
	if _, ok := first["blocks"]; ok {
		return true
	}
	if _, ok := first["visual"]; ok {
		return true
	}
	return false
}

func convertSemanticDeckToPayload(deck semanticPPTXDeck, fallback, requestedStyle string, enableImages bool) pptxPayload {
	stylePreset := firstNonEmpty(strings.TrimSpace(deck.StylePreset), strings.TrimSpace(requestedStyle))
	payload := pptxPayload{
		Title:       strings.TrimSpace(deck.Title),
		StylePreset: stylePreset,
		Slides:      make([]officegen.Slide, 0, len(deck.Slides)),
	}
	for idx, slide := range deck.Slides {
		payload.Slides = append(payload.Slides, convertSemanticSlide(deck, slide, idx, enableImages))
	}
	if payload.Title == "" {
		payload.Title = firstNonEmpty(deck.Title, generateengine.ExtractTitleFromDescription(fallback), "Presentation")
	}
	return payload
}

func convertSemanticSlide(deck semanticPPTXDeck, slide semanticPPTXSlide, idx int, enableImages bool) officegen.Slide {
	role := normalizeNarrativeRole(firstNonEmpty(strings.TrimSpace(slide.Role), "analysis"))
	result := officegen.Slide{
		Role:          role,
		NarrativeRole: role,
		Title:         strings.TrimSpace(slide.Headline),
		Subtitle:      strings.TrimSpace(slide.Takeaway),
		Layout:        strings.ToLower(strings.TrimSpace(slide.Layout)),
		Variant:       strings.ToLower(strings.TrimSpace(slide.Variant)),
		Source:        strings.TrimSpace(slide.Source),
	}
	if role == "" {
		result.Role = strings.ToLower(strings.TrimSpace(slide.Role))
		result.NarrativeRole = ""
	}
	if result.Layout == "" {
		switch role {
		case "cover":
			result.Layout = "title"
		case "toc":
			result.Layout = "toc"
		case "chapter":
			result.Layout = "chapter"
		case "closing", "action":
			result.Layout = "closing"
		default:
			result.Layout = "content"
		}
	}
	if idx == 0 || role == "cover" {
		result.IsTitle = true
		result.Layout = "title"
		result.NarrativeRole = "cover"
		if result.Subtitle == "" {
			result.Subtitle = firstNonEmpty(strings.TrimSpace(deck.Subtitle), strings.TrimSpace(slide.Takeaway))
		}
	}
	for _, block := range slide.Blocks {
		applySemanticBlock(&result, block)
	}
	if enableImages && slide.Visual != nil && strings.EqualFold(strings.TrimSpace(slide.Visual.Kind), "image") {
		prompt := strings.TrimSpace(slide.Visual.Prompt)
		if result.Layout == "gallery" {
			result.Visuals = append(result.Visuals, officegen.SlideVisual{
				Label:   strings.TrimSpace(result.Title),
				Prompt:  prompt,
				Caption: strings.TrimSpace(result.Subtitle),
			})
		} else {
			result.HasImage = true
			result.ImagePrompt = prompt
			result.ImagePos = strings.TrimSpace(slide.Visual.Position)
		}
	}
	applySemanticReferenceStyleIntent(&result, slide, enableImages)
	if result.Layout == "closing" && len(result.Sections) == 0 && len(result.Points) > 0 {
		result = normalizeActionSlide(result)
	}
	return result
}

func applySemanticReferenceStyleIntent(result *officegen.Slide, slide semanticPPTXSlide, enableImages bool) {
	if result == nil {
		return
	}
	intent := strings.ToLower(strings.TrimSpace(slide.StyleIntent + " " + slide.Density))
	if result.Variant == "" && len(result.Sections) > 0 {
		switch {
		case strings.Contains(intent, "card") || strings.Contains(intent, "section") || strings.Contains(intent, "compact") || strings.Contains(intent, "dense"):
			result.Variant = "sections-grid"
		}
	}
	if !enableImages || result.HasImage || len(result.Visuals) > 0 {
		return
	}
	treatment := strings.ToLower(strings.TrimSpace(slide.VisualTreatment))
	if treatment == "" || !strings.Contains(treatment, "image") {
		return
	}
	result.HasImage = true
	result.ImagePos = firstNonEmpty(extractSemanticImagePosition(treatment), "right")
	result.ImagePrompt = buildFallbackImagePrompt(*result, result.Title)
}

func extractSemanticImagePosition(treatment string) string {
	for _, pos := range []string{"right", "left", "background", "center", "top", "bottom", "diagonal"} {
		if strings.Contains(treatment, pos) {
			return pos
		}
	}
	return ""
}

func applySemanticBlock(slide *officegen.Slide, block semanticPPTXBlock) {
	if slide == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(block.Type)) {
	case "narrative", "paragraph", "content":
		text := strings.TrimSpace(block.Text)
		if text == "" {
			return
		}
		if slide.Content == "" {
			slide.Content = text
			return
		}
		slide.Points = append(slide.Points, splitContentToPoints(text, 2)...)
	case "bullets", "actions", "next_steps":
		slide.Points = append(slide.Points, block.Items...)
	case "sections", "comparison", "timeline":
		if slide.Variant == "" && (strings.EqualFold(strings.TrimSpace(block.Type), "comparison") || strings.EqualFold(strings.TrimSpace(block.Type), "timeline")) {
			slide.Variant = strings.ToLower(strings.TrimSpace(block.Type))
		}
		slide.Sections = append(slide.Sections, block.Sections...)
	case "metrics", "kpis":
		slide.Metrics = append(slide.Metrics, block.Metrics...)
		if slide.Layout == "" || slide.Layout == "content" {
			slide.Layout = "dashboard"
		}
	case "chart", "evidence":
		if block.Chart != nil {
			slide.Chart = block.Chart
			if slide.Layout == "" || slide.Layout == "content" {
				slide.Layout = "chart"
			}
		}
		if len(block.Items) > 0 {
			slide.Points = append(slide.Points, block.Items...)
		}
	default:
		if len(block.Items) > 0 {
			slide.Points = append(slide.Points, block.Items...)
			return
		}
		if len(block.Sections) > 0 {
			slide.Sections = append(slide.Sections, block.Sections...)
			return
		}
		if strings.TrimSpace(block.Text) != "" {
			if slide.Content == "" {
				slide.Content = strings.TrimSpace(block.Text)
				return
			}
			slide.Points = append(slide.Points, splitContentToPoints(block.Text, 2)...)
		}
	}
}
