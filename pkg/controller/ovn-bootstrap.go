package controller

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/networkfabric"
)

var reconcileOVNClusterBootstrap = networkfabric.ConfigureOVNCluster

func reconcileOVNClusterBootstrapFromStatuses(localNode string, statuses []api.HostStatus, now time.Time, configure func(leaderIP, localIP string) error) error {
	if configure == nil {
		return nil
	}
	activeByNode := collectActiveHostStatusByNode(statuses, now)
	localStatus, ok := activeByNode[strings.TrimSpace(localNode)]
	if !ok {
		return fmt.Errorf("local node %s has no active host status", localNode)
	}
	localIP := hostStatusIPAddress(localStatus)
	if localIP == "" {
		return fmt.Errorf("local node %s has no host ip address", localNode)
	}
	leaderIP := selectOVNLeaderIP(activeByNode)
	if leaderIP == "" {
		return fmt.Errorf("no active OVN leader IP found")
	}
	return configure(leaderIP, localIP)
}

func selectOVNLeaderIP(activeByNode map[string]api.HostStatus) string {
	type candidate struct {
		nodeName  string
		hostID    uint32
		hasHostID bool
		ip        string
	}

	candidates := make([]candidate, 0, len(activeByNode))
	for node, status := range activeByNode {
		ip := hostStatusIPAddress(status)
		if ip == "" {
			continue
		}
		c := candidate{nodeName: strings.TrimSpace(node), ip: ip}
		if status.HostId != nil {
			if parsed, ok := parseHexHostID(*status.HostId); ok {
				c.hostID = parsed
				c.hasHostID = true
			}
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].hasHostID != candidates[j].hasHostID {
			return candidates[i].hasHostID
		}
		if candidates[i].hasHostID && candidates[i].hostID != candidates[j].hostID {
			return candidates[i].hostID < candidates[j].hostID
		}
		return candidates[i].nodeName < candidates[j].nodeName
	})
	return candidates[0].ip
}

func hostStatusIPAddress(status api.HostStatus) string {
	if status.IpAddress == nil {
		return ""
	}
	return strings.TrimSpace(*status.IpAddress)
}
