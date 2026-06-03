package pptxref

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/officecli/officecli/pkg/ooxmledit"
)

const maxRepresentativeSlideSummaries = 12

var (
	fontFamilyPattern = regexp.MustCompile(`typeface="([^"]+)"`)
	themeColorPattern = regexp.MustCompile(`(?i)<a:srgbClr\s+val="([0-9a-f]{6})"`)
)

type ReferenceStyleProfile struct {
	Root                             string
	DiscoveredCount                  int
	ParsedCount                      int
	FailedCount                      int
	DuplicateCount                   int
	SourceFiles                      []ReferencePPTXFile
	AggregateSlideCount              int
	RepresentativeSlideTextSummaries []string
	ThemeXMLDigests                  []string
	ThemeColors                      []string
	FontFamilies                     []string
	LayoutSignals                    map[string]int
	SourceBuckets                    map[string]int
	DensitySignals                   StyleDensity
	ReuseGuidance                    []string
	Limitations                      []string
}

type ReferencePPTXFile struct {
	Path           string
	SourceBucket   string
	SlideCount     int
	ThemeXMLDigest string
	FontFamilies   []string
	FailedReason   string
}

type StyleDensity struct {
	AverageCharsPerSlide int
	MaxCharsPerSlide     int
}

type weightedSummary struct {
	text   string
	path   string
	index  int
	weight int
}

func BuildProfile(root string, explicit []string) (*ReferenceStyleProfile, error) {
	return BuildProfileWithOptions(root, explicit, true)
}

func BuildProfileWithOptions(root string, explicit []string, scanRoot bool) (*ReferenceStyleProfile, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve reference root: %w", err)
	}
	sources, err := discoverPPTXSources(absRoot, explicit, scanRoot)
	if err != nil {
		return nil, err
	}
	profile := &ReferenceStyleProfile{
		Root:            absRoot,
		DiscoveredCount: len(sources),
		LayoutSignals:   map[string]int{},
		SourceBuckets:   map[string]int{},
	}
	seenPaths := map[string]struct{}{}
	seenHashes := map[string]struct{}{}
	fonts := map[string]int{}
	themeColors := map[string]int{}
	themeDigests := map[string]int{}
	var representativeSummaries []weightedSummary
	totalChars := 0
	maxChars := 0

	for _, source := range sources {
		cleanPath, err := filepath.Abs(source)
		if err != nil {
			profile.addFailed(source, "resolve path: "+err.Error())
			continue
		}
		if _, ok := seenPaths[cleanPath]; ok {
			profile.DuplicateCount++
			continue
		}
		seenPaths[cleanPath] = struct{}{}

		file := ReferencePPTXFile{
			Path:         cleanPath,
			SourceBucket: classifySourceBucket(absRoot, cleanPath),
		}
		weight := bucketSignalWeight(file.SourceBucket)
		data, err := os.ReadFile(cleanPath)
		if err != nil {
			file.FailedReason = "read pptx: " + err.Error()
			profile.appendFile(file)
			continue
		}
		if len(data) == 0 {
			file.FailedReason = "pptx file is empty"
			profile.appendFile(file)
			continue
		}
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		if _, ok := seenHashes[hash]; ok {
			profile.DuplicateCount++
			continue
		}
		seenHashes[hash] = struct{}{}

		contentXMLs, err := ooxmledit.ExtractContentXML(data, ooxmledit.FileTypePPTX)
		if err != nil {
			file.FailedReason = "parse pptx content: " + err.Error()
			profile.appendFile(file)
			continue
		}
		file.SlideCount = len(contentXMLs)
		if themeXML, err := ooxmledit.ExtractThemeXML(data); err == nil {
			themeSum := sha256.Sum256([]byte(themeXML))
			file.ThemeXMLDigest = hex.EncodeToString(themeSum[:])
			themeDigests[file.ThemeXMLDigest] += weight
			for _, color := range extractThemeColors(themeXML) {
				themeColors[color] += weight
			}
		}
		for _, family := range extractPPTXFontFamilies(data) {
			fonts[family] += weight
			file.FontFamilies = append(file.FontFamilies, family)
		}
		sort.Strings(file.FontFamilies)

		summaries := ooxmledit.FormatSlideSummariesForPrompt(contentXMLs)
		summaryIndex := 0
		for _, line := range strings.Split(summaries, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			representativeSummaries = append(representativeSummaries, weightedSummary{
				text:   line,
				path:   cleanPath,
				index:  summaryIndex,
				weight: weight,
			})
			summaryIndex++
		}
		for _, xmlContent := range contentXMLs {
			chars := slideCharCount(xmlContent)
			totalChars += chars
			if chars > maxChars {
				maxChars = chars
			}
			for signal, count := range inferLayoutSignals(xmlContent) {
				profile.LayoutSignals[signal] += count * weight
			}
		}
		profile.ParsedCount++
		profile.AggregateSlideCount += file.SlideCount
		profile.appendFile(file)
	}

	if profile.AggregateSlideCount > 0 {
		profile.DensitySignals.AverageCharsPerSlide = totalChars / profile.AggregateSlideCount
	}
	profile.DensitySignals.MaxCharsPerSlide = maxChars
	profile.FontFamilies = sortedWeightedKeys(fonts)
	profile.ThemeColors = sortedWeightedKeys(themeColors)
	profile.ThemeXMLDigests = sortedWeightedKeys(themeDigests)
	profile.RepresentativeSlideTextSummaries = selectRepresentativeSummaries(representativeSummaries)
	profile.ReuseGuidance = buildReuseGuidance(profile)
	if profile.FailedCount > 0 {
		profile.Limitations = append(profile.Limitations, fmt.Sprintf("%d reference PPTX files could not be parsed and were skipped.", profile.FailedCount))
	}
	if profile.DuplicateCount > 0 {
		profile.Limitations = append(profile.Limitations, fmt.Sprintf("%d duplicate reference PPTX entries were ignored.", profile.DuplicateCount))
	}
	return profile, nil
}

func (p *ReferenceStyleProfile) appendFile(file ReferencePPTXFile) {
	p.SourceFiles = append(p.SourceFiles, file)
	if file.SourceBucket != "" {
		p.SourceBuckets[file.SourceBucket]++
	}
	if file.FailedReason != "" {
		p.FailedCount++
	}
}

func (p *ReferenceStyleProfile) addFailed(path, reason string) {
	p.appendFile(ReferencePPTXFile{
		Path:         path,
		SourceBucket: "other",
		FailedReason: reason,
	})
}

func discoverPPTXSources(root string, explicit []string, scanRoot bool) ([]string, error) {
	var sources []string
	if scanRoot {
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry == nil || entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, "~$") {
				return nil
			}
			if strings.EqualFold(filepath.Ext(name), ".pptx") {
				sources = append(sources, path)
			}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("walk reference root: %w", err)
		}
	}
	for _, item := range explicit {
		item = strings.TrimSpace(item)
		if item != "" {
			sources = append(sources, item)
		}
	}
	sort.Strings(sources)
	return sources, nil
}

func classifySourceBucket(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "other"
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, part := range parts {
		switch part {
		case ".worktrees", ".claude":
			return "worktree"
		case "tmp":
			return "tmp"
		case "testdata":
			return "testdata"
		case "output":
			return "current-output"
		case "public":
			if i+1 < len(parts) && parts[i+1] == "skills-demos" {
				return "demo-assets"
			}
		}
	}
	return "other"
}

func bucketSignalWeight(bucket string) int {
	switch bucket {
	case "other":
		return 4
	case "demo-assets":
		return 3
	case "current-output":
		return 2
	case "worktree", "tmp", "testdata":
		return 1
	default:
		return 1
	}
}

func extractPPTXFontFamilies(deck []byte) []string {
	reader, err := zip.NewReader(bytes.NewReader(deck), int64(len(deck)))
	if err != nil {
		return nil
	}
	families := map[string]struct{}{}
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "ppt/") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		for _, match := range fontFamilyPattern.FindAllStringSubmatch(string(data), -1) {
			if len(match) != 2 {
				continue
			}
			family := strings.TrimSpace(match[1])
			if family == "" {
				continue
			}
			lower := strings.ToLower(family)
			if strings.HasPrefix(lower, "+mj") || strings.HasPrefix(lower, "+mn") {
				continue
			}
			families[family] = struct{}{}
		}
	}
	return sortedKeys(families)
}

func extractThemeColors(themeXML string) []string {
	colors := map[string]struct{}{}
	for _, match := range themeColorPattern.FindAllStringSubmatch(themeXML, -1) {
		if len(match) == 2 {
			colors[strings.ToUpper(match[1])] = struct{}{}
		}
	}
	return sortedKeys(colors)
}

func inferLayoutSignals(xmlContent string) map[string]int {
	signals := map[string]int{}
	switch {
	case strings.Contains(xmlContent, "MetricBg") || strings.Contains(xmlContent, "MetricText"):
		signals["dashboard"]++
	case strings.Contains(xmlContent, "ChartPanel"):
		signals["chart"]++
	case strings.Contains(xmlContent, "SectionCard") || strings.Contains(xmlContent, "SectionBandCard") || strings.Contains(xmlContent, "SectionStaggerCard"):
		signals["sections"]++
	case strings.Contains(xmlContent, "<a:bu"):
		signals["bullets"]++
	default:
		signals["content"]++
	}
	return signals
}

func slideCharCount(xmlContent string) int {
	total := 0
	for _, run := range ooxmledit.ExtractSlideTextRuns(xmlContent) {
		total += utf8.RuneCountInString(strings.TrimSpace(run))
	}
	return total
}

func buildReuseGuidance(profile *ReferenceStyleProfile) []string {
	if profile == nil || profile.ParsedCount == 0 {
		return nil
	}
	guidance := []string{
		fmt.Sprintf("Use aggregate style signals from %d parsed reference PPTX files.", profile.ParsedCount),
	}
	if len(profile.FontFamilies) > 0 {
		guidance = append(guidance, "Prefer the recurring reference font families when renderer constraints allow: "+strings.Join(profile.FontFamilies, ", ")+".")
	}
	if len(profile.ThemeColors) > 0 {
		guidance = append(guidance, "Use recurring reference palette colors as intent, not as unrestricted low-level overrides.")
	}
	if len(profile.SourceBuckets) > 0 {
		guidance = append(guidance, "Prioritize stable current-directory references over worktree, tmp, testdata, and previous output buckets when style signals conflict; aggregate prompt signals are already ordered with this bucket weighting.")
	}
	return guidance
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedWeightedKeys(values map[string]int) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if values[left] != values[right] {
			return values[left] > values[right]
		}
		return left < right
	})
	return out
}

func selectRepresentativeSummaries(summaries []weightedSummary) []string {
	sort.SliceStable(summaries, func(i, j int) bool {
		left, right := summaries[i], summaries[j]
		if left.weight != right.weight {
			return left.weight > right.weight
		}
		if left.path != right.path {
			return left.path < right.path
		}
		return left.index < right.index
	})
	limit := maxRepresentativeSlideSummaries
	if len(summaries) < limit {
		limit = len(summaries)
	}
	out := make([]string, 0, limit)
	for _, summary := range summaries[:limit] {
		out = append(out, summary.text)
	}
	return out
}
