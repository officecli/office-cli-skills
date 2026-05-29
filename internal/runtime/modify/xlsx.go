package modify

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/officecli/officecli/engine"
	"github.com/officecli/officecli/engine/edit"
	"github.com/officecli/officecli/engine/nonppt"
	"github.com/officecli/officecli/pkg/ooxmledit"
)

func (m *Modifier) modifyXLSX(ctx context.Context, p Params, srcBytes []byte, progress engine.ProgressEmitter) (*Result, error) {
	meta := &ResultMeta{RouterPath: "B"}

	if progress != nil {
		progress.Emit(ctx, engine.ProgressEvent{Step: "modify_xlsx", Status: "extracting_content"})
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(srcBytes, ooxmledit.FileTypeXLSX)
	if err != nil {
		return nil, fmt.Errorf("extract xlsx content: %w", err)
	}

	rowsBySheet := nonppt.XLSXRowsFromContent(contentXMLs)
	previews := buildSheetPreviews(rowsBySheet)

	prompt := edit.BuildXLSXEditPrompt(edit.XLSXEditPromptInput{
		Prompt:        p.Prompt,
		SheetPreviews: previews,
		Language:      p.Language,
		Style:         p.Style,
	})

	if progress != nil {
		progress.Emit(ctx, engine.ProgressEvent{Step: "modify_xlsx", Status: "calling_llm"})
	}

	resp, err := parseWithRetry(ctx, m.llm, prompt, meta, p.MaxParseRetries, p.MaxNeedsRewrite)
	if err != nil {
		meta.AttentionRequired = true
		return &Result{
			Bytes:      srcBytes,
			ResultMeta: *meta,
		}, fmt.Errorf("path B parse: %w", err)
	}

	if progress != nil {
		progress.Emit(ctx, engine.ProgressEvent{Step: "modify_xlsx", Status: "applying_ops"})
	}

	resultBytes := applyOps(ooxmledit.FileTypeXLSX, srcBytes, resp.Ops, meta)

	pathBOps := meta.OpsApplied

	if len(resp.NeedsRewrite) > 0 {
		resultBytes = m.runXLSXEscalations(ctx, p, resultBytes, resp.NeedsRewrite, meta, progress)
		if pathBOps > 0 {
			meta.RouterPath = "mixed"
		} else {
			meta.RouterPath = "C"
		}
	}

	fidelity := buildFidelityReport(srcBytes, resultBytes)
	meta.Fidelity = fidelity
	meta.computeAttention()

	return &Result{
		Bytes:      resultBytes,
		ResultMeta: *meta,
	}, nil
}

func buildSheetPreviews(rowsBySheet map[string][][]string) []edit.SheetPreview {
	keys := make([]string, 0, len(rowsBySheet))
	for k := range rowsBySheet {
		keys = append(keys, k)
	}
	sortStrings(keys)

	previews := make([]edit.SheetPreview, 0, len(keys))
	for _, key := range keys {
		rows := rowsBySheet[key]
		cappedRows := rows
		if len(cappedRows) > modifyPreviewRows {
			cappedRows = cappedRows[:modifyPreviewRows]
		}
		for i, row := range cappedRows {
			if len(row) > modifyPreviewCols {
				cappedRows[i] = row[:modifyPreviewCols]
			}
		}
		previews = append(previews, edit.SheetPreview{
			SheetName: key,
			Rows:      cappedRows,
		})
	}
	return previews
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func (m *Modifier) runXLSXEscalations(ctx context.Context, p Params, currentBytes []byte, targets []string, meta *ResultMeta, progress engine.ProgressEmitter) []byte {
	for _, target := range targets {
		if !strings.HasPrefix(target, "sheet:") {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("unknown sentinel target: %s", target))
			continue
		}

		sheetRef := strings.TrimPrefix(target, "sheet:")
		worksheetIndex, err := resolveSheetIndex(sheetRef)
		if err != nil {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("invalid sentinel target: %s", target))
			continue
		}

		if progress != nil {
			progress.Emit(ctx, engine.ProgressEvent{Step: "modify_xlsx", Status: fmt.Sprintf("escalating_sheet_%d", worksheetIndex)})
		}

		contentXMLs, err := ooxmledit.ExtractContentXML(currentBytes, ooxmledit.FileTypeXLSX)
		if err != nil {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("extract sheet %d for escalation: %s", worksheetIndex, err.Error()))
			continue
		}

		rowsBySheet := nonppt.XLSXRowsFromContent(contentXMLs)
		sheetKey := fmt.Sprintf("sheet%d", worksheetIndex)
		rows := rowsBySheet[sheetKey]

		escalationPrompt := nonppt.BuildXLSXModifyPrompt(nonppt.XLSXModifyPromptInput{
			Intent:      "rewrite_xlsx_sheet",
			Description: p.Prompt,
			Target: nonppt.PromptTargetMetadata{
				WorksheetIndex: worksheetIndex,
				WorksheetName:  sheetRef,
			},
			WorksheetSummaries: []map[string]any{
				{"worksheetIndex": worksheetIndex, "name": sheetRef, "rows": rows},
			},
			DefaultWorksheet: worksheetIndex,
		})

		raw, llmErr := callLLM(ctx, m.llm, escalationPrompt)
		meta.LLMCalls++
		if llmErr != nil {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("escalation llm call for sheet %d: %s", worksheetIndex, llmErr.Error()))
			continue
		}

		escalationResp, warnings, parseErr := edit.ParseEditResponse(raw, 0)
		meta.Warnings = append(meta.Warnings, warnings...)
		if parseErr != nil {
			meta.OpsFailed++
			meta.Warnings = append(meta.Warnings, "pathc_parse_failed")
			continue
		}

		if len(escalationResp.NeedsRewrite) > 0 {
			meta.Warnings = append(meta.Warnings, "sentinel_dropped_max_depth")
		}

		for _, opRaw := range escalationResp.Ops {
			decoded, decErr := edit.DecodeOp(ooxmledit.FileTypeXLSX, opRaw)
			if decErr != nil {
				meta.OpsFailed++
				meta.Warnings = append(meta.Warnings, decErr.Error())
				continue
			}
			op := decoded.(*nonppt.XLSXModifyOperation)
			req := nonppt.ModifyRequest{Intent: op.Intent, Target: nonppt.ModifyTargetMetadata{WorksheetIndex: op.WorksheetIndex}}
			result, applyErr := nonppt.ApplyXLSXModification(req, currentBytes, op)
			if applyErr != nil {
				meta.OpsFailed++
				meta.Warnings = append(meta.Warnings, applyErr.Error())
				continue
			}
			currentBytes = result
			meta.OpsApplied++
		}
	}
	return currentBytes
}

func resolveSheetIndex(ref string) (int, error) {
	if strings.HasPrefix(ref, "Sheet") {
		numStr := strings.TrimPrefix(ref, "Sheet")
		n, err := strconv.Atoi(numStr)
		if err != nil || n < 1 {
			return 0, fmt.Errorf("invalid sheet reference: %s", ref)
		}
		return n, nil
	}
	n, err := strconv.Atoi(ref)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid sheet reference: %s", ref)
	}
	return n, nil
}
