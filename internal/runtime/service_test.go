package runtime

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
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
		jsonResponse: `{"title":"企业协作平台介绍","sections":[{"heading":"产品概述","level":1,"paragraphs":["这是一款面向企业的协作平台产品。"]}]}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeDOCX,
		Prompt:       "介绍这款企业协作平台",
		Topic:        "企业协作平台介绍",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["word/document.xml"], "企业协作平台") {
		t.Fatalf("document xml = %q", contentXMLs["word/document.xml"])
	}
}

func TestServiceGeneratePPTXWithFakeLLM(t *testing.T) {
	service := NewService(&fakeLLMClient{
		jsonResponse: `{
			"title":"企业协作平台介绍",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"企业协作平台介绍","layout":"title","subtitle":"产品和企业状况","isTitle":true},
				{"title":"产品能力","layout":"content","points":["多人协作","实时编辑","企业管理"]}
			]
		}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
		Topic:        "企业协作平台介绍",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypePPTX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["ppt/slides/slide1.xml"], "企业协作平台介绍") {
		t.Fatalf("slide xml = %q", contentXMLs["ppt/slides/slide1.xml"])
	}
}

func TestBuildPPTXPrompt_ImagesEnabledIncludesImageGuidance(t *testing.T) {
	prompt := BuildPPTXPrompt("介绍产品能力", generateengine.PromptTarget{}, true)
	for _, needle := range []string{
		`"hasImage": true`,
		`"imagePrompt": "适合直接送给图像模型的具体视觉描述"`,
		`"imagePos": "right"`,
		"优先挑 1-3 页配图",
		"chart 或 dashboard 布局不要配图",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q:\n%s", needle, prompt)
		}
	}
}

func TestBuildPPTXPrompt_ImagesDisabledForbidsImageFields(t *testing.T) {
	prompt := BuildPPTXPrompt("介绍产品能力", generateengine.PromptTarget{}, false)
	if strings.Contains(prompt, `"hasImage": true`) {
		t.Fatalf("prompt should not include image schema when disabled:\n%s", prompt)
	}
	if !strings.Contains(prompt, "不要输出 hasImage、imagePrompt、imagePos") {
		t.Fatalf("prompt should forbid image fields when disabled:\n%s", prompt)
	}
}

func TestBuildPPTXPrompt_IncludesQualityConstraints(t *testing.T) {
	prompt := BuildPPTXPrompt("介绍产品能力", generateengine.PromptTarget{
		Language: "zh-CN",
		Style:    "专业克制",
		Audience: "潜在企业客户",
	}, true)
	for _, needle := range []string{
		"总页数控制在 5-7 页",
		"第 2 页优先给出总览/关键结论",
		"subtitle 必须是一句结论",
		"标题尽量控制在 4-12 个字",
		"禁止出现省略号",
		"优先使用 sections",
		"content 页面 points 控制在 3-4 条",
		"整套里至少要有 1 页 chart 或 dashboard",
		"行动建议、落地计划、发布节奏、培训路径这类页面",
		"不要用 chart 表达优先级打分、里程碑、策略、风险、流程",
		"dashboard 指标卡最多 4 个",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q:\n%s", needle, prompt)
		}
	}
}

func TestBuildPPTXPrompt_UsesArchetypeRules(t *testing.T) {
	companyPrompt := BuildPPTXPrompt("企业协作平台介绍", generateengine.PromptTarget{}, true)
	if !strings.Contains(companyPrompt, "本主题固定按 6 页组织：1封面，2方案总览，3核心能力，4客户价值，5典型场景，6落地路径") {
		t.Fatalf("company prompt missing archetype outline:\n%s", companyPrompt)
	}
	marketPrompt := BuildPPTXPrompt("AI 办公出海市场机会分析", generateengine.PromptTarget{}, false)
	if !strings.Contains(marketPrompt, "第 3 页“市场空间”必须使用 chart") {
		t.Fatalf("market prompt missing archetype outline:\n%s", marketPrompt)
	}
	opsPrompt := BuildPPTXPrompt("SaaS 季度经营复盘", generateengine.PromptTarget{}, false)
	if !strings.Contains(opsPrompt, "本主题固定按 6 页组织：1封面，2经营结论，3核心指标，4问题定位，5下季重点，6执行动作") {
		t.Fatalf("ops prompt missing archetype outline:\n%s", opsPrompt)
	}
}

func TestCleanSentence_PreservesTimeNumbers(t *testing.T) {
	if got := cleanSentence("30 天内完成首轮验证。"); got != "30 天内完成首轮验证" {
		t.Fatalf("cleanSentence() = %q", got)
	}
	if got := cleanSentence("1. 明确目标"); got != "明确目标" {
		t.Fatalf("cleanSentence() = %q", got)
	}
}

func TestFitTextForLayout_PrefersWholeClause(t *testing.T) {
	got := fitTextForLayout("建议以东南亚为首站验证 PMF，再以欧洲做高客单扩张", 18)
	if got != "建议以东南亚为首站验证 PMF" {
		t.Fatalf("fitTextForLayout() = %q", got)
	}
	if strings.Contains(got, "...") {
		t.Fatalf("fitTextForLayout() should avoid ellipsis: %q", got)
	}
}

func TestNormalizeActionSlide_ConvertsPointsToSections(t *testing.T) {
	slide := normalizeActionSlide(officegen.Slide{
		Title: "进入建议",
		Points: []string{
			"30 天内由产品负责人确定主场景并完成目标客户访谈",
			"60 天内由渠道负责人签下伙伴并验证线索成本",
		},
	})
	if len(slide.Sections) != 2 {
		t.Fatalf("sections = %#v", slide.Sections)
	}
	if slide.Sections[0].Heading != "30 天内" {
		t.Fatalf("first heading = %q", slide.Sections[0].Heading)
	}
	if len(slide.Points) != 0 {
		t.Fatalf("points should be cleared after section normalization: %#v", slide.Points)
	}
}

func TestNormalizeEvidenceSlide_PromotesValueAndMarketSlides(t *testing.T) {
	valueSlide := normalizeEvidenceSlide(officegen.Slide{Title: "客户价值"})
	if valueSlide.Layout != "dashboard" || len(valueSlide.Metrics) == 0 {
		t.Fatalf("value slide = %#v", valueSlide)
	}
	marketSlide := normalizeEvidenceSlide(officegen.Slide{Title: "市场空间"})
	if marketSlide.Layout != "chart" || marketSlide.Chart == nil {
		t.Fatalf("market slide = %#v", marketSlide)
	}
	if marketSlide.Source == "" {
		t.Fatalf("market slide should inject source hint")
	}
}

func TestNormalizePPTXPayload_EnforcesCompanySkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "企业协作平台介绍",
		Slides: []officegen.Slide{
			{Title: "企业协作平台介绍", Layout: "title", IsTitle: true},
			{Title: "第一页"},
			{Title: "第二页"},
		},
	}
	normalizePPTXPayload(payload, "企业协作平台介绍", true)
	if len(payload.Slides) != 6 {
		t.Fatalf("slide count = %d, want 6", len(payload.Slides))
	}
	if payload.Slides[3].Layout != "content" || len(payload.Slides[3].Sections) != 3 {
		t.Fatalf("company value slide = %#v", payload.Slides[3])
	}
	if payload.Slides[5].Title != "落地路径" || len(payload.Slides[5].Sections) == 0 {
		t.Fatalf("company closing slide = %#v", payload.Slides[5])
	}
}

func TestNormalizePPTXPayload_EnforcesMarketSkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "AI 办公出海市场机会分析",
		Slides: []officegen.Slide{
			{Title: "AI 办公出海市场机会分析", Layout: "title", IsTitle: true},
			{Title: "第一页"},
			{Title: "第二页"},
		},
	}
	normalizePPTXPayload(payload, "AI 办公出海市场机会分析", true)
	if len(payload.Slides) != 6 {
		t.Fatalf("slide count = %d, want 6", len(payload.Slides))
	}
	if payload.Slides[2].Layout != "chart" || payload.Slides[2].Chart == nil {
		t.Fatalf("market chart slide = %#v", payload.Slides[2])
	}
	if payload.Slides[4].Title != "竞争格局" || len(payload.Slides[4].Points) == 0 {
		t.Fatalf("market competition slide = %#v", payload.Slides[4])
	}
}

func TestNormalizePPTXPayload_EnforcesOpsSkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "SaaS 季度经营复盘",
		Slides: []officegen.Slide{
			{Title: "SaaS 季度经营复盘", Layout: "title", IsTitle: true},
			{Title: "第一页"},
			{Title: "第二页"},
		},
	}
	normalizePPTXPayload(payload, "SaaS 季度经营复盘", true)
	if len(payload.Slides) != 6 {
		t.Fatalf("slide count = %d, want 6", len(payload.Slides))
	}
	if payload.Slides[2].Layout != "chart" || payload.Slides[2].Chart == nil {
		t.Fatalf("ops chart slide = %#v", payload.Slides[2])
	}
	if payload.Slides[5].Title != "执行动作" || len(payload.Slides[5].Sections) != 3 {
		t.Fatalf("ops closing slide = %#v", payload.Slides[5])
	}
}

func TestServiceGeneratePPTX_GeneratesImagesWhenEnabled(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"产品能力介绍",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"封面","layout":"title","subtitle":"产品和企业状况","isTitle":true},
				{"title":"产品能力","layout":"content","points":["多人协作","实时编辑","企业管理"],"hasImage":true,"imagePrompt":"现代协作办公场景，明亮会议室，多人围绕大屏讨论文档","imagePos":"right"}
			]
		}`,
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "介绍这款知识协作产品的产品能力、客户价值与应用场景",
		Topic:        "知识协作产品介绍",
		Mode:         "fast",
		EnableImages: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.imageCalls != 1 {
		t.Fatalf("imageCalls = %d, want 1", llm.imageCalls)
	}
	rels := readZipEntry(t, doc.Bytes, "ppt/slides/_rels/slide2.xml.rels")
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
			"title":"产品能力介绍",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"封面","layout":"title","subtitle":"产品和企业状况","isTitle":true},
				{"title":"产品能力","layout":"content","points":["多人协作","实时编辑","企业管理"],"hasImage":true,"imagePrompt":"现代协作办公场景，明亮会议室，多人围绕大屏讨论文档","imagePos":"right"}
			]
		}`,
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "介绍这款知识协作产品的产品能力、客户价值与应用场景",
		Topic:        "知识协作产品介绍",
		Mode:         "fast",
		EnableImages: false,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.imageCalls != 0 {
		t.Fatalf("imageCalls = %d, want 0", llm.imageCalls)
	}
	rels := readZipEntry(t, doc.Bytes, "ppt/slides/_rels/slide2.xml.rels")
	if strings.Contains(rels, `relationships/image`) {
		t.Fatalf("slide rels should not include image relationship when disabled: %s", rels)
	}
}

func TestServiceGeneratePPTX_DegradesGracefullyWhenImageGenerationFails(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"产品能力介绍",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"封面","layout":"title","subtitle":"产品和企业状况","isTitle":true},
				{"title":"产品能力","layout":"content","points":["多人协作","实时编辑","企业管理"],"hasImage":true,"imagePrompt":"现代协作办公场景，明亮会议室，多人围绕大屏讨论文档","imagePos":"right"}
			]
		}`,
		imageErr: errors.New("image backend unavailable"),
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "介绍这款知识协作产品的产品能力、客户价值与应用场景",
		Topic:        "知识协作产品介绍",
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
	if got := doc.Warnings[0].Message; !strings.Contains(got, "已自动降级为无图版本") {
		t.Fatalf("warning = %q", got)
	}
	rels := readZipEntry(t, doc.Bytes, "ppt/slides/_rels/slide2.xml.rels")
	if strings.Contains(rels, `relationships/image`) {
		t.Fatalf("slide rels should not include image relationship after degradation: %s", rels)
	}
}

func TestBuildPPTXFromJSON_NormalizesQualityConstraints(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"季度总结",
		"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
		"slides":[
			{"title":"超长超长超长超长超长第一页标题需要被收敛","layout":"content","points":["第一条要点非常非常长，需要被自动截断以控制版面密度","第二条要点也特别长，需要被处理","第三条要点","第四条要点","第五条要点"],"hasImage":true,"imagePrompt":"一张复杂的市场分析海报，包含很多元素","imagePos":"background"},
			{"title":"第二页","layout":"content","points":["结论一","结论二","结论三"],"hasImage":true,"imagePrompt":"海外办公场景","imagePos":"left"},
			{"title":"第三页","layout":"content","points":["结论一","结论二","结论三"],"hasImage":true,"imagePrompt":"团队会议","imagePos":"right"},
			{"title":"第四页","layout":"content","content":"这是很长的一段内容。需要拆成多个分点。第二句继续补充。第三句继续说明。"},
			{"title":"第五页","layout":"dashboard","metrics":[{"label":"ARR","value":"820 万","note":"同比 +32%"},{"label":"NDR","value":"118%","note":"续费改善"}]},
			{"title":"第六页","layout":"chart","chart":{"title":"区域收入","type":"bar","categories":["北美","欧洲","东南亚","中东","日本","韩国"],"values":[42,31,28,17,12,9]}},
			{"title":"第七页","layout":"content","points":["结论一","结论二","结论三"]},
			{"title":"第八页","layout":"content","points":["这页应该被裁掉"]}		
		]
	}`

	fileBytes, _, warnings, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "季度总结", true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if got := countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); got != 7 {
		t.Fatalf("slide count = %d, want 7", got)
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got != 0 {
		t.Fatalf("image count = %d, want 0 after image rebalancing", got)
	}
	slide1 := readZipEntry(t, fileBytes, "ppt/slides/slide1.xml")
	if strings.Contains(slide1, "●") {
		t.Fatalf("title slide should not render bullet content: %s", slide1)
	}
	slide4 := readZipEntry(t, fileBytes, "ppt/slides/slide4.xml")
	if !strings.Contains(slide4, "需要拆成多个分点") {
		t.Fatalf("content slide should be normalized into readable points: %s", slide4)
	}
	slide6Rels := readZipEntry(t, fileBytes, filepath.ToSlash("ppt/slides/_rels/slide6.xml.rels"))
	if strings.Contains(slide6Rels, "image") {
		t.Fatalf("chart slide should not keep image rels: %s", slide6Rels)
	}
	if len(warnings) < 2 {
		t.Fatalf("warnings = %#v, want normalization warnings", warnings)
	}
}

func TestBuildPPTXFromJSON_DowngradesTimelineChartsToSections(t *testing.T) {
	content := `{
		"title":"发布节奏",
		"slides":[
			{"title":"发布节奏","layout":"title","subtitle":"先看节点"},
			{"title":"节奏与里程碑","layout":"chart","chart":{"title":"版本阶段安排","type":"bar","categories":["需求冻结","开发联调","测试灰度","正式发布"],"values":[1,3,2,2]}}
		]
	}`

	fileBytes, _, _, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "发布节奏", false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if got := countZipEntries(fileBytes, "ppt/charts/", ".xml"); got != 0 {
		t.Fatalf("chart count = %d, want 0", got)
	}
	slide2 := readZipEntry(t, fileBytes, "ppt/slides/slide2.xml")
	for _, needle := range []string{"需求冻结", "开发联调", "第1阶段，依次推进"} {
		if !strings.Contains(slide2, needle) {
			t.Fatalf("slide2 missing %q:\n%s", needle, slide2)
		}
	}
}

func TestServiceGeneratePPTX_RetriesOnceWhenJSONIsTruncated(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponses: []string{
			`{"title":"知识协作产品介绍","slides":[{"title":"封面","layout":"title","subtitle":"一句话结论","isTitle":true}`,
		},
		structuredResponse: `{
			"title":"知识协作产品介绍",
			"slides":[
				{"title":"封面","layout":"title","subtitle":"一句话结论","isTitle":true},
				{"title":"产品能力","layout":"content","points":["协作效率提升","权限治理清晰","落地路径明确"]}
			]
		}`,
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "介绍这款知识协作产品的产品能力、客户价值与应用场景",
		Topic:        "知识协作产品介绍",
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
	slide2 := readZipEntry(t, doc.Bytes, "ppt/slides/slide2.xml")
	if !strings.Contains(slide2, "产品能力") {
		t.Fatalf("slide2 = %s", slide2)
	}
}

func TestServiceGenerateDOCXEmitsProgressEvents(t *testing.T) {
	collector := &runtimeProgressCollector{}
	service := NewService(&fakeLLMClient{
		jsonResponse: `{"title":"企业协作平台介绍","sections":[{"heading":"产品概述","level":1,"paragraphs":["这是一款面向企业的协作平台产品。"]}]}`,
	}, collector)

	_, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeDOCX,
		Prompt:       "介绍这款企业协作平台",
		Topic:        "企业协作平台介绍",
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
		"generate_llm:running:正在调用 LLM 生成 docx 内容",
		"generate_llm:completed:已收到 docx 结构结果",
		"assemble:running:正在组装 docx 文件",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("progress output missing %q:\n%s", needle, output)
		}
	}
}
