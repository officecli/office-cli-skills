package pptxref

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli/pkg/officegen"
)

func TestBuildProfileDiscoversStablePPTXRecursivelyAndSkipsGeneratedDirsByDefault(t *testing.T) {
	root := t.TempDir()
	writeDeck(t, filepath.Join(root, "brand.pptx"), "Brand Deck", "Zed Sans")
	writeDeck(t, filepath.Join(root, "references", "brand-nested.pptx"), "Nested Brand Deck", "Zed Sans")
	writeDeck(t, filepath.Join(root, "output", "generated.pptx"), "Generated Deck", "Aptos")
	writeDeck(t, filepath.Join(root, ".worktrees", "feature", "fixture.pptx"), "Fixture Deck", "Aardvark Sans")
	writeDeck(t, filepath.Join(root, "tmp", "scratch.pptx"), "Scratch Deck", "Aardvark Sans")
	writeDeck(t, filepath.Join(root, "testdata", "fixture.pptx"), "Test Fixture Deck", "Aardvark Sans")

	profile, err := BuildProfile(root, nil)
	if err != nil {
		t.Fatalf("BuildProfile: %v", err)
	}

	if profile.DiscoveredCount != 2 {
		t.Fatalf("DiscoveredCount = %d, want stable recursive references only", profile.DiscoveredCount)
	}
	if profile.ParsedCount != 2 {
		t.Fatalf("ParsedCount = %d, want stable recursive references only", profile.ParsedCount)
	}
	if profile.SourceBuckets["other"] != 2 || profile.SourceBuckets["current-output"] != 0 || profile.SourceBuckets["worktree"] != 0 || profile.SourceBuckets["tmp"] != 0 || profile.SourceBuckets["testdata"] != 0 {
		t.Fatalf("SourceBuckets = %#v, want only stable reference buckets", profile.SourceBuckets)
	}
	if profile.AggregateSlideCount != 2 {
		t.Fatalf("AggregateSlideCount = %d, want 2", profile.AggregateSlideCount)
	}
	if !containsString(profile.FontFamilies, "Zed Sans") {
		t.Fatalf("FontFamilies = %#v, want stable root font", profile.FontFamilies)
	}
	if containsString(profile.FontFamilies, "Aptos") || containsString(profile.FontFamilies, "Aardvark Sans") {
		t.Fatalf("FontFamilies = %#v, low-signal bucket fonts should not drive aggregate style signals when stable references exist", profile.FontFamilies)
	}
	if len(profile.RepresentativeSlideTextSummaries) == 0 {
		t.Fatal("expected representative slide summaries")
	}
	if !strings.Contains(profile.RepresentativeSlideTextSummaries[0], "Brand Deck") {
		t.Fatalf("first representative summary should come from stable root deck, got %#v", profile.RepresentativeSlideTextSummaries)
	}
	if profile.LayoutSignals["bullets"] == 0 && profile.LayoutSignals["content"] == 0 {
		t.Fatalf("expected weighted layout signals, got %#v", profile.LayoutSignals)
	}
	if len(profile.ThemeXMLDigests) == 0 {
		t.Fatal("expected theme XML digests")
	}
}

func TestBuildProfileExtractsSlideLevelPaletteColors(t *testing.T) {
	root := t.TempDir()
	deckPath := filepath.Join(root, "brand.pptx")
	writeDeck(t, deckPath, "Brand Deck", "Zed Sans")
	injectSlideColor(t, deckPath, "FF6A00")

	profile, err := BuildProfile(root, nil)
	if err != nil {
		t.Fatalf("BuildProfile: %v", err)
	}

	if !containsString(profile.ThemeColors, "FF6A00") {
		t.Fatalf("ThemeColors = %#v, want slide-level palette color FF6A00", profile.ThemeColors)
	}
}

func TestBuildProfileDedupesExplicitFilesAndIsolatesFailures(t *testing.T) {
	root := t.TempDir()
	deckPath := filepath.Join(root, "brand.pptx")
	writeDeck(t, deckPath, "Brand Deck", "Aptos")
	if err := os.WriteFile(filepath.Join(root, "broken.pptx"), []byte("not a zip"), 0o644); err != nil {
		t.Fatalf("write broken pptx: %v", err)
	}

	profile, err := BuildProfile(root, []string{deckPath})
	if err != nil {
		t.Fatalf("BuildProfile: %v", err)
	}

	if profile.DiscoveredCount != 3 {
		t.Fatalf("DiscoveredCount = %d, want discovered root files plus explicit duplicate", profile.DiscoveredCount)
	}
	if profile.ParsedCount != 1 {
		t.Fatalf("ParsedCount = %d, want 1", profile.ParsedCount)
	}
	if profile.DuplicateCount != 1 {
		t.Fatalf("DuplicateCount = %d, want 1", profile.DuplicateCount)
	}
	if profile.FailedCount != 1 {
		t.Fatalf("FailedCount = %d, want 1", profile.FailedCount)
	}
	if len(profile.SourceFiles) != 2 {
		t.Fatalf("SourceFiles = %d, want parsed plus failed", len(profile.SourceFiles))
	}
	var failedReason string
	for _, file := range profile.SourceFiles {
		if strings.HasSuffix(file.Path, "broken.pptx") {
			failedReason = file.FailedReason
		}
	}
	if failedReason == "" {
		t.Fatalf("expected failed reason for broken.pptx: %#v", profile.SourceFiles)
	}
}

func TestBuildProfileSkipsCurrentOutputReferencesByDefault(t *testing.T) {
	root := t.TempDir()
	writeDeck(t, filepath.Join(root, "output", "generated-a.pptx"), "Generated A", "Aptos")
	writeDeck(t, filepath.Join(root, "output", "generated-b.pptx"), "Generated B", "Aptos")

	profile, err := BuildProfile(root, nil)
	if err != nil {
		t.Fatalf("BuildProfile: %v", err)
	}

	if profile.DiscoveredCount != 0 {
		t.Fatalf("DiscoveredCount = %d, want generated output references skipped by default", profile.DiscoveredCount)
	}
	if profile.ParsedCount != 0 {
		t.Fatalf("ParsedCount = %d, want generated output references skipped by default", profile.ParsedCount)
	}
	if profile.SourceBuckets["current-output"] != 0 {
		t.Fatalf("SourceBuckets = %#v, want no current-output bucket from default scan", profile.SourceBuckets)
	}
}

func TestBuildProfileSkipsManyCurrentOutputsWhenStableRootExists(t *testing.T) {
	root := t.TempDir()
	writeDeck(t, filepath.Join(root, "brand.pptx"), "Brand Deck", "Zed Sans")
	for idx := 0; idx < 20; idx++ {
		writeDeck(t, filepath.Join(root, "output", fmt.Sprintf("generated-%02d.pptx", idx)), "Generated Deck", "Aptos")
	}

	profile, err := BuildProfile(root, nil)
	if err != nil {
		t.Fatalf("BuildProfile: %v", err)
	}

	if profile.DiscoveredCount != 1 || profile.ParsedCount != 1 {
		t.Fatalf("DiscoveredCount=%d ParsedCount=%d, want only stable root reference", profile.DiscoveredCount, profile.ParsedCount)
	}
	if profile.SourceBuckets["other"] != 1 || profile.SourceBuckets["current-output"] != 0 {
		t.Fatalf("SourceBuckets = %#v, want output decks skipped by default", profile.SourceBuckets)
	}
	if !containsString(profile.FontFamilies, "Zed Sans") || containsString(profile.FontFamilies, "Aptos") {
		t.Fatalf("FontFamilies = %#v, want stable root font without generated-output fonts", profile.FontFamilies)
	}
	if !strings.Contains(profile.RepresentativeSlideTextSummaries[0], "Brand Deck") {
		t.Fatalf("first representative summary should stay with stable root deck, got %#v", profile.RepresentativeSlideTextSummaries)
	}
	if containsSubstring(profile.ReuseGuidance, "stable reference decks drive aggregate style signals") {
		t.Fatalf("ReuseGuidance = %#v, should not mention skipped low-signal buckets", profile.ReuseGuidance)
	}
}

func TestBuildProfileAllowsExplicitCurrentOutputReferences(t *testing.T) {
	root := t.TempDir()
	generatedPath := filepath.Join(root, "output", "generated-a.pptx")
	writeDeck(t, generatedPath, "Generated A", "Aptos")

	profile, err := BuildProfileWithOptions(root, []string{generatedPath}, false)
	if err != nil {
		t.Fatalf("BuildProfileWithOptions: %v", err)
	}

	if profile.ParsedCount != 1 {
		t.Fatalf("ParsedCount = %d, want explicit current-output reference parsed", profile.ParsedCount)
	}
	if profile.SourceBuckets["current-output"] != 1 {
		t.Fatalf("SourceBuckets = %#v, want explicit current-output bucket", profile.SourceBuckets)
	}
}

func writeDeck(t *testing.T, path, title, font string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{{
		Title:   title,
		Content: "Reference deck content",
		Layout:  "content",
		Points:  []string{"Consistent title hierarchy", "Reusable card rhythm"},
	}}, officegen.PPTXOptions{
		Title:   title,
		Creator: "test",
		Theme: &officegen.SlideTheme{
			PrimaryColor:   "123456",
			AccentColor:    "ABCDEF",
			BackgroundType: "solid",
			BgColor1:       "FFFFFF",
			BgColor2:       "FFFFFF",
			TextColor:      "111111",
			TitleTextColor: "222222",
			FontFamily:     font,
			EAFontFamily:   "Noto Sans CJK SC",
		},
	})
	if err != nil {
		t.Fatalf("generate deck: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write deck: %v", err)
	}
}

func injectSlideColor(t *testing.T, path, color string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read deck: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open pptx: %v", err)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	replaced := false
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip member %s: %v", file.Name, err)
		}
		member, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read zip member %s: %v", file.Name, err)
		}
		if file.Name == "ppt/slides/slide1.xml" {
			marker := "</p:spTree>"
			shapeXML := `<p:sp><p:nvSpPr><p:cNvPr id="9001" name="Injected Palette Signal"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="1" cy="1"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="` + color + `"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p/></p:txBody></p:sp>`
			next := strings.Replace(string(member), marker, shapeXML+marker, 1)
			if next != string(member) {
				replaced = true
				member = []byte(next)
			}
		}
		header := file.FileHeader
		header.Method = zip.Deflate
		w, err := writer.CreateHeader(&header)
		if err != nil {
			t.Fatalf("create zip member %s: %v", file.Name, err)
		}
		if _, err := w.Write(member); err != nil {
			t.Fatalf("write zip member %s: %v", file.Name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if !replaced {
		t.Fatalf("did not inject slide color %s in %s", color, path)
	}
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatalf("write patched deck: %v", err)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func containsSubstring(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

func indexOfString(items []string, want string) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return len(items) + 1
}
