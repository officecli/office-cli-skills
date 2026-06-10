package runtime

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	reviewprovider "github.com/officecli/officecli/internal/review"
	"github.com/officecli/officecli/internal/runtime/pptxref"
	"github.com/officecli/officecli/pkg/officegen"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

type fakeLLMClient struct {
	textResponse        string
	jsonResponse        string
	jsonResponses       []string
	jsonCallCount       int
	structuredResponse  string
	structuredResponses []string
	structuredErr       error
	structuredDelay     time.Duration
	structuredCallCount int
	lastStructuredReq   engine.StructuredCompletionRequest
	lastJSONMsgs        []engine.LLMMessage
	completionCharged   int
	completionBalance   int
	completionValid     bool
	imageResult         *engine.ImageGenerationResult
	imageResults        []*engine.ImageGenerationResult
	imageErr            error
	imageErrors         []error
	imageDelay          time.Duration
	imageCalls          int
	lastImageRequest    engine.ImageGenerationRequest
	imageRequests       []engine.ImageGenerationRequest
}

type fakePPTXArtifactPreviewReviewer struct {
	result   *PPTXArtifactPreviewReviewResult
	err      error
	calls    int
	requests []PPTXArtifactPreviewReviewRequest
}

func (f *fakePPTXArtifactPreviewReviewer) ReviewPPTXArtifactPreviews(_ context.Context, req PPTXArtifactPreviewReviewRequest) (*PPTXArtifactPreviewReviewResult, error) {
	f.calls++
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func (f fakeLLMClient) CompleteText(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return f.textResponse, nil
}

func (f *fakeLLMClient) CompleteJSON(_ context.Context, msgs []engine.LLMMessage) (string, error) {
	f.lastJSONMsgs = append([]engine.LLMMessage(nil), msgs...)
	if len(f.jsonResponses) > 0 {
		idx := f.jsonCallCount
		if idx >= len(f.jsonResponses) {
			idx = len(f.jsonResponses) - 1
		}
		f.jsonCallCount++
		return f.jsonResponses[idx], nil
	}
	return f.jsonResponse, nil
}

func (f *fakeLLMClient) CompleteStructured(ctx context.Context, req engine.StructuredCompletionRequest) (string, error) {
	f.structuredCallCount++
	f.lastStructuredReq = req
	if f.structuredDelay > 0 {
		select {
		case <-time.After(f.structuredDelay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.structuredErr != nil {
		return "", f.structuredErr
	}
	if len(f.structuredResponses) > 0 {
		idx := f.structuredCallCount - 1
		if idx >= len(f.structuredResponses) {
			idx = len(f.structuredResponses) - 1
		}
		return f.structuredResponses[idx], nil
	}
	return f.structuredResponse, nil
}

func (f *fakeLLMClient) LastCompletionCredits() (charged int, balance int, valid bool) {
	return f.completionCharged, f.completionBalance, f.completionValid
}

func (f *fakeLLMClient) GenerateImage(ctx context.Context, req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	f.imageCalls++
	f.lastImageRequest = req
	f.imageRequests = append(f.imageRequests, req)
	if f.imageDelay > 0 {
		select {
		case <-time.After(f.imageDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if len(f.imageErrors) > 0 {
		idx := f.imageCalls - 1
		if idx >= len(f.imageErrors) {
			idx = len(f.imageErrors) - 1
		}
		if f.imageErrors[idx] != nil {
			return nil, f.imageErrors[idx]
		}
	}
	if len(f.imageResults) > 0 {
		idx := f.imageCalls - 1
		if idx >= len(f.imageResults) {
			idx = len(f.imageResults) - 1
		}
		if f.imageResults[idx] != nil {
			return f.imageResults[idx], nil
		}
	}
	if f.imageErr != nil {
		return nil, f.imageErr
	}
	return f.imageResult, nil
}

type runtimeProgressCollector struct {
	events []engine.ProgressEvent
}

func containsIssueCode(items []engine.GenerateIssue, code string) bool {
	for _, item := range items {
		if item.Code == code {
			return true
		}
	}
	return false
}

func containsStringContaining(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

func generateIssueMessages(items []engine.GenerateIssue) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Message)
	}
	return out
}

const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+yF9sAAAAASUVORK5CYII="

func mustTinyPNG(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return data
}

func writePPTXArtifactFakeDiagnostics(t *testing.T, req pptxArtifactWorkerRequest, editableItems, nativeCharts int) *pptxArtifactWorkerOutput {
	t.Helper()
	if editableItems <= 0 {
		editableItems = 1
	}
	if err := os.MkdirAll(req.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview dir: %v", err)
	}
	previewFiles := make([]string, 0, len(req.Slides))
	for idx := range req.Slides {
		previewPath := filepath.Join(req.PreviewDir, fmt.Sprintf("slide-%02d.png", idx+1))
		writePPTXArtifactPreviewFixture(t, previewPath, idx)
		previewFiles = append(previewFiles, previewPath)
	}
	editable := make([]map[string]string, editableItems)
	for idx := range editable {
		editable[idx] = map[string]string{"kind": "text"}
	}
	charts := make([]map[string]string, nativeCharts)
	for idx := range charts {
		charts[idx] = map[string]string{"kind": "bar"}
	}
	var images []map[string]any
	for _, asset := range req.VisualAssets {
		if asset.Slide <= 0 {
			continue
		}
		images = append(images, map[string]any{
			"path":  asset.Path,
			"slide": asset.Slide,
			"bbox": map[string]any{
				"left":   780,
				"top":    118,
				"width":  320,
				"height": 250,
			},
		})
	}
	var visualItems []map[string]any
	if isPPTXArtifactReferenceLearningRequest(req) {
		imageSlides := map[int]bool{}
		for _, image := range images {
			if slide, ok := image["slide"].(int); ok {
				imageSlides[slide] = true
			}
		}
		for _, slide := range req.DesignPlan.Slides {
			if slide.Slide <= 0 || imageSlides[slide.Slide] {
				continue
			}
			role := ""
			switch strings.TrimSpace(slide.Role) {
			case "cover":
				role = "fallback-motif-signal-panel"
			case "closing":
				role = "closing-motif-frame"
			}
			if role == "" {
				continue
			}
			visualItems = append(visualItems, map[string]any{
				"kind":  "shape",
				"role":  role,
				"slide": slide.Slide,
				"bbox": map[string]any{
					"left":   784,
					"top":    120,
					"width":  326,
					"height": 226,
				},
			})
		}
	}
	inspect := map[string]any{
		"editableItems": editable,
		"images":        images,
		"nativeCharts":  charts,
		"previews":      previewFiles,
		"visualItems":   visualItems,
	}
	inspectBytes, err := json.Marshal(inspect)
	if err != nil {
		t.Fatalf("marshal inspect: %v", err)
	}
	if err := os.WriteFile(req.InspectPath, inspectBytes, 0o644); err != nil {
		t.Fatalf("write inspect: %v", err)
	}
	return &pptxArtifactWorkerOutput{
		OutputPPTX:     req.OutputPPTX,
		PreviewFiles:   previewFiles,
		InspectPath:    req.InspectPath,
		EditableItems:  editableItems,
		NativeCharts:   nativeCharts,
		ArtifactToolOK: true,
	}
}

func writePPTXArtifactReferenceLearningFakeDiagnostics(t *testing.T, req pptxArtifactWorkerRequest, editableItems, nativeCharts int) *pptxArtifactWorkerOutput {
	t.Helper()
	output := writePPTXArtifactFakeDiagnostics(t, req, editableItems, nativeCharts)
	data, err := os.ReadFile(req.InspectPath)
	if err != nil {
		t.Fatalf("read fake inspect: %v", err)
	}
	var inspect map[string]any
	if err := json.Unmarshal(data, &inspect); err != nil {
		t.Fatalf("unmarshal fake inspect: %v", err)
	}
	var visualItems []map[string]any
	for idx, slide := range req.Slides {
		slideNo := idx + 1
		if idx == 0 {
			visualItems = append(visualItems, map[string]any{
				"kind":  "shape",
				"role":  "fallback-motif-signal-panel",
				"slide": slideNo,
				"bbox": map[string]any{
					"left":   790,
					"top":    112,
					"width":  316,
					"height": 258,
				},
			})
		}
		if idx == len(req.Slides)-1 {
			visualItems = append(visualItems, map[string]any{
				"kind":  "shape",
				"role":  "closing-motif-frame",
				"slide": slideNo,
				"bbox": map[string]any{
					"left":   784,
					"top":    120,
					"width":  326,
					"height": 226,
				},
			})
		}
		if slide.Chart == nil {
			continue
		}
		for _, role := range []string{"chart-panel", "chart-insight-card", "chart-insight-card"} {
			visualItems = append(visualItems, map[string]any{
				"kind":  "shape",
				"role":  role,
				"slide": slideNo,
				"bbox": map[string]any{
					"left":   80,
					"top":    180,
					"width":  640,
					"height": 320,
				},
			})
		}
	}
	inspect["visualItems"] = visualItems
	next, err := json.Marshal(inspect)
	if err != nil {
		t.Fatalf("marshal fake inspect: %v", err)
	}
	if err := os.WriteFile(req.InspectPath, next, 0o644); err != nil {
		t.Fatalf("write fake inspect: %v", err)
	}
	return output
}

func writePPTXArtifactPreviewFixture(t *testing.T, path string, seed int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	palette := []color.RGBA{
		{R: 245, G: 248, B: 251, A: 255},
		{R: 23, G: 32, B: 51, A: 255},
		{R: 27, G: 166, B: 166, A: 255},
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, palette[(x/4+y/4+seed)%len(palette)])
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create preview: %v", err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode preview: %v", err)
	}
}

func writePPTXArtifactBlankPreviewFixture(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create blank preview: %v", err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode blank preview: %v", err)
	}
}

func writeLowContrastPreviewFixture(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	palette := []color.RGBA{
		{R: 228, G: 232, B: 236, A: 255},
		{R: 232, G: 236, B: 240, A: 255},
		{R: 236, G: 240, B: 244, A: 255},
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, palette[(x/4+y/4)%len(palette)])
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create low contrast preview: %v", err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode low contrast preview: %v", err)
	}
}

func readZipEntry(t *testing.T, archive []byte, name string) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", name, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read zip entry %s: %v", name, err)
		}
		return string(data)
	}
	t.Fatalf("zip entry not found: %s", name)
	return ""
}

func countZipEntries(archive []byte, prefix, suffix string) int {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return 0
	}
	count := 0
	for _, file := range reader.File {
		if prefix != "" && !strings.HasPrefix(file.Name, prefix) {
			continue
		}
		if suffix != "" && !strings.HasSuffix(file.Name, suffix) {
			continue
		}
		count++
	}
	return count
}

func countPPTXChartXMLParts(archive []byte) int {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return 0
	}
	count := 0
	for _, file := range reader.File {
		if strings.Contains(file.Name, "/charts/chart") && strings.HasSuffix(file.Name, ".xml") {
			count++
		}
	}
	return count
}

func archiveContainsEntryWithSubstring(t *testing.T, archive []byte, prefix, suffix, needle string) bool {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, file := range reader.File {
		if prefix != "" && !strings.HasPrefix(file.Name, prefix) {
			continue
		}
		if suffix != "" && !strings.HasSuffix(file.Name, suffix) {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}
		if strings.Contains(string(data), needle) {
			return true
		}
	}
	return false
}

func (c *runtimeProgressCollector) Emit(_ context.Context, event engine.ProgressEvent) {
	c.events = append(c.events, event)
}

func intPtr(value int) *int {
	return &value
}

func TestServiceGenerateDOCXWithFakeLLM(t *testing.T) {
	service := NewService(&fakeLLMClient{
		jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeDOCX,
		Prompt:       "Introduce this enterprise collaboration platform",
		Topic:        "Enterprise Collaboration Platform Overview",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["word/document.xml"], "Enterprise Collaboration Platform") {
		t.Fatalf("document xml = %q", contentXMLs["word/document.xml"])
	}
}

func TestServiceGenerateXLSXWithFakeLLM(t *testing.T) {
	service := NewService(&fakeLLMClient{
		jsonResponse: `{"title":"Sales Workbook","sheets":[{"name":"Pipeline","headers":["Region","Amount"],"rows":[["East","100"],["West","120"]]}]}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeXLSX,
		Prompt:       "Create a regional sales workbook",
		Topic:        "Sales Workbook",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if doc.DocumentName != "Sales_Workbook.xlsx" {
		t.Fatalf("document name = %q", doc.DocumentName)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["xl/workbook.xml"], "Pipeline") ||
		!strings.Contains(contentXMLs["xl/sharedStrings.xml"], "East") ||
		!strings.Contains(contentXMLs["xl/worksheets/sheet1.xml"], ">120<") {
		t.Fatalf("workbook xml = %q\nshared strings = %q\nsheet xml = %q", contentXMLs["xl/workbook.xml"], contentXMLs["xl/sharedStrings.xml"], contentXMLs["xl/worksheets/sheet1.xml"])
	}
}

func TestServiceGenerateIMGUsesImageProviderAndRatio(t *testing.T) {
	imageBytes := []byte("server-image")
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: imageBytes, MIME: "image/png", CreditBalance: intPtr(9), CreditsCharged: intPtr(3)},
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeIMG,
		Prompt:       "A polished product launch hero image",
		Topic:        "Launch Visual",
		ImageRatio:   "portrait",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if doc.DocumentType != "img" {
		t.Fatalf("document type = %q", doc.DocumentType)
	}
	if doc.DocumentName != "Launch_Visual.png" {
		t.Fatalf("document name = %q", doc.DocumentName)
	}
	if string(doc.Bytes) != string(imageBytes) {
		t.Fatalf("image bytes = %q", string(doc.Bytes))
	}
	if llm.imageCalls != 1 {
		t.Fatalf("image calls = %d", llm.imageCalls)
	}
	if llm.lastImageRequest.TargetAspectRatio != 9.0/16.0 {
		t.Fatalf("aspect ratio = %f", llm.lastImageRequest.TargetAspectRatio)
	}
	if !strings.Contains(llm.lastImageRequest.Prompt, "product launch hero") {
		t.Fatalf("prompt = %q", llm.lastImageRequest.Prompt)
	}
	if doc.HostedCreditBalance == nil || *doc.HostedCreditBalance != 9 {
		t.Fatalf("hosted credit balance = %#v", doc.HostedCreditBalance)
	}
	if doc.HostedCreditsCharged == nil || *doc.HostedCreditsCharged != 3 {
		t.Fatalf("hosted credits charged = %#v", doc.HostedCreditsCharged)
	}
}

func TestServiceGenerateGIFBuildsAnimatedArtifactAndSheetSidecar(t *testing.T) {
	sheetBytes := makeGIFSheetPNG(t, 1024, 1024)
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: sheetBytes, MIME: "image/png", CreditBalance: intPtr(9), CreditsCharged: intPtr(3)},
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeGIF,
		Prompt:       "一个女生先眨眼，然后说 Token 用完了吗，最后笑一下。",
		Topic:        "Token Reaction",
		GifFPS:       16,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if doc.DocumentType != "gif" {
		t.Fatalf("document type = %q", doc.DocumentType)
	}
	if doc.DocumentName != "Token_Reaction.gif" {
		t.Fatalf("document name = %q", doc.DocumentName)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(doc.Bytes))
	if err != nil {
		t.Fatalf("decode gif: %v", err)
	}
	if len(decoded.Image) != 16 {
		t.Fatalf("gif frames = %d, want 16", len(decoded.Image))
	}
	if decoded.LoopCount != 0 {
		t.Fatalf("loop count = %d, want infinite loop", decoded.LoopCount)
	}
	if len(decoded.Delay) != 16 || decoded.Delay[0] != 6 {
		t.Fatalf("gif delay = %#v, want 6cs at 16fps", decoded.Delay)
	}
	if len(doc.Sidecars) != 1 || doc.Sidecars[0].FileName != "Token_Reaction.sheet.png" {
		t.Fatalf("sidecars = %#v", doc.Sidecars)
	}
	if !bytes.Equal(doc.Sidecars[0].Bytes, sheetBytes) {
		t.Fatal("sheet sidecar bytes changed")
	}
	if llm.lastImageRequest.Size != "1024x1024" {
		t.Fatalf("image size = %q", llm.lastImageRequest.Size)
	}
	if llm.lastImageRequest.TargetAspectRatio != 1 {
		t.Fatalf("aspect ratio = %f", llm.lastImageRequest.TargetAspectRatio)
	}
	for _, needle := range []string{"4x4", "16", "1024x1024", "同一个主体", "Token 用完了吗"} {
		if !strings.Contains(llm.lastImageRequest.Prompt, needle) {
			t.Fatalf("gif prompt missing %q:\n%s", needle, llm.lastImageRequest.Prompt)
		}
	}
	if doc.HostedCreditBalance == nil || *doc.HostedCreditBalance != 9 {
		t.Fatalf("hosted credit balance = %#v", doc.HostedCreditBalance)
	}
	if doc.HostedCreditsCharged == nil || *doc.HostedCreditsCharged != 3 {
		t.Fatalf("hosted credits charged = %#v", doc.HostedCreditsCharged)
	}
}

func TestPPTXBuildOptionsCreditChargedSinkAccumulates(t *testing.T) {
	called := 0
	totalCharged := 0
	sink := func(charged int) {
		called++
		totalCharged += charged
	}
	opts := PPTXBuildOptions{
		CreditChargedSink: sink,
	}
	opts.CreditChargedSink(4)
	opts.CreditChargedSink(7)
	opts.CreditChargedSink(11)
	if called != 3 {
		t.Fatalf("expected 3 sink invocations, got %d", called)
	}
	if totalCharged != 22 {
		t.Fatalf("expected accumulated total 22, got %d", totalCharged)
	}
}

func makeGIFSheetPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	cellW := width / 4
	cellH := height / 4
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			idx := row*4 + col
			fill := color.RGBA{R: uint8(16 * idx), G: uint8(255 - 12*idx), B: uint8(64 + 8*idx), A: 255}
			for y := row * cellH; y < (row+1)*cellH; y++ {
				for x := col * cellW; x < (col+1)*cellW; x++ {
					img.SetRGBA(x, y, fill)
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode sheet png: %v", err)
	}
	return buf.Bytes()
}

func TestServiceGenerateIMGRejectsEmptyImageData(t *testing.T) {
	service := NewService(&fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{MIME: "image/png"},
	}, nil)

	_, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeIMG,
		Prompt:       "A polished product launch hero image",
		Topic:        "Launch Visual",
		ImageRatio:   "square",
	})
	if err == nil || !strings.Contains(err.Error(), "image generation returned empty data") {
		t.Fatalf("err = %v", err)
	}
}

func TestPrepareAgentPayloadForReport(t *testing.T) {
	workbookBytes, err := officegen.NewXLSXGenerator().Generate([]officegen.XlsxSheet{
		{
			Name: "Summary",
			Rows: [][]string{
				{"Region", "Revenue", "Growth"},
				{"North America", "128", "+12%"},
				{"Europe", "96", "+8%"},
			},
		},
	}, officegen.XLSXOptions{Title: "Q2 Review", Creator: "OfficeCLI"})
	if err != nil {
		t.Fatalf("Generate workbook: %v", err)
	}
	workbookPath := filepath.Join(t.TempDir(), "source.xlsx")
	if err := os.WriteFile(workbookPath, workbookBytes, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	prepared, err := PrepareAgentPayload(PrepareParams{
		DocumentType:   engine.DocumentTypeReport,
		Topic:          "Q2 Review",
		SourceFilePath: workbookPath,
	})
	if err != nil {
		t.Fatalf("PrepareAgentPayload: %v", err)
	}
	if prepared.PreferredTool != "office.render" || !prepared.PrepareRequired {
		t.Fatalf("unexpected prepare metadata: %#v", prepared)
	}
	if !strings.Contains(prepared.WorkbookSummary, "North America") {
		t.Fatalf("unexpected workbook summary: %s", prepared.WorkbookSummary)
	}
	if len(prepared.BaseReportJSON) == 0 {
		t.Fatal("expected base report json")
	}
}

func TestServiceRenderDOCXWithoutLLM(t *testing.T) {
	service := NewService(nil, nil)
	doc, err := service.Render(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeDOCX,
		Topic:        "Quarterly Brief",
	}, json.RawMessage(`{"title":"Quarterly Brief","sections":[{"heading":"Summary","level":1,"paragraphs":["Delivery-ready content."]}]}`))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["word/document.xml"], "Quarterly Brief") {
		t.Fatalf("document xml = %q", contentXMLs["word/document.xml"])
	}
}

func TestServiceGeneratePPTXWithFakeLLM(t *testing.T) {
	service := NewService(&fakeLLMClient{
		jsonResponse: `{
			"title":"Enterprise Collaboration Platform Overview",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"Enterprise Collaboration Platform Overview","layout":"title","subtitle":"Product context and business status","isTitle":true},
				{"title":"Product Capabilities","layout":"content","points":["Multi-user collaboration","Real-time editing","Enterprise administration"]}
			]
		}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Describe the product capabilities, customer value, and use cases of this collaboration platform.",
		Topic:        "Enterprise Collaboration Platform Overview",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(doc.Bytes, ooxmledit.FileTypePPTX)
	if err != nil {
		t.Fatalf("ExtractContentXML: %v", err)
	}
	if !strings.Contains(contentXMLs["ppt/slides/slide1.xml"], "Enterprise Collabo") {
		t.Fatalf("slide xml = %q", contentXMLs["ppt/slides/slide1.xml"])
	}
}

func TestServiceGeneratePPTXArtifactDebugMetadataRequiresDebugFlag(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, workDir string) (*pptxArtifactWorkerOutput, error) {
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 2, 1)
		output.WorkerDir = workDir
		output.ScriptPath = filepath.Join(workDir, "pptx_artifact_worker.mjs")
		output.RequestPath = filepath.Join(workDir, "request.json")
		output.ResponsePath = filepath.Join(workDir, "response.json")
		output.WorkerVersion = "artifact-experimental-test"
		return output, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	response := `{
		"title":"Artifact Debug Service",
		"slides":[
			{"title":"Artifact Debug Service","layout":"title","subtitle":"Debug metadata smoke","isTitle":true},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Signal","categories":["A","B"],"values":[1,2]}}
		]
	}`
	service := NewService(&fakeLLMClient{jsonResponse: response}, nil)
	withoutDebug, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Create a concise editable presentation.",
		Topic:        "Artifact Debug Service",
		PPTXBackend:  PPTXBackendArtifactWorker,
	})
	if err != nil {
		t.Fatalf("Generate without debug: %v", err)
	}
	if withoutDebug.PPTXArtifactDebug != nil {
		t.Fatalf("debug metadata should be omitted without debug flag: %#v", withoutDebug.PPTXArtifactDebug)
	}
	withDebug, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Create a concise editable presentation.",
		Topic:        "Artifact Debug Service",
		PPTXBackend:  PPTXBackendArtifactWorker,
		Debug:        true,
	})
	if err != nil {
		t.Fatalf("Generate with debug: %v", err)
	}
	if withDebug.PPTXArtifactDebug == nil {
		t.Fatal("debug metadata missing with debug flag")
	}
	if withDebug.PPTXArtifactDebug.WorkerVersion != "artifact-experimental-test" || withDebug.PPTXArtifactDebug.PreviewCount == 0 {
		t.Fatalf("debug metadata = %#v", withDebug.PPTXArtifactDebug)
	}
}

func TestServiceGeneratePPTXInjectsReferenceStyleProfile(t *testing.T) {
	root := t.TempDir()
	writeRuntimeReferenceDeck(t, filepath.Join(root, "brand.pptx"), "Brand Reference", "Georgia")
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"Reference Guided Deck",
			"slides":[
				{"title":"Reference Guided Deck","layout":"title","subtitle":"Style-aware overview","isTitle":true},
				{"title":"Reuse Pattern","layout":"content","points":["Use reference rhythm","Keep content editable"]}
			]
		}`,
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType:         engine.DocumentTypePPTX,
		Prompt:               "Create a concise product update deck",
		Topic:                "Reference Guided Deck",
		Mode:                 "fast",
		ReferenceScanEnabled: true,
		ReferenceScanRoot:    root,
		ReferencePPTXSources: nil,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(llm.lastJSONMsgs) != 1 {
		t.Fatalf("CompleteJSON messages = %d, want 1", len(llm.lastJSONMsgs))
	}
	prompt := llm.lastJSONMsgs[0].Content
	for _, needle := range []string{
		"Reference style profile",
		"Discovered PPTX files: 1",
		"Parsed PPTX files: 1",
		"Georgia",
		"Use these as style intent only",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q:\n%s", needle, prompt)
		}
	}
	if strings.Contains(prompt, "<p:sld") || strings.Contains(prompt, "<a:theme") {
		t.Fatalf("reference prompt should not include raw PPTX XML:\n%s", prompt)
	}
	if doc.ReferenceStyle == nil || doc.ReferenceStyle.ParsedCount != 1 || doc.ReferenceStyle.DiscoveredCount != 1 {
		t.Fatalf("reference style metadata = %#v", doc.ReferenceStyle)
	}
}

func TestBuildPPTXPromptHonorsExplicitFiveSlideCount(t *testing.T) {
	description := "Create a concise 5-slide editable PowerPoint for a quarterly business review. Include: cover, executive summary, KPI trend chart, risks and next actions, closing decision slide."
	prompt := BuildPPTXPromptWithReferenceBrief(description, generateengine.PromptTarget{
		Style:    officegen.StylePresetEditorialLight,
		Audience: "executive leadership",
	}, false, nil, nil)

	for _, needle := range []string{
		"Keep the deck to exactly 5 slides.",
		"slides array must contain exactly 5 slide objects",
		"do not add a contents, agenda, chapter divider, or extra expansion slide",
		"Explicit slide count overrides archetype storyline examples",
		`If an exact slide count is specified, it overrides all "usually" storyline, toc, chapter, and expansion guidance.`,
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing exact slide-count rule %q:\n%s", needle, prompt)
		}
	}
}

func TestDetectExplicitPPTXSlideCount(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{text: "Create a concise 5-slide editable PowerPoint.", want: 5},
		{text: "Create a concise 5 slides deck.", want: 5},
		{text: "Create a concise five-slide deck.", want: 5},
		{text: "做一个 5页 PPT。", want: 5},
		{text: "做一个 五页 PPT。", want: 5},
		{text: "Keep content slide points to 3-4 concise bullets.", want: 0},
	} {
		if got := detectExplicitPPTXSlideCount(tc.text); got != tc.want {
			t.Fatalf("detectExplicitPPTXSlideCount(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestBuildPPTXFromJSONWithOptionsEnforcesExplicitFiveSlideCount(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 5, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Quarterly Business Review",
		"stylePreset":"editorial-light",
		"slides":[
			{"title":"Quarterly Business Review","layout":"title","subtitle":"Executive update","isTitle":true},
			{"title":"Contents","layout":"toc","points":["Summary","Metrics","Risks"]},
			{"title":"Chapter 1","layout":"chapter","subtitle":"Business context"},
			{"title":"Executive Summary","layout":"content","sections":[{"heading":"Growth","detail":"Revenue improved."},{"heading":"Risk","detail":"Pipeline timing remains open."}]},
			{"title":"Operating Context","layout":"content","points":["Demand improved","Retention held"]},
			{"title":"KPI Trend","layout":"chart","chart":{"type":"bar","title":"Revenue attainment","categories":["Q1","Q2","Q3","Q4"],"values":[72,78,86,91]}},
			{"title":"Risk Detail","layout":"content","points":["Pipeline timing","Enterprise procurement"]},
			{"title":"Risk Detail (cont.)","layout":"content","points":["Collections watch","Support load"]},
			{"title":"Next Actions","layout":"content","sections":[{"heading":"Owner","detail":"Revenue team"},{"heading":"Timing","detail":"This quarter"}]},
			{"title":"Closing Decision","layout":"closing","content":"Approve the next-quarter operating plan."}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Quarterly Business Review", officegen.StylePresetEditorialLight, false, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise 5-slide editable PowerPoint for a quarterly business review. Include: cover, executive summary, KPI trend chart, risks and next actions, closing decision slide.",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if len(captured.Slides) != 5 {
		t.Fatalf("artifact worker slide count = %d, want 5; slides=%#v", len(captured.Slides), captured.Slides)
	}
	for _, slide := range captured.Slides {
		if strings.EqualFold(slide.Title, "Contents") || strings.EqualFold(slide.Layout, "toc") || strings.EqualFold(slide.Layout, "chapter") {
			t.Fatalf("explicit slide-count deck should drop scaffold slide: %#v", slide)
		}
	}
	if !containsPPTXChartSlide(captured.Slides) {
		t.Fatalf("explicit slide-count deck should preserve a chart slide: %#v", captured.Slides)
	}
	if !strings.Contains(strings.ToLower(captured.Slides[len(captured.Slides)-1].Title), "closing") && !looksLikeClosingSlide(captured.Slides[len(captured.Slides)-1]) {
		t.Fatalf("explicit slide-count deck should preserve a closing slide, got %#v", captured.Slides[len(captured.Slides)-1])
	}
	if !containsIssueCode(warnings, "WARN_PPT_EXPLICIT_SLIDE_COUNT_ENFORCED") {
		t.Fatalf("warnings missing explicit slide-count normalization warning: %#v", warnings)
	}
}

func TestBuildPPTXReferenceStylePromptOmitsGeneratedOutputDetailsWhenOnlyCurrentOutput(t *testing.T) {
	profile := &pptxref.ReferenceStyleProfile{
		DiscoveredCount:                  2,
		ParsedCount:                      2,
		SourceBuckets:                    map[string]int{"current-output": 2},
		AggregateSlideCount:              8,
		FontFamilies:                     []string{"Aptos", "Generated Sans"},
		ThemeColors:                      []string{"0E2841", "38D9FF"},
		LayoutSignals:                    map[string]int{"content": 16},
		RepresentativeSlideTextSummaries: []string{"Slide 1: Generated output title that should not be learned"},
		ReuseGuidance:                    []string{"Treat previous output PPTX files as weak style hints."},
		Limitations:                      []string{"Only previous output PPTX files were found; these are weak reference signals."},
		SourceFiles: []pptxref.ReferencePPTXFile{
			{Path: "/root/output/generated-a.pptx", SourceBucket: "current-output"},
			{Path: "/root/output/generated-b.pptx", SourceBucket: "current-output"},
		},
	}
	prompt := BuildPPTXReferenceStylePrompt(profile, "Create a deck", generateengine.PromptTarget{})
	for _, forbidden := range []string{
		"Generated output title",
		"Generated Sans",
		"0E2841",
		"38D9FF",
		"Layout signals: content",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("output-only reference prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
	for _, expected := range []string{
		"Source buckets: current-output=2",
		"Only previous output PPTX files were found",
		"Treat previous output PPTX files as weak style hints",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("output-only reference prompt missing %q:\n%s", expected, prompt)
		}
	}
}

func TestDeterministicPPTXReferenceStyleBriefTreatsCurrentOutputOnlyAsUnavailable(t *testing.T) {
	profile := &pptxref.ReferenceStyleProfile{
		ParsedCount:         2,
		SourceBuckets:       map[string]int{"current-output": 2},
		FontFamilies:        []string{"Generated Sans"},
		ThemeColors:         []string{"0E2841", "38D9FF"},
		LayoutSignals:       map[string]int{"dashboard": 10},
		AggregateSlideCount: 8,
		SourceFiles: []pptxref.ReferencePPTXFile{
			{Path: "/root/output/generated-a.pptx", SourceBucket: "current-output"},
			{Path: "/root/output/generated-b.pptx", SourceBucket: "current-output"},
		},
	}
	brief := deterministicPPTXReferenceStyleBrief(profile)
	if brief == nil {
		t.Fatal("deterministicPPTXReferenceStyleBrief returned nil")
	}
	for _, forbidden := range []string{"Generated Sans", "0E2841", "38D9FF", "dashboard"} {
		if strings.Contains(brief.PaletteIntent, forbidden) ||
			strings.Contains(brief.TypographyIntent, forbidden) ||
			strings.Contains(brief.LayoutRhythm, forbidden) {
			t.Fatalf("output-only deterministic brief leaked %q: %#v", forbidden, brief)
		}
	}
	if !strings.Contains(brief.LayoutRhythm, "No stable reference") {
		t.Fatalf("LayoutRhythm = %q, want unavailable stable reference guidance", brief.LayoutRhythm)
	}
	if !containsString(brief.RendererConstraints, "ignore previous generated-output style details unless the user explicitly asks to reuse them") {
		t.Fatalf("RendererConstraints = %#v, want generated-output ignore constraint", brief.RendererConstraints)
	}
}

func TestDeterministicPPTXReferenceStyleBriefUsesSlidePaletteForPreset(t *testing.T) {
	cases := []struct {
		name   string
		colors []string
		want   string
	}{
		{name: "warm copper", colors: []string{"FF6A00", "9A3412", "FBF4EB"}, want: officegen.StylePresetReviewCopper},
		{name: "forest", colors: []string{"000000", "111111", "222222", "14532D", "4D7C0F", "EEF5EA"}, want: officegen.StylePresetProjectForest},
		{name: "dark", colors: []string{"0B1020", "111827", "F97316"}, want: officegen.StylePresetExecutiveDark},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			brief := deterministicPPTXReferenceStyleBrief(&pptxref.ReferenceStyleProfile{
				ParsedCount:   1,
				SourceBuckets: map[string]int{"other": 1},
				SourceFiles: []pptxref.ReferencePPTXFile{
					{Path: "/root/brand.pptx", SourceBucket: "other"},
				},
				ThemeColors: tc.colors,
			})
			if brief == nil {
				t.Fatal("deterministicPPTXReferenceStyleBrief returned nil")
			}
			if brief.StylePresetHint != tc.want {
				t.Fatalf("StylePresetHint = %q, want %q; brief=%#v", brief.StylePresetHint, tc.want, brief)
			}
			if !strings.Contains(brief.PaletteIntent, tc.colors[0]) {
				t.Fatalf("PaletteIntent = %q, want reference color %s", brief.PaletteIntent, tc.colors[0])
			}
		})
	}
}

func TestBuildPPTXReferenceStyleBriefSkipsLLMForCurrentOutputOnlyProfile(t *testing.T) {
	profile := &pptxref.ReferenceStyleProfile{
		ParsedCount:         2,
		SourceBuckets:       map[string]int{"current-output": 2},
		AggregateSlideCount: 8,
		SourceFiles: []pptxref.ReferencePPTXFile{
			{Path: "/root/output/generated-a.pptx", SourceBucket: "current-output"},
			{Path: "/root/output/generated-b.pptx", SourceBucket: "current-output"},
		},
	}
	llm := &fakeLLMClient{
		structuredResponse: `{
			"stylePresetHint":"editorial-light",
			"paletteIntent":"use generated output palette",
			"typographyIntent":"copy generated output typography",
			"layoutRhythm":"copy generated output rhythm",
			"imageTreatment":"reuse prior output images",
			"doNotCopy":["raw XML"],
			"rendererConstraints":["keep editable text"]
		}`,
	}
	service := NewService(llm, nil)
	brief, warnings := service.buildPPTXReferenceStyleBrief(context.Background(), profile, "Create a deck", generateengine.PromptTarget{})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if llm.structuredCallCount != 0 {
		t.Fatalf("structured calls = %d, want output-only profile to skip LLM style brief", llm.structuredCallCount)
	}
	if brief == nil || brief.StylePresetHint != officegen.StylePresetExecutiveDark {
		t.Fatalf("brief = %#v, want deterministic executive-dark fallback", brief)
	}
	if strings.Contains(brief.PaletteIntent, "generated output palette") {
		t.Fatalf("brief leaked LLM output-only style: %#v", brief)
	}
}

func TestReferenceStyleMetadataOmitsGeneratedOutputOnlySignals(t *testing.T) {
	profile := &pptxref.ReferenceStyleProfile{
		Root:                "/root",
		DiscoveredCount:     2,
		ParsedCount:         2,
		SourceBuckets:       map[string]int{"current-output": 2},
		AggregateSlideCount: 8,
		FontFamilies:        []string{"Generated Sans"},
		ThemeColors:         []string{"0E2841", "38D9FF"},
		LayoutSignals:       map[string]int{"dashboard": 10},
		Limitations:         []string{"Only previous output PPTX files were found; these are weak reference signals."},
		SourceFiles: []pptxref.ReferencePPTXFile{
			{Path: "/root/output/generated-a.pptx", SourceBucket: "current-output"},
			{Path: "/root/output/generated-b.pptx", SourceBucket: "current-output"},
		},
	}
	meta := referenceStyleMetadata(profile, deterministicPPTXReferenceStyleBrief(profile), true)
	if meta == nil {
		t.Fatal("referenceStyleMetadata returned nil")
	}
	if len(meta.FontFamilies) != 0 || len(meta.ThemeColors) != 0 || len(meta.LayoutSignals) != 0 {
		t.Fatalf("metadata leaked generated-output style signals: %#v", meta)
	}
	if meta.SourceBuckets["current-output"] != 2 || meta.AggregateSlideCount != 8 {
		t.Fatalf("metadata should keep transparent source counts, got %#v", meta)
	}
	if !containsStringContaining(meta.Limitations, "Only previous output PPTX files were found") {
		t.Fatalf("Limitations = %#v, want output-only limitation", meta.Limitations)
	}
	if meta.StyleBrief == nil || !containsString(meta.StyleBrief.RendererConstraints, "ignore previous generated-output style details unless the user explicitly asks to reuse them") {
		t.Fatalf("StyleBrief = %#v, want safe generated-output constraints", meta.StyleBrief)
	}
}

func TestServiceGeneratePPTXReferenceStyleBriefMapsRendererPreset(t *testing.T) {
	root := t.TempDir()
	writeRuntimeReferenceDeck(t, filepath.Join(root, "training.pptx"), "Training Reference", "Aptos")
	llm := &fakeLLMClient{
		structuredResponse: `{
			"stylePresetHint":"training-manual",
			"paletteIntent":"quiet instructional palette",
			"typographyIntent":"clear training hierarchy",
			"layoutRhythm":"step-by-step sections",
			"imageTreatment":"minimal diagrams",
			"doNotCopy":["source wording"],
			"rendererConstraints":["keep editable text"]
		}`,
		jsonResponse: `{
			"title":"Reference Guided Training",
			"slides":[
				{"title":"Reference Guided Training","layout":"title","subtitle":"Style-aware overview","isTitle":true},
				{"title":"Reuse Pattern","layout":"content","points":["Use reference rhythm","Keep content editable"]}
			]
		}`,
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType:         engine.DocumentTypePPTX,
		Prompt:               "Create a concise training deck",
		Topic:                "Reference Guided Training",
		Mode:                 "fast",
		ReferenceScanEnabled: true,
		ReferenceScanRoot:    root,
		LocalPreview:         true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.structuredCallCount != 1 {
		t.Fatalf("structured calls = %d, want 1", llm.structuredCallCount)
	}
	if llm.lastStructuredReq.Schema.Name != "pptx_reference_style_brief" {
		t.Fatalf("schema name = %q", llm.lastStructuredReq.Schema.Name)
	}
	if !strings.Contains(llm.lastJSONMsgs[0].Content, "PPTX reference style brief") ||
		!strings.Contains(llm.lastJSONMsgs[0].Content, "training-manual") {
		t.Fatalf("prompt did not include style brief:\n%s", llm.lastJSONMsgs[0].Content)
	}
	if doc.ReferenceStyle == nil || doc.ReferenceStyle.StyleBrief == nil {
		t.Fatalf("reference style metadata missing brief: %#v", doc.ReferenceStyle)
	}
	var preview struct {
		StylePreset string `json:"stylePreset"`
	}
	if err := json.Unmarshal(doc.PreviewJSON, &preview); err != nil {
		t.Fatalf("parse preview json: %v\n%s", err, string(doc.PreviewJSON))
	}
	if preview.StylePreset != officegen.StylePresetTrainingManual {
		t.Fatalf("preview stylePreset = %q, want %q", preview.StylePreset, officegen.StylePresetTrainingManual)
	}
}

func writeRuntimeReferenceDeck(t *testing.T, path, title, font string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir reference deck: %v", err)
	}
	data, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{{
		Title:   title,
		Content: "Reference content",
		Layout:  "content",
		Points:  []string{"Reusable rhythm", "Editable foreground text"},
	}}, officegen.PPTXOptions{
		Title:   title,
		Creator: "test",
		Theme: &officegen.SlideTheme{
			PrimaryColor:   "123456",
			AccentColor:    "ABCDEF",
			BackgroundType: "solid",
			BgColor1:       "FFFFFF",
			BgColor2:       "FFFFFF",
			TextColor:      "111111",
			TitleTextColor: "222222",
			FontFamily:     font,
			EAFontFamily:   "Noto Sans CJK SC",
		},
	})
	if err != nil {
		t.Fatalf("generate reference deck: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write reference deck: %v", err)
	}
}

func injectRuntimeSlideColors(t *testing.T, path string, colors []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read deck: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open pptx: %v", err)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	replaced := false
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip member %s: %v", file.Name, err)
		}
		member, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read zip member %s: %v", file.Name, err)
		}
		if file.Name == "ppt/slides/slide1.xml" {
			var shapes strings.Builder
			for idx, color := range colors {
				shapes.WriteString(fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Injected Palette Signal %d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="%d" y="0"/><a:ext cx="1" cy="1"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p/></p:txBody></p:sp>`, 9100+idx, idx+1, idx+1, color))
			}
			next := strings.Replace(string(member), "</p:spTree>", shapes.String()+"</p:spTree>", 1)
			if next != string(member) {
				replaced = true
				member = []byte(next)
			}
		}
		header := file.FileHeader
		header.Method = zip.Deflate
		w, err := writer.CreateHeader(&header)
		if err != nil {
			t.Fatalf("create zip member %s: %v", file.Name, err)
		}
		if _, err := w.Write(member); err != nil {
			t.Fatalf("write zip member %s: %v", file.Name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if !replaced {
		t.Fatalf("did not inject slide colors %#v in %s", colors, path)
	}
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatalf("write patched deck: %v", err)
	}
}

func TestServiceGeneratePPTXCombinesCompletionAndImageHostedCredits(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"Product Launch",
			"slides":[
				{"title":"Product Launch","layout":"title","subtitle":"Go-to-market overview","isTitle":true,"hasImage":true,"imagePrompt":"A polished product launch visual","imagePos":"background"}
			]
		}`,
		completionCharged: 4,
		completionBalance: 1100240,
		completionValid:   true,
		imageResult:       &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png", CreditBalance: intPtr(1100230), CreditsCharged: intPtr(10)},
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Explain the launch plan",
		Topic:        "Product Launch",
		EnableImages: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if doc.HostedCreditsCharged == nil || *doc.HostedCreditsCharged != 14 {
		t.Fatalf("hosted credits charged = %#v, want 14", doc.HostedCreditsCharged)
	}
	if doc.HostedCreditBalance == nil || *doc.HostedCreditBalance != 1100230 {
		t.Fatalf("hosted credit balance = %#v, want 1100230", doc.HostedCreditBalance)
	}
	if llm.imageCalls != 1 {
		t.Fatalf("image calls = %d, want 1", llm.imageCalls)
	}
}

func TestServiceGeneratePPTXReportsCompletionHostedCreditsWithoutImages(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"Product Launch",
			"slides":[
				{"title":"Product Launch","layout":"title","subtitle":"Go-to-market overview","isTitle":true}
			]
		}`,
		completionCharged: 4,
		completionBalance: 1100240,
		completionValid:   true,
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Explain the launch plan",
		Topic:        "Product Launch",
		EnableImages: false,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if doc.HostedCreditsCharged == nil || *doc.HostedCreditsCharged != 4 {
		t.Fatalf("hosted credits charged = %#v, want 4", doc.HostedCreditsCharged)
	}
	if doc.HostedCreditBalance == nil || *doc.HostedCreditBalance != 1100240 {
		t.Fatalf("hosted credit balance = %#v, want 1100240", doc.HostedCreditBalance)
	}
	if llm.imageCalls != 0 {
		t.Fatalf("image calls = %d, want 0", llm.imageCalls)
	}
}

func TestServiceRenderPPTXWithoutTextLLMCalls(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png", CreditBalance: intPtr(42), CreditsCharged: intPtr(5)},
	}
	service := NewService(llm, nil)

	doc, err := service.Render(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "Enterprise Collaboration Platform Overview",
		Style:        "editorial-light",
		EnableImages: true,
	}, json.RawMessage(`{
		"title":"Enterprise Collaboration Platform Overview",
		"stylePreset":"editorial-light",
		"theme":null,
		"slides":[
			{"title":"Enterprise Collaboration Platform Overview","content":"","isTitle":true,"layout":"title","variant":"title-center","narrativeRole":"cover","sectionIndex":0,"sectionTitle":"","subtitle":"Product context and business status","points":[],"sections":[],"chart":null,"metrics":[],"source":"","bgColor":"","bgColor2":"","hasImage":true,"imagePrompt":"A polished enterprise dashboard hero image","imagePos":"background","visuals":[]}
		]
	}`))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if llm.jsonCallCount != 0 || llm.structuredCallCount != 0 {
		t.Fatalf("unexpected text llm calls: json=%d structured=%d", llm.jsonCallCount, llm.structuredCallCount)
	}
	if llm.imageCalls == 0 {
		t.Fatal("expected image call")
	}
	if countZipEntries(doc.Bytes, "ppt/media/", ".png") == 0 {
		t.Fatal("expected embedded ppt media")
	}
	if doc.HostedCreditsCharged == nil || *doc.HostedCreditsCharged != 5 {
		t.Fatalf("hosted credits charged = %#v", doc.HostedCreditsCharged)
	}
}

func TestServiceGenerateReportWithFakeLLM(t *testing.T) {
	workbookBytes, err := officegen.NewXLSXGenerator().Generate([]officegen.XlsxSheet{
		{
			Name: "Revenue",
			Rows: [][]string{
				{"Region", "Revenue"},
				{"North America", "128"},
				{"Europe", "96"},
				{"APAC", "74"},
			},
		},
	}, officegen.XLSXOptions{Title: "Q2 Business Review", Creator: "test"})
	if err != nil {
		t.Fatalf("Generate workbook: %v", err)
	}
	workbookPath := filepath.Join(t.TempDir(), "q2_metrics.xlsx")
	if err := os.WriteFile(workbookPath, workbookBytes, 0o644); err != nil {
		t.Fatalf("Write workbook: %v", err)
	}

	service := NewService(&fakeLLMClient{
		jsonResponse: `{
			"title":"Q2 Business Review",
			"subtitle":"Commercial momentum and decision points",
			"audience":"Board and investors",
			"summary":"Growth continued, but conversion efficiency softened in the final mile.",
			"kpis":[{"label":"Revenue","value":"$12.4M","change":"+8% QoQ"}],
			"findings":["North America remained ahead of plan."],
			"sections":[
				{
					"title":"Demand momentum",
					"subtitle":"Headline view of regional performance",
					"narrative":["North America led the quarter while Europe stayed stable."],
					"charts":[
						{
							"type":"bar",
							"title":"Regional revenue",
							"categories":["North America","Europe","APAC"],
							"series":[{"name":"Revenue","values":[128,96,74]}],
							"source":"Internal finance data"
						}
					]
				}
			],
			"appendixTables":[
				{
					"title":"Supporting table",
					"headers":["Region","Revenue"],
					"rows":[["North America","128"],["Europe","96"]]
				}
			]
		}`,
	}, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType:   engine.DocumentTypeReport,
		Prompt:         "Create a board-ready report for the latest business review.",
		Topic:          "Q2 Business Review",
		SourceFilePath: workbookPath,
		Mode:           "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	output := string(doc.Bytes)
	for _, needle := range []string{
		"<html lang=",
		"Q2 Business Review",
		"Demand momentum",
		"echarts.min.js",
		"Regional revenue",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("report html missing %q:\n%s", needle, output)
		}
	}
}

func TestBuildPPTXPrompt_ImagesEnabledIncludesImageGuidance(t *testing.T) {
	prompt := BuildPPTXPrompt("Introduce product capabilities", generateengine.PromptTarget{}, true)
	for _, needle := range []string{
		`"visual": {"kind": "image"`,
		`"prompt": "A concrete visual prompt that can be sent directly to an image model"`,
		"Use images sparingly. Prefer at most one hero image slide plus at most one gallery slide",
		"Do not add images to chart, dashboard, toc, or closing layouts",
		"For gallery slides, use a visual image for the page theme",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q:\n%s", needle, prompt)
		}
	}
}

func TestBuildPPTXPrompt_ImagesDisabledForbidsImageFields(t *testing.T) {
	prompt := BuildPPTXPrompt("Introduce product capabilities", generateengine.PromptTarget{}, false)
	if strings.Contains(prompt, `"hasImage": true`) {
		t.Fatalf("prompt should not include image schema when disabled:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Do not output visual objects or image fields.") {
		t.Fatalf("prompt should forbid image fields when disabled:\n%s", prompt)
	}
}

func TestBuildPPTXPrompt_IncludesQualityConstraints(t *testing.T) {
	prompt := BuildPPTXPrompt("Introduce product capabilities", generateengine.PromptTarget{
		Language: "en-US",
		Style:    "Professional and restrained",
		Audience: "Prospective enterprise customers",
	}, true)
	for _, needle := range []string{
		"Keep the deck to 6-10 slides, usually 7-9.",
		"slide 2 should usually be a toc page",
		"takeaway must be a slide-level conclusion sentence",
		"Use at most 3 sections, at most 4 dashboard metrics",
		"Do not use charts for priorities, milestones, strategy, risks, or process flows",
		"Only use rollout-style endings when the request explicitly asks for rollout",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q:\n%s", needle, prompt)
		}
	}
}

func TestBuildPPTXPrompt_RequestsSemanticDeckSpec(t *testing.T) {
	prompt := BuildPPTXPrompt("Introduce product capabilities", generateengine.PromptTarget{}, true)
	for _, needle := range []string{
		`"headline": "Cover Title"`,
		`"takeaway": "One-sentence takeaway"`,
		`"blocks": [`,
		`{"type": "sections"`,
		`"visual": {"kind": "image"`,
		"Use role/headline/takeaway/blocks/visual as the preferred semantic schema",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing semantic schema marker %q:\n%s", needle, prompt)
		}
	}
}

func TestBuildPPTXPrompt_RespectsExplicitFourSlideConciseRequest(t *testing.T) {
	prompt := BuildPPTXPrompt("Create a concise editable presentation. Include a cover slide, key observations, one simple chart, and a closing slide.", generateengine.PromptTarget{}, true)
	if !strings.Contains(prompt, "Keep the deck to 4 slides") {
		t.Fatalf("prompt should preserve explicit four-slide structure:\n%s", prompt)
	}
	if strings.Contains(prompt, "Keep the deck to 6-10 slides") {
		t.Fatalf("prompt should not force 6-10 slides for explicit four-slide request:\n%s", prompt)
	}
}

func TestBuildPPTXPrompt_UsesArchetypeRules(t *testing.T) {
	companyPrompt := BuildPPTXPrompt("enterprise collaboration platform", generateengine.PromptTarget{}, true)
	if !strings.Contains(companyPrompt, "a strong storyline is usually cover -> toc -> chapter -> key takeaways -> core capabilities -> customer value -> use cases -> chapter -> rollout path") {
		t.Fatalf("company prompt missing archetype outline:\n%s", companyPrompt)
	}
	marketPrompt := BuildPPTXPrompt("market opportunity analysis", generateengine.PromptTarget{}, false)
	if !strings.Contains(marketPrompt, "Slide 3 must use a chart and include a source.") {
		t.Fatalf("market prompt missing archetype outline:\n%s", marketPrompt)
	}
	opsPrompt := BuildPPTXPrompt("business review", generateengine.PromptTarget{}, false)
	if !strings.Contains(opsPrompt, "a strong storyline is usually cover -> toc -> chapter -> business takeaways -> core metrics -> issue diagnosis -> next-quarter priorities -> chapter -> execution actions") {
		t.Fatalf("ops prompt missing archetype outline:\n%s", opsPrompt)
	}
	trainingPrompt := BuildPPTXPrompt("new hire onboarding training", generateengine.PromptTarget{}, false)
	if !strings.Contains(trainingPrompt, "a strong storyline is usually cover -> toc -> chapter -> learning goals -> installation and setup -> common commands -> example workflow -> chapter -> cautions") {
		t.Fatalf("training prompt missing archetype outline:\n%s", trainingPrompt)
	}
	explainerPrompt := BuildPPTXPrompt("minecraft 游戏介绍", generateengine.PromptTarget{}, true)
	if !strings.Contains(explainerPrompt, "go straight into the topic with a 6-8 slide explainer arc") {
		t.Fatalf("explainer prompt missing direct explainer outline:\n%s", explainerPrompt)
	}
	if !strings.Contains(explainerPrompt, "Do not insert contents or chapter-divider scaffolding for this topic.") {
		t.Fatalf("explainer prompt missing scaffold skip rule:\n%s", explainerPrompt)
	}
}

func TestCleanSentence_PreservesTimeNumbers(t *testing.T) {
	if got := cleanSentence("Complete the first validation cycle within 30 days."); got != "Complete the first validation cycle within 30 days" {
		t.Fatalf("cleanSentence() = %q", got)
	}
	if got := cleanSentence("1. Clarify the goal"); got != "Clarify the goal" {
		t.Fatalf("cleanSentence() = %q", got)
	}
}

func TestFitTextForLayout_PrefersWholeClause(t *testing.T) {
	got := fitTextForLayout("Validate PMF in Southeast Asia first, then expand into Europe for higher-value deals", 18)
	if strings.Contains(got, "...") {
		t.Fatalf("fitTextForLayout() should avoid ellipsis: %q", got)
	}
	if len(got) == 0 {
		t.Fatal("fitTextForLayout() should return non-empty text")
	}
}

func TestFitTextForLayout_PreservesLongSpacedText(t *testing.T) {
	input := "This sentence has many words and no punctuation before the maximum layout boundary"
	got := fitTextForLayout(input, 32)
	if got != input {
		t.Fatalf("fitTextForLayout() = %q", got)
	}
}

func TestFitTextForLayout_FinishesTruncatedPhrase(t *testing.T) {
	got := fitTextForLayout("Clear decision rights prevent duplicated work and missed handoffs across functions", 18)
	if got != "Clear decision rights prevent duplicated work and missed handoffs across functions" {
		t.Fatalf("fitTextForLayout() produced unfinished phrase: %q", got)
	}
}

func TestNormalizePointsAndSections_ControlTextDensity(t *testing.T) {
	points := normalizePoints([]string{
		"This point is intentionally much too long for a slide bullet and needs to be shortened before rendering",
	}, 4, 32)
	if len(points) != 1 || strings.HasSuffix(points[0], " and") {
		t.Fatalf("points = %#v, want one complete point", points)
	}

	sections := normalizeSections([]officegen.SlideSection{
		{
			Heading: "A very long section heading that should not consume the whole card",
			Detail:  "A very long section detail that would otherwise create dense unreadable card copy in the generated slide layout",
		},
	}, 3)
	if len(sections) != 1 {
		t.Fatalf("sections = %#v, want one section", sections)
	}
	if strings.HasSuffix(sections[0].Heading, " and") || strings.HasSuffix(sections[0].Detail, " and") {
		t.Fatalf("section was not kept semantically complete: %#v", sections[0])
	}
}

func TestNormalizeActionSlide_ConvertsPointsToSections(t *testing.T) {
	slide := normalizeActionSlide(officegen.Slide{
		Title: "execution actions",
		Points: []string{
			"30 days product lead confirms the main scenario and finishes target-customer interviews",
			"60 days channel lead signs partners and validates lead cost",
		},
	})
	if len(slide.Sections) != 2 {
		t.Fatalf("sections = %#v", slide.Sections)
	}
	if slide.Sections[0].Heading != "Step 1" {
		t.Fatalf("first heading = %q", slide.Sections[0].Heading)
	}
	if len(slide.Points) != 0 {
		t.Fatalf("points should be cleared after section normalization: %#v", slide.Points)
	}
}

func TestNormalizeEvidenceSlide_PromotesValueAndMarketSlides(t *testing.T) {
	valueSlide := normalizeEvidenceSlide(officegen.Slide{Title: "customer value"})
	if valueSlide.Layout != "dashboard" || len(valueSlide.Metrics) == 0 {
		t.Fatalf("value slide = %#v", valueSlide)
	}
	marketSlide := normalizeEvidenceSlide(officegen.Slide{Title: "market size"})
	if marketSlide.Layout != "chart" || marketSlide.Chart == nil {
		t.Fatalf("market slide = %#v", marketSlide)
	}
	if marketSlide.Source == "" {
		t.Fatalf("market slide should inject source hint")
	}
}

func TestNormalizePPTXPayload_EnforcesCompanySkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "enterprise collaboration platform",
		Slides: []officegen.Slide{
			{Title: "enterprise collaboration platform", Layout: "title", IsTitle: true},
			{Title: "first slide"},
			{Title: "second slide"},
		},
	}
	normalizePPTXPayload(payload, "enterprise collaboration platform", "", true)
	if len(payload.Slides) < 7 {
		t.Fatalf("slide count = %d, want scaffolded deck", len(payload.Slides))
	}
	if payload.Slides[1].Layout != "toc" {
		t.Fatalf("company toc slide = %#v", payload.Slides[1])
	}
	if payload.Slides[2].Layout != "chapter" {
		t.Fatalf("company first chapter slide = %#v", payload.Slides[2])
	}
	if payload.Slides[len(payload.Slides)-1].Layout != "closing" || len(payload.Slides[len(payload.Slides)-1].Sections) == 0 {
		t.Fatalf("company closing slide = %#v", payload.Slides[len(payload.Slides)-1])
	}
}

func TestNormalizePPTXPayload_EnforcesMarketSkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "market opportunity analysis",
		Slides: []officegen.Slide{
			{Title: "market opportunity analysis", Layout: "title", IsTitle: true},
			{Title: "first slide"},
			{Title: "second slide"},
		},
	}
	normalizePPTXPayload(payload, "market opportunity analysis", "", true)
	if payload.Slides[1].Layout != "toc" {
		t.Fatalf("market toc slide = %#v", payload.Slides[1])
	}
	foundChart := false
	for _, slide := range payload.Slides {
		if slide.Layout == "chart" && slide.Chart != nil {
			foundChart = true
			break
		}
	}
	if !foundChart {
		t.Fatalf("market deck should retain an evidence chart: %#v", payload.Slides)
	}
	if payload.Slides[len(payload.Slides)-1].Layout != "closing" || len(payload.Slides[len(payload.Slides)-1].Sections) == 0 {
		t.Fatalf("market closing slide = %#v", payload.Slides[len(payload.Slides)-1])
	}
}

func TestNormalizePPTXPayload_EnforcesOpsSkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "business review",
		Slides: []officegen.Slide{
			{Title: "business review", Layout: "title", IsTitle: true},
			{Title: "first slide"},
			{Title: "second slide"},
		},
	}
	normalizePPTXPayload(payload, "business review", "", true)
	if payload.Slides[1].Layout != "toc" {
		t.Fatalf("ops toc slide = %#v", payload.Slides[1])
	}
	foundChart := false
	for _, slide := range payload.Slides {
		if slide.Layout == "chart" && slide.Chart != nil {
			foundChart = true
			break
		}
	}
	if !foundChart {
		t.Fatalf("ops deck should retain an evidence chart: %#v", payload.Slides)
	}
	if payload.Slides[len(payload.Slides)-1].Layout != "closing" || len(payload.Slides[len(payload.Slides)-1].Sections) < 2 {
		t.Fatalf("ops closing slide = %#v", payload.Slides[len(payload.Slides)-1])
	}
}

func TestNormalizePPTXPayload_EnforcesTrainingSkeleton(t *testing.T) {
	payload := &pptxPayload{
		Title: "new hire onboarding training",
		Slides: []officegen.Slide{
			{Title: "new hire onboarding training", Layout: "title", IsTitle: true},
			{Title: "first slide"},
			{Title: "second slide"},
		},
	}
	normalizePPTXPayload(payload, "new hire onboarding training", "", true)
	if payload.Slides[1].Layout != "toc" {
		t.Fatalf("training toc slide = %#v", payload.Slides[1])
	}
	if payload.Slides[len(payload.Slides)-1].Layout != "closing" || len(payload.Slides[len(payload.Slides)-1].Sections) < 2 {
		t.Fatalf("training closing slide = %#v", payload.Slides[len(payload.Slides)-1])
	}
}

func TestNormalizePPTXSlide_BusinessClosingUsesDecisionBannerAndDropsBackgroundImage(t *testing.T) {
	coverBudget := 0
	closingBudget := 0
	imageBudget := 1
	galleryBudget := 0
	visualBudget := 0

	slide, imageKept, _ := normalizePPTXSlide(officegen.Slide{
		Title:         "Recommendation",
		Layout:        "closing",
		NarrativeRole: "closing",
		Subtitle:      "Approve the first pilot now.",
		Sections: []officegen.SlideSection{
			{Heading: "Decision", Detail: "Approve the pilot scope this week."},
			{Heading: "Guardrail", Detail: "Keep the first validation cycle limited to one team."},
		},
		HasImage:    true,
		ImagePos:    "background",
		ImagePrompt: "A boardroom hero background",
	}, 5, "OfficeCLI", pptxArchetypeCompany, true, &coverBudget, &closingBudget, &imageBudget, &galleryBudget, &visualBudget)

	if slide.Variant != "closing-decision-banner" {
		t.Fatalf("variant = %q, want closing-decision-banner", slide.Variant)
	}
	if slide.HasImage || imageKept {
		t.Fatalf("business closing should drop background image: %+v kept=%v", slide, imageKept)
	}
}

func TestNormalizePPTXSlide_ExplainerClosingUsesStarterGuidanceAndDropsBackgroundImage(t *testing.T) {
	coverBudget := 0
	closingBudget := 1
	imageBudget := 0
	galleryBudget := 0
	visualBudget := 0

	slide, imageKept, _ := normalizePPTXSlide(officegen.Slide{
		Title:         "How to Start",
		Layout:        "closing",
		NarrativeRole: "closing",
		Subtitle:      "Start small and learn the loop by doing.",
		Sections: []officegen.SlideSection{
			{Heading: "Pick One Mode", Detail: "Creative or Survival."},
			{Heading: "Try One Goal", Detail: "Build one shelter."},
		},
		HasImage:    true,
		ImagePos:    "background",
		ImagePrompt: "A bright voxel landscape at sunset",
	}, 5, "Minecraft", pptxArchetypeExplainer, true, &coverBudget, &closingBudget, &imageBudget, &galleryBudget, &visualBudget)

	if slide.Variant != "closing-starter-guidance" {
		t.Fatalf("variant = %q, want closing-starter-guidance", slide.Variant)
	}
	if slide.HasImage || imageKept {
		t.Fatalf("closing slides should not keep background image: %+v kept=%v", slide, imageKept)
	}
}

func TestDefaultActionSlide_GeneralAvoidsLegacyNextStepsTemplateAndMetaCopy(t *testing.T) {
	slide := defaultActionSlide(pptxArchetypeGeneral, "AI PPT 生成架构升级")
	if slide.Title == "Next Steps" {
		t.Fatalf("general fallback should no longer use legacy title: %+v", slide)
	}
	if slide.Subtitle == "Close with a small set of actions, owners, and validation points" {
		t.Fatalf("general fallback should no longer use legacy subtitle: %+v", slide)
	}
	text := strings.ToLower(slide.Title + " " + slide.Subtitle)
	for _, section := range slide.Sections {
		text += " " + strings.ToLower(section.Heading+" "+section.Detail)
	}
	for _, forbidden := range []string{"close with", "proof point", "proof points needed", "highest-friction document workflow"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("generic closing should not expose scaffold copy %q: %+v", forbidden, slide)
		}
	}
	if slide.Variant != "closing-decision-banner" {
		t.Fatalf("variant = %q, want closing-decision-banner", slide.Variant)
	}
	if len(slide.Sections) != 3 {
		t.Fatalf("sections = %#v, want 3 concrete closing items", slide.Sections)
	}
}

func TestNormalizePPTXPayload_DoesNotBackfillClosingImage(t *testing.T) {
	payload := &pptxPayload{
		Title: "AI PPT 生成架构升级",
		Slides: []officegen.Slide{
			{Title: "AI PPT 生成架构升级", Layout: "title", IsTitle: true},
			{Title: "Core Architecture", Layout: "content", Sections: []officegen.SlideSection{{Heading: "Spec", Detail: "Use semantic slides"}, {Heading: "Renderer", Detail: "Own layout and contrast"}}},
			{Title: "Recommendation", Layout: "closing", Variant: "closing-decision-banner", Sections: []officegen.SlideSection{{Heading: "Decision", Detail: "Run a scoped pilot."}}},
		},
	}
	normalizePPTXPayload(payload, "AI PPT 生成架构升级", "", true)
	if len(payload.Slides) == 0 {
		t.Fatal("slides should not be empty")
	}
	last := payload.Slides[len(payload.Slides)-1]
	if last.Layout != "closing" {
		t.Fatalf("last slide should remain closing: %+v", last)
	}
	if last.HasImage || strings.TrimSpace(last.ImagePrompt) != "" || strings.TrimSpace(last.ImagePos) != "" {
		t.Fatalf("closing slide should not be backfilled with image: %+v", last)
	}
}

func TestServiceGeneratePPTX_GeneratesImagesWhenEnabled(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"product capability overview",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"cover","layout":"title","subtitle":"product context and business status","isTitle":true},
				{"title":"product capabilities","layout":"content","points":["Multi-user collaboration","Real-time editing","Enterprise administration"],"hasImage":true,"imagePrompt":"A modern collaboration workspace, a bright meeting room, and several people reviewing documents around a large display","imagePos":"right"}
			]
		}`,
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Describe the product capabilities, customer value, and use cases of this knowledge collaboration product.",
		Topic:        "Knowledge Collaboration Product Overview",
		Mode:         "fast",
		EnableImages: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.imageCalls != 2 {
		t.Fatalf("imageCalls = %d, want 2", llm.imageCalls)
	}
	if !archiveContainsEntryWithSubstring(t, doc.Bytes, "ppt/slides/_rels/", ".rels", `relationships/image`) {
		t.Fatalf("deck rels missing image relationship")
	}
	if len(doc.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", doc.Warnings)
	}
}

func TestServiceGeneratePPTX_SkipsImagesWhenDisabled(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"product capability overview",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"cover","layout":"title","subtitle":"product context and business status","isTitle":true},
				{"title":"product capabilities","layout":"content","points":["Multi-user collaboration","Real-time editing","Enterprise administration"],"hasImage":true,"imagePrompt":"A modern collaboration workspace, a bright meeting room, and several people reviewing documents around a large display","imagePos":"right"}
			]
		}`,
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Describe the product capabilities, customer value, and use cases of this knowledge collaboration product.",
		Topic:        "Knowledge Collaboration Product Overview",
		Mode:         "fast",
		EnableImages: false,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.imageCalls != 0 {
		t.Fatalf("imageCalls = %d, want 0", llm.imageCalls)
	}
	if archiveContainsEntryWithSubstring(t, doc.Bytes, "ppt/slides/_rels/", ".rels", `relationships/image`) {
		t.Fatalf("deck rels should not include image relationship when disabled")
	}
}

func TestServiceGeneratePPTX_DegradesGracefullyWhenImageGenerationFails(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponse: `{
			"title":"product capability overview",
			"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
			"slides":[
				{"title":"cover","layout":"title","subtitle":"product context and business status","isTitle":true},
				{"title":"product capabilities","layout":"content","points":["Multi-user collaboration","Real-time editing","Enterprise administration"],"hasImage":true,"imagePrompt":"A modern collaboration workspace, a bright meeting room, and several people reviewing documents around a large display","imagePos":"right"}
			]
		}`,
		imageErr: errors.New("image backend unavailable"),
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Describe the product capabilities, customer value, and use cases of this knowledge collaboration product.",
		Topic:        "Knowledge Collaboration Product Overview",
		Mode:         "fast",
		EnableImages: true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.imageCalls != 2 {
		t.Fatalf("imageCalls = %d, want 2", llm.imageCalls)
	}
	if len(doc.Warnings) == 0 {
		t.Fatalf("warnings = %#v, want degradation warning", doc.Warnings)
	}
	if got := doc.Warnings[0].Message; !strings.Contains(got, "PPT images failed to generate through the hosted image route") {
		t.Fatalf("warning = %q", got)
	}
	if got := doc.Warnings[0].Message; !strings.Contains(got, "officecli login") {
		t.Fatalf("warning should include hosted login guidance: %q", got)
	}
	if archiveContainsEntryWithSubstring(t, doc.Bytes, "ppt/slides/_rels/", ".rels", `relationships/image`) {
		t.Fatalf("deck rels should not include image relationship after degradation")
	}
}

func TestBuildPPTXFromJSON_GeneratesGalleryVisuals(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"Visual Gallery Demo",
		"slides":[
			{"title":"Visual Gallery Demo","layout":"title","variant":"title-center","subtitle":"Open with the topic"},
			{"title":"Product Scenes","layout":"gallery","variant":"gallery","narrativeRole":"analysis","sectionIndex":1,"sectionTitle":"Core Storyline","subtitle":"Use visuals to show the product context","visuals":[
				{"label":"Workspace","prompt":"A modern collaboration workspace with documents and comments","caption":"Workspace view"},
				{"label":"Meeting","prompt":"A product review meeting around a large display with dashboard UI","caption":"Review scene"}
			]}
		]
	}`

	fileBytes, _, warnings, _, _, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "Visual Gallery Demo", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if llm.imageCalls < 1 {
		t.Fatalf("imageCalls = %d, want at least 1", llm.imageCalls)
	}
	// premium-only 模式 visualBudget=1，可能产生 rebalance 通知，但不应有 degraded 警告。
	for _, w := range warnings {
		if w.Code == "WARN_PPT_IMAGE_DEGRADED" {
			t.Fatalf("unexpected degraded warning: %#v", warnings)
		}
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got < 1 {
		t.Fatalf("image count = %d, want at least 1", got)
	}
}

func TestBuildPPTXFromJSON_AcceptsSemanticPayload(t *testing.T) {
	content := `{
		"title":"Enterprise Collaboration Platform",
		"subtitle":"Board-ready narrative",
		"stylePreset":"executive-dark",
		"slides":[
			{"role":"cover","headline":"Enterprise Collaboration Platform","takeaway":"A concise board-ready story"},
			{"role":"summary","headline":"Readiness Snapshot","takeaway":"The platform is ready when value, governance, and rollout are aligned.","blocks":[{"type":"sections","sections":[
				{"heading":"Value","detail":"Teams reduce coordination drag through one shared workspace."},
				{"heading":"Governance","detail":"Permissions and audit trails keep enterprise controls visible."},
				{"heading":"Rollout","detail":"Start with a focused department before broad expansion."}
			]}]},
			{"role":"evidence","headline":"Adoption Evidence","takeaway":"The strongest signal is cross-team activation, not isolated usage.","blocks":[{"type":"chart","chart":{"title":"Activation Index","type":"bar","categories":["Pilot","Expansion","Scaled"],"values":[32,58,81]},"items":["Expansion cohorts show higher activation.","Scaled teams sustain the highest usage."]}]},
			{"role":"action","headline":"Decision Path","takeaway":"Approve a staged rollout with explicit owners and acceptance criteria.","blocks":[{"type":"actions","items":["Confirm pilot owner this month","Measure activation and governance readiness","Expand only after two validation cycles"]}]},
			{"role":"closing","headline":"Closing Decision","takeaway":"Move forward with a controlled rollout, not a broad-bang launch.","blocks":[{"type":"sections","sections":[
				{"heading":"Ask","detail":"Approve the pilot-to-scale path."},
				{"heading":"Guardrail","detail":"Review adoption and control metrics before expansion."}
			]}]}
		]
	}`

	fileBytes, _, _, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Enterprise Collaboration Platform", "", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	for _, needle := range []string{"Readiness Snapshot", "Adoption Evidence", "Decision Path", "Closing Decision"} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", needle) {
			t.Fatalf("semantic deck should preserve headline %q", needle)
		}
		if !strings.Contains(string(previewJSON), needle) {
			t.Fatalf("preview json missing semantic headline %q:\n%s", needle, string(previewJSON))
		}
	}
	if !strings.Contains(string(previewJSON), `"layout": "chart"`) {
		t.Fatalf("semantic evidence block should map to chart layout:\n%s", string(previewJSON))
	}
	if !strings.Contains(string(previewJSON), `"layout": "closing"`) {
		t.Fatalf("semantic action or closing role should map to closing layout:\n%s", string(previewJSON))
	}
}

func TestBuildPPTXFromJSON_DefaultBackendDoesNotUseArtifactWorker(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(context.Context, pptxArtifactWorkerRequest, string) (*pptxArtifactWorkerOutput, error) {
		t.Fatal("artifact worker should not run for default go-spine backend")
		return nil, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Default Backend Demo",
		"slides":[
			{"title":"Default Backend Demo","layout":"title","subtitle":"Go spine is default","isTitle":true},
			{"title":"Body","layout":"content","points":["Default path","No worker"]}
		]
	}`
	fileBytes, fileName, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Default Backend Demo", "", false, false, PPTXBuildOptions{})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if len(fileBytes) == 0 || fileName == "" {
		t.Fatalf("empty output: bytes=%d fileName=%q", len(fileBytes), fileName)
	}
}

func TestBuildPPTXFromJSON_DefaultGoSpineUsesEditableChartFallback(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(context.Context, pptxArtifactWorkerRequest, string) (*pptxArtifactWorkerOutput, error) {
		t.Fatal("artifact worker should not run for default go-spine backend")
		return nil, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Go Spine Chart Demo",
		"slides":[
			{"title":"Go Spine Chart Demo","layout":"title","subtitle":"Editable chart fallback","isTitle":true},
			{"title":"Signal","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Text","Chart"],"values":[4,2]},"points":["Chart remains editable as shapes"]}
		]
	}`
	fileBytes, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Go Spine Chart Demo", "", false, false, PPTXBuildOptions{})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if countZipEntries(fileBytes, "ppt/charts/", ".xml") != 0 {
		t.Fatal("go-spine default should not emit native chart XML")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "shape fallback") {
		t.Fatal("go-spine chart fallback marker missing")
	}
	if !containsIssueCode(warnings, "WARN_PPTX_NATIVE_CHART_FALLBACK") {
		t.Fatalf("warnings = %#v, want native chart fallback warning", warnings)
	}
}

func TestBuildPPTXFromJSON_ArtifactExperimentalAliasesGoSpine(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(context.Context, pptxArtifactWorkerRequest, string) (*pptxArtifactWorkerOutput, error) {
		t.Fatal("artifact worker should not run for deprecated artifact-experimental alias")
		return nil, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Alias Demo",
		"slides":[
			{"title":"Alias Demo","layout":"title","subtitle":"Alias routes to go-spine","isTitle":true},
			{"title":"Body","layout":"content","points":["No Node worker"]}
		]
	}`
	fileBytes, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Alias Demo", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactExperimental,
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if len(fileBytes) == 0 {
		t.Fatal("empty output")
	}
	if !containsIssueCode(warnings, "WARN_PPTX_BACKEND_DEPRECATED") {
		t.Fatalf("warnings = %#v, want deprecated backend warning", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalUsesWorker(t *testing.T) {
	original := runPPTXArtifactWorker
	workerCalled := false
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		workerCalled = true
		if req.StylePreset != officegen.StylePresetTrainingManual {
			t.Fatalf("StylePreset = %q, want %q", req.StylePreset, officegen.StylePresetTrainingManual)
		}
		if len(req.Slides) < 2 {
			t.Fatalf("Slides = %d, want normalized deck with at least 2 slides", len(req.Slides))
		}
		if req.StyleBrief == nil || req.StyleBrief.StylePresetHint != officegen.StylePresetTrainingManual {
			t.Fatalf("StyleBrief = %#v", req.StyleBrief)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 1, 0)
		output.Warnings = []string{"fake worker warning"}
		return output, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Backend Demo",
		"stylePreset":"training-manual",
		"slides":[
			{"title":"Artifact Backend Demo","layout":"title","subtitle":"Worker route","isTitle":true},
			{"title":"Body","layout":"content","points":["Editable text","Worker output"]}
		]
	}`
	fileBytes, fileName, warnings, previewHTML, previewJSON, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Backend Demo", officegen.StylePresetTrainingManual, false, true, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: officegen.StylePresetTrainingManual,
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if !workerCalled {
		t.Fatal("artifact worker was not called")
	}
	if len(fileBytes) == 0 || !strings.HasSuffix(fileName, ".pptx") {
		t.Fatalf("output = bytes %d, file %q", len(fileBytes), fileName)
	}
	if len(previewHTML) == 0 || len(previewJSON) == 0 {
		t.Fatal("expected local preview sidecars for artifact backend")
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_WORKER") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalAddsNativeChartWhenPromptRequiresOne(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 4, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Board Update Smoke",
		"slides":[
			{"title":"Board Update Smoke","layout":"title","subtitle":"Product analytics team update","isTitle":true},
			{"title":"Operating Metrics","layout":"dashboard","metrics":[
				{"label":"Activation","value":"82","note":"Index"},
				{"label":"Retention","value":"74","note":"Index"},
				{"label":"Velocity","value":"88","note":"Index"}
			]},
			{"title":"Strategic Risks","layout":"content","sections":[
				{"heading":"Data Quality","detail":"Instrumentation drift can hide real activation issues."},
				{"heading":"Adoption","detail":"Workflow friction slows repeat usage."}
			]},
			{"title":"Decision Request","layout":"closing","content":"Approve the next product analytics focus area."}
		]
	}`
	fileBytes, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Board Update Smoke", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise board update presentation for a product analytics team. Include a cover slide, three operating metrics, one simple chart, strategic risks, and a closing decision slide.",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	chartSlides := 0
	for _, slide := range captured.Slides {
		if slide.Chart != nil {
			chartSlides++
		}
	}
	if chartSlides == 0 {
		t.Fatalf("captured worker request has no chart slides: %+v", captured.Slides)
	}
	if countPPTXChartXMLParts(fileBytes) == 0 {
		t.Fatal("artifact backend output should contain a native chart XML entry")
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalHonorsExplicitLightStyleOverDarkReferenceBrief(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 4, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Light Style Sample",
		"stylePreset":"editorial-light",
		"slides":[
			{"title":"Light Style Sample","layout":"title","subtitle":"A light editorial reference-learning deck.","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["White canvas","Soft cards","Thin borders"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Light style signal","categories":["Canvas","Cards","Chart"],"values":[88,82,86]}},
			{"title":"Closing","layout":"closing","content":"Use light style intent when it is explicit."}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Light Style Sample", officegen.StylePresetEditorialLight, false, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable light-theme presentation. Use a clean light editorial style: white canvas, soft cards, thin borders, teal or blue accents, and generous whitespace. Include a cover slide, key observations, one simple native chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: officegen.StylePresetExecutiveDark,
			PaletteIntent:   "dark neutral palette with cyan and amber accents",
			LayoutRhythm:    "dark cards, clear hierarchy, restrained density",
		},
		GenerateArtifactDesignPlan: true,
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if captured.StylePreset != officegen.StylePresetEditorialLight {
		t.Fatalf("worker StylePreset = %q, want explicit %q", captured.StylePreset, officegen.StylePresetEditorialLight)
	}
	if captured.DesignPlan == nil {
		t.Fatal("expected artifact design plan")
	}
	if captured.DesignPlan.StyleBias != "editorial-light" {
		t.Fatalf("DesignPlan.StyleBias = %q, want editorial-light despite dark reference brief", captured.DesignPlan.StyleBias)
	}
}

func TestServiceGeneratePPTXArtifactExperimentalUsesStableReferencePaletteFallback(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		if req.StylePreset != officegen.StylePresetProjectForest {
			t.Fatalf("StylePreset = %q, want %q from stable reference palette", req.StylePreset, officegen.StylePresetProjectForest)
		}
		if req.StyleBrief == nil || req.StyleBrief.StylePresetHint != officegen.StylePresetProjectForest {
			t.Fatalf("StyleBrief = %#v, want project-forest from stable reference palette", req.StyleBrief)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactReferenceLearningFakeDiagnostics(t, req, 6, 1)
		output.WorkerVersion = "artifact-experimental-test"
		return output, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	root := t.TempDir()
	referencePath := filepath.Join(root, "brand-reference.pptx")
	writeRuntimeReferenceDeck(t, referencePath, "Forest Reference", "Aptos")
	injectRuntimeSlideColors(t, referencePath, []string{"14532D", "4D7C0F"})

	planResponse := `{
		"deckIntent":"stable-reference-style-learning",
		"styleBias":"project-forest reference palette",
		"builderRecipe":"codex-reference-learning",
		"builderPatch":{"slides":[]},
		"slides":[
			{"slide":1,"role":"cover","layoutMode":"cover-split-visual","composition":"split","visualTreatment":"native-shapes","densityTarget":"spacious","kicker":"","displayTitle":"Reference Palette Demo","displaySubtitle":"Stable reference palette drives the renderer.","displayBody":"","takeaway":"","visualIntent":"Use forest-toned panels.","cards":[],"chartCallouts":[]},
			{"slide":2,"role":"observations","layoutMode":"observation-cards","composition":"three-cards","visualTreatment":"native-shapes","densityTarget":"balanced","kicker":"KEY OBSERVATIONS","displayTitle":"Stable references outrank generated outputs","displaySubtitle":"","displayBody":"","takeaway":"","visualIntent":"Use three cards.","cards":[{"heading":"Stable deck","detail":"Root reference deck supplies palette intent."},{"heading":"Editable objects","detail":"Text remains editable."},{"heading":"Quality loop","detail":"Preview checks stay active."}],"chartCallouts":[]},
			{"slide":3,"role":"evidence","layoutMode":"chart-insight-stack","composition":"chart-with-callouts","visualTreatment":"native-chart","densityTarget":"balanced","kicker":"EVIDENCE","displayTitle":"Reference signals become a native chart","displaySubtitle":"","displayBody":"","takeaway":"","visualIntent":"Use a native chart.","cards":[],"chartCallouts":[{"heading":"Palette","detail":"Stable colors affect preset selection."},{"heading":"Structure","detail":"Charts stay editable."}]},
			{"slide":4,"role":"closing","layoutMode":"closing-takeaway","composition":"split-callout","visualTreatment":"native-shapes","densityTarget":"spacious","kicker":"RECOMMENDATION","displayTitle":"Use stable references as intent","displaySubtitle":"","displayBody":"Keep the renderer in control of editable structure.","takeaway":"","visualIntent":"Use a closing callout.","cards":[],"chartCallouts":[]}
		]
	}`
	llm := &fakeLLMClient{
		structuredResponses: []string{
			`not json`,
			planResponse,
			planResponse,
		},
		jsonResponse: `{
			"title":"Reference Palette Demo",
			"slides":[
				{"title":"Reference Palette Demo","layout":"title","subtitle":"Stable reference palette","isTitle":true},
				{"title":"Key Observations","layout":"content","points":["Stable references outrank generated outputs","Important text stays editable","Preview checks stay active"]},
				{"title":"Signal Chart","layout":"chart","points":["Palette and structure contribute"],"chart":{"type":"bar","title":"Reference signal","categories":["Palette","Structure"],"values":[92,88]}},
				{"title":"Closing","layout":"closing","content":"Use stable references as renderer intent."}
			]
		}`,
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType:         engine.DocumentTypePPTX,
		Prompt:               "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		Topic:                "Reference Palette Demo",
		ReferenceScanEnabled: true,
		ReferenceScanRoot:    root,
		PPTXBackend:          PPTXBackendArtifactWorker,
		LocalPreview:         true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if captured.StyleBrief == nil {
		t.Fatal("worker did not receive style brief")
	}
	if doc.ReferenceStyle == nil || doc.ReferenceStyle.StyleBrief == nil {
		t.Fatalf("reference style missing from generated artifact: %#v", doc.ReferenceStyle)
	}
	if doc.ReferenceStyle.StyleBrief.StylePresetHint != officegen.StylePresetProjectForest {
		t.Fatalf("metadata style brief = %#v, want project-forest", doc.ReferenceStyle.StyleBrief)
	}
	if !containsString(doc.ReferenceStyle.ThemeColors, "14532D") || !containsString(doc.ReferenceStyle.ThemeColors, "4D7C0F") {
		t.Fatalf("reference theme colors = %#v, want injected stable slide colors", doc.ReferenceStyle.ThemeColors)
	}
	if !containsIssueCode(doc.Warnings, "WARN_PPTX_REFERENCE_STYLE_BRIEF_FALLBACK") {
		t.Fatalf("warnings = %#v, want deterministic fallback warning from invalid reference brief response", doc.Warnings)
	}
	if len(doc.Bytes) == 0 {
		t.Fatal("generated artifact has no bytes")
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalPassesVisualAssetsAndChartIntent(t *testing.T) {
	root := t.TempDir()
	assetPath := filepath.Join(root, "reference-visual.png")
	if err := os.WriteFile(assetPath, []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
		0x42, 0x60, 0x82,
	}, 0o644); err != nil {
		t.Fatalf("write reference asset: %v", err)
	}

	original := runPPTXArtifactWorker
	workerCalled := false
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		workerCalled = true
		if len(req.VisualAssets) == 0 {
			t.Fatalf("VisualAssets is empty; artifact backend should pass local image assets to the worker")
		}
		if req.VisualAssets[0].Path != assetPath {
			t.Fatalf("VisualAssets[0].Path = %q, want %q", req.VisualAssets[0].Path, assetPath)
		}
		foundChart := false
		for _, slide := range req.Slides {
			if slide.Chart != nil {
				foundChart = true
				break
			}
		}
		if !foundChart {
			t.Fatalf("worker request did not preserve chart intent: %#v", req.Slides)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 1, 1)
		output.VisualAssets = 1
		return output, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Backend Visual Assets",
		"slides":[
			{"title":"Artifact Backend Visual Assets","layout":"title","subtitle":"Worker route","isTitle":true},
			{"title":"Quality Chart","layout":"chart","points":["Use a native chart"],"chart":{"type":"bar","title":"Quality signals","categories":["Assets","Charts"],"values":[1,1]}}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Backend Visual Assets", "", true, false, PPTXBuildOptions{
		Backend:           PPTXBackendArtifactWorker,
		ReferenceScanRoot: root,
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if !workerCalled {
		t.Fatal("artifact worker was not called")
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalDoesNotImportGeneratedOutputReferences(t *testing.T) {
	original := runPPTXArtifactWorker
	workerCalled := false
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		workerCalled = true
		if len(req.ReferenceFiles) != 0 {
			t.Fatalf("ReferenceFiles = %#v, want no current-output references imported into artifact worker", req.ReferenceFiles)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 1, 0), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Generated Output References",
		"slides":[
			{"title":"Generated Output References","layout":"title","subtitle":"Worker route","isTitle":true},
			{"title":"Body","layout":"content","points":["Editable text","No generated-output imports"]}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Generated Output References", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
		ReferenceProfile: &pptxref.ReferenceStyleProfile{
			SourceFiles: []pptxref.ReferencePPTXFile{
				{Path: "/root/output/generated-a.pptx", SourceBucket: "current-output"},
				{Path: "/root/output/generated-b.pptx", SourceBucket: "current-output"},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if !workerCalled {
		t.Fatal("artifact worker was not called")
	}
}

func TestBuildPPTXFromJSON_CompactsExplicitFourSlideArtifactRequest(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		if len(req.Slides) != 4 {
			t.Fatalf("Slides = %d, want explicit four-slide request compacted to 4", len(req.Slides))
		}
		if req.Slides[0].Layout != "title" {
			t.Fatalf("slide 1 layout = %q, want title", req.Slides[0].Layout)
		}
		if req.Slides[0].Title != "Explicit Four Slide Demo" {
			t.Fatalf("slide 1 title = %q, want deck title", req.Slides[0].Title)
		}
		if req.Slides[1].Title != "Key Observations" {
			t.Fatalf("slide 2 title = %q, want Key Observations", req.Slides[1].Title)
		}
		if req.Slides[2].Chart == nil {
			t.Fatalf("slide 3 should preserve chart intent: %#v", req.Slides)
		}
		if req.Slides[3].Layout != "closing" {
			t.Fatalf("slide 4 layout = %q, want closing", req.Slides[3].Layout)
		}
		if req.Slides[3].Title != "Closing" {
			t.Fatalf("slide 4 title = %q, want Closing", req.Slides[3].Title)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 1, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Explicit Four Slide Demo",
		"slides":[
			{"title":"Explicit Four Slide Demo","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Context","layout":"content","points":["Context"]},
			{"title":"Key Observations","layout":"content","points":["Observation 1","Observation 2"]},
			{"title":"More Detail","layout":"content","points":["Detail"]},
			{"title":"Simple Chart","layout":"chart","points":["Chart point"],"chart":{"type":"bar","title":"Simple signal","categories":["A","B"],"values":[1,2]}},
			{"title":"Evidence Detail","layout":"content","points":["Evidence"]},
			{"title":"Next Steps","layout":"content","points":["Step"]},
			{"title":"Closing","layout":"closing","points":["Close"]}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Explicit Four Slide Demo", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation. Include a cover slide, key observations, one simple chart, and a closing slide.",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalPassesDesignPlan(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		if req.DesignPlan == nil {
			t.Fatal("DesignPlan is nil; artifact backend should pass a task-specific design plan to the worker")
		}
		if req.DesignPlan.DeckIntent != "concise-reference-style-learning" {
			t.Fatalf("DesignPlan.DeckIntent = %q, want concise-reference-style-learning", req.DesignPlan.DeckIntent)
		}
		wantModes := []string{"cover-split-visual", "observation-cards", "chart-insight-stack", "closing-takeaway"}
		if len(req.DesignPlan.Slides) != len(wantModes) {
			t.Fatalf("DesignPlan.Slides = %#v, want %d slides", req.DesignPlan.Slides, len(wantModes))
		}
		for idx, want := range wantModes {
			if req.DesignPlan.Slides[idx].Slide != idx+1 {
				t.Fatalf("DesignPlan.Slides[%d].Slide = %d, want %d", idx, req.DesignPlan.Slides[idx].Slide, idx+1)
			}
			if req.DesignPlan.Slides[idx].LayoutMode != want {
				t.Fatalf("DesignPlan.Slides[%d].LayoutMode = %q, want %q; plan=%#v", idx, req.DesignPlan.Slides[idx].LayoutMode, want, req.DesignPlan)
			}
		}
		if req.DesignPlan.Slides[1].Kicker != "KEY OBSERVATIONS" {
			t.Fatalf("observation kicker = %q, want KEY OBSERVATIONS", req.DesignPlan.Slides[1].Kicker)
		}
		if text := pptxArtifactDesignPlanVisibleText(req.DesignPlan); containsPPTXArtifactImplementationNarrative(text) {
			t.Fatalf("DesignPlan leaks implementation narrative into visible copy: %q", text)
		}
		if req.DesignPlan.Slides[2].Kicker != "SIMPLE CHART" {
			t.Fatalf("chart kicker = %q, want SIMPLE CHART", req.DesignPlan.Slides[2].Kicker)
		}
		if req.DesignPlan.Slides[3].Kicker != "Recommendation" {
			t.Fatalf("closing kicker = %q, want Recommendation", req.DesignPlan.Slides[3].Kicker)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 1, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Explicit Four Slide Demo",
		"slides":[
			{"title":"Explicit Four Slide Demo","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Observation 1","Observation 2"]},
			{"title":"Simple Chart","layout":"chart","points":["Chart point"],"chart":{"type":"bar","title":"Simple signal","categories":["A","B"],"values":[1,2]}},
			{"title":"Closing","layout":"closing","points":["Close"]}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Explicit Four Slide Demo", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
}

func pptxArtifactDesignPlanVisibleText(plan *pptxArtifactDesignPlan) string {
	if plan == nil {
		return ""
	}
	var parts []string
	for _, slide := range plan.Slides {
		parts = append(parts, slide.Kicker, slide.DisplayTitle, slide.DisplaySubtitle, slide.DisplayBody, slide.Takeaway)
		for _, card := range slide.Cards {
			parts = append(parts, card.Heading, card.Detail)
		}
		for _, callout := range slide.ChartCallouts {
			parts = append(parts, callout.Heading, callout.Detail)
		}
	}
	return strings.Join(parts, "\n")
}

func containsPPTXArtifactImplementationNarrative(text string) bool {
	value := strings.ToLower(text)
	for _, token := range []string{"codex", "officecli", "worker", "artifact", "builder", "preview", "previews", "patch", "rendered evidence", "visual qa", "agent loop", "implementation"} {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalDefaultsReferenceLearningToDarkPreset(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		if req.DesignPlan == nil || req.DesignPlan.DeckIntent != "concise-reference-style-learning" {
			t.Fatalf("DesignPlan = %#v, want reference-learning plan", req.DesignPlan)
		}
		if req.StylePreset != officegen.StylePresetExecutiveDark {
			t.Fatalf("StylePreset = %q, want %q for reference-learning fallback", req.StylePreset, officegen.StylePresetExecutiveDark)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 1, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Observation 1","Observation 2"]},
			{"title":"Simple Chart","layout":"chart","points":["Chart point"],"chart":{"type":"bar","title":"Simple signal","categories":["A","B"],"values":[1,2]}},
			{"title":"Closing","layout":"closing","points":["Close"]}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalUsesLLMDesignPlanWhenEnabled(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 1, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"LLM Design Plan Demo",
		"slides":[
			{"title":"LLM Design Plan Demo","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Observation 1","Observation 2"]},
			{"title":"Simple Chart","layout":"chart","points":["Chart point"],"chart":{"type":"bar","title":"Simple signal","categories":["A","B"],"values":[1,2]}},
			{"title":"Closing","layout":"closing","points":["Close"]}
		]
	}`
	llm := &fakeLLMClient{structuredResponse: `{
		"deckIntent":"llm-bespoke-reference-builder",
		"styleBias":"dark-structured",
		"slides":[
			{"slide":1,"role":"cover","layoutMode":"cover-split-visual","visualTreatment":"reference-visual-panel","densityTarget":"spacious","kicker":"","takeaway":"Plan-made cover note","visualIntent":"Use a bold reference visual."},
			{"slide":2,"role":"observations","layoutMode":"observation-cards","visualTreatment":"native-shapes","densityTarget":"balanced","kicker":"LLM OBSERVATIONS","takeaway":"Plan-made observation takeaway.","visualIntent":"Use three planned cards.","cards":[
				{"heading":"Repeatable style beats single-slide mimicry","detail":"Use recurring panels and accent rules rather than copying one output deck."},
				{"heading":"Important content stays editable","detail":"Keep slide words, labels, metrics, and callouts as selectable objects."},
				{"heading":"Visual QA changes the final design","detail":"Rendered previews catch overflow, weak contrast, blank pages, and chart defaults."}
			]},
			{"slide":3,"role":"evidence","layoutMode":"chart-insight-stack","visualTreatment":"native-chart","densityTarget":"spacious","kicker":"LLM CHART","takeaway":"","visualIntent":"Use a bespoke native chart.","chartCallouts":[
				{"heading":"Why this matters","detail":"The builder should compose around the task, not only semantic JSON."},
				{"heading":"Hard gate","detail":"Fail on missing previews, native charts, or structural regressions."}
			]},
			{"slide":4,"role":"closing","layoutMode":"closing-takeaway","visualTreatment":"native-shapes","densityTarget":"compact","kicker":"LLM CLOSE","takeaway":"Plan-made closing note.","visualIntent":"Use a closing support panel."}
		]
	}`}
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, nil, content, "LLM Design Plan Demo", "", true, false, PPTXBuildOptions{
		Backend:                    PPTXBackendArtifactWorker,
		UserPrompt:                 "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		GenerateArtifactDesignPlan: true,
		ReferenceBrief:             &PPTXReferenceStyleBrief{StylePresetHint: "executive-dark", PaletteIntent: "dark neutral"},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if llm.structuredCallCount != 3 {
		t.Fatalf("structured calls = %d, want initial plan plus two preview-informed polish plan attempts", llm.structuredCallCount)
	}
	if llm.lastStructuredReq.Schema.Name != "pptx_artifact_design_plan_polish" {
		t.Fatalf("schema name = %q, want pptx_artifact_design_plan_polish", llm.lastStructuredReq.Schema.Name)
	}
	if captured.DesignPlan == nil {
		t.Fatal("captured design plan is nil")
	}
	if !captured.StrictVisualQuality {
		t.Fatal("StrictVisualQuality is false; artifact backend should enable strict visual/content verdict when LLM design planning is active")
	}
	if captured.DesignPlan.DeckIntent != "concise-reference-style-learning" {
		t.Fatalf("deck intent = %q, want deterministic reference-learning intent", captured.DesignPlan.DeckIntent)
	}
	if captured.DesignPlan.Slides[1].Kicker != "LLM OBSERVATIONS" {
		t.Fatalf("observation kicker = %q, want LLM OBSERVATIONS", captured.DesignPlan.Slides[1].Kicker)
	}
	if captured.DesignPlan.Slides[2].Kicker != "LLM CHART" {
		t.Fatalf("chart kicker = %q, want LLM CHART", captured.DesignPlan.Slides[2].Kicker)
	}
	if got := captured.DesignPlan.Slides[1].Cards[0].Heading; got != "Repeatable style beats single-slide mimicry" {
		t.Fatalf("first planned observation card heading = %q", got)
	}
	if got := captured.DesignPlan.Slides[2].ChartCallouts[0].Detail; got != "The builder should compose around the task." {
		t.Fatalf("first planned chart callout detail = %q", got)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalFallsBackWhenLLMDesignPlanInvalid(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 1, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Fallback Design Plan Demo",
		"slides":[
			{"title":"Fallback Design Plan Demo","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Observation 1","Observation 2"]},
			{"title":"Simple Chart","layout":"chart","points":["Chart point"],"chart":{"type":"bar","title":"Simple signal","categories":["A","B"],"values":[1,2]}},
			{"title":"Closing","layout":"closing","points":["Close"]}
		]
	}`
	llm := &fakeLLMClient{structuredResponse: `{"deckIntent": 123}`}
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, nil, content, "Fallback Design Plan Demo", "", true, false, PPTXBuildOptions{
		Backend:                    PPTXBackendArtifactWorker,
		UserPrompt:                 "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		GenerateArtifactDesignPlan: true,
		ReferenceBrief:             &PPTXReferenceStyleBrief{StylePresetHint: "executive-dark", PaletteIntent: "dark neutral"},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if captured.DesignPlan == nil || captured.DesignPlan.DeckIntent != "concise-reference-style-learning" {
		t.Fatalf("captured plan = %#v, want deterministic fallback plan", captured.DesignPlan)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_DESIGN_PLAN_FALLBACK") {
		t.Fatalf("warnings = %+v, want design plan fallback warning", warnings)
	}
}

func TestNormalizePPTXArtifactDesignPlanCompactsChartCalloutsForNarrowCards(t *testing.T) {
	fallback := &pptxArtifactDesignPlan{
		DeckIntent: "concise-reference-style-learning",
		StyleBias:  "dark-structured",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "evidence", LayoutMode: "chart-insight-stack", Composition: "chart-with-side-insights"},
		},
	}
	generated := pptxArtifactDesignPlan{
		DeckIntent: "concise-reference-style-learning",
		StyleBias:  "dark-structured",
		Slides: []pptxArtifactSlideDesignPlan{{
			Slide:      1,
			Role:       "evidence",
			LayoutMode: "chart-insight-stack",
			ChartCallouts: []pptxArtifactPlanCard{{
				Heading: "Prioritize recurring cues",
				Detail:  "The most reliable signal should lead the composition and visual emphasis across the whole slide.",
			}},
		}},
	}
	plan, err := normalizePPTXArtifactDesignPlan(generated, fallback)
	if err != nil {
		t.Fatalf("normalizePPTXArtifactDesignPlan: %v", err)
	}
	if got := utf8.RuneCountInString(plan.Slides[0].ChartCallouts[0].Detail); got > 44 {
		t.Fatalf("chart callout detail runes = %d, want <= 44: %q", got, plan.Slides[0].ChartCallouts[0].Detail)
	}
}

func TestNormalizePPTXArtifactDesignPlanReferenceLearningPreservesStableTitles(t *testing.T) {
	fallback := &pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		StyleBias:     "dark-structured",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", DisplayTitle: "PPT Reference Style Test", DisplaySubtitle: "Style cues guide palette, rhythm, hierarchy."},
			{Slide: 2, Role: "observations", LayoutMode: "observation-cards", DisplayTitle: "What the reference directory actually teaches", DisplaySubtitle: "System, not template."},
			{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", DisplayTitle: "Fidelity comes from multiple enforced layers", DisplaySubtitle: "The chart stays native and editable."},
			{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", DisplayTitle: "Reference style becomes a reusable system", DisplayBody: "Carry palette, rhythm, and hierarchy into a concise deck while keeping the message clear."},
		},
	}
	generated := pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		StyleBias:     "dark-structured",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", DisplayTitle: "Generic Style Deck", DisplaySubtitle: "No stable reference layout is available."},
			{Slide: 2, Role: "observations", LayoutMode: "observation-cards", DisplayTitle: "Key Observations", DisplaySubtitle: "The learned style shows up."},
			{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", DisplayTitle: "Most Common Style Signals", DisplaySubtitle: "Generic chart context."},
			{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", DisplayTitle: "Use the Style Cues in New Decks", DisplayBody: "Use these reference cues as guidance for future decks."},
		},
	}
	plan, err := normalizePPTXArtifactDesignPlan(generated, fallback)
	if err != nil {
		t.Fatalf("normalizePPTXArtifactDesignPlan: %v", err)
	}
	for idx, slide := range plan.Slides {
		if slide.DisplayTitle != fallback.Slides[idx].DisplayTitle {
			t.Fatalf("slide %d display title = %q, want stable %q", idx+1, slide.DisplayTitle, fallback.Slides[idx].DisplayTitle)
		}
		if slide.DisplaySubtitle != fallback.Slides[idx].DisplaySubtitle {
			t.Fatalf("slide %d display subtitle = %q, want stable %q", idx+1, slide.DisplaySubtitle, fallback.Slides[idx].DisplaySubtitle)
		}
		if slide.DisplayBody != fallback.Slides[idx].DisplayBody {
			t.Fatalf("slide %d display body = %q, want stable %q", idx+1, slide.DisplayBody, fallback.Slides[idx].DisplayBody)
		}
	}
}

func TestNormalizePPTXArtifactDesignPlanReferenceLearningAcceptsStrongTaskSpecificCopy(t *testing.T) {
	fallback := &pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		StyleBias:     "dark-structured",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", DisplayTitle: "PPT Reference Style Test", DisplaySubtitle: "Style cues guide palette, rhythm, hierarchy."},
			{Slide: 2, Role: "observations", LayoutMode: "observation-cards", DisplayTitle: "What the reference directory actually teaches", DisplaySubtitle: "System, not template."},
			{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", DisplayTitle: "Fidelity comes from multiple enforced layers", DisplaySubtitle: "The chart stays native and editable."},
			{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", DisplayTitle: "Reference style becomes a reusable system", DisplayBody: "Carry palette, rhythm, and hierarchy into a concise deck while keeping the message clear."},
		},
	}
	generated := pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		StyleBias:     "dark-structured",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", DisplayTitle: "Generic Style Deck", DisplaySubtitle: "No stable reference layout is available."},
			{Slide: 2, Role: "observations", LayoutMode: "observation-cards", DisplayTitle: "The reference set teaches a visual system", DisplaySubtitle: "Palette, density, hierarchy, and system behavior."},
			{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", DisplayTitle: "Native charts stay editable after preview checks", DisplaySubtitle: "Editable objects survive export checks."},
			{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", DisplayTitle: "Reference style becomes a reusable system", DisplayBody: "The outcome is not a copied template but a reusable deck system that stays clear, calm, and editable."},
		},
	}
	plan, err := normalizePPTXArtifactDesignPlan(generated, fallback)
	if err != nil {
		t.Fatalf("normalizePPTXArtifactDesignPlan: %v", err)
	}
	if got := plan.Slides[0].DisplayTitle; got != fallback.Slides[0].DisplayTitle {
		t.Fatalf("cover title = %q, want stable fallback %q", got, fallback.Slides[0].DisplayTitle)
	}
	if got := plan.Slides[1].DisplayTitle; got != "The reference set teaches a visual system" {
		t.Fatalf("observation title = %q", got)
	}
	if got := plan.Slides[1].DisplaySubtitle; got != "Palette, density, hierarchy, and system behavior." {
		t.Fatalf("observation subtitle = %q", got)
	}
	if got := plan.Slides[2].DisplayTitle; got != fallback.Slides[2].DisplayTitle {
		t.Fatalf("chart title = %q, want fallback because generated copy leaks implementation terms", got)
	}
	if got := plan.Slides[3].DisplayBody; got != fallback.Slides[3].DisplayBody {
		t.Fatalf("closing body = %q, want stable fallback %q", got, fallback.Slides[3].DisplayBody)
	}
}

func TestNormalizePPTXArtifactDesignPlanReferenceLearningRejectsWeakClosingCopy(t *testing.T) {
	fallback := &pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		StyleBias:     "dark-structured",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", DisplayTitle: "PPT Reference Style Test"},
			{Slide: 2, Role: "observations", LayoutMode: "observation-cards", DisplayTitle: "What the reference directory actually teaches"},
			{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", DisplayTitle: "Fidelity comes from multiple enforced layers"},
			{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", DisplayTitle: "Reference style becomes a reusable system", DisplayBody: "Carry palette, rhythm, and hierarchy into a concise deck while keeping the message clear."},
		},
	}
	generated := pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		StyleBias:     "dark-structured",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", DisplayTitle: "Generic Style Deck"},
			{Slide: 2, Role: "observations", LayoutMode: "observation-cards", DisplayTitle: "The reference set teaches a visual system"},
			{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", DisplayTitle: "Native charts stay editable after preview checks"},
			{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", DisplayTitle: "Reference Style Needs a Builder Loop", DisplayBody: "Use reference signals as intent, then refine through an editable builder and visual QA loop."},
		},
	}
	plan, err := normalizePPTXArtifactDesignPlan(generated, fallback)
	if err != nil {
		t.Fatalf("normalizePPTXArtifactDesignPlan: %v", err)
	}
	if got := plan.Slides[3].DisplayTitle; got != fallback.Slides[3].DisplayTitle {
		t.Fatalf("closing title = %q, want fallback", got)
	}
	if got := plan.Slides[3].DisplayBody; got != fallback.Slides[3].DisplayBody {
		t.Fatalf("closing body = %q, want fallback", got)
	}
}

func TestNormalizePPTXArtifactDesignPlanReferenceLearningRejectsWeakObservationTakeaway(t *testing.T) {
	fallback := &pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		StyleBias:     "dark-structured",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual"},
			{Slide: 2, Role: "observations", LayoutMode: "observation-cards", Takeaway: "Use recurring visual choices as a system, not a literal template."},
		},
	}
	generated := pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		StyleBias:     "dark-structured",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual"},
			{Slide: 2, Role: "observations", LayoutMode: "observation-cards", Takeaway: "A builder loop turns palette, density, hierarchy, and rendered evidence into a coherent deck."},
		},
	}
	plan, err := normalizePPTXArtifactDesignPlan(generated, fallback)
	if err != nil {
		t.Fatalf("normalizePPTXArtifactDesignPlan: %v", err)
	}
	if got := plan.Slides[1].Takeaway; got != fallback.Slides[1].Takeaway {
		t.Fatalf("observation takeaway = %q, want fallback", got)
	}
}

func TestNormalizePPTXArtifactDesignPlanReferenceLearningDropsIncompleteCards(t *testing.T) {
	fallback := &pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual"},
			{
				Slide:      2,
				Role:       "observations",
				LayoutMode: "observation-cards",
				Cards: []pptxArtifactPlanCard{
					{Heading: "Repeatable style beats single-slide mimicry", Detail: "Use repeated panels, accent rules, and compact cards instead of copying a deck."},
					{Heading: "Important content stays editable", Detail: "Keep words, labels, and chart callouts editable, not baked into images."},
					{Heading: "Visual QA changes the final design", Detail: "Previews catch overflow, contrast issues, blank pages, and chart defaults."},
				},
			},
		},
	}
	generated := pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual"},
			{
				Slide:      2,
				Role:       "observations",
				LayoutMode: "observation-cards",
				Cards: []pptxArtifactPlanCard{
					{Heading: "System over template", Detail: "A consistent title scale and restrained accents do more than any single slide."},
					{Heading: "Editability is essential"},
					{Heading: "Quality comes from compact execution", Detail: "Short content paired with rendered checks helps preserve readability."},
				},
			},
		},
	}
	plan, err := normalizePPTXArtifactDesignPlan(generated, fallback)
	if err != nil {
		t.Fatalf("normalizePPTXArtifactDesignPlan: %v", err)
	}
	if got := len(plan.Slides[1].Cards); got != 3 {
		t.Fatalf("cards = %d, want 3", got)
	}
	if got := plan.Slides[1].Cards[0].Heading; got != "Repeatable style beats single-slide mimicry" {
		t.Fatalf("first card heading = %q, want fallback because generated card was generic", got)
	}
	if got := plan.Slides[1].Cards[1].Heading; got != "Important content stays editable" {
		t.Fatalf("second card heading = %q, want fallback because generated card was incomplete", got)
	}
	if got := plan.Slides[1].Cards[0].Heading; got != "Repeatable style beats single-slide mimicry" {
		t.Fatalf("first card heading = %q, want task-specific Codex-level fallback", got)
	}
	if got := plan.Slides[1].Cards[0].Detail; got != "Use repeated panels, accent rules, and compact cards instead of copying a deck." {
		t.Fatalf("first card detail = %q", got)
	}
	if got := plan.Slides[1].Cards[1].Heading; got != "Important content stays editable" {
		t.Fatalf("second card heading = %q, want task-specific Codex-level fallback", got)
	}
	if got := plan.Slides[1].Cards[1].Detail; got != "Keep words, labels, and chart callouts editable, not baked into images." {
		t.Fatalf("second card detail = %q", got)
	}
	if got := plan.Slides[1].Cards[2].Heading; got != "Visual QA changes the final design" {
		t.Fatalf("third card heading = %q, want fallback because generated card was generic quality phrasing", got)
	}
	if got := plan.Slides[1].Cards[2].Detail; got != "Previews catch overflow, contrast issues, blank pages, and chart defaults." {
		t.Fatalf("third card detail = %q", got)
	}
}

func TestBuildPPTXArtifactDesignPlanReferenceLearningUsesClosingSplitCalloutComposition(t *testing.T) {
	payload := pptxPayload{
		Title:       "Reference Style Learning Summary",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Style Learning Summary", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Reference Style Signals", Layout: "content", Points: []string{"Observation one", "Observation two"}},
			{Title: "Simple Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
	}
	plan := buildPPTXArtifactDesignPlan(payload, "Reference Style Learning Summary", PPTXBuildOptions{
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral",
		},
	})
	if plan == nil || len(plan.Slides) != 4 {
		t.Fatalf("plan = %#v, want four-slide reference-learning plan", plan)
	}
	if got := plan.Slides[3].Composition; got != "split-callout" {
		t.Fatalf("closing composition = %q, want split-callout", got)
	}
}

func TestBuildPPTXArtifactDesignPlanReferenceLearningUsesTaskSpecificVisibleCopy(t *testing.T) {
	payload := pptxPayload{
		Title:       "Reference Style Learning Summary",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Style Learning Summary", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Reference Style Signals", Layout: "content", Points: []string{"Observation one", "Observation two"}},
			{Title: "Simple Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
	}
	plan := buildPPTXArtifactDesignPlan(payload, "Reference Style Learning Summary", PPTXBuildOptions{
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral",
		},
	})
	if plan == nil || len(plan.Slides) != 4 {
		t.Fatalf("plan = %#v, want four-slide reference-learning plan", plan)
	}
	want := []string{
		"Reference Style Learning Summary",
		"What the reference directory actually teaches",
		"Fidelity comes from multiple enforced layers",
		"Reference style becomes a reusable system",
	}
	for idx, expected := range want {
		if got := plan.Slides[idx].DisplayTitle; got != expected {
			t.Fatalf("slide %d display title = %q, want %q", idx+1, got, expected)
		}
	}
	if got := plan.Slides[3].DisplayBody; got != "Carry palette, hierarchy, and spacing into one clear deck system." {
		t.Fatalf("closing display body = %q", got)
	}
	if got := plan.Slides[0].DisplaySubtitle; got != "Same prompt, reference style intent, and editable visual motifs." {
		t.Fatalf("cover display subtitle = %q", got)
	}
	if got := utf8.RuneCountInString(plan.Slides[0].DisplaySubtitle); got > 72 {
		t.Fatalf("cover display subtitle runes = %d, want <= 72: %q", got, plan.Slides[0].DisplaySubtitle)
	}
}

func TestBuildPPTXArtifactDesignPlanReferenceLearningIncludesStableCardsAndCallouts(t *testing.T) {
	payload := pptxPayload{
		Title:       "Reference Style Learning Summary",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Style Learning Summary", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Reference Style Signals", Layout: "content", Points: []string{"Observation one", "Observation two"}},
			{Title: "Simple Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
	}
	plan := buildPPTXArtifactDesignPlan(payload, "Reference Style Learning Summary", PPTXBuildOptions{
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral",
		},
	})
	if plan == nil || len(plan.Slides) != 4 {
		t.Fatalf("plan = %#v, want four-slide reference-learning plan", plan)
	}
	if got := len(plan.Slides[1].Cards); got != 3 {
		t.Fatalf("observation cards = %d, want 3", got)
	}
	if got := plan.Slides[1].Cards[2].Heading; got != "Readable hierarchy guides the deck" {
		t.Fatalf("third observation card heading = %q", got)
	}
	if got := len(plan.Slides[2].ChartCallouts); got != 2 {
		t.Fatalf("chart callouts = %d, want 2", got)
	}
	if got := plan.Slides[2].ChartCallouts[1].Heading; got != "Style focus" {
		t.Fatalf("second chart callout heading = %q", got)
	}
}

func TestBuildPPTXArtifactDesignPlanReferenceLearningStaysUnderReviewTextBudget(t *testing.T) {
	payload := pptxPayload{
		Title:       "Reference Style Learning Summary",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Style Learning Summary", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Reference Style Signals", Layout: "content", Points: []string{"Observation one", "Observation two"}},
			{Title: "Simple Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
	}
	plan := buildPPTXArtifactDesignPlan(payload, "Reference Style Learning Summary", PPTXBuildOptions{
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
	})
	if plan == nil || len(plan.Slides) != 4 {
		t.Fatalf("plan = %#v, want four-slide reference-learning plan", plan)
	}
	for _, idx := range []int{1, 3} {
		if got := designPlanSlideTextRunes(plan.Slides[idx]); got > 600 {
			t.Fatalf("slide %d planned text budget = %d, want <= 600 so readable card layouts stay below review density threshold; slide=%+v", idx+1, got, plan.Slides[idx])
		}
	}
}

func designPlanSlideTextRunes(slide pptxArtifactSlideDesignPlan) int {
	total := utf8.RuneCountInString(slide.Kicker) +
		utf8.RuneCountInString(slide.DisplayTitle) +
		utf8.RuneCountInString(slide.DisplaySubtitle) +
		utf8.RuneCountInString(slide.DisplayBody) +
		utf8.RuneCountInString(slide.Takeaway)
	for _, card := range slide.Cards {
		total += utf8.RuneCountInString(card.Heading) + utf8.RuneCountInString(card.Detail)
	}
	for _, card := range slide.ChartCallouts {
		total += utf8.RuneCountInString(card.Heading) + utf8.RuneCountInString(card.Detail)
	}
	return total
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalUsesDedicatedPlannerLLM(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 1, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Dedicated Planner Demo",
		"slides":[
			{"title":"Dedicated Planner Demo","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Observation 1","Observation 2"]},
			{"title":"Simple Chart","layout":"chart","points":["Chart point"],"chart":{"type":"bar","title":"Simple signal","categories":["A","B"],"values":[1,2]}},
			{"title":"Closing","layout":"closing","points":["Close"]}
		]
	}`
	imageLLM := &fakeLLMClient{structuredErr: errors.New("image profile should not be used for planner")}
	textLLM := &fakeLLMClient{structuredResponse: `{
		"deckIntent":"dedicated-planner",
		"styleBias":"dark-structured",
		"slides":[
			{"slide":1,"role":"cover","layoutMode":"cover-split-visual","visualTreatment":"reference-visual-panel","densityTarget":"spacious","kicker":"","takeaway":"","visualIntent":"Use a cover visual."},
			{"slide":2,"role":"observations","layoutMode":"observation-cards","visualTreatment":"native-shapes","densityTarget":"balanced","kicker":"PLANNED OBSERVATIONS","takeaway":"Plan-made observation takeaway.","visualIntent":"Use three cards."},
			{"slide":3,"role":"evidence","layoutMode":"chart-insight-stack","visualTreatment":"native-chart","densityTarget":"spacious","kicker":"PLANNED CHART","takeaway":"","visualIntent":"Use a chart."},
			{"slide":4,"role":"closing","layoutMode":"closing-takeaway","visualTreatment":"native-shapes","densityTarget":"compact","kicker":"PLANNED CLOSE","takeaway":"","visualIntent":"Use support cards."}
		]
	}`}
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), imageLLM, nil, content, "Dedicated Planner Demo", "", true, false, PPTXBuildOptions{
		Backend:                    PPTXBackendArtifactWorker,
		UserPrompt:                 "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		GenerateArtifactDesignPlan: true,
		ArtifactDesignPlanLLM:      textLLM,
		ReferenceBrief:             &PPTXReferenceStyleBrief{StylePresetHint: "executive-dark", PaletteIntent: "dark neutral"},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if imageLLM.structuredCallCount != 0 {
		t.Fatalf("image structured calls = %d, want 0", imageLLM.structuredCallCount)
	}
	if textLLM.structuredCallCount != 3 {
		t.Fatalf("text structured calls = %d, want initial plan plus two preview-informed polish plan attempts", textLLM.structuredCallCount)
	}
	if containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_DESIGN_PLAN_FALLBACK") {
		t.Fatalf("warnings = %+v, did not expect design plan fallback", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalPolishesConciseReferenceNarrative(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 1, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Observation 1","Observation 2"]},
			{"title":"Simple Chart","layout":"chart","points":["Chart point"],"chart":{"type":"bar","title":"Simple signal","categories":["A","B"],"values":[1,2]}},
			{"title":"Closing","layout":"closing","points":["Close"]}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint:  "executive-dark",
			PaletteIntent:    "dark executive canvas with cyan and amber accents",
			LayoutRhythm:     "strong title band, compact cards, and consistent left alignment",
			TypographyIntent: "large headings and restrained body text",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if len(captured.Slides) != 4 {
		t.Fatalf("captured slides = %d, want 4", len(captured.Slides))
	}
	if len(captured.Slides[0].Points) < 3 {
		t.Fatalf("cover points = %#v, want concrete reference-signal chips", captured.Slides[0].Points)
	}
	if captured.Slides[1].Title != "Reference Style Signals" {
		t.Fatalf("observation title = %q", captured.Slides[1].Title)
	}
	if captured.Slides[1].Subtitle != "System over template." {
		t.Fatalf("observation subtitle = %q", captured.Slides[1].Subtitle)
	}
	if !strings.Contains(captured.Slides[1].Content, "coherent visual system") {
		t.Fatalf("observation content = %q, want bottom takeaway about visual system", captured.Slides[1].Content)
	}
	if len(captured.Slides[1].Sections) != 3 {
		t.Fatalf("observation sections = %#v, want 3 designed cards", captured.Slides[1].Sections)
	}
	for idx, section := range captured.Slides[1].Sections {
		if strings.TrimSpace(section.Detail) == "" || pptxArtifactVisibleTextLooksDangling(section.Detail) {
			t.Fatalf("observation section %d detail incomplete: %#v", idx, section)
		}
	}
	if got := textDensityRunes(captured.Slides[1]); got > 460 {
		t.Fatalf("observation slide text density = %d, want <= 460; slide=%+v", got, captured.Slides[1])
	}
	wantCategories := []string{"Style Profile", "Layout Rhythm", "Editable Objects", "Readable Flow"}
	if captured.Slides[2].Chart == nil {
		t.Fatal("chart slide lost chart data")
	}
	if strings.Join(captured.Slides[2].Chart.Categories, "|") != strings.Join(wantCategories, "|") {
		t.Fatalf("chart categories = %#v, want %#v", captured.Slides[2].Chart.Categories, wantCategories)
	}
	if captured.Slides[3].Title == "Closing" || len(captured.Slides[3].Sections) < 2 {
		t.Fatalf("closing should have a task-specific title and supporting sections: %+v", captured.Slides[3])
	}
}

func TestPPTXArtifactReferenceLearningUsesFallbackTopicAsDeckTitle(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 1, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Reference Style Signal Assessment",
		"slides":[
			{"title":"Reference Style Signal Assessment","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Observation 1","Observation 2"]},
			{"title":"Simple Chart","layout":"chart","points":["Chart point"],"chart":{"type":"bar","title":"Simple signal","categories":["A","B"],"values":[1,2]}},
			{"title":"Closing","layout":"closing","points":["Close"]}
		]
	}`
	_, fileName, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if captured.Title != "PPT Reference Style Test" {
		t.Fatalf("worker title = %q, want fallback topic", captured.Title)
	}
	if fileName != "PPT_Reference_Style_Test.pptx" {
		t.Fatalf("fileName = %q, want topic-derived filename", fileName)
	}
	if captured.DesignPlan == nil || captured.DesignPlan.Slides[0].DisplayTitle != "PPT Reference Style Test" {
		t.Fatalf("cover display title = %#v, want topic-derived title", captured.DesignPlan)
	}
}

func TestDiscoverPPTXArtifactVisualAssetsSkipsGeneratedOutputAndScratch(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"output", "tmp/slides/preview", "参考文章/topic"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	imageBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	files := []string{
		"output/generated-preview.png",
		"tmp/slides/preview/slide-01.png",
		"参考文章/topic/image-01.png",
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(root, file), imageBytes, 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}
	assets, err := discoverPPTXArtifactVisualAssets(root, 8)
	if err != nil {
		t.Fatalf("discoverPPTXArtifactVisualAssets: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("assets = %#v, want only the non-generated reference image", assets)
	}
	if !strings.Contains(filepath.ToSlash(assets[0].Path), "参考文章/topic/image-01.png") {
		t.Fatalf("asset path = %q", assets[0].Path)
	}
}

func TestDiscoverPPTXArtifactVisualAssetsPrefersCoverAcrossReferenceArticles(t *testing.T) {
	root := t.TempDir()
	imageBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	files := []string{
		"参考文章/Alpha/cover.jpg",
		"参考文章/Alpha/image-01.png",
		"参考文章/Alpha/image-02.png",
		"参考文章/Beta/cover.jpg",
		"参考文章/Beta/image-01.png",
		"参考文章/Gamma/cover.jpg",
		"参考文章/Gamma/image-01.png",
	}
	for _, file := range files {
		path := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, imageBytes, 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}
	assets, err := discoverPPTXArtifactVisualAssets(root, 3)
	if err != nil {
		t.Fatalf("discoverPPTXArtifactVisualAssets: %v", err)
	}
	got := make([]string, 0, len(assets))
	for _, asset := range assets {
		got = append(got, filepath.ToSlash(asset.Path))
	}
	for _, want := range []string{
		"参考文章/Alpha/cover.jpg",
		"参考文章/Beta/cover.jpg",
		"参考文章/Gamma/cover.jpg",
	} {
		found := false
		for _, path := range got {
			if strings.Contains(path, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("assets = %#v, want cover asset %s", got, want)
		}
	}
}

func TestDiscoverPPTXArtifactVisualAssetsKeepsArticleImagesForSecondaryPlates(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{
		"参考文章/Alpha/cover.png",
		"参考文章/Alpha/image-01.png",
		"参考文章/Beta/cover.png",
		"参考文章/Beta/image-02.png",
		"参考文章/Gamma/cover.png",
		"参考文章/Gamma/image-03.png",
		"参考文章/Delta/cover.png",
		"参考文章/Delta/image-04.png",
	} {
		writeCheckerPNGFixture(t, filepath.Join(root, file), color.RGBA{R: 8, G: 16, B: 32, A: 255}, color.RGBA{R: 56, G: 217, B: 255, A: 255})
	}

	assets, err := discoverPPTXArtifactVisualAssets(root, 4)
	if err != nil {
		t.Fatalf("discoverPPTXArtifactVisualAssets: %v", err)
	}
	if len(assets) != 4 {
		t.Fatalf("assets = %#v, want four assets", assets)
	}
	if !strings.Contains(filepath.ToSlash(assets[0].Path), "cover.png") {
		t.Fatalf("first asset = %q, want a cover plate for the deck cover", assets[0].Path)
	}
	foundArticleImage := false
	foundPreferredSecondary := false
	for _, asset := range assets[1:] {
		if strings.Contains(filepath.Base(asset.Path), "image-") {
			foundArticleImage = true
		}
		if strings.Contains(filepath.Base(asset.Path), "image-02") || strings.Contains(filepath.Base(asset.Path), "image-03") {
			foundPreferredSecondary = true
		}
	}
	if !foundArticleImage {
		t.Fatalf("assets = %#v, want a non-cover article image for secondary visual plates", assets)
	}
	if !foundPreferredSecondary {
		t.Fatalf("assets = %#v, want image-02 or image-03 for secondary visual plates", assets)
	}
}

func TestRepresentativeVisualAssetsPrefersPromptRelevantSecondaryPlate(t *testing.T) {
	root := t.TempDir()
	unrelated := filepath.Join(root, "参考文章/别急着裁人，AI 更像是收益放大器，不是降本器/image-02.png")
	relevant := filepath.Join(root, "参考文章/AI 写得越快，测试越不能省/image-02.png")
	for _, topic := range []string{
		"Background Agents 与软件交付的下一个时代",
		"AI 写得越快，测试越不能省",
		"AI时代的新竞争法则：不是谁的模型更强，而是谁落地更快",
		"软件工程的未来：当 AI 写代码，工程师的价值在哪",
		"别急着裁人，AI 更像是收益放大器，不是降本器",
		"AI 时代的基础设施重构：程序员不是被替代，而是跃迁",
		"会用 AI，到用好 AI：差的不是工具，是工作方式",
		"谈谈这半年对AI的感受",
	} {
		writeSizedCheckerPNGFixture(t, filepath.Join(root, "参考文章", topic, "cover.png"), 60, 24, color.RGBA{R: 8, G: 16, B: 32, A: 255}, color.RGBA{R: 56, G: 217, B: 255, A: 255})
	}
	writeCheckerPNGFixture(t, unrelated, color.RGBA{R: 4, G: 8, B: 16, A: 255}, color.RGBA{R: 255, G: 180, B: 24, A: 255})
	writeSolidPNGFixture(t, relevant, color.RGBA{R: 238, G: 246, B: 250, A: 255})

	assets := representativeVisualAssets(PPTXBuildOptions{
		ReferenceScanRoot: root,
		UserPrompt:        "Create a concise editable presentation about reference style learning, preview validation, visual QA, and testing quality.",
	}, true)
	if len(assets) < 2 {
		t.Fatalf("assets = %#v, want cover plus secondary visual plate", assets)
	}
	foundRelevant := false
	for _, asset := range assets[1:] {
		if asset.Path == relevant {
			foundRelevant = true
			break
		}
	}
	if !foundRelevant {
		t.Fatalf("assets = %#v, want prompt-relevant secondary image %q", assets, relevant)
	}
}

func TestRepresentativeVisualAssetsBindsLocalFallbacksForReferenceLearningDeck(t *testing.T) {
	root := t.TempDir()
	for _, topic := range []string{
		"Background Agents 与软件交付的下一个时代",
		"AI 写得越快，测试越不能省",
		"AI时代的新竞争法则：不是谁的模型更强，而是谁落地更快",
		"软件工程的未来：当 AI 写代码，工程师的价值在哪",
		"别急着裁人，AI 更像是收益放大器，不是降本器",
		"AI 时代的基础设施重构：程序员不是被替代，而是跃迁",
		"会用 AI，到用好 AI：差的不是工具，是工作方式",
		"谈谈这半年对AI的感受",
	} {
		writeSizedCheckerPNGFixture(t, filepath.Join(root, "参考文章", topic, "cover.png"), 60, 24, color.RGBA{R: 8, G: 16, B: 32, A: 255}, color.RGBA{R: 56, G: 217, B: 255, A: 255})
	}
	badExactCover := filepath.Join(root, "参考文章", "白底拼贴", "image-01.png")
	writeSizedCheckerPNGFixture(t, badExactCover, 32, 25, color.RGBA{R: 250, G: 250, B: 250, A: 255}, color.RGBA{R: 250, G: 250, B: 250, A: 255})
	goodCover := filepath.Join(root, "参考文章", "比例匹配封面", "image-01.png")
	writeSizedCheckerPNGFixture(t, goodCover, 36, 28, color.RGBA{R: 8, G: 16, B: 32, A: 255}, color.RGBA{R: 56, G: 217, B: 255, A: 255})
	closing := filepath.Join(root, "参考文章", "AI 写得越快，测试越不能省", "image-02.png")
	writeSizedCheckerPNGFixture(t, closing, 36, 25, color.RGBA{R: 4, G: 8, B: 16, A: 255}, color.RGBA{R: 255, G: 180, B: 24, A: 255})

	assets := representativeVisualAssetsForDesignPlan(PPTXBuildOptions{
		ReferenceScanRoot: root,
		UserPrompt:        "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
	}, true, &pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", DisplayTitle: "PPT Reference Style Test"},
			{Slide: 2, Role: "observations", DisplayTitle: "What the reference directory actually teaches", Cards: []pptxArtifactPlanCard{
				{Heading: "Visual QA changes the final design", Detail: "Previews catch overflow, contrast issues, blank pages, and chart defaults."},
			}},
			{Slide: 4, Role: "closing", DisplayTitle: "Turn reference cues into a repeatable deck system"},
		},
	})
	if len(assets) != 2 {
		t.Fatalf("assets = %#v, want cover and closing local visual fallbacks", assets)
	}
	if assets[0].Slide != 1 || assets[1].Slide != 4 {
		t.Fatalf("asset slides = %d,%d; want slide 1 and slide 4 fallbacks", assets[0].Slide, assets[1].Slide)
	}
	if assets[0].Path != goodCover {
		t.Fatalf("cover fallback = %q, want ratio-matched cover %q", assets[0].Path, goodCover)
	}
	if assets[1].Path != closing {
		t.Fatalf("closing fallback = %q, want ratio-matched secondary %q", assets[1].Path, closing)
	}
	if assets[0].Frame == nil || assets[1].Frame == nil {
		t.Fatalf("assets = %#v, want slide-bound frames", assets)
	}
	if assets[0].TargetAspectRatio <= 0 || assets[1].SourceAspectRatio <= 0 {
		t.Fatalf("assets = %#v, want target/source ratios", assets)
	}
	if assets[0].TextDetection == nil || assets[1].TextDetection == nil {
		t.Fatalf("assets = %#v, want explicit text-detection metadata", assets)
	}
	if strings.Contains(strings.ToLower(assets[0].Name), "reference") || strings.Contains(strings.ToLower(assets[0].Name), "cover") {
		t.Fatalf("cover fallback name = %q, should be sanitized", assets[0].Name)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactReferenceLearningGeneratesTextFreeVisualPlate(t *testing.T) {
	original := runPPTXArtifactWorker
	originalDetector := detectPPTXArtifactImageText
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		if len(req.VisualAssets) == 0 {
			t.Fatalf("VisualAssets is empty; reference-learning artifact backend should pass generated text-free plates to the worker")
		}
		if len(req.VisualAssets) < 2 {
			t.Fatalf("VisualAssets = %#v, want default cover and closing text-free plates", req.VisualAssets)
		}
		if !strings.Contains(filepath.ToSlash(req.VisualAssets[0].Path), "artifact-text-free-plates") {
			t.Fatalf("visual asset path = %q, want generated text-free plate path", req.VisualAssets[0].Path)
		}
		if req.VisualAssets[0].Slide != 1 || req.VisualAssets[1].Slide != 4 {
			t.Fatalf("visual asset slides = %d,%d; want slide 1 cover and slide 4 closing", req.VisualAssets[0].Slide, req.VisualAssets[1].Slide)
		}
		for _, asset := range req.VisualAssets {
			if asset.Width <= 0 || asset.Height <= 0 {
				t.Fatalf("visual asset dimensions = %dx%d for %#v, want decoded dimensions", asset.Width, asset.Height, asset)
			}
			if asset.Frame == nil || asset.Frame.Width <= 0 || asset.Frame.Height <= 0 {
				t.Fatalf("visual asset frame = %#v for %#v, want target placement frame", asset.Frame, asset)
			}
			if asset.TargetAspectRatio <= 0 || asset.SourceAspectRatio <= 0 {
				t.Fatalf("visual asset ratios target=%.4f source=%.4f for %#v", asset.TargetAspectRatio, asset.SourceAspectRatio, asset)
			}
			if asset.TextDetection == nil || !asset.TextDetection.Checked || asset.TextDetection.Status != "passed" || asset.TextDetection.Attempts != 1 {
				t.Fatalf("visual asset text detection = %#v for %#v, want checked passed on first attempt", asset.TextDetection, asset)
			}
		}
		if strings.Contains(strings.ToLower(req.VisualAssets[0].Name), "reference") {
			t.Fatalf("visual asset name = %q, should not look like a copied reference image", req.VisualAssets[0].Name)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 4, 1)
		output.VisualAssets = len(req.VisualAssets)
		return output, nil
	}
	detectPPTXArtifactImageText = func(context.Context, string) (string, bool, error) {
		return "", true, nil
	}
	defer func() {
		runPPTXArtifactWorker = original
		detectPPTXArtifactImageText = originalDetector
	}()

	imageLLM := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), imageLLM, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral palette with cyan and amber accents",
			LayoutRhythm:    "dark cards, clear hierarchy, restrained density",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if imageLLM.imageCalls == 0 {
		t.Fatal("image LLM was not called for reference-learning text-free plate")
	}
	if imageLLM.imageCalls != 2 {
		t.Fatalf("image calls = %d, want default cover and closing plates", imageLLM.imageCalls)
	}
	if got := imageLLM.lastImageRequest.Prompt; !strings.Contains(strings.ToLower(got), "no text") || !strings.Contains(strings.ToLower(got), "text-free") {
		t.Fatalf("image prompt should request a text-free plate, got: %q", got)
	}
	for _, term := range []string{"editorial tech illustration", "semi-flat vector", "not photorealistic", "blank document panels"} {
		if !strings.Contains(strings.ToLower(imageLLM.lastImageRequest.Prompt), term) {
			t.Fatalf("image prompt should steer toward Codex-style text-free illustration term %q, got: %q", term, imageLLM.lastImageRequest.Prompt)
		}
	}
	if len(imageLLM.imageRequests) != 2 {
		t.Fatalf("image requests = %d, want 2", len(imageLLM.imageRequests))
	}
	for idx, request := range imageLLM.imageRequests {
		for _, forbidden := range []string{"PPT Reference Style Test", "Key Observations", "Simple Chart", "Closing"} {
			if strings.Contains(request.Prompt, forbidden) {
				t.Fatalf("image request %d prompt leaked editable title %q: %q", idx+1, forbidden, request.Prompt)
			}
		}
	}
	expectedPromptTerms := []string{"right-side cover", "right-side closing"}
	expectedRatios := []float64{320.0 / 250.0, 326.0 / 226.0}
	for idx, term := range expectedPromptTerms {
		if !strings.Contains(strings.ToLower(imageLLM.imageRequests[idx].Prompt), term) {
			t.Fatalf("image request %d prompt = %q, want role-specific term %q", idx+1, imageLLM.imageRequests[idx].Prompt, term)
		}
		if got, want := imageLLM.imageRequests[idx].TargetAspectRatio, expectedRatios[idx]; got < want-0.01 || got > want+0.01 {
			t.Fatalf("image request %d aspect ratio = %.4f, want %.4f", idx+1, got, want)
		}
	}
	if captured.VisualAssets[0].MIME != "image/png" {
		t.Fatalf("visual asset MIME = %q, want image/png", captured.VisualAssets[0].MIME)
	}
	if containsIssueCode(warnings, "WARN_PPT_IMAGE_DEGRADED") {
		t.Fatalf("warnings = %+v, did not expect image degradation when plate generation succeeds", warnings)
	}
}

func TestPPTXArtifactTextFreePlateSlidePlansDefaultsToCoverAndClosingUnlessExplicit(t *testing.T) {
	plan := &pptxArtifactDesignPlan{Slides: []pptxArtifactSlideDesignPlan{
		{Slide: 1, Role: "cover", VisualTreatment: "native-shapes"},
		{Slide: 2, Role: "observations", VisualTreatment: "native-shapes"},
		{Slide: 3, Role: "evidence", VisualTreatment: "native-chart"},
		{Slide: 4, Role: "closing", VisualTreatment: "native-shapes"},
	}}
	if got := pptxArtifactTextFreePlateSlidePlans(plan); len(got) != 2 || got[0].Slide != 1 || got[1].Slide != 4 {
		t.Fatalf("default plate plans = %#v, want cover plus closing", got)
	}

	plan.Slides[1].VisualTreatment = "text-free-visual-plate"
	plan.Slides[3].VisualTreatment = "text-free-visual-plate"
	got := pptxArtifactTextFreePlateSlidePlans(plan)
	if len(got) != 3 || got[0].Slide != 1 || got[1].Slide != 2 || got[2].Slide != 4 {
		t.Fatalf("explicit plate plans = %#v, want cover plus explicit observation/closing", got)
	}
}

func TestDedupePPTXArtifactSlideBoundVisualAssetsKeepsFirstCandidatePerSlide(t *testing.T) {
	assets := []pptxArtifactVisualAsset{
		{Slide: 1, Path: "generated-cover.png"},
		{Slide: 4, Path: "generated-closing.png"},
		{Slide: 1, Path: "local-cover-fallback.png"},
		{Slide: 4, Path: "local-closing-fallback.png"},
		{Path: "unbound.png"},
	}
	got := dedupePPTXArtifactSlideBoundVisualAssets(assets)
	if len(got) != 3 {
		t.Fatalf("deduped assets = %#v, want generated cover, generated closing, and unbound asset", got)
	}
	if got[0].Path != "generated-cover.png" || got[1].Path != "generated-closing.png" || got[2].Path != "unbound.png" {
		t.Fatalf("deduped assets order/content = %#v", got)
	}
}

func TestPPTXArtifactTextFreePlateParallelismRequiresProgressAndAllowsEnvOverride(t *testing.T) {
	if got := pptxArtifactTextFreePlateParallelism(nil, 3); got != 1 {
		t.Fatalf("parallelism without progress = %d, want sequential", got)
	}
	progress := runtimeProgressCollector{}
	if got := pptxArtifactTextFreePlateParallelism(&progress, 3); got != 2 {
		t.Fatalf("default parallelism = %d, want 2", got)
	}
	t.Setenv("OFFICECLI_PPTX_ARTIFACT_PARALLEL_PLATES", "1")
	if got := pptxArtifactTextFreePlateParallelism(&progress, 3); got != 1 {
		t.Fatalf("env parallelism 1 = %d, want 1", got)
	}
	t.Setenv("OFFICECLI_PPTX_ARTIFACT_PARALLEL_PLATES", "9")
	if got := pptxArtifactTextFreePlateParallelism(&progress, 3); got != 3 {
		t.Fatalf("env parallelism should cap at slide count, got %d", got)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactReferenceLearningFallsBackWhenTextFreePlateFails(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		if len(req.VisualAssets) != 0 {
			t.Fatalf("VisualAssets = %#v, want no generated plate after image failure", req.VisualAssets)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 4, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	imageLLM := &fakeLLMClient{imageErr: errors.New("image provider unavailable")}
	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), imageLLM, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral palette with cyan and amber accents",
			LayoutRhythm:    "dark cards, clear hierarchy, restrained density",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions should keep artifact worker path alive after image fallback: %v", err)
	}
	if imageLLM.imageCalls == 0 {
		t.Fatal("image LLM was not called before falling back to native motifs")
	}
	if len(captured.VisualAssets) != 0 {
		t.Fatalf("captured.VisualAssets = %#v, want native-motif fallback without visual assets", captured.VisualAssets)
	}
	if !containsIssueCode(warnings, "WARN_PPT_IMAGE_DEGRADED") {
		t.Fatalf("warnings = %+v, want WARN_PPT_IMAGE_DEGRADED", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactReferenceLearningKeepsSuccessfulTextFreePlatesWhenLaterPlateFails(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		if len(req.VisualAssets) != 1 {
			t.Fatalf("VisualAssets = %#v, want the first successful generated plate to be preserved", req.VisualAssets)
		}
		if req.VisualAssets[0].Slide != 1 {
			t.Fatalf("VisualAssets[0].Slide = %d, want cover plate preserved", req.VisualAssets[0].Slide)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 4, 1)
		output.VisualAssets = len(req.VisualAssets)
		return output, nil
	}
	originalDetector := detectPPTXArtifactImageText
	detectPPTXArtifactImageText = func(context.Context, string) (string, bool, error) {
		return "", true, nil
	}
	defer func() {
		runPPTXArtifactWorker = original
		detectPPTXArtifactImageText = originalDetector
	}()

	imageLLM := &fakeLLMClient{
		structuredResponse: `{
			"deckIntent":"concise-reference-style-learning",
			"styleBias":"dark-structured",
			"builderRecipe":"codex-reference-learning",
			"slides":[
				{"slide":1,"role":"cover","layoutMode":"cover-split-visual","visualTreatment":"native-shapes","visualIntent":"Use a nonverbal cover plate."},
				{"slide":2,"role":"observations","layoutMode":"observation-cards","visualTreatment":"text-free-visual-plate","visualIntent":"Use a small nonverbal observation plate."},
				{"slide":3,"role":"evidence","layoutMode":"chart-insight-stack","visualTreatment":"native-chart"},
				{"slide":4,"role":"closing","layoutMode":"closing-takeaway","visualTreatment":"native-shapes"}
			]
		}`,
		imageErrors: []error{nil, errors.New("image provider unavailable")},
		imageResult: &engine.ImageGenerationResult{
			Data: mustTinyPNG(t),
			MIME: "image/png",
		},
	}
	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), imageLLM, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral palette with cyan and amber accents",
			LayoutRhythm:    "dark cards, clear hierarchy, restrained density",
		},
		GenerateArtifactDesignPlan: true,
		ArtifactDesignPlanLLM:      imageLLM,
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions should keep artifact worker path alive after partial image failure: %v", err)
	}
	if len(captured.VisualAssets) != 1 {
		t.Fatalf("captured.VisualAssets = %#v, want one preserved generated plate", captured.VisualAssets)
	}
	if imageLLM.imageCalls != 3 {
		t.Fatalf("image calls = %d, want cover, explicit observation, and closing plate attempts", imageLLM.imageCalls)
	}
	if !containsIssueCode(warnings, "WARN_PPT_IMAGE_DEGRADED") {
		t.Fatalf("warnings = %+v, want WARN_PPT_IMAGE_DEGRADED", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactReferenceLearningTimesOutSlowTextFreePlate(t *testing.T) {
	restoreTimeout := SetPPTXArtifactTextFreePlateTimeoutForTesting(30 * time.Millisecond)
	defer restoreTimeout()

	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		if len(req.VisualAssets) != 0 {
			t.Fatalf("VisualAssets = %#v, want no generated plate after image timeout", req.VisualAssets)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 4, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	imageLLM := &fakeLLMClient{
		imageDelay:  200 * time.Millisecond,
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`

	start := time.Now()
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), imageLLM, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral palette with cyan and amber accents",
			LayoutRhythm:    "dark cards, clear hierarchy, restrained density",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions should keep artifact worker path alive after image timeout: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("text-free plate timeout fallback took too long: %s", elapsed)
	}
	if imageLLM.imageCalls == 0 {
		t.Fatal("image LLM was not called before timing out")
	}
	if imageLLM.imageCalls != 4 {
		t.Fatalf("image calls = %d, want each default text-free plate to get one timeout retry", imageLLM.imageCalls)
	}
	if len(captured.VisualAssets) != 0 {
		t.Fatalf("captured.VisualAssets = %#v, want native-motif fallback without visual assets", captured.VisualAssets)
	}
	if !containsIssueCode(warnings, "WARN_PPT_IMAGE_DEGRADED") {
		t.Fatalf("warnings = %+v, want WARN_PPT_IMAGE_DEGRADED", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactReferenceLearningRetriesTransientTextFreePlateFailure(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		if len(req.VisualAssets) != 2 {
			t.Fatalf("VisualAssets = %#v, want recovered cover and closing plates", req.VisualAssets)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 4, 1)
		output.VisualAssets = len(req.VisualAssets)
		return output, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	imageLLM := &fakeLLMClient{
		imageErrors: []error{context.DeadlineExceeded, nil, errors.New("temporary EOF"), nil},
		imageResult: &engine.ImageGenerationResult{
			Data: mustTinyPNG(t),
			MIME: "image/png",
		},
	}
	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), imageLLM, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral palette with cyan and amber accents",
			LayoutRhythm:    "dark cards, clear hierarchy, restrained density",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions should recover from transient image failures: %v", err)
	}
	if imageLLM.imageCalls != 4 {
		t.Fatalf("image calls = %d, want one transient retry per default plate", imageLLM.imageCalls)
	}
	if len(imageLLM.imageRequests) != 4 {
		t.Fatalf("image requests = %d, want 4", len(imageLLM.imageRequests))
	}
	if !strings.Contains(strings.ToLower(imageLLM.imageRequests[1].Prompt), "simple dark tech composition") {
		t.Fatalf("retry prompt should be simplified, got: %q", imageLLM.imageRequests[1].Prompt)
	}
	if len(captured.VisualAssets) != 2 {
		t.Fatalf("captured.VisualAssets = %#v, want recovered visual assets", captured.VisualAssets)
	}
	if containsIssueCode(warnings, "WARN_PPT_IMAGE_DEGRADED") {
		t.Fatalf("warnings = %+v, did not expect degradation after transient retry recovery", warnings)
	}
}

func TestPPTXArtifactWorkerFallbackMotifUsesIllustrativeNativeShapes(t *testing.T) {
	for _, needle := range []string{
		"fallback-motif-diagonal-flow",
		"fallback-motif-system-node",
		"fallback-motif-signal-panel",
	} {
		if !strings.Contains(pptxArtifactWorkerScript, needle) {
			t.Fatalf("worker script missing illustrative fallback motif role %q", needle)
		}
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactReferenceLearningKeepsLaterTextFreePlatesWhenFirstPlateFails(t *testing.T) {
	original := runPPTXArtifactWorker
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		if len(req.VisualAssets) != 1 {
			t.Fatalf("VisualAssets = %#v, want the later successful generated plate to be preserved", req.VisualAssets)
		}
		if req.VisualAssets[0].Slide != 4 {
			t.Fatalf("VisualAssets[0].Slide = %d, want closing plate preserved after cover failure", req.VisualAssets[0].Slide)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 4, 1)
		output.VisualAssets = len(req.VisualAssets)
		return output, nil
	}
	originalDetector := detectPPTXArtifactImageText
	detectPPTXArtifactImageText = func(context.Context, string) (string, bool, error) {
		return "", true, nil
	}
	defer func() {
		runPPTXArtifactWorker = original
		detectPPTXArtifactImageText = originalDetector
	}()

	imageLLM := &fakeLLMClient{
		imageErrors: []error{errors.New("cover provider unavailable"), nil},
		imageResult: &engine.ImageGenerationResult{
			Data: mustTinyPNG(t),
			MIME: "image/png",
		},
	}
	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), imageLLM, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral palette with cyan and amber accents",
			LayoutRhythm:    "dark cards, clear hierarchy, restrained density",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions should keep artifact worker path alive after first image failure: %v", err)
	}
	if len(captured.VisualAssets) != 1 {
		t.Fatalf("captured.VisualAssets = %#v, want one preserved generated plate", captured.VisualAssets)
	}
	if imageLLM.imageCalls != 2 {
		t.Fatalf("image calls = %d, want cover failure plus closing attempt", imageLLM.imageCalls)
	}
	if !containsIssueCode(warnings, "WARN_PPT_IMAGE_DEGRADED") {
		t.Fatalf("warnings = %+v, want WARN_PPT_IMAGE_DEGRADED", warnings)
	}
}

func TestPPTXArtifactWorkerScriptDoesNotReuseSlideBoundReferenceLearningAssets(t *testing.T) {
	for _, needle := range []string{
		"usesCodexReferenceLearningRecipe() && hasSlideBoundVisualAssets()",
		"function hasSlideBoundVisualAssets()",
		"return visualAssets.some((item) => Number(item && item.slide || 0) > 0);",
	} {
		if !strings.Contains(pptxArtifactWorkerScript, needle) {
			t.Fatalf("worker script missing slide-bound asset guard %q", needle)
		}
	}
}

func TestPPTXArtifactWorkerScriptIgnoresReferenceLearningBuilderPatchUnderlay(t *testing.T) {
	for _, needle := range []string{
		"if (usesCodexReferenceLearningRecipe()) return;",
		"function applyDynamicBuilderPatchUnderlay(slide, index, colors)",
	} {
		if !strings.Contains(pptxArtifactWorkerScript, needle) {
			t.Fatalf("worker script missing reference-learning patch guard %q", needle)
		}
	}
}

func TestPPTXArtifactWorkerScriptNormalizesReferenceLearningKickers(t *testing.T) {
	if !strings.Contains(pptxArtifactWorkerScript, `usesCodexReferenceLearningRecipe() ? String(out || "").toUpperCase() : out`) {
		t.Fatal("worker script should normalize reference-learning section kickers to uppercase")
	}
}

func TestPPTXArtifactWorkerScriptLetsExplicitLightStyleOverrideDarkReferenceBrief(t *testing.T) {
	for _, needle := range []string{
		`const explicitStyle = String([`,
		`const lightMode = /editorial-light|light-theme|light theme|light style|white canvas|off-white|off white|bright canvas|airy whitespace/.test(explicitStyle);`,
		`const darkMode = !lightMode && /executive-dark|dark|dark-neutral|dark neutral|night|deep canvas/.test(intent);`,
		`return "White canvas, soft cards, teal accents.";`,
	} {
		if !strings.Contains(pptxArtifactWorkerScript, needle) {
			t.Fatalf("worker script missing explicit light-style guard %q", needle)
		}
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactReferenceLearningRejectsTextBearingPlate(t *testing.T) {
	original := runPPTXArtifactWorker
	originalDetector := detectPPTXArtifactImageText
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		if len(req.VisualAssets) != 0 {
			t.Fatalf("VisualAssets = %#v, want generated plates rejected after OCR detects readable text", req.VisualAssets)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 4, 1), nil
	}
	detectPPTXArtifactImageText = func(context.Context, string) (string, bool, error) {
		return "Q4 ROADMAP", true, nil
	}
	defer func() {
		runPPTXArtifactWorker = original
		detectPPTXArtifactImageText = originalDetector
	}()

	imageLLM := &fakeLLMClient{imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"}}
	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), imageLLM, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral palette with cyan and amber accents",
			LayoutRhythm:    "dark cards, clear hierarchy, restrained density",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions should keep artifact worker path alive after OCR rejection: %v", err)
	}
	if imageLLM.imageCalls != 4 {
		t.Fatalf("image calls = %d, want each default plate retried once before native-motif fallback", imageLLM.imageCalls)
	}
	if len(captured.VisualAssets) != 0 {
		t.Fatalf("captured.VisualAssets = %#v, want native-motif fallback after OCR rejection", captured.VisualAssets)
	}
	if !containsIssueCode(warnings, "WARN_PPT_IMAGE_DEGRADED") {
		t.Fatalf("warnings = %+v, want WARN_PPT_IMAGE_DEGRADED", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactReferenceLearningRetriesTextBearingPlate(t *testing.T) {
	original := runPPTXArtifactWorker
	originalDetector := detectPPTXArtifactImageText
	var captured pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		captured = req
		if len(req.VisualAssets) != 2 {
			t.Fatalf("VisualAssets = %#v, want default cover and closing plates after OCR retry", req.VisualAssets)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 4, 1)
		output.VisualAssets = len(req.VisualAssets)
		return output, nil
	}
	detectCalls := 0
	detectPPTXArtifactImageText = func(context.Context, string) (string, bool, error) {
		detectCalls++
		if detectCalls%2 == 1 {
			return "Q4 ROADMAP", true, nil
		}
		return "", true, nil
	}
	defer func() {
		runPPTXArtifactWorker = original
		detectPPTXArtifactImageText = originalDetector
	}()

	imageLLM := &fakeLLMClient{imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"}}
	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), imageLLM, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral palette with cyan and amber accents",
			LayoutRhythm:    "dark cards, clear hierarchy, restrained density",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if imageLLM.imageCalls != 4 {
		t.Fatalf("image calls = %d, want one OCR retry for each default cover and closing plate", imageLLM.imageCalls)
	}
	if len(captured.VisualAssets) != 2 {
		t.Fatalf("captured.VisualAssets = %#v, want recovered cover and closing plates", captured.VisualAssets)
	}
	for _, asset := range captured.VisualAssets {
		if asset.TextDetection == nil || asset.TextDetection.Status != "passed" || asset.TextDetection.Attempts != 2 {
			t.Fatalf("asset text detection = %#v for %#v, want passed after retry", asset.TextDetection, asset)
		}
	}
	if containsIssueCode(warnings, "WARN_PPT_IMAGE_DEGRADED") {
		t.Fatalf("warnings = %+v, did not expect degradation after successful OCR retry", warnings)
	}
}

func TestPPTXArtifactNarrativePlanIncludesVisualAssetPlan(t *testing.T) {
	request := pptxArtifactWorkerRequest{
		Title:       "Visual Asset Plan",
		StylePreset: "executive-dark",
		VisualAssets: []pptxArtifactVisualAsset{{
			Path:              "/tmp/slide-01-cover.png",
			Name:              "slide-01-cover.png",
			Slide:             1,
			Frame:             &pptxArtifactAssetFrame{Left: 780, Top: 118, Width: 320, Height: 250},
			TargetAspectRatio: 320.0 / 250.0,
			SourceAspectRatio: 1.5,
			TextDetection:     &pptxArtifactTextCheck{Checked: true, Status: "passed", Attempts: 2},
		}},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "executive-dark",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{{
				Slide:           1,
				Role:            "cover",
				LayoutMode:      "cover-split-visual",
				VisualTreatment: "text-free-visual-plate",
				DisplayTitle:    "Visual Asset Plan",
				VisualIntent:    "Use a right-side text-free plate.",
			}},
		},
	}
	markdown := buildPPTXArtifactNarrativePlanMarkdown(request, pptxArtifactWorkerOutput{VisualAssets: 1}, 2)
	for _, expected := range []string{
		"## Visual Asset Plan",
		"Slide 1 `text-free-visual-plate`",
		"Asset `slide-01-cover.png`",
		"frame left 780 top 118 width 320 height 250",
		"target ratio 1.28",
		"source ratio 1.50",
		"OCR passed after 2 attempts",
	} {
		if !strings.Contains(markdown, expected) {
			t.Fatalf("narrative plan missing %q:\n%s", expected, markdown)
		}
	}
}

func TestPPTXArtifactImageSignalDetectsLowInformationPlate(t *testing.T) {
	root := t.TempDir()
	solid := filepath.Join(root, "solid.png")
	checker := filepath.Join(root, "checker.png")
	writeSolidPNGFixture(t, solid, color.RGBA{R: 18, G: 24, B: 38, A: 255})
	writeCheckerPNGFixture(t, checker, color.RGBA{R: 8, G: 16, B: 32, A: 255}, color.RGBA{R: 245, G: 158, B: 11, A: 255})
	solidBytes, err := os.ReadFile(solid)
	if err != nil {
		t.Fatalf("read solid: %v", err)
	}
	checkerBytes, err := os.ReadFile(checker)
	if err != nil {
		t.Fatalf("read checker: %v", err)
	}
	solidSignal := pptxArtifactImageSignalFromBytes(solidBytes)
	if solidSignal == nil || solidSignal.Status != "low" {
		t.Fatalf("solid signal = %#v, want low", solidSignal)
	}
	checkerSignal := pptxArtifactImageSignalFromBytes(checkerBytes)
	if checkerSignal == nil || checkerSignal.Status != "ok" {
		t.Fatalf("checker signal = %#v, want ok", checkerSignal)
	}
}

func TestPPTXArtifactPreviewQualityIssuesDetectLowContrast(t *testing.T) {
	root := t.TempDir()
	preview := filepath.Join(root, "preview.png")
	writeLowContrastPreviewFixture(t, preview)
	signal, err := validatePPTXArtifactPreviewImage(preview)
	if err != nil {
		t.Fatalf("validate preview: %v", err)
	}
	issues := pptxArtifactPreviewQualityIssues(pptxArtifactWorkerRequest{
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			BuilderRecipe: "codex-reference-learning",
		},
	}, signal)
	found := false
	for _, issue := range issues {
		if issue.Code == "PREVIEW_LOW_CONTRAST" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("preview issues = %+v, want PREVIEW_LOW_CONTRAST; signal=%#v", issues, signal)
	}
}

func TestPPTXArtifactReferenceLearningVisualCoverageRequiresCoverAndClosingVisual(t *testing.T) {
	request := pptxArtifactWorkerRequest{
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway"},
			},
		},
	}
	err := validatePPTXArtifactReferenceLearningVisualCoverage(request, nil, nil)
	var visualErr *pptxArtifactVisualVerdictError
	if !errors.As(err, &visualErr) {
		t.Fatalf("err = %v, want visual verdict coverage error", err)
	}
	if len(visualErr.issues) != 2 {
		t.Fatalf("issues = %+v, want cover and closing coverage issues", visualErr.issues)
	}
	if slides := pptxArtifactVisualAssetRepairSlides(err); len(slides) != 2 || slides[0] != 1 || slides[1] != 4 {
		t.Fatalf("repair slides = %#v, want cover and closing slides", slides)
	}
	if guidance := buildPPTXArtifactVisualIssueGuidance(visualErr.issues); !strings.Contains(guidance, "MISSING_REFERENCE_LEARNING_VISUAL") {
		t.Fatalf("guidance = %q, want missing reference-learning visual guidance", guidance)
	}
}

func TestPPTXArtifactReferenceLearningVisualCoverageAllowsImageOrNativeFallback(t *testing.T) {
	request := pptxArtifactWorkerRequest{
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway"},
			},
		},
	}
	err := validatePPTXArtifactReferenceLearningVisualCoverage(request, []pptxArtifactVisualInspectItem{
		{Slide: 4, Role: "closing-motif-frame"},
	}, []pptxArtifactImageInspectItem{
		{Slide: 1, Path: "/tmp/cover.png"},
	})
	if err != nil {
		t.Fatalf("coverage should allow cover image and closing native fallback: %v", err)
	}
}

func TestPPTXArtifactRepairPromptIncludesPreviewPixelGuidance(t *testing.T) {
	failure := &pptxArtifactVisualVerdictError{
		status: "preview-fail",
		score:  78,
		issues: []pptxArtifactVisualVerdictIssue{{
			Code:     "PREVIEW_LOW_CONTRAST",
			Severity: "error",
			Message:  "Rendered preview has too little luminance variation.",
			Slide:    2,
		}},
	}
	prompt := buildPPTXArtifactDesignRepairPrompt(pptxPayload{
		Title: "Preview Pixel Repair",
		Slides: []officegen.Slide{
			{Title: "Preview Pixel Repair", Layout: "title", IsTitle: true},
			{Title: "Observation", Layout: "content", Points: []string{"Keep hierarchy readable"}},
		},
	}, "Preview Pixel Repair", PPTXBuildOptions{
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory.",
	}, &pptxArtifactDesignPlan{
		DeckIntent:    "concise-reference-style-learning",
		BuilderRecipe: "codex-reference-learning",
		Slides: []pptxArtifactSlideDesignPlan{
			{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", VisualTreatment: "native-shapes"},
			{Slide: 2, Role: "observations", LayoutMode: "observation-cards", VisualTreatment: "native-shapes"},
		},
	}, failure)
	for _, expected := range []string{
		"PREVIEW_LOW_CONTRAST",
		"builderPatch",
		"stronger foreground/background separation",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("repair prompt missing %q:\n%s", expected, prompt)
		}
	}
}

func TestDiscoverPPTXArtifactVisualAssetsPrefersHighContrastCovers(t *testing.T) {
	root := t.TempDir()
	lowContrast := filepath.Join(root, "参考文章/Alpha/cover.png")
	highContrast := filepath.Join(root, "参考文章/Beta/cover.png")
	writeSolidPNGFixture(t, lowContrast, color.RGBA{R: 225, G: 232, B: 236, A: 255})
	writeCheckerPNGFixture(t, highContrast, color.RGBA{R: 8, G: 16, B: 32, A: 255}, color.RGBA{R: 255, G: 170, B: 24, A: 255})

	assets, err := discoverPPTXArtifactVisualAssets(root, 1)
	if err != nil {
		t.Fatalf("discoverPPTXArtifactVisualAssets: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("assets = %#v, want one high-contrast cover", assets)
	}
	if assets[0].Path != highContrast {
		t.Fatalf("first asset = %q, want high-contrast cover %q", assets[0].Path, highContrast)
	}
}

func writeSolidPNGFixture(t *testing.T, path string, fill color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 24, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	writePNGFixture(t, path, img)
}

func writeCheckerPNGFixture(t *testing.T, path string, a, b color.RGBA) {
	t.Helper()
	writeSizedCheckerPNGFixture(t, path, 24, 24, a, b)
}

func writeSizedCheckerPNGFixture(t *testing.T, path string, width, height int, a, b color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if (x/6+y/6)%2 == 0 {
				img.SetRGBA(x, y, a)
			} else {
				img.SetRGBA(x, y, b)
			}
		}
	}
	writePNGFixture(t, path, img)
}

func writePNGFixture(t *testing.T, path string, img image.Image) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalHardFails(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(context.Context, pptxArtifactWorkerRequest, string) (*pptxArtifactWorkerOutput, error) {
		return nil, errors.New("node missing")
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Backend Failure",
		"slides":[
			{"title":"Artifact Backend Failure","layout":"title","subtitle":"Worker route","isTitle":true},
			{"title":"Body","layout":"content","points":["Editable text"]}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Backend Failure", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
	})
	if err == nil || !strings.Contains(err.Error(), "artifact experimental backend failed") || !strings.Contains(err.Error(), "node missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalHardFailsBelowStructuralThreshold(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		data, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{
			{Title: "IMG_PLACEHOLDER_BAD", Layout: "content", Points: []string{"IMG_PLACEHOLDER_BAD"}},
		}, officegen.PPTXOptions{Title: req.Title, Creator: "test", StylePreset: officegen.StylePresetExecutiveDark})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return &pptxArtifactWorkerOutput{OutputPPTX: req.OutputPPTX, ArtifactToolOK: true}, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Backend Bad Review",
		"slides":[
			{"title":"Artifact Backend Bad Review","layout":"title","subtitle":"Bad review","isTitle":true}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Backend Bad Review", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
	})
	if err == nil || !strings.Contains(err.Error(), "structural review score") || !strings.Contains(err.Error(), "PLACEHOLDER_RESIDUE") {
		t.Fatalf("err = %v, want structural hard failure with issue code", err)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalRetriesTextDensityOnce(t *testing.T) {
	original := runPPTXArtifactWorker
	attempts := 0
	var firstSlideRuneCounts []int
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		attempts++
		if attempts == 1 {
			for _, slide := range req.Slides {
				firstSlideRuneCounts = append(firstSlideRuneCounts, textDensityRunes(slide))
			}
			data, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{
				{
					Title:   "Dense Artifact Output",
					Layout:  "content",
					Content: strings.Repeat("dense content ", 120),
					Points: []string{
						strings.Repeat("dense point ", 80),
						strings.Repeat("more dense point ", 80),
					},
				},
				{
					Title:   "Second Dense Artifact Output",
					Layout:  "content",
					Content: strings.Repeat("second dense content ", 120),
					Points: []string{
						strings.Repeat("second dense point ", 80),
						strings.Repeat("second more dense point ", 80),
					},
				},
			}, officegen.PPTXOptions{Title: req.Title, Creator: "test", StylePreset: officegen.StylePresetExecutiveDark})
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
				return nil, err
			}
			return writePPTXArtifactFakeDiagnostics(t, req, 1, 0), nil
		}
		if attempts != 2 {
			t.Fatalf("attempts = %d, want exactly one retry", attempts)
		}
		if req.OutputPPTX == "" || req.OutputPPTX == filepath.Join(filepath.Dir(req.OutputPPTX), "output.pptx") {
			t.Fatalf("retry should use a fresh output path, got %q", req.OutputPPTX)
		}
		compactedDenseSlide := false
		for idx, slide := range req.Slides {
			if idx >= len(firstSlideRuneCounts) || firstSlideRuneCounts[idx] <= 220 {
				continue
			}
			if textDensityRunes(slide) >= firstSlideRuneCounts[idx] {
				t.Fatalf("retry slide %d was not compacted: before=%d after=%d", idx, firstSlideRuneCounts[idx], textDensityRunes(slide))
			}
			compactedDenseSlide = true
		}
		if !compactedDenseSlide {
			t.Fatalf("retry request did not include a compacted dense slide: before=%v after=%v", firstSlideRuneCounts, req.Slides)
		}
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 1, 0), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Backend Retry Density",
		"slides":[
			{"title":"Artifact Backend Retry Density","layout":"title","subtitle":"Retry density","isTitle":true},
			{"title":"Dense Body","layout":"content","content":"Dense content should be compacted before retry.","points":["This point is intentionally long so the retry request has something to shorten before the second worker run.","Another intentionally long point should also be shortened."]}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Backend Retry Density", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_RETRY") {
		t.Fatalf("warnings = %#v, want retry warning", warnings)
	}
}

func TestCompactDeckTextDensityShortensContentAndSubtitle(t *testing.T) {
	slides := []officegen.Slide{
		{
			Title:    strings.Repeat("long title phrase ", 8),
			Subtitle: strings.Repeat("long subtitle phrase ", 8),
			Content:  strings.Repeat("long content phrase ", 10),
			Points: []string{
				strings.Repeat("long point phrase ", 6),
				strings.Repeat("another long point phrase ", 6),
			},
		},
	}
	originalContent := slides[0].Content
	originalSubtitle := slides[0].Subtitle
	originalTitle := slides[0].Title
	compacted := compactDeckTextDensity(slides, 150)
	if got := textDensityRunes(compacted[0]); got > 150 {
		t.Fatalf("textDensityRunes = %d, want <= 150; slide=%+v", got, compacted[0])
	}
	if compacted[0].Content == originalContent {
		t.Fatalf("content was not compacted: %+v", compacted[0])
	}
	if compacted[0].Subtitle == originalSubtitle {
		t.Fatalf("subtitle was not compacted: %+v", compacted[0])
	}
	if compacted[0].Title == originalTitle {
		t.Fatalf("title was not compacted: %+v", compacted[0])
	}
}

func TestSimplifyPPTXArtifactRetrySlideKeepsThreeReferenceObservationCards(t *testing.T) {
	slide := officegen.Slide{
		Title:  "What the Reference Directory Teaches",
		Layout: "content",
		Sections: []officegen.SlideSection{
			{Heading: "Repeatable style", Detail: "Use recurring panels, accent rules, large headings, and compact cards instead of copying one source slide."},
			{Heading: "Structured content", Detail: "Keep slide words, chart labels, metrics, and callouts as selectable presentation objects."},
			{Heading: "Quality loop", Detail: "Let rendered evidence catch overflow, weak contrast, blank pages, and default chart styling before export."},
		},
		Points: []string{"Repeatable style beats single-slide mimicry.", "Important content stays in structured objects.", "Rendered quality checks shape the final layout."},
	}
	simplified := simplifyPPTXArtifactRetrySlide(slide)
	if len(simplified.Sections) != 3 {
		t.Fatalf("sections = %#v, want 3 reference observation cards preserved", simplified.Sections)
	}
	for idx, section := range simplified.Sections {
		if utf8.RuneCountInString(section.Detail) > 72 {
			t.Fatalf("section %d detail was not shortened for repair: %#v", idx, section)
		}
	}
}

func TestPreparePPTXArtifactRetrySlidesRestoresReferenceLearningNarrative(t *testing.T) {
	slides := []officegen.Slide{
		{Title: "Reference Deck", Layout: "title", IsTitle: true, Points: []string{"Style profile", "Card rhythm", "Quality loop"}},
		{
			Title:    "What the Reference Directory Teaches",
			Subtitle: "The useful signal is a visual system, not a literal template.",
			Layout:   "content",
			Sections: []officegen.SlideSection{
				{Heading: "Repeatable style", Detail: "Use recurring panels, accent rules, large headings, and compact cards."},
				{Heading: "Structured content", Detail: "Keep words, labels, metrics, and callouts as selectable objects."},
				{Heading: "Quality loop", Detail: "Use rendered evidence to catch overflow, weak contrast, and chart defaults."},
			},
			Points: []string{"Repeatable style beats single-slide mimicry.", "Important content stays in structured objects.", "Rendered quality checks shape the final layout."},
		},
		{Title: "Fidelity Comes From Enforced Layers", Layout: "chart", Chart: &officegen.ChartData{Type: "bar", Title: "High-fidelity contribution", Categories: []string{"Style Profile", "Layout Rhythm", "Editable Objects", "Readable Flow"}, Values: []float64{78, 86, 82, 80}}},
		{Title: "Reference Style Needs a Builder Loop", Layout: "closing", Content: "Turn reference signals into fresh, structured slides.", Sections: []officegen.SlideSection{{Heading: "Intent", Detail: "Learn palette, hierarchy, density, and rhythm from the reference set."}, {Heading: "Execution", Detail: "Compose text, shapes, imagery, and chart objects deliberately."}}},
	}
	plan := &pptxArtifactDesignPlan{DeckIntent: "concise-reference-style-learning"}
	retry := preparePPTXArtifactRetrySlides(slides, 150, "simplified", plan)
	if len(retry) != 4 {
		t.Fatalf("retry slides = %d, want 4", len(retry))
	}
	if len(retry[1].Sections) != 3 {
		t.Fatalf("observation retry sections = %#v, want 3 curated cards", retry[1].Sections)
	}
	if retry[1].Sections[2].Heading != "Readable hierarchy" {
		t.Fatalf("third observation card = %#v", retry[1].Sections[2])
	}
	if strings.HasSuffix(strings.TrimSpace(retry[3].Content), "into.") {
		t.Fatalf("closing content should not be an incomplete fragment: %q", retry[3].Content)
	}
	if len(retry[3].Sections) < 2 {
		t.Fatalf("closing sections = %#v, want support cards", retry[3].Sections)
	}
}

func TestPreparePPTXArtifactRetrySlidesMinimalKeepsReferenceCardsReadable(t *testing.T) {
	slides := []officegen.Slide{
		{Title: "Reference Deck", Layout: "title", IsTitle: true},
		{
			Title:    "Reference Style Signals",
			Subtitle: "The useful signal is a visual system, not a literal template.",
			Layout:   "content",
			Content:  "Recurring palette, density, and hierarchy become a coherent visual system.",
			Sections: []officegen.SlideSection{
				{Heading: "Repeatable style", Detail: "Use recurring panels, accent rules, large headings, and compact cards."},
				{Heading: "Structured content", Detail: "Keep words, labels, metrics, and callouts as selectable objects."},
				{Heading: "Readable hierarchy", Detail: "Keep contrast, spacing, and title scale consistent across slides."},
			},
		},
		{Title: "Fidelity Comes From Enforced Layers", Layout: "chart", Chart: referenceSignalChart(nil)},
		{Title: "Reference Style Becomes a Reusable System", Layout: "closing"},
	}
	plan := &pptxArtifactDesignPlan{DeckIntent: "concise-reference-style-learning"}
	retry := preparePPTXArtifactRetrySlides(slides, 110, "minimal", plan)
	if len(retry[1].Sections) != 3 {
		t.Fatalf("minimal observation sections = %#v, want 3 curated reference cards", retry[1].Sections)
	}
	expectedDetails := []string{
		"Use repeated panels, accent rules, and compact cards.",
		"Keep words, labels, and chart callouts editable.",
		"Keep contrast, spacing, and title scale consistent across slides.",
	}
	for idx, section := range retry[1].Sections {
		if strings.Contains(section.Heading, "builder loop") || strings.Contains(section.Detail, "turns palette") {
			t.Fatalf("section %d contains split content fragment: %#v", idx, section)
		}
		if section.Detail != expectedDetails[idx] {
			t.Fatalf("section %d detail = %q, want compact retry copy %q", idx, section.Detail, expectedDetails[idx])
		}
	}
	if strings.Contains(retry[1].Content, "palette, density, hierarchy") {
		t.Fatalf("minimal observation content should stay compact, got %q", retry[1].Content)
	}
}

func TestShortenLayoutTextAvoidsWeakTerminalFragments(t *testing.T) {
	for _, tc := range []struct {
		input string
		max   int
	}{
		{input: "Use a calm, repeatable presentation rhythm with a clear cover and consistent page cues.", max: 32},
		{input: "Reference styling works through connected layers.", max: 32},
		{input: "Show one lead series clearly and keep the chart hierarchy simple.", max: 44},
		{input: "Use minimal chart styling so the message stays clear.", max: 44},
		{input: "Slides feel more polished when structure matches the message.", max: 44},
		{input: "Structure choices should be intentional so each slide matches the message.", max: 44},
		{input: "Large titles and compact supporting text keep the message clear.", max: 52},
		{input: "Repeated choices in spacing, alignment, and emphasis create a system.", max: 32},
		{input: "Consistent card spacing, contrast, and hierarchy make the deck feel intentional and easy to scan.", max: 76},
	} {
		got := shortenLayoutText(tc.input, tc.max)
		if strings.HasSuffix(got, "repeatable.") || strings.HasSuffix(got, "through.") || strings.HasSuffix(got, "keep.") || strings.HasSuffix(got, "so.") || strings.HasSuffix(got, "so the message.") || strings.HasSuffix(got, "when structure.") || strings.HasSuffix(got, "keep the message.") || strings.HasSuffix(got, "in spacing.") || strings.HasSuffix(got, "make the deck.") || strings.HasSuffix(got, "feel.") {
			t.Fatalf("shortenLayoutText(%q, %d) = %q, want no dangling terminal fragment", tc.input, tc.max, got)
		}
	}
}

func TestPPTXArtifactVisibleTextRejectsWeakTerminalRepairFragments(t *testing.T) {
	for _, text := range []string{
		"Favor light backgrounds, dark neutrals, and a cool blue accent used",
		"Use clear title bands, simple cards, and moderate whitespace to keep ideas",
		"Use reference signals as intent, then simplify copy until each card carries",
		"Use reference signals as intent, then simplify copy until each card",
		"The useful signal is a visual system, not a",
		"A simple business-safe palette supports.",
		"Consistent structure helps the deck feel.",
		"A reusable style shows up in spacing, hierarchy, and card structure more",
		"Consistent title bands, spacing, and card groupings create a presentation",
		"Strong visual hierarchy makes the main.",
		"Simple recurring structures make short.",
		"A restrained palette uses dark neutrals, cool accents, and selective",
		"The target is a visual system that can be rebuilt, adapted, and kept",
		"Translate recurring cues into repeatable rules instead of recreating any",
		"Chart styling should stay.",
		"Use restrained contrast, cool emphasis, and light surfaces to create a calm",
		"Use restrained color, clear hierarchy, and generous spacing to keep every",
		"A reusable style comes from repeated patterns in hierarchy, spacing",
		"References are most useful when they guide tone and layout discipline",
		"Use recurring panels, accent rules, large headings, and compact cards",
		"Use repeated panels, accent rules, and compact cards instead of copying",
		"Use recurring panels and accent rules instead of copying",
		"Use repeated panels, accent rules, and compact cards instead.",
		"Consistent hierarchy and spacing usually.",
		"A small accent palette helps the chart.",
		"A style-aware builder can infer the system, generate editable slides, and improve",
		"The strongest result comes from an agent loop that interprets references, builds",
		"Codex succeeds when it converts reference signals into an editable deck, then",
	} {
		if !pptxArtifactVisibleTextLooksDangling(text) {
			t.Fatalf("pptxArtifactVisibleTextLooksDangling(%q) = false, want true", text)
		}
	}
}

func TestPPTXArtifactVisibleTextAllowsCodexStyleCoverChips(t *testing.T) {
	for _, text := range []string{"Editable text", "Native chart", "Previewed"} {
		if pptxArtifactVisibleTextHasImplementationLeak(text) {
			t.Fatalf("pptxArtifactVisibleTextHasImplementationLeak(%q) = true, want false for short cover chip", text)
		}
	}
	if !pptxArtifactVisibleTextHasImplementationLeak("This slide uses a native chart object generated by the worker.") {
		t.Fatal("long implementation wording should still be rejected")
	}
}

func TestPPTXArtifactVisibleTextAllowsCompleteClearPhrases(t *testing.T) {
	for _, text := range []string{
		"Hierarchy must stay clear",
		"Labels stay clear and restrained.",
		"Large titles keep the message clear.",
		"Readable hierarchy",
		"Reusable hierarchy",
	} {
		if pptxArtifactVisibleTextLooksDangling(text) {
			t.Fatalf("pptxArtifactVisibleTextLooksDangling(%q) = true, want false", text)
		}
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalHardFailsWithoutPreviewDiagnostics(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.InspectPath, []byte(`{"editableItems":[{"kind":"text"}],"nativeCharts":[],"previews":[]}`), 0o644); err != nil {
			return nil, err
		}
		return &pptxArtifactWorkerOutput{
			OutputPPTX:     req.OutputPPTX,
			InspectPath:    req.InspectPath,
			EditableItems:  1,
			ArtifactToolOK: true,
		}, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Backend Missing Preview",
		"slides":[
			{"title":"Artifact Backend Missing Preview","layout":"title","subtitle":"Missing diagnostics","isTitle":true},
			{"title":"Body","layout":"content","points":["Editable text"]}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Backend Missing Preview", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
	})
	if err == nil || !strings.Contains(err.Error(), "preview images") {
		t.Fatalf("err = %v, want preview diagnostics hard failure", err)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalHardFailsForBlankPreview(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(req.PreviewDir, 0o755); err != nil {
			return nil, err
		}
		previewFiles := make([]string, 0, len(req.Slides))
		for idx := range req.Slides {
			previewPath := filepath.Join(req.PreviewDir, fmt.Sprintf("slide-%02d.png", idx+1))
			writePPTXArtifactBlankPreviewFixture(t, previewPath)
			previewFiles = append(previewFiles, previewPath)
		}
		inspect := map[string]any{
			"editableItems": []map[string]string{{"kind": "text"}},
			"nativeCharts":  []map[string]string{},
			"previews":      previewFiles,
		}
		inspectBytes, err := json.Marshal(inspect)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.InspectPath, inspectBytes, 0o644); err != nil {
			return nil, err
		}
		return &pptxArtifactWorkerOutput{
			OutputPPTX:     req.OutputPPTX,
			PreviewFiles:   previewFiles,
			InspectPath:    req.InspectPath,
			EditableItems:  1,
			ArtifactToolOK: true,
		}, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Backend Blank Preview",
		"slides":[
			{"title":"Artifact Backend Blank Preview","layout":"title","subtitle":"Blank preview","isTitle":true},
			{"title":"Body","layout":"content","points":["Editable text"]}
		]
	}`
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Backend Blank Preview", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
	})
	if err == nil || !strings.Contains(err.Error(), "blank or single-color") {
		t.Fatalf("err = %v, want blank preview hard failure", err)
	}
}

func TestPPTXArtifactVisibleTextQualityRejectsImplementationWording(t *testing.T) {
	err := validatePPTXArtifactVisibleText([]pptxArtifactEditableInspectItem{
		{Role: "chart-insight-body", Text: "Use a native chart object."},
	})
	if err == nil || !strings.Contains(err.Error(), "implementation wording") {
		t.Fatalf("err = %v, want implementation wording failure", err)
	}
}

func TestPPTXArtifactVisibleTextQualityRejectsDanglingFragments(t *testing.T) {
	for _, text := range []string{"Font is the lowest.", "Recurring is the lowest at 4."} {
		err := validatePPTXArtifactVisibleText([]pptxArtifactEditableInspectItem{
			{Role: "chart-insight-body", Text: text},
		})
		if err == nil || !strings.Contains(err.Error(), "looks incomplete") {
			t.Fatalf("text = %q, err = %v, want dangling text failure", text, err)
		}
	}
}

func TestPPTXArtifactVisibleTextQualityRejectsTerminalConnector(t *testing.T) {
	for _, text := range []string{"Keep style learning grounded in reference patterns while.", "Use reference signals because.", "The strongest style signals are.", "Quiet cadence with a clear.", "Use recurring panels, accent rules, large.", "Professional cool-toned contrast with dark ink, muted neutrals, and one to two.", "Codex succeeds when it converts reference signals into an editable deck, then.", "The output is a compact, editable deck system derived."} {
		err := validatePPTXArtifactVisibleText([]pptxArtifactEditableInspectItem{
			{Role: "body", Text: text},
		})
		if err == nil || !strings.Contains(err.Error(), "looks incomplete") {
			t.Fatalf("text = %q, err = %v, want terminal connector failure", text, err)
		}
	}
}

func TestPPTXArtifactVisibleTextQualityAllowsAudienceFacingEditableTopic(t *testing.T) {
	err := validatePPTXArtifactVisibleText([]pptxArtifactEditableInspectItem{
		{Role: "title", Text: "Building Editable Presentations"},
		{Role: "chart-insight-body", Text: "Use the pattern to guide emphasis."},
	})
	if err != nil {
		t.Fatalf("validatePPTXArtifactVisibleText: %v", err)
	}
}

func TestPPTXArtifactDiagnosticsRejectsReferenceLearningChartWithoutDesignedPanel(t *testing.T) {
	workDir := t.TempDir()
	previewDir := filepath.Join(workDir, "preview")
	if err := os.MkdirAll(previewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	previewFiles := make([]string, 0, 4)
	for idx := 0; idx < 4; idx++ {
		previewPath := filepath.Join(previewDir, fmt.Sprintf("slide-%02d.png", idx+1))
		writePPTXArtifactPreviewFixture(t, previewPath, idx)
		previewFiles = append(previewFiles, previewPath)
	}
	inspectPath := filepath.Join(workDir, "inspect.json")
	inspect := pptxArtifactInspectSummary{
		EditableItems: []pptxArtifactEditableInspectItem{
			{Role: "heading", Text: "Cover", Slide: 1, FontSize: 28, TextChars: len("Cover"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 80, Top: 120, Width: 400, Height: 50}},
			{Role: "heading", Text: "Observations", Slide: 2, FontSize: 28, TextChars: len("Observations"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 80, Top: 120, Width: 400, Height: 50}},
			{Role: "heading", Text: "Chart", Slide: 3, FontSize: 28, TextChars: len("Chart"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 80, Top: 120, Width: 400, Height: 50}},
			{Role: "heading", Text: "Closing", Slide: 4, FontSize: 28, TextChars: len("Closing"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 80, Top: 120, Width: 400, Height: 50}},
		},
		NativeCharts: []any{map[string]any{"kind": "bar"}},
		Previews:     previewFiles,
	}
	inspectBytes, err := json.Marshal(inspect)
	if err != nil {
		t.Fatalf("marshal inspect: %v", err)
	}
	if err := os.WriteFile(inspectPath, inspectBytes, 0o644); err != nil {
		t.Fatalf("write inspect: %v", err)
	}
	err = validatePPTXArtifactDiagnostics(pptxArtifactWorkerRequest{
		Slides: []officegen.Slide{
			{Title: "Cover", Layout: "title", IsTitle: true},
			{Title: "Observations", Layout: "content"},
			{Title: "Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing"},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack"},
			},
		},
		InspectPath: inspectPath,
	}, &pptxArtifactWorkerOutput{
		InspectPath:   inspectPath,
		PreviewFiles:  previewFiles,
		EditableItems: len(inspect.EditableItems),
		NativeCharts:  1,
		WorkerVersion: "artifact-experimental-v2",
	})
	if err == nil || !strings.Contains(err.Error(), "reference-learning chart slide") {
		t.Fatalf("err = %v, want reference-learning chart visual structure failure", err)
	}
}

func TestPPTXArtifactRepresentativeReferenceFilesSkipsGeneratedOutputBuckets(t *testing.T) {
	profile := &pptxref.ReferenceStyleProfile{
		SourceFiles: []pptxref.ReferencePPTXFile{
			{Path: "/root/output/generated-a.pptx", SourceBucket: "current-output"},
			{Path: "/root/tmp/scratch.pptx", SourceBucket: "tmp"},
			{Path: "/root/.worktrees/feature/fixture.pptx", SourceBucket: "worktree"},
			{Path: "/root/testdata/fixture.pptx", SourceBucket: "testdata"},
			{Path: "/root/brand.pptx", SourceBucket: "other"},
			{Path: "/root/public/skills-demos/demo.pptx", SourceBucket: "demo-assets"},
			{Path: "/root/output/generated-b.pptx", SourceBucket: "current-output"},
		},
	}
	got := representativeReferenceFiles(PPTXBuildOptions{ReferenceProfile: profile})
	want := []string{"/root/brand.pptx", "/root/public/skills-demos/demo.pptx"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("representativeReferenceFiles = %#v, want stable reference files %#v", got, want)
	}
}

func TestPPTXArtifactRepresentativeReferenceFilesReturnsEmptyForCurrentOutputOnly(t *testing.T) {
	profile := &pptxref.ReferenceStyleProfile{
		SourceFiles: []pptxref.ReferencePPTXFile{
			{Path: "/root/output/generated-a.pptx", SourceBucket: "current-output"},
			{Path: "/root/output/generated-b.pptx", SourceBucket: "current-output"},
		},
	}
	if got := representativeReferenceFiles(PPTXBuildOptions{ReferenceProfile: profile}); len(got) != 0 {
		t.Fatalf("representativeReferenceFiles = %#v, want no generated-output references", got)
	}
}

func TestPPTXArtifactDesignPlanUsesCodexReferenceLearningRecipe(t *testing.T) {
	payload := pptxPayload{
		Title: "Reference Style Test",
		Slides: []officegen.Slide{
			{Title: "Reference Style Test", Layout: "title", IsTitle: true},
			{Title: "Key Observations", Layout: "content", Points: []string{"Reference style"}},
			{Title: "Simple Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing"},
		},
	}
	plan := buildPPTXArtifactDesignPlan(payload, "Create a concise editable presentation that learns the style from PPTX files in this directory.", PPTXBuildOptions{
		UserPrompt:     "Create a concise editable presentation that learns the style from PPTX files in this directory.",
		ReferenceBrief: &PPTXReferenceStyleBrief{StylePresetHint: officegen.StylePresetExecutiveDark},
	})
	if plan == nil {
		t.Fatal("buildPPTXArtifactDesignPlan returned nil")
	}
	if plan.BuilderRecipe != "codex-reference-learning" {
		t.Fatalf("BuilderRecipe = %q, want codex-reference-learning", plan.BuilderRecipe)
	}
}

func TestPPTXArtifactEditableTextLayoutRejectsTooSmallFont(t *testing.T) {
	err := validatePPTXArtifactEditableTextLayout([]pptxArtifactEditableInspectItem{
		{Role: "body", Text: "Unreadable annotation", Slide: 1, FontSize: 9, TextChars: len("Unreadable annotation"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 100, Width: 240, Height: 30}},
	})
	if err == nil || !strings.Contains(err.Error(), "too-small font") {
		t.Fatalf("err = %v, want too-small font failure", err)
	}
}

func TestPPTXArtifactEditableTextLayoutRejectsOverlappingText(t *testing.T) {
	err := validatePPTXArtifactEditableTextLayout([]pptxArtifactEditableInspectItem{
		{Role: "heading", Text: "First card", Slide: 2, FontSize: 18, TextChars: len("First card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 220, Height: 60}},
		{Role: "body", Text: "Second card", Slide: 2, FontSize: 16, TextChars: len("Second card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 120, Top: 138, Width: 220, Height: 60}},
	})
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("err = %v, want overlap failure", err)
	}
}

func TestPPTXArtifactEditableTextLayoutRejectsTooManyTextObjects(t *testing.T) {
	items := make([]pptxArtifactEditableInspectItem, 15)
	for idx := range items {
		text := fmt.Sprintf("Card %02d", idx+1)
		items[idx] = pptxArtifactEditableInspectItem{
			Role:      "body",
			Text:      text,
			Slide:     2,
			FontSize:  14,
			TextChars: len(text),
			TextLines: 1,
			BBox:      pptxArtifactInspectBBox{Left: 80 + float64(idx%5)*210, Top: 120 + float64(idx/5)*70, Width: 180, Height: 38},
		}
	}
	err := validatePPTXArtifactEditableTextLayout(items)
	if err == nil || !strings.Contains(err.Error(), "too many text objects") {
		t.Fatalf("err = %v, want too many text objects failure", err)
	}
}

func TestPPTXArtifactEditableTextLayoutRejectsNarrowLongTextBox(t *testing.T) {
	text := "This long callout is likely to wrap badly"
	err := validatePPTXArtifactEditableTextLayout([]pptxArtifactEditableInspectItem{
		{Role: "body", Text: text, Slide: 2, FontSize: 14, TextChars: len(text), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 140, Height: 60}},
	})
	if err == nil || !strings.Contains(err.Error(), "too narrow text box") {
		t.Fatalf("err = %v, want narrow text box failure", err)
	}
}

func TestPPTXArtifactEditableTextLayoutAllowsCleanRecords(t *testing.T) {
	err := validatePPTXArtifactEditableTextLayout([]pptxArtifactEditableInspectItem{
		{Role: "heading", Text: "First card", Slide: 2, FontSize: 18, TextChars: len("First card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 220, Height: 40}},
		{Role: "body", Text: "Second card", Slide: 2, FontSize: 16, TextChars: len("Second card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 190, Width: 220, Height: 40}},
	})
	if err != nil {
		t.Fatalf("validatePPTXArtifactEditableTextLayout: %v", err)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalRetriesSimplifiedForLayoutDiagnostics(t *testing.T) {
	original := runPPTXArtifactWorker
	var calls int
	var retryRequest pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		calls++
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(req.PreviewDir, 0o755); err != nil {
			return nil, err
		}
		previewFiles := make([]string, 0, len(req.Slides))
		for idx := range req.Slides {
			previewPath := filepath.Join(req.PreviewDir, fmt.Sprintf("slide-%02d.png", idx+1))
			writePPTXArtifactPreviewFixture(t, previewPath, idx)
			previewFiles = append(previewFiles, previewPath)
		}
		var editable []pptxArtifactEditableInspectItem
		if calls == 1 {
			editable = []pptxArtifactEditableInspectItem{
				{Role: "heading", Text: "First card", Slide: 2, FontSize: 18, TextChars: len("First card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 220, Height: 60}},
				{Role: "body", Text: "Second card", Slide: 2, FontSize: 16, TextChars: len("Second card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 120, Top: 138, Width: 220, Height: 60}},
			}
		} else {
			retryRequest = req
			editable = []pptxArtifactEditableInspectItem{
				{Role: "heading", Text: "First card", Slide: 2, FontSize: 18, TextChars: len("First card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 220, Height: 40}},
				{Role: "body", Text: "Second card", Slide: 2, FontSize: 16, TextChars: len("Second card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 190, Width: 220, Height: 40}},
			}
		}
		inspect := pptxArtifactInspectSummary{
			EditableItems: editable,
			NativeCharts:  []any{},
			Previews:      previewFiles,
		}
		inspectBytes, err := json.Marshal(inspect)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.InspectPath, inspectBytes, 0o644); err != nil {
			return nil, err
		}
		return &pptxArtifactWorkerOutput{
			OutputPPTX:     req.OutputPPTX,
			PreviewFiles:   previewFiles,
			InspectPath:    req.InspectPath,
			EditableItems:  len(editable),
			ArtifactToolOK: true,
		}, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Layout Repair",
		"slides":[
			{"title":"Artifact Layout Repair","layout":"title","subtitle":"Layout repair smoke","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["First card should remain readable","Second card should remain readable","Third card should be removed on repair"],"sections":[
				{"heading":"One","detail":"First detail"},
				{"heading":"Two","detail":"Second detail"},
				{"heading":"Three","detail":"Third detail"}
			]}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Layout Repair", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if calls != 2 {
		t.Fatalf("worker calls = %d, want 2", calls)
	}
	if retryRequest.RepairMode != "simplified" {
		t.Fatalf("retry repair mode = %q, want simplified", retryRequest.RepairMode)
	}
	if len(retryRequest.Slides) < 2 || len(retryRequest.Slides[1].Points) > 2 || len(retryRequest.Slides[1].Sections) > 2 {
		t.Fatalf("retry request was not simplified: %+v", retryRequest.Slides)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_RETRY") {
		t.Fatalf("warnings = %+v, want WARN_PPTX_ARTIFACT_RETRY", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalRetriesSimplifiedForVisualVerdict(t *testing.T) {
	original := runPPTXArtifactWorker
	var calls int
	var retryRequest pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		calls++
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(req.PreviewDir, 0o755); err != nil {
			return nil, err
		}
		previewFiles := make([]string, 0, len(req.Slides))
		for idx := range req.Slides {
			previewPath := filepath.Join(req.PreviewDir, fmt.Sprintf("slide-%02d.png", idx+1))
			writePPTXArtifactPreviewFixture(t, previewPath, idx)
			previewFiles = append(previewFiles, previewPath)
		}
		editable := []pptxArtifactEditableInspectItem{
			{Role: "heading", Text: "Visual verdict deck", Slide: 1, FontSize: 24, TextChars: len("Visual verdict deck"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 420, Height: 48}},
			{Role: "body", Text: "Reference style signal", Slide: 2, FontSize: 16, TextChars: len("Reference style signal"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 210, Width: 520, Height: 42}},
		}
		verdict := map[string]any{
			"status": "pass",
			"score":  92,
		}
		if calls == 1 {
			verdict = map[string]any{
				"status": "fail",
				"score":  62,
				"issues": []map[string]any{{
					"code":     "CONTENT_DENSITY_HIGH",
					"severity": "error",
					"message":  "Slide has too much text density for a clean preview.",
					"slide":    2,
				}},
			}
		} else {
			retryRequest = req
		}
		inspect := map[string]any{
			"editableItems": editable,
			"nativeCharts":  []any{},
			"previews":      previewFiles,
			"visualVerdict": verdict,
		}
		inspectBytes, err := json.Marshal(inspect)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.InspectPath, inspectBytes, 0o644); err != nil {
			return nil, err
		}
		return &pptxArtifactWorkerOutput{
			OutputPPTX:     req.OutputPPTX,
			PreviewFiles:   previewFiles,
			InspectPath:    req.InspectPath,
			EditableItems:  len(editable),
			ArtifactToolOK: true,
		}, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Visual Verdict Repair",
		"slides":[
			{"title":"Artifact Visual Verdict Repair","layout":"title","subtitle":"Visual repair smoke","isTitle":true},
			{"title":"Dense Observations","layout":"content","points":["First dense card should be simplified","Second dense card should be simplified","Third dense card should be removed"],"sections":[
				{"heading":"One","detail":"First detail"},
				{"heading":"Two","detail":"Second detail"},
				{"heading":"Three","detail":"Third detail"}
			]}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Visual Verdict Repair", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if calls != 2 {
		t.Fatalf("worker calls = %d, want 2", calls)
	}
	if retryRequest.RepairMode != "simplified" {
		t.Fatalf("retry repair mode = %q, want simplified", retryRequest.RepairMode)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_RETRY") {
		t.Fatalf("warnings = %+v, want WARN_PPTX_ARTIFACT_RETRY", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalRetriesWithLLMDesignRepair(t *testing.T) {
	original := runPPTXArtifactWorker
	var calls int
	var repairedRequest pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		calls++
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(req.PreviewDir, 0o755); err != nil {
			return nil, err
		}
		previewFiles := make([]string, 0, len(req.Slides))
		for idx := range req.Slides {
			previewPath := filepath.Join(req.PreviewDir, fmt.Sprintf("slide-%02d.png", idx+1))
			writePPTXArtifactPreviewFixture(t, previewPath, idx)
			previewFiles = append(previewFiles, previewPath)
		}
		editable := []pptxArtifactEditableInspectItem{
			{Role: "heading", Text: "Design repair deck", Slide: 1, FontSize: 24, TextChars: len("Design repair deck"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 420, Height: 48}},
			{Role: "body", Text: "Reference style signal", Slide: 2, FontSize: 16, TextChars: len("Reference style signal"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 210, Width: 520, Height: 42}},
		}
		verdict := map[string]any{"status": "pass", "score": 92}
		if calls == 1 {
			verdict = map[string]any{
				"status": "fail",
				"score":  64,
				"issues": []map[string]any{{
					"code":     "CONTENT_DENSITY_HIGH",
					"severity": "error",
					"message":  "Slide has too many editable text blocks for a clean preview.",
					"slide":    2,
				}},
			}
		} else {
			repairedRequest = req
		}
		inspect := map[string]any{
			"editableItems": editable,
			"nativeCharts":  []any{},
			"previews":      previewFiles,
			"visualVerdict": verdict,
		}
		inspectBytes, err := json.Marshal(inspect)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.InspectPath, inspectBytes, 0o644); err != nil {
			return nil, err
		}
		return &pptxArtifactWorkerOutput{
			OutputPPTX:     req.OutputPPTX,
			PreviewFiles:   previewFiles,
			InspectPath:    req.InspectPath,
			EditableItems:  len(editable),
			ArtifactToolOK: true,
		}, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Design Repair",
		"slides":[
			{"title":"Artifact Design Repair","layout":"title","subtitle":"Design repair smoke","isTitle":true},
			{"title":"Dense Observations","layout":"content","points":["First dense card should be rewritten","Second dense card should be rewritten","Third dense card should be dropped"]}
		]
	}`
	planJSON := func(cardHeading, cardDetail string) string {
		return fmt.Sprintf(`{
			"deckIntent":"semantic-editable-deck",
			"styleBias":"dark-structured",
			"slides":[
				{"slide":1,"role":"cover","layoutMode":"cover-split-visual","visualTreatment":"native-shapes","densityTarget":"spacious","kicker":"","takeaway":"Design repair smoke.","visualIntent":"Use a cover panel.","cards":[],"chartCallouts":[]},
				{"slide":2,"role":"content","layoutMode":"section-cards","visualTreatment":"native-shapes","densityTarget":"balanced","kicker":"","takeaway":"Review structure before details.","visualIntent":"Use native cards.","cards":[],"chartCallouts":[]},
				{"slide":3,"role":"content","layoutMode":"content-cards","visualTreatment":"native-shapes","densityTarget":"spacious","kicker":"","takeaway":"Set up the evidence clearly.","visualIntent":"Use native cards.","cards":[],"chartCallouts":[]},
				{"slide":4,"role":"observations","layoutMode":"observation-cards","visualTreatment":"native-shapes","densityTarget":"spacious","kicker":"KEY OBSERVATIONS","takeaway":"A tighter card set improves scan quality.","visualIntent":"Use fewer, stronger cards.","cards":[
					{"heading":%q,"detail":%q},
					{"heading":"Cleaner scan","detail":"Short support copy keeps the layout readable."}
				],"chartCallouts":[]},
				{"slide":5,"role":"content","layoutMode":"content-cards","visualTreatment":"native-shapes","densityTarget":"balanced","kicker":"","takeaway":"Support the headline with decision-grade facts.","visualIntent":"Use native cards.","cards":[],"chartCallouts":[]},
				{"slide":6,"role":"content","layoutMode":"content-cards","visualTreatment":"native-shapes","densityTarget":"spacious","kicker":"","takeaway":"Shift the story before the final slide.","visualIntent":"Use native cards.","cards":[],"chartCallouts":[]},
				{"slide":7,"role":"closing","layoutMode":"closing-takeaway","visualTreatment":"native-shapes","densityTarget":"compact","kicker":"Recommendation","takeaway":"Approve one focused validation cycle.","visualIntent":"Use a restrained closing panel.","cards":[],"chartCallouts":[]}
			]
		}`, cardHeading, cardDetail)
	}
	initialPlan := planJSON("Dense card one", "Too much copy for the preview.")
	repairedPlan := planJSON("Fewer cards", "Keep only the strongest style signals.")
	llm := &fakeLLMClient{structuredResponses: []string{initialPlan, repairedPlan}}
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, nil, content, "Artifact Design Repair", "", false, false, PPTXBuildOptions{
		Backend:                    PPTXBackendArtifactWorker,
		GenerateArtifactDesignPlan: true,
		UserPrompt:                 "Create a concise editable presentation that learns the style from PPTX files in this directory.",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if calls != 2 {
		t.Fatalf("worker calls = %d, want 2", calls)
	}
	if llm.structuredCallCount != 2 {
		t.Fatalf("structured calls = %d, want initial plan plus repair", llm.structuredCallCount)
	}
	if repairedRequest.RepairMode != "design-repair" {
		t.Fatalf("repair mode = %q, want design-repair; structured=%d warnings=%+v plan=%#v", repairedRequest.RepairMode, llm.structuredCallCount, warnings, repairedRequest.DesignPlan)
	}
	if repairedRequest.DesignPlan == nil || len(repairedRequest.DesignPlan.Slides) < 2 {
		t.Fatalf("repaired design plan missing: %#v", repairedRequest.DesignPlan)
	}
	if got := repairedRequest.DesignPlan.Slides[3].Cards[0].Heading; got != "Fewer cards" {
		t.Fatalf("repaired card heading = %q, want Fewer cards", got)
	}
	if len(repairedRequest.Slides) != 7 {
		t.Fatalf("design repair should keep expanded semantic slides before rerender: got %d slides", len(repairedRequest.Slides))
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_DESIGN_REPAIR_APPLIED") {
		t.Fatalf("warnings = %+v, want WARN_PPTX_ARTIFACT_DESIGN_REPAIR_APPLIED", warnings)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_RETRY") {
		t.Fatalf("warnings = %+v, want WARN_PPTX_ARTIFACT_RETRY", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalRepairPromptIncludesVisualIssueGuidance(t *testing.T) {
	original := runPPTXArtifactWorker
	originalStructureReview := runPPTXArtifactStructureReview
	var calls int
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		calls++
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(req.PreviewDir, 0o755); err != nil {
			return nil, err
		}
		previewFiles := make([]string, 0, len(req.Slides))
		for idx := range req.Slides {
			previewPath := filepath.Join(req.PreviewDir, fmt.Sprintf("slide-%02d.png", idx+1))
			writePPTXArtifactPreviewFixture(t, previewPath, idx)
			previewFiles = append(previewFiles, previewPath)
		}
		verdict := map[string]any{"status": "pass", "score": 94}
		if calls == 1 {
			verdict = map[string]any{
				"status": "fail",
				"score":  58,
				"issues": []map[string]any{{
					"code":     "LOW_INFORMATION_VISUAL_ASSET",
					"severity": "error",
					"message":  "A generated visual plate has too little luminance variation and may read as blank.",
					"slide":    1,
				}, {
					"code":     "VISUAL_ASSET_ASPECT_RATIO_MISMATCH",
					"severity": "error",
					"message":  "The source ratio differs too much from the planned frame.",
					"slide":    1,
				}},
			}
		}
		var nativeCharts []map[string]any
		for _, slide := range req.Slides {
			if slide.Chart == nil {
				continue
			}
			nativeCharts = append(nativeCharts, map[string]any{
				"kind":       firstNonEmpty(slide.Chart.Type, "bar"),
				"title":      slide.Chart.Title,
				"categories": len(slide.Chart.Categories),
				"values":     len(slide.Chart.Values),
			})
		}
		var images []map[string]any
		imageSlides := map[int]bool{}
		for _, asset := range req.VisualAssets {
			if asset.Slide <= 0 {
				continue
			}
			imageSlides[asset.Slide] = true
			images = append(images, map[string]any{
				"path":  asset.Path,
				"slide": asset.Slide,
				"bbox": map[string]any{
					"left":   780,
					"top":    118,
					"width":  320,
					"height": 250,
				},
			})
		}
		var visualItems []map[string]any
		if req.DesignPlan != nil {
			for _, slide := range req.DesignPlan.Slides {
				if slide.Slide <= 0 || imageSlides[slide.Slide] {
					continue
				}
				role := ""
				switch strings.TrimSpace(slide.Role) {
				case "cover":
					role = "fallback-motif-signal-panel"
				case "closing":
					role = "closing-motif-frame"
				}
				if role == "" {
					continue
				}
				visualItems = append(visualItems, map[string]any{
					"kind":  "shape",
					"role":  role,
					"slide": slide.Slide,
					"bbox": map[string]any{
						"left":   784,
						"top":    120,
						"width":  326,
						"height": 226,
					},
				})
			}
		}
		inspect := map[string]any{
			"editableItems": []pptxArtifactEditableInspectItem{
				{Role: "heading", Text: "Visual repair deck", Slide: 1, FontSize: 24, TextChars: len("Visual repair deck"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 420, Height: 48}},
			},
			"images":        images,
			"nativeCharts":  nativeCharts,
			"previews":      previewFiles,
			"visualItems":   visualItems,
			"visualVerdict": verdict,
		}
		inspectBytes, err := json.Marshal(inspect)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.InspectPath, inspectBytes, 0o644); err != nil {
			return nil, err
		}
		return &pptxArtifactWorkerOutput{
			OutputPPTX:     req.OutputPPTX,
			PreviewFiles:   previewFiles,
			InspectPath:    req.InspectPath,
			EditableItems:  1,
			ArtifactToolOK: true,
		}, nil
	}
	runPPTXArtifactStructureReview = func(context.Context, string) error {
		return nil
	}
	defer func() {
		runPPTXArtifactWorker = original
		runPPTXArtifactStructureReview = originalStructureReview
	}()

	content := `{
		"title":"Visual Repair Guidance",
		"slides":[
			{"title":"Visual Repair Guidance","layout":"title","subtitle":"Repair visual issue","isTitle":true}
		]
	}`
	planJSON := func(firstTreatment, firstIntent string) string {
		return fmt.Sprintf(`{
			"deckIntent":"concise-reference-style-learning",
			"styleBias":"executive-dark",
			"builderRecipe":"codex-reference-learning",
			"slides":[
				{"slide":1,"role":"cover","layoutMode":"cover-split-visual","visualTreatment":%q,"densityTarget":"spacious","kicker":"REFERENCE","displayTitle":"Visual Repair Guidance","displaySubtitle":"Repair visual issue","displayBody":"Keep the slide editable.","takeaway":"Repair the visual layer.","visualIntent":%q,"cards":[],"chartCallouts":[]},
				{"slide":2,"role":"content","layoutMode":"content-cards","visualTreatment":"native-shapes","densityTarget":"balanced","kicker":"","displayTitle":"Context","displaySubtitle":"Reference style setup","displayBody":"Keep the setup concise.","takeaway":"Use the source directory as intent.","visualIntent":"Use native motifs.","cards":[],"chartCallouts":[]},
				{"slide":3,"role":"content","layoutMode":"content-cards","visualTreatment":"native-shapes","densityTarget":"balanced","kicker":"","displayTitle":"Style System","displaySubtitle":"Reusable hierarchy","displayBody":"Use repeated cards and rails.","takeaway":"System beats mimicry.","visualIntent":"Use native motifs.","cards":[],"chartCallouts":[]},
				{"slide":4,"role":"observations","layoutMode":"observation-cards","visualTreatment":"native-shapes","densityTarget":"balanced","kicker":"KEY OBSERVATIONS","displayTitle":"What the reference directory teaches","displaySubtitle":"System, not template.","displayBody":"Keep observations concise.","takeaway":"Use recurring choices as a system.","visualIntent":"Use three cards.","cards":[{"heading":"Repeatable style","detail":"Use repeated hierarchy and spacing."},{"heading":"Editable content","detail":"Keep important words editable."}],"chartCallouts":[]},
				{"slide":5,"role":"content","layoutMode":"content-cards","visualTreatment":"native-shapes","densityTarget":"balanced","kicker":"","displayTitle":"Execution Pattern","displaySubtitle":"Builder and preview","displayBody":"Use preview evidence to improve layout.","takeaway":"Preview informs the next pass.","visualIntent":"Use native motifs.","cards":[],"chartCallouts":[]},
				{"slide":6,"role":"content","layoutMode":"content-cards","visualTreatment":"native-shapes","densityTarget":"balanced","kicker":"","displayTitle":"Quality Gate","displaySubtitle":"Visual issues drive repair","displayBody":"Reject weak visual plates.","takeaway":"Quality checks should be actionable.","visualIntent":"Use native motifs.","cards":[],"chartCallouts":[]},
				{"slide":7,"role":"closing","layoutMode":"closing-takeaway","visualTreatment":"native-shapes","densityTarget":"compact","kicker":"CLOSING","displayTitle":"Reference style becomes reusable","displaySubtitle":"Keep the system editable","displayBody":"Carry hierarchy into future decks.","takeaway":"A reusable system beats a copied template.","visualIntent":"Use a restrained closing panel.","cards":[],"chartCallouts":[]}
			]
		}`, firstTreatment, firstIntent)
	}
	initialPlan := planJSON("text-free-visual-plate", "Use a text-free plate.")
	repairedPlan := planJSON("native-shapes", "Replace the weak plate with native editable motifs.")
	llm := &fakeLLMClient{structuredResponses: []string{initialPlan, repairedPlan}}
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, nil, content, "Visual Repair Guidance", "", false, false, PPTXBuildOptions{
		Backend:                    PPTXBackendArtifactWorker,
		GenerateArtifactDesignPlan: true,
		UserPrompt:                 "Create a concise editable presentation that learns the style from PPTX files in this directory.",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if calls != 2 {
		t.Fatalf("worker calls = %d, want initial plus design repair", calls)
	}
	if llm.structuredCallCount != 2 {
		t.Fatalf("structured calls = %d, want initial plan plus repair", llm.structuredCallCount)
	}
	prompt := llm.lastStructuredReq.Messages[0].Content
	for _, expected := range []string{
		"LOW_INFORMATION_VISUAL_ASSET",
		"replace or regenerate",
		"VISUAL_ASSET_ASPECT_RATIO_MISMATCH",
		"aspect ratio matches the source plate",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("repair prompt missing %q:\n%s", expected, prompt)
		}
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_DESIGN_REPAIR_APPLIED") {
		t.Fatalf("warnings = %+v, want design repair applied", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalRepairsWeakVisualAssetBeforeDesignRepair(t *testing.T) {
	original := runPPTXArtifactWorker
	originalDetector := detectPPTXArtifactImageText
	var calls int
	var assetRepairRequest pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		calls++
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(req.PreviewDir, 0o755); err != nil {
			return nil, err
		}
		previewFiles := make([]string, 0, len(req.Slides))
		for idx := range req.Slides {
			previewPath := filepath.Join(req.PreviewDir, fmt.Sprintf("slide-%02d.png", idx+1))
			writePPTXArtifactPreviewFixture(t, previewPath, idx)
			previewFiles = append(previewFiles, previewPath)
		}
		verdict := map[string]any{"status": "pass", "score": 94}
		if calls == 1 {
			verdict = map[string]any{
				"status": "fail",
				"score":  58,
				"issues": []map[string]any{{
					"code":     "LOW_INFORMATION_VISUAL_ASSET",
					"severity": "error",
					"message":  "The cover plate is nearly blank.",
					"slide":    1,
				}},
			}
		} else {
			assetRepairRequest = req
		}
		var inspectImages []map[string]any
		if calls > 1 {
			for _, asset := range req.VisualAssets {
				inspectImages = append(inspectImages, map[string]any{
					"path":  asset.Path,
					"slide": asset.Slide,
					"bbox": map[string]any{
						"left":   780,
						"top":    118,
						"width":  320,
						"height": 250,
					},
				})
			}
		}
		inspect := map[string]any{
			"editableItems": []pptxArtifactEditableInspectItem{
				{Role: "heading", Text: "Asset repair deck", Slide: 1, FontSize: 24, TextChars: len("Asset repair deck"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 420, Height: 48}},
			},
			"visualItems": []map[string]any{{
				"kind":  "shape",
				"role":  "closing-motif-frame",
				"slide": 4,
				"bbox": map[string]any{
					"left":   784,
					"top":    120,
					"width":  326,
					"height": 226,
				},
			}},
			"images": inspectImages,
			"nativeCharts": []map[string]any{{
				"kind":       "bar",
				"title":      "Quality signal",
				"categories": 2,
				"values":     2,
			}},
			"previews":      previewFiles,
			"visualVerdict": verdict,
		}
		inspectBytes, err := json.Marshal(inspect)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.InspectPath, inspectBytes, 0o644); err != nil {
			return nil, err
		}
		return &pptxArtifactWorkerOutput{
			OutputPPTX:     req.OutputPPTX,
			PreviewFiles:   previewFiles,
			InspectPath:    req.InspectPath,
			EditableItems:  1,
			ArtifactToolOK: true,
		}, nil
	}
	detectPPTXArtifactImageText = func(context.Context, string) (string, bool, error) {
		return "", true, nil
	}
	defer func() {
		runPPTXArtifactWorker = original
		detectPPTXArtifactImageText = originalDetector
	}()

	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	llm := &fakeLLMClient{imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"}}
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, nil, content, "PPT Reference Style Test", "", true, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ReferenceBrief: &PPTXReferenceStyleBrief{
			StylePresetHint: "executive-dark",
			PaletteIntent:   "dark neutral palette with cyan and amber accents",
			LayoutRhythm:    "dark cards, clear hierarchy, restrained density",
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if calls != 2 {
		t.Fatalf("worker calls = %d, want initial plus asset repair", calls)
	}
	if assetRepairRequest.RepairMode != "asset-repair" {
		t.Fatalf("repair mode = %q, want asset-repair", assetRepairRequest.RepairMode)
	}
	if len(assetRepairRequest.VisualAssets) == 0 || !strings.Contains(filepath.ToSlash(assetRepairRequest.VisualAssets[0].Path), "artifact-text-free-plates-repair") {
		t.Fatalf("asset repair did not replace the cover plate: %#v", assetRepairRequest.VisualAssets)
	}
	if llm.imageCalls != 3 {
		t.Fatalf("image calls = %d, want initial cover and closing plates plus 1 repair plate", llm.imageCalls)
	}
	if !strings.Contains(strings.ToLower(llm.lastImageRequest.Prompt), "repair correction") || !strings.Contains(strings.ToLower(llm.lastImageRequest.Prompt), "luminance variation") {
		t.Fatalf("repair image prompt missing visual issue correction:\n%s", llm.lastImageRequest.Prompt)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_ASSET_REPAIR_APPLIED") {
		t.Fatalf("warnings = %+v, want asset repair applied", warnings)
	}
	if containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_DESIGN_REPAIR_APPLIED") {
		t.Fatalf("warnings = %+v, did not expect design repair when asset repair passes", warnings)
	}
}

func TestPPTXArtifactPolishEvidenceIncludesWorkerVisualIssues(t *testing.T) {
	output := &pptxArtifactWorkerOutput{
		WorkerVersion: "artifact-experimental-test",
		OutputPPTX:    "/tmp/output.pptx",
		InspectPath:   "",
		PreviewFiles:  []string{"/tmp/slide-01.png"},
		EditableItems: 8,
		VisualVerdict: "pass",
		VisualScore:   88,
		VisualIssues: []string{
			"LOW_INFORMATION_VISUAL_ASSET on slide 1: plate was weak but non-blocking",
		},
	}
	evidence := summarizePPTXArtifactPolishEvidence(output)
	if !strings.Contains(evidence, "LOW_INFORMATION_VISUAL_ASSET") {
		t.Fatalf("polish evidence missing visual issue summary:\n%s", evidence)
	}
}

func TestPPTXArtifactPolishEvidenceIncludesPreviewPixelSignals(t *testing.T) {
	dir := t.TempDir()
	previewA := filepath.Join(dir, "slide-01.png")
	previewB := filepath.Join(dir, "slide-02.png")
	writePPTXArtifactPreviewFixture(t, previewA, 0)
	writeLowContrastPreviewFixture(t, previewB)

	output := &pptxArtifactWorkerOutput{
		WorkerVersion: "artifact-experimental-test",
		OutputPPTX:    filepath.Join(dir, "output.pptx"),
		PreviewFiles:  []string{previewA, previewB},
		EditableItems: 8,
		VisualVerdict: "pass",
		VisualScore:   92,
	}
	evidence := summarizePPTXArtifactPolishEvidence(output)
	var summary map[string]any
	if err := json.Unmarshal([]byte(evidence), &summary); err != nil {
		t.Fatalf("unmarshal polish evidence: %v\n%s", err, evidence)
	}
	signals, ok := summary["previewSignals"].([]any)
	if !ok || len(signals) != 2 {
		t.Fatalf("previewSignals = %#v, want 2 entries in evidence:\n%s", summary["previewSignals"], evidence)
	}
	first, ok := signals[0].(map[string]any)
	if !ok {
		t.Fatalf("previewSignals[0] = %#v, want object", signals[0])
	}
	for _, key := range []string{"width", "height", "meanLuma", "lumaRange", "lumaStdDev", "distinctColors"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("preview signal missing %q: %#v", key, first)
		}
	}
	if got, _ := first["distinctColors"].(float64); got < 2 {
		t.Fatalf("distinctColors = %v, want varied rendered preview signal", first["distinctColors"])
	}
	if got, _ := first["lumaRange"].(float64); got < 100 {
		t.Fatalf("lumaRange = %v, want strong contrast signal", first["lumaRange"])
	}
	second, ok := signals[1].(map[string]any)
	if !ok {
		t.Fatalf("previewSignals[1] = %#v, want object", signals[1])
	}
	if got, _ := second["lumaRange"].(float64); got > 20 {
		t.Fatalf("low contrast lumaRange = %v, want compact low-contrast signal", second["lumaRange"])
	}
}

func TestPPTXArtifactQualitySummaryFailsOnVisualAndPreviewIssues(t *testing.T) {
	summary := buildPPTXArtifactQualitySummary(pptxArtifactWorkerOutput{
		WorkerVersion: "artifact-experimental-test",
		PreviewFiles:  []string{"slide-01.png"},
		EditableItems: 3,
		NativeCharts:  0,
		VisualVerdict: "pass",
		VisualScore:   88,
		VisualIssues:  []string{"LOW_INFORMATION_VISUAL_ASSET on slide 1"},
		PreviewIssues: []string{"PREVIEW_LOW_CONTRAST on slide 1"},
	}, pptxArtifactWorkerRequest{
		Slides: []officegen.Slide{
			{Title: "Cover", Layout: "title", IsTitle: true},
			{Title: "Chart", Layout: "chart", Chart: &officegen.ChartData{Type: "bar", Title: "Signal", Categories: []string{"A"}, Values: []float64{1}}},
		},
	}, 1)
	if summary.QualityGate != "fail" {
		t.Fatalf("quality gate = %q, want fail: %#v", summary.QualityGate, summary)
	}
	for _, expected := range []string{"native_chart", "preview_coverage", "visual_issues", "preview_issues"} {
		found := false
		for _, got := range summary.MissingRequirements {
			if got == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing requirements = %+v, want %s", summary.MissingRequirements, expected)
		}
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalSkipsPolishWhenPlannerHasNoPlan(t *testing.T) {
	original := runPPTXArtifactWorker
	var calls int
	var polishRequest pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		calls++
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if req.RepairMode == "polish" {
			polishRequest = req
		}
		return writePPTXArtifactFakeDiagnostics(t, req, 4, 1), nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "PPT Reference Style Test", "", false, false, PPTXBuildOptions{
		Backend:                    PPTXBackendArtifactWorker,
		GenerateArtifactDesignPlan: true,
		UserPrompt:                 "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if calls != 1 {
		t.Fatalf("worker calls = %d, want initial pass only when polish planner has no usable plan", calls)
	}
	if polishRequest.RepairMode != "" {
		t.Fatalf("unexpected polish request when planner has no usable plan: %#v", polishRequest)
	}
	if containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_POLISH") {
		t.Fatalf("warnings = %+v, did not expect polish pass warning", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalPolishUsesLLMDesignPatch(t *testing.T) {
	original := runPPTXArtifactWorker
	var calls int
	var polishRequest pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		calls++
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if req.RepairMode == "polish" {
			polishRequest = req
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 4, 1)
		output.VisualVerdict = "pass"
		output.VisualScore = 96
		return output, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	initialPlan := `{
		"deckIntent":"concise-reference-style-learning",
		"styleBias":"dark-structured",
		"builderRecipe":"codex-reference-learning",
		"slides":[
			{"slide":1,"role":"cover","layoutMode":"cover-split-visual","composition":"split-hero","visualTreatment":"native-shapes","densityTarget":"spacious","kicker":"STYLE-INFORMED SUMMARY","displayTitle":"PPT Reference Style Test","displaySubtitle":"Same prompt, reference style intent, and editable visual motifs.","takeaway":"","visualIntent":"Use a split hero.","cards":[],"chartCallouts":[]},
			{"slide":2,"role":"observations","layoutMode":"observation-cards","composition":"numbered-cards","visualTreatment":"native-shapes","densityTarget":"spacious","kicker":"KEY OBSERVATIONS","displayTitle":"What the reference directory actually teaches","displaySubtitle":"System, not template.","takeaway":"Use recurring visual choices as a system, not a literal template.","visualIntent":"Use three cards.","cards":[{"heading":"Repeatable style beats single-slide mimicry","detail":"Use repeated panels, accent rules, and compact cards instead of copying a deck."}],"chartCallouts":[]},
			{"slide":3,"role":"evidence","layoutMode":"chart-insight-stack","composition":"chart-with-side-insights","visualTreatment":"native-chart","densityTarget":"spacious","kicker":"SIMPLE CHART","displayTitle":"Fidelity comes from multiple enforced layers","displaySubtitle":"A styled evidence panel makes reference signals easy to scan.","takeaway":"","visualIntent":"Use a chart.","cards":[],"chartCallouts":[{"heading":"Why it matters","detail":"Content alone cannot define composition."},{"heading":"Quality loop","detail":"Rendered checks shape final layout."}]},
			{"slide":4,"role":"closing","layoutMode":"closing-takeaway","composition":"split-callout","visualTreatment":"native-shapes","densityTarget":"compact","kicker":"CLOSING TAKEAWAY","displayTitle":"Reference style becomes a reusable system","displayBody":"Carry palette, rhythm, and hierarchy into a concise deck while keeping the message clear.","takeaway":"","visualIntent":"Use split closing.","cards":[],"chartCallouts":[]}
		]
	}`
	patchedPlan := strings.Replace(initialPlan, "Fidelity comes from multiple enforced layers", "Style hierarchy drives the final composition", 1)
	patchedPlan = strings.Replace(patchedPlan, `"slides":[`, `"builderPatch":{"slides":[{"slide":3,"accentRail":"top","backplate":"right-band"}]},"slides":[`, 1)
	llm := &fakeLLMClient{structuredResponses: []string{initialPlan, patchedPlan, `{}`}}
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, nil, content, "PPT Reference Style Test", "", false, false, PPTXBuildOptions{
		Backend:                    PPTXBackendArtifactWorker,
		GenerateArtifactDesignPlan: true,
		UserPrompt:                 "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if calls != 2 {
		t.Fatalf("worker calls = %d, want initial plus one preview-informed polish pass", calls)
	}
	if llm.structuredCallCount != 3 {
		t.Fatalf("structured calls = %d, want initial plan, one preview-informed polish plan, and one skipped second polish attempt", llm.structuredCallCount)
	}
	if polishRequest.DesignPlan == nil {
		t.Fatal("polish request missing design plan")
	}
	if got := polishRequest.DesignPlan.Slides[2].DisplayTitle; got != "Style hierarchy drives the final composition" {
		t.Fatalf("polish chart title = %q, want patched LLM design plan", got)
	}
	if polishRequest.DesignPlan.BuilderPatch == nil || len(polishRequest.DesignPlan.BuilderPatch.Slides) != 1 {
		t.Fatalf("polish request missing dynamic builder patch: %#v", polishRequest.DesignPlan.BuilderPatch)
	}
	if got := polishRequest.DesignPlan.BuilderPatch.Slides[0]; got.Slide != 3 || got.AccentRail != "top" || got.Backplate != "right-band" {
		t.Fatalf("polish builder patch = %#v, want slide 3 top rail and right band", got)
	}
	if llm.lastStructuredReq.Schema.Name != "pptx_artifact_design_plan_polish" {
		t.Fatalf("last schema = %q, want polish schema", llm.lastStructuredReq.Schema.Name)
	}
	if !strings.Contains(llm.lastStructuredReq.Messages[0].Content, "Rendered preview/inspect evidence") {
		t.Fatalf("polish prompt did not include inspect evidence:\n%s", llm.lastStructuredReq.Messages[0].Content)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_POLISH_DESIGN_APPLIED") {
		t.Fatalf("warnings = %+v, want polish design applied warning", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalPolishUsesSecondPreviewInformedPass(t *testing.T) {
	original := runPPTXArtifactWorker
	var calls int
	var finalPolishRequest pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		calls++
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if req.RepairMode == "polish" {
			finalPolishRequest = req
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 4, 1)
		output.VisualVerdict = "pass"
		output.VisualScore = 96
		return output, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"PPT Reference Style Test",
		"slides":[
			{"title":"PPT Reference Style Test","layout":"title","subtitle":"Cover","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["Recurring panels","Editable text"]},
			{"title":"Simple Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["Style","Builder"],"values":[70,90]}},
			{"title":"Closing","layout":"closing","content":"Turn reference signals into editable slides."}
		]
	}`
	initialPlan := `{
		"deckIntent":"concise-reference-style-learning",
		"styleBias":"dark-structured",
		"builderRecipe":"codex-reference-learning",
		"builderPatch":{"slides":[]},
		"slides":[
			{"slide":1,"role":"cover","layoutMode":"cover-split-visual","composition":"split-hero","visualTreatment":"native-shapes","densityTarget":"spacious","kicker":"STYLE-INFORMED SUMMARY","displayTitle":"PPT Reference Style Test","displaySubtitle":"Same prompt, reference style intent, and editable visual motifs.","takeaway":"","visualIntent":"Use a split hero.","cards":[],"chartCallouts":[]},
			{"slide":2,"role":"observations","layoutMode":"observation-cards","composition":"numbered-cards","visualTreatment":"native-shapes","densityTarget":"spacious","kicker":"KEY OBSERVATIONS","displayTitle":"What the reference directory actually teaches","displaySubtitle":"System, not template.","takeaway":"Use recurring visual choices as a system, not a literal template.","visualIntent":"Use three cards.","cards":[{"heading":"Repeatable style beats single-slide mimicry","detail":"Use repeated panels, accent rules, and compact cards instead of copying a deck."}],"chartCallouts":[]},
			{"slide":3,"role":"evidence","layoutMode":"chart-insight-stack","composition":"chart-with-side-insights","visualTreatment":"native-chart","densityTarget":"spacious","kicker":"SIMPLE CHART","displayTitle":"Fidelity comes from multiple enforced layers","displaySubtitle":"A styled evidence panel makes reference signals easy to scan.","takeaway":"","visualIntent":"Use a chart.","cards":[],"chartCallouts":[{"heading":"Why it matters","detail":"Content alone cannot define composition."},{"heading":"Quality loop","detail":"Rendered checks shape final layout."}]},
			{"slide":4,"role":"closing","layoutMode":"closing-takeaway","composition":"split-callout","visualTreatment":"native-shapes","densityTarget":"compact","kicker":"CLOSING TAKEAWAY","displayTitle":"Reference style becomes a reusable system","displayBody":"Carry palette, rhythm, and hierarchy into a concise deck while keeping the message clear.","takeaway":"","visualIntent":"Use split closing.","cards":[],"chartCallouts":[]}
		]
	}`
	firstPolishPlan := strings.Replace(initialPlan, "Fidelity comes from multiple enforced layers", "Style hierarchy drives the final composition", 1)
	firstPolishPlan = strings.Replace(firstPolishPlan, `"builderPatch":{"slides":[]}`, `"builderPatch":{"slides":[{"slide":3,"accentRail":"top","backplate":"right-band"}]}`, 1)
	secondPolishPlan := strings.Replace(firstPolishPlan, "Style hierarchy drives the final composition", "Style hierarchy sharpens the final composition", 1)
	secondPolishPlan = strings.Replace(secondPolishPlan, `"accentRail":"top","backplate":"right-band"`, `"accentRail":"left","backplate":"bottom-band"`, 1)
	llm := &fakeLLMClient{structuredResponses: []string{initialPlan, firstPolishPlan, secondPolishPlan}}
	previewReviewer := &fakePPTXArtifactPreviewReviewer{result: &PPTXArtifactPreviewReviewResult{
		Score:     82,
		Summary:   "Rendered previews are readable, but the chart slide still feels too generic and the closing slide needs stronger hierarchy.",
		Strengths: []string{"Editable foreground text is visible", "The dark canvas and accent rails are consistent"},
		Issues: []PPTXArtifactPreviewReviewIssue{{
			Severity:     "medium",
			Code:         "CHART_CALLOUT_HIERARCHY_WEAK",
			Title:        "Weak chart callout hierarchy",
			Message:      "Slide 3 side callouts read as equal notes rather than a clear evidence stack.",
			SlideNumbers: []int{3},
			Suggestion:   "Make one chart callout a stronger thesis and reduce secondary callout density.",
		}},
	}}
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, nil, content, "PPT Reference Style Test", "", false, false, PPTXBuildOptions{
		Backend:                    PPTXBackendArtifactWorker,
		GenerateArtifactDesignPlan: true,
		UserPrompt:                 "Create a concise editable presentation that learns the style from PPTX files in this directory. Include a cover slide, key observations, one simple chart, and a closing slide.",
		ArtifactPreviewReviewer:    previewReviewer,
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if calls != 3 {
		t.Fatalf("worker calls = %d, want initial plus two preview-informed polish passes", calls)
	}
	if llm.structuredCallCount != 3 {
		t.Fatalf("structured calls = %d, want initial plan plus two preview-informed polish plans", llm.structuredCallCount)
	}
	if finalPolishRequest.DesignPlan == nil {
		t.Fatal("final polish request missing design plan")
	}
	if got := finalPolishRequest.DesignPlan.Slides[2].DisplayTitle; got != "Style hierarchy sharpens the final composition" {
		t.Fatalf("final polish chart title = %q, want second preview-informed patch", got)
	}
	if finalPolishRequest.DesignPlan.BuilderPatch == nil || len(finalPolishRequest.DesignPlan.BuilderPatch.Slides) != 1 {
		t.Fatalf("final polish request missing dynamic builder patch: %#v", finalPolishRequest.DesignPlan.BuilderPatch)
	}
	if got := finalPolishRequest.DesignPlan.BuilderPatch.Slides[0]; got.AccentRail != "left" || got.Backplate != "bottom-band" {
		t.Fatalf("final polish builder patch = %#v, want second pass patch", got)
	}
	if !strings.Contains(llm.lastStructuredReq.Messages[0].Content, "preview-polish") {
		t.Fatalf("second polish prompt should use first-pass preview evidence:\n%s", llm.lastStructuredReq.Messages[0].Content)
	}
	if !strings.Contains(llm.lastStructuredReq.Messages[0].Content, "previewSignals") {
		t.Fatalf("second polish prompt should include preview pixel signals:\n%s", llm.lastStructuredReq.Messages[0].Content)
	}
	if previewReviewer.calls < 2 {
		t.Fatalf("preview reviewer calls = %d, want review before each polish pass", previewReviewer.calls)
	}
	if len(previewReviewer.requests) == 0 || len(previewReviewer.requests[0].PreviewFiles) != 4 {
		t.Fatalf("preview reviewer requests = %#v, want rendered slide previews", previewReviewer.requests)
	}
	if !strings.Contains(llm.lastStructuredReq.Messages[0].Content, "visionPreviewReview") ||
		!strings.Contains(llm.lastStructuredReq.Messages[0].Content, "CHART_CALLOUT_HIERARCHY_WEAK") ||
		!strings.Contains(llm.lastStructuredReq.Messages[0].Content, "Make one chart callout") {
		t.Fatalf("second polish prompt should include screenshot visual review evidence:\n%s", llm.lastStructuredReq.Messages[0].Content)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_POLISH_DESIGN_APPLIED") {
		t.Fatalf("warnings = %+v, want polish design applied warning", warnings)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_PREVIEW_REVIEW_APPLIED") {
		t.Fatalf("warnings = %+v, want preview review applied warning", warnings)
	}
	if !containsStringContaining(generateIssueMessages(warnings), "2 preview-informed polish passes") {
		t.Fatalf("warnings = %+v, want two-pass polish warning", warnings)
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalEmitsDebugMetadata(t *testing.T) {
	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, workDir string) (*pptxArtifactWorkerOutput, error) {
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 3, 1)
		output.WorkerDir = workDir
		output.ScriptPath = filepath.Join(workDir, "pptx_artifact_worker.mjs")
		output.RequestPath = filepath.Join(workDir, "request.json")
		output.ResponsePath = filepath.Join(workDir, "response.json")
		output.WorkerVersion = "artifact-experimental-test"
		output.VisualVerdict = "pass"
		output.VisualScore = 96
		return output, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Debug Metadata",
		"slides":[
			{"title":"Artifact Debug Metadata","layout":"title","subtitle":"Debug metadata smoke","isTitle":true},
			{"title":"Evidence Chart","layout":"chart","chart":{"type":"bar","title":"Quality signal","categories":["A","B"],"values":[1,2]}}
		]
	}`
	var debugMeta PPTXArtifactDebugMetadata
	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Debug Metadata", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
		ArtifactDebugSink: func(meta PPTXArtifactDebugMetadata) {
			debugMeta = meta
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if !debugMeta.Enabled || debugMeta.Backend != PPTXBackendArtifactWorker {
		t.Fatalf("debug metadata identity = %#v", debugMeta)
	}
	if debugMeta.WorkerVersion != "artifact-experimental-test" || debugMeta.VisualVerdict != "pass" || debugMeta.VisualScore != 96 {
		t.Fatalf("debug metadata worker verdict = %#v", debugMeta)
	}
	if debugMeta.QualitySummary == nil {
		t.Fatalf("missing quality summary: %#v", debugMeta)
	}
	if debugMeta.QualitySummary.QualityGate != "pass" || !debugMeta.QualitySummary.EditableCoverageOK || !debugMeta.QualitySummary.NativeChartOK || !debugMeta.QualitySummary.PreviewCoverageOK || !debugMeta.QualitySummary.VisualVerdictOK {
		t.Fatalf("quality summary = %#v, want passing gate with coverage", debugMeta.QualitySummary)
	}
	if debugMeta.QualitySummary.ExpectedCharts != 1 || debugMeta.QualitySummary.NativeCharts != 1 || debugMeta.QualitySummary.PreviewCount != len(debugMeta.PreviewFiles) {
		t.Fatalf("quality summary counts = %#v", debugMeta.QualitySummary)
	}
	for label, value := range map[string]string{
		"worker_dir":         debugMeta.WorkerDir,
		"worker_script_path": debugMeta.WorkerScriptPath,
		"request_path":       debugMeta.RequestPath,
		"response_path":      debugMeta.ResponsePath,
		"inspect_path":       debugMeta.InspectPath,
		"final_output_pptx":  debugMeta.FinalOutputPPTX,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("%s missing in debug metadata: %#v", label, debugMeta)
		}
	}
	if debugMeta.PreviewCount == 0 || debugMeta.PreviewCount != len(debugMeta.PreviewFiles) {
		t.Fatalf("debug preview metadata = %#v", debugMeta)
	}
	if debugMeta.NativeCharts != 1 || debugMeta.EditableItems != 3 {
		t.Fatalf("debug structure counts = %#v", debugMeta)
	}
	for _, expected := range []string{"# Narrative Plan", "## Source Plan", "## Visual System", "## Imagegen Plan", "## Editability Plan"} {
		if !strings.Contains(debugMeta.NarrativePlanMarkdown, expected) {
			t.Fatalf("narrative plan markdown missing %q:\n%s", expected, debugMeta.NarrativePlanMarkdown)
		}
	}
}

func TestBuildPPTXFromJSON_PPTXArtifactExperimentalRetriesMinimalOnSecondLayoutFailure(t *testing.T) {
	original := runPPTXArtifactWorker
	var calls int
	var minimalRequest pptxArtifactWorkerRequest
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		calls++
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(req.PreviewDir, 0o755); err != nil {
			return nil, err
		}
		previewFiles := make([]string, 0, len(req.Slides))
		for idx := range req.Slides {
			previewPath := filepath.Join(req.PreviewDir, fmt.Sprintf("slide-%02d.png", idx+1))
			writePPTXArtifactPreviewFixture(t, previewPath, idx)
			previewFiles = append(previewFiles, previewPath)
		}
		editable := []pptxArtifactEditableInspectItem{
			{Role: "heading", Text: "First card", Slide: 2, FontSize: 18, TextChars: len("First card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 220, Height: 60}},
			{Role: "body", Text: "Second card", Slide: 2, FontSize: 16, TextChars: len("Second card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 120, Top: 138, Width: 220, Height: 60}},
		}
		if calls == 3 {
			minimalRequest = req
			editable = []pptxArtifactEditableInspectItem{
				{Role: "heading", Text: "Only card", Slide: 2, FontSize: 18, TextChars: len("Only card"), TextLines: 1, BBox: pptxArtifactInspectBBox{Left: 100, Top: 120, Width: 220, Height: 40}},
			}
		}
		inspect := pptxArtifactInspectSummary{
			EditableItems: editable,
			NativeCharts:  []any{},
			Previews:      previewFiles,
		}
		inspectBytes, err := json.Marshal(inspect)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.InspectPath, inspectBytes, 0o644); err != nil {
			return nil, err
		}
		return &pptxArtifactWorkerOutput{
			OutputPPTX:     req.OutputPPTX,
			PreviewFiles:   previewFiles,
			InspectPath:    req.InspectPath,
			EditableItems:  len(editable),
			ArtifactToolOK: true,
		}, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	content := `{
		"title":"Artifact Minimal Repair",
		"slides":[
			{"title":"Artifact Minimal Repair","layout":"title","subtitle":"Minimal repair smoke","isTitle":true},
			{"title":"Key Observations","layout":"content","points":["First card should remain readable","Second card should be removed on minimal repair","Third card should also be removed"],"sections":[
				{"heading":"One","detail":"First detail"},
				{"heading":"Two","detail":"Second detail"},
				{"heading":"Three","detail":"Third detail"}
			]}
		]
	}`
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Artifact Minimal Repair", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if calls != 3 {
		t.Fatalf("worker calls = %d, want 3", calls)
	}
	if minimalRequest.RepairMode != "minimal" {
		t.Fatalf("repair mode = %q, want minimal", minimalRequest.RepairMode)
	}
	if len(minimalRequest.Slides) < 2 || len(minimalRequest.Slides[1].Points) > 1 || len(minimalRequest.Slides[1].Sections) > 1 {
		t.Fatalf("minimal request was not minimized: %+v", minimalRequest.Slides)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_RETRY") {
		t.Fatalf("warnings = %+v, want WARN_PPTX_ARTIFACT_RETRY", warnings)
	}
}

func TestPPTXArtifactWorkerIntegrationOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	content := `{
		"title":"Reference Builder Integration",
			"slides":[
				{"title":"Reference Builder Integration","layout":"title","subtitle":"Two-slide smoke","isTitle":true},
				{"title":"Key Observations","layout":"content","subtitle":"The strongest reusable signals are hierarchy.","points":["Consistent visual system","Editable native structure","Rendered preview checks"],"sections":[
					{"heading":"Visual system","detail":"Repeatable cards and rhythm"},
					{"heading":"Editable structure","detail":"Important content stays editable"},
					{"heading":"Preview QA","detail":"Rendered checks shape the final design"}
				]},
				{"title":"Native Objects","layout":"chart","points":["Editable title","Editable body"],"chart":{"type":"bar","title":"Quarterly signal","categories":["Q1","Q2"],"values":[12,18]}},
				{"title":"Closing","layout":"closing","content":"Approve one focused validation cycle before scaling the approach.","points":["Keep final slides editable and preview-checked."]}
			]
	}`
	fileBytes, fileName, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Reference Builder Integration", "", false, false, PPTXBuildOptions{
		Backend:    PPTXBackendArtifactWorker,
		UserPrompt: "Create a concise editable presentation. Include a cover slide, key observations, one simple chart, and a closing slide.",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if len(fileBytes) == 0 || !strings.HasSuffix(fileName, ".pptx") {
		t.Fatalf("output = bytes %d, file %q", len(fileBytes), fileName)
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Reference Builder Integration") {
		t.Fatal("exported pptx does not contain expected editable text")
	}
	for _, expected := range []string{"Style profile", "Card rhythm", "Quality loop"} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", expected) {
			t.Fatalf("artifact worker cover should expose polished reference-signal chip %q", expected)
		}
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Reference rhythm") {
		t.Fatal("artifact worker should add an editable fallback visual motif when no local images are used")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Quality loop") {
		t.Fatalf("artifact worker should render observations as a designed three-card layout even when sections are present; warnings=%+v", warnings)
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Style signal") {
		t.Fatal("artifact worker chart slide should include reference-learning insight scaffolding")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Quality loop") {
		t.Fatal("artifact worker chart slide should include a quality-loop callout")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "SIMPLE CHART") {
		t.Fatal("artifact worker should render the chart kicker from the artifact design plan")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Palette, spacing, hierarchy align.") {
		t.Fatal("artifact worker should use polished audience-facing chart insight text")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Rendered checks shape final layout.") {
		t.Fatal("artifact worker should use polished chart focus text")
	}
	for _, expected := range []string{"Intent", "Execution"} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", expected) {
			t.Fatalf("artifact worker closing slide should expose polished closing support section %q", expected)
		}
	}
	for _, forbidden := range []string{"Hard gate", "native chart", "Preview gate.", "Editable evidence.", "Editable objects", "Rendered preview checks", "Editable title", "preview-checked"} {
		if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", forbidden) {
			t.Fatalf("artifact worker chart slide should not expose implementation wording %q", forbidden)
		}
	}
}

func TestPPTXArtifactWorkerIntegrationReferenceLearningObservationCardsUseReadableSpaceOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	request := pptxArtifactWorkerRequest{
		Title:       "Reference Style Card Geometry",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Style Card Geometry", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{
				Title:    "Reference Style Signals",
				Layout:   "content",
				Subtitle: "The useful signal is a visual system, not a literal template.",
				Sections: []officegen.SlideSection{
					{Heading: "Repeatable style", Detail: "Use recurring panels, accent rules, large headings, and compact cards."},
					{Heading: "Structured content", Detail: "Keep words, labels, metrics, and callouts as selectable objects."},
					{Heading: "Quality loop", Detail: "Let rendered previews catch overflow, weak contrast, and chart defaults."},
				},
			},
			{Title: "Fidelity Comes From Enforced Layers", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Reference Style Needs a Builder Loop", Layout: "closing", Content: "Turn signals into structured slides."},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent: "concise-reference-style-learning",
			StyleBias:  "dark-structured",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual"},
				{Slide: 2, Role: "observations", LayoutMode: "observation-cards", Kicker: "KEY OBSERVATIONS"},
				{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", Kicker: "SIMPLE CHART"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", Kicker: "CLOSING TAKEAWAY"},
			},
		},
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  filepath.Join(workDir, "preview"),
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	output, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir)
	if err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	if output.EditableItems == 0 {
		t.Fatal("expected editable inspect items")
	}
	inspectBytes, err := os.ReadFile(request.InspectPath)
	if err != nil {
		t.Fatalf("read inspect: %v", err)
	}
	var inspect pptxArtifactInspectSummary
	if err := json.Unmarshal(inspectBytes, &inspect); err != nil {
		t.Fatalf("parse inspect: %v", err)
	}
	var detailHeights []float64
	for _, item := range inspect.EditableItems {
		if item.Slide == 2 && item.Role == "observation-detail" {
			detailHeights = append(detailHeights, item.BBox.Height)
		}
	}
	if len(detailHeights) != 3 {
		t.Fatalf("observation detail boxes = %d, want 3; items=%+v", len(detailHeights), inspect.EditableItems)
	}
	for _, height := range detailHeights {
		if height < 80 {
			t.Fatalf("observation detail box height = %.1f, want at least 80 for Codex-like readable cards", height)
		}
	}
}

func TestPPTXArtifactWorkerIntegrationReferenceLearningObservationUsesTakeawayBannerOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	request := pptxArtifactWorkerRequest{
		Title:       "Reference Learning Observation",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Learning Observation", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Key Observations", Layout: "content", Points: []string{"Recurring panels", "Editable text"}},
			{Title: "Simple Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "dark-structured",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", Composition: "split-hero"},
				{Slide: 2, Role: "observations", LayoutMode: "observation-cards", Composition: "numbered-cards", Kicker: "KEY OBSERVATIONS", Takeaway: "Visual iteration turns style signals into a usable deck."},
				{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", Composition: "chart-with-side-insights", Kicker: "SIMPLE CHART"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", Composition: "split-callout", Kicker: "CLOSING TAKEAWAY"},
			},
		},
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  filepath.Join(workDir, "preview"),
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	if _, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir); err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	inspectBytes, err := os.ReadFile(request.InspectPath)
	if err != nil {
		t.Fatalf("read inspect: %v", err)
	}
	var inspect pptxArtifactInspectSummary
	if err := json.Unmarshal(inspectBytes, &inspect); err != nil {
		t.Fatalf("parse inspect: %v", err)
	}
	found := false
	for _, item := range inspect.EditableItems {
		if item.Slide == 2 && item.Role == "observation-takeaway" {
			found = true
			if item.BBox.Height < 28 {
				t.Fatalf("observation takeaway height = %.1f, want readable banner text", item.BBox.Height)
			}
			break
		}
	}
	if !found {
		t.Fatalf("reference-learning observation slide should include an editable takeaway banner; items=%+v", inspect.EditableItems)
	}
}

func TestPPTXArtifactWorkerIntegrationUsesPlannedCardsAndChartCalloutsOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	request := pptxArtifactWorkerRequest{
		Title:       "Planned Builder Copy",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Planned Builder Copy", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{
				Title:    "Reference Style Signals",
				Layout:   "content",
				Subtitle: "The useful signal is a visual system, not a literal template.",
				Sections: []officegen.SlideSection{
					{Heading: "Fallback section", Detail: "This fallback section should not become the first planned card."},
				},
			},
			{Title: "Fidelity Comes From Enforced Layers", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Reference Style Needs a Builder Loop", Layout: "closing", Content: "Turn signals into structured slides."},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent: "concise-reference-style-learning",
			StyleBias:  "dark-structured",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual"},
				{
					Slide:      2,
					Role:       "observations",
					LayoutMode: "observation-cards",
					Kicker:     "KEY OBSERVATIONS",
					Cards: []pptxArtifactPlanCard{
						{Heading: "Single-slide mimicry loses", Detail: "Use repeated panels, accent rules, and compact cards instead of copying a deck."},
						{Heading: "Content stays selectable", Detail: "Keep labels, metrics, and callouts selectable on the slide."},
						{Heading: "Build for reuse", Detail: "Prevents overload."},
					},
				},
				{
					Slide:      3,
					Role:       "evidence",
					LayoutMode: "chart-insight-stack",
					Kicker:     "SIMPLE CHART",
					ChartCallouts: []pptxArtifactPlanCard{
						{Heading: "Builder fit", Detail: "Compose around the task instead of only replaying semantic JSON."},
						{Heading: "Review loop", Detail: "Use preview evidence to decide what gets simplified."},
						{Heading: "Overflow callout", Detail: "This third planned chart callout should stay out of the rendered slide."},
					},
				},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", Kicker: "CLOSING TAKEAWAY"},
			},
		},
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  filepath.Join(workDir, "preview"),
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	output, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir)
	if err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	fileBytes, err := os.ReadFile(output.OutputPPTX)
	if err != nil {
		t.Fatalf("read output pptx: %v", err)
	}
	for _, expected := range []string{
		"Single-slide mimicry loses",
		"Content stays selectable",
		"Visual QA changes the final design",
		"Why it matters",
		"Quality loop",
		"A styled evidence panel makes reference signals easy to scan.",
		"Custom composition plus preview iteration carries the fidelity signal.",
	} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", expected) {
			t.Fatalf("reference-learning worker copy %q was not exported as editable slide text", expected)
		}
	}
	inspectBytes, err := os.ReadFile(request.InspectPath)
	if err != nil {
		t.Fatalf("read inspect: %v", err)
	}
	var inspect pptxArtifactInspectSummary
	if err := json.Unmarshal(inspectBytes, &inspect); err != nil {
		t.Fatalf("parse inspect: %v", err)
	}
	editableText := make(map[string]bool)
	for _, item := range inspect.EditableItems {
		editableText[item.Text] = true
	}
	for _, expected := range []string{
		"Use repeated panels, accent rules, and compact cards instead of copying a deck.",
		"Keep labels, metrics, and callouts selectable on the slide.",
		"Previews catch overflow, contrast issues, blank pages, and chart defaults.",
	} {
		if !editableText[expected] {
			t.Fatalf("reference-learning worker detail %q was not recorded as editable slide text; items=%+v", expected, inspect.EditableItems)
		}
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Fallback section") {
		t.Fatal("reference-learning observation cards should outrank fallback semantic sections")
	}
	for _, rejected := range []string{
		"Repeatable style beats single-slide mimicry",
		"Important content stays editable",
		"Builder fit",
		"Compose around the task instead",
	} {
		if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", rejected) {
			t.Fatalf("reference-learning worker should prefer stable narrative over planned copy %q", rejected)
		}
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Overflow callout") {
		t.Fatal("planned chart callouts should be capped at two to keep the chart slide readable")
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "3. The goal is") {
		t.Fatal("planned observation headings should not be replaced by dangling detail-derived headings")
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Prevents overload.") {
		t.Fatal("planned observation cards should replace low-information detail copy")
	}
	for _, dangling := range []string{"instead of copying.", "instead of copying</a:t>", "compact cards instead.", "compact cards instead</a:t>"} {
		if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", dangling) {
			t.Fatalf("planned observation detail should not export dangling copy fragment %q", dangling)
		}
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "1. Repeatable style beats single-slide mimicry") {
		t.Fatal("codex-reference-learning recipe should render card numbers as separate editable labels, not as heading prefixes")
	}
	for _, expectedIndex := range []string{"01", "02", "03"} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", expectedIndex) {
			t.Fatalf("codex-reference-learning recipe should render separate card index %q", expectedIndex)
		}
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Visual QA changes the final design") {
		t.Fatal("planned observation cards should still fill weak planned slots with stable reference-learning fallback")
	}
}

func TestPPTXArtifactWorkerIntegrationReferenceLearningChartRecordsDesignedPanelOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	request := pptxArtifactWorkerRequest{
		Title:       "Reference Learning Chart Panel",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Learning Chart Panel", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Key Observations", Layout: "content", Points: []string{"Recurring panels", "Editable text"}},
			{Title: "Fidelity Comes From Enforced Layers", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "dark-structured",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", Composition: "split-hero"},
				{Slide: 2, Role: "observations", LayoutMode: "observation-cards", Composition: "numbered-cards", Kicker: "KEY OBSERVATIONS"},
				{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", Composition: "chart-with-side-insights", Kicker: "SIMPLE CHART"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", Composition: "split-callout", Kicker: "CLOSING TAKEAWAY"},
			},
		},
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  filepath.Join(workDir, "preview"),
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	if _, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir); err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	inspectBytes, err := os.ReadFile(request.InspectPath)
	if err != nil {
		t.Fatalf("read inspect: %v", err)
	}
	var inspect pptxArtifactInspectSummary
	if err := json.Unmarshal(inspectBytes, &inspect); err != nil {
		t.Fatalf("parse inspect: %v", err)
	}
	roleCounts := map[string]int{}
	for _, item := range inspect.VisualItems {
		if item.Slide == 3 {
			roleCounts[item.Role]++
		}
	}
	for _, role := range []string{"chart-panel"} {
		if roleCounts[role] == 0 {
			t.Fatalf("chart slide visual role %q missing; roles=%v", role, roleCounts)
		}
	}
	if roleCounts["chart-insight-card"] < 2 {
		t.Fatalf("chart slide should record at least two designed insight cards; roles=%v", roleCounts)
	}
}

func TestPPTXArtifactWorkerIntegrationReferenceLearningPolishUsesRoomyChartCalloutsOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	request := pptxArtifactWorkerRequest{
		Title:       "Reference Learning Polish",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Learning Polish", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Key Observations", Layout: "content", Points: []string{"Recurring panels", "Editable text"}},
			{Title: "Fidelity Comes From Enforced Layers", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "dark-structured",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", Composition: "split-hero"},
				{Slide: 2, Role: "observations", LayoutMode: "observation-cards", Composition: "numbered-cards", Kicker: "KEY OBSERVATIONS"},
				{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", Composition: "chart-with-side-insights", Kicker: "SIMPLE CHART"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", Composition: "split-callout", Kicker: "CLOSING TAKEAWAY"},
			},
		},
		RepairMode:  "polish",
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  filepath.Join(workDir, "preview"),
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	if _, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir); err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	inspectBytes, err := os.ReadFile(request.InspectPath)
	if err != nil {
		t.Fatalf("read inspect: %v", err)
	}
	var inspect pptxArtifactInspectSummary
	if err := json.Unmarshal(inspectBytes, &inspect); err != nil {
		t.Fatalf("parse inspect: %v", err)
	}
	var bodyHeights []float64
	for _, item := range inspect.EditableItems {
		if item.Slide == 3 && item.Role == "chart-insight-body" {
			bodyHeights = append(bodyHeights, item.BBox.Height)
		}
	}
	if len(bodyHeights) < 2 {
		t.Fatalf("chart insight bodies = %d, want at least 2; items=%+v", len(bodyHeights), inspect.EditableItems)
	}
	for _, height := range bodyHeights[:2] {
		if height < 48 {
			t.Fatalf("polish chart insight body height = %.1f, want roomy Codex-like callout body", height)
		}
	}
}

func TestPPTXArtifactWorkerIntegrationReferenceLearningRepairModesAvoidNarrowLongChartCopyOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	for _, repairMode := range []string{"", "design-repair", "simplified", "minimal"} {
		t.Run(firstNonEmpty(repairMode, "initial"), func(t *testing.T) {
			workDir := t.TempDir()
			request := pptxArtifactWorkerRequest{
				Title:       "Reference Learning Chart Repair",
				StylePreset: officegen.StylePresetExecutiveDark,
				Slides: []officegen.Slide{
					{Title: "Reference Learning Chart Repair", Layout: "title", Subtitle: "Cover", IsTitle: true},
					{Title: "Key Observations", Layout: "content", Points: []string{"Recurring panels", "Editable text"}},
					{Title: "Fidelity Comes From Enforced Layers", Layout: "chart", Chart: referenceSignalChart(nil)},
					{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
				},
				DesignPlan: &pptxArtifactDesignPlan{
					DeckIntent:    "concise-reference-style-learning",
					StyleBias:     "dark-structured",
					BuilderRecipe: "codex-reference-learning",
					Slides: []pptxArtifactSlideDesignPlan{
						{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", Composition: "split-hero"},
						{Slide: 2, Role: "observations", LayoutMode: "observation-cards", Composition: "numbered-cards", Kicker: "KEY OBSERVATIONS", DisplaySubtitle: "The reference signals point to a coherent presentation system."},
						{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", Composition: "chart-with-side-insights", Kicker: "SIMPLE CHART"},
						{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", Composition: "split-callout", Kicker: "CLOSING TAKEAWAY"},
					},
				},
				StrictVisualQuality: true,
				RepairMode:          repairMode,
				OutputPPTX:          filepath.Join(workDir, "output.pptx"),
				PreviewDir:          filepath.Join(workDir, "preview"),
				InspectPath:         filepath.Join(workDir, "inspect.json"),
			}
			if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
				t.Fatalf("mkdir preview: %v", err)
			}
			if _, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir); err != nil {
				t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
			}
			inspectBytes, err := os.ReadFile(request.InspectPath)
			if err != nil {
				t.Fatalf("read inspect: %v", err)
			}
			var inspect pptxArtifactInspectSummary
			if err := json.Unmarshal(inspectBytes, &inspect); err != nil {
				t.Fatalf("parse inspect: %v", err)
			}
			if inspect.VisualVerdict == nil {
				t.Fatal("visual verdict missing")
			}
			if inspect.VisualVerdict.Status != "pass" {
				t.Fatalf("visual verdict = %s score=%d issues=%+v", inspect.VisualVerdict.Status, inspect.VisualVerdict.Score, inspect.VisualVerdict.Issues)
			}
			for _, item := range inspect.EditableItems {
				if item.Slide != 3 || item.Role != "chart-insight-body" {
					continue
				}
				if utf8.RuneCountInString(item.Text) > 44 && item.BBox.Width < 260 {
					t.Fatalf("chart insight body remains too narrow: text=%q width=%.1f", item.Text, item.BBox.Width)
				}
			}
		})
	}
}

func TestPPTXArtifactWorkerIntegrationReferenceLearningPolishUsesStableObservationNarrativeOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	request := pptxArtifactWorkerRequest{
		Title:       "Reference Learning Polish Narrative",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Learning Polish Narrative", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Key Observations", Layout: "content", Points: []string{"Recurring panels", "Editable text"}},
			{Title: "Simple Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "dark-structured",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", Composition: "split-hero"},
				{
					Slide:       2,
					Role:        "observations",
					LayoutMode:  "observation-cards",
					Composition: "numbered-cards",
					Kicker:      "KEY OBSERVATIONS",
					Cards: []pptxArtifactPlanCard{
						{Heading: "Weak generic card", Detail: "Short copy and clear grouping help the style read cleanly on first glance."},
						{Heading: "Another generic card", Detail: "Concise content improves fidelity when slides keep a calm rhythm."},
						{Heading: "Readable but generic", Detail: "Simple evidence cards keep the deck organized and polished."},
					},
				},
				{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", Composition: "chart-with-side-insights", Kicker: "SIMPLE CHART"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", Composition: "split-callout", Kicker: "CLOSING TAKEAWAY"},
			},
		},
		RepairMode:  "polish",
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  filepath.Join(workDir, "preview"),
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	if _, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir); err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	fileBytes, err := os.ReadFile(request.OutputPPTX)
	if err != nil {
		t.Fatalf("read output pptx: %v", err)
	}
	for _, expected := range []string{
		"Repeatable style beats single-slide mimicry",
		"Important content stays editable",
		"Visual QA changes the final design",
	} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", expected) {
			t.Fatalf("polish observation narrative %q was not exported as editable text", expected)
		}
	}
	for _, rejected := range []string{"Weak generic card", "Another generic card", "Readable but generic"} {
		if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", rejected) {
			t.Fatalf("polish observation narrative should replace generic planned card %q", rejected)
		}
	}
}

func TestPPTXArtifactWorkerIntegrationReferenceLearningIgnoresDynamicBuilderPatchOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	request := pptxArtifactWorkerRequest{
		Title:       "Dynamic Builder Patch",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Dynamic Builder Patch", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Key Observations", Layout: "content", Points: []string{"Recurring panels", "Editable text"}},
			{Title: "Simple Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "dark-structured",
			BuilderRecipe: "codex-reference-learning",
			BuilderPatch: &pptxArtifactBuilderPatch{
				Slides: []pptxArtifactBuilderSlidePatch{
					{Slide: 3, AccentRail: "top", Backplate: "right-band"},
				},
			},
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", Composition: "split-hero"},
				{Slide: 2, Role: "observations", LayoutMode: "observation-cards", Composition: "numbered-cards", Kicker: "KEY OBSERVATIONS"},
				{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", Composition: "chart-with-side-insights", Kicker: "SIMPLE CHART"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", Composition: "split-callout", Kicker: "CLOSING TAKEAWAY"},
			},
		},
		RepairMode:  "polish",
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  filepath.Join(workDir, "preview"),
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	if _, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir); err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	inspectBytes, err := os.ReadFile(request.InspectPath)
	if err != nil {
		t.Fatalf("read inspect: %v", err)
	}
	var inspect pptxArtifactInspectSummary
	if err := json.Unmarshal(inspectBytes, &inspect); err != nil {
		t.Fatalf("parse inspect: %v", err)
	}
	roleCounts := map[string]int{}
	for _, item := range inspect.VisualItems {
		if item.Slide == 3 {
			roleCounts[item.Role]++
		}
	}
	if roleCounts["dynamic-builder-accent-rail"] != 0 {
		t.Fatalf("reference-learning slide should ignore dynamic builder accent rail; roles=%v", roleCounts)
	}
	if roleCounts["dynamic-builder-backplate"] != 0 {
		t.Fatalf("reference-learning slide should ignore dynamic builder backplate; roles=%v", roleCounts)
	}
}

func TestPPTXArtifactWorkerIntegrationReferenceLearningClosingUsesSplitCalloutWithoutAssetOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	request := pptxArtifactWorkerRequest{
		Title:       "Reference Learning Closing",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Learning Closing", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Key Observations", Layout: "content", Points: []string{"Recurring panels", "Editable text"}},
			{Title: "Simple Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "dark-structured",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", Composition: "split-hero"},
				{Slide: 2, Role: "observations", LayoutMode: "observation-cards", Composition: "numbered-cards", Kicker: "KEY OBSERVATIONS"},
				{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", Composition: "chart-with-side-insights", Kicker: "SIMPLE CHART"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", Composition: "split-callout", Kicker: "CLOSING TAKEAWAY"},
			},
		},
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  filepath.Join(workDir, "preview"),
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	if _, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir); err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	inspectBytes, err := os.ReadFile(request.InspectPath)
	if err != nil {
		t.Fatalf("read inspect: %v", err)
	}
	var inspect pptxArtifactInspectSummary
	if err := json.Unmarshal(inspectBytes, &inspect); err != nil {
		t.Fatalf("parse inspect: %v", err)
	}
	roles := map[string]bool{}
	for _, item := range inspect.EditableItems {
		if item.Slide == 4 {
			roles[item.Role] = true
		}
	}
	for _, role := range []string{"closing-eyebrow", "closing-title", "closing-body", "closing-visual-title", "closing-visual-body"} {
		if !roles[role] {
			t.Fatalf("closing slide editable role %q missing; roles=%v", role, roles)
		}
	}
	fileBytes, err := os.ReadFile(request.OutputPPTX)
	if err != nil {
		t.Fatalf("read output pptx: %v", err)
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "closing-panel") {
		t.Fatal("reference-learning split-callout closing should not use the fallback full-width closing panel")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Keep it readable") {
		t.Fatal("split-callout closing should keep the right-side reference-style callout title editable")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Prefer hierarchy over template mimicry.") {
		t.Fatal("split-callout closing should explain the reference-style readability focus")
	}
}

func TestPPTXArtifactWorkerIntegrationReferenceLearningClosingReplacesWeakReusableSystemTitleOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	request := pptxArtifactWorkerRequest{
		Title:       "Reference Learning Closing",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Reference Learning Closing", Layout: "title", Subtitle: "Cover", IsTitle: true},
			{Title: "Key Observations", Layout: "content", Points: []string{"Recurring panels", "Editable text"}},
			{Title: "Simple Chart", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Closing", Layout: "closing", Content: "Turn reference signals into editable slides."},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "dark-structured",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", Composition: "split-hero"},
				{Slide: 2, Role: "observations", LayoutMode: "observation-cards", Composition: "numbered-cards", Kicker: "KEY OBSERVATIONS"},
				{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", Composition: "chart-with-side-insights", Kicker: "SIMPLE CHART"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", Composition: "split-callout", Kicker: "CLOSING", DisplayTitle: "The gap is the builder loop", DisplayBody: "Carry palette, hierarchy, and spacing into one clear deck system."},
			},
		},
		RepairMode:  "polish",
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  filepath.Join(workDir, "preview"),
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	if _, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir); err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	fileBytes, err := os.ReadFile(request.OutputPPTX)
	if err != nil {
		t.Fatalf("read output pptx: %v", err)
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Reference style becomes a reusable system.") {
		t.Fatal("closing slide should export the audience-facing fallback title as editable text")
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "The gap is the builder loop") {
		t.Fatal("closing slide should replace implementation-narrative title")
	}
}

func TestPPTXArtifactWorkerIntegrationUsesPlannedVisibleCopyOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	request := pptxArtifactWorkerRequest{
		Title:       "Reference Style Learning Summary",
		StylePreset: officegen.StylePresetExecutiveDark,
		Slides: []officegen.Slide{
			{Title: "Raw Cover Title", Layout: "title", Subtitle: "Raw cover subtitle", IsTitle: true},
			{Title: "Raw Observation Title", Layout: "content", Points: []string{"Recurring panels", "Editable text"}},
			{Title: "Raw Chart Title", Layout: "chart", Chart: referenceSignalChart(nil)},
			{Title: "Raw Closing Title", Layout: "closing", Content: "Raw closing body."},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "dark-structured",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{
					Slide:           1,
					Role:            "cover",
					LayoutMode:      "cover-split-visual",
					Composition:     "split-hero",
					DisplayTitle:    "PPTX Style Learning Summary",
					DisplaySubtitle: "Same prompt, reference style intent, and editable visual motifs.",
				},
				{
					Slide:           2,
					Role:            "observations",
					LayoutMode:      "observation-cards",
					Composition:     "numbered-cards",
					Kicker:          "KEY OBSERVATIONS",
					DisplayTitle:    "What the reference directory teaches",
					DisplaySubtitle: "System, not template.",
				},
				{
					Slide:           3,
					Role:            "evidence",
					LayoutMode:      "chart-insight-stack",
					Composition:     "chart-with-side-insights",
					Kicker:          "SIMPLE CHART",
					DisplayTitle:    "Fidelity comes from enforced layers",
					DisplaySubtitle: "The chart stays native and editable.",
				},
				{
					Slide:        4,
					Role:         "closing",
					LayoutMode:   "closing-takeaway",
					Composition:  "split-callout",
					Kicker:       "CLOSING TAKEAWAY",
					DisplayTitle: "The gap is the builder loop",
					DisplayBody:  "Carry palette, rhythm, and hierarchy into a concise deck while keeping the message clear.",
				},
			},
		},
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  filepath.Join(workDir, "preview"),
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	if err := os.MkdirAll(request.PreviewDir, 0o755); err != nil {
		t.Fatalf("mkdir preview: %v", err)
	}
	if _, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir); err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	fileBytes, err := os.ReadFile(request.OutputPPTX)
	if err != nil {
		t.Fatalf("read output pptx: %v", err)
	}
	for _, expected := range []string{
		"PPTX Style Learning Summary",
		"Same prompt, reference style intent, and editable visual motifs",
		"Editable text",
		"Native chart",
		"Previewed",
		"What the reference directory teaches",
		"Fidelity comes from enforced layers",
		"Reference style becomes a reusable system",
		"Carry palette, rhythm, and hierarchy",
	} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", expected) {
			t.Fatalf("planned visible copy %q was not exported as editable text", expected)
		}
	}
	for _, rejected := range []string{"Raw Observation Title", "Raw Chart Title", "Raw Closing Title", "Raw closing body", "The gap is the builder loop"} {
		if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", rejected) {
			t.Fatalf("worker should prefer planned visible copy over semantic placeholder %q", rejected)
		}
	}
}

func TestPPTXArtifactWorkerIntegrationEmbedsLocalVisualAssetAndNativeChartOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	root := t.TempDir()
	imageBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference-visual.png"), imageBytes, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	content := `{
		"title":"Reference Visual Integration",
		"slides":[
			{"title":"Reference Visual Integration","layout":"title","subtitle":"Visual plate smoke","isTitle":true},
			{"title":"Evidence Chart","layout":"chart","points":["Editable text","Native chart"],"chart":{"type":"bar","title":"Quality signal","categories":["Visual","Chart"],"values":[70,90]}}
		]
	}`
	fileBytes, fileName, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Reference Visual Integration", "", true, false, PPTXBuildOptions{
		Backend:           PPTXBackendArtifactWorker,
		ReferenceScanRoot: root,
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if len(fileBytes) == 0 || !strings.HasSuffix(fileName, ".pptx") {
		t.Fatalf("output = bytes %d, file %q", len(fileBytes), fileName)
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ""); got < 1 {
		t.Fatalf("media count = %d, want at least 1", got)
	}
	if got := countZipEntries(fileBytes, "ppt/slides/charts/", ".xml"); got < 1 {
		t.Fatalf("chart xml count = %d, want at least 1", got)
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "points | peak") {
		t.Fatal("artifact worker chart slide should include an editable chart summary strip")
	}
}

func TestPPTXArtifactWorkerIntegrationTracksBoundVisualAssetSlidesOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	previewDir := filepath.Join(workDir, "preview")
	coverAsset := filepath.Join(workDir, "slide-01-cover.png")
	observationAsset := filepath.Join(workDir, "slide-02-observations.png")
	closingAsset := filepath.Join(workDir, "slide-04-closing.png")
	writeCheckerPNGFixture(t, coverAsset, color.RGBA{R: 8, G: 16, B: 32, A: 255}, color.RGBA{R: 56, G: 217, B: 255, A: 255})
	writeCheckerPNGFixture(t, observationAsset, color.RGBA{R: 10, G: 22, B: 42, A: 255}, color.RGBA{R: 96, G: 165, B: 250, A: 255})
	writeCheckerPNGFixture(t, closingAsset, color.RGBA{R: 14, G: 20, B: 34, A: 255}, color.RGBA{R: 245, G: 158, B: 11, A: 255})
	request := pptxArtifactWorkerRequest{
		Title:       "Bound Visual Asset Slides",
		StylePreset: "executive-dark",
		Slides: []officegen.Slide{
			{Title: "Bound Visual Asset Slides", Layout: "title", Subtitle: "Cover visual plate", IsTitle: true},
			{Title: "Key Observations", Layout: "content", Points: []string{"Style cues", "Editable text", "Preview checks"}},
			{Title: "Simple Chart", Layout: "chart", Chart: &officegen.ChartData{Type: "bar", Title: "Quality signal", Categories: []string{"Visual", "Chart"}, Values: []float64{70, 90}}},
			{Title: "Closing", Layout: "closing", Content: "Use the style system as intent."},
		},
		VisualAssets: []pptxArtifactVisualAsset{
			{Path: coverAsset, Name: filepath.Base(coverAsset), MIME: "image/png", Slide: 1, Frame: &pptxArtifactAssetFrame{Left: 780, Top: 118, Width: 320, Height: 250}, TargetAspectRatio: 320.0 / 250.0, SourceAspectRatio: 320.0 / 250.0, TextDetection: &pptxArtifactTextCheck{Checked: true, Status: "passed", Attempts: 1}, Width: 128, Height: 100},
			{Path: observationAsset, Name: filepath.Base(observationAsset), MIME: "image/png", Slide: 2, Frame: &pptxArtifactAssetFrame{Left: 936, Top: 90, Width: 172, Height: 92}, TargetAspectRatio: 172.0 / 92.0, SourceAspectRatio: 172.0 / 92.0, TextDetection: &pptxArtifactTextCheck{Checked: true, Status: "passed", Attempts: 1}, Width: 172, Height: 92},
			{Path: closingAsset, Name: filepath.Base(closingAsset), MIME: "image/png", Slide: 4, Frame: &pptxArtifactAssetFrame{Left: 784, Top: 120, Width: 326, Height: 226}, TargetAspectRatio: 326.0 / 226.0, SourceAspectRatio: 326.0 / 226.0, TextDetection: &pptxArtifactTextCheck{Checked: true, Status: "passed", Attempts: 1}, Width: 144, Height: 100},
		},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "dark-structured",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", DisplayTitle: "Bound Visual Asset Slides"},
				{Slide: 2, Role: "observations", LayoutMode: "observation-cards", DisplayTitle: "What the reference directory actually teaches"},
				{Slide: 3, Role: "evidence", LayoutMode: "chart-insight-stack", DisplayTitle: "Fidelity comes from multiple enforced layers"},
				{Slide: 4, Role: "closing", LayoutMode: "closing-takeaway", DisplayTitle: "Reference style becomes a reusable system", DisplayBody: "Carry palette, hierarchy, and spacing into one clear deck system."},
			},
		},
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  previewDir,
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	output, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir)
	if err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	if output.VisualVerdict != "pass" {
		t.Fatalf("visual verdict = %q", output.VisualVerdict)
	}
	var inspect struct {
		VisualAssets []struct {
			Path          string                  `json:"path"`
			Slide         int                     `json:"slide"`
			Frame         *pptxArtifactAssetFrame `json:"frame"`
			TextDetection *pptxArtifactTextCheck  `json:"textDetection"`
		} `json:"visualAssets"`
		Images []struct {
			Path  string                  `json:"path"`
			Slide int                     `json:"slide"`
			BBox  *pptxArtifactAssetFrame `json:"bbox"`
		} `json:"images"`
	}
	data, err := os.ReadFile(request.InspectPath)
	if err != nil {
		t.Fatalf("read inspect: %v", err)
	}
	if err := json.Unmarshal(data, &inspect); err != nil {
		t.Fatalf("unmarshal inspect: %v", err)
	}
	seen := map[string]int{}
	for _, image := range inspect.Images {
		seen[image.Path] = image.Slide
	}
	if seen[coverAsset] != 1 || seen[observationAsset] != 2 || seen[closingAsset] != 4 {
		t.Fatalf("bound image slides = %#v, want cover=1 observation=2 closing=4", seen)
	}
	assetFrames := map[string]*pptxArtifactAssetFrame{}
	assetTextDetections := map[string]*pptxArtifactTextCheck{}
	for _, asset := range inspect.VisualAssets {
		assetFrames[asset.Path] = asset.Frame
		assetTextDetections[asset.Path] = asset.TextDetection
	}
	for _, image := range inspect.Images {
		frame := assetFrames[image.Path]
		if frame == nil || image.BBox == nil {
			t.Fatalf("missing frame/bbox for %q: frame=%#v bbox=%#v", image.Path, frame, image.BBox)
		}
		if frame.Left != image.BBox.Left || frame.Top != image.BBox.Top || frame.Width != image.BBox.Width || frame.Height != image.BBox.Height {
			t.Fatalf("frame/bbox mismatch for %q: frame=%#v bbox=%#v", image.Path, frame, image.BBox)
		}
		textDetection := assetTextDetections[image.Path]
		if textDetection == nil || textDetection.Status != "passed" || textDetection.Attempts != 1 {
			t.Fatalf("text detection not preserved for %q: %#v", image.Path, textDetection)
		}
	}
}

func TestPPTXArtifactWorkerIntegrationEmitsQualitySummaryOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	content := `{
		"title":"Quality Summary Smoke",
		"stylePreset":"executive-dark",
		"slides":[
			{"title":"Quality Summary Smoke","layout":"title","subtitle":"Editable worker smoke","isTitle":true},
			{"title":"Simple Chart","layout":"chart","points":["Editable text","Native chart"],"chart":{"type":"bar","title":"Quality signal","categories":["Editable","Chart"],"values":[80,95]}}
		]
	}`
	var debugMeta PPTXArtifactDebugMetadata
	fileBytes, fileName, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Quality Summary Smoke", "", false, false, PPTXBuildOptions{
		Backend: PPTXBackendArtifactWorker,
		ArtifactDebugSink: func(meta PPTXArtifactDebugMetadata) {
			debugMeta = meta
		},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v warnings=%+v", err, warnings)
	}
	if len(fileBytes) == 0 || !strings.HasSuffix(fileName, ".pptx") {
		t.Fatalf("output = bytes %d file %q", len(fileBytes), fileName)
	}
	if debugMeta.QualitySummary == nil {
		t.Fatalf("missing quality summary: %#v", debugMeta)
	}
	summary := debugMeta.QualitySummary
	if summary.QualityGate != "pass" || !summary.EditableCoverageOK || !summary.NativeChartOK || !summary.PreviewCoverageOK || !summary.VisualVerdictOK || !summary.IssueFree {
		t.Fatalf("quality summary = %#v, want pass gate", summary)
	}
	if summary.ExpectedCharts != 1 || summary.NativeCharts < 1 || summary.PreviewCount < 2 || summary.EditableItems == 0 {
		t.Fatalf("quality summary counts = %#v", summary)
	}
}

func TestPPTXArtifactWorkerIntegrationFailsWeakVisualAssetQualityOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	workDir := t.TempDir()
	previewDir := filepath.Join(workDir, "preview")
	assetPath := filepath.Join(workDir, "weak-cover.png")
	writeSolidPNGFixture(t, assetPath, color.RGBA{R: 18, G: 24, B: 38, A: 255})
	request := pptxArtifactWorkerRequest{
		Title:               "Weak Visual Asset Gate",
		StylePreset:         "executive-dark",
		StrictVisualQuality: true,
		Slides: []officegen.Slide{
			{Title: "Weak Visual Asset Gate", Layout: "title", Subtitle: "Cover visual plate", IsTitle: true},
		},
		VisualAssets: []pptxArtifactVisualAsset{{
			Path:              assetPath,
			Name:              filepath.Base(assetPath),
			MIME:              "image/png",
			Slide:             1,
			Frame:             &pptxArtifactAssetFrame{Left: 780, Top: 118, Width: 320, Height: 250},
			TargetAspectRatio: 4.0,
			SourceAspectRatio: 1.0,
			TextDetection:     &pptxArtifactTextCheck{Checked: true, Status: "passed", Attempts: 1},
			VisualSignal:      &pptxArtifactImageSignal{Status: "low", LumaRange: 0, LumaStdDev: 0, SampleCount: 1},
			Width:             20,
			Height:            20,
		}},
		DesignPlan: &pptxArtifactDesignPlan{
			DeckIntent:    "concise-reference-style-learning",
			StyleBias:     "dark-structured",
			BuilderRecipe: "codex-reference-learning",
			Slides: []pptxArtifactSlideDesignPlan{
				{Slide: 1, Role: "cover", LayoutMode: "cover-split-visual", VisualTreatment: "text-free-visual-plate", DisplayTitle: "Weak Visual Asset Gate"},
			},
		},
		OutputPPTX:  filepath.Join(workDir, "output.pptx"),
		PreviewDir:  previewDir,
		InspectPath: filepath.Join(workDir, "inspect.json"),
	}
	output, err := runPPTXArtifactWorkerDefault(context.Background(), request, workDir)
	if err != nil {
		t.Fatalf("runPPTXArtifactWorkerDefault: %v", err)
	}
	if output.VisualVerdict != "fail" {
		t.Fatalf("visual verdict = %q score=%d, want fail", output.VisualVerdict, output.VisualScore)
	}
	var inspect struct {
		VisualVerdict struct {
			Status string `json:"status"`
			Issues []struct {
				Code string `json:"code"`
			} `json:"issues"`
		} `json:"visualVerdict"`
	}
	data, err := os.ReadFile(request.InspectPath)
	if err != nil {
		t.Fatalf("read inspect: %v", err)
	}
	if err := json.Unmarshal(data, &inspect); err != nil {
		t.Fatalf("unmarshal inspect: %v", err)
	}
	for _, expected := range []string{"LOW_RESOLUTION_VISUAL_ASSET", "VISUAL_ASSET_ASPECT_RATIO_MISMATCH", "LOW_INFORMATION_VISUAL_ASSET"} {
		found := false
		for _, issue := range inspect.VisualVerdict.Issues {
			if issue.Code == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing visual issue %s in %+v", expected, inspect.VisualVerdict.Issues)
		}
	}
}

func TestPPTXArtifactWorkerIntegrationPassesStructuralReviewOptIn(t *testing.T) {
	if os.Getenv("OFFICECLI_RUN_ARTIFACT_WORKER_TEST") != "1" {
		t.Skip("set OFFICECLI_RUN_ARTIFACT_WORKER_TEST=1 to run the local artifact-tool worker integration test")
	}
	root := t.TempDir()
	imageBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "reference-visual.png"), imageBytes, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}
	content := `{
		"title":"Reference-Driven Presentation Style Learning",
		"stylePreset":"executive-dark",
		"slides":[
			{"title":"Reference-Driven Presentation Style Learning","layout":"title","subtitle":"Capture recurring structure and tone while producing a fresh deck.","isTitle":true},
			{"title":"Key Observations","layout":"content","subtitle":"The strongest signals are structural consistency.","points":["Consistent visual system","Stable slide rhythm","Rendered preview checks are required to catch overflow and weak layouts."]},
			{"title":"Reference Profile at a Glance","layout":"chart","subtitle":"The sample set is broad enough to infer a coherent style direction.","points":["Content is the highest.","PPTX files is the lowest."],"chart":{"type":"bar","title":"Key Data Comparison","categories":["PPTX files","Parsed","Total","Content"],"values":[10,10,60,120]}},
			{"title":"Closing","layout":"closing","content":"Adopt a reference-driven builder that learns high-level presentation patterns and validates output through preview checks.","points":["Use reference signals as intent.","Keep final slides editable and preview-checked."]}
		]
	}`
	fileBytes, fileName, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), &fakeLLMClient{}, nil, content, "Reference-Driven Presentation Style Learning", "", true, false, PPTXBuildOptions{
		Backend:           PPTXBackendArtifactWorker,
		ReferenceScanRoot: root,
		UserPrompt:        "Create a concise editable presentation. Include a cover slide, key observations, one simple chart, and a closing slide.",
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Rendered preview checks are required") {
		t.Fatal("artifact worker should not leak implementation-oriented observation fragments")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Recommendation") {
		t.Fatal("artifact worker should avoid duplicating the closing title as the closing eyebrow")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/", ".xml", "Quality loop") {
		t.Fatal("artifact worker chart slide should include validation callout text")
	}
	outputPath := filepath.Join(root, fileName)
	if err := os.WriteFile(outputPath, fileBytes, 0o644); err != nil {
		t.Fatalf("write worker output: %v", err)
	}
	result, err := reviewprovider.NewService(nil, nil, nil).Review(context.Background(), reviewprovider.Request{
		FilePath:     outputPath,
		DocumentType: string(engine.DocumentTypePPTX),
		EnableVisual: false,
		RuntimeMode:  "external",
	})
	if err != nil {
		t.Fatalf("review artifact output: %v", err)
	}
	if result.StructureScore < 88 {
		t.Fatalf("structure score = %d, want >= 88; issues=%#v", result.StructureScore, result.Issues)
	}
}

func TestParsePPTXPayload_MapsReferenceSemanticStyleIntentToRendererControls(t *testing.T) {
	content := `{
		"title":"Reference Intent Demo",
		"referenceStyleSummary":"Use compact card rhythm from local references.",
		"slides":[
			{"role":"cover","headline":"Reference Intent Demo","takeaway":"Safe style intent only"},
			{"role":"summary","headline":"Reference Rhythm","takeaway":"Use cards and an editable image plate.","styleIntent":"section card rhythm","density":"compact","visualTreatment":"image-right","blocks":[{"type":"sections","sections":[
				{"heading":"Signal","detail":"Profile informs layout rhythm."},
				{"heading":"Guardrail","detail":"Renderer still owns visual tokens."}
			]}]}
		]
	}`

	payload, err := parsePPTXPayload(content, "Reference Intent Demo", "", true)
	if err != nil {
		t.Fatalf("parsePPTXPayload: %v", err)
	}
	if len(payload.Slides) < 2 {
		t.Fatalf("slides = %d", len(payload.Slides))
	}
	slide := payload.Slides[1]
	if slide.Variant != "sections-grid" {
		t.Fatalf("variant = %q, want sections-grid", slide.Variant)
	}
	if !slide.HasImage || slide.ImagePos != "right" || strings.TrimSpace(slide.ImagePrompt) == "" {
		t.Fatalf("visual treatment was not mapped to renderer image controls: %+v", slide)
	}
}

func TestBuildPPTXFromJSON_SemanticPayloadUsesControlledDesignSystem(t *testing.T) {
	content := `{
		"title":"Controlled Design Demo",
		"stylePreset":"executive-dark",
		"theme":{"preset":"executive","primaryColor":"F8FAFC","accentColor":"F9FAFB","backgroundColor":"F8FAFC","surfaceColor":"F9FAFB","textColor":"F8FAFC","titleColor":"F8FAFC"},
		"slides":[
			{"role":"cover","headline":"Controlled Design Demo","takeaway":"Low-level visual tokens from the model must not control the deck.","bgColor":"101010","bgColor2":"111111"},
			{"role":"summary","headline":"Readable Summary","takeaway":"The renderer owns contrast and surface choices.","bgColor":"F9FAFB","blocks":[{"type":"sections","sections":[
				{"heading":"Theme","detail":"Unsafe colors are ignored for semantic payloads."},
				{"heading":"Layout","detail":"Slides use controlled layout variants."}
			]}]},
			{"role":"closing","headline":"Decision","takeaway":"Keep editable slides while avoiding invisible text.","blocks":[{"type":"actions","items":["Use semantic content only","Render with controlled design tokens"]}]}
		]
	}`

	fileBytes, _, _, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Controlled Design Demo", "", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	var preview struct {
		Theme struct {
			PrimaryColor   string `json:"primaryColor"`
			AccentColor    string `json:"accentColor"`
			BgColor1       string `json:"bgColor1"`
			TextColor      string `json:"textColor"`
			TitleTextColor string `json:"titleTextColor"`
		} `json:"theme"`
		Slides []officegen.Slide `json:"slides"`
	}
	if err := json.Unmarshal(previewJSON, &preview); err != nil {
		t.Fatalf("unmarshal preview json: %v\n%s", err, string(previewJSON))
	}
	for _, unsafe := range []string{"F8FAFC", "F9FAFB"} {
		if preview.Theme.TextColor == unsafe || preview.Theme.TitleTextColor == unsafe || preview.Theme.PrimaryColor == unsafe || preview.Theme.AccentColor == unsafe {
			t.Fatalf("semantic payload leaked unsafe model theme into preview: %+v", preview.Theme)
		}
	}
	for idx, slide := range preview.Slides {
		if slide.BgColor != "" || slide.BgColor2 != "" {
			t.Fatalf("slide %d kept model-controlled background overrides: %+v", idx+1, slide)
		}
	}
	for _, unsafe := range []string{"101010", "111111", "F9FAFB"} {
		if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", unsafe) {
			t.Fatalf("pptx XML should not contain model-controlled unsafe color %s", unsafe)
		}
	}
}

func TestBuildPPTXFromJSON_SemanticGalleryVisualGeneratesAsset(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"Product Scenes",
		"slides":[
			{"role":"cover","headline":"Product Scenes","takeaway":"Show the usage context"},
			{"role":"analysis","layout":"gallery","variant":"gallery","headline":"Workspace Scene","takeaway":"A visual page should keep the generated asset editable as an image object.","blocks":[{"type":"bullets","items":["Review workflow","Shared comments"]}],"visual":{"kind":"image","position":"right","prompt":"A modern product workspace with document comments and review panels, no text overlay"}}
		]
	}`

	fileBytes, _, warnings, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "Product Scenes", "", true, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if llm.imageCalls == 0 {
		t.Fatalf("imageCalls = %d, want semantic visual image generation", llm.imageCalls)
	}
	// premium-only 模式可能产生 WARN_PPT_IMAGES_REBALANCED；只确保没有 degraded 警告。
	for _, w := range warnings {
		if w.Code == "WARN_PPT_IMAGE_DEGRADED" {
			t.Fatalf("unexpected degraded warning: %#v", warnings)
		}
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got == 0 {
		t.Fatalf("image count = %d, want generated visual asset", got)
	}
	_ = previewJSON
}

func TestBuildPPTXFromJSON_ReducesAdjacentVariantRepetition(t *testing.T) {
	content := `{
		"title":"Operating Review",
		"slides":[
			{"title":"Operating Review","layout":"title","subtitle":"A concise review"},
			{"title":"Summary","layout":"content","variant":"sections-grid","sections":[{"heading":"A","detail":"Alpha"},{"heading":"B","detail":"Beta"},{"heading":"C","detail":"Gamma"}]},
			{"title":"Customer Value","layout":"content","variant":"sections-grid","sections":[{"heading":"A","detail":"Alpha"},{"heading":"B","detail":"Beta"},{"heading":"C","detail":"Gamma"}]},
			{"title":"Operating Model","layout":"content","variant":"sections-grid","sections":[{"heading":"A","detail":"Alpha"},{"heading":"B","detail":"Beta"},{"heading":"C","detail":"Gamma"}]},
			{"title":"Execution Path","layout":"content","variant":"sections-grid","sections":[{"heading":"A","detail":"Alpha"},{"heading":"B","detail":"Beta"},{"heading":"C","detail":"Gamma"}]},
			{"title":"Risk Controls","layout":"content","variant":"sections-grid","sections":[{"heading":"A","detail":"Alpha"},{"heading":"B","detail":"Beta"},{"heading":"C","detail":"Gamma"}]},
			{"title":"Next Decision","layout":"closing","variant":"closing","sections":[{"heading":"Ask","detail":"Approve the next stage"},{"heading":"Guardrail","detail":"Review adoption before scale"}]}
		]
	}`

	_, _, _, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Operating Review", "", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	var preview struct {
		Slides []struct {
			Title   string `json:"title"`
			Layout  string `json:"layout"`
			Variant string `json:"variant"`
		} `json:"slides"`
	}
	if err := json.Unmarshal(previewJSON, &preview); err != nil {
		t.Fatalf("unmarshal preview json: %v\n%s", err, string(previewJSON))
	}
	if len(preview.Slides) < 6 {
		t.Fatalf("slide count = %d, want at least 6", len(preview.Slides))
	}
	for idx := 2; idx < len(preview.Slides); idx++ {
		prev := preview.Slides[idx-1]
		cur := preview.Slides[idx]
		if prev.Layout == "content" && cur.Layout == "content" && prev.Variant != "" && prev.Variant == cur.Variant {
			t.Fatalf("adjacent content slides %d and %d reuse variant %q:\n%s", idx, idx+1, cur.Variant, string(previewJSON))
		}
	}
}

func TestBuildPPTXFromJSON_ReducesRenderedBulletVariantRepetition(t *testing.T) {
	slides := reduceAdjacentVariantRepetition([]officegen.Slide{
		{Title: "Operating Model", Layout: "content", Variant: "bullets", Points: []string{"Clarify owners", "Ship in phases", "Measure quality"}},
		{Title: "Delivery Model", Layout: "content", Variant: "bullets-plain", Points: []string{"Separate semantic spec", "Control rendering", "Review output"}},
	})
	if len(slides) != 2 {
		t.Fatalf("slides = %#v", slides)
	}
	if renderedVariantRhythmKey(slides[0]) == renderedVariantRhythmKey(slides[1]) {
		t.Fatalf("render-equivalent bullet variants should be diversified: %#v", slides)
	}
}

func TestBuildPPTXFromJSON_NormalizesQualityConstraints(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"Quarterly Summary",
		"theme":{"primaryColor":"1A73E8","accentColor":"E8710A","backgroundType":"gradient","bgColor1":"F0F4FF","bgColor2":"FFFFFF"},
		"slides":[
			{"title":"An Extremely Long First Slide Title That Needs To Be Tightened","layout":"content","points":["The first point is intentionally very long and should be truncated to control slide density","The second point is also deliberately long and should be processed","Third point","Fourth point","Fifth point"],"hasImage":true,"imagePrompt":"A complex market-analysis poster with many visual elements","imagePos":"background"},
			{"title":"Second Slide","layout":"content","points":["Conclusion one","Conclusion two","Conclusion three"],"hasImage":true,"imagePrompt":"An international office scene","imagePos":"left"},
			{"title":"Third Slide","layout":"content","points":["Conclusion one","Conclusion two","Conclusion three"],"hasImage":true,"imagePrompt":"A team meeting","imagePos":"right"},
			{"title":"Fourth Slide","layout":"content","content":"This is a long paragraph. It should be split into multiple readable points. The second sentence adds more context. The third sentence keeps explaining the idea."},
			{"title":"Fifth Slide","layout":"dashboard","metrics":[{"label":"ARR","value":"8.2M","note":"+32% YoY"},{"label":"NDR","value":"118%","note":"Renewal improvement"}]},
			{"title":"Sixth Slide","layout":"chart","chart":{"title":"Regional Revenue","type":"bar","categories":["North America","Europe","Southeast Asia","Middle East","Japan","Korea"],"values":[42,31,28,17,12,9]}},
			{"title":"Seventh Slide","layout":"content","points":["Conclusion one","Conclusion two","Conclusion three"]},
			{"title":"Eighth Slide","layout":"content","points":["This slide should be trimmed"]}		
		]
	}`

	fileBytes, _, warnings, _, _, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "Quarterly Summary", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if got := countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); got != 10 {
		t.Fatalf("slide count = %d, want 10", got)
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got > 2 {
		t.Fatalf("image count = %d, want at most 2 after image rebalancing", got)
	}
	slide1 := readZipEntry(t, fileBytes, "ppt/slides/slide1.xml")
	if strings.Contains(slide1, "●") {
		t.Fatalf("title slide should not render bullet content: %s", slide1)
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "It should be split") {
		t.Fatalf("content slide should be normalized into readable points")
	}
	for idx := 1; idx <= countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); idx++ {
		rels := readZipEntry(t, fileBytes, filepath.ToSlash(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", idx)))
		slideXML := readZipEntry(t, fileBytes, fmt.Sprintf("ppt/slides/slide%d.xml", idx))
		if strings.Contains(slideXML, "ChartPanel") && strings.Contains(rels, "image") {
			t.Fatalf("chart slide should not keep image rels: %s", rels)
		}
	}
	if len(warnings) == 0 {
		t.Fatalf("warnings = %#v, want normalization warnings", warnings)
	}
}

func TestBuildPPTXFromJSON_ExplainerUsesMixedLayoutsAndSkipsScaffold(t *testing.T) {
	content := `{
		"title":"Minecraft Introduction",
		"slides":[
			{"title":"Minecraft Introduction","layout":"title","subtitle":"A beginner-friendly overview"},
			{"title":"Overview","layout":"content","sections":[
				{"heading":"High Replayability","detail":"Minecraft is best understood as an open-ended sandbox built around exploration, building, and survival."},
				{"heading":"Creative Range","detail":"Players can keep discovering new goals instead of following one fixed path."},
				{"heading":"Shared Play","detail":"Friends can explore and build together in the same world."}
			]},
			{"title":"Main Loop","layout":"content","points":["Learn a few core recipes first","Gather materials and build a simple shelter","Use short sessions to discover the main loop"]},
			{"title":"Standout Traits","layout":"content","points":["Try the mode that matches your mood and skill level","The same world can feel relaxing, creative, or challenging","Replayability comes from self-directed goals"]},
			{"title":"Audience Fit","layout":"content","points":["Beginners can start small","Creative players can experiment freely","Challenge seekers can focus on survival"]}
		]
	}`

	fileBytes, _, _, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "minecraft 游戏介绍", "", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if got := countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); got != 6 {
		t.Fatalf("slide count = %d, want 6", got)
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "Contents") {
		t.Fatalf("explainer deck should not contain a contents slide")
	}
	for _, needle := range []string{"What It Is", "Learn a few", "How to Start"} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", needle) {
			t.Fatalf("deck should preserve %q", needle)
		}
	}
	if !strings.Contains(string(previewJSON), `"stylePreset": "explainer-voxel-light"`) {
		t.Fatalf("preview json = %s", string(previewJSON))
	}
	for _, needle := range []string{`"variant": "bullets-plain"`, `"variant": "timeline-axis"`, `"variant": "comparison-columns"`, `"variant": "sections-grid-3up"`, `"variant": "timeline-steps"`} {
		if !strings.Contains(string(previewJSON), needle) {
			t.Fatalf("preview json missing %q:\n%s", needle, string(previewJSON))
		}
	}
}

func TestSuggestStylePreset_RoutesChineseBusinessThemes(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		archetype pptxArchetype
		want      string
	}{
		{name: "pitch", text: "AI 客服质检平台融资路演", archetype: pptxArchetypeGeneral, want: officegen.StylePresetInvestorWarm},
		{name: "sales procurement", text: "企业协作平台采购建议与价值解读", archetype: pptxArchetypeCompany, want: officegen.StylePresetSlateSerif},
		{name: "project", text: "集团数字化项目实施方案", archetype: pptxArchetypeGeneral, want: officegen.StylePresetProjectForest},
		{name: "training", text: "新员工远程协作入职培训", archetype: pptxArchetypeTraining, want: officegen.StylePresetTrainingManual},
		{name: "review", text: "2026 年第一季度经营复盘", archetype: pptxArchetypeOps, want: officegen.StylePresetReviewCopper},
		{name: "board review", text: "Q3 Board Review", archetype: pptxArchetypeOps, want: officegen.StylePresetReviewCopper},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := suggestStylePreset("", tc.archetype, tc.text); got != tc.want {
				t.Fatalf("suggestStylePreset(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestDetectPPTXArchetype_RecognizesBoardReview(t *testing.T) {
	if got := detectPPTXArchetype("Create a 6-8 slide board-review deck for OfficeCLI.", "Q3 Board Review"); got != pptxArchetypeOps {
		t.Fatalf("detectPPTXArchetype(board review) = %q, want %q", got, pptxArchetypeOps)
	}
}

func TestBuildPPTXFromJSON_ProjectPlanUsesStructuredLaunchArc(t *testing.T) {
	content := `{
		"title":"Cross-Functional Launch Plan",
		"slides":[
			{"role":"cover","layout":"title","headline":"Cross-Functional Launch Plan","takeaway":"A clear operating model will align teams and reduce launch risk"},
			{"role":"summary","layout":"content","headline":"Executive Summary","takeaway":"The launch team should align on four measurable goals.","blocks":[{"type":"sections","sections":[
				{"heading":"Goals","detail":"Lock scope, hit launch date, achieve readiness across GTM and support, and stabilize quality before release"},
				{"heading":"Operating Model","detail":"Use weekly decision forums."},
				{"heading":"Decision Need","detail":"Approve owners."}
			]}]},
			{"role":"action","layout":"timeline","headline":"Milestones and Decision Gates","takeaway":"A milestone-driven path creates clear handoffs.","blocks":[{"type":"timeline","sections":[
				{"heading":"T-8 to T-6 Weeks","detail":"Finalize goals, scope, owners, and timeline."},
				{"heading":"T-5 to T-3 Weeks","detail":"Complete QA, messaging, enablement, and support prep."}
			]}]}
		]
	}`

	fileBytes, _, _, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "OfficeCLI New Release Plan", "", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if got := countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); got != 7 {
		t.Fatalf("slide count = %d, want 7", got)
	}
	for _, needle := range []string{"Decision Snapshot: GO", "Gate Scorecard: Proceed", "Workstream Ownership", "Milestone Gates", "Risk Controls: Covered", "Decision Request: Approve GO"} {
		if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", needle) {
			t.Fatalf("project deck missing %q", needle)
		}
	}
	if archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "Executive Summary") {
		t.Fatalf("project deck should replace repeated executive-summary fragments")
	}
	preview := string(previewJSON)
	for _, needle := range []string{`"stylePreset": "tech-contrast"`, `"variant": "comparison-spotlight"`, `"variant": "kpi-band"`, `"variant": "comparison-columns"`, `"variant": "timeline-steps"`, `"variant": "closing-decision-banner"`} {
		if !strings.Contains(preview, needle) {
			t.Fatalf("preview json missing %q:\n%s", needle, preview)
		}
	}
	if got := strings.Count(preview, `"variant": "sections-grid`); got > 1 {
		t.Fatalf("project deck repeats sections-grid variants %d times; preview:\n%s", got, preview)
	}
	if got := strings.Count(strings.ToLower(preview), "green-light"); got > 1 {
		t.Fatalf("project deck repeats green-light decision language %d times; preview:\n%s", got, preview)
	}
	for idx, bad := range map[int][]string{
		2: {"Hold if launch quality."},
		3: {"Launch risk is green only."},
		4: {"Quality or enablement gaps would create.", "A blocker needs a DRI, mitigation plan."},
	} {
		slideXML := readZipEntry(t, fileBytes, fmt.Sprintf("ppt/slides/slide%d.xml", idx))
		for _, fragment := range bad {
			if strings.Contains(slideXML, fragment) {
				t.Fatalf("slide %d renders incomplete fragment %q:\n%s", idx, fragment, slideXML)
			}
		}
	}
}

func TestNormalizePPTXPayload_AutoThemeOverridesLLMThemeWhenStyleIsImplicit(t *testing.T) {
	payload := &pptxPayload{
		Title:       "企业协作平台采购建议与价值解读",
		StylePreset: "editorial-light",
		Theme: &officegen.SlideTheme{
			PrimaryColor:   "1A73E8",
			AccentColor:    "E8710A",
			BackgroundType: "gradient",
			BgColor1:       "F0F4FF",
			BgColor2:       "FFFFFF",
			TextColor:      "0F172A",
			TitleTextColor: "0F172A",
		},
		Slides: []officegen.Slide{
			{Title: "企业协作平台采购建议与价值解读", Layout: "title", Subtitle: "管理层摘要"},
		},
	}

	normalizePPTXPayload(payload, "企业协作平台采购建议与价值解读", "", true)

	if payload.StylePreset != officegen.StylePresetSlateSerif {
		t.Fatalf("style preset = %q, want %q", payload.StylePreset, officegen.StylePresetSlateSerif)
	}
	if payload.Theme == nil || payload.Theme.AccentColor != "2563EB" || payload.Theme.BgColor1 != "F6F8FB" || payload.Theme.TextColor != "1E293B" {
		t.Fatalf("theme = %+v, want collaboration-slate preset theme", payload.Theme)
	}
}

func TestNormalizePPTXPayload_CompactsSlideTextDensity(t *testing.T) {
	payload := &pptxPayload{
		Title:       "生成方案升级",
		StylePreset: "tech-contrast",
		Slides: []officegen.Slide{
			{Title: "生成方案升级", Layout: "title", Subtitle: "管理层摘要"},
			{
				Title:         "落地路径：先立中间层，再逐步替换直出链路",
				Layout:        "timeline",
				Variant:       "timeline",
				NarrativeRole: "action",
				Subtitle:      "建议用三阶段实施，先把质量闸门建起来，再扩模板与场景，避免一次性重构风险过高。",
				Sections: []officegen.SlideSection{
					{Heading: "阶段一：建立 Spec 与校验基线（4周）", Detail: "Owner：平台工程 + 产品；产出统一 JSON schema、失败类型字典、基础对比度与溢出校验；验收：标准 8 页汇报无人工重排。"},
					{Heading: "阶段二：接入受控设计系统（4-6周）", Detail: "Owner：文档生成工程；把主题、颜色、字号和布局槽位全部收敛到 renderer，禁止模型输出低层样式。"},
					{Heading: "阶段三：扩大模板覆盖（持续迭代）", Detail: "Owner：产品与质量评估；按业务汇报、培训、市场分析等场景扩展布局，同时沉淀可量化质量指标。"},
				},
			},
		},
	}

	normalizePPTXPayload(payload, "生成方案升级", "", false)
	if len(payload.Slides) < 2 {
		t.Fatalf("slides = %#v", payload.Slides)
	}
	for idx, slide := range payload.Slides {
		total := utf8.RuneCountInString(slide.Title) + utf8.RuneCountInString(slide.Subtitle)
		for _, point := range slide.Points {
			total += utf8.RuneCountInString(point)
		}
		for _, section := range slide.Sections {
			total += utf8.RuneCountInString(section.Heading) + utf8.RuneCountInString(section.Detail)
		}
		if total > 240 {
			t.Fatalf("slide %d text density = %d, want <= 240: %+v", idx+1, total, slide)
		}
	}
}

func TestDiversifyBusinessLayouts_ReducesRepeatedSectionCardsAndClosing(t *testing.T) {
	slides := []officegen.Slide{
		{Title: "Cover", Layout: "title"},
		{Title: "Summary", Layout: "content", Variant: "sections-grid", Sections: []officegen.SlideSection{{Heading: "A", Detail: "Alpha"}, {Heading: "B", Detail: "Beta"}, {Heading: "C", Detail: "Gamma"}}},
		{Title: "Capabilities", Layout: "content", Variant: "sections-grid", Sections: []officegen.SlideSection{{Heading: "A", Detail: "Alpha"}, {Heading: "B", Detail: "Beta"}, {Heading: "C", Detail: "Gamma"}}},
		{Title: "Action Plan", Layout: "closing", Variant: "closing", Sections: []officegen.SlideSection{{Heading: "Now", Detail: "Do first"}, {Heading: "Next", Detail: "Do second"}}},
		{Title: "Next Steps", Layout: "closing", Variant: "closing", Sections: []officegen.SlideSection{{Heading: "Week 1", Detail: "Kick off"}, {Heading: "Week 2", Detail: "Review"}}},
	}

	got := diversifyBusinessLayouts(slides, pptxArchetypeOps)

	if got[1].Layout != "content" || got[1].Variant != "bullets" || len(got[1].Points) == 0 {
		t.Fatalf("first repeated sections-grid slide should become bullets: %+v", got[1])
	}
	if got[3].Layout == "closing" {
		t.Fatalf("penultimate closing slide should be diversified: %+v", got[3])
	}
	if got[4].Layout != "closing" {
		t.Fatalf("final slide should remain closing: %+v", got[4])
	}
}

func TestBuildPPTXFromJSON_ExplainerImagesUseHeroAndGameplayVisuals(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	content := `{
		"title":"Minecraft Introduction",
		"slides":[
			{"title":"Minecraft Introduction","layout":"title","subtitle":"A beginner-friendly overview"},
			{"title":"What It Is","layout":"content","points":["A sandbox world built from blocks","Players gather, craft, and build","Different modes change the experience"]}
		]
	}`

	fileBytes, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "minecraft 游戏介绍", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if got := countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); got != 7 {
		t.Fatalf("slide count = %d, want 7", got)
	}
	if got := countZipEntries(fileBytes, "ppt/media/", ".png"); got != 3 {
		t.Fatalf("image count = %d, want 3", got)
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "Example / Gameplay Visual") {
		t.Fatalf("deck should contain the visual example slide")
	}
}

func TestBuildPPTXFromJSON_ExplainerSingleVisualFallsBackToImageRight(t *testing.T) {
	llm := &fakeLLMClient{
		imageResults: []*engine.ImageGenerationResult{
			nil,
			{Data: mustTinyPNG(t), MIME: "image/png"},
			nil,
		},
		imageErrors: []error{
			errors.New("cover failed"),
			nil,
			errors.New("second visual failed"),
		},
	}
	content := `{
		"title":"Minecraft Introduction",
		"slides":[
			{"title":"Minecraft Introduction","layout":"title","subtitle":"A beginner-friendly overview"},
			{"title":"What It Is","layout":"content","points":["A sandbox world built from blocks","Players gather, craft, and build","Different modes change the experience"]}
		]
	}`

	_, _, warnings, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "minecraft 游戏介绍", "", true, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if !strings.Contains(string(previewJSON), `"title": "Example / Gameplay Visual"`) || !strings.Contains(string(previewJSON), `"variant": "image-right-focus"`) {
		t.Fatalf("preview json should downgrade the example slide to image-right:\n%s", string(previewJSON))
	}
	if len(warnings) == 0 || !strings.Contains(warnings[len(warnings)-1].Message, "successfully generated visuals were kept") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestBuildPPTXFromJSON_ExplainerNoVisualSuccessFallsBackToTextPage(t *testing.T) {
	llm := &fakeLLMClient{
		imageErrors: []error{
			errors.New("cover failed"),
			errors.New("visual 1 failed"),
			errors.New("visual 2 failed"),
		},
	}
	content := `{
		"title":"Minecraft Introduction",
		"slides":[
			{"title":"Minecraft Introduction","layout":"title","subtitle":"A beginner-friendly overview"},
			{"title":"What It Is","layout":"content","points":["A sandbox world built from blocks","Players gather, craft, and build","Different modes change the experience"]}
		]
	}`

	_, _, warnings, _, previewJSON, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "minecraft 游戏介绍", "", true, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if strings.Contains(string(previewJSON), `"layout": "gallery"`) && strings.Contains(string(previewJSON), `"title": "Example / Gameplay Visual"`) {
		t.Fatalf("preview json should not keep the example slide as gallery when no visuals succeed:\n%s", string(previewJSON))
	}
	if !strings.Contains(string(previewJSON), `"title": "Example / Gameplay Visual"`) || !strings.Contains(string(previewJSON), `"variant": "bullets-callout"`) {
		t.Fatalf("preview json should turn the example slide into a text explain page:\n%s", string(previewJSON))
	}
	if len(warnings) == 0 || !strings.Contains(warnings[len(warnings)-1].Message, "deck was generated without images") {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestBuildPPTXFromJSON_ImageFailureWarningIncludesReason(t *testing.T) {
	llm := &fakeLLMClient{
		imageErr: errors.New("llm request failed: status=429 body=upstream saturated"),
	}
	content := `{
		"title":"Product Launch",
		"slides":[
			{"title":"Product Launch","layout":"title","subtitle":"Go-to-market overview","hasImage":true,"imagePrompt":"A polished product launch visual","imagePos":"background"},
			{"title":"Market Signal","layout":"content","points":["Demand is rising","Pipeline is qualified","Launch window is clear"]}
		]
	}`

	_, _, warnings, _, _, err := BuildPPTXFromJSON(context.Background(), llm, nil, content, "Product Launch", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("warnings = %#v, want image degradation warning", warnings)
	}
	if got := warnings[len(warnings)-1].Message; !strings.Contains(got, "status=429") || !strings.Contains(got, "upstream saturated") {
		t.Fatalf("warning should include image failure reason, got: %q", got)
	}
}

func TestBuildPPTXFromJSON_PremiumImagePromptAndCoverUseSafeLayout(t *testing.T) {
	balance := 9
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png", CreditBalance: &balance},
	}
	content := `{
		"title":"Product Launch",
		"slides":[
			{"title":"Product Launch","layout":"title","subtitle":"Go-to-market overview","hasImage":true,"imagePrompt":"A polished product launch poster with dashboard text","imagePos":"background"},
			{"title":"Market Signal","layout":"content","points":["Demand is rising","Pipeline is qualified","Launch window is clear"],"hasImage":true,"imagePrompt":"A market dashboard visual","imagePos":"right"},
			{"title":"Decision","layout":"closing","narrativeRole":"closing","sections":[{"heading":"Ask","detail":"Approve the launch window"}],"hasImage":true,"imagePrompt":"A bright closing background","imagePos":"background"}
		]
	}`

	fileBytes, _, warnings, _, previewJSON, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, nil, content, "Product Launch", "", true, true, PPTXBuildOptions{})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if llm.imageCalls == 0 {
		t.Fatalf("build should request at least one image")
	}
	if got := llm.lastImageRequest.Prompt; !strings.Contains(got, "no text, no letters, no words, no UI labels, no charts with labels, no typography") {
		t.Fatalf("image prompt missing no-text constraints: %q", got)
	}
	if !containsIssueCode(warnings, "INFO_PPT_HOSTED_IMAGE_CREDITS") {
		t.Fatalf("warnings should expose hosted image credit balance: %#v", warnings)
	}
	var preview struct {
		Slides []officegen.Slide `json:"slides"`
	}
	if err := json.Unmarshal(previewJSON, &preview); err != nil {
		t.Fatalf("unmarshal preview json: %v\n%s", err, string(previewJSON))
	}
	if len(preview.Slides) < 2 {
		t.Fatalf("preview slides = %#v", preview.Slides)
	}
	cover := preview.Slides[0]
	if cover.ImagePos == "background" {
		t.Fatalf("premium cover image should not be full-slide background: %+v", cover)
	}
	slideXML := readZipEntry(t, fileBytes, "ppt/slides/slide1.xml")
	picStart := strings.Index(slideXML, "<p:pic>")
	picEnd := -1
	if picStart >= 0 {
		picEnd = strings.Index(slideXML[picStart:], "</p:pic>")
	}
	if picStart < 0 || picEnd < 0 {
		t.Fatalf("premium cover should render an image:\n%s", slideXML)
	}
	picXML := slideXML[picStart : picStart+picEnd]
	if strings.Contains(picXML, `<a:off x="0" y="0"/>`) && strings.Contains(picXML, `cx="12192000"`) {
		t.Fatalf("premium cover should render a side image, not a background image:\n%s", slideXML)
	}
	closing := preview.Slides[len(preview.Slides)-1]
	if closing.HasImage || strings.TrimSpace(closing.ImagePrompt) != "" || strings.TrimSpace(closing.ImagePos) != "" {
		t.Fatalf("premium closing slide should not keep images: %+v", closing)
	}
}

func TestBuildFallbackImagePrompt_MinecraftUsesVoxelConstraints(t *testing.T) {
	prompt := buildFallbackImagePrompt(officegen.Slide{
		Title:         "Minecraft Introduction",
		NarrativeRole: "analysis",
		Subtitle:      "Show the blocky sandbox world and the survival loop",
	}, "Minecraft Introduction")
	for _, needle := range []string{"blocky voxel sandbox", "Minecraft-like cubic terrain", "crafting", "biomes", "survival shelter", "block building"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
	for _, needle := range []string{"hand-painted fantasy", "corporate diagram"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing negative constraint %q: %s", needle, prompt)
		}
	}
}

func TestBuildPPTXFromJSON_DowngradesTimelineChartsToSections(t *testing.T) {
	content := `{
		"title":"Release Cadence",
		"slides":[
			{"title":"Release Cadence","layout":"title","subtitle":"Start with the milestones"},
			{"title":"Cadence and Milestones","layout":"chart","chart":{"title":"Release Stage Plan","type":"bar","categories":["Requirements Freeze","Development Sync","Testing Rollout","General Availability"],"values":[1,3,2,2]}}
		]
	}`

	fileBytes, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Release Cadence", "", false, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "Executive Summary") {
		t.Fatalf("deck should contain an overview slide")
	}
	if !archiveContainsEntryWithSubstring(t, fileBytes, "ppt/slides/slide", ".xml", "Cadence and Milest") {
		t.Fatalf("deck should contain the downgraded timeline title")
	}
	for idx := 1; idx <= countZipEntries(fileBytes, "ppt/slides/slide", ".xml"); idx++ {
		slideXML := readZipEntry(t, fileBytes, fmt.Sprintf("ppt/slides/slide%d.xml", idx))
		if strings.Contains(slideXML, "Cadence and Milest") && strings.Contains(slideXML, `r:id="rId1"`) {
			t.Fatalf("timeline slide should be downgraded from chart rels:\n%s", slideXML)
		}
	}
}

func TestBuildPPTXFromJSON_BuildsLocalPreviewSidecars(t *testing.T) {
	content := `{
		"title":"Local Preview Test",
		"slides":[
			{"title":"Local Preview Test","layout":"title","variant":"title-center","subtitle":"Start with the structure"},
			{"title":"Key Takeaway","layout":"content","variant":"bullets","subtitle":"Lead with the conclusion","points":["Point one","Point two","Point three"]}
		]
	}`

	_, _, _, previewHTML, previewJSON, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, nil, content, "Local Preview Test", "executive-dark", false, true)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}
	if !strings.Contains(string(previewHTML), "Preset: executive-dark") {
		t.Fatalf("preview html = %s", string(previewHTML))
	}
	if !strings.Contains(string(previewJSON), `"stylePreset": "executive-dark"`) {
		t.Fatalf("preview json = %s", string(previewJSON))
	}
}

func TestServiceGeneratePPTX_RetriesOnceWhenJSONIsTruncated(t *testing.T) {
	llm := &fakeLLMClient{
		jsonResponses: []string{
			`{"title":"Knowledge Collaboration Product Overview","slides":[{"title":"Cover","layout":"title","subtitle":"One-line takeaway","isTitle":true}`,
		},
		structuredResponse: `{
			"title":"Knowledge Collaboration Product Overview",
			"slides":[
				{"title":"Cover","layout":"title","subtitle":"One-line takeaway","isTitle":true},
				{"title":"Product Capabilities","layout":"content","points":["Higher collaboration efficiency","Clear permission governance","A clear rollout path"]}
			]
		}`,
	}
	service := NewService(llm, nil)

	doc, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypePPTX,
		Prompt:       "Describe the product capabilities, customer value, and use cases of this knowledge collaboration product.",
		Topic:        "Knowledge Collaboration Product Overview",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if llm.jsonCallCount != 1 {
		t.Fatalf("jsonCallCount = %d, want 1", llm.jsonCallCount)
	}
	if llm.structuredCallCount != 1 {
		t.Fatalf("structuredCallCount = %d, want 1", llm.structuredCallCount)
	}
	if llm.lastStructuredReq.Schema.Name != "pptx_payload_repair" {
		t.Fatalf("schema name = %q", llm.lastStructuredReq.Schema.Name)
	}
	if len(llm.lastStructuredReq.Messages) != 3 {
		t.Fatalf("repair messages = %d, want 3", len(llm.lastStructuredReq.Messages))
	}
	if !archiveContainsEntryWithSubstring(t, doc.Bytes, "ppt/slides/slide", ".xml", "Higher collaboration efficiency") {
		t.Fatalf("deck should contain repaired slide content")
	}
}

func TestBuildPPTXFromJSON_EmitsStartAndReadyPerImage(t *testing.T) {
	llm := &fakeLLMClient{
		imageResult: &engine.ImageGenerationResult{Data: mustTinyPNG(t), MIME: "image/png"},
	}
	collector := &runtimeProgressCollector{}
	content := `{
		"title":"Visual Gallery Demo",
		"slides":[
			{"title":"Visual Gallery Demo","layout":"title","variant":"title-center","subtitle":"Open with the topic"},
			{"title":"Product Scenes","layout":"gallery","variant":"gallery","narrativeRole":"analysis","sectionIndex":1,"sectionTitle":"Core Storyline","subtitle":"Use visuals to show the product context","visuals":[
				{"label":"Workspace","prompt":"A modern collaboration workspace","caption":"Workspace view"},
				{"label":"Meeting","prompt":"A product review meeting","caption":"Review scene"},
				{"label":"Field","prompt":"A field deployment scene","caption":"Field scene"}
			]}
		]
	}`

	_, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), llm, collector, content, "Visual Gallery Demo", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}

	starts := 0
	readys := 0
	for _, event := range collector.events {
		if event.Step != progressStepAssemble {
			continue
		}
		if strings.HasPrefix(event.Content, "Generating image asset (") {
			starts++
		}
		if strings.HasPrefix(event.Content, "Image asset ") && strings.HasSuffix(event.Content, " ready") {
			readys++
		}
	}
	// premium-only 模式 visualBudget=1，每张幻灯片只生成一张图，因此只期望 >=1。
	if starts < 1 {
		t.Fatalf("expected >=1 'Generating image asset' events, got %d (events=%+v)", starts, collector.events)
	}
	if readys != starts {
		t.Fatalf("expected one 'ready' per start; starts=%d readys=%d (events=%+v)", starts, readys, collector.events)
	}
	if llm.imageCalls != starts {
		t.Fatalf("expected imageCalls (%d) to equal start events (%d)", llm.imageCalls, starts)
	}
}

func TestBuildPPTXFromJSON_EmitsFailedPerAssetWhenImageProviderErrors(t *testing.T) {
	llm := &fakeLLMClient{
		imageErr: errors.New("provider unreachable"),
	}
	collector := &runtimeProgressCollector{}
	content := `{
		"title":"Visual Gallery Demo",
		"slides":[
			{"title":"Visual Gallery Demo","layout":"title","variant":"title-center","subtitle":"Open"},
			{"title":"Scene","layout":"gallery","variant":"gallery","narrativeRole":"analysis","sectionIndex":1,"sectionTitle":"Core","subtitle":"caption","visuals":[
				{"label":"Workspace","prompt":"A workspace","caption":"Workspace view"}
			]}
		]
	}`

	_, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), llm, collector, content, "Visual Gallery Demo", "", true, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}

	failed := 0
	for _, event := range collector.events {
		if strings.HasPrefix(event.Content, "Image asset ") && strings.Contains(event.Content, " failed: ") {
			failed++
		}
	}
	if failed == 0 {
		t.Fatalf("expected at least one 'failed' progress event, got events=%+v", collector.events)
	}
}

func TestBuildPPTXArtifactExperimentalEmitsDesignPlannerHeartbeat(t *testing.T) {
	restore := SetStructuredLLMHeartbeatIntervalForTesting(20 * time.Millisecond)
	defer restore()

	original := runPPTXArtifactWorker
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 1, 0)
		output.WorkerVersion = "artifact-experimental-test"
		return output, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	llm := &fakeLLMClient{
		structuredDelay:    90 * time.Millisecond,
		structuredResponse: `{"deckIntent":"concise-reference-style-learning","visualSystem":"editorial","builderRecipe":"codex-reference-learning","slides":[{"slide":1,"role":"cover","layoutMode":"title","composition":"split","densityTarget":"medium","title":"Reference Style Test","subtitle":"Editable summary","takeaway":"Reference style captured","cards":[],"chartCallouts":[]}],"builderPatch":{"slides":[]}}`,
	}
	collector := &runtimeProgressCollector{}
	content := `{
		"title":"Reference Style Test",
		"slides":[
			{"title":"Reference Style Test","layout":"title","subtitle":"Editable summary"}
		]
	}`

	_, _, _, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, collector, content, "Reference Style Test", "executive-dark", false, false, PPTXBuildOptions{
		Backend:                    PPTXBackendArtifactWorker,
		UserPrompt:                 "Create a concise editable presentation that learns the style from PPTX files in this directory.",
		GenerateArtifactDesignPlan: true,
		ArtifactDesignPlanLLM:      llm,
		ReferenceBrief:             &PPTXReferenceStyleBrief{StylePresetHint: officegen.StylePresetExecutiveDark},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}

	found := false
	for _, event := range collector.events {
		if event.Step == progressStepAssemble && strings.Contains(event.Content, "Still waiting on artifact design planner") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing artifact design planner heartbeat; events=%+v", collector.events)
	}
}

func TestBuildPPTXArtifactExperimentalFallsBackWhenDesignPlannerTimesOut(t *testing.T) {
	restoreTimeout := SetPPTXArtifactDesignPlanTimeoutForTesting(30 * time.Millisecond)
	defer restoreTimeout()

	original := runPPTXArtifactWorker
	var capturedPlan *pptxArtifactDesignPlan
	runPPTXArtifactWorker = func(_ context.Context, req pptxArtifactWorkerRequest, _ string) (*pptxArtifactWorkerOutput, error) {
		capturedPlan = req.DesignPlan
		data, err := officegen.NewPPTXGenerator().Generate(req.Slides, officegen.PPTXOptions{
			Title:       req.Title,
			Creator:     "test",
			Theme:       req.Theme,
			StylePreset: req.StylePreset,
		})
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(req.OutputPPTX, data, 0o644); err != nil {
			return nil, err
		}
		output := writePPTXArtifactFakeDiagnostics(t, req, 1, 0)
		output.WorkerVersion = "artifact-experimental-test"
		return output, nil
	}
	defer func() { runPPTXArtifactWorker = original }()

	llm := &fakeLLMClient{
		structuredDelay:    200 * time.Millisecond,
		structuredResponse: `{"deckIntent":"should-not-arrive","slides":[]}`,
	}
	content := `{
		"title":"Reference Style Test",
		"slides":[
			{"title":"Reference Style Test","layout":"title","subtitle":"Editable summary"}
		]
	}`

	start := time.Now()
	_, _, warnings, _, _, err := BuildPPTXFromJSONWithOptions(context.Background(), llm, nil, content, "Reference Style Test", "executive-dark", false, false, PPTXBuildOptions{
		Backend:                    PPTXBackendArtifactWorker,
		UserPrompt:                 "Create a concise editable presentation that learns the style from PPTX files in this directory.",
		GenerateArtifactDesignPlan: true,
		ArtifactDesignPlanLLM:      llm,
		ReferenceBrief:             &PPTXReferenceStyleBrief{StylePresetHint: officegen.StylePresetExecutiveDark},
	})
	if err != nil {
		t.Fatalf("BuildPPTXFromJSONWithOptions: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("planner timeout fallback took too long: %s", elapsed)
	}
	if !containsIssueCode(warnings, "WARN_PPTX_ARTIFACT_DESIGN_PLAN_FALLBACK") {
		t.Fatalf("missing design-plan fallback warning: %#v", warnings)
	}
	if capturedPlan == nil || capturedPlan.DeckIntent == "should-not-arrive" {
		t.Fatalf("worker should receive deterministic fallback plan, got %#v", capturedPlan)
	}
}

func TestBuildPPTXFromJSON_EmitsFinalizingBeforeAssemblyCompleted(t *testing.T) {
	collector := &runtimeProgressCollector{}
	content := `{
		"title":"Text Only Deck",
		"slides":[
			{"title":"Text Only Deck","layout":"title","variant":"title-center","subtitle":"Cover"},
			{"title":"Body","layout":"text","variant":"bullets","subtitle":"Points","points":["alpha","beta"]}
		]
	}`

	_, _, _, _, _, err := BuildPPTXFromJSON(context.Background(), &fakeLLMClient{}, collector, content, "Text Only Deck", "", false, false)
	if err != nil {
		t.Fatalf("BuildPPTXFromJSON: %v", err)
	}

	var finalizingIdx, completedIdx int = -1, -1
	for i, event := range collector.events {
		if strings.Contains(event.Content, "Finalizing PPTX layout") {
			finalizingIdx = i
		}
		if strings.Contains(event.Content, "PPTX assembly completed") {
			completedIdx = i
		}
	}
	if finalizingIdx < 0 {
		t.Fatalf("expected 'Finalizing PPTX layout' event, got events=%+v", collector.events)
	}
	if completedIdx < 0 {
		t.Fatalf("expected 'PPTX assembly completed' event, got events=%+v", collector.events)
	}
	if finalizingIdx >= completedIdx {
		t.Fatalf("finalizing event should come before completed, got finalizing=%d completed=%d", finalizingIdx, completedIdx)
	}
}

func TestServiceGenerateDOCXEmitsProgressEvents(t *testing.T) {
	collector := &runtimeProgressCollector{}
	service := NewService(&fakeLLMClient{
		jsonResponse: `{"title":"Enterprise Collaboration Platform Overview","sections":[{"heading":"Product Overview","level":1,"paragraphs":["This collaboration platform is designed for enterprise teams."]}]}`,
	}, collector)

	_, err := service.Generate(context.Background(), GenerateParams{
		DocumentType: engine.DocumentTypeDOCX,
		Prompt:       "Introduce this enterprise collaboration platform",
		Topic:        "Enterprise Collaboration Platform Overview",
		Mode:         "fast",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	joined := make([]string, 0, len(collector.events))
	for _, event := range collector.events {
		joined = append(joined, event.Step+":"+event.Status+":"+event.Content)
	}
	output := strings.Join(joined, "\n")
	for _, needle := range []string{
		"generate_llm:running:Requesting DOCX content from the LLM",
		"generate_llm:completed:Received DOCX structure output",
		"assemble:running:Assembling the DOCX file",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("progress output missing %q:\n%s", needle, output)
		}
	}
}
