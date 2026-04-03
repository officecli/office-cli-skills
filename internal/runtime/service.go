package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	"github.com/officecli/officecli/pkg/officegen"
)

type GenerateParams struct {
	DocumentType engine.DocumentType
	Topic        string
	Prompt       string
	Mode         string
	Language     string
	Style        string
	Audience     string
	EnableImages bool
}

type GeneratedArtifact struct {
	DocumentName string
	DocumentType string
	Bytes        []byte
	Warnings     []engine.GenerateIssue
	Errors       []engine.GenerateIssue
}

type Service struct {
	llm      engine.LLMClient
	progress engine.ProgressEmitter
}

func NewService(llm engine.LLMClient, progress any) *Service {
	service := &Service{llm: llm}
	if emitter, ok := progress.(engine.ProgressEmitter); ok {
		service.progress = emitter
	}
	return service
}

func (s *Service) Generate(ctx context.Context, params GenerateParams) (*GeneratedArtifact, error) {
	if s == nil || s.llm == nil {
		return nil, fmt.Errorf("llm client is unavailable")
	}

	envelope, meta, err := generateengine.ParsePromptEnvelope(params.Prompt)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(envelope.Prompt) == "" {
		envelope.Prompt = strings.TrimSpace(params.Topic)
	}
	target := envelope.Target
	if strings.TrimSpace(target.Language) == "" {
		target.Language = strings.TrimSpace(params.Language)
	}
	if strings.TrimSpace(target.Style) == "" {
		target.Style = strings.TrimSpace(params.Style)
	}
	if strings.TrimSpace(target.Audience) == "" {
		target.Audience = strings.TrimSpace(params.Audience)
	}

	switch params.DocumentType {
	case engine.DocumentTypeDOCX:
		return s.generateDOCX(ctx, envelope.Prompt, params.Topic, target, meta)
	case engine.DocumentTypeXLSX:
		return s.generateXLSX(ctx, envelope.Prompt, params.Topic, target, meta)
	case engine.DocumentTypePPTX:
		return s.generatePPTX(ctx, envelope.Prompt, params.Topic, target, meta, params.EnableImages)
	default:
		return nil, fmt.Errorf("unsupported document type: %s", params.DocumentType)
	}
}

func (s *Service) generateDOCX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta) (*GeneratedArtifact, error) {
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "正在调用 LLM 生成 docx 内容")
	response, err := s.llm.CompleteJSON(ctx, []engine.LLMMessage{{Role: "user", Content: generateengine.BuildDOCXPrompt(prompt, target)}})
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "docx 内容生成失败")
		return nil, fmt.Errorf("生成内容阶段失败：%w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "已收到 docx 结构结果")
	emitProgress(ctx, s.progress, progressStepAssemble, "running", "正在组装 docx 文件")
	fileBytes, fileName, err := generateengine.BuildDOCXFromJSON(response, fallbackDescription(topic, prompt))
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "docx 组装失败")
		return nil, fmt.Errorf("文档组装阶段失败：%w", err)
	}
	emitProgress(ctx, s.progress, progressStepAssemble, "completed", "docx 文件组装完成")
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypeDOCX),
		Bytes:        fileBytes,
		Warnings:     convertIssues(meta),
	}, nil
}

func (s *Service) generateXLSX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta) (*GeneratedArtifact, error) {
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "正在调用 LLM 生成 xlsx 内容")
	response, err := s.llm.CompleteJSON(ctx, []engine.LLMMessage{{Role: "user", Content: generateengine.BuildXLSXPrompt(prompt, target)}})
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "xlsx 内容生成失败")
		return nil, fmt.Errorf("生成内容阶段失败：%w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "已收到 xlsx 结构结果")
	emitProgress(ctx, s.progress, progressStepAssemble, "running", "正在组装 xlsx 文件")
	fileBytes, fileName, err := generateengine.BuildXLSXFromJSON(response, fallbackDescription(topic, prompt))
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "xlsx 组装失败")
		return nil, fmt.Errorf("文档组装阶段失败：%w", err)
	}
	emitProgress(ctx, s.progress, progressStepAssemble, "completed", "xlsx 文件组装完成")
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypeXLSX),
		Bytes:        fileBytes,
		Warnings:     convertIssues(meta),
	}, nil
}

func (s *Service) generatePPTX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta, enableImages bool) (*GeneratedArtifact, error) {
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "正在调用 LLM 生成 pptx 内容")
	response, err := s.llm.CompleteJSON(ctx, []engine.LLMMessage{{Role: "user", Content: BuildPPTXPrompt(prompt, target, enableImages)}})
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "pptx 内容生成失败")
		return nil, fmt.Errorf("生成内容阶段失败：%w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "已收到 pptx 结构结果")
	fileBytes, fileName, warnings, err := BuildPPTXFromJSON(ctx, s.llm, s.progress, response, fallbackDescription(topic, prompt), enableImages)
	if err != nil {
		return nil, err
	}
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypePPTX),
		Bytes:        fileBytes,
		Warnings:     append(convertIssues(meta), warnings...),
	}, nil
}

func convertIssues(meta *generateengine.PPTXMeta) []engine.GenerateIssue {
	if meta == nil || len(meta.Warnings) == 0 {
		return nil
	}
	out := make([]engine.GenerateIssue, 0, len(meta.Warnings))
	for _, warning := range meta.Warnings {
		out = append(out, engine.GenerateIssue{
			Code:       warning.Code,
			Message:    warning.Message,
			Field:      warning.Field,
			ReasonCode: warning.ReasonCode,
			Retryable:  warning.Retryable,
		})
	}
	return out
}

func fallbackDescription(topic, prompt string) string {
	if strings.TrimSpace(topic) != "" {
		return strings.TrimSpace(topic)
	}
	return strings.TrimSpace(prompt)
}

type pptxPayload struct {
	Title  string                `json:"title"`
	Theme  *officegen.SlideTheme `json:"theme"`
	Slides []officegen.Slide     `json:"slides"`
}

func BuildPPTXPrompt(description string, target generateengine.PromptTarget, enableImages bool) string {
	slideExample := `    {
      "title": "章节标题",
      "layout": "content",
      "points": ["要点1", "要点2", "要点3"],
      "source": "可选的数据来源"
    }`
	imageRules := "- 不要输出 hasImage、imagePrompt、imagePos 这些图片字段"
	if enableImages {
		slideExample = `    {
      "title": "章节标题",
      "layout": "content",
      "points": ["要点1", "要点2", "要点3"],
      "hasImage": true,
      "imagePrompt": "适合直接送给图像模型的具体视觉描述",
      "imagePos": "right",
      "source": "可选的数据来源"
    }`
		imageRules = `- 内容型页面可优先挑 1-3 页配图，不要求每页都配
- 配图页只允许输出 hasImage、imagePrompt、imagePos，其中 imagePos 只能是 right、left、background、center、top、bottom、diagonal
- imagePrompt 必须是可直接送给图像模型的具体视觉描述，不要写抽象词
- chart 或 dashboard 布局不要配图`
	}
	return fmt.Sprintf(`请根据以下需求生成一个 PPT 演示文稿的 JSON 结构。

需求：%s
%s

请严格输出 JSON，不要输出任何额外说明：
{
  "title": "演示文稿标题",
  "theme": {
    "primaryColor": "1A73E8",
    "accentColor": "E8710A",
    "backgroundType": "gradient",
    "bgColor1": "F0F4FF",
    "bgColor2": "FFFFFF"
  },
  "slides": [
    {
      "title": "封面标题",
      "layout": "title",
      "subtitle": "副标题",
      "isTitle": true
    },
%s
  ]
}

要求：
- 总页数控制在 4-8 页
- 首页必须是 title 布局
- 内容页优先使用 content，必要时可用 chart 或 dashboard
- points 要具体、有信息量，避免空话
- 如果适合做图表，chart 结构可包含 type/categories/values/title
- 如果适合做指标页，metrics 可包含 label/value/note
%s`, description, generateengine.FormatDocumentPromptTarget(target), slideExample, imageRules)
}

func BuildPPTXFromJSON(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, content, fallback string, enableImages bool) ([]byte, string, []engine.GenerateIssue, error) {
	emitProgress(ctx, progress, progressStepAssemble, "running", "正在解析 pptx 结构并准备素材")
	content = generateengine.RepairUnescapedQuotes(generateengine.ExtractJSON(content))

	var payload pptxPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "pptx 结构解析失败")
		return nil, "", nil, fmt.Errorf("文档组装阶段失败：parse llm response: %w", err)
	}
	if len(payload.Slides) == 0 {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "pptx 结构为空")
		return nil, "", nil, fmt.Errorf("文档组装阶段失败：slides cannot be empty")
	}
	warnings := make([]engine.GenerateIssue, 0, 1)
	if !enableImages {
		for idx := range payload.Slides {
			payload.Slides[idx].HasImage = false
			payload.Slides[idx].ImagePrompt = ""
			payload.Slides[idx].ImageData = nil
			payload.Slides[idx].ImageMIME = ""
		}
	}
	imageTotal := 0
	for idx := range payload.Slides {
		if payload.Slides[idx].HasImage && strings.TrimSpace(payload.Slides[idx].ImagePrompt) != "" {
			imageTotal++
		}
	}
	imageIndex := 0
	for idx := range payload.Slides {
		if payload.Slides[idx].HasImage && strings.TrimSpace(payload.Slides[idx].ImagePrompt) != "" && llm != nil {
			imageIndex++
			emitProgress(ctx, progress, progressStepAssemble, "running", fmt.Sprintf("正在生成图片素材（%d/%d）", imageIndex, imageTotal))
			aspectRatio := officegen.TargetAspectRatioForSlide(payload.Slides[idx])
			image, err := llm.GenerateImage(ctx, engine.ImageGenerationRequest{
				Prompt:            payload.Slides[idx].ImagePrompt,
				TargetAspectRatio: aspectRatio,
			})
			if err == nil && image != nil {
				payload.Slides[idx].ImageData = image.Data
				payload.Slides[idx].ImageMIME = image.MIME
				continue
			}
			payload.Slides[idx].ImageData = nil
			payload.Slides[idx].ImageMIME = ""
			if len(warnings) == 0 {
				warnings = append(warnings, engine.GenerateIssue{
					Code:    "WARN_PPT_IMAGE_DEGRADED",
					Message: "部分图片生成失败，已自动降级为无图版本。",
					Field:   "slides",
				})
			}
		}
	}

	emitProgress(ctx, progress, progressStepAssemble, "running", "正在打包 pptx 文件")
	fileBytes, err := officegen.NewPPTXGenerator().Generate(payload.Slides, officegen.PPTXOptions{
		Title:   payload.Title,
		Creator: "ClaudeOffice",
		Theme:   payload.Theme,
	})
	if err != nil {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "pptx 打包失败")
		return nil, "", nil, fmt.Errorf("文档组装阶段失败：generate pptx: %w", err)
	}
	emitProgress(ctx, progress, progressStepAssemble, "completed", "pptx 文件组装完成")

	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = generateengine.ExtractTitleFromDescription(fallback)
	}
	return fileBytes, fmt.Sprintf("%s.pptx", generateengine.SanitizeFileName(title)), warnings, nil
}

func DecodeBase64Image(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}
