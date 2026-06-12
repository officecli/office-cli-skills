package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/officecli/officecli/engine"
	"github.com/officecli/officecli/internal/runtime"
	"github.com/officecli/officecli/pkg/officegen"
)

type blockingLLMClient struct {
	jsonResponse string
	wait         <-chan struct{}
}

func (b blockingLLMClient) CompleteText(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (b blockingLLMClient) CompleteJSON(ctx context.Context, _ []engine.LLMMessage) (string, error) {
	if b.wait != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-b.wait:
		}
	}
	return b.jsonResponse, nil
}

func (b blockingLLMClient) CompleteStructured(context.Context, engine.StructuredCompletionRequest) (string, error) {
	return "", nil
}

func (b blockingLLMClient) GenerateImage(context.Context, engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	return nil, nil
}

func TestAgentBridgeInitializeAndInvoke(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	markerPath := filepath.Join(tmpDir, "bridge-preflight-ran")
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("HOME", homeDir)
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	t.Setenv(officeTaskPreflightSkipEnv, "0")
	writeTestPreflightScript(t, filepath.Join(homeDir, ".codex", "skills", "officecli", "fix-officecli-env.sh"), "#!/usr/bin/env bash\nset -euo pipefail\n: > \""+markerPath+"\"\n")
	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		Runtime:  RuntimeConfig{Mode: RuntimeModeExternal},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	outReader := bufio.NewReader(outR)
	app := NewApp(outW, bytes.NewBuffer(nil), inR)
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is built for enterprise teams."]}]}`}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx, []string{"agent-bridge"})
	}()

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"})
	initMsg := readRPC(t, outReader)
	if initMsg["result"] == nil {
		t.Fatalf("initialize result missing: %#v", initMsg)
	}
	initResult := initMsg["result"].(map[string]any)
	capabilities, ok := initResult["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities missing: %#v", initResult)
	}
	docGen, ok := capabilities["document_generation"].(map[string]any)
	if !ok {
		t.Fatalf("document_generation missing: %#v", capabilities)
	}
	imgCaps, ok := docGen["img"].(map[string]any)
	if !ok {
		t.Fatalf("img capabilities missing: %#v", docGen)
	}
	if imgCaps["preferred_tool"] != "office.generate" || imgCaps["agent_render_supported"] != false {
		t.Fatalf("unexpected img capabilities: %#v", imgCaps)
	}
	imageGeneration, ok := capabilities["image_generation"].(map[string]any)
	if !ok {
		t.Fatalf("image_generation missing: %#v", capabilities)
	}
	if imageGeneration["provider_control"] != "server" {
		t.Fatalf("unexpected image_generation capability: %#v", imageGeneration)
	}
	if imageGeneration["publish_supported"] != true || imageGeneration["default_publish"] != true {
		t.Fatalf("unexpected image publish capability: %#v", imageGeneration)
	}
	if imageGeneration["disable_flag"] != "--no-publish" {
		t.Fatalf("unexpected image publish disable flag: %#v", imageGeneration["disable_flag"])
	}
	if imageGeneration["config_command"] != "officecli config set-publish" {
		t.Fatalf("unexpected image publish config command: %#v", imageGeneration["config_command"])
	}
	refCaps, ok := imageGeneration["reference_image"].(map[string]any)
	if !ok {
		t.Fatalf("reference_image capability missing: %#v", imageGeneration)
	}
	if refCaps["supported"] != true || refCaps["invoke_field"] != "reference_image" {
		t.Fatalf("unexpected reference_image capability: %#v", refCaps)
	}
	imgPublishSupport, ok := imgCaps["publish_support"].(map[string]any)
	if !ok {
		t.Fatalf("img publish_support missing: %#v", imgCaps)
	}
	if imgPublishSupport["default_publish"] != true || imgPublishSupport["disable_flag"] != "--no-publish" {
		t.Fatalf("unexpected img publish_support: %#v", imgPublishSupport)
	}
	imgImageGeneration, ok := imgCaps["image_generation"].(map[string]any)
	if !ok {
		t.Fatalf("img image_generation missing: %#v", imgCaps)
	}
	imgRefCaps, ok := imgImageGeneration["reference_image"].(map[string]any)
	if !ok || imgRefCaps["max_count"] != float64(8) {
		t.Fatalf("unexpected img reference_image capability: %#v", imgImageGeneration)
	}
	if imgRefCaps["invoke_field_array"] != "reference_images" {
		t.Fatalf("expected invoke_field_array=reference_images, got %#v", imgRefCaps)
	}
	if _, ok := imgImageGeneration["size"].(map[string]any); !ok {
		t.Fatalf("img size capability missing: %#v", imgImageGeneration)
	}
	pptxCaps, ok := docGen["pptx"].(map[string]any)
	if !ok {
		t.Fatalf("pptx capabilities missing: %#v", docGen)
	}
	imageSupport, ok := pptxCaps["image_support"].(map[string]any)
	if !ok {
		t.Fatalf("pptx image_support missing: %#v", pptxCaps)
	}
	if imageSupport["disable_flag"] != "--no-images" {
		t.Fatalf("unexpected disable_flag: %#v", imageSupport["disable_flag"])
	}
	if imageSupport["config_command"] != "officecli config set-generation" {
		t.Fatalf("unexpected config_command: %#v", imageSupport["config_command"])
	}
	if quality, ok := imageSupport["quality_field"].(string); !ok || !strings.Contains(quality, "deprecated") {
		t.Fatalf("expected quality_field to be marked deprecated, got: %#v", imageSupport["quality_field"])
	}
	if _, ok := imageSupport["quality_values"]; ok {
		t.Fatalf("quality_values should be removed, still present: %#v", imageSupport["quality_values"])
	}
	referenceStyle, ok := pptxCaps["reference_style"].(map[string]any)
	if !ok {
		t.Fatalf("pptx reference_style capability missing: %#v", pptxCaps)
	}
	if referenceStyle["default_recursive_scan"] != true ||
		referenceStyle["invoke_tool"] != "office.generate" ||
		referenceStyle["root_field"] != "reference_root" ||
		referenceStyle["enable_field"] != "enable_reference_scan" ||
		referenceStyle["explicit_field"] != "reference_pptx" ||
		referenceStyle["explicit_field_array"] != "reference_pptxs" {
		t.Fatalf("unexpected reference_style capability: %#v", referenceStyle)
	}
	backends, ok := pptxCaps["pptx_backends"].(map[string]any)
	if !ok {
		t.Fatalf("pptx_backends capability missing: %#v", pptxCaps)
	}
	if backends["default"] != runtime.PPTXBackendOfficegen {
		t.Fatalf("unexpected pptx_backends capability: %#v", backends)
	}
	values := backends["values"]
	if !containsString(values, runtime.PPTXBackendOfficegen) || containsString(values, runtime.PPTXBackendGoSpine) || containsString(values, runtime.PPTXBackendArtifactExperimental) {
		t.Fatalf("unexpected pptx_backends values: %#v", backends)
	}

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/open"})
	sessionMsg := readRPC(t, outReader)
	sessionResult := sessionMsg["result"].(map[string]any)
	sessionID := sessionResult["id"].(string)

	writeRPC(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "task/invoke",
		"params": map[string]any{
			"session_id":    sessionID,
			"tool":          "office.generate",
			"interactive":   false,
			"output_format": "json",
			"args": map[string]any{
				"document_type": "docx",
				"topic":         "Enterprise Collaboration Platform Overview",
				"prompt":        "Introduce this enterprise collaboration platform",
				"mode":          "fast",
				"out":           tmpDir,
				"publish":       false,
			},
		},
	})

	var taskID string
	var sawStarted, sawOutput, sawCompleted bool
	timeout := time.After(3 * time.Second)
	for !sawCompleted {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for bridge events")
		default:
		}
		msg := readRPC(t, outReader)
		if result, ok := msg["result"].(map[string]any); ok {
			if id, ok := result["task_id"].(string); ok {
				taskID = id
			}
			continue
		}
		if msg["method"] != "event" {
			continue
		}
		params := msg["params"].(map[string]any)
		switch params["type"] {
		case bridgeEventTaskStarted:
			sawStarted = true
		case bridgeEventTaskOutput:
			sawOutput = true
		case bridgeEventTaskCompleted:
			sawCompleted = true
		}
	}
	if !sawStarted || !sawOutput || taskID == "" {
		t.Fatalf("missing expected events: started=%t output=%t taskID=%q", sawStarted, sawOutput, taskID)
	}

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 4, "method": "task/status", "params": map[string]any{"task_id": taskID}})
	statusMsg := readRPC(t, outReader)
	status := statusMsg["result"].(map[string]any)
	if status["status"] != "completed" {
		t.Fatalf("unexpected task status: %#v", status)
	}

	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("bridge exited with error: %v", err)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected bridge preflight marker: %v", err)
	}
}

func TestAgentBridgeCapabilitiesGetIncludesPPTImageSupport(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	caps := server.initializeResult(context.Background()).Capabilities

	docGen, ok := caps["document_generation"].(map[string]any)
	if !ok {
		t.Fatalf("document_generation missing: %#v", caps)
	}
	pptxCaps, ok := docGen["pptx"].(map[string]any)
	if !ok {
		t.Fatalf("pptx capabilities missing: %#v", docGen)
	}
	imageSupport, ok := pptxCaps["image_support"].(map[string]any)
	if !ok {
		t.Fatalf("image_support missing: %#v", pptxCaps)
	}
	if imageSupport["default_enabled"] != true {
		t.Fatalf("unexpected default_enabled: %#v", imageSupport["default_enabled"])
	}
	if imageSupport["invoke_field"] != "enable_images" {
		t.Fatalf("unexpected invoke_field: %#v", imageSupport["invoke_field"])
	}
	if pptxCaps["agent_render_supported"] != true {
		t.Fatalf("unexpected agent_render_supported: %#v", pptxCaps["agent_render_supported"])
	}
	if pptxCaps["preferred_tool"] != "office.render" {
		t.Fatalf("unexpected preferred_tool: %#v", pptxCaps["preferred_tool"])
	}
	referenceStyle, ok := pptxCaps["reference_style"].(map[string]any)
	if !ok {
		t.Fatalf("reference_style missing: %#v", pptxCaps)
	}
	if referenceStyle["invoke_tool"] != "office.generate" {
		t.Fatalf("reference_style should be generate-only, got: %#v", referenceStyle)
	}
	if _, ok := pptxCaps["pptx_backends"].(map[string]any); !ok {
		t.Fatalf("pptx_backends missing: %#v", pptxCaps)
	}
	if _, ok := pptxCaps["payload_schema"].(map[string]any); !ok {
		t.Fatalf("payload_schema missing: %#v", pptxCaps)
	}
}

func TestAgentBridgeCapabilitiesIncludeUpdateInfo(t *testing.T) {
	t.Setenv(updateCheckSkipEnv, "0")
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	originalVersion := Version
	originalBuildDate := BuildDate
	Version = "0.2.2"
	BuildDate = "2026-04-09T09:07:59Z"
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
	}()
	app.checkForUpdates = func(ctx context.Context) (UpdateInfo, error) {
		return UpdateInfo{
			Available:           true,
			CurrentVersion:      "0.2.2",
			LatestVersionLabel:  "0.2.6",
			InstallMethod:       InstallMethodScript,
			Channel:             UpdateChannelLatest,
			AutoUpdateSupported: true,
			UpdateCommand:       "curl -fsSL https://example.com/install.sh | bash",
		}, nil
	}
	server := newAgentBridgeServer(app, Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	caps := server.initializeResult(context.Background()).Capabilities
	updateCaps, ok := caps["update"].(map[string]any)
	if !ok {
		t.Fatalf("update capabilities missing: %#v", caps)
	}
	if updateCaps["available"] != true {
		t.Fatalf("unexpected available: %#v", updateCaps["available"])
	}
	if updateCaps["update_command"] != "curl -fsSL https://example.com/install.sh | bash" {
		t.Fatalf("unexpected update command: %#v", updateCaps["update_command"])
	}
}

func TestAgentBridgeCapabilitiesExposePrepareAndRenderTools(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	tools := server.initializeResult(context.Background()).Tools
	names := make([]string, 0, len(tools))
	var generateSchema map[string]any
	for _, tool := range tools {
		if name, ok := tool["name"].(string); ok {
			names = append(names, name)
			if name == "office.generate" {
				generateSchema, _ = tool["input_schema"].(map[string]any)
			}
		}
	}
	for _, expected := range []string{"office.prepare", "office.render", "office.generate"} {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing tool %q in %#v", expected, names)
		}
	}
	if generateSchema["publish"] != "boolean" {
		t.Fatalf("office.generate publish schema = %#v", generateSchema["publish"])
	}
}

func containsString(items any, want string) bool {
	switch typed := items.(type) {
	case []string:
		for _, item := range typed {
			if item == want {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if item == want {
				return true
			}
		}
	}
	return false
}

func TestAgentBridgePrepareReportReturnsWorkbookContext(t *testing.T) {
	tmpDir := t.TempDir()
	workbookBytes, err := officegen.NewXLSXGenerator().Generate([]officegen.XlsxSheet{
		{
			Name: "Summary",
			Rows: [][]string{
				{"Region", "Revenue"},
				{"North America", "128"},
				{"Europe", "96"},
			},
		},
	}, officegen.XLSXOptions{Title: "Q2 Review", Creator: "OfficeCLI"})
	if err != nil {
		t.Fatalf("Generate workbook: %v", err)
	}
	workbookPath := filepath.Join(tmpDir, "source.xlsx")
	if err := os.WriteFile(workbookPath, workbookBytes, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- newAgentBridgeServer(NewApp(outW, bytes.NewBuffer(nil), inR), Config{}, inR, outW, bytes.NewBuffer(nil)).Serve(ctx)
	}()
	outReader := bufio.NewReader(outR)

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "task/invoke", "params": map[string]any{
		"tool":          "office.prepare",
		"output_format": "json",
		"args": map[string]any{
			"document_type": "report",
			"topic":         "Q2 Review",
			"file_path":     workbookPath,
		},
	}})

	var sawCompleted bool
	var taskID string
	eventTaskIDs := map[string]struct{}{}
	timeout := time.After(3 * time.Second)
	for !sawCompleted {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for prepare events")
		default:
		}
		msg := readRPC(t, outReader)
		if result, ok := msg["result"].(map[string]any); ok {
			if id, ok := result["task_id"].(string); ok {
				taskID = id
			}
			continue
		}
		if msg["method"] != "event" {
			continue
		}
		params := msg["params"].(map[string]any)
		if eventTaskID, ok := params["task_id"].(string); ok {
			eventTaskIDs[eventTaskID] = struct{}{}
		}
		if params["type"] == bridgeEventTaskCompleted {
			sawCompleted = true
		}
	}
	if taskID == "" {
		t.Fatal("expected task_id from invoke response")
	}
	if len(eventTaskIDs) != 1 {
		t.Fatalf("expected single distinct event task_id, got %v", eventTaskIDs)
	}
	if _, ok := eventTaskIDs[taskID]; !ok {
		t.Fatalf("event task_id set %v does not contain invoke task_id %q", eventTaskIDs, taskID)
	}

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "task/status", "params": map[string]any{"task_id": taskID}})
	statusMsg := readRPC(t, outReader)
	status := statusMsg["result"].(map[string]any)
	result := status["result"].(map[string]any)
	if result["preferred_tool"] != "office.render" {
		t.Fatalf("unexpected preferred_tool: %#v", result)
	}
	if !strings.Contains(result["workbook_summary"].(string), "North America") {
		t.Fatalf("unexpected workbook_summary: %#v", result["workbook_summary"])
	}
	if _, ok := result["payload_schema"].(map[string]any); !ok {
		t.Fatalf("payload_schema missing: %#v", result)
	}

	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("bridge exited with error: %v", err)
	}
}

func TestAgentBridgeRenderSupportsAllDocumentTypes(t *testing.T) {
	tmpDir := t.TempDir()
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		t.Fatal("render path should not initialize llm for these cases")
		return nil, nil
	}
	server := newAgentBridgeServer(app, Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))

	cases := []struct {
		name         string
		documentType string
		topic        string
		payload      string
		enableImages *bool
		wantExt      string
	}{
		{
			name:         "docx",
			documentType: "docx",
			topic:        "Quarterly Brief",
			payload:      `{"title":"Quarterly Brief","sections":[{"heading":"Summary","level":1,"paragraphs":["Delivery-ready content."]}]}`,
			wantExt:      ".docx",
		},
		{
			name:         "xlsx",
			documentType: "xlsx",
			topic:        "Sales Workbook",
			payload:      `{"title":"Sales Workbook","sheets":[{"name":"Pipeline","headers":["Region","Amount"],"rows":[["East","100"],["West","120"]]}]}`,
			wantExt:      ".xlsx",
		},
		{
			name:         "report",
			documentType: "report",
			topic:        "Q2 Review",
			payload:      `{"title":"Q2 Review","sections":[{"title":"Demand momentum","narrative":["North America stayed ahead of plan."],"takeaways":["Keep the pipeline focused."]}]}`,
			wantExt:      ".html",
		},
		{
			name:         "pptx",
			documentType: "pptx",
			topic:        "Platform Overview",
			payload:      `{"title":"Platform Overview","stylePreset":"editorial-light","theme":null,"slides":[{"title":"Platform Overview","content":"","isTitle":true,"layout":"title","variant":"title-center","narrativeRole":"cover","sectionIndex":0,"sectionTitle":"","subtitle":"What the platform is and why it matters","points":[],"sections":[],"chart":null,"metrics":[],"source":"","bgColor":"","bgColor2":"","hasImage":false,"imagePrompt":"","imagePos":"","visuals":[]}]}`,
			enableImages: boolPtr(false),
			wantExt:      ".pptx",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task, err := server.invokeTask(context.Background(), json.RawMessage(`1`), bridgeInvokeParams{
				Tool:         "office.render",
				OutputFormat: "json",
				Args: bridgeInvokeArgs{
					DocumentType: tc.documentType,
					Topic:        tc.topic,
					Payload:      json.RawMessage(tc.payload),
					OutputDir:    tmpDir,
					Publish:      boolPtr(false),
					EnableImages: tc.enableImages,
				},
			})
			if err != nil {
				t.Fatalf("invokeTask: %v", err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				status, err := server.taskStatus(task.ID)
				if err != nil {
					t.Fatalf("taskStatus: %v", err)
				}
				if status.Status == "completed" {
					result := status.Result.(GenerateResult)
					if filepath.Ext(result.FilePath) != tc.wantExt {
						t.Fatalf("unexpected file path: %s", result.FilePath)
					}
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatalf("timed out waiting for %s render completion", tc.documentType)
		})
	}
}

func TestAgentBridgeRenderRejectsIMG(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	_, _, err := app.buildRenderJobFromRequest(Config{}, bridgeInvokeParams{
		Tool: bridgeToolOfficeRender,
		Args: bridgeInvokeArgs{
			DocumentType: "img",
			Topic:        "Launch Visual",
			Payload:      json.RawMessage(`{"prompt":"demo"}`),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "office.render does not support img generation") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildGenerateJobFromRequest_PPTXImageQualityIsAcceptedAndIgnored(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	// image_quality 已废弃：解析后忽略，不应产生 error 也不应进入 job。
	if _, err := app.buildGenerateJobFromRequest(Config{}, bridgeInvokeParams{
		Tool: "office.generate",
		Args: bridgeInvokeArgs{
			DocumentType: "pptx",
			Topic:        "Enterprise Collaboration Platform",
			Prompt:       "Explain the business value",
			ImageQuality: "premium",
			EnableImages: boolPtr(true),
		},
	}); err != nil {
		t.Fatalf("buildGenerateJobFromRequest premium (ignored): %v", err)
	}

	// 对非 PPTX 也不再报错。
	if _, err := app.buildGenerateJobFromRequest(Config{}, bridgeInvokeParams{
		Tool: "office.generate",
		Args: bridgeInvokeArgs{
			DocumentType: "img",
			Topic:        "Launch Visual",
			ImageQuality: "premium",
		},
	}); err != nil {
		t.Fatalf("buildGenerateJobFromRequest img w/ image_quality should be accepted, got: %v", err)
	}
}

func TestBuildGenerateJobFromRequest_GIFAcceptsFPSAndReferenceImages(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))

	job, err := app.buildGenerateJobFromRequest(Config{
		Defaults: DefaultsConfig{OutputDir: t.TempDir(), Publish: false, Mode: "best"},
		Runtime:  RuntimeConfig{Mode: RuntimeModeHosted},
	}, bridgeInvokeParams{
		Tool: bridgeToolOfficeGenerate,
		Args: bridgeInvokeArgs{
			DocumentType:    "gif",
			Topic:           "Token Reaction",
			Prompt:          "一个女生眨眼说话再笑。",
			FPS:             12,
			ReferenceImages: []string{"reference.png"},
		},
	})
	if err != nil {
		t.Fatalf("buildGenerateJobFromRequest: %v", err)
	}
	if job.DocumentType != engine.DocumentTypeGIF {
		t.Fatalf("document type = %q", job.DocumentType)
	}
	if job.GifFPS != 12 {
		t.Fatalf("gif fps = %d", job.GifFPS)
	}
	if job.Mode != "fast" || job.RuntimeMode != RuntimeModeHosted || !job.Publish {
		t.Fatalf("job defaults = mode %q runtime %q publish %v", job.Mode, job.RuntimeMode, job.Publish)
	}
	if got := job.ReferenceImageSources; len(got) != 1 || got[0] != "reference.png" {
		t.Fatalf("reference image sources = %#v", got)
	}
}

func TestBuildGenerateJobFromRequest_IMGAcceptsImageWatermark(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))

	job, err := app.buildGenerateJobFromRequest(Config{}, bridgeInvokeParams{
		Tool: bridgeToolOfficeGenerate,
		Args: bridgeInvokeArgs{
			DocumentType: "img",
			Topic:        "Launch Visual",
			ImageWatermark: &ImageWatermarkOptions{
				Apply:           true,
				PaidEntitlement: false,
				CanDisable:      false,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildGenerateJobFromRequest: %v", err)
	}
	if job.ImageWatermark == nil || !job.ImageWatermark.Apply {
		t.Fatalf("image watermark = %#v, want enabled", job.ImageWatermark)
	}
	if job.ImageWatermark.PaidEntitlement || job.ImageWatermark.CanDisable {
		t.Fatalf("image watermark = %#v", job.ImageWatermark)
	}
}

func TestBuildGenerateJobFromRequest_RejectsImageWatermarkForNonIMG(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))

	_, err := app.buildGenerateJobFromRequest(Config{}, bridgeInvokeParams{
		Tool: bridgeToolOfficeGenerate,
		Args: bridgeInvokeArgs{
			DocumentType: "gif",
			Topic:        "Token Reaction",
			ImageWatermark: &ImageWatermarkOptions{
				Apply: true,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "image_watermark is only supported for img generation") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildGenerateJobFromRequest_EmitPreviewFlag(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))

	mk := func(flag *bool) GenerateJob {
		job, err := app.buildGenerateJobFromRequest(Config{}, bridgeInvokeParams{
			Tool: "office.generate",
			Args: bridgeInvokeArgs{
				DocumentType: "pptx",
				Topic:        "Quarterly Review",
				EmitPreview:  flag,
			},
		})
		if err != nil {
			t.Fatalf("buildGenerateJobFromRequest: %v", err)
		}
		return job
	}

	if mk(nil).LocalPreview {
		t.Fatal("omitted emit_preview should leave LocalPreview=false")
	}
	if mk(boolPtr(false)).LocalPreview {
		t.Fatal("emit_preview=false should leave LocalPreview=false")
	}
	if !mk(boolPtr(true)).LocalPreview {
		t.Fatal("emit_preview=true should set LocalPreview=true")
	}
}

func TestBuildRenderJobFromRequest_EmitPreviewFlag(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))

	mk := func(flag *bool) GenerateJob {
		job, _, err := app.buildRenderJobFromRequest(Config{}, bridgeInvokeParams{
			Tool: bridgeToolOfficeRender,
			Args: bridgeInvokeArgs{
				DocumentType: "pptx",
				Topic:        "Quarterly Review",
				Payload:      json.RawMessage(`{"title":"x"}`),
				EmitPreview:  flag,
			},
		})
		if err != nil {
			t.Fatalf("buildRenderJobFromRequest: %v", err)
		}
		return job
	}

	if mk(nil).LocalPreview {
		t.Fatal("omitted emit_preview should leave LocalPreview=false")
	}
	if mk(boolPtr(false)).LocalPreview {
		t.Fatal("emit_preview=false should leave LocalPreview=false")
	}
	if !mk(boolPtr(true)).LocalPreview {
		t.Fatal("emit_preview=true should set LocalPreview=true")
	}
}

func TestAgentBridgeGenerateIMGUsesServerImageRoute(t *testing.T) {
	tmpDir := t.TempDir()
	refPath := filepath.Join(tmpDir, "reference.png")
	refBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode reference image: %v", err)
	}
	if err := os.WriteFile(refPath, refBytes, 0o600); err != nil {
		t.Fatalf("write reference image: %v", err)
	}
	imageBytes := []byte("bridge-png-bytes")
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/llm/v1/image" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer hosted-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = fmt.Fprintf(w, `{"data":"%s","mime":"image/png","access_mode":"paid","remaining":41,"paid_quota_remaining":41}`, base64.StdEncoding.EncodeToString(imageBytes))
	}))
	defer server.Close()

	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		t.Fatal("img generation must not initialize the local generation provider")
		return nil, nil
	}
	licenseEvents := []string{}
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return &orderedLicenseManager{
			events: &licenseEvents,
			checkResult: &LicenseCheckResult{
				Allowed:            true,
				AccessMode:         LicenseAccessModePaid,
				PaidQuotaRemaining: 42,
			},
		}, nil
	}
	bridge := newAgentBridgeServer(app, Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: true, Mode: "fast"},
		Runtime:  RuntimeConfig{Mode: RuntimeModeHosted},
		License:  LicenseConfig{BaseURL: server.URL, APIKey: "hosted-key", Enabled: true, TimeoutSec: 5},
		Publish:  disabledPublishConfig(),
	}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))

	task, err := bridge.invokeTask(context.Background(), json.RawMessage(`1`), bridgeInvokeParams{
		Tool:         "office.generate",
		OutputFormat: "json",
		Args: bridgeInvokeArgs{
			DocumentType:   "img",
			Topic:          "Launch Visual",
			Prompt:         "A polished product launch hero image",
			Ratio:          "landscape",
			ReferenceImage: refPath,
		},
	})
	if err != nil {
		t.Fatalf("invokeTask: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status, err := bridge.taskStatus(task.ID)
		if err != nil {
			t.Fatalf("taskStatus: %v", err)
		}
		if status.Status == "completed" {
			result := status.Result.(GenerateResult)
			if result.DocumentType != "img" || filepath.Ext(result.FilePath) != ".png" {
				t.Fatalf("unexpected result: %+v", result)
			}
			if result.AccessMode != "paid" || result.PaidQuotaRemaining != 41 || result.Remaining != 41 {
				t.Fatalf("unexpected quota fields: %+v", result)
			}
			data, err := os.ReadFile(result.FilePath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if string(data) != string(imageBytes) {
				t.Fatalf("image bytes = %q", string(data))
			}
			if gotPayload["model"] != "hosted/image" || gotPayload["aspect_ratio"] != 16.0/9.0 || gotPayload["commit_token"] == nil {
				t.Fatalf("payload = %#v", gotPayload)
			}
			reference, ok := gotPayload["reference_image"].(map[string]any)
			if !ok || reference["mime"] != "image/png" || strings.TrimSpace(fmt.Sprint(reference["data"])) == "" {
				t.Fatalf("reference_image payload = %#v", gotPayload["reference_image"])
			}
			if strings.Join(licenseEvents, ",") != "check" {
				t.Fatalf("license events = %#v, want check only because server consumes image quota", licenseEvents)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for img generation")
}

func TestAgentBridgeCancelTask(t *testing.T) {
	tmpDir := t.TempDir()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	outReader := bufio.NewReader(outR)
	app := NewApp(outW, bytes.NewBuffer(nil), inR)
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	wait := make(chan struct{})
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return blockingLLMClient{
			jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`,
			wait:         wait,
		}, nil
	}
	cfg := Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- newAgentBridgeServer(app, cfg, inR, outW, bytes.NewBuffer(nil)).Serve(ctx)
	}()

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "task/invoke", "params": map[string]any{
		"tool":          "office.generate",
		"interactive":   false,
		"output_format": "json",
		"args": map[string]any{
			"document_type": "docx",
			"topic":         "Enterprise Collaboration Platform Overview",
			"prompt":        "Introduce this enterprise collaboration platform",
			"mode":          "fast",
			"out":           tmpDir,
			"publish":       false,
		},
	}})
	taskID := ""
	timeout := time.After(3 * time.Second)
	for taskID == "" {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for invoke response")
		default:
		}
		msg := readRPC(t, outReader)
		if result, ok := msg["result"].(map[string]any); ok {
			if id, ok := result["task_id"].(string); ok {
				taskID = id
			}
		}
	}

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "task/cancel", "params": map[string]any{"task_id": taskID}})
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for cancel response")
		default:
		}
		msg := readRPC(t, outReader)
		if result, ok := msg["result"].(map[string]any); ok {
			if cancelled, ok := result["cancelled"].(bool); ok && cancelled {
				break
			}
		}
	}

	timeout = time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for cancel event")
		default:
		}
		msg := readRPC(t, outReader)
		if msg["method"] != "event" {
			continue
		}
		params := msg["params"].(map[string]any)
		if params["type"] == bridgeEventTaskCancelled {
			break
		}
	}

	close(wait)
	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("bridge exited with error: %v", err)
	}
}

func TestBridgePrompterRespondsThroughTaskRespond(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	task := &bridgeTask{
		ID:        "task-1",
		SessionID: "default",
		RequestID: "req-1",
		Status:    "running",
	}
	server.tasks[task.ID] = task
	prompter := &bridgePrompter{
		ctx:    context.Background(),
		server: server,
		task:   task,
		answer: make(chan bridgePromptResponse, 1),
	}
	task.Prompt = prompter

	done := make(chan bridgePromptResponse, 1)
	go func() {
		optionID, answer, err := prompter.Ask(AskOptions{Question: "Choose an output style", Options: []string{"Concise", "Detailed"}, AllowFreeform: true})
		if err != nil {
			t.Errorf("Ask returned error: %v", err)
			return
		}
		done <- bridgePromptResponse{OptionID: optionID, Answer: answer}
	}()

	timeout := time.After(2 * time.Second)
	for task.CurrentQ == nil {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for current question")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if err := server.respondTask(bridgeRespondParams{
		TaskID:     task.ID,
		QuestionID: task.CurrentQ.ID,
		OptionID:   "2",
	}); err != nil {
		t.Fatalf("respondTask: %v", err)
	}

	select {
	case response := <-done:
		if response.OptionID != "2" {
			t.Fatalf("unexpected option id: %#v", response)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prompt answer")
	}
}

func TestBridgePrompterReviewPlanWaitsForApprove(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	task := &bridgeTask{
		ID:        "task-1",
		SessionID: "default",
		RequestID: "req-1",
		Status:    "running",
	}
	server.tasks[task.ID] = task
	prompter := &bridgePrompter{
		ctx:    context.Background(),
		server: server,
		task:   task,
		answer: make(chan bridgePromptResponse, 1),
	}
	task.Prompt = prompter

	done := make(chan PlanReviewResponse, 1)
	go func() {
		response, err := prompter.ReviewPlan(&engine.PlanSession{
			PlanID:       "plan-1",
			Revision:     2,
			PlanMarkdown: "# Proposed Plan\n\n- Build the deck after approval.",
		})
		if err != nil {
			t.Errorf("ReviewPlan returned error: %v", err)
			return
		}
		done <- response
	}()

	timeout := time.After(2 * time.Second)
	for task.CurrentPlan == nil {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for current plan")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	if task.Status != "waiting_input" {
		t.Fatalf("status = %q, want waiting_input", task.Status)
	}
	if task.CurrentPlan.ID != "plan-1" || task.CurrentPlan.Revision != 2 {
		t.Fatalf("current plan = %#v", task.CurrentPlan)
	}

	if err := server.respondTask(bridgeRespondParams{
		TaskID:   task.ID,
		OptionID: "approve",
	}); err != nil {
		t.Fatalf("respondTask: %v", err)
	}

	select {
	case response := <-done:
		if response.Action != PlanReviewApprove {
			t.Fatalf("unexpected review response: %#v", response)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for plan review response")
	}
}

func TestBridgePrompterReviewPlanAcceptsRevisionInstruction(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	task := &bridgeTask{ID: "task-1", SessionID: "default", RequestID: "req-1", Status: "running"}
	server.tasks[task.ID] = task
	prompter := &bridgePrompter{
		ctx:    context.Background(),
		server: server,
		task:   task,
		answer: make(chan bridgePromptResponse, 1),
	}
	task.Prompt = prompter

	done := make(chan PlanReviewResponse, 1)
	go func() {
		response, err := prompter.ReviewPlan(&engine.PlanSession{PlanID: "plan-1", PlanMarkdown: "# Proposed Plan"})
		if err != nil {
			t.Errorf("ReviewPlan returned error: %v", err)
			return
		}
		done <- response
	}()

	timeout := time.After(2 * time.Second)
	for task.CurrentPlan == nil {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for current plan")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	if err := server.respondTask(bridgeRespondParams{TaskID: task.ID, Answer: "Make it shorter and more executive."}); err != nil {
		t.Fatalf("respondTask: %v", err)
	}

	select {
	case response := <-done:
		if response.Action != PlanReviewRevise || response.Instruction != "Make it shorter and more executive." {
			t.Fatalf("unexpected review response: %#v", response)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for plan review response")
	}
}

func TestAgentBridgeOutputPayloadKeepsStableFields(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	payload := server.outputPayload("file", GenerateResult{
		Status:       "success",
		FilePath:     "/tmp/demo.pptx",
		DocumentType: "pptx",
		DocumentName: "demo.pptx",
		Warnings:     []string{"Image generation failed, and the PPT output was downgraded to a text-only version."},
		ReferenceStyle: &runtime.ReferenceStyleMetadata{
			Enabled:         true,
			DiscoveredCount: 2,
			ParsedCount:     1,
			FailedCount:     1,
		},
		PPTXReview: &PPTXReviewMetadata{
			Status:         "good",
			OverallScore:   82,
			StructureScore: 82,
		},
		PPTXArtifactDebug: &runtime.PPTXArtifactDebugMetadata{
			Enabled:       true,
			Backend:       runtime.PPTXBackendOfficegen,
			WorkerVersion: "artifact-experimental-test",
			PreviewCount:  4,
		},
		PPTXBackend: runtime.PPTXBackendOfficegen,
	})

	for _, key := range []string{"format", "status", "file_path", "document_type", "document_name", "warnings", "result", "result_meta"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload missing key %q: %#v", key, payload)
		}
	}
	if payload["format"] != "file" {
		t.Fatalf("format = %#v", payload["format"])
	}
	meta, ok := payload["result_meta"].(map[string]any)
	if !ok {
		t.Fatalf("result_meta = %#v", payload["result_meta"])
	}
	imageSupport, ok := meta["image_support"].(map[string]any)
	if !ok {
		t.Fatalf("image_support = %#v", meta["image_support"])
	}
	if imageSupport["config_command"] != "officecli config set-generation" {
		t.Fatalf("unexpected config command: %#v", imageSupport["config_command"])
	}
	if imageSupport["attention_required"] != true {
		t.Fatalf("unexpected attention_required: %#v", imageSupport["attention_required"])
	}
	if _, ok := meta["reference_style"].(*runtime.ReferenceStyleMetadata); !ok {
		t.Fatalf("reference_style missing from result_meta: %#v", meta)
	}
	if _, ok := meta["pptx_review"].(*PPTXReviewMetadata); !ok {
		t.Fatalf("pptx_review missing from result_meta: %#v", meta)
	}
	if _, ok := meta["pptx_artifact_debug"].(*runtime.PPTXArtifactDebugMetadata); !ok {
		t.Fatalf("pptx_artifact_debug missing from result_meta: %#v", meta)
	}
	if meta["pptx_backend"] != runtime.PPTXBackendOfficegen {
		t.Fatalf("pptx_backend missing from result_meta: %#v", meta)
	}
}

func TestAgentBridgeTaskCompletedPayloadIncludesHostedCreditBalance(t *testing.T) {
	payload := generateTaskCompletedPayload(GenerateResult{
		Status:         "success",
		FilePath:       "/tmp/demo.pptx",
		DocumentType:   "pptx",
		DocumentName:   "demo.pptx",
		CreditsCharged: 14,
		CreditBalance:  1100230,
		CreditMode:     "hosted",
	})

	if payload["credits_charged"] != 14 {
		t.Fatalf("credits_charged = %#v, want 14", payload["credits_charged"])
	}
	if payload["credit_balance"] != 1100230 {
		t.Fatalf("credit_balance = %#v, want 1100230", payload["credit_balance"])
	}
	if payload["credit_mode"] != "hosted" {
		t.Fatalf("credit_mode = %#v, want hosted", payload["credit_mode"])
	}
}

func TestAgentBridgeTaskCompletedPayloadIncludesImageWatermark(t *testing.T) {
	payload := generateTaskCompletedPayload(GenerateResult{
		Status:       "success",
		FilePath:     "/tmp/demo.png",
		DocumentType: "img",
		DocumentName: "demo.png",
		ImageWatermark: &ImageWatermarkResult{
			Applied:         true,
			PaidEntitlement: false,
			CanDisable:      false,
		},
	})

	watermark, ok := payload["image_watermark"].(map[string]any)
	if !ok {
		t.Fatalf("image_watermark = %#v", payload["image_watermark"])
	}
	if watermark["applied"] != true || watermark["paidEntitlement"] != false || watermark["canDisable"] != false {
		t.Fatalf("image_watermark = %#v", watermark)
	}
}

func TestAgentBridgeTaskStatusIncludesResultMeta(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	server.tasks["task-1"] = &bridgeTask{
		ID:        "task-1",
		SessionID: "session-1",
		Tool:      bridgeToolOfficeGenerate,
		Status:    "completed",
		OutputFmt: "json",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Result: GenerateResult{
			Status:       "success",
			FilePath:     "/tmp/demo.pptx",
			DocumentType: "pptx",
			DocumentName: "demo.pptx",
			Warnings: []string{
				"Image generation failed, and the PPT output was downgraded to a text-only version. Check the image model URL, API key, and model name, or run `officecli config set-generation`; use `--no-images` for a text-only deck.",
			},
		},
	}

	status, err := server.taskStatus("task-1")
	if err != nil {
		t.Fatalf("taskStatus: %v", err)
	}
	if status.ResultMeta == nil {
		t.Fatal("expected result meta")
	}
	imageSupport, ok := status.ResultMeta["image_support"].(map[string]any)
	if !ok {
		t.Fatalf("image_support = %#v", status.ResultMeta["image_support"])
	}
	if imageSupport["reason"] != "image_generation_degraded" {
		t.Fatalf("unexpected reason: %#v", imageSupport["reason"])
	}
}

func TestClassifyBridgeError(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantType  string
		wantCode  string
		retryable bool
	}{
		{name: "config", err: io.EOF, wantType: "execution_error", wantCode: "execution_failed"},
		{name: "missing config", err: errString("generation service is not fully configured: missing base url"), wantType: "configuration_error", wantCode: "configuration_missing"},
		{name: "llm", err: errString("content generation failed: llm request failed"), wantType: "llm_error", wantCode: "llm_request_failed", retryable: true},
		{name: "assembly", err: errString("document assembly failed: parse llm response"), wantType: "assembly_error", wantCode: "document_assembly_failed"},
		{name: "validation", err: errString("unsupported tool: foo"), wantType: "validation_error", wantCode: "invalid_request"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyBridgeError(tc.err)
			if got.Type != tc.wantType || got.Code != tc.wantCode || got.Retryable != tc.retryable {
				t.Fatalf("got=%+v wantType=%s wantCode=%s retryable=%t", got, tc.wantType, tc.wantCode, tc.retryable)
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func boolPtr(v bool) *bool { return &v }

func writeRPC(t *testing.T, w io.Writer, payload map[string]any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if _, err := io.WriteString(w, "Content-Length: "+strconv.Itoa(len(raw))+"\r\n\r\n"); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := w.Write(raw); err != nil {
		t.Fatalf("write body: %v", err)
	}
}

func readRPC(t *testing.T, reader *bufio.Reader) map[string]any {
	t.Helper()
	contentLength := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read header: %v", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			value := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "content-length:"))
			length, err := strconv.Atoi(value)
			if err != nil {
				t.Fatalf("invalid content length: %v", err)
			}
			contentLength = length
		}
	}
	if contentLength == 0 {
		t.Fatal("missing content length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("unmarshal body: %v body=%s", err, string(body))
	}
	return msg
}

func TestReadRPCConsumesBackToBackMessagesFromSharedReader(t *testing.T) {
	var stream bytes.Buffer
	writeRPC(t, &stream, map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"task_id": "task-1"}})
	writeRPC(t, &stream, map[string]any{"jsonrpc": "2.0", "method": "event", "params": map[string]any{"type": bridgeEventTaskCompleted}})

	reader := bufio.NewReader(&stream)
	first := readRPC(t, reader)
	second := readRPC(t, reader)

	if first["id"].(float64) != 1 {
		t.Fatalf("unexpected first message: %#v", first)
	}
	if second["method"] != "event" {
		t.Fatalf("unexpected second message: %#v", second)
	}
}

func TestAgentBridgeReviewTask(t *testing.T) {
	tmpDir := t.TempDir()
	deckPath := filepath.Join(tmpDir, "deck.pptx")
	if err := os.WriteFile(deckPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	outReader := bufio.NewReader(outR)
	app := NewApp(outW, bytes.NewBuffer(nil), inR)
	app.newReviewer = func(cfg Config, progress engine.ProgressEmitter) (Reviewer, error) {
		return &stubReviewer{result: &ReviewResult{
			Status:         "good",
			DocumentType:   "pptx",
			FilePath:       deckPath,
			OverallScore:   78,
			VisualScore:    80,
			StructureScore: 72,
			Summary:        "Overall quality is acceptable, but there is still room to improve.",
			UsedVisual:     true,
		}}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- newAgentBridgeServer(app, Config{}, inR, outW, bytes.NewBuffer(nil)).Serve(ctx)
	}()

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "task/invoke", "params": map[string]any{
		"tool":          "office.review",
		"interactive":   false,
		"output_format": "json",
		"args": map[string]any{
			"document_type": "pptx",
			"file_path":     deckPath,
			"enable_visual": true,
			"fail_below":    70,
		},
	}})

	var taskID string
	var sawCompleted bool
	timeout := time.After(3 * time.Second)
	for !sawCompleted {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for review events")
		default:
		}
		msg := readRPC(t, outReader)
		if result, ok := msg["result"].(map[string]any); ok {
			if id, ok := result["task_id"].(string); ok {
				taskID = id
			}
			continue
		}
		if msg["method"] != "event" {
			continue
		}
		params := msg["params"].(map[string]any)
		if params["type"] == bridgeEventTaskCompleted {
			sawCompleted = true
		}
	}
	if taskID == "" {
		t.Fatal("expected task id")
	}

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "task/status", "params": map[string]any{"task_id": taskID}})
	statusMsg := readRPC(t, outReader)
	status := statusMsg["result"].(map[string]any)
	if status["tool"] != "office.review" || status["status"] != "completed" {
		t.Fatalf("unexpected task status: %#v", status)
	}
	result := status["result"].(map[string]any)
	if result["overall_score"].(float64) != 78 {
		t.Fatalf("unexpected review result: %#v", result)
	}

	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("bridge exited with error: %v", err)
	}
}

func TestInferCreditModeReturnsExpectedLabels(t *testing.T) {
	cases := []struct {
		name string
		job  GenerateJob
		want string
	}{
		{
			name: "hosted runtime with authenticated user",
			job:  GenerateJob{RuntimeMode: RuntimeModeHosted, LicenseCheck: &LicenseCheckResult{AccessMode: LicenseAccessModeHosted, CommitToken: UsageCommitToken{UserID: 42, FingerprintHash: "fp"}}},
			want: "hosted",
		},
		{
			name: "hosted runtime with anonymous fingerprint only",
			job:  GenerateJob{RuntimeMode: RuntimeModeHosted, LicenseCheck: &LicenseCheckResult{AccessMode: LicenseAccessModeHosted, CommitToken: UsageCommitToken{FingerprintHash: "fp-anon"}}},
			want: "anonymous",
		},
		{
			name: "external runtime mapped to api_key",
			job:  GenerateJob{RuntimeMode: RuntimeModeExternal, LicenseCheck: &LicenseCheckResult{AccessMode: LicenseAccessModePaid, CommitToken: UsageCommitToken{UserID: 7, FingerprintHash: "fp"}}},
			want: "api_key",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferCreditMode(tc.job); got != tc.want {
				t.Fatalf("inferCreditMode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFailureEventPayloadIncludesCreditFields(t *testing.T) {
	payload := failureEventPayload(bridgeErrorPayload{Type: "llm_error", Code: "llm_request_failed", Message: "boom", Retryable: true}, "hosted")
	if payload["type"] != "llm_error" || payload["code"] != "llm_request_failed" || payload["message"] != "boom" {
		t.Fatalf("missing classified fields: %#v", payload)
	}
	if payload["retryable"] != true {
		t.Fatalf("retryable not preserved: %#v", payload)
	}
	if payload["credits_charged"] != 0 {
		t.Fatalf("credits_charged = %#v, want 0", payload["credits_charged"])
	}
	if payload["credit_mode"] != "hosted" {
		t.Fatalf("credit_mode = %#v, want hosted", payload["credit_mode"])
	}
}

func TestAgentBridgeRenderEmitsCreditFieldsInTaskCompleted(t *testing.T) {
	tmpDir := t.TempDir()
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		t.Fatal("render path should not initialize llm")
		return nil, nil
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- newAgentBridgeServer(app, Config{
			Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
			License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
			Publish:  disabledPublishConfig(),
		}, inR, outW, bytes.NewBuffer(nil)).Serve(ctx)
	}()
	outReader := bufio.NewReader(outR)

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "task/invoke", "params": map[string]any{
		"tool":          bridgeToolOfficeRender,
		"output_format": "json",
		"args": map[string]any{
			"document_type": "docx",
			"topic":         "Quarterly Brief",
			"payload":       json.RawMessage(`{"title":"Quarterly Brief","sections":[{"heading":"Summary","level":1,"paragraphs":["Delivery-ready content."]}]}`),
			"output_dir":    tmpDir,
			"publish":       false,
		},
	}})

	var sawCompleted bool
	var completedParams map[string]any
	timeout := time.After(3 * time.Second)
	for !sawCompleted {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for completion event")
		default:
		}
		msg := readRPC(t, outReader)
		if msg["method"] != "event" {
			continue
		}
		params := msg["params"].(map[string]any)
		if params["type"] == bridgeEventTaskCompleted {
			completedParams = params["payload"].(map[string]any)
			sawCompleted = true
		}
	}

	if _, ok := completedParams["credits_charged"]; !ok {
		t.Fatalf("task.completed missing credits_charged: %#v", completedParams)
	}
	if mode, ok := completedParams["credit_mode"].(string); !ok || mode == "" {
		t.Fatalf("task.completed missing credit_mode string: %#v", completedParams)
	}

	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("bridge exited with error: %v", err)
	}
}

type heartbeatSlowLLM struct {
	delay  time.Duration
	result *engine.ImageGenerationResult
}

func (heartbeatSlowLLM) CompleteText(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (heartbeatSlowLLM) CompleteJSON(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (heartbeatSlowLLM) CompleteStructured(context.Context, engine.StructuredCompletionRequest) (string, error) {
	return "", nil
}

func (s heartbeatSlowLLM) GenerateImage(ctx context.Context, _ engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	select {
	case <-time.After(s.delay):
		return s.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// TestAgentBridgeForwardsImageHeartbeatProgressUnder60s verifies the bug-report
// acceptance criterion: when a hosted image provider takes a long time to
// respond, the bridge stdout MUST emit a task.progress notification at least
// every 60 seconds so OfficeDex's stall detector does not falsely flag the
// task as stuck.
func TestAgentBridgeForwardsImageHeartbeatProgressUnder60s(t *testing.T) {
	restore := runtime.SetImageHeartbeatIntervalForTesting(40 * time.Millisecond)
	defer restore()

	out := bytes.NewBuffer(nil)
	app := NewApp(out, bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	server := newAgentBridgeServer(app, Config{
		Defaults: DefaultsConfig{Publish: false, Mode: "fast"},
		Publish:  disabledPublishConfig(),
	}, bytes.NewBuffer(nil), out, bytes.NewBuffer(nil))

	task := &bridgeTask{
		ID:        "task-test",
		SessionID: "default",
		RequestID: "1",
		Tool:      bridgeToolOfficeRender,
		Status:    "running",
		OutputFmt: "json",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	emitter := &bridgeProgressEmitter{server: server, task: task}

	llm := heartbeatSlowLLM{
		delay:  260 * time.Millisecond,
		result: &engine.ImageGenerationResult{Data: mustTinyPNGForCLI(t), MIME: "image/png"},
	}
	content := `{
		"title":"Heartbeat Demo",
		"slides":[
			{"title":"Heartbeat Demo","layout":"title","variant":"title-center","subtitle":"Cover","hasImage":true,"imagePrompt":"A neutral hero image","imagePos":"background"}
		]
	}`

	_, _, _, _, _, err := runtime.BuildPPTXFromJSON(context.Background(), llm, emitter, content, "Heartbeat Demo", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}

	progressEvents := parseBridgeProgressTimes(t, out.Bytes())
	if len(progressEvents) < 3 {
		t.Fatalf("expected at least 3 task.progress events, got %d: %+v", len(progressEvents), progressEvents)
	}

	maxGap := time.Duration(0)
	for i := 1; i < len(progressEvents); i++ {
		if gap := progressEvents[i].ts.Sub(progressEvents[i-1].ts); gap > maxGap {
			maxGap = gap
		}
	}
	if maxGap > 60*time.Second {
		t.Fatalf("max gap between task.progress events exceeds 60s budget: %s", maxGap)
	}

	heartbeatSeen := false
	for _, event := range progressEvents {
		if strings.Contains(event.content, "Still waiting on image provider") {
			heartbeatSeen = true
			break
		}
	}
	if !heartbeatSeen {
		t.Fatalf("no 'Still waiting on image provider' heartbeat event observed in bridge stdout: %+v", progressEvents)
	}
}

type bridgeProgressRecord struct {
	ts      time.Time
	step    string
	content string
}

func parseBridgeProgressTimes(t *testing.T, raw []byte) []bridgeProgressRecord {
	t.Helper()
	reader := bufio.NewReader(bytes.NewReader(raw))
	out := make([]bridgeProgressRecord, 0, 8)
	for {
		var contentLength int
		headerDone := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return out
			}
			line = strings.TrimSpace(line)
			if line == "" {
				headerDone = true
				break
			}
			if strings.HasPrefix(strings.ToLower(line), "content-length:") {
				value := strings.TrimSpace(line[len("content-length:"):])
				n, perr := strconv.Atoi(value)
				if perr != nil {
					t.Fatalf("bad Content-Length: %v", perr)
				}
				contentLength = n
			}
		}
		if !headerDone || contentLength <= 0 {
			return out
		}
		body := make([]byte, contentLength)
		if _, err := io.ReadFull(reader, body); err != nil {
			return out
		}
		var msg map[string]any
		if err := json.Unmarshal(body, &msg); err != nil {
			t.Fatalf("invalid bridge stdout message: %v", err)
		}
		if msg["method"] != "event" {
			continue
		}
		params, ok := msg["params"].(map[string]any)
		if !ok {
			continue
		}
		if params["type"] != bridgeEventTaskProgress {
			continue
		}
		ts, _ := time.Parse(time.RFC3339Nano, fmt.Sprint(params["ts"]))
		payload, _ := params["payload"].(map[string]any)
		step, _ := payload["step"].(string)
		content, _ := payload["content"].(string)
		out = append(out, bridgeProgressRecord{ts: ts, step: step, content: content})
	}
}

func TestGenerateTaskIDIsUniqueAndUUIDv7(t *testing.T) {
	const n = 100
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := generateTaskID()
		if !bridgeTaskIDUUIDPattern.MatchString(id) {
			t.Fatalf("id %q does not match UUID regex", id)
		}
		if id[14] != '7' {
			t.Fatalf("id %q is not UUIDv7 (version nibble = %c)", id, id[14])
		}
		if v := id[19]; v != '8' && v != '9' && v != 'a' && v != 'b' {
			t.Fatalf("id %q has wrong UUID variant nibble %c", id, v)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id generated at iteration %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestAgentBridgeConcurrentInvokeProducesUniqueUUIDs(t *testing.T) {
	tmpDir := t.TempDir()
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	server := newAgentBridgeServer(app, Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))

	const n = 100
	var wg sync.WaitGroup
	ids := make([]string, n)
	errs := make([]error, n)
	payload := json.RawMessage(`{"title":"T","sections":[{"heading":"H","level":1,"paragraphs":["x"]}]}`)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task, err := server.invokeTask(context.Background(), json.RawMessage(strconv.Itoa(idx)), bridgeInvokeParams{
				Tool:         "office.render",
				OutputFormat: "json",
				Args: bridgeInvokeArgs{
					DocumentType: "docx",
					Topic:        "Concurrent",
					Payload:      payload,
					OutputDir:    tmpDir,
					Publish:      boolPtr(false),
				},
			})
			if err != nil {
				errs[idx] = err
				return
			}
			ids[idx] = task.ID
		}(i)
	}
	wg.Wait()

	seen := make(map[string]struct{}, n)
	for i, id := range ids {
		if errs[i] != nil {
			t.Fatalf("invoke %d failed: %v", i, errs[i])
		}
		if !bridgeTaskIDUUIDPattern.MatchString(id) {
			t.Fatalf("task id %q does not match UUID regex", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate task id across concurrent invokes: %s", id)
		}
		seen[id] = struct{}{}
	}

	// Drain async render goroutines so t.TempDir cleanup doesn't race with
	// in-flight file writes.
	deadline := time.Now().Add(10 * time.Second)
	for _, id := range ids {
		for time.Now().Before(deadline) {
			status, err := server.taskStatus(id)
			if err != nil {
				t.Fatalf("taskStatus(%s): %v", id, err)
			}
			if status.Status == "completed" || status.Status == "failed" || status.Status == "cancelled" {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestIsValidTaskIDAcceptsUUIDAndLegacy(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"new uuidv7", generateTaskID(), true},
		{"legacy task 1", "task-1", true},
		{"legacy task padded", "task-000001", true},
		{"legacy task large", "task-9999999999", true},
		{"empty", "", false},
		{"bare word", "hello", false},
		{"uppercase uuid", strings.ToUpper(generateTaskID()), false},
		{"task without digits", "task-", false},
		{"task with letters", "task-abc", false},
		{"oversize", strings.Repeat("a", 256), false},
		{"uuid with prefix", "task-" + generateTaskID(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsValidTaskID(tc.input); got != tc.want {
				t.Fatalf("IsValidTaskID(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestAgentBridgeCancelAcceptsLegacyAndRejectsGarbage(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	outReader := bufio.NewReader(outR)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- newAgentBridgeServer(app, Config{}, inR, outW, bytes.NewBuffer(nil)).Serve(ctx)
	}()

	// Legacy ID on an empty server: must route through, returning task-not-found
	// (NOT invalid_task_id) so we stay backward-compatible with on-disk records.
	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "task/cancel", "params": map[string]any{"task_id": "task-000001"}})
	msg := readRPC(t, outReader)
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error for unknown legacy task, got: %#v", msg)
	}
	if message, _ := errObj["message"].(string); !strings.Contains(message, "task not found") {
		t.Fatalf("expected task not found, got: %#v", errObj)
	}

	for i, bad := range []string{"", "hello", strings.Repeat("a", 256)} {
		writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 2 + i, "method": "task/cancel", "params": map[string]any{"task_id": bad}})
		msg := readRPC(t, outReader)
		errObj, ok := msg["error"].(map[string]any)
		if !ok {
			t.Fatalf("expected error for garbage task_id %q, got: %#v", bad, msg)
		}
		if message, _ := errObj["message"].(string); !strings.Contains(message, "invalid_task_id") {
			t.Fatalf("expected invalid_task_id for %q, got: %#v", bad, errObj)
		}
	}

	for i, bad := range []string{"", "hello", strings.Repeat("a", 256)} {
		writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 100 + i, "method": "task/respond", "params": map[string]any{"task_id": bad, "answer": "x"}})
		msg := readRPC(t, outReader)
		errObj, ok := msg["error"].(map[string]any)
		if !ok {
			t.Fatalf("expected error for garbage task/respond task_id %q, got: %#v", bad, msg)
		}
		if message, _ := errObj["message"].(string); !strings.Contains(message, "invalid_task_id") {
			t.Fatalf("expected invalid_task_id for respond %q, got: %#v", bad, errObj)
		}
	}

	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("bridge exited with error: %v", err)
	}
}

func TestBuildGenerateJobFromRequestCarriesPromptTemplateID(t *testing.T) {
	app := NewApp(io.Discard, io.Discard, strings.NewReader(""))
	req := bridgeInvokeParams{
		Tool: bridgeToolOfficeGenerate,
		Args: bridgeInvokeArgs{
			DocumentType:     "img",
			Topic:            "poster",
			Prompt:           "with a red bicycle",
			PromptTemplateID: "template-liblib-style",
		},
	}
	job, err := app.buildGenerateJobFromRequest(Config{}, req)
	if err != nil {
		t.Fatalf("buildGenerateJobFromRequest: %v", err)
	}
	if job.PromptTemplateID != "template-liblib-style" {
		t.Fatalf("PromptTemplateID = %q", job.PromptTemplateID)
	}
}

func TestListImagePromptTemplatesFetchesPlatformData(t *testing.T) {
	app := NewApp(io.Discard, io.Discard, strings.NewReader(""))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/image-templates" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":7,"slug":"poster","title":"Poster","description":"Poster style","prompt_preset":"cinematic preset","thumbnail_url":"/api/image-templates/7/thumbnail","sort_order":10,"enabled":true}]}`)
	}))
	defer server.Close()

	items, err := app.listImagePromptTemplates(context.Background(), Config{License: LicenseConfig{BaseURL: server.URL}})
	if err != nil {
		t.Fatalf("listImagePromptTemplates: %v", err)
	}
	if len(items) != 1 || items[0].ID != 7 || items[0].Slug != "poster" || items[0].ThumbnailURL != server.URL+"/api/image-templates/7/thumbnail" {
		t.Fatalf("unexpected templates: %#v", items)
	}
	if items[0].PromptPreset != "cinematic preset" {
		t.Fatalf("PromptPreset = %q", items[0].PromptPreset)
	}
}

func TestListImagePromptTemplatesUsesCLISessionForPrivateTemplates(t *testing.T) {
	app := NewApp(io.Discard, io.Discard, strings.NewReader(""))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/cli/image-templates" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ocli_sess_private" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"public":[{"id":7,"visibility":"platform_public","slug":"poster","title":"Poster","description":"Poster style","prompt_preset":"cinematic preset","thumbnail_url":"/api/image-templates/7/thumbnail","sort_order":10,"enabled":true}],"private":[{"id":9,"owner_user_id":42,"visibility":"user_private","slug":"my-poster","title":"My Poster","description":"Mine","prompt_preset":"private preset","sort_order":0,"enabled":true}]}}`)
	}))
	defer server.Close()

	items, err := app.listImagePromptTemplates(context.Background(), Config{License: LicenseConfig{BaseURL: server.URL, SessionToken: "ocli_sess_private"}})
	if err != nil {
		t.Fatalf("listImagePromptTemplates: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("templates = %#v", items)
	}
	if items[0].Visibility != "platform_public" || items[1].Visibility != "user_private" || items[1].OwnerUserID != 42 {
		t.Fatalf("unexpected template scopes: %#v", items)
	}
	if items[0].ThumbnailURL != server.URL+"/api/image-templates/7/thumbnail" {
		t.Fatalf("thumbnail url = %q", items[0].ThumbnailURL)
	}
}

func TestCreateUserImagePromptTemplateUsesCLISession(t *testing.T) {
	app := NewApp(io.Discard, io.Discard, strings.NewReader(""))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/cli/image-templates" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ocli_sess_private" {
			t.Fatalf("authorization = %q", got)
		}
		var req CreateUserImagePromptTemplateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.SourceTemplateID != 7 || req.Slug != "my-poster" || req.Title != "My Poster" {
			t.Fatalf("unexpected create request: %#v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":9,"owner_user_id":42,"visibility":"user_private","slug":"my-poster","title":"My Poster","description":"Mine","prompt_preset":"private preset","sort_order":0,"enabled":true}}`)
	}))
	defer server.Close()

	item, err := app.createUserImagePromptTemplate(context.Background(), Config{License: LicenseConfig{BaseURL: server.URL, SessionToken: "ocli_sess_private"}}, CreateUserImagePromptTemplateRequest{
		SourceTemplateID: 7,
		Slug:             "my-poster",
		Title:            "My Poster",
	})
	if err != nil {
		t.Fatalf("createUserImagePromptTemplate: %v", err)
	}
	if item.ID != 9 || item.Visibility != "user_private" || item.OwnerUserID != 42 {
		t.Fatalf("unexpected template: %#v", item)
	}
}

func TestCreateImageTemplatePublishRequestUsesCLISessionAndRequestID(t *testing.T) {
	app := NewApp(io.Discard, io.Discard, strings.NewReader(""))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/cli/image-template-publish-requests" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ocli_sess_private" {
			t.Fatalf("authorization = %q", got)
		}
		var req CreateImageTemplatePublishRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.PrivateTemplateID != 9 || req.RequestID != "req-img-1" || req.SubmitterNote != "please review" {
			t.Fatalf("unexpected publish request: %#v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":31,"private_template_id":9,"requester_user_id":42,"provenance_id":11,"status":"pending","submitter_note":"please review"}}`)
	}))
	defer server.Close()

	item, err := app.createImageTemplatePublishRequest(context.Background(), Config{License: LicenseConfig{BaseURL: server.URL, SessionToken: "ocli_sess_private"}}, CreateImageTemplatePublishRequest{
		PrivateTemplateID: 9,
		RequestID:         "req-img-1",
		SubmitterNote:     "please review",
	})
	if err != nil {
		t.Fatalf("createImageTemplatePublishRequest: %v", err)
	}
	if item.ID != 31 || item.Status != "pending" || item.ProvenanceID != 11 {
		t.Fatalf("unexpected publish request response: %#v", item)
	}
}

func TestApplyImagePromptTemplateComposesPrompt(t *testing.T) {
	app := NewApp(io.Discard, io.Discard, strings.NewReader(""))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/image-templates/7/compose" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Prompt != "red bicycle" {
			t.Fatalf("prompt = %q", req.Prompt)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"prompt":"preset prompt\n\nUser prompt:\nred bicycle"}}`)
	}))
	defer server.Close()

	job, err := app.applyImagePromptTemplate(context.Background(), Config{License: LicenseConfig{BaseURL: server.URL}}, GenerateJob{
		DocumentType:     engine.DocumentTypeIMG,
		Prompt:           "red bicycle",
		OriginalPrompt:   "red bicycle",
		PromptTemplateID: "7",
	})
	if err != nil {
		t.Fatalf("applyImagePromptTemplate: %v", err)
	}
	if job.Prompt != "preset prompt\n\nUser prompt:\nred bicycle" {
		t.Fatalf("Prompt = %q", job.Prompt)
	}
	if job.OriginalPrompt != "red bicycle" {
		t.Fatalf("OriginalPrompt = %q", job.OriginalPrompt)
	}
}

func TestListImagePromptTemplatesDecodesSlots(t *testing.T) {
	app := NewApp(io.Discard, io.Discard, strings.NewReader(""))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":7,"slug":"poster","title":"Poster","description":"Poster style","prompt_preset":"A {{product}} poster","thumbnail_url":"/api/image-templates/7/thumbnail","sort_order":10,"enabled":true,"slots":[{"key":"product","label":"Product","example":"running shoes","default_value":"a product","required":true,"multiline":true}]}]}`)
	}))
	defer server.Close()

	items, err := app.listImagePromptTemplates(context.Background(), Config{License: LicenseConfig{BaseURL: server.URL}})
	if err != nil {
		t.Fatalf("listImagePromptTemplates: %v", err)
	}
	if len(items) != 1 || len(items[0].Slots) != 1 {
		t.Fatalf("unexpected templates: %#v", items)
	}
	slot := items[0].Slots[0]
	if slot.Key != "product" || slot.Label != "Product" || slot.DefaultValue != "a product" || !slot.Required || !slot.Multiline {
		t.Fatalf("unexpected slot: %#v", slot)
	}
}

func TestListImagePromptTemplatesOldShapeNoSlots(t *testing.T) {
	app := NewApp(io.Discard, io.Discard, strings.NewReader(""))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Old server payload with no "slots" key must still decode.
		_, _ = io.WriteString(w, `{"data":[{"id":7,"slug":"poster","title":"Poster","description":"Poster style","prompt_preset":"cinematic preset","sort_order":10,"enabled":true}]}`)
	}))
	defer server.Close()

	items, err := app.listImagePromptTemplates(context.Background(), Config{License: LicenseConfig{BaseURL: server.URL}})
	if err != nil {
		t.Fatalf("listImagePromptTemplates: %v", err)
	}
	if len(items) != 1 || len(items[0].Slots) != 0 {
		t.Fatalf("expected one template with no slots: %#v", items)
	}
}
