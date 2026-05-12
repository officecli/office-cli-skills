package cli

import (
	"testing"

	"github.com/officecli/officecli/engine"
)

func TestHostedModelNamesUseModalityProfiles(t *testing.T) {
	textJobs := []GenerateJob{
		{DocumentType: engine.DocumentTypeDOCX},
		{DocumentType: engine.DocumentTypeXLSX},
		{DocumentType: engine.DocumentTypeReport},
		{DocumentType: engine.DocumentTypePPTX, EnableImages: false},
		{DocumentType: engine.DocumentTypePPTX, EnableImages: true},
	}
	for _, job := range textJobs {
		if got := hostedTextModelName(job); got != "hosted/text" {
			t.Fatalf("hostedTextModelName(%s, images=%v) = %q", job.DocumentType, job.EnableImages, got)
		}
		if got := hostedImageModelName(job); got != "hosted/image" {
			t.Fatalf("hostedImageModelName(%s, images=%v) = %q", job.DocumentType, job.EnableImages, got)
		}
	}

	imgJob := GenerateJob{DocumentType: engine.DocumentTypeIMG}
	if got := hostedTextModelName(imgJob); got != "hosted/image" {
		t.Fatalf("hostedTextModelName(img) = %q", got)
	}
	if got := hostedImageModelName(imgJob); got != "hosted/image" {
		t.Fatalf("hostedImageModelName(img) = %q", got)
	}
}
