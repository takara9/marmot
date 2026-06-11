# Docker コンテナを仮想マシンと同じように動かす


コンテナに OVSを接続できますか？

はい、できます。

むしろ技術的にはかなり昔から行われています。

Kubernetesやコンテナの世界では一般的にLinux BridgeやOverlay Network（VXLANなど）が使われますが、コンテナのvethを直接 **Open vSwitch** に接続することも可能です。

構成としては、

```text
Container
   │
 veth
   │
Open vSwitch
   │
 VLAN/VXLAN
   │
Physical Network
```

になります。

---

## 方法1: Docker + OVS

最も単純です。

コンテナ作成後に、

```bash
ovs-vsctl add-port br-int veth1234
```

のようにvethをOVSへ接続します。

実験レベルなら簡単です。

---

## 方法2: Kubernetes + OVS CNI

KubernetesではCNIプラグインを利用します。

代表例は

* OVN-Kubernetes
* OpenShift のOVNネットワーク
* Antrea

です。

最近は特にOVN系が主流です。

構成は、

```text
Pod
 ↓
OVS
 ↓
OVN
 ↓
Physical Network
```

になります。

---

## 方法3: SR-IOV + OVS

性能重視の場合です。

```text
Container
 ↓
VF
 ↓
OVS
 ↓
NIC
```

のように接続します。

通信性能は非常に高くなります。

AIクラスタやNFV基盤で使われています。

---

## Marmot視点で面白い点

実はここがMarmotと相性が良い部分です。

VMは通常、

```text
VM
 ↓
virtio-net
 ↓
OVS
```

です。

コンテナも

```text
Container
 ↓
veth
 ↓
OVS
```

にすると、

Marmotから見ると、

```text
Workload
 ↓
Virtual NIC
 ↓
OVS Port
```

で統一できます。

つまり、

* VM
* コンテナ

の違いをネットワークレイヤでほぼ隠せます。

---

## Marmotで狙うなら

個人的には、

```text
Marmot
  ↓
Open vSwitch
    ├─ VM
    ├─ Container
    └─ Bare Metal
```

という構成はかなり筋が良いと思います。

なぜなら、

OpenStackもoVirtもKubeVirtも最終的には

> 仮想NICをどこへ接続するか

という問題に帰着するからです。

もしMarmotが

* VM管理
* コンテナ管理
* 将来的なベアメタル管理

を統一したいなら、

OVSを共通ネットワーク基盤として採用するのは自然な設計です。

実際、OpenStackのネットワーク（Neutron + OVS）やOVNの設計思想とかなり近い方向になります。そうすると利用者には、

```yaml
network: production
ip: 10.0.0.10
```

だけ見せて、

裏側ではVMでもコンテナでも同じOVSに接続する、という抽象化が可能になります。


DockerコンテナとOVSの接続は、Marmotの構想を考える上でかなり重要なポイントです。

基本的な考え方は、

```text
Container
  ↓
eth0
  ↓
veth pair
  ↓
Open vSwitch
  ↓
Physical Network
```

です。

実はVMの

```text
VM
 ↓
virtio-net
 ↓
tap
 ↓
Open vSwitch
```

と非常によく似ています。

---

# Linuxのvethとは

コンテナには通常、

```bash
docker run nginx
```

すると、

```text
Container Namespace
 └─ eth0
```

が作られます。

実際には

```text
eth0 <====> vethXXXX
```

というペアです。

片側がコンテナ内、

片側がホスト側です。

確認すると

```bash
ip link
```

で

```text
veth2a3f4b
veth9c8d7e
```

のようなデバイスが見えます。

---

# Docker標準ネットワーク

通常は

```text
Container
 ↓
veth
 ↓
docker0 (Linux Bridge)
 ↓
Host NIC
```

です。

DockerはLinux Bridgeを使います。

---

# OVSへ変更する

Linux Bridgeの代わりにOVSへ接続します。

まずOVSブリッジを作成します。

```bash
ovs-vsctl add-br br-int
```

確認

```bash
ovs-vsctl show
```

---

# コンテナのPID取得

```bash
docker inspect nginx
```

または

```bash
docker inspect -f '{{.State.Pid}}' nginx
```

例

```text
12345
```

---

# コンテナのNamespaceへアクセス

```bash
mkdir -p /var/run/netns

ln -s \
 /proc/12345/ns/net \
 /var/run/netns/nginx
```

---

# vethペア作成

```bash
ip link add veth-host type veth peer name veth-cont
```

構成

```text
veth-host <====> veth-cont
```

---

# コンテナへ移動

```bash
ip link set veth-cont netns nginx
```

コンテナ内で

```bash
ip netns exec nginx \
 ip link set veth-cont name eth1
```

---

# OVSへ接続

ホスト側

```bash
ovs-vsctl add-port br-int veth-host
```

起動

```bash
ip link set veth-host up
```

コンテナ側

```bash
ip netns exec nginx \
 ip link set eth1 up
```

---

# IP設定

例えば

```bash
ip netns exec nginx \
 ip addr add 192.168.10.100/24 dev eth1
```

---

結果

```text
Container
 └─ eth1
      │
      ▼
veth-cont
      │
      ▼
veth-host
      │
      ▼
OVS(br-int)
      │
      ▼
Physical Network
```

になります。

---

# VLANも簡単

OVSなので

```bash
ovs-vsctl set port veth-host tag=100
```

だけです。

VMと同じです。

```text
Container
 ↓
OVS Port
 ↓ VLAN100
 ↓
Network
```

---

# Marmotで面白い点

Marmotの世界観なら、

VM

```text
VM
 ↓
tap
 ↓
OVS
```

コンテナ

```text
Container
 ↓
veth
 ↓
OVS
```

です。

つまりOVSから見ると

```text
Port
 ├─ tapXXX
 └─ vethXXX
```

の違いしかありません。

そのためMarmotは

```yaml
network: production
vlan: 100
```

だけ受け取り、

内部で

* VMならtapを接続
* コンテナならvethを接続

という実装ができます。

これはOpenStackのNeutronがやっていることに近いですが、Marmotならもっとシンプルにできるかもしれません。

さらに進めると、最近のOVNではコンテナとVMを同じ論理スイッチに収容できます。

```text
OVN Logical Switch
 ├─ VM
 ├─ VM
 ├─ Container
 └─ Container
```

です。

もしMarmotが「VMとコンテナを同じWorkloadとして扱う」ことを目指すなら、ネットワークについては **OVS/OVNを共通基盤にするのが最も自然な設計**だと思います。むしろネットワークは、ストレージより先に統一できる部分かもしれません。


---

その認識はかなり本質的だと思います。

「コンテナをVMと同じように扱う」と言うと、多くの人は起動方法ばかり考えますが、実際には運用面での差異が問題になります。

Marmotが目指すなら、特に以下の3つが大きな壁です。

## 1. ネットワーク

VMの場合

```text
VM
 └─ vNIC
      └─ VLAN
           └─ IP
```

非常に分かりやすいです。

運用者は

* IPアドレス
* VLAN
* ルーティング

を管理します。

一方コンテナは

```text
Pod
 └─ Overlay Network
      └─ Service
           └─ Ingress
```

となり、運用者から見ると複雑です。

Marmotでは、

```text
Workload
 └─ Network
      └─ IP
```

に統一した方が良いかもしれません。

つまり内部が

* VMならブリッジ
* コンテナならCNI

でも、

利用者には

```yaml
network: frontend
ip: 10.0.0.10
```

だけ見せる。

---

## 2. ストレージ

VM

```text
disk.qcow2
```

コンテナ

```text
image layers
```

思想が全く違います。

VMは

* 永続化前提

コンテナは

* 破棄前提

です。

しかし業務システムでは、

実際にはコンテナも永続化したい。

そのため

```text
Workload
 └─ Root Disk
```

という概念を作り、

内部的には

* qcow2
* OCI image

どちらでも良い

という抽象化が考えられます。

---

## 3. ボリューム

これは最も重要かもしれません。

VM

```text
VM
 └─ vDisk
```

コンテナ

```text
Pod
 └─ PVC
       └─ PV
```

です。

Kubernetes初心者が最初に苦労するのは、

実はネットワークよりPVCです。

Marmotなら、

```yaml
volumes:
  - db-data
```

だけ記述し、

内部で

* Ceph
* NFS
* Local
* iSCSI

を吸収する方が分かりやすいでしょう。

---

## 実は4つ目の課題がある

私なら、

3つに加えて

### ライフサイクル

を重視します。

VM

```text
create
start
stop
snapshot
clone
migrate
```

が当たり前です。

しかしコンテナは

```text
deploy
delete
redeploy
```

思想です。

ここが大きく違います。

---

例えば運用者は

```text
障害が起きた
↓
昨日の状態へ戻したい
```

と考えます。

VMならスナップショットです。

コンテナなら再デプロイです。

考え方が違います。

---

もしMarmotが

> VMとコンテナを同じように扱う

なら、

実はネットワーク・ストレージ・ボリューム以上に、

**ライフサイクルを統一すること**

が差別化になるかもしれません。

例えば、

```text
marmot snapshot
marmot clone
marmot migrate
```

がVMでもコンテナでも同じように使える。

これはKubernetesにも、従来の仮想化基盤にもない発想です。

そして、その方向はあなたが以前話していた「運用者目線の基盤」という考え方とも整合しています。

Marmotは「コンテナ基盤」ではなく、「運用対象（Workload）基盤」として設計すると、VM中心の思想を保ちながらコンテナを自然に取り込めるように思います。
