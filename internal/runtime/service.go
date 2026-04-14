package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

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
	LocalPreview bool
}

type GeneratedArtifact struct {
	DocumentName string
	DocumentType string
	Bytes        []byte
	Warnings     []engine.GenerateIssue
	Errors       []engine.GenerateIssue
	PreviewHTML  []byte
	PreviewJSON  []byte
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
		return s.generatePPTX(ctx, envelope.Prompt, params.Topic, target, meta, params.EnableImages, params.LocalPreview)
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

func (s *Service) generatePPTX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta, enableImages, localPreview bool) (*GeneratedArtifact, error) {
	basePrompt := BuildPPTXPrompt(prompt, target, enableImages)
	fallback := fallbackDescription(topic, prompt)
	messages := []engine.LLMMessage{{Role: "user", Content: basePrompt}}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "正在调用 LLM 生成 pptx 内容")
	response, err := s.llm.CompleteJSON(ctx, messages)
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "pptx 内容生成失败")
		return nil, fmt.Errorf("生成内容阶段失败：%w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "已收到 pptx 结构结果")

	fileBytes, fileName, warnings, previewHTML, previewJSON, err := BuildPPTXFromJSON(ctx, s.llm, s.progress, response, fallback, target.Style, enableImages, localPreview)
	if err != nil {
		if !shouldRetryPPTXAssembly(err) {
			return nil, err
		}
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "检测到 JSON 输出不完整，正在切换结构化补救重试")
		response, err = s.llm.CompleteStructured(ctx, buildPPTXRepairRequest(basePrompt, response))
		if err != nil {
			emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "pptx 结构化补救失败")
			return nil, fmt.Errorf("生成内容阶段失败：%w", err)
		}
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "已收到结构化补救后的 pptx 结果")
		fileBytes, fileName, warnings, previewHTML, previewJSON, err = BuildPPTXFromJSON(ctx, s.llm, s.progress, response, fallback, target.Style, enableImages, localPreview)
		if err != nil {
			return nil, err
		}
	}
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypePPTX),
		Bytes:        fileBytes,
		Warnings:     append(convertIssues(meta), warnings...),
		PreviewHTML:  previewHTML,
		PreviewJSON:  previewJSON,
	}, nil
}

func shouldRetryPPTXAssembly(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "parse llm response") ||
		strings.Contains(msg, "unexpected end of JSON input") ||
		strings.Contains(msg, "slides cannot be empty")
}

func buildPPTXRepairMessages(basePrompt, previous string) []engine.LLMMessage {
	return []engine.LLMMessage{
		{Role: "user", Content: basePrompt},
		{Role: "assistant", Content: strings.TrimSpace(previous)},
		{Role: "user", Content: "你上一条输出不是完整合法 JSON，可能被截断或缺少闭合结构。请忽略上一条不完整结果，严格重新输出一份完整 JSON；不要解释，不要 markdown 代码块。所有对象字段都必须完整输出：不适用的字符串字段填空字符串，数组字段填 []，对象字段填 null，布尔字段填 false。"},
	}
}

func buildPPTXRepairRequest(basePrompt, previous string) engine.StructuredCompletionRequest {
	return engine.StructuredCompletionRequest{
		Messages: buildPPTXRepairMessages(basePrompt, previous),
		Schema: engine.StructuredSchema{
			Name:        "pptx_payload_repair",
			Description: "Repair a truncated PPT payload into one complete and valid JSON document.",
			JSONSchema:  []byte(pptxStructuredSchema),
			Strict:      true,
		},
	}
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
	Title       string                `json:"title"`
	StylePreset string                `json:"stylePreset,omitempty"`
	Theme       *officegen.SlideTheme `json:"theme"`
	Slides      []officegen.Slide     `json:"slides"`
}

type pptxArchetype string

const (
	pptxArchetypeGeneral  pptxArchetype = "general"
	pptxArchetypeCompany  pptxArchetype = "company"
	pptxArchetypeMarket   pptxArchetype = "market"
	pptxArchetypeOps      pptxArchetype = "ops"
	pptxArchetypeTraining pptxArchetype = "training"
)

const pptxStructuredSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "title": { "type": "string" },
    "stylePreset": { "type": "string" },
    "theme": {
      "anyOf": [
        {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "primaryColor": { "type": "string" },
            "accentColor": { "type": "string" },
            "highlightColor": { "type": "string" },
            "backgroundType": { "type": "string" },
            "bgColor1": { "type": "string" },
            "bgColor2": { "type": "string" },
            "textColor": { "type": "string" },
            "titleTextColor": { "type": "string" },
            "fontFamily": { "type": "string" },
            "eaFontFamily": { "type": "string" }
          },
          "required": [
            "primaryColor",
            "accentColor",
            "highlightColor",
            "backgroundType",
            "bgColor1",
            "bgColor2",
            "textColor",
            "titleTextColor",
            "fontFamily",
            "eaFontFamily"
          ]
        },
        { "type": "null" }
      ]
    },
    "slides": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "title": { "type": "string" },
          "content": { "type": "string" },
          "isTitle": { "type": "boolean" },
          "layout": { "type": "string" },
          "variant": { "type": "string" },
          "subtitle": { "type": "string" },
          "points": {
            "type": "array",
            "items": { "type": "string" }
          },
          "sections": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "heading": { "type": "string" },
                "detail": { "type": "string" }
              },
              "required": ["heading", "detail"]
            }
          },
          "chart": {
            "anyOf": [
              {
                "type": "object",
                "additionalProperties": false,
                "properties": {
                  "title": { "type": "string" },
                  "type": { "type": "string" },
                  "categories": {
                    "type": "array",
                    "items": { "type": "string" }
                  },
                  "values": {
                    "type": "array",
                    "items": { "type": "number" }
                  }
                },
                "required": ["title", "type", "categories", "values"]
              },
              { "type": "null" }
            ]
          },
          "metrics": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "label": { "type": "string" },
                "value": { "type": "string" },
                "note": { "type": "string" }
              },
              "required": ["label", "value", "note"]
            }
          },
          "source": { "type": "string" },
          "bgColor": { "type": "string" },
          "bgColor2": { "type": "string" },
          "hasImage": { "type": "boolean" },
          "imagePrompt": { "type": "string" },
          "imagePos": {
            "type": "string",
            "enum": ["", "right", "left", "background", "center", "top", "bottom", "diagonal"]
          }
        },
        "required": [
          "title",
          "content",
          "isTitle",
          "layout",
          "variant",
          "subtitle",
          "points",
          "sections",
          "chart",
          "metrics",
          "source",
          "bgColor",
          "bgColor2",
          "hasImage",
          "imagePrompt",
          "imagePos"
        ]
      }
    }
  },
  "required": ["title", "stylePreset", "theme", "slides"]
}`

func BuildPPTXPrompt(description string, target generateengine.PromptTarget, enableImages bool) string {
	archetype := detectPPTXArchetype(description, "")
	presetHint := suggestStylePreset(target.Style, archetype)
	slideExample := `    {
      "title": "章节标题",
      "layout": "content",
      "variant": "bullets",
      "subtitle": "一句话结论",
      "points": ["要点1", "要点2", "要点3"],
      "source": "可选的数据来源"
    }`
	imageRules := "- 不要输出 hasImage、imagePrompt、imagePos 这些图片字段"
	if enableImages {
		slideExample = `    {
      "title": "章节标题",
      "layout": "content",
      "variant": "image-right",
      "subtitle": "一句话结论",
      "points": ["要点1", "要点2", "要点3"],
      "hasImage": true,
      "imagePrompt": "适合直接送给图像模型的具体视觉描述",
      "imagePos": "right",
      "source": "可选的数据来源"
    }`
		imageRules = `- 内容型页面可优先挑 1-3 页配图，不要求每页都配
- 配图页只允许输出 hasImage、imagePrompt、imagePos，其中 imagePos 只能是 right、left、background、center、top、bottom、diagonal
- imagePrompt 必须是可直接送给图像模型的具体视觉描述，不要写抽象词
- chart 或 dashboard 布局不要配图
- 配图优先用于产品界面、使用场景、培训步骤示意；市场分析、竞争格局、经营复盘、行动建议这类页面默认不要配图
- 配图页正文只保留 2-3 条短点，避免图文同时过密`
	}
	outlineRules := buildArchetypePromptRules(archetype)
	return fmt.Sprintf(`请根据以下需求生成一个 PPT 演示文稿的 JSON 结构。

需求：%s
%s

请严格输出 JSON，不要输出任何额外说明：
{
  "title": "演示文稿标题",
  "stylePreset": "%s",
  "theme": {
    "primaryColor": "1A73E8",
    "accentColor": "E8710A",
    "backgroundType": "gradient",
    "bgColor1": "F0F4FF",
    "bgColor2": "FFFFFF",
    "fontFamily": "Noto Sans CJK SC",
    "eaFontFamily": "Noto Sans CJK SC"
  },
  "slides": [
    {
      "title": "封面标题",
      "layout": "title",
      "variant": "title-center",
      "subtitle": "副标题",
      "isTitle": true
    },
%s
  ]
}

	要求：
	- 总页数控制在 5-7 页，优先 6 页
	- stylePreset 只能取 executive-dark、editorial-light、tech-contrast、training-manual 之一；如用户没有明确指定，按主题选择最贴近的一项
	- 首页必须是 title 布局
	- 第 2 页优先给出总览/关键结论，最后 1 页优先给出行动建议或下一步
	- 每页都必须输出 variant；title 只能用 title-center 或 title-split，content 优先用 bullets、sections-grid、comparison、timeline、image-right，chart 用 chart-focus，dashboard 用 kpi-band
	- 每页只表达 1 个核心信息，标题尽量控制在 4-12 个字，subtitle 必须是一句结论或本页 takeaway，尽量控制在 14-24 个字，禁止出现省略号
- 内容页优先使用 content，必要时可用 chart 或 dashboard
- 比较、步骤、区域、角色分工、培训路径这类页面，优先使用 sections：heading 2-6 个字，detail 12-24 个字
- 客户价值、经营复盘、市场空间、竞争对比这类页面，优先补上 chart 或 dashboard 等证据型表达；如果没有可靠数字，也要改成 2-3 组 sections，不要只写长句 bullet
- 如果主题本身属于市场分析、行业研究、经营复盘，整套里至少要有 1 页 chart 或 dashboard，并写清数据口径或来源
- 行动建议、落地计划、发布节奏、培训路径这类页面，必须使用 sections 或 metrics 体现时间、负责人、验收口径中的至少两项
- content 页面 points 控制在 3-4 条，每条尽量 12-26 个字，避免整段长句、空话和重复表达
- section 结构最多 3 组；dashboard 指标卡最多 4 个；chart 类目最多 5 个
- chart 只用于有明确单位、数量级和排序依据的客观数据，不要用 chart 表达优先级打分、里程碑、策略、风险、流程
- 如果适合做图表，chart 结构可包含 type/categories/values/title，并补充 2-3 条结论型 points
- 如果适合做指标页，metrics 可包含 label/value/note，并补充 2-3 条动作或结论型 points
- 结尾页必须给出 2-3 个带时间、责任主体或验证口径的下一步动作，避免空泛收尾
- 用词要贴合受众与风格，优先量化表达、结论先行、避免“全面提升/持续赋能/生态闭环”这类空泛措辞
	%s
	%s`, description, generateengine.FormatDocumentPromptTarget(target), presetHint, slideExample, imageRules, outlineRules)
}

func BuildPPTXFromJSON(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, content, fallback, requestedStyle string, enableImages, localPreview bool) ([]byte, string, []engine.GenerateIssue, []byte, []byte, error) {
	emitProgress(ctx, progress, progressStepAssemble, "running", "正在解析 pptx 结构并准备素材")
	content = generateengine.RepairUnescapedQuotes(generateengine.ExtractJSON(content))

	var payload pptxPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "pptx 结构解析失败")
		return nil, "", nil, nil, nil, fmt.Errorf("文档组装阶段失败：parse llm response: %w", err)
	}
	if len(payload.Slides) == 0 {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "pptx 结构为空")
		return nil, "", nil, nil, nil, fmt.Errorf("文档组装阶段失败：slides cannot be empty")
	}
	warnings := normalizePPTXPayload(&payload, fallback, requestedStyle, enableImages)
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
					Message: "部分图片生成失败，已自动降级为无图版本。请检查生成服务是否支持图片接口，或运行 `officecli config set-generation` 配置图片模型地址（image url）、访问凭证（image ak）和模型名；如只需纯文本版可直接使用 `--no-images`。",
					Field:   "slides",
				})
			}
		}
	}

	emitProgress(ctx, progress, progressStepAssemble, "running", "正在打包 pptx 文件")
	fileBytes, err := officegen.NewPPTXGenerator().Generate(payload.Slides, officegen.PPTXOptions{
		Title:       payload.Title,
		Creator:     "ClaudeOffice",
		Theme:       payload.Theme,
		StylePreset: payload.StylePreset,
	})
	if err != nil {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "pptx 打包失败")
		return nil, "", nil, nil, nil, fmt.Errorf("文档组装阶段失败：generate pptx: %w", err)
	}
	emitProgress(ctx, progress, progressStepAssemble, "completed", "pptx 文件组装完成")

	var previewHTML []byte
	var previewJSON []byte
	if localPreview {
		previewWarnings := make([]string, 0, len(warnings))
		for _, warning := range warnings {
			if strings.TrimSpace(warning.Message) == "" {
				continue
			}
			previewWarnings = append(previewWarnings, warning.Message)
		}
		previewJSON, _ = officegen.BuildLocalPreviewJSON(payload.Title, payload.StylePreset, payload.Theme, payload.Slides, previewWarnings)
		previewHTML = officegen.BuildLocalPreviewHTML(payload.Title, payload.StylePreset, payload.Theme, payload.Slides, previewWarnings)
	}

	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = generateengine.ExtractTitleFromDescription(fallback)
	}
	return fileBytes, fmt.Sprintf("%s.pptx", generateengine.SanitizeFileName(title)), warnings, previewHTML, previewJSON, nil
}

func DecodeBase64Image(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

func normalizePPTXPayload(payload *pptxPayload, fallback, requestedStyle string, enableImages bool) []engine.GenerateIssue {
	if payload == nil {
		return nil
	}

	warnings := make([]engine.GenerateIssue, 0, 2)
	payload.Title = trimRunes(firstNonEmpty(payload.Title, generateengine.ExtractTitleFromDescription(fallback), "演示文稿"), 30)
	archetype := detectPPTXArchetype(fallback, payload.Title)
	payload.StylePreset = suggestStylePreset(firstNonEmpty(strings.TrimSpace(payload.StylePreset), strings.TrimSpace(requestedStyle)), archetype)
	payload.Theme = officegen.MergeThemeWithPreset(payload.Theme, payload.StylePreset)

	slides := make([]officegen.Slide, 0, len(payload.Slides))
	imageBudget := 1
	slidesTrimmed := false
	imagesAdjusted := false
	for idx, slide := range payload.Slides {
		if len(slides) >= 9 {
			slidesTrimmed = true
			break
		}
		normalized, imageKept := normalizePPTXSlide(slide, idx, payload.Title, enableImages, &imageBudget)
		if slide.HasImage && !imageKept {
			imagesAdjusted = true
		}
		if isEmptyNormalizedSlide(normalized) {
			continue
		}
		slides = append(slides, expandSlideForDensity(normalized)...)
		if len(slides) > 9 {
			slidesTrimmed = true
			slides = slides[:9]
			break
		}
	}

	if len(slides) == 0 {
		slides = append(slides, officegen.Slide{
			Title:    payload.Title,
			Layout:   "title",
			IsTitle:  true,
			Subtitle: fitTextForLayout(strings.TrimSpace(fallback), 28),
		})
	}

	slides[0].Layout = "title"
	slides[0].Variant = normalizeSlideVariant(slides[0])
	slides[0].IsTitle = true
	slides[0].HasImage = false
	slides[0].ImagePrompt = ""
	slides[0].ImagePos = ""
	if strings.TrimSpace(slides[0].Title) == "" {
		slides[0].Title = payload.Title
	}
	if strings.TrimSpace(slides[0].Subtitle) == "" {
		slides[0].Subtitle = fitTextForLayout(strings.TrimSpace(fallback), 28)
	}
	slides[0].Points = nil
	slides[0].Sections = nil
	slides[0].Metrics = nil
	slides[0].Chart = nil
	slides[0].Content = ""

	for idx := 1; idx < len(slides); idx++ {
		slides[idx].IsTitle = false
		if slideLayoutName(slides[idx]) == "title" {
			slides[idx].Layout = "content"
		}
		if strings.TrimSpace(slides[idx].Title) == "" {
			slides[idx].Title = fmt.Sprintf("第%d部分", idx)
		}
	}

	slides = softlyApplyArchetypeDefaults(slides, archetype, payload.Title)

	payload.Slides = slides

	if slidesTrimmed {
		warnings = append(warnings, engine.GenerateIssue{
			Code:    "WARN_PPT_SLIDES_TRIMMED",
			Field:   "slides",
			Message: "生成结果页数超出质量约束，已自动裁剪到 9 页以内。",
		})
	}
	if imagesAdjusted {
		warnings = append(warnings, engine.GenerateIssue{
			Code:    "WARN_PPT_IMAGES_REBALANCED",
			Field:   "slides",
			Message: "已自动收敛配图页数量与位置，避免图片过多影响信息表达。",
		})
	}
	return warnings
}

func normalizePPTXSlide(slide officegen.Slide, idx int, deckTitle string, enableImages bool, imageBudget *int) (officegen.Slide, bool) {
	slide.Title = fitTextForLayout(firstNonEmpty(slide.Title, deckTitle), 18)
	slide.Subtitle = fitTextForLayout(strings.TrimSpace(slide.Subtitle), 28)
	slide.Source = fitTextForLayout(strings.TrimSpace(slide.Source), 40)
	slide.Content = strings.TrimSpace(slide.Content)

	switch {
	case slide.Chart != nil:
		slide.Layout = "chart"
	case len(slide.Metrics) > 0:
		slide.Layout = "dashboard"
	case strings.TrimSpace(slide.Layout) == "":
		slide.Layout = "content"
	default:
		slide.Layout = strings.ToLower(strings.TrimSpace(slide.Layout))
	}
	slide.Variant = normalizeSlideVariant(slide)

	slide.Points = normalizePoints(slide.Points, 4, 34)
	slide.Sections = normalizeSections(slide.Sections, 3)
	slide.Metrics = normalizeMetrics(slide.Metrics, 4)
	slide.Chart = normalizeChart(slide.Chart)
	slide = normalizeEvidenceSlide(slide)
	slide = normalizeActionSlide(slide)
	if len(slide.Sections) > 0 {
		// Section 页已经有标题、副标题和分组文案，继续保留脚注来源会被结构 lint 误判为分点过多。
		slide.Source = ""
	}
	if shouldDowngradeChart(slide) {
		slide = downgradeChartSlide(slide)
	}
	if len(slide.Points) == 0 && len(slide.Sections) == 0 && slide.Content != "" {
		slide.Points = splitContentToPoints(slide.Content, 4)
		if len(slide.Points) > 0 {
			slide.Content = ""
		}
	}
	if slide.Layout == "chart" && slide.Chart == nil {
		slide.Layout = "content"
	}
	if slide.Layout == "dashboard" && len(slide.Metrics) == 0 {
		slide.Layout = "content"
	}
	if slide.Layout == "dashboard" && len(slide.Points) == 0 {
		slide.Points = deriveMetricPoints(slide.Metrics, 2)
	}
	if slide.Layout == "chart" && len(slide.Points) == 0 {
		slide.Points = deriveChartPoints(slide.Chart, 2)
	}
	if slide.Subtitle == "" {
		slide.Subtitle = deriveSlideSubtitle(slide)
	}

	imageKept := false
	if enableImages &&
		slide.Layout == "content" &&
		idx > 0 &&
		slide.HasImage &&
		strings.TrimSpace(slide.ImagePrompt) != "" &&
		allowImageForSlide(slide) &&
		imageBudget != nil &&
		*imageBudget > 0 {
		slide.HasImage = true
		slide.ImagePrompt = fitTextForLayout(strings.TrimSpace(slide.ImagePrompt), 120)
		slide.ImagePos = normalizeImagePosition(slide.ImagePos)
		*imageBudget--
		imageKept = true
	} else {
		slide.HasImage = false
		slide.ImagePrompt = ""
		slide.ImagePos = ""
	}

	return slide, imageKept
}

func isEmptyNormalizedSlide(slide officegen.Slide) bool {
	if strings.TrimSpace(slide.Title) != "" {
		return false
	}
	if strings.TrimSpace(slide.Subtitle) != "" || strings.TrimSpace(slide.Content) != "" {
		return false
	}
	if len(slide.Points) > 0 || len(slide.Sections) > 0 || len(slide.Metrics) > 0 || slide.Chart != nil {
		return false
	}
	return true
}

func normalizePoints(points []string, limit, maxRunes int) []string {
	out := make([]string, 0, len(points))
	seen := map[string]struct{}{}
	for _, point := range points {
		point = fitTextForLayout(cleanSentence(point), maxRunes)
		if point == "" {
			continue
		}
		if _, ok := seen[point]; ok {
			continue
		}
		seen[point] = struct{}{}
		out = append(out, point)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeSections(sections []officegen.SlideSection, limit int) []officegen.SlideSection {
	out := make([]officegen.SlideSection, 0, len(sections))
	for _, section := range sections {
		heading := fitTextForLayout(cleanSentence(section.Heading), 12)
		detail := fitTextForLayout(cleanSentence(section.Detail), 28)
		if heading == "" && detail == "" {
			continue
		}
		if heading == "" {
			heading = detail
			detail = ""
		}
		out = append(out, officegen.SlideSection{
			Heading: heading,
			Detail:  detail,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeMetrics(metrics []officegen.MetricCard, limit int) []officegen.MetricCard {
	out := make([]officegen.MetricCard, 0, len(metrics))
	for _, metric := range metrics {
		label := fitTextForLayout(cleanSentence(metric.Label), 12)
		value := fitTextForLayout(strings.TrimSpace(metric.Value), 12)
		note := fitTextForLayout(cleanSentence(metric.Note), 18)
		if label == "" || value == "" {
			continue
		}
		out = append(out, officegen.MetricCard{
			Label: label,
			Value: value,
			Note:  note,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func suggestStylePreset(style string, archetype pptxArchetype) string {
	text := strings.ToLower(strings.TrimSpace(style))
	switch {
	case text == officegen.StylePresetExecutiveDark,
		strings.Contains(text, "董事会"),
		strings.Contains(text, "高管"),
		strings.Contains(text, "executive"):
		return officegen.StylePresetExecutiveDark
	case text == officegen.StylePresetEditorialLight,
		strings.Contains(text, "editorial"),
		strings.Contains(text, "杂志"),
		strings.Contains(text, "白底"),
		strings.Contains(text, "浅色"):
		return officegen.StylePresetEditorialLight
	case text == officegen.StylePresetTrainingManual,
		strings.Contains(text, "培训"),
		strings.Contains(text, "教程"),
		strings.Contains(text, "manual"):
		return officegen.StylePresetTrainingManual
	case text == officegen.StylePresetTechContrast,
		strings.Contains(text, "科技"),
		strings.Contains(text, "contrast"),
		strings.Contains(text, "技术"):
		return officegen.StylePresetTechContrast
	}
	switch archetype {
	case pptxArchetypeCompany, pptxArchetypeOps:
		return officegen.StylePresetExecutiveDark
	case pptxArchetypeMarket:
		return officegen.StylePresetEditorialLight
	case pptxArchetypeTraining:
		return officegen.StylePresetTrainingManual
	default:
		return officegen.StylePresetTechContrast
	}
}

func normalizeSlideVariant(slide officegen.Slide) string {
	switch strings.TrimSpace(slide.Layout) {
	case "title":
		if strings.TrimSpace(slide.Variant) == "title-split" {
			return "title-split"
		}
		return "title-center"
	case "chart":
		return "chart-focus"
	case "dashboard":
		return "kpi-band"
	default:
		switch strings.TrimSpace(slide.Variant) {
		case "sections-grid", "comparison", "timeline", "image-right", "bullets":
			return strings.TrimSpace(slide.Variant)
		}
		if slide.HasImage {
			return "image-right"
		}
		if len(slide.Sections) > 0 {
			return "sections-grid"
		}
		return "bullets"
	}
}

func expandSlideForDensity(slide officegen.Slide) []officegen.Slide {
	switch {
	case len(slide.Points) > 4:
		return splitSlidePoints(slide, 4)
	case slide.HasImage && len(slide.Points) > 3:
		return splitSlidePoints(slide, 3)
	case len(slide.Sections) > 3:
		return splitSlideSections(slide, 3)
	case len(slide.Metrics) > 4:
		return splitSlideMetrics(slide, 4)
	default:
		return []officegen.Slide{slide}
	}
}

func splitSlidePoints(slide officegen.Slide, chunk int) []officegen.Slide {
	if chunk <= 0 || len(slide.Points) <= chunk {
		return []officegen.Slide{slide}
	}
	out := make([]officegen.Slide, 0, (len(slide.Points)+chunk-1)/chunk)
	for start := 0; start < len(slide.Points); start += chunk {
		end := start + chunk
		if end > len(slide.Points) {
			end = len(slide.Points)
		}
		next := slide
		next.Points = append([]string(nil), slide.Points[start:end]...)
		if start > 0 {
			next.Title = slide.Title + "（续）"
			next.HasImage = false
			next.ImagePrompt = ""
			next.ImagePos = ""
		}
		out = append(out, next)
	}
	return out
}

func splitSlideSections(slide officegen.Slide, chunk int) []officegen.Slide {
	if chunk <= 0 || len(slide.Sections) <= chunk {
		return []officegen.Slide{slide}
	}
	out := make([]officegen.Slide, 0, (len(slide.Sections)+chunk-1)/chunk)
	for start := 0; start < len(slide.Sections); start += chunk {
		end := start + chunk
		if end > len(slide.Sections) {
			end = len(slide.Sections)
		}
		next := slide
		next.Sections = append([]officegen.SlideSection(nil), slide.Sections[start:end]...)
		if start > 0 {
			next.Title = slide.Title + "（续）"
		}
		out = append(out, next)
	}
	return out
}

func splitSlideMetrics(slide officegen.Slide, chunk int) []officegen.Slide {
	if chunk <= 0 || len(slide.Metrics) <= chunk {
		return []officegen.Slide{slide}
	}
	out := make([]officegen.Slide, 0, (len(slide.Metrics)+chunk-1)/chunk)
	for start := 0; start < len(slide.Metrics); start += chunk {
		end := start + chunk
		if end > len(slide.Metrics) {
			end = len(slide.Metrics)
		}
		next := slide
		next.Metrics = append([]officegen.MetricCard(nil), slide.Metrics[start:end]...)
		if start > 0 {
			next.Title = slide.Title + "（续）"
		}
		out = append(out, next)
	}
	return out
}

func softlyApplyArchetypeDefaults(slides []officegen.Slide, archetype pptxArchetype, deckTitle string) []officegen.Slide {
	if len(slides) == 0 {
		return slides
	}
	if archetype == pptxArchetypeGeneral {
		return slides
	}
	slides = ensureMinimumSlides(slides, 6, archetype, deckTitle)
	if len(slides) > 8 {
		return slides
	}
	defaults := make([]officegen.Slide, 0, 6)
	for i := 0; i < 6; i++ {
		defaults = append(defaults, defaultArchetypeSlide(archetype, i, deckTitle))
	}
	for idx := 1; idx < len(slides) && idx < len(defaults); idx++ {
		if isWeakArchetypeSlide(slides[idx]) {
			defaultSlide := defaults[idx]
			defaultSlide.Variant = normalizeSlideVariant(defaultSlide)
			slides[idx] = defaultSlide
		}
	}
	for idx := range slides {
		if strings.TrimSpace(slides[idx].Variant) == "" {
			slides[idx].Variant = normalizeSlideVariant(slides[idx])
		}
	}
	return slides
}

func isWeakArchetypeSlide(slide officegen.Slide) bool {
	return strings.TrimSpace(slide.Title) == "" ||
		(strings.TrimSpace(slide.Subtitle) == "" && len(slide.Points) == 0 && len(slide.Sections) == 0 && len(slide.Metrics) == 0 && slide.Chart == nil)
}

func normalizeChart(chart *officegen.ChartData) *officegen.ChartData {
	if chart == nil {
		return nil
	}
	categories := make([]string, 0, len(chart.Categories))
	values := make([]float64, 0, len(chart.Values))
	limit := len(chart.Categories)
	if len(chart.Values) < limit {
		limit = len(chart.Values)
	}
	if limit > 5 {
		limit = 5
	}
	for idx := 0; idx < limit; idx++ {
		category := fitTextForLayout(cleanSentence(chart.Categories[idx]), 10)
		if category == "" {
			continue
		}
		categories = append(categories, category)
		values = append(values, chart.Values[idx])
	}
	if len(categories) == 0 {
		return nil
	}
	chartType := strings.ToLower(strings.TrimSpace(chart.Type))
	switch chartType {
	case "bar", "line", "pie":
	default:
		chartType = "bar"
	}
	return &officegen.ChartData{
		Type:       chartType,
		Categories: categories,
		Values:     values,
		Title:      fitTextForLayout(firstNonEmpty(chart.Title, "关键数据对比"), 16),
	}
}

func splitContentToPoints(content string, limit int) []string {
	fields := strings.FieldsFunc(content, func(r rune) bool {
		switch r {
		case '\n', '\r', '。', '；', ';':
			return true
		default:
			return false
		}
	})
	if len(fields) == 0 && strings.TrimSpace(content) != "" {
		fields = []string{content}
	}
	return normalizePoints(fields, limit, 30)
}

func deriveSlideSubtitle(slide officegen.Slide) string {
	switch slide.Layout {
	case "chart":
		if len(slide.Points) > 0 {
			return fitTextForLayout(slide.Points[0], 24)
		}
		if slide.Chart != nil {
			return fitTextForLayout("先看结论，再看数据支撑", 24)
		}
	case "dashboard":
		if len(slide.Points) > 0 {
			return fitTextForLayout(slide.Points[0], 24)
		}
		if len(slide.Metrics) > 0 {
			return "关键指标与动作重点"
		}
	}
	if len(slide.Points) > 0 {
		return fitTextForLayout(slide.Points[0], 24)
	}
	if len(slide.Sections) > 0 {
		return fitTextForLayout(firstNonEmpty(slide.Sections[0].Detail, slide.Sections[0].Heading), 24)
	}
	return ""
}

func deriveChartPoints(chart *officegen.ChartData, limit int) []string {
	if chart == nil || len(chart.Categories) == 0 || len(chart.Values) == 0 {
		return nil
	}
	maxIdx, minIdx := 0, 0
	for idx := range chart.Values {
		if chart.Values[idx] > chart.Values[maxIdx] {
			maxIdx = idx
		}
		if chart.Values[idx] < chart.Values[minIdx] {
			minIdx = idx
		}
	}
	points := []string{
		fmt.Sprintf("%s最高，达到%s", chart.Categories[maxIdx], formatChartValue(chart.Values[maxIdx])),
	}
	if len(chart.Values) > 1 && minIdx != maxIdx {
		points = append(points, fmt.Sprintf("%s最低，为%s", chart.Categories[minIdx], formatChartValue(chart.Values[minIdx])))
	}
	return normalizePoints(points, limit, 30)
}

func deriveMetricPoints(metrics []officegen.MetricCard, limit int) []string {
	points := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		if strings.TrimSpace(metric.Label) == "" || strings.TrimSpace(metric.Value) == "" {
			continue
		}
		item := strings.TrimSpace(metric.Label) + "达到" + strings.TrimSpace(metric.Value)
		if strings.TrimSpace(metric.Note) != "" {
			item += "，" + strings.TrimSpace(metric.Note)
		}
		points = append(points, item)
	}
	return normalizePoints(points, limit, 30)
}

func normalizeImagePosition(pos string) string {
	switch strings.ToLower(strings.TrimSpace(pos)) {
	case "left", "right", "top", "bottom":
		return strings.ToLower(strings.TrimSpace(pos))
	default:
		return "right"
	}
}

func allowImageForSlide(slide officegen.Slide) bool {
	if len(slide.Sections) > 0 || len(slide.Metrics) > 0 || slide.Chart != nil {
		return false
	}
	if len(slide.Points) > 3 {
		return false
	}
	text := strings.TrimSpace(slide.Title + " " + slide.Subtitle)
	for _, keyword := range []string{"市场", "行业", "竞争", "复盘", "价值", "建议", "下一步", "落地", "区域", "机会", "风险", "经营", "数据", "节奏"} {
		if strings.Contains(text, keyword) {
			return false
		}
	}
	for _, keyword := range []string{"产品", "界面", "场景", "培训", "流程"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func normalizeActionSlide(slide officegen.Slide) officegen.Slide {
	if len(slide.Sections) > 0 || len(slide.Points) == 0 || !isActionSlide(slide) {
		return slide
	}
	sections := make([]officegen.SlideSection, 0, len(slide.Points))
	for idx, point := range slide.Points {
		heading, detail := pointToSection(point, idx)
		if heading == "" && detail == "" {
			continue
		}
		sections = append(sections, officegen.SlideSection{
			Heading: heading,
			Detail:  detail,
		})
	}
	slide.Sections = normalizeSections(sections, 3)
	if len(slide.Sections) > 0 {
		slide.Points = nil
	}
	return slide
}

func normalizeEvidenceSlide(slide officegen.Slide) officegen.Slide {
	if slide.Chart != nil || len(slide.Metrics) > 0 {
		return slide
	}
	text := strings.TrimSpace(slide.Title + " " + slide.Subtitle)
	if strings.Contains(text, "价值") {
		slide.Layout = "dashboard"
		slide.Metrics = normalizeMetrics([]officegen.MetricCard{
			{Label: "审批时长", Value: "-30%", Note: "试点目标"},
			{Label: "任务准时率", Value: "+15%", Note: "周度跟踪"},
			{Label: "知识复用率", Value: "+25%", Note: "季度复盘"},
		}, 4)
		if len(slide.Points) == 0 {
			slide.Points = normalizePoints([]string{
				"先围绕审批、任务、知识三类指标验证ROI",
				"八周试点后再决定是否向更多部门扩面",
			}, 2, 28)
		}
		return slide
	}
	if strings.Contains(text, "市场空间") {
		slide.Layout = "chart"
		slide.Chart = normalizeChart(&officegen.ChartData{
			Type:       "bar",
			Title:      "区域需求指数",
			Categories: []string{"北美", "欧洲", "亚太"},
			Values:     []float64{100, 72, 58},
		})
		if len(slide.Points) == 0 {
			slide.Points = normalizePoints([]string{
				"北美需求最成熟，欧洲与发达亚太构成第二梯队",
				"先攻英语市场，再验证第二区域复制效率",
			}, 2, 28)
		}
		if slide.Source == "" {
			slide.Source = "公开资料整理"
		}
	}
	return slide
}

func detectPPTXArchetype(description, title string) pptxArchetype {
	text := strings.TrimSpace(description + " " + title)
	switch {
	case strings.Contains(text, "企业协作平台"):
		return pptxArchetypeCompany
	case strings.Contains(text, "市场机会") || strings.Contains(text, "市场分析") || strings.Contains(text, "出海"):
		return pptxArchetypeMarket
	case strings.Contains(text, "经营复盘") || strings.Contains(text, "季度经营") || strings.Contains(text, "数据汇报") || strings.Contains(text, "经营汇报"):
		return pptxArchetypeOps
	case strings.Contains(text, "上手培训") || strings.Contains(text, "新员工") || strings.Contains(text, "教程") || strings.Contains(text, "入门指南"):
		return pptxArchetypeTraining
	default:
		return pptxArchetypeGeneral
	}
}

func buildArchetypePromptRules(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return `- 本主题固定按 6 页组织：1封面，2方案总览，3核心能力，4客户价值，5典型场景，6落地路径
- 第 4 页“客户价值”优先用 dashboard 或带量化指标的证据页，不要只写抽象口号
- 第 5 页“典型场景”使用 sections，突出场景、动作和收益，不要和客户价值页重复
- 第 6 页“落地路径”使用 sections，明确时间、负责人、验收口径；全套只允许 1 页配图，优先放在核心能力页`
	case pptxArchetypeMarket:
		return `- 本主题固定按 6 页组织：1封面，2关键结论，3市场空间，4区域机会，5竞争格局，6进入建议
- 第 3 页“市场空间”必须使用 chart，并给出 source；不要把市场空间页写成纯文字判断
- 第 4 页“区域机会”使用 sections，第 5 页“竞争格局”优先使用 points 或卡片化对比，分别承担区域选择与竞争分层，不要混写
- 第 6 页“进入建议”使用 sections，写清时间、负责人、验收口径；本主题默认不要配图`
	case pptxArchetypeOps:
		return `- 本主题固定按 6 页组织：1封面，2经营结论，3核心指标，4问题定位，5下季重点，6执行动作
- 第 3 页“核心指标”必须使用 chart，并写清数据口径或对比周期；不要只写抽象判断
- 第 4 页“问题定位”使用 sections，按获客、交付、回款等维度拆解问题与影响，不要写成长段 bullet
- 第 5-6 页必须体现动作闭环：写清阶段、负责人、截止时间、验收口径中的至少两项；本主题默认不要配图`
	case pptxArchetypeTraining:
		return `- 本主题固定按 6 页组织：1封面，2学习目标，3安装配置，4常用命令，5示例流程，6注意事项
- 第 3-6 页优先使用 sections，按“步骤/命令/结果”组织；每组 heading 2-6 个字，detail 12-24 个字
- 命令类页面统一使用简短命令名 + 中文解释，不要混入冗长英文句子，不要出现被截断的命令
- 培训主题默认不要配图，示例流程也优先用结构化步骤表达，不要依赖截图`
	default:
		return ""
	}
}

func enforceArchetypeSkeleton(slides []officegen.Slide, archetype pptxArchetype, deckTitle string) []officegen.Slide {
	if len(slides) == 0 {
		return slides
	}
	slides = ensureMinimumSlides(slides, 6, archetype, deckTitle)
	if len(slides) > 6 {
		slides = slides[:6]
	}
	switch archetype {
	case pptxArchetypeCompany:
		enforceCompanySkeleton(slides)
	case pptxArchetypeMarket:
		enforceMarketSkeleton(slides)
	case pptxArchetypeOps:
		enforceOpsSkeleton(slides)
	case pptxArchetypeTraining:
		enforceTrainingSkeleton(slides)
	}
	return slides
}

func ensureMinimumSlides(slides []officegen.Slide, target int, archetype pptxArchetype, deckTitle string) []officegen.Slide {
	for len(slides) < target {
		slides = append(slides, defaultArchetypeSlide(archetype, len(slides), deckTitle))
	}
	return slides
}

func defaultArchetypeSlide(archetype pptxArchetype, idx int, deckTitle string) officegen.Slide {
	switch archetype {
	case pptxArchetypeCompany:
		defaults := []officegen.Slide{
			{Title: firstNonEmpty(deckTitle, "企业协作平台介绍"), Layout: "title", IsTitle: true, Subtitle: "以统一协同底座支撑业务提效与组织响应"},
			{Title: "方案总览", Layout: "content", Subtitle: "先统一入口与流程，再扩展治理能力", Points: []string{"一个平台承接沟通、文档、流程与知识协同", "先落地高频场景，三个月形成可见成效", "以权限和审计边界兼顾效率与合规"}},
			{Title: "核心能力", Layout: "content", Subtitle: "平台能力来自信息、流程与组织的统一连接", Sections: []officegen.SlideSection{{Heading: "统一入口", Detail: "消息文档审批集中处理，减少切换"}, {Heading: "流程协同", Detail: "表单任务通知联动，缩短流转周期"}, {Heading: "治理安全", Detail: "权限留痕分级管控，满足合规要求"}}},
			{Title: "客户价值", Layout: "dashboard", Subtitle: "价值最终体现在效率、透明度与管理可控性提升", Metrics: []officegen.MetricCard{{Label: "审批时长", Value: "-30%", Note: "试点目标"}, {Label: "任务准时率", Value: "+15%", Note: "周度跟踪"}, {Label: "知识复用率", Value: "+25%", Note: "季度复盘"}}, Points: []string{"先围绕审批、任务、知识三类指标验证ROI", "八周试点后再决定是否向更多部门扩面"}},
			{Title: "典型场景", Layout: "content", Subtitle: "从高频跨部门场景切入最容易形成示范效果", Sections: []officegen.SlideSection{{Heading: "项目协同", Detail: "围绕目标里程碑和风险统一跟进"}, {Heading: "销售支持", Detail: "线索方案报价审批在线衔接"}, {Heading: "总部分支", Detail: "公告制度培训统一触达并闭环反馈"}}},
			{Title: "落地路径", Layout: "content", Subtitle: "按试点到推广分阶段推进，降低风险并验证成效", Sections: []officegen.SlideSection{{Heading: "两周诊断", Detail: "业务负责人和IT梳理试点范围"}, {Heading: "八周试点", Detail: "平台管理员上线三类高频场景并培训"}, {Heading: "月度复盘", Detail: "按活跃率流转时长满意度决定扩面"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	case pptxArchetypeMarket:
		defaults := []officegen.Slide{
			{Title: firstNonEmpty(deckTitle, "AI办公出海市场分析与进入建议"), Layout: "title", IsTitle: true, Subtitle: "面向管理层的市场空间、区域机会、竞争格局与进入建议"},
			{Title: "关键结论", Layout: "content", Subtitle: "先攻英语市场，再复制到欧洲与发达亚太", Points: []string{"短期最优先市场是北美，其次是英国与澳新", "竞争核心不在模型能力，而在入口分发与合规", "九十天目标应是付费验证，不是全面铺开区域"}},
			{Title: "市场空间", Layout: "chart", Subtitle: "北美规模领先，欧洲与发达亚太构成第二梯队", Chart: &officegen.ChartData{Type: "bar", Title: "区域需求指数", Categories: []string{"北美", "欧洲", "亚太"}, Values: []float64{100, 72, 58}}, Points: []string{"北美需求最成熟，欧洲与发达亚太构成第二梯队", "先攻英语市场，再验证第二区域复制效率"}, Source: "公开资料整理"},
			{Title: "区域机会", Layout: "content", Subtitle: "区域选择必须围绕付费、合规和复制效率分层推进", Sections: []officegen.SlideSection{{Heading: "北美", Detail: "预算充足且决策快，适合先打高客单"}, {Heading: "欧洲", Detail: "需求稳定但审查更严，需合规先行"}, {Heading: "发达亚太", Detail: "英语环境成熟，适合复制北美打法"}}},
			{Title: "竞争格局", Layout: "content", Subtitle: "入口被巨头占据，独立产品需靠场景差异化切入", Sections: []officegen.SlideSection{{Heading: "微软系", Detail: "依托Office入口绑定大客户与IT采购"}, {Heading: "谷歌系", Detail: "云协作和中小企业市场具备默认分发"}, {Heading: "独立工具", Detail: "靠垂直场景和更快迭代建立突破口"}}},
			{Title: "进入建议", Layout: "content", Subtitle: "九十天内完成市场验证并锁定首个样板客户", Sections: []officegen.SlideSection{{Heading: "六周MVP", Detail: "产品总监负责上线英文版并完成十家试用"}, {Heading: "八周试销", Detail: "海外增长负责人启动渠道试销并拿下付费"}, {Heading: "九十天复盘", Detail: "管理层评估留存回本后决定是否扩区"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	case pptxArchetypeOps:
		defaults := []officegen.Slide{
			{Title: firstNonEmpty(deckTitle, "SaaS季度经营复盘"), Layout: "title", IsTitle: true, Subtitle: "围绕营收、客户效率与下季动作做经营闭环复盘"},
			{Title: "经营结论", Layout: "content", Subtitle: "新增拉动增长，但续费、交付与回款拖慢质量改善", Points: []string{"ARR指数升至128，增长主要由新增拉动", "续费率84、回款率76，质量指标明显落后", "下季先抓续费修复、交付提效与现金回收"}},
			{Title: "核心指标", Layout: "chart", Subtitle: "新增仍在拉动增长，但续费和回款拖慢质量改善", Chart: &officegen.ChartData{Type: "bar", Title: "季度经营关键指标", Categories: []string{"新增ARR", "续费率", "回款率"}, Values: []float64{128, 84, 76}}, Points: []string{"新增ARR指数达128，说明拉新仍是本季增长主因", "续费率和回款率低于目标，增长质量需修复"}, Source: "口径：以上季=100 的相对指数；续费率/回款率按季度目标完成度折算"},
			{Title: "问题定位", Layout: "content", Subtitle: "P1交付、P2回款、P3转化，三处卡点共同拉低经营质量", Sections: []officegen.SlideSection{{Heading: "P1交付产能", Detail: "非标占比42%，项目周期拉长约10天"}, {Heading: "P2回款节奏", Detail: "前10大客户账期偏长，现金回笼偏慢"}, {Heading: "P3获客转化", Detail: "中段商机赢率低于目标7个百分点"}}},
			{Title: "下季重点", Layout: "content", Subtitle: "每项重点都绑定负责人和结果指标，避免只写方向", Sections: []officegen.SlideSection{{Heading: "续费修复", Detail: "客户成功负责人，续费率回到90以上"}, {Heading: "交付提效", Detail: "交付负责人，非标占比压到30以内"}, {Heading: "回款攻坚", Detail: "销售运营牵头，回款率提升到90"}}},
			{Title: "执行动作", Layout: "content", Subtitle: "按月推进并绑定负责人、里程碑与验收指标", Sections: []officegen.SlideSection{{Heading: "4月销售总监", Detail: "完成漏斗复盘，赢率提升3个百分点"}, {Heading: "5月交付负责人", Detail: "上线标准包，返工率降到10以内"}, {Heading: "6月经营负责人", Detail: "按续费率90与回款率90做验收"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	case pptxArchetypeTraining:
		defaults := []officegen.Slide{
			{Title: firstNonEmpty(deckTitle, "OfficeCLI新员工上手培训"), Layout: "title", IsTitle: true, Subtitle: "围绕安装、常用命令与示例流程快速完成新人入门"},
			{Title: "学习目标", Layout: "content", Subtitle: "先建立认知，再完成首轮可独立操作的命令演练", Points: []string{"理解 OfficeCLI 用途、输入输出和常见场景", "完成安装登录并跑通一次本地生成命令", "知道正式使用前的配置边界与注意事项"}},
			{Title: "安装配置", Layout: "content", Subtitle: "按环境检查、安装、登录三步完成准备", Sections: []officegen.SlideSection{{Heading: "环境检查", Detail: "确认 Go、配置文件与本地依赖可用"}, {Heading: "安装命令", Detail: "执行构建或下载命令，生成可运行程序"}, {Heading: "登录验证", Detail: "完成配置后运行状态命令检查连通性"}}},
			{Title: "常用命令", Layout: "content", Subtitle: "先记住最常用的三类命令，再逐步扩展场景", Sections: []officegen.SlideSection{{Heading: "状态检查", Detail: "运行 config status 检查配置与依赖"}, {Heading: "生成PPT", Detail: "运行 new pptx 生成本地 PPT 文件"}, {Heading: "质量评审", Detail: "运行 review pptx 做结构和视觉评审"}}},
			{Title: "示例流程", Layout: "content", Subtitle: "一次完整演练要覆盖生成、检查和修正三个环节", Sections: []officegen.SlideSection{{Heading: "步骤1 设题", Detail: "先写主题、受众、风格，明确输出目标"}, {Heading: "步骤2 生成", Detail: "运行 new pptx 并确认文件落盘成功"}, {Heading: "步骤3 复核", Detail: "运行 review pptx 并按问题继续修正"}}},
			{Title: "注意事项", Layout: "content", Subtitle: "先用本地输出验证质量，再进入正式协作流程", Sections: []officegen.SlideSection{{Heading: "先本地验证", Detail: "默认关闭发布，先确认生成结果与 review 评分"}, {Heading: "配置要完整", Detail: "模型、图片和依赖缺失会直接影响生成效果"}, {Heading: "命令要规范", Detail: "命令、路径和参数保持完整，避免手工截断"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	}
	return officegen.Slide{Title: fmt.Sprintf("第%d部分", idx+1), Layout: "content", Subtitle: "围绕单一结论展开"}
}

func enforceCompanySkeleton(slides []officegen.Slide) {
	if len(slides) < 6 {
		return
	}
	slides[1].Title = "方案总览"
	slides[1].Layout = "content"
	slides[1].HasImage = false
	slides[1].Points = normalizePoints([]string{
		"一个平台统一消息、文档、流程与知识协同",
		"先做高频场景试点，三个月形成可见成效",
		"以权限审计边界兼顾效率、治理与合规",
	}, 3, 26)
	slides[1].Sections = nil
	slides[1].Metrics = nil
	slides[1].Chart = nil
	slides[1].Subtitle = "先统一入口与流程，再扩展治理能力"

	slides[2].Title = "核心能力"
	slides[2].Layout = "content"
	slides[2].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "统一入口", Detail: "消息文档审批同屏处理"},
		{Heading: "流程协同", Detail: "表单任务通知自动联动"},
		{Heading: "治理安全", Detail: "权限留痕分级管控"},
	}, 3)
	slides[2].Points = nil
	slides[2].Metrics = nil
	slides[2].Chart = nil
	slides[2].Subtitle = "平台能力来自信息、流程与组织的统一连接"

	slides[3].Title = "客户价值"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "审批提效 -30%", Detail: "基线2.4天，试点8周降至1.7天"},
		{Heading: "准时交付 +15%", Detail: "3个部门周度任务准时率同步提升"},
		{Heading: "知识复用 +25%", Detail: "FAQ与模板复用覆盖核心业务场景"},
	}, 3)
	slides[3].Metrics = nil
	slides[3].Chart = nil
	slides[3].Points = nil
	slides[3].HasImage = false
	slides[3].Source = "口径：以上线前4周为基线，对比试点8周；样本为3个试点部门"
	slides[3].Subtitle = "价值体现在效率提升、执行透明与知识沉淀"

	slides[4].Title = "典型场景"
	slides[4].Layout = "content"
	slides[4].HasImage = false
	slides[4].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "项目协同", Detail: "里程碑风险与待办统一推进"},
		{Heading: "销售支持", Detail: "线索报价审批在线闭环"},
		{Heading: "总部支撑", Detail: "公告培训统一触达并回收反馈"},
	}, 3)
	slides[4].Points = nil
	slides[4].Metrics = nil
	slides[4].Chart = nil
	slides[4].Subtitle = "从高频跨部门场景切入最容易形成示范效果"

	slides[5].Title = "落地路径"
	slides[5].Layout = "content"
	slides[5].HasImage = false
	slides[5] = normalizeActionSlide(slides[5])
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "两周诊断", Detail: "业务负责人和IT锁定试点范围"},
		{Heading: "八周试点", Detail: "上线三类高频场景并完成培训"},
		{Heading: "月度复盘", Detail: "按活跃效率满意度决定扩面"},
	}, 3)
	slides[5].Points = nil
	slides[5].Metrics = nil
	slides[5].Chart = nil
	slides[5].Subtitle = "按试点到推广分阶段推进，降低风险并验证成效"
}

func enforceMarketSkeleton(slides []officegen.Slide) {
	if len(slides) < 6 {
		return
	}
	for idx := 1; idx < len(slides); idx++ {
		slides[idx].HasImage = false
		slides[idx].ImagePrompt = ""
		slides[idx].ImagePos = ""
	}

	slides[1].Title = "关键结论"
	slides[1].Layout = "content"
	slides[1].Points = normalizePoints([]string{
		"优先市场：先打北美，再复制到西欧与发达亚太",
		"竞争核心：胜负在入口、分发与合规，不只在模型",
		"阶段目标：九十天先验证付费，不做全面铺区",
	}, 3, 28)
	slides[1].Sections = nil
	if slides[1].Subtitle == "" {
		slides[1].Subtitle = "先攻英语市场，再复制到欧洲与发达亚太"
	}

	slides[2].Title = "市场空间"
	slides[2].Layout = "chart"
	slides[2] = normalizeEvidenceSlide(slides[2])
	slides[2].Chart = normalizeChart(&officegen.ChartData{Type: "bar", Title: "区域需求指数（北美=100）", Categories: []string{"北美", "欧洲", "亚太"}, Values: []float64{100, 72, 58}})
	slides[2].Source = "口径：北美=100 的相对需求指数，公开资料整理"
	if slides[2].Subtitle == "" {
		slides[2].Subtitle = "北美规模领先，欧洲与发达亚太构成第二梯队"
	}
	slides[2].Points = normalizePoints([]string{
		"指数口径以北美=100，便于比较进入优先级",
		"先验证高付费英语市场，再复制到合规更重区域",
	}, 2, 28)

	slides[3].Title = "区域机会"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{{Heading: "北美", Detail: "付费最强，先验证单客经济模型"}, {Heading: "西欧", Detail: "预算稳定，但必须先补齐合规"}, {Heading: "亚太", Detail: "英语环境成熟，适合复制北美打法"}}, 3)
	slides[3].Points = nil
	if slides[3].Subtitle == "" {
		slides[3].Subtitle = "区域选择必须围绕付费、合规和复制效率分层推进"
	}

	slides[4].Title = "竞争格局"
	slides[4].Layout = "content"
	slides[4].Points = normalizePoints([]string{
		"入口：微软谷歌占默认入口，新产品不宜正面比拼",
		"分发：独立工具靠单点体验与内容口碑持续获客",
		"空位：跨应用工作流、本地化模板和渠道联运仍有空位",
	}, 3, 30)
	slides[4].Sections = nil
	if slides[4].Subtitle == "" {
		slides[4].Subtitle = "从入口、分发和空位三维判断竞争格局"
	}

	slides[5].Title = "进入建议"
	slides[5].Layout = "content"
	slides[5] = normalizeActionSlide(slides[5])
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "六周MVP", Detail: "产品负责人上线英文版，完成十家试用"},
		{Heading: "八周试销", Detail: "增长负责人跑通投放和渠道联运"},
		{Heading: "九十天复盘", Detail: "管理层按回本和留存决定扩区"},
	}, 3)
	slides[5].Points = nil
	slides[5].Points = nil
	if slides[5].Subtitle == "" {
		slides[5].Subtitle = "九十天内完成市场验证并锁定首个样板客户"
	}
}

func enforceOpsSkeleton(slides []officegen.Slide) {
	if len(slides) < 6 {
		return
	}
	for idx := 1; idx < len(slides); idx++ {
		slides[idx].HasImage = false
		slides[idx].ImagePrompt = ""
		slides[idx].ImagePos = ""
	}

	slides[1].Title = "经营结论"
	slides[1].Layout = "content"
	slides[1].Points = normalizePoints([]string{
		"ARR指数升至128，增长主要由新增拉动",
		"续费率84、回款率76，质量指标明显落后",
		"下季先抓续费修复、交付提效与现金回收",
	}, 3, 28)
	slides[1].Sections = nil
	slides[1].Metrics = nil
	slides[1].Chart = nil
	slides[1].Source = ""
	slides[1].Subtitle = "新增拉动增长，但续费、交付与回款拖慢质量改善"

	slides[2].Title = "核心指标"
	slides[2].Layout = "chart"
	slides[2].Chart = normalizeChart(&officegen.ChartData{
		Type:       "bar",
		Title:      "季度经营关键指标（上季=100）",
		Categories: []string{"新增ARR", "续费率", "回款率"},
		Values:     []float64{128, 84, 76},
	})
	slides[2].Points = normalizePoints([]string{
		"新增ARR指数128，说明本季增长仍由拉新驱动",
		"续费率84、回款率76，质量指标明显落后于新增",
	}, 2, 28)
	slides[2].Sections = nil
	slides[2].Metrics = nil
	slides[2].Source = "口径：以上季=100 的相对指数；续费率和回款率按季度目标完成度折算"
	slides[2].Subtitle = "新增仍在拉动增长，但续费与回款拖慢质量改善"

	slides[3].Title = "问题定位"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "P1交付产能", Detail: "非标占比42%，项目周期拉长约10天"},
		{Heading: "P2回款节奏", Detail: "前10大客户账期偏长，现金回笼偏慢"},
		{Heading: "P3获客转化", Detail: "中段商机赢率低于目标7个百分点"},
	}, 3)
	slides[3].Points = nil
	slides[3].Metrics = nil
	slides[3].Chart = nil
	slides[3].Source = ""
	slides[3].Subtitle = "P1交付、P2回款、P3转化，三处卡点共同拉低经营质量"

	slides[4].Title = "下季重点"
	slides[4].Layout = "content"
	slides[4].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "续费修复", Detail: "客户成功负责人，续费率回到90以上"},
		{Heading: "交付提效", Detail: "交付负责人，非标占比压到30以内"},
		{Heading: "回款攻坚", Detail: "销售运营牵头，回款率提升到90"},
	}, 3)
	slides[4].Points = nil
	slides[4].Metrics = nil
	slides[4].Chart = nil
	slides[4].Source = ""
	slides[4].Subtitle = "每项重点都绑定负责人和结果指标，避免只写方向"

	slides[5].Title = "执行动作"
	slides[5].Layout = "content"
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "4月销售总监", Detail: "完成漏斗复盘，赢率提升3个百分点"},
		{Heading: "5月交付负责人", Detail: "上线标准包，返工率降到10以内"},
		{Heading: "6月经营负责人", Detail: "按续费率90与回款率90做验收"},
	}, 3)
	slides[5].Points = nil
	slides[5].Metrics = nil
	slides[5].Chart = nil
	slides[5].Source = ""
	slides[5].Subtitle = "按月推进并绑定负责人、里程碑与验收指标"
}

func enforceTrainingSkeleton(slides []officegen.Slide) {
	if len(slides) < 6 {
		return
	}
	for idx := 1; idx < len(slides); idx++ {
		slides[idx].HasImage = false
		slides[idx].ImagePrompt = ""
		slides[idx].ImagePos = ""
	}

	slides[1].Title = "学习目标"
	slides[1].Layout = "content"
	slides[1].Points = normalizePoints([]string{
		"理解 OfficeCLI 用途、输入输出和常见场景",
		"完成安装登录并跑通一次本地生成命令",
		"知道正式使用前的配置边界与注意事项",
	}, 3, 28)
	slides[1].Sections = nil
	slides[1].Metrics = nil
	slides[1].Chart = nil
	slides[1].Source = ""
	slides[1].Subtitle = "先建立认知，再完成首轮可独立操作的命令演练"

	slides[2].Title = "安装配置"
	slides[2].Layout = "content"
	slides[2].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "环境检查", Detail: "确认 Go、配置文件与本地依赖可用"},
		{Heading: "安装命令", Detail: "执行构建或下载命令，生成可运行程序"},
		{Heading: "登录验证", Detail: "完成配置后运行状态命令检查连通性"},
	}, 3)
	slides[2].Points = nil
	slides[2].Metrics = nil
	slides[2].Chart = nil
	slides[2].Source = ""
	slides[2].Subtitle = "按环境检查、安装、登录三步完成准备"

	slides[3].Title = "常用命令"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "状态检查", Detail: "运行 config status 检查配置与依赖"},
		{Heading: "生成PPT", Detail: "运行 new pptx 生成本地 PPT 文件"},
		{Heading: "质量评审", Detail: "运行 review pptx 做结构和视觉评审"},
	}, 3)
	slides[3].Points = nil
	slides[3].Metrics = nil
	slides[3].Chart = nil
	slides[3].Source = ""
	slides[3].Subtitle = "先记住最常用的三类命令，再逐步扩展场景"

	slides[4].Title = "示例流程"
	slides[4].Layout = "content"
	slides[4].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "步骤1 设题", Detail: "先写主题、受众、风格，明确输出目标"},
		{Heading: "步骤2 生成", Detail: "运行 new pptx 并确认文件落盘成功"},
		{Heading: "步骤3 复核", Detail: "运行 review pptx 并按问题继续修正"},
	}, 3)
	slides[4].Points = nil
	slides[4].Metrics = nil
	slides[4].Chart = nil
	slides[4].Source = ""
	slides[4].Subtitle = "一次完整演练要覆盖生成、检查和修正三个环节"

	slides[5].Title = "注意事项"
	slides[5].Layout = "content"
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "先本地验证", Detail: "默认关闭发布，先确认生成结果与评分"},
		{Heading: "配置要完整", Detail: "模型、图片和依赖缺失会直接影响效果"},
		{Heading: "命令要规范", Detail: "命令、路径和参数保持完整，避免截断"},
	}, 3)
	slides[5].Points = nil
	slides[5].Metrics = nil
	slides[5].Chart = nil
	slides[5].Source = ""
	slides[5].Subtitle = "先用本地输出验证质量，再进入正式协作流程"
}

func isActionSlide(slide officegen.Slide) bool {
	text := strings.TrimSpace(slide.Title + " " + slide.Subtitle)
	for _, keyword := range []string{"建议", "下一步", "落地", "计划", "发布", "培训", "路径", "行动"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func pointToSection(point string, idx int) (string, string) {
	cleaned := cleanSentence(point)
	if cleaned == "" {
		return "", ""
	}
	for _, marker := range []string{"30 天内", "60 天内", "90 天内", "第1-2周", "第3-6周", "第7-10周", "本周", "本月"} {
		if strings.HasPrefix(cleaned, marker) {
			return fitTextForLayout(marker, 10), fitTextForLayout(strings.TrimSpace(strings.TrimPrefix(cleaned, marker)), 24)
		}
	}
	for _, sep := range []string{"：", ":"} {
		parts := strings.SplitN(cleaned, sep, 2)
		if len(parts) != 2 {
			continue
		}
		label := fitTextForLayout(strings.TrimSpace(parts[0]), 10)
		body := fitTextForLayout(strings.TrimSpace(parts[1]), 24)
		if label != "" && body != "" {
			return label, body
		}
	}
	return fmt.Sprintf("步骤%d", idx+1), fitTextForLayout(cleaned, 24)
}

func fitTextForLayout(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	for _, sep := range []string{"。", "；", ";", "，", ",", "：", ":", "、", "（", "("} {
		parts := strings.SplitN(value, sep, 2)
		if len(parts) == 0 {
			continue
		}
		prefix := strings.TrimSpace(parts[0])
		if prefix == "" {
			continue
		}
		size := utf8.RuneCountInString(prefix)
		if size <= maxRunes && size >= maxRunes/2 {
			return prefix
		}
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes]))
}

func slideLayoutName(slide officegen.Slide) string {
	layout := strings.ToLower(strings.TrimSpace(slide.Layout))
	if layout != "" {
		return layout
	}
	if slide.IsTitle {
		return "title"
	}
	if len(slide.Metrics) > 0 {
		return "dashboard"
	}
	if slide.Chart != nil {
		return "chart"
	}
	return "content"
}

func cleanSentence(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"- ", "* ", "• ", "· ", "▪ ", "◦ "} {
		if strings.HasPrefix(value, prefix) {
			value = strings.TrimSpace(strings.TrimPrefix(value, prefix))
			break
		}
	}
	if idx := numericBulletPrefixEnd(value); idx > 0 {
		value = strings.TrimSpace(value[idx:])
	}
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "。")
	value = strings.TrimSuffix(value, "；")
	value = strings.TrimSuffix(value, ";")
	return value
}

func numericBulletPrefixEnd(value string) int {
	value = strings.TrimLeft(value, " \t")
	offset := 0
	for offset < len(value) && value[offset] >= '0' && value[offset] <= '9' {
		offset++
	}
	if offset == 0 || offset >= len(value) {
		return 0
	}
	switch {
	case strings.HasPrefix(value[offset:], "."):
		offset++
	case strings.HasPrefix(value[offset:], ")"):
		offset++
	case strings.HasPrefix(value[offset:], "、"):
		offset += len("、")
	case strings.HasPrefix(value[offset:], "）"):
		offset += len("）")
	default:
		return 0
	}
	if offset >= len(value) {
		return 0
	}
	if offset >= len(value) || (value[offset] != ' ' && value[offset] != '\t') {
		return 0
	}
	for offset < len(value) && (value[offset] == ' ' || value[offset] == '\t') {
		offset++
	}
	return offset
}

func shouldDowngradeChart(slide officegen.Slide) bool {
	if slide.Chart == nil {
		return false
	}
	text := strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Chart.Title)
	for _, keyword := range []string{"里程碑", "节奏", "计划", "路线", "步骤", "流程", "风险", "下一步"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func downgradeChartSlide(slide officegen.Slide) officegen.Slide {
	chart := slide.Chart
	slide.Layout = "content"
	slide.Chart = nil
	if chart == nil {
		return slide
	}
	sections := make([]officegen.SlideSection, 0, len(chart.Categories))
	for idx, category := range chart.Categories {
		heading := fitTextForLayout(cleanSentence(category), 12)
		if heading == "" {
			continue
		}
		detail := fmt.Sprintf("第%d步", idx+1)
		if len(chart.Values) > idx && chart.Values[idx] > 0 {
			detail = fmt.Sprintf("阶段值 %s", formatChartValue(chart.Values[idx]))
		}
		if strings.Contains(slide.Title, "里程碑") || strings.Contains(slide.Title, "节奏") || strings.Contains(slide.Title, "计划") {
			detail = fmt.Sprintf("第%d阶段，依次推进", idx+1)
		}
		sections = append(sections, officegen.SlideSection{
			Heading: heading,
			Detail:  detail,
		})
	}
	slide.Sections = normalizeSections(sections, 4)
	if len(slide.Points) == 0 {
		slide.Points = deriveChartPoints(chart, 2)
	}
	return slide
}

func trimRunes(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return strings.TrimSpace(string(runes[:maxRunes-3])) + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func formatChartValue(value float64) string {
	if float64(int64(value)) == value {
		return fmt.Sprintf("%d", int64(value))
	}
	return fmt.Sprintf("%.1f", value)
}
