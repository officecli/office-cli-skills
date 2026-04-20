package officegen

import "strings"

// DocumentTheme stores cross-format visual tokens for office documents.
type DocumentTheme struct {
	Preset          string `json:"preset,omitempty"`
	PrimaryColor    string `json:"primaryColor,omitempty"`
	AccentColor     string `json:"accentColor,omitempty"`
	AccentSoft      string `json:"accentSoft,omitempty"`
	BackgroundColor string `json:"backgroundColor,omitempty"`
	SurfaceColor    string `json:"surfaceColor,omitempty"`
	BorderColor     string `json:"borderColor,omitempty"`
	TextColor       string `json:"textColor,omitempty"`
	MutedColor      string `json:"mutedColor,omitempty"`
	TitleColor      string `json:"titleColor,omitempty"`
	FontFamily      string `json:"fontFamily,omitempty"`
	EAFontFamily    string `json:"eaFontFamily,omitempty"`
}

const (
	DocumentPresetExecutive = "executive"
	DocumentPresetEditorial = "editorial"
	DocumentPresetAnalysis  = "analysis"
	DocumentPresetTraining  = "training"
)

func NormalizeDocumentPreset(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "executive", StylePresetExecutiveDark, "board", "formal":
		return DocumentPresetExecutive
	case "editorial", StylePresetEditorialLight, "proposal":
		return DocumentPresetEditorial
	case "training", StylePresetTrainingManual, "manual":
		return DocumentPresetTraining
	default:
		return DocumentPresetAnalysis
	}
}

func DefaultDocumentTheme(preset string) DocumentTheme {
	switch NormalizeDocumentPreset(preset) {
	case DocumentPresetExecutive:
		return DocumentTheme{
			Preset:          DocumentPresetExecutive,
			PrimaryColor:    "0F172A",
			AccentColor:     "F97316",
			AccentSoft:      "FFEDD5",
			BackgroundColor: "F8FAFC",
			SurfaceColor:    "FFFFFF",
			BorderColor:     "CBD5E1",
			TextColor:       "1E293B",
			MutedColor:      "64748B",
			TitleColor:      "020617",
			FontFamily:      "Aptos",
			EAFontFamily:    "Microsoft YaHei",
		}
	case DocumentPresetEditorial:
		return DocumentTheme{
			Preset:          DocumentPresetEditorial,
			PrimaryColor:    "7C2D12",
			AccentColor:     "B45309",
			AccentSoft:      "FEF3C7",
			BackgroundColor: "FFFBF5",
			SurfaceColor:    "FFFFFF",
			BorderColor:     "E7D8C9",
			TextColor:       "292524",
			MutedColor:      "78716C",
			TitleColor:      "1C1917",
			FontFamily:      "Georgia",
			EAFontFamily:    "Noto Serif CJK SC",
		}
	case DocumentPresetTraining:
		return DocumentTheme{
			Preset:          DocumentPresetTraining,
			PrimaryColor:    "1D4ED8",
			AccentColor:     "0F172A",
			AccentSoft:      "DBEAFE",
			BackgroundColor: "F8FBFF",
			SurfaceColor:    "FFFFFF",
			BorderColor:     "BFDBFE",
			TextColor:       "0F172A",
			MutedColor:      "475569",
			TitleColor:      "0F172A",
			FontFamily:      "Aptos",
			EAFontFamily:    "Microsoft YaHei",
		}
	default:
		return DocumentTheme{
			Preset:          DocumentPresetAnalysis,
			PrimaryColor:    "1D4ED8",
			AccentColor:     "0F766E",
			AccentSoft:      "D1FAE5",
			BackgroundColor: "F8FAFC",
			SurfaceColor:    "FFFFFF",
			BorderColor:     "DCE4F2",
			TextColor:       "0F172A",
			MutedColor:      "64748B",
			TitleColor:      "020617",
			FontFamily:      "Aptos",
			EAFontFamily:    "Microsoft YaHei",
		}
	}
}

func MergeDocumentTheme(base DocumentTheme, override *DocumentTheme) DocumentTheme {
	if override == nil {
		return base
	}
	merged := base
	if strings.TrimSpace(override.Preset) != "" {
		merged.Preset = NormalizeDocumentPreset(override.Preset)
	}
	if strings.TrimSpace(override.PrimaryColor) != "" {
		merged.PrimaryColor = override.PrimaryColor
	}
	if strings.TrimSpace(override.AccentColor) != "" {
		merged.AccentColor = override.AccentColor
	}
	if strings.TrimSpace(override.AccentSoft) != "" {
		merged.AccentSoft = override.AccentSoft
	}
	if strings.TrimSpace(override.BackgroundColor) != "" {
		merged.BackgroundColor = override.BackgroundColor
	}
	if strings.TrimSpace(override.SurfaceColor) != "" {
		merged.SurfaceColor = override.SurfaceColor
	}
	if strings.TrimSpace(override.BorderColor) != "" {
		merged.BorderColor = override.BorderColor
	}
	if strings.TrimSpace(override.TextColor) != "" {
		merged.TextColor = override.TextColor
	}
	if strings.TrimSpace(override.MutedColor) != "" {
		merged.MutedColor = override.MutedColor
	}
	if strings.TrimSpace(override.TitleColor) != "" {
		merged.TitleColor = override.TitleColor
	}
	if strings.TrimSpace(override.FontFamily) != "" {
		merged.FontFamily = override.FontFamily
	}
	if strings.TrimSpace(override.EAFontFamily) != "" {
		merged.EAFontFamily = override.EAFontFamily
	}
	return merged
}

func ResolveDocumentTheme(style string, override *DocumentTheme) DocumentTheme {
	return MergeDocumentTheme(DefaultDocumentTheme(style), override)
}

func documentThemeToSlideTheme(theme DocumentTheme) *SlideTheme {
	backgroundType := "gradient"
	bgColor2 := theme.SurfaceColor
	if theme.Preset == DocumentPresetExecutive {
		backgroundType = "dark"
		bgColor2 = "111827"
	}
	if bgColor2 == "" {
		bgColor2 = theme.BackgroundColor
	}
	return &SlideTheme{
		PrimaryColor:   theme.PrimaryColor,
		AccentColor:    theme.AccentColor,
		HighlightColor: strings.TrimPrefix(theme.AccentSoft, "#"),
		BackgroundType: backgroundType,
		BgColor1:       theme.BackgroundColor,
		BgColor2:       bgColor2,
		TextColor:      theme.TextColor,
		TitleTextColor: theme.TitleColor,
		FontFamily:     theme.FontFamily,
		EAFontFamily:   theme.EAFontFamily,
	}
}

func DocumentThemeToSlideTheme(theme DocumentTheme) *SlideTheme {
	return documentThemeToSlideTheme(theme)
}

func normalizeHexColor(value, fallback string) string {
	cleaned := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(value)), "#")
	if len(cleaned) != 6 {
		return strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(fallback)), "#")
	}
	for _, ch := range cleaned {
		if (ch < '0' || ch > '9') && (ch < 'A' || ch > 'F') {
			return strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(fallback)), "#")
		}
	}
	return cleaned
}
