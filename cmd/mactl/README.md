# mactlのV2 

Cobraフレームワークを使って開発するリメイク版

作成するサブコマンドは以下の機能

- create: 新たなVMの作成
- destroy: 稼働中のVMの削除
- start: 停止中のVMの起動
- stop: 稼働中のVMを停止
- status: VMのリストを出力
- version: バージョン表示

設定 YAML はローカルファイルだけでなく raw URL も指定可能

```sh
mactl server create --configfile https://raw.githubusercontent.com/takara9/marmot/refs/heads/main/cmd/mactl/testdata/test-server-03-host-bridge-ip.yaml
mactl network create -f https://raw.githubusercontent.com/takara9/marmot/refs/heads/main/cmd/mactl/testdata/test-network-02-test-net-2.yaml
```

