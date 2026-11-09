package clusteradm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

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
	var running []ServerRef
	for _, sv := range servers {
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
	if len(current) == 0 {
		return nil
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
	return nil
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"`)
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

	// etcd_url が既にローカルで、ローカル etcd が止まっており、退避した設定（*.bak）の etcd_url が
	// ローカルではない場合は、前回の離脱が手順4以降で中断された状態なので、残りの手順から続行する
	resume := false
	if isLocalURL(cfg.EtcdURL) && !env.UnitActive(EtcdUnit) {
		if prev, err := readNodeConfig(env.ConfigPath + ".bak"); err == nil && prev.EtcdURL != "" && !isLocalURL(prev.EtcdURL) {
			resume = true
		}
	}

	if resume {
		_, _ = fmt.Fprintln(out, "前回の離脱が途中で中断されています。ローカル構成の復元から続行します。")
		if opts.DryRun {
			_, _ = fmt.Fprintln(out, "--dry-run が指定されたため、何も変更しません。")
			return nil
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
			return fmt.Errorf("マスターノード（%s）は離脱できません。先に他のノードを離脱させてください", cfg.NodeName)
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

		// 2. VM の停止と削除
		if err := env.removeServers(ctx, store, cfg.NodeName, out); err != nil {
			return err
		}

		// 3. クラスタ上の自ノード情報の削除
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
	if resume {
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

	// 6. marmotd の起動
	if err := env.Systemctl("start", MarmotUnit); err != nil {
		return incomplete("手順6（marmot の起動）", err)
	}
	if err := env.waitUnitActive(ctx, MarmotUnit); err != nil {
		return incomplete("手順6（marmot の起動確認）", err)
	}
	_, _ = fmt.Fprintln(out, "クラスタから離脱しました。シングルノードとして起動しました。")
	return nil
}
