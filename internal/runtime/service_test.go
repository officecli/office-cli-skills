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
	"unicode/utf8"

	"github.com/officecli/officecli-internal/engine"
	generateengine "github.com/officecli/officecli-internal/engine/generate"
	"github.com/officecli/officecli-internal/pkg/officegen"
	"github.com/officecli/officecli-internal/pkg/ooxmledit"
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

func containsIssueCode(items []engine.GenerateIssue, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
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

func intPtr(value int) *int {
	return &value
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
		!strings.Contains(contentXMLs["xl/worksheets/sheet1.xml"], ">120<") {
		t.Fatalf("workbook xml = %q\nshared strings = %q\nsheet xml = %q", contentXMLs["xl/workbook.xml"], contentXMLs["xl/sharedStrings.xml"], contentXMLs["xl/worksheets/sheet1.xml"])
	}
}

func TestServiceGenerateIMGUsesImageProviderAndRatio(t *testing.T) {
	imageBytes := []byte("server-image")
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: imageBytes, MIME: "image/png", CreditBalance: intPtr(9), CreditsCharged: intPtr(3)},
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeIMG,
		Prompt:       "A polished product launch hero image",
		Topic:        "Launch Visual",
		ImageRatio:   "portrait",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if doc.DocumentType != "img" {
		t.Fatalf("document type = %q", doc.DocumentType)
	}
	if doc.DocumentName != "Launch_Visual.png" {
		t.Fatalf("document name = %q", doc.DocumentName)
	}
	if string(doc.Bytes) != string(imageBytes) {
		t.Fatalf("image bytes = %q", string(doc.Bytes))
	}
	if llm.imageCalls != 1 {
		t.Fatalf("image calls = %d", llm.imageCalls)
	}
	if llm.lastImageRequest.TargetAspectRatio != 9.0/16.0 {
		t.Fatalf("aspect ratio = %f", llm.lastImageRequest.TargetAspectRatio)
	}
	if !strings.Contains(llm.lastImageRequest.Prompt, "product launch hero") {
		t.Fatalf("prompt = %q", llm.lastImageRequest.Prompt)
	}
	if doc.HostedCreditBalance == nil || *doc.HostedCreditBalance != 9 {
		t.Fatalf("hosted credit balance = %#v", doc.HostedCreditBalance)
	}
	if doc.HostedCreditsCharged == nil || *doc.HostedCreditsCharged != 3 {
		t.Fatalf("hosted credits charged = %#v", doc.HostedCreditsCharged)
	}
}

func TestPPTXBuildOptionsCreditChargedSinkAccumulates(t *testing.T) {
	called := 0
	totalCharged := 0
	sink := func(charged int) {
		called++
		totalCharged += charged
	}
	opts := PPTXBuildOptions{
		CreditChargedSink: sink,
	}
	opts.CreditChargedSink(4)
	opts.CreditChargedSink(7)
	opts.CreditChargedSink(11)
	if called != 3 {
		t.Fatalf("expected 3 sink invocations, got %d", called)
	}
	if totalCharged != 22 {
		t.Fatalf("expected accumulated total 22, got %d", totalCharged)
	}
}

func TestServiceGenerateIMGRejectsEmptyImageData(t *testing.T) {
	service := NewService(&fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{MIME: "image/png"},
	}, nil)

	_, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeIMG,
		Prompt:       "A polished product launch hero image",
		Topic:        "Launch Visual",
		ImageRatio:   "square",
	})
	if err == nil || !strings.Contains(err.Error(), "image generation returned empty data") {
		t.Fatalf("err = %v", err)
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
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png", CreditBalance: intPtr(42), CreditsCharged: intPtr(5)},
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
	if doc.HostedCreditsCharged == nil || *doc.HostedCreditsCharged != 5 {
		t.Fatalf("hosted credits charged = %#v", doc.HostedCreditsCharged)
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
		`"visual": {"kind": "image"`,
		`"prompt": "A concrete visual prompt that can be sent directly to an image model"`,
		"Use images sparingly. Prefer at most one hero image slide plus at most one gallery slide",
		"Do not add images to chart, dashboard, toc, or closing layouts",
		"For gallery slides, use a visual image for the page theme",
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
	if !strings.Contains(prompt, "Do not output visual objects or image fields.") {
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
		"takeaway must be a slide-level conclusion sentence",
		"Use at most 3 sections, at most 4 dashboard metrics",
		"Do not use charts for priorities, milestones, strategy, risks, or process flows",
		"Only use rollout-style endings when the request explicitly asks for rollout",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q:\n%s", needle, prompt)
		}
	}
}

func TestBuildPPTXPrompt_RequestsSemanticDeckSpec(t *testing.T) {
	prompt := BuildPPTXPrompt("Introduce product capabilities", generateengine.PromptTarget{}, true)
	for _, needle := range []string{
		`"headline": "Cover Title"`,
		`"takeaway": "One-sentence takeaway"`,
		`"blocks": [`,
		`{"type": "sections"`,
		`"visual": {"kind": "image"`,
		"Use role/headline/takeaway/blocks/visual as the preferred semantic schema",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing semantic schema marker %q:\n%s", needle, prompt)
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

func TestFitTextForLayout_PreservesLongSpacedText(t *testing.T) {
	input := "This sentence has many words and no punctuation before the maximum layout boundary"
	got := fitTextForLayout(input, 32)
	if got != input {
		t.Fatalf("fitTextForLayout() = %q", got)
	}
}

func TestFitTextForLayout_FinishesTruncatedPhrase(t *testing.T) {
	got := fitTextForLayout("Clear decision rights prevent duplicated work and missed handoffs across functions", 18)
	if got != "Clear decision rights prevent duplicated work and missed handoffs across functions" {
		t.Fatalf("fitTextForLayout() produced unfinished phrase: %q", got)
	}
}

func TestNormalizePointsAndSections_ControlTextDensity(t *testing.T) {
	points := normalizePoints([]string{
		"This point is intentionally much too long for a slide bullet and needs to be shortened before rendering",
	}, 4, 32)
	if len(points) != 1 || strings.HasSuffix(points[0], " and") {
		t.Fatalf("points = %#v, want one complete point", points)
	}

	sections := normalizeSections([]officegen.SlideSection{
		{
			Heading: "A very long section heading that should not consume the whole card",
			Detail:  "A very long section detail that would otherwise create dense unreadable card copy in the generated slide layout",
		},
	}, 3)
	if len(sections) != 1 {
		t.Fatalf("sections = %#v, want one section", sections)
	}
	if strings.HasSuffix(sections[0].Heading, " and") || strings.HasSuffix(sections[0].Detail, " and") {
		t.Fatalf("section was not kept semantically complete: %#v", sections[0])
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
	if payload.Slides[len(payload.Slides)-1].Layout != "closing" || len(payload.Slides[len(payload.Slides)-1].Sections) < 2 {
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
	if payload.Slides[len(payload.Slides)-1].Layout != "closing" || len(payload.Slides[len(payload.Slides)-1].Sections) < 2 {
		t.Fatalf("training closing slide = %#v", payload.Slides[len(payload.Slides)-1])
	}
}

func TestNormalizePPTXSlide_BusinessClosingUsesDecisionBannerAndDropsBackgroundImage(t *testing.T) {
	coverBudget := 0
	closingBudget := 0
	imageBudget := 1
	galleryBudget := 0
	visualBudget := 0

	slide, imageKept, _ := normalizePPTXSlide(officegen.Slide{
		Title:         "Recommendation",
		Layout:        "closing",
		NarrativeRole: "closing",
		Subtitle:      "Approve the first pilot now.",
		Sections: []officegen.SlideSection{
			{Heading: "Decision", Detail: "Approve the pilot scope this week."},
			{Heading: "Guardrail", Detail: "Keep the first validation cycle limited to one team."},
		},
		HasImage:    true,
		ImagePos:    "background",
		ImagePrompt: "A boardroom hero background",
	}, 5, "OfficeCLI", pptxArchetypeCompany, true, &coverBudget, &closingBudget, &imageBudget, &galleryBudget, &visualBudget)

	if slide.Variant != "closing-decision-banner" {
		t.Fatalf("variant = %q, want closing-decision-banner", slide.Variant)
	}
	if slide.HasImage || imageKept {
		t.Fatalf("business closing should drop background image: %+v kept=%v", slide, imageKept)
	}
}

func TestNormalizePPTXSlide_ExplainerClosingUsesStarterGuidanceAndDropsBackgroundImage(t *testing.T) {
	coverBudget := 0
	closingBudget := 1
	imageBudget := 0
	galleryBudget := 0
	visualBudget := 0

	slide, imageKept, _ := normalizePPTXSlide(officegen.Slide{
		Title:         "How to Start",
		Layout:        "closing",
		NarrativeRole: "closing",
		Subtitle:      "Start small and learn the loop by doing.",
		Sections: []officegen.SlideSection{
			{Heading: "Pick One Mode", Detail: "Creative or Survival."},
			{Heading: "Try One Goal", Detail: "Build one shelter."},
		},
		HasImage:    true,
		ImagePos:    "background",
		ImagePrompt: "A bright voxel landscape at sunset",
	}, 5, "Minecraft", pptxArchetypeExplainer, true, &coverBudget, &closingBudget, &imageBudget, &galleryBudget, &visualBudget)

	if slide.Variant != "closing-starter-guidance" {
		t.Fatalf("variant = %q, want closing-starter-guidance", slide.Variant)
	}
	if slide.HasImage || imageKept {
		t.Fatalf("closing slides should not keep background image: %+v kept=%v", slide, imageKept)
	}
}

func TestDefaultActionSlide_GeneralAvoidsLegacyNextStepsTemplateAndMetaCopy(t *testing.T) {
	slide := defaultActionSlide(pptxArchetypeGeneral, "AI PPT 生成架构升级")
	if slide.Title == "Next Steps" {
		t.Fatalf("general fallback should no longer use legacy title: %+v", slide)
	}
	if slide.Subtitle == "Close with a small set of actions, owners, and validation points" {
		t.Fatalf("general fallback should no longer use legacy subtitle: %+v", slide)
	}
	text := strings.ToLower(slide.Title + " " + slide.Subtitle)
	for _, section := range slide.Sections {
		text += " " + strings.ToLower(section.Heading+" "+section.Detail)
	}
	for _, forbidden := range []string{"close with", "proof point", "proof points needed", "highest-friction document workflow"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("generic closing should not expose scaffold copy %q: %+v", forbidden, slide)
		}
	}
	if slide.Variant != "closing-decision-banner" {
		t.Fatalf("variant = %q, want closing-decision-banner", slide.Variant)
	}
	if len(slide.Sections) != 3 {
		t.Fatalf("sections = %#v, want 3 concrete closing items", slide.Sections)
	}
}

func TestNormalizePPTXPayload_DoesNotBackfillClosingImage(t *testing.T) {
	payload := &pptxPayload{
		Title: "AI PPT 生成架构升级",
		Slides: []officegen.Slide{
			{Title: "AI PPT 生成架构升级", Layout: "title", IsTitle: true},
			{Title: "Core Architecture", Layout: "content", Sections: []officegen.SlideSection{{Heading: "Spec", Detail: "Use semantic slides"}, {Heading: "Renderer", Detail: "Own layout and contrast"}}},
			{Title: "Recommendation", Layout: "closing", Variant: "closing-decision-banner", Sections: []officegen.SlideSection{{Heading: "Decision", Detail: "Run a scoped pilot."}}},
		},
	}
	normalizePPTXPayload(payload, "AI PPT 生成架构升级", "", true)
	if len(payload.Slides) == 0 {
		t.Fatal("slides should not be empty")
	}
	last := payload.Slides[len(payload.Slides)-1]
	if last.Layout != "closing" {
		t.Fatalf("last slide should remain closing: %+v", last)
	}
	if last.HasImage || strings.TrimSpace(last.ImagePrompt) != "" || strings.TrimSpace(last.ImagePos) != "" {
		t.Fatalf("closing slide should not be backfilled with image: %+v", last)
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
	if llm.imageCalls != 2 {
		t.Fatalf("imageCalls = %d, want 2", llm.imageCalls)
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
	if llm.imageCalls != 2 {
		t.Fatalf("imageCalls = %d, want 2", llm.imageCalls)
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

func TestBuildPPTXFromJSON_AcceptsSemanticPayload(t *testing.T) {
	content := `{
		"title":"Enterprise Collaboration Platform",
		"subtitle":"Board-ready narrative",
		"stylePreset":"executive-dark",
		"slides":[
			{"role":"cover","headline":"Enterprise Collaboration Platform","takeaway":"A concise board-ready story"},
			{"role":"summary","headline":"Readiness Snapshot","takeaway":"The platform is ready when value, governance, and rollout are aligned.","blocks":[{"type":"sections","sections":[
				{"heading":"Value","detail":"Teams reduce coordination drag through one shared workspace."},
				{"heading":"Governance","detail":"Permissions and audit trails keep enterprise controls visible."},
				{"heading":"Rollout","detail":"Start with a focused department before broad expansion."}
			]}]},
			{"role":"evidence","headline":"Adoption Evidence","takeaway":"The strongest signal is cross-team activation, not isolated usage.","blocks":[{"type":"chart","chart":{"title":"Activation Index","type":"bar","categories":["Pilot","Expansion","Scaled"],"values":[32,58,81]},"items":["Expansion cohorts show higher activation.","Scaled teams sustain the highest usage."]}]},
			{"role":"action","headline":"Decision Path","takeaway":"Approve a staged rollout with explicit owners and acceptance criteria.","blocks":[{"type":"actions","items":["Confirm pilot owner this month","Measure activation and governance readiness","Expand only after two validation cycles"]}]},
			{"role":"closing","headline":"Closing Decision","takeaway":"Move forward with a controlled rollout, not a broad-bang launch.","blocks":[{"type":"sections","sections":[
				{"heading":"Ask","detail":"Approve the pilot-to-scale path."},
				{"heading":"Guardrail","detail":"Review adoption and control metrics before expansion."}
			]}]}
		]
	}`

	fileBytes, _, _, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Enterprise Collaboration Platform", "", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	for _, needle := range []string{"Readiness Snapshot", "Adoption Evidence", "Decision Path", "Closing Decision"} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", needle) {
			t.Fatalf("semantic deck should preserve headline %q", needle)
		}
		if !strings.Contains(string(previewJSON), needle) {
			t.Fatalf("preview json missing semantic headline %q:\n%s", needle, string(previewJSON))
		}
	}
	if !strings.Contains(string(previewJSON), `"layout": "chart"`) {
		t.Fatalf("semantic evidence block should map to chart layout:\n%s", string(previewJSON))
	}
	if !strings.Contains(string(previewJSON), `"layout": "closing"`) {
		t.Fatalf("semantic action or closing role should map to closing layout:\n%s", string(previewJSON))
	}
}

func TestBuildPPTXFromJSON_SemanticPayloadUsesControlledDesignSystem(t *testing.T) {
	content := `{
		"title":"Controlled Design Demo",
		"stylePreset":"executive-dark",
		"theme":{"preset":"executive","primaryColor":"F8FAFC","accentColor":"F9FAFB","backgroundColor":"F8FAFC","surfaceColor":"F9FAFB","textColor":"F8FAFC","titleColor":"F8FAFC"},
		"slides":[
			{"role":"cover","headline":"Controlled Design Demo","takeaway":"Low-level visual tokens from the model must not control the deck.","bgColor":"101010","bgColor2":"111111"},
			{"role":"summary","headline":"Readable Summary","takeaway":"The renderer owns contrast and surface choices.","bgColor":"F9FAFB","blocks":[{"type":"sections","sections":[
				{"heading":"Theme","detail":"Unsafe colors are ignored for semantic payloads."},
				{"heading":"Layout","detail":"Slides use controlled layout variants."}
			]}]},
			{"role":"closing","headline":"Decision","takeaway":"Keep editable slides while avoiding invisible text.","blocks":[{"type":"actions","items":["Use semantic content only","Render with controlled design tokens"]}]}
		]
	}`

	fileBytes, _, _, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Controlled Design Demo", "", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	var preview struct {
		Theme struct {
			PrimaryColor   string `json:"primaryColor"`
			AccentColor    string `json:"accentColor"`
			BgColor1       string `json:"bgColor1"`
			TextColor      string `json:"textColor"`
			TitleTextColor string `json:"titleTextColor"`
		} `json:"theme"`
		Slides []officegen.Slide `json:"slides"`
	}
	if err := json.Unmarshal(previewJSON, &preview); err != nil {
		t.Fatalf("unmarshal preview json: %v\n%s", err, string(previewJSON))
	}
	for _, unsafe := range []string{"F8FAFC", "F9FAFB"} {
		if preview.Theme.TextColor == unsafe || preview.Theme.TitleTextColor == unsafe || preview.Theme.PrimaryColor == unsafe || preview.Theme.AccentColor == unsafe {
			t.Fatalf("semantic payload leaked unsafe model theme into preview: %+v", preview.Theme)
		}
	}
	for idx, slide := range preview.Slides {
		if slide.BgColor != "" || slide.BgColor2 != "" {
			t.Fatalf("slide %d kept model-controlled background overrides: %+v", idx+1, slide)
		}
	}
	for _, unsafe := range []string{"101010", "111111", "F9FAFB"} {
		if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", unsafe) {
			t.Fatalf("pptx XML should not contain model-controlled unsafe color %s", unsafe)
		}
	}
}

func TestBuildPPTXFromJSON_SemanticGalleryVisualGeneratesAsset(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"Product Scenes",
		"slides":[
			{"role":"cover","headline":"Product Scenes","takeaway":"Show the usage context"},
			{"role":"analysis","layout":"gallery","variant":"gallery","headline":"Workspace Scene","takeaway":"A visual page should keep the generated asset editable as an image object.","blocks":[{"type":"bullets","items":["Review workflow","Shared comments"]}],"visual":{"kind":"image","position":"right","prompt":"A modern product workspace with document comments and review panels, no text overlay"}}
		]
	}`

	fileBytes, _, warnings, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "Product Scenes", "", true, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if llm.imageCalls == 0 {
		t.Fatalf("imageCalls = %d, want semantic visual image generation", llm.imageCalls)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got == 0 {
		t.Fatalf("image count = %d, want generated visual asset", got)
	}
	if !strings.Contains(string(previewJSON), `"layout": "gallery"`) {
		t.Fatalf("semantic visual should remain a gallery slide:\n%s", string(previewJSON))
	}
}

func TestBuildPPTXFromJSON_ReducesAdjacentVariantRepetition(t *testing.T) {
	content := `{
		"title":"Operating Review",
		"slides":[
			{"title":"Operating Review","layout":"title","subtitle":"A concise review"},
			{"title":"Summary","layout":"content","variant":"sections-grid","sections":[{"heading":"A","detail":"Alpha"},{"heading":"B","detail":"Beta"},{"heading":"C","detail":"Gamma"}]},
			{"title":"Customer Value","layout":"content","variant":"sections-grid","sections":[{"heading":"A","detail":"Alpha"},{"heading":"B","detail":"Beta"},{"heading":"C","detail":"Gamma"}]},
			{"title":"Operating Model","layout":"content","variant":"sections-grid","sections":[{"heading":"A","detail":"Alpha"},{"heading":"B","detail":"Beta"},{"heading":"C","detail":"Gamma"}]},
			{"title":"Execution Path","layout":"content","variant":"sections-grid","sections":[{"heading":"A","detail":"Alpha"},{"heading":"B","detail":"Beta"},{"heading":"C","detail":"Gamma"}]},
			{"title":"Risk Controls","layout":"content","variant":"sections-grid","sections":[{"heading":"A","detail":"Alpha"},{"heading":"B","detail":"Beta"},{"heading":"C","detail":"Gamma"}]},
			{"title":"Next Decision","layout":"closing","variant":"closing","sections":[{"heading":"Ask","detail":"Approve the next stage"},{"heading":"Guardrail","detail":"Review adoption before scale"}]}
		]
	}`

	_, _, _, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Operating Review", "", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	var preview struct {
		Slides []struct {
			Title   string `json:"title"`
			Layout  string `json:"layout"`
			Variant string `json:"variant"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(previewJSON, &preview); err != nil {
		t.Fatalf("unmarshal preview json: %v\n%s", err, string(previewJSON))
	}
	if len(preview.Slides) < 6 {
		t.Fatalf("slide count = %d, want at least 6", len(preview.Slides))
	}
	for idx := 2; idx < len(preview.Slides); idx++ {
		prev := preview.Slides[idx-1]
		cur := preview.Slides[idx]
		if prev.Layout == "content" && cur.Layout == "content" && prev.Variant != "" && prev.Variant == cur.Variant {
			t.Fatalf("adjacent content slides %d and %d reuse variant %q:\n%s", idx, idx+1, cur.Variant, string(previewJSON))
		}
	}
}

func TestBuildPPTXFromJSON_ReducesRenderedBulletVariantRepetition(t *testing.T) {
	slides := reduceAdjacentVariantRepetition([]officegen.Slide{
		{Title: "Operating Model", Layout: "content", Variant: "bullets", Points: []string{"Clarify owners", "Ship in phases", "Measure quality"}},
		{Title: "Delivery Model", Layout: "content", Variant: "bullets-plain", Points: []string{"Separate semantic spec", "Control rendering", "Review output"}},
	})
	if len(slides) != 2 {
		t.Fatalf("slides = %#v", slides)
	}
	if renderedVariantRhythmKey(slides[0]) == renderedVariantRhythmKey(slides[1]) {
		t.Fatalf("render-equivalent bullet variants should be diversified: %#v", slides)
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
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "It should be split") {
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
		{name: "board review", text: "Q3 Board Review", archetype: pptxArchetypeOps, want: officegen.StylePresetReviewCopper},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := suggestStylePreset("", tc.archetype, tc.text); got != tc.want {
				t.Fatalf("suggestStylePreset(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestDetectPPTXArchetype_RecognizesBoardReview(t *testing.T) {
	if got := detectPPTXArchetype("Create a 6-8 slide board-review deck for OfficeCLI.", "Q3 Board Review"); got != pptxArchetypeOps {
		t.Fatalf("detectPPTXArchetype(board review) = %q, want %q", got, pptxArchetypeOps)
	}
}

func TestBuildPPTXFromJSON_ProjectPlanUsesStructuredLaunchArc(t *testing.T) {
	content := `{
		"title":"Cross-Functional Launch Plan",
		"slides":[
			{"role":"cover","layout":"title","headline":"Cross-Functional Launch Plan","takeaway":"A clear operating model will align teams and reduce launch risk"},
			{"role":"summary","layout":"content","headline":"Executive Summary","takeaway":"The launch team should align on four measurable goals.","blocks":[{"type":"sections","sections":[
				{"heading":"Goals","detail":"Lock scope, hit launch date, achieve readiness across GTM and support, and stabilize quality before release"},
				{"heading":"Operating Model","detail":"Use weekly decision forums."},
				{"heading":"Decision Need","detail":"Approve owners."}
			]}]},
			{"role":"action","layout":"timeline","headline":"Milestones and Decision Gates","takeaway":"A milestone-driven path creates clear handoffs.","blocks":[{"type":"timeline","sections":[
				{"heading":"T-8 to T-6 Weeks","detail":"Finalize goals, scope, owners, and timeline."},
				{"heading":"T-5 to T-3 Weeks","detail":"Complete QA, messaging, enablement, and support prep."}
			]}]}
		]
	}`

	fileBytes, _, _, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "OfficeCLI New Release Plan", "", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if got := countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); got != 7 {
		t.Fatalf("slide count = %d, want 7", got)
	}
	for _, needle := range []string{"Decision Snapshot: GO", "Gate Scorecard: Proceed", "Workstream Ownership", "Milestone Gates", "Risk Controls: Covered", "Decision Request: Approve GO"} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", needle) {
			t.Fatalf("project deck missing %q", needle)
		}
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "Executive Summary") {
		t.Fatalf("project deck should replace repeated executive-summary fragments")
	}
	preview := string(previewJSON)
	for _, needle := range []string{`"stylePreset": "tech-contrast"`, `"variant": "comparison-spotlight"`, `"variant": "kpi-band"`, `"variant": "comparison-columns"`, `"variant": "timeline-steps"`, `"variant": "closing-decision-banner"`} {
		if !strings.Contains(preview, needle) {
			t.Fatalf("preview json missing %q:\n%s", needle, preview)
		}
	}
	if got := strings.Count(preview, `"variant": "sections-grid`); got > 1 {
		t.Fatalf("project deck repeats sections-grid variants %d times; preview:\n%s", got, preview)
	}
	if got := strings.Count(strings.ToLower(preview), "green-light"); got > 1 {
		t.Fatalf("project deck repeats green-light decision language %d times; preview:\n%s", got, preview)
	}
	for idx, bad := range map[int][]string{
		2: {"Hold if launch quality."},
		3: {"Launch risk is green only."},
		4: {"Quality or enablement gaps would create.", "A blocker needs a DRI, mitigation plan."},
	} {
		slideXML := readZipEntry(t, fileBytes, fmt.Sprintf("ppt/slides/slide%d.xml", idx))
		for _, fragment := range bad {
			if strings.Contains(slideXML, fragment) {
				t.Fatalf("slide %d renders incomplete fragment %q:\n%s", idx, fragment, slideXML)
			}
		}
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
	if payload.Theme == nil || payload.Theme.AccentColor != "2563EB" || payload.Theme.BgColor1 != "F6F8FB" || payload.Theme.TextColor != "1E293B" {
		t.Fatalf("theme = %+v, want collaboration-slate preset theme", payload.Theme)
	}
}

func TestNormalizePPTXPayload_CompactsSlideTextDensity(t *testing.T) {
	payload := &pptxPayload{
		Title:       "生成方案升级",
		StylePreset: "tech-contrast",
		Slides: []officegen.Slide{
			{Title: "生成方案升级", Layout: "title", Subtitle: "管理层摘要"},
			{
				Title:         "落地路径：先立中间层，再逐步替换直出链路",
				Layout:        "timeline",
				Variant:       "timeline",
				NarrativeRole: "action",
				Subtitle:      "建议用三阶段实施，先把质量闸门建起来，再扩模板与场景，避免一次性重构风险过高。",
				Sections: []officegen.SlideSection{
					{Heading: "阶段一：建立 Spec 与校验基线（4周）", Detail: "Owner：平台工程 + 产品；产出统一 JSON schema、失败类型字典、基础对比度与溢出校验；验收：标准 8 页汇报无人工重排。"},
					{Heading: "阶段二：接入受控设计系统（4-6周）", Detail: "Owner：文档生成工程；把主题、颜色、字号和布局槽位全部收敛到 renderer，禁止模型输出低层样式。"},
					{Heading: "阶段三：扩大模板覆盖（持续迭代）", Detail: "Owner：产品与质量评估；按业务汇报、培训、市场分析等场景扩展布局，同时沉淀可量化质量指标。"},
				},
			},
		},
	}

	normalizePPTXPayload(payload, "生成方案升级", "", false)
	if len(payload.Slides) < 2 {
		t.Fatalf("slides = %#v", payload.Slides)
	}
	for idx, slide := range payload.Slides {
		total := utf8.RuneCountInString(slide.Title) + utf8.RuneCountInString(slide.Subtitle)
		for _, point := range slide.Points {
			total += utf8.RuneCountInString(point)
		}
		for _, section := range slide.Sections {
			total += utf8.RuneCountInString(section.Heading) + utf8.RuneCountInString(section.Detail)
		}
		if total > 240 {
			t.Fatalf("slide %d text density = %d, want <= 240: %+v", idx+1, total, slide)
		}
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
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got != 3 {
		t.Fatalf("image count = %d, want 3", got)
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

func TestBuildPPTXFromJSON_ImageFailureWarningIncludesReason(t *testing.T) {
	llm := &fakeLLMClient{
		imageErr: errors.New("llm request failed: status=429 body=upstream saturated"),
	}
	content := `{
		"title":"Product Launch",
		"slides":[
			{"title":"Product Launch","layout":"title","subtitle":"Go-to-market overview","hasImage":true,"imagePrompt":"A polished product launch visual","imagePos":"background"},
			{"title":"Market Signal","layout":"content","points":["Demand is rising","Pipeline is qualified","Launch window is clear"]}
		]
	}`

	_, _, warnings, _, _, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "Product Launch", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("warnings = %#v, want image degradation warning", warnings)
	}
	if got := warnings[len(warnings)-1].Message; !strings.Contains(got, "status=429") || !strings.Contains(got, "upstream saturated") {
		t.Fatalf("warning should include image failure reason, got: %q", got)
	}
}

func TestBuildPPTXFromJSON_PremiumImagePromptAndCoverUseSafeLayout(t *testing.T) {
	balance := 9
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png", CreditBalance: &balance},
	}
	content := `{
		"title":"Product Launch",
		"slides":[
			{"title":"Product Launch","layout":"title","subtitle":"Go-to-market overview","hasImage":true,"imagePrompt":"A polished product launch poster with dashboard text","imagePos":"background"},
			{"title":"Market Signal","layout":"content","points":["Demand is rising","Pipeline is qualified","Launch window is clear"],"hasImage":true,"imagePrompt":"A market dashboard visual","imagePos":"right"},
			{"title":"Decision","layout":"closing","narrativeRole":"closing","sections":[{"heading":"Ask","detail":"Approve the launch window"}],"hasImage":true,"imagePrompt":"A bright closing background","imagePos":"background"}
		]
	}`

	fileBytes, _, warnings, _, previewJSON, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, nil, content, "Product Launch", "", true, true, PPTXBuildOptions{
		ImageQuality: "premium",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if llm.imageCalls == 0 {
		t.Fatalf("premium build should request at least one image")
	}
	if got := llm.lastImageRequest.Prompt; !strings.Contains(got, "no text, no letters, no words, no UI labels, no charts with labels, no typography") {
		t.Fatalf("premium image prompt missing no-text constraints: %q", got)
	}
	if !containsIssueCode(warnings, "INFO_PPT_HOSTED_IMAGE_CREDITS") {
		t.Fatalf("warnings should expose premium image credit balance: %#v", warnings)
	}
	var preview struct {
		Slides []officegen.Slide `json:"slides"`
	}
	if err := json.Unmarshal(previewJSON, &preview); err != nil {
		t.Fatalf("unmarshal preview json: %v\n%s", err, string(previewJSON))
	}
	if len(preview.Slides) < 2 {
		t.Fatalf("preview slides = %#v", preview.Slides)
	}
	cover := preview.Slides[0]
	if cover.ImagePos == "background" {
		t.Fatalf("premium cover image should not be full-slide background: %+v", cover)
	}
	slideXML := readZipEntry(t, fileBytes, "ppt/slides/slide1.xml")
	if !strings.Contains(slideXML, `name="TitleSideImage"`) || strings.Contains(slideXML, `name="BackgroundImage"`) {
		t.Fatalf("premium cover should render a side image, not a background image:\n%s", slideXML)
	}
	closing := preview.Slides[len(preview.Slides)-1]
	if closing.HasImage || strings.TrimSpace(closing.ImagePrompt) != "" || strings.TrimSpace(closing.ImagePos) != "" {
		t.Fatalf("premium closing slide should not keep images: %+v", closing)
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

func TestBuildPPTXFromJSON_EmitsStartAndReadyPerImage(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	collector := &runtimeProgressCollector{}
	content := `{
		"title":"Visual Gallery Demo",
		"slides":[
			{"title":"Visual Gallery Demo","layout":"title","variant":"title-center","subtitle":"Open with the topic"},
			{"title":"Product Scenes","layout":"gallery","variant":"gallery","narrativeRole":"analysis","sectionIndex":1,"sectionTitle":"Core Storyline","subtitle":"Use visuals to show the product context","visuals":[
				{"label":"Workspace","prompt":"A modern collaboration workspace","caption":"Workspace view"},
				{"label":"Meeting","prompt":"A product review meeting","caption":"Review scene"},
				{"label":"Field","prompt":"A field deployment scene","caption":"Field scene"}
			]}
		]
	}`

	_, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), llm, collector, content, "Visual Gallery Demo", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}

	starts := 0
	readys := 0
	for _, event := range collector.events {
		if event.Step != progressStepAssemble {
			continue
		}
		if strings.HasPrefix(event.Content, "Generating image asset (") {
			starts++
		}
		if strings.HasPrefix(event.Content, "Image asset ") && strings.HasSuffix(event.Content, " ready") {
			readys++
		}
	}
	if starts < 3 {
		t.Fatalf("expected >=3 'Generating image asset' events, got %d (events=%+v)", starts, collector.events)
	}
	if readys != starts {
		t.Fatalf("expected one 'ready' per start; starts=%d readys=%d (events=%+v)", starts, readys, collector.events)
	}
	if llm.imageCalls != starts {
		t.Fatalf("expected imageCalls (%d) to equal start events (%d)", llm.imageCalls, starts)
	}
}

func TestBuildPPTXFromJSON_EmitsFailedPerAssetWhenImageProviderErrors(t *testing.T) {
	llm := &fakeLLMClient{
		imageErr: errors.New("provider unreachable"),
	}
	collector := &runtimeProgressCollector{}
	content := `{
		"title":"Visual Gallery Demo",
		"slides":[
			{"title":"Visual Gallery Demo","layout":"title","variant":"title-center","subtitle":"Open"},
			{"title":"Scene","layout":"gallery","variant":"gallery","narrativeRole":"analysis","sectionIndex":1,"sectionTitle":"Core","subtitle":"caption","visuals":[
				{"label":"Workspace","prompt":"A workspace","caption":"Workspace view"}
			]}
		]
	}`

	_, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), llm, collector, content, "Visual Gallery Demo", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}

	failed := 0
	for _, event := range collector.events {
		if strings.HasPrefix(event.Content, "Image asset ") && strings.Contains(event.Content, " failed: ") {
			failed++
		}
	}
	if failed == 0 {
		t.Fatalf("expected at least one 'failed' progress event, got events=%+v", collector.events)
	}
}

func TestBuildPPTXFromJSON_EmitsFinalizingBeforeAssemblyCompleted(t *testing.T) {
	collector := &runtimeProgressCollector{}
	content := `{
		"title":"Text Only Deck",
		"slides":[
			{"title":"Text Only Deck","layout":"title","variant":"title-center","subtitle":"Cover"},
			{"title":"Body","layout":"text","variant":"bullets","subtitle":"Points","points":["alpha","beta"]}
		]
	}`

	_, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, collector, content, "Text Only Deck", "", false, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}

	var finalizingIdx, completedIdx int = -1, -1
	for i, event := range collector.events {
		if strings.Contains(event.Content, "Finalizing PPTX layout") {
			finalizingIdx = i
		}
		if strings.Contains(event.Content, "PPTX assembly completed") {
			completedIdx = i
		}
	}
	if finalizingIdx < 0 {
		t.Fatalf("expected 'Finalizing PPTX layout' event, got events=%+v", collector.events)
	}
	if completedIdx < 0 {
		t.Fatalf("expected 'PPTX assembly completed' event, got events=%+v", collector.events)
	}
	if finalizingIdx >= completedIdx {
		t.Fatalf("finalizing event should come before completed, got finalizing=%d completed=%d", finalizingIdx, completedIdx)
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
