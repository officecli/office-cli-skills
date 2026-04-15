package officegen

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

type Report struct {
	Title          string          `json:"title"`
	Subtitle       string          `json:"subtitle,omitempty"`
	Language       string          `json:"language,omitempty"`
	Audience       string          `json:"audience,omitempty"`
	Summary        string          `json:"summary,omitempty"`
	UpdatedAt      string          `json:"updatedAt,omitempty"`
	Theme          ReportTheme     `json:"theme,omitempty"`
	KPIs           []ReportKPI     `json:"kpis,omitempty"`
	Findings       []string        `json:"findings,omitempty"`
	Sections       []ReportSection `json:"sections,omitempty"`
	AppendixTables []ReportTable   `json:"appendixTables,omitempty"`
}

type ReportTheme struct {
	Name         string `json:"name,omitempty"`
	AccentColor  string `json:"accentColor,omitempty"`
	AccentSoft   string `json:"accentSoft,omitempty"`
	TextColor    string `json:"textColor,omitempty"`
	MutedColor   string `json:"mutedColor,omitempty"`
	Background   string `json:"background,omitempty"`
	SurfaceColor string `json:"surfaceColor,omitempty"`
	BorderColor  string `json:"borderColor,omitempty"`
}

type ReportKPI struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Change string `json:"change,omitempty"`
	Note   string `json:"note,omitempty"`
}

type ReportSection struct {
	Title     string        `json:"title"`
	Subtitle  string        `json:"subtitle,omitempty"`
	Narrative []string      `json:"narrative,omitempty"`
	Takeaways []string      `json:"takeaways,omitempty"`
	Charts    []ReportChart `json:"charts,omitempty"`
	Table     *ReportTable  `json:"table,omitempty"`
}

type ReportChart struct {
	Type       string        `json:"type"`
	Title      string        `json:"title"`
	Subtitle   string        `json:"subtitle,omitempty"`
	Categories []string      `json:"categories,omitempty"`
	Series     []ChartSeries `json:"series"`
	Unit       string        `json:"unit,omitempty"`
	Source     string        `json:"source,omitempty"`
	Notes      []string      `json:"notes,omitempty"`
}

type ChartSeries struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

type ReportTable struct {
	Title   string     `json:"title,omitempty"`
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

func BuildReport(report Report) ([]byte, error) {
	report = NormalizeReport(report)
	if strings.TrimSpace(report.Title) == "" {
		return nil, fmt.Errorf("report title cannot be empty")
	}
	if len(report.Sections) == 0 {
		return nil, fmt.Errorf("report sections cannot be empty")
	}

	chartSnippets := make([]string, 0)
	chartIndex := 0
	var sectionHTML strings.Builder
	for _, section := range report.Sections {
		sectionHTML.WriteString(renderHTMLSection(section, &chartIndex, &chartSnippets))
	}

	var appendixHTML strings.Builder
	for _, table := range report.AppendixTables {
		appendixHTML.WriteString(renderHTMLTable(table))
	}

	script := strings.Join(chartSnippets, "\n")
	if script == "" {
		script = "window.__officecliCharts = true;"
	}

	htmlDoc := fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s</title>
  <link rel="preconnect" href="https://cdn.jsdelivr.net" crossorigin>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@tabler/core@1.0.0-beta20/dist/css/tabler.min.css">
  <style>
    :root {
      --report-bg: %s;
      --report-surface: %s;
      --report-border: %s;
      --report-text: %s;
      --report-muted: %s;
      --report-accent: %s;
      --report-accent-soft: %s;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: var(--report-text);
      background:
        radial-gradient(circle at top left, rgba(37, 99, 235, 0.10), transparent 28%%),
        linear-gradient(180deg, #f8fbff 0%%, var(--report-bg) 42%%, #eef4ff 100%%);
    }
    .report-shell { max-width: 1220px; margin: 0 auto; padding: 32px 20px 56px; }
    .hero {
      background: linear-gradient(135deg, #0f172a 0%%, #16233f 55%%, #1d4ed8 100%%);
      color: #f8fafc;
      border-radius: 28px;
      padding: 28px;
      box-shadow: 0 24px 80px rgba(15, 23, 42, 0.22);
    }
    .hero-meta {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      margin-bottom: 16px;
    }
    .hero-chip {
      display: inline-flex;
      align-items: center;
      padding: 6px 10px;
      border-radius: 999px;
      background: rgba(255,255,255,0.12);
      color: rgba(248,250,252,0.92);
      font-size: 12px;
      letter-spacing: 0.02em;
      text-transform: uppercase;
    }
    .hero h1 { margin: 0 0 10px; font-size: clamp(32px, 4vw, 52px); line-height: 1.05; }
    .hero .subtitle { margin: 0; max-width: 760px; color: rgba(226,232,240,0.90); font-size: 18px; line-height: 1.6; }
    .summary-grid {
      display: grid;
      grid-template-columns: minmax(0, 1.35fr) minmax(280px, 0.65fr);
      gap: 20px;
      margin-top: 24px;
    }
    .surface {
      background: rgba(255,255,255,0.88);
      border: 1px solid rgba(255,255,255,0.14);
      border-radius: 22px;
      backdrop-filter: blur(10px);
    }
    .summary-card, .findings-card {
      padding: 22px;
      background: var(--report-surface);
      border: 1px solid var(--report-border);
    }
    .eyebrow {
      margin: 0 0 10px;
      color: var(--report-accent);
      font-size: 12px;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }
    .summary-card p, .section-copy p, .appendix-note { margin: 0; color: var(--report-muted); line-height: 1.7; }
    .findings-list {
      margin: 0;
      padding-left: 18px;
      display: grid;
      gap: 10px;
      color: var(--report-text);
    }
    .kpi-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      gap: 16px;
      margin-top: 24px;
    }
    .kpi-card {
      padding: 20px;
      background: var(--report-surface);
      border: 1px solid var(--report-border);
      border-radius: 20px;
      box-shadow: 0 20px 50px rgba(15, 23, 42, 0.06);
    }
    .kpi-label { color: var(--report-muted); font-size: 13px; text-transform: uppercase; letter-spacing: 0.04em; }
    .kpi-value { margin-top: 8px; font-size: 32px; font-weight: 700; line-height: 1.1; }
    .kpi-change { margin-top: 8px; color: var(--report-accent); font-weight: 600; }
    .kpi-note { margin-top: 6px; color: var(--report-muted); font-size: 13px; }
    .section-block { margin-top: 28px; padding: 24px; background: var(--report-surface); border: 1px solid var(--report-border); border-radius: 24px; }
    .section-head { display: flex; justify-content: space-between; gap: 12px; align-items: flex-start; margin-bottom: 18px; }
    .section-head h2 { margin: 0 0 6px; font-size: 28px; line-height: 1.2; }
    .section-head p { margin: 0; color: var(--report-muted); }
    .section-grid {
      display: grid;
      grid-template-columns: minmax(0, 0.92fr) minmax(340px, 1.08fr);
      gap: 20px;
      align-items: start;
    }
    .section-copy { display: grid; gap: 14px; }
    .takeaways {
      margin: 0;
      padding-left: 18px;
      display: grid;
      gap: 8px;
    }
    .chart-stack { display: grid; gap: 18px; }
    .chart-card { border: 1px solid var(--report-border); border-radius: 20px; background: #fff; padding: 18px; }
    .chart-card h3 { margin: 0 0 6px; font-size: 18px; }
    .chart-card .chart-subtitle, .chart-source { color: var(--report-muted); font-size: 13px; }
    .chart-source { margin-top: 10px; }
    .chart-canvas { width: 100%%; min-height: 320px; height: 320px; }
    .table-card { margin-top: 18px; border: 1px solid var(--report-border); border-radius: 18px; overflow: hidden; background: #fff; }
    .table-card h3 { margin: 0; padding: 16px 18px 0; font-size: 18px; }
    .table-wrap { overflow-x: auto; padding: 12px 18px 18px; }
    table { width: 100%%; border-collapse: collapse; min-width: 560px; }
    th, td { padding: 10px 12px; border-bottom: 1px solid #e5e7eb; text-align: left; vertical-align: top; }
    th { position: sticky; top: 0; background: #f8fafc; color: #0f172a; font-size: 12px; text-transform: uppercase; letter-spacing: 0.04em; }
    .appendix { margin-top: 30px; padding: 24px; background: var(--report-surface); border: 1px solid var(--report-border); border-radius: 24px; }
    .appendix h2 { margin: 0 0 8px; }
    @media (max-width: 980px) {
      .summary-grid, .section-grid { grid-template-columns: 1fr; }
    }
    @media (max-width: 640px) {
      .report-shell { padding: 18px 14px 32px; }
      .hero, .section-block, .appendix { padding: 18px; border-radius: 20px; }
      .chart-canvas { min-height: 280px; height: 280px; }
    }
  </style>
</head>
<body>
  <main class="report-shell">
    <section class="hero">
      <div class="hero-meta">%s</div>
      <h1>%s</h1>
      <p class="subtitle">%s</p>
    </section>
    <section class="summary-grid">
      <article class="summary-card surface">
        <p class="eyebrow">Executive Summary</p>
        <p>%s</p>
      </article>
      <aside class="findings-card surface">
        <p class="eyebrow">Key Findings</p>
        <ol class="findings-list">%s</ol>
      </aside>
    </section>
    <section class="kpi-grid">%s</section>
    %s
    <section class="appendix">
      <p class="eyebrow">Appendix</p>
      <h2>Data Tables</h2>
      <p class="appendix-note">Detailed tables and supporting records are included below for traceability.</p>
      %s
    </section>
  </main>
  <script src="https://cdn.jsdelivr.net/npm/echarts@5.5.1/dist/echarts.min.js"></script>
  <script>
%s
  </script>
</body>
</html>`,
		html.EscapeString(report.Language),
		html.EscapeString(report.Title),
		report.Theme.Background,
		report.Theme.SurfaceColor,
		report.Theme.BorderColor,
		report.Theme.TextColor,
		report.Theme.MutedColor,
		report.Theme.AccentColor,
		report.Theme.AccentSoft,
		renderHeroMeta(report),
		html.EscapeString(report.Title),
		html.EscapeString(report.Subtitle),
		html.EscapeString(report.Summary),
		renderFindings(report.Findings),
		renderKPIs(report.KPIs),
		sectionHTML.String(),
		appendixHTML.String(),
		script,
	)
	return []byte(htmlDoc), nil
}

func NormalizeReport(report Report) Report {
	report.Title = strings.TrimSpace(report.Title)
	report.Subtitle = strings.TrimSpace(report.Subtitle)
	report.Language = firstNonEmpty(strings.TrimSpace(report.Language), "en")
	report.Audience = strings.TrimSpace(report.Audience)
	report.Summary = strings.TrimSpace(report.Summary)
	report.UpdatedAt = firstNonEmpty(strings.TrimSpace(report.UpdatedAt), time.Now().UTC().Format("2006-01-02"))
	report.Theme = normalizeReportTheme(report.Theme)
	report.Findings = compactStrings(report.Findings)

	kpis := make([]ReportKPI, 0, len(report.KPIs))
	for _, item := range report.KPIs {
		item.Label = strings.TrimSpace(item.Label)
		item.Value = strings.TrimSpace(item.Value)
		item.Change = strings.TrimSpace(item.Change)
		item.Note = strings.TrimSpace(item.Note)
		if item.Label == "" || item.Value == "" {
			continue
		}
		kpis = append(kpis, item)
	}
	if len(kpis) == 0 {
		kpis = []ReportKPI{
			{Label: "Coverage", Value: "4 sections", Note: "Narrative and charts are balanced."},
			{Label: "Charts", Value: "3 visuals", Note: "Built for a data-storytelling layout."},
			{Label: "Audience", Value: firstNonEmpty(report.Audience, "Business stakeholders"), Note: "Optimized for external sharing."},
		}
	}
	report.KPIs = kpis

	sections := make([]ReportSection, 0, len(report.Sections))
	for idx, section := range report.Sections {
		section.Title = firstNonEmpty(strings.TrimSpace(section.Title), fmt.Sprintf("Section %d", idx+1))
		section.Subtitle = strings.TrimSpace(section.Subtitle)
		section.Narrative = compactStrings(section.Narrative)
		section.Takeaways = compactStrings(section.Takeaways)
		charts := make([]ReportChart, 0, len(section.Charts))
		for _, chart := range section.Charts {
			if normalized, ok := normalizeReportChart(chart); ok {
				charts = append(charts, normalized)
			}
		}
		section.Charts = charts
		if section.Table != nil {
			table := normalizeReportTable(*section.Table)
			section.Table = &table
		}
		if len(section.Narrative) == 0 && len(section.Takeaways) == 0 {
			section.Narrative = []string{"This section highlights the main shift, the supporting evidence, and the implication for the next decision."}
		}
		sections = append(sections, section)
	}
	report.Sections = sections

	appendix := make([]ReportTable, 0, len(report.AppendixTables))
	for _, table := range report.AppendixTables {
		normalized := normalizeReportTable(table)
		if len(normalized.Headers) == 0 || len(normalized.Rows) == 0 {
			continue
		}
		appendix = append(appendix, normalized)
	}
	if len(appendix) == 0 && len(report.Sections) > 0 && report.Sections[0].Table != nil {
		appendix = append(appendix, *report.Sections[0].Table)
	}
	report.AppendixTables = appendix
	return report
}

func normalizeReportTheme(theme ReportTheme) ReportTheme {
	return ReportTheme{
		Name:         firstNonEmpty(strings.TrimSpace(theme.Name), "global-report"),
		AccentColor:  firstNonEmpty(strings.TrimSpace(theme.AccentColor), "#2563eb"),
		AccentSoft:   firstNonEmpty(strings.TrimSpace(theme.AccentSoft), "rgba(37, 99, 235, 0.12)"),
		TextColor:    firstNonEmpty(strings.TrimSpace(theme.TextColor), "#0f172a"),
		MutedColor:   firstNonEmpty(strings.TrimSpace(theme.MutedColor), "#475569"),
		Background:   firstNonEmpty(strings.TrimSpace(theme.Background), "#f3f7ff"),
		SurfaceColor: firstNonEmpty(strings.TrimSpace(theme.SurfaceColor), "rgba(255,255,255,0.94)"),
		BorderColor:  firstNonEmpty(strings.TrimSpace(theme.BorderColor), "rgba(148,163,184,0.22)"),
	}
}

func normalizeReportChart(chart ReportChart) (ReportChart, bool) {
	chart.Type = normalizeReportChartType(chart.Type)
	chart.Title = strings.TrimSpace(chart.Title)
	chart.Subtitle = strings.TrimSpace(chart.Subtitle)
	chart.Unit = strings.TrimSpace(chart.Unit)
	chart.Source = strings.TrimSpace(chart.Source)
	chart.Notes = compactStrings(chart.Notes)
	chart.Categories = compactStrings(chart.Categories)
	if chart.Title == "" {
		chart.Title = "Key metric trend"
	}
	series := make([]ChartSeries, 0, len(chart.Series))
	maxLen := len(chart.Categories)
	for idx, item := range chart.Series {
		item.Name = firstNonEmpty(strings.TrimSpace(item.Name), fmt.Sprintf("Series %d", idx+1))
		if len(item.Values) == 0 {
			continue
		}
		if maxLen == 0 || len(item.Values) < maxLen {
			maxLen = len(item.Values)
		}
		series = append(series, item)
	}
	if len(series) == 0 {
		return ReportChart{}, false
	}
	if maxLen <= 0 {
		return ReportChart{}, false
	}
	if len(chart.Categories) == 0 {
		categories := make([]string, maxLen)
		for i := range maxLen {
			categories[i] = fmt.Sprintf("Point %d", i+1)
		}
		chart.Categories = categories
	} else if len(chart.Categories) > maxLen {
		chart.Categories = chart.Categories[:maxLen]
	}
	for i := range series {
		if len(series[i].Values) > len(chart.Categories) {
			series[i].Values = series[i].Values[:len(chart.Categories)]
		}
	}
	chart.Series = series
	return chart, true
}

func normalizeReportTable(table ReportTable) ReportTable {
	table.Title = strings.TrimSpace(table.Title)
	table.Headers = compactStrings(table.Headers)
	rows := make([][]string, 0, len(table.Rows))
	for _, row := range table.Rows {
		next := compactStrings(row)
		if len(next) == 0 {
			continue
		}
		rows = append(rows, next)
	}
	table.Rows = rows
	return table
}

func normalizeReportChartType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "line", "area", "bar", "stacked_bar", "donut", "scatter", "waterfall":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "bar"
	}
}

func renderHeroMeta(report Report) string {
	items := []string{
		"Audience: " + firstNonEmpty(report.Audience, "Business stakeholders"),
		"Updated: " + firstNonEmpty(report.UpdatedAt, time.Now().UTC().Format("2006-01-02")),
		"Format: Report (HTML output)",
	}
	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(`<span class="hero-chip">`)
		sb.WriteString(html.EscapeString(item))
		sb.WriteString(`</span>`)
	}
	return sb.String()
}

func renderFindings(items []string) string {
	if len(items) == 0 {
		items = []string{
			"Summarize the headline movement before walking into detail.",
			"Use charts to explain scale, direction, and comparison.",
			"Close each section with a takeaway for the next action.",
		}
	}
	var sb strings.Builder
	for _, item := range items {
		sb.WriteString("<li>")
		sb.WriteString(html.EscapeString(item))
		sb.WriteString("</li>")
	}
	return sb.String()
}

func renderKPIs(items []ReportKPI) string {
	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(`<article class="kpi-card"><div class="kpi-label">`)
		sb.WriteString(html.EscapeString(item.Label))
		sb.WriteString(`</div><div class="kpi-value">`)
		sb.WriteString(html.EscapeString(item.Value))
		sb.WriteString(`</div>`)
		if item.Change != "" {
			sb.WriteString(`<div class="kpi-change">`)
			sb.WriteString(html.EscapeString(item.Change))
			sb.WriteString(`</div>`)
		}
		if item.Note != "" {
			sb.WriteString(`<div class="kpi-note">`)
			sb.WriteString(html.EscapeString(item.Note))
			sb.WriteString(`</div>`)
		}
		sb.WriteString(`</article>`)
	}
	return sb.String()
}

func renderHTMLSection(section ReportSection, chartIndex *int, snippets *[]string) string {
	var copyHTML strings.Builder
	for _, paragraph := range section.Narrative {
		copyHTML.WriteString("<p>")
		copyHTML.WriteString(html.EscapeString(paragraph))
		copyHTML.WriteString("</p>")
	}
	if len(section.Takeaways) > 0 {
		copyHTML.WriteString(`<div><p class="eyebrow">Takeaways</p><ul class="takeaways">`)
		for _, item := range section.Takeaways {
			copyHTML.WriteString("<li>")
			copyHTML.WriteString(html.EscapeString(item))
			copyHTML.WriteString("</li>")
		}
		copyHTML.WriteString(`</ul></div>`)
	}

	var chartHTML strings.Builder
	chartHTML.WriteString(`<div class="chart-stack">`)
	for _, chart := range section.Charts {
		containerID := fmt.Sprintf("chart-%d", *chartIndex)
		*chartIndex++
		chartHTML.WriteString(`<article class="chart-card"><h3>`)
		chartHTML.WriteString(html.EscapeString(chart.Title))
		chartHTML.WriteString(`</h3>`)
		if chart.Subtitle != "" {
			chartHTML.WriteString(`<p class="chart-subtitle">`)
			chartHTML.WriteString(html.EscapeString(chart.Subtitle))
			chartHTML.WriteString(`</p>`)
		}
		chartHTML.WriteString(`<div id="`)
		chartHTML.WriteString(containerID)
		chartHTML.WriteString(`" class="chart-canvas"></div>`)
		if chart.Source != "" {
			chartHTML.WriteString(`<div class="chart-source">Source: `)
			chartHTML.WriteString(html.EscapeString(chart.Source))
			chartHTML.WriteString(`</div>`)
		}
		chartHTML.WriteString(`</article>`)
		*snippets = append(*snippets, renderChartScript(containerID, chart))
	}
	chartHTML.WriteString(`</div>`)

	var tableHTML string
	if section.Table != nil {
		tableHTML = renderHTMLTable(*section.Table)
	}

	return fmt.Sprintf(`<section class="section-block">
  <div class="section-head">
    <div>
      <p class="eyebrow">Analysis Section</p>
      <h2>%s</h2>
      <p>%s</p>
    </div>
  </div>
  <div class="section-grid">
    <div class="section-copy">%s</div>
    <div>%s%s</div>
  </div>
</section>`,
		html.EscapeString(section.Title),
		html.EscapeString(section.Subtitle),
		copyHTML.String(),
		chartHTML.String(),
		tableHTML,
	)
}

func renderHTMLTable(table ReportTable) string {
	if len(table.Headers) == 0 || len(table.Rows) == 0 {
		return ""
	}
	var headerHTML strings.Builder
	for _, cell := range table.Headers {
		headerHTML.WriteString("<th>")
		headerHTML.WriteString(html.EscapeString(cell))
		headerHTML.WriteString("</th>")
	}
	var bodyHTML strings.Builder
	for _, row := range table.Rows {
		bodyHTML.WriteString("<tr>")
		for _, cell := range row {
			bodyHTML.WriteString("<td>")
			bodyHTML.WriteString(html.EscapeString(cell))
			bodyHTML.WriteString("</td>")
		}
		bodyHTML.WriteString("</tr>")
	}
	title := firstNonEmpty(table.Title, "Supporting table")
	return fmt.Sprintf(`<section class="table-card"><h3>%s</h3><div class="table-wrap"><table><thead><tr>%s</tr></thead><tbody>%s</tbody></table></div></section>`,
		html.EscapeString(title), headerHTML.String(), bodyHTML.String(),
	)
}

func renderChartScript(containerID string, chart ReportChart) string {
	payload := map[string]any{
		"type":       chart.Type,
		"title":      chart.Title,
		"categories": chart.Categories,
		"series":     chart.Series,
		"unit":       chart.Unit,
	}
	blob, _ := json.Marshal(payload)
	return fmt.Sprintf(`(function () {
  var root = document.getElementById(%q);
  if (!root || typeof echarts === "undefined") return;
  var spec = %s;
  var manyPoints = Array.isArray(spec.categories) && spec.categories.length > 18;
  var renderer = (spec.type === "scatter" || manyPoints) ? "canvas" : "svg";
  var chart = echarts.init(root, null, {renderer: renderer});
  var colors = ["#2563eb", "#0f766e", "#ea580c", "#7c3aed", "#dc2626"];
  var series = [];
  for (var i = 0; i < spec.series.length; i++) {
    var item = spec.series[i];
    var base = {
      name: item.name,
      data: item.values,
      emphasis: {focus: "series"}
    };
    switch (spec.type) {
      case "line":
        base.type = "line";
        base.smooth = true;
        series.push(base);
        break;
      case "area":
        base.type = "line";
        base.smooth = true;
        base.areaStyle = {};
        series.push(base);
        break;
      case "donut":
        base.type = "pie";
        base.radius = ["48%%", "72%%"];
        base.data = [];
        for (var j = 0; j < spec.categories.length; j++) {
          base.data.push({name: spec.categories[j], value: item.values[j]});
        }
        series.push(base);
        break;
      case "scatter":
        base.type = "scatter";
        base.data = [];
        for (var j = 0; j < spec.categories.length; j++) {
          base.data.push([j + 1, item.values[j]]);
        }
        series.push(base);
        break;
      default:
        base.type = "bar";
        if (spec.type === "stacked_bar") {
          base.stack = "total";
        }
        series.push(base);
        break;
    }
  }
  var option = {
    color: colors,
    grid: {left: 32, right: 16, top: 36, bottom: 40, containLabel: true},
    tooltip: {trigger: spec.type === "donut" ? "item" : "axis"},
    legend: {top: 0},
    xAxis: spec.type === "donut" ? undefined : {
      type: spec.type === "scatter" ? "value" : "category",
      data: spec.type === "scatter" ? undefined : spec.categories,
      axisLabel: {color: "#64748b"}
    },
    yAxis: (spec.type === "donut") ? undefined : {
      type: "value",
      axisLabel: {
        color: "#64748b",
        formatter: function (value) {
          return spec.unit ? String(value) + " " + spec.unit : String(value);
        }
      },
      splitLine: {lineStyle: {color: "#e2e8f0"}}
    },
    series: series
  };
  chart.setOption(option);
  window.addEventListener("resize", function () { chart.resize(); });
})();`, containerID, safeJS(blob))
}

func safeJS(blob []byte) string {
	replacer := strings.NewReplacer("<", "\\u003c", ">", "\\u003e", "&", "\\u0026")
	return replacer.Replace(string(blob))
}

func compactStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func BuildReportFromWorkbook(title string, sheets []XlsxSheet) Report {
	report := Report{
		Title:     firstNonEmpty(title, "Workbook Report"),
		Subtitle:  "A narrative report generated from workbook data.",
		Language:  "en",
		Audience:  "Business stakeholders",
		Summary:   "This report converts workbook data into an executive-friendly narrative with headline metrics, chart sections, and appendix tables.",
		UpdatedAt: time.Now().UTC().Format("2006-01-02"),
		KPIs: []ReportKPI{
			{Label: "Worksheets", Value: fmt.Sprintf("%d", len(sheets)), Note: "Each worksheet can map into one analysis section."},
			{Label: "Output", Value: "HTML", Note: "Generated as a local shareable report file."},
			{Label: "Charts", Value: "ECharts", Note: "Loaded from CDN for global users."},
		},
	}
	sections := make([]ReportSection, 0, len(sheets))
	appendix := make([]ReportTable, 0, len(sheets))
	for _, sheet := range sheets {
		if len(sheet.Rows) == 0 {
			continue
		}
		headers := append([]string(nil), sheet.Rows[0]...)
		rows := make([][]string, 0, maxInt(0, len(sheet.Rows)-1))
		for _, row := range sheet.Rows[1:] {
			rows = append(rows, append([]string(nil), row...))
		}
		table := normalizeReportTable(ReportTable{Title: sheet.Name, Headers: headers, Rows: rows})
		appendix = append(appendix, table)
		section := ReportSection{
			Title:     firstNonEmpty(sheet.Name, "Worksheet"),
			Subtitle:  "Key rows from the workbook are summarized below.",
			Narrative: []string{"The original workbook has been transformed into a shareable data report while preserving the supporting table in the appendix."},
			Table:     &table,
		}
		if chart, ok := buildWorkbookChart(table); ok {
			section.Charts = []ReportChart{chart}
		}
		sections = append(sections, section)
	}
	report.Sections = sections
	report.AppendixTables = appendix
	return NormalizeReport(report)
}

func buildWorkbookChart(table ReportTable) (ReportChart, bool) {
	if len(table.Headers) < 2 || len(table.Rows) == 0 {
		return ReportChart{}, false
	}
	values := make([]float64, 0, len(table.Rows))
	categories := make([]string, 0, len(table.Rows))
	for _, row := range table.Rows {
		if len(row) < 2 {
			continue
		}
		var value float64
		if _, err := fmt.Sscanf(strings.TrimSpace(row[1]), "%f", &value); err != nil {
			continue
		}
		categories = append(categories, row[0])
		values = append(values, value)
	}
	if len(values) < 2 {
		return ReportChart{}, false
	}
	return ReportChart{
		Type:       "bar",
		Title:      firstNonEmpty(table.Title, "Worksheet summary"),
		Categories: categories,
		Series:     []ChartSeries{{Name: table.Headers[1], Values: values}},
	}, true
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func SortedChartTypes() []string {
	items := []string{"area", "bar", "donut", "line", "scatter", "stacked_bar", "waterfall"}
	sort.Strings(items)
	return items
}
