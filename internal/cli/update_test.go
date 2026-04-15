package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectInstallMethodUsesEnvOverride(t *testing.T) {
	t.Setenv(updateInstallMethodEnv, string(InstallMethodNPM))

	method, err := detectInstallMethod("/tmp/officecli")
	if err != nil {
		t.Fatalf("detectInstallMethod: %v", err)
	}
	if method != InstallMethodNPM {
		t.Fatalf("method = %q, want %q", method, InstallMethodNPM)
	}
}

func TestDetectInstallMethodFallsBackToScriptPath(t *testing.T) {
	t.Setenv(updateInstallMethodEnv, "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	method, err := detectInstallMethod(filepath.Join(home, ".local", "bin", "officecli"))
	if err != nil {
		t.Fatalf("detectInstallMethod: %v", err)
	}
	if method != InstallMethodScript {
		t.Fatalf("method = %q, want %q", method, InstallMethodScript)
	}
}

func TestUpdateCommandForPackageManager(t *testing.T) {
	testCases := []struct {
		name    string
		manager string
		want    string
	}{
		{name: "npm", manager: "npm", want: "npm install -g officecli"},
		{name: "pnpm", manager: "pnpm", want: "pnpm add -g officecli"},
		{name: "yarn", manager: "yarn", want: "yarn global add officecli"},
		{name: "bun", manager: "bun", want: "bun install -g officecli"},
		{name: "unknown", manager: "", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := updateCommandForPackageManager(tc.manager); got != tc.want {
				t.Fatalf("updateCommandForPackageManager(%q) = %q, want %q", tc.manager, got, tc.want)
			}
		})
	}
}

func TestVersionIsOlder(t *testing.T) {
	testCases := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "older patch", current: "0.2.5", latest: "0.2.6", want: true},
		{name: "same version", current: "0.2.6", latest: "0.2.6", want: false},
		{name: "newer patch", current: "0.2.7", latest: "0.2.6", want: false},
		{name: "prefixed version", current: "v0.2.5", latest: "0.2.6", want: true},
		{name: "non numeric fallback", current: "dev-build", latest: "latest", want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionIsOlder(tc.current, tc.latest); got != tc.want {
				t.Fatalf("versionIsOlder(%q, %q) = %t, want %t", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestDefaultPerformUpdateForNPMUsesCurrentPackageManager(t *testing.T) {
	originalPath := os.Getenv("PATH")
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "pnpm")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$OFFICECLI_TEST_ARGS_FILE\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	argsPath := filepath.Join(tmpDir, "args.txt")
	t.Setenv("OFFICECLI_TEST_ARGS_FILE", argsPath)
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)

	err := defaultPerformUpdate(context.Background(), UpdateInfo{InstallMethod: InstallMethodNPM, PackageManager: "pnpm"})
	if err != nil {
		t.Fatalf("defaultPerformUpdate: %v", err)
	}

	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := string(raw); got != "add\n-g\nofficecli\n" {
		t.Fatalf("args = %q", got)
	}
}

func TestDefaultPerformUpdateForNPMRequiresKnownPackageManager(t *testing.T) {
	err := defaultPerformUpdate(context.Background(), UpdateInfo{InstallMethod: InstallMethodNPM})
	if err == nil {
		t.Fatal("expected error for unknown package manager")
	}
}

func TestDefaultRestartCommandForNPMUsesPathCommand(t *testing.T) {
	originalPath := os.Getenv("PATH")
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "officecli")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$0\" \"$OFFICECLI_SKIP_UPDATE_CHECK\" \"$OFFICECLI_INSTALL_METHOD\" \"$OFFICECLI_PACKAGE_MANAGER\" \"$1\" \"$2\" > \"$OFFICECLI_TEST_ARGS_FILE\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	argsPath := filepath.Join(tmpDir, "restart.txt")
	t.Setenv("OFFICECLI_TEST_ARGS_FILE", argsPath)
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)

	err := defaultRestartCommand(context.Background(), UpdateInfo{
		InstallMethod:  InstallMethodNPM,
		PackageManager: "pnpm",
	}, []string{"config", "status"})
	if err != nil {
		t.Fatalf("defaultRestartCommand: %v", err)
	}

	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := string(raw); got != filepath.Join(tmpDir, "officecli")+"\n1\nnpm\npnpm\nconfig\nstatus\n" {
		t.Fatalf("restart = %q", got)
	}
}

func TestMaybeHandleUpdateSkipsPromptWhenAutoUpdateUnsupported(t *testing.T) {
	stdout := &terminalBuffer{}
	app := NewApp(stdout, os.Stderr, &terminalInputBuffer{Reader: strings.NewReader("yes\n")})
	originalVersion := Version
	originalBuildDate := BuildDate
	Version = "0.2.2"
	BuildDate = "2026-04-09T09:07:59Z"
	defer func() {
		Version = originalVersion
		BuildDate = originalBuildDate
	}()
	app.checkForUpdates = func(ctx context.Context) (UpdateInfo, error) {
		return UpdateInfo{
			Available:           true,
			CurrentVersion:      "0.2.2",
			LatestVersionLabel:  "0.2.6",
			InstallMethod:       InstallMethodNPM,
			Channel:             UpdateChannelNPM,
			AutoUpdateSupported: false,
		}, nil
	}
	app.performUpdate = func(ctx context.Context, info UpdateInfo) error {
		t.Fatal("performUpdate should not be called")
		return nil
	}

	if err := app.Run(context.Background(), []string{"config", "status"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(stdout.String(), "Update now and continue") {
		t.Fatalf("unexpected prompt in output: %q", stdout.String())
	}
}
