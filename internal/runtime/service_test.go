package runtime

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	"github.com/officecli/officecli/pkg/officegen"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

type fakeLLMClient struct {
	textResponse        string
	jsonResponse        string
	jsonResponses       []string
	jsonCallCount       int
	structuredResponse  string
	structuredCallCount int
	lastStructuredReq   engine.StructuredCompletionRequest
	imageResult         *engine.ImageGenerationResult
	imageErr            error
	imageCalls          int
	lastImageRequest    engine.ImageGenerationRequest
}

func (f fakeLLMClient) CompleteText(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return f.textResponse, nil
}

func (f *fakeLLMClient) CompleteJSON(_ context.Context, _ []engine.LLMMessage) (string, error) {
	if len(f.jsonResponses) > 0 {
		idx := f.jsonCallCount
		if idx >= len(f.jsonResponses) {
			idx = len(f.jsonResponses) - 1
		}
		f.jsonCallCount++
		return f.jsonResponses[idx], nil
	}
	return f.jsonResponse, nil
}

func (f *fakeLLMClient) CompleteStructured(_ context.Context, req engine.StructuredCompletionRequest) (string, error) {
	f.structuredCallCount++
	f.lastStructuredReq = req
	return f.structuredResponse, nil
}

func (f *fakeLLMClient) GenerateImage(_ context.Context, req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	f.imageCalls++
	f.lastImageRequest = req
	if f.imageErr != nil {
		return nil, f.imageErr
	}
	return f.imageResult, nil
}

type runtimeProgressCollector struct {
	events []engine.ProgressEvent
}

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+yF9sAAAAASUVORK5CYII="

func mustTinyPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return data
}

func readZipEntry(t *testing.T, archive []byte, name string) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", name, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read zip entry %s: %v", name, err)
		}
		return string(data)
	}
	t.Fatalf("zip entry not found: %s", name)
	return ""
}

func countZipEntries(archive []byte, prefix, suffix string) int {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return 0
	}
	count := 0
	for _, file := range reader.File {
		if prefix != "" && !strings.HasPrefix(file.Name, prefix) {
			continue
		}
		if suffix != "" && !strings.HasSuffix(file.Name, suffix) {
			continue
		}
		count++
	}
	return count
}

func archiveContainsEntryWithSubstring(t *testing.T, archive []byte, prefix, suffix, needle string) bool {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, file := range reader.File {
		if prefix != "" && !strings.HasPrefix(file.Name, prefix) {
			continue
		}
		if suffix != "" && !strings.HasSuffix(file.Name, suffix) {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}
		if strings.Contains(string(data), needle) {
			return true
		}
	}
	return false
}

func (c *runtimeProgressCollector) Emit(_ context.Context, event engine.ProgressEvent) {
	c.events = append(c.events, event)
}

func TestServiceGenerateDOCXWithFakeLLM(t *testing.T) {
	service := NewService(&fakeLLMClient{
		jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeDOCX,
		Prompt:       "Introduce this enterprise collaboration platform",
		Topic:        "Enterprise Collaboration Platform Overview",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["word/document.xml"], "Enterprise Collaboration Platform") {
		t.Fatalf("document xml = %q", contentXMLs["word/document.xml"])
	}
}

func TestServiceGenerateXLSXWithFakeLLM(t *testing.T) {
	service := NewService(&fakeLLMClient{
		jsonResponse: `{"title":"Sales Workbook","sheets":[{"name":"Pipeline","headers":["Region","Amount"],"rows":[["East","100"],["West","120"]]}]}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeXLSX,
		Prompt:       "Create a regional sales workbook",
		Topic:        "Sales Workbook",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if doc.DocumentName != "Sales_Workbook.xlsx" {
		t.Fatalf("document name = %q", doc.DocumentName)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["xl/workbook.xml"], "Pipeline") ||
		!strings.Contains(contentXMLs["xl/sharedStrings.xml"], "East") ||
		!strings.Contains(contentXMLs["xl/worksheets/sheet1.xml"], "120") {
		t.Fatalf("workbook xml = %q\nshared strings = %q\nsheet1 = %q", contentXMLs["xl/workbook.xml"], contentXMLs["xl/sharedStrings.xml"], contentXMLs["xl/worksheets/sheet1.xml"])
	}
}

func TestServiceGeneratePPTXWithFakeLLM(t *testing.T) {
	service := NewService(&fakeLLMClient{
		jsonResponse: `{
			"title":"Enterprise Collaboration Platform Overview",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"Enterprise Collaboration Platform Overview","layout":"title","subtitle":"Product context and business status","isTitle":true},
				{"title":"Product Capabilities","layout":"content","points":["Multi-user collaboration","Real-time editing","Enterprise administration"]}
			]
		}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		Topic:        "Enterprise Collaboration Platform Overview",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypePPTX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["ppt/slides/slide1.xml"], "Enterprise Collabo") {
		t.Fatalf("slide xml = %q", contentXMLs["ppt/slides/slide1.xml"])
	}
}

func TestServiceGenerateReportWithFakeLLM(t *testing.T) {
	workbookBytes, err := officegen.NewXLSXGenerator().Generate([]officegen.XlsxSheet{
		{
			Name: "Revenue",
			Rows: [][]string{
				{"Region", "Revenue"},
				{"North America", "128"},
				{"Europe", "96"},
				{"APAC", "74"},
			},
		},
	}, officegen.XLSXOptions{Title: "Q2 Business Review", Creator: "test"})
	if err != nil {
		t.Fatalf("Generate workbook: %v", err)
	}
	workbookPath := filepath.Join(t.TempDir(), "q2_metrics.xlsx")
	if err := os.WriteFile(workbookPath, workbookBytes, 0o644); err != nil {
		t.Fatalf("Write workbook: %v", err)
	}

	service := NewService(&fakeLLMClient{
		jsonResponse: `{
			"title":"Q2 Business Review",
			"subtitle":"Commercial momentum and decision points",
			"audience":"Board and investors",
			"summary":"Growth continued, but conversion efficiency softened in the final mile.",
			"kpis":[{"label":"Revenue","value":"$12.4M","change":"+8% QoQ"}],
			"findings":["North America remained ahead of plan."],
			"sections":[
				{
					"title":"Demand momentum",
					"subtitle":"Headline view of regional performance",
					"narrative":["North America led the quarter while Europe stayed stable."],
					"charts":[
						{
							"type":"bar",
							"title":"Regional revenue",
							"categories":["North America","Europe","APAC"],
							"series":[{"name":"Revenue","values":[128,96,74]}],
							"source":"Internal finance data"
						}
					]
				}
			],
			"appendixTables":[
				{
					"title":"Supporting table",
					"headers":["Region","Revenue"],
					"rows":[["North America","128"],["Europe","96"]]
				}
			]
		}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType:   engine.DocumentTypeReport,
		Prompt:         "Create a board-ready report for the latest business review.",
		Topic:          "Q2 Business Review",
		SourceFilePath: workbookPath,
		Mode:           "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	output := string(doc.Bytes)
	for _, needle := range []string{
		"<html lang=",
		"Q2 Business Review",
		"Demand momentum",
		"echarts.min.js",
		"Regional revenue",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("report html missing %q:\n%s", needle, output)
		}
	}
}

func TestBuildPPTXPrompt_ImagesEnabledIncludesImageGuidance(t *testing.T) {
	prompt := BuildPPTXPrompt("Introduce product capabilities", generateengine.PromptTarget{}, true)
	for _, needle := range []string{
		`"visuals": [`,
		`"prompt": "A concrete visual prompt that can be sent directly to an image model"`,
		"Use images sparingly. Prefer at most one hero image slide plus at most one gallery slide",
		"Do not add images to chart, dashboard, toc, or closing layouts",
		"On gallery slides, use visuals with 2-4 concrete prompts",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q:\n%s", needle, prompt)
		}
	}
}

func TestBuildPPTXPrompt_ImagesDisabledForbidsImageFields(t *testing.T) {
	prompt := BuildPPTXPrompt("Introduce product capabilities", generateengine.PromptTarget{}, false)
	if strings.Contains(prompt, `"hasImage": true`) {
		t.Fatalf("prompt should not include image schema when disabled:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Do not output the image fields hasImage, imagePrompt, or imagePos.") {
		t.Fatalf("prompt should forbid image fields when disabled:\n%s", prompt)
	}
}

func TestBuildPPTXPrompt_IncludesQualityConstraints(t *testing.T) {
	prompt := BuildPPTXPrompt("Introduce product capabilities", generateengine.PromptTarget{
		Language: "en-US",
		Style:    "Professional and restrained",
		Audience: "Prospective enterprise customers",
	}, true)
	for _, needle := range []string{
		"Keep the deck to 6-10 slides, usually 7-9.",
		"slide 2 should usually be a toc page",
		"subtitle must be a takeaway sentence",
		"Use at most 3 sections, at most 4 dashboard metrics",
		"Do not use charts for priorities, milestones, strategy, risks, or process flows",
		"The closing slide must include 2-3 next-step actions",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q:\n%s", needle, prompt)
		}
	}
}

func TestBuildPPTXPrompt_UsesArchetypeRules(t *testing.T) {
	companyPrompt := BuildPPTXPrompt("enterprise collaboration platform", generateengine.PromptTarget{}, true)
	if !strings.Contains(companyPrompt, "a strong storyline is usually cover -> toc -> chapter -> key takeaways -> core capabilities -> customer value -> use cases -> chapter -> rollout path") {
		t.Fatalf("company prompt missing archetype outline:\n%s", companyPrompt)
	}
	marketPrompt := BuildPPTXPrompt("market opportunity analysis", generateengine.PromptTarget{}, false)
	if !strings.Contains(marketPrompt, "Slide 3 must use a chart and include a source.") {
		t.Fatalf("market prompt missing archetype outline:\n%s", marketPrompt)
	}
	opsPrompt := BuildPPTXPrompt("business review", generateengine.PromptTarget{}, false)
	if !strings.Contains(opsPrompt, "a strong storyline is usually cover -> toc -> chapter -> business takeaways -> core metrics -> issue diagnosis -> next-quarter priorities -> chapter -> execution actions") {
		t.Fatalf("ops prompt missing archetype outline:\n%s", opsPrompt)
	}
	trainingPrompt := BuildPPTXPrompt("new hire onboarding training", generateengine.PromptTarget{}, false)
	if !strings.Contains(trainingPrompt, "a strong storyline is usually cover -> toc -> chapter -> learning goals -> installation and setup -> common commands -> example workflow -> chapter -> cautions") {
		t.Fatalf("training prompt missing archetype outline:\n%s", trainingPrompt)
	}
}

func TestCleanSentence_PreservesTimeNumbers(t *testing.T) {
	if got := cleanSentence("Complete the first validation cycle within 30 days."); got != "Complete the first validation cycle within 30 days" {
		t.Fatalf("cleanSentence() = %q", got)
	}
	if got := cleanSentence("1. Clarify the goal"); got != "Clarify the goal" {
		t.Fatalf("cleanSentence() = %q", got)
	}
}

func TestFitTextForLayout_PrefersWholeClause(t *testing.T) {
	got := fitTextForLayout("Validate PMF in Southeast Asia first, then expand into Europe for higher-value deals", 18)
	if strings.Contains(got, "...") {
		t.Fatalf("fitTextForLayout() should avoid ellipsis: %q", got)
	}
	if len(got) == 0 {
		t.Fatal("fitTextForLayout() should return non-empty text")
	}
}

func TestNormalizeActionSlide_ConvertsPointsToSections(t *testing.T) {
	slide := normalizeActionSlide(officegen.Slide{
		Title: "execution actions",
		Points: []string{
			"30 days product lead confirms the main scenario and finishes target-customer interviews",
			"60 days channel lead signs partners and validates lead cost",
		},
	})
	if len(slide.Sections) != 2 {
		t.Fatalf("sections = %#v", slide.Sections)
	}
	if slide.Sections[0].Heading != "Step 1" {
		t.Fatalf("first heading = %q", slide.Sections[0].Heading)
	}
	if len(slide.Points) != 0 {
		t.Fatalf("points should be cleared after section normalization: %#v", slide.Points)
	}
}

func TestNormalizeEvidenceSlide_PromotesValueAndMarketSlides(t *testing.T) {
	valueSlide := normalizeEvidenceSlide(officegen.Slide{Title: "customer value"})
	if valueSlide.Layout != "dashboard" || len(valueSlide.Metrics) == 0 {
		t.Fatalf("value slide = %#v", valueSlide)
	}
	marketSlide := normalizeEvidenceSlide(officegen.Slide{Title: "market size"})
	if marketSlide.Layout != "chart" || marketSlide.Chart == nil {
		t.Fatalf("market slide = %#v", marketSlide)
	}
	if marketSlide.Source == "" {
		t.Fatalf("market slide should inject source hint")
	}
}

func TestNormalizePPTXPayload_EnforcesCompanySkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "enterprise collaboration platform",
		Slides: []officegen.Slide{
			{Title: "enterprise collaboration platform", Layout: "title", IsTitle: true},
			{Title: "first slide"},
			{Title: "second slide"},
		},
	}
	normalizePPTXPayload(payload, "enterprise collaboration platform", "", true)
	if len(payload.Slides) < 7 {
		t.Fatalf("slide count = %d, want scaffolded deck", len(payload.Slides))
	}
	if payload.Slides[1].Layout != "toc" {
		t.Fatalf("company toc slide = %#v", payload.Slides[1])
	}
	if payload.Slides[2].Layout != "chapter" {
		t.Fatalf("company first chapter slide = %#v", payload.Slides[2])
	}
	if payload.Slides[len(payload.Slides)-1].Layout != "closing" || len(payload.Slides[len(payload.Slides)-1].Sections) == 0 {
		t.Fatalf("company closing slide = %#v", payload.Slides[len(payload.Slides)-1])
	}
}

func TestNormalizePPTXPayload_EnforcesMarketSkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "market opportunity analysis",
		Slides: []officegen.Slide{
			{Title: "market opportunity analysis", Layout: "title", IsTitle: true},
			{Title: "first slide"},
			{Title: "second slide"},
		},
	}
	normalizePPTXPayload(payload, "market opportunity analysis", "", true)
	if payload.Slides[1].Layout != "toc" {
		t.Fatalf("market toc slide = %#v", payload.Slides[1])
	}
	foundChart := false
	for _, slide := range payload.Slides {
		if slide.Layout == "chart" && slide.Chart != nil {
			foundChart = true
			break
		}
	}
	if !foundChart {
		t.Fatalf("market deck should retain an evidence chart: %#v", payload.Slides)
	}
	if payload.Slides[len(payload.Slides)-1].Layout != "closing" || len(payload.Slides[len(payload.Slides)-1].Sections) == 0 {
		t.Fatalf("market closing slide = %#v", payload.Slides[len(payload.Slides)-1])
	}
}

func TestNormalizePPTXPayload_EnforcesOpsSkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "business review",
		Slides: []officegen.Slide{
			{Title: "business review", Layout: "title", IsTitle: true},
			{Title: "first slide"},
			{Title: "second slide"},
		},
	}
	normalizePPTXPayload(payload, "business review", "", true)
	if payload.Slides[1].Layout != "toc" {
		t.Fatalf("ops toc slide = %#v", payload.Slides[1])
	}
	foundChart := false
	for _, slide := range payload.Slides {
		if slide.Layout == "chart" && slide.Chart != nil {
			foundChart = true
			break
		}
	}
	if !foundChart {
		t.Fatalf("ops deck should retain an evidence chart: %#v", payload.Slides)
	}
	if payload.Slides[len(payload.Slides)-1].Layout != "closing" || len(payload.Slides[len(payload.Slides)-1].Sections) != 3 {
		t.Fatalf("ops closing slide = %#v", payload.Slides[len(payload.Slides)-1])
	}
}

func TestNormalizePPTXPayload_EnforcesTrainingSkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "new hire onboarding training",
		Slides: []officegen.Slide{
			{Title: "new hire onboarding training", Layout: "title", IsTitle: true},
			{Title: "first slide"},
			{Title: "second slide"},
		},
	}
	normalizePPTXPayload(payload, "new hire onboarding training", "", true)
	if payload.Slides[1].Layout != "toc" {
		t.Fatalf("training toc slide = %#v", payload.Slides[1])
	}
	if payload.Slides[len(payload.Slides)-1].Layout != "closing" || len(payload.Slides[len(payload.Slides)-1].Sections) != 3 {
		t.Fatalf("training closing slide = %#v", payload.Slides[len(payload.Slides)-1])
	}
}

func TestServiceGeneratePPTX_GeneratesImagesWhenEnabled(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"product capability overview",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"cover","layout":"title","subtitle":"product context and business status","isTitle":true},
				{"title":"product capabilities","layout":"content","points":["Multi-user collaboration","Real-time editing","Enterprise administration"],"hasImage":true,"imagePrompt":"A modern collaboration workspace, a bright meeting room, and several people reviewing documents around a large display","imagePos":"right"}
			]
		}`,
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Describe the product capabilities, customer value, and use cases of this knowledge collaboration product.",
		Topic:        "Knowledge Collaboration Product Overview",
		Mode:         "fast",
		EnableImages: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.imageCalls != 1 {
		t.Fatalf("imageCalls = %d, want 1", llm.imageCalls)
	}
	if !archiveContainsEntryWithSubstring(t, doc.Bytes, "ppt/slides/_rels/", ".rels", `relationships/image`) {
		t.Fatalf("deck rels missing image relationship")
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", doc.Warnings)
	}
}

func TestServiceGeneratePPTX_SkipsImagesWhenDisabled(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"product capability overview",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"cover","layout":"title","subtitle":"product context and business status","isTitle":true},
				{"title":"product capabilities","layout":"content","points":["Multi-user collaboration","Real-time editing","Enterprise administration"],"hasImage":true,"imagePrompt":"A modern collaboration workspace, a bright meeting room, and several people reviewing documents around a large display","imagePos":"right"}
			]
		}`,
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Describe the product capabilities, customer value, and use cases of this knowledge collaboration product.",
		Topic:        "Knowledge Collaboration Product Overview",
		Mode:         "fast",
		EnableImages: false,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.imageCalls != 0 {
		t.Fatalf("imageCalls = %d, want 0", llm.imageCalls)
	}
	if archiveContainsEntryWithSubstring(t, doc.Bytes, "ppt/slides/_rels/", ".rels", `relationships/image`) {
		t.Fatalf("deck rels should not include image relationship when disabled")
	}
}

func TestServiceGeneratePPTX_DegradesGracefullyWhenImageGenerationFails(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"product capability overview",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"cover","layout":"title","subtitle":"product context and business status","isTitle":true},
				{"title":"product capabilities","layout":"content","points":["Multi-user collaboration","Real-time editing","Enterprise administration"],"hasImage":true,"imagePrompt":"A modern collaboration workspace, a bright meeting room, and several people reviewing documents around a large display","imagePos":"right"}
			]
		}`,
		imageErr: errors.New("image backend unavailable"),
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Describe the product capabilities, customer value, and use cases of this knowledge collaboration product.",
		Topic:        "Knowledge Collaboration Product Overview",
		Mode:         "fast",
		EnableImages: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.imageCalls != 1 {
		t.Fatalf("imageCalls = %d, want 1", llm.imageCalls)
	}
	if len(doc.Warnings) == 0 {
		t.Fatalf("warnings = %#v, want degradation warning", doc.Warnings)
	}
	if got := doc.Warnings[0].Message; !strings.Contains(got, "automatically downgraded to a text-only version") {
		t.Fatalf("warning = %q", got)
	}
	if got := doc.Warnings[0].Message; !strings.Contains(got, "officecli config set-generation") {
		t.Fatalf("warning should include config guidance: %q", got)
	}
	if archiveContainsEntryWithSubstring(t, doc.Bytes, "ppt/slides/_rels/", ".rels", `relationships/image`) {
		t.Fatalf("deck rels should not include image relationship after degradation")
	}
}

func TestBuildPPTXFromJSON_GeneratesGalleryVisuals(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"Visual Gallery Demo",
		"slides":[
			{"title":"Visual Gallery Demo","layout":"title","variant":"title-center","subtitle":"Open with the topic"},
			{"title":"Product Scenes","layout":"gallery","variant":"gallery","narrativeRole":"analysis","sectionIndex":1,"sectionTitle":"Core Storyline","subtitle":"Use visuals to show the product context","visuals":[
				{"label":"Workspace","prompt":"A modern collaboration workspace with documents and comments","caption":"Workspace view"},
				{"label":"Meeting","prompt":"A product review meeting around a large display with dashboard UI","caption":"Review scene"}
			]}
		]
	}`

	fileBytes, _, warnings, _, _, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "Visual Gallery Demo", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if llm.imageCalls < 2 {
		t.Fatalf("imageCalls = %d, want at least 2", llm.imageCalls)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got < 2 {
		t.Fatalf("image count = %d, want at least 2", got)
	}
}

func TestBuildPPTXFromJSON_NormalizesQualityConstraints(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"Quarterly Summary",
		"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
		"slides":[
			{"title":"An Extremely Long First Slide Title That Needs To Be Tightened","layout":"content","points":["The first point is intentionally very long and should be truncated to control slide density","The second point is also deliberately long and should be processed","Third point","Fourth point","Fifth point"],"hasImage":true,"imagePrompt":"A complex market-analysis poster with many visual elements","imagePos":"background"},
			{"title":"Second Slide","layout":"content","points":["Conclusion one","Conclusion two","Conclusion three"],"hasImage":true,"imagePrompt":"An international office scene","imagePos":"left"},
			{"title":"Third Slide","layout":"content","points":["Conclusion one","Conclusion two","Conclusion three"],"hasImage":true,"imagePrompt":"A team meeting","imagePos":"right"},
			{"title":"Fourth Slide","layout":"content","content":"This is a long paragraph. It should be split into multiple readable points. The second sentence adds more context. The third sentence keeps explaining the idea."},
			{"title":"Fifth Slide","layout":"dashboard","metrics":[{"label":"ARR","value":"8.2M","note":"+32% YoY"},{"label":"NDR","value":"118%","note":"Renewal improvement"}]},
			{"title":"Sixth Slide","layout":"chart","chart":{"title":"Regional Revenue","type":"bar","categories":["North America","Europe","Southeast Asia","Middle East","Japan","Korea"],"values":[42,31,28,17,12,9]}},
			{"title":"Seventh Slide","layout":"content","points":["Conclusion one","Conclusion two","Conclusion three"]},
			{"title":"Eighth Slide","layout":"content","points":["This slide should be trimmed"]}		
		]
	}`

	fileBytes, _, warnings, _, _, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "Quarterly Summary", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if got := countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); got != 10 {
		t.Fatalf("slide count = %d, want 10", got)
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got != 0 {
		t.Fatalf("image count = %d, want 0 after image rebalancing", got)
	}
	slide1 := readZipEntry(t, fileBytes, "ppt/slides/slide1.xml")
	if strings.Contains(slide1, "●") {
		t.Fatalf("title slide should not render bullet content: %s", slide1)
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "It should be split into multip") {
		t.Fatalf("content slide should be normalized into readable points")
	}
	for idx := 1; idx <= countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); idx++ {
		rels := readZipEntry(t, fileBytes, filepath.ToSlash(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", idx)))
		slideXML := readZipEntry(t, fileBytes, fmt.Sprintf("ppt/slides/slide%d.xml", idx))
		if strings.Contains(slideXML, "ChartPanel") && strings.Contains(rels, "image") {
			t.Fatalf("chart slide should not keep image rels: %s", rels)
		}
	}
	if len(warnings) == 0 {
		t.Fatalf("warnings = %#v, want normalization warnings", warnings)
	}
}

func TestBuildPPTXFromJSON_DowngradesTimelineChartsToSections(t *testing.T) {
	content := `{
		"title":"Release Cadence",
		"slides":[
			{"title":"Release Cadence","layout":"title","subtitle":"Start with the milestones"},
			{"title":"Cadence and Milestones","layout":"chart","chart":{"title":"Release Stage Plan","type":"bar","categories":["Requirements Freeze","Development Sync","Testing Rollout","General Availability"],"values":[1,3,2,2]}}
		]
	}`

	fileBytes, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Release Cadence", "", false, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "Executive Summary") {
		t.Fatalf("deck should contain an overview slide")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "Cadence and Milest") {
		t.Fatalf("deck should contain the downgraded timeline title")
	}
	for idx := 1; idx <= countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); idx++ {
		slideXML := readZipEntry(t, fileBytes, fmt.Sprintf("ppt/slides/slide%d.xml", idx))
		if strings.Contains(slideXML, "Cadence and Milest") && strings.Contains(slideXML, `r:id="rId1"`) {
			t.Fatalf("timeline slide should be downgraded from chart rels:\n%s", slideXML)
		}
	}
}

func TestBuildPPTXFromJSON_BuildsLocalPreviewSidecars(t *testing.T) {
	content := `{
		"title":"Local Preview Test",
		"slides":[
			{"title":"Local Preview Test","layout":"title","variant":"title-center","subtitle":"Start with the structure"},
			{"title":"Key Takeaway","layout":"content","variant":"bullets","subtitle":"Lead with the conclusion","points":["Point one","Point two","Point three"]}
		]
	}`

	_, _, _, previewHTML, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Local Preview Test", "executive-dark", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if !strings.Contains(string(previewHTML), "Preset: executive-dark") {
		t.Fatalf("preview html = %s", string(previewHTML))
	}
	if !strings.Contains(string(previewJSON), `"stylePreset": "executive-dark"`) {
		t.Fatalf("preview json = %s", string(previewJSON))
	}
}

func TestServiceGeneratePPTX_RetriesOnceWhenJSONIsTruncated(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponses: []string{
			`{"title":"Knowledge Collaboration Product Overview","slides":[{"title":"Cover","layout":"title","subtitle":"One-line takeaway","isTitle":true}`,
		},
		structuredResponse: `{
			"title":"Knowledge Collaboration Product Overview",
			"slides":[
				{"title":"Cover","layout":"title","subtitle":"One-line takeaway","isTitle":true},
				{"title":"Product Capabilities","layout":"content","points":["Higher collaboration efficiency","Clear permission governance","A clear rollout path"]}
			]
		}`,
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Describe the product capabilities, customer value, and use cases of this knowledge collaboration product.",
		Topic:        "Knowledge Collaboration Product Overview",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.jsonCallCount != 1 {
		t.Fatalf("jsonCallCount = %d, want 1", llm.jsonCallCount)
	}
	if llm.structuredCallCount != 1 {
		t.Fatalf("structuredCallCount = %d, want 1", llm.structuredCallCount)
	}
	if llm.lastStructuredReq.Schema.Name != "pptx_payload_repair" {
		t.Fatalf("schema name = %q", llm.lastStructuredReq.Schema.Name)
	}
	if len(llm.lastStructuredReq.Messages) != 3 {
		t.Fatalf("repair messages = %d, want 3", len(llm.lastStructuredReq.Messages))
	}
	if !archiveContainsEntryWithSubstring(t, doc.Bytes, "ppt/slides/slide", ".xml", "Higher collaboration efficiency") {
		t.Fatalf("deck should contain repaired slide content")
	}
}

func TestServiceGenerateDOCXEmitsProgressEvents(t *testing.T) {
	collector := &runtimeProgressCollector{}
	service := NewService(&fakeLLMClient{
		jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`,
	}, collector)

	_, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeDOCX,
		Prompt:       "Introduce this enterprise collaboration platform",
		Topic:        "Enterprise Collaboration Platform Overview",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	joined := make([]string, 0, len(collector.events))
	for _, event := range collector.events {
		joined = append(joined, event.Step+":"+event.Status+":"+event.Content)
	}
	output := strings.Join(joined, "\n")
	for _, needle := range []string{
		"generate_llm:running:Requesting DOCX content from the LLM",
		"generate_llm:completed:Received DOCX structure output",
		"assemble:running:Assembling the DOCX file",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("progress output missing %q:\n%s", needle, output)
		}
	}
}
