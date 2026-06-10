package pptxrender

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgilbir/spine/common/dml"
	"github.com/mgilbir/spine/common/enum"
	"github.com/mgilbir/spine/pptx"
)

type Renderer struct {
	opts RenderOptions
}

func NewRenderer(opts RenderOptions) *Renderer {
	return &Renderer{opts: opts.withDefaults()}
}

func (r *Renderer) Render(ctx context.Context, ir PresentationIR) ([]byte, RenderReport, error) {
	opts := r.opts.withDefaults()
	if opts.RelationshipIDs == nil {
		opts.RelationshipIDs = NewSequentialRelationshipAllocator()
	}
	if err := validateIR(ir, opts.StrictValidation); err != nil {
		return nil, RenderReport{}, err
	}

	slideSize := opts.slideSize(ir)
	p := pptx.CreateWidescreen()
	p.Properties.Title = "Go/spine PresentationIR"
	p.Properties.Creator = "pptxrender"

	report := RenderReport{SlideCount: len(ir.Slides)}
	assets := assetMap(ir.Assets)
	var shapePatches []shapePatch
	var tablePatches []tablePatch
	var imageBindings []imageBinding
	mediaCounter := 0

	for slideIndex, slideIR := range ir.Slides {
		if err := ctx.Err(); err != nil {
			return nil, report, err
		}
		slide := p.AddSlide()
		slide.SetName(slideIR.Name)
		slideContext := &SlideContext{
			Slide:           slide,
			SlideIndex:      slideIndex + 1,
			SlideSize:       slideSize,
			Theme:           mergeTheme(ir.Theme, opts.Defaults),
			Units:           opts.Units,
			Assets:          assets,
			AssetResolver:   opts.Assets,
			RelationshipIDs: opts.RelationshipIDs,
			ChartMode:       opts.ChartMode,
			ChartFallback:   opts.ChartFallback,
			Report:          &report,
			shapePatches:    &shapePatches,
			tablePatches:    &tablePatches,
			imageBindings:   &imageBindings,
			mediaCounter:    &mediaCounter,
		}
		addBackground(slideContext, slideIR.Background)
		for _, element := range slideIR.Elements {
			if err := ctx.Err(); err != nil {
				return nil, report, err
			}
			if err := renderElement(ctx, slideContext, element); err != nil {
				return nil, report, err
			}
		}
	}

	var buf bytes.Buffer
	if err := p.SaveTo(&buf); err != nil {
		return nil, report, err
	}
	if !opts.EnableOOXMLPatch {
		return buf.Bytes(), report, nil
	}

	editor, err := NewPackageEditor(buf.Bytes())
	if err != nil {
		return nil, report, err
	}
	pipeline := PatchPipeline{
		patchers: []OOXMLPatcher{
			SlideSizePatcher{SlideSize: slideSize, Units: opts.Units},
			ShapeStylePatcher{Patches: shapePatches},
			TableBorderPatcher{Patches: tablePatches},
			MediaPartPatcher{Bindings: imageBindings},
			ImageRelationshipPatcher{Bindings: imageBindings},
		},
	}
	if err := pipeline.Apply(editor); err != nil {
		return nil, report, err
	}
	deck, err := editor.Bytes()
	if err != nil {
		return nil, report, err
	}
	return deck, report, nil
}

func (r *Renderer) RenderToFile(ctx context.Context, ir PresentationIR, outputPath string) (RenderReport, error) {
	deck, report, err := r.Render(ctx, ir)
	if err != nil {
		return report, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return report, err
	}
	return report, os.WriteFile(outputPath, deck, 0644)
}

type SlideContext struct {
	Slide           *pptx.Slide
	SlideIndex      int
	SlideSize       SlideSize
	Theme           Theme
	Units           UnitOptions
	Assets          map[string]Asset
	AssetResolver   AssetResolver
	RelationshipIDs RelationshipAllocator
	ChartMode       ChartMode
	ChartFallback   ChartFallbackOptions
	Report          *RenderReport

	elementCounter int
	shapePatches   *[]shapePatch
	tablePatches   *[]tablePatch
	imageBindings  *[]imageBinding
	mediaCounter   *int
}

type ElementRenderer interface {
	Supports(element Element) bool
	Render(ctx context.Context, slide *SlideContext, element Element) error
}

type TextRenderer struct{}
type ShapeRenderer struct{}
type TableRenderer struct{}
type ImageRenderer struct{}
type ChartRenderer struct{}

func renderElement(ctx context.Context, slide *SlideContext, element Element) error {
	renderers := []ElementRenderer{
		TextRenderer{},
		ShapeRenderer{},
		TableRenderer{},
		ImageRenderer{},
		ChartRenderer{},
	}
	for _, renderer := range renderers {
		if renderer.Supports(element) {
			return renderer.Render(ctx, slide, element)
		}
	}
	return fmt.Errorf("unsupported element type %q", element.Type)
}

func (TextRenderer) Supports(element Element) bool {
	return element.Type == "text"
}

func (TextRenderer) Render(_ context.Context, slide *SlideContext, element Element) error {
	addText(slide, element)
	return nil
}

func (ShapeRenderer) Supports(element Element) bool {
	return element.Type == "shape"
}

func (ShapeRenderer) Render(_ context.Context, slide *SlideContext, element Element) error {
	addShape(slide, element)
	return nil
}

func (TableRenderer) Supports(element Element) bool {
	return element.Type == "table"
}

func (TableRenderer) Render(_ context.Context, slide *SlideContext, element Element) error {
	addTable(slide, element)
	return nil
}

func (ImageRenderer) Supports(element Element) bool {
	return element.Type == "image"
}

func (ImageRenderer) Render(ctx context.Context, slide *SlideContext, element Element) error {
	return addImage(ctx, slide, element)
}

func (ChartRenderer) Supports(element Element) bool {
	return element.Type == "chart"
}

func (ChartRenderer) Render(_ context.Context, slide *SlideContext, element Element) error {
	switch slide.ChartMode {
	case ChartModeUnsupportedError:
		return fmt.Errorf("native chart unsupported for chart %q", element.Title)
	case ChartModeNativeOOXML:
		return fmt.Errorf("native OOXML chart rendering is not implemented")
	case ChartModeShapeFallback:
		addChartFallback(slide, element)
		slide.Report.Warnings = append(slide.Report.Warnings, "native chart downgraded to shape fallback")
		return nil
	default:
		return fmt.Errorf("unsupported chart mode %q", slide.ChartMode)
	}
}

func addBackground(slide *SlideContext, background string) {
	if background == "" {
		background = slide.Theme.Colors["background"]
	}
	element := Element{
		Type:     "shape",
		Geometry: pptx.PresetRect,
		BBox:     BBox{Width: slide.SlideSize.Width, Height: slide.SlideSize.Height},
		Fill:     background,
		Line:     LineStyle{Color: background, Width: 0},
	}
	addShapeWithName(slide, element, slide.nextShapeName("background"))
}

func addText(slide *SlideContext, element Element) {
	tb := slide.Slide.AddTextBox()
	tb.SetName(slide.nextShapeName("text"))
	tb.SetPosition(slide.px(element.BBox.Left), slide.px(element.BBox.Top))
	tb.SetSize(slide.px(element.BBox.Width), slide.px(element.BBox.Height))
	applyText(slide, tb.TextFrame(), element.Text, element.Style, false)
}

func addShape(slide *SlideContext, element Element) string {
	return addShapeWithName(slide, element, slide.nextShapeName("shape"))
}

func addShapeWithName(slide *SlideContext, element Element, name string) string {
	geometry := element.Geometry
	if geometry == "" {
		geometry = pptx.PresetRect
	}
	shape := pptx.NewAutoShape(geometry)
	shape.SetName(name)
	shape.SetPosition(slide.px(element.BBox.Left), slide.px(element.BBox.Top))
	shape.SetSize(slide.px(element.BBox.Width), slide.px(element.BBox.Height))
	if element.Fill != "" {
		shape.SetFill(dml.NewSolidFill(color(element.Fill)))
	} else {
		shape.SetFill(dml.NewNoFill())
	}
	if element.Line.Color != "" || element.Line.Width > 0 {
		shape.SetLine(dml.Line{
			Width: element.Line.Width,
			Color: color(defaultString(element.Line.Color, element.Fill)),
			Dash:  dash(element.Line.Style),
		})
	}
	if element.Text != "" {
		applyText(slide, shape.TextFrame(), element.Text, element.Style, true)
	}
	slide.Slide.AddShape(shape)
	*slide.shapePatches = append(*slide.shapePatches, shapePatch{
		SlideIndex: slide.SlideIndex,
		ShapeName:  name,
		Fill:       element.Fill,
		Line:       element.Line,
	})
	return name
}

func addTable(slide *SlideContext, element Element) {
	rows := len(element.Values)
	cols := 0
	if rows > 0 {
		cols = len(element.Values[0])
	}
	table := slide.Slide.AddTable(rows, cols)
	name := slide.nextShapeName("table")
	table.SetName(name)
	table.SetPosition(slide.px(element.BBox.Left), slide.px(element.BBox.Top))
	table.SetSize(slide.px(element.BBox.Width), slide.px(element.BBox.Height))

	headerRows := element.TableStyle.HeaderRows
	if headerRows == 0 {
		headerRows = 1
	}
	table.SetFirstRow(headerRows > 0)
	table.SetBandedRows(true)

	columnWidths := element.TableStyle.ColumnWidths
	for _, col := range element.Columns {
		columnWidths = append(columnWidths, col.Width)
	}
	for colIndex, width := range columnWidths {
		table.SetColWidth(colIndex, slide.px(width))
	}

	border := tableBorder(element)
	if border != nil {
		*slide.tablePatches = append(*slide.tablePatches, tablePatch{
			SlideIndex: slide.SlideIndex,
			ShapeName:  name,
			Border:     *border,
		})
	}

	rowHeight := slide.px(element.BBox.Height / math.Max(1, float64(rows)))
	for rowIndex := 0; rowIndex < rows; rowIndex++ {
		if row := table.Row(rowIndex); row != nil {
			row.SetHeight(rowHeight)
		}
		for colIndex := 0; colIndex < cols; colIndex++ {
			cell := table.Cell(rowIndex, colIndex)
			if cell == nil {
				continue
			}
			cellText := ""
			if colIndex < len(element.Values[rowIndex]) {
				cellText = element.Values[rowIndex][colIndex]
			}
			cell.SetText(cellText)
			cell.SetVerticalAlign(enum.VerticalAlignMiddle)
			if border != nil {
				cell.SetBorders(&pptx.TableBorder{
					Width: slide.px(defaultFloat(border.Width, 1)),
					Color: color(border.Color),
					Style: pptx.BorderStyleSingle,
				})
			}
			style := element.Style
			if rowIndex < headerRows {
				cell.SetFill(color(defaultString(element.HeaderFill, slide.Theme.Colors["text"])))
				style.Color = defaultString(element.HeaderText, "#FFFFFF")
				style.Bold = true
				style.FontSize = defaultFloat(style.FontSize, 16)
			} else {
				if rowIndex%2 == 0 {
					cell.SetFill(color("#F8FAFC"))
				}
				style.FontSize = defaultFloat(style.FontSize, 16)
			}
			applyText(slide, cell.TextFrame(), cellText, style, false)
		}
	}
}

func addImage(ctx context.Context, slide *SlideContext, element Element) error {
	asset, err := resolveAsset(ctx, slide, element.AssetID)
	if err != nil {
		return err
	}
	name := slide.nextShapeName("image")
	pic := pptx.NewPicture()
	pic.SetName(name)
	pic.SetImageData(asset.Bytes, asset.ContentType)
	pic.SetDescription(defaultString(element.Alt, asset.AltText))
	pic.SetPosition(slide.px(element.BBox.Left), slide.px(element.BBox.Top))
	pic.SetSize(slide.px(element.BBox.Width), slide.px(element.BBox.Height))
	slide.Slide.AddShape(pic)

	(*slide.mediaCounter)++
	relationship := slide.RelationshipIDs.Next(slide.SlideIndex)
	mediaName := fmt.Sprintf("ppt/media/image%d.%s", *slide.mediaCounter, asset.Ext)
	*slide.imageBindings = append(*slide.imageBindings, imageBinding{
		SlideIndex:   slide.SlideIndex,
		ShapeName:    name,
		Data:         asset.Bytes,
		ContentType:  asset.ContentType,
		Extension:    asset.Ext,
		Relationship: relationship,
		MediaName:    mediaName,
	})
	return nil
}

func addChartFallback(slide *SlideContext, element Element) {
	addText(slide, Element{
		Type: "text",
		Text: element.Title,
		BBox: BBox{Left: element.BBox.Left, Top: element.BBox.Top - 34, Width: element.BBox.Width, Height: 30},
		Style: TextStyle{
			FontSize: 19,
			Bold:     true,
			Color:    slide.Theme.Colors["text"],
			Font:     slide.Theme.Fonts["title"],
			Align:    "left",
		},
	})
	addText(slide, Element{
		Type: "text",
		Text: "shape fallback",
		BBox: BBox{Left: element.BBox.Left + element.BBox.Width - 132, Top: element.BBox.Top - 32, Width: 132, Height: 24},
		Style: TextStyle{
			FontSize: 13,
			Color:    slide.Theme.Colors["mutedText"],
			Font:     slide.Theme.Fonts["body"],
			Align:    "right",
		},
	})

	maxValue := 1.0
	for _, series := range element.Series {
		for _, value := range series.Values {
			if value > maxValue {
				maxValue = value
			}
		}
	}
	left := element.BBox.Left + slide.ChartFallback.PlotPadding.Left
	top := element.BBox.Top + slide.ChartFallback.PlotPadding.Top
	barAreaWidth := element.BBox.Width - slide.ChartFallback.PlotPadding.Left - slide.ChartFallback.PlotPadding.Right
	for i, category := range element.Categories {
		y := top + float64(i)*slide.ChartFallback.BarGap
		addText(slide, Element{
			Type:  "text",
			Text:  category,
			BBox:  BBox{Left: element.BBox.Left, Top: y - 4, Width: slide.ChartFallback.PlotPadding.Left - 18, Height: 30},
			Style: TextStyle{FontSize: 15, Color: slide.Theme.Colors["mutedText"], Font: slide.Theme.Fonts["body"], Align: "right"},
		})
		for j, series := range element.Series {
			if i >= len(series.Values) {
				continue
			}
			width := barAreaWidth * series.Values[i] / maxValue
			addShape(slide, Element{
				Type:     "shape",
				Geometry: pptx.PresetRect,
				BBox: BBox{
					Left:   left,
					Top:    y + float64(j)*slide.ChartFallback.SeriesGap,
					Width:  width,
					Height: slide.ChartFallback.BarHeight,
				},
				Fill: series.Color,
				Line: LineStyle{Color: series.Color, Width: 0},
			})
			addText(slide, Element{
				Type: "text",
				Text: formatValue(series.Values[i]),
				BBox: BBox{
					Left:   left + width + 8,
					Top:    y + float64(j)*slide.ChartFallback.SeriesGap - 3,
					Width:  42,
					Height: 22,
				},
				Style: TextStyle{
					FontSize: 13,
					Bold:     true,
					Color:    slide.Theme.Colors["mutedText"],
					Font:     slide.Theme.Fonts["body"],
					Align:    "left",
				},
			})
		}
	}
	for j, series := range element.Series {
		x := left + float64(j)*180
		addShape(slide, Element{
			Type:     "shape",
			Geometry: pptx.PresetRect,
			BBox: BBox{
				Left:   x,
				Top:    element.BBox.Top + element.BBox.Height - 32,
				Width:  24,
				Height: 12,
			},
			Fill: series.Color,
			Line: LineStyle{Color: series.Color, Width: 0},
		})
		addText(slide, Element{
			Type:  "text",
			Text:  series.Name,
			BBox:  BBox{Left: x + 32, Top: element.BBox.Top + element.BBox.Height - 39, Width: 140, Height: 26},
			Style: TextStyle{FontSize: 14, Color: slide.Theme.Colors["mutedText"], Font: slide.Theme.Fonts["body"], Align: "left"},
		})
	}
}

func applyText(slide *SlideContext, tf *pptx.TextFrame, text string, style TextStyle, middle bool) {
	tf.SetText(text)
	if middle || style.VAlign == "middle" {
		tf.SetAnchor(enum.TextAnchorMiddle)
	}
	for _, paragraph := range tf.Paragraphs() {
		paragraph.SetAlignment(textAlign(style.Align))
		for _, run := range paragraph.Runs() {
			run.SetFont(defaultString(style.Font, slide.Theme.Fonts["body"]))
			run.SetFontSize(slide.pt(style.FontSize))
			run.SetBold(style.Bold)
			run.SetColor(color(defaultString(defaultString(style.Color, style.TextColor), slide.Theme.Colors["text"])))
		}
	}
}

func resolveAsset(ctx context.Context, slide *SlideContext, id string) (AssetData, error) {
	if id == "" {
		return AssetData{}, fmt.Errorf("image element has empty assetId")
	}
	if slide.AssetResolver != nil {
		return slide.AssetResolver.Resolve(ctx, id)
	}
	asset, ok := slide.Assets[id]
	if !ok {
		return AssetData{}, fmt.Errorf("unknown asset %q", id)
	}
	return NewFileAssetResolver("", []Asset{asset}).Resolve(ctx, id)
}

func (slide *SlideContext) nextShapeName(kind string) string {
	slide.elementCounter++
	kind = strings.Trim(kind, "-")
	if kind == "" {
		kind = "element"
	}
	return fmt.Sprintf("pptxrender-%s-%d-%d", kind, slide.SlideIndex, slide.elementCounter)
}

func (slide *SlideContext) px(value float64) dml.EMU {
	return dml.EMU(emuFromPx(value, slide.Units.PxPerInch))
}

func (slide *SlideContext) pt(value float64) float64 {
	if value == 0 {
		return 12
	}
	return value * slide.Units.FontPxToPt
}

func formatValue(value float64) string {
	if math.Abs(value-math.Round(value)) < 0.000001 {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.1f", value)
}

func color(hex string) dml.Color {
	rgb, err := dml.ParseRGB(strings.TrimPrefix(defaultString(hex, "#000000"), "#"))
	if err != nil {
		return dml.ColorBlack
	}
	return rgb.ToColor()
}

func dash(style string) dml.DashStyle {
	switch style {
	case "dashed":
		return dml.DashDash
	case "dotted":
		return dml.DashDot
	default:
		return dml.DashSolid
	}
}

func textAlign(align string) enum.TextAlign {
	switch align {
	case "center":
		return enum.TextAlignCenter
	case "right":
		return enum.TextAlignRight
	default:
		return enum.TextAlignLeft
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultFloat(value, fallback float64) float64 {
	if value == 0 {
		return fallback
	}
	return value
}

func assetMap(assets []Asset) map[string]Asset {
	byID := make(map[string]Asset, len(assets))
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	return byID
}

func tableBorder(element Element) *LineStyle {
	if element.TableStyle.Border != nil {
		return element.TableStyle.Border
	}
	if element.Border == "" {
		return nil
	}
	return &LineStyle{Color: element.Border, Width: 1}
}

func mergeTheme(theme Theme, defaults DefaultStyleOptions) Theme {
	colors := map[string]string{
		"background": defaults.Colors.Background,
		"text":       defaults.Colors.Text,
		"mutedText":  defaults.Colors.MutedText,
		"accent1":    defaults.Colors.Accent1,
		"accent2":    defaults.Colors.Accent2,
		"border":     defaults.Colors.Border,
	}
	for key, value := range theme.Colors {
		colors[key] = value
	}
	fonts := map[string]string{
		"title": defaults.Fonts.Title,
		"body":  defaults.Fonts.Body,
	}
	for key, value := range theme.Fonts {
		fonts[key] = value
	}
	return Theme{Colors: colors, Fonts: fonts}
}

func validateIR(ir PresentationIR, strict bool) error {
	if !strict {
		return nil
	}
	for slideIndex, slide := range ir.Slides {
		for elementIndex, element := range slide.Elements {
			if element.BBox.Width < 0 || element.BBox.Height < 0 {
				return fmt.Errorf("slide %d element %d has negative size", slideIndex+1, elementIndex+1)
			}
			if element.Type == "chart" {
				for _, series := range element.Series {
					if len(series.Values) != len(element.Categories) {
						return fmt.Errorf("slide %d chart series %q has %d values for %d categories", slideIndex+1, series.Name, len(series.Values), len(element.Categories))
					}
				}
			}
		}
	}
	return nil
}
