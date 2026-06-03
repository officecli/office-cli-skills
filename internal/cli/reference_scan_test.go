package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli/engine"
	appruntime "github.com/officecli/officecli/internal/runtime"
)

func TestBuildGenerateJob_PPTXReferenceScanDefaultsToCWD(t *testing.T) {
	cwd := t.TempDir()

	job, err := BuildGenerateJob([]string{
		"pptx",
		"Brand Reuse Deck",
	}, Config{}, InputSources{IsTTY: true, CWD: cwd})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}

	if job.DocumentType != engine.DocumentTypePPTX {
		t.Fatalf("DocumentType = %q", job.DocumentType)
	}
	if !job.ReferenceScanEnabled {
		t.Fatal("expected reference scan to be enabled by default for pptx")
	}
	if job.ReferenceScanRoot != cwd {
		t.Fatalf("ReferenceScanRoot = %q, want %q", job.ReferenceScanRoot, cwd)
	}
	if job.PPTXBackend != appruntime.PPTXBackendOfficegen {
		t.Fatalf("PPTXBackend = %q, want %q", job.PPTXBackend, appruntime.PPTXBackendOfficegen)
	}
}

func TestBuildGenerateJob_PPTXBackendArtifactExperimental(t *testing.T) {
	job, err := BuildGenerateJob([]string{
		"pptx",
		"Artifact Worker Deck",
		"--pptx-backend", "artifact-experimental",
	}, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}
	if job.PPTXBackend != appruntime.PPTXBackendArtifactExperimental {
		t.Fatalf("PPTXBackend = %q", job.PPTXBackend)
	}
}

func TestBuildGenerateJob_PPTXBackendRejectsInvalidAndNonPPTX(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "invalid backend",
			args: []string{"pptx", "Deck", "--pptx-backend", "unknown"},
			want: "unsupported pptx backend",
		},
		{
			name: "non pptx",
			args: []string{"docx", "Memo", "--pptx-backend", "artifact-experimental"},
			want: "--pptx-backend is only supported for pptx generation",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildGenerateJob(tc.args, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildGenerateJobFromRequest_PPTXReferenceOptions(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	cwd := t.TempDir()
	first := filepath.Join(cwd, "brand.pptx")
	second := filepath.Join(cwd, "team-template.pptx")
	if err := os.WriteFile(first, []byte("pptx placeholder"), 0o644); err != nil {
		t.Fatalf("write first reference: %v", err)
	}
	if err := os.WriteFile(second, []byte("pptx placeholder"), 0o644); err != nil {
		t.Fatalf("write second reference: %v", err)
	}

	job, err := app.buildGenerateJobFromRequest(Config{}, bridgeInvokeParams{
		Tool: bridgeToolOfficeGenerate,
		Args: bridgeInvokeArgs{
			DocumentType:         "pptx",
			Topic:                "Brand Reuse Deck",
			Prompt:               "Explain the business value",
			ReferenceRoot:        "brand-assets",
			PPTXBackend:          "artifact-experimental",
			EnableReferenceScan:  boolPtr(false),
			ReferencePPTX:        first,
			ReferencePPTXSources: []string{second},
		},
	})
	if err != nil {
		t.Fatalf("buildGenerateJobFromRequest: %v", err)
	}
	if job.ReferenceScanEnabled {
		t.Fatal("expected bridge enable_reference_scan=false to disable automatic scanning")
	}
	if job.ReferenceScanRoot != "brand-assets" {
		t.Fatalf("ReferenceScanRoot = %q", job.ReferenceScanRoot)
	}
	if job.PPTXBackend != appruntime.PPTXBackendArtifactExperimental {
		t.Fatalf("PPTXBackend = %q", job.PPTXBackend)
	}
	wantSources := []string{first, second}
	if len(job.ReferencePPTXSources) != len(wantSources) {
		t.Fatalf("ReferencePPTXSources = %#v", job.ReferencePPTXSources)
	}
	for i, want := range wantSources {
		if job.ReferencePPTXSources[i] != want {
			t.Fatalf("ReferencePPTXSources[%d] = %q, want %q", i, job.ReferencePPTXSources[i], want)
		}
	}
}

func TestBuildGenerateJobFromRequest_PPTXBackendRejectsInvalidAndNonPPTX(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	cases := []struct {
		name string
		args bridgeInvokeArgs
		want string
	}{
		{
			name: "invalid",
			args: bridgeInvokeArgs{DocumentType: "pptx", Topic: "Deck", Prompt: "Build", PPTXBackend: "unknown"},
			want: "unsupported pptx backend",
		},
		{
			name: "non pptx",
			args: bridgeInvokeArgs{DocumentType: "docx", Topic: "Memo", Prompt: "Write", PPTXBackend: "artifact-experimental"},
			want: "pptx_backend is only supported for pptx generation",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := app.buildGenerateJobFromRequest(Config{}, bridgeInvokeParams{
				Tool: bridgeToolOfficeGenerate,
				Args: tc.args,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildGenerateJobFromRequest_PPTXReferenceOptionsRejectNonPPTX(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	_, err := app.buildGenerateJobFromRequest(Config{}, bridgeInvokeParams{
		Tool: bridgeToolOfficeGenerate,
		Args: bridgeInvokeArgs{
			DocumentType:  "docx",
			Topic:         "Memo",
			Prompt:        "Write a memo",
			ReferenceRoot: "brand-assets",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "reference_root is only supported for pptx generation") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildRenderJobFromRequest_RejectsReferenceOptions(t *testing.T) {
	app := NewApp(bytes.NewBuffer(nil), bytes.NewBuffer(nil), bytes.NewBuffer(nil))
	cases := []struct {
		name string
		args bridgeInvokeArgs
		want string
	}{
		{
			name: "root",
			args: bridgeInvokeArgs{ReferenceRoot: "brand-assets"},
			want: "reference_root is only supported for office.generate",
		},
		{
			name: "single explicit pptx",
			args: bridgeInvokeArgs{ReferencePPTX: "brand.pptx"},
			want: "reference_pptx is only supported for office.generate",
		},
		{
			name: "multiple explicit pptx",
			args: bridgeInvokeArgs{ReferencePPTXSources: []string{"brand.pptx"}},
			want: "reference_pptx is only supported for office.generate",
		},
		{
			name: "enable scan",
			args: bridgeInvokeArgs{EnableReferenceScan: boolPtr(false)},
			want: "enable_reference_scan is only supported for office.generate",
		},
		{
			name: "pptx backend",
			args: bridgeInvokeArgs{PPTXBackend: "artifact-experimental"},
			want: "pptx_backend is only supported for office.generate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.args
			args.DocumentType = "pptx"
			args.Topic = "Rendered Deck"
			args.Payload = json.RawMessage(`{"title":"Rendered Deck","slides":[]}`)
			_, _, err := app.buildRenderJobFromRequest(Config{}, bridgeInvokeParams{
				Tool: bridgeToolOfficeRender,
				Args: args,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildGenerateJob_PPTXReferenceScanCanBeDisabledAndRootOverridden(t *testing.T) {
	cwd := t.TempDir()
	referenceRoot := filepath.Join(cwd, "brand")

	job, err := BuildGenerateJob([]string{
		"pptx",
		"Brand Reuse Deck",
		"--reference-root", referenceRoot,
		"--no-reference-scan",
	}, Config{}, InputSources{IsTTY: true, CWD: cwd})
	if err != nil {
		t.Fatalf("BuildGenerateJob: %v", err)
	}

	if job.ReferenceScanEnabled {
		t.Fatal("expected reference scan to be disabled")
	}
	if job.ReferenceScanRoot != referenceRoot {
		t.Fatalf("ReferenceScanRoot = %q, want %q", job.ReferenceScanRoot, referenceRoot)
	}
}

func TestBuildGenerateJob_PPTXReferenceOptionsRejectNonPPTX(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "reference root",
			args: []string{"docx", "Memo", "--reference-root", "brand"},
			want: "--reference-root is only supported for pptx generation",
		},
		{
			name: "reference pptx",
			args: []string{"img", "Visual", "--reference-pptx", "brand.pptx"},
			want: "--reference-pptx is only supported for pptx generation",
		},
		{
			name: "no reference scan",
			args: []string{"xlsx", "Workbook", "--no-reference-scan"},
			want: "--no-reference-scan is only supported for pptx generation",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildGenerateJob(tc.args, Config{}, InputSources{IsTTY: true, CWD: t.TempDir()})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildGenerateJob_PPTXReferencePPTXValidatesExplicitFiles(t *testing.T) {
	cwd := t.TempDir()
	txtPath := filepath.Join(cwd, "brand.txt")
	if err := os.WriteFile(txtPath, []byte("not pptx"), 0o644); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	largePath := filepath.Join(cwd, "large.pptx")
	largeFile, err := os.Create(largePath)
	if err != nil {
		t.Fatalf("create large pptx: %v", err)
	}
	if err := largeFile.Truncate(maxExplicitReferencePPTXBytes + 1); err != nil {
		t.Fatalf("truncate large pptx: %v", err)
	}
	if err := largeFile.Close(); err != nil {
		t.Fatalf("close large pptx: %v", err)
	}
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "missing", path: filepath.Join(cwd, "missing.pptx"), want: "reference pptx does not exist"},
		{name: "not pptx", path: txtPath, want: "reference pptx must point to a .pptx file"},
		{name: "too large", path: largePath, want: "reference pptx exceeds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildGenerateJob([]string{
				"pptx",
				"Brand Reuse Deck",
				"--reference-pptx", tc.path,
			}, Config{}, InputSources{IsTTY: true, CWD: cwd})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
