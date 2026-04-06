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
		{Title: "封面", IsTitle: true, Subtitle: "演示"},
		{
			Title:  "问题页",
			Layout: "content",
			Points: []string{
				"IMG_PLACEHOLDER_1",
				strings.Repeat("这是一段很长的说明文字", 20),
				"要点 3", "要点 4", "要点 5", "要点 6", "要点 7", "要点 8", "要点 9",
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

func TestServiceReview_UsesVisualResultWhenAvailable(t *testing.T) {
	deckPath := writeDeckFile(t, buildTestDeck(t, []officegen.Slide{
		{Title: "封面", IsTitle: true, Subtitle: "演示"},
		{Title: "概览", Layout: "content", Points: []string{"重点一", "重点二", "重点三"}},
	}))
	pdfPath := filepath.Join(t.TempDir(), "deck.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	service := NewService(
		stubConverter{pdfPath: pdfPath},
		stubVisualReviewer{result: &VisualResult{
			Score:     90,
			Summary:   "视觉表达清晰，层级明确。",
			Strengths: []string{"页面层级清楚"},
			Issues: []Issue{{
				Severity:     "low",
				Code:         "VISUAL_TIGHT_LAYOUT",
				Title:        "个别页面留白偏少",
				Message:      "第 2 页文字区域略拥挤。",
				SlideNumbers: []int{2},
				Suggestion:   "增加边距或拆页。",
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
	deckPath := writeDeckFile(t, buildTestDeck(t, []officegen.Slide{{Title: "封面", IsTitle: true, Subtitle: "演示"}}))
	service := NewService(stubConverter{err: fmt.Errorf("未找到 LibreOffice（soffice）")}, stubVisualReviewer{}, nil)

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
	if len(result.Warnings) == 0 || !strings.Contains(result.Warnings[0], "视觉评审已跳过") {
		t.Fatalf("expected degradation warning, got %+v", result.Warnings)
	}
}

func buildTestDeck(t *testing.T, slides []officegen.Slide) []byte {
	t.Helper()
	data, err := officegen.NewPPTXGenerator().Generate(slides, officegen.PPTXOptions{Title: "测试", Creator: "test"})
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
