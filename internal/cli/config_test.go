package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConfigPathDevProfileUsesIsolatedDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OFFICE_CLI_PROFILE", "dev")
	t.Setenv("OFFICE_CLI_CONFIG", "")
	t.Setenv("OFFICECLI_DEV_CONFIG", "")

	got := ResolveConfigPath("")
	want := filepath.Join(home, ".config", "officecli-dev", "config.json")
	if got != want {
		t.Fatalf("ResolveConfigPath(dev) = %q, want %q", got, want)
	}
}

func TestResolveConfigPathDevProfileAllowsExplicitDevConfig(t *testing.T) {
	t.Setenv("OFFICE_CLI_PROFILE", "dev")
	t.Setenv("OFFICE_CLI_CONFIG", "")
	t.Setenv("OFFICECLI_DEV_CONFIG", "/tmp/officecli-dev-test.json")

	if got := ResolveConfigPath(""); got != "/tmp/officecli-dev-test.json" {
		t.Fatalf("ResolveConfigPath(dev override) = %q", got)
	}
}

func TestLoadConfigDevProfileUsesTestingPlatformDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OFFICE_CLI_PROFILE", "dev")
	t.Setenv("OFFICE_CLI_CONFIG", "")
	t.Setenv("OFFICECLI_DEV_CONFIG", "")
	t.Setenv("OFFICECLI_DEV_PLATFORM_BASE_URL", "")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.License.BaseURL != "https://officecli.shimodev.com" {
		t.Fatalf("license base url = %q", cfg.License.BaseURL)
	}
	if cfg.Publish.BaseURL != "https://officecli.shimodev.com" {
		t.Fatalf("publish base url = %q", cfg.Publish.BaseURL)
	}
	if cfg.Runtime.Mode != RuntimeModeHosted {
		t.Fatalf("runtime mode = %q", cfg.Runtime.Mode)
	}
	if !cfg.Publish.Enabled || !cfg.Defaults.Publish {
		t.Fatalf("publish enabled=%t default publish=%t", cfg.Publish.Enabled, cfg.Defaults.Publish)
	}
}

func TestLoadConfigDevProfileAllowsTestingPlatformOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OFFICE_CLI_PROFILE", "dev")
	t.Setenv("OFFICE_CLI_CONFIG", "")
	t.Setenv("OFFICECLI_DEV_CONFIG", "")
	t.Setenv("OFFICECLI_DEV_PLATFORM_BASE_URL", "https://staging.example.com")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.License.BaseURL != "https://staging.example.com" {
		t.Fatalf("license base url = %q", cfg.License.BaseURL)
	}
	if cfg.Publish.BaseURL != "https://staging.example.com" {
		t.Fatalf("publish base url = %q", cfg.Publish.BaseURL)
	}
}

func TestLoadConfigNormalProfileKeepsProductionDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OFFICE_CLI_PROFILE", "")
	t.Setenv("OFFICE_CLI_CONFIG", "")
	t.Setenv("OFFICECLI_DEV_CONFIG", "")
	t.Setenv("OFFICECLI_DEV_PLATFORM_BASE_URL", "https://staging.example.com")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if strings.Contains(ResolveConfigPath(""), "officecli-dev") {
		t.Fatalf("normal profile should not use dev config path: %q", ResolveConfigPath(""))
	}
	if cfg.License.BaseURL != "https://platform.officecli.io" {
		t.Fatalf("license base url = %q", cfg.License.BaseURL)
	}
	if cfg.Publish.BaseURL != "https://platform.officecli.io" {
		t.Fatalf("publish base url = %q", cfg.Publish.BaseURL)
	}
}

func TestRuntimeModeOrDefaultMapsLegacyCustomToExternalWithGenerationConfig(t *testing.T) {
	cfg := Config{
		Runtime: RuntimeConfig{Mode: RuntimeMode("custom")},
		LLM: LLMConfig{
			BaseURL: "https://llm.example.com/v1",
			APIKey:  "sk-test",
			Model:   "gpt-4.1",
		},
	}

	if got := cfg.RuntimeModeOrDefault(); got != RuntimeModeExternal {
		t.Fatalf("RuntimeModeOrDefault() = %q, want external", got)
	}
}

func TestRuntimeModeOrDefaultMapsLegacyCustomToExternalWithoutGenerationConfig(t *testing.T) {
	cfg := Config{Runtime: RuntimeConfig{Mode: RuntimeMode("custom")}}

	if got := cfg.RuntimeModeOrDefault(); got != RuntimeModeExternal {
		t.Fatalf("RuntimeModeOrDefault() = %q, want external", got)
	}
}

func TestRuntimeModeOrDefaultFallsBackForUnknownMode(t *testing.T) {
	withGeneration := Config{
		Runtime: RuntimeConfig{Mode: RuntimeMode("weird")},
		LLM: LLMConfig{
			BaseURL: "https://llm.example.com/v1",
			APIKey:  "sk-test",
			Model:   "gpt-4.1",
		},
	}
	if got := withGeneration.RuntimeModeOrDefault(); got != RuntimeModeExternal {
		t.Fatalf("RuntimeModeOrDefault(with generation) = %q, want external", got)
	}

	withoutGeneration := Config{Runtime: RuntimeConfig{Mode: RuntimeMode("weird")}}
	if got := withoutGeneration.RuntimeModeOrDefault(); got != RuntimeModeHosted {
		t.Fatalf("RuntimeModeOrDefault(without generation) = %q, want hosted", got)
	}
}
