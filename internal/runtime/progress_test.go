package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/officecli/officecli/engine"
)

type slowImageLLM struct {
	delay  time.Duration
	result *engine.ImageGenerationResult
	err    error
}

func (slowImageLLM) CompleteText(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (slowImageLLM) CompleteJSON(context.Context, []engine.LLMMessage) (string, error) {
	return "", nil
}

func (slowImageLLM) CompleteStructured(context.Context, engine.StructuredCompletionRequest) (string, error) {
	return "", nil
}

func (s slowImageLLM) GenerateImage(ctx context.Context, _ engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	select {
	case <-time.After(s.delay):
		return s.result, s.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type lockedProgressCollector struct {
	mu     sync.Mutex
	events []engine.ProgressEvent
}

func (c *lockedProgressCollector) Emit(_ context.Context, event engine.ProgressEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *lockedProgressCollector) snapshot() []engine.ProgressEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]engine.ProgressEvent, len(c.events))
	copy(out, c.events)
	return out
}

func withImageHeartbeatInterval(t *testing.T, d time.Duration) {
	t.Helper()
	t.Cleanup(SetImageHeartbeatIntervalForTesting(d))
}

func TestGenerateImageWithHeartbeat_EmitsPeriodicHeartbeatDuringSlowCall(t *testing.T) {
	withImageHeartbeatInterval(t, 20*time.Millisecond)
	collector := &lockedProgressCollector{}
	llm := slowImageLLM{
		delay:  220 * time.Millisecond,
		result: &engine.ImageGenerationResult{Data: []byte("ok"), MIME: "image/png"},
	}

	image, err := generateImageWithHeartbeat(context.Background(), llm, collector, engine.ImageGenerationRequest{Prompt: "a cat"}, progressStepAssemble, 1, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if image == nil || string(image.Data) != "ok" {
		t.Fatalf("unexpected image result: %+v", image)
	}

	events := collector.snapshot()
	heartbeatCount := 0
	for _, event := range events {
		if event.Step != progressStepAssemble {
			t.Fatalf("unexpected step %q", event.Step)
		}
		if event.Status != "running" {
			t.Fatalf("unexpected status %q", event.Status)
		}
		if !strings.Contains(event.Content, "Still waiting on image provider") {
			t.Fatalf("unexpected heartbeat content %q", event.Content)
		}
		if !strings.Contains(event.Content, "asset 1/3") {
			t.Fatalf("heartbeat missing asset counter %q", event.Content)
		}
		if event.ElapsedMs <= 0 {
			t.Fatalf("heartbeat should populate ElapsedMs, got %d", event.ElapsedMs)
		}
		heartbeatCount++
	}
	if heartbeatCount < 3 {
		t.Fatalf("expected >=3 heartbeat events, got %d (events=%+v)", heartbeatCount, events)
	}
}

func TestGenerateImageWithHeartbeat_StopsImmediatelyAfterReturn(t *testing.T) {
	withImageHeartbeatInterval(t, 30*time.Millisecond)
	collector := &lockedProgressCollector{}
	llm := slowImageLLM{
		delay:  5 * time.Millisecond,
		result: &engine.ImageGenerationResult{Data: []byte("ok"), MIME: "image/png"},
	}

	_, err := generateImageWithHeartbeat(context.Background(), llm, collector, engine.ImageGenerationRequest{}, progressStepAssemble, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if got := len(collector.snapshot()); got != 0 {
		t.Fatalf("expected no heartbeat after fast call, got %d events", got)
	}
}

func TestGenerateImageWithHeartbeat_PropagatesError(t *testing.T) {
	withImageHeartbeatInterval(t, 10*time.Millisecond)
	collector := &lockedProgressCollector{}
	llm := slowImageLLM{delay: 80 * time.Millisecond, err: errors.New("upstream boom")}

	_, err := generateImageWithHeartbeat(context.Background(), llm, collector, engine.ImageGenerationRequest{}, progressStepGenerateLLM, 1, 1)
	if err == nil || !strings.Contains(err.Error(), "upstream boom") {
		t.Fatalf("expected upstream error, got %v", err)
	}
	if got := len(collector.snapshot()); got < 2 {
		t.Fatalf("expected heartbeats even on failing call, got %d", got)
	}
	for _, event := range collector.snapshot() {
		if event.Step != progressStepGenerateLLM {
			t.Fatalf("expected generate_llm step, got %q", event.Step)
		}
	}
}

func TestGenerateImageWithHeartbeat_RespectsContextCancellation(t *testing.T) {
	withImageHeartbeatInterval(t, 10*time.Millisecond)
	collector := &lockedProgressCollector{}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	llm := slowImageLLM{delay: 500 * time.Millisecond, result: &engine.ImageGenerationResult{Data: []byte("ok")}}

	start := time.Now()
	_, err := generateImageWithHeartbeat(ctx, llm, collector, engine.ImageGenerationRequest{}, progressStepAssemble, 2, 4)
	if err == nil {
		t.Fatalf("expected context error")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("expected fast return after cancellation, took %s", elapsed)
	}
}

func TestGenerateImageWithHeartbeat_NilEmitterIsSilent(t *testing.T) {
	withImageHeartbeatInterval(t, 5*time.Millisecond)
	llm := slowImageLLM{delay: 30 * time.Millisecond, result: &engine.ImageGenerationResult{Data: []byte("ok")}}

	image, err := generateImageWithHeartbeat(context.Background(), llm, nil, engine.ImageGenerationRequest{}, progressStepAssemble, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if image == nil {
		t.Fatalf("expected image result")
	}
}
