# LoadBalancer の最小設計案

## 目的

issue 369 の LoadBalancer を、Gateway のような専用 VM を起動せずに実装する。
Marmot はすでに OVN/OVS を制御する経路を持つため、LoadBalancer も controller から OVN の論理オブジェクトを直接操作する。

このメモでは、最小実装として次を成立条件にする。

- 専用 VM を作らない
- Ansible を使わない
- バックエンドは既存の Server リソースを使う
- 内部仮想ネットワーク上の VIP 提供を先に実装する
- bindPublicIpAddress を使う外部公開は、この設計メモの範囲外とする

## この設計で切り捨てるもの

最小実装では、以下はスコープ外とする。

- host-bridge 上の公開 VIP を使う外部公開
- bindPublicIpAddress
- L7 機能
- ヘルスチェック
- セッション維持
- 重み付け
- backend が Server 以外のオブジェクト

理由は、内部ネットワーク向けの L4 LoadBalancer であれば OVN の論理 load balancer を直接使える一方、host-bridge への公開は localnet、router、NAT、ARP 応答の整理まで必要になり、最小設計から外れるためである。

## API の最小仕様

```yaml
apiVersion: v1
kind: LoadBalancer
metadata:
  name: lb1
spec:
  backendMode: auto
  internalVirtualNetwork: webs-net
  serverPorts:
    - http
    - https
    - 1234/tcp
```

最小実装で有効にする項目:

- metadata.name
- spec.backendMode
- spec.internalVirtualNetwork
- spec.serverPorts

最小実装で optional にする項目:

- spec.virtualIpAddress
- spec.internalServers (backendMode=manual のときのみ)

最小実装で扱わない項目:

- spec.bindPublicIpAddress

bindPublicIpAddress は、この設計メモでは範囲外とする。
したがって、このメモに従う実装では spec.bindPublicIpAddress を受け付けない。
指定された場合は 400 で reject する。

## backendMode の扱い

backendMode は次の 2 値を受け付ける。

- manual
- auto

デフォルトは auto とする。

### manual

- internalServers の指定を必須とする
- backend は internalServers で指定された Server のみを対象とする

### auto

- internalServers の指定を禁止する
- backend は internalVirtualNetwork 上の Server から自動検出する

auto の対象条件は次の 3 点。

1. internalVirtualNetwork に接続していること
2. status が RUNNING であること
3. ラベル lb-enabled=true を持つこと

## virtualIpAddress の扱い

issue 369 の記述では bindPublicIpAddress を省略した場合に internalVirtualNetwork 上へ VIP を置く想定になっている。
この設計では virtualIpAddress は optional とし、指定があればその値を使い、未指定なら controller が internalVirtualNetwork から VIP を自動採番する。

VIP の決定規則は次の通り。

1. spec.virtualIpAddress が指定されていれば、その値を使う
2. 未指定なら controller が DB の IP 割当ロジックを使って未使用 IP を採番する
3. 採番した VIP は labels または status に保持し、再 reconcile で再利用する

この方式により、利用者は VIP を明示指定してもよく、省略して自動採番に任せてもよい。

### 自動採番の前提

自動採番は、internalVirtualNetwork に割当対象の IPv4 ネットワークが定義されていることを前提とする。
採番は既存 Server と同じ IP 管理台帳を使い、backend IP と競合しないことを保証する。

### 明示指定と自動採番のトレードオフ

- 利点: 実装が単純
- 利点: 期待値が明確
- 利点: API テストが簡単
- 利点: 未指定なら利用者が VIP を決めなくてよい
- 注意点: 自動採番には IP ネットワーク定義と使用中 IP の一元管理が必要

最小実装では、自動採番まで含めて扱う。

## アーキテクチャ

### 責務分離

- API/CLI: LoadBalancer リソースを受け付けて保存する
- DB: LoadBalancer の spec、status、label を保持する
- controller: spec を監視し、backend IP を解決し、OVN を同期する
- networkfabric: OVN load balancer の create/update/delete を隠蔽する

Gateway 方式との違いは、Linux guest OS に iptables を入れるのではなく、OVN Northbound DB 上に load balancer を作成して論理スイッチへ関連付ける点にある。

### データプレーン

最小実装のデータプレーンは OVN の load balancer を使う。

- VIP は spec.virtualIpAddress または自動採番した VIP
- backend は backendMode に応じて manual 指定または auto 検出で解決した IP 群
- protocol/port は serverPorts から展開する
- 対象 logical switch は internalVirtualNetwork に対応するスイッチ

VIP が確定したら、internalVirtualNetwork の内部 DNS に metadata.name を使って A レコードを登録する。

概念上の同期結果は以下。

- LoadBalancer リソース 1 個
- OVN load balancer 1 個
- serverPorts の各要素に対応する VIP エントリ
- internalVirtualNetwork の logical switch への関連付け
- metadata.name.internalVirtualNetwork に対応する内部 DNS エントリ 1 個

## controller の最小ステート

既存の controller 流儀に合わせ、15 秒ループでよい。

- PENDING: spec のバリデーションと backend 解決を行う
- PROVISIONING: OVN load balancer を作成または更新する
- ACTIVE: drift 修復だけ行う
- DELETING: OVN から切り離して削除する
- FAILED: 再 apply か更新待ち

最小設計では Gateway のような VM ライフサイクル待ちは存在しないため、PENDING から ACTIVE まで 1 ループで遷移可能である。

## reconcile の流れ

1. LoadBalancer を列挙する
2. internalVirtualNetwork が存在することを確認する
3. backendMode を解決する (未指定時は auto)
4. backendMode=manual の場合は internalServers を検証し、指定された server から backend IP を解決する
5. backendMode=auto の場合は internalVirtualNetwork 上の server を走査し、対象条件 3 点で backend を自動検出する
6. backend が 0 台の場合は PROVISIONING を維持する
7. virtualIpAddress が未指定なら VIP を自動採番し、指定があれば妥当性を検証する
8. serverPorts を tcp/udp の具体値へ展開する
9. OVN load balancer の desired state を生成する
10. desired state を OVN に反映する
11. VIP を内部 DNS へ登録する
12. status を ACTIVE に更新する

削除時は逆順で、内部 DNS エントリを削除し、logical switch から関連付けを外し、load balancer を削除し、DB エントリを消す。

## バリデーション仕様

作成時に最低限チェックする内容:

1. metadata.name は internalVirtualNetwork 単位で一意
2. backendMode は manual または auto のみ許可 (未指定時は auto)
3. backendMode=manual では internalServers を必須とする
4. backendMode=manual では internalServers に重複不可
5. backendMode=auto では internalServers の指定を禁止する
6. internalVirtualNetwork は存在必須
7. virtualIpAddress が指定されている場合は internalVirtualNetwork の CIDR 内にあること
8. virtualIpAddress が指定されている場合は backend IP と重複不可
9. serverPorts は service 名または n/tcp, n/udp のみ許可
10. bindPublicIpAddress は範囲外として reject
11. virtualIpAddress が未指定で自動採番できない場合は reject

更新時の最小仕様:

1. 変更許可は backendMode, internalServers, serverPorts のみ
2. internalVirtualNetwork は immutable
3. virtualIpAddress は初回確定後は immutable
4. bindPublicIpAddress は範囲外のため指定不可

## backend IP 解決

backendMode により解決方法を切り替える。

### manual

internalServers は Server の名前リストとして扱う。

### auto

internalVirtualNetwork 上の Server から backend を自動検出し、対象条件は次の 3 点。

1. internalVirtualNetwork に接続している
2. status が RUNNING
3. ラベル lb-enabled=true を持つ

共通で、各 backend は以下の条件を満たす必要がある。

- Server が存在する
- status が RUNNING である
- internalVirtualNetwork 上に NIC を持つ
- その NIC に IP が割り当てられている

1 台でも解決できない server がある場合、最小実装では全体を FAILED にせず、ACTIVE には遷移させないで PROVISIONING を維持する方が運用しやすい。

理由:

- 起動順の前後に耐えやすい
- backend の復帰時に自動収束しやすい

ただし、manual で存在しない server 名が指定された場合は利用者エラーなので FAILED が妥当である。

auto で条件に一致する backend が 0 台のときは、PROVISIONING を維持する。

## VIP の確定と内部 DNS 登録

VIP は次のいずれかで確定する。

- spec.virtualIpAddress による明示指定
- controller による自動採番

VIP が確定したら、内部 DNS に metadata.name をホスト名、internalVirtualNetwork をサブドメインとして登録する。
これにより、同一内部ネットワーク上のクライアントは LoadBalancer を名前で解決できる。

削除時は DNS エントリも必ず削除する。

## networkfabric への追加責務

既存の OVN bridge/switch 制御とは別に、次の抽象を追加する。

- EnsureLoadBalancer
- DeleteLoadBalancer
- GetLoadBalancerStatus

入力は次の程度で足りる。

- LoadBalancer ID
- logical switch 名
- VIP 一覧
- backend 一覧

controller は desired state の生成だけ行い、ovn-nbctl の詳細は networkfabric に閉じ込める。

## OVN への反映イメージ

最小実装では、OVN 上で次を冪等に同期する。

1. marmot 管理の load balancer オブジェクトを確保する
2. 各 serverPort に対応する VIP と backend 群を同期する
3. internalVirtualNetwork の logical switch に load balancer を関連付ける
4. delete 時は逆順で切り離す

識別子は external_ids に以下を持たせると追跡しやすい。

- marmot_loadbalancer_id
- marmot_network_id
- marmot_managed=true

## status と labels

status の最小構成:

- statusCode
- message
- observedGeneration
- virtualIpAddress
- resolvedBackends

labels に保持すると有用なもの:

- appliedConfigHash
- logicalSwitchName
- ovnLoadBalancerName

appliedConfigHash があれば、spec と backend 解決結果が同じときに無駄な再同期を避けられる。

## テストの最小範囲

VM なしで成立するテストを優先する。

### ユニットテスト

- serverPorts の解決
- backendMode の既定値が auto になること
- backendMode=auto で internalServers を reject すること
- backendMode=manual で internalServers 必須になること
- virtualIpAddress 指定時の CIDR 検証
- virtualIpAddress 未指定時の自動採番
- backend 解決 (manual)
- backend 自動検出 (auto, 3 条件)
- VIP の内部 DNS 登録
- immutable 項目の update reject
- desired OVN state 生成

### OVN コマンドモックテスト

既存の [pkg/networkfabric/ovn_test.go](pkg/networkfabric/ovn_test.go#L1) と同じ方式で、ovn-nbctl 実行を差し替える。

- lb 作成
- VIP 更新
- logical switch への関連付け
- delete

### 統合テスト

最小設計では必須ではない。
少なくとも API/DB/controller/networkfabric の大半はモックで検証できる。

## 段階的リリース案

### Phase 1

- API/DB/CLI で LoadBalancer を保存・表示できる
- bindPublicIpAddress は範囲外として reject
- controller は未実装

### Phase 2

- controller が backend を解決し、OVN load balancer を内部ネットワークへ作成できる
- virtualIpAddress は optional とし、未指定時は自動採番する
- VIP を内部 DNS へ登録する

### Phase 3

- ACTIVE で drift 修復
- backend の増減を自動反映
- configHash で不要同期を抑制

### Phase 4

- bindPublicIpAddress を使う外部公開が必要なら、別設計メモとして追加で定義する
- 必要なら localnet / logical router / NAT の導入を検討

## 実装順序の提案

1. API 型と DB 保存を追加する
2. serverPorts の正規化関数を切り出す
3. backend 解決関数を切り出す
4. networkfabric に OVN load balancer 操作を追加する
5. controller に reconcile を追加する
6. モックテストを追加する

## 結論

issue 369 は、専用 VM なしで実装できる。
最小実装としては、内部仮想ネットワーク向けの OVN ネイティブ L4 LoadBalancer に限定し、bindPublicIpAddress はこの設計メモの範囲外として扱うのが最も小さく、既存の Marmot の controller / networkfabric 構造にも適合する。