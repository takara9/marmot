package clusteradm

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
)

type JoinOptions struct {
	ClusterID         string
	MasterNode        string
	UnderlayInterface string
	DryRun            bool
	// nil でなければ、変更の実行前に呼び出し、false なら何も変更せず中止する
	Confirm func(summary string) bool
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func printNodes(out io.Writer, statuses []api.HostStatus) {
	sort.Slice(statuses, func(i, j int) bool {
		return derefString(statuses[i].NodeName) < derefString(statuses[j].NodeName)
	})
	_, _ = fmt.Fprintf(out, "%-16s %-10s %s\n", "NODE", "HOSTID", "IP")
	for _, st := range statuses {
		_, _ = fmt.Fprintf(out, "%-16s %-10s %s\n", derefString(st.NodeName), derefString(st.HostId), derefString(st.IpAddress))
	}
}

// Join は、シングルノードの marmot をクラスタのメンバーへ編入する。
func Join(ctx context.Context, env *Env, opts JoinOptions, out io.Writer) error {
	// 1. 事前チェック
	if !env.IsRoot() {
		return fmt.Errorf("root 権限で実行してください")
	}
	masterNode := strings.TrimSpace(opts.MasterNode)
	if masterNode == "" || strings.Contains(masterNode, ":") {
		return fmt.Errorf("--master-node には、ポート番号を含まない IP アドレスまたはホスト名を指定してください: %q", opts.MasterNode)
	}
	cfg, err := readNodeConfig(env.ConfigPath)
	if err != nil {
		return err
	}
	if cfg.NodeName == "" {
		return fmt.Errorf("%s に node_name が設定されていません", env.ConfigPath)
	}
	iface := opts.UnderlayInterface
	if iface == "" {
		iface = DefaultUnderlayInterface
	}
	if _, err := env.InterfaceIPv4(iface); err != nil {
		return err
	}
	hostID, err := env.HostID()
	if err != nil {
		return err
	}

	currentURL := cfg.EtcdURL
	if currentURL == "" {
		currentURL = etcdURL("127.0.0.1")
	}
	local, err := env.OpenStore(currentURL)
	if err != nil {
		return fmt.Errorf("etcd (%s) へ接続できません: %w", currentURL, err)
	}
	info, err := local.GetClusterInfo()
	if err == nil {
		_ = local.Close()
		return fmt.Errorf("このノードは既にクラスタ %s のメンバーです", info.ClusterId)
	} else if !isNotFound(err) {
		_ = local.Close()
		return fmt.Errorf("クラスタ情報を取得できません: %w", err)
	}
	objects, err := local.ListManagedObjects()
	_ = local.Close()
	if err != nil {
		return fmt.Errorf("ローカルの管理対象を確認できません: %w", err)
	}
	if len(objects) > 0 {
		shown := objects
		if len(shown) > 5 {
			shown = shown[:5]
		}
		return fmt.Errorf("ローカルの etcd に VM やネットワークなどの管理対象が %d 件あります（クラスタへ引き継がれません）。削除してから実行してください: %s",
			len(objects), strings.Join(shown, ", "))
	}

	// 2. マスターノードへの問い合わせ
	masterURL := etcdURL(masterNode)
	master, err := env.OpenStore(masterURL)
	if err != nil {
		return fmt.Errorf("マスターの etcd (%s) へ接続できません: %w", masterURL, err)
	}
	defer func() { _ = master.Close() }()
	masterInfo, err := master.GetClusterInfo()
	if isNotFound(err) {
		return fmt.Errorf("マスター (%s) でクラスタが有効になっていません。先に maadm cluster init を実行してください", masterNode)
	} else if err != nil {
		return fmt.Errorf("マスターのクラスタ情報を取得できません: %w", err)
	}
	if masterInfo.ClusterId != opts.ClusterID {
		return fmt.Errorf("CLUSTER-ID が一致しません（指定: %s, マスター: %s）", opts.ClusterID, masterInfo.ClusterId)
	}
	members, err := master.ListHostStatus()
	if err != nil {
		return fmt.Errorf("マスターのノード一覧を取得できません: %w", err)
	}
	for _, m := range members {
		if derefString(m.NodeName) == cfg.NodeName {
			return fmt.Errorf("node_name %q は、既にクラスタへ登録されています", cfg.NodeName)
		}
		if derefString(m.HostId) == hostID {
			return fmt.Errorf("HOSTID %s は、クラスタ内のノード %q と重複しています（/etc/hostid を変更してください）", hostID, derefString(m.NodeName))
		}
	}
	if joiningNodeWouldBecomeSchedulerLeader(cfg.NodeName, hostID, members, time.Now()) {
		return fmt.Errorf("HOSTID %s のノード %q では既存の OVN central 接続先を安全に維持できないため、クラスタへ参加できません。OVN DB とゲスト論理ポートを移行してから実行してください", hostID, cfg.NodeName)
	}

	summary := fmt.Sprintf(`以下の変更を行います。
  %s と %s を停止し、ローカルの %s を無効化（disable --now）
  %s: etcd_url=%s, default_underlay_interface=%s
  %s を起動し、マスターの etcd へノード %s が登録されることを確認
  このノード上の既存の管理対象は引き継がれません（0 件であることを確認済み）
`, MarmotUnit, EtcdUnit, EtcdUnit, env.ConfigPath, masterURL, iface, MarmotUnit, cfg.NodeName)
	_, _ = fmt.Fprint(out, summary)

	if opts.DryRun {
		_, _ = fmt.Fprintln(out, "--dry-run が指定されたため、何も変更しません。")
		return nil
	}

	if opts.Confirm != nil && !opts.Confirm(summary) {
		return fmt.Errorf("中止しました")
	}

	// 以降は失敗したら、元のシングルノード構成へ戻す
	marmotWasActive := env.UnitActive(MarmotUnit)
	marmotTouched, etcdDisabled := false, false
	var cfgBackup *fileBackup
	rollback := func(cause error) error {
		_, _ = fmt.Fprintf(out, "エラー: %v\n元のシングルノード構成へ戻します。\n", cause)
		report := func(err error) {
			if err != nil {
				_, _ = fmt.Fprintf(out, "ロールバック失敗: %v\n", err)
			}
		}
		if marmotTouched {
			report(env.Systemctl("stop", MarmotUnit))
			// 停止後に、マスターへ登録された自ノードの情報を削除する
			report(master.DeleteHostStatus(cfg.NodeName))
		}
		if cfgBackup != nil {
			report(cfgBackup.restore())
		}
		if etcdDisabled {
			if err := env.Systemctl("enable", "--now", EtcdUnit); err != nil {
				report(err)
			} else {
				report(env.waitEtcd(ctx, currentURL))
			}
		}
		if marmotTouched && marmotWasActive {
			report(env.Systemctl("restart", MarmotUnit))
		}
		return fmt.Errorf("cluster join に失敗しました（元の構成へ戻しました）: %w", cause)
	}

	// 3. ローカル構成の変更
	cfgBackup, err = backupFile(env.ConfigPath)
	if err != nil {
		return err
	}
	if marmotWasActive {
		marmotTouched = true
		if err := env.Systemctl("stop", MarmotUnit); err != nil {
			return rollback(err)
		}
	}
	etcdDisabled = true
	if err := env.Systemctl("disable", "--now", EtcdUnit); err != nil {
		return rollback(err)
	}
	data, err := os.ReadFile(env.ConfigPath)
	if err != nil {
		return rollback(err)
	}
	updated, err := updateJSONStrings(data, map[string]string{
		"etcd_url":                   masterURL,
		"default_underlay_interface": iface,
	})
	if err != nil {
		return rollback(fmt.Errorf("%s の更新に失敗: %w", env.ConfigPath, err))
	}
	if err := writeFileAtomic(env.ConfigPath, updated, cfgBackup.mode); err != nil {
		return rollback(err)
	}

	// 4. marmotd の起動
	marmotTouched = true
	if err := env.Systemctl("start", MarmotUnit); err != nil {
		return rollback(err)
	}
	if err := env.waitUnitActive(ctx, MarmotUnit); err != nil {
		return rollback(err)
	}

	// 5. 登録の確認
	var statuses []api.HostStatus
	err = env.waitFor(ctx, "ノード "+cfg.NodeName+" のクラスタへの登録", func() error {
		list, err := master.ListHostStatus()
		if err != nil {
			return err
		}
		for _, st := range list {
			if derefString(st.NodeName) == cfg.NodeName {
				statuses = list
				return nil
			}
		}
		return fmt.Errorf("node %s is not registered yet", cfg.NodeName)
	})
	if err != nil {
		return rollback(err)
	}

	_, _ = fmt.Fprintf(out, "クラスタに編入しました。\nCLUSTER-ID: %s\n", masterInfo.ClusterId)
	printNodes(out, statuses)
	return nil
}

func joiningNodeWouldBecomeSchedulerLeader(nodeName, hostID string, members []api.HostStatus, now time.Time) bool {
	joiningID, err := parseClusterHostID(hostID)
	if err != nil {
		return false
	}

	var leaderID uint64
	leaderName := ""
	for _, member := range members {
		if member.NodeName == nil || strings.TrimSpace(*member.NodeName) == "" ||
			member.HostId == nil || member.LastUpdated == nil ||
			!member.LastUpdated.After(now.Add(-30*time.Second)) {
			continue
		}
		memberID, err := parseClusterHostID(*member.HostId)
		if err != nil {
			continue
		}
		memberName := strings.TrimSpace(*member.NodeName)
		if leaderName == "" || memberID < leaderID || (memberID == leaderID && memberName < leaderName) {
			leaderID = memberID
			leaderName = memberName
		}
	}

	return leaderName == "" ||
		joiningID < leaderID || (joiningID == leaderID && strings.TrimSpace(nodeName) < leaderName)
}

func parseClusterHostID(hostID string) (uint64, error) {
	value := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(hostID)), "0x")
	id, err := strconv.ParseUint(value, 16, 32)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("invalid HOSTID %q", hostID)
	}
	return id, nil
}
