package pptxrender

import (
	"archive/zip"
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
)

func TestRendererUsesSlideSizeImagesTablesAndChartFallback(t *testing.T) {
	ir := PresentationIR{
		SlideSize: SlideSize{Width: 1024, Height: 768},
		Theme: Theme{
			Colors: map[string]string{
				"background": "#FFFFFF",
				"text":       "#172033",
				"mutedText":  "#64748B",
				"accent1":    "#2563EB",
				"accent2":    "#0F9F6E",
				"border":     "#CBD5E1",
			},
			Fonts: map[string]string{"title": "Aptos Display", "body": "Aptos"},
		},
		Slides: []SlideIR{
			{
				Name:       "Objects",
				Background: "#FFFFFF",
				Elements: []Element{
					{
						Type: "text",
						Text: "Go spine renderer",
						BBox: BBox{Left: 64, Top: 48, Width: 520, Height: 70},
						Style: TextStyle{
							FontSize: 34,
							Bold:     true,
							Color:    "#172033",
							Font:     "Aptos Display",
						},
					},
					{
						Type:       "table",
						BBox:       BBox{Left: 64, Top: 150, Width: 480, Height: 180},
						HeaderFill: "#172033",
						HeaderText: "#FFFFFF",
						Border:     "#CBD5E1",
						Columns:    []ColumnIR{{Width: 160}, {Width: 160}, {Width: 160}},
						Style:      TextStyle{FontSize: 15, Font: "Aptos", Color: "#172033"},
						Values: [][]string{
							{"Area", "Node", "OfficeCLI"},
							{"Text", "Pass", "Pass"},
							{"Image", "Pass", "Pass"},
						},
					},
					{Type: "image", AssetID: "logo", BBox: BBox{Left: 610, Top: 150, Width: 160, Height: 120}, Alt: "Logo"},
					{Type: "image", AssetID: "badge", BBox: BBox{Left: 800, Top: 150, Width: 160, Height: 120}, Alt: "Badge"},
					{
						Type:       "chart",
						ChartType:  "bar",
						Title:      "Renderer signal",
						BBox:       BBox{Left: 64, Top: 410, Width: 820, Height: 260},
						Categories: []string{"Text", "Shape", "Image"},
						Series: []SeriesIR{
							{Name: "OfficeCLI", Values: []float64{4, 3, 2}, Color: "#2563EB"},
						},
					},
				},
			},
		},
	}

	deck, report, err := NewRenderer(RenderOptions{
		Assets: NewMapAssetResolver(map[string]AssetData{
			"logo":  {Bytes: tinyPNG(), Ext: "png", ContentType: "image/png", AltText: "Logo"},
			"badge": {Bytes: tinyPNG(), Ext: "png", ContentType: "image/png", AltText: "Badge"},
		}),
	}).Render(context.Background(), ir)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if report.SlideCount != 1 {
		t.Fatalf("SlideCount = %d, want 1", report.SlideCount)
	}
	if !containsWarning(report.Warnings, "native chart downgraded to shape fallback") {
		t.Fatalf("Warnings = %#v", report.Warnings)
	}

	presentationXML := zipPart(t, deck, "ppt/presentation.xml")
	for _, want := range []string{`cx="9753600"`, `cy="7315200"`} {
		if !strings.Contains(presentationXML, want) {
			t.Fatalf("presentation.xml missing %s:\n%s", want, presentationXML)
		}
	}
	_ = zipPart(t, deck, "ppt/media/image1.png")
	_ = zipPart(t, deck, "ppt/media/image2.png")
	slideXML := zipPart(t, deck, "ppt/slides/slide1.xml")
	for _, want := range []string{"Go spine renderer", "<a:tbl", "shape fallback", `val="CBD5E1"`} {
		if !strings.Contains(slideXML, want) {
			t.Fatalf("slide1.xml missing %q:\n%s", want, slideXML)
		}
	}
	relsXML := zipPart(t, deck, "ppt/slides/_rels/slide1.xml.rels")
	imageRelIDs := regexp.MustCompile(`Id="(rId\d+)" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"`).FindAllStringSubmatch(relsXML, -1)
	if len(imageRelIDs) != 2 || imageRelIDs[0][1] == imageRelIDs[1][1] {
		t.Fatalf("image relationships should be unique:\n%s", relsXML)
	}
}

func containsWarning(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

func zipPart(t *testing.T, data []byte, name string) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return buf.String()
	}
	t.Fatalf("missing zip part %s", name)
	return ""
}

func tinyPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00,
		0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x01, 0x08, 0x04, 0x00, 0x00, 0x00, 0xb5,
		0x1c, 0x0c, 0x02, 0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41,
		0x54, 0x78, 0xda, 0x63, 0xfc, 0xff, 0x1f, 0x00, 0x03, 0x03,
		0x02, 0x00, 0xef, 0xb2, 0x17, 0xdb, 0x00, 0x00, 0x00, 0x00,
		0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
}
