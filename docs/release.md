$ VERSION=`cat TAG`
$ echo $VERSION
$ git checkout main
$ git pull
$ git tag -a -m "Release v$VERSION" "v$VERSION"
$ git tag -ln
$ git push origin "v$VERSION"
$ make package
$ gh release list
$ gh release create "v$VERSION"
$ cd bin
$ gh release upload "v$VERSION" --repo github.com/takara9/marmot marmot-v$VERSION.tgz
$ cd ..
$ make clean


https://cli.github.com/manual/gh_release_create