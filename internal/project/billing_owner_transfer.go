package project

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/platformevent"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const BillingOwnerTransferRequestType = "project.billing_owner_transfer"

var (
	ErrBillingOwnerTransferInvalid   = errors.New("billing owner transfer input is invalid")
	ErrBillingOwnerTransferForbidden = errors.New("billing owner transfer is forbidden")
	ErrBillingOwnerTransferConflict  = errors.New("billing owner transfer conflicts with current state")
	ErrBillingOwnerTransferNotFound  = errors.New("billing owner transfer request was not found")
	ErrBillingOwnerTransferStale     = errors.New("billing owner transfer request is stale")
	ErrBillingOwnerTransferExpired   = errors.New("billing owner transfer request expired")
)

type BillingOwnerTransferService struct {
	DB *gorm.DB
}

type billingOwnerTransferPayload struct {
	PreviousBillingOwnerUserID string `json:"previousBillingOwnerUserId"`
	ProjectName                string `json:"projectName"`
	RequesterName              string `json:"requesterName"`
	RecipientName              string `json:"recipientName"`
}

func (s BillingOwnerTransferService) Request(ctx context.Context, requesterUserID, projectID, recipientUserID string) (request model.InboxActionRequest, err error) {
	ctx, end := telemetry.StartOperation(ctx, "project", "billing_owner_transfer.request")
	defer func() { end(err) }()
	requesterUserID = strings.TrimSpace(requesterUserID)
	projectID = strings.TrimSpace(projectID)
	recipientUserID = strings.TrimSpace(recipientUserID)
	if s.DB == nil || requesterUserID == "" || projectID == "" || recipientUserID == "" || requesterUserID == recipientUserID {
		return model.InboxActionRequest{}, ErrBillingOwnerTransferInvalid
	}

	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var project model.Project
		if result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, "id = ?", projectID); result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return ErrBillingOwnerTransferNotFound
			}
			return result.Error
		}
		if project.BillingOwnerUserID == recipientUserID {
			return ErrBillingOwnerTransferConflict
		}
		if project.BillingOwnerUserID != requesterUserID {
			return ErrBillingOwnerTransferForbidden
		}

		var requesterMember model.ProjectMember
		if result := tx.First(&requesterMember, "project_id = ? and user_id = ?", projectID, requesterUserID); result.Error != nil || requesterMember.Role != authz.ProjectRoleOwner {
			return ErrBillingOwnerTransferForbidden
		}
		var recipientMember model.ProjectMember
		if result := tx.First(&recipientMember, "project_id = ? and user_id = ?", projectID, recipientUserID); result.Error != nil {
			return ErrBillingOwnerTransferInvalid
		}
		var requester model.User
		if result := tx.First(&requester, "id = ? and disabled = ?", requesterUserID, false); result.Error != nil {
			return ErrBillingOwnerTransferForbidden
		}
		var recipient model.User
		if result := tx.First(&recipient, "id = ? and disabled = ?", recipientUserID, false); result.Error != nil {
			return ErrBillingOwnerTransferInvalid
		}

		var pending int64
		if result := tx.Model(&model.InboxActionRequest{}).
			Where("type = ? and project_id = ? and status in ?", BillingOwnerTransferRequestType, projectID, []string{"pending", "processing"}).
			Count(&pending); result.Error != nil {
			return result.Error
		}
		if pending > 0 {
			return ErrBillingOwnerTransferConflict
		}

		payload := billingOwnerTransferPayload{
			PreviousBillingOwnerUserID: project.BillingOwnerUserID,
			ProjectName:                project.Name,
			RequesterName:              requester.Name,
			RecipientName:              recipient.Name,
		}
		created, createErr := inbox.NewService(tx).CreateActionRequest(ctx, inbox.CreateActionRequestInput{
			Type: BillingOwnerTransferRequestType, RequesterUserID: requesterUserID, RecipientUserID: recipientUserID,
			ProjectID: projectID, ResourceType: "project", ResourceID: projectID, Payload: structMap(payload),
		})
		if createErr != nil {
			return createErr
		}
		request = created
		if _, _, publishErr := inbox.NewService(tx).Publish(ctx, inbox.PublishInput{
			RecipientUserID: recipientUserID,
			Type:            "project.billing_owner_transfer_requested",
			Category:        "action",
			Priority:        "high",
			ActorID:         requesterUserID,
			ProjectID:       projectID,
			ResourceType:    "project",
			ResourceID:      projectID,
			TitleKey:        "inbox.messages.project.billingOwnerTransferRequested.title",
			ContentKey:      "inbox.messages.project.billingOwnerTransferRequested.content",
			Params:          structMap(payload),
			ActionRequestID: request.ID,
			DeepLink:        "/inbox?filter=action",
			DedupKey:        "billing-owner-transfer-request:" + request.ID,
		}); publishErr != nil {
			return publishErr
		}
		return createBillingTransferAudit(tx, requesterUserID, "project.billing_owner_transfer.request", request.ID, true, "created")
	})
	if err != nil {
		telemetry.RecordError(ctx, "project.billing_owner_transfer.request.failed", err, slog.String("project.id", projectID))
		return model.InboxActionRequest{}, err
	}
	telemetry.Logger().InfoContext(ctx, "billing owner transfer requested",
		slog.String("event.name", "project.billing_owner_transfer.requested"),
		slog.String("project.id", projectID),
		slog.String("request.id", request.ID),
	)
	return request, nil
}

func (s BillingOwnerTransferService) Decide(ctx context.Context, recipientUserID, requestID, decision string, expectedVersion int64) (request model.InboxActionRequest, err error) {
	ctx, end := telemetry.StartOperation(ctx, "project", "billing_owner_transfer.decide")
	defer func() { end(err) }()
	recipientUserID = strings.TrimSpace(recipientUserID)
	requestID = strings.TrimSpace(requestID)
	decision = strings.TrimSpace(decision)
	if s.DB == nil || recipientUserID == "" || requestID == "" || expectedVersion <= 0 || (decision != "accept" && decision != "reject") {
		return model.InboxActionRequest{}, ErrBillingOwnerTransferInvalid
	}

	expired := false
	var invalidatedErr error
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&request, "id = ? and type = ?", requestID, BillingOwnerTransferRequestType); result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return ErrBillingOwnerTransferNotFound
			}
			return result.Error
		}
		if request.RecipientUserID != recipientUserID {
			return ErrBillingOwnerTransferForbidden
		}
		if request.Status != "pending" || request.RowVersion != expectedVersion {
			return ErrBillingOwnerTransferStale
		}
		now := time.Now()
		if request.ExpiresAt != nil && !request.ExpiresAt.After(now) {
			if updateErr := updateBillingTransferRequest(tx, &request, "expired", expectedVersion, now); updateErr != nil {
				return updateErr
			}
			_ = createBillingTransferAudit(tx, recipientUserID, "project.billing_owner_transfer.expire", request.ID, false, "expired")
			expired = true
			return nil
		}

		var payload billingOwnerTransferPayload
		if unmarshalErr := json.Unmarshal([]byte(request.PayloadJSON), &payload); unmarshalErr != nil {
			return errors.Join(ErrBillingOwnerTransferInvalid, unmarshalErr)
		}
		if decision == "reject" {
			if updateErr := updateBillingTransferRequest(tx, &request, "rejected", expectedVersion, now); updateErr != nil {
				return updateErr
			}
			if publishErr := publishBillingTransferResult(ctx, tx, request, payload, recipientUserID, "rejected"); publishErr != nil {
				return publishErr
			}
			return createBillingTransferAudit(tx, recipientUserID, "project.billing_owner_transfer.reject", request.ID, true, "rejected")
		}
		cancelInvalidatedRequest := func(reason error) error {
			if updateErr := updateBillingTransferRequest(tx, &request, inbox.ActionRequestStatusCancelled, expectedVersion, now); updateErr != nil {
				return updateErr
			}
			if publishErr := publishBillingTransferResult(ctx, tx, request, payload, recipientUserID, inbox.ActionRequestStatusCancelled); publishErr != nil {
				return publishErr
			}
			if auditErr := createBillingTransferAudit(tx, recipientUserID, "project.billing_owner_transfer.cancel", request.ID, false, "invalidated"); auditErr != nil {
				return auditErr
			}
			invalidatedErr = reason
			return nil
		}

		var project model.Project
		if result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, "id = ?", request.ProjectID); result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return cancelInvalidatedRequest(ErrBillingOwnerTransferStale)
			}
			return result.Error
		}
		if project.BillingOwnerUserID != payload.PreviousBillingOwnerUserID {
			return cancelInvalidatedRequest(ErrBillingOwnerTransferStale)
		}
		var requesterMember model.ProjectMember
		if result := tx.First(&requesterMember, "project_id = ? and user_id = ? and role = ?", request.ProjectID, request.RequesterUserID, authz.ProjectRoleOwner); result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return cancelInvalidatedRequest(ErrBillingOwnerTransferStale)
			}
			return result.Error
		}
		var recipientMember model.ProjectMember
		if result := tx.First(&recipientMember, "project_id = ? and user_id = ?", request.ProjectID, recipientUserID); result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return cancelInvalidatedRequest(ErrBillingOwnerTransferStale)
			}
			return result.Error
		}
		var recipient model.User
		if result := tx.First(&recipient, "id = ? and disabled = ?", recipientUserID, false); result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return cancelInvalidatedRequest(ErrBillingOwnerTransferStale)
			}
			return result.Error
		}

		if result := tx.Model(&model.Project{}).Where("id = ? and billing_owner_user_id = ?", project.ID, payload.PreviousBillingOwnerUserID).
			Update("billing_owner_user_id", recipientUserID); result.Error != nil {
			return result.Error
		} else if result.RowsAffected != 1 {
			return ErrBillingOwnerTransferStale
		}
		if updateErr := updateBillingTransferRequest(tx, &request, "completed", expectedVersion, now); updateErr != nil {
			return updateErr
		}
		if _, _, eventErr := (platformevent.Service{DB: tx}).Record(ctx, platformevent.RecordInput{
			Type: "project.billing_owner_transferred", Severity: "info", ProjectID: project.ID,
			ResourceType: "project", ResourceID: project.ID, ActorID: recipientUserID,
			Detail: map[string]any{"previousBillingOwnerUserId": payload.PreviousBillingOwnerUserID, "billingOwnerUserId": recipientUserID},
			Links:  map[string]string{"project": "/projects/" + project.ID}, DedupKey: "billing-owner-transfer-completed:" + request.ID,
		}); eventErr != nil {
			return eventErr
		}
		if publishErr := publishBillingTransferResult(ctx, tx, request, payload, recipientUserID, "completed"); publishErr != nil {
			return publishErr
		}
		return createBillingTransferAudit(tx, recipientUserID, "project.billing_owner_transfer.accept", request.ID, true, "completed")
	})
	if err != nil {
		telemetry.RecordError(ctx, "project.billing_owner_transfer.decision.failed", err, slog.String("request.id", requestID))
		return request, err
	}
	if expired {
		err = ErrBillingOwnerTransferExpired
		return request, err
	}
	if invalidatedErr != nil {
		err = invalidatedErr
		return request, err
	}
	telemetry.Logger().InfoContext(ctx, "billing owner transfer decided",
		slog.String("event.name", "project.billing_owner_transfer.decided"),
		slog.String("request.id", requestID),
		slog.String("decision", decision),
	)
	return request, nil
}

func updateBillingTransferRequest(tx *gorm.DB, request *model.InboxActionRequest, status string, expectedVersion int64, respondedAt time.Time) error {
	result := tx.Model(&model.InboxActionRequest{}).
		Where("id = ? and status = ? and row_version = ?", request.ID, "pending", expectedVersion).
		Updates(map[string]any{"status": status, "row_version": expectedVersion + 1, "responded_at": respondedAt, "updated_at": respondedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrBillingOwnerTransferStale
	}
	request.Status = status
	request.RowVersion = expectedVersion + 1
	request.RespondedAt = &respondedAt
	request.UpdatedAt = respondedAt
	return nil
}

func publishBillingTransferResult(ctx context.Context, tx *gorm.DB, request model.InboxActionRequest, payload billingOwnerTransferPayload, actorID, status string) error {
	messageType := "project.billing_owner_transfer_" + status
	for _, recipientID := range uniqueUserIDs(request.RequesterUserID, request.RecipientUserID) {
		if _, _, err := inbox.NewService(tx).Publish(ctx, inbox.PublishInput{
			RecipientUserID: recipientID,
			Type:            messageType,
			Category:        "billing",
			Priority:        "high",
			ActorID:         actorID,
			ProjectID:       request.ProjectID,
			ResourceType:    "project",
			ResourceID:      request.ProjectID,
			TitleKey:        "inbox.messages.project.billingOwnerTransfer." + status + ".title",
			ContentKey:      "inbox.messages.project.billingOwnerTransfer." + status + ".content",
			Params:          structMap(payload),
			DeepLink:        "/projects/" + request.ProjectID,
			DedupKey:        "billing-owner-transfer-" + status + ":" + request.ID + ":" + recipientID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func createBillingTransferAudit(tx *gorm.DB, userID, action, resource string, success bool, message string) error {
	return tx.Create(&model.AuditLog{
		ID: id.New("aud"), UserID: userID, Action: action, Resource: resource, Success: success, Message: message, CreatedAt: time.Now(),
	}).Error
}

func uniqueUserIDs(values ...string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func structMap(value any) map[string]any {
	data, _ := json.Marshal(value)
	result := map[string]any{}
	_ = json.Unmarshal(data, &result)
	return result
}
