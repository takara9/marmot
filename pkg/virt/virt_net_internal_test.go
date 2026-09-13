package virt

import (
	"errors"
	"testing"
)

func TestIsNetworkAlreadyActiveError(t *testing.T) {
	if isNetworkAlreadyActiveError(nil) {
		t.Fatalf("nil error should not be considered already-active")
	}

	alreadyActive := errors.New("virError(Code=55, Domain=19, Message='Requested operation is not valid: network is already active')")
	if !isNetworkAlreadyActiveError(alreadyActive) {
		t.Fatalf("expected already-active error to be recognized")
	}

	other := errors.New("virError(Code=43, Domain=19, Message='Network not found')")
	if isNetworkAlreadyActiveError(other) {
		t.Fatalf("unrelated error should not be recognized as already-active")
	}
}
