# marmotのパッケージ作成法

package 実行時に CGO を有効化して実行

```
sudo apt install gcc pkg-config libvirt-dev
pkg-config --modversion libvirt
CGO_ENABLED=1 make package
```

