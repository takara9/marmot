package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestValidateServerAuthSpecRejectsUserAndUsersTogether(t *testing.T) {
	user := util.StringPtr("root")
	users := &[]string{"root", "ubuntu"}
	auth := &api.Auth{User: user, Users: users}

	if err := validateServerAuthSpec(auth); err == nil {
		t.Fatalf("validateServerAuthSpec() expected error for user/users combination")
	}
}

func TestValidateServerAuthSpecRejectsEmptyUsernames(t *testing.T) {
	user := util.StringPtr("   ")
	users := &[]string{"root", ""}

	if err := validateServerAuthSpec(&api.Auth{User: user}); err == nil {
		t.Fatalf("validateServerAuthSpec() expected error for empty user")
	}
	if err := validateServerAuthSpec(&api.Auth{Users: users}); err == nil {
		t.Fatalf("validateServerAuthSpec() expected error for empty users entry")
	}
}
