package edit

import (
	"strings"
	"testing"
)

func TestBuildPPTXEditPromptContainsAllOpTypes(t *testing.T) {
	prompt := BuildPPTXEditPrompt(PPTXEditPromptInput{
		Prompt: "Change title",
		SlidePreviews: []SlidePreview{
			{SlideIndex: 1, TextPreview: "Hello World"},
		},
	})
	for _, op := range pptxOpTypes {
		if !strings.Contains(prompt, op) {
			t.Errorf("PPTX prompt missing op_type %q", op)
		}
	}
}

func TestBuildDOCXEditPromptContainsAllOpTypes(t *testing.T) {
	prompt := BuildDOCXEditPrompt(DOCXEditPromptInput{
		Prompt: "Replace paragraph",
		ParagraphPreviews: []ParagraphPreview{
			{ParagraphIndex: 1, Text: "First paragraph"},
		},
	})
	for _, op := range docxOpTypes {
		if !strings.Contains(prompt, op) {
			t.Errorf("DOCX prompt missing op_type %q", op)
		}
	}
}

func TestBuildXLSXEditPromptContainsAllOpTypes(t *testing.T) {
	prompt := BuildXLSXEditPrompt(XLSXEditPromptInput{
		Prompt: "Update cells",
		SheetPreviews: []SheetPreview{
			{SheetName: "Sheet1", Rows: [][]string{{"A", "B"}}},
		},
	})
	for _, op := range xlsxOpTypes {
		if !strings.Contains(prompt, op) {
			t.Errorf("XLSX prompt missing op_type %q", op)
		}
	}
}

func TestEditPromptOpNamesMatchDecodeOp(t *testing.T) {
	prompts := map[string]string{
		"pptx": BuildPPTXEditPrompt(PPTXEditPromptInput{
			Prompt:        "test",
			SlidePreviews: []SlidePreview{{SlideIndex: 1, TextPreview: "test"}},
		}),
		"docx": BuildDOCXEditPrompt(DOCXEditPromptInput{
			Prompt:            "test",
			ParagraphPreviews: []ParagraphPreview{{ParagraphIndex: 1, Text: "test"}},
		}),
		"xlsx": BuildXLSXEditPrompt(XLSXEditPromptInput{
			Prompt:        "test",
			SheetPreviews: []SheetPreview{{SheetName: "Sheet1", Rows: [][]string{{"a"}}}},
		}),
	}

	allOps := map[string][]string{
		"pptx": pptxOpTypes,
		"docx": docxOpTypes,
		"xlsx": xlsxOpTypes,
	}

	for format, ops := range allOps {
		prompt := prompts[format]
		for _, opName := range ops {
			if !strings.Contains(prompt, opName) {
				t.Errorf("prompt for %s does not contain op_type %q", format, opName)
				continue
			}
			raw := buildTestOp(format, opName)
			_, err := DecodeOp(fileTypeFromString(format), raw)
			if err != nil {
				t.Errorf("DecodeOp(%s, %s) failed: %v", format, opName, err)
			}
		}
	}
}

func TestBuildPPTXEditPromptIncludesLanguageAndStyle(t *testing.T) {
	prompt := BuildPPTXEditPrompt(PPTXEditPromptInput{
		Prompt:        "Change title",
		SlidePreviews: []SlidePreview{{SlideIndex: 1, TextPreview: "Hello"}},
		Language:      "Chinese",
		Style:         "formal",
	})
	if !strings.Contains(prompt, "Language: Chinese") {
		t.Error("prompt missing Language field")
	}
	if !strings.Contains(prompt, "Style: formal") {
		t.Error("prompt missing Style field")
	}
}
