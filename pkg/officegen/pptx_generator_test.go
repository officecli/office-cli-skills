package officegen

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var samplePNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0xf0,
	0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99,
	0x3d, 0x1d, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
	0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func samplePNGWithSize(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	fill := color.RGBA{R: 0x44, G: 0x88, B: 0xcc, A: 0xff}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestPPTXWithChart(t *testing.T) {
	slides := []Slide{
		{
			Title:   "Chart Test",
			IsTitle: true,
			Layout:  "title",
		},
		{
			Title:  "Bar Chart Example",
			Layout: "chart",
			Points: []string{"Key finding 1", "Key finding 2"},
			Chart: &ChartData{
				Type:       "bar",
				Title:      "Quarterly Revenue",
				Categories: []string{"Q1", "Q2", "Q3", "Q4"},
				Values:     []float64{120, 180, 150, 210},
			},
		},
	}

	gen := NewPPTXGenerator()
	data, err := gen.Generate(slides, PPTXOptions{
		Title:   "Chart Test",
		Creator: "Test",
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Save to a temporary file for manual inspection when needed.
	tmpFile := "/tmp/test_chart.pptx"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatalf("Write file failed: %v", err)
	}

	t.Logf("Generated PPTX with chart: %s (%d bytes)", tmpFile, len(data))
}

func TestPPTXNoChart(t *testing.T) {
	slides := []Slide{
		{Title: "Title Slide", IsTitle: true, Layout: "title"},
		{Title: "Content Slide", Layout: "content", Points: []string{"Point 1", "Point 2"}},
	}
	gen := NewPPTXGenerator()
	data, err := gen.Generate(slides, PPTXOptions{Title: "No Chart Test", Creator: "Test"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	tmpFile := "/tmp/test_no_chart.pptx"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatalf("Write file failed: %v", err)
	}
	t.Logf("Generated PPTX without chart: %s (%d bytes)", tmpFile, len(data))
}

func TestPPTXWithEmbeddedImageAssets(t *testing.T) {
	slides := []Slide{
		{
			Title:     "Image Slide",
			Layout:    "content",
			Points:    []string{"Point one", "Point two"},
			HasImage:  true,
			ImagePos:  "right",
			ImageData: samplePNG,
		},
	}

	gen := NewPPTXGenerator()
	data, err := gen.Generate(slides, PPTXOptions{Title: "Image Test", Creator: "Test"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	files := openZipFiles(t, data)
	if _, ok := files["ppt/media/image1.png"]; !ok {
		t.Fatalf("missing embedded image media file")
	}
	if !strings.Contains(files["[Content_Types].xml"], `Extension="png"`) {
		t.Fatalf("content types must include png default when embedding images")
	}
	rels := files["ppt/slides/_rels/slide1.xml.rels"]
	if !strings.Contains(rels, `relationships/image`) || !strings.Contains(rels, `Target="../media/image1.png"`) {
		t.Fatalf("slide rels must include image relationship, got %s", rels)
	}
	slideXML := files["ppt/slides/slide1.xml"]
	if !strings.Contains(slideXML, "<p:pic>") || !strings.Contains(slideXML, `r:embed="rId2"`) {
		t.Fatalf("slide xml must contain picture element referencing rId2")
	}
}

func TestPPTXWithEmbeddedJPEGAssets(t *testing.T) {
	slides := []Slide{
		{
			Title:     "Image Slide",
			Layout:    "content",
			Points:    []string{"Point one"},
			HasImage:  true,
			ImagePos:  "right",
			ImageData: []byte("fake-jpeg"),
			ImageMIME: "image/jpeg",
		},
	}

	gen := NewPPTXGenerator()
	data, err := gen.Generate(slides, PPTXOptions{Title: "JPEG Image Test", Creator: "Test"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	files := openZipFiles(t, data)
	if _, ok := files["ppt/media/image1.jpeg"]; !ok {
		t.Fatalf("missing embedded jpeg media file")
	}
	rels := files["ppt/slides/_rels/slide1.xml.rels"]
	if !strings.Contains(rels, `Target="../media/image1.jpeg"`) {
		t.Fatalf("slide rels must include jpeg image relationship, got %s", rels)
	}
}

func TestPPTXGalleryEmbedsMultipleImages(t *testing.T) {
	slides := []Slide{
		{
			Title:   "Visual Gallery",
			Layout:  "gallery",
			Variant: "gallery",
			Visuals: []SlideVisual{
				{Label: "Hero", Caption: "Primary scene", ImageData: samplePNG, ImageMIME: "image/png"},
				{Label: "Detail", Caption: "Supporting scene", ImageData: samplePNG, ImageMIME: "image/png"},
			},
		},
	}

	gen := NewPPTXGenerator()
	data, err := gen.Generate(slides, PPTXOptions{Title: "Gallery Test", Creator: "Test"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	files := openZipFiles(t, data)
	for _, path := range []string{"ppt/media/image1.png", "ppt/media/image2.png"} {
		if _, ok := files[path]; !ok {
			t.Fatalf("missing gallery asset %s", path)
		}
	}
	rels := files["ppt/slides/_rels/slide1.xml.rels"]
	for _, needle := range []string{`Id="rId2"`, `Target="../media/image1.png"`, `Id="rId3"`, `Target="../media/image2.png"`} {
		if !strings.Contains(rels, needle) {
			t.Fatalf("gallery rels missing %q: %s", needle, rels)
		}
	}
	slideXML := files["ppt/slides/slide1.xml"]
	if count := strings.Count(slideXML, "<p:pic>"); count != 2 {
		t.Fatalf("picture count = %d, want 2: %s", count, slideXML)
	}
}

func TestPPTXImageLayoutsRenderPicture(t *testing.T) {
	cases := []struct {
		name      string
		slide     Slide
		contains  []string
		notExists []string
	}{
		{
			name: "title background",
			slide: Slide{
				Title:     "Cover",
				Layout:    "title",
				Subtitle:  "Illustrated cover",
				HasImage:  true,
				ImagePos:  "background",
				ImageData: samplePNG,
			},
			contains: []string{`<p:pic>`, `name="BackgroundImage"`, `name="ImageOverlay"`},
		},
		{
			name: "content left",
			slide: Slide{
				Title:     "Image Left Text Right",
				Layout:    "content",
				Content:   "Supporting copy",
				HasImage:  true,
				ImagePos:  "left",
				ImageData: samplePNG,
			},
			contains: []string{`<p:pic>`, `name="LeftImage"`},
		},
		{
			name: "content center",
			slide: Slide{
				Title:     "Centered Image",
				Layout:    "content",
				Points:    []string{"Notes"},
				HasImage:  true,
				ImagePos:  "center",
				ImageData: samplePNG,
			},
			contains: []string{`<p:pic>`, `name="CenterImage"`},
		},
		{
			name: "content top",
			slide: Slide{
				Title:     "Top Banner",
				Layout:    "content",
				Points:    []string{"Notes"},
				HasImage:  true,
				ImagePos:  "top",
				ImageData: samplePNG,
			},
			contains: []string{`<p:pic>`, `name="TopImage"`},
		},
		{
			name: "content bottom",
			slide: Slide{
				Title:     "Bottom Banner",
				Layout:    "content",
				Points:    []string{"Notes"},
				HasImage:  true,
				ImagePos:  "bottom",
				ImageData: samplePNG,
			},
			contains: []string{`<p:pic>`, `name="BottomImage"`},
		},
		{
			name: "content diagonal",
			slide: Slide{
				Title:     "Diagonal Image",
				Layout:    "content",
				Points:    []string{"Notes"},
				HasImage:  true,
				ImagePos:  "diagonal",
				ImageData: samplePNG,
			},
			contains: []string{`<p:pic>`, `name="DiagonalImage"`},
		},
		{
			name: "chart ignores image",
			slide: Slide{
				Title:     "Chart Slide",
				Layout:    "chart",
				HasImage:  true,
				ImagePos:  "right",
				ImageData: samplePNG,
				Chart: &ChartData{
					Type:       "bar",
					Title:      "Sales",
					Categories: []string{"Q1"},
					Values:     []float64{1},
				},
			},
			notExists: []string{`<p:pic>`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gen := NewPPTXGenerator()
			data, err := gen.Generate([]Slide{tc.slide}, PPTXOptions{Title: tc.name, Creator: "Test"})
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			files := openZipFiles(t, data)
			slideXML := files["ppt/slides/slide1.xml"]
			for _, want := range tc.contains {
				if !strings.Contains(slideXML, want) {
					t.Fatalf("slide xml missing %q: %s", want, slideXML)
				}
			}
			for _, unwanted := range tc.notExists {
				if strings.Contains(slideXML, unwanted) {
					t.Fatalf("slide xml unexpectedly contains %q: %s", unwanted, slideXML)
				}
			}
		})
	}
}

func TestPPTXImageLayoutsApplyCropForMismatchedAspectRatio(t *testing.T) {
	wideImage := samplePNGWithSize(t, 1600, 600)
	tallImage := samplePNGWithSize(t, 600, 1600)

	tests := []struct {
		name           string
		slide          Slide
		wantCropMarker string
	}{
		{
			name: "wide image in diagonal frame crops horizontally",
			slide: Slide{
				Title:     "Diagonal Image",
				Layout:    "content",
				Points:    []string{"Notes"},
				HasImage:  true,
				ImagePos:  "diagonal",
				ImageData: wideImage,
			},
			wantCropMarker: `a:srcRect l="`,
		},
		{
			name: "tall image in top banner crops vertically",
			slide: Slide{
				Title:     "Top Banner",
				Layout:    "content",
				Points:    []string{"Notes"},
				HasImage:  true,
				ImagePos:  "top",
				ImageData: tallImage,
			},
			wantCropMarker: `a:srcRect l="0" t="`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gen := NewPPTXGenerator()
			data, err := gen.Generate([]Slide{tc.slide}, PPTXOptions{Title: tc.name, Creator: "Test"})
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			files := openZipFiles(t, data)
			slideXML := files["ppt/slides/slide1.xml"]
			if !strings.Contains(slideXML, tc.wantCropMarker) {
				t.Fatalf("slide xml missing crop marker %q: %s", tc.wantCropMarker, slideXML)
			}
		})
	}
}

func TestTargetAspectRatioForSlide(t *testing.T) {
	ratio := TargetAspectRatioForSlide(Slide{
		Layout:    "content",
		HasImage:  true,
		ImagePos:  "diagonal",
		ImageData: samplePNG,
	})
	if ratio <= 1.5 || ratio >= 1.6 {
		t.Fatalf("ratio = %f, want diagonal image frame ratio", ratio)
	}
}

func TestPPTXChartMatchesLocalOfficeSDKCompatibleFormat(t *testing.T) {
	slides := []Slide{
		{Title: "Title Slide", IsTitle: true, Layout: "title"},
		{
			Title:  "Chart Slide",
			Layout: "chart",
			Chart: &ChartData{
				Type:       "bar",
				Title:      "Sales",
				Categories: []string{"Q1", "Q2", "Q3"},
				Values:     []float64{10, 20, 30},
			},
		},
	}

	gen := NewPPTXGenerator()
	data, err := gen.Generate(slides, PPTXOptions{Title: "Compatibility Test", Creator: "Test"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open pptx zip failed: %v", err)
	}

	files := map[string]string{}
	for _, f := range zr.File {
		switch f.Name {
		case "ppt/charts/chart1.xml", "ppt/charts/_rels/chart1.xml.rels",
			"ppt/slides/_rels/slide2.xml.rels", "ppt/slides/slide2.xml",
			"ppt/presentation.xml", "ppt/_rels/presentation.xml.rels":
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s failed: %v", f.Name, err)
			}
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(rc); err != nil {
				rc.Close()
				t.Fatalf("read %s failed: %v", f.Name, err)
			}
			rc.Close()
			files[f.Name] = buf.String()
		}
	}

	chartXML, ok := files["ppt/charts/chart1.xml"]
	if !ok {
		t.Fatalf("missing ppt/charts/chart1.xml")
	}
	chartRelsXML, ok := files["ppt/charts/_rels/chart1.xml.rels"]
	if !ok {
		t.Fatalf("missing ppt/charts/_rels/chart1.xml.rels")
	}
	slideRelsXML, ok := files["ppt/slides/_rels/slide2.xml.rels"]
	if !ok {
		t.Fatalf("missing ppt/slides/_rels/slide2.xml.rels")
	}
	slideXML, ok := files["ppt/slides/slide2.xml"]
	if !ok {
		t.Fatalf("missing ppt/slides/slide2.xml")
	}
	presentationXML, ok := files["ppt/presentation.xml"]
	if !ok {
		t.Fatalf("missing ppt/presentation.xml")
	}
	presentationRelsXML, ok := files["ppt/_rels/presentation.xml.rels"]
	if !ok {
		t.Fatalf("missing ppt/_rels/presentation.xml.rels")
	}

	expectedFiles := map[string]string{
		"ppt/charts/chart1.xml":            readFixtureFile(t, "local_preview_contract/ppt/charts/chart1.xml"),
		"ppt/charts/_rels/chart1.xml.rels": readFixtureFile(t, "local_preview_contract/ppt/charts/_rels/chart1.xml.rels"),
		"ppt/slides/_rels/slide2.xml.rels": readFixtureFile(t, "local_preview_contract/ppt/slides/_rels/slide2.xml.rels"),
		"ppt/presentation.xml":             readFixtureFile(t, "local_preview_contract/ppt/presentation.xml"),
		"ppt/_rels/presentation.xml.rels":  readFixtureFile(t, "local_preview_contract/ppt/_rels/presentation.xml.rels"),
	}
	for name, expected := range expectedFiles {
		if actual := files[name]; actual != expected {
			t.Fatalf("%s does not match fixture; if this change is intentional, update the contract fixture after re-verifying OfficeSDK preview compatibility", name)
		}
	}

	// The locally previewable sample includes externalData.
	if !strings.Contains(chartXML, `externalData`) {
		t.Fatalf("chart1.xml must contain externalData for local OfficeSDK compatibility")
	}

	// chart rels must keep oleObject + chartStyle + chartColorStyle
	if !strings.Contains(chartRelsXML, `oleObject`) {
		t.Fatalf("chart1.xml.rels must contain oleObject relationship")
	}
	if !strings.Contains(chartRelsXML, `Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/oleObject"`) {
		t.Fatalf("chart1.xml.rels missing rId1=oleObject")
	}
	if !strings.Contains(chartRelsXML, `Id="rId2" Type="http://schemas.microsoft.com/office/2011/relationships/chartStyle"`) {
		t.Fatalf("chart1.xml.rels missing rId2=chartStyle")
	}
	if !strings.Contains(chartRelsXML, `Id="rId3" Type="http://schemas.microsoft.com/office/2011/relationships/chartColorStyle"`) {
		t.Fatalf("chart1.xml.rels missing rId3=chartColorStyle")
	}

	// slide rels must have rId1=chart, rId2=slideLayout
	if !strings.Contains(slideRelsXML, `Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/chart"`) {
		t.Fatalf("slide2.xml.rels missing rId1=chart")
	}
	if !strings.Contains(slideRelsXML, `Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout"`) {
		t.Fatalf("slide2.xml.rels missing rId2=slideLayout")
	}

	// slide XML must reference chart via rId1 and keep inline namespaces
	if !strings.Contains(slideXML, `r:id="rId1"`) {
		t.Fatalf("slide2.xml must reference chart via r:id=\"rId1\"")
	}
	if !strings.Contains(slideXML, `xmlns:c="http://schemas.openxmlformats.org/drawingml/2006/chart"`) {
		t.Fatalf("slide2.xml must keep inline chart namespace declarations")
	}
	for _, needle := range []string{`name="ChartPanel"`, `name="FooterRule"`, `name="PageNumber"`} {
		if !strings.Contains(slideXML, needle) {
			t.Fatalf("slide2.xml missing %q", needle)
		}
	}

	// presentation-level package structure must match the local working sample
	if strings.Contains(presentationXML, `type="screen4x3"`) {
		t.Fatalf("presentation.xml must not contain slide size type=screen4x3")
	}
	if !strings.Contains(presentationXML, `<p:defaultTextStyle>`) {
		t.Fatalf("presentation.xml must contain defaultTextStyle")
	}
	if !strings.Contains(presentationRelsXML, `presProps`) || !strings.Contains(presentationRelsXML, `viewProps`) || !strings.Contains(presentationRelsXML, `tableStyles`) {
		t.Fatalf("presentation.xml.rels must contain presProps/viewProps/tableStyles relationships")
	}
}

func readFixtureFile(t *testing.T, relativePath string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", relativePath))
	if err != nil {
		t.Fatalf("read fixture %s failed: %v", relativePath, err)
	}
	return strings.TrimRight(string(data), "\n")
}

func openZipFiles(t *testing.T, data []byte) map[string]string {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open pptx zip failed: %v", err)
	}

	files := make(map[string]string, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s failed: %v", f.Name, err)
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(rc); err != nil {
			rc.Close()
			t.Fatalf("read %s failed: %v", f.Name, err)
		}
		rc.Close()
		files[f.Name] = buf.String()
	}
	return files
}
