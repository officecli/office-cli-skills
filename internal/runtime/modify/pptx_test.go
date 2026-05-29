package modify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli-internal/engine"
	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

type fakeLLMClient struct {
	responses [][]byte
	calls     int
}

func (f *fakeLLMClient) CompleteText(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return f.next()
}

func (f *fakeLLMClient) CompleteJSON(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return f.next()
}

func (f *fakeLLMClient) CompleteStructured(_ context.Context, _ engine.StructuredCompletionRequest) (string, error) {
	return f.next()
}

func (f *fakeLLMClient) GenerateImage(_ context.Context, _ engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *fakeLLMClient) next() (string, error) {
	if f.calls >= len(f.responses) {
		return "", fmt.Errorf("no more responses (called %d times, have %d)", f.calls+1, len(f.responses))
	}
	resp := string(f.responses[f.calls])
	f.calls++
	return resp, nil
}

func seedPPTXPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join("testdata", "seed", "pptx", "source.pptx")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("seed pptx not found at %s: %v", p, err)
	}
	return p
}

func TestModifyPPTX_PathB_ReplaceSlideTitle(t *testing.T) {
	src := seedPPTXPath(t)

	llmResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "replace_slide_title",
				"target":  map[string]any{"slide": 1},
				"payload": map[string]any{"new_title": "Q3 Revenue Summary"},
			},
		},
		"__needs_rewrite": []string{},
	}
	respBytes, _ := json.Marshal(llmResp)

	fake := &fakeLLMClient{responses: [][]byte{respBytes}}
	mod := New(fake)

	result, err := mod.Modify(context.Background(), Params{
		SourceFilePath: src,
		Prompt:         "Change the title of slide 1 to Q3 Revenue Summary",
	}, nil)
	if err != nil {
		t.Fatalf("Modify failed: %v", err)
	}

	if result.ResultMeta.RouterPath != "B" {
		t.Errorf("expected router_path=B, got %s", result.ResultMeta.RouterPath)
	}
	if result.ResultMeta.OpsApplied != 1 {
		t.Errorf("expected ops_applied=1, got %d", result.ResultMeta.OpsApplied)
	}
	if result.ResultMeta.OpsFailed != 0 {
		t.Errorf("expected ops_failed=0, got %d", result.ResultMeta.OpsFailed)
	}
	if result.ResultMeta.LLMCalls > 5 {
		t.Errorf("expected llm_calls <= 5, got %d", result.ResultMeta.LLMCalls)
	}
	if fake.calls != 1 {
		t.Errorf("expected exactly 1 LLM call, got %d", fake.calls)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(result.Bytes, ooxmledit.FileTypePPTX)
	if err != nil {
		t.Fatalf("extract result content: %v", err)
	}
	slide1, ok := contentXMLs["ppt/slides/slide1.xml"]
	if !ok {
		t.Fatal("slide1 not found in output")
	}
	if !strings.Contains(slide1, "Q3 Revenue Summary") {
		t.Errorf("slide 1 should contain 'Q3 Revenue Summary', got: %s", slide1[:min(len(slide1), 500)])
	}
}

func TestModifyPPTX_PathC_Sentinel(t *testing.T) {
	src := seedPPTXPath(t)

	pathBResp := map[string]any{
		"ops":             []map[string]any{},
		"__needs_rewrite": []string{"slide:1"},
	}
	pathBBytes, _ := json.Marshal(pathBResp)

	pathCResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "rewrite_entire_slide",
				"target":  map[string]any{"slide": 1},
				"payload": map[string]any{"new_slide_xml": buildMinimalSlideXML("Rewritten Title")},
			},
		},
		"__needs_rewrite": []string{},
	}
	pathCBytes, _ := json.Marshal(pathCResp)

	fake := &fakeLLMClient{responses: [][]byte{pathBBytes, pathCBytes}}
	mod := New(fake)

	result, err := mod.Modify(context.Background(), Params{
		SourceFilePath: src,
		Prompt:         "Rewrite slide 1 completely",
	}, nil)
	if err != nil {
		t.Fatalf("Modify failed: %v", err)
	}

	if result.ResultMeta.RouterPath != "C" {
		t.Errorf("expected router_path=C, got %s", result.ResultMeta.RouterPath)
	}
	if result.ResultMeta.LLMCalls != 2 {
		t.Errorf("expected llm_calls=2, got %d", result.ResultMeta.LLMCalls)
	}
	if result.ResultMeta.LLMCalls > 5 {
		t.Errorf("llm_calls must be <= 5, got %d", result.ResultMeta.LLMCalls)
	}
	if result.ResultMeta.OpsApplied != 1 {
		t.Errorf("expected ops_applied=1, got %d", result.ResultMeta.OpsApplied)
	}
}

func TestModifyPPTX_PartialFailure(t *testing.T) {
	src := seedPPTXPath(t)

	llmResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "replace_slide_title",
				"target":  map[string]any{"slide": 1},
				"payload": map[string]any{"new_title": "First Op Applied"},
			},
			{
				"op_type": "totally_invalid_op",
				"target":  map[string]any{"slide": 1},
				"payload": map[string]any{},
			},
			{
				"op_type": "replace_slide_title",
				"target":  map[string]any{"slide": 1},
				"payload": map[string]any{"new_title": "Third Op Applied"},
			},
		},
		"__needs_rewrite": []string{},
	}
	respBytes, _ := json.Marshal(llmResp)

	fake := &fakeLLMClient{responses: [][]byte{respBytes}}
	mod := New(fake)

	result, err := mod.Modify(context.Background(), Params{
		SourceFilePath: src,
		Prompt:         "Multiple ops, one bad",
	}, nil)
	if err != nil {
		t.Fatalf("Modify failed: %v", err)
	}

	if result.ResultMeta.OpsApplied != 2 {
		t.Errorf("expected ops_applied=2, got %d", result.ResultMeta.OpsApplied)
	}
	if result.ResultMeta.OpsFailed != 1 {
		t.Errorf("expected ops_failed=1, got %d", result.ResultMeta.OpsFailed)
	}
	if !result.ResultMeta.AttentionRequired {
		t.Error("expected attention_required=true")
	}

	hasPartialFailure := false
	for _, w := range result.ResultMeta.Warnings {
		if w == "partial_failure" {
			hasPartialFailure = true
			break
		}
	}
	if !hasPartialFailure {
		t.Errorf("expected 'partial_failure' in warnings, got: %v", result.ResultMeta.Warnings)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(result.Bytes, ooxmledit.FileTypePPTX)
	if err != nil {
		t.Fatalf("extract result: %v", err)
	}
	slide1 := contentXMLs["ppt/slides/slide1.xml"]
	if !strings.Contains(slide1, "Third Op Applied") {
		t.Errorf("output should reflect op #3 (Third Op Applied), got: %s", slide1[:min(len(slide1), 500)])
	}
}

func TestModifyPPTX_SentinelCap(t *testing.T) {
	src := seedPPTXPath(t)

	llmResp := map[string]any{
		"ops": []map[string]any{},
		"__needs_rewrite": []string{
			"slide:1", "slide:1", "slide:1", "slide:1", "slide:1",
		},
	}
	respBytes, _ := json.Marshal(llmResp)

	escalationResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "replace_slide_title",
				"target":  map[string]any{"slide": 1},
				"payload": map[string]any{"new_title": "Escalated"},
			},
		},
		"__needs_rewrite": []string{},
	}
	escBytes, _ := json.Marshal(escalationResp)

	fake := &fakeLLMClient{responses: [][]byte{respBytes, escBytes, escBytes, escBytes}}
	mod := New(fake)

	result, err := mod.Modify(context.Background(), Params{
		SourceFilePath: src,
		Prompt:         "Too many sentinels",
	}, nil)
	if err != nil {
		t.Fatalf("Modify failed: %v", err)
	}

	hasTruncated := false
	for _, w := range result.ResultMeta.Warnings {
		if w == "sentinel_truncated_max_targets" {
			hasTruncated = true
			break
		}
	}
	if !hasTruncated {
		t.Errorf("expected 'sentinel_truncated_max_targets' warning, got: %v", result.ResultMeta.Warnings)
	}

	if result.ResultMeta.LLMCalls > 5 {
		t.Errorf("llm_calls must be <= 5, got %d", result.ResultMeta.LLMCalls)
	}

	// 1 initial + 3 escalations (capped from 5)
	expectedCalls := 4
	if fake.calls != expectedCalls {
		t.Errorf("expected %d LLM calls, got %d", expectedCalls, fake.calls)
	}
}

func buildMinimalSlideXML(title string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <p:cSld>
    <p:spTree>
      <p:sp>
        <p:txBody>
          <a:p><a:r><a:t>%s</a:t></a:r></a:p>
        </p:txBody>
      </p:sp>
    </p:spTree>
  </p:cSld>
</p:sld>`, title)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
