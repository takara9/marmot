# ネットワークブロックストレージの設計メモ


## ボリュームコントローラーに機能を追加

### 前提条件
- 機能はロジカルボリュームに限定
- 仮想マシンには、iSCSIのイニシエーターIDがセットされている。






### ブロックストレージ作成のプロセス

#### ボリュームの作成要求を受けた時の処理
- APIでリモートアクセスのブロックストレージ作成共有を受ける。条件は、LVMで、データディスクであること
- ロジカルボリュームを作成する　（既存の処理を流用すること）
- ターゲットのバックストアーを作成 
  - "targetcli /backstores/block create <disk-volumeId(バックストア名)> <デバイス名>"
  - バックストア名は、disk-volumeIdとする
  - 上記で作成した論理デバイス名で "/dev/vg2/lv01" など
- ターゲットを作成
  - "targetcli /iscsi create iqn.2024-01.com.marmot:<target-仮想マシンID>"
  - 仮想マシン（クライアント）が認識するアドレス。末尾":"以降が名前で　target-仮想マシンIDをセットする。
  - target-VMIdとするのが良い
- ターゲットとバックストアを対応付け、LUNを作成
　- "targetcli /iscsi/iqn.2024-01.com.marmot:<target-vmid>/tpg1/luns create /backstores/block/<disk-volumeid>"
  - 第一パラメータ "/iscsi/iqn.2024-01.com.marmot:<target-仮想マシンID>/tpg1/luns"
  - 第二パラメータ "/backstores/block/<disk-volumeid>"
- アクセス許可先のイニシエーターのIQNを設定
  - "targetcli /iscsi/iqn.2024-01.com.marmot:<target-vmid>/tpg1/acls create <イニシエーターID>"
    - クラスタメンバーの情報に、イニシエーターIDを取得して保存しておく。
    - クラスターメンバーの全イニシエーターIDを登録する。
- 設定を保存


#### VM起動時のイニシエーターの設定

- ドメインを作成する時のXMLファイルに、DISK設定を追加

```
    <disk type='network' device='disk'>
      <driver name='qemu' type='raw' cache='none' io='native'/>
      <source protocol='iscsi' name='iqn.2024-01.com.marmot:target-user1/0'>
        <host name='192.168.1.210' port='3260'/>
        <initiator>
          <iqn name='iqn.2004-10.com.ubuntu:01:c1b5e3a5db'/>
        </initiator>
      </source>
      <target dev='vdb' bus='virtio'/>
      <address type='pci' domain='0x0000' bus='0x07' slot='0x00' function='0x0'/>
    </disk>
```

- <source protocol='iscsi' name='iqn.2024-01.com.marmot:target-user1/0'>  ターゲット名
- <host name='192.168.1.210' port='3260'/> iSCSIダーゲットのIPアドレスとポート番号
- <iqn name='iqn.2004-10.com.ubuntu:01:c1b5e3a5db'/> marmotd が稼働するVMホストのイニシエーター名
- <target dev='vdb' bus='virtio'/> 他のデバイスと重ならないこと

上記の設定を実施して、仮想サーバーを起動する。





    - 認証のためのシークレットを指定
    - iSCSIのターゲット名＋LUN番号をセット
    - iSCSIターゲットのIPドレスポート
    - 自己のiSCSIイニシエーター名をセット
- 