package officegen

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

func BuildDOCXPreviewJSON(spec DocxDocumentSpec, style string) ([]byte, error) {
	spec = normalizeDocxSpec(spec)
	payload := map[string]any{
		"title":    spec.Title,
		"subtitle": spec.Subtitle,
		"theme":    ResolveDocumentTheme(style, spec.Theme),
		"blocks":   spec.Blocks,
	}
	return json.MarshalIndent(payload, "", "  ")
}

func BuildDOCXPreviewHTML(spec DocxDocumentSpec, style string) []byte {
	spec = normalizeDocxSpec(spec)
	theme := ResolveDocumentTheme(style, spec.Theme)
	var blocks strings.Builder
	for _, block := range spec.Blocks {
		switch block.Type {
		case "heading":
			level := block.Level
			if level < 1 {
				level = 1
			}
			if level > 4 {
				level = 4
			}
			blocks.WriteString(fmt.Sprintf("<h%d>%s</h%d>", level+1, html.EscapeString(block.Text), level+1))
		case "bullets", "numbered_list":
			tag := "ul"
			if block.Type == "numbered_list" {
				tag = "ol"
			}
			blocks.WriteString("<" + tag + ">")
			for _, item := range block.Items {
				blocks.WriteString(fmt.Sprintf("<li>%s</li>", html.EscapeString(item)))
			}
			blocks.WriteString("</" + tag + ">")
		case "quote":
			blocks.WriteString(fmt.Sprintf("<blockquote>%s</blockquote>", html.EscapeString(block.Text)))
		case "callout":
			blocks.WriteString("<section class=\"callout\">")
			if block.Title != "" {
				blocks.WriteString(fmt.Sprintf("<h3>%s</h3>", html.EscapeString(block.Title)))
			}
			blocks.WriteString(fmt.Sprintf("<p>%s</p></section>", html.EscapeString(block.Text)))
		case "table":
			if block.Title != "" {
				blocks.WriteString(fmt.Sprintf("<h3>%s</h3>", html.EscapeString(block.Title)))
			}
			blocks.WriteString("<div class=\"table-wrap\"><table><thead><tr>")
			for _, col := range block.Columns {
				blocks.WriteString(fmt.Sprintf("<th>%s</th>", html.EscapeString(col)))
			}
			blocks.WriteString("</tr></thead><tbody>")
			for _, row := range block.Rows {
				blocks.WriteString("<tr>")
				for _, cell := range row {
					blocks.WriteString(fmt.Sprintf("<td>%s</td>", html.EscapeString(cell)))
				}
				blocks.WriteString("</tr>")
			}
			blocks.WriteString("</tbody></table></div>")
		case "divider":
			blocks.WriteString("<hr>")
		default:
			blocks.WriteString(fmt.Sprintf("<p>%s</p>", html.EscapeString(block.Text)))
		}
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
      background: linear-gradient(180deg, #ffffff 0%%, var(--bg) 100%%);
      color: var(--text);
      padding: 32px 16px;
    }
    .shell {
      max-width: 920px;
      margin: 0 auto;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 24px;
      padding: 40px 44px;
      box-shadow: 0 24px 64px rgba(15, 23, 42, 0.08);
    }
    .eyebrow {
      margin: 0;
      color: var(--accent);
      text-transform: uppercase;
      letter-spacing: 0.08em;
      font-size: 12px;
      font-weight: 700;
    }
    h1 { margin: 8px 0 10px; font-size: 40px; line-height: 1.05; }
    .subtitle { margin: 0 0 28px; color: var(--muted); font-size: 18px; line-height: 1.6; }
    h2, h3, h4, h5 { margin: 28px 0 12px; color: var(--text); }
    p, li, td, th, blockquote { line-height: 1.75; }
    .callout {
      margin: 18px 0;
      padding: 18px 20px;
      background: var(--accent-soft);
      border: 1px solid var(--border);
      border-left: 6px solid var(--accent);
      border-radius: 18px;
    }
    .callout h3 { margin-top: 0; }
    blockquote {
      margin: 18px 0;
      padding: 0 0 0 18px;
      border-left: 4px solid var(--accent);
      color: var(--muted);
      font-style: italic;
    }
    hr { border: 0; border-top: 1px solid var(--border); margin: 24px 0; }
    .table-wrap { overflow-x: auto; margin: 14px 0 22px; }
    table { width: 100%%; border-collapse: collapse; min-width: 480px; }
    th, td { padding: 10px 12px; border: 1px solid var(--border); text-align: left; }
    th { background: var(--accent-soft); }
  </style>
</head>
<body>
  <main class="shell">
    <p class="eyebrow">DOCX Preview</p>
    <h1>%s</h1>
    <p class="subtitle">%s</p>
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
		html.EscapeString(spec.Title),
		html.EscapeString(spec.Subtitle),
		blocks.String(),
	)
	return []byte(doc)
}
