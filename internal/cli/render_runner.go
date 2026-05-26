package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/officecli/officecli/engine"
	publishprovider "github.com/officecli/officecli/internal/providers/publish"
	"github.com/officecli/officecli/internal/runtime"
)

func (a *App) executeRenderJob(ctx context.Context, cfg Config, job GenerateJob, payload json.RawMessage, progress progressController) (GenerateResult, error) {
	if job.RuntimeMode == RuntimeModeHosted {
		if missing := missingHostedConfig(cfg); missing != "" {
			return GenerateResult{}, fmt.Errorf("platform service is not fully configured: missing %s. Run `officecli login` to finish account access setup", missing)
		}
	}

	licenseCheck, err := a.runLicenseCheck(ctx, cfg.License, job.RuntimeMode, string(job.DocumentType), "generate", "Checking access status", progress)
	if err != nil {
		return GenerateResult{}, err
	}
	job.LicenseCheck = licenseCheck

	var imageLLM GeneratorLLMClient
	if job.DocumentType == engine.DocumentTypePPTX && job.EnableImages {
		var imageWarnings []engine.GenerateIssue
		imageLLM, imageWarnings, err = a.pptxImageClient(ctx, cfg, job)
		if err != nil {
			return GenerateResult{}, err
		}
		job.Warnings = append(job.Warnings, imageWarnings...)
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
	service := runtime.NewService(imageLLM, progress)
	if imageLLM != nil {
		service.WithImageLLM(imageLLM)
	}
	artifact, err := service.Render(ctx, GenerateParams{
		DocumentType:    job.DocumentType,
		Topic:           job.Topic,
		Prompt:          job.Prompt,
		SourceFilePath:  job.SourceFilePath,
		Mode:            job.Mode,
		Language:        job.Language,
		Style:           job.Style,
		Audience:        job.Audience,
		EnableImages:    job.EnableImages,
		ReferenceImages: append([]engine.ImageReference(nil), job.ReferenceImages...),
		LocalPreview:    job.LocalPreview,
	}, payload)
	if err != nil {
		emitProgress(ctx, progress, progressStepGenerate, "failed", "Document assembly failed")
		return GenerateResult{}, err
	}
	executor := NewExecutor(nil, publisher, manager)
	executor.progress = progress
	return executor.finalizeArtifact(ctx, job, artifact)
}

func (a *App) buildRenderJobFromRequest(cfg Config, req bridgeInvokeParams) (GenerateJob, json.RawMessage, error) {
	if strings.TrimSpace(req.Tool) == "" {
		req.Tool = bridgeToolOfficeRender
	}
	if req.Tool != bridgeToolOfficeRender {
		return GenerateJob{}, nil, fmt.Errorf("unsupported tool: %s", req.Tool)
	}
	if len(req.Args.Payload) == 0 || strings.TrimSpace(string(req.Args.Payload)) == "" {
		return GenerateJob{}, nil, fmt.Errorf("payload is required")
	}

	documentType, err := parseDocumentType(req.Args.DocumentType)
	if err != nil {
		return GenerateJob{}, nil, err
	}
	if documentType == engine.DocumentTypeIMG {
		return GenerateJob{}, nil, fmt.Errorf("office.render does not support img generation; use office.generate with document_type=img")
	}
	topic := strings.TrimSpace(req.Args.Topic)
	if topic == "" {
		topic = strings.TrimSpace(req.Args.Prompt)
	}
	if topic == "" {
		topic = string(documentType)
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
		return GenerateJob{}, nil, fmt.Errorf("unsupported runtime mode: %s", runtimeMode)
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
	// image_quality 字段已废弃：解析后忽略，永远走 hosted image 路径。
	style := strings.TrimSpace(req.Args.Style)
	styleSpecified := style != ""
	if style == "" && documentType == engine.DocumentTypePPTX {
		style = strings.TrimSpace(cfg.Defaults.PPTXStylePreset)
	}

	localPreview := req.Args.EmitPreview != nil && *req.Args.EmitPreview

	return GenerateJob{
		DocumentType:   documentType,
		Topic:          topic,
		OriginalPrompt: topic,
		Prompt:         topic,
		SourceFilePath: strings.TrimSpace(req.Args.FilePath),
		RuntimeMode:    runtimeMode,
		Mode:           "fast",
		Language:       strings.TrimSpace(req.Args.Language),
		Style:          style,
		StyleSpecified: styleSpecified,
		Audience:       strings.TrimSpace(req.Args.Audience),
		EnableImages:   enableImages,
		LocalPreview:   localPreview,
		OutputDir:      outputDir,
		Publish:        publish,
		JSONOutput:     true,
	}, req.Args.Payload, nil
}
