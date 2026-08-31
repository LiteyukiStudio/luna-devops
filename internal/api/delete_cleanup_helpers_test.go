package api

import (
	"errors"
	"sync"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestMarkResourceDeletingHasSingleConcurrentWinner(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix:       "resource_delete_start_test",
		MaxOpenConnections: 4,
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{})
		},
	})
	project := model.Project{ID: "prj_delete_start", Identifier: "delete-start", Name: "Delete Start", DeleteStatus: "active"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- markResourceDeleting(db, &model.Project{}, project.ID)
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	succeeded := 0
	alreadyStarted := 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, errResourceDeleteAlreadyStarted):
			alreadyStarted++
		default:
			t.Fatalf("markResourceDeleting() unexpected error = %v", err)
		}
	}
	if succeeded != 1 || alreadyStarted != 1 {
		t.Fatalf("delete start outcomes = success:%d already-started:%d", succeeded, alreadyStarted)
	}
	var stored model.Project
	if err := db.First(&stored, "id = ?", project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.DeleteStatus != "deleting" || stored.DeleteStartedAt == nil {
		t.Fatalf("stored delete state = %#v", stored)
	}
}
