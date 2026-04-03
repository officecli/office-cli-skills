package generate

import (
	"strings"
	"testing"

	"github.com/officecli/officecli/pkg/ooxmledit"
)

func TestBuildDOCXPrompt_AndBuildDOCXFromJSON(t *testing.T) {
	prompt := BuildDOCXPrompt("写一份项目复盘", PromptTarget{Language: "中文", Audience: "管理层"})
	for _, needle := range []string{"写一份项目复盘", "语言=中文", "受众=管理层"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}

	fileBytes, fileName, err := BuildDOCXFromJSON(`{"title":"项目复盘","sections":[{"heading":"背景","level":1,"paragraphs":["第一段"]}]}`, "fallback")
	if err != nil {
		t.Fatalf("BuildDOCXFromJSON: %v", err)
	}
	if fileName != "项目复盘.docx" {
		t.Fatalf("fileName = %q", fileName)
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(fileBytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["word/document.xml"], "项目复盘") || !strings.Contains(contentXMLs["word/document.xml"], "第一段") {
		t.Fatalf("document xml = %q", contentXMLs["word/document.xml"])
	}
}

func TestBuildXLSXPrompt_AndBuildXLSXFromJSON(t *testing.T) {
	prompt := BuildXLSXPrompt("生成销售表", PromptTarget{Style: "正式"})
	if !strings.Contains(prompt, "风格=正式") {
		t.Fatalf("prompt = %s", prompt)
	}

	fileBytes, fileName, err := BuildXLSXFromJSON(`{"title":"销售表","sheets":[{"name":"Sheet1","headers":["区域","金额"],"rows":[["华东","100"]]}]}`, "fallback")
	if err != nil {
		t.Fatalf("BuildXLSXFromJSON: %v", err)
	}
	if fileName != "销售表.xlsx" {
		t.Fatalf("fileName = %q", fileName)
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(fileBytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["xl/sharedStrings.xml"], "华东") || !strings.Contains(contentXMLs["xl/sharedStrings.xml"], "100") {
		t.Fatalf("sharedStrings = %q", contentXMLs["xl/sharedStrings.xml"])
	}
}
