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
		structuredResponses: []string{`{"questions":[{"id":"audience","question":"这份内容主要给谁看？","allowFreeform":true,"options":[{"id":"management","label":"管理层","description":"突出结论与判断。","recommended":true},{"id":"team","label":"团队内部","description":"突出执行细节。"}]}]}`},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "做一个关于 minecraft 的 ppt",
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
		UserPrompt:     "做一个关于 minecraft 的 ppt",
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
		jsonResponses: []string{`{"questions":[{"id":"audience","question":"这份内容主要给谁看？","allowFreeform":true,"options":[{"label":"管理层","description":"突出结论。","recommended":true},{"label":"团队内部","description":"突出执行细节。"}]}]}`},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "制作一份项目汇报 PPT",
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
	if session.CurrentQuestion == nil || session.CurrentQuestion.ID != "audience" {
		t.Fatalf("current question = %#v", session.CurrentQuestion)
	}
}

func TestAnswerExecutionPlanQuestion_BestGeneratesPlanAndBlueprint(t *testing.T) {
	client := &fakeLLMClient{
		structuredResponses: []string{
			`{"questions":[{"id":"audience","question":"这份内容主要给谁看？","allowFreeform":true,"options":[{"id":"management","label":"管理层","description":"突出结论与判断。","recommended":true},{"id":"team","label":"团队内部","description":"突出执行细节。"}]},{"id":"shape","question":"你更希望怎么组织？","allowFreeform":true,"options":[{"id":"concise","label":"结论先行","description":"适合短汇报。","recommended":true},{"id":"detailed","label":"完整展开","description":"适合详细介绍。"}]}]}`,
			`{"plan_markdown":"# 执行计划\n\n## Summary\n- 面向管理层，结论先行。","execution_prompt":"按照 6 页以内、面向管理层、结论先行的方式生成 PPT。"}`,
		},
		jsonResponses: []string{
			`{"presentationType":"介绍型","targetAudience":"管理层","presentationPurpose":"介绍 Minecraft","pageCount":6,"contentStyle":"结论先行","visualEffect":"简洁可信","slideOutline":[{"slideIndex":1,"purpose":"封面","contentFormat":"paragraph","suggestedLayout":"title","maxItems":1,"contentRequirements":"说明主题与对象","visualSuggestion":"hero"}],"contentGuideline":"每页只保留一个核心信息点"}`,
		},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "做一个关于 minecraft 的 ppt",
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
		Answer:     "控制在 6 页以内，先讲结论",
		RequestID:  "req-answer-2",
	})
	if err != nil {
		t.Fatalf("AnswerExecutionPlanQuestion second error: %v", err)
	}

	if session.Status != "review_pending" {
		t.Fatalf("status = %q, want review_pending", session.Status)
	}
	if !strings.Contains(session.PlanMarkdown, "## 框架蓝图") {
		t.Fatalf("plan markdown = %q, want framework blueprint section", session.PlanMarkdown)
	}
	if !strings.Contains(session.FrameworkBlueprint, "第 1 页") {
		t.Fatalf("framework blueprint = %q", session.FrameworkBlueprint)
	}
	if session.ExecutionPrompt != "按照 6 页以内、面向管理层、结论先行的方式生成 PPT。" {
		t.Fatalf("execution prompt = %q", session.ExecutionPrompt)
	}
}

func TestReviseExecutionPlan_RebuildsArtifacts(t *testing.T) {
	client := &fakeLLMClient{
		structuredResponses: []string{
			`{"questions":[{"id":"audience","question":"这份内容主要给谁看？","allowFreeform":true,"options":[{"id":"management","label":"管理层","description":"突出结论与判断。","recommended":true},{"id":"team","label":"团队内部","description":"突出执行细节。"}]}]}`,
			`{"plan_markdown":"# 执行计划\n\n## Summary\n- 第一版计划。","execution_prompt":"第一版执行提示。"}`,
			`{"plan_markdown":"# 执行计划\n\n## Summary\n- 第二版计划，加入风险页。","execution_prompt":"第二版执行提示，加入风险页。"}`,
		},
		jsonResponses: []string{
			`{"presentationType":"介绍型","targetAudience":"管理层","presentationPurpose":"介绍 Minecraft","pageCount":6,"contentStyle":"结论先行","slideOutline":[{"slideIndex":1,"purpose":"封面","contentFormat":"paragraph"}]}`,
			`{"presentationType":"介绍型","targetAudience":"管理层","presentationPurpose":"介绍 Minecraft","pageCount":7,"contentStyle":"结论先行","slideOutline":[{"slideIndex":1,"purpose":"封面","contentFormat":"paragraph"},{"slideIndex":2,"purpose":"风险","contentFormat":"points"}]}`,
		},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "做一个关于 minecraft 的 ppt",
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
		Instruction: "增加一页风险与应对",
		RequestID:   "req-revise",
	})
	if err != nil {
		t.Fatalf("ReviseExecutionPlan error: %v", err)
	}
	if !strings.Contains(session.PlanMarkdown, "第二版计划") {
		t.Fatalf("plan markdown = %q", session.PlanMarkdown)
	}
	if !strings.Contains(session.ExecutionPrompt, "风险页") {
		t.Fatalf("execution prompt = %q", session.ExecutionPrompt)
	}
}

func TestApproveExecutionPlan_UsesExistingArtifacts(t *testing.T) {
	client := &fakeLLMClient{
		structuredResponses: []string{
			`{"questions":[{"id":"audience","question":"这份内容主要给谁看？","allowFreeform":true,"options":[{"id":"management","label":"管理层","description":"突出结论与判断。","recommended":true},{"id":"team","label":"团队内部","description":"突出执行细节。"}]}]}`,
			`{"plan_markdown":"# 执行计划\n\n## Summary\n- 第一版计划。","execution_prompt":"第一版执行提示。"}`,
		},
		jsonResponses: []string{
			`{"presentationType":"介绍型","targetAudience":"管理层","presentationPurpose":"介绍 Minecraft","pageCount":6,"contentStyle":"结论先行","slideOutline":[{"slideIndex":1,"purpose":"封面","contentFormat":"paragraph"}]}`,
		},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-1",
		UserPrompt:     "做一个关于 minecraft 的 ppt",
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
