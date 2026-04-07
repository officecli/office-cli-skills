package officegen

import (
	"strings"
	"testing"
)

func TestGenerateThemeXML_UsesConfiguredFonts(t *testing.T) {
	xml := generateThemeXML(&SlideTheme{
		PrimaryColor: "1A73E8",
		AccentColor:  "E8710A",
		FontFamily:   "Liberation Sans",
		EAFontFamily: "Noto Sans CJK SC",
	})
	for _, needle := range []string{
		`typeface="Liberation Sans"`,
		`typeface="Noto Sans CJK SC"`,
	} {
		if !strings.Contains(xml, needle) {
			t.Fatalf("theme xml missing %q:\n%s", needle, xml)
		}
	}
}
