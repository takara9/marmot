# Open-VPNクライアントのサポート

OpenVPMクライアントで、marmot内部の仮想ネットワークへ接続するためのゲートウェイ機能


## インストーラー
  - インストールパッケージ (.deb ファイル) を作成する際に、このVpnGatway インスタンスを作成する際に必要な Ansible playbook を /var/lib/marmot/ansible-playbooks に配置する様に同梱する。
  - インストール時に、既存のプレイブックは削除して、すべでコピーで作成する。
  - VpnGateway となる仮想サーバーの設定は、Ansibleのplaybookによって実施する。

## marmot起動時の準備作業
  - /etc/marmot/keys のディレクトリ下に、鍵ファイルがあれば、何もしない。
  - ディレクトリが無ければ作成して、ssh鍵ペアを生成して、ファイル public.key と private.key に保存する。


## オブジェクトの作成
  - mactl は、VPNゲートウェイ・リソースのファイル、または、URLからマニフェストを取得して、JSON形式に変換して、marmotd に送信する。
  - marmotd は、受けたVPNゲートウェイ・リソースの作成要求を、etcd に保存して、オブジェクトの作成は、VpnGatewayコントローラーに任せる。
  - 内部的には、他のオブジェクト同様に、uuidから導出した id で オブジェクトは識別する。 id の重複は許さない。
  - 仮想ネットワーク(internalVirtualNetwork上)で、同一名称は許さない


## VPNゲートウェイ・リソースのAPI
```
apiVersion: v1
kind: VpnGateway
metadata:
    name: vpn-gw
spec:
    bindPublicIpAddress: 192.168.1.100  # パブリック側のIPアドレスでアクセスを許可　ネットワークは host-bridge 固定
    internalVirtualNetwork: admin-net   # 内部側の仮想ネットワーク
    remoteCIDR: 192.168.1.200/24        # 接続を許すリモートのIPアドレス、省略時は、0.0.0.0/0 で何処からでも受け入れる
```


## VpnGatewayコントローラーにより、オブジェクトの作成、変更、削除
  - 15 秒間隔で、制御ループを実行
  - VPNゲートウェイ・オブジェクトの作成
    - 作成されていないVPNゲートウェイ・オブジェクトを発見したら、以下の動作を実施する。
        - オブジェクトの作成は、次のマニフェストから作成される etcd 内の JSONデータの情報を取得して実行する。
        - OS: ubuntu24.04
        - CPU: 1
        - Memory: 2048
        - ssh認証の秘密鍵は、/etc/marmot/keysに保存された public.keyを root ユーザーにセットする
        - インターフェースが接続する外側ブリッジは、host-bridge に固定して、bindPublicIpAddress を割当
        - internalVirtualNetworkを作る内側ブリッジに接続する。
        - 内部用ブリッジとは、デフォルトで作成するdefault, host-bridge, ovs-network 以外の marmotd が作成したブリッジを指す。
        - 内部用ブリッジに接続するIPアドレスは、x.x.x.1 を割り当てる。xは任意の数字
        - オブジェクトの仮想サーバーが起動したら、次に ansible で /var/lib/marmot/ansible-playbooks に保存したansible playbook を使ってVPNゲートウェイの仮想サーバーをセットアップする。
        - ansibleで設定に必要な秘密鍵は、/etc/marmot/keys のディレクトリ下にある private.keyを使用する。
        - ansible を使った設定が失敗したら、次の制御ループで、リトライを繰り返す。もし、5回を超えて失敗したら、オブジェクトの作成を中止して、Status.statusをFAILED状態として、Status.messageに原因を記録する。
        - Status.statusの状態が遷移した時は、Status.status.message を nil にする。
        - 仮想のラベルには、"managedBy": "vpn-gateway-controller" をセットして、コントローラー管理下であることを区別できるようにする。
        - bindPublicIpAddress, remoteCIDR は、IPv4, IPv6 に対応する。 ansible playbook も 両者に対応しなければならない。

  - ゲートウェイ・オブジェクトの削除
    - deleteionTimestamp が作成されてから、15秒以上経過したオブジェクトは、削除処理を実施する。
    - 稼働サーバーを削除して、etcdのデータをクリアして削除完了となる。

  - ゲートウェイ・オブジェクトの変更
  　- 変更は、remoteCIDRに限定する。

## VpnGateway のクライアント用接続ファイルのダウンロード
mactl get vpn-cert <vpn-gwの名前> で、ダウンロードできるようにする。


## ansible playbookで実行する仮想マシンの設定
以下は、VpnGateway用に起動した Ubuntu 24.04 の設定です。ansibleのplaybook に書き直して、実施する予定です。

1. easy-rsa で PKI（証明書環境）を構築

```bash
sudo apt update
sudo apt install -y openvpn easy-rsa

# easy-rsa の作業ディレクトリを作成
make-cadir ~/easy-rsa
cd ~/easy-rsa

# PKI を初期化
./easyrsa init-pki

# CA を作成（パスフレーズとCommon Nameを設定）
./easyrsa build-ca

# サーバー証明書を作成
./easyrsa gen-req server nopass
./easyrsa sign-req server server

# DH パラメータを生成（少し時間がかかる）
./easyrsa gen-dh

# TLS 認証キーを生成
openvpn --genkey secret ~/easy-rsa/pki/ta.key
```

2. 証明書・鍵を OpenVPN ディレクトリにコピー

```bash
sudo cp ~/easy-rsa/pki/ca.crt          /etc/openvpn/server/
sudo cp ~/easy-rsa/pki/issued/server.crt /etc/openvpn/server/
sudo cp ~/easy-rsa/pki/private/server.key /etc/openvpn/server/
sudo cp ~/easy-rsa/pki/dh.pem          /etc/openvpn/server/
sudo cp ~/easy-rsa/pki/ta.key          /etc/openvpn/server/
```

3. サーバー設定ファイルを作成

以下の場所にファイルを作成する。
```bash
sudo vim /etc/openvpn/server/server.conf
```

このファイルの中で３箇所置き換えを実施する。
- push "route 仮想ネットワークに割当られたネットワークアドレス 255.255.255.0"
- push "dhcp-option DNS marmotdを実行するホストのパブリック側のIPアドレス"


```file:server.conf
port 1194
proto udp
dev tun

ca   /etc/openvpn/server/ca.crt
cert /etc/openvpn/server/server.crt
key  /etc/openvpn/server/server.key
dh   /etc/openvpn/server/dh.pem

tls-auth /etc/openvpn/server/ta.key 0

# 以下のアドレスは、VPNトンネル用に固定します。
server 10.8.0.0 255.255.0.0

# このアドレスは、仮想ネットワークのIPネットワークのアドレスに置き換えます。
push "route 172.16.30.0 255.255.255.0"

ifconfig-pool-persist /var/log/openvpn/ipp.txt

# クライアントにデフォルトゲートウェイを通知する場合
# push "redirect-gateway def1 bypass-dhcp"

# DNSは、marmotd が稼働するホストのIPアドレスに置き換えます。
push "dhcp-option DNS 192.168.1.201"

keepalive 10 120
cipher AES-256-GCM
user nobody
group nogroup
persist-key
persist-tun

status  /var/log/openvpn/status.log
log     /var/log/openvpn/openvpn.log
verb 3
```

4. IP フォワーディングを有効化

```bash
# 即時有効化
sudo sysctl -w net.ipv4.ip_forward=1

# 再起動後も有効にする
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf
```

5. OpenVPN サービスを起動
```bash
sudo mkdir -p /var/log/openvpn

sudo systemctl enable openvpn-server@server
sudo systemctl start  openvpn-server@server

# 状態確認
sudo systemctl status openvpn-server@server
```

6. クライアント証明書の作成
```bash
cd ~/easy-rsa

# クライアント証明書を作成（user1 の例）
./easyrsa gen-req user1 nopass
./easyrsa sign-req client user1
```

7. クライアント用 .ovpn ファイルを生成
- ファイル名は、接続する仮想ネットワークの名前で置き換える。
- remoteのIPアドレスは、spec.bindPublicIpAddress を使用する。

```bash
cat > ~/${spec.internalVirtualNetwork}.ovpn << EOF
client
dev tun
proto udp
remote ${spec.bindPublicIpAddress} 1194
resolv-retry infinite
nobind
persist-key
persist-tun
cipher AES-256-GCM
verb 3
key-direction 1

<ca>
$(cat ~/easy-rsa/pki/ca.crt)
</ca>
<cert>
$(cat ~/easy-rsa/pki/issued/${spec.internalVirtualNetwork}.crt)
</cert>
<key>
$(cat ~/easy-rsa/pki/private/${spec.internalVirtualNetwork}.key)
</key>
<tls-auth>
$(cat ~/easy-rsa/pki/ta.key)
</tls-auth>
EOF
```

この ファイル ${spec.internalVirtualNetwork}.ovpn は、/var/lib/marmot/vpn の下に保存する。
mactl get vpn-cert <vpn-gwの名前> で、ダウンロードできるようにする。


ファイアウォール設定（UFW使用の場合）
```bash
sudo ufw allow 1194/udp
sudo ufw reload
```

動作確認
```bash
# tun0 インターフェースが作成されているか確認
ip addr show tun0

# 接続クライアントの確認
sudo cat /var/log/openvpn/status.log
```





## 接続対象のサーバーのルーティング設定

spec.vpnAccess = true が設定された 仮想サーバーは、生成時に
iPNetworkAddressで指定したアドレスの 1 を VPNトンネルのGWとして割り当てる。

```network-6.yaml 
apiVersion: v1
kind: VirtualNetwork
metadata:
    name: admin-net
    comment: VPNクライアントが使用するIPネットワークを持った仮想ネットワーク
spec:
    iPNetworkAddress: 172.16.0.0/24
    forwardMode: bridge
    vpnAccess: true
```

上記の admin-net に接続する仮想サーバーは、以下の設定に相当するnetplanの設定をいれる。

```
sudo ip route add 10.8.0.0/24 via 172.16.0.1
```


