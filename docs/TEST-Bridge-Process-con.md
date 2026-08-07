# OVSブリッジ、Linux ブリッジとプロセスとの疎通テスト

プライベートの仮想ネットワークに繋がった仮想サーバーと、marmotホスト上のプロセスとの間で、疎通を取りたい。

## やりたい事
- marmotホスト上のプロセスからプライベートの仮想ネットワーク上の仮想サーバーにsshコマンドを実行する
- 仮想ネットワーク上の仮想サーバーから、marmotホスト上のプロセスのサーバーポートへアクセスする。


## 進め方

1. 専用のネットワーク空間（NetNS）を作成する
    ホストOS全体とは独立したネットワークインターフェースやルーティングテーブルを持つ空間を作ります。

2. 仮想LANケーブル（veth ペア）を作成する
    片端を「ホスト側のブリッジネットワーク」、もう片端を「作成したNetNS」に差し込みます。

3. NetNS内で特定プロセスを起動する
    そのNetNSを指定してコマンドを実行することで、特定のプロセスだけがそのブリッジを経由して通信するようになります。



以下の172.16.90.2 へ、コマンドラインから接続する

ubuntu@ws1:~$ mactl get srv db
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE
----             ----          ------        ---  -------  ----------       -------          ---
db               hv1           RUNNING       4    4096     10.1.1.10        host-bridge      6d
                                                           172.16.90.2      db-net       


## 手順

```console
sudo ip netns add ns-app
sudo ip link add veth-ovs type veth peer name veth-ns
sudo ovs-vsctl add-port br-ae9f7 veth-ovs
sudo ip link set veth-ovs up
sudo ip link set veth-ns netns ns-app
sudo ip netns exec ns-app ip link set veth-ns name eth0
sudo ip netns exec ns-app ip addr add 172.16.90.100/24 dev eth0
sudo ip netns exec ns-app ip link set eth0 up
sudo ip netns exec ns-app ip link set lo up
sudo ip netns exec ns-app ping 172.16.90.2
sudo ip netns exec ns-app ssh ubuntu@172.16.90.2
```


## コマンドの意味

```bash
sudo ip link add veth-ovs type veth peer name veth-ns
```

- `ip link add` : 新しいネットワークインターフェース（リンク）を作成するコマンドです。
- `veth-ovs` : 作成するインターフェースの名前（このコマンドを実行した直後の名前）です。
- `type veth` : 作成するリンクの種類を **veth（Virtual Ethernet）** に指定します。veth は必ず2つのインターフェースが対になって生成される仮想LANケーブルのようなデバイスです。片方に入れたパケットは、必ずもう片方から出てきます。
- `peer name veth-ns` : veth のもう片方の名前を `veth-ns` に指定します。

つまりこの1行で、**`veth-ovs` ⟷ `veth-ns`** という名前の仮想ケーブルの両端を同時に作成しています。

## このドキュメントでの役割

- `veth-ovs` 側 → ホスト側に残し、TEST-Bridge-Process-con.mdの`sudo ovs-vsctl add-port br-ae9f7 veth-ovs`でOVSブリッジ`br-ae9f7`のポートとして接続します。
- `veth-ns` 側 → 後続の`sudo ip link set veth-ns netns ns-app`で、専用ネットワーク名前空間`ns-app`の中に移動され、`eth0`にリネームされます。

結果として、「OVSブリッジ（`br-ae9f7`、＝VM `db` が繋がる `db-net`）」と「`ns-app`という隔離されたプロセス用ネットワーク空間」が、この veth ペアを通じて1本の仮想LANケーブルで直結される、という構成になります。


```bash
sudo ip link set veth-ns netns ns-app
```

- `ip link set <デバイス名>` : 既存のネットワークインターフェースの設定を変更するコマンドです。
- `veth-ns` : 対象のインターフェース名。1つ前のTEST-Bridge-Process-con.mdの`ip link add veth-ovs type veth peer name veth-ns`で作られたveth-ovsの対向側です。
- `netns ns-app` : このインターフェースの**所属先を、現在のネットワーク名前空間（ホスト側のデフォルトnetns）から、`ns-app`という名前空間へ移動する**、という指定です。

## やっていること

Linuxのネットワークインターフェースは、通常「1つのネットワーク名前空間」にしか所属できません。このコマンドは、ホスト側に作られた`veth-ns`を、TEST-Bridge-Process-con.mdの`ip netns add ns-app`で作成済みの`ns-app`という隔離空間へ**引っ越しさせる**操作です。

- 実行前: `veth-ns`はホストの`ip link`一覧に見える（ホストのデフォルトnetnsに所属）
- 実行後: `veth-ns`はホスト側からは見えなくなり、`sudo ip netns exec ns-app ip link`でのみ見えるようになる

このあとのTEST-Bridge-Process-con.mdの`ip netns exec ns-app ip link set veth-ns name eth0`で、`ns-app`内に移動した`veth-ns`を`eth0`という名前に付け替えています。

結果として、`veth-ovs`（OVSブリッジ側に残る）↔ `veth-ns`＝`eth0`（`ns-app`内）という1本の仮想ケーブルの両端が、それぞれ別々のネットワーク空間に配置される形になります。



## 接続結果
`sudo ip netns exec ns-app ping 172.16.90.2` で ネットワークネームスペース ns-app から ping すると疎通がある 

```console
ubuntu@hv1:~$ sudo ip netns exec ns-app ping 172.16.90.2
PING 172.16.90.2 (172.16.90.2) 56(84) bytes of data.
64 bytes from 172.16.90.2: icmp_seq=1 ttl=64 time=0.414 ms
64 bytes from 172.16.90.2: icmp_seq=2 ttl=64 time=0.091 ms
^C
 --- 172.16.90.2 ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 1032ms
rtt min/avg/max/mdev = 0.091/0.252/0.414/0.161 ms
```

ただの `ping 172.16.90.2` では疎通が無い。

```console
ubuntu@hv1:~$ ping 172.16.90.2
PING 172.16.90.2 (172.16.90.2) 56(84) bytes of data.
From 10.1.0.1 icmp_seq=1 Destination Host Unreachable
From 10.1.0.1 icmp_seq=2 Destination Host Unreachable
From 10.1.0.1 icmp_seq=3 Destination Host Unreachable
```

## ネームスペース nsapp の設定状態

```console
ubuntu@hv1:~$ sudo ip netns exec ns-app ip r
172.16.90.0/24 dev eth0 proto kernel scope link src 172.16.90.100 
ubuntu@hv1:~$ sudo ip netns exec ns-app ip l
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN mode DEFAULT group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
32: eth0@if33: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP mode DEFAULT group default qlen 1000
    link/ether 4a:5f:19:24:19:aa brd ff:ff:ff:ff:ff:ff link-netnsid 0
ubuntu@hv1:~$ sudo ip netns exec ns-app ip a
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet6 ::1/128 scope host 
       valid_lft forever preferred_lft forever
32: eth0@if33: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP group default qlen 1000
    link/ether 4a:5f:19:24:19:aa brd ff:ff:ff:ff:ff:ff link-netnsid 0
    inet 172.16.90.100/24 scope global eth0
       valid_lft forever preferred_lft forever
    inet6 fe80::485f:19ff:fe24:19aa/64 scope link 
       valid_lft forever preferred_lft forever
```


## クリーンアップ手順

```bash
# 1. OVSブリッジからポート定義を明示的に削除（先にやることで、以前起きたような
#    "veth-ovs no such device" のスタレージエントリ化を防ぐ）
sudo ovs-vsctl del-port br-ae9f7 veth-ovs

# 2. ネットワーク名前空間を削除（内部の veth-ns/eth0 が消え、veth pairなのでホスト側の
#    veth-ovs も自動的に一緒に削除される）
sudo ip netns delete ns-app
```

## 確認コマンド

```bash
# netns が消えていること
ip netns list

# veth-ovs / veth-ns がホストから消えていること
ip link show veth-ovs
ip link show veth-ns

# OVSブリッジに veth-ovs が残っていないこと
sudo ovs-vsctl show
```

いずれも「No such device」やエントリなしになっていれば、クリーンアップ完了です。


## Go言語での実装

選択部分（netns 作成→veth 作成→OVS 接続→アドレス設定→疎通確認）をGoで実装する例です。本リポジトリでは `pkg/util/hostbridge.go` や `pkg/networkfabric/ovs.go` と同様、`netlink` などのライブラリは使わず `exec.Command` で `ip` / `ovs-vsctl` を呼び出す方式に統一しています。

```go
package netnsbridge

import (
	"context"
	"fmt"
	"os/exec"
)

// Config は netns ⟷ OVS ブリッジ接続に必要なパラメータです。
type Config struct {
	NetNS   string // 例: "ns-app"
	VethOVS string // 例: "veth-ovs" (ホスト/OVS側)
	VethNS  string // 例: "veth-ns"  (netns側、後でeth0にrename)
	Bridge  string // 例: "br-ae9f7"
	IfName  string // netns内でのIF名。例: "eth0"
	CIDR    string // 例: "172.16.90.100/24"
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w, output: %s", name, args, err, string(out))
	}
	return nil
}

// Setup は選択部分のシェル手順をGoで再現します。
func Setup(ctx context.Context, c Config) error {
	if err := run(ctx, "ip", "netns", "add", c.NetNS); err != nil {
		return err
	}

	if err := run(ctx, "ip", "link", "add", c.VethOVS, "type", "veth", "peer", "name", c.VethNS); err != nil {
		_ = run(ctx, "ip", "netns", "delete", c.NetNS)
		return err
	}

	if err := run(ctx, "ovs-vsctl", "add-port", c.Bridge, c.VethOVS); err != nil {
		return errCleanup(ctx, c, err)
	}

	if err := run(ctx, "ip", "link", "set", c.VethOVS, "up"); err != nil {
		return errCleanup(ctx, c, err)
	}

	if err := run(ctx, "ip", "link", "set", c.VethNS, "netns", c.NetNS); err != nil {
		return errCleanup(ctx, c, err)
	}

	nsExec := func(args ...string) error {
		full := append([]string{"netns", "exec", c.NetNS}, args...)
		return run(ctx, "ip", full...)
	}

	if err := nsExec("ip", "link", "set", c.VethNS, "name", c.IfName); err != nil {
		return errCleanup(ctx, c, err)
	}
	if err := nsExec("ip", "addr", "add", c.CIDR, "dev", c.IfName); err != nil {
		return errCleanup(ctx, c, err)
	}
	if err := nsExec("ip", "link", "set", c.IfName, "up"); err != nil {
		return errCleanup(ctx, c, err)
	}
	if err := nsExec("ip", "link", "set", "lo", "up"); err != nil {
		return errCleanup(ctx, c, err)
	}

	return nil
}

// errCleanup は失敗時に作成済みリソースを可能な範囲で削除してからエラーを返します。
func errCleanup(ctx context.Context, c Config, cause error) error {
	_ = run(ctx, "ovs-vsctl", "del-port", c.Bridge, c.VethOVS)
	_ = run(ctx, "ip", "netns", "delete", c.NetNS)
	return cause
}

// Teardown はクリーンアップ手順（del-port → netns delete）を実行します。
func Teardown(ctx context.Context, c Config) error {
	if err := run(ctx, "ovs-vsctl", "del-port", c.Bridge, c.VethOVS); err != nil {
		return err
	}
	return run(ctx, "ip", "netns", "delete", c.NetNS)
}

// PingFromNS は netns内から指定IPへpingして疎通確認します（`-c count`回）。
func PingFromNS(ctx context.Context, netns, target string, count int) error {
	return run(ctx, "ip", "netns", "exec", netns, "ping", "-c", fmt.Sprintf("%d", count), target)
}

// SSHFromNS は netns内からsshコマンドを実行するための *exec.Cmd を返します
// （対話的な認証が必要なため、呼び出し側で Stdin/Stdout/Stderr を接続してRunしてください）。
func SSHFromNS(ctx context.Context, netns, sshTarget string) *exec.Cmd {
	return exec.CommandContext(ctx, "ip", "netns", "exec", netns, "ssh", sshTarget)
}
```

### 使い方の例

```go
cfg := Config{
	NetNS:   "ns-app",
	VethOVS: "veth-ovs",
	VethNS:  "veth-ns",
	Bridge:  "br-ae9f7",
	IfName:  "eth0",
	CIDR:    "172.16.90.100/24",
}

if err := Setup(context.Background(), cfg); err != nil {
	log.Fatal(err)
}
defer Teardown(context.Background(), cfg)

if err := PingFromNS(context.Background(), cfg.NetNS, "172.16.90.2", 2); err != nil {
	log.Fatal(err)
}
```

### ポイント

- **失敗時のロールバック**: シェルスクリプトでは1コマンドずつ手動実行しますが、Goでは途中失敗時に `errCleanup` でOVSポート削除→netns削除まで戻す設計にしています（`veth-ns` は netns 削除時に自動で消えるので個別削除は不要）。
- **`ip netns exec` のラップ**: 複数コマンドを同じnetns内で実行するため `nsExec` ヘルパーで共通化しています。
- **ssh は対話的**: `SSHFromNS` は `*exec.Cmd` を返すだけにし、呼び出し側で `Stdin`/`Stdout`/`Stderr` を `os.Stdin` などに接続してから `Run()` する想定です（パスワードプロンプトや鍵認証のため）。


## Go言語での実装の検討２

OVSブリッジへのポート追加部分だけ `ovs-vsctl` コマンドを利用し、それ以外（netns作成・veth作成・netns移動・アドレス設定・up・ping）はシステムコール（`AF_NETLINK` ソケット経由のrtnetlink操作、`unshare(2)`/`setns(2)`）で実装する案です。

`ovs-vsctl add-port` はカーネルsyscallではなく、ユーザー空間の `ovsdb-server` に対するOVSDB(JSON-RPC over UNIXドメインソケット)通信のため、この部分だけはコマンド呼び出しを残しています。

### 追加が必要な依存

```
go get github.com/vishvananda/netlink
go get github.com/vishvananda/netns
```

（現状の marmot は `exec.Command` 統一のためこの2つは未使用。導入する場合は go.mod への追加が必要です）

### 実装コード

```go
//go:build linux

package netnsbridge

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

// Config は netns ⟷ OVS ブリッジ接続に必要なパラメータです。
type Config struct {
	NetNS   string // 例: "ns-app"
	VethOVS string // 例: "veth-ovs" (ホスト/OVS側)
	VethNS  string // 例: "veth-ns"  (netns側、後でeth0にrename)
	Bridge  string // 例: "br-ae9f7"
	IfName  string // netns内でのIF名。例: "eth0"
	CIDR    string // 例: "172.16.90.100/24"
}

// runOVS はOVSDBを操作するため ovs-vsctl コマンドのみ利用する（syscallでは完結しないため）。
func runOVS(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ovs-vsctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ovs-vsctl %v failed: %w, output: %s", args, err, string(out))
	}
	return nil
}

// Setup は選択部分をnetns/veth操作=syscall、OVS接続=コマンドで再現します。
func Setup(ctx context.Context, c Config) (err error) {
	// 1. netns作成（unshareベース、"ip netns add"相当）
	newHandle, err := netns.NewNamed(c.NetNS)
	if err != nil {
		return fmt.Errorf("create netns: %w", err)
	}
	defer newHandle.Close()

	origHandle, err := netns.Get()
	if err != nil {
		return fmt.Errorf("get current netns: %w", err)
	}
	defer origHandle.Close()

	// netns.NewNamed実行後はカレントスレッドが新netnsに切り替わっているため、ホスト側に戻す
	if err := netns.Set(origHandle); err != nil {
		_ = netns.DeleteNamed(c.NetNS)
		return fmt.Errorf("restore original netns: %w", err)
	}

	defer func() {
		if err != nil {
			_ = runOVS(ctx, "del-port", c.Bridge, c.VethOVS)
			_ = netns.DeleteNamed(c.NetNS)
		}
	}()

	// 2. veth pair作成（rtnetlink RTM_NEWLINK、"ip link add ... type veth peer name ..."相当）
	veth := &netlink.Veth{
		LinkAttrs: netlink.LinkAttrs{Name: c.VethOVS},
		PeerName:  c.VethNS,
	}
	if err = netlink.LinkAdd(veth); err != nil {
		return fmt.Errorf("add veth: %w", err)
	}

	// 3. veth-ovsをOVSブリッジへ接続（OVSDB通信のためコマンド利用）
	if err = runOVS(ctx, "add-port", c.Bridge, c.VethOVS); err != nil {
		return err
	}

	// 4. veth-ovsをup
	if err = netlink.LinkSetUp(veth); err != nil {
		return fmt.Errorf("set veth-ovs up: %w", err)
	}

	// 5. veth-nsを対象netnsへ移動（RTM_SETLINK + IFLA_NET_NS_FD、"ip link set veth-ns netns ns-app"相当）
	peer, err := netlink.LinkByName(c.VethNS)
	if err != nil {
		return fmt.Errorf("lookup veth-ns: %w", err)
	}
	if err = netlink.LinkSetNsFd(peer, int(newHandle)); err != nil {
		return fmt.Errorf("move veth-ns to netns: %w", err)
	}

	// 6. netns内でrename/addr/up設定（setns + rtnetlinkの組み合わせ）
	if err = configureInNS(c); err != nil {
		return err
	}

	return nil
}

// configureInNS はカレントスレッドを対象netnsへ切り替えてから
// rename/addr-add/link-up/lo-upを行い、完了後にホストnetnsへ戻します。
func configureInNS(c Config) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	origHandle, err := netns.Get()
	if err != nil {
		return fmt.Errorf("get current netns: %w", err)
	}
	defer origHandle.Close()

	targetHandle, err := netns.GetFromName(c.NetNS)
	if err != nil {
		return fmt.Errorf("get target netns: %w", err)
	}
	defer targetHandle.Close()

	if err := netns.Set(targetHandle); err != nil {
		return fmt.Errorf("enter netns %s: %w", c.NetNS, err)
	}
	defer netns.Set(origHandle) // このスレッドをホストnetnsへ戻す

	// "ip netns exec ns-app ip link set veth-ns name eth0" 相当
	link, err := netlink.LinkByName(c.VethNS)
	if err != nil {
		return fmt.Errorf("lookup %s in netns: %w", c.VethNS, err)
	}
	if err := netlink.LinkSetName(link, c.IfName); err != nil {
		return fmt.Errorf("rename to %s: %w", c.IfName, err)
	}

	// renameでLinkAttrsが変わるため取り直す
	link, err = netlink.LinkByName(c.IfName)
	if err != nil {
		return fmt.Errorf("lookup %s after rename: %w", c.IfName, err)
	}

	// "ip netns exec ns-app ip addr add 172.16.90.100/24 dev eth0" 相当
	addr, err := netlink.ParseAddr(c.CIDR)
	if err != nil {
		return fmt.Errorf("parse CIDR %s: %w", c.CIDR, err)
	}
	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("add addr: %w", err)
	}

	// "ip netns exec ns-app ip link set eth0 up" 相当
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("set %s up: %w", c.IfName, err)
	}

	// "ip netns exec ns-app ip link set lo up" 相当
	lo, err := netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("lookup lo: %w", err)
	}
	if err := netlink.LinkSetUp(lo); err != nil {
		return fmt.Errorf("set lo up: %w", err)
	}

	return nil
}

// Teardown はクリーンアップ（OVSポート削除=コマンド、netns削除=syscall）を行います。
func Teardown(ctx context.Context, c Config) error {
	if err := runOVS(ctx, "del-port", c.Bridge, c.VethOVS); err != nil {
		return err
	}
	return netns.DeleteNamed(c.NetNS)
}
```

### ポイント

- **OVS部分だけ `exec.Command("ovs-vsctl", ...)`**: `add-port` / `del-port` は OVSDB(JSON-RPC)通信であり、rtnetlinkのようなカーネルsyscallでは完結しないため、ここだけはコマンド呼び出しを残しています。
- **それ以外は `vishvananda/netlink` / `vishvananda/netns`**: 内部的には `AF_NETLINK` ソケットと `unshare(2)`/`setns(2)` syscallをGoから直接呼んでおり、`ip` プロセスをforkしません。
- **`runtime.LockOSThread()` が必須**: `setns(2)` はスレッド単位に効くため、netns切り替えを伴う `configureInNS` ではOSスレッドを固定し、処理後に必ずホストnetnsへ戻しています。
- **失敗時のクリーンアップ**: `Setup` の途中で失敗した場合、`defer` 内で `err != nil` を見てOVSポート削除とnetns削除を行います（`veth-ns` はnetns削除時に自動消滅するため個別削除不要）。