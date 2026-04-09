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
	officeTaskPreflight func(ctx context.Context, command string) error
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
		officeTaskPreflight: func(ctx context.Context, command string) error {
			return runInstalledSkillPreflight(ctx, stdin, stdout, stderr, command)
		},
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
		case "review":
			help = ReviewHelpText()
		case "agent-bridge":
			help = AgentBridgeHelpText()
		default:
			help = HelpText()
		}
		_, err := io.WriteString(a.Stdout, help)
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
		if err := a.officeTaskPreflight(ctx, args[0]); err != nil {
			return err
		}
		cfg, err := LoadConfig("")
		if err != nil {
			return err
		}
		return a.runNew(ctx, cfg, args[1:])
	case "review":
		cfg, err := LoadConfig("")
		if err != nil {
			return err
		}
		return a.runReview(ctx, cfg, args[1:])
	case "agent-bridge":
		if err := a.officeTaskPreflight(ctx, args[0]); err != nil {
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
			return Config{}, fmt.Errorf("缺少必填环境变量，请先补全平台访问凭证，或改用 `officecli config set-license`")
		}
		return Config{}, fmt.Errorf("缺少必填环境变量，请先补全生成服务配置，或改用 `officecli config set-generation`")
	}
	if cfg.Publish.Enabled {
		if strings.TrimSpace(cfg.Publish.BaseURL) == "" {
			return Config{}, errors.New("缺少必填环境变量，请补全在线预览发布服务地址")
		}
		if strings.TrimSpace(cfg.Publish.APIKey) == "" {
			return Config{}, errors.New("缺少必填环境变量，请补全在线预览发布访问凭证")
		}
	}
	cfg.Defaults.Publish = cfg.Publish.Enabled
	return cfg, nil
}

func defaultInitConfig() Config {
	return Config{
		Defaults: DefaultsConfig{
			OutputDir: "./output",
			Mode:      "fast",
			Publish:   true,
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
			BaseURL:    "https://your-publish-service.example.com/api",
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

用自然语言生成 Office 文档的命令行工具。

子命令：
  config                  查看或更新本地配置
  auth                    查看或设置授权信息
  new                     生成新的 PPTX / DOCX / XLSX 文件
  review                  评估本地 PPTX 文件质量
  agent-bridge            通过 JSON-RPC over stdio 提供 agent 接口

用法：
  officecli new <pptx|docx|xlsx> <topic> [brief]
  officecli config status
  officecli auth status
  officecli auth set-key <api-key>
  officecli review pptx ./deck.pptx

常用选项：
  --prompt <text>         直接提供完整提示词
  --prompt-file <path>    从文件读取提示词
  --mode fast|best        选择快速生成或补问增强模式
  --lang <value>          指定语言
  --style <value>         指定风格
  --audience <value>      指定受众
  --out <dir>             指定输出目录
  --publish               强制发布在线预览
  --no-publish            禁止发布在线预览
  --no-images             关闭 PPT 自动配图
  --json                  输出 JSON 结果
  --version               显示当前版本

默认行为：
  - 默认输出目录：./output
  - 默认模式：fast
  - 若已接入额度服务，则生成前会先校验可用额度
  - 如果 defaults.publish=true 且发布端已配置，则生成后自动发布
  - 如果发布端未配置，则只保存本地文件并提示跳过在线预览

配置文件：
  macOS   ~/Library/Application Support/officecli/config.json
  Linux   ~/.config/officecli/config.json
  Windows %AppData%\officecli\config.json

示例：
  officecli config status
  officecli config set-generation
  officecli config set-license
  officecli config set-publish
  officecli config set-defaults
  officecli auth status
  officecli auth --help
  officecli auth set-key <your-api-key>
  officecli new pptx "企业协作平台介绍" "介绍这款企业协作平台的产品能力、客户价值与应用场景"
  officecli review pptx ./output/企业协作平台介绍.pptx
  officecli new --help
  officecli review --help
  officecli new docx "季度复盘" --prompt-file ./examples/prompt.txt
  officecli new xlsx "销售分析表" --json
  officecli --version
`
}

func ConfigHelpText() string {
	return `用法：
  officecli config status
  officecli config set-generation
  officecli config set-license
  officecli config set-publish
  officecli config set-defaults

说明：
  查看当前配置状态，或分别更新生成服务、额度服务、在线预览发布和默认值。
`
}

func AuthHelpText() string {
	return `用法：
  officecli auth status
  officecli auth set-key <api-key>

说明：
  查看额度状态，或写入付费额度密钥。
`
}

func NewHelpText() string {
	return `用法：
  officecli new <pptx|docx|xlsx> <topic> [brief]

常用选项：
  --prompt <text>         直接提供完整提示词
  --prompt-file <path>    从文件读取提示词
  --mode fast|best        选择快速生成或补问增强模式
  --lang <value>          指定语言
  --style <value>         指定风格
  --audience <value>      指定受众
  --out <dir>             指定输出目录
  --publish               强制发布在线预览
  --no-publish            禁止发布在线预览
  --no-images             关闭 PPT 自动配图
  --json                  输出 JSON 结果
`
}

func ReviewHelpText() string {
	return `用法：
  officecli review pptx <file>

常用选项：
  --json                  输出 JSON 结果
  --no-visual             只执行结构检查，不调用视觉评审
  --fail-below <0-100>    当总分低于阈值时返回非零退出码
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

func (a *App) runConfigStatus(cfg Config) error {
	configPath := ResolveConfigPath("")
	if configPath == "" {
		return fmt.Errorf("unable to resolve config path")
	}
	if _, err := fmt.Fprintf(a.Stdout, "配置文件路径：%s\n", configPath); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "生成服务已配置：%t\n", hasGenerationConfig(cfg)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "额度校验已启用：%t\n", cfg.License.Enabled); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "付费额度密钥已配置：%t\n", strings.TrimSpace(cfg.License.APIKey) != ""); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "在线预览发布已启用：%t\n", cfg.Publish.Enabled); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "默认输出目录：%s\n", fallbackString(cfg.Defaults.OutputDir, "./output")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "默认生成模式：%s\n", fallbackString(cfg.Defaults.Mode, "fast")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "视觉评审模型：%s\n", fallbackString(cfg.LLM.ReviewModel, "gpt-5.4-mini")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "生成后默认发布：%t\n", cfg.Defaults.Publish); err != nil {
		return err
	}
	return nil
}

func (a *App) runConfigSetGeneration(cfg Config) error {
	reader := bufio.NewReader(a.Stdin)
	var err error
	if cfg.LLM.BaseURL, err = a.promptRequiredLine(reader, "请输入生成服务地址", cfg.LLM.BaseURL); err != nil {
		return err
	}
	if cfg.LLM.APIKey, err = a.promptRequiredLine(reader, "请输入生成服务访问凭证", cfg.LLM.APIKey); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.LLM.Model) == "" {
		cfg.LLM.Model = defaultInitConfig().LLM.Model
	}
	path, err := WriteConfig("", cfg, true)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Stdout, "已更新生成服务配置：%s\n", path)
	return err
}

func (a *App) runConfigSetLicense(cfg Config) error {
	reader := bufio.NewReader(a.Stdin)
	var err error
	cfg.License.Enabled, err = a.promptYesNo(reader, "是否启用额度校验？(yes/no)", cfg.License.Enabled)
	if err != nil {
		return err
	}
	cfg.License.BaseURL = defaultInitConfig().License.BaseURL
	if cfg.License.Enabled {
		if cfg.License.APIKey, err = a.promptLine(reader, "请输入付费额度密钥（可留空，默认先走免费额度）", cfg.License.APIKey); err != nil {
			return err
		}
	} else {
		cfg.License.APIKey = ""
	}
	path, err := WriteConfig("", cfg, true)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Stdout, "已更新额度配置：%s\n", path)
	return err
}

func (a *App) runConfigSetPublish(cfg Config) error {
	reader := bufio.NewReader(a.Stdin)
	var err error
	cfg.Publish.Enabled, err = a.promptYesNo(reader, "是否启用在线预览发布？(yes/no)", cfg.Publish.Enabled)
	if err != nil {
		return err
	}
	cfg.Defaults.Publish = cfg.Publish.Enabled
	if cfg.Publish.Enabled {
		if cfg.Publish.BaseURL, err = a.promptRequiredLine(reader, "请输入发布服务地址", cfg.Publish.BaseURL); err != nil {
			return err
		}
		if cfg.Publish.APIKey, err = a.promptRequiredLine(reader, "请输入发布服务访问凭证", cfg.Publish.APIKey); err != nil {
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
	_, err = fmt.Fprintf(a.Stdout, "已更新在线预览发布配置：%s\n", path)
	return err
}

func (a *App) runConfigSetDefaults(cfg Config) error {
	reader := bufio.NewReader(a.Stdin)
	var err error
	if cfg.Defaults.OutputDir, err = a.promptLine(reader, "请输入默认输出目录", fallbackString(cfg.Defaults.OutputDir, "./output")); err != nil {
		return err
	}
	modeChoice, err := a.promptChoice(reader, "请选择默认生成模式", []string{"fast", "best"})
	if err != nil {
		return err
	}
	if modeChoice == 2 {
		cfg.Defaults.Mode = "best"
	} else {
		cfg.Defaults.Mode = "fast"
	}
	cfg.Defaults.Publish, err = a.promptYesNo(reader, "生成后默认发布在线预览？(yes/no)", cfg.Defaults.Publish)
	if err != nil {
		return err
	}
	path, err := WriteConfig("", cfg, true)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Stdout, "已更新默认配置：%s\n", path)
	return err
}

func hasGenerationConfig(cfg Config) bool {
	return strings.TrimSpace(cfg.LLM.BaseURL) != "" &&
		strings.TrimSpace(cfg.LLM.APIKey) != "" &&
		strings.TrimSpace(cfg.LLM.Model) != ""
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
		return "生成服务地址"
	case strings.TrimSpace(cfg.LLM.APIKey) == "":
		return "生成服务访问凭证"
	case strings.TrimSpace(cfg.LLM.Model) == "":
		return "生成能力默认配置"
	default:
		return ""
	}
}

func missingHostedConfig(cfg Config) string {
	switch {
	case strings.TrimSpace(cfg.License.BaseURL) == "":
		return "平台服务地址"
	case strings.TrimSpace(cfg.License.APIKey) == "":
		return "平台访问凭证"
	default:
		return ""
	}
}

func (a *App) collectInitConfig(reader *bufio.Reader, base Config) (Config, error) {
	cfg := mergeInitBaseConfig(defaultInitConfig(), base)

	var err error
	if cfg.Runtime.Mode == RuntimeModeExternal {
		if cfg.LLM.BaseURL, err = a.promptRequiredLine(reader, "请输入生成服务地址", cfg.LLM.BaseURL); err != nil {
			return Config{}, err
		}
		if cfg.LLM.APIKey, err = a.promptRequiredLine(reader, "请输入生成服务访问凭证", cfg.LLM.APIKey); err != nil {
			return Config{}, err
		}
		if strings.TrimSpace(cfg.LLM.Model) == "" {
			cfg.LLM.Model = defaultInitConfig().LLM.Model
		}
	}
	if cfg.License.APIKey, err = a.promptLine(reader, "请输入付费额度密钥（可留空，默认先走免费额度）", cfg.License.APIKey); err != nil {
		return Config{}, err
	}
	cfg.Publish.Enabled, err = a.promptYesNo(reader, "是否启用在线预览发布？(yes/no)", cfg.Publish.Enabled)
	if err != nil {
		return Config{}, err
	}
	cfg.Defaults.Publish = cfg.Publish.Enabled
	if cfg.Publish.Enabled {
		if cfg.Publish.BaseURL, err = a.promptRequiredLine(reader, "请输入发布服务地址", cfg.Publish.BaseURL); err != nil {
			return Config{}, err
		}
		if cfg.Publish.APIKey, err = a.promptRequiredLine(reader, "请输入发布服务访问凭证", cfg.Publish.APIKey); err != nil {
			return Config{}, err
		}
	} else {
		cfg.Publish.BaseURL = ""
		cfg.Publish.APIKey = ""
	}
	if outputDir, err := a.promptLine(reader, "请输入默认输出目录", cfg.Defaults.OutputDir); err == nil {
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
		if _, err := fmt.Fprintln(a.Stdout, "该字段不能为空，请重新输入。"); err != nil {
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
			if _, err := fmt.Fprintln(a.Stdout, "请输入 yes 或 no。"); err != nil {
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
		if _, err := fmt.Fprintln(a.Stdout, "请输入有效编号。"); err != nil {
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
	defer progress.Close()
	result, err := a.executeGenerateJob(ctx, cfg, job, isTTY, progress, nil)
	if err != nil {
		return err
	}
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
			key, err := a.promptLine(bufio.NewReader(a.Stdin), "请输入付费额度密钥", "")
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
	if _, err := fmt.Fprintf(a.Stdout, "当前授权状态：%s\n", displayAccessMode(result.AccessMode)); err != nil {
		return err
	}
	if result.PlanName != "" {
		if _, err := fmt.Fprintf(a.Stdout, "当前套餐：%s\n", result.PlanName); err != nil {
			return err
		}
	}
	if result.AccessMode == LicenseAccessModeFree {
		if _, err := fmt.Fprintf(a.Stdout, "剩余免费次数：%d\n", result.FreeRemaining); err != nil {
			return err
		}
	}
	if result.AccessMode == LicenseAccessModeReward {
		if _, err := fmt.Fprintf(a.Stdout, "剩余奖励次数：%d\n", result.RewardRemaining); err != nil {
			return err
		}
	}
	if result.AccessMode == LicenseAccessModePaid {
		if _, err := fmt.Fprintf(a.Stdout, "剩余付费次数：%d\n", result.PaidQuotaRemaining); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(a.Stdout, "额度校验已启用：%t\n", cfg.License.Enabled); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(a.Stdout, "付费额度密钥已配置：%t\n", strings.TrimSpace(cfg.License.APIKey) != ""); err != nil {
		return err
	}
	if strings.TrimSpace(result.Message) != "" {
		if _, err := fmt.Fprintln(a.Stdout, result.Message); err != nil {
			return err
		}
	}
	return nil
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
		return fmt.Errorf("付费额度密钥校验失败：%s", fallbackMessage(result.Message, "密钥无效、已过期、次数已耗尽或额度服务不可用"))
	}
	if _, err := WriteConfig("", cfg, true); err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.Stdout, "已写入付费额度密钥，当前授权状态：%s\n", displayAccessMode(result.AccessMode))
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
			Message:    "当前未接入额度校验服务。",
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
			return nil, fmt.Errorf("api-key 校验失败：%w。当前付费模式要求在线校验", err)
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
		return nil, fmt.Errorf("license proof 校验失败：%w", err)
	}
	if !result.Allowed {
		fallback := "免费额度已用完，请在配置文件中填写 license.api_key 后重试。"
		if result.ReasonCode == "hosted_credit_exhausted" {
			fallback = "当前托管 credits 已耗尽，请先充值 credits。"
		}
		if strings.TrimSpace(cfg.APIKey) != "" || result.ReasonCode == "paid_quota_exhausted" {
			fallback = "当前 key 次数已耗尽，请更换或充值次数包。"
		}
		return nil, fmt.Errorf("%s", fallbackMessage(result.Message, fallback))
	}
	if result.AccessMode == LicenseAccessModeFree && strings.TrimSpace(result.Message) == "" {
		result.Message = fmt.Sprintf("当前为免费模式，剩余 %d 次生成额度。", result.FreeRemaining)
	}
	if result.AccessMode == LicenseAccessModeReward && strings.TrimSpace(result.Message) == "" {
		result.Message = fmt.Sprintf("当前为奖励模式，剩余 %d 次生成额度。", result.RewardRemaining)
	}
	if result.AccessMode == LicenseAccessModePaid && strings.TrimSpace(result.Message) == "" && result.PaidQuotaTotal > 0 {
		result.Message = fmt.Sprintf("当前为付费模式，剩余 %d 次生成额度。", result.PaidQuotaRemaining)
	}
	if result.AccessMode == LicenseAccessModeHosted && strings.TrimSpace(result.Message) == "" {
		result.Message = fmt.Sprintf("当前为托管模式，剩余 %d credits。", result.CreditBalance)
	}
	return result, nil
}

func displayAccessMode(mode LicenseAccessMode) string {
	switch mode {
	case LicenseAccessModeDisabled:
		return "未启用"
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
		return job, fmt.Errorf("best 模式需要交互补问，请在 TTY 中运行或改用 --mode fast")
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
	emitProgress(ctx, progress, progressStepPlanPrepare, "running", "正在准备生成计划")
	session, err := workflow.PrepareExecutionPlan(ctx, engine.PrepareExecutionPlanRequest{
		DocumentType:   string(job.DocumentType),
		UserPrompt:     job.Prompt,
		GenerationMode: job.Mode,
	})
	if err != nil {
		emitProgress(ctx, progress, progressStepPlanPrepare, "failed", "生成计划准备失败")
		return job, err
	}
	emitProgress(ctx, progress, progressStepPlanPrepare, "completed", "生成计划准备完成")
	for session != nil && session.Status == "questioning" && session.CurrentQuestion != nil {
		emitProgress(ctx, progress, progressStepQuestion, "running", "正在等待你回答补充问题")
		if pauser, ok := progress.(interface{ Pause(string) }); ok {
			pauser.Pause("等待你回答补充问题")
		}
		optionID, answer, err := prompter.Ask(session.CurrentQuestion.Question, optionLabels(session.CurrentQuestion), session.CurrentQuestion.AllowFreeform)
		if err != nil {
			emitProgress(ctx, progress, progressStepQuestion, "failed", "补充问题回答失败")
			return job, err
		}
		emitProgress(ctx, progress, progressStepQuestion, "completed", "已收到补充问题答案")
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
		session, err = workflow.AnswerExecutionPlanQuestion(ctx, req)
		if err != nil {
			emitProgress(ctx, progress, progressStepQuestion, "failed", "更新生成计划失败")
			return job, err
		}
	}
	if session != nil && session.Status != "approved" {
		emitProgress(ctx, progress, progressStepPlanConfirm, "running", "正在确认生成计划")
		session, err = workflow.ApproveExecutionPlan(ctx, engine.ApproveExecutionPlanRequest{PlanID: session.PlanID})
		if err != nil {
			emitProgress(ctx, progress, progressStepPlanConfirm, "failed", "确认生成计划失败")
			return job, err
		}
		emitProgress(ctx, progress, progressStepPlanConfirm, "completed", "生成计划确认完成")
	}
	if session != nil && strings.TrimSpace(session.ExecutionPrompt) != "" {
		job.Prompt = session.ExecutionPrompt
	}
	return job, nil
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
