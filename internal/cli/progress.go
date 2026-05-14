package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/officecli/officecli-internal/engine"
)

const (
	progressStepInit         = "init"
	progressStepLicense      = "license"
	progressStepPlanPrepare  = "plan_prepare"
	progressStepQuestion     = "question"
	progressStepPlanConfirm  = "plan_confirm"
	progressStepGenerate     = "generate"
	progressStepGenerateLLM  = "generate_llm"
	progressStepAssemble     = "assemble"
	progressStepWriteFile    = "write_file"
	progressStepPublish      = "publish"
	progressStepReviewLint   = "review_lint"
	progressStepReviewPDF    = "review_pdf"
	progressStepReviewVisual = "review_visual"
	progressStepFinalize     = "finalize"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
var spinnerFrameInterval = 100 * time.Millisecond

type terminalDetector interface {
	IsTerminal() bool
}

type progressState struct {
	step    string
	message string
	frame   int
	active  bool
}

type ProgressRenderer struct {
	w                    io.Writer
	enabled              bool
	isTTY                bool
	transientCompletions bool
	mu                   sync.Mutex
	lastWidth            int
	state                progressState
	stopCh               chan struct{}
	doneCh               chan struct{}
}

func NewProgressRenderer(w io.Writer, jsonOutput bool, isTTY bool) *ProgressRenderer {
	r := &ProgressRenderer{w: w, enabled: w != nil && !jsonOutput, isTTY: isTTY}
	if r.enabled && r.isTTY {
		r.stopCh = make(chan struct{})
		r.doneCh = make(chan struct{})
		go r.loop()
	}
	return r
}

func (r *ProgressRenderer) SetTransientCompletions(enabled bool) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transientCompletions = enabled
}

func (r *ProgressRenderer) loop() {
	ticker := time.NewTicker(spinnerFrameInterval)
	defer ticker.Stop()
	defer close(r.doneCh)
	for {
		select {
		case <-ticker.C:
			r.mu.Lock()
			if r.state.active {
				r.state.frame = (r.state.frame + 1) % len(spinnerFrames)
				r.renderDynamicLocked()
			}
			r.mu.Unlock()
		case <-r.stopCh:
			return
		}
	}
}

func (r *ProgressRenderer) Emit(_ context.Context, event engine.ProgressEvent) {
	if r == nil || !r.enabled || r.w == nil {
		return
	}
	message := strings.TrimSpace(event.ActiveContent)
	if message == "" {
		message = strings.TrimSpace(event.Content)
	}
	if message == "" {
		message = defaultProgressMessage(event)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.isTTY {
		line := message
		_, _ = fmt.Fprintln(r.w, line)
		return
	}

	switch event.Status {
	case "running":
		r.state = progressState{step: event.Step, message: message, frame: 0, active: true}
		r.renderDynamicLocked()
	case "completed":
		r.state.active = false
		if r.transientCompletions {
			r.writeTTYLineLocked(fmt.Sprintf("✔ %s", message), false)
			return
		}
		r.writeTTYLineLocked(fmt.Sprintf("✔ %s", message), true)
	case "failed":
		r.state.active = false
		r.writeTTYLineLocked(fmt.Sprintf("✖ %s", message), true)
	default:
		r.state = progressState{step: event.Step, message: message, frame: 0, active: true}
		r.renderDynamicLocked()
	}
}

func (r *ProgressRenderer) Pause(message string) {
	if r == nil || !r.enabled || r.w == nil {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Waiting for the next action"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.isTTY {
		_, _ = fmt.Fprintf(r.w, "[20%%] %s\n", message)
		return
	}
	r.state.active = false
	r.writeTTYLineLocked(fmt.Sprintf("… %s", message), true)
}

func (r *ProgressRenderer) Close() {
	if r == nil || !r.enabled || !r.isTTY || r.stopCh == nil {
		return
	}
	close(r.stopCh)
	<-r.doneCh
	r.stopCh = nil
	r.doneCh = nil
}

func (r *ProgressRenderer) Clear() {
	if r == nil || !r.enabled || !r.isTTY || r.w == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = progressState{}
	if r.lastWidth == 0 {
		return
	}
	clear := strings.Repeat(" ", r.lastWidth)
	_, _ = fmt.Fprintf(r.w, "\r%s\r", clear)
	r.lastWidth = 0
}

func (r *ProgressRenderer) renderDynamicLocked() {
	frame := spinnerFrames[r.state.frame%len(spinnerFrames)]
	line := fmt.Sprintf("%s %s...", frame, r.state.message)
	r.writeTTYLineLocked(line, false)
}

func (r *ProgressRenderer) writeTTYLineLocked(line string, persist bool) {
	width := len([]rune(line))
	clear := ""
	if width < r.lastWidth {
		clear = strings.Repeat(" ", r.lastWidth-width)
	}
	if persist {
		_, _ = fmt.Fprintf(r.w, "\r%s%s\n", line, clear)
		r.lastWidth = 0
		return
	}
	_, _ = fmt.Fprintf(r.w, "\r%s%s", line, clear)
	r.lastWidth = width
}

func isTerminalWriter(w io.Writer) bool {
	if detector, ok := w.(terminalDetector); ok {
		return detector.IsTerminal()
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func defaultProgressMessage(event engine.ProgressEvent) string {
	switch event.Step {
	case progressStepLicense:
		return "Checking access status"
	case progressStepPlanPrepare:
		return "Preparing the generation plan"
	case progressStepQuestion:
		return "Waiting for follow-up input"
	case progressStepPlanConfirm:
		return "Confirming the generation plan"
	case progressStepGenerate, progressStepGenerateLLM:
		return "Generating document content"
	case progressStepAssemble:
		return "Assembling document files"
	case progressStepWriteFile:
		return "Writing local files"
	case progressStepPublish:
		return "Publishing online preview"
	case progressStepReviewLint:
		return "Running PPT structural checks"
	case progressStepReviewPDF:
		return "Converting PPT to PDF"
	case progressStepReviewVisual:
		return "Running visual quality review"
	case progressStepFinalize:
		return "Document generated"
	default:
		return strings.TrimSpace(event.Status)
	}
}

func emitProgress(ctx context.Context, emitter engine.ProgressEmitter, step, status, content string) {
	if emitter == nil {
		return
	}
	emitter.Emit(ctx, engine.ProgressEvent{Step: step, Status: status, Content: strings.TrimSpace(content)})
}
