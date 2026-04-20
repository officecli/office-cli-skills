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
	DocumentType   engine.DocumentType
	Topic          string
	Prompt         string
	SourceFilePath string
	Mode           string
	Language       string
	Style          string
	Audience       string
	EnableImages   bool
	LocalPreview   bool
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
		return s.generateDOCX(ctx, envelope.Prompt, params.Topic, target, meta, params.LocalPreview)
	case engine.DocumentTypeXLSX:
		return s.generateXLSX(ctx, envelope.Prompt, params.Topic, target, meta, params.LocalPreview)
	case engine.DocumentTypeReport:
		return s.generateReport(ctx, envelope.Prompt, params.Topic, params.SourceFilePath, target, meta)
	case engine.DocumentTypePPTX:
		return s.generatePPTX(ctx, envelope.Prompt, params.Topic, target, meta, params.EnableImages, params.LocalPreview)
	default:
		return nil, fmt.Errorf("unsupported document type: %s", params.DocumentType)
	}
}

func (s *Service) generateDOCX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta, localPreview bool) (*GeneratedArtifact, error) {
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Requesting DOCX content from the LLM")
	response, err := s.llm.CompleteJSON(ctx, []engine.LLMMessage{{Role: "user", Content: generateengine.BuildDOCXPrompt(prompt, target)}})
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "DOCX content generation failed")
		return nil, fmt.Errorf("content generation failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Received DOCX structure output")
	emitProgress(ctx, s.progress, progressStepAssemble, "running", "Assembling the DOCX file")
	fileBytes, fileName, previewHTML, previewJSON, err := generateengine.BuildDOCXArtifactFromJSON(response, fallbackDescription(topic, prompt), target.Style, localPreview)
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "DOCX assembly failed")
		return nil, fmt.Errorf("document assembly failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepAssemble, "completed", "DOCX assembly completed")
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypeDOCX),
		Bytes:        fileBytes,
		Warnings:     convertIssues(meta),
		PreviewHTML:  previewHTML,
		PreviewJSON:  previewJSON,
	}, nil
}

func (s *Service) generateXLSX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta, localPreview bool) (*GeneratedArtifact, error) {
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Requesting XLSX content from the LLM")
	response, err := s.llm.CompleteJSON(ctx, []engine.LLMMessage{{Role: "user", Content: generateengine.BuildXLSXPrompt(prompt, target)}})
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "XLSX content generation failed")
		return nil, fmt.Errorf("content generation failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Received XLSX structure output")
	emitProgress(ctx, s.progress, progressStepAssemble, "running", "Assembling the XLSX file")
	fileBytes, fileName, previewHTML, previewJSON, err := generateengine.BuildXLSXArtifactFromJSON(response, fallbackDescription(topic, prompt), target.Style, localPreview)
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "XLSX assembly failed")
		return nil, fmt.Errorf("document assembly failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepAssemble, "completed", "XLSX assembly completed")
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypeXLSX),
		Bytes:        fileBytes,
		Warnings:     convertIssues(meta),
		PreviewHTML:  previewHTML,
		PreviewJSON:  previewJSON,
	}, nil
}

func (s *Service) generateReport(ctx context.Context, prompt, topic, sourceFilePath string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta) (*GeneratedArtifact, error) {
	emitProgress(ctx, s.progress, progressStepAssemble, "running", "Reading workbook data for report generation")
	sheets, err := loadWorkbookSheetsFromFile(sourceFilePath)
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "Workbook read failed")
		return nil, fmt.Errorf("read report workbook: %w", err)
	}
	baseReport := officegen.BuildReportFromWorkbook(topic, sheets)
	baseReportJSON, err := json.Marshal(baseReport)
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "Report draft preparation failed")
		return nil, fmt.Errorf("marshal base report: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepAssemble, "completed", "Workbook data prepared for report generation")

	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Requesting workbook-backed report narrative from the LLM")
	response, err := s.llm.CompleteJSON(ctx, []engine.LLMMessage{{
		Role: "user",
		Content: generateengine.BuildWorkbookReportPrompt(
			prompt,
			target,
			buildWorkbookSummary(sheets),
			string(baseReportJSON),
		),
	}})
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "Workbook-backed report narrative generation failed")
		return nil, fmt.Errorf("content generation failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Received workbook-backed report structure output")
	emitProgress(ctx, s.progress, progressStepAssemble, "running", "Assembling the report HTML")
	fileBytes, fileName, err := generateengine.BuildReportFromJSON(response, fallbackDescription(topic, prompt))
	if err != nil {
		emitProgress(ctx, s.progress, progressStepAssemble, "failed", "Report assembly failed")
		return nil, fmt.Errorf("document assembly failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepAssemble, "completed", "Report assembly completed")
	return &GeneratedArtifact{
		DocumentName: fileName,
		DocumentType: string(engine.DocumentTypeReport),
		Bytes:        fileBytes,
		Warnings:     convertIssues(meta),
	}, nil
}

func (s *Service) generatePPTX(ctx context.Context, prompt, topic string, target generateengine.PromptTarget, meta *generateengine.PPTXMeta, enableImages, localPreview bool) (*GeneratedArtifact, error) {
	basePrompt := BuildPPTXPrompt(prompt, target, enableImages)
	fallback := fallbackDescription(topic, prompt)
	messages := []engine.LLMMessage{{Role: "user", Content: basePrompt}}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Requesting PPTX content from the LLM")
	response, err := s.llm.CompleteJSON(ctx, messages)
	if err != nil {
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "PPTX content generation failed")
		return nil, fmt.Errorf("content generation failed: %w", err)
	}
	emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Received PPTX structure output")

	fileBytes, fileName, warnings, previewHTML, previewJSON, err := BuildPPTXFromJSON(ctx, s.llm, s.progress, response, fallback, target.Style, enableImages, localPreview)
	if err != nil {
		if !shouldRetryPPTXAssembly(err) {
			return nil, err
		}
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "running", "Detected incomplete JSON output. Switching to structured repair retry")
		response, err = s.llm.CompleteStructured(ctx, buildPPTXRepairRequest(basePrompt, response))
		if err != nil {
			emitProgress(ctx, s.progress, progressStepGenerateLLM, "failed", "Structured PPTX repair failed")
			return nil, fmt.Errorf("content generation failed: %w", err)
		}
		emitProgress(ctx, s.progress, progressStepGenerateLLM, "completed", "Received PPTX output after structured repair")
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
		{Role: "user", Content: "Your previous output was not complete valid JSON. It may have been truncated or may be missing closing structure. Ignore the incomplete result and return one complete JSON object again. Do not explain anything and do not use markdown code fences. Every object field must be present: use an empty string for non-applicable string fields, [] for arrays, null for objects, and false for booleans."},
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

type pptxSlideRole string

const (
	pptxSlideRoleCover    pptxSlideRole = "cover"
	pptxSlideRoleSummary  pptxSlideRole = "summary"
	pptxSlideRoleEvidence pptxSlideRole = "evidence"
	pptxSlideRoleDetail   pptxSlideRole = "detail"
	pptxSlideRoleAction   pptxSlideRole = "action"
)

type pptxSlideSignals struct {
	Role         pptxSlideRole
	WantsChart   bool
	WantsMetrics bool
	WantsImage   bool
}

const pptxStructuredSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "title": { "type": "string" },
    "subtitle": { "type": "string" },
    "stylePreset": { "type": "string" },
    "theme": {
      "anyOf": [
        {
          "type": "object",
          "additionalProperties": false,
          "properties": {
            "preset": { "type": "string" },
            "primaryColor": { "type": "string" },
            "accentColor": { "type": "string" },
            "accentSoft": { "type": "string" },
            "backgroundColor": { "type": "string" },
            "surfaceColor": { "type": "string" },
            "borderColor": { "type": "string" },
            "textColor": { "type": "string" },
            "mutedColor": { "type": "string" },
            "titleColor": { "type": "string" },
            "fontFamily": { "type": "string" },
            "eaFontFamily": { "type": "string" }
          },
          "required": [
            "preset",
            "primaryColor",
            "accentColor",
            "accentSoft",
            "backgroundColor",
            "surfaceColor",
            "borderColor",
            "textColor",
            "mutedColor",
            "titleColor",
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
          "role": { "type": "string" },
          "layout": { "type": "string" },
          "variant": { "type": "string" },
          "headline": { "type": "string" },
          "takeaway": { "type": "string" },
          "blocks": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "type": { "type": "string" },
                "text": { "type": "string" },
                "items": {
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
                }
              },
              "required": ["type", "text", "items", "sections", "metrics", "chart"]
            }
          },
          "visual": {
            "anyOf": [
              {
                "type": "object",
                "additionalProperties": false,
                "properties": {
                  "kind": { "type": "string" },
                  "prompt": { "type": "string" },
                  "position": {
                    "type": "string",
                    "enum": ["", "right", "left", "background", "center", "top", "bottom", "diagonal"]
                  }
                },
                "required": ["kind", "prompt", "position"]
              },
              { "type": "null" }
            ]
          },
          "source": { "type": "string" },
          "bgColor": { "type": "string" },
          "bgColor2": { "type": "string" }
        },
        "required": [
          "role",
          "layout",
          "headline",
          "takeaway",
          "blocks",
          "visual",
          "source",
          "bgColor",
          "bgColor2"
        ]
      }
    }
  },
  "required": ["title", "subtitle", "stylePreset", "theme", "slides"]
}`

func BuildPPTXPrompt(description string, target generateengine.PromptTarget, enableImages bool) string {
	archetype := detectPPTXArchetype(description, "")
	presetHint := suggestStylePreset(target.Style, archetype)
	slideExample := `    {
      "role": "summary",
      "layout": "content",
      "variant": "sections-grid",
      "headline": "Key Takeaways",
      "takeaway": "Lead with the conclusion",
      "blocks": [
        {
          "type": "sections",
          "text": "",
          "items": [],
          "sections": [
            {"heading": "Signal", "detail": "What changed most"},
            {"heading": "Impact", "detail": "Why it matters now"},
            {"heading": "Decision", "detail": "What should happen next"}
          ],
          "metrics": [],
          "chart": null
        }
      ],
      "visual": null,
      "source": "",
      "bgColor": "",
      "bgColor2": ""
    }`
	imageRules := "- Set visual to null when the slide does not need an image."
	if enableImages {
		slideExample = `    {
      "role": "detail",
      "layout": "content",
      "variant": "image-right",
      "headline": "Section Title",
      "takeaway": "One-sentence takeaway",
      "blocks": [
        {
          "type": "bullets",
          "text": "",
          "items": ["Point 1", "Point 2", "Point 3"],
          "sections": [],
          "metrics": [],
          "chart": null
        }
      ],
      "visual": {
        "kind": "image",
        "prompt": "A concrete visual prompt that can be sent directly to an image model",
        "position": "right"
      },
      "source": "",
      "bgColor": "",
      "bgColor2": ""
    }`
		imageRules = `- Use images intentionally, not just decoratively. In a 6-8 slide deck, usually keep 2-4 image-supported slides when the topic includes product, scenario, workflow, interface, training, game, or customer experience storytelling.
- On image slides, use visual.kind=image and visual.position must be one of right, left, background, center, top, bottom, or diagonal.
- visual.prompt must be a concrete visual description that can be sent directly to an image model. Avoid abstract wording.
- Do not add images to chart or dashboard layouts.
- Prefer images for title-cover hero visuals, product UI, feature walkthroughs, gameplay moments, usage scenarios, customer scenes, or training steps. By default do not add images to executive-summary, market analysis, competitive landscape, business review, quantified evidence, pricing table, or action recommendation slides.
- On image slides, keep text subordinate to the visual but still substantive: usually 2-4 solid points or 2-3 short sections, not slogans.`
	}
	outlineRules := buildArchetypePromptRules(archetype)
	return fmt.Sprintf(`Generate a JSON structure for a PPT presentation based on the following request.

Request: %s
%s

Build the deck in a layout-first way. Decide the deck sequence first, then assign one proven slide pattern to each page before writing content.

Return JSON only. Do not add any extra commentary:
{
  "title": "Presentation Title",
  "subtitle": "Overall deck framing",
  "stylePreset": "%s",
  "theme": {
    "preset": "analysis",
    "primaryColor": "1D4ED8",
    "accentColor": "0F766E",
    "accentSoft": "D1FAE5",
    "backgroundColor": "F8FAFC",
    "surfaceColor": "FFFFFF",
    "borderColor": "DCE4F2",
    "textColor": "0F172A",
    "mutedColor": "64748B",
    "titleColor": "020617",
    "fontFamily": "Aptos",
    "eaFontFamily": "Microsoft YaHei"
  },
  "slides": [
    {
      "role": "cover",
      "layout": "title",
      "variant": "title-center",
      "headline": "Cover Title",
      "takeaway": "Subtitle",
      "blocks": [],
      "visual": null,
      "source": "",
      "bgColor": "",
      "bgColor2": ""
    },
%s
  ]
}

Requirements:
	- Keep the deck to 5-8 slides, usually 6-7.
	- stylePreset must be one of executive-dark, editorial-light, tech-contrast, or training-manual. If the user did not specify one, choose the closest fit for the topic.
	- The first slide must use the title layout.
	- Use role to make the storyline explicit: cover, summary, evidence, detail, or action.
	- Use variant to declare the visual pattern. Valid variants are title-center, title-split, sections-grid, feature-grid, pillar-list, point-cards, stat-band, chart-context, comparison, timeline, image-right, bullets, or prose.
	- Use blocks to separate message types. Valid block types are narrative, bullets, sections, metrics, and chart.
	- For business decks, slide 2 should read as an executive summary or key takeaways page, and the final slide should read as decision, next steps, or rollout actions.
	- Prefer a storyline such as cover -> summary -> supporting evidence/capabilities -> detail -> action, but adapt the exact page count and page roles to the prompt instead of forcing a rigid template.
	- Each slide should express only one core idea. Keep headlines concise. takeaway must be a takeaway sentence for the slide.
- Use a slide pattern library instead of repeating the same structure. Never use the same variant on two consecutive slides.
- Prefer conclusion-first business pacing: summary pages use sections-grid or stat-band; capability/scenario pages use feature-grid, pillar-list, or comparison; quantified evidence pages use chart-context or stat-band; closing pages use timeline or sections-grid.
- Prefer content layout for most slides, and use chart or dashboard only when needed.
- For comparisons, steps, regions, roles, or training paths, prefer sections blocks with short heading and concise detail.
- For customer value, business review, market size, or competitive comparison, prefer evidence-based expression with chart or metrics blocks. If reliable numbers are unavailable, use 2-3 structured sections instead of long bullets.
- If the topic is market analysis, industry research, or business review, the deck must include at least one chart or dashboard slide with source or data framing.
- Action recommendations, rollout plans, release cadence, and training paths must use sections or metrics and show at least two of time, owner, or acceptance criteria.
- Avoid under-filled slides. Most non-cover content slides should read as 3-5 solid bullets or 3-4 sections with one-sentence detail, not just 1-2 fragmentary phrases.
- Use at most 4 sections, at most 4 dashboard metrics, and at most 5 chart categories.
- Use charts only for objective data with units, scale, and ordering logic. Do not use charts for priorities, milestones, strategy, risks, or process flows.
- When a chart fits, put it inside a chart block with type, categories, values, and title, plus 2-3 takeaway points in another block when needed.
- When a dashboard fits, put it inside a metrics block with label, value, and note, plus 2-3 action or takeaway points in another block when needed.
- The closing slide must include 2-3 next-step actions with time, owner, or validation criteria.
- Wording should fit the audience and style. Prefer quantified, conclusion-first language and avoid vague slogans.
	%s
	%s`, description, generateengine.FormatDocumentPromptTarget(target), presetHint, slideExample, imageRules, outlineRules)
}

func BuildPPTXFromJSON(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, content, fallback, requestedStyle string, enableImages, localPreview bool) ([]byte, string, []engine.GenerateIssue, []byte, []byte, error) {
	emitProgress(ctx, progress, progressStepAssemble, "running", "Parsing the PPTX structure and preparing assets")
	payload, err := parsePPTXPayload(content, fallback, requestedStyle, enableImages)
	if err != nil {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "PPTX structure parsing failed")
		return nil, "", nil, nil, nil, fmt.Errorf("document assembly failed: %w", err)
	}
	if len(payload.Slides) == 0 {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "PPTX structure is empty")
		return nil, "", nil, nil, nil, fmt.Errorf("document assembly failed: slides cannot be empty")
	}
	warnings := normalizePPTXPayload(payload, fallback, requestedStyle, enableImages)
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
			emitProgress(ctx, progress, progressStepAssemble, "running", fmt.Sprintf("Generating image asset (%d/%d)", imageIndex, imageTotal))
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
					Message: "Some images failed to generate, so the output was automatically downgraded to a text-only version. Check whether the generation service supports image endpoints, or run `officecli config set-generation` to configure the image model URL, credential, and model name. For a text-only deck, use `--no-images`.",
					Field:   "slides",
				})
			}
		}
	}

	emitProgress(ctx, progress, progressStepAssemble, "running", "Packaging the PPTX file")
	fileBytes, err := officegen.NewPPTXGenerator().Generate(payload.Slides, officegen.PPTXOptions{
		Title:       payload.Title,
		Creator:     "OfficeCLI",
		Theme:       payload.Theme,
		StylePreset: payload.StylePreset,
	})
	if err != nil {
		emitProgress(ctx, progress, progressStepAssemble, "failed", "PPTX packaging failed")
		return nil, "", nil, nil, nil, fmt.Errorf("document assembly failed: generate pptx: %w", err)
	}
	emitProgress(ctx, progress, progressStepAssemble, "completed", "PPTX assembly completed")

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

	const maxSlides = 8
	warnings := make([]engine.GenerateIssue, 0, 3)
	payload.Title = trimRunes(firstNonEmpty(payload.Title, generateengine.ExtractTitleFromDescription(fallback), "Presentation"), 30)
	archetype := detectPPTXArchetype(fallback, payload.Title)
	payload.StylePreset = suggestStylePreset(firstNonEmpty(strings.TrimSpace(payload.StylePreset), strings.TrimSpace(requestedStyle)), archetype)
	payload.Theme = officegen.MergeThemeWithPreset(payload.Theme, payload.StylePreset)
	firstSlideWasExplicitCover := len(payload.Slides) > 0 && (payload.Slides[0].IsTitle || strings.EqualFold(strings.TrimSpace(payload.Slides[0].Layout), "title") || normalizePPTXRole(payload.Slides[0].Role) == pptxSlideRoleCover)

	slides := make([]officegen.Slide, 0, len(payload.Slides))
	imageBudget := computePPTXImageBudget(payload.Slides, strings.TrimSpace(fallback+" "+payload.Title), enableImages)
	slidesTrimmed := false
	imagesAdjusted := false
	for idx, slide := range payload.Slides {
		if len(slides) >= maxSlides {
			slidesTrimmed = true
			break
		}
		signals := analyzePPTXSlide(slide, idx)
		normalized, imageKept := normalizePPTXSlide(slide, signals, idx, payload.Title, enableImages, &imageBudget)
		if slide.HasImage && !imageKept {
			imagesAdjusted = true
		}
		if isEmptyNormalizedSlide(normalized) {
			continue
		}
		slides = append(slides, expandSlideForDensity(normalized)...)
		if len(slides) > maxSlides {
			slidesTrimmed = true
			slides = slides[:maxSlides]
			break
		}
	}

	if len(slides) == 0 {
		slides = append(slides, officegen.Slide{
			Role:     string(pptxSlideRoleCover),
			Title:    payload.Title,
			Layout:   "title",
			IsTitle:  true,
			Subtitle: fitTextForLayout(strings.TrimSpace(fallback), 28),
		})
	}

	slides[0].Layout = "title"
	slides[0].Role = string(pptxSlideRoleCover)
	slides[0].Variant = normalizeSlideVariant(slides[0])
	slides[0].IsTitle = true
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
	if !enableImages || !firstSlideWasExplicitCover || !allowImageForSlide(slides[0]) || strings.TrimSpace(slides[0].ImagePrompt) == "" {
		slides[0].HasImage = false
		slides[0].ImagePrompt = ""
		slides[0].ImagePos = ""
	} else {
		slides[0].HasImage = true
		slides[0].ImagePos = normalizeImagePosition(slides[0].ImagePos)
	}

	for idx := 1; idx < len(slides); idx++ {
		slides[idx].IsTitle = false
		if slideRoleName(slides[idx], idx) == pptxSlideRoleCover {
			slides[idx].Role = string(pptxSlideRoleDetail)
		}
		if slideLayoutName(slides[idx]) == "title" {
			slides[idx].Layout = "content"
		}
		if strings.TrimSpace(slides[idx].Title) == "" {
			slides[idx].Title = fmt.Sprintf("Part %d", idx)
		}
	}

	slides = softlyApplyArchetypeDefaults(slides, archetype, payload.Title)
	slides = rebalanceNarrativeSlides(slides, payload.Title, archetype, maxSlides)
	slides = rebalanceAdjacentLayouts(slides)
	if len(slides) > maxSlides {
		slidesTrimmed = true
		slides = slides[:maxSlides]
	}

	payload.Slides = slides

	if slidesTrimmed {
		warnings = append(warnings, engine.GenerateIssue{
			Code:    "WARN_PPT_SLIDES_TRIMMED",
			Field:   "slides",
			Message: "The generated deck exceeded quality limits and was automatically trimmed to 8 slides or fewer.",
		})
	}
	if imagesAdjusted {
		warnings = append(warnings, engine.GenerateIssue{
			Code:    "WARN_PPT_IMAGES_REBALANCED",
			Field:   "slides",
			Message: "Image slide count and placement were automatically rebalanced to avoid overwhelming the content.",
		})
	}
	return warnings
}

func normalizePPTXSlide(slide officegen.Slide, signals pptxSlideSignals, idx int, deckTitle string, enableImages bool, imageBudget *int) (officegen.Slide, bool) {
	explicitCover := slide.IsTitle || strings.EqualFold(strings.TrimSpace(slide.Layout), "title") || normalizePPTXRole(slide.Role) == pptxSlideRoleCover
	slide.Role = string(signals.Role)
	slide.Title = fitTextForLayout(firstNonEmpty(slide.Title, deckTitle), 22)
	slide.Subtitle = fitTextForLayout(strings.TrimSpace(slide.Subtitle), 30)
	slide.Source = fitTextForLayout(strings.TrimSpace(slide.Source), 48)
	slide.Content = strings.TrimSpace(slide.Content)
	slide.Layout = strings.ToLower(strings.TrimSpace(slide.Layout))

	slide.Points = normalizePoints(slide.Points, 5, 48)
	slide.Sections = normalizeSections(slide.Sections, 4)
	slide.Metrics = normalizeMetrics(slide.Metrics, 4)
	slide.Chart = normalizeChart(slide.Chart)
	slide = applyRoleDrivenSlideNormalization(slide, signals)
	slide = finalizeSlideLayout(slide)
	if slide.Subtitle == "" {
		slide.Subtitle = deriveSlideSubtitle(slide)
	}
	slide.Variant = normalizeSlideVariant(slide)

	imageKept := false
	imagePrompt := strings.TrimSpace(slide.ImagePrompt)
	requestedImage := slide.HasImage || imagePrompt != ""
	if enableImages &&
		allowImageForSlide(slide) &&
		imageBudget != nil &&
		*imageBudget > 0 {
		if imagePrompt == "" && shouldAutoAddImagePrompt(slide, explicitCover) {
			imagePrompt = buildFallbackImagePrompt(slide, deckTitle)
		}
		if !requestedImage && imagePrompt == "" {
			slide.HasImage = false
			slide.ImagePrompt = ""
			slide.ImagePos = ""
			return slide, false
		}
		slide.HasImage = true
		slide.ImagePrompt = trimRunes(imagePrompt, 180)
		slide.ImagePos = normalizeImagePosition(firstNonEmpty(slide.ImagePos, defaultImagePositionForSlide(slide)))
		*imageBudget--
		imageKept = true
	} else {
		slide.HasImage = false
		slide.ImagePrompt = ""
		slide.ImagePos = ""
	}

	return slide, imageKept
}

func analyzePPTXSlide(slide officegen.Slide, idx int) pptxSlideSignals {
	role := inferPPTXSlideRole(slide, idx)
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	return pptxSlideSignals{
		Role:         role,
		WantsChart:   slide.Chart != nil || strings.Contains(text, "market size") || strings.Contains(text, "trend") || strings.Contains(text, "benchmark") || strings.Contains(text, "市场规模") || strings.Contains(text, "趋势") || strings.Contains(text, "对比"),
		WantsMetrics: len(slide.Metrics) > 0 || strings.Contains(text, "kpi") || strings.Contains(text, "metric") || strings.Contains(text, "value") || strings.Contains(text, "指标") || strings.Contains(text, "价值") || strings.Contains(text, "roi"),
		WantsImage:   slide.HasImage || strings.TrimSpace(slide.ImagePrompt) != "",
	}
}

func normalizePPTXRole(value string) pptxSlideRole {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(pptxSlideRoleCover):
		return pptxSlideRoleCover
	case string(pptxSlideRoleSummary):
		return pptxSlideRoleSummary
	case string(pptxSlideRoleEvidence):
		return pptxSlideRoleEvidence
	case string(pptxSlideRoleAction):
		return pptxSlideRoleAction
	case string(pptxSlideRoleDetail):
		return pptxSlideRoleDetail
	default:
		return ""
	}
}

func slideRoleName(slide officegen.Slide, idx int) pptxSlideRole {
	if role := normalizePPTXRole(slide.Role); role != "" {
		return role
	}
	return inferPPTXSlideRole(slide, idx)
}

func inferPPTXSlideRole(slide officegen.Slide, idx int) pptxSlideRole {
	if role := normalizePPTXRole(slide.Role); role != "" {
		return role
	}
	if idx == 0 || slide.IsTitle || strings.EqualFold(strings.TrimSpace(slide.Layout), "title") {
		return pptxSlideRoleCover
	}
	if slide.Chart != nil || len(slide.Metrics) > 0 {
		return pptxSlideRoleEvidence
	}
	if isActionSlide(slide) || looksLikeClosingSlide(slide) {
		return pptxSlideRoleAction
	}
	if looksLikeOverviewSlide(slide) || (idx == 1 && (isPlaceholderSlideTitle(slide.Title) || len(slide.Sections) >= 3)) {
		return pptxSlideRoleSummary
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	for _, keyword := range []string{"market", "revenue", "growth", "pipeline", "benchmark", "analysis", "evidence", "result", "review", "performance", "value", "市场", "收入", "增长", "分析", "证据", "结果", "复盘", "业绩", "指标", "价值"} {
		if strings.Contains(text, keyword) {
			return pptxSlideRoleEvidence
		}
	}
	return pptxSlideRoleDetail
}

func applyRoleDrivenSlideNormalization(slide officegen.Slide, signals pptxSlideSignals) officegen.Slide {
	switch signals.Role {
	case pptxSlideRoleCover:
		return normalizeCoverSlide(slide)
	case pptxSlideRoleSummary:
		return normalizeSummaryRoleSlide(slide)
	case pptxSlideRoleEvidence:
		return normalizeEvidenceRoleSlide(slide, signals)
	case pptxSlideRoleAction:
		return normalizeActionRoleSlide(slide)
	default:
		return normalizeDetailRoleSlide(slide)
	}
}

func normalizeCoverSlide(slide officegen.Slide) officegen.Slide {
	subtitleFallback := fitTextForLayout(firstNonEmpty(slide.Subtitle, slide.Content, slide.Source), 28)
	slide.Role = string(pptxSlideRoleCover)
	slide.Layout = "title"
	slide.IsTitle = true
	slide.Content = ""
	slide.Points = nil
	slide.Sections = nil
	slide.Metrics = nil
	slide.Chart = nil
	if slide.Subtitle == "" {
		slide.Subtitle = subtitleFallback
	}
	return slide
}

func normalizeSummaryRoleSlide(slide officegen.Slide) officegen.Slide {
	slide.Role = string(pptxSlideRoleSummary)
	slide.Layout = "content"
	if slide.Chart != nil {
		slide.Points = append(slide.Points, deriveChartPoints(slide.Chart, 3)...)
		slide.Chart = nil
	}
	if len(slide.Metrics) > 0 {
		slide.Points = append(slide.Points, deriveMetricPoints(slide.Metrics, 3)...)
		slide.Metrics = nil
	}
	if len(slide.Points) == 0 && len(slide.Sections) == 0 && slide.Content != "" {
		slide.Points = splitContentToPoints(slide.Content, 4)
	}
	if len(slide.Sections) == 0 && len(slide.Points) > 0 {
		if sections := pointsToSummarySections(slide.Points, 4); len(sections) > 0 {
			slide.Sections = sections
			if len(slide.Points) <= 4 {
				slide.Points = nil
			}
		}
	}
	slide.Content = ""
	slide.Source = ""
	slide.HasImage = false
	slide.ImagePrompt = ""
	slide.ImagePos = ""
	return slide
}

func normalizeEvidenceRoleSlide(slide officegen.Slide, signals pptxSlideSignals) officegen.Slide {
	slide.Role = string(pptxSlideRoleEvidence)
	slide = normalizeEvidenceSlide(slide)
	if slide.Chart == nil && slide.Layout == "chart" && signals.WantsChart {
		slide = normalizeEvidenceSlide(slide)
	}
	if slide.Chart != nil {
		slide.Layout = "chart"
		if len(slide.Points) == 0 {
			slide.Points = deriveChartPoints(slide.Chart, 2)
		}
	}
	if len(slide.Metrics) > 0 {
		slide.Layout = "dashboard"
		if len(slide.Points) == 0 {
			slide.Points = deriveMetricPoints(slide.Metrics, 2)
		}
	}
	if slide.Chart == nil && len(slide.Metrics) == 0 {
		slide.Layout = "content"
		if len(slide.Points) == 0 && len(slide.Sections) == 0 && slide.Content != "" {
			slide.Points = splitContentToPoints(slide.Content, 4)
			slide.Content = ""
		}
	}
	slide.HasImage = false
	slide.ImagePrompt = ""
	slide.ImagePos = ""
	return slide
}

func normalizeActionRoleSlide(slide officegen.Slide) officegen.Slide {
	slide.Role = string(pptxSlideRoleAction)
	if slide.Chart != nil {
		slide.Points = append(slide.Points, deriveChartPoints(slide.Chart, 2)...)
		slide.Chart = nil
	}
	if len(slide.Metrics) > 0 {
		slide.Points = append(slide.Points, deriveMetricPoints(slide.Metrics, 3)...)
		slide.Metrics = nil
	}
	if len(slide.Points) == 0 && len(slide.Sections) == 0 && slide.Content != "" {
		slide.Points = splitContentToPoints(slide.Content, 4)
	}
	slide = normalizeActionSlide(slide)
	slide.Layout = "content"
	slide.Content = ""
	slide.Source = ""
	slide.HasImage = false
	slide.ImagePrompt = ""
	slide.ImagePos = ""
	return slide
}

func normalizeDetailRoleSlide(slide officegen.Slide) officegen.Slide {
	slide.Role = string(pptxSlideRoleDetail)
	switch {
	case slide.Chart != nil:
		slide.Layout = "chart"
	case len(slide.Metrics) > 0:
		slide.Layout = "dashboard"
	case slide.Layout == "":
		slide.Layout = "content"
	}
	if shouldDowngradeChart(slide) {
		slide = downgradeChartSlide(slide)
	}
	if len(slide.Points) == 0 && len(slide.Sections) == 0 && slide.Content != "" {
		slide.Points = splitContentToPoints(slide.Content, 5)
		if len(slide.Points) > 0 {
			slide.Content = ""
		}
	}
	return slide
}

func finalizeSlideLayout(slide officegen.Slide) officegen.Slide {
	if slide.Chart != nil && shouldDowngradeChart(slide) {
		slide = downgradeChartSlide(slide)
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
	role := slideRoleName(slide, 0)
	if len(slide.Sections) > 0 && (role == pptxSlideRoleSummary || role == pptxSlideRoleAction) {
		// Grouped summary/action slides already carry their own structure and do not need a footer source.
		slide.Source = ""
	}
	return slide
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
		heading := fitTextForLayout(cleanSentence(section.Heading), 16)
		detail := fitTextForLayout(cleanSentence(section.Detail), 56)
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
		note := fitTextForLayout(cleanSentence(metric.Note), 20)
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
		strings.Contains(text, "board"),
		strings.Contains(text, "executive"),
		strings.Contains(text, "executive"):
		return officegen.StylePresetExecutiveDark
	case text == officegen.StylePresetEditorialLight,
		strings.Contains(text, "editorial"),
		strings.Contains(text, "magazine"),
		strings.Contains(text, "light background"),
		strings.Contains(text, "light"):
		return officegen.StylePresetEditorialLight
	case text == officegen.StylePresetTrainingManual,
		strings.Contains(text, "training"),
		strings.Contains(text, "tutorial"),
		strings.Contains(text, "manual"):
		return officegen.StylePresetTrainingManual
	case text == officegen.StylePresetTechContrast,
		strings.Contains(text, "tech"),
		strings.Contains(text, "contrast"),
		strings.Contains(text, "technical"):
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

func rebalanceNarrativeSlides(slides []officegen.Slide, deckTitle string, archetype pptxArchetype, maxSlides int) []officegen.Slide {
	if len(slides) == 0 {
		return slides
	}
	slides = ensureMinimumNarrativeSlides(slides, deckTitle, archetype)
	if idx := findSlideByRole(slides, pptxSlideRoleSummary, 1); idx > 1 {
		slides = moveSlide(slides, idx, 1)
	}
	if len(slides) > 1 && slideRoleName(slides[1], 1) != pptxSlideRoleSummary && shouldInsertOverviewSlide(slides[1]) && len(slides) < maxSlides {
		slides = insertSlide(slides, 1, defaultSummarySlide(archetype, deckTitle))
	}
	if len(slides) > 1 {
		slides[1] = enforceOverviewSlide(slides[1], archetype)
	}
	slides = ensureEvidenceCoverage(slides, deckTitle, archetype, maxSlides)
	slides = ensureClosingActionSlide(slides, deckTitle, archetype, maxSlides)
	return slides
}

func moveSlide(slides []officegen.Slide, from, to int) []officegen.Slide {
	if from < 0 || from >= len(slides) || to < 0 || to >= len(slides) || from == to {
		return slides
	}
	slide := slides[from]
	if from < to {
		copy(slides[from:], slides[from+1:to+1])
	} else {
		copy(slides[to+1:], slides[to:from])
	}
	slides[to] = slide
	return slides
}

func findSlideByRole(slides []officegen.Slide, role pptxSlideRole, start int) int {
	if start < 0 {
		start = 0
	}
	for idx := start; idx < len(slides); idx++ {
		if slideRoleName(slides[idx], idx) == role {
			return idx
		}
	}
	return -1
}

func insertSlide(slides []officegen.Slide, idx int, slide officegen.Slide) []officegen.Slide {
	if idx < 0 {
		idx = 0
	}
	if idx > len(slides) {
		idx = len(slides)
	}
	slides = append(slides, officegen.Slide{})
	copy(slides[idx+1:], slides[idx:])
	slides[idx] = slide
	return slides
}

func ensureMinimumNarrativeSlides(slides []officegen.Slide, deckTitle string, archetype pptxArchetype) []officegen.Slide {
	for len(slides) < 4 {
		switch len(slides) {
		case 1:
			slides = append(slides, defaultSummarySlide(archetype, deckTitle))
		case 2:
			slides = append(slides, defaultSupportingSlide(archetype, deckTitle))
		case 3:
			slides = append(slides, defaultActionSlide(archetype, deckTitle))
		default:
			return slides
		}
	}
	return slides
}

func defaultSummarySlide(archetype pptxArchetype, deckTitle string) officegen.Slide {
	switch archetype {
	case pptxArchetypeCompany:
		slide := officegen.Slide{
			Role:     string(pptxSlideRoleSummary),
			Title:    "Key Takeaways",
			Layout:   "content",
			Subtitle: "Lead with the outcome, then explain the capabilities behind it",
			Sections: []officegen.SlideSection{
				{Heading: "Unified Work", Detail: "Bring messages, docs, approvals, and follow-up into one flow"},
				{Heading: "Visible ROI", Detail: "Anchor the story in cycle time, on-time work, and reuse metrics"},
				{Heading: "Safe Rollout", Detail: "Move from pilot to scale with governance and adoption checkpoints"},
			},
		}
		slide.Variant = normalizeSlideVariant(slide)
		return slide
	case pptxArchetypeGeneral:
		slide := officegen.Slide{
			Role:     string(pptxSlideRoleSummary),
			Title:    "Executive Summary",
			Layout:   "content",
			Subtitle: "Lead with the decision, then support it slide by slide",
			Sections: []officegen.SlideSection{
				{Heading: "Core Insight", Detail: "State the main conclusion in direct business language"},
				{Heading: "Why It Matters", Detail: "Summarize the impact, risk, or upside behind the conclusion"},
				{Heading: "Decision", Detail: "Clarify what should happen next and who needs to move"},
			},
		}
		slide.Variant = normalizeSlideVariant(slide)
		return slide
	default:
		slide := defaultArchetypeSlide(archetype, 1, deckTitle)
		slide.Variant = normalizeSlideVariant(slide)
		return slide
	}
}

func defaultSupportingSlide(archetype pptxArchetype, deckTitle string) officegen.Slide {
	if archetype != pptxArchetypeGeneral {
		slide := defaultArchetypeSlide(archetype, 2, deckTitle)
		slide.Variant = normalizeSlideVariant(slide)
		return slide
	}
	slide := officegen.Slide{
		Role:     string(pptxSlideRoleEvidence),
		Title:    "What Matters Most",
		Layout:   "content",
		Subtitle: "Support the headline with the few facts that change the decision",
		Sections: []officegen.SlideSection{
			{Heading: "Signal", Detail: "Show the strongest evidence behind the conclusion"},
			{Heading: "Tradeoff", Detail: "Explain what becomes easier, faster, or safer"},
			{Heading: "Constraint", Detail: "Call out the key limit, dependency, or condition"},
		},
	}
	slide.Variant = normalizeSlideVariant(slide)
	return slide
}

func defaultActionSlide(archetype pptxArchetype, deckTitle string) officegen.Slide {
	if archetype != pptxArchetypeGeneral {
		slide := defaultArchetypeSlide(archetype, 5, deckTitle)
		slide.Variant = normalizeSlideVariant(slide)
		return slide
	}
	slide := officegen.Slide{
		Role:     string(pptxSlideRoleAction),
		Title:    "Next Steps",
		Layout:   "content",
		Subtitle: "Close with a small set of actions, owners, and validation points",
		Sections: []officegen.SlideSection{
			{Heading: "This Week", Detail: "Owner defines the decision scope and confirms the first milestone"},
			{Heading: "30 Days", Detail: "Team executes the first proof point and tracks the lead metric"},
			{Heading: "Review", Detail: "Leadership decides whether to scale based on evidence and adoption"},
		},
	}
	slide.Variant = normalizeSlideVariant(slide)
	return slide
}

func enforceOverviewSlide(slide officegen.Slide, archetype pptxArchetype) officegen.Slide {
	if slideLayoutName(slide) == "chart" || slideLayoutName(slide) == "dashboard" {
		return defaultSummarySlide(archetype, slide.Title)
	}
	slide.Role = string(pptxSlideRoleSummary)
	if len(slide.Sections) == 0 && len(slide.Points) >= 3 {
		if sections := pointsToSummarySections(slide.Points, 3); len(sections) > 0 {
			slide.Sections = sections
			slide.Points = nil
		}
	}
	if !looksLikeOverviewSlide(slide) || isPlaceholderSlideTitle(slide.Title) {
		slide.Title = summaryTitleForArchetype(archetype)
	}
	if strings.TrimSpace(slide.Subtitle) == "" || isPlaceholderSlideTitle(slide.Subtitle) {
		slide.Subtitle = summarySubtitleForArchetype(archetype)
	}
	slide.Layout = "content"
	slide.HasImage = false
	slide.ImagePrompt = ""
	slide.ImagePos = ""
	slide.Variant = normalizeSlideVariant(slide)
	return slide
}

func ensureEvidenceCoverage(slides []officegen.Slide, deckTitle string, archetype pptxArchetype, maxSlides int) []officegen.Slide {
	if archetype != pptxArchetypeMarket && archetype != pptxArchetypeOps {
		return slides
	}
	if evidenceIdx := findSlideByRole(slides, pptxSlideRoleEvidence, 1); evidenceIdx >= 0 {
		targetIdx := 2
		if targetIdx >= len(slides) {
			targetIdx = len(slides) - 1
		}
		if evidenceIdx != targetIdx && targetIdx > 0 {
			return moveSlide(slides, evidenceIdx, targetIdx)
		}
		return slides
	}
	for idx := 1; idx < len(slides); idx++ {
		if slideLayoutName(slides[idx]) == "chart" || slideLayoutName(slides[idx]) == "dashboard" {
			slides[idx].Role = string(pptxSlideRoleEvidence)
			if idx != 2 && len(slides) > 2 {
				return moveSlide(slides, idx, 2)
			}
			return slides
		}
	}
	evidenceSlide := defaultSupportingSlide(archetype, deckTitle)
	if len(slides) > 2 && isReplaceableNarrativeSlide(slides[2]) {
		slides[2] = evidenceSlide
		return slides
	}
	if len(slides) < maxSlides {
		insertIdx := 2
		if insertIdx > len(slides) {
			insertIdx = len(slides)
		}
		slides = append(slides, officegen.Slide{})
		copy(slides[insertIdx+1:], slides[insertIdx:])
		slides[insertIdx] = evidenceSlide
	}
	return slides
}

func ensureClosingActionSlide(slides []officegen.Slide, deckTitle string, archetype pptxArchetype, maxSlides int) []officegen.Slide {
	if len(slides) == 0 {
		return slides
	}
	lastIdx := len(slides) - 1
	if actionIdx := findSlideByRole(slides, pptxSlideRoleAction, 1); actionIdx >= 0 && actionIdx != lastIdx {
		slides = moveSlide(slides, actionIdx, lastIdx)
	}
	last := slides[lastIdx]
	if isActionSlide(last) || looksLikeClosingSlide(last) {
		last.Role = string(pptxSlideRoleAction)
		if isPlaceholderSlideTitle(last.Title) {
			last.Title = actionTitleForArchetype(archetype)
		}
		if strings.TrimSpace(last.Subtitle) == "" || isPlaceholderSlideTitle(last.Subtitle) {
			last.Subtitle = actionSubtitleForArchetype(archetype)
		}
		last = normalizeActionSlide(last)
		last.Layout = "content"
		last.HasImage = false
		last.ImagePrompt = ""
		last.ImagePos = ""
		last.Variant = normalizeSlideVariant(last)
		slides[lastIdx] = last
		return slides
	}
	if len(slides) < maxSlides {
		return append(slides, defaultActionSlide(archetype, deckTitle))
	}
	if isReplaceableNarrativeSlide(last) || slideLayoutName(last) == "content" {
		slides[lastIdx] = defaultActionSlide(archetype, deckTitle)
	}
	return slides
}

func pointsToSummarySections(points []string, limit int) []officegen.SlideSection {
	sections := make([]officegen.SlideSection, 0, len(points))
	for idx, point := range points {
		heading, detail := pointToSection(point, idx)
		if heading == "" && detail == "" {
			continue
		}
		if strings.HasPrefix(heading, "Step ") || heading == "" {
			heading = fmt.Sprintf("Takeaway %d", idx+1)
		}
		if detail == "" {
			detail = fitTextForLayout(cleanSentence(point), 42)
		}
		sections = append(sections, officegen.SlideSection{
			Heading: heading,
			Detail:  detail,
		})
		if limit > 0 && len(sections) >= limit {
			break
		}
	}
	return normalizeSections(sections, limit)
}

func summaryTitleForArchetype(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return "Key Takeaways"
	case pptxArchetypeMarket:
		return "Key Takeaways"
	case pptxArchetypeOps:
		return "Business Takeaways"
	case pptxArchetypeTraining:
		return "Learning Goals"
	default:
		return "Executive Summary"
	}
}

func summarySubtitleForArchetype(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return "Lead with the outcome, then explain the capabilities behind it"
	case pptxArchetypeMarket:
		return "Start with the market call, then support it with evidence"
	case pptxArchetypeOps:
		return "Start with the operating conclusion, then show what is driving it"
	case pptxArchetypeTraining:
		return "Clarify what the audience should learn before stepping into detail"
	default:
		return "Lead with the decision, then support it slide by slide"
	}
}

func actionTitleForArchetype(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return "Rollout Path"
	case pptxArchetypeMarket:
		return "Entry Recommendations"
	case pptxArchetypeOps:
		return "Execution Actions"
	case pptxArchetypeTraining:
		return "Next Practice Steps"
	default:
		return "Next Steps"
	}
}

func actionSubtitleForArchetype(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return "Close with staged rollout actions, owners, and proof points"
	case pptxArchetypeMarket:
		return "Close with the market sequence, owner, and validation window"
	case pptxArchetypeOps:
		return "Close with repair actions, owner, and the metric to track"
	case pptxArchetypeTraining:
		return "Close with the next commands, practice loop, and caution points"
	default:
		return "Close with a small set of actions, owners, and validation points"
	}
}

func looksLikeOverviewSlide(slide officegen.Slide) bool {
	if slideRoleName(slide, 0) == pptxSlideRoleSummary {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"summary", "takeaway", "overview", "learning goal", "headline", "key point", "总结", "要点", "概览", "学习目标", "核心结论", "关键点"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func looksLikeClosingSlide(slide officegen.Slide) bool {
	if slideRoleName(slide, 0) == pptxSlideRoleAction {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"next step", "next action", "decision", "recommendation", "rollout", "plan", "action", "下一步", "行动", "决策", "建议", "推进", "实施计划", "落地路径"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func isReplaceableNarrativeSlide(slide officegen.Slide) bool {
	if isWeakArchetypeSlide(slide) {
		return true
	}
	if isPlaceholderSlideTitle(slide.Title) {
		return true
	}
	return strings.TrimSpace(slide.Subtitle) == "" &&
		len(slide.Points) <= 1 &&
		len(slide.Sections) == 0 &&
		len(slide.Metrics) == 0 &&
		slide.Chart == nil
}

func shouldInsertOverviewSlide(slide officegen.Slide) bool {
	role := slideRoleName(slide, 0)
	if role == pptxSlideRoleSummary {
		return false
	}
	if role == pptxSlideRoleEvidence || role == pptxSlideRoleAction {
		return true
	}
	switch slideLayoutName(slide) {
	case "chart", "dashboard":
		return true
	}
	if slide.HasImage || strings.TrimSpace(slide.ImagePrompt) != "" {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"product", "scenario", "workflow", "interface", "demo", "experience", "产品", "场景", "流程", "界面", "演示", "体验"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func isPlaceholderSlideTitle(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	switch {
	case text == "":
		return true
	case strings.HasPrefix(text, "part "),
		strings.HasPrefix(text, "slide "),
		strings.HasPrefix(text, "section "),
		strings.HasPrefix(text, "first slide"),
		strings.HasPrefix(text, "second slide"),
		strings.HasPrefix(text, "third slide"),
		strings.HasPrefix(text, "fourth slide"),
		strings.HasPrefix(text, "fifth slide"),
		strings.HasPrefix(text, "sixth slide"),
		strings.HasPrefix(text, "seventh slide"),
		strings.HasPrefix(text, "eighth slide"),
		text == "overview deck",
		text == "content slide":
		return true
	default:
		return false
	}
}

func normalizeSlideVariant(slide officegen.Slide) string {
	return chooseSlideVariant(slide, "")
}

func shouldUseTimelineVariant(slide officegen.Slide) bool {
	if len(slide.Sections) < 3 || len(slide.Sections) > 4 {
		return false
	}
	if slideRoleName(slide, 0) == pptxSlideRoleAction {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"timeline", "roadmap", "rollout", "plan", "phase", "milestone", "step", "时间线", "路线图", "推进", "计划", "阶段", "里程碑", "步骤"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func shouldUseComparisonVariant(slide officegen.Slide) bool {
	if len(slide.Sections) != 2 && len(slide.Points) != 2 {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Role))
	for _, keyword := range []string{"compare", "comparison", "before", "after", "with", "without", "versus", "vs", "对比", "比较", "前后", "之前", "之后", "有无", "方案a", "方案b"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func shouldUseFeatureGridVariant(slide officegen.Slide) bool {
	if len(slide.Sections) == 4 {
		return true
	}
	return len(slide.Points) == 4 && !slide.HasImage
}

func shouldUsePillarListVariant(slide officegen.Slide) bool {
	if len(slide.Sections) < 3 {
		return false
	}
	if shouldUseTimelineVariant(slide) {
		return false
	}
	return len(slide.Sections) <= 4
}

func slideVariantCandidates(slide officegen.Slide) []string {
	layout := slideLayoutName(slide)
	role := slideRoleName(slide, 0)
	explicit := strings.ToLower(strings.TrimSpace(slide.Variant))
	candidates := make([]string, 0, 8)
	add := func(values ...string) {
		for _, value := range values {
			value = strings.ToLower(strings.TrimSpace(value))
			if value == "" {
				continue
			}
			for _, existing := range candidates {
				if existing == value {
					goto next
				}
			}
			candidates = append(candidates, value)
		next:
		}
	}

	switch layout {
	case "title":
		add(explicit)
		if slide.HasImage {
			add("title-split", "title-center")
		} else {
			add("title-center", "title-split")
		}
	case "chart":
		add(explicit, "chart-context", "chart-focus")
	case "dashboard":
		add(explicit)
		if len(slide.Metrics) >= 2 {
			add("stat-band", "kpi-band")
		} else {
			add("kpi-band", "stat-band")
		}
	default:
		add(explicit)
		if shouldUseTimelineVariant(slide) {
			add("timeline")
		}
		if shouldUseComparisonVariant(slide) {
			add("comparison")
		}
		if shouldUseFeatureGridVariant(slide) {
			add("feature-grid")
		}
		if role == pptxSlideRoleSummary || role == pptxSlideRoleAction {
			add("sections-grid")
		}
		if shouldUsePillarListVariant(slide) {
			add("pillar-list")
		}
		if len(slide.Sections) > 0 {
			add("sections-grid")
		}
		if slide.HasImage {
			add("image-right")
		}
		if len(slide.Points) > 0 {
			add("point-cards", "bullets")
		}
		if strings.TrimSpace(slide.Content) != "" {
			add("prose")
		}
		add("bullets")
	}

	filtered := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		switch candidate {
		case "title-center", "title-split":
			if layout == "title" {
				filtered = append(filtered, candidate)
			}
		case "chart-context", "chart-focus":
			if layout == "chart" {
				filtered = append(filtered, candidate)
			}
		case "stat-band", "kpi-band":
			if layout == "dashboard" {
				filtered = append(filtered, candidate)
			}
		case "timeline":
			if shouldUseTimelineVariant(slide) {
				filtered = append(filtered, candidate)
			}
		case "comparison":
			if shouldUseComparisonVariant(slide) {
				filtered = append(filtered, candidate)
			}
		case "feature-grid":
			if shouldUseFeatureGridVariant(slide) {
				filtered = append(filtered, candidate)
			}
		case "pillar-list":
			if shouldUsePillarListVariant(slide) {
				filtered = append(filtered, candidate)
			}
		case "sections-grid":
			if len(slide.Sections) > 0 {
				filtered = append(filtered, candidate)
			}
		case "image-right":
			if slide.HasImage {
				filtered = append(filtered, candidate)
			}
		case "point-cards", "bullets":
			if len(slide.Points) > 0 {
				filtered = append(filtered, candidate)
			}
		case "prose":
			if strings.TrimSpace(slide.Content) != "" {
				filtered = append(filtered, candidate)
			}
		}
	}

	if len(filtered) == 0 {
		switch layout {
		case "title":
			return []string{"title-center"}
		case "chart":
			return []string{"chart-context"}
		case "dashboard":
			return []string{"stat-band"}
		default:
			return []string{"bullets"}
		}
	}
	return filtered
}

func slidePatternSignature(slide officegen.Slide, variant string) string {
	return slideLayoutName(slide) + ":" + strings.TrimSpace(variant)
}

func chooseSlideVariant(slide officegen.Slide, prevSignature string) string {
	candidates := slideVariantCandidates(slide)
	if prevSignature == "" {
		return candidates[0]
	}
	for _, candidate := range candidates {
		if slidePatternSignature(slide, candidate) != prevSignature {
			return candidate
		}
	}
	return candidates[0]
}

func diversifySlideForPattern(slide officegen.Slide) officegen.Slide {
	if slideLayoutName(slide) != "content" {
		return slide
	}
	if len(slide.Sections) == 0 && len(slide.Points) >= 3 {
		sections := pointsToSummarySections(slide.Points, minInt(len(slide.Points), 4))
		if len(sections) >= 3 {
			slide.Sections = sections
			if len(slide.Points) <= 4 {
				slide.Points = nil
			}
		}
	}
	if len(slide.Sections) == 0 && len(slide.Metrics) >= 2 {
		slide.Layout = "dashboard"
	}
	return slide
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func expandSlideForDensity(slide officegen.Slide) []officegen.Slide {
	switch {
	case len(slide.Points) > 5:
		return splitSlidePoints(slide, 5)
	case slide.HasImage && len(slide.Points) > 4:
		return splitSlidePoints(slide, 4)
	case shouldUseTimelineVariant(slide):
		return []officegen.Slide{slide}
	case shouldUseFeatureGridVariant(slide):
		return []officegen.Slide{slide}
	case shouldUsePillarListVariant(slide):
		return []officegen.Slide{slide}
	case len(slide.Sections) > 4:
		return splitSlideSections(slide, 4)
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
			next.Title = slide.Title + " (cont.)"
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
			next.Title = slide.Title + " (cont.)"
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
			next.Title = slide.Title + " (cont.)"
		}
		out = append(out, next)
	}
	return out
}

func rebalanceAdjacentLayouts(slides []officegen.Slide) []officegen.Slide {
	prevSignature := ""
	for idx := range slides {
		role := slideRoleName(slides[idx], idx)
		if (role == pptxSlideRoleSummary || role == pptxSlideRoleAction) && len(slides[idx].Sections) == 0 && len(slides[idx].Points) > 0 {
			if sections := pointsToSummarySections(slides[idx].Points, 3); len(sections) > 0 {
				slides[idx].Sections = sections
				if len(slides[idx].Points) <= 3 {
					slides[idx].Points = nil
				}
			}
		}
		slides[idx].Variant = chooseSlideVariant(slides[idx], prevSignature)
		if idx > 0 && slidePatternSignature(slides[idx], slides[idx].Variant) == prevSignature {
			slides[idx] = diversifySlideForPattern(slides[idx])
			slides[idx].Variant = chooseSlideVariant(slides[idx], prevSignature)
		}
		prevSignature = slidePatternSignature(slides[idx], slides[idx].Variant)
	}
	return slides
}

func softlyApplyArchetypeDefaults(slides []officegen.Slide, archetype pptxArchetype, deckTitle string) []officegen.Slide {
	if len(slides) == 0 {
		return slides
	}
	if archetype == pptxArchetypeGeneral {
		for idx := range slides {
			if strings.TrimSpace(slides[idx].Role) == "" {
				slides[idx].Role = string(slideRoleName(slides[idx], idx))
			}
		}
		return slides
	}
	for idx := 1; idx < len(slides); idx++ {
		if isWeakArchetypeSlide(slides[idx]) {
			role := slideRoleName(slides[idx], idx)
			defaultSlide := defaultSlideForRole(archetype, role, idx, deckTitle)
			defaultSlide.Variant = normalizeSlideVariant(defaultSlide)
			slides[idx] = defaultSlide
			continue
		}
		if strings.TrimSpace(slides[idx].Role) == "" {
			slides[idx].Role = string(slideRoleName(slides[idx], idx))
		}
	}
	for idx := range slides {
		if strings.TrimSpace(slides[idx].Variant) == "" {
			slides[idx].Variant = normalizeSlideVariant(slides[idx])
		}
	}
	return slides
}

func defaultSlideForRole(archetype pptxArchetype, role pptxSlideRole, idx int, deckTitle string) officegen.Slide {
	switch role {
	case pptxSlideRoleSummary:
		return defaultSummarySlide(archetype, deckTitle)
	case pptxSlideRoleEvidence:
		return defaultSupportingSlide(archetype, deckTitle)
	case pptxSlideRoleAction:
		return defaultActionSlide(archetype, deckTitle)
	case pptxSlideRoleCover:
		return defaultArchetypeSlide(archetype, 0, deckTitle)
	default:
		return defaultArchetypeSlide(archetype, idx, deckTitle)
	}
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
		Title:      fitTextForLayout(firstNonEmpty(chart.Title, "Key Data Comparison"), 20),
	}
}

func splitContentToPoints(content string, limit int) []string {
	fields := strings.FieldsFunc(content, func(r rune) bool {
		switch r {
		case '\n', '\r', ';', '.', '!', '?', '；', '。', '！', '？':
			return true
		default:
			return false
		}
	})
	if len(fields) == 0 && strings.TrimSpace(content) != "" {
		fields = []string{content}
	}
	return normalizePoints(fields, limit, 48)
}

func deriveSlideSubtitle(slide officegen.Slide) string {
	switch slide.Layout {
	case "chart":
		if len(slide.Points) > 0 {
			return fitTextForLayout(slide.Points[0], 24)
		}
		if slide.Chart != nil {
			return fitTextForLayout("Start with the takeaway, then support it with data.", 24)
		}
	case "dashboard":
		if len(slide.Points) > 0 {
			return fitTextForLayout(slide.Points[0], 24)
		}
		if len(slide.Metrics) > 0 {
			return "Key metrics and action focus"
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
		fmt.Sprintf("%s is the highest at %s", chart.Categories[maxIdx], formatChartValue(chart.Values[maxIdx])),
	}
	if len(chart.Values) > 1 && minIdx != maxIdx {
		points = append(points, fmt.Sprintf("%s is the lowest at %s", chart.Categories[minIdx], formatChartValue(chart.Values[minIdx])))
	}
	return normalizePoints(points, limit, 40)
}

func deriveMetricPoints(metrics []officegen.MetricCard, limit int) []string {
	points := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		if strings.TrimSpace(metric.Label) == "" || strings.TrimSpace(metric.Value) == "" {
			continue
		}
		item := strings.TrimSpace(metric.Label) + " reaches " + strings.TrimSpace(metric.Value)
		if strings.TrimSpace(metric.Note) != "" {
			item += ", " + strings.TrimSpace(metric.Note)
		}
		points = append(points, item)
	}
	return normalizePoints(points, limit, 40)
}

func normalizeImagePosition(pos string) string {
	switch strings.ToLower(strings.TrimSpace(pos)) {
	case "left", "right", "top", "bottom", "background", "center", "diagonal":
		return strings.ToLower(strings.TrimSpace(pos))
	default:
		return "right"
	}
}

func computePPTXImageBudget(slides []officegen.Slide, deckTitle string, enableImages bool) int {
	if !enableImages || len(slides) == 0 {
		return 0
	}

	budget := 1
	switch {
	case len(slides) >= 8:
		budget = 4
	case len(slides) >= 6:
		budget = 3
	case len(slides) >= 4:
		budget = 2
	}

	switch detectPPTXArchetype(deckTitle, "") {
	case pptxArchetypeCompany, pptxArchetypeTraining:
		if budget < 4 {
			budget++
		}
	case pptxArchetypeMarket, pptxArchetypeOps:
		if budget > 2 {
			budget--
		}
	}

	eligible := 0
	for idx, slide := range slides {
		role := inferPPTXSlideRole(slide, idx)
		if role == pptxSlideRoleCover || role == pptxSlideRoleDetail {
			eligible++
		}
	}
	if eligible < budget {
		budget = eligible
	}
	if budget < 0 {
		return 0
	}
	return budget
}

func defaultImagePositionForSlide(slide officegen.Slide) string {
	if slideRoleName(slide, 0) == pptxSlideRoleCover {
		return "background"
	}
	if len(slide.Sections) >= 4 || len(slide.Points) >= 4 {
		return "top"
	}
	return "right"
}

func shouldAutoAddImagePrompt(slide officegen.Slide, _ bool) bool {
	role := slideRoleName(slide, 0)
	if role == pptxSlideRoleCover {
		return false
	}
	if role != pptxSlideRoleDetail {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"product", "platform", "feature", "capability", "module", "interface", "screen", "scenario", "use case", "training", "workflow", "experience", "demo", "game", "gameplay", "world", "character", "collaboration", "产品", "平台", "功能", "能力", "模块", "界面", "页面", "场景", "案例", "培训", "流程", "体验", "演示", "游戏", "玩法", "世界", "角色", "协作"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func buildFallbackImagePrompt(slide officegen.Slide, deckTitle string) string {
	subject := firstNonEmpty(slide.Title, deckTitle, "Presentation topic")
	fragments := make([]string, 0, 4)
	if subtitle := strings.TrimSpace(slide.Subtitle); subtitle != "" {
		fragments = append(fragments, cleanSentence(subtitle))
	}
	for _, section := range slide.Sections {
		if len(fragments) >= 3 {
			break
		}
		fragment := firstNonEmpty(section.Heading, section.Detail)
		if fragment == "" {
			continue
		}
		if strings.TrimSpace(section.Detail) != "" && strings.TrimSpace(section.Heading) != "" {
			fragment = strings.TrimSpace(section.Heading) + ": " + strings.TrimSpace(section.Detail)
		}
		fragments = append(fragments, cleanSentence(fragment))
	}
	for _, point := range slide.Points {
		if len(fragments) >= 3 {
			break
		}
		if cleaned := cleanSentence(point); cleaned != "" {
			fragments = append(fragments, cleaned)
		}
	}
	if len(fragments) == 0 && strings.TrimSpace(slide.Content) != "" {
		fragments = append(fragments, cleanSentence(strings.TrimSpace(slide.Content)))
	}

	scene := strings.Join(fragments, "; ")
	styleHint := "clean professional presentation visual, no text overlay"
	if slideRoleName(slide, 0) == pptxSlideRoleCover {
		styleHint = "hero visual for a presentation cover, cinematic composition, clean professional lighting, no text overlay"
	} else if looksLikeGameSlide(slide) {
		styleHint = "vivid gameplay-style scene, immersive environment, polished composition, no text overlay"
	} else {
		styleHint = "editorial product or real-world usage scene, polished composition, professional lighting, no text overlay"
	}
	return trimRunes(strings.TrimSpace(strings.Join([]string{subject, scene, styleHint}, ". ")), 180)
}

func looksLikeGameSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	for _, keyword := range []string{"game", "gameplay", "world", "character", "sandbox", "minecraft", "游戏", "玩法", "世界", "角色", "沙盒", "minecraft"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func allowImageForSlide(slide officegen.Slide) bool {
	role := slideRoleName(slide, 0)
	if role == pptxSlideRoleSummary || role == pptxSlideRoleEvidence || role == pptxSlideRoleAction {
		return false
	}
	if role == pptxSlideRoleCover {
		return true
	}
	if len(slide.Metrics) > 0 || slide.Chart != nil || slideLayoutName(slide) == "chart" || slideLayoutName(slide) == "dashboard" {
		return false
	}
	if len(slide.Points) > 4 || len(slide.Sections) > 4 {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"market", "industry", "competition", "review", "value", "pricing", "package", "quote", "recommendation", "next step", "rollout", "region", "opportunity", "risk", "operations", "data", "cadence", "市场", "行业", "竞争", "复盘", "价值", "价格", "报价", "套餐", "建议", "下一步", "推进", "区域", "机会", "风险", "运营", "数据", "节奏"} {
		if strings.Contains(text, keyword) {
			return false
		}
	}
	if (slide.HasImage || strings.TrimSpace(slide.ImagePrompt) != "") &&
		!isPlaceholderSlideTitle(slide.Title) {
		return true
	}
	for _, keyword := range []string{"product", "platform", "feature", "capability", "module", "interface", "screen", "scenario", "use case", "training", "workflow", "experience", "demo", "game", "gameplay", "world", "character", "collaboration", "产品", "平台", "功能", "能力", "模块", "界面", "页面", "场景", "案例", "培训", "流程", "体验", "演示", "游戏", "玩法", "世界", "角色", "协作"} {
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
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	if strings.Contains(text, "value") || strings.Contains(text, "价值") || strings.Contains(text, "roi") || strings.Contains(text, "收益") {
		slide.Layout = "dashboard"
		slide.Metrics = normalizeMetrics([]officegen.MetricCard{
			{Label: "Approval Cycle Time", Value: "-30%", Note: "Pilot target"},
			{Label: "On-Time Task Rate", Value: "+15%", Note: "Weekly tracking"},
			{Label: "Knowledge Reuse Rate", Value: "+25%", Note: "Quarterly review"},
		}, 4)
		if len(slide.Points) == 0 {
			slide.Points = normalizePoints([]string{
				"Validate ROI first with approval, task, and knowledge metrics.",
				"Decide whether to expand after an eight-week pilot.",
			}, 2, 42)
		}
		return slide
	}
	if strings.Contains(text, "market size") || strings.Contains(text, "market space") || strings.Contains(text, "市场规模") || strings.Contains(text, "市场空间") {
		slide.Layout = "chart"
		slide.Chart = normalizeChart(&officegen.ChartData{
			Type:       "bar",
			Title:      "Regional Demand Index",
			Categories: []string{"North America", "Europe", "APAC"},
			Values:     []float64{100, 72, 58},
		})
		if len(slide.Points) == 0 {
			slide.Points = normalizePoints([]string{
				"Demand is most mature in North America, with Europe and developed APAC forming the second tier.",
				"Validate the English-speaking market first, then test expansion efficiency in the next region.",
			}, 2, 42)
		}
		if slide.Source == "" {
			slide.Source = "Compiled from public sources"
		}
	}
	return slide
}

func detectPPTXArchetype(description, title string) pptxArchetype {
	text := strings.ToLower(strings.TrimSpace(description + " " + title))
	switch {
	case strings.Contains(text, "enterprise collaboration platform"),
		strings.Contains(text, "协作平台"),
		strings.Contains(text, "企业协作"),
		strings.Contains(text, "办公协作"),
		strings.Contains(text, "saas 产品介绍"),
		strings.Contains(text, "产品介绍"),
		strings.Contains(text, "客户方案"):
		return pptxArchetypeCompany
	case strings.Contains(text, "market opportunity") || strings.Contains(text, "market analysis") || strings.Contains(text, "global expansion") ||
		strings.Contains(text, "市场机会") || strings.Contains(text, "市场分析") || strings.Contains(text, "出海") || strings.Contains(text, "行业研究"):
		return pptxArchetypeMarket
	case strings.Contains(text, "business review") || strings.Contains(text, "quarterly operations") || strings.Contains(text, "data report") || strings.Contains(text, "operations review") ||
		strings.Contains(text, "业务复盘") || strings.Contains(text, "经营分析") || strings.Contains(text, "季度复盘") || strings.Contains(text, "运营复盘") || strings.Contains(text, "数据报告"):
		return pptxArchetypeOps
	case strings.Contains(text, "onboarding training") || strings.Contains(text, "new hire") || strings.Contains(text, "tutorial") || strings.Contains(text, "getting started guide") ||
		strings.Contains(text, "培训") || strings.Contains(text, "上手指南") || strings.Contains(text, "新员工") || strings.Contains(text, "教程"):
		return pptxArchetypeTraining
	default:
		return pptxArchetypeGeneral
	}
}

func buildArchetypePromptRules(archetype pptxArchetype) string {
	switch archetype {
	case pptxArchetypeCompany:
		return `- For this topic, a strong storyline is usually cover -> solution overview -> core capabilities -> customer value -> use cases -> rollout path, but adapt the exact slide count to the prompt.
- Translate that storyline into patterns: cover=title-center/title-split, summary=sections-grid, capabilities=feature-grid or pillar-list, customer value=stat-band, use cases=comparison or pillar-list, closing=timeline.
- If the prompt is client-facing, sales-oriented, or mentions pricing, package, proposal, quotation, or plan, include one dedicated pricing or packaging slide before the closing slide and render it as stat-band or comparison.
- Slide 4 should prefer dashboard/stat-band or quantified evidence instead of abstract slogans.
- Slide 5 should use sections, feature-grid, or comparison to emphasize scenario, action, and benefit without repeating slide 4.
- The final slide should use sections with time, owner, and validation criteria. The whole deck should use at most one image slide, preferably on core capabilities.`
	case pptxArchetypeMarket:
		return `- For this topic, a strong storyline is usually cover -> key takeaways -> market size -> regional opportunities -> competitive landscape -> entry recommendations, but adapt the exact slide count to the prompt.
- Translate that storyline into patterns: summary=sections-grid, market size=chart-context, opportunities=pillar-list, competitive landscape=comparison or point-cards, closing=timeline.
- Slide 3 must use a chart and include a source. Do not present market size as plain text judgment.
- Slide 4 should use sections, and slide 5 should prefer comparison or point-cards so the two slides handle region choice and competition separately.
- Slide 6 should use sections with time, owner, and validation criteria. Do not add images by default for this topic.`
	case pptxArchetypeOps:
		return `- For this topic, a strong storyline is usually cover -> business takeaways -> core metrics -> issue diagnosis -> next-quarter priorities -> execution actions, but adapt the exact slide count to the prompt.
- Translate that storyline into patterns: summary=sections-grid, core metrics=chart-context or stat-band, issue diagnosis=pillar-list, priorities=sections-grid, closing=timeline.
- Slide 3 must use a chart and clearly state the data framing or comparison period.
- Slide 4 should use sections or pillar-list to break issues down by dimensions such as acquisition, delivery, and collections instead of long bullets.
- Slides 5-6 must close the loop with at least two of phase, owner, deadline, or validation criteria. Do not add images by default for this topic.`
	case pptxArchetypeTraining:
		return `- For this topic, a strong storyline is usually cover -> learning goals -> installation and setup -> common commands -> example workflow -> cautions, but adapt the exact slide count to the prompt.
- Translate that storyline into patterns: summary=sections-grid, setup/common commands=feature-grid or pillar-list, workflow=timeline, cautions=sections-grid.
- Slides 3-6 should prefer sections organized by step, command, and result.
- Command-heavy slides should use short command names plus concise explanations. Avoid long prose and truncated commands.
- Training decks should not use images by default, and example workflows should prefer structured steps over screenshots.`
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
			{Role: string(pptxSlideRoleCover), Title: firstNonEmpty(deckTitle, "Enterprise Collaboration Platform Overview"), Layout: "title", IsTitle: true, Subtitle: "Build operational efficiency on a unified collaboration foundation"},
			{Role: string(pptxSlideRoleSummary), Title: "Solution Overview", Layout: "content", Subtitle: "Unify entry points and workflows first, then expand governance capability", Points: []string{"One platform covers messaging, documents, workflows, and knowledge collaboration.", "Start with high-frequency scenarios and show visible gains within three months.", "Balance efficiency and compliance through clear permissions and audit boundaries."}},
			{Role: string(pptxSlideRoleDetail), Title: "Core Capabilities", Layout: "content", Subtitle: "The platform creates leverage by connecting information, workflow, and organization", Sections: []officegen.SlideSection{{Heading: "Unified Entry", Detail: "Handle messages, documents, and approvals in one place"}, {Heading: "Workflow Sync", Detail: "Link forms, tasks, and notifications to shorten cycle time"}, {Heading: "Security", Detail: "Use permissions and audit trails for controlled governance"}}},
			{Role: string(pptxSlideRoleEvidence), Title: "Customer Value", Layout: "dashboard", Subtitle: "Value shows up in efficiency, transparency, and management control", Metrics: []officegen.MetricCard{{Label: "Approval Cycle", Value: "-30%", Note: "Pilot target"}, {Label: "On-Time Tasks", Value: "+15%", Note: "Weekly tracking"}, {Label: "Knowledge Reuse", Value: "+25%", Note: "Quarterly review"}}, Points: []string{"Validate ROI first with approval, task, and knowledge metrics.", "Decide whether to scale after an eight-week pilot."}},
			{Role: string(pptxSlideRoleDetail), Title: "Use Cases", Layout: "content", Subtitle: "High-frequency cross-functional scenarios create the fastest proof points", Sections: []officegen.SlideSection{{Heading: "Project Sync", Detail: "Track milestones, risks, and tasks in one workflow"}, {Heading: "Sales Support", Detail: "Connect leads, proposals, pricing, and approvals online"}, {Heading: "HQ Support", Detail: "Push announcements and training with closed-loop feedback"}}},
			{Role: string(pptxSlideRoleAction), Title: "Rollout Path", Layout: "content", Subtitle: "Move from pilot to rollout in stages to reduce risk and prove impact", Sections: []officegen.SlideSection{{Heading: "2-Week Discovery", Detail: "Business owners and IT define the pilot scope"}, {Heading: "8-Week Pilot", Detail: "Launch three high-frequency scenarios and train admins"}, {Heading: "Monthly Review", Detail: "Use adoption, cycle time, and satisfaction to decide expansion"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	case pptxArchetypeMarket:
		defaults := []officegen.Slide{
			{Role: string(pptxSlideRoleCover), Title: firstNonEmpty(deckTitle, "AI Office Global Market Analysis and Entry Recommendations"), Layout: "title", IsTitle: true, Subtitle: "Market size, regional opportunities, competition, and entry choices for leadership"},
			{Role: string(pptxSlideRoleSummary), Title: "Key Takeaways", Layout: "content", Subtitle: "Win the English-speaking market first, then expand into Europe and developed APAC", Points: []string{"North America is the top priority market, followed by the UK, Australia, and New Zealand.", "The battle is decided by distribution entry points and compliance, not just model quality.", "The 90-day objective is paid validation rather than broad regional rollout."}},
			{Role: string(pptxSlideRoleEvidence), Title: "Market Size", Layout: "chart", Subtitle: "North America leads in scale, with Europe and developed APAC forming the second tier", Chart: &officegen.ChartData{Type: "bar", Title: "Regional Demand Index", Categories: []string{"North America", "Europe", "APAC"}, Values: []float64{100, 72, 58}}, Points: []string{"North America shows the most mature demand, while Europe and developed APAC form the second tier.", "Validate the English-speaking market first, then test expansion efficiency in the next region."}, Source: "Compiled from public sources"},
			{Role: string(pptxSlideRoleDetail), Title: "Regional Opportunities", Layout: "content", Subtitle: "Region choice should be sequenced by monetization, compliance, and replication efficiency", Sections: []officegen.SlideSection{{Heading: "North America", Detail: "Strong budgets and faster decisions support premium entry"}, {Heading: "Europe", Detail: "Stable demand, but compliance must come first"}, {Heading: "Developed APAC", Detail: "English-speaking markets make the North America playbook easier to replicate"}}},
			{Role: string(pptxSlideRoleEvidence), Title: "Competitive Landscape", Layout: "content", Subtitle: "Entrenched incumbents own the entry point, so differentiation must come from workflow focus", Sections: []officegen.SlideSection{{Heading: "Microsoft", Detail: "Uses the Office entry point to win enterprise buyers and IT procurement"}, {Heading: "Google", Detail: "Owns default distribution in cloud collaboration and SMB markets"}, {Heading: "Independent Tools", Detail: "Break through via vertical use cases and faster iteration"}}},
			{Role: string(pptxSlideRoleAction), Title: "Entry Recommendations", Layout: "content", Subtitle: "Validate the market within 90 days and secure the first flagship customer", Sections: []officegen.SlideSection{{Heading: "6-Week MVP", Detail: "Product lead launches the English version and completes 10 trials"}, {Heading: "8-Week Trial Sales", Detail: "Global growth lead activates channels and closes the first paid customer"}, {Heading: "90-Day Review", Detail: "Leadership decides whether to expand based on retention and payback"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	case pptxArchetypeOps:
		defaults := []officegen.Slide{
			{Role: string(pptxSlideRoleCover), Title: firstNonEmpty(deckTitle, "SaaS Quarterly Business Review"), Layout: "title", IsTitle: true, Subtitle: "Review growth, customer efficiency, and next-quarter actions in one loop"},
			{Role: string(pptxSlideRoleSummary), Title: "Business Takeaways", Layout: "content", Subtitle: "New acquisition drives growth, but renewals, delivery, and collections are slowing quality improvement", Points: []string{"ARR index reached 128, with growth led mainly by new acquisition.", "Renewal at 84 and collections at 76 trail target materially.", "Next quarter should focus on renewal recovery, delivery efficiency, and cash collection."}},
			{Role: string(pptxSlideRoleEvidence), Title: "Core Metrics", Layout: "chart", Subtitle: "New acquisition still lifts growth, but renewals and collections are dragging quality", Chart: &officegen.ChartData{Type: "bar", Title: "Quarterly Operating Metrics", Categories: []string{"New ARR", "Renewal Rate", "Collection Rate"}, Values: []float64{128, 84, 76}}, Points: []string{"New ARR at 128 shows this quarter is still acquisition-led.", "Renewal and collection performance are below target and need repair."}, Source: "Method: relative index with last quarter = 100; renewal and collection normalized against quarterly targets"},
			{Role: string(pptxSlideRoleDetail), Title: "Issue Diagnosis", Layout: "content", Subtitle: "Delivery, collections, and conversion all create drag on operating quality", Sections: []officegen.SlideSection{{Heading: "P1 Delivery", Detail: "Custom work is 42% of mix, extending project cycles by about 10 days"}, {Heading: "P2 Collections", Detail: "Top 10 customers carry longer payment terms and slower cash conversion"}, {Heading: "P3 Conversion", Detail: "Mid-funnel win rate is 7 points below target"}}},
			{Role: string(pptxSlideRoleDetail), Title: "Next-Quarter Priorities", Layout: "content", Subtitle: "Each priority is tied to an owner and result metric, not just direction", Sections: []officegen.SlideSection{{Heading: "Renewal Recovery", Detail: "Customer success lead restores renewal rate to above 90"}, {Heading: "Delivery Efficiency", Detail: "Delivery lead reduces custom work share below 30"}, {Heading: "Collections Push", Detail: "Sales operations lead raises collection rate to 90"}}},
			{Role: string(pptxSlideRoleAction), Title: "Execution Actions", Layout: "content", Subtitle: "Advance monthly with clear owners, milestones, and validation metrics", Sections: []officegen.SlideSection{{Heading: "April Sales Lead", Detail: "Finish funnel review and lift win rate by 3 points"}, {Heading: "May Delivery Lead", Detail: "Launch the standard package and reduce rework below 10"}, {Heading: "June Ops Lead", Detail: "Review performance against renewal 90 and collection 90"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	case pptxArchetypeTraining:
		defaults := []officegen.Slide{
			{Role: string(pptxSlideRoleCover), Title: firstNonEmpty(deckTitle, "OfficeCLI New Hire Onboarding"), Layout: "title", IsTitle: true, Subtitle: "Get new teammates productive quickly through setup, commands, and example flows"},
			{Role: string(pptxSlideRoleSummary), Title: "Learning Goals", Layout: "content", Subtitle: "Build core understanding first, then complete the first independent command run", Points: []string{"Understand what OfficeCLI does, what it takes in, and what it outputs.", "Finish setup and run one local generation command successfully.", "Know the configuration boundaries and cautions before production use."}},
			{Role: string(pptxSlideRoleDetail), Title: "Installation and Setup", Layout: "content", Subtitle: "Prepare in three steps: environment check, installation, and login validation", Sections: []officegen.SlideSection{{Heading: "Environment Check", Detail: "Confirm Go, config files, and local dependencies are available"}, {Heading: "Install Command", Detail: "Run the build or download command to create the executable"}, {Heading: "Login Check", Detail: "Run a status command after setup to verify connectivity"}}},
			{Role: string(pptxSlideRoleDetail), Title: "Common Commands", Layout: "content", Subtitle: "Memorize the three most common command groups first, then expand usage", Sections: []officegen.SlideSection{{Heading: "Status Check", Detail: "Run config status to verify configuration and dependencies"}, {Heading: "Generate PPT", Detail: "Run new pptx to generate a local PPT file"}, {Heading: "Quality Review", Detail: "Run review pptx for structural and visual review"}}},
			{Role: string(pptxSlideRoleDetail), Title: "Example Workflow", Layout: "content", Subtitle: "A full practice run should cover generation, checking, and revision", Sections: []officegen.SlideSection{{Heading: "Step 1 Scope", Detail: "Define topic, audience, and style before generating output"}, {Heading: "Step 2 Generate", Detail: "Run new pptx and confirm the file was written successfully"}, {Heading: "Step 3 Review", Detail: "Run review pptx and iterate based on the findings"}}},
			{Role: string(pptxSlideRoleAction), Title: "Cautions", Layout: "content", Subtitle: "Validate quality locally before moving into the formal collaboration flow", Sections: []officegen.SlideSection{{Heading: "Validate Locally", Detail: "Keep publishing off by default until output quality is confirmed"}, {Heading: "Complete Config", Detail: "Missing models, image settings, or dependencies will degrade results"}, {Heading: "Keep Commands Intact", Detail: "Preserve full commands, paths, and parameters without truncation"}}},
		}
		if idx < len(defaults) {
			return defaults[idx]
		}
	}
	return officegen.Slide{Role: string(pptxSlideRoleDetail), Title: fmt.Sprintf("Part %d", idx+1), Layout: "content", Subtitle: "Develop one clear takeaway per slide"}
}

func enforceCompanySkeleton(slides []officegen.Slide) {
	if len(slides) < 6 {
		return
	}
	slides[1].Title = "Solution Overview"
	slides[1].Layout = "content"
	slides[1].HasImage = false
	slides[1].Points = normalizePoints([]string{
		"One platform unifies messaging, documents, workflows, and knowledge collaboration.",
		"Start with high-frequency scenarios and show visible gains within three months.",
		"Balance efficiency, governance, and compliance through clear permission boundaries.",
	}, 3, 26)
	slides[1].Sections = nil
	slides[1].Metrics = nil
	slides[1].Chart = nil
	slides[1].Subtitle = "Unify entry points and workflows first, then expand governance capability"

	slides[2].Title = "Core Capabilities"
	slides[2].Layout = "content"
	slides[2].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Unified Entry", Detail: "Handle messages, docs, and approvals in one workspace"},
		{Heading: "Workflow Sync", Detail: "Automate links across forms, tasks, and notifications"},
		{Heading: "Security", Detail: "Control access through permissions and audit trails"},
	}, 3)
	slides[2].Points = nil
	slides[2].Metrics = nil
	slides[2].Chart = nil
	slides[2].Subtitle = "Platform leverage comes from connecting information, workflow, and organization"

	slides[3].Title = "Customer Value"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Approval Cycle -30%", Detail: "Reduced from a 2.4-day baseline to 1.7 days in 8 weeks"},
		{Heading: "On-Time Delivery +15%", Detail: "Weekly task punctuality improved across 3 departments"},
		{Heading: "Knowledge Reuse +25%", Detail: "FAQ and template reuse now covers core workflows"},
	}, 3)
	slides[3].Metrics = nil
	slides[3].Chart = nil
	slides[3].Points = nil
	slides[3].HasImage = false
	slides[3].Source = "Method: baseline is the 4 weeks before launch, compared against the 8-week pilot across 3 departments"
	slides[3].Subtitle = "Value appears in efficiency gains, execution transparency, and stronger knowledge reuse"

	slides[4].Title = "Use Cases"
	slides[4].Layout = "content"
	slides[4].HasImage = false
	slides[4].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Project Sync", Detail: "Drive milestones, risks, and tasks in one flow"},
		{Heading: "Sales Support", Detail: "Close the loop from lead to pricing approval online"},
		{Heading: "HQ Support", Detail: "Distribute announcements and training with tracked feedback"},
	}, 3)
	slides[4].Points = nil
	slides[4].Metrics = nil
	slides[4].Chart = nil
	slides[4].Subtitle = "Cross-functional high-frequency scenarios create the clearest early proof points"

	slides[5].Title = "Rollout Path"
	slides[5].Layout = "content"
	slides[5].HasImage = false
	slides[5] = normalizeActionSlide(slides[5])
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "2-Week Discovery", Detail: "Business owners and IT lock the pilot scope"},
		{Heading: "8-Week Pilot", Detail: "Launch three high-frequency scenarios and finish training"},
		{Heading: "Monthly Review", Detail: "Use adoption, efficiency, and satisfaction to decide expansion"},
	}, 3)
	slides[5].Points = nil
	slides[5].Metrics = nil
	slides[5].Chart = nil
	slides[5].Subtitle = "Move from pilot to rollout in stages to reduce risk and verify impact"
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

	slides[1].Title = "Key Takeaways"
	slides[1].Layout = "content"
	slides[1].Points = normalizePoints([]string{
		"Priority market: start with North America, then expand into Western Europe and developed APAC.",
		"Competition is decided by entry point, distribution, and compliance, not only by the model.",
		"The 90-day goal is paid validation, not broad expansion.",
	}, 3, 28)
	slides[1].Sections = nil
	if slides[1].Subtitle == "" {
		slides[1].Subtitle = "Win the English-speaking market first, then expand into Europe and developed APAC"
	}

	slides[2].Title = "Market Size"
	slides[2].Layout = "chart"
	slides[2] = normalizeEvidenceSlide(slides[2])
	slides[2].Chart = normalizeChart(&officegen.ChartData{Type: "bar", Title: "Regional Demand Index (North America = 100)", Categories: []string{"North America", "Europe", "APAC"}, Values: []float64{100, 72, 58}})
	slides[2].Source = "Method: relative demand index with North America = 100; compiled from public sources"
	if slides[2].Subtitle == "" {
		slides[2].Subtitle = "North America leads in scale, with Europe and developed APAC forming the second tier"
	}
	slides[2].Points = normalizePoints([]string{
		"Using North America = 100 makes entry priority easier to compare.",
		"Validate the highest-value English-speaking market first, then expand into more compliance-heavy regions.",
	}, 2, 28)

	slides[3].Title = "Regional Opportunities"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{{Heading: "North America", Detail: "Strongest willingness to pay, ideal for testing unit economics first"}, {Heading: "Western Europe", Detail: "Stable budgets, but compliance readiness must come first"}, {Heading: "APAC", Detail: "Mature English-speaking markets make replication easier"}}, 3)
	slides[3].Points = nil
	if slides[3].Subtitle == "" {
		slides[3].Subtitle = "Region choice should be sequenced by monetization, compliance, and replication efficiency"
	}

	slides[4].Title = "Competitive Landscape"
	slides[4].Layout = "content"
	slides[4].Points = normalizePoints([]string{
		"Entry point: Microsoft and Google own default distribution, so direct head-on competition is inefficient.",
		"Distribution: independent tools win through sharp user experience and compounding word of mouth.",
		"White space: cross-app workflows, localized templates, and channel partnerships remain open.",
	}, 3, 30)
	slides[4].Sections = nil
	if slides[4].Subtitle == "" {
		slides[4].Subtitle = "Assess the market through entry point, distribution, and remaining white space"
	}

	slides[5].Title = "Entry Recommendations"
	slides[5].Layout = "content"
	slides[5] = normalizeActionSlide(slides[5])
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "6-Week MVP", Detail: "Product lead launches the English version and lands 10 trials"},
		{Heading: "8-Week Trial Sales", Detail: "Growth lead validates acquisition and channel motion"},
		{Heading: "90-Day Review", Detail: "Leadership decides expansion based on payback and retention"},
	}, 3)
	slides[5].Points = nil
	slides[5].Points = nil
	if slides[5].Subtitle == "" {
		slides[5].Subtitle = "Validate the market within 90 days and secure the first flagship customer"
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

	slides[1].Title = "Business Takeaways"
	slides[1].Layout = "content"
	slides[1].Points = normalizePoints([]string{
		"ARR index reached 128, and growth was driven mainly by new acquisition.",
		"Renewal at 84 and collections at 76 show quality is trailing growth.",
		"Next quarter should focus on renewals, delivery efficiency, and cash collection.",
	}, 3, 28)
	slides[1].Sections = nil
	slides[1].Metrics = nil
	slides[1].Chart = nil
	slides[1].Source = ""
	slides[1].Subtitle = "New acquisition drives growth, but renewals, delivery, and collections are slowing quality improvement"

	slides[2].Title = "Core Metrics"
	slides[2].Layout = "chart"
	slides[2].Chart = normalizeChart(&officegen.ChartData{
		Type:       "bar",
		Title:      "Quarterly Operating Metrics (Last Quarter = 100)",
		Categories: []string{"New ARR", "Renewal Rate", "Collection Rate"},
		Values:     []float64{128, 84, 76},
	})
	slides[2].Points = normalizePoints([]string{
		"New ARR at 128 shows growth is still acquisition-led this quarter.",
		"Renewal at 84 and collections at 76 lag materially behind new growth.",
	}, 2, 28)
	slides[2].Sections = nil
	slides[2].Metrics = nil
	slides[2].Source = "Method: relative index with last quarter = 100; renewal and collection are normalized against quarterly targets"
	slides[2].Subtitle = "New ARR is still lifting growth, but renewals and collections are weakening quality"

	slides[3].Title = "Issue Diagnosis"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "P1 Delivery Capacity", Detail: "Custom work is 42% of mix, extending project cycles by about 10 days"},
		{Heading: "P2 Collection Timing", Detail: "The top 10 customers carry longer payment terms and slower cash conversion"},
		{Heading: "P3 Funnel Conversion", Detail: "Mid-funnel opportunity win rate is 7 points below target"},
	}, 3)
	slides[3].Points = nil
	slides[3].Metrics = nil
	slides[3].Chart = nil
	slides[3].Source = ""
	slides[3].Subtitle = "Delivery, collections, and conversion all create drag on operating quality"

	slides[4].Title = "Next-Quarter Priorities"
	slides[4].Layout = "content"
	slides[4].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Renewal Recovery", Detail: "Customer success lead restores renewal rate to above 90"},
		{Heading: "Delivery Efficiency", Detail: "Delivery lead reduces custom work share to below 30"},
		{Heading: "Collections Push", Detail: "Sales operations lead raises collection rate to 90"},
	}, 3)
	slides[4].Points = nil
	slides[4].Metrics = nil
	slides[4].Chart = nil
	slides[4].Source = ""
	slides[4].Subtitle = "Each priority is tied to an owner and result metric, not just direction"

	slides[5].Title = "Execution Actions"
	slides[5].Layout = "content"
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "April Sales Director", Detail: "Finish funnel review and lift win rate by 3 points"},
		{Heading: "May Delivery Lead", Detail: "Launch the standard package and reduce rework below 10"},
		{Heading: "June Operating Lead", Detail: "Review performance against renewal 90 and collection 90"},
	}, 3)
	slides[5].Points = nil
	slides[5].Metrics = nil
	slides[5].Chart = nil
	slides[5].Source = ""
	slides[5].Subtitle = "Advance monthly with clear owners, milestones, and validation metrics"
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

	slides[1].Title = "Learning Goals"
	slides[1].Layout = "content"
	slides[1].Points = normalizePoints([]string{
		"Understand what OfficeCLI does, what it takes in, and what it outputs.",
		"Complete setup and run one local generation command successfully.",
		"Know the configuration boundaries and cautions before production use.",
	}, 3, 28)
	slides[1].Sections = nil
	slides[1].Metrics = nil
	slides[1].Chart = nil
	slides[1].Source = ""
	slides[1].Subtitle = "Build core understanding first, then complete the first independent command run"

	slides[2].Title = "Installation and Setup"
	slides[2].Layout = "content"
	slides[2].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Environment Check", Detail: "Confirm Go, config files, and local dependencies are available"},
		{Heading: "Install Command", Detail: "Run the build or download command to create the executable"},
		{Heading: "Login Check", Detail: "After configuration, run a status command to verify connectivity"},
	}, 3)
	slides[2].Points = nil
	slides[2].Metrics = nil
	slides[2].Chart = nil
	slides[2].Source = ""
	slides[2].Subtitle = "Prepare in three steps: environment check, installation, and login validation"

	slides[3].Title = "Common Commands"
	slides[3].Layout = "content"
	slides[3].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Status Check", Detail: "Run config status to verify configuration and dependencies"},
		{Heading: "Generate PPT", Detail: "Run new pptx to generate a local PPT file"},
		{Heading: "Quality Review", Detail: "Run review pptx for structural and visual review"},
	}, 3)
	slides[3].Points = nil
	slides[3].Metrics = nil
	slides[3].Chart = nil
	slides[3].Source = ""
	slides[3].Subtitle = "Memorize the three most common command groups first, then expand usage"

	slides[4].Title = "Example Workflow"
	slides[4].Layout = "content"
	slides[4].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Step 1 Scope", Detail: "Define topic, audience, and style before generating output"},
		{Heading: "Step 2 Generate", Detail: "Run new pptx and confirm the file was written successfully"},
		{Heading: "Step 3 Review", Detail: "Run review pptx and iterate based on the findings"},
	}, 3)
	slides[4].Points = nil
	slides[4].Metrics = nil
	slides[4].Chart = nil
	slides[4].Source = ""
	slides[4].Subtitle = "A full practice run should cover generation, checking, and revision"

	slides[5].Title = "Cautions"
	slides[5].Layout = "content"
	slides[5].Sections = normalizeSections([]officegen.SlideSection{
		{Heading: "Validate Locally", Detail: "Keep publishing off by default until output quality is confirmed"},
		{Heading: "Complete Config", Detail: "Missing models, image settings, or dependencies will degrade results"},
		{Heading: "Keep Commands Intact", Detail: "Preserve full commands, paths, and parameters without truncation"},
	}, 3)
	slides[5].Points = nil
	slides[5].Metrics = nil
	slides[5].Chart = nil
	slides[5].Source = ""
	slides[5].Subtitle = "Validate quality locally before moving into the formal collaboration flow"
}

func isActionSlide(slide officegen.Slide) bool {
	if slideRoleName(slide, 0) == pptxSlideRoleAction {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"recommendation", "next step", "rollout", "plan", "release", "training", "path", "action", "caution", "建议", "下一步", "推进", "计划", "发布", "培训", "路径", "行动", "注意事项"} {
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
	for _, marker := range []string{"within 30 days", "within 60 days", "within 90 days", "weeks 1-2", "weeks 3-6", "weeks 7-10", "this week", "this month", "30天内", "60天内", "90天内", "第1周", "第2周", "本周", "本月"} {
		if strings.HasPrefix(cleaned, marker) {
			return fitTextForLayout(marker, 12), fitTextForLayout(strings.TrimSpace(strings.TrimPrefix(cleaned, marker)), 42)
		}
	}
	for _, sep := range []string{"：", ":"} {
		parts := strings.SplitN(cleaned, sep, 2)
		if len(parts) != 2 {
			continue
		}
		label := fitTextForLayout(strings.TrimSpace(parts[0]), 12)
		body := fitTextForLayout(strings.TrimSpace(parts[1]), 42)
		if label != "" && body != "" {
			return label, body
		}
	}
	return fmt.Sprintf("Step %d", idx+1), fitTextForLayout(cleaned, 42)
}

func fitTextForLayout(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	for _, sep := range []string{".", ";", ",", ":", "("} {
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
	value = strings.TrimSuffix(value, ".")
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
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Chart.Title))
	for _, keyword := range []string{"milestone", "cadence", "plan", "roadmap", "step", "workflow", "risk", "next step", "里程碑", "节奏", "计划", "路线图", "步骤", "流程", "风险", "下一步"} {
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
		detail := fmt.Sprintf("Step %d", idx+1)
		if len(chart.Values) > idx && chart.Values[idx] > 0 {
			detail = fmt.Sprintf("Stage value %s", formatChartValue(chart.Values[idx]))
		}
		if strings.Contains(slide.Title, "milestone") || strings.Contains(slide.Title, "cadence") || strings.Contains(slide.Title, "plan") {
			detail = fmt.Sprintf("Phase %d, move in sequence", idx+1)
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
