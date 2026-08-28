package api

import (
	"context"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPlatformEventVisibilityDoesNotExpandForAdministratorByDefault(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	handlers := &Handlers{db: db, projects: repository.NewProjectRepository(db)}
	admin := model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin}

	var events []model.PlatformEvent
	related := handlers.platformEventsVisibleTo(admin, projectservice.ListVisibilityRelated, context.Background()).Find(&events).Statement
	relatedSQL := db.Dialector.Explain(related.SQL.String(), related.Vars...)
	if !strings.Contains(relatedSQL, "actor_id = 'usr_admin'") {
		t.Fatalf("related event query is not caller-scoped: %s", relatedSQL)
	}

	all := handlers.platformEventsVisibleTo(admin, projectservice.ListVisibilityAll, context.Background()).Find(&events).Statement
	allSQL := db.Dialector.Explain(all.SQL.String(), all.Vars...)
	if strings.Contains(allSQL, "actor_id") || strings.Contains(allSQL, "project_id in") {
		t.Fatalf("all event query remained related-scoped: %s", allSQL)
	}
}
