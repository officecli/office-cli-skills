package cli

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const referenceTestPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+yF9sAAAAASUVORK5CYII="

func TestResolveReferenceImageReadsLocalImage(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "reference.png")
	data, err := base64.StdEncoding.DecodeString(referenceTestPNG)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write png: %v", err)
	}

	ref, err := resolveReferenceImage(t.Context(), path)
	if err != nil {
		t.Fatalf("resolveReferenceImage: %v", err)
	}
	if ref.Filename != "reference.png" || ref.MIME != "image/png" || ref.Data != referenceTestPNG {
		t.Fatalf("reference image = %#v", ref)
	}
}

func TestResolveReferenceImageDownloadsURL(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(referenceTestPNG)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	ref, err := resolveReferenceImage(t.Context(), server.URL+"/reference.png")
	if err != nil {
		t.Fatalf("resolveReferenceImage: %v", err)
	}
	if ref.Filename != "reference.png" || ref.MIME != "image/png" || ref.Data != referenceTestPNG {
		t.Fatalf("reference image = %#v", ref)
	}
}

func TestResolveReferenceImageRejectsInvalidInputs(t *testing.T) {
	tmpDir := t.TempDir()
	emptyPath := filepath.Join(tmpDir, "empty.png")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	if _, err := resolveReferenceImage(t.Context(), emptyPath); err == nil || !strings.Contains(err.Error(), "reference image is empty") {
		t.Fatalf("empty error = %v", err)
	}

	textPath := filepath.Join(tmpDir, "reference.txt")
	if err := os.WriteFile(textPath, []byte("not an image"), 0o600); err != nil {
		t.Fatalf("write text: %v", err)
	}
	if _, err := resolveReferenceImage(t.Context(), textPath); err == nil || !strings.Contains(err.Error(), "unsupported reference image type") {
		t.Fatalf("type error = %v", err)
	}
}
