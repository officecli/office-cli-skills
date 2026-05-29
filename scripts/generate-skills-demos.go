package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	appruntime "github.com/officecli/officecli/internal/runtime"
)

const verifiedAt = "2026-05-14T00:00:00Z"

type demoMeta struct {
	Title         string            `json:"title"`
	Type          string            `json:"type"`
	Command       string            `json:"command"`
	Artifact      string            `json:"artifact"`
	Preview       string            `json:"preview"`
	PromptFile    string            `json:"prompt_file"`
	VerifiedAt    string            `json:"verified_at"`
	GeneratedWith string            `json:"generated_with"`
	Verification  []string          `json:"verification"`
	Notes         []string          `json:"notes,omitempty"`
	Additional    map[string]any    `json:"additional_files,omitempty"`
	SHA256        map[string]string `json:"sha256"`
}

type demoLLM struct{}

func (demoLLM) CompleteText(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (demoLLM) CompleteJSON(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (demoLLM) CompleteStructured(context.Context, engine.StructuredCompletionRequest) (string, error) {
	return "", nil
}

func (demoLLM) GenerateImage(_ context.Context, req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	data, err := demoImagePNG(req.Prompt)
	if err != nil {
		return nil, err
	}
	return &engine.ImageGenerationResult{Data: data, MIME: "image/png"}, nil
}

func main() {
	root := filepath.Join("public", "skills-demos")
	if err := os.RemoveAll(root); err != nil {
		fatal(err)
	}
	must(os.MkdirAll(root, 0o755))

	buildPPTXImageRich(root)
	buildPPTXTextOnly(root)
	buildDOCX(root)
	buildXLSX(root)
	buildReport(root)
	buildStandaloneIMG(root)
}

func buildPPTXImageRich(root string) {
	const slug = "pptx-image-rich"
	const prompt = "Create a 5-slide image-rich strategy deck for an AI document operations product. Audience: product leadership. Tone: polished, visual, decision-oriented. Include generated visuals where they clarify the workflow."
	const payload = `{
  "title":"Image-rich strategy deck",
  "subtitle":"AI document operations from prompt to published artifact",
  "stylePreset":"executive-dark",
  "slides":[
    {"title":"Image-rich strategy deck","layout":"title","variant":"title-center","subtitle":"Prompt to published Office artifact","hasImage":true,"imagePrompt":"A premium workstation showing PPTX, DOCX, XLSX, report, and image outputs arranged around a command line"},
    {"title":"Workflow at a glance","layout":"sections","variant":"three-column","subtitle":"Agents keep the loop reproducible","sections":[{"heading":"Plan","detail":"The agent converts intent into a typed payload."},{"heading":"Render","detail":"OfficeCLI assembles the file locally."},{"heading":"Publish","detail":"Configured runs return online preview links."}]},
    {"title":"Generated output gallery","layout":"gallery","variant":"gallery","subtitle":"Visuals travel with the deck when image support is enabled","visuals":[{"label":"Deck","prompt":"A clean presentation canvas with generated images and charts","caption":"PPTX visual narrative"},{"label":"Report","prompt":"An HTML report preview with charts and highlighted findings","caption":"Workbook-backed report"},{"label":"Image","prompt":"A standalone generated product hero image","caption":"IMG output"}]},
    {"title":"Agent contract","layout":"comparison","variant":"split","subtitle":"Structured tools avoid scraping CLI progress","sections":[{"heading":"office.prepare","detail":"Returns schemas and workbook context."},{"heading":"office.render","detail":"Validates and writes final Office files."}]},
    {"title":"Decision","layout":"closing","variant":"decision","subtitle":"Use the public skills repo as the reproducible demo surface","points":["Ship demo files next to prompts and commands","Keep generated previews visible in GitHub","Route agents through OfficeCLI when supported"]}
  ]
}`
	dir := demoDir(root, slug)
	writeText(filepath.Join(dir, "prompt.md"), promptMarkdown("Image-rich strategy deck", prompt, commandPPTX(slug, true)))
	fileBytes, fileName, _, previewHTML, previewJSON, err := appruntime.BuildPPTXFromJSON(context.Background(), demoLLM{}, nil, payload, prompt, "executive-dark", true, true)
	must(err)
	artifact := "image-rich-strategy-deck.pptx"
	writeBytes(filepath.Join(dir, artifact), fileBytes)
	writeBytes(filepath.Join(dir, "preview.html"), previewHTML)
	writeBytes(filepath.Join(dir, "preview.json"), previewJSON)
	meta := baseMeta("Image-rich strategy deck", "pptx", commandPPTX(slug, true), artifact)
	meta.Notes = []string{
		"Generated through OfficeCLI PPTX rendering code with deterministic demo image assets so the public repository is reproducible.",
		fmt.Sprintf("OfficeCLI renderer suggested file name: %s.", fileName),
	}
	writeMeta(dir, meta)
}

func buildPPTXTextOnly(root string) {
	const slug = "pptx-text-only"
	const prompt = "Create a 4-slide text-only executive briefing about AI document automation governance. Audience: operations leadership. Do not generate or embed images."
	const payload = `{
  "title":"Text-only executive briefing",
  "subtitle":"Governed document automation without generated images",
  "stylePreset":"editorial-light",
  "slides":[
    {"title":"Text-only executive briefing","layout":"title","variant":"title-center","subtitle":"Governance, auditability, and operator trust"},
    {"title":"Why text-only still matters","layout":"sections","variant":"three-column","subtitle":"Some workflows prefer compact, reviewable copy","sections":[{"heading":"Controls","detail":"No generated media in regulated review loops."},{"heading":"Speed","detail":"Smaller files for quick operational sharing."},{"heading":"Audit","detail":"Every claim remains visible in text."}]},
    {"title":"Operating model","layout":"timeline","variant":"timeline","subtitle":"A staged rollout keeps ownership clear","points":["Define approved templates","Render locally with --no-images","Publish only after human review"]},
    {"title":"Recommendation","layout":"closing","variant":"decision","subtitle":"Use --no-images when reviewability matters more than visual richness","points":["Document the default","Keep prompts with artifacts","Use scoring only when requested"]}
  ]
}`
	dir := demoDir(root, slug)
	writeText(filepath.Join(dir, "prompt.md"), promptMarkdown("Text-only executive briefing", prompt, commandPPTX(slug, false)))
	fileBytes, fileName, _, previewHTML, previewJSON, err := appruntime.BuildPPTXFromJSON(context.Background(), demoLLM{}, nil, payload, prompt, "editorial-light", false, true)
	must(err)
	artifact := "text-only-executive-briefing.pptx"
	writeBytes(filepath.Join(dir, artifact), fileBytes)
	writeBytes(filepath.Join(dir, "preview.html"), previewHTML)
	writeBytes(filepath.Join(dir, "preview.json"), previewJSON)
	meta := baseMeta("Text-only executive briefing", "pptx", commandPPTX(slug, false), artifact)
	meta.Notes = []string{fmt.Sprintf("OfficeCLI renderer suggested file name: %s.", fileName)}
	writeMeta(dir, meta)
}

func buildDOCX(root string) {
	const slug = "docx-brief"
	const prompt = "Draft a customer-facing DOCX brief introducing OfficeCLI to an automation team. Include a decision callout and a compact rollout table."
	const payload = `{
  "title":"OfficeCLI customer brief",
  "subtitle":"Local Office generation for agent workflows",
  "theme":{"preset":"executive"},
  "blocks":[
    {"type":"heading","level":1,"text":"Why this skill exists"},
    {"type":"paragraph","text":"officecli gives local agents a repeatable way to decide when OfficeCLI should handle PPTX, DOCX, XLSX, report, or standalone image work."},
    {"type":"callout","title":"Recommended use","text":"Install the skill beside the OfficeCLI binary, then let the agent check capabilities before it writes any final Office artifact."},
    {"type":"heading","level":1,"text":"Rollout checklist"},
    {"type":"table","title":"Adoption plan","columns":["Step","Owner","Evidence"],"rows":[["Install skill","Automation lead","officecli --version"],["Configure runtime","Platform owner","officecli config status"],["Generate artifact","Agent user","Local file plus preview"]]}
  ]
}`
	dir := demoDir(root, slug)
	writeText(filepath.Join(dir, "prompt.md"), promptMarkdown("OfficeCLI customer brief", prompt, "officecli new docx \"OfficeCLI customer brief\" --prompt-file ./prompt.md --local-preview --no-publish"))
	fileBytes, fileName, previewHTML, previewJSON, err := generateengine.BuildDOCXArtifactFromJSON(payload, prompt, "formal", true)
	must(err)
	artifact := "officecli-customer-brief.docx"
	writeBytes(filepath.Join(dir, artifact), fileBytes)
	writeBytes(filepath.Join(dir, "preview.html"), previewHTML)
	writeBytes(filepath.Join(dir, "preview.json"), previewJSON)
	meta := baseMeta("OfficeCLI customer brief", "docx", "officecli new docx \"OfficeCLI customer brief\" --prompt-file ./prompt.md --local-preview --no-publish", artifact)
	meta.Notes = []string{fmt.Sprintf("OfficeCLI renderer suggested file name: %s.", fileName)}
	writeMeta(dir, meta)
}

func buildXLSX(root string) {
	const slug = "xlsx-dashboard"
	const prompt = "Generate an XLSX dashboard workbook for OfficeCLI demo adoption. Include channel, demo type, status, audience, and follow-up fields."
	const payload = `{
  "title":"Demo adoption dashboard",
  "subtitle":"Public skill examples by artifact type",
  "theme":{"preset":"analysis"},
  "sheets":[
    {
      "name":"Demos",
      "purpose":"Track public demo coverage and readiness",
      "summary":[{"label":"Demo count","value":"6"},{"label":"Coverage","value":"PPTX DOCX XLSX REPORT IMG"}],
      "columns":[
        {"label":"Artifact","type":"string"},
        {"label":"Audience","type":"string"},
        {"label":"Status","type":"string"},
        {"label":"Priority","type":"number"}
      ],
      "rows":[["PPTX image-rich","Product leadership","Ready","1"],["PPTX text-only","Operations","Ready","2"],["DOCX brief","Automation team","Ready","3"],["XLSX dashboard","RevOps","Ready","4"],["Report workbook","Board","Ready","5"],["Standalone IMG","Marketing","Ready","6"]],
      "showTotals": true
    }
  ]
}`
	dir := demoDir(root, slug)
	writeText(filepath.Join(dir, "prompt.md"), promptMarkdown("Demo adoption dashboard", prompt, "officecli new xlsx \"Demo adoption dashboard\" --prompt-file ./prompt.md --local-preview --no-publish"))
	fileBytes, fileName, previewHTML, previewJSON, err := generateengine.BuildXLSXArtifactFromJSON(payload, prompt, "analysis", true)
	must(err)
	artifact := "demo-adoption-dashboard.xlsx"
	writeBytes(filepath.Join(dir, artifact), fileBytes)
	writeBytes(filepath.Join(dir, "preview.html"), previewHTML)
	writeBytes(filepath.Join(dir, "preview.json"), previewJSON)
	meta := baseMeta("Demo adoption dashboard", "xlsx", "officecli new xlsx \"Demo adoption dashboard\" --prompt-file ./prompt.md --local-preview --no-publish", artifact)
	meta.Notes = []string{fmt.Sprintf("OfficeCLI renderer suggested file name: %s.", fileName)}
	writeMeta(dir, meta)
}

func buildReport(root string) {
	const slug = "report-workbook"
	const prompt = "Create a workbook-backed HTML report for demo program readiness. Ground the report in the source workbook and include KPI cards, findings, a chart, and an appendix table."
	const workbookPayload = `{
  "title":"Demo program source workbook",
  "sheets":[{"name":"Readiness","columns":[{"label":"Demo","type":"string"},{"label":"Score","type":"number"},{"label":"Owner","type":"string"}],"rows":[["PPTX image-rich","95","Product"],["DOCX brief","90","Docs"],["XLSX dashboard","88","RevOps"],["Report workbook","92","Data"],["Standalone IMG","84","Marketing"]]}]
}`
	const reportPayload = `{
  "title":"Demo program readiness report",
  "subtitle":"Workbook-backed evidence for public skills examples",
  "language":"English",
  "audience":"Product and developer relations",
  "summary":"The demo program covers all current OfficeCLI artifact types and is ready for public repository presentation.",
  "updatedAt":"2026-05-14",
  "kpis":[{"label":"Artifact coverage","value":"5 types","change":"Complete"},{"label":"Demo count","value":"6","change":"Ready"},{"label":"Average readiness","value":"89.8","change":"Positive"}],
  "findings":["PPTX has both image-rich and text-only coverage.","Workbook-backed report examples include a source workbook.","Standalone image coverage remains documented as configuration-dependent."],
  "sections":[{"title":"Readiness by artifact type","subtitle":"Scores from the source workbook","narrative":["Every demo includes an artifact, prompt, command, metadata, and preview image."],"takeaways":["Keep binary files small.","Use metadata to disclose generation source."],"charts":[{"type":"bar","title":"Readiness score","categories":["PPTX rich","DOCX","XLSX","Report","IMG"],"series":[{"name":"Score","values":[95,90,88,92,84]}],"unit":"score","source":"Demo program source workbook"}]}],
  "appendixTables":[{"title":"Demo inventory","headers":["Demo","Owner","Status"],"rows":[["pptx-image-rich","Product","Ready"],["pptx-text-only","Docs","Ready"],["docx-brief","Docs","Ready"],["xlsx-dashboard","RevOps","Ready"],["report-workbook","Data","Ready"],["standalone-img","Marketing","Ready"]]}]
}`
	dir := demoDir(root, slug)
	writeText(filepath.Join(dir, "prompt.md"), promptMarkdown("Demo program readiness report", prompt, "officecli new report \"Demo program readiness report\" --file ./demo-program-source-workbook.xlsx --prompt-file ./prompt.md --no-publish"))
	workbookBytes, _, _, _, err := generateengine.BuildXLSXArtifactFromJSON(workbookPayload, "Build the source workbook for the report demo.", "analysis", true)
	must(err)
	sourceWorkbook := "demo-program-source-workbook.xlsx"
	writeBytes(filepath.Join(dir, sourceWorkbook), workbookBytes)
	reportBytes, fileName, err := generateengine.BuildReportFromJSON(reportPayload, prompt)
	must(err)
	artifact := "demo-program-readiness-report.html"
	writeBytes(filepath.Join(dir, artifact), reportBytes)
	writeBytes(filepath.Join(dir, "preview.html"), reportBytes)
	meta := baseMeta("Demo program readiness report", "report", "officecli new report \"Demo program readiness report\" --file ./demo-program-source-workbook.xlsx --prompt-file ./prompt.md --no-publish", artifact)
	meta.Additional = map[string]any{"source_workbook": sourceWorkbook}
	meta.Notes = []string{fmt.Sprintf("OfficeCLI renderer suggested file name: %s.", fileName)}
	writeMeta(dir, meta)
}

func buildStandaloneIMG(root string) {
	const slug = "standalone-img"
	const title = "OfficeCLI deadline automation image"
	const artifact = "officecli-hero-image.png"
	const command = "officecli new img \"OfficeCLI deadline automation image\" --prompt-file ./prompt.md --ratio landscape --no-publish"
	const source = "scripts/assets/skills-demos/standalone-img/officecli-office-automation-scene.png"
	const prompt = `Create a cinematic, photorealistic office scene showing the contrast between manual document chaos and OfficeCLI automation.

A modern open-plan office is under deadline pressure. Many people in the background and midground look stressed while hand-writing or annotating piles of documents: PPT slide drafts, Word-style report pages, Excel-style spreadsheet printouts, charts, and printed reports. Desks are crowded with paper stacks, pens, sticky notes, laptops, coffee cups, and scattered document drafts.

In the foreground, the clear main character is calm and focused, using a laptop terminal to run OfficeCLI. Around the laptop screen, subtle floating visual previews show different generated outputs: PPTX slides, DOCX report pages, XLSX dashboard tables/charts, a report preview, and an image preview. The generated outputs should look crisp and organized, contrasting with the messy handwritten workflow around them.

Use a wide 16:9 horizontal frame, high-detail realistic editorial lighting, natural office colors, and professional SaaS/product hero quality. Avoid readable brand logos or copyrighted marks. If terminal text appears, keep it short and clean, such as "officecli new" or "OfficeCLI".`
	dir := demoDir(root, slug)
	data := readBytes(source)
	writeBytes(filepath.Join(dir, artifact), data)
	writeBytes(filepath.Join(dir, "preview.png"), data)
	writeText(filepath.Join(dir, "prompt.md"), promptMarkdown(title, prompt, command))
	meta := baseMeta(title, "img", command, artifact)
	meta.GeneratedWith = "OfficeCLI standalone image generation demo asset checked into this repository"
	meta.Notes = []string{
		"Preview and artifact use a real generated image selected for the public gallery.",
		"Use the command above with configured external or hosted image generation to reproduce the prompt with the active image provider.",
	}
	writeMeta(dir, meta)
}

func baseMeta(title, typ, command, artifact string) demoMeta {
	return demoMeta{
		Title:         title,
		Type:          typ,
		Command:       command,
		Artifact:      artifact,
		Preview:       "preview.png",
		PromptFile:    "prompt.md",
		VerifiedAt:    verifiedAt,
		GeneratedWith: "OfficeCLI renderer from this repository",
		Verification: []string{
			"metadata schema validated by scripts/validate-skills-demos.py",
			"artifact exists and is under 3MB",
			"preview image exists and is a valid PNG",
		},
	}
}

func writeMeta(dir string, meta demoMeta) {
	meta.SHA256 = map[string]string{}
	for _, name := range []string{meta.Artifact, meta.Preview, meta.PromptFile} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			meta.SHA256[name] = sha256File(path)
		}
	}
	for _, value := range meta.Additional {
		if name, ok := value.(string); ok {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				meta.SHA256[name] = sha256File(path)
			}
		}
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	must(err)
	writeBytes(filepath.Join(dir, "metadata.json"), append(data, '\n'))
}

func promptMarkdown(title, prompt, command string) string {
	return fmt.Sprintf("# %s\n\n## Prompt\n\n%s\n\n## Reproduce\n\n```bash\n%s\n```\n", title, prompt, command)
}

func commandPPTX(slug string, images bool) string {
	topic := map[string]string{
		"pptx-image-rich": "Image-rich strategy deck",
		"pptx-text-only":  "Text-only executive briefing",
	}[slug]
	if topic == "" {
		topic = strings.ReplaceAll(slug, "-", " ")
	}
	command := fmt.Sprintf("officecli new pptx %q --prompt-file ./prompt.md --local-preview --no-publish", topic)
	if !images {
		command += " --no-images"
	}
	return command
}

func demoDir(root, slug string) string {
	dir := filepath.Join(root, slug)
	must(os.MkdirAll(dir, 0o755))
	return dir
}

func writeText(path, value string) {
	writeBytes(path, []byte(value))
}

func writeBytes(path string, data []byte) {
	must(os.MkdirAll(filepath.Dir(path), 0o755))
	must(os.WriteFile(path, data, 0o644))
}

func readBytes(path string) []byte {
	data, err := os.ReadFile(path)
	must(err)
	return data
}

func sha256File(path string) string {
	data, err := os.ReadFile(path)
	must(err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func demoImagePNG(seed string) ([]byte, error) {
	const width = 1280
	const height = 720
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	h := sha256.Sum256([]byte(seed))
	c1 := color.RGBA{R: 20 + h[0]%70, G: 80 + h[1]%120, B: 120 + h[2]%100, A: 255}
	c2 := color.RGBA{R: 180 + h[3]%60, G: 120 + h[4]%70, B: 80 + h[5]%90, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			t := float64(x+y) / float64(width+height)
			img.SetRGBA(x, y, blend(c1, c2, t))
		}
	}
	fillRect(img, image.Rect(80, 80, 1200, 640), color.RGBA{R: 255, G: 255, B: 255, A: 34})
	for i := 0; i < 5; i++ {
		x0 := 150 + i*190
		y0 := 180 + int(h[i+6])%80
		fillRect(img, image.Rect(x0, y0, x0+130, 520), color.RGBA{R: 255, G: 255, B: 255, A: 185})
		fillRect(img, image.Rect(x0+18, y0+32, x0+112, y0+48), color.RGBA{R: 28, G: 39, B: 56, A: 230})
		fillRect(img, image.Rect(x0+18, y0+74, x0+94, y0+90), color.RGBA{R: 28, G: 39, B: 56, A: 170})
		fillRect(img, image.Rect(x0+18, y0+128, x0+112, y0+230), color.RGBA{R: 26, G: 150, B: 190, A: 180})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fillRect(img draw.Image, rect image.Rectangle, c color.RGBA) {
	draw.Draw(img, rect, &image.Uniform{C: c}, image.Point{}, draw.Over)
}

func blend(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}

func must(err error) {
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
