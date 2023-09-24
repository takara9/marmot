# Go言語のインストール



https://go.dev/dl/


```console
curl -OL https://go.dev/dl/go1.21.1.linux-amd64.tar.gz
sudo tar xzvf go1.21.1.linux-amd64.tar.gz -C /usr/local
```


.bash_profileに設定

```console
export GOROOT=/usr/local/go
export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
```

