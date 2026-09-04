package worker

import (
	"context"
	"errors"
	"strings"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func (r *Runner) projectVolumeProvider(ctx context.Context, clusterID string) (kubeprovider.ProjectVolumeProvider, error) {
	if ctx == nil {
		panic("project volume provider context is required")
	}
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return nil, errors.New("runtime cluster id is required")
	}
	if r.projectVolumeProviderFactory != nil {
		return r.projectVolumeProviderFactory(ctx, clusterID)
	}
	kubeconfig, err := r.kubeconfigForRuntimeClusterID(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	provider, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, runtimeClusterKubeconfigError(err)
	}
	return provider, nil
}

func (r *Runner) volumeTransferProvider(ctx context.Context, clusterID string) (kubeprovider.VolumeTransferProvider, error) {
	if ctx == nil {
		panic("volume transfer job provider context is required")
	}
	if r.volumeTransferProviderFactory != nil {
		return r.volumeTransferProviderFactory(ctx, strings.TrimSpace(clusterID))
	}
	provider, err := r.projectVolumeProvider(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	jobProvider, ok := provider.(kubeprovider.VolumeTransferProvider)
	if !ok {
		return nil, errors.New("runtime cluster provider does not support volume transfer jobs")
	}
	return jobProvider, nil
}
