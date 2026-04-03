package generate

import "testing"

func TestParsePromptEnvelope_StrictJSONAndFallback(t *testing.T) {
	envelope, meta, err := ParsePromptEnvelope(`{"prompt":"make a deck","request_id":"req-1","options":{"generation_mode":"best"}}`)
	if err != nil {
		t.Fatalf("ParsePromptEnvelope: %v", err)
	}
	if envelope.Prompt != "make a deck" || envelope.Options.GenerationMode != ModeBest {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	if meta == nil || meta.RequestID != "req-1" {
		t.Fatalf("unexpected meta: %+v", meta)
	}

	plain, plainMeta, err := ParsePromptEnvelope("  plain prompt  ")
	if err != nil {
		t.Fatalf("ParsePromptEnvelope plain: %v", err)
	}
	if plain.Prompt != "plain prompt" || plain.Options.GenerationMode != ModeFast {
		t.Fatalf("unexpected plain envelope: %+v", plain)
	}
	if plainMeta == nil || plainMeta.Mode != "text_only" {
		t.Fatalf("unexpected plain meta: %+v", plainMeta)
	}
}

func TestParsePromptEnvelope_FallbackOnUnavailableImageSource(t *testing.T) {
	envelope, meta, err := ParsePromptEnvelope(`{"prompt":"make a deck","images":[{"source":"localfile"}],"options":{"allow_fallback":true}}`)
	if err != nil {
		t.Fatalf("ParsePromptEnvelope: %v", err)
	}
	if len(envelope.Images) != 0 {
		t.Fatalf("expected images to be cleared, got %+v", envelope.Images)
	}
	if meta == nil || meta.Mode != "text_only_fallback" || len(meta.Warnings) != 1 {
		t.Fatalf("unexpected meta: %+v", meta)
	}
}
