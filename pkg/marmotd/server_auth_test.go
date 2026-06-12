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

func TestCloudInitAuthInputsAddsRootWhenRootPasswordIsSet(t *testing.T) {
	password := "password123"
	pass, _, usernames, err := cloudInitAuthInputs(&api.Auth{RootPassword: &password})
	if err != nil {
		t.Fatalf("cloudInitAuthInputs() unexpected error: %v", err)
	}
	if pass != password {
		t.Fatalf("cloudInitAuthInputs() password = %q, want %q", pass, password)
	}
	if len(usernames) != 1 || usernames[0] != "root" {
		t.Fatalf("cloudInitAuthInputs() usernames = %v, want [root]", usernames)
	}
}

func TestCloudInitAuthInputsDoesNotDuplicateRoot(t *testing.T) {
	password := "password123"
	users := &[]string{"root", "ubuntu"}
	_, _, usernames, err := cloudInitAuthInputs(&api.Auth{RootPassword: &password, Users: users})
	if err != nil {
		t.Fatalf("cloudInitAuthInputs() unexpected error: %v", err)
	}

	rootCount := 0
	for _, username := range usernames {
		if username == "root" {
			rootCount++
		}
	}
	if rootCount != 1 {
		t.Fatalf("cloudInitAuthInputs() root count = %d, want 1 (usernames=%v)", rootCount, usernames)
	}
}
