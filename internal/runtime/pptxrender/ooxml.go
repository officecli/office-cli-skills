package pptxrender

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"strings"
)

type imageBinding struct {
	SlideIndex   int
	ShapeName    string
	Data         []byte
	ContentType  string
	Extension    string
	Relationship string
	MediaName    string
}

type shapePatch struct {
	SlideIndex int
	ShapeName  string
	Fill       string
	Line       LineStyle
}

type tablePatch struct {
	SlideIndex int
	ShapeName  string
	Border     LineStyle
}

type zipEntry struct {
	Name   string
	Method uint16
	Body   []byte
}

type PackageEditor struct {
	entries []zipEntry
}

func NewPackageEditor(data []byte) (*PackageEditor, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	entries := make([]zipEntry, 0, len(reader.File))
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		entries = append(entries, zipEntry{Name: file.Name, Method: file.Method, Body: body})
	}
	return &PackageEditor{entries: entries}, nil
}

func (p *PackageEditor) Part(name string) ([]byte, bool) {
	for _, entry := range p.entries {
		if entry.Name == name {
			return entry.Body, true
		}
	}
	return nil, false
}

func (p *PackageEditor) Upsert(name string, body []byte) {
	for index := range p.entries {
		if p.entries[index].Name == name {
			p.entries[index].Body = body
			return
		}
	}
	p.entries = append(p.entries, zipEntry{Name: name, Method: zip.Deflate, Body: body})
}

func (p *PackageEditor) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, entry := range p.entries {
		header := &zip.FileHeader{Name: entry.Name, Method: entry.Method}
		if header.Method == 0 {
			header.Method = zip.Deflate
		}
		w, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(entry.Body); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type OOXMLPatcher interface {
	Apply(pkg *PackageEditor) error
}

type PatchPipeline struct {
	patchers []OOXMLPatcher
}

func (p PatchPipeline) Apply(pkg *PackageEditor) error {
	for _, patcher := range p.patchers {
		if err := patcher.Apply(pkg); err != nil {
			return err
		}
	}
	return nil
}

type SlideSizePatcher struct {
	SlideSize SlideSize
	Units     UnitOptions
}

func (p SlideSizePatcher) Apply(pkg *PackageEditor) error {
	body, ok := pkg.Part("ppt/presentation.xml")
	if !ok {
		return fmt.Errorf("missing ppt/presentation.xml")
	}
	replacement := fmt.Sprintf(`<p:sldSz cx="%d" cy="%d"/>`, emuFromPx(p.SlideSize.Width, p.Units.PxPerInch), emuFromPx(p.SlideSize.Height, p.Units.PxPerInch))
	updated := sldSizeRegexp.ReplaceAllString(string(body), replacement)
	if updated == string(body) {
		return fmt.Errorf("presentation.xml has no p:sldSz")
	}
	pkg.Upsert("ppt/presentation.xml", []byte(updated))
	return nil
}

type ShapeStylePatcher struct {
	Patches []shapePatch
}

func (p ShapeStylePatcher) Apply(pkg *PackageEditor) error {
	if len(p.Patches) == 0 {
		return nil
	}
	patchesBySlide := make(map[int][]shapePatch)
	for _, patch := range p.Patches {
		patchesBySlide[patch.SlideIndex] = append(patchesBySlide[patch.SlideIndex], patch)
	}
	for slideIndex, patches := range patchesBySlide {
		slideName := fmt.Sprintf("ppt/slides/slide%d.xml", slideIndex)
		body, ok := pkg.Part(slideName)
		if !ok {
			return fmt.Errorf("missing %s", slideName)
		}
		updated, err := applyShapeStylePatches(string(body), patches)
		if err != nil {
			return fmt.Errorf("%s: %w", slideName, err)
		}
		pkg.Upsert(slideName, []byte(updated))
	}
	return nil
}

type TableBorderPatcher struct {
	Patches []tablePatch
}

func (p TableBorderPatcher) Apply(pkg *PackageEditor) error {
	if len(p.Patches) == 0 {
		return nil
	}
	patchesBySlide := make(map[int][]tablePatch)
	for _, patch := range p.Patches {
		patchesBySlide[patch.SlideIndex] = append(patchesBySlide[patch.SlideIndex], patch)
	}
	for slideIndex, patches := range patchesBySlide {
		slideName := fmt.Sprintf("ppt/slides/slide%d.xml", slideIndex)
		body, ok := pkg.Part(slideName)
		if !ok {
			return fmt.Errorf("missing %s", slideName)
		}
		updated, err := applyTableBorderPatches(string(body), patches)
		if err != nil {
			return fmt.Errorf("%s: %w", slideName, err)
		}
		pkg.Upsert(slideName, []byte(updated))
	}
	return nil
}

type MediaPartPatcher struct {
	Bindings []imageBinding
}

func (p MediaPartPatcher) Apply(pkg *PackageEditor) error {
	if len(p.Bindings) == 0 {
		return nil
	}
	contentTypesBody, ok := pkg.Part("[Content_Types].xml")
	if !ok {
		return fmt.Errorf("missing [Content_Types].xml")
	}
	contentTypes := string(contentTypesBody)
	for _, binding := range p.Bindings {
		pkg.Upsert(binding.MediaName, binding.Data)
		contentTypes = ensureContentTypeDefault(contentTypes, binding.Extension, binding.ContentType)
	}
	pkg.Upsert("[Content_Types].xml", []byte(contentTypes))
	return nil
}

type ImageRelationshipPatcher struct {
	Bindings []imageBinding
}

func (p ImageRelationshipPatcher) Apply(pkg *PackageEditor) error {
	if len(p.Bindings) == 0 {
		return nil
	}
	for _, binding := range p.Bindings {
		slideName := fmt.Sprintf("ppt/slides/slide%d.xml", binding.SlideIndex)
		slideBody, ok := pkg.Part(slideName)
		if !ok {
			return fmt.Errorf("missing %s", slideName)
		}
		slideXML, replaced := applyImageBinding(string(slideBody), binding)
		if !replaced {
			return fmt.Errorf("no image blip found for %q in %s", binding.ShapeName, slideName)
		}
		pkg.Upsert(slideName, []byte(slideXML))

		relsName := fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", binding.SlideIndex)
		relsBody, ok := pkg.Part(relsName)
		if !ok {
			relsBody = defaultRelationshipsXML()
		}
		relsXML := appendRelationship(
			string(relsBody),
			binding.Relationship,
			"http://schemas.openxmlformats.org/officeDocument/2006/relationships/image",
			"../media/"+filepath.Base(binding.MediaName),
		)
		pkg.Upsert(relsName, []byte(relsXML))
	}
	return nil
}

var (
	sldSizeRegexp       = regexp.MustCompile(`<p:sldSz[^>]*/>`)
	spBlockRegexp       = regexp.MustCompile(`(?s)<p:sp>.*?</p:sp>`)
	picBlockRegexp      = regexp.MustCompile(`(?s)<p:pic>.*?</p:pic>`)
	graphicFrameRegexp  = regexp.MustCompile(`(?s)<p:graphicFrame>.*?</p:graphicFrame>`)
	cNvPrNameRegexp     = regexp.MustCompile(`<p:cNvPr\b[^>]*\bname="([^"]*)"`)
	tableCellPropRegexp = regexp.MustCompile(`(?s)<a:tcPr([^>]*)/>|<a:tcPr([^>]*)>(.*?)</a:tcPr>`)
)

func applyShapeStylePatches(slideXML string, patches []shapePatch) (string, error) {
	patchByName := make(map[string]shapePatch, len(patches))
	for _, patch := range patches {
		patchByName[patch.ShapeName] = patch
	}
	applied := make(map[string]bool, len(patches))
	result := spBlockRegexp.ReplaceAllStringFunc(slideXML, func(block string) string {
		name := cNvPrName(block)
		patch, ok := patchByName[name]
		if !ok {
			return block
		}
		applied[name] = true
		return applyShapeStyle(block, patch)
	})
	for _, patch := range patches {
		if !applied[patch.ShapeName] {
			return result, fmt.Errorf("shape patch target %q not found", patch.ShapeName)
		}
	}
	return result, nil
}

func applyShapeStyle(block string, patch shapePatch) string {
	spPrEnd := strings.Index(block, "</p:spPr>")
	if spPrEnd == -1 {
		return block
	}
	styleXML := shapeStyleXML(patch)
	return block[:spPrEnd] + styleXML + block[spPrEnd:]
}

func shapeStyleXML(patch shapePatch) string {
	var fillXML string
	if patch.Fill == "" {
		fillXML = "<a:noFill/>"
	} else {
		fillXML = fmt.Sprintf(`<a:solidFill><a:srgbClr val="%s"/></a:solidFill>`, hexNoHash(patch.Fill))
	}

	lineColor := defaultString(patch.Line.Color, patch.Fill)
	lineWidth := int(math.Round(defaultFloat(patch.Line.Width, 0) * 12700))
	if lineWidth <= 0 || lineColor == "" {
		return fillXML + `<a:ln w="0"><a:noFill/></a:ln>`
	}
	return fillXML + fmt.Sprintf(
		`<a:ln w="%d"><a:solidFill><a:srgbClr val="%s"/></a:solidFill></a:ln>`,
		lineWidth,
		hexNoHash(lineColor),
	)
}

func applyTableBorderPatches(slideXML string, patches []tablePatch) (string, error) {
	patchByName := make(map[string]tablePatch, len(patches))
	for _, patch := range patches {
		patchByName[patch.ShapeName] = patch
	}
	applied := make(map[string]bool, len(patches))
	result := graphicFrameRegexp.ReplaceAllStringFunc(slideXML, func(block string) string {
		name := cNvPrName(block)
		patch, ok := patchByName[name]
		if !ok {
			return block
		}
		applied[name] = true
		return applyTableBorder(block, patch.Border)
	})
	for _, patch := range patches {
		if !applied[patch.ShapeName] {
			return result, fmt.Errorf("table patch target %q not found", patch.ShapeName)
		}
	}
	return result, nil
}

func applyTableBorder(block string, border LineStyle) string {
	borderXML := tableBorderXML(border)
	return tableCellPropRegexp.ReplaceAllStringFunc(block, func(tcPr string) string {
		if strings.Contains(tcPr, "<a:lnL") {
			return tcPr
		}
		if strings.HasSuffix(tcPr, "/>") {
			return strings.TrimSuffix(tcPr, "/>") + ">" + borderXML + "</a:tcPr>"
		}
		closeIndex := strings.Index(tcPr, ">")
		if closeIndex == -1 {
			return tcPr
		}
		return tcPr[:closeIndex+1] + borderXML + tcPr[closeIndex+1:]
	})
}

func tableBorderXML(border LineStyle) string {
	color := hexNoHash(defaultString(border.Color, "#CBD5E1"))
	width := int(math.Round(defaultFloat(border.Width, 1) * 12700))
	oneSide := func(tag string) string {
		return fmt.Sprintf(`<a:%s w="%d"><a:solidFill><a:srgbClr val="%s"/></a:solidFill></a:%s>`, tag, width, color, tag)
	}
	return oneSide("lnL") + oneSide("lnR") + oneSide("lnT") + oneSide("lnB")
}

func applyImageBinding(slideXML string, binding imageBinding) (string, bool) {
	replaced := false
	result := picBlockRegexp.ReplaceAllStringFunc(slideXML, func(block string) string {
		if replaced || cNvPrName(block) != binding.ShapeName {
			return block
		}
		updated, ok := addImageEmbed(block, binding.Relationship)
		if ok {
			replaced = true
			return updated
		}
		return block
	})
	return result, replaced
}

func addImageEmbed(slideXML, relationship string) (string, bool) {
	replacement := fmt.Sprintf(`<a:blip r:embed="%s"/>`, relationship)
	for _, needle := range []string{`<a:blip/>`, `<a:blip></a:blip>`} {
		if strings.Contains(slideXML, needle) {
			return strings.Replace(slideXML, needle, replacement, 1), true
		}
	}
	if strings.Contains(slideXML, `r:embed=""`) {
		return strings.Replace(slideXML, `r:embed=""`, `r:embed="`+relationship+`"`, 1), true
	}
	if strings.Contains(slideXML, `<a:blip `) && !strings.Contains(slideXML, `r:embed=`) {
		return strings.Replace(slideXML, `<a:blip `, `<a:blip r:embed="`+relationship+`" `, 1), true
	}
	return slideXML, false
}

func ensureContentTypeDefault(contentTypes, ext, contentType string) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if ext == "" || contentType == "" {
		return contentTypes
	}
	if regexp.MustCompile(`(?i)Extension="` + regexp.QuoteMeta(ext) + `"`).MatchString(contentTypes) {
		return contentTypes
	}
	defaultXML := fmt.Sprintf(`<Default Extension="%s" ContentType="%s"/>`, xmlEscape(ext), xmlEscape(contentType))
	return strings.Replace(contentTypes, "</Types>", defaultXML+"</Types>", 1)
}

func defaultRelationshipsXML() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`)
}

func appendRelationship(relsXML, id, relType, target string) string {
	if strings.Contains(relsXML, `Id="`+id+`"`) {
		return relsXML
	}
	relationship := fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="%s"/>`, xmlEscape(id), xmlEscape(relType), xmlEscape(target))
	return strings.Replace(relsXML, "</Relationships>", relationship+"</Relationships>", 1)
}

func cNvPrName(block string) string {
	match := cNvPrNameRegexp.FindStringSubmatch(block)
	if len(match) != 2 {
		return ""
	}
	return html.UnescapeString(match[1])
}

func hexNoHash(value string) string {
	return strings.ToUpper(strings.TrimPrefix(defaultString(value, "#000000"), "#"))
}

func xmlEscape(value string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(value)); err != nil {
		return value
	}
	return buf.String()
}

func emuFromPx(value, pxPerInch float64) int64 {
	if pxPerInch <= 0 {
		pxPerInch = 96
	}
	return int64(math.Round(value * 914400 / pxPerInch))
}
