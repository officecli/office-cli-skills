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
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			return cmd.Run()
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
