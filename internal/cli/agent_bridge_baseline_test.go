package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataBaselinePath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata", "baseline", name)
}

func TestAgentBridgeOfficeRenderUnchanged(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	result := server.initializeResult(context.Background())

	tools := result.Tools
	var renderSchema map[string]any
	for _, tool := range tools {
		if name, ok := tool["name"].(string); ok && name == "office.render" {
			renderSchema = tool
			break
		}
	}
	if renderSchema == nil {
		t.Fatal("office.render tool not found in initialize result")
	}

	assertGoldenJSON(t, "office_render.json", renderSchema)
}

func TestAgentBridgeOfficeGenerateUnchanged(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	result := server.initializeResult(context.Background())

	tools := result.Tools
	var generateSchema map[string]any
	for _, tool := range tools {
		if name, ok := tool["name"].(string); ok && name == "office.generate" {
			generateSchema = tool
			break
		}
	}
	if generateSchema == nil {
		t.Fatal("office.generate tool not found in initialize result")
	}

	assertGoldenJSON(t, "office_generate.json", generateSchema)
}

func TestAgentBridgeOfficePrepareUnchanged(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	result := server.initializeResult(context.Background())

	tools := result.Tools
	var prepareSchema map[string]any
	for _, tool := range tools {
		if name, ok := tool["name"].(string); ok && name == "office.prepare" {
			prepareSchema = tool
			break
		}
	}
	if prepareSchema == nil {
		t.Fatal("office.prepare tool not found in initialize result")
	}

	assertGoldenJSON(t, "office_prepare.json", prepareSchema)
}

func TestRunNewUnchanged(t *testing.T) {
	server := newAgentBridgeServer(NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil)), Config{}, bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	result := server.initializeResult(context.Background())

	caps, ok := result.Capabilities["document_generation"].(map[string]any)
	if !ok {
		t.Fatal("document_generation capability missing")
	}

	assertGoldenJSON(t, "run_new.json", caps)
}

func assertGoldenJSON(t *testing.T, filename string, actual any) {
	t.Helper()

	actualJSON, err := json.MarshalIndent(actual, "", "  ")
	if err != nil {
		t.Fatalf("marshal actual: %v", err)
	}

	goldenPath := testdataBaselinePath(filename)

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(goldenPath, actualJSON, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v\nRun with UPDATE_GOLDEN=1 to create", goldenPath, err)
	}

	var expectedNorm, actualNorm any
	if err := json.Unmarshal(expected, &expectedNorm); err != nil {
		t.Fatalf("parse golden JSON: %v", err)
	}
	if err := json.Unmarshal(actualJSON, &actualNorm); err != nil {
		t.Fatalf("parse actual JSON: %v", err)
	}

	expectedBytes, _ := json.MarshalIndent(expectedNorm, "", "  ")
	actualBytes, _ := json.MarshalIndent(actualNorm, "", "  ")

	if string(expectedBytes) != string(actualBytes) {
		t.Errorf("golden mismatch for %s\n--- expected ---\n%s\n--- actual ---\n%s", filename, string(expectedBytes), string(actualBytes))
	}
}
