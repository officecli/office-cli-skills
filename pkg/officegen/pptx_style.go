package officegen

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

const (
	StylePresetExecutiveDark  = "executive-dark"
	StylePresetEditorialLight = "editorial-light"
	StylePresetExplainerVoxel = "explainer-voxel-light"
	StylePresetInvestorWarm   = "investor-warm"
	StylePresetProjectForest  = "project-forest"
	StylePresetReviewCopper   = "review-copper"
	StylePresetSlateSerif     = "collaboration-slate"
	StylePresetTechContrast   = "tech-contrast"
	StylePresetTrainingManual = "training-manual"
)

type PPTXStylePreset struct {
	ID                string
	TitleAlign        string
	TitleAccentShape  string
	ContentCardFill   string
	ContentCardAlpha  int
	SectionBadgeFill  string
	FooterLineColor   string
	BackgroundOverlay string
}

func NormalizeStylePreset(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case StylePresetExecutiveDark:
		return StylePresetExecutiveDark
	case StylePresetEditorialLight:
		return StylePresetEditorialLight
	case StylePresetExplainerVoxel:
		return StylePresetExplainerVoxel
	case StylePresetInvestorWarm:
		return StylePresetInvestorWarm
	case StylePresetProjectForest:
		return StylePresetProjectForest
	case StylePresetReviewCopper:
		return StylePresetReviewCopper
	case StylePresetSlateSerif:
		return StylePresetSlateSerif
	case StylePresetTrainingManual:
		return StylePresetTrainingManual
	default:
		return StylePresetTechContrast
	}
}

func ResolveStylePreset(value string) PPTXStylePreset {
	switch NormalizeStylePreset(value) {
	case StylePresetExecutiveDark:
		return PPTXStylePreset{
			ID:                StylePresetExecutiveDark,
			TitleAlign:        "l",
			TitleAccentShape:  "rect",
			ContentCardFill:   "111827",
			ContentCardAlpha:  9000,
			SectionBadgeFill:  "F97316",
			FooterLineColor:   "F97316",
			BackgroundOverlay: "111827",
		}
	case StylePresetEditorialLight:
		return PPTXStylePreset{
			ID:                StylePresetEditorialLight,
			TitleAlign:        "l",
			TitleAccentShape:  "line",
			ContentCardFill:   "FFF8EF",
			ContentCardAlpha:  98000,
			SectionBadgeFill:  "D97706",
			FooterLineColor:   "D6D3D1",
			BackgroundOverlay: "F8F5EF",
		}
	case StylePresetExplainerVoxel:
		return PPTXStylePreset{
			ID:                StylePresetExplainerVoxel,
			TitleAlign:        "l",
			TitleAccentShape:  "line",
			ContentCardFill:   "FFFFFF",
			ContentCardAlpha:  96000,
			SectionBadgeFill:  "3A7D44",
			FooterLineColor:   "D8E1E8",
			BackgroundOverlay: "F4F1E8",
		}
	case StylePresetInvestorWarm:
		return PPTXStylePreset{
			ID:                StylePresetInvestorWarm,
			TitleAlign:        "l",
			TitleAccentShape:  "line",
			ContentCardFill:   "FFF8F0",
			ContentCardAlpha:  98500,
			SectionBadgeFill:  "C96C3A",
			FooterLineColor:   "D8C7B6",
			BackgroundOverlay: "FBF4EB",
		}
	case StylePresetProjectForest:
		return PPTXStylePreset{
			ID:                StylePresetProjectForest,
			TitleAlign:        "l",
			TitleAccentShape:  "rect",
			ContentCardFill:   "F4F8F1",
			ContentCardAlpha:  98500,
			SectionBadgeFill:  "4D7C0F",
			FooterLineColor:   "D7E2D1",
			BackgroundOverlay: "EEF5EA",
		}
	case StylePresetReviewCopper:
		return PPTXStylePreset{
			ID:                StylePresetReviewCopper,
			TitleAlign:        "l",
			TitleAccentShape:  "line",
			ContentCardFill:   "FFF7F1",
			ContentCardAlpha:  98500,
			SectionBadgeFill:  "9A3412",
			FooterLineColor:   "E7D4C7",
			BackgroundOverlay: "FAF1EA",
		}
	case StylePresetSlateSerif:
		return PPTXStylePreset{
			ID:                StylePresetSlateSerif,
			TitleAlign:        "l",
			TitleAccentShape:  "rect",
			ContentCardFill:   "FFFFFF",
			ContentCardAlpha:  98500,
			SectionBadgeFill:  "2563EB",
			FooterLineColor:   "D6DEE8",
			BackgroundOverlay: "F6F8FB",
		}
	case StylePresetTrainingManual:
		return PPTXStylePreset{
			ID:                StylePresetTrainingManual,
			TitleAlign:        "l",
			TitleAccentShape:  "rect",
			ContentCardFill:   "FFFDF6",
			ContentCardAlpha:  98000,
			SectionBadgeFill:  "0F766E",
			FooterLineColor:   "D7E5DE",
			BackgroundOverlay: "F4F8F3",
		}
	default:
		return PPTXStylePreset{
			ID:                StylePresetTechContrast,
			TitleAlign:        "l",
			TitleAccentShape:  "rect",
			ContentCardFill:   "F8FAFC",
			ContentCardAlpha:  98000,
			SectionBadgeFill:  "1D4ED8",
			FooterLineColor:   "CBD5E1",
			BackgroundOverlay: "E2E8F0",
		}
	}
}

func DefaultThemeForPreset(preset string) *SlideTheme {
	switch NormalizeStylePreset(preset) {
	case StylePresetExecutiveDark:
		return &SlideTheme{
			PrimaryColor:   "1F2937",
			AccentColor:    "F97316",
			HighlightColor: "FDBA74",
			BackgroundType: "dark",
			BgColor1:       "0B1020",
			BgColor2:       "111827",
			TextColor:      "E5E7EB",
			TitleTextColor: "FFFFFF",
			FontFamily:     "Aptos",
			EAFontFamily:   "Microsoft YaHei",
		}
	case StylePresetEditorialLight:
		return &SlideTheme{
			PrimaryColor:   "7C2D12",
			AccentColor:    "D97706",
			HighlightColor: "B45309",
			BackgroundType: "solid",
			BgColor1:       "F8F5EF",
			BgColor2:       "F8F5EF",
			TextColor:      "292524",
			TitleTextColor: "1C1917",
			FontFamily:     "Georgia",
			EAFontFamily:   "Noto Serif CJK SC",
		}
	case StylePresetExplainerVoxel:
		return &SlideTheme{
			PrimaryColor:   "16324F",
			AccentColor:    "3A7D44",
			HighlightColor: "D9A441",
			BackgroundType: "solid",
			BgColor1:       "F4F1E8",
			BgColor2:       "F4F1E8",
			TextColor:      "1F2933",
			TitleTextColor: "16324F",
			FontFamily:     "Aptos",
			EAFontFamily:   "Noto Sans CJK SC",
		}
	case StylePresetInvestorWarm:
		return &SlideTheme{
			PrimaryColor:   "6E2F1D",
			AccentColor:    "C96C3A",
			HighlightColor: "D9A441",
			BackgroundType: "solid",
			BgColor1:       "FBF4EB",
			BgColor2:       "FBF4EB",
			TextColor:      "2F241B",
			TitleTextColor: "5B2419",
			FontFamily:     "Georgia",
			EAFontFamily:   "Noto Serif CJK SC",
		}
	case StylePresetProjectForest:
		return &SlideTheme{
			PrimaryColor:   "2F4F3E",
			AccentColor:    "4D7C0F",
			HighlightColor: "B45309",
			BackgroundType: "solid",
			BgColor1:       "EEF5EA",
			BgColor2:       "EEF5EA",
			TextColor:      "23312A",
			TitleTextColor: "254234",
			FontFamily:     "Aptos",
			EAFontFamily:   "Noto Sans CJK SC",
		}
	case StylePresetReviewCopper:
		return &SlideTheme{
			PrimaryColor:   "7C2D12",
			AccentColor:    "9A3412",
			HighlightColor: "A16207",
			BackgroundType: "solid",
			BgColor1:       "FAF1EA",
			BgColor2:       "FAF1EA",
			TextColor:      "3A2A22",
			TitleTextColor: "5E2415",
			FontFamily:     "Georgia",
			EAFontFamily:   "Noto Serif CJK SC",
		}
	case StylePresetSlateSerif:
		return &SlideTheme{
			PrimaryColor:   "111827",
			AccentColor:    "2563EB",
			HighlightColor: "C2410C",
			BackgroundType: "solid",
			BgColor1:       "F6F8FB",
			BgColor2:       "F6F8FB",
			TextColor:      "1E293B",
			TitleTextColor: "0F172A",
			FontFamily:     "Aptos",
			EAFontFamily:   "Noto Sans CJK SC",
		}
	case StylePresetTrainingManual:
		return &SlideTheme{
			PrimaryColor:   "14532D",
			AccentColor:    "0F766E",
			HighlightColor: "B45309",
			BackgroundType: "solid",
			BgColor1:       "F4F8F3",
			BgColor2:       "F4F8F3",
			TextColor:      "1F2937",
			TitleTextColor: "14532D",
			FontFamily:     "Aptos",
			EAFontFamily:   "Noto Sans CJK SC",
		}
	default:
		return &SlideTheme{
			PrimaryColor:   "0F172A",
			AccentColor:    "2563EB",
			HighlightColor: "0F766E",
			BackgroundType: "gradient",
			BgColor1:       "F8FAFC",
			BgColor2:       "EEF2FF",
			TextColor:      "0F172A",
			TitleTextColor: "020617",
			FontFamily:     "Aptos",
			EAFontFamily:   "Microsoft YaHei",
		}
	}
}

func MergeThemeWithPreset(theme *SlideTheme, preset string) *SlideTheme {
	base := DefaultThemeForPreset(preset)
	if theme == nil {
		return getTheme(base)
	}
	merged := *theme
	if strings.TrimSpace(merged.PrimaryColor) == "" {
		merged.PrimaryColor = base.PrimaryColor
	}
	if strings.TrimSpace(merged.AccentColor) == "" {
		merged.AccentColor = base.AccentColor
	}
	if strings.TrimSpace(merged.HighlightColor) == "" {
		merged.HighlightColor = base.HighlightColor
	}
	if strings.TrimSpace(merged.BackgroundType) == "" {
		merged.BackgroundType = base.BackgroundType
	}
	if strings.TrimSpace(merged.BgColor1) == "" {
		merged.BgColor1 = base.BgColor1
	}
	if strings.TrimSpace(merged.BgColor2) == "" {
		merged.BgColor2 = base.BgColor2
	}
	if strings.TrimSpace(merged.TextColor) == "" {
		merged.TextColor = base.TextColor
	}
	if strings.TrimSpace(merged.TitleTextColor) == "" {
		merged.TitleTextColor = base.TitleTextColor
	}
	if strings.TrimSpace(merged.FontFamily) == "" {
		merged.FontFamily = base.FontFamily
	}
	if strings.TrimSpace(merged.EAFontFamily) == "" {
		merged.EAFontFamily = base.EAFontFamily
	}
	return getTheme(&merged)
}

func BuildLocalPreviewJSON(title, preset string, theme *SlideTheme, slides []Slide, warnings []string) ([]byte, error) {
	payload := map[string]any{
		"title":       strings.TrimSpace(title),
		"stylePreset": NormalizeStylePreset(preset),
		"theme":       theme,
		"slides":      slides,
		"warnings":    warnings,
	}
	return json.MarshalIndent(payload, "", "  ")
}

func BuildLocalPreviewHTML(title, preset string, theme *SlideTheme, slides []Slide, warnings []string) []byte {
	theme = MergeThemeWithPreset(theme, preset)
	var slideCards strings.Builder
	for idx, slide := range slides {
		var body strings.Builder
		if strings.TrimSpace(slide.Subtitle) != "" {
			body.WriteString(fmt.Sprintf("<p class=\"subtitle\">%s</p>", html.EscapeString(slide.Subtitle)))
		}
		if strings.TrimSpace(slide.NarrativeRole) != "" || strings.TrimSpace(slide.SectionTitle) != "" {
			body.WriteString(fmt.Sprintf("<p class=\"meta\">role=%s / section=%s</p>", html.EscapeString(strings.TrimSpace(slide.NarrativeRole)), html.EscapeString(strings.TrimSpace(slide.SectionTitle))))
		}
		if len(slide.Points) > 0 {
			body.WriteString("<ul>")
			for _, point := range slide.Points {
				body.WriteString(fmt.Sprintf("<li>%s</li>", html.EscapeString(point)))
			}
			body.WriteString("</ul>")
		}
		if len(slide.Sections) > 0 {
			body.WriteString("<div class=\"sections\">")
			for _, section := range slide.Sections {
				body.WriteString(fmt.Sprintf("<div class=\"section\"><strong>%s</strong><span>%s</span></div>", html.EscapeString(section.Heading), html.EscapeString(section.Detail)))
			}
			body.WriteString("</div>")
		}
		if slide.Chart != nil {
			body.WriteString(fmt.Sprintf("<p class=\"meta\">Chart: %s</p>", html.EscapeString(slide.Chart.Title)))
		}
		if len(slide.Metrics) > 0 {
			body.WriteString("<div class=\"metrics\">")
			for _, metric := range slide.Metrics {
				body.WriteString(fmt.Sprintf("<div class=\"metric\"><strong>%s</strong><span>%s</span><em>%s</em></div>", html.EscapeString(metric.Value), html.EscapeString(metric.Label), html.EscapeString(metric.Note)))
			}
			body.WriteString("</div>")
		}
		if len(slide.Visuals) > 0 {
			body.WriteString("<div class=\"sections\">")
			for _, visual := range slide.Visuals {
				text := strings.TrimSpace(visual.Caption)
				if text == "" {
					text = strings.TrimSpace(visual.Prompt)
				}
				body.WriteString(fmt.Sprintf("<div class=\"section\"><strong>%s</strong><span>%s</span></div>", html.EscapeString(visual.Label), html.EscapeString(text)))
			}
			body.WriteString("</div>")
		}
		if strings.TrimSpace(slide.Source) != "" {
			body.WriteString(fmt.Sprintf("<p class=\"source\">%s</p>", html.EscapeString(slide.Source)))
		}
		slideCards.WriteString(fmt.Sprintf("<section class=\"slide\"><div class=\"slide-no\">%02d</div><h2>%s</h2><p class=\"meta\">layout=%s / variant=%s</p>%s</section>", idx+1, html.EscapeString(slide.Title), html.EscapeString(resolvedLayout(slide)), html.EscapeString(strings.TrimSpace(slide.Variant)), body.String()))
	}
	var warningHTML string
	if len(warnings) > 0 {
		warningHTML = "<aside class=\"warnings\"><h3>Warnings</h3><ul>"
		for _, warning := range warnings {
			if strings.TrimSpace(warning) == "" {
				continue
			}
			warningHTML += fmt.Sprintf("<li>%s</li>", html.EscapeString(warning))
		}
		warningHTML += "</ul></aside>"
	}
	htmlDoc := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s Preview</title>
  <style>
    :root {
      --bg1: #%s;
      --bg2: #%s;
      --primary: #%s;
      --accent: #%s;
      --text: #%s;
      --title: #%s;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "%s", "%s", sans-serif;
      color: var(--text);
      background: linear-gradient(135deg, var(--bg1), var(--bg2));
      padding: 24px;
    }
    header { margin-bottom: 20px; }
    .deck { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 18px; }
    .slide {
      background: rgba(255,255,255,0.9);
      border: 1px solid rgba(15,23,42,0.08);
      border-radius: 18px;
      min-height: 220px;
      padding: 20px;
      box-shadow: 0 18px 50px rgba(15,23,42,0.08);
      position: relative;
    }
    .slide-no {
      position: absolute;
      top: 14px;
      right: 16px;
      color: var(--accent);
      font-weight: 700;
    }
    h1, h2 { color: var(--title); margin: 0 0 8px; }
    .meta, .subtitle, .source, em, span { display: block; color: #475569; }
    ul { margin: 10px 0 0 18px; padding: 0; }
    .sections, .metrics { display: grid; gap: 10px; margin-top: 10px; }
    .section, .metric {
      border-left: 4px solid var(--accent);
      padding-left: 10px;
      background: rgba(255,255,255,0.55);
    }
    .warnings {
      margin-top: 20px;
      background: rgba(15,23,42,0.08);
      border-radius: 16px;
      padding: 16px 18px;
    }
  </style>
</head>
<body>
  <header>
    <h1>%s</h1>
    <p>Preset: %s</p>
  </header>
  <main class="deck">%s</main>
  %s
</body>
</html>`,
		html.EscapeString(title),
		theme.BgColor1,
		theme.BgColor2,
		theme.PrimaryColor,
		theme.AccentColor,
		theme.TextColor,
		theme.TitleTextColor,
		theme.FontFamily,
		theme.EAFontFamily,
		html.EscapeString(title),
		html.EscapeString(NormalizeStylePreset(preset)),
		slideCards.String(),
		warningHTML,
	)
	return []byte(htmlDoc)
}
