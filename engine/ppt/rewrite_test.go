package ppt

import (
	"strings"
	"testing"
)

func TestParseRewriteSlideBlueprintAndRenderOperation(t *testing.T) {
	blueprint, err := ParseRewriteSlideBlueprint(`{
		"intent":"rewrite_entire_slide",
		"slideIndex":1,
		"operation":{
			"type":"rewrite_slide",
			"layout":"title",
			"title":"Animal World",
			"subtitle":"Plant World",
			"bgColor":"#a1b2c3"
		}
	}`)
	if err != nil {
		t.Fatalf("ParseRewriteSlideBlueprint: %v", err)
	}
	if err := ValidateRewriteSlideBlueprint(blueprint, "rewrite_entire_slide", 1); err != nil {
		t.Fatalf("ValidateRewriteSlideBlueprint: %v", err)
	}
	op, err := RenderRewriteSlideOperation(*blueprint)
	if err != nil {
		t.Fatalf("RenderRewriteSlideOperation: %v", err)
	}
	if op.Operation.Type != "rewrite_slide" || !strings.Contains(op.Operation.NewSlideXML, "Animal World") {
		t.Fatalf("unexpected op: %+v", op)
	}
}

func TestRewriteOutputContainsOOXMLJargon(t *testing.T) {
	if !RewriteOutputContainsOOXMLJargon(`<a:t>geometry</a:t><a:t>Normal content</a:t>`) {
		t.Fatal("expected jargon detection")
	}
	if RewriteOutputContainsOOXMLJargon(`<a:t>Normal content</a:t>`) {
		t.Fatal("expected non-jargon content")
	}
}
