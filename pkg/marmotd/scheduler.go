package marmotd

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
)

const (
	// アクティブとみなすHostStatusの最終更新からの許容時間
	// HostControllerが10秒間隔で更新するため、3サイクル分を許容する
	ActiveHostThreshold = 30 * time.Second
)

var ErrNoActiveHosts = errors.New("scheduler: no active hosts available")

// filterActiveHosts は LastUpdated が ActiveHostThreshold 以内のホストのみを返す
func filterActiveHosts(statuses []api.HostStatus) []api.HostStatus {
	cutoff := time.Now().Add(-ActiveHostThreshold)
	var active []api.HostStatus
	for _, s := range statuses {
		if s.LastUpdated != nil && s.LastUpdated.After(cutoff) {
			active = append(active, s)
		}
	}
	return active
}

// IsSchedulerLeader は nodeName がアクティブなホスト群の中でリーダーか判定する。
// リーダー = hostId の値が最小のノード。同値の場合は NodeName 昇順。
func IsSchedulerLeader(nodeName string, statuses []api.HostStatus) bool {
	active := filterActiveHosts(statuses)
	if len(active) == 0 {
		return false
	}
	type leaderCandidate struct {
		nodeName string
		hostID   uint32
	}
	candidates := make([]leaderCandidate, 0, len(active))
	for _, s := range active {
		if s.NodeName == nil || strings.TrimSpace(*s.NodeName) == "" || s.HostId == nil {
			continue
		}
		hostID, ok := parseHostIDHex(*s.HostId)
		if !ok {
			continue
		}
		candidates = append(candidates, leaderCandidate{nodeName: *s.NodeName, hostID: hostID})
	}
	if len(candidates) == 0 {
		return false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].hostID != candidates[j].hostID {
			return candidates[i].hostID < candidates[j].hostID
		}
		return candidates[i].nodeName < candidates[j].nodeName
	})
	return candidates[0].nodeName == nodeName
}

func parseHostIDHex(v string) (uint32, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	v = strings.TrimPrefix(strings.ToLower(v), "0x")
	id, err := strconv.ParseUint(v, 16, 32)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint32(id), true
}

// SelectNode はアクティブなホスト群からスコアが最良のノード名を返す。
// スコアは RunningVMs の数（少ないほど優先）。同点の場合は NodeName 昇順。
func SelectNode(statuses []api.HostStatus) (string, error) {
	active := filterActiveHosts(statuses)
	if len(active) == 0 {
		return "", ErrNoActiveHosts
	}

	// NodeName が設定されているホストのみ対象
	var candidates []api.HostStatus
	for _, s := range active {
		if s.NodeName != nil && *s.NodeName != "" {
			candidates = append(candidates, s)
		}
	}
	if len(candidates) == 0 {
		return "", ErrNoActiveHosts
	}

	sort.Slice(candidates, func(i, j int) bool {
		runningI := runningVMs(candidates[i])
		runningJ := runningVMs(candidates[j])
		if runningI != runningJ {
			return runningI < runningJ
		}
		// 同点の場合はノード名昇順（決定的な選択）
		return *candidates[i].NodeName < *candidates[j].NodeName
	})

	return *candidates[0].NodeName, nil
}

// runningVMs は HostAllocation.RunningVMs の値を返す。未設定の場合は 0。
func runningVMs(s api.HostStatus) int {
	if s.Allocation != nil && s.Allocation.RunningVMs != nil {
		return *s.Allocation.RunningVMs
	}
	return 0
}
