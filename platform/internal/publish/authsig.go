package publish

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

func canonicalPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return "/"
	}
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	values := r.URL.Query()
	if len(values) == 0 {
		return path
	}
	normalized := url.Values{}
	for key, vals := range values {
		sorted := append([]string(nil), vals...)
		sort.Strings(sorted)
		for _, value := range sorted {
			normalized.Add(key, value)
		}
	}
	encoded := normalized.Encode()
	if encoded == "" {
		return path
	}
	return path + "?" + encoded
}

func bodySHA256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func signDynamic(secret, timestamp, method, path, bodyHash string) string {
	base := strings.Join([]string{
		strings.TrimSpace(timestamp),
		strings.ToUpper(strings.TrimSpace(method)),
		strings.TrimSpace(path),
		strings.TrimSpace(bodyHash),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
