package ooxmledit

import (
	"fmt"
	"regexp"
	"strings"
)

// CellUpdate represents a table cell update operation
type CellUpdate struct {
	Row   int
	Col   int
	Value string
}

// ReplaceSlideXML replaces a full slide XML payload at the given 1-based slide index.
func ReplaceSlideXML(ooxmlBytes []byte, slideIndex int, newSlideXML string) ([]byte, error) {
	if slideIndex < 1 {
		return nil, fmt.Errorf("slideIndex must be >= 1, got %d", slideIndex)
	}
	if strings.TrimSpace(newSlideXML) == "" {
		return nil, fmt.Errorf("new slide xml cannot be empty")
	}

	slidePath := fmt.Sprintf("ppt/slides/slide%d.xml", slideIndex)
	contentXMLs, err := ExtractContentXML(ooxmlBytes, FileTypePPTX)
	if err != nil {
		return nil, fmt.Errorf("extract content: %w", err)
	}
	if _, ok := contentXMLs[slidePath]; !ok {
		return nil, fmt.Errorf("slide %d not found", slideIndex)
	}

	contentXMLs[slidePath] = newSlideXML
	return ReplaceContentXML(ooxmlBytes, contentXMLs)
}

// ReplaceSlideTitle replaces the title text of a specific slide in a PPTX file.
// slideIndex is 1-based (slide1.xml = index 1).
func ReplaceSlideTitle(ooxmlBytes []byte, slideIndex int, newTitle string) ([]byte, error) {
	if slideIndex < 1 {
		return nil, fmt.Errorf("slideIndex must be >= 1, got %d", slideIndex)
	}

	slidePath := fmt.Sprintf("ppt/slides/slide%d.xml", slideIndex)

	// Extract all slide XMLs
	contentXMLs, err := ExtractContentXML(ooxmlBytes, FileTypePPTX)
	if err != nil {
		return nil, fmt.Errorf("extract content: %w", err)
	}

	slideXML, ok := contentXMLs[slidePath]
	if !ok {
		return nil, fmt.Errorf("slide %d not found", slideIndex)
	}

	// Find title placeholder using regex (namespace-agnostic)
	// Look for ph type="title" or type="ctrTitle" with any namespace prefix
	titlePlaceholderRe := regexp.MustCompile(`<[^:]+:ph[^>]*type="(title|ctrTitle)"[^>]*>`)
	if !titlePlaceholderRe.MatchString(slideXML) {
		// If no explicit title placeholder, just replace the first text element
		// This is a fallback for slides without proper placeholders
		textRe := regexp.MustCompile(`<[^:]+:t>[^<]*</[^:]+:t>`)
		matches := textRe.FindStringIndex(slideXML)
		if matches == nil {
			return nil, fmt.Errorf("no text found in slide %d", slideIndex)
		}
		slideXML = slideXML[:matches[0]] + fmt.Sprintf("<a:t>%s</a:t>", escapeXML(newTitle)) + slideXML[matches[1]:]
	} else {
		// Replace the first text element (assumed to be the title)
		textRe := regexp.MustCompile(`<[^:]+:t>[^<]*</[^:]+:t>`)
		matches := textRe.FindStringIndex(slideXML)
		if matches == nil {
			return nil, fmt.Errorf("no text found in slide %d", slideIndex)
		}
		slideXML = slideXML[:matches[0]] + fmt.Sprintf("<a:t>%s</a:t>", escapeXML(newTitle)) + slideXML[matches[1]:]
	}

	// Replace in content map
	contentXMLs[slidePath] = slideXML

	// Repack
	return ReplaceContentXML(ooxmlBytes, contentXMLs)
}

// AppendSlideBullets appends bullet items to the body of a specific slide.
func AppendSlideBullets(ooxmlBytes []byte, slideIndex int, newBullets []string) ([]byte, error) {
	if slideIndex < 1 {
		return nil, fmt.Errorf("slideIndex must be >= 1, got %d", slideIndex)
	}
	if len(newBullets) == 0 {
		return ooxmlBytes, nil // No-op
	}

	slidePath := fmt.Sprintf("ppt/slides/slide%d.xml", slideIndex)

	contentXMLs, err := ExtractContentXML(ooxmlBytes, FileTypePPTX)
	if err != nil {
		return nil, fmt.Errorf("extract content: %w", err)
	}

	slideXML, ok := contentXMLs[slidePath]
	if !ok {
		return nil, fmt.Errorf("slide %d not found", slideIndex)
	}

	// Find closing txBody tag (namespace-agnostic)
	txBodyEndRe := regexp.MustCompile(`</[^:]+:txBody>`)
	allMatches := txBodyEndRe.FindAllStringIndex(slideXML, -1)
	if len(allMatches) == 0 {
		return nil, fmt.Errorf("body placeholder not found in slide %d", slideIndex)
	}

	// Use the last </x:txBody> occurrence (body, not title)
	lastMatch := allMatches[len(allMatches)-1]
	insertPos := lastMatch[0]

	// Build new paragraphs
	var newParas strings.Builder
	for _, bullet := range newBullets {
		newParas.WriteString(fmt.Sprintf(`<a:p><a:r><a:t>%s</a:t></a:r></a:p>`, escapeXML(bullet)))
	}

	// Insert before last </x:txBody>
	slideXML = slideXML[:insertPos] + newParas.String() + slideXML[insertPos:]

	contentXMLs[slidePath] = slideXML
	return ReplaceContentXML(ooxmlBytes, contentXMLs)
}

// ReplaceBodyParagraph replaces a specific paragraph in the slide body.
// paragraphIndex is 1-based.
func ReplaceBodyParagraph(ooxmlBytes []byte, slideIndex int, paragraphIndex int, newText string) ([]byte, error) {
	if slideIndex < 1 {
		return nil, fmt.Errorf("slideIndex must be >= 1, got %d", slideIndex)
	}
	if paragraphIndex < 1 {
		return nil, fmt.Errorf("paragraphIndex must be >= 1, got %d", paragraphIndex)
	}

	slidePath := fmt.Sprintf("ppt/slides/slide%d.xml", slideIndex)

	contentXMLs, err := ExtractContentXML(ooxmlBytes, FileTypePPTX)
	if err != nil {
		return nil, fmt.Errorf("extract content: %w", err)
	}

	slideXML, ok := contentXMLs[slidePath]
	if !ok {
		return nil, fmt.Errorf("slide %d not found", slideIndex)
	}

	// Find all paragraph tags (namespace-agnostic, non-greedy with DOTALL)
	re := regexp.MustCompile(`(?s)<[^:]+:p[ >].*?</[^:]+:p>`)
	matches := re.FindAllStringIndex(slideXML, -1)
	if paragraphIndex > len(matches) {
		return nil, fmt.Errorf("paragraph %d not found in slide %d (only %d paragraphs)", paragraphIndex, slideIndex, len(matches))
	}

	// Replace the target paragraph
	targetIdx := paragraphIndex - 1
	start, end := matches[targetIdx][0], matches[targetIdx][1]
	newPara := fmt.Sprintf(`<a:p><a:r><a:t>%s</a:t></a:r></a:p>`, escapeXML(newText))
	slideXML = slideXML[:start] + newPara + slideXML[end:]

	contentXMLs[slidePath] = slideXML
	return ReplaceContentXML(ooxmlBytes, contentXMLs)
}

// UpdateTableCells updates specific cells in a table on a slide.
// Row and Col are 1-based.
func UpdateTableCells(ooxmlBytes []byte, slideIndex int, cellUpdates []CellUpdate) ([]byte, error) {
	if slideIndex < 1 {
		return nil, fmt.Errorf("slideIndex must be >= 1, got %d", slideIndex)
	}
	if len(cellUpdates) == 0 {
		return ooxmlBytes, nil // No-op
	}

	slidePath := fmt.Sprintf("ppt/slides/slide%d.xml", slideIndex)

	contentXMLs, err := ExtractContentXML(ooxmlBytes, FileTypePPTX)
	if err != nil {
		return nil, fmt.Errorf("extract content: %w", err)
	}

	slideXML, ok := contentXMLs[slidePath]
	if !ok {
		return nil, fmt.Errorf("slide %d not found", slideIndex)
	}

	// Find all table row tags (namespace-agnostic)
	trRe := regexp.MustCompile(`(?s)<[^:]+:tr[ >].*?</[^:]+:tr>`)
	rowMatches := trRe.FindAllStringIndex(slideXML, -1)

	if len(rowMatches) == 0 {
		return nil, fmt.Errorf("no table found in slide %d", slideIndex)
	}

	for _, update := range cellUpdates {
		if update.Row < 1 || update.Col < 1 {
			return nil, fmt.Errorf("row and col must be >= 1, got row=%d col=%d", update.Row, update.Col)
		}
		if update.Row > len(rowMatches) {
			return nil, fmt.Errorf("row %d not found (table has %d rows)", update.Row, len(rowMatches))
		}

		// Get target row XML
		rowStart, rowEnd := rowMatches[update.Row-1][0], rowMatches[update.Row-1][1]
		rowXML := slideXML[rowStart:rowEnd]

		// Find all cells in this row (namespace-agnostic)
		tcRe := regexp.MustCompile(`(?s)<[^:]+:tc[ >].*?</[^:]+:tc>`)
		cellMatches := tcRe.FindAllStringIndex(rowXML, -1)

		if update.Col > len(cellMatches) {
			return nil, fmt.Errorf("col %d not found (row has %d cells)", update.Col, len(cellMatches))
		}

		// Get target cell XML
		cellStart, cellEnd := cellMatches[update.Col-1][0], cellMatches[update.Col-1][1]
		cellXML := rowXML[cellStart:cellEnd]

		// Replace text in cell (namespace-agnostic)
		textRe := regexp.MustCompile(`<[^:]+:t>[^<]*</[^:]+:t>`)
		newCellXML := textRe.ReplaceAllString(cellXML, fmt.Sprintf(`<a:t>%s</a:t>`, escapeXML(update.Value)))

		// Rebuild row and slide
		newRowXML := rowXML[:cellStart] + newCellXML + rowXML[cellEnd:]
		slideXML = slideXML[:rowStart] + newRowXML + slideXML[rowEnd:]

		// Re-compute row matches after modification
		rowMatches = trRe.FindAllStringIndex(slideXML, -1)
	}

	contentXMLs[slidePath] = slideXML
	return ReplaceContentXML(ooxmlBytes, contentXMLs)
}

// escapeXML escapes special XML characters
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
