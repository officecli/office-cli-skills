package cli

import (
	"context"
	"fmt"
	"strings"

	generateengine "github.com/officecli/officecli/engine/generate"
	publishprovider "github.com/officecli/officecli/internal/providers/publish"
	"github.com/officecli/officecli/internal/runtime"
)

func (a *App) executeGenerateJob(ctx context.Context, cfg Config, job GenerateJob, isTTY bool, progress progressController, prompter Prompter) (GenerateResult, error) {
	if missing := missingLLMConfig(cfg); missing != "" {
		if !shouldSkipLocalLLM(job.RuntimeMode) {
			return GenerateResult{}, fmt.Errorf("生成服务未完成配置：缺少%s。请先运行 `officecli config set-generation` 完成配置", missing)
		}
	}
	if job.RuntimeMode == RuntimeModeHosted {
		if missing := missingHostedConfig(cfg); missing != "" {
			return GenerateResult{}, fmt.Errorf("平台服务未完成配置：缺少%s。请先运行 `officecli config set-license` 完成配置", missing)
		}
	}

	emitProgress(ctx, progress, progressStepLicense, "running", "正在校验授权")
	licenseCheck, err := a.checkLicenseWithRuntime(ctx, cfg.License, job.RuntimeMode, string(job.DocumentType), "generate")
	if err != nil {
		emitProgress(ctx, progress, progressStepLicense, "failed", "授权校验失败")
		return GenerateResult{}, err
	}
	emitProgress(ctx, progress, progressStepLicense, "completed", "授权校验完成")
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
				return GenerateResult{}, fmt.Errorf("best 模式需要交互补问，请在 TTY 中运行或改用 --mode fast")
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
		return GenerateJob{}, fmt.Errorf("interactive=false 时不支持 --mode best")
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

	return GenerateJob{
		DocumentType: documentType,
		Topic:        topic,
		Prompt:       prompt,
		RuntimeMode:  runtimeMode,
		Mode:         mode,
		Language:     strings.TrimSpace(req.Args.Language),
		Style:        strings.TrimSpace(req.Args.Style),
		Audience:     strings.TrimSpace(req.Args.Audience),
		EnableImages: enableImages,
		OutputDir:    outputDir,
		Publish:      publish,
		JSONOutput:   true,
	}, nil
}
