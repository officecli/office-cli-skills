package modify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

func seedXLSXPath(t *testing.T) string {
	t.Helper()
	p := filepath.Join("testdata", "seed", "xlsx", "source.xlsx")
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("seed xlsx not found at %s: %v", p, err)
	}
	return p
}

func TestModifyXLSX_PathB_UpdateCells(t *testing.T) {
	src := seedXLSXPath(t)

	llmResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "update_xlsx_cells",
				"target":  map[string]any{"sheet": "Sheet1", "worksheet_index": 1},
				"payload": map[string]any{
					"cell_updates": []map[string]any{
						{"cell": "B2", "value": "150"},
					},
				},
			},
		},
		"__needs_rewrite": []string{},
	}
	respBytes, _ := json.Marshal(llmResp)

	fake := &fakeLLMClient{responses: [][]byte{respBytes}}
	mod := New(fake)

	result, err := mod.Modify(context.Background(), Params{
		SourceFilePath: src,
		Prompt:         "Update cell B2 to 150",
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

	contentXMLs, err := ooxmledit.ExtractContentXML(result.Bytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		t.Fatalf("extract result content: %v", err)
	}
	found := false
	for _, xml := range contentXMLs {
		if strings.Contains(xml, "150") {
			found = true
			break
		}
	}
	if !found {
		t.Error("output should contain cell value '150' after update")
	}
}

func TestModifyXLSX_PathC_Sentinel(t *testing.T) {
	src := seedXLSXPath(t)

	pathBResp := map[string]any{
		"ops":             []map[string]any{},
		"__needs_rewrite": []string{"sheet:Sheet1"},
	}
	pathBBytes, _ := json.Marshal(pathBResp)

	pathCResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "rewrite_xlsx_sheet",
				"target":  map[string]any{"sheet": "Sheet1", "worksheet_index": 1},
				"payload": map[string]any{
					"rows": [][]string{
						{"Name", "Score"},
						{"Alice", "100"},
						{"Bob", "200"},
					},
				},
			},
		},
		"__needs_rewrite": []string{},
	}
	pathCBytes, _ := json.Marshal(pathCResp)

	fake := &fakeLLMClient{responses: [][]byte{pathBBytes, pathCBytes}}
	mod := New(fake)

	result, err := mod.Modify(context.Background(), Params{
		SourceFilePath: src,
		Prompt:         "Rewrite Sheet1 completely",
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

func TestModifyXLSX_PartialFailure(t *testing.T) {
	src := seedXLSXPath(t)

	llmResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "update_xlsx_cells",
				"target":  map[string]any{"sheet": "Sheet1", "worksheet_index": 1},
				"payload": map[string]any{
					"cell_updates": []map[string]any{
						{"cell": "A1", "value": "First"},
					},
				},
			},
			{
				"op_type": "totally_invalid_op",
				"target":  map[string]any{"sheet": "Sheet1", "worksheet_index": 1},
				"payload": map[string]any{},
			},
			{
				"op_type": "update_xlsx_cells",
				"target":  map[string]any{"sheet": "Sheet1", "worksheet_index": 1},
				"payload": map[string]any{
					"cell_updates": []map[string]any{
						{"cell": "A2", "value": "Third"},
					},
				},
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
}

func TestModifyXLSX_SentinelCap(t *testing.T) {
	src := seedXLSXPath(t)

	llmResp := map[string]any{
		"ops": []map[string]any{},
		"__needs_rewrite": []string{
			"sheet:Sheet1", "sheet:Sheet1", "sheet:Sheet1", "sheet:Sheet1", "sheet:Sheet1",
		},
	}
	respBytes, _ := json.Marshal(llmResp)

	escalationResp := map[string]any{
		"ops": []map[string]any{
			{
				"op_type": "update_xlsx_cells",
				"target":  map[string]any{"sheet": "Sheet1", "worksheet_index": 1},
				"payload": map[string]any{
					"cell_updates": []map[string]any{
						{"cell": "A1", "value": "Escalated"},
					},
				},
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
