# Marmot + Ceph 統合設計メモ（実装向け）

## 1. 目的
Marmot の Volume 管理を拡張し、Ceph RBD をバックエンドとして利用できるようにする。
最終的に、Marmot API から作成した ceph ボリュームを仮想サーバーへアタッチし、既存ボリュームと同等にライフサイクル管理できる状態を目指す。

## 2. 非目的
- Ceph クラスタの設計（ノード数、レプリカ数、CRUSH 設計）自体は Marmot の責務に含めない。
- Ceph インストール自動化の完全実装は本設計の主目的ではない（必要最小限の手順提供にとどめる）。
- 既存 LVM/qcow2 実装の置換は行わない。共存を前提とする。

## 3. 責任分界点
### Marmot の責務
- Volume API の受理、バリデーション、状態遷移管理。
- Ceph CLI を通じた RBD の作成、削除、情報取得。
- Ceph 接続情報の読み込み（marmotd.json）と、必要な識別子の DB 保存。
- VM アタッチに必要な接続情報の生成と引き渡し。

### Ceph 側の責務
- OSD/MON/MGR クラスタ運用。
- プール、CRUSH ルール、容量設計、耐障害性。
- 認証ユーザー管理（最小権限の Ceph ユーザー発行）。
- storageClass に対応する pool / CRUSH ルールの事前作成。

## 3.1 インストーラーの関与境界
- インストーラーは「Marmot が Ceph を利用できる状態の確認」までを責務とし、Ceph クラスタ自体の構築と運用には関与しない。
- marmot 実行ノードへ ceph-common を導入する。

### Marmotの実施する事項
- marmotd.json への Ceph 連携設定反映（ceph_enabled、接続先、認証情報、class→pool 対応）。
- Ceph 連携の有効/無効の明示切り替え。
- ceph_enabled=true の場合、ceph コマンドと rbd コマンドの存在確認（例: ceph --version, rbd --version）を事前チェックする。
- ceph_enabled=true の場合、ceph_key_file の存在、参照権限、読み取り可否を事前チェックする。
- インストール時の接続検証:
  - MON 到達性
  - 認証可否
  - ceph_pool_by_class に定義された pool の存在確認
- 検証失敗時のエラーレポート（修正対象と次アクションを提示）。

### Marmotが実施しない事項
- 本番向け Ceph クラスタ構築（OSD 配置、CRUSH、レプリカ設計）。
- 容量計画、性能設計、障害対応運用。
- Ceph 自体のアップグレードとライフサイクル管理。

## 4. 導入方針（段階導入）
### 本Phase: ボリューム管理 + サーバーアタッチ連携
- 対象: Volume の Create/Get/List/Delete。
- 対象: Server 作成時/更新時の Ceph Volume アタッチ。
- 方針: iSCSI/LVM/QCOW2 と同様に、VM 作成時の libvirt XML に Ceph ディスク定義を含める。
- 目的: Ceph 連携の接続性、永続化、異常系の安定化、および VM 起動パスへの統合。

## 5. API 拡張方針
既存の Volume Spec に対して、後方互換を維持しつつ最小拡張する。

### 5.1 既存仕様
- type: qcow2 / lvm
- kind: os / data
- iscsi, iscsiTargetIqn など
- size GB単位

### 5.2 追加仕様
- type に ceph を追加。
- Ceph 連携で API から指定する追加フィールドは storageClass のみとする。
- storageClass は、marmotd.json の ceph_pool_by_class[].storageClassに登録があれば受け付ける。
- storageClass は Ceph の pool を選択する分類子として扱う。
- API は Ceph の pool 名、image 名、cluster 名、feature 名を受け取らない。

### 5.3 API リクエスト例
```yaml
apiVersion: v1
kind: Volume
metadata:
  name: data-ceph-01
spec:
  kind: data
  type: ceph
  size: 20
  storageClass: ssd
```

### 5.4 バリデーション
- type=ceph の場合:
  - size 省略時は 1GBをセットする。
  - storageClass 省略時は、ssd をセットする。
  - /etc/marmot/marmotd.jsonのceph_pool_by_class[].storageClass に登録が無い場合はエラーとする。
  - kind=os のときは、本Phaseでは拒否（将来フェーズで検討）、kind 省略時は、dataとする。
- type!=ceph の場合:
  - storageClass は無視する。
- type省略時は qcow2 をセットする。

## 6. 設定設計（marmotd.json）
Ceph 接続設定を追加し、未設定時は Ceph 機能を無効化する。

```json
{
  "ceph_enabled": true,
  "ceph_monitors": ["10.1.4.11:6789", "10.1.4.12:6789", "10.1.4.13:6789"],
  "ceph_user": "client.marmot",
  "ceph_key_file": "/etc/marmot/ceph/client.key",
  "ceph_pool_by_class": [
    { "storageClass": "hdd", "pool": "marmot-hdd" },
    { "storageClass": "ssd", "pool": "marmot-ssd" },
    { "storageClass": "nvme", "pool": "marmot-nvme" }
  ]
}
```

注意:
- 生パスワードではなく Ceph ユーザーキーを利用する。
- キー本体は設定ファイルに直接埋め込まず、ceph_key_file を libvirt secret 登録元ファイルとして扱う。
- ceph_key_file の参照元は /etc/marmot/marmotd.json の設定値とし、実ファイルから鍵を取得する。
- ceph_user / ceph_key_file は、Marmot が固定で利用する Ceph 認証情報と、その libvirt secret 登録元として扱う。
- 認証情報はログに出さない（マスク）。
- Ceph 操作は REST ではなく Ceph CLI を利用する前提とする。
- Ceph 側で pool を事前作成し、Marmot は ceph_pool_by_class の対応を参照して利用する。

## 7. 内部データモデル
Volume オブジェクトの spec/status に Ceph 情報を保持する。

### spec（宣言）
- storageClass

### status（実測）
- provider: "ceph"
- providerVolumeId: "<pool>/<image>"
- attachProtocol: "rbd"（本Phaseで利用）
- message: エラー詳細

## 8. コンポーネント設計
### 8.1 新規パッケージ
- pkg/ceph
  - client.go: 接続初期化、共通エラー変換
  - volume.go: Create/Delete/Stat/List
  - mapper.go: API Spec と Ceph パラメータ変換
  - command.go: Ceph CLI 実行ラッパー
  - fake_client.go: 単体テスト用スタブ

実装方針:
- Ceph 操作の主経路は Ceph CLI とする。
- Ceph 操作の実装は Ceph CLI 経路に統一する。

### 8.2 既存連携ポイント
- Volume 作成フローに type=ceph 分岐を追加。
- Volume 削除フローに ceph 実体削除を追加。
- コントローラーの状態遷移（PENDING -> PROVISIONING -> AVAILABLE/ERROR）は既存パターンに合わせる。

### 8.2.1 VM 作成時の libvirt XML への Ceph 専用ディスク定義（本Phase）
本Phaseでは、type=ceph の data volume をサーバーへアタッチする際、
libvirt の domain XML に RBD 用の network disk 定義を追加する。

想定する XML 要素:
- disk type='network' device='disk'
- driver name='qemu' type='raw'
- source protocol='rbd' name='<pool>/<image>'
- source host (monitor 一覧)
- auth username='<ceph_user>'
- secret type='ceph' usage='<libvirt secret usage>'
- target dev='vdX' bus='virtio'

XML 例:
```xml
<disk type='network' device='disk'>
  <driver name='qemu' type='raw' cache='none' io='native'/>
  <source protocol='rbd' name='marmot-ssd/vol-abcde'>
    <host name='10.1.4.11' port='6789'/>
    <host name='10.1.4.12' port='6789'/>
    <host name='10.1.4.13' port='6789'/>
  </source>
  <auth username='client.marmot'>
    <secret type='ceph' usage='marmot-ceph-client'/>
  </auth>
  <target dev='vdb' bus='virtio'/>
</disk>
```

実装上の取り扱い:
- source の name は status.providerVolumeId（<pool>/<image>）を利用する。
- host は ceph_monitors を展開して複数指定できるようにする。
- auth username は marmotd.json の ceph_user を使用する。
- 認証キー本体は XML へ埋め込まず、libvirt secret 参照で渡す。
- Ceph ディスク追加時も既存の vdX 採番規則と競合しないことを保証する。
- XML 生成失敗時は Server 作成を中断し、status.message に原因を格納する。

### 8.3 Ceph 側の作成単位
- storageClass に対応する pool の対応関係（marmotd.json の ceph_pool_by_class）を参照する。
- Ceph 側で storageClass に対応する pool を事前に作成する。
- ceph_user に対して、ceph_pool_by_class で利用する対象 pool 群へのアクセス権を1回でまとめて定義しておく。
- Marmot は storageClass から既定の pool 名を決定し、その pool に対して Volume ID で一意な image を作成する。
- image 名は Volume ID を基準にして衝突しない形で固定する。
- その結果として得られる接続情報を用い、仮想サーバーからアタッチ可能にする。

### 8.4 storageClass と pool の対応表
Marmot は storageClass を Ceph の pool 選択に変換し、
pool 名のひも付けは marmotd.json で設定可能とする。

| storageClass | 想定するディスク種別 | pool 名の例 |
| --- | --- | --- |
| hdd | 回転ディスク | marmot-hdd |
| ssd | SSD | marmot-ssd |
| nvme | NVMe | marmot-nvme |

運用上の注意:
- storageClass の追加・変更は、Ceph 側の pool 準備を先に完了してから行う。
- Marmot 側では、未対応の storageClass を受理せずエラーにする。
- pool 名は marmotd.json の ceph_pool_by_class で管理し、storageClass との対応関係自体は曖昧にしない。
- Marmot は pool を新規作成しない。

## 9. 処理フロー
### 9.1 Create(type=ceph)
1. API 受理、基本バリデーション。
2. DB に PENDING 登録。
3. コントローラーが PROVISIONING へ更新。
4. storageClass に対応する事前作成済み pool を選択する。
5. Ceph CLI を用いて、Ceph に Volume ID ベースの RBD イメージを作成する。
6. marmotd.json の ceph_user に、選択した pool へのアクセス権が付与されていることを利用する。
7. Server 側のアタッチ処理で利用するため、ceph_key_file を参照可能な状態で保持する。
8. 成功時に status.provider 情報を反映し AVAILABLE。
9. 失敗時は ERROR とし、message に原因を格納。

### 9.2 Server Create/Update 時の Ceph アタッチ
1. Server spec の storage に type=ceph の volume が含まれることを判定する。
2. 各 volume の status.providerVolumeId から <pool>/<image> を取得する。
3. ceph_monitors を展開し、libvirt XML の source host を組み立てる。
4. 仮想マシンのコントローラーが、/etc/marmot/marmotd.json で指定された ceph_key_file の内容から、当該 VM 用の libvirt secret を作成する。
5. ceph_user と作成した libvirt secret を参照して auth 要素を組み立てる。
6. iSCSI/LVM/QCOW2 と同様に、対象ディスクの XML 定義を domain XML の disks に追加する。
7. 生成した XML を libvirt に渡して define/start し、失敗時はエラー内容を status.message に格納する。

### 9.3 Delete(type=ceph)
1. DELETING へ遷移。
2. 固定ユーザー前提のため、対象 image 削除時に pool 権限や ceph_key_file 自体は削除しない。
3. Volume ID ベースの RBD イメージを削除する。
4. 成功または not found の場合、DB レコード削除。
5. その他は ERROR。

### 9.3.1 Server Delete 時の libvirt secret 削除
1. 仮想マシンのコントローラーが、VM 削除時に当該 VM に対応する libvirt secret を削除する。
2. ceph_key_file の元ファイルは削除しない。

## 9.4 Ceph CLI コマンド列（実装ガイド）
この節は、Marmot が Ceph CLI を利用する場合の最小実装手順を示す。
前提として、pool は Ceph 側で事前作成済みであること。

### 入力パラメータ
- POOL: ceph_pool_by_class から選択した pool 名
- IMAGE: Volume ID ベースの image 名（例: vol-abcde）
- SIZE_GB: ボリュームサイズ（GB）
- CEPH_USER: marmotd.json の ceph_user に設定した固定 Ceph ユーザー名
- KEY_FILE: marmotd.json の ceph_key_file に設定した libvirt secret 登録元ファイル
- MON_HOSTS: Ceph monitor の接続先

### 1. RBD image 作成
```bash
rbd -m "${MON_HOSTS}" create "${POOL}/${IMAGE}" --size "${SIZE_GB}G"
```

### 2. 対象 pool 群アクセス権限の一括付与
CephX の OSD capability は、固定ユーザーに対して ceph_pool_by_class で利用する対象 pool 群をまとめて1回で定義する。

```bash
ceph -m "${MON_HOSTS}" auth get-or-create "${CEPH_USER}" \
  mon 'allow r' \
  osd "allow rwx pool=${POOL_HDD}, allow rwx pool=${POOL_SSD}, allow rwx pool=${POOL_NVME}"
```

運用メモ:
- 対象 pool 群（hdd/ssd/nvme）の capability は1回で定義し、pool 名変更時のみ再定義する。
- ceph_key_file は固定ユーザー鍵の保管ファイルであり、libvirt secret 登録元として最小権限で管理する。

### 3. 接続情報取得
固定ユーザーの認証情報は ceph_key_file に保持し、その内容を libvirt secret へ登録する。

```bash
virsh secret-set-value --secret "${LIBVIRT_SECRET_UUID}" --base64 "$(base64 -w0 "${KEY_FILE}")"
```

### 4. Delete 時の権限削除
固定ユーザー前提のため、Volume 削除時に pool 権限は削除しない。

```bash
# no-op
```

### 4.1 VM 削除時の libvirt secret 削除
VM 削除時に、VM に対応する libvirt secret を削除する。

```bash
virsh secret-undefine "${LIBVIRT_SECRET_UUID}"
```

### 5. Delete 時の image 削除
```bash
rbd -m "${MON_HOSTS}" rm "${POOL}/${IMAGE}"
```

### 6. 冪等性を考慮した補助確認
```bash
rbd -m "${MON_HOSTS}" info "${POOL}/${IMAGE}"
ceph -m "${MON_HOSTS}" auth get "${CEPH_USER}"
```

実装メモ:
- 上記コマンドは command.go 等でラップし、終了コードと標準エラーを統一的にエラー変換する。
- not found は削除系で成功寄りに扱い、状態遷移を継続する。

## 10. エラーハンドリング方針
以下を最低限の判定対象とする。
- Ceph 無効設定（ceph_enabled=false）。
- Ceph 接続失敗（MON 到達不可、認証失敗）。
- storageClass に対応する pool が存在しない。
- 空き容量不足。
- 同名イメージ衝突。
- 固定ユーザーの認証情報不備、libvirt secret 作成/削除不備、または対象 pool 群（hdd/ssd/nvme）一括権限設定不備。

実装ルール:
- API レイヤー: 入力不正は 4xx。
- 実行時失敗: status=ERROR と message 保存。
- not found 削除: 冪等扱いで成功寄りに処理。

## 11. セキュリティ
- Ceph 認証は最小権限ユーザーを前提。
- 秘密情報は設定ファイル権限を 600 相当に制限。
- ログに認証情報を出力しない。
- CI は GitHub Secrets を使用し、ログマスクを必須化。
- ceph_user / ceph_key_file で指定した固定ユーザーは、ceph_pool_by_class で参照される対象 pool のみにアクセスできる最小権限設定とする。
- XML に記述する鍵は libvirt secret に保存し、仮想マシンのコントローラーが作成し、VM 削除時に削除する。VM ごとの keyring は作成しない。
- 他の仮想サーバーからの利用は、明示的に共有用途として設計しない限り許可しない。

## 12. テスト戦略
### 12.1 単体テスト
- pkg/ceph の client/volume の正常系、異常系。
- Volume コントローラーの type=ceph 分岐。

### 12.2 結合テスト
- Ceph シングルノード環境で Create/Delete を検証。
- 容量不足、プール不存在、認証失敗の異常系を検証。

### 12.3 CI
- Ceph 専用ランナーを利用（排他実行）。
- シークレット注入で接続情報を設定。

## 13. 受け入れ基準
- type=ceph の Volume が Create でき、AVAILABLE になる。
- type=ceph の Volume が Delete でき、実体も削除される。
- type=ceph の Volume をアタッチした仮想マシンから、当該ディスクを認識できる。
- 主要異常系で ERROR へ遷移し、原因文字列を確認できる。
- 既存 type=qcow2/lvm の挙動が回帰しない。

## 14. 実装タスク（初版）
1. OpenAPI 更新（api/marmot-api-v1.yaml の VolSpec 拡張）。
2. 生成コード更新（api/marmot-api-v1.go は直接編集しない）。
3. marmotd config に Ceph 設定追加。
4. pkg/ceph 実装。
5. Volume 作成/削除フローに ceph 分岐追加。
6. Server Create/Update フローに ceph アタッチ分岐を追加。
7. libvirt XML 生成で RBD 用 disk(type='network', protocol='rbd') 定義を追加。
8. libvirt secret の運用実装（参照名規約、作成/更新/削除、エラー時ハンドリング）を追加。
9. 単体テスト、結合テスト追加（Volume + Server アタッチ経路）。
10. ドキュメントと運用手順更新。

## 15. 未決事項
- なし

## 16. 決定事項
- kind=os を Ceph で許可する時期は将来フェーズとする。
- その具体化は、ライブマイグレーション実装と同時に検討・推進する。
- Ceph の OSD capability は、ceph_pool_by_class の hdd/ssd/nvme の3固定 pool 群をまとめて1回で定義する。


