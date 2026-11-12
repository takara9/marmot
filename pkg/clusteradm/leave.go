package clusteradm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/takara9/marmot/pkg/db"
)

type LeaveOptions struct {
	ClusterID string
	DryRun    bool
	// nil でなければ、変更の実行前に呼び出し、false なら何も変更せず中止する
	Confirm func(summary string) bool
}

func isLocalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

func serverNames(servers []ServerRef) string {
	names := make([]string, 0, len(servers))
	for _, sv := range servers {
		names = append(names, fmt.Sprintf("%s(%s)", sv.Name, sv.ID))
	}
	return strings.Join(names, ", ")
}

// removeServers は、ノード上の VM をシャットダウンして削除する。
// シャットダウンが ShutdownTimeout で終わらない VM は、削除処理が強制的に停止する。
func (e *Env) removeServers(ctx context.Context, store ClusterStore, nodeName string, out io.Writer) error {
	servers, err := store.ListServersOnNode(nodeName)
	if err != nil {
		return err
	}
	// 前回の実行で、ボリュームの削除待ちが中断された場合に備え、記録済みの ID を引き継ぐ
	volumesPath := e.ConfigPath + ".leave-volumes"
	volumeIDs, err := readVolumeIDs(volumesPath)
	if err != nil {
		return err
	}
	var running []ServerRef
	for _, sv := range servers {
		volumeIDs = append(volumeIDs, sv.VolumeIDs...)
		if sv.StatusCode == db.SERVER_RUNNING {
			running = append(running, sv)
		}
	}

	if len(running) > 0 {
		_, _ = fmt.Fprintf(out, "VM %d 台をシャットダウンします（最大 %s）。\n", len(running), e.ShutdownTimeout)
		for _, sv := range running {
			if err := store.StopServer(sv.ID); err != nil {
				return fmt.Errorf("VM %s のシャットダウン要求に失敗: %w", sv.Name, err)
			}
		}
		err := e.waitForTimeout(ctx, e.ShutdownTimeout, "VM のシャットダウン", func() error {
			current, err := store.ListServersOnNode(nodeName)
			if err != nil {
				return err
			}
			for _, sv := range current {
				for _, r := range running {
					if sv.ID == r.ID && sv.StatusCode != db.SERVER_STOPPED && sv.StatusCode != db.SERVER_DELETING && sv.StatusCode != db.SERVER_ERROR {
						return fmt.Errorf("VM %s is still %s", sv.Name, db.ServerStatus[sv.StatusCode])
					}
				}
			}
			return nil
		})
		if err != nil {
			_, _ = fmt.Fprintf(out, "シャットダウンが完了しない VM があります。強制停止して削除します。(%v)\n", err)
		}
	}

	current, err := store.ListServersOnNode(nodeName)
	if err != nil {
		return err
	}
	if len(current) > 0 {
		for _, sv := range current {
			volumeIDs = append(volumeIDs, sv.VolumeIDs...)
		}
		// Server の削除後に再実行しても、ボリュームの削除完了を確認できるよう、削除の前に記録する
		if err := writeVolumeIDs(volumesPath, volumeIDs); err != nil {
			return fmt.Errorf("削除対象のボリュームを記録できません。クラスタのメンバーのままです: %w", err)
		}
		_, _ = fmt.Fprintf(out, "VM %d 台を削除します。\n", len(current))
		for _, sv := range current {
			if sv.StatusCode == db.SERVER_DELETING {
				continue
			}
			if err := store.DeleteServer(sv.ID); err != nil {
				return fmt.Errorf("VM %s の削除要求に失敗: %w", sv.Name, err)
			}
		}
		err = e.waitForTimeout(ctx, e.DeleteTimeout, "VM の削除", func() error {
			remaining, err := store.ListServersOnNode(nodeName)
			if err != nil {
				return err
			}
			if len(remaining) > 0 {
				return fmt.Errorf("%d 台が残っています: %s", len(remaining), serverNames(remaining))
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("VM の削除に失敗しました。クラスタのメンバーのままです。再実行すると、残りの VM から続行します: %w", err)
		}
	}
	err = e.waitForTimeout(ctx, e.DeleteTimeout, "ボリュームの削除", func() error {
		remaining, err := store.ExistingVolumeIDs(volumeIDs)
		if err != nil {
			return err
		}
		if len(remaining) > 0 {
			return fmt.Errorf("%d 個が残っています: %s", len(remaining), strings.Join(remaining, ", "))
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("ボリュームの削除に失敗しました。クラスタのメンバーのままです。残りのボリュームの削除が完了してから、再実行してください: %w", err)
	}
	if err := os.Remove(volumesPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("削除対象のボリューム記録 %s を削除できません: %w", volumesPath, err)
	}
	return nil
}

// readVolumeIDs は、前回の実行で記録した、削除待ちのボリューム ID を返す。記録が無ければ空を返す。
func readVolumeIDs(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("削除対象のボリューム記録 %s を読み込めません: %w", path, err)
	}
	var ids []string
	for _, line := range strings.Split(string(data), "\n") {
		if id := strings.TrimSpace(line); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func writeVolumeIDs(path string, ids []string) error {
	seen := map[string]bool{}
	var uniq []string
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	if len(uniq) == 0 {
		return nil
	}
	return writeFileAtomic(path, []byte(strings.Join(uniq, "\n")+"\n"), 0o600)
}

// waitSettled は、配置禁止の前に割り当てられた VM が、残っていないことを確認する。
func (e *Env) waitSettled(ctx context.Context, store ClusterStore, nodeName string, out io.Writer) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(e.PollInterval):
	}
	remaining, err := store.ListServersOnNode(nodeName)
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		return fmt.Errorf("配置禁止の前に割り当てられた VM が残っています。クラスタのメンバーのままです。再実行すると、残りの VM から続行します: %s", serverNames(remaining))
	}
	return nil
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"`)
}

// handOverManagementHead は、離脱ノードが mgmt ネットワークの head なら、残りのノードへ引き継ぐ。
func handOverManagementHead(store ClusterStore, nodeName string) error {
	hosts, err := store.ListHostStatus()
	if err != nil {
		return fmt.Errorf("mgmt ネットワークの head を引き継ぐノードを調べるためのノード一覧を取得できません: %w", err)
	}
	var remaining []string
	for _, h := range hosts {
		if name := derefString(h.NodeName); name != "" && name != nodeName {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	if err := store.HandOverManagementHead(nodeName, remaining); err != nil {
		return fmt.Errorf("mgmt ネットワークの head を残りのノードへ引き継げませんでした。他のノードの follower ネットワークが作成されるのを待って、再実行してください: %w", err)
	}
	return nil
}

// removeMgmtHostPort は、head だったノードの mgmt-host ポートを撤去し、撤去を確認する。
// 残ると、引き継いだ新 head が同じ IP／iface-id のポートを作成した際に競合する。
func (e *Env) removeMgmtHostPort() error {
	if _, err := e.Run("ovs-vsctl", "--if-exists", "del-port", "mgmt-host"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("mgmt-host ポートを削除できません（手動: ovs-vsctl --if-exists del-port mgmt-host）: %w", err)
	}
	out, err := e.Run("ovs-vsctl", "--if-exists", "get", "port", "mgmt-host", "_uuid")
	if err != nil {
		return fmt.Errorf("mgmt-host ポートの撤去を確認できません: %w", err)
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("mgmt-host ポートが残っています（手動: ovs-vsctl --if-exists del-port mgmt-host）")
	}
	return nil
}

// cleanupOVN は、OVS/OVN の設定から、クラスタの情報を取り除く。
// 失敗しても離脱の完了を妨げないため、警告を表示して続行する。
func (e *Env) cleanupOVN(out io.Writer) {
	warn := func(format string, args ...any) {
		_, _ = fmt.Fprintf(out, "警告: "+format+"\n", args...)
	}

	remote, err := e.Run("ovs-vsctl", "get", "open", ".", "external_ids:ovn-remote")
	if errors.Is(err, exec.ErrNotFound) {
		_, _ = fmt.Fprintln(out, "ovs-vsctl が無いため、OVN のクリーンアップを省略します。")
		return
	}
	remote = unquote(remote)
	if err != nil {
		remote = "" // 未設定
	}
	systemID, err := e.Run("ovs-vsctl", "get", "open", ".", "external_ids:system-id")
	if err != nil {
		systemID = ""
	}
	systemID = unquote(systemID)

	// 先に ovn-remote を外して ovn-controller を切り離す。繋がったまま chassis を消すと再登録される
	if remote != "" {
		if _, err := e.Run("ovs-vsctl", "remove", "open", ".", "external_ids", "ovn-remote"); err != nil {
			warn("ovn-remote を削除できませんでした: %v（手動: ovs-vsctl remove open . external_ids ovn-remote）", err)
		}
	}
	if remote == "" || systemID == "" {
		return
	}
	if _, err := e.Run("ovn-sbctl", "--db="+remote, "chassis-del", systemID); err != nil {
		warn("OVN SB DB から chassis を削除できませんでした: %v（手動: ovn-sbctl --db=%s chassis-del %s）", err, remote, systemID)
	}
}

// removeLocalNetworks は、離脱前に記録した follower ネットワークの libvirt 定義を、ローカルから除去する。
// 残すと単独構成の起動時にローカル etcd へ再取り込みされ、次の join が拒否される。
// 共有 OVN 論理スイッチは、他ノードが使うため削除しない。
func (e *Env) removeLocalNetworks(names []string, out io.Writer) {
	for _, name := range names {
		if _, err := e.Run("virsh", "net-info", name); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				_, _ = fmt.Fprintln(out, "virsh が無いため、libvirt ネットワークのクリーンアップを省略します。")
				return
			}
			continue // 定義が無い
		}
		// 非アクティブの場合は失敗するため、無視して undefine へ進む
		_, _ = e.Run("virsh", "net-destroy", name)
		if _, err := e.Run("virsh", "net-undefine", name); err != nil {
			_, _ = fmt.Fprintf(out, "警告: libvirt ネットワーク %s を削除できませんでした: %v（手動: virsh net-destroy %s; virsh net-undefine %s）\n", name, err, name, name)
		}
	}
}

// parseLeaveState は、離脱の状態ファイルから、クラスタ ID と follower ネットワーク名を取り出す。
func parseLeaveState(data string) (string, []string) {
	lines := strings.Split(strings.TrimSpace(data), "\n")
	var names []string
	for _, l := range lines[1:] {
		if l = strings.TrimSpace(l); l != "" {
			names = append(names, l)
		}
	}
	return strings.TrimSpace(lines[0]), names
}

// Leave は、クラスタのメンバーであるノードを、元のシングルノード構成へ戻す。
func Leave(ctx context.Context, env *Env, opts LeaveOptions, out io.Writer) error {
	// 1. 事前チェック
	if !env.IsRoot() {
		return fmt.Errorf("root 権限で実行してください")
	}
	cfg, err := readNodeConfig(env.ConfigPath)
	if err != nil {
		return err
	}
	if cfg.NodeName == "" {
		return fmt.Errorf("%s に node_name が設定されていません", env.ConfigPath)
	}
	localURL := etcdURL("127.0.0.1")

	// HostStatus の削除前に、クラスタ ID を記録する。記録が残っている場合は、前回の離脱が
	// 手順3以降で中断された状態なので、サービスの稼働状態に依存せず、残りの手順から続行する
	statePath := env.ConfigPath + ".leave"
	resume := false
	var localNetworks []string
	if data, err := os.ReadFile(statePath); err == nil {
		recorded, names := parseLeaveState(string(data))
		localNetworks = names
		if recorded != opts.ClusterID {
			return fmt.Errorf("CLUSTER-ID が一致しません（指定: %s, 中断中の離脱のクラスタ: %s）", opts.ClusterID, recorded)
		}
		resume = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("離脱の状態ファイル %s を読み込めません: %w", statePath, err)
	}

	if resume {
		_, _ = fmt.Fprintln(out, "前回の離脱が途中で中断されています。ローカル構成の復元から続行します。")
		if opts.DryRun {
			_, _ = fmt.Fprintln(out, "--dry-run が指定されたため、何も変更しません。")
			return nil
		}
		// 設定の変更前に中断された場合は、クラスタ上のノード情報を削除し直す（削除済みでも成功する）
		if !isLocalURL(cfg.EtcdURL) {
			store, err := env.OpenStore(cfg.EtcdURL)
			if err != nil {
				return fmt.Errorf("etcd (%s) へ接続できません。クラスタの情報を削除できないため、離脱しません: %w", cfg.EtcdURL, err)
			}
			defer func() { _ = store.Close() }()
			if err := env.Systemctl("stop", MarmotUnit); err != nil {
				return fmt.Errorf("ノード情報を削除する前の marmot 停止に失敗しました: %w", err)
			}
			if err := env.removeMgmtHostPort(); err != nil {
				return fmt.Errorf("mgmt-host ポートを撤去できないため、head を引き継ぎません: %w", err)
			}
			if err := handOverManagementHead(store, cfg.NodeName); err != nil {
				return err
			}
			if err := store.DeleteFollowerNetworks(cfg.NodeName); err != nil {
				return fmt.Errorf("クラスタ上のノードの follower ネットワークの削除に失敗しました: %w", err)
			}
			if err := store.DeleteHostStatus(cfg.NodeName); err != nil {
				return fmt.Errorf("クラスタ上のノード情報の削除に失敗しました: %w", err)
			}
		}
	} else {
		currentURL := cfg.EtcdURL
		if currentURL == "" {
			currentURL = localURL
		}
		store, err := env.OpenStore(currentURL)
		if err != nil {
			return fmt.Errorf("etcd (%s) へ接続できません。クラスタの情報を削除できないため、離脱しません: %w", currentURL, err)
		}
		defer func() { _ = store.Close() }()

		info, err := store.GetClusterInfo()
		if isNotFound(err) {
			return fmt.Errorf("このノードは、クラスタのメンバーではありません")
		} else if err != nil {
			return fmt.Errorf("クラスタ情報を取得できません: %w", err)
		}
		if info.ClusterId != opts.ClusterID {
			return fmt.Errorf("CLUSTER-ID が一致しません（指定: %s, このノードのクラスタ: %s）", opts.ClusterID, info.ClusterId)
		}
		if info.MasterNode == cfg.NodeName {
			return fmt.Errorf("ノード %s は etcd を稼働させているマスターノードのため、離脱できません（mactl cluster の LEADER 列とは別の概念です）。先に他のノードを離脱させてください", cfg.NodeName)
		}
		members, err := store.ListHostStatus()
		if err != nil {
			return fmt.Errorf("クラスタのノード一覧を取得できません: %w", err)
		}
		if leaderName, _ := schedulerLeader(members, time.Now()); leaderName == strings.TrimSpace(cfg.NodeName) {
			return fmt.Errorf("ノード %s は OVN central の接続先（最小の HOSTID）のため、離脱できません。OVN DB とゲスト論理ポートを移行してから実行してください", cfg.NodeName)
		}

		servers, err := store.ListServersOnNode(cfg.NodeName)
		if err != nil {
			return fmt.Errorf("このノードの VM 一覧を取得できません: %w", err)
		}
		if len(servers) > 0 && !env.UnitActive(MarmotUnit) {
			return fmt.Errorf("VM の削除には %s の稼働が必要です。起動してから再実行してください", MarmotUnit)
		}

		summary := "以下の変更を行います。\n"
		if len(servers) > 0 {
			summary += fmt.Sprintf("  削除される VM（%d 台、復元できません）: %s\n", len(servers), serverNames(servers))
		} else {
			summary += "  削除される VM: なし\n"
		}
		summary += fmt.Sprintf(`  クラスタ（%s）から、ノード %s の情報を削除
  %s を停止し、%s: etcd_url=%s へ戻し、ローカルの %s を有効化して起動
  OVN: ovn-remote を削除し、SB DB から chassis を削除
  %s を起動（シングルノード）
`, info.ClusterId, cfg.NodeName, MarmotUnit, env.ConfigPath, localURL, EtcdUnit, MarmotUnit)
		_, _ = fmt.Fprint(out, summary)

		if opts.DryRun {
			_, _ = fmt.Fprintln(out, "--dry-run が指定されたため、何も変更しません。")
			return nil
		}
		if opts.Confirm != nil && !opts.Confirm(summary) {
			return fmt.Errorf("中止しました")
		}

		// 2. 配置禁止にしてから、VM の停止と削除
		if err := store.CordonNode(cfg.NodeName); err != nil {
			return fmt.Errorf("ノードを配置禁止にできません。クラスタのメンバーのままです: %w", err)
		}
		if err := env.removeServers(ctx, store, cfg.NodeName, out); err != nil {
			return err
		}
		// 配置禁止の前に割り当てられた VM が残っていないことを、スケジューラーの1周期後に再確認する
		if err := env.waitSettled(ctx, store, cfg.NodeName, out); err != nil {
			return err
		}

		// ホスト情報の定期更新を止めてから、クラスタ上の自ノード情報を削除する。
		localNetworks, err = store.ListFollowerNetworkNames(cfg.NodeName)
		if err != nil {
			return fmt.Errorf("ノードの follower ネットワークを取得できません。クラスタのメンバーのままです: %w", err)
		}
		state := info.ClusterId + "\n" + strings.Join(localNetworks, "\n")
		if err := writeFileAtomic(statePath, []byte(strings.TrimSpace(state)+"\n"), 0o600); err != nil {
			return fmt.Errorf("離脱の状態ファイルを書き込めません。クラスタのメンバーのままです: %w", err)
		}
		if err := env.Systemctl("stop", MarmotUnit); err != nil {
			return fmt.Errorf("ノード情報を削除する前の marmot 停止に失敗しました: %w", err)
		}
		if err := env.removeMgmtHostPort(); err != nil {
			return fmt.Errorf("mgmt-host ポートを撤去できないため、head を引き継ぎません: %w", err)
		}
		if err := handOverManagementHead(store, cfg.NodeName); err != nil {
			return err
		}
		if err := store.DeleteFollowerNetworks(cfg.NodeName); err != nil {
			return fmt.Errorf("クラスタ上のノードの follower ネットワークの削除に失敗しました。VM は削除済みです。再実行すると、この手順から続行します: %w", err)
		}
		if err := store.DeleteHostStatus(cfg.NodeName); err != nil {
			return fmt.Errorf("クラスタ上のノード情報の削除に失敗しました。VM は削除済みです。再実行すると、この手順から続行します: %w", err)
		}
	}

	// 手順3まで完了すると、ロールバックはしない。失敗したら、再実行で続行する
	incomplete := func(step string, err error) error {
		return fmt.Errorf("%s に失敗しました。VM は削除済みです。原因を解消してから、再実行してください: %w", step, err)
	}

	// 4. ローカル構成の復元
	if err := env.Systemctl("stop", MarmotUnit); err != nil {
		return incomplete("手順4（marmot の停止）", err)
	}
	// 再実行時は、最初の実行で退避した *.bak を上書きしない
	cfgMode := os.FileMode(0o644)
	if resume && isLocalURL(cfg.EtcdURL) {
		if fi, err := os.Stat(env.ConfigPath); err == nil {
			cfgMode = fi.Mode().Perm()
		}
	} else {
		cfgBackup, err := backupFile(env.ConfigPath)
		if err != nil {
			return incomplete("手順4（設定のバックアップ）", err)
		}
		cfgMode = cfgBackup.mode
	}
	data, err := os.ReadFile(env.ConfigPath)
	if err != nil {
		return incomplete("手順4（設定の読み込み）", err)
	}
	updated, err := updateJSONStrings(data, map[string]string{"etcd_url": localURL})
	if err != nil {
		return incomplete("手順4（設定の更新）", err)
	}
	if err := writeFileAtomic(env.ConfigPath, updated, cfgMode); err != nil {
		return incomplete("手順4（設定の書き込み）", err)
	}
	if err := env.Systemctl("enable", "--now", EtcdUnit); err != nil {
		return incomplete("手順4（ローカル etcd の起動）", err)
	}
	if err := env.waitEtcd(ctx, localURL); err != nil {
		return incomplete("手順4（ローカル etcd の起動確認）", err)
	}

	// 5. OVS/OVN のクリーンアップ
	env.cleanupOVN(out)
	env.removeLocalNetworks(localNetworks, out)

	// 6. marmotd の起動
	if err := env.Systemctl("start", MarmotUnit); err != nil {
		return incomplete("手順6（marmot の起動）", err)
	}
	if err := env.waitUnitActive(ctx, MarmotUnit); err != nil {
		return incomplete("手順6（marmot の起動確認）", err)
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return incomplete("離脱の状態ファイルの削除", err)
	}
	_, _ = fmt.Fprintln(out, "クラスタから離脱しました。シングルノードとして起動しました。")
	return nil
}
