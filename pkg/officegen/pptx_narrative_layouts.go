package officegen

import (
	"fmt"
	"strings"
)

func slideNarrativeSections(slide Slide, limit int) []SlideSection {
	if len(slide.Sections) > 0 {
		if limit <= 0 || len(slide.Sections) <= limit {
			return slide.Sections
		}
		return slide.Sections[:limit]
	}
	out := make([]SlideSection, 0, len(slide.Points))
	for idx, point := range slide.Points {
		point = strings.TrimSpace(point)
		if point == "" {
			continue
		}
		out = append(out, SlideSection{
			Heading: fmt.Sprintf("%02d", idx+1),
			Detail:  point,
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func slideVisualCaption(visual SlideVisual, fallback string) string {
	for _, value := range []string{visual.Caption, visual.Label, fallback} {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (g *PPTXGenerator) createTOCSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	accentColor := getSafeTextColorForBg(theme.AccentColor, bgColor)
	items := slideNarrativeSections(slide, 6)

	var cards strings.Builder
	for idx, item := range items {
		y := 1500000 + idx*760000
		cardID := 20 + idx*3
		cards.WriteString(createFramedPanelXML(cardID, fmt.Sprintf("TOCCard%d", idx+1), stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, 900000, y, 10300000, 620000))
		cards.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="TOCIndex%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="1180000" y="%d"/><a:ext cx="900000" cy="320000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/><a:lstStyle/>
                    <a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1800" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="TOCText%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="2400000" y="%d"/><a:ext cx="7800000" cy="320000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="ctr"/><a:lstStyle/>
                    <a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="1800"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p>
                </p:txBody>
            </p:sp>`,
			cardID+1, idx+1, y+145000, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(firstNonEmpty(item.Heading, fmt.Sprintf("%02d", idx+1))),
			cardID+2, idx+1, y+145000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(firstNonEmpty(item.Detail, item.Heading))))
	}

	subtitleXML := generateSubtitleXML(firstNonEmpty(slide.Subtitle, "先看结构，再进入每个章节。"), 6, 900000, theme)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
            <p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="900000" y="250000"/><a:ext cx="10800000" cy="650000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody>
                    <a:bodyPr anchor="b"/><a:lstStyle/>
                    <a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="3200" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p>
                </p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="4" name="TitleDeco"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="500000" y="320000"/><a:ext cx="80000" cy="500000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr>
            </p:sp>%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(firstNonEmpty(slide.Title, "目录")), accentColor, subtitleXML, cards.String(), generateFooterXML(slide.Source, slideNum, totalSlides, 80, theme, stylePreset))
}

func (g *PPTXGenerator) createChapterSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int, assets slideRenderAssets) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	accentColor := getSafeTextColorForBg(theme.AccentColor, bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily

	number := fmt.Sprintf("%02d", maxInt(slide.SectionIndex, 1))
	imageXML := ""
	overlayXML := ""
	if assets.PrimaryImageRel != "" && len(slide.ImageData) > 0 {
		imageXML = createImagePictureXML(90, "ChapterImage", assets.PrimaryImageRel, 7200000, 0, 4992000, 6858000, slide.ImageData)
		overlayXML = createSolidOverlayXML(91, "ChapterOverlay", "000000", 18000, 7200000, 0, 4992000, 6858000)
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
            <p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="2" name="AccentBand"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="12192000" cy="6858000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:gradFill rotWithShape="1"><a:gsLst><a:gs pos="0"><a:srgbClr val="%s"/></a:gs><a:gs pos="100000"><a:srgbClr val="%s"><a:alpha val="6000"/></a:srgbClr></a:gs></a:gsLst><a:lin ang="5400000" scaled="1"/></a:gradFill><a:ln><a:noFill/></a:ln></p:spPr>
            </p:sp>%s%s
            <p:sp>
                <p:nvSpPr><p:cNvPr id="3" name="ChapterNo"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="1000000" y="1400000"/><a:ext cx="2000000" cy="900000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="3600" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="4" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="1000000" y="2450000"/><a:ext cx="5600000" cy="1000000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="3400" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="5" name="Subtitle"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="1000000" y="3650000"/><a:ext cx="5400000" cy="700000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="t"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="1800"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, theme.PrimaryColor, accentColor, imageXML, overlayXML, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(number), titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(firstNonEmpty(slide.SectionTitle, slide.Title, "章节")), textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(firstNonEmpty(slide.Subtitle, "建立上下文后，再进入核心内容。")), generateFooterXML(slide.Source, slideNum, totalSlides, 100, theme, stylePreset))
}

func (g *PPTXGenerator) createGallerySlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int, assets slideRenderAssets) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	accentColor := getSafeTextColorForBg(theme.AccentColor, bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	visuals := embeddedVisuals(slide)

	var gallery strings.Builder
	if len(visuals) > 0 && len(assets.VisualRelIDs) > 0 {
		type frame struct{ x, y, cx, cy int }
		frames := []frame{
			{x: 900000, y: 1600000, cx: 4800000, cy: 1800000},
			{x: 6100000, y: 1600000, cx: 4800000, cy: 1800000},
			{x: 900000, y: 3900000, cx: 4800000, cy: 1800000},
			{x: 6100000, y: 3900000, cx: 4800000, cy: 1800000},
		}
		namePrefix := "GalleryImage"
		captionPrefix := "GalleryCaption"
		switch strings.TrimSpace(slide.Variant) {
		case "gallery-filmstrip":
			frames = []frame{
				{x: 900000, y: 1500000, cx: 10300000, cy: 2600000},
				{x: 900000, y: 4450000, cx: 3200000, cy: 1000000},
				{x: 4300000, y: 4450000, cx: 3200000, cy: 1000000},
				{x: 7700000, y: 4450000, cx: 3200000, cy: 1000000},
			}
			namePrefix = "GalleryFilmstripImage"
			captionPrefix = "GalleryFilmstripCaption"
		case "gallery-focus":
			frames = []frame{
				{x: 900000, y: 1550000, cx: 6700000, cy: 3600000},
				{x: 8000000, y: 1850000, cx: 2300000, cy: 1600000},
				{x: 8000000, y: 3700000, cx: 2300000, cy: 1200000},
			}
			namePrefix = "GalleryFocusImage"
			captionPrefix = "GalleryFocusCaption"
		default:
			namePrefix = "GalleryDuoImage"
			captionPrefix = "GalleryDuoCaption"
		}
		count := len(visuals)
		if count > len(frames) {
			count = len(frames)
		}
		for idx := 0; idx < count; idx++ {
			box := frames[idx]
			gallery.WriteString(createImagePictureXML(40+idx*2, fmt.Sprintf("%s%d", namePrefix, idx+1), assets.VisualRelIDs[idx], box.x, box.y, box.cx, box.cy, visuals[idx].ImageData))
			caption := slideVisualCaption(visuals[idx], fmt.Sprintf("Visual %d", idx+1))
			gallery.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="%s%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="280000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="1200"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>`, 41+idx*2, captionPrefix, idx+1, box.x, box.y+box.cy+60000, box.cx, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(caption)))
		}
	}

	if gallery.Len() == 0 {
		items := slideNarrativeSections(slide, 4)
		for idx, item := range items {
			y := 1650000 + idx*930000
			gallery.WriteString(createFramedPanelXML(50+idx*2, fmt.Sprintf("GalleryFallback%d", idx+1), stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, 900000, y, 10300000, 720000))
			gallery.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="GalleryFallbackText%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="1200000" y="%d"/><a:ext cx="9800000" cy="420000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="1700"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>`, 51+idx*2, idx+1, y+180000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(firstNonEmpty(item.Detail, item.Heading))))
		}
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
            <p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="900000" y="250000"/><a:ext cx="10800000" cy="650000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="b"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="3200" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="3" name="TitleDeco"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="500000" y="320000"/><a:ext cx="80000" cy="500000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr>
            </p:sp>%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(firstNonEmpty(slide.Title, "Visual Gallery")), accentColor, generateSubtitleXML(firstNonEmpty(slide.Subtitle, "用更高的视觉密度承载同一主题。"), 4, 900000, theme), gallery.String(), generateFooterXML(slide.Source, slideNum, totalSlides, 160, theme, stylePreset))
}

func (g *PPTXGenerator) createComparisonSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	accentColor := getSafeTextColorForBg(theme.AccentColor, bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	items := slideNarrativeSections(slide, 3)
	if len(items) == 0 {
		items = []SlideSection{{Heading: "Option A", Detail: "Define the first side of the comparison"}, {Heading: "Option B", Detail: "Define the second side of the comparison"}}
	}

	var columns strings.Builder
	var introXML string
	switch strings.TrimSpace(slide.Variant) {
	case "comparison-vs-band":
		frames := []struct{ x, w int }{{x: 900000, w: 4300000}, {x: 7000000, w: 4300000}}
		introXML = `
            <p:sp>
                <p:nvSpPr><p:cNvPr id="30" name="ComparisonVSBand"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="5650000" y="2750000"/><a:ext cx="900000" cy="520000"/></a:xfrm><a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="` + accentColor + `"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1700" b="1"><a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill></a:rPr><a:t>VS</a:t></a:r></a:p></p:txBody>
            </p:sp>`
		for idx, item := range items {
			if idx >= len(frames) {
				break
			}
			frame := frames[idx]
			columns.WriteString(createFramedPanelXML(40+idx*3, fmt.Sprintf("CompareVSBandPanel%d", idx+1), stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, frame.x, 1850000, frame.w, 2500000))
			columns.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="CompareVSBandHeader%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="2160000"/><a:ext cx="%d" cy="460000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1800" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="CompareVSBandDetail%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="2800000"/><a:ext cx="%d" cy="1000000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="t" lIns="180000" rIns="180000"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="1500"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>`,
				41+idx*3, idx+1, frame.x+180000, frame.w-360000, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Heading),
				42+idx*3, idx+1, frame.x+180000, frame.w-360000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Detail)))
		}
	case "comparison-spotlight":
		frames := []struct{ x, y, w, h int }{
			{x: 900000, y: 1700000, w: 5600000, h: 3100000},
			{x: 6900000, y: 1700000, w: 3400000, h: 1450000},
			{x: 6900000, y: 3350000, w: 3400000, h: 1450000},
		}
		for idx, item := range items {
			if idx >= len(frames) {
				break
			}
			frame := frames[idx]
			columns.WriteString(createFramedPanelXML(40+idx*3, fmt.Sprintf("CompareSpotlightPanel%d", idx+1), stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, frame.x, frame.y, frame.w, frame.h))
			columns.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="CompareSpotlightHeader%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="420000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="%d" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="CompareSpotlightDetail%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="t" lIns="180000" rIns="180000"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="%d"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>`,
				41+idx*3, idx+1, frame.x+180000, frame.y+180000, frame.w-360000, map[bool]int{true:2200, false:1700}[idx == 0], accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Heading),
				42+idx*3, idx+1, frame.x+180000, frame.y+720000, frame.w-360000, frame.h-900000, map[bool]int{true:1550, false:1350}[idx == 0], textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Detail)))
		}
	default:
		frames := []struct{ x, w int }{{x: 900000, w: 4900000}, {x: 6200000, w: 4900000}, {x: 3550000, w: 4900000}}
		if len(items) >= 3 {
			frames = []struct{ x, w int }{
				{x: 700000, w: 3200000},
				{x: 4450000, w: 3200000},
				{x: 8200000, w: 3200000},
			}
		}
		for idx, item := range items {
			if idx >= len(frames) {
				break
			}
			frame := frames[idx]
			columns.WriteString(createFramedPanelXML(40+idx*3, fmt.Sprintf("ComparePanel%d", idx+1), stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, frame.x, 1700000, frame.w, 3000000))
			columns.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="CompareHeader%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="1960000"/><a:ext cx="%d" cy="460000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1800" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="CompareDetail%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="2580000"/><a:ext cx="%d" cy="1500000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="t" lIns="180000" rIns="180000"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="1600"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>`,
				41+idx*3, idx+1, frame.x+180000, frame.w-360000, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Heading),
				42+idx*3, idx+1, frame.x+180000, frame.w-360000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Detail)))
		}
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
            <p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="900000" y="250000"/><a:ext cx="10800000" cy="650000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="b"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="3200" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>%s%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title), generateSubtitleXML(firstNonEmpty(slide.Subtitle, "把核心差异并排展示，减少来回跳读。"), 6, 900000, theme), introXML, columns.String(), generateFooterXML(slide.Source, slideNum, totalSlides, 120, theme, stylePreset))
}

func (g *PPTXGenerator) createTimelineSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int, assets slideRenderAssets) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	accentColor := getSafeTextColorForBg(theme.AccentColor, bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	items := slideNarrativeSections(slide, 4)
	imagePos := resolvedImagePos(slide)
	imageXML := ""
	overlayXML := ""
	if imagePos == "background" && assets.PrimaryImageRel != "" && len(slide.ImageData) > 0 {
		imageXML = createImagePictureXML(90, "TimelineBackgroundImage", assets.PrimaryImageRel, 0, 0, 12192000, 6858000, slide.ImageData)
		overlayXML = createSolidOverlayXML(91, "TimelineOverlay", "000000", 18000, 0, 0, 12192000, 6858000)
		titleColor = "FFFFFF"
		textColor = "F8FAFC"
		accentColor = "D9A441"
	}
	if len(items) == 0 {
		items = []SlideSection{{Heading: "Stage 1", Detail: "Define the first transition"}, {Heading: "Stage 2", Detail: "Define the second transition"}, {Heading: "Stage 3", Detail: "Define the closing transition"}}
	}

	var timeline strings.Builder
	if strings.TrimSpace(slide.Variant) == "timeline-steps" {
		for idx, item := range items {
			y := 1700000 + idx*1180000
			timeline.WriteString(createFramedPanelXML(20+idx*3, fmt.Sprintf("TimelineStepsCard%d", idx+1), stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, 1300000, y, 9200000, 900000))
			timeline.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="TimelineStepsNo%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="1480000" y="%d"/><a:ext cx="560000" cy="320000"/></a:xfrm><a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1200" b="1"><a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill></a:rPr><a:t>%02d</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="TimelineStepsHead%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="2300000" y="%d"/><a:ext cx="2400000" cy="340000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="1650" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="TimelineStepsDetail%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="2300000" y="%d"/><a:ext cx="7200000" cy="360000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="1350"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>`,
				21+idx*3, idx+1, y+270000, accentColor, idx+1,
				22+idx*3, idx+1, y+180000, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Heading),
				23+idx*3, idx+1, y+460000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Detail)))
		}
	} else {
		timeline.WriteString(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="20" name="TimelineAxis"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="1300000" y="3300000"/><a:ext cx="9600000" cy="0"/></a:xfrm><a:prstGeom prst="line"><a:avLst/></a:prstGeom><a:ln w="19050"><a:solidFill><a:srgbClr val="` + accentColor + `"/></a:solidFill></a:ln></p:spPr>
            </p:sp>`)
		for idx, item := range items {
			x := 1300000
			if len(items) > 1 {
				x = 1300000 + idx*(9600000/(len(items)-1))
			}
			headY := 2300000
			detailY := 3620000
			if strings.TrimSpace(slide.Variant) == "timeline-zigzag" && idx%2 == 1 {
				headY = 3900000
				detailY = 2200000
			}
			namePrefix := "Timeline"
			if strings.TrimSpace(slide.Variant) == "timeline-zigzag" {
				namePrefix = "TimelineZigzag"
			}
			timeline.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="%sDot%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="3140000"/><a:ext cx="320000" cy="320000"/></a:xfrm><a:prstGeom prst="ellipse"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="%sHead%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="1800000" cy="420000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1500" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="%sDetail%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="1800000" cy="860000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="t"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1300"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>`,
				21+idx*3, namePrefix, idx+1, x-160000, accentColor,
				22+idx*3, namePrefix, idx+1, x-900000, headY, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Heading),
				23+idx*3, namePrefix, idx+1, x-900000, detailY, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Detail)))
		}
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
            <p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>%s%s
            <p:sp>
                <p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="900000" y="250000"/><a:ext cx="10800000" cy="650000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="b"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="3200" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, imageXML, overlayXML, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(slide.Title), generateSubtitleXML(firstNonEmpty(slide.Subtitle, "把阶段、时间和动作放到一条线上。"), 6, 900000, theme), timeline.String(), generateFooterXML(slide.Source, slideNum, totalSlides, 140, theme, stylePreset))
}

func (g *PPTXGenerator) createClosingSlideXML(slide Slide, theme *SlideTheme, stylePreset PPTXStylePreset, slideNum, totalSlides int, assets slideRenderAssets) string {
	bgXML := generateSlideBackgroundXML(slide, theme)
	bgColor := getEffectiveBgColor(slide, theme)
	titleColor := getSafeTextColorForBg(getSlideTitleColor(slide, theme), bgColor)
	textColor := getSafeTextColorForBg(getSlideTextColor(slide, theme), bgColor)
	accentColor := getSafeTextColorForBg(theme.AccentColor, bgColor)
	fontFamily := theme.FontFamily
	eaFontFamily := theme.EAFontFamily
	items := slideNarrativeSections(slide, 3)
	imagePos := resolvedImagePos(slide)
	imageXML := ""
	overlayXML := ""
	if imagePos == "background" && assets.PrimaryImageRel != "" && len(slide.ImageData) > 0 {
		imageXML = createImagePictureXML(90, "ClosingBackgroundImage", assets.PrimaryImageRel, 0, 0, 12192000, 6858000, slide.ImageData)
		overlayXML = createSolidOverlayXML(91, "ClosingOverlay", "000000", 18000, 0, 0, 12192000, 6858000)
		titleColor = "FFFFFF"
		textColor = "F8FAFC"
		accentColor = "D9A441"
	}
	if len(items) == 0 {
		items = []SlideSection{{Heading: "Now", Detail: "Confirm owner, scope, and milestone"}, {Heading: "30 Days", Detail: "Run the first validation cycle and measure the lead metric"}, {Heading: "Review", Detail: "Decide whether to expand based on evidence"}}
	}

	var cards strings.Builder
	switch strings.TrimSpace(slide.Variant) {
	case "closing-checklist":
		for idx, item := range items {
			y := 2100000 + idx*900000
			cards.WriteString(createFramedPanelXML(40+idx*3, fmt.Sprintf("ClosingChecklistItem%d", idx+1), stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, 1200000, y, 9800000, 640000))
			cards.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="ClosingChecklistTick%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="1440000" y="%d"/><a:ext cx="360000" cy="360000"/></a:xfrm><a:prstGeom prst="ellipse"><a:avLst/></a:prstGeom><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:ln><a:noFill/></a:ln></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1200" b="1"><a:solidFill><a:srgbClr val="FFFFFF"/></a:solidFill></a:rPr><a:t>✓</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="ClosingChecklistText%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="1980000" y="%d"/><a:ext cx="8500000" cy="340000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="1550"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s: %s</a:t></a:r></a:p></p:txBody>
            </p:sp>`,
				41+idx*3, idx+1, y+140000, accentColor,
				42+idx*3, idx+1, y+150000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Heading), escapeXML(item.Detail)))
		}
	case "closing-takeaway":
		cards.WriteString(createFramedPanelXML(40, "ClosingTakeawayPanel", stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, 1200000, 1900000, 9800000, 1200000))
		lead := items[0]
		cards.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="41" name="ClosingTakeawayText"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="1480000" y="2250000"/><a:ext cx="9240000" cy="520000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="2200" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>`, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(firstNonEmpty(lead.Detail, lead.Heading))))
		for idx, item := range items[1:] {
			x := 1900000 + idx*4300000
			cards.WriteString(createFramedPanelXML(50+idx*3, fmt.Sprintf("ClosingTakeawayChip%d", idx+1), stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, x, 3650000, 3200000, 900000))
			cards.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="ClosingTakeawayChipText%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="3950000"/><a:ext cx="2600000" cy="300000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1400"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>`,
				51+idx*3, idx+1, x+300000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(firstNonEmpty(item.Detail, item.Heading))))
		}
	default:
		totalWidth := 10200000
		gap := 180000
		cardWidth := (totalWidth - gap*(len(items)-1)) / len(items)
		cardName := "ClosingCard"
		if strings.TrimSpace(slide.Variant) == "closing-cards-light" {
			cardName = "ClosingCardLight"
		}
		for idx, item := range items {
			x := 960000 + idx*(cardWidth+gap)
			cards.WriteString(createFramedPanelXML(40+idx*3, fmt.Sprintf("%s%d", cardName, idx+1), stylePreset.ContentCardFill, stylePreset.ContentCardAlpha, accentColor, 12000, x, 2100000, cardWidth, 2400000))
			cards.WriteString(fmt.Sprintf(`
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="ClosingHead%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="2380000"/><a:ext cx="%d" cy="420000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="ctr"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1500" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>
            <p:sp>
                <p:nvSpPr><p:cNvPr id="%d" name="ClosingDetail%d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="%d" y="3000000"/><a:ext cx="%d" cy="900000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="t" lIns="120000" rIns="120000"/><a:lstStyle/><a:p><a:pPr algn="ctr"/><a:r><a:rPr lang="zh-CN" sz="1350"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>`,
				41+idx*3, idx+1, x+120000, cardWidth-240000, accentColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Heading),
				42+idx*3, idx+1, x+120000, cardWidth-240000, textColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(item.Detail)))
		}
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
    xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
    xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">
    <p:cSld>
%s
        <p:spTree>
            <p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>
            <p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>%s%s
            <p:sp>
                <p:nvSpPr><p:cNvPr id="2" name="Title"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>
                <p:spPr><a:xfrm><a:off x="900000" y="250000"/><a:ext cx="10800000" cy="650000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
                <p:txBody><a:bodyPr anchor="b"/><a:lstStyle/><a:p><a:pPr algn="l"/><a:r><a:rPr lang="zh-CN" sz="3200" b="1"><a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:latin typeface="%s"/><a:ea typeface="%s"/></a:rPr><a:t>%s</a:t></a:r></a:p></p:txBody>
            </p:sp>%s%s%s
        </p:spTree>
    </p:cSld>
</p:sld>`, bgXML, imageXML, overlayXML, titleColor, escapeXML(fontFamily), escapeXML(eaFontFamily), escapeXML(firstNonEmpty(slide.Title, "Next Steps")), generateSubtitleXML(firstNonEmpty(slide.Subtitle, "用少量动作收束整套叙事。"), 6, 900000, theme), cards.String(), generateFooterXML(slide.Source, slideNum, totalSlides, 150, theme, stylePreset))
}
