package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/officecli/officecli-internal/engine"
)

const (
	tuiStateIdle     = "idle"
	tuiStateRunning  = "running"
	tuiStateQuestion = "question"
)

const tuiStartupBanner = `╭─── OfficeCLI ─────────────────────────╮
│                                       │
│  ███ ███ ███ ███ ███ ███ ███ █   ███ │
│  █ █ █   █    █  █   █   █   █    █  │
│  █ █ ██  ██   █  █   ███ █   █    █  │
│  █ █ █   █    █  █   █   █   █    █  │
│  ███ █   █   ███ ███ ███ ███ ███ ███ │
╰───────────────────────────────────────╯`

const tuiStartupTip = `Tip: Type "Create a Word document about AI agents" and press Enter.`

type TUIOptions struct {
	NoAltScreen bool
}

type tuiEntry struct {
	role string
	text string
}

type tuiQuestionAnswer struct {
	optionID string
	answer   string
	err      error
}

type tuiProgressMsg struct {
	GenerationID int
	Event        engine.ProgressEvent
}

type tuiPauseMsg struct {
	GenerationID int
	Message      string
}

type tuiQuestionMsg struct {
	GenerationID  int
	Question      string
	Options       []string
	AllowFreeform bool
	Reply         chan tuiQuestionAnswer
}

type tuiGenerationFinishedMsg struct {
	GenerationID int
	Result       GenerateResult
	Err          error
}

type tuiAccessStatusMsg struct {
	Result *LicenseCheckResult
	Err    error
}

type tuiInitialPromptMsg struct{}
type tuiNoopMsg struct{}

type tuiProgressEmitter struct {
	events       chan<- tea.Msg
	generationID int
}

func (e tuiProgressEmitter) Emit(_ context.Context, event engine.ProgressEvent) {
	if e.events != nil {
		e.events <- tuiProgressMsg{GenerationID: e.generationID, Event: event}
	}
}

func (e tuiProgressEmitter) Pause(message string) {
	if e.events != nil {
		e.events <- tuiPauseMsg{GenerationID: e.generationID, Message: strings.TrimSpace(message)}
	}
}

type tuiProgressPrompter struct {
	events       chan<- tea.Msg
	generationID int
}

func newTUIProgressPrompter(events chan<- tea.Msg, generationID int) *tuiProgressPrompter {
	return &tuiProgressPrompter{events: events, generationID: generationID}
}

func (p *tuiProgressPrompter) Ask(question string, options []string, allowFreeform bool) (string, string, error) {
	if p.events == nil {
		return "", "", fmt.Errorf("tui question channel is unavailable")
	}
	reply := make(chan tuiQuestionAnswer, 1)
	p.events <- tuiQuestionMsg{
		GenerationID:  p.generationID,
		Question:      question,
		Options:       append([]string(nil), options...),
		AllowFreeform: allowFreeform,
		Reply:         reply,
	}
	answer := <-reply
	return answer.optionID, answer.answer, answer.err
}

type tuiModel struct {
	app     *App
	cfg     Config
	opts    TUIOptions
	rootCtx context.Context
	cwd     string
	input   textinput.Model
	view    viewport.Model
	entries []tuiEntry
	state   string

	width  int
	height int

	initialPrompt string
	currentJob    *GenerateJob
	events        chan tea.Msg
	cancel        context.CancelFunc
	question      *tuiQuestionMsg
	generationID  int
	cancelled     bool
	exitArmed     bool
	quota         tuiQuotaStatus
}

type tuiQuotaStatus struct {
	RuntimeMode        string
	AccessMode         string
	CreditBalance      int
	FreeRemaining      int
	RewardRemaining    int
	PaidQuotaRemaining int
	Remaining          int
	Checked            bool
	Err                string
}

var (
	tuiTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	tuiUserStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	tuiStatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tuiErrorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	tuiCardStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

func (a *App) runTUIProgram(ctx context.Context, cfg Config, initialPrompt string, opts TUIOptions) error {
	rootCtx := ctx
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	model := newTUIModel(a, cfg, opts, initialPrompt, a.Stderr)
	model.rootCtx = rootCtx
	programOpts := []tea.ProgramOption{
		tea.WithContext(rootCtx),
		tea.WithInput(a.Stdin),
		tea.WithOutput(a.Stdout),
	}
	if !opts.NoAltScreen {
		programOpts = append(programOpts, tea.WithAltScreen())
	}
	_, err := tea.NewProgram(model, programOpts...).Run()
	return err
}

func newTUIModel(app *App, cfg Config, opts TUIOptions, initialPrompt string, _ io.Writer) tuiModel {
	cwd, _ := os.Getwd()
	input := textinput.New()
	input.Placeholder = "Describe a document, or type /help"
	input.Prompt = "> "
	input.Focus()
	input.CharLimit = 4096
	input.Width = 76
	vp := viewport.New(80, 18)
	model := tuiModel{
		app:           app,
		cfg:           cfg,
		opts:          opts,
		rootCtx:       context.Background(),
		cwd:           cwd,
		input:         input,
		view:          vp,
		state:         tuiStateIdle,
		width:         80,
		height:        24,
		initialPrompt: strings.TrimSpace(initialPrompt),
		quota: tuiQuotaStatus{
			RuntimeMode: runtimeModeLabel(cfg.RuntimeModeOrDefault()),
		},
		entries: []tuiEntry{{
			role: "assistant",
			text: tuiStartupBanner,
		}, {
			role: "status",
			text: tuiStartupTip,
		}},
	}
	model.refreshViewport()
	return model
}

func (m tuiModel) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink, m.checkAccessStatus()}
	if strings.TrimSpace(m.initialPrompt) != "" {
		cmds = append(cmds, func() tea.Msg { return tuiInitialPromptMsg{} })
	}
	return tea.Batch(cmds...)
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case tuiInitialPromptMsg:
		prompt := strings.TrimSpace(m.initialPrompt)
		m.initialPrompt = ""
		return m.submit(prompt)
	case tuiAccessStatusMsg:
		m.applyAccessStatus(msg.Result, msg.Err)
		return m, nil
	case tuiProgressMsg:
		if msg.GenerationID != m.generationID {
			return m, m.waitForEvent()
		}
		if m.cancelled {
			return m, m.waitForEvent()
		}
		m.append("status", formatTUIProgress(msg.Event))
		return m, m.waitForEvent()
	case tuiPauseMsg:
		if msg.GenerationID != m.generationID {
			return m, m.waitForEvent()
		}
		if m.cancelled {
			return m, m.waitForEvent()
		}
		m.append("status", msg.Message)
		return m, m.waitForEvent()
	case tuiQuestionMsg:
		if msg.GenerationID != m.generationID {
			return m, m.waitForEvent()
		}
		if m.cancelled {
			if msg.Reply != nil {
				msg.Reply <- tuiQuestionAnswer{err: context.Canceled}
			}
			return m, m.waitForEvent()
		}
		m.state = tuiStateQuestion
		m.question = &msg
		m.input.Placeholder = "Answer the question above"
		m.append("assistant", formatTUIQuestion(msg.Question, msg.Options, msg.AllowFreeform))
		return m, m.waitForEvent()
	case tuiGenerationFinishedMsg:
		if msg.GenerationID != m.generationID {
			return m, nil
		}
		m.cancel = nil
		m.currentJob = nil
		m.question = nil
		m.exitArmed = false
		m.input.Placeholder = "Describe a document, or type /help"
		if m.cancelled {
			m.cancelled = false
			m.state = tuiStateIdle
			m.append("status", "Current generation cancelled")
			return m, nil
		}
		m.state = tuiStateIdle
		if msg.Err != nil {
			m.append("error", msg.Err.Error())
			return m, nil
		}
		m.applyGenerationQuota(msg.Result)
		m.append("assistant", formatTUIResult(msg.Result))
		return m, nil
	case tuiNoopMsg:
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.state == tuiStateRunning || m.state == tuiStateQuestion {
				if strings.TrimSpace(m.input.Value()) != "" {
					m.input.Reset()
					m.exitArmed = true
					m.append("status", "Input cleared. Press Ctrl+C again to cancel the current generation.")
					return m, nil
				}
				if m.cancel != nil {
					m.cancel()
				}
				if m.question != nil && m.question.Reply != nil {
					m.question.Reply <- tuiQuestionAnswer{err: context.Canceled}
				}
				m.cancelled = true
				m.exitArmed = true
				m.state = tuiStateIdle
				m.question = nil
				m.currentJob = nil
				m.input.Placeholder = "Describe a document, or type /help"
				return m, nil
			}
			if strings.TrimSpace(m.input.Value()) != "" {
				m.input.Reset()
				m.exitArmed = true
				return m, nil
			}
			if m.exitArmed {
				return m, tea.Quit
			}
			m.exitArmed = true
			m.append("status", "Press Ctrl+C again to exit.")
			return m, nil
		case tea.KeyEnter:
			m.exitArmed = false
			value := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			if value == "" {
				return m, nil
			}
			if m.state == tuiStateQuestion && m.question != nil {
				m.answerQuestion(value)
				m.state = tuiStateRunning
				m.question = nil
				m.input.Placeholder = "Generating. Ctrl+C to cancel"
				return m, nil
			}
			if m.state == tuiStateRunning {
				m.append("status", "A generation is already running. Press Ctrl+C to cancel it.")
				return m, nil
			}
			return m.handleCommandOrSubmit(value)
		}
		m.exitArmed = false
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m tuiModel) handleCommandOrSubmit(value string) (tea.Model, tea.Cmd) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "/exit", "/quit":
		return m, tea.Quit
	case "/clear":
		m.entries = nil
		m.refreshViewport()
		return m, nil
	case "/help":
		m.append("assistant", "Describe the document you want to generate in natural language. Commands: /help, /clear, /exit. Ctrl+C clears the current input; press Ctrl+C again to exit. During generation, Ctrl+C cancels the current run. For scripts, use officecli new ...")
		return m, nil
	default:
		return m.submit(value)
	}
}

func (m tuiModel) submit(prompt string) (tea.Model, tea.Cmd) {
	job, err := BuildTUIGenerateJob(prompt, m.cfg, InputSources{IsTTY: true, CWD: m.cwd})
	if err != nil {
		m.append("error", err.Error())
		return m, nil
	}
	m.generationID++
	generationID := m.generationID
	rootCtx := m.rootCtx
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(rootCtx)
	events := make(chan tea.Msg, 128)
	m.events = events
	m.cancel = cancel
	m.cancelled = false
	m.exitArmed = false
	m.currentJob = &job
	m.state = tuiStateRunning
	m.input.Placeholder = "Generating. Ctrl+C to cancel"
	m.append("user", prompt)
	m.append("status", fmt.Sprintf("Detected %s, topic: %s, mode: %s", job.DocumentType, job.Topic, job.Mode))
	return m, m.startGenerationCmd(ctx, job, generationID, events)
}

func (m tuiModel) startGenerationCmd(ctx context.Context, job GenerateJob, generationID int, events chan tea.Msg) tea.Cmd {
	app := m.app
	cfg := m.cfg
	return func() tea.Msg {
		go func() {
			result, err := app.executeGenerateJob(ctx, cfg, job, true, tuiProgressEmitter{events: events, generationID: generationID}, newTUIProgressPrompter(events, generationID))
			events <- tuiGenerationFinishedMsg{GenerationID: generationID, Result: result, Err: err}
			close(events)
		}()
		msg, ok := <-events
		if !ok {
			return tuiNoopMsg{}
		}
		return msg
	}
}

func (m tuiModel) waitForEvent() tea.Cmd {
	events := m.events
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return tuiNoopMsg{}
		}
		return msg
	}
}

func (m *tuiModel) answerQuestion(value string) {
	if m.question == nil || m.question.Reply == nil {
		return
	}
	m.append("user", value)
	if index, err := strconv.Atoi(value); err == nil && index >= 1 && index <= len(m.question.Options) {
		m.question.Reply <- tuiQuestionAnswer{optionID: value, answer: m.question.Options[index-1]}
		return
	}
	if !m.question.AllowFreeform {
		m.question.Reply <- tuiQuestionAnswer{err: fmt.Errorf("invalid option: %s", value)}
		return
	}
	m.question.Reply <- tuiQuestionAnswer{answer: value}
}

func (m *tuiModel) append(role, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	m.entries = append(m.entries, tuiEntry{role: role, text: text})
	m.refreshViewport()
}

func (m *tuiModel) resize() {
	inputWidth := m.width - 4
	if inputWidth < 20 {
		inputWidth = 20
	}
	m.input.Width = inputWidth
	viewportHeight := m.height - 8
	if viewportHeight < 6 {
		viewportHeight = 6
	}
	m.view.Width = m.width
	m.view.Height = viewportHeight
	m.refreshViewport()
}

func (m *tuiModel) refreshViewport() {
	m.view.SetContent(bottomAlignContent(m.renderEntries(), m.view.Height))
	m.view.GotoBottom()
}

func bottomAlignContent(content string, height int) string {
	content = strings.TrimRight(content, "\n")
	if content == "" || height <= 0 {
		return content
	}
	contentHeight := lipgloss.Height(content)
	if contentHeight >= height {
		return content
	}
	return strings.Repeat("\n", height-contentHeight) + content
}

func (m tuiModel) renderEntries() string {
	var b strings.Builder
	for _, entry := range m.entries {
		switch entry.role {
		case "user":
			b.WriteString(tuiUserStyle.Render("› You"))
			b.WriteString("\n")
			b.WriteString(entry.text)
		case "status":
			b.WriteString(tuiStatusStyle.Render("• " + entry.text))
		case "error":
			b.WriteString(tuiErrorStyle.Render("Error: " + entry.text))
		default:
			content := "OfficeCLI\n" + entry.text
			if strings.HasPrefix(entry.text, tuiStartupBanner) {
				content = entry.text
			}
			if strings.Contains(entry.text, "Generation complete") {
				content = tuiCardStyle.Render(entry.text)
			}
			b.WriteString(content)
		}
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m tuiModel) View() string {
	var b strings.Builder
	b.WriteString(tuiTitleStyle.Render("OfficeCLI"))
	b.WriteString("  ")
	b.WriteString(m.statusText())
	b.WriteString("\n\n")
	b.WriteString(m.view.View())
	b.WriteString("\n\n")
	b.WriteString(m.input.View())
	b.WriteString("\n")
	b.WriteString(tuiStatusStyle.Render(m.quotaText()))
	b.WriteString("\n")
	b.WriteString(tuiStatusStyle.Render("Enter send  Ctrl+C clear/cancel  Ctrl+C twice exit  /help  /clear  /exit"))
	return b.String()
}

func (m tuiModel) quotaText() string {
	mode := strings.ToLower(strings.TrimSpace(m.quota.RuntimeMode))
	if mode == "" {
		mode = runtimeModeLabel(m.cfg.RuntimeModeOrDefault())
	}
	prefix := "Mode: " + mode
	if !m.quota.Checked {
		if mode == string(RuntimeModeHosted) {
			return prefix + " · Credits: checking..."
		}
		return prefix + " · Generations: checking..."
	}
	if strings.TrimSpace(m.quota.Err) != "" {
		if mode == string(RuntimeModeHosted) {
			return prefix + " · Credits: unavailable"
		}
		return prefix + " · Generations: unavailable"
	}
	switch strings.ToLower(strings.TrimSpace(m.quota.AccessMode)) {
	case string(LicenseAccessModeHosted):
		return fmt.Sprintf("%s · Credits: %d", prefix, m.quota.CreditBalance)
	case string(LicenseAccessModeFree):
		return fmt.Sprintf("%s · Trial: %d generations", prefix, m.quota.FreeRemaining)
	case string(LicenseAccessModeReward):
		return fmt.Sprintf("%s · Generations: %d", prefix, firstPositive(m.quota.RewardRemaining, m.quota.Remaining))
	case string(LicenseAccessModePaid):
		return fmt.Sprintf("%s · Generations: %d", prefix, firstPositive(m.quota.PaidQuotaRemaining, m.quota.Remaining))
	case string(LicenseAccessModeDisabled):
		return prefix + " · Quota: disabled"
	default:
		if mode == string(RuntimeModeHosted) {
			return fmt.Sprintf("%s · Credits: %d", prefix, m.quota.CreditBalance)
		}
		return fmt.Sprintf("%s · Generations: %d", prefix, firstPositive(m.quota.PaidQuotaRemaining, m.quota.RewardRemaining, m.quota.FreeRemaining, m.quota.Remaining))
	}
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (m *tuiModel) applyAccessStatus(result *LicenseCheckResult, err error) {
	m.quota.Checked = true
	m.quota.Err = ""
	if err != nil {
		m.quota.Err = err.Error()
		return
	}
	if result == nil {
		return
	}
	runtimeMode := strings.TrimSpace(result.SelectedRuntimeMode)
	if runtimeMode == "" {
		runtimeMode = runtimeModeLabel(m.cfg.RuntimeModeOrDefault())
	}
	m.quota.RuntimeMode = runtimeMode
	m.quota.AccessMode = string(result.AccessMode)
	m.quota.CreditBalance = result.CreditBalance
	m.quota.FreeRemaining = result.FreeRemaining
	m.quota.RewardRemaining = result.RewardRemaining
	m.quota.PaidQuotaRemaining = result.PaidQuotaRemaining
}

func (m *tuiModel) applyGenerationQuota(result GenerateResult) {
	m.quota.Checked = true
	m.quota.Err = ""
	if strings.TrimSpace(result.RuntimeMode) != "" {
		m.quota.RuntimeMode = strings.TrimSpace(result.RuntimeMode)
	}
	if strings.TrimSpace(result.AccessMode) != "" {
		m.quota.AccessMode = strings.TrimSpace(result.AccessMode)
	}
	m.quota.CreditBalance = result.CreditBalance
	m.quota.FreeRemaining = result.FreeRemaining
	m.quota.RewardRemaining = result.RewardRemaining
	m.quota.PaidQuotaRemaining = result.PaidQuotaRemaining
	m.quota.Remaining = result.Remaining
}

func (m tuiModel) checkAccessStatus() tea.Cmd {
	if m.app == nil {
		return nil
	}
	return func() tea.Msg {
		app := *m.app
		app.Stderr = io.Discard
		result, err := app.checkLicenseWithRuntime(m.rootCtx, m.cfg.License, m.cfg.RuntimeModeOrDefault(), "", "status")
		return tuiAccessStatusMsg{Result: result, Err: err}
	}
}

func (m tuiModel) statusText() string {
	runtimeMode := runtimeModeLabel(m.cfg.RuntimeModeOrDefault())
	switch m.state {
	case tuiStateRunning:
		if m.currentJob != nil {
			return fmt.Sprintf("Generating · %s · %s", m.currentJob.DocumentType, runtimeMode)
		}
		return "Generating · " + runtimeMode
	case tuiStateQuestion:
		return "Waiting for answer · " + runtimeMode
	default:
		return "Ready · " + runtimeMode + " · keep typing"
	}
}

func BuildTUIGenerateJob(prompt string, cfg Config, src InputSources) (GenerateJob, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return GenerateJob{}, fmt.Errorf("document request is required")
	}
	documentType := inferTUIDocumentType(prompt)
	topic := tuiTopicFromPrompt(prompt)
	args := []string{documentType, topic, "--prompt", prompt}
	return BuildGenerateJob(args, cfg, src)
}

func inferTUIDocumentType(prompt string) string {
	normalized := strings.ToLower(prompt)
	switch {
	case strings.Contains(normalized, "xlsx"), strings.Contains(normalized, "excel"), strings.Contains(normalized, "workbook"), strings.Contains(prompt, "表格"), strings.Contains(prompt, "工作簿"):
		return "xlsx"
	case strings.Contains(normalized, "docx"), strings.Contains(normalized, "doc "), strings.Contains(normalized, "word"), strings.Contains(normalized, "brief"), strings.Contains(prompt, "文档"), strings.Contains(prompt, "方案"):
		return "docx"
	case strings.Contains(normalized, "img"), strings.Contains(normalized, "image"), strings.Contains(normalized, "poster"), strings.Contains(prompt, "图片"), strings.Contains(prompt, "海报"), strings.Contains(prompt, "视觉图"), hasChineseImagePromptIntent(prompt):
		return "img"
	default:
		return "pptx"
	}
}

func hasChineseImagePromptIntent(prompt string) bool {
	if strings.Contains(prompt, "图表") {
		return false
	}
	imageIntentPhrases := []string{
		"画一个图",
		"画一张图",
		"画幅图",
		"画图",
		"生成一个图",
		"生成一张图",
		"出一张图",
		"做一张图",
		"插画",
		"配图",
	}
	for _, phrase := range imageIntentPhrases {
		if strings.Contains(prompt, phrase) {
			return true
		}
	}
	return false
}

func tuiTopicFromPrompt(prompt string) string {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return "Untitled"
	}
	trimmed = strings.ReplaceAll(trimmed, "\n", " ")
	fields := strings.Fields(trimmed)
	if len(fields) > 12 {
		trimmed = strings.Join(fields[:12], " ")
	}
	if len([]rune(trimmed)) > 80 {
		runes := []rune(trimmed)
		trimmed = string(runes[:80])
	}
	return filepath.Base(trimmed)
}

func formatTUIProgress(event engine.ProgressEvent) string {
	message := strings.TrimSpace(event.ActiveContent)
	if message == "" {
		message = strings.TrimSpace(event.Content)
	}
	if message == "" {
		message = defaultProgressMessage(event)
	}
	if strings.TrimSpace(event.Status) == "" {
		return message
	}
	return fmt.Sprintf("%s: %s", event.Status, message)
}

func formatTUIQuestion(question string, options []string, allowFreeform bool) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(question))
	for idx, option := range options {
		sb.WriteString(fmt.Sprintf("\n%d. %s", idx+1, option))
	}
	if allowFreeform {
		sb.WriteString("\nEnter an option number, or type a custom answer.")
	} else {
		sb.WriteString("\nEnter an option number.")
	}
	return sb.String()
}

func formatTUIResult(result GenerateResult) string {
	var lines []string
	lines = append(lines, "Generation complete")
	if strings.TrimSpace(result.DocumentType) != "" {
		lines = append(lines, "Type: "+strings.TrimSpace(result.DocumentType))
	}
	if strings.TrimSpace(result.FilePath) != "" {
		lines = append(lines, "Local file: "+strings.TrimSpace(result.FilePath))
	}
	if strings.TrimSpace(result.AccessURL) != "" {
		lines = append(lines, "Preview URL: "+strings.TrimSpace(result.AccessURL))
	}
	if strings.TrimSpace(result.Password) != "" {
		lines = append(lines, "Password: "+strings.TrimSpace(result.Password))
	}
	if strings.TrimSpace(result.ExpiresAt) != "" {
		lines = append(lines, "Expires at: "+strings.TrimSpace(result.ExpiresAt))
	}
	for _, warning := range result.Warnings {
		if strings.TrimSpace(warning) != "" {
			lines = append(lines, "Warning: "+strings.TrimSpace(warning))
		}
	}
	lines = append(lines, "Enter another document request to continue.")
	return strings.Join(lines, "\n")
}
