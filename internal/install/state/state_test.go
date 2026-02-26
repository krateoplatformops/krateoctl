package state

import "testing"

func TestMergeFinalizers(t *testing.T) {
	existing := []string{"alpha", InstallationFinalizer}
	desired := []string{InstallationFinalizer, "beta"}

	merged := mergeFinalizers(existing, desired)
	if len(merged) != 3 {
		t.Fatalf("expected 3 finalizers, got %v", merged)
	}

	if merged[0] != "alpha" || merged[1] != InstallationFinalizer || merged[2] != "beta" {
		t.Fatalf("finalizers not merged deterministically: %v", merged)
	}
}
