package seed_pptx

import (
	"fmt"
	"strings"

	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

func Oracle(after []byte) error {
	contentXMLs, err := ooxmledit.ExtractContentXML(after, ooxmledit.FileTypePPTX)
	if err != nil {
		return fmt.Errorf("extract content: %w", err)
	}
	slide1XML, ok := contentXMLs["ppt/slides/slide1.xml"]
	if !ok {
		return fmt.Errorf("slide1 not found in output")
	}
	if !strings.Contains(slide1XML, "Q3 Revenue Summary") {
		return fmt.Errorf("slide 1 title should contain 'Q3 Revenue Summary', got XML: %s", truncate(slide1XML, 500))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
