# MKS (Marmot Kubernetes Engine)の設計

marmot のユーザーが、
- 抽象化された KubernetesEngine の APIを使用して、
- Service の spec.type=LoadBalancer と設定と連動して、外部ネットワークからアクセスを受け入れ可能な
- ノード間で利用可能な共有ストレージが利用可能な 
Kubernetesクラスタを利用できるようにすることを実現する。


## MKSの役割と機能

- Kubernetesを marmot の仮想サーバー群にイントールして、Kubernetesクラスタを構築する。
- 以下のYAMLファイルの記述から、Kubernetesのコントロールプレーンのプロセスを起動して、ワーカーをデプロイして、Kubernetesクラスタを構成する。初期リリースでは、コントロールプレーンはシングル構成とする。
- YAMLは可能な限り抽象化して、ユーザーが簡単にK8sクラスタを入手できるようにする

```yaml
apiVersion: v1
kind: KubernetesEngine
metadata:
    name: test-cluster-1
    comment: 検証用K8sクラスタ
spec:
    version: 1.36
    nodes: 3
    nodeSpec:
        cpu: 4
        memory: 8192
        network:
            cni-plugin: none # Cilliumを選択可能とする
            external: host-bridge | default
```

- YAMLに指定された Kubernetes バージョンのインストール、または、アップグレードができる。
- Ceph CSIを自動でインストールして、ファイルストレージ、ブロックストレージをアクセスできる。
- K8sノードになる仮想マシンは、「外部アクセス用ネットワーク」、「ノード間通信用ネットワーク」、「ストレージ用ネットワーク」に接続する。
- ノード上のポッド間は、ノード間通信用ネットワークを利用して、ノード上のポッドと通信可能にする。
- ノード間通信用ネットワーク用に、Cilliumを選択できる。
- YAMLの spec.nodes を変更することで、ノードを増減できる。
- 「外部アクセス用ネットワーク」は、ノードがインターネット上のコンテナリポジトリにアクセスするために必須なので、必ず接続が必要となる。仮想ネットワークは、default と host-bridge のどちらかに接続できる。default を選択した場合、mke専用ロードバランサーは使用できない。
- 「ノード間通信用ネットワーク」は、クラスタ毎にノード間通信用ネットワークが作成され接続されるので、指定する必要は無い。
- Cephへの接続は必須なので、「モニターのIPアドレス」、「ストレージ用ネットワーク」などの設定は、marmotd.json で配置する。
- spec.versionは、対応可能なバージョンを特に限定しない。 コントローラーから挙がったエラーを返す。


## アーキテクチャ設計

- コントロールプレーンは、クラスタ毎に専用のVMのマスターノードを配置せず、marmotdが稼働するサーバー上に、コントロールプレーンを配置する。kube-api-server等はポート番号を変えることで、K8sクラスタ毎に専用のコントロールプレーンを持つことができるようにする。
- KubernetesEngineのコントローラーが、Kubernetesクラスタの生成から削除までのライフサイクルを管理する。
- etcdは、K8sクラスタ毎に専用のetcdプロセスを marmotd が稼働するホスト上に起動する。marmotd自身が使用するetcdとは別プロセスとし、クラスタ間のデータを完全に分離する。
- クラスタ毎にetcdのクライアントポート・ピアポートを個別に割り当てる。ポート番号はKubernetesEngineのコントローラーがクラスタ作成時に採番し、他クラスタのetcdやmarmotdのetcdと衝突しないポートを選択したうえで、kube-apiserverのマニフェストファイルの --etcd-servers に設定する。
- クラスタ専用のetcd、kube-apiserver等のコントロールプレーンプロセスは、marmotホスト上のsystemdユニットとして起動する。ユニットファイルの生成・起動・停止・削除を含む作成から削除までのライフサイクル全体は、KubernetesEngineコントローラーの責務とする。
- kube-api-serverなど、クラスタ固有のKubernetesプロセスは、クラスタ毎に作成される「ノード間通信用ネットワーク」とブリッジ接続して、ノードのエージェントとポッドから参照可能にする。
- ワーカーノードの kubelet, kube-proxy, containerd は、kubenetesコントローラーがインストールする。そのため、Kubernetsコントローラーは、独立のプロセスとして実行して「ノード間通信用ネットワーク」と連携できるようにする。
- Kubernetesクラスタを構成するために共通な初期設定情報は、/etc/marmot/mks.jsonに保存しておき、デフォルト値などとして利用する。


## KubernetesEngineのコントローラーの役割
- KubernetesEngineが、etcdの /marmotから始まる オブジェクトを監視して、k8sクラスタの情報が書き込まれたら、活動を開始する。
- 「ノード間通信用ネットワーク」を作成する。
- marmotホスト上で、クラスタ専用のetcdプロセスを起動する。他クラスタ・marmotdのetcdと衝突しない空きポートを採番し、クラスタのメタデータとして記録する。
- コントロールプレーンとノード間を繋ぐための証明書を発行する
- クラスタ専用のsystemdユニットファイルを生成し、Kubernetesのコントロールプレーンのプロセスを起動する。各ユニットの作成・起動・停止・削除まで、KubernetesEngineコントローラーが一貫して管理する。
    - kube-apiserver // 起動して「ノード間通信用ネットワーク」と連携させる。--etcd-serversに、採番したクラスタ専用etcdのポートを設定
    - kube-scheduler // 同上
    - kube-controller-manager // 同上
    - cloud-controller-manager // 必要でなければ起動しないが、今後検討
    - mke-controller // marmot のKubernetesEngineコントローラーから起動されるプロセス、仮想ネットワーク経由でノードにアクセスするために別プロセス
- ノードの仮想マシンを起動する
    - 証明書を配布する
    - kubelet をイントールする
    - containerd と接続に必要なソフトウェアをインストールする 
    - kube-proxy をインストールする
- 初期段階のKubenetesクラスタを構成する
- CNIを有効化してポッドネットワークを活性化する
- CSIを有効化してCephストレージと連携可能にする
- kubectl コマンドから APIサーバーにアクセスできるように、クライアント証明書を etcdの /marmot/mke/id/client-cert等に格納する。
- リコンサイシルループをまわして、ノード要求数の変化、ノード実態の稼働数の変化等をチェックして、要求数と稼働数が合致しなければ、必要なアクションを実施する。
- kube-apiserverと連携するロードバランサーを起動して、Ssrvice のイベントを監視して、該当するサービスが起動したら、ロードバランサーのプロセスを設定する。
- Kuberntesクラスタが削除されたら、以下を削除してクリーンナップする。
  - 仮想ネットワーク
  - ストレージ
  - 仮想サーバー
  - コントロールプレーンのプロセス群（クラスタ専用etcd、kube-apiserver等のsystemdユニットの停止・無効化・ユニットファイル削除を含む）


## 設計上の妥協点
- コントロールプレーンがホストに同居 → 単一障害点が全クラスタに波及
    marmotのコントロール部分はシングル構成のため、それ以上を求めても意味がないため、初期リリースでは考慮しない。
- クラスタ毎の専用etcdによるリソース消費増加
    クラスタ数に比例してetcdプロセスのメモリ・ディスクI/Oが増加するが、データ分離の安全性を優先し許容する。
- ネットワークの二重カプセル化
    シングルノード構成では、OVSブリッジに接続となり、Cilliumを使わないブリッジ接続では、ノードの設定に手間がかかるが、二重カプセルを回避する方法があるので、問題にしない。
- marmotdホストへの多数のブリッジ接続の拡張性
    各クラスタのPod/Service CIDRの重複があっても、内部ナットワークのIPで外部と疎通することは無いので、問題にしない。marmotが管理するOVSブリッジによる仮想ネットワークは、marmotがIPAMを実施するので重複は発生しない。
- Ceph CSIに必要な特権・カーネルモジュールの前提
    Kubernetesノードには、必要に応じて Cephのクライアントモジュールと /etc/cephの設定を入れる。
- 「任意バージョンサポート」の運用負荷
    実質的に、使えるのは、K8sがサポートする３バージョンに限られるが、古いもののトラブル検証などに使いたいため、この部分は許容する。
- mke専用ロードバランサーは、シングル構成とする。将来、可用性の確保を推進する時、KeepAlivedを使いアクティブスタンバイ構成をとる。


# 実行の進め方

このドキュメントの依存関係を見ると、「API定義 → コントローラーの骨格 → ネットワーク → 専用control plane → ノード → CNI/CSI → 外部公開 → リコンサイル/削除」の順で積み上げるのが自然です。以下のフェーズ分けを提案します。

## フェーズ0: 準備
- `/etc/marmot/mks.json` の初期設定スキーマ定義（バージョンデフォルト、ネットワーク種別のデフォルトなど）
- 既存の marmotd コントローラー（Server/Network/Volume等）のパターンを参考に、KubernetesEngine用の雛形を用意

## フェーズ1: API定義
- `marmot-api-v1.yaml` に `KubernetesEngine` リソース（`spec.version`, `spec.nodes`, `spec.nodeSpec` 等）を追加し、コード生成
- `mactl create/get/delete` 相当の CRUD コマンドを先に用意（この時点ではコントローラーは動かず、etcdに書き込むだけでOK）
- 理由: 他の全フェーズがこのスキーマに依存するため最初に固める

## フェーズ2: コントローラーの骨格とライフサイクル状態機械
- `/marmot/...` プレフィックスの watch → 生成イベント検知の雛形
- クラスタの状態遷移（Pending → Provisioning → Running → Deleting 等）とetcdへの状態書き込みだけを先に実装
- 理由: 以降の各フェーズを「状態遷移の1ステップ」として差し込める土台になる

## フェーズ3: ノード間通信用ネットワーク
- クラスタ作成時に専用ネットワークを1つ作る/削除する部分だけを実装
- 既存のNetwork機能を流用できるので、比較的独立して早期に着手・検証可能

## フェーズ4: クラスタ専用etcdのポート採番とsystemdライフサイクル
- 空きポート採番ロジック（他クラスタ・marmotd自身のetcdと衝突しないこと）
- systemdユニットファイル生成・起動・停止・削除（`mke-etcd-<cluster>.service` のようなユニット）
- ここで「作成→起動→停止→削除」の一連を単体で検証しておく（後続のkube-apiserver等も同じユニット管理基盤を再利用するため）

## フェーズ5: PKI（証明書発行）
- コントロールプレーン⇔ノード間の証明書発行機構
- 単体でCA/証明書発行ロジックをテスト可能

## フェーズ6: コントロールプレーンプロセス群
- kube-apiserver, kube-scheduler, kube-controller-manager を専用etcdに向けてsystemdユニットとして起動
- この時点で `curl https://localhost:<port>/healthz` 等で単体クラスタの疎通確認ができる（ノードはまだ無い）

## フェーズ7: ノードVMプロビジョニング
- VM起動、証明書配布、kubelet/containerd/kube-proxyインストール、ノード間通信用ネットワークへの接続
- ここで初めて `kubectl get nodes` が通る状態になる

## フェーズ8: CNI有効化
- Bridge選択時は、設定とルーティング設定追加
- Cilium選択時のインストール・設定
    - mke/cni-ciliumの下にある YAMLファイル群を適用して、ciliumを有効化する。

## フェーズ9: CSI（Ceph）連携
- Ceph CSIの自動インストールとストレージ用ネットワーク接続
- mke/ceph-fs, mke/ceph-rbd の下に CEPHのための CSIのYAMLを配置
- インストール時に、mkeの下のマニフェストを /var/lib/marmot/mke-manifests にコピーする
- mke.jsonにセットするべき項目（イントール後にユーザーがセットする）
    - clusterID: 文字列
    - monitors["IP1:PORT","IP2:PORT","IP3:PORT"]
    - rbdUserId: 文字列
    - rbdUserKey: 文字列
    - cephfsUserId: 文字列
    - cephfsUserKey: 文字列
- mkeのインスタンス作成時に、/var/lib/marmot/mke-manifests 以下を /var/lib/marmot/mke-manifests/<MKE-NAME> にコピーする
- mkeのインスタンス作成時に、mke.jsonから 指定項目をセットする
    - /var/lib/marmot/mke-manifests/<MKE-NAME>/csi-config-map.yaml に cluseterIDをセット、monitors のIPアドレスとポートをセット
    - /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-rbd/csi-rbd-secret.yaml に RBDのユーザーIDとユーザーKeyをセットする
    - /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-rbd/rbd-storageclass.yaml に clusterIDをセットする
    - /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-fs/csi-cephfs-secret.yaml に CEPHFSのユーザーIDとユーザーKeyをセットする
    - /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-fs/cephfs-storeageclass.yaml にclusterIDをセットする
- mke の K8sが起動後に、/var/lib/marmot/mke-manifestsから、以下の順番で、ネームスペース kube-system に、kubectl apply -f を実施する
   1. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-conf.yaml
   2. /var/lib/marmot/mke-manifests/<MKE-NAME>/csi-config-map.yaml
   3. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-rbd/csi-provisioner-rbac.yaml
   4. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-rbd/csi-nodeplugin-rbac.yaml
   5. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-rbd/csi-rbdplugin-provisioner.yaml
   6. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-rbd/csi-rbdplugin.yaml
   7. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-rbd/csidriver.yaml
   8. /var/lib/marmot/mke-manifests/<MKE-NAME>/kms/vault.yaml
   9. /var/lib/marmot/mke-manifests/<MKE-NAME>/kms/csi-vaulttokenreview-rbac.yaml
  10. /var/lib/marmot/mke-manifests/<MKE-NAME>/kms/kms-config.yaml
  11. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-rbd/csi-rbd-secret.yaml
  12. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-rbd/rbd-storageclass.yaml
  13. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-fs/csi-provisioner-rbac.yaml
  14. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-fs/csi-nodeplugin-rbac.yaml
  15. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-fs/csi-cephfsplugin-provisioner.yaml
  16. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-fs/csi-cephfsplugin.yaml
  17. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-fs/csidriver.yaml
  18. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-fs/csi-cephfs-secret.yaml
  19. /var/lib/marmot/mke-manifests/<MKE-NAME>/ceph-fs/cephfs-storageclass.yaml

- mkeのインスタンスを削除するときは、以下を削除する
    - /var/lib/marmot/mke-manifests/<MKE-NAME>

## フェーズ10: kubectlアクセス経路
- kube-apiserverなど、コントロールプレーンのKubernetesプロセスは、marmotdが稼働するホストのIPアドレスでアクセス可能にする。
- クライアント証明書を `/marmot/mke/id/client-cert` 等へ格納し、外部からのアクセス経路を確立
- kubectl が使用する KUBECONFIGを `$HOME/.kube/config` にセットして、mactl コマンドを実行しているホームディレクトリから kubectl でK8sクラスタにアクセス可能にする。

## フェーズ11: LoadBalancer連携（mke-controller）
- KubernetesEngineでは、mkeインスタンス用のmke専用ロードバランサー用の仮想サーバー（1CPUコア, RAM 1G）を起動する。
- mke専用ロードバランサー用の仮想サーバーは、mkeのノード間通信用仮想ネットワークを通じて、kube-apiserverにアクセスする。
- MKEコントローラーは、専用ロードバランサーがkube-apiserverへアクセスできるように kubeconfigを作成して、提供する。
- この仮想サーバーのロードバランサーと、KubernnetsのノードのNodePortは、host-bridgeを経由して、リクエストトラフィックをK8s内部のサービスとポッドへ導く。ロードバランサーとの連携はhost-bridgeで固定として、NATが必要となるdefault を使用しない。
- mke専用ロードバランサーの仮想サーバーには、「HA-PROXY」と「ロードバランサーコントローラー」を起動する。
- 「ロードバランサーコントローラー」は、https://github.com/takara9/marmot-servers/blob/main/kubernetes/playbook/loadbalancer_controller/templates/loadbalancer_controller を参考に、Go言語で開発する。
- 「ロードバランサーコントローラー」は、kube-apiserverのクライアントとして、自身のリコンサイルループで、以下を実行する。
    - mkeのインスタンス(Kubernetesクラスタ)の全ノードの host-bridge に接続された address を収集する。
        - mke の kube-apiserverにアクセスして、kind=node の `metadata.lables.kubernetes.io/hostname` からホスト名を取得する
        - marmotd をアクセスして、`spec.networkInterface[].networkname="host-bridge"` の address を取得する。
    - mke の kube-apiserverにアクセスして、全てのネームスペースのサービスで、type="LoadBalancer"をサーチする
        - 処理対象のServiceが見つかったら、`spec.ports[].nodePort` を取得する
    - HA-Proxyの設定ファイルを変更して、K8s内 Service type="Loadbalancer" で指定された Service の NodePortに対して、受け取ったリクエストを分散転送する。
    - リクエスト受取用のIPアドレス(VIP)をK8s内 Service type="Loadbalancer" のEXTERNAL-IPにセットする。
    - 「ノード」、「Type="LoadBalancer"」のサービスが存在しなくなった場合は、HA-PROXYの設定を削除する。
    - VIPの払い出しは、marmotd.json の host-bridge-ip-addr-start から host-bridge-ip-addr-end で実施する。
    - VIPを払い出しと伴に、サービス名.ネームスペース名.MKEクラスタ名.HVホスト名.labo.localで、marmotd の内部DNSへ登録する。
    - サービスが削除される際に、内部DNSに登録したエントリーも削除し、VIPに使用したIPアドレスをIPAMプールへ返却する。
- MKEのノードコントローラーは、ノードとなる仮想サーバーの host-bridge に接続されたI/FのIPアドレスを、nodeの EXTERNAL-IPに表示されるように IPアドレスをセットする。

## フェーズ12: リコンサイルループ本実装
- 変更可能フィールドは`spec.nodes`と`spec.version`のみに限定
- `spec.nodes` の増減に追従するノードのスケールイン/アウト
- `nodeSpec(cpu/memory/networkの種類)`は起動済みノードに反映不能なため変更禁止
- スケールアウト(RUNNING時)
    - 仮想サーバーを起動して、ノードとしてセットアップして、Kubernetesクラスタの参加
- スケールイン(新規実装)
    - 対象ノードの選定: 命名規則-node-<index>を利用し、index降順(一番大きい番号から)で削除
    - 手順案: kubectl cordon → kubectl drain --ignore-daemonsets --delete-emptydir-data → kubectl delete node → サーバー(VM)削除要求 → IPAM/内部DNS後始末
    一度に1ノードずつのローリング処理(可用性維持のため)
    - 要調査点: Ceph CSIで払い出したPVがそのノードにpinされているケースの扱い
- バージョンアップグレード
    - spec.version変更検知時、マイナーバージョンが2つ以上飛んでいないかバリデーション(K8sは1マイナーずつが原則)
    - コントロールプレーン→ノードの順でローリング更新(cordon→drain→バイナリ差し替え→kubelet再起動→uncordon→Ready確認→次ノードへ)
    - 特定ノードで失敗した時は、エラーメッセージの表示まで。
    - etcd の アップグレードも対象とする。
- バージョンダウングレードは、サポートしない。
- mkeがノード増減、K8sのアップグレード中は、`mactl get mke` に変更操作中`UPGRADING`, `SCALING_IN`, `SCALING_OUT`, であることを表示すること。

## フェーズ13: 削除・クリーンナップ
- 仮想ネットワーク、ストレージ、仮想サーバー、コントロールプレーンプロセス群（専用etcd/kube-apiserver等）の一貫した削除処理

## フェーズ14: Kubernetes の Cloud Controller Manager（CCM）を導入

### 1. `cloud-provider` インターフェースの実装
- CCM が要求する Go interface（`cloudprovider.Interface`）を実装する `marmot-cloud-provider` （仮称）を新規開発する。
- 実装対象のサブインターフェースを選定する:
  - `LoadBalancer`: `EnsureLoadBalancer` / `UpdateLoadBalancer` / `EnsureLoadBalancerDeleted` を実装し、既存の「ロードバランサーコントローラー」（フェーズ11で開発したHA-Proxy連携ロジック）を、この標準インターフェースの内部実装として移植する。
  - `Instances`/`InstancesV2`: `InstanceExists` / `InstanceShutdown` / `InstanceMetadata` を実装し、marmotd の仮想サーバー状態（Server API）と連携してノードのライフサイクル情報を返す。
  - `Zones`: シングルクラスタ構成では不要と判断するか、marmotホスト名などをリージョン/ゾーンとして仮に返すか検討、リージョンとゾーンはmke.jsonにセットした値を返す。
  - `Routes`: OVS/Cilium 側でルーティングを完結しているため、実装不要と判断できるか検証する。
- **実施内容**:
  - k8s.io/cloud-provider非依存の方針を維持したまま、`pkg/cloudprovider.InstanceMetadata`(既存フィールド`Region`/`Zone`)に、`mke.json`の`MKEConfig.CloudProviderRegion`/`CloudProviderZone`由来の値を設定するようにした(`pkg/cloudprovider/marmotd_instances.go`の`NewMarmotdInstances`にregion/zone引数を追加)。独立した`Zones`インターフェースは追加していない(未設定時は空文字列のまま、Region/Zoneどちらも省略可能)。
  - `cmd/mke-node-controller`に`--region`/`--zone`フラグを追加し、`pkg/controller/kubernetes-engine-cloudprovider-provision.go`の`ProvisionKubernetesEngineCloudControllerManager`が`mkeConf.CloudProviderRegion`/`CloudProviderZone`が設定されている場合のみsystemdユニットのExecStartに`--region=`/`--zone=`を付与する。
  - 検証はユニットテストのみ(`go test ./pkg/cloudprovider/...`、`go test ./pkg/controller/...`)。`kubectl get nodes`での実際の反映確認は項目7に持ち越し。

### 2. 認証・API連携の設計
- CCM から marmotd API（Server/Network情報取得）へアクセスするための認証情報（APIキー等）の発行・配布方法を決める。
- kube-apiserver との通信用 kubeconfig を、他コントロールプレーンプロセス同様に発行・配置する。

### 3. systemd ユニットとしての起動管理
- `cloud-controller-manager` 用の systemd ユニットファイルをクラスタごとに生成する仕組みを、既存の kube-apiserver 等と同じライフサイクル管理基盤（フェーズ4/6で構築済みの仕組み）に統合する。
- `--cloud-provider=external` を kubelet / kube-controller-manager に設定し、CCM を有効化する起動オプションの変更を反映する。

### 4. 既存のノード初期化フローとの整合
- 現状「MKEのノードコントローラーが host-bridge IP を EXTERNAL-IP にセット」しているフェーズ11の処理を、CCM の `InstanceMetadata`（`ProviderID`, `NodeAddresses`）による標準的な設定に置き換えるか、共存させるかを決定する。
- 各ノードに `--cloud-provider=external` を設定した場合、CCM が初期化するまで `node.cloudprovider.kubernetes.io/uninitialized` taint が付与される点への対応（CCM起動タイミングとノード起動タイミングの順序保証）を設計する。
- **方針（決定事項）**:
  - 現状の `mke-lb-controller`（`cmd/mke-lb-controller`）は、`kubeclient.go` の `SetNodeAddresses` で `node.status.addresses` に InternalIP/ExternalIP(host-bridge IP) を直接PATCHしている。CCM導入後は、CCMの `Instances.InstanceMetadata` が同じ情報（`ProviderID`, `NodeAddresses`）を提供するため、両者が同時に書き込むと競合する。
  - そのため、**クラスタ単位でCCM有効/無効を排他的に切り替える**方式とする。CCMが有効なクラスタでは `mke-lb-controller` 側の `SetNodeAddresses` 呼び出しを行わない（LoadBalancer VIP管理も、項目5の実施内容の通りCCM有効/無効で排他的に切り替える）。
  - taint対応: kubeletに `--cloud-provider=external` を設定するクラスタでは、ノードVM起動前に対象クラスタのCCM(`cloud-controller-manager`)が起動済みであることを起動順序として保証する（「CCM起動 → ノードVM起動」の順）。CCMが `InstanceMetadata` を返せるようになった時点でuninitialized taintは自動解消されるため、追加のポーリング処理は不要。
  - 本項目時点ではCCM本体（実行可能バイナリ）・kubelet/kube-controller-managerへの `--cloud-provider=external` 設定はいずれも未実装のため、上記は方針の明記のみとし、コード変更は行わない。
- **実施内容**:
  - kubeletのsystemdユニット（`pkg/controller/kubernetes-engine-node-ssh.go`の`kubernetesEngineNodeKubeletUnit`）に、`mkeConf.CloudControllerManagerEnabled`が true のクラスタのノードのみ `--cloud-provider=external` を付与するようにした（`kubernetesEngineNodeProvisionData.CloudProviderEnabled`経由で`pkg/controller/kubernetes-engine-node.go`の`configureKubernetesEngineNodeBinaries`から伝播）。
  - `mke-lb-controller`（`cmd/mke-lb-controller`）に `--cloud-controller-manager-enabled` フラグを追加し、trueの場合は`reconcile()`内の`SetNodeAddresses`呼び出しをスキップするようにした（host-bridgeアドレス収集・HAProxy backend生成は継続）。VIP払い出し/解放(`requestVip`/`releaseVip`)も、同フラグがtrueの場合はスキップするようにした(項目5参照。VIP管理はmke-node-controllerに一本化)。このフラグは、`pkg/controller/kubernetes-engine-loadbalancer-ssh.go`の`kubernetesEngineLoadBalancerControllerUnit`が生成するsystemdユニットのExecStartに、`mkeConf.CloudControllerManagerEnabled`が true の場合のみ追加される（`pkg/controller/kubernetes-engine-loadbalancer.go`から伝播）。
  - 「CCM起動 → ノードVM起動」の順序保証は、`reconcileKubernetesEngineProvisioning`（`pkg/controller/kubernetes-engine-controller.go`）が同一reconcile呼び出し内で`provisionKubernetesEngineControlPlane`（CCMプロビジョニングを含む、フェーズ14項目3・6を参照）を`provisionKubernetesEngineNodes`より先に呼び出す既存の順序で自然に満たされているため、追加の順序制御ロジックは実装していない。
  - 検証はユニットテストのみ（`go test ./pkg/controller/...`、`go test ./cmd/mke-lb-controller/...`）。CCM本体との統合検証は項目7に持ち越し。

### 5. 既存 LoadBalancer 連携ロジックの置き換え／共存方針
- フェーズ11で実装済みの「ロードバランサーコントローラー」の VIP払い出し・内部DNS登録ロジックを、CCM の `LoadBalancer` インターフェース実装に移植するか、既存プロセスをそのまま残しCCMは未導入のままとするかの方針を決定する（移植コストと利点の比較）。
- **方針（決定事項）**:
  - `k8s.io/cloud-provider` には依存せず、marmot-native な `pkg/cloudprovider.LoadBalancer` インターフェース（`EnsureLoadBalancer`/`EnsureLoadBalancerDeleted`）を新設し、フェーズ11の `mke-lb-controller`（`marmotdclient.go`の`requestVip`/`releaseVip`）と同等のVIP払い出し・解放ロジックを `MarmotdLoadBalancer` として移植した。既存の「k8s.io依存を追加しない」方針は維持している。
  - `mke-lb-controller` プロセス自体は削除せず、HAProxy連携（VIPを使ったbackend生成）はそのまま維持する。VIP払い出し/解放の**呼び出し元のみ**を、クラスタのCCM有効/無効設定に応じて `mke-lb-controller` と `mke-node-controller`（CCM相当）のどちらか一方に排他的に切り替える（項目4のSetNodeAddresses排他化と同じ設計パターン）。
- **実施内容**:
  - `pkg/cloudprovider/interface.go` に `LoadBalancerService{Namespace, Name}` と `LoadBalancer` インターフェースを追加。
  - `pkg/client/kubernetes_engine.go` に既存のmarmotd REST API（`/kubernetes-engine/{id}/loadbalancer/vip`のPOST/DELETE、フェーズ11で実装済み・冪等）を呼び出す `CreateKubernetesEngineLoadBalancerVip`/`DeleteKubernetesEngineLoadBalancerVip` を追加。
  - `pkg/cloudprovider/marmotd_loadbalancer.go`（新規）に `MarmotdLoadBalancer` を実装。`EnsureLoadBalancer`はVIP払い出し後に空文字列でないことを検証し、`EnsureLoadBalancerDeleted`はmarmotd側が冪等（未払い出しVIPの削除もHTTP 200）なため404等の特別処理は行わない。
  - `cmd/mke-node-controller` に `ListLoadBalancerServices`/`SetServiceLoadBalancerIngressIP`（`kubeclient.go`）と `reconcileLoadBalancer`（`reconcile.go`）を追加し、`type=LoadBalancer` かつVIP未設定のServiceにVIPを払い出し `status.loadBalancer.ingress` へ反映、Service削除時はVIPを解放するループを実装（`knownServiceVIPs`でクロスイテレーション状態を保持）。`main.go`に`--region`/`--zone`同様の配線を追加。
  - `cmd/mke-lb-controller/main.go` の `reconcile()` 内、VIP払い出し(`mClient.requestVip`)呼び出しとVIP解放(`mClient.releaseVip`)呼び出しの両方を、既存の`SetNodeAddresses`と同じ `cloudControllerManagerEnabled` フラグでスキップするように変更し、`mke-node-controller`との排他性を確保した（HAProxy backend生成自体は、既にServiceに設定済みのVIPを消費するだけなので無条件のまま）。
  - 検証はユニットテストのみ（`go test ./pkg/cloudprovider/...`、`go test ./cmd/mke-node-controller/...`、`go test ./cmd/mke-lb-controller/...`）。実クラスタでのVIP払い出し/内部DNS登録の統合確認は項目7に持ち越し。

### 6. クラスタ削除時のクリーンナップ対応
- フェーズ13の削除処理に、CCM 用 systemdユニットの停止・無効化・ユニットファイル削除を追加する。
- **実施内容**:
  - `DeprovisionKubernetesEngineControlPlane`（`pkg/controller/kubernetes-engine-control-plane.go`）に `DeleteKubernetesEngineCloudControllerManagerUnit` の呼び出しを追加した。他の解体ステップと同様にベストエフォートで実行し、失敗してもエラーを集約するのみでIPAM解放等の後続処理を継続する。CCM未導入クラスタ（ユニット不在）でも冪等に成功する。
  - CCM用APIKeyの失効（`revokeKubernetesEngineCloudProviderApiKey`）は見送った。発行済みAPIKeyのkeyIDを永続化する仕組み（LB用途では`Server`のラベルに保存）が無く、CCMプロビジョニング自体もまだ作成フローに組み込まれていないため。CCMプロビジョニングをクラスタ作成フローへ統合する段階で、keyIDの永続化とあわせて失効処理も追加する。

### 7. 検証
- `kubectl get nodes` で `EXTERNAL-IP` / `PROVIDER-ID` が正しく反映されることを確認する。
- Service `type=LoadBalancer` 作成時に、既存のHA-Proxy連携と同等の動作（VIP払い出し、内部DNS登録）がCCM経由で行われることを確認する。
- **方針（決定事項）**:
  - 現時点で実施済みの検証: 項目1（Region/Zone設定伝播）・項目2（APIKey発行/kubeconfig生成）・項目3（systemdユニット生成/起動/削除）・項目5（marmot-native LoadBalancer、VIP払い出し/解放の排他制御）・項目6（削除フローへの組み込み）は、いずれもユニットテストでのみ検証済み。`go test ./pkg/cloudprovider/... ./pkg/controller/... ./cmd/mke-node-controller/... ./cmd/mke-lb-controller/...` で確認しており、`pkg/controller`の既知の無関係な事前既存フレーキー3件（`image-controller_test.go`の認証トークンマスキング、`kubernetes-engine-network-lifecycle_test.go`の削除待ちタイミング依存2件）を除き全てPASSしている。他パッケージ（`pkg/cloudprovider`/`cmd/mke-node-controller`/`cmd/mke-lb-controller`）は全PASS。
  - `kubectl get nodes`でのEXTERNAL-IP/PROVIDER-ID確認、およびService type=LoadBalancer経由のVIP払い出し/内部DNS登録の実クラスタでの確認は、実CCMバイナリの実装とクラスタ作成フローへの組み込み（`k8s.io/cloud-provider`依存追加を伴う、本フェーズの対象外）が完了するまで持ち越す。本フェーズ（項目7）の検証範囲はユニットテストのみとし、これ以上のコード変更は行わない。

---

**ポイント**:
- フェーズ4（専用etcd + systemdライフサイクル管理）を早めに固めることが重要です。これはkube-apiserver等の他コントロールプレーンプロセスも同じ仕組みを再利用するため、ここで作成・削除の一貫性を検証しておくと後工程が楽になります。
- 「設計上の妥協点」にある通り、コントロールプレーンは初期リリースでシングル構成前提なので、HA化やマルチcontrol-planeは後回しでよく、まずはフェーズ1〜10で**単一ノードのK8sクラスタが動く最小構成**を通すことを優先するのが良いと思います。