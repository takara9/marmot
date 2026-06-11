# Apline, Rockey, Debian などクラウドイメージ対応


## 動かしたいクラウドイメージ
- Alpine Linux 3.23.4: https://dl-cdn.alpinelinux.org/alpine/v3.24/releases/x86_64/alpine-virt-3.24.0-x86_64.iso
- Ubuntu Linux 26.04: https://cloud-images.ubuntu.com/resolute/current/resolute-server-cloudimg-amd64.img
- Rockey Linux 8: https://dl.rockylinux.org/pub/rocky/8/images/x86_64/Rocky-8-EC2-LVM.latest.x86_64.qcow2
- Rockey Linux 9: https://dl.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2
- Debian Linux 13 https://cloud.debian.org/images/cloud/trixie/latest/debian-13-generic-amd64.qcow2

## データ構造

- name: イメージの名前 任意の文字列
- osName: alpine | ubuntu｜rockey | debian　小文字英字
- osVersion: 22.04|24.04|26.04|8|9|13 : 数字とドット

osNameとosVersionの組み合わせで、対応する初期化モジュールを切り替える
name は、url, osName, osVersion を代表する名前で、検索キーとして用いる


## 起動時の初期読み込み

/etc/marmot/marmotd.json に記述された os_images の情報を元に、marmotd デーモンが利用準備を実施する

```json
  "os_images": [
    {
      "name": "ubuntu22.04",
      "url": "https://cloud-images.ubuntu.com/releases/jammy/release/ubuntu-22.04-server-cloudimg-amd64.img",
      "osName": "ubuntu",
      "osVersion": "22.04"
    },
    {
      "name": "ubuntu24.04",
      "url": "https://cloud-images.ubuntu.com/releases/noble/release/ubuntu-24.04-server-cloudimg-amd64.img",
      "osName": "ubuntu",
      "osVersion": "24.04"
    }
  ]
```

## etcd に保存される オブジェクトの構造体

OS切り替え判定のため、spec.osName, spec.osVersion を追加する。

```json
  {
    "apiVersion": "v1",
    "kind": "Image",
    "metadata": {
      "id": "1d8bf",
      "labels": {
        "headImageId": "2eec4",
        "headNodeName": "marmot1",
        "syncRole": "follower"
      },
      "name": "ubuntu24.04",
      "nodeName": "marmot2",
      "uuid": "1d8bf5a5-8022-44df-8b62-7ef4ae1eec18"
    },
    "spec": {
      "kind": "os",
      "qcow2Path": "/var/lib/marmot/images/1d8bf/osimage-1d8bf.qcow2",
      "size": 16,
      "type": "qcow2",
      "osName": "ubuntu",  　　// 追加
      "osVersion": "24.04"     // 追加
    },
    "status": {
      "creationTimeStamp": "2026-06-10T03:53:57.852199567Z",
      "lastUpdateTimeStamp": "2026-06-10T03:53:57.852200047Z",
      "message": "ヘッドノードからQCOW2イメージを取得中",
      "status": "AVAILABLE",
      "statusCode": 3
    }
  }
```

## osName と ovVersion で判定して、初期化モジュールを切り替える

現在の pkg/marmotd/server.go は、 Ubuntu 22.04/24.04 を前提に記述されている。
そのため、今回の対応では、OS個別の部分をモジュール化（関数化）して切り出して、
osName, osVersionで切り替えて動作するようにする。

その後、Alpine Linux などのOSとバージョンごとに特化したモジュールを追加することで対応を進める。


## 対応順番
1. APIの追加 osName, osVersion
1. mactl image create|import|export 等で、必須オプションの追加、export形式にメタ情報を追加
1. pkg/marmotd/server.go から Ubuntu22.04/24.04 固有部分を分離して関数化
1. Apline Linux 用モジュールの追加
1. Ubuntu Linux 26.04 用モジュールの追加
1. Rockey Linux 用モジュールの追加
1. Debian Linux 用モジュールの追加


