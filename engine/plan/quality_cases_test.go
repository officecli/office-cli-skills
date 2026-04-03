package plan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type qualityCase struct {
	ID             string   `json:"id"`
	DocumentType   string   `json:"document_type"`
	GenerationMode string   `json:"generation_mode"`
	Prompt         string   `json:"prompt"`
	Dimensions     []string `json:"dimensions"`
}

func TestQualityCasesFixture_CoversThreeDocumentTypes(t *testing.T) {
	path := filepath.Join("testdata", "quality_cases.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var cases []qualityCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("unmarshal quality cases: %v", err)
	}
	if len(cases) < 7 {
		t.Fatalf("case count = %d, want at least 7", len(cases))
	}
	covered := map[string]int{}
	for _, item := range cases {
		if item.ID == "" || item.Prompt == "" {
			t.Fatalf("invalid case: %+v", item)
		}
		if item.GenerationMode != "best" {
			t.Fatalf("case %s generation mode = %q, want best", item.ID, item.GenerationMode)
		}
		if len(item.Dimensions) != 4 {
			t.Fatalf("case %s dimensions = %d, want 4", item.ID, len(item.Dimensions))
		}
		covered[item.DocumentType]++
	}
	for _, documentType := range []string{"pptx", "docx", "xlsx"} {
		if covered[documentType] == 0 {
			t.Fatalf("document type %s not covered", documentType)
		}
	}
}
