package cli

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/officecli/officecli/engine"
)

const maxReferenceImageBytes = 10 << 20

func resolveReferenceImage(ctx context.Context, source string) (engine.ImageReference, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return engine.ImageReference{}, nil
	}
	if isHTTPReferenceImage(source) {
		return fetchReferenceImage(ctx, source)
	}
	return readReferenceImageFile(source)
}

func resolveReferenceImages(ctx context.Context, sources []string) ([]engine.ImageReference, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	out := make([]engine.ImageReference, 0, len(sources))
	for _, src := range sources {
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}
		ref, err := resolveReferenceImage(ctx, src)
		if err != nil {
			return nil, fmt.Errorf("reference image %q: %w", src, err)
		}
		if strings.TrimSpace(ref.Data) == "" {
			continue
		}
		out = append(out, ref)
	}
	return out, nil
}

func isHTTPReferenceImage(source string) bool {
	parsed, err := url.Parse(source)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func readReferenceImageFile(path string) (engine.ImageReference, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return engine.ImageReference{}, fmt.Errorf("read reference image: %w", err)
	}
	return buildReferenceImage(filepath.Base(path), data, "")
}

func fetchReferenceImage(ctx context.Context, rawURL string) (engine.ImageReference, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return engine.ImageReference{}, fmt.Errorf("build reference image request: %w", err)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return engine.ImageReference{}, fmt.Errorf("download reference image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return engine.ImageReference{}, fmt.Errorf("download reference image failed: status=%d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReferenceImageBytes+1))
	if err != nil {
		return engine.ImageReference{}, fmt.Errorf("download reference image: %w", err)
	}
	filename := "reference"
	if parsed, err := url.Parse(rawURL); err == nil {
		if base := filepath.Base(parsed.Path); base != "." && base != "/" && base != "" {
			filename = base
		}
	}
	return buildReferenceImage(filename, data, resp.Header.Get("Content-Type"))
}

func buildReferenceImage(filename string, data []byte, contentType string) (engine.ImageReference, error) {
	if len(data) == 0 {
		return engine.ImageReference{}, fmt.Errorf("reference image is empty")
	}
	if len(data) > maxReferenceImageBytes {
		return engine.ImageReference{}, fmt.Errorf("reference image exceeds %d bytes", maxReferenceImageBytes)
	}
	mime := normalizeReferenceImageMIME(contentType)
	if mime == "" {
		mime = http.DetectContentType(data)
		mime = normalizeReferenceImageMIME(mime)
	}
	if mime == "" {
		return engine.ImageReference{}, fmt.Errorf("unsupported reference image type")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" || filename == "." || filename == "/" {
		filename = "reference" + extensionForReferenceMIME(mime)
	}
	if filepath.Ext(filename) == "" {
		filename += extensionForReferenceMIME(mime)
	}
	return engine.ImageReference{
		Filename: filepath.Base(filename),
		MIME:     mime,
		Data:     base64.StdEncoding.EncodeToString(data),
	}, nil
}

func normalizeReferenceImageMIME(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.Split(value, ";")[0]))
	switch value {
	case "image/png", "image/jpeg", "image/webp":
		return value
	default:
		return ""
	}
}

func extensionForReferenceMIME(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
