package modify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

type untouchedExpectedMeta struct {
	MaskedRegions []maskedRegion `json:"masked_regions"`
}

type maskedRegion struct {
	Entry  string `json:"entry"`
	Reason string `json:"reason"`
}

func TestModifyUntouchedDiff(t *testing.T) {
	fixtures := loadFixtureCases(t)
	checkedByFormat := map[string]int{}
	total := 0

	for _, fixture := range fixtures {
		masked := readFixtureMaskedRegions(t, filepath.Join(fixture.Dir, "expected_meta.json"))
		if len(masked) == 0 {
			continue
		}
		total++
		checkedByFormat[fixture.Format]++

		t.Run(fixture.Format+"/"+fixture.Name, func(t *testing.T) {
			srcBytes, err := os.ReadFile(fixture.Source)
			if err != nil {
				t.Fatalf("read source: %v", err)
			}

			responses := readMockResponses(t, filepath.Join(fixture.Dir, "mock_response.json"))
			mod := New(&fakeLLMClient{responses: responses})
			got, err := mod.Modify(context.Background(), Params{
				SourceFilePath: fixture.Source,
				Prompt:         fixture.Prompt,
			}, nil)
			if err != nil {
				t.Fatalf("Modify failed: %v", err)
			}

			beforeXMLs := normalizedContentXMLs(t, srcBytes, fixture.FileType)
			afterXMLs := normalizedContentXMLs(t, got.Bytes, fixture.FileType)

			maskedEntries := make([]string, 0, len(masked))
			for _, region := range masked {
				maskedEntries = append(maskedEntries, region.Entry)
			}

			for entry, beforeXML := range beforeXMLs {
				if slices.Contains(maskedEntries, entry) {
					continue
				}
				afterXML, ok := afterXMLs[entry]
				if !ok {
					t.Fatalf("unmasked entry %s missing after modify", entry)
				}
				if beforeXML != afterXML {
					t.Fatalf("unmasked entry %s changed\nbefore: %s\nafter:  %s", entry, beforeXML, afterXML)
				}
			}
		})
	}

	if total < 6 {
		t.Fatalf("expected at least 6 fixtures with masked_regions, got %d", total)
	}
	for _, format := range []string{"pptx", "docx", "xlsx"} {
		if checkedByFormat[format] < 2 {
			t.Fatalf("expected at least 2 masked fixtures for %s, got %d", format, checkedByFormat[format])
		}
	}
}

func readFixtureMaskedRegions(t *testing.T, path string) []maskedRegion {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var meta untouchedExpectedMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return meta.MaskedRegions
}

func normalizedContentXMLs(t *testing.T, archive []byte, fileType ooxmledit.FileType) map[string]string {
	t.Helper()

	normalizedArchive := ooxmledit.NormalizePackedForCompare(archive)
	contentXMLs, err := ooxmledit.ExtractContentXML(normalizedArchive, fileType)
	if err != nil {
		t.Fatalf("extract normalized content XML: %v", err)
	}
	out := make(map[string]string, len(contentXMLs))
	for entry, xml := range contentXMLs {
		out[entry] = string(ooxmledit.NormalizeXMLForDiff([]byte(xml)))
	}
	return out
}
