# CoreDNSと連携するetcdにホストとドメインを登録する



## TEST の方法

```
go test
```






ubuntu@hv0:~$ ETCDCTL_API=3 etcdctl put --endpoints=127.0.0.1:2379 /skydns/local/test/hoge '{"host":"192.168.1.5","port":"8080"}'
OK

ubuntu@hv0:~$ ETCDCTL_API=3 etcdctl get --endpoints=127.0.0.1:2379 /skydns/local/test/hoge
/skydns/local/test/hoge
{"host":"192.168.1.5","port":"8080"}


ubuntu@hv0:~$ dig -p 1053 @localhost A minio.labo.local +noall +answer
minio.labo.local.	3600	IN	A	192.168.1.225

ubuntu@hv0:~$ dig -p 1053 @localhost A hoge.test +noall +answer
ubuntu@hv0:~$ dig -p 1053 @localhost A hoge.test.local +noall +answer
;; communications error to 127.0.0.1#1053: timed out

ubuntu@hv0:~$ dig -p 1053 @localhost A hv1.labo.local +noall +answer
hv1.labo.local.		3600	IN	A	10.1.0.11