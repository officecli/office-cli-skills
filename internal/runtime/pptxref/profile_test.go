package pptxref

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli/pkg/officegen"
)

func TestBuildProfileDiscoversAllPPTXRecursivelyAndClassifiesSources(t *testing.T) {
	root := t.TempDir()
	writeDeck(t, filepath.Join(root, "brand.pptx"), "Brand Deck", "Zed Sans")
	writeDeck(t, filepath.Join(root, "output", "generated.pptx"), "Generated Deck", "Aptos")
	writeDeck(t, filepath.Join(root, ".worktrees", "feature", "fixture.pptx"), "Fixture Deck", "Aardvark Sans")
	writeDeck(t, filepath.Join(root, "tmp", "scratch.pptx"), "Scratch Deck", "Aardvark Sans")

	profile, err := BuildProfile(root, nil)
	if err != nil {
		t.Fatalf("BuildProfile: %v", err)
	}

	if profile.DiscoveredCount != 4 {
		t.Fatalf("DiscoveredCount = %d, want 4", profile.DiscoveredCount)
	}
	if profile.ParsedCount != 4 {
		t.Fatalf("ParsedCount = %d, want 4", profile.ParsedCount)
	}
	if profile.SourceBuckets["other"] != 1 ||
		profile.SourceBuckets["current-output"] != 1 ||
		profile.SourceBuckets["worktree"] != 1 ||
		profile.SourceBuckets["tmp"] != 1 {
		t.Fatalf("SourceBuckets = %#v", profile.SourceBuckets)
	}
	if profile.AggregateSlideCount != 4 {
		t.Fatalf("AggregateSlideCount = %d, want 4", profile.AggregateSlideCount)
	}
	if !containsString(profile.FontFamilies, "Aptos") || !containsString(profile.FontFamilies, "Zed Sans") || !containsString(profile.FontFamilies, "Aardvark Sans") {
		t.Fatalf("FontFamilies = %#v", profile.FontFamilies)
	}
	if indexOfString(profile.FontFamilies, "Zed Sans") > indexOfString(profile.FontFamilies, "Aardvark Sans") {
		t.Fatalf("stable root font should outrank low-signal buckets: %#v", profile.FontFamilies)
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

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
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
