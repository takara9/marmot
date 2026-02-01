# データベースの内容をダンプする方法


## marmot のオブジェクト全体のキーをリストする

```
# etcdctl get /marmot --prefix --keys-only |grep -v -e '^#' -e '^$'
/marmot/hypervisor/ws1
/marmot/osTemplateImage/ubuntu18.04
/marmot/osTemplateImage/ubuntu20.04
/marmot/osTemplateImage/ubuntu22.04
/marmot/sequence/LVDATA
/marmot/sequence/LVOS
/marmot/sequence/VM
/marmot/server/1a5cc1ef
/marmot/version
/marmot/volume/b7fd113
```

## キーを指定して、読みやすい形式で、内容をダンプする

```
# KEY=/marmot/server/1a5cc1ef
# etcdctl get $KEY --print-value-only |jq -r
{
  "Network": [
    {
      "id": "host-bridge",
      "mac": "52:25:29:ee:07:61"
    }
  ],
  "bootVolume": {
    "id": "b7fd113",
    "key": "/marmot/volume/b7fd113",
    "kind": "os",
    "logicalVolume": "oslv0208",
    "name": "boot-1a5cc1ef",
    "path": "/dev/vg1/oslv0208",
    "size": 0,
    "status": 0,
    "type": "lvm",
    "volumeGroup": "vg1"
  },
  "cTime": "2026-02-01T16:15:45.961162707+09:00",
  "comment": "This is a test server configuration",
  "cpu": 2,
  "id": "1a5cc1ef",
  "instanceName": "test-server-3-1a5cc1ef",
  "memory": 2048,
  "name": "test-server-3",
  "osVariant": "ubuntu22.04",
  "status": 3,
  "uuid": "1a5cc1ef-cd6c-4258-90ac-4ba9c90a982a"
}
```

```
# KEY=/marmot/volume/b7fd113
# etcdctl get $KEY --print-value-only |jq -r
{
  "id": "b7fd113",
  "key": "/marmot/volume/b7fd113",
  "kind": "os",
  "logicalVolume": "oslv0208",
  "name": "boot-1a5cc1ef",
  "path": "/dev/vg1/oslv0208",
  "size": 0,
  "status": 2,
  "type": "lvm",
  "volumeGroup": "vg1"
}
```