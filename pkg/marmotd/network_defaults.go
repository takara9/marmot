package marmotd

import (
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"github.com/takara9/marmot/pkg/util"
)

const maxVNI = 16777215
const minAutoVNI = 100

func applyVirtualNetworkDefaults(network *api.VirtualNetwork, cfg *MarmotdConfig, database *db.Database) error {
	if network == nil {
		return nil
	}
	if network.Spec.OverlayMode == nil || strings.TrimSpace(string(*network.Spec.OverlayMode)) == "" {
		overlayMode := api.Geneve
		network.Spec.OverlayMode = &overlayMode
	}

	isVxlan := strings.EqualFold(string(*network.Spec.OverlayMode), string(api.Vxlan))
	isGeneve := strings.EqualFold(string(*network.Spec.OverlayMode), string(api.Geneve))
	if !isVxlan && !isGeneve {
		return nil
	}

	if isVxlan && network.Spec.PeerPolicy == nil {
		peerPolicy := api.Auto
		network.Spec.PeerPolicy = &peerPolicy
	}

	if network.Spec.UnderlayInterface == nil || strings.TrimSpace(*network.Spec.UnderlayInterface) == "" {
		if cfg != nil && strings.TrimSpace(cfg.DefaultUnderlayInterface) != "" {
			network.Spec.UnderlayInterface = util.StringPtr(strings.TrimSpace(cfg.DefaultUnderlayInterface))
		}
	}

	usedVNIs, err := usedVNISet(database, strings.TrimSpace(network.Metadata.Id))
	if err != nil {
		return err
	}

	if network.Spec.Vni != nil {
		if *network.Spec.Vni < 0 || *network.Spec.Vni > maxVNI {
			return fmt.Errorf("invalid vni %d", *network.Spec.Vni)
		}
		if _, exists := usedVNIs[*network.Spec.Vni]; exists {
			return fmt.Errorf("vni %d is already in use", *network.Spec.Vni)
		}
		return nil
	}

	vni, err := nextAvailableVNI(usedVNIs)
	if err != nil {
		return err
	}
	network.Spec.Vni = util.IntPtrInt(vni)

	return nil
}

func usedVNISet(database *db.Database, excludeNetworkID string) (map[int]struct{}, error) {
	used := map[int]struct{}{}
	if database == nil {
		return used, nil
	}

	networks, err := database.GetVirtualNetworks()
	if err != nil {
		return nil, err
	}

	for _, n := range networks {
		if strings.TrimSpace(api.VirtualNetworkID(n)) == excludeNetworkID {
			continue
		}
		if n.Spec.Vni == nil {
			continue
		}
		vni := *n.Spec.Vni
		if vni < 0 || vni > maxVNI {
			continue
		}
		used[vni] = struct{}{}
	}

	return used, nil
}

func nextAvailableVNI(used map[int]struct{}) (int, error) {
	for candidate := minAutoVNI; candidate <= maxVNI; candidate++ {
		if _, exists := used[candidate]; exists {
			continue
		}
		return candidate, nil
	}

	return 0, fmt.Errorf("no available vni in range %d-%d", minAutoVNI, maxVNI)
}
