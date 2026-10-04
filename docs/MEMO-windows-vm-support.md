# Windows VM 起動対応 設計メモ

## 背景・要望

marmot で Windows 仮想マシン（Windows Server 2022 および Windows Server 2025 を想定）を起動できるようにしたい。
最終的にはフルプロビジョニング対応（イメージ作成〜自動セットアップ）まで見据える。

## 現状のアーキテクチャ（Linux 専用の前提）

marmot の VM 起動パイプラインは Linux を前提に作られており、以下の点が Windows 非対応。

- **ディスクバス**: [pkg/virt/libvirtXml2.go](../pkg/virt/libvirtXml2.go) の `CreateDomainXML` でディスクの `Bus: "virtio"` が固定。
  Windows は標準で virtio ドライバを持たないため、ドライバ未組込のイメージでは初回起動不可。
- **プロビジョニング**: [pkg/marmotd/cloud-init.go](../pkg/marmotd/cloud-init.go) の `GenerateCloudInitISO` による
  cloud-init ISO 生成のみ。Windows は cloudbase-init や `unattend.xml`/sysprep が必要で、cloud-init は使えない。
- **イメージカスタマイズ**: `pkg/marmotd/image_os_module.go` の `resolveImageOSModuleFromSpec` は
  libguestfs/virt-customize ベース（`customizeQcowImageWithContext`）で、netplan・systemd など
  Linux ファイルシステム前提の処理を行う。
- **OS 種別管理**: `OsVariant` は `"ubuntu24.04"` のような文字列で管理され、Windows 用テンプレート/分岐は存在しない。
- **コンソール**: `mactl console` は ISA シリアルポート経由の PTY 接続のみ
  （[docs/... console 関連メモ参照](../memories/repo/mactl-console-libvirt-pty-path.md)）。
  Linux は cloud-init 完了後に SSH でログインする前提のデバッグ用途であり、GUI 出力先（`<video>` デバイス）が
  ドメイン定義に存在しない。Windows はシリアルポートに GUI やログイン画面を出力しないため、この仕組みだけでは
  画面を扱えない。
- SPICE の `<graphics>` 自体は既に定義されているが、`listen=127.0.0.1` 固定・パスワード未設定であり、
  ホスト外からの直接接続や認証は考慮されていない。

## 対応方針（全体ロードマップ）

範囲が広いため、3 フェーズに分割して進める（1 変更 = 1 PR の原則に従う）。

| フェーズ | 内容 | ステータス |
|---|---|---|
| Phase 1 | libvirt ドメイン生成の Windows 対応 + グラフィカルコンソール取得機能 | 未着手（提案合意待ち） |
| Phase 2 | Windows Server 2022 / 2025 のベースイメージのビルド手順整備（autounattend.xml + virtio-win ドライバ組込 + cloudbase-init 事前導入）と marmot イメージ登録への取り込み | 未着手 |
| Phase 3 | cloudbase-init 向けプロビジョニング ISO 生成（config-drive 形式）でユーザー/SSH キー/ホスト名を自動設定 | 未着手 |

対象 Windows バージョン: **Windows Server 2022 および Windows Server 2025**

## Phase 1 詳細スコープ（提案・未承認）

### 目的
1. `osVariant=windows2022` / `windows2025` 系 VM を libvirt ドメインとして正しく起動できるようにする。
2. `mactl` から SPICE 接続情報（ホスト IP:ポート、パスワード）を取得し、手元の `remote-viewer` で
   画面に接続できるようにする（Windows 系 VM 限定）。

### 非目的
- Windows ベースイメージ作成（autounattend.xml / virtio-win 組込）は含めない（Phase 2）。
- cloudbase-init による自動プロビジョニングは含めない（Phase 3）。
- Linux 系 VM の既存動作（シリアルコンソール / SPICE `127.0.0.1` 固定）は変更しない。

### 変更範囲（案）
- `pkg/virt/libvirtXml2.go`
  - `OsName == "windows"` の場合のみ分岐:
    - UEFI ファームウェア
    - ディスクバスを `virtio-scsi` に変更
    - TPM 2.0 デバイス（`backend type='emulator' version='2.0'` の `swtpm` バックエンドを追加する）
    - `clock` を `localtime` に変更
    - QXL ビデオデバイス追加
    - SPICE の `Listen` をノードのアドレスに変更 + `Passwd` を VM 生成時にランダム生成して設定
- `pkg/marmotd/image_os_module.go`
  - `resolveImageOSModuleFromSpec` に `"windows"` ケース追加（`customizeHandler` なし = virt-customize を実行せず素通し）
- `api/marmot-api-v1.yaml` + コード生成（`api/marmot-api-v1.go` は直接編集しない）
  - サーバーのグラフィカルコンソール接続情報を返すエンドポイント追加
    （例: `GET /server/{id}/console/graphical` → host, port, passwd）
- `pkg/marmotd/console.go`
  - 上記エンドポイントのハンドラ追加（稼働中ドメインの SPICE 設定を XML から取得）
- `cmd/mactl/cmd/console.go`
  - `mactl console --graphical` 等で接続情報を表示するサブコマンド/オプション追加

### 完了条件（案）
- `go build ./...` 成功
- `go test ./pkg/virt/... ./pkg/marmotd/... ./cmd/mactl/...` 成功
- `osVariant=windows2022` / `windows2025` 指定時に生成される XML が UEFI/TPM（`swtpm` エミュレータバックエンド）/QXL/SPICE
  （パスワード付き LAN Listen）になることをユニットテストで確認
- Linux 系 `osVariant` で生成される XML に差分がないことを確認
- `mactl console --graphical <windows-vm-name>` で host/port/passwd が表示されることを確認（手動確認）

## Phase 2 詳細スコープ（提案）

### 目的
利用者が正規に用意した Windows Server 2022 および Windows Server 2025 の各インストール ISO から、
marmot で利用するバージョン別ベースイメージを再現可能な手順で作成し、marmot のイメージとして登録できるようにする。

### 非目的
- Windows Server 2022 / 2025 インストール ISO の入手・配布、およびライセンスの調達・管理は行わない。
- libvirt ドメイン生成やグラフィカルコンソールの Windows 対応は含めない（Phase 1）。
- VM 起動時のユーザー、SSH キー、ホスト名などの自動設定は含めない（Phase 3）。
- VM ごとの config-drive 形式のプロビジョニング ISO 生成は含めない（Phase 3）。

### 変更範囲（案）
- Windows Server 2022 / 2025 それぞれのベースイメージ作成手順を整備する。
  - 各バージョンについて、利用者が用意した対応するインストール ISO を入力とする。
  - `autounattend.xml` を使った無人インストール手順を用意する。
  - 対象バージョンごとに virtio-win ドライバおよび cloudbase-init の対応状況を確認し、イメージに組み込む。
    初期設定と VM ごとの設定値の適用は Phase 3 で扱う。
  - 各バージョンのベースイメージを QCOW2 形式で出力する。
- バージョンを識別できる形で各イメージを marmot に登録する手順を整備する。

### 完了条件（案）
- 利用者が正規に用意した Windows Server 2022 / 2025 の各 ISO を使い、文書化された手順でそれぞれのベースイメージを作成できる。
- 各 QCOW2 イメージに対応する virtio-win ドライバと cloudbase-init が導入されていることを確認できる。
- 各イメージをバージョン識別可能な形で marmot に登録できる手順が明記されている。
- Phase 1 の対応後、各バージョンのイメージで VM の起動を個別に確認できることが検証項目に含まれている。
- ISO やライセンスなど、手順の前提条件と利用者が準備するものが明記されている。
- Phase 1 の起動・コンソール対応、および Phase 3 の VM ごとのプロビジョニングとの境界が明記されている。

## Phase 3 詳細スコープ（提案）

### 目的
Windows VM 起動時に、Linux の cloud-init 相当の処理を cloudbase-init で実現し、
ユーザー/SSH キー/ホスト名などを自動設定できるようにする。

### 非目的
- Windows ベースイメージ自体の作成（Phase 2 で対応）。
- libvirt ドメイン XML 生成の Windows 対応（Phase 1 で対応）。
- Ansible 等による VM 内部の構成管理の自動化（将来検討、本フェーズでは対象外）。

### 変更範囲（案）
- `pkg/marmotd/cloudbase-init.go`（新規）
  - `GenerateCloudbaseInitISO` 関数を新設。OpenStack ConfigDrive 互換形式
    （`openstack/latest/meta_data.json` + `user_data`）で ISO を生成する。
  - `user_data` は cloudbase-init が解釈可能な形式（cloud-config サブセット or PowerShell スクリプト）で
    ユーザー作成・パスワード設定・SSH 公開鍵登録・ホスト名設定を記述する。
- `pkg/marmotd/server_image_module.go`
  - `serverImageModule` インターフェースの実装として `serverImageModuleWindows2022` / `serverImageModuleWindows2025`
    を追加する。
  - 各実装の `GenerateCloudInitISO` を `GenerateCloudbaseInitISO` 呼び出しに差し替える。
  - `resolveServerImageModuleFromOS` に Phase 2 で導入した `osName == "windows"` 分岐を追加する。
- `pkg/marmotd/server.go`
  - ISO 生成・CD-ROM アタッチ処理は Linux/Windows 共通のインターフェース経由のため変更は最小限
    （OS 別の分岐はモジュール側に閉じる想定）。
- `api/marmot-api-v1.yaml` + コード生成（`api/marmot-api-v1.go` は直接編集しない）
  - 必要であれば Administrator パスワード等 Windows 固有の認証パラメータを API スキーマに追加する。

### 完了条件（案）
- `go build ./...` 成功。
- `go test ./pkg/marmotd/...` 成功。
- `osVariant=windows2022` / `windows2025` のサーバー作成時に ConfigDrive 形式 ISO が生成されることを
  ユニットテストで確認する。
- Linux 系 VM の既存 cloud-init 生成処理に差分がないことを確認する。
- Phase 1 / Phase 2 の成果物を用いて実際に Windows VM を起動し、cloudbase-init によってユーザー名/SSH キー/
  ホスト名が反映されることを手動確認する。

## セキュリティ上の考慮事項

- SPICE をパスワードなしで LAN 上に直接公開すると、認証なしでリモートから VM 画面・入力を
  乗っ取れる状態になる（OWASP: Broken Access Control 相当のリスク）。
  → SPICE パスワードを付与した上で LAN Listen する方針とする。
- パスワードは VM ごとにランダム生成し、API 経由で取得する形にする（固定パスワードや平文設定ファイルへの
  保存は避ける）。

## オープン課題（未確定・要確認）

- UEFI Secure Boot の要否（Phase 1 ではスコープ外とする案）
- グラフィカルコンソールの実装方式は「SPICE 接続情報を `mactl` から取得し、手元の `remote-viewer` で接続」で
  合意（ブラウザ向け noVNC/spice-html5 プロキシは今回は不採用）
- SPICE Listen アドレスは「ノードの LAN アドレスで直接接続できるようにしたい」で合意
- Windows イメージ作成方法（Phase 2）は「作成方法から相談したい」の状態で、具体的な手順は未検討
- Administrator パスワードの生成・取得方法（SPICE パスワードと同様に API 経由でランダム生成して返す方式にするか）（Phase 3）
- cloudbase-init の `user_data` 形式（cloud-config 互換 vs PowerShell スクリプト）の選定（Phase 3）
- ホスト名反映に sysprep との併用が必要か（Phase 3）
