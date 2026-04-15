package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"
)

const testProofSeed = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA"
const testOverrideProofSeed = "cHJvZC1saWNlbnNlLXByb29mLXNlZWQtMTIzNDU2Nzg"

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

func TestValidateCheckResultAcceptsLegacyUnsignedToken(t *testing.T) {
	req := CheckRequest{
		FingerprintHash: "fp-legacy",
		DocumentType:    "pptx",
		RuntimeMode:     "external",
		RequestNonce:    "nonce-legacy",
		Action:          "generate",
	}
	result := &CheckResult{
		Allowed:    true,
		AccessMode: AccessModePaid,
		CommitToken: CommitToken{
			FingerprintHash: req.FingerprintHash,
			RequestID:       "req-legacy-1",
			AccessMode:      AccessModePaid,
			APIKeyHint:      "cop_live_xxx",
		},
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

func TestValidateCheckResultRejectsLegacyFingerprintMismatch(t *testing.T) {
	req := CheckRequest{
		FingerprintHash: "fp-1",
		Action:          "generate",
	}
	result := &CheckResult{
		Allowed:    true,
		AccessMode: AccessModePaid,
		CommitToken: CommitToken{
			FingerprintHash: "fp-other",
			RequestID:       "req-legacy-2",
			AccessMode:      AccessModePaid,
		},
	}

	if err := ValidateCheckResult(result, req); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateCheckResultAcceptsConfiguredPublicKeyOverride(t *testing.T) {
	t.Setenv(licenseProofPublicKeyEnv, publicKeyForSeed(t, testOverrideProofSeed))
	req := CheckRequest{
		FingerprintHash: "fp-override",
		UserID:          7,
		DocumentType:    "pptx",
		RuntimeMode:     "external",
		RequestNonce:    "nonce-override",
		Action:          "generate",
	}
	result := &CheckResult{
		Allowed:    true,
		AccessMode: AccessModePaid,
		CommitToken: signProofTokenWithSeed(t, testOverrideProofSeed, CommitToken{
			FingerprintHash: req.FingerprintHash,
			UserID:          req.UserID,
			RequestID:       "req-override",
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

func signProofToken(t *testing.T, token CommitToken) CommitToken {
	return signProofTokenWithSeed(t, testProofSeed, token)
}

func signProofTokenWithSeed(t *testing.T, seedValue string, token CommitToken) CommitToken {
	t.Helper()
	seed, err := base64.RawURLEncoding.DecodeString(seedValue)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	token.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(commitTokenPayload(token))))
	return token
}

func publicKeyForSeed(t *testing.T, seedValue string) string {
	t.Helper()
	seed, err := base64.RawURLEncoding.DecodeString(seedValue)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	return base64.RawURLEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey))
}
