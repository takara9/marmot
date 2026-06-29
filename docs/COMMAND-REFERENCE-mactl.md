# mactl コマンドリファレンス

このドキュメントは、mactl で利用できる主要コマンドを用途別に整理したリファレンスです。

## 共通オプション

- --api: API Endpoint URL または設定ファイルパスを指定
- -o, --output: 出力形式を指定 (text/json/yaml)
- -w, --watch: 変化があった時に表示を更新
- --watch-interval: watch の更新間隔(秒)

## 認証とセッション

- mactl login USER-ID
  - パスワードでログインし、アクセストークンを保存
  - 既存トークンがある場合は、新しいログイン成功後に旧セッションを logout
- mactl logout [USER-ID]
  - 現在セッションを logout
  - 現在トークンに一致するセッション API キーを削除
- mactl passwd
  - ログイン中ユーザーのパスワードを変更
- mactl role
  - 自分のユーザーIDとロールを表示
- mactl whoami
  - mactl role のエイリアス
- mactl role list
  - 組み込みロールの一覧を表示

## ユーザー管理

- mactl user add USER-ID
- mactl user delete USER-ID
- mactl user list
  - USER-ID / ENABLED / ROLE / AGE を表示
- mactl user lock USER-ID
- mactl user unlock USER-ID
- mactl user set-passwd USER-ID

ロール割り当て:

- mactl user add-role USER-ID ROLE-NAME
- mactl user del-role USER-ID ROLE-NAME
- mactl user list-role USER-ID

API キー:

- mactl user generate-apikey
- mactl user list-apikey
- mactl user delete-apikey API-KEY-ID

セッション一覧:

- mactl user session
  - Administrator: 全ユーザーの login セッションを表示
  - Administrator 以外: 自ユーザーの login セッションのみ表示

## エンドポイント管理

- mactl ep list
- mactl ep add <URL>
- mactl ep set <番号>
- mactl ep delete <番号>

## サーバー操作

- mactl server create
- mactl server list
- mactl server detail [server-id]
- mactl server update [server-id]
- mactl server delete [server-id...]
- mactl server start [server-id...]
- mactl server stop [server-id...]
- mactl server createimage server-id image-name

## ネットワーク操作

- mactl network create
- mactl network list
- mactl network detail [network id]
- mactl network delete [network-id...]
- mactl network ipn
- mactl network ipn-by-vn [network id]
- mactl network ips [ip network id]

## ボリューム操作

- mactl volume create -f FILE.yaml
- mactl volume list
- mactl volume detail [volume id]
- mactl volume delete [volume id]
- mactl volume rename [volume id] [new name]

## イメージ操作

- mactl image create -f FILE.yaml
- mactl image list
- mactl image detail [image-id]
- mactl image update [image-id]
- mactl image delete [image-id...]
- mactl image import [filename.tgz]
- mactl image export [image-name]

## クラスタ状態

- mactl marmot status
- mactl marmot cluster

## 汎用操作

- mactl get [RESOURCE [NAME]]
- mactl describe RESOURCE NAME
- mactl create [RESOURCE]
- mactl apply [RESOURCE]
- mactl del [RESOURCE NAME]
- mactl console SERVER-NAME
- mactl version

## 運用メモ

- 認証情報はホームディレクトリ配下のトークンファイルに保存されます。
- 認証エラー時は、まず mactl logout の実行後に再度 mactl login を実施してください。
- user session は login セッションのみ対象で、生成 API キーは対象外です。
