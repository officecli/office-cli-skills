package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	"github.com/officecli/officecli/pkg/officegen"
)

type pptxArtifactWorkerRequest struct {
	Title          string                   `json:"title"`
	StylePreset    string                   `json:"stylePreset"`
	Theme          *officegen.SlideTheme    `json:"theme,omitempty"`
	Slides         []officegen.Slide        `json:"slides"`
	ReferenceFiles []string                 `json:"referenceFiles,omitempty"`
	StyleBrief     *PPTXReferenceStyleBrief `json:"styleBrief,omitempty"`
	OutputPPTX     string                   `json:"outputPptx"`
	PreviewDir     string                   `json:"previewDir"`
	InspectPath    string                   `json:"inspectPath"`
}

type pptxArtifactWorkerOutput struct {
	OutputPPTX     string   `json:"outputPptx"`
	PreviewFiles   []string `json:"previewFiles,omitempty"`
	InspectPath    string   `json:"inspectPath,omitempty"`
	ImportedRefs   int      `json:"importedRefs,omitempty"`
	EditableItems  int      `json:"editableItems,omitempty"`
	NativeCharts   int      `json:"nativeCharts,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	WorkerVersion  string   `json:"workerVersion,omitempty"`
	ArtifactToolOK bool     `json:"artifactToolOk,omitempty"`
}

type pptxArtifactWorkerRunner func(ctx context.Context, request pptxArtifactWorkerRequest, workDir string) (*pptxArtifactWorkerOutput, error)

var runPPTXArtifactWorker pptxArtifactWorkerRunner = runPPTXArtifactWorkerDefault

func buildPPTXWithArtifactWorker(ctx context.Context, progress engine.ProgressEmitter, payload pptxPayload, fallback string, localPreview bool, options PPTXBuildOptions) ([]byte, string, []engine.GenerateIssue, []byte, []byte, error) {
	workDir, err := os.MkdirTemp("", "officecli-pptx-artifact-*")
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("create artifact worker directory: %w", err)
	}
	previewDir := filepath.Join(workDir, "preview")
	if err := os.MkdirAll(previewDir, 0o755); err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("create artifact worker preview directory: %w", err)
	}
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = firstNonEmpty(generateengine.ExtractTitleFromDescription(fallback), "Presentation")
	}
	request := pptxArtifactWorkerRequest{
		Title:          title,
		StylePreset:    officegen.NormalizeStylePreset(payload.StylePreset),
		Theme:          payload.Theme,
		Slides:         append([]officegen.Slide(nil), payload.Slides...),
		ReferenceFiles: representativeReferenceFiles(options),
		StyleBrief:     options.ReferenceBrief,
		OutputPPTX:     filepath.Join(workDir, "output.pptx"),
		PreviewDir:     previewDir,
		InspectPath:    filepath.Join(workDir, "inspect.json"),
	}
	emitProgress(ctx, progress, progressStepAssemble, "running", "Running artifact experimental PPTX worker")
	workerOutput, err := runPPTXArtifactWorker(ctx, request, workDir)
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("artifact experimental backend failed: %w", err)
	}
	outputPath := request.OutputPPTX
	if workerOutput != nil && strings.TrimSpace(workerOutput.OutputPPTX) != "" {
		outputPath = workerOutput.OutputPPTX
	}
	fileBytes, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("read artifact worker output: %w", err)
	}
	if len(fileBytes) == 0 {
		return nil, "", nil, nil, nil, fmt.Errorf("artifact worker output is empty")
	}
	var warnings []engine.GenerateIssue
	if workerOutput != nil {
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

func representativeReferenceFiles(options PPTXBuildOptions) []string {
	if options.ReferenceProfile == nil {
		return nil
	}
	out := make([]string, 0, 3)
	for _, file := range options.ReferenceProfile.SourceFiles {
		if strings.TrimSpace(file.FailedReason) != "" || strings.TrimSpace(file.Path) == "" {
			continue
		}
		out = append(out, file.Path)
		if len(out) >= 3 {
			break
		}
	}
	return out
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
		return "", fmt.Errorf("node executable was not found; set OFFICECLI_PPTX_ARTIFACT_NODE to enable pptx backend %q", PPTXBackendArtifactExperimental)
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

const [requestPath, responsePath] = process.argv.slice(2);
if (!requestPath || !responsePath) {
  throw new Error("usage: node pptx_artifact_worker.mjs <request.json> <response.json>");
}

const request = JSON.parse(await fs.readFile(requestPath, "utf8"));
await fs.mkdir(path.dirname(request.outputPptx), { recursive: true });
await fs.mkdir(request.previewDir, { recursive: true });

const presentation = Presentation.create({ slideSize: { width: 1280, height: 720 } });
const theme = request.theme || {};
const colors = resolveColors(request.stylePreset, theme);
const inspect = {
  backend: "artifact-experimental",
  importedReferences: [],
  editableItems: [],
  nativeCharts: [],
  previews: [],
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
  buildSlide(slide, data, i, request.slides.length, colors);
}

for (let i = 0; i < presentation.slides.count; i++) {
  const slide = presentation.slides.getItem(i);
  const blob = await presentation.export({ slide, format: "png", scale: 1 });
  const bytes = Buffer.from(await blob.arrayBuffer());
  const previewPath = path.join(request.previewDir, "slide-" + String(i + 1).padStart(2, "0") + ".png");
  await fs.writeFile(previewPath, bytes);
  inspect.previews.push(previewPath);
}

await fs.writeFile(request.inspectPath, JSON.stringify(inspect, null, 2));
const pptx = await PresentationFile.exportPptx(presentation);
await pptx.save(request.outputPptx);

await fs.writeFile(responsePath, JSON.stringify({
  outputPptx: request.outputPptx,
  previewFiles: inspect.previews,
  inspectPath: request.inspectPath,
  importedRefs: inspect.importedReferences.filter((item) => item.status === "imported").length,
  editableItems: inspect.editableItems.length,
  nativeCharts: inspect.nativeCharts.length,
  warnings: inspect.importedReferences.some((item) => item.status === "failed") ? ["Some reference PPTX files could not be imported by the artifact worker and were used only through the Go reference profile."] : [],
  workerVersion: "artifact-experimental-v1",
  artifactToolOk: true
}, null, 2));

function buildSlide(slide, data, index, total, colors) {
  const isCover = index === 0 || data.isTitle || data.layout === "title";
  addText(slide, data.title || "Untitled", isCover ? 90 : 64, isCover ? 118 : 48, isCover ? 760 : 760, isCover ? 92 : 54, {
    role: isCover ? "title" : "heading",
    fontSize: isCover ? 42 : 30,
    bold: true,
    color: colors.title,
    typeface: "Aptos Display"
  });
  const subtitle = data.subtitle || data.content || "";
  if (subtitle) {
    addText(slide, subtitle, isCover ? 94 : 70, isCover ? 224 : 108, isCover ? 660 : 620, isCover ? 96 : 54, {
      role: isCover ? "subtitle" : "takeaway",
      fontSize: isCover ? 21 : 17,
      color: colors.body,
      typeface: "Aptos"
    });
  }
  if (isCover) {
    addPanel(slide, 880, 92, 272, 420, colors.accentSoft, colors.accent);
    addText(slide, "Reference style", 922, 150, 190, 36, { role: "reference-label", fontSize: 18, bold: true, color: colors.title, typeface: "Aptos" });
    addText(slide, "Editable semantic build", 922, 198, 190, 96, { role: "reference-note", fontSize: 20, color: colors.body, typeface: "Aptos" });
  } else if (data.chart) {
    addNativeChart(slide, data.chart, 700, 160, 430, 315, colors, index);
    addPoints(slide, data.points || [], 82, 176, 500, colors, index);
  } else if ((data.metrics || []).length) {
    addMetrics(slide, data.metrics, colors, index);
  } else if ((data.sections || []).length) {
    addSections(slide, data.sections, colors, index);
  } else {
    addPoints(slide, data.points || [], 82, 176, 650, colors, index);
  }
  addText(slide, String(index + 1).padStart(2, "0") + " / " + String(total).padStart(2, "0"), 1080, 650, 110, 24, {
    role: "footer",
    fontSize: 12,
    color: colors.muted,
    typeface: "Aptos",
    align: "right"
  });
}

function addPanel(slide, left, top, width, height, fill, line) {
  return slide.shapes.add({
    geometry: "roundRect",
    position: { left, top, width, height },
    fill,
    line: { fill: line || fill, width: 1 },
    adjustmentList: [{ name: "adj", formula: "val 12000" }]
  });
}

function addText(slide, text, left, top, width, height, style) {
  const shape = slide.shapes.add({
    geometry: "rect",
    position: { left, top, width, height },
    fill: "#FFFFFF00",
    line: { fill: "#FFFFFF00", width: 0 }
  });
  shape.text = String(text || "");
  shape.text.fontSize = style.fontSize || 18;
  shape.text.typeface = style.typeface || "Aptos";
  shape.text.color = style.color || "#111827";
  shape.text.bold = !!style.bold;
  shape.text.autoFit = "shrinkText";
  if (style.align) shape.text.alignment = style.align;
  inspect.editableItems.push({ kind: "text", role: style.role || "text", text: String(text || ""), bbox: { left, top, width, height } });
  return shape;
}

function addPoints(slide, points, left, top, width, colors, slideIndex) {
  const items = (points || []).slice(0, 5);
  for (let i = 0; i < items.length; i++) {
    const y = top + i * 72;
    addPanel(slide, left, y, width, 50, colors.surface, colors.border);
    addText(slide, items[i], left + 22, y + 10, width - 44, 30, { role: "bullet", fontSize: 18, color: colors.body, typeface: "Aptos" });
  }
}

function addSections(slide, sections, colors, slideIndex) {
  const items = (sections || []).slice(0, 4);
  const cols = items.length > 2 ? 2 : 1;
  const cardW = cols === 2 ? 500 : 720;
  for (let i = 0; i < items.length; i++) {
    const col = i % cols;
    const row = Math.floor(i / cols);
    const x = 78 + col * 540;
    const y = 165 + row * 175;
    addPanel(slide, x, y, cardW, 132, colors.surface, colors.border);
    addText(slide, items[i].heading || "Section", x + 24, y + 20, cardW - 48, 32, { role: "section-heading", fontSize: 20, bold: true, color: colors.title, typeface: "Aptos Display" });
    addText(slide, items[i].detail || "", x + 24, y + 60, cardW - 48, 52, { role: "section-detail", fontSize: 15, color: colors.body, typeface: "Aptos" });
  }
}

function addMetrics(slide, metrics, colors, slideIndex) {
  const items = (metrics || []).slice(0, 4);
  for (let i = 0; i < items.length; i++) {
    const x = 80 + i * 285;
    addPanel(slide, x, 190, 240, 190, colors.surface, colors.border);
    addText(slide, items[i].value || "", x + 24, 226, 192, 46, { role: "metric-value", fontSize: 34, bold: true, color: colors.accent, typeface: "Aptos Display" });
    addText(slide, items[i].label || "", x + 24, 288, 192, 34, { role: "metric-label", fontSize: 16, bold: true, color: colors.title, typeface: "Aptos" });
    addText(slide, items[i].note || "", x + 24, 326, 192, 42, { role: "metric-note", fontSize: 13, color: colors.body, typeface: "Aptos" });
  }
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
    chart.titleTextStyle.fontSize = 18;
    chart.titleTextStyle.fill = colors.title;
  }
  if (chart.xAxis && chart.xAxis.textStyle) {
    chart.xAxis.textStyle.typeface = "Aptos";
    chart.xAxis.textStyle.fontSize = 11;
  }
  if (chart.yAxis && chart.yAxis.textStyle) {
    chart.yAxis.textStyle.typeface = "Aptos";
    chart.yAxis.textStyle.fontSize = 11;
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

function resolveColors(stylePreset, theme) {
  const primary = hex(theme.primaryColor) || "#1D4ED8";
  const accent = hex(theme.accentColor) || "#0F766E";
  const bg = hex(theme.bgColor1) || (stylePreset === "executive-dark" ? "#0B1020" : "#F8FAFC");
  const title = hex(theme.titleTextColor) || (stylePreset === "executive-dark" ? "#FFFFFF" : "#111827");
  const body = hex(theme.textColor) || (stylePreset === "executive-dark" ? "#E5E7EB" : "#334155");
  return {
    primary,
    accent,
    background: bg,
    title,
    body,
    muted: stylePreset === "executive-dark" ? "#9CA3AF" : "#64748B",
    surface: stylePreset === "executive-dark" ? "#111827" : "#FFFFFF",
    border: stylePreset === "executive-dark" ? "#374151" : "#CBD5E1",
    accentSoft: stylePreset === "executive-dark" ? "#1F2937" : "#ECFDF5"
  };
}

function hex(value) {
  const raw = String(value || "").trim();
  if (/^#[0-9a-fA-F]{6}$/.test(raw)) return raw;
  if (/^[0-9a-fA-F]{6}$/.test(raw)) return "#" + raw;
  return "";
}
`
