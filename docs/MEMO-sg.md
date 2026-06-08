# セキュリティグループの検討メモ


**方式1: OVN を単一/クラスタの共通データプレーンにする（最も同一）**
1. 単一構成でも OVN（ovn-central + ovn-controller）を動かす。
2. SG は常に OVN ACL と address-set に変換して適用する。
3. geneve の有無はトンネル有無だけの差にする（SG 実装は同一）。

この方式の利点:
1. 単一とクラスタで「同じコードパス」を使える。
2. SG-to-SG 参照や stateful 制御を同じ表現で扱える。
3. 将来の機能追加が 1 系統で済む。

marmot の差し込み先:
1. ネットワーク層の共通適用点は ovn.go と ovs.go。
2. 制御ループは network-controller.go。
3. VM NIC の情報源は server.go と marmot-api-v1.go。


**「実装を同じ」に最も効く具体策**
1. SG API は 1 つに統一する（NIC に SG を付与）。
2. SG 適用エンジンは OVN ACL のみを正とする。
3. 単一構成でも OVN を有効化し、クラスタと同じ適用コードを通す。
4. 既存 OVS/iptables は移行期間の fallback のみにする。

要するに、完全同一を狙うなら「単一でも OVN を使う」が最短です。  
必要なら次に、marmot 向けに「どの関数に SG コンパイル/適用を追加するか」をファイル単位で実装計画に落とします。


```console
ubuntu@ws1:~/marmot$ systemctl status ovn-central
● ovn-central.service - Open Virtual Network central components
     Loaded: loaded (/usr/lib/systemd/system/ovn-central.service; enabled; preset: enabled)
     Active: active (exited) since Mon 2026-06-08 19:46:44 JST; 9min ago
   Main PID: 1462 (code=exited, status=0/SUCCESS)
        CPU: 13ms

Jun 08 19:46:43 ws1 systemd[1]: Starting ovn-central.service - Open Virtual Network central components...
Jun 08 19:46:44 ws1 systemd[1]: Finished ovn-central.service - Open Virtual Network central components.
```

```console
ubuntu@ws1:~/marmot$ systemctl status ovn-controller
● ovn-controller.service - Open Virtual Network host control daemon
     Loaded: loaded (/usr/lib/systemd/system/ovn-controller.service; static)
     Active: active (running) since Mon 2026-06-08 19:46:44 JST; 9min ago
   Main PID: 1646 (ovn-controller)
      Tasks: 5 (limit: 15288)
     Memory: 4.7M (peak: 5.7M)
        CPU: 250ms
     CGroup: /system.slice/ovn-controller.service
             └─1646 ovn-controller unix:/var/run/openvswitch/db.sock -vconsole:emer -vsyslog:err -vfile:info --no-chdir --log-file=/var/log/ovn/ovn-controller.log --pidfile=/var/run/ovn/ovn-controller.pid --detach

Jun 08 19:46:43 ws1 systemd[1]: Starting ovn-controller.service - Open Virtual Network host control daemon...
Jun 08 19:46:43 ws1 (ovn-ctl)[1464]: ovn-controller.service: Referenced but unset environment variable evaluates to an empty string: OVN_CTL_OPTS
Jun 08 19:46:44 ws1 ovn-ctl[1464]:  * Starting ovn-controller
Jun 08 19:46:44 ws1 systemd[1]: Started ovn-controller.service - Open Virtual Network host control daemon.
```

```console
ubuntu@ws1:~/marmot$ sudo ovs-vsctl get open . external_ids:ovn-remote
"tcp:10.1.0.15:6642"

ubuntu@ws1:~/marmot$ ip a |grep 10.1.0.15
    inet 10.1.0.15/8 brd 10.255.255.255 scope global enx00e04c572830
```

## 実装のロードマップ

以下が、OVN を単一/クラスタ共通データプレーンに統一するための実装ロードマップです。  
現状のあなたの環境は OVN サービスが起動済みなので、Phase 0 はほぼ満たせています。

**前提**
1. SG の唯一の正規実装は OVN ACL と address-set にする。
2. 単一ノードでも OVN を常用し、クラスタとの差はトンネル有無のみとする。
3. 既存 OVS/iptables は移行期間のみ fallback。

**Phase 0: 基盤整備（1-2日）**
1. 起動要件を固定化  
対象: marmotd-main.go, ovn_bootstrap.go  
完了条件: ovn-central, ovn-controller, ovn-remote, ovn-encap-ip が起動時に整合する。
2. 監視観点の追加  
対象: host-controller.go  
完了条件: OVN bootstrap 失敗が status/message に反映される。

**Phase 1: API モデル導入（3-5日）**
1. OpenAPI に SG リソース追加  
対象: marmot-api-v1.yaml  
追加: SecurityGroup, SecurityGroupRule, SecurityGroupAttachment。
2. NIC に SG 参照を追加  
対象: marmot-api-v1.yaml, 生成物 marmot-api-v1.go  
完了条件: Server の NIC 単位で securityGroups を指定可能。
3. CRUD ハンドラ追加  
対象: marmotd 配下 API 実装  
完了条件: SG の作成/更新/削除/一覧が動作。

**Phase 2: SG コンパイラ実装（4-6日）**
1. SG ルールを OVN ACL へ変換するコンパイラを実装  
新規候補: networkfabric 配下に sg_compiler.go, sg_compiler_test.go  
2. 変換対象  
ingress/egress, protocol, port range, CIDR source/destination, stateful 許可。
3. 完了条件  
同一入力 SG から決定的に同一 ACL セットが生成される。

**Phase 3: OVN 適用エンジン実装（4-7日）**
1. OVN オブジェクト管理  
対象: ovn.go  
実装: address-set, port-group, acl add/del, idempotent reconcile。
2. ラベル設計  
external_ids に marmot managed key を付与し GC を安全化。
3. 完了条件  
再実行しても差分のみ更新、不要 ACL は安全に回収。

**Phase 4: VM NIC 連携（3-5日）**
1. VM NIC と OVN 論理ポートの対応付けを確立  
対象: server.go, libvirtXml2.go  
方針: NIC ごとの安定キーを external_ids に保持。
2. Server 更新時の再調停  
対象: server-controller.go, network-controller.go  
完了条件: NIC の SG 変更が数秒以内で反映される。

**Phase 5: 移行モードと互換（3-4日）**
1. feature flag 追加  
例: SG_BACKEND=ovn|legacy  
対象: config と起動設定ロード部分。
2. fallback 運用  
OVN 不全時の fail-close/fail-open を明確化。
3. 完了条件  
段階移行中に既存運用を壊さない。

**Phase 6: テストとリリース（5-8日）**
1. ユニット  
対象: ovn_test.go 拡張、SG コンパイラ単体。
2. 統合  
単一構成: SG 適用の疎通検証。  
クラスタ構成: ノード跨ぎ通信で同一挙動検証。
3. E2E 受け入れシナリオ  
ssh は指定ネットワークのみ、http は許可 CIDR のみ、3306/tcp は 172.16.90.0/24 のみ許可。
4. 完了条件  
単一/クラスタで同一 API 入力に対し同一の通信可否結果。

**優先実装順（最短で価値を出す順）**
1. Phase 1 の API と NIC SG 参照
2. Phase 2 の CIDR ベース ingress のみ先行
3. Phase 3-4 で OVN 反映と NIC 紐付け
4. 最後に egress と SG 参照（SG-to-SG）

**概算スケジュール**
1. MVP（CIDR ingress + NIC SG + OVN 適用）: 2-3 週間
2. 完全版（egress, SG-to-SG, 移行機能, E2E）: 4-6 週間

必要であれば次に、上記をそのまま MEMO-sg.md に実装タスク形式（担当・期限・Done 条件つき）で追記します。

---

以下は、SG導入後を想定した サーバー生成YAML のたたき台です。  
前提として、現行APIには未実装なので、提案フィールドとして見てください。

想定する拡張ポイント
1. kind: SecurityGroup を追加
2. Server.spec.networkInterface[].securityGroups を追加
3. ルールは ingress/egress の allow-list

YAML案（複数リソースを一括適用）

    apiVersion: v1
    kind: SecurityGroup
    metadata:
      name: sg-web-frontend
    spec:
      description: public nic for ssh and http
      rules:
        - direction: ingress
          protocol: tcp
          portRange:
            from: 22
            to: 22
          source:
            cidrs:
              - 10.10.0.0/16
              - 192.168.10.0/24
          description: allow ssh from admin networks

        - direction: ingress
          protocol: tcp
          portRange:
            from: 80
            to: 80
          source:
            cidrs:
              - 203.0.113.10/32
              - 198.51.100.0/24
          description: allow http from known addresses

        - direction: egress
          protocol: all
          destination:
            cidrs:
              - 0.0.0.0/0
          description: allow all outbound

    ---
    apiVersion: v1
    kind: SecurityGroup
    metadata:
      name: sg-db-3306
    spec:
      description: allow mysql from app segment only
      rules:
        - direction: ingress
          protocol: tcp
          portRange:
            from: 3306
            to: 3306
          source:
            cidrs:
              - 172.16.90.0/24
          description: mysql from app network only

        - direction: egress
          protocol: all
          destination:
            cidrs:
              - 0.0.0.0/0
          description: allow outbound

    ---
    apiVersion: v1
    kind: Server
    metadata:
      name: app-01
    spec:
      cpu: 4
      memory: 8192
      osVariant: ubuntu24.04
      networkInterface:
        - networkname: host-bridge
          address: 10.1.0.50
          netmasklen: 24
          securityGroups:
            - sg-web-frontend

        - networkname: private-app
          address: 172.16.90.10
          netmasklen: 24
          securityGroups:
            - sg-db-3306

この案で表現できること
1. SSHは管理ネットワークのみ許可
2. HTTPは既知アドレスのみ許可
3. 3306/tcpは 172.16.90.0/24 のみ許可
4. NICごとに異なるSGを適用

必要なら次に、marmot向けに実装しやすい最小フィールドセットへ削った版（まずCIDRのみ、SG参照なし）も出します。