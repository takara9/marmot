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
- **ネットワークアクセス**: VM のネットワークインターフェースは host-bridge に接続できるが、
  VM ごとに host-bridge 上の IP アドレスを確保し特定ポート（例: RDP の 3389/tcp）への到達性を
  確保する仕組みや、接続元を CIDR で制限する仕組みは、Internet Gateway / Network LB など一部の
  リソース種別にしか存在せず、サーバー（VM）に対する標準機能としては整備されていない。
  また、VM 自体が host-bridge に接続されていない場合（`mgmt` ネットワークのみに接続された VM）に
  外部から到達させる手段（ノード側での待受ポート払い出し・`mgmt` ネットワークへの転送）も存在しない。
  Windows は SPICE 経由のグラフィカルコンソールに加えて RDP での運用が一般的なため、この欠落が
  Windows 対応上の課題となる。

## 対応方針（全体ロードマップ）

範囲が広いため、3 フェーズに分割して進める（1 変更 = 1 PR の原則に従う）。

| フェーズ | 内容 | ステータス |
|---|---|---|
| Phase 1 | libvirt ドメイン生成の Windows 対応 + グラフィカルコンソール（SPICE）取得機能 + RDP 接続対応（ノードの host-bridge アドレス上の空きポート経由で `mgmt` ネットワークへ転送、VM 自体の host-bridge 接続有無に依存しない Windows サーバー専用機能） | 未着手（詳細スコープの個別論点は決定済み、Phase 全体の着手承認は未取得） |
| Phase 2 | Windows Server 2022 / 2025 のベースイメージのビルド手順整備（autounattend.xml によるインストール自動化 + virtio-win ドライバ組込 + cloudbase-init 事前導入 + `sysprep /generalize` によるイメージ汎用化）と marmot イメージ登録（`sourceUrl: file://` 対応含む）への取り込み | 未着手 |
| Phase 3 | sysprep 済みイメージの specialize/OOBE パスで cloudbase-init によるプロビジョニング ISO 生成（config-drive 形式）を実行し、ユーザー/SSH キー/ホスト名を自動設定 | 未着手 |

対象 Windows バージョン: **Windows Server 2022 および Windows Server 2025**

## Phase 1 詳細スコープ（詳細論点は決定済み・Phase 全体の着手承認は未取得）

### 目的
1. `osVariant=windows2022` / `windows2025` 系 VM を libvirt ドメインとして正しく起動できるようにする。
2. `mactl` から SPICE 接続情報（ホスト IP:ポート、パスワード）を取得し、手元の `remote-viewer` で
   画面に接続できるようにする（Windows 系 VM 限定）。
3. VM 自体が host-bridge に接続されていない場合でも、**ノードの host-bridge アドレス上の空きポート**で
   RDP 接続を受け、`mgmt` ネットワーク経由で対象 VM の 3389/tcp へ転送することで接続できるようにする
   （Windows サーバー専用機能。既存の Internet Gateway / Network LB の
   `bindPublicIpAddress`/`remoteCIDR` とは独立した仕組みとする）。

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
  **（実装済み）**
- `pkg/marmotd/image_os_module.go`
  - `resolveImageOSModuleFromSpec` に `"windows"` ケース追加（`customizeHandler` なし = virt-customize を実行せず素通し）
- `api/marmot-api-v1.yaml` + コード生成（`api/marmot-api-v1.go` は直接編集しない）
  - サーバーのグラフィカルコンソール接続情報を返すエンドポイント追加
    （`GET /server/{id}/console/graphical` → host, port, passwd）**（実装済み）**
  - Windows サーバー専用の RDP アクセス情報を返すエンドポイント追加
    （例: `GET /server/{id}/console/rdp` → host（ノードの host-bridge アドレス）, port（払い出し済みの
    待受ポート）。接続許可元 CIDR はサーバー作成時に指定できる API スキーマ項目として追加する。
    ポート番号は marmot 側が空きポートから自動採番する値であり、利用者が指定するものではない。
    Server リソース専用とし、Internet Gateway / Network LB の `bindPublicIpAddress`/`remoteCIDR`
    とは別フィールドとして、既存リソースとの重複・流用はしない）**（未実装）**
- `pkg/marmotd/console.go`
  - 上記エンドポイントのハンドラ追加（稼働中ドメインの SPICE 設定を XML から取得）**（実装済み）**
  - `pkg/marmotd/server.go` で、Windows 系サーバー生成時に `SpiceListenAddress`（ノードの LAN アドレス、
    `util.NameserverForDNSListenAddr(CurrentConfig().DNSListenAddr)` で解決）と `SpicePasswd`
    （`crypto/rand` による都度ランダム生成）を `virt.ServerSpec` へ設定する配線を追加
    （`pkg/marmotd/spice_password.go`）**（実装済み）**
- `cmd/mactl/cmd/console.go`
  - `mactl console --graphical` / `mactl console --rdp` 等で接続情報を表示するサブコマンド/オプション追加
    （`--graphical` は実装済み。host/port/passwd と `remote-viewer spice://host:port` のヒントを表示する。
    `--rdp` は RDP 転送機能の実装待ち）
- ノードの host-bridge アドレス（`dns_listen_addr` 等で使われる、ノード自身の host-bridge 側 IP）上に
  VM ごとに空きポートを自動採番して TCP 待受を開始し、受けた RDP 接続を `mgmt` ネットワーク経由で
  対象 VM の `mgmt` IP アドレス（`10.245.0.0/16` 内）の 3389/tcp へ転送する仕組みの実装。
  接続元は指定された CIDR のみ許可する（VM 自体が host-bridge 未接続でも動作する、
  Windows サーバー専用の仕組み。ポートの採番・解放・多重割当防止を含む）。**（未実装）**

### 完了条件（案）
- `go build ./...` 成功
- `go test ./pkg/virt/... ./pkg/marmotd/... ./cmd/mactl/...` 成功
- `osVariant=windows2022` / `windows2025` 指定時に生成される XML が UEFI/TPM（`swtpm` エミュレータバックエンド）/QXL/SPICE
  （パスワード付き LAN Listen）になることをユニットテストで確認
- Linux 系 `osVariant` で生成される XML に差分がないことを確認
- Windows 系 VM が host-bridge に未接続の状態でも、ノードの host-bridge アドレス上に払い出された
  ポートへ外部クライアントから RDP（3389/tcp 相当）接続でき、指定した CIDR 以外からは到達できないことを確認
- 同一ノード上に複数の Windows VM を起動した場合でも、ポート採番が重複しないことを確認
- Linux 系 VM には RDP ポートフォワード機能が適用されず、既存の `mgmt` ネットワークの動作
  （DNS/apt-cacher-ng 等の既存機能）に差分がないことを確認
- `mactl console --graphical <windows-vm-name>` で host/port/passwd が表示されることを確認（手動確認）

## Phase 2 詳細スコープ（詳細論点は決定済み・Phase 全体の着手承認は未取得）

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
  - インストール ISO は、既存の `kind: Image` マニフェストと同じ形式で `spec.sourceUrl` に
    ローカルファイルパスを `file://` スキームで指定する形で marmot へ渡す（決定事項）。

    ```yaml
    apiVersion: v1
    kind: Image
    metadata:
        name: windows2022
    spec:
        sourceUrl: file:///home/ubuntu/Downloads/SERVER_EVAL_x64FRE_ja-jp.iso
        osName: windows
        osVersion: "2022"
    ```

  - **（実装済み）**: 現状の `downloadImageWithContext`
    （[pkg/marmotd/image.go](../pkg/marmotd/image.go)）は `net/http.Client` で直接
    `http.NewRequestWithContext` → `client.Do` を行っており、`file://` スキームには非対応
    （Go 標準の `http.Transport` に `file://` 用 RoundTripper が登録されていないため
    `unsupported protocol scheme "file"` エラーになる）という課題があったため、`sourceUrl` が
    `file://` の場合はローカルファイルコピーに分岐する `copyLocalImageFile` を追加した。
  - `autounattend.xml` を使った無人インストール手順を用意する。
  - 対象バージョンごとに virtio-win ドライバおよび cloudbase-init の対応状況を確認し、イメージに組み込む。
    初期設定と VM ごとの設定値の適用は Phase 3 で扱う。
  - **イメージキャプチャ前に `sysprep /generalize /oobe /shutdown` を実行し、SID 等マシン固有情報を
    除去したうえで QCOW2 化する**（cloudbase-init は sysprep で汎用化されたイメージが初回起動時に通る
    specialize/OOBE パスにフックして動作する設計のため、sysprep による汎用化は cloudbase-init 動作の前提条件。
    これは Phase 3 でホスト名反映に「sysprep 併用が必要」とした決定事項の実体であり、Phase 3 側で
    sysprep を都度実行するという意味ではなく、Phase 2 で汎用化済みのイメージを用意しておくことを指す）。
  - 各バージョンのベースイメージを QCOW2 形式で出力する。
- バージョンを識別できる形で各イメージを marmot に登録する手順を整備する。

### 完了条件（案）
- 利用者が正規に用意した Windows Server 2022 / 2025 の各 ISO を使い、文書化された手順でそれぞれのベースイメージを作成できる。
- 各 QCOW2 イメージに対応する virtio-win ドライバと cloudbase-init が導入されていることを確認できる。
- `sysprep /generalize` によってイメージが汎用化されている（マシン固有の SID 等が除去されている）ことを確認できる。
- `spec.sourceUrl: file://` 指定でローカル ISO から Image 登録ができることを確認する
  （`downloadImageWithContext` のローカルファイル読み込み対応を含む）。
- 各イメージをバージョン識別可能な形で marmot に登録できる手順が明記されている。
- Phase 1 の対応後、各バージョンのイメージで VM の起動を個別に確認できることが検証項目に含まれている。
- ISO やライセンスなど、手順の前提条件と利用者が準備するものが明記されている。
- Phase 1 の起動・コンソール対応、および Phase 3 の VM ごとのプロビジョニングとの境界が明記されている。

## Phase 3 詳細スコープ（詳細論点は決定済み・Phase 全体の着手承認は未取得）

### 目的
Windows VM 起動時に、Linux の cloud-init 相当の処理を cloudbase-init で実現し、
ユーザー/SSH キー/ホスト名などを自動設定できるようにする。

### 非目的
- Windows ベースイメージ自体の作成（Phase 2 で対応）。
- `sysprep /generalize` によるイメージ汎用化そのものの実行（Phase 2 でベースイメージ作成時に実施済み。
  Phase 3 は、汎用化済みイメージが初回起動時に通る specialize/OOBE パスに cloudbase-init がフックして
  動作することを前提とし、sysprep コマンド自体を Phase 3 側で実行することはない）。
- libvirt ドメイン XML 生成の Windows 対応（Phase 1 で対応）。
- Ansible 等による VM 内部の構成管理の自動化（将来検討、本フェーズでは対象外）。

### 変更範囲（案）
- `pkg/marmotd/cloudbase-init.go`（新規）
  - `GenerateCloudbaseInitISO` 関数を新設。OpenStack ConfigDrive 互換形式
    （`openstack/latest/meta_data.json` + `user_data`）で ISO を生成する。
  - `user_data` は cloudbase-init が解釈可能な形式（cloud-config サブセット or PowerShell スクリプト）で
    ユーザー作成・パスワード設定・SSH 公開鍵登録・ホスト名設定を記述する。
  - ホスト名設定は、Phase 2 で `sysprep /generalize` 済みのイメージが初回起動時に specialize パスへ入り、
    その中で cloudbase-init が実行されることで反映される（Phase 3 では sysprep を呼び出さない。
    前提として Phase 2 で汎用化されたイメージを使用していることが必須）。
- `pkg/marmotd/server_image_module.go`
  - `serverImageModule` インターフェースの実装として `serverImageModuleWindows2022` / `serverImageModuleWindows2025`
    を追加する。
  - 各実装の `GenerateCloudInitISO` を `GenerateCloudbaseInitISO` 呼び出しに差し替える。
  - `resolveServerImageModuleFromOS` に Phase 2 で導入した `osName == "windows"` 分岐を追加する。
- `pkg/marmotd/server.go`
  - ISO 生成・CD-ROM アタッチ処理は Linux/Windows 共通のインターフェース経由のため変更は最小限
    （OS 別の分岐はモジュール側に閉じる想定）。
- `api/marmot-api-v1.yaml` + コード生成（`api/marmot-api-v1.go` は直接編集しない）
  - Administrator パスワード等 Windows 固有の認証パラメータを API スキーマに追加する
    （SPICE パスワードと同様に API 経由でランダム生成して返す方式、決定済み）。

### 完了条件（案）
- `go build ./...` 成功。
- `go test ./pkg/marmotd/...` 成功。
- `osVariant=windows2022` / `windows2025` のサーバー作成時に ConfigDrive 形式 ISO が生成されることを
  ユニットテストで確認する。
- Linux 系 VM の既存 cloud-init 生成処理に差分がないことを確認する。
- Phase 2 で `sysprep /generalize` 済みのベースイメージを用いて実際に Windows VM を起動し、
  specialize/OOBE パス経由で cloudbase-init が実行され、ユーザー名/SSH キー/ホスト名が
  反映されることを手動確認する（sysprep はこの確認の前提条件であり、Phase 3 側で実行するものではない）。

## セキュリティ上の考慮事項

- SPICE をパスワードなしで LAN 上に直接公開すると、認証なしでリモートから VM 画面・入力を
  乗っ取れる状態になる（OWASP: Broken Access Control 相当のリスク）。
  → SPICE パスワードを付与した上で LAN Listen する方針とする。
- パスワードは VM ごとにランダム生成し、API 経由で取得する形にする（固定パスワードや平文設定ファイルへの
  保存は避ける）。
- RDP ポートフォワード（ノードの host-bridge アドレス上の払い出しポート → 対象 VM の 3389/tcp）を
  CIDR 制限なしで公開すると、Administrator アカウントへの総当たり攻撃等の対象になり得る。
  → 接続元 CIDR の指定を必須（または安全側のデフォルト）とし、Administrator パスワードは
  ランダム生成・API経由取得とする方針と合わせて運用する。
- ノードの host-bridge アドレスに複数 VM 分の RDP ポートが集約されるため、ポートスキャンにより
  稼働中の Windows VM の存在が外部から推測されやすくなる。
  → 採番するポート範囲の管理、および CIDR 制限の徹底で影響を抑える。

## オープン課題（未確定・要確認）

- UEFI Secure Boot は Phase 1 ではスコープ外とする（決定）
- グラフィカルコンソールの実装方式は「SPICE 接続情報を `mactl` から取得し、手元の `remote-viewer` で接続」で
  合意（ブラウザ向け noVNC/spice-html5 プロキシは今回は不採用）（決定）
- 別途 RDP 接続用に、Windows サーバー専用の機能として実装する方針で合意（決定）。
  検討の経緯:
  1. 当初案「VM ごとに host-bridge 上の IP アドレスを確保（`bindPublicIpAddress` 相当）」は、
     Internet Gateway / Network LB と機能重複し、かつ VM が host-bridge 未接続の場合に使えないため不採用。
  2. 次善案「`mgmt-host`（`10.245.0.1`）で外部クライアントからの RDP 接続を直接受ける」は、
     `mgmt` ネットワークが OVN 上の内部限定ネットワークであり（ゲストVM→ホストの一方向、
     ACL許可リストで通信を厳格制御、外部クライアントが参加・到達する経路が存在しない）、
     技術的に成立しないため不採用（[MEMO-mgmt-dns.md](./MEMO-mgmt-dns.md) 参照）。
  3. **最終決定**: ノードの host-bridge アドレス（`dns_listen_addr` 等で使われる、ノード自身が
     既に保持する host-bridge 側 IP。VM 個別の IP ではない）上に、VM ごとに空きポートを自動採番して
     TCP 待受を開始し、そこで受けた RDP 接続を `mgmt` ネットワーク経由で対象 VM の `mgmt` IP の
     3389/tcp へ転送する。接続元は CIDR で制限する。これにより VM 自体は host-bridge に未接続でも
     （`mgmt` NIC のみでも）RDP 到達可能になる。
- SPICE Listen アドレスは「ノードの LAN アドレスで直接接続できるようにしたい」で合意（決定）
- インストール ISO の受け渡し方法（Phase 2）は「既存の `kind: Image` マニフェストで
  `spec.sourceUrl: file:///path/to/xxx.iso` を指定する」で合意。現状の `downloadImageWithContext`
  （`pkg/marmotd/image.go`）は `net/http.Client` のみで `file://` に非対応と確認済み
  （`unsupported protocol scheme "file"` になる）。Phase 2 でローカルファイル読み込み分岐の
  実装が必要（変更範囲に追記済み）
- Administrator パスワードは、SPICE パスワードと同様に API 経由でランダム生成して返す方式とする（決定）（Phase 3）
- cloudbase-init の `user_data` 形式は、cloudbase-init 標準の処理に任せ、cloud-config 互換 /
  PowerShell スクリプトのどちらでも受け付ける（形式を限定しない）方針とする（決定）（Phase 3）
- ホスト名反映には sysprep との併用が必要（決定）。ただし、これは Phase 3 の都度の処理として
  sysprep を実行する、という意味ではない。**sysprep `/generalize` は Phase 2 のベースイメージ作成時に
  1回だけ実行し、イメージを汎用化（SID 等のマシン固有情報を除去）しておく**。cloudbase-init は、
  その汎用化済みイメージが VM 初回起動時に通る Windows の specialize/OOBE パスにフックして動作する
  設計になっており、ホスト名反映はこの仕組みの上で実現される（Phase 2 変更範囲・Phase 3 変更範囲/
  完了条件に反映済み）

### 実装上の課題（要調査・未着手）

Phase 1（RDP 転送機能）関連:

- **mgmt ネットワーク ACL の方向性**: [MEMO-mgmt-dns.md](./MEMO-mgmt-dns.md) によれば、`mgmt` ネットワークの
  OVN ACL は「ゲストVM→ホスト」方向の許可リスト方式（ゲストVM間通信も含め原則全拒否）。
  今回実装するのは逆方向の「ホスト→ゲストVM」（ノードの待受ポートから VM の 3389/tcp への転送）であり、
  現在の ACL ルール（`pkg/marmotd/network_management_acl_test.go` の `BuildManagementNetworkACLRules`）が
  この通信パターンを許可するかどうか未確認。新たな ACL エントリ追加が必要になる可能性が高いが、
  Phase 1 の変更範囲にこのファイルへの言及がない。
- **ポート払い出しの永続化・衝突回避の設計が未定**: 「空きポートを自動採番」とあるが、
  (a) marmotd 自身の API/DNS 等と衝突しない予約ポート範囲の設計、
  (b) 採番結果を etcd 等へ永続化し marmotd 再起動後も復元する方法、
  (c) VM 削除時のポート解放、が未設計。
- **転送方式（アプリケーション層プロキシ vs iptables DNAT）が未決定**: marmotd プロセス内で TCP プロキシを
  実装するか、OS の iptables/nftables で DNAT するかで、実装の複雑さ・root 権限要否・
  再起動時の状態復元方法が大きく変わる。
- **マルチノードクラスタでの `host` 解決**: API が返す `host`（ノードの host-bridge アドレス）は、
  VM が稼働する実際のノードのアドレスを動的に解決する必要がある。VM のノード間移動（マイグレーション）時の
  再割当も未考慮。
- **swtpm パッケージ依存**: TPM 2.0 デバイス（`swtpm` バックエンド）は全 marmot ノードに `swtpm` パッケージが
  導入済みであることが前提だが、deb パッケージング（`tools/deb/postinst` 等）への依存追加が
  変更範囲に含まれていない。

Phase 2（ベースイメージ作成）関連:

- **自動化ツールの選定が未定**: sysprep は Windows ゲスト内部から実行する必要があり、
  「ISO ダウンロード→無人インストール→ドライバ/cloudbase-init 導入→sysprep→シャットダウン→QCOW2変換」
  という一連の作業を、手動手順にするのか、Packer 等のビルド自動化ツールを導入するのかが未定。
- **virtio ドライバのインストール順序（鶏と卵問題）**: Phase 1 でディスクバスを `virtio-scsi` に
  決定したため、Windows セットアップ自体が仮想ディスクを認識するには、インストール時点で virtio-win
  ドライバ ISO を追加の CD-ROM としてマウントし、`autounattend.xml` 側でドライバパスを指定する必要がある。
  現在の変更範囲は「ドライバをイメージに組み込む」としか書かれておらず、インストール時点での
  注入手順が明記されていない。
- **評価版 ISO のライセンス制約**: 決定済み YAML 例のファイル名が `SERVER_EVAL_x64FRE...`（評価版）に
  なっている。評価版は初回起動から 180 日の評価期間制限があり、`sysprep /generalize` を行っても
  評価期限はリセットされない。ベースイメージとして繰り返し利用する運用に向くかは要確認。
- **cloudbase-init インストーラの入手経路**: イメージ作成時にインターネットから cloudbase-init
  インストーラをダウンロードする前提か、オフラインで事前取得したバイナリを使うのか未記載
  （apt-cacher-ng 相当の Windows 向けキャッシュ機構は存在しない）。
