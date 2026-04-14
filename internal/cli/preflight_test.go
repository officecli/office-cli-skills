package cli

import (
	"bytes"
	"testing"
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

	if _, err := filter.Write([]byte("请输入生成服务地址: ")); err != nil {
		t.Fatalf("Write(prompt): %v", err)
	}
	if _, err := filter.Write([]byte("生成服务配置缺失\n")); err != nil {
		t.Fatalf("Write(error): %v", err)
	}
	if err := filter.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	got := out.String()
	if got != "请输入生成服务地址: 生成服务配置缺失\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}
