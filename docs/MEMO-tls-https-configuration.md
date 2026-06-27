# mactl と marmotd 間の TLS/HTTPS 通信設定

## 概要
mactl クライアントと marmotd サーバー間の通信を TLS/HTTPS で暗号化するための設定方法。

## 実装状況

### クライアント側 (pkg/client/core.go)
- ✅ `NewMarmotdEp()` が scheme パラメータを使用するように修正
- ✅ `TLSClientConfig.InsecureSkipVerify` を false に変更（本番環境）
  - 開発時は環境変数で InsecureSkipVerify=true に戻すことを検討
- ✅ URL scheme（http/https）が config の `api_server` URL から正しく抽出される

### サーバー側 (pkg/marmotd/marmotd-config.go)
- ✅ TLS 証明書ファイルパス設定を追加
  - `tls_cert_file`: サーバー証明書 PEM ファイル
  - `tls_key_file`: 秘密鍵 PEM ファイル
- ✅ 空の場合は HTTP にフォールバック

### サーバー起動 (cmd/marmotd/marmotd-main.go)
- ⚠️ 実装予定: `e.StartTLS()` または `e.Start()` を条件分岐で使い分け

## 設定方法

### marmotd 設定ファイル例（/etc/marmot/marmotd.json）

```json
{
  "node_name": "hv1",
  "etcd_url": "http://127.0.0.1:2379",
  "api_listen_addr": "0.0.0.0:8750",
  "tls_cert_file": "/etc/marmot/certs/server.crt",
  "tls_key_file": "/etc/marmot/certs/server.key",
  "dns_listen_addr": "0.0.0.0:53",
  "dns_upstream": "8.8.8.8:53"
}
```

### mactl クライアント設定例（~/.marmot）

```yaml
endpoints:
  - name: lab
    url: https://hv1.example.com:8750
    username: admin
    description: 本番ハイパーバイザー
  - name: dev
    url: http://localhost:8750
    username: admin
    description: 開発環境（TLS未使用）
active_endpoint: lab
```

## 証明書の生成方法

### 自己署名証明書の生成（開発用）

```bash
# 秘密鍵を生成
openssl genrsa -out /etc/marmot/certs/server.key 2048

# 証明書署名要求(CSR)を生成
openssl req -new -key /etc/marmot/certs/server.key \
  -out /etc/marmot/certs/server.csr \
  -subj "/CN=hv1.example.com/O=Marmot/C=JP"

# 自己署名証明書を生成（365日有効）
openssl x509 -req -days 365 \
  -in /etc/marmot/certs/server.csr \
  -signkey /etc/marmot/certs/server.key \
  -out /etc/marmot/certs/server.crt

# パーミッション設定
chmod 600 /etc/marmot/certs/server.key
chmod 644 /etc/marmot/certs/server.crt
```

## 注意事項

### セキュリティ

1. **本番環境での CA 証明書**
   - オレオレ証明書ではなく、正式な CA で署名された証明書を使用
   - または企業内 CA から発行

2. **クライアント側検証**
   - 開発環境では InsecureSkipVerify=true でも許容
   - 本番環境では InsecureSkipVerify=false が必須
   - 環境変数 `MARMOT_INSECURE_SKIP_VERIFY=true` で制御可能にする検討

3. **秘密鍵の保護**
   - ファイルシステムのパーミッション: 0600
   - ディレクトリのパーミッション: 0700
   - etcd にはシークレットとして保存しない

### 互換性

- TLS 設定ファイルが空（または省略）の場合は HTTP で起動
- 段階的な移行が可能：既存の HTTP 運用から TLS への段階的移行をサポート
- 新規インストール時は HTTPS を強く推奨

## トラブルシューティング

### 証明書検証エラー

```
x509: certificate signed by unknown authority
```

**原因**：自己署名証明書をクライアントが検証できない

**対応**：
- 開発環境：`MARMOT_INSECURE_SKIP_VERIFY=true` を使用
- 本番環境：CA 証明書チェーンをシステムに追加、または正式な CA 証明書を使用

### ポートバインドエラー

```
error binding to port: permission denied
```

**原因**：ポート 443 などの特権ポート使用時に権限不足

**対応**：
- 非特権ポート（8750 など）を使用
- または `sudo` 権限で実行

## 将来の拡張

1. **mTLS（相互認証）**
   - クライアント証明書検証
   - api_key や Bearer token との併用

2. **Let's Encrypt 統合**
   - 自動更新対応
   - ACME プロトコル対応

3. **設定の Hot Reload**
   - 再起動なしで証明書更新を反映

4. **監査ログ**
   - TLS ハンドシェイク失敗記録
   - 証明書検証エラー追跡
