package officegen

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
)

// DocxParagraph represents a paragraph in the legacy document model.
type DocxParagraph struct {
	Text    string `json:"text"`
	Level   int    `json:"level"`
	IsBold  bool   `json:"isBold"`
	IsList  bool   `json:"isList"`
	ListNum int    `json:"listNum"`
}

// DocxBlock represents one semantic document block.
type DocxBlock struct {
	Type    string     `json:"type"`
	Level   int        `json:"level,omitempty"`
	Text    string     `json:"text,omitempty"`
	Title   string     `json:"title,omitempty"`
	Tone    string     `json:"tone,omitempty"`
	Items   []string   `json:"items,omitempty"`
	Columns []string   `json:"columns,omitempty"`
	Rows    [][]string `json:"rows,omitempty"`
}

// DocxDocumentSpec is the semantic input model for DOCX generation.
type DocxDocumentSpec struct {
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle,omitempty"`
	Theme    *DocumentTheme `json:"theme,omitempty"`
	Blocks   []DocxBlock    `json:"blocks,omitempty"`
}

// DOCXOptions configures DOCX generation.
type DOCXOptions struct {
	Title   string
	Creator string
	Style   string
	Theme   *DocumentTheme
}

// DOCXGenerator builds DOCX files.
type DOCXGenerator struct{}

// NewDOCXGenerator creates a DOCX generator instance.
func NewDOCXGenerator() *DOCXGenerator {
	return &DOCXGenerator{}
}

// Generate keeps the legacy paragraph-based API for modify flows.
func (g *DOCXGenerator) Generate(paragraphs []DocxParagraph, opts DOCXOptions) ([]byte, error) {
	if len(paragraphs) == 0 {
		return nil, fmt.Errorf("paragraphs cannot be empty")
	}
	return g.GenerateSpec(legacyDocxSpecFromParagraphs(paragraphs), opts)
}

// GenerateSpec builds a DOCX file from the semantic block model.
func (g *DOCXGenerator) GenerateSpec(spec DocxDocumentSpec, opts DOCXOptions) ([]byte, error) {
	spec = normalizeDocxSpec(spec)
	if len(spec.Blocks) == 0 && strings.TrimSpace(spec.Title) == "" {
		return nil, fmt.Errorf("docx spec cannot be empty")
	}

	theme := ResolveDocumentTheme(opts.Style, spec.Theme)
	if opts.Theme != nil {
		theme = MergeDocumentTheme(theme, opts.Theme)
	}

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	files := g.buildFiles(spec, opts, theme)

	for path, content := range files {
		f, err := w.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create file %s: %w", path, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}
	return buf.Bytes(), nil
}

func legacyDocxSpecFromParagraphs(paragraphs []DocxParagraph) DocxDocumentSpec {
	blocks := make([]DocxBlock, 0, len(paragraphs))
	var listType string
	var listItems []string
	flushList := func() {
		if len(listItems) == 0 {
			listType = ""
			return
		}
		blocks = append(blocks, DocxBlock{Type: listType, Items: append([]string(nil), listItems...)})
		listType = ""
		listItems = nil
	}

	for _, p := range paragraphs {
		text := strings.TrimSpace(p.Text)
		if text == "" {
			continue
		}
		if p.IsList {
			nextType := "bullets"
			if p.ListNum > 0 {
				nextType = "numbered_list"
			}
			if listType != "" && listType != nextType {
				flushList()
			}
			listType = nextType
			listItems = append(listItems, text)
			continue
		}
		flushList()
		if p.Level >= 1 && p.Level <= 6 {
			blocks = append(blocks, DocxBlock{Type: "heading", Level: p.Level, Text: text})
			continue
		}
		blocks = append(blocks, DocxBlock{Type: "paragraph", Text: text})
	}
	flushList()
	return DocxDocumentSpec{Blocks: blocks}
}

func normalizeDocxSpec(spec DocxDocumentSpec) DocxDocumentSpec {
	spec.Title = strings.TrimSpace(spec.Title)
	spec.Subtitle = strings.TrimSpace(spec.Subtitle)
	blocks := make([]DocxBlock, 0, len(spec.Blocks))
	for _, block := range spec.Blocks {
		block.Type = normalizeDocxBlockType(block.Type)
		block.Text = strings.TrimSpace(block.Text)
		block.Title = strings.TrimSpace(block.Title)
		block.Tone = strings.TrimSpace(block.Tone)
		block.Items = compactStrings(block.Items)
		block.Columns = compactStrings(block.Columns)
		rows := make([][]string, 0, len(block.Rows))
		for _, row := range block.Rows {
			cells := make([]string, 0, len(row))
			for _, cell := range row {
				cells = append(cells, strings.TrimSpace(cell))
			}
			rows = append(rows, cells)
		}
		block.Rows = rows
		if block.Level <= 0 {
			block.Level = 2
		}
		if block.Type == "table" && len(block.Columns) == 0 && len(block.Rows) > 0 {
			block.Columns = append([]string(nil), block.Rows[0]...)
			if len(block.Rows) > 1 {
				block.Rows = append([][]string(nil), block.Rows[1:]...)
			} else {
				block.Rows = nil
			}
		}
		if docxBlockEmpty(block) {
			continue
		}
		blocks = append(blocks, block)
	}
	spec.Blocks = blocks
	return spec
}

func normalizeDocxBlockType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "heading", "title":
		return "heading"
	case "bullets", "bullet_list", "bullet":
		return "bullets"
	case "numbered_list", "numbered", "ordered_list":
		return "numbered_list"
	case "table":
		return "table"
	case "callout", "note":
		return "callout"
	case "quote":
		return "quote"
	case "divider":
		return "divider"
	default:
		return "paragraph"
	}
}

func docxBlockEmpty(block DocxBlock) bool {
	switch block.Type {
	case "bullets", "numbered_list":
		return len(block.Items) == 0
	case "table":
		return len(block.Columns) == 0 && len(block.Rows) == 0 && block.Title == ""
	case "divider":
		return false
	default:
		return block.Text == "" && block.Title == ""
	}
}

func (g *DOCXGenerator) buildFiles(spec DocxDocumentSpec, opts DOCXOptions, theme DocumentTheme) map[string]string {
	title := opts.Title
	if strings.TrimSpace(title) == "" {
		title = spec.Title
	}
	files := map[string]string{
		"[Content_Types].xml":          docxContentTypes,
		"_rels/.rels":                  docxRootRels,
		"docProps/core.xml":            g.generateCoreXML(title, opts.Creator),
		"docProps/app.xml":             docxAppXML,
		"word/document.xml":            g.generateDocumentXML(spec, theme),
		"word/_rels/document.xml.rels": docxDocumentRels,
		"word/styles.xml":              g.generateStylesXML(theme),
		"word/settings.xml":            g.generateSettingsXML(),
		"word/fontTable.xml":           g.generateFontTableXML(theme),
		"word/numbering.xml":           docxNumberingXML,
		"word/theme/theme1.xml":        officeThemeXML,
	}
	return files
}

func (g *DOCXGenerator) generateCoreXML(title, creator string) string {
	if strings.TrimSpace(title) == "" {
		title = "Untitled Document"
	}
	if strings.TrimSpace(creator) == "" {
		creator = "officecli"
	}
	createdTime := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:dcterms="http://purl.org/dc/terms/"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
    <dc:title>%s</dc:title>
    <dc:creator>%s</dc:creator>
    <dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created>
</cp:coreProperties>`, escapeXML(title), escapeXML(creator), createdTime)
}

func (g *DOCXGenerator) generateDocumentXML(spec DocxDocumentSpec, theme DocumentTheme) string {
	var body strings.Builder
	if spec.Title != "" {
		body.WriteString(g.createStyledParagraphXML("DocTitle", spec.Title))
	}
	if spec.Subtitle != "" {
		body.WriteString(g.createStyledParagraphXML("DocSubtitle", spec.Subtitle))
	}
	for _, block := range spec.Blocks {
		switch block.Type {
		case "heading":
			level := block.Level
			if level < 1 {
				level = 1
			}
			if level > 6 {
				level = 6
			}
			body.WriteString(g.createStyledParagraphXML(fmt.Sprintf("Heading%d", level), block.Text))
		case "bullets":
			for _, item := range block.Items {
				body.WriteString(g.createListParagraphXML(item, 1))
			}
		case "numbered_list":
			for _, item := range block.Items {
				body.WriteString(g.createListParagraphXML(item, 2))
			}
		case "quote":
			body.WriteString(g.createStyledParagraphXML("Quote", block.Text))
		case "callout":
			if block.Title != "" {
				body.WriteString(g.createStyledParagraphXML("CalloutTitle", block.Title))
			}
			body.WriteString(g.createStyledParagraphXML("Callout", block.Text))
		case "divider":
			body.WriteString(docxDividerXML)
		case "table":
			if block.Title != "" {
				body.WriteString(g.createStyledParagraphXML("Heading3", block.Title))
			}
			body.WriteString(g.createTableXML(block, theme))
		default:
			body.WriteString(g.createStyledParagraphXML("Normal", block.Text))
		}
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
    <w:body>
%s        <w:sectPr>
            <w:pgSz w:w="11906" w:h="16838"/>
            <w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="720" w:footer="720" w:gutter="0"/>
        </w:sectPr>
    </w:body>
</w:document>`, body.String())
}

func (g *DOCXGenerator) createStyledParagraphXML(styleID, text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return fmt.Sprintf(`        <w:p>
            <w:pPr><w:pStyle w:val="%s"/></w:pPr>
%s        </w:p>
`, escapeXML(styleID), paragraphRunsXML(text, false))
}

func (g *DOCXGenerator) createListParagraphXML(text string, numID int) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return fmt.Sprintf(`        <w:p>
            <w:pPr>
                <w:pStyle w:val="ListParagraph"/>
                <w:numPr>
                    <w:ilvl w:val="0"/>
                    <w:numId w:val="%d"/>
                </w:numPr>
            </w:pPr>
%s        </w:p>
`, numID, paragraphRunsXML(text, false))
}

func paragraphRunsXML(text string, bold bool) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	var runs strings.Builder
	runProps := ""
	if bold {
		runProps = "<w:rPr><w:b/></w:rPr>"
	}
	for idx, line := range lines {
		if idx > 0 {
			runs.WriteString("            <w:r><w:br/></w:r>\n")
		}
		runs.WriteString(fmt.Sprintf("            <w:r>%s<w:t xml:space=\"preserve\">%s</w:t></w:r>\n", runProps, escapeXML(line)))
	}
	return runs.String()
}

func (g *DOCXGenerator) createTableXML(block DocxBlock, theme DocumentTheme) string {
	columnCount := len(block.Columns)
	for _, row := range block.Rows {
		if len(row) > columnCount {
			columnCount = len(row)
		}
	}
	if columnCount == 0 {
		return ""
	}
	width := 8300 / columnCount
	borderColor := normalizeHexColor(theme.BorderColor, "CBD5E1")
	headerFill := normalizeHexColor(theme.AccentSoft, "DBEAFE")
	headerText := normalizeHexColor(theme.TitleColor, "0F172A")

	var grid strings.Builder
	for i := 0; i < columnCount; i++ {
		grid.WriteString(fmt.Sprintf("            <w:gridCol w:w=\"%d\"/>\n", width))
	}

	var rows strings.Builder
	if len(block.Columns) > 0 {
		rows.WriteString("        <w:tr>\n")
		for idx := 0; idx < columnCount; idx++ {
			value := ""
			if idx < len(block.Columns) {
				value = block.Columns[idx]
			}
			rows.WriteString(docxTableCellXML(value, width, true, borderColor, headerFill, headerText))
		}
		rows.WriteString("        </w:tr>\n")
	}
	for _, row := range block.Rows {
		rows.WriteString("        <w:tr>\n")
		for idx := 0; idx < columnCount; idx++ {
			value := ""
			if idx < len(row) {
				value = row[idx]
			}
			rows.WriteString(docxTableCellXML(value, width, false, borderColor, "FFFFFF", theme.TextColor))
		}
		rows.WriteString("        </w:tr>\n")
	}

	return fmt.Sprintf(`        <w:tbl>
            <w:tblPr>
                <w:tblStyle w:val="TableGrid"/>
                <w:tblW w:w="5000" w:type="pct"/>
                <w:tblLayout w:type="fixed"/>
                <w:tblBorders>
                    <w:top w:val="single" w:sz="8" w:space="0" w:color="%s"/>
                    <w:left w:val="single" w:sz="8" w:space="0" w:color="%s"/>
                    <w:bottom w:val="single" w:sz="8" w:space="0" w:color="%s"/>
                    <w:right w:val="single" w:sz="8" w:space="0" w:color="%s"/>
                    <w:insideH w:val="single" w:sz="6" w:space="0" w:color="%s"/>
                    <w:insideV w:val="single" w:sz="6" w:space="0" w:color="%s"/>
                </w:tblBorders>
            </w:tblPr>
            <w:tblGrid>
%s            </w:tblGrid>
%s        </w:tbl>
`, borderColor, borderColor, borderColor, borderColor, borderColor, borderColor, grid.String(), rows.String())
}

func docxTableCellXML(value string, width int, header bool, borderColor, fillColor, textColor string) string {
	fill := normalizeHexColor(fillColor, "FFFFFF")
	text := normalizeHexColor(textColor, "0F172A")
	runProps := ""
	if header {
		runProps = fmt.Sprintf(`<w:rPr><w:b/><w:color w:val="%s"/></w:rPr>`, text)
	}
	return fmt.Sprintf(`            <w:tc>
                <w:tcPr>
                    <w:tcW w:w="%d" w:type="dxa"/>
                    <w:shd w:val="clear" w:color="auto" w:fill="%s"/>
                    <w:tcBorders>
                        <w:top w:val="single" w:sz="4" w:space="0" w:color="%s"/>
                        <w:left w:val="single" w:sz="4" w:space="0" w:color="%s"/>
                        <w:bottom w:val="single" w:sz="4" w:space="0" w:color="%s"/>
                        <w:right w:val="single" w:sz="4" w:space="0" w:color="%s"/>
                    </w:tcBorders>
                </w:tcPr>
                <w:p>
                    <w:pPr><w:spacing w:after="80"/></w:pPr>
                    <w:r>%s<w:t xml:space="preserve">%s</w:t></w:r>
                </w:p>
            </w:tc>
`, width, fill, borderColor, borderColor, borderColor, borderColor, runProps, escapeXML(value))
}

func (g *DOCXGenerator) generateSettingsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
    <w:zoom w:percent="100"/>
    <w:defaultTabStop w:val="720"/>
    <w:characterSpacingControl w:val="doNotCompress"/>
    <w:displayBackgroundShape/>
</w:settings>`
}

func (g *DOCXGenerator) generateFontTableXML(theme DocumentTheme) string {
	latin := escapeXML(theme.FontFamily)
	ea := escapeXML(theme.EAFontFamily)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:fonts xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
    <w:font w:name="%s">
        <w:family w:val="swiss"/>
        <w:pitch w:val="variable"/>
    </w:font>
    <w:font w:name="%s">
        <w:charset w:val="86"/>
        <w:family w:val="swiss"/>
        <w:pitch w:val="variable"/>
    </w:font>
</w:fonts>`, latin, ea)
}

func (g *DOCXGenerator) generateStylesXML(theme DocumentTheme) string {
	accent := normalizeHexColor(theme.AccentColor, "1D4ED8")
	accentSoft := normalizeHexColor(theme.AccentSoft, "DBEAFE")
	border := normalizeHexColor(theme.BorderColor, "CBD5E1")
	text := normalizeHexColor(theme.TextColor, "0F172A")
	title := normalizeHexColor(theme.TitleColor, "020617")
	muted := normalizeHexColor(theme.MutedColor, "64748B")
	latin := escapeXML(theme.FontFamily)
	ea := escapeXML(theme.EAFontFamily)

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
    <w:docDefaults>
        <w:rPrDefault>
            <w:rPr>
                <w:rFonts w:ascii="%s" w:eastAsia="%s" w:hAnsi="%s"/>
                <w:color w:val="%s"/>
                <w:sz w:val="22"/>
                <w:szCs w:val="22"/>
                <w:lang w:val="en-US" w:eastAsia="zh-CN"/>
            </w:rPr>
        </w:rPrDefault>
        <w:pPrDefault>
            <w:pPr>
                <w:spacing w:before="0" w:after="140" w:line="320" w:lineRule="auto"/>
            </w:pPr>
        </w:pPrDefault>
    </w:docDefaults>
    <w:style w:type="paragraph" w:default="1" w:styleId="Normal">
        <w:name w:val="Normal"/>
    </w:style>
    <w:style w:type="paragraph" w:styleId="DocTitle">
        <w:name w:val="Document Title"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="DocSubtitle"/>
        <w:pPr><w:spacing w:before="0" w:after="80"/></w:pPr>
        <w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="34"/><w:szCs w:val="34"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="DocSubtitle">
        <w:name w:val="Document Subtitle"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr><w:spacing w:before="0" w:after="240"/></w:pPr>
        <w:rPr><w:color w:val="%s"/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading1">
        <w:name w:val="heading 1"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr><w:spacing w:before="320" w:after="100"/><w:outlineLvl w:val="0"/></w:pPr>
        <w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="28"/><w:szCs w:val="28"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading2">
        <w:name w:val="heading 2"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr><w:spacing w:before="260" w:after="80"/><w:outlineLvl w:val="1"/></w:pPr>
        <w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="24"/><w:szCs w:val="24"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading3">
        <w:name w:val="heading 3"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr><w:spacing w:before="220" w:after="60"/><w:outlineLvl w:val="2"/></w:pPr>
        <w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading4">
        <w:name w:val="heading 4"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr><w:spacing w:before="200" w:after="40"/><w:outlineLvl w:val="3"/></w:pPr>
        <w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="20"/><w:szCs w:val="20"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading5">
        <w:name w:val="heading 5"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="18"/><w:szCs w:val="18"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading6">
        <w:name w:val="heading 6"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="16"/><w:szCs w:val="16"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Quote">
        <w:name w:val="Quote"/>
        <w:basedOn w:val="Normal"/>
        <w:pPr>
            <w:ind w:left="520" w:right="200"/>
            <w:spacing w:before="120" w:after="160"/>
            <w:pBdr><w:left w:val="single" w:sz="10" w:space="10" w:color="%s"/></w:pBdr>
        </w:pPr>
        <w:rPr><w:i/><w:color w:val="%s"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="CalloutTitle">
        <w:name w:val="Callout Title"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Callout"/>
        <w:pPr><w:spacing w:before="180" w:after="40"/></w:pPr>
        <w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="20"/><w:szCs w:val="20"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Callout">
        <w:name w:val="Callout"/>
        <w:basedOn w:val="Normal"/>
        <w:pPr>
            <w:spacing w:before="0" w:after="180"/>
            <w:ind w:left="320" w:right="120"/>
            <w:shd w:val="clear" w:color="auto" w:fill="%s"/>
            <w:pBdr>
                <w:top w:val="single" w:sz="6" w:space="0" w:color="%s"/>
                <w:left w:val="single" w:sz="6" w:space="0" w:color="%s"/>
                <w:bottom w:val="single" w:sz="6" w:space="0" w:color="%s"/>
                <w:right w:val="single" w:sz="6" w:space="0" w:color="%s"/>
            </w:pBdr>
        </w:pPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="ListParagraph">
        <w:name w:val="List Paragraph"/>
        <w:basedOn w:val="Normal"/>
        <w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr>
    </w:style>
</w:styles>`, latin, ea, latin, text, title, muted, accent, accent, accent, accent, accent, accent, accent, muted, accent, accentSoft, border, border, border, border)
}

const docxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
    <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
    <Default Extension="xml" ContentType="application/xml"/>
    <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
    <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
    <Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>
    <Override PartName="/word/fontTable.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.fontTable+xml"/>
    <Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/>
    <Override PartName="/word/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>
    <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
    <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`

const docxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
    <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
    <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
    <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const docxDocumentRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
    <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
    <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>
    <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/fontTable" Target="fontTable.xml"/>
    <Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering" Target="numbering.xml"/>
    <Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>
</Relationships>`

const docxAppXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
    <Application>officecli DOCX Generator</Application>
</Properties>`

const docxNumberingXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
    <w:abstractNum w:abstractNumId="0">
        <w:multiLevelType w:val="hybridMultilevel"/>
        <w:lvl w:ilvl="0">
            <w:start w:val="1"/>
            <w:numFmt w:val="bullet"/>
            <w:lvlText w:val="&#x2022;"/>
            <w:lvlJc w:val="left"/>
            <w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr>
        </w:lvl>
    </w:abstractNum>
    <w:abstractNum w:abstractNumId="1">
        <w:multiLevelType w:val="multilevel"/>
        <w:lvl w:ilvl="0">
            <w:start w:val="1"/>
            <w:numFmt w:val="decimal"/>
            <w:lvlText w:val="%1."/>
            <w:lvlJc w:val="left"/>
            <w:pPr><w:ind w:left="720" w:hanging="360"/></w:pPr>
        </w:lvl>
    </w:abstractNum>
    <w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>
    <w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num>
</w:numbering>`

const docxDividerXML = `        <w:p>
            <w:pPr>
                <w:spacing w:before="80" w:after="120"/>
                <w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="DCE4F2"/></w:pBdr>
            </w:pPr>
        </w:p>
`
