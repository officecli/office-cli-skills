package license

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"strings"
)

func ComputeFingerprintHash() string {
	parts := []string{
		strings.TrimSpace(readFirstNonEmpty(
			os.Getenv("OFFICE_CLI_MACHINE_ID"),
			readFile("/etc/machine-id"),
			readFile("/var/lib/dbus/machine-id"),
		)),
		strings.TrimSpace(os.Getenv("COMPUTERNAME")),
		strings.TrimSpace(hostname()),
		runtime.GOOS,
		runtime.GOARCH,
	}
	joined := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}

func readFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}
