# Ceph 統合の検証

```console
sudo apt install -y chrony lvm2
sudo systemctl enable --now chrony
wget -q -O- 'https://download.ceph.com/keys/release.asc' | sudo tee /etc/apt/trusted.gpg.d/ceph.asc
echo "deb https://download.ceph.com/debian-tentacle/ $(lsb_release -sc) main" | sudo tee /etc/apt/sources.list.d/ceph.list
sudo apt update
sudo apt install -y cephadm ceph-common
ip a
sudo mkdir -p /etc/ceph
sudo cephadm bootstrap --mon-ip  10.1.3.11
```

```
	     URL: https://ceph-single:8443/
	    User: admin
   	Password: 9h48agi8nn

```

```console
sudo ceph orch device ls
sudo ceph orch daemon add osd $(hostname):/dev/vdb
sudo ceph orch daemon add osd $(hostname):/dev/vdc
sudo ceph orch daemon add osd $(hostname):/dev/vdd
```

```console
sudo ceph osd crush rule ls
sudo ceph osd crush rule create-replicated single-node-rule default osd
sudo ceph config set global mon_allow_pool_size_one true
sudo ceph config get mon mon_allow_pool_size_one
sudo ceph config set global mon_warn_on_pool_no_redundancy false
sudo ceph config set global osd_pool_default_size 1
sudo ceph config set global osd_pool_default_min_size 1
```


```console
sudo ceph osd pool set .mgr size 1 --yes-i-really-mean-it
sudo ceph osd pool set .mgr min_size 1
```

```console
sudo ceph osd pool create mypool 32 32 single-node-rule
sudo ceph osd pool set mypool size 1 --yes-i-really-mean-it
sudo ceph osd pool set mypool min_size 1
sudo ceph osd pool application enable mypool rbd
```


```console
sudo ceph orch apply mds cephfs --placement="1"
sudo ceph fs volume create cephfs
sudo ceph fs authorize cephfs client.vmuser / rw
```

```
[client.vmuser]
	key = AQB/BFJqdCdiDBAAI35OGjC1OlgRmFtm1eOPMw==
	caps mds = "allow rw fsname=cephfs"
	caps mon = "allow r fsname=cephfs"
	caps osd = "allow rw tag cephfs data=cephfs"
```

```console
sudo ceph auth get client.vmuser -o /etc/ceph/ceph.client.vmuser.keyring
sudo ceph auth print-key client.vmuser
```

```
AQB/BFJqdCdiDBAAI35OGjC1OlgRmFtm1eOPMw==
```


```console
sudo ceph osd pool set cephfs.cephfs.meta crush_rule single-node-rule
sudo ceph osd pool set cephfs.cephfs.meta size 1 --yes-i-really-mean-it
sudo ceph osd pool set cephfs.cephfs.meta min_size 1
sudo ceph osd pool set cephfs.cephfs.data crush_rule single-node-rule
sudo ceph osd pool set cephfs.cephfs.data size 1 --yes-i-really-mean-it
sudo ceph osd pool set cephfs.cephfs.data min_size 1
```






```console
sudo ceph status
sudo ceph fs status
sudo ceph mds stat
sudo ceph health detail
``` 


---

## CEPH-FS クライアント側

```console
sudo apt install -y ceph-common
sudo mkdir -p /mnt/cephfs
sudo vi /etc/fstab
sudo cat /etc/fstab |tail -n 1
10.1.3.11:6789:/ /mnt/cephfs ceph name=vmuser,_netdev 0 2
```

```console
sudo vi /etc/ceph/ceph.conf
cat /etc/ceph/ceph.conf 
[global]
mon_host = 10.1.3.11
```


```console
echo -e "[client.vmuser]\n\tkey = AQCb11FqnufyChAApiSsmvITe3wl19hUs+09bw==" | sudo tee /etc/ceph/ceph.client.vmuser.keyring
sudo mount /mnt/cephfs
```

---

ubuntu@ceph-single:~$ sudo ceph fs volume create cephfs2
ubuntu@ceph-single:~$ sudo ceph fs authorize cephfs2 client.vmuser2 / rw
[client.vmuser2]
	key = AQAi5FFqoTeZOBAAJE54M7M57D9ozNVd6cEMEQ==
	caps mds = "allow rw fsname=cephfs2"
	caps mon = "allow r fsname=cephfs2"
	caps osd = "allow rw tag cephfs data=cephfs2"
