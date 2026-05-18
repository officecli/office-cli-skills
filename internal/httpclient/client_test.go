package httpclient

import (
	"net/http"
	"testing"
	"time"
)

func TestNewUsesDefaultTransportOutsideDevProfile(t *testing.T) {
	t.Setenv("OFFICE_CLI_PROFILE", "")
	t.Setenv("OFFICECLI_DEV_INSECURE_TLS", "1")

	client := New(time.Second)
	if client.Timeout != time.Second {
		t.Fatalf("timeout = %s", client.Timeout)
	}
	if client.Transport != nil {
		t.Fatalf("transport = %#v, want default nil transport", client.Transport)
	}
}

func TestNewAllowsInsecureTLSOnlyForDevProfile(t *testing.T) {
	t.Setenv("OFFICE_CLI_PROFILE", "dev")
	t.Setenv("OFFICECLI_DEV_INSECURE_TLS", "1")

	client := New(time.Second)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %#v", client.Transport)
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("TLS config = %#v", transport.TLSClientConfig)
	}
}

func TestNewKeepsTLSVerificationWhenDevFlagIsUnset(t *testing.T) {
	t.Setenv("OFFICE_CLI_PROFILE", "dev")
	t.Setenv("OFFICECLI_DEV_INSECURE_TLS", "")

	client := New(time.Second)
	if transport, ok := client.Transport.(*http.Transport); ok && transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("unexpected insecure TLS config: %#v", transport.TLSClientConfig)
	}
}
