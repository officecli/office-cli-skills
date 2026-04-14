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

// SlideTheme 主题配色方案
type SlideTheme struct {
	PrimaryColor   string `json:"primaryColor"`             // 主色 hex (60%)
	AccentColor    string `json:"accentColor"`              // 强调色 hex (30%)
	HighlightColor string `json:"highlightColor,omitempty"` // 点缀色 hex (10%)
	BackgroundType string `json:"backgroundType"`           // "gradient" | "solid" | "dark"
	BgColor1       string `json:"bgColor1"`                 // 背景色1
	BgColor2       string `json:"bgColor2,omitempty"`       // 背景色2（渐变用）
	TextColor      string `json:"textColor,omitempty"`      // 正文文字色 hex
	TitleTextColor string `json:"titleTextColor,omitempty"` // 标题文字色 hex
	FontFamily     string `json:"fontFamily,omitempty"`     // 西文字体（latin）
	EAFontFamily   string `json:"eaFontFamily,omitempty"`   // 东亚字体（中日韩）
}

// ChartData 图表数据
type ChartData struct {
	Type       string    `json:"type"`       // "bar" | "pie" | "line"
	Categories []string  `json:"categories"` // X 轴标签
	Values     []float64 `json:"values"`     // 数值
	Title      string    `json:"title"`      // 图表标题
}

// MetricCard 指标卡片（大字 KPI 展示）
type MetricCard struct {
	Label string `json:"label"` // 指标名称，如 "2025 市场规模"
	Value string `json:"value"` // 指标数值，如 "$372.9 亿"
	Note  string `json:"note"`  // 补充说明，如 "↑ CAGR 5.90%"
}

// Slide 表示单张幻灯片的数据
// SlideSection 两级标题结构：一级标题（短、带序号、染色）+ 二级描述（详细内容）
type SlideSection struct {
	Heading string `json:"heading"` // 一级标题（如 "01 国际立法"），带序号，不超过5个字
	Detail  string `json:"detail"`  // 二级描述，不超过10个字的小标题或详细说明
}

type Slide struct {
	Title       string         `json:"title"`              // 幻灯片标题
	Content     string         `json:"content"`            // 幻灯片内容
	IsTitle     bool           `json:"isTitle"`            // 是否为封面标题页
	Layout      string         `json:"layout,omitempty"`   // "title" | "content" | "chart" | "dashboard"
	Variant     string         `json:"variant,omitempty"`  // 具体视觉变体
	Subtitle    string         `json:"subtitle,omitempty"` // 副标题（title 布局用）
	Points      []string       `json:"points,omitempty"`   // 分点内容（简单列表）
	Sections    []SlideSection `json:"sections,omitempty"` // 两级标题结构（罗列描述用）
	Chart       *ChartData     `json:"chart,omitempty"`    // 图表数据
	Metrics     []MetricCard   `json:"metrics,omitempty"`  // 指标卡片（dashboard 布局用）
	Source      string         `json:"source,omitempty"`   // 数据来源脚注
	BgColor     string         `json:"bgColor,omitempty"`  // 每页独立背景色（6位hex），覆盖 theme 默认背景
	BgColor2    string         `json:"bgColor2,omitempty"` // 每页背景渐变色2（6位hex，可选）
	HasImage    bool           `json:"hasImage,omitempty"` // 是否启用图片页
	ImagePrompt string         `json:"imagePrompt,omitempty"`
	ImagePos    string         `json:"imagePos,omitempty"` // "right" | "left" | "background" | "center" | "top" | "bottom" | "diagonal"
	ImageData   []byte         `json:"-"`
	ImageMIME   string         `json:"-"`
}

// PPTXOptions 配置生成选项
type PPTXOptions struct {
	Title       string      // 文档标题
	Creator     string      // 作者
	Theme       *SlideTheme // 主题配色
	StylePreset string      // 风格预设
}

// PPTXGenerator PPTX 生成器
type PPTXGenerator struct{}

// NewPPTXGenerator 创建 PPTX 生成器实例
func NewPPTXGenerator() *PPTXGenerator {
	return &PPTXGenerator{}
}

// defaultTheme 默认主题配色
func defaultTheme() *SlideTheme {
	return &SlideTheme{
		PrimaryColor:   "1A73E8",
		AccentColor:    "E8710A",
		BackgroundType: "gradient",
		BgColor1:       "F0F4FF",
		BgColor2:       "FFFFFF",
		FontFamily:     "Helvetica Neue",
		EAFontFamily:   "PingFang SC",
	}
}

// getTheme 获取有效的主题（合并默认值）
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

// Generate 生成 PPTX 并返回字节流
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

	// 构建所有必要的文件
	files := g.buildBaseFiles(opts, len(slides), chartCount, imageCount > 0, theme)

	// 添加幻灯片文件和图表文件（含嵌入 xlsx 二进制）
	slideFiles, binaryFiles, err := g.generateSlidesWithEmbeds(slides, theme, stylePreset)
	if err != nil {
		return nil, err
	}
	for path, content := range slideFiles {
		files[path] = content
	}

	// 写入所有 XML 文件到 zip
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

	// 写入二进制嵌入文件（chart 数据 xlsx）
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

// buildBaseFiles 构建基础 XML 文件
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

// generateContentTypes 动态生成 [Content_Types].xml
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

	// 如果有图片，添加 PNG 和 JPEG 的 Default Extension
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

// generateCoreXML 生成 docProps/core.xml
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

// generateAppXML 生成 docProps/app.xml
func (g *PPTXGenerator) generateAppXML(slideCount int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
    <Application>officecli PPTX Generator</Application>
    <Slides>%d</Slides>
</Properties>`, slideCount)
}

// generatePresentationXML 动态生成 ppt/presentation.xml
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

// generatePresentationRels 动态生成 ppt/_rels/presentation.xml.rels
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

// isDarkTheme 判断是否为深色背景主题
func isDarkTheme(theme *SlideTheme) bool {
	return theme.BackgroundType == "dark"
}

// isDarkColor 判断一个 hex 颜色是否为深色（基于相对亮度）
func isDarkColor(hexColor string) bool {
	return colorLuminance(hexColor) < 128
}

// colorLuminance 计算颜色的相对亮度 (0-255)
func colorLuminance(hexColor string) float64 {
	if len(hexColor) != 6 {
		return 128 // 未知颜色返回中等亮度
	}
	r, g, b := 0, 0, 0
	fmt.Sscanf(hexColor, "%02x%02x%02x", &r, &g, &b)
	return 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
}

// blendColor 将两个颜色按比例混合：ratio=0 返回 color1，ratio=1 返回 color2
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

// sanitizeGradient 修复危险的渐变配色：如果 bgColor1 和 bgColor2 亮度差距过大
// （一端浅色一端深色），会导致无论用什么文字颜色都在某一端不可读。
// 修复策略：将 bgColor2 拉向 bgColor1，使整个渐变保持在同一亮度方向。
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

	// 亮度差 > 80 即认为跨度过大（例如白色~255 到深蓝~60，差值~195）
	// 允许的最大亮度差为 80，超出部分将 bgColor2 拉向 bgColor1
	const maxLumDiff = 80.0
	if lumDiff <= maxLumDiff {
		return bgColor1, bgColor2 // 渐变安全，不修改
	}

	// 计算需要混合的比例，将 bgColor2 拉向 bgColor1
	// ratio = 0 完全保留 bgColor2，ratio = 1 完全变成 bgColor1
	ratio := 1.0 - maxLumDiff/lumDiff
	newBgColor2 := blendColor(bgColor2, bgColor1, ratio)
	return bgColor1, newBgColor2
}

// getTextColor 根据主题返回正文文字颜色
func getTextColor(theme *SlideTheme) string {
	if isDarkTheme(theme) {
		return "EEEEEE"
	}
	return "333333"
}

// getSlideTextColor 根据 slide 背景色或 theme 返回正文文字颜色
// 基于 bgColor1（渐变已通过 sanitizeGradient 保证亮度一致性）
func getSlideTextColor(slide Slide, theme *SlideTheme) string {
	if slide.BgColor != "" && isDarkColor(slide.BgColor) {
		return "EEEEEE"
	}
	return getTextColor(theme)
}

// isSimilarHue 判断两个 hex 颜色是否属于同色系（色相相近且对比度不足）
// 用于防止同色系文字放在同色系背景上导致不可读
func isSimilarHue(hexColor1, hexColor2 string) bool {
	if len(hexColor1) != 6 || len(hexColor2) != 6 {
		return false
	}
	r1, g1, b1 := 0, 0, 0
	r2, g2, b2 := 0, 0, 0
	fmt.Sscanf(hexColor1, "%02x%02x%02x", &r1, &g1, &b1)
	fmt.Sscanf(hexColor2, "%02x%02x%02x", &r2, &g2, &b2)

	// 计算两个颜色的亮度差
	lum1 := 0.299*float64(r1) + 0.587*float64(g1) + 0.114*float64(b1)
	lum2 := 0.299*float64(r2) + 0.587*float64(g2) + 0.114*float64(b2)
	lumDiff := lum1 - lum2
	if lumDiff < 0 {
		lumDiff = -lumDiff
	}
	// 如果亮度差足够大（>100），认为对比度充足，不算同色系冲突
	if lumDiff > 100 {
		return false
	}

	// 计算 RGB 欧氏距离，距离过小说明颜色太相似
	dr := float64(r1 - r2)
	dg := float64(g1 - g2)
	db := float64(b1 - b2)
	dist := dr*dr + dg*dg + db*db
	// 距离阈值：sqrt(dist) < 120 即认为太相似（约等于各通道差 ~70）
	return dist < 14400
}

// getTitleColor 根据主题返回标题颜色
// 会检查 primaryColor 是否与背景同色系，如是则回退到安全颜色
func getTitleColor(theme *SlideTheme) string {
	if isDarkTheme(theme) {
		return "FFFFFF"
	}
	// 检查 primaryColor 是否与背景色同色系
	bgColor := theme.BgColor1
	if bgColor != "" && isSimilarHue(theme.PrimaryColor, bgColor) {
		// 同色系冲突：根据背景亮度选择安全颜色
		if isDarkColor(bgColor) {
			return "FFFFFF"
		}
		return "333333"
	}
	return theme.PrimaryColor
}

// getSlideTitleColor 根据 slide 背景色或 theme 返回标题颜色
// 基于 bgColor1 做同色系检测（渐变已通过 sanitizeGradient 保证亮度一致性）
func getSlideTitleColor(slide Slide, theme *SlideTheme) string {
	effectiveBg := theme.BgColor1
	if slide.BgColor != "" {
		effectiveBg = slide.BgColor
	}

	if effectiveBg != "" && isDarkColor(effectiveBg) {
		return "FFFFFF"
	}

	titleColor := getTitleColor(theme)
	// 检查标题颜色是否与背景同色系
	if effectiveBg != "" && isSimilarHue(titleColor, effectiveBg) {
		if isDarkColor(effectiveBg) {
			return "FFFFFF"
		}
		return "333333"
	}
	return titleColor
}

// getEffectiveBgColor 获取 slide 的主要背景色（用于文字对比度检测）
// 渐变已通过 sanitizeGradient 保证亮度一致性，所以用 bgColor1 即可
func getEffectiveBgColor(slide Slide, theme *SlideTheme) string {
	if slide.BgColor != "" {
		return slide.BgColor
	}
	return theme.BgColor1
}

// getSafeTextColorForBg 检查文字颜色是否与背景色有足够对比度
// 如果文字颜色与背景色同色系，则回退到安全颜色
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

// generateBackgroundXML 根据主题生成幻灯片背景 XML
func generateBackgroundXML(theme *SlideTheme) string {
	return generateBackgroundXMLWithColors(theme.BackgroundType, theme.BgColor1, theme.BgColor2)
}

// generateSlideBackgroundXML 根据 slide 独立背景色或 theme 默认背景生成 XML
// 如果 slide 指定了 bgColor，则使用 slide 级别的背景色；否则回退到 theme 级别
func generateSlideBackgroundXML(slide Slide, theme *SlideTheme) string {
	if slide.BgColor != "" {
		bgColor2 := slide.BgColor2
		if bgColor2 != "" {
			// slide 指定了两个背景色，使用渐变
			return generateBackgroundXMLWithColors("gradient", slide.BgColor, bgColor2)
		}
		// slide 只指定了一个背景色，使用纯色
		return generateBackgroundXMLWithColors("solid", slide.BgColor, "")
	}
	return generateBackgroundXML(theme)
}

// generateBackgroundXMLWithColors 根据背景类型和颜色生成背景 XML
func generateBackgroundXMLWithColors(bgType, bgColor1, bgColor2 string) string {
	switch bgType {
	case "gradient":
		// 修复危险渐变：如果 bgColor1 和 bgColor2 亮度跨度过大，自动收敛
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

// createSlideXMLEnhanced 根据布局创建增强的幻灯片 XML
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

// generateSubtitleXML 生成二级标题 XML，放在一级标题下方
// shapeID: 该 shape 的 id，y: 纵坐标位置
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

// generateFooterXML 生成幻灯片底部脚注（当前仅保留数据来源），起始 shape id 从 baseID 开始
func generateFooterXML(source string, slideNum, totalSlides, baseID int, theme *SlideTheme) string {
	_, _ = slideNum, totalSlides
	textColor := getTextColor(theme)
	result := ""

	// 数据来源脚注（左下角）
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
                        <a:off x="400000" y="6500000"/>
                        <a:ext cx="9000000" cy="280000"/>
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
                                <a:solidFill><a:srgbClr val="%s"><a:alpha val="60000"/></a:srgbClr></a:solidFill>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, baseID, textColor, escapeXML(source))
	}

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

// createTitleSlideXML 创建封面标题页 XML
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
                        <a:pPr algn="ctr"/>
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
            </p:sp>`, subtitleX, subtitleY, subtitleCX, subtitleCY, subtitleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(subtitle))
	}

	// 装饰线条
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
            </p:grpSpPr>%s%s
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
</p:sld>`, bgXML, imageXML, overlayXML, titleX, titleY, titleCX, titleCY, titleAlign, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title), decorLineXML, subtitleXML, generateFooterXML(slide.Source, slideNum, totalSlides, 10, theme))
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

func createPointCardsXML(points []string, x, y, cx, cy int, accentColor, textColor, fontFamily, eaFontFamily string) string {
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
                    <a:solidFill><a:srgbClr val="FFFFFF"><a:alpha val="94000"/></a:srgbClr></a:solidFill>
                    <a:ln w="12700">
                        <a:solidFill><a:srgbClr val="%s"><a:alpha val="18000"/></a:srgbClr></a:solidFill>
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
            </p:sp>`, cardID, idx+1, x, cardY, cx, cardHeight, accentColor, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(displayText), accentID, idx+1, x, cardY, cardHeight, accentColor))
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
                            <a:rPr lang="zh-CN" sz="1200" b="1">
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

// createContentSlideXML 创建内容页 XML（支持分点）
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

	// 标题左侧装饰色块
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
            </p:sp>`, titleCY-200000, accentColor)

	// 构建内容区域
	contentXML := ""
	if len(slide.Sections) > 0 {
		contentXML = createSectionCardsXML(slide.Sections, contentX, contentY, contentCX, contentCY, accentColor, textColor, fontFamily, eaFontFamily, stylePreset.ContentCardFill, stylePreset.ContentCardAlpha)
	} else if len(slide.Points) > 0 {
		if imagePos == "" || imagePos == "bottom" || imagePos == "top" {
			contentXML = createPointCardsXML(slide.Points, contentX, contentY, contentCX, contentCY, accentColor, textColor, fontFamily, eaFontFamily)
		} else {
			// 分点布局：每个要点一个段落，带项目符号
			// 根据 points 数量动态调整间距和字号
			pointCount := len(slide.Points)
			pointFontSize := 2000 // 默认 20pt
			pointSpcBefore := 600 // 默认段前间距
			if pointCount <= 3 {
				pointFontSize = 2200 // 少量要点用更大字号
				pointSpcBefore = 1200
			} else if pointCount <= 4 {
				pointSpcBefore = 800
			}
			paragraphs := ""
			for _, point := range slide.Points {
				paragraphs += fmt.Sprintf(`
                    <a:p>
                        <a:pPr marL="342900" indent="-342900" algn="l">
                            <a:spcBef><a:spcPts val="%d"/></a:spcBef>
                            <a:buFont typeface="Arial"/>
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
                    </a:p>`, pointSpcBefore, pointFontSize, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(point))
			}
			contentXML = fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="3" name="Content"/>
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
                    <a:lstStyle/>%s
                </p:txBody>
            </p:sp>`, contentX, contentY, contentCX, contentCY, paragraphs)
		}
	} else if slide.Content != "" {
		// 纯文本内容，根据内容长度自动调整字体大小（最小 14pt）
		contentFontSize := 2000 // 默认 20pt
		contentLen := len([]rune(slide.Content))
		if contentLen > 500 {
			contentFontSize = 1600 // 16pt
		} else if contentLen > 200 {
			contentFontSize = 1800 // 18pt
		}
		// 字体下限保障：不低于 14pt
		if contentFontSize < 1400 {
			contentFontSize = 1400
		}
		contentXML = fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="3" name="Content"/>
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
                        <a:pPr algn="l"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="%d">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, contentX, contentY, contentCX, contentCY, contentFontSize, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Content))
	}

	// 二级标题
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
            </p:sp>%s%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, titleDecoXML, titleY, titleCY, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title), subtitleXML, contentXML, imageXML, generateFooterXML(slide.Source, slideNum, totalSlides, 10, theme))
}

// createChartSlideXML 创建包含图表的幻灯片 XML
// createChartAsShapesSlideXML renders chart data as DrawingML shapes (colored bars + labels)
// instead of embedded OOXML chart objects, for OfficeSDK compatibility.
func (g *PPTXGenerator) createChartAsShapesSlideXML(slide Slide, theme *SlideTheme, _ PPTXStylePreset, slideNum, totalSlides int) string {
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
            </p:sp>%s%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title),
		subtitleXML, chartShapesXML, pointsXML, generateFooterXML(slide.Source, slideNum, totalSlides, 100, theme))
}

// renderChartAsShapes renders chart data as colored rectangles with labels
func (g *PPTXGenerator) renderChartAsShapes(chart *ChartData, theme *SlideTheme, bgColor string, shapeID *int) string {
	if len(chart.Values) == 0 {
		return ""
	}

	textColor := getSafeTextColorForBg(getTextColor(theme), bgColor)
	colors := []string{theme.PrimaryColor, theme.AccentColor, "4CAF50", "FF9800", "9C27B0", "00BCD4", "E91E63", "795548"}

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
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, *shapeID, textColor, escapeXML(chart.Title))
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
                            <a:rPr lang="zh-CN" sz="1200" b="1">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, *shapeID, i+1, barX, barY-300000, barWidth, textColor, formatValue(val))

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
                            <a:rPr lang="zh-CN" sz="1200">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                </p:txBody>
            </p:sp>`, *shapeID, i+1, barX, chartTop+chartHeight+50000, barWidth, textColor, escapeXML(label))
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
                            <a:buFont typeface="Arial"/>
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
                    </a:p>`, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(point))
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

// createDashboardAsShapesSlideXML renders dashboard layout without embedded charts
func (g *PPTXGenerator) createDashboardAsShapesSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int) string {
	return g.createDashboardSlideXML(slide, theme, stylePreset, false, 0, slideNum, totalSlides)
}

func (g *PPTXGenerator) createChartSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, chartIndex, slideNum, totalSlides int) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily

	// 二级标题
	subtitleXML := generateSubtitleXML(slide.Subtitle, 6, 1000000, theme)

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
            </p:sp>%s
            <p:graphicFrame>
                <p:nvGraphicFramePr>
                    <p:cNvPr id="5" name="Chart %d"/>
                    <p:cNvGraphicFramePr>
                        <a:graphicFrameLocks noGrp="1"/>
                    </p:cNvGraphicFramePr>
                    <p:nvPr/>
                </p:nvGraphicFramePr>
                <p:xfrm>
                    <a:off x="1000000" y="1500000"/>
                    <a:ext cx="10200000" cy="4800000"/>
                </p:xfrm>
                <a:graphic>
                    <a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart">
                        <c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id="rId1"/>
                    </a:graphicData>
                </a:graphic>
            </p:graphicFrame>%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title), subtitleXML, chartIndex, generateFooterXML(slide.Source, slideNum, totalSlides, 10, theme))
}

// createDashboardSlideXML 创建仪表盘布局 XML：指标卡片 + 可选图表 + 可选要点
func (g *PPTXGenerator) createDashboardSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, hasChart bool, chartIndex, slideNum, totalSlides int) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	primaryColor := theme.PrimaryColor
	accentColor := getSafeTextColorForBg(theme.AccentColor, bgColor)

	// 标题左侧装饰色块
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

	// 指标卡片区域（最多 4 个卡片，排成一行）
	metricsXML := ""
	numMetrics := len(slide.Metrics)
	if numMetrics > 4 {
		numMetrics = 4
	}
	hasLowerContent := hasChart || len(slide.Points) > 0
	if numMetrics > 0 {
		// 计算卡片位置：在 700000 ~ 11500000 范围内平均分布
		totalWidth := 10800000
		cardGap := 150000
		cardWidth := (totalWidth - (numMetrics-1)*cardGap) / numMetrics
		cardHeight := 950000
		// 如果没有下方内容（图表/要点），卡片在可用区域内垂直居中
		// 可用区域：标题+副标题下方(1050000) 到 脚注上方(6400000)
		cardY := 1050000
		if !hasLowerContent {
			availTop := 1050000
			availBottom := 6400000
			cardHeight = 1280000
			cardY = availTop + (availBottom-availTop-cardHeight)/2
		}

		for i := 0; i < numMetrics; i++ {
			m := slide.Metrics[i]
			cardX := 700000 + i*(cardWidth+cardGap)
			shapeID := 20 + i*3

			// 卡片背景（圆角矩形带轻微阴影效果）
			metricsXML += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="MetricBg%d"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="%d" y="%d"/>
                        <a:ext cx="%d" cy="%d"/>
                    </a:xfrm>
                    <a:prstGeom prst="roundRect"><a:avLst><a:gd name="adj" fmla="val 5000"/></a:avLst></a:prstGeom>
                    <a:solidFill><a:srgbClr val="%s"><a:alpha val="92000"/></a:srgbClr></a:solidFill>
                    <a:ln w="9525"><a:solidFill><a:srgbClr val="%s"><a:alpha val="90000"/></a:srgbClr></a:solidFill></a:ln>
                </p:spPr>
            </p:sp>`, shapeID, i, cardX, cardY, cardWidth, cardHeight, primaryColor, primaryColor)

			// 卡片文字内容
			noteXML := ""
			if m.Note != "" {
				noteXML = fmt.Sprintf(`
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1100">
                                <a:solidFill><a:srgbClr val="FFFFFF"><a:alpha val="76000"/></a:srgbClr></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>`, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(m.Note))
			}

			metricsXML += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="%d" name="MetricText%d"/>
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
                    <a:bodyPr anchor="ctr" lIns="72000" rIns="72000"/>
                    <a:lstStyle/>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1200">
                                <a:solidFill><a:srgbClr val="FFFFFF"><a:alpha val="76000"/></a:srgbClr></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>
                    <a:p>
                        <a:pPr algn="ctr"/>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="3000" b="1">
                                <a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>%s
                </p:txBody>
            </p:sp>`, shapeID+1, i, cardX, cardY, cardWidth, cardHeight,
				escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(m.Label),
				escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(m.Value),
				noteXML)
		}
	}

	// 下半区内容：图表 + 要点
	lowerContentXML := ""
	lowerY := 2200000
	if numMetrics > 0 {
		lowerY = 2200000
	}

	if hasChart && len(slide.Points) > 0 {
		// 左侧图表 + 右侧要点
		lowerContentXML += fmt.Sprintf(`
            <p:graphicFrame>
                <p:nvGraphicFramePr>
                    <p:cNvPr id="40" name="Chart"/>
                    <p:cNvGraphicFramePr>
                        <a:graphicFrameLocks noGrp="1"/>
                    </p:cNvGraphicFramePr>
                    <p:nvPr/>
                </p:nvGraphicFramePr>
                <p:xfrm>
                    <a:off x="400000" y="%d"/>
                    <a:ext cx="6200000" cy="4000000"/>
                </p:xfrm>
                <a:graphic>
                    <a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart">
                        <c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id="rId1"/>
                    </a:graphicData>
                </a:graphic>
            </p:graphicFrame>`, lowerY)

		// 右侧要点
		pointsParagraphs := ""
		for _, point := range slide.Points {
			pointsParagraphs += fmt.Sprintf(`
                    <a:p>
                        <a:pPr marL="228600" indent="-228600" algn="l">
                            <a:buFont typeface="Arial"/>
                            <a:buChar char="●"/>
                        </a:pPr>
                        <a:r>
                            <a:rPr lang="zh-CN" sz="1400">
                                <a:solidFill><a:srgbClr val="%s"/></a:solidFill>
                                <a:latin typeface="%s"/>
                                <a:ea typeface="%s"/>
                            </a:rPr>
                            <a:t>%s</a:t>
                        </a:r>
                    </a:p>`, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(point))
		}
		lowerContentXML += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="41" name="Points"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="6800000" y="%d"/>
                        <a:ext cx="4800000" cy="4000000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t" lIns="72000" rIns="72000"/>
                    <a:lstStyle/>%s
                </p:txBody>
            </p:sp>`, lowerY, pointsParagraphs)
	} else if hasChart {
		// 仅图表（居中）
		lowerContentXML += fmt.Sprintf(`
            <p:graphicFrame>
                <p:nvGraphicFramePr>
                    <p:cNvPr id="40" name="Chart"/>
                    <p:cNvGraphicFramePr>
                        <a:graphicFrameLocks noGrp="1"/>
                    </p:cNvGraphicFramePr>
                    <p:nvPr/>
                </p:nvGraphicFramePr>
                <p:xfrm>
                    <a:off x="700000" y="%d"/>
                    <a:ext cx="10800000" cy="4000000"/>
                </p:xfrm>
                <a:graphic>
                    <a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/chart">
                        <c:chart xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" r:id="rId1"/>
                    </a:graphicData>
                </a:graphic>
            </p:graphicFrame>`, lowerY)
	} else if len(slide.Points) > 0 {
		// 仅要点（全宽）
		pointsParagraphs := ""
		for _, point := range slide.Points {
			pointsParagraphs += fmt.Sprintf(`
                    <a:p>
                        <a:pPr marL="342900" indent="-342900" algn="l">
                            <a:buFont typeface="Arial"/>
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
                    </a:p>`, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(point))
		}
		lowerContentXML += fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr>
                    <p:cNvPr id="41" name="Points"/>
                    <p:cNvSpPr/>
                    <p:nvPr/>
                </p:nvSpPr>
                <p:spPr>
                    <a:xfrm>
                        <a:off x="700000" y="%d"/>
                        <a:ext cx="10800000" cy="4000000"/>
                    </a:xfrm>
                    <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="t"/>
                    <a:lstStyle/>%s
                </p:txBody>
            </p:sp>`, lowerY, pointsParagraphs)
	}

	footerXML := generateFooterXML(slide.Source, slideNum, totalSlides, 60, theme)

	// 二级标题
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

// createSlideRelsWithChart 创建带有图表引用的幻灯片 rels
func (g *PPTXGenerator) createSlideRelsWithChart(chartIndex int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart" Target="../charts/chart%d.xml"/></Relationships>`, chartIndex)
}

func (g *PPTXGenerator) createSlideRelsWithImage(imageIndex int, imageExt string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="../media/image%d.%s"/></Relationships>`, imageIndex, imageExt)
}

// createChartRelsXML 创建图表自身的 rels
func (g *PPTXGenerator) createChartRelsXML(chartIndex int) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId3" Type="http://schemas.microsoft.com/office/2011/relationships/chartColorStyle" Target="colors%d.xml"/><Relationship Id="rId2" Type="http://schemas.microsoft.com/office/2011/relationships/chartStyle" Target="style%d.xml"/><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/oleObject" Target="" TargetMode="External"/></Relationships>`, chartIndex, chartIndex)
}

// createChartEmbeddedXlsx 为图表生成嵌入的最小化 xlsx 数据文件
// 包含与 chart XML 中 strCache/numCache 对应的 Sheet1 数据
func (g *PPTXGenerator) createChartEmbeddedXlsx(chart *ChartData) ([]byte, error) {
	// 构建 Sheet1 数据：第一行是标题行，后续行是分类+数值
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

// createChartXML 生成 OOXML chart XML（严格匹配 chart-demo 格式）
func (g *PPTXGenerator) createChartXML(chart *ChartData) string {
	switch chart.Type {
	case "pie":
		return g.createPieChartXML(chart)
	case "line":
		return g.createLineChartXML(chart)
	default: // "bar" 或默认
		return g.createBarChartXML(chart)
	}
}

// chartCommonHeader 图表 XML 公共头部（严格匹配 chart-demo 命名空间和样式声明）
const chartCommonHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<c:chartSpace xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><c:date1904 val="0"/><c:lang val="en-US"/><c:roundedCorners val="0"/><mc:AlternateContent xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"><mc:Choice Requires="c14" xmlns:c14="http://schemas.microsoft.com/office/drawing/2007/8/2/chart"><c14:style val="102"/></mc:Choice><mc:Fallback><c:style val="2"/></mc:Fallback></mc:AlternateContent>`

// chartTitleXML 图表标题样式（与 chart-demo 一致，使用自动标题样式）
const chartTitleXML = `<c:title><c:overlay val="0"/><c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr><c:txPr><a:bodyPr rot="0" spcFirstLastPara="1" vertOverflow="ellipsis" vert="horz" wrap="square" anchor="ctr" anchorCtr="1"/><a:lstStyle/><a:p><a:pPr><a:defRPr sz="1862" b="0" i="0" u="none" strike="noStrike" kern="1200" spc="0" baseline="0"><a:solidFill><a:schemeClr val="tx1"><a:lumMod val="65000"/><a:lumOff val="35000"/></a:schemeClr></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:pPr><a:endParaRPr lang="en-CN"/></a:p></c:txPr></c:title><c:autoTitleDeleted val="0"/>`

// chartLegendXML 图表图例样式（与 chart-demo 一致）
const chartLegendXML = `<c:legend><c:legendPos val="b"/><c:overlay val="0"/><c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr><c:txPr><a:bodyPr rot="0" spcFirstLastPara="1" vertOverflow="ellipsis" vert="horz" wrap="square" anchor="ctr" anchorCtr="1"/><a:lstStyle/><a:p><a:pPr><a:defRPr sz="1197" b="0" i="0" u="none" strike="noStrike" kern="1200" baseline="0"><a:solidFill><a:schemeClr val="tx1"><a:lumMod val="65000"/><a:lumOff val="35000"/></a:schemeClr></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:pPr><a:endParaRPr lang="en-CN"/></a:p></c:txPr></c:legend>`

// chartFooterXML 图表尾部属性（与 chart-demo 一致）
const chartFooterXML = `<c:plotVisOnly val="1"/><c:dispBlanksAs val="gap"/><c:showDLblsOverMax val="0"/></c:chart><c:spPr><a:noFill/><a:ln><a:noFill/></a:ln><a:effectLst/></c:spPr><c:txPr><a:bodyPr/><a:lstStyle/><a:p><a:pPr><a:defRPr/></a:pPr><a:endParaRPr lang="en-CN"/></a:p></c:txPr><c:externalData r:id="rId1"><c:autoUpdate val="0"/></c:externalData></c:chartSpace>`

// chartAxisLabelStyle 轴标签文字样式（与 chart-demo catAx/valAx 的 c:txPr 一致）
const chartAxisLabelStyle = `<c:txPr><a:bodyPr rot="-60000000" spcFirstLastPara="1" vertOverflow="ellipsis" vert="horz" wrap="square" anchor="ctr" anchorCtr="1"/><a:lstStyle/><a:p><a:pPr><a:defRPr sz="1197" b="0" i="0" u="none" strike="noStrike" kern="1200" baseline="0"><a:solidFill><a:schemeClr val="tx1"><a:lumMod val="65000"/><a:lumOff val="35000"/></a:schemeClr></a:solidFill><a:latin typeface="+mn-lt"/><a:ea typeface="+mn-ea"/><a:cs typeface="+mn-cs"/></a:defRPr></a:pPr><a:endParaRPr lang="en-CN"/></a:p></c:txPr>`

// chartAxisLineStyle 轴线条样式
const chartAxisLineStyle = `<c:spPr><a:noFill/><a:ln w="9525" cap="flat" cmpd="sng" algn="ctr"><a:solidFill><a:schemeClr val="tx1"><a:lumMod val="15000"/><a:lumOff val="85000"/></a:schemeClr></a:solidFill><a:round/></a:ln><a:effectLst/></c:spPr>`

// chartGridlineStyle 主网格线样式
const chartGridlineStyle = `<c:majorGridlines><c:spPr><a:ln w="9525" cap="flat" cmpd="sng" algn="ctr"><a:solidFill><a:schemeClr val="tx1"><a:lumMod val="15000"/><a:lumOff val="85000"/></a:schemeClr></a:solidFill><a:round/></a:ln><a:effectLst/></c:spPr></c:majorGridlines>`

// accentColors scheme 颜色名列表（用于数据点/系列着色）
var accentColors = []string{"accent1", "accent2", "accent3", "accent4", "accent5", "accent6"}

// createBarChartXML 生成柱状图 XML（严格匹配 chart-demo/chart3.xml 格式）
func (g *PPTXGenerator) createBarChartXML(chart *ChartData) string {
	var sb strings.Builder
	sb.WriteString(chartCommonHeader)
	sb.WriteString("<c:chart>")
	sb.WriteString(chartTitleXML)
	sb.WriteString("<c:plotArea><c:layout/>")

	// barChart
	sb.WriteString(`<c:barChart><c:barDir val="col"/><c:grouping val="clustered"/><c:varyColors val="0"/>`)

	// 单系列
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

// createPieChartXML 生成饼图 XML（严格匹配 chart-demo/chart1.xml 格式）
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

	// 每个数据点单独着色（与 chart-demo 一致，使用 scheme accent 色 + 白色边框）
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

// createLineChartXML 生成折线图 XML（严格匹配 chart-demo/chart2.xml 格式）
func (g *PPTXGenerator) createLineChartXML(chart *ChartData) string {
	var sb strings.Builder
	sb.WriteString(chartCommonHeader)
	sb.WriteString("<c:chart>")
	sb.WriteString(chartTitleXML)
	sb.WriteString("<c:plotArea><c:layout/>")

	// lineChart
	sb.WriteString(`<c:lineChart><c:grouping val="standard"/><c:varyColors val="0"/>`)

	// 单系列，线条样式与 chart-demo 一致
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

// buildCategoryXML 构建图表分类轴 XML（与 chart-demo 一致的紧凑格式）
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

// buildValueXML 构建图表数值轴 XML（与 chart-demo 一致的紧凑格式）
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

// escapeXML 转义 XML 特殊字符
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

// ======= Raw OOXML Assembly (LLM 直出 XML) =======

// RawSlideResult 保存 LLM 直接生成的单页 OOXML XML 内容
type RawSlideResult struct {
	SlideXML  string // 完整的 slideN.xml 内容
	ChartXML  string // 可选，完整的 chartN.xml 内容（空字符串表示无图表）
	ImageData []byte // 可选，图片二进制数据
	ImageMIME string // 可选，图片 MIME，如 image/png 或 image/jpeg
	ImagePos  string // 图片位置: "right" | "left" | "background" | "center"
}

// AssembleRawOOXML 将 LLM 生成的原始 XML（theme + slides）与 Go 生成的结构性文件组装为 PPTX zip 字节流
// themeXML: LLM 生成的完整 theme1.xml 内容
// slides: 按序排列的每页 slide XML、可选 chart XML 和可选图片数据
// opts: 文档标题、作者等元信息
func AssembleRawOOXML(themeXML string, slides []RawSlideResult, opts PPTXOptions) ([]byte, error) {
	if len(slides) == 0 {
		return nil, fmt.Errorf("slides cannot be empty")
	}

	slideCount := len(slides)

	// 统计图表数量和图片数量
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

	// 构建结构性基础文件（复用现有逻辑）
	theme := getTheme(opts.Theme)
	files := g.buildBaseFiles(opts, slideCount, chartCount, imageCount > 0, theme)

	// 如果有图片，重新生成 Content_Types（添加 png/jpeg Default）
	if imageCount > 0 {
		files["[Content_Types].xml"] = g.generateContentTypes(slideCount, chartCount, true)
	}

	// 覆盖 theme 为 LLM 生成的内容
	if themeXML != "" {
		files["ppt/theme/theme1.xml"] = themeXML
	}

	// 二进制文件单独存储（不能放在 string map 里）
	binaryFiles := make(map[string][]byte)

	// 添加每页 slide XML、chart 辅助文件、图片文件
	chartIndex := 0
	imageIndex := 0
	for i, slide := range slides {
		slideNum := i + 1
		slidePath := fmt.Sprintf("ppt/slides/slide%d.xml", slideNum)
		relsPath := fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", slideNum)

		slideXML := slide.SlideXML

		// 确定该页需要多少个 relationship（始终有 rId1=slideLayout）
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

			// 替换 slide XML 中的占位 shape 为 <p:pic> 元素
			slideXML = replaceImagePlaceholder(slideXML, slideNum, imageRId)
		}

		files[slidePath] = slideXML

		// 构建 slide rels
		relsXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(rels, "") + `</Relationships>`
		files[relsPath] = relsXML
	}

	// 打包为 zip
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	// 先写 XML 文件
	for path, content := range files {
		f, err := w.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create file %s: %w", path, err)
		}
		if _, err = f.Write([]byte(content)); err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	// 再写二进制文件（图片）
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

// replaceImagePlaceholder 将 slide XML 中的 ImagePlaceholder shape 替换为 <p:pic> 元素
// 查找包含 IMG_PLACEHOLDER_{slideNum} 文本的 <p:sp> 并替换为 <p:pic>
func replaceImagePlaceholder(slideXML string, slideNum int, imageRId string) string {
	placeholderText := fmt.Sprintf("IMG_PLACEHOLDER_%d", slideNum)

	// 如果 slide XML 中没有占位符文本，直接返回
	if !strings.Contains(slideXML, placeholderText) {
		return slideXML
	}

	// 用正则匹配包含占位符的完整 <p:sp>...</p:sp> 块
	// 匹配 name="ImagePlaceholder" 的 shape
	re := regexp.MustCompile(`(?s)<p:sp>\s*<p:nvSpPr>\s*<p:cNvPr[^>]*name="ImagePlaceholder"[^/]*/>\s*<p:cNvSpPr[^/]*/>\s*<p:nvPr[^/]*/>\s*</p:nvSpPr>\s*<p:spPr>\s*<a:xfrm>\s*<a:off x="(\d+)" y="(\d+)"[^/]*/>\s*<a:ext cx="(\d+)" cy="(\d+)"[^/]*/>\s*</a:xfrm>[\s\S]*?</p:sp>`)

	match := re.FindStringSubmatch(slideXML)
	if match == nil {
		// 回退：简单正则匹配包含 IMG_PLACEHOLDER 的 <p:sp> 块
		reSimple := regexp.MustCompile(`(?s)<p:sp>[\s\S]*?` + regexp.QuoteMeta(placeholderText) + `[\s\S]*?</p:sp>`)
		simpleMatch := reSimple.FindString(slideXML)
		if simpleMatch == "" {
			return slideXML
		}

		// 使用默认位置
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

	// 提取占位符的位置和尺寸
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

// GetThemeXMLSpec 返回 theme.xml 的规范模板（供 LLM prompt 引用）
// 传入 primaryColor 和 accentColor（6位 hex），返回完整 theme XML 示例
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
