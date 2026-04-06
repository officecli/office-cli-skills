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
	placeholderTextPattern = regexp.MustCompile(`(?i)(IMG_PLACEHOLDER_|click to add|单击此处添加|点击此处添加|placeholder)`) //nolint:lll
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
				Title:        "存在空白页",
				Message:      fmt.Sprintf("第 %d 页没有可见文本内容，容易给受众留下未完成的印象。", slideNumber),
				SlideNumbers: []int{slideNumber},
				Suggestion:   "补充该页核心信息，或直接删除该空白页。",
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
				Title:        "单页文字密度偏高",
				Message:      fmt.Sprintf("第 %d 页文本量较大，当前约 %d 个字符，阅读负担偏重。", slideNumber, totalChars),
				SlideNumbers: []int{slideNumber},
				Suggestion:   "压缩句子长度，拆分成两页，或改成图表/指标表达。",
			})
		}

		paragraphs := len(paragraphPattern.FindAllStringIndex(xmlContent, -1))
		if paragraphs > 8 || len(nonEmptyRuns) > 8 {
			hasBulletOverflow = true
			issues = append(issues, Issue{
				Severity:     "medium",
				Code:         "BULLET_OVERLOAD",
				Title:        "项目符号过多",
				Message:      fmt.Sprintf("第 %d 页包含较多分点（约 %d 段），重点容易失焦。", slideNumber, maxInt(paragraphs, len(nonEmptyRuns))),
				SlideNumbers: []int{slideNumber},
				Suggestion:   "保留 3-5 个主结论，其余内容下沉到备注或下一页。",
			})
		}

		if placeholderTextPattern.MatchString(xmlContent) {
			hasPlaceholderResidue = true
			issues = append(issues, Issue{
				Severity:     "high",
				Code:         "PLACEHOLDER_RESIDUE",
				Title:        "疑似残留模板占位符",
				Message:      fmt.Sprintf("第 %d 页包含占位符或模板痕迹，说明内容可能未完全收口。", slideNumber),
				SlideNumbers: []int{slideNumber},
				Suggestion:   "清理模板文案、占位框和测试素材，确保所有元素都为最终内容。",
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
			Title:      "字体体系不够统一",
			Message:    fmt.Sprintf("当前文档检测到 %d 种字体族，视觉一致性可能受影响。", len(fonts)),
			Suggestion: "收敛到 1 组中英文字体组合，避免同一套 deck 出现过多字体。",
		})
	} else {
		strengths = append(strengths, "字体使用基本收敛，没有明显的字体滥用迹象")
	}

	if !hasEmptySlide {
		strengths = append(strengths, "所有页面都有明确文本内容，没有出现明显空白页")
	}
	if !hasDenseSlide {
		strengths = append(strengths, "页面文字密度整体可控，适合演示场景阅读")
	}
	if !hasBulletOverflow {
		strengths = append(strengths, "页面分点数量整体克制，重点相对集中")
	}
	if !hasPlaceholderResidue {
		strengths = append(strengths, "没有检测到明显的模板占位符残留")
	}

	score := 100
	for _, issue := range issues {
		score -= structurePenalty(issue.Code)
	}
	if score < 0 {
		score = 0
	}

	summary := fmt.Sprintf("共检查 %d 页，发现 %d 个结构问题。", len(paths), len(issues))
	if len(issues) == 0 {
		summary = fmt.Sprintf("共检查 %d 页，结构规则未发现明显问题。", len(paths))
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
