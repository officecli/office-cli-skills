package officegen

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

func BuildXLSXPreviewJSON(spec XlsxWorkbookSpec, style string) ([]byte, error) {
	spec = normalizeWorkbookSpec(spec)
	payload := map[string]any{
		"title":    spec.Title,
		"subtitle": spec.Subtitle,
		"theme":    ResolveDocumentTheme(style, spec.Theme),
		"sheets":   spec.Sheets,
	}
	return json.MarshalIndent(payload, "", "  ")
}

func BuildXLSXPreviewHTML(spec XlsxWorkbookSpec, style string) []byte {
	spec = normalizeWorkbookSpec(spec)
	theme := ResolveDocumentTheme(style, spec.Theme)
	var sheets strings.Builder
	for idx, sheet := range spec.Sheets {
		sheets.WriteString(fmt.Sprintf("<section class=\"sheet\"><div class=\"sheet-chip\">Sheet %02d</div><h2>%s</h2>", idx+1, html.EscapeString(sheet.Name)))
		if sheet.Purpose != "" {
			sheets.WriteString(fmt.Sprintf("<p class=\"purpose\">%s</p>", html.EscapeString(sheet.Purpose)))
		}
		if len(sheet.Summary) > 0 {
			sheets.WriteString("<div class=\"summary-grid\">")
			for _, item := range sheet.Summary {
				sheets.WriteString(fmt.Sprintf("<article class=\"summary-item\"><span>%s</span><strong>%s</strong></article>", html.EscapeString(item.Label), html.EscapeString(item.Value)))
			}
			sheets.WriteString("</div>")
		}
		sheets.WriteString("<div class=\"table-wrap\"><table><thead><tr>")
		for _, col := range sheet.Columns {
			sheets.WriteString(fmt.Sprintf("<th>%s</th>", html.EscapeString(col.Label)))
		}
		sheets.WriteString("</tr></thead><tbody>")
		for _, row := range sheet.Rows {
			sheets.WriteString("<tr>")
			for _, cell := range row {
				sheets.WriteString(fmt.Sprintf("<td>%s</td>", html.EscapeString(cell)))
			}
			sheets.WriteString("</tr>")
		}
		sheets.WriteString("</tbody></table></div></section>")
	}

	doc := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s Preview</title>
  <style>
    :root {
      --bg: #%s;
      --surface: #%s;
      --border: #%s;
      --text: #%s;
      --muted: #%s;
      --accent: #%s;
      --accent-soft: #%s;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "%s", "%s", sans-serif;
      color: var(--text);
      background: linear-gradient(180deg, #fff 0%%, var(--bg) 100%%);
      padding: 28px 16px 40px;
    }
    .shell { max-width: 1180px; margin: 0 auto; }
    .hero {
      padding: 28px 30px;
      border-radius: 26px;
      background: linear-gradient(135deg, var(--accent) 0%%, #%s 100%%);
      color: #fff;
      box-shadow: 0 24px 64px rgba(15, 23, 42, 0.12);
    }
    .hero p { margin: 0; opacity: 0.9; }
    .hero h1 { margin: 10px 0 8px; font-size: 42px; }
    .sheet {
      margin-top: 22px;
      padding: 22px;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 24px;
    }
    .sheet-chip {
      display: inline-block;
      margin-bottom: 10px;
      padding: 6px 10px;
      border-radius: 999px;
      background: var(--accent-soft);
      color: var(--accent);
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: 0.06em;
      font-weight: 700;
    }
    h2 { margin: 0 0 8px; }
    .purpose { margin: 0 0 16px; color: var(--muted); }
    .summary-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
      gap: 12px;
      margin-bottom: 18px;
    }
    .summary-item {
      padding: 14px 16px;
      border: 1px solid var(--border);
      border-radius: 18px;
      background: var(--accent-soft);
    }
    .summary-item span { display: block; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: 0.05em; }
    .summary-item strong { display: block; margin-top: 8px; font-size: 24px; }
    .table-wrap { overflow-x: auto; }
    table { width: 100%%; border-collapse: collapse; min-width: 620px; }
    th, td { padding: 10px 12px; border-bottom: 1px solid var(--border); text-align: left; }
    th { background: var(--accent-soft); text-transform: uppercase; font-size: 12px; letter-spacing: 0.05em; }
  </style>
</head>
<body>
  <main class="shell">
    <section class="hero">
      <p>XLSX Preview</p>
      <h1>%s</h1>
      <p>%s</p>
    </section>
    %s
  </main>
</body>
</html>`,
		html.EscapeString(spec.Title),
		normalizeHexColor(theme.BackgroundColor, "F8FAFC"),
		normalizeHexColor(theme.SurfaceColor, "FFFFFF"),
		normalizeHexColor(theme.BorderColor, "CBD5E1"),
		normalizeHexColor(theme.TextColor, "0F172A"),
		normalizeHexColor(theme.MutedColor, "64748B"),
		normalizeHexColor(theme.AccentColor, "1D4ED8"),
		normalizeHexColor(theme.AccentSoft, "DBEAFE"),
		html.EscapeString(theme.FontFamily),
		html.EscapeString(theme.EAFontFamily),
		normalizeHexColor(theme.PrimaryColor, "0F172A"),
		html.EscapeString(spec.Title),
		html.EscapeString(spec.Subtitle),
		sheets.String(),
	)
	return []byte(doc)
}
