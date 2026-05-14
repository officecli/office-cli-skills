package plan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/officecli/officecli-internal/engine"
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
			{Role: "system", Content: "You are a senior office-document AI assistant. Before execution, ask a small number of high-value clarification questions. Return JSON that matches the schema exactly, with no extra text."},
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
	language := detectQuestionLanguageName(req.UserPrompt)
	pptScenario := detectPPTQuestionScenario(req.UserPrompt)
	var sb strings.Builder
	sb.WriteString("Generate focused clarification questions based on the information below.\n")
	sb.WriteString("Original user prompt: ")
	sb.WriteString(strings.TrimSpace(req.UserPrompt))
	sb.WriteString("\n")
	sb.WriteString("Task type: create a new document\n")
	sb.WriteString("Document type: ")
	sb.WriteString(documentType)
	sb.WriteString("\n")
	sb.WriteString("Generation mode: ")
	sb.WriteString(strings.TrimSpace(req.GenerationMode))
	sb.WriteString("\n")
	if normalizeDocumentType(documentType) == "pptx" {
		sb.WriteString("PPT scenario: ")
		sb.WriteString(pptScenario)
		sb.WriteString("\n")
	}
	sb.WriteString(buildQuestionGoal(req, documentType))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf(`Requirements:
1. Ask only the questions that most improve content quality and narrow the generation space.
2. Return 1 to 3 questions, favoring fewer but sharper questions.
3. Each question must include 2 to 4 selectable options, with exactly 1 option marked recommended=true.
4. allowFreeform should usually be true.
5. Write all question text in %s.
6. For PPT decks, prioritize audience, intro angle, content density, image preference, and ending style when those details are missing.
7. If the scenario is explainer, entertainment, or game-related, avoid consulting or executive jargon.
8. Avoid overlap between questions and avoid vague catch-all prompts.`, language))
	return sb.String()
}

func buildQuestionGoal(req engine.PrepareExecutionPlanRequest, documentType string) string {
	switch normalizeDocumentType(documentType) {
	case "docx":
		return "Goal: fill in document purpose, target readers, length, and argument depth with the fewest questions so the final document structure is complete and professional."
	case "xlsx":
		return "Goal: fill in analysis goal, data granularity, metric definitions, and reporting perspective with the fewest questions so the workbook structure fits the analysis scenario."
	case "report":
		return "Goal: fill in audience, report storyline, chart density, and decision focus with the fewest questions so the workbook-backed report is narrative, data-rich, and presentation-ready."
	default:
		switch detectPPTQuestionScenario(req.UserPrompt) {
		case "explainer":
			return "Goal: fill in audience familiarity, intro angle, structural approach, image preference, and page density with the fewest questions so the PPT reads like a clear explainer deck instead of a consulting-style deck."
		case "project":
			return "Goal: fill in audience, update focus, structural approach, and page density with the fewest questions so the PPT reads like a sharp progress update."
		case "training":
			return "Goal: fill in learner level, lesson emphasis, structural approach, and page density with the fewest questions so the PPT is practical and easy to follow."
		default:
			return "Goal: fill in audience, presentation goal, structural approach, and page density with the fewest questions so the PPT fits the real use case instead of defaulting to a consulting-style deck."
		}
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
              "required":["id","label","description","recommended"],
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
				Question: "What is the most important purpose of this document?",
				Options: []engine.PlanQuestionOption{
					{ID: "report", Label: "Analytical report", Description: "Emphasize conclusions, reasoning, and recommendations.", Recommended: true},
					{ID: "proposal", Label: "Proposal", Description: "Emphasize action plans, path, and execution."},
					{ID: "memo", Label: "Memo or recap", Description: "Emphasize facts, decisions, and follow-up items."},
				},
				AllowFreeform: true,
			},
			{
				ID:       "docx_audience",
				Question: "Who is the primary audience for this document?",
				Options: []engine.PlanQuestionOption{
					{ID: "management", Label: "Leadership", Description: "Emphasize conclusions, judgment, and recommendations.", Recommended: true},
					{ID: "client", Label: "Client or partner", Description: "Emphasize value communication and readability."},
					{ID: "team", Label: "Internal team", Description: "Emphasize background, method, and execution detail."},
				},
				AllowFreeform: true,
			},
			{
				ID:       "docx_depth",
				Question: "How deep should this document go?",
				Options: []engine.PlanQuestionOption{
					{ID: "concise", Label: "Concise", Description: "Keep it short and lead with conclusions and key analysis.", Recommended: true},
					{ID: "balanced", Label: "Balanced", Description: "Balance conclusions, analysis, and recommendations."},
					{ID: "deep", Label: "Deep dive", Description: "Allow fuller argumentation, background, and detail."},
				},
				AllowFreeform: true,
			},
		}
	case "xlsx":
		return []engine.PlanQuestion{
			{
				ID:       "xlsx_goal",
				Question: "What is the main analysis goal of this workbook?",
				Options: []engine.PlanQuestionOption{
					{ID: "summary", Label: "Business summary", Description: "Lead with core KPIs, variance, and summary.", Recommended: true},
					{ID: "trend", Label: "Trend analysis", Description: "Emphasize time-based changes and trajectories."},
					{ID: "detail", Label: "Detail audit", Description: "Emphasize detailed tables, checking, and traceability."},
				},
				AllowFreeform: true,
			},
			{
				ID:       "xlsx_granularity",
				Question: "What level of granularity should the output emphasize?",
				Options: []engine.PlanQuestionOption{
					{ID: "monthly", Label: "Monthly", Description: "Fits business analysis and trend comparison.", Recommended: true},
					{ID: "quarterly", Label: "Quarterly", Description: "Fits executive summaries and stage reviews."},
					{ID: "custom", Label: "Custom", Description: "Use custom time, region, or product dimensions."},
				},
				AllowFreeform: true,
			},
			{
				ID:       "xlsx_view",
				Question: "Which perspective should this workbook prioritize?",
				Options: []engine.PlanQuestionOption{
					{ID: "variance", Label: "Budget variance", Description: "Emphasize actuals, budget, and variance.", Recommended: true},
					{ID: "composition", Label: "Composition split", Description: "Emphasize regional, product, or channel mix."},
					{ID: "pipeline", Label: "Progress tracking", Description: "Emphasize status, owner, and timeline milestones."},
				},
				AllowFreeform: true,
			},
		}
	case "report":
		return []engine.PlanQuestion{
			{
				ID:       "report_audience",
				Question: "Who is the primary audience for this workbook-backed report?",
				Options: []engine.PlanQuestionOption{
					{ID: "exec", Label: "Executives or board", Description: "Lead with concise conclusions, KPI shifts, and decision implications.", Recommended: true},
					{ID: "client", Label: "Clients or partners", Description: "Lead with externally understandable framing and business impact."},
					{ID: "ops", Label: "Operating teams", Description: "Keep more implementation detail and supporting evidence."},
				},
				AllowFreeform: true,
			},
			{
				ID:       "report_story",
				Question: "What narrative should this report emphasize first?",
				Options: []engine.PlanQuestionOption{
					{ID: "business_review", Label: "Business review", Description: "Summarize performance, changes, and next actions.", Recommended: true},
					{ID: "market_report", Label: "Market insight", Description: "Explain external trends, comparison, and opportunity."},
					{ID: "pipeline", Label: "Pipeline or operating review", Description: "Explain funnel, conversion, and execution bottlenecks."},
				},
				AllowFreeform: true,
			},
			{
				ID:       "report_density",
				Question: "How chart-heavy should the report be?",
				Options: []engine.PlanQuestionOption{
					{ID: "balanced", Label: "Balanced", Description: "Keep each section narrative-led with one strong chart.", Recommended: true},
					{ID: "chart_heavy", Label: "Chart-heavy", Description: "Use more visual evidence and denser KPI framing."},
					{ID: "light", Label: "Narrative-first", Description: "Use fewer charts and more explanatory text."},
				},
				AllowFreeform: true,
			},
		}
	default:
		return []engine.PlanQuestion{
			{
				ID:       "ppt_audience",
				Question: "Who is the main audience for this deck?",
				Options: []engine.PlanQuestionOption{
					{ID: "management", Label: "Leadership", Description: "Emphasize conclusions, judgment, and decisions.", Recommended: true},
					{ID: "client", Label: "Client or partner", Description: "Emphasize value, outcomes, and persuasion."},
					{ID: "team", Label: "Internal team", Description: "Emphasize background, process, and execution detail."},
				},
				AllowFreeform: true,
			},
			{
				ID:       "ppt_goal",
				Question: "What should this presentation solve first?",
				Options: []engine.PlanQuestionOption{
					{ID: "decision", Label: "Fast decision", Description: "Prioritize conclusions, rationale, and recommended actions.", Recommended: true},
					{ID: "alignment", Label: "Alignment", Description: "Prioritize background, current state, and the reasoning frame."},
					{ID: "progress", Label: "Progress sync", Description: "Prioritize results, risks, and next steps."},
				},
				AllowFreeform: true,
			},
			{
				ID:       "ppt_shape",
				Question: "What structure should this PPT lean toward?",
				Options: []engine.PlanQuestionOption{
					{ID: "concise", Label: "Concise and conclusion-first", Description: "Use fewer slides and fit a 6-8 slide short deck.", Recommended: true},
					{ID: "balanced", Label: "Balanced", Description: "Balance background, process, and conclusions."},
					{ID: "detailed", Label: "More complete", Description: "Allow more slides and finer explanation."},
				},
				AllowFreeform: true,
			},
		}
	}
}

func buildTemplateFallbackQuestions(req engine.PrepareExecutionPlanRequest, documentType string) []engine.PlanQuestion {
	prompt := strings.TrimSpace(req.UserPrompt)
	switch normalizeDocumentType(documentType) {
	case "docx":
		questions := buildExecutionPlanQuestions("docx")
		switch {
		case containsAnyKeyword(prompt, "report", "analysis", "research", "insight", "\u62a5\u544a", "\u5206\u6790", "\u7814\u7a76", "\u6d1e\u5bdf"):
			questions[0].Question = "What value should this analytical document emphasize most?"
			questions[0].Options = []engine.PlanQuestionOption{
				{ID: "insight", Label: "Key insights", Description: "Lead with conclusions, trends, and judgment.", Recommended: true},
				{ID: "strategy", Label: "Strategic recommendations", Description: "Lead with recommendations, path, and actions."},
				{ID: "evidence", Label: "Evidence base", Description: "Lead with facts, data, and analytical reasoning."},
			}
		case containsAnyKeyword(prompt, "proposal", "plan", "recommendation", "\u65b9\u6848", "\u89c4\u5212", "\u5efa\u8bae\u4e66"):
			questions[0].Question = "Which part should this proposal emphasize most?"
			questions[0].Options = []engine.PlanQuestionOption{
				{ID: "path", Label: "Execution path", Description: "Lead with goals, phases, and rollout steps.", Recommended: true},
				{ID: "value", Label: "Business value", Description: "Lead with benefits, impact, and priorities."},
				{ID: "risk", Label: "Risk and safeguards", Description: "Lead with dependencies, risks, and mitigation."},
			}
		}
		return questions
	case "xlsx":
		questions := buildExecutionPlanQuestions("xlsx")
		switch {
		case containsAnyKeyword(prompt, "budget", "variance", "expense", "revenue", "\u9884\u7b97", "\u504f\u5dee", "\u8d39\u7528", "\u6536\u5165"):
			questions[2].Question = "Which variance angle should this business-analysis workbook show first?"
			questions[2].Options = []engine.PlanQuestionOption{
				{ID: "budget", Label: "Budget vs actual", Description: "Show variance amount and root causes first.", Recommended: true},
				{ID: "region", Label: "Regional difference", Description: "Show performance gaps across regions first."},
				{ID: "product", Label: "Product mix", Description: "Show product-line contribution and drag first."},
			}
		case containsAnyKeyword(prompt, "project", "progress", "tracker", "tracking", "\u9879\u76ee", "\u8fdb\u5ea6", "\u53f0\u8d26", "\u8ddf\u8e2a"):
			questions[0].Question = "What management goal matters most for this tracker?"
			questions[0].Options = []engine.PlanQuestionOption{
				{ID: "risk", Label: "Risk warning", Description: "Prioritize overdue, blocked, and abnormal items.", Recommended: true},
				{ID: "milestone", Label: "Milestone progress", Description: "Prioritize the status of key milestones."},
				{ID: "resource", Label: "Resource coordination", Description: "Prioritize owners, dependencies, and resource state."},
			}
		}
		return questions
	case "report":
		questions := buildExecutionPlanQuestions("report")
		switch {
		case containsAnyKeyword(prompt, "market", "industry", "competitor", "benchmark", "demand", "trend", "\u5e02\u573a", "\u884c\u4e1a", "\u7ade\u54c1", "\u5bf9\u6807", "\u8d8b\u52bf"):
			questions[1].Question = "What should this market-facing report prove first?"
			questions[1].Options = []engine.PlanQuestionOption{
				{ID: "opportunity", Label: "Opportunity size", Description: "Lead with category size, growth, and where demand is strongest.", Recommended: true},
				{ID: "positioning", Label: "Competitive position", Description: "Lead with peer comparison and differentiation."},
				{ID: "timing", Label: "Why now", Description: "Lead with demand shifts and timing advantages."},
			}
		case containsAnyKeyword(prompt, "quarterly", "business review", "qbr", "revenue", "sales", "pipeline", "\u5b63\u5ea6", "\u590d\u76d8", "\u6536\u5165", "\u9500\u552e", "\u7ebf\u7d22"):
			questions[1].Question = "Which business-review lens matters most in this report?"
			questions[1].Options = []engine.PlanQuestionOption{
				{ID: "performance", Label: "Performance change", Description: "Lead with KPI movement, variance, and what changed.", Recommended: true},
				{ID: "efficiency", Label: "Efficiency and quality", Description: "Lead with conversion, mix, and execution quality."},
				{ID: "action", Label: "Next actions", Description: "Lead with decisions, priorities, and recommendations."},
			}
		}
		return questions
	default:
		questions := buildExecutionPlanQuestions("pptx")
		switch {
		case containsAnyKeyword(prompt, "fundraising", "roadshow", "investor", "business model", "valuation", "\u878d\u8d44", "\u8def\u6f14", "\u6295\u8d44\u4eba", "\u5546\u4e1a\u6a21\u5f0f", "\u4f30\u503c"):
			return []engine.PlanQuestion{
				{
					ID:       "ppt_pitch_audience",
					Question: "What audience is this fundraising deck primarily for?",
					Options: []engine.PlanQuestionOption{
						{ID: "vc", Label: "Financial investors", Description: "Care more about market size, growth, and returns.", Recommended: true},
						{ID: "strategic", Label: "Strategic investors", Description: "Care more about synergies and industry position."},
						{ID: "mixed", Label: "Mixed audience", Description: "Need both market story and product/commercial story."},
					},
					AllowFreeform: true,
				},
				{
					ID:       "ppt_pitch_focus",
					Question: "What should this roadshow win the audience on first?",
					Options: []engine.PlanQuestionOption{
						{ID: "market", Label: "Market opportunity", Description: "Lead with market size, pain points, and timing window.", Recommended: true},
						{ID: "product", Label: "Product and moat", Description: "Lead with capability, differentiation, and defensibility."},
						{ID: "business", Label: "Business model", Description: "Lead with revenue logic, growth path, and use of funds."},
					},
					AllowFreeform: true,
				},
				questions[1],
				questions[2],
			}
		case containsAnyKeyword(prompt, "project", "update", "progress", "milestone", "review", "plan", "quarterly", "\u9879\u76ee", "\u6c47\u62a5", "\u8fdb\u5ea6", "\u91cc\u7a0b\u7891", "\u590d\u76d8", "\u8ba1\u5212", "\u5b63\u5ea6"):
			return []engine.PlanQuestion{
				{
					ID:            "ppt_report_audience",
					Question:      "Who is the main audience for this project update?",
					Options:       questions[0].Options,
					AllowFreeform: true,
				},
				{
					ID:       "ppt_report_focus",
					Question: "What should this project update emphasize most?",
					Options: []engine.PlanQuestionOption{
						{ID: "progress", Label: "Current progress", Description: "Prioritize milestone status and advancement.", Recommended: true},
						{ID: "achievements", Label: "Stage achievements", Description: "Prioritize results and highlights already delivered."},
						{ID: "next_plan", Label: "Next plan and risks", Description: "Prioritize next steps, risks, and resource asks."},
					},
					AllowFreeform: true,
				},
				questions[1],
				questions[2],
			}
		case containsAnyKeyword(prompt, "training", "class", "teaching", "sharing", "\u57f9\u8bad", "\u8bfe\u5802", "\u6559\u5b66", "\u5206\u4eab"):
			return []engine.PlanQuestion{
				questions[0],
				{
					ID:       "ppt_share_focus",
					Question: "What should the audience remember first from this session?",
					Options: []engine.PlanQuestionOption{
						{ID: "framework", Label: "Core framework", Description: "Establish the concept and structure first.", Recommended: true},
						{ID: "case", Label: "Case example", Description: "Use examples first to improve understanding."},
						{ID: "practice", Label: "Practical steps", Description: "Lead with actionable methods and steps."},
					},
					AllowFreeform: true,
				},
				questions[1],
				questions[2],
			}
		case detectPPTQuestionScenario(prompt) == "explainer":
			return buildExplainerFallbackQuestions(prompt)
		default:
			if isChineseQuestionPrompt(prompt) {
				return buildChinesePPTFallbackQuestions()
			}
			return questions
		}
	}
}

func detectQuestionLanguageName(prompt string) string {
	if isChineseQuestionPrompt(prompt) {
		return "Simplified Chinese"
	}
	return "English"
}

func isChineseQuestionPrompt(prompt string) bool {
	han := 0
	letters := 0
	for _, r := range strings.TrimSpace(prompt) {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
			letters++
		case r >= '0' && r <= '9':
		case r > 127:
			if r >= 0x4E00 && r <= 0x9FFF {
				han++
			}
		}
	}
	return han > letters/2
}

func detectPPTQuestionScenario(prompt string) string {
	switch {
	case containsAnyKeyword(prompt, "fundraising", "roadshow", "investor", "business model", "valuation", "\u878d\u8d44", "\u8def\u6f14", "\u6295\u8d44\u4eba", "\u5546\u4e1a\u6a21\u5f0f", "\u4f30\u503c"):
		return "business"
	case containsAnyKeyword(prompt, "project", "update", "progress", "milestone", "review", "plan", "quarterly", "\u9879\u76ee", "\u6c47\u62a5", "\u8fdb\u5ea6", "\u91cc\u7a0b\u7891", "\u590d\u76d8", "\u8ba1\u5212", "\u5b63\u5ea6"):
		return "project"
	case containsAnyKeyword(prompt, "training", "class", "teaching", "sharing", "\u57f9\u8bad", "\u8bfe\u5802", "\u6559\u5b66", "\u5206\u4eab"):
		return "training"
	case containsAnyKeyword(prompt, "game", "minecraft", "explain", "introduction", "overview", "what is", "guide", "\u6e38\u620f", "\u4ecb\u7ecd", "\u79d1\u666e", "\u662f\u4ec0\u4e48", "\u600e\u4e48\u73a9", "\u5165\u95e8", "\u5386\u53f2"):
		return "explainer"
	default:
		return "general"
	}
}

func buildExplainerFallbackQuestions(prompt string) []engine.PlanQuestion {
	if isChineseQuestionPrompt(prompt) {
		return []engine.PlanQuestion{
			{
				ID:       "ppt_explainer_audience",
				Question: "这份介绍主要是讲给谁看的？",
				Options: []engine.PlanQuestionOption{
					{ID: "beginner", Label: "第一次接触的人", Description: "先讲清楚它是什么、为什么值得了解。", Recommended: true},
					{ID: "interested", Label: "有点兴趣但不熟的人", Description: "兼顾基础介绍和几个代表性亮点。"},
					{ID: "familiar", Label: "已经了解一些的人", Description: "减少铺垫，更多讲机制、特色或比较。"},
				},
				AllowFreeform: true,
			},
			{
				ID:       "ppt_explainer_focus",
				Question: "这份介绍应该先从什么角度切入？",
				Options: []engine.PlanQuestionOption{
					{ID: "basics", Label: "先讲它是什么和怎么玩", Description: "适合入门观众，先建立最基本理解。", Recommended: true},
					{ID: "standout", Label: "先讲它为什么特别", Description: "先抓兴趣点，再补基础信息。"},
					{ID: "usage", Label: "先讲适合谁和怎么开始", Description: "适合更实用的入门型介绍。"},
				},
				AllowFreeform: true,
			},
			{
				ID:       "ppt_explainer_density",
				Question: "视觉和信息密度更适合哪种策略？",
				Options: []engine.PlanQuestionOption{
					{ID: "recommended", Label: "6-8 页、每页简洁、保留 2-3 张强相关图片", Description: "默认推荐，用简洁页面和少量强相关视觉来讲清主题。", Recommended: true},
					{ID: "text_only", Label: "6 页左右、偏正文、不要单独视觉页", Description: "适合禁图或更偏内容讲解的版本。"},
					{ID: "lighter", Label: "更轻量、文字更少、视觉更集中", Description: "适合更短更快的扫读型介绍。"},
				},
				AllowFreeform: true,
			},
		}
	}
	return []engine.PlanQuestion{
		{
			ID:       "ppt_explainer_audience",
			Question: "Who is this explainer deck mainly for?",
			Options: []engine.PlanQuestionOption{
				{ID: "beginner", Label: "People new to the topic", Description: "Start with what it is and why it matters.", Recommended: true},
				{ID: "interested", Label: "Interested but not familiar", Description: "Balance basics with standout examples."},
				{ID: "familiar", Label: "Already somewhat familiar", Description: "Spend less time on basics and more on specifics."},
			},
			AllowFreeform: true,
		},
		{
			ID:       "ppt_explainer_focus",
			Question: "What should the deck help the audience understand first?",
			Options: []engine.PlanQuestionOption{
				{ID: "basics", Label: "What it is and how it works", Description: "Best for beginner-friendly explainers.", Recommended: true},
				{ID: "standout", Label: "Why it stands out", Description: "Lead with the most memorable strengths or traits."},
				{ID: "usage", Label: "Who it suits and how to start", Description: "Lead with practical starting guidance."},
			},
			AllowFreeform: true,
		},
		{
			ID:       "ppt_explainer_density",
			Question: "How should visuals and information density be handled?",
			Options: []engine.PlanQuestionOption{
				{ID: "recommended", Label: "6-8 slides, keep each slide concise, preserve 2-3 strong related images", Description: "Recommended default for a short, visual-first explainer.", Recommended: true},
				{ID: "text_only", Label: "Around 6 slides, more text-led, no separate visual slide", Description: "Better when images are disabled or not important."},
				{ID: "lighter", Label: "Shorter copy, fewer visuals, faster scan", Description: "Best for a very light overview."},
			},
			AllowFreeform: true,
		},
	}
}

func buildChinesePPTFallbackQuestions() []engine.PlanQuestion {
	return []engine.PlanQuestion{
		{
			ID:       "ppt_audience",
			Question: "这份 PPT 主要是给谁看的？",
			Options: []engine.PlanQuestionOption{
				{ID: "beginner", Label: "第一次接触主题的人", Description: "需要先讲清背景、概念和核心信息。", Recommended: true},
				{ID: "mixed", Label: "有一定了解的混合受众", Description: "需要兼顾背景介绍和重点信息。"},
				{ID: "expert", Label: "已经比较熟悉的人", Description: "可以减少铺垫，直接进入重点。"},
			},
			AllowFreeform: true,
		},
		{
			ID:       "ppt_goal",
			Question: "这份演示最想先帮观众获得什么？",
			Options: []engine.PlanQuestionOption{
				{ID: "understand", Label: "快速理解主题", Description: "优先讲清主题、结构和关键点。", Recommended: true},
				{ID: "compare", Label: "看清差异或亮点", Description: "优先突出对比、特色和判断依据。"},
				{ID: "action", Label: "知道怎么开始或怎么用", Description: "优先给出步骤、建议或实践路径。"},
			},
			AllowFreeform: true,
		},
		{
			ID:       "ppt_shape",
			Question: "这份 PPT 更适合哪种节奏？",
			Options: []engine.PlanQuestionOption{
				{ID: "concise", Label: "6-8 页，简洁清楚", Description: "每页一个重点，信息量更克制。", Recommended: true},
				{ID: "balanced", Label: "平衡一些", Description: "兼顾背景、重点和结论。"},
				{ID: "detailed", Label: "更完整一些", Description: "允许更多背景和细节展开。"},
			},
			AllowFreeform: true,
		},
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
