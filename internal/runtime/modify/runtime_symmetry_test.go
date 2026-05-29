package modify

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/officecli/officecli/pkg/ooxmledit"
)

type hostedStyleLLMClient struct {
	*fakeLLMClient
}

type externalStyleLLMClient struct {
	*fakeLLMClient
}

func TestModifyRuntimeSymmetry(t *testing.T) {
	fixture := mustFixtureByName(t, "pptx", "replace-title-slide-2")
	responses := readMockResponses(t, filepath.Join(fixture.Dir, "mock_response.json"))

	hostedResult, err := New(&hostedStyleLLMClient{fakeLLMClient: &fakeLLMClient{responses: cloneResponses(responses)}}).Modify(
		context.Background(),
		Params{SourceFilePath: fixture.Source, Prompt: fixture.Prompt},
		nil,
	)
	if err != nil {
		t.Fatalf("hosted-style Modify failed: %v", err)
	}

	externalResult, err := New(&externalStyleLLMClient{fakeLLMClient: &fakeLLMClient{responses: cloneResponses(responses)}}).Modify(
		context.Background(),
		Params{SourceFilePath: fixture.Source, Prompt: fixture.Prompt},
		nil,
	)
	if err != nil {
		t.Fatalf("external-style Modify failed: %v", err)
	}

	hostedPacked := ooxmledit.NormalizePackedForCompare(hostedResult.Bytes)
	externalPacked := ooxmledit.NormalizePackedForCompare(externalResult.Bytes)
	if !bytes.Equal(hostedPacked, externalPacked) {
		t.Fatalf("hosted/external outputs differ after packed normalization: hosted=%d bytes external=%d bytes", len(hostedPacked), len(externalPacked))
	}
}

func mustFixtureByName(t *testing.T, format, name string) fixtureCase {
	t.Helper()
	for _, fixture := range loadFixtureCases(t) {
		if fixture.Format == format && fixture.Name == name {
			return fixture
		}
	}
	t.Fatalf("fixture %s/%s not found", format, name)
	return fixtureCase{}
}

func cloneResponses(in [][]byte) [][]byte {
	out := make([][]byte, 0, len(in))
	for _, resp := range in {
		out = append(out, append([]byte(nil), resp...))
	}
	return out
}
