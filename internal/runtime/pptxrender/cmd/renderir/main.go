package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/officecli/officecli/internal/runtime/pptxrender"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) != 3 {
		log.Fatalf("usage: renderir <presentation.ir.json> <output.pptx>")
	}

	irPath := os.Args[1]
	outputPath := os.Args[2]
	ir, err := loadIR(irPath)
	if err != nil {
		log.Fatal(err)
	}

	baseDir := assetBaseDir(irPath, ir.Assets)
	renderer := pptxrender.NewRenderer(pptxrender.RenderOptions{
		Assets: pptxrender.NewFileAssetResolver(baseDir, ir.Assets),
	})
	if _, err := renderer.RenderToFile(context.Background(), ir, outputPath); err != nil {
		log.Fatal(err)
	}
	fmt.Println(outputPath)
}

func loadIR(path string) (pptxrender.PresentationIR, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pptxrender.PresentationIR{}, err
	}
	var ir pptxrender.PresentationIR
	if err := json.Unmarshal(data, &ir); err != nil {
		return pptxrender.PresentationIR{}, err
	}
	return ir, nil
}

func assetBaseDir(irPath string, assets []pptxrender.Asset) string {
	irDir := filepath.Dir(irPath)
	for _, asset := range assets {
		if asset.Path == "" || filepath.IsAbs(asset.Path) {
			continue
		}
		if _, err := os.Stat(filepath.Join(irDir, asset.Path)); err == nil {
			return irDir
		}
	}
	return filepath.Dir(irDir)
}
