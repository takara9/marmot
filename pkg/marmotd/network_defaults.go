package marmotd

import (
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

const maxVNI = 16777215

func applyVirtualNetworkDefaults(network *api.VirtualNetwork, cfg *MarmotdConfig, database *db.Database) error {
	if network == nil {
		return nil
	}
	if network.Spec.OverlayMode == nil {
		overlayMode := api.Geneve
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
	return nil
}
