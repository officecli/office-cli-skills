package llm

import (
	"context"
	"encoding/base64"
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

func TestOpenAIClient_CompleteTextFallsBackToStreamingWhenContentIsEmpty(t *testing.T) {
	t.Parallel()

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requestCount++
		if requestCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":""}}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"流式\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"补救\"}}]}\n\n")
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
		{Role: "user", Content: "只回复流式补救"},
	})
	if err != nil {
		t.Fatalf("CompleteText: %v", err)
	}
	if content != "流式补救" {
		t.Fatalf("unexpected content: %q", content)
	}
	if requestCount != 2 {
		t.Fatalf("expected 2 requests, got %d", requestCount)
	}
}

func TestOpenAIClient_CompleteJSONReturnsLLMRequestFailedForNonJSONBody(t *testing.T) {
	t.Parallel()

	const responseBody = "<html><body>upstream unavailable</body></html>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, responseBody)
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

	_, err = client.CompleteJSON(context.Background(), []engine.LLMMessage{
		{Role: "user", Content: "返回 JSON"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "llm request failed: invalid json response") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), responseBody) {
		t.Fatalf("expected full response body in error, got: %v", err)
	}
}

func TestOpenAIClient_CompleteTextReturnsLLMRequestFailedForInvalidStreamingPayload(t *testing.T) {
	t.Parallel()

	const streamPayload = "<html><body>bad gateway</body></html>"
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
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", streamPayload)
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

	_, err = client.CompleteText(context.Background(), []engine.LLMMessage{
		{Role: "user", Content: "只回复测试"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "llm request failed: invalid streaming response") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), streamPayload) {
		t.Fatalf("expected full stream payload in error, got: %v", err)
	}
}

func TestInternalClient_CompleteJSONReturnsLLMRequestFailedForNonJSONBody(t *testing.T) {
	t.Parallel()

	const responseBody = "<xml><error>proxy failure</error></xml>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/json" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprint(w, responseBody)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: "internal",
		BaseURL:  server.URL,
		Model:    "internal-model",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.CompleteJSON(context.Background(), []engine.LLMMessage{
		{Role: "user", Content: "返回 JSON"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "llm request failed: invalid json response") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), responseBody) {
		t.Fatalf("expected full response body in error, got: %v", err)
	}
}

func TestOpenAIClient_GenerateImageSupportsGoogleEndpoint(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-2.5-flash-image:generateContent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "google-key" {
			t.Fatalf("x-goog-api-key = %q", got)
		}
		_, _ = fmt.Fprintf(w, `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"%s"}}]}}]}`, imageData)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider:     "openai",
		BaseURL:      "https://unused.example.com/v1",
		APIKey:       "openai-key",
		Model:        "gpt-test",
		ImageBaseURL: server.URL,
		ImageAPIKey:  "google-key",
		ImageModel:   "gemini-2.5-flash-image",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	image, err := client.GenerateImage(context.Background(), engine.ImageGenerationRequest{
		Prompt: "生成一张蓝色文件夹插图",
	})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(image.Data) != "png-bytes" {
		t.Fatalf("unexpected image data: %q", string(image.Data))
	}
	if image.MIME != "image/png" {
		t.Fatalf("unexpected mime: %q", image.MIME)
	}
}
