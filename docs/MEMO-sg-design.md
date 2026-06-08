# セキュリティグループ設計メモ

設計要件
- AWSのセキュリティグループ(以下 SGとする)に準じ、仮想サーバーのNICの通信を制御する。
- SGは、単一ノード／クラスタ共通データプレーンとして、OVNに統一する。
- SGは、host-bridge, default を除外する。kind: VirtualNetwork として作られたネットワークのみ適用可能とする。
- host-bridge, default に SG が設定されたら、バリデーターが弾いて、mactl コマンドが失敗する。
- api/marmot-api-v1.yaml にAPIを追加して、oapi-codegenでapi/marmot-api-v1.go を生成する。手書きは禁止。
- kind: Gateway, kind: VpnGateway, kind ApplicationLoadBalancer, kind: NetworkLoadBalancer に SGの適用は行わない。
- SGは、kind: SecurityGroup でAPIを定義する。
- SGはステートフルとする（AWS SG互換）。
- ルール判定の対象は「新規通信（NEW）」のみとする。
- いずれかのSGルールで許可された通信に対する戻り通信（ESTABLISHED, RELATED）は、明示ルールなしで許可する。
- 未許可の新規通信はDROPする。
- 複数SGが紐づく場合、NEW通信に対する許可判定は「allowの和集合」で行う。
- 例:
    - egressで tcp/443 を 0.0.0.0/0 に許可した場合、
      外向きHTTPS通信の戻りパケットは ingressルール未定義でも許可される。
    - 外部からの新規 ingress tcp/443 は、ingressルールで明示許可がない限りDROPされる。
- 優先順位なし。
- CIDR ベースのみ, 他SG参照はしない。
- mactl get など CLIで操作するYAMLは、metadata.name を使用するが、REST-APIでオブジェクトを特定するCRUD操作では、metadata.id を使用する。
- metadata.id は、オブジェクトの作成時に、一意のid が自動採番される。
- 以下は、APIのイメージ

```yaml
apiVersion: v1
kind: SecurityGroup
metadata:
  name: sg-web-frontend
spec:
  description: public nic for ssh and http
  rules:
    - direction: ingress
      protocol: tcp
      portRange:
        from: 22
        to: 22
      source:
        cidrs:
          - 10.10.0.0/16
          - 192.168.10.0/24
      description: allow ssh from admin networks

    - direction: ingress
      protocol: tcp
      portRange:
        from: 80
        to: 80
      source:
        cidrs:
          - 203.0.113.10/32
          - 198.51.100.0/24
      description: allow http from known addresses

    - direction: egress
      protocol: all
      destination:
        cidrs:
          - 0.0.0.0/0
      description: allow all outbound

---
apiVersion: v1
kind: SecurityGroup
metadata:
  name: sg-db-3306
spec:
  description: allow mysql from app segment only
  rules:
    - direction: ingress
      protocol: tcp
      portRange:
        from: 3306
        to: 3306
      source:
        cidrs:
          - 172.16.90.0/24
      description: mysql from app network only

    - direction: egress
      protocol: all
      destination:
        cidrs:
          - 0.0.0.0/0
      description: allow outbound

---
apiVersion: v1
kind: Server
metadata:
  name: app-01
spec:
  cpu: 4
  memory: 8192
  osVariant: ubuntu24.04
  networkInterface:
    - networkname: web-zone
      address: 10.1.0.50
      netmasklen: 24
      securityGroups:
        - sg-web-frontend

    - networkname: db-zone
      address: 172.16.90.10
      netmasklen: 24
      securityGroups:
        - sg-db-3306
```

CRUD エンドポイント例です（`/api/v1` 配下）。

- `POST /security-group` 作成
- `GET /security-group` 一覧取得
- `GET /security-group/{id}` 一件取得
- `GET /security-group/{id}/rules`
- `GET /security-group/{id}/rules/{ruleId}`
- `PUT /security-group/{id}` 更新（name 以外の spec 更新）
- `PUT /security-group/{id}/rules/{ruleId}`
- `DELETE /security-group/{id}` 削除（参照中なら 409 Conflict）
- `DELETE /security-group/{id}/rules/{ruleId}`

Server/NIC への適用について

- 既存 Server 更新に統合
- `PUT /server/{id}` の `spec.networkInterface[].securityGroups` で指定
- 例:
```yaml
spec:
  networkInterface:
    - networkname: web-zone
      securityGroups: [sg-web-frontend]
    - networkname: db-zone
      securityGroups: [sg-db-3306]
```
- 代替（操作系APIを増やす場合）
- `POST /server/{id}/network-interface/{nicIndex}/security-group/{sgId}`
- `DELETE /server/{id}/network-interface/{nicIndex}/security-group/{sgId}`

