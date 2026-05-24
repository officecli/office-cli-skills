package issuereports

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

var fixedNow = time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

func TestDeriveClientEventID_ValidWindow(t *testing.T) {
	// T1: payload.timestamp = now - 30s → no fallback, stable derivation.
	ts := fixedNow.Add(-30 * time.Second).UTC().Format(time.RFC3339)
	in := DeriveInput{
		UserID:           ptr(uint64(42)),
		PayloadTimestamp: ts,
		PayloadRequestID: "req-1",
		Summary:          "app crashed on open",
		Now:              fixedNow,
	}

	id, fb, clientTs, serverTs := DeriveClientEventID(in)

	require.False(t, fb, "fallback should not engage for valid timestamp")
	require.True(t, strings.HasPrefix(id, "v1:"), "must have v1 prefix")
	require.Equal(t, fixedNow.UnixMilli(), serverTs)
	require.Equal(t, fixedNow.Add(-30*time.Second).UnixMilli(), clientTs)

	// Confirm the result is deterministic.
	id2, _, _, _ := DeriveClientEventID(in)
	require.Equal(t, id, id2, "same input must produce same ID")
}

func TestDeriveClientEventID_PastBeyondWindow(t *testing.T) {
	// T2: payload.timestamp = now - 10min → fallback engaged.
	ts := fixedNow.Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	in := DeriveInput{
		UserID:           ptr(uint64(42)),
		PayloadTimestamp: ts,
		PayloadRequestID: "req-2",
		Summary:          "app crashed on open",
		Now:              fixedNow,
	}

	_, fb, clientTs, serverTs := DeriveClientEventID(in)

	require.True(t, fb, "fallback must engage for stale timestamp")
	require.Equal(t, serverTs, clientTs, "fallback uses server time")
}

func TestDeriveClientEventID_FutureBeyondWindow(t *testing.T) {
	// T3: payload.timestamp = now + 10min → fallback engaged.
	ts := fixedNow.Add(10 * time.Minute).UTC().Format(time.RFC3339)
	in := DeriveInput{
		UserID:           ptr(uint64(42)),
		PayloadTimestamp: ts,
		PayloadRequestID: "req-3",
		Summary:          "app crashed on open",
		Now:              fixedNow,
	}

	_, fb, clientTs, serverTs := DeriveClientEventID(in)

	require.True(t, fb, "fallback must engage for future timestamp")
	require.Equal(t, serverTs, clientTs, "fallback uses server time")
}

func TestDeriveClientEventID_TimezoneOffsetRejected(t *testing.T) {
	// T4: timezone offset string → fallback engaged.
	in := DeriveInput{
		UserID:           ptr(uint64(42)),
		PayloadTimestamp: "2026-05-24T08:42:00+08:00",
		PayloadRequestID: "req-4",
		Summary:          "app crashed on open",
		Now:              fixedNow,
	}

	_, fb, clientTs, serverTs := DeriveClientEventID(in)

	require.True(t, fb, "fallback must engage for timezone-offset timestamp")
	require.Equal(t, serverTs, clientTs, "fallback uses server time")
}

func TestDeriveClientEventID_Anonymous(t *testing.T) {
	// T5: anonymous → "v1:anon:" prefix + UUID form.
	in := DeriveInput{
		UserID:           nil,
		PayloadTimestamp: fixedNow.UTC().Format(time.RFC3339),
		PayloadRequestID: "req-5",
		Summary:          "app crashed on open",
		Now:              fixedNow,
	}

	id, fb, _, _ := DeriveClientEventID(in)

	require.False(t, fb)
	require.True(t, strings.HasPrefix(id, "v1:anon:"), "anonymous must have v1:anon: prefix")
	// UUID portion must be 36 chars (8-4-4-4-12).
	uuidPart := strings.TrimPrefix(id, "v1:anon:")
	require.Len(t, uuidPart, 36, "UUID part should be 36 chars")
}

func TestDeriveClientEventID_Idempotent(t *testing.T) {
	// T6: same input twice → identical output (idempotency proof).
	ts := fixedNow.Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	in := DeriveInput{
		UserID:           ptr(uint64(99)),
		PayloadTimestamp: ts,
		PayloadRequestID: "req-idem",
		Summary:          "reproducible crash in editor",
		Now:              fixedNow,
	}

	id1, fb1, ts1, srv1 := DeriveClientEventID(in)
	id2, fb2, ts2, srv2 := DeriveClientEventID(in)

	require.Equal(t, id1, id2, "idempotent: same input must yield same ID")
	require.Equal(t, fb1, fb2)
	require.Equal(t, ts1, ts2)
	require.Equal(t, srv1, srv2)
}
