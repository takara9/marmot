package marmotd

import (
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

const autoVNIStart = 100
const maxVNI = 16777215

func applyVirtualNetworkDefaults(network *api.VirtualNetwork, cfg *MarmotdConfig, database *db.Database) error {
	if network == nil || network.Spec == nil {
		return nil
	}
	if network.Spec.OverlayMode == nil {
		overlayMode := api.Vxlan
		network.Spec.OverlayMode = &overlayMode
	}
	if !strings.EqualFold(string(*network.Spec.OverlayMode), string(api.Vxlan)) {
		return nil
	}
	if network.Spec.PeerPolicy == nil {
		peerPolicy := api.Auto
		network.Spec.PeerPolicy = &peerPolicy
	}

	if network.Spec.UnderlayInterface == nil || strings.TrimSpace(*network.Spec.UnderlayInterface) == "" {
		if cfg != nil && strings.TrimSpace(cfg.DefaultUnderlayInterface) != "" {
			network.Spec.UnderlayInterface = util.StringPtr(strings.TrimSpace(cfg.DefaultUnderlayInterface))
		}
	}

	if network.Spec.Vni != nil {
		if *network.Spec.Vni < 0 || *network.Spec.Vni > maxVNI {
			return fmt.Errorf("invalid vni %d", *network.Spec.Vni)
		}
		return nil
	}
	if database == nil {
		return fmt.Errorf("database is required to auto-assign vni")
	}

	networks, err := database.GetVirtualNetworks()
	if err != nil {
		return err
	}
	vni, err := nextAvailableVNI(networks)
	if err != nil {
		return err
	}
	network.Spec.Vni = util.IntPtrInt(vni)
	return nil
}

func nextAvailableVNI(networks []api.VirtualNetwork) (int, error) {
	used := make(map[int]struct{}, len(networks))
	for _, network := range networks {
		if network.Spec == nil || network.Spec.Vni == nil {
			continue
		}
		vni := *network.Spec.Vni
		if vni < 0 || vni > maxVNI {
			continue
		}
		used[vni] = struct{}{}
	}

	for candidate := autoVNIStart; candidate <= maxVNI; candidate++ {
		if _, exists := used[candidate]; !exists {
			return candidate, nil
		}
	}

	return 0, fmt.Errorf("no available vni")
}
