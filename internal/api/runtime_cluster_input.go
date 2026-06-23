package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

func (h *Handlers) runtimeClusterFromInput(ctx *gin.Context, user model.User, input runtimeClusterInput, clusterID string) (model.RuntimeCluster, bool) {
	scope, ownerRef, projectIDs, ok := h.normalizeScopedOwnerWithProjects(ctx, user, input.Scope, input.OwnerRef, input.ProjectIDs, "只有平台管理员可以维护全局运行集群")
	if !ok {
		return model.RuntimeCluster{}, false
	}
	if input.IsDefault && scope != "global" {
		writeError(ctx, http.StatusBadRequest, "只有全局运行集群可以设为默认集群")
		return model.RuntimeCluster{}, false
	}
	kubeconfigRef := ""
	if strings.TrimSpace(input.Kubeconfig) != "" {
		kubeconfig, err := flattenKubeconfig(input.Kubeconfig)
		if err != nil {
			writeError(ctx, http.StatusBadRequest, err.Error())
			return model.RuntimeCluster{}, false
		}
		kubeconfigRef = h.secrets.Store(kubeconfig, user.ID, "runtime_cluster:"+clusterID+":kubeconfig")
	}
	return model.RuntimeCluster{
		ID:                  clusterID,
		Name:                strings.TrimSpace(input.Name),
		Type:                normalizeRuntimeClusterType(input.Type),
		Endpoint:            strings.TrimSpace(input.Endpoint),
		Scope:               scope,
		OwnerRef:            ownerRef,
		ProjectIDs:          projectIDs,
		KubeconfigRef:       kubeconfigRef,
		IsDefault:           input.IsDefault,
		MaxConcurrentBuilds: normalizeBuildConcurrency(input.MaxConcurrentBuilds, defaultClusterBuildConcurrency),
		GatewayRootDomain:   normalizeGatewayRootDomain(input.GatewayRootDomain, h.legacyGatewayRootDomain()),
		GatewayPublicScheme: normalizeGatewayPublicScheme(input.GatewayPublicScheme),
		Status:              fallback(strings.TrimSpace(input.Status), "unknown"),
		CreatedBy:           user.ID,
	}, true
}

func flattenKubeconfig(kubeconfig string) (string, error) {
	config, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		return "", fmt.Errorf("kubeconfig 无效，请检查格式")
	}
	if err := api.FlattenConfig(config); err != nil {
		return "", fmt.Errorf("kubeconfig 引用了当前 API 无法读取的证书文件，请导入已内联证书的 kubeconfig: %w", err)
	}
	output, err := clientcmd.Write(*config)
	if err != nil {
		return "", fmt.Errorf("kubeconfig 序列化失败")
	}
	return string(output), nil
}

func (h *Handlers) saveRuntimeClusterWithDefault(cluster model.RuntimeCluster) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		if cluster.IsDefault {
			if cluster.Scope != "global" {
				return errors.New("只有全局运行集群可以设为默认集群")
			}
			if err := tx.Model(&model.RuntimeCluster{}).Where("scope = ? and id <> ?", "global", cluster.ID).Update("is_default", false).Error; err != nil {
				return err
			}
		} else if cluster.Scope != "global" {
			cluster.IsDefault = false
		}
		if err := tx.Save(&cluster).Error; err != nil {
			return err
		}
		return h.replaceScopedResourceProjectBindings(tx, scopedResourceRuntimeCluster, cluster.ID, sortedProjectIDs(cluster.ProjectIDs), nil)
	})
}

func normalizeRuntimeClusterType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "docker-compose":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "kubernetes"
	}
}

type runtimeClusterInput struct {
	Name                string   `json:"name" binding:"required"`
	Type                string   `json:"type"`
	Endpoint            string   `json:"endpoint"`
	Scope               string   `json:"scope"`
	OwnerRef            string   `json:"ownerRef"`
	ProjectIDs          []string `json:"projectIds"`
	Kubeconfig          string   `json:"kubeconfig"`
	IsDefault           bool     `json:"isDefault"`
	MaxConcurrentBuilds int      `json:"maxConcurrentBuilds"`
	GatewayRootDomain   string   `json:"gatewayRootDomain"`
	GatewayPublicScheme string   `json:"gatewayPublicScheme"`
	Status              string   `json:"status"`
}

func normalizeGatewayRootDomain(value string, fallbackValue string) string {
	rootDomain := strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".")
	if rootDomain == "" {
		rootDomain = strings.Trim(strings.ToLower(strings.TrimSpace(fallbackValue)), ".")
	}
	if rootDomain == "" {
		return "apps.local"
	}
	return rootDomain
}

func normalizeGatewayPublicScheme(value string) string {
	if strings.ToLower(strings.TrimSpace(value)) == "https" {
		return "https"
	}
	return "http"
}
