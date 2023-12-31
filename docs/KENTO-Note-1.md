# Marmot の改善検討ノート

## 処理の概要

1. コンフィグファイルを読み込む
1. メモリ、CPU、ストレージの空きリソースのHVホストを探す
1. 対象HVホストで、VM起動用のテンプレートからイメージを作る
1. 対象HVホストで、IPアドレス、ホスト名を設定、鍵を設定する
1. 対象HVホストで、VMを起動して、Ansible Playbookを適用する


## (1) コンフィグファイルを読み込む


GO言語で、コマンドラインの引数からのコンフィグファイル名を受け取る方法の確認が必要
GO言語で、YAMLを読む処理は、問題なし。




## (2) メモリ、CPU、ストレージの空きリソースのHVホストを探す

etcdにアクセスして、該当データを探す処理が必要
現行のデータ設計を確認しないと解らない。

GO言語でetcdデータベースへのアクセスは、検証済




## (3) 対象HVホストで、VM起動用のテンプレートからイメージを作る

1. lvmの操作で、LVMのテンプレートイメージから、起動用スナップショットは可能



## (4)対象HVホストで、IPアドレス、ホスト名を設定、鍵を設定する
2. ストレージをローカルにマウントして、コンフィグを書き換える処理の案


~~~
イメージのボリューム内のパーティションをマッパーに追加する
kpartx -av /dev/vg1/lv01

作業用ディレクトリにマウントして、ファイルを操作する
mount -t ext4 /dev/mapper/vg1-lv01p2 /mnt

以下のファイルを変更する
/mnt/etc/hostname
/mnt/etc/netplan/00-installer-config.yaml
鍵の配置

kpartx -d /dev/vg1/lv01
~~~
参考資料、http://www.microhowto.info/howto/mount_a_partition_located_inside_a_file_or_logical_volume.html

※ Go言語の中から、コマンド実行する方法


3. コントロールから、ハイパーバイザーへコマンド送信するルートが欲しい。現行はsshで実行



## (5) 対象HVホストで、VMを起動して、Ansible Playbookを適用する

Go言語からコマンド操作する方法



