# Go言語のインストール



https://go.dev/dl/


1.21 は、現在 libvirtが動かない

```console
GOVER=1.21.1
```

```console
GOVER=1.19.13
```

```
GOVER=1.23.4
curl -OL https://go.dev/dl/go${GOVER}.linux-amd64.tar.gz
sudo tar xzvf go${GOVER}.linux-amd64.tar.gz -C /usr/local
```

.bashrcの下部に設定

```console
export GOROOT=/usr/local/go
export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
```

