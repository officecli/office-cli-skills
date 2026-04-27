package review

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli/pkg/officegen"
)

type stubConverter struct {
	pdfPath string
	err     error
}

func (s stubConverter) Convert(context.Context, string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.pdfPath, nil
}

type stubVisualReviewer struct {
	result *VisualResult
	err    error
}

func (s stubVisualReviewer) ReviewPDF(context.Context, string, StructureReport) (*VisualResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func TestLintPPTX_FlagsCommonQualityIssues(t *testing.T) {
	deck := buildTestDeck(t, []officegen.Slide{
		{Title: "Cover", IsTitle: true, Subtitle: "Demo"},
		{
			Title:    "Problem Slide",
			Layout:   "content",
			HasImage: true,
			ImagePos: "right",
			ImageData: []byte("img"),
			Points: []string{
				"IMG_PLACEHOLDER_1",
				strings.Repeat("This is a very long descriptive sentence. ", 20),
				"Point 3", "Point 4", "Point 5", "Point 6", "Point 7", "Point 8", "Point 9",
			},
		},
	})

	report, err := lintPPTX("demo.pptx", deck)
	if err != nil {
		t.Fatalf("lintPPTX: %v", err)
	}
	codes := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		codes = append(codes, issue.Code)
	}
	for _, code := range []string{"PLACEHOLDER_RESIDUE", "TEXT_DENSITY_HIGH", "BULLET_OVERLOAD"} {
		if !containsCode(codes, code) {
			t.Fatalf("expected issue %s in %v", code, codes)
		}
	}
	if report.Score >= 100 {
		t.Fatalf("expected score drop, got %d", report.Score)
	}
}

func TestLintPPTX_DoesNotFlagBulletOverloadForSectionLayouts(t *testing.T) {
	deck := buildTestDeck(t, []officegen.Slide{
		{Title: "Cover", IsTitle: true, Subtitle: "Demo"},
		{
			Title:    "What It Is",
			Layout:   "content",
			Variant:  "sections-grid",
			Sections: []officegen.SlideSection{{Heading: "High Replayability", Detail: "Long-form grouped copy should stay out of bullet-overload linting."}, {Heading: "Creative Players", Detail: "Card layouts can legitimately contain multiple paragraphs without being bullet slides."}, {Heading: "Simple Start", Detail: "This should remain a section-card page, not a bullet page."}},
		},
		{
			Title:    "How to Start",
			Layout:   "closing",
			Variant:  "closing",
			Sections: []officegen.SlideSection{{Heading: "Pick a Mode", Detail: "Start with a low-pressure first step."}, {Heading: "Try a Small Goal", Detail: "Use one short task to make the topic concrete."}, {Heading: "See if It Fits", Detail: "End with a beginner-friendly takeaway."}},
		},
	})

	report, err := lintPPTX("demo.pptx", deck)
	if err != nil {
		t.Fatalf("lintPPTX: %v", err)
	}
	codes := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		codes = append(codes, issue.Code)
	}
	if containsCode(codes, "BULLET_OVERLOAD") {
		t.Fatalf("did not expect BULLET_OVERLOAD in %v", codes)
	}
}

func TestLintPPTX_FlagsRepeatedSectionGridLayouts(t *testing.T) {
	deck := buildTestDeck(t, []officegen.Slide{
		{Title: "Cover", IsTitle: true, Subtitle: "Demo"},
		{Title: "What It Is", Layout: "content", Variant: "sections-grid", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}, {Heading: "C", Detail: "Three"}}},
		{Title: "Core Ways to Play", Layout: "content", Variant: "sections-grid", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}, {Heading: "C", Detail: "Three"}}},
		{Title: "Who It Suits", Layout: "content", Variant: "sections-grid", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}, {Heading: "C", Detail: "Three"}}},
	})

	report, err := lintPPTX("demo.pptx", deck)
	if err != nil {
		t.Fatalf("lintPPTX: %v", err)
	}
	codes := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		codes = append(codes, issue.Code)
	}
	if !containsCode(codes, "LAYOUT_REPETITION_HIGH") {
		t.Fatalf("expected LAYOUT_REPETITION_HIGH in %v", codes)
	}
}

func TestLintPPTX_FlagsOverusedTwoCardGrid(t *testing.T) {
	deck := buildTestDeck(t, []officegen.Slide{
		{Title: "Cover", IsTitle: true, Subtitle: "Demo"},
		{Title: "What It Is", Layout: "content", Variant: "sections-grid", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}}},
		{Title: "Who It Suits", Layout: "content", Variant: "sections-grid", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}}},
	})

	report, err := lintPPTX("demo.pptx", deck)
	if err != nil {
		t.Fatalf("lintPPTX: %v", err)
	}
	codes := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		codes = append(codes, issue.Code)
	}
	if !containsCode(codes, "TWO_CARD_GRID_OVERUSED") {
		t.Fatalf("expected TWO_CARD_GRID_OVERUSED in %v", codes)
	}
}

func TestLintPPTX_FlagsLowVariantVariety(t *testing.T) {
	deck := buildTestDeck(t, []officegen.Slide{
		{Title: "Cover", IsTitle: true, Subtitle: "Demo"},
		{Title: "One", Layout: "content", Variant: "sections-grid-band", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}, {Heading: "C", Detail: "Three"}}},
		{Title: "Two", Layout: "content", Variant: "sections-grid-band", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}, {Heading: "C", Detail: "Three"}}},
		{Title: "Three", Layout: "timeline", Variant: "timeline-axis", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}, {Heading: "C", Detail: "Three"}}},
		{Title: "Four", Layout: "timeline", Variant: "timeline-axis", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}, {Heading: "C", Detail: "Three"}}},
		{Title: "Five", Layout: "closing", Variant: "closing-checklist", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}}},
	})

	report, err := lintPPTX("demo.pptx", deck)
	if err != nil {
		t.Fatalf("lintPPTX: %v", err)
	}
	codes := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		codes = append(codes, issue.Code)
	}
	if !containsCode(codes, "VARIANT_VARIETY_LOW") {
		t.Fatalf("expected VARIANT_VARIETY_LOW in %v", codes)
	}
}

func TestLintPPTX_FlagsAdjacentRepeatedVariants(t *testing.T) {
	deck := buildTestDeck(t, []officegen.Slide{
		{Title: "Cover", IsTitle: true, Subtitle: "Demo"},
		{Title: "One", Layout: "timeline", Variant: "timeline-zigzag", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}, {Heading: "C", Detail: "Three"}}},
		{Title: "Two", Layout: "timeline", Variant: "timeline-zigzag", Sections: []officegen.SlideSection{{Heading: "A", Detail: "One"}, {Heading: "B", Detail: "Two"}, {Heading: "C", Detail: "Three"}}},
	})

	report, err := lintPPTX("demo.pptx", deck)
	if err != nil {
		t.Fatalf("lintPPTX: %v", err)
	}
	codes := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		codes = append(codes, issue.Code)
	}
	if !containsCode(codes, "VARIANT_REPETITION_ADJACENT") {
		t.Fatalf("expected VARIANT_REPETITION_ADJACENT in %v", codes)
	}
}

func TestServiceReview_UsesVisualResultWhenAvailable(t *testing.T) {
	deckPath := writeDeckFile(t, buildTestDeck(t, []officegen.Slide{
		{Title: "Cover", IsTitle: true, Subtitle: "Demo"},
		{Title: "Overview", Layout: "content", Points: []string{"Key point one", "Key point two", "Key point three"}},
	}))
	pdfPath := filepath.Join(t.TempDir(), "deck.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	service := NewService(
		stubConverter{pdfPath: pdfPath},
		stubVisualReviewer{result: &VisualResult{
			Score:     90,
			Summary:   "Visual communication is clear and the hierarchy is well defined.",
			Strengths: []string{"Page hierarchy is clear"},
			Issues: []Issue{{
				Severity:     "low",
				Code:         "VISUAL_TIGHT_LAYOUT",
				Title:        "Some slides have limited whitespace",
				Message:      "The text area on slide 2 feels slightly crowded.",
				SlideNumbers: []int{2},
				Suggestion:   "Increase margins or split the content across slides.",
			}},
		}},
		nil,
	)

	result, err := service.Review(context.Background(), Request{
		DocumentType: "pptx",
		FilePath:     deckPath,
		EnableVisual: true,
		RuntimeMode:  "external",
		LLMProvider:  "openai",
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if !result.UsedVisual {
		t.Fatal("expected visual review to be used")
	}
	if result.VisualScore != 90 {
		t.Fatalf("visual score = %d", result.VisualScore)
	}
	if result.OverallScore != 93 {
		t.Fatalf("overall score = %d, want 93", result.OverallScore)
	}
	if result.Status == statusPartial {
		t.Fatalf("expected non-partial status, got %s", result.Status)
	}
}

func TestServiceReview_DegradesWhenPDFConversionFails(t *testing.T) {
	deckPath := writeDeckFile(t, buildTestDeck(t, []officegen.Slide{{Title: "Cover", IsTitle: true, Subtitle: "Demo"}}))
	service := NewService(stubConverter{err: fmt.Errorf("LibreOffice (soffice) was not found")}, stubVisualReviewer{}, nil)

	result, err := service.Review(context.Background(), Request{
		DocumentType: "pptx",
		FilePath:     deckPath,
		EnableVisual: true,
		RuntimeMode:  "external",
		LLMProvider:  "openai",
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if result.Status != statusPartial {
		t.Fatalf("status = %s", result.Status)
	}
	if result.UsedVisual {
		t.Fatal("expected visual review to be skipped")
	}
	if len(result.Warnings) == 0 || !strings.Contains(result.Warnings[0], "Visual review was skipped") {
		t.Fatalf("expected degradation warning, got %+v", result.Warnings)
	}
}

func buildTestDeck(t *testing.T, slides []officegen.Slide) []byte {
	t.Helper()
	data, err := officegen.NewPPTXGenerator().Generate(slides, officegen.PPTXOptions{Title: "Test", Creator: "test"})
	if err != nil {
		t.Fatalf("Generate PPTX: %v", err)
	}
	return data
}

func writeDeckFile(t *testing.T, deck []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deck.pptx")
	if err := os.WriteFile(path, deck, 0o644); err != nil {
		t.Fatalf("Write deck: %v", err)
	}
	return path
}

func containsCode(codes []string, target string) bool {
	for _, code := range codes {
		if code == target {
			return true
		}
	}
	return false
}
