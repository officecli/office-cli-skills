package ooxmledit

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	docxParagraphRegex = regexp.MustCompile(`(?s)<w:p\b[^>]*>.*?</w:p>`)
	docxTextRegex      = regexp.MustCompile(`(?s)<w:t\b[^>]*>(.*?)</w:t>`)
	docxPPrRegex       = regexp.MustCompile(`(?s)(<w:pPr\b[^>]*>.*?</w:pPr>)`)
)

func ExtractDOCXParagraphTexts(documentXML string) []string {
	paragraphs := docxParagraphRegex.FindAllString(documentXML, -1)
	if len(paragraphs) == 0 {
		return nil
	}
	texts := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		matches := docxTextRegex.FindAllStringSubmatch(paragraph, -1)
		if len(matches) == 0 {
			continue
		}
		var parts []string
		for _, match := range matches {
			parts = append(parts, html.UnescapeString(match[1]))
		}
		text := strings.TrimSpace(strings.Join(parts, ""))
		if text != "" {
			texts = append(texts, text)
		}
	}
	return texts
}

func ReplaceDOCXParagraph(ooxmlBytes []byte, paragraphIndex int, newText string) ([]byte, error) {
	if paragraphIndex < 1 {
		return nil, fmt.Errorf("paragraphIndex must be >= 1, got %d", paragraphIndex)
	}
	contentXMLs, err := ExtractContentXML(ooxmlBytes, FileTypeDOCX)
	if err != nil {
		return nil, err
	}
	documentXML, ok := contentXMLs["word/document.xml"]
	if !ok {
		return nil, fmt.Errorf("word/document.xml not found")
	}
	paragraphs := docxParagraphRegex.FindAllString(documentXML, -1)
	if paragraphIndex > len(paragraphs) {
		return nil, fmt.Errorf("paragraph %d not found (only %d paragraphs)", paragraphIndex, len(paragraphs))
	}
	target := paragraphs[paragraphIndex-1]
	pPr := ""
	if match := docxPPrRegex.FindStringSubmatch(target); len(match) == 2 {
		pPr = match[1]
	}
	replaced := buildDOCXParagraphXML(newText, pPr)
	documentXML = strings.Replace(documentXML, target, replaced, 1)
	return ReplaceContentXML(ooxmlBytes, map[string]string{"word/document.xml": documentXML})
}

func AppendDOCXParagraph(ooxmlBytes []byte, paragraphIndex int, newText string) ([]byte, error) {
	if paragraphIndex < 1 {
		return nil, fmt.Errorf("paragraphIndex must be >= 1, got %d", paragraphIndex)
	}
	contentXMLs, err := ExtractContentXML(ooxmlBytes, FileTypeDOCX)
	if err != nil {
		return nil, err
	}
	documentXML, ok := contentXMLs["word/document.xml"]
	if !ok {
		return nil, fmt.Errorf("word/document.xml not found")
	}
	paragraphs := docxParagraphRegex.FindAllString(documentXML, -1)
	if paragraphIndex > len(paragraphs) {
		return nil, fmt.Errorf("paragraph %d not found (only %d paragraphs)", paragraphIndex, len(paragraphs))
	}
	target := paragraphs[paragraphIndex-1]
	documentXML = strings.Replace(documentXML, target, target+buildDOCXParagraphXML(newText, ""), 1)
	return ReplaceContentXML(ooxmlBytes, map[string]string{"word/document.xml": documentXML})
}

func ReplaceDOCXTextByPreview(ooxmlBytes []byte, oldText, newText string) ([]byte, error) {
	contentXMLs, err := ExtractContentXML(ooxmlBytes, FileTypeDOCX)
	if err != nil {
		return nil, err
	}
	documentXML, ok := contentXMLs["word/document.xml"]
	if !ok {
		return nil, fmt.Errorf("word/document.xml not found")
	}
	escapedOld := escapeXML(strings.TrimSpace(oldText))
	if escapedOld == "" || !strings.Contains(documentXML, escapedOld) {
		return nil, fmt.Errorf("selected text preview not found")
	}
	documentXML = strings.Replace(documentXML, escapedOld, escapeXML(newText), 1)
	return ReplaceContentXML(ooxmlBytes, map[string]string{"word/document.xml": documentXML})
}

func buildDOCXParagraphXML(text, pPr string) string {
	if pPr != "" && !strings.HasSuffix(pPr, "\n") {
		pPr += "\n"
	}
	lines := strings.Split(text, "\n")
	var runs strings.Builder
	for i, line := range lines {
		if i > 0 {
			runs.WriteString(`<w:r><w:br/></w:r>`)
		}
		runs.WriteString(fmt.Sprintf(`<w:r><w:t xml:space="preserve">%s</w:t></w:r>`, escapeXML(line)))
	}
	return fmt.Sprintf(`<w:p>%s%s</w:p>`, pPr, runs.String())
}
