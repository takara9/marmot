package virt

import (
	"errors"
	"testing"
	"time"

	"libvirt.org/go/libvirt"
)

func TestWaitForDomainShutoffAlreadyShutoff(t *testing.T) {
	getState := func() (libvirt.DomainState, int, error) {
		return libvirt.DOMAIN_SHUTOFF, 0, nil
	}
	if err := waitForDomainShutoff(getState, time.Second, time.Millisecond); err != nil {
		t.Fatalf("waitForDomainShutoff() unexpected err: %v", err)
	}
}

func TestWaitForDomainShutoffBecomesShutoffBeforeTimeout(t *testing.T) {
	calls := 0
	getState := func() (libvirt.DomainState, int, error) {
		calls++
		if calls < 3 {
			return libvirt.DOMAIN_RUNNING, 0, nil
		}
		return libvirt.DOMAIN_SHUTOFF, 0, nil
	}
	if err := waitForDomainShutoff(getState, time.Second, time.Millisecond); err != nil {
		t.Fatalf("waitForDomainShutoff() unexpected err: %v", err)
	}
	if calls < 3 {
		t.Fatalf("expected at least 3 polls, got %d", calls)
	}
}

func TestWaitForDomainShutoffTimesOut(t *testing.T) {
	getState := func() (libvirt.DomainState, int, error) {
		return libvirt.DOMAIN_RUNNING, 0, nil
	}
	err := waitForDomainShutoff(getState, 5*time.Millisecond, time.Millisecond)
	if err == nil {
		t.Fatalf("waitForDomainShutoff() expected timeout error")
	}
}

func TestWaitForDomainShutoffPropagatesGetStateError(t *testing.T) {
	wantErr := errors.New("get state failed")
	getState := func() (libvirt.DomainState, int, error) {
		return libvirt.DOMAIN_RUNNING, 0, wantErr
	}
	err := waitForDomainShutoff(getState, time.Second, time.Millisecond)
	if !errors.Is(err, wantErr) {
		t.Fatalf("waitForDomainShutoff() = %v, want %v", err, wantErr)
	}
}
