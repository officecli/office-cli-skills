package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/officecli/officecli/engine"
	planengine "github.com/officecli/officecli/engine/plan"
	licenseprovider "github.com/officecli/officecli/internal/license"
	llmprovider "github.com/officecli/officecli/internal/providers/llm"
	publishprovider "github.com/officecli/officecli/internal/providers/publish"
	reviewprovider "github.com/officecli/officecli/internal/review"
)

type App struct {
	Stdout              io.Writer
	Stderr              io.Writer
	Stdin               io.Reader
	newLLMClient        func(cfg LLMConfig) (GeneratorLLMClient, error)
	newLicenseService   func(cfg LicenseConfig) (LicenseManager, error)
	newReviewer         func(cfg Config, progress engine.ProgressEmitter) (Reviewer, error)
	officeTaskPreflight func(ctx context.Context, command string, args []string) error
	checkForUpdates     func(ctx context.Context) (UpdateInfo, error)
	performUpdate       func(ctx context.Context, info UpdateInfo) error
	restartCommand      func(ctx context.Context, info UpdateInfo, args []string) error
}

var Version = "dev"
var Commit = "unknown"
var BuildDate = "unknown"

func NewApp(stdout, stderr io.Writer, stdin io.Reader) *App {
	return &App{
		Stdout: stdout,
		Stderr: stderr,
		Stdin:  stdin,
		newLLMClient: func(cfg LLMConfig) (GeneratorLLMClient, error) {
			return llmprovider.NewClient(llmprovider.Config{
				Provider:     cfg.Provider,
				BaseURL:      cfg.BaseURL,
				APIKey:       cfg.APIKey,
				Model:        cfg.Model,
				ImageBaseURL: cfg.ImageBaseURL,
				ImageAPIKey:  cfg.ImageAPIKey,
				ImageModel:   cfg.ImageModel,
				ReviewModel:  cfg.ReviewModel,
				TimeoutSec:   cfg.TimeoutSec,
			})
		},
		newLicenseService: func(cfg LicenseConfig) (LicenseManager, error) {
			svc, err := licenseprovider.NewService(licenseprovider.Config(cfg))
			if err != nil {
				return nil, err
			}
			if svc == nil {
				return nil, nil
			}
			return svc, nil
		},
		newReviewer: func(cfg Config, progress engine.ProgressEmitter) (Reviewer, error) {
			return reviewprovider.NewService(
				reviewprovider.NewSofficeConverter(),
				reviewprovider.NewOpenAIReviewer(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.ReviewModel, cfg.LLM.TimeoutSec),
				reviewProgressReporter{emitter: progress},
			), nil
		},
		officeTaskPreflight: func(ctx context.Context, command string, args []string) error {
			return runInstalledSkillPreflight(ctx, stdin, stdout, stderr, command, args)
		},
		checkForUpdates: defaultCheckForUpdates,
		performUpdate:   defaultPerformUpdate,
		restartCommand:  defaultRestartCommand,
	}
}

func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		_, err := io.WriteString(a.Stdout, HelpText())
		return err
	}
	if isVersionArg(args[0]) {
		_, err := fmt.Fprintln(a.Stdout, VersionText())
		return err
	}
	if len(args) > 1 && isHelpArg(args[1]) {
		var help string
		switch args[0] {
		case "config":
			help = ConfigHelpText()
		case "auth":
			help = AuthHelpText()
		case "new":
			help = NewHelpText()
		case "score":
			help = ReviewHelpText()
		case "review":
			help = ReviewHelpText()
		case "upgrade":
			help = UpgradeHelpText()
		case "agent-bridge":
			help = AgentBridgeHelpText()
		default:
			help = HelpText()
		}
		_, err := io.WriteString(a.Stdout, help)
		return err
	}
	if args[0] == "new" && len(args) > 2 {
		for _, arg := range args[1:] {
			if isHelpArg(arg) {
				_, err := io.WriteString(a.Stdout, NewHelpText())
				return err
			}
		}
	}
	if (args[0] == "review" || args[0] == "score") && len(args) > 2 {
		for _, arg := range args[1:] {
			if isHelpArg(arg) {
				_, err := io.WriteString(a.Stdout, ReviewHelpText())
				return err
			}
		}
	}
	if err := a.maybeHandleUpdate(ctx, args); err != nil {
		if errors.Is(err, errCommandHandledByRestart) {
			return nil
		}
		return err
	}
	switch args[0] {
	case "config":
		return a.runConfig(args[1:])
	case "auth":
		cfg, err := LoadConfig("")
		if err != nil {
			return err
		}
		return a.runAuth(ctx, cfg, args[1:])
	case "new":
		if err := a.officeTaskPreflight(ctx, args[0], args[1:]); err != nil {
			return err
		}
		cfg, err := LoadConfig("")
		if err != nil {
			return err
		}
		return a.runNew(ctx, cfg, args[1:])
	case "score":
		cfg, err := LoadConfig("")
		if err != nil {
			return err
		}
		return a.runReview(ctx, cfg, args[1:])
	case "review":
		cfg, err := LoadConfig("")
		if err != nil {
			return err
		}
		return a.runReview(ctx, cfg, args[1:])
	case "upgrade":
		return a.runUpgrade(ctx, args[1:])
	case "agent-bridge":
		if err := a.officeTaskPreflight(ctx, args[0], args[1:]); err != nil {
			return err
		}
		cfg, err := LoadConfig("")
		if err != nil {
			return err
		}
		return a.runAgentBridge(ctx, cfg, args[1:])
	default:
		return fmt.Errorf("unsupported command: %s", args[0])
	}
}

func (a *App) collectInitConfigFromEnv() (Config, error) {
	cfg := Config{}
	applyEnvOverrides(&cfg)
	cfg = mergeInitBaseConfig(defaultInitConfig(), cfg)
	missing := make([]string, 0, 3)
	if cfg.RuntimeModeOrDefault() == RuntimeModeExternal {
		if strings.TrimSpace(cfg.LLM.BaseURL) == "" {
			missing = append(missing, "OFFICE_CLI_LLM_BASE_URL")
		}
		if strings.TrimSpace(cfg.LLM.APIKey) == "" {
			missing = append(missing, "OFFICE_CLI_LLM_API_KEY")
		}
		if strings.TrimSpace(cfg.LLM.Model) == "" {
			missing = append(missing, "OFFICE_CLI_LLM_MODEL")
		}
	} else {
		if strings.TrimSpace(cfg.License.BaseURL) == "" {
			missing = append(missing, "OFFICE_CLI_LICENSE_BASE_URL")
		}
		if strings.TrimSpace(cfg.License.APIKey) == "" {
			missing = append(missing, "OFFICE_CLI_LICENSE_API_KEY")
		}
	}
	if len(missing) > 0 {
		if cfg.RuntimeModeOrDefault() == RuntimeModeHosted {
			return Config{}, fmt.Errorf("missing required environment variables. Complete the platform access settings first or run `officecli config set-license`")
		}
		return Config{}, fmt.Errorf("missing required environment variables. Complete the generation service settings first or run `officecli config set-generation`")
	}
	if err := publishprovider.ValidateConfig(cfg.Publish); err != nil {
		return Config{}, err
	}
	cfg.Defaults.Publish = cfg.Publish.Enabled
	return cfg, nil
}

func defaultInitConfig() Config {
	return Config{
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
			Provider:    "openai",
			BaseURL:     "https://api.openai.com/v1",
			ImageModel:  "gpt-image-1",
			Model:       "gpt-4.1",
			ReviewModel: "gpt-5.4-mini",
			TimeoutSec:  60,
		},
		License: LicenseConfig{
			BaseURL:    "https://platform.officecli.io",
			Enabled:    true,
			TimeoutSec: 30,
		},
		Publish: publishprovider.Config{
			Provider:   "http",
			BaseURL:    "https://platform.officecli.io",
			Enabled:    true,
			TimeoutSec: 60,
		},
	}
}

func isHelpArg(value string) bool {
	switch strings.TrimSpace(value) {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func isVersionArg(value string) bool {
	switch strings.TrimSpace(value) {
	case "-v", "-version", "--version", "version":
		return true
	default:
		return false
	}
}

func HelpText() string {
	return `officecli

Generate Office documents and images from natural language.

Commands:
  config                  View or update local configuration
  auth                    View or update access settings
  new                     Generate a new PPTX / DOCX / XLSX / Report file or image
  score                   Run PPTX scoring on demand
  review                  Review the quality of a local PPTX file
  upgrade                 Check for updates and upgrade officecli
  agent-bridge            Expose an agent interface over JSON-RPC via stdio

Usage:
  officecli new <pptx|docx|xlsx|report|img> <topic> [brief]
  officecli config status
  officecli auth status
  officecli auth set-key <api-key>
  officecli upgrade
  officecli score pptx ./deck.pptx
  officecli review pptx ./deck.pptx

Common options:
  --prompt <text>         Provide the full prompt directly
  --prompt-file <path>    Read the prompt from a file
  --mode fast|best        Choose fast generation or interactive refinement
  --lang <value>          Set the output language
  --style <value>         Set the style
  --audience <value>      Set the audience
  --out <dir>             Set the output directory
  --file <path>           Provide the input workbook file (report only)
  --ratio <value>         Set image ratio: square, landscape, or portrait (img only)
  --reference-image <path-or-url>
                          Provide one reference image for img generation
  --image-quality <value> Set PPT image quality: standard or premium (pptx only)
  --local-preview         Generate local HTML/JSON preview sidecars
  --debug                 Print generation diagnostics to stderr
  --publish               Force online preview publishing
  --no-publish            Disable online preview publishing
  --no-images             Disable automatic PPT images
  --json                  Output JSON
  --version               Show the current version

Default behavior:
  - Default output directory: ./output
  - Default mode: fast
  - If access checks are enabled, availability is verified before generation
  - If defaults.publish=true and publishing is configured, document output is published automatically
  - Standalone img publishes online by default when publishing is configured; use --no-publish for local-only output
  - If publishing is not configured, files are saved locally and online preview is skipped
  - Standalone img generation always uses the OfficeCLI server and requires config set-license
  - Standalone img supports one reference image with --reference-image <path-or-url>
  - Standalone img local preview sidecars are not supported

Config file:
  macOS   ~/Library/Application Support/officecli/config.json
  Linux   ~/.config/officecli/config.json
  Windows %AppData%\officecli\config.json

Examples:
  officecli config status
  officecli config set-generation
  officecli config set-license
  officecli config set-publish
  officecli config set-defaults
  officecli auth status
  officecli auth --help
  officecli auth set-key <your-api-key>
  officecli upgrade
  officecli upgrade --help
  officecli new pptx "Enterprise Collaboration Platform Overview" "Explain the product capabilities, customer value, and use cases of this enterprise collaboration platform"
  officecli new img "Launch Visual" --prompt "A polished product launch hero image" --ratio landscape --reference-image ./reference.png
  officecli score pptx ./output/enterprise_collaboration_platform_overview.pptx
  officecli review pptx ./output/enterprise_collaboration_platform_overview.pptx
  officecli new --help
  officecli score --help
  officecli review --help
  officecli new docx "Quarterly Review" --prompt-file ./examples/prompt.txt
  officecli new xlsx "Sales Analysis" --json
  officecli new report "Q2 Business Review" --file ./data/q2_metrics.xlsx --audience "Board and investors"
  officecli --version
`
}

func ConfigHelpText() string {
	return `Usage:
  officecli config status
  officecli config set-generation
  officecli config set-license
  officecli config set-publish
  officecli config set-defaults

Description:
  View the current configuration status, or update the document generation service, PPT image service, access checks, preview publishing, and defaults.
`
}

func AuthHelpText() string {
	return `Usage:
  officecli auth status
  officecli auth set-key <api-key>

Description:
  View access status or save a paid API key.
`
}

func NewHelpText() string {
	return `Usage:
  officecli new <pptx|docx|xlsx|report|img> <topic> [brief]

Common options:
  --prompt <text>         Provide the full prompt directly
  --prompt-file <path>    Read the prompt from a file
  --mode fast|best        Choose fast generation or interactive refinement
  --lang <value>          Set the output language
  --style <value>         Set the style
  --audience <value>      Set the audience
  --out <dir>             Set the output directory
  --file <path>           Provide the input workbook file (report only)
  --ratio <value>         Set image ratio: square, landscape, or portrait (img only)
  --reference-image <path-or-url>
                          Provide one reference image for img generation
  --image-quality <value> Set PPT image quality: standard or premium (pptx only)
  --local-preview         Generate local HTML/JSON preview sidecars
  --debug                 Print generation diagnostics to stderr
  --publish               Force online preview publishing
  --no-publish            Disable online preview publishing
  --no-images             Disable automatic PPT images
  --json                  Output JSON

Description:
  - ` + "`pptx`" + ` generation tries to add images and embed them in the final file by default
  - ` + "`pptx --image-quality premium`" + ` uses the hosted image route for PPT visual assets and consumes hosted image credits only for successful image assets
  - For a text-only PPT, pass ` + "`--no-images`" + `
  - If images never appear, run ` + "`officecli config set-generation`" + ` and check the image model URL, credentials, and model name
  - ` + "`report`" + ` requires ` + "`--file <xlsx-path>`" + ` and generates a single local HTML report file from workbook data
  - ` + "`img`" + ` generates one local image through the OfficeCLI server; run ` + "`officecli config set-license`" + ` first
  - ` + "`img`" + ` supports ` + "`--ratio square|landscape|portrait`" + `, ` + "`--reference-image <path-or-url>`" + `, and publishing by default when configured
  - ` + "`img`" + ` does not support best mode, source files, or local preview
`
}

func ReviewHelpText() string {
	return `Usage:
  officecli score pptx <file>
  officecli review pptx <file>

Common options:
  --json                  Output JSON
  --no-visual             Run structural checks only
  --fail-below <0-100>    Exit non-zero when the total score is below the threshold

Description:
  - Scoring does not run automatically after generation
  - To run it manually, use ` + "`officecli score ...`" + ` or ` + "`officecli review ...`" + `
`
}

func UpgradeHelpText() string {
	return `Usage:
  officecli upgrade

Description:
  Check for a newer OfficeCLI version and apply the upgrade using the current installation channel when automatic upgrades are supported.
`
}

func (a *App) runConfig(args []string) error {
	if len(args) == 0 {
		_, err := io.WriteString(a.Stdout, ConfigHelpText())
		return err
	}
	cfg, err := LoadConfig("")
	if err != nil {
		return err
	}
	switch args[0] {
	case "status":
		return a.runConfigStatus(cfg)
	case "set-generation":
		return a.runConfigSetGeneration(cfg)
	case "set-license":
		return a.runConfigSetLicense(cfg)
	case "set-publish":
		return a.runConfigSetPublish(cfg)
	case "set-defaults":
		return a.runConfigSetDefaults(cfg)
	default:
		return fmt.Errorf("unsupported config command: %s", args[0])
	}
}

func (a *App) runUpgrade(ctx context.Context, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("unsupported upgrade command: %s", args[0])
	}
	info, err := a.checkForUpdatesNow(ctx)
	if err != nil {
		return err
	}
	if !info.Available {
		_, err := fmt.Fprintf(a.Stdout, "officecli is already up to date (%s).\n", fallbackString(info.CurrentVersion, "unknown"))
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Update available for officecli: current %s, latest stable %s.\n", fallbackString(info.CurrentVersion, "unknown"), fallbackString(info.LatestVersionLabel, "current stable")); err != nil {
		return err
	}
	if strings.TrimSpace(info.UpdateCommand) != "" {
		if _, err := fmt.Fprintf(a.Stdout, "Suggested update command: %s\n", info.UpdateCommand); err != nil {
			return err
		}
	}
	if !info.AutoUpdateSupported || a.performUpdate == nil {
		if strings.TrimSpace(info.UpdateCommand) == "" {
			_, err := fmt.Fprintln(a.Stdout, "Automatic upgrade is not available for the current installation.")
			return err
		}
		return nil
	}
	if err := a.performUpdate(ctx, info); err != nil {
		return err
	}
	_, err = fmt.Fprintln(a.Stdout, "officecli was updated.")
	return err
}

func (a *App) runConfigStatus(cfg Config) error {
	configPath := ResolveConfigPath("")
	if configPath == "" {
		return fmt.Errorf("unable to resolve config path")
	}
	if _, err := fmt.Fprintf(a.Stdout, "Config file: %s\n", configPath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Generation service configured: %t\n", hasGenerationConfig(cfg)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Image generation config: %s\n", imageConfigSummary(cfg)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Access checks enabled: %t\n", cfg.License.Enabled); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Paid API key configured: %t\n", strings.TrimSpace(cfg.License.APIKey) != ""); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Online preview publishing enabled: %t\n", cfg.Publish.Enabled); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Default output directory: %s\n", fallbackString(cfg.Defaults.OutputDir, "./output")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Default generation mode: %s\n", fallbackString(cfg.Defaults.Mode, "fast")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Default PPT style preset: %s\n", fallbackString(cfg.Defaults.PPTXStylePreset, "tech-contrast")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Visual review model: %s\n", fallbackString(cfg.LLM.ReviewModel, "gpt-5.4-mini")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Publish by default after generation: %t\n", cfg.Defaults.Publish); err != nil {
		return err
	}
	return nil
}

func (a *App) runConfigSetGeneration(cfg Config) error {
	reader := bufio.NewReader(a.Stdin)
	var err error
	if cfg.LLM.BaseURL, err = a.promptRequiredLine(reader, "Enter the generation service URL", cfg.LLM.BaseURL); err != nil {
		return err
	}
	if cfg.LLM.APIKey, err = a.promptRequiredLine(reader, "Enter the generation service credential", cfg.LLM.APIKey); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		cfg.LLM.Model = defaultInitConfig().LLM.Model
	}
	useSeparateImageService, err := a.promptYesNo(reader, "Use a separate image model service for automatic PPT images? (yes/no)", hasSeparateImageConfig(cfg))
	if err != nil {
		return err
	}
	if useSeparateImageService {
		defaultImageBaseURL := fallbackString(cfg.LLM.ImageBaseURL, cfg.LLM.BaseURL)
		if cfg.LLM.ImageBaseURL, err = a.promptRequiredLine(reader, "Enter the image model service URL (image url)", defaultImageBaseURL); err != nil {
			return err
		}
		if cfg.LLM.ImageAPIKey, err = a.promptRequiredLine(reader, "Enter the image model credential (image ak)", fallbackString(cfg.LLM.ImageAPIKey, cfg.LLM.APIKey)); err != nil {
			return err
		}
		if cfg.LLM.ImageModel, err = a.promptRequiredLine(reader, "Enter the image model name", fallbackString(cfg.LLM.ImageModel, defaultInitConfig().LLM.ImageModel)); err != nil {
			return err
		}
	} else {
		cfg.LLM.ImageBaseURL = ""
		cfg.LLM.ImageAPIKey = ""
		if strings.TrimSpace(cfg.LLM.ImageModel) == "" {
			cfg.LLM.ImageModel = defaultInitConfig().LLM.ImageModel
		}
	}
	path, err := WriteConfig("", cfg, true)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Stdout, "Updated generation service config: %s\nNote: pptx generation adds images automatically by default. Use `--no-images` for a text-only deck.\n", path)
	return err
}

func (a *App) runConfigSetLicense(cfg Config) error {
	reader := bufio.NewReader(a.Stdin)
	var err error
	cfg.License.Enabled, err = a.promptYesNo(reader, "Enable access checks? (yes/no)", cfg.License.Enabled)
	if err != nil {
		return err
	}
	cfg.License.BaseURL = defaultInitConfig().License.BaseURL
	if cfg.License.Enabled {
		if cfg.License.APIKey, err = a.promptLine(reader, "Enter the paid API key (optional; free quota will be used first)", cfg.License.APIKey); err != nil {
			return err
		}
	} else {
		cfg.License.APIKey = ""
	}
	path, err := WriteConfig("", cfg, true)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Stdout, "Updated access config: %s\n", path)
	return err
}

func (a *App) runConfigSetPublish(cfg Config) error {
	reader := bufio.NewReader(a.Stdin)
	var err error
	cfg.Publish.Enabled, err = a.promptYesNo(reader, "Enable online preview publishing? (yes/no)", cfg.Publish.Enabled)
	if err != nil {
		return err
	}
	cfg.Defaults.Publish = cfg.Publish.Enabled
	if cfg.Publish.Enabled {
		defaultBaseURL := fallbackString(cfg.Publish.BaseURL, publishprovider.EmbeddedPublishBaseURL)
		if cfg.Publish.BaseURL, err = a.promptRequiredLine(reader, "Enter the publishing service URL", defaultBaseURL); err != nil {
			return err
		}
		if cfg.Publish.APIKey, err = a.promptRequiredLine(reader, "Enter the publishing service credential", cfg.Publish.APIKey); err != nil {
			return err
		}
	} else {
		cfg.Publish.BaseURL = ""
		cfg.Publish.APIKey = ""
	}
	path, err := WriteConfig("", cfg, true)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Stdout, "Updated online preview publishing config: %s\n", path)
	return err
}

func (a *App) runConfigSetDefaults(cfg Config) error {
	reader := bufio.NewReader(a.Stdin)
	var err error
	if cfg.Defaults.OutputDir, err = a.promptLine(reader, "Enter the default output directory", fallbackString(cfg.Defaults.OutputDir, "./output")); err != nil {
		return err
	}
	modeChoice, err := a.promptChoice(reader, "Choose the default generation mode", []string{"fast", "best"})
	if err != nil {
		return err
	}
	if modeChoice == 2 {
		cfg.Defaults.Mode = "best"
	} else {
		cfg.Defaults.Mode = "fast"
	}
	cfg.Defaults.Publish, err = a.promptYesNo(reader, "Publish online preview by default after generation? (yes/no)", cfg.Defaults.Publish)
	if err != nil {
		return err
	}
	if cfg.Defaults.PPTXStylePreset, err = a.promptLine(reader, "Enter the default PPT style preset", fallbackString(cfg.Defaults.PPTXStylePreset, "tech-contrast")); err != nil {
		return err
	}
	path, err := WriteConfig("", cfg, true)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Stdout, "Updated default config: %s\n", path)
	return err
}

func hasGenerationConfig(cfg Config) bool {
	return strings.TrimSpace(cfg.LLM.BaseURL) != "" &&
		strings.TrimSpace(cfg.LLM.APIKey) != "" &&
		strings.TrimSpace(cfg.LLM.Model) != ""
}

func hasSeparateImageConfig(cfg Config) bool {
	return strings.TrimSpace(cfg.LLM.ImageBaseURL) != "" ||
		strings.TrimSpace(cfg.LLM.ImageAPIKey) != ""
}

func imageConfigSummary(cfg Config) string {
	if hasSeparateImageConfig(cfg) {
		return "Separate image model service configured"
	}
	return "Not configured separately (reuses the generation service by default)"
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func VersionText() string {
	return fmt.Sprintf("officecli version %s (%s, %s)", Version, Commit, BuildDate)
}

func missingLLMConfig(cfg Config) string {
	switch {
	case strings.TrimSpace(cfg.LLM.BaseURL) == "":
		return "generation service URL"
	case strings.TrimSpace(cfg.LLM.APIKey) == "":
		return "generation service credential"
	case strings.TrimSpace(cfg.LLM.Model) == "":
		return "generation model configuration"
	default:
		return ""
	}
}

func missingHostedConfig(cfg Config) string {
	switch {
	case strings.TrimSpace(cfg.License.BaseURL) == "":
		return "platform service URL"
	case strings.TrimSpace(cfg.License.APIKey) == "":
		return "platform service credential"
	default:
		return ""
	}
}

func (a *App) collectInitConfig(reader *bufio.Reader, base Config) (Config, error) {
	cfg := mergeInitBaseConfig(defaultInitConfig(), base)

	var err error
	if cfg.Runtime.Mode == RuntimeModeExternal {
		if cfg.LLM.BaseURL, err = a.promptRequiredLine(reader, "Enter the generation service URL", cfg.LLM.BaseURL); err != nil {
			return Config{}, err
		}
		if cfg.LLM.APIKey, err = a.promptRequiredLine(reader, "Enter the generation service credential", cfg.LLM.APIKey); err != nil {
			return Config{}, err
		}
		if strings.TrimSpace(cfg.LLM.Model) == "" {
			cfg.LLM.Model = defaultInitConfig().LLM.Model
		}
	}
	if cfg.License.APIKey, err = a.promptLine(reader, "Enter the paid API key (optional; free quota will be used first)", cfg.License.APIKey); err != nil {
		return Config{}, err
	}
	cfg.Publish.Enabled, err = a.promptYesNo(reader, "Enable online preview publishing? (yes/no)", cfg.Publish.Enabled)
	if err != nil {
		return Config{}, err
	}
	cfg.Defaults.Publish = cfg.Publish.Enabled
	if cfg.Publish.Enabled {
		defaultBaseURL := fallbackString(cfg.Publish.BaseURL, publishprovider.EmbeddedPublishBaseURL)
		if cfg.Publish.BaseURL, err = a.promptRequiredLine(reader, "Enter the publishing service URL", defaultBaseURL); err != nil {
			return Config{}, err
		}
		if cfg.Publish.APIKey, err = a.promptRequiredLine(reader, "Enter the publishing service credential", cfg.Publish.APIKey); err != nil {
			return Config{}, err
		}
	} else {
		cfg.Publish.BaseURL = ""
		cfg.Publish.APIKey = ""
	}
	if outputDir, err := a.promptLine(reader, "Enter the default output directory", cfg.Defaults.OutputDir); err == nil {
		cfg.Defaults.OutputDir = outputDir
	} else {
		return Config{}, err
	}
	return cfg, nil
}

func mergeInitBaseConfig(defaults Config, base Config) Config {
	cfg := defaults
	if strings.TrimSpace(base.Defaults.OutputDir) != "" {
		cfg.Defaults.OutputDir = base.Defaults.OutputDir
	}
	if strings.TrimSpace(base.Defaults.Mode) != "" {
		cfg.Defaults.Mode = base.Defaults.Mode
	}
	cfg.Defaults.Publish = base.Defaults.Publish
	if strings.TrimSpace(base.Defaults.PPTXStylePreset) != "" {
		cfg.Defaults.PPTXStylePreset = base.Defaults.PPTXStylePreset
	}
	if base.Runtime.Mode != "" {
		cfg.Runtime.Mode = base.Runtime.Mode
	}
	if strings.TrimSpace(base.Runtime.DefaultDocumentProfile) != "" {
		cfg.Runtime.DefaultDocumentProfile = base.Runtime.DefaultDocumentProfile
	}
	if strings.TrimSpace(base.LLM.Provider) != "" {
		cfg.LLM.Provider = base.LLM.Provider
	}
	if strings.TrimSpace(base.LLM.BaseURL) != "" {
		cfg.LLM.BaseURL = base.LLM.BaseURL
	}
	if strings.TrimSpace(base.LLM.APIKey) != "" {
		cfg.LLM.APIKey = base.LLM.APIKey
	}
	if strings.TrimSpace(base.LLM.Model) != "" {
		cfg.LLM.Model = base.LLM.Model
	}
	if strings.TrimSpace(base.LLM.ImageBaseURL) != "" {
		cfg.LLM.ImageBaseURL = base.LLM.ImageBaseURL
	}
	if strings.TrimSpace(base.LLM.ImageAPIKey) != "" {
		cfg.LLM.ImageAPIKey = base.LLM.ImageAPIKey
	}
	if strings.TrimSpace(base.LLM.ImageModel) != "" {
		cfg.LLM.ImageModel = base.LLM.ImageModel
	}
	if strings.TrimSpace(base.LLM.ReviewModel) != "" {
		cfg.LLM.ReviewModel = base.LLM.ReviewModel
	}
	if base.LLM.TimeoutSec > 0 {
		cfg.LLM.TimeoutSec = base.LLM.TimeoutSec
	}
	if strings.TrimSpace(base.License.BaseURL) != "" {
		cfg.License.BaseURL = base.License.BaseURL
	}
	if strings.TrimSpace(base.License.APIKey) != "" {
		cfg.License.APIKey = base.License.APIKey
	}
	cfg.License.Enabled = base.License.Enabled || cfg.License.Enabled
	if base.License.TimeoutSec > 0 {
		cfg.License.TimeoutSec = base.License.TimeoutSec
	}
	if strings.TrimSpace(base.Publish.Provider) != "" {
		cfg.Publish.Provider = base.Publish.Provider
	}
	if strings.TrimSpace(base.Publish.BaseURL) != "" {
		cfg.Publish.BaseURL = base.Publish.BaseURL
	}
	if strings.TrimSpace(base.Publish.APIKey) != "" {
		cfg.Publish.APIKey = base.Publish.APIKey
	}
	cfg.Publish.Enabled = base.Publish.Enabled
	if base.Publish.TimeoutSec > 0 {
		cfg.Publish.TimeoutSec = base.Publish.TimeoutSec
	}
	return cfg
}

func (a *App) promptLine(reader *bufio.Reader, label, defaultValue string) (string, error) {
	if defaultValue != "" {
		if _, err := fmt.Fprintf(a.Stdout, "%s [%s]: ", label, defaultValue); err != nil {
			return "", err
		}
	} else {
		if _, err := fmt.Fprintf(a.Stdout, "%s: ", label); err != nil {
			return "", err
		}
	}
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = defaultValue
	}
	return line, nil
}

func (a *App) promptRequiredLine(reader *bufio.Reader, label, defaultValue string) (string, error) {
	for {
		line, err := a.promptLine(reader, label, defaultValue)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(line) != "" {
			return line, nil
		}
		if _, err := fmt.Fprintln(a.Stdout, "This field cannot be empty. Please try again."); err != nil {
			return "", err
		}
	}
}

func (a *App) promptYesNo(reader *bufio.Reader, label string, defaultValue bool) (bool, error) {
	defaultText := "yes"
	if !defaultValue {
		defaultText = "no"
	}
	for {
		line, err := a.promptLine(reader, label, defaultText)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "yes", "y", "true", "1", "on":
			return true, nil
		case "no", "n", "false", "0", "off":
			return false, nil
		default:
			if _, err := fmt.Fprintln(a.Stdout, "Please enter yes or no."); err != nil {
				return false, err
			}
		}
	}
}

func (a *App) promptChoice(reader *bufio.Reader, label string, options []string) (int, error) {
	for {
		if _, err := fmt.Fprintf(a.Stdout, "%s [1-%d]: ", label, len(options)); err != nil {
			return 0, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line == "" && err == io.EOF {
			return 1, nil
		}
		switch line {
		case "1":
			return 1, nil
		case "2":
			if len(options) >= 2 {
				return 2, nil
			}
		case "3":
			if len(options) >= 3 {
				return 3, nil
			}
		}
		if _, err := fmt.Fprintln(a.Stdout, "Please enter a valid option number."); err != nil {
			return 0, err
		}
	}
}

func (a *App) runNew(ctx context.Context, cfg Config, args []string) error {
	stdinContent, isTTY, err := readStdin(a.Stdin)
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	job, err := BuildGenerateJob(args, cfg, InputSources{
		Stdin: stdinContent,
		IsTTY: isTTY,
		CWD:   cwd,
	})
	if err != nil {
		return err
	}
	progress := NewProgressRenderer(a.Stdout, job.JSONOutput, isTerminalWriter(a.Stdout))
	progress.SetTransientCompletions(true)
	defer progress.Close()
	result, err := a.executeGenerateJob(ctx, cfg, job, isTTY, progress, nil)
	if err != nil {
		return err
	}
	progress.Clear()
	return RenderResult(a.Stdout, result, job.JSONOutput)
}

func (a *App) runAuth(ctx context.Context, cfg Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("unsupported auth command")
	}
	switch args[0] {
	case "status":
		return a.runAuthStatus(ctx, cfg)
	case "set-key":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			key, err := a.promptLine(bufio.NewReader(a.Stdin), "Enter the paid API key", "")
			if err != nil {
				return err
			}
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("api-key is required")
			}
			return a.runAuthSetKey(ctx, cfg, strings.TrimSpace(key))
		}
		return a.runAuthSetKey(ctx, cfg, strings.TrimSpace(args[1]))
	default:
		return fmt.Errorf("unsupported auth command: %s", args[0])
	}
}

func (a *App) runAuthStatus(ctx context.Context, cfg Config) error {
	result, err := a.checkLicenseWithRuntime(ctx, cfg.License, cfg.RuntimeModeOrDefault(), "", "status")
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Current access mode: %s\n", displayAccessMode(result.AccessMode)); err != nil {
		return err
	}
	if result.PlanName != "" {
		if _, err := fmt.Fprintf(a.Stdout, "Current plan: %s\n", result.PlanName); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(a.Stdout, ""); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(a.Stdout, "Quota summary"); err != nil {
		return err
	}
	freeTrial, rewardQuota, paidQuota := quotaSnapshotSections(result)
	if _, err := fmt.Fprintf(a.Stdout, "Free trial today (this machine, UTC): %d total / %d used / %d remaining\n", freeTrial.Limit, freeTrial.Used, freeTrial.Remaining); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Reward quota remaining: %d\n", rewardQuota.Remaining); err != nil {
		return err
	}
	if paidQuota.CurrentKeyPrefix != "" {
		if _, err := fmt.Fprintf(a.Stdout, "Paid quota on current key (%s): %d total / %d used / %d remaining\n", paidQuota.CurrentKeyPrefix, paidQuota.CurrentKeyTotal, paidQuota.CurrentKeyUsed, paidQuota.CurrentKeyRemaining); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(a.Stdout, "Paid quota on current key: no paid API key configured"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(a.Stdout, "Access checks enabled: %t\n", cfg.License.Enabled); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "Paid API key configured: %t\n", strings.TrimSpace(cfg.License.APIKey) != ""); err != nil {
		return err
	}
	if strings.TrimSpace(result.Message) != "" {
		if _, err := fmt.Fprintln(a.Stdout, result.Message); err != nil {
			return err
		}
	}
	return nil
}

func quotaSnapshotSections(result *LicenseCheckResult) (licenseprovider.FreeTrialDailySnapshot, licenseprovider.RewardQuotaSnapshot, licenseprovider.PaidExternalQuotaSnapshot) {
	freeTrial := licenseprovider.FreeTrialDailySnapshot{
		Limit:     result.FreeLimit,
		Used:      result.FreeUsed,
		Remaining: result.FreeRemaining,
	}
	rewardQuota := licenseprovider.RewardQuotaSnapshot{Remaining: result.RewardRemaining}
	paidQuota := licenseprovider.PaidExternalQuotaSnapshot{
		CurrentKeyTotal:     result.PaidQuotaTotal,
		CurrentKeyUsed:      result.PaidQuotaUsed,
		CurrentKeyRemaining: result.PaidQuotaRemaining,
	}
	if result.QuotaSnapshot == nil {
		return freeTrial, rewardQuota, paidQuota
	}
	if snapshot := result.QuotaSnapshot.FreeTrialDaily; snapshot.Limit > 0 || snapshot.Used > 0 || snapshot.Remaining > 0 {
		freeTrial = snapshot
	}
	if snapshot := result.QuotaSnapshot.RewardQuota; snapshot.Remaining > 0 || result.RewardRemaining == 0 {
		rewardQuota = snapshot
	}
	if snapshot := result.QuotaSnapshot.PaidExternalQuota; snapshot.CurrentKeyPrefix != "" || snapshot.CurrentKeyTotal > 0 || snapshot.CurrentKeyUsed > 0 || snapshot.CurrentKeyRemaining > 0 {
		paidQuota = snapshot
	}
	return freeTrial, rewardQuota, paidQuota
}

func (a *App) runAuthSetKey(ctx context.Context, cfg Config, key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("api-key is required")
	}
	cfg.License.Enabled = true
	cfg.License.APIKey = key
	result, err := a.checkLicenseWithRuntime(ctx, cfg.License, cfg.RuntimeModeOrDefault(), "", "status")
	if err != nil {
		return err
	}
	if !result.Allowed || result.AccessMode == LicenseAccessModeBlocked {
		return fmt.Errorf("paid API key validation failed: %s", fallbackMessage(result.Message, "the key is invalid, expired, depleted, or the access service is unavailable"))
	}
	if _, err := WriteConfig("", cfg, true); err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Stdout, "Saved the paid API key. Current access mode: %s\n", displayAccessMode(result.AccessMode))
	return err
}

func (a *App) checkLicense(ctx context.Context, cfg LicenseConfig, documentType, action string) (*LicenseCheckResult, error) {
	return a.checkLicenseWithRuntime(ctx, cfg, RuntimeModeExternal, documentType, action)
}

func (a *App) checkLicenseWithRuntime(ctx context.Context, cfg LicenseConfig, runtimeMode RuntimeMode, documentType, action string) (*LicenseCheckResult, error) {
	manager, err := a.newLicenseService(cfg)
	if err != nil {
		return nil, err
	}
	if manager == nil {
		return &LicenseCheckResult{
			Allowed:    true,
			AccessMode: LicenseAccessModeDisabled,
			Message:    "Access checks are not configured.",
		}, nil
	}
	fingerprintHash := licenseprovider.ComputeFingerprintHash()
	requestNonce, err := licenseprovider.NewRequestNonce()
	if err != nil {
		return nil, err
	}
	checkReq := LicenseCheckRequest{
		FingerprintHash: fingerprintHash,
		UserID:          cfg.UserID,
		APIKey:          strings.TrimSpace(cfg.APIKey),
		CLIVersion:      Version,
		DocumentType:    documentType,
		RuntimeMode:     runtimeModeLabel(runtimeMode),
		RequestNonce:    requestNonce,
		Action:          action,
	}
	result, err := manager.Check(ctx, LicenseCheckRequest{
		FingerprintHash: checkReq.FingerprintHash,
		UserID:          checkReq.UserID,
		APIKey:          checkReq.APIKey,
		CLIVersion:      checkReq.CLIVersion,
		DocumentType:    checkReq.DocumentType,
		RuntimeMode:     checkReq.RuntimeMode,
		RequestNonce:    checkReq.RequestNonce,
		Action:          checkReq.Action,
	})
	if err != nil {
		_ = licenseprovider.SaveState(licenseprovider.State{
			FingerprintHash: fingerprintHash,
			LastCheckAt:     time.Now(),
			LastDecision:    "error",
			LastError:       err.Error(),
		})
		if strings.TrimSpace(cfg.APIKey) != "" {
			return nil, fmt.Errorf("api-key validation failed: %w. Paid access requires online validation", err)
		}
		return nil, err
	}
	_ = licenseprovider.SaveState(licenseprovider.State{
		FingerprintHash: fingerprintHash,
		LastCheckAt:     time.Now(),
		LastDecision:    boolLabel(result.Allowed),
		LastMode:        string(result.AccessMode),
	})
	if err := licenseprovider.ValidateCheckResult(result, checkReq); err != nil {
		return nil, fmt.Errorf("license proof validation failed: %w", err)
	}
	if !result.Allowed {
		fallback := "Free quota is exhausted. Add license.api_key to the config file and try again."
		if result.ReasonCode == "hosted_credit_exhausted" {
			fallback = "Hosted credits are exhausted. Please top up credits first."
		}
		if strings.TrimSpace(cfg.APIKey) != "" || result.ReasonCode == "paid_quota_exhausted" {
			fallback = "The current key is out of quota. Replace it or purchase more quota."
		}
		return nil, fmt.Errorf("%s", fallbackMessage(result.Message, fallback))
	}
	if result.AccessMode == LicenseAccessModeFree && strings.TrimSpace(result.Message) == "" {
		result.Message = fmt.Sprintf("Current mode: free. %d document generations remaining.", result.FreeRemaining)
	}
	if result.AccessMode == LicenseAccessModeReward && strings.TrimSpace(result.Message) == "" {
		result.Message = fmt.Sprintf("Current mode: reward. %d document generations remaining.", result.RewardRemaining)
	}
	if result.AccessMode == LicenseAccessModePaid && strings.TrimSpace(result.Message) == "" && result.PaidQuotaTotal > 0 {
		result.Message = fmt.Sprintf("Current mode: paid. %d document generations remaining.", result.PaidQuotaRemaining)
	}
	if result.AccessMode == LicenseAccessModeHosted && strings.TrimSpace(result.Message) == "" {
		result.Message = fmt.Sprintf("Current mode: hosted. %d credits remaining.", result.CreditBalance)
	}
	return result, nil
}

func displayAccessMode(mode LicenseAccessMode) string {
	switch mode {
	case LicenseAccessModeDisabled:
		return "disabled"
	default:
		return string(mode)
	}
}

func fallbackMessage(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func boolLabel(value bool) string {
	if value {
		return "allowed"
	}
	return "blocked"
}

func (a *App) completeBestMode(ctx context.Context, llm engine.LLMClient, prompter Prompter, job GenerateJob, isTTY bool, progress engine.ProgressEmitter) (GenerateJob, error) {
	if !isTTY {
		return job, fmt.Errorf("best mode requires interactive follow-up questions. Run in a TTY or switch to --mode fast")
	}
	return a.completeBestModeWithPrompter(ctx, llm, prompter, job, progress)
}

func (a *App) completeBestModeWithPrompter(ctx context.Context, llm engine.LLMClient, prompter Prompter, job GenerateJob, progress engine.ProgressEmitter) (GenerateJob, error) {
	if llm == nil {
		return job, fmt.Errorf("llm client is unavailable")
	}
	store := newMemoryPlanStore()
	workflow := planengine.NewWorkflow(planengine.Options{
		PlanStore:               store,
		LLMClient:               llm,
		Clock:                   staticClock{},
		IDGenerator:             staticIDs{},
		QuestionAttemptTimeout:  30 * time.Second,
		BlueprintTimeout:        30 * time.Second,
		ExecutionAttemptTimeout: 60 * time.Second,
	})
	emitProgress(ctx, progress, progressStepPlanPrepare, "running", "Preparing the execution plan")
	session, err := workflow.PrepareExecutionPlan(ctx, engine.PrepareExecutionPlanRequest{
		DocumentType:   string(job.DocumentType),
		UserPrompt:     job.Prompt,
		GenerationMode: job.Mode,
	})
	if err != nil {
		emitProgress(ctx, progress, progressStepPlanPrepare, "failed", "Failed to prepare the execution plan")
		return job, err
	}
	a.emitPlanDebug(job, session)
	emitProgress(ctx, progress, progressStepPlanPrepare, "completed", "Execution plan prepared")
	for session != nil && session.Status == "questioning" && session.CurrentQuestion != nil {
		emitProgress(ctx, progress, progressStepQuestion, "running", "Waiting for follow-up answers")
		if pauser, ok := progress.(interface{ Pause(string) }); ok {
			pauser.Pause("Waiting for follow-up answers")
		}
		optionID, answer, err := prompter.Ask(session.CurrentQuestion.Question, optionLabels(session.CurrentQuestion), session.CurrentQuestion.AllowFreeform)
		if err != nil {
			emitProgress(ctx, progress, progressStepQuestion, "failed", "Failed to capture follow-up answers")
			return job, err
		}
		emitProgress(ctx, progress, progressStepQuestion, "completed", "Received follow-up answers")
		req := engine.AnswerExecutionPlanQuestionRequest{
			PlanID:     session.PlanID,
			QuestionID: session.CurrentQuestion.ID,
			Answer:     answer,
		}
		if optionID != "" {
			index := 0
			fmt.Sscanf(optionID, "%d", &index)
			if index >= 1 && index <= len(session.CurrentQuestion.Options) {
				req.OptionID = session.CurrentQuestion.Options[index-1].ID
				req.Answer = ""
			}
		}
		finalAnswer := len(session.Answers)+1 >= len(session.Questions)
		if finalAnswer {
			emitProgress(ctx, progress, progressStepPlanPrepare, "running", "Synthesizing the execution plan from your answers")
		}
		session, err = workflow.AnswerExecutionPlanQuestion(ctx, req)
		if err != nil {
			if finalAnswer {
				emitProgress(ctx, progress, progressStepPlanPrepare, "failed", "Failed to synthesize the execution plan from your answers")
			} else {
				emitProgress(ctx, progress, progressStepQuestion, "failed", "Failed to update the execution plan")
			}
			return job, err
		}
		if finalAnswer {
			emitProgress(ctx, progress, progressStepPlanPrepare, "completed", "Execution plan synthesized from your answers")
		}
	}
	if session != nil && session.Status != "approved" {
		emitProgress(ctx, progress, progressStepPlanConfirm, "running", "Confirming the execution plan")
		session, err = workflow.ApproveExecutionPlan(ctx, engine.ApproveExecutionPlanRequest{PlanID: session.PlanID})
		if err != nil {
			emitProgress(ctx, progress, progressStepPlanConfirm, "failed", "Failed to confirm the execution plan")
			return job, err
		}
		emitProgress(ctx, progress, progressStepPlanConfirm, "completed", "Execution plan confirmed")
	}
	if session != nil && strings.TrimSpace(session.ExecutionPrompt) != "" {
		job.Prompt = session.ExecutionPrompt
	}
	if session != nil && session.QuestionSource != "" && session.QuestionSource != "llm_dynamic" {
		message := strings.TrimSpace(session.QuestionFallbackReason)
		if message == "" {
			message = "Dynamic follow-up question generation fell back to a simpler template, so the clarification flow may be less tailored than expected."
		}
		job.Warnings = append(job.Warnings, engine.GenerateIssue{
			Code:    "WARN_PLAN_QUESTIONS_FALLBACK",
			Field:   "plan.questions",
			Message: message,
		})
	}
	return job, nil
}

func (a *App) emitPlanDebug(job GenerateJob, session *engine.PlanSession) {
	if a == nil || !job.Debug || a.Stderr == nil || session == nil {
		return
	}
	_, _ = fmt.Fprintf(
		a.Stderr,
		"[debug] best_mode question_source=%s question_error_kind=%s question_fallback_reason=%q current_question_id=%s\n",
		strings.TrimSpace(session.QuestionSource),
		strings.TrimSpace(session.QuestionErrorKind),
		strings.TrimSpace(session.QuestionFallbackReason),
		currentQuestionID(session),
	)
	if strings.TrimSpace(session.QuestionValidationRule) != "" || strings.TrimSpace(session.QuestionValidationDetail) != "" {
		_, _ = fmt.Fprintf(
			a.Stderr,
			"[debug] best_mode question_validation_rule=%s question_validation_detail=%q\n",
			strings.TrimSpace(session.QuestionValidationRule),
			strings.TrimSpace(session.QuestionValidationDetail),
		)
	}
	if strings.TrimSpace(session.QuestionRawPreview) != "" {
		_, _ = fmt.Fprintf(a.Stderr, "[debug] best_mode question_raw_preview=%s\n", strings.TrimSpace(session.QuestionRawPreview))
	}
}

func currentQuestionID(session *engine.PlanSession) string {
	if session == nil || session.CurrentQuestion == nil {
		return ""
	}
	return strings.TrimSpace(session.CurrentQuestion.ID)
}

func readStdin(stdin io.Reader) (string, bool, error) {
	file, ok := stdin.(*os.File)
	if !ok {
		data, err := io.ReadAll(stdin)
		return string(data), false, err
	}
	info, err := file.Stat()
	if err != nil {
		return "", false, err
	}
	isTTY := (info.Mode() & os.ModeCharDevice) != 0
	if isTTY {
		return "", true, nil
	}
	data, err := io.ReadAll(file)
	return string(data), false, err
}

type memoryPlanStore struct {
	sessions map[string]*engine.PlanSession
}

func newMemoryPlanStore() *memoryPlanStore {
	return &memoryPlanStore{sessions: map[string]*engine.PlanSession{}}
}

func (s *memoryPlanStore) Load(_ context.Context, planID string) (*engine.PlanSession, error) {
	session, ok := s.sessions[planID]
	if !ok {
		return nil, fmt.Errorf("plan not found: %s", planID)
	}
	cloned := *session
	cloned.Questions = append([]engine.PlanQuestion(nil), session.Questions...)
	cloned.Answers = append([]engine.PlanAnswer(nil), session.Answers...)
	return &cloned, nil
}

func (s *memoryPlanStore) Save(_ context.Context, session *engine.PlanSession, _ time.Duration) error {
	cloned := *session
	cloned.Questions = append([]engine.PlanQuestion(nil), session.Questions...)
	cloned.Answers = append([]engine.PlanAnswer(nil), session.Answers...)
	s.sessions[session.PlanID] = &cloned
	return nil
}

type staticClock struct{}

func (staticClock) Now() time.Time { return time.Now() }

type staticIDs struct{}

func (staticIDs) NewID() string { return fmt.Sprintf("plan-%d", time.Now().UnixNano()) }
