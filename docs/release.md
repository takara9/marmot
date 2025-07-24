
$ VERSION=x.y.z
$ echo $VERSION
$ git checkout main
$ git pull
$ git tag -a -m "Release v$VERSION" "v$VERSION"
$ git tag -ln
$ git push origin "v$VERSION"
$ gh release list
$ gh release create "v$VERSION"
$ cd bin
$ tar czvf ../marmot-v$VERSION.tgz .
$ cd ..
$ gh release upload "v$VERSION" --repo github.com/takara9/marmot marmot-v$VERSION.tgz
$ rm marmot-v$VERSION.tgz 


https://cli.github.com/manual/gh_release_create