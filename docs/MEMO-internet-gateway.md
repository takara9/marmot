# internet-gateway / internal-gateway / external-gateway / network-gateway

外部に面する仮想ネットワーク（ブリッジ）と内部の仮想ネットワーク（ブリッジ）を繋ぐリソース


- インストールパッケージ (.deb ファイル) を作成する際に、このGatway インスタンスを作成する際に必要な Ansible playbook を /var/lib/marmot/ansible-playbooks に配置する様に同梱する。
- インストール時に、既存のプレイブックは削除して、すべで、コピーを再実施する。
- Ansibleのplaybookは、仮想サーバーで構成する Gateway オブジェクトを設定するために使用する。
- Ansible playbook で、仮想サーバーを設定する際に、ssh 公開鍵のペアが必要となるため、以下の要領で作成する。

- 起動時の準備作業
  - /etc/marmot/keys のディレクトリ下をチェックして、public.key と private.key の存在をチェックする。
  - 上記ディレクトリが存在しない場合は、ディレクトリを作成して、ssh鍵ペアを生成して、ファイル public.key と private.key に保存する。

- オブジェクト作成
  - mactl は、ゲートウェイ・リソースのファイル、または、URLからマニフェストを取得して、JSON形式に変換して、marmotd に送信する。
  - marmotd は、受けたゲートウェイ・リソースの作成要求を、etcd に保存して、オブジェクトの作成は、Gatewayコントローラーに任せる。
  - 内部的には、他のオブジェクト同様に、uuidから導出した id で オブジェクトは識別する。 id の重複は許さない。
  - 同じ仮想ネットワークinternalVirtualNetwork上で、同一名称は許さない

ゲートウェイ・リソースのAPI
```
apiVersion: v1
kind: Gateway
metadata:
    name: igw
spec:
    bindPublicIpAddress: 192.168.1.100   # パブリック側のIPアドレスでアクセスを許可　ネットワークは host-bridge 固定
    internalServerName: server-10        # サーバーの名前
    internalVirtualNetwork: web-servers  # 内部側の仮想ネットワーク
    serverPorts:                         # リクエストを転送するポート番号のリストを以下に記述する
        - http                           # /etc/servicesのデータから、ポート番号とプロトコルに変換して、設定を実施する。
        - https
        - ssh
        - 1234/tcp                       # 数字から始まり スラッシュで、tcp or udp になっていれば、変換なしに使用する
```


- Gatewayコントローラーによりオブジェクトの作成、変更、削除
  - 15 秒間隔で、制御ループを実行
  - ゲートウェイ・オブジェクトの作成
    - 作成されていないゲートウェイ・オブジェクトを発見したら、以下の動作を実施する。
        - オブジェクトの作成は、次のマニフェストから作成される etcd 内の JSONデータの情報を取得して実行する。
        - OS: ubuntu24.04
        - CPU: 1
        - Memory: 1024
        - ssh認証の秘密鍵は、/etc/marmot/keysに保存された public.keyを root ユーザーにセットする
        - インターフェースが接続する外側ブリッジは、host-bridge に固定して、bindPublicIpAddress を割当
        - インターフェースが接続する内側ブリッジに、internalServerName で指定した仮想サーバーが接続する内部用ブリッジに接続する。
        - 内部用ブリッジとは、デフォルトで作成するdefault, host-bridge, ovs-network 以外の marmotd が作成したブリッジを指す。
        - 内部用ブリッジのIPアドレスは、仮想サーバーがIPアドレスを取得する際と同じ関数で取得するので良い。特別なIPアドレスを設けない。
        - オブジェクトの仮想サーバーが起動したら、次に ansible で /var/lib/marmot/ansible-playbooks に保存したansible playbook を使って、iptablesの設定を実施する。
        - ansible に必要な秘密鍵は、/etc/marmot/keys のディレクトリ下にある private.keyを使用する。
        - ansible を使った設定が失敗したら、次の制御ループで、リトライを繰り返す。もし、３回を超えて失敗したら、オブジェクトの作成を中止して、Status.statusをFAILED状態として、Status.messageに原因を記録する。
  - ゲートウェイ・オブジェクトの削除
    - deleteionTimestamp が作成されてから、15秒以上経過したオブジェクトは、削除処理を実施する。
    - 稼働サーバーを削除して、etcdのデータをクリアして削除完了となる。
  - ゲートウェイ・オブジェクトの変更
    - 変更を禁じる対象: bindPublicIpAddress, internalServerName, internalVirtualNetwork
    - 変更を許し対象: serverPorts



- ansible playbook で実行するOSへの設定
    - "-p tcp --dport 80" は、spec.serverPortsから導出した値をセットする
    - "--to-destination" は、internalServerNameとinternalVirtualNetworkから導出したIPアドレスに、serverPortsから導出したポート番号をセット

```
# IPフォワーディングを有効化
echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
sysctl -p

# HTTP (80番) を内部サーバーに転送
iptables -t nat -A PREROUTING -p tcp --dport 80 -j DNAT --to-destination 172.16.10.2:80

# マスカレード（戻りパケットの処理）
iptables -t nat -A POSTROUTING -j MASQUERADE
```

## 実装方針


1. バリデーション仕様（作成時）
1. metadata.name は spec.internalVirtualNetwork 単位で一意にする  
2. spec.bindPublicIpAddress はクラスタ全体で一意にする  
3. spec.serverPorts は次の形式のみ許可する  
4. サービス名（http, https, ssh など）は /etc/services 解決で tcp を既定にする  
5. 数値ポートは n/tcp または n/udp 形式を許可する

2. 更新仕様（apply, update）
1. 変更禁止は spec.bindPublicIpAddress, spec.internalServerName, spec.internalVirtualNetwork  
2. 変更許可は spec.serverPorts のみ  
3. 禁止項目に差分があれば 400 で拒否し、どの項目が不正かを明示する  
4. メモ内の web-servers は internalVirtualNetwork に統一して扱う

3. コントローラー仕様（15秒ループ）
1. PENDING 検知で Gateway 用 VM を作成  
2. NIC 外側は host-bridge 固定 + bindPublicIpAddress を割当  
3. NIC 内側は internalVirtualNetwork に接続し、IP は既存のサーバー割当ロジックを流用  
4. VM 起動後に Ansible で DNAT と MASQUERADE を設定  
5. Ansible 失敗は再試行し、3回超過で FAILED と message 記録  
6. DeletionTimestamp から 15 秒超過で削除を実行

4. パッケージと起動時準備
1. deb に ansible playbook を同梱する  
2. インストール時は既存 playbook を削除して再配置する  
3. 起動準備で /etc/marmot/keys を確認し、無ければ public.key/private.key を生成する

5. テスト観点（最低限）
1. 同一 internalVirtualNetwork 内の同名は作成失敗  
2. 別 internalVirtualNetwork なら同名作成成功  
3. bindPublicIpAddress 重複はノード跨ぎでも失敗  
4. update で禁止項目変更は失敗、serverPorts 変更は成功  
5. service 名解決が tcp 既定で正しく変換される  
6. Ansible 3回失敗で FAILED 遷移


### 段階的リリース案

Phase 1: API/DB/CLI まで（保存と表示だけ）
Phase 2: Controller で VM 作成まで
Phase 3: Ansible 適用 + retry/failure 完成
Phase 4: deb 同梱と postinst 反映、E2E テスト追加
