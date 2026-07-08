# Go言語のインストール



https://go.dev/dl/


```console
GOVER=1.26.2
curl -OL https://go.dev/dl/go${GOVER}.linux-amd64.tar.gz
sudo tar xzvf go${GOVER}.linux-amd64.tar.gz -C /usr/local
```

.bashrcの下部に設定

```console
export GOROOT=/usr/local/go
export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
```

