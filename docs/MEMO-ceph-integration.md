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
インストーラーは「Marmot が Ceph を利用できる状態の確認」までを責務とし、
Ceph クラスタ自体の構築と運用には関与しない。

### 必須関与（実施する）
- marmotd.json への Ceph 連携設定反映（ceph_enabled、接続先、認証情報、class→CRUSH rule/pool 対応）。
- Ceph 連携の有効/無効の明示切り替え。
- ceph_enabled=true の場合、Ceph CLI 主経路を前提に、marmot 実行ノードへ ceph-common を導入する。
- ceph_enabled=true の場合、ceph コマンドと rbd コマンドの存在確認（例: ceph --version, rbd --version）を事前チェックする。
- ceph_enabled=true の場合、ceph_key_file の存在、参照権限、読み取り可否を事前チェックする。
- インストール時の接続検証:
  - MON 到達性
  - 認証可否
  - ceph_crush_rule_by_class に定義された CRUSH rule の存在確認
  - ceph_pool_by_class に定義された pool の存在確認
- 検証失敗時のエラーレポート（修正対象と次アクションを提示）。

### 任意関与（オプション）
- 検証/開発用途の単一ノード Ceph ブートストラップ補助。
- ただし本体インストールフローと分離し、明示フラグ（例: --with-ceph-bootstrap）時のみ実行する。
- パッケージ導入をスキップする明示フラグ（例: --skip-ceph-common-install）を提供してもよい。

### 非関与（実施しない）
- 本番向け Ceph クラスタ構築（OSD 配置、CRUSH、レプリカ設計）。
- 容量計画、性能設計、障害対応運用。
- Ceph 自体のアップグレードとライフサイクル管理。

### MVP の既定動作
- 既定は「Ceph 非関与 + 接続検証のみ」。
- Ceph 未導入でも Marmot の通常インストールは完了できる。
- Ceph 有効時に検証が失敗した場合は非0で終了し、原因と修正手順を表示する。

## 4. 導入方針（段階導入）
### Phase 1: ボリューム管理のみ
- 対象: Volume の Create/Get/List/Delete。
- サーバーアタッチは対象外。
- 目的: Ceph 連携の接続性、永続化、異常系の安定化。

### Phase 2: サーバーアタッチ連携
- 対象: Server 作成時/更新時の Ceph Volume アタッチ。
- 目的: VM 起動パスに Ceph バックエンドを統合。

この順序により、既存 iSCSI 経路の改修を先行しなくても、まずは Ceph ボリューム管理の機能価値を早期に提供できる。

## 5. API 拡張方針
既存の Volume Spec に対して、後方互換を維持しつつ最小拡張する。

### 5.1 既存仕様（現状）
- type: qcow2 / lvm
- kind: os / data
- iscsi, iscsiTargetIqn など

### 5.2 追加仕様（案）
- type に ceph を追加。
- Ceph 連携で API から指定する追加フィールドは storageClass のみとする。
- storageClass は以下の値を受け付ける。
  - hdd
  - ssd
  - nvme
- storageClass は Ceph の CRUSH rule に対応する分類子として扱う。
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
  - size 必須（1GB 以上）
  - storageClass 必須
  - storageClass は hdd / ssd / nvme のいずれか
  - kind=os のときは、Phase 1 では拒否（Phase 2 以降で検討）
- type!=ceph の場合:
  - storageClass は無視または拒否（方針を統一する）

## 6. 設定設計（marmotd.json）
Ceph 接続設定を追加し、未設定時は Ceph 機能を無効化する。

```json
{
  "ceph_enabled": true,
  "ceph_monitors": ["10.1.4.11:6789", "10.1.4.12:6789", "10.1.4.13:6789"],
  "ceph_user": "client.marmot",
  "ceph_key_file": "/etc/ceph/marmot.client.key",
  "ceph_crush_rule_by_class": {
    "hdd": "rule-hdd",
    "ssd": "rule-ssd",
    "nvme": "rule-nvme"
  },
  "ceph_pool_by_class": {
    "hdd": "marmot-hdd",
    "ssd": "marmot-ssd",
    "nvme": "marmot-nvme"
  }
}
```

注意:
- 生パスワードではなく Ceph ユーザーキーを利用する。
- キー本体は設定ファイルに直接埋め込まず、ceph_key_file 参照を使う。
- 認証情報はログに出さない（マスク）。
- Ceph 操作は REST ではなく Ceph CLI を利用する前提とする。
- Ceph 側で pool / CRUSH rule を事前作成し、Marmot は ceph_crush_rule_by_class / ceph_pool_by_class の対応を参照して利用する。

## 7. 内部データモデル
Volume オブジェクトの spec/status に Ceph 情報を保持する。

### spec（宣言）
- storageClass

### status（実測）
- provider: "ceph"
- providerVolumeId: "<pool>/<image>"
- attachProtocol: "rbd"（Phase 2 で利用）
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

### 8.3 Ceph 側の作成単位
- storageClass に対応する CRUSH rule / pool の対応関係（marmotd.json の ceph_crush_rule_by_class / ceph_pool_by_class）を参照する。
- Ceph 側で storageClass に対応する CRUSH rule と pool を事前に作成する。
- Marmot は storageClass から既定の pool 名を決定し、その pool に対して Volume ID で一意な image を作成する。
- image 名は Volume ID を基準にして衝突しない形で固定する。
- その結果として得られる接続情報を用い、仮想サーバーからアタッチ可能にする。

### 8.4 storageClass と CRUSH rule の対応表
Marmot は storageClass を Ceph の配置方針に変換するだけにとどめ、
実際の CRUSH rule 名と pool 名のひも付けは marmotd.json で設定可能とする。

| storageClass | 想定するディスク種別 | CRUSH rule の例 | pool 名の例 |
| --- | --- | --- | --- |
| hdd | 回転ディスク | rule-hdd | marmot-hdd |
| ssd | SSD | rule-ssd | marmot-ssd |
| nvme | NVMe | rule-nvme | marmot-nvme |

運用上の注意:
- storageClass の追加・変更は、Ceph 側の CRUSH rule と pool の準備を先に完了してから行う。
- Marmot 側では、未対応の storageClass を受理せずエラーにする。
- CRUSH rule 名と pool 名は marmotd.json の ceph_crush_rule_by_class / ceph_pool_by_class で管理し、storageClass との対応関係自体は曖昧にしない。
- Marmot は pool や CRUSH rule を新規作成しない。

## 9. 処理フロー
### 9.1 Create(type=ceph)
1. API 受理、基本バリデーション。
2. DB に PENDING 登録。
3. コントローラーが PROVISIONING へ更新。
4. storageClass に対応する事前作成済み pool を選択する。
5. Ceph CLI を用いて、Ceph に Volume ID ベースの RBD イメージを作成する。
6. 仮想サーバー専用の Ceph キーを発行し、対象 pool への権限を設定する。
7. 成功時に status.provider 情報を反映し AVAILABLE。
8. 失敗時は ERROR とし、message に原因を格納。

### 9.2 Delete(type=ceph)
1. DELETING へ遷移。
2. 対象 image 用のアクセス権限と専用キーを削除する。
3. Volume ID ベースの RBD イメージを削除する。
4. 成功または not found の場合、DB レコード削除。
5. その他は ERROR。

## 9.3 Ceph CLI コマンド列（実装ガイド）
この節は、Marmot が Ceph CLI を利用する場合の最小実装手順を示す。
前提として、pool / CRUSH rule は Ceph 側で事前作成済みであること。

### 入力パラメータ
- POOL: ceph_pool_by_class から選択した pool 名
- IMAGE: Volume ID ベースの image 名（例: vol-abcde）
- SIZE_GB: ボリュームサイズ（GB）
- CLIENT_USER: 仮想サーバー専用に発行する Ceph クライアント名（例: client.vm-<serverId>-<volumeId>）
- MON_HOSTS: Ceph monitor の接続先

### 1. RBD image 作成
```bash
rbd -m "${MON_HOSTS}" create "${POOL}/${IMAGE}" --size "${SIZE_GB}G"
```

### 2. pool 単位アクセス権限の付与
CephX の capability は対象 pool に限定し、必要最小限の権限にする。

```bash
ceph -m "${MON_HOSTS}" auth get-or-create "${CLIENT_USER}" \
  mon 'allow r' \
  osd "allow rwx pool=${POOL}" \
  -o /tmp/${CLIENT_USER}.keyring
```

運用メモ:
- image 単位まで厳密に絞る場合は、運用ポリシーに沿って namespace 分離または追加制約を組み合わせる。
- 生成した keyring の取り扱いは最小権限で管理し、配布後は不要ファイルを削除する。

### 3. 接続情報取得
仮想サーバーへ渡すための認証情報を取得する。

```bash
ceph -m "${MON_HOSTS}" auth get-key "${CLIENT_USER}"
```

### 4. Delete 時の権限削除
```bash
ceph -m "${MON_HOSTS}" auth del "${CLIENT_USER}"
```

### 5. Delete 時の image 削除
```bash
rbd -m "${MON_HOSTS}" rm "${POOL}/${IMAGE}"
```

### 6. 冪等性を考慮した補助確認
```bash
rbd -m "${MON_HOSTS}" info "${POOL}/${IMAGE}"
ceph -m "${MON_HOSTS}" auth get "${CLIENT_USER}"
```

実装メモ:
- 上記コマンドは command.go 等でラップし、終了コードと標準エラーを統一的にエラー変換する。
- not found は削除系で成功寄りに扱い、状態遷移を継続する。

## 10. エラーハンドリング方針
以下を最低限の判定対象とする。
- Ceph 無効設定（ceph_enabled=false）。
- Ceph 接続失敗（MON 到達不可、認証失敗）。
- storageClass に対応する CRUSH rule / pool が存在しない。
- 空き容量不足。
- 同名イメージ衝突。
- 専用キー発行失敗、または pool 単位権限設定失敗。

実装ルール:
- API レイヤー: 入力不正は 4xx。
- 実行時失敗: status=ERROR と message 保存。
- not found 削除: 冪等扱いで成功寄りに処理。

## 11. セキュリティ
- Ceph 認証は最小権限ユーザーを前提。
- 秘密情報は設定ファイル権限を 600 相当に制限。
- ログに認証情報を出力しない。
- CI は GitHub Secrets を使用し、ログマスクを必須化。
- 個別利用では、仮想サーバーごとに専用の Ceph キーを発行し、対象 pool のみにアクセスできる権限に限定する。
- そのキーは、原則として対象 pool 以外にはアクセスできない最小権限設定とする。
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
- type=ceph の Volume が作成可能で AVAILABLE になる。
- type=ceph の Volume が削除可能で実体も削除される。
- 主要異常系で ERROR へ遷移し、原因文字列を確認できる。
- 既存 type=qcow2/lvm の挙動が回帰しない。

## 14. 実装タスク（初版）
1. OpenAPI 更新（api/marmot-api-v1.yaml の VolSpec 拡張）。
2. 生成コード更新（api/marmot-api-v1.go は直接編集しない）。
3. marmotd config に Ceph 設定追加。
4. pkg/ceph 実装。
5. Volume 作成/削除フローに ceph 分岐追加。
6. 単体テスト、結合テスト追加。
7. ドキュメントと運用手順更新。

## 15. 未決事項
- なし

## 16. 決定事項
- kind=os を Ceph で許可する時期は Phase 2 以降とする。
- その具体化は、ライブマイグレーション実装と同時に検討・推進する。
- Ceph のアクセス権限は、当面 pool 単位での付与を許容する。


