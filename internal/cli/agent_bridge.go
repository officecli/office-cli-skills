package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/officecli/officecli/engine"
	"github.com/officecli/officecli/internal/runtime"
)

const bridgeTaskIDMaxLen = 128

var (
	bridgeTaskIDUUIDPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	bridgeTaskIDLegacyPattern = regexp.MustCompile(`^task-[0-9]+$`)
)

// generateTaskID returns a lowercase 36-character UUIDv7 string. Time-ordered
// so on-disk artefacts and log lines sort naturally; never resets across
// process restarts, so collisions with prior bridge runs cannot occur.
func generateTaskID() string {
	var b [16]byte
	ts := time.Now().UnixMilli()
	b[0] = byte(ts >> 40)
	b[1] = byte(ts >> 32)
	b[2] = byte(ts >> 24)
	b[3] = byte(ts >> 16)
	b[4] = byte(ts >> 8)
	b[5] = byte(ts)
	if _, err := rand.Read(b[6:]); err != nil {
		panic(fmt.Errorf("bridge: crypto/rand failed: %w", err))
	}
	b[6] = (b[6] & 0x0f) | 0x70 // version 7
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// IsValidTaskID accepts both the new UUIDv7 format and the legacy
// task-<digits> counter format used by bridge versions before the UUID
// switchover. Length is capped to keep accidental garbage from sneaking
// through. The check is strict-lowercase to match what we emit.
func IsValidTaskID(id string) bool {
	if id == "" || len(id) > bridgeTaskIDMaxLen {
		return false
	}
	return bridgeTaskIDUUIDPattern.MatchString(id) || bridgeTaskIDLegacyPattern.MatchString(id)
}

const (
	bridgeToolOfficeGenerate = "office.generate"
	bridgeToolOfficeModify   = "office.modify"
	bridgeToolOfficePrepare  = "office.prepare"
	bridgeToolOfficeRender   = "office.render"
	bridgeToolOfficeReview   = "office.review"
	bridgeToolOfficeScore    = "office.score"

	bridgeEventTaskStarted   = "task.started"
	bridgeEventTaskProgress  = "task.progress"
	bridgeEventTaskQuestion  = "task.question"
	bridgeEventTaskOutput    = "task.output"
	bridgeEventTaskCompleted = "task.completed"
	bridgeEventTaskFailed    = "task.failed"
	bridgeEventTaskCancelled = "task.cancelled"
)

type progressController interface {
	Emit(ctx context.Context, event engine.ProgressEvent)
	Pause(message string)
}

type agentBridgeServer struct {
	app    *App
	cfg    Config
	reader *bufio.Reader
	writer io.Writer
	stderr io.Writer

	writeMu  sync.Mutex
	mu       sync.Mutex
	seq      atomic.Uint64
	sessions map[string]*bridgeSession
	tasks    map[string]*bridgeTask
}

type bridgeSession struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type bridgeTask struct {
	ID          string
	SessionID   string
	RequestID   string
	Tool        string
	Status      string
	OutputFmt   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Cancel      context.CancelFunc
	CurrentQ    *bridgeQuestionState
	LastError   string
	Result      any
	Interactive bool
	Prompt      *bridgePrompter
}

type bridgeErrorPayload struct {
	Type      string `json:"type"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

type bridgeQuestionState struct {
	ID            string                 `json:"id"`
	Question      string                 `json:"question"`
	Options       []bridgeQuestionOption `json:"options,omitempty"`
	AllowFreeform bool                   `json:"allow_freeform"`
}

type bridgeQuestionOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type bridgePrompter struct {
	ctx    context.Context
	server *agentBridgeServer
	task   *bridgeTask
	answer chan bridgePromptResponse
}

type bridgePromptResponse struct {
	OptionID string
	Answer   string
}

type bridgeInvokeParams struct {
	SessionID    string            `json:"session_id,omitempty"`
	Tool         string            `json:"tool"`
	Args         bridgeInvokeArgs  `json:"args"`
	Interactive  bool              `json:"interactive"`
	OutputFormat string            `json:"output_format,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type bridgeInvokeArgs struct {
	DocumentType         string          `json:"document_type"`
	Format               string          `json:"format,omitempty"`
	Topic                string          `json:"topic"`
	Prompt               string          `json:"prompt,omitempty"`
	SourceFile           string          `json:"source_file,omitempty"`
	FilePath             string          `json:"file_path,omitempty"`
	Payload              json.RawMessage `json:"payload,omitempty"`
	Mode                 string          `json:"mode,omitempty"`
	RuntimeMode          string          `json:"runtime_mode,omitempty"`
	Language             string          `json:"lang,omitempty"`
	Style                string          `json:"style,omitempty"`
	Audience             string          `json:"audience,omitempty"`
	OutputDir            string          `json:"out,omitempty"`
	Ratio                string          `json:"ratio,omitempty"`
	Size                 string          `json:"size,omitempty"`
	PromptTemplateID     string          `json:"prompt_template_id,omitempty"`
	ReferenceImage       string          `json:"reference_image,omitempty"`
	ReferenceImages      []string        `json:"reference_images,omitempty"`
	ReferenceRoot        string          `json:"reference_root,omitempty"`
	ReferencePPTX        string          `json:"reference_pptx,omitempty"`
	ReferencePPTXSources []string        `json:"reference_pptxs,omitempty"`
	PPTXBackend          string          `json:"pptx_backend,omitempty"`
	ImageQuality         string          `json:"image_quality,omitempty"`
	Publish              *bool           `json:"publish,omitempty"`
	EnableImages         *bool           `json:"enable_images,omitempty"`
	EnableReferenceScan  *bool           `json:"enable_reference_scan,omitempty"`
	EnableVisual         *bool           `json:"enable_visual,omitempty"`
	FailBelow            *int            `json:"fail_below,omitempty"`
	EmitPreview          *bool           `json:"emit_preview,omitempty"`
	Debug                bool            `json:"debug,omitempty"`
}

type bridgePrepareResult struct {
	Status          string          `json:"status"`
	DocumentType    string          `json:"document_type"`
	PreferredTool   string          `json:"preferred_tool"`
	PrepareRequired bool            `json:"prepare_required"`
	PayloadSchema   map[string]any  `json:"payload_schema"`
	FieldNotes      []string        `json:"field_notes,omitempty"`
	WorkbookSummary string          `json:"workbook_summary,omitempty"`
	BaseReportJSON  json.RawMessage `json:"base_report_json,omitempty"`
}

type bridgeModifyResult struct {
	Status       string         `json:"status"`
	DocumentType string         `json:"document_type"`
	OutputFile   string         `json:"output_file"`
	Warnings     []string       `json:"warnings,omitempty"`
	ResultMeta   map[string]any `json:"result_meta"`
}

type bridgeRespondParams struct {
	TaskID     string `json:"task_id"`
	QuestionID string `json:"question_id,omitempty"`
	OptionID   string `json:"option_id,omitempty"`
	Answer     string `json:"answer,omitempty"`
}

type bridgeCancelParams struct {
	TaskID string `json:"task_id"`
}

type bridgeTaskStatusParams struct {
	TaskID string `json:"task_id"`
}

type bridgeSessionParams struct {
	SessionID string `json:"session_id,omitempty"`
}

type bridgeInitializeResult struct {
	ServerName      string           `json:"server_name"`
	ServerVersion   string           `json:"server_version"`
	ProtocolVersion string           `json:"protocol_version"`
	Capabilities    map[string]any   `json:"capabilities"`
	Tools           []map[string]any `json:"tools"`
}

type bridgeTaskStatusResult struct {
	TaskID          string               `json:"task_id"`
	SessionID       string               `json:"session_id"`
	Status          string               `json:"status"`
	Tool            string               `json:"tool"`
	OutputFormat    string               `json:"output_format"`
	Interactive     bool                 `json:"interactive"`
	CreatedAt       string               `json:"created_at"`
	UpdatedAt       string               `json:"updated_at"`
	CurrentQuestion *bridgeQuestionState `json:"current_question,omitempty"`
	LastError       string               `json:"last_error,omitempty"`
	Result          any                  `json:"result,omitempty"`
	ResultMeta      map[string]any       `json:"result_meta,omitempty"`
}

type bridgeEventEnvelope struct {
	EventID   string      `json:"event_id"`
	SessionID string      `json:"session_id"`
	RequestID string      `json:"request_id"`
	TaskID    string      `json:"task_id,omitempty"`
	Type      string      `json:"type"`
	TS        string      `json:"ts"`
	Payload   interface{} `json:"payload,omitempty"`
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type bridgeProgressEmitter struct {
	server *agentBridgeServer
	task   *bridgeTask
}

func newAgentBridgeServer(app *App, cfg Config, in io.Reader, out, stderr io.Writer) *agentBridgeServer {
	server := &agentBridgeServer{
		app:      app,
		cfg:      cfg,
		reader:   bufio.NewReader(in),
		writer:   out,
		stderr:   stderr,
		sessions: map[string]*bridgeSession{},
		tasks:    map[string]*bridgeTask{},
	}
	server.sessions["default"] = &bridgeSession{ID: "default", CreatedAt: time.Now().UTC()}
	return server
}

func (a *App) runAgentBridge(ctx context.Context, cfg Config, args []string) error {
	if len(args) > 0 && isHelpArg(args[0]) {
		_, err := io.WriteString(a.Stdout, AgentBridgeHelpText())
		return err
	}
	return newAgentBridgeServer(a, cfg, a.Stdin, a.Stdout, a.Stderr).Serve(ctx)
}

func AgentBridgeHelpText() string {
	return `Usage:
  officecli agent-bridge

Description:
  Exposes a structured interface for agent clients over JSON-RPC 2.0 via stdio.

Supported methods:
  initialize
  capabilities/get
  image_templates/list
  session/open
  session/close
  task/invoke
  task/respond
  task/status
  task/cancel
`
}

func (s *agentBridgeServer) Serve(ctx context.Context) error {
	for {
		req, err := readJSONRPCMessage(s.reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(req.ID) == 0 {
			continue
		}
		s.handleRequest(ctx, req)
	}
}

func (s *agentBridgeServer) handleRequest(ctx context.Context, req jsonRPCRequest) {
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		s.writeError(req.ID, -32600, "invalid jsonrpc version", nil)
		return
	}
	switch req.Method {
	case "initialize":
		s.writeResult(req.ID, s.initializeResult(ctx))
	case "capabilities/get":
		s.writeResult(req.ID, s.initializeResult(ctx).Capabilities)
	case "image_templates/list":
		templates, err := s.app.listImagePromptTemplates(ctx, s.cfg)
		if err != nil {
			s.writeError(req.ID, -32000, err.Error(), nil)
			return
		}
		s.writeResult(req.ID, templates)
	case "image_templates/create":
		var params CreateUserImagePromptTemplateRequest
		if err := decodeParams(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return
		}
		template, err := s.app.createUserImagePromptTemplate(ctx, s.cfg, params)
		if err != nil {
			s.writeError(req.ID, -32000, err.Error(), nil)
			return
		}
		s.writeResult(req.ID, template)
	case "image_template_publish_requests/create":
		var params CreateImageTemplatePublishRequest
		if err := decodeParams(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return
		}
		publishRequest, err := s.app.createImageTemplatePublishRequest(ctx, s.cfg, params)
		if err != nil {
			s.writeError(req.ID, -32000, err.Error(), nil)
			return
		}
		s.writeResult(req.ID, publishRequest)
	case "session/open":
		session := s.openSession()
		s.writeResult(req.ID, session)
	case "session/close":
		var params bridgeSessionParams
		if err := decodeParams(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return
		}
		if err := s.closeSession(params.SessionID); err != nil {
			s.writeError(req.ID, -32000, err.Error(), nil)
			return
		}
		s.writeResult(req.ID, map[string]any{"closed": true, "session_id": defaultIfEmpty(params.SessionID, "default")})
	case "task/invoke":
		var params bridgeInvokeParams
		if err := decodeParams(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return
		}
		task, err := s.invokeTask(ctx, req.ID, params)
		if err != nil {
			s.writeError(req.ID, -32000, err.Error(), nil)
			return
		}
		s.writeResult(req.ID, map[string]any{
			"task_id":    task.ID,
			"session_id": task.SessionID,
			"status":     task.Status,
		})
	case "task/respond":
		var params bridgeRespondParams
		if err := decodeParams(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return
		}
		if !IsValidTaskID(params.TaskID) {
			s.writeError(req.ID, -32602, "invalid_task_id", map[string]any{"task_id": params.TaskID})
			return
		}
		if err := s.respondTask(params); err != nil {
			s.writeError(req.ID, -32000, err.Error(), nil)
			return
		}
		s.writeResult(req.ID, map[string]any{"accepted": true, "task_id": params.TaskID})
	case "task/status":
		var params bridgeTaskStatusParams
		if err := decodeParams(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return
		}
		status, err := s.taskStatus(params.TaskID)
		if err != nil {
			s.writeError(req.ID, -32000, err.Error(), nil)
			return
		}
		s.writeResult(req.ID, status)
	case "task/cancel":
		var params bridgeCancelParams
		if err := decodeParams(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, err.Error(), nil)
			return
		}
		if !IsValidTaskID(params.TaskID) {
			s.writeError(req.ID, -32602, "invalid_task_id", map[string]any{"task_id": params.TaskID})
			return
		}
		if err := s.cancelTask(params.TaskID); err != nil {
			s.writeError(req.ID, -32000, err.Error(), nil)
			return
		}
		s.writeResult(req.ID, map[string]any{"cancelled": true, "task_id": params.TaskID})
	default:
		s.writeError(req.ID, -32601, "method not found", map[string]any{"method": req.Method})
	}
}

func (s *agentBridgeServer) initializeResult(ctx context.Context) bridgeInitializeResult {
	return bridgeInitializeResult{
		ServerName:      "officecli-agent-bridge",
		ServerVersion:   Version,
		ProtocolVersion: "2026-04-03",
		Capabilities: map[string]any{
			"methods": []string{
				"initialize",
				"capabilities/get",
				"image_templates/list",
				"session/open",
				"session/close",
				"task/invoke",
				"task/respond",
				"task/status",
				"task/cancel",
			},
			"event_types": []string{
				bridgeEventTaskStarted,
				bridgeEventTaskProgress,
				bridgeEventTaskQuestion,
				bridgeEventTaskOutput,
				bridgeEventTaskCompleted,
				bridgeEventTaskFailed,
				bridgeEventTaskCancelled,
			},
			"output_formats": []string{"json", "file", "bundle"},
			"document_generation": map[string]any{
				"pptx":   s.documentGenerationCapability(engine.DocumentTypePPTX),
				"docx":   s.documentGenerationCapability(engine.DocumentTypeDOCX),
				"xlsx":   s.documentGenerationCapability(engine.DocumentTypeXLSX),
				"report": s.documentGenerationCapability(engine.DocumentTypeReport),
				"img":    s.documentGenerationCapability(engine.DocumentTypeIMG),
			},
			"document_modification": map[string]any{
				"pptx": s.documentModificationCapability(engine.DocumentTypePPTX),
				"docx": s.documentModificationCapability(engine.DocumentTypeDOCX),
				"xlsx": s.documentModificationCapability(engine.DocumentTypeXLSX),
			},
			"image_generation": map[string]any{
				"provider_control":  "server",
				"preferred_tool":    bridgeToolOfficeGenerate,
				"document_type":     "img",
				"ratio_values":      []string{"square", "landscape", "portrait"},
				"publish_supported": true,
				"default_publish":   true,
				"disable_flag":      "--no-publish",
				"config_command":    "officecli config set-publish",
				"templates": map[string]any{
					"supported":    true,
					"list_method":  "image_templates/list",
					"invoke_field": "prompt_template_id",
				},
				"reference_image": map[string]any{
					"supported":          true,
					"max_count":          8,
					"invoke_field":       "reference_image",
					"invoke_field_array": "reference_images",
					"input":              "local path or http/https URL; pass an array via reference_images for multiple",
				},
				"size": map[string]any{
					"supported":    true,
					"invoke_field": "size",
					"format":       "WxH (e.g. 1280x768)",
					"notes":        "Overrides ratio when set; upstream model may snap to nearest supported tier.",
				},
				"notes": []string{
					"Standalone image generation uses the configured local image provider in external runtime mode.",
					"Hosted standalone image generation goes through the OfficeCLI server and consumes hosted credits.",
					"Successful standalone image generation is free and unlimited in external runtime mode.",
					"Standalone images publish online by default when publishing is configured; pass publish=false or --no-publish for local-only output.",
				},
			},
			"update": s.updateCapability(ctx),
		},
		Tools: []map[string]any{
			{
				"name": "office.prepare",
				"input_schema": map[string]any{
					"document_type": "pptx|docx|xlsx|report",
					"topic":         "string",
					"prompt":        "string",
					"file_path":     "string (.xlsx for report)",
					"lang":          "string",
					"style":         "string",
					"audience":      "string",
				},
			},
			{
				"name": "office.render",
				"input_schema": map[string]any{
					"document_type": "pptx|docx|xlsx|report",
					"topic":         "string",
					"payload":       "object",
					"runtime_mode":  "external|hosted",
					"out":           "string",
					"publish":       "boolean",
					"enable_images": "boolean",
					"image_quality": "deprecated; accepted for backward compat and ignored — PPT images always use the hosted image route",
					"emit_preview":  "boolean - emit <basename>.preview.html sidecar next to the artifact for pptx|docx|xlsx",
				},
			},
			{
				"name": "office.generate",
				"input_schema": map[string]any{
					"document_type":         "pptx|docx|xlsx|report|img",
					"topic":                 "string",
					"prompt":                "string",
					"file_path":             "string (.xlsx for report)",
					"mode":                  "fast|best",
					"runtime_mode":          "external|hosted",
					"ratio":                 "square|landscape|portrait (img only)",
					"size":                  "WxH explicit pixels, e.g. 1280x768 (img only)",
					"prompt_template_id":    "server-managed image prompt template id (img only)",
					"reference_image":       "local path or http/https URL (img only)",
					"reference_images":      "array of paths or URLs (img only)",
					"enable_reference_scan": "boolean (pptx only) - default true; false disables automatic recursive PPTX style scanning",
					"reference_root":        "directory root for recursive PPTX reference style scanning (pptx only)",
					"reference_pptx":        "explicit reference PPTX path (pptx only)",
					"reference_pptxs":       "array of explicit reference PPTX paths (pptx only)",
					"pptx_backend":          "officegen|artifact-experimental (pptx only) - default officegen",
					"image_quality":         "deprecated; accepted for backward compat and ignored — PPT images always use the hosted image route",
					"publish":               "boolean",
					"emit_preview":          "boolean - emit <basename>.preview.html sidecar next to the artifact for pptx|docx|xlsx",
				},
			},
			{
				"name": bridgeToolOfficeModify,
				"input_schema": map[string]any{
					"source_file": "string (.pptx|.docx|.xlsx)",
					"prompt":      "string",
					"format":      "pptx|docx|xlsx",
					"lang":        "string",
					"style":       "string",
					"out":         "string",
				},
			},
			{
				"name": "office.review",
				"input_schema": map[string]any{
					"document_type": "pptx",
					"file_path":     "string",
					"enable_visual": "boolean",
					"fail_below":    "0-100",
				},
			},
			{
				"name": "office.score",
				"input_schema": map[string]any{
					"document_type": "pptx",
					"file_path":     "string",
					"enable_visual": "boolean",
					"fail_below":    "0-100",
				},
			},
		},
	}
}

func (s *agentBridgeServer) updateCapability(ctx context.Context) map[string]any {
	if s == nil || s.app == nil {
		return map[string]any{"available": false}
	}
	info, err := s.app.safeCheckForUpdates(ctx)
	if err != nil {
		return map[string]any{
			"available":             false,
			"current_version":       Version,
			"current_commit":        Commit,
			"current_build_date":    BuildDate,
			"auto_update_supported": false,
			"check_error":           err.Error(),
		}
	}
	return map[string]any{
		"available":             info.Available,
		"channel":               info.Channel,
		"install_method":        info.InstallMethod,
		"package_manager":       info.PackageManager,
		"current_version":       info.CurrentVersion,
		"current_commit":        info.CurrentCommit,
		"current_build_date":    info.CurrentBuildDate,
		"latest_version_label":  info.LatestVersionLabel,
		"latest_published_at":   info.LatestPublishedAt,
		"auto_update_supported": info.AutoUpdateSupported,
		"update_command":        info.UpdateCommand,
	}
}

func (s *agentBridgeServer) documentGenerationCapability(documentType engine.DocumentType) map[string]any {
	if documentType == engine.DocumentTypeIMG {
		return map[string]any{
			"agent_render_supported": false,
			"preferred_tool":         bridgeToolOfficeGenerate,
			"prepare_required":       false,
			"image_generation": map[string]any{
				"provider_control": "server",
				"ratio_values":     []string{"square", "landscape", "portrait"},
				"templates": map[string]any{
					"supported":    true,
					"list_method":  "image_templates/list",
					"invoke_field": "prompt_template_id",
				},
				"reference_image": map[string]any{
					"supported":          true,
					"max_count":          8,
					"invoke_field":       "reference_image",
					"invoke_field_array": "reference_images",
					"input":              "local path or http/https URL; use reference_images array for multiple",
				},
				"size": map[string]any{
					"supported":    true,
					"invoke_field": "size",
					"format":       "WxH",
				},
			},
			"publish_support": map[string]any{
				"publish_supported": true,
				"default_publish":   true,
				"disable_flag":      "--no-publish",
				"config_command":    "officecli config set-publish",
			},
			"image_support": map[string]any{
				"default_enabled": true,
			},
		}
	}
	schema, err := runtime.AgentPayloadSchema(documentType)
	if err != nil {
		schema = map[string]any{
			"type":  "object",
			"error": err.Error(),
		}
	}
	capability := map[string]any{
		"agent_render_supported": true,
		"preferred_tool":         bridgeToolOfficeRender,
		"prepare_required":       documentType == engine.DocumentTypeReport,
		"payload_schema":         schema,
	}
	switch documentType {
	case engine.DocumentTypePPTX:
		capability["reference_style"] = map[string]any{
			"supported":              true,
			"invoke_tool":            bridgeToolOfficeGenerate,
			"default_recursive_scan": true,
			"enable_field":           "enable_reference_scan",
			"root_field":             "reference_root",
			"explicit_field":         "reference_pptx",
			"explicit_field_array":   "reference_pptxs",
			"disable_flag":           "--no-reference-scan",
			"root_flag":              "--reference-root",
			"explicit_flag":          "--reference-pptx",
			"notes": []string{
				"Reference style learning is available through office.generate and `officecli new pptx`; office.render consumes an already prepared payload and rejects reference scan fields.",
				"PPTX generation recursively scans the invocation working directory by default and summarizes all discovered .pptx files into a compact editable-style profile.",
				"Use enable_reference_scan=false to disable automatic scanning; explicit reference_pptx/reference_pptxs paths can still be supplied.",
				"The style profile is used as intent only; raw PPTX XML, images, and arbitrary low-level style fields are not copied into the prompt.",
			},
		}
		capability["pptx_backends"] = map[string]any{
			"supported":    true,
			"default":      runtime.PPTXBackendOfficegen,
			"invoke_field": "pptx_backend",
			"values":       []string{runtime.PPTXBackendOfficegen, runtime.PPTXBackendArtifactExperimental},
			"experimental": runtime.PPTXBackendArtifactExperimental,
			"failure_mode": "hard failure when artifact-experimental is explicitly requested and the local Node/artifact-tool worker is unavailable",
			"render_tool":  bridgeToolOfficeGenerate,
			"render_notes": "office.render rejects pptx_backend; use office.generate for reference-aware generation with experimental backend selection.",
		}
		capability["image_support"] = map[string]any{
			"default_enabled": true,
			"disable_flag":    "--no-images",
			"invoke_field":    "enable_images",
			"quality_field":   "image_quality (deprecated; accepted and ignored)",
			"config_command":  "officecli config set-generation",
			"config_fields":   []string{"image_base_url", "image_api_key", "image_model"},
			"notes": []string{
				"PPT images always use the hosted image route via account hosted credits; the legacy image_quality / --image-quality field is accepted for backward compat and ignored.",
				"If hosted credits are not configured, the deck is generated without images and a WARN_PPT_PREMIUM_IMAGE_DEGRADED warning is emitted.",
				"If you only want a text-only deck, disable images explicitly.",
			},
		}
	default:
		capability["image_support"] = map[string]any{
			"default_enabled": false,
		}
	}
	if documentType == engine.DocumentTypeReport {
		capability["source_file"] = map[string]any{
			"required":            true,
			"invoke_field":        "file_path",
			"accepted_extensions": []string{".xlsx"},
			"notes": []string{
				"Call office.prepare first for report generation so the agent receives workbook_summary and base_report_json.",
				"The workbook data is the source of truth for charts, tables, and findings.",
			},
		}
	}
	return capability
}

func (s *agentBridgeServer) documentModificationCapability(documentType engine.DocumentType) map[string]any {
	extension := "." + string(documentType)
	payloadSchema := map[string]any{
		"type":     "object",
		"required": []string{"source_file", "prompt"},
		"properties": map[string]any{
			"source_file": map[string]any{
				"type":                "string",
				"accepted_extensions": []string{extension},
			},
			"prompt": map[string]any{
				"type": "string",
			},
			"format": map[string]any{
				"type": "string",
				"enum": []string{string(documentType)},
			},
			"out": map[string]any{
				"type":        "string",
				"description": "Output directory for the modified file.",
			},
			"lang": map[string]any{
				"type": "string",
			},
			"style": map[string]any{
				"type": "string",
			},
		},
	}
	switch documentType {
	case engine.DocumentTypeDOCX:
		payloadSchema["op_aliases"] = map[string]string{
			"replace_paragraph_by_preview": "replace_docx_paragraph",
			"rewrite_section":              "rewrite_docx_document",
		}
	case engine.DocumentTypeXLSX:
		payloadSchema["op_aliases"] = map[string]string{
			"append_rows":   "append_xlsx_summary",
			"rewrite_sheet": "rewrite_xlsx_sheet",
			"update_cells":  "update_xlsx_cells",
		}
	}

	capability := map[string]any{
		"accepted_extensions":    []string{extension},
		"preferred_tool":         bridgeToolOfficeModify,
		"agent_modify_supported": true,
		"router_policy":          "sentinel-upgrade",
		"attention_whitelist":    documentModificationAttentionWhitelist(),
		"payload_schema":         payloadSchema,
	}
	switch documentType {
	case engine.DocumentTypePPTX:
		capability["limitations"] = []map[string]string{
			{
				"unsupported":          "SmartArt",
				"suggested_workaround": "Convert SmartArt to editable text or shapes before requesting modification.",
			},
		}
	case engine.DocumentTypeDOCX:
		capability["limitations"] = []string{"Minimal DOCX text modification support in v1."}
	case engine.DocumentTypeXLSX:
		capability["limitations"] = []string{"Formulas may not survive sheet rewrites."}
	}
	return capability
}

func documentModificationAttentionWhitelist() []string {
	return []string{
		"truncated_preview",
		"target_outside_preview",
		"sentinel_truncated_max_targets",
		"sentinel_dropped_max_depth",
		"partial_failure",
	}
}

func (s *agentBridgeServer) openSession() *bridgeSession {
	session := &bridgeSession{
		ID:        s.nextID("session"),
		CreatedAt: time.Now().UTC(),
	}
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
	return session
}

func (s *agentBridgeServer) closeSession(sessionID string) error {
	sessionID = defaultIfEmpty(sessionID, "default")
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	delete(s.sessions, sessionID)
	return nil
}

func (s *agentBridgeServer) invokeTask(ctx context.Context, rpcID json.RawMessage, params bridgeInvokeParams) (*bridgeTask, error) {
	sessionID := defaultIfEmpty(strings.TrimSpace(params.SessionID), "default")
	outputFormat := strings.TrimSpace(params.OutputFormat)
	if outputFormat == "" {
		outputFormat = "json"
	}
	switch outputFormat {
	case "json", "file", "bundle":
	default:
		return nil, fmt.Errorf("unsupported output_format: %s", outputFormat)
	}

	s.mu.Lock()
	if _, ok := s.sessions[sessionID]; !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	task := &bridgeTask{
		ID:          generateTaskID(),
		SessionID:   sessionID,
		RequestID:   normalizeRPCID(rpcID),
		Tool:        defaultIfEmpty(strings.TrimSpace(params.Tool), bridgeToolOfficeGenerate),
		Status:      "running",
		OutputFmt:   outputFormat,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		Interactive: params.Interactive,
	}
	s.tasks[task.ID] = task
	s.mu.Unlock()

	runCtx, cancel := context.WithCancel(ctx)
	task.Cancel = cancel
	switch defaultIfEmpty(strings.TrimSpace(params.Tool), bridgeToolOfficeGenerate) {
	case bridgeToolOfficePrepare:
		documentType, err := parseDocumentType(params.Args.DocumentType)
		if err != nil {
			s.mu.Lock()
			delete(s.tasks, task.ID)
			s.mu.Unlock()
			return nil, err
		}
		s.emitEvent(task, bridgeEventTaskStarted, map[string]any{
			"tool":          task.Tool,
			"document_type": documentType,
		})
		go s.runPrepareTask(runCtx, task, runtime.PrepareParams{
			DocumentType:   documentType,
			Topic:          strings.TrimSpace(params.Args.Topic),
			SourceFilePath: strings.TrimSpace(params.Args.FilePath),
		})
		return task, nil
	case bridgeToolOfficeRender:
		job, payload, err := s.app.buildRenderJobFromRequest(s.cfg, params)
		if err != nil {
			s.mu.Lock()
			delete(s.tasks, task.ID)
			s.mu.Unlock()
			return nil, err
		}
		s.emitEvent(task, bridgeEventTaskStarted, map[string]any{
			"tool":          task.Tool,
			"document_type": job.DocumentType,
			"runtime_mode":  job.RuntimeMode,
			"enable_images": job.EnableImages,
		})
		go s.runRenderTask(runCtx, task, job, payload)
		return task, nil
	case bridgeToolOfficeGenerate:
		job, err := s.app.buildGenerateJobFromRequest(s.cfg, params)
		if err != nil {
			s.mu.Lock()
			delete(s.tasks, task.ID)
			s.mu.Unlock()
			return nil, err
		}
		prompter := (*bridgePrompter)(nil)
		if params.Interactive && job.Mode == "best" {
			prompter = &bridgePrompter{
				ctx:    runCtx,
				server: s,
				task:   task,
				answer: make(chan bridgePromptResponse, 1),
			}
			task.Prompt = prompter
		}
		s.emitEvent(task, bridgeEventTaskStarted, map[string]any{
			"tool":          task.Tool,
			"document_type": job.DocumentType,
			"mode":          job.Mode,
			"runtime_mode":  job.RuntimeMode,
			"interactive":   params.Interactive,
		})
		go s.runGenerateTask(runCtx, task, job, prompter)
		return task, nil
	case bridgeToolOfficeModify:
		modifyParams, err := s.buildModifyParamsFromRequest(params)
		if err != nil {
			s.mu.Lock()
			delete(s.tasks, task.ID)
			s.mu.Unlock()
			return nil, err
		}
		s.emitEvent(task, bridgeEventTaskStarted, map[string]any{
			"tool":          task.Tool,
			"document_type": modifyParams.DocumentType,
			"source_file":   modifyParams.SourceFilePath,
		})
		go s.runModifyTask(runCtx, task, modifyParams)
		return task, nil
	case bridgeToolOfficeReview, bridgeToolOfficeScore:
		job, err := s.app.buildReviewJobFromRequest(params)
		if err != nil {
			s.mu.Lock()
			delete(s.tasks, task.ID)
			s.mu.Unlock()
			return nil, err
		}
		s.emitEvent(task, bridgeEventTaskStarted, map[string]any{
			"tool":          task.Tool,
			"document_type": job.DocumentType,
			"file_path":     job.FilePath,
			"enable_visual": job.EnableVisual,
			"fail_below":    job.FailBelow,
		})
		go s.runReviewTask(runCtx, task, job)
		return task, nil
	default:
		s.mu.Lock()
		delete(s.tasks, task.ID)
		s.mu.Unlock()
		return nil, fmt.Errorf("unsupported tool: %s", params.Tool)
	}
}

func (s *agentBridgeServer) buildModifyParamsFromRequest(req bridgeInvokeParams) (runtime.ModifyParams, error) {
	sourceFile := strings.TrimSpace(req.Args.SourceFile)
	if sourceFile == "" {
		sourceFile = strings.TrimSpace(req.Args.FilePath)
	}
	if sourceFile == "" {
		return runtime.ModifyParams{}, fmt.Errorf("source_file is required")
	}

	prompt := strings.TrimSpace(req.Args.Prompt)
	if prompt == "" {
		return runtime.ModifyParams{}, fmt.Errorf("prompt is required")
	}

	format := strings.TrimSpace(req.Args.Format)
	if format == "" {
		format = strings.TrimSpace(req.Args.DocumentType)
	}
	if format == "" {
		format = strings.TrimPrefix(strings.ToLower(filepath.Ext(sourceFile)), ".")
	}
	documentType, err := parseDocumentType(format)
	if err != nil {
		return runtime.ModifyParams{}, err
	}
	switch documentType {
	case engine.DocumentTypePPTX, engine.DocumentTypeDOCX, engine.DocumentTypeXLSX:
	default:
		return runtime.ModifyParams{}, fmt.Errorf("office.modify supports pptx, docx, and xlsx; got %s", documentType)
	}

	outputDir := strings.TrimSpace(req.Args.OutputDir)
	if outputDir == "" {
		outputDir = strings.TrimSpace(s.cfg.Defaults.OutputDir)
	}
	if outputDir == "" {
		outputDir = filepath.Dir(sourceFile)
		if outputDir == "" || outputDir == "." {
			outputDir, _ = os.Getwd()
		}
	}
	ext := filepath.Ext(sourceFile)
	outputPath := filepath.Join(outputDir, strings.TrimSuffix(filepath.Base(sourceFile), ext)+".modified"+ext)

	return runtime.ModifyParams{
		SourceFilePath: sourceFile,
		DocumentType:   documentType,
		Prompt:         prompt,
		Language:       strings.TrimSpace(req.Args.Language),
		Style:          strings.TrimSpace(req.Args.Style),
		OutputPath:     outputPath,
	}, nil
}

func (s *agentBridgeServer) runPrepareTask(ctx context.Context, task *bridgeTask, params runtime.PrepareParams) {
	result, err := runtime.PrepareAgentPayload(params)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			s.updateTask(task.ID, func(t *bridgeTask) {
				t.Status = "cancelled"
				t.UpdatedAt = time.Now().UTC()
				t.LastError = context.Canceled.Error()
			})
			s.emitEvent(task, bridgeEventTaskCancelled, map[string]any{"reason": "cancelled"})
			return
		}
		payload := classifyBridgeError(err)
		s.updateTask(task.ID, func(t *bridgeTask) {
			t.Status = "failed"
			t.UpdatedAt = time.Now().UTC()
			t.LastError = err.Error()
		})
		s.emitEvent(task, bridgeEventTaskFailed, payload)
		return
	}

	bridgeResult := bridgePrepareResult{
		Status:          "ready",
		DocumentType:    result.DocumentType,
		PreferredTool:   result.PreferredTool,
		PrepareRequired: result.PrepareRequired,
		PayloadSchema:   result.PayloadSchema,
		FieldNotes:      append([]string(nil), result.FieldNotes...),
		WorkbookSummary: result.WorkbookSummary,
		BaseReportJSON:  result.BaseReportJSON,
	}
	s.updateTask(task.ID, func(t *bridgeTask) {
		t.Status = "completed"
		t.UpdatedAt = time.Now().UTC()
		t.Result = bridgeResult
	})
	s.emitEvent(task, bridgeEventTaskOutput, s.outputPayload(task.OutputFmt, bridgeResult))
	s.emitEvent(task, bridgeEventTaskCompleted, map[string]any{
		"status":           bridgeResult.Status,
		"document_type":    bridgeResult.DocumentType,
		"preferred_tool":   bridgeResult.PreferredTool,
		"prepare_required": bridgeResult.PrepareRequired,
	})
}

func (s *agentBridgeServer) runModifyTask(ctx context.Context, task *bridgeTask, params runtime.ModifyParams) {
	progress := &bridgeProgressEmitter{server: s, task: task}
	if missing := missingLLMConfig(s.cfg); missing != "" {
		err := fmt.Errorf("generation service is not fully configured: missing %s. Run `officecli config set-generation` to finish setup", missing)
		s.updateTask(task.ID, func(t *bridgeTask) {
			t.Status = "failed"
			t.UpdatedAt = time.Now().UTC()
			t.LastError = err.Error()
		})
		s.emitEvent(task, bridgeEventTaskFailed, classifyBridgeError(err))
		return
	}

	llmClient, err := s.app.newLLMClient(s.cfg.LLM)
	if err != nil {
		s.updateTask(task.ID, func(t *bridgeTask) {
			t.Status = "failed"
			t.UpdatedAt = time.Now().UTC()
			t.LastError = err.Error()
		})
		s.emitEvent(task, bridgeEventTaskFailed, classifyBridgeError(err))
		return
	}

	service := runtime.NewService(llmClient, progress)
	result, err := service.Modify(ctx, params, progress)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			s.updateTask(task.ID, func(t *bridgeTask) {
				t.Status = "cancelled"
				t.UpdatedAt = time.Now().UTC()
				t.LastError = context.Canceled.Error()
			})
			s.emitEvent(task, bridgeEventTaskCancelled, map[string]any{"reason": "cancelled"})
			return
		}
		s.updateTask(task.ID, func(t *bridgeTask) {
			t.Status = "failed"
			t.UpdatedAt = time.Now().UTC()
			t.LastError = err.Error()
		})
		s.emitEvent(task, bridgeEventTaskFailed, classifyBridgeError(err))
		return
	}

	if err := os.MkdirAll(filepath.Dir(params.OutputPath), 0o755); err != nil {
		s.updateTask(task.ID, func(t *bridgeTask) {
			t.Status = "failed"
			t.UpdatedAt = time.Now().UTC()
			t.LastError = err.Error()
		})
		s.emitEvent(task, bridgeEventTaskFailed, classifyBridgeError(err))
		return
	}
	if err := os.WriteFile(params.OutputPath, result.Bytes, 0o644); err != nil {
		s.updateTask(task.ID, func(t *bridgeTask) {
			t.Status = "failed"
			t.UpdatedAt = time.Now().UTC()
			t.LastError = err.Error()
		})
		s.emitEvent(task, bridgeEventTaskFailed, classifyBridgeError(err))
		return
	}

	bridgeResult := bridgeModifyResult{
		Status:       "completed",
		DocumentType: string(params.DocumentType),
		OutputFile:   params.OutputPath,
		Warnings:     append([]string(nil), result.ResultMeta.Warnings...),
		ResultMeta:   buildModifyBridgeMeta(result.ResultMeta),
	}
	s.updateTask(task.ID, func(t *bridgeTask) {
		t.Status = "completed"
		t.UpdatedAt = time.Now().UTC()
		t.Result = bridgeResult
	})
	s.emitEvent(task, bridgeEventTaskOutput, s.outputPayload(task.OutputFmt, bridgeResult))
	s.emitEvent(task, bridgeEventTaskCompleted, map[string]any{
		"status":        bridgeResult.Status,
		"document_type": bridgeResult.DocumentType,
		"output_file":   bridgeResult.OutputFile,
		"warnings":      append([]string(nil), bridgeResult.Warnings...),
		"result_meta":   bridgeResult.ResultMeta,
	})
}

func (s *agentBridgeServer) runRenderTask(ctx context.Context, task *bridgeTask, job GenerateJob, payload json.RawMessage) {
	progress := &bridgeProgressEmitter{server: s, task: task}
	result, err := s.app.executeRenderJob(ctx, s.cfg, job, payload, progress)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			s.updateTask(task.ID, func(t *bridgeTask) {
				t.Status = "cancelled"
				t.UpdatedAt = time.Now().UTC()
				t.LastError = context.Canceled.Error()
			})
			s.emitEvent(task, bridgeEventTaskCancelled, map[string]any{"reason": "cancelled"})
			return
		}
		errPayload := failureEventPayload(classifyBridgeError(err), inferCreditMode(job))
		s.updateTask(task.ID, func(t *bridgeTask) {
			t.Status = "failed"
			t.UpdatedAt = time.Now().UTC()
			t.LastError = err.Error()
		})
		s.emitEvent(task, bridgeEventTaskFailed, errPayload)
		return
	}

	s.updateTask(task.ID, func(t *bridgeTask) {
		t.Status = "completed"
		t.UpdatedAt = time.Now().UTC()
		t.Result = result
	})
	s.emitEvent(task, bridgeEventTaskOutput, s.outputPayload(task.OutputFmt, result))
	s.emitEvent(task, bridgeEventTaskCompleted, generateTaskCompletedPayload(result))
}

func (s *agentBridgeServer) runGenerateTask(ctx context.Context, task *bridgeTask, job GenerateJob, prompter *bridgePrompter) {
	progress := &bridgeProgressEmitter{server: s, task: task}
	var protocolPrompter Prompter
	if prompter != nil {
		protocolPrompter = prompter
	}
	result, err := s.app.executeGenerateJob(ctx, s.cfg, job, false, progress, protocolPrompter)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			s.updateTask(task.ID, func(t *bridgeTask) {
				t.Status = "cancelled"
				t.UpdatedAt = time.Now().UTC()
				t.LastError = context.Canceled.Error()
				t.CurrentQ = nil
			})
			s.emitEvent(task, bridgeEventTaskCancelled, map[string]any{"reason": "cancelled"})
			return
		}
		errPayload := failureEventPayload(classifyBridgeError(err), inferCreditMode(job))
		s.updateTask(task.ID, func(t *bridgeTask) {
			t.Status = "failed"
			t.UpdatedAt = time.Now().UTC()
			t.LastError = err.Error()
			t.CurrentQ = nil
		})
		s.emitEvent(task, bridgeEventTaskFailed, errPayload)
		return
	}

	s.updateTask(task.ID, func(t *bridgeTask) {
		t.Status = "completed"
		t.UpdatedAt = time.Now().UTC()
		t.Result = result
		t.CurrentQ = nil
	})
	s.emitEvent(task, bridgeEventTaskOutput, s.outputPayload(task.OutputFmt, result))
	s.emitEvent(task, bridgeEventTaskCompleted, generateTaskCompletedPayload(result))
}

func generateTaskCompletedPayload(result GenerateResult) map[string]any {
	return map[string]any{
		"status":          result.Status,
		"document_type":   result.DocumentType,
		"document_name":   result.DocumentName,
		"file_path":       result.FilePath,
		"warnings":        append([]string(nil), result.Warnings...),
		"result_meta":     buildGenerateBridgeMeta(result),
		"credits_charged": result.CreditsCharged,
		"credit_balance":  result.CreditBalance,
		"credit_mode":     result.CreditMode,
	}
}

func (s *agentBridgeServer) runReviewTask(ctx context.Context, task *bridgeTask, job ReviewJob) {
	progress := &bridgeProgressEmitter{server: s, task: task}
	result, err := s.app.executeReviewJob(ctx, s.cfg, job, progress)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
			s.updateTask(task.ID, func(t *bridgeTask) {
				t.Status = "cancelled"
				t.UpdatedAt = time.Now().UTC()
				t.LastError = context.Canceled.Error()
			})
			s.emitEvent(task, bridgeEventTaskCancelled, map[string]any{"reason": "cancelled"})
			return
		}
		payload := classifyBridgeError(err)
		s.updateTask(task.ID, func(t *bridgeTask) {
			t.Status = "failed"
			t.UpdatedAt = time.Now().UTC()
			t.LastError = err.Error()
		})
		s.emitEvent(task, bridgeEventTaskFailed, payload)
		return
	}

	s.updateTask(task.ID, func(t *bridgeTask) {
		t.Status = "completed"
		t.UpdatedAt = time.Now().UTC()
		t.Result = result
	})
	s.emitEvent(task, bridgeEventTaskOutput, s.outputPayload(task.OutputFmt, *result))
	s.emitEvent(task, bridgeEventTaskCompleted, map[string]any{
		"status":          result.Status,
		"document_type":   result.DocumentType,
		"overall_score":   result.OverallScore,
		"visual_score":    result.VisualScore,
		"structure_score": result.StructureScore,
		"used_visual":     result.UsedVisual,
		"warnings":        append([]string(nil), result.Warnings...),
	})
}

func (s *agentBridgeServer) respondTask(params bridgeRespondParams) error {
	s.mu.Lock()
	task, ok := s.tasks[params.TaskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", params.TaskID)
	}
	if task.Prompt == nil || task.CurrentQ == nil {
		return fmt.Errorf("task %s has no pending question", params.TaskID)
	}
	if params.QuestionID != "" && task.CurrentQ.ID != params.QuestionID {
		return fmt.Errorf("question mismatch: want %s got %s", task.CurrentQ.ID, params.QuestionID)
	}
	answer := bridgePromptResponse{
		OptionID: strings.TrimSpace(params.OptionID),
		Answer:   strings.TrimSpace(params.Answer),
	}
	if answer.OptionID == "" && answer.Answer == "" {
		return fmt.Errorf("either option_id or answer is required")
	}
	select {
	case task.Prompt.answer <- answer:
		s.updateTask(task.ID, func(t *bridgeTask) {
			t.Status = "running"
			t.UpdatedAt = time.Now().UTC()
			t.CurrentQ = nil
		})
		return nil
	default:
		return fmt.Errorf("task %s is not accepting new answers", params.TaskID)
	}
}

func (s *agentBridgeServer) taskStatus(taskID string) (*bridgeTaskStatusResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}
	return &bridgeTaskStatusResult{
		TaskID:          task.ID,
		SessionID:       task.SessionID,
		Status:          task.Status,
		Tool:            task.Tool,
		OutputFormat:    task.OutputFmt,
		Interactive:     task.Interactive,
		CreatedAt:       task.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:       task.UpdatedAt.Format(time.RFC3339Nano),
		CurrentQuestion: task.CurrentQ,
		LastError:       task.LastError,
		Result:          task.Result,
		ResultMeta:      buildBridgeResultMeta(task.Result),
	}, nil
}

func (s *agentBridgeServer) cancelTask(taskID string) error {
	s.mu.Lock()
	task, ok := s.tasks[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if task.Cancel == nil {
		return fmt.Errorf("task %s cannot be cancelled", taskID)
	}
	task.Cancel()
	return nil
}

func (s *agentBridgeServer) outputPayload(outputFormat string, result any) map[string]any {
	switch typed := result.(type) {
	case bridgePrepareResult:
		return map[string]any{
			"format":           outputFormat,
			"status":           typed.Status,
			"document_type":    typed.DocumentType,
			"preferred_tool":   typed.PreferredTool,
			"prepare_required": typed.PrepareRequired,
			"payload_schema":   typed.PayloadSchema,
			"field_notes":      append([]string(nil), typed.FieldNotes...),
			"workbook_summary": typed.WorkbookSummary,
			"base_report_json": typed.BaseReportJSON,
			"result":           typed,
		}
	case GenerateResult:
		payload := map[string]any{
			"format":        outputFormat,
			"status":        typed.Status,
			"file_path":     typed.FilePath,
			"document_type": typed.DocumentType,
			"document_name": typed.DocumentName,
			"warnings":      append([]string(nil), typed.Warnings...),
			"result_meta":   buildGenerateBridgeMeta(typed),
			"result":        typed,
		}
		if outputFormat == "bundle" {
			payload["artifact"] = map[string]any{
				"file_path":     typed.FilePath,
				"document_name": typed.DocumentName,
				"document_type": typed.DocumentType,
			}
		}
		return payload
	case bridgeModifyResult:
		return map[string]any{
			"format":        outputFormat,
			"status":        typed.Status,
			"output_file":   typed.OutputFile,
			"document_type": typed.DocumentType,
			"warnings":      append([]string(nil), typed.Warnings...),
			"result_meta":   typed.ResultMeta,
			"result":        typed,
		}
	case ReviewResult:
		return map[string]any{
			"format":          outputFormat,
			"status":          typed.Status,
			"file_path":       typed.FilePath,
			"document_type":   typed.DocumentType,
			"overall_score":   typed.OverallScore,
			"visual_score":    typed.VisualScore,
			"structure_score": typed.StructureScore,
			"warnings":        append([]string(nil), typed.Warnings...),
			"result":          typed,
		}
	default:
		return map[string]any{"format": outputFormat, "result": typed}
	}
}

func buildBridgeResultMeta(result any) map[string]any {
	switch typed := result.(type) {
	case GenerateResult:
		return buildGenerateBridgeMeta(typed)
	case bridgeModifyResult:
		return typed.ResultMeta
	default:
		return nil
	}
}

func buildModifyBridgeMeta(meta runtime.ModifyResultMeta) map[string]any {
	return map[string]any{
		"modify": map[string]any{
			"router_path":        meta.RouterPath,
			"ops_applied":        meta.OpsApplied,
			"ops_failed":         meta.OpsFailed,
			"llm_calls":          meta.LLMCalls,
			"fidelity":           map[string]any{"preserved": append([]string(nil), meta.Fidelity.Preserved...), "dropped": append([]string(nil), meta.Fidelity.Dropped...)},
			"warnings":           append([]string(nil), meta.Warnings...),
			"attention_required": meta.AttentionRequired,
		},
	}
}

func buildGenerateBridgeMeta(result GenerateResult) map[string]any {
	if !strings.EqualFold(strings.TrimSpace(result.DocumentType), "pptx") {
		return nil
	}
	attentionRequired := hasPPTImageGuidanceWarning(result.Warnings)
	imageSupport := map[string]any{
		"default_enabled":    true,
		"disable_flag":       "--no-images",
		"config_command":     "officecli config set-generation",
		"config_fields":      []string{"image_base_url", "image_api_key", "image_model"},
		"attention_required": attentionRequired,
	}
	if attentionRequired {
		imageSupport["reason"] = "image_generation_degraded"
		imageSupport["message"] = "The PPT output was downgraded to a text-only version. Check the image model URL, API key, and model name, or use --no-images."
	}
	meta := map[string]any{
		"image_support": imageSupport,
	}
	if result.ReferenceStyle != nil {
		meta["reference_style"] = result.ReferenceStyle
	}
	if result.PPTXArtifactDebug != nil {
		meta["pptx_artifact_debug"] = result.PPTXArtifactDebug
	}
	if result.PPTXReview != nil {
		meta["pptx_review"] = result.PPTXReview
	}
	if strings.TrimSpace(result.PPTXBackend) != "" {
		meta["pptx_backend"] = strings.TrimSpace(result.PPTXBackend)
	}
	return meta
}

func hasPPTImageGuidanceWarning(warnings []string) bool {
	for _, warning := range warnings {
		text := strings.TrimSpace(strings.ToLower(warning))
		if text == "" {
			continue
		}
		if strings.Contains(text, "image generation failed") ||
			strings.Contains(text, "text-only version") ||
			strings.Contains(text, "set-generation") ||
			strings.Contains(text, "--no-images") {
			return true
		}
	}
	return false
}

func (s *agentBridgeServer) emitEvent(task *bridgeTask, eventType string, payload any) {
	_ = s.writeMessage(jsonRPCNotification{
		JSONRPC: "2.0",
		Method:  "event",
		Params: bridgeEventEnvelope{
			EventID:   s.nextID("event"),
			SessionID: task.SessionID,
			RequestID: task.RequestID,
			TaskID:    task.ID,
			Type:      eventType,
			TS:        time.Now().UTC().Format(time.RFC3339Nano),
			Payload:   payload,
		},
	})
}

func (s *agentBridgeServer) updateTask(taskID string, update func(*bridgeTask)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if task, ok := s.tasks[taskID]; ok {
		update(task)
	}
}

func (s *agentBridgeServer) nextID(prefix string) string {
	value := s.seq.Add(1)
	return fmt.Sprintf("%s-%06d", prefix, value)
}

func (s *agentBridgeServer) writeResult(id json.RawMessage, result any) {
	_ = s.writeMessage(jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *agentBridgeServer) writeError(id json.RawMessage, code int, message string, data any) {
	_ = s.writeMessage(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &jsonRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	})
}

func (s *agentBridgeServer) writeMessage(msg any) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := fmt.Fprintf(s.writer, "Content-Length: %d\r\n\r\n", len(raw)); err != nil {
		return err
	}
	_, err = s.writer.Write(raw)
	return err
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	return nil
}

func defaultIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeRPCID(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err == nil {
		switch value := decoded.(type) {
		case string:
			return value
		case float64:
			if value == float64(int64(value)) {
				return strconv.FormatInt(int64(value), 10)
			}
			return strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	return strings.Trim(trimmed, `"`)
}

func failureEventPayload(errInfo bridgeErrorPayload, creditMode string) map[string]any {
	return map[string]any{
		"type":            errInfo.Type,
		"code":            errInfo.Code,
		"message":         errInfo.Message,
		"retryable":       errInfo.Retryable,
		"credits_charged": 0,
		"credit_mode":     creditMode,
	}
}

func classifyBridgeError(err error) bridgeErrorPayload {
	message := strings.TrimSpace(err.Error())
	payload := bridgeErrorPayload{
		Type:    "execution_error",
		Code:    "execution_failed",
		Message: message,
	}

	switch {
	case strings.Contains(message, "missing") && strings.Contains(message, "config"):
		payload.Type = "configuration_error"
		payload.Code = "configuration_missing"
	case strings.Contains(message, "platform service is not fully configured"):
		payload.Type = "configuration_error"
		payload.Code = "platform_configuration_missing"
	case strings.Contains(message, "generation service is not fully configured"):
		payload.Type = "configuration_error"
		payload.Code = "llm_configuration_missing"
	case strings.Contains(message, "api-key validation failed"), strings.Contains(message, "access check failed"), strings.Contains(message, "license"):
		payload.Type = "auth_error"
		payload.Code = "license_check_failed"
	case strings.Contains(message, "topic is required"), strings.Contains(message, "payload is required"), strings.Contains(message, "file_path is required"), strings.Contains(message, "file is required"), strings.Contains(message, "report generation requires file_path"), strings.Contains(message, "unsupported"), strings.Contains(message, "review is currently only supported"), strings.Contains(message, "option_id"), strings.Contains(message, "answer"), strings.Contains(message, "session not found"), strings.Contains(message, "task not found"), strings.Contains(message, "question mismatch"), strings.Contains(message, "invalid fail_below"):
		payload.Type = "validation_error"
		payload.Code = "invalid_request"
	case strings.Contains(message, "document assembly failed"), strings.Contains(message, "parse llm response"), strings.Contains(message, "slides cannot be empty"):
		payload.Type = "assembly_error"
		payload.Code = "document_assembly_failed"
	case strings.Contains(message, "failed to read local PPT"):
		payload.Type = "io_error"
		payload.Code = "file_read_failed"
	case strings.Contains(message, "content generation failed"), strings.Contains(message, "llm request failed"), strings.Contains(message, "internal llm request failed"):
		payload.Type = "llm_error"
		payload.Code = "llm_request_failed"
		payload.Retryable = true
	}

	return payload
}

func readJSONRPCMessage(reader *bufio.Reader) (jsonRPCRequest, error) {
	var contentLength int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return jsonRPCRequest{}, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			if value == line {
				value = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "content-length:"))
			}
			length, err := strconv.Atoi(value)
			if err != nil {
				return jsonRPCRequest{}, fmt.Errorf("invalid Content-Length: %w", err)
			}
			contentLength = length
		}
	}
	if contentLength <= 0 {
		return jsonRPCRequest{}, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return jsonRPCRequest{}, err
	}
	var req jsonRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return jsonRPCRequest{}, err
	}
	return req, nil
}

func (p *bridgePrompter) Ask(question string, options []string, allowFreeform bool) (string, string, error) {
	questionState := &bridgeQuestionState{
		ID:            p.server.nextID("question"),
		Question:      question,
		AllowFreeform: allowFreeform,
	}
	for idx, option := range options {
		questionState.Options = append(questionState.Options, bridgeQuestionOption{
			ID:    strconv.Itoa(idx + 1),
			Label: option,
		})
	}
	p.server.updateTask(p.task.ID, func(task *bridgeTask) {
		task.Status = "waiting_input"
		task.UpdatedAt = time.Now().UTC()
		task.CurrentQ = questionState
	})
	p.server.emitEvent(p.task, bridgeEventTaskQuestion, questionState)

	select {
	case <-p.ctx.Done():
		return "", "", p.ctx.Err()
	case response := <-p.answer:
		return response.OptionID, response.Answer, nil
	}
}

func (e *bridgeProgressEmitter) Emit(_ context.Context, event engine.ProgressEvent) {
	payload := map[string]any{
		"step":    event.Step,
		"status":  event.Status,
		"content": strings.TrimSpace(event.Content),
	}
	if strings.TrimSpace(event.ActiveContent) != "" {
		payload["active_content"] = strings.TrimSpace(event.ActiveContent)
	}
	if event.ElapsedMs > 0 {
		payload["elapsed_ms"] = event.ElapsedMs
	}
	if event.DurationMs > 0 {
		payload["duration_ms"] = event.DurationMs
	}
	if event.ImageSlideCount > 0 {
		payload["image_slide_count"] = event.ImageSlideCount
	}
	if strings.TrimSpace(event.Error) != "" {
		payload["error"] = strings.TrimSpace(event.Error)
	}
	e.server.emitEvent(e.task, bridgeEventTaskProgress, payload)
}

func (e *bridgeProgressEmitter) Pause(message string) {
	e.server.emitEvent(e.task, bridgeEventTaskProgress, map[string]any{
		"step":    progressStepQuestion,
		"status":  "waiting_input",
		"content": strings.TrimSpace(message),
	})
}
