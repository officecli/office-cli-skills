package license

import (
	"strings"
	"testing"
	"time"

	"github.com/officecli/officecli-internal/platform/internal/model"
)

func TestBuildRequestIDHasStableBoundedLength(t *testing.T) {
	req := CheckRequest{
		FingerprintHash: strings.Repeat("f", 64),
		Action:          string(model.UsageActionGenerate),
		DocumentType:    "pptx",
		RuntimeMode:     "external",
		RequestNonce:    strings.Repeat("n", 128),
	}

	got := buildRequestID(req)
	if !strings.HasPrefix(got, "req_") {
		t.Fatalf("request id prefix = %q", got)
	}
	if len(got) > 128 {
		t.Fatalf("request id too long: len=%d value=%q", len(got), got)
	}
}

func TestNewProofSignerUsesLongerDefaultTTL(t *testing.T) {
	signer, err := newProofSigner(ProofConfig{})
	if err != nil {
		t.Fatalf("newProofSigner: %v", err)
	}
	if signer.ttl != time.Hour {
		t.Fatalf("default ttl = %s, want %s", signer.ttl, time.Hour)
	}
}
