package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/officecli/officecli/engine"
)

type questionMeta struct {
	Provider         string
	Model            string
	BaseURL          string
	ValidationRule   string
	ValidationDetail string
	RawPreview       string
}

type planQuestionValidationError struct {
	Rule   string
	Detail string
}

func (e *planQuestionValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Detail
}

func (w *Workflow) synthesizeQuestions(ctx context.Context, req engine.PrepareExecutionPlanRequest, documentType string) ([]engine.PlanQuestion, questionMeta, error) {
	if w == nil || w.llm == nil {
		return nil, questionMeta{}, errors.New("llm unavailable")
	}
	structuredReq := engine.StructuredCompletionRequest{
		Messages: []engine.LLMMessage{
			{Role: "system", Content: "你是一个资深中文办公文档 AI 助手，负责在执行前通过少量高价值问题补齐信息。你只返回符合 schema 的 JSON，不要输出任何额外说明。"},
			{Role: "user", Content: buildQuestionContext(req, documentType)},
		},
		Schema: engine.StructuredSchema{
			Name:        "execution_plan_questions",
			Description: "Dynamic clarification questions for execution plan",
			JSONSchema:  []byte(planQuestionStructuredSchema),
			Strict:      true,
		},
	}
	var lastErr error
	meta := questionMeta{}
	for range 2 {
		attemptCtx, cancel := w.withTimeout(ctx, w.questionAttemptTimeout)
		response, err := w.llm.CompleteStructured(attemptCtx, structuredReq)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		questions, err := decodeQuestions(response)
		if err != nil {
			lastErr = err
			if validationErr := new(planQuestionValidationError); errors.As(err, &validationErr) && validationErr != nil {
				meta.ValidationRule = strings.TrimSpace(validationErr.Rule)
				meta.ValidationDetail = strings.TrimSpace(validationErr.Detail)
				meta.RawPreview = truncateRawPreview(response)
			}
			continue
		}
		return questions, meta, nil
	}
	if lastErr == nil {
		lastErr = errors.New("question synthesis failed")
	}
	if meta.ValidationRule != "" {
		return nil, meta, lastErr
	}
	attemptCtx, cancel := w.withTimeout(ctx, w.questionAttemptTimeout)
	response, err := w.llm.CompleteJSON(attemptCtx, structuredReq.Messages)
	cancel()
	if err == nil {
		questions, decodeErr := decodeQuestions(response)
		if decodeErr == nil {
			return questions, meta, nil
		}
		lastErr = decodeErr
	}
	return nil, meta, lastErr
}

func (w *Workflow) fallbackQuestions(ctx context.Context, req engine.PrepareExecutionPlanRequest, documentType string) ([]engine.PlanQuestion, error) {
	if w == nil || w.llm == nil {
		return nil, errors.New("llm unavailable")
	}
	response, err := w.llm.CompleteJSON(ctx, []engine.LLMMessage{
		{Role: "system", Content: "你是一个资深中文办公文档 AI 助手。请只输出 JSON。"},
		{Role: "user", Content: buildQuestionContext(req, documentType)},
	})
	if err != nil {
		return nil, err
	}
	return decodeQuestions(response)
}

func classifyQuestionError(err error) string {
	if err == nil {
		return ""
	}
	var validationErr *planQuestionValidationError
	if errors.As(err, &validationErr) && validationErr != nil {
		return "schema_validate_failed"
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "invalid structured json"), strings.Contains(msg, "decode"):
		return "schema_decode_failed"
	case strings.Contains(msg, "recommended count"), strings.Contains(msg, "invalid option count"), strings.Contains(msg, "question"):
		return "schema_validate_failed"
	case strings.Contains(msg, "unauthorized"), strings.Contains(msg, "401"), strings.Contains(msg, "auth"):
		return "auth_failed"
	default:
		return "llm_request_failed"
	}
}

func truncateRawPreview(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) <= 240 {
		return raw
	}
	return raw[:240]
}

func buildQuestionContext(req engine.PrepareExecutionPlanRequest, documentType string) string {
	var sb strings.Builder
	sb.WriteString("请根据以下信息生成有针对性的澄清问题。\n")
	sb.WriteString("用户原始输入：")
	sb.WriteString(strings.TrimSpace(req.UserPrompt))
	sb.WriteString("\n")
	sb.WriteString("任务类型：新建文档\n")
	sb.WriteString("文档类型：")
	sb.WriteString(documentType)
	sb.WriteString("\n")
	sb.WriteString("生成模式：")
	sb.WriteString(strings.TrimSpace(req.GenerationMode))
	sb.WriteString("\n")
	sb.WriteString(buildQuestionGoal(documentType))
	sb.WriteString("\n\n")
	sb.WriteString(`要求：
1. 只问对内容质量最敏感、最能缩小生成空间的问题。
2. 输出 1 到 3 个问题，优先少而准。
3. 每个问题提供 2 到 4 个可点击选项，并且恰好 1 个选项标记 recommended=true。
4. allowFreeform 通常为 true。
5. 所有文案必须用中文，不要出现英文问题标题。
6. 问题之间不要重复，避免泛泛地问“还有什么补充”。`)
	return sb.String()
}

func buildQuestionGoal(documentType string) string {
	switch normalizeDocumentType(documentType) {
	case "docx":
		return "本次目标：通过最少问题补齐文档用途、目标读者、篇幅与论证深度，让后续文档结构更完整、表达更专业。"
	case "xlsx":
		return "本次目标：通过最少问题补齐分析目标、数据粒度、指标口径和输出视角，让后续工作簿结构更适合分析场景。"
	default:
		return "本次目标：通过最少问题补齐受众、汇报目标、结构方式和页数密度，让后续 PPT 更适合咨询风汇报。"
	}
}

const planQuestionStructuredSchema = `{
  "type":"object",
  "additionalProperties":false,
  "required":["questions"],
  "properties":{
    "questions":{
      "type":"array",
      "minItems":1,
      "maxItems":5,
      "items":{
        "type":"object",
        "additionalProperties":false,
        "required":["id","question","options","allowFreeform"],
        "properties":{
          "id":{"type":"string"},
          "question":{"type":"string"},
          "allowFreeform":{"type":"boolean"},
          "options":{
            "type":"array",
            "minItems":2,
            "maxItems":4,
            "items":{
              "type":"object",
              "additionalProperties":false,
              "required":["label","description"],
              "properties":{
                "id":{"type":"string"},
                "label":{"type":"string"},
                "description":{"type":"string"},
                "recommended":{"type":"boolean"}
              }
            }
          }
        }
      }
    }
  }
}`

func decodeQuestions(raw string) ([]engine.PlanQuestion, error) {
	normalized, err := normalizeQuestionPayload(raw)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Questions []questionPayload `json:"questions"`
	}
	if err := json.Unmarshal([]byte(normalized), &payload); err != nil {
		return nil, fmt.Errorf("decode plan questions: %w", err)
	}
	questions := make([]engine.PlanQuestion, 0, len(payload.Questions))
	for i, question := range payload.Questions {
		qID := normalizeIdentifier(question.ID, fmt.Sprintf("question-%d", i+1))
		qText := strings.TrimSpace(question.Question)
		if qText == "" {
			qText = strings.TrimSpace(question.Text)
		}
		q := engine.PlanQuestion{
			ID:            qID,
			Question:      qText,
			AllowFreeform: question.AllowFreeform || !question.AllowFreeform,
			Options:       make([]engine.PlanQuestionOption, 0, len(question.Options)),
		}
		for j, option := range question.Options {
			optID := normalizeIdentifier(option.ID, fmt.Sprintf("%s-option-%d", qID, j+1))
			q.Options = append(q.Options, engine.PlanQuestionOption{
				ID:          optID,
				Label:       strings.TrimSpace(option.Label),
				Description: strings.TrimSpace(option.Description),
				Recommended: option.Recommended,
			})
		}
		questions = append(questions, q)
	}
	if err := validateQuestions(questions); err != nil {
		return nil, err
	}
	return questions, nil
}

type questionPayload struct {
	ID            string          `json:"id"`
	Question      string          `json:"question"`
	Text          string          `json:"text"`
	AllowFreeform bool            `json:"allowFreeform"`
	Options       []optionPayload `json:"options"`
}

type optionPayload struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Recommended bool   `json:"recommended"`
}

func normalizeQuestionPayload(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw, nil
	}
	if questions, ok := payload["clarificationQuestions"]; ok && payload["questions"] == nil {
		payload["questions"] = questions
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func normalizeIdentifier(value string, fallback string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '_' || r == '-':
			if b.Len() > 0 && !prevDash {
				b.WriteRune(r)
				prevDash = true
			}
		default:
			if b.Len() > 0 && !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	normalized := strings.Trim(b.String(), "-")
	if normalized == "" {
		return fallback
	}
	return normalized
}

func validateQuestions(questions []engine.PlanQuestion) error {
	if len(questions) == 0 || len(questions) > 3 {
		return &planQuestionValidationError{Rule: "invalid_question_count", Detail: fmt.Sprintf("invalid question count: %d", len(questions))}
	}
	for _, question := range questions {
		if strings.TrimSpace(question.ID) == "" || strings.TrimSpace(question.Question) == "" {
			return &planQuestionValidationError{Rule: "missing_question_fields", Detail: "question id/text is required"}
		}
		if len(question.Options) < 2 || len(question.Options) > 4 {
			return &planQuestionValidationError{Rule: "invalid_option_count", Detail: fmt.Sprintf("question %s has invalid option count: %d", question.ID, len(question.Options))}
		}
		recommendedCount := 0
		for _, option := range question.Options {
			if strings.TrimSpace(option.ID) == "" || strings.TrimSpace(option.Label) == "" || strings.TrimSpace(option.Description) == "" {
				return &planQuestionValidationError{Rule: "empty_option_fields", Detail: fmt.Sprintf("question %s has empty option fields", question.ID)}
			}
			if option.Recommended {
				recommendedCount++
			}
		}
		if recommendedCount != 1 {
			return &planQuestionValidationError{Rule: "invalid_recommended_count", Detail: fmt.Sprintf("question %s recommended count = %d", question.ID, recommendedCount)}
		}
	}
	return nil
}

func buildExecutionPlanQuestions(documentType string) []engine.PlanQuestion {
	switch normalizeDocumentType(documentType) {
	case "docx":
		return []engine.PlanQuestion{
			{
				ID:       "docx_goal",
				Question: "这份文档最重要的用途是什么？",
				Options: []engine.PlanQuestionOption{
					{ID: "report", Label: "分析报告", Description: "强调结论、分析链路和建议。", Recommended: true},
					{ID: "proposal", Label: "方案文档", Description: "强调行动方案、路径和落地。"},
					{ID: "memo", Label: "纪要/总结", Description: "强调事实归纳、决定和后续事项。"},
				},
				AllowFreeform: true,
			},
			{
				ID:       "docx_audience",
				Question: "这份文档主要给谁看？",
				Options: []engine.PlanQuestionOption{
					{ID: "management", Label: "管理层", Description: "更强调结论、判断和建议。", Recommended: true},
					{ID: "client", Label: "客户/外部伙伴", Description: "更强调价值表达和可读性。"},
					{ID: "team", Label: "团队内部", Description: "更强调背景、方法和执行细节。"},
				},
				AllowFreeform: true,
			},
			{
				ID:       "docx_depth",
				Question: "这份文档希望写到什么深度？",
				Options: []engine.PlanQuestionOption{
					{ID: "concise", Label: "短版结论型", Description: "控制篇幅，先给结论和关键分析。", Recommended: true},
					{ID: "balanced", Label: "平衡型", Description: "兼顾结论、分析和建议。"},
					{ID: "deep", Label: "深度展开", Description: "允许更完整的论证、背景和细节。"},
				},
				AllowFreeform: true,
			},
		}
	case "xlsx":
		return []engine.PlanQuestion{
			{
				ID:       "xlsx_goal",
				Question: "这份表格最重要的分析目标是什么？",
				Options: []engine.PlanQuestionOption{
					{ID: "summary", Label: "经营摘要", Description: "先给核心 KPI、偏差与摘要。", Recommended: true},
					{ID: "trend", Label: "趋势分析", Description: "强调时间维度变化和走势。"},
					{ID: "detail", Label: "明细核对", Description: "强调明细表、核对与追溯。"},
				},
				AllowFreeform: true,
			},
			{
				ID:       "xlsx_granularity",
				Question: "结果展示更偏向哪种粒度？",
				Options: []engine.PlanQuestionOption{
					{ID: "monthly", Label: "按月", Description: "适合经营分析和趋势对比。", Recommended: true},
					{ID: "quarterly", Label: "按季度", Description: "适合高层汇总和阶段复盘。"},
					{ID: "custom", Label: "自定义维度", Description: "自己指定时间段、区域或产品维度。"},
				},
				AllowFreeform: true,
			},
			{
				ID:       "xlsx_view",
				Question: "这份表格希望优先呈现哪种视角？",
				Options: []engine.PlanQuestionOption{
					{ID: "variance", Label: "预算偏差", Description: "强调实际值、预算值和差异。", Recommended: true},
					{ID: "composition", Label: "结构拆分", Description: "强调地区、产品或渠道构成。"},
					{ID: "pipeline", Label: "进度跟踪", Description: "强调状态、负责人和时间节点。"},
				},
				AllowFreeform: true,
			},
		}
	default:
		return []engine.PlanQuestion{
			{
				ID:       "ppt_audience",
				Question: "这份内容主要给谁看？",
				Options: []engine.PlanQuestionOption{
					{ID: "management", Label: "管理层", Description: "更强调结论、判断和决策建议。", Recommended: true},
					{ID: "client", Label: "客户/外部伙伴", Description: "更强调价值、成果和说服力。"},
					{ID: "team", Label: "团队内部", Description: "更强调背景、过程和执行细节。"},
				},
				AllowFreeform: true,
			},
			{
				ID:       "ppt_goal",
				Question: "这次汇报最希望先解决什么？",
				Options: []engine.PlanQuestionOption{
					{ID: "decision", Label: "快速决策", Description: "优先呈现结论、依据和建议动作。", Recommended: true},
					{ID: "alignment", Label: "统一认知", Description: "优先交代背景、现状和判断框架。"},
					{ID: "progress", Label: "同步进展", Description: "优先说明结果、风险和下一步。"},
				},
				AllowFreeform: true,
			},
			{
				ID:       "ppt_shape",
				Question: "你希望这份 PPT 更偏向哪种组织方式？",
				Options: []engine.PlanQuestionOption{
					{ID: "concise", Label: "短而结论先行", Description: "页数更少，适合 6-8 页短汇报。", Recommended: true},
					{ID: "balanced", Label: "平衡型", Description: "兼顾背景、过程和结论。"},
					{ID: "detailed", Label: "内容更完整", Description: "允许更多页和更细解释。"},
				},
				AllowFreeform: true,
			},
		}
	}
}

func buildDynamicFallbackQuestions(req engine.PrepareExecutionPlanRequest, documentType string) []engine.PlanQuestion {
	prompt := strings.TrimSpace(req.UserPrompt)
	switch normalizeDocumentType(documentType) {
	case "docx":
		questions := buildExecutionPlanQuestions("docx")
		switch {
		case containsAnyKeyword(prompt, "报告", "分析", "研究", "洞察"):
			questions[0].Question = "这份分析文档最需要突出什么价值？"
			questions[0].Options = []engine.PlanQuestionOption{
				{ID: "insight", Label: "关键洞察", Description: "先给结论、趋势和判断。", Recommended: true},
				{ID: "strategy", Label: "策略建议", Description: "先给建议、路径和动作。"},
				{ID: "evidence", Label: "论据依据", Description: "先给事实、数据和分析链路。"},
			}
		case containsAnyKeyword(prompt, "方案", "规划", "建议书"):
			questions[0].Question = "这份方案文档最该突出哪部分？"
			questions[0].Options = []engine.PlanQuestionOption{
				{ID: "path", Label: "实施路径", Description: "先讲目标、阶段和落地步骤。", Recommended: true},
				{ID: "value", Label: "业务价值", Description: "先讲收益、影响和优先级。"},
				{ID: "risk", Label: "风险与保障", Description: "先讲依赖、风险和兜底。"},
			}
		}
		return questions
	case "xlsx":
		questions := buildExecutionPlanQuestions("xlsx")
		switch {
		case containsAnyKeyword(prompt, "预算", "偏差", "费用", "收入"):
			questions[2].Question = "这份经营分析表最想先看到什么偏差视角？"
			questions[2].Options = []engine.PlanQuestionOption{
				{ID: "budget", Label: "预算 vs 实际", Description: "先看偏差金额与原因。", Recommended: true},
				{ID: "region", Label: "区域差异", Description: "先看地区间差异和表现。"},
				{ID: "product", Label: "产品结构", Description: "先看产品线贡献和拖累。"},
			}
		case containsAnyKeyword(prompt, "项目", "进度", "台账", "跟踪"):
			questions[0].Question = "这份台账最重要的是哪类管理目标？"
			questions[0].Options = []engine.PlanQuestionOption{
				{ID: "risk", Label: "风险预警", Description: "优先标记逾期、阻塞和异常。", Recommended: true},
				{ID: "milestone", Label: "里程碑进度", Description: "优先跟踪关键节点达成情况。"},
				{ID: "resource", Label: "资源协调", Description: "优先呈现负责人、依赖和资源状态。"},
			}
		}
		return questions
	default:
		questions := buildExecutionPlanQuestions("pptx")
		switch {
		case containsAnyKeyword(prompt, "融资", "路演", "投资人", "商业模式", "估值"):
			return []engine.PlanQuestion{
				{
					ID:       "ppt_pitch_audience",
					Question: "这次融资路演主要面对哪类听众？",
					Options: []engine.PlanQuestionOption{
						{ID: "vc", Label: "财务投资人", Description: "更关注市场空间、增长与回报。", Recommended: true},
						{ID: "strategic", Label: "产业投资人", Description: "更关注协同价值和行业位置。"},
						{ID: "mixed", Label: "混合受众", Description: "既要讲市场，也要讲产品和商业化。"},
					},
					AllowFreeform: true,
				},
				{
					ID:       "ppt_pitch_focus",
					Question: "这次路演最想先打动对方哪一点？",
					Options: []engine.PlanQuestionOption{
						{ID: "market", Label: "市场机会", Description: "先放大赛道空间、痛点和窗口期。", Recommended: true},
						{ID: "product", Label: "产品与壁垒", Description: "先讲产品能力、差异化和护城河。"},
						{ID: "business", Label: "商业模式", Description: "先讲收入逻辑、增长路径和融资用途。"},
					},
					AllowFreeform: true,
				},
				questions[1],
				questions[2],
			}
		case containsAnyKeyword(prompt, "项目", "汇报", "进度", "里程碑", "复盘", "计划", "季度"):
			return []engine.PlanQuestion{
				{
					ID:            "ppt_report_audience",
					Question:      "这次项目汇报主要给谁看？",
					Options:       questions[0].Options,
					AllowFreeform: true,
				},
				{
					ID:       "ppt_report_focus",
					Question: "这次项目汇报最该突出什么？",
					Options: []engine.PlanQuestionOption{
						{ID: "progress", Label: "当前进度", Description: "优先说明里程碑推进到哪里。", Recommended: true},
						{ID: "achievements", Label: "阶段成果", Description: "优先强调已经拿到的结果和亮点。"},
						{ID: "next_plan", Label: "后续计划与风险", Description: "优先讲下一步安排、风险和资源诉求。"},
					},
					AllowFreeform: true,
				},
				questions[1],
				questions[2],
			}
		case containsAnyKeyword(prompt, "培训", "课堂", "教学", "分享"):
			return []engine.PlanQuestion{
				questions[0],
				{
					ID:       "ppt_share_focus",
					Question: "这次分享最希望听众先记住什么？",
					Options: []engine.PlanQuestionOption{
						{ID: "framework", Label: "核心框架", Description: "先建立概念和结构。", Recommended: true},
						{ID: "case", Label: "案例示例", Description: "先用案例帮助理解。"},
						{ID: "practice", Label: "实操步骤", Description: "先讲可落地的方法和动作。"},
					},
					AllowFreeform: true,
				},
				questions[1],
				questions[2],
			}
		default:
			return questions
		}
	}
}

func containsAnyKeyword(content string, keywords ...string) bool {
	content = strings.TrimSpace(strings.ToLower(content))
	if content == "" {
		return false
	}
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(content, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}
