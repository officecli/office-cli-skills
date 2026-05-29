package seed_docx

import (
	"fmt"
	"strings"

	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

func Oracle(after []byte) error {
	contentXMLs, err := ooxmledit.ExtractContentXML(after, ooxmledit.FileTypeDOCX)
	if err != nil {
		return fmt.Errorf("extract content: %w", err)
	}
	docXML, ok := contentXMLs["word/document.xml"]
	if !ok {
		return fmt.Errorf("word/document.xml not found in output")
	}
	if !strings.Contains(docXML, "Updated Project Status Report") {
		return fmt.Errorf("document should contain 'Updated Project Status Report', got XML: %s", truncate(docXML, 500))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
