# Windows Server 2022/2025 で sshd を有効化する。

Windows Server のベースイメージの作成処理に、sshd を有効化して、YAMLに指定された公開鍵をセットしたい。

mmImage に windows OSが指定されていて、auth に公開鍵が指定されている場合は、Windows に sshd を起動して、
公開鍵をC:\ProgramData\ssh\administrators_authorized_keysにセットする。

- ユーザーが未指定の場合は、Administrator 用に公開鍵を設定。
- ユーザーが指定されている場合は、エラーに落として、エラーメッセージで実装されていないことを表示する。

```yaml
apiVersion: v1
kind: Server
metadata:
    name: web2
spec:
    cpu: 1
    memory: 1024
    mmImage: win2k22
    auth:
        url: https://github.com/takara9.keys
```

- ファイアウォールの ssh ポートを開く
- sshd をインストールして、自動起動に設定する。
- 規定シェルをPowerShellに設定する。

## 実装可否の検討

結論: **実装可能**。API(yaml)の変更は不要。

### 既存実装との関係

- Windows の起動時設定は、cloudbase-init の ConfigDrive ISO で行っている。
  `pkg/marmotd/cloudbase-init.go` の `enableRDPCommands`(`runcmd`)が RDP を有効化しており、
  同じ方式で sshd の起動・ファイアウォール開放・鍵配置を追加できる。
- 公開鍵の取得(`auth.url` / `auth.publicKey`)は `pkg/marmotd/server.go` の `cloudInitAuthInputs` を流用できる。
- OS 別の切り替えは `pkg/marmotd/server_image_module.go` の `windows2022` / `windows2025` モジュールで行える。

### 処理の分担

メモ冒頭は「ベースイメージの作成処理」としているが、公開鍵は Server 作成時の `auth` で決まるため、役割を分ける。

| タイミング | 担当 | 内容 |
|---|---|---|
| ベースイメージ作成時 | `tools/build-windows-qcow2.sh` / autounattend | OpenSSH Server のインストール、`sshd` 自動起動設定、規定シェルを PowerShell に設定 |
| Server 起動時 | cloudbase-init の `runcmd` | `C:\ProgramData\ssh\administrators_authorized_keys` への鍵配置と ACL 設定、ファイアウォール(TCP 22)開放、`sshd` 起動 |

- `Add-WindowsCapability` によるインストールはオンライン接続が必要。
  ベースイメージ作成環境がオフラインの場合は、OpenSSH の MSI/zip を autounattend 用の assets に同梱する必要がある。

### 決定事項

- **ユーザー指定時のエラー化の範囲**: 「sshd 用の公開鍵が指定され、かつユーザーも指定された場合」に限定する。
  - 現状、Windows で `auth.user` / `auth.users` を指定すると cloudbase-init が Administrators グループのユーザーを作成する。
    公開鍵の指定がない場合は、この既存動作を変更しない。
  - 公開鍵(`auth.url` または `auth.publicKey`)とユーザー(`auth.user` または `auth.users`)が両方指定された場合は、
    「Windows でのユーザー指定付き公開鍵設定は未実装」というエラーメッセージで失敗させる。
  - 公開鍵のみ指定(ユーザー未指定)の場合は、Administrator 用に `administrators_authorized_keys` へ設定する。

- **sshd のインストール場所**: ベースイメージ側で行う。
  - OpenSSH Server のインストール、`sshd` の自動起動設定、規定シェルの PowerShell 設定は、
    `tools/build-windows-qcow2.sh` / autounattend によるベースイメージ作成時に実施する。
  - Server 起動時(cloudbase-init の `runcmd`)では、インストールは行わず、鍵配置・ACL 設定・ファイアウォール開放・`sshd` 起動のみを行う。
  - これにより、Server 起動時にオンライン接続が不要になる。
  - 既存のベースイメージ(sshd 未導入)からは sshd に接続できないため、再作成が必要。
- **`administrators_authorized_keys` の権限**: SYSTEM と Administrators のみに権限を残す。
  - 継承を無効化し、他のユーザー・グループの権限を除去する(`icacls` で設定)。
  - 未設定だと sshd が鍵を無視するため、鍵ファイルを書き込んだ後に必ず設定する。

### 未決事項

なし。

### 実装上の注意

- `runcmd` は cmd.exe で実行される。複数行の鍵を書き込むには PowerShell の `-EncodedCommand` を使うのが確実。
- `administrators_authorized_keys` の権限設定(決定事項参照)は、ロケールに依存しないよう、
  アカウント名ではなく既知 SID(SYSTEM は `*S-1-5-18`、Administrators は `*S-1-5-32-544`)で `icacls` に指定する。
- `runcmd` は1行ずつ実行され、前行の失敗で後続は止まらない(`enableRDPCommands` のコメント参照)。
  sshd の起動失敗を検知しにくいため、最終行の終了コードの扱いに注意する。
- ユーザー指定エラーは、Server 作成時(API 受付時)に返すのが望ましい。起動後の失敗では原因が分かりにくい。

### 変更範囲(案)

- `pkg/marmotd/cloudbase-init.go`(+ テスト): sshd 設定コマンドの追加
- `pkg/marmotd/server_image_module.go` または `pkg/marmotd/server.go`: ユーザー指定時のエラー
- `tools/build-windows-qcow2.sh` の autounattend: ベースイメージへの sshd 導入

### 完了条件(案)

- `go build` 成功
- cloudbase-init のユニットテスト通過(鍵あり/なし × ユーザー指定あり/なし の4通り。鍵あり+ユーザーありのみエラー)
- Windows 実機で、`auth.url` の公開鍵による SSH ログインと PowerShell 起動を確認