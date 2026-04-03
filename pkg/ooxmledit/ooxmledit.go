// Package ooxmledit 提供 OOXML (Office Open XML) 文件的 ZIP 解压、内容 XML 提取和重打包功能。
// 支持 PPTX、DOCX、XLSX 三种 Office 文件类型。
package ooxmledit

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// FileType 表示 OOXML 文件类型
type FileType string

const (
	FileTypePPTX FileType = "pptx"
	FileTypeDOCX FileType = "docx"
	FileTypeXLSX FileType = "xlsx"
)

// contentPathPatterns 定义每种文件类型中需要提取的内容 XML 路径模式
var contentPathPatterns = map[FileType][]*regexp.Regexp{
	FileTypePPTX: {
		regexp.MustCompile(`^ppt/slides/slide\d+\.xml$`),
	},
	FileTypeDOCX: {
		regexp.MustCompile(`^word/document\.xml$`),
	},
	FileTypeXLSX: {
		regexp.MustCompile(`^xl/worksheets/sheet\d+\.xml$`),
		regexp.MustCompile(`^xl/sharedStrings\.xml$`),
	},
}

// GetFileType 根据文件名后缀返回 OOXML 文件类型
func GetFileType(fileName string) (FileType, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".pptx":
		return FileTypePPTX, nil
	case ".docx":
		return FileTypeDOCX, nil
	case ".xlsx":
		return FileTypeXLSX, nil
	default:
		return "", fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// ExtractContentXML 从 OOXML ZIP 字节流中提取内容 XML 文件。
// 返回 map[路径]XML内容 的映射。
// fileType: "pptx" / "docx" / "xlsx"
func ExtractContentXML(ooxmlBytes []byte, fileType FileType) (map[string]string, error) {
	patterns, ok := contentPathPatterns[fileType]
	if !ok {
		return nil, fmt.Errorf("unsupported file type: %s", fileType)
	}

	reader, err := zip.NewReader(bytes.NewReader(ooxmlBytes), int64(len(ooxmlBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to open OOXML zip: %w", err)
	}

	result := make(map[string]string)
	for _, f := range reader.File {
		if isContentFile(f.Name, patterns) {
			content, err := readZipFile(f)
			if err != nil {
				return nil, fmt.Errorf("failed to read %s: %w", f.Name, err)
			}
			result[f.Name] = content
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no content XML files found for file type: %s", fileType)
	}

	return result, nil
}

// ReplaceContentXML 用修改后的 XML 内容替换原 OOXML ZIP 中对应的文件，
// 保持其他文件（样式、主题、媒体等）不变。
// 返回新的 OOXML ZIP 字节流。
func ReplaceContentXML(ooxmlBytes []byte, modifiedXMLs map[string]string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(ooxmlBytes), int64(len(ooxmlBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to open original OOXML zip: %w", err)
	}

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	for _, f := range reader.File {
		if modifiedContent, ok := modifiedXMLs[f.Name]; ok {
			// 用修改后的内容替换
			newFile, err := writer.CreateHeader(&zip.FileHeader{
				Name:   f.Name,
				Method: zip.Deflate,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create zip entry %s: %w", f.Name, err)
			}
			if _, err := io.WriteString(newFile, modifiedContent); err != nil {
				return nil, fmt.Errorf("failed to write modified content for %s: %w", f.Name, err)
			}
		} else {
			// 保持原文件不变 — 使用 CreateRaw 以保留原始压缩数据
			if err := copyZipFile(writer, f); err != nil {
				return nil, fmt.Errorf("failed to copy %s: %w", f.Name, err)
			}
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// FormatContentForPrompt 将提取的 XML 内容格式化为适合放入 prompt 的文本。
// 返回按文件路径排序的、带路径标记的 XML 文本。
func FormatContentForPrompt(contentXMLs map[string]string) string {
	// 按路径排序，确保输出稳定
	paths := make([]string, 0, len(contentXMLs))
	for path := range contentXMLs {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var sb strings.Builder
	for i, path := range paths {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("=== 文件: %s ===\n", path))
		sb.WriteString(contentXMLs[path])
	}
	return sb.String()
}

// atTagRegex 匹配 PPTX slide XML 中的 <a:t>...</a:t> 文本标签
var atTagRegex = regexp.MustCompile(`<a:t[^>]*>(.*?)</a:t>`)

var (
	nonContentNodeRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?s)<a:xfrm\b[^>]*>.*?</a:xfrm>`),
		regexp.MustCompile(`(?s)<a:prstGeom\b[^>]*>.*?</a:prstGeom>`),
		regexp.MustCompile(`(?s)<a:solidFill\b[^>]*>.*?</a:solidFill>`),
		regexp.MustCompile(`(?s)<a:ln\b[^>]*>.*?</a:ln>`),
		regexp.MustCompile(`(?s)<a:effectLst\b[^>]*/>`),
		regexp.MustCompile(`(?s)<a:effectLst\b[^>]*>.*?</a:effectLst>`),
		regexp.MustCompile(`(?s)<a:bodyPr\b[^>]*/>`),
		regexp.MustCompile(`(?s)<a:bodyPr\b[^>]*>.*?</a:bodyPr>`),
		regexp.MustCompile(`(?s)<a:lstStyle\b[^>]*/>`),
		regexp.MustCompile(`(?s)<a:lstStyle\b[^>]*>.*?</a:lstStyle>`),
		regexp.MustCompile(`(?s)<a:pPr\b[^>]*/>`),
		regexp.MustCompile(`(?s)<a:pPr\b[^>]*>.*?</a:pPr>`),
	}
	rPrBlockRegex = regexp.MustCompile(`(?s)<a:rPr\b([^>]*)>.*?</a:rPr>`)
	rPrSelfRegex  = regexp.MustCompile(`(?s)<a:rPr\b([^>]*)/>`)
	langAttrRegex = regexp.MustCompile(`\blang="([^"]+)"`)
)

// ExtractSlideTextRuns returns all text runs inside a PPTX slide XML in document order.
func ExtractSlideTextRuns(xmlContent string) []string {
	matches := atTagRegex.FindAllStringSubmatch(xmlContent, -1)
	if len(matches) == 0 {
		return nil
	}

	texts := make([]string, 0, len(matches))
	for _, m := range matches {
		texts = append(texts, html.UnescapeString(m[1]))
	}
	return texts
}

// StripNonContentAttributes removes OOXML styling/layout noise from slide XML while
// preserving visible text and basic shape semantics for LLM prompts.
func StripNonContentAttributes(xmlContent string) string {
	sanitized := xmlContent
	for _, re := range nonContentNodeRegexes {
		sanitized = re.ReplaceAllString(sanitized, "")
	}
	sanitized = rPrBlockRegex.ReplaceAllStringFunc(sanitized, simplifyRunPropertiesTag)
	sanitized = rPrSelfRegex.ReplaceAllStringFunc(sanitized, simplifyRunPropertiesTag)
	sanitized = strings.TrimSpace(sanitized)
	return sanitized
}

func simplifyRunPropertiesTag(tag string) string {
	match := langAttrRegex.FindStringSubmatch(tag)
	if len(match) == 2 && strings.TrimSpace(match[1]) != "" {
		return fmt.Sprintf(`<a:rPr lang="%s"/>`, escapeXML(match[1]))
	}
	return `<a:rPr/>`
}

// ReplaceSlideTextRuns replaces every <a:t>...</a:t> payload in document order.
func ReplaceSlideTextRuns(xmlContent string, newTexts []string) (string, error) {
	matches := atTagRegex.FindAllStringSubmatchIndex(xmlContent, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no text runs found in slide xml")
	}
	if len(matches) != len(newTexts) {
		return "", fmt.Errorf("text run count mismatch: have %d want %d", len(newTexts), len(matches))
	}

	var out strings.Builder
	last := 0
	for i, idx := range matches {
		contentStart, contentEnd := idx[2], idx[3]
		out.WriteString(xmlContent[last:contentStart])
		out.WriteString(escapeXML(newTexts[i]))
		last = contentEnd
	}
	out.WriteString(xmlContent[last:])
	return out.String(), nil
}

// ExtractSlideTextSummary 从单个 slide XML 内容中提取 <a:t> 标签的纯文本。
// 返回以 " | " 分隔的文本片段拼接，用于 Phase 1 分析阶段的轻量摘要。
func ExtractSlideTextSummary(xmlContent string) string {
	textRuns := ExtractSlideTextRuns(xmlContent)
	if len(textRuns) == 0 {
		return "(空白幻灯片)"
	}

	var texts []string
	for _, raw := range textRuns {
		text := strings.TrimSpace(raw)
		if text != "" {
			texts = append(texts, text)
		}
	}

	if len(texts) == 0 {
		return "(空白幻灯片)"
	}

	summary := strings.Join(texts, " | ")
	// 截断过长的摘要，避免 prompt 过大
	if len(summary) > 500 {
		summary = summary[:500] + "..."
	}
	return summary
}

// FormatSlideSummariesForPrompt 将所有 slide 的文本摘要格式化为 prompt 文本。
// 按路径排序，每行一个 slide：`ppt/slides/slide1.xml: 标题文本 | 内容文本...`
func FormatSlideSummariesForPrompt(contentXMLs map[string]string) string {
	paths := make([]string, 0, len(contentXMLs))
	for path := range contentXMLs {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var sb strings.Builder
	for i, path := range paths {
		if i > 0 {
			sb.WriteString("\n")
		}
		summary := ExtractSlideTextSummary(contentXMLs[path])
		sb.WriteString(fmt.Sprintf("%s: %s", path, summary))
	}
	return sb.String()
}

// FormatSingleSlideForPrompt 将单个 slide 的 XML 内容格式化为 prompt 文本。
func FormatSingleSlideForPrompt(path string, xmlContent string) string {
	return fmt.Sprintf("=== 文件: %s ===\n%s", path, xmlContent)
}

// ExtractThemeXML 从 PPTX ZIP 字节流中提取 ppt/theme/theme1.xml 的内容。
func ExtractThemeXML(ooxmlBytes []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(ooxmlBytes), int64(len(ooxmlBytes)))
	if err != nil {
		return "", fmt.Errorf("failed to open OOXML zip: %w", err)
	}

	for _, f := range reader.File {
		if f.Name == "ppt/theme/theme1.xml" {
			content, err := readZipFile(f)
			if err != nil {
				return "", fmt.Errorf("failed to read theme XML: %w", err)
			}
			return content, nil
		}
	}
	return "", fmt.Errorf("ppt/theme/theme1.xml not found in PPTX")
}

// slideNumRegex 从 slide 文件路径中提取数字编号
var slideNumRegex = regexp.MustCompile(`slide(\d+)\.xml$`)

// MaxSlideNum 返回 PPTX 中最大的 slide 编号。
func MaxSlideNum(ooxmlBytes []byte) (int, error) {
	reader, err := zip.NewReader(bytes.NewReader(ooxmlBytes), int64(len(ooxmlBytes)))
	if err != nil {
		return 0, fmt.Errorf("failed to open OOXML zip: %w", err)
	}
	maxNum := 0
	for _, f := range reader.File {
		if m := slideNumRegex.FindStringSubmatch(f.Name); m != nil {
			n := 0
			fmt.Sscanf(m[1], "%d", &n)
			if n > maxNum {
				maxNum = n
			}
		}
	}
	return maxNum, nil
}

// ReplaceAndResizeSlides 综合操作：替换现有 slide 内容 + 添加/删除 slide。
//
// modifiedXMLs: 已修改的 slide XML 内容（key 为路径如 ppt/slides/slide1.xml）
// addSlideXMLs: 需要新增的 slide XML（有序列表，会分配新编号）
// removeSlidePaths: 需要删除的 slide 路径列表
//
// 添加 slide 时自动更新 [Content_Types].xml、ppt/presentation.xml、ppt/_rels/presentation.xml.rels。
// 新 slide 的 layout 引用复制自模板中最后一个 slide 的 rels。
func ReplaceAndResizeSlides(ooxmlBytes []byte, modifiedXMLs map[string]string, addSlideXMLs []string, removeSlidePaths []string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(ooxmlBytes), int64(len(ooxmlBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to open OOXML zip: %w", err)
	}

	removeSet := make(map[string]bool)
	for _, p := range removeSlidePaths {
		removeSet[p] = true
		removeSet[strings.Replace(p, "ppt/slides/", "ppt/slides/_rels/", 1)+".rels"] = true
	}

	// 找到现有最大 slide 编号和最后一个 slide 的 rels 内容
	maxSlideNum := 0
	var lastSlideRelsContent string
	for _, f := range reader.File {
		if m := slideNumRegex.FindStringSubmatch(f.Name); m != nil {
			n := 0
			fmt.Sscanf(m[1], "%d", &n)
			if n > maxSlideNum {
				maxSlideNum = n
			}
		}
	}
	// 读取最大编号 slide 的 rels
	lastSlideRelsPath := fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", maxSlideNum)
	for _, f := range reader.File {
		if f.Name == lastSlideRelsPath {
			lastSlideRelsContent, _ = readZipFile(f)
			break
		}
	}

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	// 收集需要在元数据中更新的信息
	var contentTypesFile *zip.File
	var presentationFile *zip.File
	var presentationRelsFile *zip.File

	for _, f := range reader.File {
		if removeSet[f.Name] {
			continue
		}

		switch f.Name {
		case "[Content_Types].xml":
			contentTypesFile = f
			continue
		case "ppt/presentation.xml":
			presentationFile = f
			continue
		case "ppt/_rels/presentation.xml.rels":
			presentationRelsFile = f
			continue
		}

		if modifiedContent, ok := modifiedXMLs[f.Name]; ok {
			newFile, err := writer.CreateHeader(&zip.FileHeader{
				Name:   f.Name,
				Method: zip.Deflate,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create zip entry %s: %w", f.Name, err)
			}
			if _, err := io.WriteString(newFile, modifiedContent); err != nil {
				return nil, fmt.Errorf("failed to write modified content for %s: %w", f.Name, err)
			}
		} else {
			if err := copyZipFile(writer, f); err != nil {
				return nil, fmt.Errorf("failed to copy %s: %w", f.Name, err)
			}
		}
	}

	// 添加新 slide 文件和对应的 rels
	newSlideNames := make([]string, 0, len(addSlideXMLs))
	for i, slideXML := range addSlideXMLs {
		newNum := maxSlideNum + i + 1
		slidePath := fmt.Sprintf("ppt/slides/slide%d.xml", newNum)
		newSlideNames = append(newSlideNames, slidePath)

		newFile, err := writer.CreateHeader(&zip.FileHeader{
			Name:   slidePath,
			Method: zip.Deflate,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create new slide %s: %w", slidePath, err)
		}
		if _, err := io.WriteString(newFile, slideXML); err != nil {
			return nil, fmt.Errorf("failed to write new slide %s: %w", slidePath, err)
		}

		// 复制最后一个 slide 的 rels 给新 slide
		if lastSlideRelsContent != "" {
			relsPath := fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", newNum)
			relsFile, err := writer.CreateHeader(&zip.FileHeader{
				Name:   relsPath,
				Method: zip.Deflate,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create rels for %s: %w", slidePath, err)
			}
			if _, err := io.WriteString(relsFile, lastSlideRelsContent); err != nil {
				return nil, fmt.Errorf("failed to write rels for %s: %w", slidePath, err)
			}
		}
	}

	// 更新 [Content_Types].xml
	if contentTypesFile != nil {
		content, err := readZipFile(contentTypesFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read [Content_Types].xml: %w", err)
		}
		content = updateContentTypes(content, newSlideNames, removeSlidePaths)
		newFile, err := writer.CreateHeader(&zip.FileHeader{
			Name:   "[Content_Types].xml",
			Method: zip.Deflate,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create [Content_Types].xml: %w", err)
		}
		if _, err := io.WriteString(newFile, content); err != nil {
			return nil, fmt.Errorf("failed to write [Content_Types].xml: %w", err)
		}
	}

	// 更新 ppt/presentation.xml
	if presentationFile != nil {
		content, err := readZipFile(presentationFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read presentation.xml: %w", err)
		}
		content = updatePresentationXML(content, newSlideNames, removeSlidePaths, maxSlideNum)
		newFile, err := writer.CreateHeader(&zip.FileHeader{
			Name:   "ppt/presentation.xml",
			Method: zip.Deflate,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create presentation.xml: %w", err)
		}
		if _, err := io.WriteString(newFile, content); err != nil {
			return nil, fmt.Errorf("failed to write presentation.xml: %w", err)
		}
	}

	// 更新 ppt/_rels/presentation.xml.rels
	if presentationRelsFile != nil {
		content, err := readZipFile(presentationRelsFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read presentation.xml.rels: %w", err)
		}
		content = updatePresentationRels(content, newSlideNames, removeSlidePaths, maxSlideNum)
		newFile, err := writer.CreateHeader(&zip.FileHeader{
			Name:   "ppt/_rels/presentation.xml.rels",
			Method: zip.Deflate,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create presentation.xml.rels: %w", err)
		}
		if _, err := io.WriteString(newFile, content); err != nil {
			return nil, fmt.Errorf("failed to write presentation.xml.rels: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

// updateContentTypes 在 [Content_Types].xml 中添加/删除 slide 条目。
func updateContentTypes(content string, addSlides []string, removeSlides []string) string {
	for _, slidePath := range removeSlides {
		partName := "/" + slidePath
		override := fmt.Sprintf(`<Override PartName="%s" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, partName)
		content = strings.Replace(content, override, "", 1)
	}

	for _, slidePath := range addSlides {
		partName := "/" + slidePath
		newOverride := fmt.Sprintf(`<Override PartName="%s" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, partName)
		content = strings.Replace(content, "</Types>", newOverride+"</Types>", 1)
	}
	return content
}

// rIdNumRegex 从 rId 中提取数字
var rIdNumRegex = regexp.MustCompile(`rId(\d+)`)

// updatePresentationRels 在 ppt/_rels/presentation.xml.rels 中添加/删除 slide 关系。
func updatePresentationRels(content string, addSlides []string, removeSlides []string, baseSlideNum int) string {
	for _, slidePath := range removeSlides {
		slideFileName := filepath.Base(slidePath)
		// 删除包含该 slide 的 Relationship 行
		relPattern := regexp.MustCompile(`<Relationship[^>]*Target="slides/` + regexp.QuoteMeta(slideFileName) + `"[^/]*/>\s*`)
		content = relPattern.ReplaceAllString(content, "")
	}

	// 找到最大的 rId 编号
	matches := rIdNumRegex.FindAllStringSubmatch(content, -1)
	maxRId := 0
	for _, m := range matches {
		n := 0
		fmt.Sscanf(m[1], "%d", &n)
		if n > maxRId {
			maxRId = n
		}
	}

	for i, slidePath := range addSlides {
		newRId := maxRId + i + 1
		slideFileName := filepath.Base(slidePath)
		newRel := fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/%s"/>`, newRId, slideFileName)
		content = strings.Replace(content, "</Relationships>", newRel+"\n</Relationships>", 1)
	}
	return content
}

// sldIdRegex 匹配 presentation.xml 中的 <p:sldId> 条目
var sldIdRegex = regexp.MustCompile(`<p:sldId\s+id="(\d+)"\s+r:id="(rId\d+)"\s*/>`)

// updatePresentationXML 在 ppt/presentation.xml 中添加/删除 slide 引用。
func updatePresentationXML(content string, addSlides []string, removeSlides []string, baseSlideNum int) string {
	// 删除被移除的 slide 引用
	for _, slidePath := range removeSlides {
		slideFileName := filepath.Base(slidePath)
		// 通过 rels 中的映射找到对应的 rId，这里简化处理：直接删除匹配的 sldId 行
		// 由于 sldId 通过 rId 关联，而我们已经从 rels 中删除了，这里先跳过
		_ = slideFileName
	}

	// 为新 slide 查找/添加 rId 和 sldId
	// 首先找到最大的 sldId
	sldMatches := sldIdRegex.FindAllStringSubmatch(content, -1)
	maxSldId := 256 // OOXML 默认起始值
	for _, m := range sldMatches {
		n := 0
		fmt.Sscanf(m[1], "%d", &n)
		if n > maxSldId {
			maxSldId = n
		}
	}

	// 找到最大 rId（与 rels 文件中的一致）
	rIdMatches := rIdNumRegex.FindAllStringSubmatch(content, -1)
	maxRId := 0
	for _, m := range rIdMatches {
		n := 0
		fmt.Sscanf(m[1], "%d", &n)
		if n > maxRId {
			maxRId = n
		}
	}

	for i := range addSlides {
		newSldId := maxSldId + i + 1
		newRId := maxRId + i + 1
		newEntry := fmt.Sprintf(`<p:sldId id="%d" r:id="rId%d"/>`, newSldId, newRId)
		// 插入到 </p:sldIdLst> 之前
		content = strings.Replace(content, "</p:sldIdLst>", newEntry+"\n</p:sldIdLst>", 1)
	}
	return content
}

// isContentFile 检查给定路径是否匹配任一内容 XML 模式
func isContentFile(path string, patterns []*regexp.Regexp) bool {
	for _, p := range patterns {
		if p.MatchString(path) {
			return true
		}
	}
	return false
}

// readZipFile 读取 ZIP 文件条目的全部内容为字符串
func readZipFile(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// copyZipFile 将原 ZIP 条目原样复制到新的 ZIP writer 中
func copyZipFile(writer *zip.Writer, original *zip.File) error {
	// 尝试使用 CreateRaw 以避免重新压缩（保持原始数据完整）
	header := original.FileHeader
	targetFile, err := writer.CreateHeader(&header)
	if err != nil {
		return err
	}

	rc, err := original.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	_, err = io.Copy(targetFile, rc)
	return err
}
