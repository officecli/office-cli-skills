package runtime

import (
	"context"
	"strings"

	"github.com/officecli/officecli/engine"
)

const (
	progressStepGenerateLLM = "generate_llm"
	progressStepAssemble    = "assemble"
)

func emitProgress(ctx context.Context, emitter engine.ProgressEmitter, step, status, content string) {
	if emitter == nil {
		return
	}
	emitter.Emit(ctx, engine.ProgressEvent{Step: step, Status: status, Content: strings.TrimSpace(content)})
}
