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

## Issue #470 サンプル: spec.ansible

`cmd/mactl/testdata/test-server-38-ansible-playbook.yaml` は、Server 作成後に `ansible-playbook` を自動適用するサンプルです。

```sh
mkdir -p playbook
# cp /path/to/setup.yaml playbook/setup.yaml

export MARMOT_ANSIBLE_PRIVATE_KEY="$HOME/.ssh/id_ed25519"
mactl server create --configfile cmd/mactl/testdata/test-server-38-ansible-playbook.yaml
```

メモ:

- `spec.ansible.playbook` と `spec.ansible.inventory` は必須です。
- `spec.ansible` は `host-bridge` の固定IPが必要です。
- `spec.ansible.extra-args` は `ansible-playbook` コマンド引数として追加されます。
- `ansible.cfg` がカレントディレクトリに無い場合、`mactl` は `ANSIBLE_HOST_KEY_CHECKING=False` などの環境変数を補って実行します。

