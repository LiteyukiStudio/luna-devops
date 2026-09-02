package projectapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
)

type billingOwnerTransferRequestInput struct {
	RecipientUserID string `json:"recipientUserId" binding:"required"`
}

func (h *Handler) CreateBillingOwnerTransferRequest(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionProjectOwnerOnly)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	var input billingOwnerTransferRequestInput
	if !bindJSON(ctx, &input) {
		return
	}
	request, err := (projectservice.BillingOwnerTransferService{DB: h.dbFor(ctx)}).Request(
		ctx.Request.Context(), user.ID, project.ID, input.RecipientUserID,
	)
	if err != nil {
		writeInboxError(ctx, err)
		return
	}
	defaultInboxBroker.Notify(request.RecipientUserID, "")
	ctx.JSON(http.StatusCreated, request)
}

func (h *Handler) decideInboxAction(ctx context.Context, user model.User, requestID, decision string, expectedVersion int64) error {
	request, err := inbox.NewService(h.dbWithContext(ctx)).GetActionRequest(ctx, user.ID, requestID)
	if err != nil {
		return err
	}
	switch strings.TrimSpace(request.Type) {
	case projectservice.BillingOwnerTransferRequestType:
		_, err = (projectservice.BillingOwnerTransferService{DB: h.dbWithContext(ctx)}).Decide(ctx, user.ID, request.ID, decision, expectedVersion)
		return err
	default:
		return errors.New("unsupported inbox action request type")
	}
}
