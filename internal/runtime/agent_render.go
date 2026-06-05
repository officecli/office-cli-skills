package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/officecli/officecli/engine"
	generateengine "github.com/officecli/officecli/engine/generate"
	"github.com/officecli/officecli/pkg/officegen"
)

type PrepareParams struct {
	DocumentType   engine.DocumentType
	Topic          string
	SourceFilePath string
}

type PreparedPayload struct {
	DocumentType    string          `json:"document_type"`
	PreferredTool   string          `json:"preferred_tool"`
	PrepareRequired bool            `json:"prepare_required"`
	PayloadSchema   map[string]any  `json:"payload_schema"`
	FieldNotes      []string        `json:"field_notes,omitempty"`
	WorkbookSummary string          `json:"workbook_summary,omitempty"`
	BaseReportJSON  json.RawMessage `json:"base_report_json,omitempty"`
}

const docxPayloadSchema = `{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "title":{"type":"string"},
    "sections":{
      "type":"array",
      "items":{
        "type":"object",
        "additionalProperties":false,
        "properties":{
          "heading":{"type":"string"},
          "level":{"type":"integer"},
          "paragraphs":{"type":"array","items":{"type":"string"}}
        },
        "required":["heading","level","paragraphs"]
      }
    }
  },
  "required":["title","sections"]
}`

const xlsxPayloadSchema = `{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "title":{"type":"string"},
    "sheets":{
      "type":"array",
      "items":{
        "type":"object",
        "additionalProperties":false,
        "properties":{
          "name":{"type":"string"},
          "headers":{"type":"array","items":{"type":"string"}},
          "rows":{"type":"array","items":{"type":"array","items":{"type":"string"}}}
        },
        "required":["name","headers","rows"]
      }
    }
  },
  "required":["title","sheets"]
}`

const reportPayloadSchema = `{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "title":{"type":"string"},
    "subtitle":{"type":"string"},
    "language":{"type":"string"},
    "audience":{"type":"string"},
    "summary":{"type":"string"},
    "updatedAt":{"type":"string"},
    "theme":{
      "type":"object",
      "additionalProperties":false,
      "properties":{
        "name":{"type":"string"},
        "accentColor":{"type":"string"},
        "accentSoft":{"type":"string"},
        "textColor":{"type":"string"},
        "mutedColor":{"type":"string"},
        "background":{"type":"string"},
        "surfaceColor":{"type":"string"},
        "borderColor":{"type":"string"}
      }
    },
    "kpis":{
      "type":"array",
      "items":{
        "type":"object",
        "additionalProperties":false,
        "properties":{
          "label":{"type":"string"},
          "value":{"type":"string"},
          "change":{"type":"string"},
          "note":{"type":"string"}
        },
        "required":["label","value"]
      }
    },
    "findings":{"type":"array","items":{"type":"string"}},
    "sections":{
      "type":"array",
      "items":{
        "type":"object",
        "additionalProperties":false,
        "properties":{
          "title":{"type":"string"},
          "subtitle":{"type":"string"},
          "narrative":{"type":"array","items":{"type":"string"}},
          "takeaways":{"type":"array","items":{"type":"string"}},
          "charts":{
            "type":"array",
            "items":{
              "type":"object",
              "additionalProperties":false,
              "properties":{
                "type":{"type":"string"},
                "title":{"type":"string"},
                "subtitle":{"type":"string"},
                "categories":{"type":"array","items":{"type":"string"}},
                "series":{
                  "type":"array",
                  "items":{
                    "type":"object",
                    "additionalProperties":false,
                    "properties":{
                      "name":{"type":"string"},
                      "values":{"type":"array","items":{"type":"number"}}
                    },
                    "required":["name","values"]
                  }
                },
                "unit":{"type":"string"},
                "source":{"type":"string"},
                "notes":{"type":"array","items":{"type":"string"}}
              },
              "required":["type","title","series"]
            }
          },
          "table":{
            "type":"object",
            "additionalProperties":false,
            "properties":{
              "title":{"type":"string"},
              "headers":{"type":"array","items":{"type":"string"}},
              "rows":{"type":"array","items":{"type":"array","items":{"type":"string"}}}
            },
            "required":["headers","rows"]
          }
        },
        "required":["title"]
      }
    },
    "appendixTables":{
      "type":"array",
      "items":{
        "type":"object",
        "additionalProperties":false,
        "properties":{
          "title":{"type":"string"},
          "headers":{"type":"array","items":{"type":"string"}},
          "rows":{"type":"array","items":{"type":"array","items":{"type":"string"}}}
        },
        "required":["headers","rows"]
      }
    }
  },
  "required":["title","sections"]
}`

func PrepareAgentPayload(params PrepareParams) (*PreparedPayload, error) {
	schema, err := AgentPayloadSchema(params.DocumentType)
	if err != nil {
		return nil, err
	}
	result := &PreparedPayload{
		DocumentType:    string(params.DocumentType),
		PreferredTool:   "office.render",
		PrepareRequired: params.DocumentType == engine.DocumentTypeReport,
		PayloadSchema:   schema,
		FieldNotes:      agentFieldNotes(params.DocumentType),
	}
	if params.DocumentType != engine.DocumentTypeReport {
		return result, nil
	}
	if strings.TrimSpace(params.SourceFilePath) == "" {
		return nil, fmt.Errorf("report generation requires file_path")
	}
	sheets, err := loadWorkbookSheetsFromFile(params.SourceFilePath)
	if err != nil {
		return nil, fmt.Errorf("read report workbook: %w", err)
	}
	baseReport := officegen.BuildReportFromWorkbook(strings.TrimSpace(params.Topic), sheets)
	baseReportJSON, err := json.Marshal(baseReport)
	if err != nil {
		return nil, fmt.Errorf("marshal base report: %w", err)
	}
	result.WorkbookSummary = buildWorkbookSummary(sheets)
	result.BaseReportJSON = baseReportJSON
	return result, nil
}

func AgentPayloadSchema(documentType engine.DocumentType) (map[string]any, error) {
	var raw string
	switch documentType {
	case engine.DocumentTypeDOCX:
		raw = docxPayloadSchema
	case engine.DocumentTypeXLSX:
		raw = xlsxPayloadSchema
	case engine.DocumentTypeReport:
		raw = reportPayloadSchema
	case engine.DocumentTypePPTX:
		raw = pptxStructuredSchema
	default:
		return nil, fmt.Errorf("unsupported document type: %s", documentType)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(raw), &schema); err != nil {
		return nil, fmt.Errorf("decode payload schema: %w", err)
	}
	return schema, nil
}

func agentFieldNotes(documentType engine.DocumentType) []string {
	switch documentType {
	case engine.DocumentTypeDOCX:
		return []string{
			"`sections[].level` should use 1-6 heading levels; body paragraphs stay in `paragraphs`.",
			"Keep the structure complete enough that the file can be assembled without follow-up repair.",
		}
	case engine.DocumentTypeXLSX:
		return []string{
			"`headers` defines the first row of each sheet; `rows` should contain only data rows.",
			"Keep row widths consistent with the header count.",
		}
	case engine.DocumentTypeReport:
		return []string{
			"Use the prepared workbook summary and base report as the evidence boundary.",
			"Do not invent metrics, categories, or findings unsupported by the workbook.",
		}
	case engine.DocumentTypePPTX:
		return []string{
			"Return JSON that matches the strict PPT schema exactly, including required slide fields.",
			"Image prompts may be included, but actual image generation still depends on the configured image provider.",
		}
	default:
		return nil
	}
}

func (s *Service) Render(ctx context.Context, params GenerateParams, payload json.RawMessage) (*GeneratedArtifact, error) {
	content := strings.TrimSpace(string(payload))
	if content == "" {
		return nil, fmt.Errorf("payload is required")
	}
	fallback := fallbackDescription(params.Topic, params.Prompt)
	switch params.DocumentType {
	case engine.DocumentTypeDOCX:
		emitProgress(ctx, s.progress, progressStepAssemble, "running", "Assembling the DOCX file")
		fileBytes, fileName, err := generateengine.BuildDOCXFromJSON(content, fallback)
		if err != nil {
			emitProgress(ctx, s.progress, progressStepAssemble, "failed", "DOCX assembly failed")
			return nil, fmt.Errorf("document assembly failed: %w", err)
		}
		emitProgress(ctx, s.progress, progressStepAssemble, "completed", "DOCX assembly completed")
		return &GeneratedArtifact{
			DocumentName: fileName,
			DocumentType: string(engine.DocumentTypeDOCX),
			Bytes:        fileBytes,
		}, nil
	case engine.DocumentTypeXLSX:
		emitProgress(ctx, s.progress, progressStepAssemble, "running", "Assembling the XLSX file")
		fileBytes, fileName, err := generateengine.BuildXLSXFromJSON(content, fallback)
		if err != nil {
			emitProgress(ctx, s.progress, progressStepAssemble, "failed", "XLSX assembly failed")
			return nil, fmt.Errorf("document assembly failed: %w", err)
		}
		emitProgress(ctx, s.progress, progressStepAssemble, "completed", "XLSX assembly completed")
		return &GeneratedArtifact{
			DocumentName: fileName,
			DocumentType: string(engine.DocumentTypeXLSX),
			Bytes:        fileBytes,
		}, nil
	case engine.DocumentTypeReport:
		emitProgress(ctx, s.progress, progressStepAssemble, "running", "Assembling the report HTML")
		fileBytes, fileName, err := generateengine.BuildReportFromJSON(content, fallback)
		if err != nil {
			emitProgress(ctx, s.progress, progressStepAssemble, "failed", "Report assembly failed")
			return nil, fmt.Errorf("document assembly failed: %w", err)
		}
		emitProgress(ctx, s.progress, progressStepAssemble, "completed", "Report assembly completed")
		return &GeneratedArtifact{
			DocumentName: fileName,
			DocumentType: string(engine.DocumentTypeReport),
			Bytes:        fileBytes,
		}, nil
	case engine.DocumentTypePPTX:
		requestedStyle := strings.TrimSpace(params.Style)
		var hostedCreditBalance *int
		hostedCreditsCharged := 0
		hostedCreditsChargedSet := false
		imageLLM := s.llm
		if s.imageLLM != nil {
			imageLLM = s.imageLLM
		}
		fileBytes, fileName, warnings, previewHTML, previewJSON, err := BuildPPTXFromJSONWithOptions(ctx, imageLLM, s.progress, content, fallback, requestedStyle, params.EnableImages, params.LocalPreview, PPTXBuildOptions{
			CreditBalanceSink: func(balance int) {
				value := balance
				hostedCreditBalance = &value
			},
			CreditChargedSink: func(charged int) {
				hostedCreditsCharged += charged
				hostedCreditsChargedSet = true
			},
			ArtifactDesignPlanLLM: s.llm,
		})
		if err != nil {
			return nil, err
		}
		var hostedCreditsChargedPtr *int
		if hostedCreditsChargedSet {
			value := hostedCreditsCharged
			hostedCreditsChargedPtr = &value
		}
		return &GeneratedArtifact{
			DocumentName:         fileName,
			DocumentType:         string(engine.DocumentTypePPTX),
			Bytes:                fileBytes,
			Warnings:             warnings,
			PreviewHTML:          previewHTML,
			PreviewJSON:          previewJSON,
			HostedCreditBalance:  hostedCreditBalance,
			HostedCreditsCharged: hostedCreditsChargedPtr,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported document type: %s", params.DocumentType)
	}
}
