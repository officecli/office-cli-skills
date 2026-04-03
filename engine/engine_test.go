package engine

import (
	"context"
	"testing"
)

type stubGenerator struct{}

func (stubGenerator) GenerateDocument(ctx context.Context, req GenerateDocumentRequest) (*GeneratedDocument, error) {
	return &GeneratedDocument{Name: "generated-by-generator", Type: string(req.DocumentType)}, nil
}

type stubWorkflow struct{}

func (stubWorkflow) GenerateDocument(ctx context.Context, req GenerateDocumentRequest) (*GeneratedDocument, error) {
	return &GeneratedDocument{Name: "generated-by-workflow", Type: string(req.DocumentType)}, nil
}

func (stubWorkflow) ModifyDocument(ctx context.Context, req ModifyDocumentRequest) (*Document, error) {
	return &Document{ID: req.DocumentID, Version: req.BaseVersion + 1}, nil
}

func (stubWorkflow) ClassifyIntent(ctx context.Context, req ClassifyIntentRequest) (*ClassifyIntentResult, error) {
	return &ClassifyIntentResult{Action: "modify_current_document", ModifyIntent: "replace_slide_title"}, nil
}

func (stubWorkflow) PrepareExecutionPlan(ctx context.Context, req PrepareExecutionPlanRequest) (*PlanSession, error) {
	return &PlanSession{PlanID: "plan-1", Status: "questioning"}, nil
}

func (stubWorkflow) AnswerExecutionPlanQuestion(ctx context.Context, req AnswerExecutionPlanQuestionRequest) (*PlanSession, error) {
	return &PlanSession{PlanID: req.PlanID, Status: "questioning"}, nil
}

func (stubWorkflow) ReviseExecutionPlan(ctx context.Context, req ReviseExecutionPlanRequest) (*PlanSession, error) {
	return &PlanSession{PlanID: req.PlanID, Status: "review_pending"}, nil
}

func (stubWorkflow) ApproveExecutionPlan(ctx context.Context, req ApproveExecutionPlanRequest) (*PlanSession, error) {
	return &PlanSession{PlanID: req.PlanID, Status: "approved"}, nil
}

func TestComposeDocumentEngine_UsesExplicitGenerator(t *testing.T) {
	engine := ComposeDocumentEngine(stubGenerator{}, stubWorkflow{})
	if engine == nil {
		t.Fatal("expected engine")
	}
	generated, err := engine.GenerateDocument(context.Background(), GenerateDocumentRequest{DocumentType: DocumentTypePPTX})
	if err != nil {
		t.Fatalf("GenerateDocument: %v", err)
	}
	if generated.Name != "generated-by-generator" {
		t.Fatalf("generated = %+v", generated)
	}
	modified, err := engine.ModifyDocument(context.Background(), ModifyDocumentRequest{DocumentID: "doc-1", BaseVersion: 2})
	if err != nil {
		t.Fatalf("ModifyDocument: %v", err)
	}
	if modified.Version != 3 {
		t.Fatalf("modified = %+v", modified)
	}
}

func TestComposeDocumentEngine_FallsBackToWorkflowGenerator(t *testing.T) {
	engine := ComposeDocumentEngine(nil, stubWorkflow{})
	generated, err := engine.GenerateDocument(context.Background(), GenerateDocumentRequest{DocumentType: DocumentTypeDOCX})
	if err != nil {
		t.Fatalf("GenerateDocument: %v", err)
	}
	if generated.Name != "generated-by-workflow" {
		t.Fatalf("generated = %+v", generated)
	}
}
