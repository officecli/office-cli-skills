package plan

import (
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
)

func TestBuildExecutionPlanQuestions_PPTXReturnsThreeHighValueQuestions(t *testing.T) {
	questions := buildExecutionPlanQuestions("pptx")
	if len(questions) != 3 {
		t.Fatalf("question count = %d, want 3", len(questions))
	}
	for _, question := range questions {
		if !question.AllowFreeform {
			t.Fatalf("question %s should allow freeform", question.ID)
		}
		if len(question.Options) < 2 || len(question.Options) > 4 {
			t.Fatalf("question %s option count = %d", question.ID, len(question.Options))
		}
		recommended := 0
		for _, option := range question.Options {
			if option.Recommended {
				recommended++
			}
			if strings.TrimSpace(option.Label) == "" || strings.TrimSpace(option.Description) == "" {
				t.Fatalf("question %s has empty option fields", question.ID)
			}
		}
		if recommended != 1 {
			t.Fatalf("question %s recommended count = %d, want 1", question.ID, recommended)
		}
	}
}

func TestBuildExecutionPlanQuestions_DOCXAndXLSXReturnThreeQuestions(t *testing.T) {
	for _, documentType := range []string{"docx", "xlsx"} {
		questions := buildExecutionPlanQuestions(documentType)
		if len(questions) != 3 {
			t.Fatalf("document type %s question count = %d, want 3", documentType, len(questions))
		}
	}
}

func TestBuildDynamicFallbackQuestions_UsesScenarioSpecificQuestions(t *testing.T) {
	reportQuestions := buildDynamicFallbackQuestions(engine.PrepareExecutionPlanRequest{UserPrompt: "做一份季度项目复盘汇报"}, "pptx")
	if len(reportQuestions) < 3 {
		t.Fatalf("report question count = %d, want at least 3", len(reportQuestions))
	}
	if !strings.Contains(reportQuestions[1].Question, "突出") {
		t.Fatalf("question = %q, want report-specific focus", reportQuestions[1].Question)
	}

	pitchQuestions := buildDynamicFallbackQuestions(engine.PrepareExecutionPlanRequest{UserPrompt: "做一份 AI 公司融资路演材料"}, "pptx")
	if !strings.Contains(pitchQuestions[0].Question, "哪类听众") {
		t.Fatalf("question = %q, want pitch-specific audience", pitchQuestions[0].Question)
	}
}
