package officegen

import "testing"

func TestNewDOCXGeneratorAvailable(t *testing.T) {
	gen := NewDOCXGenerator()
	if gen == nil {
		t.Fatal("expected generator")
	}
}
