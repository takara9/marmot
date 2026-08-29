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
            kind: none # Cilliumを選択可能とする
            external: host-bridge | default
```

- YAMLに指定された Kubernetes バージョンのインストール、または、アップグレード、ダウングレードができる。
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
- `spec.nodes` の増減に追従するノードのスケールイン/アウト

## フェーズ13: 削除・クリーンナップ
- 仮想ネットワーク、ストレージ、仮想サーバー、コントロールプレーンプロセス群（専用etcd/kube-apiserver等）の一貫した削除処理

## フェーズ14: バージョンアップグレード/ダウングレード対応
- 最後に対応（運用上の追加機能のため、MVP後でよい）

---

**ポイント**:
- フェーズ4（専用etcd + systemdライフサイクル管理）を早めに固めることが重要です。これはkube-apiserver等の他コントロールプレーンプロセスも同じ仕組みを再利用するため、ここで作成・削除の一貫性を検証しておくと後工程が楽になります。
- 「設計上の妥協点」にある通り、コントロールプレーンは初期リリースでシングル構成前提なので、HA化やマルチcontrol-planeは後回しでよく、まずはフェーズ1〜10で**単一ノードのK8sクラスタが動く最小構成**を通すことを優先するのが良いと思います。