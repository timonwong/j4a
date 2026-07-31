package apperr

import "testing"

func TestPartialFailureHasStableExitCode(t *testing.T) {
	err := New(KindPartialFailure, "some items failed")
	if got := ExitCode(err); got != 7 {
		t.Fatalf("ExitCode() = %d, want 7", got)
	}
	if got := string(As(err).Kind); got != "partial_failure" {
		t.Fatalf("kind = %q, want partial_failure", got)
	}
}
