package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

type modifyFakeLLMClient struct {
	responses [][]byte
	calls     int
}

func (f *modifyFakeLLMClient) CompleteText(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return f.next()
}

func (f *modifyFakeLLMClient) CompleteJSON(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return f.next()
}

func (f *modifyFakeLLMClient) CompleteStructured(_ context.Context, _ engine.StructuredCompletionRequest) (string, error) {
	return f.next()
}

func (f *modifyFakeLLMClient) GenerateImage(_ context.Context, _ engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *modifyFakeLLMClient) next() (string, error) {
	if f.calls >= len(f.responses) {
		return "", fmt.Errorf("no more responses (called %d times, have %d)", f.calls+1, len(f.responses))
	}
	resp := string(f.responses[f.calls])
	f.calls++
	return resp, nil
}

func seedPath(t *testing.T, docType string) string {
	t.Helper()
	ext := docType
	p := filepath.Join("../runtime/modify/testdata/seed", docType, "source."+ext)
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("seed %s not found at %s: %v", docType, abs, err)
	}
	return abs
}

func newModifyTestApp(fake *modifyFakeLLMClient) *App {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	app.newLLMClient = func(cfg LLMConfig) (GeneratorLLMClient, error) {
		return fake, nil
	}
	app.officeTaskPreflight = func(ctx context.Context, command string, args []string) error {
		return nil
	}
	return app
}

func testLLMConfig() Config {
	return Config{
		LLM: LLMConfig{
			BaseURL: "https://api.example.com/v1",
			APIKey:  "test-key",
			Model:   "gpt-4.1",
		},
	}
}

func TestModifyRunnerPPTX(t *testing.T) {
	src := seedPath(t, "pptx")

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

	fake := &modifyFakeLLMClient{responses: [][]byte{respBytes}}
	app := newModifyTestApp(fake)

	outDir := t.TempDir()
	result, err := app.executeModifyJob(context.Background(), testLLMConfig(), ModifyJob{
		SourceFilePath: src,
		DocumentType:   engine.DocumentTypePPTX,
		Prompt:         "Change the title of slide 1 to Q3 Revenue Summary",
		OutputDir:      outDir,
		Mode:           "fast",
	}, noopProgressController{})
	if err != nil {
		t.Fatalf("executeModifyJob: %v", err)
	}

	if !strings.HasSuffix(result.FilePath, ".modified.pptx") {
		t.Errorf("expected output ending in .modified.pptx, got %s", result.FilePath)
	}
	if result.DocumentType != "pptx" {
		t.Errorf("document_type = %q, want pptx", result.DocumentType)
	}
	if result.OpsApplied != 1 {
		t.Errorf("ops_applied = %d, want 1", result.OpsApplied)
	}
	if result.OpsFailed != 0 {
		t.Errorf("ops_failed = %d, want 0", result.OpsFailed)
	}
	data, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("output file is empty")
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(data, ooxmledit.FileTypePPTX)
	if err != nil {
		t.Fatalf("extract pptx content: %v", err)
	}
	slideXML := contentXMLs["ppt/slides/slide1.xml"]
	if !strings.Contains(slideXML, "Q3 Revenue Summary") {
		t.Fatalf("modified PPTX should contain updated slide title, got: %s", slideXML[:min(len(slideXML), 500)])
	}
}

func TestModifyRunnerDOCX(t *testing.T) {
	src := seedPath(t, "docx")

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

	fake := &modifyFakeLLMClient{responses: [][]byte{respBytes}}
	app := newModifyTestApp(fake)

	outDir := t.TempDir()
	result, err := app.executeModifyJob(context.Background(), testLLMConfig(), ModifyJob{
		SourceFilePath: src,
		DocumentType:   engine.DocumentTypeDOCX,
		Prompt:         "Replace the first paragraph",
		OutputDir:      outDir,
		Mode:           "fast",
	}, noopProgressController{})
	if err != nil {
		t.Fatalf("executeModifyJob: %v", err)
	}

	if !strings.HasSuffix(result.FilePath, ".modified.docx") {
		t.Errorf("expected output ending in .modified.docx, got %s", result.FilePath)
	}
	if result.DocumentType != "docx" {
		t.Errorf("document_type = %q, want docx", result.DocumentType)
	}
	if result.OpsApplied != 1 {
		t.Errorf("ops_applied = %d, want 1", result.OpsApplied)
	}
	data, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("output file is empty")
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(data, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("extract docx content: %v", err)
	}
	documentXML := contentXMLs["word/document.xml"]
	if !strings.Contains(documentXML, "Updated first paragraph") {
		t.Fatalf("modified DOCX should contain updated paragraph, got: %s", documentXML[:min(len(documentXML), 500)])
	}
}

func TestModifyRunnerXLSX(t *testing.T) {
	src := seedPath(t, "xlsx")

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

	fake := &modifyFakeLLMClient{responses: [][]byte{respBytes}}
	app := newModifyTestApp(fake)

	outDir := t.TempDir()
	result, err := app.executeModifyJob(context.Background(), testLLMConfig(), ModifyJob{
		SourceFilePath: src,
		DocumentType:   engine.DocumentTypeXLSX,
		Prompt:         "Update cell B2 to 150",
		OutputDir:      outDir,
		Mode:           "fast",
	}, noopProgressController{})
	if err != nil {
		t.Fatalf("executeModifyJob: %v", err)
	}

	if !strings.HasSuffix(result.FilePath, ".modified.xlsx") {
		t.Errorf("expected output ending in .modified.xlsx, got %s", result.FilePath)
	}
	if result.DocumentType != "xlsx" {
		t.Errorf("document_type = %q, want xlsx", result.DocumentType)
	}
	if result.OpsApplied != 1 {
		t.Errorf("ops_applied = %d, want 1", result.OpsApplied)
	}
	data, err := os.ReadFile(result.FilePath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("output file is empty")
	}
	contentXMLs, err := ooxmledit.ExtractContentXML(data, ooxmledit.FileTypeXLSX)
	if err != nil {
		t.Fatalf("extract xlsx content: %v", err)
	}
	foundUpdatedValue := false
	for _, xml := range contentXMLs {
		if strings.Contains(xml, "150") {
			foundUpdatedValue = true
			break
		}
	}
	if !foundUpdatedValue {
		t.Fatal("modified XLSX should contain updated cell value 150")
	}
}
