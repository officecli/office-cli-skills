package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
)

func TestNewProvider_OpenAICompatible(t *testing.T) {
	cfg := Config{
		Provider:   "openai",
		BaseURL:    "https://api.example.com/v1",
		APIKey:     "token",
		Model:      "gpt-test",
		ImageModel: "gpt-image-1",
	}

	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider")
	}
}

func TestNewProvider_InternalHTTP(t *testing.T) {
	cfg := Config{
		Provider: "internal",
		BaseURL:  "https://internal.example.com",
		APIKey:   "token",
		Model:    "internal-model",
	}

	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider")
	}
}

func TestOpenAIClient_CompleteTextFallsBackToStreaming(t *testing.T) {
	t.Parallel()

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requestCount++
		if requestCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"message":"Stream must be set to true"}}`)
			return
		}
		if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			t.Fatalf("expected json request, got %q", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"测试\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"成功\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: "openai",
		BaseURL:  server.URL,
		Model:    "gpt-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	content, err := client.CompleteText(context.Background(), []engine.LLMMessage{
		{Role: "user", Content: "只回复测试成功"},
	})
	if err != nil {
		t.Fatalf("CompleteText: %v", err)
	}
	if content != "测试成功" {
		t.Fatalf("unexpected content: %q", content)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 requests, got %d", requestCount)
	}
}
