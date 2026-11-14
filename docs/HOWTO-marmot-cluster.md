# maadm cluster コマンドで marmot クラスタを構成する

`maadm cluster` は、marmotd を介さずに etcd と `/etc/marmot/marmotd.json` を直接変更して、
ノードをクラスタへ編入（`init` / `join`）、離脱（`leave`）させる管理者用コマンドです。
すべてのサブコマンドは **root 権限** で実行します。

| サブコマンド | 実行するノード | 役割 |
|---|---|---|
| `maadm cluster init` | クラスタのコントロールノード（マスター） | シングルノードの marmot をクラスタのマスターへ変更し、CLUSTER-ID を発行する |
| `maadm cluster join CLUSTER-UUID` | 追加するノード | マスターのクラスタへ編入する |
| `maadm cluster leave CLUSTER-UUID` | 離脱するノード | クラスタから離脱して、シングルノードへ戻る |

クラスタの確認は `mactl cluster` を使います（`maadm` に状態表示のコマンドはありません）。

構成手順の全体像や手動構成は [HOWTO-install-marmot-cluster.md](HOWTO-install-marmot-cluster.md)、
設計メモは [MEMO-maadm-cluster-command.md](MEMO-maadm-cluster-command.md) を参照してください。

## 共通事項

- 変更の前に、変更内容を表示して `続行しますか? [y/N]:` を確認します。`--force` で省略できます。
- `--dry-run` を指定すると、変更内容を表示するだけで何も変更しません。最初に必ず実行してください。
- 変更するファイルは `*.bak` へ退避されます。`init` と `join` は失敗すると元のシングルノード構成へ戻します。
- 各ノードで次のファイルが設定済みであることが必要です。
    - `/etc/marmot/marmotd.json` の `node_name`（クラスタ内で重複しない名前）
    - `/etc/hostid`（クラスタ内で重複しない HOSTID。[HOWTO-install-marmot-cluster.md](HOWTO-install-marmot-cluster.md) 参照）
- ノード間で次のポートが通ること。
    - etcd: TCP 2379（マスターへ）
    - OVN SB DB: TCP 6642（OVN 利用時）
    - Geneve: UDP 6081（ノード間）

## クラスタを構成する

### 1. マスターノードで init を実行する

marmot と etcd がシングルノードとして稼働している（`etcd.service` と `marmot.service` が active）ノードで実行します。

```bash
sudo maadm cluster init --etcd-ip 10.1.0.13 --underlay-interface br0
```

| オプション | 説明 | 既定値 |
|---|---|---|
| `--etcd-ip` | etcd が待ち受け、他ノードへ公開する IPv4 アドレス | `--underlay-interface` の IPv4 アドレス |
| `--underlay-interface` | ノード間の Geneve に使うインターフェース。`default_underlay_interface` に設定される | 現在の `default_underlay_interface`、未設定なら `br0` |
| `--dry-run` | 変更内容の表示のみ | 無効 |
| `--force` | 確認プロンプトを省略 | 無効 |

実行内容:

1. 事前チェック（root、etcd / marmot の稼働、インターフェースの IPv4 アドレス）。すでにクラスタが有効なら、CLUSTER-ID を表示して何も変更せず終了する。
2. drop-in `/etc/systemd/system/etcd.service.d/marmot-cluster.conf` を作成し、etcd を `127.0.0.1:2379` と `<etcd-ip>:2379` で待ち受けるよう変更して再起動する（`etcd.service` 本体は変更しない）。
   `ETCD_LISTEN_CLIENT_URLS` / `ETCD_ADVERTISE_CLIENT_URLS` が設定済みの場合は、競合を避けるため drop-in で無効化する。
3. `marmotd.json` の `etcd_url` と `default_underlay_interface` を更新する（他の項目は変更しない）。
4. etcd へ CLUSTER-ID（UUID）とマスターのノード名を保存する。
5. marmot を再起動する。OVN の設定（`ovn-encap-ip` など）は marmotd が自動で行う。

marmot の再起動中は API が一時的に使えなくなりますが、稼働中の VM は停止しません。

成功すると、他のノードで実行するコマンドが表示されます。

```
CLUSTER-ID: 6f1c0c0e-....
etcd: http://10.1.0.13:2379
他のノードで次のコマンドを実行してください:
  maadm cluster join 6f1c0c0e-.... --master-node 10.1.0.13
```

### 2. 他のノードで join を実行する

```bash
sudo maadm cluster join 6f1c0c0e-.... --master-node 10.1.0.13 --underlay-interface br0
```

| オプション | 説明 | 既定値 |
|---|---|---|
| `--master-node` | マスターの IP アドレスまたはホスト名（必須。ポート番号は含めない。etcd は 2379 固定） | なし |
| `--underlay-interface` | ノード間の Geneve に使うインターフェース | `br0`（ノードの現在の設定値は引き継がない） |
| `--dry-run` / `--force` | init と同じ | 無効 |

実行内容:

1. 事前チェック。次の場合はエラーになり、何も変更しない。
    - すでにクラスタのメンバーである。
    - **ローカルの etcd に VM・ネットワークなどの管理対象が 1 件でもある**（クラスタへ引き継がれないため）。
2. マスターの etcd へ接続し、CLUSTER-ID の一致、`node_name` と HOSTID が既存ノードと重複しないことを確認する。
3. marmot を停止し、ローカルの etcd を `systemctl disable --now` で停止・無効化する。`marmotd.json` の `etcd_url` をマスターの etcd へ、`default_underlay_interface` を指定値へ更新する。
4. marmot を起動する。
5. マスターの etcd に自ノードが登録されるのを待つ（最大 30 秒）。失敗した場合は、マスター上の自ノードの情報を削除して元のシングルノード構成へ戻す。

成功するとノード一覧が表示されます。以降のこのノードのデータはマスターの etcd に保存されます。

### 3. 確認する

```bash
mactl cluster
```

クラスタが有効なら CLUSTER-ID とノード一覧が表示され、無効ならエラーになります。
Geneve トンネルの状態は [HOWTO-trableshort-geneve](HOWTO-trableshort-geneve) を参照してください。

## クラスタから離脱する

離脱するノード上で実行します。**このノード上の VM とボリュームは削除され、復元できません。** 必要なデータは事前にバックアップしてください。

```bash
sudo maadm cluster leave 6f1c0c0e-....
```

| オプション | 説明 |
|---|---|
| `--dry-run` | 削除される VM と変更内容の表示のみ |
| `--force` | 確認プロンプトを省略 |

離脱できない条件（エラーになり、何も変更しない）:

- このノードがクラスタのメンバーでない、または CLUSTER-ID が一致しない。
- このノードが **マスターノード**（etcd を稼働させているノード）である。
- このノードが **OVN central の接続先**（稼働中のノードのうち HOSTID が最小のノード）である。
- マスターの etcd へ接続できない。
- VM があるのに marmot が停止している（VM の削除に marmot の稼働が必要）。

実行内容:

1. 事前チェックと、削除される VM の一覧表示・確認。
2. このノードを配置禁止（cordon）にし、VM をシャットダウンして削除する（シャットダウンは最大 120 秒、超えると強制停止。削除とボリューム消滅の確認は各最大 300 秒）。
3. marmot を停止し、mgmt ネットワークの head だった場合は残りのノードへ引き継ぎ、クラスタ上の自ノードの follower ネットワークとホスト情報を削除する。
4. `marmotd.json` の `etcd_url` を `http://127.0.0.1:2379` へ戻し、ローカルの etcd を `systemctl enable --now` で起動する。
5. OVN のクリーンアップ（`ovn-remote` の削除、SB DB からの chassis 削除、follower ネットワークの libvirt 定義の削除）。失敗しても警告を表示して続行する。
6. marmot を起動する（シングルノード）。

### 途中で失敗した場合

- 手順2（VM の削除）で失敗した場合は、クラスタのメンバーのままです。原因を解消して再実行すると、残りの VM から続行します。
- 手順3以降で失敗した場合、`join` / `init` のようなロールバックは **行いません**。`/etc/marmot/marmotd.json.leave` に状態が記録されるため、同じコマンド（同じ CLUSTER-ID）を再実行すると、未完了の手順から続行します。
- 削除待ちのボリューム ID は `/etc/marmot/marmotd.json.leave-volumes` に記録され、再実行時に引き継がれます。

離脱後のノードはローカルの etcd だけを使うシングルノードとして動作し、再び `join` できます。

## 現状の問題点・制限事項

### 可用性・セキュリティ

- **etcd は単一構成で、マスターが単一障害点です。** マスターの etcd が停止すると、クラスタ全体の API と各ノードの marmotd が影響を受けます。
- etcd の通信は **平文の HTTP で、認証もありません**（`http://<etcd-ip>:2379`）。信頼できる管理ネットワークでのみ利用し、TCP 2379 へのアクセスをファイアウォールで制限してください。
- マスターの etcd は設計上 1 台のため、マスターノードを離脱・移行する手段がありません（`leave` はマスターを拒否します）。マスターを入れ替える場合は、クラスタ全体の再構築が必要です。
- `init` を取り消すコマンド（クラスタの解体）はありません。全ノードを `leave` しても、マスターは元のシングルノードへは戻りません（etcd の公開設定とクラスタ情報が残ります）。

### join の制限

- ローカルの管理対象（VM、ネットワークなど）が残っているノードは `join` できません。VM などを削除してから実行します（既存データの移行機能はありません）。
- HOSTID が **現在のスケジューラーリーダー（稼働中のノードのうち最小の HOSTID）より小さい**、または稼働中のノードが無い場合、OVN central の接続先を安全に維持できないため `join` を拒否します。クラスタへ後から加えるノードは、既存ノードより大きい HOSTID にしてください（`/etc/hostid` の変更が必要）。
- `--underlay-interface` の既定値は常に `br0` で、`marmotd.json` の現在値は使われません。`br0` 以外を使うノードでは明示してください。
- `--master-node` にポート番号は指定できません（2379 固定）。

### leave の制限

- VM とボリュームは **強制的に削除** されます（移動・退避はできません）。先に別ノードへ作り直すかバックアップしてください。
- HOSTID が最小のノード（OVN central の接続先）は離脱できません。OVN DB とゲスト論理ポートの移行手段がなく、現状は、残りのノードの HOSTID を調整するなど、運用で回避する必要があります。
- 手順3以降の失敗はロールバックされず、再実行による続行が前提です。状態ファイル（`*.leave`, `*.leave-volumes`）を削除しないでください。
- OVN のクリーンアップの失敗は警告のみです。警告に表示された手動コマンド（`ovn-sbctl chassis-del` など）を確認してください。
- mgmt ネットワークの head を引き継ぐには、他のノードの follower ネットワークが作成済みである必要があります。作成待ちで失敗した場合は、時間をおいて再実行します。

### その他

- `init` / `join` は marmot（`join` では etcd も）を再起動・停止するため、実行中は API が一時的に利用できません。`init` では稼働中の VM は停止しません。
- `maadm cluster` はクラスタの状態表示、ノードの強制削除（応答しないノードの除外）を持ちません。ダウンしたノードのホスト情報は手動での削除が必要です。
- マスターの etcd 公開設定は `maadm` の drop-in（`marmot-cluster.conf`）で行うため、手動で `etcd.service` を編集した環境では、競合する設定が無効化される点に注意してください（`init` の確認画面に表示されます）。
- 手動で構成した既存クラスタを `maadm` の管理へ移行する手順は、[HOWTO-install-marmot-cluster.md](HOWTO-install-marmot-cluster.md) を参照してください。
