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



## 参考資料
* テストの書き方(Golandで作成, 並列実行, 前後に処理追加), https://www.wakuwakubank.com/posts/866-go-test/
* Goのテストに入門してみよう！, https://future-architect.github.io/articles/20200601/



