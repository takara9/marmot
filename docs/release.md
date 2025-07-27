
## リリース手順

ソースコードのマージが完了した後に、以下の手順を実施する。

### メインブランチを最新にする
```
$ git checkout main
$ git pull
```

### タグを設定する
```
$ VERSION=`cat TAG`
$ echo $VERSION
$ git tag -a -m "Release v$VERSION" "v$VERSION"
$ git tag -ln
$ git push origin "v$VERSION"
```

### パッケージを作成、リリース作成して、アップロードする。
```
$ make package
$ gh release list
$ gh release create "v$VERSION"
$ gh release upload "v$VERSION" --repo github.com/takara9/marmot marmot-v$VERSION.tgz
$ make clean
```


参考: https://cli.github.com/manual/gh_release_create