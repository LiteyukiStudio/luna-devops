package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

type oauthApplicationBarrierContextKey struct{}

type oauthApplicationMutationResult struct {
	application model.OAuthApplication
	err         error
}

func TestOAuthApplicationUpdateCannotReviveCompletedDelete(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	application, _ := createOwnedOAuthApplicationFixture(t, fixture, "user:read")
	deleteCtx, deleteLocked, releaseDelete := installOAuthApplicationQueryBarrier(t, fixture.db)
	defer releaseDelete()

	deleteDone := make(chan oauthApplicationMutationResult, 1)
	go func() {
		deleted, err := deleteOwnedOAuthApplication(fixture.db.WithContext(deleteCtx), application.ID, fixture.user.ID)
		deleteDone <- oauthApplicationMutationResult{application: deleted, err: err}
	}()
	waitForOAuthApplicationBarrier(t, deleteLocked, "delete")

	next := application
	next.Name = "stale update"
	updateDone := make(chan oauthApplicationMutationResult, 1)
	go func() {
		updated, err := updateOwnedOAuthApplication(fixture.db, application.ID, fixture.user.ID, next)
		updateDone <- oauthApplicationMutationResult{application: updated, err: err}
	}()
	assertOAuthApplicationMutationBlocked(t, updateDone, "update")

	releaseDelete()
	if result := <-deleteDone; result.err != nil {
		t.Fatalf("delete OAuth application: %v", result.err)
	}
	if result := <-updateDone; !errors.Is(result.err, gorm.ErrRecordNotFound) {
		t.Fatalf("update after completed delete error = %v, want record not found", result.err)
	}

	stored := readOAuthApplication(t, fixture.db, application.ID)
	if stored.RevokedAt == nil || stored.Name != application.Name {
		t.Fatalf("deleted application was revived or overwritten: %#v", stored)
	}
}

func TestOAuthApplicationRotateCannotReviveCompletedDelete(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	application, _ := createOwnedOAuthApplicationFixture(t, fixture, "user:read")
	deleteCtx, deleteLocked, releaseDelete := installOAuthApplicationQueryBarrier(t, fixture.db)
	defer releaseDelete()

	deleteDone := make(chan oauthApplicationMutationResult, 1)
	go func() {
		deleted, err := deleteOwnedOAuthApplication(fixture.db.WithContext(deleteCtx), application.ID, fixture.user.ID)
		deleteDone <- oauthApplicationMutationResult{application: deleted, err: err}
	}()
	waitForOAuthApplicationBarrier(t, deleteLocked, "delete")

	rotatedHash := transportapi.HashToken("rotated-after-delete")
	rotateDone := make(chan oauthApplicationMutationResult, 1)
	go func() {
		rotated, err := rotateOwnedOAuthApplicationSecret(fixture.db, application.ID, fixture.user.ID, rotatedHash)
		rotateDone <- oauthApplicationMutationResult{application: rotated, err: err}
	}()
	assertOAuthApplicationMutationBlocked(t, rotateDone, "secret rotation")

	releaseDelete()
	if result := <-deleteDone; result.err != nil {
		t.Fatalf("delete OAuth application: %v", result.err)
	}
	if result := <-rotateDone; !errors.Is(result.err, gorm.ErrRecordNotFound) {
		t.Fatalf("rotation after completed delete error = %v, want record not found", result.err)
	}

	stored := readOAuthApplication(t, fixture.db, application.ID)
	if stored.RevokedAt == nil || stored.ClientSecretHash != application.ClientSecretHash {
		t.Fatalf("deleted application was revived or rekeyed: %#v", stored)
	}
}

func TestOAuthApplicationScopeUpdatePreservesConcurrentRotatedSecret(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	application, _ := createOwnedOAuthApplicationFixture(t, fixture, "user:read,volume:export")
	grant, familyID := createOAuthApplicationCredentialFamily(t, fixture, application)
	rotateCtx, rotateLocked, releaseRotate := installOAuthApplicationQueryBarrier(t, fixture.db)
	defer releaseRotate()

	rotatedHash := transportapi.HashToken("new-authoritative-client-secret")
	rotateDone := make(chan oauthApplicationMutationResult, 1)
	go func() {
		rotated, err := rotateOwnedOAuthApplicationSecret(fixture.db.WithContext(rotateCtx), application.ID, fixture.user.ID, rotatedHash)
		rotateDone <- oauthApplicationMutationResult{application: rotated, err: err}
	}()
	waitForOAuthApplicationBarrier(t, rotateLocked, "secret rotation")

	next := application
	next.Name = "scope narrowed"
	next.AllowedScopes = "user:read"
	next.ClientSecretHash = application.ClientSecretHash
	updateDone := make(chan oauthApplicationMutationResult, 1)
	go func() {
		updated, err := updateOwnedOAuthApplication(fixture.db, application.ID, fixture.user.ID, next)
		updateDone <- oauthApplicationMutationResult{application: updated, err: err}
	}()
	assertOAuthApplicationMutationBlocked(t, updateDone, "scope update")

	releaseRotate()
	if result := <-rotateDone; result.err != nil {
		t.Fatalf("rotate OAuth application secret: %v", result.err)
	}
	if result := <-updateDone; result.err != nil {
		t.Fatalf("update OAuth application scope: %v", result.err)
	}

	stored := readOAuthApplication(t, fixture.db, application.ID)
	if stored.RevokedAt != nil || stored.ClientSecretHash != rotatedHash || stored.AllowedScopes != "user:read" {
		t.Fatalf("concurrent application state = %#v", stored)
	}
	var storedGrant model.OAuthGrant
	if err := fixture.db.First(&storedGrant, "id = ?", grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedGrant.RevokedAt == nil {
		t.Fatal("scope contraction left the prior consent grant active")
	}
	var activeCredentials int64
	if err := fixture.db.Model(&model.AccessToken{}).
		Where("oauth_grant_id = ? and oauth_family_id = ? and revoked_at is null", grant.ID, familyID).
		Count(&activeCredentials).Error; err != nil {
		t.Fatal(err)
	}
	if activeCredentials != 0 {
		t.Fatalf("scope contraction left %d access credentials active", activeCredentials)
	}
	if err := fixture.db.Model(&model.OAuthRefreshToken{}).
		Where("grant_id = ? and family_id = ? and revoked_at is null", grant.ID, familyID).
		Count(&activeCredentials).Error; err != nil {
		t.Fatal(err)
	}
	if activeCredentials != 0 {
		t.Fatalf("scope contraction left %d refresh credentials active", activeCredentials)
	}
}

func TestOAuthAuthorizationCodeRevalidatesSecretAfterConcurrentRotation(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	application, oldSecret := createOwnedOAuthApplicationFixture(t, fixture, "user:read")
	plainCode, verifier := createOAuthApplicationAuthorizationCode(t, fixture, application)
	authCtx, authenticationRead, releaseAuthentication := installOAuthApplicationQueryBarrier(t, fixture.db)
	defer releaseAuthentication()

	oldExchangeDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		oldExchangeDone <- performFormRequestWithContext(authCtx, fixture.router, http.MethodPost, "/api/v1/oauth/token", url.Values{
			"client_id":     {application.ClientID},
			"client_secret": {oldSecret},
			"grant_type":    {"authorization_code"},
			"code":          {plainCode},
			"redirect_uri":  {decodeStringList(application.RedirectURIs)[0]},
			"code_verifier": {verifier},
		})
	}()
	waitForOAuthApplicationBarrier(t, authenticationRead, "client authentication")

	newSecret := "new-client-secret-" + transportapi.RandomHex(8)
	if _, err := rotateOwnedOAuthApplicationSecret(fixture.db, application.ID, fixture.user.ID, transportapi.HashToken(newSecret)); err != nil {
		t.Fatalf("rotate OAuth application secret: %v", err)
	}
	releaseAuthentication()

	oldExchange := <-oldExchangeDone
	if oldExchange.Code != http.StatusUnauthorized || jsonString(t, oldExchange.Body.Bytes(), "error") != "invalid_client" {
		t.Fatalf("old-secret exchange after rotation = %d %s", oldExchange.Code, oldExchange.Body.String())
	}
	var code model.OAuthAuthorizationCode
	if err := fixture.db.First(&code, "code_hash = ?", transportapi.HashToken(plainCode)).Error; err != nil {
		t.Fatal(err)
	}
	if code.ConsumedAt != nil || code.GrantID != nil {
		t.Fatalf("old-secret exchange consumed authorization code: %#v", code)
	}
	assertOAuthApplicationCredentialCount(t, fixture.db, application.ID, 0)

	newExchange := performFormRequest(fixture.router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"client_id":     {application.ClientID},
		"client_secret": {newSecret},
		"grant_type":    {"authorization_code"},
		"code":          {plainCode},
		"redirect_uri":  {decodeStringList(application.RedirectURIs)[0]},
		"code_verifier": {verifier},
	})
	if newExchange.Code != http.StatusOK {
		t.Fatalf("new-secret exchange = %d %s", newExchange.Code, newExchange.Body.String())
	}
	assertOAuthApplicationCredentialCount(t, fixture.db, application.ID, 2)
}

func TestOAuthTokenRevokeRevalidatesSecretAfterConcurrentRotation(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	application, oldSecret := createOwnedOAuthApplicationFixture(t, fixture, "user:read")
	grant, familyID := createOAuthApplicationCredentialFamily(t, fixture, application)
	plainToken := "access-" + familyID
	authCtx, authenticationRead, releaseAuthentication := installOAuthApplicationQueryBarrier(t, fixture.db)
	defer releaseAuthentication()

	oldRevokeDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		oldRevokeDone <- performFormRequestWithContext(authCtx, fixture.router, http.MethodPost, "/api/v1/oauth/revoke", url.Values{
			"client_id":     {application.ClientID},
			"client_secret": {oldSecret},
			"token":         {plainToken},
		})
	}()
	waitForOAuthApplicationBarrier(t, authenticationRead, "revocation client authentication")

	newSecret := "new-revocation-secret-" + transportapi.RandomHex(8)
	if _, err := rotateOwnedOAuthApplicationSecret(fixture.db, application.ID, fixture.user.ID, transportapi.HashToken(newSecret)); err != nil {
		t.Fatalf("rotate OAuth application secret: %v", err)
	}
	releaseAuthentication()

	oldRevoke := <-oldRevokeDone
	if oldRevoke.Code != http.StatusUnauthorized || jsonString(t, oldRevoke.Body.Bytes(), "error") != "invalid_client" {
		t.Fatalf("old-secret revocation after rotation = %d %s", oldRevoke.Code, oldRevoke.Body.String())
	}
	assertOAuthFamilyAccessTokenActive(t, fixture.db, grant.ID, familyID)

	newRevoke := performFormRequest(fixture.router, http.MethodPost, "/api/v1/oauth/revoke", url.Values{
		"client_id":     {application.ClientID},
		"client_secret": {newSecret},
		"token":         {plainToken},
	})
	if newRevoke.Code != http.StatusOK {
		t.Fatalf("new-secret revocation = %d %s", newRevoke.Code, newRevoke.Body.String())
	}
	assertOAuthFamilyRevoked(t, fixture.db, grant.ID, familyID)
}

func createOwnedOAuthApplicationFixture(t *testing.T, fixture oauthFamilyTestFixture, allowedScopes string) (model.OAuthApplication, string) {
	t.Helper()
	suffix := transportapi.RandomHex(4)
	ownerUserID := fixture.user.ID
	clientSecret := "oauth-application-secret-" + suffix
	application := model.OAuthApplication{
		ID: "oapp_concurrency_" + suffix, OwnerUserID: &ownerUserID, Name: "Concurrency Client",
		ClientID: "oauth-concurrency-client-" + suffix, ClientSecretHash: transportapi.HashToken(clientSecret),
		RedirectURIs:  encodeStringList([]string{"https://client.example.com/callback"}),
		AllowedScopes: allowedScopes, AccessTokenLifetimeDays: 30,
	}
	if err := fixture.db.Create(&application).Error; err != nil {
		t.Fatal(err)
	}
	return application, clientSecret
}

func createOAuthApplicationCredentialFamily(t *testing.T, fixture oauthFamilyTestFixture, application model.OAuthApplication) (model.OAuthGrant, string) {
	t.Helper()
	familyID := id.New("ofam")
	grant := model.OAuthGrant{
		ID: id.New("ogrt"), ApplicationID: application.ID, UserID: fixture.user.ID, Scope: application.AllowedScopes,
	}
	if err := fixture.db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&model.AccessToken{
		ID: id.New("tok"), UserID: fixture.user.ID, Name: application.Name, Scope: application.AllowedScopes,
		TokenHash: transportapi.HashToken("access-" + familyID), Source: "oauth", OAuthApplicationID: application.ID,
		OAuthGrantID: grant.ID, OAuthFamilyID: familyID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&model.OAuthRefreshToken{
		ID: id.New("ortk"), ApplicationID: application.ID, GrantID: grant.ID, FamilyID: familyID,
		UserID: fixture.user.ID, TokenHash: transportapi.HashToken("refresh-" + familyID), Scope: application.AllowedScopes,
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	return grant, familyID
}

func createOAuthApplicationAuthorizationCode(t *testing.T, fixture oauthFamilyTestFixture, application model.OAuthApplication) (string, string) {
	t.Helper()
	plainCode := "lyo_code_" + transportapi.RandomHex(16)
	verifier := strings.Repeat("v", 64)
	digest := sha256.Sum256([]byte(verifier))
	code := model.OAuthAuthorizationCode{
		ID: id.New("ocod"), ApplicationID: application.ID, UserID: fixture.user.ID,
		CodeHash: transportapi.HashToken(plainCode), RedirectURI: decodeStringList(application.RedirectURIs)[0], Scope: "user:read",
		CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: "S256",
		ExpiresAt: time.Now().Add(time.Minute),
	}
	if err := fixture.db.Transaction(func(tx *gorm.DB) error { return recordOAuthAuthorizationConsent(tx, &code) }); err != nil {
		t.Fatal(err)
	}
	return plainCode, verifier
}

func installOAuthApplicationQueryBarrier(t *testing.T, db *gorm.DB) (context.Context, <-chan struct{}, func()) {
	t.Helper()
	marker := "oauth-application-barrier-" + transportapi.RandomHex(4)
	reached := make(chan struct{})
	releaseChannel := make(chan struct{})
	var reachedOnce sync.Once
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseChannel) }) }
	callbackName := "test:" + marker
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "OAuthApplication" ||
			tx.Statement.Context == nil || tx.Statement.Context.Value(oauthApplicationBarrierContextKey{}) != marker {
			return
		}
		reachedOnce.Do(func() {
			close(reached)
			<-releaseChannel
		})
	}); err != nil {
		t.Fatalf("register OAuth application query barrier: %v", err)
	}
	t.Cleanup(func() {
		release()
		_ = db.Callback().Query().Remove(callbackName)
	})
	return context.WithValue(t.Context(), oauthApplicationBarrierContextKey{}, marker), reached, release
}

func waitForOAuthApplicationBarrier(t *testing.T, reached <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not reach the application query barrier", operation)
	}
}

func assertOAuthApplicationMutationBlocked(t *testing.T, done <-chan oauthApplicationMutationResult, operation string) {
	t.Helper()
	select {
	case result := <-done:
		t.Fatalf("%s completed before the application row lock was released: %v", operation, result.err)
	case <-time.After(150 * time.Millisecond):
	}
}

func readOAuthApplication(t *testing.T, db *gorm.DB, applicationID string) model.OAuthApplication {
	t.Helper()
	var application model.OAuthApplication
	if err := db.First(&application, "id = ?", applicationID).Error; err != nil {
		t.Fatal(err)
	}
	return application
}

func performFormRequestWithContext(ctx context.Context, router http.Handler, method, path string, form url.Values) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(form.Encode())).WithContext(ctx)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertOAuthApplicationCredentialCount(t *testing.T, db *gorm.DB, applicationID string, expected int64) {
	t.Helper()
	var accessCount int64
	if err := db.Model(&model.AccessToken{}).Where("oauth_application_id = ?", applicationID).Count(&accessCount).Error; err != nil {
		t.Fatal(err)
	}
	var refreshCount int64
	if err := db.Model(&model.OAuthRefreshToken{}).Where("application_id = ?", applicationID).Count(&refreshCount).Error; err != nil {
		t.Fatal(err)
	}
	if accessCount+refreshCount != expected {
		t.Fatalf("application credential count = access %d + refresh %d, want %d", accessCount, refreshCount, expected)
	}
}

func assertOAuthFamilyAccessTokenActive(t *testing.T, db *gorm.DB, grantID, familyID string) {
	t.Helper()
	var activeCount int64
	if err := db.Model(&model.AccessToken{}).
		Where("oauth_grant_id = ? and oauth_family_id = ? and revoked_at is null", grantID, familyID).
		Count(&activeCount).Error; err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("active OAuth family access tokens = %d, want 1", activeCount)
	}
}

func assertOAuthFamilyRevoked(t *testing.T, db *gorm.DB, grantID, familyID string) {
	t.Helper()
	var activeCount int64
	if err := db.Model(&model.AccessToken{}).
		Where("oauth_grant_id = ? and oauth_family_id = ? and revoked_at is null", grantID, familyID).
		Count(&activeCount).Error; err != nil {
		t.Fatal(err)
	}
	if activeCount != 0 {
		t.Fatalf("active OAuth family access tokens = %d, want 0", activeCount)
	}
	if err := db.Model(&model.OAuthRefreshToken{}).
		Where("grant_id = ? and family_id = ? and revoked_at is null", grantID, familyID).
		Count(&activeCount).Error; err != nil {
		t.Fatal(err)
	}
	if activeCount != 0 {
		t.Fatalf("active OAuth family refresh tokens = %d, want 0", activeCount)
	}
}
