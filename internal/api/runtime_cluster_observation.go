package api

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func (h *Handlers) observeRuntimeCluster(ctx context.Context, cluster model.RuntimeCluster) model.RuntimeCluster {
	observedAt := time.Now().UTC()
	cluster.LastCheckedAt = &observedAt
	cluster.Status = observation.StatusUnavailable
	cluster.ObservationCode = "runtime_cluster.upstream_unavailable"

	if strings.TrimSpace(cluster.KubeconfigRef) == "" {
		cluster.Status = observation.StatusNotConfigured
		cluster.ObservationCode = "runtime_cluster.kubeconfig_not_configured"
		return cluster
	}
	kubeconfig := strings.TrimSpace(h.secrets.ResolveContext(ctx, cluster.KubeconfigRef))
	if kubeconfig == "" {
		cluster.Status = observation.StatusNotConfigured
		cluster.ObservationCode = "runtime_cluster.kubeconfig_unavailable"
		return cluster
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		cluster.ObservationCode = "runtime_cluster.invalid_kubeconfig"
		return cluster
	}
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := client.Ping(probeCtx); err != nil {
		return cluster
	}
	cluster.Status = observation.StatusReady
	cluster.ObservationCode = ""
	return cluster
}

func (h *Handlers) observeRuntimeClusters(ctx context.Context, clusters []model.RuntimeCluster) {
	const concurrency = 6
	guard := make(chan struct{}, concurrency)
	var wait sync.WaitGroup
	for index := range clusters {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case guard <- struct{}{}:
				defer func() { <-guard }()
			case <-ctx.Done():
				observedAt := time.Now().UTC()
				clusters[index].Status = observation.StatusUnavailable
				clusters[index].ObservationCode = "runtime_cluster.observation_cancelled"
				clusters[index].LastCheckedAt = &observedAt
				return
			}
			clusters[index] = h.observeRuntimeCluster(ctx, clusters[index])
		}()
	}
	wait.Wait()
}
