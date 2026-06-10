package runtime

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	"github.com/officecli/officecli/internal/runtime/pptxrender"
	"github.com/officecli/officecli/pkg/officegen"
)

const (
	goSpineSlideWidth  = 1280
	goSpineSlideHeight = 720
)

func buildPPTXWithGoSpine(ctx context.Context, payload pptxPayload, fallback string, localPreview bool) ([]byte, string, []engine.GenerateIssue, []byte, []byte, error) {
	ir, assets := presentationIRFromPPTXPayload(payload, fallback)
	renderer := pptxrender.NewRenderer(pptxrender.RenderOptions{
		Assets: pptxrender.NewMapAssetResolver(assets),
	})
	fileBytes, report, err := renderer.Render(ctx, ir)
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("document assembly failed: generate pptx with go-spine: %w", err)
	}

	var warnings []engine.GenerateIssue
	for _, warning := range report.Warnings {
		if strings.Contains(strings.ToLower(warning), "native chart downgraded") && !hasWarningCode(warnings, "WARN_PPTX_NATIVE_CHART_FALLBACK") {
			warnings = append(warnings, engine.GenerateIssue{
				Code:    "WARN_PPTX_NATIVE_CHART_FALLBACK",
				Field:   "slides.chart",
				Message: "Go/spine PPTX renderer v1 downgraded native charts to editable shape fallback.",
			})
		}
	}

	var previewHTML []byte
	var previewJSON []byte
	if localPreview {
		previewMessages := make([]string, 0, len(warnings))
		for _, warning := range warnings {
			if strings.TrimSpace(warning.Message) != "" {
				previewMessages = append(previewMessages, warning.Message)
			}
		}
		previewJSON, _ = officegen.BuildLocalPreviewJSON(payload.Title, payload.StylePreset, payload.Theme, payload.Slides, previewMessages)
		previewHTML = officegen.BuildLocalPreviewHTML(payload.Title, payload.StylePreset, payload.Theme, payload.Slides, previewMessages)
	}

	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = generateengine.ExtractTitleFromDescription(fallback)
	}
	if title == "" {
		title = "presentation"
	}
	return fileBytes, fmt.Sprintf("%s.pptx", generateengine.SanitizeFileName(title)), warnings, previewHTML, previewJSON, nil
}

func presentationIRFromPPTXPayload(payload pptxPayload, fallback string) (pptxrender.PresentationIR, map[string]pptxrender.AssetData) {
	theme := goSpineTheme(payload)
	assets := make(map[string]pptxrender.AssetData)
	title := firstNonEmpty(strings.TrimSpace(payload.Title), generateengine.ExtractTitleFromDescription(fallback), "Presentation")
	ir := pptxrender.PresentationIR{
		Version:   1,
		SlideSize: pptxrender.SlideSize{Width: goSpineSlideWidth, Height: goSpineSlideHeight},
		Theme:     theme.IRTheme,
		Slides:    make([]pptxrender.SlideIR, 0, len(payload.Slides)),
	}
	for index, slide := range payload.Slides {
		ir.Slides = append(ir.Slides, goSpineSlideIR(index, len(payload.Slides), title, slide, theme, assets))
	}
	return ir, assets
}

type goSpineThemeSpec struct {
	IRTheme    pptxrender.Theme
	Background string
	Surface    string
	Soft       string
	Text       string
	TitleText  string
	MutedText  string
	Accent1    string
	Accent2    string
	Border     string
	TitleFont  string
	BodyFont   string
}

func goSpineTheme(payload pptxPayload) goSpineThemeSpec {
	preset := officegen.NormalizeStylePreset(payload.StylePreset)
	theme := payload.Theme
	if theme == nil {
		theme = officegen.DefaultThemeForPreset(preset)
	}
	if theme == nil {
		theme = officegen.DefaultThemeForPreset(officegen.StylePresetTechContrast)
	}

	background := goSpineHex(firstNonEmpty(theme.BgColor1, "FFFFFF"))
	text := goSpineHex(firstNonEmpty(theme.TextColor, "172033"))
	titleText := goSpineHex(firstNonEmpty(theme.TitleTextColor, theme.TextColor, "172033"))
	accent1 := goSpineHex(firstNonEmpty(theme.PrimaryColor, "2563EB"))
	accent2 := goSpineHex(firstNonEmpty(theme.AccentColor, "0F9F6E"))
	border := goSpineHex(firstNonEmpty(theme.HighlightColor, "CBD5E1"))
	muted := "#64748B"
	surface := "#FFFFFF"
	soft := "#F8FAFC"
	if goSpineIsDark(background) {
		muted = "#CBD5E1"
		surface = "#111827"
		soft = "#1F2937"
	}
	titleFont := firstNonEmpty(strings.TrimSpace(theme.FontFamily), "Aptos Display")
	bodyFont := firstNonEmpty(strings.TrimSpace(theme.FontFamily), "Aptos")
	return goSpineThemeSpec{
		IRTheme: pptxrender.Theme{
			Colors: map[string]string{
				"background": background,
				"text":       text,
				"mutedText":  muted,
				"accent1":    accent1,
				"accent2":    accent2,
				"border":     border,
			},
			Fonts: map[string]string{
				"title": titleFont,
				"body":  bodyFont,
			},
		},
		Background: background,
		Surface:    surface,
		Soft:       soft,
		Text:       text,
		TitleText:  titleText,
		MutedText:  muted,
		Accent1:    accent1,
		Accent2:    accent2,
		Border:     border,
		TitleFont:  titleFont,
		BodyFont:   bodyFont,
	}
}

func goSpineSlideIR(index, total int, deckTitle string, slide officegen.Slide, theme goSpineThemeSpec, assets map[string]pptxrender.AssetData) pptxrender.SlideIR {
	background := goSpineHex(firstNonEmpty(slide.BgColor, theme.Background))
	out := pptxrender.SlideIR{
		Name:       firstNonEmpty(strings.TrimSpace(slide.Title), fmt.Sprintf("Slide %d", index+1)),
		Background: background,
	}
	layout := strings.ToLower(strings.TrimSpace(slide.Layout))
	if layout == "" && slide.IsTitle {
		layout = "title"
	}
	if layout == "" {
		layout = "content"
	}

	switch layout {
	case "title":
		goSpineAppendTitleSlide(&out, slide, deckTitle, theme, assets, index)
	case "chapter":
		goSpineAppendChapterSlide(&out, slide, theme)
	case "dashboard":
		goSpineAppendDashboardSlide(&out, slide, theme, assets, index)
	case "chart":
		goSpineAppendChartSlide(&out, slide, theme)
	case "gallery":
		goSpineAppendGallerySlide(&out, slide, theme, assets, index)
	case "toc":
		goSpineAppendTOCSlide(&out, slide, theme)
	case "closing":
		goSpineAppendClosingSlide(&out, slide, theme)
	default:
		goSpineAppendContentSlide(&out, slide, theme, assets, index, total)
	}
	goSpineAppendFooter(&out, slide, theme, index, total)
	return out
}

func goSpineAppendTitleSlide(out *pptxrender.SlideIR, slide officegen.Slide, deckTitle string, theme goSpineThemeSpec, assets map[string]pptxrender.AssetData, index int) {
	imageFrame := goSpinePrimaryImageFrame(slide)
	if imageFrame != nil {
		goSpineAppendPrimaryImage(out, slide, assets, index, *imageFrame)
	}
	titleWidth := 760.0
	if imageFrame == nil || strings.EqualFold(strings.TrimSpace(slide.ImagePos), "background") {
		titleWidth = 980
	}
	out.Elements = append(out.Elements,
		goSpineShape("rect", 72, 72, 12, 118, theme.Accent1, theme.Accent1),
		goSpineText(firstNonEmpty(slide.Title, deckTitle), 104, 70, titleWidth, 120, 46, true, theme.TitleText, theme.TitleFont, "left"),
	)
	subtitle := firstNonEmpty(slide.Subtitle, slide.Content)
	if subtitle != "" {
		out.Elements = append(out.Elements, goSpineText(subtitle, 106, 200, titleWidth, 92, 23, false, theme.Text, theme.BodyFont, "left"))
	}
	if len(slide.Points) > 0 {
		out.Elements = append(out.Elements, goSpineText(goSpineBulletText(slide.Points), 106, 324, titleWidth, 180, 20, false, theme.Text, theme.BodyFont, "left"))
	}
}

func goSpineAppendChapterSlide(out *pptxrender.SlideIR, slide officegen.Slide, theme goSpineThemeSpec) {
	out.Elements = append(out.Elements,
		goSpineShape("rect", 120, 220, 180, 8, theme.Accent1, theme.Accent1),
		goSpineText(slide.Title, 120, 250, 980, 120, 44, true, theme.TitleText, theme.TitleFont, "left"),
	)
	if slide.Subtitle != "" || slide.Content != "" {
		out.Elements = append(out.Elements, goSpineText(firstNonEmpty(slide.Subtitle, slide.Content), 120, 380, 820, 90, 22, false, theme.Text, theme.BodyFont, "left"))
	}
}

func goSpineAppendDashboardSlide(out *pptxrender.SlideIR, slide officegen.Slide, theme goSpineThemeSpec, assets map[string]pptxrender.AssetData, index int) {
	goSpineAppendSlideTitle(out, slide, theme)
	cardTop := 150.0
	cardCount := len(slide.Metrics)
	if cardCount == 0 {
		goSpineAppendContentSlide(out, slide, theme, assets, index, 1)
		return
	}
	cols := math.Min(4, float64(cardCount))
	cardW := (1136 - (cols-1)*22) / cols
	for metricIndex, metric := range slide.Metrics {
		row := metricIndex / int(cols)
		col := metricIndex % int(cols)
		left := 72 + float64(col)*(cardW+22)
		top := cardTop + float64(row)*132
		goSpineAppendMetricCard(out, metric, left, top, cardW, 108, theme)
	}
	contentTop := cardTop + math.Ceil(float64(cardCount)/cols)*132 + 18
	if slide.Chart != nil {
		goSpineAppendChart(out, slide.Chart, 92, contentTop+60, 1000, 230, theme)
	}
	if slide.Content != "" || len(slide.Points) > 0 {
		out.Elements = append(out.Elements, goSpineText(goSpineBodyText(slide), 92, contentTop, 980, 90, 18, false, theme.Text, theme.BodyFont, "left"))
	}
}

func goSpineAppendChartSlide(out *pptxrender.SlideIR, slide officegen.Slide, theme goSpineThemeSpec) {
	goSpineAppendSlideTitle(out, slide, theme)
	chartLeft := 92.0
	chartWidth := 760.0
	if len(slide.Points) == 0 && slide.Content == "" {
		chartWidth = 1040
	}
	if slide.Chart != nil {
		goSpineAppendChart(out, slide.Chart, chartLeft, 220, chartWidth, 360, theme)
	}
	if slide.Content != "" || len(slide.Points) > 0 {
		out.Elements = append(out.Elements,
			goSpineShape("roundRect", 890, 220, 300, 260, theme.Soft, theme.Border),
			goSpineText(goSpineBodyText(slide), 914, 246, 252, 210, 18, false, theme.Text, theme.BodyFont, "left"),
		)
	}
}

func goSpineAppendGallerySlide(out *pptxrender.SlideIR, slide officegen.Slide, theme goSpineThemeSpec, assets map[string]pptxrender.AssetData, index int) {
	goSpineAppendSlideTitle(out, slide, theme)
	visuals := goSpineEmbeddedVisuals(slide)
	if len(visuals) == 0 {
		goSpineAppendContentSlide(out, slide, theme, assets, index, 1)
		return
	}
	cols := math.Min(3, float64(len(visuals)))
	frameW := (1136 - (cols-1)*24) / cols
	frameH := 230.0
	for visualIndex, visual := range visuals {
		row := visualIndex / int(cols)
		col := visualIndex % int(cols)
		left := 72 + float64(col)*(frameW+24)
		top := 165 + float64(row)*285
		assetID := goSpineAddAsset(assets, fmt.Sprintf("slide-%02d-visual-%02d", index+1, visualIndex+1), visual.ImageData, visual.ImageMIME, firstNonEmpty(visual.Label, visual.Caption))
		out.Elements = append(out.Elements,
			goSpineImage(assetID, left, top, frameW, frameH, firstNonEmpty(visual.Label, visual.Caption)),
			goSpineText(firstNonEmpty(visual.Caption, visual.Label), left, top+frameH+12, frameW, 46, 15, false, theme.MutedText, theme.BodyFont, "left"),
		)
	}
}

func goSpineAppendTOCSlide(out *pptxrender.SlideIR, slide officegen.Slide, theme goSpineThemeSpec) {
	goSpineAppendSlideTitle(out, slide, theme)
	items := append([]string(nil), slide.Points...)
	for _, section := range slide.Sections {
		items = append(items, firstNonEmpty(section.Heading, section.Detail))
	}
	if len(items) == 0 && slide.Content != "" {
		items = append(items, splitContentToPoints(slide.Content, 6)...)
	}
	for i, item := range items {
		top := 170 + float64(i)*72
		out.Elements = append(out.Elements,
			goSpineShape("ellipse", 90, top+2, 42, 42, theme.Accent1, theme.Accent1),
			goSpineText(fmt.Sprintf("%02d", i+1), 90, top+10, 42, 24, 15, true, "#FFFFFF", theme.BodyFont, "center"),
			goSpineText(item, 154, top, 900, 52, 22, false, theme.Text, theme.BodyFont, "left"),
		)
	}
}

func goSpineAppendClosingSlide(out *pptxrender.SlideIR, slide officegen.Slide, theme goSpineThemeSpec) {
	out.Elements = append(out.Elements,
		goSpineShape("roundRect", 96, 128, 1088, 420, theme.Soft, theme.Border),
		goSpineShape("rect", 96, 128, 12, 420, theme.Accent1, theme.Accent1),
		goSpineText(slide.Title, 138, 166, 860, 82, 40, true, theme.TitleText, theme.TitleFont, "left"),
	)
	body := goSpineBodyText(slide)
	if body != "" {
		out.Elements = append(out.Elements, goSpineText(body, 140, 270, 820, 160, 22, false, theme.Text, theme.BodyFont, "left"))
	}
	if len(slide.Sections) > 0 {
		top := 430.0
		for i, section := range limitSections(slide.Sections, 3) {
			left := 140 + float64(i)*315
			goSpineAppendSectionCard(out, section, left, top, 280, 92, theme)
		}
	}
}

func goSpineAppendContentSlide(out *pptxrender.SlideIR, slide officegen.Slide, theme goSpineThemeSpec, assets map[string]pptxrender.AssetData, index, total int) {
	goSpineAppendSlideTitle(out, slide, theme)
	contentLeft := 72.0
	contentWidth := 760.0
	if frame := goSpinePrimaryImageFrame(slide); frame != nil {
		goSpineAppendPrimaryImage(out, slide, assets, index, *frame)
		if frame.Left < 400 {
			contentLeft = 550
			contentWidth = 610
		} else {
			contentWidth = 720
		}
	}
	top := 160.0
	if len(slide.Metrics) > 0 {
		for i, metric := range limitMetrics(slide.Metrics, 3) {
			goSpineAppendMetricCard(out, metric, contentLeft+float64(i)*250, top, 220, 100, theme)
		}
		top += 130
	}
	if len(slide.Sections) > 0 {
		goSpineAppendSectionsGrid(out, slide.Sections, contentLeft, top, contentWidth, theme)
	} else {
		body := goSpineBodyText(slide)
		if body != "" {
			out.Elements = append(out.Elements, goSpineText(body, contentLeft, top, contentWidth, 320, 21, false, theme.Text, theme.BodyFont, "left"))
		}
	}
	if slide.Chart != nil {
		goSpineAppendChart(out, slide.Chart, contentLeft, 450, math.Min(contentWidth, 800), 210, theme)
	}
	_ = total
}

func goSpineAppendSlideTitle(out *pptxrender.SlideIR, slide officegen.Slide, theme goSpineThemeSpec) {
	out.Elements = append(out.Elements,
		goSpineText(slide.Title, 72, 56, 970, 64, 34, true, theme.TitleText, theme.TitleFont, "left"),
		goSpineShape("rect", 72, 128, 92, 5, theme.Accent1, theme.Accent1),
	)
	if slide.Subtitle != "" {
		out.Elements = append(out.Elements, goSpineText(slide.Subtitle, 72, 140, 920, 38, 17, false, theme.MutedText, theme.BodyFont, "left"))
	}
}

func goSpineAppendSectionsGrid(out *pptxrender.SlideIR, sections []officegen.SlideSection, left, top, width float64, theme goSpineThemeSpec) {
	sections = limitSections(sections, 6)
	if len(sections) == 0 {
		return
	}
	cols := 2.0
	if len(sections) <= 3 {
		cols = 1
	}
	cardGap := 20.0
	cardW := (width - (cols-1)*cardGap) / cols
	cardH := 118.0
	for i, section := range sections {
		col := i % int(cols)
		row := i / int(cols)
		goSpineAppendSectionCard(out, section, left+float64(col)*(cardW+cardGap), top+float64(row)*(cardH+18), cardW, cardH, theme)
	}
}

func goSpineAppendSectionCard(out *pptxrender.SlideIR, section officegen.SlideSection, left, top, width, height float64, theme goSpineThemeSpec) {
	out.Elements = append(out.Elements,
		goSpineShape("roundRect", left, top, width, height, theme.Surface, theme.Border),
		goSpineShape("rect", left, top, 6, height, theme.Accent2, theme.Accent2),
		goSpineText(section.Heading, left+20, top+18, width-34, 30, 18, true, theme.TitleText, theme.TitleFont, "left"),
		goSpineText(section.Detail, left+20, top+52, width-34, height-58, 15, false, theme.Text, theme.BodyFont, "left"),
	)
}

func goSpineAppendMetricCard(out *pptxrender.SlideIR, metric officegen.MetricCard, left, top, width, height float64, theme goSpineThemeSpec) {
	out.Elements = append(out.Elements,
		goSpineShape("roundRect", left, top, width, height, theme.Surface, theme.Border),
		goSpineText(metric.Label, left+18, top+16, width-36, 24, 15, false, theme.MutedText, theme.BodyFont, "left"),
		goSpineText(metric.Value, left+18, top+42, width-36, 38, 28, true, theme.Accent1, theme.TitleFont, "left"),
	)
	if metric.Note != "" {
		out.Elements = append(out.Elements, goSpineText(metric.Note, left+18, top+78, width-36, 22, 13, false, theme.MutedText, theme.BodyFont, "left"))
	}
}

func goSpineAppendChart(out *pptxrender.SlideIR, chart *officegen.ChartData, left, top, width, height float64, theme goSpineThemeSpec) {
	if chart == nil {
		return
	}
	values := append([]float64(nil), chart.Values...)
	categories := append([]string(nil), chart.Categories...)
	for len(categories) < len(values) {
		categories = append(categories, fmt.Sprintf("Item %d", len(categories)+1))
	}
	if len(values) == 0 {
		values = []float64{1}
		categories = []string{firstNonEmpty(chart.Title, "Value")}
	}
	out.Elements = append(out.Elements, pptxrender.Element{
		Type:       "chart",
		ChartType:  firstNonEmpty(strings.TrimSpace(chart.Type), "bar"),
		Title:      firstNonEmpty(strings.TrimSpace(chart.Title), "Chart"),
		BBox:       pptxrender.BBox{Left: left, Top: top, Width: width, Height: height},
		Categories: categories,
		Series: []pptxrender.SeriesIR{{
			Name:   firstNonEmpty(strings.TrimSpace(chart.Title), "Series"),
			Values: values,
			Color:  theme.Accent1,
		}},
	})
}

func goSpineAppendPrimaryImage(out *pptxrender.SlideIR, slide officegen.Slide, assets map[string]pptxrender.AssetData, slideIndex int, frame pptxrender.BBox) {
	if len(slide.ImageData) == 0 {
		return
	}
	assetID := goSpineAddAsset(assets, fmt.Sprintf("slide-%02d-primary", slideIndex+1), slide.ImageData, slide.ImageMIME, firstNonEmpty(slide.Title, "Slide image"))
	out.Elements = append(out.Elements, goSpineImage(assetID, frame.Left, frame.Top, frame.Width, frame.Height, firstNonEmpty(slide.Title, "Slide image")))
}

func goSpineAppendFooter(out *pptxrender.SlideIR, slide officegen.Slide, theme goSpineThemeSpec, index, total int) {
	if slide.Source != "" {
		out.Elements = append(out.Elements, goSpineText(slide.Source, 72, 666, 720, 24, 12, false, theme.MutedText, theme.BodyFont, "left"))
	}
	out.Elements = append(out.Elements,
		goSpineShape("rect", 72, 650, 1136, 1, theme.Border, theme.Border),
		goSpineText(fmt.Sprintf("%d / %d", index+1, total), 1120, 662, 88, 22, 12, false, theme.MutedText, theme.BodyFont, "right"),
	)
}

func goSpinePrimaryImageFrame(slide officegen.Slide) *pptxrender.BBox {
	if len(slide.ImageData) == 0 {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(slide.ImagePos)) {
	case "left":
		return &pptxrender.BBox{Left: 72, Top: 168, Width: 410, Height: 340}
	case "background":
		return &pptxrender.BBox{Left: 0, Top: 0, Width: goSpineSlideWidth, Height: goSpineSlideHeight}
	case "center", "top":
		return &pptxrender.BBox{Left: 720, Top: 148, Width: 430, Height: 300}
	default:
		return &pptxrender.BBox{Left: 790, Top: 122, Width: 390, Height: 360}
	}
}

func goSpineImage(assetID string, left, top, width, height float64, alt string) pptxrender.Element {
	return pptxrender.Element{
		Type:    "image",
		AssetID: assetID,
		Alt:     alt,
		Fit:     "cover",
		BBox:    pptxrender.BBox{Left: left, Top: top, Width: width, Height: height},
	}
}

func goSpineText(text string, left, top, width, height, fontSize float64, bold bool, color, font, align string) pptxrender.Element {
	return pptxrender.Element{
		Type: "text",
		Text: strings.TrimSpace(text),
		BBox: pptxrender.BBox{Left: left, Top: top, Width: width, Height: height},
		Style: pptxrender.TextStyle{
			FontSize: fontSize,
			Bold:     bold,
			Color:    color,
			Font:     font,
			Align:    align,
		},
	}
}

func goSpineShape(geometry string, left, top, width, height float64, fill, line string) pptxrender.Element {
	return pptxrender.Element{
		Type:     "shape",
		Geometry: geometry,
		BBox:     pptxrender.BBox{Left: left, Top: top, Width: width, Height: height},
		Fill:     fill,
		Line:     pptxrender.LineStyle{Color: line, Width: 1},
	}
}

func goSpineBodyText(slide officegen.Slide) string {
	var parts []string
	if strings.TrimSpace(slide.Content) != "" {
		parts = append(parts, strings.TrimSpace(slide.Content))
	}
	if len(slide.Points) > 0 {
		parts = append(parts, goSpineBulletText(slide.Points))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func goSpineBulletText(points []string) string {
	lines := make([]string, 0, len(points))
	for _, point := range points {
		point = strings.TrimSpace(point)
		if point == "" {
			continue
		}
		lines = append(lines, "- "+point)
	}
	return strings.Join(lines, "\n")
}

func goSpineEmbeddedVisuals(slide officegen.Slide) []officegen.SlideVisual {
	out := make([]officegen.SlideVisual, 0, len(slide.Visuals))
	for _, visual := range slide.Visuals {
		if len(visual.ImageData) == 0 {
			continue
		}
		out = append(out, visual)
	}
	return out
}

func goSpineAddAsset(assets map[string]pptxrender.AssetData, id string, data []byte, mimeType, alt string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		id = fmt.Sprintf("asset-%d", len(assets)+1)
	}
	ext := strings.TrimPrefix(filepath.Ext(id), ".")
	if ext == "" {
		ext = strings.TrimPrefix(imageExtensionFromMIME(mimeType), ".")
	}
	if ext == "" {
		ext = "png"
	}
	assets[id] = pptxrender.AssetData{
		Bytes:       append([]byte(nil), data...),
		Ext:         ext,
		ContentType: firstNonEmpty(strings.TrimSpace(mimeType), "image/png"),
		AltText:     strings.TrimSpace(alt),
	}
	return id
}

func goSpineHex(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "#000000"
	}
	if strings.HasPrefix(value, "#") {
		return value
	}
	return "#" + value
}

func goSpineIsDark(hexColor string) bool {
	hexColor = strings.TrimPrefix(strings.TrimSpace(hexColor), "#")
	if len(hexColor) != 6 {
		return false
	}
	var r, g, b int
	if _, err := fmt.Sscanf(hexColor, "%02x%02x%02x", &r, &g, &b); err != nil {
		return false
	}
	luma := (299*r + 587*g + 114*b) / 1000
	return luma < 80
}

func limitSections(sections []officegen.SlideSection, limit int) []officegen.SlideSection {
	if len(sections) <= limit {
		return sections
	}
	return sections[:limit]
}

func limitMetrics(metrics []officegen.MetricCard, limit int) []officegen.MetricCard {
	if len(metrics) <= limit {
		return metrics
	}
	return metrics[:limit]
}
