# marmotのインストール

## etcdをインストール

```
$ sudo -s
# apt-get install etcd

```

`systemctl edit etcd` を使って、以下を追加

```
[Service]
Environment=ETCD_LISTEN_CLIENT_URLS="http://0.0.0.0:12379"
Environment=ETCD_ADVERTISE_CLIENT_URLS="http://0.0.0.0:12379"
```

`systemctl daemon-reload` の実行

```
# systemctl restart etcd
# systemctl status etcd
● etcd.service - etcd - highly-available key value store
     Loaded: loaded (/lib/systemd/system/etcd.service; enabled; vendor preset: enabled)
    Drop-In: /etc/systemd/system/etcd.service.d
             └─override.conf
     Active: active (running) since Sat 2025-01-18 13:04:33 JST; 7s ago
       Docs: https://etcd.io/docs
             man:etcd
   Main PID: 3289 (etcd)
      Tasks: 10 (limit: 154384)
     Memory: 6.0M
        CPU: 66ms
     CGroup: /system.slice/etcd.service
             └─3289 /usr/bin/etcd
```



## marmotのダウンロードと展開

mkdir work
cd work
curl -OL https://github.com/takara9/marmot/releases/download/v0.8.2/marmot-v0.8.2.tgz
tar xzvf marmot-v0.8.2.tgz


## ノード名の変更

$ vi marmot.service 

```
ExecStart=/bin/sh -c "cd /usr/local/marmot;/usr/local/marmot/vm-server --node=hv1 --etcd=http://localhost:12379"
```

インストールシェルの編集
```
# vi install.sh 
```

インストール先を /etc から /lib へ変更
```
install -m 0644 ${BINDIR}/marmot.service /lib/systemd/system
```


## インストール実行

```
# ./install.sh 
```

```
# systemctl status marmot
● marmot.service - marmot - vm cluster service
     Loaded: loaded (/lib/systemd/system/marmot.service; enabled; vendor preset: enabled)
     Active: active (running) since Sat 2025-01-18 13:05:49 JST; 51s ago
       Docs: https://github.com/takara9/marmot
             man:marmot
   Main PID: 3349 (sh)
      Tasks: 13 (limit: 154384)
     Memory: 9.8M
        CPU: 18ms
```



# vi config_marmot 
# cp config_marmot /home/ubuntu/.config_marmot
# cp config_marmot /root/.config_marmot
# mactl global-status



hv-admin  -config hypervisor-config-hv1.yaml 


