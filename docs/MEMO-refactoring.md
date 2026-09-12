## 保守性を改善するためのリファクタリング

現状
`server-controller.go:21` で定義されている `type controller struct` は、**VM/サーバー専用ではなく、パッケージ内の複数のコントローラーで共有されている型** されている。


以下の関数がすべて同じ `controller` 型を使い回しています（他ファイルにあるコメントアウトされた同名定義は、共有先である本体がserver-controller.goにあることを示す痕跡です）。

- `StartVmController`（server-controller.go）— サーバー(VM)
- `StartNetController`（network-controller.go）— ネットワーク
- `StartVolController`（volume-controller.go）— ボリューム
- `StartImageController`（image-controller.go）— イメージ
- `StartApplicationLoadBalancerController`（application-load-balancer-controller.go）
- `StartGatewayController`（gateway-controller.go）
- `StartNetworkLoadBalancerController`（network-load-balancer-controller.go）
- `StartVpnGatewayController`（vpn-gateway-controller.go）

対策として型を専用化して、将来の拡張時の影響を隔離する。
- `StartVmController` 専用の `serverController` 型を新設し、他7つは別の共有型のまま残す。


