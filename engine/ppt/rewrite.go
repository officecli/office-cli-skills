package ppt

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/officecli/officecli-internal/pkg/officegen"
	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

type SlideSection struct {
	Heading string `json:"heading"`
	Detail  string `json:"detail"`
}

type RewriteSlideBlueprintOperation struct {
	Type     string         `json:"type"`
	Layout   string         `json:"layout"`
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle"`
	Points   []string       `json:"points"`
	Sections []SlideSection `json:"sections"`
	BgColor  string         `json:"bgColor"`
	BgColor2 string         `json:"bgColor2"`
}

type RewriteSlideBlueprint struct {
	Intent     string                         `json:"intent"`
	SlideIndex int                            `json:"slideIndex"`
	Operation  RewriteSlideBlueprintOperation `json:"operation"`
}

func ParseRewriteSlideBlueprint(raw string) (*RewriteSlideBlueprint, error) {
	var blueprint RewriteSlideBlueprint
	if err := json.Unmarshal([]byte(raw), &blueprint); err != nil {
		return nil, fmt.Errorf("parse rewrite fallback response: %w", err)
	}
	return &blueprint, nil
}

func ValidateRewriteSlideBlueprint(blueprint *RewriteSlideBlueprint, expectedIntent string, expectedSlideIndex int) error {
	if blueprint == nil {
		return fmt.Errorf("rewrite slide blueprint is required")
	}
	if strings.TrimSpace(blueprint.Intent) != strings.TrimSpace(expectedIntent) {
		return fmt.Errorf("llm returned wrong intent: expected %s, got %s", expectedIntent, blueprint.Intent)
	}
	if expectedSlideIndex > 0 && blueprint.SlideIndex != expectedSlideIndex {
		return fmt.Errorf("llm returned wrong slideIndex: expected %d, got %d", expectedSlideIndex, blueprint.SlideIndex)
	}
	return nil
}

func RenderRewriteSlideXML(blueprint RewriteSlideBlueprint) (string, error) {
	layout := blueprint.Operation.Layout
	if layout == "" {
		if len(blueprint.Operation.Sections) > 0 || len(blueprint.Operation.Points) > 0 {
			layout = "content"
		} else {
			layout = "title"
		}
	}

	sections := make([]officegen.SlideSection, 0, len(blueprint.Operation.Sections))
	for _, section := range blueprint.Operation.Sections {
		sections = append(sections, officegen.SlideSection{Heading: section.Heading, Detail: section.Detail})
	}

	slide := officegen.Slide{
		Title:    blueprint.Operation.Title,
		Subtitle: blueprint.Operation.Subtitle,
		Points:   append([]string(nil), blueprint.Operation.Points...),
		Sections: sections,
		Layout:   layout,
		IsTitle:  layout == "title",
		BgColor:  SanitizeHexColor(blueprint.Operation.BgColor),
		BgColor2: SanitizeHexColor(blueprint.Operation.BgColor2),
	}
	if slide.Title == "" {
		slide.Title = "Updated Slide"
	}

	fileBytes, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{slide}, officegen.PPTXOptions{
		Title:   slide.Title,
		Creator: "OfficeCLI",
	})
	if err != nil {
		return "", err
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(fileBytes, ooxmledit.FileTypePPTX)
	if err != nil {
		return "", err
	}

	newSlideXML, ok := contentXMLs["ppt/slides/slide1.xml"]
	if !ok {
		return "", fmt.Errorf("generated slide xml not found")
	}
	return newSlideXML, nil
}

func RenderRewriteSlideOperation(blueprint RewriteSlideBlueprint) (*ModifyOperation, error) {
	newSlideXML, err := RenderRewriteSlideXML(blueprint)
	if err != nil {
		return nil, fmt.Errorf("render rewrite slide xml: %w", err)
	}
	return &ModifyOperation{
		Intent:     blueprint.Intent,
		SlideIndex: blueprint.SlideIndex,
		Operation: ModifyOperationPayload{
			Type:        "rewrite_slide",
			NewSlideXML: newSlideXML,
		},
	}, nil
}

func RewriteOutputContainsOOXMLJargon(slideXML string) bool {
	if strings.TrimSpace(slideXML) == "" {
		return false
	}
	jargonTerms := map[string]struct{}{
		"geometry":   {},
		"character":  {},
		"characters": {},
		"prstgeom":   {},
		"srgbclr":    {},
		"solidfill":  {},
		"avlst":      {},
		"xfrm":       {},
	}
	for _, raw := range ooxmledit.ExtractSlideTextRuns(slideXML) {
		text := strings.ToLower(strings.TrimSpace(raw))
		if text == "" {
			continue
		}
		if _, ok := jargonTerms[text]; ok {
			return true
		}
	}
	return false
}
