package review

import (
	"testing"

	"github.com/officecli/officecli/pkg/officegen"
)

func TestLintPPTXIgnoresNonVisibleSlideNumberPlaceholderNames(t *testing.T) {
	deck, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{
		{
			Title:   "Visible Title",
			Content: "Visible content that is final and ready.",
			Layout:  "content",
			Points:  []string{"Final point"},
		},
	}, officegen.PPTXOptions{Title: "Placeholder Name Regression"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	report, err := lintPPTX("deck.pptx", deck)
	if err != nil {
		t.Fatalf("lintPPTX: %v", err)
	}
	for _, issue := range report.Issues {
		if issue.Code == "PLACEHOLDER_RESIDUE" {
			t.Fatalf("unexpected placeholder residue issue from non-visible placeholder object name: %#v", issue)
		}
	}
}

func TestHasPlaceholderResidueIgnoresNonVisiblePlaceholderObjectNames(t *testing.T) {
	xmlContent := `<p:cNvPr id="1" name="Slide Number Placeholder 5"></p:cNvPr>`
	if hasPlaceholderResidue(xmlContent, []string{"Final visible copy"}) {
		t.Fatal("non-visible placeholder object names should not count as template placeholder residue")
	}
	if !hasPlaceholderResidue(xmlContent, []string{"Click to add final visible copy"}) {
		t.Fatal("visible placeholder copy should count as template placeholder residue")
	}
	if !hasPlaceholderResidue(`<p:pic><a:t>Final visible copy</a:t><a:descr>IMG_PLACEHOLDER_1</a:descr></p:pic>`, []string{"Final visible copy"}) {
		t.Fatal("image placeholder sentinels should still count as placeholder residue")
	}
}

func TestLintPPTXAllowsReadableThreeCardSlideDensity(t *testing.T) {
	deck, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{
		{
			Title:    "What the reference directory actually teaches",
			Layout:   "content",
			Variant:  "sections-grid",
			Subtitle: "The useful signal is a visual system, not a literal template.",
			Sections: []officegen.SlideSection{
				{Heading: "Repeatable style beats mimicry", Detail: "Use recurring panels, accent rules, large headings, and compact cards rather than copying one output deck."},
				{Heading: "Important content stays editable", Detail: "Slide words, chart labels, metrics, and callouts are shape text or native charts, not baked into images."},
				{Heading: "Visual QA changes design", Detail: "Rendered previews catch overflow, weak contrast, blank pages, and chart defaults before export is accepted."},
			},
		},
	}, officegen.PPTXOptions{Title: "Readable Card Density"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	report, err := lintPPTX("deck.pptx", deck)
	if err != nil {
		t.Fatalf("lintPPTX: %v", err)
	}
	for _, issue := range report.Issues {
		if issue.Code == "TEXT_DENSITY_HIGH" {
			t.Fatalf("readable three-card slide should not be flagged as dense: %#v", issue)
		}
	}
}
