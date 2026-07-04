# インストール直後に host-bridge が存在しない場合の作成方法

marmot の deb ファイルをインストールして、起動した直後では、host-bridgeが存在しない。
そのため、host-bridgeは、手作業で作成する必要がある。

```console
root@hv3:/home/ubuntu# virsh net-list
 Name      State    Autostart   Persistent
--------------------------------------------
 default   active   yes         yes
```

## XMLファイルの作成

host-bridge を virsh で作成するための XML ファイルを作成します。

host-bridge.xml 
```xml
<network>
  <name>host-bridge</name>
  <forward mode='bridge'/>
  <bridge name='br0'/>
</network>
```

## コマンドの投入とブリッジの活性化

virsh コマンドで、以下のコマンドで、host-bridge を定義、開始、自動開始の設定を実施します。

```console
# virsh net-define host-bridge.xml 
# virsh net-start host-bridge
# virsh net-autostart host-bridge
Network host-bridge marked as autostarted

root@hv3:/home/ubuntu# virsh net-list
 Name          State    Autostart   Persistent
------------------------------------------------
 default       active   yes         yes
 host-bridge   active   yes         yes
```

## host-bridge を marmot に認識させる

marmot を再起動することで、host-bridge が認識されます。

```console
# systemctl restart marmot
```

確認コマンドを実行します。

```
root@hv3:/home/ubuntu# mactl login admin
Password: 
Successfully logged in as admin
⚠️  You must change your password before using other commands.
   Run: mactl passwd
root@hv3:/home/ubuntu# mactl get net
NAME            NODE       BRIDGE        STATUS        AGE       IP-NET        
----            ---------  -----------   ----------    ---       --------------
default         hv3        virbr0        ACTIVE        9h        -             
host-bridge     hv3        br0           ACTIVE        35s       -         
```

