package billing

import (
	"context"
	"encoding/json"
	"errors"
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
	ReasonTransferUsage  = "storage.transfer_usage"
	ReasonGatewayUsage   = "gateway.usage"
	ResourceTypeBuildRun = "build_run"
	ResourceTypeRuntime  = "runtime_target"
	ResourceTypeStorage  = "storage_volume"
	ResourceTypeTransfer = "volume_transfer"
	ResourceTypeGateway  = "gateway_route"
	defaultCPURequest    = "500m"
	defaultMemoryRequest = "512Mi"
)

type BuildUsageInput struct {
	Run         model.BuildRun
	Job         model.BuildJob
	Environment model.Environment
	FinishedAt  time.Time
}

type RuntimeUsageInput struct {
	Context            context.Context
	ProjectID          string
	ApplicationID      string
	DeploymentTargetID string
	EnvironmentID      string
	DesiredReplicas    int32
	CPURequest         string
	MemoryRequest      string
	PeriodStart        time.Time
	PeriodEnd          time.Time
	ActorID            string
}

type ProjectVolumeStorageUsageInput struct {
	Volume                model.ProjectVolume
	ObservedCapacityBytes int64
	PeriodStart           time.Time
	PeriodEnd             time.Time
	ActorID               string
}

type VolumeTransferUsageInput struct {
	Transfer  model.VolumeTransfer
	SettledAt time.Time
	ActorID   string
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
	service := s
	if input.Context != nil {
		service.DB = s.DB.WithContext(input.Context)
	}
	if input.ProjectID == "" || input.DeploymentTargetID == "" || !input.PeriodEnd.After(input.PeriodStart) {
		return nil
	}
	if input.DesiredReplicas < 0 {
		return nil
	}
	durationHours := runtimeDurationHours(input)
	if durationHours.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	replicaHours := runtimeReplicaHours(input)
	cpuQuantity := cpuCoresFromQuantity(input.CPURequest).Mul(replicaHours)
	memoryQuantity := memoryGiBFromQuantity(input.MemoryRequest).Mul(replicaHours)
	cpuRate, err := service.rate("runtime.cpu_vcpu_hour")
	if err != nil {
		return err
	}
	memoryRate, err := service.rate("runtime.memory_gib_hour")
	if err != nil {
		return err
	}
	resourceID := runtimeUsageResourceID(input.DeploymentTargetID, input.PeriodStart)
	metadata, _ := json.Marshal(map[string]string{
		"deploymentTargetId": input.DeploymentTargetID,
		"environmentId":      input.EnvironmentID,
		"replicas":           decimal.NewFromInt(int64(input.DesiredReplicas)).String(),
		"durationHours":      durationHours.String(),
		"cpuCores":           cpuCoresFromQuantity(input.CPURequest).String(),
		"memoryGiB":          memoryGiBFromQuantity(input.MemoryRequest).String(),
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
	return service.debitUsages(records, ReasonRuntimeUsage, "Runtime resource usage", input.ActorID)
}

func runtimeReplicaHours(input RuntimeUsageInput) decimal.Decimal {
	durationHours := runtimeDurationHours(input)
	if durationHours.LessThanOrEqual(decimal.Zero) || input.DesiredReplicas < 0 {
		return decimal.Zero
	}
	return decimal.NewFromInt(int64(input.DesiredReplicas)).Mul(durationHours)
}

func runtimeDurationHours(input RuntimeUsageInput) decimal.Decimal {
	return decimal.NewFromInt(int64(input.PeriodEnd.Sub(input.PeriodStart) / time.Second)).Div(decimal.NewFromInt(3600))
}

// SettleProjectVolumeStorageWindow bills the Kubernetes-observed capacity of
// a managed project volume. Referenced claims are never billed as platform
// storage, and application/target bindings do not affect ownership or charge.
func (s Service) SettleProjectVolumeStorageWindow(ctx context.Context, input ProjectVolumeStorageUsageInput) error {
	if ctx == nil {
		return errors.New("project volume storage usage context is required")
	}
	if input.Volume.ID == "" || input.Volume.ProjectID == "" ||
		input.Volume.OwnershipMode != model.ProjectVolumeOwnershipManaged || input.ObservedCapacityBytes <= 0 ||
		!input.PeriodEnd.After(input.PeriodStart) {
		return nil
	}
	durationDays := decimal.NewFromInt(int64(input.PeriodEnd.Sub(input.PeriodStart) / time.Second)).Div(decimal.NewFromInt(86400))
	if durationDays.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	capacityGiB := decimal.NewFromInt(input.ObservedCapacityBytes).Div(decimal.NewFromInt(1024 * 1024 * 1024))
	quantity := capacityGiB.Mul(durationDays)
	contextService := Service{DB: s.DB.WithContext(ctx)}
	rate, err := contextService.rate("storage.gib_day")
	if err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]string{
		"projectVolumeId": input.Volume.ID,
		"capacityGiB":     capacityGiB.String(),
		"durationDays":    durationDays.String(),
	})
	now := time.Now()
	usage := model.BillingUsageRecord{
		ID:            id.New("busg"),
		ProjectID:     input.Volume.ProjectID,
		Meter:         "storage.gib_day",
		Quantity:      quantity,
		Unit:          "gib_day",
		AmountCredits: quantity.Mul(rate),
		ResourceType:  ResourceTypeStorage,
		ResourceID:    projectVolumeStorageUsageResourceID(input.Volume.ID, input.PeriodStart),
		PeriodStart:   input.PeriodStart,
		PeriodEnd:     input.PeriodEnd,
		Status:        "settled",
		Metadata:      string(metadata),
		SettledAt:     &now,
	}
	return contextService.debitUsage(usage, ReasonStorageUsage, "Persistent storage usage", input.ActorID)
}

// SettleVolumeTransferUsage records the bytes moved by one terminal transfer.
// The transfer ID is the billing resource ID, so retries with a new transfer
// remain distinct while repeated Worker reconciliation is idempotent.
func (s Service) SettleVolumeTransferUsage(ctx context.Context, input VolumeTransferUsageInput) error {
	if ctx == nil {
		return errors.New("volume transfer usage context is required")
	}
	transfer := input.Transfer
	if transfer.ID == "" || transfer.ProjectID == "" || transfer.TransferredBytes <= 0 || !volumeTransferTerminal(transfer.State) {
		return nil
	}
	quantity := decimal.NewFromInt(transfer.TransferredBytes).Div(decimal.NewFromInt(1024 * 1024 * 1024))
	if quantity.LessThanOrEqual(decimal.Zero) {
		return nil
	}
	contextService := Service{DB: s.DB.WithContext(ctx)}
	rate, err := contextService.rate(MeterStorageTransferGiB)
	if err != nil {
		return err
	}
	periodStart := transfer.CreatedAt.UTC()
	periodEnd := input.SettledAt.UTC()
	if transfer.FinishedAt != nil {
		periodEnd = transfer.FinishedAt.UTC()
	}
	if periodEnd.IsZero() {
		periodEnd = time.Now().UTC()
	}
	if periodStart.IsZero() {
		periodStart = periodEnd
	}
	if !periodEnd.After(periodStart) {
		periodEnd = periodStart.Add(time.Microsecond)
	}
	metadata, _ := json.Marshal(map[string]string{
		"volumeTransferId": transfer.ID,
		"projectVolumeId":  transfer.ProjectVolumeID,
		"direction":        transfer.Direction,
		"format":           transfer.Format,
		"state":            transfer.State,
		"bytes":            decimal.NewFromInt(transfer.TransferredBytes).String(),
	})
	now := time.Now().UTC()
	usage := model.BillingUsageRecord{
		ID:            id.New("busg"),
		ProjectID:     transfer.ProjectID,
		Meter:         MeterStorageTransferGiB,
		Quantity:      quantity,
		Unit:          "gib",
		AmountCredits: quantity.Mul(rate),
		ResourceType:  ResourceTypeTransfer,
		ResourceID:    transfer.ID,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		Status:        "settled",
		Metadata:      string(metadata),
		SettledAt:     &now,
	}
	actorID := input.ActorID
	if actorID == "" {
		actorID = transfer.ActorID
	}
	return contextService.debitUsage(usage, ReasonTransferUsage, "Volume transfer usage", actorID)
}

func volumeTransferTerminal(state string) bool {
	switch state {
	case model.VolumeTransferStateSucceeded, model.VolumeTransferStateFailed,
		model.VolumeTransferStateCancelled, model.VolumeTransferStateExpired:
		return true
	default:
		return false
	}
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

func projectVolumeStorageUsageResourceID(projectVolumeID string, periodStart time.Time) string {
	return projectVolumeID + ":" + periodStart.UTC().Format("2006010215")
}

func gatewayTrafficUsageResourceID(routeID string, periodStart time.Time) string {
	return routeID + ":" + periodStart.UTC().Format("200601021504")
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
