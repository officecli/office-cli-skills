package review

import "context"

type Request struct {
	FilePath      string
	DocumentType  string
	EnableVisual  bool
	FailBelow     int
	RuntimeMode   string
	LLMProvider   string
	LLMBaseURL    string
	LLMAPIKey     string
	ReviewModel   string
	LLMTimeoutSec int
}

type Issue struct {
	Severity     string `json:"severity"`
	Code         string `json:"code"`
	Title        string `json:"title"`
	Message      string `json:"message"`
	SlideNumbers []int  `json:"slide_numbers,omitempty"`
	Suggestion   string `json:"suggestion,omitempty"`
}

type Result struct {
	Status         string   `json:"status"`
	DocumentType   string   `json:"document_type"`
	FilePath       string   `json:"file_path"`
	OverallScore   int      `json:"overall_score"`
	VisualScore    int      `json:"visual_score,omitempty"`
	StructureScore int      `json:"structure_score"`
	Summary        string   `json:"summary"`
	Strengths      []string `json:"strengths,omitempty"`
	Issues         []Issue  `json:"issues,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	UsedVisual     bool     `json:"used_visual"`
}

type PDFConverter interface {
	Convert(ctx context.Context, sourcePath string) (string, error)
}

type VisualReviewer interface {
	ReviewPDF(ctx context.Context, pdfPath string, structure StructureReport) (*VisualResult, error)
}

type Reporter interface {
	EmitReviewProgress(ctx context.Context, step, status, content string)
}

type Service struct {
	converter PDFConverter
	visual    VisualReviewer
	reporter  Reporter
}

type VisualResult struct {
	Score     int
	Summary   string
	Strengths []string
	Issues    []Issue
}

type StructureReport struct {
	Score     int
	Summary   string
	Strengths []string
	Issues    []Issue
}

func NewService(converter PDFConverter, visual VisualReviewer, reporter Reporter) *Service {
	return &Service{converter: converter, visual: visual, reporter: reporter}
}
