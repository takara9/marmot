


git log --format="%H" -n 1
git tag v0.8.2 42bffa9c7e79160c25385a983dc764769a2ffbd0


gh release upload v0.8.2 --repo github.com/takara9/marmot marmot-v0.8.2.tgz


TAG=`cat TAG`
gh release upload $TAG --repo github.com/takara9/marmot marmot-$TAG.tgz



https://cli.github.com/manual/gh_release_create