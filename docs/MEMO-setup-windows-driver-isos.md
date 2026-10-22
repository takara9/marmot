# Windows Server のインストール用 ISOファイルから、qcow2を作るシェル

tools の下に Windows Server インストールイメージから、ドライバー等をインストールした qcow2ファイルを作成するシェルを作成する。


1. 実行前の準備
1-1. カレントディレクトリの isos に配下に、windowsインストール用のiso形式のファイルが存在すること。
これらのうち、一つファイルをMarmotのユーザーが自己責任で入手して、ファイルをココに置くこと。
- Win2k22の場合: SERVER_EVAL_x86FRE_jq-jp.iso
- Win2k25の場合: 26100.32230.260111-0550.lt_release_svc_refresh_SERVER_EVAL_x64FRE_ja-jp.iso


1-2. カレントディレクトリの isos に配下に、virtio-win ドライバ ISO の stable 版が、配置されていること。
このファイルは、以下のURLからMarmotのユーザーが自己責任で入手して、ファイルをココに置くこと。
 https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso

1-3. カレントディレクトリの unattend の配下に、以下のファイルが配置されていること。
- autounattend.xml  
- ユーザーの責任で https://cloudbase.it/downloads/CloudbaseInitSetup_Stable_x64.msi からダウンロードされた インストール実行形式

2. assets.iso イメージの作成
unattendのディレクトリに存在するファイルをルートディレクトリに存在する isosディレクトリの配下に、assets.iso というファイル名のISOイメージを作成する。

3. 実行開始
sudo virt-install \
    --name windows2022-build \
    --memory 8192 \
    --vcpus 4 \
    --os-variant win2k22 \
    --boot uefi \
    --tpm backend.type=emulator,backend.version=2.0,model=tpm-crb \
    --disk path=/home/ubuntu/windows2022-build.qcow2,bus=scsi,size=40 \
    --controller type=scsi,model=virtio-scsi \
    --network network=default,model=virtio \
    --graphics spice \
    --video model.type=qxl \
    --cdrom /home/ubuntu/isos/SERVER_EVAL_x64FRE_ja-jp.iso \
    --disk /home/ubuntu/isos/virtio-win.iso,device=cdrom \
    --disk /home/ubuntu/isos/assets.iso,device=cdrom \
    --noautoconsole
