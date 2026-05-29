package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
)

type promptPrepLLMClient struct {
	structuredResponse string
	structuredCalls    int
}

func (f *promptPrepLLMClient) CompleteText(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (f *promptPrepLLMClient) CompleteJSON(context.Context, []engine.LLMMessage) (string, error) {
	return f.structuredResponse, nil
}

func (f *promptPrepLLMClient) CompleteStructured(_ context.Context, _ engine.StructuredCompletionRequest) (string, error) {
	f.structuredCalls++
	return f.structuredResponse, nil
}

func (f *promptPrepLLMClient) GenerateImage(context.Context, engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	return nil, nil
}

func TestPreparePPTPrompt_EnrichesFastExplainerEnvelopeAndClearsDefaultStyle(t *testing.T) {
	app := &App{}
	llm := &promptPrepLLMClient{
		structuredResponse: `{"prompt":"做一个 7 页左右的 minecraft 游戏介绍 PPT，面向第一次接触的人，直接进入 What It Is / Core Ways to Play / Why It Stands Out / Example / Who It Suits / How to Start。默认采用 editorial-light 的轻编辑风格，封面允许 hero 图，总图量控制在 2-3 张强相关图片，每页简洁，完整保留用户可见文案，必要时通过 reflow 或拆页解决拥挤，不使用目录或章节页。","assumptions":["默认受众是第一次接触的人"]}`,
	}
	job := GenerateJob{
		DocumentType:   engine.DocumentTypePPTX,
		Topic:          "minecraft 游戏介绍",
		Prompt:         `{"prompt":"介绍 minecraft 这款游戏","target":{"language":"zh-CN"}}`,
		OriginalPrompt: `{"prompt":"介绍 minecraft 这款游戏","target":{"language":"zh-CN"}}`,
		Mode:           generateengine.ModeFast,
		Style:          "tech-contrast",
		StyleSpecified: false,
		EnableImages:   true,
	}

	updated, err := app.preparePPTPrompt(context.Background(), llm, job, nil)
	if err != nil {
		t.Fatalf("preparePPTPrompt: %v", err)
	}
	envelope, _, err := generateengine.ParsePromptEnvelope(updated.Prompt)
	if err != nil {
		t.Fatalf("ParsePromptEnvelope: %v", err)
	}
	if envelope.Target.Language != "zh-CN" {
		t.Fatalf("language = %q, want zh-CN", envelope.Target.Language)
	}
	for _, needle := range []string{"每页简洁", "完整保留用户可见文案", "不使用目录或章节页"} {
		if !strings.Contains(envelope.Prompt, needle) {
			t.Fatalf("enriched prompt missing %q:\n%s", needle, envelope.Prompt)
		}
	}
	if updated.Style != "" {
		t.Fatalf("style = %q, want empty so explainer decks can fall back to editorial-light", updated.Style)
	}
	if len(updated.Warnings) == 0 || updated.Warnings[0].Code != "WARN_PROMPT_ENRICHED" {
		t.Fatalf("warnings = %#v", updated.Warnings)
	}
}

func TestPreparePPTPrompt_SkipsBestModeRewrite(t *testing.T) {
	app := &App{}
	llm := &promptPrepLLMClient{
		structuredResponse: `{"prompt":"should not be used","assumptions":[]}`,
	}
	job := GenerateJob{
		DocumentType: engine.DocumentTypePPTX,
		Topic:        "minecraft 游戏介绍",
		Prompt:       "介绍 minecraft 这款游戏",
		Mode:         generateengine.ModeBest,
	}

	updated, err := app.preparePPTPrompt(context.Background(), llm, job, nil)
	if err != nil {
		t.Fatalf("preparePPTPrompt: %v", err)
	}
	if updated.Prompt != job.Prompt {
		t.Fatalf("prompt = %q, want unchanged", updated.Prompt)
	}
	if llm.structuredCalls != 0 {
		t.Fatalf("structuredCalls = %d, want 0", llm.structuredCalls)
	}
}
