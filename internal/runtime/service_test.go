package runtime

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
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

func (c *runtimeProgressCollector) Emit(_ context.Context, event engine.ProgressEvent) {
	c.events = append(c.events, event)
}

func TestServiceGenerateDOCXWithFakeLLM(t *testing.T) {
	service := NewService(&fakeLLMClient{
		jsonResponse: `{
			"title":"Enterprise Collaboration Platform Overview",
			"subtitle":"Board-level platform brief",
			"blocks":[
				{"type":"heading","level":1,"text":"Product Overview"},
				{"type":"paragraph","text":"This collaboration platform is designed for enterprise teams."},
				{"type":"callout","title":"Why it matters","text":"It centralizes workflows across departments."}
			]
		}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeDOCX,
		Prompt:       "Introduce this enterprise collaboration platform",
		Topic:        "Enterprise Collaboration Platform Overview",
		Mode:         "fast",
		LocalPreview: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(doc.PreviewHTML) == 0 || len(doc.PreviewJSON) == 0 {
		t.Fatalf("expected docx preview sidecars")
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["word/document.xml"], "Enterprise Collaboration Platform") ||
		!strings.Contains(contentXMLs["word/document.xml"], "Why it matters") {
		t.Fatalf("document xml = %q", contentXMLs["word/document.xml"])
	}
}

func TestServiceGenerateXLSXWithFakeLLM(t *testing.T) {
	service := NewService(&fakeLLMClient{
		jsonResponse: `{
			"title":"Sales Workbook",
			"subtitle":"Regional pipeline summary",
			"sheets":[
				{
					"name":"Pipeline",
					"purpose":"Track commercial performance by region",
					"summary":[{"label":"Top Region","value":"West"}],
					"columns":[
						{"label":"Region","type":"string"},
						{"label":"Amount","type":"currency"}
					],
					"rows":[["East","100"],["West","120"]],
					"showTotals":true
				}
			]
		}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeXLSX,
		Prompt:       "Create a regional sales workbook",
		Topic:        "Sales Workbook",
		Mode:         "fast",
		LocalPreview: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if doc.DocumentName != "Sales_Workbook.xlsx" {
		t.Fatalf("document name = %q", doc.DocumentName)
	}
	if len(doc.PreviewHTML) == 0 || len(doc.PreviewJSON) == 0 {
		t.Fatalf("expected xlsx preview sidecars")
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["xl/workbook.xml"], "Pipeline") ||
		!strings.Contains(contentXMLs["xl/sharedStrings.xml"], "East") ||
		!strings.Contains(contentXMLs["xl/worksheets/sheet1.xml"], "<f>SUBTOTAL") {
		t.Fatalf("workbook xml = %q\nshared strings = %q\nsheet1 = %q", contentXMLs["xl/workbook.xml"], contentXMLs["xl/sharedStrings.xml"], contentXMLs["xl/worksheets/sheet1.xml"])
	}
}

func TestServiceGeneratePPTXWithFakeLLM(t *testing.T) {
	service := NewService(&fakeLLMClient{
		jsonResponse: `{
			"title":"Enterprise Collaboration Platform Overview",
			"subtitle":"Product context and business status",
			"stylePreset":"executive-dark",
			"theme":{"preset":"executive","primaryColor":"0F172A","accentColor":"F97316","accentSoft":"FFEDD5","backgroundColor":"F8FAFC","surfaceColor":"FFFFFF","borderColor":"CBD5E1","textColor":"1E293B","mutedColor":"64748B","titleColor":"020617","fontFamily":"Aptos","eaFontFamily":"Microsoft YaHei"},
			"slides":[
				{"role":"cover","layout":"title","headline":"Enterprise Collaboration Platform Overview","takeaway":"Product context and business status","blocks":[],"visual":null,"source":"","bgColor":"","bgColor2":""},
				{"role":"detail","layout":"content","headline":"Product Capabilities","takeaway":"Show how the platform improves team execution","blocks":[{"type":"bullets","text":"","items":["Multi-user collaboration","Real-time editing","Enterprise administration"],"sections":[],"metrics":[],"chart":null}],"visual":null,"source":"","bgColor":"","bgColor2":""}
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

func TestBuildPPTXFromJSON_AcceptsSemanticDeckSpec(t *testing.T) {
	content := `{
		"title":"Semantic Deck Test",
		"subtitle":"A deck-level framing sentence",
		"stylePreset":"tech-contrast",
		"theme":{"preset":"analysis","primaryColor":"1D4ED8","accentColor":"0F766E","accentSoft":"D1FAE5","backgroundColor":"F8FAFC","surfaceColor":"FFFFFF","borderColor":"DCE4F2","textColor":"0F172A","mutedColor":"64748B","titleColor":"020617","fontFamily":"Aptos","eaFontFamily":"Microsoft YaHei"},
		"slides":[
			{"role":"cover","layout":"title","headline":"Semantic Deck Test","takeaway":"Start with the framing","blocks":[],"visual":null,"source":"","bgColor":"","bgColor2":""},
			{"role":"summary","layout":"content","headline":"Key Takeaways","takeaway":"Lead with the conclusion","blocks":[{"type":"sections","text":"","items":[],"sections":[{"heading":"Signal","detail":"Demand is consolidating around enterprise workflows"},{"heading":"Impact","detail":"The platform story must emphasize control and rollout"},{"heading":"Decision","detail":"Move with a pilot-first expansion motion"}],"metrics":[],"chart":null}],"visual":null,"source":"","bgColor":"","bgColor2":""}
		]
	}`

	fileBytes, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Semantic Deck Test", "", false, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	slide2 := readZipEntry(t, fileBytes, "ppt/slides/slide2.xml")
	if !strings.Contains(slide2, "Key Takeaways") || !strings.Contains(slide2, "Demand is consolidating") {
		t.Fatalf("semantic slide missing expected content:\n%s", slide2)
	}
}

func TestBuildPPTXFromJSON_AcceptsLegacySlideSpec(t *testing.T) {
	content := `{
		"title":"Legacy Deck Test",
		"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
		"slides":[
			{"title":"Legacy Deck Test","layout":"title","variant":"title-center","subtitle":"Start with the framing","isTitle":true},
			{"title":"Core Capabilities","layout":"content","variant":"bullets","subtitle":"Explain the operating model","points":["Workflow orchestration","Departmental collaboration","Governance and auditability"],"hasImage":false,"imagePrompt":"","imagePos":"","source":"Internal product brief"}
		]
	}`

	fileBytes, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Legacy Deck Test", "", false, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	slide2 := readZipEntry(t, fileBytes, "ppt/slides/slide2.xml")
	for _, needle := range []string{"Explain the operating model", "Workflow orchestration", "Departmental collaborati", "Governance and auditabil", "Internal product brief"} {
		if !strings.Contains(slide2, needle) {
			t.Fatalf("legacy slide missing %q:\n%s", needle, slide2)
		}
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
		`"kind": "image"`,
		`"prompt": "A concrete visual prompt that can be sent directly to an image model"`,
		`"position": "right"`,
		"Use images sparingly. Prefer 0-1 image slide",
		"Do not add images to chart or dashboard layouts",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q:\n%s", needle, prompt)
		}
	}
}

func TestBuildPPTXPrompt_ImagesDisabledForbidsImageFields(t *testing.T) {
	prompt := BuildPPTXPrompt("Introduce product capabilities", generateengine.PromptTarget{}, false)
	if strings.Contains(prompt, `"kind": "image"`) {
		t.Fatalf("prompt should not include image schema when disabled:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Set visual to null when the slide does not need an image.") {
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
		"Keep the deck to 5-8 slides, usually 6-7.",
		"slide 2 should read as an executive summary or key takeaways page",
		"takeaway must be a takeaway sentence",
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
	if !strings.Contains(companyPrompt, "a strong storyline is usually cover -> solution overview -> core capabilities -> customer value -> use cases -> rollout path") {
		t.Fatalf("company prompt missing archetype outline:\n%s", companyPrompt)
	}
	marketPrompt := BuildPPTXPrompt("market opportunity analysis", generateengine.PromptTarget{}, false)
	if !strings.Contains(marketPrompt, "Slide 3 must use a chart and include a source.") {
		t.Fatalf("market prompt missing archetype outline:\n%s", marketPrompt)
	}
	opsPrompt := BuildPPTXPrompt("business review", generateengine.PromptTarget{}, false)
	if !strings.Contains(opsPrompt, "a strong storyline is usually cover -> business takeaways -> core metrics -> issue diagnosis -> next-quarter priorities -> execution actions") {
		t.Fatalf("ops prompt missing archetype outline:\n%s", opsPrompt)
	}
	trainingPrompt := BuildPPTXPrompt("new hire onboarding training", generateengine.PromptTarget{}, false)
	if !strings.Contains(trainingPrompt, "a strong storyline is usually cover -> learning goals -> installation and setup -> common commands -> example workflow -> cautions") {
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

func TestNormalizeSlideVariant_PrefersComparisonAndTimeline(t *testing.T) {
	comparison := normalizeSlideVariant(officegen.Slide{
		Role:   string(pptxSlideRoleDetail),
		Title:  "Before vs After",
		Layout: "content",
		Sections: []officegen.SlideSection{
			{Heading: "Before", Detail: "Manual handoffs slow response time."},
			{Heading: "After", Detail: "One workflow keeps owners and status visible."},
		},
	})
	if comparison != "comparison" {
		t.Fatalf("comparison variant = %q", comparison)
	}

	timeline := normalizeSlideVariant(officegen.Slide{
		Role:   string(pptxSlideRoleAction),
		Title:  "Rollout Path",
		Layout: "content",
		Sections: []officegen.SlideSection{
			{Heading: "Week 1", Detail: "Lock the pilot scope."},
			{Heading: "Week 2", Detail: "Train the first owner set."},
			{Heading: "Week 4", Detail: "Review adoption and expand."},
			{Heading: "Week 8", Detail: "Decide broader rollout."},
		},
	})
	if timeline != "timeline" {
		t.Fatalf("timeline variant = %q", timeline)
	}
}

func TestExpandSlideForDensity_PreservesTimelineCandidate(t *testing.T) {
	slide := officegen.Slide{
		Role:   string(pptxSlideRoleAction),
		Title:  "Execution Actions",
		Layout: "content",
		Sections: []officegen.SlideSection{
			{Heading: "Week 1", Detail: "Confirm scope."},
			{Heading: "Week 2", Detail: "Train admins."},
			{Heading: "Week 4", Detail: "Launch pilot."},
			{Heading: "Week 8", Detail: "Review and expand."},
		},
	}

	got := expandSlideForDensity(slide)
	if len(got) != 1 {
		t.Fatalf("timeline candidate should stay on one slide: %#v", got)
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
	if len(payload.Slides) != 4 {
		t.Fatalf("slide count = %d, want 4", len(payload.Slides))
	}
	if payload.Slides[1].Title != "Key Takeaways" || len(payload.Slides[1].Sections) != 3 {
		t.Fatalf("company summary slide = %#v", payload.Slides[1])
	}
	if payload.Slides[2].Title != "Core Capabilities" || len(payload.Slides[2].Sections) != 3 {
		t.Fatalf("company capability slide = %#v", payload.Slides[2])
	}
	if payload.Slides[3].Title != "Rollout Path" || len(payload.Slides[3].Sections) == 0 {
		t.Fatalf("company closing slide = %#v", payload.Slides[3])
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
	if len(payload.Slides) != 4 {
		t.Fatalf("slide count = %d, want 4", len(payload.Slides))
	}
	if payload.Slides[2].Layout != "chart" || payload.Slides[2].Chart == nil {
		t.Fatalf("market chart slide = %#v", payload.Slides[2])
	}
	if payload.Slides[3].Title != "Entry Recommendations" || len(payload.Slides[3].Sections) == 0 {
		t.Fatalf("market closing slide = %#v", payload.Slides[3])
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
	if len(payload.Slides) != 4 {
		t.Fatalf("slide count = %d, want 4", len(payload.Slides))
	}
	if payload.Slides[2].Layout != "chart" || payload.Slides[2].Chart == nil {
		t.Fatalf("ops chart slide = %#v", payload.Slides[2])
	}
	if payload.Slides[3].Title != "Execution Actions" || len(payload.Slides[3].Sections) != 3 {
		t.Fatalf("ops closing slide = %#v", payload.Slides[3])
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
	if len(payload.Slides) != 4 {
		t.Fatalf("slide count = %d, want 4", len(payload.Slides))
	}
	if payload.Slides[2].Title != "Installation and Setup" || len(payload.Slides[2].Sections) != 3 {
		t.Fatalf("training setup slide = %#v", payload.Slides[2])
	}
	if payload.Slides[3].Title != "Cautions" || len(payload.Slides[3].Sections) != 3 {
		t.Fatalf("training closing slide = %#v", payload.Slides[3])
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
	rels := readZipEntry(t, doc.Bytes, "ppt/slides/_rels/slide3.xml.rels")
	if !strings.Contains(rels, `relationships/image`) {
		t.Fatalf("slide rels missing image relationship: %s", rels)
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
	rels := readZipEntry(t, doc.Bytes, "ppt/slides/_rels/slide3.xml.rels")
	if strings.Contains(rels, `relationships/image`) {
		t.Fatalf("slide rels should not include image relationship when disabled: %s", rels)
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
	rels := readZipEntry(t, doc.Bytes, "ppt/slides/_rels/slide3.xml.rels")
	if strings.Contains(rels, `relationships/image`) {
		t.Fatalf("slide rels should not include image relationship after degradation: %s", rels)
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
	if got := countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); got != 8 {
		t.Fatalf("slide count = %d, want 8", got)
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got != 0 {
		t.Fatalf("image count = %d, want 0 after image rebalancing", got)
	}
	slide1 := readZipEntry(t, fileBytes, "ppt/slides/slide1.xml")
	if strings.Contains(slide1, "●") {
		t.Fatalf("title slide should not render bullet content: %s", slide1)
	}
	slide4 := readZipEntry(t, fileBytes, "ppt/slides/slide4.xml")
	if !strings.Contains(slide4, "It should be split into multip") {
		t.Fatalf("content slide should be normalized into readable points: %s", slide4)
	}
	slide6Rels := readZipEntry(t, fileBytes, filepath.ToSlash("ppt/slides/_rels/slide6.xml.rels"))
	if strings.Contains(slide6Rels, "image") {
		t.Fatalf("chart slide should not keep image rels: %s", slide6Rels)
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
	slide2 := readZipEntry(t, fileBytes, "ppt/slides/slide2.xml")
	if !strings.Contains(slide2, "Executive Summary") {
		t.Fatalf("slide2 should be promoted to an overview slide:\n%s", slide2)
	}
	slide3 := readZipEntry(t, fileBytes, "ppt/slides/slide3.xml")
	slide4 := readZipEntry(t, fileBytes, "ppt/slides/slide4.xml")
	combined := slide3 + slide4
	for _, needle := range []string{"Cadence and Milest", "General Av"} {
		if !strings.Contains(combined, needle) {
			t.Fatalf("downgraded timeline slides missing %q:\nslide3=%s\nslide4=%s", needle, slide3, slide4)
		}
	}
	if strings.Contains(slide3, `r:id="rId1"`) {
		t.Fatalf("timeline slide should be downgraded from chart rels:\n%s", slide3)
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
	slide3 := readZipEntry(t, doc.Bytes, "ppt/slides/slide3.xml")
	if !strings.Contains(slide3, "Higher collaboration efficiency") {
		t.Fatalf("slide3 = %s", slide3)
	}
}

func TestDetectPPTXArchetype_SupportsChineseKeywords(t *testing.T) {
	if got := detectPPTXArchetype("企业协作平台介绍，面向潜在客户", ""); got != pptxArchetypeCompany {
		t.Fatalf("company archetype = %q", got)
	}
	if got := detectPPTXArchetype("市场分析与出海机会评估", ""); got != pptxArchetypeMarket {
		t.Fatalf("market archetype = %q", got)
	}
	if got := detectPPTXArchetype("季度经营分析和运营复盘", ""); got != pptxArchetypeOps {
		t.Fatalf("ops archetype = %q", got)
	}
	if got := detectPPTXArchetype("新员工培训上手指南", ""); got != pptxArchetypeTraining {
		t.Fatalf("training archetype = %q", got)
	}
}

func TestNormalizeSlideVariant_UsesFeatureGridAndStatBand(t *testing.T) {
	featureGrid := normalizeSlideVariant(officegen.Slide{
		Role:   string(pptxSlideRoleDetail),
		Title:  "核心功能",
		Layout: "content",
		Sections: []officegen.SlideSection{
			{Heading: "统一入口", Detail: "聚合消息、文档和审批"},
			{Heading: "流程协同", Detail: "自动串联任务和通知"},
			{Heading: "权限治理", Detail: "细粒度权限和审计"},
			{Heading: "知识沉淀", Detail: "模板和FAQ持续复用"},
		},
	})
	if featureGrid != "feature-grid" {
		t.Fatalf("feature grid variant = %q", featureGrid)
	}

	statBand := normalizeSlideVariant(officegen.Slide{
		Role:   string(pptxSlideRoleEvidence),
		Title:  "客户价值",
		Layout: "dashboard",
		Metrics: []officegen.MetricCard{
			{Label: "审批周期", Value: "-30%", Note: "8周试点"},
			{Label: "按时完成率", Value: "+15%", Note: "周度跟踪"},
			{Label: "知识复用率", Value: "+25%", Note: "季度评估"},
		},
	})
	if statBand != "stat-band" {
		t.Fatalf("stat band variant = %q", statBand)
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
