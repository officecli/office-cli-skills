package modify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli/pkg/ooxmledit"
)

func seedDOCXPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join("testdata", "seed", "docx", "source.docx")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("seed docx not found at %s: %v", p, err)
	}
	return p
}

func TestModifyDOCX_PathB_ReplaceParagraph(t *testing.T) {
	src := seedDOCXPath(t)

	llmResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "replace_docx_paragraph",
				"target":  map[string]any{"paragraph": 1},
				"payload": map[string]any{"new_text": "Updated first paragraph"},
			},
		},
		"__needs_rewrite": []string{},
	}
	respBytes, _ := json.Marshal(llmResp)

	fake := &fakeLLMClient{responses: [][]byte{respBytes}}
	mod := New(fake)

	result, err := mod.Modify(context.Background(), Params{
		SourceFilePath: src,
		Prompt:         "Replace the first paragraph",
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

	contentXMLs, err := ooxmledit.ExtractContentXML(result.Bytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("extract result content: %v", err)
	}
	docXML, ok := contentXMLs["word/document.xml"]
	if !ok {
		t.Fatal("word/document.xml not found in output")
	}
	if !strings.Contains(docXML, "Updated first paragraph") {
		t.Errorf("document should contain 'Updated first paragraph', got: %s", docXML[:min(len(docXML), 500)])
	}
}

func TestModifyDOCX_PathC_Sentinel(t *testing.T) {
	src := seedDOCXPath(t)

	pathBResp := map[string]any{
		"ops":             []map[string]any{},
		"__needs_rewrite": []string{"docx:section:intro"},
	}
	pathBBytes, _ := json.Marshal(pathBResp)

	pathCResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "rewrite_docx_document",
				"target":  map[string]any{"paragraph": 0},
				"payload": map[string]any{"paragraphs": []string{"Rewritten intro paragraph", "Second paragraph of rewrite"}},
			},
		},
		"__needs_rewrite": []string{},
	}
	pathCBytes, _ := json.Marshal(pathCResp)

	fake := &fakeLLMClient{responses: [][]byte{pathBBytes, pathCBytes}}
	mod := New(fake)

	result, err := mod.Modify(context.Background(), Params{
		SourceFilePath: src,
		Prompt:         "Rewrite the intro section",
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

func TestModifyDOCX_PartialFailure(t *testing.T) {
	src := seedDOCXPath(t)

	llmResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "replace_docx_paragraph",
				"target":  map[string]any{"paragraph": 1},
				"payload": map[string]any{"new_text": "First Op Applied"},
			},
			{
				"op_type": "totally_invalid_op",
				"target":  map[string]any{"paragraph": 1},
				"payload": map[string]any{},
			},
			{
				"op_type": "replace_docx_paragraph",
				"target":  map[string]any{"paragraph": 1},
				"payload": map[string]any{"new_text": "Third Op Applied"},
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

	contentXMLs, err := ooxmledit.ExtractContentXML(result.Bytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("extract result: %v", err)
	}
	docXML := contentXMLs["word/document.xml"]
	if !strings.Contains(docXML, "Third Op Applied") {
		t.Errorf("output should reflect op #3 (Third Op Applied), got: %s", docXML[:min(len(docXML), 500)])
	}
}

func TestModifyDOCX_SentinelCap(t *testing.T) {
	src := seedDOCXPath(t)

	llmResp := map[string]any{
		"ops": []map[string]any{},
		"__needs_rewrite": []string{
			"docx:section:intro", "docx:section:body", "docx:section:conclusion",
			"docx:section:appendix", "docx:section:references",
		},
	}
	respBytes, _ := json.Marshal(llmResp)

	escalationResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "rewrite_docx_document",
				"target":  map[string]any{"paragraph": 0},
				"payload": map[string]any{"paragraphs": []string{"Escalated paragraph"}},
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
