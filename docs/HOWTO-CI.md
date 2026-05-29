# GitHub Actions で Nested VMで テストを実施する方法

仮想サーバー上で LVM と libvirt を実行して、marmotを実行する方法です。


## marmotの仮想サーバーとAnsibleプレイブック

仮想マシンの定義とセットアップ用の Ansible playbookは、以下のレポジトリにあります。

　- https://github.com/takara9/marmot-servers/tree/main/selfhosted-vm

このレポジトリのREADME.mdに従って、仮想マシンを起動します。


## Nested VMのための Network設定

起動した仮想マシンの仮想ネットワークの設定を変更します。

[ネットワーク設定メモ](network-setup.md)



## 仮想サーバーの起動とセットアップ

ランナー上では、各ジョブ開始時に OVN/OVS ランタイムと libvirt ネットワークを再構成できる状態にします。

```bash
sudo -E env "PATH=$PATH" ./tools/setup-libvirt-networks.sh
virsh net-list --all
```

ジョブ終了時は、次ジョブに状態を持ち越さないよう必ずクリーンアップします。

```bash
sudo -E env "PATH=$PATH" ./tools/cleanup.sh
sudo -E env "PATH=$PATH" ./tools/teardown-libvirt-networks.sh
```


## ランナーの起動方法

```
$ ssh hvc
$ screen
$ cd actions-runner/
$ ./run.sh
```
