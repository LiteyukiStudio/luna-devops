package billing

import (
	"encoding/json"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/shopspring/decimal"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	ReasonBuildUsage     = "build.usage"
	ReasonRuntimeUsage   = "runtime.usage"
	ReasonStorageUsage   = "storage.usage"
	ReasonGatewayUsage   = "gateway.usage"
	ResourceTypeBuildRun = "build_run"
	ResourceTypeRuntime  = "runtime_target"
	ResourceTypeStorage  = "storage_volume"
	ResourceTypeGateway  = "gateway_route"
	defaultCPURequest    = "500m"
	defaultMemoryRequest = "512Mi"
	defaultDataCapacity  = "1Gi"
)

type BuildUsageInput struct {
	Run         model.BuildRun
	Job         model.BuildJob
	Environment model.Environment
	FinishedAt  time.Time
}

type RuntimeUsageInput struct {
	ProjectID          string
	ApplicationID      string
	DeploymentTargetID string
	Environment        model.Environment
	PeriodStart        time.Time
	PeriodEnd          time.Time
	ActorID            string
}

type StorageUsageInput struct {
	Target      model.DeploymentTarget
	PeriodStart time.Time
	PeriodEnd   time.Time
	ActorID     string
}

type GatewayTrafficUsageInput struct {
	Route         model.GatewayRoute
	ResponseBytes int64
	RequestCount  int64
	PeriodStart   time.Time
	PeriodEnd     time.Time
	ActorID       string
}

func (s Service) SettleBuildRun(input BuildUsageInput) error {
	if input.Run.ID == "" || input.Run.ProjectID == "" || input.Job.ID == "" || input.Run.StartedAt == nil {
		return nil
	}
	periodStart := *input.Run.StartedAt
	periodEnd := input.FinishedAt
	if input.Run.FinishedAt != nil {
		periodEnd = *input.Run.FinishedAt
	}
	if !periodEnd.After(periodStart) {
		periodEnd = periodStart.Add(time.Minute)
	}
	durationSeconds := int64(periodEnd.Sub(periodStart) / time.Second)
	if durationSeconds < 1 {
		durationSeconds = 1
	}
	durationMinutes := decimal.NewFromInt(durationSeconds).Div(decimal.NewFromInt(60))
	if durationMinutes.LessThan(decimal.NewFromInt(1)) {
		durationMinutes = decimal.NewFromInt(1)
	}
	cpuCores := cpuCoresFromQuantity(input.Run.BuildCPURequest)
	memoryGiB := memoryGiBFromQuantity(input.Run.BuildMemoryRequest)
	cpuAmount, memoryAmount, amount, err := s.buildAmount(cpuCores, memoryGiB, durationMinutes)
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{
		"buildJobId":         input.Job.ID,
		"durationMinutes":    durationMinutes.String(),
		"cpuCores":           cpuCores.String(),
		"memoryGiB":          memoryGiB.String(),
		"cpuCredits":         cpuAmount.String(),
		"memoryCredits":      memoryAmount.String(),
		"buildStatus":        input.Run.Status,
		"environmentId":      input.Environment.ID,
		"buildEnvironmentId": input.Environment.ID,
		"buildCPU":           input.Run.BuildCPURequest,
		"buildMemory":        input.Run.BuildMemoryRequest,
	})
	now := time.Now()
	usage := model.BillingUsageRecord{
		ID:            id.New("busg"),
		ProjectID:     input.Run.ProjectID,
		ApplicationID: input.Run.ApplicationID,
		Meter:         MeterBuildJob,
		Quantity:      durationMinutes,
		Unit:          "minute",
		AmountCredits: amount,
		ResourceType:  ResourceTypeBuildRun,
		ResourceID:    input.Run.ID,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		Status:        "settled",
		Metadata:      string(metadata),
		SettledAt:     &now,
	}
	return s.debitUsage(usage, ReasonBuildUsage, "Build job usage", input.Run.CreatedBy)
}

func (s Service) SettleRuntimeTargetWindow(input RuntimeUsageInput) error {
	if input.ProjectID == "" || input.DeploymentTargetID == "" || !input.PeriodEnd.After(input.PeriodStart) {
		return nil
	}
	replicas := input.Environment.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	durationHours := decimal.NewFromInt(int64(input.PeriodEnd.Sub(input.PeriodStart) / time.Second)).Div(decimal.NewFromInt(3600))
	if durationHours.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	replicaHours := decimal.NewFromInt(int64(replicas)).Mul(durationHours)
	cpuQuantity := cpuCoresFromQuantity(input.Environment.CPURequest).Mul(replicaHours)
	memoryQuantity := memoryGiBFromQuantity(input.Environment.MemoryRequest).Mul(replicaHours)
	cpuRate, err := s.rate("runtime.cpu_vcpu_hour")
	if err != nil {
		return err
	}
	memoryRate, err := s.rate("runtime.memory_gib_hour")
	if err != nil {
		return err
	}
	resourceID := runtimeUsageResourceID(input.DeploymentTargetID, input.PeriodStart)
	metadata, _ := json.Marshal(map[string]string{
		"deploymentTargetId": input.DeploymentTargetID,
		"environmentId":      input.Environment.ID,
		"replicas":           decimal.NewFromInt(int64(replicas)).String(),
		"durationHours":      durationHours.String(),
		"cpuCores":           cpuCoresFromQuantity(input.Environment.CPURequest).String(),
		"memoryGiB":          memoryGiBFromQuantity(input.Environment.MemoryRequest).String(),
	})
	now := time.Now()
	records := []model.BillingUsageRecord{
		{
			ID:            id.New("busg"),
			ProjectID:     input.ProjectID,
			ApplicationID: input.ApplicationID,
			Meter:         "runtime.cpu_vcpu_hour",
			Quantity:      cpuQuantity,
			Unit:          "vcpu_hour",
			AmountCredits: cpuQuantity.Mul(cpuRate),
			ResourceType:  ResourceTypeRuntime,
			ResourceID:    resourceID,
			PeriodStart:   input.PeriodStart,
			PeriodEnd:     input.PeriodEnd,
			Status:        "settled",
			Metadata:      string(metadata),
			SettledAt:     &now,
		},
		{
			ID:            id.New("busg"),
			ProjectID:     input.ProjectID,
			ApplicationID: input.ApplicationID,
			Meter:         "runtime.memory_gib_hour",
			Quantity:      memoryQuantity,
			Unit:          "gib_hour",
			AmountCredits: memoryQuantity.Mul(memoryRate),
			ResourceType:  ResourceTypeRuntime,
			ResourceID:    resourceID,
			PeriodStart:   input.PeriodStart,
			PeriodEnd:     input.PeriodEnd,
			Status:        "settled",
			Metadata:      string(metadata),
			SettledAt:     &now,
		},
	}
	return s.debitUsages(records, ReasonRuntimeUsage, "Runtime resource usage", input.ActorID)
}

func (s Service) SettleStorageTargetWindow(input StorageUsageInput) error {
	if input.Target.ProjectID == "" || input.Target.ID == "" || !input.Target.DataRetentionEnabled || !input.PeriodEnd.After(input.PeriodStart) {
		return nil
	}
	capacityGiB := deploymentTargetStorageGiB(input.Target)
	if capacityGiB.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	durationDays := decimal.NewFromInt(int64(input.PeriodEnd.Sub(input.PeriodStart) / time.Second)).Div(decimal.NewFromInt(86400))
	if durationDays.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	quantity := capacityGiB.Mul(durationDays)
	rate, err := s.rate("storage.gib_day")
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{
		"deploymentTargetId": input.Target.ID,
		"dataRetention":      "true",
		"capacityGiB":        capacityGiB.String(),
		"durationDays":       durationDays.String(),
	})
	now := time.Now()
	usage := model.BillingUsageRecord{
		ID:            id.New("busg"),
		ProjectID:     input.Target.ProjectID,
		ApplicationID: input.Target.ApplicationID,
		Meter:         "storage.gib_day",
		Quantity:      quantity,
		Unit:          "gib_day",
		AmountCredits: quantity.Mul(rate),
		ResourceType:  ResourceTypeStorage,
		ResourceID:    storageUsageResourceID(input.Target.ID, input.PeriodStart),
		PeriodStart:   input.PeriodStart,
		PeriodEnd:     input.PeriodEnd,
		Status:        "settled",
		Metadata:      string(metadata),
		SettledAt:     &now,
	}
	return s.debitUsage(usage, ReasonStorageUsage, "Persistent storage usage", input.ActorID)
}

func (s Service) SettleGatewayTrafficWindow(input GatewayTrafficUsageInput) error {
	if input.Route.ID == "" || input.Route.ProjectID == "" || input.ResponseBytes <= 0 || !input.PeriodEnd.After(input.PeriodStart) {
		return nil
	}
	responseGiB := decimal.NewFromInt(input.ResponseBytes).Div(decimal.NewFromInt(1024 * 1024 * 1024))
	if responseGiB.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	rate, err := s.rate("gateway.egress_gib")
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{
		"gatewayRouteId": input.Route.ID,
		"host":           input.Route.Host,
		"path":           input.Route.Path,
		"responseBytes":  decimal.NewFromInt(input.ResponseBytes).String(),
		"responseGiB":    responseGiB.String(),
		"requestCount":   decimal.NewFromInt(input.RequestCount).String(),
	})
	now := time.Now()
	usage := model.BillingUsageRecord{
		ID:            id.New("busg"),
		ProjectID:     input.Route.ProjectID,
		ApplicationID: input.Route.ApplicationID,
		Meter:         "gateway.egress_gib",
		Quantity:      responseGiB,
		Unit:          "gib",
		AmountCredits: responseGiB.Mul(rate),
		ResourceType:  ResourceTypeGateway,
		ResourceID:    gatewayTrafficUsageResourceID(input.Route.ID, input.PeriodStart),
		PeriodStart:   input.PeriodStart,
		PeriodEnd:     input.PeriodEnd,
		Status:        "settled",
		Metadata:      string(metadata),
		SettledAt:     &now,
	}
	return s.debitUsage(usage, ReasonGatewayUsage, "Gateway response traffic usage", input.ActorID)
}

func runtimeUsageResourceID(deploymentTargetID string, periodStart time.Time) string {
	return deploymentTargetID + ":" + periodStart.UTC().Format("2006010215")
}

func storageUsageResourceID(deploymentTargetID string, periodStart time.Time) string {
	return deploymentTargetID + ":" + periodStart.UTC().Format("2006010215")
}

func gatewayTrafficUsageResourceID(routeID string, periodStart time.Time) string {
	return routeID + ":" + periodStart.UTC().Format("200601021504")
}

func deploymentTargetStorageGiB(target model.DeploymentTarget) decimal.Decimal {
	total := decimal.Zero
	for _, volume := range deploymentTargetBillingVolumes(target) {
		total = total.Add(storageGiBFromQuantity(volume.Capacity))
	}
	if total.GreaterThan(decimal.Zero) {
		return total
	}
	return storageGiBFromQuantity(target.DataCapacity)
}

type deploymentTargetBillingVolume struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	Capacity  string `json:"capacity"`
}

func deploymentTargetBillingVolumes(target model.DeploymentTarget) []deploymentTargetBillingVolume {
	var volumes []deploymentTargetBillingVolume
	if err := json.Unmarshal([]byte(target.DataVolumes), &volumes); err != nil {
		return nil
	}
	output := make([]deploymentTargetBillingVolume, 0, len(volumes))
	for _, volume := range volumes {
		if volume.Capacity != "" {
			output = append(output, volume)
		}
	}
	return output
}

func cpuCoresFromQuantity(value string) decimal.Decimal {
	if value == "" {
		value = defaultCPURequest
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		quantity = resource.MustParse(defaultCPURequest)
	}
	return decimal.NewFromInt(quantity.MilliValue()).Div(decimal.NewFromInt(1000))
}

func memoryGiBFromQuantity(value string) decimal.Decimal {
	if value == "" {
		value = defaultMemoryRequest
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		quantity = resource.MustParse(defaultMemoryRequest)
	}
	return decimal.NewFromInt(quantity.Value()).Div(decimal.NewFromInt(1024 * 1024 * 1024))
}

func storageGiBFromQuantity(value string) decimal.Decimal {
	if value == "" {
		value = defaultDataCapacity
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil {
		quantity = resource.MustParse(defaultDataCapacity)
	}
	return decimal.NewFromInt(quantity.Value()).Div(decimal.NewFromInt(1024 * 1024 * 1024))
}
