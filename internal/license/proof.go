package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	licenseProofVersion          = "v1"
	defaultLicenseProofPublicKey = "ebVWLo_mVPlAeLES6KmLp5AfhTrmlb7X4OORC60ElmQ"
	licenseProofPublicKeyEnv     = "OFFICE_CLI_LICENSE_PROOF_PUBLIC_KEY"
	licenseProofClockSkew        = 30 * time.Second
)

var nowFunc = time.Now
var EmbeddedLicenseProofPublicKey = defaultLicenseProofPublicKey

func NewRequestNonce() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate request nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func ValidateCheckResult(result *CheckResult, req CheckRequest) error {
	if result == nil || !result.Allowed {
		return nil
	}
	return ValidateCommitToken(result.CommitToken, req, result.AccessMode)
}

func ValidateCommitToken(token CommitToken, req CheckRequest, accessMode AccessMode) error {
	if isLegacyUnsignedCommitToken(token) {
		if strings.TrimSpace(token.RequestID) == "" {
			return fmt.Errorf("license proof missing request id")
		}
		if token.FingerprintHash != "" && token.FingerprintHash != strings.TrimSpace(req.FingerprintHash) {
			return fmt.Errorf("license proof fingerprint mismatch")
		}
		if token.AccessMode != "" && token.AccessMode != accessMode {
			return fmt.Errorf("license proof access mode mismatch")
		}
		return nil
	}
	if strings.TrimSpace(token.ProofVersion) != licenseProofVersion {
		return fmt.Errorf("license proof version mismatch")
	}
	if strings.TrimSpace(token.RequestID) == "" {
		return fmt.Errorf("license proof missing request id")
	}
	if strings.TrimSpace(token.RequestNonce) == "" || strings.TrimSpace(req.RequestNonce) == "" {
		return fmt.Errorf("license proof missing request nonce")
	}
	if token.RequestNonce != strings.TrimSpace(req.RequestNonce) {
		return fmt.Errorf("license proof request nonce mismatch")
	}
	if token.FingerprintHash != strings.TrimSpace(req.FingerprintHash) {
		return fmt.Errorf("license proof fingerprint mismatch")
	}
	if token.UserID != req.UserID {
		return fmt.Errorf("license proof user mismatch")
	}
	if token.Action != strings.TrimSpace(req.Action) {
		return fmt.Errorf("license proof action mismatch")
	}
	if token.DocumentType != strings.TrimSpace(req.DocumentType) {
		return fmt.Errorf("license proof document type mismatch")
	}
	if token.RuntimeMode != strings.TrimSpace(req.RuntimeMode) {
		return fmt.Errorf("license proof runtime mode mismatch")
	}
	if token.AccessMode != accessMode {
		return fmt.Errorf("license proof access mode mismatch")
	}
	now := nowFunc().UTC()
	if token.IssuedAt.IsZero() || token.ExpiresAt.IsZero() {
		return fmt.Errorf("license proof missing validity window")
	}
	if now.Before(token.IssuedAt.Add(-licenseProofClockSkew)) {
		return fmt.Errorf("license proof issued in the future")
	}
	if now.After(token.ExpiresAt.Add(licenseProofClockSkew)) {
		return fmt.Errorf("license proof expired")
	}

	publicKey, err := base64.RawURLEncoding.DecodeString(configuredLicenseProofPublicKey())
	if err != nil {
		return fmt.Errorf("decode license proof public key: %w", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token.Signature))
	if err != nil {
		return fmt.Errorf("decode license proof signature: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), []byte(commitTokenPayload(token)), signature) {
		return fmt.Errorf("license proof signature mismatch")
	}
	return nil
}

func configuredLicenseProofPublicKey() string {
	if value := strings.TrimSpace(os.Getenv(licenseProofPublicKeyEnv)); value != "" {
		return value
	}
	if value := strings.TrimSpace(EmbeddedLicenseProofPublicKey); value != "" {
		return value
	}
	return defaultLicenseProofPublicKey
}

func isLegacyUnsignedCommitToken(token CommitToken) bool {
	return strings.TrimSpace(token.ProofVersion) == "" &&
		strings.TrimSpace(token.Signature) == "" &&
		token.IssuedAt.IsZero() &&
		token.ExpiresAt.IsZero() &&
		strings.TrimSpace(token.RequestNonce) == "" &&
		strings.TrimSpace(token.Action) == "" &&
		strings.TrimSpace(token.DocumentType) == "" &&
		strings.TrimSpace(token.RuntimeMode) == ""
}

func commitTokenPayload(token CommitToken) string {
	parts := []string{
		"version=" + strings.TrimSpace(token.ProofVersion),
		"fingerprint_hash=" + strings.TrimSpace(token.FingerprintHash),
		"user_id=" + strconv.FormatUint(token.UserID, 10),
		"request_id=" + strings.TrimSpace(token.RequestID),
		"access_mode=" + strings.TrimSpace(string(token.AccessMode)),
		"api_key_hint=" + strings.TrimSpace(token.APIKeyHint),
		"action=" + strings.TrimSpace(token.Action),
		"document_type=" + strings.TrimSpace(token.DocumentType),
		"runtime_mode=" + strings.TrimSpace(token.RuntimeMode),
		"request_nonce=" + strings.TrimSpace(token.RequestNonce),
		"issued_at=" + token.IssuedAt.UTC().Format(time.RFC3339Nano),
		"expires_at=" + token.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
	return strings.Join(parts, "\n")
}
