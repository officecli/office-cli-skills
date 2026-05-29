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
			`{"questions":[{"id":"doc_goal","question":"What is the most important purpose of this document?","allowFreeform":true,"options":[{"id":"report","label":"Analytical report","description":"Emphasize conclusions and analysis.","recommended":true},{"id":"proposal","label":"Proposal","description":"Emphasize recommendations and action."}]}]}`,
			`{"plan_markdown":"# Execution Plan\n\n## Objective\n- Produce a formal analytical report.","execution_prompt":"Generate the document as a formal analytical report. Lead with conclusions before analysis and avoid filler."}`,
		},
		jsonResponses: []string{
			`{"documentType":"Analytical report","targetAudience":"Leadership","writingGoal":"Explain market changes and recommend actions","tone":"Formal and professional","lengthHint":"Around 3000 words","contentGuideline":"Lead with conclusions before analysis","sections":[{"sectionIndex":1,"heading":"Executive summary","purpose":"Present the core conclusion first","keyPoints":["Conclusion","Recommendation"],"lengthHint":"300 words"}]}`,
		},
	}
	workflow := newWorkflowForTest(client)

	session, err := workflow.PrepareExecutionPlan(context.Background(), engine.PrepareExecutionPlanRequest{
		ConversationID: "conv-docx",
		UserPrompt:     "Write an AI industry analysis report",
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
	if !strings.Contains(session.FrameworkBlueprint, "### Section Plan") {
		t.Fatalf("framework blueprint = %q", session.FrameworkBlueprint)
	}
	if !strings.Contains(session.ExecutionPrompt, "avoid filler") {
		t.Fatalf("execution prompt = %q", session.ExecutionPrompt)
	}
}

func TestBuildExecutionPrompt_IncludesBlueprintAndTypeSpecificConstraints(t *testing.T) {
	session := &engine.PlanSession{
		DocumentType:       "xlsx",
		UserPrompt:         "Build a sales business-analysis workbook",
		GenerationMode:     "best",
		Answers:            []engine.PlanAnswer{{QuestionID: "focus", Answer: "Track revenue and budget variance monthly", Source: "freeform"}},
		FrameworkBlueprint: "## Framework Blueprint\n\n### Workbook Plan\n- Summary sheet leads with an executive summary",
	}
	prompt := buildExecutionPrompt(session)
	for _, needle := range []string{"Original request: Build a sales business-analysis workbook", "Track revenue and budget variance monthly", "Summary sheet leads with an executive summary", "metric definitions", "Keep fields and metric definitions consistent"} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("prompt missing %q: %s", needle, prompt)
		}
	}
}
