package runtime

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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
	imageResults        []*engine.ImageGenerationResult
	imageErr            error
	imageErrors         []error
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
	if len(f.imageErrors) > 0 {
		idx := f.imageCalls - 1
		if idx >= len(f.imageErrors) {
			idx = len(f.imageErrors) - 1
		}
		if f.imageErrors[idx] != nil {
			return nil, f.imageErrors[idx]
		}
	}
	if len(f.imageResults) > 0 {
		idx := f.imageCalls - 1
		if idx >= len(f.imageResults) {
			idx = len(f.imageResults) - 1
		}
		if f.imageResults[idx] != nil {
			return f.imageResults[idx], nil
		}
	}
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
		!strings.Contains(contentXMLs["xl/sharedStrings.xml"], "120") {
		t.Fatalf("workbook xml = %q\nshared strings = %q", contentXMLs["xl/workbook.xml"], contentXMLs["xl/sharedStrings.xml"])
	}
}

func TestPrepareAgentPayloadForReport(t *testing.T) {
	workbookBytes, err := officegen.NewXLSXGenerator().Generate([]officegen.XlsxSheet{
		{
			Name: "Summary",
			Rows: [][]string{
				{"Region", "Revenue", "Growth"},
				{"North America", "128", "+12%"},
				{"Europe", "96", "+8%"},
			},
		},
	}, officegen.XLSXOptions{Title: "Q2 Review", Creator: "OfficeCLI"})
	if err != nil {
		t.Fatalf("Generate workbook: %v", err)
	}
	workbookPath := filepath.Join(t.TempDir(), "source.xlsx")
	if err := os.WriteFile(workbookPath, workbookBytes, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	prepared, err := PrepareAgentPayload(PrepareParams{
		DocumentType:   engine.DocumentTypeReport,
		Topic:          "Q2 Review",
		SourceFilePath: workbookPath,
	})
	if err != nil {
		t.Fatalf("PrepareAgentPayload: %v", err)
	}
	if prepared.PreferredTool != "office.render" || !prepared.PrepareRequired {
		t.Fatalf("unexpected prepare metadata: %#v", prepared)
	}
	if !strings.Contains(prepared.WorkbookSummary, "North America") {
		t.Fatalf("unexpected workbook summary: %s", prepared.WorkbookSummary)
	}
	if len(prepared.BaseReportJSON) == 0 {
		t.Fatal("expected base report json")
	}
}

func TestServiceRenderDOCXWithoutLLM(t *testing.T) {
	service := NewService(nil, nil)
	doc, err := service.Render(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeDOCX,
		Topic:        "Quarterly Brief",
	}, json.RawMessage(`{"title":"Quarterly Brief","sections":[{"heading":"Summary","level":1,"paragraphs":["Delivery-ready content."]}]}`))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["word/document.xml"], "Quarterly Brief") {
		t.Fatalf("document xml = %q", contentXMLs["word/document.xml"])
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

func TestServiceRenderPPTXWithoutTextLLMCalls(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	service := NewService(llm, nil)

	doc, err := service.Render(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Style:        "editorial-light",
		EnableImages: true,
	}, json.RawMessage(`{
		"title":"Enterprise Collaboration Platform Overview",
		"stylePreset":"editorial-light",
		"theme":null,
		"slides":[
			{"title":"Enterprise Collaboration Platform Overview","content":"","isTitle":true,"layout":"title","variant":"title-center","narrativeRole":"cover","sectionIndex":0,"sectionTitle":"","subtitle":"Product context and business status","points":[],"sections":[],"chart":null,"metrics":[],"source":"","bgColor":"","bgColor2":"","hasImage":true,"imagePrompt":"A polished enterprise dashboard hero image","imagePos":"background","visuals":[]}
		]
	}`))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if llm.jsonCallCount != 0 || llm.structuredCallCount != 0 {
		t.Fatalf("unexpected text llm calls: json=%d structured=%d", llm.jsonCallCount, llm.structuredCallCount)
	}
	if llm.imageCalls == 0 {
		t.Fatal("expected image call")
	}
	if countZipEntries(doc.Bytes, "ppt/media/", ".png") == 0 {
		t.Fatal("expected embedded ppt media")
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
	explainerPrompt := BuildPPTXPrompt("minecraft 游戏介绍", generateengine.PromptTarget{}, true)
	if !strings.Contains(explainerPrompt, "go straight into the topic with a 6-8 slide explainer arc") {
		t.Fatalf("explainer prompt missing direct explainer outline:\n%s", explainerPrompt)
	}
	if !strings.Contains(explainerPrompt, "Do not insert contents or chapter-divider scaffolding for this topic.") {
		t.Fatalf("explainer prompt missing scaffold skip rule:\n%s", explainerPrompt)
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
	if llm.imageCalls != 3 {
		t.Fatalf("imageCalls = %d, want 3", llm.imageCalls)
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
	if llm.imageCalls != 3 {
		t.Fatalf("imageCalls = %d, want 3", llm.imageCalls)
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
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got > 2 {
		t.Fatalf("image count = %d, want at most 2 after image rebalancing", got)
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

func TestBuildPPTXFromJSON_ExplainerUsesMixedLayoutsAndSkipsScaffold(t *testing.T) {
	content := `{
		"title":"Minecraft Introduction",
		"slides":[
			{"title":"Minecraft Introduction","layout":"title","subtitle":"A beginner-friendly overview"},
			{"title":"Overview","layout":"content","sections":[
				{"heading":"High Replayability","detail":"Minecraft is best understood as an open-ended sandbox built around exploration, building, and survival."},
				{"heading":"Creative Range","detail":"Players can keep discovering new goals instead of following one fixed path."},
				{"heading":"Shared Play","detail":"Friends can explore and build together in the same world."}
			]},
			{"title":"Main Loop","layout":"content","points":["Learn a few core recipes first","Gather materials and build a simple shelter","Use short sessions to discover the main loop"]},
			{"title":"Standout Traits","layout":"content","points":["Try the mode that matches your mood and skill level","The same world can feel relaxing, creative, or challenging","Replayability comes from self-directed goals"]},
			{"title":"Audience Fit","layout":"content","points":["Beginners can start small","Creative players can experiment freely","Challenge seekers can focus on survival"]}
		]
	}`

	fileBytes, _, _, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "minecraft 游戏介绍", "", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if got := countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); got != 6 {
		t.Fatalf("slide count = %d, want 6", got)
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "Contents") {
		t.Fatalf("explainer deck should not contain a contents slide")
	}
	for _, needle := range []string{"What It Is", "Learn a few", "How to Start"} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", needle) {
			t.Fatalf("deck should preserve %q", needle)
		}
	}
	if !strings.Contains(string(previewJSON), `"stylePreset": "explainer-voxel-light"`) {
		t.Fatalf("preview json = %s", string(previewJSON))
	}
	for _, needle := range []string{`"variant": "bullets-plain"`, `"variant": "timeline-axis"`, `"variant": "comparison-columns"`, `"variant": "sections-grid-3up"`, `"variant": "timeline-steps"`} {
		if !strings.Contains(string(previewJSON), needle) {
			t.Fatalf("preview json missing %q:\n%s", needle, string(previewJSON))
		}
	}
}

func TestSuggestStylePreset_RoutesChineseBusinessThemes(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		archetype pptxArchetype
		want      string
	}{
		{name: "pitch", text: "AI 客服质检平台融资路演", archetype: pptxArchetypeGeneral, want: officegen.StylePresetInvestorWarm},
		{name: "sales procurement", text: "企业协作平台采购建议与价值解读", archetype: pptxArchetypeCompany, want: officegen.StylePresetSlateSerif},
		{name: "project", text: "集团数字化项目实施方案", archetype: pptxArchetypeGeneral, want: officegen.StylePresetProjectForest},
		{name: "training", text: "新员工远程协作入职培训", archetype: pptxArchetypeTraining, want: officegen.StylePresetTrainingManual},
		{name: "review", text: "2026 年第一季度经营复盘", archetype: pptxArchetypeOps, want: officegen.StylePresetReviewCopper},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := suggestStylePreset("", tc.archetype, tc.text); got != tc.want {
				t.Fatalf("suggestStylePreset(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestNormalizePPTXPayload_AutoThemeOverridesLLMThemeWhenStyleIsImplicit(t *testing.T) {
	payload := &pptxPayload{
		Title:       "企业协作平台采购建议与价值解读",
		StylePreset: "editorial-light",
		Theme: &officegen.SlideTheme{
			PrimaryColor:   "1A73E8",
			AccentColor:    "E8710A",
			BackgroundType: "gradient",
			BgColor1:       "F0F4FF",
			BgColor2:       "FFFFFF",
			TextColor:      "0F172A",
			TitleTextColor: "0F172A",
		},
		Slides: []officegen.Slide{
			{Title: "企业协作平台采购建议与价值解读", Layout: "title", Subtitle: "管理层摘要"},
		},
	}

	normalizePPTXPayload(payload, "企业协作平台采购建议与价值解读", "", true)

	if payload.StylePreset != officegen.StylePresetSlateSerif {
		t.Fatalf("style preset = %q, want %q", payload.StylePreset, officegen.StylePresetSlateSerif)
	}
	if payload.Theme == nil || payload.Theme.AccentColor != "0F766E" || payload.Theme.BgColor1 != "EDF5F4" {
		t.Fatalf("theme = %+v, want collaboration-slate preset theme", payload.Theme)
	}
}

func TestDiversifyBusinessLayouts_ReducesRepeatedSectionCardsAndClosing(t *testing.T) {
	slides := []officegen.Slide{
		{Title: "Cover", Layout: "title"},
		{Title: "Summary", Layout: "content", Variant: "sections-grid", Sections: []officegen.SlideSection{{Heading: "A", Detail: "Alpha"}, {Heading: "B", Detail: "Beta"}, {Heading: "C", Detail: "Gamma"}}},
		{Title: "Capabilities", Layout: "content", Variant: "sections-grid", Sections: []officegen.SlideSection{{Heading: "A", Detail: "Alpha"}, {Heading: "B", Detail: "Beta"}, {Heading: "C", Detail: "Gamma"}}},
		{Title: "Action Plan", Layout: "closing", Variant: "closing", Sections: []officegen.SlideSection{{Heading: "Now", Detail: "Do first"}, {Heading: "Next", Detail: "Do second"}}},
		{Title: "Next Steps", Layout: "closing", Variant: "closing", Sections: []officegen.SlideSection{{Heading: "Week 1", Detail: "Kick off"}, {Heading: "Week 2", Detail: "Review"}}},
	}

	got := diversifyBusinessLayouts(slides, pptxArchetypeOps)

	if got[1].Layout != "content" || got[1].Variant != "bullets" || len(got[1].Points) == 0 {
		t.Fatalf("first repeated sections-grid slide should become bullets: %+v", got[1])
	}
	if got[3].Layout == "closing" {
		t.Fatalf("penultimate closing slide should be diversified: %+v", got[3])
	}
	if got[4].Layout != "closing" {
		t.Fatalf("final slide should remain closing: %+v", got[4])
	}
}

func TestBuildPPTXFromJSON_ExplainerImagesUseHeroAndGameplayVisuals(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"Minecraft Introduction",
		"slides":[
			{"title":"Minecraft Introduction","layout":"title","subtitle":"A beginner-friendly overview"},
			{"title":"What It Is","layout":"content","points":["A sandbox world built from blocks","Players gather, craft, and build","Different modes change the experience"]}
		]
	}`

	fileBytes, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "minecraft 游戏介绍", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if got := countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); got != 7 {
		t.Fatalf("slide count = %d, want 7", got)
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got != 4 {
		t.Fatalf("image count = %d, want 4", got)
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "Example / Gameplay Visual") {
		t.Fatalf("deck should contain the visual example slide")
	}
}

func TestBuildPPTXFromJSON_ExplainerSingleVisualFallsBackToImageRight(t *testing.T) {
	llm := &fakeLLMClient{
		imageResults: []*engine.ImageGenerationResult{
			nil,
			{Data: mustTinyPNG(t), MIME: "image/png"},
			nil,
		},
		imageErrors: []error{
			errors.New("cover failed"),
			nil,
			errors.New("second visual failed"),
		},
	}
	content := `{
		"title":"Minecraft Introduction",
		"slides":[
			{"title":"Minecraft Introduction","layout":"title","subtitle":"A beginner-friendly overview"},
			{"title":"What It Is","layout":"content","points":["A sandbox world built from blocks","Players gather, craft, and build","Different modes change the experience"]}
		]
	}`

	_, _, warnings, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "minecraft 游戏介绍", "", true, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if !strings.Contains(string(previewJSON), `"title": "Example / Gameplay Visual"`) || !strings.Contains(string(previewJSON), `"variant": "image-right-focus"`) {
		t.Fatalf("preview json should downgrade the example slide to image-right:\n%s", string(previewJSON))
	}
	if len(warnings) == 0 || !strings.Contains(warnings[len(warnings)-1].Message, "successfully generated visuals were kept") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestBuildPPTXFromJSON_ExplainerNoVisualSuccessFallsBackToTextPage(t *testing.T) {
	llm := &fakeLLMClient{
		imageErrors: []error{
			errors.New("cover failed"),
			errors.New("visual 1 failed"),
			errors.New("visual 2 failed"),
		},
	}
	content := `{
		"title":"Minecraft Introduction",
		"slides":[
			{"title":"Minecraft Introduction","layout":"title","subtitle":"A beginner-friendly overview"},
			{"title":"What It Is","layout":"content","points":["A sandbox world built from blocks","Players gather, craft, and build","Different modes change the experience"]}
		]
	}`

	_, _, warnings, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "minecraft 游戏介绍", "", true, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if strings.Contains(string(previewJSON), `"layout": "gallery"`) && strings.Contains(string(previewJSON), `"title": "Example / Gameplay Visual"`) {
		t.Fatalf("preview json should not keep the example slide as gallery when no visuals succeed:\n%s", string(previewJSON))
	}
	if !strings.Contains(string(previewJSON), `"title": "Example / Gameplay Visual"`) || !strings.Contains(string(previewJSON), `"variant": "bullets-callout"`) {
		t.Fatalf("preview json should turn the example slide into a text explain page:\n%s", string(previewJSON))
	}
	if len(warnings) == 0 || !strings.Contains(warnings[len(warnings)-1].Message, "text-only version") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestBuildFallbackImagePrompt_MinecraftUsesVoxelConstraints(t *testing.T) {
	prompt := buildFallbackImagePrompt(officegen.Slide{
		Title:         "Minecraft Introduction",
		NarrativeRole: "analysis",
		Subtitle:      "Show the blocky sandbox world and the survival loop",
	}, "Minecraft Introduction")
	for _, needle := range []string{"blocky voxel sandbox", "Minecraft-like cubic terrain", "crafting", "biomes", "survival shelter", "block building"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
	for _, needle := range []string{"hand-painted fantasy", "corporate diagram"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing negative constraint %q: %s", needle, prompt)
		}
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
