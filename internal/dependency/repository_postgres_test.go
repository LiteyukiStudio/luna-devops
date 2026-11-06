package dependency

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestGormRepositoryRejectsBindingsToDeletingTargetsAtWriteTime(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "dependency_binding_lock_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Application{}, &model.DeploymentTarget{}, &model.ServiceBinding{})
		},
	})
	fixture := dependencyFixture()
	for _, application := range fixture.applications {
		application.DeleteStatus = "active"
		if err := db.Create(&application).Error; err != nil {
			t.Fatalf("create application: %v", err)
		}
	}
	for _, target := range fixture.targets {
		target.DeleteStatus = "active"
		if err := db.Create(&target).Error; err != nil {
			t.Fatalf("create deployment target: %v", err)
		}
	}
	repository := NewGormRepository(db)
	service := NewService(repository)
	created, err := service.CreateServiceBinding(t.Context(), "prj_main", "usr_owner", validBindingInput())
	if err != nil {
		t.Fatalf("create binding to active target: %v", err)
	}

	// 校验通过后目标才进入 deleting（并发删除），写入时的共享锁复查必须拒绝。
	if err := db.Model(&model.Application{}).Where("id = ?", "app_target").Update("delete_status", "deleting").Error; err != nil {
		t.Fatalf("mark target deleting: %v", err)
	}
	late := created
	late.ID = "sbind_late"
	late.URLEnvVar = "LATE_API_URL"
	if err := repository.CreateServiceBinding(t.Context(), &late); ErrorCode(err) != CodeTargetDeleting {
		t.Fatalf("CreateServiceBinding() error = %v, want %s", err, CodeTargetDeleting)
	}
	if err := repository.UpdateServiceBinding(t.Context(), &created); ErrorCode(err) != CodeTargetDeleting {
		t.Fatalf("UpdateServiceBinding() error = %v, want %s", err, CodeTargetDeleting)
	}
	var count int64
	if err := db.Model(&model.ServiceBinding{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("stored bindings = %d, %v", count, err)
	}
}
