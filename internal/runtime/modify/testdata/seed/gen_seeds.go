//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/officecli/officecli-internal/pkg/officegen"
)

func main() {
	base := os.Args[1]

	// PPTX seed
	pptxBytes, err := officegen.NewPPTXGenerator().Generate([]officegen.Slide{
		{Title: "Q3 Revenue Report", Content: "Revenue grew 15% YoY", Layout: "content", Points: []string{"North America: $128M", "Europe: $96M", "Asia: $45M"}},
		{Title: "Regional Breakdown", Content: "Detailed analysis by region", Layout: "content", Points: []string{"Strong growth in APAC", "Stable EU performance"}},
	}, officegen.PPTXOptions{Title: "Q3 Revenue", Creator: "OfficeCLI"})
	must(err)
	writeFile(base, "pptx", "source.pptx", pptxBytes)

	// DOCX seed
	docxBytes, err := officegen.NewDOCXGenerator().Generate([]officegen.DocxParagraph{
		{Text: "Project Status Report", IsBold: true},
		{Text: "The project is currently on track for Q3 delivery."},
		{Text: "Key milestones have been completed ahead of schedule."},
	}, officegen.DOCXOptions{Title: "Project Status", Creator: "OfficeCLI"})
	must(err)
	writeFile(base, "docx", "source.docx", docxBytes)

	// XLSX seed
	xlsxBytes, err := officegen.NewXLSXGenerator().Generate([]officegen.XlsxSheet{
		{
			Name: "Sales",
			Rows: [][]string{
				{"Region", "Q1", "Q2", "Q3"},
				{"North America", "100", "120", "128"},
				{"Europe", "80", "88", "96"},
				{"Asia", "30", "38", "45"},
			},
		},
	}, officegen.XLSXOptions{Title: "Sales Data", Creator: "OfficeCLI"})
	must(err)
	writeFile(base, "xlsx", "source.xlsx", xlsxBytes)

	fmt.Println("seed fixtures generated successfully")
}

func writeFile(base, format, filename string, data []byte) {
	dir := filepath.Join(base, format)
	must(os.MkdirAll(dir, 0o755))
	must(os.WriteFile(filepath.Join(dir, filename), data, 0o644))
	fmt.Printf("  %s/%s (%d bytes)\n", format, filename, len(data))
}

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
