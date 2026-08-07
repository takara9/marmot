package api

func KubernetesEngineID(k KubernetesEngine) string {
	return k.Metadata.Id
}

func SetKubernetesEngineID(k *KubernetesEngine, id string) {
	if k == nil {
		return
	}
	k.Metadata.Id = id
}
