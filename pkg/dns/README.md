# CoreDNSと連携するetcdにホストとドメインを登録する


テストのために、CoreDNSコンテナを起動して、アクセスさせる。

## ローカル環境でテスト

GitHub Actions でテストを実行する際は、CoreDNSとetcdのサービスをワークフローの中で、起動します。
しかし、ローカルでする時は、サービスを手作業で起動しなければなりません。そのために、以下のコマンドを実行します。

~~~
docker network create inter-network
docker run -d --name coredns --network inter-network -p 1053:53/udp -v /home/ubuntu/marmot/pkg/dns/testconf:/conf -e CONF="/conf/Corefile" -e PORT="53"  maho/coredns:1.11.1 
docker run -d --name etcd  --network inter-network -p 2379:2379 -e ALLOW_NONE_AUTHENTICATION=yes -e ETCD_ADVERTISE_CLIENT_URLS="http://0.0.0.0:2379" bitnami/etcd
~~~

テストが終わった後は、以下のコマンドで、停止させておきます。再スタートするには、`docker start [name]` を実行します。

~~~
docker stop coredns etcd
docker rm coredns etcd
~~~





## TEST の方法

```
go test
```



```
$ dig -p 1053 @localhost A minio.labo.local +noall +answer
minio.labo.local.	3600	IN	A	192.168.1.225

$ dig -p 1053 @localhost A hv1.labo.local +noall +answer
hv1.labo.local.		3600	IN	A	10.1.0.11
```



```
ETCDCTL_API=3 etcdctl put --endpoints=127.0.0.1:2379 /skydns/local/labo/a/hoge '{"host":"192.168.1.5","ttl":60}'
ETCDCTL_API=3 etcdctl put --endpoints=127.0.0.1:2379 /skydns/local/labo/a/hage '{"host":"192.168.1.6","ttl":60}'

ubuntu@hv0:~$ ETCDCTL_API=3 etcdctl get --endpoints=127.0.0.1:2379 /skydns/local/labo/a/hoge
/skydns/local/labo/test/hoge
{"host":"192.168.1.5","ttl":60}
ubuntu@hv0:~$ ETCDCTL_API=3 etcdctl get --endpoints=127.0.0.1:2379 /skydns/local/labo/a/hage
/skydns/local/labo/test/hage
{"host":"192.168.1.6","ttl":60}
ubuntu@hv0:~$ ETCDCTL_API=3 etcdctl get --endpoints=127.0.0.1:2379 /skydns/local/labo/a
```


```
$ dig -p 1053 @localhost A hoge.a.labo.local +noall +answer
hoge.test.labo.local.	60	IN	A	192.168.1.5

$ dig -p 1053 @localhost A hage.a.labo.local +noall +answer
hage.test.labo.local.	60	IN	A	192.168.1.6

$ dig -p 1053 @localhost A a.labo.local +noall +answer
test.labo.local.	60	IN	A	192.168.1.6
test.labo.local.	60	IN	A	192.168.1.5

```



テストする時に便利なツール

apt install dnsutils iputils-ping vim






