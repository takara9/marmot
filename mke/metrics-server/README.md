# Metrics server

引用元: https://github.com/kubernetes-sigs/metrics-server

Metrics Serverは、Kubernetesに組み込まれたオートスケーリングパイプライン向けの、スケーラブルで効率的なコンテナリソースメトリクスを提供するツールです。

Metrics Server は Kubelet からリソース メトリクスを収集し、Metrics APIを介して Kubernetes apiserver に公開して、Horizo​​ntal Pod AutoscalerおよびVertical Pod Autoscaler で利用できるようにします。Metrics API は からもアクセスできるためkubectl top、オートスケーリング パイプラインのデバッグが容易になります。


## Metrics Serverは以下の用途に使用できます。
- CPU/メモリベースの水平オートスケーリング
- コンテナに必要なリソースを自動的に調整／提案します


## インストール
次のマニフェストを使用
https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

ただし、https://github.com/kubernetes-sigs/metrics-server#configuration に記述される設定の追加が必要


## API Server の設定追加

https://kubernetes.io/docs/tasks/extend-kubernetes/configure-aggregation-layer/#enable-kubernetes-apiserver-flags


## mke での自動適用

このディレクトリの `components.yaml` は、mke のノードプロビジョニング時に
`EnsureKubernetesEngineMetricsServer`([kubernetes-engine-metrics-server.go](../../pkg/controller/kubernetes-engine-metrics-server.go))
によって自動的にコントロールプレーンへ適用される(既にインストール済みの場合は何もしない)。
上記の API Server 設定(`--requestheader-*`/`--proxy-client-*`/`--enable-aggregator-routing`)は、
front-proxy 専用CAとproxy-clientクライアント証明書とともに
`EnsureKubernetesEngineControlPlaneAssets`/`renderKubernetesEngineControlPlaneUnits` が
kube-apiserver起動時に自動で付与する。

