# VMのmachine-idとhostidの生成規則

Issue #285 対応として、生成したVMごとに一意な guest identity を付与する。

## 目的

- VMテンプレートを複製しても、guest OS 側の識別子が重複しないようにする
- hostid コマンドの出力が VM ごとに変わるようにする
- systemd 系が利用する machine-id も一緒に一意化する

## 生成規則

1. machine-id は 32 桁の16進文字列で生成する
1. Server.Metadata.Uuid があれば、ハイフンを除去して小文字化した値を machine-id として使う
1. Metadata.Uuid が無ければ、Server.Id を優先し、無ければ Server.Metadata.Name、さらに無ければ固定文字列 marmot を入力として SHA-256 を計算する
1. SHA-256 の先頭16バイトを 32 桁の16進文字列へ変換し、machine-id として使う
1. /etc/hostid は machine-id 文字列に対して CRC32 を計算し、その32bit値を little-endian の4バイトで書き込む
1. CRC32 の結果が 0 の場合だけ 1 に補正する

## 実装箇所

- pkg/util/setup-linux.go

## テスト

- pkg/util/setup-linux_identity_test.go
- pkg/util/setup-linux_test.go

### 実行例

unitテストのみ実行する場合

```
go test ./pkg/util -run TestUtilIdentity -v -args -ginkgo.label-filter='unit && identity'
```

統合テストを除外して実行する場合

```
go test ./pkg/util -run TestUtilIdentity -v -args -ginkgo.label-filter='!integration'
```

root権限と実機リソースがある環境で統合テストを実行する場合

```
sudo -E go test ./pkg/util -run TestUtil -v -args -ginkgo.label-filter='integration && requires-root'
```

### CI運用メモ

pkg/util には複数の Ginkgo suite があるため、go test の -run 指定は部分一致ではなく完全一致を使う。

- unit（非root・常時実行）
	- 実行ターゲット: make test-unit
	- 実行内容: TestUtilIdentity suite のみを実行し、integration ラベルを除外する
- integration（root・環境依存）
	- 実行ターゲット: make test-integration
	- 実行内容: TestUtil suite のうち integration と requires-root ラベルのみを実行する

Makefile では以下のように suite を固定して、RunSpecs の多重実行を防ぐ。

```
go test . -run '^TestUtilIdentity$' ...
go test . -run '^TestUtil$' ...
```

## 注意点

- /etc/machine-id と /etc/hostid は用途が違うため、両方を更新する
- /etc/hostid はテキストではなく4バイトのバイナリファイルとして書き込む
- Metadata.Uuid を優先することで、DBで払い出した VM の固有識別子と guest identity を揃えられる