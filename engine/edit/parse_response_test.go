package edit

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/officecli/officecli/engine/nonppt"
	"github.com/officecli/officecli/engine/ppt"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

func TestParseEditResponseBasic(t *testing.T) {
	input := `{
		"ops": [
			{"op_type":"replace_slide_title","target":{"slide":1},"payload":{"new_title":"New Title"}}
		],
		"__needs_rewrite": []
	}`
	resp, warnings, err := ParseEditResponse(input, 3)
	if err != nil {
		t.Fatalf("ParseEditResponse: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(resp.Ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(resp.Ops))
	}
	if len(resp.NeedsRewrite) != 0 {
		t.Fatalf("expected 0 needs_rewrite, got %d", len(resp.NeedsRewrite))
	}
}

func TestParseEditResponseSentinelCap(t *testing.T) {
	input := `{
		"ops": [],
		"__needs_rewrite": ["slide:1","slide:2","slide:3","slide:4","slide:5"]
	}`
	resp, warnings, err := ParseEditResponse(input, 3)
	if err != nil {
		t.Fatalf("ParseEditResponse: %v", err)
	}
	if len(resp.NeedsRewrite) != 3 {
		t.Fatalf("expected 3 needs_rewrite after cap, got %d", len(resp.NeedsRewrite))
	}
	if len(warnings) != 1 || warnings[0] != "sentinel_truncated_max_targets" {
		t.Fatalf("expected sentinel_truncated_max_targets warning, got %v", warnings)
	}
}

func TestParseEditResponseInvalidJSON(t *testing.T) {
	_, _, err := ParseEditResponse("not json", 3)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseEditResponseWithExtraText(t *testing.T) {
	input := `Here is the response: {"ops":[],"__needs_rewrite":[]}`
	resp, _, err := ParseEditResponse(input, 3)
	if err != nil {
		t.Fatalf("ParseEditResponse: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestDecodeOpPPTX(t *testing.T) {
	tests := []struct {
		opType string
		json   string
	}{
		{
			"replace_slide_title",
			`{"op_type":"replace_slide_title","target":{"slide":1},"payload":{"new_title":"New Title"}}`,
		},
		{
			"append_slide_bullets",
			`{"op_type":"append_slide_bullets","target":{"slide":2},"payload":{"new_bullets":["A","B"]}}`,
		},
		{
			"replace_body_paragraph",
			`{"op_type":"replace_body_paragraph","target":{"slide":1},"payload":{"new_paragraph":"New text"}}`,
		},
		{
			"update_table_cells",
			`{"op_type":"update_table_cells","target":{"slide":1},"payload":{"cell_updates":[{"row":1,"col":1,"value":"X"}]}}`,
		},
		{
			"rewrite_entire_slide",
			`{"op_type":"rewrite_entire_slide","target":{"slide":3},"payload":{"new_slide_xml":"<p:sld/>"}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.opType, func(t *testing.T) {
			result, err := DecodeOp(ooxmledit.FileTypePPTX, json.RawMessage(tc.json))
			if err != nil {
				t.Fatalf("DecodeOp: %v", err)
			}
			op, ok := result.(*ppt.ModifyOperation)
			if !ok {
				t.Fatalf("expected *ppt.ModifyOperation, got %T", result)
			}
			if op.Intent != tc.opType {
				t.Errorf("expected intent %q, got %q", tc.opType, op.Intent)
			}
		})
	}
}

func TestDecodeOpDOCX(t *testing.T) {
	tests := []struct {
		opType string
		json   string
	}{
		{
			"replace_docx_paragraph",
			`{"op_type":"replace_docx_paragraph","target":{"paragraph":1},"payload":{"new_text":"Replaced"}}`,
		},
		{
			"append_docx_paragraph",
			`{"op_type":"append_docx_paragraph","target":{"paragraph":2},"payload":{"new_text":"Appended"}}`,
		},
		{
			"rewrite_docx_document",
			`{"op_type":"rewrite_docx_document","target":{"paragraph":0},"payload":{"paragraphs":["A","B","C"]}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.opType, func(t *testing.T) {
			result, err := DecodeOp(ooxmledit.FileTypeDOCX, json.RawMessage(tc.json))
			if err != nil {
				t.Fatalf("DecodeOp: %v", err)
			}
			op, ok := result.(*nonppt.DocxModifyOperation)
			if !ok {
				t.Fatalf("expected *nonppt.DocxModifyOperation, got %T", result)
			}
			if op.Intent != tc.opType {
				t.Errorf("expected intent %q, got %q", tc.opType, op.Intent)
			}
		})
	}
}

func TestDecodeOpXLSX(t *testing.T) {
	tests := []struct {
		opType string
		json   string
	}{
		{
			"update_xlsx_cells",
			`{"op_type":"update_xlsx_cells","target":{"sheet":"Sheet1","worksheet_index":1},"payload":{"cell_updates":[{"cell":"A1","value":"100"}]}}`,
		},
		{
			"append_xlsx_summary",
			`{"op_type":"append_xlsx_summary","target":{"sheet":"Sheet1","worksheet_index":1},"payload":{"rows":[["Total","200"]]}}`,
		},
		{
			"rewrite_xlsx_sheet",
			`{"op_type":"rewrite_xlsx_sheet","target":{"sheet":"Sheet1","worksheet_index":1},"payload":{"rows":[["A","B"],["1","2"]]}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.opType, func(t *testing.T) {
			result, err := DecodeOp(ooxmledit.FileTypeXLSX, json.RawMessage(tc.json))
			if err != nil {
				t.Fatalf("DecodeOp: %v", err)
			}
			op, ok := result.(*nonppt.XLSXModifyOperation)
			if !ok {
				t.Fatalf("expected *nonppt.XLSXModifyOperation, got %T", result)
			}
			if op.Intent != tc.opType {
				t.Errorf("expected intent %q, got %q", tc.opType, op.Intent)
			}
		})
	}
}

func TestDecodeOpUnknownType(t *testing.T) {
	raw := json.RawMessage(`{"op_type":"unknown_op","target":{"slide":1},"payload":{}}`)
	_, err := DecodeOp(ooxmledit.FileTypePPTX, raw)
	if err == nil {
		t.Fatal("expected error for unknown op_type")
	}
}

func TestDecodeOpUnsupportedFormat(t *testing.T) {
	raw := json.RawMessage(`{"op_type":"replace_slide_title","target":{"slide":1},"payload":{"new_title":"X"}}`)
	_, err := DecodeOp(ooxmledit.FileType("pdf"), raw)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestDecodeOpTypesMatchEngineSwitch(t *testing.T) {
	pptxOps := map[string]string{
		"replace_slide_title":   `{"op_type":"replace_slide_title","target":{"slide":1},"payload":{"new_title":"T"}}`,
		"append_slide_bullets":  `{"op_type":"append_slide_bullets","target":{"slide":1},"payload":{"new_bullets":["a"]}}`,
		"replace_body_paragraph": `{"op_type":"replace_body_paragraph","target":{"slide":1},"payload":{"new_paragraph":"p"}}`,
		"update_table_cells":    `{"op_type":"update_table_cells","target":{"slide":1},"payload":{"cell_updates":[{"row":1,"col":1,"value":"v"}]}}`,
		"rewrite_entire_slide":  `{"op_type":"rewrite_entire_slide","target":{"slide":1},"payload":{"new_slide_xml":"<p:sld/>"}}`,
	}
	for opType, rawJSON := range pptxOps {
		result, err := DecodeOp(ooxmledit.FileTypePPTX, json.RawMessage(rawJSON))
		if err != nil {
			t.Errorf("DecodeOp(pptx, %s): %v", opType, err)
			continue
		}
		op := result.(*ppt.ModifyOperation)
		if op.Intent != opType {
			t.Errorf("decoded intent %q != expected %q", op.Intent, opType)
		}
	}

	docxOps := map[string]string{
		"replace_docx_paragraph": `{"op_type":"replace_docx_paragraph","target":{"paragraph":1},"payload":{"new_text":"t"}}`,
		"append_docx_paragraph":  `{"op_type":"append_docx_paragraph","target":{"paragraph":1},"payload":{"new_text":"t"}}`,
		"rewrite_docx_document":  `{"op_type":"rewrite_docx_document","target":{"paragraph":0},"payload":{"paragraphs":["a"]}}`,
	}
	for opType, rawJSON := range docxOps {
		result, err := DecodeOp(ooxmledit.FileTypeDOCX, json.RawMessage(rawJSON))
		if err != nil {
			t.Errorf("DecodeOp(docx, %s): %v", opType, err)
			continue
		}
		op := result.(*nonppt.DocxModifyOperation)
		if op.Intent != opType {
			t.Errorf("decoded intent %q != expected %q", op.Intent, opType)
		}
	}

	xlsxOps := map[string]string{
		"update_xlsx_cells":  `{"op_type":"update_xlsx_cells","target":{"sheet":"S","worksheet_index":1},"payload":{"cell_updates":[{"cell":"A1","value":"1"}]}}`,
		"append_xlsx_summary": `{"op_type":"append_xlsx_summary","target":{"sheet":"S","worksheet_index":1},"payload":{"rows":[["a"]]}}`,
		"rewrite_xlsx_sheet": `{"op_type":"rewrite_xlsx_sheet","target":{"sheet":"S","worksheet_index":1},"payload":{"rows":[["a"]]}}`,
	}
	for opType, rawJSON := range xlsxOps {
		result, err := DecodeOp(ooxmledit.FileTypeXLSX, json.RawMessage(rawJSON))
		if err != nil {
			t.Errorf("DecodeOp(xlsx, %s): %v", opType, err)
			continue
		}
		op := result.(*nonppt.XLSXModifyOperation)
		if op.Intent != opType {
			t.Errorf("decoded intent %q != expected %q", op.Intent, opType)
		}
	}
}

func fileTypeFromString(s string) ooxmledit.FileType {
	switch s {
	case "pptx":
		return ooxmledit.FileTypePPTX
	case "docx":
		return ooxmledit.FileTypeDOCX
	case "xlsx":
		return ooxmledit.FileTypeXLSX
	}
	return ooxmledit.FileType(s)
}

func buildTestOp(format, opType string) json.RawMessage {
	switch format {
	case "pptx":
		switch opType {
		case "replace_slide_title":
			return json.RawMessage(`{"op_type":"replace_slide_title","target":{"slide":1},"payload":{"new_title":"T"}}`)
		case "append_slide_bullets":
			return json.RawMessage(`{"op_type":"append_slide_bullets","target":{"slide":1},"payload":{"new_bullets":["a"]}}`)
		case "replace_body_paragraph":
			return json.RawMessage(`{"op_type":"replace_body_paragraph","target":{"slide":1},"payload":{"new_paragraph":"p"}}`)
		case "update_table_cells":
			return json.RawMessage(`{"op_type":"update_table_cells","target":{"slide":1},"payload":{"cell_updates":[{"row":1,"col":1,"value":"v"}]}}`)
		case "rewrite_entire_slide":
			return json.RawMessage(`{"op_type":"rewrite_entire_slide","target":{"slide":1},"payload":{"new_slide_xml":"<p:sld/>"}}`)
		}
	case "docx":
		switch opType {
		case "replace_docx_paragraph":
			return json.RawMessage(`{"op_type":"replace_docx_paragraph","target":{"paragraph":1},"payload":{"new_text":"t"}}`)
		case "append_docx_paragraph":
			return json.RawMessage(`{"op_type":"append_docx_paragraph","target":{"paragraph":1},"payload":{"new_text":"t"}}`)
		case "rewrite_docx_document":
			return json.RawMessage(`{"op_type":"rewrite_docx_document","target":{"paragraph":0},"payload":{"paragraphs":["a"]}}`)
		}
	case "xlsx":
		switch opType {
		case "update_xlsx_cells":
			return json.RawMessage(`{"op_type":"update_xlsx_cells","target":{"sheet":"S","worksheet_index":1},"payload":{"cell_updates":[{"cell":"A1","value":"1"}]}}`)
		case "append_xlsx_summary":
			return json.RawMessage(`{"op_type":"append_xlsx_summary","target":{"sheet":"S","worksheet_index":1},"payload":{"rows":[["a"]]}}`)
		case "rewrite_xlsx_sheet":
			return json.RawMessage(`{"op_type":"rewrite_xlsx_sheet","target":{"sheet":"S","worksheet_index":1},"payload":{"rows":[["a"]]}}`)
		}
	}
	return json.RawMessage(fmt.Sprintf(`{"op_type":"%s","target":{},"payload":{}}`, opType))
}
