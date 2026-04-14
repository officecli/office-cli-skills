package ppt

import (
	"strings"
	"testing"

	"github.com/officecli/officecli/pkg/officegen"
)

func TestBuildModifyPrompt_IncludesContext(t *testing.T) {
	prompt := BuildModifyPrompt(ModifyPromptInput{
		Intent:      "replace_slide_title",
		Description: "Make the title more conclusion-driven",
		Target:      TargetMetadata{SlideIndex: 2, ElementType: "title"},
		SlideXML:    "<p:spTree><a:t>Original title</a:t></p:spTree>",
	})
	for _, needle := range []string{"replace_slide_title", "Target slide: 2", "title", "Make the title more conclusion-driven", "Original title"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}

func TestParseModifyOperationAndValidate(t *testing.T) {
	op, err := ParseModifyOperation(`{"intent":"replace_slide_title","slideIndex":3,"operation":{"type":"replace_title","newTitle":"Updated title"}}`)
	if err != nil {
		t.Fatalf("ParseModifyOperation: %v", err)
	}
	if err := ValidateModifyOperation(op, "replace_slide_title", 3); err != nil {
		t.Fatalf("ValidateModifyOperation: %v", err)
	}
	if op.Operation.NewTitle != "Updated title" || op.Operation.Type != "replace_title" {
		t.Fatalf("unexpected op: %+v", op)
	}
}

func TestExtractTargetSlideXML(t *testing.T) {
	base, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{{Title: "First slide", Layout: "title", IsTitle: true}}, officegen.PPTXOptions{Title: "deck", Creator: "test"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	slideXML, err := ExtractTargetSlideXML(base, 1)
	if err != nil {
		t.Fatalf("ExtractTargetSlideXML: %v", err)
	}
	if !strings.Contains(slideXML, "First slide") {
		t.Fatalf("slide xml = %q", slideXML)
	}
}

func TestBuildRewriteSlideFallbackPrompt_UsesSlideSummary(t *testing.T) {
	prompt := BuildRewriteSlideFallbackPrompt(RewriteSlidePromptInput{
		Intent:      "rewrite_entire_slide",
		Description: "Rewrite this slide in English",
		SlideIndex:  1,
		SlideXML:    `<p:spTree><a:t>Animal World</a:t><a:t>Plant World</a:t></p:spTree>`,
	})
	for _, needle := range []string{"rewrite_entire_slide", "Rewrite this slide in English", "Animal World", "Plant World"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}
