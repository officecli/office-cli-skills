package modify

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/officecli/officecli-internal/engine"
	"github.com/officecli/officecli-internal/engine/edit"
	"github.com/officecli/officecli-internal/engine/nonppt"
	"github.com/officecli/officecli-internal/engine/ppt"
	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

func callLLM(ctx context.Context, llm engine.LLMClient, prompt string) (string, error) {
	return llm.CompleteJSON(ctx, []engine.LLMMessage{
		{Role: "user", Content: prompt},
	})
}

func parseWithRetry(ctx context.Context, llm engine.LLMClient, prompt string, meta *ResultMeta, maxRetries int, maxNeedsRewrite int) (*edit.EditResponse, error) {
	raw, err := callLLM(ctx, llm, prompt)
	meta.LLMCalls++
	if err != nil {
		return nil, fmt.Errorf("llm call: %w", err)
	}

	resp, warnings, parseErr := edit.ParseEditResponse(raw, maxNeedsRewrite)
	meta.Warnings = append(meta.Warnings, warnings...)
	if parseErr == nil {
		return resp, nil
	}

	for retry := 0; retry < maxRetries; retry++ {
		retryPrompt := prompt + fmt.Sprintf("\n\nYour last response was invalid JSON: %s. Respond again with valid JSON only.", parseErr.Error())
		raw, err = callLLM(ctx, llm, retryPrompt)
		meta.LLMCalls++
		if err != nil {
			return nil, fmt.Errorf("llm retry call: %w", err)
		}
		resp, warnings, parseErr = edit.ParseEditResponse(raw, maxNeedsRewrite)
		meta.Warnings = append(meta.Warnings, warnings...)
		if parseErr == nil {
			return resp, nil
		}
	}

	return nil, fmt.Errorf("parse failed after retries: %w", parseErr)
}

func applyOps(fileType ooxmledit.FileType, srcBytes []byte, ops []json.RawMessage, meta *ResultMeta) []byte {
	current := srcBytes
	for _, raw := range ops {
		decoded, err := edit.DecodeOp(fileType, raw)
		if err != nil {
			meta.OpsFailed++
			meta.Warnings = append(meta.Warnings, err.Error())
			continue
		}

		var result []byte
		var applyErr error

		switch fileType {
		case ooxmledit.FileTypePPTX:
			op := decoded.(*ppt.ModifyOperation)
			result, applyErr = ppt.ApplyModification(op.Intent, current, op)
		case ooxmledit.FileTypeDOCX:
			op := decoded.(*nonppt.DocxModifyOperation)
			req := nonppt.ModifyRequest{Intent: op.Intent, Target: nonppt.ModifyTargetMetadata{ParagraphIndex: op.ParagraphIndex}}
			result, applyErr = nonppt.ApplyDOCXModification(req, current, op)
		case ooxmledit.FileTypeXLSX:
			op := decoded.(*nonppt.XLSXModifyOperation)
			req := nonppt.ModifyRequest{Intent: op.Intent, Target: nonppt.ModifyTargetMetadata{WorksheetIndex: op.WorksheetIndex}}
			result, applyErr = nonppt.ApplyXLSXModification(req, current, op)
		}

		if applyErr != nil {
			meta.OpsFailed++
			meta.Warnings = append(meta.Warnings, applyErr.Error())
			continue
		}
		current = result
		meta.OpsApplied++
	}

	if meta.OpsFailed > 0 && meta.OpsApplied > 0 {
		meta.Warnings = append(meta.Warnings, "partial_failure")
	}

	return current
}

func buildFidelityReport(srcBytes, resultBytes []byte) FidelityReport {
	before, err := ooxmledit.InventoryNonText(srcBytes)
	if err != nil {
		return FidelityReport{}
	}
	after, err := ooxmledit.InventoryNonText(resultBytes)
	if err != nil {
		return FidelityReport{}
	}
	preserved, dropped := ooxmledit.DiffInventory(before, after)
	return FidelityReport{Preserved: preserved, Dropped: dropped}
}
