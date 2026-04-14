package review

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAIReviewer_ReviewPDF(t *testing.T) {
	t.Parallel()

	var sawUpload bool
	var sawReview bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files":
			sawUpload = true
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Fatalf("ParseMultipartForm: %v", err)
			}
			if got := r.FormValue("purpose"); got != "user_data" {
				t.Fatalf("purpose = %q", got)
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("FormFile: %v", err)
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("ReadAll file: %v", err)
			}
			if !strings.Contains(string(data), "%PDF") {
				t.Fatalf("uploaded file is not pdf: %q", string(data))
			}
			_, _ = w.Write([]byte(`{"id":"file-review-1"}`))
		case "/responses":
			sawReview = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode responses payload: %v", err)
			}
			if payload["model"] != "gpt-review-test" {
				t.Fatalf("model = %v", payload["model"])
			}
			_, _ = w.Write([]byte(`{"output_text":"{\"score\":88,\"summary\":\"Overall execution is strong.\",\"strengths\":[\"Clear hierarchy\"],\"issues\":[{\"severity\":\"medium\",\"code\":\"VISUAL_ALIGNMENT\",\"title\":\"Alignment is slightly loose\",\"message\":\"Elements on slide 2 use inconsistent spacing.\",\"slide_numbers\":[2],\"suggestion\":\"Use a consistent module margin.\"}]}"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	pdfPath := filepath.Join(t.TempDir(), "deck.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reviewer := NewOpenAIReviewer(server.URL, "", "gpt-review-test", 30)
	result, err := reviewer.ReviewPDF(context.Background(), pdfPath, StructureReport{Score: 92, Summary: "Structure is solid"})
	if err != nil {
		t.Fatalf("ReviewPDF: %v", err)
	}
	if !sawUpload || !sawReview {
		t.Fatalf("expected upload and review requests, got upload=%t review=%t", sawUpload, sawReview)
	}
	if result.Score != 88 {
		t.Fatalf("score = %d", result.Score)
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != "VISUAL_ALIGNMENT" {
		t.Fatalf("unexpected issues: %+v", result.Issues)
	}
}

func TestOpenAIReviewer_UploadRequestIsMultipart(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files" {
			_, _ = w.Write([]byte(`{"output_text":"{\"score\":80,\"summary\":\"ok\",\"strengths\":[],\"issues\":[]}"}`))
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("ParseMediaType: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("media type = %s", mediaType)
		}
		_, _ = w.Write([]byte(`{"id":"file-review-2"}`))
	}))
	defer server.Close()

	pdfPath := filepath.Join(t.TempDir(), "deck.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reviewer := NewOpenAIReviewer(server.URL, "", "gpt-review-test", 30)
	if _, err := reviewer.ReviewPDF(context.Background(), pdfPath, StructureReport{}); err != nil {
		t.Fatalf("ReviewPDF: %v", err)
	}
}

func TestOpenAIReviewer_FallsBackToInlinePDFWhenFilesUnsupported(t *testing.T) {
	t.Parallel()

	var sawInline bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files":
			http.NotFound(w, r)
		case "/responses":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode responses payload: %v", err)
			}
			input := payload["input"].([]any)
			user := input[1].(map[string]any)
			content := user["content"].([]any)
			filePart := content[0].(map[string]any)
			if filePart["type"] != "input_file" {
				t.Fatalf("unexpected file part: %#v", filePart)
			}
			if !strings.HasPrefix(filePart["file_data"].(string), "data:application/pdf;base64,") {
				t.Fatalf("file_data = %v", filePart["file_data"])
			}
			sawInline = true
			_, _ = w.Write([]byte(`{"output_text":"{\"score\":86,\"summary\":\"Inline PDF is supported.\",\"strengths\":[],\"issues\":[]}"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	pdfPath := filepath.Join(t.TempDir(), "deck.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reviewer := NewOpenAIReviewer(server.URL, "", "gpt-review-test", 30)
	result, err := reviewer.ReviewPDF(context.Background(), pdfPath, StructureReport{})
	if err != nil {
		t.Fatalf("ReviewPDF: %v", err)
	}
	if !sawInline {
		t.Fatal("expected inline PDF fallback")
	}
	if result.Score != 86 {
		t.Fatalf("score = %d", result.Score)
	}
}

func TestOpenAIReviewer_FallsBackToImageChatReviewWhenResponsesHasNoText(t *testing.T) {
	t.Parallel()

	var sawResponses bool
	var sawChat bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files":
			_, _ = w.Write([]byte(`{"id":"file-review-fallback"}`))
		case "/responses":
			sawResponses = true
			_, _ = w.Write([]byte(`{"status":"completed","output":[]}`))
		case "/chat/completions":
			sawChat = true
			if !strings.Contains(r.URL.RawQuery, "") {
				// no-op, keep branch for future debugging
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"score\\\":91,\\\"summary\\\":\\\"The visual hierarchy is clear.\\\",\\\"strengths\\\":[\\\"Clear hierarchy\\\"],\\\"issues\\\":[]}\"}}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	pdfPath := filepath.Join(t.TempDir(), "deck.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n%%EOF\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reviewer := NewOpenAIReviewer(server.URL, "", "gpt-review-test", 30)
	reviewer.rasterize = func(context.Context, string, int) ([]visualPageImage, error) {
		return []visualPageImage{{Page: 1, MIME: "image/png", Data: []byte("png-bytes")}}, nil
	}

	result, err := reviewer.ReviewPDF(context.Background(), pdfPath, StructureReport{Score: 88, Summary: "Structure is solid"})
	if err != nil {
		t.Fatalf("ReviewPDF: %v", err)
	}
	if !sawResponses || !sawChat {
		t.Fatalf("expected responses + chat fallback, got responses=%t chat=%t", sawResponses, sawChat)
	}
	if result.Score != 91 {
		t.Fatalf("score = %d", result.Score)
	}
	if result.Summary != "The visual hierarchy is clear." {
		t.Fatalf("summary = %q", result.Summary)
	}
}

func TestExtractResponseText_SupportsNestedTextValue(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"output": [
			{
				"content": [
					{
						"type": "output_text",
						"text": {
							"value": "{\"score\":85}"
						}
					}
				]
			}
		]
	}`)

	text, err := extractResponseText(body)
	if err != nil {
		t.Fatalf("extractResponseText: %v", err)
	}
	if text != `{"score":85}` {
		t.Fatalf("text = %q", text)
	}
}

func TestExtractResponseText_FallsBackToChoicesMessageContent(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"choices": [
			{
				"message": {
					"content": [
						{
							"type": "output_text",
							"text": "{\"score\":91}"
						}
					]
				}
			}
		]
	}`)

	text, err := extractResponseText(body)
	if err != nil {
		t.Fatalf("extractResponseText: %v", err)
	}
	if text != `{"score":91}` {
		t.Fatalf("text = %q", text)
	}
}

func TestParseVisualResultJSON_SupportsLooseChatSchema(t *testing.T) {
	t.Parallel()

	raw := `{
		"score": 63,
		"summary": {
			"overall": "The deck is fairly complete overall, but obvious occlusion issues remain."
		},
		"strengths": [
			"Clear theme",
			"Consistent color palette"
		],
		"issues": [
			{
				"page": 5,
				"severity": "high",
				"problem": "Large images occlude the main body text."
			},
			{
				"pages": [2, 6],
				"severity": "medium",
				"message": "Text is truncated on multiple slides.",
				"suggestion": "Shorten sentences and add more whitespace."
			}
		]
	}`

	result, err := parseVisualResultJSON(raw)
	if err != nil {
		t.Fatalf("parseVisualResultJSON: %v", err)
	}
	if result.Score != 63 {
		t.Fatalf("score = %d", result.Score)
	}
	if result.Summary != "The deck is fairly complete overall, but obvious occlusion issues remain." {
		t.Fatalf("summary = %q", result.Summary)
	}
	if len(result.Issues) != 2 {
		t.Fatalf("issues = %+v", result.Issues)
	}
	if got := result.Issues[0].SlideNumbers; len(got) != 1 || got[0] != 5 {
		t.Fatalf("issue[0] slide_numbers = %+v", got)
	}
	if got := result.Issues[1].SlideNumbers; len(got) != 2 || got[0] != 2 || got[1] != 6 {
		t.Fatalf("issue[1] slide_numbers = %+v", got)
	}
}
