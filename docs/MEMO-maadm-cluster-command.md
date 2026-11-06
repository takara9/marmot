# Marmot cluster を構成するためのコマンドの設計メモ

maadmコマンドは、marmotデーモンを介さず、独自に etcd や marmotd.json 等を変更する管理者用のコマンドです。
maadm cluster コマンドを利用して、ノードをクラスタへ編入、離脱を簡単に実行できるようにします。

現時点では、etcdはシングル構成で、単一障害点です。これは、marmot v1.0に達するまでに改善するものとして、
現在は、etcd, marmotサービスは、シングル構成で設計を進めます。


## シングルノードのMarmotをMarmotクラスタへ変更
クラスタのコントロールノードになるノードで、`maadm cluster init` を root 権限で実行する。

```bash
sudo maadm cluster init --etcd-ip 10.1.0.13 --underlay-interface br0
```

### 前提条件
- marmot と etcd がインストール済みで、シングルノードとして動作していること（etcd は localhost で稼働中）。
- 実行ノードが、すでにクラスタのメンバーではないこと（`init` / `join` 済みでない）。
- `/etc/hostid` が設定され、他のノードと重複しない HOSTID であること（[HOWTO-install-marmot-cluster.md](HOWTO-install-marmot-cluster.md) 参照）。
- 他のノードから到達可能な、クラスタ用のIPアドレスを持つインターフェースがあること。
- 他のノードから、以下のポートへ到達できること。
    - etcd: TCP 2379
    - OVN SB DB: TCP 6642（OVN を利用する場合。接続先は marmotd が自動設定する）
    - Geneve: UDP 6081（ノード間）

### オプション（案）
| オプション | 説明 | 既定値 |
|---|---|---|
| `--etcd-ip` | etcd が待ち受け、他ノードへ公開するIPアドレス | `--underlay-interface` のIPv4アドレス |
| `--underlay-interface` | ノード間のオーバーレイ（Geneve）に使うインターフェース。`default_underlay_interface` に設定する | 現在の `default_underlay_interface`、未設定なら `br0` |
| `--dry-run` | 変更内容を表示するだけで、何も変更しない | 無効 |
| `--force` | 確認プロンプトを省略する | 無効 |

### 内部処理
処理は以下の順に実行し、各ステップ前に、変更対象ファイルを `*.bak` へ退避する。

1. 事前チェック
    - root で実行されていること、etcd と marmotd が稼働していること、`--underlay-interface` が存在し IPv4 アドレスを持つことを確認する。
    - etcd にクラスタ有効の情報がすでにある場合は、CLUSTER-ID を表示して正常終了する（再実行しても ID は変わらない）。
2. etcd の外部公開
    - `etcd.service` の `--listen-client-urls` と `--advertise-client-urls` を、`http://<etcd-ip>:2379` へ変更する。
      localhost からの接続を維持するため、`--listen-client-urls` は `http://127.0.0.1:2379,http://<etcd-ip>:2379` とする。
    - `systemctl daemon-reload` の後に etcd を再起動し、`etcdctl endpoint health` で起動を確認する。
3. `/etc/marmot/marmotd.json` の更新
    - `etcd_url` を `http://<etcd-ip>:2379` へ変更する。
    - `default_underlay_interface` に `--underlay-interface` の値を設定する。
    - 上記以外の項目は変更しない。
4. クラスタ情報の保存
    - CLUSTER-ID（UUID v4）を生成して、クラスタ有効フラグとともに etcd へ保存する。保存先のキーは未定（要検討）。
5. marmotd の再起動
    - `marmotd` を再起動する。起動時に、marmotd が OVN の設定（`ovn-encap-type=geneve`、`ovn-encap-ip`、`ovn-remote`）を
      `default_underlay_interface` から自動で行うため、`maadm` は OVS/OVN を直接変更しない。
    - 稼働中の VM は、marmotd と etcd の再起動では停止しない。ただし、再起動中は API が一時的に利用できない。
6. 結果表示
    - 作成した CLUSTER-ID と、他ノードが `join` するときに使うコマンド（`--master-node` の値）を表示する。

```bash
$ sudo maadm cluster init --etcd-ip 10.1.0.13 --underlay-interface br0
CLUSTER-ID: 6f1c0c0e-....
etcd: http://10.1.0.13:2379
他のノードで次のコマンドを実行してください:
  maadm cluster join 6f1c0c0e-.... --master-node 10.1.0.13
```

### エラー時の動作
| 状況 | 動作 |
|---|---|
| root 権限がない、etcd / marmotd が停止している | エラーを表示して、何も変更せずに終了する |
| 指定したインターフェースが存在しない、IPv4 アドレスがない | エラーを表示して、何も変更せずに終了する |
| 手順2以降で失敗した | `*.bak` から `etcd.service` と `marmotd.json` を復元し、etcd と marmotd を再起動して、元のシングルノード構成へ戻す |
| すでにクラスタが有効 | CLUSTER-ID を表示して、正常終了する（変更しない） |

### 確認方法
クラスタが有効なら、`mactl cluster` で CLUSTER-ID とノード一覧が表示される。
クラスタが無効なら、`mactl cluster` はエラーを表示する。

```bash
$ mactl cluster
ID: CLUSTER-ID
NODE             HOSTID     IP              CAP_CPU CAP_MEM(MB)  TOTAL RUNNING STOPPED     VCPU  MEM(MB)    VNETS AGE
hv3              ad02fe16   10.1.0.13            32     128716      0       0       0       0    0              0 0d
```


## ノードの marmotクラスタへの編入

クラスタへ編入するノード上で、`maadm cluster join` を root 権限で実行する。

```bash
sudo maadm cluster join CLUSTER-UUID --master-node 10.1.0.13 --underlay-interface br0
```

### 前提条件
- marmot と etcd がインストール済みで、シングルノードとして構成されていること。
- マスターノードで `maadm cluster init` が完了し、CLUSTER-ID が分かっていること。
- 実行ノードが、すでにクラスタのメンバーではないこと。
- `/etc/hostid` が設定され、HOSTID がクラスタ内の他のノードと重複しないこと（[HOWTO-install-marmot-cluster.md](HOWTO-install-marmot-cluster.md) 参照）。
- `node_name`（`marmotd.json`）が、クラスタ内の他のノードと重複しないこと。
- 実行ノードが、マスターノードの以下のポートへ到達できること。
    - etcd: TCP 2379
    - OVN SB DB: TCP 6642（OVN を利用する場合）
- ノード間で、Geneve（UDP 6081）が通過できること。
- 実行ノードに、VM とネットワークなどの管理対象が存在しないこと（ローカル etcd のデータはクラスタへ引き継がないため）。

### オプション（案）
| オプション | 説明 | 既定値 |
|---|---|---|
| `--master-node` | マスターノードのIPアドレス（必須）。etcd のポートは 2379 | なし |
| `--underlay-interface` | ノード間のオーバーレイ（Geneve）に使うインターフェース。`default_underlay_interface` に設定する | `br0`（共用インターフェース） |
| `--dry-run` | 変更内容を表示するだけで、何も変更しない | 無効 |
| `--force` | 確認プロンプトを省略する | 無効 |

### 内部処理
処理は以下の順に実行し、各ステップ前に、変更対象ファイルを `*.bak` へ退避する。

1. 事前チェック
    - root で実行されていること、`--underlay-interface` が存在し IPv4 アドレスを持つこと、すでにクラスタのメンバーでないことを確認する。
    - ローカルの etcd に VM などの管理対象がある場合は、エラーを表示して終了する。
2. マスターノードへの問い合わせ
    - `--master-node` の etcd へ接続して、CLUSTER-ID が一致することを確認する。
        - 一致する場合は、次のステップへ進む。
        - 一致しない、またはマスターへ接続できない場合は、エラーメッセージを表示して、何も変更せずに終了する。
    - 登録済みのノードと、`node_name` および HOSTID が重複していないことを確認する。重複している場合は、エラーを表示して終了する。
3. ローカル構成の変更
    - ローカルの marmotd を停止する。
    - ローカルの etcd を停止して、起動しないように無効化する（`systemctl disable --now etcd`）。クラスタでは、マスターの etcd だけを使う。
    - `/etc/marmot/marmotd.json` を更新する。それ以外の項目は変更しない。
        - `etcd_url`: マスターノードの etcd のURL（例: `http://10.1.0.13:2379`）。
        - `default_underlay_interface`: `--underlay-interface` の値。
4. marmotd の起動
    - marmotd を起動する。起動時に、marmotd が OVN の設定（`ovn-encap-type=geneve`、`ovn-encap-ip`、`ovn-remote`）を、
      `default_underlay_interface` とクラスタのホスト情報から自動で行うため、`maadm` は OVS/OVN を直接変更しない。
    - marmotd は、ノード情報（`node_name`、HOSTID、IPアドレス、CPU、メモリ）を etcd へ登録する。
5. 結果の確認と表示
    - マスターの etcd に自ノードが登録されたことを確認して、CLUSTER-ID とクラスタのノード一覧を表示する。
    - 登録を確認できない場合は、エラーを表示して、処理を終了する（ロールバックの対象とする）。

### エラー時の動作
| 状況 | 動作 |
|---|---|
| root 権限がない、インターフェースが不正、すでにメンバーである | エラーを表示して、何も変更せずに終了する |
| CLUSTER-ID の不一致、マスターへ接続できない、`node_name` / HOSTID の重複 | エラーを表示して、何も変更せずに終了する |
| 手順3以降で失敗した | `*.bak` から `marmotd.json` を復元し、ローカル etcd を有効化して起動し、marmotd を再起動して、元のシングルノード構成へ戻す |

### 確認方法
`mactl cluster` を実行すると、以下のように、クラスタメンバーのノードが表示される。
Geneve トンネルの状態は、[HOWTO-trableshort-geneve](HOWTO-trableshort-geneve) の手順で確認する。

```bash
$ mactl cluster
ID: CLUSTER-ID
NODE             HOSTID     IP              CAP_CPU CAP_MEM(MB)  TOTAL RUNNING STOPPED     VCPU  MEM(MB)    VNETS AGE
hv3              ad02fe16   10.1.0.13            32     128716      8       8       0       22    30720        8 56d
hv4              24e97535   10.1.0.14            32     128715      7       7       0       14    16384        8 56d
```


## marmotクラスタから、ノードを離脱

クラスタから離脱するノード上で、`maadm cluster leave` を root 権限で実行することで、クラスタのメンバーから離脱する。
離脱するノード上の VM は、シャットダウンして削除する。離脱後のノードは、元のシングルノードの構成に戻る。

```bash
sudo maadm cluster leave CLUSTER-UUID
```

### 前提条件
- 実行ノードが、`CLUSTER-UUID` のクラスタのメンバーであること。
- マスターノード（etcd が稼働するノード）自身は、離脱できない。マスターを外す場合は、先に他のノードを離脱させる。
- 離脱するノードで動作している VM は、削除されて復元できない。必要なデータは、事前にバックアップすること。

### オプション（案）
| オプション | 説明 | 既定値 |
|---|---|---|
| `--force` | 確認プロンプトを省略する | 無効 |
| `--dry-run` | 削除される VM とその他の変更内容を表示するだけで、何も変更しない | 無効 |

実行すると、削除される VM の一覧を表示して、`--force` がなければ、続行してよいか確認する。

### 内部処理
処理は以下の順に実行し、各ステップ前に、変更対象ファイルを `*.bak` へ退避する。

1. 事前チェック
    - root で実行されていること、`CLUSTER-UUID` がクラスタの CLUSTER-ID と一致すること、自ノードがマスターでないことを確認する。
2. VM の停止と削除
    - 自ノード上の VM をシャットダウンして削除する。シャットダウンが一定時間（既定 120 秒）で完了しない場合は、強制停止する。
    - いずれかの VM の削除に失敗した場合は、エラーを表示して処理を中断する。この時点では、クラスタのメンバーのままである。
3. クラスタ上の自ノード情報の削除
    - etcd の自ノードのホスト情報（`/marmot/hoststatus` 配下の自ノードのエントリ）を削除する。
    - ホスト情報が残ると、スケジューラのリーダー選出（HOSTID が最小のノード）や、`mactl cluster` の表示に影響する。
4. ローカル構成の復元
    - marmotd を停止する。
    - `/etc/marmot/marmotd.json` の `etcd_url` を、ローカルの etcd（`http://127.0.0.1:2379`）に戻す。それ以外の項目は変更しない。
    - ローカルの etcd を有効化して起動する（`systemctl enable --now etcd`）。
5. OVS/OVN のクリーンアップ
    - `ovn-remote` を削除する。ローカルの marmotd が起動時に、自ノードの設定に合わせて再設定する。
    - OVN の SB DB から自ノードの chassis を削除する（`ovn-sbctl chassis-del <system-id>`）。
    - 他ノードの Geneve トンネルポートは、自ノードの ovn-controller の停止に伴って、OVN が削除する。残った場合は、手動で削除する。
6. marmotd の起動と結果の表示
    - marmotd を起動する。シングルノードとして起動したことを表示する。

### 離脱後の状態
- 離脱したノードは、ローカルの etcd だけを使うシングルノードとして動作し、クラスタの情報は参照できない。
- クラスタに残ったノードの `mactl cluster` から、離脱したノードは消える。
- 再びクラスタへ参加する場合は、`maadm cluster join` を実行する。

### エラー時の動作
| 状況 | 動作 |
|---|---|
| root 権限がない、CLUSTER-ID の不一致、自ノードがマスター、クラスタのメンバーでない | エラーを表示して、何も変更せずに終了する |
| マスターの etcd へ接続できない | エラーを表示して、何も変更せずに終了する。クラスタの情報を削除できないため、離脱は行わない |
| VM の削除に失敗した | エラーを表示して、処理を中断する。再実行で、残りの VM から続行できる |
| 手順3以降で失敗した | エラーの内容と未完了の手順を表示して終了する。VM は削除済みのため、再実行で、未完了の手順から続行する |

### 確認方法
離脱したノードで、`mactl cluster` を実行すると、シングルノードのクラスタ情報なしの状態（クラスタ無効）になっている。
残りのノードで `mactl cluster` を実行すると、離脱したノードが表示されない。
