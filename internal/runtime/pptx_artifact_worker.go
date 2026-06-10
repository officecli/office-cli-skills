package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	reviewprovider "github.com/officecli/officecli/internal/review"
	"github.com/officecli/officecli/pkg/officegen"
)

const pptxArtifactMinimumStructureScore = 88
const pptxArtifactMaxPolishPasses = 2
const pptxArtifactMaxTextFreePlateAttempts = 2

type pptxArtifactImageTextDetector func(ctx context.Context, imagePath string) (text string, checked bool, err error)

var detectPPTXArtifactImageText pptxArtifactImageTextDetector = detectPPTXArtifactImageTextDefault
var pptxArtifactDesignPlanTimeout = 45 * time.Second
var pptxArtifactTextFreePlateTimeout = 90 * time.Second
var runPPTXArtifactStructureReview = validatePPTXArtifactStructure

type pptxArtifactLockedProgressEmitter struct {
	mu    *sync.Mutex
	inner engine.ProgressEmitter
}

func (e pptxArtifactLockedProgressEmitter) Emit(ctx context.Context, event engine.ProgressEvent) {
	if e.inner == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inner.Emit(ctx, event)
}

func SetPPTXArtifactDesignPlanTimeoutForTesting(d time.Duration) (restore func()) {
	previous := pptxArtifactDesignPlanTimeout
	pptxArtifactDesignPlanTimeout = d
	return func() { pptxArtifactDesignPlanTimeout = previous }
}

func SetPPTXArtifactTextFreePlateTimeoutForTesting(d time.Duration) (restore func()) {
	previous := pptxArtifactTextFreePlateTimeout
	pptxArtifactTextFreePlateTimeout = d
	return func() { pptxArtifactTextFreePlateTimeout = previous }
}

type pptxArtifactWorkerRequest struct {
	Title               string                    `json:"title"`
	StylePreset         string                    `json:"stylePreset"`
	Theme               *officegen.SlideTheme     `json:"theme,omitempty"`
	Slides              []officegen.Slide         `json:"slides"`
	ReferenceFiles      []string                  `json:"referenceFiles,omitempty"`
	VisualAssets        []pptxArtifactVisualAsset `json:"visualAssets,omitempty"`
	StyleBrief          *PPTXReferenceStyleBrief  `json:"styleBrief,omitempty"`
	DesignPlan          *pptxArtifactDesignPlan   `json:"designPlan,omitempty"`
	StrictVisualQuality bool                      `json:"strictVisualQuality,omitempty"`
	RepairMode          string                    `json:"repairMode,omitempty"`
	OutputPPTX          string                    `json:"outputPptx"`
	PreviewDir          string                    `json:"previewDir"`
	InspectPath         string                    `json:"inspectPath"`
}

type pptxArtifactWorkerOutput struct {
	OutputPPTX     string   `json:"outputPptx"`
	PreviewFiles   []string `json:"previewFiles,omitempty"`
	InspectPath    string   `json:"inspectPath,omitempty"`
	WorkerDir      string   `json:"workerDir,omitempty"`
	ScriptPath     string   `json:"scriptPath,omitempty"`
	RequestPath    string   `json:"requestPath,omitempty"`
	ResponsePath   string   `json:"responsePath,omitempty"`
	ImportedRefs   int      `json:"importedRefs,omitempty"`
	EditableItems  int      `json:"editableItems,omitempty"`
	NativeCharts   int      `json:"nativeCharts,omitempty"`
	VisualAssets   int      `json:"visualAssets,omitempty"`
	VisualVerdict  string   `json:"visualVerdict,omitempty"`
	VisualScore    int      `json:"visualScore,omitempty"`
	VisualIssues   []string `json:"visualIssues,omitempty"`
	PreviewIssues  []string `json:"previewIssues,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	WorkerVersion  string   `json:"workerVersion,omitempty"`
	ArtifactToolOK bool     `json:"artifactToolOk,omitempty"`

	PreviewReview *PPTXArtifactPreviewReviewResult `json:"previewReview,omitempty"`
}

type pptxArtifactVisualAsset struct {
	Path              string                   `json:"path"`
	Name              string                   `json:"name,omitempty"`
	MIME              string                   `json:"mime,omitempty"`
	Slide             int                      `json:"slide,omitempty"`
	Frame             *pptxArtifactAssetFrame  `json:"frame,omitempty"`
	TargetAspectRatio float64                  `json:"targetAspectRatio,omitempty"`
	SourceAspectRatio float64                  `json:"sourceAspectRatio,omitempty"`
	TextDetection     *pptxArtifactTextCheck   `json:"textDetection,omitempty"`
	VisualSignal      *pptxArtifactImageSignal `json:"visualSignal,omitempty"`
	Width             int                      `json:"width,omitempty"`
	Height            int                      `json:"height,omitempty"`
	SizeBytes         int64                    `json:"sizeBytes,omitempty"`
}

type pptxArtifactTextCheck struct {
	Checked  bool   `json:"checked"`
	Status   string `json:"status,omitempty"`
	Attempts int    `json:"attempts,omitempty"`
}

type pptxArtifactImageSignal struct {
	Status      string  `json:"status,omitempty"`
	LumaRange   float64 `json:"lumaRange,omitempty"`
	LumaStdDev  float64 `json:"lumaStdDev,omitempty"`
	SampleCount int     `json:"sampleCount,omitempty"`
}

type pptxArtifactPreviewSignal struct {
	Path           string  `json:"path,omitempty"`
	Slide          int     `json:"slide,omitempty"`
	Width          int     `json:"width,omitempty"`
	Height         int     `json:"height,omitempty"`
	MeanLuma       float64 `json:"meanLuma,omitempty"`
	LumaRange      float64 `json:"lumaRange,omitempty"`
	LumaStdDev     float64 `json:"lumaStdDev,omitempty"`
	DistinctColors int     `json:"distinctColors,omitempty"`
	OpaqueSamples  int     `json:"opaqueSamples,omitempty"`
	SampleCount    int     `json:"sampleCount,omitempty"`
}

type pptxArtifactAssetFrame struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type pptxArtifactDesignPlan struct {
	DeckIntent    string                        `json:"deckIntent,omitempty"`
	StyleBias     string                        `json:"styleBias,omitempty"`
	BuilderRecipe string                        `json:"builderRecipe,omitempty"`
	BuilderPatch  *pptxArtifactBuilderPatch     `json:"builderPatch,omitempty"`
	Slides        []pptxArtifactSlideDesignPlan `json:"slides,omitempty"`
}

type pptxArtifactBuilderPatch struct {
	Slides []pptxArtifactBuilderSlidePatch `json:"slides,omitempty"`
}

type pptxArtifactBuilderSlidePatch struct {
	Slide      int    `json:"slide"`
	AccentRail string `json:"accentRail,omitempty"`
	Backplate  string `json:"backplate,omitempty"`
}

type pptxArtifactSlideDesignPlan struct {
	Slide           int                    `json:"slide"`
	Role            string                 `json:"role,omitempty"`
	LayoutMode      string                 `json:"layoutMode,omitempty"`
	Composition     string                 `json:"composition,omitempty"`
	VisualTreatment string                 `json:"visualTreatment,omitempty"`
	DensityTarget   string                 `json:"densityTarget,omitempty"`
	Kicker          string                 `json:"kicker,omitempty"`
	DisplayTitle    string                 `json:"displayTitle,omitempty"`
	DisplaySubtitle string                 `json:"displaySubtitle,omitempty"`
	DisplayBody     string                 `json:"displayBody,omitempty"`
	Takeaway        string                 `json:"takeaway,omitempty"`
	VisualIntent    string                 `json:"visualIntent,omitempty"`
	Cards           []pptxArtifactPlanCard `json:"cards,omitempty"`
	ChartCallouts   []pptxArtifactPlanCard `json:"chartCallouts,omitempty"`
}

type pptxArtifactPlanCard struct {
	Heading string `json:"heading,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type pptxArtifactWorkerRunner func(ctx context.Context, request pptxArtifactWorkerRequest, workDir string) (*pptxArtifactWorkerOutput, error)

var runPPTXArtifactWorker pptxArtifactWorkerRunner = runPPTXArtifactWorkerDefault

func buildPPTXWithArtifactWorker(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, payload pptxPayload, fallback string, enableImages, localPreview bool, options PPTXBuildOptions) ([]byte, string, []engine.GenerateIssue, []byte, []byte, error) {
	workDir, err := os.MkdirTemp("", "officecli-pptx-artifact-*")
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("create artifact worker directory: %w", err)
	}
	previewDir := filepath.Join(workDir, "preview")
	if err := os.MkdirAll(previewDir, 0o755); err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("create artifact worker preview directory: %w", err)
	}
	designPlan, planWarnings := resolvePPTXArtifactDesignPlan(ctx, llm, progress, payload, fallback, options)
	title := pptxArtifactDeckTitle(payload, fallback, designPlan)
	visualAssets := representativeVisualAssetsForDesignPlan(options, enableImages, designPlan)
	generatedVisualAssets, visualAssetWarnings := generatePPTXArtifactTextFreeVisualAssets(ctx, llm, progress, workDir, title, designPlan, options, enableImages)
	if len(generatedVisualAssets) > 0 {
		visualAssets = append(generatedVisualAssets, visualAssets...)
	}
	visualAssets = dedupePPTXArtifactSlideBoundVisualAssets(visualAssets)
	request := pptxArtifactWorkerRequest{
		Title:               title,
		StylePreset:         pptxArtifactWorkerStylePreset(payload, designPlan, options),
		Theme:               payload.Theme,
		Slides:              append([]officegen.Slide(nil), payload.Slides...),
		ReferenceFiles:      representativeReferenceFiles(options),
		VisualAssets:        visualAssets,
		StyleBrief:          options.ReferenceBrief,
		DesignPlan:          designPlan,
		StrictVisualQuality: options.GenerateArtifactDesignPlan,
		OutputPPTX:          filepath.Join(workDir, "output.pptx"),
		PreviewDir:          previewDir,
		InspectPath:         filepath.Join(workDir, "inspect.json"),
	}
	emitProgress(ctx, progress, progressStepAssemble, "running", "Running artifact experimental PPTX worker")
	workerOutput, _, fileBytes, finalRequest, attempts, repairWarnings, err := runPPTXArtifactWorkerWithRepairs(ctx, llm, progress, request, workDir, payload, fallback, options)
	if err != nil {
		return nil, "", nil, nil, nil, err
	}
	request = finalRequest
	warnings := append([]engine.GenerateIssue(nil), planWarnings...)
	warnings = append(warnings, visualAssetWarnings...)
	warnings = append(warnings, repairWarnings...)
	if attempts > 1 && request.RepairMode != "polish" {
		warnings = append(warnings, engine.GenerateIssue{
			Code:    "WARN_PPTX_ARTIFACT_RETRY",
			Field:   "pptx_backend",
			Message: fmt.Sprintf("Artifact experimental worker completed after %d preview/inspect repair attempts.", attempts),
		})
	}
	if workerOutput != nil {
		emitPPTXArtifactDebugMetadata(options, *workerOutput, request, attempts)
		for _, warning := range workerOutput.Warnings {
			if strings.TrimSpace(warning) == "" {
				continue
			}
			warnings = append(warnings, engine.GenerateIssue{
				Code:    "WARN_PPTX_ARTIFACT_WORKER",
				Field:   "pptx_backend",
				Message: strings.TrimSpace(warning),
			})
		}
	}
	var previewHTML []byte
	var previewJSON []byte
	if localPreview {
		previewMessages := make([]string, 0, len(warnings))
		for _, warning := range warnings {
			previewMessages = append(previewMessages, warning.Message)
		}
		previewJSON, _ = officegen.BuildLocalPreviewJSON(payload.Title, payload.StylePreset, payload.Theme, payload.Slides, previewMessages)
		previewHTML = officegen.BuildLocalPreviewHTML(payload.Title, payload.StylePreset, payload.Theme, payload.Slides, previewMessages)
	}
	return fileBytes, fmt.Sprintf("%s.pptx", generateengine.SanitizeFileName(title)), warnings, previewHTML, previewJSON, nil
}

func emitPPTXArtifactDebugMetadata(options PPTXBuildOptions, output pptxArtifactWorkerOutput, request pptxArtifactWorkerRequest, attempts int) {
	if options.ArtifactDebugSink == nil {
		return
	}
	previewFiles := append([]string(nil), output.PreviewFiles...)
	options.ArtifactDebugSink(PPTXArtifactDebugMetadata{
		Enabled:               true,
		Backend:               PPTXBackendArtifactWorker,
		WorkerVersion:         strings.TrimSpace(output.WorkerVersion),
		WorkerDir:             strings.TrimSpace(output.WorkerDir),
		WorkerScriptPath:      strings.TrimSpace(output.ScriptPath),
		RequestPath:           strings.TrimSpace(output.RequestPath),
		ResponsePath:          strings.TrimSpace(output.ResponsePath),
		InspectPath:           firstNonEmpty(output.InspectPath, request.InspectPath),
		PreviewFiles:          previewFiles,
		PreviewCount:          len(previewFiles),
		RepairMode:            strings.TrimSpace(request.RepairMode),
		Attempts:              attempts,
		ImportedRefs:          output.ImportedRefs,
		EditableItems:         output.EditableItems,
		NativeCharts:          output.NativeCharts,
		VisualAssets:          output.VisualAssets,
		VisualVerdict:         strings.TrimSpace(output.VisualVerdict),
		VisualScore:           output.VisualScore,
		QualitySummary:        buildPPTXArtifactQualitySummary(output, request, attempts),
		FinalOutputPPTX:       firstNonEmpty(output.OutputPPTX, request.OutputPPTX),
		NarrativePlanMarkdown: buildPPTXArtifactNarrativePlanMarkdown(request, output, attempts),
	})
}

func buildPPTXArtifactQualitySummary(output pptxArtifactWorkerOutput, request pptxArtifactWorkerRequest, attempts int) *PPTXArtifactQualitySummary {
	expectedCharts := 0
	for _, slide := range request.Slides {
		if slide.Chart != nil {
			expectedCharts++
		}
	}
	previewCount := len(output.PreviewFiles)
	editableOK := output.EditableItems > 0
	nativeChartOK := expectedCharts == 0 || output.NativeCharts >= expectedCharts
	previewOK := previewCount >= len(request.Slides)
	verdict := strings.TrimSpace(output.VisualVerdict)
	verdictOK := verdict == "" || strings.EqualFold(verdict, "pass") || strings.EqualFold(verdict, "ok")
	if output.VisualScore > 0 && output.VisualScore < 80 {
		verdictOK = false
	}
	issueFree := len(output.VisualIssues) == 0 && len(output.PreviewIssues) == 0
	var missing []string
	if !editableOK {
		missing = append(missing, "editable_text")
	}
	if !nativeChartOK {
		missing = append(missing, "native_chart")
	}
	if !previewOK {
		missing = append(missing, "preview_coverage")
	}
	if !verdictOK {
		missing = append(missing, "visual_verdict")
	}
	if len(output.VisualIssues) > 0 {
		missing = append(missing, "visual_issues")
	}
	if len(output.PreviewIssues) > 0 {
		missing = append(missing, "preview_issues")
	}
	gate := "pass"
	if len(missing) > 0 {
		gate = "fail"
	}
	return &PPTXArtifactQualitySummary{
		Backend:             PPTXBackendArtifactWorker,
		WorkerVersion:       strings.TrimSpace(output.WorkerVersion),
		SlideCount:          len(request.Slides),
		ExpectedCharts:      expectedCharts,
		PreviewCount:        previewCount,
		Attempts:            attempts,
		RepairMode:          strings.TrimSpace(request.RepairMode),
		EditableItems:       output.EditableItems,
		NativeCharts:        output.NativeCharts,
		VisualAssets:        output.VisualAssets,
		ImportedRefs:        output.ImportedRefs,
		VisualVerdict:       verdict,
		VisualScore:         output.VisualScore,
		VisualIssues:        append([]string(nil), output.VisualIssues...),
		PreviewIssues:       append([]string(nil), output.PreviewIssues...),
		EditableCoverageOK:  editableOK,
		NativeChartOK:       nativeChartOK,
		PreviewCoverageOK:   previewOK,
		VisualVerdictOK:     verdictOK,
		IssueFree:           issueFree,
		QualityGate:         gate,
		MissingRequirements: missing,
	}
}

func buildPPTXArtifactNarrativePlanMarkdown(request pptxArtifactWorkerRequest, output pptxArtifactWorkerOutput, attempts int) string {
	var b strings.Builder
	b.WriteString("# Narrative Plan\n\n")
	b.WriteString("## Audience\n")
	b.WriteString("Not specified by the request; optimize for a concise, reviewable presentation.\n\n")
	b.WriteString("## Objective\n")
	b.WriteString(fmt.Sprintf("Generate `%s` as an editable PPTX using the `%s` backend.\n\n", strings.TrimSpace(firstNonEmpty(request.Title, "Presentation")), PPTXBackendArtifactWorker))
	b.WriteString("## Narrative Arc\n")
	if request.DesignPlan != nil && len(request.DesignPlan.Slides) > 0 {
		for _, slide := range request.DesignPlan.Slides {
			title := firstNonEmpty(slide.DisplayTitle, slide.Takeaway, slide.Role, fmt.Sprintf("Slide %d", slide.Slide))
			b.WriteString(fmt.Sprintf("- Slide %d: %s", slide.Slide, strings.TrimSpace(title)))
			if strings.TrimSpace(slide.LayoutMode) != "" {
				b.WriteString(fmt.Sprintf(" (`%s`)", strings.TrimSpace(slide.LayoutMode)))
			}
			b.WriteString("\n")
		}
	} else {
		for idx, slide := range request.Slides {
			b.WriteString(fmt.Sprintf("- Slide %d: %s\n", idx+1, strings.TrimSpace(firstNonEmpty(slide.Title, slide.Layout, "Untitled"))))
		}
	}
	b.WriteString("\n## Source Plan\n")
	if len(request.ReferenceFiles) > 0 {
		for _, path := range request.ReferenceFiles {
			b.WriteString(fmt.Sprintf("- Import reference PPTX intent from `%s`.\n", strings.TrimSpace(path)))
		}
	} else {
		b.WriteString("- No reference deck is copied or imported as a template.\n")
	}
	if request.StyleBrief != nil {
		if strings.TrimSpace(request.StyleBrief.PaletteIntent) != "" {
			b.WriteString(fmt.Sprintf("- Palette intent: %s\n", strings.TrimSpace(request.StyleBrief.PaletteIntent)))
		}
		if strings.TrimSpace(request.StyleBrief.LayoutRhythm) != "" {
			b.WriteString(fmt.Sprintf("- Layout rhythm: %s\n", strings.TrimSpace(request.StyleBrief.LayoutRhythm)))
		}
		if len(request.StyleBrief.DoNotCopy) > 0 {
			b.WriteString("- Do not copy: " + strings.Join(request.StyleBrief.DoNotCopy, ", ") + "\n")
		}
	}
	b.WriteString("\n## Visual System\n")
	if request.DesignPlan != nil {
		b.WriteString(fmt.Sprintf("- Style bias: `%s`.\n", strings.TrimSpace(firstNonEmpty(request.DesignPlan.StyleBias, request.StylePreset, "default"))))
		b.WriteString(fmt.Sprintf("- Builder recipe: `%s`.\n", strings.TrimSpace(firstNonEmpty(request.DesignPlan.BuilderRecipe, "standard"))))
	} else {
		b.WriteString(fmt.Sprintf("- Style preset: `%s`.\n", strings.TrimSpace(firstNonEmpty(request.StylePreset, "default"))))
	}
	b.WriteString("- Use editable text, cards, rules, native chart objects, and motif shapes for the foreground.\n")
	b.WriteString("\n## Imagegen Plan\n")
	if len(request.VisualAssets) > 0 {
		b.WriteString("- Use selected text-free local visual plates only as supporting imagery.\n")
	} else if isPPTXArtifactReferenceLearningDesignPlan(request.DesignPlan) {
		b.WriteString("- Do not copy local images from reference/output directories; use editable native motifs unless a future imagegen plate pipeline supplies text-free art.\n")
	} else {
		b.WriteString("- No image plate assets were selected for this worker run.\n")
	}
	b.WriteString("\n## Asset Needs\n")
	b.WriteString(fmt.Sprintf("- Reference files imported: %d.\n", output.ImportedRefs))
	b.WriteString(fmt.Sprintf("- Visual assets embedded: %d.\n", output.VisualAssets))
	b.WriteString(fmt.Sprintf("- Render/inspect attempts: %d.\n", attempts))
	b.WriteString("\n## Visual Asset Plan\n")
	b.WriteString(buildPPTXArtifactVisualAssetPlanMarkdown(request))
	b.WriteString("\n## Editability Plan\n")
	b.WriteString("- Keep important slide words, labels, callouts, and section text editable.\n")
	b.WriteString("- Keep charts as native PowerPoint chart objects when chart content is present.\n")
	b.WriteString("- Keep implementation notes out of visible slide text; use this sidecar and inspect records for process details.\n")
	return b.String()
}

func buildPPTXArtifactVisualAssetPlanMarkdown(request pptxArtifactWorkerRequest) string {
	assetsBySlide := map[int][]pptxArtifactVisualAsset{}
	for _, asset := range request.VisualAssets {
		assetsBySlide[asset.Slide] = append(assetsBySlide[asset.Slide], asset)
	}
	var b strings.Builder
	if request.DesignPlan != nil && len(request.DesignPlan.Slides) > 0 {
		for _, slide := range request.DesignPlan.Slides {
			title := strings.TrimSpace(firstNonEmpty(slide.DisplayTitle, slide.Role, fmt.Sprintf("Slide %d", slide.Slide)))
			b.WriteString(fmt.Sprintf("- Slide %d `%s`: %s", slide.Slide, strings.TrimSpace(firstNonEmpty(slide.VisualTreatment, "native-shapes")), title))
			if strings.TrimSpace(slide.VisualIntent) != "" {
				b.WriteString("; intent: " + strings.TrimSpace(slide.VisualIntent))
			}
			b.WriteString(".\n")
			for _, asset := range assetsBySlide[slide.Slide] {
				b.WriteString("  - Asset `" + strings.TrimSpace(firstNonEmpty(asset.Name, filepath.Base(asset.Path), "visual-plate")) + "`")
				if asset.Frame != nil {
					b.WriteString("; frame " + formatPPTXArtifactFrame(*asset.Frame))
				}
				if asset.TargetAspectRatio > 0 {
					b.WriteString(fmt.Sprintf("; target ratio %.2f", asset.TargetAspectRatio))
				}
				if asset.SourceAspectRatio > 0 {
					b.WriteString(fmt.Sprintf("; source ratio %.2f", asset.SourceAspectRatio))
				}
				if asset.TextDetection != nil {
					b.WriteString("; OCR " + formatPPTXArtifactTextCheck(*asset.TextDetection))
				}
				b.WriteString(".\n")
			}
		}
		return b.String()
	}
	if len(request.VisualAssets) == 0 {
		return "- No visual assets were available; use native editable motifs.\n"
	}
	for _, asset := range request.VisualAssets {
		b.WriteString(fmt.Sprintf("- Slide %d asset `%s`", asset.Slide, strings.TrimSpace(firstNonEmpty(asset.Name, filepath.Base(asset.Path), "visual-plate"))))
		if asset.Frame != nil {
			b.WriteString("; frame " + formatPPTXArtifactFrame(*asset.Frame))
		}
		if asset.TextDetection != nil {
			b.WriteString("; OCR " + formatPPTXArtifactTextCheck(*asset.TextDetection))
		}
		b.WriteString(".\n")
	}
	return b.String()
}

func formatPPTXArtifactFrame(frame pptxArtifactAssetFrame) string {
	return fmt.Sprintf("left %.0f top %.0f width %.0f height %.0f", frame.Left, frame.Top, frame.Width, frame.Height)
}

func formatPPTXArtifactTextCheck(check pptxArtifactTextCheck) string {
	status := strings.TrimSpace(check.Status)
	if status == "" {
		if check.Checked {
			status = "checked"
		} else {
			status = "unchecked"
		}
	}
	if check.Attempts > 0 {
		suffix := ""
		if check.Attempts != 1 {
			suffix = "s"
		}
		return fmt.Sprintf("%s after %d attempt%s", status, check.Attempts, suffix)
	}
	return status
}

func pptxArtifactDeckTitle(payload pptxPayload, fallback string, designPlan *pptxArtifactDesignPlan) string {
	if designPlan != nil && designPlan.DeckIntent == "concise-reference-style-learning" {
		if title := pptxArtifactFallbackTopicTitle(fallback); title != "" {
			return title
		}
	}
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = firstNonEmpty(generateengine.ExtractTitleFromDescription(fallback), "Presentation")
	}
	return title
}

func pptxArtifactWorkerStylePreset(payload pptxPayload, designPlan *pptxArtifactDesignPlan, options PPTXBuildOptions) string {
	if strings.TrimSpace(options.RequestedStyle) != "" {
		return officegen.NormalizeStylePreset(options.RequestedStyle)
	}
	if designPlan != nil && designPlan.DeckIntent == "concise-reference-style-learning" {
		return officegen.StylePresetExecutiveDark
	}
	return officegen.NormalizeStylePreset(payload.StylePreset)
}

func pptxArtifactFallbackTopicTitle(fallback string) string {
	value := strings.TrimSpace(fallback)
	if value == "" || utf8.RuneCountInString(value) > 80 {
		return ""
	}
	normalized := strings.ToLower(value)
	for _, prefix := range []string{"create ", "generate ", "make ", "build ", "write ", "draft ", "produce "} {
		if strings.HasPrefix(normalized, prefix) {
			return ""
		}
	}
	if strings.Contains(normalized, "include a ") || strings.Contains(normalized, "include one ") || strings.Contains(normalized, " slides") || strings.Contains(normalized, " slide") {
		return ""
	}
	return value
}

func runPPTXArtifactWorkerWithRepairs(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, request pptxArtifactWorkerRequest, workDir string, payload pptxPayload, fallback string, options PPTXBuildOptions) (*pptxArtifactWorkerOutput, string, []byte, pptxArtifactWorkerRequest, int, []engine.GenerateIssue, error) {
	attempts := []struct {
		mode     string
		output   string
		preview  string
		inspect  string
		maxRunes int
		simplify bool
		progress string
	}{
		{mode: "", output: request.OutputPPTX, preview: request.PreviewDir, inspect: request.InspectPath, progress: "Running artifact experimental PPTX worker"},
		{mode: "simplified", output: filepath.Join(workDir, "output-retry-simplified.pptx"), preview: filepath.Join(workDir, "preview-retry-simplified"), inspect: filepath.Join(workDir, "inspect-retry-simplified.json"), maxRunes: 150, simplify: true, progress: "Retrying artifact experimental PPTX worker with simplified layout repair"},
		{mode: "minimal", output: filepath.Join(workDir, "output-retry-minimal.pptx"), preview: filepath.Join(workDir, "preview-retry-minimal"), inspect: filepath.Join(workDir, "inspect-retry-minimal.json"), maxRunes: 110, simplify: true, progress: "Retrying artifact experimental PPTX worker with minimal layout repair"},
	}
	var lastErr error
	var repairWarnings []engine.GenerateIssue
	workerCalls := 0
	for idx, attempt := range attempts {
		next := request
		next.RepairMode = attempt.mode
		next.OutputPPTX = attempt.output
		next.PreviewDir = attempt.preview
		next.InspectPath = attempt.inspect
		if attempt.simplify {
			next.Slides = preparePPTXArtifactRetrySlides(request.Slides, attempt.maxRunes, attempt.mode, request.DesignPlan)
		}
		if err := os.MkdirAll(next.PreviewDir, 0o755); err != nil {
			return nil, "", nil, next, workerCalls, repairWarnings, fmt.Errorf("create artifact worker preview directory: %w", err)
		}
		if idx > 0 {
			emitProgress(ctx, progress, progressStepAssemble, "running", attempt.progress)
		}
		workerCalls++
		workerOutput, outputPath, fileBytes, err := runAndValidatePPTXArtifactWorker(ctx, next, workDir)
		if err == nil {
			if attempt.mode == "" && shouldRunPPTXArtifactPolishPass(next, options) {
				polishedOutput, polishedPath, polishedBytes, polishedRequest, polishCalls, warnings, polishErr := runPPTXArtifactPolishPasses(ctx, llm, progress, next, workerOutput, workDir, payload, fallback, options)
				repairWarnings = append(repairWarnings, warnings...)
				workerCalls += polishCalls
				if polishErr != nil {
					return nil, "", nil, polishedRequest, workerCalls, repairWarnings, polishErr
				}
				if polishCalls > 0 {
					return polishedOutput, polishedPath, polishedBytes, polishedRequest, workerCalls, repairWarnings, nil
				}
			}
			return workerOutput, outputPath, fileBytes, next, workerCalls, repairWarnings, nil
		}
		lastErr = err
		if !isRetryablePPTXArtifactError(err) {
			return nil, "", nil, next, workerCalls, repairWarnings, fmt.Errorf("artifact experimental backend failed: %w", err)
		}
		if idx == 0 {
			assetRepaired, assetWarnings := repairPPTXArtifactVisualAssets(ctx, llm, progress, request, workDir, request.Title, options, err)
			repairWarnings = append(repairWarnings, assetWarnings...)
			if assetRepaired != nil {
				assetRepaired.RepairMode = "asset-repair"
				assetRepaired.OutputPPTX = filepath.Join(workDir, "output-retry-asset.pptx")
				assetRepaired.PreviewDir = filepath.Join(workDir, "preview-retry-asset")
				assetRepaired.InspectPath = filepath.Join(workDir, "inspect-retry-asset.json")
				if err := os.MkdirAll(assetRepaired.PreviewDir, 0o755); err != nil {
					return nil, "", nil, *assetRepaired, workerCalls, repairWarnings, fmt.Errorf("create artifact worker preview directory: %w", err)
				}
				emitProgress(ctx, progress, progressStepAssemble, "running", "Retrying artifact experimental PPTX worker with regenerated visual assets")
				workerCalls++
				workerOutput, outputPath, fileBytes, err := runAndValidatePPTXArtifactWorker(ctx, *assetRepaired, workDir)
				if err == nil {
					return workerOutput, outputPath, fileBytes, *assetRepaired, workerCalls, repairWarnings, nil
				}
				lastErr = err
				if !isRetryablePPTXArtifactError(err) {
					return nil, "", nil, *assetRepaired, workerCalls, repairWarnings, fmt.Errorf("artifact experimental backend failed: %w", err)
				}
			}
			repairedPlan, warnings := resolvePPTXArtifactRepairDesignPlan(ctx, llm, progress, payload, fallback, options, request.DesignPlan, err)
			repairWarnings = append(repairWarnings, warnings...)
			if repairedPlan != nil {
				repaired := request
				repaired.DesignPlan = repairedPlan
				repaired.RepairMode = "design-repair"
				repaired.OutputPPTX = filepath.Join(workDir, "output-retry-design.pptx")
				repaired.PreviewDir = filepath.Join(workDir, "preview-retry-design")
				repaired.InspectPath = filepath.Join(workDir, "inspect-retry-design.json")
				if err := os.MkdirAll(repaired.PreviewDir, 0o755); err != nil {
					return nil, "", nil, repaired, workerCalls, repairWarnings, fmt.Errorf("create artifact worker preview directory: %w", err)
				}
				emitProgress(ctx, progress, progressStepAssemble, "running", "Retrying artifact experimental PPTX worker with preview-informed design repair")
				workerCalls++
				workerOutput, outputPath, fileBytes, err := runAndValidatePPTXArtifactWorker(ctx, repaired, workDir)
				if err == nil {
					return workerOutput, outputPath, fileBytes, repaired, workerCalls, repairWarnings, nil
				}
				lastErr = err
				if !isRetryablePPTXArtifactError(err) {
					return nil, "", nil, repaired, workerCalls, repairWarnings, fmt.Errorf("artifact experimental backend failed: %w", err)
				}
			}
		}
	}
	return nil, "", nil, request, workerCalls, repairWarnings, fmt.Errorf("artifact experimental backend failed after %d preview/inspect repair attempts: %w", workerCalls, lastErr)
}

func shouldRunPPTXArtifactPolishPass(request pptxArtifactWorkerRequest, options PPTXBuildOptions) bool {
	if !options.GenerateArtifactDesignPlan {
		return false
	}
	if strings.TrimSpace(request.RepairMode) != "" {
		return false
	}
	return isPPTXArtifactReferenceLearningRequest(request)
}

func runPPTXArtifactPolishPasses(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, request pptxArtifactWorkerRequest, output *pptxArtifactWorkerOutput, workDir string, payload pptxPayload, fallback string, options PPTXBuildOptions) (*pptxArtifactWorkerOutput, string, []byte, pptxArtifactWorkerRequest, int, []engine.GenerateIssue, error) {
	currentRequest := request
	currentOutput := output
	var finalOutput *pptxArtifactWorkerOutput
	var finalPath string
	var finalBytes []byte
	var warnings []engine.GenerateIssue
	polishCalls := 0
	appliedDesignPlan := false
	for pass := 1; pass <= pptxArtifactMaxPolishPasses; pass++ {
		warnings = append(warnings, maybeReviewPPTXArtifactPreviews(ctx, progress, options, currentRequest, currentOutput)...)
		polishPlan, planWarnings := resolvePPTXArtifactPolishDesignPlan(ctx, llm, progress, payload, fallback, options, currentRequest.DesignPlan, currentOutput)
		warnings = append(warnings, planWarnings...)
		if polishPlan == nil {
			break
		}
		polished := currentRequest
		polished.DesignPlan = polishPlan
		appliedDesignPlan = true
		polished.RepairMode = "polish"
		polished.OutputPPTX = filepath.Join(workDir, pptxArtifactPolishOutputName(pass))
		polished.PreviewDir = filepath.Join(workDir, pptxArtifactPolishPreviewDirName(pass))
		polished.InspectPath = filepath.Join(workDir, pptxArtifactPolishInspectName(pass))
		if err := os.MkdirAll(polished.PreviewDir, 0o755); err != nil {
			return nil, "", nil, polished, polishCalls, warnings, fmt.Errorf("create artifact worker preview directory: %w", err)
		}
		emitProgress(ctx, progress, progressStepAssemble, "running", pptxArtifactPolishProgressMessage(pass))
		polishCalls++
		polishedOutput, polishedPath, polishedBytes, polishErr := runAndValidatePPTXArtifactWorker(ctx, polished, workDir)
		if polishErr != nil {
			return nil, "", nil, polished, polishCalls, warnings, fmt.Errorf("artifact experimental backend failed during preview-informed polish: %w", polishErr)
		}
		finalOutput = polishedOutput
		finalPath = polishedPath
		finalBytes = polishedBytes
		currentRequest = polished
		currentOutput = polishedOutput
	}
	if polishCalls == 0 {
		return nil, "", nil, request, 0, warnings, nil
	}
	if !appliedDesignPlan {
		warnings = removePPTXArtifactIssueCode(warnings, "WARN_PPTX_ARTIFACT_POLISH_DESIGN_APPLIED")
	} else {
		warnings = dedupePPTXArtifactIssueCode(warnings, "WARN_PPTX_ARTIFACT_POLISH_DESIGN_APPLIED")
	}
	warnings = append(warnings, engine.GenerateIssue{
		Code:    "WARN_PPTX_ARTIFACT_POLISH",
		Field:   "pptx_backend",
		Message: fmt.Sprintf("Artifact experimental worker applied %d preview-informed polish pass%s after the initial render passed validation.", polishCalls, pluralS(polishCalls)),
	})
	return finalOutput, finalPath, finalBytes, currentRequest, polishCalls, warnings, nil
}

func maybeReviewPPTXArtifactPreviews(ctx context.Context, progress engine.ProgressEmitter, options PPTXBuildOptions, request pptxArtifactWorkerRequest, output *pptxArtifactWorkerOutput) []engine.GenerateIssue {
	if options.ArtifactPreviewReviewer == nil || output == nil || output.PreviewReview != nil {
		return nil
	}
	previewFiles := append([]string(nil), output.PreviewFiles...)
	if len(previewFiles) == 0 {
		previewFiles = pptxArtifactInspectPreviewFiles(output.InspectPath)
	}
	if len(previewFiles) == 0 {
		return nil
	}
	emitProgress(ctx, progress, progressStepAssemble, "running", "Reviewing artifact experimental rendered previews")
	result, err := options.ArtifactPreviewReviewer.ReviewPPTXArtifactPreviews(ctx, PPTXArtifactPreviewReviewRequest{
		PreviewFiles: previewFiles,
		InspectPath:  strings.TrimSpace(output.InspectPath),
		OutputPPTX:   firstNonEmpty(output.OutputPPTX, request.OutputPPTX),
		SlideCount:   len(request.Slides),
	})
	if err != nil {
		return []engine.GenerateIssue{{
			Code:    "WARN_PPTX_ARTIFACT_PREVIEW_REVIEW_SKIPPED",
			Field:   "pptx_backend",
			Message: "Artifact experimental visual preview review was skipped: " + strings.TrimSpace(err.Error()),
		}}
	}
	if result == nil {
		return nil
	}
	normalized := normalizePPTXArtifactPreviewReviewResult(*result)
	output.PreviewReview = &normalized
	return []engine.GenerateIssue{{
		Code:    "WARN_PPTX_ARTIFACT_PREVIEW_REVIEW_APPLIED",
		Field:   "pptx_backend",
		Message: "Artifact experimental visual preview review was included in the preview-informed polish evidence.",
	}}
}

func pptxArtifactInspectPreviewFiles(inspectPath string) []string {
	inspectPath = strings.TrimSpace(inspectPath)
	if inspectPath == "" {
		return nil
	}
	data, err := os.ReadFile(inspectPath)
	if err != nil {
		return nil
	}
	var inspect pptxArtifactInspectSummary
	if err := json.Unmarshal(data, &inspect); err != nil {
		return nil
	}
	return append([]string(nil), inspect.Previews...)
}

func normalizePPTXArtifactPreviewReviewResult(result PPTXArtifactPreviewReviewResult) PPTXArtifactPreviewReviewResult {
	if result.Score < 0 {
		result.Score = 0
	}
	if result.Score > 100 {
		result.Score = 100
	}
	result.Summary = shortenLayoutText(strings.TrimSpace(result.Summary), 220)
	result.Strengths = limitStrings(trimPPTXArtifactStrings(result.Strengths), 4)
	issues := make([]PPTXArtifactPreviewReviewIssue, 0, artifactMinInt(len(result.Issues), 8))
	for _, issue := range result.Issues {
		code := strings.TrimSpace(issue.Code)
		title := shortenLayoutText(strings.TrimSpace(issue.Title), 60)
		message := shortenLayoutText(strings.TrimSpace(issue.Message), 180)
		suggestion := shortenLayoutText(strings.TrimSpace(issue.Suggestion), 180)
		if code == "" && title == "" && message == "" && suggestion == "" {
			continue
		}
		severity := strings.ToLower(strings.TrimSpace(issue.Severity))
		switch severity {
		case "high", "medium", "low":
		default:
			severity = "medium"
		}
		issues = append(issues, PPTXArtifactPreviewReviewIssue{
			Severity:     severity,
			Code:         firstNonEmpty(code, "VISUAL_PREVIEW_ISSUE"),
			Title:        title,
			Message:      message,
			SlideNumbers: normalizePPTXArtifactSlideNumbers(issue.SlideNumbers),
			Suggestion:   suggestion,
		})
		if len(issues) >= 8 {
			break
		}
	}
	result.Issues = issues
	return result
}

func trimPPTXArtifactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, shortenLayoutText(value, 140))
		}
	}
	return out
}

func normalizePPTXArtifactSlideNumbers(values []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
		if len(out) >= 6 {
			break
		}
	}
	sort.Ints(out)
	return out
}

func pptxArtifactPolishOutputName(pass int) string {
	if pass <= 1 {
		return "output-polish.pptx"
	}
	return fmt.Sprintf("output-polish-%d.pptx", pass)
}

func pptxArtifactPolishPreviewDirName(pass int) string {
	if pass <= 1 {
		return "preview-polish"
	}
	return fmt.Sprintf("preview-polish-%d", pass)
}

func pptxArtifactPolishInspectName(pass int) string {
	if pass <= 1 {
		return "inspect-polish.json"
	}
	return fmt.Sprintf("inspect-polish-%d.json", pass)
}

func pptxArtifactPolishProgressMessage(pass int) string {
	if pass <= 1 {
		return "Polishing artifact experimental PPTX with preview-informed layout pass"
	}
	return fmt.Sprintf("Polishing artifact experimental PPTX with preview-informed layout pass %d", pass)
}

func removePPTXArtifactIssueCode(issues []engine.GenerateIssue, code string) []engine.GenerateIssue {
	out := issues[:0]
	for _, issue := range issues {
		if issue.Code == code {
			continue
		}
		out = append(out, issue)
	}
	return out
}

func dedupePPTXArtifactIssueCode(issues []engine.GenerateIssue, code string) []engine.GenerateIssue {
	out := issues[:0]
	seen := false
	for _, issue := range issues {
		if issue.Code == code {
			if seen {
				continue
			}
			seen = true
		}
		out = append(out, issue)
	}
	return out
}

func pluralS(count int) string {
	if count == 1 {
		return ""
	}
	return "es"
}

func runAndValidatePPTXArtifactWorker(ctx context.Context, request pptxArtifactWorkerRequest, workDir string) (*pptxArtifactWorkerOutput, string, []byte, error) {
	workerOutput, err := runPPTXArtifactWorker(ctx, request, workDir)
	if err != nil {
		return nil, "", nil, err
	}
	outputPath := request.OutputPPTX
	if workerOutput != nil && strings.TrimSpace(workerOutput.OutputPPTX) != "" {
		outputPath = workerOutput.OutputPPTX
	}
	fileBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return workerOutput, outputPath, nil, fmt.Errorf("read artifact worker output: %w", err)
	}
	if len(fileBytes) == 0 {
		return workerOutput, outputPath, nil, fmt.Errorf("artifact worker output is empty")
	}
	if err := runPPTXArtifactStructureReview(ctx, outputPath); err != nil {
		return workerOutput, outputPath, nil, err
	}
	if err := validatePPTXArtifactDiagnostics(request, workerOutput); err != nil {
		return workerOutput, outputPath, nil, err
	}
	return workerOutput, outputPath, fileBytes, nil
}

type pptxArtifactInspectSummary struct {
	EditableItems []pptxArtifactEditableInspectItem `json:"editableItems"`
	VisualItems   []pptxArtifactVisualInspectItem   `json:"visualItems"`
	NativeCharts  []any                             `json:"nativeCharts"`
	Images        []pptxArtifactImageInspectItem    `json:"images"`
	Previews      []string                          `json:"previews"`
	VisualVerdict *pptxArtifactVisualVerdict        `json:"visualVerdict,omitempty"`
}

type pptxArtifactEditableInspectItem struct {
	Kind      string                  `json:"kind,omitempty"`
	Role      string                  `json:"role,omitempty"`
	Text      string                  `json:"text,omitempty"`
	Slide     int                     `json:"slide,omitempty"`
	FontSize  float64                 `json:"fontSize,omitempty"`
	TextChars int                     `json:"textChars,omitempty"`
	TextLines int                     `json:"textLines,omitempty"`
	BBox      pptxArtifactInspectBBox `json:"bbox,omitempty"`
}

type pptxArtifactInspectBBox struct {
	Left   float64 `json:"left,omitempty"`
	Top    float64 `json:"top,omitempty"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

type pptxArtifactVisualInspectItem struct {
	Kind  string                  `json:"kind,omitempty"`
	Role  string                  `json:"role,omitempty"`
	Slide int                     `json:"slide,omitempty"`
	BBox  pptxArtifactInspectBBox `json:"bbox,omitempty"`
}

type pptxArtifactImageInspectItem struct {
	Path   string                  `json:"path,omitempty"`
	Alt    string                  `json:"alt,omitempty"`
	Slide  int                     `json:"slide,omitempty"`
	Failed bool                    `json:"failed,omitempty"`
	BBox   pptxArtifactInspectBBox `json:"bbox,omitempty"`
}

type pptxArtifactVisualVerdict struct {
	Status string                           `json:"status,omitempty"`
	Score  int                              `json:"score,omitempty"`
	Issues []pptxArtifactVisualVerdictIssue `json:"issues,omitempty"`
}

type pptxArtifactVisualVerdictIssue struct {
	Code     string `json:"code,omitempty"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`
	Slide    int    `json:"slide,omitempty"`
}

func validatePPTXArtifactDiagnostics(request pptxArtifactWorkerRequest, output *pptxArtifactWorkerOutput) error {
	if output == nil {
		return fmt.Errorf("artifact worker did not return diagnostics")
	}
	if strings.TrimSpace(output.InspectPath) == "" {
		output.InspectPath = request.InspectPath
	}
	if strings.TrimSpace(output.InspectPath) == "" {
		return fmt.Errorf("artifact worker did not return an inspect JSON path")
	}
	inspectBytes, err := os.ReadFile(output.InspectPath)
	if err != nil {
		return fmt.Errorf("read artifact worker inspect JSON: %w", err)
	}
	var inspect pptxArtifactInspectSummary
	if err := json.Unmarshal(inspectBytes, &inspect); err != nil {
		return fmt.Errorf("parse artifact worker inspect JSON: %w", err)
	}
	editableCount := artifactMaxInt(output.EditableItems, len(inspect.EditableItems))
	if editableCount == 0 {
		return fmt.Errorf("artifact worker inspect found no editable text/items")
	}
	if err := validatePPTXArtifactVisibleText(inspect.EditableItems); err != nil {
		return err
	}
	if err := validatePPTXArtifactEditableTextLayout(inspect.EditableItems); err != nil {
		return err
	}
	enrichPPTXArtifactOutputVisualVerdict(output, inspect.VisualVerdict)
	if err := validatePPTXArtifactVisualVerdict(inspect.VisualVerdict); err != nil {
		return err
	}
	expectedCharts := 0
	for _, slide := range request.Slides {
		if slide.Chart != nil {
			expectedCharts++
		}
	}
	if expectedCharts > 0 {
		nativeChartCount := artifactMaxInt(output.NativeCharts, len(inspect.NativeCharts))
		if nativeChartCount < expectedCharts {
			return fmt.Errorf("artifact worker inspect found %d native charts, expected at least %d", nativeChartCount, expectedCharts)
		}
	}
	if err := validatePPTXArtifactVisualStructure(request, inspect.VisualItems, output.WorkerVersion); err != nil {
		return err
	}
	if err := validatePPTXArtifactReferenceLearningVisualCoverage(request, inspect.VisualItems, inspect.Images); err != nil {
		return err
	}
	previewFiles := append([]string(nil), output.PreviewFiles...)
	if len(previewFiles) == 0 {
		previewFiles = append(previewFiles, inspect.Previews...)
	}
	if len(previewFiles) < len(request.Slides) {
		return fmt.Errorf("artifact worker rendered %d preview images, expected at least %d", len(previewFiles), len(request.Slides))
	}
	previewIssues, err := validatePPTXArtifactPreviewImages(request, previewFiles[:len(request.Slides)])
	if len(previewIssues) > 0 {
		output.PreviewIssues = summarizePPTXArtifactVisualVerdictIssueStrings(previewIssues, 8)
	}
	if err != nil {
		return err
	}
	return nil
}

func enrichPPTXArtifactOutputVisualVerdict(output *pptxArtifactWorkerOutput, verdict *pptxArtifactVisualVerdict) {
	if output == nil || verdict == nil {
		return
	}
	if strings.TrimSpace(output.VisualVerdict) == "" {
		output.VisualVerdict = strings.TrimSpace(verdict.Status)
	}
	if output.VisualScore == 0 {
		output.VisualScore = verdict.Score
	}
	output.VisualIssues = summarizePPTXArtifactVisualVerdictIssueStrings(verdict.Issues, 8)
}

func validatePPTXArtifactVisualStructure(request pptxArtifactWorkerRequest, items []pptxArtifactVisualInspectItem, workerVersion string) error {
	if !isPPTXArtifactReferenceLearningRequest(request) || strings.TrimSpace(workerVersion) == "" {
		return nil
	}
	chartSlides := map[int]bool{}
	for idx, slide := range request.Slides {
		if slide.Chart != nil {
			chartSlides[idx+1] = true
		}
	}
	if len(chartSlides) == 0 {
		return nil
	}
	roleCounts := map[int]map[string]int{}
	for _, item := range items {
		if !chartSlides[item.Slide] {
			continue
		}
		if roleCounts[item.Slide] == nil {
			roleCounts[item.Slide] = map[string]int{}
		}
		roleCounts[item.Slide][item.Role]++
	}
	for slideNo := range chartSlides {
		counts := roleCounts[slideNo]
		if counts["chart-panel"] == 0 && counts["chart-panel-simple"] == 0 {
			return fmt.Errorf("reference-learning chart slide %d is missing a designed chart panel", slideNo)
		}
		if counts["chart-insight-card"]+counts["chart-compact-insight-card"] < 2 {
			return fmt.Errorf("reference-learning chart slide %d is missing designed insight cards", slideNo)
		}
	}
	return nil
}

func validatePPTXArtifactReferenceLearningVisualCoverage(request pptxArtifactWorkerRequest, items []pptxArtifactVisualInspectItem, images []pptxArtifactImageInspectItem) error {
	if !isPPTXArtifactReferenceLearningRequest(request) {
		return nil
	}
	required := map[int]string{}
	for _, slide := range request.DesignPlan.Slides {
		role := strings.TrimSpace(slide.Role)
		if slide.Slide <= 0 {
			continue
		}
		switch role {
		case "cover", "closing":
			required[slide.Slide] = role
		}
	}
	if len(required) == 0 {
		return nil
	}
	hasImage := map[int]bool{}
	for _, image := range images {
		if image.Slide > 0 && !image.Failed {
			hasImage[image.Slide] = true
		}
	}
	rolesBySlide := map[int]map[string]bool{}
	for _, item := range items {
		if item.Slide <= 0 {
			continue
		}
		if rolesBySlide[item.Slide] == nil {
			rolesBySlide[item.Slide] = map[string]bool{}
		}
		rolesBySlide[item.Slide][item.Role] = true
	}
	var issues []pptxArtifactVisualVerdictIssue
	for slideNo, role := range required {
		if hasImage[slideNo] || pptxArtifactHasNativeVisualFallback(role, rolesBySlide[slideNo]) {
			continue
		}
		issues = append(issues, pptxArtifactVisualVerdictIssue{
			Code:     "MISSING_REFERENCE_LEARNING_VISUAL",
			Severity: "error",
			Slide:    slideNo,
			Message:  "Reference-learning cover and closing slides need a visible text-free plate or deliberate native visual motif.",
		})
	}
	if len(issues) == 0 {
		return nil
	}
	return &pptxArtifactVisualVerdictError{
		status: "visual-coverage-fail",
		score:  artifactMaxInt(0, 90-len(issues)*16),
		issues: issues,
	}
}

func pptxArtifactHasNativeVisualFallback(role string, roles map[string]bool) bool {
	if len(roles) == 0 {
		return false
	}
	switch strings.TrimSpace(role) {
	case "cover":
		return roles["fallback-motif-signal-panel"] || roles["fallback-motif-diagonal-flow"]
	case "closing":
		return roles["closing-motif-frame"] || roles["closing-motif-rule"]
	default:
		return false
	}
}

func isPPTXArtifactReferenceLearningRequest(request pptxArtifactWorkerRequest) bool {
	if request.DesignPlan == nil {
		return false
	}
	return request.DesignPlan.DeckIntent == "concise-reference-style-learning" || request.DesignPlan.BuilderRecipe == "codex-reference-learning"
}

func validatePPTXArtifactVisualVerdict(verdict *pptxArtifactVisualVerdict) error {
	if verdict == nil {
		return nil
	}
	status := strings.ToLower(strings.TrimSpace(verdict.Status))
	if status == "" || status == "pass" || status == "ok" {
		if verdict.Score > 0 && verdict.Score < 80 {
			return &pptxArtifactVisualVerdictError{
				status: "low-score",
				score:  verdict.Score,
				issues: append([]pptxArtifactVisualVerdictIssue(nil), verdict.Issues...),
			}
		}
		return nil
	}
	return &pptxArtifactVisualVerdictError{
		status: status,
		score:  verdict.Score,
		issues: append([]pptxArtifactVisualVerdictIssue(nil), verdict.Issues...),
	}
}

type pptxArtifactVisualVerdictError struct {
	status string
	score  int
	issues []pptxArtifactVisualVerdictIssue
}

func (e *pptxArtifactVisualVerdictError) Error() string {
	if e == nil {
		return ""
	}
	summary := summarizePPTXArtifactVisualVerdictIssues(e.issues, 3)
	if strings.TrimSpace(summary) != "" {
		return fmt.Sprintf("artifact worker visual verdict is %s with score %d: %s", e.status, e.score, summary)
	}
	return fmt.Sprintf("artifact worker visual verdict is %s with score %d", e.status, e.score)
}

func validatePPTXArtifactPreviewImages(request pptxArtifactWorkerRequest, previewFiles []string) ([]pptxArtifactVisualVerdictIssue, error) {
	var issues []pptxArtifactVisualVerdictIssue
	for idx, previewPath := range previewFiles {
		signal, err := validatePPTXArtifactPreviewImage(previewPath)
		if err != nil {
			return issues, err
		}
		signal.Slide = idx + 1
		issues = append(issues, pptxArtifactPreviewQualityIssues(request, signal)...)
	}
	if len(issues) == 0 {
		return nil, nil
	}
	if isPPTXArtifactReferenceLearningRequest(request) {
		return issues, &pptxArtifactVisualVerdictError{
			status: "preview-fail",
			score:  artifactMaxInt(0, 92-len(issues)*14),
			issues: issues,
		}
	}
	return issues, nil
}

func validatePPTXArtifactPreviewImage(path string) (pptxArtifactPreviewSignal, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return pptxArtifactPreviewSignal{}, fmt.Errorf("artifact worker returned an empty preview path")
	}
	file, err := os.Open(path)
	if err != nil {
		return pptxArtifactPreviewSignal{}, fmt.Errorf("open artifact worker preview image %q: %w", path, err)
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return pptxArtifactPreviewSignal{}, fmt.Errorf("decode artifact worker preview image %q: %w", path, err)
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return pptxArtifactPreviewSignal{}, fmt.Errorf("artifact worker preview image %q has invalid dimensions %dx%d", path, width, height)
	}
	distinctColors := map[uint32]struct{}{}
	opaqueSamples := 0
	sampleCount := 0
	var sum float64
	var sumSq float64
	minLuma := math.Inf(1)
	maxLuma := math.Inf(-1)
	for y := bounds.Min.Y; y < bounds.Max.Y; y += artifactPreviewSampleStep(height) {
		for x := bounds.Min.X; x < bounds.Max.X; x += artifactPreviewSampleStep(width) {
			r, g, b, a := img.At(x, y).RGBA()
			if a > 0x7fff {
				opaqueSamples++
			}
			luma := 0.2126*float64(r)/257.0 + 0.7152*float64(g)/257.0 + 0.0722*float64(b)/257.0
			if luma < minLuma {
				minLuma = luma
			}
			if luma > maxLuma {
				maxLuma = luma
			}
			sum += luma
			sumSq += luma * luma
			sampleCount++
			key := uint32((r>>12)<<20 | (g>>12)<<12 | (b>>12)<<4 | (a >> 12))
			distinctColors[key] = struct{}{}
		}
	}
	mean := 0.0
	stdDev := 0.0
	lumaRange := 0.0
	if sampleCount > 0 {
		mean = sum / float64(sampleCount)
		variance := math.Max(0, sumSq/float64(sampleCount)-mean*mean)
		stdDev = math.Sqrt(variance)
		lumaRange = maxLuma - minLuma
	}
	signal := pptxArtifactPreviewSignal{
		Path:           path,
		Width:          width,
		Height:         height,
		MeanLuma:       mean,
		LumaRange:      lumaRange,
		LumaStdDev:     stdDev,
		DistinctColors: len(distinctColors),
		OpaqueSamples:  opaqueSamples,
		SampleCount:    sampleCount,
	}
	if opaqueSamples == 0 {
		return signal, fmt.Errorf("artifact worker preview image %q appears fully transparent", path)
	}
	if len(distinctColors) < 3 {
		return signal, fmt.Errorf("artifact worker preview image %q appears blank or single-color", path)
	}
	return signal, nil
}

func pptxArtifactPreviewQualityIssues(request pptxArtifactWorkerRequest, signal pptxArtifactPreviewSignal) []pptxArtifactVisualVerdictIssue {
	if signal.SampleCount == 0 {
		return nil
	}
	var issues []pptxArtifactVisualVerdictIssue
	if signal.LumaRange < 42 || signal.LumaStdDev < 10 {
		issues = append(issues, pptxArtifactPreviewIssue("PREVIEW_LOW_CONTRAST", signal, "Rendered preview has too little luminance variation; composition may read flat or unfinished."))
	}
	if signal.MeanLuma > 238 && signal.LumaStdDev < 18 {
		issues = append(issues, pptxArtifactPreviewIssue("PREVIEW_TOO_LIGHT", signal, "Rendered preview is too close to a blank light canvas for a high-fidelity reference-learning deck."))
	}
	if signal.MeanLuma < 18 && signal.LumaStdDev < 18 {
		issues = append(issues, pptxArtifactPreviewIssue("PREVIEW_TOO_DARK", signal, "Rendered preview is too dark and lacks enough visible structure."))
	}
	return issues
}

func pptxArtifactPreviewIssue(code string, signal pptxArtifactPreviewSignal, message string) pptxArtifactVisualVerdictIssue {
	return pptxArtifactVisualVerdictIssue{
		Code:     code,
		Severity: "error",
		Message:  message,
		Slide:    signal.Slide,
	}
}

func validatePPTXArtifactVisibleText(items []pptxArtifactEditableInspectItem) error {
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" || strings.EqualFold(strings.TrimSpace(item.Role), "footer") {
			continue
		}
		if pptxArtifactVisibleTextHasImplementationLeak(text) {
			return fmt.Errorf("artifact worker visible text contains implementation wording %q", text)
		}
		if pptxArtifactVisibleTextLooksDangling(text) {
			return fmt.Errorf("artifact worker visible text looks incomplete %q", text)
		}
	}
	return nil
}

func validatePPTXArtifactEditableTextLayout(items []pptxArtifactEditableInspectItem) error {
	bySlide := map[int][]pptxArtifactEditableInspectItem{}
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		role := strings.TrimSpace(item.Role)
		if text == "" || strings.EqualFold(role, "footer") {
			continue
		}
		if item.Slide <= 0 {
			return fmt.Errorf("artifact worker inspect text %q is missing slide number", text)
		}
		if item.BBox.Width <= 0 || item.BBox.Height <= 0 {
			return fmt.Errorf("artifact worker inspect text %q has invalid bbox", text)
		}
		if item.FontSize > 0 && item.FontSize < 11 {
			return fmt.Errorf("artifact worker inspect text %q has too-small font size %.1f", text, item.FontSize)
		}
		if item.TextChars > 0 && item.TextChars != len([]rune(item.Text)) {
			return fmt.Errorf("artifact worker inspect text %q has inconsistent textChars", text)
		}
		if item.TextLines <= 0 {
			return fmt.Errorf("artifact worker inspect text %q is missing textLines", text)
		}
		if artifactTextBBoxOutOfBounds(item.BBox) {
			return fmt.Errorf("artifact worker inspect text %q is outside slide bounds", text)
		}
		bySlide[item.Slide] = append(bySlide[item.Slide], item)
	}
	for slide, slideItems := range bySlide {
		if len(slideItems) > 14 {
			return fmt.Errorf("artifact worker inspect has too many text objects on slide %d: %d", slide, len(slideItems))
		}
		for i := 0; i < len(slideItems); i++ {
			if artifactTextItemTooNarrow(slideItems[i]) {
				return fmt.Errorf("artifact worker inspect text %q has too narrow text box", strings.TrimSpace(slideItems[i].Text))
			}
			for j := i + 1; j < len(slideItems); j++ {
				if artifactTextItemsMayOverlap(slideItems[i], slideItems[j]) {
					return fmt.Errorf("artifact worker inspect text boxes overlap on slide %d: %q and %q", slide, strings.TrimSpace(slideItems[i].Text), strings.TrimSpace(slideItems[j].Text))
				}
			}
		}
	}
	return nil
}

func artifactTextBBoxOutOfBounds(box pptxArtifactInspectBBox) bool {
	const slideWidth = 1280.0
	const slideHeight = 720.0
	if box.Left < 32 || box.Top < 32 {
		return true
	}
	if box.Left+box.Width > slideWidth-24 {
		return true
	}
	if box.Top+box.Height > slideHeight-24 {
		return true
	}
	return false
}

func artifactTextItemTooNarrow(item pptxArtifactEditableInspectItem) bool {
	if strings.EqualFold(item.Role, "footer") {
		return false
	}
	textLen := utf8.RuneCountInString(strings.TrimSpace(item.Text))
	if textLen <= 28 {
		return false
	}
	return item.BBox.Width < 170
}

func artifactTextItemsMayOverlap(left, right pptxArtifactEditableInspectItem) bool {
	if strings.EqualFold(left.Role, "footer") || strings.EqualFold(right.Role, "footer") {
		return false
	}
	intersection := artifactBBoxIntersectionArea(left.BBox, right.BBox)
	if intersection <= 0 {
		return false
	}
	smaller := artifactMinFloat(artifactBBoxArea(left.BBox), artifactBBoxArea(right.BBox))
	if smaller <= 0 {
		return false
	}
	return intersection/smaller > 0.08
}

func artifactBBoxIntersectionArea(left, right pptxArtifactInspectBBox) float64 {
	x1 := artifactMaxFloat(left.Left, right.Left)
	y1 := artifactMaxFloat(left.Top, right.Top)
	x2 := artifactMinFloat(left.Left+left.Width, right.Left+right.Width)
	y2 := artifactMinFloat(left.Top+left.Height, right.Top+right.Height)
	if x2 <= x1 || y2 <= y1 {
		return 0
	}
	return (x2 - x1) * (y2 - y1)
}

func artifactBBoxArea(box pptxArtifactInspectBBox) float64 {
	if box.Width <= 0 || box.Height <= 0 {
		return 0
	}
	return box.Width * box.Height
}

func artifactMinFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func artifactMaxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func pptxArtifactVisibleTextHasImplementationLeak(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	switch normalized {
	case "editable text", "native chart", "previewed":
		return false
	}
	for _, phrase := range []string{
		"native chart",
		"powerpoint chart object",
		"artifact worker",
		"artifact-tool",
		"imagegen",
		"generated with",
		"built with",
		"verified output",
		"preview gate",
		"hard gate",
		"editable title",
		"editable body",
		"editable text",
		"editable objects",
		"preview-checked",
		"no source images copied",
	} {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

func pptxArtifactVisibleTextLooksDangling(text string) bool {
	if isPPTXArtifactShortDisplayLabel(text) {
		return false
	}
	normalized := strings.ToLower(strings.Trim(strings.TrimSpace(text), ".。"))
	if normalized == "" {
		return false
	}
	words := strings.Fields(normalized)
	if pptxArtifactLooksCompleteShortHeading(words) {
		return false
	}
	if pptxArtifactTerminalConnector(words) {
		return true
	}
	if pptxArtifactEndsWithWeakTerminalVerb(words) {
		return true
	}
	if pptxArtifactEndsWithWeakTerminalAdjective(words) {
		return true
	}
	if len(words) >= 2 {
		prev := strings.ToLower(strings.Trim(words[len(words)-2], ".,;:，。；：()[]{}"))
		tail := strings.ToLower(strings.Trim(words[len(words)-1], ".,;:，。；：()[]{}"))
		if (prev == "stay" || prev == "stays") && tail == "clear" {
			return false
		}
		if isWeakTerminalVerb(prev) && (isWeakTerminalNoun(tail) || isWeakTerminalAdjective(tail)) {
			return true
		}
		if isWeakTerminalAdjective(prev) && isWeakTerminalNoun(tail) {
			return true
		}
		if isWeakTerminalNounPair(prev, tail) {
			return true
		}
	}
	if len(words) >= 3 {
		prev2 := strings.ToLower(strings.Trim(words[len(words)-3], ".,;:，。；：()[]{}"))
		prev1 := strings.ToLower(strings.Trim(words[len(words)-2], ".,;:，。；：()[]{}"))
		tail := strings.ToLower(strings.Trim(words[len(words)-1], ".,;:，。；：()[]{}"))
		if prev2 == "until" && (prev1 == "each" || prev1 == "every" || prev1 == "any") {
			return true
		}
		if isWeakTerminalVerb(prev2) && (prev1 == "the" || prev1 == "a" || prev1 == "an") && (isWeakTerminalNoun(tail) || isWeakTerminalAdjective(tail)) {
			return true
		}
	}
	if len(words) <= 5 && strings.HasSuffix(normalized, " is") {
		return true
	}
	if len(words) <= 8 && pptxArtifactVisibleTextHasWeakRankingPhrase(normalized) {
		return true
	}
	if strings.HasSuffix(normalized, " one to two") || strings.HasSuffix(normalized, " 1 to 2") {
		return true
	}
	for _, prefix := range []string{"are ", "is ", "was ", "were ", "should ", "must ", "need ", "needs ", "required ", "requires "} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func pptxArtifactLooksCompleteShortHeading(words []string) bool {
	if len(words) < 2 || len(words) > 3 {
		return false
	}
	for _, word := range words {
		clean := strings.ToLower(strings.Trim(word, ".,;:，。；：()[]{}"))
		if clean == "" || clean == "the" || clean == "a" || clean == "an" || isWeakTerminalVerb(clean) {
			return false
		}
	}
	tail := strings.ToLower(strings.Trim(words[len(words)-1], ".,;:，。；：()[]{}"))
	if !isWeakTerminalNoun(tail) {
		return false
	}
	return true
}

func isPPTXArtifactShortDisplayLabel(text string) bool {
	value := strings.TrimSpace(text)
	if value == "" || utf8.RuneCountInString(value) > 32 {
		return false
	}
	words := strings.Fields(value)
	if len(words) == 0 || len(words) > 3 {
		return false
	}
	hasLetter := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			return false
		}
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
		}
	}
	return hasLetter
}

func pptxArtifactTerminalConnector(words []string) bool {
	if len(words) == 0 {
		return false
	}
	tail := strings.ToLower(strings.Trim(words[len(words)-1], ".,;:，。；：()[]{}"))
	switch tail {
	case "and", "or", "but", "with", "without", "for", "from", "to", "by", "of", "in", "on", "at", "as", "the", "a", "an", "any", "every", "while", "because", "although", "though", "before", "after", "then", "if", "when", "where", "whether", "which", "that", "is", "are", "was", "were", "through", "into", "instead", "more", "less":
		return len(words) > 1
	default:
		return pptxArtifactEndsWithArticleAdjective(words) || pptxArtifactEndsWithWeakAdjectiveFragment(words, tail)
	}
}

func pptxArtifactEndsWithArticleAdjective(words []string) bool {
	if len(words) < 2 {
		return false
	}
	prev := strings.ToLower(strings.Trim(words[len(words)-2], ".,;:，。；：()[]{}"))
	if prev != "a" && prev != "an" && prev != "the" {
		return false
	}
	tail := strings.ToLower(strings.Trim(words[len(words)-1], ".,;:，。；：()[]{}"))
	switch tail {
	case "clear", "concise", "strong", "simple", "consistent", "recurring", "readable", "structured", "fresh", "quiet", "clean", "repeatable", "derived":
		return true
	default:
		return false
	}
}

func pptxArtifactEndsWithWeakAdjectiveFragment(words []string, tail string) bool {
	if len(words) <= 4 || !isWeakTerminalAdjective(tail) {
		return false
	}
	prev := strings.ToLower(strings.Trim(words[len(words)-2], ".,;:，。；：()[]{}"))
	if tail == "clear" {
		switch prev {
		case "stay", "stays", "message", "labels":
			return false
		}
	}
	return true
}

func pptxArtifactEndsWithWeakTerminalVerb(words []string) bool {
	if len(words) < 4 {
		return false
	}
	tail := strings.ToLower(strings.Trim(words[len(words)-1], ".,;:，。；：()[]{}"))
	switch tail {
	case "use", "uses", "used", "using", "support", "supports", "supported", "supporting", "help", "helps", "helped", "helping", "feel", "feels", "felt", "feeling", "carry", "carries", "carried", "carrying", "keep", "keeps", "kept", "keeping", "stay", "stays", "stayed", "staying", "make", "makes", "made", "making", "create", "creates", "created", "creating", "copy", "copies", "copied", "copying", "strengthen", "strengthens", "strengthened", "strengthening", "improve", "improves", "improved", "improving", "build", "builds", "built", "building":
		return true
	default:
		return false
	}
}

func pptxArtifactEndsWithWeakTerminalAdjective(words []string) bool {
	if len(words) < 4 {
		return false
	}
	tail := strings.ToLower(strings.Trim(words[len(words)-1], ".,;:，。；：()[]{}"))
	switch tail {
	case "selective", "main":
		return true
	default:
		return false
	}
}

func pptxArtifactVisibleTextHasWeakRankingPhrase(normalized string) bool {
	for _, phrase := range []string{" is highest", " is lowest", " is the highest", " is the lowest"} {
		if strings.HasSuffix(normalized, phrase) || strings.Contains(normalized, phrase+" at ") {
			return true
		}
	}
	return false
}

func artifactPreviewSampleStep(size int) int {
	if size <= 32 {
		return 1
	}
	step := size / 32
	if step < 1 {
		return 1
	}
	return step
}

func validatePPTXArtifactStructure(ctx context.Context, outputPath string) error {
	result, err := reviewprovider.NewService(nil, nil, nil).Review(ctx, reviewprovider.Request{
		FilePath:     outputPath,
		DocumentType: string(engine.DocumentTypePPTX),
		EnableVisual: false,
		RuntimeMode:  "external",
	})
	if err != nil {
		return fmt.Errorf("artifact worker structural review failed: %w", err)
	}
	if result.StructureScore >= pptxArtifactMinimumStructureScore {
		return nil
	}
	issueSummary := summarizePPTXArtifactReviewIssues(result.Issues, 3)
	return &pptxArtifactStructureError{
		score:     result.StructureScore,
		threshold: pptxArtifactMinimumStructureScore,
		issues:    append([]reviewprovider.Issue(nil), result.Issues...),
		summary:   issueSummary,
	}
}

type pptxArtifactStructureError struct {
	score     int
	threshold int
	issues    []reviewprovider.Issue
	summary   string
}

func (e *pptxArtifactStructureError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.summary) != "" {
		return fmt.Sprintf("artifact worker structural review score is %d, below required threshold %d: %s", e.score, e.threshold, e.summary)
	}
	return fmt.Sprintf("artifact worker structural review score is %d, below required threshold %d", e.score, e.threshold)
}

func isRetryablePPTXArtifactStructureError(err error) bool {
	var structureErr *pptxArtifactStructureError
	if !errors.As(err, &structureErr) || structureErr == nil || len(structureErr.issues) == 0 {
		return false
	}
	for _, issue := range structureErr.issues {
		if strings.TrimSpace(issue.Code) != "TEXT_DENSITY_HIGH" {
			return false
		}
	}
	return true
}

func isRetryablePPTXArtifactError(err error) bool {
	if isRetryablePPTXArtifactStructureError(err) {
		return true
	}
	var visualVerdictErr *pptxArtifactVisualVerdictError
	if errors.As(err, &visualVerdictErr) {
		return true
	}
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, phrase := range []string{
		"text boxes overlap",
		"outside slide bounds",
		"too-small font",
		"too many text objects",
		"too narrow text box",
		"visible text looks incomplete",
	} {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

func repairPPTXArtifactVisualAssets(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, request pptxArtifactWorkerRequest, workDir, title string, options PPTXBuildOptions, failure error) (*pptxArtifactWorkerRequest, []engine.GenerateIssue) {
	if llm == nil || request.DesignPlan == nil || !isPPTXArtifactReferenceLearningRequest(request) {
		return nil, nil
	}
	slides := pptxArtifactVisualAssetRepairSlides(failure)
	if len(slides) == 0 {
		return nil, nil
	}
	guidanceBySlide := pptxArtifactVisualAssetRepairGuidanceBySlide(failure)
	plateDir := filepath.Join(workDir, "artifact-text-free-plates-repair")
	if err := os.MkdirAll(plateDir, 0o755); err != nil {
		return nil, []engine.GenerateIssue{pptxArtifactTextFreePlateWarning("could not create repair plate directory: " + err.Error())}
	}
	plansBySlide := map[int]pptxArtifactSlideDesignPlan{}
	for _, slide := range request.DesignPlan.Slides {
		if slide.Slide > 0 {
			plansBySlide[slide.Slide] = slide
		}
	}
	replacements := map[int]pptxArtifactVisualAsset{}
	var warnings []engine.GenerateIssue
	for idx, slideNo := range slides {
		slidePlan, ok := plansBySlide[slideNo]
		if !ok {
			continue
		}
		if guidance := strings.TrimSpace(guidanceBySlide[slideNo]); guidance != "" {
			slidePlan.VisualIntent = strings.TrimSpace(firstNonEmpty(slidePlan.VisualIntent, slidePlan.DisplayTitle, slidePlan.Role) + ". Repair correction: " + guidance)
		}
		asset, warning := generatePPTXArtifactTextFreeVisualAsset(ctx, llm, progress, plateDir, title, request.DesignPlan, slidePlan, options, idx+1, len(slides))
		if warning != nil {
			warnings = append(warnings, *warning)
		}
		if asset != nil {
			replacements[slideNo] = *asset
		}
	}
	if len(replacements) == 0 {
		return nil, warnings
	}
	next := request
	next.VisualAssets = replacePPTXArtifactVisualAssets(request.VisualAssets, replacements)
	warnings = append(warnings, engine.GenerateIssue{
		Code:    "WARN_PPTX_ARTIFACT_ASSET_REPAIR_APPLIED",
		Field:   "pptx_backend",
		Message: fmt.Sprintf("Artifact experimental worker regenerated %d weak text-free visual plate%s before rerendering.", len(replacements), pluralS(len(replacements))),
	})
	return &next, warnings
}

func pptxArtifactVisualAssetRepairGuidanceBySlide(err error) map[int]string {
	var visualErr *pptxArtifactVisualVerdictError
	if !errors.As(err, &visualErr) || visualErr == nil {
		return nil
	}
	out := map[int][]string{}
	for _, issue := range visualErr.issues {
		if issue.Slide <= 0 || !pptxArtifactVisualIssueCanRepairAsset(issue.Code) {
			continue
		}
		switch strings.TrimSpace(issue.Code) {
		case "LOW_INFORMATION_VISUAL_ASSET":
			out[issue.Slide] = append(out[issue.Slide], "increase luminance variation, visible texture, depth, and a non-text focal structure")
		case "VISUAL_ASSET_ASPECT_RATIO_MISMATCH":
			out[issue.Slide] = append(out[issue.Slide], "compose for the exact target frame ratio and avoid important detail near crop edges")
		case "LOW_RESOLUTION_VISUAL_ASSET":
			out[issue.Slide] = append(out[issue.Slide], "produce a high-resolution plate suitable for a large slide crop")
		case "VISUAL_ASSET_COVERAGE_TOO_LOW":
			out[issue.Slide] = append(out[issue.Slide], "make the visual strong enough to read at its planned frame size")
		case "MISSING_BOUND_VISUAL_ASSET":
			out[issue.Slide] = append(out[issue.Slide], "produce a replacement plate that can be embedded directly on this slide")
		case "MISSING_REFERENCE_LEARNING_VISUAL":
			out[issue.Slide] = append(out[issue.Slide], "produce a strong text-free editorial illustration plate for this reference-learning slide")
		}
	}
	joined := map[int]string{}
	for slide, parts := range out {
		joined[slide] = strings.Join(parts, "; ")
	}
	return joined
}

func pptxArtifactVisualAssetRepairSlides(err error) []int {
	var visualErr *pptxArtifactVisualVerdictError
	if !errors.As(err, &visualErr) || visualErr == nil {
		return nil
	}
	seen := map[int]bool{}
	var slides []int
	for _, issue := range visualErr.issues {
		if issue.Slide <= 0 || !pptxArtifactVisualIssueCanRepairAsset(issue.Code) || seen[issue.Slide] {
			continue
		}
		seen[issue.Slide] = true
		slides = append(slides, issue.Slide)
	}
	sort.Ints(slides)
	return slides
}

func pptxArtifactVisualIssueCanRepairAsset(code string) bool {
	switch strings.TrimSpace(code) {
	case "LOW_INFORMATION_VISUAL_ASSET", "VISUAL_ASSET_ASPECT_RATIO_MISMATCH", "LOW_RESOLUTION_VISUAL_ASSET", "VISUAL_ASSET_COVERAGE_TOO_LOW", "MISSING_BOUND_VISUAL_ASSET", "MISSING_REFERENCE_LEARNING_VISUAL":
		return true
	default:
		return false
	}
}

func replacePPTXArtifactVisualAssets(current []pptxArtifactVisualAsset, replacements map[int]pptxArtifactVisualAsset) []pptxArtifactVisualAsset {
	out := make([]pptxArtifactVisualAsset, 0, len(current)+len(replacements))
	used := map[int]bool{}
	for _, asset := range current {
		if replacement, ok := replacements[asset.Slide]; ok {
			if !used[asset.Slide] {
				out = append(out, replacement)
				used[asset.Slide] = true
			}
			continue
		}
		out = append(out, asset)
	}
	slides := make([]int, 0, len(replacements))
	for slide := range replacements {
		if !used[slide] {
			slides = append(slides, slide)
		}
	}
	sort.Ints(slides)
	for _, slide := range slides {
		out = append(out, replacements[slide])
	}
	return out
}

func simplifyPPTXArtifactRetrySlides(slides []officegen.Slide) []officegen.Slide {
	for idx := range slides {
		slides[idx] = simplifyPPTXArtifactRetrySlide(slides[idx])
	}
	return slides
}

func preparePPTXArtifactRetrySlides(slides []officegen.Slide, maxRunes int, mode string, designPlan *pptxArtifactDesignPlan) []officegen.Slide {
	referenceLearning := designPlan != nil && designPlan.DeckIntent == "concise-reference-style-learning"
	next := compactDeckTextDensity(append([]officegen.Slide(nil), slides...), maxRunes)
	next = simplifyPPTXArtifactRetrySlides(next)
	if referenceLearning {
		next = restorePPTXArtifactReferenceLearningRetrySlides(next)
	}
	if mode == "minimal" {
		next = minimizePPTXArtifactRetrySlides(next)
		if referenceLearning {
			next = restorePPTXArtifactReferenceLearningRetrySlides(next)
		}
	}
	return next
}

func restorePPTXArtifactReferenceLearningRetrySlides(slides []officegen.Slide) []officegen.Slide {
	if len(slides) < 4 {
		return slides
	}
	out := append([]officegen.Slide(nil), slides...)
	out[0].Subtitle = "Same prompt, reference style intent, and editable visual motifs."
	out[0].Points = []string{"Style cues", "Chart signal", "Readable flow"}

	out[1].Title = "What the Reference Directory Actually Teaches"
	out[1].Subtitle = "System over template."
	out[1].Content = "Recurring cues become a usable visual system."
	out[1].Points = nil
	out[1].Sections = []officegen.SlideSection{
		{Heading: "Repeatable style", Detail: "Use repeated panels, accent rules, and compact cards."},
		{Heading: "Structured content", Detail: "Keep words, labels, and chart callouts editable."},
		{Heading: "Readable hierarchy", Detail: "Keep contrast, spacing, and title scale consistent across slides."},
	}

	out[2].Title = "Fidelity Comes From Multiple Enforced Layers"
	out[2].Subtitle = ""
	out[2].Content = ""
	out[2].Points = []string{"Let repeated reference cues guide emphasis.", "Labels stay clear and restrained."}
	out[2].Layout = "chart"
	if out[2].Chart == nil {
		out[2].Chart = referenceSignalChart(nil)
	}
	out[2].Chart.Title = "High-fidelity contribution"
	out[2].Chart.Categories = []string{"Style Profile", "Layout Rhythm", "Editable Objects", "Readable Flow"}
	if len(out[2].Chart.Values) != 4 {
		out[2].Chart.Values = []float64{78, 86, 82, 80}
	}

	out[3].Title = "Reference Style Becomes a Reusable System"
	out[3].Subtitle = ""
	out[3].Content = "Carry palette, hierarchy, and spacing into one clear deck system."
	out[3].Points = nil
	out[3].Sections = []officegen.SlideSection{
		{Heading: "Apply the system", Detail: "Reuse the recurring palette, spacing, and card rhythm."},
		{Heading: "Keep it readable", Detail: "Prefer clear hierarchy over literal template mimicry."},
	}
	return out
}

func minimizePPTXArtifactRetrySlides(slides []officegen.Slide) []officegen.Slide {
	for idx := range slides {
		if len(slides[idx].Points) > 1 {
			slides[idx].Points = slides[idx].Points[:1]
		}
		if len(slides[idx].Sections) > 1 {
			slides[idx].Sections = slides[idx].Sections[:1]
		}
		if len(slides[idx].Metrics) > 1 {
			slides[idx].Metrics = slides[idx].Metrics[:1]
		}
		slides[idx].Subtitle = shortenLayoutText(slides[idx].Subtitle, 38)
		slides[idx].Content = shortenLayoutText(slides[idx].Content, 42)
		slides[idx].Title = shortenLayoutText(slides[idx].Title, 46)
	}
	return slides
}

func simplifyPPTXArtifactRetrySlide(slide officegen.Slide) officegen.Slide {
	keepThreeObservationCards := isPPTXArtifactReferenceObservationSlide(slide)
	pointLimit := 2
	sectionLimit := 2
	if keepThreeObservationCards {
		pointLimit = 3
		sectionLimit = 3
	}
	if len(slide.Points) > pointLimit && slide.Chart == nil {
		slide.Points = slide.Points[:pointLimit]
	}
	if len(slide.Points) > 2 && slide.Chart != nil {
		slide.Points = slide.Points[:2]
	}
	if len(slide.Sections) > sectionLimit {
		slide.Sections = slide.Sections[:sectionLimit]
	}
	if len(slide.Metrics) > 2 {
		slide.Metrics = slide.Metrics[:2]
	}
	for idx := range slide.Points {
		slide.Points[idx] = shortenLayoutText(slide.Points[idx], 34)
	}
	for idx := range slide.Sections {
		headingLimit := 22
		detailLimit := 42
		if keepThreeObservationCards {
			headingLimit = 26
			detailLimit = 72
		}
		slide.Sections[idx].Heading = shortenLayoutText(slide.Sections[idx].Heading, headingLimit)
		slide.Sections[idx].Detail = shortenLayoutText(slide.Sections[idx].Detail, detailLimit)
	}
	for idx := range slide.Metrics {
		slide.Metrics[idx].Label = shortenLayoutText(slide.Metrics[idx].Label, 22)
		slide.Metrics[idx].Note = shortenLayoutText(slide.Metrics[idx].Note, 36)
	}
	slide.Title = shortenLayoutText(slide.Title, 58)
	slide.Subtitle = shortenLayoutText(slide.Subtitle, 50)
	slide.Content = shortenLayoutText(slide.Content, 64)
	slide.Source = ""
	return slide
}

func isPPTXArtifactReferenceObservationSlide(slide officegen.Slide) bool {
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle + " " + slide.Content))
	if strings.Contains(text, "reference style signals") || strings.Contains(text, "reference directory teaches") || strings.Contains(text, "key observation") {
		return true
	}
	if len(slide.Sections) >= 3 {
		for _, section := range slide.Sections {
			heading := strings.ToLower(strings.TrimSpace(section.Heading))
			if strings.Contains(heading, "quality loop") || strings.Contains(heading, "structured content") {
				return true
			}
		}
	}
	return false
}

func summarizePPTXArtifactReviewIssues(issues []reviewprovider.Issue, limit int) string {
	if limit <= 0 || len(issues) == 0 {
		return ""
	}
	if len(issues) < limit {
		limit = len(issues)
	}
	parts := make([]string, 0, limit)
	for _, issue := range issues[:limit] {
		code := strings.TrimSpace(issue.Code)
		if code == "" {
			code = "UNKNOWN"
		}
		if len(issue.SlideNumbers) > 0 {
			parts = append(parts, fmt.Sprintf("%s on slide %d", code, issue.SlideNumbers[0]))
		} else {
			parts = append(parts, code)
		}
	}
	return strings.Join(parts, "; ")
}

func summarizePPTXArtifactVisualVerdictIssues(issues []pptxArtifactVisualVerdictIssue, limit int) string {
	if limit <= 0 || len(issues) == 0 {
		return ""
	}
	if len(issues) < limit {
		limit = len(issues)
	}
	parts := make([]string, 0, limit)
	for _, issue := range issues[:limit] {
		parts = append(parts, formatPPTXArtifactVisualVerdictIssue(issue))
	}
	return strings.Join(parts, "; ")
}

func summarizePPTXArtifactVisualVerdictIssueStrings(issues []pptxArtifactVisualVerdictIssue, limit int) []string {
	if limit <= 0 || len(issues) == 0 {
		return nil
	}
	if len(issues) < limit {
		limit = len(issues)
	}
	out := make([]string, 0, limit)
	for _, issue := range issues[:limit] {
		out = append(out, formatPPTXArtifactVisualVerdictIssue(issue))
	}
	return out
}

func formatPPTXArtifactVisualVerdictIssue(issue pptxArtifactVisualVerdictIssue) string {
	code := strings.TrimSpace(issue.Code)
	if code == "" {
		code = "VISUAL_VERDICT"
	}
	if issue.Slide > 0 {
		code = fmt.Sprintf("%s on slide %d", code, issue.Slide)
	}
	message := strings.TrimSpace(issue.Message)
	if message != "" {
		return fmt.Sprintf("%s: %s", code, shortenLayoutText(message, 160))
	}
	return code
}

func summarizePPTXArtifactVisualRepairGuidance(err error) string {
	var visualErr *pptxArtifactVisualVerdictError
	if !errors.As(err, &visualErr) || len(visualErr.issues) == 0 {
		return ""
	}
	return buildPPTXArtifactVisualIssueGuidance(visualErr.issues)
}

func buildPPTXArtifactVisualIssueGuidance(issues []pptxArtifactVisualVerdictIssue) string {
	if len(issues) == 0 {
		return ""
	}
	seen := map[string]bool{}
	var lines []string
	for _, issue := range issues {
		code := strings.TrimSpace(issue.Code)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		switch code {
		case "LOW_INFORMATION_VISUAL_ASSET":
			lines = append(lines, "- LOW_INFORMATION_VISUAL_ASSET: replace or regenerate the affected visual plate with stronger contrast, texture, depth, and non-text focal structure; do not keep a near-solid decorative filler.")
		case "VISUAL_ASSET_ASPECT_RATIO_MISMATCH":
			lines = append(lines, "- VISUAL_ASSET_ASPECT_RATIO_MISMATCH: choose a visual treatment and frame whose aspect ratio matches the source plate, or switch that slide to native editable motifs instead of unpredictable cropping.")
		case "LOW_RESOLUTION_VISUAL_ASSET":
			lines = append(lines, "- LOW_RESOLUTION_VISUAL_ASSET: avoid tiny source images for large visual plates; use native motifs or request a higher-resolution text-free plate.")
		case "VISUAL_ASSET_COVERAGE_TOO_LOW":
			lines = append(lines, "- VISUAL_ASSET_COVERAGE_TOO_LOW: enlarge the visual role or remove the asset and rely on deliberate native shapes; do not keep token-sized imagery.")
		case "MISSING_BOUND_VISUAL_ASSET":
			lines = append(lines, "- MISSING_BOUND_VISUAL_ASSET: either bind the asset to the intended slide or update the plan to a native-shape visual treatment.")
		case "MISSING_REFERENCE_LEARNING_VISUAL":
			lines = append(lines, "- MISSING_REFERENCE_LEARNING_VISUAL: add a text-free visual plate or deliberate native motif to cover and closing slides; do not leave key reference-learning pages as copy-only cards.")
		case "CONTENT_DENSITY_HIGH", "TOO_MUCH_SMALL_TEXT", "NARROW_LONG_TEXT":
			lines = append(lines, "- "+code+": reduce cards/callouts, shorten visible copy, and keep hierarchy readable without shrinking text.")
		case "LOW_VALUE_REFERENCE_LEARNING_COPY":
			lines = append(lines, "- LOW_VALUE_REFERENCE_LEARNING_COPY: rewrite generic reference-learning copy into a task-specific thesis about palette, hierarchy, editable structure, and readable reference-style behavior.")
		case "PREVIEW_LOW_CONTRAST":
			lines = append(lines, "- PREVIEW_LOW_CONTRAST: adjust builderPatch, backplates, accent rails, and visual treatment so the rendered preview has stronger foreground/background separation and visible structure.")
		case "PREVIEW_TOO_LIGHT":
			lines = append(lines, "- PREVIEW_TOO_LIGHT: use a stronger dark or structured canvas, richer panels, and visible accent rails; avoid sparse light pages that read like unfinished defaults.")
		case "PREVIEW_TOO_DARK":
			lines = append(lines, "- PREVIEW_TOO_DARK: add brighter panels, accent rails, and readable contrast so the preview does not collapse into a dark blank field.")
		}
	}
	return strings.Join(lines, "\n")
}

func artifactMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func artifactMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func representativeVisualAssets(options PPTXBuildOptions, enableImages bool) []pptxArtifactVisualAsset {
	return representativeVisualAssetsForDesignPlan(options, enableImages, nil)
}

func representativeVisualAssetsForDesignPlan(options PPTXBuildOptions, enableImages bool, designPlan *pptxArtifactDesignPlan) []pptxArtifactVisualAsset {
	if !enableImages {
		return nil
	}
	root := strings.TrimSpace(options.ReferenceScanRoot)
	if root == "" && options.ReferenceProfile != nil {
		root = strings.TrimSpace(options.ReferenceProfile.Root)
	}
	if root == "" {
		return nil
	}
	const limit = 8
	prompt := strings.TrimSpace(options.UserPrompt + " " + summarizePPTXArtifactDesignPlanForAssetRelevance(designPlan))
	if isPPTXArtifactReferenceLearningDesignPlan(designPlan) {
		assets, _ := discoverPPTXArtifactVisualAssets(root, 128)
		return bindPPTXArtifactReferenceLearningVisualAssets(assets, designPlan)
	}
	if prompt == "" {
		assets, _ := discoverPPTXArtifactVisualAssets(root, limit)
		return assets
	}
	assets, _ := discoverPPTXArtifactVisualAssets(root, 128)
	return selectPPTXArtifactVisualAssetsForPrompt(assets, limit, prompt)
}

func isPPTXArtifactReferenceLearningDesignPlan(designPlan *pptxArtifactDesignPlan) bool {
	if designPlan == nil {
		return false
	}
	return designPlan.DeckIntent == "concise-reference-style-learning" || designPlan.BuilderRecipe == "codex-reference-learning"
}

func bindPPTXArtifactReferenceLearningVisualAssets(assets []pptxArtifactVisualAsset, designPlan *pptxArtifactDesignPlan) []pptxArtifactVisualAsset {
	if len(assets) == 0 {
		return nil
	}
	plateSlides := pptxArtifactTextFreePlateSlidePlans(designPlan)
	if len(plateSlides) == 0 {
		return nil
	}
	out := make([]pptxArtifactVisualAsset, 0, len(plateSlides))
	used := make(map[string]bool, len(assets))
	for _, slidePlan := range plateSlides {
		asset, ok := pickPPTXArtifactReferenceLearningVisualAsset(assets, used, slidePlan)
		if !ok {
			continue
		}
		bound := enrichPPTXArtifactLocalVisualAsset(asset, slidePlan)
		out = append(out, bound)
		used[asset.Path] = true
	}
	return out
}

func pickPPTXArtifactReferenceLearningVisualAsset(assets []pptxArtifactVisualAsset, used map[string]bool, slidePlan pptxArtifactSlideDesignPlan) (pptxArtifactVisualAsset, bool) {
	role := strings.TrimSpace(slidePlan.Role)
	preferCover := role == "cover"
	var preferredMismatch *pptxArtifactVisualAsset
	for pass := 0; pass < 2; pass++ {
		var best *pptxArtifactVisualAsset
		bestCost := math.Inf(1)
		bestDrift := math.Inf(1)
		for _, asset := range assets {
			if used[asset.Path] {
				continue
			}
			isCover := artifactVisualAssetIsCover(asset.Path)
			if pass == 0 && preferCover != isCover {
				continue
			}
			drift := pptxArtifactVisualAssetRatioDrift(asset, pptxArtifactTextFreePlateAspectRatio(slidePlan))
			cost := drift + pptxArtifactVisualAssetBrightCanvasPenalty(asset.Path)
			if best == nil || cost < bestCost {
				candidate := asset
				best = &candidate
				bestCost = cost
				bestDrift = drift
			}
		}
		if best != nil {
			if pass == 0 && bestDrift > 0.32 {
				preferredMismatch = best
				continue
			}
			return *best, true
		}
	}
	if preferredMismatch != nil {
		return *preferredMismatch, true
	}
	if len(assets) == 0 {
		return pptxArtifactVisualAsset{}, false
	}
	return assets[0], true
}

func pptxArtifactVisualAssetRatioDrift(asset pptxArtifactVisualAsset, targetRatio float64) float64 {
	if targetRatio <= 0 {
		return 0
	}
	sourceRatio := asset.SourceAspectRatio
	if sourceRatio <= 0 {
		sourceRatio = pptxArtifactAspectRatio(asset.Width, asset.Height)
	}
	if sourceRatio <= 0 {
		return math.Inf(1)
	}
	return math.Abs(math.Log(sourceRatio / targetRatio))
}

func pptxArtifactVisualAssetBrightCanvasPenalty(path string) float64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return 0
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return 0
	}
	stepX := artifactVisualSampleStep(width)
	stepY := artifactVisualSampleStep(height)
	var samples, bright int
	var lumSum, satSum float64
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			if a16 < 0x4000 {
				continue
			}
			r := float64(r16 >> 8)
			g := float64(g16 >> 8)
			b := float64(b16 >> 8)
			lum := 0.2126*r + 0.7152*g + 0.0722*b
			if lum > 230 {
				bright++
			}
			maxRGB := artifactMaxFloat(r, artifactMaxFloat(g, b))
			minRGB := artifactMinFloat(r, artifactMinFloat(g, b))
			satSum += maxRGB - minRGB
			lumSum += lum
			samples++
		}
	}
	if samples == 0 {
		return 0
	}
	meanLum := lumSum / float64(samples)
	brightRatio := float64(bright) / float64(samples)
	meanSat := satSum / float64(samples)
	switch {
	case brightRatio > 0.55 && meanLum > 185:
		return 0.9
	case brightRatio > 0.40 && meanLum > 175 && meanSat < 35:
		return 0.55
	default:
		return 0
	}
}

func enrichPPTXArtifactLocalVisualAsset(asset pptxArtifactVisualAsset, slidePlan pptxArtifactSlideDesignPlan) pptxArtifactVisualAsset {
	asset.Slide = slidePlan.Slide
	asset.Frame = pptxArtifactTextFreePlateFrame(slidePlan)
	asset.TargetAspectRatio = pptxArtifactTextFreePlateAspectRatio(slidePlan)
	if asset.SourceAspectRatio <= 0 {
		asset.SourceAspectRatio = pptxArtifactAspectRatio(asset.Width, asset.Height)
	}
	if asset.Name == "" || strings.Contains(strings.ToLower(asset.Name), "reference") || strings.Contains(strings.ToLower(asset.Name), "cover") || strings.Contains(strings.ToLower(asset.Name), "image-") {
		asset.Name = fmt.Sprintf("local-visual-plate-slide-%02d%s", slidePlan.Slide, pptxArtifactImageExtension(asset.MIME))
	}
	if asset.VisualSignal == nil {
		if data, err := os.ReadFile(asset.Path); err == nil {
			asset.VisualSignal = pptxArtifactImageSignalFromBytes(data)
			if asset.SizeBytes <= 0 {
				asset.SizeBytes = int64(len(data))
			}
		}
	}
	if asset.TextDetection == nil {
		asset.TextDetection = &pptxArtifactTextCheck{Checked: false, Status: "local-reference-unchecked"}
	}
	return asset
}

func generatePPTXArtifactTextFreeVisualAssets(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, workDir, title string, designPlan *pptxArtifactDesignPlan, options PPTXBuildOptions, enableImages bool) ([]pptxArtifactVisualAsset, []engine.GenerateIssue) {
	if !enableImages || llm == nil || !isPPTXArtifactReferenceLearningDesignPlan(designPlan) {
		return nil, nil
	}
	plateDir := filepath.Join(workDir, "artifact-text-free-plates")
	if err := os.MkdirAll(plateDir, 0o755); err != nil {
		return nil, []engine.GenerateIssue{pptxArtifactTextFreePlateWarning("could not create plate directory: " + err.Error())}
	}
	plateSlides := pptxArtifactTextFreePlateSlidePlans(designPlan)
	if len(plateSlides) == 0 {
		return nil, nil
	}
	parallelism := pptxArtifactTextFreePlateParallelism(progress, len(plateSlides))
	if parallelism > 1 {
		return generatePPTXArtifactTextFreeVisualAssetsParallel(ctx, llm, progress, plateDir, title, designPlan, plateSlides, options, parallelism)
	}
	assets := make([]pptxArtifactVisualAsset, 0, len(plateSlides))
	var warnings []engine.GenerateIssue
	for idx, slidePlan := range plateSlides {
		asset, warning := generatePPTXArtifactTextFreeVisualAsset(ctx, llm, progress, plateDir, title, designPlan, slidePlan, options, idx+1, len(plateSlides))
		if warning != nil {
			warnings = append(warnings, *warning)
			continue
		}
		if asset != nil {
			assets = append(assets, *asset)
		}
	}
	return assets, warnings
}

func generatePPTXArtifactTextFreeVisualAssetsParallel(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, plateDir, title string, designPlan *pptxArtifactDesignPlan, plateSlides []pptxArtifactSlideDesignPlan, options PPTXBuildOptions, parallelism int) ([]pptxArtifactVisualAsset, []engine.GenerateIssue) {
	type result struct {
		asset   *pptxArtifactVisualAsset
		warning *engine.GenerateIssue
	}
	results := make([]result, len(plateSlides))
	if parallelism < 1 {
		parallelism = 1
	}
	if parallelism > len(plateSlides) {
		parallelism = len(plateSlides)
	}
	var sinkMu sync.Mutex
	safeOptions := options
	if options.CreditBalanceSink != nil {
		safeOptions.CreditBalanceSink = func(balance int) {
			sinkMu.Lock()
			defer sinkMu.Unlock()
			options.CreditBalanceSink(balance)
		}
	}
	if options.CreditChargedSink != nil {
		safeOptions.CreditChargedSink = func(charged int) {
			sinkMu.Lock()
			defer sinkMu.Unlock()
			options.CreditChargedSink(charged)
		}
	}
	safeProgress := progress
	if progress != nil {
		safeProgress = pptxArtifactLockedProgressEmitter{mu: &sinkMu, inner: progress}
	}
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup
	for idx, slidePlan := range plateSlides {
		idx, slidePlan := idx, slidePlan
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				warning := pptxArtifactTextFreePlateWarning(fmt.Sprintf("slide %d: %s", slidePlan.Slide, summarizeImageGenerationError(ctx.Err())))
				results[idx].warning = &warning
				return
			}
			asset, warning := generatePPTXArtifactTextFreeVisualAsset(ctx, llm, safeProgress, plateDir, title, designPlan, slidePlan, safeOptions, idx+1, len(plateSlides))
			results[idx] = result{asset: asset, warning: warning}
		}()
	}
	wg.Wait()
	assets := make([]pptxArtifactVisualAsset, 0, len(plateSlides))
	var warnings []engine.GenerateIssue
	for _, item := range results {
		if item.warning != nil {
			warnings = append(warnings, *item.warning)
			continue
		}
		if item.asset != nil {
			assets = append(assets, *item.asset)
		}
	}
	return assets, warnings
}

func pptxArtifactTextFreePlateParallelism(progress engine.ProgressEmitter, count int) int {
	if count <= 1 || progress == nil {
		return 1
	}
	value := strings.TrimSpace(os.Getenv("OFFICECLI_PPTX_ARTIFACT_PARALLEL_PLATES"))
	if value != "" {
		n, err := strconv.Atoi(value)
		if err == nil {
			if n < 1 {
				return 1
			}
			if n > count {
				return count
			}
			return n
		}
	}
	if count < 2 {
		return count
	}
	return 2
}

func generatePPTXArtifactTextFreeVisualAsset(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, plateDir, title string, designPlan *pptxArtifactDesignPlan, slidePlan pptxArtifactSlideDesignPlan, options PPTXBuildOptions, ordinal, total int) (*pptxArtifactVisualAsset, *engine.GenerateIssue) {
	var lastReason string
	for attempt := 1; attempt <= pptxArtifactMaxTextFreePlateAttempts; attempt++ {
		prompt := buildPPTXArtifactTextFreePlatePrompt(title, designPlan, slidePlan, options)
		targetAspectRatio := pptxArtifactTextFreePlateAspectRatio(slidePlan)
		if attempt > 1 {
			if strings.Contains(strings.ToLower(lastReason), "readable text") {
				prompt += ". Retry correction: the previous candidate appeared to contain readable text; produce a clean editorial illustration with blank panels only, no glyphs, no pseudo-letters, no numerals, no labels, and no signage."
			} else {
				prompt = buildPPTXArtifactTextFreePlateRetryPrompt(title, slidePlan, designPlan, options)
			}
		}
		emitProgress(ctx, progress, progressStepAssemble, "running", fmt.Sprintf("Generating artifact text-free visual plate (%d/%d)", ordinal, total))
		imageCtx, cancel := contextWithPPTXArtifactTextFreePlateTimeout(ctx)
		image, err := generateImageWithHeartbeat(imageCtx, llm, progress, engine.ImageGenerationRequest{
			Prompt:            buildPPTXImagePrompt(prompt),
			TargetAspectRatio: targetAspectRatio,
		}, progressStepAssemble, ordinal, total)
		cancel()
		if err != nil || image == nil || len(image.Data) == 0 {
			lastReason = summarizeImageGenerationError(err)
			if attempt < pptxArtifactMaxTextFreePlateAttempts && retryablePPTXArtifactTextFreePlateGenerationFailure(err) {
				continue
			}
			break
		}
		if image.CreditBalance != nil && options.CreditBalanceSink != nil {
			options.CreditBalanceSink(*image.CreditBalance)
		}
		if image.CreditsCharged != nil && options.CreditChargedSink != nil {
			options.CreditChargedSink(*image.CreditsCharged)
		}
		mime := strings.TrimSpace(image.MIME)
		if mime == "" {
			mime = "image/png"
		}
		assetPath := filepath.Join(plateDir, fmt.Sprintf("slide-%02d-%s%s", slidePlan.Slide, pptxArtifactVisualAssetRoleName(slidePlan.Role), pptxArtifactImageExtension(mime)))
		if err := os.WriteFile(assetPath, image.Data, 0o644); err != nil {
			lastReason = "could not write generated plate: " + err.Error()
			break
		}
		detectedText, checkedText, detectErr := detectPPTXArtifactImageText(ctx, assetPath)
		if detectErr == nil && checkedText && pptxArtifactDetectedReadableText(detectedText) {
			_ = os.Remove(assetPath)
			lastReason = "generated text-free plate appears to contain readable text"
			if attempt < pptxArtifactMaxTextFreePlateAttempts {
				continue
			}
			break
		}
		width, height := pptxArtifactImageDimensions(image.Data)
		return &pptxArtifactVisualAsset{
			Path:              assetPath,
			Name:              filepath.Base(assetPath),
			MIME:              mime,
			Slide:             slidePlan.Slide,
			Frame:             pptxArtifactTextFreePlateFrame(slidePlan),
			TargetAspectRatio: targetAspectRatio,
			SourceAspectRatio: pptxArtifactAspectRatio(width, height),
			TextDetection:     pptxArtifactTextDetectionResult(checkedText, detectErr, attempt),
			VisualSignal:      pptxArtifactImageSignalFromBytes(image.Data),
			Width:             width,
			Height:            height,
			SizeBytes:         int64(len(image.Data)),
		}, nil
	}
	if strings.TrimSpace(lastReason) == "" {
		lastReason = "image generation failed"
	}
	emitProgress(ctx, progress, progressStepAssemble, "running", "Artifact text-free visual plate generation skipped: "+lastReason)
	warning := pptxArtifactTextFreePlateWarning(fmt.Sprintf("slide %d: %s", slidePlan.Slide, lastReason))
	return nil, &warning
}

func dedupePPTXArtifactSlideBoundVisualAssets(assets []pptxArtifactVisualAsset) []pptxArtifactVisualAsset {
	if len(assets) <= 1 {
		return assets
	}
	seenSlides := map[int]bool{}
	out := make([]pptxArtifactVisualAsset, 0, len(assets))
	for _, asset := range assets {
		if asset.Slide <= 0 {
			out = append(out, asset)
			continue
		}
		if seenSlides[asset.Slide] {
			continue
		}
		seenSlides[asset.Slide] = true
		out = append(out, asset)
	}
	return out
}

func retryablePPTXArtifactTextFreePlateGenerationFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, needle := range []string{
		"deadline exceeded",
		"timeout",
		"timed out",
		"temporarily unavailable",
		"temporary",
		"connection reset",
		"connection refused",
		"eof",
		"status=429",
		"status=502",
		"status=503",
		"status=504",
	} {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func detectPPTXArtifactImageTextDefault(ctx context.Context, imagePath string) (string, bool, error) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OFFICECLI_PPTX_ARTIFACT_TEXT_DETECTION")), "off") ||
		strings.TrimSpace(os.Getenv("OFFICECLI_PPTX_ARTIFACT_TEXT_DETECTION")) == "0" {
		return "", false, nil
	}
	tesseract, err := exec.LookPath("tesseract")
	if err != nil {
		return "", false, nil
	}
	detectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(detectCtx, tesseract, imagePath, "stdout", "--psm", "6").Output()
	if err != nil {
		return "", true, err
	}
	return string(output), true, nil
}

func pptxArtifactDetectedReadableText(value string) bool {
	var run int
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			run++
			if run >= 3 {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

func pptxArtifactTextDetectionResult(checked bool, err error, attempts int) *pptxArtifactTextCheck {
	status := "unchecked"
	if checked {
		status = "passed"
	}
	if err != nil {
		status = "error"
	}
	return &pptxArtifactTextCheck{
		Checked:  checked,
		Status:   status,
		Attempts: attempts,
	}
}

func pptxArtifactImageDimensions(data []byte) (int, int) {
	if len(data) == 0 {
		return 0, 0
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func pptxArtifactAspectRatio(width, height int) float64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	return float64(width) / float64(height)
}

func pptxArtifactImageSignalFromBytes(data []byte) *pptxArtifactImageSignal {
	if len(data) == 0 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil
	}
	stepX := pptxArtifactMaxInt(1, width/64)
	stepY := pptxArtifactMaxInt(1, height/64)
	var count int
	var sum float64
	var sumSq float64
	minLuma := math.Inf(1)
	maxLuma := math.Inf(-1)
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r, g, b, _ := img.At(x, y).RGBA()
			luma := 0.2126*float64(r)/257.0 + 0.7152*float64(g)/257.0 + 0.0722*float64(b)/257.0
			if luma < minLuma {
				minLuma = luma
			}
			if luma > maxLuma {
				maxLuma = luma
			}
			sum += luma
			sumSq += luma * luma
			count++
		}
	}
	if count == 0 {
		return nil
	}
	mean := sum / float64(count)
	variance := math.Max(0, sumSq/float64(count)-mean*mean)
	stdDev := math.Sqrt(variance)
	lumaRange := maxLuma - minLuma
	status := "ok"
	if lumaRange < 24 || stdDev < 6 {
		status = "low"
	}
	return &pptxArtifactImageSignal{
		Status:      status,
		LumaRange:   lumaRange,
		LumaStdDev:  stdDev,
		SampleCount: count,
	}
}

func pptxArtifactMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func pptxArtifactTextFreePlateAspectRatio(slidePlan pptxArtifactSlideDesignPlan) float64 {
	switch strings.TrimSpace(slidePlan.Role) {
	case "cover":
		return 320.0 / 250.0
	case "observations":
		return 172.0 / 92.0
	case "closing":
		return 326.0 / 226.0
	default:
		return 16.0 / 9.0
	}
}

func pptxArtifactTextFreePlateFrame(slidePlan pptxArtifactSlideDesignPlan) *pptxArtifactAssetFrame {
	switch strings.TrimSpace(slidePlan.Role) {
	case "cover":
		return &pptxArtifactAssetFrame{Left: 780, Top: 118, Width: 320, Height: 250}
	case "observations":
		return &pptxArtifactAssetFrame{Left: 936, Top: 90, Width: 172, Height: 92}
	case "closing":
		return &pptxArtifactAssetFrame{Left: 784, Top: 120, Width: 326, Height: 226}
	default:
		return nil
	}
}

func pptxArtifactTextFreePlateSlidePlans(designPlan *pptxArtifactDesignPlan) []pptxArtifactSlideDesignPlan {
	if designPlan == nil {
		return nil
	}
	out := make([]pptxArtifactSlideDesignPlan, 0, 2)
	for _, slide := range designPlan.Slides {
		role := strings.TrimSpace(slide.Role)
		if slide.Slide <= 0 {
			continue
		}
		visualTreatment := normalizePPTXArtifactVisualTreatment(slide.VisualTreatment)
		switch {
		case role == "cover":
		case role == "closing":
		case visualTreatment == "text-free-visual-plate" && (role == "observations" || slide.Slide == 2):
		default:
			continue
		}
		if slide.Slide == 2 && role == "" {
			slide.Role = "observations"
		}
		out = append(out, slide)
		if len(out) >= 3 {
			return out
		}
	}
	if len(out) == 0 && len(designPlan.Slides) > 0 && designPlan.Slides[0].Slide > 0 {
		out = append(out, designPlan.Slides[0])
	}
	return out
}

func normalizePPTXArtifactVisualTreatment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.Join(strings.Fields(value), "-")
	return value
}

func pptxArtifactVisualAssetRoleName(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		return "plate"
	}
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		if r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, role)
}

func buildPPTXArtifactTextFreePlatePrompt(title string, designPlan *pptxArtifactDesignPlan, slidePlan pptxArtifactSlideDesignPlan, options PPTXBuildOptions) string {
	_ = title
	styleMood := "dark refined canvas, calm negative space, concrete workflow imagery, blank document panels, human-and-machine collaboration cues, soft depth, subtle cyan and amber accent rhythm"
	if pptxArtifactLightStyleIntent(strings.TrimSpace(options.RequestedStyle+" "+options.UserPrompt), designPlan) {
		styleMood = "light editorial canvas, airy whitespace, soft paper panels, concrete workflow imagery, blank document panels, human-and-machine collaboration cues, subtle teal and blue accent rhythm"
	}
	parts := []string{
		"text-free editorial narrative art plate for a PowerPoint layout",
		"clean editorial tech illustration style, semi-flat vector depth, crisp ink-like outlines, not photorealistic, not stock photography, not a screenshot",
		styleMood,
		"purely nonverbal imagery: no readable words, no letters, no numbers, no glyph-like marks, no pseudo text, no signage, no charts with labels, no UI text",
		"documents and screens must contain only blank rectangles, abstract strokes, or unlabeled blocks",
	}
	parts = append(parts, pptxArtifactTextFreePlateCompositionPrompt(slidePlan))
	if slidePlan.Slide > 0 {
		parts = append(parts, fmt.Sprintf("slide %d role: %s", slidePlan.Slide, strings.TrimSpace(slidePlan.Role)))
	}
	if strings.TrimSpace(slidePlan.VisualIntent) != "" {
		parts = append(parts, "visual intent: "+strings.TrimSpace(slidePlan.VisualIntent))
	}
	if strings.TrimSpace(slidePlan.Composition) != "" {
		parts = append(parts, "composition: "+strings.TrimSpace(slidePlan.Composition))
	}
	if designPlan != nil {
		if strings.TrimSpace(designPlan.StyleBias) != "" {
			parts = append(parts, "style bias: "+strings.TrimSpace(designPlan.StyleBias))
		}
		if len(designPlan.Slides) > 0 {
			parts = append(parts, "nonverbal visual arc: "+summarizePPTXArtifactDesignPlanForTextFreePlate(designPlan))
		}
	}
	if options.ReferenceBrief != nil {
		if strings.TrimSpace(options.ReferenceBrief.PaletteIntent) != "" {
			parts = append(parts, "palette intent: "+strings.TrimSpace(options.ReferenceBrief.PaletteIntent))
		}
		if strings.TrimSpace(options.ReferenceBrief.ImageTreatment) != "" {
			parts = append(parts, "image treatment: "+strings.TrimSpace(options.ReferenceBrief.ImageTreatment))
		}
	}
	return strings.Join(parts, ". ")
}

func buildPPTXArtifactTextFreePlateRetryPrompt(title string, slidePlan pptxArtifactSlideDesignPlan, designPlan *pptxArtifactDesignPlan, options PPTXBuildOptions) string {
	_ = title
	role := strings.TrimSpace(slidePlan.Role)
	if role == "" {
		role = "supporting"
	}
	styleMood := "simple dark tech composition with one clear focal motif, blank document rectangles, soft cyan and amber accents"
	if pptxArtifactLightStyleIntent(strings.TrimSpace(options.RequestedStyle+" "+options.UserPrompt), designPlan) {
		styleMood = "simple light editorial composition with one clear focal motif, blank document rectangles, soft teal and blue accents"
	}
	return strings.Join([]string{
		"text-free editorial vector art plate for a PowerPoint slide",
		styleMood,
		"no readable text, no letters, no numbers, no logos, no signage, no labeled screens",
		pptxArtifactTextFreePlateCompositionPrompt(slidePlan),
		fmt.Sprintf("slide %d role: %s", slidePlan.Slide, role),
	}, ". ")
}

func pptxArtifactTextFreePlateCompositionPrompt(slidePlan pptxArtifactSlideDesignPlan) string {
	switch strings.TrimSpace(slidePlan.Role) {
	case "cover":
		return "right-side cover visual crop for a rounded panel; show a nonverbal before-to-after workflow scene with a document system moving from rough input to polished output; leave calm negative space and avoid central title-like marks"
	case "observations":
		return "top-right observation micro-plate for a compact horizontal frame; use abstract cards, tiny icon containers, and texture only"
	case "closing":
		return "right-side closing summary plate for a rounded panel; show a nonverbal quality review loop with a person, document surfaces, and automation arm or agent-like assistant; use quiet depth"
	default:
		return "supporting slide visual crop; keep it decorative and secondary to editable foreground text"
	}
}

func pptxArtifactImageExtension(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func pptxArtifactTextFreePlateWarning(reason string) engine.GenerateIssue {
	return engine.GenerateIssue{
		Code:    "WARN_PPT_IMAGE_DEGRADED",
		Field:   "image_quality",
		Message: "Artifact experimental text-free visual plate generation failed for at least one slide, so affected slides used editable native motifs instead: " + strings.TrimSpace(reason),
	}
}

func summarizePPTXArtifactDesignPlanForAssetRelevance(plan *pptxArtifactDesignPlan) string {
	if plan == nil {
		return ""
	}
	parts := []string{plan.DeckIntent, plan.StyleBias, plan.BuilderRecipe}
	for _, slide := range plan.Slides {
		parts = append(parts,
			slide.Role,
			slide.LayoutMode,
			slide.DisplayTitle,
			slide.DisplaySubtitle,
			slide.DisplayBody,
			slide.Takeaway,
			slide.VisualIntent,
		)
		for _, card := range slide.Cards {
			parts = append(parts, card.Heading, card.Detail)
		}
		for _, callout := range slide.ChartCallouts {
			parts = append(parts, callout.Heading, callout.Detail)
		}
	}
	if plan.DeckIntent == "concise-reference-style-learning" {
		for _, card := range referenceLearningPlanObservationCards() {
			parts = append(parts, card.Heading, card.Detail)
		}
		for _, callout := range referenceLearningPlanChartCallouts() {
			parts = append(parts, callout.Heading, callout.Detail)
		}
	}
	return strings.Join(parts, " ")
}

func summarizePPTXArtifactDesignPlanForTextFreePlate(plan *pptxArtifactDesignPlan) string {
	if plan == nil {
		return ""
	}
	parts := []string{plan.DeckIntent, plan.StyleBias, plan.BuilderRecipe}
	for _, slide := range plan.Slides {
		parts = append(parts,
			slide.Role,
			slide.LayoutMode,
			slide.Composition,
			slide.VisualTreatment,
			slide.DensityTarget,
		)
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " ")
}

func selectPPTXArtifactVisualAssetsForPrompt(assets []pptxArtifactVisualAsset, limit int, prompt string) []pptxArtifactVisualAsset {
	if limit <= 0 || len(assets) == 0 {
		return nil
	}
	if len(assets) <= limit {
		return assets
	}
	out := append([]pptxArtifactVisualAsset(nil), assets[:limit]...)
	if pptxArtifactHasPromptRelevantSecondaryAsset(out, prompt) {
		return out
	}
	var chosen *pptxArtifactVisualAsset
	chosenScore := 0
	for idx := range assets {
		asset := assets[idx]
		if artifactVisualAssetIsCover(asset.Path) {
			continue
		}
		score := pptxArtifactAssetPromptRelevanceScore(asset.Path, prompt)
		if score <= 0 {
			continue
		}
		if chosen == nil || score > chosenScore || (score == chosenScore && artifactVisualAssetSecondaryRank(asset.Path) < artifactVisualAssetSecondaryRank(chosen.Path)) {
			candidate := asset
			chosen = &candidate
			chosenScore = score
		}
	}
	if chosen == nil {
		return out
	}
	replaceAt := len(out) - 1
	for idx := len(out) - 1; idx >= 0; idx-- {
		if !artifactVisualAssetIsCover(out[idx].Path) {
			replaceAt = idx
			break
		}
	}
	out[replaceAt] = *chosen
	return out
}

func pptxArtifactHasPromptRelevantSecondaryAsset(assets []pptxArtifactVisualAsset, prompt string) bool {
	for _, asset := range assets {
		if artifactVisualAssetIsCover(asset.Path) {
			continue
		}
		if pptxArtifactAssetPromptRelevanceScore(asset.Path, prompt) > 0 {
			return true
		}
	}
	return false
}

func pptxArtifactAssetPromptRelevanceScore(path, prompt string) int {
	pathText := pptxArtifactAssetSemanticPath(path)
	promptText := strings.ToLower(prompt)
	score := 0
	for _, weightedGroup := range []struct {
		weight int
		terms  []string
	}{
		{weight: 220, terms: []string{"test", "testing", "qa", "quality", "validation", "verify", "verification", "preview", "review", "测试", "质量", "验证", "预览"}},
		{weight: 70, terms: []string{"agent", "builder", "background", "worker", "交付", "工程", "软件"}},
		{weight: 25, terms: []string{"style", "reference", "palette", "layout", "hierarchy", "风格", "参考"}},
	} {
		if pptxArtifactContainsAny(promptText, weightedGroup.terms) && pptxArtifactContainsAny(pathText, weightedGroup.terms) {
			score += weightedGroup.weight
		}
	}
	if strings.Contains(pathText, "image-02") || strings.Contains(pathText, "image-03") {
		score += 10
	}
	return score
}

func pptxArtifactAssetSemanticPath(path string) string {
	normalized := strings.ToLower(filepath.ToSlash(path))
	for _, marker := range []string{"/参考文章/", "/reference/", "/refs/"} {
		if idx := strings.Index(normalized, marker); idx >= 0 {
			return normalized[idx+1:]
		}
	}
	return normalized
}

func pptxArtifactContainsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func resolvePPTXArtifactDesignPlan(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, payload pptxPayload, fallback string, options PPTXBuildOptions) (*pptxArtifactDesignPlan, []engine.GenerateIssue) {
	deterministic := buildPPTXArtifactDesignPlan(payload, fallback, options)
	if deterministic == nil || !options.GenerateArtifactDesignPlan || llm == nil {
		return deterministic, nil
	}
	plannerLLM := llm
	if options.ArtifactDesignPlanLLM != nil {
		plannerLLM = options.ArtifactDesignPlanLLM
	}
	plannerCtx, cancel := contextWithPPTXArtifactDesignPlanTimeout(ctx)
	defer cancel()
	response, err := completeStructuredWithHeartbeat(plannerCtx, plannerLLM, progress, buildPPTXArtifactDesignPlanRequest(payload, fallback, options, deterministic), progressStepAssemble, "artifact design planner")
	if err != nil {
		return deterministic, []engine.GenerateIssue{pptxArtifactDesignPlanFallbackWarning("structured planner failed: " + err.Error())}
	}
	var generated pptxArtifactDesignPlan
	if err := json.Unmarshal([]byte(generateengine.ExtractJSON(response)), &generated); err != nil {
		return deterministic, []engine.GenerateIssue{pptxArtifactDesignPlanFallbackWarning("structured planner returned invalid JSON")}
	}
	plan, err := normalizePPTXArtifactDesignPlan(generated, deterministic)
	if err != nil {
		return deterministic, []engine.GenerateIssue{pptxArtifactDesignPlanFallbackWarning(err.Error())}
	}
	return plan, nil
}

func pptxArtifactDesignPlanFallbackWarning(reason string) engine.GenerateIssue {
	return engine.GenerateIssue{
		Code:    "WARN_PPTX_ARTIFACT_DESIGN_PLAN_FALLBACK",
		Field:   "pptx_backend",
		Message: "Artifact experimental design planner fell back to deterministic planning: " + strings.TrimSpace(reason),
	}
}

func contextWithPPTXArtifactDesignPlanTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if pptxArtifactDesignPlanTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, pptxArtifactDesignPlanTimeout)
}

func contextWithPPTXArtifactTextFreePlateTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if pptxArtifactTextFreePlateTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, pptxArtifactTextFreePlateTimeout)
}

func pptxArtifactDesignRepairWarning(reason string) engine.GenerateIssue {
	return engine.GenerateIssue{
		Code:    "WARN_PPTX_ARTIFACT_DESIGN_REPAIR",
		Field:   "pptx_backend",
		Message: "Artifact experimental preview-informed design repair was skipped: " + strings.TrimSpace(reason),
	}
}

func pptxArtifactDesignRepairAppliedWarning() engine.GenerateIssue {
	return engine.GenerateIssue{
		Code:    "WARN_PPTX_ARTIFACT_DESIGN_REPAIR_APPLIED",
		Field:   "pptx_backend",
		Message: "Artifact experimental worker retried with a preview-informed design plan.",
	}
}

func pptxArtifactDesignPolishWarning(reason string) engine.GenerateIssue {
	return engine.GenerateIssue{
		Code:    "WARN_PPTX_ARTIFACT_POLISH_DESIGN",
		Field:   "pptx_backend",
		Message: "Artifact experimental preview-informed design polish was skipped: " + strings.TrimSpace(reason),
	}
}

func pptxArtifactDesignPolishAppliedWarning() engine.GenerateIssue {
	return engine.GenerateIssue{
		Code:    "WARN_PPTX_ARTIFACT_POLISH_DESIGN_APPLIED",
		Field:   "pptx_backend",
		Message: "Artifact experimental worker polished the design plan using rendered preview/inspect evidence.",
	}
}

func resolvePPTXArtifactRepairDesignPlan(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, payload pptxPayload, fallback string, options PPTXBuildOptions, current *pptxArtifactDesignPlan, failure error) (*pptxArtifactDesignPlan, []engine.GenerateIssue) {
	if current == nil {
		return nil, []engine.GenerateIssue{pptxArtifactDesignRepairWarning("missing current design plan")}
	}
	if !options.GenerateArtifactDesignPlan || llm == nil {
		return nil, nil
	}
	plannerLLM := llm
	if options.ArtifactDesignPlanLLM != nil {
		plannerLLM = options.ArtifactDesignPlanLLM
	}
	plannerCtx, cancel := contextWithPPTXArtifactDesignPlanTimeout(ctx)
	defer cancel()
	response, err := completeStructuredWithHeartbeat(plannerCtx, plannerLLM, progress, buildPPTXArtifactDesignRepairRequest(payload, fallback, options, current, failure), progressStepAssemble, "artifact design repair planner")
	if err != nil {
		return nil, []engine.GenerateIssue{pptxArtifactDesignRepairWarning("structured repair failed: " + err.Error())}
	}
	var generated pptxArtifactDesignPlan
	if err := json.Unmarshal([]byte(generateengine.ExtractJSON(response)), &generated); err != nil {
		return nil, []engine.GenerateIssue{pptxArtifactDesignRepairWarning("structured repair returned invalid JSON")}
	}
	plan, err := normalizePPTXArtifactDesignPlan(generated, current)
	if err != nil {
		return nil, []engine.GenerateIssue{pptxArtifactDesignRepairWarning(err.Error())}
	}
	return plan, []engine.GenerateIssue{pptxArtifactDesignRepairAppliedWarning()}
}

func resolvePPTXArtifactPolishDesignPlan(ctx context.Context, llm engine.LLMClient, progress engine.ProgressEmitter, payload pptxPayload, fallback string, options PPTXBuildOptions, current *pptxArtifactDesignPlan, output *pptxArtifactWorkerOutput) (*pptxArtifactDesignPlan, []engine.GenerateIssue) {
	if current == nil {
		return nil, []engine.GenerateIssue{pptxArtifactDesignPolishWarning("missing current design plan")}
	}
	if !options.GenerateArtifactDesignPlan || llm == nil {
		return nil, nil
	}
	plannerLLM := llm
	if options.ArtifactDesignPlanLLM != nil {
		plannerLLM = options.ArtifactDesignPlanLLM
	}
	plannerCtx, cancel := contextWithPPTXArtifactDesignPlanTimeout(ctx)
	defer cancel()
	response, err := completeStructuredWithHeartbeat(plannerCtx, plannerLLM, progress, buildPPTXArtifactDesignPolishRequest(payload, fallback, options, current, output), progressStepAssemble, "artifact design polish planner")
	if err != nil {
		return nil, []engine.GenerateIssue{pptxArtifactDesignPolishWarning("structured polish failed: " + err.Error())}
	}
	var generated pptxArtifactDesignPlan
	if err := json.Unmarshal([]byte(generateengine.ExtractJSON(response)), &generated); err != nil {
		return nil, []engine.GenerateIssue{pptxArtifactDesignPolishWarning("structured polish returned invalid JSON")}
	}
	plan, err := normalizePPTXArtifactDesignPlan(generated, current)
	if err != nil {
		return nil, []engine.GenerateIssue{pptxArtifactDesignPolishWarning(err.Error())}
	}
	return plan, []engine.GenerateIssue{pptxArtifactDesignPolishAppliedWarning()}
}

func buildPPTXArtifactDesignPlanRequest(payload pptxPayload, fallback string, options PPTXBuildOptions, deterministic *pptxArtifactDesignPlan) engine.StructuredCompletionRequest {
	return engine.StructuredCompletionRequest{
		Messages: []engine.LLMMessage{{
			Role:    "user",
			Content: buildPPTXArtifactDesignPlanPrompt(payload, fallback, options, deterministic),
		}},
		Schema: engine.StructuredSchema{
			Name:        "pptx_artifact_design_plan",
			Description: "Create a slide-level editable PowerPoint builder plan for the artifact experimental backend.",
			JSONSchema:  []byte(pptxArtifactDesignPlanSchema),
			Strict:      true,
		},
	}
}

func buildPPTXArtifactDesignRepairRequest(payload pptxPayload, fallback string, options PPTXBuildOptions, current *pptxArtifactDesignPlan, failure error) engine.StructuredCompletionRequest {
	return engine.StructuredCompletionRequest{
		Messages: []engine.LLMMessage{{
			Role:    "user",
			Content: buildPPTXArtifactDesignRepairPrompt(payload, fallback, options, current, failure),
		}},
		Schema: engine.StructuredSchema{
			Name:        "pptx_artifact_design_plan_repair",
			Description: "Repair a slide-level editable PowerPoint builder plan after preview and inspect validation failed.",
			JSONSchema:  []byte(pptxArtifactDesignPlanSchema),
			Strict:      true,
		},
	}
}

func buildPPTXArtifactDesignPolishRequest(payload pptxPayload, fallback string, options PPTXBuildOptions, current *pptxArtifactDesignPlan, output *pptxArtifactWorkerOutput) engine.StructuredCompletionRequest {
	return engine.StructuredCompletionRequest{
		Messages: []engine.LLMMessage{{
			Role:    "user",
			Content: buildPPTXArtifactDesignPolishPrompt(payload, fallback, options, current, output),
		}},
		Schema: engine.StructuredSchema{
			Name:        "pptx_artifact_design_plan_polish",
			Description: "Polish a slide-level editable PowerPoint builder plan after rendered preview and inspect validation passed.",
			JSONSchema:  []byte(pptxArtifactDesignPlanSchema),
			Strict:      true,
		},
	}
}

func buildPPTXArtifactDesignPlanPrompt(payload pptxPayload, fallback string, options PPTXBuildOptions, deterministic *pptxArtifactDesignPlan) string {
	var b strings.Builder
	b.WriteString("Create a slide-level builder plan for an editable PowerPoint deck.\n\n")
	b.WriteString("User request:\n")
	b.WriteString(strings.TrimSpace(firstNonEmpty(options.UserPrompt, fallback)))
	b.WriteString("\n\nDeck title:\n")
	b.WriteString(strings.TrimSpace(firstNonEmpty(payload.Title, fallback, "Presentation")))
	b.WriteString("\n\nRules:\n")
	b.WriteString("- Plan editable PowerPoint objects only: text, shapes, tables, native charts, and optional text-free visual plates.\n")
	b.WriteString("- Do not copy source slide wording, raw XML, absolute coordinates, or low-level color/font values.\n")
	b.WriteString("- Use displayTitle/displaySubtitle/displayBody/kicker/takeaway as audience-facing visible text only; no implementation wording.\n")
	b.WriteString("- displayTitle should be a task-specific slide headline, not a raw semantic JSON placeholder.\n")
	b.WriteString("- displaySubtitle and displayBody should be concise enough to pass a rendered slide density check.\n")
	b.WriteString("- For observation-cards slides, provide up to 3 cards with concise audience-facing heading/detail pairs.\n")
	b.WriteString("- For chart-insight-stack slides, provide up to 3 chartCallouts with concise audience-facing heading/detail pairs.\n")
	b.WriteString("- Keep the same number of slides and slide numbers as the input.\n")
	b.WriteString("- builderRecipe must be one of standard, codex-reference-learning. Use codex-reference-learning only for concise reference-style-learning tasks.\n")
	b.WriteString("- builderPatch is a safe dynamic builder DSL. Use slides with slide, accentRail, and backplate only; no code, coordinates, fonts, colors, or XML.\n")
	b.WriteString("- builderPatch accentRail must be one of left, right, top, bottom, or empty string. backplate must be one of left-band, right-band, top-band, bottom-band, or empty string.\n")
	b.WriteString("- layoutMode must be one of cover-split-visual, observation-cards, chart-insight-stack, closing-takeaway, metric-cards, section-cards, point-cards, content-cards.\n\n")
	b.WriteString("- composition must be one of standard, split-hero, numbered-cards, chart-with-side-insights, split-callout.\n\n")
	if options.ReferenceBrief != nil {
		briefBytes, _ := json.Marshal(options.ReferenceBrief)
		b.WriteString("Reference style brief:\n")
		b.Write(briefBytes)
		b.WriteString("\n\n")
	}
	if deterministic != nil {
		planBytes, _ := json.Marshal(deterministic)
		b.WriteString("Deterministic baseline plan to improve:\n")
		b.Write(planBytes)
		b.WriteString("\n\n")
	}
	b.WriteString("Slides:\n")
	for idx, slide := range payload.Slides {
		summary := map[string]any{
			"slide":    idx + 1,
			"title":    slide.Title,
			"subtitle": slide.Subtitle,
			"layout":   slide.Layout,
			"isTitle":  slide.IsTitle,
			"hasChart": slide.Chart != nil,
			"sections": len(slide.Sections),
			"points":   limitStrings(slide.Points, 4),
		}
		bytes, _ := json.Marshal(summary)
		b.Write(bytes)
		b.WriteByte('\n')
	}
	return b.String()
}

func buildPPTXArtifactDesignRepairPrompt(payload pptxPayload, fallback string, options PPTXBuildOptions, current *pptxArtifactDesignPlan, failure error) string {
	var b strings.Builder
	b.WriteString("Repair the slide-level builder plan for an editable PowerPoint deck after rendered preview/inspect validation failed.\n\n")
	b.WriteString("User request:\n")
	b.WriteString(strings.TrimSpace(firstNonEmpty(options.UserPrompt, fallback)))
	b.WriteString("\n\nFailure evidence from preview/inspect validation:\n")
	b.WriteString(summarizePPTXArtifactRepairFailure(failure))
	if guidance := summarizePPTXArtifactVisualRepairGuidance(failure); strings.TrimSpace(guidance) != "" {
		b.WriteString("\n\nVisual repair guidance:\n")
		b.WriteString(guidance)
	}
	b.WriteString("\n\nRepair rules:\n")
	b.WriteString("- Keep the same number of slides and slide numbers.\n")
	b.WriteString("- Keep all important slide words editable; do not move important text into images.\n")
	b.WriteString("- Rewrite visible copy so it is audience-facing, specific to the user's request, and not implementation wording.\n")
	b.WriteString("- Use fewer, stronger cards/callouts when the evidence indicates density, overlap, narrow text, small text, or incomplete phrasing.\n")
	b.WriteString("- Preserve native chart intent on chart slides, but simplify surrounding callouts if the chart slide failed.\n")
	b.WriteString("- Keep builderPatch as a safe dynamic builder DSL only: slide, accentRail, and backplate tokens; no code, coordinates, fonts, colors, or XML.\n")
	b.WriteString("- Do not copy source slide wording, raw XML, absolute coordinates, or low-level color/font values.\n\n")
	if options.ReferenceBrief != nil {
		briefBytes, _ := json.Marshal(options.ReferenceBrief)
		b.WriteString("Reference style brief:\n")
		b.Write(briefBytes)
		b.WriteString("\n\n")
	}
	if current != nil {
		planBytes, _ := json.Marshal(current)
		b.WriteString("Current plan to repair:\n")
		b.Write(planBytes)
		b.WriteString("\n\n")
	}
	b.WriteString("Slides:\n")
	for idx, slide := range payload.Slides {
		summary := map[string]any{
			"slide":    idx + 1,
			"title":    slide.Title,
			"subtitle": slide.Subtitle,
			"layout":   slide.Layout,
			"isTitle":  slide.IsTitle,
			"hasChart": slide.Chart != nil,
			"sections": len(slide.Sections),
			"points":   limitStrings(slide.Points, 4),
		}
		bytes, _ := json.Marshal(summary)
		b.Write(bytes)
		b.WriteByte('\n')
	}
	return b.String()
}

func buildPPTXArtifactDesignPolishPrompt(payload pptxPayload, fallback string, options PPTXBuildOptions, current *pptxArtifactDesignPlan, output *pptxArtifactWorkerOutput) string {
	var b strings.Builder
	b.WriteString("Polish the slide-level builder plan for an editable PowerPoint deck after rendered preview/inspect validation passed.\n\n")
	b.WriteString("User request:\n")
	b.WriteString(strings.TrimSpace(firstNonEmpty(options.UserPrompt, fallback)))
	b.WriteString("\n\nRendered preview/inspect evidence:\n")
	b.WriteString(summarizePPTXArtifactPolishEvidence(output))
	b.WriteString("\n\nPolish rules:\n")
	b.WriteString("- Keep the same number of slides and slide numbers.\n")
	b.WriteString("- Do not introduce absolute coordinates, raw XML, or low-level color/font values.\n")
	b.WriteString("- Keep important text editable; do not move important words into images.\n")
	b.WriteString("- Preserve native chart intent on chart slides and keep the chart as a native chart.\n")
	b.WriteString("- Patch visible copy, composition labels, card headings/details, and chart callouts only.\n")
	b.WriteString("- You may also add or revise builderPatch using only safe underlay tokens: accentRail left/right/top/bottom and backplate left-band/right-band/top-band/bottom-band.\n")
	b.WriteString("- Make the deck more task-specific and Codex-like: stronger thesis, clearer slide jobs, fewer generic labels, and more deliberate card/callout hierarchy.\n")
	b.WriteString("- If evidence already passes validation, polish for narrative and composition quality without increasing density.\n\n")
	if options.ReferenceBrief != nil {
		briefBytes, _ := json.Marshal(options.ReferenceBrief)
		b.WriteString("Reference style brief:\n")
		b.Write(briefBytes)
		b.WriteString("\n\n")
	}
	if current != nil {
		planBytes, _ := json.Marshal(current)
		b.WriteString("Current plan to polish:\n")
		b.Write(planBytes)
		b.WriteString("\n\n")
	}
	b.WriteString("Slides:\n")
	for idx, slide := range payload.Slides {
		summary := map[string]any{
			"slide":    idx + 1,
			"title":    slide.Title,
			"subtitle": slide.Subtitle,
			"layout":   slide.Layout,
			"isTitle":  slide.IsTitle,
			"hasChart": slide.Chart != nil,
			"sections": len(slide.Sections),
			"points":   limitStrings(slide.Points, 4),
		}
		bytes, _ := json.Marshal(summary)
		b.Write(bytes)
		b.WriteByte('\n')
	}
	return b.String()
}

func summarizePPTXArtifactRepairFailure(err error) string {
	if err == nil {
		return "Unknown validation failure."
	}
	var visualErr *pptxArtifactVisualVerdictError
	if errors.As(err, &visualErr) {
		summary := summarizePPTXArtifactVisualVerdictIssues(visualErr.issues, 5)
		if strings.TrimSpace(summary) != "" {
			return fmt.Sprintf("Visual verdict %s, score %d: %s", visualErr.status, visualErr.score, summary)
		}
		return fmt.Sprintf("Visual verdict %s, score %d.", visualErr.status, visualErr.score)
	}
	var structureErr *pptxArtifactStructureError
	if errors.As(err, &structureErr) {
		if strings.TrimSpace(structureErr.summary) != "" {
			return fmt.Sprintf("Structure score %d below %d: %s", structureErr.score, structureErr.threshold, structureErr.summary)
		}
		return fmt.Sprintf("Structure score %d below %d.", structureErr.score, structureErr.threshold)
	}
	return shortenLayoutText(err.Error(), 260)
}

func summarizePPTXArtifactPolishEvidence(output *pptxArtifactWorkerOutput) string {
	summary := map[string]any{}
	if output == nil {
		summary["status"] = "missing worker output"
		return marshalPPTXArtifactEvidenceSummary(summary)
	}
	summary["workerVersion"] = strings.TrimSpace(output.WorkerVersion)
	summary["outputPptx"] = strings.TrimSpace(output.OutputPPTX)
	summary["inspectPath"] = strings.TrimSpace(output.InspectPath)
	summary["workerCounts"] = map[string]int{
		"editableItems": output.EditableItems,
		"nativeCharts":  output.NativeCharts,
		"visualAssets":  output.VisualAssets,
		"previews":      len(output.PreviewFiles),
	}
	if strings.TrimSpace(output.VisualVerdict) != "" || output.VisualScore != 0 {
		summary["workerVisualVerdict"] = map[string]any{
			"status": strings.TrimSpace(output.VisualVerdict),
			"score":  output.VisualScore,
			"issues": limitStrings(output.VisualIssues, 8),
		}
	}
	if len(output.PreviewIssues) > 0 {
		summary["workerPreviewIssues"] = limitStrings(output.PreviewIssues, 8)
	}
	if output.PreviewReview != nil {
		summary["visionPreviewReview"] = summarizePPTXArtifactPreviewReview(*output.PreviewReview)
	}
	previewFiles := append([]string(nil), output.PreviewFiles...)
	if len(previewFiles) > 0 {
		summary["previewFiles"] = limitStrings(previewFiles, 6)
		summary["previewSignals"] = summarizePPTXArtifactPreviewSignals(previewFiles, 6)
	}
	inspectPath := strings.TrimSpace(output.InspectPath)
	if inspectPath == "" {
		return marshalPPTXArtifactEvidenceSummary(summary)
	}
	data, err := os.ReadFile(inspectPath)
	if err != nil {
		summary["inspectError"] = shortenLayoutText(err.Error(), 180)
		return marshalPPTXArtifactEvidenceSummary(summary)
	}
	var inspect pptxArtifactInspectSummary
	if err := json.Unmarshal(data, &inspect); err != nil {
		summary["inspectError"] = "invalid inspect JSON: " + shortenLayoutText(err.Error(), 160)
		return marshalPPTXArtifactEvidenceSummary(summary)
	}
	previewCount := len(inspect.Previews)
	if previewCount == 0 {
		previewCount = len(output.PreviewFiles)
	}
	if len(previewFiles) == 0 && len(inspect.Previews) > 0 {
		previewFiles = append(previewFiles, inspect.Previews...)
		summary["previewFiles"] = limitStrings(previewFiles, 6)
		summary["previewSignals"] = summarizePPTXArtifactPreviewSignals(previewFiles, 6)
	}
	summary["inspectCounts"] = map[string]int{
		"editableItems": len(inspect.EditableItems),
		"visualItems":   len(inspect.VisualItems),
		"nativeCharts":  len(inspect.NativeCharts),
		"images":        len(inspect.Images),
		"previews":      previewCount,
	}
	summary["editableItemsBySlideRole"] = summarizePPTXArtifactEditableItemsBySlideRole(inspect.EditableItems)
	summary["visualItemsBySlideRole"] = summarizePPTXArtifactVisualItemsBySlideRole(inspect.VisualItems)
	if inspect.VisualVerdict != nil {
		issues := make([]map[string]any, 0, len(inspect.VisualVerdict.Issues))
		for _, issue := range inspect.VisualVerdict.Issues {
			issues = append(issues, map[string]any{
				"slide":    issue.Slide,
				"code":     issue.Code,
				"severity": issue.Severity,
				"message":  shortenLayoutText(issue.Message, 120),
			})
			if len(issues) >= 5 {
				break
			}
		}
		summary["visualVerdict"] = map[string]any{
			"status": strings.TrimSpace(inspect.VisualVerdict.Status),
			"score":  inspect.VisualVerdict.Score,
			"issues": issues,
		}
	}
	return marshalPPTXArtifactEvidenceSummary(summary)
}

func summarizePPTXArtifactPreviewReview(result PPTXArtifactPreviewReviewResult) map[string]any {
	result = normalizePPTXArtifactPreviewReviewResult(result)
	issues := make([]map[string]any, 0, len(result.Issues))
	for _, issue := range result.Issues {
		issues = append(issues, map[string]any{
			"severity":     strings.TrimSpace(issue.Severity),
			"code":         strings.TrimSpace(issue.Code),
			"title":        strings.TrimSpace(issue.Title),
			"message":      strings.TrimSpace(issue.Message),
			"slideNumbers": issue.SlideNumbers,
			"suggestion":   strings.TrimSpace(issue.Suggestion),
		})
	}
	return map[string]any{
		"score":     result.Score,
		"summary":   strings.TrimSpace(result.Summary),
		"strengths": result.Strengths,
		"issues":    issues,
	}
}

func summarizePPTXArtifactPreviewSignals(paths []string, limit int) []map[string]any {
	if limit <= 0 {
		return nil
	}
	out := make([]map[string]any, 0, artifactMinInt(len(paths), limit))
	for idx, path := range paths {
		if idx >= limit {
			break
		}
		entry := map[string]any{
			"slide": idx + 1,
			"path":  shortenLayoutText(strings.TrimSpace(path), 180),
			"file":  filepath.Base(strings.TrimSpace(path)),
		}
		signal, err := validatePPTXArtifactPreviewImage(path)
		if err != nil {
			entry["error"] = shortenLayoutText(err.Error(), 180)
			out = append(out, entry)
			continue
		}
		entry["width"] = signal.Width
		entry["height"] = signal.Height
		entry["meanLuma"] = roundPPTXArtifactEvidenceFloat(signal.MeanLuma)
		entry["lumaRange"] = roundPPTXArtifactEvidenceFloat(signal.LumaRange)
		entry["lumaStdDev"] = roundPPTXArtifactEvidenceFloat(signal.LumaStdDev)
		entry["distinctColors"] = signal.DistinctColors
		entry["opaqueSamples"] = signal.OpaqueSamples
		entry["sampleCount"] = signal.SampleCount
		out = append(out, entry)
	}
	return out
}

func roundPPTXArtifactEvidenceFloat(value float64) float64 {
	return math.Round(value*10) / 10
}

func summarizePPTXArtifactEditableItemsBySlideRole(items []pptxArtifactEditableInspectItem) []map[string]any {
	counts := map[string]int{}
	for _, item := range items {
		key := pptxArtifactSlideRoleKey(item.Slide, item.Role)
		counts[key]++
	}
	return pptxArtifactSortedCountSummary(counts)
}

func summarizePPTXArtifactVisualItemsBySlideRole(items []pptxArtifactVisualInspectItem) []map[string]any {
	counts := map[string]int{}
	for _, item := range items {
		key := pptxArtifactSlideRoleKey(item.Slide, item.Role)
		counts[key]++
	}
	return pptxArtifactSortedCountSummary(counts)
}

func pptxArtifactSlideRoleKey(slide int, role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		role = "unspecified"
	}
	return fmt.Sprintf("%03d|%s", slide, role)
}

func pptxArtifactSortedCountSummary(counts map[string]int) []map[string]any {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		slide := 0
		role := key
		if parts := strings.SplitN(key, "|", 2); len(parts) == 2 {
			fmt.Sscanf(parts[0], "%d", &slide)
			role = parts[1]
		}
		out = append(out, map[string]any{
			"slide": slide,
			"role":  role,
			"count": counts[key],
		})
	}
	return out
}

func marshalPPTXArtifactEvidenceSummary(summary map[string]any) string {
	data, err := json.Marshal(summary)
	if err != nil {
		return "{}"
	}
	return string(data)
}

const pptxArtifactDesignPlanSchema = `{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "deckIntent": { "type": "string" },
    "styleBias": { "type": "string" },
    "builderRecipe": { "type": "string" },
    "builderPatch": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "slides": {
          "type": "array",
          "items": {
            "type": "object",
            "additionalProperties": false,
            "properties": {
              "slide": { "type": "integer" },
              "accentRail": { "type": "string" },
              "backplate": { "type": "string" }
            },
            "required": ["slide", "accentRail", "backplate"]
          }
        }
      },
      "required": ["slides"]
    },
    "slides": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "properties": {
          "slide": { "type": "integer" },
          "role": { "type": "string" },
          "layoutMode": { "type": "string" },
          "composition": { "type": "string" },
          "visualTreatment": { "type": "string" },
          "densityTarget": { "type": "string" },
          "kicker": { "type": "string" },
          "displayTitle": { "type": "string" },
          "displaySubtitle": { "type": "string" },
          "displayBody": { "type": "string" },
          "takeaway": { "type": "string" },
          "visualIntent": { "type": "string" },
          "cards": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "heading": { "type": "string" },
                "detail": { "type": "string" }
              },
              "required": ["heading", "detail"]
            }
          },
          "chartCallouts": {
            "type": "array",
            "items": {
              "type": "object",
              "additionalProperties": false,
              "properties": {
                "heading": { "type": "string" },
                "detail": { "type": "string" }
              },
              "required": ["heading", "detail"]
            }
          }
        },
        "required": ["slide", "role", "layoutMode", "composition", "visualTreatment", "densityTarget", "kicker", "displayTitle", "displaySubtitle", "displayBody", "takeaway", "visualIntent", "cards", "chartCallouts"]
      }
    }
  },
  "required": ["deckIntent", "styleBias", "builderRecipe", "builderPatch", "slides"]
}`

func normalizePPTXArtifactDesignPlan(generated pptxArtifactDesignPlan, fallback *pptxArtifactDesignPlan) (*pptxArtifactDesignPlan, error) {
	if fallback == nil {
		return nil, fmt.Errorf("missing deterministic design plan")
	}
	if len(generated.Slides) != len(fallback.Slides) {
		return nil, fmt.Errorf("structured planner returned %d slides, want %d", len(generated.Slides), len(fallback.Slides))
	}
	out := &pptxArtifactDesignPlan{
		DeckIntent:    firstNonEmpty(safeArtifactPlanText(generated.DeckIntent, 80), fallback.DeckIntent),
		StyleBias:     firstNonEmpty(safeArtifactPlanText(generated.StyleBias, 64), fallback.StyleBias),
		BuilderRecipe: normalizePPTXArtifactBuilderRecipe(generated.BuilderRecipe, fallback.BuilderRecipe),
		BuilderPatch:  normalizePPTXArtifactBuilderPatch(generated.BuilderPatch, fallback.BuilderPatch, len(fallback.Slides)),
		Slides:        make([]pptxArtifactSlideDesignPlan, 0, len(fallback.Slides)),
	}
	if fallback.DeckIntent == "concise-reference-style-learning" {
		out.DeckIntent = fallback.DeckIntent
		out.BuilderRecipe = fallback.BuilderRecipe
	}
	for idx, fallbackSlide := range fallback.Slides {
		slide := generated.Slides[idx]
		if slide.Slide != idx+1 {
			slide.Slide = idx + 1
		}
		normalized := pptxArtifactSlideDesignPlan{
			Slide:           idx + 1,
			Role:            firstNonEmpty(safeArtifactPlanText(slide.Role, 32), fallbackSlide.Role),
			LayoutMode:      normalizePPTXArtifactLayoutMode(slide.LayoutMode, fallbackSlide.LayoutMode),
			Composition:     normalizePPTXArtifactComposition(slide.Composition, fallbackSlide.Composition),
			VisualTreatment: firstNonEmpty(safeArtifactPlanText(slide.VisualTreatment, 48), fallbackSlide.VisualTreatment),
			DensityTarget:   normalizePPTXArtifactDensityTarget(slide.DensityTarget, fallbackSlide.DensityTarget),
			Kicker:          firstNonEmpty(safeArtifactVisiblePlanText(slide.Kicker, 28), fallbackSlide.Kicker),
			DisplayTitle:    firstNonEmpty(safeArtifactVisiblePlanText(slide.DisplayTitle, 72), fallbackSlide.DisplayTitle),
			DisplaySubtitle: firstNonEmpty(safeArtifactVisiblePlanText(slide.DisplaySubtitle, 92), fallbackSlide.DisplaySubtitle),
			DisplayBody:     firstNonEmpty(safeArtifactVisiblePlanText(slide.DisplayBody, 128), fallbackSlide.DisplayBody),
			Takeaway:        firstNonEmpty(safeArtifactVisiblePlanText(slide.Takeaway, 118), fallbackSlide.Takeaway),
			VisualIntent:    firstNonEmpty(safeArtifactPlanText(slide.VisualIntent, 140), fallbackSlide.VisualIntent),
			Cards:           normalizePPTXArtifactPlanCards(slide.Cards, fallbackSlide.Cards, 3, 64, 96),
			ChartCallouts:   normalizePPTXArtifactPlanCards(slide.ChartCallouts, fallbackSlide.ChartCallouts, 2, 34, 44),
		}
		if out.DeckIntent == "concise-reference-style-learning" {
			normalized.DisplayTitle = pptxArtifactReferenceLearningVisibleOverride(normalized.DisplayTitle, fallbackSlide.DisplayTitle, normalized.Role, "title")
			normalized.DisplaySubtitle = pptxArtifactReferenceLearningVisibleOverride(normalized.DisplaySubtitle, fallbackSlide.DisplaySubtitle, normalized.Role, "subtitle")
			normalized.DisplayBody = pptxArtifactReferenceLearningVisibleOverride(normalized.DisplayBody, fallbackSlide.DisplayBody, normalized.Role, "body")
			if normalized.Role == "observations" || normalized.Slide == 2 {
				normalized.Takeaway = pptxArtifactReferenceLearningMechanismCopyOverride(normalized.Takeaway, fallbackSlide.Takeaway)
			}
			if normalized.Role == "closing" {
				normalized.DisplayTitle = pptxArtifactReferenceLearningClosingVisibleOverride(normalized.DisplayTitle, fallbackSlide.DisplayTitle, "title")
				normalized.DisplayBody = pptxArtifactReferenceLearningClosingVisibleOverride(normalized.DisplayBody, fallbackSlide.DisplayBody, "body")
			}
			if normalized.Role == "observations" || normalized.Slide == 2 {
				normalized.Cards = normalizePPTXArtifactReferenceLearningObservationCards(slide.Cards, fallbackSlide.Cards, 3, 64, 96)
			} else {
				normalized.Cards = normalizePPTXArtifactCompletePlanCards(slide.Cards, fallbackSlide.Cards, 3, 64, 96)
			}
			normalized.ChartCallouts = normalizePPTXArtifactCompletePlanCards(slide.ChartCallouts, fallbackSlide.ChartCallouts, 2, 34, 44)
		}
		out.Slides = append(out.Slides, normalized)
	}
	return out, nil
}

func normalizePPTXArtifactBuilderPatch(generated, fallback *pptxArtifactBuilderPatch, slideCount int) *pptxArtifactBuilderPatch {
	if slideCount <= 0 {
		return nil
	}
	var out []pptxArtifactBuilderSlidePatch
	if generated != nil {
		out = append(out, normalizePPTXArtifactBuilderSlidePatches(generated.Slides, slideCount)...)
	}
	if len(out) == 0 && fallback != nil {
		out = append(out, normalizePPTXArtifactBuilderSlidePatches(fallback.Slides, slideCount)...)
	}
	if len(out) == 0 {
		return nil
	}
	return &pptxArtifactBuilderPatch{Slides: out}
}

func normalizePPTXArtifactBuilderSlidePatches(patches []pptxArtifactBuilderSlidePatch, slideCount int) []pptxArtifactBuilderSlidePatch {
	out := make([]pptxArtifactBuilderSlidePatch, 0, len(patches))
	seen := map[int]bool{}
	for _, patch := range patches {
		slide := patch.Slide
		if slide < 1 || slide > slideCount || seen[slide] {
			continue
		}
		accentRail := normalizePPTXArtifactBuilderAccentRail(patch.AccentRail)
		backplate := normalizePPTXArtifactBuilderBackplate(patch.Backplate)
		if accentRail == "" && backplate == "" {
			continue
		}
		out = append(out, pptxArtifactBuilderSlidePatch{
			Slide:      slide,
			AccentRail: accentRail,
			Backplate:  backplate,
		})
		seen[slide] = true
	}
	return out
}

func normalizePPTXArtifactBuilderAccentRail(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	switch normalized {
	case "left", "right", "top", "bottom":
		return normalized
	default:
		return ""
	}
}

func normalizePPTXArtifactBuilderBackplate(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	switch normalized {
	case "left-band", "right-band", "top-band", "bottom-band":
		return normalized
	default:
		return ""
	}
}

func pptxArtifactReferenceLearningVisibleOverride(generated, fallback, role, field string) string {
	if strings.TrimSpace(role) == "cover" {
		return fallback
	}
	generated = strings.TrimSpace(generated)
	if generated == "" {
		return fallback
	}
	if !pptxArtifactReferenceLearningCopyIsStrong(generated, field) {
		return fallback
	}
	return generated
}

func pptxArtifactReferenceLearningClosingVisibleOverride(generated, fallback, field string) string {
	if field == "body" {
		return fallback
	}
	generated = strings.TrimSpace(generated)
	if generated == "" {
		return fallback
	}
	if !pptxArtifactReferenceLearningClosingCopyIsStrong(generated, field) {
		return fallback
	}
	return generated
}

func pptxArtifactReferenceLearningMechanismCopyOverride(generated, fallback string) string {
	generated = strings.TrimSpace(generated)
	if generated == "" {
		return fallback
	}
	if !pptxArtifactReferenceLearningMechanismCopyIsStrong(generated) {
		return fallback
	}
	return generated
}

func pptxArtifactReferenceLearningClosingCopyIsStrong(value, field string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return false
	}
	if pptxArtifactReferenceLearningCopyHasImplementationNarrative(text) {
		return false
	}
	if strings.Contains(text, "learned style") {
		return false
	}
	if field == "title" {
		return strings.Contains(text, "reference") || strings.Contains(text, "style") || strings.Contains(text, "system")
	}
	return pptxArtifactReferenceLearningCopyHasStyleSignal(text)
}

func pptxArtifactReferenceLearningMechanismCopyIsStrong(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	if pptxArtifactReferenceLearningCopyHasImplementationNarrative(text) {
		return false
	}
	for _, weak := range []string{"hierarchy, rhythm", "learn the system behind the slides"} {
		if strings.Contains(text, weak) {
			return false
		}
	}
	return pptxArtifactReferenceLearningCopyHasStyleSignal(text)
}

func pptxArtifactReferenceLearningCopyIsStrong(value, field string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return false
	}
	if pptxArtifactReferenceLearningCopyHasImplementationNarrative(text) {
		return false
	}
	for _, generic := range []string{
		"generic",
		"key observations",
		"most common style signals",
		"use the style cues",
		"learned style shows up",
		"generic chart context",
		"no stable reference",
		"guidance for future decks",
	} {
		if strings.Contains(text, generic) {
			return false
		}
	}
	score := 0
	for _, term := range []string{
		"reference", "visual", "palette", "density", "hierarchy", "spacing", "rhythm", "system", "template", "evidence", "editable", "native", "chart", "quality", "composition", "message", "cards", "deck",
	} {
		if strings.Contains(text, term) {
			score++
		}
	}
	switch field {
	case "title":
		return score >= 2 && utf8.RuneCountInString(value) >= 18
	case "subtitle", "body":
		return score >= 2 && utf8.RuneCountInString(value) >= 24
	default:
		return score >= 2
	}
}

func pptxArtifactReferenceLearningCopyHasImplementationNarrative(text string) bool {
	for _, term := range []string{"codex", "officecli", "worker", "artifact", "builder", "preview", "previews", "patch", "patches", "rendered evidence", "visual qa", "agent loop", "implementation"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func pptxArtifactReferenceLearningCopyHasStyleSignal(text string) bool {
	score := 0
	for _, term := range []string{"reference", "style", "visual", "palette", "rhythm", "hierarchy", "spacing", "density", "system", "template", "deck", "cards", "editable", "message"} {
		if strings.Contains(text, term) {
			score++
		}
	}
	return score >= 2
}

func normalizePPTXArtifactPlanCards(generated, fallback []pptxArtifactPlanCard, limit, headingRunes, detailRunes int) []pptxArtifactPlanCard {
	if limit <= 0 {
		return nil
	}
	out := make([]pptxArtifactPlanCard, 0, limit)
	for _, card := range generated {
		heading := safeArtifactVisiblePlanText(card.Heading, headingRunes)
		detail := safeArtifactVisiblePlanText(card.Detail, detailRunes)
		if heading == "" && detail == "" {
			continue
		}
		out = append(out, pptxArtifactPlanCard{Heading: heading, Detail: detail})
		if len(out) >= limit {
			return out
		}
	}
	for _, card := range fallback {
		heading := safeArtifactVisiblePlanText(card.Heading, headingRunes)
		detail := safeArtifactVisiblePlanText(card.Detail, detailRunes)
		if heading == "" && detail == "" {
			continue
		}
		out = append(out, pptxArtifactPlanCard{Heading: heading, Detail: detail})
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func normalizePPTXArtifactCompletePlanCards(generated, fallback []pptxArtifactPlanCard, limit, headingRunes, detailRunes int) []pptxArtifactPlanCard {
	if limit <= 0 {
		return nil
	}
	out := make([]pptxArtifactPlanCard, 0, limit)
	for idx := 0; idx < limit; idx++ {
		var selected pptxArtifactPlanCard
		if idx < len(generated) {
			selected = normalizePPTXArtifactCompletePlanCard(generated[idx], headingRunes, detailRunes)
		}
		if (selected == pptxArtifactPlanCard{}) && idx < len(fallback) {
			selected = normalizePPTXArtifactCompletePlanCard(fallback[idx], headingRunes, detailRunes)
		}
		if selected != (pptxArtifactPlanCard{}) {
			out = append(out, selected)
		}
	}
	for _, card := range fallback {
		if len(out) >= limit {
			return out
		}
		selected := normalizePPTXArtifactCompletePlanCard(card, headingRunes, detailRunes)
		if selected == (pptxArtifactPlanCard{}) || pptxArtifactPlanCardsContain(out, selected) {
			continue
		}
		out = append(out, selected)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func normalizePPTXArtifactReferenceLearningObservationCards(generated, fallback []pptxArtifactPlanCard, limit, headingRunes, detailRunes int) []pptxArtifactPlanCard {
	if limit <= 0 {
		return nil
	}
	out := make([]pptxArtifactPlanCard, 0, limit)
	for idx := 0; idx < limit; idx++ {
		var selected pptxArtifactPlanCard
		if idx < len(generated) {
			candidate := normalizePPTXArtifactCompletePlanCard(generated[idx], headingRunes, detailRunes)
			if pptxArtifactReferenceLearningCardIsTaskSpecific(candidate) {
				selected = candidate
			}
		}
		if (selected == pptxArtifactPlanCard{}) && idx < len(fallback) {
			selected = normalizePPTXArtifactCompletePlanCard(fallback[idx], headingRunes, detailRunes)
		}
		if selected != (pptxArtifactPlanCard{}) {
			out = append(out, selected)
		}
	}
	for _, card := range fallback {
		if len(out) >= limit {
			return out
		}
		selected := normalizePPTXArtifactCompletePlanCard(card, headingRunes, detailRunes)
		if selected == (pptxArtifactPlanCard{}) || pptxArtifactPlanCardsContain(out, selected) {
			continue
		}
		out = append(out, selected)
	}
	return out
}

func pptxArtifactReferenceLearningCardIsTaskSpecific(card pptxArtifactPlanCard) bool {
	if card == (pptxArtifactPlanCard{}) {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(card.Heading + " " + card.Detail))
	score := 0
	for _, term := range []string{
		"reference", "mimicry", "copy", "copying", "output", "selectable", "preview", "previews", "overflow", "chart", "builder", "qa", "export",
	} {
		if strings.Contains(text, term) {
			score++
		}
	}
	return score >= 1
}

func normalizePPTXArtifactCompletePlanCard(card pptxArtifactPlanCard, headingRunes, detailRunes int) pptxArtifactPlanCard {
	heading := safeArtifactVisiblePlanText(card.Heading, headingRunes)
	detail := safeArtifactVisiblePlanText(card.Detail, detailRunes)
	if heading == "" || detail == "" {
		return pptxArtifactPlanCard{}
	}
	return pptxArtifactPlanCard{Heading: heading, Detail: detail}
}

func pptxArtifactPlanCardsContain(cards []pptxArtifactPlanCard, target pptxArtifactPlanCard) bool {
	key := strings.ToLower(strings.TrimSpace(target.Heading))
	for _, card := range cards {
		if strings.ToLower(strings.TrimSpace(card.Heading)) == key {
			return true
		}
	}
	return false
}

func normalizePPTXArtifactLayoutMode(value, fallback string) string {
	switch strings.TrimSpace(value) {
	case "cover-split-visual", "observation-cards", "chart-insight-stack", "closing-takeaway", "metric-cards", "section-cards", "point-cards", "content-cards":
		return strings.TrimSpace(value)
	default:
		return fallback
	}
}

func normalizePPTXArtifactDensityTarget(value, fallback string) string {
	switch strings.TrimSpace(value) {
	case "spacious", "balanced", "compact":
		return strings.TrimSpace(value)
	default:
		return fallback
	}
}

func normalizePPTXArtifactComposition(value, fallback string) string {
	switch strings.TrimSpace(value) {
	case "standard", "split-hero", "numbered-cards", "chart-with-side-insights", "split-callout":
		return strings.TrimSpace(value)
	default:
		return firstNonEmpty(strings.TrimSpace(fallback), "standard")
	}
}

func normalizePPTXArtifactBuilderRecipe(value, fallback string) string {
	switch strings.TrimSpace(value) {
	case "standard", "codex-reference-learning":
		return strings.TrimSpace(value)
	default:
		return firstNonEmpty(strings.TrimSpace(fallback), "standard")
	}
}

func safeArtifactPlanText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || pptxArtifactVisibleTextHasImplementationLeak(value) {
		return ""
	}
	return shortenLayoutText(value, maxRunes)
}

func safeArtifactVisiblePlanText(value string, maxRunes int) string {
	value = safeArtifactPlanText(value, maxRunes)
	if value == "" || pptxArtifactVisibleTextLooksDangling(value) {
		return ""
	}
	return value
}

func buildPPTXArtifactDesignPlan(payload pptxPayload, fallback string, options PPTXBuildOptions) *pptxArtifactDesignPlan {
	slides := append([]officegen.Slide(nil), payload.Slides...)
	if len(slides) == 0 {
		return nil
	}
	plan := &pptxArtifactDesignPlan{
		DeckIntent: pptxArtifactDeckIntent(slides, fallback, options),
		StyleBias:  pptxArtifactStyleBias(payload.StylePreset, options.ReferenceBrief),
		Slides:     make([]pptxArtifactSlideDesignPlan, 0, len(slides)),
	}
	plan.BuilderRecipe = pptxArtifactBuilderRecipe(plan.DeckIntent)
	for idx, slide := range slides {
		role := pptxArtifactSlideRole(slide, idx, len(slides))
		slidePlan := pptxArtifactSlideDesignPlan{
			Slide:           idx + 1,
			Role:            role,
			LayoutMode:      pptxArtifactLayoutMode(slide, idx, len(slides), plan.DeckIntent),
			Composition:     pptxArtifactComposition(slide, idx, len(slides), plan.DeckIntent),
			VisualTreatment: pptxArtifactVisualTreatment(slide, idx, options),
			DensityTarget:   pptxArtifactDensityTarget(slide),
			Kicker:          pptxArtifactSlideKicker(slide, idx, len(slides), plan.DeckIntent),
			DisplayTitle:    pptxArtifactSlideDisplayTitle(slide, idx, len(slides), plan.DeckIntent),
			DisplaySubtitle: pptxArtifactSlideDisplaySubtitle(slide, idx, len(slides), plan.DeckIntent),
			DisplayBody:     pptxArtifactSlideDisplayBody(slide, idx, len(slides), plan.DeckIntent),
			Takeaway:        pptxArtifactSlideTakeaway(slide, idx, len(slides), plan.DeckIntent),
			VisualIntent:    pptxArtifactSlideVisualIntent(slide, idx, plan.DeckIntent, options),
		}
		if plan.DeckIntent == "concise-reference-style-learning" {
			switch {
			case idx == 1 || role == "observations":
				slidePlan.Cards = referenceLearningPlanObservationCards()
			case role == "evidence":
				slidePlan.ChartCallouts = referenceLearningPlanChartCallouts()
			}
		}
		plan.Slides = append(plan.Slides, slidePlan)
	}
	if plan.DeckIntent == "concise-reference-style-learning" && len(plan.Slides) > 0 {
		if title := pptxArtifactFallbackTopicTitle(fallback); title != "" {
			plan.Slides[0].DisplayTitle = title
		}
	}
	return plan
}

func referenceLearningPlanObservationCards() []pptxArtifactPlanCard {
	return []pptxArtifactPlanCard{
		{Heading: "Repeatable style beats single-slide mimicry", Detail: "Use repeated panels, accent rules, and compact cards instead of copying a deck."},
		{Heading: "Important content stays editable", Detail: "Keep words, labels, and chart callouts editable, not baked into images."},
		{Heading: "Readable hierarchy guides the deck", Detail: "Keep contrast, spacing, and title scale consistent across slides."},
	}
}

func referenceLearningPlanChartCallouts() []pptxArtifactPlanCard {
	return []pptxArtifactPlanCard{
		{Heading: "Style signal", Detail: "Palette, spacing, hierarchy align."},
		{Heading: "Style focus", Detail: "Spacing clarifies chart callouts."},
	}
}

func pptxArtifactComposition(slide officegen.Slide, idx, total int, deckIntent string) string {
	role := pptxArtifactSlideRole(slide, idx, total)
	if deckIntent == "concise-reference-style-learning" {
		switch {
		case role == "cover":
			return "split-hero"
		case idx == 1 || role == "observations":
			return "numbered-cards"
		case role == "evidence":
			return "chart-with-side-insights"
		case role == "closing":
			return "split-callout"
		}
	}
	switch role {
	case "cover":
		return "split-hero"
	case "observations":
		return "numbered-cards"
	case "evidence":
		return "chart-with-side-insights"
	case "closing":
		return "split-callout"
	default:
		return "standard"
	}
}

func pptxArtifactBuilderRecipe(deckIntent string) string {
	if strings.TrimSpace(deckIntent) == "concise-reference-style-learning" {
		return "codex-reference-learning"
	}
	return "standard"
}

func pptxArtifactDeckIntent(slides []officegen.Slide, fallback string, options PPTXBuildOptions) string {
	text := strings.ToLower(strings.TrimSpace(options.UserPrompt + " " + fallback))
	if wantsConciseFourSlidePPTX(text, options.ReferenceBrief) || len(slides) == 4 {
		if strings.Contains(text, "reference") || options.ReferenceBrief != nil {
			return "concise-reference-style-learning"
		}
		return "concise-four-slide"
	}
	if options.ReferenceBrief != nil {
		return "reference-informed-deck"
	}
	return "semantic-editable-deck"
}

func pptxArtifactStyleBias(stylePreset string, brief *PPTXReferenceStyleBrief) string {
	explicit := strings.ToLower(strings.TrimSpace(stylePreset))
	if pptxArtifactLightStyleIntent(explicit, nil) {
		return "editorial-light"
	}
	if strings.Contains(explicit, "dark") {
		return "dark-structured"
	}
	intent := explicit
	if brief != nil {
		intent = strings.TrimSpace(intent + " " + brief.StylePresetHint + " " + brief.PaletteIntent + " " + brief.LayoutRhythm)
	}
	switch {
	case strings.Contains(intent, "dark"):
		return "dark-structured"
	case strings.Contains(intent, "editorial"):
		return "editorial-light"
	case strings.Contains(intent, "training"):
		return "instructional"
	default:
		return "clean-office"
	}
}

func pptxArtifactLightStyleIntent(text string, designPlan *pptxArtifactDesignPlan) bool {
	intent := strings.ToLower(strings.TrimSpace(text))
	if designPlan != nil {
		intent = strings.TrimSpace(intent + " " + strings.ToLower(designPlan.StyleBias))
	}
	for _, term := range []string{"editorial-light", "light-theme", "light theme", "light style", "white canvas", "off-white", "off white", "bright canvas", "airy whitespace"} {
		if strings.Contains(intent, term) {
			return true
		}
	}
	return false
}

func pptxArtifactSlideRole(slide officegen.Slide, idx, total int) string {
	if idx == 0 || slide.IsTitle || strings.EqualFold(slide.Layout, "title") {
		return "cover"
	}
	if idx == total-1 || strings.EqualFold(slide.Layout, "closing") {
		return "closing"
	}
	if slide.Chart != nil || strings.EqualFold(slide.Layout, "chart") {
		return "evidence"
	}
	text := strings.ToLower(slide.Title + " " + slide.Subtitle + " " + slide.Content)
	if strings.Contains(text, "observation") || strings.Contains(text, "takeaway") || strings.Contains(text, "summary") {
		return "observations"
	}
	if len(slide.Metrics) > 0 {
		return "metrics"
	}
	return "content"
}

func pptxArtifactLayoutMode(slide officegen.Slide, idx, total int, deckIntent string) string {
	if deckIntent == "concise-reference-style-learning" && idx == 1 {
		return "observation-cards"
	}
	role := pptxArtifactSlideRole(slide, idx, total)
	switch role {
	case "cover":
		return "cover-split-visual"
	case "observations":
		return "observation-cards"
	case "evidence":
		return "chart-insight-stack"
	case "closing":
		return "closing-takeaway"
	case "metrics":
		return "metric-cards"
	default:
		if len(slide.Sections) > 0 {
			return "section-cards"
		}
		if strings.Contains(deckIntent, "reference") {
			return "point-cards"
		}
		return "content-cards"
	}
}

func pptxArtifactVisualTreatment(slide officegen.Slide, idx int, options PPTXBuildOptions) string {
	if idx == 0 && options.ReferenceScanRoot != "" {
		return "reference-visual-panel"
	}
	if slide.Chart != nil {
		return "native-chart"
	}
	if slide.HasImage || len(slide.Visuals) > 0 {
		return "local-asset-panel"
	}
	return "native-shapes"
}

func pptxArtifactDensityTarget(slide officegen.Slide) string {
	runes := textDensityRunes(slide)
	switch {
	case runes <= 120:
		return "spacious"
	case runes <= 220:
		return "balanced"
	default:
		return "compact"
	}
}

func pptxArtifactSlideKicker(slide officegen.Slide, idx, total int, deckIntent string) string {
	if deckIntent == "concise-reference-style-learning" && idx == 1 {
		return "KEY OBSERVATIONS"
	}
	role := pptxArtifactSlideRole(slide, idx, total)
	switch role {
	case "observations":
		return "KEY OBSERVATIONS"
	case "evidence":
		if strings.Contains(deckIntent, "concise") {
			return "SIMPLE CHART"
		}
		return "EVIDENCE SNAPSHOT"
	case "closing":
		return "Recommendation"
	default:
		return ""
	}
}

func pptxArtifactSlideDisplayTitle(slide officegen.Slide, idx, total int, deckIntent string) string {
	role := pptxArtifactSlideRole(slide, idx, total)
	if deckIntent == "concise-reference-style-learning" {
		switch {
		case role == "cover":
			return shortenLayoutText(strings.TrimSpace(slide.Title), 72)
		case idx == 1 || role == "observations":
			return "What the reference directory actually teaches"
		case role == "evidence":
			return "Fidelity comes from multiple enforced layers"
		case role == "closing":
			return "Reference style becomes a reusable system"
		}
	}
	return shortenLayoutText(strings.TrimSpace(slide.Title), 72)
}

func pptxArtifactSlideDisplaySubtitle(slide officegen.Slide, idx, total int, deckIntent string) string {
	role := pptxArtifactSlideRole(slide, idx, total)
	if deckIntent == "concise-reference-style-learning" {
		switch {
		case role == "cover":
			return "Same prompt, reference style intent, and editable visual motifs."
		case idx == 1 || role == "observations":
			return "System, not template."
		case role == "evidence":
			return "The chart stays native and editable."
		}
	}
	return shortenLayoutText(strings.TrimSpace(firstNonEmpty(slide.Subtitle, slide.Content)), 92)
}

func pptxArtifactSlideDisplayBody(slide officegen.Slide, idx, total int, deckIntent string) string {
	role := pptxArtifactSlideRole(slide, idx, total)
	if deckIntent == "concise-reference-style-learning" && role == "closing" {
		return "Carry palette, hierarchy, and spacing into one clear deck system."
	}
	return shortenLayoutText(strings.TrimSpace(firstNonEmpty(slide.Content, slide.Subtitle, firstPPTXArtifactPoint(slide.Points))), 128)
}

func pptxArtifactSlideTakeaway(slide officegen.Slide, idx, total int, deckIntent string) string {
	role := pptxArtifactSlideRole(slide, idx, total)
	if deckIntent == "concise-reference-style-learning" && (role == "observations" || idx == 1) {
		return "Use recurring visual choices as a system, not a literal template."
	}
	value := strings.TrimSpace(firstNonEmpty(slide.Subtitle, slide.Content, firstPPTXArtifactPoint(slide.Points)))
	if value == "" {
		return ""
	}
	return shortenLayoutText(value, 110)
}

func firstPPTXArtifactPoint(points []string) string {
	for _, point := range points {
		if strings.TrimSpace(point) != "" {
			return strings.TrimSpace(point)
		}
	}
	return ""
}

func pptxArtifactSlideVisualIntent(slide officegen.Slide, idx int, deckIntent string, options PPTXBuildOptions) string {
	switch {
	case idx == 0 && strings.TrimSpace(options.ReferenceScanRoot) != "":
		return "Use one reference-derived visual plate as a supporting style signal, with all slide words editable."
	case slide.Chart != nil:
		return "Use a styled editable chart plus concise insight cards."
	case strings.EqualFold(slide.Layout, "closing"):
		if strings.Contains(deckIntent, "reference") {
			return "Use a closing visual only when it reinforces the learned reference style."
		}
		return "Use a restrained closing panel with editable support cards."
	default:
		if strings.Contains(deckIntent, "reference") {
			return "Use native cards and accent rhythm from the reference style brief."
		}
		return "Use native editable shapes and text."
	}
}

func discoverPPTXArtifactVisualAssets(root string, limit int) ([]pptxArtifactVisualAsset, error) {
	if limit <= 0 {
		return nil, nil
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve visual asset root: %w", err)
	}
	var paths []string
	err = filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry == nil {
			return nil
		}
		if entry.IsDir() {
			switch strings.ToLower(entry.Name()) {
			case ".git", "node_modules", "output", "tmp":
				if path != absRoot {
					return filepath.SkipDir
				}
			case ".omx", ".omo", ".claude", ".worktrees":
				if path != absRoot {
					return filepath.SkipDir
				}
			default:
				if strings.HasPrefix(entry.Name(), ".") && path != absRoot {
					return filepath.SkipDir
				}
			}
			if path != absRoot && strings.HasSuffix(strings.ToLower(entry.Name()), "-preview") {
				return filepath.SkipDir
			}
			return nil
		}
		if isArtifactVisualAsset(path) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk visual asset root: %w", err)
	}
	sort.Strings(paths)
	paths = prioritizePPTXArtifactVisualAssets(paths, limit)
	assets := make([]pptxArtifactVisualAsset, 0, len(paths))
	for _, path := range paths {
		asset := pptxArtifactVisualAsset{
			Path: path,
			Name: filepath.Base(path),
			MIME: artifactVisualAssetMIME(path),
		}
		if info, err := os.Stat(path); err == nil {
			asset.SizeBytes = info.Size()
		}
		if file, err := os.Open(path); err == nil {
			if cfg, _, err := image.DecodeConfig(file); err == nil {
				asset.Width = cfg.Width
				asset.Height = cfg.Height
			}
			_ = file.Close()
		}
		assets = append(assets, asset)
	}
	return assets, nil
}

func prioritizePPTXArtifactVisualAssets(paths []string, limit int) []string {
	if limit <= 0 || len(paths) == 0 {
		return nil
	}
	groups := make(map[string][]string)
	dirs := make([]string, 0)
	scores := make(map[string]float64, len(paths))
	scoreFor := func(path string) float64 {
		if score, ok := scores[path]; ok {
			return score
		}
		score := artifactVisualAssetQualityScore(path)
		scores[path] = score
		return score
	}
	for _, path := range paths {
		dir := filepath.Dir(path)
		if _, ok := groups[dir]; !ok {
			dirs = append(dirs, dir)
		}
		groups[dir] = append(groups[dir], path)
	}
	for dir := range groups {
		sort.SliceStable(groups[dir], func(i, j int) bool {
			leftRank := artifactVisualAssetFileRank(groups[dir][i])
			rightRank := artifactVisualAssetFileRank(groups[dir][j])
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			leftScore := scoreFor(groups[dir][i])
			rightScore := scoreFor(groups[dir][j])
			if leftScore != rightScore {
				return leftScore > rightScore
			}
			return strings.ToLower(filepath.ToSlash(groups[dir][i])) < strings.ToLower(filepath.ToSlash(groups[dir][j]))
		})
	}
	sort.SliceStable(dirs, func(i, j int) bool {
		leftRank := artifactVisualAssetDirRank(dirs[i])
		rightRank := artifactVisualAssetDirRank(dirs[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftScore := 0.0
		rightScore := 0.0
		if len(groups[dirs[i]]) > 0 {
			leftScore = scoreFor(groups[dirs[i]][0])
		}
		if len(groups[dirs[j]]) > 0 {
			rightScore = scoreFor(groups[dirs[j]][0])
		}
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return strings.ToLower(filepath.ToSlash(dirs[i])) < strings.ToLower(filepath.ToSlash(dirs[j]))
	})
	out := make([]string, 0, artifactMinInt(limit, len(paths)))
	for offset := 0; len(out) < limit; offset++ {
		added := false
		for _, dir := range dirs {
			group := groups[dir]
			if offset >= len(group) {
				continue
			}
			out = append(out, group[offset])
			added = true
			if len(out) >= limit {
				break
			}
		}
		if !added {
			break
		}
	}
	if limit >= 4 && !pptxArtifactHasNonCoverAsset(out) {
		secondaryCandidates := make([]string, 0)
		for _, dir := range dirs {
			for _, candidate := range groups[dir] {
				if artifactVisualAssetIsCover(candidate) || pptxArtifactPathInList(out, candidate) {
					continue
				}
				secondaryCandidates = append(secondaryCandidates, candidate)
			}
		}
		sort.SliceStable(secondaryCandidates, func(i, j int) bool {
			leftRank := artifactVisualAssetSecondaryRank(secondaryCandidates[i])
			rightRank := artifactVisualAssetSecondaryRank(secondaryCandidates[j])
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			leftScore := scoreFor(secondaryCandidates[i])
			rightScore := scoreFor(secondaryCandidates[j])
			if leftScore != rightScore {
				return leftScore > rightScore
			}
			return strings.ToLower(filepath.ToSlash(secondaryCandidates[i])) < strings.ToLower(filepath.ToSlash(secondaryCandidates[j]))
		})
		if len(secondaryCandidates) > 0 {
			if len(out) == 0 {
				out = append(out, secondaryCandidates[0])
			} else {
				out[len(out)-1] = secondaryCandidates[0]
			}
		}
	}
	return out
}

func pptxArtifactHasNonCoverAsset(paths []string) bool {
	for _, path := range paths {
		if !artifactVisualAssetIsCover(path) {
			return true
		}
	}
	return false
}

func pptxArtifactPathInList(paths []string, candidate string) bool {
	for _, path := range paths {
		if path == candidate {
			return true
		}
	}
	return false
}

func artifactVisualAssetIsCover(path string) bool {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	return name == "cover" || strings.Contains(name, "cover") || strings.Contains(name, "hero")
}

func artifactVisualAssetSecondaryRank(path string) int {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	switch {
	case name == "image-02":
		return 0
	case name == "image-03":
		return 1
	case strings.HasPrefix(name, "image-") && name != "image-01":
		return 2
	case strings.HasPrefix(name, "image-"):
		return 3
	default:
		return 4
	}
}

func artifactVisualAssetQualityScore(path string) float64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return 0
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return 0
	}
	stepX := artifactVisualSampleStep(width)
	stepY := artifactVisualSampleStep(height)
	minLum := 255.0
	maxLum := 0.0
	saturationSum := 0.0
	samples := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r16, g16, b16, a16 := img.At(x, y).RGBA()
			if a16 < 0x4000 {
				continue
			}
			r := float64(r16 >> 8)
			g := float64(g16 >> 8)
			b := float64(b16 >> 8)
			lum := 0.2126*r + 0.7152*g + 0.0722*b
			if lum < minLum {
				minLum = lum
			}
			if lum > maxLum {
				maxLum = lum
			}
			maxRGB := artifactMaxFloat(r, artifactMaxFloat(g, b))
			minRGB := artifactMinFloat(r, artifactMinFloat(g, b))
			saturationSum += maxRGB - minRGB
			samples++
		}
	}
	if samples == 0 {
		return 0
	}
	return (maxLum - minLum) + saturationSum/float64(samples)
}

func artifactVisualSampleStep(size int) int {
	if size <= 32 {
		return 1
	}
	step := size / 32
	if step < 1 {
		return 1
	}
	return step
}

func artifactVisualAssetDirRank(dir string) int {
	normalized := strings.ToLower(filepath.ToSlash(dir))
	switch {
	case strings.Contains(normalized, "/参考文章/"):
		return 0
	case strings.Contains(normalized, "/reference") || strings.Contains(normalized, "/refs/"):
		return 1
	default:
		return 2
	}
}

func artifactVisualAssetFileRank(path string) int {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	switch {
	case name == "cover":
		return 0
	case strings.Contains(name, "cover") || strings.Contains(name, "hero"):
		return 1
	case strings.HasPrefix(name, "image-"):
		return 2
	default:
		return 3
	}
}

func isArtifactVisualAsset(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	default:
		return false
	}
}

func artifactVisualAssetMIME(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func representativeReferenceFiles(options PPTXBuildOptions) []string {
	if options.ReferenceProfile == nil {
		return nil
	}
	out := make([]string, 0, 3)
	for _, file := range options.ReferenceProfile.SourceFiles {
		if strings.TrimSpace(file.FailedReason) != "" || strings.TrimSpace(file.Path) == "" {
			continue
		}
		if !isStableArtifactReferenceBucket(file.SourceBucket) {
			continue
		}
		out = append(out, file.Path)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func isStableArtifactReferenceBucket(bucket string) bool {
	switch strings.TrimSpace(bucket) {
	case "", "other", "demo-assets":
		return true
	default:
		return false
	}
}

func runPPTXArtifactWorkerDefault(ctx context.Context, request pptxArtifactWorkerRequest, workDir string) (*pptxArtifactWorkerOutput, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}
	if err := ensureArtifactToolResolution(workDir); err != nil {
		return nil, err
	}
	scriptPath := filepath.Join(workDir, "pptx_artifact_worker.mjs")
	requestPath := filepath.Join(workDir, "request.json")
	responsePath := filepath.Join(workDir, "response.json")
	if err := os.WriteFile(scriptPath, []byte(pptxArtifactWorkerScript), 0o644); err != nil {
		return nil, fmt.Errorf("write artifact worker script: %w", err)
	}
	requestData, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal artifact worker request: %w", err)
	}
	if err := os.WriteFile(requestPath, requestData, 0o644); err != nil {
		return nil, fmt.Errorf("write artifact worker request: %w", err)
	}
	nodePath, err := resolveArtifactWorkerNode()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, nodePath, scriptPath, requestPath, responsePath)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run node artifact worker: %w: %s", err, strings.TrimSpace(string(output)))
	}
	responseData, err := os.ReadFile(responsePath)
	if err != nil {
		return nil, fmt.Errorf("read artifact worker response: %w", err)
	}
	var response pptxArtifactWorkerOutput
	if err := json.Unmarshal(responseData, &response); err != nil {
		return nil, fmt.Errorf("parse artifact worker response: %w", err)
	}
	response.WorkerDir = workDir
	response.ScriptPath = scriptPath
	response.RequestPath = requestPath
	response.ResponsePath = responsePath
	if strings.TrimSpace(response.InspectPath) == "" {
		response.InspectPath = request.InspectPath
	}
	return &response, nil
}

func resolveArtifactWorkerNode() (string, error) {
	if value := strings.TrimSpace(os.Getenv("OFFICECLI_PPTX_ARTIFACT_NODE")); value != "" {
		info, err := os.Stat(value)
		if err != nil || info.IsDir() {
			return "", fmt.Errorf("OFFICECLI_PPTX_ARTIFACT_NODE does not point to an executable file: %s", value)
		}
		return value, nil
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", fmt.Errorf("node executable was not found; set OFFICECLI_PPTX_ARTIFACT_NODE to enable pptx backend %q", PPTXBackendArtifactWorker)
	}
	return nodePath, nil
}

func ensureArtifactToolResolution(workDir string) error {
	nodeModules, err := resolveArtifactNodeModules()
	if err != nil {
		return err
	}
	if nodeModules == "" {
		return nil
	}
	linkPath := filepath.Join(workDir, "node_modules")
	if _, err := os.Lstat(linkPath); err == nil {
		return nil
	}
	if err := os.Symlink(nodeModules, linkPath); err != nil {
		return fmt.Errorf("link artifact worker node_modules: %w", err)
	}
	return nil
}

func resolveArtifactNodeModules() (string, error) {
	if value := strings.TrimSpace(os.Getenv("OFFICECLI_PPTX_ARTIFACT_NODE_MODULES")); value != "" {
		if hasArtifactToolPackage(value) {
			return value, nil
		}
		return "", fmt.Errorf("OFFICECLI_PPTX_ARTIFACT_NODE_MODULES does not contain @oai/artifact-tool: %s", value)
	}
	candidates := candidateNodeModules()
	for _, candidate := range candidates {
		if hasArtifactToolPackage(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("@oai/artifact-tool was not found; set OFFICECLI_PPTX_ARTIFACT_NODE_MODULES to a node_modules directory that contains it")
}

func candidateNodeModules() []string {
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		for {
			candidates = append(candidates, filepath.Join(cwd, "node_modules"))
			parent := filepath.Dir(cwd)
			if parent == cwd {
				break
			}
			cwd = parent
		}
	}
	if nodePath, err := resolveArtifactWorkerNode(); err == nil {
		binDir := filepath.Dir(nodePath)
		candidates = append(candidates,
			filepath.Join(filepath.Dir(binDir), "node_modules"),
			filepath.Join(filepath.Dir(filepath.Dir(binDir)), "node_modules"),
		)
	}
	return candidates
}

func hasArtifactToolPackage(nodeModules string) bool {
	if strings.TrimSpace(nodeModules) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(nodeModules, "@oai", "artifact-tool"))
	return err == nil && info.IsDir()
}

const pptxArtifactWorkerScript = `
import fs from "node:fs/promises";
import path from "node:path";
import { FileBlob, Presentation, PresentationFile } from "@oai/artifact-tool";

const W = 1280;
const H = 720;
const [requestPath, responsePath] = process.argv.slice(2);
if (!requestPath || !responsePath) {
  throw new Error("usage: node pptx_artifact_worker.mjs <request.json> <response.json>");
}

const request = JSON.parse(await fs.readFile(requestPath, "utf8"));
await fs.mkdir(path.dirname(request.outputPptx), { recursive: true });
await fs.mkdir(request.previewDir, { recursive: true });

const presentation = Presentation.create({ slideSize: { width: W, height: H } });
const colors = resolveColors(request.stylePreset, request.theme || {});
const visualAssets = Array.isArray(request.visualAssets) ? request.visualAssets.filter((item) => item && item.path) : [];
const repairMode = String(request.repairMode || "");
const simplifiedLayout = repairMode === "simplified" || repairMode === "minimal";
const polishMode = repairMode === "polish";
const strictVisualQuality = Boolean(request.strictVisualQuality);
let currentSlideNumber = 0;
const inspect = {
  backend: "artifact-experimental",
  workerVersion: "artifact-experimental-v2",
  repairMode,
  designPlan: request.designPlan || null,
  slidePlans: [],
  importedReferences: [],
  visualAssets: visualAssets.map((item) => ({
    path: item.path,
    name: item.name || "",
    slide: Number(item.slide || 0),
    frame: item.frame || null,
    targetAspectRatio: Number(item.targetAspectRatio || 0),
    sourceAspectRatio: Number(item.sourceAspectRatio || 0),
    textDetection: item.textDetection || null,
    visualSignal: item.visualSignal || null,
    width: item.width || 0,
    height: item.height || 0
  })),
  editableItems: [],
  visualItems: [],
  nativeCharts: [],
  images: [],
  previews: [],
  visualVerdict: null
};

for (const referencePath of request.referenceFiles || []) {
  try {
    await PresentationFile.importPptx(await FileBlob.load(referencePath));
    inspect.importedReferences.push({ path: referencePath, status: "imported" });
  } catch (error) {
    inspect.importedReferences.push({ path: referencePath, status: "failed", error: String(error && error.message || error) });
  }
}

for (let i = 0; i < request.slides.length; i++) {
  const data = request.slides[i] || {};
  const slide = presentation.slides.add();
  slide.background.fill = colors.background;
  await buildSlide(slide, data, i, request.slides.length, colors, pickAsset(i));
}

for (let i = 0; i < presentation.slides.count; i++) {
  const slide = presentation.slides.getItem(i);
  const blob = await presentation.export({ slide, format: "png", scale: 1 });
  const bytes = Buffer.from(await blob.arrayBuffer());
  const previewPath = path.join(request.previewDir, "slide-" + String(i + 1).padStart(2, "0") + ".png");
  await fs.writeFile(previewPath, bytes);
  inspect.previews.push(previewPath);
}

inspect.visualVerdict = buildVisualVerdict();
await fs.writeFile(request.inspectPath, JSON.stringify(inspect, null, 2));
const pptx = await PresentationFile.exportPptx(presentation);
await pptx.save(request.outputPptx);

const warnings = [];
if (inspect.importedReferences.some((item) => item.status === "failed")) {
  warnings.push("Some reference PPTX files could not be imported by the artifact worker and were used only through the Go reference profile.");
}
if (visualAssets.length > 0 && inspect.images.length === 0) {
  warnings.push("Local visual assets were discovered but could not be embedded by the artifact worker.");
}

await fs.writeFile(responsePath, JSON.stringify({
  outputPptx: request.outputPptx,
  previewFiles: inspect.previews,
  inspectPath: request.inspectPath,
  importedRefs: inspect.importedReferences.filter((item) => item.status === "imported").length,
  editableItems: inspect.editableItems.length,
  nativeCharts: inspect.nativeCharts.length,
  visualAssets: visualAssets.length,
  visualVerdict: inspect.visualVerdict.status,
  visualScore: inspect.visualVerdict.score,
  warnings,
  workerVersion: "artifact-experimental-v2",
  artifactToolOk: true
}, null, 2));

function buildVisualVerdict() {
  const issues = [];
  const bySlide = new Map();
  for (const item of inspect.editableItems || []) {
    const slideNo = Number(item.slide || 0);
    if (!slideNo) continue;
    if (!bySlide.has(slideNo)) bySlide.set(slideNo, []);
    if (!isDensityNeutralTextItem(item) && String(item.text || "").trim()) {
      bySlide.get(slideNo).push(item);
    }
  }
  for (let slideNo = 1; slideNo <= request.slides.length; slideNo++) {
    const items = bySlide.get(slideNo) || [];
    if (items.length === 0) {
      issues.push({
        code: "NO_EDITABLE_TEXT_ON_SLIDE",
        severity: "error",
        slide: slideNo,
        message: "Slide preview has no editable audience-facing text records."
      });
      continue;
    }
    if (items.length > 12) {
      issues.push({
        code: "CONTENT_DENSITY_HIGH",
        severity: "error",
        slide: slideNo,
        message: "Slide has too many editable text blocks for a clean preview."
      });
    }
    const tinyCount = items.filter((item) => Number(item.fontSize || 0) > 0 && Number(item.fontSize || 0) < 13).length;
    if (tinyCount > 2) {
      issues.push({
        code: "TOO_MUCH_SMALL_TEXT",
        severity: "error",
        slide: slideNo,
        message: "Slide relies on too many small text objects."
      });
    }
    const longNarrow = items.some((item) => Array.from(String(item.text || "")).length > 44 && item.bbox && Number(item.bbox.width || 0) < 260);
    if (longNarrow) {
      issues.push({
        code: "NARROW_LONG_TEXT",
        severity: "error",
        slide: slideNo,
        message: "Long copy is placed in a narrow box and is likely to wrap poorly."
      });
    }
    if (usesCodexReferenceLearningRecipe()) {
      for (const item of items) {
        if (!isLowValueReferenceLearningText(item)) continue;
        issues.push({
          code: "LOW_VALUE_REFERENCE_LEARNING_COPY",
          severity: strictVisualQuality ? "error" : "warning",
          slide: slideNo,
          message: "Reference-learning slide copy is too generic and should be rewritten around palette, hierarchy, editable structure, and readable style behavior."
        });
        break;
      }
    }
  }
  const chartSlides = (request.slides || []).filter((slide) => slide && slide.chart).length;
  if (chartSlides > 0 && inspect.nativeCharts.length < chartSlides) {
    issues.push({
      code: "MISSING_NATIVE_CHART",
      severity: "error",
      message: "A chart slide did not produce enough native chart records."
    });
  }
  for (const asset of visualAssets || []) {
    const slideNo = Number(asset && asset.slide || 0);
    if (!slideNo) continue;
    const assetPath = String(asset.path || "");
    const usedImage = (inspect.images || []).find((item) => Number(item && item.slide || 0) === slideNo && String(item && item.path || "") === assetPath && !item.failed);
    const used = Boolean(usedImage);
    if (!used) {
      issues.push({
        code: "MISSING_BOUND_VISUAL_ASSET",
        severity: "error",
        slide: slideNo,
        message: "A slide-bound visual asset was not embedded on its target slide."
      });
      continue;
    }
    const qualityIssues = visualAssetQualityIssues(asset, usedImage);
    for (const issue of qualityIssues) {
      issues.push(issue);
    }
  }
  const errorCount = issues.filter((issue) => String(issue.severity || "").toLowerCase() === "error").length;
  const warningCount = issues.length - errorCount;
  const score = Math.max(0, 96 - errorCount * 18 - warningCount * 5);
  return {
    status: errorCount > 0 || score < 80 ? "fail" : "pass",
    score,
    issues
  };
}

function visualAssetQualityIssues(asset, imageRecord) {
  const issues = [];
  const slideNo = Number(asset && asset.slide || imageRecord && imageRecord.slide || 0);
  const strict = strictVisualQuality || usesCodexReferenceLearningRecipe();
  const width = Number(asset && asset.width || 0);
  const height = Number(asset && asset.height || 0);
  const targetRatio = Number(asset && asset.targetAspectRatio || 0);
  const sourceRatio = Number(asset && asset.sourceAspectRatio || 0);
  const bbox = imageRecord && imageRecord.bbox || asset && asset.frame || null;
  if (width > 0 && height > 0 && Math.min(width, height) < 48) {
    issues.push({
      code: "LOW_RESOLUTION_VISUAL_ASSET",
      severity: strict ? "error" : "warning",
      slide: slideNo,
      message: "A visual plate source image is too small to carry a high-fidelity slide crop."
    });
  }
  if (targetRatio > 0 && sourceRatio > 0) {
    const ratioDrift = Math.abs(Math.log(sourceRatio / targetRatio));
    if (ratioDrift > 0.32) {
      issues.push({
        code: "VISUAL_ASSET_ASPECT_RATIO_MISMATCH",
        severity: strict ? "error" : "warning",
        slide: slideNo,
        message: "A visual plate source ratio differs too much from its planned frame and will crop unpredictably."
      });
    }
  }
  if (bbox) {
    const coverage = (Number(bbox.width || 0) * Number(bbox.height || 0)) / (W * H);
    if (coverage > 0 && coverage < 0.012) {
      issues.push({
        code: "VISUAL_ASSET_COVERAGE_TOO_LOW",
        severity: strict ? "error" : "warning",
        slide: slideNo,
        message: "A slide-bound visual plate occupies too little of the slide to meaningfully affect the composition."
      });
    }
  }
  const visualSignal = asset && asset.visualSignal || null;
  if (visualSignal && String(visualSignal.status || "").toLowerCase() === "low") {
    issues.push({
      code: "LOW_INFORMATION_VISUAL_ASSET",
      severity: strict ? "error" : "warning",
      slide: slideNo,
      message: "A generated visual plate has too little luminance variation and may read as blank or decorative filler."
    });
  }
  return issues;
}

function isDensityNeutralTextItem(item) {
  const role = String(item && item.role || "").toLowerCase();
  if (role === "footer" || role === "observation-index") return true;
  const text = String(item && item.text || "").trim();
  if (/^\d{2}$/.test(text) && Number(item && item.fontSize || 0) <= 20) return true;
  return false;
}

function isLowValueReferenceLearningText(item) {
  const role = String(item && item.role || "").toLowerCase();
  const text = String(item && item.text || "").trim().toLowerCase();
  if (!text) return false;
  const checkedRoles = new Set([
    "observation-takeaway",
    "closing-title",
    "closing-body",
    "closing-next-step-text",
    "planned-subtitle"
  ]);
  if (!checkedRoles.has(role)) return false;
  const strongSignals = [
    "palette",
    "hierarchy",
    "spacing",
    "card rhythm",
    "visual rhythm",
    "editable",
    "reference style",
    "visual system",
    "deck system",
    "readable",
    "template mimicry"
  ];
  if (strongSignals.some((signal) => text.includes(signal))) return false;
	const weakPatterns = [
	"style signals",
	"reference signals",
	"reference cues",
	"learned cues",
	"learned style",
	"builder loop",
	"structured slides",
	"readable density",
	"review discipline",
	"visual qa loop",
	"refine through",
	"coherent deck"
  ];
  return weakPatterns.some((pattern) => text.includes(pattern));
}

function pickAsset(index) {
  if (visualAssets.length === 0) return null;
  const exact = visualAssets.find((item) => Number(item && item.slide || 0) === index + 1);
  if (exact) return exact;
  if (usesCodexReferenceLearningRecipe() && hasSlideBoundVisualAssets()) {
    return null;
  }
  const plan = slidePlan(index);
  if (usesCodexReferenceLearningRecipe() && String(plan && plan.role || "") === "closing") {
    const secondary = visualAssets.find((item) => !isCoverAsset(item));
    if (secondary) return secondary;
  }
  return visualAssets[index % visualAssets.length];
}

function hasSlideBoundVisualAssets() {
  return visualAssets.some((item) => Number(item && item.slide || 0) > 0);
}

function isCoverAsset(asset) {
  if (Number(asset && asset.slide || 0) === 1) return true;
  const name = String(asset && (asset.name || path.basename(asset.path || "")) || "").toLowerCase();
  const base = name.replace(/\.[^.]+$/, "");
  return base === "cover" || base.includes("cover") || base.includes("hero");
}

function slidePlan(index) {
  const plans = request.designPlan && Array.isArray(request.designPlan.slides) ? request.designPlan.slides : [];
  return plans.find((item) => Number(item && item.slide) === index + 1) || null;
}

function builderPatchFor(index) {
  const patches = request.designPlan && request.designPlan.builderPatch && Array.isArray(request.designPlan.builderPatch.slides)
    ? request.designPlan.builderPatch.slides
    : [];
  return patches.find((item) => Number(item && item.slide) === index + 1) || null;
}

function applyDynamicBuilderPatchUnderlay(slide, index, colors) {
  const plan = slidePlan(index);
  if (usesCodexReferenceLearningRecipe()) return;
  const patch = builderPatchFor(index);
  if (!patch) return;
  const backplate = String(patch.backplate || "").trim();
  if (backplate === "right-band") {
    addShape(slide, "rect", 1138, 0, 142, H, colors.accentSoft, "#00000000", 0, { role: "dynamic-builder-backplate", slideNo: index + 1 });
  } else if (backplate === "left-band") {
    addShape(slide, "rect", 0, 0, 142, H, colors.accentSoft, "#00000000", 0, { role: "dynamic-builder-backplate", slideNo: index + 1 });
  } else if (backplate === "top-band") {
    addShape(slide, "rect", 0, 0, W, 72, colors.accentSoft, "#00000000", 0, { role: "dynamic-builder-backplate", slideNo: index + 1 });
  } else if (backplate === "bottom-band") {
    addShape(slide, "rect", 0, 646, W, 74, colors.accentSoft, "#00000000", 0, { role: "dynamic-builder-backplate", slideNo: index + 1 });
  }
  const rail = String(patch.accentRail || "").trim();
  if (rail === "top") {
    addShape(slide, "rect", 72, 36, 420, 7, colors.accent, colors.accent, 0, { role: "dynamic-builder-accent-rail", slideNo: index + 1 });
  } else if (rail === "bottom") {
    addShape(slide, "rect", 72, 626, 420, 7, colors.accent, colors.accent, 0, { role: "dynamic-builder-accent-rail", slideNo: index + 1 });
  } else if (rail === "right") {
    addShape(slide, "rect", 1180, 76, 7, 442, colors.accent, colors.accent, 0, { role: "dynamic-builder-accent-rail", slideNo: index + 1 });
  } else if (rail === "left") {
    addShape(slide, "rect", 36, 76, 7, 442, colors.accent, colors.accent, 0, { role: "dynamic-builder-accent-rail", slideNo: index + 1 });
  }
}

function isObservationPlan(plan, index) {
  const role = String(plan && plan.role || "").trim();
  const layoutMode = String(plan && plan.layoutMode || "").trim();
  return index === 1 || role === "observations" || layoutMode === "observation-cards";
}

function planKicker(index, fallback) {
  const plan = slidePlan(index);
  const value = String(plan && plan.kicker || "").trim();
  const out = value || fallback;
  return usesCodexReferenceLearningRecipe() ? String(out || "").toUpperCase() : out;
}

function planDisplayTitle(index, fallback) {
  const plan = slidePlan(index);
  const value = cleanPlannedVisibleText(String(plan && plan.displayTitle || "").trim(), 58);
  if (usesCodexReferenceLearningRecipe() && String(plan && plan.role || "") === "closing") {
    const stableFallback = referenceLearningClosingTitleFallback();
    if (!value || isLowValueReferenceLearningText({ role: "closing-title", text: value })) return stableFallback;
  }
  return value || fallback;
}

function planDisplaySubtitle(index, fallback) {
  const plan = slidePlan(index);
  const value = cleanPlannedVisibleText(String(plan && plan.displaySubtitle || "").trim(), 72);
  if (usesCodexReferenceLearningRecipe() && (String(plan && plan.role || "") === "observations" || index === 1)) {
    const stableFallback = referenceLearningObservationSubtitleFallback();
    if (!value || isLowValueReferenceLearningText({ role: "planned-subtitle", text: value })) return stableFallback;
  }
  return value || fallback;
}

function planDisplayBody(index, fallback) {
  const plan = slidePlan(index);
  const value = shortSupportText(String(plan && plan.displayBody || "").trim(), 86);
  if (usesCodexReferenceLearningRecipe() && String(plan && plan.role || "") === "closing") {
    const stableFallback = referenceLearningClosingBodyFallback();
    if (!value || isLowValueReferenceLearningText({ role: "closing-body", text: value })) return stableFallback;
  }
  return value || fallback;
}

function planTakeaway(index, fallback) {
  const plan = slidePlan(index);
  const value = String(plan && plan.takeaway || "").trim();
  if (usesCodexReferenceLearningRecipe() && (String(plan && plan.role || "") === "observations" || index === 1)) {
    const stableFallback = referenceLearningObservationTakeawayFallback();
    if (!value || isLowValueReferenceLearningText({ role: "observation-takeaway", text: value })) return stableFallback;
  }
  return value || fallback;
}

function referenceLearningClosingTitleFallback() {
  return "Reference style becomes a reusable system.";
}

function referenceLearningClosingBodyFallback() {
  return "Carry palette, hierarchy, spacing, and visual rhythm into one clear editable deck system.";
}

function referenceLearningObservationTakeawayFallback() {
  return "Use recurring visual choices as a system, not a literal template.";
}

function referenceLearningObservationSubtitleFallback() {
  return "System, not template.";
}

function planCards(index, field) {
  const plan = slidePlan(index);
  const raw = Array.isArray(plan && plan[field]) ? plan[field] : [];
  const out = [];
  for (const item of raw) {
    const clean = field === "cards" ? cleanPlannedCardText : cleanSupportText;
    const heading = clean(String(item && item.heading || "").trim());
    const detail = clean(String(item && item.detail || "").trim());
    if (!heading && !detail) continue;
    out.push({ heading, detail });
    if (out.length >= 3) break;
  }
  return out;
}

function compositionFor(index, fallback) {
  const plan = slidePlan(index);
  const value = String(plan && plan.composition || "").trim();
  return value || fallback;
}

function layoutModeFor(index, data) {
  const plan = slidePlan(index);
  const planned = String(plan && plan.layoutMode || "").trim();
  if (data.chart && planned !== "chart-insight-stack") return "chart-insight-stack";
  if ((index === 0 || data.isTitle || data.layout === "title") && planned !== "cover-split-visual") return "cover-split-visual";
  if (data.layout === "closing" && planned !== "closing-takeaway") return "closing-takeaway";
  if (planned) return planned;
  if (index === 0 || data.isTitle || data.layout === "title") return "cover-split-visual";
  if (data.layout === "closing") return "closing-takeaway";
  if (data.chart) return "chart-insight-stack";
  if (isObservationSlide(data)) return "observation-cards";
  if ((data.metrics || []).length) return "metric-cards";
  if ((data.sections || []).length) return "section-cards";
  return "content-cards";
}

async function buildSlide(slide, data, index, total, colors, asset) {
  currentSlideNumber = index + 1;
  addBackground(slide, colors, index);
  applyDynamicBuilderPatchUnderlay(slide, index, colors);
  const plan = slidePlan(index);
  const layoutMode = layoutModeFor(index, data);
  const composition = compositionFor(index, "standard");
  inspect.slidePlans.push({
    slide: index + 1,
    role: plan && plan.role || "",
    layoutMode,
    composition,
    visualTreatment: plan && plan.visualTreatment || "",
    densityTarget: plan && plan.densityTarget || "",
    kicker: plan && plan.kicker || "",
    displayTitle: plan && plan.displayTitle || "",
    displaySubtitle: plan && plan.displaySubtitle || "",
    displayBody: plan && plan.displayBody || "",
    takeaway: plan && plan.takeaway || "",
    visualIntent: plan && plan.visualIntent || "",
    cards: Array.isArray(plan && plan.cards) ? plan.cards.length : 0,
    chartCallouts: Array.isArray(plan && plan.chartCallouts) ? plan.chartCallouts.length : 0
  });
  if (layoutMode === "cover-split-visual") {
    await buildCover(slide, data, colors, index, asset);
  } else if (layoutMode === "closing-takeaway") {
    await buildClosingSlide(slide, data, colors, index, asset);
  } else if (layoutMode === "chart-insight-stack" && data.chart) {
    if (simplifiedLayout) buildSimpleChartSlide(slide, data, colors, index);
    else buildChartSlide(slide, data, colors, index);
  } else if (layoutMode === "observation-cards") {
    await buildObservationSlide(slide, data, colors, index, asset);
  } else if (layoutMode === "metric-cards" && (data.metrics || []).length) {
    if (simplifiedLayout) buildSimpleContentSlide(slide, data, colors, index);
    else buildMetricSlide(slide, data, colors, index);
  } else if (layoutMode === "section-cards" && (data.sections || []).length) {
    if (simplifiedLayout) buildSimpleContentSlide(slide, data, colors, index);
    else buildSectionSlide(slide, data, colors, index);
  } else {
    if (simplifiedLayout) buildSimpleContentSlide(slide, data, colors, index);
    else await buildContentSlide(slide, data, colors, index, asset);
  }
  addText(slide, String(index + 1).padStart(2, "0") + " / " + String(total).padStart(2, "0"), 1080, 650, 110, 24, {
    role: "footer",
    fontSize: 12,
    color: colors.muted,
    typeface: "Aptos",
    align: "right"
  });
}

function addBackground(slide, colors, index) {
  addShape(slide, "rect", 0, 0, W, H, colors.background, "#00000000", 0, { role: "background", slideNo: index + 1 });
  addShape(slide, "rect", 0, 0, 9, H, index % 2 === 0 ? colors.accent : colors.primary, "#00000000", 0, { role: "accent-rule", slideNo: index + 1 });
}

async function buildCover(slide, data, colors, index, asset) {
  addPanel(slide, 88, 82, 620, 450, colors.surface, colors.border, 11000, { role: "cover-copy" });
  const title = planDisplayTitle(0, data.title || "Untitled");
  const titleSize = title.length > 30 ? 38 : (title.length > 18 ? 40 : 44);
  addText(slide, title, 126, 126, 550, 148, {
    role: "title",
    fontSize: titleSize,
    bold: true,
    color: colors.title,
    typeface: "Aptos Display"
  });
  const subtitle = planDisplaySubtitle(0, data.subtitle || data.content || "");
  addText(slide, subtitle, 130, 300, 520, 70, { role: "subtitle", fontSize: 19, color: colors.body, typeface: "Aptos" });
  const cards = coverSignalCards(data);
  for (let i = 0; i < cards.length; i++) {
    const accent = i === 1 ? colors.primary : (i === 2 ? "#F59E0B" : colors.accent);
    addMiniCard(slide, 130 + i * 172, 400, 148, 86, cards[i].title, cards[i].body, colors, accent);
  }
  addPanel(slide, 736, 74, 410, 468, colors.accentSoft, colors.accent, 12000, { role: "visual-frame" });
  if (asset) {
    await addImagePlate(slide, asset, 780, 118, 320, 250, "Reference visual plate", index + 1);
    addText(slide, "Style signals", 784, 392, 190, 28, { role: "reference-note-title", fontSize: 21, bold: true, color: colors.title, typeface: "Aptos Display" });
    addText(slide, referenceStyleSignalText(colors), 786, 432, 296, 56, { role: "reference-note-body", fontSize: 15, color: colors.body, typeface: "Aptos" });
  } else {
    addFallbackVisualMotif(slide, colors);
  }
}

function coverSignalCards(data) {
  if (usesCodexReferenceLearningRecipe()) {
    return [
      { title: "Style cues", body: "" },
      { title: "Chart signal", body: "" },
      { title: "Readable flow", body: "" }
    ];
  }
  const out = [];
  for (const point of data.points || []) {
    const text = cleanObservationItem(point, "");
    if (!text) continue;
    out.push({ title: shortCardText(text, 22), body: "" });
    if (out.length >= 3) return out;
  }
  for (const section of data.sections || []) {
    const normalized = normalizeSectionItem(section, out.length);
    out.push({ title: shortCardText(normalized.heading, 22), body: shortCardText(normalized.detail, 30) });
    if (out.length >= 3) return out;
  }
  return [
    { title: "Style profile", body: "" },
    { title: "Card rhythm", body: "" },
    { title: "Readable flow", body: "" }
  ];
}

function referenceStyleSignalText(colors) {
  if (colors && !colors.darkMode) {
    return "White canvas, soft cards, teal accents.";
  }
  const brief = request.styleBrief || {};
  const text = String(brief.paletteIntent || brief.typographyIntent || brief.layoutRhythm || "").trim();
  if (text.length > 82) {
    return "Dark canvas, card rhythm, accent hierarchy.";
  }
  const candidate = shortCardText(text, 82);
  if (candidate && !isDanglingObservationFragment(candidate) && !endsWithWeakAdjectiveText(candidate)) {
    return candidate;
  }
  return "Dark canvas, card rhythm, accent hierarchy.";
}

function addFallbackVisualMotif(slide, colors) {
  const depthFill = colors.darkMode ? "#0B3A4A" : "#DDF7F4";
  const warmFill = colors.darkMode ? "#F5B84B" : "#FEF3C7";
  addShape(slide, "ellipse", 970, 94, 178, 178, depthFill, "#00000000", 0, { role: "fallback-motif-depth-disc" });
  addPanel(slide, 790, 112, 316, 258, colors.surface, colors.border, 9000, { role: "fallback-motif-signal-panel" });
  addShape(slide, "rect", 810, 132, 128, 184, warmFill, "#00000000", 0, { role: "fallback-motif-warm-field" });
  addShape(slide, "rect", 938, 132, 148, 184, colors.accentSoft, "#00000000", 0, { role: "fallback-motif-cool-field" });
  addShape(slide, "rect", 826, 150, 76, 46, colors.background, colors.border, 1, { role: "fallback-motif-source-card" });
  addShape(slide, "rect", 846, 214, 74, 42, colors.background, colors.border, 1, { role: "fallback-motif-source-card" });
  addShape(slide, "rect", 968, 154, 74, 42, colors.background, colors.border, 1, { role: "fallback-motif-system-node" });
  addShape(slide, "rect", 1006, 224, 68, 38, colors.background, colors.border, 1, { role: "fallback-motif-system-node" });
  for (let i = 0; i < 5; i++) {
    addShape(slide, "rect", 902 + i * 24, 286 - i * 25, 44, 8, i % 2 === 0 ? colors.accent : colors.primary, "#00000000", 0, { role: "fallback-motif-diagonal-flow" });
  }
  addShape(slide, "rect", 1036, 160, 44, 8, colors.accent, "#00000000", 0, { role: "fallback-motif-signal-line" });
  addShape(slide, "rect", 1036, 182, 30, 8, colors.primary, "#00000000", 0, { role: "fallback-motif-signal-line" });
  addShape(slide, "ellipse", 986, 286, 18, 18, colors.accent, "#00000000", 0, { role: "fallback-motif-system-node" });
  addShape(slide, "ellipse", 1022, 296, 14, 14, colors.primary, "#00000000", 0, { role: "fallback-motif-system-node" });
  addText(slide, "Style signals", 794, 398, 190, 28, { role: "reference-note-title", fontSize: 21, bold: true, color: colors.title, typeface: "Aptos Display" });
  addText(slide, referenceStyleSignalText(colors), 796, 438, 286, 58, { role: "reference-note-body", fontSize: 15, color: colors.body, typeface: "Aptos" });
}

function buildChartSlide(slide, data, colors, index) {
  addHeader(slide, data, colors, index);
  addReferenceLearningChartSubtitle(slide, colors);
  addText(slide, planKicker(index, "EVIDENCE SNAPSHOT"), 76, 62, 180, 24, { role: "chart-kicker", fontSize: 14, bold: true, color: colors.accent, typeface: "Aptos" });
  addPanel(slide, 72, 214, 748, 384, colors.surface, colors.border, 10000, { role: "chart-panel" });
  addNativeChart(slide, data.chart, 112, 260, 668, 280, colors, index);
  addChartInsightStack(slide, data, colors, index);
  if (!usesCodexReferenceLearningRecipe()) addChartSummaryStrip(slide, data.chart, colors);
}

function buildSimpleChartSlide(slide, data, colors, index) {
  addHeader(slide, data, colors, index);
  addReferenceLearningChartSubtitle(slide, colors);
  addText(slide, planKicker(index, "EVIDENCE SNAPSHOT"), 76, 62, 180, 24, { role: "chart-kicker", fontSize: 14, bold: true, color: colors.accent, typeface: "Aptos" });
  addPanel(slide, 76, 218, 760, 372, colors.surface, colors.border, 10000, { role: "chart-panel-simple" });
  addNativeChart(slide, data.chart, 120, 266, 672, 262, colors, index);
  const top = chartTopSignal(data.chart || {});
  const referenceInsights = isReferenceLearningDeck() ? referenceLearningChartInsights().map((item, i) => ({
    label: item.label,
    body: item.body,
    accent: i === 1 ? "#F59E0B" : colors.accent,
    fill: i === 1 ? "#221A0F" : colors.surface,
    border: i === 1 ? "#8B5A12" : colors.border,
    titleColor: i === 1 ? "#FBBF24" : colors.title
  })) : [];
  const defaultInsights = referenceInsights.length > 0 ? referenceInsights : [
    { label: "What it means", body: "Let repeated reference cues guide emphasis.", accent: colors.accent, fill: colors.surface, border: colors.border, titleColor: colors.title },
    { label: top.label, body: top.body, accent: "#F59E0B", fill: "#221A0F", border: "#8B5A12", titleColor: "#FBBF24" },
    { label: "Next focus", body: "Labels stay clear and restrained.", accent: colors.primary, fill: colors.surface, border: colors.border, titleColor: colors.title }
  ];
  const plannedCallouts = isReferenceLearningDeck() ? [] : planCards(index, "chartCallouts").slice(0, 2);
  const compactInsights = plannedCallouts.map((item, i) => ({
    label: item.heading || defaultInsights[i % defaultInsights.length].label,
    body: item.detail || defaultInsights[i % defaultInsights.length].body,
    accent: i === 1 ? "#F59E0B" : (i === 2 ? colors.primary : colors.accent),
    fill: i === 1 ? "#221A0F" : colors.surface,
    border: i === 1 ? "#8B5A12" : colors.border,
    titleColor: i === 1 ? "#FBBF24" : colors.title
  }));
  if (compactInsights.length === 0) {
    compactInsights.push(...defaultInsights);
  }
  for (let i = 0; i < compactInsights.length; i++) {
    const item = compactInsights[i];
    const y = 238 + i * 96;
    addPanel(slide, 858, y, 274, 70, item.fill, item.border, 8000, { role: "chart-compact-insight-card" });
    addShape(slide, "rect", 858, y, 7, 70, item.accent, item.accent, 0, { role: "chart-compact-insight-accent" });
    addText(slide, item.label, 888, y + 14, 210, 22, { role: "chart-compact-insight-label", fontSize: 17, bold: true, color: item.titleColor, typeface: "Aptos Display" });
    addText(slide, item.body, 890, y + 42, 210, 20, { role: "chart-compact-insight-body", fontSize: 13, color: colors.body, typeface: "Aptos" });
  }
  if (!usesCodexReferenceLearningRecipe()) addChartSummaryStrip(slide, data.chart, colors);
}

function addReferenceLearningChartSubtitle(slide, colors) {
  if (!usesCodexReferenceLearningRecipe()) return;
  addText(slide, "A styled evidence panel makes reference signals easy to scan.", 74, 154, 680, 36, { role: "chart-subtitle", fontSize: 18, color: colors.body, typeface: "Aptos" });
}

async function buildContentSlide(slide, data, colors, index, asset) {
  addHeader(slide, data, colors, index);
  const points = data.points || splitContent(data.content);
  if (isObservationSlide(data)) {
    addObservationCards(slide, points, colors, index);
  } else if (asset) {
    addPanel(slide, 790, 164, 340, 330, colors.accentSoft, colors.accent, 11000, { role: "side-visual-frame" });
    await addImagePlate(slide, asset, 822, 200, 276, 206, "Supporting local visual", index + 1);
    addPointCards(slide, points, 82, 190, 610, colors, index);
  } else {
    addPointCards(slide, points, 82, 190, 720, colors, index);
  }
}

async function buildObservationSlide(slide, data, colors, index, asset) {
  addText(slide, planKicker(index, "KEY OBSERVATIONS"), 76, 62, 180, 24, { role: "observation-kicker", fontSize: 14, bold: true, color: colors.accent, typeface: "Aptos" });
  if (usesCodexReferenceLearningRecipe()) {
    addText(slide, planDisplayTitle(index, data.title || "Key observations"), 72, 86, asset ? 640 : 760, 55, {
      role: "heading",
      fontSize: 32,
      bold: true,
      color: colors.title,
      typeface: "Aptos Display"
    });
    const subtitle = planDisplaySubtitle(index, "");
    if (subtitle) {
      addText(slide, subtitle, 74, 148, 560, 30, { role: "planned-subtitle", fontSize: 17, color: colors.body, typeface: "Aptos" });
    }
  } else {
    addHeader(slide, data, colors, index);
  }
  if (asset) {
    addPanel(slide, 916, 72, 212, 128, colors.accentSoft, colors.border, 7500, { role: "observation-visual-panel" });
    await addImagePlate(slide, asset, 936, 90, 172, 92, "Observation visual plate", index + 1);
  }
  addObservationCards(slide, observationPoints(data), colors, index);
  const takeaway = cleanSupportText(planTakeaway(index, data.content || ""));
  if (takeaway) {
    addPanel(slide, 184, 548, 874, 58, colors.accentSoft, colors.border, 6500, { role: "observation-takeaway-panel" });
    addText(slide, takeaway, 220, 562, 802, 32, { role: "observation-takeaway", fontSize: 15, bold: true, color: colors.title, typeface: "Aptos" });
  }
}

function observationPoints(data) {
  const points = Array.isArray(data.points) ? data.points.filter(Boolean) : [];
  if (points.length) return points;
  const sectionPoints = (data.sections || []).map((section) => {
    const normalized = normalizeSectionItem(section, 0);
    return normalized.detail || normalized.heading;
  }).filter(Boolean);
  if (sectionPoints.length) return sectionPoints;
  return splitContent(data.content).filter(Boolean);
}

function buildSimpleContentSlide(slide, data, colors, index) {
  addHeader(slide, data, colors, index);
  const items = simplifiedContentItems(data);
  for (let i = 0; i < items.length; i++) {
    const y = 226 + i * 126;
    addPanel(slide, 94, y, 940, 88, colors.surface, colors.border, 8500, { role: "simple-content-card" });
    addShape(slide, "rect", 94, y, 7, 88, i === 1 ? "#F59E0B" : colors.accent, colors.accent, 0, { role: "simple-content-accent" });
    addText(slide, items[i].heading, 124, y + 18, 240, 28, { role: "simple-content-heading", fontSize: 19, bold: true, color: colors.title, typeface: "Aptos Display" });
    addText(slide, items[i].detail, 386, y + 20, 590, 34, { role: "simple-content-detail", fontSize: 16, color: colors.body, typeface: "Aptos" });
  }
}

function simplifiedContentItems(data) {
  const out = [];
  for (const section of data.sections || []) {
    const normalized = normalizeSectionItem(section, out.length);
    out.push({ heading: normalized.heading, detail: normalized.detail });
    if (out.length >= 2) return out;
  }
  for (const metric of data.metrics || []) {
    const heading = String(metric && metric.label || "Metric").trim();
    const detail = [metric && metric.value, metric && metric.note].filter(Boolean).join(" | ");
    out.push({ heading: shortCardText(heading, 24), detail: shortCardText(metricNoteText(detail, out.length), 52) });
    if (out.length >= 2) return out;
  }
  const points = (data.points || splitContent(data.content)).filter(Boolean);
  for (const point of points) {
    const clean = cleanObservationItem(point, fallbackObservationDetail(out.length));
    out.push({ heading: shortHeading(clean), detail: shortCardText(observationDetail(clean, shortHeading(clean), out.length), 52) });
    if (out.length >= 2) return out;
  }
  while (out.length < 2) {
    const idx = out.length;
    out.push({ heading: fallbackObservationHeading(idx), detail: fallbackObservationDetail(idx) });
  }
  return out;
}

async function buildClosingSlide(slide, data, colors, index, asset) {
  addShape(slide, "rect", 0, 0, W, H, colors.background, "#00000000", 0, { role: "closing-background" });
  const kicker = planKicker(index, "Recommendation");
  const useSplitCallout = asset || usesCodexReferenceLearningRecipe() || compositionFor(index, "") === "split-callout";
  if (useSplitCallout) {
    addPanel(slide, 74, 74, 616, 496, colors.surface, colors.border, 12000, { role: "closing-copy-panel" });
    addText(slide, kicker, 118, 124, 160, 28, { role: "closing-eyebrow", fontSize: 15, bold: true, color: colors.accent, typeface: "Aptos" });
    addText(slide, planDisplayTitle(index, data.title || "Closing"), 118, 184, 520, 104, { role: "closing-title", fontSize: 38, bold: true, color: colors.title, typeface: "Aptos Display" });
    const body = planDisplayBody(index, data.content || data.subtitle || firstText(data.points) || "");
    addText(slide, body, 120, 318, 492, 74, { role: "closing-body", fontSize: 19, color: colors.body, typeface: "Aptos" });
    const sections = closingSupportSections(data);
    if (sections.length > 0) {
      addPanel(slide, 118, 466, 470, 64, colors.accentSoft, colors.accent, 7000, { role: "closing-next-step" });
      addText(slide, sections[0].heading + ": " + sections[0].detail, 148, 486, 410, 22, { role: "closing-next-step-text", fontSize: 15, bold: true, color: colors.title, typeface: "Aptos" });
    }
    addPanel(slide, 760, 96, 374, 426, colors.accentSoft, colors.border, 11000, { role: "closing-visual-panel" });
    if (asset) {
      await addImagePlate(slide, asset, 784, 120, 326, 226, "Closing visual plate", index + 1);
    } else {
      addEditableClosingMotif(slide, colors, 784, 120, 326, 226);
    }
    addText(slide, closingVisualTitle(data), 794, 392, 250, 28, { role: "closing-visual-title", fontSize: 21, bold: true, color: colors.title, typeface: "Aptos Display" });
    addText(slide, closingVisualSummary(data), 796, 436, 282, 58, { role: "closing-visual-body", fontSize: 15, color: colors.body, typeface: "Aptos" });
    return;
  }
  addPanel(slide, 84, 82, 1096, 500, colors.surface, colors.border, 12000, { role: "closing-panel" });
  addText(slide, kicker, 128, 122, 160, 28, { role: "closing-eyebrow", fontSize: 15, bold: true, color: colors.accent, typeface: "Aptos" });
  addText(slide, planDisplayTitle(index, data.title || "Closing"), 128, 176, 850, 88, { role: "closing-title", fontSize: 40, bold: true, color: colors.title, typeface: "Aptos Display" });
  const body = planDisplayBody(index, data.content || data.subtitle || firstText(data.points) || "");
  addText(slide, body, 130, 300, 820, 82, { role: "closing-body", fontSize: 20, color: colors.body, typeface: "Aptos" });
  const sections = closingSupportSections(data);
  if (sections.length > 0) {
    for (let i = 0; i < sections.length; i++) {
      const x = 130 + i * 428;
      const fill = i === 1 ? "#221A0F" : colors.accentSoft;
      const border = i === 1 ? "#8B5A12" : colors.accent;
      addPanel(slide, x, 432, 392, 76, fill, border, 8000, { role: "closing-support-card" });
      addText(slide, sections[i].heading, x + 24, 446, 330, 22, { role: "closing-support-heading", fontSize: 17, bold: true, color: i === 1 ? "#FBBF24" : colors.title, typeface: "Aptos Display" });
      addText(slide, sections[i].detail, x + 24, 474, 330, 18, { role: "closing-support-detail", fontSize: 13, color: colors.body, typeface: "Aptos" });
    }
  } else {
    const chips = (data.points || []).slice(body ? 1 : 0, body ? 3 : 2);
    const cleanChips = chips.map((item, idx) => cleanObservationItem(item, idx === 0 ? "Use the style system as intent." : "Keep the deck concise.")).filter(Boolean);
    const chipText = cleanChips.length ? cleanChips.join("  |  ") : "Use the style system as intent.  |  Keep the deck concise.";
    addPanel(slide, 130, 448, 860, 68, colors.accentSoft, colors.accent, 10000, { role: "closing-next-step" });
    addText(slide, chipText, 160, 469, 800, 30, { role: "closing-next-step-text", fontSize: 19, bold: true, color: colors.title, typeface: "Aptos" });
  }
}

function addEditableClosingMotif(slide, colors, left, top, width, height) {
  addPanel(slide, left, top, width, height, colors.surface, colors.border, 9000, { role: "closing-motif-frame" });
  addShape(slide, "rect", left + 26, top + 32, 8, height - 64, colors.accent, colors.accent, 0, { role: "closing-motif-rule" });
  for (let i = 0; i < 3; i++) {
    const y = top + 38 + i * 58;
    addShape(slide, "ellipse", left + 58, y + 6, 16, 16, i === 1 ? colors.primary : colors.accent, "#00000000", 0, { role: "closing-motif-dot" });
    addShape(slide, "rect", left + 90, y + 8, width - 134 - i * 26, 10, colors.border, "#00000000", 0, { role: "closing-motif-line" });
    addShape(slide, "rect", left + 90, y + 30, width - 170 + i * 18, 14, i === 1 ? colors.primary : colors.accent, "#00000000", 0, { role: "closing-motif-bar" });
  }
}

function closingVisualSummary(data) {
  const sections = closingSupportSections(data);
  if (sections.length > 1) {
    return shortCardText(sections[1].detail, 92);
  }
  return "Reference signals become an editable visual system across the deck.";
}

function closingVisualTitle(data) {
  const sections = closingSupportSections(data);
  if (usesCodexReferenceLearningRecipe() && sections.length > 1 && sections[1].heading) {
    return shortCardText(sections[1].heading, 32);
  }
  return "Style outcome";
}

function closingSupportSections(data) {
  const out = [];
  const detailLimit = usesCodexReferenceLearningRecipe() ? 52 : 52;
  if (usesCodexReferenceLearningRecipe()) {
    return [
      { heading: "Apply the system", detail: "Reuse palette, spacing, and card rhythm." },
      { heading: "Keep it readable", detail: "Prefer hierarchy over template mimicry." }
    ];
  }
  for (const section of data.sections || []) {
    const normalized = normalizeSectionItem(section, out.length);
    out.push({ heading: shortCardText(normalized.heading, 22), detail: shortCardText(normalized.detail, detailLimit) });
    if (out.length >= 2) return out;
  }
  return [
    { heading: "Apply the system", detail: "Reuse palette, spacing, and card rhythm." },
    { heading: "Keep it readable", detail: "Prefer hierarchy over template mimicry." }
  ];
}

function buildSectionSlide(slide, data, colors, index) {
  addHeader(slide, data, colors, index);
  addSections(slide, data.sections, colors, index);
}

function buildMetricSlide(slide, data, colors, index) {
  addHeader(slide, data, colors, index);
  addMetrics(slide, data.metrics, colors, index);
}

function addHeader(slide, data, colors, index) {
  addText(slide, planDisplayTitle(index, data.title || "Untitled"), 72, 86, 760, 55, {
    role: "heading",
    fontSize: 32,
    bold: true,
    color: colors.title,
    typeface: "Aptos Display"
  });
  const takeaway = cleanSupportText(planDisplaySubtitle(index, data.subtitle || data.content || ""));
  if (takeaway && !(usesCodexReferenceLearningRecipe() && data.chart)) {
    addText(slide, takeaway, 74, 146, 680, 38, { role: "takeaway", fontSize: 17, color: colors.body, typeface: "Aptos" });
  }
}

function cleanSupportText(value) {
  const text = String(value || "").trim();
  if (!text || isDanglingObservationFragment(text) || isLowValueChartInsight(text) || isImplementationChartInsight(text)) {
    return "";
  }
  const finished = finishCardText(text);
  if (!finished || isDanglingObservationFragment(finished)) {
    return "";
  }
  return finished;
}

function shortSupportText(value, maxChars) {
  const text = cleanSupportText(value);
  if (!text) return "";
  if (text.length <= maxChars) return text;
  return shortCardText(text, maxChars);
}

function cleanPlannedVisibleText(value, maxChars) {
  const text = String(value || "").trim();
  if (!text || isDanglingObservationFragment(text) || isLowValueChartInsight(text) || isImplementationChartInsight(text)) {
    return "";
  }
  const candidate = shortCardText(text, maxChars);
  if (!candidate || isDanglingObservationFragment(candidate)) {
    return "";
  }
  return candidate;
}

function cleanPlannedCardText(value) {
  const text = String(value || "").trim();
  if (!text || isDanglingObservationFragment(text) || isImplementationChartInsight(text)) {
    return "";
  }
  const finished = finishCardText(text);
  if (!finished || isDanglingObservationFragment(finished)) {
    return "";
  }
  return finished;
}

function addShape(slide, geometry, left, top, width, height, fill, line, lineWidth, meta) {
  const shape = slide.shapes.add({
    geometry,
    position: { left, top, width, height },
    fill,
    line: { style: "solid", fill: line || "#00000000", width: lineWidth || 0 }
  });
  recordVisualItem(meta, "shape", left, top, width, height);
  return shape;
}

function addPanel(slide, left, top, width, height, fill, line, radius, meta) {
  const panel = slide.shapes.add({
    geometry: "roundRect",
    position: { left, top, width, height },
    fill,
    line: { style: "solid", fill: line || fill, width: 1 },
    adjustmentList: [{ name: "adj", formula: "val " + String(radius || 10000) }]
  });
  recordVisualItem(meta, "panel", left, top, width, height);
  return panel;
}

function recordVisualItem(meta, kind, left, top, width, height) {
  const role = String(meta && meta.role || "").trim();
  if (!role) return;
  inspect.visualItems.push({
    kind,
    slide: currentSlideNumber,
    role,
    bbox: { left, top, width, height }
  });
}

function addText(slide, text, left, top, width, height, style) {
  const value = String(text || "");
  const fontSize = style.fontSize || 18;
  const shape = slide.shapes.add({
    geometry: "rect",
    position: { left, top, width, height },
    fill: "#FFFFFF00",
    line: { style: "solid", fill: "#FFFFFF00", width: 0 }
  });
  shape.text = value;
  shape.text.fontSize = fontSize;
  shape.text.typeface = style.typeface || "Aptos";
  shape.text.color = style.color || "#111827";
  shape.text.bold = !!style.bold;
  shape.text.autoFit = "shrinkText";
  shape.text.insets = { left: 2, right: 2, top: 2, bottom: 2 };
  if (style.align) shape.text.alignment = style.align;
  if (style.valign) shape.text.verticalAlignment = style.valign;
  const lineCount = value ? value.split(/\r\n|\r|\n/).length : 0;
  inspect.editableItems.push({
    kind: "text",
    slide: currentSlideNumber,
    role: style.role || "text",
    text: value,
    textChars: Array.from(value).length,
    textLines: lineCount,
    fontSize,
    bbox: { left, top, width, height }
  });
  return shape;
}

function addMiniCard(slide, x, y, w, h, title, body, colors, accent) {
  addPanel(slide, x, y, w, h, colors.surface, colors.border, 7000, { role: "mini-card" });
  addShape(slide, "rect", x, y, 7, h, accent, accent, 0, { role: "card-accent" });
  addText(slide, title, x + 26, y + 24, w - 52, 28, { role: "card-title", fontSize: 18, bold: true, color: colors.title, typeface: "Aptos Display" });
  if (String(body || "").trim()) {
    addText(slide, body, x + 26, y + 62, w - 52, h - 70, { role: "card-body", fontSize: 14, color: colors.body, typeface: "Aptos" });
  }
}

function addPointCards(slide, points, left, top, width, colors, slideIndex) {
  const items = (points || []).filter(Boolean).slice(0, 4);
  if (items.length === 0) {
    addText(slide, "Key message", left, top, width, 38, { role: "empty-point", fontSize: 20, bold: true, color: colors.title, typeface: "Aptos Display" });
    return;
  }
  for (let i = 0; i < items.length; i++) {
    const y = top + i * 86;
    addPanel(slide, left, y, width, 68, colors.surface, colors.border, 8000, { role: "point-card" });
    addShape(slide, "rect", left, y, 7, 68, i % 2 === 0 ? colors.accent : colors.primary, colors.accent, 0, { role: "point-accent" });
    addText(slide, items[i], left + 24, y + 14, width - 48, 38, { role: "bullet", fontSize: 17, color: colors.body, typeface: "Aptos" });
  }
}

function addChartInsightStack(slide, data, colors, slideIndex) {
  const insights = chartInsightItems(data, slideIndex);
  if (polishMode && isReferenceLearningDeck()) {
    for (let i = 0; i < insights.length; i++) {
      const y = 236 + i * 152;
      const fill = i === 1 ? "#221A0F" : colors.surface;
      const border = i === 1 ? "#8B5A12" : colors.border;
      const accent = i === 1 ? "#F59E0B" : colors.accent;
      addPanel(slide, 862, y, 270, 116, fill, border, 8000, { role: "chart-insight-card" });
      addShape(slide, "rect", 862, y, 7, 116, accent, accent, 0, { role: "chart-insight-accent" });
      addText(slide, insights[i].label, 890, y + 24, 220, 28, { role: "chart-insight-label", fontSize: 21, bold: true, color: i === 1 ? "#FBBF24" : colors.title, typeface: "Aptos Display" });
      addText(slide, insights[i].body, 890, y + 66, 218, 54, { role: "chart-insight-body", fontSize: 15, color: colors.body, typeface: "Aptos" });
    }
    return;
  }
  for (let i = 0; i < insights.length; i++) {
    const y = 238 + i * 118;
    const fill = i === 1 ? "#221A0F" : colors.surface;
    const border = i === 1 ? "#8B5A12" : colors.border;
    const accent = i === 1 ? "#F59E0B" : (i === 0 ? colors.accent : colors.primary);
    addPanel(slide, 858, y, 274, 82, fill, border, 8000, { role: "chart-insight-card" });
    addShape(slide, "rect", 858, y, 7, 82, accent, accent, 0, { role: "chart-insight-accent" });
    addText(slide, insights[i].label, 888, y + 16, 214, 24, { role: "chart-insight-label", fontSize: 18, bold: true, color: i === 1 ? "#FBBF24" : colors.title, typeface: "Aptos Display" });
    addText(slide, insights[i].body, 890, y + 48, 210, 28, { role: "chart-insight-body", fontSize: 13, color: colors.body, typeface: "Aptos" });
  }
}

function chartInsightItems(data, slideIndex) {
  const chart = data.chart || {};
  if (isReferenceLearningDeck()) {
    return referenceLearningChartInsights();
  }
  const planned = planCards(slideIndex, "chartCallouts").slice(0, 2);
  if (planned.length > 0) {
    const out = planned.map((item, i) => ({
      label: item.heading || "Insight",
      body: completeChartInsight(item.detail, fallbackChartInsightBody(i))
    })).filter((item) => item.label || item.body);
    if (out.length === 1) {
      const top = chartTopSignal(chart);
      out.push({ label: top.label, body: top.body });
    }
    if (out.length > 0) {
      return out.slice(0, 2);
    }
  }
  const points = uniqueCleanPoints(data.points || []);
  const top = chartTopSignal(chart);
  return [
    {
      label: "What it means",
      body: completeChartInsight(points[0], "Let repeated reference cues guide emphasis.")
    },
    {
      label: top.label,
      body: top.body
    },
    {
      label: "Next focus",
      body: completeChartInsight(points[1], "Labels stay clear and restrained.")
    }
  ];
}

function strongPlannedChartInsights(slideIndex) {
  const planned = planCards(slideIndex, "chartCallouts").slice(0, 2);
  const out = [];
  for (let i = 0; i < planned.length; i++) {
    const label = strongPlannedHeading(planned[i].heading);
    const body = strongPlannedDetail(planned[i].detail, 8);
    if (!label || !body) continue;
    out.push({ label, body });
  }
  return out;
}

function referenceLearningChartInsights() {
  return [
    {
      label: "Style signal",
      body: "Palette, spacing, hierarchy align."
    },
    {
      label: "Style focus",
      body: "Spacing clarifies chart callouts."
    }
  ];
}

function fillReferenceLearningChartInsights(items) {
  const out = Array.isArray(items) ? items.slice(0, 2) : [];
  const fallback = referenceLearningChartInsights();
  for (const item of fallback) {
    if (out.length >= 2) break;
    const key = String(item.label || "").toLowerCase();
    if (out.some((existing) => String(existing.label || "").toLowerCase() === key)) continue;
    out.push(item);
  }
  return out.slice(0, 2);
}

function fallbackChartInsightBody(index) {
  const bodies = [
    "Let repeated reference cues guide emphasis.",
    "Keep the structure readable and restrained.",
    "Labels stay clear and concise."
  ];
  return bodies[index % bodies.length];
}

function uniqueCleanPoints(points) {
  const seen = new Set();
  const out = [];
  for (const item of points || []) {
    const value = completeChartInsight(item, "").replace(/[.。]\s*$/, "");
    const key = value.toLowerCase();
    if (!value || seen.has(key)) continue;
    seen.add(key);
    out.push(value + ".");
  }
  return out;
}

function completeChartInsight(value, fallback) {
  const text = String(value || "").trim();
  if (!text || isDanglingObservationFragment(text)) return fallback;
  if (isLowValueChartInsight(text)) return fallback;
  if (isImplementationChartInsight(text)) return fallback;
  return text;
}

function isImplementationChartInsight(text) {
  const value = String(text || "").trim();
  if (/^(editable title|editable body|editable text|editable objects|native chart)$/i.test(value)) return true;
  if (/\beditable objects\b/i.test(value)) return true;
  if (/\b(native chart|powerpoint chart|screenshot|bitmap|artifact worker)\b/i.test(value)) return true;
  if (/\b(use|keep|add)\s+(a\s+)?native\b/i.test(value)) return true;
  return false;
}

function isLowValueChartInsight(text) {
  const value = String(text || "").trim().replace(/[.。]\s*$/, "");
  if (!value) return true;
  const words = value.split(/\s+/).filter(Boolean);
  if (words.length <= 3 && /\bis$/i.test(value)) return true;
  if (words.length <= 7 && /\bis\s+(the\s+)?(highest|lowest|higher|lower)(\s+at\s+[-+]?\d+(\.\d+)?)?$/i.test(value)) return true;
  if (words.length <= 5 && /^(theme|layout|content|aggregate|parsed|total|style)\s+is\b/i.test(value)) return true;
  return false;
}

function chartTopSignal(chart) {
  const categories = chart && Array.isArray(chart.categories) ? chart.categories : [];
  const values = chart && Array.isArray(chart.values) ? chart.values : [];
  let bestIndex = -1;
  let bestValue = -Infinity;
  for (let i = 0; i < values.length; i++) {
    const n = Number(values[i]);
    if (Number.isFinite(n) && n > bestValue) {
      bestValue = n;
      bestIndex = i;
    }
  }
  if (bestIndex >= 0 && categories[bestIndex]) {
    return {
      label: "Top signal",
      body: String(categories[bestIndex]) + " leads at " + compactNumber(bestValue) + "."
    };
  }
  return {
    label: "Top signal",
    body: "The chart highlights the strongest reusable reference pattern."
  };
}

function addChartSummaryStrip(slide, chart, colors) {
  const summary = chartSummary(chart);
  addPanel(slide, 112, 548, 668, 34, "#0B1221CC", colors.border, 5000, { role: "chart-summary-strip" });
  addText(slide, summary, 136, 556, 620, 18, { role: "chart-summary-text", fontSize: 14, bold: true, color: colors.body, typeface: "Aptos" });
}

function chartSummary(chart) {
  if (isReferenceLearningDeck()) {
    return "Composition and spacing carry the fidelity signal.";
  }
  const categories = chart && Array.isArray(chart.categories) ? chart.categories : [];
  const values = chart && Array.isArray(chart.values) ? chart.values : [];
  if (!categories.length || !values.length) {
    return "Evidence view | clear labels";
  }
  const count = Math.min(categories.length, values.length);
  const max = Math.max(...values.slice(0, count).map((value) => Number(value)).filter(Number.isFinite));
  if (Number.isFinite(max)) {
    return String(count) + " points | peak " + compactNumber(max);
  }
  return String(count) + " points";
}

function compactNumber(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return String(value || "");
  if (Math.abs(n) >= 1000) return String(Math.round(n / 100) / 10) + "k";
  return String(Math.round(n * 10) / 10).replace(/\.0$/, "");
}

function shortCardText(value, maxChars) {
  const text = String(value || "").trim();
  if (!text) return text;
  if (text.length <= maxChars) return finishCardText(text);
  const clipped = text.slice(0, maxChars).replace(/\s+\S*$/, "").replace(/[,:;，：；-]\s*$/, "").trim();
  return finishCardText(clipped || text.slice(0, maxChars).trim());
}

function finishCardText(value) {
  let text = String(value || "").trim().replace(/[ ,;:，；：]+$/, "");
  const words = text.split(/\s+/).filter(Boolean);
  while (words.length > 1 && isTerminalConnector(words[words.length - 1])) {
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length >= 4 && isWeakTerminalFinalVerb(words[words.length - 1])) {
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length >= 3 && String(words[words.length - 3] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "") === "so" && ["the", "a", "an"].includes(String(words[words.length - 2] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, ""))) {
    words.pop();
    words.pop();
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length >= 2 && isWeakTerminalLeadIn(words[words.length - 2]) && isWeakTerminalNoun(words[words.length - 1])) {
    words.pop();
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length >= 2 && isWeakTerminalPreposition(words[words.length - 2]) && isWeakTerminalNoun(words[words.length - 1])) {
    words.pop();
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length >= 2 && isWeakTerminalVerb(words[words.length - 2]) && isWeakTerminalNoun(words[words.length - 1])) {
    words.pop();
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length >= 2 && isWeakTerminalAdjective(words[words.length - 2]) && isWeakTerminalNoun(words[words.length - 1])) {
    words.pop();
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length >= 2 && isWeakTerminalNounPair(words[words.length - 2], words[words.length - 1])) {
    words.pop();
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length >= 3 && isWeakTerminalVerb(words[words.length - 3]) && ["the", "a", "an"].includes(String(words[words.length - 2] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "")) && (isWeakTerminalNoun(words[words.length - 1]) || isWeakTerminalAdjective(words[words.length - 1]))) {
    words.pop();
    words.pop();
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length > 1 && endsWithArticleAdjective(words)) {
    words.pop();
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length > 4 && isWeakTerminalAdjective(words[words.length - 1])) {
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  while (words.length > 1 && isTerminalConnector(words[words.length - 1])) {
    words.pop();
    text = words.join(" ").replace(/[ ,;:，；：]+$/, "");
  }
  return text;
}

function isTerminalConnector(value) {
  const tail = String(value || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  return [
    "and", "or", "but", "with", "without", "for", "from", "to", "by", "of", "in", "on", "at", "as", "the", "a", "an", "any", "every",
    "while", "because", "although", "though", "before", "after", "then", "if", "when", "where", "whether", "which", "that",
    "is", "are", "was", "were", "through", "into", "instead", "keep", "so", "more", "less"
  ].includes(tail);
}

function endsWithArticleAdjective(words) {
  if (!Array.isArray(words) || words.length < 2) return false;
  const article = String(words[words.length - 2] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  const tail = String(words[words.length - 1] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  if (!["a", "an", "the"].includes(article)) return false;
  return isWeakTerminalAdjective(tail);
}

function isWeakTerminalAdjective(value) {
  const tail = String(value || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  return ["clear", "concise", "strong", "simple", "consistent", "recurring", "readable", "structured", "fresh", "quiet", "clean", "large", "compact", "repeatable", "main", "short", "selective", "calm", "usually", "derived"].includes(tail);
}

function isWeakTerminalLeadIn(value) {
  const tail = String(value || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  return ["when", "where", "how", "why"].includes(tail);
}

function isWeakTerminalPreposition(value) {
  const tail = String(value || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  return ["in", "on", "of", "with", "for", "through", "around"].includes(tail);
}

function isWeakTerminalVerb(value) {
  const tail = String(value || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  return ["keep", "keeps", "kept", "carry", "carries", "hold", "holds", "show", "shows", "use", "uses", "used", "support", "supports", "supported", "help", "helps", "helped", "feel", "feels", "felt", "stay", "stays", "stayed", "make", "makes", "made", "create", "creates", "created"].includes(tail);
}

function isWeakTerminalNoun(value) {
  const tail = String(value || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  return ["structure", "message", "layout", "hierarchy", "spacing", "signal", "signals", "style", "system", "content", "idea", "ideas", "point", "points", "presentation", "deck", "card", "cards", "chart", "discipline"].includes(tail);
}

function isWeakTerminalNounPair(prev, tail) {
  const left = String(prev || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  const right = String(tail || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  return ["hierarchy spacing", "layout discipline"].includes(left + " " + right);
}

function isWeakTerminalFinalVerb(value) {
  const tail = String(value || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "");
  return ["use", "uses", "used", "using", "support", "supports", "supported", "supporting", "help", "helps", "helped", "helping", "feel", "feels", "felt", "feeling", "carry", "carries", "carried", "carrying", "keep", "keeps", "kept", "keeping", "stay", "stays", "stayed", "staying", "make", "makes", "made", "making", "create", "creates", "created", "creating", "copy", "copies", "copied", "copying", "strengthen", "strengthens", "strengthened", "strengthening", "improve", "improves", "improved", "improving", "build", "builds", "built", "building"].includes(tail);
}

function endsWithWeakAdjectiveText(value) {
  const words = String(value || "").trim().split(/\s+/).filter(Boolean);
  if (words.length <= 4) return false;
  return isWeakTerminalAdjective(words[words.length - 1]);
}

function addObservationCards(slide, points, colors, slideIndex) {
  const fallback = [
    fallbackObservationDetail(0),
    fallbackObservationDetail(1),
    fallbackObservationDetail(2)
  ];
  const items = observationItemsForCards(points, slideIndex).slice(0, 3);
  while (items.length < 3) items.push(fallback[items.length]);
  const cardW = 330;
  const cardTop = 230;
  const cardHeight = 260;
  for (let i = 0; i < 3; i++) {
    const x = 78 + i * 382;
    const item = cleanObservationItem(items[i].detail || items[i].heading || items[i], fallback[i]);
    addPanel(slide, x, cardTop, cardW, cardHeight, colors.surface, colors.border, 8500, { role: "observation-card" });
    addShape(slide, "rect", x, cardTop, 7, cardHeight, i === 1 ? "#F59E0B" : (i % 2 === 0 ? colors.accent : colors.primary), colors.accent, 0, { role: "observation-accent" });
    const heading = String(items[i].heading || "").trim() || (item === fallback[i] ? fallbackObservationHeading(i) : shortHeading(item));
    if (usesCodexReferenceLearningRecipe()) {
      addText(slide, String(i + 1).padStart(2, "0"), x + 26, cardTop + 30, 64, 32, { role: "observation-index", fontSize: 18, bold: true, color: i === 1 ? "#FBBF24" : colors.accent, typeface: "Aptos Display" });
      addText(slide, heading, x + 26, cardTop + 72, cardW - 52, 58, { role: "observation-heading", fontSize: 21, bold: true, color: colors.title, typeface: "Aptos Display" });
    } else {
      addText(slide, String(i + 1) + ". " + heading, x + 26, cardTop + 38, cardW - 52, 58, { role: "observation-heading", fontSize: 21, bold: true, color: colors.title, typeface: "Aptos Display" });
    }
    const detail = shortCardText(observationDetail(item, heading, i), 92);
    if (detail) {
      const detailTop = usesCodexReferenceLearningRecipe() ? cardTop + 146 : cardTop + 124;
      const detailHeight = usesCodexReferenceLearningRecipe() ? 88 : 104;
      addText(slide, detail, x + 26, detailTop, cardW - 52, detailHeight, { role: "observation-detail", fontSize: 16, color: colors.body, typeface: "Aptos" });
    }
  }
}

function observationItemsForCards(points, slideIndex) {
  if (isReferenceLearningDeck()) {
    if (polishMode) return referenceLearningObservationItems();
    return fillReferenceLearningObservationItems(strongPlannedObservationItems(slideIndex));
  }
  const out = [];
  for (const card of planCards(slideIndex, "cards")) {
    const idx = out.length;
    const detail = usefulObservationDetail(card.detail, idx);
    out.push({
      heading: card.heading || shortHeading(detail),
      detail
    });
    if (out.length >= 3) return out;
  }
  for (const section of request.slides[slideIndex] && request.slides[slideIndex].sections || []) {
    const normalized = normalizeSectionItem(section, out.length);
    out.push({ heading: normalized.heading, detail: normalized.detail });
    if (out.length >= 3) return out;
  }
  for (const point of points || []) {
    const clean = cleanObservationItem(point, fallbackObservationDetail(out.length));
    out.push({ heading: shortHeading(clean), detail: clean });
    if (out.length >= 3) return out;
  }
  return out;
}

function isReferenceLearningDeck() {
  return String(request.designPlan && request.designPlan.deckIntent || "") === "concise-reference-style-learning";
}

function builderRecipe() {
  const explicit = String(request.designPlan && request.designPlan.builderRecipe || "").trim();
  if (explicit) return explicit;
  if (isReferenceLearningDeck()) return "codex-reference-learning";
  return "standard";
}

function usesCodexReferenceLearningRecipe() {
  return builderRecipe() === "codex-reference-learning";
}

function strongPlannedObservationItems(slideIndex) {
  const out = [];
  for (const card of planCards(slideIndex, "cards")) {
    const idx = out.length;
    const heading = strongPlannedHeading(card.heading);
    const detail = strongPlannedDetail(card.detail, 6);
    if (!heading || !detail) continue;
    out.push({ heading, detail });
    if (out.length >= 3) return out;
  }
  return out;
}

function strongPlannedHeading(value) {
  const text = String(value || "").trim();
  if (!text || isDanglingObservationFragment(text) || isImplementationChartInsight(text)) return "";
  const words = text.replace(/[.。]\s*$/, "").split(/\s+/).filter(Boolean);
  if (words.length < 2) return "";
  return shortCardText(text, 40).replace(/[.。]\s*$/, "");
}

function strongPlannedDetail(value, minWords) {
  const text = cleanObservationItem(value, "");
  if (!text || isLowInformationObservationDetail(text) || isDanglingObservationFragment(text)) return "";
  const words = text.replace(/[.。]\s*$/, "").split(/\s+/).filter(Boolean);
  if (words.length < minWords) return "";
  const clipped = shortCardText(text, 92);
  if (!clipped || isDanglingObservationFragment(clipped)) return "";
  return clipped;
}

function referenceLearningObservationItems() {
  return [
    {
      heading: "Repeatable style beats single-slide mimicry",
      detail: "Use repeated panels, accent rules, and compact cards instead of copying a deck."
    },
    {
      heading: "Important content stays editable",
      detail: "Keep words, labels, and chart callouts editable, not baked into images."
    },
	{
      heading: "Visual QA changes the final design",
      detail: "Rendered previews catch overflow, weak contrast, blank pages, and chart defaults."
    }
  ];
}

function fillReferenceLearningObservationItems(items) {
  const out = Array.isArray(items) ? items.slice(0, 3) : [];
  const fallback = referenceLearningObservationItems();
  const start = Math.min(out.length, fallback.length);
  for (const item of fallback.slice(start)) {
    if (out.length >= 3) break;
    const key = String(item.heading || "").toLowerCase();
    if (out.some((existing) => String(existing.heading || "").toLowerCase() === key)) continue;
    out.push(item);
  }
  for (const item of fallback) {
    if (out.length >= 3) break;
    const key = String(item.heading || "").toLowerCase();
    if (out.some((existing) => String(existing.heading || "").toLowerCase() === key)) continue;
    out.push(item);
  }
  return out.slice(0, 3);
}

function cleanObservationItem(value, fallback) {
  const text = String(value || "").trim();
  if (!text || isDanglingObservationFragment(text) || isImplementationChartInsight(text)) {
    return fallback;
  }
  return text;
}

function usefulObservationDetail(value, index) {
  const text = cleanObservationItem(value, "");
  if (!text || isLowInformationObservationDetail(text)) {
    return fallbackObservationDetail(index);
  }
  return text;
}

function isLowInformationObservationDetail(value) {
  const text = String(value || "").trim().replace(/[.。]\s*$/, "");
  if (!text) return true;
  const words = text.split(/\s+/).filter(Boolean);
  if (words.length < 6) return true;
  if (/^(prevents?|avoids?|reduces?)\s+(overload|clutter|noise)$/i.test(text)) return true;
  if (/^(keeps?|makes?)\s+\w+\s+(clear|readable|simple)$/i.test(text)) return true;
  return false;
}

function addSections(slide, sections, colors, slideIndex) {
  const items = (sections || []).slice(0, 4);
  const cols = items.length > 2 ? 2 : 1;
  const cardW = cols === 2 ? 500 : 720;
  for (let i = 0; i < items.length; i++) {
    const col = i % cols;
    const row = Math.floor(i / cols);
    const x = 78 + col * 540;
    const y = 205 + row * 165;
    const section = normalizeSectionItem(items[i], i);
    addPanel(slide, x, y, cardW, 132, colors.surface, colors.border, 8500, { role: "section-card" });
    addShape(slide, "rect", x, y, 7, 132, i % 2 === 0 ? colors.accent : colors.primary, colors.accent, 0, { role: "section-accent" });
    addText(slide, section.heading, x + 26, y + 20, cardW - 52, 32, { role: "section-heading", fontSize: 20, bold: true, color: colors.title, typeface: "Aptos Display" });
    addText(slide, section.detail, x + 26, y + 60, cardW - 52, 54, { role: "section-detail", fontSize: 15, color: colors.body, typeface: "Aptos" });
  }
}

function normalizeSectionItem(item, index) {
  const rawHeading = String(item && item.heading || "").trim();
  const rawDetail = String(item && item.detail || "").trim();
  const cleanedDetail = cleanObservationItem(rawDetail, fallbackObservationDetail(index));
  if (/^takeaway\s+\d+$/i.test(rawHeading) && cleanedDetail) {
    const heading = cleanedDetail === fallbackObservationDetail(index) ? fallbackObservationHeading(index) : shortHeading(cleanedDetail);
    return { heading, detail: shortCardText(observationDetail(cleanedDetail, heading, index), 76) };
  }
  const cleanedHeading = rawHeading && !isImplementationChartInsight(rawHeading) ? rawHeading : (cleanedDetail === fallbackObservationDetail(index) ? fallbackObservationHeading(index) : shortHeading(cleanedDetail));
  return {
    heading: cleanedHeading || "Section",
    detail: shortCardText(cleanedDetail || fallbackObservationDetail(index), 76)
  };
}

function addMetrics(slide, metrics, colors, slideIndex) {
  const items = (metrics || []).slice(0, 4);
  for (let i = 0; i < items.length; i++) {
    const x = 80 + i * 285;
    addPanel(slide, x, 210, 240, 190, colors.surface, colors.border, 9000, { role: "metric-card" });
    addText(slide, items[i].value || "", x + 24, 246, 192, 46, { role: "metric-value", fontSize: 34, bold: true, color: colors.accent, typeface: "Aptos Display" });
    addText(slide, items[i].label || "", x + 24, 308, 192, 34, { role: "metric-label", fontSize: 16, bold: true, color: colors.title, typeface: "Aptos" });
    addText(slide, metricNoteText(items[i].note, i), x + 24, 346, 192, 42, { role: "metric-note", fontSize: 13, color: colors.body, typeface: "Aptos" });
  }
}

function metricNoteText(note, index) {
  const value = String(note || "").trim();
  if (!value || isDanglingObservationFragment(value)) {
    return fallbackObservationDetail(index);
  }
  return value;
}

async function addImagePlate(slide, asset, left, top, width, height, alt, slideNo) {
  try {
    const blob = await readImageBlob(asset.path);
    const image = slide.images.add({ blob, fit: "cover", alt: alt || asset.name || "Local reference visual" });
    image.position = { left, top, width, height };
    inspect.images.push({ path: asset.path, alt: alt || "", slide: Number(slideNo || 0), bbox: { left, top, width, height } });
    return image;
  } catch (error) {
    inspect.images.push({ path: asset.path, failed: true, error: String(error && error.message || error), slide: Number(slideNo || 0), bbox: { left, top, width, height } });
    return null;
  }
}

async function readImageBlob(imagePath) {
  const bytes = await fs.readFile(imagePath);
  return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
}

function addNativeChart(slide, chartData, left, top, width, height, colors, slideIndex) {
  const kind = normalizeChartType(chartData.type);
  const chart = slide.charts.add(kind);
  chart.position = { left, top, width, height };
  chart.title = chartData.title || "";
  chart.categories = chartData.categories || [];
  const series = chart.series.add(chartData.title || "Series");
  series.values = chartData.values || [];
  series.categories = chart.categories;
  series.fill = colors.accent;
  series.stroke = { width: 2, style: "solid", fill: colors.accent };
  chart.hasLegend = false;
  if (chart.titleTextStyle) {
    chart.titleTextStyle.typeface = "Aptos Display";
    chart.titleTextStyle.fontSize = 20;
    chart.titleTextStyle.fill = colors.title;
  }
  if (chart.xAxis && chart.xAxis.textStyle) {
    chart.xAxis.textStyle.typeface = "Aptos";
    chart.xAxis.textStyle.fontSize = 14;
    chart.xAxis.textStyle.fill = colors.body;
  }
  if (chart.yAxis && chart.yAxis.textStyle) {
    chart.yAxis.textStyle.typeface = "Aptos";
    chart.yAxis.textStyle.fontSize = 13;
    chart.yAxis.textStyle.fill = colors.body;
  }
  if (chart.lineOptions) chart.lineOptions.grouping = "standard";
  inspect.nativeCharts.push({ kind, title: chartData.title || "", categories: chart.categories.length, values: series.values.length });
}

function normalizeChartType(value) {
  const v = String(value || "").toLowerCase();
  if (v.includes("pie")) return "pie";
  if (v.includes("line")) return "line";
  return "bar";
}

function splitContent(content) {
  return String(content || "").split(/[.;。；]/).map((item) => item.trim()).filter(Boolean);
}

function isObservationSlide(data) {
  const text = String((data.title || "") + " " + (data.subtitle || "") + " " + (data.content || "")).toLowerCase();
  return text.includes("observation") || text.includes("summary") || text.includes("takeaway");
}

function firstText(items) {
  return (items || []).find((item) => String(item || "").trim()) || "";
}

function shortHeading(text) {
  const value = String(text || "").replace(/[:：].*$/, "").trim();
  const words = value.split(/\s+/).filter(Boolean);
  if (words.length > 0 && words.length <= 4) return value.replace(/[.。]$/, "");
  if (words.length > 4) return words.slice(0, 3).join(" ");
  return value.slice(0, 16).replace(/[.。]$/, "") || "Observation";
}

function observationDetail(text, heading, index) {
  let detail = String(text || "").trim();
  const normalizedHeading = String(heading || "").trim().toLowerCase().replace(/[.。]$/, "");
  const normalizedDetail = detail.toLowerCase().replace(/[.。]$/, "");
  if (!detail || normalizedDetail === normalizedHeading) {
    return fallbackObservationDetail(index);
  }
  if (normalizedDetail.startsWith(normalizedHeading)) {
    detail = detail.slice(String(heading || "").length).replace(/^[:：.\s]+/, "");
  }
  if (!String(detail || "").trim()) {
    return fallbackObservationDetail(index);
  }
  if (isDanglingObservationFragment(detail)) {
    return fallbackObservationDetail(index);
  }
  if (detail.length > 92) {
    return shortCardText(detail, 92);
  }
  return finishCardText(detail);
}

function isDanglingObservationFragment(detail) {
  const value = String(detail || "").trim().toLowerCase();
  if (/^(are|is|was|were|should|must|need|needs|required|requires|help|helps)\b/.test(value)) return true;
  const words = value.replace(/[.。]\s*$/, "").split(/\s+/).filter(Boolean);
  if (words.length >= 4 && isWeakTerminalFinalVerb(words[words.length - 1])) return true;
  if (words.length >= 4 && ["selective", "main"].includes(String(words[words.length - 1] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, ""))) return true;
  if (words.length >= 2 && isWeakTerminalVerb(words[words.length - 2]) && (isWeakTerminalNoun(words[words.length - 1]) || isWeakTerminalAdjective(words[words.length - 1]))) return true;
  if (words.length >= 2 && isWeakTerminalAdjective(words[words.length - 2]) && isWeakTerminalNoun(words[words.length - 1])) return true;
  if (words.length >= 2 && isWeakTerminalNounPair(words[words.length - 2], words[words.length - 1])) return true;
  if (words.length >= 2 && String(words[words.length - 2] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "") === "not" && ["a", "an", "the"].includes(String(words[words.length - 1] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, ""))) return true;
  if (words.length >= 3 && isWeakTerminalVerb(words[words.length - 3]) && ["the", "a", "an"].includes(String(words[words.length - 2] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "")) && (isWeakTerminalNoun(words[words.length - 1]) || isWeakTerminalAdjective(words[words.length - 1]))) return true;
  if (words.length >= 3 && String(words[words.length - 3] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, "") === "until" && ["each", "every", "any"].includes(String(words[words.length - 2] || "").toLowerCase().replace(/^[\s.,;:，。；：()[\]{}]+|[\s.,;:，。；：()[\]{}]+$/g, ""))) return true;
  return false;
}

function fallbackObservationDetail(index) {
  const details = [
    "Use repeated panels, accent rules, and compact cards instead of copying a deck.",
    "Keep words, labels, and chart callouts editable, not baked into images.",
    "Keep contrast, spacing, and title scale consistent across slides."
  ];
  return details[index % details.length];
}

function fallbackObservationHeading(index) {
  const headings = [
    "Stable structure",
    "Readable hierarchy",
    "Concise copy"
  ];
  return headings[index % headings.length];
}

function resolveColors(stylePreset, theme) {
  const brief = request.styleBrief || {};
  const explicitStyle = String([
    stylePreset,
    request.designPlan && request.designPlan.styleBias
  ].filter(Boolean).join(" ")).toLowerCase();
  const intent = String([
    stylePreset,
    request.designPlan && request.designPlan.styleBias,
    brief.stylePresetHint,
    brief.paletteIntent,
    brief.layoutRhythm,
    brief.imageTreatment
  ].filter(Boolean).join(" ")).toLowerCase();
  const lightMode = /editorial-light|light-theme|light theme|light style|white canvas|off-white|off white|bright canvas|airy whitespace/.test(explicitStyle);
  const darkMode = !lightMode && /executive-dark|dark|dark-neutral|dark neutral|night|deep canvas/.test(intent);
  const primary = darkMode ? "#3B82F6" : (hex(theme.primaryColor) || "#3C82F6");
  const accent = darkMode ? "#22D3EE" : (hex(theme.accentColor) || "#1BA6A6");
  const bg = darkMode ? "#0B1020" : (hex(theme.bgColor1) || "#F6F8FB");
  const title = darkMode ? "#F8FAFC" : (hex(theme.titleTextColor) || "#172033");
  const body = darkMode ? "#CBD5E1" : (hex(theme.textColor) || "#42526B");
  return {
    primary,
    accent,
    background: bg,
    title,
    body,
    muted: darkMode ? "#94A3B8" : "#7C8798",
    surface: darkMode ? "#111827" : "#FFFFFF",
    border: darkMode ? "#334155" : "#D8DEE9",
    accentSoft: darkMode ? "#1E293B" : "#DDF7F4",
    darkMode
  };
}

function hex(value) {
  const raw = String(value || "").trim();
  if (/^#[0-9a-fA-F]{6}$/.test(raw)) return raw;
  if (/^[0-9a-fA-F]{6}$/.test(raw)) return "#" + raw;
  return "";
}
`
