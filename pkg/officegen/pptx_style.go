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
	case StylePresetTrainingManual:
		return PPTXStylePreset{
			ID:                StylePresetTrainingManual,
			TitleAlign:        "l",
			TitleAccentShape:  "rect",
			ContentCardFill:   "F8FAFC",
			ContentCardAlpha:  98000,
			SectionBadgeFill:  "2563EB",
			FooterLineColor:   "2563EB",
			BackgroundOverlay: "EFF6FF",
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
		return documentThemeToSlideTheme(DefaultDocumentTheme(DocumentPresetExecutive))
	case StylePresetEditorialLight:
		return documentThemeToSlideTheme(DefaultDocumentTheme(DocumentPresetEditorial))
	case StylePresetTrainingManual:
		return documentThemeToSlideTheme(DefaultDocumentTheme(DocumentPresetTraining))
	default:
		return documentThemeToSlideTheme(DefaultDocumentTheme(DocumentPresetAnalysis))
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
		role := strings.TrimSpace(slide.NarrativeRole)
		if role == "" {
			role = strings.TrimSpace(slide.Role)
		}
		if role == "" {
			role = "detail"
		}
		slideCards.WriteString(fmt.Sprintf("<section class=\"slide\"><div class=\"slide-no\">%02d</div><h2>%s</h2><p class=\"meta\">role=%s / layout=%s / variant=%s</p>%s</section>", idx+1, html.EscapeString(slide.Title), html.EscapeString(role), html.EscapeString(resolvedLayout(slide)), html.EscapeString(strings.TrimSpace(slide.Variant)), body.String()))
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
