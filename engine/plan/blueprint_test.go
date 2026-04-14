package plan

import (
	"strings"
	"testing"
)

func TestBuildFrameworkBlueprintMarkdown_PPTXIncludesPagePlanning(t *testing.T) {
	markdown, err := buildFrameworkBlueprintMarkdown("pptx", `{"presentationType":"Project update","targetAudience":"Leadership","presentationPurpose":"Share quarterly results","pageCount":6,"contentStyle":"Conclusion-first","visualEffect":"Clean and credible","contentGuideline":"Keep one conclusion per slide","slideOutline":[{"slideIndex":1,"purpose":"Cover","suggestedLayout":"title","contentFormat":"paragraph","maxItems":1,"contentRequirements":"State the topic","visualSuggestion":"hero"}]}`)
	if err != nil {
		t.Fatalf("buildFrameworkBlueprintMarkdown error: %v", err)
	}
	for _, needle := range []string{"## Framework Blueprint", "### Slide Plan", "Slide 1", "Keep one conclusion per slide"} {
		if !strings.Contains(markdown, needle) {
			t.Fatalf("markdown missing %q: %s", needle, markdown)
		}
	}
}

func TestBuildFrameworkBlueprintMarkdown_DOCXIncludesSectionPlanning(t *testing.T) {
	markdown, err := buildFrameworkBlueprintMarkdown("docx", `{"documentType":"Analytical report","targetAudience":"Leadership","writingGoal":"Explain market changes and recommendations","tone":"Formal and professional","lengthHint":"Around 3000 words","contentGuideline":"Lead with conclusions before analysis","sections":[{"sectionIndex":1,"heading":"Executive summary","purpose":"Present conclusions first","keyPoints":["Conclusion","Recommendation"],"lengthHint":"300 words"}]}`)
	if err != nil {
		t.Fatalf("buildFrameworkBlueprintMarkdown error: %v", err)
	}
	for _, needle := range []string{"## Framework Blueprint", "### Section Plan", "Section 1", "Lead with conclusions before analysis"} {
		if !strings.Contains(markdown, needle) {
			t.Fatalf("markdown missing %q: %s", needle, markdown)
		}
	}
}

func TestBuildFrameworkBlueprintMarkdown_XLSXIncludesWorkbookPlanning(t *testing.T) {
	markdown, err := buildFrameworkBlueprintMarkdown("xlsx", `{"workbookType":"Business analysis","targetAudience":"Leadership","analysisGoal":"Track revenue and budget variance","summaryStyle":"Summary first, details second","contentGuideline":"Keep metric definitions consistent","sheets":[{"sheetIndex":1,"name":"Summary","purpose":"Executive summary","columns":["Month","Revenue","Budget variance"],"notes":"Keep only the core KPIs"}]}`)
	if err != nil {
		t.Fatalf("buildFrameworkBlueprintMarkdown error: %v", err)
	}
	for _, needle := range []string{"## Framework Blueprint", "### Workbook Plan", "Summary", "Keep metric definitions consistent"} {
		if !strings.Contains(markdown, needle) {
			t.Fatalf("markdown missing %q: %s", needle, markdown)
		}
	}
}
