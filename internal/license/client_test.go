package license

import "testing"

func TestDecodeEnvelope_AcceptsWrappedPayload(t *testing.T) {
	body := []byte(`{"data":{"allowed":true,"access_mode":"paid","paid_quota_remaining":12}}`)
	var result CheckResult
	if err := decodeEnvelope(body, &result); err != nil {
		t.Fatalf("decodeEnvelope: %v", err)
	}
	if !result.Allowed || result.AccessMode != AccessModePaid || result.PaidQuotaRemaining != 12 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDecodeEnvelope_AcceptsPlainPayload(t *testing.T) {
	body := []byte(`{"access_mode":"paid","remaining":9}`)
	var result ConsumeResult
	if err := decodeEnvelope(body, &result); err != nil {
		t.Fatalf("decodeEnvelope: %v", err)
	}
	if result.AccessMode != AccessModePaid || result.Remaining != 9 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
