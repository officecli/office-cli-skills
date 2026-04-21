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
	reportQuestions := buildDynamicFallbackQuestions(engine.PrepareExecutionPlanRequest{UserPrompt: "Prepare a quarterly project review deck"}, "pptx")
	if len(reportQuestions) < 3 {
		t.Fatalf("report question count = %d, want at least 3", len(reportQuestions))
	}
	if !strings.Contains(reportQuestions[1].Question, "emphasize") {
		t.Fatalf("question = %q, want report-specific focus", reportQuestions[1].Question)
	}

	pitchQuestions := buildDynamicFallbackQuestions(engine.PrepareExecutionPlanRequest{UserPrompt: "Build a fundraising roadshow deck for an AI startup"}, "pptx")
	if !strings.Contains(pitchQuestions[0].Question, "audience") {
		t.Fatalf("question = %q, want pitch-specific audience", pitchQuestions[0].Question)
	}
}

func TestBuildQuestionContext_PPTXExplainerFollowsUserLanguage(t *testing.T) {
	context := buildQuestionContext(engine.PrepareExecutionPlanRequest{
		UserPrompt:     "介绍 minecraft 这款游戏",
		GenerationMode: "best",
	}, "pptx")
	if !strings.Contains(context, "Write all question text in Simplified Chinese.") {
		t.Fatalf("context = %q, want Simplified Chinese requirement", context)
	}
	if !strings.Contains(context, "audience familiarity") {
		t.Fatalf("context = %q, want explainer-oriented goal", context)
	}
}

func TestBuildDynamicFallbackQuestions_PPTXExplainerChineseQuestions(t *testing.T) {
	questions := buildDynamicFallbackQuestions(engine.PrepareExecutionPlanRequest{UserPrompt: "介绍 minecraft 这款游戏"}, "pptx")
	if len(questions) != 3 {
		t.Fatalf("question count = %d, want 3", len(questions))
	}
	if !strings.Contains(questions[0].Question, "主要是讲给谁看的") {
		t.Fatalf("question = %q, want Chinese explainer audience question", questions[0].Question)
	}
	if !strings.Contains(questions[1].Question, "应该先突出什么") {
		t.Fatalf("question = %q, want Chinese explainer focus question", questions[1].Question)
	}
}
