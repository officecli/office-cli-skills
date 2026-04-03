package llm

import "testing"

func TestNewProvider_OpenAICompatible(t *testing.T) {
	cfg := Config{
		Provider:   "openai",
		BaseURL:    "https://api.example.com/v1",
		APIKey:     "token",
		Model:      "gpt-test",
		ImageModel: "gpt-image-1",
	}

	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider")
	}
}

func TestNewProvider_InternalHTTP(t *testing.T) {
	cfg := Config{
		Provider: "internal",
		BaseURL:  "https://internal.example.com",
		APIKey:   "token",
		Model:    "internal-model",
	}

	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider")
	}
}
