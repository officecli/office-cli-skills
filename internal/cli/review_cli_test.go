package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/officecli/officecli-internal/engine"
)

type stubReviewer struct {
	result  *ReviewResult
	err     error
	lastReq ReviewRequest
}

func (s *stubReviewer) Review(_ context.Context, req ReviewRequest) (*ReviewResult, error) {
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func TestBuildReviewJob_ParsesFlags(t *testing.T) {
	job, err := BuildReviewJob([]string{"--json", "--no-visual", "--fail-below", "78", "pptx", "./deck.pptx"})
	if err != nil {
		t.Fatalf("BuildReviewJob: %v", err)
	}
	if !job.JSONOutput || job.EnableVisual || job.FailBelow != 78 {
		t.Fatalf("unexpected job: %+v", job)
	}
}

func TestAppRun_ReviewJSONOutput(t *testing.T) {
	deckPath := filepath.Join(t.TempDir(), "deck.pptx")
	if err := os.WriteFile(deckPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr, strings.NewReader(""))
	stub := &stubReviewer{result: &ReviewResult{
		Status:         "partial",
		DocumentType:   "pptx",
		FilePath:       deckPath,
		OverallScore:   82,
		StructureScore: 82,
		Summary:        "Structural checks passed.",
		Strengths:      []string{"Clear structure"},
		Warnings:       []string{"Visual review was disabled explicitly. The result only contains structural checks."},
	}}
	app.newReviewer = func(cfg Config, progress engine.ProgressEmitter) (Reviewer, error) {
		return stub, nil
	}

	if err := app.Run(context.Background(), []string{"review", "--json", "--no-visual", "pptx", deckPath}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stub.lastReq.EnableVisual {
		t.Fatalf("expected no visual request, got %+v", stub.lastReq)
	}
	var result ReviewResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal stdout: %v\n%s", err, stdout.String())
	}
	if result.OverallScore != 82 || result.FilePath != deckPath {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestAppRun_ScoreJSONOutput(t *testing.T) {
	deckPath := filepath.Join(t.TempDir(), "deck.pptx")
	if err := os.WriteFile(deckPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr, strings.NewReader(""))
	stub := &stubReviewer{result: &ReviewResult{
		Status:         "good",
		DocumentType:   "pptx",
		FilePath:       deckPath,
		OverallScore:   90,
		StructureScore: 90,
		Summary:        "The deck is structurally solid.",
	}}
	app.newReviewer = func(cfg Config, progress engine.ProgressEmitter) (Reviewer, error) {
		return stub, nil
	}

	if err := app.Run(context.Background(), []string{"score", "--json", "--no-visual", "pptx", deckPath}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stub.lastReq.EnableVisual {
		t.Fatalf("expected no visual request, got %+v", stub.lastReq)
	}
}

func TestAppRun_ReviewFailBelowReturnsError(t *testing.T) {
	deckPath := filepath.Join(t.TempDir(), "deck.pptx")
	if err := os.WriteFile(deckPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var stdout bytes.Buffer
	app := NewApp(&stdout, &bytes.Buffer{}, strings.NewReader(""))
	app.newReviewer = func(cfg Config, progress engine.ProgressEmitter) (Reviewer, error) {
		return &stubReviewer{result: &ReviewResult{
			Status:         "needs_work",
			DocumentType:   "pptx",
			FilePath:       deckPath,
			OverallScore:   60,
			StructureScore: 60,
			Summary:        "Several issues need attention.",
		}}, nil
	}

	err := app.Run(context.Background(), []string{"review", "pptx", deckPath, "--fail-below", "80"})
	if err == nil {
		t.Fatal("expected fail-below error")
	}
	if !strings.Contains(err.Error(), "below the threshold 80") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Review completed: total score 60") {
		t.Fatalf("expected rendered output, got %q", stdout.String())
	}
}
