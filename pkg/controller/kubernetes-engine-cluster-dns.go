package controller

import (
	"fmt"
	"strings"

	"github.com/takara9/marmot/api"
)

// kubernetesEngineClusterDNSImage は、埋め込みマニフェストで使用するCoreDNSの
// コンテナイメージ。バージョンを上げたい場合はここを変更する。
const kubernetesEngineClusterDNSImage = "registry.k8s.io/coredns/coredns:v1.11.3"

// kubernetesEngineClusterDNSServiceIP は、kubeletのclusterDNS設定
// (renderKubernetesEngineKubeletConfig)と一致させる必要があるkube-dns ServiceのClusterIP。
const kubernetesEngineClusterDNSServiceIP = "10.96.0.10"

// kubernetesEngineClusterDNSProbeURLPath は、CoreDNSが既にインストール済みかどうかの
// 判定に使うDeploymentのURLパス。
const kubernetesEngineClusterDNSProbeURLPath = "/apis/apps/v1/namespaces/kube-system/deployments/coredns"

// EnsureKubernetesEngineClusterDNS は、クラスタ内DNS(CoreDNS)をコントロールプレーンの
// APIサーバーへ適用する。kubeletはclusterDNS=10.96.0.10を設定済みだが、CoreDNSを
// 稼働させないとPodからのDNS解決が常に失敗するため、ノードプロビジョニング時に
// 必ず適用する。既にCoreDNSのDeploymentが存在する場合は何もしない（冪等）。
func EnsureKubernetesEngineClusterDNS(ke api.KubernetesEngine) error {
	if ke.Status == nil || ke.Status.ControlPlaneIpAddress == nil || ke.Status.ApiServerPort == nil {
		return fmt.Errorf("KubernetesEngine control plane status is incomplete")
	}
	clusterName := strings.TrimSpace(ke.Metadata.Name)
	namespace, _, _, err := KubernetesEngineControlPlaneNetworkNames(clusterName)
	if err != nil {
		return err
	}
	caPath, _ := KubernetesEngineCAPaths(DefaultKubernetesEnginePkiDir, clusterName)
	adminCertPath, adminKeyPath, err := IssueKubernetesEngineCertificate(DefaultKubernetesEnginePkiDir, clusterName, KubernetesEngineCertRequest{
		Name:          "controller-admin",
		CommonName:    "mke-controller-admin",
		Organizations: []string{"system:masters"},
		Usage:         KubernetesEngineCertUsageClient,
	})
	if err != nil {
		return err
	}
	apiEndpointBase := fmt.Sprintf("https://%s:%d", *ke.Status.ControlPlaneIpAddress, *ke.Status.ApiServerPort)

	installed, err := kubernetesEngineAPIResourceExists(namespace, caPath, adminCertPath, adminKeyPath,
		apiEndpointBase+kubernetesEngineClusterDNSProbeURLPath)
	if err != nil {
		return err
	}
	if installed {
		return nil
	}

	for _, doc := range kubernetesEngineClusterDNSManifests() {
		if err := applyKubernetesEngineManifestObject(namespace, caPath, adminCertPath, adminKeyPath, apiEndpointBase, []byte(doc)); err != nil {
			return err
		}
	}
	return nil
}

// kubernetesEngineClusterDNSManifests は、CoreDNSをkube-system namespaceへデプロイする
// ための一連のマニフェスト（ServiceAccount/ClusterRole/ClusterRoleBinding/ConfigMap/
// Deployment/Service）を返す。kubeadmの標準的なCoreDNS構成に準拠する。
func kubernetesEngineClusterDNSManifests() []string {
	return []string{
		kubernetesEngineClusterDNSServiceAccountYAML(),
		kubernetesEngineClusterDNSClusterRoleYAML(),
		kubernetesEngineClusterDNSClusterRoleBindingYAML(),
		kubernetesEngineClusterDNSConfigMapYAML(),
		kubernetesEngineClusterDNSDeploymentYAML(),
		kubernetesEngineClusterDNSServiceYAML(),
	}
}

func kubernetesEngineClusterDNSServiceAccountYAML() string {
	return `apiVersion: v1
kind: ServiceAccount
metadata:
  name: coredns
  namespace: kube-system
`
}

func kubernetesEngineClusterDNSClusterRoleYAML() string {
	return `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: system:coredns
rules:
  - apiGroups: [""]
    resources: ["endpoints", "services", "pods", "namespaces"]
    verbs: ["list", "watch"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get"]
  - apiGroups: ["discovery.k8s.io"]
    resources: ["endpointslices"]
    verbs: ["list", "watch"]
`
}

func kubernetesEngineClusterDNSClusterRoleBindingYAML() string {
	return `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: system:coredns
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: system:coredns
subjects:
  - kind: ServiceAccount
    name: coredns
    namespace: kube-system
`
}

func kubernetesEngineClusterDNSConfigMapYAML() string {
	return `apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    .:53 {
        errors
        health {
           lameduck 5s
        }
        ready
        kubernetes cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        prometheus :9153
        forward . /etc/resolv.conf {
           max_concurrent 1000
        }
        cache 30
        loop
        reload
        loadbalance
    }
`
}

func kubernetesEngineClusterDNSDeploymentYAML() string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: coredns
  namespace: kube-system
  labels:
    k8s-app: kube-dns
spec:
  replicas: 2
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
  selector:
    matchLabels:
      k8s-app: kube-dns
  template:
    metadata:
      labels:
        k8s-app: kube-dns
    spec:
      serviceAccountName: coredns
      # CorefileのforwardはこのPod自身の/etc/resolv.confを使うため、dnsPolicy未指定(既定のClusterFirst)だと
      # kubeletがclusterDNS=自分自身に書き換えてしまい自己参照ループになる。ノードの実DNSを使わせる。
      dnsPolicy: Default
      containers:
        - name: coredns
          image: %s
          args: ["-conf", "/etc/coredns/Corefile"]
          volumeMounts:
            - name: config-volume
              mountPath: /etc/coredns
              readOnly: true
          ports:
            - containerPort: 53
              name: dns
              protocol: UDP
            - containerPort: 53
              name: dns-tcp
              protocol: TCP
            - containerPort: 9153
              name: metrics
              protocol: TCP
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
              scheme: HTTP
            initialDelaySeconds: 60
            timeoutSeconds: 5
          readinessProbe:
            httpGet:
              path: /ready
              port: 8181
              scheme: HTTP
      volumes:
        - name: config-volume
          configMap:
            name: coredns
            items:
              - key: Corefile
                path: Corefile
`, kubernetesEngineClusterDNSImage)
}

func kubernetesEngineClusterDNSServiceYAML() string {
	return fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: kube-dns
  namespace: kube-system
  labels:
    k8s-app: kube-dns
spec:
  selector:
    k8s-app: kube-dns
  clusterIP: %s
  ports:
    - name: dns
      port: 53
      protocol: UDP
    - name: dns-tcp
      port: 53
      protocol: TCP
    - name: metrics
      port: 9153
      protocol: TCP
`, kubernetesEngineClusterDNSServiceIP)
}
