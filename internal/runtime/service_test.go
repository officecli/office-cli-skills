package runtime

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

type fakeLLMClient struct {
	textResponse       string
	jsonResponse       string
	structuredResponse string
	imageResult        *engine.ImageGenerationResult
	imageErr           error
	imageCalls         int
	lastImageRequest   engine.ImageGenerationRequest
}

func (f fakeLLMClient) CompleteText(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return f.textResponse, nil
}

func (f fakeLLMClient) CompleteJSON(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return f.jsonResponse, nil
}

func (f fakeLLMClient) CompleteStructured(_ context.Context, _ engine.StructuredCompletionRequest) (string, error) {
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

func TestServiceGeneratePPTX_GeneratesImagesWhenEnabled(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"企业协作平台介绍",
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
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
		Topic:        "企业协作平台介绍",
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
			"title":"企业协作平台介绍",
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
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
		Topic:        "企业协作平台介绍",
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
			"title":"企业协作平台介绍",
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
		Prompt:       "介绍这款企业协作平台的产品能力、客户价值与应用场景",
		Topic:        "企业协作平台介绍",
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
