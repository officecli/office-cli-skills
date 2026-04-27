package plan

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/officecli/officecli/engine"
)

type memoryPlanStore struct {
	sessions map[string]*engine.PlanSession
}

func newMemoryPlanStore() *memoryPlanStore {
	return &memoryPlanStore{sessions: make(map[string]*engine.PlanSession)}
}

func (s *memoryPlanStore) Load(_ context.Context, planID string) (*engine.PlanSession, error) {
	session, ok := s.sessions[planID]
	if !ok {
		return nil, errors.New("not found")
	}
	cloned := *session
	if session.CurrentQuestion != nil {
		current := *session.CurrentQuestion
		cloned.CurrentQuestion = &current
	}
	cloned.Questions = append([]engine.PlanQuestion(nil), session.Questions...)
	cloned.Answers = append([]engine.PlanAnswer(nil), session.Answers...)
	return &cloned, nil
}

func (s *memoryPlanStore) Save(_ context.Context, session *engine.PlanSession, _ time.Duration) error {
	cloned := *session
	if session.CurrentQuestion != nil {
		current := *session.CurrentQuestion
		cloned.CurrentQuestion = &current
	}
	cloned.Questions = append([]engine.PlanQuestion(nil), session.Questions...)
	cloned.Answers = append([]engine.PlanAnswer(nil), session.Answers...)
	s.sessions[session.PlanID] = &cloned
	return nil
}

type staticClock struct{}

func (staticClock) Now() time.Time { return time.Unix(1700000000, 0) }

type staticIDs struct{ next int }

func (g *staticIDs) NewID() string {
	g.next++
	return "plan-test-id"
}

type fakeLLMClient struct {
	structuredResponses []string
	structuredErrors    []error
	jsonResponses       []string
	jsonErrors          []error
	structuredCalls     int
	jsonCalls           int
	lastStructuredReq   engine.StructuredCompletionRequest
	lastStructuredMsgs  []engine.LLMMessage
	lastJSONMsgs        []engine.LLMMessage
}

func (f *fakeLLMClient) CompleteText(_ context.Context, _ []engine.LLMMessage) (string, error) {
	return "", errors.New("unused")
}

func (f *fakeLLMClient) CompleteJSON(_ context.Context, msgs []engine.LLMMessage) (string, error) {
	f.jsonCalls++
	f.lastJSONMsgs = append([]engine.LLMMessage(nil), msgs...)
	if len(f.jsonErrors) >= f.jsonCalls && f.jsonErrors[f.jsonCalls-1] != nil {
		return "", f.jsonErrors[f.jsonCalls-1]
	}
	if len(f.jsonResponses) >= f.jsonCalls {
		return f.jsonResponses[f.jsonCalls-1], nil
	}
	return "", errors.New("missing json response")
}

func (f *fakeLLMClient) CompleteStructured(_ context.Context, req engine.StructuredCompletionRequest) (string, error) {
	f.structuredCalls++
	f.lastStructuredReq = req
	f.lastStructuredMsgs = append([]engine.LLMMessage(nil), req.Messages...)
	if len(f.structuredErrors) >= f.structuredCalls && f.structuredErrors[f.structuredCalls-1] != nil {
		return "", f.structuredErrors[f.structuredCalls-1]
	}
	if len(f.structuredResponses) >= f.structuredCalls {
		return f.structuredResponses[f.structuredCalls-1], nil
	}
	return "", errors.New("missing structured response")
}

func (f *fakeLLMClient) GenerateImage(_ context.Context, _ engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	return nil, errors.New("unused")
}

func newWorkflowForTest(client engine.LLMClient) *Workflow {
	return NewWorkflow(Options{
		PlanStore:   newMemoryPlanStore(),
		LLMClient:   client,
		Clock:       staticClock{},
		IDGenerator: &staticIDs{},
	})
}

func TestPrepareExecutionPlan_BestStartsQuestioning(t *testing.T) {
	client := &fakeLLMClient{
		structuredResponses: []string{`{"questions":[{"id":"audience","question":"Who is the main audience for this deck?","allowFreeform":true,"options":[{"id":"management","label":"Leadership","description":"Emphasize conclusions and judgment.","recommended":true},{"id":"team","label":"Internal team","description":"Emphasize execution detail."}]}]}`},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "Create a PPT about Minecraft",
		DocumentType:   "pptx",
		RequestID:      "req-1",
		GenerationMode: "best",
	})
	if err != nil {
		t.Fatalf("PrepareExecutionPlan error: %v", err)
	}
	if session.Status != "questioning" {
		t.Fatalf("status = %q, want questioning", session.Status)
	}
	if session.QuestionSource != "llm_dynamic" {
		t.Fatalf("question source = %q, want llm_dynamic", session.QuestionSource)
	}
	if session.CurrentQuestion == nil || session.CurrentQuestion.ID != "audience" {
		t.Fatalf("current question = %#v", session.CurrentQuestion)
	}
}

func TestPrepareExecutionPlan_FastSkipsQuestions(t *testing.T) {
	workflow := newWorkflowForTest(nil)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "Create a PPT about Minecraft",
		DocumentType:   "pptx",
		RequestID:      "req-fast",
		GenerationMode: "fast",
	})
	if err != nil {
		t.Fatalf("PrepareExecutionPlan error: %v", err)
	}
	if session.Status != "approved" {
		t.Fatalf("status = %q, want approved", session.Status)
	}
	if session.QuestionSource != "fast_path" {
		t.Fatalf("question source = %q, want fast_path", session.QuestionSource)
	}
	if session.ExecutionPrompt == "" {
		t.Fatal("expected execution prompt")
	}
}

func TestPrepareExecutionPlan_FallsBackToPromptJSONWhenStructuredFails(t *testing.T) {
	client := &fakeLLMClient{
		structuredErrors: []error{
			errors.New("openai: invalid structured json response"),
			errors.New("openai: invalid structured json response"),
		},
		jsonResponses: []string{`{"questions":[{"id":"audience","question":"Who is the main audience for this deck?","allowFreeform":true,"options":[{"label":"Leadership","description":"Emphasize conclusions.","recommended":true},{"label":"Internal team","description":"Emphasize execution detail."}]}]}`},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "Create a project update PPT",
		DocumentType:   "pptx",
		RequestID:      "req-fallback",
		GenerationMode: "best",
	})
	if err != nil {
		t.Fatalf("PrepareExecutionPlan error: %v", err)
	}
	if client.structuredCalls != 2 {
		t.Fatalf("structured calls = %d, want 2", client.structuredCalls)
	}
	if client.jsonCalls != 1 {
		t.Fatalf("json calls = %d, want 1", client.jsonCalls)
	}
	if session.QuestionSource != "llm_dynamic" {
		t.Fatalf("question source = %q, want llm_dynamic", session.QuestionSource)
	}
	if session.CurrentQuestion == nil || session.CurrentQuestion.ID != "audience" {
		t.Fatalf("current question = %#v", session.CurrentQuestion)
	}
}

func TestPrepareExecutionPlan_FallsBackToTemplateQuestionsWhenLLMGenerationFails(t *testing.T) {
	client := &fakeLLMClient{
		structuredErrors: []error{
			errors.New("upstream unavailable"),
			errors.New("upstream unavailable"),
		},
		jsonErrors: []error{errors.New("json unavailable")},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "Create a quarterly project review deck",
		DocumentType:   "pptx",
		RequestID:      "req-template-fallback",
		GenerationMode: "best",
	})
	if err != nil {
		t.Fatalf("PrepareExecutionPlan error: %v", err)
	}
	if session.QuestionSource != "template_fallback" {
		t.Fatalf("question source = %q, want template_fallback", session.QuestionSource)
	}
	if session.QuestionErrorKind != "llm_request_failed" {
		t.Fatalf("question error kind = %q, want llm_request_failed", session.QuestionErrorKind)
	}
	if session.QuestionFallbackReason == "" {
		t.Fatal("expected question fallback reason")
	}
	if session.CurrentQuestion == nil || session.CurrentQuestion.ID != "ppt_report_audience" {
		t.Fatalf("current question = %#v", session.CurrentQuestion)
	}
}

func TestAnswerExecutionPlanQuestion_BestGeneratesPlanAndBlueprint(t *testing.T) {
	client := &fakeLLMClient{
		structuredResponses: []string{
			`{"questions":[{"id":"audience","question":"Who is the main audience for this deck?","allowFreeform":true,"options":[{"id":"management","label":"Leadership","description":"Emphasize conclusions and judgment.","recommended":true},{"id":"team","label":"Internal team","description":"Emphasize execution detail."}]},{"id":"shape","question":"How should the deck be structured?","allowFreeform":true,"options":[{"id":"concise","label":"Conclusion-first","description":"Fits a short deck.","recommended":true},{"id":"detailed","label":"Expanded","description":"Fits a more detailed explanation."}]}]}`,
			`{"plan_markdown":"# Execution Plan\n\n## Summary\n- Conclusion-first for leadership.","execution_prompt":"Generate the PPT in 6 slides or fewer, for leadership, with a conclusion-first structure."}`,
		},
		jsonResponses: []string{
			`{"presentationType":"Overview deck","targetAudience":"Leadership","presentationPurpose":"Introduce Minecraft","pageCount":6,"contentStyle":"Conclusion-first","visualEffect":"Clean and credible","slideOutline":[{"slideIndex":1,"purpose":"Cover","contentFormat":"paragraph","suggestedLayout":"title","maxItems":1,"contentRequirements":"State the topic and audience","visualSuggestion":"hero"}],"contentGuideline":"Keep one core point per slide"}`,
		},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "Create a PPT about Minecraft",
		DocumentType:   "pptx",
		RequestID:      "req-prepare",
		GenerationMode: "best",
	})
	if err != nil {
		t.Fatalf("PrepareExecutionPlan error: %v", err)
	}

	session, err = workflow.AnswerExecutionPlanQuestion(context.Background(), engine.AnswerExecutionPlanQuestionRequest{
		PlanID:     session.PlanID,
		QuestionID: session.CurrentQuestion.ID,
		OptionID:   session.CurrentQuestion.Options[0].ID,
		RequestID:  "req-answer-1",
	})
	if err != nil {
		t.Fatalf("AnswerExecutionPlanQuestion first error: %v", err)
	}

	session, err = workflow.AnswerExecutionPlanQuestion(context.Background(), engine.AnswerExecutionPlanQuestionRequest{
		PlanID:     session.PlanID,
		QuestionID: session.CurrentQuestion.ID,
		Answer:     "Keep it within 6 slides and lead with conclusions",
		RequestID:  "req-answer-2",
	})
	if err != nil {
		t.Fatalf("AnswerExecutionPlanQuestion second error: %v", err)
	}

	if session.Status != "review_pending" {
		t.Fatalf("status = %q, want review_pending", session.Status)
	}
	if !strings.Contains(session.PlanMarkdown, "## Framework Blueprint") {
		t.Fatalf("plan markdown = %q, want framework blueprint section", session.PlanMarkdown)
	}
	if !strings.Contains(session.FrameworkBlueprint, "Slide 1") {
		t.Fatalf("framework blueprint = %q", session.FrameworkBlueprint)
	}
	if session.ExecutionPrompt != "Generate the PPT in 6 slides or fewer, for leadership, with a conclusion-first structure." {
		t.Fatalf("execution prompt = %q", session.ExecutionPrompt)
	}
}

func TestReviseExecutionPlan_RebuildsArtifacts(t *testing.T) {
	client := &fakeLLMClient{
		structuredResponses: []string{
			`{"questions":[{"id":"audience","question":"Who is the main audience for this deck?","allowFreeform":true,"options":[{"id":"management","label":"Leadership","description":"Emphasize conclusions and judgment.","recommended":true},{"id":"team","label":"Internal team","description":"Emphasize execution detail."}]}]}`,
			`{"plan_markdown":"# Execution Plan\n\n## Summary\n- First draft plan.","execution_prompt":"First execution prompt."}`,
			`{"plan_markdown":"# Execution Plan\n\n## Summary\n- Second draft plan with a risk slide.","execution_prompt":"Second execution prompt with a risk slide."}`,
		},
		jsonResponses: []string{
			`{"presentationType":"Overview deck","targetAudience":"Leadership","presentationPurpose":"Introduce Minecraft","pageCount":6,"contentStyle":"Conclusion-first","slideOutline":[{"slideIndex":1,"purpose":"Cover","contentFormat":"paragraph"}]}`,
			`{"presentationType":"Overview deck","targetAudience":"Leadership","presentationPurpose":"Introduce Minecraft","pageCount":7,"contentStyle":"Conclusion-first","slideOutline":[{"slideIndex":1,"purpose":"Cover","contentFormat":"paragraph"},{"slideIndex":2,"purpose":"Risk","contentFormat":"points"}]}`,
		},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "Create a PPT about Minecraft",
		DocumentType:   "pptx",
		RequestID:      "req-prepare",
		GenerationMode: "best",
	})
	if err != nil {
		t.Fatalf("PrepareExecutionPlan error: %v", err)
	}
	session, err = workflow.AnswerExecutionPlanQuestion(context.Background(), engine.AnswerExecutionPlanQuestionRequest{
		PlanID:     session.PlanID,
		QuestionID: session.CurrentQuestion.ID,
		OptionID:   session.CurrentQuestion.Options[0].ID,
		RequestID:  "req-answer",
	})
	if err != nil {
		t.Fatalf("AnswerExecutionPlanQuestion error: %v", err)
	}
	session, err = workflow.ReviseExecutionPlan(context.Background(), engine.ReviseExecutionPlanRequest{
		PlanID:      session.PlanID,
		Instruction: "Add one slide for risks and mitigations",
		RequestID:   "req-revise",
	})
	if err != nil {
		t.Fatalf("ReviseExecutionPlan error: %v", err)
	}
	if !strings.Contains(session.PlanMarkdown, "Second draft plan") {
		t.Fatalf("plan markdown = %q", session.PlanMarkdown)
	}
	if !strings.Contains(session.ExecutionPrompt, "risk slide") {
		t.Fatalf("execution prompt = %q", session.ExecutionPrompt)
	}
}

func TestApproveExecutionPlan_UsesExistingArtifacts(t *testing.T) {
	client := &fakeLLMClient{
		structuredResponses: []string{
			`{"questions":[{"id":"audience","question":"Who is the main audience for this deck?","allowFreeform":true,"options":[{"id":"management","label":"Leadership","description":"Emphasize conclusions and judgment.","recommended":true},{"id":"team","label":"Internal team","description":"Emphasize execution detail."}]}]}`,
			`{"plan_markdown":"# Execution Plan\n\n## Summary\n- First draft plan.","execution_prompt":"First execution prompt."}`,
		},
		jsonResponses: []string{
			`{"presentationType":"Overview deck","targetAudience":"Leadership","presentationPurpose":"Introduce Minecraft","pageCount":6,"contentStyle":"Conclusion-first","slideOutline":[{"slideIndex":1,"purpose":"Cover","contentFormat":"paragraph"}]}`,
		},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "Create a PPT about Minecraft",
		DocumentType:   "pptx",
		RequestID:      "req-prepare",
		GenerationMode: "best",
	})
	if err != nil {
		t.Fatalf("PrepareExecutionPlan error: %v", err)
	}
	session, err = workflow.AnswerExecutionPlanQuestion(context.Background(), engine.AnswerExecutionPlanQuestionRequest{
		PlanID:     session.PlanID,
		QuestionID: session.CurrentQuestion.ID,
		OptionID:   session.CurrentQuestion.Options[0].ID,
		RequestID:  "req-answer",
	})
	if err != nil {
		t.Fatalf("AnswerExecutionPlanQuestion error: %v", err)
	}
	session, err = workflow.ApproveExecutionPlan(context.Background(), engine.ApproveExecutionPlanRequest{
		PlanID:    session.PlanID,
		RequestID: "req-approve",
	})
	if err != nil {
		t.Fatalf("ApproveExecutionPlan error: %v", err)
	}
	if session.Status != "approved" {
		t.Fatalf("status = %q, want approved", session.Status)
	}
	if session.ExecutionPrompt == "" {
		t.Fatal("expected execution prompt")
	}
}

func TestBuildExecutionPlanSynthesisMessages_ExplainerAddsScenarioRequirements(t *testing.T) {
	session := &engine.PlanSession{
		DocumentType: "pptx",
		UserPrompt:   "介绍 minecraft 这款游戏",
		Answers: []engine.PlanAnswer{
			{QuestionID: "ppt_explainer_audience", Answer: "第一次接触的人"},
			{QuestionID: "ppt_explainer_focus", Answer: "先讲它是什么和怎么玩"},
		},
	}
	msgs := buildExecutionPlanSynthesisMessages(session)
	if len(msgs) != 2 {
		t.Fatalf("message count = %d, want 2", len(msgs))
	}
	userMsg := msgs[1].Content
	for _, needle := range []string{
		"audience familiarity level",
		"cover hero visual allowance",
		"preserve complete visible wording",
		"beginner tips, who it suits, or how to start",
	} {
		if !strings.Contains(userMsg, needle) {
			t.Fatalf("user synthesis prompt missing %q:\n%s", needle, userMsg)
		}
	}
}
