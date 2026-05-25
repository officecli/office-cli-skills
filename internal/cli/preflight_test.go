package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreflightOutputFilter_SuppressesSkillRefreshNoise(t *testing.T) {
	var out bytes.Buffer
	filter := newPreflightOutputFilter(&out)

	input := "" +
		"installed skill to /Users/demo/.codex/skills/officecli\n" +
		"skipped officecli binary auto-install (AUTO_INSTALL_BINARY=0)\n" +
		"officecli version before refresh: officecli version 0.2.3 (...)\n" +
		"restart your client to pick up the refreshed skill\n"
	if _, err := filter.Write([]byte(input)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := filter.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestPreflightOutputFilter_PreservesNormalPromptAndErrors(t *testing.T) {
	var out bytes.Buffer
	filter := newPreflightOutputFilter(&out)

	if _, err := filter.Write([]byte("Enter the generation service URL: ")); err != nil {
		t.Fatalf("Write(prompt): %v", err)
	}
	if _, err := filter.Write([]byte("Generation service configuration is missing\n")); err != nil {
		t.Fatalf("Write(error): %v", err)
	}
	if err := filter.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	got := out.String()
	if got != "Enter the generation service URL: Generation service configuration is missing\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestPreflightOutputFilter_SuppressesReadyJSON(t *testing.T) {
	var out bytes.Buffer
	filter := newPreflightOutputFilter(&out)

	if _, err := filter.Write([]byte(`{"status":"ready","missing_items":[]}`)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := filter.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("unexpected output: %q", out.String())
	}
	status := filter.Status()
	if status == nil || status.Status != "ready" {
		t.Fatalf("status = %#v", status)
	}
}

func TestRunInstalledSkillPreflight_ReturnsReadableRepairableError(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	scriptPath := filepath.Join(homeDir, ".codex", "skills", "officecli", "fix-officecli-env.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte(`#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' '{"status":"repairable","missing_items":["generation_config","license_config"]}'
exit 10
`), 0o755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}
	t.Setenv("HOME", homeDir)
	t.Setenv(officeTaskPreflightSkipEnv, "0")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runInstalledSkillPreflight(context.Background(), bytes.NewBuffer(nil), &stdout, &stderr, "new", []string{"docx", "Quarterly Review"})
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if err.Error() != "officecli setup is incomplete: missing generation config, access config" {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected structured preflight output to be suppressed, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestPreflightSkipsGenerationButKeepsPublishSetupForIMG(t *testing.T) {
	args := []string{"--json", "--prompt", "Create a launch image", "img", "Launch Visual"}
	if !shouldSkipGenerationPreflight("new", args) {
		t.Fatal("expected img preflight to skip generation setup")
	}
	if shouldSkipPublishPreflight("new", args) {
		t.Fatal("expected img preflight to keep publish setup")
	}
	if !shouldRequireLicenseAPIKeyPreflight("new", args) {
		t.Fatal("expected img preflight to require license api key")
	}
	if !shouldSkipPublishPreflight("new", append(args, "--no-publish")) {
		t.Fatal("expected --no-publish to skip publish setup")
	}
}

func TestRunInstalledSkillPreflight_IMGPassesSkipSetupEnv(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	scriptPath := filepath.Join(homeDir, ".codex", "skills", "officecli", "fix-officecli-env.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte(`#!/usr/bin/env bash
set -euo pipefail
test "${OFFICECLI_SKIP_GENERATION_SETUP:-}" = "1"
test "${OFFICECLI_SKIP_PUBLISH_SETUP:-}" = ""
test "${OFFICECLI_REQUIRE_LICENSE_API_KEY:-}" = "1"
printf '%s\n' '{"status":"ready","missing_items":[]}'
`), 0o755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}
	t.Setenv("HOME", homeDir)
	t.Setenv(officeTaskPreflightSkipEnv, "0")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := runInstalledSkillPreflight(context.Background(), bytes.NewBuffer(nil), &stdout, &stderr, "new", []string{"img", "Launch Visual"})
	if err != nil {
		t.Fatalf("runInstalledSkillPreflight: %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("expected ready preflight output to be suppressed, stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestPreflightStatusError_BlockedRefreshIncludesHint(t *testing.T) {
	err := &preflightStatusError{
		payload: preflightStatusPayload{
			Status:        "blocked",
			FailureReason: "failed to refresh officecli skill bundle",
		},
	}
	msg := err.Error()
	if expected := "officecli setup failed: failed to refresh officecli skill bundle"; !strings.Contains(msg, expected) {
		t.Fatalf("expected %q in error, got: %s", expected, msg)
	}
	if !strings.Contains(msg, "OFFICECLI_SKIP_SKILL_PREFLIGHT=1") {
		t.Fatalf("expected skip hint in error, got: %s", msg)
	}
}

func TestPreflightStatusError_BlockedLoginNoHint(t *testing.T) {
	err := &preflightStatusError{
		payload: preflightStatusPayload{
			Status:        "blocked",
			FailureReason: "account login or API key required for image generation",
			MissingItems:  []string{"account_login"},
		},
	}
	msg := err.Error()
	if strings.Contains(msg, "OFFICECLI_SKIP_SKILL_PREFLIGHT=1") {
		t.Fatalf("should not show skip hint for auth failure, got: %s", msg)
	}
}

func TestPreflightStatusError_BlockedInstallIncludesHint(t *testing.T) {
	err := &preflightStatusError{
		payload: preflightStatusPayload{
			Status:        "blocked",
			FailureReason: "failed to install or refresh officecli binary",
			MissingItems:  []string{"officecli_binary"},
		},
	}
	msg := err.Error()
	if !strings.Contains(msg, "OFFICECLI_SKIP_SKILL_PREFLIGHT=1") {
		t.Fatalf("expected skip hint for install failure, got: %s", msg)
	}
}

func TestShouldOverrideAccountLoginPreflight_AcceptsBlockedAndRepairable(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	cfgJSON := `{"license":{"base_url":"https://platform.officecli.io","session_token":"ocli_sess_dummy","session_expires_at":"` + expiresAt + `","enabled":true}}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}
	t.Setenv("OFFICE_CLI_CONFIG", cfgPath)

	for _, status := range []string{"repairable", "blocked", "REPAIRABLE", "Blocked"} {
		t.Run(status, func(t *testing.T) {
			statusErr := &preflightStatusError{payload: preflightStatusPayload{
				Status:       status,
				MissingItems: []string{"account_login"},
			}}
			if !shouldOverrideAccountLoginPreflight(statusErr) {
				t.Fatalf("expected override to fire for status %q with valid session", status)
			}
		})
	}

	// Sanity: still rejects other missing items even on blocked status.
	statusErr := &preflightStatusError{payload: preflightStatusPayload{
		Status:       "blocked",
		MissingItems: []string{"account_login", "license_config"},
	}}
	if shouldOverrideAccountLoginPreflight(statusErr) {
		t.Fatal("override should not fire when other items are missing")
	}

	// Sanity: still rejects unknown statuses.
	statusErr = &preflightStatusError{payload: preflightStatusPayload{
		Status:       "ready",
		MissingItems: []string{"account_login"},
	}}
	if shouldOverrideAccountLoginPreflight(statusErr) {
		t.Fatal("override should not fire for non-failure status")
	}
}

func TestRunInstalledSkillPreflight_OverridesBlockedAccountLoginWhenSessionValid(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	scriptPath := filepath.Join(homeDir, ".codex", "skills", "officecli", "fix-officecli-env.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Simulate a stale fix-officecli-env.sh that misclassifies a logged-in
	// session as anonymous and exits with status=blocked.
	if err := os.WriteFile(scriptPath, []byte(`#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' '{"status":"blocked","failure_reason":"account login or API key required for image generation","missing_items":["account_login"]}'
exit 10
`), 0o755); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}

	cfgPath := filepath.Join(tmpDir, "config.json")
	expiresAt := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	cfgJSON := `{"license":{"base_url":"https://platform.officecli.io","session_token":"ocli_sess_dummy","session_expires_at":"` + expiresAt + `","enabled":true}}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0o600); err != nil {
		t.Fatalf("WriteFile(config): %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("OFFICE_CLI_CONFIG", cfgPath)
	t.Setenv(officeTaskPreflightSkipEnv, "0")

	var stdout, stderr bytes.Buffer
	err := runInstalledSkillPreflight(context.Background(), bytes.NewBuffer(nil), &stdout, &stderr, "new", []string{"img", "Launch Visual"})
	if err != nil {
		t.Fatalf("expected blocked account_login to be overridden, got: %v (stderr=%q)", err, stderr.String())
	}
}
