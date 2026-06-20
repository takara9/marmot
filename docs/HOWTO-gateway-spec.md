Gateway の仕様

1. 外部公開IPで受けた通信を、内部ネットワーク上の特定サーバーへ転送する
2. 転送ルールは Gateway の serverPorts に従って作る
3. 転送元の許可範囲は remoteCIDR または remoteCIDRs で制御する
4. 実際の通信制御は Gateway 用VM上の iptables で実施する
5. コントローラーが状態遷移しながら自動適用する
PENDING → PROVISIONING → CONFIGURING → ACTIVE
失敗が続くと FAILED

iptables 側でやっていること
1. IPv4フォワードを有効化
2. 専用チェーンを作成して INPUT と FORWARD と PREROUTING を接続
3. 許可CIDRからの通信だけ通す
4. 指定ポートを内部ターゲットIPへ DNAT
5. 戻り通信のため POSTROUTING で MASQUERADE

つまり「Marmot の Gateway は、iptables ベースの簡易L3/L4ゲートウェイ兼ポートフォワーダ」です。

仕様の実装位置
1. Gateway コントローラー本体: gateway-controller.go
2. iptables playbook 実行: gateway-ansible.go
3. 実際の playbook テンプレート: gateway-iptables.yaml.tmpl
4. 仕様メモ: MEMO-internet-gateway.md

