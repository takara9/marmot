package cmd

import (
	"strings"
	"testing"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

func TestValidateKubernetesEngineApplyForbiddenChanges(t *testing.T) {
	existing := api.KubernetesEngine{
		ApiVersion: "v1",
		Kind:       "KubernetesEngine",
		Metadata: api.Metadata{
			Id:   "mke01",
			Name: "cluster-1",
		},
		Spec: api.KubernetesEngineSpec{
			Version: "1.30",
			Nodes:   3,
			NodeSpec: &api.KubernetesEngineNodeSpec{
				Cpu:    util.IntPtrInt(2),
				Memory: util.IntPtrInt(4096),
				Network: &api.KubernetesEngineNodeNetwork{
					Kind:     util.StringPtr("cilium"),
					External: util.StringPtr("default"),
				},
			},
		},
	}

	t.Run("allow spec.nodes change", func(t *testing.T) {
		desired := existing
		desired.Spec.Nodes = 5
		if err := validateKubernetesEngineApplyForbiddenChanges(existing, desired); err != nil {
			t.Fatalf("validateKubernetesEngineApplyForbiddenChanges() unexpected err: %v", err)
		}
	})

	t.Run("allow spec.version change", func(t *testing.T) {
		desired := existing
		desired.Spec.Version = "1.31"
		if err := validateKubernetesEngineApplyForbiddenChanges(existing, desired); err != nil {
			t.Fatalf("validateKubernetesEngineApplyForbiddenChanges() unexpected err: %v", err)
		}
	})

	t.Run("reject spec.nodeSpec change", func(t *testing.T) {
		desired := existing
		desired.Spec.NodeSpec = &api.KubernetesEngineNodeSpec{
			Cpu:    util.IntPtrInt(4),
			Memory: util.IntPtrInt(4096),
			Network: &api.KubernetesEngineNodeNetwork{
				Kind:     util.StringPtr("cilium"),
				External: util.StringPtr("default"),
			},
		}
		err := validateKubernetesEngineApplyForbiddenChanges(existing, desired)
		if err == nil {
			t.Fatalf("validateKubernetesEngineApplyForbiddenChanges() expected err, got nil")
		}
		if !strings.Contains(err.Error(), "spec.nodeSpec") {
			t.Fatalf("expected error to mention spec.nodeSpec, got: %v", err)
		}
	})

	t.Run("reject apiVersion change", func(t *testing.T) {
		desired := existing
		desired.ApiVersion = "v2"
		err := validateKubernetesEngineApplyForbiddenChanges(existing, desired)
		if err == nil {
			t.Fatalf("validateKubernetesEngineApplyForbiddenChanges() expected err, got nil")
		}
		if !strings.Contains(err.Error(), "apiVersion") {
			t.Fatalf("expected error to mention apiVersion, got: %v", err)
		}
	})

	t.Run("reject metadata.name change", func(t *testing.T) {
		desired := existing
		desired.Metadata.Name = "cluster-2"
		err := validateKubernetesEngineApplyForbiddenChanges(existing, desired)
		if err == nil {
			t.Fatalf("validateKubernetesEngineApplyForbiddenChanges() expected err, got nil")
		}
		if !strings.Contains(err.Error(), "metadata.name") {
			t.Fatalf("expected error to mention metadata.name, got: %v", err)
		}
	})

	t.Run("allow when non-controlled fields omitted", func(t *testing.T) {
		desired := api.KubernetesEngine{
			Spec: api.KubernetesEngineSpec{
				NodeSpec: existing.Spec.NodeSpec,
			},
		}
		if err := validateKubernetesEngineApplyForbiddenChanges(existing, desired); err != nil {
			t.Fatalf("validateKubernetesEngineApplyForbiddenChanges() unexpected err: %v", err)
		}
	})
}
