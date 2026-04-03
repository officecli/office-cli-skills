package plan

import (
	"context"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
)

func TestAnswerExecutionPlanQuestion_DOCXBuildsBlueprintAndExecutionPrompt(t *testing.T) {
	client := &fakeLLMClient{
		structuredResponses: []string{
			`{"questions":[{"id":"doc_goal","question":"这份文档最重要的用途是什么？","allowFreeform":true,"options":[{"id":"report","label":"分析报告","description":"突出结论与分析。","recommended":true},{"id":"proposal","label":"方案文档","description":"突出建议与行动。"}]}]}`,
			`{"plan_markdown":"# 执行计划\n\n## 目标理解\n- 形成正式分析报告。","execution_prompt":"按照正式分析报告结构生成文档，先结论后分析，避免空话。"}`,
		},
		jsonResponses: []string{
			`{"documentType":"分析报告","targetAudience":"管理层","writingGoal":"说明市场变化并提出建议","tone":"正式专业","lengthHint":"约 3000 字","contentGuideline":"先结论后分析","sections":[{"sectionIndex":1,"heading":"摘要","purpose":"先给核心结论","keyPoints":["结论","建议"],"lengthHint":"300 字"}]}`,
		},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-docx",
		UserPrompt:     "写一份 AI 行业分析报告",
		DocumentType:   "docx",
		RequestID:      "req-docx-prepare",
		GenerationMode: "best",
	})
	if err != nil {
		t.Fatalf("PrepareExecutionPlan error: %v", err)
	}
	if session.CurrentQuestion == nil {
		t.Fatal("expected current question")
	}

	session, err = workflow.AnswerExecutionPlanQuestion(context.Background(), engine.AnswerExecutionPlanQuestionRequest{
		PlanID:     session.PlanID,
		QuestionID: session.CurrentQuestion.ID,
		OptionID:   session.CurrentQuestion.Options[0].ID,
		RequestID:  "req-docx-answer",
	})
	if err != nil {
		t.Fatalf("AnswerExecutionPlanQuestion error: %v", err)
	}
	if !strings.Contains(session.FrameworkBlueprint, "### 章节规划") {
		t.Fatalf("framework blueprint = %q", session.FrameworkBlueprint)
	}
	if !strings.Contains(session.ExecutionPrompt, "避免空话") {
		t.Fatalf("execution prompt = %q", session.ExecutionPrompt)
	}
}

func TestBuildExecutionPrompt_IncludesBlueprintAndTypeSpecificConstraints(t *testing.T) {
	session := &engine.PlanSession{
		DocumentType:       "xlsx",
		UserPrompt:         "做一份销售经营分析表",
		GenerationMode:     "best",
		Answers:            []engine.PlanAnswer{{QuestionID: "focus", Answer: "按月跟踪收入与预算偏差", Source: "freeform"}},
		FrameworkBlueprint: "## 框架蓝图\n\n### 工作簿规划\n- Summary sheet 先给摘要",
	}
	prompt := buildExecutionPrompt(session)
	for _, needle := range []string{"原始需求：做一份销售经营分析表", "按月跟踪收入与预算偏差", "Summary sheet 先给摘要", "字段一致性", "指标口径"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}
