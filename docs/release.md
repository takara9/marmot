

    $ VERSION=x.y.z
    $ echo $VERSION

    $ git checkout main
    $ git pull
    $ git checkout -b "bump-$VERSION"

`CHANGELOG.md` を編集

    $ git commit -a -m "Bump version to $VERSION"
    $ git push -u origin HEAD
    $ gh pr create -f


    # Set VERSION again.
    $ VERSION=x.y.z
    $ echo $VERSION

    $ git checkout main
    $ git pull
    $ git tag -a -m "Release v$VERSION" "v$VERSION"

    $ git tag -ln | grep $VERSION
    $ git push origin "v$VERSION"
    


git log --format="%H" -n 1
git tag v0.8.2 42bffa9c7e79160c25385a983dc764769a2ffbd0


gh release upload v0.8.2 --repo github.com/takara9/marmot marmot-v0.8.2.tgz


TAG=`cat TAG`
gh release upload $TAG --repo github.com/takara9/marmot marmot-$TAG.tgz



https://cli.github.com/manual/gh_release_create