package marmotd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// stubAptCacherSystemctl は is-active / is-enabled の結果を固定し、変更系コマンドを記録する。
func stubAptCacherSystemctl(t *testing.T, active, enabled bool, changeErr error) *[]string {
	t.Helper()
	var changes []string
	orig := aptCacherNGSystemctl
	aptCacherNGSystemctl = func(args ...string) error {
		switch args[0] {
		case "is-active":
			if !active {
				return fmt.Errorf("inactive")
			}
			return nil
		case "is-enabled":
			if !enabled {
				return fmt.Errorf("disabled")
			}
			return nil
		}
		changes = append(changes, strings.Join(args, " "))
		return changeErr
	}
	t.Cleanup(func() { aptCacherNGSystemctl = orig })
	return &changes
}

func TestEnsureManagementAptCacher(t *testing.T) {
	enabledCfg := &MarmotdConfig{AptCacherNGEnabled: true}

	tests := []struct {
		name            string
		cfg             *MarmotdConfig
		head            bool
		active, enabled bool
		want            []string
	}{
		{name: "disabled config does nothing on head", cfg: &MarmotdConfig{}, head: true},
		{name: "disabled config does nothing on follower", cfg: &MarmotdConfig{}, head: false, active: true, enabled: true},
		{name: "nil config does nothing", cfg: nil, head: true},
		{name: "head starts and enables when stopped", cfg: enabledCfg, head: true, want: []string{"enable --now apt-cacher-ng.service"}},
		{name: "head enables when only active", cfg: enabledCfg, head: true, active: true, want: []string{"enable --now apt-cacher-ng.service"}},
		{name: "head leaves running service alone", cfg: enabledCfg, head: true, active: true, enabled: true},
		{name: "follower stops and disables when running", cfg: enabledCfg, head: false, active: true, enabled: true, want: []string{"disable --now apt-cacher-ng.service"}},
		{name: "follower disables when only enabled", cfg: enabledCfg, head: false, enabled: true, want: []string{"disable --now apt-cacher-ng.service"}},
		{name: "follower leaves stopped service alone", cfg: enabledCfg, head: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := stubAptCacherSystemctl(t, tt.active, tt.enabled, nil)
			if err := EnsureManagementAptCacher(tt.cfg, tt.head); err != nil {
				t.Fatalf("EnsureManagementAptCacher() error = %v", err)
			}
			if !reflect.DeepEqual(*changes, tt.want) && (len(*changes) != 0 || len(tt.want) != 0) {
				t.Fatalf("systemctl changes = %v, want %v", *changes, tt.want)
			}
		})
	}
}

func TestEnsureManagementAptCacher_PropagatesSystemctlError(t *testing.T) {
	stubAptCacherSystemctl(t, false, false, fmt.Errorf("boom"))
	if err := EnsureManagementAptCacher(&MarmotdConfig{AptCacherNGEnabled: true}, true); err == nil {
		t.Fatal("EnsureManagementAptCacher() error = nil, want error when systemctl fails")
	}
}
