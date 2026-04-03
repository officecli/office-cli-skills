package ppt

import (
	"fmt"
	"strings"

	"github.com/officecli/officecli/pkg/ooxmledit"
)

type CellUpdate struct {
	Row   int
	Col   int
	Value string
}

func ApplyModification(intent string, ooxmlBytes []byte, op *ModifyOperation) ([]byte, error) {
	switch intent {
	case "replace_slide_title":
		return ooxmledit.ReplaceSlideTitle(ooxmlBytes, op.SlideIndex, op.Operation.NewTitle)
	case "append_slide_bullets":
		return ooxmledit.AppendSlideBullets(ooxmlBytes, op.SlideIndex, op.Operation.NewBullets)
	case "replace_body_paragraph":
		return ooxmledit.ReplaceBodyParagraph(ooxmlBytes, op.SlideIndex, 1, op.Operation.NewParagraph)
	case "update_table_cells":
		cellUpdates := make([]ooxmledit.CellUpdate, len(op.Operation.CellUpdates))
		for i, cu := range op.Operation.CellUpdates {
			cellUpdates[i] = ooxmledit.CellUpdate{Row: cu.Row, Col: cu.Col, Value: cu.Value}
		}
		return ooxmledit.UpdateTableCells(ooxmlBytes, op.SlideIndex, cellUpdates)
	case "rewrite_entire_slide":
		return ooxmledit.ReplaceSlideXML(ooxmlBytes, op.SlideIndex, op.Operation.NewSlideXML)
	default:
		return nil, fmt.Errorf("unsupported intent: %s", intent)
	}
}

func SanitizeHexColor(raw string) string {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "#"))
	if len(raw) != 6 {
		return ""
	}
	for _, r := range raw {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return ""
		}
	}
	return strings.ToUpper(raw)
}
