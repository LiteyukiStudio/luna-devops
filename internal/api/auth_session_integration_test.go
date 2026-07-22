package api

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestInitializeAdminWithLockAllowsSingleConcurrentInitializer(t *testing.T) {
	db := authIntegrationDB(t)
	users := []model.User{
		{ID: "usr_bootstrap_a", Email: "bootstrap-a@example.com", Name: "A", Role: "platform_admin", Language: "en-US", Password: "hash"},
		{ID: "usr_bootstrap_b", Email: "bootstrap-b@example.com", Name: "B", Role: "platform_admin", Language: "en-US", Password: "hash"},
	}

	start := make(chan struct{})
	errorsByAttempt := make(chan error, len(users))
	var workers sync.WaitGroup
	for _, user := range users {
		user := user
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			errorsByAttempt <- initializeAdminWithLock(db, user)
		}()
	}
	close(start)
	workers.Wait()
	close(errorsByAttempt)

	succeeded := 0
	alreadyInitialized := 0
	for err := range errorsByAttempt {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, errBootstrapAlreadyInitialized):
			alreadyInitialized++
		default:
			t.Fatalf("unexpected bootstrap error: %v", err)
		}
	}
	if succeeded != 1 || alreadyInitialized != 1 {
		t.Fatalf("bootstrap results: succeeded=%d alreadyInitialized=%d", succeeded, alreadyInitialized)
	}

	var adminCount int64
	if err := db.Model(&model.User{}).Where("role = ?", "platform_admin").Count(&adminCount).Error; err != nil {
		t.Fatalf("count administrators: %v", err)
	}
	if adminCount != 1 {
		t.Fatalf("administrator count = %d", adminCount)
	}
}

func TestRotateRememberLoginConsumesTokenOnce(t *testing.T) {
	db := authIntegrationDB(t)
	user := model.User{ID: "usr_remember", Email: "remember@example.com", Name: "Remember", Role: "user", Language: "en-US", Password: "hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	plainToken := "rem_original"
	original := model.UserRememberToken{
		ID:        "rem_original",
		UserID:    user.ID,
		FamilyID:  "remf_concurrent",
		TokenHash: hashToken(plainToken),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&original).Error; err != nil {
		t.Fatalf("create remember token: %v", err)
	}
	h := &Handlers{db: db, mode: "production"}

	start := make(chan struct{})
	errorsByAttempt := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, _, _, err := h.rotateRememberLogin(user.ID, plainToken)
			errorsByAttempt <- err
		}()
	}
	close(start)
	workers.Wait()
	close(errorsByAttempt)

	succeeded := 0
	rejected := 0
	for err := range errorsByAttempt {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, errRememberTokenReused):
			rejected++
		default:
			t.Fatalf("unexpected rotation error: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("rotation results: succeeded=%d rejected=%d", succeeded, rejected)
	}

	var rememberCount int64
	if err := db.Model(&model.UserRememberToken{}).Where("user_id = ?", user.ID).Count(&rememberCount).Error; err != nil {
		t.Fatalf("count remember tokens: %v", err)
	}
	var sessionCount int64
	if err := db.Model(&model.UserSession{}).Where("user_id = ?", user.ID).Count(&sessionCount).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if rememberCount != 2 || sessionCount != 0 {
		t.Fatalf("credential counts: remember=%d session=%d", rememberCount, sessionCount)
	}
	assertRecordCount(t, db, &model.UserRememberToken{}, "user_id = ? and consumed_at is not null", []any{user.ID}, 1)
	assertRecordCount(t, db, &model.UserRememberToken{}, "user_id = ? and revoked_at is not null", []any{user.ID}, 2)
}

func TestOIDCRegistrationToggleOnlyBlocksNewUsers(t *testing.T) {
	db := authIntegrationDB(t)
	settings := model.AuthRegistrationSettings{ID: authRegistrationSettingsID, AllowOIDCRegistration: true, SMTPPort: 587, SMTPSecurity: "starttls"}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatalf("create registration settings: %v", err)
	}
	if err := db.Model(&settings).Update("allow_oidc_registration", false).Error; err != nil {
		t.Fatalf("disable OIDC registration: %v", err)
	}
	if err := db.Create(&model.AuthAdmissionPolicy{ID: defaultAdmissionPolicyID, AllowLocalLogin: true, AllowOIDCLogin: true, RequireVerifiedOIDCEmail: true, DefaultRole: "user"}).Error; err != nil {
		t.Fatalf("create admission policy: %v", err)
	}
	provider := model.AuthProvider{ID: "oidcp_registration_toggle", Type: "oidc", Name: "OIDC", Enabled: true}
	if err := db.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	h := &Handlers{db: db, mode: "production"}

	newClaims := oidcIdentityClaims{Subject: "new-subject", Email: "new-oidc@example.com", EmailVerified: true, Name: "New OIDC User"}
	if _, err := h.findOrCreateOIDCUser(provider, newClaims); !errors.Is(err, errOIDCRegistrationDisabled) {
		t.Fatalf("new OIDC user error = %v, want registration disabled", err)
	}

	existing := model.User{ID: "usr_existing_oidc", Email: "existing-oidc@example.com", Name: "Existing", Role: "user", Language: "en-US"}
	identity := model.ExternalIdentity{ID: "ext_existing_oidc", UserID: existing.ID, ProviderID: provider.ID, Subject: "existing-subject", Email: existing.Email, EmailVerified: true}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}
	if err := db.Create(&identity).Error; err != nil {
		t.Fatalf("create existing identity: %v", err)
	}
	loggedIn, err := h.findOrCreateOIDCUser(provider, oidcIdentityClaims{Subject: identity.Subject, Email: existing.Email, EmailVerified: true})
	if err != nil || loggedIn.ID != existing.ID {
		t.Fatalf("existing OIDC login = %#v, %v", loggedIn, err)
	}
}

func TestRememberTokenReplayRevokesCompromisedFamilyOnly(t *testing.T) {
	db := authIntegrationDB(t)
	user := model.User{ID: "usr_replay", Email: "replay@example.com", Name: "Replay", Role: "user", Language: "en-US", Password: "hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now()
	plainToken := "rem_replay_original"
	compromised := model.UserRememberToken{
		ID:        "rem_replay_original",
		UserID:    user.ID,
		FamilyID:  "remf_compromised",
		TokenHash: hashToken(plainToken),
		ExpiresAt: now.Add(time.Hour),
	}
	unrelatedToken, _ := newUserRememberTokenInFamily(user.ID, "remf_unrelated", now.Add(time.Hour))
	unrelatedSession, _ := newUserSessionInFamily(user.ID, "", unrelatedToken.FamilyID, now)
	if err := db.Create(&[]model.UserRememberToken{compromised, unrelatedToken}).Error; err != nil {
		t.Fatalf("create remember tokens: %v", err)
	}
	if err := db.Create(&unrelatedSession).Error; err != nil {
		t.Fatalf("create unrelated session: %v", err)
	}

	h := &Handlers{db: db, mode: "production"}
	if _, _, _, err := h.rotateRememberLogin(user.ID, plainToken); err != nil {
		t.Fatalf("rotate remember token: %v", err)
	}
	var rotatedSession model.UserSession
	if err := db.First(&rotatedSession, "user_id = ? and remember_family_id = ?", user.ID, compromised.FamilyID).Error; err != nil {
		t.Fatalf("find rotated session: %v", err)
	}
	assertion := newTestStepUpAssertion("sua_replayed_family", user.ID, rotatedSession.ID)
	if err := db.Create(&assertion).Error; err != nil {
		t.Fatalf("create family assertion: %v", err)
	}

	if _, _, _, err := h.rotateRememberLogin(user.ID, plainToken); !errors.Is(err, errRememberTokenReused) {
		t.Fatalf("replay error = %v", err)
	}
	assertRecordCount(t, db, &model.UserSession{}, "user_id = ? and remember_family_id = ?", []any{user.ID, compromised.FamilyID}, 0)
	assertRecordCount(t, db, &model.StepUpAssertion{}, "id = ?", []any{assertion.ID}, 0)
	assertRecordCount(t, db, &model.UserRememberToken{}, "user_id = ? and family_id = ? and revoked_at is null", []any{user.ID, compromised.FamilyID}, 0)
	assertRecordCount(t, db, &model.UserSession{}, "id = ?", []any{unrelatedSession.ID}, 1)
	assertRecordCount(t, db, &model.UserRememberToken{}, "id = ? and revoked_at is null", []any{unrelatedToken.ID}, 1)
}

func TestRememberRotationPreservesPrimaryAuthenticationAndSingleSession(t *testing.T) {
	db := authIntegrationDB(t)
	user := model.User{ID: "usr_rotate_family", Email: "rotate-family@example.com", Name: "Rotate Family", Role: "user", Language: "en-US"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now()
	primaryAuthenticatedAt := now.Add(-2 * time.Hour)
	familyExpiresAt := now.Add(12 * time.Hour)
	remember, rememberPlainToken := newUserRememberTokenInFamily(user.ID, "remf_rotate_family", familyExpiresAt)
	oldSession, _ := newUserSessionInFamilyWithPrimaryAuthentication(user.ID, "", remember.FamilyID, now.Add(-time.Hour), &primaryAuthenticatedAt, now.Add(time.Hour))
	if err := db.Create(&remember).Error; err != nil {
		t.Fatalf("create remember token: %v", err)
	}
	if err := db.Create(&oldSession).Error; err != nil {
		t.Fatalf("create old session: %v", err)
	}
	assertion := newTestStepUpAssertion("sua_rotate_family", user.ID, oldSession.ID)
	if err := db.Create(&assertion).Error; err != nil {
		t.Fatalf("create old assertion: %v", err)
	}

	h := &Handlers{db: db, mode: "production"}
	_, _, rotatedRememberToken, err := h.rotateRememberLogin(user.ID, rememberPlainToken)
	if err != nil {
		t.Fatalf("first rotation: %v", err)
	}
	assertRecordCount(t, db, &model.UserSession{}, "user_id = ? and remember_family_id = ?", []any{user.ID, remember.FamilyID}, 1)
	assertRecordCount(t, db, &model.StepUpAssertion{}, "id = ?", []any{assertion.ID}, 0)
	var firstRotated model.UserSession
	if err := db.First(&firstRotated, "user_id = ? and remember_family_id = ?", user.ID, remember.FamilyID).Error; err != nil {
		t.Fatalf("read first rotated session: %v", err)
	}
	if !postgresTimestampEqual(firstRotated.PrimaryAuthenticatedAt, primaryAuthenticatedAt) {
		t.Fatalf("primary authentication changed: got=%v want=%v", firstRotated.PrimaryAuthenticatedAt, primaryAuthenticatedAt)
	}
	if firstRotated.ExpiresAt.After(familyExpiresAt) {
		t.Fatalf("session expiry %v exceeds family expiry %v", firstRotated.ExpiresAt, familyExpiresAt)
	}

	if _, _, _, err := h.rotateRememberLogin(user.ID, rotatedRememberToken); err != nil {
		t.Fatalf("second rotation: %v", err)
	}
	assertRecordCount(t, db, &model.UserSession{}, "user_id = ? and remember_family_id = ?", []any{user.ID, remember.FamilyID}, 1)
	var secondRotated model.UserSession
	if err := db.First(&secondRotated, "user_id = ? and remember_family_id = ?", user.ID, remember.FamilyID).Error; err != nil {
		t.Fatalf("read second rotated session: %v", err)
	}
	if !postgresTimestampEqual(secondRotated.PrimaryAuthenticatedAt, primaryAuthenticatedAt) {
		t.Fatalf("primary authentication refreshed after second rotation: got=%v want=%v", secondRotated.PrimaryAuthenticatedAt, primaryAuthenticatedAt)
	}
}

func TestLogoutNonRememberedSessionLeavesRememberFamiliesAlone(t *testing.T) {
	db := authIntegrationDB(t)
	user := model.User{ID: "usr_logout", Email: "logout@example.com", Name: "Logout", Role: "user", Language: "en-US", Password: "hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	currentSession, currentPlainToken := newUserSession(user.ID, "", time.Now())
	otherSession, _ := newUserSession(user.ID, "", time.Now())
	if err := db.Create(&[]model.UserSession{currentSession, otherSession}).Error; err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	firstRemember, _ := newUserRememberToken(user.ID, time.Now())
	secondRemember, _ := newUserRememberToken(user.ID, time.Now())
	if err := db.Create(&[]model.UserRememberToken{firstRemember, secondRemember}).Error; err != nil {
		t.Fatalf("create remember tokens: %v", err)
	}
	assertions := []model.StepUpAssertion{
		newTestStepUpAssertion("sua_current", user.ID, currentSession.ID),
		newTestStepUpAssertion("sua_other", user.ID, otherSession.ID),
	}
	if err := db.Create(&assertions).Error; err != nil {
		t.Fatalf("create assertions: %v", err)
	}

	h := &Handlers{db: db}
	userID, err := h.revokeCurrentSessionAndRememberTokens(currentPlainToken)
	if err != nil {
		t.Fatalf("revoke logout credentials: %v", err)
	}
	if userID != user.ID {
		t.Fatalf("revoked user ID = %q", userID)
	}

	assertRecordCount(t, db, &model.UserSession{}, "user_id = ?", []any{user.ID}, 1)
	assertRecordCount(t, db, &model.UserRememberToken{}, "user_id = ?", []any{user.ID}, 2)
	assertRecordCount(t, db, &model.UserRememberToken{}, "user_id = ? and revoked_at is null", []any{user.ID}, 2)
	assertRecordCount(t, db, &model.StepUpAssertion{}, "session_id = ?", []any{currentSession.ID}, 0)
	assertRecordCount(t, db, &model.StepUpAssertion{}, "session_id = ?", []any{otherSession.ID}, 1)
}

func TestLogoutRememberedSessionRevokesOnlyCurrentFamily(t *testing.T) {
	db := authIntegrationDB(t)
	user := model.User{ID: "usr_family_logout", Email: "family-logout@example.com", Name: "Family Logout", Role: "user", Language: "en-US", Password: "hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now()
	currentRemember, _ := newUserRememberTokenInFamily(user.ID, "remf_logout_current", now.Add(time.Hour))
	unrelatedRemember, _ := newUserRememberTokenInFamily(user.ID, "remf_logout_other", now.Add(time.Hour))
	currentSession, currentPlainToken := newUserSessionInFamily(user.ID, "", currentRemember.FamilyID, now)
	staleFamilySession, _ := newUserSessionInFamily(user.ID, "", currentRemember.FamilyID, now.Add(-time.Minute))
	unrelatedSession, _ := newUserSessionInFamily(user.ID, "", unrelatedRemember.FamilyID, now)
	if err := db.Create(&[]model.UserRememberToken{currentRemember, unrelatedRemember}).Error; err != nil {
		t.Fatalf("create remember tokens: %v", err)
	}
	if err := db.Create(&[]model.UserSession{currentSession, staleFamilySession, unrelatedSession}).Error; err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	assertions := []model.StepUpAssertion{
		newTestStepUpAssertion("sua_family_current", user.ID, currentSession.ID),
		newTestStepUpAssertion("sua_family_stale", user.ID, staleFamilySession.ID),
		newTestStepUpAssertion("sua_family_other", user.ID, unrelatedSession.ID),
	}
	if err := db.Create(&assertions).Error; err != nil {
		t.Fatalf("create assertions: %v", err)
	}

	h := &Handlers{db: db}
	if _, err := h.revokeCurrentSessionAndRememberTokens(currentPlainToken); err != nil {
		t.Fatalf("logout remembered session: %v", err)
	}
	assertRecordCount(t, db, &model.UserSession{}, "user_id = ? and remember_family_id = ?", []any{user.ID, currentRemember.FamilyID}, 0)
	assertRecordCount(t, db, &model.StepUpAssertion{}, "id in ?", []any{[]string{"sua_family_current", "sua_family_stale"}}, 0)
	assertRecordCount(t, db, &model.UserRememberToken{}, "id = ? and revoked_at is not null", []any{currentRemember.ID}, 1)
	assertRecordCount(t, db, &model.UserSession{}, "id = ?", []any{unrelatedSession.ID}, 1)
	assertRecordCount(t, db, &model.StepUpAssertion{}, "id = ?", []any{"sua_family_other"}, 1)
	assertRecordCount(t, db, &model.UserRememberToken{}, "id = ? and revoked_at is null", []any{unrelatedRemember.ID}, 1)
}

func TestExpiredRememberTombstonesAreDeletedOnlyAfterWholeFamilyExpires(t *testing.T) {
	db := authIntegrationDB(t)
	user := model.User{ID: "usr_tombstone", Email: "tombstone@example.com", Name: "Tombstone", Role: "user", Language: "en-US", Password: "hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now()
	expiredOne, _ := newUserRememberTokenInFamily(user.ID, "remf_expired", now.Add(-2*time.Hour))
	expiredTwo, _ := newUserRememberTokenInFamily(user.ID, "remf_expired", now.Add(-time.Hour))
	mixedExpired, _ := newUserRememberTokenInFamily(user.ID, "remf_mixed", now.Add(-time.Hour))
	mixedActive, _ := newUserRememberTokenInFamily(user.ID, "remf_mixed", now.Add(time.Hour))
	if err := db.Create(&[]model.UserRememberToken{expiredOne, expiredTwo, mixedExpired, mixedActive}).Error; err != nil {
		t.Fatalf("create remember tombstones: %v", err)
	}
	expiredSession, _ := newUserSessionInFamily(user.ID, "", expiredOne.FamilyID, now.Add(-time.Hour))
	mixedSession, _ := newUserSessionInFamily(user.ID, "", mixedActive.FamilyID, now)
	if err := db.Create(&[]model.UserSession{expiredSession, mixedSession}).Error; err != nil {
		t.Fatalf("create family sessions: %v", err)
	}
	expiredAssertion := newTestStepUpAssertion("sua_expired_family", user.ID, expiredSession.ID)
	if err := db.Create(&expiredAssertion).Error; err != nil {
		t.Fatalf("create expired-family assertion: %v", err)
	}

	h := &Handlers{db: db}
	if err := h.cleanupExpiredRememberTokenFamilies(user.ID, now); err != nil {
		t.Fatalf("cleanup expired families: %v", err)
	}
	assertRecordCount(t, db, &model.UserRememberToken{}, "family_id = ?", []any{expiredOne.FamilyID}, 0)
	assertRecordCount(t, db, &model.UserSession{}, "remember_family_id = ?", []any{expiredOne.FamilyID}, 0)
	assertRecordCount(t, db, &model.StepUpAssertion{}, "id = ?", []any{expiredAssertion.ID}, 0)
	assertRecordCount(t, db, &model.UserRememberToken{}, "family_id = ?", []any{mixedActive.FamilyID}, 2)
	assertRecordCount(t, db, &model.UserSession{}, "id = ?", []any{mixedSession.ID}, 1)
}

func TestRevokeUserAuthenticationClearsEverySession(t *testing.T) {
	db := authIntegrationDB(t)
	user := model.User{ID: "usr_revoke", Email: "revoke@example.com", Name: "Revoke", Role: "user", Language: "en-US", Password: "hash"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	firstSession, _ := newUserSession(user.ID, "", time.Now())
	secondSession, _ := newUserSession(user.ID, "", time.Now())
	if err := db.Create(&[]model.UserSession{firstSession, secondSession}).Error; err != nil {
		t.Fatalf("create sessions: %v", err)
	}
	remember, _ := newUserRememberToken(user.ID, time.Now())
	if err := db.Create(&remember).Error; err != nil {
		t.Fatalf("create remember token: %v", err)
	}
	assertion := newTestStepUpAssertion("sua_revoke", user.ID, firstSession.ID)
	if err := db.Create(&assertion).Error; err != nil {
		t.Fatalf("create assertion: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return revokeUserAuthentication(tx, user.ID)
	}); err != nil {
		t.Fatalf("revoke authentication: %v", err)
	}

	assertRecordCount(t, db, &model.UserSession{}, "user_id = ?", []any{user.ID}, 0)
	assertRecordCount(t, db, &model.UserRememberToken{}, "user_id = ?", []any{user.ID}, 1)
	assertRecordCount(t, db, &model.UserRememberToken{}, "user_id = ? and revoked_at is not null", []any{user.ID}, 1)
	assertRecordCount(t, db, &model.StepUpAssertion{}, "user_id = ?", []any{user.ID}, 0)
}

func newTestStepUpAssertion(assertionID, userID, sessionID string) model.StepUpAssertion {
	now := time.Now()
	return model.StepUpAssertion{
		ID:                assertionID,
		UserID:            userID,
		SessionID:         sessionID,
		Purpose:           "runtime_exec",
		VerifiedAt:        now,
		LastActivityAt:    now,
		IdleExpiresAt:     now.Add(time.Hour),
		AbsoluteExpiresAt: now.Add(time.Hour),
	}
}

func assertRecordCount(t *testing.T, db *gorm.DB, value any, query string, args []any, expected int64) {
	t.Helper()
	var count int64
	if err := db.Model(value).Where(query, args...).Count(&count).Error; err != nil {
		t.Fatalf("count %T records: %v", value, err)
	}
	if count != expected {
		t.Fatalf("%T record count = %d, want %d", value, count, expected)
	}
}

func postgresTimestampEqual(actual *time.Time, expected time.Time) bool {
	return actual != nil && actual.UTC().Truncate(time.Microsecond).Equal(expected.UTC().Truncate(time.Microsecond))
}

func authIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}

	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schema := fmt.Sprintf("auth_session_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatalf("create integration schema: %v", err)
	}

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schema)
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open schema database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Project{},
		&model.ProjectMember{},
		&model.AuthProvider{},
		&model.ExternalIdentity{},
		&model.AuthAdmissionPolicy{},
		&model.AuthRegistrationSettings{},
		&model.UserSession{},
		&model.UserRememberToken{},
		&model.StepUpAssertion{},
	); err != nil {
		t.Fatalf("migrate integration schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		_ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
