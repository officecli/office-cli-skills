package officegen

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"regexp"
	"strings"
	"time"
)

// SlideTheme defines the theme color palette.
type SlideTheme struct {
	PrimaryColor   string `json:"primaryColor"`             // Primary hex color (60%).
	AccentColor    string `json:"accentColor"`              // Accent hex color (30%).
	HighlightColor string `json:"highlightColor,omitempty"` // Highlight hex color (10%).
	BackgroundType string `json:"backgroundType"`           // "gradient" | "solid" | "dark"
	BgColor1       string `json:"bgColor1"`                 // Background color 1.
	BgColor2       string `json:"bgColor2,omitempty"`       // Background color 2 for gradients.
	TextColor      string `json:"textColor,omitempty"`      // Body text hex color.
	TitleTextColor string `json:"titleTextColor,omitempty"` // Title text hex color.
	FontFamily     string `json:"fontFamily,omitempty"`     // Latin font family.
	EAFontFamily   string `json:"eaFontFamily,omitempty"`   // East Asian font family.
}

// ChartData defines chart content.
type ChartData struct {
	Type       string    `json:"type"`       // "bar" | "pie" | "line"
	Categories []string  `json:"categories"` // X-axis labels.
	Values     []float64 `json:"values"`     // Numeric values.
	Title      string    `json:"title"`      // Chart title.
}

// MetricCard represents a large KPI card.
type MetricCard struct {
	Label string `json:"label"` // Metric label.
	Value string `json:"value"` // Metric value.
	Note  string `json:"note"`  // Supplemental note.
}

// SlideSection represents a heading-detail pair.
type SlideSection struct {
	Heading string `json:"heading"` // Primary heading.
	Detail  string `json:"detail"`  // Secondary detail.
}

// Slide represents a single slide.
type Slide struct {
	Role        string         `json:"role,omitempty"`     // cover | summary | evidence | detail | action
	Title       string         `json:"title"`              // Slide title.
	Content     string         `json:"content"`            // Slide body content.
	IsTitle     bool           `json:"isTitle"`            // Whether this is a title slide.
	Layout      string         `json:"layout,omitempty"`   // "title" | "content" | "chart" | "dashboard"
	Variant     string         `json:"variant,omitempty"`  // Optional visual variant.
	Subtitle    string         `json:"subtitle,omitempty"` // Subtitle for title layout.
	Points      []string       `json:"points,omitempty"`   // Simple bullet points.
	Sections    []SlideSection `json:"sections,omitempty"` // Structured heading-detail sections.
	Chart       *ChartData     `json:"chart,omitempty"`    // Chart data.
	Metrics     []MetricCard   `json:"metrics,omitempty"`  // Dashboard metric cards.
	Source      string         `json:"source,omitempty"`   // Data source footnote.
	BgColor     string         `json:"bgColor,omitempty"`  // Per-slide background color override.
	BgColor2    string         `json:"bgColor2,omitempty"` // Optional second gradient color.
	HasImage    bool           `json:"hasImage,omitempty"` // Whether the slide uses an image.
	ImagePrompt string         `json:"imagePrompt,omitempty"`
	ImagePos    string         `json:"imagePos,omitempty"` // "right" | "left" | "background" | "center" | "top" | "bottom" | "diagonal"
	ImageData   []byte         `json:"-"`
	ImageMIME   string         `json:"-"`
}

// PPTXOptions configures PPTX generation.
type PPTXOptions struct {
	Title       string      // Document title.
	Creator     string      // Document creator.
	Theme       *SlideTheme // Theme palette.
	StylePreset string      // Style preset.
}

// PPTXGenerator builds PPTX files.
type PPTXGenerator struct{}

// NewPPTXGenerator creates a PPTX generator instance.
func NewPPTXGenerator() *PPTXGenerator {
	return &PPTXGenerator{}
}

// defaultTheme returns the default theme palette.
func defaultTheme() *SlideTheme {
	return documentThemeToSlideTheme(DefaultDocumentTheme(DocumentPresetAnalysis))
}

// getTheme resolves a theme by filling in missing defaults.
func getTheme(theme *SlideTheme) *SlideTheme {
	def := defaultTheme()
	if theme == nil {
		return def
	}
	if theme.PrimaryColor == "" {
		theme.PrimaryColor = def.PrimaryColor
	}
	if theme.AccentColor == "" {
		theme.AccentColor = def.AccentColor
	}
	if theme.BackgroundType == "" {
		theme.BackgroundType = def.BackgroundType
	}
	if theme.BgColor1 == "" {
		theme.BgColor1 = def.BgColor1
	}
	if theme.BgColor2 == "" {
		theme.BgColor2 = def.BgColor2
	}
	if theme.FontFamily == "" {
		theme.FontFamily = def.FontFamily
	}
	if theme.EAFontFamily == "" {
		theme.EAFontFamily = def.EAFontFamily
	}
	return theme
}

func resolvedLayout(slide Slide) string {
	if slide.Layout != "" {
		return slide.Layout
	}
	if slide.IsTitle {
		return "title"
	}
	if len(slide.Metrics) > 0 {
		return "dashboard"
	}
	return "content"
}

func normalizeImagePos(pos string) string {
	switch strings.ToLower(strings.TrimSpace(pos)) {
	case "left", "split-left":
		return "left"
	case "right", "split-right":
		return "right"
	case "background", "fullbleed", "cover":
		return "background"
	case "center":
		return "center"
	case "top", "hero":
		return "top"
	case "bottom":
		return "bottom"
	case "diagonal", "corner":
		return "diagonal"
	default:
		return "right"
	}
}

func resolvedImagePos(slide Slide) string {
	if !slide.HasImage || len(slide.ImageData) == 0 {
		return ""
	}
	return requestedImagePos(slide)
}

func requestedImagePos(slide Slide) string {
	if !slide.HasImage {
		return ""
	}
	switch resolvedLayout(slide) {
	case "title":
		pos := normalizeImagePos(slide.ImagePos)
		if pos == "background" || pos == "center" {
			return pos
		}
		return "center"
	case "content":
		pos := normalizeImagePos(slide.ImagePos)
		if pos == "background" {
			return "right"
		}
		return pos
	default:
		return ""
	}
}

func hasEmbeddedImage(slide Slide) bool {
	return resolvedImagePos(slide) != ""
}

type imageFrame struct {
	x  int
	y  int
	cx int
	cy int
}

func imageFrameForSlide(slide Slide) (imageFrame, bool) {
	imagePos := requestedImagePos(slide)
	if imagePos == "" {
		return imageFrame{}, false
	}

	if resolvedLayout(slide) == "title" {
		switch imagePos {
		case "background":
			return imageFrame{x: 0, y: 0, cx: 12192000, cy: 6858000}, true
		case "center":
			return imageFrame{x: 2460000, y: 1550000, cx: 7272000, cy: 3200000}, true
		default:
			return imageFrame{}, false
		}
	}

	switch imagePos {
	case "right":
		return imageFrame{x: 6400000, y: 1450000, cx: 4800000, cy: 4300000}, true
	case "left":
		return imageFrame{x: 700000, y: 1450000, cx: 4800000, cy: 4300000}, true
	case "center":
		return imageFrame{x: 2460000, y: 1500000, cx: 7272000, cy: 3000000}, true
	case "top":
		return imageFrame{x: 1460000, y: 1200000, cx: 9272000, cy: 2000000}, true
	case "bottom":
		return imageFrame{x: 1460000, y: 4300000, cx: 9272000, cy: 1800000}, true
	case "diagonal":
		return imageFrame{x: 6900000, y: 1200000, cx: 4000000, cy: 2600000}, true
	default:
		return imageFrame{}, false
	}
}

func TargetAspectRatioForSlide(slide Slide) float64 {
	frame, ok := imageFrameForSlide(slide)
	if !ok || frame.cy == 0 {
		return 0
	}
	return float64(frame.cx) / float64(frame.cy)
}

func imageExtensionFromMIME(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/jpeg", "image/jpg":
		return "jpeg"
	default:
		return "png"
	}
}

// Generate builds a PPTX file and returns its bytes.
func (g *PPTXGenerator) Generate(slides []Slide, opts PPTXOptions) ([]byte, error) {
	if len(slides) == 0 {
		return nil, fmt.Errorf("slides cannot be empty")
	}

	theme := getTheme(opts.Theme)
	stylePreset := ResolveStylePreset(opts.StylePreset)
	if strings.TrimSpace(opts.StylePreset) != "" {
		theme = MergeThemeWithPreset(opts.Theme, opts.StylePreset)
	}

	// Count charts for Content_Types registration
	chartCount := 0
	imageCount := 0
	for _, s := range slides {
		if s.Chart != nil && (s.Layout == "chart" || s.Layout == "dashboard") {
			chartCount++
		}
		if hasEmbeddedImage(s) {
			imageCount++
		}
	}

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// Build all required package files.
	files := g.buildBaseFiles(opts, len(slides), chartCount, imageCount > 0, theme)

	// Add slide files and chart files, including embedded XLSX binaries.
	slideFiles, binaryFiles, err := g.generateSlidesWithEmbeds(slides, theme, stylePreset)
	if err != nil {
		return nil, err
	}
	for path, content := range slideFiles {
		files[path] = content
	}

	// Write all XML files into the ZIP archive.
	for path, content := range files {
		f, err := w.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create file %s: %w", path, err)
		}
		_, err = f.Write([]byte(content))
		if err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	// Write embedded binary files such as chart XLSX payloads.
	for path, data := range binaryFiles {
		f, err := w.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create binary file %s: %w", path, err)
		}
		if _, err = f.Write(data); err != nil {
			return nil, fmt.Errorf("failed to write binary file %s: %w", path, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// buildBaseFiles builds the base XML package files.
func (g *PPTXGenerator) buildBaseFiles(opts PPTXOptions, slideCount, chartCount int, hasImages bool, theme *SlideTheme) map[string]string {
	return map[string]string{
		"[Content_Types].xml":                          g.generateContentTypes(slideCount, chartCount, hasImages),
		"_rels/.rels":                                  rootRels,
		"docProps/core.xml":                            g.generateCoreXML(opts),
		"docProps/app.xml":                             g.generateAppXML(slideCount),
		"ppt/presentation.xml":                         g.generatePresentationXML(slideCount),
		"ppt/_rels/presentation.xml.rels":              g.generatePresentationRels(slideCount),
		"ppt/theme/theme1.xml":                         generateThemeXML(theme),
		"ppt/presProps.xml":                            presPropsXML,
		"ppt/viewProps.xml":                            viewPropsXML,
		"ppt/tableStyles.xml":                          tableStylesXML,
		"ppt/slideMasters/slideMaster1.xml":            slideMasterXML,
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": slideMasterRels,
		"ppt/slideLayouts/slideLayout1.xml":            slideLayoutXML,
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": slideLayoutRels,
	}
}

// generateSlidesWithEmbeds generates all slide XMLs and embedded chart files.
func (g *PPTXGenerator) generateSlidesWithEmbeds(slides []Slide, theme *SlideTheme, stylePreset PPTXStylePreset) (map[string]string, map[string][]byte, error) {
	result := make(map[string]string)
	binaries := make(map[string][]byte)

	chartIndex := 0
	imageIndex := 0
	for i, slide := range slides {
		slideNum := i + 1
		slidePath := fmt.Sprintf("ppt/slides/slide%d.xml", slideNum)
		relsPath := fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", slideNum)

		hasChart := slide.Chart != nil && (slide.Layout == "chart" || slide.Layout == "dashboard")
		hasImage := !hasChart && hasEmbeddedImage(slide)

		if hasChart {
			chartIndex++

			// Chart XML
			result[fmt.Sprintf("ppt/charts/chart%d.xml", chartIndex)] = g.createChartXML(slide.Chart)

			// Chart style (pie uses different style)
			if slide.Chart.Type == "pie" {
				result[fmt.Sprintf("ppt/charts/style%d.xml", chartIndex)] = chartStyleXMLPie
			} else {
				result[fmt.Sprintf("ppt/charts/style%d.xml", chartIndex)] = chartStyleXMLDefault
			}

			// Chart colors
			result[fmt.Sprintf("ppt/charts/colors%d.xml", chartIndex)] = chartColorsXML

			// Chart rels
			result[fmt.Sprintf("ppt/charts/_rels/chart%d.xml.rels", chartIndex)] = g.createChartRelsXML(chartIndex)

			// Slide rels with chart reference
			result[relsPath] = g.createSlideRelsWithChart(chartIndex)
		} else if hasImage {
			imageIndex++
			imageExt := imageExtensionFromMIME(slide.ImageMIME)
			result[relsPath] = g.createSlideRelsWithImage(imageIndex, imageExt)
			binaries[fmt.Sprintf("ppt/media/image%d.%s", imageIndex, imageExt)] = slide.ImageData
		} else {
			result[relsPath] = slideRels
		}

		result[slidePath] = g.createSlideXMLEnhanced(slide, theme, stylePreset, hasChart, chartIndex, slideNum, len(slides))
	}

	return result, binaries, nil
}

// generateContentTypes builds [Content_Types].xml dynamically.
func (g *PPTXGenerator) generateContentTypes(slideCount, chartCount int, hasImages ...bool) string {
	slideOverrides := ""
	for i := 1; i <= slideCount; i++ {
		slideOverrides += fmt.Sprintf(`    <Override PartName="/ppt/slides/slide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>
`, i)
	}

	chartOverrides := ""
	for i := 1; i <= chartCount; i++ {
		chartOverrides += fmt.Sprintf(`    <Override PartName="/ppt/charts/chart%d.xml" ContentType="application/vnd.openxmlformats-officedocument.drawingml.chart+xml"/>
`, i)
		chartOverrides += fmt.Sprintf(`    <Override PartName="/ppt/charts/style%d.xml" ContentType="application/vnd.ms-office.chartstyle+xml"/>
`, i)
		chartOverrides += fmt.Sprintf(`    <Override PartName="/ppt/charts/colors%d.xml" ContentType="application/vnd.ms-office.chartcolorstyle+xml"/>
`, i)
	}

	// When images exist, register PNG and JPEG default extensions.
	imageDefaults := ""
	if len(hasImages) > 0 && hasImages[0] {
		imageDefaults = `    <Default Extension="png" ContentType="image/png"/>
    <Default Extension="jpeg" ContentType="image/jpeg"/>
`
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
    <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
    <Default Extension="xml" ContentType="application/xml"/>
%s    <Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
    <Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>
    <Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>
%s%s    <Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>
    <Override PartName="/ppt/presProps.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presProps+xml"/>
    <Override PartName="/ppt/viewProps.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.viewProps+xml"/>
    <Override PartName="/ppt/tableStyles.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.tableStyles+xml"/>
    <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
    <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`, imageDefaults, slideOverrides, chartOverrides)
}

// generateCoreXML builds docProps/core.xml.
func (g *PPTXGenerator) generateCoreXML(opts PPTXOptions) string {
	title := opts.Title
	if title == "" {
		title = "Untitled Presentation"
	}
	creator := opts.Creator
	if creator == "" {
		creator = "officecli"
	}
	createdTime := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" 
    xmlns:dc="http://purl.org/dc/elements/1.1/" 
    xmlns:dcterms="http://purl.org/dc/terms/" 
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
    <dc:title>%s</dc:title>
    <dc:creator>%s</dc:creator>
    <dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created>
</cp:coreProperties>`, escapeXML(title), escapeXML(creator), createdTime)
}

// generateAppXML builds docProps/app.xml.
func (g *PPTXGenerator) generateAppXML(slideCount int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
    <Application>officecli PPTX Generator</Application>
    <Slides>%d</Slides>
</Properties>`, slideCount)
}

// generatePresentationXML builds ppt/presentation.xml dynamically.
func (g *PPTXGenerator) generatePresentationXML(slideCount int) string {
	slideIdList := ""
	for i := 1; i <= slideCount; i++ {
		slideIdList += fmt.Sprintf(`        <p:sldId id="%d" r:id="rId%d"/>
`, 255+i, i+1)
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" 
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" 
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
    <p:sldMasterIdLst>
        <p:sldMasterId id="2147483648" r:id="rId1"/>
    </p:sldMasterIdLst>
    <p:sldIdLst>
%s    </p:sldIdLst>
    <p:sldSz cx="12192000" cy="6858000"/>
    <p:notesSz cx="6858000" cy="9144000"/>
    <p:defaultTextStyle>
        <a:defPPr>
            <a:defRPr lang="zh-CN"/>
        </a:defPPr>
    </p:defaultTextStyle>
</p:presentation>`, slideIdList)
}

// generatePresentationRels builds ppt/_rels/presentation.xml.rels dynamically.
func (g *PPTXGenerator) generatePresentationRels(slideCount int) string {
	relationships := `    <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>
`
	for i := 1; i <= slideCount; i++ {
		relationships += fmt.Sprintf(`    <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>
`, i+1, i)
	}
	themeID := slideCount + 2
	relationships += fmt.Sprintf(`    <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>
    <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/presProps" Target="presProps.xml"/>
    <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/viewProps" Target="viewProps.xml"/>
    <Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/tableStyles" Target="tableStyles.xml"/>
`, themeID, themeID+1, themeID+2, themeID+3)

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
%s</Relationships>`, relationships)
}

// isDarkTheme reports whether the theme uses a dark background.
func isDarkTheme(theme *SlideTheme) bool {
	return theme.BackgroundType == "dark"
}

// isDarkColor reports whether a hex color is dark based on relative luminance.
func isDarkColor(hexColor string) bool {
	return colorLuminance(hexColor) < 128
}

// colorLuminance computes the relative luminance of a color on a 0-255 scale.
func colorLuminance(hexColor string) float64 {
	if len(hexColor) != 6 {
		return 128 // Unknown colors fall back to mid luminance.
	}
	r, g, b := 0, 0, 0
	fmt.Sscanf(hexColor, "%02x%02x%02x", &r, &g, &b)
	return 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
}

// blendColor mixes two colors by ratio: 0 keeps color1 and 1 keeps color2.
func blendColor(hexColor1, hexColor2 string, ratio float64) string {
	if len(hexColor1) != 6 || len(hexColor2) != 6 {
		return hexColor1
	}
	r1, g1, b1 := 0, 0, 0
	r2, g2, b2 := 0, 0, 0
	fmt.Sscanf(hexColor1, "%02x%02x%02x", &r1, &g1, &b1)
	fmt.Sscanf(hexColor2, "%02x%02x%02x", &r2, &g2, &b2)
	r := int(float64(r1)*(1-ratio) + float64(r2)*ratio)
	g := int(float64(g1)*(1-ratio) + float64(g2)*ratio)
	b := int(float64(b1)*(1-ratio) + float64(b2)*ratio)
	if r > 255 {
		r = 255
	}
	if g > 255 {
		g = 255
	}
	if b > 255 {
		b = 255
	}
	return fmt.Sprintf("%02X%02X%02X", r, g, b)
}

// sanitizeGradient fixes unsafe gradients. When bgColor1 and bgColor2 differ too
// much in luminance, text becomes unreadable on one side. The fix pulls bgColor2
// toward bgColor1 so the gradient stays within the same luminance direction.
func sanitizeGradient(bgColor1, bgColor2 string) (string, string) {
	if bgColor1 == "" || bgColor2 == "" || len(bgColor1) != 6 || len(bgColor2) != 6 {
		return bgColor1, bgColor2
	}
	lum1 := colorLuminance(bgColor1)
	lum2 := colorLuminance(bgColor2)
	lumDiff := lum1 - lum2
	if lumDiff < 0 {
		lumDiff = -lumDiff
	}

	// A luminance gap above 80 is treated as too large. Anything beyond that is
	// pulled back toward bgColor1.
	const maxLumDiff = 80.0
	if lumDiff <= maxLumDiff {
		return bgColor1, bgColor2 // The gradient is safe as-is.
	}

	// Compute the blend ratio that moves bgColor2 toward bgColor1.
	ratio := 1.0 - maxLumDiff/lumDiff
	newBgColor2 := blendColor(bgColor2, bgColor1, ratio)
	return bgColor1, newBgColor2
}

// getTextColor returns the default body text color for the theme.
func getTextColor(theme *SlideTheme) string {
	if isDarkTheme(theme) {
		return "EEEEEE"
	}
	return "333333"
}

// getSlideTextColor returns the body text color based on the slide background or theme.
func getSlideTextColor(slide Slide, theme *SlideTheme) string {
	if slide.BgColor != "" && isDarkColor(slide.BgColor) {
		return "EEEEEE"
	}
	return getTextColor(theme)
}

// isSimilarHue checks whether two colors are too close in hue and contrast.
// It is used to avoid unreadable text on same-family backgrounds.
func isSimilarHue(hexColor1, hexColor2 string) bool {
	if len(hexColor1) != 6 || len(hexColor2) != 6 {
		return false
	}
	r1, g1, b1 := 0, 0, 0
	r2, g2, b2 := 0, 0, 0
	fmt.Sscanf(hexColor1, "%02x%02x%02x", &r1, &g1, &b1)
	fmt.Sscanf(hexColor2, "%02x%02x%02x", &r2, &g2, &b2)

	// Compute the luminance gap between the colors.
	lum1 := 0.299*float64(r1) + 0.587*float64(g1) + 0.114*float64(b1)
	lum2 := 0.299*float64(r2) + 0.587*float64(g2) + 0.114*float64(b2)
	lumDiff := lum1 - lum2
	if lumDiff < 0 {
		lumDiff = -lumDiff
	}
	// A large luminance gap is treated as sufficient contrast.
	if lumDiff > 100 {
		return false
	}

	// Compute RGB Euclidean distance to catch overly similar colors.
	dr := float64(r1 - r2)
	dg := float64(g1 - g2)
	db := float64(b1 - b2)
	dist := dr*dr + dg*dg + db*db
	// Distances below this threshold are considered too similar.
	return dist < 14400
}

// getTitleColor returns the title color for the theme and falls back when the
// primary color is too close to the background.
func getTitleColor(theme *SlideTheme) string {
	if isDarkTheme(theme) {
		return "FFFFFF"
	}
	// Check whether primaryColor is too close to the background.
	bgColor := theme.BgColor1
	if bgColor != "" && isSimilarHue(theme.PrimaryColor, bgColor) {
		// Resolve same-family conflicts with a safer fallback color.
		if isDarkColor(bgColor) {
			return "FFFFFF"
		}
		return "333333"
	}
	return theme.PrimaryColor
}

// getSlideTitleColor returns the title color based on the slide background or theme.
func getSlideTitleColor(slide Slide, theme *SlideTheme) string {
	effectiveBg := theme.BgColor1
	if slide.BgColor != "" {
		effectiveBg = slide.BgColor
	}

	if effectiveBg != "" && isDarkColor(effectiveBg) {
		return "FFFFFF"
	}

	titleColor := getTitleColor(theme)
	// Check whether the title color is too close to the background.
	if effectiveBg != "" && isSimilarHue(titleColor, effectiveBg) {
		if isDarkColor(effectiveBg) {
			return "FFFFFF"
		}
		return "333333"
	}
	return titleColor
}

// getEffectiveBgColor returns the main slide background color for text-contrast checks.
func getEffectiveBgColor(slide Slide, theme *SlideTheme) string {
	if slide.BgColor != "" {
		return slide.BgColor
	}
	return theme.BgColor1
}

// getSafeTextColorForBg ensures that text color has enough contrast against the background.
func getSafeTextColorForBg(textColor string, bgColor string) string {
	if bgColor == "" {
		return textColor
	}
	if isSimilarHue(textColor, bgColor) {
		if isDarkColor(bgColor) {
			return "FFFFFF"
		}
		return "333333"
	}
	return textColor
}

// generateBackgroundXML builds the slide background XML from the theme.
func generateBackgroundXML(theme *SlideTheme) string {
	return generateBackgroundXMLWithColors(theme.BackgroundType, theme.BgColor1, theme.BgColor2)
}

// generateSlideBackgroundXML builds background XML from either the slide override
// or the theme default background.
func generateSlideBackgroundXML(slide Slide, theme *SlideTheme) string {
	if slide.BgColor != "" {
		bgColor2 := slide.BgColor2
		if bgColor2 != "" {
			// The slide provides two colors, so use a gradient.
			return generateBackgroundXMLWithColors("gradient", slide.BgColor, bgColor2)
		}
		// The slide provides one color, so use a solid fill.
		return generateBackgroundXMLWithColors("solid", slide.BgColor, "")
	}
	return generateBackgroundXML(theme)
}

// generateBackgroundXMLWithColors builds background XML from background type and colors.
func generateBackgroundXMLWithColors(bgType, bgColor1, bgColor2 string) string {
	switch bgType {
	case "gradient":
		// Automatically converge unsafe gradients when luminance spread is too large.
		safeBg1, safeBg2 := sanitizeGradient(bgColor1, bgColor2)
		return fmt.Sprintf(`        <p:bg>
            <p:bgPr>
                <a:gradFill>
                    <a:gsLst>
                        <a:gs pos="0"><a:srgbClr val="%s"/></a:gs>
                        <a:gs pos="100000"><a:srgbClr val="%s"/></a:gs>
                    </a:gsLst>
                    <a:lin ang="5400000" scaled="1"/>
                </a:gradFill>
                <a:effectLst/>
            </p:bgPr>
        </p:bg>`, safeBg1, safeBg2)
	case "dark":
		return fmt.Sprintf(`        <p:bg>
            <p:bgPr>
                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                <a:effectLst/>
            </p:bgPr>
        </p:bg>`, bgColor1)
	default: // "solid"
		return fmt.Sprintf(`        <p:bg>
            <p:bgPr>
                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                <a:effectLst/>
            </p:bgPr>
        </p:bg>`, bgColor1)
	}
}

// createSlideXMLEnhanced builds richer slide XML based on the selected layout.
func (g *PPTXGenerator) createSlideXMLEnhanced(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, hasChart bool, chartIndex, slideNum, totalSlides int) string {
	layout := resolvedLayout(slide)

	switch layout {
	case "title":
		return g.createTitleSlideXML(slide, theme, stylePreset, slideNum, totalSlides)
	case "chart":
		if hasChart {
			return g.createChartSlideXML(slide, theme, stylePreset, chartIndex, slideNum, totalSlides)
		}
		return g.createChartAsShapesSlideXML(slide, theme, stylePreset, slideNum, totalSlides)
	case "dashboard":
		if hasChart {
			return g.createDashboardSlideXML(slide, theme, stylePreset, hasChart, chartIndex, slideNum, totalSlides)
		}
		return g.createDashboardAsShapesSlideXML(slide, theme, stylePreset, slideNum, totalSlides)
	default:
		return g.createContentSlideXML(slide, theme, stylePreset, slideNum, totalSlides)
	}
}

// generateSubtitleXML builds subtitle XML below the main title.
// shapeID is the shape id and y is the vertical offset.
func generateSubtitleXML(subtitle string, shapeID int, y int, theme *SlideTheme) string {
	if subtitle == "" {
		return ""
	}
	textColor := getTextColor(theme)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	return fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="Subtitle"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="700000" y="%d"/>
                        <a:ext cx="10800000" cy="400000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="2000">
                                <a:solidFill><a:srgbClr val="%s"><a:alpha val="80000"/></a:srgbClr></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, shapeID, y, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(subtitle))
}

// generateFooterXML builds footer XML for the bottom of the slide.
// It currently renders the data source only and starts shape ids from baseID.
func generateFooterXML(source string, slideNum, totalSlides, baseID int, theme *SlideTheme, stylePreset PPTXStylePreset) string {
	textColor := getTextColor(theme)
	lineColor := strings.TrimSpace(stylePreset.FooterLineColor)
	if lineColor == "" {
		lineColor = theme.AccentColor
	}
	result := ""

	result += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="FooterRule"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="500000" y="6200000"/>
                        <a:ext cx="11100000" cy="0"/>
                    </a:xfrm>
                    <a:prstGeom prst="line"><a:avLst/></a:prstGeom>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="34000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>`, baseID, lineColor)

	// Data-source footnote in the lower-left corner.
	if source != "" {
		result += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="Source"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="500000" y="6320000"/>
                        <a:ext cx="8500000" cy="260000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="900" i="1">
                                <a:solidFill><a:srgbClr val="%s"><a:alpha val="52000"/></a:srgbClr></a:solidFill>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, baseID+1, textColor, escapeXML(source))
	}

	pageLabel := fmt.Sprintf("%02d", slideNum)
	if totalSlides > 0 {
		pageLabel = fmt.Sprintf("%02d/%02d", slideNum, totalSlides)
	}
	result += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="PageNumber"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="10300000" y="6300000"/>
                        <a:ext cx="1200000" cy="260000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="r"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="900">
                                <a:solidFill><a:srgbClr val="%s"><a:alpha val="56000"/></a:srgbClr></a:solidFill>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, baseID+2, textColor, pageLabel)

	return result
}

func createImagePictureXML(shapeID int, name, relID string, x, y, cx, cy int, imageData []byte) string {
	srcRectXML := ""
	if crop := calculateImageCrop(imageData, cx, cy); crop != nil {
		srcRectXML = fmt.Sprintf(`
                    <a:srcRect l="%d" t="%d" r="%d" b="%d"/>`, crop.left, crop.top, crop.right, crop.bottom)
	}
	return fmt.Sprintf(`
            <p:pic>
                <p:nvPicPr>
                    <p:cNvPr id="%d" name="%s"/>
                    <p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr>
                    <p:nvPr/>
                </p:nvPicPr>
                <p:blipFill>
                    <a:blip r:embed="%s"/>%s
                    <a:stretch><a:fillRect/></a:stretch>
                </p:blipFill>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
            </p:pic>`, shapeID, escapeXML(name), relID, srcRectXML, x, y, cx, cy)
}

type imageCrop struct {
	left   int
	top    int
	right  int
	bottom int
}

func calculateImageCrop(imageData []byte, frameCX, frameCY int) *imageCrop {
	if len(imageData) == 0 || frameCX <= 0 || frameCY <= 0 {
		return nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(imageData))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return nil
	}

	imageRatio := float64(cfg.Width) / float64(cfg.Height)
	frameRatio := float64(frameCX) / float64(frameCY)
	if imageRatio <= 0 || frameRatio <= 0 {
		return nil
	}
	if math.Abs(imageRatio-frameRatio) < 0.01 {
		return nil
	}

	crop := &imageCrop{}
	if imageRatio > frameRatio {
		visible := frameRatio / imageRatio
		trim := int(math.Round((1 - visible) * 50000))
		crop.left = trim
		crop.right = trim
		return crop
	}

	visible := imageRatio / frameRatio
	trim := int(math.Round((1 - visible) * 50000))
	crop.top = trim
	crop.bottom = trim
	return crop
}

func createSolidOverlayXML(shapeID int, name, color string, alpha int, x, y, cx, cy int) string {
	return fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="%s"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
            </p:sp>`, shapeID, escapeXML(name), x, y, cx, cy, color, alpha)
}

func createFramedPanelXML(shapeID int, name, fillColor string, fillAlpha int, lineColor string, lineAlpha int, x, y, cx, cy int) string {
	return fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="%s"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst><a:gd name="adj" fmla="val 2800"/></a:avLst></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>`, shapeID, escapeXML(name), x, y, cx, cy, fillColor, fillAlpha, lineColor, lineAlpha)
}

// createTitleSlideXML builds XML for the title slide.
func (g *PPTXGenerator) createTitleSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	subtitleColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	imagePos := resolvedImagePos(slide)

	titleX, titleY, titleCX, titleCY := 1500000, 2200000, 9200000, 1200000
	subtitleX, subtitleY, subtitleCX, subtitleCY := 1500000, 3800000, 9200000, 800000
	decorX, decorY, decorCX := 4596000, 3600000, 3000000
	titleAlign := "ctr"
	if slide.Variant == "title-split" || stylePreset.ID == StylePresetExecutiveDark {
		titleX, titleY, titleCX, titleCY = 1000000, 1850000, 5000000, 1450000
		subtitleX, subtitleY, subtitleCX, subtitleCY = 1000000, 3550000, 5000000, 850000
		decorX, decorY, decorCX = 1000000, 1500000, 2600000
		titleAlign = stylePreset.TitleAlign
	}
	imageXML := ""
	overlayXML := ""
	titlePanelXML := ""
	panelAlpha := 72000
	panelLineAlpha := 12000
	if isDarkTheme(theme) {
		panelAlpha = 18000
		panelLineAlpha = 18000
	}
	if imagePos == "background" {
		titleColor = "FFFFFF"
		subtitleColor = "FFFFFF"
		imageXML = createImagePictureXML(90, "BackgroundImage", "rId2", 0, 0, 12192000, 6858000, slide.ImageData)
		overlayXML = createSolidOverlayXML(91, "ImageOverlay", "000000", 35000, 0, 0, 12192000, 6858000)
	} else if imagePos == "center" {
		titleY = 750000
		subtitleY = 5450000
		decorY = 4700000
		imageXML = createImagePictureXML(90, "CenterImage", "rId2", 2460000, 1550000, 7272000, 3200000, slide.ImageData)
	} else {
		titlePanelXML = createFramedPanelXML(92, "TitlePanel", stylePreset.BackgroundOverlay, panelAlpha, theme.AccentColor, panelLineAlpha, titleX-250000, titleY-450000, titleCX+500000, 2500000)
	}

	subtitle := slide.Subtitle
	if subtitle == "" {
		subtitle = slide.Content
	}

	subtitleXML := ""
	if subtitle != "" {
		subtitleXML = fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="3" name="Subtitle"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="%s"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="2000">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, subtitleX, subtitleY, subtitleCX, subtitleCY, titleAlign, subtitleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(subtitle))
	}

	// Decorative line.
	accentColor := theme.AccentColor
	decorLineXML := fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="4" name="DecorLine"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="0"/>
                    </a:xfrm>
                    <a:prstGeom prst="%s"><a:avLst/></a:prstGeom>
                    <a:ln w="28575">
                        <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>`, decorX, decorY, decorCX, stylePreset.TitleAccentShape, accentColor)

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" 
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" 
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr>
                <p:cNvPr id="1" name=""/>
                <p:cNvGrpSpPr/>
                <p:nvPr/>
            </p:nvGrpSpPr>
            <p:grpSpPr>
                <a:xfrm>
                    <a:off x="0" y="0"/>
                    <a:ext cx="0" cy="0"/>
                    <a:chOff x="0" y="0"/>
                    <a:chExt cx="0" cy="0"/>
                </a:xfrm>
            </p:grpSpPr>%s%s%s
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="2" name="Title"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="%s"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="4400" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, imageXML, overlayXML, titlePanelXML, titleX, titleY, titleCX, titleCY, titleAlign, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title), decorLineXML, subtitleXML, generateFooterXML(slide.Source, slideNum, totalSlides, 10, theme, stylePreset))
}

type contentPattern string

const (
	contentPatternEmpty        contentPattern = "empty"
	contentPatternPointCards   contentPattern = "point-cards"
	contentPatternSectionCards contentPattern = "section-cards"
	contentPatternFeatureGrid  contentPattern = "feature-grid"
	contentPatternPillarList   contentPattern = "pillar-list"
	contentPatternBulletList   contentPattern = "bullet-list"
	contentPatternProse        contentPattern = "prose"
	contentPatternComparison   contentPattern = "comparison"
	contentPatternTimeline     contentPattern = "timeline"
)

func resolveContentPattern(slide Slide, imagePos string) contentPattern {
	switch strings.ToLower(strings.TrimSpace(slide.Variant)) {
	case "feature-grid":
		if canRenderFeatureGridPattern(slide) {
			return contentPatternFeatureGrid
		}
	case "pillar-list":
		if canRenderPillarListPattern(slide) {
			return contentPatternPillarList
		}
	case "sections-grid":
		if len(slide.Sections) > 0 {
			return contentPatternSectionCards
		}
	case "point-cards":
		if len(slide.Points) > 0 {
			return contentPatternPointCards
		}
	case "bullets":
		if len(slide.Points) > 0 {
			return contentPatternBulletList
		}
	case "prose":
		if strings.TrimSpace(slide.Content) != "" {
			return contentPatternProse
		}
	}
	if canRenderTimelinePattern(slide) {
		return contentPatternTimeline
	}
	if canRenderComparisonPattern(slide) {
		return contentPatternComparison
	}
	if canRenderFeatureGridPattern(slide) {
		return contentPatternFeatureGrid
	}
	if canRenderPillarListPattern(slide) {
		return contentPatternPillarList
	}
	if len(slide.Sections) > 0 {
		return contentPatternSectionCards
	}
	if len(slide.Points) > 0 {
		if imagePos != "" && imagePos != "bottom" && imagePos != "top" {
			return contentPatternBulletList
		}
		return contentPatternPointCards
	}
	if strings.TrimSpace(slide.Content) != "" {
		return contentPatternProse
	}
	return contentPatternEmpty
}

func canRenderComparisonPattern(slide Slide) bool {
	variant := strings.ToLower(strings.TrimSpace(slide.Variant))
	if variant == "comparison" {
		return len(slide.Sections) >= 2 || len(slide.Points) >= 2
	}
	text := strings.ToLower(strings.TrimSpace(slide.Role + " " + slide.Title + " " + slide.Subtitle))
	if len(slide.Sections) == 2 || len(slide.Points) == 2 {
		for _, keyword := range []string{"compare", "comparison", "before", "after", "with", "without", "versus", "vs", "对比", "比较", "前后", "之前", "之后", "有无"} {
			if strings.Contains(text, keyword) {
				return true
			}
		}
	}
	return false
}

func canRenderTimelinePattern(slide Slide) bool {
	if len(slide.Sections) < 3 || len(slide.Sections) > 4 {
		return false
	}
	variant := strings.ToLower(strings.TrimSpace(slide.Variant))
	if variant == "timeline" {
		return true
	}
	role := strings.ToLower(strings.TrimSpace(slide.Role))
	if role == "action" {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(slide.Title + " " + slide.Subtitle))
	for _, keyword := range []string{"timeline", "roadmap", "cadence", "plan", "rollout", "phase", "milestone", "step", "时间线", "路线图", "节奏", "计划", "推进", "阶段", "里程碑", "步骤"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func canRenderFeatureGridPattern(slide Slide) bool {
	return len(slide.Sections) == 4
}

func canRenderPillarListPattern(slide Slide) bool {
	if len(slide.Sections) < 3 || len(slide.Sections) > 4 {
		return false
	}
	return !canRenderTimelinePattern(slide)
}

func splitPointCard(point string) (string, string) {
	point = strings.TrimSpace(point)
	for _, sep := range []string{"：", ":"} {
		parts := strings.SplitN(point, sep, 2)
		if len(parts) != 2 {
			continue
		}
		label := strings.TrimSpace(parts[0])
		body := strings.TrimSpace(parts[1])
		if label != "" && body != "" && len([]rune(label)) <= 10 {
			return label, body
		}
	}
	return "", point
}

func comparisonSectionsForSlide(slide Slide) []SlideSection {
	if len(slide.Sections) >= 2 {
		return []SlideSection{slide.Sections[0], slide.Sections[1]}
	}
	if len(slide.Points) >= 2 {
		sections := make([]SlideSection, 0, 2)
		for idx, point := range slide.Points[:2] {
			heading, body := splitPointCard(point)
			if heading == "" {
				heading = fmt.Sprintf("Option %d", idx+1)
				body = point
			}
			sections = append(sections, SlideSection{
				Heading: heading,
				Detail:  body,
			})
		}
		return sections
	}
	return nil
}

func createFeatureGridXML(sections []SlideSection, x, y, cx, cy int, accentColor, textColor, fontFamily, eaFontFamily, cardFill string, cardAlpha int) string {
	if len(sections) < 4 {
		return ""
	}

	sections = sections[:4]
	gapX := 260000
	gapY := 260000
	cardW := (cx - gapX) / 2
	cardH := (cy - gapY) / 2
	if cardH < 1600000 {
		cardH = 1600000
	}
	circleColors := []string{
		accentColor,
		blendColor(accentColor, textColor, 0.2),
		blendColor(accentColor, cardFill, 0.55),
		blendColor(accentColor, cardFill, 0.25),
	}

	var sb strings.Builder
	for idx, section := range sections {
		row := idx / 2
		col := idx % 2
		cardX := x + col*(cardW+gapX)
		cardY := y + row*(cardH+gapY)
		shapeID := 200 + idx*5
		circleFill := circleColors[idx%len(circleColors)]
		circleTextColor := getSafeTextColorForBg("FFFFFF", circleFill)

		sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="FeatureGridCard%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="18000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="FeatureGridBadge%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="420000" cy="420000"/>
                    </a:xfrm>
                    <a:prstGeom prst="ellipse"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%02d</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="FeatureGridTitle%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="420000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1800" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="FeatureGridBody%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t" lIns="140000" tIns="0" rIns="140000" bIns="0"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`,
			shapeID, idx+1, cardX, cardY, cardW, cardH, cardFill, cardAlpha, accentColor,
			shapeID+1, idx+1, cardX+180000, cardY+180000, circleFill, circleTextColor, escapeXML(fontFamily), escapeXML(eaFontFamily), idx+1,
			shapeID+2, idx+1, cardX+700000, cardY+180000, cardW-900000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Heading),
			shapeID+3, idx+1, cardX+180000, cardY+760000, cardW-360000, cardH-900000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Detail)))
	}

	return sb.String()
}

func createPillarListXML(sections []SlideSection, x, y, cx, cy int, accentColor, textColor, fontFamily, eaFontFamily, cardFill string, cardAlpha int) string {
	if len(sections) < 3 {
		return ""
	}

	count := len(sections)
	rowGap := 200000
	rowH := (cy - rowGap*(count-1)) / count
	if rowH < 1000000 {
		rowH = 1000000
	}
	lineX := x + 260000
	nodeSize := 260000

	var sb strings.Builder
	for idx, section := range sections {
		rowY := y + idx*(rowH+rowGap)
		nodeY := rowY + 180000
		cardX := x + 520000
		cardW := cx - 520000
		cardH := rowH
		shapeID := 260 + idx*5
		if idx < count-1 {
			nextNodeY := rowY + rowH + rowGap + 180000
			sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="PillarConnector%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="0" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="line"><a:avLst/></a:prstGeom>
                    <a:ln w="19050">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="42000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>`, shapeID, idx+1, lineX+nodeSize/2, nodeY+nodeSize, nextNodeY-nodeY-nodeSize, accentColor))
		}

		sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="PillarNode%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="ellipse"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600" b="1">
                                <a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%02d</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="PillarCard%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="16000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="PillarTitle%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="360000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1800" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="PillarBody%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t" lIns="0" tIns="0" rIns="0" bIns="0"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`,
			shapeID+1, idx+1, lineX, nodeY, nodeSize, nodeSize, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), idx+1,
			shapeID+2, idx+1, cardX, rowY, cardW, cardH, cardFill, cardAlpha, accentColor,
			shapeID+3, idx+1, cardX+180000, rowY+180000, cardW-360000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Heading),
			shapeID+4, idx+1, cardX+180000, rowY+600000, cardW-360000, cardH-700000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Detail)))
	}

	return sb.String()
}

func createBulletListXML(points []string, x, y, cx, cy int, textColor, fontFamily, eaFontFamily string) string {
	if len(points) == 0 {
		return ""
	}

	pointCount := len(points)
	pointFontSize := 2000
	pointSpcBefore := 600
	anchor := "ctr"
	if pointCount <= 3 {
		pointFontSize = 2200
		pointSpcBefore = 1200
	} else if pointCount <= 4 {
		pointSpcBefore = 800
	} else {
		pointFontSize = 1800
		anchor = "t"
	}

	var paragraphs strings.Builder
	for idx, point := range points {
		spcBefore := pointSpcBefore
		if idx == 0 && anchor == "t" {
			spcBefore = 0
		}
		paragraphs.WriteString(fmt.Sprintf(`
                    <a:p>
                        <a:pPr marL="342900" indent="-342900" algn="l">
                            <a:spcBef><a:spcPts val="%d"/></a:spcBef>
                            <a:buFont typeface="%s"/>
                            <a:buChar char="●"/>
                        </a:pPr>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="%d">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>`, spcBefore, escapeXML(fontFamily), pointFontSize, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(point)))
	}

	return fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="30" name="ContentBulletList"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="%s" lIns="180000" tIns="180000" rIns="120000" bIns="120000"/>
                    <a:lstStyle/>%s
                </p:txBody>
            </p:sp>`, x, y, cx, cy, anchor, paragraphs.String())
}

func createProseBodyXML(content string, x, y, cx, cy int, textColor, fontFamily, eaFontFamily string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	contentFontSize := 2000
	contentLen := len([]rune(content))
	if contentLen > 500 {
		contentFontSize = 1600
	} else if contentLen > 200 {
		contentFontSize = 1800
	}
	if contentFontSize < 1400 {
		contentFontSize = 1400
	}

	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	paragraphs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paragraphs = append(paragraphs, line)
	}
	if len(paragraphs) == 0 {
		paragraphs = []string{content}
	}

	anchor := "ctr"
	if len(paragraphs) > 1 || contentLen > 260 {
		anchor = "t"
	}

	var body strings.Builder
	for idx, paragraph := range paragraphs {
		spcBefore := 0
		if idx > 0 {
			spcBefore = 900
		}
		body.WriteString(fmt.Sprintf(`
                    <a:p>
                        <a:pPr algn="l">
                            <a:spcBef><a:spcPts val="%d"/></a:spcBef>
                        </a:pPr>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="%d">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>`, spcBefore, contentFontSize, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(paragraph)))
	}

	return fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="31" name="ContentProse"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="%s" lIns="180000" tIns="180000" rIns="120000" bIns="120000"/>
                    <a:lstStyle/>%s
                </p:txBody>
            </p:sp>`, x, y, cx, cy, anchor, body.String())
}

func createComparisonCardsXML(slide Slide, x, y, cx, cy int, accentColor, textColor, fontFamily, eaFontFamily, cardFill string, cardAlpha int) string {
	sections := comparisonSectionsForSlide(slide)
	if len(sections) < 2 {
		return ""
	}

	gapX := 260000
	cardW := (cx - gapX) / 2
	cardY := y + 180000
	cardH := cy - 260000
	if cardH < 2200000 {
		cardH = 2200000
	}

	leftBadgeColor := accentColor
	rightBadgeColor := blendColor(accentColor, cardFill, 0.35)
	leftBadgeTextColor := getSafeTextColorForBg("FFFFFF", leftBadgeColor)
	rightBadgeTextColor := getSafeTextColorForBg("FFFFFF", rightBadgeColor)
	dividerFill := blendColor(accentColor, cardFill, 0.65)
	dividerTextColor := getSafeTextColorForBg(textColor, dividerFill)

	var sb strings.Builder
	for idx, section := range sections[:2] {
		cardX := x + idx*(cardW+gapX)
		cardID := 60 + idx*4
		badgeID := cardID + 1
		headingID := cardID + 2
		bodyID := cardID + 3
		badgeColor := leftBadgeColor
		badgeTextColor := leftBadgeTextColor
		if idx == 1 {
			badgeColor = rightBadgeColor
			badgeTextColor = rightBadgeTextColor
		}

		sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="ComparisonCard%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="18000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="ComparisonBadge%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="360000"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="ComparisonHeading%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="600000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="2200" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="ComparisonBody%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t" lIns="80000" tIns="20000" rIns="40000" bIns="0"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1800">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`,
			cardID, idx+1, cardX, cardY, cardW, cardH, cardFill, cardAlpha, accentColor,
			badgeID, idx+1, cardX+220000, cardY+180000, 1500000, badgeColor,
			headingID, idx+1, cardX+220000, cardY+700000, cardW-440000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Heading),
			bodyID, idx+1, cardX+220000, cardY+1450000, cardW-440000, cardH-1650000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Detail)))

		sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="ComparisonBadgeText%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="360000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, bodyID+10, idx+1, cardX+220000, cardY+180000, 1500000, badgeTextColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Heading)))
	}

	centerX := x + cardW + gapX/2 - 280000
	centerY := y + cy/2 - 280000
	sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="78" name="ComparisonDivider"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="560000" cy="560000"/>
                    </a:xfrm>
                    <a:prstGeom prst="ellipse"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="72000"/></a:srgbClr></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>VS</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, centerX, centerY, dividerFill, dividerTextColor, escapeXML(fontFamily), escapeXML(eaFontFamily)))

	return sb.String()
}

func createTimelineCardsXML(sections []SlideSection, x, y, cx, cy int, accentColor, textColor, fontFamily, eaFontFamily, cardFill string, cardAlpha int) string {
	if len(sections) < 3 {
		return ""
	}

	count := len(sections)
	gapX := 180000
	cardW := (cx - gapX*(count-1)) / count
	if cardW < 2100000 {
		cardW = 2100000
	}
	totalWidth := cardW*count + gapX*(count-1)
	startX := x + (cx-totalWidth)/2
	if totalWidth > cx {
		startX = x
		cardW = (cx - gapX*(count-1)) / count
	}

	railY := y + 420000
	nodeSize := 260000
	cardY := y + 780000
	cardH := cy - 980000
	if cardH < 1800000 {
		cardH = 1800000
	}
	nodeTextColor := getSafeTextColorForBg("FFFFFF", accentColor)

	var sb strings.Builder
	for idx, section := range sections {
		cardX := startX + idx*(cardW+gapX)
		centerX := cardX + cardW/2
		nodeX := centerX - nodeSize/2
		connectorID := 80 + idx*5
		nodeID := connectorID + 1
		nodeLabelID := connectorID + 2
		cardID := connectorID + 3
		bodyID := connectorID + 4

		if idx < count-1 {
			nextCenterX := centerX + cardW + gapX
			sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="TimelineConnector%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="0"/>
                    </a:xfrm>
                    <a:prstGeom prst="line"><a:avLst/></a:prstGeom>
                    <a:ln w="19050">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="48000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>`, connectorID, idx+1, centerX+nodeSize/2, railY+nodeSize/2, nextCenterX-centerX-nodeSize, accentColor))
		}

		sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="TimelineNode%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="ellipse"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="TimelineNodeLabel%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%02d</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="TimelineCard%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="18000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t" lIns="160000" tIns="180000" rIns="160000" bIns="120000"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1800" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="TimelineBody%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t" lIns="120000" tIns="20000" rIns="120000" bIns="0"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`,
			nodeID, idx+1, nodeX, railY, nodeSize, nodeSize, accentColor,
			nodeLabelID, idx+1, nodeX, railY, nodeSize, nodeSize, nodeTextColor, escapeXML(fontFamily), escapeXML(eaFontFamily), idx+1,
			cardID, idx+1, cardX, cardY, cardW, cardH, cardFill, cardAlpha, accentColor, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Heading),
			bodyID, idx+1, cardX+80000, cardY+620000, cardW-160000, cardH-700000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Detail)))
	}

	return sb.String()
}

func createPointCardsXML(points []string, x, y, cx, cy int, accentColor, textColor, fontFamily, eaFontFamily, cardFill string, cardAlpha int) string {
	if len(points) == 0 {
		return ""
	}
	gap := 180000
	count := len(points)
	cardHeight := (cy - gap*(count-1)) / count
	if cardHeight < 780000 {
		cardHeight = 780000
	}

	var sb strings.Builder
	for idx, point := range points {
		cardY := y + idx*(cardHeight+gap)
		cardID := 20 + idx*2
		accentID := cardID + 1
		label, body := splitPointCard(point)
		displayText := body
		if label == "" {
			label = fmt.Sprintf("%02d", idx+1)
		}
		if body == "" {
			body = point
			displayText = body
		}
		if body != point && label != "" {
			displayText = label + "：" + body
		}
		sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="PointCard%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="16000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr lIns="260000" tIns="240000" rIns="240000" bIns="180000" anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="2100">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="PointAccent%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="80000" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
            </p:sp>`, cardID, idx+1, x, cardY, cx, cardHeight, cardFill, cardAlpha, accentColor, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(displayText), accentID, idx+1, x, cardY, cardHeight, accentColor))
	}
	return sb.String()
}

func createSectionCardsXML(sections []SlideSection, x, y, cx, cy int, accentColor, textColor, fontFamily, eaFontFamily, cardFill string, cardAlpha int) string {
	if len(sections) == 0 {
		return ""
	}
	cols := 1
	switch {
	case len(sections) >= 3 && cx >= 9000000:
		cols = 3
	case len(sections) >= 2 && cx >= 6800000:
		cols = 2
	}
	rows := int(math.Ceil(float64(len(sections)) / float64(cols)))
	gapX, gapY := 220000, 220000
	cardW := (cx - gapX*(cols-1)) / cols
	cardH := (cy - gapY*(rows-1)) / rows
	if cardH < 1250000 {
		cardH = 1250000
	}
	startY := y
	if rows == 1 {
		targetHeight := cardH
		if targetHeight > 2800000 {
			targetHeight = 2800000
		}
		if targetHeight < 2200000 {
			targetHeight = 2200000
		}
		startY = y + (cy-targetHeight)/2
		cardH = targetHeight
	}

	var sb strings.Builder
	for idx, section := range sections {
		row := idx / cols
		col := idx % cols
		cardX := x + col*(cardW+gapX)
		cardY := startY + row*(cardH+gapY)
		cardID := 40 + idx*3
		headerID := cardID + 1
		textID := cardID + 2
		sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="SectionCard%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="18000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="SectionHeader%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="360000"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600" b="1">
                                <a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="SectionBody%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr lIns="220000" tIns="250000" rIns="220000" bIns="180000" anchor="t"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1800">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, cardID, idx+1, cardX, cardY, cardW, cardH, cardFill, cardAlpha, accentColor, headerID, idx+1, cardX+220000, cardY-120000, 1500000, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Heading), textID, idx+1, cardX, cardY+220000, cardW, cardH-260000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(section.Detail)))
	}
	return sb.String()
}

// createContentSlideXML builds XML for a content slide, including bullet support.
func (g *PPTXGenerator) createContentSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	accentColor := getSafeTextColorForBg(theme.AccentColor, bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	imagePos := resolvedImagePos(slide)

	contentX, contentY, contentCX, contentCY := 700000, 1500000, 10800000, 4800000
	titleY, titleCY := 300000, 700000
	subtitleY := 1000000
	imageXML := ""
	contentPanelXML := ""
	if slide.Variant == "image-right" {
		imagePos = "right"
	}
	if imagePos == "right" {
		contentX, contentY, contentCX, contentCY = 700000, 1500000, 5400000, 4800000
		imageXML = createImagePictureXML(90, "RightImage", "rId2", 6400000, 1450000, 4800000, 4300000, slide.ImageData)
	} else if imagePos == "left" {
		contentX, contentY, contentCX, contentCY = 6100000, 1500000, 5000000, 4800000
		imageXML = createImagePictureXML(90, "LeftImage", "rId2", 700000, 1450000, 4800000, 4300000, slide.ImageData)
	} else if imagePos == "center" {
		contentX, contentY, contentCX, contentCY = 1700000, 5250000, 8800000, 900000
		titleY, titleCY = 300000, 600000
		subtitleY = 950000
		imageXML = createImagePictureXML(90, "CenterImage", "rId2", 2460000, 1500000, 7272000, 3000000, slide.ImageData)
	} else if imagePos == "top" {
		contentX, contentY, contentCX, contentCY = 900000, 3600000, 10300000, 2600000
		titleY, titleCY = 250000, 600000
		subtitleY = 980000
		imageXML = createImagePictureXML(90, "TopImage", "rId2", 1460000, 1200000, 9272000, 2000000, slide.ImageData)
	} else if imagePos == "bottom" {
		contentX, contentY, contentCX, contentCY = 900000, 1500000, 10300000, 2500000
		titleY, titleCY = 250000, 600000
		subtitleY = 980000
		imageXML = createImagePictureXML(90, "BottomImage", "rId2", 1460000, 4300000, 9272000, 1800000, slide.ImageData)
	} else if imagePos == "diagonal" {
		contentX, contentY, contentCX, contentCY = 900000, 1650000, 6000000, 4200000
		titleY, titleCY = 250000, 600000
		subtitleY = 980000
		imageXML = createImagePictureXML(90, "DiagonalImage", "rId2", 6900000, 1200000, 4000000, 2600000, slide.ImageData)
	}

	// Accent block on the left side of the title.
	titleAccentColor := strings.TrimSpace(stylePreset.SectionBadgeFill)
	if titleAccentColor == "" {
		titleAccentColor = accentColor
	}
	titleDecoXML := fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="4" name="TitleDeco"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="500000" y="350000"/>
                        <a:ext cx="80000" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
            </p:sp>`, titleCY-200000, titleAccentColor)

	pattern := resolveContentPattern(slide, imagePos)
	if pattern != contentPatternEmpty &&
		pattern != contentPatternSectionCards &&
		pattern != contentPatternFeatureGrid &&
		pattern != contentPatternPillarList &&
		pattern != contentPatternComparison &&
		pattern != contentPatternTimeline &&
		(len(slide.Sections) == 0 && (imagePos != "" || slide.Content != "" || slide.Variant == "bullets")) {
		contentPanelXML = createFramedPanelXML(6, "ContentPanel", stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, contentX, contentY-100000, contentCX, contentCY+100000)
	}

	// Build the main content area.
	contentXML := ""
	switch pattern {
	case contentPatternSectionCards:
		contentXML = createSectionCardsXML(slide.Sections, contentX, contentY, contentCX, contentCY, accentColor, textColor, fontFamily, eaFontFamily, stylePreset.ContentCardFill, stylePreset.ContentCardAlpha)
	case contentPatternFeatureGrid:
		contentXML = createFeatureGridXML(slide.Sections, contentX, contentY, contentCX, contentCY, accentColor, textColor, fontFamily, eaFontFamily, stylePreset.ContentCardFill, stylePreset.ContentCardAlpha)
	case contentPatternPillarList:
		contentXML = createPillarListXML(slide.Sections, contentX, contentY, contentCX, contentCY, accentColor, textColor, fontFamily, eaFontFamily, stylePreset.ContentCardFill, stylePreset.ContentCardAlpha)
	case contentPatternPointCards:
		contentXML = createPointCardsXML(slide.Points, contentX, contentY, contentCX, contentCY, accentColor, textColor, fontFamily, eaFontFamily, stylePreset.ContentCardFill, stylePreset.ContentCardAlpha)
	case contentPatternBulletList:
		contentXML = createBulletListXML(slide.Points, contentX, contentY, contentCX, contentCY, textColor, fontFamily, eaFontFamily)
	case contentPatternProse:
		contentXML = createProseBodyXML(slide.Content, contentX, contentY, contentCX, contentCY, textColor, fontFamily, eaFontFamily)
	case contentPatternComparison:
		contentXML = createComparisonCardsXML(slide, contentX, contentY, contentCX, contentCY, accentColor, textColor, fontFamily, eaFontFamily, stylePreset.ContentCardFill, stylePreset.ContentCardAlpha)
	case contentPatternTimeline:
		contentXML = createTimelineCardsXML(slide.Sections, contentX, contentY, contentCX, contentCY, accentColor, textColor, fontFamily, eaFontFamily, stylePreset.ContentCardFill, stylePreset.ContentCardAlpha)
	}

	// Subtitle.
	subtitleXML := generateSubtitleXML(slide.Subtitle, 5, subtitleY, theme)

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" 
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" 
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr>
                <p:cNvPr id="1" name=""/>
                <p:cNvGrpSpPr/>
                <p:nvPr/>
            </p:nvGrpSpPr>
            <p:grpSpPr>
                <a:xfrm>
                    <a:off x="0" y="0"/>
                    <a:ext cx="0" cy="0"/>
                    <a:chOff x="0" y="0"/>
                    <a:chExt cx="0" cy="0"/>
                </a:xfrm>
            </p:grpSpPr>%s
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="2" name="Title"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="700000" y="%d"/>
                        <a:ext cx="10800000" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="3200" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>%s%s%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, titleDecoXML, titleY, titleCY, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title), subtitleXML, contentPanelXML, contentXML, imageXML, generateFooterXML(slide.Source, slideNum, totalSlides, 10, theme, stylePreset))
}

// createChartSlideXML builds XML for a slide that contains an embedded chart.
// createChartAsShapesSlideXML renders chart data as DrawingML shapes (colored bars + labels)
// instead of embedded OOXML chart objects, for OfficeSDK compatibility.
func (g *PPTXGenerator) createChartAsShapesSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily

	subtitleXML := generateSubtitleXML(slide.Subtitle, 6, 1000000, theme)

	// Build chart visualization as shapes
	chartShapesXML := ""
	shapeID := 10
	if slide.Chart != nil && len(slide.Chart.Values) > 0 {
		chartShapesXML = g.renderChartAsShapes(slide.Chart, theme, bgColor, &shapeID)
	}
	chartPanelXML := createFramedPanelXML(5, "ChartPanel", stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, theme.AccentColor, 12000, 850000, 1350000, 10400000, 4200000)

	// Points below chart area
	pointsXML := ""
	if len(slide.Points) > 0 {
		pointsY := 5500000
		pointsXML = generatePointsXML(slide.Points, shapeID, pointsY, textColor, fontFamily, eaFontFamily, theme.PrimaryColor)
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr>
                <p:cNvPr id="1" name=""/>
                <p:cNvGrpSpPr/>
                <p:nvPr/>
            </p:nvGrpSpPr>
            <p:grpSpPr>
                <a:xfrm>
                    <a:off x="0" y="0"/>
                    <a:ext cx="0" cy="0"/>
                    <a:chOff x="0" y="0"/>
                    <a:chExt cx="0" cy="0"/>
                </a:xfrm>
            </p:grpSpPr>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="2" name="Title"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="700000" y="300000"/>
                        <a:ext cx="10800000" cy="700000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="3200" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>%s%s%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title),
		subtitleXML, chartPanelXML, chartShapesXML, pointsXML, generateFooterXML(slide.Source, slideNum, totalSlides, 100, theme, stylePreset))
}

// renderChartAsShapes renders chart data as colored rectangles with labels
func (g *PPTXGenerator) renderChartAsShapes(chart *ChartData, theme *SlideTheme, bgColor string, shapeID *int) string {
	if len(chart.Values) == 0 {
		return ""
	}

	textColor := getSafeTextColorForBg(getTextColor(theme), bgColor)
	fontFamily := escapeXML(theme.FontFamily)
	eaFontFamily := escapeXML(theme.EAFontFamily)
	colors := []string{
		theme.PrimaryColor,
		theme.AccentColor,
		blendColor(theme.PrimaryColor, theme.AccentColor, 0.35),
		blendColor(theme.PrimaryColor, bgColor, 0.2),
		blendColor(theme.AccentColor, bgColor, 0.2),
	}

	result := ""

	// Chart title
	if chart.Title != "" {
		*shapeID++
		result += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="ChartTitle"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="1000000" y="1300000"/>
                        <a:ext cx="10200000" cy="400000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1800" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, *shapeID, textColor, fontFamily, eaFontFamily, escapeXML(chart.Title))
	}

	maxVal := 0.0
	for _, v := range chart.Values {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	n := len(chart.Values)
	chartLeft := 1200000   // left margin
	chartWidth := 9800000  // total width for bars
	chartTop := 1800000    // top of chart area
	chartHeight := 3200000 // max bar height
	barGap := 200000       // gap between bars

	barWidth := (chartWidth - barGap*(n+1)) / n
	if barWidth < 400000 {
		barWidth = 400000
	}

	for i := 0; i < n; i++ {
		color := colors[i%len(colors)]
		val := chart.Values[i]
		barH := int(float64(chartHeight) * val / maxVal)
		if barH < 100000 {
			barH = 100000
		}
		barX := chartLeft + barGap + i*(barWidth+barGap)
		barY := chartTop + chartHeight - barH

		// Bar rectangle
		*shapeID++
		result += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="Bar%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln w="0"><a:noFill/></a:ln>
                </p:spPr>
            </p:sp>`, *shapeID, i+1, barX, barY, barWidth, barH, color)

		// Value label above bar
		*shapeID++
		result += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="Val%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="300000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, *shapeID, i+1, barX, barY-300000, barWidth, textColor, fontFamily, eaFontFamily, formatValue(val))

		// Category label below bar
		label := ""
		if i < len(chart.Categories) {
			label = chart.Categories[i]
		}
		*shapeID++
		result += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="Cat%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="300000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, *shapeID, i+1, barX, chartTop+chartHeight+50000, barWidth, textColor, fontFamily, eaFontFamily, escapeXML(label))
	}

	return result
}

// formatValue formats a float for display (removes .0 for integers)
func formatValue(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}

// generatePointsXML renders bullet points as a text shape
func generatePointsXML(points []string, startID, y int, textColor, fontFamily, eaFontFamily, primaryColor string) string {
	if len(points) == 0 {
		return ""
	}
	paragraphs := ""
	for _, point := range points {
		paragraphs += fmt.Sprintf(`
                    <a:p>
                        <a:pPr marL="342900" indent="-342900" algn="l">
                            <a:spcBef><a:spcPts val="600"/></a:spcBef>
                            <a:buFont typeface="%s"/>
                            <a:buChar char="●"/>
                        </a:pPr>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>`, escapeXML(fontFamily), textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(point))
	}
	return fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="Points"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="700000" y="%d"/>
                        <a:ext cx="10800000" cy="1200000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t"/>
                    <a:lstStyle/>%s
                </p:txBody>
            </p:sp>`, startID, y, paragraphs)
}

func createInsightStackXML(points []string, x, y, cx, cy int, accentColor, textColor, fontFamily, eaFontFamily, cardFill string, cardAlpha, baseID int, prefix string) string {
	if len(points) == 0 {
		return ""
	}
	count := len(points)
	if count > 3 {
		count = 3
	}
	gapY := 180000
	cardH := (cy - gapY*(count-1)) / count
	if cardH < 960000 {
		cardH = 960000
	}

	var sb strings.Builder
	for idx, point := range points[:count] {
		cardY := y + idx*(cardH+gapY)
		heading, body := splitPointCard(point)
		if heading == "" {
			heading = fmt.Sprintf("Insight %d", idx+1)
			body = point
		}
		shapeID := baseID + idx*4
		sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="%sCard%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="18000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="%sTitle%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="320000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="%sBody%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t" lIns="0" tIns="0" rIns="0" bIns="0"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="%sAccent%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="90000" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
            </p:sp>`,
			shapeID, prefix, idx+1, x, cardY, cx, cardH, cardFill, cardAlpha, accentColor,
			shapeID+1, prefix, idx+1, x+180000, cardY+180000, cx-360000, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(heading),
			shapeID+2, prefix, idx+1, x+180000, cardY+540000, cx-360000, cardH-640000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(body),
			shapeID+3, prefix, idx+1, x, cardY, cardH, accentColor))
	}

	return sb.String()
}

func createMetricBandXML(metrics []MetricCard, x, y, cx, cy int, primaryColor, accentColor, textColor, fontFamily, eaFontFamily, cardFill string, cardAlpha int) string {
	if len(metrics) == 0 {
		return ""
	}
	count := len(metrics)
	if count > 4 {
		count = 4
	}
	gapX := 180000
	cardW := (cx - gapX*(count-1)) / count
	cardH := cy

	var sb strings.Builder
	for idx, metric := range metrics[:count] {
		cardX := x + idx*(cardW+gapX)
		shapeID := 500 + idx*4
		valueSize := 4200
		if len([]rune(metric.Value)) >= 6 {
			valueSize = 3400
		}
		sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="MetricBandCard%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="%d"/></a:srgbClr></a:solidFill>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="18000"/></a:srgbClr></a:solidFill>
                    </a:ln>
                </p:spPr>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="MetricBandValue%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="820000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="%d" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="MetricBandLabel%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="360000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`,
			shapeID, idx+1, cardX, y, cardW, cardH, cardFill, cardAlpha, accentColor,
			shapeID+1, idx+1, cardX, y+260000, cardW, valueSize, primaryColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(metric.Value),
			shapeID+2, idx+1, cardX, y+1120000, cardW, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(metric.Label)))

		if strings.TrimSpace(metric.Note) != "" {
			sb.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="MetricBandNote%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="360000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1600">
                                <a:solidFill><a:srgbClr val="%s"><a:alpha val="68000"/></a:srgbClr></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`,
				shapeID+3, idx+1, cardX, y+1480000, cardW, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(metric.Note)))
		}
	}

	return sb.String()
}

// createDashboardAsShapesSlideXML renders dashboard layout without embedded charts
func (g *PPTXGenerator) createDashboardAsShapesSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int) string {
	return g.createDashboardSlideXML(slide, theme, stylePreset, false, 0, slideNum, totalSlides)
}

func (g *PPTXGenerator) createChartSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, chartIndex, slideNum, totalSlides int) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	accentColor := getSafeTextColorForBg(theme.AccentColor, bgColor)

	subtitleXML := generateSubtitleXML(slide.Subtitle, 6, 1000000, theme)
	titleDecoXML := fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="4" name="TitleDeco"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="500000" y="250000"/>
                        <a:ext cx="80000" cy="500000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
            </p:sp>`, accentColor)

	chartPanelXML := createFramedPanelXML(5, "ChartPanel", stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, 850000, 1350000, 6600000, 4200000)
	chartX, chartCX := 1000000, 6200000
	if len(slide.Points) == 0 {
		chartPanelXML = createFramedPanelXML(5, "ChartPanel", stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, 850000, 1350000, 10400000, 4200000)
		chartX, chartCX = 1000000, 9900000
	}
	insightXML := createInsightStackXML(slide.Points, 7700000, 1500000, 3300000, 3900000, accentColor, textColor, fontFamily, eaFontFamily, stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, 40, "ChartInsight")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" 
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" 
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"
    xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr>
                <p:cNvPr id="1" name=""/>
                <p:cNvGrpSpPr/>
                <p:nvPr/>
            </p:nvGrpSpPr>
            <p:grpSpPr>
                <a:xfrm>
                    <a:off x="0" y="0"/>
                    <a:ext cx="0" cy="0"/>
                    <a:chOff x="0" y="0"/>
                    <a:chExt cx="0" cy="0"/>
                </a:xfrm>
            </p:grpSpPr>%s
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="2" name="Title"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="700000" y="300000"/>
                        <a:ext cx="10800000" cy="700000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="3200" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>%s%s
            <p:graphicFrame>
                <p:nvGraphicFramePr>
                    <p:cNvPr id="30" name="Chart %d"/>
                    <p:cNvGraphicFramePr>
                        <a:graphicFrameLocks noGrp="1"/>
                    </p:cNvGraphicFramePr>
                    <p:nvPr/>
                </p:nvGraphicFramePr>
                <p:xfrm>
                    <a:off x="%d" y="1600000"/>
                    <a:ext cx="%d" cy="3800000"/>
                </p:xfrm>
                <a:graphic>
                    <a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart">
                        <c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id="rId1"/>
                    </a:graphicData>
                </a:graphic>
            </p:graphicFrame>%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, titleDecoXML, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title), subtitleXML, chartPanelXML, chartIndex, chartX, chartCX, insightXML, generateFooterXML(slide.Source, slideNum, totalSlides, 10, theme, stylePreset))
}

// createDashboardSlideXML builds dashboard-layout XML with metric cards,
// an optional chart, and optional supporting points.
func (g *PPTXGenerator) createDashboardSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, hasChart bool, chartIndex, slideNum, totalSlides int) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	primaryColor := theme.PrimaryColor
	accentColor := getSafeTextColorForBg(theme.AccentColor, bgColor)
	metricFill := stylePreset.ContentCardFill
	if strings.TrimSpace(metricFill) == "" {
		metricFill = "FFFFFF"
	}
	metricFillAlpha := stylePreset.ContentCardAlpha
	if metricFillAlpha <= 0 {
		metricFillAlpha = 96000
	}

	titleDecoXML := fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="50" name="TitleDeco"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="500000" y="200000"/>
                        <a:ext cx="80000" cy="450000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                    <a:ln><a:noFill/></a:ln>
                </p:spPr>
            </p:sp>`, accentColor)

	metricsXML := createMetricBandXML(slide.Metrics, 700000, 1500000, 10800000, 2100000, primaryColor, accentColor, textColor, fontFamily, eaFontFamily, metricFill, metricFillAlpha)
	lowerContentXML := ""
	if hasChart {
		chartPanelXML := createFramedPanelXML(80, "ChartPanel", stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, 850000, 3900000, 6600000, 2400000)
		chartX, chartCX := 1000000, 6200000
		if len(slide.Points) == 0 {
			chartPanelXML = createFramedPanelXML(80, "ChartPanel", stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, 850000, 3900000, 10400000, 2400000)
			chartX, chartCX = 1000000, 9900000
		}
		lowerContentXML = fmt.Sprintf(`%s
            <p:graphicFrame>
                <p:nvGraphicFramePr>
                    <p:cNvPr id="81" name="Chart"/>
                    <p:cNvGraphicFramePr>
                        <a:graphicFrameLocks noGrp="1"/>
                    </p:cNvGraphicFramePr>
                    <p:nvPr/>
                </p:nvGraphicFramePr>
                <p:xfrm>
                    <a:off x="%d" y="4100000"/>
                    <a:ext cx="%d" cy="2000000"/>
                </p:xfrm>
                <a:graphic>
                    <a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart">
                        <c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id="rId1"/>
                    </a:graphicData>
                </a:graphic>
            </p:graphicFrame>`, chartPanelXML, chartX, chartCX)
		if len(slide.Points) > 0 {
			lowerContentXML += createInsightStackXML(slide.Points, 7700000, 3920000, 3300000, 2300000, accentColor, textColor, fontFamily, eaFontFamily, metricFill, metricFillAlpha, 90, "DashboardInsight")
		}
	} else if len(slide.Points) > 0 {
		lowerContentXML = createPointCardsXML(slide.Points, 700000, 4100000, 10800000, 1700000, accentColor, textColor, fontFamily, eaFontFamily, metricFill, metricFillAlpha)
	}

	footerXML := generateFooterXML(slide.Source, slideNum, totalSlides, 60, theme, stylePreset)
	dashSubtitleXML := generateSubtitleXML(slide.Subtitle, 55, 750000, theme)

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" 
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" 
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"
    xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr>
                <p:cNvPr id="1" name=""/>
                <p:cNvGrpSpPr/>
                <p:nvPr/>
            </p:nvGrpSpPr>
            <p:grpSpPr>
                <a:xfrm>
                    <a:off x="0" y="0"/>
                    <a:ext cx="0" cy="0"/>
                    <a:chOff x="0" y="0"/>
                    <a:chExt cx="0" cy="0"/>
                </a:xfrm>
            </p:grpSpPr>%s
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="2" name="Title"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="700000" y="150000"/>
                        <a:ext cx="10800000" cy="600000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="2800" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>%s%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, titleDecoXML, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title), dashSubtitleXML, metricsXML, lowerContentXML, footerXML)
}

// createSlideRelsWithChart builds slide rels with a chart reference.
func (g *PPTXGenerator) createSlideRelsWithChart(chartIndex int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="../charts/chart%d.xml"/></Relationships>`, chartIndex)
}

func (g *PPTXGenerator) createSlideRelsWithImage(imageIndex int, imageExt string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image%d.%s"/></Relationships>`, imageIndex, imageExt)
}

// createChartRelsXML builds rels for the chart itself.
func (g *PPTXGenerator) createChartRelsXML(chartIndex int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId3" Type="http://schemas.microsoft.com/office/2011/relationships/chartColorStyle" Target="colors%d.xml"/><Relationship Id="rId2" Type="http://schemas.microsoft.com/office/2011/relationships/chartStyle" Target="style%d.xml"/><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/oleObject" Target="" TargetMode="External"/></Relationships>`, chartIndex, chartIndex)
}

// createChartEmbeddedXlsx builds the minimal embedded XLSX data file for a chart.
// It contains the Sheet1 data that matches the chart XML caches.
func (g *PPTXGenerator) createChartEmbeddedXlsx(chart *ChartData) ([]byte, error) {
	// Build Sheet1 data: title row first, then category/value rows.
	rows := [][]string{{"Category", chart.Title}}
	for i, cat := range chart.Categories {
		val := ""
		if i < len(chart.Values) {
			val = fmt.Sprintf("%g", chart.Values[i])
		}
		rows = append(rows, []string{cat, val})
	}

	gen := NewXLSXGenerator()
	return gen.Generate([]XlsxSheet{{Name: "Sheet1", Rows: rows}}, XLSXOptions{
		Title:   chart.Title,
		Creator: "officecli",
	})
}

// createChartXML builds OOXML chart XML aligned with the chart-demo format.
func (g *PPTXGenerator) createChartXML(chart *ChartData) string {
	switch chart.Type {
	case "pie":
		return g.createPieChartXML(chart)
	case "line":
		return g.createLineChartXML(chart)
	default: // "bar" or default
		return g.createBarChartXML(chart)
	}
}

// chartCommonHeader is the shared chart XML header aligned with chart-demo.
const chartCommonHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><c:date1904 val="0"/><c:lang val="en-US"/><c:roundedCorners val="0"/><mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"><mc:Choice Requires="c14" xmlns:c14="http://schemas.microsoft.com/office/drawing/2007/8/2/chart"><c14:style val="102"/></mc:Choice><mc:Fallback><c:style val="2"/></mc:Fallback></mc:AlternateContent>`

// chartTitleXML is the chart title style aligned with chart-demo.
const chartTitleXML = `<c:title><c:overlay val="0"/><c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr><c:txPr><a:bodyPr rot="0" spcFirstLastPara="1" vertOverflow="ellipsis" vert="horz" wrap="square" anchor="ctr" anchorCtr="1"/><a:lstStyle/><a:p><a:pPr><a:defRPr sz="1862" b="0" i="0" u="none" strike="noStrike" kern="1200" spc="0" baseline="0"><a:solidFill><a:schemeClr val="tx1"><a:lumMod val="65000"/><a:lumOff val="35000"/></a:schemeClr></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:pPr><a:endParaRPr lang="en-CN"/></a:p></c:txPr></c:title><c:autoTitleDeleted val="0"/>`

// chartLegendXML is the chart legend style aligned with chart-demo.
const chartLegendXML = `<c:legend><c:legendPos val="b"/><c:overlay val="0"/><c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr><c:txPr><a:bodyPr rot="0" spcFirstLastPara="1" vertOverflow="ellipsis" vert="horz" wrap="square" anchor="ctr" anchorCtr="1"/><a:lstStyle/><a:p><a:pPr><a:defRPr sz="1197" b="0" i="0" u="none" strike="noStrike" kern="1200" baseline="0"><a:solidFill><a:schemeClr val="tx1"><a:lumMod val="65000"/><a:lumOff val="35000"/></a:schemeClr></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:pPr><a:endParaRPr lang="en-CN"/></a:p></c:txPr></c:legend>`

// chartFooterXML is the chart footer block aligned with chart-demo.
const chartFooterXML = `<c:plotVisOnly val="1"/><c:dispBlanksAs val="gap"/><c:showDLblsOverMax val="0"/></c:chart><c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr><c:txPr><a:bodyPr/><a:lstStyle/><a:p><a:pPr><a:defRPr/></a:pPr><a:endParaRPr lang="en-CN"/></a:p></c:txPr><c:externalData r:id="rId1"><c:autoUpdate val="0"/></c:externalData></c:chartSpace>`

// chartAxisLabelStyle is the axis-label text style aligned with chart-demo.
const chartAxisLabelStyle = `<c:txPr><a:bodyPr rot="-60000000" spcFirstLastPara="1" vertOverflow="ellipsis" vert="horz" wrap="square" anchor="ctr" anchorCtr="1"/><a:lstStyle/><a:p><a:pPr><a:defRPr sz="1197" b="0" i="0" u="none" strike="noStrike" kern="1200" baseline="0"><a:solidFill><a:schemeClr val="tx1"><a:lumMod val="65000"/><a:lumOff val="35000"/></a:schemeClr></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:pPr><a:endParaRPr lang="en-CN"/></a:p></c:txPr>`

// chartAxisLineStyle defines the axis-line style.
const chartAxisLineStyle = `<c:spPr><a:noFill/><a:ln w="9525" cap="flat" cmpd="sng" algn="ctr"><a:solidFill><a:schemeClr val="tx1"><a:lumMod val="15000"/><a:lumOff val="85000"/></a:schemeClr></a:solidFill><a:round/></a:ln><a:effectLst/></c:spPr>`

// chartGridlineStyle defines the major-gridline style.
const chartGridlineStyle = `<c:majorGridlines><c:spPr><a:ln w="9525" cap="flat" cmpd="sng" algn="ctr"><a:solidFill><a:schemeClr val="tx1"><a:lumMod val="15000"/><a:lumOff val="85000"/></a:schemeClr></a:solidFill><a:round/></a:ln><a:effectLst/></c:spPr></c:majorGridlines>`

// accentColors lists the scheme colors used for series and datapoint coloring.
var accentColors = []string{"accent1", "accent2", "accent3", "accent4", "accent5", "accent6"}

// createBarChartXML builds bar-chart XML aligned with chart-demo/chart3.xml.
func (g *PPTXGenerator) createBarChartXML(chart *ChartData) string {
	var sb strings.Builder
	sb.WriteString(chartCommonHeader)
	sb.WriteString("<c:chart>")
	sb.WriteString(chartTitleXML)
	sb.WriteString("<c:plotArea><c:layout/>")

	// barChart
	sb.WriteString(`<c:barChart><c:barDir val="col"/><c:grouping val="clustered"/><c:varyColors val="0"/>`)

	// Single series.
	accent := accentColors[0]
	sb.WriteString(`<c:ser><c:idx val="0"/><c:order val="0"/>`)
	sb.WriteString(fmt.Sprintf(`<c:tx><c:strRef><c:f>Sheet1!$B$1</c:f><c:strCache><c:ptCount val="1"/><c:pt idx="0"><c:v>%s</c:v></c:pt></c:strCache></c:strRef></c:tx>`, escapeXML(chart.Title)))
	sb.WriteString(fmt.Sprintf(`<c:spPr><a:solidFill><a:schemeClr val="%s"/></a:solidFill><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr>`, accent))
	sb.WriteString(`<c:invertIfNegative val="0"/>`)
	sb.WriteString(g.buildCategoryXML(chart.Categories))
	sb.WriteString(g.buildValueXML(chart.Values))
	sb.WriteString(`</c:ser>`)

	sb.WriteString(`<c:dLbls><c:showLegendKey val="0"/><c:showVal val="1"/><c:showCatName val="0"/><c:showSerName val="0"/><c:showPercent val="0"/><c:showBubbleSize val="0"/></c:dLbls>`)
	sb.WriteString(`<c:gapWidth val="219"/><c:overlap val="-27"/>`)
	sb.WriteString(`<c:axId val="111111111"/><c:axId val="222222222"/>`)
	sb.WriteString(`</c:barChart>`)

	// catAx
	sb.WriteString(`<c:catAx><c:axId val="111111111"/><c:scaling><c:orientation val="minMax"/></c:scaling><c:delete val="0"/><c:axPos val="b"/>`)
	sb.WriteString(`<c:numFmt formatCode="General" sourceLinked="1"/>`)
	sb.WriteString(`<c:majorTickMark val="none"/><c:minorTickMark val="none"/><c:tickLblPos val="nextTo"/>`)
	sb.WriteString(chartAxisLineStyle)
	sb.WriteString(chartAxisLabelStyle)
	sb.WriteString(`<c:crossAx val="222222222"/><c:crosses val="autoZero"/><c:auto val="1"/><c:lblAlgn val="ctr"/><c:lblOffset val="100"/><c:noMultiLvlLbl val="0"/>`)
	sb.WriteString(`</c:catAx>`)

	// valAx
	sb.WriteString(`<c:valAx><c:axId val="222222222"/><c:scaling><c:orientation val="minMax"/></c:scaling><c:delete val="0"/><c:axPos val="l"/>`)
	sb.WriteString(chartGridlineStyle)
	sb.WriteString(`<c:numFmt formatCode="General" sourceLinked="1"/>`)
	sb.WriteString(`<c:majorTickMark val="none"/><c:minorTickMark val="none"/><c:tickLblPos val="nextTo"/>`)
	sb.WriteString(`<c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr>`)
	sb.WriteString(chartAxisLabelStyle)
	sb.WriteString(`<c:crossAx val="111111111"/><c:crosses val="autoZero"/><c:crossBetween val="between"/>`)
	sb.WriteString(`</c:valAx>`)

	sb.WriteString(`<c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr>`)
	sb.WriteString(`</c:plotArea>`)
	sb.WriteString(chartLegendXML)
	sb.WriteString(chartFooterXML)

	return sb.String()
}

// createPieChartXML builds pie-chart XML aligned with chart-demo/chart1.xml.
func (g *PPTXGenerator) createPieChartXML(chart *ChartData) string {
	var sb strings.Builder
	sb.WriteString(chartCommonHeader)
	sb.WriteString("<c:chart>")
	sb.WriteString(chartTitleXML)
	sb.WriteString("<c:plotArea><c:layout/>")

	// pieChart
	sb.WriteString(`<c:pieChart><c:varyColors val="1"/>`)
	sb.WriteString(`<c:ser><c:idx val="0"/><c:order val="0"/>`)
	sb.WriteString(fmt.Sprintf(`<c:tx><c:strRef><c:f>Sheet1!$B$1</c:f><c:strCache><c:ptCount val="1"/><c:pt idx="0"><c:v>%s</c:v></c:pt></c:strCache></c:strRef></c:tx>`, escapeXML(chart.Title)))

	// Color each datapoint individually, aligned with chart-demo.
	numPts := len(chart.Values)
	for i := 0; i < numPts; i++ {
		accent := accentColors[i%len(accentColors)]
		sb.WriteString(fmt.Sprintf(`<c:dPt><c:idx val="%d"/><c:bubble3D val="0"/><c:spPr><a:solidFill><a:schemeClr val="%s"/></a:solidFill><a:ln w="19050"><a:solidFill><a:schemeClr val="lt1"/></a:solidFill></a:ln><a:effectLst/></c:spPr></c:dPt>`, i, accent))
	}

	sb.WriteString(g.buildCategoryXML(chart.Categories))
	sb.WriteString(g.buildValueXML(chart.Values))
	sb.WriteString(`</c:ser>`)

	sb.WriteString(`<c:dLbls><c:showLegendKey val="0"/><c:showVal val="0"/><c:showCatName val="0"/><c:showSerName val="0"/><c:showPercent val="0"/><c:showBubbleSize val="0"/><c:showLeaderLines val="1"/></c:dLbls>`)
	sb.WriteString(`<c:firstSliceAng val="0"/>`)
	sb.WriteString(`</c:pieChart>`)

	sb.WriteString(`<c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr>`)
	sb.WriteString(`</c:plotArea>`)
	sb.WriteString(chartLegendXML)
	sb.WriteString(chartFooterXML)

	return sb.String()
}

// createLineChartXML builds line-chart XML aligned with chart-demo/chart2.xml.
func (g *PPTXGenerator) createLineChartXML(chart *ChartData) string {
	var sb strings.Builder
	sb.WriteString(chartCommonHeader)
	sb.WriteString("<c:chart>")
	sb.WriteString(chartTitleXML)
	sb.WriteString("<c:plotArea><c:layout/>")

	// lineChart
	sb.WriteString(`<c:lineChart><c:grouping val="standard"/><c:varyColors val="0"/>`)

	// Single series with chart-demo-aligned line styling.
	accent := accentColors[0]
	sb.WriteString(`<c:ser><c:idx val="0"/><c:order val="0"/>`)
	sb.WriteString(fmt.Sprintf(`<c:tx><c:strRef><c:f>Sheet1!$B$1</c:f><c:strCache><c:ptCount val="1"/><c:pt idx="0"><c:v>%s</c:v></c:pt></c:strCache></c:strRef></c:tx>`, escapeXML(chart.Title)))
	sb.WriteString(fmt.Sprintf(`<c:spPr><a:ln w="28575" cap="rnd"><a:solidFill><a:schemeClr val="%s"/></a:solidFill><a:round/></a:ln><a:effectLst/></c:spPr>`, accent))
	sb.WriteString(`<c:marker><c:symbol val="none"/></c:marker>`)
	sb.WriteString(g.buildCategoryXML(chart.Categories))
	sb.WriteString(g.buildValueXML(chart.Values))
	sb.WriteString(`<c:smooth val="0"/>`)
	sb.WriteString(`</c:ser>`)

	sb.WriteString(`<c:dLbls><c:showLegendKey val="0"/><c:showVal val="0"/><c:showCatName val="0"/><c:showSerName val="0"/><c:showPercent val="0"/><c:showBubbleSize val="0"/></c:dLbls>`)
	sb.WriteString(`<c:smooth val="0"/>`)
	sb.WriteString(`<c:axId val="111111111"/><c:axId val="222222222"/>`)
	sb.WriteString(`</c:lineChart>`)

	// catAx
	sb.WriteString(`<c:catAx><c:axId val="111111111"/><c:scaling><c:orientation val="minMax"/></c:scaling><c:delete val="0"/><c:axPos val="b"/>`)
	sb.WriteString(`<c:numFmt formatCode="General" sourceLinked="1"/>`)
	sb.WriteString(`<c:majorTickMark val="none"/><c:minorTickMark val="none"/><c:tickLblPos val="nextTo"/>`)
	sb.WriteString(chartAxisLineStyle)
	sb.WriteString(chartAxisLabelStyle)
	sb.WriteString(`<c:crossAx val="222222222"/><c:crosses val="autoZero"/><c:auto val="1"/><c:lblAlgn val="ctr"/><c:lblOffset val="100"/><c:noMultiLvlLbl val="0"/>`)
	sb.WriteString(`</c:catAx>`)

	// valAx
	sb.WriteString(`<c:valAx><c:axId val="222222222"/><c:scaling><c:orientation val="minMax"/></c:scaling><c:delete val="0"/><c:axPos val="l"/>`)
	sb.WriteString(chartGridlineStyle)
	sb.WriteString(`<c:numFmt formatCode="General" sourceLinked="1"/>`)
	sb.WriteString(`<c:majorTickMark val="none"/><c:minorTickMark val="none"/><c:tickLblPos val="nextTo"/>`)
	sb.WriteString(`<c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr>`)
	sb.WriteString(chartAxisLabelStyle)
	sb.WriteString(`<c:crossAx val="111111111"/><c:crosses val="autoZero"/><c:crossBetween val="between"/>`)
	sb.WriteString(`</c:valAx>`)

	sb.WriteString(`<c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr>`)
	sb.WriteString(`</c:plotArea>`)
	sb.WriteString(chartLegendXML)
	sb.WriteString(chartFooterXML)

	return sb.String()
}

// buildCategoryXML builds category-axis XML in a compact chart-demo-compatible format.
func (g *PPTXGenerator) buildCategoryXML(categories []string) string {
	if len(categories) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<c:cat><c:strRef><c:f>Sheet1!$A$2:$A$%d</c:f><c:strCache><c:ptCount val="%d"/>`, len(categories)+1, len(categories)))
	for i, cat := range categories {
		sb.WriteString(fmt.Sprintf(`<c:pt idx="%d"><c:v>%s</c:v></c:pt>`, i, escapeXML(cat)))
	}
	sb.WriteString(`</c:strCache></c:strRef></c:cat>`)
	return sb.String()
}

// buildValueXML builds value-axis XML in a compact chart-demo-compatible format.
func (g *PPTXGenerator) buildValueXML(values []float64) string {
	if len(values) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<c:val><c:numRef><c:f>Sheet1!$B$2:$B$%d</c:f><c:numCache><c:formatCode>General</c:formatCode><c:ptCount val="%d"/>`, len(values)+1, len(values)))
	for i, val := range values {
		sb.WriteString(fmt.Sprintf(`<c:pt idx="%d"><c:v>%g</c:v></c:pt>`, i, val))
	}
	sb.WriteString(`</c:numCache></c:numRef></c:val>`)
	return sb.String()
}

// escapeXML escapes XML special characters.
func escapeXML(s string) string {
	result := ""
	for _, c := range s {
		switch c {
		case '&':
			result += "&amp;"
		case '<':
			result += "&lt;"
		case '>':
			result += "&gt;"
		case '"':
			result += "&quot;"
		case '\'':
			result += "&apos;"
		default:
			result += string(c)
		}
	}
	return result
}

// ======= Raw OOXML Assembly (LLM-generated XML) =======

// RawSlideResult stores one slide of OOXML XML generated directly by the LLM.
type RawSlideResult struct {
	SlideXML  string // Full slideN.xml payload.
	ChartXML  string // Optional full chartN.xml payload; empty means no chart.
	ImageData []byte // Optional image binary payload.
	ImageMIME string // Optional image MIME such as image/png or image/jpeg.
	ImagePos  string // Image position: "right" | "left" | "background" | "center".
}

// AssembleRawOOXML assembles a PPTX ZIP from raw LLM-generated XML plus
// the structural files generated by Go.
func AssembleRawOOXML(themeXML string, slides []RawSlideResult, opts PPTXOptions) ([]byte, error) {
	if len(slides) == 0 {
		return nil, fmt.Errorf("slides cannot be empty")
	}

	slideCount := len(slides)

	// Count charts and images.
	chartCount := 0
	imageCount := 0
	for _, s := range slides {
		if s.ChartXML != "" {
			chartCount++
		}
		if len(s.ImageData) > 0 {
			imageCount++
		}
	}

	g := NewPPTXGenerator()

	// Build the structural base files using the existing generator logic.
	theme := getTheme(opts.Theme)
	files := g.buildBaseFiles(opts, slideCount, chartCount, imageCount > 0, theme)

	// If images are present, regenerate Content_Types with PNG/JPEG defaults.
	if imageCount > 0 {
		files["[Content_Types].xml"] = g.generateContentTypes(slideCount, chartCount, true)
	}

	// Override the theme with the LLM-generated theme XML.
	if themeXML != "" {
		files["ppt/theme/theme1.xml"] = themeXML
	}

	// Store binary payloads separately from the string map.
	binaryFiles := make(map[string][]byte)

	// Add slide XML, chart support files, and image files for each slide.
	chartIndex := 0
	imageIndex := 0
	for i, slide := range slides {
		slideNum := i + 1
		slidePath := fmt.Sprintf("ppt/slides/slide%d.xml", slideNum)
		relsPath := fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", slideNum)

		slideXML := slide.SlideXML

		// Build the relationship set for this slide. rId1 is always the slideLayout.
		hasChart := slide.ChartXML != ""
		hasImage := len(slide.ImageData) > 0

		var rels []string
		rels = append(rels, `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>`)

		if hasChart {
			chartIndex++
			chartPath := fmt.Sprintf("ppt/charts/chart%d.xml", chartIndex)
			files[chartPath] = slide.ChartXML

			stylePath := fmt.Sprintf("ppt/charts/style%d.xml", chartIndex)
			files[stylePath] = chartStyleXMLDefault

			colorsPath := fmt.Sprintf("ppt/charts/colors%d.xml", chartIndex)
			files[colorsPath] = chartColorsXML

			chartRelsPath := fmt.Sprintf("ppt/charts/_rels/chart%d.xml.rels", chartIndex)
			files[chartRelsPath] = g.createChartRelsXML(chartIndex)

			rels = append(rels, fmt.Sprintf(`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="../charts/chart%d.xml"/>`, chartIndex))
		}

		if hasImage {
			imageIndex++
			imageExt := imageExtensionFromMIME(slide.ImageMIME)
			imageMediaPath := fmt.Sprintf("ppt/media/image%d.%s", imageIndex, imageExt)
			binaryFiles[imageMediaPath] = slide.ImageData

			imageRId := fmt.Sprintf("rIdImg%d", imageIndex)
			rels = append(rels, fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image%d.%s"/>`, imageRId, imageIndex, imageExt))

			// Replace the image placeholder shape with a <p:pic> element.
			slideXML = replaceImagePlaceholder(slideXML, slideNum, imageRId)
		}

		files[slidePath] = slideXML

		// Build slide rels.
		relsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(rels, "") + `</Relationships>`
		files[relsPath] = relsXML
	}

	// Package the output as a ZIP archive.
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// Write XML files first.
	for path, content := range files {
		f, err := w.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create file %s: %w", path, err)
		}
		if _, err = f.Write([]byte(content)); err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	// Then write binary files such as images.
	for path, data := range binaryFiles {
		f, err := w.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create binary file %s: %w", path, err)
		}
		if _, err = f.Write(data); err != nil {
			return nil, fmt.Errorf("failed to write binary file %s: %w", path, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// replaceImagePlaceholder swaps an ImagePlaceholder shape in slide XML with a <p:pic> element.
func replaceImagePlaceholder(slideXML string, slideNum int, imageRId string) string {
	placeholderText := fmt.Sprintf("IMG_PLACEHOLDER_%d", slideNum)

	// Return unchanged when the placeholder text is missing.
	if !strings.Contains(slideXML, placeholderText) {
		return slideXML
	}

	// Match the full <p:sp> block that contains the placeholder.
	re := regexp.MustCompile(`(?s)<p:sp>\s*<p:nvSpPr>\s*<p:cNvPr[^>]*name="ImagePlaceholder"[^/]*/>\s*<p:cNvSpPr[^/]*/>\s*<p:nvPr[^/]*/>\s*</p:nvSpPr>\s*<p:spPr>\s*<a:xfrm>\s*<a:off x="(\d+)" y="(\d+)"[^/]*/>\s*<a:ext cx="(\d+)" cy="(\d+)"[^/]*/>\s*</a:xfrm>[\s\S]*?</p:sp>`)

	match := re.FindStringSubmatch(slideXML)
	if match == nil {
		// Fallback: match any <p:sp> block containing the placeholder text.
		reSimple := regexp.MustCompile(`(?s)<p:sp>[\s\S]*?` + regexp.QuoteMeta(placeholderText) + `[\s\S]*?</p:sp>`)
		simpleMatch := reSimple.FindString(slideXML)
		if simpleMatch == "" {
			return slideXML
		}

		// Use a default placement when exact geometry is unavailable.
		picXML := fmt.Sprintf(`<p:pic>
    <p:nvPicPr>
        <p:cNvPr id="100" name="Picture %d"/>
        <p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr>
        <p:nvPr/>
    </p:nvPicPr>
    <p:blipFill>
        <a:blip r:embed="%s"/>
        <a:stretch><a:fillRect/></a:stretch>
    </p:blipFill>
    <p:spPr>
        <a:xfrm>
            <a:off x="6600000" y="200000"/>
            <a:ext cx="5200000" cy="6400000"/>
        </a:xfrm>
        <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
    </p:spPr>
</p:pic>`, slideNum, imageRId)

		return strings.Replace(slideXML, simpleMatch, picXML, 1)
	}

	// Extract placeholder position and size.
	x := match[1]
	y := match[2]
	cx := match[3]
	cy := match[4]

	picXML := fmt.Sprintf(`<p:pic>
    <p:nvPicPr>
        <p:cNvPr id="100" name="Picture %d"/>
        <p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr>
        <p:nvPr/>
    </p:nvPicPr>
    <p:blipFill>
        <a:blip r:embed="%s"/>
        <a:stretch><a:fillRect/></a:stretch>
    </p:blipFill>
    <p:spPr>
        <a:xfrm>
            <a:off x="%s" y="%s"/>
            <a:ext cx="%s" cy="%s"/>
        </a:xfrm>
        <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
    </p:spPr>
</p:pic>`, slideNum, imageRId, x, y, cx, cy)

	return strings.Replace(slideXML, match[0], picXML, 1)
}

// GetThemeXMLSpec returns a theme.xml template suitable for LLM prompts.
func GetThemeXMLSpec(primaryColor, accentColor string) string {
	if primaryColor == "" {
		primaryColor = "1A73E8"
	}
	if accentColor == "" {
		accentColor = "E8710A"
	}
	return generateThemeXML(&SlideTheme{
		PrimaryColor: primaryColor,
		AccentColor:  accentColor,
	})
}
