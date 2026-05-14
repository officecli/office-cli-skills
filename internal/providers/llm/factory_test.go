package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/officecli/officecli-internal/engine"
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
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"test\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" success\"}}]}\n\n")
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
		{Role: "user", Content: "Reply with test success only"},
	})
	if err != nil {
		t.Fatalf("CompleteText: %v", err)
	}
	if content != "test success" {
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
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"stream\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" fallback\"}}]}\n\n")
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
		{Role: "user", Content: "Reply with stream fallback only"},
	})
	if err != nil {
		t.Fatalf("CompleteText: %v", err)
	}
	if content != "stream fallback" {
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
		if r.URL.Path != "/custom/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, responseBody)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: "openai",
		BaseURL:  server.URL + "/custom",
		Model:    "gpt-test",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.CompleteJSON(context.Background(), []engine.LLMMessage{
		{Role: "user", Content: "Return JSON"},
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

func TestOpenAIClient_CompleteJSONRetriesV1WhenRootEndpointReturnsHTML(t *testing.T) {
	t.Parallel()

	var sawRoot bool
	var sawV1 bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			sawRoot = true
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><html><head><title>New API</title></head><body><div id="root"></div></body></html>`)
		case "/v1/chat/completions":
			sawV1 = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
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

	content, err := client.CompleteJSON(context.Background(), []engine.LLMMessage{
		{Role: "user", Content: "Return JSON"},
	})
	if err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if content != `{"ok":true}` {
		t.Fatalf("unexpected content: %q", content)
	}
	if !sawRoot || !sawV1 {
		t.Fatalf("expected root request then /v1 retry, sawRoot=%t sawV1=%t", sawRoot, sawV1)
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
		{Role: "user", Content: "Reply with test only"},
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
		{Role: "user", Content: "Return JSON"},
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
	client := &openAIClient{
		client: &http.Client{Timeout: timeoutFor(0)},
	}
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

	image, err := client.generateGoogleImage(context.Background(), server.URL, "google-key", "gemini-2.5-flash-image", engine.ImageGenerationRequest{
		Prompt: "Generate an illustration of a blue folder",
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

func TestOpenAIClient_GenerateImageUsesOpenAIEndpointForCompatibleGateway(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := payload["response_format"]; ok {
			t.Fatalf("compatible image request should not send legacy response_format: %#v", payload["response_format"])
		}
		if strings.Contains(r.URL.Path, ":generateContent") {
			t.Fatalf("should not route compatible gateway by model prefix: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer image-key" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, imageData)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider:     "openai",
		BaseURL:      "https://unused.example.com/v1",
		APIKey:       "openai-key",
		Model:        "gpt-test",
		ImageBaseURL: server.URL + "/v1",
		ImageAPIKey:  "image-key",
		ImageModel:   "gemini-3.1-flash-image-preview",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	image, err := client.GenerateImage(context.Background(), engine.ImageGenerationRequest{
		Prompt:            "Generate an illustration of a blue folder",
		TargetAspectRatio: 16.0 / 9.0,
	})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(image.Data) != "png-bytes" {
		t.Fatalf("unexpected image data: %q", string(image.Data))
	}
}

func TestOpenAIClient_GenerateImageFetchesURLResult(t *testing.T) {
	t.Parallel()

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/generated/image.jpeg" {
			t.Fatalf("unexpected image path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = fmt.Fprint(w, "jpeg-bytes")
	}))
	defer imageServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"url": imageServer.URL + "/generated/image.jpeg"},
			},
		})
	}))
	defer apiServer.Close()

	client, err := NewClient(Config{
		Provider:     "openai",
		BaseURL:      "https://unused.example.com/v1",
		APIKey:       "openai-key",
		Model:        "gpt-test",
		ImageBaseURL: apiServer.URL + "/v1",
		ImageAPIKey:  "image-key",
		ImageModel:   "grok-4.2-image",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	image, err := client.GenerateImage(context.Background(), engine.ImageGenerationRequest{
		Prompt: "Generate an illustration of a blue folder",
	})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(image.Data) != "jpeg-bytes" {
		t.Fatalf("unexpected image data: %q", string(image.Data))
	}
	if image.MIME != "image/jpeg" {
		t.Fatalf("unexpected mime: %q", image.MIME)
	}
}

func TestOpenAIClient_GenerateImageFallsBackToResponsesImageGeneration(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	var sawImages bool
	var sawResponses bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			sawImages = true
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":{"message":"image endpoint unsupported for this model"}}`)
		case "/v1/responses":
			sawResponses = true
			_, _ = fmt.Fprintf(w, `{"output":[{"type":"image_generation_call","result":"%s"}]}`, imageData)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider:     "openai",
		BaseURL:      "https://unused.example.com/v1",
		APIKey:       "openai-key",
		Model:        "gpt-test",
		ImageBaseURL: server.URL + "/v1",
		ImageAPIKey:  "image-key",
		ImageModel:   "grok-4-image",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	image, err := client.GenerateImage(context.Background(), engine.ImageGenerationRequest{
		Prompt: "Generate an illustration of a blue folder",
	})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if !sawImages || !sawResponses {
		t.Fatalf("expected images endpoint then responses fallback, saw images=%t responses=%t", sawImages, sawResponses)
	}
	if string(image.Data) != "png-bytes" {
		t.Fatalf("unexpected image data: %q", string(image.Data))
	}
}

func TestInternalClient_GenerateImageReturnsCreditBalance(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/image" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = fmt.Fprintf(w, `{"data":"%s","mime":"image/png","credit_balance":11}`, imageData)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: "internal",
		BaseURL:  server.URL,
		APIKey:   "hosted-key",
		Model:    "hosted/image",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	image, err := client.GenerateImage(context.Background(), engine.ImageGenerationRequest{
		Prompt:            "Generate an illustration of a blue folder",
		TargetAspectRatio: 1,
	})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(image.Data) != "png-bytes" {
		t.Fatalf("unexpected image data: %q", string(image.Data))
	}
	if image.CreditBalance == nil || *image.CreditBalance != 11 {
		t.Fatalf("credit balance = %#v", image.CreditBalance)
	}
}

func TestInternalClient_CompleteSendsAnonymousAccessFields(t *testing.T) {
	t.Parallel()

	var payload struct {
		Model           string          `json:"model"`
		FingerprintHash string          `json:"fingerprint_hash"`
		AccessMode      string          `json:"access_mode"`
		CommitToken     json.RawMessage `json:"commit_token"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/json" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("anonymous request should not send Authorization, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		_, _ = fmt.Fprint(w, `{"content":"{\"ok\":true}"}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: "internal",
		BaseURL:  server.URL,
		Model:    "hosted/text",
		ImageAccess: &InternalImageAccess{
			FingerprintHash: "fp-anon",
			AccessMode:      "free",
			CommitToken:     json.RawMessage(`{"request_id":"req-anon"}`),
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.CompleteJSON(context.Background(), []engine.LLMMessage{{Role: "user", Content: "Return JSON"}})
	if err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if payload.Model != "hosted/text" || payload.FingerprintHash != "fp-anon" || payload.AccessMode != "free" {
		t.Fatalf("payload = %+v", payload)
	}
	if !strings.Contains(string(payload.CommitToken), "req-anon") {
		t.Fatalf("commit token = %s", string(payload.CommitToken))
	}
}

func TestOpenAIClient_GenerateImageWithReferenceUsesImageEditMultipart(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	var uploadedImage []byte
	var filename string
	var contentType string
	var formValues map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/edits" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer image-key" {
			t.Fatalf("unexpected authorization: %s", r.Header.Get("Authorization"))
		}
		if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(8 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		formValues = map[string]string{
			"model":  r.FormValue("model"),
			"prompt": r.FormValue("prompt"),
			"size":   r.FormValue("size"),
		}
		file, header, err := r.FormFile("image[]")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer file.Close()
		filename = header.Filename
		contentType = header.Header.Get("Content-Type")
		uploadedImage, err = io.ReadAll(file)
		if err != nil {
			t.Fatalf("read uploaded image: %v", err)
		}
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, imageData)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider:     "openai",
		BaseURL:      "https://chat.example.com/v1",
		APIKey:       "text-key",
		Model:        "gpt-test",
		ImageBaseURL: server.URL,
		ImageAPIKey:  "image-key",
		ImageModel:   "gpt-image-1",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	image, err := client.GenerateImage(context.Background(), engine.ImageGenerationRequest{
		Prompt:            "Use the uploaded reference image as visual context",
		TargetAspectRatio: 16.0 / 9.0,
		ReferenceImages: []engine.ImageReference{{
			Filename: "reference.webp",
			MIME:     "image/webp",
			Data:     base64.StdEncoding.EncodeToString([]byte("reference-bytes")),
		}},
	})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if string(image.Data) != "png-bytes" {
		t.Fatalf("unexpected image data: %q", string(image.Data))
	}
	if string(uploadedImage) != "reference-bytes" {
		t.Fatalf("unexpected uploaded image: %q", string(uploadedImage))
	}
	if filename != "reference.webp" {
		t.Fatalf("filename = %q", filename)
	}
	if contentType != "image/webp" {
		t.Fatalf("content type = %q", contentType)
	}
	wantValues := map[string]string{
		"model":  "gpt-image-1",
		"prompt": "Use the uploaded reference image as visual context",
		"size":   "1536x1024",
	}
	if fmt.Sprint(formValues) != fmt.Sprint(wantValues) {
		t.Fatalf("form values = %#v", formValues)
	}
}

func TestInternalClient_GenerateImageSendsReferenceImage(t *testing.T) {
	t.Parallel()

	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	var payload struct {
		Model          string                `json:"model"`
		Prompt         string                `json:"prompt"`
		ReferenceImage engine.ImageReference `json:"reference_image"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/image" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		_, _ = fmt.Fprintf(w, `{"data":"%s","mime":"image/png","credit_balance":11}`, imageData)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Provider: "internal",
		BaseURL:  server.URL,
		APIKey:   "hosted-key",
		Model:    "hosted/image",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.GenerateImage(context.Background(), engine.ImageGenerationRequest{
		Prompt: "Use the uploaded reference image as visual context",
		ReferenceImages: []engine.ImageReference{{
			MIME: "image/png",
			Data: "cmVmZXJlbmNlLWJ5dGVz",
		}},
	})
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if payload.ReferenceImage.MIME != "image/png" || payload.ReferenceImage.Data != "cmVmZXJlbmNlLWJ5dGVz" {
		t.Fatalf("reference image = %#v", payload.ReferenceImage)
	}
}
