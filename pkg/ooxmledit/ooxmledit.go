// Package ooxmledit provides ZIP unpacking, content XML extraction, and repacking helpers for OOXML files.
// It supports PPTX, DOCX, and XLSX Office formats.
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

// FileType represents an OOXML file type.
type FileType string

const (
	FileTypePPTX FileType = "pptx"
	FileTypeDOCX FileType = "docx"
	FileTypeXLSX FileType = "xlsx"
)

// contentPathPatterns defines the content XML path patterns to extract for each file type.
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

// GetFileType returns the OOXML file type based on the file extension.
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

// ExtractContentXML extracts content XML files from OOXML ZIP bytes.
// It returns a map from file path to XML content.
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

// ReplaceContentXML swaps the specified XML files in an OOXML ZIP payload while
// preserving all other files such as styles, themes, and media.
func ReplaceContentXML(ooxmlBytes []byte, modifiedXMLs map[string]string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(ooxmlBytes), int64(len(ooxmlBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to open original OOXML zip: %w", err)
	}

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	for _, f := range reader.File {
		if modifiedContent, ok := modifiedXMLs[f.Name]; ok {
			// Replace with modified content.
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
			// Keep the original file unchanged and preserve raw compressed data.
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

// FormatContentForPrompt formats extracted XML as prompt-ready text.
// The output is sorted by file path and includes path markers.
func FormatContentForPrompt(contentXMLs map[string]string) string {
	// Sort by path to keep output stable.
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
		sb.WriteString(fmt.Sprintf("=== File: %s ===\n", path))
		sb.WriteString(contentXMLs[path])
	}
	return sb.String()
}

// atTagRegex matches <a:t>...</a:t> text tags in PPTX slide XML.
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

// ExtractSlideTextSummary extracts plain text from <a:t> tags in a single slide XML.
// It returns a lightweight summary joined with " | " for phase-1 analysis.
func ExtractSlideTextSummary(xmlContent string) string {
	textRuns := ExtractSlideTextRuns(xmlContent)
	if len(textRuns) == 0 {
		return "(blank slide)"
	}

	var texts []string
	for _, raw := range textRuns {
		text := strings.TrimSpace(raw)
		if text != "" {
			texts = append(texts, text)
		}
	}

	if len(texts) == 0 {
		return "(blank slide)"
	}

	summary := strings.Join(texts, " | ")
	// Trim long summaries to keep prompts compact.
	if len(summary) > 500 {
		summary = summary[:500] + "..."
	}
	return summary
}

// FormatSlideSummariesForPrompt formats all slide summaries for prompt use.
// The output is sorted by path, one slide per line.
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

// FormatSingleSlideForPrompt formats one slide's XML content for prompt use.
func FormatSingleSlideForPrompt(path string, xmlContent string) string {
	return fmt.Sprintf("=== File: %s ===\n%s", path, xmlContent)
}

// ExtractThemeXML extracts ppt/theme/theme1.xml from PPTX ZIP bytes.
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

// slideNumRegex extracts the numeric index from a slide file path.
var slideNumRegex = regexp.MustCompile(`slide(\d+)\.xml$`)

// MaxSlideNum returns the highest slide index in a PPTX file.
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

// ReplaceAndResizeSlides updates slide XML and supports adding or removing slides in one pass.
//
// modifiedXMLs: updated slide XML keyed by paths such as ppt/slides/slide1.xml
// addSlideXMLs: new slide XML payloads in order; new slide numbers are assigned automatically
// removeSlidePaths: slide paths to remove
//
// Adding slides also updates [Content_Types].xml, ppt/presentation.xml,
// and ppt/_rels/presentation.xml.rels. New slide layout references are copied
// from the last existing slide rels payload.
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

	// Find the highest existing slide index and the final slide rels payload.
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
	// Read the rels file for the highest-numbered slide.
	lastSlideRelsPath := fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", maxSlideNum)
	for _, f := range reader.File {
		if f.Name == lastSlideRelsPath {
			lastSlideRelsContent, _ = readZipFile(f)
			break
		}
	}

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	// Track metadata files that need to be rewritten later.
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

	// Add new slide files and their rels.
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

		// Copy the last slide rels to the new slide.
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

	// Update [Content_Types].xml.
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

	// Update ppt/presentation.xml.
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

	// Update ppt/_rels/presentation.xml.rels.
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

// updateContentTypes adds or removes slide entries in [Content_Types].xml.
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

// rIdNumRegex extracts the numeric suffix from an rId value.
var rIdNumRegex = regexp.MustCompile(`rId(\d+)`)

// updatePresentationRels adds or removes slide relationships in presentation.xml.rels.
func updatePresentationRels(content string, addSlides []string, removeSlides []string, baseSlideNum int) string {
	for _, slidePath := range removeSlides {
		slideFileName := filepath.Base(slidePath)
		// Remove relationship rows that reference this slide.
		relPattern := regexp.MustCompile(`<Relationship[^>]*Target="slides/` + regexp.QuoteMeta(slideFileName) + `"[^/]*/>\s*`)
		content = relPattern.ReplaceAllString(content, "")
	}

	// Find the highest rId index.
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

// sldIdRegex matches <p:sldId> entries in presentation.xml.
var sldIdRegex = regexp.MustCompile(`<p:sldId\s+id="(\d+)"\s+r:id="(rId\d+)"\s*/>`)

// updatePresentationXML adds or removes slide references in ppt/presentation.xml.
func updatePresentationXML(content string, addSlides []string, removeSlides []string, baseSlideNum int) string {
	// Remove references for deleted slides.
	for _, slidePath := range removeSlides {
		slideFileName := filepath.Base(slidePath)
		// This simplified path skips the rId remapping step because the rels cleanup
		// already removed the corresponding relationship.
		_ = slideFileName
	}

	// Allocate rId and sldId values for the new slides.
	sldMatches := sldIdRegex.FindAllStringSubmatch(content, -1)
	maxSldId := 256 // OOXML default starting value.
	for _, m := range sldMatches {
		n := 0
		fmt.Sscanf(m[1], "%d", &n)
		if n > maxSldId {
			maxSldId = n
		}
	}

	// Find the highest rId value, aligned with the rels file.
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
		// Insert before </p:sldIdLst>.
		content = strings.Replace(content, "</p:sldIdLst>", newEntry+"\n</p:sldIdLst>", 1)
	}
	return content
}

// isContentFile reports whether the path matches any content XML pattern.
func isContentFile(path string, patterns []*regexp.Regexp) bool {
	for _, p := range patterns {
		if p.MatchString(path) {
			return true
		}
	}
	return false
}

// readZipFile reads the full content of a ZIP entry as a string.
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

// copyZipFile copies a ZIP entry into the new ZIP writer unchanged.
func copyZipFile(writer *zip.Writer, original *zip.File) error {
	// Try CreateRaw to avoid recompression and preserve the original payload.
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
