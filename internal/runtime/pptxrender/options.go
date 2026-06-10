package pptxrender

import "strconv"

type RenderOptions struct {
	Units            UnitOptions
	Defaults         DefaultStyleOptions
	Assets           AssetResolver
	RelationshipIDs  RelationshipAllocator
	ChartMode        ChartMode
	ChartFallback    ChartFallbackOptions
	EnableOOXMLPatch bool
	StrictValidation bool
}

type UnitOptions struct {
	PxPerInch  float64
	FontPxToPt float64
}

type DefaultStyleOptions struct {
	SlideSize SlideSize
	Fonts     FontDefaults
	Colors    ColorDefaults
}

type FontDefaults struct {
	Title string
	Body  string
}

type ColorDefaults struct {
	Background string
	Text       string
	MutedText  string
	Accent1    string
	Accent2    string
	Border     string
}

type RenderReport struct {
	SlideCount int
	Warnings   []string
}

type RelationshipAllocator interface {
	Next(slideIndex int) string
}

type sequentialRelationshipAllocator struct {
	nextBySlide map[int]int
}

func NewSequentialRelationshipAllocator() RelationshipAllocator {
	return &sequentialRelationshipAllocator{nextBySlide: make(map[int]int)}
}

func (a *sequentialRelationshipAllocator) Next(slideIndex int) string {
	if a.nextBySlide[slideIndex] == 0 {
		a.nextBySlide[slideIndex] = 1
	}
	a.nextBySlide[slideIndex]++
	return "rId" + strconv.Itoa(a.nextBySlide[slideIndex])
}

type ChartMode string

const (
	ChartModeShapeFallback    ChartMode = "shapeFallback"
	ChartModeUnsupportedError ChartMode = "unsupportedError"
	ChartModeNativeOOXML      ChartMode = "nativeOOXML"
)

type ChartFallbackOptions struct {
	Orientation string
	PlotPadding Padding
	BarHeight   float64
	BarGap      float64
	SeriesGap   float64
	LabelPolicy string
}

func (opts RenderOptions) withDefaults() RenderOptions {
	out := opts
	if out.Units.PxPerInch <= 0 {
		out.Units.PxPerInch = 96
	}
	if out.Units.FontPxToPt <= 0 {
		out.Units.FontPxToPt = 0.75
	}
	if out.Defaults.SlideSize.Width <= 0 {
		out.Defaults.SlideSize.Width = 1280
	}
	if out.Defaults.SlideSize.Height <= 0 {
		out.Defaults.SlideSize.Height = 720
	}
	if out.Defaults.Fonts.Title == "" {
		out.Defaults.Fonts.Title = "Poppins"
	}
	if out.Defaults.Fonts.Body == "" {
		out.Defaults.Fonts.Body = "Lato"
	}
	if out.Defaults.Colors.Background == "" {
		out.Defaults.Colors.Background = "#FFFFFF"
	}
	if out.Defaults.Colors.Text == "" {
		out.Defaults.Colors.Text = "#172033"
	}
	if out.Defaults.Colors.MutedText == "" {
		out.Defaults.Colors.MutedText = "#64748B"
	}
	if out.Defaults.Colors.Accent1 == "" {
		out.Defaults.Colors.Accent1 = "#2563EB"
	}
	if out.Defaults.Colors.Accent2 == "" {
		out.Defaults.Colors.Accent2 = "#0F9F6E"
	}
	if out.Defaults.Colors.Border == "" {
		out.Defaults.Colors.Border = "#CBD5E1"
	}
	if out.ChartMode == "" {
		out.ChartMode = ChartModeShapeFallback
	}
	if out.ChartFallback.Orientation == "" {
		out.ChartFallback.Orientation = "horizontal"
	}
	if out.ChartFallback.PlotPadding.Left == 0 {
		out.ChartFallback.PlotPadding.Left = 110
	}
	if out.ChartFallback.PlotPadding.Top == 0 {
		out.ChartFallback.PlotPadding.Top = 54
	}
	if out.ChartFallback.PlotPadding.Right == 0 {
		out.ChartFallback.PlotPadding.Right = 80
	}
	if out.ChartFallback.PlotPadding.Bottom == 0 {
		out.ChartFallback.PlotPadding.Bottom = 70
	}
	if out.ChartFallback.BarHeight == 0 {
		out.ChartFallback.BarHeight = 18
	}
	if out.ChartFallback.BarGap == 0 {
		out.ChartFallback.BarGap = 58
	}
	if out.ChartFallback.SeriesGap == 0 {
		out.ChartFallback.SeriesGap = 22
	}
	if out.ChartFallback.LabelPolicy == "" {
		out.ChartFallback.LabelPolicy = "value"
	}
	out.EnableOOXMLPatch = true
	return out
}

func (opts RenderOptions) slideSize(ir PresentationIR) SlideSize {
	if ir.SlideSize.Width > 0 && ir.SlideSize.Height > 0 {
		return ir.SlideSize
	}
	return opts.Defaults.SlideSize
}
