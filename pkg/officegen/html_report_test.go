package officegen

import (
	"strings"
	"testing"
)

func TestBuildHTMLReport_RendersEChartsAndSectionContent(t *testing.T) {
	report := HTMLReport{
		Title:    "Q2 Business Review",
		Subtitle: "A concise view of commercial performance",
		Audience: "Board and investors",
		Summary:  "Revenue held up, but conversion quality softened in the lower funnel.",
		KPIs: []HTMLReportKPI{
			{Label: "Revenue", Value: "$12.4M", Change: "+8% QoQ"},
		},
		Findings: []string{"North America stayed ahead of plan."},
		Sections: []HTMLReportSection{
			{
				Title:     "Demand momentum",
				Narrative: []string{"Demand stayed strongest in North America while Europe remained stable."},
				Charts: []HTMLReportChart{
					{
						Type:       "line",
						Title:      "Regional revenue trend",
						Categories: []string{"Jan", "Feb", "Mar"},
						Series:     []HTMLChartSeries{{Name: "Revenue", Values: []float64{100, 114, 128}}},
						Source:     "Internal finance data",
					},
				},
			},
		},
		AppendixTables: []HTMLReportTable{
			{
				Title:   "Supporting table",
				Headers: []string{"Region", "Revenue"},
				Rows:    [][]string{{"North America", "128"}},
			},
		},
	}

	data, err := BuildHTMLReport(report)
	if err != nil {
		t.Fatalf("BuildHTMLReport: %v", err)
	}
	output := string(data)
	for _, needle := range []string{
		"https://cdn.jsdelivr.net/npm/echarts",
		"https://cdn.jsdelivr.net/npm/@tabler/core",
		"Q2 Business Review",
		"Demand momentum",
		"Regional revenue trend",
		"Supporting table",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("html missing %q:\n%s", needle, output)
		}
	}
}

func TestNormalizeHTMLReport_NormalizesUnsupportedChartType(t *testing.T) {
	report := NormalizeHTMLReport(HTMLReport{
		Title: "Test",
		Sections: []HTMLReportSection{
			{
				Title: "Section",
				Charts: []HTMLReportChart{
					{
						Type:       "bubble",
						Title:      "Chart",
						Categories: []string{"A", "B"},
						Series:     []HTMLChartSeries{{Name: "Series 1", Values: []float64{1, 2}}},
					},
				},
			},
		},
	})

	if got := report.Sections[0].Charts[0].Type; got != "bar" {
		t.Fatalf("chart type = %q, want bar", got)
	}
}
