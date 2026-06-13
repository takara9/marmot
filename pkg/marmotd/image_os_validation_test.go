package marmotd

import (
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestValidateImageOSSpecAllowsEmptyPair(t *testing.T) {
	err := validateImageOSSpec(&api.ImageSpec{})
	if err != nil {
		t.Fatalf("validateImageOSSpec() unexpected error: %v", err)
	}
}

func TestValidateImageOSSpecRequiresPair(t *testing.T) {
	err := validateImageOSSpec(&api.ImageSpec{OsVersion: util.StringPtr("24.04")})
	if err == nil {
		t.Fatalf("validateImageOSSpec() expected error when osName is missing")
	}

	err = validateImageOSSpec(&api.ImageSpec{OsName: util.StringPtr("ubuntu")})
	if err == nil {
		t.Fatalf("validateImageOSSpec() expected error when osVersion is missing")
	}
}

func TestValidateImageOSSpecRejectsUppercaseName(t *testing.T) {
	err := validateImageOSSpec(&api.ImageSpec{
		OsName:    util.StringPtr("Ubuntu"),
		OsVersion: util.StringPtr("24.04"),
	})
	if err == nil {
		t.Fatalf("validateImageOSSpec() expected lowercase validation error")
	}
}

func TestValidateImageOSSpecSupportsRequestedOSMatrix(t *testing.T) {
	tests := []struct {
		name      string
		osName    string
		osVersion string
	}{
		{name: "alpine", osName: "alpine", osVersion: "3.23"},
		{name: "ubuntu", osName: "ubuntu", osVersion: "26.04"},
		{name: "rockey", osName: "rockey", osVersion: "9"},
		{name: "debian", osName: "debian", osVersion: "13"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateImageOSSpec(&api.ImageSpec{
				OsName:    util.StringPtr(tt.osName),
				OsVersion: util.StringPtr(tt.osVersion),
			})
			if err != nil {
				t.Fatalf("validateImageOSSpec() unexpected error: %v", err)
			}
		})
	}
}

func TestValidateImageOSSpecRejectsInvalidVersionPerOS(t *testing.T) {
	err := validateImageOSSpec(&api.ImageSpec{
		OsName:    util.StringPtr("alpine"),
		OsVersion: util.StringPtr("3.24"),
	})
	if err == nil {
		t.Fatalf("validateImageOSSpec() expected version validation error")
	}

	err = validateImageOSSpec(&api.ImageSpec{
		OsName:    util.StringPtr("debian"),
		OsVersion: util.StringPtr("12"),
	})
	if err == nil {
		t.Fatalf("validateImageOSSpec() expected version validation error")
	}
}

func TestValidateImageOSSpecRejectsTypoOSNames(t *testing.T) {
	err := validateImageOSSpec(&api.ImageSpec{
		OsName:    util.StringPtr("rockery"),
		OsVersion: util.StringPtr("8"),
	})
	if err == nil {
		t.Fatalf("validateImageOSSpec() expected osName validation error for rockery")
	}

	err = validateImageOSSpec(&api.ImageSpec{
		OsName:    util.StringPtr("debina"),
		OsVersion: util.StringPtr("13"),
	})
	if err == nil {
		t.Fatalf("validateImageOSSpec() expected osName validation error for debina")
	}
}
