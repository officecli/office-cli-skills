package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	officeTaskPreflightSkipEnv        = "OFFICECLI_SKIP_SKILL_PREFLIGHT"
	officeTaskPreflightSkipPublishEnv = "OFFICECLI_SKIP_PUBLISH_SETUP"
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
		retry, err := shouldRetryAfterScriptRefresh(script, func() error {
			cmd := exec.CommandContext(ctx, "bash", script)
			cmd.Env = append(os.Environ(), officeTaskPreflightSkipEnv+"=1")
			if shouldSkipPublishPreflight(command, args) {
				cmd.Env = append(cmd.Env, officeTaskPreflightSkipPublishEnv+"=1")
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
			return runErr
		})
		if err == nil {
			continue
		}
		if retry {
			continue
		}
		if err != nil {
			return fmt.Errorf("skill preflight failed for %s: %w", script, err)
		}
	}
	return nil
}

type preflightOutputFilter struct {
	target  io.Writer
	pending string
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
		if shouldSuppressPreflightLine(line) {
			continue
		}
		if _, err := io.WriteString(f.target, line+"\n"); err != nil {
			return 0, err
		}
	}
	if f.pending != "" && !looksLikeSuppressedPreflightPrefix(f.pending) {
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
	if shouldSuppressPreflightLine(f.pending) || looksLikeSuppressedPreflightPrefix(f.pending) {
		f.pending = ""
		return nil
	}
	_, err := io.WriteString(f.target, f.pending)
	f.pending = ""
	return err
}

func shouldSuppressPreflightLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
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
