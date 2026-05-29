package edit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/officecli/officecli-internal/engine/nonppt"
	"github.com/officecli/officecli-internal/engine/ppt"
	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

type EditResponse struct {
	Ops          []json.RawMessage `json:"ops"`
	NeedsRewrite []string          `json:"__needs_rewrite"`
}

type rawOp struct {
	OpType  string          `json:"op_type"`
	Target  json.RawMessage `json:"target"`
	Payload json.RawMessage `json:"payload"`
}

type pptxTarget struct {
	Slide int `json:"slide"`
}

type pptxPayload struct {
	NewTitle     string       `json:"new_title"`
	NewBullets   []string     `json:"new_bullets"`
	NewParagraph string       `json:"new_paragraph"`
	CellUpdates  []cellUpdate `json:"cell_updates"`
	NewSlideXML  string       `json:"new_slide_xml"`
}

type cellUpdate struct {
	Row   int    `json:"row"`
	Col   int    `json:"col"`
	Value string `json:"value"`
}

type docxTarget struct {
	Paragraph int `json:"paragraph"`
}

type docxPayload struct {
	NewText    string   `json:"new_text"`
	Paragraphs []string `json:"paragraphs"`
}

type xlsxTarget struct {
	Sheet          string `json:"sheet"`
	WorksheetIndex int    `json:"worksheet_index"`
}

type xlsxPayload struct {
	CellUpdates []xlsxCellUpdate `json:"cell_updates"`
	Rows        [][]string       `json:"rows"`
}

type xlsxCellUpdate struct {
	Cell  string `json:"cell"`
	Value string `json:"value"`
}

func ParseEditResponse(s string, maxNeedsRewrite int) (*EditResponse, []string, error) {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "{"); idx > 0 {
		s = s[idx:]
	}
	if idx := strings.LastIndex(s, "}"); idx >= 0 && idx < len(s)-1 {
		s = s[:idx+1]
	}

	var resp EditResponse
	if err := json.Unmarshal([]byte(s), &resp); err != nil {
		return nil, nil, fmt.Errorf("parse edit response: %w", err)
	}

	var warnings []string
	if len(resp.NeedsRewrite) > maxNeedsRewrite {
		resp.NeedsRewrite = resp.NeedsRewrite[:maxNeedsRewrite]
		warnings = append(warnings, "sentinel_truncated_max_targets")
	}

	return &resp, warnings, nil
}

func DecodeOp(format ooxmledit.FileType, raw json.RawMessage) (any, error) {
	var op rawOp
	if err := json.Unmarshal(raw, &op); err != nil {
		return nil, fmt.Errorf("decode op envelope: %w", err)
	}

	switch format {
	case ooxmledit.FileTypePPTX:
		return decodePPTXOp(op)
	case ooxmledit.FileTypeDOCX:
		return decodeDOCXOp(op)
	case ooxmledit.FileTypeXLSX:
		return decodeXLSXOp(op)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

func decodePPTXOp(op rawOp) (*ppt.ModifyOperation, error) {
	var target pptxTarget
	if err := json.Unmarshal(op.Target, &target); err != nil {
		return nil, fmt.Errorf("decode pptx target: %w", err)
	}
	var payload pptxPayload
	if err := json.Unmarshal(op.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode pptx payload: %w", err)
	}

	result := &ppt.ModifyOperation{
		Intent:     op.OpType,
		SlideIndex: target.Slide,
	}

	switch op.OpType {
	case "replace_slide_title":
		result.Operation = ppt.ModifyOperationPayload{
			Type:     "replace_title",
			NewTitle: payload.NewTitle,
		}
	case "append_slide_bullets":
		result.Operation = ppt.ModifyOperationPayload{
			Type:       "append_bullets",
			NewBullets: payload.NewBullets,
		}
	case "replace_body_paragraph":
		result.Operation = ppt.ModifyOperationPayload{
			Type:         "replace_paragraph",
			NewParagraph: payload.NewParagraph,
		}
	case "update_table_cells":
		cells := make([]ppt.CellUpdate, len(payload.CellUpdates))
		for i, cu := range payload.CellUpdates {
			cells[i] = ppt.CellUpdate{Row: cu.Row, Col: cu.Col, Value: cu.Value}
		}
		result.Operation = ppt.ModifyOperationPayload{
			Type:        "update_table_cells",
			CellUpdates: cells,
		}
	case "rewrite_entire_slide":
		result.Operation = ppt.ModifyOperationPayload{
			Type:        "rewrite_slide",
			NewSlideXML: payload.NewSlideXML,
		}
	default:
		return nil, fmt.Errorf("unknown pptx op_type: %s", op.OpType)
	}

	return result, nil
}

func decodeDOCXOp(op rawOp) (*nonppt.DocxModifyOperation, error) {
	var target docxTarget
	if err := json.Unmarshal(op.Target, &target); err != nil {
		return nil, fmt.Errorf("decode docx target: %w", err)
	}
	var payload docxPayload
	if err := json.Unmarshal(op.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode docx payload: %w", err)
	}

	result := &nonppt.DocxModifyOperation{
		Intent:         op.OpType,
		ParagraphIndex: target.Paragraph,
	}

	switch op.OpType {
	case "replace_docx_paragraph":
		result.Operation = nonppt.DocxOperation{
			Type:    "replace_paragraph",
			NewText: payload.NewText,
		}
	case "append_docx_paragraph":
		result.Operation = nonppt.DocxOperation{
			Type:    "append_paragraph",
			NewText: payload.NewText,
		}
	case "rewrite_docx_document":
		result.Operation = nonppt.DocxOperation{
			Type:       "rewrite_document",
			Paragraphs: payload.Paragraphs,
		}
	default:
		return nil, fmt.Errorf("unknown docx op_type: %s", op.OpType)
	}

	return result, nil
}

func decodeXLSXOp(op rawOp) (*nonppt.XLSXModifyOperation, error) {
	var target xlsxTarget
	if err := json.Unmarshal(op.Target, &target); err != nil {
		return nil, fmt.Errorf("decode xlsx target: %w", err)
	}
	var payload xlsxPayload
	if err := json.Unmarshal(op.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode xlsx payload: %w", err)
	}

	result := &nonppt.XLSXModifyOperation{
		Intent:         op.OpType,
		WorksheetIndex: target.WorksheetIndex,
	}

	switch op.OpType {
	case "update_xlsx_cells":
		cells := make([]nonppt.XLSXCellValue, len(payload.CellUpdates))
		for i, cu := range payload.CellUpdates {
			cells[i] = nonppt.XLSXCellValue{Cell: cu.Cell, Value: cu.Value}
		}
		result.Operation = nonppt.XLSXOperation{
			Type:        "update_cells",
			CellUpdates: cells,
		}
	case "append_xlsx_summary":
		result.Operation = nonppt.XLSXOperation{
			Type: "append_summary",
			Rows: payload.Rows,
		}
	case "rewrite_xlsx_sheet":
		result.Operation = nonppt.XLSXOperation{
			Type: "rewrite_sheet",
			Rows: payload.Rows,
		}
	default:
		return nil, fmt.Errorf("unknown xlsx op_type: %s", op.OpType)
	}

	return result, nil
}
