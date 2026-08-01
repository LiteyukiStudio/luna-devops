package api

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/inbox"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

func publishProjectMemberInbox(ctx context.Context, tx *gorm.DB, input projectMemberInboxInput) error {
	params := map[string]any{
		"actorName":   input.Actor.Name,
		"projectName": input.Project.Name,
		"role":        input.Role,
	}
	if input.PreviousRole != "" {
		params["previousRole"] = input.PreviousRole
	}
	deepLink := "/projects/" + input.Project.ID
	if input.Type == "project.member_removed" {
		deepLink = "/projects"
	}
	_, _, err := inbox.NewService(tx).Publish(ctx, inbox.PublishInput{
		RecipientUserID: input.RecipientUserID,
		Type:            input.Type,
		Category:        "project",
		Priority:        input.Priority,
		ActorID:         input.Actor.ID,
		ProjectID:       input.Project.ID,
		ResourceType:    "project_member",
		ResourceID:      input.MemberID,
		TitleKey:        "inbox.messages." + input.Type + ".title",
		ContentKey:      "inbox.messages." + input.Type + ".content",
		Params:          params,
		DeepLink:        deepLink,
		DedupKey:        input.DedupKey,
	})
	return err
}

type projectMemberInboxInput struct {
	Type            string
	Priority        string
	Project         model.Project
	Actor           model.User
	RecipientUserID string
	MemberID        string
	Role            string
	PreviousRole    string
	DedupKey        string
}
