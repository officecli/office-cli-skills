package pptxrender

type PresentationIR struct {
	Version   int       `json:"version"`
	Theme     Theme     `json:"theme"`
	Assets    []Asset   `json:"assets"`
	Slides    []SlideIR `json:"slides"`
	SlideSize SlideSize `json:"slideSize"`
}

type IR = PresentationIR
type Size = SlideSize

type Theme struct {
	Colors map[string]string `json:"colors"`
	Fonts  map[string]string `json:"fonts"`
}

type SlideSize struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Asset struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
	Alt         string `json:"alt"`
}

type SlideIR struct {
	Name       string    `json:"name"`
	Background string    `json:"background"`
	Elements   []Element `json:"elements"`
}

type Element struct {
	Type       string     `json:"type"`
	Role       string     `json:"role"`
	Text       string     `json:"text"`
	Geometry   string     `json:"geometry"`
	BBox       BBox       `json:"bbox"`
	Style      TextStyle  `json:"style"`
	Fill       string     `json:"fill"`
	Line       LineStyle  `json:"line"`
	Columns    []ColumnIR `json:"columns"`
	HeaderFill string     `json:"headerFill"`
	HeaderText string     `json:"headerText"`
	Border     string     `json:"border"`
	TableStyle TableStyle `json:"tableStyle"`
	Values     [][]string `json:"values"`
	AssetID    string     `json:"assetId"`
	Fit        string     `json:"fit"`
	Alt        string     `json:"alt"`
	ChartType  string     `json:"chartType"`
	Title      string     `json:"title"`
	Categories []string   `json:"categories"`
	Series     []SeriesIR `json:"series"`
}

type BBox struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type TextStyle struct {
	FontSize  float64 `json:"fontSize"`
	Bold      bool    `json:"bold"`
	Color     string  `json:"color"`
	Font      string  `json:"font"`
	Align     string  `json:"align"`
	VAlign    string  `json:"valign"`
	TitleFont string  `json:"titleFont"`
	TextColor string  `json:"textColor"`
	GridColor string  `json:"gridColor"`
	AxisColor string  `json:"axisColor"`
}

type LineStyle struct {
	Color string  `json:"color"`
	Width float64 `json:"width"`
	Style string  `json:"style"`
}

type ColumnIR struct {
	Width float64 `json:"width"`
}

type SeriesIR struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
	Color  string    `json:"color"`
}

type Padding struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

type TableStyle struct {
	HeaderRows   int        `json:"headerRows"`
	BandedRows   bool       `json:"bandedRows"`
	Border       *LineStyle `json:"border"`
	CellPadding  Padding    `json:"cellPadding"`
	ColumnWidths []float64  `json:"columnWidths"`
}
