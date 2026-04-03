package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func RenderResult(w io.Writer, result GenerateResult, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	if _, err := fmt.Fprintf(w, "生成完成！已保存至 %s\n", result.FilePath); err != nil {
		return err
	}
	if result.Published {
		if _, err := fmt.Fprintf(w, "在线访问地址：%s；访问密码：%s\n", result.AccessURL, result.Password); err != nil {
			return err
		}
		if strings.TrimSpace(result.ExpiresAt) != "" {
			if _, err := fmt.Fprintf(w, "链接有效期至：%s\n", result.ExpiresAt); err != nil {
				return err
			}
		}
	}
	for _, warning := range result.Warnings {
		if strings.TrimSpace(warning) == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "提示：%s\n", warning); err != nil {
			return err
		}
	}
	return nil
}
