package modify

import (
	"context"
	"fmt"
	"strings"

	"github.com/officecli/officecli-internal/engine"
	"github.com/officecli/officecli-internal/engine/edit"
	"github.com/officecli/officecli-internal/engine/nonppt"
	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

func (m *Modifier) modifyDOCX(ctx context.Context, p Params, srcBytes []byte, progress engine.ProgressEmitter) (*Result, error) {
	meta := &ResultMeta{RouterPath: "B"}

	if progress != nil {
		progress.Emit(ctx, engine.ProgressEvent{Step: "modify_docx", Status: "extracting_content"})
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(srcBytes, ooxmledit.FileTypeDOCX)
	if err != nil {
		return nil, fmt.Errorf("extract docx content: %w", err)
	}

	documentXML, ok := contentXMLs["word/document.xml"]
	if !ok {
		return nil, fmt.Errorf("word/document.xml not found")
	}

	paragraphTexts := ooxmledit.ExtractDOCXParagraphTexts(documentXML)
	previews := buildParagraphPreviews(paragraphTexts, modifyPreviewParagraphs)
	if len(previews) < len(paragraphTexts) {
		meta.Warnings = append(meta.Warnings, "truncated_preview")
	}

	prompt := edit.BuildDOCXEditPrompt(edit.DOCXEditPromptInput{
		Prompt:            p.Prompt,
		ParagraphPreviews: previews,
		Language:          p.Language,
		Style:             p.Style,
	})

	if progress != nil {
		progress.Emit(ctx, engine.ProgressEvent{Step: "modify_docx", Status: "calling_llm"})
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
		progress.Emit(ctx, engine.ProgressEvent{Step: "modify_docx", Status: "applying_ops"})
	}

	resultBytes := applyOps(ooxmledit.FileTypeDOCX, srcBytes, resp.Ops, meta)

	pathBOps := meta.OpsApplied

	if len(resp.NeedsRewrite) > 0 {
		resultBytes = m.runDOCXEscalations(ctx, p, resultBytes, resp.NeedsRewrite, meta, progress)
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

func buildParagraphPreviews(paragraphTexts []string, cap int) []edit.ParagraphPreview {
	limit := len(paragraphTexts)
	if limit > cap {
		limit = cap
	}
	previews := make([]edit.ParagraphPreview, 0, limit)
	for i := 0; i < limit; i++ {
		previews = append(previews, edit.ParagraphPreview{
			ParagraphIndex: i + 1,
			Text:           paragraphTexts[i],
		})
	}
	return previews
}

func (m *Modifier) runDOCXEscalations(ctx context.Context, p Params, currentBytes []byte, targets []string, meta *ResultMeta, progress engine.ProgressEmitter) []byte {
	for _, target := range targets {
		if !strings.HasPrefix(target, "docx:") {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("unknown sentinel target: %s", target))
			continue
		}

		if progress != nil {
			progress.Emit(ctx, engine.ProgressEvent{Step: "modify_docx", Status: fmt.Sprintf("escalating_%s", target)})
		}

		contentXMLs, err := ooxmledit.ExtractContentXML(currentBytes, ooxmledit.FileTypeDOCX)
		if err != nil {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("extract docx for escalation %s: %s", target, err.Error()))
			continue
		}
		documentXML := contentXMLs["word/document.xml"]
		paragraphTexts := ooxmledit.ExtractDOCXParagraphTexts(documentXML)

		escalationPrompt := buildDOCXRewritePrompt(p.Prompt, paragraphTexts)

		raw, llmErr := callLLM(ctx, m.llm, escalationPrompt)
		meta.LLMCalls++
		if llmErr != nil {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("escalation llm call for %s: %s", target, llmErr.Error()))
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
			decoded, decErr := edit.DecodeOp(ooxmledit.FileTypeDOCX, opRaw)
			if decErr != nil {
				meta.OpsFailed++
				meta.Warnings = append(meta.Warnings, decErr.Error())
				continue
			}
			op := decoded.(*nonppt.DocxModifyOperation)
			req := nonppt.ModifyRequest{Intent: op.Intent, Target: nonppt.ModifyTargetMetadata{ParagraphIndex: op.ParagraphIndex}}
			result, applyErr := nonppt.ApplyDOCXModification(req, currentBytes, op)
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

func buildDOCXRewritePrompt(userPrompt string, paragraphTexts []string) string {
	summary := strings.Join(paragraphTexts, " | ")
	if len(summary) > 2000 {
		summary = summary[:2000]
	}
	return fmt.Sprintf(`You are a professional Word document rewrite assistant. Return JSON only.

Intent: rewrite_docx_document
User instruction: %s
Current document paragraphs summary: %s

Output format (strict JSON):
{
  "ops": [
    {
      "op_type": "rewrite_docx_document",
      "target": {"paragraph": 0},
      "payload": {
        "paragraphs": ["paragraph 1 text", "paragraph 2 text"]
      }
    }
  ],
  "__needs_rewrite": []
}

Constraints:
- Rewrite the entire document
- Preserve the document's topic and key facts unless the user explicitly asks for new content
- paragraphs array must contain at least one element
`, userPrompt, summary)
}
