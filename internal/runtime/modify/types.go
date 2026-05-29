package modify

import "github.com/officecli/officecli-internal/engine"

const (
	DefaultMaxNeedsRewrite = 3
	DefaultMaxParseRetries = 1
	DefaultMaxEscalations  = 1

	modifyPreviewSlides     = 24
	modifyPreviewParagraphs = 80
	modifyPreviewRows       = 24
	modifyPreviewCols       = 24
)

var attentionWarningTags = map[string]bool{
	"truncated_preview":              true,
	"target_outside_preview":         true,
	"sentinel_truncated_max_targets": true,
	"sentinel_dropped_max_depth":     true,
	"partial_failure":                true,
}

func AttentionWarningTags() []string {
	tags := make([]string, 0, len(attentionWarningTags))
	for t := range attentionWarningTags {
		tags = append(tags, t)
	}
	return tags
}

type Params struct {
	SourceFilePath  string
	DocumentType    engine.DocumentType
	Prompt          string
	Language        string
	Style           string
	OutputPath      string
	MaxEscalations  int
	MaxParseRetries int
	MaxNeedsRewrite int
}

type Result struct {
	OutputPath string
	Bytes      []byte
	ResultMeta ResultMeta
}

type ResultMeta struct {
	RouterPath        string         `json:"router_path"`
	OpsApplied        int            `json:"ops_applied"`
	OpsFailed         int            `json:"ops_failed"`
	LLMCalls          int            `json:"llm_calls"`
	Fidelity          FidelityReport `json:"fidelity"`
	Warnings          []string       `json:"warnings"`
	AttentionRequired bool           `json:"attention_required"`
}

type FidelityReport struct {
	Preserved []string `json:"preserved"`
	Dropped   []string `json:"dropped"`
}

func (m *ResultMeta) computeAttention() {
	if m.OpsFailed > 0 {
		m.AttentionRequired = true
		return
	}
	if len(m.Fidelity.Dropped) > 0 {
		m.AttentionRequired = true
		return
	}
	for _, w := range m.Warnings {
		if attentionWarningTags[w] {
			m.AttentionRequired = true
			return
		}
	}
}

func applyDefaults(p *Params) {
	if p.MaxNeedsRewrite == 0 {
		p.MaxNeedsRewrite = DefaultMaxNeedsRewrite
	}
	if p.MaxParseRetries == 0 {
		p.MaxParseRetries = DefaultMaxParseRetries
	}
	if p.MaxEscalations == 0 {
		p.MaxEscalations = DefaultMaxEscalations
	}
}
