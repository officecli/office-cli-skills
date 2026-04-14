package review

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/officecli/officecli/pkg/ooxmledit"
)

var (
	slideNumPattern        = regexp.MustCompile(`slide(\d+)\.xml$`)
	paragraphPattern       = regexp.MustCompile(`(?s)<a:p\b`)
	placeholderTextPattern = regexp.MustCompile(`(?i)(IMG_PLACEHOLDER_|click to add|placeholder)`) //nolint:lll
	fontFamilyPattern      = regexp.MustCompile(`typeface="([^"]+)"`)
)

func lintPPTX(_ string, deck []byte) (StructureReport, error) {
	contentXMLs, err := ooxmledit.ExtractContentXML(deck, ooxmledit.FileTypePPTX)
	if err != nil {
		return StructureReport{}, err
	}

	paths := make([]string, 0, len(contentXMLs))
	for path := range contentXMLs {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	issues := make([]Issue, 0)
	strengths := make([]string, 0, 4)
	hasEmptySlide := false
	hasDenseSlide := false
	hasBulletOverflow := false
	hasPlaceholderResidue := false

	for _, path := range paths {
		slideNumber := extractSlideNumber(path)
		xmlContent := contentXMLs[path]
		textRuns := ooxmledit.ExtractSlideTextRuns(xmlContent)
		nonEmptyRuns := make([]string, 0, len(textRuns))
		for _, run := range textRuns {
			trimmed := strings.TrimSpace(run)
			if trimmed != "" {
				nonEmptyRuns = append(nonEmptyRuns, trimmed)
			}
		}

		if len(nonEmptyRuns) == 0 {
			hasEmptySlide = true
			issues = append(issues, Issue{
				Severity:     "high",
				Code:         "EMPTY_SLIDE",
				Title:        "Empty slide detected",
				Message:      fmt.Sprintf("Slide %d has no visible text content and may look unfinished to the audience.", slideNumber),
				SlideNumbers: []int{slideNumber},
				Suggestion:   "Add the core message for this slide, or remove the empty slide.",
			})
			continue
		}

		totalChars := 0
		for _, run := range nonEmptyRuns {
			totalChars += utf8.RuneCountInString(run)
		}
		if totalChars > 240 {
			hasDenseSlide = true
			issues = append(issues, Issue{
				Severity:     "medium",
				Code:         "TEXT_DENSITY_HIGH",
				Title:        "Text density is too high",
				Message:      fmt.Sprintf("Slide %d contains a large amount of text, currently about %d characters, which increases reading load.", slideNumber, totalChars),
				SlideNumbers: []int{slideNumber},
				Suggestion:   "Shorten the sentences, split the content into two slides, or convert it into charts or metrics.",
			})
		}

		paragraphs := len(paragraphPattern.FindAllStringIndex(xmlContent, -1))
		if paragraphs > 8 || len(nonEmptyRuns) > 8 {
			hasBulletOverflow = true
			issues = append(issues, Issue{
				Severity:     "medium",
				Code:         "BULLET_OVERLOAD",
				Title:        "Too many bullet points",
				Message:      fmt.Sprintf("Slide %d contains too many bullet points (about %d paragraphs), which makes the main point harder to follow.", slideNumber, maxInt(paragraphs, len(nonEmptyRuns))),
				SlideNumbers: []int{slideNumber},
				Suggestion:   "Keep 3-5 main takeaways and move the rest into speaker notes or a follow-up slide.",
			})
		}

		if placeholderTextPattern.MatchString(xmlContent) {
			hasPlaceholderResidue = true
			issues = append(issues, Issue{
				Severity:     "high",
				Code:         "PLACEHOLDER_RESIDUE",
				Title:        "Template placeholder residue",
				Message:      fmt.Sprintf("Slide %d still contains placeholder or template residue, which suggests the content may be unfinished.", slideNumber),
				SlideNumbers: []int{slideNumber},
				Suggestion:   "Remove template copy, placeholders, and test assets so every element reflects final content.",
			})
		}
	}

	fonts, err := extractPPTXFontFamilies(deck)
	if err != nil {
		return StructureReport{}, err
	}
	if len(fonts) > 4 {
		issues = append(issues, Issue{
			Severity:   "medium",
			Code:       "FONT_INCONSISTENT",
			Title:      "Font system is inconsistent",
			Message:    fmt.Sprintf("The document uses %d font families, which may weaken visual consistency.", len(fonts)),
			Suggestion: "Limit the deck to one coordinated font system instead of mixing many fonts.",
		})
	} else {
		strengths = append(strengths, "Font usage is mostly consistent without obvious overuse.")
	}

	if !hasEmptySlide {
		strengths = append(strengths, "Every slide contains visible text content and there are no obvious blank slides.")
	}
	if !hasDenseSlide {
		strengths = append(strengths, "Text density stays within a readable range for presentation use.")
	}
	if !hasBulletOverflow {
		strengths = append(strengths, "Bullet usage stays restrained and the main points remain focused.")
	}
	if !hasPlaceholderResidue {
		strengths = append(strengths, "No obvious template placeholder residue was detected.")
	}

	score := 100
	for _, issue := range issues {
		score -= structurePenalty(issue.Code)
	}
	if score < 0 {
		score = 0
	}

	summary := fmt.Sprintf("Checked %d slides and found %d structural issues.", len(paths), len(issues))
	if len(issues) == 0 {
		summary = fmt.Sprintf("Checked %d slides and found no obvious structural issues.", len(paths))
	}

	return StructureReport{
		Score:     score,
		Summary:   summary,
		Strengths: compactStrings(strengths, 4),
		Issues:    sortIssues(issues),
	}, nil
}

func extractSlideNumber(path string) int {
	base := filepath.Base(path)
	match := slideNumPattern.FindStringSubmatch(base)
	if len(match) != 2 {
		return 0
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return value
}

func extractPPTXFontFamilies(deck []byte) ([]string, error) {
	reader, err := zip.NewReader(bytes.NewReader(deck), int64(len(deck)))
	if err != nil {
		return nil, fmt.Errorf("failed to open pptx: %w", err)
	}
	families := map[string]struct{}{}
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "ppt/") {
			continue
		}
		if !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		raw, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", file.Name, err)
		}
		data, readErr := ioReadAll(raw)
		_ = raw.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", file.Name, readErr)
		}
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
	out := make([]string, 0, len(families))
	for family := range families {
		out = append(out, family)
	}
	sort.Strings(out)
	return out, nil
}

func structurePenalty(code string) int {
	switch code {
	case "EMPTY_SLIDE":
		return 25
	case "PLACEHOLDER_RESIDUE":
		return 22
	case "TEXT_DENSITY_HIGH":
		return 12
	case "BULLET_OVERLOAD":
		return 10
	case "FONT_INCONSISTENT":
		return 15
	default:
		return 8
	}
}

func sortIssues(items []Issue) []Issue {
	out := append([]Issue(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		if severityRank(out[i].Severity) != severityRank(out[j].Severity) {
			return severityRank(out[i].Severity) > severityRank(out[j].Severity)
		}
		left := 0
		if len(out[i].SlideNumbers) > 0 {
			left = out[i].SlideNumbers[0]
		}
		right := 0
		if len(out[j].SlideNumbers) > 0 {
			right = out[j].SlideNumbers[0]
		}
		if left != right {
			return left < right
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func severityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func compactStrings(items []string, limit int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func ioReadAll(reader io.Reader) ([]byte, error) {
	return io.ReadAll(reader)
}
