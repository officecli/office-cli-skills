package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
	updateInstallMethodEnv  = "OFFICECLI_INSTALL_METHOD"
	updatePackageManagerEnv = "OFFICECLI_PACKAGE_MANAGER"
	defaultDistRepo         = "officecli/officecli-dist"
	defaultDistBranch       = "main"
	defaultNPMPackageName   = "officecli"
	defaultNPMRegistryURL   = "https://registry.npmjs.org/%s/latest"
	defaultUpdateCheckURL   = "https://api.github.com/repos/%s/releases/latest"
	defaultInstallScriptURL = "https://raw.githubusercontent.com/%s/%s/scripts/install-officecli.sh"
)

var errCommandHandledByRestart = errors.New("command already handled by restarted process")

type InstallMethod string

const (
	InstallMethodUnknown InstallMethod = "unknown"
	InstallMethodBrew    InstallMethod = "brew"
	InstallMethodNPM     InstallMethod = "npm"
	InstallMethodScript  InstallMethod = "script"
)

type UpdateChannel string

const (
	UpdateChannelUnknown UpdateChannel = "unknown"
	UpdateChannelBrew    UpdateChannel = "brew"
	UpdateChannelNPM     UpdateChannel = "npm"
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
	PackageManager      string        `json:"package_manager,omitempty"`
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

type npmLatestLookup struct {
	Version string `json:"version"`
}

type npmRuntimeMetadata struct {
	PackageManager string `json:"packageManager"`
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
	if !info.AutoUpdateSupported || a.performUpdate == nil {
		return nil
	}
	reader := bufio.NewReader(a.Stdin)
	confirm, err := a.promptUpdateChoice(reader)
	if err != nil {
		return err
	}
	if !confirm {
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
	if err := a.restartCommand(ctx, info, args); err != nil {
		return err
	}
	return errCommandHandledByRestart
}

func (a *App) safeCheckForUpdates(ctx context.Context) (UpdateInfo, error) {
	if truthy(os.Getenv(updateCheckSkipEnv)) {
		return UpdateInfo{
			CurrentVersion:   Version,
			CurrentCommit:    Commit,
			CurrentBuildDate: BuildDate,
		}, nil
	}
	return a.checkForUpdatesNow(ctx)
}

func (a *App) checkForUpdatesNow(ctx context.Context) (UpdateInfo, error) {
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
	if isHelpArg(args[0]) || isVersionArg(args[0]) || args[0] == "agent-bridge" || args[0] == "upgrade" {
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
	case InstallMethodNPM:
		return checkNPMForUpdates(ctx)
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
	if method := installMethodFromEnv(); method != InstallMethodUnknown {
		return method, nil
	}
	paths := executablePaths(execPath)
	if len(paths) == 0 {
		return InstallMethodUnknown, nil
	}
	return detectInstallMethodFromPaths(paths), nil
}

func installMethodFromEnv() InstallMethod {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(updateInstallMethodEnv))) {
	case string(InstallMethodBrew):
		return InstallMethodBrew
	case string(InstallMethodNPM):
		return InstallMethodNPM
	case string(InstallMethodScript):
		return InstallMethodScript
	default:
		return InstallMethodUnknown
	}
}

func isBrewInstall(execPath string) bool {
	lower := filepath.ToSlash(strings.ToLower(strings.TrimSpace(execPath)))
	return strings.Contains(lower, "/cellar/officecli/") || strings.Contains(lower, "/homebrew/")
}

func isNPMInstall(execPath string) bool {
	lower := filepath.ToSlash(strings.ToLower(strings.TrimSpace(execPath)))
	if lower == "" {
		return false
	}
	if isNPMSidecarRuntime(execPath) {
		return true
	}
	if strings.Contains(lower, "/lib/node_modules/officecli/") || strings.Contains(lower, "/node_modules/officecli/") {
		return true
	}
	if strings.Contains(lower, "/node_modules/.bin/officecli") {
		return true
	}
	if strings.HasSuffix(lower, "/bin/officecli") {
		switch {
		case strings.Contains(lower, "/.nvm/versions/node/"),
			strings.Contains(lower, "/fnm/"),
			strings.Contains(lower, "/volta/bin/"),
			strings.Contains(lower, "/npm-global/"):
			return true
		}
	}
	return false
}

func isNPMSidecarRuntime(execPath string) bool {
	metadataPath, ok := npmRuntimeMetadataPath(execPath)
	if !ok {
		return false
	}
	_, err := os.Stat(metadataPath)
	return err == nil
}

func npmRuntimeMetadataPath(execPath string) (string, bool) {
	cleaned := filepath.Clean(strings.TrimSpace(execPath))
	if cleaned == "" {
		return "", false
	}
	if filepath.Base(cleaned) != "officecli" {
		return "", false
	}
	runtimeDir := filepath.Dir(cleaned)
	if filepath.Base(runtimeDir) != "runtime" {
		return "", false
	}
	packageJSONPath := filepath.Join(filepath.Dir(runtimeDir), "package.json")
	raw, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return "", false
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return "", false
	}
	if strings.TrimSpace(pkg.Name) != defaultNPMPackageName {
		return "", false
	}
	return filepath.Join(runtimeDir, "metadata.json"), true
}

func npmRuntimePackageManager(execPath string) string {
	metadataPath, ok := npmRuntimeMetadataPath(execPath)
	if !ok {
		return ""
	}
	raw, err := os.ReadFile(metadataPath)
	if err != nil {
		return ""
	}
	var metadata npmRuntimeMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(metadata.PackageManager)) {
	case "npm", "pnpm", "yarn", "bun":
		return strings.ToLower(strings.TrimSpace(metadata.PackageManager))
	default:
		return ""
	}
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

func checkNPMForUpdates(ctx context.Context) (UpdateInfo, error) {
	packageName := fallbackString(os.Getenv("OFFICECLI_NPM_PACKAGE_NAME"), defaultNPMPackageName)
	url := fmt.Sprintf(defaultNPMRegistryURL, packageName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return UpdateInfo{}, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return UpdateInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return UpdateInfo{}, fmt.Errorf("npm update check failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var latest npmLatestLookup
	if err := json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		return UpdateInfo{}, err
	}
	manager := detectedPackageManager()
	info := UpdateInfo{
		InstallMethod:      InstallMethodNPM,
		Channel:            UpdateChannelNPM,
		PackageManager:     manager,
		CurrentVersion:     Version,
		CurrentCommit:      Commit,
		CurrentBuildDate:   BuildDate,
		LatestVersionLabel: strings.TrimSpace(latest.Version),
	}
	if manager != "" {
		info.AutoUpdateSupported = true
		info.UpdateCommand = updateCommandForPackageManager(manager)
	}
	if versionIsOlder(info.CurrentVersion, info.LatestVersionLabel) {
		info.Available = true
	}
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
	if latestReleaseIsNewer(info.CurrentVersion, info.LatestVersionLabel, info.CurrentBuildDate, info.LatestPublishedAt) {
		info.Available = true
	}
	return info, nil
}

func defaultPerformUpdate(ctx context.Context, info UpdateInfo) error {
	execPath, _ := os.Executable()
	if fallbackInfo, ok := fallbackNPMUpdateInfoForExecPath(info, execPath); ok {
		info = fallbackInfo
	}
	switch info.InstallMethod {
	case InstallMethodBrew:
		return runInteractiveCommand(ctx, "brew", "upgrade", "officecli")
	case InstallMethodNPM:
		name, args := packageManagerCommand(info.PackageManager)
		if name == "" {
			return fmt.Errorf("the current npm installation does not expose a supported package manager for automatic updates")
		}
		return runInteractiveCommand(ctx, name, args...)
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

func defaultRestartCommand(ctx context.Context, info UpdateInfo, args []string) error {
	executable := ""
	if info.InstallMethod == InstallMethodNPM {
		executable = "officecli"
	} else {
		var err error
		executable, err = os.Executable()
		if err != nil {
			return err
		}
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), updateCheckSkipEnv+"=1")
	if info.InstallMethod == InstallMethodNPM {
		cmd.Env = append(cmd.Env, updateInstallMethodEnv+"="+string(InstallMethodNPM))
		if strings.TrimSpace(info.PackageManager) != "" {
			cmd.Env = append(cmd.Env, updatePackageManagerEnv+"="+strings.TrimSpace(info.PackageManager))
		}
	}
	return cmd.Run()
}

func runInteractiveCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func detectedPackageManager() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(updatePackageManagerEnv))) {
	case "pnpm":
		return "pnpm"
	case "yarn":
		return "yarn"
	case "bun":
		return "bun"
	case "npm":
		return "npm"
	default:
		paths := executablePaths("")
		if manager := detectPackageManagerFromNPMMetadata(paths); manager != "" {
			return manager
		}
		return detectPackageManagerFromPaths(paths)
	}
}

func detectPackageManagerFromExecPath(execPath string) string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(updatePackageManagerEnv))) {
	case "pnpm":
		return "pnpm"
	case "yarn":
		return "yarn"
	case "bun":
		return "bun"
	case "npm":
		return "npm"
	}
	paths := make([]string, 0, 2)
	appendPath := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		cleaned := filepath.Clean(trimmed)
		for _, existing := range paths {
			if existing == cleaned {
				return
			}
		}
		paths = append(paths, cleaned)
	}
	appendPath(execPath)
	if realPath, err := filepath.EvalSymlinks(strings.TrimSpace(execPath)); err == nil {
		appendPath(realPath)
	}
	if manager := detectPackageManagerFromNPMMetadata(paths); manager != "" {
		return manager
	}
	return detectPackageManagerFromPaths(paths)
}

func executablePaths(execPath string) []string {
	paths := make([]string, 0, 4)
	appendPath := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		cleaned := filepath.Clean(trimmed)
		for _, existing := range paths {
			if existing == cleaned {
				return
			}
		}
		paths = append(paths, cleaned)
	}
	addExecutablePath := func(value string) {
		appendPath(value)
		if realPath, err := filepath.EvalSymlinks(strings.TrimSpace(value)); err == nil {
			appendPath(realPath)
		}
	}
	if strings.TrimSpace(execPath) != "" {
		addExecutablePath(execPath)
	} else if currentExecPath, err := os.Executable(); err == nil {
		addExecutablePath(currentExecPath)
	}
	if lookedUpPath, err := exec.LookPath("officecli"); err == nil {
		addExecutablePath(lookedUpPath)
	}
	return paths
}

func detectInstallMethodFromPaths(paths []string) InstallMethod {
	for _, path := range paths {
		if isBrewInstall(path) {
			return InstallMethodBrew
		}
	}
	for _, path := range paths {
		if isNPMInstall(path) {
			return InstallMethodNPM
		}
	}
	for _, path := range paths {
		if isScriptInstall(path) {
			return InstallMethodScript
		}
	}
	return InstallMethodUnknown
}

func detectPackageManagerFromPaths(paths []string) string {
	for _, path := range paths {
		lower := filepath.ToSlash(strings.ToLower(strings.TrimSpace(path)))
		switch {
		case strings.Contains(lower, "/pnpm/") || strings.Contains(lower, "/.pnpm/") || strings.Contains(lower, "/pnpm-global/"):
			return "pnpm"
		case strings.Contains(lower, "/.config/yarn/") || strings.Contains(lower, "/yarn/global/"):
			return "yarn"
		case strings.Contains(lower, "/.bun/") || strings.Contains(lower, "/bun/install/global/"):
			return "bun"
		case isNPMInstall(path):
			return "npm"
		}
	}
	if detectInstallMethodFromPaths(paths) == InstallMethodNPM {
		return "npm"
	}
	return ""
}

func detectPackageManagerFromNPMMetadata(paths []string) string {
	for _, path := range paths {
		if manager := npmRuntimePackageManager(path); manager != "" {
			return manager
		}
	}
	return ""
}

func fallbackNPMUpdateInfo(info UpdateInfo) (UpdateInfo, bool) {
	execPath, err := os.Executable()
	if err != nil {
		return info, false
	}
	return fallbackNPMUpdateInfoForExecPath(info, execPath)
}

func fallbackNPMUpdateInfoForExecPath(info UpdateInfo, execPath string) (UpdateInfo, bool) {
	if info.InstallMethod == InstallMethodNPM {
		if strings.TrimSpace(info.PackageManager) == "" {
			info.PackageManager = detectPackageManagerFromExecPath(execPath)
			info.UpdateCommand = updateCommandForPackageManager(info.PackageManager)
		}
		return info, true
	}
	if !isNPMInstall(execPath) {
		return info, false
	}
	manager := strings.TrimSpace(info.PackageManager)
	if manager == "" {
		manager = detectPackageManagerFromExecPath(execPath)
	}
	if manager == "" {
		return info, false
	}
	info.InstallMethod = InstallMethodNPM
	info.Channel = UpdateChannelNPM
	info.PackageManager = manager
	info.AutoUpdateSupported = true
	info.UpdateCommand = updateCommandForPackageManager(manager)
	return info, true
}

func (a *App) promptUpdateChoice(reader *bufio.Reader) (bool, error) {
	for {
		if _, err := fmt.Fprintln(a.Stdout, "Update now and continue with the current command?"); err != nil {
			return false, err
		}
		if _, err := fmt.Fprintln(a.Stdout, "1. Update now and continue"); err != nil {
			return false, err
		}
		if _, err := fmt.Fprintln(a.Stdout, "2. Continue without updating"); err != nil {
			return false, err
		}
		if _, err := fmt.Fprint(a.Stdout, "Enter an option number [1-2] (default 1): "); err != nil {
			return false, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "1", "yes", "y":
			return true, nil
		case "2", "no", "n":
			return false, nil
		default:
			if _, err := fmt.Fprintln(a.Stdout, "Please enter 1 or 2."); err != nil {
				return false, err
			}
		}
	}
}

func packageManagerCommand(manager string) (string, []string) {
	switch strings.ToLower(strings.TrimSpace(manager)) {
	case "pnpm":
		return "pnpm", []string{"add", "-g", defaultNPMPackageName}
	case "yarn":
		return "yarn", []string{"global", "add", defaultNPMPackageName}
	case "bun":
		return "bun", []string{"install", "-g", defaultNPMPackageName}
	case "npm":
		return "npm", []string{"install", "-g", defaultNPMPackageName}
	default:
		return "", nil
	}
}

func updateCommandForPackageManager(manager string) string {
	name, args := packageManagerCommand(manager)
	if strings.TrimSpace(name) == "" {
		return ""
	}
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

func latestReleaseIsNewer(currentVersion, latestVersionLabel, currentBuildDate, latestPublishedAt string) bool {
	if cmp, comparable := compareVersions(currentVersion, latestVersionLabel); comparable {
		return cmp < 0
	}
	buildTime, buildErr := time.Parse(time.RFC3339, strings.TrimSpace(currentBuildDate))
	publishedAt, publishErr := time.Parse(time.RFC3339, strings.TrimSpace(latestPublishedAt))
	return buildErr == nil && publishErr == nil && buildTime.Before(publishedAt)
}

func versionIsOlder(current, latest string) bool {
	if cmp, comparable := compareVersions(current, latest); comparable {
		return cmp < 0
	}
	currentTrimmed := normalizeVersionLabel(current)
	latestTrimmed := normalizeVersionLabel(latest)
	return currentTrimmed != "" && latestTrimmed != "" && currentTrimmed != latestTrimmed
}

func compareVersions(current, latest string) (int, bool) {
	currentParts, currentOK := parseVersionParts(current)
	latestParts, latestOK := parseVersionParts(latest)
	if currentOK && latestOK {
		maxLen := len(currentParts)
		if len(latestParts) > maxLen {
			maxLen = len(latestParts)
		}
		for i := 0; i < maxLen; i++ {
			var currentPart int
			if i < len(currentParts) {
				currentPart = currentParts[i]
			}
			var latestPart int
			if i < len(latestParts) {
				latestPart = latestParts[i]
			}
			if currentPart < latestPart {
				return -1, true
			}
			if currentPart > latestPart {
				return 1, true
			}
		}
		return 0, true
	}
	currentTrimmed := normalizeVersionLabel(current)
	latestTrimmed := normalizeVersionLabel(latest)
	if currentTrimmed != "" && latestTrimmed != "" && strings.EqualFold(currentTrimmed, latestTrimmed) {
		return 0, true
	}
	return 0, false
}

func parseVersionParts(raw string) ([]int, bool) {
	trimmed := normalizeVersionLabel(raw)
	if trimmed == "" {
		return nil, false
	}
	parts := strings.Split(trimmed, ".")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		digits := part
		for i, r := range part {
			if r < '0' || r > '9' {
				if i == 0 {
					return nil, false
				}
				digits = part[:i]
				break
			}
		}
		value := 0
		for _, r := range digits {
			value = value*10 + int(r-'0')
		}
		values = append(values, value)
	}
	return values, true
}

func normalizeVersionLabel(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if first := trimmed[0]; first == 'v' || first == 'V' {
		trimmed = strings.TrimSpace(trimmed[1:])
	}
	return trimmed
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
