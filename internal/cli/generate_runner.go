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

	emitProgress(ctx, progress, progressStepLicense, "running", "Checking access status")
	licenseCheck, err := a.checkLicenseWithRuntime(ctx, cfg.License, job.RuntimeMode, string(job.DocumentType), "generate")
	if err != nil {
		emitProgress(ctx, progress, progressStepLicense, "failed", "Access check failed")
		return GenerateResult{}, err
	}
	emitProgress(ctx, progress, progressStepLicense, "completed", "Access check completed")
	job.LicenseCheck = licenseCheck

	var llmClient GeneratorLLMClient
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

	publisher, err := publishprovider.NewPublisher(cfg.Publish)
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

	mode := strings.TrimSpace(req.Args.Mode)
	if mode == "" {
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

	runtimeMode := RuntimeMode(strings.ToLower(strings.TrimSpace(req.Args.RuntimeMode)))
	if runtimeMode == "" {
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
	enableImages := true
	if req.Args.EnableImages != nil {
		enableImages = *req.Args.EnableImages
	}

	prompt := strings.TrimSpace(req.Args.Prompt)
	if prompt == "" {
		prompt = topic
	}
	style := strings.TrimSpace(req.Args.Style)
	if style == "" && documentType == engine.DocumentTypePPTX {
		style = strings.TrimSpace(cfg.Defaults.PPTXStylePreset)
	}
	sourceFile := strings.TrimSpace(req.Args.FilePath)
	if documentType == engine.DocumentTypeReport {
		if sourceFile == "" {
			return GenerateJob{}, fmt.Errorf("report generation requires args.file_path")
		}
		if strings.ToLower(filepath.Ext(sourceFile)) != ".xlsx" {
			return GenerateJob{}, fmt.Errorf("report file_path must point to an .xlsx workbook: %s", sourceFile)
		}
	} else if sourceFile != "" {
		return GenerateJob{}, fmt.Errorf("file_path is only supported for report generation")
	}

	return GenerateJob{
		DocumentType:   documentType,
		Topic:          topic,
		Prompt:         prompt,
		SourceFilePath: sourceFile,
		RuntimeMode:    runtimeMode,
		Mode:           mode,
		Language:       strings.TrimSpace(req.Args.Language),
		Style:          style,
		Audience:       strings.TrimSpace(req.Args.Audience),
		EnableImages:   enableImages,
		LocalPreview:   false,
		OutputDir:      outputDir,
		Publish:        publish,
		JSONOutput:     true,
	}, nil
}
