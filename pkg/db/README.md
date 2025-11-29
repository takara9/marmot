# marmotのetcdをアクセスする


パッケージのテストを実行する

~~~
$ go mod init dbaccess
$ go mod tidy
$ go test -v .
~~~


~~~
$ etcdctl get se --prefix
serial
{"Serial":20}
~~~


## キー
旧キーと新キーの比較を整理しておく。
旧キーを廃止して、新キーに移行する。
シーケンス番号は廃止の方向で、UUIDを信頼して、UUIDへ移行する。ただし、重複チェックを入れること


|旧Key | 新Key|説明
|---|---|---|
|hv |/marmot/hypervisor/*key*|ハイパーバイザーの情報、ノード名、資源の量、使用状態など|
|vm|/marmot/vristualmachine/*key*|仮想マシンの情報、VM名、資源割当量、稼働状態など|
|OSI_|/marmot/osimage/*key*|OSのイメージテンプレート、名前、ラベル、OSバージョン、生成日、更新日など|
|N/A| /marmot/volume/*key*|データボリューム、名前、ラベル、容量、ファイル名、生成日、更新日など|
|SEQNO_|/marmot/sequence/*key*|シリアル番号の管理用（廃止予定）
|version|/marmot/version|marmotのバージョン番号



## 参考資料
* テストの書き方(Golandで作成, 並列実行, 前後に処理追加), https://www.wakuwakubank.com/posts/866-go-test/
* Goのテストに入門してみよう！, https://future-architect.github.io/articles/20200601/



