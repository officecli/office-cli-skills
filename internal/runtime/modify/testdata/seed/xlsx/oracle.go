package seed_xlsx

import (
	"fmt"
	"strings"

	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

func Oracle(after []byte) error {
	contentXMLs, err := ooxmledit.ExtractContentXML(after, ooxmledit.FileTypeXLSX)
	if err != nil {
		return fmt.Errorf("extract content: %w", err)
	}
	found := false
	for path, xml := range contentXMLs {
		if strings.Contains(path, "sheet") && strings.Contains(xml, "150") {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("worksheet should contain cell value '150' after update")
	}
	return nil
}
