package modify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli-internal/engine/nonppt"
	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

type fixtureCase struct {
	Name     string
	Format   string
	Dir      string
	Source   string
	FileType ooxmledit.FileType
	Prompt   string
	Meta     *fixtureExpectedMeta
}

type fixtureExpectedMeta struct {
	RouterPath string `json:"router_path"`
	OpsApplied *int   `json:"ops_applied"`
	OpsFailed  *int   `json:"ops_failed"`
}

type oracleAssertion struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Entry string `json:"entry"`
}

func TestModifyFixtures(t *testing.T) {
	result := runModifyFixtures(t, "")
	if result.Total < 13 {
		t.Fatalf("expected at least 13 modify fixtures, got %d", result.Total)
	}
}

func TestModifyPathBFixtures(t *testing.T) {
	result := runModifyFixtures(t, "B")
	if result.Total < 9 {
		t.Fatalf("expected at least 9 Path-B fixtures, got %d", result.Total)
	}
}

func TestModifyPathCFixtures(t *testing.T) {
	result := runModifyFixtures(t, "C")
	if result.Total < 6 {
		t.Fatalf("expected at least 6 Path-C fixtures, got %d", result.Total)
	}
	for _, format := range []string{"pptx", "docx", "xlsx"} {
		if result.ByFormat[format] < 2 {
			t.Fatalf("expected at least 2 Path-C fixtures for %s, got %d", format, result.ByFormat[format])
		}
	}
}

type fixtureRunResult struct {
	Total    int
	ByFormat map[string]int
}

func runModifyFixtures(t *testing.T, routerPathFilter string) fixtureRunResult {
	t.Helper()

	fixtures := loadFixtureCases(t)
	result := fixtureRunResult{ByFormat: map[string]int{}}

	for _, fixture := range fixtures {
		if routerPathFilter != "" {
			if fixture.Meta == nil || fixture.Meta.RouterPath != routerPathFilter {
				continue
			}
		}

		result.Total++
		result.ByFormat[fixture.Format]++

		t.Run(fixture.Format+"/"+fixture.Name, func(t *testing.T) {
			responses := readMockResponses(t, filepath.Join(fixture.Dir, "mock_response.json"))
			mod := New(&fakeLLMClient{responses: responses})

			got, err := mod.Modify(context.Background(), Params{
				SourceFilePath: fixture.Source,
				Prompt:         fixture.Prompt,
			}, nil)
			if err != nil {
				t.Fatalf("Modify failed: %v", err)
			}

			assertExpectedMeta(t, fixture, got.ResultMeta)
			assertOracle(t, fixture, got.Bytes)
		})
	}

	return result
}

func loadFixtureCases(t *testing.T) []fixtureCase {
	t.Helper()

	var fixtures []fixtureCase
	for _, format := range []string{"pptx", "docx", "xlsx"} {
		root := filepath.Join("testdata", format)
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read fixture root %s: %v", root, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(root, entry.Name())
			source := filepath.Join(dir, "source."+format)
			if _, err := os.Stat(source); err != nil {
				t.Fatalf("fixture %s missing source.%s: %v", dir, format, err)
			}

			promptBytes, err := os.ReadFile(filepath.Join(dir, "prompt.txt"))
			if err != nil {
				t.Fatalf("fixture %s missing prompt.txt: %v", dir, err)
			}

			fixtures = append(fixtures, fixtureCase{
				Name:     entry.Name(),
				Format:   format,
				Dir:      dir,
				Source:   source,
				FileType: fileTypeForFormat(t, format),
				Prompt:   strings.TrimSpace(string(promptBytes)),
				Meta:     readExpectedMeta(t, filepath.Join(dir, "expected_meta.json")),
			})
		}
	}
	return fixtures
}

func fileTypeForFormat(t *testing.T, format string) ooxmledit.FileType {
	t.Helper()
	switch format {
	case "pptx":
		return ooxmledit.FileTypePPTX
	case "docx":
		return ooxmledit.FileTypeDOCX
	case "xlsx":
		return ooxmledit.FileTypeXLSX
	default:
		t.Fatalf("unsupported fixture format %q", format)
		return ""
	}
}

func readExpectedMeta(t *testing.T, path string) *fixtureExpectedMeta {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var meta fixtureExpectedMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return &meta
}

func readMockResponses(t *testing.T, path string) [][]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		t.Fatalf("parse %s as response array: %v", path, err)
	}
	if len(raws) == 0 {
		t.Fatalf("%s must contain at least one response", path)
	}
	responses := make([][]byte, 0, len(raws))
	for _, raw := range raws {
		responses = append(responses, []byte(raw))
	}
	return responses
}

func assertOracle(t *testing.T, fixture fixtureCase, output []byte) {
	t.Helper()
	path := filepath.Join(fixture.Dir, "oracle.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var assertions []oracleAssertion
	if err := json.Unmarshal(data, &assertions); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(assertions) == 0 {
		t.Fatalf("%s must contain at least one assertion", path)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(output, fixture.FileType)
	if err != nil {
		t.Fatalf("extract output content XML: %v", err)
	}

	for _, assertion := range assertions {
		switch assertion.Kind {
		case "contains_bytes":
			if !strings.Contains(string(output), assertion.Value) {
				t.Fatalf("output bytes do not contain %q", assertion.Value)
			}
		case "contains_text":
			if !contentContains(fixture.FileType, contentXMLs, assertion.Value) {
				t.Fatalf("content XML does not contain %q", assertion.Value)
			}
		case "contains_entry":
			xml, ok := contentXMLs[assertion.Entry]
			if !ok {
				t.Fatalf("content XML entry %q not found", assertion.Entry)
			}
			if !strings.Contains(xml, assertion.Value) {
				t.Fatalf("content XML entry %q does not contain %q", assertion.Entry, assertion.Value)
			}
		default:
			t.Fatalf("unsupported oracle kind %q", assertion.Kind)
		}
	}
}

func contentContains(fileType ooxmledit.FileType, contentXMLs map[string]string, value string) bool {
	for _, xml := range contentXMLs {
		if strings.Contains(xml, value) {
			return true
		}
	}
	if fileType == ooxmledit.FileTypeXLSX {
		for _, rows := range nonppt.XLSXRowsFromContent(contentXMLs) {
			for _, row := range rows {
				for _, cell := range row {
					if strings.Contains(cell, value) {
						return true
					}
				}
			}
		}
	}
	return false
}

func assertExpectedMeta(t *testing.T, fixture fixtureCase, got ResultMeta) {
	t.Helper()
	if fixture.Meta == nil {
		return
	}
	if fixture.Meta.RouterPath != "" && got.RouterPath != fixture.Meta.RouterPath {
		t.Fatalf("router_path: expected %s, got %s", fixture.Meta.RouterPath, got.RouterPath)
	}
	if fixture.Meta.OpsApplied != nil && got.OpsApplied != *fixture.Meta.OpsApplied {
		t.Fatalf("ops_applied: expected %d, got %d", *fixture.Meta.OpsApplied, got.OpsApplied)
	}
	if fixture.Meta.OpsFailed != nil && got.OpsFailed != *fixture.Meta.OpsFailed {
		t.Fatalf("ops_failed: expected %d, got %d", *fixture.Meta.OpsFailed, got.OpsFailed)
	}
	if got.LLMCalls < 1 || got.LLMCalls > 5 {
		t.Fatalf("llm_calls must be in [1,5], got %d", got.LLMCalls)
	}
	if len(got.Fidelity.Dropped) > 0 {
		t.Fatalf("expected no dropped non-text OOXML entries, got %v", got.Fidelity.Dropped)
	}
	if got.OpsFailed > 0 {
		t.Fatalf("expected fixture to avoid failed ops, got %d warnings=%v", got.OpsFailed, got.Warnings)
	}
	if fixture.Meta.RouterPath == "C" && got.LLMCalls < 2 {
		t.Fatalf("Path-C fixture should use at least 2 LLM calls, got %d", got.LLMCalls)
	}
}
