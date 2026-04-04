package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"
)

const testProofSeed = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA"

func TestValidateCheckResultAcceptsSignedProof(t *testing.T) {
	req := CheckRequest{
		FingerprintHash: "fp-1",
		UserID:          42,
		DocumentType:    "pptx",
		RuntimeMode:     "external",
		RequestNonce:    "nonce-1",
		Action:          "generate",
	}
	result := &CheckResult{
		Allowed:    true,
		AccessMode: AccessModePaid,
		CommitToken: signProofToken(t, CommitToken{
			FingerprintHash: req.FingerprintHash,
			UserID:          req.UserID,
			RequestID:       "req-1",
			AccessMode:      AccessModePaid,
			Action:          req.Action,
			DocumentType:    req.DocumentType,
			RuntimeMode:     req.RuntimeMode,
			RequestNonce:    req.RequestNonce,
			ProofVersion:    "v1",
			IssuedAt:        time.Now().UTC().Add(-time.Minute),
			ExpiresAt:       time.Now().UTC().Add(time.Minute),
		}),
	}

	if err := ValidateCheckResult(result, req); err != nil {
		t.Fatalf("ValidateCheckResult() error = %v", err)
	}
}

func TestValidateCheckResultRejectsNonceReplay(t *testing.T) {
	req := CheckRequest{
		FingerprintHash: "fp-1",
		UserID:          42,
		DocumentType:    "pptx",
		RuntimeMode:     "external",
		RequestNonce:    "nonce-2",
		Action:          "generate",
	}
	result := &CheckResult{
		Allowed:    true,
		AccessMode: AccessModePaid,
		CommitToken: signProofToken(t, CommitToken{
			FingerprintHash: req.FingerprintHash,
			UserID:          req.UserID,
			RequestID:       "req-1",
			AccessMode:      AccessModePaid,
			Action:          req.Action,
			DocumentType:    req.DocumentType,
			RuntimeMode:     req.RuntimeMode,
			RequestNonce:    "captured-nonce",
			ProofVersion:    "v1",
			IssuedAt:        time.Now().UTC().Add(-time.Minute),
			ExpiresAt:       time.Now().UTC().Add(time.Minute),
		}),
	}

	if err := ValidateCheckResult(result, req); err == nil {
		t.Fatal("expected error")
	}
}

func signProofToken(t *testing.T, token CommitToken) CommitToken {
	t.Helper()
	seed, err := base64.RawURLEncoding.DecodeString(testProofSeed)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	token.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(commitTokenPayload(token))))
	return token
}
