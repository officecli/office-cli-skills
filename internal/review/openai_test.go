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
			_, _ = w.Write([]byte(`{"output_text":"{\"score\":88,\"summary\":\"整体表现良好。\",\"strengths\":[\"层级清晰\"],\"issues\":[{\"severity\":\"medium\",\"code\":\"VISUAL_ALIGNMENT\",\"title\":\"对齐略松散\",\"message\":\"第 2 页元素间距不均。\",\"slide_numbers\":[2],\"suggestion\":\"统一模块边距。\"}]}"}`))
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
	result, err := reviewer.ReviewPDF(context.Background(), pdfPath, StructureReport{Score: 92, Summary: "结构良好"})
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
