package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	publishprovider "github.com/officecli/officecli/internal/providers/publish"
	"github.com/officecli/officecli/internal/runtime"
)

func (a *App) executeGenerateJob(ctx context.Context, cfg Config, job GenerateJob, isTTY bool, progress progressController, prompter Prompter) (GenerateResult, error) {
	if job.DocumentType == engine.DocumentTypeIMG {
		return a.executeHostedImageJob(ctx, cfg, job, progress)
	}
	if missing := missingLLMConfig(cfg); missing != "" {
		if !shouldSkipLocalLLM(job.RuntimeMode) {
			return GenerateResult{}, fmt.Errorf("generation service is not fully configured: missing %s. Run `officecli config set-generation` to finish setup", missing)
		}
	}
	if job.RuntimeMode == RuntimeModeHosted {
		if missing := missingHostedConfig(cfg); missing != "" {
			return GenerateResult{}, fmt.Errorf("platform service is not fully configured: missing %s. Run `officecli config set-license` to finish setup", missing)
		}
	}

	if _, err := a.runLicenseCheck(ctx, cfg.License, job.RuntimeMode, string(job.DocumentType), "status", "Checking access status", progress); err != nil {
		return GenerateResult{}, err
	}

	var (
		err       error
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
	licenseCheck, err := a.runLicenseCheck(ctx, cfg.License, job.RuntimeMode, string(job.DocumentType), "generate", "Refreshing access status before generation", progress)
	if err != nil {
		return GenerateResult{}, err
	}
	job.LicenseCheck = licenseCheck

	publishCfg := cfg.Publish
	if strings.TrimSpace(publishCfg.APIKey) == "" {
		publishCfg.APIKey = strings.TrimSpace(cfg.License.APIKey)
	}
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

func (a *App) executeHostedImageJob(ctx context.Context, cfg Config, job GenerateJob, progress progressController) (GenerateResult, error) {
	if strings.TrimSpace(cfg.License.BaseURL) == "" {
		missing := "platform service URL"
		return GenerateResult{}, fmt.Errorf("platform service is not fully configured: missing %s. Run `officecli config set-license` to finish setup", missing)
	}
	job.RuntimeMode = RuntimeModeExternal
	job.Mode = "fast"
	job.Publish = false

	licenseCheck, err := a.runLicenseCheck(ctx, cfg.License, job.RuntimeMode, string(job.DocumentType), "generate", "Checking image generation access", progress)
	if err != nil {
		return GenerateResult{}, err
	}
	job.LicenseCheck = licenseCheck

	llmClient, err := newHostedLLMClient(cfg.License, job)
	if err != nil {
		return GenerateResult{}, buildHostedModeError(err)
	}
	executor := NewExecutor(runtime.NewService(llmClient, progress), nil, nil)
	executor.progress = progress
	return executor.Run(ctx, job)
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
		mode = "fast"
	}
	mode = strings.ToLower(mode)
	switch mode {
	case "fast", "best":
	default:
		return GenerateJob{}, fmt.Errorf("unsupported mode: %s", mode)
	}
	if mode == "best" && !req.Interactive {
		return GenerateJob{}, fmt.Errorf("--mode best is not supported when interactive=false")
	}
	if documentType == engine.DocumentTypeIMG && modeSpecified && mode == "best" {
		return GenerateJob{}, fmt.Errorf("--mode best is not supported for img generation")
	}

	runtimeMode := RuntimeMode(strings.ToLower(strings.TrimSpace(req.Args.RuntimeMode)))
	if documentType == engine.DocumentTypeIMG && runtimeMode == "" {
		runtimeMode = RuntimeModeExternal
	} else if runtimeMode == "" {
		runtimeMode = cfg.Runtime.Mode
	}
	if runtimeMode == "" {
		runtimeMode = RuntimeModeExternal
	}
	switch runtimeMode {
	case RuntimeModeExternal, RuntimeModeHosted:
	default:
		return GenerateJob{}, fmt.Errorf("unsupported runtime mode: %s", runtimeMode)
	}
	if documentType == engine.DocumentTypeIMG {
		runtimeMode = RuntimeModeExternal
	}

	outputDir := strings.TrimSpace(req.Args.OutputDir)
	if outputDir == "" {
		outputDir = strings.TrimSpace(cfg.Defaults.OutputDir)
	}
	if outputDir == "" {
		outputDir = "./output"
	}

	publish := cfg.Defaults.Publish
	if req.Args.Publish != nil {
		publish = *req.Args.Publish
	}
	var warnings []engine.GenerateIssue
	if documentType == engine.DocumentTypeIMG {
		if req.Args.Publish != nil && *req.Args.Publish {
			return GenerateJob{}, fmt.Errorf("publish is not supported for img generation")
		}
		if publish {
			warnings = append(warnings, engine.GenerateIssue{
				Code:    "WARN_IMG_PUBLISH_UNSUPPORTED",
				Message: "Image publishing is not supported yet, so the generated image will be saved locally only.",
				Field:   "publish",
			})
			publish = false
		}
	}
	enableImages := true
	if req.Args.EnableImages != nil {
		enableImages = *req.Args.EnableImages
	}

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

	return GenerateJob{
		DocumentType:   documentType,
		Topic:          topic,
		OriginalPrompt: prompt,
		Prompt:         prompt,
		SourceFilePath: sourceFile,
		RuntimeMode:    runtimeMode,
		Mode:           mode,
		Language:       strings.TrimSpace(req.Args.Language),
		Style:          style,
		StyleSpecified: styleSpecified,
		Audience:       strings.TrimSpace(req.Args.Audience),
		EnableImages:   enableImages,
		ImageRatio:     imageRatio,
		LocalPreview:   false,
		OutputDir:      outputDir,
		Publish:        publish,
		JSONOutput:     true,
		Warnings:       warnings,
	}, nil
}
