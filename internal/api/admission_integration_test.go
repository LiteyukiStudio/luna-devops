package api

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestEnsureAdmissionPolicyInitializesDefault(t *testing.T) {
	db := authIntegrationDB(t)
	h := &Handlers{db: db, mode: "production"}

	policy, err := h.ensureAdmissionPolicy()
	if err != nil {
		t.Fatalf("ensure admission policy: %v", err)
	}
	if policy.ID != defaultAdmissionPolicyID ||
		!policy.AllowLocalLogin ||
		!policy.AllowOIDCLogin ||
		!policy.RequireVerifiedOIDCEmail ||
		policy.DefaultRole != authz.PlatformRoleUser {
		t.Fatalf("unexpected default admission policy: %#v", policy)
	}
}

func TestEnsureAdmissionPolicyFailsClosedOnDatabaseError(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.Migrator().DropTable(&model.AuthAdmissionPolicy{}); err != nil {
		t.Fatalf("drop admission policy table: %v", err)
	}
	h := &Handlers{db: db, mode: "production"}

	policy, err := h.ensureAdmissionPolicy()
	if err == nil || !strings.Contains(err.Error(), "load authentication admission policy") {
		t.Fatalf("ensure admission policy error = %v", err)
	}
	if policy.AllowLocalLogin || policy.AllowOIDCLogin {
		t.Fatalf("database failure returned permissive policy: %#v", policy)
	}
}

func TestEnsureAdmissionPolicyReturnsInitializationError(t *testing.T) {
	db := authIntegrationDB(t)
	tx := db.Begin(&sql.TxOptions{ReadOnly: true})
	if tx.Error != nil {
		t.Fatalf("begin read-only transaction: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
	})
	h := &Handlers{db: tx, mode: "production"}

	policy, err := h.ensureAdmissionPolicy()
	if err == nil || !strings.Contains(err.Error(), "initialize authentication admission policy") {
		t.Fatalf("ensure admission policy error = %v", err)
	}
	if policy.AllowLocalLogin || policy.AllowOIDCLogin {
		t.Fatalf("initialization failure returned permissive policy: %#v", policy)
	}
}
