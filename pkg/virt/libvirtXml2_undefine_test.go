package virt

import (
	"errors"
	"testing"

	"libvirt.org/go/libvirt"
)

type fakeUndefiner struct {
	calls []libvirt.DomainUndefineFlagsValues
	errs  []error
}

func (f *fakeUndefiner) UndefineFlags(flags libvirt.DomainUndefineFlagsValues) error {
	f.calls = append(f.calls, flags)
	if len(f.errs) >= len(f.calls) {
		return f.errs[len(f.calls)-1]
	}
	return nil
}

// UEFI(NVRAM)と TPM を持つドメイン(Windows)を削除できるよう、両方の状態を削除するフラグを渡す。
func TestUndefineDomainWithStateRemovesNVRAMAndTPM(t *testing.T) {
	f := &fakeUndefiner{}
	if err := undefineDomainWithState(f); err != nil {
		t.Fatal(err)
	}
	want := libvirt.DOMAIN_UNDEFINE_NVRAM | libvirt.DOMAIN_UNDEFINE_TPM
	if len(f.calls) != 1 || f.calls[0] != want {
		t.Fatalf("calls = %v, want [%v]", f.calls, want)
	}
}

// TPM 削除フラグ未対応の libvirt(8.9 未満)では、NVRAM のみで再試行する。
func TestUndefineDomainWithStateFallsBackWithoutTPM(t *testing.T) {
	f := &fakeUndefiner{errs: []error{libvirt.Error{Code: libvirt.ERR_INVALID_ARG}}}
	if err := undefineDomainWithState(f); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || f.calls[1] != libvirt.DOMAIN_UNDEFINE_NVRAM {
		t.Fatalf("calls = %v", f.calls)
	}
}

// 未対応以外のエラーは再試行せずに返す。
func TestUndefineDomainWithStateReturnsOtherErrors(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeUndefiner{errs: []error{boom}}
	if err := undefineDomainWithState(f); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if len(f.calls) != 1 {
		t.Fatalf("unexpected retry: %v", f.calls)
	}
}
