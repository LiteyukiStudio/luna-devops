package aimodel

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestConcurrentDisablePreservesEnabledModel(t *testing.T) {
	db := openAIModelTestDB(t)
	service := NewService(db)
	for iteration := 0; iteration < 12; iteration++ {
		if err := db.Exec("TRUNCATE TABLE ai_models").Error; err != nil {
			t.Fatalf("truncate models: %v", err)
		}
		first := createModelFixture(t, service, fmt.Sprintf("first-%d", iteration), true)
		second := createModelFixture(t, service, fmt.Sprintf("second-%d", iteration), true)

		start := make(chan struct{})
		errorsByModel := make(chan error, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		for _, item := range []model.AIModel{first, second} {
			item := item
			go func() {
				ready.Done()
				<-start
				disabled := false
				_, err := NewService(db).Update(t.Context(), item.ID, writeInputFor(item.Name, &disabled))
				errorsByModel <- err
			}()
		}
		ready.Wait()
		close(start)
		firstErr, secondErr := <-errorsByModel, <-errorsByModel
		if !(errors.Is(firstErr, ErrLastEnabled) || errors.Is(secondErr, ErrLastEnabled)) {
			t.Fatalf("iteration %d errors = (%v, %v), want one last-enabled rejection", iteration, firstErr, secondErr)
		}
		assertEnabledModelCount(t, db, 1)
	}
}

func TestConcurrentCreateAndDisablePreservesEnabledModel(t *testing.T) {
	db := openAIModelTestDB(t)
	service := NewService(db)
	for iteration := 0; iteration < 12; iteration++ {
		if err := db.Exec("TRUNCATE TABLE ai_models").Error; err != nil {
			t.Fatalf("truncate models: %v", err)
		}
		existing := createModelFixture(t, service, fmt.Sprintf("existing-%d", iteration), true)
		disabled := false
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := NewService(db).Update(t.Context(), existing.ID, writeInputFor(existing.Name, &disabled))
			results <- err
		}()
		go func() {
			<-start
			_, err := NewService(db).Create(t.Context(), writeInputFor(fmt.Sprintf("created-%d", iteration), &disabled))
			results <- err
		}()
		close(start)
		if err := <-results; err != nil && !errors.Is(err, ErrLastEnabled) {
			t.Fatalf("first concurrent mutation: %v", err)
		}
		if err := <-results; err != nil && !errors.Is(err, ErrLastEnabled) {
			t.Fatalf("second concurrent mutation: %v", err)
		}
		assertAtLeastOneEnabledModel(t, db)
	}
}

func TestConcurrentDisabledCreatesEnableAtLeastOneModel(t *testing.T) {
	db := openAIModelTestDB(t)
	for iteration := 0; iteration < 12; iteration++ {
		if err := db.Exec("TRUNCATE TABLE ai_models").Error; err != nil {
			t.Fatalf("truncate models: %v", err)
		}
		disabled := false
		start := make(chan struct{})
		results := make(chan error, 2)
		for candidate := 0; candidate < 2; candidate++ {
			candidate := candidate
			go func() {
				<-start
				_, err := NewService(db).Create(t.Context(), writeInputFor(
					fmt.Sprintf("created-%d-%d", iteration, candidate), &disabled,
				))
				results <- err
			}()
		}
		close(start)
		if err := <-results; err != nil {
			t.Fatalf("first concurrent create: %v", err)
		}
		if err := <-results; err != nil {
			t.Fatalf("second concurrent create: %v", err)
		}
		assertEnabledModelCount(t, db, 1)
	}
}

func TestFirstCreatedModelIsEnabled(t *testing.T) {
	db := openAIModelTestDB(t)
	service := NewService(db)
	disabled := false
	created, err := service.Create(t.Context(), writeInputFor("first-disabled-request", &disabled))
	if err != nil {
		t.Fatalf("create first model: %v", err)
	}
	if !created.Enabled {
		t.Fatal("first model remained disabled")
	}
	assertEnabledModelCount(t, db, 1)
}

func TestUpdateRepairsCatalogWithoutEnabledModel(t *testing.T) {
	db := openAIModelTestDB(t)
	disabled := model.AIModel{
		ID: "aimod_disabled", Name: "disabled-existing-model", MaxContextTokens: 524288, MaxOutputTokens: 65536, Enabled: false,
	}
	if err := db.Select("*").Create(&disabled).Error; err != nil {
		t.Fatalf("seed disabled model: %v", err)
	}
	updated, err := NewService(db).Update(t.Context(), disabled.ID, WriteInput{Name: disabled.Name, MaxContextTokens: 524288, MaxOutputTokens: 65536})
	if err != nil {
		t.Fatalf("update disabled model: %v", err)
	}
	if !updated.Enabled {
		t.Fatal("update did not repair empty enabled-model set")
	}
	assertEnabledModelCount(t, db, 1)
}

func TestDeleteRemovesModel(t *testing.T) {
	db := openAIModelTestDB(t)
	service := NewService(db)
	disabled := false
	target := createModelFixture(t, service, "delete-target", disabled)
	createModelFixture(t, service, "delete-keeper", true)

	deleted, err := service.Delete(t.Context(), target.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if deleted.ID != target.ID {
		t.Fatalf("Delete() id = %q, want %q", deleted.ID, target.ID)
	}
	var count int64
	if err := db.Model(&model.AIModel{}).Where("id = ?", target.ID).Count(&count).Error; err != nil {
		t.Fatalf("count deleted model: %v", err)
	}
	if count != 0 {
		t.Fatalf("deleted model still present, count = %d", count)
	}
	if _, err := service.Delete(t.Context(), target.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete() error = %v, want ErrNotFound", err)
	}
}

func TestConcurrentDeletePreservesEnabledModel(t *testing.T) {
	db := openAIModelTestDB(t)
	service := NewService(db)
	for iteration := 0; iteration < 12; iteration++ {
		if err := db.Exec("TRUNCATE TABLE ai_models").Error; err != nil {
			t.Fatalf("truncate models: %v", err)
		}
		first := createModelFixture(t, service, fmt.Sprintf("first-%d", iteration), true)
		second := createModelFixture(t, service, fmt.Sprintf("second-%d", iteration), true)

		start := make(chan struct{})
		errorsByModel := make(chan error, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		for _, item := range []model.AIModel{first, second} {
			item := item
			go func() {
				ready.Done()
				<-start
				_, err := NewService(db).Delete(t.Context(), item.ID)
				errorsByModel <- err
			}()
		}
		ready.Wait()
		close(start)
		firstErr, secondErr := <-errorsByModel, <-errorsByModel
		if !(errors.Is(firstErr, ErrLastEnabled) || errors.Is(secondErr, ErrLastEnabled)) {
			t.Fatalf("iteration %d errors = (%v, %v), want one last-enabled rejection", iteration, firstErr, secondErr)
		}
		assertEnabledModelCount(t, db, 1)
	}
}

func createModelFixture(t *testing.T, service *Service, name string, enabled bool) model.AIModel {
	t.Helper()
	created, err := service.Create(t.Context(), writeInputFor(name, &enabled))
	if err != nil {
		t.Fatalf("create model %q: %v", name, err)
	}
	return created
}

func writeInputFor(name string, enabled *bool) WriteInput {
	return WriteInput{
		Name: name, InputCreditsPerMillion: "1", OutputCreditsPerMillion: "2",
		MaxContextTokens: 524288, MaxOutputTokens: 65536,
		CachedInputCreditsPerMillion: "1", CachedOutputCreditsPerMillion: "2", Enabled: enabled,
	}
}

func assertEnabledModelCount(t *testing.T, db *gorm.DB, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&model.AIModel{}).Where("enabled = ?", true).Count(&count).Error; err != nil {
		t.Fatalf("count enabled models: %v", err)
	}
	if count != want {
		t.Fatalf("enabled model count = %d, want %d", count, want)
	}
}

func assertAtLeastOneEnabledModel(t *testing.T, db *gorm.DB) {
	t.Helper()
	var count int64
	if err := db.Model(&model.AIModel{}).Where("enabled = ?", true).Count(&count).Error; err != nil {
		t.Fatalf("count enabled models: %v", err)
	}
	if count < 1 {
		t.Fatal("AI model catalog has no enabled model")
	}
}

func openAIModelTestDB(t *testing.T) *gorm.DB {
	return testdb.Open(t, testdb.Options{
		SchemaPrefix:       "ai_model_test",
		MaxOpenConnections: 8,
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.AIModel{})
		},
	})
}
