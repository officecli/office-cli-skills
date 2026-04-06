package review

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SofficeConverter struct {
	binary string
}

func NewSofficeConverter() *SofficeConverter {
	return &SofficeConverter{binary: findSofficeBinary()}
}

func (c *SofficeConverter) Convert(ctx context.Context, sourcePath string) (string, error) {
	binary := strings.TrimSpace(c.binary)
	if binary == "" {
		return "", fmt.Errorf("未找到 LibreOffice（soffice）")
	}
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", fmt.Errorf("解析文件路径失败：%w", err)
	}
	tmpDir, err := os.MkdirTemp("", "officecli-review-pdf-*")
	if err != nil {
		return "", fmt.Errorf("创建临时目录失败：%w", err)
	}
	outputPath := filepath.Join(tmpDir, strings.TrimSuffix(filepath.Base(absSource), filepath.Ext(absSource))+".pdf")
	cmd := exec.CommandContext(ctx, binary, "--headless", "--convert-to", "pdf", "--outdir", tmpDir, absSource)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("LibreOffice 转 PDF 失败：%w: %s", err, strings.TrimSpace(string(output)))
	}
	if _, err := os.Stat(outputPath); err != nil {
		return "", fmt.Errorf("LibreOffice 未生成 PDF：%w", err)
	}
	return outputPath, nil
}

func findSofficeBinary() string {
	for _, candidate := range []string{"soffice", "libreoffice"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}
