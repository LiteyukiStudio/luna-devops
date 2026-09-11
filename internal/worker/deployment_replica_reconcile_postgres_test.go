package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestReplicaReconcileWaitsForConcurrentTargetUpdate(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix:       "deployment_replica_reconcile",
		MaxOpenConnections: 2,
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.DeploymentTarget{})
		},
	})
	target := model.DeploymentTarget{
		ID:             "dplt_reconcile",
		ProjectID:      "prj_reconcile",
		ApplicationID:  "app_reconcile",
		Name:           "Reconcile",
		Stage:          "dev",
		KubernetesName: "reconcile-dev",
		Replicas:       1,
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create deployment target: %v", err)
	}

	lockHeld := make(chan struct{})
	allowCommit := make(chan struct{})
	updateDone := make(chan error, 1)
	go func() {
		updateDone <- db.WithContext(context.Background()).Transaction(func(tx *gorm.DB) error {
			var current model.DeploymentTarget
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id = ?", target.ID).Error; err != nil {
				return err
			}
			if err := tx.Model(&current).UpdateColumn("replicas", 0).Error; err != nil {
				return err
			}
			close(lockHeld)
			<-allowCommit
			return nil
		})
	}()
	select {
	case <-lockHeld:
	case err := <-updateDone:
		t.Fatalf("lock target for update: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for target update lock")
	}
	defer func() {
		select {
		case <-allowCommit:
		default:
			close(allowCommit)
		}
	}()

	reconcileQueryStarted := make(chan struct{})
	var startedOnce sync.Once
	callbackName := "test:replica_reconcile_query_started"
	if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "DeploymentTarget" {
			if _, ok := tx.Statement.Clauses["FOR"]; ok {
				startedOnce.Do(func() { close(reconcileQueryStarted) })
			}
		}
	}); err != nil {
		t.Fatalf("register query callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })

	manager := &recordingNamespaceManager{}
	reconcileDone := make(chan error, 1)
	go func() {
		reconcileDone <- (&Runner{db: db}).reconcileApplicationResourceReplicas(
			context.Background(),
			model.Release{ProjectID: target.ProjectID, ApplicationID: target.ApplicationID, DeploymentTargetID: target.ID},
			manager,
			kubeprovider.ApplicationResourcesSpec{Replicas: 1},
		)
	}()
	select {
	case <-reconcileQueryStarted:
	case err := <-reconcileDone:
		t.Fatalf("replica reconcile failed before querying the locked target: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for replica reconcile query")
	}
	select {
	case err := <-reconcileDone:
		t.Fatalf("replica reconcile completed before the target update committed: %v", err)
	default:
	}

	close(allowCommit)
	if err := <-updateDone; err != nil {
		t.Fatalf("commit target update: %v", err)
	}
	if err := <-reconcileDone; err != nil {
		t.Fatalf("reconcile replicas: %v", err)
	}
	if len(manager.replicaScales) != 1 || manager.replicaScales[0] != 0 {
		t.Fatalf("replica scales = %#v, want [0]", manager.replicaScales)
	}
}
