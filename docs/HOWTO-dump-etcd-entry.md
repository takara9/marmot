# データベースの内容をダンプする方法


marmot のオブジェクト全体のキーをリストする

```
# tools/key.sh
1 : /marmot/volume/e3603
2 : /marmot/volume/de725
3 : /marmot/volume/cd549
4 : /marmot/volume/ca92a
5 : /marmot/volume/7bc80
6 : /marmot/volume/60bf1
7 : /marmot/volume/3994e
8 : /marmot/volume/1cb83
9 : /marmot/volume/02249
10 : /marmot/server/ea875
11 : /marmot/server/d812e
12 : /marmot/server/d3b81
13 : /marmot/server/d16dc
14 : /marmot/server/9a32d
15 : /marmot/server/8fef7
16 : /marmot/server/31aca
17 : /marmot/network/f8662
18 : /marmot/network/cb75b
19 : /marmot/network/a37cd
20 : /marmot/network/4f03e/ip_network/71bb5/ip_address/192.168.1.0/24/192.168.1.61
21 : /marmot/network/4f03e/ip_network/71bb5/ip_address/192.168.1.0/24/192.168.1.60
22 : /marmot/network/4f03e/ip_network/71bb5/ip_address/192.168.1.0/24/192.168.1.50
23 : /marmot/network/4f03e/ip_network/71bb5/ip_address/192.168.1.0/24/192.168.1.109
24 : /marmot/network/4f03e/ip_network/71bb5/ip_address/192.168.1.0/24/192.168.1.102
25 : /marmot/network/4f03e/ip_network/71bb5/ip_address/192.168.1.0/24/192.168.1.101
26 : /marmot/network/4f03e/ip_network/71bb5/ip_address/192.168.1.0/24/192.168.1.100
27 : /marmot/network/4f03e/ip_network/71bb5
28 : /marmot/network/4f03e
29 : /marmot/network/03cc5/ip_network/544b5/ip_address/172.16.10.0/24/172.16.10.2
30 : /marmot/network/03cc5/ip_network/544b5
31 : /marmot/network/03cc5
32 : /marmot/image/77ba7
33 : /marmot/image/02b13
34 : /marmot/hoststatus/ws1
35 : /marmot/dns/net-5/server-50
36 : /marmot/dns/host-bridge/server-61
37 : /marmot/dns/host-bridge/server-60
38 : /marmot/dns/host-bridge/server-50
39 : /marmot/dns/host-bridge/server-109
40 : /marmot/dns/host-bridge/server-102
41 : /marmot/dns/host-bridge/server-101
42 : /marmot/dns/host-bridge/server-100
Enter the line number to view the value: 31
{
  "apiVersion": "v1",
  "kind": "VirtualNetwork",
  "metadata": {
    "comment": "プライベートな仮想ネットワークで、VXLANで別ノードに配置されたVM間の疎通を確保します。",
    "id": "03cc5",
    "labels": {
      "headNetworkId": "",
      "headNodeName": "ws1",
      "syncRole": "head"
    },
    "name": "net-5",
    "nodeName": "ws1",
    "uuid": "03cc50c5-3961-488d-a8e2-f4237cbf9006"
  },
  "spec": {
    "bridgeName": "br-03cc5",
    "forwardMode": "bridge",
    "iPNetworkAddress": "172.16.10.0/24",
    "ipNetworkId": "544b5",
    "overlayMode": "vxlan",
    "peerPolicy": "auto",
    "underlayInterface": "enp2s0",
    "vni": 101
  },
  "status": {
    "creationTimeStamp": "2026-05-16T09:55:54.716922465+09:00",
    "lastUpdateTimeStamp": "2026-05-16T09:55:54.716922575+09:00",
    "message": "provisioning:in-progress",
    "status": "ACTIVE",
    "statusCode": 2
  }
}
```