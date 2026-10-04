# RHEL 互換 Linux クラウドイメージ対応メモ

Issue: [#622 RHEL互換Linux のクラウドイメージに対応する](https://github.com/takara9/marmot/issues/622)

## 目的

Rocky Linux、AlmaLinux などの RHEL 互換ディストリビューションが提供するクラウドイメージを marmot に登録し、VM の作成・起動・初期設定まで行えるようにする。

クラウドイメージの URL を登録できるだけでは対応完了としない。OS の識別、ダウンロード後のイメージ加工、起動時のボリューム設定、cloud-init のユーザー設定、VM 起動後の接続までを一連の動作として確認する。

## 現状

- `os_images` の設定には `name`、`url`、`osName`、`osVersion` を指定できる。起動時の初期イメージ登録もこの情報を利用する。
- `validateImageOSSpec` は既に `rockey` のバージョン `8` と `9` を許可している。一方、表記が `rockey` であり、一般的な名称 `rocky` とは異なる。既存データとの互換性を確認せずに値を置換しないこと。
- `resolveImageOSModuleFromSpec` と `resolveServerImageModuleFromOS` が対応する OS は現在 Ubuntu と Alpine であり、検証を通る `rockey` も起動用モジュールには解決されない。
- VM の OS variant から OS 名とバージョンを推定する `deriveOSFromVariant` にも Rocky の分岐はない。
- ダウンロードイメージの共通カスタマイズ処理は Ubuntu 向け処理を呼び出す。この処理は `ubuntu` の root パスワード、Ubuntu 固有の SSH 設定、netplan 設定を書き込むため、RHEL 互換イメージにそのまま適用してはならない。
- cloud-init の user-data 生成は OS 共通の形式を基本としているが、デフォルトユーザー、SSH サービス名、ネットワーク設定、および各 upstream イメージでの cloud-init の有効化状態は個別に検証する必要がある。
- リポジトリ内には Rocky Linux 8/9 のイメージ候補が記載されているが、AlmaLinux の登録・起動モジュールはまだ定義されていない。

## 実装前に決めること

1. **対象ディストリビューションとバージョン**
   - 対象ディストリビューションは Rocky Linux、AlmaLinux の両方とする。
   - バージョンは `osVersion` にセットされた値（`8`、`9` など）で判定する。
   - 初回対応は Rocky Linux 9 に限定する。AlmaLinux および他バージョンへの対応は、初回対応で確認した内容をもとに別途進める。
   - 初回対応（Rocky Linux 9）のテスト用 upstream cloud image URL は以下とする。`latest` 参照のため、イメージ内容の更新・再現性の扱いは別途確認する。
     - `https://download.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud.latest.x86_64.qcow2`
   - CPU アーキテクチャ、および AlmaLinux を含む他イメージで採用する upstream cloud image は別途確認する。
2. **OS 名と variant の互換性**
   - 新規設定で使う `osName` の正式表記を決める。
   - 既存の `rockey` 値や `rockey8` / `rockey9` 形式の variant が保存済みかを調べ、読み込み時の互換方針を定める。
   - Rocky と AlmaLinux は別の OS 名として扱い、名前だけで同じイメージや同一バージョンとみなさない。
   - upstream イメージの URL は、Ubuntu と同様に marmot のマニフェスト、または `marmotd.json` に指定された値（`os_images` の `url`）から取得する。OS 名に応じてコード側に URL をハードコードする新たな取得経路は設けない。
3. **初期ユーザーと初期化の責務**
   - 初期ユーザーと初期化の責務は、これまでの Ubuntu と同様の方針を踏襲する。
   - 初期ユーザーは upstream のデフォルトユーザーに合わせ、Rocky Linux は `rocky`、AlmaLinux は `almalinux` とする。
   - SSH 公開鍵の反映方法、パスワード設定の要否は upstream の仕様で確認する。
   - cloud-init が処理する設定と、イメージ登録時のカスタマイズ処理の責務の分け方は、Ubuntu と同じ考え方を適用する。

初回対応は、確認済みの最小バージョン範囲に限定する。未検証のメジャーバージョンを、同じファミリーという理由だけで自動的にサポート対象に含めない。

## 対応の進め方

### Phase 1: 対象イメージと識別規約の確定

- 上記の決定事項を埋め、ディストリビューション／バージョン／アーキテクチャごとの対応表を作る。
- 各 upstream イメージの形式、cloud-init の有無と設定、初期ユーザー、SSH、ネットワークデバイス名を確認する。
- marmot の `osName`、`osVersion`、イメージ名、`osVariant` の対応を定義し、既存値の読み込み互換性も明記する。
- `os_images` に登録する URL とバージョンが対応表と一致することをレビューできる形にする。

### Phase 2: OS 解決とイメージ準備

- `validateImageOSSpec` の許可値を、Phase 1 で合意した正式な OS 名・バージョンに合わせる。
- イメージ OS モジュールとサーバーイメージモジュールの両方に各対象 OS の解決を追加する。
- OS variant からの推定が必要な経路には対応する変換を加え、明示的な `osName` / `osVersion` と矛盾しないことを確認する。
- Rocky / AlmaLinux 向けのイメージ加工要否を upstream イメージごとに判断する。加工する場合は、Ubuntu 専用処理を流用せず、ファイル配置やパッケージ管理系を確認した専用処理にする。
- イメージ複製・ノード間同期でも `osName` と `osVersion` が保持されることを確認する。

### Phase 3: VM 初期設定と起動

- cloud-init に渡すユーザー名・SSH 公開鍵・パスワード設定が各対象イメージで適用されることを確認する。
- 既存の Linux 共通設定とディストリビューション固有設定を切り分ける。特にネットワーク設定ファイルやインターフェース名を固定しない。
- 起動用モジュールが、OS 種別に応じたブートボリューム準備を選択することを確認する。
- 既存 Ubuntu / Alpine の動作を変えずに、各対象イメージから VM を作成・起動・SSH 接続できることを確認する。

### Phase 4: 運用手順と対応範囲の明示

- `os_images` への登録例、採用した upstream イメージ、対応バージョン、初期ユーザー、更新時の確認手順を文書化する。
- サポート対象外のバージョンやイメージ形式、および upstream イメージ更新による再検証条件を明記する。
- イメージ配布元の利用条件・ライセンスを確認し、イメージそのものを marmot が再配布する場合は別途承認を得る。

## テストと完了条件

- OS 名・バージョンの検証テストで、合意した値のみを受け入れ、未対応値を明示的なエラーにする。
- イメージ OS モジュール、サーバーイメージモジュール、variant 推定の各テストで Rocky / AlmaLinux が正しいモジュールへ解決される。
- 既存の `rockey` 値を維持する場合、その読み込み・モジュール解決の互換テストを追加する。
- イメージ加工を行う場合、生成される `virt-customize` 引数や設定ファイルが対象 OS に適合し、Ubuntu 固有の設定を混入させないことをテストする。
- user-data のテストで各 OS の初期ユーザー、SSH 公開鍵、必要な認証設定を確認する。
- 対応する実イメージを用いた統合確認で、イメージ登録、VM 作成、起動、ネットワーク疎通、SSH ログインまで成功する。
- Ubuntu / Alpine の既存テストおよび動作に回帰がない。

## 対象外

- cloud image を提供する upstream の保守や、サポート対象外バージョンの動作保証。
- Rocky Linux と AlmaLinux のバイナリ互換性・ライフサイクルを同一とみなすこと。
- Issue で明示的に合意されていない他の RPM 系 OS（例: Oracle Linux、CentOS Stream）への一括対応。
- cloud-init が利用できないイメージの無人セットアップ方式。必要になった場合は別途方式とスコープを決める。
