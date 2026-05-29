//go:build v11_self_test

package modify

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestV11FailsOnSyntheticViolation(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	packageDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(packageDir, "..", "..", ".."))
	syntheticPath := filepath.Join(packageDir, "zz_v11_synthetic_violation.go")

	const syntheticSource = `package modify

import _ "github.com/officecli/officecli-internal/internal/runtime"
`
	if err := os.WriteFile(syntheticPath, []byte(syntheticSource), 0o644); err != nil {
		t.Fatalf("write synthetic violation: %v", err)
	}
	defer func() {
		if err := os.Remove(syntheticPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove synthetic violation: %v", err)
		}
	}()

	cmd := exec.Command("bash", "-c", `set -o pipefail; out=$(go list -e -deps ./internal/runtime/modify | grep -E "^github.com/.+/internal/runtime$" || true); test -z "$out"`)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected V11 synthetic violation assertion to fail, output:\n%s", out)
	}
	if strings.Contains(string(out), "zz_v11_synthetic_violation.go") {
		t.Fatalf("V11 self-test should fail from dependency detection, not go list import-cycle output:\n%s", out)
	}
}
