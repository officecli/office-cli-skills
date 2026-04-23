package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
)

const planSessionTTL = 24 * time.Hour

type Options struct {
	PlanStore               engine.PlanStore
	LLMClient               engine.LLMClient
	Clock                   engine.Clock
	IDGenerator             engine.IDGenerator
	QuestionAttemptTimeout  time.Duration
	BlueprintTimeout        time.Duration
	ExecutionAttemptTimeout time.Duration
}

type Workflow struct {
	store                   engine.PlanStore
	llm                     engine.LLMClient
	clock                   engine.Clock
	ids                     engine.IDGenerator
	questionAttemptTimeout  time.Duration
	blueprintTimeout        time.Duration
	executionAttemptTimeout time.Duration
}

func NewWorkflow(opts Options) *Workflow {
	return &Workflow{
		store:                   opts.PlanStore,
		llm:                     opts.LLMClient,
		clock:                   opts.Clock,
		ids:                     opts.IDGenerator,
		questionAttemptTimeout:  opts.QuestionAttemptTimeout,
		blueprintTimeout:        opts.BlueprintTimeout,
		executionAttemptTimeout: opts.ExecutionAttemptTimeout,
	}
}

func (w *Workflow) PrepareExecutionPlan(ctx context.Context, req engine.PrepareExecutionPlanRequest) (*engine.PlanSession, error) {
	if w == nil || w.store == nil || w.ids == nil {
		return nil, fmt.Errorf("planning workflow is unavailable")
	}
	documentType := normalizeDocumentType(req.DocumentType)
	mode := generateengine.NormalizeGenerationMode(req.GenerationMode)
	if strings.EqualFold(strings.TrimSpace(req.GenerationMode), generateengine.ModeFast) {
		session := &engine.PlanSession{
			PlanID:         w.ids.NewID(),
			ConversationID: strings.TrimSpace(req.ConversationID),
			DocumentID:     strings.TrimSpace(req.DocumentID),
			DocumentType:   documentType,
			EditTarget:     strings.TrimSpace(req.EditTarget),
			GenerationMode: generateengine.ModeFast,
			IntentType:     "generate_new_document",
			Status:         "approved",
			UserPrompt:     strings.TrimSpace(req.UserPrompt),
			QuestionSource: "fast_path",
			Revision:       1,
		}
		applyLocalArtifacts(session)
		if err := w.store.Save(ctx, session, planSessionTTL); err != nil {
			return nil, err
		}
		return session, nil
	}

	questions, questionMeta, err := w.synthesizeQuestions(ctx, req, documentType)
	questionSource := "llm_dynamic"
	questionErrorKind := ""
	questionFallbackReason := ""
	if err != nil {
		questionErrorKind = classifyQuestionError(err)
		if questionMeta.ValidationRule != "" {
			questionErrorKind = "schema_validate_failed"
		}
		questions = buildDynamicFallbackQuestions(req, documentType)
		if len(questions) == 0 {
			questions, err = w.fallbackQuestions(ctx, req, documentType)
			if err != nil {
				questions = buildExecutionPlanQuestions(documentType)
				questionSource = "static_fallback"
				questionFallbackReason = "Dynamic clarification generation failed, so the workflow fell back to static clarification questions."
			}
		}
	}
	session := &engine.PlanSession{
		PlanID:                   w.ids.NewID(),
		ConversationID:           strings.TrimSpace(req.ConversationID),
		DocumentID:               strings.TrimSpace(req.DocumentID),
		DocumentType:             documentType,
		EditTarget:               strings.TrimSpace(req.EditTarget),
		GenerationMode:           mode,
		IntentType:               "generate_new_document",
		Status:                   "questioning",
		UserPrompt:               strings.TrimSpace(req.UserPrompt),
		Questions:                questions,
		QuestionSource:           questionSource,
		QuestionErrorKind:        questionErrorKind,
		QuestionFallbackReason:   questionFallbackReason,
		QuestionValidationRule:   questionMeta.ValidationRule,
		QuestionValidationDetail: questionMeta.ValidationDetail,
		QuestionRawPreview:       questionMeta.RawPreview,
		QuestionLLMProvider:      questionMeta.Provider,
		QuestionLLMModel:         questionMeta.Model,
		QuestionLLMBaseURL:       questionMeta.BaseURL,
		Revision:                 1,
	}
	if len(questions) > 0 {
		current := questions[0]
		session.CurrentQuestion = &current
	}
	session.PlanMarkdown = buildExecutionPlanMarkdown(session)
	if err := w.store.Save(ctx, session, planSessionTTL); err != nil {
		return nil, err
	}
	return session, nil
}

func (w *Workflow) withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func (w *Workflow) AnswerExecutionPlanQuestion(ctx context.Context, req engine.AnswerExecutionPlanQuestionRequest) (*engine.PlanSession, error) {
	session, err := w.load(ctx, req.PlanID)
	if err != nil {
		return nil, err
	}
	questionID := strings.TrimSpace(req.QuestionID)
	if questionID == "" && session.CurrentQuestion != nil {
		questionID = session.CurrentQuestion.ID
	}
	answerText := strings.TrimSpace(req.Answer)
	optionID := strings.TrimSpace(req.OptionID)
	source := "freeform"
	question := findQuestion(session.Questions, questionID)
	if option := findOption(question, optionID); option != nil {
		if answerText == "" {
			answerText = strings.TrimSpace(option.Label)
		}
		source = "option"
		optionID = option.ID
	}
	if answerText == "" {
		return nil, fmt.Errorf("plan answer is required")
	}
	session.Answers = append(session.Answers, engine.PlanAnswer{
		QuestionID: questionID,
		OptionID:   optionID,
		Answer:     answerText,
		Source:     source,
	})
	if len(session.Answers) < len(session.Questions) {
		next := session.Questions[len(session.Answers)]
		session.Status = "questioning"
		session.CurrentQuestion = &next
		session.ExecutionPrompt = ""
		session.FrameworkBlueprint = ""
		session.PlanMarkdown = buildExecutionPlanMarkdown(session)
		return session, w.store.Save(ctx, session, planSessionTTL)
	}
	session.Status = "review_pending"
	session.CurrentQuestion = nil
	w.refreshArtifacts(ctx, session)
	if err := w.store.Save(ctx, session, planSessionTTL); err != nil {
		return nil, err
	}
	return session, nil
}

func (w *Workflow) ReviseExecutionPlan(ctx context.Context, req engine.ReviseExecutionPlanRequest) (*engine.PlanSession, error) {
	session, err := w.load(ctx, req.PlanID)
	if err != nil {
		return nil, err
	}
	session.Answers = append(session.Answers, engine.PlanAnswer{
		QuestionID: "revision",
		Answer:     "Plan revision: " + strings.TrimSpace(req.Instruction),
		Source:     "freeform",
	})
	session.Revision++
	session.Status = "review_pending"
	w.refreshArtifacts(ctx, session)
	if err := w.store.Save(ctx, session, planSessionTTL); err != nil {
		return nil, err
	}
	return session, nil
}

func (w *Workflow) ApproveExecutionPlan(ctx context.Context, req engine.ApproveExecutionPlanRequest) (*engine.PlanSession, error) {
	session, err := w.load(ctx, req.PlanID)
	if err != nil {
		return nil, err
	}
	session.Status = "approved"
	if strings.TrimSpace(session.ExecutionPrompt) == "" || strings.TrimSpace(session.PlanMarkdown) == "" {
		w.refreshArtifacts(ctx, session)
	}
	if err := w.store.Save(ctx, session, planSessionTTL); err != nil {
		return nil, err
	}
	return session, nil
}

func (w *Workflow) load(ctx context.Context, planID string) (*engine.PlanSession, error) {
	if w == nil || w.store == nil {
		return nil, fmt.Errorf("planning workflow is unavailable")
	}
	return w.store.Load(ctx, planID)
}

func (w *Workflow) refreshArtifacts(ctx context.Context, session *engine.PlanSession) {
	applyLocalArtifacts(session)
	if shouldBuildFrameworkBlueprint(session) {
		if blueprint, err := w.synthesizeFrameworkBlueprint(ctx, session); err == nil {
			session.FrameworkBlueprint = blueprint
			session.PlanMarkdown = injectFrameworkBlueprint(session.PlanMarkdown, blueprint)
		}
	}
	if shouldSynthesizeExecutionPlan(session) {
		if planMarkdown, executionPrompt, err := w.synthesizeExecutionPlan(ctx, session); err == nil {
			session.PlanMarkdown = injectFrameworkBlueprint(planMarkdown, session.FrameworkBlueprint)
			session.ExecutionPrompt = executionPrompt
		}
	}
}

func normalizeDocumentType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "docx":
		return "docx"
	case "xlsx":
		return "xlsx"
	case "report":
		return "report"
	default:
		return "pptx"
	}
}

func findQuestion(questions []engine.PlanQuestion, questionID string) *engine.PlanQuestion {
	for i := range questions {
		if questions[i].ID == questionID {
			return &questions[i]
		}
	}
	return nil
}

func findOption(question *engine.PlanQuestion, optionID string) *engine.PlanQuestionOption {
	if question == nil {
		return nil
	}
	for i := range question.Options {
		if question.Options[i].ID == optionID {
			return &question.Options[i]
		}
	}
	return nil
}

func buildExecutionPlanMarkdown(session *engine.PlanSession) string {
	if session == nil {
		return ""
	}
	if session.Status == "questioning" && session.CurrentQuestion != nil {
		return session.CurrentQuestion.Question
	}
	var sb strings.Builder
	sb.WriteString("# Execution Plan\n\n")
	sb.WriteString("## Objective\n")
	sb.WriteString("- Task: create a new ")
	sb.WriteString(buildDocumentLabel(session.DocumentType))
	sb.WriteString(".\n")
	if prompt := strings.TrimSpace(session.UserPrompt); prompt != "" {
		sb.WriteString("- Original request: ")
		sb.WriteString(prompt)
		sb.WriteString("\n")
	}
	if session.EditTarget != "" {
		sb.WriteString("- Preferred edit target: ")
		sb.WriteString(strings.TrimSpace(session.EditTarget))
		sb.WriteString("\n")
	}
	if session.FrameworkBlueprint != "" {
		sb.WriteString("\n")
		sb.WriteString(strings.TrimSpace(session.FrameworkBlueprint))
		sb.WriteString("\n")
	}
	sb.WriteString("\n## Steps\n")
	if generateengine.NormalizeGenerationMode(session.GenerationMode) == generateengine.ModeBest {
		sb.WriteString("1. **Theme Identification**: clarify the core storyline, audience, and output goal.\n")
		sb.WriteString("2. **Framework Blueprint**: define the structure first, then decide emphasis, order, and expression.\n")
		sb.WriteString("3. **Content Expansion**: build the content according to the confirmed level of detail, structure, and quality constraints.\n")
		sb.WriteString("4. **Generate After Approval**: start actual generation only after you approve the plan.\n")
	} else {
		sb.WriteString("1. Clarify the document's main thread, audience, and output form.\n")
		sb.WriteString("2. Organize the structure and expand the content.\n")
		sb.WriteString("3. Start generation only after you approve the plan.\n")
	}
	sb.WriteString("\n## Constraints\n")
	for _, line := range buildConstraintLines(session) {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if strings.TrimSpace(session.ExecutionPrompt) != "" {
		sb.WriteString("\n## Execution Baseline\n")
		sb.WriteString("- A structured execution prompt is ready. Later generation should follow this plan strictly.\n")
	}
	return strings.TrimSpace(sb.String())
}

func buildConstraintLines(session *engine.PlanSession) []string {
	if session == nil {
		return nil
	}
	lines := make([]string, 0, len(session.Answers)+2)
	for _, answer := range session.Answers {
		question := findQuestion(session.Questions, answer.QuestionID)
		if question != nil {
			lines = append(lines, fmt.Sprintf("- %s: %s", strings.TrimSpace(question.Question), strings.TrimSpace(answer.Answer)))
			continue
		}
		lines = append(lines, fmt.Sprintf("- Additional requirement: %s", strings.TrimSpace(answer.Answer)))
	}
	lines = append(lines, "- Keep the original user request as the main thread and avoid expanding unrelated content.")
	lines = append(lines, "- Until you approve it, this plan is for review only and will not be executed directly.")
	return lines
}

func buildDocumentLabel(documentType string) string {
	switch normalizeDocumentType(documentType) {
	case "docx":
		return "Word document"
	case "xlsx":
		return "Excel workbook"
	case "report":
		return "workbook-backed report"
	default:
		return "PPT presentation"
	}
}

func applyLocalArtifacts(session *engine.PlanSession) {
	if session == nil {
		return
	}
	session.PlanMarkdown = buildExecutionPlanMarkdown(session)
	session.ExecutionPrompt = buildExecutionPrompt(session)
}

func buildExecutionPrompt(session *engine.PlanSession) string {
	if session == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Generate the new document strictly according to the approved plan below. Do not drift beyond scope.\n")
	sb.WriteString("Original request: ")
	sb.WriteString(strings.TrimSpace(session.UserPrompt))
	sb.WriteString("\n")
	sb.WriteString("Document type: ")
	sb.WriteString(normalizeDocumentType(session.DocumentType))
	sb.WriteString("\n")
	for i, answer := range session.Answers {
		sb.WriteString("Additional note ")
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString(": ")
		sb.WriteString(strings.TrimSpace(answer.Answer))
		sb.WriteString("\n")
	}
	if blueprint := strings.TrimSpace(session.FrameworkBlueprint); blueprint != "" {
		sb.WriteString("Framework blueprint summary:\n")
		sb.WriteString(blueprint)
		sb.WriteString("\n")
	}
	sb.WriteString("Quality constraints:\n")
	for _, line := range buildExecutionQualityConstraints(session.DocumentType) {
		sb.WriteString("- ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("Ensure the final content stays fully aligned with the framework blueprint, additional notes, and quality constraints. Do not expand unrelated content.")
	return strings.TrimSpace(sb.String())
}

func shouldSynthesizeExecutionPlan(session *engine.PlanSession) bool {
	return session != nil && generateengine.NormalizeGenerationMode(session.GenerationMode) == generateengine.ModeBest
}

func shouldBuildFrameworkBlueprint(session *engine.PlanSession) bool {
	return shouldSynthesizeExecutionPlan(session)
}

func injectFrameworkBlueprint(planMarkdown, blueprint string) string {
	planMarkdown = strings.TrimSpace(planMarkdown)
	blueprint = strings.TrimSpace(blueprint)
	if blueprint == "" || strings.Contains(planMarkdown, "## Framework Blueprint") {
		return planMarkdown
	}
	if planMarkdown == "" {
		return blueprint
	}
	return strings.TrimSpace(planMarkdown + "\n\n" + blueprint)
}

type synthesizedPlan struct {
	PlanMarkdown    string `json:"plan_markdown"`
	ExecutionPrompt string `json:"execution_prompt"`
}

func (w *Workflow) synthesizeExecutionPlan(ctx context.Context, session *engine.PlanSession) (string, string, error) {
	if w == nil || w.llm == nil {
		return "", "", errors.New("llm unavailable")
	}
	req := engine.StructuredCompletionRequest{
		Messages: buildExecutionPlanSynthesisMessages(session),
		Schema: engine.StructuredSchema{
			Name:        "execution_plan_synthesis",
			Description: "Synthesize a user-facing markdown plan plus a strict execution prompt.",
			JSONSchema: []byte(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "plan_markdown":{"type":"string"},
    "execution_prompt":{"type":"string"}
  },
  "required":["plan_markdown","execution_prompt"]
}`),
			Strict: true,
		},
	}
	var lastErr error
	for range 2 {
		attemptCtx, cancel := w.withTimeout(ctx, w.executionAttemptTimeout)
		response, err := w.llm.CompleteStructured(attemptCtx, req)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		var result synthesizedPlan
		if err := json.Unmarshal([]byte(response), &result); err != nil {
			lastErr = err
			continue
		}
		result.PlanMarkdown = strings.TrimSpace(result.PlanMarkdown)
		result.ExecutionPrompt = strings.TrimSpace(result.ExecutionPrompt)
		if result.PlanMarkdown == "" || result.ExecutionPrompt == "" {
			lastErr = errors.New("execution plan synthesis returned empty fields")
			continue
		}
		return result.PlanMarkdown, result.ExecutionPrompt, nil
	}
	if lastErr == nil {
		lastErr = errors.New("execution plan synthesis failed")
	}
	return "", "", lastErr
}

func buildExecutionPlanSynthesisMessages(session *engine.PlanSession) []engine.LLMMessage {
	var answers strings.Builder
	for _, answer := range session.Answers {
		question := findQuestion(session.Questions, answer.QuestionID)
		if question != nil {
			answers.WriteString("- ")
			answers.WriteString(strings.TrimSpace(question.Question))
			answers.WriteString(": ")
			answers.WriteString(strings.TrimSpace(answer.Answer))
			answers.WriteString("\n")
			continue
		}
		answers.WriteString("- Additional requirement: ")
		answers.WriteString(strings.TrimSpace(answer.Answer))
		answers.WriteString("\n")
	}
	if answers.Len() == 0 {
		answers.WriteString("- No additional notes\n")
	}
	var blueprintSection string
	if strings.TrimSpace(session.FrameworkBlueprint) != "" {
		blueprintSection = "\nExisting framework blueprint:\n" + strings.TrimSpace(session.FrameworkBlueprint) + "\n"
	}
	constraints := strings.Join(buildExecutionQualityConstraints(session.DocumentType), "\n- ")
	if constraints != "" {
		constraints = "- " + constraints
	}
	extraRequirements := strings.TrimSpace(buildExecutionPlanSynthesisRequirements(session))
	userPrompt := fmt.Sprintf(`Based on the information below, prepare a user-facing execution plan for review and also produce the execution baseline for later generation.

Task type: create a new document
Document type: %s
Original request: %s
Revision: %d

Clarification results:
%s
%s

Output requirements:
1. plan_markdown must be markdown.
2. If an existing framework blueprint is present, plan_markdown must keep a section named "## Framework Blueprint" and absorb its structural guidance.
3. For best-mode new-document plans, the implementation section must explicitly include and highlight the first two step titles: **Theme Identification** and **Framework Blueprint**.
4. execution_prompt must be a strict execution baseline for the real generation step.
5. execution_prompt must explicitly include the following quality constraints:
%s`,
		buildDocumentLabel(session.DocumentType),
		strings.TrimSpace(session.UserPrompt),
		session.Revision,
		strings.TrimSpace(answers.String()),
		blueprintSection,
		constraints,
	)
	if extraRequirements != "" {
		userPrompt += "\n6. Also satisfy these scenario-specific execution-prompt requirements:\n" + extraRequirements
	}
	return []engine.LLMMessage{
		{Role: "system", Content: "You organize execution plans for office documents. You must return JSON that matches the schema exactly."},
		{Role: "user", Content: userPrompt},
	}
}

func buildExecutionQualityConstraints(documentType string) []string {
	switch normalizeDocumentType(documentType) {
	case "docx":
		return []string{
			"Keep the section order clear. Lead with conclusions before analysis, and avoid structural jumps.",
			"Use a formal, professional tone. Avoid filler, empty phrases, and low-information writing.",
			"Control argument depth and length according to the document purpose and reader level.",
		}
	case "xlsx":
		return []string{
			"Give each sheet a clear role, with a clean split between summary and detail.",
			"Keep fields and metric definitions consistent, and avoid mixing near-duplicate labels.",
			"Prioritize the analysis goal; provide an executive summary first when useful before expanding into detail.",
		}
	case "report":
		return []string{
			"Keep the report narrative chart-driven and explicitly grounded in the workbook data before adding interpretation.",
			"Use charts only for evidence-based comparisons, trends, or composition with consistent units and labels from the workbook.",
			"Maintain a reader-friendly single-page HTML report structure suitable for external business sharing.",
		}
	default:
		return []string{
			"Control page count and information density, favoring a concise 6-8 slide conclusion-first deck.",
			"Give each slide one clear job and avoid repetitive or low-value slides.",
			"Keep the slide-to-slide narrative progressive, and make titles and points serve the core conclusion.",
		}
	}
}

func buildExecutionPlanSynthesisRequirements(session *engine.PlanSession) string {
	if session == nil || normalizeDocumentType(session.DocumentType) != "pptx" {
		return ""
	}
	switch detectPPTQuestionScenario(session.UserPrompt) {
	case "explainer":
		return strings.Join([]string{
			"- execution_prompt must explicitly state the intended audience familiarity level.",
			"- execution_prompt must explicitly state a 6-8 slide target and keep the structure direct rather than consulting-style.",
			"- execution_prompt must explicitly state the image strategy, including a cover hero visual allowance and a total budget of 2-3 strong related images when images are used.",
			"- execution_prompt must explicitly state density rules: short titles, short card headings, preserve complete visible wording, and reflow or split instead of clipping copy.",
			"- execution_prompt must explicitly state the ending style as beginner tips, who it suits, or how to start, and must forbid owners, milestones, rollout, or business next-step framing.",
		}, "\n")
	default:
		return ""
	}
}
