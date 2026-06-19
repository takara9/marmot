package cmd

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestValidateServerApplyForbiddenChanges(t *testing.T) {
	baseLabels := map[string]interface{}{"env": "dev"}
	existing := api.Server{
		ApiVersion: "v1",
		Kind:       "Server",
		Metadata: api.Metadata{
			Id:      "srv01",
			Name:    "server-1",
			Comment: util.StringPtr("old-comment"),
			Labels:  &baseLabels,
		},
		Spec: api.ServerSpec{
			Cpu:    util.IntPtrInt(2),
			Memory: util.IntPtrInt(2048),
			OsVg:   util.StringPtr("vg1"),
		},
	}

	t.Run("allow when non-controlled fields omitted", func(t *testing.T) {
		desired := api.Server{}
		if err := validateServerApplyForbiddenChanges(existing, desired); err != nil {
			t.Fatalf("validateServerApplyForbiddenChanges() unexpected err: %v", err)
		}
	})

	t.Run("allow metadata.labels change", func(t *testing.T) {
		newLabels := map[string]interface{}{"env": "prod"}
		desired := api.Server{Metadata: api.Metadata{Labels: &newLabels}}
		if err := validateServerApplyForbiddenChanges(existing, desired); err != nil {
			t.Fatalf("validateServerApplyForbiddenChanges() unexpected err: %v", err)
		}
	})

	t.Run("allow metadata.comment change", func(t *testing.T) {
		desired := api.Server{Metadata: api.Metadata{Comment: util.StringPtr("new-comment")}}
		if err := validateServerApplyForbiddenChanges(existing, desired); err != nil {
			t.Fatalf("validateServerApplyForbiddenChanges() unexpected err: %v", err)
		}
	})

	t.Run("allow spec.memory change", func(t *testing.T) {
		desired := api.Server{Spec: api.ServerSpec{Memory: util.IntPtrInt(4096)}}
		if err := validateServerApplyForbiddenChanges(existing, desired); err != nil {
			t.Fatalf("validateServerApplyForbiddenChanges() unexpected err: %v", err)
		}
	})

	t.Run("allow spec.cpu change", func(t *testing.T) {
		desired := api.Server{Spec: api.ServerSpec{Cpu: util.IntPtrInt(4)}}
		if err := validateServerApplyForbiddenChanges(existing, desired); err != nil {
			t.Fatalf("validateServerApplyForbiddenChanges() unexpected err: %v", err)
		}
	})

	t.Run("allow cpu change with minimal networkInterface request", func(t *testing.T) {
		existingWithNIC := existing
		existingWithNIC.Spec.NetworkInterface = &[]api.NetworkInterface{
			{
				Networkname: "host-bridge",
				Networkid:   "default",
				Address:     util.StringPtr("192.168.1.20"),
				IpNetworkId: util.StringPtr("ip-0001"),
			},
		}

		desired := api.Server{
			Spec: api.ServerSpec{
				Cpu: util.IntPtrInt(2),
				NetworkInterface: &[]api.NetworkInterface{
					{
						Networkname: "host-bridge",
					},
				},
			},
		}

		if err := validateServerApplyForbiddenChanges(existingWithNIC, desired); err != nil {
			t.Fatalf("validateServerApplyForbiddenChanges() unexpected err: %v", err)
		}
	})

	t.Run("reject networkInterface networkname change", func(t *testing.T) {
		existingWithNIC := existing
		existingWithNIC.Spec.NetworkInterface = &[]api.NetworkInterface{
			{
				Networkname: "host-bridge",
			},
		}

		desired := api.Server{
			Spec: api.ServerSpec{
				NetworkInterface: &[]api.NetworkInterface{
					{
						Networkname: "another-network",
					},
				},
			},
		}

		err := validateServerApplyForbiddenChanges(existingWithNIC, desired)
		if err == nil {
			t.Fatalf("validateServerApplyForbiddenChanges() expected error")
		}
		if !strings.Contains(err.Error(), "spec.networkInterface") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject spec.osVg change", func(t *testing.T) {
		desired := api.Server{Spec: api.ServerSpec{OsVg: util.StringPtr("vg2")}}
		err := validateServerApplyForbiddenChanges(existing, desired)
		if err == nil {
			t.Fatalf("validateServerApplyForbiddenChanges() expected error")
		}
		if !strings.Contains(err.Error(), "spec.osVg") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject metadata.nodeName change", func(t *testing.T) {
		desired := api.Server{Metadata: api.Metadata{NodeName: util.StringPtr("node-2")}}
		err := validateServerApplyForbiddenChanges(existing, desired)
		if err == nil {
			t.Fatalf("validateServerApplyForbiddenChanges() expected error")
		}
		if !strings.Contains(err.Error(), "metadata.nodeName") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("reject multiple non-allowed changes", func(t *testing.T) {
		desired := api.Server{
			Metadata: api.Metadata{
				NodeName: util.StringPtr("node-2"),
			},
			Spec: api.ServerSpec{
				OsVg: util.StringPtr("vg2"),
			},
		}
		err := validateServerApplyForbiddenChanges(existing, desired)
		if err == nil {
			t.Fatalf("validateServerApplyForbiddenChanges() expected error")
		}
		for _, field := range []string{"metadata.nodeName", "spec.osVg"} {
			if !strings.Contains(err.Error(), field) {
				t.Fatalf("error does not include %s: %v", field, err)
			}
		}
	})
}
