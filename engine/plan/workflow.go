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
				questionFallbackReason = "动态澄清题生成失败，已降级为固定澄清题。"
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
		Answer:     "计划修订：" + strings.TrimSpace(req.Instruction),
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
	sb.WriteString("# 执行计划\n\n")
	sb.WriteString("## 目标理解\n")
	sb.WriteString("- 本次任务：新建一份")
	sb.WriteString(buildDocumentLabel(session.DocumentType))
	sb.WriteString("。\n")
	if prompt := strings.TrimSpace(session.UserPrompt); prompt != "" {
		sb.WriteString("- 原始需求：")
		sb.WriteString(prompt)
		sb.WriteString("\n")
	}
	if session.EditTarget != "" {
		sb.WriteString("- 优先编辑区域：")
		sb.WriteString(strings.TrimSpace(session.EditTarget))
		sb.WriteString("\n")
	}
	if session.FrameworkBlueprint != "" {
		sb.WriteString("\n")
		sb.WriteString(strings.TrimSpace(session.FrameworkBlueprint))
		sb.WriteString("\n")
	}
	sb.WriteString("\n## 执行步骤\n")
	if generateengine.NormalizeGenerationMode(session.GenerationMode) == generateengine.ModeBest {
		sb.WriteString("1. **主题识别**：先明确核心主线、受众和输出目标。\n")
		sb.WriteString("2. **整体框架蓝图**：先规划结构骨架，再确定重点分布与表达方式。\n")
		sb.WriteString("3. **逐步展开内容**：按确认后的详略、结构和质量约束展开内容。\n")
		sb.WriteString("4. **确认后正式生成**：在你确认计划后，再进入实际生成。\n")
	} else {
		sb.WriteString("1. 先明确文档主线、受众和输出方式。\n")
		sb.WriteString("2. 再组织整体结构并展开内容。\n")
		sb.WriteString("3. 在你确认计划后，再进入实际生成。\n")
	}
	sb.WriteString("\n## 关键约束\n")
	for _, line := range buildConstraintLines(session) {
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	if strings.TrimSpace(session.ExecutionPrompt) != "" {
		sb.WriteString("\n## 执行基线\n")
		sb.WriteString("- 已生成结构化执行提示，后续生成将严格参考这份计划。\n")
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
			lines = append(lines, fmt.Sprintf("- %s：%s", strings.TrimSpace(question.Question), strings.TrimSpace(answer.Answer)))
			continue
		}
		lines = append(lines, fmt.Sprintf("- 补充要求：%s", strings.TrimSpace(answer.Answer)))
	}
	lines = append(lines, "- 以用户原始需求为主线组织内容，不扩写无关部分。")
	lines = append(lines, "- 在你确认之前，这份计划只用于审阅，不会直接进入生成。")
	return lines
}

func buildDocumentLabel(documentType string) string {
	switch normalizeDocumentType(documentType) {
	case "docx":
		return "Word 文档"
	case "xlsx":
		return "Excel 表格"
	default:
		return "PPT 演示文稿"
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
	sb.WriteString("请严格按照以下已确认计划生成新文档，不要偏离范围。\n")
	sb.WriteString("原始需求：")
	sb.WriteString(strings.TrimSpace(session.UserPrompt))
	sb.WriteString("\n")
	sb.WriteString("文档类型：")
	sb.WriteString(normalizeDocumentType(session.DocumentType))
	sb.WriteString("\n")
	for i, answer := range session.Answers {
		sb.WriteString("补充说明 ")
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString("：")
		sb.WriteString(strings.TrimSpace(answer.Answer))
		sb.WriteString("\n")
	}
	if blueprint := strings.TrimSpace(session.FrameworkBlueprint); blueprint != "" {
		sb.WriteString("结构蓝图摘要：\n")
		sb.WriteString(blueprint)
		sb.WriteString("\n")
	}
	sb.WriteString("质量约束：\n")
	for _, line := range buildExecutionQualityConstraints(session.DocumentType) {
		sb.WriteString("- ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("请确保最终内容与结构蓝图、补充说明、质量约束完全一致，不扩写无关内容。")
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
	if blueprint == "" || strings.Contains(planMarkdown, "## 框架蓝图") {
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
			answers.WriteString("：")
			answers.WriteString(strings.TrimSpace(answer.Answer))
			answers.WriteString("\n")
			continue
		}
		answers.WriteString("- 补充要求：")
		answers.WriteString(strings.TrimSpace(answer.Answer))
		answers.WriteString("\n")
	}
	if answers.Len() == 0 {
		answers.WriteString("- 无额外补充\n")
	}
	var blueprintSection string
	if strings.TrimSpace(session.FrameworkBlueprint) != "" {
		blueprintSection = "\n现有框架蓝图：\n" + strings.TrimSpace(session.FrameworkBlueprint) + "\n"
	}
	constraints := strings.Join(buildExecutionQualityConstraints(session.DocumentType), "\n- ")
	if constraints != "" {
		constraints = "- " + constraints
	}
	userPrompt := fmt.Sprintf(`请基于以下信息，整理一份给用户审阅的执行计划，并同时输出后续执行基线。

任务类型：新建文档
文档类型：%s
原始需求：%s
修订版本：%d

澄清结果：
%s
%s

输出要求：
1. plan_markdown 必须是 markdown。
2. 如果存在现有框架蓝图，plan_markdown 必须保留一个“## 框架蓝图”章节，并吸收其中的结构规划要点。
3. 如果是最佳效果的新建文档计划，要在实施方案里明确体现并高亮前两步标题：**主题识别**、**整体框架蓝图**。
4. execution_prompt 必须是严格的执行基线，供后续真正生成复用。
5. execution_prompt 必须明确写入以下质量约束：
%s`,
		buildDocumentLabel(session.DocumentType),
		strings.TrimSpace(session.UserPrompt),
		session.Revision,
		strings.TrimSpace(answers.String()),
		blueprintSection,
		constraints,
	)
	return []engine.LLMMessage{
		{Role: "system", Content: "你是办公文档执行计划整理器。你必须只输出符合 schema 的 JSON。"},
		{Role: "user", Content: userPrompt},
	}
}

func buildExecutionQualityConstraints(documentType string) []string {
	switch normalizeDocumentType(documentType) {
	case "docx":
		return []string{
			"章节顺序清晰，先结论后分析，避免结构跳跃。",
			"段落表达正式专业，避免空话、套话和无信息量表述。",
			"每节都要围绕用途和读者层级控制论证深度与篇幅。",
		}
	case "xlsx":
		return []string{
			"sheet 职责清晰，摘要与明细分工明确。",
			"字段一致性和指标口径必须统一，避免同义字段混用。",
			"结果优先服务分析目标，必要时先给管理摘要再展开明细。",
		}
	default:
		return []string{
			"控制页数与信息密度，优先形成 6-8 页结论先行的短 deck。",
			"每页只承担一个清晰职责，避免重复页和无效铺陈。",
			"页间叙事要递进，标题和要点必须服务于核心结论。",
		}
	}
}
