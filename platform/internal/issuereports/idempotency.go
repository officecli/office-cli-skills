// Package issuereports implements idempotency-key derivation and timestamp
// validation for OfficeDex issue reports. It is a pure-computation package
// with no logger or metrics dependencies; callers are responsible for
// surfacing fallbackEngaged via their own observability.
package issuereports

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxTimestampDrift = 5 * time.Minute

// DeriveInput holds the parameters needed to derive a client event ID.
type DeriveInput struct {
	UserID           *uint64   // nil = anonymous
	PayloadTimestamp string    // desktop RFC3339 string, may be empty
	PayloadRequestID string    // desktop requestId, may be empty
	Summary          string    // full summary (pre-truncation)
	Now              time.Time // injected for testability
}

// DeriveClientEventID produces a deterministic idempotency key for an issue
// report. Anonymous callers receive a unique UUID-based key. Authenticated
// callers get a content-derived key so duplicate submissions can be detected.
//
// Return values:
//   - clientEventID: the derived key
//   - fallbackEngaged: true when the payload timestamp was unusable and
//     server time was substituted
//   - normalizedClientTsMs: the millisecond timestamp used in the derivation
//   - serverNowMs: the server's current time in milliseconds
func DeriveClientEventID(in DeriveInput) (clientEventID string, fallbackEngaged bool, normalizedClientTsMs int64, serverNowMs int64) {
	serverNowMs = in.Now.UnixMilli()

	// Anonymous path.
	if in.UserID == nil {
		return "v1:anon:" + uuid.NewString(), false, serverNowMs, serverNowMs
	}

	// Authenticated path: try to use the client-supplied timestamp.
	tsMs, ok := parseAndValidateTimestamp(in.PayloadTimestamp, in.Now)
	if !ok {
		tsMs = serverNowMs
		fallbackEngaged = true
	}
	normalizedClientTsMs = tsMs

	// Build the hash preimage.
	summaryHash := sha256.Sum256([]byte(in.Summary))
	preimage := strings.Join([]string{
		strconv.FormatUint(*in.UserID, 10),
		strconv.FormatInt(tsMs, 10),
		in.PayloadRequestID,
		hex.EncodeToString(summaryHash[:]),
	}, "|")

	h := sha256.Sum256([]byte(preimage))
	clientEventID = "v1:" + hex.EncodeToString(h[:])
	return clientEventID, fallbackEngaged, normalizedClientTsMs, serverNowMs
}

// parseAndValidateTimestamp parses a RFC3339 timestamp string and validates it
// against the server's current time. It enforces UTC-only ("Z" suffix) and a
// symmetric 5-minute drift window.
func parseAndValidateTimestamp(raw string, now time.Time) (tsMs int64, ok bool) {
	if raw == "" {
		return 0, false
	}

	// Reject any timezone offset — only accept explicit "Z" suffix.
	if !strings.HasSuffix(raw, "Z") {
		return 0, false
	}

	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0, false
	}

	tsMs = t.UnixMilli()
	nowMs := now.UnixMilli()
	diff := tsMs - nowMs
	if diff < 0 {
		diff = -diff
	}
	if diff > maxTimestampDrift.Milliseconds() {
		return 0, false
	}

	return tsMs, true
}
