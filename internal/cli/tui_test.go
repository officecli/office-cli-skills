package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/officecli/officecli-internal/engine"
)

func TestBuildTUIGenerateJobDefaultsNaturalLanguageToPPTX(t *testing.T) {
	job, err := BuildTUIGenerateJob("做一个 6 页 Q3 业务复盘 PPT", Config{
		Defaults: DefaultsConfig{OutputDir: "./output", Mode: "fast", Publish: false},
	}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildTUIGenerateJob: %v", err)
	}
	if job.DocumentType != engine.DocumentTypePPTX {
		t.Fatalf("document type = %q", job.DocumentType)
	}
	if job.Topic != "做一个 6 页 Q3 业务复盘 PPT" {
		t.Fatalf("topic = %q", job.Topic)
	}
	if job.Prompt != "做一个 6 页 Q3 业务复盘 PPT" {
		t.Fatalf("prompt = %q", job.Prompt)
	}
	if job.JSONOutput {
		t.Fatal("TUI jobs should render human progress, not JSON output")
	}
}

func TestBuildTUIGenerateJobInfersWordAsDOCX(t *testing.T) {
	job, err := BuildTUIGenerateJob("做一个word内容随意", Config{
		Defaults: DefaultsConfig{OutputDir: "./output", Mode: "fast", Publish: false},
	}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildTUIGenerateJob: %v", err)
	}
	if job.DocumentType != engine.DocumentTypeDOCX {
		t.Fatalf("document type = %q", job.DocumentType)
	}
}

func TestTUIModelRenderShowsResultAndReturnsIdleText(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.state = tuiStateIdle
	model.append("assistant", formatTUIResult(GenerateResult{
		Status:       "ok",
		FilePath:     "/tmp/launch-brief.docx",
		DocumentType: "docx",
		DocumentName: "launch-brief",
	}))

	view := model.View()
	for _, needle := range []string{"/tmp/launch-brief.docx", "docx", "Enter another document request"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("view missing %q:\n%s", needle, view)
		}
	}
}

func TestTUIModelBottomAlignsShortConversationNearInput(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	model = updated.(tuiModel)

	view := model.View()
	lines := strings.Split(view, "\n")
	bannerLine := lineIndexContaining(lines, "╭─── OfficeCLI")
	logoLine := lineIndexContaining(lines, "███ ███ ███ ███ ███ ███ ███ █   ███")
	tipLine := lineIndexContaining(lines, "Tip: Type")
	inputLine := lineIndexContaining(lines, "> Describe a document")
	if bannerLine < 0 {
		t.Fatalf("startup banner not found:\n%s", view)
	}
	if logoLine < 0 {
		t.Fatalf("solid OfficeCLI logo not found:\n%s", view)
	}
	if tipLine < 0 || !strings.Contains(view, "Word document") || !strings.Contains(view, "AI agents") {
		t.Fatalf("startup tip should teach a Word document prompt:\n%s", view)
	}
	for _, removed := range []string{"Welcome to OfficeCLI", "▗▟▙", "▐▛", "▝▙", "▘▘"} {
		if strings.Contains(view, removed) {
			t.Fatalf("startup banner should not contain removed mascot/welcome content %q:\n%s", removed, view)
		}
	}
	if inputLine < 0 {
		t.Fatalf("input prompt not found:\n%s", view)
	}
	if inputLine-logoLine > 13 {
		t.Fatalf("short conversation should sit near the input, logo line=%d input line=%d:\n%s", logoLine, inputLine, view)
	}
}

func TestTUIModelFooterShowsRuntimeModeAndQuota(t *testing.T) {
	model := newTUIModel(&App{}, Config{Runtime: RuntimeConfig{Mode: RuntimeModeHosted}}, TUIOptions{}, "", io.Discard)
	view := model.View()
	if !strings.Contains(view, "Mode: hosted") || !strings.Contains(view, "Credits: checking") {
		t.Fatalf("hosted footer should show mode and checking credits:\n%s", view)
	}

	updated, _ := model.Update(tuiAccessStatusMsg{Result: &LicenseCheckResult{
		AccessMode:    LicenseAccessModeHosted,
		CreditBalance: 17,
	}})
	model = updated.(tuiModel)
	view = model.View()
	if !strings.Contains(view, "Mode: hosted") || !strings.Contains(view, "Credits: 17") {
		t.Fatalf("hosted footer should show credits:\n%s", view)
	}

	model = newTUIModel(&App{}, Config{Runtime: RuntimeConfig{Mode: RuntimeModeExternal}}, TUIOptions{}, "", io.Discard)
	updated, _ = model.Update(tuiAccessStatusMsg{Result: &LicenseCheckResult{
		AccessMode:         LicenseAccessModePaid,
		PaidQuotaRemaining: 42,
	}})
	model = updated.(tuiModel)
	view = model.View()
	if !strings.Contains(view, "Mode: external") || !strings.Contains(view, "Generations: 42") {
		t.Fatalf("external footer should show paid generation quota:\n%s", view)
	}

	updated, _ = model.Update(tuiAccessStatusMsg{Result: &LicenseCheckResult{
		AccessMode:    LicenseAccessModeFree,
		FreeRemaining: 3,
	}})
	model = updated.(tuiModel)
	view = model.View()
	if !strings.Contains(view, "Mode: external") || !strings.Contains(view, "Trial: 3 generations") {
		t.Fatalf("trial footer should show trial generation quota:\n%s", view)
	}
}

func TestTUIModelFooterRefreshesQuotaAfterGeneration(t *testing.T) {
	model := newTUIModel(&App{}, Config{Runtime: RuntimeConfig{Mode: RuntimeModeHosted}}, TUIOptions{}, "", io.Discard)
	updated, _ := model.Update(tuiGenerationFinishedMsg{GenerationID: 0, Result: GenerateResult{
		Status:        "ok",
		FilePath:      "/tmp/brief.docx",
		DocumentType:  "docx",
		RuntimeMode:   "hosted",
		AccessMode:    "hosted",
		CreditBalance: 11,
	}})
	model = updated.(tuiModel)
	view := model.View()
	if !strings.Contains(view, "Mode: hosted") || !strings.Contains(view, "Credits: 11") {
		t.Fatalf("footer should refresh from generation result:\n%s", view)
	}
}

func TestTUIModelClearRemovesContentWithoutOldPadding(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	model = updated.(tuiModel)
	model.input.SetValue("/clear")

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	view := model.View()
	if strings.Contains(view, "Welcome to OfficeCLI") {
		t.Fatalf("/clear should remove previous conversation content:\n%s", view)
	}
	if !strings.Contains(view, "> Describe a document") {
		t.Fatalf("/clear should leave the input usable:\n%s", view)
	}
}

func TestTUIModelShowsProgressAndWarnings(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.state = tuiStateRunning
	updated, _ := model.Update(tuiProgressMsg{Event: engine.ProgressEvent{
		Step:    progressStepLicense,
		Status:  "running",
		Content: "Checking access status",
	}})
	model = updated.(tuiModel)
	model.append("assistant", formatTUIResult(GenerateResult{
		Status:       "ok",
		FilePath:     "/tmp/q3.pptx",
		DocumentType: "pptx",
		Warnings:     []string{"部分图片生成失败，已降级"},
	}))

	view := model.View()
	for _, needle := range []string{"Checking access status", "部分图片生成失败", "/tmp/q3.pptx"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("view missing %q:\n%s", needle, view)
		}
	}
}

func TestTUIProgressPrompterSendsQuestionsAndReceivesAnswer(t *testing.T) {
	events := make(chan tea.Msg, 1)
	prompter := newTUIProgressPrompter(events, 7)
	done := make(chan tuiQuestionAnswer, 1)

	go func() {
		optionID, answer, err := prompter.Ask("Who is the audience?", []string{"Leadership", "Team"}, false)
		done <- tuiQuestionAnswer{optionID: optionID, answer: answer, err: err}
	}()
	msg := (<-events).(tuiQuestionMsg)
	if msg.GenerationID != 7 || msg.Question != "Who is the audience?" {
		t.Fatalf("question message = %#v", msg)
	}
	msg.Reply <- tuiQuestionAnswer{optionID: "1", answer: "Leadership"}
	answer := <-done
	if answer.err != nil {
		t.Fatalf("Ask: %v", answer.err)
	}
	if answer.optionID != "1" || answer.answer != "Leadership" {
		t.Fatalf("answer = (%q, %q)", answer.optionID, answer.answer)
	}
}

func TestTUIModelSlashExitQuits(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("/exit")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected /exit to quit")
	}
}

func TestTUIModelQuestionAcceptsAnswer(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.state = tuiStateRunning
	model.generationID = 3
	reply := make(chan tuiQuestionAnswer, 1)

	updated, _ := model.Update(tuiQuestionMsg{
		GenerationID:  3,
		Question:      "Who is the audience?",
		Options:       []string{"Leadership", "Team"},
		AllowFreeform: true,
		Reply:         reply,
	})
	model = updated.(tuiModel)
	if model.state != tuiStateQuestion {
		t.Fatalf("state = %q", model.state)
	}
	if !strings.Contains(model.View(), "Who is the audience?") || !strings.Contains(model.View(), "2. Team") {
		t.Fatalf("question not rendered:\n%s", model.View())
	}

	model.input.SetValue("2")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	answer := <-reply
	if answer.err != nil {
		t.Fatalf("answer error: %v", answer.err)
	}
	if answer.optionID != "2" || answer.answer != "Team" {
		t.Fatalf("answer = %#v", answer)
	}
	if model.state != tuiStateRunning {
		t.Fatalf("state after answer = %q", model.state)
	}
}

func TestTUIModelCancelledGenerationSuppressesLateEvents(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.state = tuiStateRunning
	model.generationID = 9
	model.cancelled = true

	updated, _ := model.Update(tuiProgressMsg{GenerationID: 9, Event: engine.ProgressEvent{
		Status:  "running",
		Content: "late progress",
	}})
	model = updated.(tuiModel)
	if strings.Contains(model.View(), "late progress") {
		t.Fatalf("cancelled progress should not render:\n%s", model.View())
	}

	reply := make(chan tuiQuestionAnswer, 1)
	updated, _ = model.Update(tuiQuestionMsg{
		GenerationID:  9,
		Question:      "late question",
		AllowFreeform: true,
		Reply:         reply,
	})
	model = updated.(tuiModel)
	answer := <-reply
	if !errors.Is(answer.err, context.Canceled) {
		t.Fatalf("question answer err = %v", answer.err)
	}
	if strings.Contains(model.View(), "late question") {
		t.Fatalf("cancelled question should not render:\n%s", model.View())
	}
}
func TestTUIProgressEmitterForwardsEvents(t *testing.T) {
	progress := make(chan tea.Msg, 1)
	emitter := tuiProgressEmitter{events: progress}

	emitter.Emit(context.Background(), engine.ProgressEvent{Step: progressStepGenerate, Status: "running", Content: "Generating"})
	msg := <-progress
	event, ok := msg.(tuiProgressMsg)
	if !ok {
		t.Fatalf("message = %T", msg)
	}
	if !strings.Contains(formatTUIProgress(event.Event), "Generating") {
		t.Fatalf("progress was not forwarded: %#v", event)
	}
}

func TestTUIModelTransitionsRunningCompletedIdle(t *testing.T) {
	model := newTUIModel(&App{}, Config{Defaults: DefaultsConfig{Mode: "fast"}}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("做一个word内容随意")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if model.state != tuiStateRunning {
		t.Fatalf("state after submit = %q", model.state)
	}
	if cmd == nil {
		t.Fatal("expected generation command after submit")
	}
	if model.currentJob == nil || model.currentJob.DocumentType != engine.DocumentTypeDOCX {
		t.Fatalf("current job = %#v", model.currentJob)
	}

	updated, _ = model.Update(tuiGenerationFinishedMsg{GenerationID: model.generationID, Result: GenerateResult{
		Status:       "ok",
		FilePath:     "/tmp/brief.docx",
		DocumentType: "docx",
	}})
	model = updated.(tuiModel)
	if model.state != tuiStateIdle {
		t.Fatalf("state after finish = %q", model.state)
	}
	view := model.View()
	for _, needle := range []string{"/tmp/brief.docx", "docx", "Enter another document request"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("view missing %q:\n%s", needle, view)
		}
	}
}

func TestTUIModelShowsProgressWarningAndError(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	updated, _ := model.Update(tuiProgressMsg{Event: engine.ProgressEvent{
		Step:    progressStepGenerate,
		Status:  "running",
		Content: "Generating document content",
	}})
	model = updated.(tuiModel)
	updated, _ = model.Update(tuiGenerationFinishedMsg{Result: GenerateResult{
		Status:       "ok",
		FilePath:     "/tmp/q3.pptx",
		DocumentType: "pptx",
		Warnings:     []string{"部分图片生成失败"},
	}})
	model = updated.(tuiModel)
	updated, _ = model.Update(tuiGenerationFinishedMsg{Err: errors.New("boom")})
	model = updated.(tuiModel)

	view := model.View()
	for _, needle := range []string{"Generating document content", "部分图片生成失败", "/tmp/q3.pptx", "boom"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("view missing %q:\n%s", needle, view)
		}
	}
}

func TestTUIModelSlashExitAndCtrlC(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("/exit")
	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected /exit to quit")
	}

	model = newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("first empty ctrl-c should show a quit hint, not quit")
	}
	if !model.exitArmed {
		t.Fatal("expected first empty ctrl-c to arm exit")
	}
	if !strings.Contains(model.View(), "Press Ctrl+C again to exit") {
		t.Fatalf("missing double ctrl-c hint:\n%s", model.View())
	}
	_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected second empty ctrl-c to quit while idle")
	}

	model = newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("draft prompt")
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("ctrl-c with input should clear, not quit")
	}
	if model.input.Value() != "" {
		t.Fatalf("input after ctrl-c = %q", model.input.Value())
	}
	if !model.exitArmed {
		t.Fatal("expected ctrl-c clearing input to arm exit")
	}
	_, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected ctrl-c after clearing input to quit")
	}

	model = newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.state = tuiStateRunning
	cancelled := false
	model.cancel = func() { cancelled = true }
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("ctrl-c while running should cancel, not quit immediately")
	}
	if !cancelled {
		t.Fatal("expected running ctrl-c to call cancel")
	}
	if !model.cancelled {
		t.Fatal("expected running ctrl-c to mark generation as cancelled")
	}
	if model.state != tuiStateIdle {
		t.Fatalf("state after cancel = %q", model.state)
	}
}

func TestTUIModelCtrlCClearsQuestionInputBeforeCancelling(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.state = tuiStateQuestion
	model.question = &tuiQuestionMsg{Reply: make(chan tuiQuestionAnswer, 1)}
	model.input.SetValue("partial answer")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("ctrl-c with question input should clear, not quit")
	}
	if model.input.Value() != "" {
		t.Fatalf("input after ctrl-c = %q", model.input.Value())
	}
	if model.state != tuiStateQuestion {
		t.Fatalf("state after clearing question input = %q", model.state)
	}
	if !model.exitArmed {
		t.Fatal("expected clearing question input to arm the next ctrl-c")
	}
}

func lineIndexContaining(lines []string, needle string) int {
	for idx, line := range lines {
		if strings.Contains(line, needle) {
			return idx
		}
	}
	return -1
}
