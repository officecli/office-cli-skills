package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/officecli/officecli/engine"
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
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)
	_, err := WriteConfig("", Config{
		Defaults: DefaultsConfig{OutputDir: tmpDir, Publish: false, Mode: "fast"},
		LLM:      LLMConfig{BaseURL: "https://api.example.com/v1", APIKey: "llm-key", Model: "gpt-4.1"},
		License:  LicenseConfig{BaseURL: "https://license.example.com/api", Enabled: true, TimeoutSec: 60},
		Publish:  disabledPublishConfig(),
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	app := NewApp(outW, bytes.NewBuffer(nil), inR)
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fakeAppLLMClient{jsonResponse: `{"title":"企业协作平台介绍","sections":[{"heading":"产品概述","level":1,"paragraphs":["这是一款面向企业的协作平台产品。"]}]}`}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- app.Run(ctx, []string{"agent-bridge"})
	}()

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"})
	initMsg := readRPC(t, outR)
	if initMsg["result"] == nil {
		t.Fatalf("initialize result missing: %#v", initMsg)
	}

	writeRPC(t, inW, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/open"})
	sessionMsg := readRPC(t, outR)
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
				"topic":         "企业协作平台介绍",
				"prompt":        "介绍这款企业协作平台",
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
		msg := readRPC(t, outR)
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
	statusMsg := readRPC(t, outR)
	status := statusMsg["result"].(map[string]any)
	if status["status"] != "completed" {
		t.Fatalf("unexpected task status: %#v", status)
	}

	_ = inW.Close()
	if err := <-done; err != nil {
		t.Fatalf("bridge exited with error: %v", err)
	}
}

func TestAgentBridgeCancelTask(t *testing.T) {
	tmpDir := t.TempDir()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	app := NewApp(outW, bytes.NewBuffer(nil), inR)
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		return stubLicenseManager{checkResult: &LicenseCheckResult{Allowed: true, AccessMode: LicenseAccessModePaid}}, nil
	}
	wait := make(chan struct{})
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return blockingLLMClient{
			jsonResponse: `{"title":"企业协作平台介绍","sections":[{"heading":"产品概述","level":1,"paragraphs":["这是一款面向企业的协作平台产品。"]}]}`,
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
			"topic":         "企业协作平台介绍",
			"prompt":        "介绍这款企业协作平台",
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
		msg := readRPC(t, outR)
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
		msg := readRPC(t, outR)
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
		msg := readRPC(t, outR)
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
		optionID, answer, err := prompter.Ask("请选择输出风格", []string{"简洁", "详细"}, true)
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

func TestAgentBridgeOutputPayloadKeepsStableFields(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	payload := server.outputPayload("file", GenerateResult{
		Status:       "success",
		FilePath:     "/tmp/demo.pptx",
		DocumentType: "pptx",
		DocumentName: "demo.pptx",
		Warnings:     []string{"image degraded"},
	})

	for _, key := range []string{"format", "status", "file_path", "document_type", "document_name", "warnings", "result"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload missing key %q: %#v", key, payload)
		}
	}
	if payload["format"] != "file" {
		t.Fatalf("format = %#v", payload["format"])
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
		{name: "missing config", err: errString("生成服务未完成配置：缺少生成服务地址"), wantType: "configuration_error", wantCode: "configuration_missing"},
		{name: "llm", err: errString("生成内容阶段失败：llm request failed"), wantType: "llm_error", wantCode: "llm_request_failed", retryable: true},
		{name: "assembly", err: errString("文档组装阶段失败：parse llm response"), wantType: "assembly_error", wantCode: "document_assembly_failed"},
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

func readRPC(t *testing.T, r io.Reader) map[string]any {
	t.Helper()
	reader := bufio.NewReader(r)
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

func TestAgentBridgeReviewTask(t *testing.T) {
	tmpDir := t.TempDir()
	deckPath := filepath.Join(tmpDir, "deck.pptx")
	if err := os.WriteFile(deckPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	app := NewApp(outW, bytes.NewBuffer(nil), inR)
	app.newReviewer = func(cfg Config, progress engine.ProgressEmitter) (Reviewer, error) {
		return &stubReviewer{result: &ReviewResult{
			Status:         "good",
			DocumentType:   "pptx",
			FilePath:       deckPath,
			OverallScore:   78,
			VisualScore:    80,
			StructureScore: 72,
			Summary:        "整体可用，但还有优化空间。",
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
		msg := readRPC(t, outR)
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
	statusMsg := readRPC(t, outR)
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
