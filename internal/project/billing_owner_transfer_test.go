package project

import (
	"context"
	"errors"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestBillingOwnerTransferCompletesExactlyOnce(t *testing.T) {
	db := openBillingOwnerTransferTestDB(t)
	if err := db.AutoMigrate(
		&model.User{}, &model.Project{}, &model.ProjectMember{}, &model.InboxMessage{},
		&model.InboxActionRequest{}, &model.AuditLog{}, &model.PlatformEvent{},
	); err != nil {
		t.Fatalf("migrate transfer schema: %v", err)
	}
	users := []model.User{
		{ID: "usr_requester", Email: "requester@example.com", Name: "Requester", Role: "user", Language: "zh-CN"},
		{ID: "usr_recipient", Email: "recipient@example.com", Name: "Recipient", Role: "user", Language: "zh-CN"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	project := model.Project{ID: "prj_transfer", Identifier: "transfer", Name: "Transfer Project", NamespaceStrategy: "project", BillingOwnerUserID: users[0].ID}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	members := []model.ProjectMember{
		{ID: "mem_requester", ProjectID: project.ID, UserID: users[0].ID, Role: authz.ProjectRoleOwner},
		{ID: "mem_recipient", ProjectID: project.ID, UserID: users[1].ID, Role: authz.ProjectRoleDeveloper},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatalf("create members: %v", err)
	}

	service := BillingOwnerTransferService{DB: db}
	request, err := service.Request(context.Background(), users[0].ID, project.ID, users[1].ID)
	if err != nil {
		t.Fatalf("request transfer: %v", err)
	}
	if request.Status != "pending" || request.RowVersion != 1 {
		t.Fatalf("unexpected request state: %#v", request)
	}
	if _, err := service.Request(context.Background(), users[0].ID, project.ID, users[1].ID); !errors.Is(err, ErrBillingOwnerTransferConflict) {
		t.Fatalf("duplicate request error = %v", err)
	}

	completed, err := service.Decide(context.Background(), users[1].ID, request.ID, "accept", request.RowVersion)
	if err != nil {
		t.Fatalf("accept transfer: %v", err)
	}
	if completed.Status != "completed" || completed.RowVersion != 2 {
		t.Fatalf("unexpected completed state: %#v", completed)
	}
	if err := db.First(&project, "id = ?", project.ID).Error; err != nil {
		t.Fatalf("reload project: %v", err)
	}
	if project.BillingOwnerUserID != users[1].ID {
		t.Fatalf("billing owner = %q", project.BillingOwnerUserID)
	}
	if _, err := service.Decide(context.Background(), users[1].ID, request.ID, "accept", request.RowVersion); !errors.Is(err, ErrBillingOwnerTransferStale) {
		t.Fatalf("repeated decision error = %v", err)
	}

	var messageCount int64
	if err := db.Model(&model.InboxMessage{}).Count(&messageCount).Error; err != nil {
		t.Fatalf("count inbox messages: %v", err)
	}
	if messageCount != 3 {
		t.Fatalf("message count = %d, want 3", messageCount)
	}
	var eventCount int64
	if err := db.Model(&model.PlatformEvent{}).Where("type = ?", "project.billing_owner_transferred").Count(&eventCount).Error; err != nil || eventCount != 1 {
		t.Fatalf("event count = %d, err=%v", eventCount, err)
	}
}

func TestBillingOwnerTransferRejectsNonOwner(t *testing.T) {
	db := openBillingOwnerTransferTestDB(t)
	if err := db.AutoMigrate(&model.User{}, &model.Project{}, &model.ProjectMember{}, &model.InboxMessage{}, &model.InboxActionRequest{}); err != nil {
		t.Fatalf("migrate transfer schema: %v", err)
	}
	users := []model.User{
		{ID: "usr_viewer", Email: "viewer@example.com", Name: "Viewer", Role: "user", Language: "zh-CN"},
		{ID: "usr_target", Email: "target@example.com", Name: "Target", Role: "user", Language: "zh-CN"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	project := model.Project{ID: "prj_forbidden", Identifier: "forbidden", Name: "Forbidden", NamespaceStrategy: "project", BillingOwnerUserID: users[0].ID}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := db.Create(&[]model.ProjectMember{
		{ID: "mem_viewer", ProjectID: project.ID, UserID: users[0].ID, Role: authz.ProjectRoleViewer},
		{ID: "mem_target", ProjectID: project.ID, UserID: users[1].ID, Role: authz.ProjectRoleDeveloper},
	}).Error; err != nil {
		t.Fatalf("create members: %v", err)
	}
	_, err := (BillingOwnerTransferService{DB: db}).Request(context.Background(), users[0].ID, project.ID, users[1].ID)
	if !errors.Is(err, ErrBillingOwnerTransferForbidden) {
		t.Fatalf("request error = %v", err)
	}
}

func openBillingOwnerTransferTestDB(t *testing.T) *gorm.DB {
	return testdb.Open(t, testdb.Options{SchemaPrefix: "billing_owner_transfer_test"})
}
