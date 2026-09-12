## default ネットワークに接続する仮想サーバーの起動に失敗した時の対処方法

```bash
ubuntu@ws1:~/marmot-manifests/01-simple-server$ mactl get server
NAME             NODE          STATUS        CPU  RAM(MB)  IP-ADDRESS       NETWORK          AGE
----             ----          ------        ---  -------  ----------       -------          ---
ceph-single      hv0           RUNNING       4    12208    192.168.1.170    host-bridge      17d
server-01        hv0           ERROR         1    1024     N/A              default          44s
ubuntu@ws1:~/marmot-manifests/01-simple-server$ mactl get server server-01 -o json  |jq .[].status
{
  "creationTimeStamp": "2026-09-12T22:41:42.949071134Z",
  "lastUpdateTimeStamp": "2026-09-12T22:41:51.763507958Z",
  "message": "サーバーのプロビジョニングに失敗した。原因エラー: failed to define/start virtual network default: virError(Code=38, Domain=0, Message='error creating bridge interface virbr0: File exists')",
  "status": "ERROR",
  "statusCode": 4
}
```

## virbr0 が何処に作られているか確認する

```bash
root@hv0:/home/ubuntu# virsh net-list
```
default が表示されなければ、以下のコマンドで、OVSかLinux Bridgeか確認する。


```bash
root@hv0:/home/ubuntu# ovs-vsctl show
root@hv0:/home/ubuntu# brctl show
```

OVS管理下にブリッジが作られていたら、コマンドで削除する

```bash
# ポートが無いことを確認（既に internal port のみのはず）
ovs-vsctl list-ports virbr0

# OVS 側の virbr0 ブリッジを削除
ovs-vsctl del-br virbr0

# 削除できたか確認
ovs-vsctl show
```

削除後の再試行

 ```bash
 # libvirt側から直接起動を試す場合
virsh net-start default

# もしくは marmot 側にサーバー作成をリトライさせる場合
mactl apply -f <server-01のマニフェスト>
```

