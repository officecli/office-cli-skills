package generate

import (
	"strings"
	"testing"

	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

func TestBuildDOCXPrompt_AndBuildDOCXFromJSON(t *testing.T) {
	prompt := BuildDOCXPrompt("Write a project retrospective", PromptTarget{Language: "English", Audience: "Leadership"})
	for _, needle := range []string{"Write a project retrospective", "language=English", "audience=Leadership"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}

	fileBytes, fileName, previewHTML, previewJSON, err := BuildDOCXArtifactFromJSON(`{
		"title":"Project Retrospective",
		"subtitle":"Leadership readout for the quarter",
		"theme":{"preset":"executive"},
		"blocks":[
			{"type":"heading","level":1,"text":"Background"},
			{"type":"paragraph","text":"First paragraph"},
			{"type":"callout","title":"Decision","text":"Prioritize the automation workstream."},
			{"type":"table","title":"Action plan","columns":["Workstream","Owner"],"rows":[["Automation","Ops"]]}
		]
	}`, "fallback", "formal", true)
	if err != nil {
		t.Fatalf("BuildDOCXArtifactFromJSON: %v", err)
	}
	if fileName != "Project_Retrospective.docx" {
		t.Fatalf("fileName = %q", fileName)
	}
	if !strings.Contains(string(previewHTML), "Action plan") || !strings.Contains(string(previewJSON), `"type": "callout"`) {
		t.Fatalf("preview missing semantic blocks:\nhtml=%s\njson=%s", string(previewHTML), string(previewJSON))
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(fileBytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["word/document.xml"], "Project Retrospective") ||
		!strings.Contains(contentXMLs["word/document.xml"], "First paragraph") ||
		!strings.Contains(contentXMLs["word/document.xml"], "<w:tbl>") {
		t.Fatalf("document xml = %q", contentXMLs["word/document.xml"])
	}
}

func TestBuildXLSXPrompt_AndBuildXLSXFromJSON(t *testing.T) {
	prompt := BuildXLSXPrompt("Generate a sales workbook", PromptTarget{Style: "formal"})
	if !strings.Contains(prompt, "style=formal") {
		t.Fatalf("prompt = %s", prompt)
	}

	fileBytes, fileName, previewHTML, previewJSON, err := BuildXLSXArtifactFromJSON(`{
		"title":"Sales Workbook",
		"subtitle":"Regional pipeline snapshot",
		"theme":{"preset":"analysis"},
		"sheets":[
			{
				"name":"Sheet1",
				"purpose":"Compare regional commercial performance",
				"summary":[{"label":"Top Region","value":"East"}],
				"columns":[
					{"label":"Region","type":"string"},
					{"label":"Amount","type":"currency"},
					{"label":"YoY","type":"percent"}
				],
				"rows":[["East","100","12%"],["West","120","8%"]],
				"showTotals": true
			}
		]
	}`, "fallback", "analysis", true)
	if err != nil {
		t.Fatalf("BuildXLSXArtifactFromJSON: %v", err)
	}
	if fileName != "Sales_Workbook.xlsx" {
		t.Fatalf("fileName = %q", fileName)
	}
	if !strings.Contains(string(previewHTML), "Top Region") || !strings.Contains(string(previewJSON), `"type": "currency"`) {
		t.Fatalf("preview missing workbook semantics:\nhtml=%s\njson=%s", string(previewHTML), string(previewJSON))
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(fileBytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["xl/sharedStrings.xml"], "East") ||
		!strings.Contains(contentXMLs["xl/worksheets/sheet1.xml"], "<autoFilter") ||
		!strings.Contains(contentXMLs["xl/worksheets/sheet1.xml"], "<f>SUBTOTAL") {
		t.Fatalf("sharedStrings = %q\nsheet1=%q", contentXMLs["xl/sharedStrings.xml"], contentXMLs["xl/worksheets/sheet1.xml"])
	}
}

func TestBuildXLSXArtifactFromJSON_AcceptsScalarCells(t *testing.T) {
	fileBytes, fileName, previewHTML, previewJSON, err := BuildXLSXArtifactFromJSON(`{
		"title":"Minecraft Metrics",
		"subtitle":"Global demand snapshot",
		"sheets":[
			{
				"name":"Sales",
				"summary":[{"label":"Units Sold","value":300000000}],
				"columns":[
					{"label":"Market","type":"string"},
					{"label":"Units Sold","type":"number"},
					{"label":"Featured","type":"bool"}
				],
				"rows":[["Global",300000000,true],["Console",170000000,false]]
			}
		]
	}`, "fallback", "analysis", true)
	if err != nil {
		t.Fatalf("BuildXLSXArtifactFromJSON: %v", err)
	}
	if fileName != "Minecraft_Metrics.xlsx" {
		t.Fatalf("fileName = %q", fileName)
	}
	if !strings.Contains(string(previewHTML), "Units Sold") || !strings.Contains(string(previewJSON), `"type": "bool"`) {
		t.Fatalf("preview missing scalar cell support:\nhtml=%s\njson=%s", string(previewHTML), string(previewJSON))
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(fileBytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["xl/sharedStrings.xml"], "300000000") ||
		!strings.Contains(contentXMLs["xl/worksheets/sheet1.xml"], "<v>300000000</v>") ||
		!strings.Contains(contentXMLs["xl/worksheets/sheet1.xml"], " t=\"b\"") {
		t.Fatalf("sharedStrings = %q\nsheet1=%q", contentXMLs["xl/sharedStrings.xml"], contentXMLs["xl/worksheets/sheet1.xml"])
	}
}

func TestBuildReportPrompt_AndBuildReportFromJSON(t *testing.T) {
	prompt := BuildWorkbookReportPrompt("Create a board-ready report", PromptTarget{Audience: "Board"}, "Sheet 1: Revenue", `{"title":"Draft"}`)
	for _, needle := range []string{"Create a board-ready report", "audience=Board", "Sheet 1: Revenue", `"title":"Draft"`} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}

	fileBytes, fileName, err := BuildReportFromJSON(`{
		"title":"Q2 Business Review",
		"summary":"Commercial momentum stayed positive.",
		"sections":[
			{
				"title":"Demand momentum",
				"narrative":["North America remained the strongest region."],
				"charts":[
					{
						"type":"line",
						"title":"Regional revenue trend",
						"categories":["Jan","Feb","Mar"],
						"series":[{"name":"Revenue","values":[100,110,128]}]
					}
				]
			}
		]
	}`, "fallback")
	if err != nil {
		t.Fatalf("BuildReportFromJSON: %v", err)
	}
	if fileName != "Q2_Business_Review.html" {
		t.Fatalf("fileName = %q", fileName)
	}
	output := string(fileBytes)
	for _, needle := range []string{"Q2 Business Review", "Demand momentum", "echarts.min.js"} {
		if !strings.Contains(output, needle) {
			t.Fatalf("html missing %q: %s", needle, output)
		}
	}
}
