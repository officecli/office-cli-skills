package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestAgentBridgeInitializeIncludesDocumentModification(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	result := server.initializeResult(context.Background())

	caps, ok := result.Capabilities["document_modification"].(map[string]any)
	if !ok {
		t.Fatalf("document_modification capability missing: %#v", result.Capabilities)
	}

	expectedWhitelist := []string{
		"truncated_preview",
		"target_outside_preview",
		"sentinel_truncated_max_targets",
		"sentinel_dropped_max_depth",
		"partial_failure",
	}
	for format, expectedExt := range map[string]string{
		"pptx": ".pptx",
		"docx": ".docx",
		"xlsx": ".xlsx",
	} {
		t.Run(format, func(t *testing.T) {
			formatCaps, ok := caps[format].(map[string]any)
			if !ok {
				t.Fatalf("%s modification capability missing: %#v", format, caps)
			}
			if formatCaps["router_policy"] != "sentinel-upgrade" {
				t.Fatalf("%s router_policy = %#v", format, formatCaps["router_policy"])
			}
			if formatCaps["preferred_tool"] != "office.modify" {
				t.Fatalf("%s preferred_tool = %#v", format, formatCaps["preferred_tool"])
			}
			if formatCaps["agent_modify_supported"] != true {
				t.Fatalf("%s agent_modify_supported = %#v", format, formatCaps["agent_modify_supported"])
			}
			if got := stringSlice(t, formatCaps["accepted_extensions"]); !reflect.DeepEqual(got, []string{expectedExt}) {
				t.Fatalf("%s accepted_extensions = %#v", format, got)
			}
			if got := stringSlice(t, formatCaps["attention_whitelist"]); !reflect.DeepEqual(got, expectedWhitelist) {
				t.Fatalf("%s attention_whitelist = %#v", format, got)
			}
		})
	}
}

func TestAgentBridgeOfficeModify(t *testing.T) {
	src := seedPath(t, "pptx")
	outDir := t.TempDir()
	llmResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "replace_slide_title",
				"target":  map[string]any{"slide": 1},
				"payload": map[string]any{"new_title": "Bridge Modified Title"},
			},
		},
		"__needs_rewrite": []string{},
	}
	respBytes, _ := json.Marshal(llmResp)

	out := bytes.NewBuffer(nil)
	server := newAgentBridgeServer(
		newModifyTestApp(&modifyFakeLLMClient{responses: [][]byte{respBytes}}),
		testLLMConfig(),
		bytes.NewBuffer(nil),
		out,
		bytes.NewBuffer(nil),
	)

	var params bridgeInvokeParams
	rawParams, _ := json.Marshal(map[string]any{
		"tool":          "office.modify",
		"output_format": "json",
		"args": map[string]any{
			"source_file": src,
			"prompt":      "Change the title of slide 1 to Bridge Modified Title",
			"format":      "pptx",
			"out":         outDir,
		},
	})
	if err := decodeParams(rawParams, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}

	task, err := server.invokeTask(context.Background(), json.RawMessage(`1`), params)
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
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	status, err := server.taskStatus(task.ID)
	if err != nil {
		t.Fatalf("taskStatus final: %v", err)
	}
	if status.Status != "completed" {
		t.Fatalf("task status = %s, last_error=%s", status.Status, status.LastError)
	}

	messages := readBufferedRPCMessages(t, out.Bytes())
	var outputs []map[string]any
	for _, msg := range messages {
		if msg["method"] != "event" {
			continue
		}
		params := msg["params"].(map[string]any)
		if params["type"] != bridgeEventTaskOutput {
			continue
		}
		outputs = append(outputs, params["payload"].(map[string]any))
	}
	if len(outputs) != 1 {
		t.Fatalf("task.output count = %d, want 1; messages=%#v", len(outputs), messages)
	}

	payload := outputs[0]
	resultMeta := payload["result_meta"].(map[string]any)
	modifyMeta := resultMeta["modify"].(map[string]any)
	if modifyMeta["router_path"] != "B" {
		t.Fatalf("router_path = %#v", modifyMeta["router_path"])
	}
	if modifyMeta["ops_applied"] != float64(1) {
		t.Fatalf("ops_applied = %#v", modifyMeta["ops_applied"])
	}
	if modifyMeta["llm_calls"] != float64(1) {
		t.Fatalf("llm_calls = %#v", modifyMeta["llm_calls"])
	}
	if _, ok := modifyMeta["fidelity"].(map[string]any); !ok {
		t.Fatalf("fidelity missing from modify meta: %#v", modifyMeta)
	}
	resultPayload := payload["result"].(map[string]any)
	outputFile, ok := resultPayload["output_file"].(string)
	if !ok || filepath.Ext(outputFile) != ".pptx" {
		t.Fatalf("result.output_file = %#v", resultPayload["output_file"])
	}
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			s, ok := item.(string)
			if !ok {
				t.Fatalf("non-string slice item: %#v", item)
			}
			out = append(out, s)
		}
		return out
	default:
		t.Fatalf("value is not a string slice: %#v", value)
		return nil
	}
}

func readBufferedRPCMessages(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	reader := bufio.NewReader(bytes.NewReader(raw))
	var messages []map[string]any
	for {
		if _, err := reader.Peek(1); err != nil {
			break
		}
		messages = append(messages, readRPC(t, reader))
	}
	return messages
}
