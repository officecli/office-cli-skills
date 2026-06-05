package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/officecli/officecli/engine"
)

const (
	progressStepGenerateLLM = "generate_llm"
	progressStepAssemble    = "assemble"
)

// imageHeartbeatInterval controls how often the image-generation heartbeat
// emits a "Still waiting on image provider" task.progress event. It is a
// package-level variable so tests can shorten it; production keeps the
// 25s default so downstream stall detectors (which are tuned for ≥60s
// silence) always see fresh activity.
var imageHeartbeatInterval = 25 * time.Second
var structuredLLMHeartbeatInterval = 25 * time.Second

// SetImageHeartbeatIntervalForTesting overrides the image generation
// heartbeat cadence and returns a restore function. It is exported so
// integration tests in sibling packages (e.g. internal/cli agent bridge
// tests) can drive the heartbeat without waiting for the production 25s
// cadence. Production code MUST NOT call this.
func SetImageHeartbeatIntervalForTesting(d time.Duration) (restore func()) {
	previous := imageHeartbeatInterval
	imageHeartbeatInterval = d
	return func() { imageHeartbeatInterval = previous }
}

// SetStructuredLLMHeartbeatIntervalForTesting overrides the structured LLM
// heartbeat cadence for tests. Production keeps the 25s default.
func SetStructuredLLMHeartbeatIntervalForTesting(d time.Duration) (restore func()) {
	previous := structuredLLMHeartbeatInterval
	structuredLLMHeartbeatInterval = d
	return func() { structuredLLMHeartbeatInterval = previous }
}

func emitProgress(ctx context.Context, emitter engine.ProgressEmitter, step, status, content string) {
	if emitter == nil {
		return
	}
	emitter.Emit(ctx, engine.ProgressEvent{Step: step, Status: status, Content: strings.TrimSpace(content)})
}

func completeStructuredWithHeartbeat(
	ctx context.Context,
	llm engine.LLMClient,
	emitter engine.ProgressEmitter,
	req engine.StructuredCompletionRequest,
	step string,
	label string,
) (string, error) {
	if llm == nil {
		return "", fmt.Errorf("llm client is nil")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "structured LLM response"
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	if emitter != nil && structuredLLMHeartbeatInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			ticker := time.NewTicker(structuredLLMHeartbeatInterval)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					elapsed := time.Since(start)
					emitter.Emit(ctx, engine.ProgressEvent{
						Step:      step,
						Status:    "running",
						Content:   fmt.Sprintf("Still waiting on %s (elapsed %ds)", label, int(elapsed.Seconds())),
						ElapsedMs: elapsed.Milliseconds(),
					})
				}
			}
		}()
	}
	response, err := llm.CompleteStructured(ctx, req)
	close(done)
	wg.Wait()
	return response, err
}

// generateImageWithHeartbeat invokes llm.GenerateImage while emitting
// periodic progress heartbeats so downstream observers (e.g. the
// agent-bridge JSON-RPC stream consumed by OfficeDex) keep seeing
// task.progress events even when a hosted image provider takes minutes
// to respond. The heartbeat goroutine stops as soon as GenerateImage
// returns or ctx is cancelled.
//
// step controls which progress step heartbeat events are tagged with.
// assetIndex/assetTotal feed the human-readable counter shown in each
// heartbeat. When llm is nil the function returns an error without
// emitting anything (callers should guard before invoking).
func generateImageWithHeartbeat(
	ctx context.Context,
	llm engine.LLMClient,
	emitter engine.ProgressEmitter,
	req engine.ImageGenerationRequest,
	step string,
	assetIndex, assetTotal int,
) (*engine.ImageGenerationResult, error) {
	if llm == nil {
		return nil, fmt.Errorf("llm client is nil")
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	if emitter != nil && imageHeartbeatInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			ticker := time.NewTicker(imageHeartbeatInterval)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					elapsed := time.Since(start)
					emitter.Emit(ctx, engine.ProgressEvent{
						Step:      step,
						Status:    "running",
						Content:   fmt.Sprintf("Still waiting on image provider (asset %d/%d, elapsed %ds)", assetIndex, assetTotal, int(elapsed.Seconds())),
						ElapsedMs: elapsed.Milliseconds(),
					})
				}
			}
		}()
	}
	image, err := llm.GenerateImage(ctx, req)
	close(done)
	wg.Wait()
	return image, err
}
