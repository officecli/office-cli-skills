package ppt

import (
	"strings"
	"testing"

	"github.com/officecli/officecli/pkg/officegen"
)

func TestBuildModifyPrompt_IncludesContext(t *testing.T) {
	prompt := BuildModifyPrompt(ModifyPromptInput{
		Intent:      "replace_slide_title",
		Description: "把标题改得更有结论感",
		Target:      TargetMetadata{SlideIndex: 2, ElementType: "title"},
		SlideXML:    "<p:spTree><a:t>原始标题</a:t></p:spTree>",
	})
	for _, needle := range []string{"replace_slide_title", "第2页", "title", "把标题改得更有结论感", "原始标题"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}

func TestParseModifyOperationAndValidate(t *testing.T) {
	op, err := ParseModifyOperation(`{"intent":"replace_slide_title","slideIndex":3,"operation":{"type":"replace_title","newTitle":"新标题"}}`)
	if err != nil {
		t.Fatalf("ParseModifyOperation: %v", err)
	}
	if err := ValidateModifyOperation(op, "replace_slide_title", 3); err != nil {
		t.Fatalf("ValidateModifyOperation: %v", err)
	}
	if op.Operation.NewTitle != "新标题" || op.Operation.Type != "replace_title" {
		t.Fatalf("unexpected op: %+v", op)
	}
}

func TestExtractTargetSlideXML(t *testing.T) {
	base, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{{Title: "第一页", Layout: "title", IsTitle: true}}, officegen.PPTXOptions{Title: "deck", Creator: "test"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	slideXML, err := ExtractTargetSlideXML(base, 1)
	if err != nil {
		t.Fatalf("ExtractTargetSlideXML: %v", err)
	}
	if !strings.Contains(slideXML, "第一页") {
		t.Fatalf("slide xml = %q", slideXML)
	}
}

func TestBuildRewriteSlideFallbackPrompt_UsesSlideSummary(t *testing.T) {
	prompt := BuildRewriteSlideFallbackPrompt(RewriteSlidePromptInput{
		Intent:      "rewrite_entire_slide",
		Description: "把这一页改成英文",
		SlideIndex:  1,
		SlideXML:    `<p:spTree><a:t>动物世界</a:t><a:t>植物世界</a:t></p:spTree>`,
	})
	for _, needle := range []string{"rewrite_entire_slide", "把这一页改成英文", "动物世界", "植物世界"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}
