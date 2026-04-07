package license

import "testing"

func TestComputeFingerprintHashUsesOfficeCLIMachineIDOverride(t *testing.T) {
	t.Setenv("OFFICE_CLI_MACHINE_ID", "ci-fixed-machine")
	t.Setenv("COMPUTERNAME", "")

	got1 := ComputeFingerprintHash()
	got2 := ComputeFingerprintHash()
	if got1 == "" {
		t.Fatal("fingerprint hash should not be empty")
	}
	if got1 != got2 {
		t.Fatalf("fingerprint hash should be stable for same override: %q != %q", got1, got2)
	}

	t.Setenv("OFFICE_CLI_MACHINE_ID", "ci-fixed-machine-next")
	got3 := ComputeFingerprintHash()
	if got3 == "" {
		t.Fatal("fingerprint hash should not be empty after override change")
	}
	if got3 == got1 {
		t.Fatalf("fingerprint hash should change when OFFICE_CLI_MACHINE_ID changes: %q", got3)
	}
}
