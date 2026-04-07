package license

import (
	"crypto/sha256"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/officecli/officecli/platform/internal/model"
)

const (
	licenseProofVersion          = "v1"
	defaultLicenseProofSeed      = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA"
	defaultLicenseProofTTL       = 2 * time.Minute
	defaultLicenseProofClockSkew = 30 * time.Second
)

type ProofConfig struct {
	Seed string
	TTL  time.Duration
}

type proofSigner struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	ttl        time.Duration
	clock      func() time.Time
}

func newProofSigner(cfg ProofConfig) (*proofSigner, error) {
	seedValue := strings.TrimSpace(cfg.Seed)
	if seedValue == "" {
		seedValue = defaultLicenseProofSeed
	}
	seed, err := base64.RawURLEncoding.DecodeString(seedValue)
	if err != nil {
		return nil, fmt.Errorf("decode license proof seed: %w", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("license proof seed must be %d bytes", ed25519.SeedSize)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(publicKey, privateKey.Public().(ed25519.PublicKey))
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = defaultLicenseProofTTL
	}
	return &proofSigner{
		privateKey: privateKey,
		publicKey:  publicKey,
		ttl:        ttl,
		clock:      time.Now,
	}, nil
}

func (s *proofSigner) issueCommitToken(req CheckRequest, accessMode model.AccessMode, apiKeyHint string) (*CommitToken, error) {
	if s == nil {
		return nil, fmt.Errorf("license proof signer is unavailable")
	}
	now := time.Now().UTC()
	if s.clock != nil {
		now = s.clock().UTC()
	}
	token := &CommitToken{
		FingerprintHash: strings.TrimSpace(req.FingerprintHash),
		UserID:          req.UserID,
		RequestID:       buildRequestID(req),
		AccessMode:      accessMode,
		APIKeyHint:      strings.TrimSpace(apiKeyHint),
		Action:          strings.TrimSpace(req.Action),
		DocumentType:    strings.TrimSpace(req.DocumentType),
		RuntimeMode:     strings.TrimSpace(req.RuntimeMode),
		RequestNonce:    strings.TrimSpace(req.RequestNonce),
		ProofVersion:    licenseProofVersion,
		IssuedAt:        now,
		ExpiresAt:       now.Add(s.ttl),
	}
	token.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(s.privateKey, []byte(commitTokenPayload(*token))))
	return token, nil
}

func (s *proofSigner) verifyCommitToken(token CommitToken, req ConsumeRequest, accessMode model.AccessMode) error {
	if s == nil {
		return fmt.Errorf("license proof signer is unavailable")
	}
	if strings.TrimSpace(token.ProofVersion) != licenseProofVersion {
		return fmt.Errorf("license proof version mismatch")
	}
	if strings.TrimSpace(token.Signature) == "" {
		return fmt.Errorf("license proof signature is required")
	}
	if token.FingerprintHash != strings.TrimSpace(req.FingerprintHash) {
		return fmt.Errorf("license proof fingerprint mismatch")
	}
	if token.UserID != req.UserID {
		return fmt.Errorf("license proof user mismatch")
	}
	if token.RequestID != strings.TrimSpace(req.RequestID) {
		return fmt.Errorf("license proof request id mismatch")
	}
	if token.AccessMode != accessMode || token.AccessMode != req.AccessMode {
		return fmt.Errorf("license proof access mode mismatch")
	}
	if token.Action != string(model.UsageActionGenerate) || strings.TrimSpace(req.UsageType) != string(model.UsageActionGenerate) {
		return fmt.Errorf("license proof usage action mismatch")
	}
	now := time.Now().UTC()
	if s.clock != nil {
		now = s.clock().UTC()
	}
	if token.IssuedAt.IsZero() || token.ExpiresAt.IsZero() {
		return fmt.Errorf("license proof missing validity window")
	}
	if now.Before(token.IssuedAt.Add(-defaultLicenseProofClockSkew)) {
		return fmt.Errorf("license proof issued in the future")
	}
	if now.After(token.ExpiresAt.Add(defaultLicenseProofClockSkew)) {
		return fmt.Errorf("license proof expired")
	}
	signature, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token.Signature))
	if err != nil {
		return fmt.Errorf("decode license proof signature: %w", err)
	}
	if !ed25519.Verify(s.publicKey, []byte(commitTokenPayload(token)), signature) {
		return fmt.Errorf("license proof signature mismatch")
	}
	return nil
}

func buildRequestID(req CheckRequest) string {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	payload := strings.Join([]string{
		strings.TrimSpace(req.FingerprintHash),
		strings.TrimSpace(req.Action),
		strings.TrimSpace(req.DocumentType),
		strings.TrimSpace(req.RuntimeMode),
		strings.TrimSpace(req.RequestNonce),
		now,
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return "req_" + hex.EncodeToString(sum[:])
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
