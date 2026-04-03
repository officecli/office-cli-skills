package engine

import (
	"context"
	"fmt"
)

type composedDocumentEngine struct {
	generator GenerateDocumentService
	workflow  DocumentWorkflowService
}

func ComposeDocumentEngine(generator GenerateDocumentService, workflow DocumentWorkflowService) DocumentEngine {
	if workflow == nil {
		return nil
	}
	if generator == nil {
		generator = workflow
	}
	return &composedDocumentEngine{
		generator: generator,
		workflow:  workflow,
	}
}

func (e *composedDocumentEngine) GenerateDocument(ctx context.Context, req GenerateDocumentRequest) (*GeneratedDocument, error) {
	if e == nil || e.generator == nil {
		return nil, fmt.Errorf("generate document service is unavailable")
	}
	return e.generator.GenerateDocument(ctx, req)
}

func (e *composedDocumentEngine) ModifyDocument(ctx context.Context, req ModifyDocumentRequest) (*Document, error) {
	if e == nil || e.workflow == nil {
		return nil, fmt.Errorf("document workflow service is unavailable")
	}
	return e.workflow.ModifyDocument(ctx, req)
}

func (e *composedDocumentEngine) ClassifyIntent(ctx context.Context, req ClassifyIntentRequest) (*ClassifyIntentResult, error) {
	if e == nil || e.workflow == nil {
		return nil, fmt.Errorf("document workflow service is unavailable")
	}
	return e.workflow.ClassifyIntent(ctx, req)
}

func (e *composedDocumentEngine) PrepareExecutionPlan(ctx context.Context, req PrepareExecutionPlanRequest) (*PlanSession, error) {
	if e == nil || e.workflow == nil {
		return nil, fmt.Errorf("document workflow service is unavailable")
	}
	return e.workflow.PrepareExecutionPlan(ctx, req)
}

func (e *composedDocumentEngine) AnswerExecutionPlanQuestion(ctx context.Context, req AnswerExecutionPlanQuestionRequest) (*PlanSession, error) {
	if e == nil || e.workflow == nil {
		return nil, fmt.Errorf("document workflow service is unavailable")
	}
	return e.workflow.AnswerExecutionPlanQuestion(ctx, req)
}

func (e *composedDocumentEngine) ReviseExecutionPlan(ctx context.Context, req ReviseExecutionPlanRequest) (*PlanSession, error) {
	if e == nil || e.workflow == nil {
		return nil, fmt.Errorf("document workflow service is unavailable")
	}
	return e.workflow.ReviseExecutionPlan(ctx, req)
}

func (e *composedDocumentEngine) ApproveExecutionPlan(ctx context.Context, req ApproveExecutionPlanRequest) (*PlanSession, error) {
	if e == nil || e.workflow == nil {
		return nil, fmt.Errorf("document workflow service is unavailable")
	}
	return e.workflow.ApproveExecutionPlan(ctx, req)
}
