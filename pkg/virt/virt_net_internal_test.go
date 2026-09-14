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

func TestIsNetworkAlreadyExistsError(t *testing.T) {
	if isNetworkAlreadyExistsError(nil) {
		t.Fatalf("nil error should not be considered already-exists")
	}

	alreadyExists := errors.New("virError(Code=9, Domain=19, Message='operation failed: network \"mgmt\" already exists with uuid 401591f5-38a3-4eff-9d1f-69449a6683fe')")
	if !isNetworkAlreadyExistsError(alreadyExists) {
		t.Fatalf("expected already-exists error to be recognized")
	}

	other := errors.New("virError(Code=55, Domain=19, Message='Requested operation is not valid: network is already active')")
	if isNetworkAlreadyExistsError(other) {
		t.Fatalf("unrelated error should not be recognized as already-exists")
	}
}
