package modify

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/officecli/officecli-internal/engine"
	"github.com/officecli/officecli-internal/engine/edit"
	"github.com/officecli/officecli-internal/engine/ppt"
	"github.com/officecli/officecli-internal/pkg/ooxmledit"
)

func (m *Modifier) modifyPPTX(ctx context.Context, p Params, srcBytes []byte, progress engine.ProgressEmitter) (*Result, error) {
	meta := &ResultMeta{RouterPath: "B"}

	if progress != nil {
		progress.Emit(ctx, engine.ProgressEvent{Step: "modify_pptx", Status: "extracting_content"})
	}

	contentXMLs, err := ooxmledit.ExtractContentXML(srcBytes, ooxmledit.FileTypePPTX)
	if err != nil {
		return nil, fmt.Errorf("extract pptx content: %w", err)
	}

	maxSlide, err := ooxmledit.MaxSlideNum(srcBytes)
	if err != nil {
		return nil, fmt.Errorf("count slides: %w", err)
	}

	previews := buildSlidePreviews(contentXMLs, maxSlide, modifyPreviewSlides)
	if len(previews) < maxSlide {
		meta.Warnings = append(meta.Warnings, "truncated_preview")
	}

	prompt := edit.BuildPPTXEditPrompt(edit.PPTXEditPromptInput{
		Prompt:        p.Prompt,
		SlidePreviews: previews,
		Language:      p.Language,
		Style:         p.Style,
	})

	if progress != nil {
		progress.Emit(ctx, engine.ProgressEvent{Step: "modify_pptx", Status: "calling_llm"})
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
		progress.Emit(ctx, engine.ProgressEvent{Step: "modify_pptx", Status: "applying_ops"})
	}

	resultBytes := applyPPTXOps(srcBytes, resp, meta, maxSlide)

	pathBOps := meta.OpsApplied

	if len(resp.NeedsRewrite) > 0 {
		resultBytes = m.runPPTXEscalations(ctx, p, resultBytes, resp.NeedsRewrite, meta, progress)
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

func buildSlidePreviews(contentXMLs map[string]string, maxSlide, cap int) []edit.SlidePreview {
	limit := maxSlide
	if limit > cap {
		limit = cap
	}
	previews := make([]edit.SlidePreview, 0, limit)
	for i := 1; i <= limit; i++ {
		path := fmt.Sprintf("ppt/slides/slide%d.xml", i)
		xml, ok := contentXMLs[path]
		if !ok {
			continue
		}
		previews = append(previews, edit.SlidePreview{
			SlideIndex:  i,
			TextPreview: ooxmledit.ExtractSlideTextSummary(xml),
		})
	}
	return previews
}

func applyPPTXOps(srcBytes []byte, resp *edit.EditResponse, meta *ResultMeta, maxSlide int) []byte {
	current := srcBytes
	for _, raw := range resp.Ops {
		decoded, err := edit.DecodeOp(ooxmledit.FileTypePPTX, raw)
		if err != nil {
			meta.OpsFailed++
			meta.Warnings = append(meta.Warnings, err.Error())
			continue
		}
		op := decoded.(*ppt.ModifyOperation)

		if op.SlideIndex < 1 || op.SlideIndex > maxSlide {
			meta.OpsFailed++
			meta.Warnings = append(meta.Warnings, "target_outside_preview")
			continue
		}

		result, applyErr := ppt.ApplyModification(op.Intent, current, op)
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

func (m *Modifier) runPPTXEscalations(ctx context.Context, p Params, currentBytes []byte, targets []string, meta *ResultMeta, progress engine.ProgressEmitter) []byte {
	for _, target := range targets {
		if !strings.HasPrefix(target, "slide:") {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("unknown sentinel target: %s", target))
			continue
		}

		slideNum, err := strconv.Atoi(strings.TrimPrefix(target, "slide:"))
		if err != nil || slideNum < 1 {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("invalid sentinel target: %s", target))
			continue
		}

		if progress != nil {
			progress.Emit(ctx, engine.ProgressEvent{Step: "modify_pptx", Status: fmt.Sprintf("escalating_slide_%d", slideNum)})
		}

		slideXML, err := ppt.ExtractTargetSlideXML(currentBytes, slideNum)
		if err != nil {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("extract slide %d for escalation: %s", slideNum, err.Error()))
			continue
		}

		escalationPrompt := ppt.BuildRewriteSlideFallbackPrompt(ppt.RewriteSlidePromptInput{
			Intent:      "rewrite_entire_slide",
			Description: p.Prompt,
			SlideIndex:  slideNum,
			SlideXML:    slideXML,
		})

		raw, llmErr := callLLM(ctx, m.llm, escalationPrompt)
		meta.LLMCalls++
		if llmErr != nil {
			meta.Warnings = append(meta.Warnings, fmt.Sprintf("escalation llm call for slide %d: %s", slideNum, llmErr.Error()))
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
			decoded, decErr := edit.DecodeOp(ooxmledit.FileTypePPTX, opRaw)
			if decErr != nil {
				meta.OpsFailed++
				meta.Warnings = append(meta.Warnings, decErr.Error())
				continue
			}
			op := decoded.(*ppt.ModifyOperation)
			result, applyErr := ppt.ApplyModification(op.Intent, currentBytes, op)
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
