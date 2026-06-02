## ロードバランサー機能の設計メモ

### 基本機能

- ロードバランサーは、内部に haproxy を実行する仮想サーバーで、Public network (host-bridge) と 仮想ネットワークに接続する。
- Public network (host-bridge)に VIPを持ち、もう一方の仮想ネットワークに繋がった、 Webサーバーなどへ、リクエストを分配する。
- ロードバランサーの仮想サーバーには、marmotd のクライアントになるHAPROXYコントローラー・プロセスを起動して、marmod 経由で、etcdの情報を収集して、haproxy の設定を実施する。
- この HAPROXYコントローラー・プロセスは、5秒間隔で、marmotd の APIをチェックして、変更があった場合、ロードバランサーの設定を変更する。
- ロードバランサーの仮想サーバーのPublic network (host-bridge)の アクセスを許容する CIDR、プロトコルの設定は、リソース GW で開発した ansible playbook を利用する。



```yaml
apiVersion: v1
kind: LoadBalancer
metadata:
    name: lb-1
spec:
    remoteCIDR: 192.168.1.0/24            # 接続を許すリモートのIPアドレス
    bindPublicIpAddress: 192.168.1.70     # パブリック側のIPアドレスでアクセスを許可
    internalVirtualNetwork: web-backends  # 内部側の仮想ネットワークに、起動しており、ラベルが付与されたサーバーを負荷分散対象とする
    serverPorts:                          # リクエストを転送するポート番号のリストを以下に記述する
        - https                           # httpとhttpの通信を許可、それ以外は禁止    
        - http
    healthCheck: true                     # 検知された対象サーバーに ヘルスチェックを実施する。10回続けて、応答が無い時は、対象のサーバーを再起動する。
    sessionPersistence: true              # HTTPヘッダーに セッションを表すクッキーが入っている場合、バックエンドの Webサーバーを固定する。
    responseMetrics: true                 # True の時は、
```