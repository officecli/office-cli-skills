package cli

import (
	"context"
	"os"
	"path/filepath"
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
		{name: "npm default", manager: "", want: "npm install -g officecli"},
		{name: "pnpm", manager: "pnpm", want: "pnpm add -g officecli"},
		{name: "yarn", manager: "yarn", want: "yarn global add officecli"},
		{name: "bun", manager: "bun", want: "bun install -g officecli"},
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
	t.Setenv(updatePackageManagerEnv, "pnpm")
	originalPath := os.Getenv("PATH")
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "pnpm")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$OFFICECLI_TEST_ARGS_FILE\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	argsPath := filepath.Join(tmpDir, "args.txt")
	t.Setenv("OFFICECLI_TEST_ARGS_FILE", argsPath)
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)

	err := defaultPerformUpdate(context.Background(), UpdateInfo{InstallMethod: InstallMethodNPM})
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
