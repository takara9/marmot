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
$ gh release upload "v$VERSION" --repo github.com/takara9/marmot dist/marmot-v${VERSION}.tgz
$ gh release upload "v$VERSION" --repo github.com/takara9/marmot dist/marmot_v${VERSION}_amd64.deb
$ make clean
```


参考: https://cli.github.com/manual/gh_release_create

## リリースノート記載例

Issue #285 向けの記載例

```
- fix(server): VM作成時にguest identityを一意化
	- /etc/machine-id をMetadata.Uuid優先で生成
	- /etc/hostid をmachine-id由来の4バイト値で生成
	- 同一テンプレートから作成した複数VMで hostid が重複しないように改善
	- 関連テスト: pkg/util/setup-linux_identity_test.go, pkg/util/setup-linux_test.go
```