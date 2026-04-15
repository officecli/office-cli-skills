package officegen

import (
	"strings"
	"testing"
)

func TestBuildReport_RendersEChartsAndSectionContent(t *testing.T) {
	report := Report{
		Title:    "Q2 Business Review",
		Subtitle: "A concise view of commercial performance",
		Audience: "Board and investors",
		Summary:  "Revenue held up, but conversion quality softened in the lower funnel.",
		KPIs: []ReportKPI{
			{Label: "Revenue", Value: "$12.4M", Change: "+8% QoQ"},
		},
		Findings: []string{"North America stayed ahead of plan."},
		Sections: []ReportSection{
			{
				Title:     "Demand momentum",
				Narrative: []string{"Demand stayed strongest in North America while Europe remained stable."},
				Charts: []ReportChart{
					{
						Type:       "line",
						Title:      "Regional revenue trend",
						Categories: []string{"Jan", "Feb", "Mar"},
						Series:     []ChartSeries{{Name: "Revenue", Values: []float64{100, 114, 128}}},
						Source:     "Internal finance data",
					},
				},
			},
		},
		AppendixTables: []ReportTable{
			{
				Title:   "Supporting table",
				Headers: []string{"Region", "Revenue"},
				Rows:    [][]string{{"North America", "128"}},
			},
		},
	}

	data, err := BuildReport(report)
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
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

func TestNormalizeReport_NormalizesUnsupportedChartType(t *testing.T) {
	report := NormalizeReport(Report{
		Title: "Test",
		Sections: []ReportSection{
			{
				Title: "Section",
				Charts: []ReportChart{
					{
						Type:       "bubble",
						Title:      "Chart",
						Categories: []string{"A", "B"},
						Series:     []ChartSeries{{Name: "Series 1", Values: []float64{1, 2}}},
					},
				},
			},
		},
	})

	if got := report.Sections[0].Charts[0].Type; got != "bar" {
		t.Fatalf("chart type = %q, want bar", got)
	}
}
