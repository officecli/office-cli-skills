package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/officecli/officecli-internal/engine"
	generateengine "github.com/officecli/officecli-internal/engine/generate"
	publishprovider "github.com/officecli/officecli-internal/internal/providers/publish"
	"github.com/officecli/officecli-internal/internal/runtime"
)

func (a *App) executeGenerateJob(ctx context.Context, cfg Config, job GenerateJob, isTTY bool, progress progressController, prompter Prompter) (GenerateResult, error) {
	if job.DocumentType == engine.DocumentTypeIMG {
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
	if documentType == engine.DocumentTypeIMG && mode == "" {
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
	if documentType == engine.DocumentTypeIMG && modeSpecified && mode == "best" {
		return GenerateJob{}, fmt.Errorf("--mode best is not supported for img generation")
	}

	runtimeMode := RuntimeMode(strings.ToLower(strings.TrimSpace(req.Args.RuntimeMode)))
	if documentType == engine.DocumentTypeIMG && runtimeMode == "" {
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
	if documentType == engine.DocumentTypeIMG {
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
	if documentType == engine.DocumentTypeIMG {
		if sourceFile != "" {
			return GenerateJob{}, fmt.Errorf("file_path is not supported for img generation")
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
	if len(referenceImageList) > 0 && documentType != engine.DocumentTypeIMG {
		return GenerateJob{}, fmt.Errorf("reference_image is only supported for img generation")
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
		ReferenceImageSources: referenceImageList,
		LocalPreview:          localPreview,
		OutputDir:             outputDir,
		Publish:               publish,
		JSONOutput:            true,
		Warnings:              warnings,
	}, nil
}
