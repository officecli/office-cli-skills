package generate

import (
	"strings"
	"testing"

	"github.com/officecli/officecli/pkg/ooxmledit"
)

func TestBuildDOCXPrompt_AndBuildDOCXFromJSON(t *testing.T) {
	prompt := BuildDOCXPrompt("Write a project retrospective", PromptTarget{Language: "English", Audience: "Leadership"})
	for _, needle := range []string{"Write a project retrospective", "language=English", "audience=Leadership"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}

	fileBytes, fileName, err := BuildDOCXFromJSON(`{"title":"Project Retrospective","sections":[{"heading":"Background","level":1,"paragraphs":["First paragraph"]}]}`, "fallback")
	if err != nil {
		t.Fatalf("BuildDOCXFromJSON: %v", err)
	}
	if fileName != "Project Retrospective.docx" {
		t.Fatalf("fileName = %q", fileName)
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(fileBytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["word/document.xml"], "Project Retrospective") || !strings.Contains(contentXMLs["word/document.xml"], "First paragraph") {
		t.Fatalf("document xml = %q", contentXMLs["word/document.xml"])
	}
}

func TestBuildXLSXPrompt_AndBuildXLSXFromJSON(t *testing.T) {
	prompt := BuildXLSXPrompt("Generate a sales workbook", PromptTarget{Style: "formal"})
	if !strings.Contains(prompt, "style=formal") {
		t.Fatalf("prompt = %s", prompt)
	}

	fileBytes, fileName, err := BuildXLSXFromJSON(`{"title":"Sales Workbook","sheets":[{"name":"Sheet1","headers":["Region","Amount"],"rows":[["East","100"]]}]}`, "fallback")
	if err != nil {
		t.Fatalf("BuildXLSXFromJSON: %v", err)
	}
	if fileName != "Sales Workbook.xlsx" {
		t.Fatalf("fileName = %q", fileName)
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(fileBytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["xl/sharedStrings.xml"], "East") || !strings.Contains(contentXMLs["xl/sharedStrings.xml"], "100") {
		t.Fatalf("sharedStrings = %q", contentXMLs["xl/sharedStrings.xml"])
	}
}
