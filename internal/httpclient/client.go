package httpclient

import (
	"crypto/tls"
	"net/http"
	"os"
	"strings"
	"time"
)

func New(timeout time.Duration) *http.Client {
	client := &http.Client{Timeout: timeout}
	if devInsecureTLS() {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec
	}
	return client
}

func devInsecureTLS() bool {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("OFFICE_CLI_PROFILE")), "dev") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OFFICECLI_DEV_INSECURE_TLS"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
