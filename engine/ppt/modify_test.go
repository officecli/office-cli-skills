package ppt

import (
	"strings"
	"testing"

	"github.com/officecli/officecli/pkg/officegen"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

func TestSanitizeHexColor(t *testing.T) {
	if got := SanitizeHexColor("#a1b2c3"); got != "A1B2C3" {
		t.Fatalf("got %q", got)
	}
	if got := SanitizeHexColor("xyz"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyModification_ReplaceSlideTitle(t *testing.T) {
	base, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{{Title: "旧标题", Layout: "title", IsTitle: true}}, officegen.PPTXOptions{Title: "deck", Creator: "test"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	modified, err := ApplyModification("replace_slide_title", base, &ModifyOperation{
		SlideIndex: 1,
		Operation:  ModifyOperationPayload{NewTitle: "新标题"},
	})
	if err != nil {
		t.Fatalf("ApplyModification: %v", err)
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(modified, ooxmledit.FileTypePPTX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["ppt/slides/slide1.xml"], "新标题") {
		t.Fatalf("slide xml = %q", contentXMLs["ppt/slides/slide1.xml"])
	}
}
