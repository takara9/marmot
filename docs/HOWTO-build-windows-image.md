# HOWTO: Windows Server ベースイメージの作成と marmot への登録

対象: Windows Server 2022 / Windows Server 2025（設計: [MEMO-windows-vm-support.md](./MEMO-windows-vm-support.md)）

この手順は、利用者が正規に用意した Windows Server のインストール ISO から、marmot で
利用可能な QCOW2 ベースイメージを作成し、`mactl image create` で marmot に登録するまでを
説明します。

## 前提条件

- **ビルド用ホスト**: libvirt / QEMU / `virt-install` / OVMF（UEFI ファームウェア）/ `swtpm`
  （TPM 2.0 エミュレーション）が利用可能な Linux マシン。marmot ノードをそのまま使うか、
  別途用意した作業用マシンのどちらでも構いません（marmot 自体はこのビルド作業を自動化しません。
  Phase 1 で対応した libvirt ドメイン設定（UEFI/TPM/virtio-scsi/QXL）と同じ構成を、
  このビルド用 VM でも手動で用意します）。
- **Windows Server インストール ISO**: 利用者が正規に用意したライセンス済みのメディア。
  ISO の入手・配布・ライセンス管理は marmot の対象外です。
  （評価版 ISO を使う場合は、下記「注意事項」を必ず確認してください。）
- **virtio-win ドライバ ISO**: 最新の stable 版を
  <https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso>
  から入手します。
- **cloudbase-init インストーラ（MSI）**: 安定版を
  <https://www.cloudbase.it/downloads/CloudbaseInitSetup_Stable_x64.msi>
  から入手します（最新のダウンロードリンクは <https://cloudbase.it/cloudbase-init/> で確認してください）。
  オフライン環境向けに、あらかじめダウンロードした MSI をビルド用 VM に渡す方法を後述します。

## 1. autounattend.xml の作成

Windows Setup を無人実行し、仕上げに cloudbase-init の導入と `sysprep /generalize` の実行まで
自動化する `autounattend.xml` を用意します。以下は最小構成の例です。
**プレースホルダ（`<!-- ... -->` で示した箇所）は環境に合わせて書き換えてください。**

```xml
<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend">

  <!-- Windows PE: ディスクパーティション作成 + virtio-scsi ドライバの注入 -->
  <settings pass="windowsPE">
    <component name="Microsoft-Windows-PnpCustomizationsWinPE"
                processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35"
                language="neutral" versionScope="nonSxS"
                xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
      <DriverPaths>
        <!-- virtio-win ISO 内のドライバパス。実際のドライブレターは環境により異なるため、
             ビルド用VM起動後に Shift+F10 でコマンドプロンプトを開き、
             各CD-ROMの内容を確認してから書き換えてください。 -->
        <PathAndCredentials wcm:action="add" wcm:keyValue="1">
          <Path>D:\vioscsi\2k22\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="2">
          <Path>D:\NetKVM\2k22\amd64</Path>
        </PathAndCredentials>
      </DriverPaths>
    </component>
    <component name="Microsoft-Windows-International-Core-WinPE"
                processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35"
                language="neutral" versionScope="nonSxS">
      <SetupUILanguage>
        <UILanguage>en-US</UILanguage>
      </SetupUILanguage>
      <!-- Windows ServerのOOBE(msoobe.exe)は、oobeSystemパスのInternational-Core設定だけでは
           「Localization」(国/地域・言語・キーボード)ページの表示を抑止できないため、
           ディスクイメージ展開前のこの時点でロケールを確定させておく必要がある。 -->
      <InputLocale>en-US</InputLocale>
      <SystemLocale>en-US</SystemLocale>
      <UILanguage>en-US</UILanguage>
      <UserLocale>en-US</UserLocale>
    </component>
    <component name="Microsoft-Windows-Setup"
                processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35"
                language="neutral" versionScope="nonSxS"
                xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
      <DiskConfiguration>
        <Disk wcm:action="add">
          <DiskID>0</DiskID>
          <WillWipeDisk>true</WillWipeDisk>
          <CreatePartitions>
            <!-- EFI システムパーティション -->
            <CreatePartition wcm:action="add">
              <Order>1</Order>
              <Type>EFI</Type>
              <Size>100</Size>
            </CreatePartition>
            <!-- MSR -->
            <CreatePartition wcm:action="add">
              <Order>2</Order>
              <Type>MSR</Type>
              <Size>16</Size>
            </CreatePartition>
            <!-- Windows 本体(残り全領域) -->
            <CreatePartition wcm:action="add">
              <Order>3</Order>
              <Type>Primary</Type>
              <Extend>true</Extend>
            </CreatePartition>
          </CreatePartitions>
          <ModifyPartitions>
            <ModifyPartition wcm:action="add">
              <Order>1</Order>
              <PartitionID>1</PartitionID>
              <Format>FAT32</Format>
              <Label>System</Label>
            </ModifyPartition>
            <ModifyPartition wcm:action="add">
              <Order>2</Order>
              <PartitionID>2</PartitionID>
            </ModifyPartition>
            <ModifyPartition wcm:action="add">
              <Order>3</Order>
              <PartitionID>3</PartitionID>
              <Format>NTFS</Format>
              <Label>Windows</Label>
              <Letter>C</Letter>
            </ModifyPartition>
          </ModifyPartitions>
        </Disk>
      </DiskConfiguration>
      <ImageInstall>
        <OSImage>
          <InstallFrom>
            <MetaData wcm:action="add">
              <Key>/IMAGE/INDEX</Key>
              <!-- ISO内のエディション(Index)。dism /Get-WimInfo /WimFile:install.wim で確認 -->
              <Value>4</Value>
            </MetaData>
          </InstallFrom>
          <InstallTo>
            <DiskID>0</DiskID>
            <PartitionID>3</PartitionID>
          </InstallTo>
        </OSImage>
      </ImageInstall>
      <UserData>
        <AcceptEula>true</AcceptEula>
        <!-- 評価版以外は正規のプロダクトキーを指定してください -->
        <!-- <ProductKey><Key>XXXXX-XXXXX-XXXXX-XXXXX-XXXXX</Key></ProductKey> -->
      </UserData>
    </component>
  </settings>

  <!-- specialize: コンピュータ名・タイムゾーン -->
  <settings pass="specialize">
    <component name="Microsoft-Windows-Shell-Setup"
                processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35"
                language="neutral" versionScope="nonSxS">
      <ComputerName>*</ComputerName>
      <TimeZone>UTC</TimeZone>
    </component>
  </settings>

  <!-- oobeSystem: ロケール(国/地域・言語・キーボード) + Administrator初期パスワード + cloudbase-init導入 + sysprep -->
  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-International-Core"
                processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35"
                language="neutral" versionScope="nonSxS">
      <!-- OOBEの「こんにちは」(国/地域・言語・キーボード選択)画面を省略するための既定ロケール。
           Microsoft公式ドキュメント(Automate OOBE)により、この設定は specialize ではなく
           oobeSystem パスに置く必要がある。 -->
      <InputLocale>ja-JP</InputLocale>
      <SystemLocale>ja-JP</SystemLocale>
      <UILanguage>ja-JP</UILanguage>
      <UserLocale>ja-JP</UserLocale>
    </component>
    <component name="Microsoft-Windows-Shell-Setup"
                processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35"
                language="neutral" versionScope="nonSxS"
                xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
      <UserAccounts>
        <AdministratorPassword>
          <!-- ベースイメージ作成作業中のみ使用する一時パスワード。
               sysprepで汎用化されるため、本番運用の実パスワードではない。 -->
          <Value>ChangeMe-Build-Only!</Value>
          <PlainText>true</PlainText>
        </AdministratorPassword>
      </UserAccounts>
      <OOBE>
        <HideEULAPage>true</HideEULAPage>
        <HideOEMRegistrationScreen>true</HideOEMRegistrationScreen>
        <HideOnlineAccountScreens>true</HideOnlineAccountScreens>
        <HideWirelessSetupInOOBE>true</HideWirelessSetupInOOBE>
        <NetworkLocation>Work</NetworkLocation>
        <SkipMachineOOBE>true</SkipMachineOOBE>
        <SkipUserOOBE>true</SkipUserOOBE>
      </OOBE>
      <AutoLogon>
        <Enabled>true</Enabled>
        <Username>Administrator</Username>
        <Password>
          <Value>ChangeMe-Build-Only!</Value>
          <PlainText>true</PlainText>
        </Password>
        <LogonCount>1</LogonCount>
      </AutoLogon>
      <FirstLogonCommands>
        <!-- 1. virtio-win の残りドライバ(NetKVM/Balloon/qxldod/vioserial等)を一括導入。
             pnputil は、ワイルドカードで一括指定した inf の一部が適用対象外の場合に
             0 以外の終了コードを返すことがある。SynchronousCommand は 0 以外の
             終了コードを返すと、それ以降の FirstLogonCommands を一切実行せずに
             無言でユーザーのデスクトップへ制御を渡してしまうため(実機で確認済み)、
             終了コードを必ず 0 にする。 -->
        <SynchronousCommand wcm:action="add">
          <Order>1</Order>
          <CommandLine>cmd /c pnputil /add-driver D:\*\2k22\amd64\*.inf /subdirs /install &amp; exit /b 0</CommandLine>
          <Description>Install remaining virtio-win drivers</Description>
        </SynchronousCommand>
        <!-- 2. cloudbase-init のサイレントインストール
             (E: はインストーラISO/フロッピーのドライブレター。環境に合わせて変更)。
             msiexec は /norestart 指定時でも、再起動が必要な場合は
             ERROR_SUCCESS_REBOOT_REQUIRED(終了コード 3010、実質成功)を返す
             ことがあり、Order 1 と同じ理由で終了コードを必ず 0 にする。 -->
        <SynchronousCommand wcm:action="add">
          <Order>2</Order>
          <CommandLine>cmd /c msiexec /i E:\CloudbaseInitSetup_Stable_x64.msi /qn /norestart LOGGING_SERIAL_PORT_NAME=COM1 RUN_SERVICE_AS_LOCAL_SYSTEM=1 &amp; exit /b 0</CommandLine>
          <Description>Install cloudbase-init</Description>
        </SynchronousCommand>
        <!-- 3. UEFI ブートマネージャーをフォールバックパス(\EFI\Boot\bootx64.efi)にも複製する。
             Windows Setup は通常 \EFI\Microsoft\Boot\bootmgfw.efi のみを書き込み、
             そこを指す起動エントリを「このビルド用VM専用の」NVRAM(OVMF_VARS)に登録する。
             marmot はベースイメージ(qcow2)を新しいドメインとして登録・起動するため、
             ドメインごとに別の空のNVRAMが自動生成され、ビルド用VMのNVRAM起動エントリは
             一切引き継がれない。これにより「ディスクは正しいがどこから起動すべきか
             ファームウェアが分からない」状態（BIOS/UEFI画面で停止）になる。
             ファームウェアの標準フォールバックパス(\EFI\Boot\bootx64.efi、通常はリムーバブル
             メディア/USBインストール時のみ使われる)にも複製しておくことで、NVRAMに起動
             エントリが無い新規ドメインでも自動検出・起動できるようにする。 -->
        <SynchronousCommand wcm:action="add">
          <Order>3</Order>
          <CommandLine>cmd /c mountvol S: /S &amp;&amp; mkdir S:\EFI\Boot &amp;&amp; copy /Y S:\EFI\Microsoft\Boot\bootmgfw.efi S:\EFI\Boot\bootx64.efi &amp;&amp; mountvol S: /D</CommandLine>
          <Description>Copy UEFI boot manager to the firmware fallback path for NVRAM-independent boot</Description>
        </SynchronousCommand>
        <!-- 4. リモートデスクトップ(RDP)の接続受け入れを有効にする。
             レジストリとファイアウォールの設定は sysprep /generalize 後も残るため、ベースイメージに
             RDP 有効の状態が入る(autounattend.xml は初回インストール時にしか適用されないため、
             specialize パスのコンポーネントではなく、sysprep より前のこのコマンドで設定する)。
             ファイアウォールのルールグループは、表示名がロケールで変わる(日本語では
             「リモート デスクトップ」)ため、ロケールに依存しないリソース参照で指定する。
             NLA(ネットワークレベル認証)は Windows Server の既定(有効)のままにする。
             なお、marmot はサーバー作成時にも同等の設定を行うため、二重設定になるが冪等で問題ない。 -->
        <SynchronousCommand wcm:action="add">
          <Order>4</Order>
          <CommandLine>cmd /c reg add "HKLM\SYSTEM\CurrentControlSet\Control\Terminal Server" /v fDenyTSConnections /t REG_DWORD /d 0 /f &amp;&amp; netsh advfirewall firewall set rule group="@FirewallAPI.dll,-28752" new enable=Yes</CommandLine>
          <Description>Enable Remote Desktop</Description>
        </SynchronousCommand>
        <!-- 5. sysprep 前に、残存する answer file を削除する。
             Windows Setup は、インストール時に使った autounattend.xml を
             C:\Windows\Panther\unattend.xml 等にコピーして保持する。このファイルを
             削除せずに sysprep /generalize すると、ベースイメージから作成した
             本番 VM の初回起動(specialize パス)でも同じ answer file が
             再度見つかり、再適用されてしまう(実機のログで
             「UnattendSearchExplicitPath: Found usable unattend file for pass
             [specialize] at [C:\Windows\Panther\unattend.xml]」として確認済み)。
             本番 VM にはビルド専用の virtio ドライバ ISO や cloudbase-init MSI の
             ドライブがアタッチされていないため、再適用された FirstLogonCommands
             (ドライバ導入・MSI インストール・2 回目の sysprep 呼び出し等)が
             正常に完了できず、specialize/OOBE パスの完了(GeneralizationState=7
             への遷移)が妨げられる場合がある。cloudbase-init はこの
             GeneralizationState=7 をタイムアウト無しで待ち続けるため、
             ホスト名・NIC 設定などのプロビジョニングがいつまでも実行されない
             原因になる。そのため、sysprep /generalize の直前で明示的に
             削除しておく(未存在の場合の削除失敗は無視してよいため 2>nul で
             抑制し、最後に終了コード 0 で終える)。 -->
        <SynchronousCommand wcm:action="add">
          <Order>5</Order>
          <CommandLine>cmd /c del /f /q "C:\Windows\Panther\unattend.xml" 2>nul &amp; del /f /q "C:\unattend.xml" 2>nul &amp; del /f /q "C:\Windows\System32\Sysprep\unattend.xml" 2>nul &amp; exit /b 0</CommandLine>
          <Description>Remove residual answer files before sysprep to prevent reapplication on deployed VMs</Description>
        </SynchronousCommand>
        <!-- 6. イメージの汎用化(sysprep /generalize)。これが最後のコマンドになること。
             cloudbase-init は、この sysprep によって汎用化されたイメージが次回起動時に
             通る specialize/OOBE パスにフックして動作する設計のため、
             このコマンド以降に他の処理を追加しないこと。 -->
        <SynchronousCommand wcm:action="add">
          <Order>6</Order>
          <CommandLine>C:\Windows\System32\Sysprep\sysprep.exe /generalize /oobe /shutdown /quiet</CommandLine>
          <Description>Generalize image with sysprep and shut down</Description>
        </SynchronousCommand>
      </FirstLogonCommands>
    </component>
  </settings>

</unattend>
```

### 注意点

- `wcm:keyValue`・ドライバパス（`D:\vioscsi\2k22\amd64` 等）・CD-ROM のドライブレター（`D:`/`E:`）は、
  実際にアタッチした CD-ROM の順序によって変わります。一度手動インストールを試して
  `Shift+F10` でコマンドプロンプトを開き、`Get-Volume` 等で実際のドライブレターを確認してください。
- virtio-win ISO 内のドライバフォルダ名はバージョンによって変わります。Windows Server 2025 用の
  専用フォルダ（`2k25` 等）が無い場合は、`2k22` 用ドライバで動作するか virtio-win の
  リリースノートを確認してください。
- `/IMAGE/INDEX` の値は、ISO に含まれるエディション（Standard/Datacenter、Desktop
  Experience の有無）によって異なります。ビルド用ホストで
  `dism /Get-WimInfo /WimFile:D:\sources\install.wim` を実行して確認してください。
- `AdministratorPassword`/`AutoLogon` に設定する値は、**ベースイメージ作成作業中にのみ使われる
  一時的なパスワード**です。`sysprep /generalize` によってイメージが汎用化されるため、
  この平文パスワードはイメージに残りません。Phase 3 で cloudbase-init により実運用の
  Administrator パスワードがランダム生成・設定されます。
- **【重要】sysprep 前に残存 answer file(`C:\Windows\Panther\unattend.xml` 等)を削除すること**:
  Windows Setup はインストールに使った `autounattend.xml` を `C:\Windows\Panther\unattend.xml`
  等にコピーして保持します。これを削除せずに `sysprep /generalize` すると、ベースイメージから
  作成した本番 VM の初回起動(specialize パス)でも同じ answer file が再度見つかり、再適用されて
  しまいます。本番 VM にはビルド専用の virtio ドライバ ISO や cloudbase-init MSI のドライブが
  アタッチされていないため、再適用された `FirstLogonCommands`(ドライバ導入・MSI インストール・
  2 回目の `sysprep` 呼び出し等)が正常に完了できず、specialize/OOBE パスの完了
  (`GeneralizationState=7` への遷移)が妨げられ、cloudbase-init がホスト名・NIC 設定などの
  プロビジョニングを永久に実行できなくなる場合があります(タイムアウト無しで待ち続けるため)。
  上記 `autounattend.xml` の `FirstLogonCommands` の Order 5 はこの対策で、`sysprep` の直前に
  残存 answer file を削除します。**この手順を含めずに作成したイメージでホスト名・NIC 設定が
  反映されない場合は、ゲスト内で `C:\Windows\Panther\unattend.xml` の有無と
  `cloudbase-init.log` の `Waiting for sysprep completion. GeneralizationState: 4` の
  繰り返しを確認してください。**
- **【重要】`FirstLogonCommands` の各コマンドは終了コードを必ず 0 にすること**:
  `<SynchronousCommand>` は 0 以外の終了コードを返すと、Windows Setup が**それ以降の
  `FirstLogonCommands` を一切実行せずに、無言でユーザーのデスクトップへ制御を渡してしまう**
  (エラーダイアログ等は出ないことが多い)。実機で、Order 1(`pnputil /add-driver` の
  ワイルドカード一括導入は、一部ドライバが適用対象外の場合に 0 以外を返すことがある)や
  Order 2(`msiexec /i ... /norestart` は、再起動が必要な場合に `ERROR_SUCCESS_REBOOT_REQUIRED`
  (終了コード `3010`、実質成功)を返すことがある)が原因で、それ以降の再起動・cloudbase-init
  の仕上げ処理・`sysprep` が一切実行されず、cloudbase-init サービス自体は起動しているのに
  `GeneralizationState` が `4` のまま進まなくなる事象を確認した。そのため、上記
  `autounattend.xml` の Order 1・Order 2 は `cmd /c <コマンド> & exit /b 0` の形で
  終了コードを強制的に `0` にしている。新たに `FirstLogonCommands` を追加する場合も、
  必ずこのパターンを踏襲すること。
- **【重要】UEFI 起動エントリ（NVRAM）は新しいドメインに引き継がれません**:
  Windows Setup は通常、UEFI ブートマネージャーを ESP 上の `\EFI\Microsoft\Boot\bootmgfw.efi`
  にのみ書き込み、そこを指す起動エントリを「インストールに使ったビルド用 VM 専用の」
  NVRAM（libvirt では `OVMF_VARS` ファイル、ドメインごとに別ファイルとして自動生成される）
  に登録します。marmot はベースイメージ（qcow2）を**新しいドメインとして**登録・起動するため、
  ドメインごとに別の空の NVRAM が自動生成され、ビルド用 VM の NVRAM 起動エントリは
  一切引き継がれません。この状態で起動すると、ディスクには正しく Windows がインストール
  されているにもかかわらず、ファームウェアがどこから起動すべきか分からず
  **BIOS/UEFI 画面（起動デバイス未検出）で停止します**。
  上記 `autounattend.xml` の `FirstLogonCommands` の Order 3（`mountvol` + `copy`）は、
  ファームウェアの標準フォールバックパス（`\EFI\Boot\bootx64.efi`、通常はリムーバブル
  メディア/USB インストール時のみ使われる場所）にもブートマネージャーを複製することで、
  NVRAM に起動エントリが無い新規ドメインでも自動検出・起動できるようにする対策です。
  **この手順を含めずに作成したイメージは起動しません。** 既にこの対策を含めずに
  作成済みのイメージがある場合は、以下のいずれかで復旧してください。
  - ベースイメージ作成をやり直す（`autounattend.xml` に Order 3 を含めて再実行）。
  - 既存の qcow2 を一時的に Linux VM（または `guestfish`/`virt-customize` 等）で
    マウントし、ESP パーティション内で `EFI/Microsoft/Boot/bootmgfw.efi` を
    `EFI/Boot/bootx64.efi` としてコピーする。

## 2. ビルド用 VM の作成と無人インストールの実行

Phase 1 で実装した libvirt ドメイン設定（UEFI/TPM 2.0/virtio-scsi/QXL、
[pkg/virt/libvirtXml2.go](../pkg/virt/libvirtXml2.go)）と同じ構成で、`virt-install` を使って
ビルド用 VM を作成します。

```console
$ qemu-img create -f qcow2 /var/lib/libvirt/images/windows2022-build.qcow2 64G

$ virt-install \
    --name windows2022-build \
    --memory 4096 \
    --vcpus 2 \
    --os-variant win2k22 \
    --boot uefi \
    --tpm backend.type=emulator,backend.version=2.0,model=tpm-crb \
    --disk path=/var/lib/libvirt/images/windows2022-build.qcow2,bus=scsi,size=64 \
    --controller type=scsi,model=virtio-scsi \
    --network network=default,model=virtio \
    --graphics spice \
    --video model.type=qxl \
    --cdrom /path/to/WindowsServer2022.iso \
    --disk /path/to/virtio-win.iso,device=cdrom \
    --disk /path/to/autounattend-floppy.img,device=floppy \
    --noautoconsole
```

- `autounattend.xml` は、Windows Setup が自動検出できる場所（リムーバブルメディアのルート、
  または `virt-install` でフロッピー/CD-ROM として追加した仮想メディアのルート）に置きます。
  上記例ではフロッピーイメージとして渡す方法を示していますが、小さな ISO（`autounattend.iso`）
  として作成して追加の CD-ROM で渡しても構いません。
- cloudbase-init の MSI は、オフライン環境では別の小さな ISO（例: `cloudbase-init-assets.iso`）に
  格納し、追加の CD-ROM としてアタッチしてください（上記 autounattend.xml の `E:\` はこの ISO を想定）。
- `--os-variant win2k22` は Windows Server 2025 の場合は利用可能な最新の `osinfo-db` の値
  （例: `win2k25` 相当。`osinfo-query os | grep -i "2025"` で確認）に置き換えてください。
  対応する `os-variant` がまだ `osinfo-db` に無い場合は、近い一般的な値（例: `win2k22`）で
  代用しても動作上の問題はありません（`--os-variant` は最適化パラメータの選択に使われるのみで、
  UEFI/TPM/virtio-scsi/QXL 自体は明示的に指定しているため必須ではありません）。

インストールが進行すると、`autounattend.xml` の `FirstLogonCommands` が自動実行され、
cloudbase-init 導入 → RDP 有効化 → 残存 answer file の削除 →
`sysprep /generalize /oobe /shutdown` の順に実行されて VM が
自動的にシャットダウンします。`virsh list --all` で `shut off` になったことを確認してください。

```console
$ virsh list --all
 Id   Name                 State
----------------------------------
 -    windows2022-build    shut off
```

## 3. ベースイメージの確認

```console
$ qemu-img info /var/lib/libvirt/images/windows2022-build.qcow2
image: windows2022-build.qcow2
file format: qcow2
virtual size: 64 GiB (68719476736 bytes)
...
```

- `file format: qcow2` であることを確認します（marmot は QCOW2 のみ受け付けます。
  `validateQcowV2Image` によるマジックナンバー検証があります）。
- イメージが `sysprep /generalize` 済みであることを、実機確認済みの場合は手順書やチェックリストに
  明記しておくことを推奨します（marmot 側では汎用化済みかどうかの自動検証は行いません）。

## 4. marmot への登録

marmot は、ダウンロード/コピー後に OS 種別を判定して、Windows（NTFS）イメージに対しては
Linux 向けの `virt-customize` 処理や 16GB 固定リサイズを**実行しません**
（実施すると `NTFS` は resize 非対応のためエラーになります。この分岐は実装済みです）。
登録手順自体は、既存の Linux 系イメージと同じ `mactl image create` コマンドで行います。

### 登録用 YAML の作成

決定済みの形式（`docs/MEMO-windows-vm-support.md` Phase 2 参照）に従い、`sourceUrl` に
ビルドしたローカル QCOW2 ファイルのパスを `file://` スキームで指定します。

```yaml
# windows2022.yaml
apiVersion: v1
kind: Image
metadata:
    name: windows2022
spec:
    sourceUrl: file:///var/lib/libvirt/images/windows2022-build.qcow2
    osName: windows
    osVersion: "2022"
```

Windows Server 2025 の場合は `metadata.name`/`osVersion` を置き換えます。

```yaml
# windows2025.yaml
apiVersion: v1
kind: Image
metadata:
    name: windows2025
spec:
    sourceUrl: file:///var/lib/libvirt/images/windows2025-build.qcow2
    osName: windows
    osVersion: "2025"
```

`spec.size` は指定不要です。marmot がイメージの実際の仮想ディスクサイズを
`qemu-img info` から自動検出し、`Image.Spec.Size` に反映します。

### 登録コマンド

```console
$ mactl image create --configfile windows2022.yaml
{"id":"a1b2c","message":"Image creation request accepted"}
```

`sourceUrl` が `file://` の場合、marmot はネットワークダウンロードではなくローカルファイル
コピーを行います（ビルド用ホストと marmot ノードが同一マシンの場合はパスがそのまま使われます。
別マシンの場合は、事前に QCOW2 ファイルを marmot ノード上の参照可能なパスへ転送してください）。

### 登録状態の確認

```console
$ mactl image detail a1b2c --output json
```

`status.statusCode` が `AVAILABLE`（`3`）になれば登録完了です。`ERROR` になった場合は
`status.message` を確認してください。

```console
$ mactl image list
NAME          OS        VERSION   SIZE   STATUS
------------  --------  --------  -----  ---------
windows2022   windows   2022      64     AVAILABLE
windows2025   windows   2025      80     AVAILABLE
```

`osName`/`osVersion` の組み合わせでバージョンが識別できるため、`name` には
`windows2022`/`windows2025` のようにバージョンが分かる名前を付けることを推奨します。

## 5. 次のステップ

- 登録したイメージを使って `osVariant: windows2022`（または `windows2025`）を指定した
  サーバーを作成し、Phase 1 で対応した UEFI/TPM/virtio-scsi/QXL/SPICE 経由で VM が
  正しく起動することを確認してください（`mactl console --graphical` で接続情報を取得できます）。
- ユーザー/SSH キー/ホスト名の自動設定（cloudbase-init によるプロビジョニング）は
  Phase 3 の対応範囲です。現時点ではサーバー作成後、SPICE 経由のグラフィカルコンソールで
  手動ログイン・設定が必要です。

## 注意事項

- **評価版 ISO のライセンス制約**: 評価版（Evaluation）メディアは初回起動から 180 日の評価期間
  制限があります。`sysprep /generalize` を実行しても評価期限はリセットされません。
  ベースイメージとして繰り返し利用する運用には、正規ライセンス版メディアの使用を推奨します。
- **ボリュームサイズの上限**: marmot は OS ボリュームのサイズに 100GB の上限を設けています
  （`pkg/marmotd/volumes.go` の `requestedOSVolumeSizeGB`）。ベースイメージの仮想ディスクサイズは
  この上限を超えないようにしてください。
- **cloudbase-init インストーラの入手経路**: 本手順ではオフライン導入（事前ダウンロードした MSI を
  追加の CD-ROM で渡す方式）を前提としています。ビルド用ホストがインターネットに到達できる場合は、
  `FirstLogonCommands` 内で `Invoke-WebRequest` 等を使ってその場でダウンロードする方式に
  置き換えても構いません。
