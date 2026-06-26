# メトリックスとログの機能強化

OpenTelemetryのライブラリを組み込んで、

- メトリックスを Prometheus から スクレイブできるようにしたい。
- ログを loki に飛ばせるようにしたい。


# メトリックスの項目


- VM総数
- VMのStatus RUNNING
- VMのStatus ERROR
- VMのStatus PENDING

- VitrualNetwork総数
- VitrualNetwork ACTIVE数
- VitrualNetwork ERROR数

- Volume総数
- Volume Available数
- Volume Failed数

- CPU総数
- 割当済 CPU数
- Free CPU数

- メモリ総量
- メモリ割当量

# ログ出力

ログは、slogの出力を lokiへ切り替えて飛ばす

出力先のLokiのアドレスは、/etc/marmot/marmotd.json に 項目を追加して、
宛先アドレスを /etc/marmot/marmotd.jsonから取得する。
