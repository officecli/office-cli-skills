package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	licenseprovider "github.com/officecli/officecli/internal/license"
	publishprovider "github.com/officecli/officecli/internal/providers/publish"
)

func LoadConfig(path string) (Config, error) {
	cfg := Config{}
	configPath := ResolveConfigPath(path)
	if configPath != "" {
		if data, err := os.ReadFile(configPath); err == nil {
			_ = json.Unmarshal(data, &cfg)
		}
	}
	applyEnvOverrides(&cfg)
	return cfg, nil
}

func ResolveConfigPath(path string) string {
	configPath := strings.TrimSpace(path)
	if configPath == "" {
		configPath = os.Getenv("OFFICE_CLI_CONFIG")
	}
	if configPath == "" {
		dir, err := os.UserConfigDir()
		if err == nil {
			configPath = filepath.Join(dir, "officecli", "config.json")
		}
	}
	return configPath
}

func WriteDefaultConfig(path string) (string, error) {
	defaultConfig := Config{
		Defaults: DefaultsConfig{
			OutputDir:       "./output",
			Mode:            "fast",
			Publish:         true,
			PPTXStylePreset: "tech-contrast",
		},
		Runtime: RuntimeConfig{
			Mode: RuntimeModeExternal,
		},
		LLM: LLMConfig{
			Provider:     "openai",
			BaseURL:      "https://your-generation-service.example.com/v1",
			APIKey:       "YOUR_GENERATION_SERVICE_KEY",
			Model:        "gpt-4.1",
			ImageBaseURL: "",
			ImageAPIKey:  "",
			ImageModel:   "gpt-image-1",
			ReviewModel:  "gpt-5.4-mini",
			TimeoutSec:   60,
		},
		License: licenseprovider.Config{
			BaseURL:    "https://platform.officecli.io",
			APIKey:     "",
			Enabled:    true,
			TimeoutSec: 30,
		},
		Publish: publishprovider.Config{
			Provider:   "http",
			BaseURL:    "https://platform.officecli.io",
			APIKey:     "",
			Enabled:    true,
			TimeoutSec: 60,
		},
	}
	return WriteConfig(path, defaultConfig, false)
}

func WriteConfig(path string, cfg Config, overwrite bool) (string, error) {
	configPath := ResolveConfigPath(path)
	if configPath == "" {
		return "", fmt.Errorf("unable to resolve config path")
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(configPath); err == nil && !overwrite {
		return configPath, nil
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return "", err
	}
	return configPath, nil
}

func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	setIfPresent(&cfg.LLM.Provider, "OFFICE_CLI_LLM_PROVIDER")
	setIfPresent(&cfg.LLM.BaseURL, "OFFICE_CLI_LLM_BASE_URL")
	setIfPresent(&cfg.LLM.APIKey, "OFFICE_CLI_LLM_API_KEY")
	setIfPresent(&cfg.LLM.Model, "OFFICE_CLI_LLM_MODEL")
	setIfPresent(&cfg.LLM.ImageBaseURL, "OFFICE_CLI_LLM_IMAGE_BASE_URL")
	setIfPresent(&cfg.LLM.ImageAPIKey, "OFFICE_CLI_LLM_IMAGE_API_KEY")
	setIfPresent(&cfg.LLM.ImageModel, "OFFICE_CLI_LLM_IMAGE_MODEL")
	setIfPresent(&cfg.LLM.ReviewModel, "OFFICE_CLI_LLM_REVIEW_MODEL")
	setIfPresent((*string)(&cfg.Runtime.Mode), "OFFICE_CLI_RUNTIME_MODE")
	setIfPresent(&cfg.Runtime.DefaultDocumentProfile, "OFFICE_CLI_RUNTIME_DEFAULT_DOCUMENT_PROFILE")
	setIfPresent(&cfg.License.BaseURL, "OFFICE_CLI_LICENSE_BASE_URL")
	setIfPresent(&cfg.License.APIKey, "OFFICE_CLI_LICENSE_API_KEY")
	setIfPresent(&cfg.Publish.Provider, "OFFICE_CLI_PUBLISH_PROVIDER")
	setIfPresent(&cfg.Publish.BaseURL, "OFFICE_CLI_PUBLISH_BASE_URL")
	setIfPresent(&cfg.Publish.APIKey, "OFFICE_CLI_PUBLISH_API_KEY")
	setIfPresent(&cfg.Defaults.OutputDir, "OFFICE_CLI_OUTPUT_DIR")
	setIfPresent(&cfg.Defaults.Mode, "OFFICE_CLI_MODE")
	setIfPresent(&cfg.Defaults.PPTXStylePreset, "OFFICE_CLI_PPTX_STYLE_PRESET")

	if value := os.Getenv("OFFICE_CLI_PUBLISH_ENABLED"); value != "" {
		cfg.Publish.Enabled = parseBool(value, cfg.Publish.Enabled)
	}
	if value := os.Getenv("OFFICE_CLI_LICENSE_ENABLED"); value != "" {
		cfg.License.Enabled = parseBool(value, cfg.License.Enabled)
	}
	if value := os.Getenv("OFFICE_CLI_DEFAULT_PUBLISH"); value != "" {
		cfg.Defaults.Publish = parseBool(value, cfg.Defaults.Publish)
	}
	if value := os.Getenv("OFFICE_CLI_LLM_TIMEOUT_SEC"); value != "" {
		cfg.LLM.TimeoutSec = parseInt(value, cfg.LLM.TimeoutSec)
	}
	if value := os.Getenv("OFFICE_CLI_LICENSE_TIMEOUT_SEC"); value != "" {
		cfg.License.TimeoutSec = parseInt(value, cfg.License.TimeoutSec)
	}
	if value := os.Getenv("OFFICE_CLI_LICENSE_USER_ID"); value != "" {
		cfg.License.UserID = uint64(parseInt(value, int(cfg.License.UserID)))
	}
	if value := os.Getenv("OFFICE_CLI_PUBLISH_TIMEOUT_SEC"); value != "" {
		cfg.Publish.TimeoutSec = parseInt(value, cfg.Publish.TimeoutSec)
	}
	if strings.TrimSpace(cfg.Publish.APIKey) == "" {
		cfg.Publish.APIKey = strings.TrimSpace(cfg.License.APIKey)
	}
}

func setIfPresent(target *string, env string) {
	if value := os.Getenv(env); value != "" {
		*target = value
	}
}

func parseBool(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}
