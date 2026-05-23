package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

func TestBuildTUIGenerateJobInfersChineseDrawPictureAsIMG(t *testing.T) {
	job, err := BuildTUIGenerateJob("画一个图，关于长江", Config{
		Defaults: DefaultsConfig{OutputDir: "./output", Mode: "fast", Publish: false},
	}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildTUIGenerateJob: %v", err)
	}
	if job.DocumentType != engine.DocumentTypeIMG {
		t.Fatalf("document type = %q", job.DocumentType)
	}
}

func TestBuildTUIGenerateJobForTypeUsesExplicitDocumentType(t *testing.T) {
	job, err := BuildTUIGenerateJobForType("做一个word内容随意", "pptx", Config{
		Defaults: DefaultsConfig{OutputDir: "./output", Mode: "fast", Publish: false},
	}, InputSources{IsTTY: true, CWD: t.TempDir()}, "")
	if err != nil {
		t.Fatalf("BuildTUIGenerateJobForType: %v", err)
	}
	if job.DocumentType != engine.DocumentTypePPTX {
		t.Fatalf("document type = %q", job.DocumentType)
	}
	if job.Prompt != "做一个word内容随意" {
		t.Fatalf("prompt = %q", job.Prompt)
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

func TestTUIModelFooterSuppressesExternalTrialWhenZeroOrLoggedIn(t *testing.T) {
	model := newTUIModel(&App{}, Config{Runtime: RuntimeConfig{Mode: RuntimeModeExternal}}, TUIOptions{}, "", io.Discard)
	updated, _ := model.Update(tuiAccessStatusMsg{Result: &LicenseCheckResult{
		AccessMode:    LicenseAccessModeFree,
		FreeRemaining: 0,
	}})
	model = updated.(tuiModel)
	view := model.View()
	if !strings.Contains(view, "Mode: external") {
		t.Fatalf("external footer should still show mode:\n%s", view)
	}
	if strings.Contains(view, "Trial:") {
		t.Fatalf("external footer should NOT show 'Trial:' when FreeRemaining is 0:\n%s", view)
	}

	cfg := Config{Runtime: RuntimeConfig{Mode: RuntimeModeExternal}}
	cfg.License.SessionToken = "ocli_sess_test"
	model = newTUIModel(&App{}, cfg, TUIOptions{}, "", io.Discard)
	updated, _ = model.Update(tuiAccessStatusMsg{Result: &LicenseCheckResult{
		AccessMode:    LicenseAccessModeFree,
		FreeRemaining: 5,
	}})
	model = updated.(tuiModel)
	view = model.View()
	if strings.Contains(view, "Trial:") {
		t.Fatalf("external footer should NOT show 'Trial:' for logged-in user:\n%s", view)
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
	if !strings.Contains(view, "Unknown command: /clear") {
		t.Fatalf("/clear should be rejected as an unknown command:\n%s", view)
	}
	if strings.Contains(view, "Choose file type") {
		t.Fatalf("/clear should not start document generation:\n%s", view)
	}
	if !strings.Contains(view, "> Describe a document") {
		t.Fatalf("unknown /clear should leave the input usable:\n%s", view)
	}
}

func TestTUIModelSlashLoginRunsDeviceLoginAndRefreshesQuota(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("OFFICE_CLI_CONFIG", configPath)

	expiresAt := time.Now().Add(10 * time.Minute).UTC().Truncate(time.Second)
	var pollCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/cli/login/start":
			_, _ = fmt.Fprintf(w, `{"data":{"challenge_id":"cli_test","login_url":"http://%s/api/cli/login/verify?user_code=ABCD-EFGH","user_code":"ABCD-EFGH","verification_url":"http://%s/api/cli/login/verify","poll_interval_seconds":1,"expires_at":%q}}`, r.Host, r.Host, expiresAt.Format(time.RFC3339))
		case "/api/cli/login/poll":
			pollCount++
			_, _ = fmt.Fprintf(w, `{"data":{"status":"completed","expires_at":%q}}`, expiresAt.Format(time.RFC3339))
		case "/api/cli/login/exchange":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode exchange: %v", err)
			}
			if payload["challenge_id"] != "cli_test" || payload["code_verifier"] == "" {
				t.Fatalf("exchange payload = %+v", payload)
			}
			_, _ = fmt.Fprintf(w, `{"data":{"token":"ocli_sess_new","token_prefix":"ocli_sess","user_id":42,"user_email":"dev@example.com","expires_at":%q}}`, expiresAt.Format(time.RFC3339))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := WriteConfig("", Config{
		Runtime: RuntimeConfig{Mode: RuntimeModeHosted},
		License: LicenseConfig{
			BaseURL:    server.URL,
			Enabled:    true,
			TimeoutSec: 60,
		},
	}, true)
	if err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}

	app := NewApp(&bytes.Buffer{}, &bytes.Buffer{}, bytes.NewBuffer(nil))
	app.openBrowser = func(string) error { return fmt.Errorf("browser unavailable") }
	app.newLicenseService = func(cfg LicenseConfig) (LicenseManager, error) {
		if strings.TrimSpace(cfg.SessionToken) != "ocli_sess_new" {
			t.Fatalf("quota refresh did not load saved session token: %#v", cfg.SessionToken)
		}
		return stubLicenseManager{checkResult: &LicenseCheckResult{
			Allowed:       true,
			AccessMode:    LicenseAccessModeHosted,
			CreditBalance: 77,
		}}, nil
	}
	model := newTUIModel(app, Config{
		Runtime: RuntimeConfig{Mode: RuntimeModeHosted},
		License: LicenseConfig{
			BaseURL:    server.URL,
			Enabled:    true,
			TimeoutSec: 60,
		},
	}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("/login")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if cmd == nil {
		t.Fatal("expected /login to start a login command")
	}
	model = runTUICommandsUntil(t, model, cmd, func(model tuiModel) bool {
		view := model.View()
		return strings.Contains(view, "Logged in as dev@example.com") && strings.Contains(view, "Credits: 77")
	})

	if pollCount == 0 {
		t.Fatal("poll endpoint was not called")
	}
	view := model.View()
	for _, needle := range []string{"Login URL:", "ABCD-EFGH", "Could not open your browser automatically", "Logged in as dev@example.com", "Credits: 77"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("TUI login output missing %q:\n%s", needle, view)
		}
	}
	if model.state != tuiStateIdle {
		t.Fatalf("state after login = %q", model.state)
	}
}

func TestTUIModelSlashLoginBlocksGenerationInput(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.state = tuiStateLogin
	model.input.SetValue("做一个 PPT")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("login input should not start a command")
	}
	if model.currentJob != nil || model.state != tuiStateLogin {
		t.Fatalf("login input should not start generation, state=%q job=%#v", model.state, model.currentJob)
	}
	if !strings.Contains(model.View(), "Login is already running") {
		t.Fatalf("missing login-running message:\n%s", model.View())
	}
}

func TestTUIModelCtrlCCancelsLogin(t *testing.T) {
	cancelled := false
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.state = tuiStateLogin
	model.cancel = func() { cancelled = true }

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("ctrl-c while logging in should not quit")
	}
	if !cancelled {
		t.Fatal("expected login ctrl-c to cancel the login context")
	}
	if model.state != tuiStateIdle {
		t.Fatalf("state after login cancel = %q", model.state)
	}
	if !strings.Contains(model.View(), "Login cancelled") {
		t.Fatalf("missing login cancellation message:\n%s", model.View())
	}
}

func TestTUIModelHelpWrapsAndListsCurrentCommands(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 64, Height: 20})
	model = updated.(tuiModel)
	model.input.SetValue("/help")

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)

	view := model.View()
	for _, needle := range []string{"/help", "/login", "/exit", "Discord: https://discord.gg/ezAHMkdG"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("help/footer missing %q:\n%s", needle, view)
		}
	}
	if strings.Contains(view, "/clear") {
		t.Fatalf("help/footer should not mention /clear:\n%s", view)
	}
	rendered := model.renderEntries()
	if strings.Count(rendered, "\n") < 8 {
		t.Fatalf("help should render as multiple lines:\n%s", rendered)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if width := lipgloss.Width(line); width > model.view.Width {
			t.Fatalf("help line width = %d, want <= %d: %q\n%s", width, model.view.Width, line, rendered)
		}
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

func TestTUIModelWrapsLongErrorLines(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 64, Height: 20})
	model = updated.(tuiModel)
	model.append("error", "content generation failed: internal LLM request failed: status=400 body={\"error\":\"hosted upstream request failed because this token value is invalid and the response body is very long\"}")

	rendered := model.renderEntries()
	lines := strings.Split(rendered, "\n")
	errorLineCount := 0
	for _, line := range lines {
		if strings.Contains(line, "Error:") || strings.Contains(line, "status=400") || strings.Contains(line, "invalid") || strings.Contains(line, "response body") {
			errorLineCount++
			if width := lipgloss.Width(line); width > model.view.Width {
				t.Fatalf("error line width = %d, want <= %d: %q\n%s", width, model.view.Width, line, rendered)
			}
		}
	}
	if errorLineCount < 2 {
		t.Fatalf("expected long error to wrap onto multiple lines:\n%s", rendered)
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

func TestTUIModelPromptEnterShowsTypeSelector(t *testing.T) {
	model := newTUIModel(&App{}, Config{Defaults: DefaultsConfig{Mode: "fast"}}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("做一个word内容随意")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if model.state != tuiStateTypeSelect {
		t.Fatalf("state after prompt submit = %q", model.state)
	}
	if cmd != nil {
		t.Fatal("type selection should not start generation yet")
	}
	if model.currentJob != nil {
		t.Fatalf("current job before type confirmation = %#v", model.currentJob)
	}
	view := model.View()
	for _, needle := range []string{"Choose file type", "> docx", "pptx", "xlsx", "img", "report"} {
		if !strings.Contains(view, needle) {
			t.Fatalf("type selector missing %q:\n%s", needle, view)
		}
	}
}

func TestTUIModelTypeSelectorArrowsAndEnterStartSelectedType(t *testing.T) {
	model := newTUIModel(&App{}, Config{Defaults: DefaultsConfig{Mode: "fast"}}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("做一个word内容随意")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("prompt submit should only open type selector")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(tuiModel)
	if !strings.Contains(model.View(), "> pptx") {
		t.Fatalf("down should select pptx:\n%s", model.View())
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if model.state != tuiStateRunning {
		t.Fatalf("state after type confirmation = %q", model.state)
	}
	if cmd == nil {
		t.Fatal("expected generation command after type confirmation")
	}
	if model.currentJob == nil || model.currentJob.DocumentType != engine.DocumentTypePPTX {
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

func TestTUIModelTypeSelectorUsesInferredImageDefault(t *testing.T) {
	model := newTUIModel(&App{}, Config{Defaults: DefaultsConfig{Mode: "fast"}}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("画一个图，关于长江")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("prompt submit should only open type selector")
	}
	if model.state != tuiStateTypeSelect {
		t.Fatalf("state = %q", model.state)
	}
	if !strings.Contains(model.View(), "> img") {
		t.Fatalf("image prompt should default to img:\n%s", model.View())
	}
}

func TestTUIModelUpDownNavigatesPromptHistory(t *testing.T) {
	model := newTUIModel(&App{}, Config{Defaults: DefaultsConfig{Mode: "fast"}}, TUIOptions{}, "", io.Discard)

	model.input.SetValue("first prompt")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(tuiModel)

	model.input.SetValue("second prompt")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(tuiModel)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("history navigation should not start a command")
	}
	if model.input.Value() != "second prompt" {
		t.Fatalf("first up input = %q", model.input.Value())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(tuiModel)
	if model.input.Value() != "first prompt" {
		t.Fatalf("second up input = %q", model.input.Value())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(tuiModel)
	if model.input.Value() != "second prompt" {
		t.Fatalf("down input = %q", model.input.Value())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(tuiModel)
	if model.input.Value() != "" {
		t.Fatalf("down at newest input = %q", model.input.Value())
	}
}

func TestTUIModelHistoryDownRestoresDraft(t *testing.T) {
	model := newTUIModel(&App{}, Config{Defaults: DefaultsConfig{Mode: "fast"}}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("previous prompt")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(tuiModel)

	model.input.SetValue("draft prompt")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(tuiModel)
	if model.input.Value() != "previous prompt" {
		t.Fatalf("up input = %q", model.input.Value())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(tuiModel)
	if model.input.Value() != "draft prompt" {
		t.Fatalf("down should restore draft, got %q", model.input.Value())
	}
}

func TestTUIModelReportTypeUsesWorkbookPathFromPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	workbookPath := tmpDir + "/metrics.xlsx"
	model := newTUIModel(&App{}, Config{Defaults: DefaultsConfig{Mode: "fast"}}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("根据 " + workbookPath + " 做一个经营 report")

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	model.selectedTypeIndex = 4
	model.refreshTypeSelectionEntry()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if model.state != tuiStateRunning {
		t.Fatalf("state after report confirmation = %q", model.state)
	}
	if cmd == nil {
		t.Fatal("expected generation command after report confirmation")
	}
	if model.currentJob == nil || model.currentJob.DocumentType != engine.DocumentTypeReport {
		t.Fatalf("current job = %#v", model.currentJob)
	}
	if model.currentJob.SourceFilePath != workbookPath {
		t.Fatalf("source file = %q", model.currentJob.SourceFilePath)
	}
}

func TestTUIModelReportTypePromptsForMissingWorkbookPath(t *testing.T) {
	model := newTUIModel(&App{}, Config{Defaults: DefaultsConfig{Mode: "fast"}}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("做一个经营 report")

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	model.selectedTypeIndex = 4
	model.refreshTypeSelectionEntry()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("missing report path should not start generation")
	}
	if model.state != tuiStateReportFile {
		t.Fatalf("state after report confirmation = %q", model.state)
	}
	if !strings.Contains(model.View(), "Enter the .xlsx file path") {
		t.Fatalf("missing report file prompt:\n%s", model.View())
	}

	model.input.SetValue("/tmp/metrics.xlsx")
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	if model.state != tuiStateRunning {
		t.Fatalf("state after report path = %q", model.state)
	}
	if cmd == nil {
		t.Fatal("expected generation command after report path")
	}
	if model.currentJob == nil || model.currentJob.SourceFilePath != "/tmp/metrics.xlsx" {
		t.Fatalf("current job = %#v", model.currentJob)
	}
}

func TestTUIModelCtrlCCancelsTypeSelection(t *testing.T) {
	model := newTUIModel(&App{}, Config{}, TUIOptions{}, "", io.Discard)
	model.input.SetValue("做一个word内容随意")

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(tuiModel)
	if cmd != nil {
		t.Fatal("ctrl-c in type selector should not quit")
	}
	if model.state != tuiStateIdle {
		t.Fatalf("state after ctrl-c = %q", model.state)
	}
	if model.pendingPrompt != "" {
		t.Fatalf("pending prompt = %q", model.pendingPrompt)
	}
	if !strings.Contains(model.View(), "Selection cancelled") {
		t.Fatalf("missing cancellation message:\n%s", model.View())
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

func runTUICommandsUntil(t *testing.T, model tuiModel, cmd tea.Cmd, done func(tuiModel) bool) tuiModel {
	t.Helper()
	for i := 0; i < 40; i++ {
		if cmd == nil {
			t.Fatalf("command stream ended before condition was met:\n%s", model.View())
		}
		msg := cmd()
		updated, next := model.Update(msg)
		model = updated.(tuiModel)
		if done(model) {
			return model
		}
		cmd = next
	}
	t.Fatalf("condition was not met after processing TUI commands:\n%s", model.View())
	return model
}

func TestTUIModelModeCommandSwitchesRuntimeMode(t *testing.T) {
	model := newTUIModel(&App{}, Config{Runtime: RuntimeConfig{Mode: RuntimeModeHosted}}, TUIOptions{}, "", io.Discard)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	model = updated.(tuiModel)

	view := model.View()
	if !strings.Contains(view, "Mode: hosted") {
		t.Fatalf("default footer should show hosted:\n%s", view)
	}

	model.input.SetValue("/mode external")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	view = model.View()
	if !strings.Contains(view, "Mode: external") {
		t.Fatalf("/mode external should switch footer to external:\n%s", view)
	}
	if model.cfg.Runtime.Mode != RuntimeModeExternal {
		t.Fatalf("cfg.Runtime.Mode = %q, want external", model.cfg.Runtime.Mode)
	}

	model.input.SetValue("/mode hosted")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	view = model.View()
	if !strings.Contains(view, "Mode: hosted") {
		t.Fatalf("/mode hosted should switch footer back to hosted:\n%s", view)
	}
	if model.cfg.Runtime.Mode != RuntimeModeHosted {
		t.Fatalf("cfg.Runtime.Mode = %q, want hosted", model.cfg.Runtime.Mode)
	}
}

func TestTUIModelModeCommandWithoutArgsShowsCurrentMode(t *testing.T) {
	model := newTUIModel(&App{}, Config{Runtime: RuntimeConfig{Mode: RuntimeModeHosted}}, TUIOptions{}, "", io.Discard)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	model = updated.(tuiModel)

	model.input.SetValue("/mode")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	view := model.View()
	if !strings.Contains(view, "Current mode: hosted") {
		t.Fatalf("/mode without args should report current mode:\n%s", view)
	}
	if !strings.Contains(view, "/mode [hosted|external]") {
		t.Fatalf("/mode without args should show usage hint:\n%s", view)
	}
	if model.cfg.Runtime.Mode != RuntimeModeHosted {
		t.Fatalf("/mode without args should not change mode, got %q", model.cfg.Runtime.Mode)
	}
}

func TestTUIModelModeCommandRejectsUnknownMode(t *testing.T) {
	model := newTUIModel(&App{}, Config{Runtime: RuntimeConfig{Mode: RuntimeModeHosted}}, TUIOptions{}, "", io.Discard)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	model = updated.(tuiModel)

	model.input.SetValue("/mode bogus")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(tuiModel)
	view := model.View()
	if !strings.Contains(view, "Unknown mode: bogus") {
		t.Fatalf("/mode bogus should report an unknown mode error:\n%s", view)
	}
	if model.cfg.Runtime.Mode != RuntimeModeHosted {
		t.Fatalf("/mode bogus should not change mode, got %q", model.cfg.Runtime.Mode)
	}
	if !strings.Contains(view, "Mode: hosted") {
		t.Fatalf("footer should still show hosted after rejected /mode:\n%s", view)
	}
}

func TestTUIHelpTextListsModeCommand(t *testing.T) {
	if !strings.Contains(tuiHelpText, "/mode [hosted|external]") {
		t.Fatalf("tuiHelpText should advertise /mode command:\n%s", tuiHelpText)
	}
}
