package imagewatermark

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font"
)

func TestApplyExtendsCanvasWithBottomFooterAndRestoresCornerFootmark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.png")
	sourceColor := color.RGBA{R: 40, G: 90, B: 160, A: 255}
	writeSolidPNG(t, path, 480, 270, sourceColor)

	if err := Apply(path, Options{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	img := readPNG(t, path)
	if img.Bounds().Dx() != 480 {
		t.Fatalf("width = %d, want 480", img.Bounds().Dx())
	}
	wantHeight := 270 + bottomWatermarkHeight(480, 270)
	if img.Bounds().Dy() != wantHeight {
		t.Fatalf("height = %d, want original height plus bottom watermark %d", img.Bounds().Dy(), wantHeight)
	}
	footerRect := image.Rect(0, 270, 480, wantHeight)
	if averageLuma(img, footerRect) < 230 {
		t.Fatal("bottom watermark footer is not white")
	}
	if darkPixelCount(img, image.Rect(12, 270, 260, wantHeight)) == 0 {
		t.Fatal("bottom watermark footer has no left-side text pixels")
	}
	if changedPixelCount(img, sourceColor, image.Rect(265, 218, 466, 258)) == 0 {
		t.Fatal("bottom-right corner has no footmark pixels")
	}
	if changedPixelCount(img, sourceColor, image.Rect(150, 75, 330, 190)) != 0 {
		t.Fatal("watermark changed center pixels; diagonal watermark should stay removed")
	}
}

func TestApplyRemovesQRCodeFromCornerFootmark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.png")
	sourceColor := color.RGBA{R: 40, G: 90, B: 160, A: 255}
	writeSolidPNG(t, path, 480, 270, sourceColor)

	if err := Apply(path, Options{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	img := readPNG(t, path)
	oldCornerQRRect := image.Rect(408, 198, 468, 258)
	if changedPixelCount(img, sourceColor, oldCornerQRRect) > 1800 {
		t.Fatal("corner footmark still contains a QR-code-sized changed block")
	}
}

func TestApplyPlacesQRCodeOnRightSideOfBottomFooter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generated.png")
	sourceColor := color.RGBA{R: 40, G: 90, B: 160, A: 255}
	writeSolidPNG(t, path, 640, 360, sourceColor)

	if err := Apply(path, Options{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	img := readPNG(t, path)
	footerHeight := bottomWatermarkHeight(640, 360)
	qrRect := expectedRightFooterQRCodeRect(640, 360, footerHeight)
	if darkPixelCount(img, qrRect) < 200 {
		t.Fatal("footer QR area has too few dark pixels")
	}
	if !hasMixedLightAndDarkPixels(img, qrRect) {
		t.Fatal("footer QR area does not contain a scannable high-contrast pattern")
	}
	if darkPixelCount(img, image.Rect(qrRect.Max.X+8, 360, 640-12, 360+footerHeight)) == 0 {
		t.Fatal("footer right-side text is not visible to the right of the QR code")
	}
}

func expectedRightFooterQRCodeRect(width, originalHeight, footerHeight int) image.Rectangle {
	outerPad := clamp(width/36, 12, 28)
	qrSize := clamp(footerHeight-outerPad, 36, 72)
	if qrSize > width/6 {
		qrSize = width / 6
	}
	if qrSize < 32 {
		qrSize = 32
	}

	mainFace := loadFace(float64(clamp(width/96, 8, 12)))
	defer mainFace.Close()
	subFace := loadFace(float64(clamp(width/192, 4, 6)))
	defer subFace.Close()
	rightTitleFace := loadFace(float64(clamp(width/128, 6, 10)))
	defer rightTitleFace.Close()
	rightMetaFace := loadFace(float64(clamp(width/192, 4, 7)))
	defer rightMetaFace.Close()

	leftX := outerPad
	rightGap := clamp(width/70, 10, 18)
	dividerGap := clamp(width/100, 8, 14)
	dividerWidth := 1
	leftTextWidth := maxInt(font.MeasureString(mainFace, defaultFootmarkText).Ceil(), font.MeasureString(subFace, footerPoweredByText).Ceil())
	rightTextWidth := maxInt(font.MeasureString(rightTitleFace, footerRightTitleText).Ceil(), font.MeasureString(rightMetaFace, footerRightMetaText).Ceil())
	rightGroupWidth := qrSize + dividerGap + dividerWidth + dividerGap + rightTextWidth
	rightGroupX := width - outerPad - rightGroupWidth
	minRightGroupX := leftX + leftTextWidth + rightGap
	if rightGroupX < minRightGroupX {
		rightGroupX = minRightGroupX
	}
	qrY := originalHeight + (footerHeight-qrSize)/2
	return image.Rect(rightGroupX, qrY, rightGroupX+qrSize, qrY+qrSize)
}

func TestApplyUsesWhiteBottomFooterOnLightImages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "light.png")
	sourceColor := color.RGBA{R: 238, G: 241, B: 246, A: 255}
	writeSolidPNG(t, path, 480, 270, sourceColor)

	if err := Apply(path, Options{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	img := readPNG(t, path)
	footerRect := image.Rect(0, 270, 480, img.Bounds().Dy())
	if averageLuma(img, footerRect) < 230 {
		t.Fatal("light image footer is not white")
	}
	if darkPixelCount(img, footerRect) == 0 {
		t.Fatal("light image footer has no dark watermark text or QR pixels")
	}
}

func TestApplyUsesWhiteBottomFooterOnDarkImages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dark.png")
	sourceColor := color.RGBA{R: 32, G: 38, B: 48, A: 255}
	writeSolidPNG(t, path, 480, 270, sourceColor)

	if err := Apply(path, Options{}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	img := readPNG(t, path)
	footerRect := image.Rect(0, 270, 480, img.Bounds().Dy())
	if averageLuma(img, footerRect) < 230 {
		t.Fatal("dark image footer is not white")
	}
	if darkPixelCount(img, footerRect) == 0 {
		t.Fatal("dark image footer has no dark watermark text or QR pixels")
	}
}

func TestApplyFailureDoesNotModifyOriginalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.png")
	original := []byte("not an image")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Apply(path, Options{}); err == nil {
		t.Fatal("Apply succeeded, want decode failure")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("file changed after failure: got %q want %q", got, original)
	}
}

func rectHasChangedPixel(img image.Image, sourceColor color.RGBA, rect image.Rectangle) bool {
	rect = rect.Intersect(img.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if got != sourceColor {
				return true
			}
		}
	}
	return false
}

func changedPixelDirectionCounts(img image.Image, sourceColor color.RGBA, rect image.Rectangle) (darker int, lighter int) {
	rect = rect.Intersect(img.Bounds())
	sourceLuma := luma(sourceColor)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if got == sourceColor {
				continue
			}
			if luma(got) < sourceLuma {
				darker++
			} else {
				lighter++
			}
		}
	}
	return darker, lighter
}

func hasMixedLightAndDarkPixels(img image.Image, rect image.Rectangle) bool {
	rect = rect.Intersect(img.Bounds())
	var light, dark int
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if luma(got) > 210 {
				light++
			}
			if luma(got) < 80 {
				dark++
			}
		}
	}
	return light > 200 && dark > 200
}

func darkPixelCount(img image.Image, rect image.Rectangle) int {
	rect = rect.Intersect(img.Bounds())
	count := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if luma(got) < 120 {
				count++
			}
		}
	}
	return count
}

func luma(c color.RGBA) float64 {
	return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
}

func changedPixelCount(img image.Image, sourceColor color.RGBA, rect image.Rectangle) int {
	rect = rect.Intersect(img.Bounds())
	count := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			got := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if got != sourceColor {
				count++
			}
		}
	}
	return count
}

func writeSolidPNG(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func readPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}
