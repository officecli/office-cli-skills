package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	publishprovider "github.com/officecli/officecli/internal/providers/publish"
	reviewprovider "github.com/officecli/officecli/internal/review"
	"github.com/officecli/officecli/internal/runtime"
)

func (a *App) executeGenerateJob(ctx context.Context, cfg Config, job GenerateJob, isTTY bool, progress progressController, prompter Prompter) (GenerateResult, error) {
	if isStandaloneImageDocumentType(job.DocumentType) {
		if job.RuntimeMode != RuntimeModeHosted {
			return a.executeExternalImageJob(ctx, cfg, job, progress)
		}
		return a.executeHostedImageJob(ctx, cfg, job, progress)
	}
	if missing := missingLLMConfig(cfg); missing != "" {
		if !shouldSkipLocalLLM(job.RuntimeMode) {
			return GenerateResult{}, fmt.Errorf("generation service is not fully configured: missing %s. Run `officecli config set-generation` to finish setup", missing)
		}
	}
	if job.RuntimeMode == RuntimeModeHosted {
		if missing := missingHostedConfig(cfg); missing != "" {
			return GenerateResult{}, fmt.Errorf("platform service is not fully configured: missing %s. Run `officecli login` to finish account access setup", missing)
		}
	}

	initialAccessAction := "status"
	if job.RuntimeMode == RuntimeModeHosted {
		initialAccessAction = "generate"
	}
	initialLicenseCheck, err := a.runLicenseCheck(ctx, cfg.License, job.RuntimeMode, string(job.DocumentType), initialAccessAction, "Checking access status", progress)
	if err != nil {
		return GenerateResult{}, err
	}
	if job.RuntimeMode == RuntimeModeHosted {
		job.LicenseCheck = initialLicenseCheck
	}

	var (
		llmClient GeneratorLLMClient
	)
	if job.RuntimeMode == RuntimeModeHosted {
		llmClient, err = newHostedLLMClient(cfg.License, job)
		if err != nil {
			return GenerateResult{}, buildHostedModeError(err)
		}
	} else {
		llmClient, err = a.newLLMClient(cfg.LLM)
		if err != nil {
			return GenerateResult{}, err
		}
	}
	imageLLMClient, imageWarnings, err := a.pptxImageClient(ctx, cfg, job)
	if err != nil {
		return GenerateResult{}, err
	}
	job.Warnings = append(job.Warnings, imageWarnings...)

	job, err = a.preparePPTPrompt(ctx, llmClient, job, progress)
	if err != nil {
		return GenerateResult{}, err
	}

	if job.Mode == generateengine.ModeBest {
		if prompter == nil {
			if !isTTY {
				return GenerateResult{}, fmt.Errorf("best mode requires interactive follow-up questions. Run in a TTY or switch to --mode fast")
			}
			prompter = NewConsolePrompter(a.Stdin, a.Stdout)
		}
		job, err = a.completeBestModeWithPrompter(ctx, llmClient, prompter, job, progress)
		if err != nil {
			return GenerateResult{}, err
		}
	}
	if job.LicenseCheck == nil {
		licenseCheck, err := a.runLicenseCheck(ctx, cfg.License, job.RuntimeMode, string(job.DocumentType), "generate", "Refreshing access status before generation", progress)
		if err != nil {
			return GenerateResult{}, err
		}
		job.LicenseCheck = licenseCheck
	}

	publishCfg := publishConfigForLicense(cfg.Publish, cfg.License)
	publisher, err := publishprovider.NewPublisher(publishCfg)
	if err != nil {
		return GenerateResult{}, err
	}
	manager, err := a.newLicenseService(cfg.License)
	if err != nil {
		return GenerateResult{}, err
	}
	service := runtime.NewService(llmClient, progress)
	if imageLLMClient != nil {
		service.WithImageLLM(imageLLMClient)
	}
	if previewReviewer := pptxArtifactPreviewReviewerForConfig(cfg, job); previewReviewer != nil {
		service.WithPPTXArtifactPreviewReviewer(previewReviewer)
	}
	executor := NewExecutor(service, publisher, manager)
	executor.progress = progress
	return executor.Run(ctx, job)
}

func (a *App) executeExternalImageJob(ctx context.Context, cfg Config, job GenerateJob, progress progressController) (GenerateResult, error) {
	if missing := missingLLMConfig(cfg); missing != "" {
		return GenerateResult{}, fmt.Errorf("generation service is not fully configured: missing %s. Run `officecli config set-generation` to finish setup", missing)
	}
	if job.RuntimeMode == "" {
		job.RuntimeMode = RuntimeModeExternal
	}
	job.Mode = "fast"
	var err error
	job, err = a.applyImagePromptTemplate(ctx, cfg, job)
	if err != nil {
		return GenerateResult{}, err
	}

	licenseCheck, err := a.runLicenseCheck(ctx, cfg.License, job.RuntimeMode, string(job.DocumentType), "generate", "Checking image generation access", progress)
	if err != nil {
		return GenerateResult{}, err
	}
	job.LicenseCheck = licenseCheck
	if len(job.ReferenceImageSources) > 0 {
		references, err := resolveReferenceImages(ctx, job.ReferenceImageSources)
		if err != nil {
			return GenerateResult{}, err
		}
		job.ReferenceImages = references
	}

	llmClient, err := a.newLLMClient(cfg.LLM)
	if err != nil {
		return GenerateResult{}, err
	}
	publishCfg := publishConfigForLicense(cfg.Publish, cfg.License)
	publisher, err := publishprovider.NewPublisher(publishCfg)
	if err != nil {
		return GenerateResult{}, err
	}
	manager, err := a.newLicenseService(cfg.License)
	if err != nil {
		return GenerateResult{}, err
	}
	executor := NewExecutor(runtime.NewService(llmClient, progress), publisher, manager)
	executor.progress = progress
	return executor.Run(ctx, job)
}

func (a *App) pptxImageClient(_ context.Context, cfg Config, job GenerateJob) (GeneratorLLMClient, []engine.GenerateIssue, error) {
	if job.DocumentType != engine.DocumentTypePPTX || !job.EnableImages {
		return nil, nil, nil
	}
	if missing := missingHostedConfig(cfg); missing != "" {
		return nil, []engine.GenerateIssue{{
			Code:    "WARN_PPT_PREMIUM_IMAGE_DEGRADED",
			Message: fmt.Sprintf("PPT images require account hosted credits, but platform service is not fully configured: missing %s. The deck will be generated without images. Run `officecli login` or purchase account hosted credits.", missing),
			Field:   "image_quality",
		}}, nil
	}
	llm, err := newHostedImageLLMClient(cfg.License)
	if err != nil {
		return nil, []engine.GenerateIssue{{
			Code:    "WARN_PPT_PREMIUM_IMAGE_DEGRADED",
			Message: fmt.Sprintf("PPT images are unavailable: %v. The deck will be generated without images. Run `officecli login` or purchase account hosted credits.", err),
			Field:   "image_quality",
		}}, nil
	}
	return llm, nil, nil
}

func (a *App) executeHostedImageJob(ctx context.Context, cfg Config, job GenerateJob, progress progressController) (GenerateResult, error) {
	if strings.TrimSpace(cfg.License.BaseURL) == "" {
		missing := "platform service URL"
		return GenerateResult{}, fmt.Errorf("platform service is not fully configured: missing %s. Run `officecli login` to finish account access setup", missing)
	}
	licenseCfg := cfg.License
	licenseCfg.Enabled = true
	if job.RuntimeMode == "" {
		job.RuntimeMode = RuntimeModeExternal
	}
	job.Mode = "fast"
	var err error
	job, err = a.applyImagePromptTemplate(ctx, cfg, job)
	if err != nil {
		return GenerateResult{}, err
	}

	licenseCheck, err := a.runLicenseCheck(ctx, licenseCfg, job.RuntimeMode, string(job.DocumentType), "generate", "Checking image generation access", progress)
	if err != nil {
		return GenerateResult{}, err
	}
	job.LicenseCheck = licenseCheck
	if len(job.ReferenceImageSources) > 0 {
		references, err := resolveReferenceImages(ctx, job.ReferenceImageSources)
		if err != nil {
			return GenerateResult{}, err
		}
		job.ReferenceImages = references
	}

	llmClient, err := newHostedLLMClient(licenseCfg, job)
	if err != nil {
		return GenerateResult{}, buildHostedModeError(err)
	}
	publishCfg := publishConfigForLicense(cfg.Publish, licenseCfg)
	publisher, err := publishprovider.NewPublisher(publishCfg)
	if err != nil {
		return GenerateResult{}, err
	}
	executor := NewExecutor(runtime.NewService(llmClient, progress), publisher, nil)
	executor.progress = progress
	return executor.Run(ctx, job)
}

func publishConfigForLicense(publishCfg publishprovider.Config, licenseCfg LicenseConfig) publishprovider.Config {
	if strings.TrimSpace(licenseCfg.SessionToken) != "" && publishUsesLicensePlatform(publishCfg, licenseCfg) {
		publishCfg.APIKey = strings.TrimSpace(licenseCfg.SessionToken)
		return publishCfg
	}
	if strings.TrimSpace(publishCfg.APIKey) == "" {
		publishCfg.APIKey = strings.TrimSpace(licenseCfg.APIKey)
	}
	return publishCfg
}

type pptxArtifactPreviewReviewerAdapter struct {
	reviewer *reviewprovider.OpenAIReviewer
}

func pptxArtifactPreviewReviewerForConfig(cfg Config, job GenerateJob) runtime.PPTXArtifactPreviewReviewer {
	if job.DocumentType != engine.DocumentTypePPTX || strings.TrimSpace(job.PPTXBackend) != runtime.PPTXBackendArtifactExperimental {
		return nil
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.LLM.Provider))
	if provider != "" && provider != "openai" {
		return nil
	}
	if missingLLMConfig(cfg) != "" {
		return nil
	}
	return pptxArtifactPreviewReviewerAdapter{
		reviewer: reviewprovider.NewOpenAIReviewer(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.ReviewModel, cfg.LLM.TimeoutSec),
	}
}

func (r pptxArtifactPreviewReviewerAdapter) ReviewPPTXArtifactPreviews(ctx context.Context, req runtime.PPTXArtifactPreviewReviewRequest) (*runtime.PPTXArtifactPreviewReviewResult, error) {
	if r.reviewer == nil {
		return nil, fmt.Errorf("visual preview reviewer is unavailable")
	}
	pages := make([]reviewprovider.ImageReviewPage, 0, len(req.PreviewFiles))
	for idx, previewPath := range req.PreviewFiles {
		previewPath = strings.TrimSpace(previewPath)
		if previewPath == "" {
			continue
		}
		data, err := os.ReadFile(previewPath)
		if err != nil {
			return nil, fmt.Errorf("read artifact preview image %q: %w", previewPath, err)
		}
		pages = append(pages, reviewprovider.ImageReviewPage{
			Page: idx + 1,
			MIME: "image/png",
			Data: data,
		})
		if len(pages) >= 8 {
			break
		}
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("no artifact preview images were available for visual review")
	}
	visual, err := r.reviewer.ReviewImages(ctx, pages, reviewprovider.StructureReport{
		Score:   100,
		Summary: fmt.Sprintf("Artifact experimental worker rendered %d preview image(s) for %d slide(s).", len(pages), req.SlideCount),
	})
	if err != nil {
		return nil, err
	}
	return convertPPTXArtifactPreviewReviewResult(visual), nil
}

func convertPPTXArtifactPreviewReviewResult(visual *reviewprovider.VisualResult) *runtime.PPTXArtifactPreviewReviewResult {
	if visual == nil {
		return nil
	}
	issues := make([]runtime.PPTXArtifactPreviewReviewIssue, 0, len(visual.Issues))
	for _, issue := range visual.Issues {
		issues = append(issues, runtime.PPTXArtifactPreviewReviewIssue{
			Severity:     issue.Severity,
			Code:         issue.Code,
			Title:        issue.Title,
			Message:      issue.Message,
			SlideNumbers: append([]int(nil), issue.SlideNumbers...),
			Suggestion:   issue.Suggestion,
		})
	}
	return &runtime.PPTXArtifactPreviewReviewResult{
		Score:     visual.Score,
		Summary:   visual.Summary,
		Strengths: append([]string(nil), visual.Strengths...),
		Issues:    issues,
	}
}

func (a *App) applyImagePromptTemplate(ctx context.Context, cfg Config, job GenerateJob) (GenerateJob, error) {
	templateID := strings.TrimSpace(job.PromptTemplateID)
	if templateID == "" {
		return job, nil
	}
	if job.DocumentType != engine.DocumentTypeIMG {
		return GenerateJob{}, fmt.Errorf("prompt_template_id is only supported for img generation")
	}
	var resp struct {
		Prompt string `json:"prompt"`
	}
	path := "/api/image-templates/" + templateID + "/compose"
	if err := a.platformJSON(ctx, cfg.License.BaseURL, http.MethodPost, path, map[string]any{
		"prompt": job.Prompt,
	}, "", &resp); err != nil {
		return GenerateJob{}, fmt.Errorf("compose image prompt template %q: %w", templateID, err)
	}
	prompt := strings.TrimSpace(resp.Prompt)
	if prompt == "" {
		return GenerateJob{}, fmt.Errorf("compose image prompt template %q returned empty prompt", templateID)
	}
	job.Prompt = prompt
	return job, nil
}

func (a *App) listImagePromptTemplates(ctx context.Context, cfg Config) ([]ImagePromptTemplate, error) {
	if strings.TrimSpace(cfg.License.SessionToken) != "" {
		var grouped struct {
			Public  []ImagePromptTemplate `json:"public"`
			Private []ImagePromptTemplate `json:"private"`
		}
		if err := a.platformJSON(ctx, cfg.License.BaseURL, http.MethodGet, "/api/cli/image-templates", nil, cfg.License.SessionToken, &grouped); err == nil {
			templates := append([]ImagePromptTemplate{}, grouped.Public...)
			templates = append(templates, grouped.Private...)
			absolutizeImageTemplateThumbnailURLs(templates, cfg)
			return templates, nil
		}
	}
	var templates []ImagePromptTemplate
	if err := a.platformJSON(ctx, cfg.License.BaseURL, http.MethodGet, "/api/image-templates", nil, "", &templates); err != nil {
		return nil, err
	}
	absolutizeImageTemplateThumbnailURLs(templates, cfg)
	return templates, nil
}

func (a *App) createUserImagePromptTemplate(ctx context.Context, cfg Config, req CreateUserImagePromptTemplateRequest) (*ImagePromptTemplate, error) {
	if strings.TrimSpace(cfg.License.SessionToken) == "" {
		return nil, fmt.Errorf("login is required to create private image templates")
	}
	var template ImagePromptTemplate
	if err := a.platformJSON(ctx, cfg.License.BaseURL, http.MethodPost, "/api/cli/image-templates", req, cfg.License.SessionToken, &template); err != nil {
		return nil, err
	}
	templates := []ImagePromptTemplate{template}
	absolutizeImageTemplateThumbnailURLs(templates, cfg)
	return &templates[0], nil
}

func (a *App) createImageTemplatePublishRequest(ctx context.Context, cfg Config, req CreateImageTemplatePublishRequest) (*ImageTemplatePublishRequest, error) {
	if strings.TrimSpace(cfg.License.SessionToken) == "" {
		return nil, fmt.Errorf("login is required to publish private image templates")
	}
	var publishRequest ImageTemplatePublishRequest
	if err := a.platformJSON(ctx, cfg.License.BaseURL, http.MethodPost, "/api/cli/image-template-publish-requests", req, cfg.License.SessionToken, &publishRequest); err != nil {
		return nil, err
	}
	return &publishRequest, nil
}

func absolutizeImageTemplateThumbnailURLs(templates []ImagePromptTemplate, cfg Config) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.License.BaseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(defaultInitConfig().License.BaseURL), "/")
	}
	for i := range templates {
		thumbnailURL := strings.TrimSpace(templates[i].ThumbnailURL)
		if strings.HasPrefix(thumbnailURL, "/") && baseURL != "" {
			templates[i].ThumbnailURL = baseURL + thumbnailURL
		}
	}
}

func publishUsesLicensePlatform(publishCfg publishprovider.Config, licenseCfg LicenseConfig) bool {
	publishBaseURL := strings.TrimRight(strings.TrimSpace(publishCfg.BaseURL), "/")
	if publishBaseURL == "" {
		publishBaseURL = strings.TrimRight(strings.TrimSpace(publishprovider.EmbeddedPublishBaseURL), "/")
	}
	licenseBaseURL := strings.TrimRight(strings.TrimSpace(licenseCfg.BaseURL), "/")
	if licenseBaseURL == "" {
		licenseBaseURL = strings.TrimRight(strings.TrimSpace(defaultInitConfig().License.BaseURL), "/")
	}
	return strings.EqualFold(publishBaseURL, licenseBaseURL)
}

func (a *App) buildGenerateJobFromRequest(cfg Config, req bridgeInvokeParams) (GenerateJob, error) {
	if strings.TrimSpace(req.Tool) == "" {
		req.Tool = bridgeToolOfficeGenerate
	}
	if req.Tool != bridgeToolOfficeGenerate {
		return GenerateJob{}, fmt.Errorf("unsupported tool: %s", req.Tool)
	}

	documentType, err := parseDocumentType(req.Args.DocumentType)
	if err != nil {
		return GenerateJob{}, err
	}
	topic := strings.TrimSpace(req.Args.Topic)
	if topic == "" {
		return GenerateJob{}, fmt.Errorf("topic is required")
	}

	modeSpecified := strings.TrimSpace(req.Args.Mode) != ""
	mode := strings.TrimSpace(req.Args.Mode)
	if isStandaloneImageDocumentType(documentType) && mode == "" {
		mode = "fast"
	} else if mode == "" {
		mode = strings.TrimSpace(cfg.Defaults.Mode)
	}
	if mode == "" {
		mode = "best"
	}
	mode = strings.ToLower(mode)
	switch mode {
	case "fast", "best":
	default:
		return GenerateJob{}, fmt.Errorf("unsupported mode: %s", mode)
	}
	if mode == "best" && !req.Interactive && !modeSpecified {
		mode = "fast"
	}
	if mode == "best" && !req.Interactive {
		return GenerateJob{}, fmt.Errorf("--mode best is not supported when interactive=false")
	}
	if isStandaloneImageDocumentType(documentType) && modeSpecified && mode == "best" {
		return GenerateJob{}, fmt.Errorf("--mode best is not supported for %s generation", documentType)
	}

	runtimeMode := RuntimeMode(strings.ToLower(strings.TrimSpace(req.Args.RuntimeMode)))
	if isStandaloneImageDocumentType(documentType) && runtimeMode == "" {
		runtimeMode = cfg.RuntimeModeOrDefault()
	} else if runtimeMode == "" {
		runtimeMode = cfg.RuntimeModeOrDefault()
	}
	if runtimeMode == "" {
		runtimeMode = RuntimeModeHosted
	}
	switch runtimeMode {
	case RuntimeModeExternal, RuntimeModeHosted:
	default:
		return GenerateJob{}, fmt.Errorf("unsupported runtime mode: %s", runtimeMode)
	}

	outputDir := strings.TrimSpace(req.Args.OutputDir)
	if outputDir == "" {
		outputDir = strings.TrimSpace(cfg.Defaults.OutputDir)
	}
	if outputDir == "" {
		outputDir = "./output"
	}

	publish := cfg.Defaults.Publish
	if isStandaloneImageDocumentType(documentType) {
		publish = true
	}
	if req.Args.Publish != nil {
		publish = *req.Args.Publish
	}
	var warnings []engine.GenerateIssue
	enableImages := true
	if req.Args.EnableImages != nil {
		enableImages = *req.Args.EnableImages
	}
	// image_quality 字段已废弃：解析后忽略，永远走 hosted image 路径。
	_ = req.Args.ImageQuality

	prompt := strings.TrimSpace(req.Args.Prompt)
	if prompt == "" {
		prompt = topic
	}
	style := strings.TrimSpace(req.Args.Style)
	styleSpecified := style != ""
	if style == "" && documentType == engine.DocumentTypePPTX {
		style = strings.TrimSpace(cfg.Defaults.PPTXStylePreset)
	}
	sourceFile := strings.TrimSpace(req.Args.FilePath)
	if isStandaloneImageDocumentType(documentType) {
		if sourceFile != "" {
			return GenerateJob{}, fmt.Errorf("file_path is not supported for %s generation", documentType)
		}
	} else if documentType == engine.DocumentTypeReport {
		if sourceFile == "" {
			return GenerateJob{}, fmt.Errorf("report generation requires args.file_path")
		}
		if strings.ToLower(filepath.Ext(sourceFile)) != ".xlsx" {
			return GenerateJob{}, fmt.Errorf("report file_path must point to an .xlsx workbook: %s", sourceFile)
		}
	} else if sourceFile != "" {
		return GenerateJob{}, fmt.Errorf("file_path is only supported for report generation")
	}
	imageRatio := strings.ToLower(strings.TrimSpace(req.Args.Ratio))
	if imageRatio != "" && documentType != engine.DocumentTypeIMG {
		return GenerateJob{}, fmt.Errorf("ratio is only supported for img generation")
	}
	if imageRatio == "" {
		imageRatio = "square"
	}
	switch imageRatio {
	case "square", "landscape", "portrait":
	default:
		return GenerateJob{}, fmt.Errorf("unsupported image ratio: %s", req.Args.Ratio)
	}
	referenceImageList := make([]string, 0, len(req.Args.ReferenceImages)+1)
	if legacy := strings.TrimSpace(req.Args.ReferenceImage); legacy != "" {
		referenceImageList = append(referenceImageList, legacy)
	}
	for _, src := range req.Args.ReferenceImages {
		if v := strings.TrimSpace(src); v != "" {
			referenceImageList = append(referenceImageList, v)
		}
	}
	if len(referenceImageList) > 0 && !isStandaloneImageDocumentType(documentType) {
		return GenerateJob{}, fmt.Errorf("reference_image is only supported for img or gif generation")
	}
	referenceScanEnabled, referenceScanRoot, referencePPTXList, err := pptxReferenceOptionsFromBridgeArgs(documentType, req.Args)
	if err != nil {
		return GenerateJob{}, err
	}
	pptxBackend, err := pptxBackendFromBridgeArgs(documentType, req.Args)
	if err != nil {
		return GenerateJob{}, err
	}
	imageSize := strings.TrimSpace(req.Args.Size)
	if imageSize != "" && documentType != engine.DocumentTypeIMG {
		return GenerateJob{}, fmt.Errorf("size is only supported for img generation")
	}
	if imageSize != "" {
		if _, _, err := parseImageSize(imageSize); err != nil {
			return GenerateJob{}, err
		}
	}
	promptTemplateID := strings.TrimSpace(req.Args.PromptTemplateID)
	if promptTemplateID != "" && documentType != engine.DocumentTypeIMG {
		return GenerateJob{}, fmt.Errorf("prompt_template_id is only supported for img generation")
	}
	imageWatermark := req.Args.ImageWatermark
	if imageWatermark != nil && documentType != engine.DocumentTypeIMG {
		return GenerateJob{}, fmt.Errorf("image_watermark is only supported for img generation")
	}
	gifFPS := req.Args.FPS
	if gifFPS != 0 && documentType != engine.DocumentTypeGIF {
		return GenerateJob{}, fmt.Errorf("fps is only supported for gif generation")
	}
	if documentType == engine.DocumentTypeGIF {
		if gifFPS == 0 {
			gifFPS = 16
		}
		if gifFPS < 4 || gifFPS > 24 {
			return GenerateJob{}, fmt.Errorf("fps must be between 4 and 24")
		}
		imageRatio = "square"
		imageSize = "1024x1024"
	}

	localPreview := req.Args.EmitPreview != nil && *req.Args.EmitPreview

	return GenerateJob{
		DocumentType:          documentType,
		Topic:                 topic,
		OriginalPrompt:        prompt,
		Prompt:                prompt,
		SourceFilePath:        sourceFile,
		RuntimeMode:           runtimeMode,
		Mode:                  mode,
		Language:              strings.TrimSpace(req.Args.Language),
		Style:                 style,
		StyleSpecified:        styleSpecified,
		Audience:              strings.TrimSpace(req.Args.Audience),
		EnableImages:          enableImages,
		ImageRatio:            imageRatio,
		ImageSize:             imageSize,
		GifFPS:                gifFPS,
		ImageWatermark:        cloneImageWatermarkOptions(imageWatermark),
		PromptTemplateID:      promptTemplateID,
		ReferenceImageSources: referenceImageList,
		ReferenceScanEnabled:  referenceScanEnabled,
		ReferenceScanRoot:     referenceScanRoot,
		ReferencePPTXSources:  referencePPTXList,
		PPTXBackend:           pptxBackend,
		LocalPreview:          localPreview,
		OutputDir:             outputDir,
		Publish:               publish,
		Debug:                 req.Args.Debug,
		JSONOutput:            true,
		Warnings:              warnings,
	}, nil
}

func cloneImageWatermarkOptions(options *ImageWatermarkOptions) *ImageWatermarkOptions {
	if options == nil {
		return nil
	}
	clone := *options
	return &clone
}

func pptxBackendFromBridgeArgs(documentType engine.DocumentType, args bridgeInvokeArgs) (string, error) {
	backend := strings.TrimSpace(args.PPTXBackend)
	if backend != "" && documentType != engine.DocumentTypePPTX {
		return "", fmt.Errorf("pptx_backend is only supported for pptx generation")
	}
	if documentType != engine.DocumentTypePPTX {
		return "", nil
	}
	return runtime.NormalizePPTXBackend(backend)
}

func pptxReferenceOptionsFromBridgeArgs(documentType engine.DocumentType, args bridgeInvokeArgs) (bool, string, []string, error) {
	referenceRoot := strings.TrimSpace(args.ReferenceRoot)
	referencePPTXList := make([]string, 0, len(args.ReferencePPTXSources)+1)
	if legacy := strings.TrimSpace(args.ReferencePPTX); legacy != "" {
		referencePPTXList = append(referencePPTXList, legacy)
	}
	for _, src := range args.ReferencePPTXSources {
		if v := strings.TrimSpace(src); v != "" {
			referencePPTXList = append(referencePPTXList, v)
		}
	}
	if referenceRoot != "" && documentType != engine.DocumentTypePPTX {
		return false, "", nil, fmt.Errorf("reference_root is only supported for pptx generation")
	}
	if len(referencePPTXList) > 0 && documentType != engine.DocumentTypePPTX {
		return false, "", nil, fmt.Errorf("reference_pptx is only supported for pptx generation")
	}
	if args.EnableReferenceScan != nil && documentType != engine.DocumentTypePPTX {
		return false, "", nil, fmt.Errorf("enable_reference_scan is only supported for pptx generation")
	}
	if documentType != engine.DocumentTypePPTX {
		return false, "", nil, nil
	}
	if err := validateExplicitReferencePPTXFiles(referencePPTXList); err != nil {
		return false, "", nil, err
	}
	referenceScanEnabled := true
	if args.EnableReferenceScan != nil {
		referenceScanEnabled = *args.EnableReferenceScan
	}
	if referenceRoot == "" {
		referenceRoot = "."
	}
	return referenceScanEnabled, referenceRoot, referencePPTXList, nil
}
