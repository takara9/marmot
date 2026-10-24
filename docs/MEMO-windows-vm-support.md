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
| Phase 1 | libvirt ドメイン生成の Windows 対応 + グラフィカルコンソール（SPICE）取得機能 + RDP 接続対応（ノードの host-bridge アドレス上の空きポート経由で `mgmt` ネットワークへ転送、VM 自体の host-bridge 接続有無に依存しない Windows サーバー専用機能） | libvirt ドメイン生成・グラフィカルコンソールは実装済み・**実機確認済み（2026-10-09）**。RDP 接続対応は未着手 |
| Phase 2 | Windows Server 2022 のベースイメージのビルド手順整備（autounattend.xml によるインストール自動化 + virtio-win ドライバ組込 + cloudbase-init 事前導入 + `sysprep /generalize` によるイメージ汎用化）と marmot イメージ登録（`sourceUrl: file://` 対応含む）への取り込み | **完了（Windows Server 2022、実機確認済み・2026-10-11）**（[HOWTO-build-windows-image.md](./HOWTO-build-windows-image.md)）。Windows Server 2025 のイメージ作成・起動確認はスコープ外 |
| Phase 3 | sysprep 済みイメージの specialize/OOBE パスで cloudbase-init によるプロビジョニング ISO 生成（config-drive 形式）を実行し、ユーザー/SSH キー/ホスト名/静的 IP を自動設定 | ISO 生成（ホスト名・静的 IP）は実装・実機確認済み（2026-10-11）。Administrator パスワードの API 化は未着手 |

対象 Windows バージョン: **Windows Server 2022 および Windows Server 2025**

## Phase 1 詳細スコープ（libvirt ドメイン生成・グラフィカルコンソールは実装・実機確認済み。RDP 接続対応は未着手）

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
  - `resolveImageOSModuleFromSpec` に `"windows"`（`"2022"`/`"2025"`）ケースを追加し、
    `imageOSModuleWindows2022`/`imageOSModuleWindows2025` として登録。
    `customizeHandler` には明示的な no-op 関数（`customizeWindowsQcowImageWithContext`）を設定する
    （`nil` のままでは `commonImageOSModule.customizeDownloadedImage` が既定の Ubuntu 向け
    `virt-customize` 処理にフォールバックしてしまい、NTFS を持つ Windows イメージに対して
    実行されてしまうため、意図通りに「何もしない」ことを保証するには明示的な no-op ハンドラが必要
    であると実装時に判明した。設計時点の「`customizeHandler` なし = 素通し」という記述は誤りだったため、
    この記述を訂正する）。**（実装済み）**
  - **（実装済み・実装時に新たに判明した追加修正）**: `pkg/marmotd/image.go` の Image 作成フローは、
    ダウンロード/コピー後に OS 種別を問わず一律で「`qemu-img resize` による 16GB への強制リサイズ」
    + 「`growFilesystem` によるパーティション/ファイルシステム拡張」を実行していた。
    `growFilesystem` は `ext2/3/4`・`xfs` のみに対応しており、Windows の `NTFS` は
    `unsupported filesystem type for resize` として必ずエラーになるため、このままでは
    Windows イメージの登録（`mactl image create`）自体が失敗する状態だった。
    `image.Spec.OsName == "windows"` の場合はこの強制リサイズ処理をスキップし、
    代わりに `qemu-img info` から実際の仮想ディスクサイズを取得して `Image.Spec.Size` に
    正しく反映する（`qcowVirtualSizeGB` 関数を追加）方式に変更した。
    Windows イメージは Phase 2 の手順で既に意図したサイズで作成済みであることが前提となる。
  - **（実機テストで新たに判明・実装済み）**: `pkg/marmotd/image_os_validation.go` の
    `validateImageOSSpec`（`ApiCreateImage` の入り口で呼ばれる、`resolveImageOSModuleFromSpec`
    とは別の独立した許可リスト）にも `"windows"` が含まれておらず、`mactl image create` が
    `invalid spec.osName: "windows" (allowed: alpine, ubuntu, rocky, rockey, almalinux, debian)`
    で即座に拒否されていた。`canonicalOSName`・`allowedVersions` に `"windows"`（`"2022"`/`"2025"`）
    を追加して解消した。
  - **（実機テストで新たに判明・実装済み）**: `pkg/marmotd/server_image_module.go` の
    `resolveServerImageModuleFromOS`（サーバー作成時に `pkg/marmotd/server.go` の
    `CreateServerManage` から呼ばれ、`SetupBootVolume`/`GenerateCloudInitISO` を解決する）にも
    `"windows"` ケースが無く、Image 登録が通ってもサーバー作成コマンド自体が libvirt ドメイン定義
    （VM起動）に到達する前にエラーになっていた。Phase 3 の正式な cloudbase-init 対応
    （`GenerateCloudbaseInitISO`）はまだ実装しないが、Phase 1 の動作確認をブロックしないよう、
    最小限のプレースホルダとして `serverImageModuleWindows2022`/`serverImageModuleWindows2025` を
    追加した。`SetupBootVolume` は `setupBootVolumeNoOpWindows`（NTFS に netplan/ifupdown を
    書き込まない no-op）、`GenerateCloudInitISO` は既定の cloud-init ISO 生成をそのまま使う
    （Windows には無意味だが、CD-ROM として必ずアタッチされるため、有効な ISO ファイルを
    欠落させないための暫定措置）。
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
- **（実機確認済み・2026-10-09）**: 実機（ホスト名 `hv0`）で `mactl server create` により
  Windows 用ダミーイメージ（`osName: windows`, NTFS ではない 20GB の素の qcow2）から VM を起動し、
  `sudo virsh dumpxml` で以下を実機確認した。
  - `<os firmware='efi'>`（UEFI）
  - `<target dev='vda' bus='scsi'/>`（virtio-scsi ディスク）
  - `<tpm model='tpm-crb'>`（TPM 2.0、`swtpm` エミュレータバックエンド）
  - `<graphics type='spice' ... listen='192.168.1.8'>`（ノードの LAN アドレスへの Listen、`127.0.0.1` 固定ではない）
  - `<model type='qxl' ...>`（QXL ビデオデバイス）
  - `mactl console --graphical windows-dummy-vm` で host/port/passwd が正しく取得できることを確認
  - 取得した接続情報で `remote-viewer spice://<host>:<port>` から実際に SPICE 接続し、
    パスワード認証を含めて画面接続が確立することを確認
  - **（実機で発見した別課題・Phase 2 の手順として対処済み）**: [HOWTO-build-windows-image.md](./HOWTO-build-windows-image.md)
    の手順に従って実際の Windows Server 2022 ISO からベースイメージを作成・登録・起動したところ、
    Windows が BIOS/UEFI 画面（起動デバイス未検出）から進まない事象が発生した。原因は、
    Windows Setup が UEFI ブートマネージャーの起動エントリを「ビルド用 VM 専用の」NVRAM
    （ドメインごとに別ファイルとして自動生成される `OVMF_VARS`）にのみ登録するため、
    marmot が新しいドメインとしてベースイメージを起動する際に起動エントリが一切引き継がれない
    ことだった（libvirt/marmot 側の不具合ではなく、UEFI ディスクイメージの可搬性に関する
    一般的な制約）。`autounattend.xml` の `FirstLogonCommands` に、ファームウェアの標準
    フォールバックパス（`\EFI\Boot\bootx64.efi`）へブートマネージャーを複製するコマンドを
    追加することで解消した（文書化済み）。
- Windows 系 VM が host-bridge に未接続の状態でも、ノードの host-bridge アドレス上に払い出された
  ポートへ外部クライアントから RDP（3389/tcp 相当）接続でき、指定した CIDR 以外からは到達できないことを確認
  **（未実施。RDP 転送機構自体が未実装のため対象外）**
- 同一ノード上に複数の Windows VM を起動した場合でも、ポート採番が重複しないことを確認
  **（未実施。RDP 転送機構自体が未実装のため対象外）**
- Linux 系 VM には RDP ポートフォワード機能が適用されず、既存の `mgmt` ネットワークの動作
  （DNS/apt-cacher-ng 等の既存機能）に差分がないことを確認 **（未実施。RDP 転送機構自体が未実装のため対象外）**
- `mactl console --graphical <windows-vm-name>` で host/port/passwd が表示されることを確認（手動確認）

## Phase 2 詳細スコープ（Windows Server 2022 について完了・実機確認済み。Windows Server 2025 のイメージ作成・起動確認はスコープ外）

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
  - `autounattend.xml` を使った無人インストール手順を用意する。**（文書化済み:
    [HOWTO-build-windows-image.md](./HOWTO-build-windows-image.md)）**
  - 対象バージョンごとに virtio-win ドライバおよび cloudbase-init の対応状況を確認し、イメージに組み込む。
    初期設定と VM ごとの設定値の適用は Phase 3 で扱う。**（文書化済み）**
  - **イメージキャプチャ前に `sysprep /generalize /oobe /shutdown` を実行し、SID 等マシン固有情報を
    除去したうえで QCOW2 化する**（cloudbase-init は sysprep で汎用化されたイメージが初回起動時に通る
    specialize/OOBE パスにフックして動作する設計のため、sysprep による汎用化は cloudbase-init 動作の前提条件。
    これは Phase 3 でホスト名反映に「sysprep 併用が必要」とした決定事項の実体であり、Phase 3 側で
    sysprep を都度実行するという意味ではなく、Phase 2 で汎用化済みのイメージを用意しておくことを指す）。
    **（文書化済み。autounattend.xml の `FirstLogonCommands` 内で実行する方式とした）**
  - 各バージョンのベースイメージを QCOW2 形式で出力する。**（文書化済み）**
- バージョンを識別できる形で各イメージを marmot に登録する手順を整備する。**（文書化済み:
  [HOWTO-build-windows-image.md](./HOWTO-build-windows-image.md) の「4. marmot への登録」）**

### 完了条件（案）
- 利用者が正規に用意した Windows Server 2022 の ISO を使い、文書化された手順でベースイメージを作成できる。
  **（実機確認済み・2026-10-11。Windows Server 2025 はスコープ外）**
- QCOW2 イメージに virtio-win ドライバと cloudbase-init が導入されていることを確認できる。
  **（実機確認済み・2026-10-11。vioscsi の認識、NetKVM 経由のネットワーク疎通（IP 設定後にデフォルト GW へ ping 到達）、
  cloudbase-init の導入を確認）**
- `sysprep /generalize` によってイメージが汎用化されている（マシン固有の SID 等が除去されている）ことを確認できる。
  **（実機確認済み・2026-10-11）**
- `spec.sourceUrl: file://` 指定でローカル ISO から Image 登録ができることを確認する
  （`downloadImageWithContext` のローカルファイル読み込み対応を含む）。**（実装・ユニットテストで確認済み）**
- 各イメージをバージョン識別可能な形で marmot に登録できる手順が明記されている。**（文書化済み）**
- Phase 1 の対応後、イメージで VM の起動を確認できることが検証項目に含まれている。
  **（実機確認済み・2026-10-11。`win2k22` イメージから起動し、デスクトップ表示まで確認）**
- ISO やライセンスなど、手順の前提条件と利用者が準備するものが明記されている。**（文書化済み。
  評価版ISOの180日制限についても明記）**
- Phase 1 の起動・コンソール対応、および Phase 3 の VM ごとのプロビジョニングとの境界が明記されている。
  **（文書化済み）**

## Phase 3 詳細スコープ（ConfigDrive ISO 生成は実装済み。Administrator パスワードの API 化は未着手）

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
    （ボリュームラベル `config-2`、`openstack/latest/meta_data.json` + `user_data` + `network_data.json`）で
    ISO を生成する。**（実装済み）**
  - `meta_data.json` はホスト名（VM 名を英数字とハイフン、15 文字以内に変換）・`admin_pass`・SSH 公開鍵、
    `user_data`（cloud-config）は追加ユーザー（`Administrators` グループ。Windows に存在しない `root` は除外）、
    `network_data.json` は MAC アドレスで照合する静的 IPv4（アドレス、ネットマスク、ルート、DNS）を扱う。
    静的 IP を持たない NIC は出力せず、Windows 既定の DHCP のままにする。host-bridge と mgmt の両方を対象とする。
    なお、Linux では SetupBootVolume がブートボリュームへ書き込んでいたネットワーク設定を、Windows では
    この ConfigDrive で渡す（Windows の SetupBootVolume は no-op）。
  - `user_data` は cloudbase-init が解釈可能な形式（cloud-config サブセット or PowerShell スクリプト）で
    ユーザー作成・パスワード設定・SSH 公開鍵登録・ホスト名設定を記述する。
  - ホスト名設定は、Phase 2 で `sysprep /generalize` 済みのイメージが初回起動時に specialize パスへ入り、
    その中で cloudbase-init が実行されることで反映される（Phase 3 では sysprep を呼び出さない。
    前提として Phase 2 で汎用化されたイメージを使用していることが必須）。
- `pkg/marmotd/server_image_module.go`
  - `serverImageModuleWindows2022`/`serverImageModuleWindows2025` の `GenerateCloudInitISO` を
    `GenerateCloudbaseInitISO` 呼び出しに差し替えた。**（実装済み。Phase 1 の暫定実装から置き換え）**
    インターフェースにホスト名と NIC 情報の引数を追加したが、Linux 系の ISO 生成処理は変更していない
    （Windows では ansible の設定を使用しない）。
- `pkg/marmotd/server.go`
  - ISO 生成の呼び出しでホスト名（`Metadata.Name`）と NIC 情報を渡す。**（実装済み）**
    ISO 生成・CD-ROM アタッチ処理は Linux/Windows 共通のインターフェース経由で、OS 別の分岐はモジュール側に閉じる。
- `api/marmot-api-v1.yaml` + コード生成（`api/marmot-api-v1.go` は直接編集しない）
  - Administrator パスワード等 Windows 固有の認証パラメータを API スキーマに追加する
    （SPICE パスワードと同様に API 経由でランダム生成して返す方式、決定済み）。

### 完了条件（案）
- `go build ./...` 成功。**（確認済み）**
- `go test ./pkg/marmotd/...` 成功。**（追加した単体テストは成功。`TestMarmotdCeph` 等は Ceph・OVS・libguestfs の
  実行環境に依存するため、環境依存の失敗として対象外）**
- `osVariant=windows2022` / `windows2025` のサーバー作成時に ConfigDrive 形式 ISO が生成されることを
  ユニットテストで確認する。**（確認済み。ラベル `config-2` と `meta_data.json`/`user_data`/`network_data.json` を検証）**
- Linux 系 VM の既存 cloud-init 生成処理に差分がないことを確認する。**（確認済み。Ubuntu のラベルが従来の
  `cidata` であることをテストで確認）**
- Phase 2 で `sysprep /generalize` 済みのベースイメージを用いて実際に Windows VM を起動し、
  specialize/OOBE パス経由で cloudbase-init が実行され、ユーザー名/SSH キー/ホスト名が
  反映されることを手動確認する（sysprep はこの確認の前提条件であり、Phase 3 側で実行するものではない）。
  **（実機確認済み・2026-10-11: ホスト名、および host-bridge・mgmt の静的 IP の反映を確認。
  ユーザー作成・SSH 公開鍵・パスワード（`admin_pass`）の反映は未確認。Administrator パスワードの
  API 化（ランダム生成して返す方式）は未着手）**

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
