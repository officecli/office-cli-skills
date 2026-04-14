package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	updateCheckSkipEnv      = "OFFICECLI_SKIP_UPDATE_CHECK"
	defaultDistRepo         = "officecli/officecli-dist"
	defaultDistBranch       = "main"
	defaultUpdateCheckURL   = "https://api.github.com/repos/%s/releases/latest"
	defaultInstallScriptURL = "https://raw.githubusercontent.com/%s/%s/scripts/install-officecli.sh"
)

type InstallMethod string

const (
	InstallMethodUnknown InstallMethod = "unknown"
	InstallMethodBrew    InstallMethod = "brew"
	InstallMethodScript  InstallMethod = "script"
)

type UpdateChannel string

const (
	UpdateChannelUnknown UpdateChannel = "unknown"
	UpdateChannelBrew    UpdateChannel = "brew"
	UpdateChannelLatest  UpdateChannel = "latest"
)

type UpdateInfo struct {
	Available           bool          `json:"available"`
	CurrentVersion      string        `json:"current_version,omitempty"`
	CurrentCommit       string        `json:"current_commit,omitempty"`
	CurrentBuildDate    string        `json:"current_build_date,omitempty"`
	LatestVersionLabel  string        `json:"latest_version_label,omitempty"`
	LatestPublishedAt   string        `json:"latest_published_at,omitempty"`
	Channel             UpdateChannel `json:"channel,omitempty"`
	InstallMethod       InstallMethod `json:"install_method,omitempty"`
	AutoUpdateSupported bool          `json:"auto_update_supported"`
	UpdateCommand       string        `json:"update_command,omitempty"`
	CheckError          string        `json:"check_error,omitempty"`
	InstallDir          string        `json:"-"`
	Prefix              string        `json:"-"`
}

type releaseLookup struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
}

type brewOutdatedResult struct {
	Formulae []struct {
		Name              string   `json:"name"`
		InstalledVersions []string `json:"installed_versions"`
		CurrentVersion    string   `json:"current_version"`
	} `json:"formulae"`
}

func (a *App) maybeHandleUpdate(ctx context.Context, args []string) error {
	if !shouldCheckForUpdates(a, args) {
		return nil
	}
	info, err := a.safeCheckForUpdates(ctx)
	if err != nil || !info.Available {
		return nil
	}
	if _, err := fmt.Fprintf(a.Stdout, "Update available for officecli: current %s, latest %s.\n", fallbackString(info.CurrentVersion, "unknown"), fallbackString(info.LatestVersionLabel, "latest")); err != nil {
		return err
	}
	if strings.TrimSpace(info.UpdateCommand) != "" {
		if _, err := fmt.Fprintf(a.Stdout, "Suggested update command: %s\n", info.UpdateCommand); err != nil {
			return err
		}
	}
	reader := bufio.NewReader(a.Stdin)
	confirm, err := a.promptYesNo(reader, "Update now and continue with the current command? (yes/no)", true)
	if err != nil {
		return err
	}
	if !confirm {
		return nil
	}
	if a.performUpdate == nil {
		return nil
	}
	if err := a.performUpdate(ctx, info); err != nil {
		if _, writeErr := fmt.Fprintf(a.Stderr, "Automatic update failed, continuing with the current command: %v\n", err); writeErr != nil {
			return writeErr
		}
		return nil
	}
	if _, err := fmt.Fprintln(a.Stdout, "officecli was updated. Continuing with the current command..."); err != nil {
		return err
	}
	if a.restartCommand == nil {
		return nil
	}
	return a.restartCommand(ctx, args)
}

func (a *App) safeCheckForUpdates(ctx context.Context) (UpdateInfo, error) {
	if truthy(os.Getenv(updateCheckSkipEnv)) {
		return UpdateInfo{
			CurrentVersion:   Version,
			CurrentCommit:    Commit,
			CurrentBuildDate: BuildDate,
		}, nil
	}
	if a == nil || a.checkForUpdates == nil {
		return UpdateInfo{}, nil
	}
	info, err := a.checkForUpdates(ctx)
	if err != nil {
		return UpdateInfo{}, err
	}
	info.CurrentVersion = fallbackString(info.CurrentVersion, Version)
	info.CurrentCommit = fallbackString(info.CurrentCommit, Commit)
	info.CurrentBuildDate = fallbackString(info.CurrentBuildDate, BuildDate)
	return info, nil
}

func shouldCheckForUpdates(a *App, args []string) bool {
	if a == nil || len(args) == 0 {
		return false
	}
	if truthy(os.Getenv(updateCheckSkipEnv)) {
		return false
	}
	if Version == "dev" || strings.EqualFold(strings.TrimSpace(BuildDate), "unknown") {
		return false
	}
	if !isTerminalWriter(a.Stdout) || !isTerminalReader(a.Stdin) {
		return false
	}
	if isHelpArg(args[0]) || isVersionArg(args[0]) || args[0] == "agent-bridge" {
		return false
	}
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "--json" || strings.HasPrefix(trimmed, "--json=") {
			return false
		}
	}
	return true
}

func defaultCheckForUpdates(ctx context.Context) (UpdateInfo, error) {
	execPath, err := os.Executable()
	if err != nil {
		return UpdateInfo{}, err
	}
	method, err := detectInstallMethod(execPath)
	if err != nil {
		return UpdateInfo{}, err
	}
	switch method {
	case InstallMethodBrew:
		return checkBrewForUpdates(ctx)
	case InstallMethodScript:
		return checkLatestReleaseForUpdates(ctx, execPath)
	default:
		info, err := checkLatestReleaseForUpdates(ctx, execPath)
		if err != nil {
			return UpdateInfo{
				InstallMethod:       InstallMethodUnknown,
				Channel:             UpdateChannelUnknown,
				CurrentVersion:      Version,
				CurrentCommit:       Commit,
				CurrentBuildDate:    BuildDate,
				AutoUpdateSupported: false,
				CheckError:          err.Error(),
			}, nil
		}
		info.InstallMethod = InstallMethodUnknown
		info.Channel = UpdateChannelUnknown
		info.AutoUpdateSupported = false
		return info, nil
	}
}

func detectInstallMethod(execPath string) (InstallMethod, error) {
	resolved := strings.TrimSpace(execPath)
	if resolved == "" {
		return InstallMethodUnknown, nil
	}
	if realPath, err := filepath.EvalSymlinks(resolved); err == nil && strings.TrimSpace(realPath) != "" {
		resolved = realPath
	}
	if isBrewInstall(resolved) {
		return InstallMethodBrew, nil
	}
	if isScriptInstall(resolved) {
		return InstallMethodScript, nil
	}
	return InstallMethodUnknown, nil
}

func isBrewInstall(execPath string) bool {
	lower := filepath.ToSlash(strings.ToLower(strings.TrimSpace(execPath)))
	return strings.Contains(lower, "/cellar/officecli/") || strings.Contains(lower, "/homebrew/")
}

func isScriptInstall(execPath string) bool {
	cleaned := filepath.Clean(strings.TrimSpace(execPath))
	if cleaned == "" {
		return false
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "officecli"),
		"/usr/local/bin/officecli",
	}
	for _, candidate := range candidates {
		if candidate != "" && filepath.Clean(candidate) == cleaned {
			return true
		}
	}
	return false
}

func checkBrewForUpdates(ctx context.Context) (UpdateInfo, error) {
	info := UpdateInfo{
		InstallMethod:       InstallMethodBrew,
		Channel:             UpdateChannelBrew,
		CurrentVersion:      Version,
		CurrentCommit:       Commit,
		CurrentBuildDate:    BuildDate,
		AutoUpdateSupported: true,
		UpdateCommand:       "brew upgrade officecli",
	}
	cmd := exec.CommandContext(ctx, "brew", "outdated", "--json=v2", "officecli")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return UpdateInfo{}, fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return UpdateInfo{}, err
	}
	var parsed brewOutdatedResult
	if err := json.Unmarshal(output, &parsed); err != nil {
		return UpdateInfo{}, err
	}
	if len(parsed.Formulae) == 0 {
		return info, nil
	}
	info.Available = true
	info.LatestVersionLabel = strings.TrimSpace(parsed.Formulae[0].CurrentVersion)
	return info, nil
}

func checkLatestReleaseForUpdates(ctx context.Context, execPath string) (UpdateInfo, error) {
	repo := fallbackString(os.Getenv("OFFICECLI_DIST_REPO"), defaultDistRepo)
	branch := fallbackString(os.Getenv("OFFICECLI_DIST_BRANCH"), defaultDistBranch)
	installDir, prefix := resolveInstallDirs(execPath)
	url := fmt.Sprintf(defaultUpdateCheckURL, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return UpdateInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return UpdateInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return UpdateInfo{}, fmt.Errorf("update check failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var release releaseLookup
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return UpdateInfo{}, err
	}
	info := UpdateInfo{
		InstallMethod:       InstallMethodScript,
		Channel:             UpdateChannelLatest,
		CurrentVersion:      Version,
		CurrentCommit:       Commit,
		CurrentBuildDate:    BuildDate,
		LatestVersionLabel:  fallbackString(strings.TrimSpace(release.TagName), "latest"),
		LatestPublishedAt:   strings.TrimSpace(release.PublishedAt),
		AutoUpdateSupported: true,
		InstallDir:          installDir,
		Prefix:              prefix,
		UpdateCommand: fmt.Sprintf(
			"curl -fsSL https://raw.githubusercontent.com/%s/%s/scripts/install-officecli.sh | PREFIX=%q BIN_DIR=%q INSTALL_DIR=%q DIST_REPO=%s bash",
			repo,
			branch,
			prefix,
			installDir,
			installDir,
			repo,
		),
	}
	buildTime, buildErr := time.Parse(time.RFC3339, strings.TrimSpace(BuildDate))
	publishedAt, publishErr := time.Parse(time.RFC3339, strings.TrimSpace(release.PublishedAt))
	if buildErr == nil && publishErr == nil && buildTime.Before(publishedAt) {
		info.Available = true
	}
	return info, nil
}

func defaultPerformUpdate(ctx context.Context, info UpdateInfo) error {
	switch info.InstallMethod {
	case InstallMethodBrew:
		return runInteractiveCommand(ctx, "brew", "upgrade", "officecli")
	case InstallMethodScript:
		return runInstallScriptUpdate(ctx, info)
	default:
		if strings.TrimSpace(info.UpdateCommand) == "" {
			return fmt.Errorf("the current installation method does not support automatic updates")
		}
		return fmt.Errorf("the current installation method is not recognized. Run this manually: %s", info.UpdateCommand)
	}
}

func runInstallScriptUpdate(ctx context.Context, info UpdateInfo) error {
	repo := fallbackString(os.Getenv("OFFICECLI_DIST_REPO"), defaultDistRepo)
	branch := fallbackString(os.Getenv("OFFICECLI_DIST_BRANCH"), defaultDistBranch)
	scriptURL := fmt.Sprintf(defaultInstallScriptURL, repo, branch)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scriptURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("failed to download the install script: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	scriptBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "bash")
	cmd.Stdin = strings.NewReader(string(scriptBody))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	installDir := fallbackString(info.InstallDir, filepath.Join(userHomeDirOrEmpty(), ".local", "bin"))
	prefix := fallbackString(info.Prefix, filepath.Join(userHomeDirOrEmpty(), ".local"))
	cmd.Env = append(os.Environ(),
		"DIST_REPO="+repo,
		"PREFIX="+prefix,
		"BIN_DIR="+installDir,
		"INSTALL_DIR="+installDir,
	)
	return cmd.Run()
}

func defaultRestartCommand(ctx context.Context, args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), updateCheckSkipEnv+"=1")
	return cmd.Run()
}

func runInteractiveCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func userHomeDirOrEmpty() string {
	home, _ := os.UserHomeDir()
	return strings.TrimSpace(home)
}

func resolveInstallDirs(execPath string) (string, string) {
	cleaned := filepath.Clean(strings.TrimSpace(execPath))
	if cleaned == "" {
		home := userHomeDirOrEmpty()
		return filepath.Join(home, ".local", "bin"), filepath.Join(home, ".local")
	}
	installDir := filepath.Dir(cleaned)
	prefix := filepath.Dir(installDir)
	if installDir == "." || prefix == "." {
		home := userHomeDirOrEmpty()
		return filepath.Join(home, ".local", "bin"), filepath.Join(home, ".local")
	}
	return installDir, prefix
}
