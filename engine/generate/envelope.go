package generate

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ModeFast = "fast"
	ModeBest = "best"
)

type Issue struct {
	Code       string
	Message    string
	Field      string
	ReasonCode string
	Retryable  bool
}

type PPTXMeta struct {
	RequestID string
	Mode      string
	Warnings  []Issue
}

type PromptEnvelope struct {
	RequestID string             `json:"request_id,omitempty"`
	Prompt    string             `json:"prompt"`
	Target    PromptTarget       `json:"target"`
	Images    []PromptImage      `json:"images,omitempty"`
	Options   PromptEnvelopeOpts `json:"options,omitempty"`
}

type PromptTarget struct {
	DocType  string `json:"doc_type,omitempty"`
	Language string `json:"language,omitempty"`
	Style    string `json:"style,omitempty"`
	Audience string `json:"audience,omitempty"`
}

type PromptImage struct {
	ImageID   string `json:"image_id,omitempty"`
	MIMEType  string `json:"mime_type,omitempty"`
	Source    string `json:"source,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type PromptEnvelopeOpts struct {
	AllowFallback  bool   `json:"allow_fallback,omitempty"`
	GenerationMode string `json:"generation_mode,omitempty"`
}

func ParsePromptEnvelope(description string) (PromptEnvelope, *PPTXMeta, error) {
	trimmed := strings.TrimSpace(description)
	meta := &PPTXMeta{Mode: "text_only"}
	if trimmed == "" {
		return PromptEnvelope{Prompt: ""}, meta, nil
	}
	var envelope PromptEnvelope
	if strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, `"prompt"`) {
		if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
			return PromptEnvelope{}, nil, fmt.Errorf("invalid JSON envelope: %w", err)
		}
		if strings.TrimSpace(envelope.Prompt) == "" {
			return PromptEnvelope{}, nil, fmt.Errorf("envelope prompt cannot be empty")
		}
		envelope.RequestID = strings.TrimSpace(envelope.RequestID)
		envelope.Prompt = strings.TrimSpace(envelope.Prompt)
		envelope.Target = NormalizePromptTarget(envelope.Target)
		envelope.Images = NormalizePromptImages(envelope.Images)
		envelope.Options.GenerationMode = NormalizeGenerationMode(envelope.Options.GenerationMode)
		meta.RequestID = envelope.RequestID
		if issue, hasIssue := FirstUnavailablePromptImageIssue(envelope.Images); hasIssue {
			if envelope.Options.AllowFallback {
				envelope.Images = nil
				meta.Mode = "text_only_fallback"
				meta.Warnings = append(meta.Warnings, Issue{
					Code:       "WARN_IMAGE_CONTEXT_FALLBACK",
					Message:    issue.Message,
					Field:      issue.Field,
					ReasonCode: issue.Code,
					Retryable:  issue.Retryable,
				})
				return envelope, meta, nil
			}
			return PromptEnvelope{}, nil, fmt.Errorf("%s: %s", issue.Code, issue.Message)
		}
		if len(envelope.Images) > 0 {
			meta.Mode = "image_augmented"
		}
		return envelope, meta, nil
	}
	return PromptEnvelope{
		Prompt: trimmed,
		Options: PromptEnvelopeOpts{
			GenerationMode: ModeFast,
		},
	}, meta, nil
}

func NormalizeGenerationMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ModeBest:
		return ModeBest
	default:
		return ModeFast
	}
}

func IsEmptyPromptTarget(target PromptTarget) bool {
	return strings.TrimSpace(target.DocType) == "" && strings.TrimSpace(target.Language) == "" && strings.TrimSpace(target.Style) == "" && strings.TrimSpace(target.Audience) == ""
}

func FormatImageReferences(images []PromptImage) string {
	if len(images) == 0 {
		return ""
	}
	lines := make([]string, 0, len(images))
	for idx, img := range images {
		lines = append(lines, fmt.Sprintf("image[%d]: id=%s mime=%s source=%s", idx, strings.TrimSpace(img.ImageID), strings.TrimSpace(img.MIMEType), strings.TrimSpace(img.Source)))
	}
	return strings.Join(lines, "\n")
}

func NormalizePromptTarget(target PromptTarget) PromptTarget {
	return PromptTarget{
		DocType:  strings.TrimSpace(target.DocType),
		Language: strings.TrimSpace(target.Language),
		Style:    strings.TrimSpace(target.Style),
		Audience: strings.TrimSpace(target.Audience),
	}
}

func NormalizePromptImages(images []PromptImage) []PromptImage {
	out := make([]PromptImage, 0, len(images))
	for _, image := range images {
		out = append(out, PromptImage{
			ImageID:   strings.TrimSpace(image.ImageID),
			MIMEType:  strings.TrimSpace(image.MIMEType),
			Source:    strings.TrimSpace(image.Source),
			SizeBytes: image.SizeBytes,
		})
	}
	return out
}

func FirstUnavailablePromptImageIssue(images []PromptImage) (Issue, bool) {
	for idx, image := range images {
		if strings.TrimSpace(image.Source) == "" || !IsSupportedPromptImageSource(image.Source) {
			return Issue{
				Code:      "ERR_IMAGE_SOURCE_UNAVAILABLE",
				Message:   fmt.Sprintf("image[%d] source is unavailable for image-aware generation", idx),
				Field:     fmt.Sprintf("images[%d].source", idx),
				Retryable: true,
			}, true
		}
	}
	return Issue{}, false
}

func IsSupportedPromptImageSource(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	switch {
	case strings.HasPrefix(source, "http://"),
		strings.HasPrefix(source, "https://"),
		strings.HasPrefix(source, "minio://"),
		strings.HasPrefix(source, "data:image/"):
		return true
	default:
		return false
	}
}
