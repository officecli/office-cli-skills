package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	officeTaskPreflightSkipEnv                 = "OFFICECLI_SKIP_SKILL_PREFLIGHT"
	officeTaskPreflightSkipPublishEnv          = "OFFICECLI_SKIP_PUBLISH_SETUP"
	officeTaskPreflightSkipGenEnv              = "OFFICECLI_SKIP_GENERATION_SETUP"
	officeTaskPreflightRequireLicenseAPIKeyEnv = "OFFICECLI_REQUIRE_LICENSE_API_KEY"
)

func runInstalledSkillPreflight(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, command string, args []string) error {
	if truthy(os.Getenv(officeTaskPreflightSkipEnv)) {
		return nil
	}

	scripts, err := installedSkillPreflightScripts(command)
	if err != nil {
		return err
	}
	for _, script := range scripts {
		_, err := shouldRetryAfterScriptRefresh(script, func() error {
			cmd := exec.CommandContext(ctx, "bash", script)
			cmd.Env = append(os.Environ(), officeTaskPreflightSkipEnv+"=1")
			if shouldSkipPublishPreflight(command, args) {
				cmd.Env = append(cmd.Env, officeTaskPreflightSkipPublishEnv+"=1")
			}
			if shouldSkipGenerationPreflight(command, args) {
				cmd.Env = append(cmd.Env, officeTaskPreflightSkipGenEnv+"=1")
			}
			if shouldRequireLicenseAPIKeyPreflight(command, args) {
				cmd.Env = append(cmd.Env, officeTaskPreflightRequireLicenseAPIKeyEnv+"=1")
			}
			if isTerminalReader(stdin) {
				cmd.Stdin = stdin
			}
			stdoutFilter := newPreflightOutputFilter(stdout)
			stderrFilter := newPreflightOutputFilter(stderr)
			cmd.Stdout = stdoutFilter
			cmd.Stderr = stderrFilter
			runErr := cmd.Run()
			if flushErr := stdoutFilter.Flush(); flushErr != nil && runErr == nil {
				runErr = flushErr
			}
			if flushErr := stderrFilter.Flush(); flushErr != nil && runErr == nil {
				runErr = flushErr
			}
			if runErr != nil {
				if statusErr := preflightStatusErrorFromFilters(stdoutFilter, stderrFilter); statusErr != nil {
					return statusErr
				}
			}
			return runErr
		})
		if err == nil {
			continue
		}
		var statusErr *preflightStatusError
		if errors.As(err, &statusErr) {
			if shouldOverrideAccountLoginPreflight(statusErr) {
				continue
			}
			return statusErr
		}
		return fmt.Errorf("skill preflight failed for %s: %w", script, err)
	}
	return nil
}

// shouldOverrideAccountLoginPreflight suppresses a stale shell-script
// preflight verdict when its ONLY complaint is the user being "anonymous" but
// the binary's own config shows a valid CLI session or an API key. This guards
// against an older ~/.codex/skills/officecli/env-common.sh whose parse_auth_mode
// does not yet understand the binary's "Mode: logged in" whoami output. The
// stale fix script emits status="blocked" via fail_fix; older repairable
// verdicts are also accepted for forward-compatibility with future scripts
// that downgrade the severity.
func shouldOverrideAccountLoginPreflight(statusErr *preflightStatusError) bool {
	if statusErr == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(statusErr.payload.Status)) {
	case "repairable", "blocked":
	default:
		return false
	}
	hasAccountLogin := false
	for _, item := range statusErr.payload.MissingItems {
		switch strings.TrimSpace(item) {
		case "account_login":
			hasAccountLogin = true
		case "":
			// ignore
		default:
			return false
		}
	}
	if !hasAccountLogin {
		return false
	}
	cfg, err := LoadConfig("")
	if err != nil {
		return false
	}
	if strings.TrimSpace(cfg.License.APIKey) != "" {
		return true
	}
	if strings.TrimSpace(cfg.License.SessionToken) == "" {
		return false
	}
	if cfg.License.SessionExpiresAt == nil {
		return true
	}
	return time.Now().Before(*cfg.License.SessionExpiresAt)
}

type preflightStatusPayload struct {
	Status        string   `json:"status"`
	MissingItems  []string `json:"missing_items"`
	FailureReason string   `json:"failure_reason,omitempty"`
}

type preflightStatusError struct {
	payload preflightStatusPayload
}

func (e *preflightStatusError) Error() string {
	if e == nil {
		return ""
	}
	status := strings.ToLower(strings.TrimSpace(e.payload.Status))
	missing := formatPreflightMissingItems(e.payload.MissingItems)
	reason := strings.TrimSpace(e.payload.FailureReason)
	switch status {
	case "repairable":
		if missing != "" {
			return fmt.Sprintf("officecli setup is incomplete: missing %s", missing)
		}
		return "officecli setup is incomplete"
	case "blocked":
		var msg string
		if reason != "" && missing != "" {
			msg = fmt.Sprintf("officecli setup failed: %s (missing %s)", reason, missing)
		} else if reason != "" {
			msg = fmt.Sprintf("officecli setup failed: %s", reason)
		} else if missing != "" {
			msg = fmt.Sprintf("officecli setup failed: missing %s", missing)
		} else {
			msg = "officecli setup failed"
		}
		if isNetworkRelatedPreflightFailure(reason) {
			msg += "\nHint: set OFFICECLI_SKIP_SKILL_PREFLIGHT=1 to bypass this check if you believe your setup is correct"
		}
		return msg
	default:
		if reason != "" {
			return fmt.Sprintf("officecli preflight failed: %s", reason)
		}
		if missing != "" {
			return fmt.Sprintf("officecli preflight failed: missing %s", missing)
		}
		return "officecli preflight failed"
	}
}

type preflightOutputFilter struct {
	target  io.Writer
	pending string
	status  *preflightStatusPayload
}

func newPreflightOutputFilter(target io.Writer) *preflightOutputFilter {
	return &preflightOutputFilter{target: target}
}

func (f *preflightOutputFilter) Write(p []byte) (int, error) {
	if f == nil || f.target == nil {
		return len(p), nil
	}
	f.pending += string(p)
	for {
		idx := strings.IndexByte(f.pending, '\n')
		if idx < 0 {
			break
		}
		line := f.pending[:idx]
		f.pending = f.pending[idx+1:]
		if err := f.writeLine(line + "\n"); err != nil {
			return 0, err
		}
	}
	if f.pending != "" &&
		!looksLikeSuppressedPreflightPrefix(f.pending) &&
		!looksLikePreflightJSONFragment(f.pending) {
		if _, err := io.WriteString(f.target, f.pending); err != nil {
			return 0, err
		}
		f.pending = ""
	}
	return len(p), nil
}

func (f *preflightOutputFilter) Flush() error {
	if f == nil || f.target == nil || f.pending == "" {
		return nil
	}
	if err := f.writeLine(f.pending); err == nil {
		f.pending = ""
		return nil
	}
	if shouldSuppressPreflightLine(f.pending) ||
		looksLikeSuppressedPreflightPrefix(f.pending) ||
		looksLikePreflightJSONFragment(f.pending) {
		f.pending = ""
		return nil
	}
	_, err := io.WriteString(f.target, f.pending)
	f.pending = ""
	return err
}

func (f *preflightOutputFilter) writeLine(line string) error {
	if f == nil {
		return nil
	}
	if payload, ok := parsePreflightStatusPayload(line); ok {
		f.status = &payload
		return nil
	}
	if shouldSuppressPreflightLine(line) {
		return nil
	}
	if _, err := io.WriteString(f.target, line); err != nil {
		return err
	}
	return nil
}

func (f *preflightOutputFilter) Status() *preflightStatusPayload {
	if f == nil || f.status == nil {
		return nil
	}
	payload := *f.status
	payload.MissingItems = append([]string(nil), payload.MissingItems...)
	return &payload
}

func shouldSuppressPreflightLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if payload, ok := parsePreflightStatusPayload(trimmed); ok {
		return strings.EqualFold(strings.TrimSpace(payload.Status), "ready")
	}
	for _, prefix := range suppressedPreflightPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func looksLikeSuppressedPreflightPrefix(fragment string) bool {
	trimmed := strings.TrimSpace(fragment)
	if trimmed == "" {
		return false
	}
	for _, prefix := range suppressedPreflightPrefixes {
		if strings.HasPrefix(prefix, trimmed) || strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func looksLikePreflightJSONFragment(fragment string) bool {
	trimmed := strings.TrimSpace(fragment)
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "{")
}

func parsePreflightStatusPayload(line string) (preflightStatusPayload, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return preflightStatusPayload{}, false
	}
	var payload preflightStatusPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return preflightStatusPayload{}, false
	}
	if strings.TrimSpace(payload.Status) == "" {
		return preflightStatusPayload{}, false
	}
	return payload, true
}

func preflightStatusErrorFromFilters(filters ...*preflightOutputFilter) *preflightStatusError {
	for _, filter := range filters {
		if filter == nil {
			continue
		}
		if payload := filter.Status(); payload != nil && !strings.EqualFold(strings.TrimSpace(payload.Status), "ready") {
			return &preflightStatusError{payload: *payload}
		}
	}
	return nil
}

func isNetworkRelatedPreflightFailure(reason string) bool {
	r := strings.ToLower(reason)
	return strings.Contains(r, "refresh") || strings.Contains(r, "network") || strings.Contains(r, "install")
}

func formatPreflightMissingItems(items []string) string {
	if len(items) == 0 {
		return ""
	}
	labels := make([]string, 0, len(items))
	for _, item := range items {
		switch strings.TrimSpace(item) {
		case "officecli_binary":
			labels = append(labels, "officecli binary")
		case "cli_surface":
			labels = append(labels, "CLI surface")
		case "generation_config":
			labels = append(labels, "generation config")
		case "license_config":
			labels = append(labels, "access config")
		case "publish_config":
			labels = append(labels, "publish config")
		case "agent_bridge":
			labels = append(labels, "agent bridge")
		case "account_login":
			labels = append(labels, "account login (run 'officecli login' to sign in)")
		default:
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				labels = append(labels, strings.ReplaceAll(trimmed, "_", " "))
			}
		}
	}
	return strings.Join(labels, ", ")
}

var suppressedPreflightPrefixes = []string{
	"installed skill to ",
	"installed OpenClaw skill to ",
	"skipped officecli binary auto-install",
	"officecli binary already available:",
	"installed officecli binary:",
	"officecli version before refresh:",
	"officecli version after refresh:",
	"restart your client to pick up the refreshed skill",
	"warning: curl not found, skipped officecli binary auto-install",
	"warning: failed to auto-install officecli binary from public dist",
	"warning: Homebrew not found, falling back to direct binary install",
	"warning: brew install failed, falling back to direct binary install",
	"note: add ",
}

func shouldSkipPublishPreflight(command string, args []string) bool {
	if command != "new" {
		return false
	}
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "--no-publish" || strings.HasPrefix(trimmed, "--no-publish=") {
			return true
		}
	}
	return false
}

func shouldSkipGenerationPreflight(command string, args []string) bool {
	return command == "new" && isImageNewCommand(args)
}

func shouldRequireLicenseAPIKeyPreflight(command string, args []string) bool {
	return command == "new" && isImageNewCommand(args)
}

func isImageNewCommand(args []string) bool {
	documentType := strings.ToLower(strings.TrimSpace(firstNewDocumentTypeArg(args)))
	return documentType == "img" || documentType == "image" || documentType == "gif"
}

func firstNewDocumentTypeArg(args []string) string {
	for i := 0; i < len(args); i++ {
		current := strings.TrimSpace(args[i])
		if current == "" {
			continue
		}
		switch current {
		case "--prompt", "--prompt-file", "--mode", "--runtime-mode", "--lang", "--style", "--audience", "--out", "--file", "--ratio", "--reference-image", "--reference-root", "--reference-pptx", "--pptx-backend", "--fail-below":
			i++
			continue
		case "--json", "--publish", "--no-publish", "--no-images", "--no-reference-scan", "--no-visual", "--local-preview", "--debug":
			continue
		}
		if strings.HasPrefix(current, "--") {
			continue
		}
		return current
	}
	return ""
}

func shouldRetryAfterScriptRefresh(script string, run func() error) (bool, error) {
	before, _ := os.Stat(script)
	err := run()
	if err == nil {
		return false, nil
	}
	after, statErr := os.Stat(script)
	if statErr != nil || before == nil || after == nil {
		return false, err
	}
	if sameFileInfo(before, after) {
		return false, err
	}
	if retryErr := run(); retryErr != nil {
		return true, retryErr
	}
	return true, nil
}

func sameFileInfo(before, after os.FileInfo) bool {
	if before == nil || after == nil {
		return true
	}
	return before.Size() == after.Size() && before.ModTime().Equal(after.ModTime().Round(time.Second))
}

func installedSkillPreflightScripts(command string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil, err
	}

	candidates := []string{
		filepath.Join(home, ".codex", "skills", "officecli", "fix-officecli-env.sh"),
	}
	if command == "agent-bridge" {
		candidates = append(candidates, filepath.Join(home, ".openclaw", "skills", "openclaw-officecli", "fix-officecli-env.sh"))
	}

	scripts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		scripts = append(scripts, candidate)
	}
	return scripts, nil
}

func isTerminalReader(r io.Reader) bool {
	if detector, ok := r.(interface{ IsTerminal() bool }); ok {
		return detector.IsTerminal()
	}
	file, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
