package officegen

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"
)

// DocxParagraph 表示文档中的一个段落
type DocxParagraph struct {
	Text    string `json:"text"`    // 段落文本
	Level   int    `json:"level"`   // 标题级别: 0=正文, 1-6=标题级别
	IsBold  bool   `json:"isBold"`  // 是否加粗
	IsList  bool   `json:"isList"`  // 是否列表项
	ListNum int    `json:"listNum"` // 列表序号(0 表示无序列表)
}

// DOCXOptions 配置生成选项
type DOCXOptions struct {
	Title   string // 文档标题
	Creator string // 作者
}

// DOCXGenerator DOCX 生成器
type DOCXGenerator struct{}

// NewDOCXGenerator 创建 DOCX 生成器实例
func NewDOCXGenerator() *DOCXGenerator {
	return &DOCXGenerator{}
}

// Generate 生成 DOCX 并返回字节流
func (g *DOCXGenerator) Generate(paragraphs []DocxParagraph, opts DOCXOptions) ([]byte, error) {
	if len(paragraphs) == 0 {
		return nil, fmt.Errorf("paragraphs cannot be empty")
	}

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	files := g.buildFiles(paragraphs, opts)

	for path, content := range files {
		f, err := w.Create(path)
		if err != nil {
			return nil, fmt.Errorf("failed to create file %s: %w", path, err)
		}
		_, err = f.Write([]byte(content))
		if err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to close zip writer: %w", err)
	}

	return buf.Bytes(), nil
}

func (g *DOCXGenerator) buildFiles(paragraphs []DocxParagraph, opts DOCXOptions) map[string]string {
	files := map[string]string{
		"[Content_Types].xml":          docxContentTypes,
		"_rels/.rels":                  docxRootRels,
		"docProps/core.xml":            g.generateCoreXML(opts),
		"docProps/app.xml":             docxAppXML,
		"word/document.xml":            g.generateDocumentXML(paragraphs),
		"word/_rels/document.xml.rels": docxDocumentRels,
		"word/styles.xml":              docxStylesXML,
		"word/settings.xml":            docxSettingsXML,
		"word/fontTable.xml":           docxFontTableXML,
		"word/numbering.xml":           docxNumberingXML,
		"word/theme/theme1.xml":        officeThemeXML,
	}
	return files
}

func (g *DOCXGenerator) generateCoreXML(opts DOCXOptions) string {
	title := opts.Title
	if title == "" {
		title = "Untitled Document"
	}
	creator := opts.Creator
	if creator == "" {
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

func (g *DOCXGenerator) generateDocumentXML(paragraphs []DocxParagraph) string {
	var body strings.Builder
	for _, p := range paragraphs {
		body.WriteString(g.createParagraphXML(p))
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
    <w:body>
%s        <w:sectPr>
            <w:pgSz w:w="11906" w:h="16838"/>
            <w:pgMar w:top="1440" w:right="1800" w:bottom="1440" w:left="1800" w:header="851" w:footer="992" w:gutter="0"/>
        </w:sectPr>
    </w:body>
</w:document>`, body.String())
}

func (g *DOCXGenerator) createParagraphXML(p DocxParagraph) string {
	// 段落属性
	pPr := ""
	rPr := ""

	if p.Level >= 1 && p.Level <= 6 {
		// 标题段落
		pPr = fmt.Sprintf(`            <w:pPr><w:pStyle w:val="Heading%d"/></w:pPr>
`, p.Level)
	} else if p.IsList {
		// 列表项
		numFmt := "bullet"
		if p.ListNum > 0 {
			numFmt = "decimal"
		}
		_ = numFmt
		pPr = `            <w:pPr>
                <w:numPr>
                    <w:ilvl w:val="0"/>
                    <w:numId w:val="1"/>
                </w:numPr>
            </w:pPr>
`
	}

	if p.IsBold {
		rPr = `                <w:rPr><w:b/></w:rPr>
`
	}

	// 处理多行文本：每行一个 run，中间用换行符分隔
	lines := strings.Split(p.Text, "\n")
	var runs strings.Builder
	for i, line := range lines {
		if i > 0 {
			runs.WriteString(`            <w:r><w:br/></w:r>
`)
		}
		runs.WriteString(fmt.Sprintf(`            <w:r>
%s                <w:t xml:space="preserve">%s</w:t>
            </w:r>
`, rPr, escapeXML(line)))
	}

	return fmt.Sprintf(`        <w:p>
%s%s        </w:p>
`, pPr, runs.String())
}

// ---- 静态模板 ----

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

const docxSettingsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
    <w:defaultTabStop w:val="720"/>
    <w:characterSpacingControl w:val="doNotCompress"/>
</w:settings>`

const docxFontTableXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:fonts xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
    <w:font w:name="Microsoft YaHei">
        <w:panose1 w:val="020B0503020204020204"/>
        <w:charset w:val="86"/>
        <w:family w:val="swiss"/>
        <w:pitch w:val="variable"/>
    </w:font>
    <w:font w:name="Times New Roman">
        <w:panose1 w:val="02020603050405020304"/>
        <w:charset w:val="00"/>
        <w:family w:val="roman"/>
        <w:pitch w:val="variable"/>
    </w:font>
</w:fonts>`

const docxStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
    <w:docDefaults>
        <w:rPrDefault>
            <w:rPr>
                <w:rFonts w:ascii="Microsoft YaHei" w:eastAsia="Microsoft YaHei" w:hAnsi="Microsoft YaHei"/>
                <w:sz w:val="24"/>
                <w:szCs w:val="24"/>
                <w:lang w:val="en-US" w:eastAsia="zh-CN"/>
            </w:rPr>
        </w:rPrDefault>
        <w:pPrDefault>
            <w:pPr>
                <w:spacing w:after="200" w:line="276" w:lineRule="auto"/>
            </w:pPr>
        </w:pPrDefault>
    </w:docDefaults>
    <w:style w:type="paragraph" w:default="1" w:styleId="Normal">
        <w:name w:val="Normal"/>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading1">
        <w:name w:val="heading 1"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr>
            <w:spacing w:before="480" w:after="120"/>
            <w:outlineLvl w:val="0"/>
        </w:pPr>
        <w:rPr><w:b/><w:sz w:val="48"/><w:szCs w:val="48"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading2">
        <w:name w:val="heading 2"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr>
            <w:spacing w:before="360" w:after="80"/>
            <w:outlineLvl w:val="1"/>
        </w:pPr>
        <w:rPr><w:b/><w:sz w:val="36"/><w:szCs w:val="36"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading3">
        <w:name w:val="heading 3"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr>
            <w:spacing w:before="280" w:after="80"/>
            <w:outlineLvl w:val="2"/>
        </w:pPr>
        <w:rPr><w:b/><w:sz w:val="28"/><w:szCs w:val="28"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading4">
        <w:name w:val="heading 4"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr>
            <w:spacing w:before="240" w:after="60"/>
            <w:outlineLvl w:val="3"/>
        </w:pPr>
        <w:rPr><w:b/><w:sz w:val="24"/><w:szCs w:val="24"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading5">
        <w:name w:val="heading 5"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr>
            <w:spacing w:before="200" w:after="60"/>
            <w:outlineLvl w:val="4"/>
        </w:pPr>
        <w:rPr><w:b/><w:sz w:val="22"/><w:szCs w:val="22"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="Heading6">
        <w:name w:val="heading 6"/>
        <w:basedOn w:val="Normal"/>
        <w:next w:val="Normal"/>
        <w:pPr>
            <w:spacing w:before="200" w:after="60"/>
            <w:outlineLvl w:val="5"/>
        </w:pPr>
        <w:rPr><w:b/><w:sz w:val="20"/><w:szCs w:val="20"/></w:rPr>
    </w:style>
    <w:style w:type="paragraph" w:styleId="ListParagraph">
        <w:name w:val="List Paragraph"/>
        <w:basedOn w:val="Normal"/>
        <w:pPr>
            <w:ind w:left="720"/>
        </w:pPr>
    </w:style>
</w:styles>`

const docxNumberingXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
    <w:abstractNum w:abstractNumId="0">
        <w:multiLevelType w:val="hybridMultilevel"/>
        <w:lvl w:ilvl="0">
            <w:start w:val="1"/>
            <w:numFmt w:val="bullet"/>
            <w:lvlText w:val="&#x2022;"/>
            <w:lvlJc w:val="left"/>
            <w:pPr>
                <w:ind w:left="720" w:hanging="360"/>
            </w:pPr>
        </w:lvl>
    </w:abstractNum>
    <w:num w:numId="1">
        <w:abstractNumId w:val="0"/>
    </w:num>
</w:numbering>`
