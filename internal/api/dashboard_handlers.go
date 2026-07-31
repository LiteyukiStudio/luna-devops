package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	dashboardservice "github.com/LiteyukiStudio/devops/internal/dashboard"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) GetDashboard(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	platformAdmin := authz.IsPlatformAdmin(user.Role)
	if platformAdmin {
		if _, err := h.ensurePlatformSystemProject(user, ctx.Request.Context()); err != nil {
			writeErrorCode(ctx, http.StatusInternalServerError, "dashboard.load_failed", err.Error())
			return
		}
	}
	projectIDs := []string{}
	if !platformAdmin {
		projectIDs = h.projectIDsForUser(ctx.Request.Context(), user.ID)
	}
	scope := dashboardservice.Scope{
		UserID:            user.ID,
		PlatformAdmin:     platformAdmin,
		VisibleProjectIDs: projectIDs,
	}
	service := dashboardservice.NewService(h.dbFor(ctx))
	overview, err := service.Overview(ctx.Request.Context(), scope)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "dashboard.load_failed", err.Error())
		return
	}
	clusters, registries, err := service.ReadinessResources(ctx.Request.Context(), scope)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "dashboard.load_failed", err.Error())
		return
	}
	overview.Readiness = h.observeDashboardReadiness(ctx.Request.Context(), user, clusters, registries)
	overview.Summary.HealthyClusters = overview.Readiness.Clusters.Available
	overview.Summary.TotalClusters = overview.Readiness.Clusters.Total
	ctx.JSON(http.StatusOK, overview)
}

func (h *Handlers) observeDashboardReadiness(ctx context.Context, user model.User, clusters []model.RuntimeCluster, registries []model.ArtifactRegistry) dashboardservice.Readiness {
	observedAt := time.Now().UTC()
	h.observeRuntimeClusters(ctx, clusters)
	clusterAvailable := 0
	for _, cluster := range clusters {
		if cluster.Status == observation.StatusReady {
			clusterAvailable++
		}
	}

	registryAvailable := h.observeDashboardRegistries(ctx, user, registries)
	return dashboardservice.Readiness{
		Clusters:   dashboardReadinessItem(clusterAvailable, len(clusters), observedAt, "dashboard.clusters"),
		Registries: dashboardReadinessItem(registryAvailable, len(registries), observedAt, "dashboard.registries"),
	}
}

func (h *Handlers) observeDashboardRegistries(ctx context.Context, user model.User, registries []model.ArtifactRegistry) int {
	const concurrency = 6
	guard := make(chan struct{}, concurrency)
	var wait sync.WaitGroup
	var lock sync.Mutex
	available := 0
	for _, registry := range registries {
		registry := registry
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case guard <- struct{}{}:
				defer func() { <-guard }()
			case <-ctx.Done():
				return
			}
			probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			if h.pingRegistry(probeCtx, user, registry).Success {
				lock.Lock()
				available++
				lock.Unlock()
			}
		}()
	}
	wait.Wait()
	return available
}

func dashboardReadinessItem(available int, total int, observedAt time.Time, codePrefix string) dashboardservice.ReadinessItem {
	item := dashboardservice.ReadinessItem{
		Available:  available,
		Total:      total,
		ObservedAt: observedAt,
	}
	switch {
	case total == 0:
		item.Status = observation.StatusNotConfigured
		item.ObservationCode = codePrefix + "_not_configured"
	case available == total:
		item.Status = observation.StatusReady
	case available > 0:
		item.Status = observation.StatusDegraded
		item.ObservationCode = codePrefix + "_partially_unavailable"
	default:
		item.Status = observation.StatusUnavailable
		item.ObservationCode = codePrefix + "_upstream_unavailable"
	}
	return item
}
